// Package sqlite white-box tests for HandicapStore.RunTx.
// This file is in package sqlite (not package sqlite_test) so it can access
// unexported fields (q, inTx) and type-assert tx.(*HandicapStore).q.(*sql.Tx)
// to perform writes inside a callback without needing write methods on the
// handicaps.Store interface.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"league_app/backend/domains/handicaps"
	"league_app/db"
)

// initInternalDB initialises a fresh SQLite database in a temp directory.
// The DB is closed automatically when the test ends.
func initInternalDB(t *testing.T) {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
}

// writeInsideRunTx type-asserts the tx-scoped Store to *HandicapStore, then
// asserts its querier to *sql.Tx, and executes the given SQL directly.
// This is the only way to prove rollback/commit behaviour because the
// handicaps.Store interface intentionally has no write methods.
func writeInsideRunTx(t *testing.T, tx handicaps.Store, query string, args ...any) {
	t.Helper()
	hs, ok := tx.(*HandicapStore)
	if !ok {
		t.Fatalf("type assertion tx.(*HandicapStore) failed: got %T", tx)
	}
	sqlTx, ok := hs.q.(*sql.Tx)
	if !ok {
		t.Fatalf("type assertion hs.q.(*sql.Tx) failed: got %T", hs.q)
	}
	if _, err := sqlTx.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("write inside tx: %v", err)
	}
}

// rowCountDirect counts rows via db.DB (outside any transaction).
func rowCountDirect(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return n
}

// --- RunTx white-box tests ---------------------------------------------------

// TestHandicapStore_RunTx_RollsBackWriteOnError inserts a season_rule row inside
// a RunTx callback, returns an error, and verifies the row is absent afterwards.
func TestHandicapStore_RunTx_RollsBackWriteOnError(t *testing.T) {
	initInternalDB(t)

	var leagueID, seasonID int64
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L', '8ball') RETURNING id`).Scan(&leagueID)
	db.DB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&seasonID)

	store := NewHandicapStore(db.DB)
	sentinel := errors.New("abort tx")

	err := store.RunTx(context.Background(), func(tx handicaps.Store) error {
		writeInsideRunTx(t, tx,
			`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'test_key', '', 'val')`,
			seasonID)
		return sentinel // trigger rollback
	})

	if !errors.Is(err, sentinel) {
		t.Errorf("want sentinel error, got %v", err)
	}
	n := rowCountDirect(t, `SELECT COUNT(*) FROM season_rules WHERE season_id=? AND rule_key='test_key'`, seasonID)
	if n != 0 {
		t.Errorf("want 0 rows after rollback, got %d", n)
	}
}

// TestHandicapStore_RunTx_PanicRollsBack inserts a row inside RunTx, panics inside
// the callback, and asserts three things:
//  1. the panic is re-propagated by RunTx (not swallowed);
//  2. the recovered value matches the deliberate panic value;
//  3. the transactional write is rolled back.
func TestHandicapStore_RunTx_PanicRollsBack(t *testing.T) {
	initInternalDB(t)

	var leagueID, seasonID int64
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L', '8ball') RETURNING id`).Scan(&leagueID)
	db.DB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&seasonID)

	store := NewHandicapStore(db.DB)

	const panicMsg = "deliberate panic for RunTx test"
	var recovered any
	panicked := false

	// Wrap RunTx in an anonymous function so we can recover the re-propagated panic.
	func() {
		defer func() {
			r := recover()
			if r != nil {
				panicked = true
				recovered = r
			}
		}()
		_ = store.RunTx(context.Background(), func(tx handicaps.Store) error {
			writeInsideRunTx(t, tx,
				`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'panic_key', '', 'val')`,
				seasonID)
			panic(panicMsg)
		})
	}()

	// Assert 1: RunTx must re-propagate the panic; test fails if it was swallowed.
	if !panicked {
		t.Fatal("want RunTx to re-propagate the panic; no panic was observed (RunTx may have swallowed it)")
	}
	// Assert 2: the recovered value must match the deliberate panic value.
	if recovered != panicMsg {
		t.Errorf("want recovered value %q, got %v", panicMsg, recovered)
	}
	// Assert 3: the transactional write must have been rolled back.
	n := rowCountDirect(t, `SELECT COUNT(*) FROM season_rules WHERE season_id=? AND rule_key='panic_key'`, seasonID)
	if n != 0 {
		t.Errorf("want 0 rows after panic+rollback, got %d", n)
	}
}

