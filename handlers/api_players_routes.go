package handlers

import "net/http"

// registerPlayerRoutes mounts player CRUD routes onto mux, scoped to ?league_id=.
// GET reads are unprotected; mutations are gated by clearanceAuth.
func registerPlayerRoutes(mux *http.ServeMux, playerMgr PlayerManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/players", func(w http.ResponseWriter, r *http.Request) {
		listPlayers(w, r, playerMgr)
	})
	mux.HandleFunc("POST /api/players",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			createPlayer(w, r, playerMgr)
		}),
	)
	mux.HandleFunc("GET /api/players/{id}", func(w http.ResponseWriter, r *http.Request) {
		getPlayer(w, r, playerMgr)
	})
	mux.HandleFunc("PUT /api/players/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			updatePlayer(w, r, playerMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/players/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deletePlayer(w, r, playerMgr)
		}),
	)
	mux.HandleFunc("POST /api/players/{id}/merge",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			mergePlayer(w, r, playerMgr)
		}),
	)
}
