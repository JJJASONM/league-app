package handicaps_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/handicaps"
)

// ============================================================================
// Helpers shared by Apply tests
// ============================================================================

const validUUID = "550e8400-e29b-41d4-a716-446655440000"

// applyRosterEntry returns a single actionable roster player with 15 racks
// that produce an implied HC of roughly 3.0 vs opponent HC 0.0.
// The function also returns a valid rec token via a Recommendations call so
// the Apply test can supply a correct token without embedding the algorithm.
func applyStoreWithToken(t *testing.T) (*stubStore, string) {
	t.Helper()
	oppHC := 0.0
	store := &stubStore{
		seasonExists: true,
		rules: handicaps.HandicapRuleRow{
			UpdateMethod: ptr("game_diff_average"),
		},
		closedWeeks: 1,
		roster: []handicaps.RosterEntry{
			{PlayerID: 1, PlayerName: "Alice", TeamName: "Strikers", AssignedHC: 1.0},
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
		updateHCUpdated: true,
	}

	svc := handicaps.NewService(store)
	resp, err := svc.Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(resp.Recommendations) != 1 {
		t.Fatalf("want 1 rec, got %d", len(resp.Recommendations))
	}
	rec := resp.Recommendations[0]
	if rec.RecToken == "" {
		t.Fatal("expected non-empty rec token for actionable player")
	}
	return store, rec.RecToken
}

// applyReq builds a single-entry ApplyRequest using the given token and
// the live recommendation values from the store's current state.
func applyReqFor(t *testing.T, svc *handicaps.Service, token string) handicaps.ApplyRequest {
	t.Helper()
	resp, err := svc.Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	rec := resp.Recommendations[0]
	return handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              rec.PlayerID,
			ExpectedAssignedHC:    rec.AssignedHC,
			ExpectedRecommendedHC: *rec.RecommendedHC,
			RecToken:              token,
		}},
	}
}

// ============================================================================
// validateApplyRequest — structural validation (no store calls)
// ============================================================================

func TestApply_InvalidRequest_MissingApplyRequestID(t *testing.T) {
	store, _ := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		Entries: []handicaps.ApplyEntry{{PlayerID: 1, RecToken: "x"}},
	})
	if err == nil {
		t.Fatal("want error for missing apply_request_id")
	}
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestApply_InvalidRequest_InvalidUUID(t *testing.T) {
	store, _ := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: "not-a-uuid",
		Entries:        []handicaps.ApplyEntry{{PlayerID: 1, RecToken: "x"}},
	})
	if err == nil {
		t.Fatal("want error for invalid UUID")
	}
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestApply_InvalidRequest_EmptyEntries(t *testing.T) {
	store, _ := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        []handicaps.ApplyEntry{},
	})
	if err == nil {
		t.Fatal("want error for empty entries")
	}
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestApply_InvalidRequest_DuplicatePlayerID(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{
			{PlayerID: 1, RecToken: tok},
			{PlayerID: 1, RecToken: tok},
		},
	})
	if err == nil {
		t.Fatal("want error for duplicate player_id")
	}
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestApply_InvalidRequest_NonFiniteHC(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:           1,
			ExpectedAssignedHC: math.Inf(1),
			RecToken:           tok,
		}},
	})
	if err == nil {
		t.Fatal("want error for non-finite expected_assigned_hc")
	}
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

func TestApply_InvalidRequest_MissingRecToken(t *testing.T) {
	store, _ := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:           1,
			ExpectedAssignedHC: 1.0,
			RecToken:           "",
		}},
	})
	if err == nil {
		t.Fatal("want error for missing rec_token")
	}
	if !domainerr.IsCategory(err, domainerr.InvalidInput) {
		t.Errorf("want InvalidInput, got %v", err)
	}
}

// ============================================================================
// Method gate
// ============================================================================

func TestApply_MethodNotApply_ManualReview_Returns422(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("manual_review")},
		closedWeeks:  1,
	}
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        []handicaps.ApplyEntry{{PlayerID: 1, RecToken: "tok", ExpectedAssignedHC: 1.0}},
	})
	if err == nil {
		t.Fatal("want error for manual_review method")
	}
	if !domainerr.IsCategory(err, domainerr.Unprocessable) {
		t.Errorf("want Unprocessable, got %v (category check)", err)
	}
}

