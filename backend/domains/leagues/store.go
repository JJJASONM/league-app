package leagues

import (
	"context"
	"errors"

	"league_app/models"
)

// ErrNotFound is returned when a requested league row does not exist.
var ErrNotFound = errors.New("league not found")

// LeagueStore is the persistence interface for league CRUD operations.
type LeagueStore interface {
	// ListLeagues returns all leagues ordered by id.
	// Returns a non-nil empty slice when none exist.
	ListLeagues(ctx context.Context) ([]models.League, error)

	// GetLeague returns the league by ID.
	// Returns ErrNotFound (wrapped) when no row exists.
	GetLeague(ctx context.Context, id int64) (models.League, error)

	// CreateLeague inserts a new league and returns the stored row (without
	// re-fetching created_at, preserving the previous handler's behavior).
	CreateLeague(ctx context.Context, input CreateLeagueInput) (models.League, error)

	// UpdateLeague updates name, game_format, and day_of_week.
	// No error is returned when the row does not exist (UPDATE affects 0 rows).
	UpdateLeague(ctx context.Context, id int64, input UpdateLeagueInput) error

	// DeleteLeague removes the league by ID.
	// No error is returned when the row does not exist.
	DeleteLeague(ctx context.Context, id int64) error
}

// CreateLeagueInput carries the user-supplied fields for league creation.
type CreateLeagueInput struct {
	Name       string
	GameFormat string
	DayOfWeek  string
}

// UpdateLeagueInput carries the mutable fields for a league update.
type UpdateLeagueInput struct {
	Name       string
	GameFormat string
	DayOfWeek  string
}
