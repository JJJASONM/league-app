package handicaps_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/handicaps"
)

// ============================================================================
// Stub store
// ============================================================================

// stubStore implements handicaps.Store using in-memory data.
// RunTx and RunWriteTx both call fn(s) directly so the same stub serves as
// both root and tx-scoped store.
type stubStore struct {
	// Read-side fields
	seasonExists    bool
	seasonExistsErr error
	closedWeeks     int
	closedWeeksErr  error
	rules           handicaps.HandicapRuleRow
	rulesErr        error
	roster          []handicaps.RosterEntry
	rosterErr       error
	racks           []handicaps.RackRow
	racksErr        error

	// Write-side fields (Phase B)
	priorHistory        []handicaps.AppliedHistory
	priorHistoryErr     error
	updateHCUpdated     bool
	updateHCErr         error
	insertHistoryErr    error
	insertedHistoryRows []handicaps.HandicapHistoryRow

	// Preview-side fields
	gameDiffRecs    []handicaps.GameDiffAverageRow
	gameDiffRecsErr error
}

func (s *stubStore) RunTx(_ context.Context, fn func(handicaps.Store) error) error {
	return fn(s)
}
func (s *stubStore) RunWriteTx(_ context.Context, fn func(handicaps.Store) error) error {
	return fn(s)
}
func (s *stubStore) SeasonExists(_ context.Context, _ int64) (bool, error) {
	return s.seasonExists, s.seasonExistsErr
}
func (s *stubStore) ClosedWeekCount(_ context.Context, _ int64) (int, error) {
	return s.closedWeeks, s.closedWeeksErr
}
func (s *stubStore) SeasonHandicapRules(_ context.Context, _ int64) (handicaps.HandicapRuleRow, error) {
	return s.rules, s.rulesErr
}
func (s *stubStore) SeasonRoster(_ context.Context, _ int64) ([]handicaps.RosterEntry, error) {
	return s.roster, s.rosterErr
}
func (s *stubStore) EligibleRacks(_ context.Context, _ []int64) ([]handicaps.RackRow, error) {
	return s.racks, s.racksErr
}
func (s *stubStore) AppliedChangesByRequestID(_ context.Context, _ string) ([]handicaps.AppliedHistory, error) {
	if s.priorHistory == nil {
		return []handicaps.AppliedHistory{}, s.priorHistoryErr
	}
	return s.priorHistory, s.priorHistoryErr
}
func (s *stubStore) UpdatePlayerHandicap(_ context.Context, _ int64, _, _ float64) (bool, error) {
	return s.updateHCUpdated, s.updateHCErr
}
func (s *stubStore) InsertHandicapHistory(_ context.Context, row handicaps.HandicapHistoryRow) error {
	s.insertedHistoryRows = append(s.insertedHistoryRows, row)
	return s.insertHistoryErr
}

func (s *stubStore) GameDiffAverageRecs(_ context.Context, _ int64) ([]handicaps.GameDiffAverageRow, error) {
	return s.gameDiffRecs, s.gameDiffRecsErr
}

// runTxTrackingStore counts RunTx calls and panics on direct data-method calls.
// This proves that Recommendations calls RunTx exactly once and that all reads
// go through the tx-scoped Store passed to the callback, not the root Store.
// Phase B write methods panic so Recommendations tests can detect unexpected writes.
type runTxTrackingStore struct {
	inner  *stubStore
	runTxN int
}

