package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domains/matches"
	"league_app/models"
)

// RoundStore implements matches.RoundStore against a SQLite database.
// Use NewRoundStore; do not copy by value after first use.
type RoundStore struct {
	db   *sql.DB // used by RunTx to begin transactions
	q    querier // either db or the active *sql.Tx
	inTx bool    // true when this instance is scoped to an active transaction
}

// NewRoundStore returns a RoundStore backed by db.
func NewRoundStore(db *sql.DB) *RoundStore {
	return &RoundStore{db: db, q: db}
}

// RunTx executes fn inside a single read/write transaction (BEGIN DEFERRED).
// If the store is already inside a transaction, fn is called directly without nesting.
// A panic in fn rolls back the transaction before re-propagating.
// An error returned by fn rolls back; nil commits.
func (s *RoundStore) RunTx(ctx context.Context, fn func(matches.RoundStore) error) (retErr error) {
	if s.inTx {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("round store: begin tx: %w", err)
	}
	txStore := &RoundStore{db: s.db, q: tx, inTx: true}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if retErr != nil {
			_ = tx.Rollback()
		} else if commitErr := tx.Commit(); commitErr != nil {
			retErr = fmt.Errorf("round store: commit: %w", commitErr)
		}
	}()
	retErr = fn(txStore)
	return
}

// IsWeekClosed returns true when matches.week_closed=1 for the given matchID.
func (s *RoundStore) IsWeekClosed(ctx context.Context, matchID int64) (bool, error) {
	var wc int
	s.q.QueryRowContext(ctx, `SELECT week_closed FROM matches WHERE id=?`, matchID).Scan(&wc)
	return wc == 1, nil
}

// IsSeasonClosedForMatch returns true when the season containing matchID has closed_at set.
func (s *RoundStore) IsSeasonClosedForMatch(ctx context.Context, matchID int64) (bool, error) {
	var closed int
	s.q.QueryRowContext(ctx,
		`SELECT s.closed_at IS NOT NULL
		   FROM matches m
		   JOIN seasons s ON s.id = m.season_id
		  WHERE m.id=?`, matchID).Scan(&closed)
	return closed == 1, nil
}

// LoadMatchContext returns season_id, home_team_id, and away_team_id for a match.
func (s *RoundStore) LoadMatchContext(ctx context.Context, matchID int64) (matches.MatchContext, error) {
	var mc matches.MatchContext
	err := s.q.QueryRowContext(ctx,
		`SELECT season_id, COALESCE(home_team_id,0), COALESCE(away_team_id,0) FROM matches WHERE id=?`,
		matchID).Scan(&mc.SeasonID, &mc.HomeTeamID, &mc.AwayTeamID)
	if err != nil {
		return matches.MatchContext{}, fmt.Errorf("match %d: context: %w", matchID, err)
	}
	return mc, nil
}

// LoadPlayerHandicap returns the current handicap for the given player.
func (s *RoundStore) LoadPlayerHandicap(ctx context.Context, playerID int64) (float64, error) {
	var hc float64
	if err := s.q.QueryRowContext(ctx, `SELECT handicap FROM players WHERE id=?`, playerID).Scan(&hc); err != nil {
		return 0, fmt.Errorf("player %d: handicap: %w", playerID, err)
	}
	return hc, nil
}

// LoadPriorSnapshots returns the stored HC snapshots for existing round_results rows
// for the match. Used to preserve handicap history on re-save.
func (s *RoundStore) LoadPriorSnapshots(ctx context.Context, matchID int64) ([]matches.PriorSnapshotRow, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT round_number, home_player_id, away_player_id,
		       home_handicap_used, away_handicap_used
		FROM round_results WHERE match_id=?`, matchID)
	if err != nil {
		return nil, fmt.Errorf("load prior snapshots: %w", err)
	}
	defer rows.Close()
	var result []matches.PriorSnapshotRow
	for rows.Next() {
		var pr matches.PriorSnapshotRow
		if err := rows.Scan(&pr.RoundNumber, &pr.HomePlayerID, &pr.AwayPlayerID,
			&pr.HomeHandicapUsed, &pr.AwayHandicapUsed); err != nil {
			return nil, fmt.Errorf("load prior snapshots: scan: %w", err)
		}
		result = append(result, pr)
	}
	return result, rows.Err()
}

// DeleteRoundResults deletes all round_results rows for the match.
func (s *RoundStore) DeleteRoundResults(ctx context.Context, matchID int64) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM round_results WHERE match_id=?`, matchID); err != nil {
		return fmt.Errorf("delete round results: %w", err)
	}
	return nil
}

// InsertRoundResult inserts one round_results row with full HC snapshot columns.
func (s *RoundStore) InsertRoundResult(ctx context.Context, row matches.RoundResultRow) error {
	_, err := s.q.ExecContext(ctx, `
		INSERT INTO round_results
		  (match_id, round_number, home_player_id, away_player_id,
		   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		   home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.MatchID, row.RoundNumber, row.HomePlayerID, row.AwayPlayerID,
		row.Game1Home, row.Game1Away,
		row.Game2Home, row.Game2Away,
		row.Game3Home, row.Game3Away,
		row.HomeHCUsed, row.AwayHCUsed,
		row.HandicapPtsUsed, row.HandicapTo)
	if err != nil {
		return fmt.Errorf("insert round result: %w", err)
	}
	return nil
}

// DeleteMatchResults deletes all match_results rows for the match.
func (s *RoundStore) DeleteMatchResults(ctx context.Context, matchID int64) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM match_results WHERE match_id=?`, matchID); err != nil {
		return fmt.Errorf("delete match results: %w", err)
	}
	return nil
}

// InsertMatchResult inserts one match_results row.
func (s *RoundStore) InsertMatchResult(ctx context.Context, row matches.MatchResultRow) error {
	_, err := s.q.ExecContext(ctx, `
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff, sets_won, sets_lost)
		VALUES (?,?,?,?,?,?,?,?)`,
		row.MatchID, row.PlayerID, row.TeamID,
		row.GamesWon, row.GamesLost, row.Diff, row.SetsWon, row.SetsLost)
	if err != nil {
		return fmt.Errorf("insert match result: %w", err)
	}
	return nil
}

