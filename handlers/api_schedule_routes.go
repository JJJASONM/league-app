package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerScheduleGenerateRoute mounts the schedule generation route.
// Mutation; gated by guardedLeagueAdminAction, scoped to the body's
// season_id (looked up, not trusted directly -- see seasonIDBodyScope).
// Callers must guard on deps.ScheduleMgr != nil before calling, matching
// the existing registration guard in Register.
func registerScheduleGenerateRoute(mux *http.ServeMux, deps Dependencies, scheduleMgr ScheduleManager, seasonMgr SeasonManager) {
	mux.HandleFunc("POST /api/matches/generate",
		guardedLeagueAdminAction(deps, auth.ActionScheduleMutate, seasonIDBodyScope(seasonMgr), func(w http.ResponseWriter, r *http.Request) {
			generateSchedule(w, r, scheduleMgr)
		}),
	)
}

// registerPushbackPreviewRoute mounts the schedule pushback preview route.
// Uses POST because the body carries cutoff/shift params, but computes a
// read-only preview -- gated the same as pushback-apply (scoped to the
// season's league) since it still exposes season schedule data across
// leagues if left open. Callers must guard on deps.PushbackMgr != nil
// before calling, matching the existing registration guard in Register.
func registerPushbackPreviewRoute(mux *http.ServeMux, deps Dependencies, pushbackMgr PushbackPreviewer, seasonMgr SeasonManager) {
	mux.HandleFunc("POST /api/seasons/{id}/schedule/pushback-preview",
		guardedLeagueAdminAction(deps, auth.ActionScheduleMutate, seasonIDPathScope(seasonMgr), func(w http.ResponseWriter, r *http.Request) {
			pushbackPreview(w, r, pushbackMgr)
		}),
	)
}

// registerPushbackApplyRoute mounts the schedule pushback apply route.
// Mutation; gated by guardedLeagueAdminAction, scoped to the season's
// league. Callers must guard on deps.PushbackApplyMgr != nil before
// calling, matching the existing registration guard in Register.
func registerPushbackApplyRoute(mux *http.ServeMux, deps Dependencies, pushbackApplyMgr PushbackApplier, seasonMgr SeasonManager) {
	mux.HandleFunc("POST /api/seasons/{id}/schedule/pushback-apply",
		guardedLeagueAdminAction(deps, auth.ActionScheduleMutate, seasonIDPathScope(seasonMgr), func(w http.ResponseWriter, r *http.Request) {
			pushbackApply(w, r, pushbackApplyMgr)
		}),
	)
}
