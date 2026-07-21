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
	matchCount        int
	weekStatus        string
	closeErr          error
	closeCalled       bool
	reopenErr         error
	reopenCalled      bool
	acks              []models.CloseAck
	acksErr           error
	advanceSummary    matches.WeekAdvanceSummary
	advanceSummaryErr error
	isDraft           bool
	isDraftErr        error
	recapData         matches.WeekRecapData
	recapDataErr      error
	playerStats       []models.RecapPlayerStat
	playerStatsErr    error
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

func (s *stubWeekStore) GetWeekValidationData(_ context.Context, _, _ int64) (matches.WeekValidationData, error) {
	return matches.WeekValidationData{}, nil
}

func (s *stubWeekStore) IsSeasonDraft(_ context.Context, _ int64) (bool, error) {
	return s.isDraft, s.isDraftErr
}

func (s *stubWeekStore) GetWeekRecapData(_ context.Context, _, _ int64) (matches.WeekRecapData, error) {
	return s.recapData, s.recapDataErr
}

func (s *stubWeekStore) GetWeekPlayerStats(_ context.Context, _, _ int64) ([]models.RecapPlayerStat, error) {
	return s.playerStats, s.playerStatsErr
}

// newTestSvc creates a WeekService backed by the stub store.
// previewer is optional; pass nil to test the nil-safe path.
func newTestSvc(t *testing.T, store *stubWeekStore, previewer matches.HandicapPreviewer) *matches.WeekService {
	t.Helper()
	return matches.NewWeekService(store, previewer, &stubRuleStore{})
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

func TestWeekService_CloseWeek_DraftSeasonBlocked(t *testing.T) {
	store := &stubWeekStore{isDraft: true}
	svc := newTestSvc(t, store, nil)

	_, err := svc.CloseWeek(context.Background(), matches.CloseWeekRequest{
		SeasonID:   1,
		WeekNumber: 1,
	})

	if err == nil {
		t.Fatal("want error for draft season, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want *domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Conflict {
		t.Errorf("want Conflict category, got %v", de.Category)
	}
	if de.Code != "WEEK_CLOSE_SEASON_DRAFT" {
		t.Errorf("want code WEEK_CLOSE_SEASON_DRAFT, got %q", de.Code)
	}
	if store.closeCalled {
		t.Error("store.CloseWeek must not be called for a draft season")
	}
}

func TestWeekService_CloseWeek_DraftCheckErrorPropagates(t *testing.T) {
	store := &stubWeekStore{isDraftErr: errors.New("db timeout")}
	svc := newTestSvc(t, store, nil)

	_, err := svc.CloseWeek(context.Background(), matches.CloseWeekRequest{
		SeasonID:   1,
		WeekNumber: 1,
	})

	if err == nil {
		t.Fatal("want error when draft check fails, got nil")
	}
	var de *domainerr.Err
	if errors.As(err, &de) {
		t.Errorf("want plain wrapped error (not domainerr) for store failure, got %v", err)
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

// --- WeekRecap ---

func TestWeekService_WeekRecap_NotFoundWhenNoMatches(t *testing.T) {
	store := &stubWeekStore{matchCount: 0}
	svc := newTestSvc(t, store, nil)

	_, err := svc.WeekRecap(context.Background(), 1, 1)

	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Errorf("want NotFound domainerr, got %v", err)
	}
}

func TestWeekService_WeekRecap_CountsMissingMatches(t *testing.T) {
	homeID := int64(10)
	awayID := int64(11)
	store := &stubWeekStore{
		matchCount: 1,
		recapData: matches.WeekRecapData{
			Status: "closed",
			Matches: []models.RecapMatchRow{
				{MatchID: 1, HomeTeamID: &homeID, AwayTeamID: &awayID, HasResult: true},
				{MatchID: 2, HomeTeamID: &homeID, AwayTeamID: &awayID, HasResult: false},
			},
		},
	}
	svc := newTestSvc(t, store, nil)

	recap, err := svc.WeekRecap(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recap.MissingCount != 1 {
		t.Errorf("want MissingCount=1, got %d", recap.MissingCount)
	}
	if len(recap.Matches) != 2 {
		t.Errorf("want 2 matches, got %d", len(recap.Matches))
	}
}

func TestWeekService_WeekRecap_PopulatesSeasonAndWeek(t *testing.T) {
	store := &stubWeekStore{matchCount: 1, recapData: matches.WeekRecapData{Status: "open"}}
	svc := newTestSvc(t, store, nil)

	recap, err := svc.WeekRecap(context.Background(), 5, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recap.SeasonID != 5 {
		t.Errorf("want SeasonID=5, got %d", recap.SeasonID)
	}
	if recap.WeekNumber != 2 {
		t.Errorf("want WeekNumber=2, got %d", recap.WeekNumber)
	}
}

func TestWeekService_WeekRecap_AcksNilBecomesEmptySlice(t *testing.T) {
	store := &stubWeekStore{matchCount: 1, acks: nil}
	svc := newTestSvc(t, store, nil)

	recap, err := svc.WeekRecap(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recap.Acknowledgments == nil {
		t.Error("want non-nil Acknowledgments slice, got nil")
	}
}

func TestWeekService_WeekRecap_PlayerStatsPopulated(t *testing.T) {
	stats := []models.RecapPlayerStat{
		{PlayerID: 1, PlayerName: "Alice", SetsWon: 2, SetsLost: 1, GamesWon: 6, GamesLost: 3},
	}
	store := &stubWeekStore{matchCount: 1, playerStats: stats}
	svc := newTestSvc(t, store, nil)

	recap, err := svc.WeekRecap(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recap.PlayerStats) != 1 {
		t.Fatalf("want 1 player stat, got %d", len(recap.PlayerStats))
	}
	if recap.PlayerStats[0].PlayerName != "Alice" {
		t.Errorf("want PlayerName=%q, got %q", "Alice", recap.PlayerStats[0].PlayerName)
	}
}

func TestWeekService_WeekRecap_PlayerStatsNilBecomesEmptySlice(t *testing.T) {
	store := &stubWeekStore{matchCount: 1, playerStats: nil}
	svc := newTestSvc(t, store, nil)

	recap, err := svc.WeekRecap(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recap.PlayerStats == nil {
		t.Error("want non-nil PlayerStats slice, got nil")
	}
	if len(recap.PlayerStats) != 0 {
		t.Errorf("want empty PlayerStats, got %d entries", len(recap.PlayerStats))
	}
}
