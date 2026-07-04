package leagues_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/leagues"
	"league_app/models"
)

// stubLeagueStore implements leagues.LeagueStore using configurable function fields.
type stubLeagueStore struct {
	listFn   func(ctx context.Context) ([]models.League, error)
	getFn    func(ctx context.Context, id int64) (models.League, error)
	createFn func(ctx context.Context, input leagues.CreateLeagueInput) (models.League, error)
	updateFn func(ctx context.Context, id int64, input leagues.UpdateLeagueInput) error
	deleteFn func(ctx context.Context, id int64) error
}

func (s *stubLeagueStore) ListLeagues(ctx context.Context) ([]models.League, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return []models.League{}, nil
}
func (s *stubLeagueStore) GetLeague(ctx context.Context, id int64) (models.League, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return models.League{}, nil
}
func (s *stubLeagueStore) CreateLeague(ctx context.Context, input leagues.CreateLeagueInput) (models.League, error) {
	if s.createFn != nil {
		return s.createFn(ctx, input)
	}
	return models.League{}, nil
}
func (s *stubLeagueStore) UpdateLeague(ctx context.Context, id int64, input leagues.UpdateLeagueInput) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, id, input)
	}
	return nil
}
func (s *stubLeagueStore) DeleteLeague(ctx context.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func newSvc(store leagues.LeagueStore) *leagues.LeagueService {
	return leagues.NewLeagueService(store)
}

// ── ListLeagues ───────────────────────────────────────────────────────────────

func TestLeagueService_ListLeagues_DelegatesToStore(t *testing.T) {
	want := []models.League{{ID: 1, Name: "Monday"}, {ID: 2, Name: "Tuesday"}}
	svc := newSvc(&stubLeagueStore{
		listFn: func(_ context.Context) ([]models.League, error) { return want, nil },
	})
	got, err := svc.ListLeagues(context.Background())
	if err != nil {
		t.Fatalf("ListLeagues: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 leagues, got %d", len(got))
	}
}

func TestLeagueService_ListLeagues_PropagatesStoreError(t *testing.T) {
	svc := newSvc(&stubLeagueStore{
		listFn: func(_ context.Context) ([]models.League, error) {
			return nil, errors.New("db offline")
		},
	})
	_, err := svc.ListLeagues(context.Background())
	if err == nil {
		t.Error("want error, got nil")
	}
}

// ── GetLeague ─────────────────────────────────────────────────────────────────

func TestLeagueService_GetLeague_DelegatesToStore(t *testing.T) {
	want := models.League{ID: 42, Name: "Wednesday"}
	svc := newSvc(&stubLeagueStore{
		getFn: func(_ context.Context, id int64) (models.League, error) {
			if id != 42 {
				t.Errorf("want id=42, got %d", id)
			}
			return want, nil
		},
	})
	got, err := svc.GetLeague(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetLeague: %v", err)
	}
	if got.Name != "Wednesday" {
		t.Errorf("want Name=Wednesday, got %q", got.Name)
	}
}

func TestLeagueService_GetLeague_PropagatesNotFound(t *testing.T) {
	svc := newSvc(&stubLeagueStore{
		getFn: func(_ context.Context, _ int64) (models.League, error) {
			return models.League{}, leagues.ErrNotFound
		},
	})
	_, err := svc.GetLeague(context.Background(), 9999)
	if !errors.Is(err, leagues.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── CreateLeague ──────────────────────────────────────────────────────────────

func TestLeagueService_CreateLeague_EmptyName_ReturnsError(t *testing.T) {
	svc := newSvc(&stubLeagueStore{})
	_, err := svc.CreateLeague(context.Background(), leagues.CreateLeagueInput{Name: "  "})
	if err == nil {
		t.Fatal("want error for empty name, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != "LEAGUE_NAME_REQUIRED" {
		t.Errorf("want code LEAGUE_NAME_REQUIRED, got %q", de.Code)
	}
	if de.Category != domainerr.InvalidInput {
		t.Errorf("want InvalidInput category, got %v", de.Category)
	}
}

func TestLeagueService_CreateLeague_EmptyGameFormat_DefaultsTo8ball(t *testing.T) {
	var received leagues.CreateLeagueInput
	svc := newSvc(&stubLeagueStore{
		createFn: func(_ context.Context, input leagues.CreateLeagueInput) (models.League, error) {
			received = input
			return models.League{}, nil
		},
	})
	_, err := svc.CreateLeague(context.Background(), leagues.CreateLeagueInput{Name: "Thursday"})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	if received.GameFormat != "8ball" {
		t.Errorf("want GameFormat=8ball, got %q", received.GameFormat)
	}
}

func TestLeagueService_CreateLeague_ExplicitGameFormat_PassedThrough(t *testing.T) {
	var received leagues.CreateLeagueInput
	svc := newSvc(&stubLeagueStore{
		createFn: func(_ context.Context, input leagues.CreateLeagueInput) (models.League, error) {
			received = input
			return models.League{}, nil
		},
	})
	_, err := svc.CreateLeague(context.Background(), leagues.CreateLeagueInput{Name: "Friday", GameFormat: "9ball"})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	if received.GameFormat != "9ball" {
		t.Errorf("want GameFormat=9ball, got %q", received.GameFormat)
	}
}

func TestLeagueService_CreateLeague_ValidInput_DelegatesToStore(t *testing.T) {
	called := false
	svc := newSvc(&stubLeagueStore{
		createFn: func(_ context.Context, input leagues.CreateLeagueInput) (models.League, error) {
			called = true
			if input.Name != "Saturday" {
				t.Errorf("want Name=Saturday, got %q", input.Name)
			}
			return models.League{ID: 7, Name: input.Name, GameFormat: input.GameFormat}, nil
		},
	})
	got, err := svc.CreateLeague(context.Background(), leagues.CreateLeagueInput{Name: "Saturday"})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	if !called {
		t.Error("want store.CreateLeague called, was not")
	}
	if got.ID != 7 {
		t.Errorf("want ID=7, got %d", got.ID)
	}
}

// ── UpdateLeague ──────────────────────────────────────────────────────────────

func TestLeagueService_UpdateLeague_DelegatesToStore(t *testing.T) {
	var receivedID int64
	svc := newSvc(&stubLeagueStore{
		updateFn: func(_ context.Context, id int64, _ leagues.UpdateLeagueInput) error {
			receivedID = id
			return nil
		},
	})
	if err := svc.UpdateLeague(context.Background(), 5, leagues.UpdateLeagueInput{Name: "New Name"}); err != nil {
		t.Fatalf("UpdateLeague: %v", err)
	}
	if receivedID != 5 {
		t.Errorf("want id=5, got %d", receivedID)
	}
}

// ── DeleteLeague ──────────────────────────────────────────────────────────────

func TestLeagueService_DeleteLeague_DelegatesToStore(t *testing.T) {
	called := false
	svc := newSvc(&stubLeagueStore{
		deleteFn: func(_ context.Context, id int64) error {
			called = true
			if id != 3 {
				t.Errorf("want id=3, got %d", id)
			}
			return nil
		},
	})
	if err := svc.DeleteLeague(context.Background(), 3); err != nil {
		t.Fatalf("DeleteLeague: %v", err)
	}
	if !called {
		t.Error("want store.DeleteLeague called, was not")
	}
}
