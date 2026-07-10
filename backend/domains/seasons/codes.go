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
