package handlers

import "net/http"

// registerPlayerOverviewRoute mounts GET /api/players/{id}/overview.
// Protected by clearanceAuth (league_admin/admin/system_admin) as of
// Player Overview Phase 2's money-integration correction: the route now
// exposes the same per-player dues/payment data Financial Phase 1 put
// behind clearanceAuth for its own routes, resolving the privacy
// inconsistency flagged when the money integration first landed (see
// PLAYERS-Q002 in doc/roadmap.md). The whole route is gated, not just
// the money field, per PM decision -- simpler and clearer than
// field-level auth, and this screen is admin-facing until real player
// login/permissions exist. Requires MatchMgr and RoundMgr (schedule and
// stats); only registered when both are wired, matching the nil-guard
// convention used by other cross-domain composition routes (e.g.
// registerSeasonCloseRoutes). financeMgr may be nil (money falls back
// to the Phase 1 placeholder); ruleMgr is always non-nil in production
// but is passed through rather than assumed. When applyAuth is nil
// (e.g. the shared testServer() test helper), clearanceAuth is a
// passthrough and the route remains open, matching every other
// clearanceAuth-protected route's behavior under that same test setup.
func registerPlayerOverviewRoute(
	mux *http.ServeMux,
	playerMgr PlayerManager, seasonMgr SeasonManager, teamMgr TeamManager,
	matchMgr MatchManager, roundMgr RoundManager, financeMgr FinanceManager, ruleMgr RuleManager,
	applyAuth ApplyAuthResolver,
) {
	mux.HandleFunc("GET /api/players/{id}/overview", clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
		getPlayerOverview(w, r, playerMgr, seasonMgr, teamMgr, matchMgr, roundMgr, financeMgr, ruleMgr)
	}))
}
