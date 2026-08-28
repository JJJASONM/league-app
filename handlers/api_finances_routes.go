package handlers

import "net/http"

// registerFinanceRoutes mounts dues and payout routes onto mux, scoped to
// a season. Unlike most other domains, ALL four routes here (reads and
// writes) are gated by clearanceAuth (league_admin/admin/system_admin) --
// per PM decision, money data is not made public just because other
// domain reads are open.
func registerFinanceRoutes(mux *http.ServeMux, financeMgr FinanceManager, seasonMgr SeasonManager, ruleMgr RuleManager, roundMgr RoundManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/seasons/{id}/finances/dues",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			getSeasonDues(w, r, financeMgr, seasonMgr, ruleMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/finances/dues-payments",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			postDuesPayment(w, r, financeMgr, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/seasons/{id}/finances/payouts",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			getSeasonPayouts(w, r, financeMgr, seasonMgr, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/finances/payouts",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			postPayout(w, r, financeMgr, seasonMgr)
		}),
	)
}
