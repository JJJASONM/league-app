package teams_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/teams"
	"league_app/models"
)

type stubTeamStore struct {
	listFn   func(ctx context.Context, leagueID *int64) ([]models.Team, error)
	getFn    func(ctx context.Context, id int64) (models.Team, error)
	createFn func(ctx context.Context, input teams.CreateTeamInput) (models.Team, error)
	updateFn func(ctx context.Context, id int64, input teams.UpdateTeamInput) error
	deleteFn func(ctx context.Context, id int64) error
}

func (s *stubTeamStore) ListTeams(ctx context.Context, leagueID *int64) ([]models.Team, error) {
	if s.listFn != nil {
		return s.listFn(ctx, leagueID)
	}
	return []models.Team{}, nil
}
func (s *stubTeamStore) GetTeam(ctx context.Context, id int64) (models.Team, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return models.Team{}, nil
}
func (s *stubTeamStore) CreateTeam(ctx context.Context, input teams.CreateTeamInput) (models.Team, error) {
	if s.createFn != nil {
		return s.createFn(ctx, input)
	}
	return models.Team{}, nil
}
func (s *stubTeamStore) UpdateTeam(ctx context.Context, id int64, input teams.UpdateTeamInput) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, id, input)
	}
	return nil
}
func (s *stubTeamStore) DeleteTeam(ctx context.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func newTeamSvc(store teams.TeamStore) *teams.TeamService {
	return teams.NewTeamService(store)
}

// ── CreateTeam ────────────────────────────────────────────────────────────────

func TestTeamService_CreateTeam_EmptyName_ReturnsError(t *testing.T) {
	svc := newTeamSvc(&stubTeamStore{})
	_, err := svc.CreateTeam(context.Background(), teams.CreateTeamInput{Name: "  ", LeagueID: 1})
	if err == nil {
		t.Fatal("want error for empty name, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != "TEAM_NAME_REQUIRED" {
		t.Errorf("want code TEAM_NAME_REQUIRED, got %q", de.Code)
	}
	if de.Category != domainerr.InvalidInput {
		t.Errorf("want InvalidInput category, got %v", de.Category)
	}
}

func TestTeamService_CreateTeam_ZeroLeagueID_ReturnsError(t *testing.T) {
	svc := newTeamSvc(&stubTeamStore{})
	_, err := svc.CreateTeam(context.Background(), teams.CreateTeamInput{Name: "Red Team", LeagueID: 0})
	if err == nil {
		t.Fatal("want error for zero league_id, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != "TEAM_LEAGUE_REQUIRED" {
		t.Errorf("want code TEAM_LEAGUE_REQUIRED, got %q", de.Code)
	}
	if de.Category != domainerr.InvalidInput {
		t.Errorf("want InvalidInput category, got %v", de.Category)
	}
}

func TestTeamService_CreateTeam_ValidInput_DelegatesToStore(t *testing.T) {
	var received teams.CreateTeamInput
	svc := newTeamSvc(&stubTeamStore{
		createFn: func(_ context.Context, input teams.CreateTeamInput) (models.Team, error) {
			received = input
			return models.Team{ID: 5}, nil
		},
	})
	got, err := svc.CreateTeam(context.Background(), teams.CreateTeamInput{Name: "Blue Team", LeagueID: 3})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if got.ID != 5 {
		t.Errorf("want ID=5, got %d", got.ID)
	}
	if received.Name != "Blue Team" {
		t.Errorf("want Name=Blue Team passed to store, got %q", received.Name)
	}
	if received.LeagueID != 3 {
		t.Errorf("want LeagueID=3 passed to store, got %d", received.LeagueID)
	}
}

// ── UpdateTeam ────────────────────────────────────────────────────────────────

func TestTeamService_UpdateTeam_DelegatesToStore(t *testing.T) {
	var receivedID int64
	svc := newTeamSvc(&stubTeamStore{
		updateFn: func(_ context.Context, id int64, _ teams.UpdateTeamInput) error {
			receivedID = id
			return nil
		},
	})
	if err := svc.UpdateTeam(context.Background(), 7, teams.UpdateTeamInput{Name: "New Name"}); err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}
	if receivedID != 7 {
		t.Errorf("want id=7, got %d", receivedID)
	}
}

// ── DeleteTeam ────────────────────────────────────────────────────────────────

func TestTeamService_DeleteTeam_DelegatesToStore(t *testing.T) {
	called := false
	svc := newTeamSvc(&stubTeamStore{
		deleteFn: func(_ context.Context, id int64) error {
			called = true
			return nil
		},
	})
	if err := svc.DeleteTeam(context.Background(), 3); err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}
	if !called {
		t.Error("want store.DeleteTeam called, was not")
	}
}

// ── ListTeams ─────────────────────────────────────────────────────────────────

func TestTeamService_ListTeams_DelegatesToStore(t *testing.T) {
	want := []models.Team{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}
	svc := newTeamSvc(&stubTeamStore{
		listFn: func(_ context.Context, _ *int64) ([]models.Team, error) {
			return want, nil
		},
	})
	got, err := svc.ListTeams(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 teams, got %d", len(got))
	}
}

func TestTeamService_ListTeams_LeagueFilterPassedThrough(t *testing.T) {
	var receivedLeagueID *int64
	lid := int64(42)
	svc := newTeamSvc(&stubTeamStore{
		listFn: func(_ context.Context, leagueID *int64) ([]models.Team, error) {
			receivedLeagueID = leagueID
			return []models.Team{}, nil
		},
	})
	if _, err := svc.ListTeams(context.Background(), &lid); err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if receivedLeagueID == nil || *receivedLeagueID != 42 {
		t.Errorf("want leagueID=42 passed to store, got %v", receivedLeagueID)
	}
}

// ── GetTeam ───────────────────────────────────────────────────────────────────

func TestTeamService_GetTeam_DelegatesToStore(t *testing.T) {
	want := models.Team{ID: 99, Name: "Gamma"}
	svc := newTeamSvc(&stubTeamStore{
		getFn: func(_ context.Context, id int64) (models.Team, error) {
			if id != 99 {
				t.Errorf("want id=99, got %d", id)
			}
			return want, nil
		},
	})
	got, err := svc.GetTeam(context.Background(), 99)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.Name != "Gamma" {
		t.Errorf("want Name=Gamma, got %q", got.Name)
	}
}

func TestTeamService_GetTeam_PropagatesNotFound(t *testing.T) {
	svc := newTeamSvc(&stubTeamStore{
		getFn: func(_ context.Context, _ int64) (models.Team, error) {
			return models.Team{}, teams.ErrNotFound
		},
	})
	_, err := svc.GetTeam(context.Background(), 9999)
	if !errors.Is(err, teams.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