func (s *runTxTrackingStore) RunTx(_ context.Context, fn func(handicaps.Store) error) error {
	s.runTxN++
	return fn(s.inner)
}
func (s *runTxTrackingStore) RunWriteTx(_ context.Context, _ func(handicaps.Store) error) error {
	panic("RunWriteTx called on Recommendations-only tracking store")
}
func (s *runTxTrackingStore) SeasonExists(_ context.Context, _ int64) (bool, error) {
	panic("SeasonExists called directly on root store")
}
func (s *runTxTrackingStore) ClosedWeekCount(_ context.Context, _ int64) (int, error) {
	panic("ClosedWeekCount called directly on root store")
}
func (s *runTxTrackingStore) SeasonHandicapRules(_ context.Context, _ int64) (handicaps.HandicapRuleRow, error) {
	panic("SeasonHandicapRules called directly on root store")
}
func (s *runTxTrackingStore) SeasonRoster(_ context.Context, _ int64) ([]handicaps.RosterEntry, error) {
	panic("SeasonRoster called directly on root store")
}
func (s *runTxTrackingStore) EligibleRacks(_ context.Context, _ []int64) ([]handicaps.RackRow, error) {
	panic("EligibleRacks called directly on root store")
}
func (s *runTxTrackingStore) AppliedChangesByRequestID(_ context.Context, _ string) ([]handicaps.AppliedHistory, error) {
	panic("AppliedChangesByRequestID called on Recommendations-only tracking store")
}
func (s *runTxTrackingStore) UpdatePlayerHandicap(_ context.Context, _ int64, _, _ float64) (bool, error) {
	panic("UpdatePlayerHandicap called on Recommendations-only tracking store")
}
func (s *runTxTrackingStore) InsertHandicapHistory(_ context.Context, _ handicaps.HandicapHistoryRow) error {
	panic("InsertHandicapHistory called on Recommendations-only tracking store")
}
func (s *runTxTrackingStore) GameDiffAverageRecs(_ context.Context, _ int64) ([]handicaps.GameDiffAverageRow, error) {
	panic("GameDiffAverageRecs called on Recommendations-only tracking store")
}

func ptr(s string) *string { return &s }

// ============================================================================
// RunTx contract tests
// ============================================================================

func TestRecommendations_RunTxCalledExactlyOnce(t *testing.T) {
	inner := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("manual_review")},
	}
	tracker := &runTxTrackingStore{inner: inner}
	svc := handicaps.NewService(tracker)

	if _, err := svc.Recommendations(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.runTxN != 1 {
		t.Errorf("want RunTx called once, got %d", tracker.runTxN)
	}
}

// TestRecommendations_ReadsUseTransactionScopedStore panics (caught as t.Error)
// if any data method is called on the root store directly instead of via the
// tx-scoped Store passed to the RunTx callback.
func TestRecommendations_ReadsUseTransactionScopedStore(t *testing.T) {
	inner := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("manual_review")},
	}
	tracker := &runTxTrackingStore{inner: inner}
	svc := handicaps.NewService(tracker)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("root store data method called directly: %v", r)
		}
	}()
	if _, err := svc.Recommendations(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRecommendations_CallbackErrorWrappedAsDataError proves that a plain
// adapter error surfaced inside RunTx is translated to HC_DATA_ERROR with
// the original cause attached via Unwrap.
func TestRecommendations_CallbackErrorWrappedAsDataError(t *testing.T) {
	cause := errors.New("connection refused")
	inner := &stubStore{
		seasonExists:    true,
		seasonExistsErr: cause,
	}
	tracker := &runTxTrackingStore{inner: inner}
	svc := handicaps.NewService(tracker)

	_, err := svc.Recommendations(context.Background(), 1)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want *domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != "HC_DATA_ERROR" {
		t.Errorf("want code=HC_DATA_ERROR, got %q", de.Code)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
	if de.Cause == nil {
		t.Error("want Cause set on wrapped error")
	}
}

// TestRecommendations_DomainErrorPassesThroughUnchanged proves that a domain
// error created inside the RunTx callback (e.g. NotFound) is returned unchanged
// and not re-wrapped as HC_DATA_ERROR.
func TestRecommendations_DomainErrorPassesThroughUnchanged(t *testing.T) {
	// seasonExists=false triggers HC_SEASON_NOT_FOUND inside compute.
	inner := &stubStore{seasonExists: false}
	tracker := &runTxTrackingStore{inner: inner}
	svc := handicaps.NewService(tracker)

	_, err := svc.Recommendations(context.Background(), 1)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want *domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != "HC_SEASON_NOT_FOUND" {
		t.Errorf("want code=HC_SEASON_NOT_FOUND (not HC_DATA_ERROR), got %q", de.Code)
	}
	if de.Category != domainerr.NotFound {
		t.Errorf("want NotFound category, got %v", de.Category)
	}
}

// ============================================================================
// Method routing
// ============================================================================

func TestRecommendations_SeasonNotFound_ReturnsNotFound(t *testing.T) {
	svc := handicaps.NewService(&stubStore{seasonExists: false})
	_, err := svc.Recommendations(context.Background(), 99)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !domainerr.IsCategory(err, domainerr.NotFound) {
		t.Errorf("want NotFound category, got %v", err)
	}
}

func TestRecommendations_ManualReview_ReturnsNoAutoApply(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("manual_review")},
	}
	resp, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "no_auto_apply" {
		t.Errorf("want status=no_auto_apply, got %q", resp.Status)
	}
	if resp.Method != "manual_review" {
		t.Errorf("want method=manual_review, got %q", resp.Method)
	}
}

