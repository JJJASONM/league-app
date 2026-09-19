package handlers

import (
	"errors"
	"log"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/leagues"
	"league_app/models"
)

func listLeagues(w http.ResponseWriter, r *http.Request, mgr LeagueManager) {
	ls, err := mgr.ListLeagues(r.Context())
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, ls)
}

func createLeague(w http.ResponseWriter, r *http.Request, mgr LeagueManager) {
	var body struct {
		Name       string `json:"name"`
		GameFormat string `json:"game_format"`
		DayOfWeek  string `json:"day_of_week"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	l, err := mgr.CreateLeague(r.Context(), leagues.CreateLeagueInput{
		Name:       body.Name,
		GameFormat: body.GameFormat,
		DayOfWeek:  body.DayOfWeek,
	})
	if err != nil {
		mapLeagueErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, l)
}

// createLeagueWithSelfGrant is createLeague plus the Users/Roles Phase 1
// rule: "League creation atomically grants the creator league_admin
// access to the new league. Other league admins do not automatically
// receive access to that league." (product decisions 10-12). The league
// insert and the creator's role_assignments grant commit together in a
// single database transaction (sqlite.LeagueSelfGrantStore) -- either both
// happen or neither does, with no compensating-delete crash window.
func createLeagueWithSelfGrant(w http.ResponseWriter, r *http.Request, mgr LeagueSelfGrantManager) {
	actor := identityFromContext(r.Context())
	if actor == nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	var body struct {
		Name       string `json:"name"`
		GameFormat string `json:"game_format"`
		DayOfWeek  string `json:"day_of_week"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	l, err := mgr.CreateLeagueWithLeagueAdminGrant(r.Context(), leagues.CreateLeagueInput{
		Name:       body.Name,
		GameFormat: body.GameFormat,
		DayOfWeek:  body.DayOfWeek,
	}, actor.UserID)
	if err != nil {
		log.Printf("createLeagueWithSelfGrant: %v", err)
		jsonError(w, "could not create league", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, l)
}

func getLeague(w http.ResponseWriter, r *http.Request, mgr LeagueManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	l, err := mgr.GetLeague(r.Context(), id)
	if err != nil {
		mapLeagueErr(w, err)
		return
	}
	jsonOK(w, l)
}

func updateLeague(w http.ResponseWriter, r *http.Request, mgr LeagueManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var body struct {
		Name       string `json:"name"`
		GameFormat string `json:"game_format"`
		DayOfWeek  string `json:"day_of_week"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.UpdateLeague(r.Context(), id, leagues.UpdateLeagueInput{
		Name:       body.Name,
		GameFormat: body.GameFormat,
		DayOfWeek:  body.DayOfWeek,
	}); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, models.League{ID: id, Name: body.Name, GameFormat: body.GameFormat, DayOfWeek: body.DayOfWeek})
}

func deleteLeague(w http.ResponseWriter, r *http.Request, mgr LeagueManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.DeleteLeague(r.Context(), id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// mapLeagueErr translates league domain errors to HTTP responses.
func mapLeagueErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	switch {
	case errors.Is(err, leagues.ErrNotFound):
		jsonError(w, "league not found", http.StatusNotFound)
	case errors.As(err, &de):
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
	default:
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
}
