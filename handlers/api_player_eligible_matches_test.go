// Player Score Approval Phase 1C integration tests for
// GET /api/players/{id}/eligible-matches (the "My Matches" list). Uses
// seedApprovalFixture/linkUserToPlayer/seedUnrelatedPlayer from
// api_match_approval_phase1b_test.go -- the same real sqlite-backed auth
// stack as every other Phase 1B/1C approval test.
package handlers_test

import (
	"net/http"
	"strconv"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/db"
	"league_app/models"
)

func TestEligibleMatches_LinkedPlayer_SeesOwnEligibleMatch(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "em-home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, _ := loginCookies(t, f.srv, "em-home@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.homePlayerID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	rows := decodeJSON[[]models.PlayerEligibleMatch](t, resp)
	if len(rows) != 1 || rows[0].MatchID != f.matchID {
		t.Fatalf("want exactly the fixture match returned, got %+v", rows)
	}
	if !rows[0].EligibleHome || rows[0].EligibleAway {
		t.Errorf("want EligibleHome=true, EligibleAway=false for the home-rostered player, got %+v", rows[0])
	}
	if rows[0].Home.State != "pending" {
		t.Errorf("want home approval state 'pending' by default, got %q", rows[0].Home.State)
	}
}

func TestEligibleMatches_UnrelatedPlayer_SeesEmptyList(t *testing.T) {
	f := seedApprovalFixture(t)
	cookies, _ := seedUnrelatedPlayer(t, f, "em-unrelated@example.com")

	var unrelatedID int64
	if err := db.DB.QueryRow(`SELECT id FROM players WHERE first_name='Unrelated'`).Scan(&unrelatedID); err != nil {
		t.Fatalf("resolve unrelated player id: %v", err)
	}

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(unrelatedID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	rows := decodeJSON[[]models.PlayerEligibleMatch](t, resp)
	if len(rows) != 0 {
		t.Errorf("want an empty list for a player with no roster/participation link, got %+v", rows)
	}
}

func TestEligibleMatches_LinkedPlayer_CannotViewAnotherPlayersList(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "em-home2@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, _ := loginCookies(t, f.srv, "em-home2@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.awayPlayerID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 viewing another player's eligible-matches list, got %d", resp.StatusCode)
	}
}

func TestEligibleMatches_LeagueAdmin_SameLeague_Allowed(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "em-adminA@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	cookies, _ := loginCookies(t, f.srv, "em-adminA@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.homePlayerID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for a league_admin of the player's own league, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	rows := decodeJSON[[]models.PlayerEligibleMatch](t, resp)
	if len(rows) != 1 {
		t.Errorf("want the league_admin to see the player's one eligible match, got %+v", rows)
	}
}

func TestEligibleMatches_LeagueAdmin_DifferentLeague_Forbidden(t *testing.T) {
	f := seedApprovalFixture(t)
	otherLeagueID := seedTestLeague(t, "Eligible Matches Other League")
	seedPasswordUser(t, "em-adminB@example.com", "hunter22", auth.RoleLeagueAdmin, &otherLeagueID)
	cookies, _ := loginCookies(t, f.srv, "em-adminB@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.homePlayerID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 for a league_admin of a DIFFERENT league, got %d", resp.StatusCode)
	}
}

func TestEligibleMatches_SystemAdmin_CanViewAnyPlayer(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "em-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, f.srv, "em-sysadmin@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.awayPlayerID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for system_admin viewing any player's list, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	rows := decodeJSON[[]models.PlayerEligibleMatch](t, resp)
	if len(rows) != 1 || !rows[0].EligibleAway {
		t.Errorf("want the away player's one eligible match with EligibleAway=true, got %+v", rows)
	}
}

