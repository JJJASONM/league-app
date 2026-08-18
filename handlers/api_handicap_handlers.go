package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/handicaps"
)

// --- Handicap Review ---------------------------------------------------------

// getHandicapRecommendations handles GET /api/seasons/{id}/handicap-recommendations.
// Delegates to the handicaps.Service; translates domainerr.Category to HTTP status.
// No DB access in this handler; all logic lives in the service and adapter.
//
// Error mapping:
//   - domainerr.NotFound     -> 404 with the safe domain Message
//   - domainerr.InvalidInput -> 400 with the safe domain Message
//   - domainerr.Internal     -> 500 with the safe domain Message
//   - any non-domain error   -> 500 with fixed text "internal error" (no cause leak)
func getHandicapRecommendations(w http.ResponseWriter, r *http.Request, svc HandicapRecommender) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	resp, err := svc.Recommendations(r.Context(), seasonID)
	if err != nil {
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
		} else {
			// Non-domain error: never expose the cause to the client.
			jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	jsonOK(w, resp)
}

// --- Handicap Apply ----------------------------------------------------------

// applyEntryDTO is the handler-local JSON shape for one entry in an apply request.
// It uses pointer types so missing fields can be detected as nil.
// Never exported; conversion to handicaps.ApplyEntry happens in postHandicapApply.
type applyEntryDTO struct {
	PlayerID              *int64   `json:"player_id"`
	ExpectedAssignedHC    *float64 `json:"expected_assigned_hc"`
	ExpectedRecommendedHC *float64 `json:"expected_recommended_hc"`
	RecToken              *string  `json:"rec_token"`
}

// applyRequestDTO is the handler-local JSON shape for the apply request body.
type applyRequestDTO struct {
	ApplyRequestID *string         `json:"apply_request_id"`
	WeekNumber     *int            `json:"week_number,omitempty"`
	Entries        []applyEntryDTO `json:"entries"`
}

// isFiniteFloat mirrors isFiniteHC for handler-side validation of decoded floats.
func isFiniteFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// postHandicapApply handles POST /api/seasons/{id}/handicap-apply.
//
// Error mapping:
//   - domainerr.InvalidInput  -> 400
//   - domainerr.NotFound      -> 404
//   - domainerr.Conflict      -> 409
//   - domainerr.Unprocessable -> 422
//   - *ApplyConflictErr       -> 409
//   - *ApplyRejectionErr      -> 422
//   - domainerr.Internal      -> 500
func postHandicapApply(w http.ResponseWriter, r *http.Request, svc HandicapApplier) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	var dto applyRequestDTO
	if err := decode(r, &dto); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}

	// Validate required fields at the handler boundary.
	if dto.ApplyRequestID == nil {
		jsonError(w, "apply_request_id is required", 400)
		return
	}
	if dto.Entries == nil {
		jsonError(w, "entries is required", 400)
		return
	}

	entries := make([]handicaps.ApplyEntry, 0, len(dto.Entries))
	for i, e := range dto.Entries {
		if e.PlayerID == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: player_id is required", i), 400)
			return
		}
		if e.ExpectedAssignedHC == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_assigned_hc is required", i), 400)
			return
		}
		if !isFiniteFloat(*e.ExpectedAssignedHC) {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_assigned_hc must be finite", i), 400)
			return
		}
		if e.ExpectedRecommendedHC == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_recommended_hc is required", i), 400)
			return
		}
		if !isFiniteFloat(*e.ExpectedRecommendedHC) {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_recommended_hc must be finite", i), 400)
			return
		}
		if e.RecToken == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: rec_token is required", i), 400)
			return
		}
		entries = append(entries, handicaps.ApplyEntry{
			PlayerID:              *e.PlayerID,
			ExpectedAssignedHC:    *e.ExpectedAssignedHC,
			ExpectedRecommendedHC: *e.ExpectedRecommendedHC,
			RecToken:              *e.RecToken,
			AppliedByUserID:       applyUserIDFromContext(r.Context()),
		})
	}

	req := handicaps.ApplyRequest{
		ApplyRequestID: *dto.ApplyRequestID,
		WeekNumber:     dto.WeekNumber,
		Entries:        entries,
	}

	result, err := svc.Apply(r.Context(), seasonID, req)
	if err != nil {
		var conflictErr *handicaps.ApplyConflictErr
		var rejectionErr *handicaps.ApplyRejectionErr
		var de *domainerr.Err

		switch {
		case errors.As(err, &conflictErr):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":     "apply conflicts must be resolved before retrying",
				"conflicts": conflictErr.Conflicts,
			})
		case errors.As(err, &rejectionErr):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":      "one or more players are not eligible for apply",
				"rejections": rejectionErr.Rejections,
			})
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
			jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	jsonOK(w, result)
}
