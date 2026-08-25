package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

func wantDomainErr(t *testing.T, err error, wantCode string, wantCat domainerr.Category) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want *domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != wantCode {
		t.Errorf("want code %q, got %q", wantCode, de.Code)
	}
	if de.Category != wantCat {
		t.Errorf("want category %v, got %v", wantCat, de.Category)
	}
}

func userID(v int64) *int64 { return &v }

// --- ApproveMatch ---

func TestApproveMatch_NotFound_ReturnsNotFound(t *testing.T) {
	store := &stubRoundStore{approvalState: matches.MatchApprovalState{Exists: false}}
	svc := newTestRoundSvc(store)
	err := svc.ApproveMatch(context.Background(), 1, nil, "")
	wantDomainErr(t, err, matches.CodeMatchNotFound, domainerr.NotFound)
}

func TestApproveMatch_SeasonClosed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true},
		seasonClosed:  true,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveMatch(context.Background(), 1, nil, "")
	wantDomainErr(t, err, "SEASON_CLOSED", domainerr.Conflict)
}

func TestApproveMatch_WeekClosed_ReturnsConflict(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true},
		weekClosed:    true,
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveMatch(context.Background(), 1, nil, "")
	wantDomainErr(t, err, "WEEK_CLOSED", domainerr.Conflict)
}

func TestApproveMatch_NotScored_ReturnsUnprocessable(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: false},
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveMatch(context.Background(), 1, nil, "")
	wantDomainErr(t, err, matches.CodeMatchNotScored, domainerr.Unprocessable)
}

func TestApproveMatch_AlreadyProcessed_ReturnsConflict(t *testing.T) {
	processedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ProcessedAt: &processedAt,
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.ApproveMatch(context.Background(), 1, nil, "")
	wantDomainErr(t, err, matches.CodeMatchAlreadyProcessed, domainerr.Conflict)
}

func TestApproveMatch_Success_RecordsApproverAndNote(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true},
	}
	svc := newTestRoundSvc(store)
	if err := svc.ApproveMatch(context.Background(), 1, userID(7), "captain confirmed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.approvedByUserID == nil || *store.approvedByUserID != 7 {
		t.Errorf("want approvedByUserID=7, got %v", store.approvedByUserID)
	}
	if store.approvedNote != "captain confirmed" {
		t.Errorf("want approvedNote=%q, got %q", "captain confirmed", store.approvedNote)
	}
}

func TestApproveMatch_Success_NilUserIDAllowed(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true},
	}
	svc := newTestRoundSvc(store)
	if err := svc.ApproveMatch(context.Background(), 1, nil, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.approvedByUserID != nil {
		t.Errorf("want nil approvedByUserID, got %v", store.approvedByUserID)
	}
}

// --- ProcessMatch ---

func TestProcessMatch_NotFound_ReturnsNotFound(t *testing.T) {
	store := &stubRoundStore{approvalState: matches.MatchApprovalState{Exists: false}}
	svc := newTestRoundSvc(store)
	err := svc.ProcessMatch(context.Background(), 1, nil)
	wantDomainErr(t, err, matches.CodeMatchNotFound, domainerr.NotFound)
}

func TestProcessMatch_SeasonClosed_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ApprovedAt: &approvedAt},
		seasonClosed:  true,
	}
	svc := newTestRoundSvc(store)
	err := svc.ProcessMatch(context.Background(), 1, nil)
	wantDomainErr(t, err, "SEASON_CLOSED", domainerr.Conflict)
}

func TestProcessMatch_WeekClosed_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ApprovedAt: &approvedAt},
		weekClosed:    true,
	}
	svc := newTestRoundSvc(store)
	err := svc.ProcessMatch(context.Background(), 1, nil)
	wantDomainErr(t, err, "WEEK_CLOSED", domainerr.Conflict)
}

func TestProcessMatch_NotApproved_ReturnsUnprocessable(t *testing.T) {
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true},
	}
	svc := newTestRoundSvc(store)
	err := svc.ProcessMatch(context.Background(), 1, nil)
	wantDomainErr(t, err, matches.CodeMatchNotApproved, domainerr.Unprocessable)
}

func TestProcessMatch_Success_RecordsProcessor(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ApprovedAt: &approvedAt},
	}
	svc := newTestRoundSvc(store)
	if err := svc.ProcessMatch(context.Background(), 1, userID(9)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.processedByUserID == nil || *store.processedByUserID != 9 {
		t.Errorf("want processedByUserID=9, got %v", store.processedByUserID)
	}
}

// --- UnapproveMatch ---

