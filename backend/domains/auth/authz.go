package auth

// Action names a single authorizable capability. Every new route this
// phase adds, and every existing route this phase rewires, calls Authorize
// with one of these rather than checking identity.Assignments or a raw
// role string directly -- "hiding a screen is not authorization; backend
// enforcement is required" (PM decision), and a single entry point is what
// makes that enforcement reviewable in one place instead of scattered
// through handlers.
type Action string

const (
	ActionSystemUserAdmin     Action = "system_user_admin"
	ActionBackup              Action = "backup"
	ActionLeagueCreate        Action = "league_create"
	ActionLeagueAdminister    Action = "league_administer"
	ActionSeasonSetup         Action = "season_setup"
	ActionRosterMutate        Action = "roster_mutate"
	ActionScheduleMutate      Action = "schedule_mutate"
	ActionLineupMutate        Action = "lineup_mutate"
	ActionMatchScoreMutate    Action = "match_score_mutate"
	ActionMatchApproval       Action = "match_approval"
	ActionWeekCloseReopen     Action = "week_close_reopen"
	ActionFinanceRead         Action = "finance_read"
	ActionFinanceWrite        Action = "finance_write"
	ActionHandicapApply       Action = "handicap_apply"
	ActionPlayerOverviewOwn   Action = "player_overview_own"
	ActionPlayerOverviewOther Action = "player_overview_other"
)

// Scope names the resource an Action targets. LeagueID is the authoritative
// league for league-scoped actions -- callers resolve it from whatever
// identifier the route actually receives (season_id, match_id, team_id,
// player_id, or league_id directly) before calling Authorize; Authorize
// itself never does that resolution, so it stays a pure, easily-tested
// policy function. PlayerID is only relevant to the player-overview actions.
type Scope struct {
	LeagueID *int64
	PlayerID *int64
}

// Authorize is the single centralized authorization policy entry point.
// It returns true when identity may perform action against scope. Every
// case is deliberately simple and reviewable in one place:
//
//   - An inactive identity is never authorized, for any action.
//   - system_admin is allowed everywhere, unconditionally.
//   - Global (non-league-scoped) admin actions require system_admin.
//   - League-scoped admin actions require system_admin OR league_admin
//     for that exact league -- there is no partial/inherited scope.
//   - League creation is allowed for system_admin OR any existing
//     league_admin (see Identity.IsAnyLeagueAdmin's doc comment for why).
//   - Player-overview actions add a narrow ownership carve-out: a player
//     may always view their own overview (PlayerID match), never anyone
//     else's, regardless of role.
func Authorize(identity Identity, action Action, scope Scope) bool {
	if !identity.Active {
		return false
	}
	if identity.IsSystemAdmin() {
		return true
	}

	switch action {
	case ActionSystemUserAdmin, ActionBackup:
		return false // system_admin-only; already granted above if applicable

	case ActionLeagueCreate:
		return identity.IsAnyLeagueAdmin()

	case ActionLeagueAdminister,
		ActionSeasonSetup,
		ActionRosterMutate,
		ActionScheduleMutate,
		ActionLineupMutate,
		ActionMatchScoreMutate,
		ActionMatchApproval,
		ActionWeekCloseReopen,
		ActionFinanceRead,
		ActionFinanceWrite,
		ActionHandicapApply:
		return scope.LeagueID != nil && identity.IsLeagueAdminFor(*scope.LeagueID)

	case ActionPlayerOverviewOwn:
		if scope.LeagueID != nil && identity.IsLeagueAdminFor(*scope.LeagueID) {
			return true
		}
		return identity.IsPlayer() && scope.PlayerID != nil && *identity.PlayerID == *scope.PlayerID

	case ActionPlayerOverviewOther:
		return scope.LeagueID != nil && identity.IsLeagueAdminFor(*scope.LeagueID)

	default:
		return false
	}
}
