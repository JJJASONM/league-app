package handlers

import "net/http"

// registerWeekRoutes mounts the week workflow routes onto mux: list, validate,
// close, reopen, acknowledgments, advance-preview, and recap. GET reads are
// unprotected; close and reopen are gated by clearanceAuth. Callers must
// guard on deps.WeekMgr != nil before calling, matching the existing
// registration guard in Register.
func registerWeekRoutes(mux *http.ServeMux, weekMgr WeekManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/seasons/{id}/weeks", func(w http.ResponseWriter, r *http.Request) {
		listWeeks(w, r, weekMgr)
	})
	mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/validate", func(w http.ResponseWriter, r *http.Request) {
		validateWeekHandler(w, r, weekMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/weeks/{week}/close",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			closeWeekHandler(w, r, weekMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/weeks/{week}/reopen",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			reopenWeekHandler(w, r, weekMgr)
		}),
	)
	mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/acknowledgments", func(w http.ResponseWriter, r *http.Request) {
		getWeekAcknowledgments(w, r, weekMgr)
	})
	mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/advance-preview", func(w http.ResponseWriter, r *http.Request) {
		getAdvancePreview(w, r, weekMgr)
	})
	mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/recap", func(w http.ResponseWriter, r *http.Request) {
		recapWeekHandler(w, r, weekMgr)
	})
}
