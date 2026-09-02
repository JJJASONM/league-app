package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// ── stub ──────────────────────────────────────────────────────────────────────

type stubLineupStore struct {
	listResult  []models.LineupPlan
	listErr     error
	saveErr     error
	deleteErr   error
	lastListReq matches.ListLineupPlansRequest
	lastSaveReq matches.SaveLineupRequest
	lastDeleteID int64

	getPlanResult  models.LineupPlan
	getPlanErr     error
	findMatchID    int64
	findMatchFound bool
	findMatchErr   error
	setSubResult   models.LineupPlan
	setSubErr      error
	lastSetSubReq  matches.SetSubstituteRequest
	clearSubResult models.LineupPlan
	clearSubErr    error
	lastClearSubID int64
}

func (s *stubLineupStore) ListLineupPlans(_ context.Context, req matches.ListLineupPlansRequest) ([]models.LineupPlan, error) {
	s.lastListReq = req
	return s.listResult, s.listErr
}

func (s *stubLineupStore) SaveTeamLineup(_ context.Context, req matches.SaveLineupRequest) error {
	s.lastSaveReq = req
	return s.saveErr
}

func (s *stubLineupStore) DeleteLineupPlan(_ context.Context, id int64) error {
	s.lastDeleteID = id
	return s.deleteErr
}

func (s *stubLineupStore) GetLineupPlan(_ context.Context, _ int64) (models.LineupPlan, error) {
	return s.getPlanResult, s.getPlanErr
}

func (s *stubLineupStore) FindMatchID(_ context.Context, _, _, _ int64) (int64, bool, error) {
	return s.findMatchID, s.findMatchFound, s.findMatchErr
}

func (s *stubLineupStore) SetSubstitute(_ context.Context, req matches.SetSubstituteRequest) (models.LineupPlan, error) {
	s.lastSetSubReq = req
	return s.setSubResult, s.setSubErr
}

func (s *stubLineupStore) ClearSubstitute(_ context.Context, id int64) (models.LineupPlan, error) {
	s.lastClearSubID = id
	return s.clearSubResult, s.clearSubErr
}

// stubMatchLockChecker is a stub matches.MatchLockChecker. All fields default
// to "nothing is locked," matching the common case of a match that hasn't
// been approved/processed/closed.
type stubMatchLockChecker struct {
	seasonClosed    bool
	seasonClosedErr error
	weekClosed      bool
	weekClosedErr   error
	approvalState   matches.MatchApprovalState
	approvalErr     error
}

func (s *stubMatchLockChecker) IsSeasonClosedForMatch(_ context.Context, _ int64) (bool, error) {
	return s.seasonClosed, s.seasonClosedErr
}

func (s *stubMatchLockChecker) IsWeekClosed(_ context.Context, _ int64) (bool, error) {
	return s.weekClosed, s.weekClosedErr
}

func (s *stubMatchLockChecker) GetMatchApprovalState(_ context.Context, _ int64) (matches.MatchApprovalState, error) {
	return s.approvalState, s.approvalErr
}

// newLineupSvc builds a LineupService with a permissive default lock checker
// (no match found for the team/week, so nothing is locked) unless the test
// needs to exercise a specific lock state.
func newLineupSvc(store matches.LineupStore) *matches.LineupService {
	return matches.NewLineupService(store, &stubMatchLockChecker{})
}

// ── ListLineupPlans ───────────────────────────────────────────────────────────

