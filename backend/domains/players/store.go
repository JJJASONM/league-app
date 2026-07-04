package players

import (
	"context"
	"errors"

	"league_app/models"
)

// ErrNotFound is returned by the store when a player row does not exist.
var ErrNotFound = errors.New("player not found")

// ErrHasHistory is returned by DeletePlayer when handicap_history records exist
// for the player. Callers should surface this as a 409 Conflict.
var ErrHasHistory = errors.New("player has handicap history")

// PlayerStore is the persistence interface for player CRUD operations.
type PlayerStore interface {
	// ListPlayers returns all players ordered by last_name, first_name.
	// When leagueID is non-nil the result is filtered to that league via the teams join.
	// Returns a non-nil empty slice when none match.
	ListPlayers(ctx context.Context, leagueID *int64) ([]models.Player, error)

	// GetPlayer returns the player by ID with team context joined.
	// Returns ErrNotFound (wrapped) when no row exists.
	GetPlayer(ctx context.Context, id int64) (models.Player, error)

	// CreatePlayer inserts a new player and returns the stored fields without
	// re-fetching created_at, preserving the previous handler's response shape.
	// PlayerNumber is set at creation and cannot be changed after.
	CreatePlayer(ctx context.Context, input CreatePlayerInput) (models.Player, error)

	// UpdatePlayer updates the mutable player fields. PlayerNumber is intentionally
	// excluded — it is locked once set on creation.
	// No error is returned when the row does not exist (UPDATE affects 0 rows).
	UpdatePlayer(ctx context.Context, id int64, input UpdatePlayerInput) error

	// DeletePlayer removes a player by ID.
	// Returns ErrHasHistory when handicap_history records exist for the player.
	// No error is returned when the row does not exist and no history is found.
	DeletePlayer(ctx context.Context, id int64) error
}

// CreatePlayerInput carries user-supplied fields for player creation.
// Active and Note are intentionally excluded — not in the INSERT statement.
type CreatePlayerInput struct {
	PlayerNumber string
	FirstName    string
	LastName     string
	Phone        string
	Email        string
	TeamID       *int64
	Handicap     float64
	AdminHold    bool
}

// UpdatePlayerInput carries the mutable fields for a player update.
// PlayerNumber, Active, and Note are intentionally excluded:
// PlayerNumber is locked once set; Active and Note were not in the original UPDATE.
type UpdatePlayerInput struct {
	FirstName string
	LastName  string
	Phone     string
	Email     string
	TeamID    *int64
	Handicap  float64
	AdminHold bool
}
