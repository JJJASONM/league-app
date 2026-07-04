package teams

import (
	"context"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// TeamService implements business logic for team CRUD operations.
type TeamService struct {
	store TeamStore
}

// NewTeamService returns a TeamService backed by the given store.
func NewTeamService(store TeamStore) *TeamService {
	return &TeamService{store: store}
}

func (s *TeamService) ListTeams(ctx context.Context, leagueID *int64) ([]models.Team, error) {
	return s.store.ListTeams(ctx, leagueID)
}

func (s *TeamService) GetTeam(ctx context.Context, id int64) (models.Team, error) {
	return s.store.GetTeam(ctx, id)
}

// CreateTeam validates that name and league_id are non-empty, then delegates to the store.
func (s *TeamService) CreateTeam(ctx context.Context, input CreateTeamInput) (models.Team, error) {
	if strings.TrimSpace(input.Name) == "" {
		return models.Team{}, domainerr.New("TEAM_NAME_REQUIRED", domainerr.InvalidInput, "name is required")
	}
	if input.LeagueID == 0 {
		return models.Team{}, domainerr.New("TEAM_LEAGUE_REQUIRED", domainerr.InvalidInput, "league_id is required")
	}
	return s.store.CreateTeam(ctx, input)
}

func (s *TeamService) UpdateTeam(ctx context.Context, id int64, input UpdateTeamInput) error {
	return s.store.UpdateTeam(ctx, id, input)
}

func (s *TeamService) DeleteTeam(ctx context.Context, id int64) error {
	return s.store.DeleteTeam(ctx, id)
}
