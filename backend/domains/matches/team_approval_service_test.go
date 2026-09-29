package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
)

func playerID(v int64) *int64 { return &v }

func approvedSide(rev int) matches.TeamSideApprovalState {
	r := rev
	return matches.TeamSideApprovalState{State: matches.ApprovalStateApproved, StateScoreRevision: &r}
}

func pendingSide() matches.TeamSideApprovalState {
	return matches.TeamSideApprovalState{State: matches.ApprovalStatePending}
}

// --- ApproveTeamSide ---

func TestApproveTeamSide_InvalidSide_ReturnsInvalidInput(t *testing.T) {
	svc := newTestRoundSvc(&stubRoundStore{})
	err := svc.ApproveTeamSide(context.Background(), 1, "north", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalInvalidSide, domainerr.InvalidInput)
}

func TestApproveTeamSide_NoLinkedPlayer_ReturnsIneligible(t *testing.T) {
	svc := newTestRoundSvc(&stubRoundStore{})
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{})
	wantDomainErr(t, err, matches.CodeApprovalIneligiblePlayer, domainerr.Unprocessable)
}

func TestApproveTeamSide_MatchNotFound_ReturnsNotFound(t *testing.T) {
	store := &stubRoundStore{approvalState: matches.MatchApprovalState{Exists: false}}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeMatchNotFound, domainerr.NotFound)
}

// The following tests exercise mapTeamActionGuardErr -- season/week/
// processed/admin-accepted/not-scored/stale-revision are no longer decided
// by a service-layer pre-check (removed per PM's atomicity correction: a
// pre-transaction check cannot close the race the store's single
// conditional UPDATE closes atomically). The service now delegates
// entirely to the store's guarded write and maps whatever sentinel comes
// back -- these tests simulate that by setting the stub's error field
// directly, exactly as the real SQLite guard would return it after losing
// a race.

func TestApproveTeamSide_SeasonClosed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState:      matches.MatchApprovalState{Exists: true, Completed: true},
		eligibilityResult:  true,
		approveTeamSideErr: matches.ErrGuardSeasonClosed,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, "SEASON_CLOSED", domainerr.Conflict)
}

func TestApproveTeamSide_WeekClosed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState:      matches.MatchApprovalState{Exists: true, Completed: true},
		eligibilityResult:  true,
		approveTeamSideErr: matches.ErrGuardWeekClosed,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, "WEEK_CLOSED", domainerr.Conflict)
}

func TestApproveTeamSide_Processed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState:      matches.MatchApprovalState{Exists: true, Completed: true},
		eligibilityResult:  true,
		approveTeamSideErr: matches.ErrGuardProcessed,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeMatchProcessed, domainerr.Conflict)
}

func TestApproveTeamSide_AdminAlreadyAccepted_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState:      matches.MatchApprovalState{Exists: true, Completed: true},
		eligibilityResult:  true,
		approveTeamSideErr: matches.ErrGuardAdminAccepted,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeMatchAdminAccepted, domainerr.Conflict)
}

func TestApproveTeamSide_NotScored_ReturnsUnprocessable(t *testing.T) {
	store := &stubRoundStore{
		approvalState:      matches.MatchApprovalState{Exists: true, Completed: false},
		eligibilityResult:  true,
		approveTeamSideErr: matches.ErrGuardNotScored,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeMatchNotScored, domainerr.Unprocessable)
}

func TestApproveTeamSide_StaleRevision_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState:      matches.MatchApprovalState{Exists: true, Completed: true, ScoreRevision: 3},
		eligibilityResult:  true,
		approveTeamSideErr: matches.ErrApprovalRevisionStale,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 1, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalRevisionStale, domainerr.Conflict)
}

func TestApproveTeamSide_IneligiblePlayer_ReturnsUnprocessable(t *testing.T) {
	store := &stubRoundStore{
		approvalState:     matches.MatchApprovalState{Exists: true, Completed: true},
		eligibilityResult: false,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalIneligiblePlayer, domainerr.Unprocessable)
}

