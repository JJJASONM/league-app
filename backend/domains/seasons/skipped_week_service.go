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

// CreateSkippedWeek inserts a skipped week (idempotent on duplicate skip_date),
// returns the row whether newly inserted or pre-existing, and marks the season
// schedule stale when unplayed matches exist.
func (s *SeasonService) CreateSkippedWeek(ctx context.Context, seasonID int64, skipDate, reason string) (models.SkippedWeek, error) {
	sw, err := s.store.CreateSkippedWeek(ctx, seasonID, skipDate, reason)
	if err != nil {
		return models.SkippedWeek{}, err
	}
	_ = s.store.MarkStaleIfScheduled(ctx, seasonID)
	return sw, nil
}

// DeleteSkippedWeek removes a skipped week scoped to the season by id and
// marks the season schedule stale when unplayed matches exist. No error is
// returned when the row does not exist (matches original handler behavior).
func (s *SeasonService) DeleteSkippedWeek(ctx context.Context, seasonID, id int64) error {
	if err := s.store.DeleteSkippedWeek(ctx, seasonID, id); err != nil {
		return err
	}
	_ = s.store.MarkStaleIfScheduled(ctx, seasonID)
	return nil
}
