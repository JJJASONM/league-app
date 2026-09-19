package matches

import (
	"context"
	"errors"

	"league_app/models"
)

// ErrMatchNotFound is returned by MatchStore when the requested match does not exist.
var ErrMatchNotFound = errors.New("match not found")

// ListMatchesRequest filters for ListMatches.
// Both fields are optional. When SeasonID is set, LeagueID is ignored.
type ListMatchesRequest struct {
	SeasonID int64
	LeagueID int64
}

// MatchStore is the persistence interface for match read and team-assignment operations.
// Implementations must be safe for concurrent use by multiple goroutines.
type MatchStore interface {
	// ListMatches returns matches filtered by the request. Both filter fields are
	// optional; when neither is set all matches are returned ordered by week and id.
	ListMatches(ctx context.Context, req ListMatchesRequest) ([]models.Match, error)

	// GetMatch returns the match and its results for the given ID.
	// Returns ErrMatchNotFound when no match with that ID exists.
	GetMatch(ctx context.Context, id int64) (models.MatchDetail, error)

	// AssignMatchTeams sets home_team_id and away_team_id on the match. Either
	// value may be nil to NULL the column. No error is returned when the match
	// ID does not exist (preserves original handler behavior).
	AssignMatchTeams(ctx context.Context, id int64, homeTeamID, awayTeamID *int64) error

	// IsSeasonClosedForMatch returns true when the season containing matchID has
	// closed_at set. Returns false when the match or season does not exist.
	IsSeasonClosedForMatch(ctx context.Context, matchID int64) (bool, error)

	// SeasonLeagueID returns seasonID's league_id, or found=false if no such
	// season exists. Used by AssignMatchTeams to validate that an assigned
	// team belongs to the match's own league (PM correction: a related-
	// resource id from the request body must never be allowed to attach a
	// different league's team to a match).
	SeasonLeagueID(ctx context.Context, seasonID int64) (leagueID int64, found bool, err error)

	// TeamLeagueID returns teamID's league_id, or found=false if no such
	// team exists.
	TeamLeagueID(ctx context.Context, teamID int64) (leagueID int64, found bool, err error)
}