func TestRecommendations_NilUpdateMethod_DefaultsToManualReview(t *testing.T) {
	store := &stubStore{seasonExists: true, rules: handicaps.HandicapRuleRow{}}
	resp, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "no_auto_apply" {
		t.Errorf("want status=no_auto_apply, got %q", resp.Status)
	}
}

func TestRecommendations_KickerAveragePreview_ReturnsUnsupported(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("kicker_average_preview")},
	}
	resp, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "unsupported" {
		t.Errorf("want status=unsupported, got %q", resp.Status)
	}
}

// ============================================================================
// NoData path
// ============================================================================

func TestRecommendations_NoClosedWeeks_ReturnsNoData(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
		closedWeeks:  0,
	}
	resp, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "no_data" {
		t.Errorf("want status=no_data, got %q", resp.Status)
	}
}

// ============================================================================
// Rule interpretation
// ============================================================================

func TestRecommendations_InvalidWindow_ReturnsInternalError(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules: handicaps.HandicapRuleRow{
			UpdateMethod: ptr("game_diff_average"),
			WindowSize:   ptr("not-a-number"),
		},
		closedWeeks: 1,
	}
	_, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err == nil {
		t.Fatal("want error for invalid window, got nil")
	}
	if !domainerr.IsCategory(err, domainerr.Internal) {
		t.Errorf("want Internal category, got %v", err)
	}
}

func TestRecommendations_ZeroWindow_ReturnsInternalError(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules: handicaps.HandicapRuleRow{
			UpdateMethod: ptr("game_diff_average"),
			WindowSize:   ptr("0"),
		},
		closedWeeks: 1,
	}
	_, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err == nil {
		t.Fatal("want error for zero window, got nil")
	}
	if !domainerr.IsCategory(err, domainerr.Internal) {
		t.Errorf("want Internal category, got %v", err)
	}
}

// Blank window silently defaults to 15 -- no error, returns preview.
func TestRecommendations_BlankWindow_DefaultsTo15(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules: handicaps.HandicapRuleRow{
			UpdateMethod: ptr("game_diff_average"),
			WindowSize:   ptr(""), // blank = default
		},
		closedWeeks: 1,
		roster:      []handicaps.RosterEntry{},
	}
	resp, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "preview" {
		t.Errorf("want status=preview for empty roster, got %q", resp.Status)
	}
}