// MarkMatchCompleted sets matches.completed=1 for the match.
func (s *RoundStore) MarkMatchCompleted(ctx context.Context, matchID int64) error {
	if _, err := s.q.ExecContext(ctx, `UPDATE matches SET completed=1 WHERE id=?`, matchID); err != nil {
		return fmt.Errorf("mark match completed: %w", err)
	}
	return nil
}

// MarkMatchIncomplete sets matches.completed=0 for the match.
func (s *RoundStore) MarkMatchIncomplete(ctx context.Context, matchID int64) error {
	if _, err := s.q.ExecContext(ctx, `UPDATE matches SET completed=0 WHERE id=?`, matchID); err != nil {
		return fmt.Errorf("mark match incomplete: %w", err)
	}
	return nil
}

// GetRoundResults returns all round_results rows for the match joined to player names
// and current handicaps, ordered by round_number then id.
func (s *RoundStore) GetRoundResults(ctx context.Context, matchID int64) ([]models.RoundResult, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT rr.id, rr.match_id, rr.round_number,
		       rr.home_player_id, hp.first_name||' '||hp.last_name, hp.handicap,
		       rr.away_player_id, ap.first_name||' '||ap.last_name, ap.handicap,
		       rr.game1_home, rr.game1_away,
		       rr.game2_home, rr.game2_away,
		       rr.game3_home, rr.game3_away,
		       rr.home_handicap_used, rr.away_handicap_used,
		       rr.handicap_pts_used,  rr.handicap_to
		FROM round_results rr
		JOIN players hp ON hp.id = rr.home_player_id
		JOIN players ap ON ap.id = rr.away_player_id
		WHERE rr.match_id = ?
		ORDER BY rr.round_number, rr.id`, matchID)
	if err != nil {
		return nil, fmt.Errorf("get round results: %w", err)
	}
	defer rows.Close()
	var result []models.RoundResult
	for rows.Next() {
		var rr models.RoundResult
		if err := rows.Scan(
			&rr.ID, &rr.MatchID, &rr.RoundNumber,
			&rr.HomePlayerID, &rr.HomePlayerName, &rr.HomeHandicap,
			&rr.AwayPlayerID, &rr.AwayPlayerName, &rr.AwayHandicap,
			&rr.Game1Home, &rr.Game1Away,
			&rr.Game2Home, &rr.Game2Away,
			&rr.Game3Home, &rr.Game3Away,
			&rr.HomeHandicapUsed, &rr.AwayHandicapUsed,
			&rr.HandicapPtsUsed, &rr.HandicapToUsed); err != nil {
			return nil, fmt.Errorf("get round results: scan: %w", err)
		}
		result = append(result, rr)
	}
	return result, rows.Err()
}

// GetStandingsData returns teams, completed+week-closed matches, and per-match results
// for the given season, ready to pass to logic.ComputeStandings.
func (s *RoundStore) GetStandingsData(ctx context.Context, seasonID int64) (matches.StandingsData, error) {
	teamRows, err := s.q.QueryContext(ctx, `
		SELECT t.id, t.name FROM teams t
		JOIN seasons se ON se.league_id = t.league_id
		WHERE se.id=? ORDER BY t.name`, seasonID)
	if err != nil {
		return matches.StandingsData{}, fmt.Errorf("standings: teams: %w", err)
	}
	var teams []models.Team
	for teamRows.Next() {
		var t models.Team
		teamRows.Scan(&t.ID, &t.Name)
		teams = append(teams, t)
	}
	teamRows.Close()

	matchRows, err := s.q.QueryContext(ctx, `
		SELECT id, season_id, home_team_id, away_team_id, match_date, week_number, completed, created_at
		FROM matches WHERE season_id=? AND completed=1 AND week_closed=1`, seasonID)
	if err != nil {
		return matches.StandingsData{}, fmt.Errorf("standings: matches: %w", err)
	}
	var ms []models.Match
	for matchRows.Next() {
		var m models.Match
		var completed int
		matchRows.Scan(&m.ID, &m.SeasonID, &m.HomeTeamID, &m.AwayTeamID,
			&m.MatchDate, &m.WeekNumber, &completed, &m.CreatedAt)
		m.Completed = completed == 1
		ms = append(ms, m)
	}
	matchRows.Close()

	resultMap := make(map[int64][]models.MatchResult)
	for _, m := range ms {
		resRows, err := s.q.QueryContext(ctx, `
			SELECT id, match_id, player_id, team_id, sets_won, sets_lost,
			       games_won, games_lost, diff, created_at
			FROM match_results WHERE match_id=?`, m.ID)
		if err != nil {
			continue
		}
		for resRows.Next() {
			var res models.MatchResult
			resRows.Scan(&res.ID, &res.MatchID, &res.PlayerID, &res.TeamID,
				&res.SetsWon, &res.SetsLost, &res.GamesWon, &res.GamesLost, &res.Diff, &res.CreatedAt)
			resultMap[m.ID] = append(resultMap[m.ID], res)
		}
		resRows.Close()
	}

	return matches.StandingsData{Teams: teams, Matches: ms, ResultMap: resultMap}, nil
}

// GetPlayerStats returns aggregated match_results for the given season or league scope.
// Returns nil when neither SeasonID nor LeagueID is set (caller normalises to empty slice).
// SeasonID scope includes players present in that season's season_rosters even when
// players.team_id is NULL. LeagueID scope includes players assigned via season_rosters
// or lineup_plans for any season in that league, even when players.team_id is NULL or
// belongs to a different league -- see the league-scoped case below for detail; team
// display name resolution intentionally mirrors the season-scoped case's precedence
// (direct team_id first, then roster/lineup team) but cannot share its exact SQL since
// league scope has no single season_id to resolve against.
func (s *RoundStore) GetPlayerStats(ctx context.Context, req matches.PlayerStatsRequest) ([]models.PlayerStat, error) {
	var query string
	var args []any
	switch {
	case req.SeasonID != 0:
		// Team resolution prefers this season's season_rosters entry (the
		// target model, where team assignment can differ per season and
		// players.team_id may be NULL) and falls back to the player's
		// direct players.team_id when they have no season_rosters row for
		// this specific season. This keeps existing team_id-only players
		// working unchanged while including roster-only players that the
		// old players.team_id-only JOIN silently dropped.
		query = `
			WITH season_team AS (
			    SELECT player_id, team_id FROM season_rosters WHERE season_id = ?
			)
			SELECT p.id, COALESCE(p.player_number,''), p.first_name || ' ' || p.last_name,
			       COALESCE(t.name,''), p.handicap,
			       COALESCE(SUM(mr.sets_won),0), COALESCE(SUM(mr.sets_lost),0),
			       COALESCE(SUM(mr.games_won),0), COALESCE(SUM(mr.games_lost),0)
			FROM players p
			LEFT JOIN season_team st ON st.player_id = p.id
			JOIN teams t ON t.id = COALESCE(st.team_id, p.team_id)
			JOIN seasons se ON se.league_id = t.league_id AND se.id = ?
			LEFT JOIN match_results mr ON mr.player_id = p.id
			    AND mr.match_id IN (
			        SELECT id FROM matches
			        WHERE season_id=? AND completed=1 AND week_closed=1
			    )
			GROUP BY p.id ORDER BY SUM(mr.sets_won) DESC, SUM(mr.games_won) DESC`
		args = []any{req.SeasonID, req.SeasonID, req.SeasonID}
	case req.LeagueID != 0:
		// league_players is deduplicated (plain UNION) to exactly one row per
		// eligible player_id before any join touches match_results -- joining
		// season_rosters/lineup_plans directly (both can have many rows per
		// player across a league's seasons) would otherwise multiply
		// match_results rows and inflate the SUM(...) aggregates. A player is
		// eligible via any of three sources: a direct players.team_id in this
		// league (existing behavior, preserved), a season_rosters row for any
		// season in this league, or a lineup_plans row for any season in this
		// league (covers a substitute who was never added to season_rosters).
		// Team display name uses the same precedence, each resolved as an
		// independent scalar subquery (not a join) for the same
		// no-row-multiplication reason: direct team_id first, else the most
		// recent (highest season_id, then id) season_rosters team, else the
		// most recent lineup_plans team.
		query = `
			WITH league_players AS (
				SELECT p.id AS player_id FROM players p
				JOIN teams t ON t.id = p.team_id AND t.league_id = ?
				UNION
				SELECT sr.player_id FROM season_rosters sr
				JOIN seasons se ON se.id = sr.season_id WHERE se.league_id = ?
				UNION
				SELECT lp.player_id FROM lineup_plans lp
				JOIN seasons se ON se.id = lp.season_id WHERE se.league_id = ?
			)
			SELECT p.id, COALESCE(p.player_number,''), p.first_name || ' ' || p.last_name,
			       COALESCE(
			           (SELECT t.name FROM teams t WHERE t.id = p.team_id AND t.league_id = ?),
			           (SELECT t.name FROM teams t
			            JOIN season_rosters sr ON sr.team_id = t.id
			            JOIN seasons se ON se.id = sr.season_id
			            WHERE se.league_id = ? AND sr.player_id = p.id
			            ORDER BY sr.season_id DESC, sr.id DESC LIMIT 1),
			           (SELECT t.name FROM teams t
			            JOIN lineup_plans lp ON lp.team_id = t.id
			            JOIN seasons se ON se.id = lp.season_id
			            WHERE se.league_id = ? AND lp.player_id = p.id
			            ORDER BY lp.season_id DESC, lp.id DESC LIMIT 1),
			           ''
			       ),
			       p.handicap,
			       COALESCE(SUM(mr.sets_won),0), COALESCE(SUM(mr.sets_lost),0),
			       COALESCE(SUM(mr.games_won),0), COALESCE(SUM(mr.games_lost),0)
			FROM players p
			JOIN league_players lps ON lps.player_id = p.id
			LEFT JOIN match_results mr ON mr.player_id = p.id
			GROUP BY p.id ORDER BY SUM(mr.sets_won) DESC, SUM(mr.games_won) DESC`
		args = []any{req.LeagueID, req.LeagueID, req.LeagueID, req.LeagueID, req.LeagueID, req.LeagueID}
	default:
		return nil, nil
	}

	rows, err := s.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("player stats: %w", err)
	}
	defer rows.Close()
	var stats []models.PlayerStat
	for rows.Next() {
		var st models.PlayerStat
		rows.Scan(&st.PlayerID, &st.PlayerNumber, &st.PlayerName, &st.TeamName, &st.Handicap,
			&st.SetsWon, &st.SetsLost, &st.GamesWon, &st.GamesLost)
		stats = append(stats, st)
	}
	return stats, rows.Err()
}

// SubmitMatchResults replaces match_results for a match and marks it
// completed, wrapped in a transaction. Player Score Approval Phase 1A: the
// caller (RoundService.submitResults) is responsible for calling
// AdvanceScoreRevisionForEdit as the FIRST statement inside the SAME
// transaction, before this method runs -- RunTx nests transparently when
// this method's own RunTx call runs against an already-tx-scoped store, so
// both writes share one commit/rollback unit. This method itself no longer
// touches score_revision or team-approval state.
func (s *RoundStore) SubmitMatchResults(ctx context.Context, matchID int64, results []models.MatchResult) error {
	return s.RunTx(ctx, func(tx matches.RoundStore) error {
		if err := tx.DeleteMatchResults(ctx, matchID); err != nil {
			return err
		}
		for _, res := range results {
			row := matches.MatchResultRow{
				MatchID:   matchID,
				PlayerID:  res.PlayerID,
				TeamID:    res.TeamID,
				SetsWon:   res.SetsWon,
				SetsLost:  res.SetsLost,
				GamesWon:  res.GamesWon,
				GamesLost: res.GamesLost,
				Diff:      res.Diff,
			}
			if err := tx.InsertMatchResult(ctx, row); err != nil {
				return err
			}
		}
		return tx.MarkMatchCompleted(ctx, matchID)
	})
}

// ClearMatchResults deletes match_results for a match and marks it
// incomplete, wrapped in a transaction (Player Score Approval Phase 1A --
// previously two separate statements outside any transaction). The caller
// (RoundService.clearResults) calls AdvanceScoreRevisionForEdit as the
// FIRST statement inside the same transaction, before this method runs --
// see SubmitMatchResults's doc comment for how RunTx nesting makes that one
// commit/rollback unit.
func (s *RoundStore) ClearMatchResults(ctx context.Context, matchID int64) error {
	return s.RunTx(ctx, func(tx matches.RoundStore) error {
		if err := tx.DeleteMatchResults(ctx, matchID); err != nil {
			return fmt.Errorf("clear match results: %w", err)
		}
		return tx.MarkMatchIncomplete(ctx, matchID)
	})
}

// GetMatchApprovalState returns the match's completed/approved/processed
// state plus Player Score Approval Phase 1A's score revision and per-side
// team-approval state. Exists is false when no match row matches matchID.
func (s *RoundStore) GetMatchApprovalState(ctx context.Context, matchID int64) (matches.MatchApprovalState, error) {
	var completed int
	var approvedAt, processedAt sql.NullString
	var scoreRevision int
	var homeState, awayState string
	var homeStateAt, awayStateAt sql.NullString
	var homeActorUserID, awayActorUserID sql.NullInt64
	var homeActorPlayerID, awayActorPlayerID sql.NullInt64
	var homeActorName, awayActorName string
	var homeApprovedRev, awayApprovedRev sql.NullInt64
	var homeNote, awayNote string
	err := s.q.QueryRowContext(ctx, `
		SELECT completed, approved_at, processed_at, score_revision,
		       home_approval_state, home_approval_state_at,
		       home_approval_actor_user_id, home_approval_actor_player_id,
		       home_approval_actor_name_snapshot, home_approval_state_score_revision,
		       home_correction_note,
		       away_approval_state, away_approval_state_at,
		       away_approval_actor_user_id, away_approval_actor_player_id,
		       away_approval_actor_name_snapshot, away_approval_state_score_revision,
		       away_correction_note
		FROM matches WHERE id=?`, matchID).
		Scan(&completed, &approvedAt, &processedAt, &scoreRevision,
			&homeState, &homeStateAt, &homeActorUserID, &homeActorPlayerID, &homeActorName, &homeApprovedRev, &homeNote,
			&awayState, &awayStateAt, &awayActorUserID, &awayActorPlayerID, &awayActorName, &awayApprovedRev, &awayNote)
	if err == sql.ErrNoRows {
		return matches.MatchApprovalState{}, nil
	}
	if err != nil {
		return matches.MatchApprovalState{}, fmt.Errorf("match %d: approval state: %w", matchID, err)
	}
	state := matches.MatchApprovalState{
		Exists:        true,
		Completed:     completed == 1,
		ScoreRevision: scoreRevision,
		HomeSide: matches.TeamSideApprovalState{
			State:             homeState,
			ActorNameSnapshot: homeActorName,
			CorrectionNote:    homeNote,
		},
		AwaySide: matches.TeamSideApprovalState{
			State:             awayState,
			ActorNameSnapshot: awayActorName,
			CorrectionNote:    awayNote,
		},
	}
	if approvedAt.Valid && approvedAt.String != "" {
		state.ApprovedAt = &approvedAt.String
	}
	if processedAt.Valid && processedAt.String != "" {
		state.ProcessedAt = &processedAt.String
	}
	if homeStateAt.Valid {
		state.HomeSide.StateAt = &homeStateAt.String
	}
	if homeActorUserID.Valid {
		state.HomeSide.ActorUserID = &homeActorUserID.Int64
	}
	if homeActorPlayerID.Valid {
		state.HomeSide.ActorPlayerID = &homeActorPlayerID.Int64
	}
	if homeApprovedRev.Valid {
		v := int(homeApprovedRev.Int64)
		state.HomeSide.StateScoreRevision = &v
	}
	if awayStateAt.Valid {
		state.AwaySide.StateAt = &awayStateAt.String
	}
	if awayActorUserID.Valid {
		state.AwaySide.ActorUserID = &awayActorUserID.Int64
	}
	if awayActorPlayerID.Valid {
		state.AwaySide.ActorPlayerID = &awayActorPlayerID.Int64
	}
	if awayApprovedRev.Valid {
		v := int(awayApprovedRev.Int64)
		state.AwaySide.StateScoreRevision = &v
	}
	return state, nil
}

// matchGuardSnapshot holds a read-only view of every condition the guarded
// writes below (AdvanceScoreRevisionForEdit, ApproveTeamSide,
// WithdrawTeamApproval, RequestCorrection, GuardedAdminAccept) depend on.
// It exists only to classify WHY a conditional UPDATE affected zero rows --
// never to decide whether a write is allowed (that decision belongs solely
// to the conditional UPDATE's own WHERE clause, evaluated atomically at
// write time).
type matchGuardSnapshot struct {
	ScoreRevision     int
	Completed         bool
	SeasonClosed      bool
	WeekClosed        bool
	Processed         bool
	AdminAccepted     bool
	HomeState         string
	AwayState         string
	HomeStateRevision *int
	AwayStateRevision *int
	HomeNote          string
	AwayNote          string
}

// loadMatchGuardSnapshot performs a read-only query (no writes) to build a
// matchGuardSnapshot for matchID. Returns matches.ErrGuardMatchNotFound
// when matchID does not exist. HomeStateRevision/AwayStateRevision and
// HomeNote/AwayNote are read alongside the state labels specifically for
// GuardedAdminAccept's classifyAdminAcceptBlock, which must detect a
// correction note (or revision) rewritten at the same state label.
func loadMatchGuardSnapshot(ctx context.Context, q querier, matchID int64) (matchGuardSnapshot, error) {
	var snap matchGuardSnapshot
	var completed, weekClosed int
	var approvedAt, processedAt, seasonClosedAt sql.NullString
	var homeStateRev, awayStateRev sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT m.score_revision, m.completed, m.approved_at, m.processed_at, m.week_closed,
		       se.closed_at, m.home_approval_state, m.away_approval_state,
		       m.home_approval_state_score_revision, m.home_correction_note,
		       m.away_approval_state_score_revision, m.away_correction_note
		FROM matches m JOIN seasons se ON se.id = m.season_id
		WHERE m.id = ?`, matchID).
		Scan(&snap.ScoreRevision, &completed, &approvedAt, &processedAt, &weekClosed,
			&seasonClosedAt, &snap.HomeState, &snap.AwayState,
			&homeStateRev, &snap.HomeNote, &awayStateRev, &snap.AwayNote)
	if err == sql.ErrNoRows {
		return matchGuardSnapshot{}, matches.ErrGuardMatchNotFound
	}
	if err != nil {
		return matchGuardSnapshot{}, fmt.Errorf("load match guard snapshot: %w", err)
	}
	snap.Completed = completed == 1
	snap.WeekClosed = weekClosed == 1
	snap.Processed = processedAt.Valid && processedAt.String != ""
	snap.AdminAccepted = approvedAt.Valid && approvedAt.String != ""
	snap.SeasonClosed = seasonClosedAt.Valid && seasonClosedAt.String != ""
	if homeStateRev.Valid {
		v := int(homeStateRev.Int64)
		snap.HomeStateRevision = &v
	}
	if awayStateRev.Valid {
		v := int(awayStateRev.Int64)
		snap.AwayStateRevision = &v
	}
	return snap, nil
}