func TestApply_MethodNotApply_KickerAveragePreview_Returns422(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("kicker_average_preview")},
		closedWeeks:  1,
	}
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        []handicaps.ApplyEntry{{PlayerID: 1, RecToken: "tok", ExpectedAssignedHC: 1.0}},
	})
	if err == nil {
		t.Fatal("want error for kicker_average_preview method")
	}
	if !domainerr.IsCategory(err, domainerr.Unprocessable) {
		t.Errorf("want Unprocessable, got %v", err)
	}
}

// ============================================================================
// Season not found
// ============================================================================

func TestApply_SeasonNotFound_Returns404(t *testing.T) {
	store := &stubStore{seasonExists: false}
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 99, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        []handicaps.ApplyEntry{{PlayerID: 1, RecToken: "tok", ExpectedAssignedHC: 1.0}},
	})
	if err == nil {
		t.Fatal("want error for season not found")
	}
	if !domainerr.IsCategory(err, domainerr.NotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

// ============================================================================
// Idempotency replay
// ============================================================================

func TestApply_Idempotent_SameHash_SamePlayerSet_ReturnsReplayed(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)
	req := applyReqFor(t, svc, tok)

	// Seed prior history so replay detects the same hash.
	// We need the actual request_hash; use the same entries.
	// To get the right hash, run Apply once with updateHCUpdated=true,
	// then capture and seed the history.
	result, err := svc.Apply(context.Background(), 1, req)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("first Apply: want 1 applied, got %d", len(result.Applied))
	}

	// Now seed the store's priorHistory with the written row
	// and run Apply again — should return replayed=true.
	row := store.insertedHistoryRows[0]
	store.priorHistory = []handicaps.AppliedHistory{{
		PlayerID:           row.PlayerID,
		PlayerNameSnapshot: row.PlayerNameSnapshot,
		OldHandicap:        row.OldHandicap,
		NewHandicap:        row.NewHandicap,
		RequestHash:        row.RequestHash,
	}}
	// Reset write state.
	store.insertedHistoryRows = nil
	store.updateHCUpdated = true

	result2, err := svc.Apply(context.Background(), 1, req)
	if err != nil {
		t.Fatalf("second Apply (replay): %v", err)
	}
	if !result2.Replayed {
		t.Error("want Replayed=true on second Apply with same request")
	}
	if len(result2.Applied) != 1 {
		t.Errorf("want 1 applied in replay, got %d", len(result2.Applied))
	}
	// Verify no new writes on replay.
	if len(store.insertedHistoryRows) != 0 {
		t.Error("want no InsertHandicapHistory calls on replay")
	}
}

func TestApply_Idempotent_DifferentHash_Returns409Conflict(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	// Seed prior history with a different hash for the same request ID.
	store.priorHistory = []handicaps.AppliedHistory{{
		PlayerID:    1,
		RequestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}}
	req := applyReqFor(t, svc, tok)

	_, err := svc.Apply(context.Background(), 1, req)
	if err == nil {
		t.Fatal("want error for idempotency key reused with different hash")
	}
	if !domainerr.IsCategory(err, domainerr.Conflict) {
		t.Errorf("want Conflict, got %v", err)
	}
}

// ============================================================================
// Per-entry conflicts
// ============================================================================

func TestApply_NotInRoster_ReturnsConflict(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              999, // not in roster
			ExpectedAssignedHC:    1.0,
			ExpectedRecommendedHC: 3.0,
			RecToken:              tok,
		}},
	})
	var ce *handicaps.ApplyConflictErr
	if !errors.As(err, &ce) {
		t.Fatalf("want *ApplyConflictErr, got %T: %v", err, err)
	}
	if len(ce.Conflicts) != 1 || ce.Conflicts[0].Code != handicaps.ConflictNotInRoster {
		t.Errorf("want ConflictNotInRoster, got %+v", ce.Conflicts)
	}
}