// Invalid max_individual_handicap silently defaults to 4.5.
// A player with implied HC > 4.5 should be capped at 4.5.
func TestRecommendations_InvalidMaxHC_DefaultsTo4_5(t *testing.T) {
	oppHC := 5.0
	store := &stubStore{
		seasonExists: true,
		rules: handicaps.HandicapRuleRow{
			UpdateMethod: ptr("game_diff_average"),
			MaxHC:        ptr("bad"), // invalid -- silently defaults to 4.5
		},
		closedWeeks: 1,
		roster: []handicaps.RosterEntry{
			{PlayerID: 1, PlayerName: "Test Player", TeamName: "Team A", AssignedHC: 1.5},
		},
		// 15 racks: player wins 10-7 vs opponent HC 5.0 -> implied ~8.5 > 4.5 -> capped
		racks: func() []handicaps.RackRow {
			rows := make([]handicaps.RackRow, 15)
			for i := range rows {
				rows[i] = handicaps.RackRow{
					HomePlayerID: 1, AwayPlayerID: 2,
					G1H: 10, G1A: 7,
					G2H: 10, G2A: 7,
					G3H: 10, G3A: 7,
					AwayHCUsed: &oppHC,
				}
			}
			return rows
		}(),
	}
	resp, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Recommendations) != 1 {
		t.Fatalf("want 1 recommendation, got %d", len(resp.Recommendations))
	}
	rec := resp.Recommendations[0]
	if rec.Reason != "capped" {
		t.Errorf("want reason=capped, got %q", rec.Reason)
	}
	if rec.RecommendedHC == nil || *rec.RecommendedHC != 4.5 {
		t.Errorf("want recommended_hc=4.5, got %v", rec.RecommendedHC)
	}
}

// ============================================================================
// domainerr safety
// ============================================================================

// Error() must return only Message so infrastructure details cannot leak
// through an unguarded err.Error() call in a handler.
func TestDomainerrErr_ErrorOmitsCause(t *testing.T) {
	cause := errors.New("secret db path: /var/data/prod.db")
	err := domainerr.Wrap("CODE", domainerr.Internal, "internal error", cause)
	if err.Error() == cause.Error() || err.Error() == "internal error: "+cause.Error() {
		t.Errorf("Error() must not include cause, got %q", err.Error())
	}
	if err.Error() != "internal error" {
		t.Errorf("Error() want %q, got %q", "internal error", err.Error())
	}
}

// ============================================================================
// Preview message
// ============================================================================

func TestRecommendations_Preview_ChangedCountMessage(t *testing.T) {
	oppHC := 2.0
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
		closedWeeks:  1,
		roster: []handicaps.RosterEntry{
			{PlayerID: 1, PlayerName: "Alice", TeamName: "T1", AssignedHC: 0.0},
		},
		racks: func() []handicaps.RackRow {
			rows := make([]handicaps.RackRow, 15)
			for i := range rows {
				rows[i] = handicaps.RackRow{
					HomePlayerID: 1, AwayPlayerID: 2,
					G1H: 10, G1A: 7,
					G2H: 10, G2A: 7,
					G3H: 10, G3A: 7,
					AwayHCUsed: &oppHC,
				}
			}
			return rows
		}(),
	}
	resp, err := handicaps.NewService(store).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "preview" {
		t.Errorf("want status=preview, got %q", resp.Status)
	}
	if len(resp.Recommendations) != 1 {
		t.Fatalf("want 1 recommendation, got %d", len(resp.Recommendations))
	}
	if resp.Message == "" {
		t.Error("want non-empty message")
	}
}

// ============================================================================
// HandicapPreview
// ============================================================================

