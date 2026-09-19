package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerLineupRoutes mounts lineup plan routes onto mux: pre-game slot
// assignments per team/week. GET reads are unprotected; mutations are
// gated by guardedLeagueAdminAction, scoped via season (for the create/
// save route, whose body carries season_id) or via plan -> season ->
// league (for routes addressing an existing lineup_plans row by id).
// Callers must guard on deps.LineupMgr != nil before calling, matching
// the existing registration guard in Register.
func registerLineupRoutes(mux *http.ServeMux, deps Dependencies, lineupMgr LineupManager, seasonMgr SeasonManager) {
	planScope := lineupPlanIDPathScope(lineupMgr, seasonMgr)

	mux.HandleFunc("GET /api/lineup-plans", func(w http.ResponseWriter, r *http.Request) {
		listLineupPlans(w, r, lineupMgr)
	})
	mux.HandleFunc("POST /api/lineup-plans",
		guardedLeagueAdminAction(deps, auth.ActionLineupMutate, seasonIDBodyScope(seasonMgr), func(w http.ResponseWriter, r *http.Request) {
			saveTeamLineup(w, r, lineupMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/lineup-plans/{id}",
		guardedLeagueAdminAction(deps, auth.ActionLineupMutate, planScope, func(w http.ResponseWriter, r *http.Request) {
			deleteLineupPlan(w, r, lineupMgr)
		}),
	)
	// Substitute Workflow Phase 1: set/clear a substitute for an existing
	// lineup slot. Same guardedLeagueAdminAction gate as the other mutations.
	mux.HandleFunc("POST /api/lineup-plans/{id}/substitute",
		guardedLeagueAdminAction(deps, auth.ActionLineupMutate, planScope, func(w http.ResponseWriter, r *http.Request) {
			setLineupSubstitute(w, r, lineupMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/lineup-plans/{id}/substitute",
		guardedLeagueAdminAction(deps, auth.ActionLineupMutate, planScope, func(w http.ResponseWriter, r *http.Request) {
			clearLineupSubstitute(w, r, lineupMgr)
		}),
	)
}
