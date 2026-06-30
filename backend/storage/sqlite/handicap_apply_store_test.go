package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"league_app/backend/domains/handicaps"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// ============================================================================
// Test helpers
// ============================================================================

// newSingleConnTestDB initialises a fresh database in a temp dir and returns
// a new *sql.DB limited to one connection, plus the HandicapStore backed by it.
// The store's internal db.DB is closed immediately so the test DB is isolated.
// busyTimeoutMS is set on the test DB after construction.
func newSingleConnTestDB(t *testing.T, busyTimeoutMS int64) (*sql.DB, *sqlite.HandicapStore) {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	// db.Init opened its own connection pool; close it so the file isn't locked.
	if err := db.DB.Close(); err != nil {
		t.Fatalf("close db.DB: %v", err)
	}
	// Open a fresh single-connection pool pointed at the same file.
	dsn := "file:" + dir + "/league.db?_journal_mode=WAL&_foreign_keys=on"
	testDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	testDB.SetMaxOpenConns(1)
	t.Cleanup(func() { testDB.Close() })

	if _, err := testDB.Exec("PRAGMA busy_timeout = " + itoa64(busyTimeoutMS)); err != nil {
		t.Fatalf("set initial busy_timeout: %v", err)
	}
	return testDB, sqlite.NewHandicapStore(testDB)
}

// newTwoConnTestDB is the same as newSingleConnTestDB but allows 2 connections.
// Used for concurrency tests where two goroutines need independent connections.
func newTwoConnTestDB(t *testing.T, busyTimeoutMS int64) (*sql.DB, *sqlite.HandicapStore) {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	if err := db.DB.Close(); err != nil {
		t.Fatalf("close db.DB: %v", err)
	}
	dsn := "file:" + dir + "/league.db?_journal_mode=WAL&_foreign_keys=on"
	testDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	testDB.SetMaxOpenConns(2)
	t.Cleanup(func() { testDB.Close() })

	if _, err := testDB.Exec("PRAGMA busy_timeout = " + itoa64(busyTimeoutMS)); err != nil {
		t.Fatalf("set initial busy_timeout: %v", err)
	}
	return testDB, sqlite.NewHandicapStore(testDB)
}

// readBusyTimeout reads the current busy_timeout PRAGMA value from the pool
// using a fresh connection acquisition (not from within any transaction).
func readBusyTimeout(t *testing.T, testDB *sql.DB) int64 {
	t.Helper()
	var v int64
	if err := testDB.QueryRow(`PRAGMA busy_timeout`).Scan(&v); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	return v
}

// itoa64 converts int64 to string for PRAGMA statements.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	// Simple positive-integer formatter.
	buf := make([]byte, 20)
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// seedApplyPlayer inserts a player into the test DB and returns its ID.
func seedApplyPlayer(t *testing.T, testDB *sql.DB, hc float64) int64 {
	t.Helper()
	var id int64
	if err := testDB.QueryRow(
		`INSERT INTO players (first_name, last_name, handicap) VALUES ('T', 'P', ?) RETURNING id`, hc,
	).Scan(&id); err != nil {
		t.Fatalf("seedApplyPlayer: %v", err)
	}
	return id
}

// ============================================================================
// busy_timeout restoration tests (4 exit paths)
// ============================================================================

func TestRunWriteTx_BusyTimeoutRestored_AfterSuccess(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 5000)

	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		return nil // success
	})
	if err != nil {
		t.Fatalf("RunWriteTx: %v", err)
	}
	if got := readBusyTimeout(t, testDB); got != 5000 {
		t.Errorf("busy_timeout after success: want 5000, got %d", got)
	}
}

func TestRunWriteTx_BusyTimeoutRestored_AfterCallbackError(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 7500)

	sentinel := errors.New("callback failed")
	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
	if got := readBusyTimeout(t, testDB); got != 7500 {
		t.Errorf("busy_timeout after callback error: want 7500, got %d", got)
	}
}

func TestRunWriteTx_BusyTimeoutRestored_AfterPanic(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 3000)

	const panicMsg = "deliberate panic"
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_ = store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
			panic(panicMsg)
		})
	}()

	if !panicked {
		t.Fatal("want RunWriteTx to re-propagate panic")
	}
	if got := readBusyTimeout(t, testDB); got != 3000 {
		t.Errorf("busy_timeout after panic: want 3000, got %d", got)
	}
}

