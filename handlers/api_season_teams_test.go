package handlers_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"league_app/db"
)

// --- Season Teams & Rosters (Phase One) ---

// postSeasonTeamFromID adds an existing team to a draft season via from_team_id.
func postSeasonTeamFromID(t *testing.T, srv *httptest.Server, seasonID, teamID int64) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"from_team_id":%d}`, teamID)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST season team: %v", err)
	}
	return resp
}

// postNewSeasonTeam creates a brand-new team inside a draft season via name.
func postNewSeasonTeam(t *testing.T, srv *httptest.Server, seasonID int64, name string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q}`, name)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST new season team: %v", err)
	}
	return resp
}

// postRosterPlayer adds a player to a team's season roster.
func postRosterPlayer(t *testing.T, srv *httptest.Server, seasonID, teamID, playerID int64) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"player_id":%d}`, playerID)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST roster player: %v", err)
	}
	return resp
}

// httpDo sends an arbitrary request; body may be empty.
func httpDo(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var req *http.Request
	if body != "" {
		req, _ = http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, srv.URL+path, nil)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// postActivateSeason POSTs /api/seasons/{id}/activate.
func postActivateSeason(t *testing.T, srv *httptest.Server, seasonID int64) *http.Response {
	t.Helper()
	return httpDo(t, srv, http.MethodPost, fmt.Sprintf("/api/seasons/%d/activate", seasonID), "")
}

// getChecklist GETs /api/seasons/{id}/checklist and returns the decoded body map.
func getChecklist(t *testing.T, srv *httptest.Server, seasonID int64) map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/checklist", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET checklist: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("checklist: want 200, got %d", resp.StatusCode)
	}
	var c map[string]any
	json.NewDecoder(resp.Body).Decode(&c)
	return c
}

// createTestPlayer POSTs a player and returns their ID.
func createTestPlayer(t *testing.T, srv *httptest.Server, first, last string) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"first_name":%q,"last_name":%q,"handicap":0}`, first, last)
	resp, err := http.Post(srv.URL+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST player: %v", err)
	}
	defer resp.Body.Close()
	var p map[string]any
	json.NewDecoder(resp.Body).Decode(&p)
	return int64(p["id"].(float64))
}

// setPlayerTeam assigns a player to a team directly in the DB (avoids full-PUT field clobber).
func setPlayerTeam(t *testing.T, playerID, teamID int64) {
	t.Helper()
	if _, err := db.DB.Exec(`UPDATE players SET team_id=? WHERE id=?`, teamID, playerID); err != nil {
		t.Fatalf("setPlayerTeam: %v", err)
	}
}

// --- Tests ---

func TestSeasonTeams_AddAndList(t *testing.T) {
	srv := testServer(t)
	_, seasonID, _ := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Managed first season: only the name path is allowed.
	r := postNewSeasonTeam(t, srv, seasonID, "Gamma")
	if r.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		t.Fatalf("add team: want 201, got %d -- %s", r.StatusCode, body)
	}
	var added map[string]any
	json.NewDecoder(r.Body).Decode(&added)
	r.Body.Close()
	addedTeamID := int64(added["team_id"].(float64))

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var teams []map[string]any
	json.NewDecoder(resp.Body).Decode(&teams)
	if len(teams) != 1 {
		t.Fatalf("want 1 season team, got %d", len(teams))
	}
	if int64(teams[0]["team_id"].(float64)) != addedTeamID {
		t.Errorf("team_id: want %d, got %v", addedTeamID, teams[0]["team_id"])
	}
}

func TestSeasonTeams_DuplicateRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	// Downgrade to legacy mode so from_team_id works without from_season_id.
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, seasonID)

	r1 := postSeasonTeamFromID(t, srv, seasonID, teamIDs[0])
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("first add: want 201, got %d", r1.StatusCode)
	}

	r2 := postSeasonTeamFromID(t, srv, seasonID, teamIDs[0])
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate add: want 400, got %d", r2.StatusCode)
	}
}