func TestApply_TokenMismatch_ReturnsConflict(t *testing.T) {
	store, _ := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	resp, _ := svc.Recommendations(context.Background(), 1)
	rec := resp.Recommendations[0]

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              rec.PlayerID,
			ExpectedAssignedHC:    rec.AssignedHC,
			ExpectedRecommendedHC: *rec.RecommendedHC,
			RecToken:              "stale-or-wrong-token",
		}},
	})
	var ce *handicaps.ApplyConflictErr
	if !errors.As(err, &ce) {
		t.Fatalf("want *ApplyConflictErr, got %T: %v", err, err)
	}
	if len(ce.Conflicts) != 1 || ce.Conflicts[0].Code != handicaps.ConflictTokenMismatch {
		t.Errorf("want ConflictTokenMismatch, got %+v", ce.Conflicts)
	}
}

func TestApply_AssignedHCChanged_ReturnsConflict(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	resp, _ := svc.Recommendations(context.Background(), 1)
	rec := resp.Recommendations[0]

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              rec.PlayerID,
			ExpectedAssignedHC:    rec.AssignedHC + 1.0, // wrong expected
			ExpectedRecommendedHC: *rec.RecommendedHC,
			RecToken:              tok,
		}},
	})
	var ce *handicaps.ApplyConflictErr
	if !errors.As(err, &ce) {
		t.Fatalf("want *ApplyConflictErr, got %T: %v", err, err)
	}
	if len(ce.Conflicts) != 1 || ce.Conflicts[0].Code != handicaps.ConflictAssignedHCChanged {
		t.Errorf("want ConflictAssignedHCChanged, got %+v", ce.Conflicts)
	}
}

func TestApply_RecommendedHCChanged_ReturnsConflict(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	resp, _ := svc.Recommendations(context.Background(), 1)
	rec := resp.Recommendations[0]

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              rec.PlayerID,
			ExpectedAssignedHC:    rec.AssignedHC,
			ExpectedRecommendedHC: *rec.RecommendedHC + 1.0, // wrong expected
			RecToken:              tok,
		}},
	})
	var ce *handicaps.ApplyConflictErr
	if !errors.As(err, &ce) {
		t.Fatalf("want *ApplyConflictErr, got %T: %v", err, err)
	}
	if len(ce.Conflicts) != 1 || ce.Conflicts[0].Code != handicaps.ConflictRecommendedHCChanged {
		t.Errorf("want ConflictRecommendedHCChanged, got %+v", ce.Conflicts)
	}
}

// ============================================================================
// Per-entry rejections
// ============================================================================

func TestApply_AdminHold_ReturnsRejection(t *testing.T) {
	oppHC := 0.0
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
		closedWeeks:  1,
		roster: []handicaps.RosterEntry{
			{PlayerID: 1, PlayerName: "Alice", TeamName: "Strikers", AssignedHC: 1.0, AdminHold: true},
		},
		racks: func() []handicaps.RackRow {
			rows := make([]handicaps.RackRow, 15)
			for i := range rows {
				rows[i] = handicaps.RackRow{HomePlayerID: 1, AwayPlayerID: 2, G1H: 10, G1A: 7, G2H: 10, G2A: 7, G3H: 10, G3A: 7, AwayHCUsed: &oppHC}
			}
			return rows
		}(),
	}
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:           1,
			ExpectedAssignedHC: 1.0,
			RecToken:           "any",
		}},
	})
	var re *handicaps.ApplyRejectionErr
	if !errors.As(err, &re) {
		t.Fatalf("want *ApplyRejectionErr, got %T: %v", err, err)
	}
	if len(re.Rejections) != 1 || re.Rejections[0].Code != handicaps.RejectionAdminHold {
		t.Errorf("want RejectionAdminHold, got %+v", re.Rejections)
	}
}

func TestApply_BelowThreshold_ReturnsRejection(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
		closedWeeks:  1,
		roster:       []handicaps.RosterEntry{{PlayerID: 1, PlayerName: "Bob", AssignedHC: 1.0}},
		// No racks → WindowRacks=0 < threshold=15 → below_threshold
		racks: nil,
	}
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        []handicaps.ApplyEntry{{PlayerID: 1, ExpectedAssignedHC: 1.0, RecToken: "any"}},
	})
	var re *handicaps.ApplyRejectionErr
	if !errors.As(err, &re) {
		t.Fatalf("want *ApplyRejectionErr, got %T: %v", err, err)
	}
	found := false
	for _, r := range re.Rejections {
		if r.PlayerID == 1 {
			found = true
			if r.Code != handicaps.RejectionBelowThreshold && r.Code != handicaps.RejectionNoData {
				t.Errorf("want below_threshold or no_data, got %q", r.Code)
			}
		}
	}
	if !found {
		t.Error("want rejection for player 1")
	}
}

