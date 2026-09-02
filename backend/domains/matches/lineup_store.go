package matches

import (
	"context"

	"league_app/models"
)

// ListLineupPlansRequest filters lineup plan queries. SeasonID is required;
// WeekNumber and TeamID of 0 mean no filter.
type ListLineupPlansRequest struct {
	SeasonID   int64
	WeekNumber int64
	TeamID     int64
}

// SaveLineupRequest replaces all lineup slots for one team/week in a season.
type SaveLineupRequest struct {
	SeasonID   int64
	TeamID     int64
	WeekNumber int64
	PlayerIDs  []int64
}

// SetSubstituteRequest replaces one lineup slot's player with a substitute,
// recording the original player via sub_for_id (Substitute Workflow Phase 1).
// OriginalPlayerID is resolved by the service (via GetLineupPlan) before
// calling the store, so the store does not need a second read to know what
// to record as sub_for_id.
type SetSubstituteRequest struct {
	LineupPlanID       int64
	SubstitutePlayerID int64
	OriginalPlayerID   int64
}

// LineupStore defines persistence operations for lineup plans.
type LineupStore interface {
	ListLineupPlans(ctx context.Context, req ListLineupPlansRequest) ([]models.LineupPlan, error)
	SaveTeamLineup(ctx context.Context, req SaveLineupRequest) error
	DeleteLineupPlan(ctx context.Context, id int64) error

	// GetLineupPlan returns one lineup plan row by id. Returns
	// domainerr.NotFound when no row matches.
	GetLineupPlan(ctx context.Context, id int64) (models.LineupPlan, error)

	// FindMatchID returns the id of the match where teamID plays (home or
	// away) in seasonID/weekNumber, or found=false when no such match
	// exists yet. Used to look up lock state (season closed, week closed,
	// approved, processed) before allowing a substitute change.
	FindMatchID(ctx context.Context, seasonID, teamID int64, weekNumber int64) (matchID int64, found bool, err error)

	// SetSubstitute updates the lineup slot to the substitute player,
	// setting is_sub=true and sub_for_id to req.OriginalPlayerID. Returns
	// the updated row. Returns a UNIQUE-constraint error (mapped by the
	// service) when the substitute is already in this team/week/season's
	// lineup under a different slot.
	SetSubstitute(ctx context.Context, req SetSubstituteRequest) (models.LineupPlan, error)

	// ClearSubstitute reverts a substituted slot back to its original
	// player (is_sub=false, sub_for_id=NULL). Returns domainerr.InvalidInput
	// when the slot is not currently substituted.
	ClearSubstitute(ctx context.Context, id int64) (models.LineupPlan, error)
}
