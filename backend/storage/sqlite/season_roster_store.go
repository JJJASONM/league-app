package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domains/seasons"
	"league_app/models"
)

const rosterEntryCols = `
	SELECT sr.id, sr.season_id, sr.team_id, t.name,
	       sr.player_id, p.first_name||' '||p.last_name,
	       COALESCE(p.player_number,''), p.handicap
	FROM season_rosters sr
	JOIN teams t ON t.id = sr.team_id
	JOIN players p ON p.id = sr.player_id`

func scanRosterEntry(row interface{ Scan(...any) error }) (models.SeasonRosterEntry, error) {
	var e models.SeasonRosterEntry
	err := row.Scan(&e.ID, &e.SeasonID, &e.TeamID, &e.TeamName,
		&e.PlayerID, &e.PlayerName, &e.PlayerNumber, &e.Handicap)
	return e, err
}

// ListRoster returns all players on a team's season roster, ordered by player name.
func (s *SeasonStore) ListRoster(ctx context.Context, seasonID, teamID int64) ([]models.SeasonRosterEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		rosterEntryCols+` WHERE sr.season_id=? AND sr.team_id=? ORDER BY p.last_name, p.first_name`,
		seasonID, teamID)
	if err != nil {
		return nil, fmt.Errorf("list roster %d/%d: %w", seasonID, teamID, err)
	}
	defer rows.Close()
	var out []models.SeasonRosterEntry
	for rows.Next() {
		e, err := scanRosterEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan roster entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetPlayerRosterTeam returns the team the player is currently rostered on in this season.
// found=false when the player is not rostered anywhere in the season.
func (s *SeasonStore) GetPlayerRosterTeam(ctx context.Context, seasonID, playerID int64) (int64, bool, error) {
	var teamID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT team_id FROM season_rosters WHERE season_id=? AND player_id=?`,
		seasonID, playerID,
	).Scan(&teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get player roster team %d/%d: %w", seasonID, playerID, err)
	}
	return teamID, true, nil
}

// InsertOrGetRosterPlayer inserts the player into the team's season roster
// (INSERT OR IGNORE) and returns the full entry whether newly inserted or pre-existing.
func (s *SeasonStore) InsertOrGetRosterPlayer(ctx context.Context, seasonID, teamID, playerID int64) (models.SeasonRosterEntry, error) {
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamID, playerID,
	); err != nil {
		return models.SeasonRosterEntry{}, fmt.Errorf("insert roster %d/%d/%d: %w", seasonID, teamID, playerID, err)
	}
	row := s.db.QueryRowContext(ctx,
		rosterEntryCols+` WHERE sr.season_id=? AND sr.team_id=? AND sr.player_id=?`,
		seasonID, teamID, playerID)
	entry, err := scanRosterEntry(row)
	if err != nil {
		return models.SeasonRosterEntry{}, fmt.Errorf("fetch roster entry %d/%d/%d: %w", seasonID, teamID, playerID, err)
	}
	return entry, nil
}

// DeleteRosterPlayer removes a player from a team's season roster and clears
// captain_id in season_teams if that player was captain.
// Returns ErrRosterEntryNotFound (wrapped) when the entry does not exist.
func (s *SeasonStore) DeleteRosterPlayer(ctx context.Context, seasonID, teamID, playerID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete roster player: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`DELETE FROM season_rosters WHERE season_id=? AND team_id=? AND player_id=?`,
		seasonID, teamID, playerID)
	if err != nil {
		return fmt.Errorf("delete roster %d/%d/%d: %w", seasonID, teamID, playerID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("roster %d/%d/%d: %w", seasonID, teamID, playerID, seasons.ErrRosterEntryNotFound)
	}

	if _, err = tx.ExecContext(ctx,
		`UPDATE season_teams SET captain_id=NULL WHERE season_id=? AND team_id=? AND captain_id=?`,
		seasonID, teamID, playerID); err != nil {
		return fmt.Errorf("clear captain %d/%d/%d: %w", seasonID, teamID, playerID, err)
	}

	return tx.Commit()
}

// ListAvailablePlayers returns active players not already rostered in this season.
// Returns ErrNotFound (wrapped) when the season does not exist.
func (s *SeasonStore) ListAvailablePlayers(ctx context.Context, seasonID int64) ([]models.Player, error) {
	var leagueID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT league_id FROM seasons WHERE id=?`, seasonID,
	).Scan(&leagueID); errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("season %d: %w", seasonID, seasons.ErrNotFound)
	} else if err != nil {
		return nil, fmt.Errorf("list available players season %d: %w", seasonID, err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, COALESCE(p.player_number,''), p.first_name, p.last_name,
		       p.first_name||' '||p.last_name,
		       COALESCE(p.phone,''), COALESCE(p.email,''),
		       p.team_id, COALESCE(t.name,''), COALESCE(t.league_id,0),
		       p.handicap, COALESCE(p.admin_hold,0), COALESCE(p.active,1), COALESCE(p.note,''),
		       p.created_at
		FROM players p
		LEFT JOIN teams t ON t.id = p.team_id
		WHERE p.id NOT IN (
		        SELECT player_id FROM season_rosters WHERE season_id=?
		      )
		  AND COALESCE(p.active,1) = 1
		ORDER BY p.last_name, p.first_name`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list available players %d: %w", seasonID, err)
	}
	defer rows.Close()

	var out []models.Player
	for rows.Next() {
		var p models.Player
		var adminHold, activeInt int
		if err := rows.Scan(&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name,
			&p.Phone, &p.Email, &p.TeamID, &p.TeamName, &p.LeagueID,
			&p.Handicap, &adminHold, &activeInt, &p.Note, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan available player: %w", err)
		}
		p.AdminHold = adminHold == 1
		p.Active = activeInt == 1
		out = append(out, p)
	}
	return out, rows.Err()
}
