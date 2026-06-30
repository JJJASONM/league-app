package db_test

import (
	"testing"

	"league_app/db"
)

// preApplyHandicapHistorySchema is the original CREATE TABLE statement for
// handicap_history before Phase B columns were added. Used to simulate an
// existing database that needs upgrading.
const preApplyHandicapHistorySchema = `
CREATE TABLE IF NOT EXISTS handicap_history (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id      INTEGER NOT NULL REFERENCES players(id),
    old_handicap   REAL    NOT NULL,
    new_handicap   REAL    NOT NULL,
    effective_date DATE    NOT NULL,
    admin_hold     INTEGER NOT NULL DEFAULT 0,
    note           TEXT,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// phaseBAuditColumns lists all ten Phase B column names that must be present
// after migration. Used by both fresh-DB and upgrade-path tests.
var phaseBAuditColumns = []string{
	"apply_request_id",
	"request_hash",
	"player_name_snapshot",
	"season_id",
	"method",
	"window_size",
	"window_racks",
	"lifetime_racks",
	"rec_token",
	"applied_by_user_id",
}

// hasColumn queries PRAGMA table_info to check whether a column exists.
func hasColumn(t *testing.T, table, column string) bool {
	t.Helper()
	rows, err := db.DB.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}

// hasIndex queries sqlite_master to check whether an index exists.
func hasIndex(t *testing.T, indexName string) bool {
	t.Helper()
	var count int
	if err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, indexName,
	).Scan(&count); err != nil {
		t.Fatalf("index check: %v", err)
	}
	return count == 1
}

// TestHandicapHistoryMigration_FreshDB_AllColumnsPresent verifies that a fresh
// database (created by db.Init from scratch) already has all Phase B columns and
// the idempotency index from the additive migrations slice.
func TestHandicapHistoryMigration_FreshDB_AllColumnsPresent(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	for _, col := range phaseBAuditColumns {
		if !hasColumn(t, "handicap_history", col) {
			t.Errorf("fresh DB: missing column handicap_history.%s", col)
		}
	}
	if !hasIndex(t, "idx_hc_history_apply_idempotent") {
		t.Error("fresh DB: missing index idx_hc_history_apply_idempotent")
	}
}

// TestHandicapHistoryMigration_PreApplyDB_UpgradesWithoutDataLoss simulates an
// existing database that was created before Phase B columns existed. It:
//  1. Calls db.Init to create the schema (with all new columns).
//  2. Drops and recreates handicap_history using the pre-Phase-B schema.
//  3. Seeds a legacy row (no Phase B columns).
//  4. Closes db.DB to release the connection.
//  5. Re-runs db.Init to apply additive migrations.
//  6. Verifies all Phase B columns are present.
//  7. Verifies the legacy row is intact (old_handicap and new_handicap correct).
func TestHandicapHistoryMigration_PreApplyDB_UpgradesWithoutDataLoss(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("first db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	// Seed required parent data.
	var playerID int64
	if err := db.DB.QueryRow(
		`INSERT INTO players (first_name, last_name, handicap) VALUES ('A','B',1.0) RETURNING id`,
	).Scan(&playerID); err != nil {
		t.Fatalf("seed player: %v", err)
	}

	// Drop and recreate handicap_history without Phase B columns.
	if _, err := db.DB.Exec(`DROP TABLE handicap_history`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := db.DB.Exec(preApplyHandicapHistorySchema); err != nil {
		t.Fatalf("recreate pre-phase-B table: %v", err)
	}

	// Seed a legacy row using only the original columns.
	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?, 1.5, 2.0, '2025-01-01')`, playerID,
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// Close db.DB before re-init (required on Windows; avoids lock conflict).
	if err := db.DB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Re-run Init — additive migrations should add the Phase B columns.
	if err := db.Init(dir); err != nil {
		t.Fatalf("second db.Init: %v", err)
	}

	// All Phase B columns must now exist.
	for _, col := range phaseBAuditColumns {
		if !hasColumn(t, "handicap_history", col) {
			t.Errorf("after upgrade: missing column handicap_history.%s", col)
		}
	}
	if !hasIndex(t, "idx_hc_history_apply_idempotent") {
		t.Error("after upgrade: missing index idx_hc_history_apply_idempotent")
	}

	// Legacy row must be preserved with correct values.
	var gotOld, gotNew float64
	if err := db.DB.QueryRow(
		`SELECT old_handicap, new_handicap FROM handicap_history WHERE player_id = ?`, playerID,
	).Scan(&gotOld, &gotNew); err != nil {
		t.Fatalf("read legacy row: %v", err)
	}
	if gotOld != 1.5 {
		t.Errorf("old_handicap: want 1.5, got %f", gotOld)
	}
	if gotNew != 2.0 {
		t.Errorf("new_handicap: want 2.0, got %f", gotNew)
	}
}

// TestHandicapHistoryMigration_Idempotent_SecondInitNoError verifies that
// running db.Init twice on the same database directory does not return an error.
// The additive migrations use CREATE UNIQUE INDEX IF NOT EXISTS and ALTER TABLE
// (whose errors are intentionally ignored), so double-init must be harmless.
func TestHandicapHistoryMigration_Idempotent_SecondInitNoError(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("first db.Init: %v", err)
	}

	// Close db.DB between runs (required on Windows).
	if err := db.DB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	if err := db.Init(dir); err != nil {
		t.Errorf("second db.Init: want nil, got %v", err)
	}
}

// TestHandicapHistoryMigration_IdempotencyIndex_EnforcesUniqueConstraint verifies
// that the idx_hc_history_apply_idempotent index actually prevents duplicate
// (apply_request_id, player_id) rows after the migration runs.
func TestHandicapHistoryMigration_IdempotencyIndex_EnforcesUniqueConstraint(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var playerID int64
	if err := db.DB.QueryRow(
		`INSERT INTO players (first_name, last_name, handicap) VALUES ('X','Y',1.0) RETURNING id`,
	).Scan(&playerID); err != nil {
		t.Fatalf("seed player: %v", err)
	}

	const reqID = "550e8400-e29b-41d4-a716-446655440000"

	// First insert must succeed.
	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date, apply_request_id)
		 VALUES (?, 1.0, 2.0, '2026-01-01', ?)`, playerID, reqID,
	); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Second insert with same (apply_request_id, player_id) must fail.
	_, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date, apply_request_id)
		 VALUES (?, 1.0, 3.0, '2026-01-02', ?)`, playerID, reqID,
	)
	if err == nil {
		t.Error("want unique constraint violation for duplicate (apply_request_id, player_id), got nil error")
	}
}