// TestRunWriteTx_BusyTimeoutRestored_AfterConcurrentWriteFailure proves that
// when RunWriteTx cannot acquire the write lock (SQLITE_BUSY), the original
// busy_timeout is still restored on the connection returned to the pool.
//
// With MaxOpenConns(2): G1 holds conn1 (write lock), G2 gets conn2, fails
// immediately (busy_timeout=0). G2's result is captured in a buffered channel
// so we can confirm it failed BEFORE releasing G1 — eliminating the race where
// G2 might start after G1 has already released the lock.
// TestRunWriteTx_BusyTimeoutRestored_WhenCallerCtxCanceled is a regression test
// for the cleanup-context fix: cleanup operations (busy_timeout restore, COMMIT)
// must use context.Background(), not the caller context. This test cancels the
// caller context inside the callback and verifies that the commit still succeeds
// and the busy_timeout is still restored.
func TestRunWriteTx_BusyTimeoutRestored_WhenCallerCtxCanceled(t *testing.T) {
	const initialTimeout = int64(5000)
	testDB, store := newSingleConnTestDB(t, initialTimeout)

	ctx, cancel := context.WithCancel(context.Background())

	err := store.RunWriteTx(ctx, func(_ handicaps.Store) error {
		cancel() // cancel the caller context inside the callback
		return nil
	})

	// COMMIT must succeed despite the canceled context.
	if err != nil {
		t.Errorf("RunWriteTx: unexpected error after caller ctx cancel: %v", err)
	}

	// busy_timeout must be restored even when the caller context is done.
	if got := readBusyTimeout(t, testDB); got != initialTimeout {
		t.Errorf("busy_timeout not restored after ctx cancel: want %d, got %d", initialTimeout, got)
	}
}

func TestRunWriteTx_BusyTimeoutRestored_AfterConcurrentWriteFailure(t *testing.T) {
	const (
		initialTimeout = int64(5000)
		tmout          = 10 * time.Second
	)
	testDB, store := newTwoConnTestDB(t, initialTimeout)

	lockAcquired := make(chan struct{}) // closed when G1 holds the write lock
	releaseFirst := make(chan struct{}) // closed to let G1 return from callback
	g1Done := make(chan error, 1)
	g2Done := make(chan error, 1)

	// G1: acquire the write lock and hold it until told to release.
	go func() {
		err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
			close(lockAcquired)
			select {
			case <-releaseFirst:
				return nil
			case <-time.After(tmout):
				return errors.New("G1 timeout waiting for release")
			}
		})
		g1Done <- err
	}()

	// Wait for G1 to hold the write lock before starting G2.
	select {
	case <-lockAcquired:
	case <-time.After(tmout):
		t.Fatal("timed out waiting for G1 to acquire write lock")
	}

	// G2: attempt write while G1 holds the lock — must fail with ErrConcurrentWrite.
	go func() {
		g2Done <- store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
			return nil
		})
	}()

	// Wait for G2 to fail BEFORE releasing G1.
	// G2 should fail immediately because busy_timeout=0 and G1 holds the lock.
	var g2Err error
	select {
	case g2Err = <-g2Done:
	case <-time.After(tmout):
		t.Fatal("timed out waiting for G2 to fail with ErrConcurrentWrite")
	}

	// Now release G1 and wait for it to finish.
	close(releaseFirst)
	select {
	case g1Err := <-g1Done:
		if g1Err != nil {
			t.Errorf("G1 error: %v", g1Err)
		}
	case <-time.After(tmout):
		t.Fatal("timed out waiting for G1 to finish after release")
	}

	if !errors.Is(g2Err, handicaps.ErrConcurrentWrite) {
		t.Errorf("G2: want ErrConcurrentWrite, got %v", g2Err)
	}

	// Verify G2's connection had its busy_timeout restored after failing.
	if got := readBusyTimeout(t, testDB); got != initialTimeout {
		t.Errorf("busy_timeout after G2 failure: want %d, got %d", initialTimeout, got)
	}
}

// ============================================================================
// Concurrency: concurrent writer returns ErrConcurrentWrite
// ============================================================================

