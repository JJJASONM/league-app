package handlers

import "net/http"

// registerTeamRoutes mounts team CRUD routes onto mux, scoped to ?league_id=.
// GET reads are unprotected; mutations are gated by clearanceAuth.
func registerTeamRoutes(mux *http.ServeMux, teamMgr TeamManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/teams", func(w http.ResponseWriter, r *http.Request) {
		listTeams(w, r, teamMgr)
	})
	mux.HandleFunc("POST /api/teams",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			createTeam(w, r, teamMgr)
		}),
	)
	mux.HandleFunc("GET /api/teams/{id}", func(w http.ResponseWriter, r *http.Request) {
		getTeam(w, r, teamMgr)
	})
	mux.HandleFunc("PUT /api/teams/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			updateTeam(w, r, teamMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/teams/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deleteTeam(w, r, teamMgr)
		}),
	)
}
