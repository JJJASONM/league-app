package db_test

import (
	"database/sql"
	"testing"

	"league_app/db"
)

// preScoreApprovalMatchesSchema is the matches table shape after Weekly
// Score Processing Phase 1A/1B (approved_at/processed_at etc. already
// present) but before Player Score Approval Phase 1A's score_revision and
// per-side approval columns. Used to simulate an existing database that
// needs upgrading.
const preScoreApprovalMatchesSchema = `
CREATE TABLE IF NOT EXISTS matches (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id            INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    home_team_id         INTEGER REFERENCES teams(id),
    away_team_id         INTEGER REFERENCES teams(id),
    match_date           DATE,
    week_number          INTEGER NOT NULL DEFAULT 1,
    match_number         INTEGER,
    table_numbers        TEXT,
    completed            INTEGER NOT NULL DEFAULT 0,
    created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
    week_closed          INTEGER NOT NULL DEFAULT 0,
    approved_at          DATETIME,
    approved_by_user_id  INTEGER,
    approval_note        TEXT NOT NULL DEFAULT '',
    processed_at         DATETIME,
    processed_by_user_id INTEGER
)`

// scoreApprovalMatchColumns lists every column Player Score Approval
// Phase 1A adds to matches.
var scoreApprovalMatchColumns = []string{
	"score_revision",
	"home_approval_state",
	"home_approval_state_at",
	"home_approval_actor_user_id",
	"home_approval_actor_player_id",
	"home_approval_actor_name_snapshot",
	"home_approval_state_score_revision",
	"home_correction_note",
	"away_approval_state",
	"away_approval_state_at",
	"away_approval_actor_user_id",
	"away_approval_actor_player_id",
	"away_approval_actor_name_snapshot",
	"away_approval_state_score_revision",
	"away_correction_note",
}

