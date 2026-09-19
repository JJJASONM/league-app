package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerMatchRoutes mounts match read and assignment routes onto mux.
// GET reads are unprotected; the assign mutation is gated by
// guardedLeagueAdminAction, scoped via match -> season -> league. Callers
// must guard on deps.MatchMgr != nil before calling, matching the
// existing registration guard in Register.
func registerMatchRoutes(mux *http.ServeMux, deps Dependencies, matchMgr MatchManager, seasonMgr SeasonManager) {
	mux.HandleFunc("GET /api/matches", func(w http.ResponseWriter, r *http.Request) {
		listMatches(w, r, matchMgr)
	})
	mux.HandleFunc("GET /api/matches/{id}", func(w http.ResponseWriter, r *http.Request) {
		getMatch(w, r, matchMgr)
	})
	mux.HandleFunc("PATCH /api/matches/{id}/assign",
		guardedLeagueAdminAction(deps, auth.ActionMatchScoreMutate, matchIDPathScope(matchMgr, seasonMgr), func(w http.ResponseWriter, r *http.Request) {
			assignMatchTeams(w, r, matchMgr)
		}),
	)
}
