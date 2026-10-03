package handlers

import "net/http"

// registerPlayerEligibleMatchesRoute mounts GET
// /api/players/{id}/eligible-matches -- Player Score Approval Phase 1C's
// "My Matches" list: every match the requested player is eligible to act
// on for at least one side, in their own league's active season (or an
// explicit ?season_id=). Reuses playerOverviewAuth completely unchanged --
// the access model is identical to Player Overview: system_admin may view
// any player's list, a league_admin may view only players in a league
// they administer, a linked player may view only their own list, any
// other identity is forbidden. Only registered when RoundMgr is wired
// (mirrors registerPlayerOverviewRoute's nil-guard convention, since
// eligibility and approval state both live behind RoundManager).
func registerPlayerEligibleMatchesRoute(
	mux *http.ServeMux,
	deps Dependencies,
	playerMgr PlayerManager, seasonMgr SeasonManager, roundMgr RoundManager,
) {
	mux.HandleFunc("GET /api/players/{id}/eligible-matches", playerOverviewAuth(deps, playerMgr, func(w http.ResponseWriter, r *http.Request) {
		getPlayerEligibleMatches(w, r, playerMgr, seasonMgr, roundMgr)
	}))
}
