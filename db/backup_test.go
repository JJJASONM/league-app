package db_test

import (
	"database/sql"
	"testing"

	"league_app/db"
)

// TestBackup_CheckspointsWAL_CapturesUncommittedWALData verifies that Backup
// checkpoints the WAL before copying league.db. Without the checkpoint, a
// row written just before Backup is called can still be sitting only in
// league.db-wal (not yet auto-checkpointed into league.db by SQLite's
// default 1000-page threshold), so a plain file copy of league.db alone
// would silently omit it from the backup.
func TestBackup_CheckspointsWAL_CapturesUncommittedWALData(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var leagueID int64
	if err := db.DB.QueryRow(
		`INSERT INTO leagues (name) VALUES ('WAL Backup Test') RETURNING id`,
	).Scan(&leagueID); err != nil {
		t.Fatalf("seed league: %v", err)
	}

	backupPath, err := db.Backup(dir)
	if err != nil {
		t.Fatalf("db.Backup: %v", err)
	}

	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatalf("open backup db: %v", err)
	}
	defer backupDB.Close()

	var gotName string
	if err := backupDB.QueryRow(
		`SELECT name FROM leagues WHERE id = ?`, leagueID,
	).Scan(&gotName); err != nil {
		t.Fatalf("read league from backup (WAL data may not have been checkpointed): %v", err)
	}
	if gotName != "WAL Backup Test" {
		t.Errorf("want league name %q in backup, got %q", "WAL Backup Test", gotName)
	}
}
