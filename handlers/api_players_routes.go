package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerPlayerRoutes mounts player CRUD routes onto mux, scoped to
// ?league_id=. GET reads are unprotected; mutations are gated by
// guardedLeagueAdminAction (session or Bearer, scoped to the player's
// owning league -- see api_scope_resolvers.go).
func registerPlayerRoutes(mux *http.ServeMux, deps Dependencies) {
	playerMgr := deps.PlayerMgr
	teamMgr := deps.TeamMgr
	mux.HandleFunc("GET /api/players", func(w http.ResponseWriter, r *http.Request) {
		listPlayers(w, r, playerMgr)
	})
	mux.HandleFunc("POST /api/players",
		guardedLeagueAdminAction(deps, auth.ActionRosterMutate, createPlayerScope(teamMgr), func(w http.ResponseWriter, r *http.Request) {
			createPlayer(w, r, playerMgr, teamMgr)
		}),
	)
	mux.HandleFunc("GET /api/players/{id}", func(w http.ResponseWriter, r *http.Request) {
		getPlayer(w, r, playerMgr)
	})
	mux.HandleFunc("PUT /api/players/{id}",
		guardedLeagueAdminAction(deps, auth.ActionRosterMutate, updatePlayerScope(playerMgr, teamMgr), func(w http.ResponseWriter, r *http.Request) {
			updatePlayer(w, r, playerMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/players/{id}",
		guardedLeagueAdminAction(deps, auth.ActionRosterMutate, playerIDPathScope(playerMgr), func(w http.ResponseWriter, r *http.Request) {
			deletePlayer(w, r, playerMgr)
		}),
	)
	mux.HandleFunc("POST /api/players/{id}/merge",
		guardedLeagueAdminAction(deps, auth.ActionRosterMutate, mergePlayerScope(playerMgr), func(w http.ResponseWriter, r *http.Request) {
			mergePlayer(w, r, playerMgr)
		}),
	)
}
