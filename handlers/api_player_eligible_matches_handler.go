package handlers

import (
	"net/http"

	"league_app/backend/domains/matches"
	"league_app/models"
)

// getPlayerEligibleMatches handles GET /api/players/{id}/eligible-matches.
// Handler-level composition, matching getPlayerOverview's precedent
// (Player Score Approval Phase 1C does not introduce a new cross-domain
// service layer for a single read). season_id is optional: when omitted,
// the player's own league's active season is used, exactly like Player
// Overview -- the shell's currently selected admin league/season context
// must never be assumed to be this player's own. Eligibility itself stays
// entirely authoritative in RoundManager.ListEligibleMatchesForPlayer
// (backend/domains/matches.RoundStore.ResolveApprovalEligibility's exact
// rule, applied per match): this handler only composes the response, it
// never approximates or re-derives eligibility from player.TeamID.
//
// PM correction (2026-09-30): an explicit season_id was accepted
// unconditionally, with no check that it actually belongs to this
// player's own league -- a caller could request a different league's
// season and (depending on what ListEligibleMatchesForPlayer found for
// that season_id) potentially see that other league's protected match/
// approval data. Fixed by validating the explicit season exactly like
// Player Overview does: resolve it via seasonMgr.GetSeason (mapSeasonErr
// on failure, the same status/error shape Player Overview uses), then
// reject with 404 "player is not in this season's league" whenever the
// player resolves to a real league (LeagueID != 0) that disagrees with
// the season's -- the same unconditional check, applied regardless of
// caller identity, so system_admin and league_admin callers get the
// same contract Player Overview already gives them for a mismatched
// season rather than a second, bespoke policy.
//
// Access control is enforced entirely by playerOverviewAuth before this
// handler ever runs (see api_player_eligible_matches_routes.go) -- the
// same ownership model as Player Overview.
func getPlayerEligibleMatches(
	w http.ResponseWriter, r *http.Request,
	playerMgr PlayerManager, seasonMgr SeasonManager, roundMgr RoundManager,
) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	player, err := playerMgr.GetPlayer(r.Context(), id)
	if err != nil {
		mapPlayerErr(w, err)
		return
	}

	var seasonID int64
	if sid, ok := qparamInt(r, "season_id"); ok {
		season, err := seasonMgr.GetSeason(r.Context(), sid)
		if err != nil {
			mapSeasonErr(w, err)
			return
		}
		// player.LeagueID is derived by joining through players.team_id --
		// a teamless player has no derivable league at all (LeagueID==0),
		// which is not a conflict with any season, just an unknown one.
		// Only reject when the player DOES resolve to a real league and it
		// disagrees with the season's (mirrors getPlayerOverview exactly).
		if player.LeagueID != 0 && season.LeagueID != player.LeagueID {
			jsonError(w, "player is not in this season's league", http.StatusNotFound)
			return
		}
		seasonID = sid
	} else {
		activeID, found, err := seasonMgr.FindActiveSeasonByLeague(r.Context(), player.LeagueID)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !found {
			// No active season for this player's league (or a teamless
			// player with no derivable league at all): an empty "My
			// Matches" list is the correct response, not an error --
			// there is nothing wrong with the request.
			jsonOK(w, []models.PlayerEligibleMatch{})
			return
		}
		seasonID = activeID
	}

	rows, err := roundMgr.ListEligibleMatchesForPlayer(r.Context(), seasonID, id)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]models.PlayerEligibleMatch, len(rows))
	for i, row := range rows {
		out[i] = toPlayerEligibleMatchModel(row)
	}
	jsonOK(w, out)
}

// toPlayerEligibleMatchModel converts one domain-layer
// matches.PlayerEligibleMatch row into its wire shape, reusing
// toSideApprovalSnapshotModel so this list's per-side approval fields
// render identically to the dedicated GET .../approval read.
func toPlayerEligibleMatchModel(row matches.PlayerEligibleMatch) models.PlayerEligibleMatch {
	return models.PlayerEligibleMatch{
		MatchID:       row.MatchID,
		SeasonID:      row.SeasonID,
		WeekNumber:    row.WeekNumber,
		MatchDate:     row.MatchDate,
		Completed:     row.Completed,
		WeekClosed:    row.WeekClosed,
		HomeTeamID:    row.HomeTeamID,
		HomeTeamName:  row.HomeTeamName,
		AwayTeamID:    row.AwayTeamID,
		AwayTeamName:  row.AwayTeamName,
		EligibleHome:  row.EligibleHome,
		EligibleAway:  row.EligibleAway,
		ScoreRevision: row.Approval.ScoreRevision,
		Home:          toSideApprovalSnapshotModel(row.Approval.HomeSide),
		Away:          toSideApprovalSnapshotModel(row.Approval.AwaySide),
	}
}
