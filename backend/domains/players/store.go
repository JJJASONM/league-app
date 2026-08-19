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

// Merge blocker sentinels. MergePlayers returns one of these when repointing
// source's references onto target would violate a unique constraint or would
// otherwise produce a nonsensical result. Callers should surface these as a
// 409 Conflict.
var (
	// ErrMergeSeasonRosterConflict: both players already have a season_rosters
	// row for the same season (regardless of team) -- repointing would violate
	// UNIQUE(season_id, player_id). This is also the general case of "source
	// and target are rostered on different teams in the same season."
	ErrMergeSeasonRosterConflict = errors.New("source and target are both rostered in the same season")

	// ErrMergeRoundResultConflict: both players already participate (as home
	// OR away, in any row) in the same (match_id, round_number) -- repointing
	// would make target appear twice in one round. Covers both-home,
	// both-away, and home-in-one-row/away-in-another-row combinations; the
	// direct-opponent case is ErrMergeSelfOpponentConflict instead, for a
	// more specific message.
	ErrMergeRoundResultConflict = errors.New("source and target already both participate in the same scoresheet round")

	// ErrMergeSelfOpponentConflict: source and target already played against
	// each other (one home, one away) in the same round_results row --
	// repointing would make a player play themselves.
	ErrMergeSelfOpponentConflict = errors.New("source and target already played against each other in a round result")

	// ErrMergeLineupPlanConflict: both players already have a lineup_plans row
	// for the same team/week/season -- repointing would violate
	// UNIQUE(team_id, week_number, season_id, player_id).
	ErrMergeLineupPlanConflict = errors.New("source and target would collide in an existing lineup plan slot")
)

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

	// MergePlayers repoints every supported player reference (match_results,
	// handicap_history, round_results home/away, lineup_plans player/sub_for,
	// season_teams captain, season_rosters, teams captain) from sourceID to
	// targetID, then deletes the source player row. Runs in one transaction;
	// all changes roll back together if any step fails.
	//
	// Callers are responsible for confirming sourceID != targetID and that
	// both players exist before calling -- this method assumes both is true
	// and focuses solely on blocker detection and repointing.
	//
	// Returns one of the ErrMerge* sentinels when a blocker prevents a safe
	// merge; no repointing or deletion happens in that case.
	MergePlayers(ctx context.Context, sourceID, targetID int64) error
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
