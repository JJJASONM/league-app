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

// LineupStore defines persistence operations for lineup plans.
type LineupStore interface {
	ListLineupPlans(ctx context.Context, req ListLineupPlansRequest) ([]models.LineupPlan, error)
	SaveTeamLineup(ctx context.Context, req SaveLineupRequest) error
	DeleteLineupPlan(ctx context.Context, id int64) error
}
