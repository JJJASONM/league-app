package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
)

// --- stub ---

type stubPushbackStore struct {
	exists       bool
	existsErr    error
	closed       bool
	closedErr    error
	rows         []matches.PushbackMatchRow
	rowsErr      error
	applyErr     error
	appliedInput *matches.PushbackApplyInput
}

func (s *stubPushbackStore) SeasonExists(_ context.Context, _ int64) (bool, error) {
	return s.exists, s.existsErr
}
func (s *stubPushbackStore) HasClosedWeeksAtOrAfter(_ context.Context, _ int64, _ int) (bool, error) {
	return s.closed, s.closedErr
}
func (s *stubPushbackStore) GetPushbackMatches(_ context.Context, _ int64) ([]matches.PushbackMatchRow, error) {
	return s.rows, s.rowsErr
}
func (s *stubPushbackStore) ApplyPushback(_ context.Context, input matches.PushbackApplyInput) error {
	s.appliedInput = &input
	return s.applyErr
}

func strPtr(s string) *string { return &s }

func newPushbackSvc(store *stubPushbackStore) *matches.PushbackService {
	return matches.NewPushbackService(store)
}

func baseStore() *stubPushbackStore {
	return &stubPushbackStore{exists: true, closed: false}
}

// --- validation ---

func TestPushbackPreview_InvalidCutoff_ReturnsError(t *testing.T) {
	svc := newPushbackSvc(baseStore())
	_, err := svc.Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 0, WeeksToAdd: 1,
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Fatalf("want InvalidInput, got %v", err)
	}
	if code(err) != "PUSHBACK_INVALID_CUTOFF" {
		t.Errorf("want PUSHBACK_INVALID_CUTOFF, got %q", code(err))
	}
}

func TestPushbackPreview_InvalidWeeksToAdd_ReturnsError(t *testing.T) {
	svc := newPushbackSvc(baseStore())
	_, err := svc.Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 3, WeeksToAdd: 0,
	})
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Fatalf("want InvalidInput, got %v", err)
	}
	if code(err) != "PUSHBACK_INVALID_WEEKS_TO_ADD" {
		t.Errorf("want PUSHBACK_INVALID_WEEKS_TO_ADD, got %q", code(err))
	}
}

func TestPushbackPreview_SeasonNotFound_ReturnsNotFound(t *testing.T) {
	store := &stubPushbackStore{exists: false}
	svc := newPushbackSvc(store)
	_, err := svc.Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 99, CutoffWeek: 1, WeeksToAdd: 1,
	})
	if !domainerr.IsCategory(err, domainerr.NotFound) {
		t.Fatalf("want NotFound, got %v", err)
	}
}

// --- closed-week guard ---

func TestPushbackPreview_ClosedWeekAtCutoff_ReturnsConflict(t *testing.T) {
	store := &stubPushbackStore{exists: true, closed: true}
	svc := newPushbackSvc(store)
	_, err := svc.Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if !domainerr.IsCategory(err, domainerr.Conflict) {
		t.Fatalf("want Conflict, got %v", err)
	}
	if code(err) != "PUSHBACK_HAS_CLOSED_WEEKS" {
		t.Errorf("want PUSHBACK_HAS_CLOSED_WEEKS, got %q", code(err))
	}
}

func TestPushbackPreview_ClosedWeekBeforeCutoff_DoesNotBlock(t *testing.T) {
	// closed=false means the store says no closed weeks at/after cutoff
	store := &stubPushbackStore{exists: true, closed: false}
	svc := newPushbackSvc(store)
	_, err := svc.Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("want no error when closed weeks only before cutoff, got %v", err)
	}
}

// --- partitioning ---

func TestPushbackPreview_CompletedMatchAtCutoff_IsPreserved(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 1, WeekNumber: 5, MatchDate: strPtr("2026-10-06"), Completed: true, HomeTeamID: 1, AwayTeamID: 2},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Preserved) != 1 {
		t.Fatalf("want 1 preserved, got %d", len(res.Preserved))
	}
	if res.Preserved[0].ID != 1 {
		t.Errorf("want preserved ID=1, got %d", res.Preserved[0].ID)
	}
	if len(res.Shifted) != 0 {
		t.Errorf("want no shifted matches, got %d", len(res.Shifted))
	}
}

