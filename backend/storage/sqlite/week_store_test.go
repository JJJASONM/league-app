package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// initWeekDB initialises a fresh in-memory-like temp DB for week store tests.
func initWeekDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
}

// weekStoreSeed creates the minimum rows for week store tests and returns the
// season ID and first match ID. All inserts go directly to db.DB.
func weekStoreSeed(t *testing.T) (seasonID, matchID int64) {
	t.Helper()
	d := db.DB

	r, err := d.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('L','8ball','Monday')`)
	if err != nil {
		t.Fatalf("seed league: %v", err)
	}
	lgID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?,?,?,?,?)`,
		lgID, "S1", "2026-01-01", "single_rr", 3)
	if err != nil {
		t.Fatalf("seed season: %v", err)
	}
	seasonID, _ = r.LastInsertId()

	rA, err := d.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team A')`, lgID)
	if err != nil {
		t.Fatalf("seed team A: %v", err)
	}
	tA, _ := rA.LastInsertId()

	rB, err := d.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team B')`, lgID)
	if err != nil {
		t.Fatalf("seed team B: %v", err)
	}
	tB, _ := rB.LastInsertId()

	rm, err := d.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,1)`,
		seasonID, tA, tB)
	if err != nil {
		t.Fatalf("seed match: %v", err)
	}
	matchID, _ = rm.LastInsertId()
	return
}

func TestWeekStore_WeekMatchCount_ZeroWhenNone(t *testing.T) {
	initWeekDB(t)
	store := sqlite.NewWeekStore(db.DB)

	count, err := store.WeekMatchCount(context.Background(), 99, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0, got %d", count)
	}
}

func TestWeekStore_WeekMatchCount_ReturnsCount(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	count, err := store.WeekMatchCount(context.Background(), seasonID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("want 1, got %d", count)
	}
}

func TestWeekStore_GetWeekStatus_EmptyWhenNoRow(t *testing.T) {
	initWeekDB(t)
	store := sqlite.NewWeekStore(db.DB)

	status, err := store.GetWeekStatus(context.Background(), 99, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "" {
		t.Errorf("want empty string, got %q", status)
	}
}

func TestWeekStore_CloseWeek_UpsertLeagueWeeks(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	if err := store.CloseWeek(context.Background(), seasonID, 1, nil); err != nil {
		t.Fatalf("CloseWeek: %v", err)
	}

	status, err := store.GetWeekStatus(context.Background(), seasonID, 1)
	if err != nil {
		t.Fatalf("GetWeekStatus: %v", err)
	}
	if status != "closed" {
		t.Errorf("want status=closed, got %q", status)
	}
}

func TestWeekStore_CloseWeek_SetsMatchWeekClosed(t *testing.T) {
	initWeekDB(t)
	seasonID, matchID := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	if err := store.CloseWeek(context.Background(), seasonID, 1, nil); err != nil {
		t.Fatalf("CloseWeek: %v", err)
	}

	var wc int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, matchID).Scan(&wc)
	if wc != 1 {
		t.Errorf("want week_closed=1, got %d", wc)
	}
}

func TestWeekStore_CloseWeek_InsertsAckRows(t *testing.T) {
	initWeekDB(t)
	seasonID, matchID := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	acks := []matches.AckEntry{
		{MatchID: matchID, WarningCode: "TEST_WARN", Field: "f1", Notes: "note"},
	}
	if err := store.CloseWeek(context.Background(), seasonID, 1, acks); err != nil {
		t.Fatalf("CloseWeek: %v", err)
	}

	var cnt int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, seasonID).Scan(&cnt)
	if cnt != 1 {
		t.Errorf("want 1 ack row, got %d", cnt)
	}
}

func TestWeekStore_CloseWeek_IdempotentReClose(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	if err := store.CloseWeek(context.Background(), seasonID, 1, nil); err != nil {
		t.Fatalf("first CloseWeek: %v", err)
	}
	if err := store.CloseWeek(context.Background(), seasonID, 1, nil); err != nil {
		t.Fatalf("second CloseWeek (re-close): %v", err)
	}
}

func TestWeekStore_ReopenWeek_ResetsStatus(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	store.CloseWeek(context.Background(), seasonID, 1, nil)
	if err := store.ReopenWeek(context.Background(), seasonID, 1); err != nil {
		t.Fatalf("ReopenWeek: %v", err)
	}

	status, _ := store.GetWeekStatus(context.Background(), seasonID, 1)
	if status != "open" {
		t.Errorf("want status=open after reopen, got %q", status)
	}
}

func TestWeekStore_ReopenWeek_ClearsMatchWeekClosed(t *testing.T) {
	initWeekDB(t)
	seasonID, matchID := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	store.CloseWeek(context.Background(), seasonID, 1, nil)
	store.ReopenWeek(context.Background(), seasonID, 1)

	var wc int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, matchID).Scan(&wc)
	if wc != 0 {
		t.Errorf("want week_closed=0 after reopen, got %d", wc)
	}
}

func TestWeekStore_ListWeekSummaries_EmptyWhenNoMatches(t *testing.T) {
	initWeekDB(t)
	store := sqlite.NewWeekStore(db.DB)

	summaries, err := store.ListWeekSummaries(context.Background(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("want empty slice, got %v", summaries)
	}
}

func TestWeekStore_ListWeekSummaries_StatusOpenByDefault(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	summaries, err := store.ListWeekSummaries(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("want 1 summary, got %d", len(summaries))
	}
	if summaries[0].Status != "open" {
		t.Errorf("want status=open (no league_weeks row), got %q", summaries[0].Status)
	}
	if summaries[0].MatchCount != 1 {
		t.Errorf("want MatchCount=1, got %d", summaries[0].MatchCount)
	}
}

func TestWeekStore_ListWeekSummaries_ClosedAfterClose(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	store.CloseWeek(context.Background(), seasonID, 1, nil)

	summaries, err := store.ListWeekSummaries(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) == 0 || summaries[0].Status != "closed" {
		t.Errorf("want status=closed after CloseWeek, got %v", summaries)
	}
}

func TestWeekStore_ListAcknowledgments_Empty(t *testing.T) {
	initWeekDB(t)
	store := sqlite.NewWeekStore(db.DB)

	acks, err := store.ListAcknowledgments(context.Background(), 99, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(acks) != 0 {
		t.Errorf("want empty, got %v", acks)
	}
}

func TestWeekStore_ListAcknowledgments_ReturnsInsertedRows(t *testing.T) {
	initWeekDB(t)
	seasonID, matchID := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	acks := []matches.AckEntry{
		{MatchID: matchID, WarningCode: "W1", Field: "f1", Notes: "note1"},
		{MatchID: matchID, WarningCode: "W2", Field: "f2", Notes: "note2"},
	}
	store.CloseWeek(context.Background(), seasonID, 1, acks)

	got, err := store.ListAcknowledgments(context.Background(), seasonID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 ack rows, got %d", len(got))
	}
}

func TestWeekStore_GetWeekAdvanceSummary_NoMatches_EmptySummary(t *testing.T) {
	initWeekDB(t)
	store := sqlite.NewWeekStore(db.DB)

	summary, err := store.GetWeekAdvanceSummary(context.Background(), 99, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.ClosedWeek.MatchCount != 0 {
		t.Errorf("want 0 matches, got %d", summary.ClosedWeek.MatchCount)
	}
	if summary.NextWeekNumber != nil {
		t.Error("want nil NextWeekNumber when no matches")
	}
	if summary.NextWeek != nil {
		t.Error("want nil NextWeek when no matches")
	}
}

func TestWeekStore_GetWeekAdvanceSummary_CountsMatchCompletedClosed(t *testing.T) {
	initWeekDB(t)
	seasonID, matchID := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	// Mark match as completed and closed.
	db.DB.Exec(`UPDATE matches SET completed=1, week_closed=1 WHERE id=?`, matchID)

	summary, err := store.GetWeekAdvanceSummary(context.Background(), seasonID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.ClosedWeek.MatchCount != 1 {
		t.Errorf("want MatchCount=1, got %d", summary.ClosedWeek.MatchCount)
	}
	if summary.ClosedWeek.CompletedCount != 1 {
		t.Errorf("want CompletedCount=1, got %d", summary.ClosedWeek.CompletedCount)
	}
	if summary.ClosedWeek.ClosedCount != 1 {
		t.Errorf("want ClosedCount=1, got %d", summary.ClosedWeek.ClosedCount)
	}
}

func TestWeekStore_GetWeekAdvanceSummary_StatusClosedAfterClose(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	store.CloseWeek(context.Background(), seasonID, 1, nil)

	summary, err := store.GetWeekAdvanceSummary(context.Background(), seasonID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.ClosedWeek.Status != "closed" {
		t.Errorf("want status=closed after CloseWeek, got %q", summary.ClosedWeek.Status)
	}
}

func TestWeekStore_GetWeekAdvanceSummary_StatusOpenByDefault(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t)
	store := sqlite.NewWeekStore(db.DB)

	summary, err := store.GetWeekAdvanceSummary(context.Background(), seasonID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.ClosedWeek.Status != "open" {
		t.Errorf("want status=open (no league_weeks row), got %q", summary.ClosedWeek.Status)
	}
}

func TestWeekStore_GetWeekAdvanceSummary_NextWeekDetected(t *testing.T) {
	initWeekDB(t)
	seasonID, _ := weekStoreSeed(t) // seeds week 1

	// Seed a second match in week 2.
	var tA, tB int64
	db.DB.QueryRow(`SELECT home_team_id, away_team_id FROM matches WHERE season_id=?`, seasonID).Scan(&tA, &tB)
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,2)`, seasonID, tA, tB)

	store := sqlite.NewWeekStore(db.DB)
	summary, err := store.GetWeekAdvanceSummary(context.Background(), seasonID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.NextWeekNumber == nil || *summary.NextWeekNumber != 2 {
		t.Errorf("want NextWeekNumber=2, got %v", summary.NextWeekNumber)
	}
	if summary.NextWeek == nil {
		t.Fatal("want non-nil NextWeek")
	}
	if summary.NextWeek.MatchCount != 1 {
		t.Errorf("want NextWeek.MatchCount=1, got %d", summary.NextWeek.MatchCount)
	}
}
