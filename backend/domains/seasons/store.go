package seasons

import (
	"context"
	"errors"

	"league_app/models"
)

// ErrNotFound is returned when a requested season row does not exist.
var ErrNotFound = errors.New("season not found")

// SeasonMeta holds the season-level columns needed for lifecycle decisions.
type SeasonMeta struct {
	LeagueID      int64
	StartDate     *string
	EndDate       *string
	TeamsManaged  bool
	ScheduleStale bool
}

// TeamSummary holds per-team checklist data for one team registered in a season.
type TeamSummary struct {
	TeamID          int64
	Name            string
	CaptainID       *int64
	RosterCount     int
	CaptainOnRoster bool
}

// SeasonTeamEntry is a single team record returned by previous-season lookups.
type SeasonTeamEntry struct {
	TeamID     int64  `json:"team_id"`
	TeamName   string `json:"team_name"`
	SeasonName string `json:"season_name"`
	CaptainID  *int64 `json:"captain_id"`
}

// PreviousSeasonResult is returned by SeasonService.PreviousSeason.
// Season is nil when no previous season exists. Teams is always non-nil.
type PreviousSeasonResult struct {
	Season *models.Season    `json:"season"`
	Teams  []SeasonTeamEntry `json:"teams"`
}

// ChecklistBlockErr is returned by SeasonService.Activate when checklist
// blockers prevent activation. Handlers map this to HTTP 422.
type ChecklistBlockErr struct {
	Blockers []models.ChecklistItem
}

func (e *ChecklistBlockErr) Error() string {
	return "season cannot be activated; resolve all blockers first"
}

// SeasonStore is the persistence interface for season lifecycle operations.
// All methods accept a context and are safe for concurrent use.
type SeasonStore interface {
	// IsDraft returns true when activated_at IS NULL for the season.
	IsDraft(ctx context.Context, seasonID int64) (bool, error)

	// GetMeta returns lifecycle columns for the given season.
	// Returns ErrNotFound (wrapped) when no row exists.
	GetMeta(ctx context.Context, seasonID int64) (SeasonMeta, error)

	// GetTeamSummaries returns per-team checklist data ordered by season_teams.id.
	// Returns a non-nil empty slice when no teams are registered.
	GetTeamSummaries(ctx context.Context, seasonID int64) ([]TeamSummary, error)

	// GetMatchCount returns the number of matches for the season.
	GetMatchCount(ctx context.Context, seasonID int64) (int, error)

	// Activate atomically deactivates all other seasons in leagueID and
	// sets active=1 + activated_at=COALESCE(activated_at, CURRENT_TIMESTAMP).
	Activate(ctx context.Context, seasonID, leagueID int64) error

	// MarkStaleIfScheduled sets schedule_stale=1 when unplayed matches exist.
	MarkStaleIfScheduled(ctx context.Context, seasonID int64) error

	// FindActiveWithNoEndDate returns the active season in the league with no
	// end_date, excluding excludeSeasonID. Returns nil, nil when none exists.
	FindActiveWithNoEndDate(ctx context.Context, leagueID, excludeSeasonID int64) (*models.Season, error)

	// FindClosestPriorByEndDate returns the season with the greatest end_date
	// before beforeDate (or the most recent if beforeDate is nil), in the given
	// league, excluding excludeSeasonID. Returns nil, nil when none exists.
	FindClosestPriorByEndDate(ctx context.Context, leagueID, excludeSeasonID int64, beforeDate *string) (*models.Season, error)

	// GetSeasonTeams returns teams registered in the season via season_teams,
	// ordered by insertion. Returns a non-nil empty slice when none exist.
	GetSeasonTeams(ctx context.Context, seasonID int64) ([]SeasonTeamEntry, error)

	// GetMatchTeams returns distinct teams that appeared in the season's matches.
	// Used as fallback when GetSeasonTeams returns an empty slice.
	GetMatchTeams(ctx context.Context, seasonID int64) ([]SeasonTeamEntry, error)
}