// intPtrEqual is a NULL-safe equality check for two optional revisions:
// true when both are nil, true when both are non-nil and equal, false
// otherwise. Mirrors SQL's `IS` operator semantics for the Go-side
// classification path.
func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// classifyScoreEditBlock determines why a score-edit guard
// (AdvanceScoreRevisionForEdit) found zero matching rows, given a read-only
// snapshot taken immediately after. The fallback at the end covers the
// theoretical case where the snapshot no longer shows any blocking
// condition -- a benign race where the state changed again between the
// failed write and this read -- treated conservatively as stale so the
// caller retries with fresh data rather than getting a confusing "unknown"
// error.
func classifyScoreEditBlock(snap matchGuardSnapshot, expectedRevision *int) error {
	if expectedRevision != nil && snap.ScoreRevision != *expectedRevision {
		return matches.ErrApprovalRevisionStale
	}
	if snap.SeasonClosed {
		return matches.ErrGuardSeasonClosed
	}
	if snap.WeekClosed {
		return matches.ErrGuardWeekClosed
	}
	if snap.Processed {
		return matches.ErrGuardProcessed
	}
	if snap.AdminAccepted {
		return matches.ErrGuardAdminAccepted
	}
	return matches.ErrApprovalRevisionStale
}

// classifyTeamActionBlock determines why a team-action guard (ApproveTeamSide,
// WithdrawTeamApproval, RequestCorrection) found zero matching rows.
// requireCompleted applies to Approve/RequestCorrection (not Withdraw, which
// can only ever apply to an already-approved, and therefore already-scored,
// side). requireFromApproved applies only to Withdraw.
func classifyTeamActionBlock(snap matchGuardSnapshot, expectedRevision int, side string, requireCompleted, requireFromApproved bool) error {
	if snap.ScoreRevision != expectedRevision {
		return matches.ErrApprovalRevisionStale
	}
	if snap.SeasonClosed {
		return matches.ErrGuardSeasonClosed
	}
	if snap.WeekClosed {
		return matches.ErrGuardWeekClosed
	}
	if snap.Processed {
		return matches.ErrGuardProcessed
	}
	if snap.AdminAccepted {
		return matches.ErrGuardAdminAccepted
	}
	if requireCompleted && !snap.Completed {
		return matches.ErrGuardNotScored
	}
	if requireFromApproved {
		sideState := snap.HomeState
		if side == matches.ApprovalSideAway {
			sideState = snap.AwayState
		}
		if sideState != matches.ApprovalStateApproved {
			return matches.ErrGuardInvalidTransition
		}
	}
	return matches.ErrApprovalRevisionStale
}

