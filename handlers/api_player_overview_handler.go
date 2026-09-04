package handlers

import (
	"context"
	"net/http"

	"league_app/backend/domains/matches"
	"league_app/models"
)

// getPlayerOverview handles GET /api/players/{id}/overview. Composes a
// read-only Player Overview from existing managers: player identity,
// season/team context, that team's matches for the season, the player's
// season stats, current handicap, and money status (Player Overview
// Phase 2 -- backed by the finances domain from Financial Phase 1).
// Handler-level composition per PM decision -- Player Overview does not
// introduce a new cross-domain service layer; if this grows, promoting
// it into a dedicated service is a later decision.
//
// season_id is optional: when omitted, the player's league's active
// season is used. An explicit season_id belonging to a different league
// than the player returns 404 -- composing a player from one league with
// a season from another would otherwise silently fall back to the
// player's direct team_id and produce a nonsensical cross-league
// overview instead of rejecting the invalid combination. Team is
// resolved by preferring the season roster
// (season_rosters, the target per-season team model) and falling back to
// the player's direct players.team_id when they have no roster entry for
// the season -- this covers both non-roster-managed seasons and a player
// who simply isn't on that season's roster. When no team resolves at all,
// team is null and schedule/stats are returned empty rather than erroring.
//
// financeMgr is nil in test-only setups that don't wire one (e.g. the
// shared testServer() helper); when nil, money falls back to the old
// Phase 1 "not tracked" placeholder instead of erroring.
//
// Access control (Player Account Access Phase 1): system_admin/admin/
// league_admin may view any player's overview, unchanged from Phase 2's
// money-integration correction. A resolved role="player" user may view
// only the one player their account is linked to (models.User.PlayerID);
// requesting any other player's overview is forbidden. Any other role is
// forbidden. When no user is in the request context (ApplyAuth not wired,
// e.g. the shared testServer() helper), access is left open -- see
// checkPlayerOverviewAccess.
func getPlayerOverview(
	w http.ResponseWriter, r *http.Request,
	playerMgr PlayerManager, seasonMgr SeasonManager, teamMgr TeamManager,
	matchMgr MatchManager, roundMgr RoundManager, financeMgr FinanceManager, ruleMgr RuleManager,
) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !checkPlayerOverviewAccess(r, id) {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}

	player, err := playerMgr.GetPlayer(r.Context(), id)
	if err != nil {
		mapPlayerErr(w, err)
		return
	}

	var seasonID int64
	if sid, ok := qparamInt(r, "season_id"); ok {
		seasonID = sid
	} else {
		activeID, found, err := seasonMgr.FindActiveSeasonByLeague(r.Context(), player.LeagueID)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !found {
			jsonError(w, "no active season for this player's league", http.StatusNotFound)
			return
		}
		seasonID = activeID
	}

	season, err := seasonMgr.GetSeason(r.Context(), seasonID)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	// player.LeagueID is derived by joining through players.team_id
	// (COALESCE(t.league_id,0) in PlayerStore) -- a teamless player has no
	// derivable league at all (LeagueID==0), which is not a conflict with
	// any season, just an unknown one. Only reject when the player DOES
	// resolve to a real league and it disagrees with the season's.
	if player.LeagueID != 0 && season.LeagueID != player.LeagueID {
		jsonError(w, "player is not in this season's league", http.StatusNotFound)
		return
	}

	overview := models.PlayerOverview{
		Player:   player,
		Season:   models.PlayerOverviewSeason{ID: season.ID, Name: season.Name},
		Handicap: models.PlayerOverviewHandicap{Current: player.Handicap},
		Schedule: []models.PlayerOverviewMatch{},
		Stats:    models.PlayerOverviewStats{},
		Money: models.PlayerOverviewMoney{
			Tracked: false,
			Message: "Dues and payouts are not tracked yet.",
		},
	}

	if financeMgr != nil {
		overview.Money = playerOverviewMoney(r.Context(), financeMgr, ruleMgr, seasonID, id)
	}

	teamID, found, err := seasonMgr.GetPlayerRosterTeam(r.Context(), seasonID, id)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !found && player.TeamID != nil {
		teamID = *player.TeamID
		found = true
	}

	if found {
		if team, err := teamMgr.GetTeam(r.Context(), teamID); err == nil {
			overview.Team = &models.PlayerOverviewTeam{ID: team.ID, Name: team.Name}
		}

		if allMatches, err := matchMgr.ListMatches(r.Context(), matches.ListMatchesRequest{SeasonID: seasonID}); err == nil {
			for _, m := range allMatches {
				if m.HomeTeamID != teamID && m.AwayTeamID != teamID {
					continue
				}
				homeOrAway, opponent := "away", m.HomeTeamName
				if m.HomeTeamID == teamID {
					homeOrAway, opponent = "home", m.AwayTeamName
				}
				overview.Schedule = append(overview.Schedule, models.PlayerOverviewMatch{
					MatchID:          m.ID,
					WeekNumber:       m.WeekNumber,
					MatchDate:        m.MatchDate,
					OpponentTeamName: opponent,
					HomeOrAway:       homeOrAway,
					Completed:        m.Completed,
				})
			}
		}
	}

	if stats, err := roundMgr.GetPlayerStats(r.Context(), matches.PlayerStatsRequest{SeasonID: seasonID}); err == nil {
		for _, s := range stats {
			if s.PlayerID != id {
				continue
			}
			overview.Stats = models.PlayerOverviewStats{
				SetsWon:   s.SetsWon,
				SetsLost:  s.SetsLost,
				GamesWon:  s.GamesWon,
				GamesLost: s.GamesLost,
				WinPct:    s.WinPct,
			}
			break
		}
	}

	jsonOK(w, overview)
}

