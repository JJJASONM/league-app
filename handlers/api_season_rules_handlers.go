package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/models"
)

// Season Rules -------------------------------------------------------------

func listSeasonRules(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rows, err := mgr.List(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, rows)
}

func createSeasonRule(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var ru models.SeasonRule
	if err := decode(r, &ru); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	ru.SeasonID = sid
	saved, err := mgr.Upsert(r.Context(), ru)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.InvalidInput {
			jsonError(w, de.Message, http.StatusBadRequest)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, saved)
}

func updateSeasonRule(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	rid, err := pathID(r, "rid")
	if err != nil {
		jsonError(w, "invalid rule id", 400)
		return
	}
	var ru models.SeasonRule
	if err := decode(r, &ru); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.Update(r.Context(), rid, ru.RuleLabel, ru.RuleValue); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) {
			switch de.Category {
			case domainerr.NotFound:
				jsonError(w, de.Message, http.StatusNotFound)
			case domainerr.InvalidInput:
				jsonError(w, de.Message, http.StatusBadRequest)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	ru.ID = rid
	jsonOK(w, ru)
}

func deleteSeasonRule(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	rid, err := pathID(r, "rid")
	if err != nil {
		jsonError(w, "invalid rule id", 400)
		return
	}
	if err := mgr.Delete(r.Context(), rid); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}