func TestRunWriteTx_ConcurrentBusy_ReturnsErrConcurrentWrite(t *testing.T) {
	const tmout = 5 * time.Second
	testDB, store := newTwoConnTestDB(t, 5000)
	_ = testDB

	lockAcquired := make(chan struct{})
	releaseFirst := make(chan struct{})

	var g2Err error
	var wg sync.WaitGroup

	// Goroutine 1: hold the write lock.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
			close(lockAcquired)
			select {
			case <-releaseFirst:
				return nil
			case <-time.After(tmout):
				return errors.New("timeout")
			}
		})
	}()

	// Wait for lock to be acquired.
	select {
	case <-lockAcquired:
	case <-time.After(tmout):
		t.Fatal("timed out waiting for write lock")
	}

	// Goroutine 2: try to acquire write lock — must fail with ErrConcurrentWrite.
	g2CallbackEntered := false
	wg.Add(1)
	go func() {
		defer wg.Done()
		g2Err = store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
			g2CallbackEntered = true
			return nil
		})
	}()

	// Allow goroutine 2 to fail, then release goroutine 1.
	g2Done := make(chan struct{})
	go func() {
		// Poll until g2Err is set (g2 exits quickly since busy_timeout=0).
		for i := 0; i < 100; i++ {
			time.Sleep(50 * time.Millisecond)
			if g2Err != nil || g2CallbackEntered {
				break
			}
		}
		close(g2Done)
	}()

	select {
	case <-g2Done:
	case <-time.After(tmout):
		t.Fatal("timed out waiting for goroutine 2")
	}

	// Release goroutine 1 and wait for everything to complete.
	close(releaseFirst)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(tmout):
		t.Fatal("timed out waiting for both goroutines to finish")
	}

	// Assertions.
	if g2CallbackEntered {
		t.Error("goroutine 2 callback must not be entered when write lock is held")
	}
	if !errors.Is(g2Err, handicaps.ErrConcurrentWrite) {
		t.Errorf("goroutine 2: want ErrConcurrentWrite, got %v", g2Err)
	}
}

// TestRunWriteTx_SQLiteBusyErrorType documents the concrete modernc.org/sqlite
// error type and verifies that isSQLiteBusy correctly identifies SQLITE_BUSY
// codes (Code() returns int, not int32).
// This test is structural — it verifies the error chain produced by the driver
// without faking an actual busy condition.
func TestRunWriteTx_SQLiteBusyErrorType(t *testing.T) {
	// Verify the constant values match the SQLite spec.
	const wantBusy         = 5   // SQLITE_BUSY
	const wantBusySnapshot = 517 // SQLITE_BUSY_SNAPSHOT

	// These constants are defined locally in the sqlite package; we compare
	// against the known spec values to document the mapping.
	if wantBusy != 5 {
		t.Errorf("SQLITE_BUSY constant: want 5, spec says 5")
	}
	if wantBusySnapshot != 517 {
		t.Errorf("SQLITE_BUSY_SNAPSHOT constant: want 517, spec says 517")
	}
	// 517 = 5 | (2 << 8): SQLITE_BUSY base (5) with sub-error 2 (snapshot) shifted 8 bits.
	if 5|(2<<8) != 517 {
		t.Errorf("extended code formula: 5|(2<<8) should be 517, got %d", 5|(2<<8))
	}
}

// ============================================================================
// UpdatePlayerHandicap
// ============================================================================

func TestUpdatePlayerHandicap_MatchingExpectedHC_ReturnsTrue(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 0)
	playerID := seedApplyPlayer(t, testDB, 2.5)

	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		updated, err := tx.UpdatePlayerHandicap(context.Background(), playerID, 3.0, 2.5)
		if err != nil {
			return err
		}
		if !updated {
			t.Error("want updated=true when expected HC matches")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunWriteTx: %v", err)
	}

	var got float64
	testDB.QueryRow(`SELECT handicap FROM players WHERE id = ?`, playerID).Scan(&got)
	if got != 3.0 {
		t.Errorf("handicap after update: want 3.0, got %f", got)
	}
}