func TestSeasonTeams_ActiveSeasonBlocked(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Lock the season by setting activated_at directly in DB (bypasses checklist).
	// isDraftSeason checks activated_at IS NULL, so this simulates first activation.
	if _, err := db.DB.Exec(
		`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, seasonID); err != nil {
		t.Fatalf("set activated_at: %v", err)
	}

	r := postSeasonTeamFromID(t, srv, seasonID, teamIDs[0])
	defer r.Body.Close()
	if r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("add team to active season: want 422, got %d", r.StatusCode)
	}
}

func TestSeasonRoster_AddAndList(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	pID := createTestPlayer(t, srv, "Alice", "Smith")
	setPlayerTeam(t, pID, teamIDs[0])

	rr := postRosterPlayer(t, srv, seasonID, teamIDs[0], pID)
	defer rr.Body.Close()
	if rr.StatusCode != http.StatusCreated {
		t.Fatalf("add roster player: want 201, got %d", rr.StatusCode)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var roster []map[string]any
	json.NewDecoder(resp.Body).Decode(&roster)
	if len(roster) != 1 {
		t.Fatalf("want 1 roster entry, got %d", len(roster))
	}
	if int64(roster[0]["player_id"].(float64)) != pID {
		t.Errorf("player_id: want %d, got %v", pID, roster[0]["player_id"])
	}
}

func TestSeasonRoster_DuplicateOnOtherTeamRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, teamIDs)

	pID := createTestPlayer(t, srv, "Bob", "Jones")
	setPlayerTeam(t, pID, teamIDs[0])

	postRosterPlayer(t, srv, seasonID, teamIDs[0], pID).Body.Close()

	// Same player on a different team must be rejected.
	rr2 := postRosterPlayer(t, srv, seasonID, teamIDs[1], pID)
	defer rr2.Body.Close()
	if rr2.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-team duplicate: want 400, got %d", rr2.StatusCode)
	}
}

func TestSeasonRoster_ActiveSeasonBlocked(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Lock the season by setting activated_at directly in DB (bypasses checklist).
	// isDraftSeason checks activated_at IS NULL, so this simulates first activation.
	if _, err := db.DB.Exec(
		`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, seasonID); err != nil {
		t.Fatalf("set activated_at: %v", err)
	}

	// Add team directly in DB (activated season cannot be modified via API).
	if _, err := db.DB.Exec(
		`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Alpha')`,
		seasonID, teamIDs[0]); err != nil {
		t.Fatalf("seed season_teams: %v", err)
	}
	pID := createTestPlayer(t, srv, "Carol", "Lee")
	setPlayerTeam(t, pID, teamIDs[0])

	rr := postRosterPlayer(t, srv, seasonID, teamIDs[0], pID)
	defer rr.Body.Close()
	if rr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("roster add to active season: want 422, got %d", rr.StatusCode)
	}
}

func TestSeasonTeams_RemoveTeam_ClearsRoster(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})
	pID := createTestPlayer(t, srv, "Dan", "Brown")
	setPlayerTeam(t, pID, teamIDs[0])
	postRosterPlayer(t, srv, seasonID, teamIDs[0], pID).Body.Close()

	del := httpDo(t, srv, http.MethodDelete,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), "")
	del.Body.Close()
	if del.StatusCode != http.StatusOK {
		t.Fatalf("delete season team: want 200, got %d", del.StatusCode)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var roster []map[string]any
	json.NewDecoder(resp.Body).Decode(&roster)
	if len(roster) != 0 {
		t.Errorf("roster after team removal: want 0, got %d", len(roster))
	}
}

func TestSeasonTeams_UpdateCaptain(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})
	pID := createTestPlayer(t, srv, "Eve", "Green")
	setPlayerTeam(t, pID, teamIDs[0])
	postRosterPlayer(t, srv, seasonID, teamIDs[0], pID).Body.Close()

	body := fmt.Sprintf(`{"season_name":"Alpha A","captain_id":%d}`, pID)
	upd := httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body)
	defer upd.Body.Close()
	if upd.StatusCode != http.StatusOK {
		t.Fatalf("update captain: want 200, got %d", upd.StatusCode)
	}

	var st map[string]any
	json.NewDecoder(upd.Body).Decode(&st)
	if int64(st["captain_id"].(float64)) != pID {
		t.Errorf("captain_id: want %d, got %v", pID, st["captain_id"])
	}
}

func TestSeasonTeams_UpdateCaptainNotOnRosterRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})
	// Player is created but NOT added to season roster.
	pID := createTestPlayer(t, srv, "Frank", "White")
	setPlayerTeam(t, pID, teamIDs[0])

	body := fmt.Sprintf(`{"season_name":"Alpha","captain_id":%d}`, pID)
	upd := httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body)
	defer upd.Body.Close()
	if upd.StatusCode != http.StatusBadRequest {
		t.Fatalf("captain not on roster: want 400, got %d", upd.StatusCode)
	}
}

// TestSeasonRoster_RemoveNonCaptain_CaptainUnchanged verifies that removing a
// non-captain player from a draft season roster leaves the team's captain_id intact.
func TestSeasonRoster_RemoveNonCaptain_CaptainUnchanged(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	captainID := createTestPlayer(t, srv, "Cap", "Tain")
	otherID := createTestPlayer(t, srv, "Other", "Player")

	// Add both players to the roster.
	postRosterPlayer(t, srv, seasonID, teamIDs[0], captainID).Body.Close()
	postRosterPlayer(t, srv, seasonID, teamIDs[0], otherID).Body.Close()

	// Assign captainID as captain.
	body := fmt.Sprintf(`{"season_name":"Alpha","captain_id":%d}`, captainID)
	httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body).Body.Close()

	// Remove the NON-captain player.
	del := httpDo(t, srv, http.MethodDelete,
		fmt.Sprintf("/api/seasons/%d/teams/%d/roster/%d", seasonID, teamIDs[0], otherID), "")
	defer del.Body.Close()
	if del.StatusCode != http.StatusOK {
		t.Fatalf("remove non-captain: want 200, got %d", del.StatusCode)
	}

	// Captain must still be set.
	var captainNow *int64
	if err := db.DB.QueryRow(
		`SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`,
		seasonID, teamIDs[0]).Scan(&captainNow); err != nil {
		t.Fatalf("query captain: %v", err)
	}
	if captainNow == nil || *captainNow != captainID {
		t.Errorf("captain_id: want %d, got %v", captainID, captainNow)
	}
}

