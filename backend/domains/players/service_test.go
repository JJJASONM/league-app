package players_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/players"
	"league_app/models"
)

// stubPlayerStore implements players.PlayerStore using configurable function fields.
type stubPlayerStore struct {
	listFn   func(ctx context.Context, leagueID *int64) ([]models.Player, error)
	getFn    func(ctx context.Context, id int64) (models.Player, error)
	createFn func(ctx context.Context, input players.CreatePlayerInput) (models.Player, error)
	updateFn func(ctx context.Context, id int64, input players.UpdatePlayerInput) error
	deleteFn func(ctx context.Context, id int64) error
}

func (s *stubPlayerStore) ListPlayers(ctx context.Context, leagueID *int64) ([]models.Player, error) {
	if s.listFn != nil {
		return s.listFn(ctx, leagueID)
	}
	return []models.Player{}, nil
}
func (s *stubPlayerStore) GetPlayer(ctx context.Context, id int64) (models.Player, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return models.Player{}, nil
}
func (s *stubPlayerStore) CreatePlayer(ctx context.Context, input players.CreatePlayerInput) (models.Player, error) {
	if s.createFn != nil {
		return s.createFn(ctx, input)
	}
	return models.Player{}, nil
}
func (s *stubPlayerStore) UpdatePlayer(ctx context.Context, id int64, input players.UpdatePlayerInput) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, id, input)
	}
	return nil
}
func (s *stubPlayerStore) DeletePlayer(ctx context.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func newSvc(store players.PlayerStore) *players.PlayerService {
	return players.NewPlayerService(store)
}

// ── CreatePlayer ──────────────────────────────────────────────────────────────

func TestPlayerService_CreatePlayer_BothNamesEmpty_ReturnsError(t *testing.T) {
	svc := newSvc(&stubPlayerStore{})
	_, err := svc.CreatePlayer(context.Background(), players.CreatePlayerInput{
		FirstName: "  ",
		LastName:  "",
	})
	if err == nil {
		t.Fatal("want error for empty names, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != "PLAYER_NAME_REQUIRED" {
		t.Errorf("want code PLAYER_NAME_REQUIRED, got %q", de.Code)
	}
	if de.Category != domainerr.InvalidInput {
		t.Errorf("want InvalidInput category, got %v", de.Category)
	}
}

func TestPlayerService_CreatePlayer_FirstNameOnly_Delegates(t *testing.T) {
	called := false
	svc := newSvc(&stubPlayerStore{
		createFn: func(_ context.Context, input players.CreatePlayerInput) (models.Player, error) {
			called = true
			return models.Player{ID: 1}, nil
		},
	})
	_, err := svc.CreatePlayer(context.Background(), players.CreatePlayerInput{FirstName: "Alice"})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	if !called {
		t.Error("want store.CreatePlayer called, was not")
	}
}

func TestPlayerService_CreatePlayer_LastNameOnly_Delegates(t *testing.T) {
	called := false
	svc := newSvc(&stubPlayerStore{
		createFn: func(_ context.Context, input players.CreatePlayerInput) (models.Player, error) {
			called = true
			return models.Player{ID: 2}, nil
		},
	})
	_, err := svc.CreatePlayer(context.Background(), players.CreatePlayerInput{LastName: "Smith"})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	if !called {
		t.Error("want store.CreatePlayer called, was not")
	}
}

func TestPlayerService_CreatePlayer_ValidInput_DelegatesToStore(t *testing.T) {
	var received players.CreatePlayerInput
	svc := newSvc(&stubPlayerStore{
		createFn: func(_ context.Context, input players.CreatePlayerInput) (models.Player, error) {
			received = input
			return models.Player{ID: 10}, nil
		},
	})
	got, err := svc.CreatePlayer(context.Background(), players.CreatePlayerInput{
		FirstName: "Bob",
		LastName:  "Jones",
		Handicap:  2.5,
	})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	if got.ID != 10 {
		t.Errorf("want ID=10, got %d", got.ID)
	}
	if received.FirstName != "Bob" {
		t.Errorf("want FirstName=Bob, got %q", received.FirstName)
	}
}

// ── UpdatePlayer ──────────────────────────────────────────────────────────────

func TestPlayerService_UpdatePlayer_DelegatesToStore(t *testing.T) {
	var receivedID int64
	svc := newSvc(&stubPlayerStore{
		updateFn: func(_ context.Context, id int64, _ players.UpdatePlayerInput) error {
			receivedID = id
			return nil
		},
	})
	if err := svc.UpdatePlayer(context.Background(), 7, players.UpdatePlayerInput{FirstName: "New"}); err != nil {
		t.Fatalf("UpdatePlayer: %v", err)
	}
	if receivedID != 7 {
		t.Errorf("want id=7, got %d", receivedID)
	}
}

// ── DeletePlayer ──────────────────────────────────────────────────────────────

func TestPlayerService_DeletePlayer_NoHistory_Succeeds(t *testing.T) {
	called := false
	svc := newSvc(&stubPlayerStore{
		deleteFn: func(_ context.Context, id int64) error {
			called = true
			return nil
		},
	})
	if err := svc.DeletePlayer(context.Background(), 3); err != nil {
		t.Fatalf("DeletePlayer: %v", err)
	}
	if !called {
		t.Error("want store.DeletePlayer called, was not")
	}
}

func TestPlayerService_DeletePlayer_HasHistory_ReturnsConflict(t *testing.T) {
	svc := newSvc(&stubPlayerStore{
		deleteFn: func(_ context.Context, _ int64) error {
			return players.ErrHasHistory
		},
	})
	err := svc.DeletePlayer(context.Background(), 5)
	if err == nil {
		t.Fatal("want error for ErrHasHistory, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) {
		t.Fatalf("want domainerr.Err, got %T: %v", err, err)
	}
	if de.Code != "PLAYER_HAS_HISTORY" {
		t.Errorf("want code PLAYER_HAS_HISTORY, got %q", de.Code)
	}
	if de.Category != domainerr.Conflict {
		t.Errorf("want Conflict category, got %v", de.Category)
	}
}

// ── ListPlayers ───────────────────────────────────────────────────────────────

func TestPlayerService_ListPlayers_DelegatesToStore(t *testing.T) {
	want := []models.Player{{ID: 1, FirstName: "Alice"}, {ID: 2, FirstName: "Bob"}}
	svc := newSvc(&stubPlayerStore{
		listFn: func(_ context.Context, _ *int64) ([]models.Player, error) {
			return want, nil
		},
	})
	got, err := svc.ListPlayers(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 players, got %d", len(got))
	}
}

func TestPlayerService_ListPlayers_LeagueFilterPassedThrough(t *testing.T) {
	var receivedLeagueID *int64
	lid := int64(42)
	svc := newSvc(&stubPlayerStore{
		listFn: func(_ context.Context, leagueID *int64) ([]models.Player, error) {
			receivedLeagueID = leagueID
			return []models.Player{}, nil
		},
	})
	if _, err := svc.ListPlayers(context.Background(), &lid); err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}
	if receivedLeagueID == nil || *receivedLeagueID != 42 {
		t.Errorf("want leagueID=42 passed to store, got %v", receivedLeagueID)
	}
}

// ── GetPlayer ─────────────────────────────────────────────────────────────────

func TestPlayerService_GetPlayer_DelegatesToStore(t *testing.T) {
	want := models.Player{ID: 99, FirstName: "Carol"}
	svc := newSvc(&stubPlayerStore{
		getFn: func(_ context.Context, id int64) (models.Player, error) {
			if id != 99 {
				t.Errorf("want id=99, got %d", id)
			}
			return want, nil
		},
	})
	got, err := svc.GetPlayer(context.Background(), 99)
	if err != nil {
		t.Fatalf("GetPlayer: %v", err)
	}
	if got.FirstName != "Carol" {
		t.Errorf("want FirstName=Carol, got %q", got.FirstName)
	}
}

func TestPlayerService_GetPlayer_PropagatesNotFound(t *testing.T) {
	svc := newSvc(&stubPlayerStore{
		getFn: func(_ context.Context, _ int64) (models.Player, error) {
			return models.Player{}, players.ErrNotFound
		},
	})
	_, err := svc.GetPlayer(context.Background(), 9999)
	if !errors.Is(err, players.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