// TestMatchApprovalMigration_FreshDB_AllColumnsPresent verifies that a fresh
// database (created by db.Init from scratch) already has every Player Score
// Approval Phase 1A column, all defaulting correctly, plus the
// match_approval_events table.
func TestMatchApprovalMigration_FreshDB_AllColumnsPresent(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	for _, col := range scoreApprovalMatchColumns {
		if !hasColumn(t, "matches", col) {
			t.Errorf("fresh DB: missing column matches.%s", col)
		}
	}

	var count int
	if err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='match_approval_events'`,
	).Scan(&count); err != nil {
		t.Fatalf("check match_approval_events exists: %v", err)
	}
	if count != 1 {
		t.Error("fresh DB: missing table match_approval_events")
	}
}

// TestMatchApprovalMigration_NewMatch_DefaultsToZeroRevisionBothPending
// verifies a freshly inserted match starts at score_revision=0 with both
// sides "pending" and every actor/note field empty -- the schema's own
// defaults, not something a service layer has to set explicitly.
func TestMatchApprovalMigration_NewMatch_DefaultsToZeroRevisionBothPending(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var leagueID, seasonID, matchID int64
	if err := db.DB.QueryRow(`INSERT INTO leagues (name) VALUES ('L') RETURNING id`).Scan(&leagueID); err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := db.DB.QueryRow(`INSERT INTO seasons (league_id, name) VALUES (?, 'S') RETURNING id`, leagueID).Scan(&seasonID); err != nil {
		t.Fatalf("seed season: %v", err)
	}
	if err := db.DB.QueryRow(`INSERT INTO matches (season_id) VALUES (?) RETURNING id`, seasonID).Scan(&matchID); err != nil {
		t.Fatalf("seed match: %v", err)
	}

	var rev int
	var homeState, awayState, homeNote, awayNote, homeActorName, awayActorName string
	if err := db.DB.QueryRow(`
		SELECT score_revision, home_approval_state, away_approval_state,
		       home_correction_note, away_correction_note,
		       home_approval_actor_name_snapshot, away_approval_actor_name_snapshot
		FROM matches WHERE id = ?`, matchID).
		Scan(&rev, &homeState, &awayState, &homeNote, &awayNote, &homeActorName, &awayActorName); err != nil {
		t.Fatalf("read defaults: %v", err)
	}
	if rev != 0 {
		t.Errorf("score_revision: want 0, got %d", rev)
	}
	if homeState != "pending" || awayState != "pending" {
		t.Errorf("want both sides pending, got home=%s away=%s", homeState, awayState)
	}
	if homeNote != "" || awayNote != "" || homeActorName != "" || awayActorName != "" {
		t.Error("want empty note/actor-name defaults")
	}
}

// TestMatchApprovalMigration_CheckConstraint_RejectsInvalidState verifies
// the schema itself -- not just the domain/service layer -- rejects an
// approval_state value outside pending/approved/correction_requested.
func TestMatchApprovalMigration_CheckConstraint_RejectsInvalidState(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var leagueID, seasonID, matchID int64
	db.DB.QueryRow(`INSERT INTO leagues (name) VALUES ('L') RETURNING id`).Scan(&leagueID)
	db.DB.QueryRow(`INSERT INTO seasons (league_id, name) VALUES (?, 'S') RETURNING id`, leagueID).Scan(&seasonID)
	db.DB.QueryRow(`INSERT INTO matches (season_id) VALUES (?) RETURNING id`, seasonID).Scan(&matchID)

	if _, err := db.DB.Exec(`UPDATE matches SET home_approval_state = 'bogus' WHERE id = ?`, matchID); err == nil {
		t.Error("want CHECK constraint to reject an invalid home_approval_state, got nil error")
	}
	if _, err := db.DB.Exec(`UPDATE matches SET away_approval_state = 'bogus' WHERE id = ?`, matchID); err == nil {
		t.Error("want CHECK constraint to reject an invalid away_approval_state, got nil error")
	}

	// The three real values must all be accepted.
	for _, v := range []string{"pending", "approved", "correction_requested"} {
		if _, err := db.DB.Exec(`UPDATE matches SET home_approval_state = ? WHERE id = ?`, v, matchID); err != nil {
			t.Errorf("want %q accepted, got error: %v", v, err)
		}
	}
}

// TestMatchApprovalMigration_PreExistingDB_UpgradesWithoutDataLoss simulates
// an existing database from before Player Score Approval Phase 1A (Weekly
// Score Processing Phase 1A/1B columns already present, but none of this
// phase's columns or the match_approval_events table). It verifies
// re-running Init adds everything without disturbing an existing match row.
func TestMatchApprovalMigration_PreExistingDB_UpgradesWithoutDataLoss(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("first db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var leagueID, seasonID int64
	if err := db.DB.QueryRow(`INSERT INTO leagues (name) VALUES ('L') RETURNING id`).Scan(&leagueID); err != nil {
		t.Fatalf("seed league: %v", err)
	}
	if err := db.DB.QueryRow(`INSERT INTO seasons (league_id, name) VALUES (?, 'S') RETURNING id`, leagueID).Scan(&seasonID); err != nil {
		t.Fatalf("seed season: %v", err)
	}

	// Rebuild matches without this phase's columns (round_results/
	// match_results/lineup_plans reference matches by id but are left
	// alone here -- SQLite's DROP TABLE does not require an empty table
	// and this test seeds no child rows, so there is nothing to disturb).
	if _, err := db.DB.Exec(`DROP TABLE match_approval_events`); err != nil {
		t.Fatalf("drop match_approval_events: %v", err)
	}
	if _, err := db.DB.Exec(`DROP TABLE matches`); err != nil {
		t.Fatalf("drop matches: %v", err)
	}
	if _, err := db.DB.Exec(preScoreApprovalMatchesSchema); err != nil {
		t.Fatalf("recreate pre-phase-1A matches table: %v", err)
	}

	var matchID int64
	if err := db.DB.QueryRow(
		`INSERT INTO matches (season_id, week_number, completed) VALUES (?, 3, 1) RETURNING id`,
		seasonID,
	).Scan(&matchID); err != nil {
		t.Fatalf("seed legacy match row: %v", err)
	}

	if err := db.DB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := db.Init(dir); err != nil {
		t.Fatalf("second db.Init: %v", err)
	}

	for _, col := range scoreApprovalMatchColumns {
		if !hasColumn(t, "matches", col) {
			t.Errorf("after upgrade: missing column matches.%s", col)
		}
	}
	var count int
	if err := db.DB.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='match_approval_events'`,
	).Scan(&count); err != nil {
		t.Fatalf("check match_approval_events exists: %v", err)
	}
	if count != 1 {
		t.Error("after upgrade: missing table match_approval_events")
	}

	// The legacy row must survive with its own data intact, and the new
	// columns must have picked up their defaults for that pre-existing row.
	var weekNumber, completed, rev int
	var homeState string
	if err := db.DB.QueryRow(
		`SELECT week_number, completed, score_revision, home_approval_state FROM matches WHERE id = ?`, matchID,
	).Scan(&weekNumber, &completed, &rev, &homeState); err != nil {
		t.Fatalf("read upgraded row: %v", err)
	}
	if weekNumber != 3 {
		t.Errorf("week_number: want 3 (preserved), got %d", weekNumber)
	}
	if completed != 1 {
		t.Errorf("completed: want 1 (preserved), got %d", completed)
	}
	if rev != 0 {
		t.Errorf("score_revision: want 0 (backfilled default), got %d", rev)
	}
	if homeState != "pending" {
		t.Errorf("home_approval_state: want pending (backfilled default), got %s", homeState)
	}
}