// TestSeasonRoster_RemoveCaptain_ClearsCaptain verifies that removing the current
// captain from a draft season roster atomically sets captain_id to NULL in season_teams.
func TestSeasonRoster_RemoveCaptain_ClearsCaptain(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	captainID := createTestPlayer(t, srv, "Cap", "Tain")

	// Add player and assign as captain.
	postRosterPlayer(t, srv, seasonID, teamIDs[0], captainID).Body.Close()
	body := fmt.Sprintf(`{"season_name":"Alpha","captain_id":%d}`, captainID)
	httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body).Body.Close()

	// Confirm captain is set before the DELETE.
	var before *int64
	db.DB.QueryRow(`SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`,
		seasonID, teamIDs[0]).Scan(&before)
	if before == nil || *before != captainID {
		t.Fatalf("precondition: captain_id should be %d, got %v", captainID, before)
	}

	// Remove the captain from the roster.
	del := httpDo(t, srv, http.MethodDelete,
		fmt.Sprintf("/api/seasons/%d/teams/%d/roster/%d", seasonID, teamIDs[0], captainID), "")
	defer del.Body.Close()
	if del.StatusCode != http.StatusOK {
		t.Fatalf("remove captain: want 200, got %d", del.StatusCode)
	}

	// captain_id must now be NULL in season_teams.
	var after *int64
	if err := db.DB.QueryRow(
		`SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`,
		seasonID, teamIDs[0]).Scan(&after); err != nil {
		t.Fatalf("query captain after removal: %v", err)
	}
	if after != nil {
		t.Errorf("captain_id: want NULL after captain removed from roster, got %d", *after)
	}

	// Verify the roster row is also gone.
	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND player_id=?`,
		seasonID, captainID).Scan(&count)
	if count != 0 {
		t.Errorf("season_rosters: player row should be deleted, count=%d", count)
	}
}

func TestSeasonTeams_CopyFromPriorWithRoster(t *testing.T) {
	srv := testServer(t)
	leagueID, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, prevSeasonID, []int64{teamIDs[0]})
	pID := createTestPlayer(t, srv, "Grace", "Hall")
	setPlayerTeam(t, pID, teamIDs[0])
	postRosterPlayer(t, srv, prevSeasonID, teamIDs[0], pID).Body.Close()

	// Give the prior season an end_date so PreviousSeason can find it (correction 6).
	if _, err := db.DB.Exec(`UPDATE seasons SET end_date='2026-09-30' WHERE id=?`, prevSeasonID); err != nil {
		t.Fatalf("set end_date: %v", err)
	}

	// Create a new season in the same league.
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Fall 2026","start_date":"2026-10-01"}`, leagueID)))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	newSeasonID := int64(s2["id"].(float64))

	// Copy team from prior season, preserving roster.
	cpBody := fmt.Sprintf(`{"from_team_id":%d,"from_season_id":%d}`, teamIDs[0], prevSeasonID)
	cp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, newSeasonID),
		"application/json", strings.NewReader(cpBody))
	cp.Body.Close()
	if cp.StatusCode != http.StatusCreated {
		t.Fatalf("copy team: want 201, got %d", cp.StatusCode)
	}

	rosterResp, _ := http.Get(
		fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, newSeasonID, teamIDs[0]))
	defer rosterResp.Body.Close()
	var roster []map[string]any
	json.NewDecoder(rosterResp.Body).Decode(&roster)

	found := false
	for _, e := range roster {
		if int64(e["player_id"].(float64)) == pID {
			found = true
		}
	}
	if !found {
		t.Errorf("copied roster must contain player %d; got %v", pID, roster)
	}
}

func TestSeasonTeams_MarkStaleWhenMatchesExist(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-09-01"
	_, seasonID := seedScheduleFixture(t, srv, startDate)

	// Generate schedule (sets schedule_stale=0).
	generateAndGetMatches(t, srv, seasonID, startDate, nil)

	sResp, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d", srv.URL, seasonID))
	var s1 map[string]any
	json.NewDecoder(sResp.Body).Decode(&s1)
	sResp.Body.Close()
	if s1["schedule_stale"].(bool) {
		t.Fatal("schedule_stale should be false immediately after generation")
	}

	// Add a new team -- unplayed matches exist so stale flag must be set.
	postNewSeasonTeam(t, srv, seasonID, "Delta").Body.Close()

	sResp2, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d", srv.URL, seasonID))
	var s2 map[string]any
	json.NewDecoder(sResp2.Body).Decode(&s2)
	sResp2.Body.Close()
	if !s2["schedule_stale"].(bool) {
		t.Error("schedule_stale must be true after adding a team when unplayed matches exist")
	}
}

