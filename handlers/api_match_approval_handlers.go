package handlers

import (
	"net/http"

	"league_app/backend/domains/matches"
	"league_app/models"
)

// approvalActorFromContext builds a matches.ApprovalActor from the identity
// requireMatchTeamApprovalAction stored in context. Callers only reach
// these handlers once that gate has already confirmed an active password
// session with a linked player, so identity and identity.PlayerID are
// always non-nil here.
func approvalActorFromContext(r *http.Request) matches.ApprovalActor {
	identity := identityFromContext(r.Context())
	userID := identity.UserID
	return matches.ApprovalActor{
		UserID:       &userID,
		PlayerID:     identity.PlayerID,
		NameSnapshot: identity.PlayerName,
	}
}

// decodeExpectedRevision decodes a body carrying at least expected_revision
// and reports whether it was present -- every team-approval mutation route
// requires it unconditionally (PM rule: "Team approval applies to one
// exact score_revision").
type expectedRevisionBody struct {
	ExpectedRevision *int `json:"expected_revision"`
}

// approveTeamSide handles POST /api/matches/{id}/approve-home and
// .../approve-away. side is fixed by which route registered the call, not
// read from the body.
func approveTeamSide(w http.ResponseWriter, r *http.Request, mgr RoundManager, side string) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req expectedRevisionBody
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.ExpectedRevision == nil {
		jsonError(w, "expected_revision is required", 400)
		return
	}
	if err := mgr.ApproveTeamSide(r.Context(), matchID, side, *req.ExpectedRevision, approvalActorFromContext(r)); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	writeApprovalSnapshot(w, r, mgr, matchID)
}

// withdrawTeamApproval handles POST /api/matches/{id}/withdraw-approval.
// side is not encoded in the path, so the body must carry it.
func withdrawTeamApproval(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req struct {
		Side             string `json:"side"`
		ExpectedRevision *int   `json:"expected_revision"`
	}
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.ExpectedRevision == nil {
		jsonError(w, "expected_revision is required", 400)
		return
	}
	if err := mgr.WithdrawTeamApproval(r.Context(), matchID, req.Side, *req.ExpectedRevision, approvalActorFromContext(r)); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	writeApprovalSnapshot(w, r, mgr, matchID)
}

// requestCorrection handles POST /api/matches/{id}/request-correction. side
// is not encoded in the path, so the body must carry it, along with a
// required, nonblank note (RequestCorrection itself validates blankness --
// CodeApprovalNoteRequired maps to 400 via writeMatchApprovalErr).
func requestCorrection(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req struct {
		Side             string `json:"side"`
		ExpectedRevision *int   `json:"expected_revision"`
		Note             string `json:"note"`
	}
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.ExpectedRevision == nil {
		jsonError(w, "expected_revision is required", 400)
		return
	}
	if err := mgr.RequestCorrection(r.Context(), matchID, req.Side, *req.ExpectedRevision, req.Note, approvalActorFromContext(r)); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	writeApprovalSnapshot(w, r, mgr, matchID)
}

// writeApprovalSnapshot fetches matchID's current approval state and writes
// it as the response body -- the "updated approval snapshot" every
// team-approval mutation route returns (PM rule: "Return the updated
// approval snapshot ... that gives the client the resulting authoritative
// state and current score revision").
func writeApprovalSnapshot(w http.ResponseWriter, r *http.Request, mgr RoundManager, matchID int64) {
	state, err := mgr.GetApprovalSnapshot(r.Context(), matchID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, toApprovalSnapshotModel(matchID, state))
}

// getMatchApproval handles GET /api/matches/{id}/approval.
func getMatchApproval(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	state, err := mgr.GetApprovalSnapshot(r.Context(), matchID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !state.Exists {
		jsonError(w, "match not found", http.StatusNotFound)
		return
	}
	jsonOK(w, toApprovalSnapshotModel(matchID, state))
}

// getMatchApprovalEvents handles GET /api/matches/{id}/approval-events.
func getMatchApprovalEvents(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	events, err := mgr.ListApprovalEvents(r.Context(), matchID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, events)
}

func toApprovalSnapshotModel(matchID int64, state matches.MatchApprovalState) models.MatchApprovalSnapshot {
	return models.MatchApprovalSnapshot{
		MatchID:       matchID,
		ScoreRevision: state.ScoreRevision,
		Completed:     state.Completed,
		ApprovedAt:    state.ApprovedAt,
		ProcessedAt:   state.ProcessedAt,
		Home:          toSideApprovalSnapshotModel(state.HomeSide),
		Away:          toSideApprovalSnapshotModel(state.AwaySide),
	}
}

func toSideApprovalSnapshotModel(side matches.TeamSideApprovalState) models.TeamSideApprovalSnapshot {
	return models.TeamSideApprovalSnapshot{
		State:              side.State,
		StateAt:            side.StateAt,
		ActorPlayerID:      side.ActorPlayerID,
		ActorNameSnapshot:  side.ActorNameSnapshot,
		StateScoreRevision: side.StateScoreRevision,
		CorrectionNote:     side.CorrectionNote,
	}
}
