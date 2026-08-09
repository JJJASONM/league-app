package handlers

import "net/http"

// registerScheduleGenerateRoute mounts the schedule generation route.
// Mutation; gated by clearanceAuth. Callers must guard on
// deps.ScheduleMgr != nil before calling, matching the existing
// registration guard in Register.
func registerScheduleGenerateRoute(mux *http.ServeMux, scheduleMgr ScheduleManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("POST /api/matches/generate",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			generateSchedule(w, r, scheduleMgr)
		}),
	)
}

// registerPushbackPreviewRoute mounts the schedule pushback preview route.
// Uses POST because the body carries cutoff/shift params, but it performs
// no side effects and is intentionally left unprotected. Callers must guard
// on deps.PushbackMgr != nil before calling, matching the existing
// registration guard in Register.
func registerPushbackPreviewRoute(mux *http.ServeMux, pushbackMgr PushbackPreviewer) {
	mux.HandleFunc("POST /api/seasons/{id}/schedule/pushback-preview", func(w http.ResponseWriter, r *http.Request) {
		pushbackPreview(w, r, pushbackMgr)
	})
}

// registerPushbackApplyRoute mounts the schedule pushback apply route.
// Mutation; gated by clearanceAuth. Callers must guard on
// deps.PushbackApplyMgr != nil before calling, matching the existing
// registration guard in Register.
func registerPushbackApplyRoute(mux *http.ServeMux, pushbackApplyMgr PushbackApplier, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("POST /api/seasons/{id}/schedule/pushback-apply",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			pushbackApply(w, r, pushbackApplyMgr)
		}),
	)
}
