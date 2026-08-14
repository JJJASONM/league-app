package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"league_app/backend/domains/seasons"
)

// Seasons -- scoped to league_id -----------------------------------------------

func listSeasons(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	leagueID, hasLeague := qparamInt(r, "league_id")
	var lid *int64
	if hasLeague {
		lid = &leagueID
	}
	seasons, err := mgr.ListSeasons(r.Context(), lid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, seasons)
}

func createSeason(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	var body struct {
		LeagueID     int64   `json:"league_id"`
		Name         string  `json:"name"`
		StartDate    *string `json:"start_date"`
		ScheduleType string  `json:"schedule_type"`
		NumWeeks     int     `json:"num_weeks"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	s, err := mgr.CreateSeason(r.Context(), seasons.CreateSeasonInput{
		LeagueID:     body.LeagueID,
		Name:         body.Name,
		StartDate:    body.StartDate,
		ScheduleType: body.ScheduleType,
		NumWeeks:     body.NumWeeks,
	})
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, s)
}

func getSeason(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	s, err := mgr.GetSeason(r.Context(), id)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, s)
}

func updateSeason(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var body struct {
		Name         string  `json:"name"`
		StartDate    *string `json:"start_date"`
		ScheduleType string  `json:"schedule_type"`
		NumWeeks     int     `json:"num_weeks"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	s, err := mgr.UpdateSeason(r.Context(), id, seasons.UpdateSeasonInput{
		Name:         body.Name,
		StartDate:    body.StartDate,
		ScheduleType: body.ScheduleType,
		NumWeeks:     body.NumWeeks,
	})
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, s)
}

func deleteSeason(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.DeleteSeason(r.Context(), id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func activateSeason(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.Activate(r.Context(), id); err != nil {
		var blockErr *seasons.ChecklistBlockErr
		switch {
		case errors.As(err, &blockErr):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    "season cannot be activated; resolve all blockers first",
				"blockers": blockErr.Blockers,
			})
		case errors.Is(err, seasons.ErrNotFound):
			jsonError(w, "season not found", 404)
		default:
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, map[string]string{"status": "activated"})
}
