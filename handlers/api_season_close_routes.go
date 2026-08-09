package handlers

import "net/http"

// registerSeasonCloseRoutes mounts season close-preview, close, and reopen
// routes onto mux. Requires both a WeekManager (ListWeeks) and a
// RoundManager (GetStandings); callers must guard on
// deps.WeekMgr != nil && deps.RoundMgr != nil before calling, matching the
// existing registration guard in Register.
func registerSeasonCloseRoutes(mux *http.ServeMux, seasonMgr SeasonManager, weekMgr WeekManager, roundMgr RoundManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/seasons/{id}/close-preview", func(w http.ResponseWriter, r *http.Request) {
		closeSeasonPreviewHandler(w, r, seasonMgr, weekMgr, roundMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/close",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			closeSeasonHandler(w, r, seasonMgr, weekMgr, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/reopen",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			reopenSeasonHandler(w, r, seasonMgr)
		}),
	)
}