// TestHandicapStore_RunTx_CommitsSuccessfulWrite inserts a row inside RunTx,
// returns nil, and verifies the row is present afterwards.
func TestHandicapStore_RunTx_CommitsSuccessfulWrite(t *testing.T) {
	initInternalDB(t)

	var leagueID, seasonID int64
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L', '8ball') RETURNING id`).Scan(&leagueID)
	db.DB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&seasonID)

	store := NewHandicapStore(db.DB)

	err := store.RunTx(context.Background(), func(tx handicaps.Store) error {
		writeInsideRunTx(t, tx,
			`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'commit_key', '', 'val')`,
			seasonID)
		return nil // trigger commit
	})

	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	n := rowCountDirect(t, `SELECT COUNT(*) FROM season_rules WHERE season_id=? AND rule_key='commit_key'`, seasonID)
	if n != 1 {
		t.Errorf("want 1 committed row, got %d", n)
	}
}

// TestHandicapStore_RunTx_SnapshotConsistency proves that the read transaction
// holds a stable WAL snapshot: an external commit that occurs after the first
// read establishes the snapshot is invisible to subsequent reads inside the same
// transaction, and becomes visible only after the transaction ends.
//
// Synchronization is deterministic -- channels, no sleeps:
//
//	snapshotEstablished (close) - RunTx first read done; goroutine may write
//	writeResult (send)          - goroutine write complete (or error)
//
// SQLite WAL mode allows the external writer to commit while the read
// transaction is open because WAL readers hold no file locks that block writers.
//
// Note on commit-error injection: SQLite does not provide a reliable mechanism
// to force a commit failure on an open, writable database without corrupting
// the file. Testing the "commit fails -> retErr set" branch would require a
// database-level fault injection harness that is out of scope for unit tests.
// The named-return defer pattern is verified structurally by code review.
func TestHandicapStore_RunTx_SnapshotConsistency(t *testing.T) {
	initInternalDB(t)

	var leagueID, seasonID, teamID int64
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L', '8ball') RETURNING id`).Scan(&leagueID)
	db.DB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&seasonID)
	db.DB.QueryRow(`INSERT INTO teams (league_id, name) VALUES (?, 'T') RETURNING id`, leagueID).Scan(&teamID)

	// Channels for deterministic synchronization; no sleeps.
	snapshotEstablished := make(chan struct{}) // closed when first tx read is done
	writeResult := make(chan error, 1)         // goroutine sends write outcome

	// External writer: wait for the tx snapshot to be established, then insert
	// a closed match using a separate connection (db.DB, not the tx connection).
	// In WAL mode the writer does not block and does not wait for the reader.
	go func() {
		<-snapshotEstablished
		_, err := db.DB.Exec(
			`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, match_number, completed, week_closed) VALUES (?, ?, ?, '2026-01-07', 1, 1, 1, 1)`,
			seasonID, teamID, teamID)
		writeResult <- err
	}()

	var closedWeeksInsideTxBefore, closedWeeksInsideTxAfter int
	store := NewHandicapStore(db.DB)

	err := store.RunTx(context.Background(), func(tx handicaps.Store) error {
		// First read establishes the WAL read snapshot (mxFrame = current WAL tip).
		n, err := tx.ClosedWeekCount(context.Background(), seasonID)
		if err != nil {
			return err
		}
		closedWeeksInsideTxBefore = n // expect 0: no matches yet

		// Signal the external writer to proceed.
		close(snapshotEstablished)
		// Wait for the external write to commit. This is deterministic: the
		// goroutine closes writeResult only after db.DB.Exec returns.
		if writeErr := <-writeResult; writeErr != nil {
			return fmt.Errorf("external write: %w", writeErr)
		}

		// Second read inside the same transaction. WAL snapshot isolation
		// means this must still return 0 even though the external commit has
		// landed in the WAL at a frame beyond our read mark.
		n2, err := tx.ClosedWeekCount(context.Background(), seasonID)
		if err != nil {
			return err
		}
		closedWeeksInsideTxAfter = n2 // want 0 (snapshot isolation)
		return nil
	})
	if err != nil {
		t.Fatalf("RunTx: %v", err)
	}

	// After the read transaction ends, a fresh read must see the external commit.
	closedWeeksAfterTx, err := store.ClosedWeekCount(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("post-tx ClosedWeekCount: %v", err)
	}

	if closedWeeksInsideTxBefore != 0 {
		t.Errorf("before external write (inside tx): want 0 closed weeks, got %d", closedWeeksInsideTxBefore)
	}
	if closedWeeksInsideTxAfter != 0 {
		t.Errorf("after external write (inside tx, snapshot isolation): want 0 closed weeks, got %d", closedWeeksInsideTxAfter)
	}
	if closedWeeksAfterTx != 1 {
		t.Errorf("after tx ends: want 1 closed week (external commit now visible), got %d", closedWeeksAfterTx)
	}
}