// classifyAdminAcceptBlock determines why GuardedAdminAccept found zero
// matching rows. pinnedHome/pinnedAway are the COMPLETE side snapshots
// (state, state-score-revision including nil, and correction note)
// RoundService.AdminAcceptMatch's accept-vs-override decision was based
// on; a mismatch in any one of the three means that decision is no longer
// valid -- e.g. a correction note rewritten at the same revision with the
// same "correction_requested" label.
func classifyAdminAcceptBlock(snap matchGuardSnapshot, expectedRevision int, pinnedHome, pinnedAway matches.TeamSideApprovalState) error {
	if snap.ScoreRevision != expectedRevision {
		return matches.ErrApprovalRevisionStale
	}
	if snap.SeasonClosed {
		return matches.ErrGuardSeasonClosed
	}
	if snap.WeekClosed {
		return matches.ErrGuardWeekClosed
	}
	if !snap.Completed {
		return matches.ErrGuardNotScored
	}
	if snap.Processed {
		return matches.ErrGuardProcessed
	}
	if snap.AdminAccepted {
		return matches.ErrGuardAdminAccepted
	}
	if snap.HomeState != pinnedHome.State ||
		!intPtrEqual(snap.HomeStateRevision, pinnedHome.StateScoreRevision) ||
		snap.HomeNote != pinnedHome.CorrectionNote {
		return matches.ErrGuardTeamStateChanged
	}
	if snap.AwayState != pinnedAway.State ||
		!intPtrEqual(snap.AwayStateRevision, pinnedAway.StateScoreRevision) ||
		snap.AwayNote != pinnedAway.CorrectionNote {
		return matches.ErrGuardTeamStateChanged
	}
	return matches.ErrApprovalRevisionStale
}