// TestMatchApprovalMigration_Idempotent_SecondInitNoError verifies that
// running db.Init twice on the same database directory does not return an
// error once every Player Score Approval Phase 1A column and table exist.
func TestMatchApprovalMigration_Idempotent_SecondInitNoError(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("first db.Init: %v", err)
	}
	if err := db.DB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	if err := db.Init(dir); err != nil {
		t.Errorf("second db.Init: want nil, got %v", err)
	}
}

// TestMatchApprovalEvents_CascadeOnMatchDelete verifies match_approval_events
// rows are removed when their match is deleted (ON DELETE CASCADE), so no
// orphaned history can accumulate.
func TestMatchApprovalEvents_CascadeOnMatchDelete(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var leagueID, seasonID, matchID int64
	db.DB.QueryRow(`INSERT INTO leagues (name) VALUES ('L') RETURNING id`).Scan(&leagueID)
	db.DB.QueryRow(`INSERT INTO seasons (league_id, name) VALUES (?, 'S') RETURNING id`, leagueID).Scan(&seasonID)
	db.DB.QueryRow(`INSERT INTO matches (season_id) VALUES (?) RETURNING id`, seasonID).Scan(&matchID)

	if _, err := db.DB.Exec(
		`INSERT INTO match_approval_events (match_id, event_scope, event_type, score_revision) VALUES (?, 'home', 'approved', 0)`,
		matchID); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if _, err := db.DB.Exec(`DELETE FROM matches WHERE id = ?`, matchID); err != nil {
		t.Fatalf("delete match: %v", err)
	}
	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM match_approval_events WHERE match_id = ?`, matchID).Scan(&count)
	if count != 0 {
		t.Errorf("want 0 events after cascading match delete, got %d", count)
	}
}

// TestMatchApprovalActorPlayerColumns_NoForeignKey_DeletingPlayerNeverBlocked
// verifies matches.home_approval_actor_player_id / away_approval_actor_player_id
// carry no foreign key -- a player currently holding an active team approval
// must never block a raw player delete, matching this schema's established
// attribution convention.
func TestMatchApprovalActorPlayerColumns_NoForeignKey_DeletingPlayerNeverBlocked(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var leagueID, seasonID, playerID, matchID int64
	db.DB.QueryRow(`INSERT INTO leagues (name) VALUES ('L') RETURNING id`).Scan(&leagueID)
	db.DB.QueryRow(`INSERT INTO seasons (league_id, name) VALUES (?, 'S') RETURNING id`, leagueID).Scan(&seasonID)
	db.DB.QueryRow(`INSERT INTO players (first_name, last_name) VALUES ('A','B') RETURNING id`).Scan(&playerID)
	db.DB.QueryRow(`INSERT INTO matches (season_id) VALUES (?) RETURNING id`, seasonID).Scan(&matchID)

	if _, err := db.DB.Exec(
		`UPDATE matches SET home_approval_actor_player_id = ? WHERE id = ?`, playerID, matchID,
	); err != nil {
		t.Fatalf("set actor player id: %v", err)
	}

	if _, err := db.DB.Exec(`DELETE FROM players WHERE id = ?`, playerID); err != nil {
		t.Fatalf("want player delete to succeed with no foreign key on the actor column, got: %v", err)
	}

	var stillThere sql.NullInt64
	if err := db.DB.QueryRow(
		`SELECT home_approval_actor_player_id FROM matches WHERE id = ?`, matchID,
	).Scan(&stillThere); err != nil {
		t.Fatalf("read match after player delete: %v", err)
	}
	if !stillThere.Valid {
		t.Error("want the dangling actor id left as-is (no ON DELETE SET NULL either -- merge is the only repoint path)")
	}
}