func TestApproveTeamSide_RaceLostAtStoreLayer_MapsToRevisionStale(t *testing.T) {
	store := &stubRoundStore{
		approvalState:      matches.MatchApprovalState{Exists: true, Completed: true},
		eligibilityResult:  true,
		approveTeamSideErr: matches.ErrApprovalRevisionStale,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalRevisionStale, domainerr.Conflict)
}

func TestApproveTeamSide_Success_DelegatesToStore(t *testing.T) {
	store := &stubRoundStore{
		approvalState:     matches.MatchApprovalState{Exists: true, Completed: true, ScoreRevision: 2},
		eligibilityResult: true,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveTeamSide(context.Background(), 1, "away", 2, matches.ApprovalActor{PlayerID: playerID(5), UserID: userID(9), NameSnapshot: "Eve"})
	if err != nil {
		t.Fatalf("want success, got: %v", err)
	}
	if store.lastApproveSide != "away" || store.lastApproveRevision != 2 {
		t.Errorf("want store.ApproveTeamSide called with side=away revision=2, got side=%s revision=%d", store.lastApproveSide, store.lastApproveRevision)
	}
}

// --- WithdrawTeamApproval ---

func TestWithdrawTeamApproval_NotCurrentlyApproved_ReturnsInvalidTransition(t *testing.T) {
	store := &stubRoundStore{
		approvalState:     matches.MatchApprovalState{Exists: true, Completed: true, HomeSide: pendingSide()},
		eligibilityResult: true,
		withdrawTeamErr:   matches.ErrGuardInvalidTransition,
	}
	svc := newTestRoundSvc(store)
	err := svc.WithdrawTeamApproval(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalInvalidTransition, domainerr.Conflict)
}

func TestWithdrawTeamApproval_AnyEligibleTeammate_NotJustOriginalApprover_Succeeds(t *testing.T) {
	store := &stubRoundStore{
		approvalState:     matches.MatchApprovalState{Exists: true, Completed: true, ScoreRevision: 0, HomeSide: approvedSide(0)},
		eligibilityResult: true,
	}
	svc := newTestRoundSvc(store)
	// A different player (id 77) than whoever originally approved withdraws.
	err := svc.WithdrawTeamApproval(context.Background(), 1, "home", 0, matches.ApprovalActor{PlayerID: playerID(77)})
	if err != nil {
		t.Fatalf("want any eligible teammate able to withdraw, got: %v", err)
	}
	if store.lastWithdrawSide != "home" {
		t.Errorf("want store.WithdrawTeamApproval called for home, got %s", store.lastWithdrawSide)
	}
}

func TestWithdrawTeamApproval_StaleRevision_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState:     matches.MatchApprovalState{Exists: true, Completed: true, ScoreRevision: 4, HomeSide: approvedSide(4)},
		eligibilityResult: true,
		withdrawTeamErr:   matches.ErrApprovalRevisionStale,
	}
	svc := newTestRoundSvc(store)
	err := svc.WithdrawTeamApproval(context.Background(), 1, "home", 1, matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalRevisionStale, domainerr.Conflict)
}

// --- RequestCorrection ---

func TestRequestCorrection_EmptyNote_ReturnsInvalidInput(t *testing.T) {
	svc := newTestRoundSvc(&stubRoundStore{})
	err := svc.RequestCorrection(context.Background(), 1, "home", 0, "", matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalNoteRequired, domainerr.InvalidInput)
}

func TestRequestCorrection_WhitespaceOnlyNote_ReturnsInvalidInput(t *testing.T) {
	svc := newTestRoundSvc(&stubRoundStore{})
	err := svc.RequestCorrection(context.Background(), 1, "home", 0, "   \t  ", matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeApprovalNoteRequired, domainerr.InvalidInput)
}