// ============================================================================
// Successful apply
// ============================================================================

func TestApply_Success_WritesHistoryAndReturnsApplied(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)
	req := applyReqFor(t, svc, tok)

	result, err := svc.Apply(context.Background(), 1, req)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("want 1 applied, got %d", len(result.Applied))
	}
	if result.Replayed {
		t.Error("want Replayed=false on first apply")
	}
	if result.ApplyRequestID != validUUID {
		t.Errorf("want ApplyRequestID=%q, got %q", validUUID, result.ApplyRequestID)
	}
	// Verify handicap_history row was written.
	if len(store.insertedHistoryRows) != 1 {
		t.Fatalf("want 1 history row, got %d", len(store.insertedHistoryRows))
	}
	row := store.insertedHistoryRows[0]
	if row.ApplyRequestID != validUUID {
		t.Errorf("history row: want apply_request_id=%q, got %q", validUUID, row.ApplyRequestID)
	}
	if row.Method != "game_diff_average" {
		t.Errorf("history row: want method=game_diff_average, got %q", row.Method)
	}
	if row.SeasonID != 1 {
		t.Errorf("history row: want season_id=1, got %d", row.SeasonID)
	}
	if row.RecToken == "" {
		t.Error("history row: want non-empty rec_token")
	}
	if row.RequestHash == "" {
		t.Error("history row: want non-empty request_hash")
	}
	if row.PlayerNameSnapshot == "" {
		t.Error("history row: want non-empty player_name_snapshot")
	}
	if row.EffectiveDate == "" {
		t.Error("history row: want non-empty effective_date")
	}
}

// TestApply_WriteUsesLiveRecommendedValue verifies that the written new_handicap
// is the live recommended value, not the request's expected_recommended_hc.
func TestApply_WriteUsesLiveRecommendedValue(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	resp, _ := svc.Recommendations(context.Background(), 1)
	rec := resp.Recommendations[0]
	liveRec := *rec.RecommendedHC

	result, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              rec.PlayerID,
			ExpectedAssignedHC:    rec.AssignedHC,
			ExpectedRecommendedHC: liveRec, // must match live
			RecToken:              tok,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("want 1 applied, got %d", len(result.Applied))
	}
	if result.Applied[0].NewHandicap != liveRec {
		t.Errorf("NewHandicap: want live value %.4f, got %.4f", liveRec, result.Applied[0].NewHandicap)
	}
}

// ============================================================================
// ConcurrentWrite
// ============================================================================

func TestApply_ConcurrentWrite_ReturnsConflictConcurrentWrite(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	store.updateHCUpdated = false // not needed; RunWriteTx will return ErrConcurrentWrite
	svc := handicaps.NewService(store)
	req := applyReqFor(t, svc, tok)

	// Wrap the store so RunWriteTx returns ErrConcurrentWrite.
	busyStore := &busyWriteStore{inner: store}
	svcBusy := handicaps.NewService(busyStore)

	_, err := svcBusy.Apply(context.Background(), 1, req)
	var ce *handicaps.ApplyConflictErr
	if !errors.As(err, &ce) {
		t.Fatalf("want *ApplyConflictErr, got %T: %v", err, err)
	}
	found := false
	for _, c := range ce.Conflicts {
		if c.Code == handicaps.ConflictConcurrentWrite {
			found = true
		}
	}
	if !found {
		t.Errorf("want ConflictConcurrentWrite in conflicts, got %+v", ce.Conflicts)
	}
}

// busyWriteStore wraps stubStore and returns ErrConcurrentWrite from RunWriteTx.
type busyWriteStore struct {
	inner *stubStore
}

