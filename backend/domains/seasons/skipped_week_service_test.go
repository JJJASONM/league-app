package seasons_test

import (
	"context"
	"errors"
	"testing"

	"league_app/models"
)

// ── ListSkippedWeeks ──────────────────────────────────────────────────────────

func TestSeasonService_ListSkippedWeeks_ReturnsWeeks(t *testing.T) {
	want := []models.SkippedWeek{{ID: 1, SkipDate: "2026-07-04"}}
	store := &stubSeasonStore{skippedWeeks: want}
	got, err := newSvc(store).ListSkippedWeeks(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListSkippedWeeks: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("want 1 week ID=1, got %v", got)
	}
}

func TestSeasonService_ListSkippedWeeks_ReturnsEmptySliceWhenNone(t *testing.T) {
	store := &stubSeasonStore{skippedWeeks: nil}
	got, err := newSvc(store).ListSkippedWeeks(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListSkippedWeeks: %v", err)
	}
	if got == nil {
		t.Error("want empty slice, got nil")
	}
}

func TestSeasonService_ListSkippedWeeks_StoreErrorPropagates(t *testing.T) {
	store := &stubSeasonStore{skippedWeeksErr: errors.New("db down")}
	_, err := newSvc(store).ListSkippedWeeks(context.Background(), 1)
	if err == nil {
		t.Error("want error, got nil")
	}
}

// ── CreateSkippedWeek ─────────────────────────────────────────────────────────

func TestSeasonService_CreateSkippedWeek_DelegatesToStore(t *testing.T) {
	want := models.SkippedWeek{ID: 5, SkipDate: "2026-07-04", Reason: "Holiday"}
	store := &stubSeasonStore{createdSkipWeek: want}
	got, err := newSvc(store).CreateSkippedWeek(context.Background(), 1, "2026-07-04", "Holiday")
	if err != nil {
		t.Fatalf("CreateSkippedWeek: %v", err)
	}
	if got.ID != want.ID || got.SkipDate != want.SkipDate {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestSeasonService_CreateSkippedWeek_StoreErrorPropagates(t *testing.T) {
	store := &stubSeasonStore{createSkipErr: errors.New("db down")}
	_, err := newSvc(store).CreateSkippedWeek(context.Background(), 1, "2026-07-04", "")
	if err == nil {
		t.Error("want error, got nil")
	}
}

// ── DeleteSkippedWeek ─────────────────────────────────────────────────────────

func TestSeasonService_DeleteSkippedWeek_DelegatesToStore(t *testing.T) {
	store := &stubSeasonStore{}
	if err := newSvc(store).DeleteSkippedWeek(context.Background(), 1); err != nil {
		t.Fatalf("DeleteSkippedWeek: %v", err)
	}
}

func TestSeasonService_DeleteSkippedWeek_StoreErrorPropagates(t *testing.T) {
	store := &stubSeasonStore{deleteSkipErr: errors.New("db down")}
	if err := newSvc(store).DeleteSkippedWeek(context.Background(), 1); err == nil {
		t.Error("want error, got nil")
	}
}
