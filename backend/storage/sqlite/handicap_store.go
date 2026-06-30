// Package sqlite provides SQLite-backed implementations of domain Store interfaces.
// Adapters in this package import database/sql and modernc.org/sqlite but must NOT
// import domainerr or any domain service package. All errors are fmt.Errorf wraps.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"league_app/backend/domains/handicaps"
)

// querier is satisfied by both *sql.DB and *sql.Tx, allowing HandicapStore
// to share query methods across transactional and non-transactional contexts.
// ExecContext is required by the Phase B write methods on the tx-scoped store.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// HandicapStore implements handicaps.Store against a SQLite database.
// Use NewHandicapStore; do not copy by value after first use.
type HandicapStore struct {
	db   *sql.DB // used by RunTx to begin transactions
	q    querier // either db or the active *sql.Tx
	inTx bool    // true when this instance is scoped to an active transaction
}

// NewHandicapStore returns a HandicapStore backed by db.
func NewHandicapStore(db *sql.DB) *HandicapStore {
	return &HandicapStore{db: db, q: db}
}

// RunTx executes fn inside a single read/write transaction (BEGIN DEFERRED).
// If the store is already inside a transaction, fn is called directly without nesting.
// A panic in fn rolls back the transaction before re-propagating.
// An error returned by fn rolls back; nil commits.
func (s *HandicapStore) RunTx(ctx context.Context, fn func(handicaps.Store) error) (retErr error) {
	if s.inTx {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("handicap store: begin tx: %w", err)
	}
	txStore := &HandicapStore{db: s.db, q: tx, inTx: true}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if retErr != nil {
			_ = tx.Rollback()
		} else if commitErr := tx.Commit(); commitErr != nil {
			retErr = fmt.Errorf("handicap store: commit: %w", commitErr)
		}
	}()
	retErr = fn(txStore)
	return
}

// SeasonExists returns true when a season with the given ID exists.
func (s *HandicapStore) SeasonExists(ctx context.Context, seasonID int64) (bool, error) {
	var count int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM seasons WHERE id=?`, seasonID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("season %d: exists check: %w", seasonID, err)
	}
	return count > 0, nil
}

// ClosedWeekCount returns the number of distinct week_numbers that have at least
// one match with week_closed=1 in the given season.
func (s *HandicapStore) ClosedWeekCount(ctx context.Context, seasonID int64) (int, error) {
	var count int
	err := s.q.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT week_number) FROM matches WHERE season_id=? AND week_closed=1`,
		seasonID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("season %d: closed week count: %w", seasonID, err)
	}
	return count, nil
}

