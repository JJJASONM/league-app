package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerMatchResultsRoutes mounts match results, rounds, standings, and
// player-stats routes onto mux. Kept together in one helper because all six
// routes share a single RoundManager guard and standings/player-stats are
// read directly from the same round-results data that the results/rounds
// mutations write -- splitting further would fragment one cohesive guard
// block without a clear ownership boundary.
//
// Score-entry/correction mutations (results, rounds) are gated by
// guardedLeagueAdminAction with ActionMatchScoreMutate; approval-workflow
// mutations (approve/process/unapprove/unprocess) use ActionMatchApproval.
// Both are scoped via match -> season -> league. GET reads are
// unprotected. The season-closed-before-RosterEligible check inside
// saveRounds is handler logic and is unaffected by this registration move.
// Callers must guard on deps.RoundMgr != nil before calling, matching the
// existing registration guard in Register.
func registerMatchResultsRoutes(mux *http.ServeMux, deps Dependencies, roundMgr RoundManager, seasonMgr SeasonManager, matchMgr MatchManager) {
	scope := matchIDPathScope(matchMgr, seasonMgr)

	mux.HandleFunc("POST /api/matches/{id}/results",
		guardedLeagueAdminAction(deps, auth.ActionMatchScoreMutate, scope, func(w http.ResponseWriter, r *http.Request) {
			submitResults(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/matches/{id}/results",
		guardedLeagueAdminAction(deps, auth.ActionMatchScoreMutate, scope, func(w http.ResponseWriter, r *http.Request) {
			clearResults(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("GET /api/matches/{id}/rounds", func(w http.ResponseWriter, r *http.Request) {
		getRounds(w, r, roundMgr)
	})
	mux.HandleFunc("POST /api/matches/{id}/rounds",
		guardedLeagueAdminAction(deps, auth.ActionMatchScoreMutate, scope, func(w http.ResponseWriter, r *http.Request) {
			saveRounds(w, r, roundMgr, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/standings", func(w http.ResponseWriter, r *http.Request) {
		getStandings(w, r, roundMgr, seasonMgr)
	})
	mux.HandleFunc("GET /api/player-stats", func(w http.ResponseWriter, r *http.Request) {
		getPlayerStats(w, r, roundMgr)
	})

	// Weekly Score Processing Phase 1A: match-level approval/processing.
	mux.HandleFunc("POST /api/matches/{id}/approve",
		guardedLeagueAdminAction(deps, auth.ActionMatchApproval, scope, func(w http.ResponseWriter, r *http.Request) {
			approveMatch(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/matches/{id}/process",
		guardedLeagueAdminAction(deps, auth.ActionMatchApproval, scope, func(w http.ResponseWriter, r *http.Request) {
			processMatch(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/matches/{id}/unapprove",
		guardedLeagueAdminAction(deps, auth.ActionMatchApproval, scope, func(w http.ResponseWriter, r *http.Request) {
			unapproveMatch(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/matches/{id}/unprocess",
		guardedLeagueAdminAction(deps, auth.ActionMatchApproval, scope, func(w http.ResponseWriter, r *http.Request) {
			unprocessMatch(w, r, roundMgr)
		}),
	)
}
