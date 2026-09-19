package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerSeasonCloseRoutes mounts season close-preview, close, and reopen
// routes onto mux. Requires both a WeekManager (ListWeeks) and a
// RoundManager (GetStandings); callers must guard on
// deps.WeekMgr != nil && deps.RoundMgr != nil before calling, matching the
// existing registration guard in Register. close and reopen are gated by
// guardedLeagueAdminAction, scoped to the season's league.
func registerSeasonCloseRoutes(mux *http.ServeMux, deps Dependencies, seasonMgr SeasonManager, weekMgr WeekManager, roundMgr RoundManager) {
	scope := seasonIDPathScope(seasonMgr)

	mux.HandleFunc("GET /api/seasons/{id}/close-preview", func(w http.ResponseWriter, r *http.Request) {
		closeSeasonPreviewHandler(w, r, seasonMgr, weekMgr, roundMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/close",
		guardedLeagueAdminAction(deps, auth.ActionSeasonSetup, scope, func(w http.ResponseWriter, r *http.Request) {
			closeSeasonHandler(w, r, seasonMgr, weekMgr, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/reopen",
		guardedLeagueAdminAction(deps, auth.ActionSeasonSetup, scope, func(w http.ResponseWriter, r *http.Request) {
			reopenSeasonHandler(w, r, seasonMgr)
		}),
	)
}
