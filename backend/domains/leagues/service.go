package leagues

import (
	"context"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// LeagueService implements business logic for league CRUD operations.
type LeagueService struct {
	store LeagueStore
}

// NewLeagueService returns a LeagueService backed by the given store.
func NewLeagueService(store LeagueStore) *LeagueService {
	return &LeagueService{store: store}
}

func (s *LeagueService) ListLeagues(ctx context.Context) ([]models.League, error) {
	return s.store.ListLeagues(ctx)
}

func (s *LeagueService) GetLeague(ctx context.Context, id int64) (models.League, error) {
	return s.store.GetLeague(ctx, id)
}

// CreateLeague validates name, defaults game_format to "8ball", then delegates.
func (s *LeagueService) CreateLeague(ctx context.Context, input CreateLeagueInput) (models.League, error) {
	if strings.TrimSpace(input.Name) == "" {
		return models.League{}, domainerr.New("LEAGUE_NAME_REQUIRED", domainerr.InvalidInput, "name is required")
	}
	if input.GameFormat == "" {
		input.GameFormat = "8ball"
	}
	return s.store.CreateLeague(ctx, input)
}

func (s *LeagueService) UpdateLeague(ctx context.Context, id int64, input UpdateLeagueInput) error {
	return s.store.UpdateLeague(ctx, id, input)
}

func (s *LeagueService) DeleteLeague(ctx context.Context, id int64) error {
	return s.store.DeleteLeague(ctx, id)
}