func TestRequestCorrection_NotScored_ReturnsUnprocessable(t *testing.T) {
	store := &stubRoundStore{
		approvalState:        matches.MatchApprovalState{Exists: true, Completed: false},
		eligibilityResult:    true,
		requestCorrectionErr: matches.ErrGuardNotScored,
	}
	svc := newTestRoundSvc(store)
	err := svc.RequestCorrection(context.Background(), 1, "home", 0, "something's wrong", matches.ApprovalActor{PlayerID: playerID(1)})
	wantDomainErr(t, err, matches.CodeMatchNotScored, domainerr.Unprocessable)
}

func TestRequestCorrection_Success_RecordsNote(t *testing.T) {
	store := &stubRoundStore{
		approvalState:     matches.MatchApprovalState{Exists: true, Completed: true},
		eligibilityResult: true,
	}
	svc := newTestRoundSvc(store)
	err := svc.RequestCorrection(context.Background(), 1, "away", 0, "game 3 score is wrong", matches.ApprovalActor{PlayerID: playerID(3)})
	if err != nil {
		t.Fatalf("want success, got: %v", err)
	}
	if store.lastCorrectionNote != "game 3 score is wrong" {
		t.Errorf("want note passed through, got %q", store.lastCorrectionNote)
	}
}

// --- AdminAcceptMatch ---

func TestAdminAcceptMatch_NotFound_ReturnsNotFound(t *testing.T) {
	store := &stubRoundStore{approvalState: matches.MatchApprovalState{Exists: false}}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	wantDomainErr(t, err, matches.CodeMatchNotFound, domainerr.NotFound)
}

func TestAdminAcceptMatch_NotScored_ReturnsUnprocessable(t *testing.T) {
	store := &stubRoundStore{approvalState: matches.MatchApprovalState{Exists: true, Completed: false}}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	wantDomainErr(t, err, matches.CodeMatchNotScored, domainerr.Unprocessable)
}

func TestAdminAcceptMatch_AlreadyProcessed_ReturnsConflict(t *testing.T) {
	processedAt := "2026-01-01"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ProcessedAt: &processedAt},
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	wantDomainErr(t, err, matches.CodeMatchAlreadyProcessed, domainerr.Conflict)
}

func TestAdminAcceptMatch_BothApproved_NoOverrideNeeded_RecordsAdminAccepted(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: approvedSide(0), AwaySide: approvedSide(0),
		},
	}
	svc := newTestRoundSvc(store)
	uid := int64(1)
	if err := svc.AdminAcceptMatch(context.Background(), 1, &uid, "looks good", false, 0); err != nil {
		t.Fatalf("want success when both sides already approved, got: %v", err)
	}
	if store.lastAdminAcceptanceEvent.EventType != "admin_accepted" {
		t.Errorf("want admin_accepted event, got %q", store.lastAdminAcceptanceEvent.EventType)
	}
	if store.lastGuardedAcceptRevision != 0 ||
		store.lastGuardedAcceptHomePinned.State != matches.ApprovalStateApproved ||
		store.lastGuardedAcceptAwayPinned.State != matches.ApprovalStateApproved {
		t.Errorf("want GuardedAdminAccept called with revision=0 and both pinned states approved, got revision=%d home=%s away=%s",
			store.lastGuardedAcceptRevision, store.lastGuardedAcceptHomePinned.State, store.lastGuardedAcceptAwayPinned.State)
	}
	if store.lastGuardedAcceptHomePinned.StateScoreRevision == nil || *store.lastGuardedAcceptHomePinned.StateScoreRevision != 0 {
		t.Errorf("want pinned home StateScoreRevision=0, got %v", store.lastGuardedAcceptHomePinned.StateScoreRevision)
	}
}

func TestAdminAcceptMatch_BothApprovedAtNonZeroRevision_Succeeds(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 7,
			HomeSide: approvedSide(7), AwaySide: approvedSide(7),
		},
	}
	svc := newTestRoundSvc(store)
	uid := int64(1)
	if err := svc.AdminAcceptMatch(context.Background(), 1, &uid, "fine", false, 7); err != nil {
		t.Fatalf("want success when both sides approved at the exact expected (non-zero) revision, got: %v", err)
	}
	if store.lastAdminAcceptanceEvent.EventType != "admin_accepted" {
		t.Errorf("want admin_accepted event, got %q", store.lastAdminAcceptanceEvent.EventType)
	}
}