func (b *busyWriteStore) RunTx(ctx context.Context, fn func(handicaps.Store) error) error {
	return b.inner.RunTx(ctx, fn)
}
func (b *busyWriteStore) RunWriteTx(_ context.Context, _ func(handicaps.Store) error) error {
	return handicaps.ErrConcurrentWrite
}
func (b *busyWriteStore) SeasonExists(ctx context.Context, id int64) (bool, error) {
	return b.inner.SeasonExists(ctx, id)
}
func (b *busyWriteStore) ClosedWeekCount(ctx context.Context, id int64) (int, error) {
	return b.inner.ClosedWeekCount(ctx, id)
}
func (b *busyWriteStore) SeasonHandicapRules(ctx context.Context, id int64) (handicaps.HandicapRuleRow, error) {
	return b.inner.SeasonHandicapRules(ctx, id)
}
func (b *busyWriteStore) SeasonRoster(ctx context.Context, id int64) ([]handicaps.RosterEntry, error) {
	return b.inner.SeasonRoster(ctx, id)
}
func (b *busyWriteStore) EligibleRacks(ctx context.Context, ids []int64) ([]handicaps.RackRow, error) {
	return b.inner.EligibleRacks(ctx, ids)
}
func (b *busyWriteStore) AppliedChangesByRequestID(ctx context.Context, id string) ([]handicaps.AppliedHistory, error) {
	return b.inner.AppliedChangesByRequestID(ctx, id)
}
func (b *busyWriteStore) UpdatePlayerHandicap(ctx context.Context, pid int64, n, e float64) (bool, error) {
	return b.inner.UpdatePlayerHandicap(ctx, pid, n, e)
}
func (b *busyWriteStore) InsertHandicapHistory(ctx context.Context, row handicaps.HandicapHistoryRow) error {
	return b.inner.InsertHandicapHistory(ctx, row)
}
func (b *busyWriteStore) GameDiffAverageRecs(ctx context.Context, id int64) ([]handicaps.GameDiffAverageRow, error) {
	return b.inner.GameDiffAverageRecs(ctx, id)
}

// ============================================================================
// Token / hash determinism
// ============================================================================

// TestApply_RecToken_IsDeterministic proves that calling Recommendations twice
// with the same inputs produces the same token (no randomness).
func TestApply_RecToken_IsDeterministic(t *testing.T) {
	store, tok1 := applyStoreWithToken(t)
	// Reset and get token again from the same store state.
	svc := handicaps.NewService(store)
	resp, err := svc.Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("second Recommendations: %v", err)
	}
	tok2 := resp.Recommendations[0].RecToken
	if tok1 != tok2 {
		t.Errorf("tokens differ: %q vs %q", tok1, tok2)
	}
}

// TestApply_RecToken_ChangesWhenRackAdded proves that adding one more rack
// changes the token (it covers all samples, not just the window slice).
func TestApply_RecToken_ChangesWhenRackAdded(t *testing.T) {
	store1, tok1 := applyStoreWithToken(t)
	_ = store1

	// Add one more rack to the store.
	oppHC := 0.0
	extraRack := handicaps.RackRow{
		HomePlayerID: 1, AwayPlayerID: 2,
		G1H: 10, G1A: 5,
		G2H: 10, G2A: 5,
		G3H: 10, G3A: 5,
		AwayHCUsed: &oppHC,
	}
	store2 := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
		closedWeeks:  1,
		roster:       []handicaps.RosterEntry{{PlayerID: 1, PlayerName: "Alice", TeamName: "Strikers", AssignedHC: 1.0}},
		racks:        append(store1.racks, extraRack),
	}
	resp2, err := handicaps.NewService(store2).Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recommendations with extra rack: %v", err)
	}
	tok2 := resp2.Recommendations[0].RecToken
	if tok1 == tok2 {
		t.Error("want token to change when a rack is added, but it did not")
	}
}

// TestApply_CentEquivalentAssignedHC_ProduceSameToken is a regression test for
// the token normalization fix: float noise in the stored assigned HC (e.g.
// 2.4999999 vs 2.50) must not produce different rec tokens. The token builder
// normalizes AssignedHC and RecommendedHC to 0.01 precision before hashing.
func TestApply_CentEquivalentAssignedHC_ProduceSameToken(t *testing.T) {
	oppHC := 0.0

	makeStore := func(hc float64) *stubStore {
		rows := make([]handicaps.RackRow, 15)
		for i := range rows {
			rows[i] = handicaps.RackRow{
				HomePlayerID: 1, AwayPlayerID: 2,
				G1H: 10, G1A: 7, G2H: 10, G2A: 7, G3H: 10, G3A: 7,
				AwayHCUsed: &oppHC,
			}
		}
		return &stubStore{
			seasonExists: true,
			rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
			closedWeeks:  1,
			roster:       []handicaps.RosterEntry{{PlayerID: 1, PlayerName: "Alice", AssignedHC: hc}},
			racks:        rows,
		}
	}

	getToken := func(hc float64) string {
		resp, err := handicaps.NewService(makeStore(hc)).Recommendations(context.Background(), 1)
		if err != nil {
			t.Fatalf("Recommendations(hc=%.10f): %v", hc, err)
		}
		if len(resp.Recommendations) != 1 {
			t.Fatalf("want 1 rec, got %d", len(resp.Recommendations))
		}
		tok := resp.Recommendations[0].RecToken
		if tok == "" {
			t.Fatalf("want non-empty token for actionable player (hc=%.10f)", hc)
		}
		return tok
	}

	tok1 := getToken(2.50)
	tok2 := getToken(2.4999999999)

	if tok1 != tok2 {
		t.Errorf("cent-equivalent assigned HCs must produce same token:\n  2.50:         %q\n  2.4999999999: %q", tok1, tok2)
	}
}

