package players

import (
	"context"
	"errors"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// PlayerService implements business logic for player CRUD operations.
type PlayerService struct {
	store PlayerStore
}

// NewPlayerService returns a PlayerService backed by the given store.
func NewPlayerService(store PlayerStore) *PlayerService {
	return &PlayerService{store: store}
}

func (s *PlayerService) ListPlayers(ctx context.Context, leagueID *int64) ([]models.Player, error) {
	return s.store.ListPlayers(ctx, leagueID)
}

func (s *PlayerService) GetPlayer(ctx context.Context, id int64) (models.Player, error) {
	return s.store.GetPlayer(ctx, id)
}

// CreatePlayer validates that at least one of first name or last name is non-empty,
// then delegates to the store.
func (s *PlayerService) CreatePlayer(ctx context.Context, input CreatePlayerInput) (models.Player, error) {
	if strings.TrimSpace(input.FirstName) == "" && strings.TrimSpace(input.LastName) == "" {
		return models.Player{}, domainerr.New("PLAYER_NAME_REQUIRED", domainerr.InvalidInput, "first or last name is required")
	}
	return s.store.CreatePlayer(ctx, input)
}

// UpdatePlayer delegates to the store. PlayerNumber is locked at creation and not
// present in UpdatePlayerInput.
func (s *PlayerService) UpdatePlayer(ctx context.Context, id int64, input UpdatePlayerInput) error {
	return s.store.UpdatePlayer(ctx, id, input)
}

// DeletePlayer blocks deletion when handicap history exists for the player,
// returning a Conflict-category domain error in that case.
func (s *PlayerService) DeletePlayer(ctx context.Context, id int64) error {
	err := s.store.DeletePlayer(ctx, id)
	if errors.Is(err, ErrHasHistory) {
		return domainerr.New("PLAYER_HAS_HISTORY", domainerr.Conflict,
			"This player has handicap history records and cannot be deleted. Deactivate the player instead.")
	}
	return err
}