// matchStillEditableSQL is the shared WHERE-clause fragment every guarded
// write below re-verifies atomically: the match is not admin-accepted, not
// processed, its week is open, and its season is open. Embedded via string
// concatenation (never with user input) into each conditional UPDATE.
const matchStillEditableSQL = `
	  AND approved_at IS NULL
	  AND processed_at IS NULL
	  AND week_closed = 0
	  AND EXISTS (SELECT 1 FROM seasons se WHERE se.id = matches.season_id AND se.closed_at IS NULL)`

// advanceScoreRevisionForEdit is the shared implementation behind
// RoundStore.AdvanceScoreRevisionForEdit and the SQLite LineupStore's
// substitute-swap methods (SetSubstitute/ClearSubstitute), which are a
// different concrete store type but must apply the exact same guard inside
// their own transaction. q is whichever connection is active -- *sql.DB or
// an in-flight *sql.Tx -- so this must always be called from inside the
// same transaction as the mutation that triggered it, BEFORE that
// mutation's own destructive writes.
func advanceScoreRevisionForEdit(ctx context.Context, q querier, matchID int64, expectedRevision *int) (int, error) {
	res, err := q.ExecContext(ctx,
		`UPDATE matches SET score_revision = score_revision + 1
		 WHERE id = ? AND (? IS NULL OR score_revision = ?)`+matchStillEditableSQL,
		matchID, expectedRevision, expectedRevision)
	if err != nil {
		return 0, fmt.Errorf("advance score revision for edit: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("advance score revision for edit: rows affected: %w", err)
	}
	if n == 0 {
		snap, err := loadMatchGuardSnapshot(ctx, q, matchID)
		if err != nil {
			return 0, err
		}
		return 0, classifyScoreEditBlock(snap, expectedRevision)
	}

	var newRevision int
	var homeState, awayState string
	if err := q.QueryRowContext(ctx,
		`SELECT score_revision, home_approval_state, away_approval_state FROM matches WHERE id = ?`,
		matchID).Scan(&newRevision, &homeState, &awayState); err != nil {
		return 0, fmt.Errorf("advance score revision for edit: read new state: %w", err)
	}
	for _, side := range [...]struct{ name, state string }{
		{"home", homeState},
		{"away", awayState},
	} {
		if side.state == matches.ApprovalStatePending {
			continue
		}
		clearSQL := fmt.Sprintf(`UPDATE matches SET
			%[1]s_approval_state = 'pending',
			%[1]s_approval_state_at = CURRENT_TIMESTAMP,
			%[1]s_approval_actor_user_id = NULL,
			%[1]s_approval_actor_player_id = NULL,
			%[1]s_approval_actor_name_snapshot = '',
			%[1]s_approval_state_score_revision = NULL,
			%[1]s_correction_note = ''
			WHERE id = ?`, side.name)
		if _, err := q.ExecContext(ctx, clearSQL, matchID); err != nil {
			return 0, fmt.Errorf("advance score revision for edit: clear %s approval: %w", side.name, err)
		}
		if _, err := q.ExecContext(ctx, `
			INSERT INTO match_approval_events (match_id, event_scope, event_type, score_revision, note)
			VALUES (?, ?, 'cleared_by_edit', ?, '')`,
			matchID, side.name, newRevision); err != nil {
			return 0, fmt.Errorf("advance score revision for edit: log cleared event for %s: %w", side.name, err)
		}
	}
	return newRevision, nil
}

// AdvanceScoreRevisionForEdit implements matches.RoundStore's method of the
// same name by delegating to the shared implementation above using this
// store's active connection (s.q is either the pooled *sql.DB or, when this
// instance is tx-scoped via RunTx, the in-flight *sql.Tx).
func (s *RoundStore) AdvanceScoreRevisionForEdit(ctx context.Context, matchID int64, expectedRevision *int) (int, error) {
	return advanceScoreRevisionForEdit(ctx, s.q, matchID, expectedRevision)
}

// ResolveApprovalEligibility reports whether playerID is eligible to act
// for the given side of matchID: either currently rostered to that side's
// team for the match's season (season_rosters), or an actual round_results
// participant on that side for this specific match.
func (s *RoundStore) ResolveApprovalEligibility(ctx context.Context, matchID int64, side string, playerID int64) (bool, error) {
	var exists int
	err := s.q.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM matches m
			JOIN season_rosters sr ON sr.season_id = m.season_id AND sr.player_id = ?
			WHERE m.id = ?
			  AND sr.team_id = (CASE WHEN ? = 'home' THEN m.home_team_id ELSE m.away_team_id END)
		) OR EXISTS(
			SELECT 1 FROM round_results rr
			WHERE rr.match_id = ?
			  AND (
			        (? = 'home' AND rr.home_player_id = ?)
			     OR (? = 'away' AND rr.away_player_id = ?)
			      )
		)`,
		playerID, matchID, side,
		matchID,
		side, playerID,
		side, playerID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("resolve approval eligibility: %w", err)
	}
	return exists == 1, nil
}

// ApproveTeamSide sets the given side's approval_state to 'approved' for
// expectedRevision via one conditional UPDATE that atomically re-verifies,
// at write time: match exists, score_revision matches, completed=1, and
// matchStillEditableSQL (season open, week open, not admin-accepted, not
// processed). On zero rows matched, classifyTeamActionBlock determines
// exactly which condition failed via a read-only follow-up query.
func (s *RoundStore) ApproveTeamSide(ctx context.Context, matchID int64, side string, expectedRevision int, actorUserID, actorPlayerID *int64, actorNameSnapshot string) error {
	prefix := side
	return s.RunTx(ctx, func(txIface matches.RoundStore) error {
		tx := txIface.(*RoundStore)
		res, err := tx.q.ExecContext(ctx, fmt.Sprintf(`
			UPDATE matches SET
				%[1]s_approval_state = 'approved',
				%[1]s_approval_state_at = CURRENT_TIMESTAMP,
				%[1]s_approval_actor_user_id = ?,
				%[1]s_approval_actor_player_id = ?,
				%[1]s_approval_actor_name_snapshot = ?,
				%[1]s_approval_state_score_revision = ?,
				%[1]s_correction_note = ''
			WHERE id = ? AND score_revision = ? AND completed = 1`+matchStillEditableSQL, prefix),
			actorUserID, actorPlayerID, actorNameSnapshot, expectedRevision, matchID, expectedRevision)
		if err != nil {
			return fmt.Errorf("approve team side: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("approve team side: rows affected: %w", err)
		}
		if n == 0 {
			snap, err := loadMatchGuardSnapshot(ctx, tx.q, matchID)
			if err != nil {
				return err
			}
			return classifyTeamActionBlock(snap, expectedRevision, side, true, false)
		}
		if _, err := tx.q.ExecContext(ctx, `
			INSERT INTO match_approval_events
				(match_id, event_scope, event_type, score_revision, actor_user_id, actor_player_id, actor_name_snapshot)
			VALUES (?, ?, 'approved', ?, ?, ?, ?)`,
			matchID, side, expectedRevision, actorUserID, actorPlayerID, actorNameSnapshot); err != nil {
			return fmt.Errorf("approve team side: log event: %w", err)
		}
		return nil
	})
}

// WithdrawTeamApproval resets the given side's approval_state from
// 'approved' back to 'pending' for expectedRevision via one conditional
// UPDATE that additionally requires the side's current state to already be
// 'approved', atomically, alongside the same match-editable conditions
// ApproveTeamSide re-verifies. On zero rows matched, classifyTeamActionBlock
// determines exactly which condition failed.
func (s *RoundStore) WithdrawTeamApproval(ctx context.Context, matchID int64, side string, expectedRevision int, actorUserID, actorPlayerID *int64, actorNameSnapshot string) error {
	prefix := side
	return s.RunTx(ctx, func(txIface matches.RoundStore) error {
		tx := txIface.(*RoundStore)
		res, err := tx.q.ExecContext(ctx, fmt.Sprintf(`
			UPDATE matches SET
				%[1]s_approval_state = 'pending',
				%[1]s_approval_state_at = CURRENT_TIMESTAMP,
				%[1]s_approval_actor_user_id = NULL,
				%[1]s_approval_actor_player_id = NULL,
				%[1]s_approval_actor_name_snapshot = '',
				%[1]s_approval_state_score_revision = NULL,
				%[1]s_correction_note = ''
			WHERE id = ? AND score_revision = ? AND %[1]s_approval_state = 'approved'`+matchStillEditableSQL, prefix),
			matchID, expectedRevision)
		if err != nil {
			return fmt.Errorf("withdraw team approval: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("withdraw team approval: rows affected: %w", err)
		}
		if n == 0 {
			snap, err := loadMatchGuardSnapshot(ctx, tx.q, matchID)
			if err != nil {
				return err
			}
			return classifyTeamActionBlock(snap, expectedRevision, side, false, true)
		}
		if _, err := tx.q.ExecContext(ctx, `
			INSERT INTO match_approval_events
				(match_id, event_scope, event_type, score_revision, actor_user_id, actor_player_id, actor_name_snapshot)
			VALUES (?, ?, 'withdrawn', ?, ?, ?, ?)`,
			matchID, side, expectedRevision, actorUserID, actorPlayerID, actorNameSnapshot); err != nil {
			return fmt.Errorf("withdraw team approval: log event: %w", err)
		}
		return nil
	})
}

// RequestCorrection sets the given side's approval_state to
// 'correction_requested' for expectedRevision with the given note via one
// conditional UPDATE requiring the same match-editable conditions as
// ApproveTeamSide (no FROM-state restriction -- a correction may be
// requested from 'pending' or 'approved' alike, and only ever affects this
// one side). On zero rows matched, classifyTeamActionBlock determines
// exactly which condition failed.
func (s *RoundStore) RequestCorrection(ctx context.Context, matchID int64, side string, expectedRevision int, note string, actorUserID, actorPlayerID *int64, actorNameSnapshot string) error {
	prefix := side
	return s.RunTx(ctx, func(txIface matches.RoundStore) error {
		tx := txIface.(*RoundStore)
		res, err := tx.q.ExecContext(ctx, fmt.Sprintf(`
			UPDATE matches SET
				%[1]s_approval_state = 'correction_requested',
				%[1]s_approval_state_at = CURRENT_TIMESTAMP,
				%[1]s_approval_actor_user_id = ?,
				%[1]s_approval_actor_player_id = ?,
				%[1]s_approval_actor_name_snapshot = ?,
				%[1]s_approval_state_score_revision = ?,
				%[1]s_correction_note = ?
			WHERE id = ? AND score_revision = ? AND completed = 1`+matchStillEditableSQL, prefix),
			actorUserID, actorPlayerID, actorNameSnapshot, expectedRevision, note, matchID, expectedRevision)
		if err != nil {
			return fmt.Errorf("request correction: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("request correction: rows affected: %w", err)
		}
		if n == 0 {
			snap, err := loadMatchGuardSnapshot(ctx, tx.q, matchID)
			if err != nil {
				return err
			}
			return classifyTeamActionBlock(snap, expectedRevision, side, true, false)
		}
		if _, err := tx.q.ExecContext(ctx, `
			INSERT INTO match_approval_events
				(match_id, event_scope, event_type, score_revision, actor_user_id, actor_player_id, actor_name_snapshot, note)
			VALUES (?, ?, 'correction_requested', ?, ?, ?, ?, ?)`,
			matchID, side, expectedRevision, actorUserID, actorPlayerID, actorNameSnapshot, note); err != nil {
			return fmt.Errorf("request correction: log event: %w", err)
		}
		return nil
	})
}

// GuardedAdminAccept performs the admin-acceptance write for
// RoundService.AdminAcceptMatch via one conditional UPDATE that atomically
// re-verifies, at write time: match exists, score_revision matches,
// completed=1, matchStillEditableSQL, AND that both team sides' current
// approval_state still equal pinnedHomeState/pinnedAwayState -- the states
// AdminAcceptMatch's accept-vs-override decision was based on. On zero rows
// matched, classifyAdminAcceptBlock determines exactly which condition
// failed.
func (s *RoundStore) GuardedAdminAccept(ctx context.Context, matchID int64, expectedRevision int, pinnedHome, pinnedAway matches.TeamSideApprovalState, approvedByUserID *int64, note string) error {
	res, err := s.q.ExecContext(ctx, `
		UPDATE matches SET
			approved_at = CURRENT_TIMESTAMP,
			approved_by_user_id = ?,
			approval_note = ?
		WHERE id = ? AND score_revision = ? AND completed = 1
		  AND home_approval_state = ? AND home_approval_state_score_revision IS ? AND home_correction_note = ?
		  AND away_approval_state = ? AND away_approval_state_score_revision IS ? AND away_correction_note = ?`+matchStillEditableSQL,
		approvedByUserID, note, matchID, expectedRevision,
		pinnedHome.State, pinnedHome.StateScoreRevision, pinnedHome.CorrectionNote,
		pinnedAway.State, pinnedAway.StateScoreRevision, pinnedAway.CorrectionNote)
	if err != nil {
		return fmt.Errorf("guarded admin accept: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("guarded admin accept: rows affected: %w", err)
	}
	if n == 0 {
		snap, err := loadMatchGuardSnapshot(ctx, s.q, matchID)
		if err != nil {
			return err
		}
		return classifyAdminAcceptBlock(snap, expectedRevision, pinnedHome, pinnedAway)
	}
	return nil
}

// ListApprovalEvents returns the full match_approval_events history for
// matchID, ordered oldest first.
func (s *RoundStore) ListApprovalEvents(ctx context.Context, matchID int64) ([]models.MatchApprovalEvent, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT id, match_id, event_scope, event_type, score_revision,
		       actor_user_id, actor_player_id, actor_name_snapshot, note,
		       home_state_snapshot, home_note_snapshot,
		       away_state_snapshot, away_note_snapshot, created_at
		FROM match_approval_events WHERE match_id = ? ORDER BY id ASC`, matchID)
	if err != nil {
		return nil, fmt.Errorf("list approval events: %w", err)
	}
	defer rows.Close()
	var out []models.MatchApprovalEvent
	for rows.Next() {
		var e models.MatchApprovalEvent
		var actorUserID, actorPlayerID sql.NullInt64
		var homeSnap, awaySnap sql.NullString
		if err := rows.Scan(&e.ID, &e.MatchID, &e.EventScope, &e.EventType, &e.ScoreRevision,
			&actorUserID, &actorPlayerID, &e.ActorNameSnapshot, &e.Note,
			&homeSnap, &e.HomeNoteSnapshot, &awaySnap, &e.AwayNoteSnapshot, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("list approval events: scan: %w", err)
		}
		if actorUserID.Valid {
			e.ActorUserID = &actorUserID.Int64
		}
		if actorPlayerID.Valid {
			e.ActorPlayerID = &actorPlayerID.Int64
		}
		if homeSnap.Valid {
			e.HomeStateSnapshot = &homeSnap.String
		}
		if awaySnap.Valid {
			e.AwayStateSnapshot = &awaySnap.String
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecordAdminAcceptanceEvent inserts one match-scope match_approval_events
// row for an admin acceptance or override decision.
func (s *RoundStore) RecordAdminAcceptanceEvent(ctx context.Context, ev matches.AdminAcceptanceEvent) error {
	homeState := ev.HomeState
	awayState := ev.AwayState
	if _, err := s.q.ExecContext(ctx, `
		INSERT INTO match_approval_events
			(match_id, event_scope, event_type, score_revision, actor_user_id, note,
			 home_state_snapshot, home_note_snapshot, away_state_snapshot, away_note_snapshot)
		VALUES (?, 'match', ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.MatchID, ev.EventType, ev.ScoreRevision, ev.ActorUserID, ev.Note,
		homeState, ev.HomeNote, awayState, ev.AwayNote); err != nil {
		return fmt.Errorf("record admin acceptance event: %w", err)
	}
	return nil
}

// ApproveMatch sets approved_at to the current time and stores the approver
// and note. approvedByUserID may be nil.
func (s *RoundStore) ApproveMatch(ctx context.Context, matchID int64, approvedByUserID *int64, note string) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE matches SET approved_at=CURRENT_TIMESTAMP, approved_by_user_id=?, approval_note=? WHERE id=?`,
		approvedByUserID, note, matchID)
	if err != nil {
		return fmt.Errorf("approve match: %w", err)
	}
	return nil
}

// ProcessMatch sets processed_at to the current time and stores the processor.
// processedByUserID may be nil.
func (s *RoundStore) ProcessMatch(ctx context.Context, matchID int64, processedByUserID *int64) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE matches SET processed_at=CURRENT_TIMESTAMP, processed_by_user_id=? WHERE id=?`,
		processedByUserID, matchID)
	if err != nil {
		return fmt.Errorf("process match: %w", err)
	}
	return nil
}

// UnapproveMatch clears approved_at, approved_by_user_id, and approval_note.
func (s *RoundStore) UnapproveMatch(ctx context.Context, matchID int64) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE matches SET approved_at=NULL, approved_by_user_id=NULL, approval_note='' WHERE id=?`,
		matchID)
	if err != nil {
		return fmt.Errorf("unapprove match: %w", err)
	}
	return nil
}

// UnprocessMatch clears processed_at and processed_by_user_id. Approval
// fields are left untouched.
func (s *RoundStore) UnprocessMatch(ctx context.Context, matchID int64) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE matches SET processed_at=NULL, processed_by_user_id=NULL WHERE id=?`,
		matchID)
	if err != nil {
		return fmt.Errorf("unprocess match: %w", err)
	}
	return nil
}