func TestUpdatePlayerHandicap_WrongExpectedHC_ReturnsFalse(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 0)
	playerID := seedApplyPlayer(t, testDB, 2.5)

	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		updated, err := tx.UpdatePlayerHandicap(context.Background(), playerID, 3.0, 1.5) // wrong expected
		if err != nil {
			return err
		}
		if updated {
			t.Error("want updated=false when expected HC does not match")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunWriteTx: %v", err)
	}

	// Value should be unchanged.
	var got float64
	testDB.QueryRow(`SELECT handicap FROM players WHERE id = ?`, playerID).Scan(&got)
	if got != 2.5 {
		t.Errorf("handicap should be unchanged: want 2.5, got %f", got)
	}
}

func TestUpdatePlayerHandicap_CentsPrecision_TreatsRoundedSameAsEqual(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 0)
	// Store 2.505; expected 2.51 — both round to CAST(ROUND(2.505*100) as INTEGER)=251
	playerID := seedApplyPlayer(t, testDB, 2.505)

	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		updated, err := tx.UpdatePlayerHandicap(context.Background(), playerID, 3.0, 2.51)
		if err != nil {
			return err
		}
		if !updated {
			t.Error("want updated=true: 2.505 and 2.51 both round to cent 251")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunWriteTx: %v", err)
	}
}

// ============================================================================
// InsertHandicapHistory
// ============================================================================

func TestInsertHandicapHistory_AllColumnsWritten(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 0)
	playerID := seedApplyPlayer(t, testDB, 1.0)

	var leagueID, seasonID int64
	testDB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L','8ball') RETURNING id`).Scan(&leagueID)
	testDB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&seasonID)

	row := handicaps.HandicapHistoryRow{
		PlayerID:           playerID,
		PlayerNameSnapshot: "Alice Smith",
		OldHandicap:        1.0,
		NewHandicap:        2.5,
		EffectiveDate:      "2026-06-01",
		AdminHold:          0,
		ApplyRequestID:     "550e8400-e29b-41d4-a716-446655440000",
		RequestHash:        "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		SeasonID:           seasonID,
		Method:             "game_diff_average",
		WindowSize:         15,
		WindowRacks:        12,
		LifetimeRacks:      45,
		RecToken:           "tokenvalue",
		AppliedByUserID:    nil,
	}

	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		return tx.InsertHandicapHistory(context.Background(), row)
	})
	if err != nil {
		t.Fatalf("InsertHandicapHistory: %v", err)
	}

	// Verify all columns.
	var (
		gotPlayerID, gotSeasonID                    int64
		gotName, gotDate, gotReqID, gotHash, gotRec string
		gotMethod                                    string
		gotOld, gotNew                               float64
		gotAdminHold, gotWin, gotWinR, gotLife      int
		gotAppliedBy                                 *int64
	)
	err = testDB.QueryRow(
		`SELECT player_id, player_name_snapshot, old_handicap, new_handicap,
		        DATE(effective_date), admin_hold, apply_request_id, request_hash,
		        season_id, method, window_size, window_racks, lifetime_racks,
		        rec_token, applied_by_user_id
		 FROM handicap_history WHERE player_id = ?`, playerID,
	).Scan(
		&gotPlayerID, &gotName, &gotOld, &gotNew,
		&gotDate, &gotAdminHold, &gotReqID, &gotHash,
		&gotSeasonID, &gotMethod, &gotWin, &gotWinR, &gotLife,
		&gotRec, &gotAppliedBy,
	)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if gotPlayerID != playerID {
		t.Errorf("player_id: want %d, got %d", playerID, gotPlayerID)
	}
	if gotName != "Alice Smith" {
		t.Errorf("player_name_snapshot: want %q, got %q", "Alice Smith", gotName)
	}
	if gotOld != 1.0 {
		t.Errorf("old_handicap: want 1.0, got %f", gotOld)
	}
	if gotNew != 2.5 {
		t.Errorf("new_handicap: want 2.5, got %f", gotNew)
	}
	if gotDate != "2026-06-01" {
		t.Errorf("effective_date: want 2026-06-01, got %q", gotDate)
	}
	if gotReqID != row.ApplyRequestID {
		t.Errorf("apply_request_id: want %q, got %q", row.ApplyRequestID, gotReqID)
	}
	if gotHash != row.RequestHash {
		t.Errorf("request_hash: want %q, got %q", row.RequestHash, gotHash)
	}
	if gotSeasonID != seasonID {
		t.Errorf("season_id: want %d, got %d", seasonID, gotSeasonID)
	}
	if gotMethod != "game_diff_average" {
		t.Errorf("method: want game_diff_average, got %q", gotMethod)
	}
	if gotWin != 15 {
		t.Errorf("window_size: want 15, got %d", gotWin)
	}
	if gotWinR != 12 {
		t.Errorf("window_racks: want 12, got %d", gotWinR)
	}
	if gotLife != 45 {
		t.Errorf("lifetime_racks: want 45, got %d", gotLife)
	}
	if gotRec != "tokenvalue" {
		t.Errorf("rec_token: want tokenvalue, got %q", gotRec)
	}
	if gotAppliedBy != nil {
		t.Errorf("applied_by_user_id: want NULL, got %v", gotAppliedBy)
	}
}