// TestApply_RequestHash_IsOrderIndependent proves that two requests with the
// same players in different order produce the same request hash (verified by
// the idempotency replay logic accepting both as the same request).
func TestApply_RequestHash_IsOrderIndependent(t *testing.T) {
	// Set up a two-player store with sufficient racks for both players.
	opp1HC := 0.0
	opp2HC := 0.0
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
		closedWeeks:  1,
		roster: []handicaps.RosterEntry{
			{PlayerID: 1, PlayerName: "Alice", AssignedHC: 1.0},
			{PlayerID: 2, PlayerName: "Bob", AssignedHC: 1.0},
		},
		racks: func() []handicaps.RackRow {
			rows := make([]handicaps.RackRow, 30) // 15 per player
			for i := 0; i < 15; i++ {
				rows[i] = handicaps.RackRow{
					HomePlayerID: 1, AwayPlayerID: 3,
					G1H: 10, G1A: 7, G2H: 10, G2A: 7, G3H: 10, G3A: 7,
					AwayHCUsed: &opp1HC,
				}
				rows[i+15] = handicaps.RackRow{
					HomePlayerID: 2, AwayPlayerID: 3,
					G1H: 10, G1A: 7, G2H: 10, G2A: 7, G3H: 10, G3A: 7,
					AwayHCUsed: &opp2HC,
				}
			}
			return rows
		}(),
		updateHCUpdated: true,
	}
	svc := handicaps.NewService(store)

	resp, err := svc.Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(resp.Recommendations) < 2 {
		t.Fatalf("want >= 2 recommendations, got %d", len(resp.Recommendations))
	}

	// Find actionable recs.
	var entries1, entries2 []handicaps.ApplyEntry
	for _, rec := range resp.Recommendations {
		if rec.RecToken != "" && rec.RecommendedHC != nil {
			e := handicaps.ApplyEntry{
				PlayerID:              rec.PlayerID,
				ExpectedAssignedHC:    rec.AssignedHC,
				ExpectedRecommendedHC: *rec.RecommendedHC,
				RecToken:              rec.RecToken,
			}
			entries1 = append(entries1, e)
		}
	}
	if len(entries1) < 2 {
		t.Skip("need 2 actionable players for order test")
	}
	// Reverse order.
	entries2 = []handicaps.ApplyEntry{entries1[1], entries1[0]}

	// Run first Apply.
	result1, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        entries1,
	})
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// Seed replay history with the first Apply's written rows.
	priorHistory := make([]handicaps.AppliedHistory, len(store.insertedHistoryRows))
	for i, row := range store.insertedHistoryRows {
		priorHistory[i] = handicaps.AppliedHistory{
			PlayerID:           row.PlayerID,
			PlayerNameSnapshot: row.PlayerNameSnapshot,
			OldHandicap:        row.OldHandicap,
			NewHandicap:        row.NewHandicap,
			RequestHash:        row.RequestHash,
		}
	}
	store.priorHistory = priorHistory
	store.insertedHistoryRows = nil
	_ = result1

	// Second Apply with reversed entries — should replay.
	result2, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        entries2,
	})
	if err != nil {
		t.Fatalf("second Apply (reversed): %v", err)
	}
	if !result2.Replayed {
		t.Error("want Replayed=true: same hash regardless of entry order")
	}
}

