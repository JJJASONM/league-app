package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerLeagueRoutes mounts league CRUD routes onto mux. GET reads are
// unprotected; mutations are gated by guardedLeagueAdminAction (session or
// Bearer, routed through the centralized auth.Authorize policy whenever
// the scoping subsystem is wired -- see guardedAction's doc comment).
// PUT/DELETE are scoped to the league's own id; POST (create) additionally
// performs the atomic creator self-grant (see createLeagueWithSelfGrant)
// whenever deps.LeagueSelfGrantMgr is wired, which it always is in
// production alongside the rest of the auth domain.
func registerLeagueRoutes(mux *http.ServeMux, deps Dependencies) {
	leagueMgr := deps.LeagueMgr

	mux.HandleFunc("GET /api/leagues", func(w http.ResponseWriter, r *http.Request) {
		listLeagues(w, r, leagueMgr)
	})

	if deps.LeagueSelfGrantMgr != nil {
		mux.HandleFunc("POST /api/leagues",
			guardedLeagueAdminAction(deps, auth.ActionLeagueCreate, noScope, func(w http.ResponseWriter, r *http.Request) {
				createLeagueWithSelfGrant(w, r, deps.LeagueSelfGrantMgr)
			}),
		)
	} else {
		mux.HandleFunc("POST /api/leagues",
			guardedLeagueAdminAction(deps, auth.ActionLeagueCreate, noScope, func(w http.ResponseWriter, r *http.Request) {
				createLeague(w, r, leagueMgr)
			}),
		)
	}

	mux.HandleFunc("GET /api/leagues/{id}", func(w http.ResponseWriter, r *http.Request) {
		getLeague(w, r, leagueMgr)
	})
	mux.HandleFunc("PUT /api/leagues/{id}",
		guardedLeagueAdminAction(deps, auth.ActionLeagueAdminister, leagueIDPathScope, func(w http.ResponseWriter, r *http.Request) {
			updateLeague(w, r, leagueMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/leagues/{id}",
		guardedLeagueAdminAction(deps, auth.ActionLeagueAdminister, leagueIDPathScope, func(w http.ResponseWriter, r *http.Request) {
			deleteLeague(w, r, leagueMgr)
		}),
	)
}
