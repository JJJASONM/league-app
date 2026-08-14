package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/seasons"
)

// Season Teams -----------------------------------------------------------------

func listSeasonTeams(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	teams, err := mgr.ListSeasonTeams(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, teams)
}

func addSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req seasons.AddTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	st, err := mgr.AddTeam(r.Context(), sid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, st)
}

func updateSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	var req seasons.UpdateTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	st, err := mgr.UpdateTeam(r.Context(), sid, tid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, st)
}

func removeSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	if err := mgr.RemoveTeam(r.Context(), sid, tid); err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

// mapSeasonErr translates seasons domain errors to HTTP responses.
func mapSeasonErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	switch {
	case errors.Is(err, seasons.ErrNotFound):
		jsonError(w, "season not found", http.StatusNotFound)
	case errors.As(err, &de):
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		case domainerr.Conflict:
			jsonError(w, de.Message, http.StatusConflict)
		case domainerr.Unprocessable:
			jsonError(w, de.Message, http.StatusUnprocessableEntity)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
	default:
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
}

// Season Rosters -----------------------------------------------------------------

func listSeasonRoster(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	entries, err := mgr.ListRoster(r.Context(), sid, tid)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, entries)
}

type addRosterPlayerRequest struct {
	PlayerID int64 `json:"player_id"`
}

func addRosterPlayer(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	var req addRosterPlayerRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.PlayerID == 0 {
		jsonError(w, "player_id is required", 400)
		return
	}
	entry, err := mgr.AddRosterPlayer(r.Context(), sid, tid, req.PlayerID)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, entry)
}

func removeRosterPlayer(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	pid, err := pathID(r, "pid")
	if err != nil {
		jsonError(w, "invalid player id", 400)
		return
	}
	if err := mgr.RemoveRosterPlayer(r.Context(), sid, tid, pid); err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

// Available Players --------------------------------------------------------------

func listAvailablePlayers(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	players, err := mgr.ListAvailablePlayers(r.Context(), sid)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, players)
}

// Previous Season ------------------------------------------------------------------

func getPreviousSeasonTeams(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	result, err := mgr.PreviousSeason(r.Context(), sid)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			jsonError(w, "season not found", 404)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, result)
}

// Setup Checklist --------------------------------------------------------------------

func getSeasonChecklist(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	c, err := mgr.Checklist(r.Context(), sid)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			jsonError(w, "season not found", 404)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, c)
}