// TestSeasonChecklist_LegacySeason_CanActivate verifies correction 2:
// a season with teams_managed=0 (legacy) bypasses all checklist enforcement.
// The season is created via API (teams_managed=1) then downgraded in the DB
// to simulate a pre-Phase-One record.
func TestSeasonChecklist_LegacySeason_CanActivate(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// Downgrade to legacy mode (simulates seasons created before Phase One).
	if _, err := db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, sid); err != nil {
		t.Fatalf("set teams_managed=0: %v", err)
	}

	c := getChecklist(t, srv, sid)
	if canActivate, _ := c["can_activate"].(bool); !canActivate {
		t.Errorf("legacy season: want can_activate=true; got %v", c)
	}
	blockers, _ := c["blockers"].([]any)
	if len(blockers) != 0 {
		t.Errorf("legacy season: want no blockers, got %v", blockers)
	}
}

// TestSeasonChecklist_ManagedNoTeams_BlocksTooFew verifies correction 1:
// a managed season (teams_managed=1, set by createSeason) with no teams
// returns TEAMS_TOO_FEW and cannot activate.
func TestSeasonChecklist_ManagedNoTeams_BlocksTooFew(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	// sid was created via API -> teams_managed=1; no teams added yet.

	c := getChecklist(t, srv, sid)
	if canActivate, _ := c["can_activate"].(bool); canActivate {
		t.Error("managed season with no teams: want can_activate=false")
	}
	blockers, _ := c["blockers"].([]any)
	found := false
	for _, b := range blockers {
		bm := b.(map[string]any)
		if bm["code"].(string) == "TEAMS_TOO_FEW" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TEAMS_TOO_FEW blocker; got: %v", blockers)
	}
}

func TestSeasonChecklist_TwoTeamsNoSchedule_Blocked(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, teamIDs)

	// One player per team on roster, set as captain.
	for i, tid := range teamIDs {
		pID := createTestPlayer(t, srv, fmt.Sprintf("P%d", i), "Tester")
		setPlayerTeam(t, pID, tid)
		postRosterPlayer(t, srv, seasonID, tid, pID).Body.Close()
		httpDo(t, srv, http.MethodPut,
			fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, tid),
			fmt.Sprintf(`{"season_name":"Team%d","captain_id":%d}`, i, pID)).Body.Close()
	}

	c := getChecklist(t, srv, seasonID)
	if canActivate, _ := c["can_activate"].(bool); canActivate {
		t.Errorf("no schedule generated: want can_activate=false; got %v", c)
	}

	blockers, _ := c["blockers"].([]any)
	found := false
	for _, b := range blockers {
		bm := b.(map[string]any)
		if bm["code"].(string) == "NO_SCHEDULE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected NO_SCHEDULE blocker; got: %v", blockers)
	}
}

func TestSeasonChecklist_AllGood_CanActivate(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, teamIDs)

	for i, tid := range teamIDs {
		pID := createTestPlayer(t, srv, fmt.Sprintf("Q%d", i), "Ready")
		setPlayerTeam(t, pID, tid)
		postRosterPlayer(t, srv, seasonID, tid, pID).Body.Close()
		httpDo(t, srv, http.MethodPut,
			fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, tid),
			fmt.Sprintf(`{"season_name":"T%d","captain_id":%d}`, i, pID)).Body.Close()
	}

	// Generate a schedule (2 teams, 1 match).
	genBody := fmt.Sprintf(`{"season_id":%d,"start_date":"2026-09-01","schedule_type":"single_rr"}`, seasonID)
	genResp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(genBody))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	genResp.Body.Close()
	if genResp.StatusCode != http.StatusOK {
		t.Fatalf("generate: want 200, got %d", genResp.StatusCode)
	}

	c := getChecklist(t, srv, seasonID)
	if canActivate, _ := c["can_activate"].(bool); !canActivate {
		t.Errorf("happy path: want can_activate=true; checklist: %v", c)
	}
	blockers, _ := c["blockers"].([]any)
	if len(blockers) != 0 {
		t.Errorf("happy path: want no blockers, got %v", blockers)
	}
}

func TestActivateSeason_BlockedByChecklist(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// One team only -> TEAMS_TOO_FEW; no players -> TEAM_NO_PLAYERS; no schedule -> NO_SCHEDULE.
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	act := postActivateSeason(t, srv, seasonID)
	defer act.Body.Close()
	if act.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("activate with blockers: want 422, got %d", act.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(act.Body).Decode(&body)
	blockers, _ := body["blockers"].([]any)
	if len(blockers) == 0 {
		t.Error("expected blockers array in 422 response body")
	}
}

func TestAvailablePlayers_ExcludesRostered(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	p1 := createTestPlayer(t, srv, "Karl", "One")
	p2 := createTestPlayer(t, srv, "Lara", "Two")
	setPlayerTeam(t, p1, teamIDs[0])
	setPlayerTeam(t, p2, teamIDs[0])

	// Roster p1 only; p2 remains unrostered.
	postRosterPlayer(t, srv, seasonID, teamIDs[0], p1).Body.Close()

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/players/available", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var players []map[string]any
	json.NewDecoder(resp.Body).Decode(&players)

	foundP1, foundP2 := false, false
	for _, p := range players {
		pid := int64(p["id"].(float64))
		if pid == p1 {
			foundP1 = true
		}
		if pid == p2 {
			foundP2 = true
		}
	}
	if foundP1 {
		t.Error("rostered player must not appear in available list")
	}
	if !foundP2 {
		t.Error("unrostered player must appear in available list")
	}
}

func TestPreviousSeason_ReturnsNilWhenNoPrior(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/previous", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["season"] != nil {
		t.Errorf("want null season when no prior exists, got %v", body["season"])
	}
	teams, _ := body["teams"].([]any)
	if len(teams) != 0 {
		t.Errorf("want empty teams list, got %d items", len(teams))
	}
}

