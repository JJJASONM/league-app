package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/teams"
	"league_app/models"
)

func listTeams(w http.ResponseWriter, r *http.Request, mgr TeamManager) {
	leagueID, hasLeague := qparamInt(r, "league_id")
	var filter *int64
	if hasLeague {
		filter = &leagueID
	}
	ts, err := mgr.ListTeams(r.Context(), filter)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, ts)
}

func createTeam(w http.ResponseWriter, r *http.Request, mgr TeamManager) {
	var body models.Team
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	created, err := mgr.CreateTeam(r.Context(), teams.CreateTeamInput{
		Name:     body.Name,
		LeagueID: body.LeagueID,
	})
	if err != nil {
		mapTeamErr(w, err)
		return
	}
	body.ID = created.ID
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, body)
}

func getTeam(w http.ResponseWriter, r *http.Request, mgr TeamManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	t, err := mgr.GetTeam(r.Context(), id)
	if err != nil {
		mapTeamErr(w, err)
		return
	}
	jsonOK(w, t)
}

func updateTeam(w http.ResponseWriter, r *http.Request, mgr TeamManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var body models.Team
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.UpdateTeam(r.Context(), id, teams.UpdateTeamInput{
		Name:      body.Name,
		CaptainID: body.CaptainID,
	}); err != nil {
		mapTeamErr(w, err)
		return
	}
	body.ID = id
	jsonOK(w, body)
}

func deleteTeam(w http.ResponseWriter, r *http.Request, mgr TeamManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.DeleteTeam(r.Context(), id); err != nil {
		mapTeamErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func mapTeamErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	switch {
	case errors.Is(err, teams.ErrNotFound):
		jsonError(w, "team not found", http.StatusNotFound)
	case errors.As(err, &de):
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
	default:
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
}
