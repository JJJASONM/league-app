package seasons

import (
	"context"
	"strings"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// ListSeasons returns all seasons. When leagueID is non-nil only that league's
// seasons are returned. Returns a non-nil empty slice when none exist.
func (s *SeasonService) ListSeasons(ctx context.Context, leagueID *int64) ([]models.Season, error) {
	out, err := s.store.ListSeasons(ctx, leagueID)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []models.Season{}
	}
	return out, nil
}

// GetSeason returns the full season record by ID.
// Propagates ErrNotFound (wrapped) from the store when the season does not exist.
func (s *SeasonService) GetSeason(ctx context.Context, seasonID int64) (models.Season, error) {
	return s.store.GetSeason(ctx, seasonID)
}

// CreateSeason validates the input, applies defaults, and inserts the season.
// Returns the stored row including all server-assigned fields.
// Returns domainerr.InvalidInput when name is empty or league_id is zero.
func (s *SeasonService) CreateSeason(ctx context.Context, input CreateSeasonInput) (models.Season, error) {
	if strings.TrimSpace(input.Name) == "" {
		return models.Season{}, domainerr.New("SEASON_NAME_REQUIRED", domainerr.InvalidInput, "name is required")
	}
	if input.LeagueID == 0 {
		return models.Season{}, domainerr.New("LEAGUE_ID_REQUIRED", domainerr.InvalidInput, "league_id is required")
	}
	if input.ScheduleType == "" {
		input.ScheduleType = matches.ScheduleTypeDoubleRR
	}
	return s.store.CreateSeason(ctx, input)
}

// UpdateSeason applies defaults and updates the season's mutable fields.
// Returns the full stored row after the update so callers always see authoritative data.
// Propagates ErrNotFound (wrapped) from the store when the season does not exist.
func (s *SeasonService) UpdateSeason(ctx context.Context, seasonID int64, input UpdateSeasonInput) (models.Season, error) {
	if input.ScheduleType == "" {
		input.ScheduleType = matches.ScheduleTypeDoubleRR
	}
	return s.store.UpdateSeason(ctx, seasonID, input)
}

// DeleteSeason removes the season record by ID.
// No error is returned when the row does not exist.
func (s *SeasonService) DeleteSeason(ctx context.Context, seasonID int64) error {
	return s.store.DeleteSeason(ctx, seasonID)
}