func TestPreviousSeason_ReturnsTeamsFromSeasonTeams(t *testing.T) {
	srv := testServer(t)
	leagueID, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Register both teams in the prior season.
	ensureSeasonTeams(t, prevSeasonID, teamIDs)

	// Give the prior season an end_date so it qualifies as a previous season.
	if _, err := db.DB.Exec(`UPDATE seasons SET end_date='2026-09-30' WHERE id=?`, prevSeasonID); err != nil {
		t.Fatalf("set end_date: %v", err)
	}

	// Create a newer season.
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(
			`{"league_id":%d,"name":"Fall 2026","start_date":"2026-10-01"}`, leagueID)))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	newSeasonID := int64(s2["id"].(float64))

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/previous", srv.URL, newSeasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["season"] == nil {
		t.Fatal("want a previous season, got null")
	}
	teams, _ := body["teams"].([]any)
	if len(teams) != 2 {
		t.Errorf("want 2 prior teams, got %d", len(teams))
	}
}

func TestSaveRounds_BlockedByInsufficientRoster(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Add both teams to season (enables roster enforcement).
	ensureSeasonTeams(t, seasonID, teamIDs)

	// Add 1 player per team (below the 3-player minimum).
	for i, tid := range teamIDs {
		pID := createTestPlayer(t, srv, fmt.Sprintf("Rnd%d", i), "Player")
		setPlayerTeam(t, pID, tid)
		postRosterPlayer(t, srv, seasonID, tid, pID).Body.Close()
	}

	// Insert a match directly so we have a match ID to target.
	res, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number)
		 VALUES (?,?,?,?,?)`,
		seasonID, teamIDs[0], teamIDs[1], "2026-09-01", 1)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	matchID, _ := res.LastInsertId()

	rr := httpDo(t, srv, http.MethodPost,
		fmt.Sprintf("/api/matches/%d/rounds", matchID), `{"rounds":[]}`)
	defer rr.Body.Close()
	if rr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("insufficient roster: want 422, got %d", rr.StatusCode)
	}

	var errBody map[string]string
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message in 422 body")
	}
}

// --- Correction regression tests ---

// TestSeasonActivated_SetupLockedPersistently verifies correction 3:
// activated_at is set once on first activation. Simulating a second season
// becoming active (deactivating the first) must NOT re-enable setup on the
// first season -- isDraftSeason checks activated_at, not active.
func TestSeasonActivated_SetupLockedPersistently(t *testing.T) {
	srv := testServer(t)
	leagueID, s1ID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Lock season 1 via DB (teams_managed=1 so API activation would need teams).
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, s1ID)

	// Create season 2 and activate it (demotes s1 to active=0).
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"S2","start_date":"2027-01-01"}`, leagueID)))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	s2ID := int64(s2["id"].(float64))

	// Set s2 active via DB (it has teams_managed=1 but no teams -> checklist blocks API activate).
	db.DB.Exec(`UPDATE seasons SET active=0 WHERE league_id=?`, leagueID)
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, s2ID)

	// Now season 1 has active=0 but activated_at is still set.
	// Adding a team to season 1 must still be rejected.
	r := postSeasonTeamFromID(t, srv, s1ID, teamIDs[0])
	defer r.Body.Close()
	if r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("add team to locked (deactivated) season: want 422, got %d", r.StatusCode)
	}
}

// TestAvailablePlayers_IncludesUnassigned verifies correction 4:
// players with no team_id (unassigned to any team) appear in available players.
func TestAvailablePlayers_IncludesUnassigned(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	postSeasonTeamFromID(t, srv, seasonID, teamIDs[0]).Body.Close()

	// Create a player with no team assigned.
	pUnassigned := createTestPlayer(t, srv, "Zara", "Solo")
	// pUnassigned has no team_id (created via API, no team_id in body).

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/players/available", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var players []map[string]any
	json.NewDecoder(resp.Body).Decode(&players)

	found := false
	for _, p := range players {
		if int64(p["id"].(float64)) == pUnassigned {
			found = true
		}
	}
	if !found {
		t.Errorf("unassigned player %d must appear in available list; got %d players", pUnassigned, len(players))
	}
}

