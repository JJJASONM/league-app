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

	// ActionMatchTeamApproval is Player Score Approval Phase 1B's
	// player-scoped team-side action: approve, withdraw, or request
	// correction for one side of a match. Authorize deliberately checks
	// only the CREDENTIAL shape here (an active password session, a linked
	// player) -- per-match, per-side eligibility is a domain fact
	// (matches.RoundStore.ResolveApprovalEligibility) that a pure,
	// database-free policy function cannot evaluate, and the Phase 1A
	// service methods this action guards (ApproveTeamSide,
	// WithdrawTeamApproval, RequestCorrection) already enforce it
	// authoritatively and report the exact domain status (not found,
	// ineligible, stale revision, locked) themselves.
	//
	// PM correction: this action has NO admin bypass of any kind, not even
	// the otherwise-unconditional system_admin shortcut every other action
	// gets. Authorize evaluates ActionMatchTeamApproval BEFORE that
	// shortcut (see Authorize's own doc comment and implementation) --
	// role membership alone, including system_admin, must never authorize
	// a player-scoped action. A system_admin (or any other identity) may
	// use this action only by being signed in through an actual browser
	// session AND holding a linked, eligible player -- at that point they
	// are authorized as that player, not as an administrator. A
	// league/system administrator who needs to act without both team
	// approvals uses AdminAcceptMatch's own override contract on the
	// existing acceptance route instead of masquerading as a player.
	ActionMatchTeamApproval Action = "match_team_approval"

	// ActionMatchApprovalRead is Player Score Approval Phase 1B's
	// protected read for a match's approval snapshot/history and for
	// draft/in-progress round and result detail that is not yet public
	// (see the matches domain's read-authorization matrix). Scoped like
	// every other league-scoped admin action; a linked player's own
	// eligibility-based read access is layered on top at the HTTP layer,
	// the same way ActionMatchTeamApproval's eligibility is, since it is
	// likewise a database fact this pure policy function cannot evaluate.
	ActionMatchApprovalRead Action = "match_approval_read"
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
//   - ActionMatchTeamApproval is evaluated BEFORE the system_admin
//     shortcut below -- see that Action's own doc comment. This is the
//     one deliberate exception to the next rule: team approval is a
//     player-scoped action a role can never satisfy on its own, session
//     or not.
//   - system_admin is allowed everywhere else, unconditionally.
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

	// PM correction: ActionMatchTeamApproval must not inherit the
	// system_admin bypass below -- team approval requires an active
	// browser session AND a linked, eligible player, full stop. Checking
	// this before the bypass (rather than as a case inside the switch,
	// which the bypass would already have short-circuited past) is what
	// makes that true even for a system_admin identity.
	if action == ActionMatchTeamApproval {
		return identity.AuthMethod == AuthMethodSession && identity.IsPlayer()
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
		ActionMatchApprovalRead,
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
