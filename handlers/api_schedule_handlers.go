package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
)

func generateSchedule(w http.ResponseWriter, r *http.Request, mgr ScheduleManager) {
	var req matches.GenerateRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	result, err := mgr.GenerateSchedule(r.Context(), req)
	if err != nil {
		mapScheduleErr(w, err)
		return
	}
	jsonOK(w, result)
}

func pushbackPreview(w http.ResponseWriter, r *http.Request, mgr PushbackPreviewer) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", http.StatusBadRequest)
		return
	}
	var body struct {
		CutoffWeek int `json:"cutoff_week"`
		WeeksToAdd int `json:"weeks_to_add"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	result, err := mgr.Preview(r.Context(), matches.PushbackPreviewRequest{
		SeasonID:   seasonID,
		CutoffWeek: body.CutoffWeek,
		WeeksToAdd: body.WeeksToAdd,
	})
	if err != nil {
		mapScheduleErr(w, err)
		return
	}
	jsonOK(w, result)
}

func pushbackApply(w http.ResponseWriter, r *http.Request, mgr PushbackApplier) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", http.StatusBadRequest)
		return
	}
	var body struct {
		CutoffWeek int `json:"cutoff_week"`
		WeeksToAdd int `json:"weeks_to_add"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	result, err := mgr.Apply(r.Context(), matches.PushbackPreviewRequest{
		SeasonID:   seasonID,
		CutoffWeek: body.CutoffWeek,
		WeeksToAdd: body.WeeksToAdd,
	})
	if err != nil {
		mapScheduleErr(w, err)
		return
	}
	jsonOK(w, result)
}

// mapScheduleErr translates schedule domain errors to HTTP responses.
func mapScheduleErr(w http.ResponseWriter, err error) {
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
