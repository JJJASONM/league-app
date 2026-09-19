package matches

import (
	"context"
	"errors"

	"league_app/backend/domainerr"
	"league_app/models"
)

// MatchService provides match read and team-assignment operations.
type MatchService struct {
	store MatchStore
}

// NewMatchService returns a MatchService backed by the given store.
func NewMatchService(store MatchStore) *MatchService {
	return &MatchService{store: store}
}

// ListMatches returns matches filtered by the request, ordered by week then id.
// Returns an empty (non-nil) slice when no matches exist.
func (s *MatchService) ListMatches(ctx context.Context, req ListMatchesRequest) ([]models.Match, error) {
	ms, err := s.store.ListMatches(ctx, req)
	if err != nil {
		return nil, domainerr.New("MATCH_LIST_FAILED", domainerr.Internal, "list matches failed")
	}
	if ms == nil {
		ms = []models.Match{}
	}
	return ms, nil
}

// GetMatch returns the match detail for the given ID.
// Returns a domainerr.NotFound error when the match does not exist.
func (s *MatchService) GetMatch(ctx context.Context, id int64) (models.MatchDetail, error) {
	d, err := s.store.GetMatch(ctx, id)
	if err != nil {
		if errors.Is(err, ErrMatchNotFound) {
			return models.MatchDetail{}, domainerr.New("MATCH_NOT_FOUND", domainerr.NotFound, "match not found")
		}
		return models.MatchDetail{}, domainerr.New("MATCH_GET_FAILED", domainerr.Internal, "get match failed")
	}
	return d, nil
}

// AssignMatchTeams sets the home and away team IDs on the match.
// Either value may be nil to NULL the column.
// Returns domainerr.Conflict when the match is already completed, or when
// a non-nil team id does not belong to the match's own season's league
// (PM correction: this invariant lives here, at the authoritative
// backend service boundary, not only in frontend team-picker filtering).
func (s *MatchService) AssignMatchTeams(ctx context.Context, id int64, homeTeamID, awayTeamID *int64) error {
	if sc, err := s.store.IsSeasonClosedForMatch(ctx, id); err != nil {
		return domainerr.New("MATCH_ASSIGN_FAILED", domainerr.Internal, "assign teams failed")
	} else if sc {
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	}
	d, err := s.store.GetMatch(ctx, id)
	if err != nil && !errors.Is(err, ErrMatchNotFound) {
		return domainerr.New("MATCH_ASSIGN_FAILED", domainerr.Internal, "assign teams failed")
	}
	if err == nil {
		if d.Match.Completed {
			return domainerr.New("MATCH_ALREADY_COMPLETED", domainerr.Conflict,
				"match is completed; team assignments cannot be changed")
		}
		if err := s.rejectCrossLeagueTeams(ctx, d.Match.SeasonID, homeTeamID, awayTeamID); err != nil {
			return err
		}
	}
	if err := s.store.AssignMatchTeams(ctx, id, homeTeamID, awayTeamID); err != nil {
		return domainerr.New("MATCH_ASSIGN_FAILED", domainerr.Internal, "assign teams failed")
	}
	return nil
}

// rejectCrossLeagueTeams returns domainerr.Conflict if any non-nil team id
// does not belong to seasonID's own league. A season or team that cannot
// be resolved at all is treated as a validation failure here too, rather
// than silently allowing the assignment through.
func (s *MatchService) rejectCrossLeagueTeams(ctx context.Context, seasonID int64, teamIDs ...*int64) error {
	leagueID, found, err := s.store.SeasonLeagueID(ctx, seasonID)
	if err != nil {
		return domainerr.New("MATCH_ASSIGN_FAILED", domainerr.Internal, "assign teams failed")
	}
	if !found {
		return nil
	}
	for _, teamID := range teamIDs {
		if teamID == nil {
			continue
		}
		teamLeagueID, found, err := s.store.TeamLeagueID(ctx, *teamID)
		if err != nil {
			return domainerr.New("MATCH_ASSIGN_FAILED", domainerr.Internal, "assign teams failed")
		}
		if !found || teamLeagueID != leagueID {
			return domainerr.New("MATCH_ASSIGN_CROSS_LEAGUE", domainerr.Conflict,
				"team does not belong to this match's league")
		}
	}
	return nil
}