func TestLineupService_ListLineupPlans_ReturnsEmptySliceWhenNone(t *testing.T) {
	svc := newLineupSvc(&stubLineupStore{listResult: nil})
	plans, err := svc.ListLineupPlans(context.Background(), matches.ListLineupPlansRequest{SeasonID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plans == nil {
		t.Error("want non-nil empty slice, got nil")
	}
	if len(plans) != 0 {
		t.Errorf("want 0 plans, got %d", len(plans))
	}
}

func TestLineupService_ListLineupPlans_PassesSeasonIDToStore(t *testing.T) {
	stub := &stubLineupStore{listResult: []models.LineupPlan{}}
	svc := newLineupSvc(stub)
	svc.ListLineupPlans(context.Background(), matches.ListLineupPlansRequest{SeasonID: 42})
	if stub.lastListReq.SeasonID != 42 {
		t.Errorf("want season_id=42 forwarded, got %d", stub.lastListReq.SeasonID)
	}
}

func TestLineupService_ListLineupPlans_PassesWeekAndTeamFilters(t *testing.T) {
	stub := &stubLineupStore{listResult: []models.LineupPlan{}}
	svc := newLineupSvc(stub)
	svc.ListLineupPlans(context.Background(), matches.ListLineupPlansRequest{
		SeasonID:   1,
		WeekNumber: 3,
		TeamID:     7,
	})
	if stub.lastListReq.WeekNumber != 3 {
		t.Errorf("want week_number=3 forwarded, got %d", stub.lastListReq.WeekNumber)
	}
	if stub.lastListReq.TeamID != 7 {
		t.Errorf("want team_id=7 forwarded, got %d", stub.lastListReq.TeamID)
	}
}

func TestLineupService_ListLineupPlans_StoreErrorBecomesInternal(t *testing.T) {
	stub := &stubLineupStore{listErr: errors.New("db down")}
	svc := newLineupSvc(stub)
	_, err := svc.ListLineupPlans(context.Background(), matches.ListLineupPlansRequest{SeasonID: 1})
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
}

// ── SaveTeamLineup ────────────────────────────────────────────────────────────

func TestLineupService_SaveTeamLineup_PassesRequestToStore(t *testing.T) {
	stub := &stubLineupStore{}
	svc := newLineupSvc(stub)
	req := matches.SaveLineupRequest{
		SeasonID:   5,
		TeamID:     2,
		WeekNumber: 4,
		PlayerIDs:  []int64{10, 11, 12},
	}
	if err := svc.SaveTeamLineup(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.lastSaveReq.SeasonID != 5 || stub.lastSaveReq.TeamID != 2 || stub.lastSaveReq.WeekNumber != 4 {
		t.Errorf("request not forwarded correctly: %+v", stub.lastSaveReq)
	}
	if len(stub.lastSaveReq.PlayerIDs) != 3 {
		t.Errorf("want 3 player IDs forwarded, got %d", len(stub.lastSaveReq.PlayerIDs))
	}
}

func TestLineupService_SaveTeamLineup_StoreErrorBecomesInternal(t *testing.T) {
	stub := &stubLineupStore{saveErr: errors.New("tx failed")}
	svc := newLineupSvc(stub)
	err := svc.SaveTeamLineup(context.Background(), matches.SaveLineupRequest{})
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
}

// ── DeleteLineupPlan ──────────────────────────────────────────────────────────

func TestLineupService_DeleteLineupPlan_PassesIDToStore(t *testing.T) {
	stub := &stubLineupStore{}
	svc := newLineupSvc(stub)
	if err := svc.DeleteLineupPlan(context.Background(), 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.lastDeleteID != 99 {
		t.Errorf("want id=99 forwarded, got %d", stub.lastDeleteID)
	}
}

func TestLineupService_DeleteLineupPlan_StoreErrorBecomesInternal(t *testing.T) {
	stub := &stubLineupStore{deleteErr: errors.New("constraint")}
	svc := newLineupSvc(stub)
	err := svc.DeleteLineupPlan(context.Background(), 1)
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
}

// -- SetSubstitute -------------------------------------------------------------

func TestLineupService_SetSubstitute_ZeroSubstituteID_ReturnsError(t *testing.T) {
	svc := newLineupSvc(&stubLineupStore{})
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "SUB_PLAYER_REQUIRED" || de.Category != domainerr.InvalidInput {
		t.Fatalf("want SUB_PLAYER_REQUIRED InvalidInput, got %v", err)
	}
}

func TestLineupService_SetSubstitute_LineupPlanNotFound_ReturnsNotFound(t *testing.T) {
	stub := &stubLineupStore{getPlanErr: domainerr.New("LINEUP_PLAN_NOT_FOUND", domainerr.NotFound, "lineup plan not found")}
	svc := newLineupSvc(stub)
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 2})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestLineupService_SetSubstitute_SamePlayer_ReturnsError(t *testing.T) {
	stub := &stubLineupStore{getPlanResult: models.LineupPlan{PlayerID: 5}}
	svc := newLineupSvc(stub)
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 5})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "SUB_SAME_PLAYER" || de.Category != domainerr.InvalidInput {
		t.Fatalf("want SUB_SAME_PLAYER InvalidInput, got %v", err)
	}
}

func TestLineupService_SetSubstitute_SeasonClosed_ReturnsConflict(t *testing.T) {
	store := &stubLineupStore{
		getPlanResult:  models.LineupPlan{PlayerID: 5, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		findMatchID:    99,
		findMatchFound: true,
	}
	svc := matches.NewLineupService(store, &stubMatchLockChecker{seasonClosed: true})
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 6})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "SEASON_CLOSED" || de.Category != domainerr.Conflict {
		t.Fatalf("want SEASON_CLOSED Conflict, got %v", err)
	}
}

func TestLineupService_SetSubstitute_WeekClosed_ReturnsConflict(t *testing.T) {
	store := &stubLineupStore{
		getPlanResult:  models.LineupPlan{PlayerID: 5, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		findMatchID:    99,
		findMatchFound: true,
	}
	svc := matches.NewLineupService(store, &stubMatchLockChecker{weekClosed: true})
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 6})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "WEEK_CLOSED" || de.Category != domainerr.Conflict {
		t.Fatalf("want WEEK_CLOSED Conflict, got %v", err)
	}
}

