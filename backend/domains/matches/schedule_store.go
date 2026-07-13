package matches

import (
	"context"
	"errors"
)

// ErrSeasonNotFound is returned by ScheduleStore when the requested season does not exist.
var ErrSeasonNotFound = errors.New("season not found")

// ScheduleSeasonMeta holds the season attributes needed to drive schedule generation.
type ScheduleSeasonMeta struct {
	LeagueID     int64
	TeamsManaged bool
	Active       bool // true when seasons.active = 1
}

// MatchEntry is one scheduled match slot produced by a schedule generator.
// HomeTeamID/AwayTeamID may be 0 for blanket (unassigned) slots.
type MatchEntry struct {
	HomeTeamID int64
	AwayTeamID int64
	WeekNumber int
	MatchDate  string // YYYY-MM-DD; empty when no start date was supplied
}

// SaveScheduleRequest carries the generated schedule to be persisted atomically.
type SaveScheduleRequest struct {
	SeasonID     int64
	ScheduleType string
	NumWeeks     int
	EndDate      string // YYYY-MM-DD of the last scheduled match; empty for undated schedules
	Entries      []MatchEntry
}

// ScheduleStore is the persistence interface for schedule generation.
// Implementations must be safe for concurrent use by multiple goroutines.
type ScheduleStore interface {
	// GetScheduleSeasonMeta returns the season's league_id and teams_managed flag.
	// Returns ErrSeasonNotFound when the season does not exist.
	GetScheduleSeasonMeta(ctx context.Context, seasonID int64) (ScheduleSeasonMeta, error)

	// LoadByeRequests returns approved bye requests that have a specific week
	// number (week_number > 0) for the season. Key: week number; value: team ID.
	// Returns an empty map when none exist.
	LoadByeRequests(ctx context.Context, seasonID int64) (map[int]int64, error)

	// LoadTeamIDsFromHistory returns the distinct team IDs that appeared in
	// matches for the given season. Used for the legacy from_season_id path.
	LoadTeamIDsFromHistory(ctx context.Context, fromSeasonID int64) ([]int64, error)

	// LoadTeamIDsForSchedule returns the team IDs to use when generating a schedule.
	// When teamsManaged is true, returns only season_teams rows for seasonID.
	// When teamsManaged is false, returns season_teams if any exist; otherwise
	// falls back to all teams in the league.
	LoadTeamIDsForSchedule(ctx context.Context, seasonID, leagueID int64, teamsManaged bool) ([]int64, error)

	// HasClosedWeeks reports whether any league_weeks row for the season has
	// status "closed". Used to guard against regenerating a schedule after
	// official results have been committed.
	HasClosedWeeks(ctx context.Context, seasonID int64) (bool, error)

	// HasCompletedMatches reports whether the season has any match with completed=1.
	HasCompletedMatches(ctx context.Context, seasonID int64) (bool, error)

	// SaveGeneratedSchedule atomically deletes unplayed matches for the season,
	// inserts the new match rows, and updates the season row
	// (schedule_type, num_weeks, end_date, schedule_stale=0).
	SaveGeneratedSchedule(ctx context.Context, req SaveScheduleRequest) error
}