func TestEligibleMatches_ActualParticipantSubstitute_NoRosterRow_StillListed(t *testing.T) {
	f := seedApprovalFixture(t)
	// A genuine last-minute substitute: normally rostered to a THIRD team in
	// the same league/season (so their own league/active-season resolution
	// still works), but actually played home for f.matchID -- a match
	// belonging to a different team's roster -- with no season_rosters row
	// linking them to f.homeTeamID at all.
	rt, err := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?, 'Sub Bench Team')`, f.leagueID)
	if err != nil {
		t.Fatalf("seed sub's own team: %v", err)
	}
	subTeamID, _ := rt.LastInsertId()
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('Sub', 'Player', ?)`, subTeamID)
	if err != nil {
		t.Fatalf("seed sub player: %v", err)
	}
	subPlayerID, _ := res.LastInsertId()
	if _, err := db.DB.Exec(`UPDATE round_results SET home_player_id=? WHERE match_id=?`, subPlayerID, f.matchID); err != nil {
		t.Fatalf("rewrite round_results participant: %v", err)
	}
	userID := seedPasswordUser(t, "em-sub@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, subPlayerID)
	cookies, _ := loginCookies(t, f.srv, "em-sub@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(subPlayerID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	rows := decodeJSON[[]models.PlayerEligibleMatch](t, resp)
	if len(rows) != 1 || rows[0].MatchID != f.matchID {
		t.Fatalf("want the substitute's actual match returned despite no season_rosters row, got %+v", rows)
	}
	if !rows[0].EligibleHome {
		t.Error("want EligibleHome=true for an actual round_results participant with no roster membership")
	}
}

func TestEligibleMatches_NoLeagueNoTeam_ReturnsEmptyListNotError(t *testing.T) {
	f := seedApprovalFixture(t)
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Teamless', 'Player')`)
	if err != nil {
		t.Fatalf("seed teamless player: %v", err)
	}
	teamlessID, _ := res.LastInsertId()
	userID := seedPasswordUser(t, "em-teamless@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, teamlessID)
	cookies, _ := loginCookies(t, f.srv, "em-teamless@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(teamlessID, 10)+"/eligible-matches", "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (empty list, not an error) for a teamless player with no derivable league, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	rows := decodeJSON[[]models.PlayerEligibleMatch](t, resp)
	if len(rows) != 0 {
		t.Errorf("want an empty list, got %+v", rows)
	}
}

func TestEligibleMatches_ExplicitSeasonID_Honored(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "em-explicit@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, _ := loginCookies(t, f.srv, "em-explicit@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.homePlayerID, 10)+"/eligible-matches?season_id="+strconv.FormatInt(f.seasonID, 10), "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	rows := decodeJSON[[]models.PlayerEligibleMatch](t, resp)
	if len(rows) != 1 || rows[0].SeasonID != f.seasonID {
		t.Errorf("want the explicit season_id honored, got %+v", rows)
	}
}

// seedOtherLeagueSeason creates an independent league and one season in it,
// unrelated to f's own league/season -- used to prove an explicit
// cross-league season_id is rejected regardless of caller identity.
func seedOtherLeagueSeason(t *testing.T, name string) (leagueID, seasonID int64) {
	t.Helper()
	leagueID = seedTestLeague(t, name)
	res, err := db.DB.Exec(`INSERT INTO seasons (league_id, name) VALUES (?, ?)`, leagueID, name+" Season")
	if err != nil {
		t.Fatalf("seed other-league season: %v", err)
	}
	seasonID, _ = res.LastInsertId()
	return leagueID, seasonID
}

// PM correction (2026-09-30): an explicit season_id from a DIFFERENT
// league than the requested player must be rejected -- it must not leak
// that other league's protected match/approval information, and the
// rejection must match Player Overview's own established contract (404
// "player is not in this season's league") rather than a new policy.

func TestEligibleMatches_LinkedPlayer_DifferentLeagueExplicitSeason_Rejected(t *testing.T) {
	f := seedApprovalFixture(t)
	_, otherSeasonID := seedOtherLeagueSeason(t, "Eligible Matches Cross-League A")
	userID := seedPasswordUser(t, "em-crossleague-player@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, _ := loginCookies(t, f.srv, "em-crossleague-player@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.homePlayerID, 10)+"/eligible-matches?season_id="+strconv.FormatInt(otherSeasonID, 10), "", cookies, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 (season belongs to a different league than the player), got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestEligibleMatches_SameLeagueAdmin_DifferentLeagueExplicitSeasonForPlayer_Rejected(t *testing.T) {
	f := seedApprovalFixture(t)
	_, otherSeasonID := seedOtherLeagueSeason(t, "Eligible Matches Cross-League B")
	seedPasswordUser(t, "em-crossleague-adminA@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	cookies, _ := loginCookies(t, f.srv, "em-crossleague-adminA@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.homePlayerID, 10)+"/eligible-matches?season_id="+strconv.FormatInt(otherSeasonID, 10), "", cookies, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 even for the player's own league's admin, when the explicit season belongs to a different league, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestEligibleMatches_SystemAdmin_DifferentLeagueExplicitSeasonForPlayer_RejectedLikePlayerOverview(t *testing.T) {
	f := seedApprovalFixture(t)
	_, otherSeasonID := seedOtherLeagueSeason(t, "Eligible Matches Cross-League C")
	seedPasswordUser(t, "em-crossleague-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, f.srv, "em-crossleague-sysadmin@example.com", "hunter22")

	resp := authDo(t, f.srv, http.MethodGet,
		"/api/players/"+strconv.FormatInt(f.homePlayerID, 10)+"/eligible-matches?season_id="+strconv.FormatInt(otherSeasonID, 10), "", cookies, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 for system_admin too (same unconditional player<->season league check Player Overview uses, not a second policy), got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}
