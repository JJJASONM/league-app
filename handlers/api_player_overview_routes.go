package handlers

import "net/http"

// registerPlayerOverviewRoute mounts GET /api/players/{id}/overview.
// Protected by a personal API key (no static-token fallback) since Player
// Overview Phase 2's money-integration correction, resolving the privacy
// inconsistency flagged when the money integration first landed (see
// PLAYERS-Q002 in doc/roadmap.md). Player Account Access Phase 1 widened
// who may hold a key that passes this gate: system_admin/admin/league_admin
// may view any player's overview (unchanged), and a new role="player" may
// view only their own linked player's overview. Because that ownership
// check needs the requested player id from the URL path -- not available
// until Go's mux has matched the route -- this route uses
// requirePersonalKeyOnly (auth only, no fixed role set) rather than
// clearanceAuth, and getPlayerOverview itself enforces the role/ownership
// rule once it has parsed the path id. Requires MatchMgr and RoundMgr
// (schedule and stats); only registered when both are wired, matching the
// nil-guard convention used by other cross-domain composition routes (e.g.
// registerSeasonCloseRoutes). financeMgr may be nil (money falls back to
// the Phase 1 placeholder); ruleMgr is always non-nil in production but is
// passed through rather than assumed. When applyAuth is nil (e.g. the
// shared testServer() test helper), requirePersonalKeyOnly is a passthrough
// and the route remains open, matching every other personal-key-protected
// route's behavior under that same test setup.
func registerPlayerOverviewRoute(
	mux *http.ServeMux,
	playerMgr PlayerManager, seasonMgr SeasonManager, teamMgr TeamManager,
	matchMgr MatchManager, roundMgr RoundManager, financeMgr FinanceManager, ruleMgr RuleManager,
	applyAuth ApplyAuthResolver,
) {
	mux.HandleFunc("GET /api/players/{id}/overview", requirePersonalKeyOnly(applyAuth, func(w http.ResponseWriter, r *http.Request) {
		getPlayerOverview(w, r, playerMgr, seasonMgr, teamMgr, matchMgr, roundMgr, financeMgr, ruleMgr)
	}))
}

// requirePersonalKeyOnly wraps h with personal-key auth (no static-token
// fallback, matching every other clearance route) but does not restrict by
// a fixed set of roles the way clearanceAuth does -- used by routes that
// need per-resource ownership checks the middleware layer can't see (the
// resource id lives in the URL path, resolved by the handler, not by this
// wrapper). When resolver is nil, h is returned unmodified, matching
// clearanceAuth's nil-resolver passthrough for test/minimal-dependency
// setups.
func requirePersonalKeyOnly(resolver ApplyAuthResolver, h http.HandlerFunc) http.HandlerFunc {
	if resolver == nil {
		return h
	}
	return requirePersonalKeyAuth(resolver, h)
}
