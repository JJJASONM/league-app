package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

func listMatches(w http.ResponseWriter, r *http.Request, mgr MatchManager) {
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
	jsonOK(w, ms)
}

func getMatch(w http.ResponseWriter, r *http.Request, mgr MatchManager) {
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
	jsonOK(w, detail)
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
