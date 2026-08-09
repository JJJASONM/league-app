package handlers

import "net/http"

// registerLineupRoutes mounts lineup plan routes onto mux: pre-game slot
// assignments per team/week. GET reads are unprotected; mutations are
// gated by clearanceAuth. Callers must guard on deps.LineupMgr != nil
// before calling, matching the existing registration guard in Register.
func registerLineupRoutes(mux *http.ServeMux, lineupMgr LineupManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/lineup-plans", func(w http.ResponseWriter, r *http.Request) {
		listLineupPlans(w, r, lineupMgr)
	})
	mux.HandleFunc("POST /api/lineup-plans",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			saveTeamLineup(w, r, lineupMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/lineup-plans/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deleteLineupPlan(w, r, lineupMgr)
		}),
	)
}