// TestAdminAcceptMatch_ApprovedLabelWithNilRevision_RequiresOverride and
// TestAdminAcceptMatch_ApprovedLabelWithStaleRevision_RequiresOverride
// cover PM's correction 1 directly: a state-label check alone (State ==
// "approved") is not sufficient -- sideApprovedAtRevision additionally
// requires StateScoreRevision to be non-nil and equal to expectedRevision.
// Both scenarios below have State == "approved" on both sides (the OLD,
// insufficient check would have let normal acceptance proceed) but must
// still require override.

func TestAdminAcceptMatch_ApprovedLabelWithNilRevision_RequiresOverride(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: matches.TeamSideApprovalState{State: matches.ApprovalStateApproved, StateScoreRevision: nil},
			AwaySide: approvedSide(0),
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	wantDomainErr(t, err, matches.CodeApprovalOverrideRequired, domainerr.Conflict)
	if store.lastAdminAcceptanceEvent.EventType == "admin_accepted" {
		t.Error("want no normal admin_accepted event when a side's approval has a nil state-score-revision")
	}
}

func TestAdminAcceptMatch_ApprovedLabelWithStaleRevision_RequiresOverride(t *testing.T) {
	staleRev := 3
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 5,
			HomeSide: matches.TeamSideApprovalState{State: matches.ApprovalStateApproved, StateScoreRevision: &staleRev},
			AwaySide: approvedSide(5),
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 5)
	wantDomainErr(t, err, matches.CodeApprovalOverrideRequired, domainerr.Conflict)
	if store.lastAdminAcceptanceEvent.EventType == "admin_accepted" {
		t.Error("want no normal admin_accepted event when a side's approval revision is stale")
	}
}

func TestAdminAcceptMatch_MissingApproval_NoOverride_ReturnsOverrideRequired(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: approvedSide(0), AwaySide: pendingSide(),
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	wantDomainErr(t, err, matches.CodeApprovalOverrideRequired, domainerr.Conflict)
	if store.approvedByUserID != nil {
		t.Error("want zero writes when override is required but not given")
	}
}

func TestAdminAcceptMatch_MissingApproval_WithOverride_RecordsAdminOverride(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: approvedSide(0), AwaySide: pendingSide(),
		},
	}
	svc := newTestRoundSvc(store)
	uid := int64(1)
	if err := svc.AdminAcceptMatch(context.Background(), 1, &uid, "captains unavailable", true, 0); err != nil {
		t.Fatalf("want success with override=true, got: %v", err)
	}
	if store.lastAdminAcceptanceEvent.EventType != "admin_override" {
		t.Errorf("want admin_override event, got %q", store.lastAdminAcceptanceEvent.EventType)
	}
	if store.lastAdminAcceptanceEvent.HomeState != matches.ApprovalStateApproved || store.lastAdminAcceptanceEvent.AwayState != matches.ApprovalStatePending {
		t.Errorf("want event to snapshot both sides' actual state, got home=%s away=%s",
			store.lastAdminAcceptanceEvent.HomeState, store.lastAdminAcceptanceEvent.AwayState)
	}
}

func TestAdminAcceptMatch_StaleExpectedRevision_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 5,
			HomeSide: approvedSide(5), AwaySide: approvedSide(5),
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 1)
	wantDomainErr(t, err, matches.CodeApprovalRevisionStale, domainerr.Conflict)
	if store.lastAdminAcceptanceEvent.EventType != "" {
		t.Error("want zero writes on a stale expected revision")
	}
}

// TestAdminAcceptMatch_TeamStateChangedAtGuard_ReturnsConflict simulates the
// real SQLite guard (GuardedAdminAccept) detecting that a team side's state
// changed in the narrow window between AdminAcceptMatch's initial read
// (which decided "both approved, no override needed") and its guarded
// write -- e.g. a withdrawal landed in between. The decision made from
// stale data must not silently succeed.
func TestAdminAcceptMatch_TeamStateChangedAtGuard_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: approvedSide(0), AwaySide: approvedSide(0),
		},
		guardedAdminAcceptErr: matches.ErrGuardTeamStateChanged,
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	wantDomainErr(t, err, matches.CodeApprovalTeamStateChanged, domainerr.Conflict)
	if store.lastAdminAcceptanceEvent.EventType != "" {
		t.Error("want zero writes when the guard detects a team-state race")
	}
}

