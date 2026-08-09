package handlers

import "net/http"

// registerLeagueRoutes mounts league CRUD routes onto mux.
// GET reads are unprotected; mutations are gated by clearanceAuth.
func registerLeagueRoutes(mux *http.ServeMux, leagueMgr LeagueManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/leagues", func(w http.ResponseWriter, r *http.Request) {
		listLeagues(w, r, leagueMgr)
	})
	mux.HandleFunc("POST /api/leagues",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			createLeague(w, r, leagueMgr)
		}),
	)
	mux.HandleFunc("GET /api/leagues/{id}", func(w http.ResponseWriter, r *http.Request) {
		getLeague(w, r, leagueMgr)
	})
	mux.HandleFunc("PUT /api/leagues/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			updateLeague(w, r, leagueMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/leagues/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deleteLeague(w, r, leagueMgr)
		}),
	)
}
