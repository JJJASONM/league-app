package seasons

import (
	"context"
	"errors"

	"league_app/models"
)

// ErrNotFound is returned when a requested season row does not exist.
var ErrNotFound = errors.New("season not found")

// ErrTeamAlreadyInSeason is returned by AddSeasonTeamCopy when the team is already registered.
var ErrTeamAlreadyInSeason = errors.New("team is already in this season")

// ErrTeamNotInSeason is returned when a season team operation targets an unregistered team.
var ErrTeamNotInSeason = errors.New("team not found in this season")

// ErrByeNotFound is returned when a bye request is not found within the given season.
var ErrByeNotFound = errors.New("bye request not found")

// ErrTeamNotInPriorSeason is returned by AddSeasonTeamCopy when the prior season was
// managed but the team was not registered in it.
var ErrTeamNotInPriorSeason = errors.New("team did not participate in the previous season")

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

// AddTeamRequest is the input for SeasonService.AddTeam.
// Exactly one of (FromTeamID + FromSeasonID) or Name must be provided.
type AddTeamRequest struct {
	FromTeamID   int64  `json:"from_team_id"`   // copy from an existing team
	FromSeasonID int64  `json:"from_season_id"` // prior season the team played in
	Name         string `json:"name"`           // new-team path: creates a teams record
}

// UpdateTeamRequest is the input for SeasonService.UpdateTeam.
type UpdateTeamRequest struct {
	SeasonName string `json:"season_name"`
	CaptainID  *int64 `json:"captain_id"`
}

// CreateByeRequestInput is the input for SeasonService.CreateByeRequest.
type CreateByeRequestInput struct {
	TeamID     int64  `json:"team_id"`
	WeekNumber int    `json:"week_number"` // 0 = TBD
	Reason     string `json:"reason"`
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

	// ── Team management ───────────────────────────────────────────────────────

	// GetTeamLeagueID returns the league_id for a team.
	// Returns ErrNotFound (wrapped) when the team does not exist.
	GetTeamLeagueID(ctx context.Context, teamID int64) (int64, error)

	// GetSeasonTeam returns the full SeasonTeam for a season+team.
	// Returns ErrTeamNotInSeason (wrapped) when not registered.
	GetSeasonTeam(ctx context.Context, seasonID, teamID int64) (models.SeasonTeam, error)

	// AddSeasonTeamCopy registers teamID in seasonID by copying metadata and
	// roster from fromSeasonID. When fromSeasonID==0 and teamsManaged is false,
	// active players from players.team_id are used as the initial roster.
	// Returns ErrTeamAlreadyInSeason when already registered.
	// Returns ErrTeamNotInPriorSeason when the prior season was managed but the
	// team was not registered in it.
	AddSeasonTeamCopy(ctx context.Context, seasonID, teamID, fromSeasonID int64, teamsManaged bool) error

	// AddSeasonTeamNew creates a new team in leagueID and registers it in seasonID.
	// Returns the new team's ID.
	AddSeasonTeamNew(ctx context.Context, seasonID, leagueID int64, name string) (int64, error)

	// CheckPlayerOnSeasonRoster reports whether playerID is on teamID's season roster.
	CheckPlayerOnSeasonRoster(ctx context.Context, seasonID, teamID, playerID int64) (bool, error)

	// UpdateSeasonTeamMeta updates season_name and captain_id for a registered team.
	// Returns ErrTeamNotInSeason (wrapped) when the team is not registered.
	UpdateSeasonTeamMeta(ctx context.Context, seasonID, teamID int64, seasonName string, captainID *int64) error

	// RemoveSeasonTeam deletes a team from the season (roster + season_teams row).
	// When the team has no match history and no other season registrations, the
	// team record and player assignments are also cleaned up.
	// Returns ErrTeamNotInSeason (wrapped) when the team is not registered.
	RemoveSeasonTeam(ctx context.Context, seasonID, teamID int64) error

	// ── Bye request management ────────────────────────────────────────────────

	// CountParticipatingTeams returns the effective team count for a season.
	// For managed seasons (or legacy seasons with season_teams rows), uses
	// season_teams count. Falls back to league team count otherwise.
	CountParticipatingTeams(ctx context.Context, seasonID, leagueID int64, teamsManaged bool) (int, error)

	// CheckTeamInSeason reports whether teamID is registered in seasonID via season_teams.
	CheckTeamInSeason(ctx context.Context, seasonID, teamID int64) (bool, error)

	// HasDuplicateBye reports whether a bye request already exists for the
	// season/team/weekNumber combination.
	HasDuplicateBye(ctx context.Context, seasonID, teamID int64, weekNumber int) (bool, error)

	// InsertByeRequest creates a new bye request and returns the full record.
	InsertByeRequest(ctx context.Context, seasonID, teamID int64, weekNumber int, reason string) (models.ByeRequest, error)

	// GetByeRequest returns a bye request scoped to the season.
	// Returns ErrByeNotFound (wrapped) when not found in the season.
	GetByeRequest(ctx context.Context, seasonID, byeID int64) (models.ByeRequest, error)

	// HasByeConflict reports whether another bye request (not excludeByeID) is
	// already approved for the same season+week.
	HasByeConflict(ctx context.Context, seasonID int64, weekNumber int, excludeByeID int64) (bool, error)

	// SetByeApproval updates the approved flag on a bye request and returns the
	// updated record. Returns ErrByeNotFound (wrapped) when not found in the season.
	SetByeApproval(ctx context.Context, seasonID, byeID int64, approved bool) (models.ByeRequest, error)
}