// TestAdminAcceptMatch_LockStateChangedAtGuard_ReturnsConflict simulates a
// concurrent week close/season close/processing landing between
// AdminAcceptMatch's initial read and its guarded write.
func TestAdminAcceptMatch_LockStateChangedAtGuard_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: approvedSide(0), AwaySide: approvedSide(0),
		},
		guardedAdminAcceptErr: matches.ErrGuardWeekClosed,
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	wantDomainErr(t, err, "WEEK_CLOSED", domainerr.Conflict)
	if store.lastAdminAcceptanceEvent.EventType != "" {
		t.Error("want zero writes when the guard detects a lock-state race")
	}
}

// TestAdminAcceptMatch_BothApprovedStaleRevision_ReturnsConflict covers the
// case where both sides show "approved" in the initial read but that
// approval belongs to a revision the fast-fail check already rejects as
// stale before the guard is ever attempted.
func TestAdminAcceptMatch_BothApprovedStaleRevision_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 9,
			HomeSide: approvedSide(9), AwaySide: approvedSide(9),
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 3)
	wantDomainErr(t, err, matches.CodeApprovalRevisionStale, domainerr.Conflict)
}

// TestAdminAcceptMatch_GuardFails_EventNeverWritten proves the event write
// only happens AFTER the guarded write succeeds -- if the guard fails, the
// event is never attempted, so a real transaction has nothing from this
// call to roll back beyond the guard's own (already-zero) writes.
func TestAdminAcceptMatch_GuardFails_EventNeverWritten(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: approvedSide(0), AwaySide: approvedSide(0),
		},
		guardedAdminAcceptErr: matches.ErrGuardProcessed,
	}
	svc := newTestRoundSvc(store)
	_ = svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	if store.lastAdminAcceptanceEvent.EventType != "" {
		t.Error("want the event never written when the guarded write itself fails")
	}
}

// TestAdminAcceptMatch_EventWriteFails_ReturnsError proves that if the
// guarded write succeeds but the subsequent event write fails, the overall
// call still returns an error -- in a real transaction (RunTx), that error
// rolls back the whole thing, including the guarded write that already
// "succeeded" from the store's point of view.
func TestAdminAcceptMatch_EventWriteFails_ReturnsError(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ScoreRevision: 0,
			HomeSide: approvedSide(0), AwaySide: approvedSide(0),
		},
		recordAdminEventErr: errors.New("simulated event write failure"),
	}
	svc := newTestRoundSvc(store)
	err := svc.AdminAcceptMatch(context.Background(), 1, nil, "", false, 0)
	if err == nil {
		t.Fatal("want an error when the event write fails after a successful guarded write")
	}
}

// --- BothSidesApproved ---

func TestBothSidesApproved_TrueWhenBothApprovedAtCurrentRevision(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, ScoreRevision: 2, HomeSide: approvedSide(2), AwaySide: approvedSide(2),
		},
	}
	svc := newTestRoundSvc(store)
	ok, err := svc.BothSidesApproved(context.Background(), 1)
	if err != nil {
		t.Fatalf("BothSidesApproved: %v", err)
	}
	if !ok {
		t.Error("want true when both sides approved at the current revision")
	}
}

func TestBothSidesApproved_FalseWhenOnlyOneApproved(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, ScoreRevision: 0, HomeSide: approvedSide(0), AwaySide: pendingSide(),
		},
	}
	svc := newTestRoundSvc(store)
	ok, err := svc.BothSidesApproved(context.Background(), 1)
	if err != nil {
		t.Fatalf("BothSidesApproved: %v", err)
	}
	if ok {
		t.Error("want false when only one side approved")
	}
}
