package handlers

import (
	"context"
	"net/http"

	"league_app/backend/domains/auth"
)

// This file collects every scopeFn used by guardedLeagueAdminAction /
// requireAction across the route families PM's Phase 1 correction review
// required to move onto scoped authorization. Each resolves the request's
// AUTHORITATIVE league server-side, from whatever identifier the route
// actually carries -- never trusting a client-supplied league_id once an
// existing resource can determine its own ownership (createSeasonScope in
// api_season_setup_scope.go remains the one deliberate exception, since a
// season being created does not exist yet to look up).

// seasonIDBodyScope is a scopeFn for routes whose body carries a
// pre-existing season_id (e.g. schedule generation, lineup save) --
// unlike createSeasonScope, this looks the season up rather than trusting
// the id's league directly, since the season already exists.
func seasonIDBodyScope(seasonMgr SeasonManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		var body struct {
			SeasonID int64 `json:"season_id"`
		}
		if err := peekJSONBody(r, &body); err != nil {
			return auth.Scope{}, false, err
		}
		if body.SeasonID == 0 {
			return auth.Scope{}, false, nil
		}
		season, err := seasonMgr.GetSeason(r.Context(), body.SeasonID)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		leagueID := season.LeagueID
		return auth.Scope{LeagueID: &leagueID}, true, nil
	}
}

// matchIDPathScope resolves the authoritative league for a route whose
// {id} path parameter is a match_id, via match -> season -> league_id.
func matchIDPathScope(matchMgr MatchManager, seasonMgr SeasonManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		id, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		detail, err := matchMgr.GetMatch(r.Context(), id)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		season, err := seasonMgr.GetSeason(r.Context(), detail.Match.SeasonID)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		leagueID := season.LeagueID
		return auth.Scope{LeagueID: &leagueID}, true, nil
	}
}

// lineupPlanIDPathScope resolves the authoritative league for a route
// whose {id} path parameter is a lineup_plans row, via plan -> season ->
// league_id.
func lineupPlanIDPathScope(lineupMgr LineupManager, seasonMgr SeasonManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		id, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		plan, err := lineupMgr.GetLineupPlan(r.Context(), id)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		season, err := seasonMgr.GetSeason(r.Context(), plan.SeasonID)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		leagueID := season.LeagueID
		return auth.Scope{LeagueID: &leagueID}, true, nil
	}
}

// createTeamScope resolves the authoritative league for a team-create
// request directly from its body's league_id -- the team does not exist
// yet, so there is no other identifier to resolve it from (mirrors
// createSeasonScope).
func createTeamScope(r *http.Request) (auth.Scope, bool, error) {
	var body struct {
		LeagueID int64 `json:"league_id"`
	}
	if err := peekJSONBody(r, &body); err != nil {
		return auth.Scope{}, false, err
	}
	if body.LeagueID == 0 {
		return auth.Scope{}, false, nil
	}
	return auth.Scope{LeagueID: &body.LeagueID}, true, nil
}

// teamIDPathScope resolves the authoritative league for a route whose
// {id} path parameter is a team_id, from the team's own league_id.
func teamIDPathScope(teamMgr TeamManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		id, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		team, err := teamMgr.GetTeam(r.Context(), id)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		leagueID := team.LeagueID
		return auth.Scope{LeagueID: &leagueID}, true, nil
	}
}

// createPlayerScope resolves the authoritative league for a player-create
// request. If body.team_id is supplied, the TEAM'S OWN league_id is
// authoritative -- a client-supplied body.league_id is never trusted for
// authorization once team_id identifies a real, persisted team (PM
// correction: "do not trust body league_id when team_id already
// identifies an authoritative league"). createPlayer itself separately
// rejects a body.league_id that disagrees with the team's real league
// (a 409, not an authorization decision). When no team_id is supplied at
// all -- a genuinely unassigned player, with no persisted ownership
// whatsoever -- creation is system_admin-only: an empty Scope (no
// LeagueID) fails ActionRosterMutate's league_admin branch while still
// succeeding for system_admin via Authorize's unconditional top-level
// bypass. A client-supplied league_id can never manufacture durable
// ownership for a resource that does not have any yet.
func createPlayerScope(teamMgr TeamManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		var body struct {
			TeamID *int64 `json:"team_id"`
		}
		if err := peekJSONBody(r, &body); err != nil {
			return auth.Scope{}, false, err
		}
		if body.TeamID == nil {
			return auth.Scope{}, true, nil
		}
		team, err := teamMgr.GetTeam(r.Context(), *body.TeamID)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		leagueID := team.LeagueID
		return auth.Scope{LeagueID: &leagueID}, true, nil
	}
}