func TestLineupService_SetSubstitute_MatchApproved_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-01-01"
	store := &stubLineupStore{
		getPlanResult:  models.LineupPlan{PlayerID: 5, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		findMatchID:    99,
		findMatchFound: true,
	}
	svc := matches.NewLineupService(store, &stubMatchLockChecker{
		approvalState: matches.MatchApprovalState{ApprovedAt: &approvedAt},
	})
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 6})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "MATCH_APPROVED" || de.Category != domainerr.Conflict {
		t.Fatalf("want MATCH_APPROVED Conflict, got %v", err)
	}
}

func TestLineupService_SetSubstitute_MatchProcessed_ReturnsConflict(t *testing.T) {
	processedAt := "2026-01-01"
	store := &stubLineupStore{
		getPlanResult:  models.LineupPlan{PlayerID: 5, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		findMatchID:    99,
		findMatchFound: true,
	}
	svc := matches.NewLineupService(store, &stubMatchLockChecker{
		approvalState: matches.MatchApprovalState{ProcessedAt: &processedAt},
	})
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 6})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "MATCH_PROCESSED" || de.Category != domainerr.Conflict {
		t.Fatalf("want MATCH_PROCESSED Conflict, got %v", err)
	}
}

func TestLineupService_SetSubstitute_NoMatchScheduled_AllowsChange(t *testing.T) {
	stub := &stubLineupStore{
		getPlanResult:  models.LineupPlan{ID: 1, PlayerID: 5, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		findMatchFound: false,
		setSubResult:   models.LineupPlan{ID: 1, PlayerID: 6, IsSub: true, SubForID: int64Ptr(5)},
	}
	svc := newLineupSvc(stub)
	got, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 6})
	if err != nil {
		t.Fatalf("SetSubstitute: %v", err)
	}
	if got.PlayerID != 6 || !got.IsSub {
		t.Errorf("want substitute applied, got %+v", got)
	}
	if stub.lastSetSubReq.OriginalPlayerID != 5 {
		t.Errorf("want OriginalPlayerID=5 passed to store, got %d", stub.lastSetSubReq.OriginalPlayerID)
	}
	if stub.lastSetSubReq.SubstitutePlayerID != 6 {
		t.Errorf("want SubstitutePlayerID=6 passed to store, got %d", stub.lastSetSubReq.SubstitutePlayerID)
	}
}

func TestLineupService_SetSubstitute_StoreUniqueConstraint_ReturnsConflict(t *testing.T) {
	stub := &stubLineupStore{
		getPlanResult: models.LineupPlan{PlayerID: 5, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		setSubErr:     errors.New("UNIQUE constraint failed: lineup_plans.season_id, lineup_plans.team_id, lineup_plans.week_number, lineup_plans.player_id"),
	}
	svc := newLineupSvc(stub)
	_, err := svc.SetSubstitute(context.Background(), matches.SetSubstituteRequest{LineupPlanID: 1, SubstitutePlayerID: 6})
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "SUB_ALREADY_IN_LINEUP" || de.Category != domainerr.Conflict {
		t.Fatalf("want SUB_ALREADY_IN_LINEUP Conflict, got %v", err)
	}
}

// -- ClearSubstitute -----------------------------------------------------------

func TestLineupService_ClearSubstitute_NotCurrentlySubstituted_ReturnsError(t *testing.T) {
	stub := &stubLineupStore{getPlanResult: models.LineupPlan{IsSub: false}}
	svc := newLineupSvc(stub)
	_, err := svc.ClearSubstitute(context.Background(), 1)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "SUB_NOT_ACTIVE" || de.Category != domainerr.InvalidInput {
		t.Fatalf("want SUB_NOT_ACTIVE InvalidInput, got %v", err)
	}
}

func TestLineupService_ClearSubstitute_WeekClosed_ReturnsConflict(t *testing.T) {
	store := &stubLineupStore{
		getPlanResult:  models.LineupPlan{IsSub: true, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		findMatchID:    99,
		findMatchFound: true,
	}
	svc := matches.NewLineupService(store, &stubMatchLockChecker{weekClosed: true})
	_, err := svc.ClearSubstitute(context.Background(), 1)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Code != "WEEK_CLOSED" || de.Category != domainerr.Conflict {
		t.Fatalf("want WEEK_CLOSED Conflict, got %v", err)
	}
}

func TestLineupService_ClearSubstitute_ValidInput_DelegatesToStore(t *testing.T) {
	stub := &stubLineupStore{
		getPlanResult:  models.LineupPlan{ID: 1, IsSub: true, SeasonID: 1, TeamID: 2, WeekNumber: 3},
		findMatchFound: false,
		clearSubResult: models.LineupPlan{ID: 1, PlayerID: 5, IsSub: false},
	}
	svc := newLineupSvc(stub)
	got, err := svc.ClearSubstitute(context.Background(), 1)
	if err != nil {
		t.Fatalf("ClearSubstitute: %v", err)
	}
	if got.IsSub {
		t.Error("want is_sub=false after clearing")
	}
	if stub.lastClearSubID != 1 {
		t.Errorf("want id=1 forwarded to store, got %d", stub.lastClearSubID)
	}
}

func int64Ptr(v int64) *int64 { return &v }
