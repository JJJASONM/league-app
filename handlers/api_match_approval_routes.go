package handlers

import (
	"net/http"

	"league_app/backend/domains/matches"
)

// registerMatchApprovalRoutes mounts Player Score Approval Phase 1B's
// team-side approval mutation routes and protected approval reads. The
// mutation routes (approve-home, approve-away, withdraw-approval,
// request-correction) are exclusively player-scoped -- see
// requireMatchTeamApprovalAction's doc comment -- and carry no admin
// masquerade path; an administrator who needs to accept a match without
// both team approvals uses AdminAcceptMatch's override contract on the
// existing POST /api/matches/{id}/approve route instead. The read routes
// (approval snapshot, approval-event history) accept either a scoped
// administrator or an eligible linked player's session. Callers must guard
// on deps.RoundMgr != nil and deps.MatchMgr != nil before calling, matching
// the existing registration guards in Register.
func registerMatchApprovalRoutes(mux *http.ServeMux, deps Dependencies, roundMgr RoundManager, matchMgr MatchManager, seasonMgr SeasonManager) {
	mux.HandleFunc("POST /api/matches/{id}/approve-home",
		requireMatchTeamApprovalAction(deps, func(w http.ResponseWriter, r *http.Request) {
			approveTeamSide(w, r, roundMgr, matches.ApprovalSideHome)
		}),
	)
	mux.HandleFunc("POST /api/matches/{id}/approve-away",
		requireMatchTeamApprovalAction(deps, func(w http.ResponseWriter, r *http.Request) {
			approveTeamSide(w, r, roundMgr, matches.ApprovalSideAway)
		}),
	)
	mux.HandleFunc("POST /api/matches/{id}/withdraw-approval",
		requireMatchTeamApprovalAction(deps, func(w http.ResponseWriter, r *http.Request) {
			withdrawTeamApproval(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("POST /api/matches/{id}/request-correction",
		requireMatchTeamApprovalAction(deps, func(w http.ResponseWriter, r *http.Request) {
			requestCorrection(w, r, roundMgr)
		}),
	)

	mux.HandleFunc("GET /api/matches/{id}/approval",
		requireAdminOrEligiblePlayerRead(deps, matchMgr, seasonMgr, roundMgr, func(w http.ResponseWriter, r *http.Request) {
			getMatchApproval(w, r, roundMgr)
		}),
	)
	mux.HandleFunc("GET /api/matches/{id}/approval-events",
		requireAdminOrEligiblePlayerRead(deps, matchMgr, seasonMgr, roundMgr, func(w http.ResponseWriter, r *http.Request) {
			getMatchApprovalEvents(w, r, roundMgr)
		}),
	)
}