func TestUnapproveMatch_NotFound_ReturnsNotFound(t *testing.T) {
	store := &stubRoundStore{approvalState: matches.MatchApprovalState{Exists: false}}
	svc := newTestRoundSvc(store)
	err := svc.UnapproveMatch(context.Background(), 1)
	wantDomainErr(t, err, matches.CodeMatchNotFound, domainerr.NotFound)
}

func TestUnapproveMatch_WeekClosed_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ApprovedAt: &approvedAt},
		weekClosed:    true,
	}
	svc := newTestRoundSvc(store)
	err := svc.UnapproveMatch(context.Background(), 1)
	wantDomainErr(t, err, "WEEK_CLOSED", domainerr.Conflict)
}

func TestUnapproveMatch_AlreadyProcessed_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	processedAt := "2026-08-02 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ApprovedAt: &approvedAt, ProcessedAt: &processedAt,
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.UnapproveMatch(context.Background(), 1)
	wantDomainErr(t, err, matches.CodeMatchAlreadyProcessed, domainerr.Conflict)
}

func TestUnapproveMatch_Success_ClearsApproval(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ApprovedAt: &approvedAt},
	}
	svc := newTestRoundSvc(store)
	if err := svc.UnapproveMatch(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.unapproveCalled {
		t.Error("want UnapproveMatch to be called on the store")
	}
}

// --- UnprocessMatch ---

func TestUnprocessMatch_NotFound_ReturnsNotFound(t *testing.T) {
	store := &stubRoundStore{approvalState: matches.MatchApprovalState{Exists: false}}
	svc := newTestRoundSvc(store)
	err := svc.UnprocessMatch(context.Background(), 1)
	wantDomainErr(t, err, matches.CodeMatchNotFound, domainerr.NotFound)
}

func TestUnprocessMatch_WeekClosed_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	processedAt := "2026-08-02 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ApprovedAt: &approvedAt, ProcessedAt: &processedAt,
		},
		weekClosed: true,
	}
	svc := newTestRoundSvc(store)
	err := svc.UnprocessMatch(context.Background(), 1)
	wantDomainErr(t, err, "WEEK_CLOSED", domainerr.Conflict)
}

func TestUnprocessMatch_Success_ClearsProcessingPreservesApproval(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	processedAt := "2026-08-02 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ApprovedAt: &approvedAt, ProcessedAt: &processedAt,
		},
	}
	svc := newTestRoundSvc(store)
	if err := svc.UnprocessMatch(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.unprocessCalled {
		t.Error("want UnprocessMatch to be called on the store")
	}
	if store.unapproveCalled {
		t.Error("UnprocessMatch must not also clear approval -- approval is a separate, explicit action")
	}
}

// --- Score edits blocked after approval/processing ---

func TestSaveRounds_Approved_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ApprovedAt: &approvedAt},
	}
	svc := newTestRoundSvc(store)
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{MatchID: 1})
	wantDomainErr(t, err, matches.CodeMatchApproved, domainerr.Conflict)
}

func TestSaveRounds_Processed_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	processedAt := "2026-08-02 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ApprovedAt: &approvedAt, ProcessedAt: &processedAt,
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{MatchID: 1})
	wantDomainErr(t, err, matches.CodeMatchProcessed, domainerr.Conflict)
}

func TestSubmitResults_Approved_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{Exists: true, Completed: true, ApprovedAt: &approvedAt},
	}
	svc := newTestRoundSvc(store)
	err := svc.SubmitResults(context.Background(), 1, nil)
	wantDomainErr(t, err, matches.CodeMatchApproved, domainerr.Conflict)
}

func TestClearResults_Processed_ReturnsConflict(t *testing.T) {
	approvedAt := "2026-08-01 00:00:00"
	processedAt := "2026-08-02 00:00:00"
	store := &stubRoundStore{
		approvalState: matches.MatchApprovalState{
			Exists: true, Completed: true, ApprovedAt: &approvedAt, ProcessedAt: &processedAt,
		},
	}
	svc := newTestRoundSvc(store)
	err := svc.ClearResults(context.Background(), 1)
	wantDomainErr(t, err, matches.CodeMatchProcessed, domainerr.Conflict)
}

// TestSaveRounds_NotApprovedOrProcessed_Unaffected proves the new
// checkMatchEditable gate does not interfere with the ordinary, most-common
// case: a match that has never been approved (approvalState defaults to
// Exists:false), matching every pre-existing SaveRounds test in this package.
func TestSaveRounds_NotApprovedOrProcessed_Unaffected(t *testing.T) {
	store := &stubRoundStore{
		matchCtx:  matches.MatchContext{SeasonID: 1, HomeTeamID: 10, AwayTeamID: 20},
		playerHCs: map[int64]float64{1: 1.0, 2: 2.0},
	}
	svc := newTestRoundSvc(store)
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID: 1,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: 1, AwayPlayerID: 2, Game1Home: 10, Game1Away: 5},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
