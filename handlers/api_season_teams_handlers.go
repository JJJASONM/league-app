package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// Season Teams -----------------------------------------------------------------

func listSeasonTeams(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	teams, err := mgr.ListSeasonTeams(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, teams)
}

func addSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req seasons.AddTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	st, err := mgr.AddTeam(r.Context(), sid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, st)
}

func updateSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	var req seasons.UpdateTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	st, err := mgr.UpdateTeam(r.Context(), sid, tid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, st)
}

func removeSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	if err := mgr.RemoveTeam(r.Context(), sid, tid); err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

// mapSeasonErr translates seasons domain errors to HTTP responses.
func mapSeasonErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	switch {
	case errors.Is(err, seasons.ErrNotFound):
		jsonError(w, "season not found", http.StatusNotFound)
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
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
}

// Season Rosters -----------------------------------------------------------------

func listSeasonRoster(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	entries, err := mgr.ListRoster(r.Context(), sid, tid)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, entries)
}

type addRosterPlayerRequest struct {
	PlayerID int64 `json:"player_id"`
}

func addRosterPlayer(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	var req addRosterPlayerRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.PlayerID == 0 {
		jsonError(w, "player_id is required", 400)
		return
	}
	entry, err := mgr.AddRosterPlayer(r.Context(), sid, tid, req.PlayerID)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, entry)
}

func removeRosterPlayer(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	pid, err := pathID(r, "pid")
	if err != nil {
		jsonError(w, "invalid player id", 400)
		return
	}
	if err := mgr.RemoveRosterPlayer(r.Context(), sid, tid, pid); err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

// Available Players --------------------------------------------------------------

func listAvailablePlayers(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	players, err := mgr.ListAvailablePlayers(r.Context(), sid)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, players)
}

// Previous Season ------------------------------------------------------------------

func getPreviousSeasonTeams(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	result, err := mgr.PreviousSeason(r.Context(), sid)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			jsonError(w, "season not found", 404)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, result)
}

// Setup Checklist --------------------------------------------------------------------

// checklistCodeTeamNoDefaultLineup is the warning code for a season team
// with fewer than 3 default-lineup (week_number=0) rows. Defined at the
// handler layer, not in backend/domains/seasons/codes.go, because the
// check itself is composed here (see appendDefaultLineupWarnings) rather
// than inside SeasonService.computeChecklist -- the seasons domain has no
// dependency on lineups, matching how Financial/Player Overview/the League
// Admin hub already compose cross-domain reads at the handler layer
// instead of reaching into another domain's service.
const checklistCodeTeamNoDefaultLineup = "TEAM_NO_DEFAULT_LINEUP"

func getSeasonChecklist(w http.ResponseWriter, r *http.Request, mgr SeasonManager, lineupMgr LineupManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	c, err := mgr.Checklist(r.Context(), sid)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			jsonError(w, "season not found", 404)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	appendDefaultLineupWarnings(r.Context(), &c, sid, mgr, lineupMgr)
	jsonOK(w, c)
}

// appendDefaultLineupWarnings adds a non-blocking Warning to c for every
// season team with fewer than 3 default-lineup (week_number=0) rows. This
// is a warning only -- it never touches c.Blockers or c.CanActivate, so it
// cannot block season activation, matching the PM constraint that Close
// Week, activation, and schedule generation must not depend on future
// lineups. lineupMgr is optional (nil when unwired, same convention as
// FinanceMgr elsewhere); failures anywhere in this composition degrade to
// "no warning added" rather than failing the whole checklist request,
// since this is an advisory nudge, not required data.
func appendDefaultLineupWarnings(ctx context.Context, c *models.SetupChecklist, seasonID int64, seasonMgr SeasonManager, lineupMgr LineupManager) {
	if lineupMgr == nil {
		return
	}
	teams, err := seasonMgr.ListSeasonTeams(ctx, seasonID)
	if err != nil || len(teams) == 0 {
		return
	}
	defaultPlans, err := lineupMgr.ListLineupPlans(ctx, matches.ListLineupPlansRequest{
		SeasonID:   seasonID,
		WeekNumber: 0,
	})
	if err != nil {
		return
	}
	countByTeam := map[int64]int{}
	for _, p := range defaultPlans {
		countByTeam[p.TeamID]++
	}
	for _, t := range teams {
		n := countByTeam[t.TeamID]
		if n >= 3 {
			continue
		}
		var msg string
		if n == 0 {
			msg = fmt.Sprintf("team %q has no default lineup set", t.SeasonName)
		} else {
			msg = fmt.Sprintf("team %q has an incomplete default lineup (%d/3 players set)", t.SeasonName, n)
		}
		c.Warnings = append(c.Warnings, models.ChecklistItem{
			Code:    checklistCodeTeamNoDefaultLineup,
			Message: msg,
			TeamID:  t.TeamID,
		})
	}
}
