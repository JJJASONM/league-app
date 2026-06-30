package sqlite

// Phase B write adapter for HandicapStore: RunWriteTx, UpdatePlayerHandicap,
// InsertHandicapHistory, AppliedChangesByRequestID.
// isSQLiteBusy uses the modernc.org/sqlite error type (Code() int, not int32).

import (
	"context"
	"errors"
	"fmt"

	sqlib "modernc.org/sqlite"

	"league_app/backend/domains/handicaps"
)

// SQLite extended result codes for busy conditions.
// Extended result codes are enabled by the modernc driver at connection open.
const (
	sqliteBusy         = 5   // SQLITE_BUSY
	sqliteBusySnapshot = 517 // SQLITE_BUSY_SNAPSHOT
)

// isSQLiteBusy reports whether err is a SQLite busy error (SQLITE_BUSY or
// SQLITE_BUSY_SNAPSHOT). The concrete error type is *sqlib.Error with
// Code() int — not int32. Extended result codes are enabled by the driver.
func isSQLiteBusy(err error) bool {
	var sqErr *sqlib.Error
	if !errors.As(err, &sqErr) {
		return false
	}
	c := sqErr.Code()
	return c == sqliteBusy || c == sqliteBusySnapshot
}

// RunWriteTx executes fn inside a BEGIN IMMEDIATE transaction using a dedicated
// connection acquired from the pool. This serialises all handicap writes at the
// SQLite level; no other writer can start inside the IMMEDIATE lock window.
//
// Busy-timeout protocol:
//  1. Read the original busy_timeout PRAGMA before changing anything.
//  2. Register defer (runs second-to-last, LIFO): restore original busy_timeout.
//  3. Set busy_timeout=0 so that BEGIN IMMEDIATE fails instantly on contention.
//  4. Attempt BEGIN IMMEDIATE; SQLITE_BUSY/BUSY_SNAPSHOT → wrap ErrConcurrentWrite.
//  5. Register defer (runs first, LIFO): ROLLBACK on error, COMMIT on success.
//  6. Call fn with the tx-scoped store.
//
// LIFO defer execution order:
//   - defer 3 (commit/rollback) — runs FIRST
//   - defer 2 (restore timeout)  — runs SECOND (conn still valid)
//   - defer 1 (conn.Close)       — runs LAST (returns conn to pool)
//
// Restoration errors are attached to retErr only when no more important error
// is already set, so BEGIN/callback/commit failures are never masked.
func (s *HandicapStore) RunWriteTx(ctx context.Context, fn func(handicaps.Store) error) (retErr error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	// Defer 1: return connection to pool — runs LAST.
	defer conn.Close()

	// cleanupCtx is independent of the caller's context so that deferred cleanup
	// (busy_timeout restore, ROLLBACK, COMMIT) succeeds even if the request context
	// is canceled or times out after the transaction starts.
	cleanupCtx := context.Background()

	// Read original busy_timeout before modifying it.
	var origBusyTimeout int64
	if err := conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&origBusyTimeout); err != nil {
		return fmt.Errorf("read busy_timeout: %w", err)
	}

	// Defer 2: restore original busy_timeout — runs SECOND-TO-LAST.
	// Runs even when BEGIN IMMEDIATE fails (before Defer 3 is registered).
	defer func() {
		_, restoreErr := conn.ExecContext(cleanupCtx, fmt.Sprintf(`PRAGMA busy_timeout = %d`, origBusyTimeout))
		if restoreErr != nil && retErr == nil {
			retErr = fmt.Errorf("restore busy_timeout: %w", restoreErr)
		}
	}()

	// Set busy_timeout=0: BEGIN IMMEDIATE returns SQLITE_BUSY immediately on
	// contention rather than waiting.
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 0`); err != nil {
		return fmt.Errorf("set busy_timeout=0: %w", err)
	}

	// Acquire the exclusive write lock.
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		if isSQLiteBusy(err) {
			return fmt.Errorf("%w: %v", handicaps.ErrConcurrentWrite, err)
		}
		return fmt.Errorf("BEGIN IMMEDIATE: %w", err)
	}

	// Defer 3: commit or rollback — runs FIRST (innermost defer).
	// Uses cleanupCtx so that commit/rollback succeeds even if caller ctx is done.
	defer func() {
		if p := recover(); p != nil {
			conn.ExecContext(cleanupCtx, `ROLLBACK`) //nolint:errcheck — rolling back after panic
			panic(p)                                 // re-propagate
		}
		if retErr != nil {
			conn.ExecContext(cleanupCtx, `ROLLBACK`) //nolint:errcheck — best-effort rollback
			return
		}
		if _, commitErr := conn.ExecContext(cleanupCtx, `COMMIT`); commitErr != nil {
			conn.ExecContext(cleanupCtx, `ROLLBACK`) //nolint:errcheck — best-effort rollback after commit failure
			retErr = fmt.Errorf("COMMIT: %w", commitErr)
		}
	}()

	// Execute the callback with a tx-scoped HandicapStore that routes all
	// reads and writes through conn (the connection holding the IMMEDIATE lock).
	txStore := &HandicapStore{db: s.db, q: conn, inTx: true}
	retErr = fn(txStore)
	return retErr
}

// UpdatePlayerHandicap sets players.handicap = newHC for the given player,
// conditional on the current stored value rounding to the same cent as expectedHC.
// Returns (true, nil) when the row was updated; (false, nil) when the
// conditional check failed (another writer changed the value).
func (s *HandicapStore) UpdatePlayerHandicap(ctx context.Context, playerID int64, newHC, expectedHC float64) (bool, error) {
	res, err := s.q.ExecContext(ctx,
		`UPDATE players SET handicap = ?
		 WHERE id = ?
		   AND CAST(ROUND(handicap * 100) AS INTEGER) = CAST(ROUND(? * 100) AS INTEGER)`,
		newHC, playerID, expectedHC,
	)
	if err != nil {
		return false, fmt.Errorf("UpdatePlayerHandicap: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("UpdatePlayerHandicap RowsAffected: %w", err)
	}
	return n == 1, nil
}

// InsertHandicapHistory inserts one row into handicap_history with all Phase B columns.
// All fifteen values are written; AppliedByUserID becomes SQL NULL when nil.
func (s *HandicapStore) InsertHandicapHistory(ctx context.Context, row handicaps.HandicapHistoryRow) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO handicap_history
		 (player_id, player_name_snapshot,
		  old_handicap, new_handicap, effective_date, admin_hold,
		  apply_request_id, request_hash,
		  season_id, method, window_size, window_racks, lifetime_racks,
		  rec_token, applied_by_user_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.PlayerID, row.PlayerNameSnapshot,
		row.OldHandicap, row.NewHandicap, row.EffectiveDate, row.AdminHold,
		row.ApplyRequestID, row.RequestHash,
		row.SeasonID, row.Method, row.WindowSize, row.WindowRacks, row.LifetimeRacks,
		row.RecToken, row.AppliedByUserID,
	)
	if err != nil {
		return fmt.Errorf("InsertHandicapHistory: %w", err)
	}
	return nil
}

// AppliedChangesByRequestID returns all handicap_history rows for the given
// apply_request_id, ordered by player_id ASC. Returns an empty (non-nil) slice
// when no rows exist so callers can check len() without nil-guard.
func (s *HandicapStore) AppliedChangesByRequestID(ctx context.Context, applyRequestID string) ([]handicaps.AppliedHistory, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT player_id,
		        COALESCE(player_name_snapshot, ''),
		        COALESCE(old_handicap, 0),
		        COALESCE(new_handicap, 0),
		        COALESCE(request_hash, '')
		 FROM handicap_history
		 WHERE apply_request_id = ?
		 ORDER BY player_id ASC`,
		applyRequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("AppliedChangesByRequestID: %w", err)
	}
	defer rows.Close()

	out := []handicaps.AppliedHistory{} // non-nil empty slice
	for rows.Next() {
		var h handicaps.AppliedHistory
		if err := rows.Scan(&h.PlayerID, &h.PlayerNameSnapshot, &h.OldHandicap, &h.NewHandicap, &h.RequestHash); err != nil {
			return nil, fmt.Errorf("AppliedChangesByRequestID scan: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("AppliedChangesByRequestID rows: %w", err)
	}
	return out, nil
}
