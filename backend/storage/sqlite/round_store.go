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
func (s *RoundStore) GetPlayerStats(ctx context.Context, req matches.PlayerStatsRequest) ([]models.PlayerStat, error) {
	var query string
	var args []any
	switch {
	case req.SeasonID != 0:
		query = `
			SELECT p.id, COALESCE(p.player_number,''), p.first_name || ' ' || p.last_name,
			       COALESCE(t.name,''), p.handicap,
			       COALESCE(SUM(mr.sets_won),0), COALESCE(SUM(mr.sets_lost),0),
			       COALESCE(SUM(mr.games_won),0), COALESCE(SUM(mr.games_lost),0)
			FROM players p
			JOIN teams t ON t.id = p.team_id
			JOIN seasons se ON se.league_id = t.league_id AND se.id = ?
			LEFT JOIN match_results mr ON mr.player_id = p.id
			    AND mr.match_id IN (
			        SELECT id FROM matches
			        WHERE season_id=? AND completed=1 AND week_closed=1
			    )
			GROUP BY p.id ORDER BY SUM(mr.sets_won) DESC, SUM(mr.games_won) DESC`
		args = []any{req.SeasonID, req.SeasonID}
	case req.LeagueID != 0:
		query = `
			SELECT p.id, COALESCE(p.player_number,''), p.first_name || ' ' || p.last_name,
			       COALESCE(t.name,''), p.handicap,
			       COALESCE(SUM(mr.sets_won),0), COALESCE(SUM(mr.sets_lost),0),
			       COALESCE(SUM(mr.games_won),0), COALESCE(SUM(mr.games_lost),0)
			FROM players p
			JOIN teams t ON t.id = p.team_id AND t.league_id = ?
			LEFT JOIN match_results mr ON mr.player_id = p.id
			GROUP BY p.id ORDER BY SUM(mr.sets_won) DESC, SUM(mr.games_won) DESC`
		args = []any{req.LeagueID}
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

// SubmitMatchResults replaces match_results for a match and marks it completed,
// wrapped in a transaction.
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

// ClearMatchResults deletes match_results for a match and marks it incomplete.
func (s *RoundStore) ClearMatchResults(ctx context.Context, matchID int64) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM match_results WHERE match_id=?`, matchID); err != nil {
		return fmt.Errorf("clear match results: %w", err)
	}
	if _, err := s.q.ExecContext(ctx, `UPDATE matches SET completed=0 WHERE id=?`, matchID); err != nil {
		return fmt.Errorf("clear match results: mark incomplete: %w", err)
	}
	return nil
}
