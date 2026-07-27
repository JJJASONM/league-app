package seasons

// Checklist and blocker codes emitted by computeChecklist and UpdateTeam.
// These values are returned in ChecklistItem.Code and domainerr codes and
// must remain stable because API consumers key on them.
const (
	ChecklistTeamsTooFew        = "TEAMS_TOO_FEW"
	ChecklistTeamNoPlayers      = "TEAM_NO_PLAYERS"
	ChecklistTeamFewPlayers     = "TEAM_FEW_PLAYERS"
	ChecklistTeamNoCaptain      = "TEAM_NO_CAPTAIN"
	ChecklistCaptainNotOnRoster = "CAPTAIN_NOT_ON_ROSTER"
	ChecklistScheduleStale      = "SCHEDULE_STALE"
	ChecklistNoSchedule         = "NO_SCHEDULE"
	ChecklistNoEndDate          = "NO_END_DATE"
)

// Season-close blocker and warning codes.
// Returned in SeasonClosePreview.Blockers / Warnings and as domainerr codes.
const (
	CodeSeasonCloseDraft      = "SEASON_CLOSE_DRAFT"
	CodeSeasonAlreadyClosed   = "SEASON_ALREADY_CLOSED"
	CodeSeasonCloseNoSchedule = "SEASON_CLOSE_NO_SCHEDULE"
	CodeSeasonCloseUnclosed   = "SEASON_CLOSE_UNCLOSED"
	CodeMissingResults        = "MISSING_RESULTS"
)

// CodeSeasonClosed is returned by mutation endpoints when the season is closed.
const CodeSeasonClosed = "SEASON_CLOSED"

// CodeSeasonNotClosed is returned by the reopen endpoint when the season is not closed.
const CodeSeasonNotClosed = "SEASON_NOT_CLOSED"
