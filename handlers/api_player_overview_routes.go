package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerPlayerOverviewRoute mounts GET /api/players/{id}/overview.
// Protected by playerOverviewAuth, which accepts a session cookie or a
// Bearer personal API key (PM's Phase 1 correction review: a
// password-authenticated player logged into Player View could not
// previously load this route at all, since it was gated by
// requirePersonalKeyOnly -- Bearer-only). Access control: system_admin
// may view any player; a league_admin may view only players in a league
// they are assigned to; a linked player may view only their own overview;
// any other identity is forbidden. Requires MatchMgr and RoundMgr
// (schedule and stats); only registered when both are wired, matching the
// nil-guard convention used by other cross-domain composition routes (e.g.
// registerSeasonCloseRoutes). financeMgr may be nil (money falls back to
// the Phase 1 placeholder); ruleMgr is always non-nil in production but is
// passed through rather than assumed.
func registerPlayerOverviewRoute(
	mux *http.ServeMux,
	deps Dependencies,
	playerMgr PlayerManager, seasonMgr SeasonManager, teamMgr TeamManager,
	matchMgr MatchManager, roundMgr RoundManager, financeMgr FinanceManager, ruleMgr RuleManager,
) {
	mux.HandleFunc("GET /api/players/{id}/overview", playerOverviewAuth(deps, playerMgr, func(w http.ResponseWriter, r *http.Request) {
		getPlayerOverview(w, r, playerMgr, seasonMgr, teamMgr, matchMgr, roundMgr, financeMgr, ruleMgr)
	}))
}

// playerOverviewAuth resolves this route's per-resource scope (the
// requested player's own league AND their own player id -- Player
// Overview needs both: a league_admin's access depends on the player's
// league, a player's access depends on the player id matching their own
// link) and, when the scoping subsystem is wired, routes through the
// single centralized auth.Authorize policy via requireAction, exactly
// like every other guarded route in this phase. Uses the exact same
// two-condition gate as guardedAction (PM correction: the original
// version gated everything on `deps.ApplyAuth == nil` alone, which fell
// open for a session-only configuration): fully open only when NOTHING
// auth-related is wired at all; the flat, Bearer-only
// checkPlayerOverviewAccess fallback only when session support AND
// scoping were both never wired (ApplyAuth-only, test-only); otherwise
// always routed through requireAction, which handles a nil ApplyAuth
// gracefully on its own.
func playerOverviewAuth(deps Dependencies, playerMgr PlayerManager, next http.HandlerFunc) http.HandlerFunc {
	if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return next
	}
	if deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return requirePersonalKeyAuth(deps.ApplyAuth, func(w http.ResponseWriter, r *http.Request) {
			id, err := pathID(r, "id")
			if err != nil {
				jsonError(w, "invalid id", http.StatusBadRequest)
				return
			}
			if !checkPlayerOverviewAccess(r, id) {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
		})
	}
	scopeFn := func(r *http.Request) (auth.Scope, bool, error) {
		id, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		player, err := playerMgr.GetPlayer(r.Context(), id)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		leagueID := player.LeagueID
		return auth.Scope{LeagueID: &leagueID, PlayerID: &id}, true, nil
	}
	return requireAction(deps, auth.ActionPlayerOverviewOwn, scopeFn, next)
}
