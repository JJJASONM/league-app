package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/players"
	"league_app/models"
)

func listPlayers(w http.ResponseWriter, r *http.Request, mgr PlayerManager) {
	leagueID, hasLeague := qparamInt(r, "league_id")
	var lid *int64
	if hasLeague {
		lid = &leagueID
	}
	list, err := mgr.ListPlayers(r.Context(), lid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, list)
}

func createPlayer(w http.ResponseWriter, r *http.Request, mgr PlayerManager) {
	var body models.Player
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	created, err := mgr.CreatePlayer(r.Context(), players.CreatePlayerInput{
		PlayerNumber: body.PlayerNumber,
		FirstName:    body.FirstName,
		LastName:     body.LastName,
		Phone:        body.Phone,
		Email:        body.Email,
		TeamID:       body.TeamID,
		Handicap:     body.Handicap,
		AdminHold:    body.AdminHold,
	})
	if err != nil {
		mapPlayerErr(w, err)
		return
	}
	body.ID = created.ID
	body.Name = body.FirstName + " " + body.LastName
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, body)
}

func getPlayer(w http.ResponseWriter, r *http.Request, mgr PlayerManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	p, err := mgr.GetPlayer(r.Context(), id)
	if err != nil {
		mapPlayerErr(w, err)
		return
	}
	jsonOK(w, p)
}

func updatePlayer(w http.ResponseWriter, r *http.Request, mgr PlayerManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var body models.Player
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.UpdatePlayer(r.Context(), id, players.UpdatePlayerInput{
		FirstName: body.FirstName,
		LastName:  body.LastName,
		Phone:     body.Phone,
		Email:     body.Email,
		TeamID:    body.TeamID,
		Handicap:  body.Handicap,
		AdminHold: body.AdminHold,
	}); err != nil {
		mapPlayerErr(w, err)
		return
	}
	body.ID = id
	body.Name = body.FirstName + " " + body.LastName
	jsonOK(w, body)
}

func deletePlayer(w http.ResponseWriter, r *http.Request, mgr PlayerManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.DeletePlayer(r.Context(), id); err != nil {
		mapPlayerErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// mapPlayerErr translates player domain errors to HTTP responses.
func mapPlayerErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	switch {
	case errors.Is(err, players.ErrNotFound):
		jsonError(w, "player not found", http.StatusNotFound)
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