// TestAvailablePlayers_IncludesCrossLeague verifies correction 4:
// players assigned to a team in a different league appear in available players.
func TestAvailablePlayers_IncludesCrossLeague(t *testing.T) {
	srv := testServer(t)
	_, seasonID, _ := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Create a second league with a team and a player.
	lg2Resp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg2 map[string]any
	json.NewDecoder(lg2Resp.Body).Decode(&lg2)
	lg2Resp.Body.Close()
	lg2ID := int64(lg2["id"].(float64))

	tm2Resp, _ := http.Post(srv.URL+"/api/teams", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Outsiders"}`, lg2ID)))
	var tm2 map[string]any
	json.NewDecoder(tm2Resp.Body).Decode(&tm2)
	tm2Resp.Body.Close()
	otherTeamID := int64(tm2["id"].(float64))

	pCrossLeague := createTestPlayer(t, srv, "Cross", "Player")
	setPlayerTeam(t, pCrossLeague, otherTeamID)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/players/available", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var players []map[string]any
	json.NewDecoder(resp.Body).Decode(&players)

	found := false
	for _, p := range players {
		if int64(p["id"].(float64)) == pCrossLeague {
			found = true
		}
	}
	if !found {
		t.Errorf("cross-league player %d must appear in available list; got %v players", pCrossLeague, len(players))
	}
}

// TestSeasonTeams_FromSeasonID_MustBePreviousSeason verifies correction 5:
// from_season_id must be the immediately previous season, not just any season.
func TestSeasonTeams_FromSeasonID_MustBePreviousSeason(t *testing.T) {
	srv := testServer(t)
	leagueID, olderSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-01-01", "Alpha", "Bravo")

	// Register team in older season.
	postSeasonTeamFromID(t, srv, olderSeasonID, teamIDs[0]).Body.Close()
	db.DB.Exec(`UPDATE seasons SET end_date='2026-06-01' WHERE id=?`, olderSeasonID)

	// Create a middle season (not the draft) with its own end_date.
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Middle","start_date":"2026-07-01"}`, leagueID)))
	var s2m map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2m)
	s2Resp.Body.Close()
	middleSeasonID := int64(s2m["id"].(float64))
	db.DB.Exec(`UPDATE seasons SET end_date='2026-12-31' WHERE id=?`, middleSeasonID)

	// Create the draft season (start_date after middle).
	draftResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Draft","start_date":"2027-01-01"}`, leagueID)))
	var draftS map[string]any
	json.NewDecoder(draftResp.Body).Decode(&draftS)
	draftResp.Body.Close()
	draftSeasonID := int64(draftS["id"].(float64))

	// Try to copy using the OLDER season (not the immediately previous one).
	cpBody := fmt.Sprintf(`{"from_team_id":%d,"from_season_id":%d}`, teamIDs[0], olderSeasonID)
	cp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, draftSeasonID),
		"application/json", strings.NewReader(cpBody))
	cp.Body.Close()
	if cp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-prior from_season_id: want 400, got %d", cp.StatusCode)
	}
}

// TestSeasonTeams_FromSeasonID_TeamNotInPrevSeason_Rejected verifies correction 5:
// the team must have participated in the previous season.
func TestSeasonTeams_FromSeasonID_TeamNotInPrevSeason_Rejected(t *testing.T) {
	srv := testServer(t)
	leagueID, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// prevSeason has NO season_teams rows for teamIDs[0] -- it was managed (created via API).
	db.DB.Exec(`UPDATE seasons SET end_date='2026-09-30' WHERE id=?`, prevSeasonID)

	// Create draft season.
	draftResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Draft","start_date":"2026-10-01"}`, leagueID)))
	var draftS map[string]any
	json.NewDecoder(draftResp.Body).Decode(&draftS)
	draftResp.Body.Close()
	draftSeasonID := int64(draftS["id"].(float64))

	// Copy with from_season_id = prevSeason, but team is NOT in prevSeason's season_teams.
	cpBody := fmt.Sprintf(`{"from_team_id":%d,"from_season_id":%d}`, teamIDs[0], prevSeasonID)
	cp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, draftSeasonID),
		"application/json", strings.NewReader(cpBody))
	cp.Body.Close()
	if cp.StatusCode != http.StatusBadRequest {
		t.Fatalf("team not in prev season: want 400, got %d", cp.StatusCode)
	}
}

// TestSeasonRosters_DbLevelEnforcementTriggered verifies correction 7:
// inserting into season_rosters without a matching season_teams row is blocked
// at the database level by the trigger.
func TestSeasonRosters_DbLevelEnforcementTriggered(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	pID := createTestPlayer(t, srv, "Trigger", "Test")

	// Insert directly into season_rosters WITHOUT adding the team to season_teams first.
	_, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamIDs[0], pID)
	if err == nil {
		t.Fatal("expected trigger to reject insert into season_rosters without season_teams row")
	}
}

// TestSeasonRosters_DbLevelEnforcementAllowsValid verifies correction 7:
// inserting into season_rosters WITH a matching season_teams row succeeds.
func TestSeasonRosters_DbLevelEnforcementAllowsValid(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Add the team to season_teams first (satisfies trigger condition).
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	pID := createTestPlayer(t, srv, "Valid", "Insert")

	// Insert directly into season_rosters WITH a matching season_teams row.
	_, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamIDs[0], pID)
	if err != nil {
		t.Fatalf("trigger should allow valid insert: %v", err)
	}
}