// SeasonHandicapRules returns raw stored values for the four handicap rule keys.
// A nil field means the row is absent; a non-nil pointer to an empty string means
// the row is present but stored blank. All four keys are fetched in one query.
func (s *HandicapStore) SeasonHandicapRules(ctx context.Context, seasonID int64) (handicaps.HandicapRuleRow, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT rule_key, rule_value FROM season_rules
		WHERE season_id = ? AND rule_key IN (
			'handicap_update_method',
			'handicap_current_game_window',
			'handicap_min_games_for_recommendation',
			'max_individual_handicap'
		)`, seasonID)
	if err != nil {
		return handicaps.HandicapRuleRow{}, fmt.Errorf("season %d: handicap rules query: %w", seasonID, err)
	}
	defer rows.Close()

	var out handicaps.HandicapRuleRow
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			return handicaps.HandicapRuleRow{}, fmt.Errorf("season %d: handicap rules scan: %w", seasonID, err)
		}
		v := val // new variable per iteration so &v is safe
		switch key {
		case "handicap_update_method":
			out.UpdateMethod = &v
		case "handicap_current_game_window":
			out.WindowSize = &v
		case "handicap_min_games_for_recommendation":
			out.Threshold = &v
		case "max_individual_handicap":
			out.MaxHC = &v
		}
	}
	if err := rows.Err(); err != nil {
		return handicaps.HandicapRuleRow{}, fmt.Errorf("season %d: handicap rules rows: %w", seasonID, err)
	}
	return out, nil
}

// SeasonRoster returns all players in season_rosters for this season, joined to
// season_teams.season_name, ordered by team name then player last/first name.
func (s *HandicapStore) SeasonRoster(ctx context.Context, seasonID int64) ([]handicaps.RosterEntry, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT sr.player_id,
		       p.first_name || ' ' || p.last_name,
		       st.season_name,
		       p.handicap,
		       p.admin_hold
		FROM season_rosters sr
		JOIN players     p  ON p.id        = sr.player_id
		JOIN season_teams st ON st.season_id = sr.season_id AND st.team_id = sr.team_id
		WHERE sr.season_id = ?
		ORDER BY st.season_name, p.last_name, p.first_name`,
		seasonID)
	if err != nil {
		return nil, fmt.Errorf("season %d: roster query: %w", seasonID, err)
	}
	defer rows.Close()

	var result []handicaps.RosterEntry
	for rows.Next() {
		var e handicaps.RosterEntry
		var adminHold int
		if err := rows.Scan(&e.PlayerID, &e.PlayerName, &e.TeamName, &e.AssignedHC, &adminHold); err != nil {
			return nil, fmt.Errorf("season %d: roster scan: %w", seasonID, err)
		}
		e.AdminHold = adminHold != 0
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("season %d: roster rows: %w", seasonID, err)
	}
	return result, nil
}

// EligibleRacks returns round_results rows where any player in playerIDs appears
// as home or away, the match has completed=1 AND week_closed=1, and the league
// game_format is '8ball'. Ordered most-recent-first (match_date DESC, match.id DESC,
// round_number DESC). Returns nil when playerIDs is empty.
func (s *HandicapStore) EligibleRacks(ctx context.Context, playerIDs []int64) ([]handicaps.RackRow, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}

	ph := strings.Repeat("?,", len(playerIDs))
	ph = ph[:len(ph)-1]

	// playerIDs appears twice: once for home_player_id IN, once for away_player_id IN.
	args := make([]any, len(playerIDs)*2)
	for i, pid := range playerIDs {
		args[i] = pid
		args[len(playerIDs)+i] = pid
	}

	rows, err := s.q.QueryContext(ctx, fmt.Sprintf(`
		SELECT rr.home_player_id, rr.away_player_id,
		       rr.game1_home, rr.game1_away,
		       rr.game2_home, rr.game2_away,
		       rr.game3_home, rr.game3_away,
		       rr.home_handicap_used, rr.away_handicap_used
		FROM round_results rr
		JOIN matches m ON m.id  = rr.match_id
		JOIN seasons  s ON s.id  = m.season_id
		JOIN leagues  l ON l.id  = s.league_id
		WHERE (rr.home_player_id IN (%s) OR rr.away_player_id IN (%s))
		  AND m.completed   = 1
		  AND m.week_closed = 1
		  AND l.game_format = '8ball'
		ORDER BY m.match_date DESC, m.id DESC, rr.round_number DESC`,
		ph, ph), args...)
	if err != nil {
		return nil, fmt.Errorf("eligible racks query: %w", err)
	}
	defer rows.Close()

	var result []handicaps.RackRow
	for rows.Next() {
		var rr handicaps.RackRow
		var homeHC, awayHC sql.NullFloat64
		if err := rows.Scan(
			&rr.HomePlayerID, &rr.AwayPlayerID,
			&rr.G1H, &rr.G1A,
			&rr.G2H, &rr.G2A,
			&rr.G3H, &rr.G3A,
			&homeHC, &awayHC,
		); err != nil {
			return nil, fmt.Errorf("eligible racks scan: %w", err)
		}
		if homeHC.Valid {
			v := homeHC.Float64
			rr.HomeHCUsed = &v
		}
		if awayHC.Valid {
			v := awayHC.Float64
			rr.AwayHCUsed = &v
		}
		result = append(result, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("eligible racks rows: %w", err)
	}
	return result, nil
}