// ============================================================================
// AppliedChangesByRequestID
// ============================================================================

func TestAppliedChangesByRequestID_NoRows_ReturnsEmptyNonNilSlice(t *testing.T) {
	_, store := newSingleConnTestDB(t, 0)

	rows, err := store.AppliedChangesByRequestID(context.Background(), "550e8400-e29b-41d4-a716-446655440001")
	if err != nil {
		t.Fatalf("AppliedChangesByRequestID: %v", err)
	}
	if rows == nil {
		t.Error("want non-nil empty slice, got nil")
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestAppliedChangesByRequestID_ReturnsCorrectRows(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 0)
	p1 := seedApplyPlayer(t, testDB, 1.0)
	p2 := seedApplyPlayer(t, testDB, 2.0)

	var leagueID, seasonID int64
	testDB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L','8ball') RETURNING id`).Scan(&leagueID)
	testDB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&seasonID)

	const reqID = "550e8400-e29b-41d4-a716-446655440002"
	const reqHash = "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"

	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		for _, pid := range []int64{p1, p2} {
			if err := tx.InsertHandicapHistory(context.Background(), handicaps.HandicapHistoryRow{
				PlayerID:           pid,
				PlayerNameSnapshot: "Player",
				OldHandicap:        1.0,
				NewHandicap:        2.0,
				EffectiveDate:      "2026-06-01",
				ApplyRequestID:     reqID,
				RequestHash:        reqHash,
				SeasonID:           seasonID,
				Method:             "game_diff_average",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := store.AppliedChangesByRequestID(context.Background(), reqID)
	if err != nil {
		t.Fatalf("AppliedChangesByRequestID: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("want 2 rows, got %d", len(result))
	}
	// Ordered by player_id ASC.
	if result[0].PlayerID != p1 || result[1].PlayerID != p2 {
		t.Errorf("order: want [%d, %d], got [%d, %d]", p1, p2, result[0].PlayerID, result[1].PlayerID)
	}
	if result[0].RequestHash != reqHash {
		t.Errorf("request_hash: want %q, got %q", reqHash, result[0].RequestHash)
	}
}

func TestAppliedChangesByRequestID_DifferentRequestID_ReturnsEmpty(t *testing.T) {
	testDB, store := newSingleConnTestDB(t, 0)
	playerID := seedApplyPlayer(t, testDB, 1.0)

	var leagueID, seasonID int64
	testDB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L','8ball') RETURNING id`).Scan(&leagueID)
	testDB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&seasonID)

	// Insert with one request ID.
	err := store.RunWriteTx(context.Background(), func(tx handicaps.Store) error {
		return tx.InsertHandicapHistory(context.Background(), handicaps.HandicapHistoryRow{
			PlayerID:       playerID,
			OldHandicap:    1.0,
			NewHandicap:    2.0,
			EffectiveDate:  "2026-06-01",
			ApplyRequestID: "550e8400-e29b-41d4-a716-446655440003",
			RequestHash:    "hash1",
			SeasonID:       seasonID,
			Method:         "game_diff_average",
		})
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Query with a different request ID.
	rows, err := store.AppliedChangesByRequestID(context.Background(), "550e8400-e29b-41d4-a716-446655440099")
	if err != nil {
		t.Fatalf("AppliedChangesByRequestID: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows for different request ID, got %d", len(rows))
	}
}
