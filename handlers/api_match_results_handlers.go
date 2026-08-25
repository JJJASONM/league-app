package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

func submitResults(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req models.SubmitResultsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.SubmitResults(r.Context(), id, req.Results); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.Conflict {
			jsonError(w, de.Message, http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "saved"})
}

func clearResults(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.ClearResults(r.Context(), id); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.Conflict {
			jsonError(w, de.Message, http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "cleared"})
}

func getStandings(w http.ResponseWriter, r *http.Request, roundMgr RoundManager, seasonMgr SeasonManager) {
	sid, ok := qparamInt(r, "season_id")
	if !ok {
		leagueID, lok := qparamInt(r, "league_id")
		if !lok {
			jsonOK(w, []models.Standing{})
			return
		}
		var found bool
		var err error
		sid, found, err = seasonMgr.FindActiveSeasonByLeague(r.Context(), leagueID)
		if err != nil || !found {
			jsonOK(w, []models.Standing{})
			return
		}
	}
	standings, err := roundMgr.GetStandings(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, standings)
}

// --- 8-Ball Round Results ---

func getRounds(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rounds, err := mgr.GetRounds(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, rounds)
}

func saveRounds(w http.ResponseWriter, r *http.Request, roundMgr RoundManager, seasonMgr SeasonManager) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req models.SaveRoundsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if sc, _ := roundMgr.IsSeasonClosedForMatch(r.Context(), matchID); sc {
		jsonError(w, "season is closed; this action is not allowed", http.StatusConflict)
		return
	}
	if ok, msg, err := seasonMgr.RosterEligible(r.Context(), matchID, 3); err == nil && !ok {
		jsonError(w, msg, http.StatusUnprocessableEntity)
		return
	}
	err = roundMgr.SaveRounds(r.Context(), matches.SaveRoundsInput{MatchID: matchID, Rounds: req.Rounds})
	if err != nil {
		var vErr *matches.RoundValidationError
		if errors.As(err, &vErr) {
			jsonValidation(w, vErr.Result.Result)
			return
		}
		var de *domainerr.Err
		if errors.As(err, &de) {
			switch de.Category {
			case domainerr.Conflict:
				jsonError(w, de.Message, http.StatusConflict)
			case domainerr.Unprocessable:
				jsonError(w, de.Message, http.StatusUnprocessableEntity)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"saved": len(req.Rounds)})
}

// --- Weekly Score Processing Phase 1A: match approval/processing ---

// approvingUserID returns the resolved clearance user's ID, or nil when no
// user is in context (e.g. clearanceAuth's nil-resolver test compatibility
// path). Admin-attested approval in Phase 1A does not require a user record.
func approvingUserID(r *http.Request) *int64 {
	u := clearanceUserFromContext(r.Context())
	if u == nil {
		return nil
	}
	id := u.ID
	return &id
}

func writeMatchApprovalErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	if errors.As(err, &de) {
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.Conflict:
			jsonError(w, de.Message, http.StatusConflict)
		case domainerr.Unprocessable:
			jsonError(w, de.Message, http.StatusUnprocessableEntity)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
		return
	}
	jsonError(w, err.Error(), http.StatusInternalServerError)
}

func approveMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	// Body is optional per the API contract (approve accepts {"note": "..."});
	// an empty or absent body simply leaves note blank.
	_ = decode(r, &req)
	if err := mgr.ApproveMatch(r.Context(), id, approvingUserID(r), req.Note); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "approved"})
}

func processMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.ProcessMatch(r.Context(), id, approvingUserID(r)); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "processed"})
}

func unapproveMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.UnapproveMatch(r.Context(), id); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "unapproved"})
}

func unprocessMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.UnprocessMatch(r.Context(), id); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "unprocessed"})
}

func getPlayerStats(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	var req matches.PlayerStatsRequest
	if sid, ok := qparamInt(r, "season_id"); ok {
		req.SeasonID = sid
	} else if lid, ok := qparamInt(r, "league_id"); ok {
		req.LeagueID = lid
	} else {
		jsonOK(w, []models.PlayerStat{})
		return
	}
	stats, err := mgr.GetPlayerStats(r.Context(), req)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, stats)
}