// TestApply_FloatNoisyExpectedHC_TreatedAsIdempotentReplay proves that a second
// Apply call carrying float-noisy expected handicap values (e.g. 0.9999999999
// instead of 1.0) is correctly identified as the same idempotent request.
// The request hash normalizes expected HCs to 0.01 before hashing, so both
// calls produce the same hash and the replay path fires.
func TestApply_FloatNoisyExpectedHC_TreatedAsIdempotentReplay(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	svc := handicaps.NewService(store)

	// First Apply with exact values.
	resp, err := svc.Recommendations(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	rec := resp.Recommendations[0]
	exactReq := handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              rec.PlayerID,
			ExpectedAssignedHC:    rec.AssignedHC,       // e.g. 1.0
			ExpectedRecommendedHC: *rec.RecommendedHC,   // e.g. 3.0
			RecToken:              tok,
		}},
	}
	result1, err := svc.Apply(context.Background(), 1, exactReq)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if len(result1.Applied) != 1 {
		t.Fatalf("first Apply: want 1 applied, got %d", len(result1.Applied))
	}

	// Seed prior history from the first Apply's written row.
	row := store.insertedHistoryRows[0]
	store.priorHistory = []handicaps.AppliedHistory{{
		PlayerID:           row.PlayerID,
		PlayerNameSnapshot: row.PlayerNameSnapshot,
		OldHandicap:        row.OldHandicap,
		NewHandicap:        row.NewHandicap,
		RequestHash:        row.RequestHash,
	}}
	store.insertedHistoryRows = nil

	// Second Apply — same semantic values but float-noisy expected fields.
	noisyReq := handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries: []handicaps.ApplyEntry{{
			PlayerID:              rec.PlayerID,
			ExpectedAssignedHC:    rec.AssignedHC - 0.0000000001,   // cent-equivalent noise
			ExpectedRecommendedHC: *rec.RecommendedHC + 0.0000000001,
			RecToken:              tok,
		}},
	}
	result2, err := svc.Apply(context.Background(), 1, noisyReq)
	if err != nil {
		t.Fatalf("second Apply (float-noisy): %v", err)
	}
	if !result2.Replayed {
		t.Error("want Replayed=true: float-noisy expected HCs must produce same request hash")
	}
	if len(store.insertedHistoryRows) != 0 {
		t.Error("want no writes on replay")
	}
}

// ============================================================================
// Error propagation
// ============================================================================

func TestApply_InsertHistoryError_ReturnsInternalError(t *testing.T) {
	store, tok := applyStoreWithToken(t)
	store.insertHistoryErr = errors.New("disk full")
	svc := handicaps.NewService(store)
	req := applyReqFor(t, svc, tok)
	// Reset insertHistoryErr after computing req so Recommendations call succeeds.
	store.insertHistoryErr = errors.New("disk full")

	_, err := svc.Apply(context.Background(), 1, req)
	if err == nil {
		t.Fatal("want error when InsertHandicapHistory fails")
	}
	if !domainerr.IsCategory(err, domainerr.Internal) {
		t.Errorf("want Internal, got %v", err)
	}
}

// TestApply_AllRejections_ErrorMessageContainsCount proves the rejection error
// message surface has useful information without requiring a specific string.
func TestApply_AllRejections_ErrorMessageContainsCount(t *testing.T) {
	store := &stubStore{
		seasonExists: true,
		rules:        handicaps.HandicapRuleRow{UpdateMethod: ptr("game_diff_average")},
		closedWeeks:  1,
		roster:       []handicaps.RosterEntry{{PlayerID: 1, PlayerName: "Bob", AssignedHC: 1.0}},
		// No racks → no_data or below_threshold rejection.
	}
	svc := handicaps.NewService(store)

	_, err := svc.Apply(context.Background(), 1, handicaps.ApplyRequest{
		ApplyRequestID: validUUID,
		Entries:        []handicaps.ApplyEntry{{PlayerID: 1, ExpectedAssignedHC: 1.0, RecToken: "any"}},
	})
	var re *handicaps.ApplyRejectionErr
	if !errors.As(err, &re) {
		t.Fatalf("want *ApplyRejectionErr, got %T: %v", err, err)
	}
	msg := re.Error()
	if !strings.Contains(msg, "1") {
		t.Errorf("error message should contain count: %q", msg)
	}
}
