package seasons_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// ── ListByeRequests ───────────────────────────────────────────────────────────

func TestSeasonService_ListByeRequests_ReturnsByes(t *testing.T) {
	want := []models.ByeRequest{{ID: 3, TeamID: 10}}
	store := &stubSeasonStore{byeList: want}
	got, err := newSvc(store).ListByeRequests(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListByeRequests: %v", err)
	}
	if len(got) != 1 || got[0].ID != 3 {
		t.Errorf("want bye ID=3, got %v", got)
	}
}

func TestSeasonService_ListByeRequests_ReturnsEmptySliceWhenNone(t *testing.T) {
	store := &stubSeasonStore{byeList: nil}
	got, err := newSvc(store).ListByeRequests(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListByeRequests: %v", err)
	}
	if got == nil {
		t.Error("want empty slice, got nil")
	}
}

func TestSeasonService_ListByeRequests_StoreErrorPropagates(t *testing.T) {
	store := &stubSeasonStore{byeListErr: errors.New("db down")}
	_, err := newSvc(store).ListByeRequests(context.Background(), 1)
	if err == nil {
		t.Error("want error, got nil")
	}
}

// ── DeleteByeRequest ──────────────────────────────────────────────────────────

func TestSeasonService_DeleteByeRequest_SucceedsOnFound(t *testing.T) {
	store := &stubSeasonStore{}
	if err := newSvc(store).DeleteByeRequest(context.Background(), 1, 2); err != nil {
		t.Fatalf("DeleteByeRequest: %v", err)
	}
}

func TestSeasonService_DeleteByeRequest_NotFoundReturns404(t *testing.T) {
	store := &stubSeasonStore{deleteByeErr: fmt.Errorf("wrap: %w", seasons.ErrByeNotFound)}
	err := newSvc(store).DeleteByeRequest(context.Background(), 1, 99)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Errorf("want NotFound domainerr, got %v", err)
	}
}

func TestSeasonService_DeleteByeRequest_OtherErrorPropagates(t *testing.T) {
	store := &stubSeasonStore{deleteByeErr: errors.New("connection refused")}
	err := newSvc(store).DeleteByeRequest(context.Background(), 1, 2)
	if err == nil {
		t.Error("want error, got nil")
	}
	var de *domainerr.Err
	if errors.As(err, &de) {
		t.Errorf("want raw error, got domainerr %v", err)
	}
}

// ── ListSeasonTeams (service method lives in service.go) ──────────────────────

func TestSeasonService_ListSeasonTeams_ReturnsTeams(t *testing.T) {
	want := []models.SeasonTeam{{ID: 1, TeamID: 5, TeamName: "Alpha"}}
	store := &stubSeasonStore{seasonTeamList: want}
	got, err := newSvc(store).ListSeasonTeams(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListSeasonTeams: %v", err)
	}
	if len(got) != 1 || got[0].TeamID != 5 {
		t.Errorf("want team ID=5, got %v", got)
	}
}

func TestSeasonService_ListSeasonTeams_ReturnsEmptySliceWhenNone(t *testing.T) {
	store := &stubSeasonStore{seasonTeamList: nil}
	got, err := newSvc(store).ListSeasonTeams(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListSeasonTeams: %v", err)
	}
	if got == nil {
		t.Error("want empty slice, got nil")
	}
}
