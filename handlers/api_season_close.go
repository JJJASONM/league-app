package handlers

import (
	"net/http"
)

// reopenSeasonHandler handles POST /api/seasons/{id}/reopen.
// Clears closed_at on a closed season, returning it to Historical state.
// Returns 409 Conflict when the season is not closed.
// Returns 404 when the season does not exist.
func reopenSeasonHandler(w http.ResponseWriter, r *http.Request, seasonMgr SeasonManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	season, err := seasonMgr.ReopenSeason(r.Context(), seasonID)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, season)
}

// closeSeasonPreviewHandler handles GET /api/seasons/{id}/close-preview.
// Returns the read-only validation state: can_close, blockers, warnings,
// standings preview, missing_results count, and unclosed_weeks list.
func closeSeasonPreviewHandler(
	w http.ResponseWriter,
	r *http.Request,
	seasonMgr SeasonManager,
	weekMgr WeekManager,
	roundMgr RoundManager,
) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	weeks, err := weekMgr.ListWeeks(r.Context(), seasonID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	standings, err := roundMgr.GetStandings(r.Context(), seasonID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	preview, err := seasonMgr.ClosePreview(r.Context(), seasonID, weeks, standings)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, preview)
}

// closeSeasonHandler handles POST /api/seasons/{id}/close.
// Validates all blockers, stores the final standings snapshot, sets closed_at,
// and deactivates the season when it is currently active.
// Returns the updated season on success, or a domain error response on failure.
func closeSeasonHandler(
	w http.ResponseWriter,
	r *http.Request,
	seasonMgr SeasonManager,
	weekMgr WeekManager,
	roundMgr RoundManager,
) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	weeks, err := weekMgr.ListWeeks(r.Context(), seasonID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	standings, err := roundMgr.GetStandings(r.Context(), seasonID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	season, err := seasonMgr.CloseSeason(r.Context(), seasonID, weeks, standings)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, season)
}