// resolveExistingPlayerScope resolves the authoritative scope for an
// EXISTING player by id. A rostered player's league comes from its team
// (PlayerManager.GetPlayer already resolves this via a join). An
// unassigned player has no persisted ownership at all -- this
// deliberately never falls back to any request-body field to manufacture
// one (PM correction: DELETE has no body, and merge's body carries
// target_id, not league_id; reading league_id from either was wrong and
// caused an accidental 400 from JSON-decode EOF on a bodyless DELETE).
// An unassigned player therefore resolves to an empty Scope, authorizing
// system_admin only -- league_admin access requires real, persisted
// league ownership, full stop.
func resolveExistingPlayerScope(ctx context.Context, playerMgr PlayerManager, id int64) (auth.Scope, bool, error) {
	p, err := playerMgr.GetPlayer(ctx, id)
	if err != nil {
		return auth.Scope{}, false, nil
	}
	if p.LeagueID == 0 {
		return auth.Scope{}, true, nil
	}
	leagueID := p.LeagueID
	return auth.Scope{LeagueID: &leagueID}, true, nil
}

// playerIDPathScope resolves the authoritative league for a route whose
// {id} path parameter is a player_id and which needs no other
// information from the request (DELETE /api/players/{id}). Never reads
// the request body.
func playerIDPathScope(playerMgr PlayerManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		id, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		return resolveExistingPlayerScope(r.Context(), playerMgr, id)
	}
}

// updatePlayerScope resolves the authoritative scope for
// PUT /api/players/{id}. The player's own (origin) league is
// authoritative for editing them at all; when the body also supplies a
// non-null team_id, the DESTINATION team's real league is resolved and
// compared against the origin -- a same-league team change is authorized
// normally, but a cross-league move (or any move of a currently
// unassigned player) is system_admin-only (PM correction round 2: "a
// league_admin must not move a player into or out of a league they do not
// administer" -- resolved conservatively as system_admin-only for any
// actual league change, rather than trying to check both leagues against
// a possibly-dual-scoped league_admin).
//
// PM correction round 3: updatePlayer (api_players_handlers.go) is a
// full-PUT handler -- it always persists body.TeamID as given, so a
// request with team_id:null, OR a request that simply omits the team_id
// field, persists players.team_id = NULL exactly the same way (Go's JSON
// decode leaves a *int64 field nil in both cases; there is no way to tell
// "the caller means to unassign" from "the caller forgot the field").
// Treating a nil body.TeamID as "no team change, keep authorizing
// same-league" was therefore wrong: it let a league_admin authorize an
// update against the player's current (administered) league and then
// have the handler silently unassign the player, a state only
// system_admin can subsequently repair (resolveExistingPlayerScope's own
// empty-Scope-for-unassigned rule). A nil body.TeamID -- meaning THIS
// request will unassign the player -- is therefore system_admin-only, the
// same as any other cross-league move.
func updatePlayerScope(playerMgr PlayerManager, teamMgr TeamManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		id, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		scope, ok, err := resolveExistingPlayerScope(r.Context(), playerMgr, id)
		if err != nil || !ok || scope.LeagueID == nil {
			return scope, ok, err
		}
		originLeagueID := *scope.LeagueID

		var body struct {
			TeamID *int64 `json:"team_id"`
		}
		if err := peekJSONBody(r, &body); err != nil {
			return auth.Scope{}, false, err
		}
		if body.TeamID == nil {
			// This request persists team_id = NULL (unassignment):
			// system_admin only.
			return auth.Scope{}, true, nil
		}
		team, err := teamMgr.GetTeam(r.Context(), *body.TeamID)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		if team.LeagueID != originLeagueID {
			// Cross-league move: system_admin only.
			return auth.Scope{}, true, nil
		}
		return auth.Scope{LeagueID: &originLeagueID}, true, nil
	}
}

// mergePlayerScope resolves the authoritative scope for
// POST /api/players/{id}/merge. Both the source (path {id}) and the
// target (body.target_id) are resolved -- PM correction: "do not
// authorize only the source while accepting an unchecked target_id." A
// league_admin may merge only when both players resolve to the SAME
// league they administer; a cross-league merge, or a merge touching an
// unowned player on either side, is system_admin-only.
func mergePlayerScope(playerMgr PlayerManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		sourceID, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		var body struct {
			TargetID int64 `json:"target_id"`
		}
		if err := peekJSONBody(r, &body); err != nil {
			return auth.Scope{}, false, err
		}
		if body.TargetID == 0 {
			return auth.Scope{}, false, nil
		}
		source, err := playerMgr.GetPlayer(r.Context(), sourceID)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		target, err := playerMgr.GetPlayer(r.Context(), body.TargetID)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		if source.LeagueID == 0 || target.LeagueID == 0 || source.LeagueID != target.LeagueID {
			// Cross-league, or either side unowned: system_admin only.
			return auth.Scope{}, true, nil
		}
		leagueID := source.LeagueID
		return auth.Scope{LeagueID: &leagueID}, true, nil
	}
}
