package teams

import (
	"context"
	"errors"

	"league_app/models"
)

// ErrNotFound is returned by the store when a team row does not exist.
var ErrNotFound = errors.New("team not found")

// TeamStore is the persistence interface for team CRUD operations.
type TeamStore interface {
	// ListTeams returns all teams ordered by name.
	// When leagueID is non-nil the result is filtered to that league.
	// Returns a non-nil empty slice when none match.
	ListTeams(ctx context.Context, leagueID *int64) ([]models.Team, error)

	// GetTeam returns the team by ID with embedded players.
	// Returns ErrNotFound (wrapped) when no row exists.
	GetTeam(ctx context.Context, id int64) (models.Team, error)

	// CreateTeam inserts a new team and returns the stored fields.
	// TeamNumber is not set on creation — it is managed by the season workflow.
	CreateTeam(ctx context.Context, input CreateTeamInput) (models.Team, error)

	// UpdateTeam updates the mutable team fields (name and captain).
	// TeamNumber is intentionally excluded — it is not editable via the team API.
	// No error is returned when the row does not exist (UPDATE affects 0 rows).
	UpdateTeam(ctx context.Context, id int64, input UpdateTeamInput) error

	// DeleteTeam removes a team by ID and nulls any player team assignments.
	// No error is returned when the row does not exist.
	DeleteTeam(ctx context.Context, id int64) error
}

// CreateTeamInput carries user-supplied fields for team creation.
// TeamNumber is intentionally excluded — it is not insertable via the team API.
type CreateTeamInput struct {
	LeagueID int64
	Name     string
}

// UpdateTeamInput carries the mutable fields for a team update.
// TeamNumber is intentionally excluded — it is not updatable via the team API.
type UpdateTeamInput struct {
	Name      string
	CaptainID *int64
}