// TestSeasonRoster_IncludesPlayerNumber verifies that GET /api/seasons/{id}/teams/{tid}/roster
// returns player_number for players that have one set.
func TestSeasonRoster_IncludesPlayerNumber(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, teamIDs[:1])
	teamID := teamIDs[0]

	// Create player with player_number via API.
	body := `{"first_name":"Jane","last_name":"Doe","player_number":"99","handicap":2.5}`
	resp, err := http.Post(srv.URL+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST player: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST player: want 201, got %d: %s", resp.StatusCode, b)
	}
	var p map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		resp.Body.Close()
		t.Fatalf("POST player decode: %v", err)
	}
	resp.Body.Close()
	rawID, ok := p["id"]
	if !ok {
		t.Fatal("POST player response missing id field")
	}
	playerID := int64(rawID.(float64))

	postRosterPlayer(t, srv, seasonID, teamID, playerID).Body.Close()

	r, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamID))
	if err != nil {
		t.Fatalf("GET roster: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", r.StatusCode)
	}
	var roster []map[string]any
	json.NewDecoder(r.Body).Decode(&roster)
	if len(roster) == 0 {
		t.Fatal("expected at least one roster entry")
	}
	got, ok := roster[0]["player_number"]
	if !ok {
		t.Fatal("player_number field missing from roster response")
	}
	if got != "99" {
		t.Errorf("player_number: want %q, got %v", "99", got)
	}
}

// TestSeasonRoster_ZeroHandicapIncluded verifies that a player with handicap=0
// appears in the roster response with "handicap":0, not omitted.
func TestSeasonRoster_ZeroHandicapIncluded(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, teamIDs[:1])
	teamID := teamIDs[0]

	body := `{"first_name":"Zero","last_name":"Handi","handicap":0}`
	resp, err := http.Post(srv.URL+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST player: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST player: want 201, got %d: %s", resp.StatusCode, b)
	}
	var p map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		resp.Body.Close()
		t.Fatalf("POST player decode: %v", err)
	}
	resp.Body.Close()
	rawID, ok := p["id"]
	if !ok {
		t.Fatal("POST player response missing id field")
	}
	playerID := int64(rawID.(float64))

	postRosterPlayer(t, srv, seasonID, teamID, playerID).Body.Close()

	r, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamID))
	if err != nil {
		t.Fatalf("GET roster: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", r.StatusCode)
	}
	var roster []map[string]any
	json.NewDecoder(r.Body).Decode(&roster)
	if len(roster) == 0 {
		t.Fatal("expected at least one roster entry")
	}
	hc, ok := roster[0]["handicap"]
	if !ok {
		t.Fatal("handicap field missing for zero-handicap player")
	}
	if hc != float64(0) {
		t.Errorf("handicap: want 0, got %v", hc)
	}
}

// --- Regression tests: corrections 1-4 ---

// TestCreateSeason_ResponseHasTeamsManagedTrue verifies that createSeason returns
// teams_managed=true immediately, matching the persisted value (correction 4).
func TestCreateSeason_ResponseHasTeamsManagedTrue(t *testing.T) {
	srv := testServer(t)
	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Test League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()

	sResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Spring 2026"}`, int64(lg["id"].(float64)))))
	defer sResp.Body.Close()
	if sResp.StatusCode != http.StatusCreated {
		t.Fatalf("create season: want 201, got %d", sResp.StatusCode)
	}
	var s map[string]any
	json.NewDecoder(sResp.Body).Decode(&s)
	if tm, _ := s["teams_managed"].(bool); !tm {
		t.Errorf("create season response: want teams_managed=true, got %v", s["teams_managed"])
	}
}

// TestScheduleGenerate_ManagedNoTeams_Rejected verifies that schedule generation
// returns 400 for a managed season with no season_teams (correction 1).
func TestScheduleGenerate_ManagedNoTeams_Rejected(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-09-01"
	// seedScheduleFixtureWithTeams creates a managed season but does NOT register season_teams.
	_, seasonID, _ := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")

	body := fmt.Sprintf(`{"season_id":%d,"start_date":%q,"schedule_type":"single_rr"}`, seasonID, startDate)
	resp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /matches/generate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+no teams: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

// TestByeRequest_ManagedNoTeams_RejectsDespiteLeagueTeams verifies that bye
// validation uses season_teams count for managed seasons even when the league
// has odd teams (correction 2). A managed season with 0 season_teams has an
// even (0) participating count and must reject bye requests.
func TestByeRequest_ManagedNoTeams_RejectsDespiteLeagueTeams(t *testing.T) {
	srv := testServer(t)
	// 3 league teams (odd) but none registered in season_teams.
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo", "Charlie")

	resp := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+no season_teams: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSeasonTeams_ManagedRequiresFromSeasonIdAlways verifies that from_team_id
// always requires from_season_id in managed seasons, regardless of whether a
// prior season exists (correction 3).
func TestSeasonTeams_ManagedRequiresFromSeasonIdAlways(t *testing.T) {
	srv := testServer(t)
	_, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	// Give the previous season an end_date so PreviousSeason can find it.
	db.DB.Exec(`UPDATE seasons SET end_date='2026-12-31' WHERE id=?`, prevSeasonID)

	// Create a new draft season in the same league.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, prevSeasonID).Scan(&leagueID)
	sResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Next Season","start_date":"2027-01-10"}`, leagueID)))
	var newSeason map[string]any
	json.NewDecoder(sResp.Body).Decode(&newSeason)
	sResp.Body.Close()
	newSeasonID := int64(newSeason["id"].(float64))

	// Attempt to add team without from_season_id -> must be rejected.
	body := fmt.Sprintf(`{"from_team_id":%d}`, teamIDs[0])
	resp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, newSeasonID),
		"application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+prior season+no from_season_id: want 400, got %d", resp.StatusCode)
	}
}

