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

// ── ListLineupPlans ───────────────────────────────────────────────────────────

func TestLineupService_ListLineupPlans_ReturnsEmptySliceWhenNone(t *testing.T) {
	svc := matches.NewLineupService(&stubLineupStore{listResult: nil})
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
	svc := matches.NewLineupService(stub)
	svc.ListLineupPlans(context.Background(), matches.ListLineupPlansRequest{SeasonID: 42})
	if stub.lastListReq.SeasonID != 42 {
		t.Errorf("want season_id=42 forwarded, got %d", stub.lastListReq.SeasonID)
	}
}

func TestLineupService_ListLineupPlans_PassesWeekAndTeamFilters(t *testing.T) {
	stub := &stubLineupStore{listResult: []models.LineupPlan{}}
	svc := matches.NewLineupService(stub)
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
	svc := matches.NewLineupService(stub)
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
	svc := matches.NewLineupService(stub)
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
	svc := matches.NewLineupService(stub)
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
	svc := matches.NewLineupService(stub)
	if err := svc.DeleteLineupPlan(context.Background(), 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.lastDeleteID != 99 {
		t.Errorf("want id=99 forwarded, got %d", stub.lastDeleteID)
	}
}

func TestLineupService_DeleteLineupPlan_StoreErrorBecomesInternal(t *testing.T) {
	stub := &stubLineupStore{deleteErr: errors.New("constraint")}
	svc := matches.NewLineupService(stub)
	err := svc.DeleteLineupPlan(context.Background(), 1)
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Category != domainerr.Internal {
		t.Errorf("want Internal category, got %v", de.Category)
	}
}
