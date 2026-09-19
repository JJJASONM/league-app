package matches

import (
	"context"

	"league_app/models"
)

// ListLineupPlansRequest filters lineup plan queries. SeasonID is required.
// WeekNumber is always an exact filter -- 0 means the real, meaningful
// "Default Lineup" week (lineup_plans.week_number=0), never "all weeks."
// No caller needs "all weeks in one call" today; every caller (Dashboard,
// Match Entry, the League Admin hub, and the Lineups screen) always passes
// an explicit real week or 0 for the default. TeamID of 0 means no team
// filter (team ids are never 0, so it is unambiguous, unlike WeekNumber).
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

	// SeasonInfo returns seasonID's league_id and its teams_managed flag,
	// or found=false if no such season exists. Used by SaveTeamLineup to
	// validate that the requested team belongs to the season's own league
	// (PM correction: a related-resource team_id from the request body
	// must never be allowed to attach a different league's team to a
	// season's lineup), and, for teams_managed seasons, that it actually
	// participates in the season.
	SeasonInfo(ctx context.Context, seasonID int64) (leagueID int64, teamsManaged bool, found bool, err error)

	// TeamLeagueID returns teamID's league_id, or found=false if no such
	// team exists.
	TeamLeagueID(ctx context.Context, teamID int64) (leagueID int64, found bool, err error)

	// TeamParticipatesInSeason reports whether teamID is registered in
	// season_teams for seasonID.
	TeamParticipatesInSeason(ctx context.Context, seasonID, teamID int64) (bool, error)

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

	// PlayerInMatchLineup returns true when playerID already has a
	// lineup_plans row for seasonID/weekNumber under either homeTeamID or
	// awayTeamID, other than excludePlanID (the slot currently being
	// substituted). Used to reject a substitute who is already in the same
	// scheduled match on either side, even though they are otherwise
	// allowed to come from any team/league.
	PlayerInMatchLineup(ctx context.Context, seasonID, weekNumber, homeTeamID, awayTeamID, excludePlanID, playerID int64) (bool, error)

	// ClearSubstitute reverts a substituted slot back to its original
	// player (is_sub=false, sub_for_id=NULL). Returns domainerr.InvalidInput
	// when the slot is not currently substituted.
	ClearSubstitute(ctx context.Context, id int64) (models.LineupPlan, error)
}