func TestHandicapPreview_ManualReview_NoAutoApply(t *testing.T) {
	method := "manual_review"
	store := &stubStore{
		rules: handicaps.HandicapRuleRow{UpdateMethod: &method},
	}
	svc := handicaps.NewService(store)

	hc, err := svc.HandicapPreview(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hc.Method != "manual_review" {
		t.Errorf("want method=manual_review, got %q", hc.Method)
	}
	if hc.Status != "no_auto_apply" {
		t.Errorf("want status=no_auto_apply, got %q", hc.Status)
	}
}

func TestHandicapPreview_KickerAverage_Unsupported(t *testing.T) {
	method := "kicker_average_preview"
	store := &stubStore{
		rules: handicaps.HandicapRuleRow{UpdateMethod: &method},
	}
	svc := handicaps.NewService(store)

	hc, err := svc.HandicapPreview(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hc.Status != "unsupported" {
		t.Errorf("want status=unsupported, got %q", hc.Status)
	}
}

func TestHandicapPreview_GameDiffAverage_NoRecs_NoChange(t *testing.T) {
	method := "game_diff_average"
	store := &stubStore{
		rules:        handicaps.HandicapRuleRow{UpdateMethod: &method},
		gameDiffRecs: []handicaps.GameDiffAverageRow{},
	}
	svc := handicaps.NewService(store)

	hc, err := svc.HandicapPreview(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hc.Status != "preview" {
		t.Errorf("want status=preview, got %q", hc.Status)
	}
	if len(hc.Recommendations) != 0 {
		t.Errorf("want 0 recommendations, got %d", len(hc.Recommendations))
	}
}

func TestHandicapPreview_GameDiffAverage_OneChange_MessageSingular(t *testing.T) {
	method := "game_diff_average"
	maxHC := "4.5"
	store := &stubStore{
		rules: handicaps.HandicapRuleRow{UpdateMethod: &method, MaxHC: &maxHC},
		gameDiffRecs: []handicaps.GameDiffAverageRow{
			{PlayerID: 1, PlayerName: "Alice A", CurrentHC: 2.0, MatchCount: 3, TotalDiff: 9.0},
		},
	}
	svc := handicaps.NewService(store)

	hc, err := svc.HandicapPreview(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hc.Status != "preview" {
		t.Errorf("want status=preview, got %q", hc.Status)
	}
	if len(hc.Recommendations) != 1 {
		t.Fatalf("want 1 recommendation, got %d", len(hc.Recommendations))
	}
	rec := hc.Recommendations[0]
	if rec.RecommendedHandicap != 3.0 {
		t.Errorf("want recommended=3.0, got %v", rec.RecommendedHandicap)
	}
	if rec.Reason == "no_change" {
		t.Error("expected a change reason, got no_change")
	}
}

func TestHandicapPreview_GameDiffAverage_AdminHold_Skipped(t *testing.T) {
	method := "game_diff_average"
	store := &stubStore{
		rules: handicaps.HandicapRuleRow{UpdateMethod: &method},
		gameDiffRecs: []handicaps.GameDiffAverageRow{
			{PlayerID: 1, PlayerName: "Bob B", CurrentHC: 2.0, AdminHold: true, MatchCount: 5, TotalDiff: 15.0},
		},
	}
	svc := handicaps.NewService(store)

	hc, err := svc.HandicapPreview(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hc.Recommendations) != 1 {
		t.Fatalf("want 1 rec, got %d", len(hc.Recommendations))
	}
	rec := hc.Recommendations[0]
	if !rec.Skipped {
		t.Error("want Skipped=true for admin_hold player")
	}
	if rec.Reason != "admin_hold" {
		t.Errorf("want reason=admin_hold, got %q", rec.Reason)
	}
}

func TestHandicapPreview_GameDiffAverage_CappedAtMaxHC(t *testing.T) {
	method := "game_diff_average"
	maxHC := "3.0"
	store := &stubStore{
		rules: handicaps.HandicapRuleRow{UpdateMethod: &method, MaxHC: &maxHC},
		gameDiffRecs: []handicaps.GameDiffAverageRow{
			{PlayerID: 1, PlayerName: "Carol C", CurrentHC: 1.0, MatchCount: 2, TotalDiff: 10.0},
		},
	}
	svc := handicaps.NewService(store)

	hc, err := svc.HandicapPreview(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hc.Recommendations) != 1 {
		t.Fatalf("want 1 rec, got %d", len(hc.Recommendations))
	}
	rec := hc.Recommendations[0]
	if rec.RecommendedHandicap != 3.0 {
		t.Errorf("want recommended capped at 3.0, got %v", rec.RecommendedHandicap)
	}
	if rec.Reason != "capped" {
		t.Errorf("want reason=capped, got %q", rec.Reason)
	}
}

func TestHandicapPreview_RulesError_Propagates(t *testing.T) {
	store := &stubStore{rulesErr: errors.New("db error")}
	svc := handicaps.NewService(store)

	_, err := svc.HandicapPreview(context.Background(), 1)
	if err == nil {
		t.Fatal("want error when rules query fails")
	}
}
