package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/players"
	"league_app/models"
)

// PlayerStore implements players.PlayerStore against a SQLite database.
type PlayerStore struct {
	db *sql.DB
}

// NewPlayerStore returns a PlayerStore backed by db.
func NewPlayerStore(db *sql.DB) *PlayerStore {
	return &PlayerStore{db: db}
}

// playerCols is the SELECT column list for full player rows.
// Requires the query to alias players as p and LEFT JOIN teams as t.
// Surrounded by single spaces so it can be concatenated with SELECT and FROM.
const playerCols = ` p.id, COALESCE(p.player_number,''), p.first_name, p.last_name,` +
	` p.first_name || ' ' || p.last_name,` +
	` COALESCE(p.phone,''), COALESCE(p.email,''),` +
	` p.team_id, COALESCE(t.name,''), COALESCE(t.league_id,0),` +
	` p.handicap, p.admin_hold, COALESCE(p.active,1), COALESCE(p.note,''),` +
	` p.created_at `

const playerJoin = ` FROM players p LEFT JOIN teams t ON t.id = p.team_id `

func scanPlayer(row interface{ Scan(...any) error }) (models.Player, error) {
	var p models.Player
	var adminHold int
	var activeInt int
	err := row.Scan(
		&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name,
		&p.Phone, &p.Email, &p.TeamID, &p.TeamName, &p.LeagueID,
		&p.Handicap, &adminHold, &activeInt, &p.Note, &p.CreatedAt,
	)
	if err != nil {
		return models.Player{}, err
	}
	p.AdminHold = adminHold == 1
	p.Active = activeInt == 1
	return p, nil
}

