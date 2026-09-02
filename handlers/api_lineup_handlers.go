package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

func listLineupPlans(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	seasonID, hasSeason := qparamInt(r, "season_id")
	if !hasSeason {
		jsonError(w, "season_id required", 400)
		return
	}
	req := matches.ListLineupPlansRequest{SeasonID: seasonID}
	if v, ok := qparamInt(r, "week_number"); ok {
		req.WeekNumber = v
	}
	if v, ok := qparamInt(r, "team_id"); ok {
		req.TeamID = v
	}
	plans, err := mgr.ListLineupPlans(r.Context(), req)
	if err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, plans)
}

// saveTeamLineup atomically replaces all lineup slots for one team/week.
// Body: { season_id, team_id, week_number, player_ids: [id1, id2, id3] }
func saveTeamLineup(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	var req models.SaveTeamLineupRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.SeasonID == 0 || req.TeamID == 0 {
		jsonError(w, "season_id and team_id required", 400)
		return
	}
	if err := mgr.SaveTeamLineup(r.Context(), matches.SaveLineupRequest{
		SeasonID:   req.SeasonID,
		TeamID:     req.TeamID,
		WeekNumber: int64(req.WeekNumber),
		PlayerIDs:  req.PlayerIDs,
	}); err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "saved"})
}

func deleteLineupPlan(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.DeleteLineupPlan(r.Context(), id); err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// setLineupSubstitute handles POST /api/lineup-plans/{id}/substitute
// (Substitute Workflow Phase 1). Body: { substitute_player_id }. Rejected
// with 409 when the team's match for this season/week is season-closed,
// week-closed, approved, or processed -- the same lock set score edits
// respect, since a substitute swap changes who is credited for a match.
func setLineupSubstitute(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var body struct {
		SubstitutePlayerID int64 `json:"substitute_player_id"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	plan, err := mgr.SetSubstitute(r.Context(), matches.SetSubstituteRequest{
		LineupPlanID:       id,
		SubstitutePlayerID: body.SubstitutePlayerID,
	})
	if err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, plan)
}

// clearLineupSubstitute handles DELETE /api/lineup-plans/{id}/substitute
// (Substitute Workflow Phase 1), reverting a substituted slot back to its
// original player. Subject to the same lock checks as setLineupSubstitute.
func clearLineupSubstitute(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	plan, err := mgr.ClearSubstitute(r.Context(), id)
	if err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, plan)
}

// mapLineupErr translates lineup domain errors to HTTP responses.
func mapLineupErr(w http.ResponseWriter, err error) {
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
