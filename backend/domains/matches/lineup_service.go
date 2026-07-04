package matches

import (
	"context"

	"league_app/backend/domainerr"
	"league_app/models"
)

// LineupService provides lineup plan read and write operations.
type LineupService struct{ store LineupStore }

// NewLineupService returns a LineupService backed by the given store.
func NewLineupService(store LineupStore) *LineupService {
	return &LineupService{store: store}
}

// ListLineupPlans returns lineup plans filtered by the request.
// Returns an empty (non-nil) slice when no plans exist.
func (s *LineupService) ListLineupPlans(ctx context.Context, req ListLineupPlansRequest) ([]models.LineupPlan, error) {
	plans, err := s.store.ListLineupPlans(ctx, req)
	if err != nil {
		return nil, domainerr.New("LINEUP_LIST_FAILED", domainerr.Internal, "list lineup plans failed")
	}
	if plans == nil {
		plans = []models.LineupPlan{}
	}
	return plans, nil
}

// SaveTeamLineup atomically replaces all lineup slots for one team/week.
func (s *LineupService) SaveTeamLineup(ctx context.Context, req SaveLineupRequest) error {
	if err := s.store.SaveTeamLineup(ctx, req); err != nil {
		return domainerr.New("LINEUP_SAVE_FAILED", domainerr.Internal, "save lineup failed")
	}
	return nil
}

// DeleteLineupPlan removes a lineup plan by ID. Deleting a non-existent plan is not an error.
func (s *LineupService) DeleteLineupPlan(ctx context.Context, id int64) error {
	if err := s.store.DeleteLineupPlan(ctx, id); err != nil {
		return domainerr.New("LINEUP_DELETE_FAILED", domainerr.Internal, "delete lineup plan failed")
	}
	return nil
}
