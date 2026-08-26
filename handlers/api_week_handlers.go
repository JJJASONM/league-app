package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
)

// Week Workflow ---------------------------------------------------------------

func listWeeks(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	summaries, err := mgr.ListWeeks(r.Context(), seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, summaries)
}

func validateWeekHandler(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}
	result, err := mgr.ValidateWeek(r.Context(), seasonID, weekNum)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, result)
}

func closeWeekHandler(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	type ackReq struct {
		Acknowledgments []matches.AckEntry `json:"acknowledgments"`
	}
	var body ackReq
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			jsonError(w, "invalid close week request body", http.StatusBadRequest)
			return
		}
	}

	result, err := mgr.CloseWeek(r.Context(), matches.CloseWeekRequest{
		SeasonID:          seasonID,
		WeekNumber:        weekNum,
		Acknowledgments:   body.Acknowledgments,
		ProcessedByUserID: approvingUserID(r),
	})
	if err != nil {
		var wce *matches.WeekCloseErr
		var de *domainerr.Err
		switch {
		case errors.As(err, &wce):
			jsonValidation(w, wce.Result)
		case errors.As(err, &de):
			switch de.Category {
			case domainerr.Conflict:
				jsonError(w, de.Message, http.StatusConflict)
			case domainerr.NotFound:
				jsonError(w, de.Message, http.StatusNotFound)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
		default:
			jsonError(w, err.Error(), 500)
		}
		return
	}

	// Best-effort advance result; close is already committed.
	ar, aerr := mgr.AdvanceData(r.Context(), seasonID, weekNum)
	if aerr != nil {
		jsonOK(w, map[string]any{
			"closed":               true,
			"week_number":          int(weekNum),
			"acknowledgment_count": result.AckCount,
			"processed_count":      result.ProcessedCount,
		})
		return
	}
	ar.Message = "Week closed. Standings and player stats now include this week's results."
	jsonOK(w, map[string]any{
		"closed":               true,
		"week_number":          int(weekNum),
		"acknowledgment_count": result.AckCount,
		"processed_count":      result.ProcessedCount,
		"advance_result":       ar,
	})
}

func reopenWeekHandler(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	if err := mgr.ReopenWeek(r.Context(), seasonID, weekNum); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) {
			switch de.Category {
			case domainerr.NotFound:
				jsonError(w, de.Message, http.StatusNotFound)
			case domainerr.Conflict:
				jsonError(w, de.Message, http.StatusConflict)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, map[string]any{"reopened": true, "week_number": int(weekNum)})
}

func getWeekAcknowledgments(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	acks, err := mgr.ListAcknowledgments(r.Context(), seasonID, weekNum)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.NotFound {
			jsonError(w, de.Message, http.StatusNotFound)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, acks)
}

func getAdvancePreview(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	preview, err := mgr.AdvancePreview(r.Context(), seasonID, weekNum)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.NotFound {
			jsonError(w, de.Message, http.StatusNotFound)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, preview)
}

func recapWeekHandler(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	recap, err := mgr.WeekRecap(r.Context(), seasonID, weekNum)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.NotFound {
			jsonError(w, de.Message, http.StatusNotFound)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, recap)
}
