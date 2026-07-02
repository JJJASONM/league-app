package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// stubWeekStore is a test double for matches.WeekStore.
type stubWeekStore struct {
	matchCount      int
	weekStatus      string
	closeErr        error
	closeCalled     bool
	reopenErr       error
	reopenCalled    bool
	acks            []models.CloseAck
	acksErr         error
	advanceSummary  matches.WeekAdvanceSummary
	advanceSummaryErr error
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

func (s *stubWeekStore) GetWeekAdvanceSummary(_ context.Context, _, _ int64) (matches.WeekAdvanceSummary, error) {
	return s.advanceSummary, s.advanceSummaryErr
}

// stubHandicapPreviewer is a test double for matches.HandicapPreviewer.
type stubHandicapPreviewer struct {
	result models.AdvancePreviewHandicap
	err    error
}

func (s *stubHandicapPreviewer) HandicapPreview(_ context.Context, _ int64) (models.AdvancePreviewHandicap, error) {
	return s.result, s.err
}

func (s *stubWeekStore) SeasonRoundConfig(_ context.Context, _ int64) (matches.RoundConfig, error) {
	return matches.RoundConfig{Multiplier: 2.55}, nil
}

func (s *stubWeekStore) GetWeekValidationData(_ context.Context, _, _ int64) (matches.WeekValidationData, error) {
	return matches.WeekValidationData{}, nil
}

// newTestSvc creates a WeekService backed by the stub store.
// previewer is optional; pass nil to test the nil-safe path.
func newTestSvc(t *testing.T, store *stubWeekStore, previewer matches.HandicapPreviewer) *matches.WeekService {
	t.Helper()
	return matches.NewWeekService(store, previewer)
}

// --- ReopenWeek ---

func TestWeekService_ReopenWeek_NotFoundWhenNoMatches(t *testing.T) {
	store := &stubWeekStore{matchCount: 0}
	svc := newTestSvc(t, store, nil)

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
	svc := newTestSvc(t, store, nil)

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
	svc := newTestSvc(t, store, nil)

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
	svc := newTestSvc(t, store, nil)

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
	svc := newTestSvc(t, store, nil)

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
	svc := newTestSvc(t, store, nil)

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
	svc := newTestSvc(t, store, nil)

	result, err := svc.CloseWeek(context.Background(), matches.CloseWeekRequest{
		SeasonID:   1,
		WeekNumber: 1,
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
	svc := newTestSvc(t, store, nil)

	_, err := svc.CloseWeek(context.Background(), matches.CloseWeekRequest{
		SeasonID:   1,
		WeekNumber: 1,
	})

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var wce *matches.WeekCloseErr
	if errors.As(err, &wce) {
		t.Error("want a wrapped store error, not a WeekCloseErr (that is for validation failures)")
	}
}

// --- AdvanceData ---

func TestWeekService_AdvanceData_DelegatesMatchCounts(t *testing.T) {
	nw := 2
	store := &stubWeekStore{
		advanceSummary: matches.WeekAdvanceSummary{
			ClosedWeek: models.AdvancePreviewWeekSummary{
				MatchCount: 3, CompletedCount: 3, ClosedCount: 3, Status: "closed",
			},
			NextWeekNumber: &nw,
			NextWeek: &models.AdvancePreviewNextWeek{
				MatchCount: 2, AssignedCount: 2, UnassignedCount: 0,
				MissingLineupTeamIDs: []int64{},
			},
		},
	}
	svc := newTestSvc(t, store, nil)

	ar, err := svc.AdvanceData(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ar.ClosedWeek.MatchCount != 3 {
		t.Errorf("want MatchCount=3, got %d", ar.ClosedWeek.MatchCount)
	}
	if ar.NextWeekNumber == nil || *ar.NextWeekNumber != 2 {
		t.Errorf("want NextWeekNumber=2, got %v", ar.NextWeekNumber)
	}
}

func TestWeekService_AdvanceData_NilPreviewer_EmptyHandicap(t *testing.T) {
	store := &stubWeekStore{}
	svc := newTestSvc(t, store, nil)

	ar, err := svc.AdvanceData(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ar.Handicap.Method != "" || ar.Handicap.Status != "" {
		t.Errorf("want empty Handicap when previewer is nil, got %+v", ar.Handicap)
	}
}

func TestWeekService_AdvanceData_WithPreviewer_PopulatesHandicap(t *testing.T) {
	store := &stubWeekStore{}
	previewer := &stubHandicapPreviewer{
		result: models.AdvancePreviewHandicap{Method: "manual_review", Status: "no_auto_apply"},
	}
	svc := newTestSvc(t, store, previewer)

	ar, err := svc.AdvanceData(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ar.Handicap.Method != "manual_review" {
		t.Errorf("want Handicap.Method=manual_review, got %q", ar.Handicap.Method)
	}
}

func TestWeekService_AdvanceData_StoreSummaryError_Propagates(t *testing.T) {
	store := &stubWeekStore{advanceSummaryErr: errors.New("store failure")}
	svc := newTestSvc(t, store, nil)

	_, err := svc.AdvanceData(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

// --- AdvancePreview ---

func TestWeekService_AdvancePreview_NotFoundWhenNoMatches(t *testing.T) {
	store := &stubWeekStore{matchCount: 0}
	svc := newTestSvc(t, store, nil)

	_, err := svc.AdvancePreview(context.Background(), 1, 1)

	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Errorf("want NotFound domainerr, got %v", err)
	}
}

func TestWeekService_AdvancePreview_CanClose_WhenNoValidationErrors(t *testing.T) {
	// matchCount=1 passes the existence check; empty DB → ValidateWeek finds no rounds → no errors/warnings.
	store := &stubWeekStore{matchCount: 1}
	svc := newTestSvc(t, store, nil)

	preview, err := svc.AdvancePreview(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !preview.CanClose {
		t.Error("want CanClose=true when no validation errors")
	}
}

func TestWeekService_AdvancePreview_PopulatesSeasonAndWeek(t *testing.T) {
	store := &stubWeekStore{matchCount: 1}
	svc := newTestSvc(t, store, nil)

	preview, err := svc.AdvancePreview(context.Background(), 7, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.SeasonID != 7 {
		t.Errorf("want SeasonID=7, got %d", preview.SeasonID)
	}
	if preview.WeekNumber != 3 {
		t.Errorf("want WeekNumber=3, got %d", preview.WeekNumber)
	}
}
