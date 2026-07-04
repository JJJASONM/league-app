package sqlite_test

import (
	"context"
	"testing"

	"league_app/db"
)

// ── ListSkippedWeeks ──────────────────────────────────────────────────────────

func TestSeasonSkipStore_ListSkippedWeeks_ReturnsWeeks(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	db.DB.Exec(`INSERT INTO skipped_weeks (season_id, skip_date, reason) VALUES (?,?,?)`,
		sid, "2026-07-04", "Holiday")

	weeks, err := store.ListSkippedWeeks(ctx, sid)
	if err != nil {
		t.Fatalf("ListSkippedWeeks: %v", err)
	}
	if len(weeks) != 1 {
		t.Fatalf("want 1 week, got %d", len(weeks))
	}
	if weeks[0].SkipDate != "2026-07-04" {
		t.Errorf("want skip_date=2026-07-04, got %q", weeks[0].SkipDate)
	}
}

func TestSeasonSkipStore_ListSkippedWeeks_EmptyWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	weeks, err := store.ListSkippedWeeks(ctx, sid)
	if err != nil {
		t.Fatalf("ListSkippedWeeks: %v", err)
	}
	if len(weeks) != 0 {
		t.Errorf("want 0 weeks, got %d", len(weeks))
	}
}

// ── CreateSkippedWeek ─────────────────────────────────────────────────────────

func TestSeasonSkipStore_CreateSkippedWeek_InsertsRow(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	sw, err := store.CreateSkippedWeek(ctx, sid, "2026-08-01", "Summer break")
	if err != nil {
		t.Fatalf("CreateSkippedWeek: %v", err)
	}
	if sw.ID == 0 {
		t.Error("want non-zero ID")
	}
	if sw.SkipDate != "2026-08-01" {
		t.Errorf("want skip_date=2026-08-01, got %q", sw.SkipDate)
	}
	if sw.Reason != "Summer break" {
		t.Errorf("want reason='Summer break', got %q", sw.Reason)
	}
}

func TestSeasonSkipStore_CreateSkippedWeek_IdempotentOnDuplicate(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	first, err := store.CreateSkippedWeek(ctx, sid, "2026-09-01", "Labor Day")
	if err != nil {
		t.Fatalf("first CreateSkippedWeek: %v", err)
	}
	second, err := store.CreateSkippedWeek(ctx, sid, "2026-09-01", "Labor Day")
	if err != nil {
		t.Fatalf("second CreateSkippedWeek: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("want same ID on duplicate, got first=%d second=%d", first.ID, second.ID)
	}
}

// ── DeleteSkippedWeek ─────────────────────────────────────────────────────────

func TestSeasonSkipStore_DeleteSkippedWeek_DeletesRow(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	sw, err := store.CreateSkippedWeek(ctx, sid, "2026-10-01", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.DeleteSkippedWeek(ctx, sw.ID); err != nil {
		t.Fatalf("DeleteSkippedWeek: %v", err)
	}
	var n int
	db.DB.QueryRow(`SELECT COUNT(*) FROM skipped_weeks WHERE id=?`, sw.ID).Scan(&n)
	if n != 0 {
		t.Errorf("want 0 rows after delete, got %d", n)
	}
}

func TestSeasonSkipStore_DeleteSkippedWeek_NonExistentNoError(t *testing.T) {
	store := newSeasonStore(t)
	if err := store.DeleteSkippedWeek(context.Background(), 9999); err != nil {
		t.Errorf("want nil error for non-existent id, got %v", err)
	}
}
