package handlers

import "net/http"

// registerPlayerOverviewRoute mounts GET /api/players/{id}/overview.
// Unprotected read, consistent with every other GET in the app -- no new
// auth for Player Overview Phase 1. Requires MatchMgr and RoundMgr
// (schedule and stats); only registered when both are wired, matching the
// nil-guard convention used by other cross-domain composition routes
// (e.g. registerSeasonCloseRoutes).
func registerPlayerOverviewRoute(
	mux *http.ServeMux,
	playerMgr PlayerManager, seasonMgr SeasonManager, teamMgr TeamManager,
	matchMgr MatchManager, roundMgr RoundManager,
) {
	mux.HandleFunc("GET /api/players/{id}/overview", func(w http.ResponseWriter, r *http.Request) {
		getPlayerOverview(w, r, playerMgr, seasonMgr, teamMgr, matchMgr, roundMgr)
	})
}