func (s *PlayerStore) ListPlayers(ctx context.Context, leagueID *int64) ([]models.Player, error) {
	var rows *sql.Rows
	var err error
	if leagueID != nil {
		rows, err = s.db.QueryContext(ctx,
			`SELECT`+playerCols+playerJoin+`WHERE t.league_id = ? ORDER BY p.last_name, p.first_name`,
			*leagueID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT`+playerCols+playerJoin+`ORDER BY p.last_name, p.first_name`)
	}
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	defer rows.Close()
	out := []models.Player{}
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PlayerStore) GetPlayer(ctx context.Context, id int64) (models.Player, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+playerCols+playerJoin+`WHERE p.id = ?`, id)
	p, err := scanPlayer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Player{}, fmt.Errorf("player %d: %w", id, players.ErrNotFound)
	}
	if err != nil {
		return models.Player{}, fmt.Errorf("get player %d: %w", id, err)
	}
	return p, nil
}

// CreatePlayer inserts a player row and returns the stored fields without
// re-fetching created_at, matching the previous handler's response shape.
func (s *PlayerStore) CreatePlayer(ctx context.Context, input players.CreatePlayerInput) (models.Player, error) {
	adminHold := 0
	if input.AdminHold {
		adminHold = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO players (player_number, first_name, last_name, phone, email, team_id, handicap, admin_hold)
		 VALUES (?,?,?,?,?,?,?,?)`,
		input.PlayerNumber, input.FirstName, input.LastName,
		input.Phone, input.Email, input.TeamID, input.Handicap, adminHold)
	if err != nil {
		return models.Player{}, fmt.Errorf("create player: %w", err)
	}
	id, _ := res.LastInsertId()
	return models.Player{
		ID:           id,
		PlayerNumber: input.PlayerNumber,
		FirstName:    input.FirstName,
		LastName:     input.LastName,
		Name:         input.FirstName + " " + input.LastName,
		Phone:        input.Phone,
		Email:        input.Email,
		TeamID:       input.TeamID,
		Handicap:     input.Handicap,
		AdminHold:    input.AdminHold,
	}, nil
}

// UpdatePlayer updates mutable player fields. PlayerNumber is intentionally
// excluded from the UPDATE — it is locked once set on creation.
// No error is returned when the row does not exist (UPDATE affects 0 rows).
func (s *PlayerStore) UpdatePlayer(ctx context.Context, id int64, input players.UpdatePlayerInput) error {
	adminHold := 0
	if input.AdminHold {
		adminHold = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE players SET first_name=?, last_name=?, phone=?, email=?,
		 team_id=?, handicap=?, admin_hold=? WHERE id=?`,
		input.FirstName, input.LastName, input.Phone, input.Email,
		input.TeamID, input.Handicap, adminHold, id)
	if err != nil {
		return fmt.Errorf("update player %d: %w", id, err)
	}
	return nil
}

// DeletePlayer removes a player by ID. Returns ErrHasHistory when
// handicap_history records exist for the player (business rule: deactivate
// players with history instead of deleting them).
// No error is returned when the row does not exist and no history is found.
func (s *PlayerStore) DeletePlayer(ctx context.Context, id int64) error {
	var historyCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM handicap_history WHERE player_id = ?`, id,
	).Scan(&historyCount); err != nil {
		return fmt.Errorf("check player history %d: %w", id, err)
	}
	if historyCount > 0 {
		return players.ErrHasHistory
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM players WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete player %d: %w", id, err)
	}
	return nil
}

// mergeRepointStmts lists every UPDATE needed to move a player reference from
// one ID to another. Each entry updates exactly one column; a table with two
// player-referencing columns (lineup_plans, round_results) appears twice.
// Kept as package-level data so MergePlayers's transaction loop stays short.
var mergeRepointStmts = []string{
	`UPDATE match_results   SET player_id      = ? WHERE player_id      = ?`,
	`UPDATE handicap_history SET player_id     = ? WHERE player_id      = ?`,
	`UPDATE round_results   SET home_player_id = ? WHERE home_player_id = ?`,
	`UPDATE round_results   SET away_player_id = ? WHERE away_player_id = ?`,
	`UPDATE lineup_plans    SET player_id      = ? WHERE player_id      = ?`,
	`UPDATE lineup_plans    SET sub_for_id     = ? WHERE sub_for_id     = ?`,
	`UPDATE season_teams    SET captain_id     = ? WHERE captain_id     = ?`,
	`UPDATE season_rosters  SET player_id      = ? WHERE player_id      = ?`,
	`UPDATE teams           SET captain_id     = ? WHERE captain_id     = ?`,
}

// MergePlayers repoints every supported player reference from sourceID to
// targetID, then deletes the source player. Runs in one transaction: blocker
// checks, all repointing, and the delete either all succeed or all roll back.
//
// Blocker checks run against the transaction's own view, immediately before
// the writes, so a conflict introduced between an earlier read and this call
// cannot silently corrupt a merge -- it either isn't seen (fine, still safe)
// or the transaction's write later fails on the same unique constraint the
// check exists to catch, and the whole merge rolls back.
//
// Assumes the caller (PlayerService.MergePlayers) has already confirmed
// sourceID != targetID and that both players exist.
func (s *PlayerStore) MergePlayers(ctx context.Context, sourceID, targetID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("merge players %d into %d: begin tx: %w", sourceID, targetID, err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Blocker: season_rosters -- both players already have a row for the same
	// season (regardless of team). Covers the "different teams, same season"
	// case explicitly, since season_rosters has no team_id in its unique key.
	var rosterConflicts int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM season_rosters src
		JOIN   season_rosters tgt
		       ON tgt.season_id = src.season_id AND tgt.player_id = ?
		WHERE  src.player_id = ?`, targetID, sourceID,
	).Scan(&rosterConflicts); err != nil {
		return fmt.Errorf("merge players: check season_rosters conflict: %w", err)
	}
	if rosterConflicts > 0 {
		return players.ErrMergeSeasonRosterConflict
	}

	// Blocker: source and target already played against each other (one home,
	// one away) in the same round_results row -- repointing would make a
	// player play themselves. Checked before the broader participation check
	// below so this more specific, more informative error takes priority.
	var selfOpponentConflicts int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM round_results
		WHERE (home_player_id = ? AND away_player_id = ?)
		   OR (home_player_id = ? AND away_player_id = ?)`,
		sourceID, targetID, targetID, sourceID,
	).Scan(&selfOpponentConflicts); err != nil {
		return fmt.Errorf("merge players: check self-opponent conflict: %w", err)
	}
	if selfOpponentConflicts > 0 {
		return players.ErrMergeSelfOpponentConflict
	}

	// Blocker: round_results participation -- source and target already both
	// participate (as home OR away, in any row, not necessarily the same row)
	// in the same (match_id, round_number). Repointing would make target
	// appear twice in one round -- the same condition Close Week rejects as
	// WEEK_PLAYER_DUPLICATE, and the same ambiguity
	// TestSaveRounds_AmbiguousSubstitutionReturns422 guards against for a
	// single save. This covers, beyond the self-opponent case above: both as
	// home, both as away, and home-in-one-row/away-in-another-row (either
	// direction) within the same match/round.
	var roundConflicts int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM round_results src
		WHERE (src.home_player_id = ? OR src.away_player_id = ?)
		  AND EXISTS (
		        SELECT 1 FROM round_results tgt
		        WHERE tgt.match_id = src.match_id
		          AND tgt.round_number = src.round_number
		          AND (tgt.home_player_id = ? OR tgt.away_player_id = ?)
		      )`,
		sourceID, sourceID, targetID, targetID,
	).Scan(&roundConflicts); err != nil {
		return fmt.Errorf("merge players: check round_results conflict: %w", err)
	}
	if roundConflicts > 0 {
		return players.ErrMergeRoundResultConflict
	}

	// Blocker: lineup_plans -- both players already have a row for the same
	// team/week/season.
	var lineupConflicts int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM lineup_plans src
		JOIN   lineup_plans tgt
		       ON tgt.team_id = src.team_id
		      AND tgt.week_number = src.week_number
		      AND tgt.season_id = src.season_id
		      AND tgt.player_id = ?
		WHERE  src.player_id = ?`, targetID, sourceID,
	).Scan(&lineupConflicts); err != nil {
		return fmt.Errorf("merge players: check lineup_plans conflict: %w", err)
	}
	if lineupConflicts > 0 {
		return players.ErrMergeLineupPlanConflict
	}

	// Repoint every supported reference. round_results' handicap snapshot
	// columns (home_handicap_used, away_handicap_used, handicap_pts_used,
	// handicap_to) and every other non-FK column are untouched by these
	// UPDATEs -- only the player-ID columns move.
	for _, stmt := range mergeRepointStmts {
		if _, err := tx.ExecContext(ctx, stmt, targetID, sourceID); err != nil {
			return fmt.Errorf("merge players: repoint %q: %w", stmt, err)
		}
	}

	// Source now has zero remaining references in any repointed table, so
	// this delete cannot hit a foreign-key restriction.
	if _, err := tx.ExecContext(ctx, `DELETE FROM players WHERE id = ?`, sourceID); err != nil {
		return fmt.Errorf("merge players: delete source %d: %w", sourceID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("merge players %d into %d: commit: %w", sourceID, targetID, err)
	}
	return nil
}
