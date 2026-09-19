package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerTeamRoutes mounts team CRUD routes onto mux, scoped to
// ?league_id=. GET reads are unprotected; mutations are gated by
// guardedLeagueAdminAction (session or Bearer, scoped to the team's
// owning league -- see api_scope_resolvers.go).
func registerTeamRoutes(mux *http.ServeMux, deps Dependencies) {
	teamMgr := deps.TeamMgr
	mux.HandleFunc("GET /api/teams", func(w http.ResponseWriter, r *http.Request) {
		listTeams(w, r, teamMgr)
	})
	mux.HandleFunc("POST /api/teams",
		guardedLeagueAdminAction(deps, auth.ActionRosterMutate, createTeamScope, func(w http.ResponseWriter, r *http.Request) {
			createTeam(w, r, teamMgr)
		}),
	)
	mux.HandleFunc("GET /api/teams/{id}", func(w http.ResponseWriter, r *http.Request) {
		getTeam(w, r, teamMgr)
	})
	mux.HandleFunc("PUT /api/teams/{id}",
		guardedLeagueAdminAction(deps, auth.ActionRosterMutate, teamIDPathScope(teamMgr), func(w http.ResponseWriter, r *http.Request) {
			updateTeam(w, r, teamMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/teams/{id}",
		guardedLeagueAdminAction(deps, auth.ActionRosterMutate, teamIDPathScope(teamMgr), func(w http.ResponseWriter, r *http.Request) {
			deleteTeam(w, r, teamMgr)
		}),
	)
}
