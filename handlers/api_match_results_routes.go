package handlers

import "net/http"

// registerMatchResultsRoutes mounts match results, rounds, standings, and
// player-stats routes onto mux. Kept together in one helper because all six
// routes share a single RoundManager guard and standings/player-stats are
// read directly from the same round-results data that the results/rounds
// mutations write -- splitting further would fragment one cohesive guard
// block without a clear ownership boundary.
//
// Mutations are gated by clearanceAuth; GET reads are unprotected. The
// season-closed-before-RosterEligible check inside saveRounds is handler
// logic and is unaffected by this registration move. Callers must guard on
// deps.RoundMgr != nil before calling, matching the existing registration
// guard in Register.
func registerMatchResultsRoutes(mux *http.ServeMux, roundMgr RoundManager, seasonMgr SeasonManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("POST /api/matches/{id}/results",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			submitResults(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/matches/{id}/results",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			clearResults(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("GET /api/matches/{id}/rounds", func(w http.ResponseWriter, r *http.Request) {
		getRounds(w, r, roundMgr)
	})
	mux.HandleFunc("POST /api/matches/{id}/rounds",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			saveRounds(w, r, roundMgr, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/standings", func(w http.ResponseWriter, r *http.Request) {
		getStandings(w, r, roundMgr, seasonMgr)
	})
	mux.HandleFunc("GET /api/player-stats", func(w http.ResponseWriter, r *http.Request) {
		getPlayerStats(w, r, roundMgr)
	})
}