func TestPushbackPreview_UnplayedMatchAtCutoff_IsShifted(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 2, WeekNumber: 5, MatchDate: strPtr("2026-10-06"), Completed: false, HomeTeamID: 3, AwayTeamID: 4},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Shifted) != 1 {
		t.Fatalf("want 1 shifted, got %d", len(res.Shifted))
	}
	sm := res.Shifted[0]
	if sm.ID != 2 {
		t.Errorf("want shifted ID=2, got %d", sm.ID)
	}
	if sm.WeekNumber != 5 {
		t.Errorf("want WeekNumber=5, got %d", sm.WeekNumber)
	}
	if sm.NewWeekNumber != 6 {
		t.Errorf("want NewWeekNumber=6, got %d", sm.NewWeekNumber)
	}
	if sm.MatchDate == nil || *sm.MatchDate != "2026-10-06" {
		t.Errorf("want MatchDate=2026-10-06, got %v", sm.MatchDate)
	}
	if sm.NewMatchDate == nil || *sm.NewMatchDate != "2026-10-13" {
		t.Errorf("want NewMatchDate=2026-10-13, got %v", sm.NewMatchDate)
	}
}

func TestPushbackPreview_UnplayedMatchAfterCutoff_IsShifted(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 3, WeekNumber: 7, MatchDate: strPtr("2026-10-20"), Completed: false, HomeTeamID: 1, AwayTeamID: 2},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Shifted) != 1 {
		t.Fatalf("want 1 shifted, got %d", len(res.Shifted))
	}
	if res.Shifted[0].NewWeekNumber != 9 {
		t.Errorf("want NewWeekNumber=9, got %d", res.Shifted[0].NewWeekNumber)
	}
	if res.Shifted[0].NewMatchDate == nil || *res.Shifted[0].NewMatchDate != "2026-11-03" {
		t.Errorf("want NewMatchDate=2026-11-03, got %v", res.Shifted[0].NewMatchDate)
	}
}

func TestPushbackPreview_UnplayedMatchBeforeCutoff_IsOmitted(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 4, WeekNumber: 3, MatchDate: strPtr("2026-09-22"), Completed: false, HomeTeamID: 1, AwayTeamID: 2},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Shifted) != 0 {
		t.Errorf("want 0 shifted (before cutoff omitted), got %d", len(res.Shifted))
	}
	if len(res.Preserved) != 0 {
		t.Errorf("want 0 preserved (before cutoff omitted), got %d", len(res.Preserved))
	}
}

func TestPushbackPreview_UndatedMatch_ShiftsWeekNumberOnly(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 5, WeekNumber: 5, MatchDate: nil, Completed: false, HomeTeamID: 1, AwayTeamID: 2},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Shifted) != 1 {
		t.Fatalf("want 1 shifted, got %d", len(res.Shifted))
	}
	sm := res.Shifted[0]
	if sm.NewWeekNumber != 6 {
		t.Errorf("want NewWeekNumber=6, got %d", sm.NewWeekNumber)
	}
	if sm.MatchDate != nil {
		t.Errorf("want MatchDate nil for undated match, got %v", sm.MatchDate)
	}
	if sm.NewMatchDate != nil {
		t.Errorf("want NewMatchDate nil for undated match, got %v", sm.NewMatchDate)
	}
}

// --- new_end_date ---

func TestPushbackPreview_NewEndDateReflectsShiftedMatches(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 1, WeekNumber: 5, MatchDate: strPtr("2026-10-06"), Completed: false, HomeTeamID: 1, AwayTeamID: 2},
		{ID: 2, WeekNumber: 6, MatchDate: strPtr("2026-10-13"), Completed: false, HomeTeamID: 3, AwayTeamID: 4},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.NewEndDate == nil {
		t.Fatal("want NewEndDate set, got nil")
	}
	// Last match was 2026-10-13, shifted by 1 week = 2026-10-20.
	if *res.NewEndDate != "2026-10-20" {
		t.Errorf("want NewEndDate=2026-10-20, got %q", *res.NewEndDate)
	}
}

func TestPushbackPreview_NewEndDateIncludesPreservedCompleted(t *testing.T) {
	// A completed match at week 6 (date 2026-10-13) is later than any shifted date.
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 1, WeekNumber: 5, MatchDate: strPtr("2026-10-06"), Completed: false, HomeTeamID: 1, AwayTeamID: 2},
		{ID: 2, WeekNumber: 6, MatchDate: strPtr("2026-10-13"), Completed: true, HomeTeamID: 3, AwayTeamID: 4},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// shifted match: 2026-10-06 + 7 days = 2026-10-13. Preserved: 2026-10-13. Max = 2026-10-13.
	if res.NewEndDate == nil || *res.NewEndDate != "2026-10-13" {
		t.Errorf("want NewEndDate=2026-10-13, got %v", res.NewEndDate)
	}
}

