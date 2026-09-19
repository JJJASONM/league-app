package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerFinanceRoutes mounts dues and payout routes onto mux, scoped to
// a season. Unlike most other domains, ALL four routes here (reads and
// writes) are gated by guardedLeagueAdminAction (league_admin/admin/
// system_admin) -- per PM decision, money data is not made public just
// because other domain reads are open. Reads use ActionFinanceRead,
// writes use ActionFinanceWrite, both scoped to the season's league.
func registerFinanceRoutes(mux *http.ServeMux, deps Dependencies, financeMgr FinanceManager, seasonMgr SeasonManager, ruleMgr RuleManager, roundMgr RoundManager) {
	scope := seasonIDPathScope(seasonMgr)

	mux.HandleFunc("GET /api/seasons/{id}/finances/dues",
		guardedLeagueAdminAction(deps, auth.ActionFinanceRead, scope, func(w http.ResponseWriter, r *http.Request) {
			getSeasonDues(w, r, financeMgr, seasonMgr, ruleMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/finances/dues-payments",
		guardedLeagueAdminAction(deps, auth.ActionFinanceWrite, scope, func(w http.ResponseWriter, r *http.Request) {
			postDuesPayment(w, r, financeMgr, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/seasons/{id}/finances/payouts",
		guardedLeagueAdminAction(deps, auth.ActionFinanceRead, scope, func(w http.ResponseWriter, r *http.Request) {
			getSeasonPayouts(w, r, financeMgr, seasonMgr, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/finances/payouts",
		guardedLeagueAdminAction(deps, auth.ActionFinanceWrite, scope, func(w http.ResponseWriter, r *http.Request) {
			postPayout(w, r, financeMgr, seasonMgr)
		}),
	)
}
