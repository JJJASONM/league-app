package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// listMatches handles GET /api/matches. This route predates Player Score
// Approval and has never been gated -- it stays that way (public schedule
// information remains public), but admin-attested approval metadata
// (approved_by_user_id, approval_note, processed_by_user_id) is stripped
// from every returned match unless the caller is authorized for the
// request's EFFECTIVE season_id/league_id filter (see
// canSeeApprovalMetadataInList's doc comment for the exact precedence and
// why this route cannot use per-match scoping the way getMatch does).
func listMatches(w http.ResponseWriter, r *http.Request, mgr MatchManager, seasonMgr SeasonManager, deps Dependencies) {
	req := matches.ListMatchesRequest{}
	if v, ok := qparamInt(r, "season_id"); ok {
		req.SeasonID = v
	}
	if v, ok := qparamInt(r, "league_id"); ok {
		req.LeagueID = v
	}
	ms, err := mgr.ListMatches(r.Context(), req)
	if err != nil {
		mapMatchErr(w, err)
		return
	}
	if !canSeeApprovalMetadataInList(r, deps, seasonMgr, req) {
		for i := range ms {
			stripApprovalMetadata(&ms[i])
		}
	}
	jsonOK(w, ms)
}

// getMatch handles GET /api/matches/{id}. Schedule fields (teams, date,
// week, completed) always stay public. Two DIFFERENT visibility rules
// apply, checked independently (PM correction: an earlier version stripped
// both together, guarded only by "not yet final," which meant a final
// closed match leaked approval metadata to every public caller):
//   - Results (draft score content): emptied unless the match is already
//     final (completed AND week_closed -- "public final closed results")
//     OR the caller is authorized.
//   - Approval metadata (approved_by_user_id, approval_note,
//     processed_by_user_id): stripped UNCONDITIONALLY unless the caller is
//     authorized, regardless of whether the match is final -- a closed
//     match's final results are public; who approved it and any
//     correction note are never public.
//
// "Authorized" is a scoped administrator for the match's own league, or an
// eligible session player for either side (Player Score Approval Phase
// 1B's read-authorization matrix). seasonMgr/deps resolve the match's
// owning league for that check; deps.RoundMgr may be nil (this route is
// registered before RoundMgr's presence is known), in which case only the
// admin branch can succeed.
func getMatch(w http.ResponseWriter, r *http.Request, mgr MatchManager, seasonMgr SeasonManager, deps Dependencies) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	detail, err := mgr.GetMatch(r.Context(), id)
	if err != nil {
		mapMatchErr(w, err)
		return
	}

	season, seasonErr := seasonMgr.GetSeason(r.Context(), detail.Match.SeasonID)
	leagueID := int64(0)
	if seasonErr == nil {
		leagueID = season.LeagueID
	}
	authorized := canSeeMatchDraftDetailWithLeague(r, deps, deps.RoundMgr, leagueID, id)

	final := detail.Match.Completed && detail.Match.WeekClosed
	if !final && !authorized {
		detail.Results = []models.MatchResult{}
	}
	if !authorized {
		stripApprovalMetadata(&detail.Match)
	}
	jsonOK(w, detail)
}

// stripApprovalMetadata clears the admin-attested approval/processing
// fields the read-authorization matrix reserves for scoped administrators
// and eligible session players -- see listMatches/getMatch. Schedule and
// score fields (teams, date, completed, week_closed) are untouched.
func stripApprovalMetadata(m *models.Match) {
	m.ApprovedAt = nil
	m.ApprovedByUserID = nil
	m.ApprovalNote = ""
	m.ProcessedAt = nil
	m.ProcessedByUserID = nil
}

// assignMatchTeams assigns home/away teams to a blanket (unassigned) match slot.
func assignMatchTeams(w http.ResponseWriter, r *http.Request, mgr MatchManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req models.AssignMatchTeamsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.AssignMatchTeams(r.Context(), id, req.HomeTeamID, req.AwayTeamID); err != nil {
		mapMatchErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "assigned"})
}

// mapMatchErr translates match domain errors to HTTP responses.
func mapMatchErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	if errors.As(err, &de) {
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		case domainerr.Conflict:
			jsonError(w, de.Message, http.StatusConflict)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
		return
	}
	jsonError(w, err.Error(), http.StatusInternalServerError)
}