// checkPlayerOverviewAccess enforces per-viewer access to Player Overview
// (Player Account Access Phase 1): system_admin/admin/league_admin may view
// any player's overview; a resolved role="player" user may view only their
// own linked player_id; any other role is forbidden. When no user is in
// the request context -- ApplyAuth is not wired, e.g. the shared
// testServer() test helper, so requirePersonalKeyOnly was a passthrough --
// access is left open, matching every other personal-key-protected route's
// behavior under that same test setup.
func checkPlayerOverviewAccess(r *http.Request, requestedPlayerID int64) bool {
	user := clearanceUserFromContext(r.Context())
	if user == nil {
		return true
	}
	switch user.Role {
	case "system_admin", "admin", "league_admin":
		return true
	case "player":
		return user.PlayerID != nil && *user.PlayerID == requestedPlayerID
	default:
		return false
	}
}

// playerOverviewMoney composes the player's dues status for the season.
// Only called when financeMgr is non-nil. Paid is true whenever the
// player has at least one dues_payments row -- there is no partial-
// payment/balance math. dues_amount is read from the season_rules
// freeform key of the same name for display only; ruleMgr is always
// non-nil in production (RuleManager is required elsewhere in Register),
// but is still nil-checked here since it is only reachable through this
// function's caller, not guaranteed by this function's own signature.
// Errors from either manager are treated the same way the rest of this
// handler treats optional composition failures: fail soft into an empty
// result rather than failing the whole overview request.
func playerOverviewMoney(ctx context.Context, financeMgr FinanceManager, ruleMgr RuleManager, seasonID, playerID int64) models.PlayerOverviewMoney {
	money := models.PlayerOverviewMoney{Tracked: true, Payments: []models.DuesPayment{}}

	if payments, err := financeMgr.ListDuesPaymentsByPlayer(ctx, seasonID, playerID); err == nil {
		money.Payments = payments
	}
	for _, p := range money.Payments {
		money.TotalPaid += p.Amount
	}
	money.Paid = len(money.Payments) > 0
	if !money.Paid {
		money.Message = "No dues payment recorded yet."
	}

	if ruleMgr != nil {
		if rules, err := ruleMgr.List(ctx, seasonID); err == nil {
			for _, rule := range rules {
				if rule.RuleKey == "dues_amount" {
					money.DuesAmount = rule.RuleValue
					break
				}
			}
		}
	}

	return money
}
