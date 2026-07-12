package seasons

import (
	"context"
	"errors"

	"league_app/backend/domainerr"
	"league_app/models"
)

// ListByeRequests returns all bye requests for a season.
func (s *SeasonService) ListByeRequests(ctx context.Context, seasonID int64) ([]models.ByeRequest, error) {
	byes, err := s.store.ListByeRequests(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	if byes == nil {
		byes = []models.ByeRequest{}
	}
	return byes, nil
}

// DeleteByeRequest deletes a bye request scoped to the season.
// Returns domainerr.NotFound when the request does not exist in the season.
// Marks the season stale after a successful delete because the deleted request
// may have been approved and baked into the generated schedule.
func (s *SeasonService) DeleteByeRequest(ctx context.Context, seasonID, byeID int64) error {
	if err := s.store.DeleteByeRequest(ctx, seasonID, byeID); err != nil {
		if errors.Is(err, ErrByeNotFound) {
			return domainerr.New("BYE_NOT_FOUND", domainerr.NotFound, "bye request not found")
		}
		return err
	}
	_ = s.store.MarkStaleIfScheduled(ctx, seasonID)
	return nil
}