// TestSeasonTeams_ManagedFirstSeason_FromTeamIdRejected verifies that from_team_id
// is rejected for a managed season even when no prior season exists; the only
// allowed path for first-season team creation is the name field (correction 3).
func TestSeasonTeams_ManagedFirstSeason_FromTeamIdRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	// No prior season exists; from_team_id must still be rejected for managed seasons.
	body := fmt.Sprintf(`{"from_team_id":%d}`, teamIDs[0])
	resp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed first season+from_team_id: want 400, got %d", resp.StatusCode)
	}
}

// TestScheduleGenerate_ManagedRejectsFromSeasonId verifies that schedule generation
// rejects a nonzero from_season_id for managed seasons; prior-season inference is
// legacy-only (correction 1).
func TestScheduleGenerate_ManagedRejectsFromSeasonId(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-09-01"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	body := fmt.Sprintf(
		`{"season_id":%d,"start_date":%q,"schedule_type":"single_rr","from_season_id":999}`,
		seasonID, startDate)
	resp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /matches/generate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+from_season_id: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSeasonTeams_ListReturnsTeamNumber verifies that GET /api/seasons/{id}/teams
// includes the team_number field for teams that have one set.
// team_number is stored on the teams table and projected through seasonTeamSelect;
// it is display-only in Phase 1 and not writable via the teams API.
func TestSeasonTeams_ListReturnsTeamNumber(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Set team_number directly; updateTeam does not expose this field.
	if _, err := db.DB.Exec(`UPDATE teams SET team_number='07' WHERE id=?`, teamIDs[0]); err != nil {
		t.Fatalf("set team_number: %v", err)
	}
	ensureSeasonTeams(t, seasonID, teamIDs[:1])

	r, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET season teams: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", r.StatusCode)
	}
	var teams []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&teams); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(teams) == 0 {
		t.Fatal("expected at least one team in response")
	}
	got, ok := teams[0]["team_number"]
	if !ok {
		t.Fatal("team_number field missing from season-teams response")
	}
	if got != "07" {
		t.Errorf("team_number: want %q, got %v", "07", got)
	}
}

// TestByeRequest_ManagedTeamNotInSeasonTeams_Rejected verifies that a bye request
// for a team that belongs to the league but is not registered in season_teams is
// rejected for managed seasons (correction 2).
func TestByeRequest_ManagedTeamNotInSeasonTeams_Rejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo", "Charlie")
	// Register only Alpha and Bravo; Charlie is in the league but not in season_teams.
	ensureSeasonTeams(t, seasonID, teamIDs[:2])

	// Charlie is not registered -- bye request must be rejected.
	resp := postByeRequest(t, srv, seasonID, teamIDs[2], 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("team not in season_teams: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestUpdateSeasonTeam_BlankNameRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha")
	ensureSeasonTeams(t, seasonID, teamIDs)
	teamID := teamIDs[0]

	putTeam := func(body string) *http.Response {
		req, _ := http.NewRequest(http.MethodPut,
			fmt.Sprintf("%s/api/seasons/%d/teams/%d", srv.URL, seasonID, teamID),
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT season team: %v", err)
		}
		return resp
	}

	cases := []struct{ label, body string }{
		{"empty string", `{"season_name":"","captain_id":null}`},
		{"whitespace only", `{"season_name":"   ","captain_id":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			resp := putTeam(tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", resp.StatusCode)
			}
			var errBody map[string]string
			json.NewDecoder(resp.Body).Decode(&errBody)
			if errBody["error"] == "" {
				t.Error("expected non-empty error message")
			}
		})
	}

	// Verify stored season_name was not blanked by the rejected requests.
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET teams: %v", err)
	}
	defer resp.Body.Close()
	var teams []map[string]any
	json.NewDecoder(resp.Body).Decode(&teams)
	if len(teams) == 0 {
		t.Fatal("expected at least one registered team")
	}
	if name, _ := teams[0]["season_name"].(string); name == "" {
		t.Errorf("season_name was blanked after rejected PUT; got %q", name)
	}
}
