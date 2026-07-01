package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/db"
	"league_app/models"
)

// stubWeekStore is a test double for matches.WeekStore.
type stubWeekStore struct {
	matchCount   int
	weekStatus   string
	closeErr     error
	closeCalled  bool
	reopenErr    error
	reopenCalled bool
	acks         []models.CloseAck
	acksErr      error
}

func (s *stubWeekStore) ListWeekSummaries(_ context.Context, _ int64) ([]models.WeekSummary, error) {
	return nil, nil
}

func (s *stubWeekStore) WeekMatchCount(_ context.Context, _, _ int64) (int, error) {
	return s.matchCount, nil
}

func (s *stubWeekStore) GetWeekStatus(_ context.Context, _, _ int64) (string, error) {
	return s.weekStatus, nil
}

func (s *stubWeekStore) CloseWeek(_ context.Context, _, _ int64, _ []matches.AckEntry) error {
	s.closeCalled = true
	return s.closeErr
}

func (s *stubWeekStore) ReopenWeek(_ context.Context, _, _ int64) error {
	s.reopenCalled = true
	return s.reopenErr
}

func (s *stubWeekStore) ListAcknowledgments(_ context.Context, _, _ int64) ([]models.CloseAck, error) {
	return s.acks, s.acksErr
}

// newTestSvc creates a WeekService backed by the stub store and a fresh empty DB.
func newTestSvc(t *testing.T, store *stubWeekStore) *matches.WeekService {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return matches.NewWeekService(store, db.DB)
}

// --- ReopenWeek ---

func TestWeekService_ReopenWeek_NotFoundWhenNoMatches(t *testing.T) {
	store := &stubWeekStore{matchCount: 0}
	svc := newTestSvc(t, store)

	err := svc.ReopenWeek(context.Background(), 1, 1)

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want *domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.NotFound {
		t.Errorf("want NotFound, got category %v", de.Category)
	}
	if store.reopenCalled {
		t.Error("store.ReopenWeek must not be called when no matches exist")
	}
}

func TestWeekService_ReopenWeek_ConflictWhenNotClosed(t *testing.T) {
	store := &stubWeekStore{matchCount: 1, weekStatus: "open"}
	svc := newTestSvc(t, store)

	err := svc.ReopenWeek(context.Background(), 1, 1)

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want *domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Conflict {
		t.Errorf("want Conflict, got category %v", de.Category)
	}
	if store.reopenCalled {
		t.Error("store.ReopenWeek must not be called when week is not closed")
	}
}

func TestWeekService_ReopenWeek_ConflictWhenNoLeagueWeeksRow(t *testing.T) {
	// status="" (no league_weeks row) means implicitly open → Conflict.
	store := &stubWeekStore{matchCount: 1, weekStatus: ""}
	svc := newTestSvc(t, store)

	err := svc.ReopenWeek(context.Background(), 1, 1)

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.Conflict {
		t.Errorf("want Conflict domainerr, got %v", err)
	}
}

func TestWeekService_ReopenWeek_SuccessCallsStore(t *testing.T) {
	store := &stubWeekStore{matchCount: 1, weekStatus: "closed"}
	svc := newTestSvc(t, store)

	err := svc.ReopenWeek(context.Background(), 1, 1)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.reopenCalled {
		t.Error("store.ReopenWeek was not called on success path")
	}
}

// --- ListAcknowledgments ---

func TestWeekService_ListAcknowledgments_NotFoundWhenNoMatches(t *testing.T) {
	store := &stubWeekStore{matchCount: 0}
	svc := newTestSvc(t, store)

	_, err := svc.ListAcknowledgments(context.Background(), 1, 1)

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Errorf("want NotFound domainerr, got %v", err)
	}
}

func TestWeekService_ListAcknowledgments_ReturnsEmptySlice(t *testing.T) {
	store := &stubWeekStore{matchCount: 1, acks: nil}
	svc := newTestSvc(t, store)

	acks, err := svc.ListAcknowledgments(context.Background(), 1, 1)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(acks) != 0 {
		t.Errorf("want empty slice, got %v", acks)
	}
}

// --- CloseWeek ---

// With an empty DB, ValidateWeek finds no matches → no errors, no warnings.
// CloseWeek should proceed directly to store.CloseWeek.
func TestWeekService_CloseWeek_SuccessOnEmptyWeek(t *testing.T) {
	store := &stubWeekStore{}
	svc := newTestSvc(t, store)

	result, err := svc.CloseWeek(context.Background(), matches.CloseWeekRequest{
		SeasonID:   1,
		WeekNumber: 1,
		Cfg:        matches.RoundConfig{Multiplier: 2.55},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.closeCalled {
		t.Error("store.CloseWeek was not called on success path")
	}
	if result.AckCount != 0 {
		t.Errorf("want AckCount=0 (no warnings), got %d", result.AckCount)
	}
}

func TestWeekService_CloseWeek_PropagatesStoreError(t *testing.T) {
	store := &stubWeekStore{closeErr: errors.New("db failure")}
	svc := newTestSvc(t, store)

	_, err := svc.CloseWeek(context.Background(), matches.CloseWeekRequest{
		SeasonID:   1,
		WeekNumber: 1,
		Cfg:        matches.RoundConfig{Multiplier: 2.55},
	})

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var wce *matches.WeekCloseErr
	if errors.As(err, &wce) {
		t.Error("want a wrapped store error, not a WeekCloseErr (that is for validation failures)")
	}
}
