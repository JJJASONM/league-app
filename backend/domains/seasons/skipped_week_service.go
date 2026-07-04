package seasons

import (
	"context"

	"league_app/models"
)

// ListSkippedWeeks returns all skipped weeks for a season.
func (s *SeasonService) ListSkippedWeeks(ctx context.Context, seasonID int64) ([]models.SkippedWeek, error) {
	weeks, err := s.store.ListSkippedWeeks(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	if weeks == nil {
		weeks = []models.SkippedWeek{}
	}
	return weeks, nil
}

// CreateSkippedWeek inserts a skipped week (idempotent on duplicate skip_date)
// and returns the row whether newly inserted or pre-existing.
func (s *SeasonService) CreateSkippedWeek(ctx context.Context, seasonID int64, skipDate, reason string) (models.SkippedWeek, error) {
	return s.store.CreateSkippedWeek(ctx, seasonID, skipDate, reason)
}

// DeleteSkippedWeek removes a skipped week by id. No error is returned when the
// row does not exist (matches original handler behavior).
func (s *SeasonService) DeleteSkippedWeek(ctx context.Context, id int64) error {
	return s.store.DeleteSkippedWeek(ctx, id)
}
