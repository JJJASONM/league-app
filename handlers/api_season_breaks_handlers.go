package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// Skipped Weeks --------------------------------------------------------------

func listSkippedWeeks(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weeks, err := mgr.ListSkippedWeeks(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, weeks)
}

func createSkippedWeek(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var sw models.SkippedWeek
	if err := decode(r, &sw); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	created, err := mgr.CreateSkippedWeek(r.Context(), sid, sw.SkipDate, sw.Reason)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.Conflict {
			jsonError(w, de.Message, http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, created)
}

func deleteSkippedWeek(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", 400)
		return
	}
	swid, err := pathID(r, "sid")
	if err != nil {
		jsonError(w, "invalid skip id", 400)
		return
	}
	if err := mgr.DeleteSkippedWeek(r.Context(), sid, swid); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// Bye Requests -----------------------------------------------------------------

func listByeRequests(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	byes, err := mgr.ListByeRequests(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, byes)
}

func createByeRequest(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req seasons.CreateByeRequestInput
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	b, err := mgr.CreateByeRequest(r.Context(), sid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, b)
}

func updateByeRequest(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", 400)
		return
	}
	bid, err := pathID(r, "bid")
	if err != nil {
		jsonError(w, "invalid bye id", 400)
		return
	}
	var body struct {
		Approved bool `json:"approved"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	b, err := mgr.UpdateByeRequest(r.Context(), sid, bid, body.Approved)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, b)
}

func deleteByeRequest(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", 400)
		return
	}
	bid, err := pathID(r, "bid")
	if err != nil {
		jsonError(w, "invalid bye id", 400)
		return
	}
	if err := mgr.DeleteByeRequest(r.Context(), sid, bid); err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}
