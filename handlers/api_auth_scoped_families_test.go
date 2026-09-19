package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/db"
	"league_app/models"
)

// This file covers PM's Phase 1 correction review requirement to prove,
// end-to-end over real HTTP with real sqlite-backed stores, that session
// authentication now works across every protected route family (not only
// league/season CRUD), and that league scope is enforced from each
// route's own resource -- never a client-supplied league_id -- for each
// of those families. api_auth_integration_test.go already covers
// login/session/CSRF mechanics and league/season CRUD scoping; this file
// extends the same fixture pattern to players, teams, matches, lineups,
// finances, schedule, and week close.

func seedTestSeason(t *testing.T, leagueID int64, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO seasons (league_id, name) VALUES (?, ?)`, leagueID, name)
	if err != nil {
		t.Fatalf("seed season %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedTestTeam(t *testing.T, leagueID int64, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?, ?)`, leagueID, name)
	if err != nil {
		t.Fatalf("seed team %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedTestPlayer(t *testing.T, teamID int64, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES (?, 'Player', ?)`, name, teamID)
	if err != nil {
		t.Fatalf("seed player %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedTestMatch(t *testing.T, seasonID, homeTeamID, awayTeamID int64) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?, ?, ?, 1)`,
		seasonID, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("seed match: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// scopedFixture is a two-league world (League A, League B), each with a
// season and two teams, League A additionally with a scheduled match --
// plus a system_admin and a league_admin scoped ONLY to League A.
type scopedFixture struct {
	leagueA, leagueB       int64
	seasonA, seasonB       int64
	teamA1, teamA2, teamB1 int64
	matchA                 int64
}

func newScopedFixture(t *testing.T) *scopedFixture {
	t.Helper()
	leagueA := seedTestLeague(t, "Families League A")
	leagueB := seedTestLeague(t, "Families League B")
	seasonA := seedTestSeason(t, leagueA, "Season A")
	seasonB := seedTestSeason(t, leagueB, "Season B")
	teamA1 := seedTestTeam(t, leagueA, "A Team 1")
	teamA2 := seedTestTeam(t, leagueA, "A Team 2")
	teamB1 := seedTestTeam(t, leagueB, "B Team 1")
	matchA := seedTestMatch(t, seasonA, teamA1, teamA2)

	seedPasswordUser(t, "families-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	seedPasswordUser(t, "families-adminA@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueA)

	return &scopedFixture{
		leagueA: leagueA, leagueB: leagueB,
		seasonA: seasonA, seasonB: seasonB,
		teamA1: teamA1, teamA2: teamA2, teamB1: teamB1,
		matchA: matchA,
	}
}

func TestAuthIntegration_ScopedFamilies(t *testing.T) {
	srv := testServerWithAuth(t)
	f := newScopedFixture(t)
	sysCookies, sysCSRF := loginCookies(t, srv, "families-sysadmin@example.com", "hunter22")
	aCookies, aCSRF := loginCookies(t, srv, "families-adminA@example.com", "hunter22")
	sysHeaders := map[string]string{"X-CSRF-Token": sysCSRF}
	aHeaders := map[string]string{"X-CSRF-Token": aCSRF}

	leagueAStr := strconv.FormatInt(f.leagueA, 10)
	leagueBStr := strconv.FormatInt(f.leagueB, 10)
	seasonAStr := strconv.FormatInt(f.seasonA, 10)
	seasonBStr := strconv.FormatInt(f.seasonB, 10)

	t.Run("players create", func(t *testing.T) {
		teamA1Str := strconv.FormatInt(f.teamA1, 10)
		teamB1Str := strconv.FormatInt(f.teamB1, 10)
		bodyA := `{"first_name":"Own","last_name":"League","team_id":` + teamA1Str + `,"league_id":` + leagueAStr + `}`
		bodyB := `{"first_name":"Other","last_name":"League","team_id":` + teamB1Str + `,"league_id":` + leagueBStr + `}`

		if code := postCode(t, srv, "/api/players", bodyA, aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A create player on own league A's team: want non-403, got %d", code)
		}
		if code := postCode(t, srv, "/api/players", bodyB, aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A create player on League B's team: want 403, got %d", code)
		}
		if code := postCode(t, srv, "/api/players", bodyB, sysCookies, sysHeaders); code == 403 {
			t.Errorf("system_admin create player on League B's team: want non-403, got %d", code)
		}

		// A league_admin claiming League A's team_id but League B's
		// league_id in the same body must be rejected as a mismatch (409),
		// not silently authorized against whichever value is convenient.
		mismatchBody := `{"first_name":"Mismatch","last_name":"Body","team_id":` + teamA1Str + `,"league_id":` + leagueBStr + `}`
		if code := postCode(t, srv, "/api/players", mismatchBody, aCookies, aHeaders); code != http.StatusConflict {
			t.Errorf("team_id/league_id mismatch: want 409, got %d", code)
		}

		// Unassigned-player creation (no team_id at all) is system_admin-only.
		unassignedBody := `{"first_name":"No","last_name":"Team"}`
		if code := postCode(t, srv, "/api/players", unassignedBody, aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A create an unassigned player: want 403 (system_admin-only), got %d", code)
		}
		if code := postCode(t, srv, "/api/players", unassignedBody, sysCookies, sysHeaders); code == 403 {
			t.Errorf("system_admin create an unassigned player: want non-403, got %d", code)
		}
	})

	t.Run("teams create", func(t *testing.T) {
		bodyA := `{"name":"New Team A","league_id":` + leagueAStr + `}`
		bodyB := `{"name":"New Team B","league_id":` + leagueBStr + `}`

		if code := postCode(t, srv, "/api/teams", bodyA, aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A create team in own league A: want non-403, got %d", code)
		}
		if code := postCode(t, srv, "/api/teams", bodyB, aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A create team in League B: want 403, got %d", code)
		}
	})

	t.Run("match assign", func(t *testing.T) {
		matchAStr := strconv.FormatInt(f.matchA, 10)
		body := `{"home_team_id":` + strconv.FormatInt(f.teamA1, 10) + `,"away_team_id":` + strconv.FormatInt(f.teamA2, 10) + `}`

		if code := patchCode(t, srv, "/api/matches/"+matchAStr+"/assign", body, aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A assign match in own league A: want non-403, got %d", code)
		}

		// A league_admin-B (scoped only to League B) must be denied on League A's match.
		seedPasswordUser(t, "families-adminB-assign@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueB)
		bCookies, bCSRF := loginCookies(t, srv, "families-adminB-assign@example.com", "hunter22")
		if code := patchCode(t, srv, "/api/matches/"+matchAStr+"/assign", body, bCookies, map[string]string{"X-CSRF-Token": bCSRF}); code != 403 {
			t.Errorf("league_admin-B assign match in League A: want 403, got %d", code)
		}

		// Even system_admin (authorized) must be rejected by the domain
		// service when a related-resource team_id belongs to another
		// league entirely -- this is a data-integrity invariant, not an
		// authorization decision.
		crossLeagueTeamBody := `{"home_team_id":` + strconv.FormatInt(f.teamB1, 10) + `,"away_team_id":` + strconv.FormatInt(f.teamA2, 10) + `}`
		if code := patchCode(t, srv, "/api/matches/"+matchAStr+"/assign", crossLeagueTeamBody, sysCookies, sysHeaders); code != http.StatusConflict {
			t.Errorf("assigning a League B team to a League A match: want 409, got %d", code)
		}
	})

	t.Run("lineup save", func(t *testing.T) {
		bodyA := `{"season_id":` + seasonAStr + `,"team_id":` + strconv.FormatInt(f.teamA1, 10) + `,"week_number":1,"player_ids":[]}`
		bodyB := `{"season_id":` + seasonBStr + `,"team_id":` + strconv.FormatInt(f.teamB1, 10) + `,"week_number":1,"player_ids":[]}`

		if code := postCode(t, srv, "/api/lineup-plans", bodyA, aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A save lineup in own league A: want non-403, got %d", code)
		}
		if code := postCode(t, srv, "/api/lineup-plans", bodyB, aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A save lineup in League B: want 403, got %d", code)
		}

		// Even system_admin must be rejected by the domain service when
		// the supplied team_id belongs to a different league than the
		// supplied season_id -- a data-integrity invariant, independent
		// of authorization.
		crossLeagueBody := `{"season_id":` + seasonAStr + `,"team_id":` + strconv.FormatInt(f.teamB1, 10) + `,"week_number":1,"player_ids":[]}`
		if code := postCode(t, srv, "/api/lineup-plans", crossLeagueBody, sysCookies, sysHeaders); code != http.StatusConflict {
			t.Errorf("saving a lineup for League A's season with a League B team: want 409, got %d", code)
		}
	})

	t.Run("finances read and write", func(t *testing.T) {
		if code := getCode(t, srv, "/api/seasons/"+seasonAStr+"/finances/dues", aCookies); code == 403 {
			t.Errorf("league_admin-A read dues in own league A: want non-403, got %d", code)
		}
		if code := getCode(t, srv, "/api/seasons/"+seasonBStr+"/finances/dues", aCookies); code != 403 {
			t.Errorf("league_admin-A read dues in League B: want 403, got %d", code)
		}

		payBody := `{"player_id":1,"amount":10,"paid_at":"2026-01-01"}`
		if code := postCode(t, srv, "/api/seasons/"+seasonBStr+"/finances/dues-payments", payBody, aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A record dues payment in League B: want 403, got %d", code)
		}
	})

	t.Run("schedule generate", func(t *testing.T) {
		bodyA := `{"season_id":` + seasonAStr + `,"start_date":"2026-01-05","schedule_type":"single_rr"}`
		bodyB := `{"season_id":` + seasonBStr + `,"start_date":"2026-01-05","schedule_type":"single_rr"}`

		if code := postCode(t, srv, "/api/matches/generate", bodyA, aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A generate schedule in own league A: want non-403, got %d", code)
		}
		if code := postCode(t, srv, "/api/matches/generate", bodyB, aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A generate schedule in League B: want 403, got %d", code)
		}
	})

	t.Run("week close", func(t *testing.T) {
		if code := postCode(t, srv, "/api/seasons/"+seasonAStr+"/weeks/1/close", "{}", aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A close week in own league A: want non-403, got %d", code)
		}
		if code := postCode(t, srv, "/api/seasons/"+seasonBStr+"/weeks/1/close", "{}", aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A close week in League B: want 403, got %d", code)
		}
	})

	t.Run("player update: same-league team change ok, cross-league move denied", func(t *testing.T) {
		p := seedTestPlayer(t, f.teamA1, "MoveMe")
		pIDStr := strconv.FormatInt(p, 10)

		sameLeagueBody := `{"first_name":"MoveMe","last_name":"Player","team_id":` + strconv.FormatInt(f.teamA2, 10) + `}`
		if code := statusCode(t, srv, "PUT", "/api/players/"+pIDStr, sameLeagueBody, aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A moving a player between two of their own League A teams: want non-403, got %d", code)
		}

		crossLeagueBody := `{"first_name":"MoveMe","last_name":"Player","team_id":` + strconv.FormatInt(f.teamB1, 10) + `}`
		if code := statusCode(t, srv, "PUT", "/api/players/"+pIDStr, crossLeagueBody, aCookies, aHeaders); code != http.StatusForbidden {
			t.Errorf("league_admin-A moving a player from League A into a League B team: want 403, got %d", code)
		}
		if code := statusCode(t, srv, "PUT", "/api/players/"+pIDStr, crossLeagueBody, sysCookies, sysHeaders); code == http.StatusForbidden {
			t.Errorf("system_admin moving a player cross-league: want non-403, got %d", code)
		}
	})

	// PM correction round 3: updatePlayer is a full-PUT handler that always
	// persists body.TeamID, so a request with team_id:null (or one that
	// simply omits team_id -- Go's JSON decode cannot tell the two apart)
	// unassigns the player. That must be system_admin-only, the same as
	// any other cross-league move, or a league_admin could authorize
	// against a player in their own league and then strip that player's
	// league ownership entirely.
	t.Run("player update: league_admin cannot unassign, system_admin can", func(t *testing.T) {
		p := seedTestPlayer(t, f.teamA1, "UnassignMe")
		pIDStr := strconv.FormatInt(p, 10)

		getPlayerTeamID := func() *int64 {
			resp := authDo(t, srv, "GET", "/api/players/"+pIDStr, "", aCookies, aHeaders)
			player := decodeJSON[models.Player](t, resp)
			return player.TeamID
		}

		explicitNullBody := `{"first_name":"UnassignMe","last_name":"Player","team_id":null}`
		if code := statusCode(t, srv, "PUT", "/api/players/"+pIDStr, explicitNullBody, aCookies, aHeaders); code != http.StatusForbidden {
			t.Errorf("league_admin-A setting team_id:null: want 403, got %d", code)
		}
		if got := getPlayerTeamID(); got == nil || *got != f.teamA1 {
			t.Errorf("player must remain assigned to team A1 after rejected null-team_id update, got %v", got)
		}

		omittedFieldBody := `{"first_name":"UnassignMe","last_name":"Player"}`
		if code := statusCode(t, srv, "PUT", "/api/players/"+pIDStr, omittedFieldBody, aCookies, aHeaders); code != http.StatusForbidden {
			t.Errorf("league_admin-A omitting team_id entirely: want 403, got %d", code)
		}
		if got := getPlayerTeamID(); got == nil || *got != f.teamA1 {
			t.Errorf("player must remain assigned to team A1 after rejected omitted-team_id update, got %v", got)
		}

		if code := statusCode(t, srv, "PUT", "/api/players/"+pIDStr, explicitNullBody, sysCookies, sysHeaders); code == http.StatusForbidden {
			t.Errorf("system_admin setting team_id:null: want non-403, got %d", code)
		}
		if got := getPlayerTeamID(); got != nil {
			t.Errorf("want player unassigned (team_id nil) after system_admin's null-team_id update, got %v", *got)
		}
	})

	t.Run("player merge: same-league ok, cross-league denied", func(t *testing.T) {
		sourceA := seedTestPlayer(t, f.teamA1, "MergeSourceA")
		targetA := seedTestPlayer(t, f.teamA2, "MergeTargetA")
		targetB := seedTestPlayer(t, f.teamB1, "MergeTargetB")

		sameLeagueBody := `{"target_id":` + strconv.FormatInt(targetA, 10) + `}`
		if code := postCode(t, srv, "/api/players/"+strconv.FormatInt(sourceA, 10)+"/merge", sameLeagueBody, aCookies, aHeaders); code == 403 {
			t.Errorf("league_admin-A merging two of their own League A players: want non-403, got %d", code)
		}

		source2 := seedTestPlayer(t, f.teamA1, "MergeSource2A")
		crossLeagueBody := `{"target_id":` + strconv.FormatInt(targetB, 10) + `}`
		if code := postCode(t, srv, "/api/players/"+strconv.FormatInt(source2, 10)+"/merge", crossLeagueBody, aCookies, aHeaders); code != http.StatusForbidden {
			t.Errorf("league_admin-A merging a League A player into a League B player: want 403, got %d", code)
		}
		if code := postCode(t, srv, "/api/players/"+strconv.FormatInt(source2, 10)+"/merge", crossLeagueBody, sysCookies, sysHeaders); code == http.StatusForbidden {
			t.Errorf("system_admin merging cross-league players: want non-403, got %d", code)
		}
	})

	t.Run("season activate and rules", func(t *testing.T) {
		if code := postCode(t, srv, "/api/seasons/"+seasonBStr+"/activate", "", aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A activate League B's season: want 403, got %d", code)
		}
		ruleBody := `{"rule_key":"handicap_multiplier","rule_label":"Multiplier","rule_value":"2.55"}`
		if code := postCode(t, srv, "/api/seasons/"+seasonBStr+"/rules", ruleBody, aCookies, aHeaders); code != 403 {
			t.Errorf("league_admin-A add a rule to League B's season: want 403, got %d", code)
		}
	})
}

// TestAuthIntegration_UnassignedPlayerRoutes proves PM's item 2 correction:
// an unassigned player (no team, no persisted league) can be managed only
// by system_admin -- a league_admin is denied even for routes with no
// league_id-bearing body at all (DELETE has no body; merge's body carries
// target_id, not league_id) -- and that a bodyless DELETE does not fail
// with an accidental 400 from JSON-decode EOF.
func TestAuthIntegration_UnassignedPlayerRoutes(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "Unassigned League A")
	teamA := seedTestTeam(t, leagueA, "Unassigned League A Team")
	seedPasswordUser(t, "unassigned-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	seedPasswordUser(t, "unassigned-adminA@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueA)
	sysCookies, sysCSRF := loginCookies(t, srv, "unassigned-sysadmin@example.com", "hunter22")
	aCookies, aCSRF := loginCookies(t, srv, "unassigned-adminA@example.com", "hunter22")
	sysHeaders := map[string]string{"X-CSRF-Token": sysCSRF}
	aHeaders := map[string]string{"X-CSRF-Token": aCSRF}

	seedUnassignedPlayer := func(name string) int64 {
		res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES (?, 'Unassigned')`, name)
		if err != nil {
			t.Fatalf("seed unassigned player: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	t.Run("league_admin denied delete of unassigned player", func(t *testing.T) {
		id := seedUnassignedPlayer("DeniedDelete")
		code := statusCode(t, srv, "DELETE", "/api/players/"+strconv.FormatInt(id, 10), "", aCookies, aHeaders)
		if code != http.StatusForbidden {
			t.Errorf("want 403, got %d", code)
		}
	})

	t.Run("system_admin deletes unassigned player, bodyless DELETE does not 400", func(t *testing.T) {
		id := seedUnassignedPlayer("SysAdminDelete")
		code := statusCode(t, srv, "DELETE", "/api/players/"+strconv.FormatInt(id, 10), "", sysCookies, sysHeaders)
		if code == http.StatusBadRequest {
			t.Errorf("want a real decision (200/403/404), not 400 from JSON-decode EOF on a bodyless DELETE, got %d", code)
		}
		if code != http.StatusOK {
			t.Errorf("want 200 for system_admin deleting an unassigned player, got %d", code)
		}
	})

	t.Run("league_admin denied merge touching an unassigned player", func(t *testing.T) {
		sourceID := seedUnassignedPlayer("MergeSourceUnassigned")
		targetID := seedUnassignedPlayer("MergeTargetUnassigned")
		body := `{"target_id":` + strconv.FormatInt(targetID, 10) + `}`
		code := postCode(t, srv, "/api/players/"+strconv.FormatInt(sourceID, 10)+"/merge", body, aCookies, aHeaders)
		if code != http.StatusForbidden {
			t.Errorf("want 403 for league_admin merging two unassigned players, got %d", code)
		}
	})

	t.Run("system_admin allowed merge touching an unassigned player", func(t *testing.T) {
		sourceID := seedUnassignedPlayer("MergeSourceSysAdmin")
		targetID := seedUnassignedPlayer("MergeTargetSysAdmin")
		body := `{"target_id":` + strconv.FormatInt(targetID, 10) + `}`
		code := postCode(t, srv, "/api/players/"+strconv.FormatInt(sourceID, 10)+"/merge", body, sysCookies, sysHeaders)
		if code == http.StatusForbidden {
			t.Errorf("want non-403 for system_admin merging two unassigned players, got %d", code)
		}
	})

	// An update STARTING from an already-unassigned player (assigning it
	// to a team for the first time) is system_admin-only, same as every
	// other unassigned-player route -- resolveExistingPlayerScope resolves
	// an empty Scope for the origin, which fails league_admin regardless
	// of what the destination team's league is.
	t.Run("league_admin denied update (assign) of an unassigned player", func(t *testing.T) {
		id := seedUnassignedPlayer("DeniedAssign")
		body := `{"first_name":"DeniedAssign","last_name":"Player","team_id":` + strconv.FormatInt(teamA, 10) + `}`
		code := statusCode(t, srv, "PUT", "/api/players/"+strconv.FormatInt(id, 10), body, aCookies, aHeaders)
		if code != http.StatusForbidden {
			t.Errorf("want 403 for league_admin assigning an unassigned player into their own league, got %d", code)
		}
	})

	t.Run("system_admin allowed update (assign) of an unassigned player", func(t *testing.T) {
		id := seedUnassignedPlayer("SysAdminAssign")
		body := `{"first_name":"SysAdminAssign","last_name":"Player","team_id":` + strconv.FormatInt(teamA, 10) + `}`
		code := statusCode(t, srv, "PUT", "/api/players/"+strconv.FormatInt(id, 10), body, sysCookies, sysHeaders)
		if code == http.StatusForbidden {
			t.Errorf("want non-403 for system_admin assigning an unassigned player, got %d", code)
		}
	})
}

// TestAuthIntegration_PlayerOverview_SessionAccess proves the PM-required
// fix: a player logged in with email/password (Player View, session
// cookie, no personal API key at all) can load their own Player Overview,
// cannot load another player's, and a scoped league_admin can load any
// player in their own league but not one in a different league.
func TestAuthIntegration_PlayerOverview_SessionAccess(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "Overview League A")
	leagueB := seedTestLeague(t, "Overview League B")
	teamA := seedTestTeam(t, leagueA, "Overview Team A")
	seasonA := seedTestSeason(t, leagueA, "Overview Season A")
	if _, err := db.DB.Exec(`UPDATE seasons SET active = 1 WHERE id = ?`, seasonA); err != nil {
		t.Fatalf("activate season A: %v", err)
	}

	resA, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('Own', 'Player', ?)`, teamA)
	if err != nil {
		t.Fatalf("seed player A: %v", err)
	}
	playerAID, _ := resA.LastInsertId()
	resOther, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('Other', 'Player', ?)`, teamA)
	if err != nil {
		t.Fatalf("seed other player: %v", err)
	}
	otherPlayerID, _ := resOther.LastInsertId()

	playerUserID := seedPasswordUser(t, "overview-player@example.com", "hunter22", "", nil)
	if _, err := db.DB.Exec(`UPDATE users SET player_id=? WHERE id=?`, playerAID, playerUserID); err != nil {
		t.Fatalf("link player: %v", err)
	}
	seedPasswordUser(t, "overview-adminA@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueA)
	seedPasswordUser(t, "overview-adminB@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueB)

	playerCookies, _ := loginCookies(t, srv, "overview-player@example.com", "hunter22")
	playerAIDStr := strconv.FormatInt(playerAID, 10)
	otherPlayerIDStr := strconv.FormatInt(otherPlayerID, 10)

	if code := getCode(t, srv, "/api/players/"+playerAIDStr+"/overview", playerCookies); code != http.StatusOK {
		t.Errorf("want 200 loading own overview via session, got %d", code)
	}
	if code := getCode(t, srv, "/api/players/"+otherPlayerIDStr+"/overview", playerCookies); code != http.StatusForbidden {
		t.Errorf("want 403 loading another player's overview, got %d", code)
	}

	adminACookies, _ := loginCookies(t, srv, "overview-adminA@example.com", "hunter22")
	if code := getCode(t, srv, "/api/players/"+playerAIDStr+"/overview", adminACookies); code != http.StatusOK {
		t.Errorf("want 200 for league_admin-A viewing a player in their own league, got %d", code)
	}

	adminBCookies, _ := loginCookies(t, srv, "overview-adminB@example.com", "hunter22")
	if code := getCode(t, srv, "/api/players/"+playerAIDStr+"/overview", adminBCookies); code != http.StatusForbidden {
		t.Errorf("want 403 for league_admin-B viewing a player in a different league, got %d", code)
	}
}

// TestAuthIntegration_PasswordSetup_FullFlow exercises the real
// system_admin-issues-token -> user-redeems-token -> user-logs-in-with-new-password
// path entirely through the HTTP API, matching what the login screen's
// setup mode calls.
func TestAuthIntegration_PasswordSetup_FullFlow(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "setup-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	sysCookies, sysCSRF := loginCookies(t, srv, "setup-sysadmin@example.com", "hunter22")

	newUserID := seedPasswordUser(t, "needs-setup@example.com", "placeholder-not-usable", "", nil)
	// Clear the placeholder hash to mirror a provisioned-but-not-yet-set-up
	// account (ProvisionUser normally leaves password_hash NULL; this test
	// seeds via seedPasswordUser for convenience, so reset it explicitly).
	if _, err := db.DB.Exec(`UPDATE users SET password_hash = NULL WHERE id = ?`, newUserID); err != nil {
		t.Fatalf("clear placeholder password: %v", err)
	}

	resp := authDo(t, srv, "POST", "/api/auth/admin/users/"+strconv.FormatInt(newUserID, 10)+"/setup-token", "", sysCookies, map[string]string{"X-CSRF-Token": sysCSRF})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("issue setup token: want 200, got %d", resp.StatusCode)
	}
	body := decodeJSON[map[string]string](t, resp)
	setupToken := body["setup_token"]
	if setupToken == "" {
		t.Fatal("want a non-empty setup_token in the response")
	}

	setupResp := authDo(t, srv, "POST", "/api/auth/password-setup",
		`{"setup_token":"`+setupToken+`","new_password":"brandNewPassword1"}`, nil, nil)
	setupResp.Body.Close()
	if setupResp.StatusCode != http.StatusOK {
		t.Fatalf("password-setup: want 200, got %d", setupResp.StatusCode)
	}

	loginResp := authDo(t, srv, "POST", "/api/auth/login",
		`{"email":"needs-setup@example.com","password":"brandNewPassword1"}`, nil, nil)
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login with newly-set password: want 200, got %d", loginResp.StatusCode)
	}

	// The same token must not be redeemable a second time.
	replayResp := authDo(t, srv, "POST", "/api/auth/password-setup",
		`{"setup_token":"`+setupToken+`","new_password":"anotherPassword2"}`, nil, nil)
	replayResp.Body.Close()
	if replayResp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 (setup token already used) on replay, got %d", replayResp.StatusCode)
	}
}

// --- small status-only HTTP helpers -----------------------------------

func statusCode(t *testing.T, srv *httptest.Server, method, path, body string, cookies []*http.Cookie, headers map[string]string) int {
	t.Helper()
	resp := authDo(t, srv, method, path, body, cookies, headers)
	resp.Body.Close()
	return resp.StatusCode
}

func postCode(t *testing.T, srv *httptest.Server, path, body string, cookies []*http.Cookie, headers map[string]string) int {
	return statusCode(t, srv, "POST", path, body, cookies, headers)
}

func patchCode(t *testing.T, srv *httptest.Server, path, body string, cookies []*http.Cookie, headers map[string]string) int {
	return statusCode(t, srv, "PATCH", path, body, cookies, headers)
}

func getCode(t *testing.T, srv *httptest.Server, path string, cookies []*http.Cookie) int {
	return statusCode(t, srv, "GET", path, "", cookies, nil)
}