// --- empty results ---

func TestPushbackPreview_NoMatchesAtOrAfterCutoff_ReturnsEmptySlices(t *testing.T) {
	store := baseStore()
	// Only a match before the cutoff.
	store.rows = []matches.PushbackMatchRow{
		{ID: 1, WeekNumber: 2, MatchDate: strPtr("2026-09-08"), Completed: false, HomeTeamID: 1, AwayTeamID: 2},
	}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Shifted) != 0 {
		t.Errorf("want empty shifted, got %d", len(res.Shifted))
	}
	if len(res.Preserved) != 0 {
		t.Errorf("want empty preserved, got %d", len(res.Preserved))
	}
	// NewEndDate may still be set from the before-cutoff match date.
}

func TestPushbackPreview_NoMatches_ReturnsEmptySlices(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{}
	res, err := newPushbackSvc(store).Preview(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 1, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Shifted == nil {
		t.Error("want non-nil shifted slice, got nil")
	}
	if res.Preserved == nil {
		t.Error("want non-nil preserved slice, got nil")
	}
	if res.NewEndDate != nil {
		t.Errorf("want nil NewEndDate when no matches, got %q", *res.NewEndDate)
	}
}

// --- apply ---

func TestApply_InvalidCutoff_ReturnsError(t *testing.T) {
	svc := newPushbackSvc(baseStore())
	_, err := svc.Apply(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 0, WeeksToAdd: 1,
	})
	if code(err) != "PUSHBACK_INVALID_CUTOFF" {
		t.Errorf("want PUSHBACK_INVALID_CUTOFF, got %q", code(err))
	}
}

func TestApply_ClosedWeek_ReturnsConflict(t *testing.T) {
	store := &stubPushbackStore{exists: true, closed: true}
	_, err := newPushbackSvc(store).Apply(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if !domainerr.IsCategory(err, domainerr.Conflict) {
		t.Fatalf("want Conflict, got %v", err)
	}
}

func TestApply_HappyPath_CallsStoreAndReturnsResult(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{
		{ID: 10, WeekNumber: 5, MatchDate: strPtr("2026-10-06"), Completed: false, HomeTeamID: 1, AwayTeamID: 2},
	}
	res, err := newPushbackSvc(store).Apply(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 5, WeeksToAdd: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Shifted) != 1 {
		t.Fatalf("want 1 shifted, got %d", len(res.Shifted))
	}
	if store.appliedInput == nil {
		t.Fatal("want store.ApplyPushback called, got nil")
	}
	if len(store.appliedInput.ShiftedIDs) != 1 || store.appliedInput.ShiftedIDs[0] != 10 {
		t.Errorf("want ShiftedIDs=[10], got %v", store.appliedInput.ShiftedIDs)
	}
	if store.appliedInput.WeeksToAdd != 1 {
		t.Errorf("want WeeksToAdd=1, got %d", store.appliedInput.WeeksToAdd)
	}
	if store.appliedInput.DayShift != 7 {
		t.Errorf("want DayShift=7, got %d", store.appliedInput.DayShift)
	}
}

func TestApply_StoreError_IsReturned(t *testing.T) {
	store := baseStore()
	store.applyErr = errors.New("db down")
	_, err := newPushbackSvc(store).Apply(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 1, WeeksToAdd: 1,
	})
	if err == nil {
		t.Fatal("want error from store, got nil")
	}
}

func TestApply_EmptyShifted_CallsStoreWithNoIDs(t *testing.T) {
	store := baseStore()
	store.rows = []matches.PushbackMatchRow{}
	_, err := newPushbackSvc(store).Apply(context.Background(), matches.PushbackPreviewRequest{
		SeasonID: 1, CutoffWeek: 1, WeeksToAdd: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.appliedInput == nil {
		t.Fatal("want store.ApplyPushback called even with no matches")
	}
	if len(store.appliedInput.ShiftedIDs) != 0 {
		t.Errorf("want empty ShiftedIDs, got %v", store.appliedInput.ShiftedIDs)
	}
}

// --- helper ---

func code(err error) string {
	var de *domainerr.Err
	if errors.As(err, &de) {
		return de.Code
	}
	return ""
}
