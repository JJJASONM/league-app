package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"league_app/db"
)

func TestListSkippedWeeks_SkipDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	const wantDate = "2026-07-04"
	body := fmt.Sprintf(`{"skip_date":%q,"reason":"Independence Day"}`, wantDate)
	resp, err := http.Post(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp2, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var skips []map[string]any
	json.NewDecoder(resp2.Body).Decode(&skips)
	if len(skips) == 0 {
		t.Fatal("no skipped weeks returned")
	}
	got, _ := skips[0]["skip_date"].(string)
	if got != wantDate {
		t.Errorf("skip_date: want %q, got %q (must be YYYY-MM-DD)", wantDate, got)
	}
}

func TestListMatches_MatchDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	for _, m := range matches {
		raw, _ := m["match_date"].(string)
		if raw == "" {
			continue
		}
		if len(raw) != 10 || raw[4] != '-' || raw[7] != '-' {
			t.Errorf("match_date %q is not YYYY-MM-DD", raw)
		}
	}
}

func TestGenerateSchedule_SkipDateExcluded(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	const skipDate = "2026-07-13"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, []string{skipDate})
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	for _, m := range matches {
		if d, _ := m["match_date"].(string); d == skipDate {
			t.Errorf("match on %q should have been skipped", skipDate)
		}
	}
}

// TestGenerateSchedule_ISOSkipDateExcluded verifies that ISO timestamps sent as
// skip_dates (e.g. "2026-07-13T00:00:00Z") are normalised before comparison.
func TestGenerateSchedule_ISOSkipDateExcluded(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	const skipDate = "2026-07-13"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	// Simulate the frontend sending an ISO timestamp instead of YYYY-MM-DD.
	matches := generateAndGetMatches(t, srv, seasonID, startDate, []string{skipDate + "T00:00:00Z"})
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	for _, m := range matches {
		if d, _ := m["match_date"].(string); d == skipDate {
			t.Errorf("match on %q should have been skipped even with ISO timestamp skip date", skipDate)
		}
	}
}

func TestGenerateSchedule_ConsecutiveSkipsHonored(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	skipDates := []string{"2026-07-13", "2026-07-20"}
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, skipDates)
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	skipped := map[string]bool{"2026-07-13": true, "2026-07-20": true}
	for _, m := range matches {
		if d, _ := m["match_date"].(string); skipped[d] {
			t.Errorf("match on %q should have been skipped", d)
		}
	}
}

func TestSkippedWeeks_AreScopedToSeason(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	leagueID, firstSeasonID := seedScheduleFixture(t, srv, startDate)

	body := fmt.Sprintf(`{"league_id":%d,"name":"Second Season","start_date":%q}`, leagueID, startDate)
	resp, err := http.Post(srv.URL+"/api/seasons", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST second season: %v", err)
	}
	defer resp.Body.Close()
	var secondSeason map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&secondSeason); err != nil {
		t.Fatalf("decode second season: %v", err)
	}
	secondSeasonID := int64(secondSeason["id"].(float64))

	skipBody := `{"skip_date":"2026-07-13","reason":"First season only"}`
	skipResp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, firstSeasonID),
		"application/json", strings.NewReader(skipBody))
	if err != nil {
		t.Fatalf("POST skipped week: %v", err)
	}
	skipResp.Body.Close()

	listResp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, secondSeasonID))
	if err != nil {
		t.Fatalf("GET second season skipped weeks: %v", err)
	}
	defer listResp.Body.Close()
	var skips []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&skips); err != nil {
		t.Fatalf("decode skipped weeks: %v", err)
	}
	if len(skips) != 0 {
		t.Fatalf("second season inherited %d skipped weeks; want none", len(skips))
	}
}

func TestGenerateSchedule_PreservesCompletedMatches(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}

	completedID := int64(matches[0]["id"].(float64))
	if _, err := db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, completedID); err != nil {
		t.Fatalf("mark match completed: %v", err)
	}

	regenerated := generateAndGetMatches(t, srv, seasonID, startDate, []string{"2026-07-13"})
	for _, match := range regenerated {
		if int64(match["id"].(float64)) == completedID {
			if completed, _ := match["completed"].(bool); !completed {
				t.Fatal("preserved match lost its completed status")
			}
			return
		}
	}
	t.Fatalf("completed match %d was deleted during regeneration", completedID)
}



// approveByeRequest approves the bye request with the given id.
func approveByeRequest(t *testing.T, srv *httptest.Server, seasonID, byeID int64) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, seasonID, byeID),
		strings.NewReader(`{"approved":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT approve: %v", err)
	}
	resp.Body.Close()
}

// matchesInWeek filters matches for a specific week number.
func matchesInWeek(matches []map[string]any, week int) []map[string]any {
	var out []map[string]any
	for _, m := range matches {
		if wn, ok := m["week_number"].(float64); ok && int(wn) == week {
			out = append(out, m)
		}
	}
	return out
}

// teamAppearsInMatches returns true if teamID is home or away in any of the matches.
func teamAppearsInMatches(teamID int64, matches []map[string]any) bool {
	for _, m := range matches {
		if int64(m["home_team_id"].(float64)) == teamID ||
			int64(m["away_team_id"].(float64)) == teamID {
			return true
		}
	}
	return false
}

func TestByeRequest_EvenTeamCountRejected(t *testing.T) {
	srv := testServer(t)
	// Two teams = even -> bye request must be rejected.
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, teamIDs)
	resp := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("even-team bye request: want 400, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestByeRequest_TeamFromAnotherLeagueRejected(t *testing.T) {
	srv := testServer(t)
	// Create first league with 3 teams and a season.
	_, seasonID, teamIDs1 := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs1)

	// Create a second league and team.
	lg2Resp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg2 map[string]any
	json.NewDecoder(lg2Resp.Body).Decode(&lg2)
	lg2Resp.Body.Close()
	lg2ID := int64(lg2["id"].(float64))

	tm2Resp, _ := http.Post(srv.URL+"/api/teams", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Outsider"}`, lg2ID)))
	var tm2 map[string]any
	json.NewDecoder(tm2Resp.Body).Decode(&tm2)
	tm2Resp.Body.Close()
	foreignTeamID := int64(tm2["id"].(float64))

	resp := postByeRequest(t, srv, seasonID, foreignTeamID, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("foreign-team bye request: want 400, got %d", resp.StatusCode)
	}
}

func TestByeRequest_DuplicateRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("first request: want 201, got %d", r1.StatusCode)
	}

	r2 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate request: want 400, got %d", r2.StatusCode)
	}
}

func TestByeRequest_ApprovedHonoredInSchedule(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	// 3 teams (odd) -> one natural bye per week.
	// With Alpha(1), Bravo(2), Charlie(3) and single_rr:
	//   Week 1: Bravo vs Charlie  (Alpha bye)
	//   Week 2: Alpha vs Charlie  (Bravo bye)
	//   Week 3: Alpha vs Bravo    (Charlie bye)
	// Request Charlie's bye moved to week 1.
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	charlieID := teamIDs[2]

	// Create and approve the bye request.
	r := postByeRequest(t, srv, seasonID, charlieID, 1)
	var created map[string]any
	json.NewDecoder(r.Body).Decode(&created)
	r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("create bye: want 201, got %d", r.StatusCode)
	}
	byeID := int64(created["id"].(float64))
	approveByeRequest(t, srv, seasonID, byeID)

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	week1 := matchesInWeek(matches, 1)
	if len(week1) == 0 {
		t.Fatal("no matches in week 1")
	}
	if teamAppearsInMatches(charlieID, week1) {
		t.Errorf("Charlie should have the bye in week 1 but appears in a match")
	}
	// Exactly one match in week 1 (3 teams -> 1 match per week).
	if len(week1) != 1 {
		t.Errorf("week 1: want 1 match, got %d", len(week1))
	}
}

func TestByeRequest_UnapprovedIgnoredInSchedule(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	charlieID := teamIDs[2]

	// Create but do NOT approve the bye request for week 1.
	r := postByeRequest(t, srv, seasonID, charlieID, 1)
	r.Body.Close()

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	week1 := matchesInWeek(matches, 1)
	// Without approval, the natural rotation applies: Charlie appears in week 1.
	if !teamAppearsInMatches(charlieID, week1) {
		t.Error("unapproved request should have no effect; Charlie should play in week 1")
	}
}

func TestByeRequest_NaturalByeNotDuplicated(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	charlieID := teamIDs[2]

	r := postByeRequest(t, srv, seasonID, charlieID, 1)
	var created map[string]any
	json.NewDecoder(r.Body).Decode(&created)
	r.Body.Close()
	approveByeRequest(t, srv, seasonID, int64(created["id"].(float64)))

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	// Total matches for single_rr with 3 teams = 3 (one per week).
	if len(matches) != 3 {
		t.Errorf("expected 3 matches, got %d -- bye request must not create extra bye", len(matches))
	}
	// Every week has exactly 1 match.
	for week := 1; week <= 3; week++ {
		if n := len(matchesInWeek(matches, week)); n != 1 {
			t.Errorf("week %d: want 1 match, got %d", week, n)
		}
	}
}

// --- Player assignment (no duplicate) ---

func TestAssignExistingPlayer_NamePreserved(t *testing.T) {
	srv := testServer(t)

	// Create a league, team, and an unassigned player.
	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Test League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()
	lgID := int64(lg["id"].(float64))

	tmResp, _ := http.Post(srv.URL+"/api/teams", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Team A"}`, lgID)))
	var tm map[string]any
	json.NewDecoder(tmResp.Body).Decode(&tm)
	tmResp.Body.Close()
	teamID := int64(tm["id"].(float64))

	pResp, _ := http.Post(srv.URL+"/api/players", "application/json",
		strings.NewReader(`{"first_name":"Jane","last_name":"Doe","handicap":1.5}`))
	var player map[string]any
	json.NewDecoder(pResp.Body).Decode(&player)
	pResp.Body.Close()
	playerID := int64(player["id"].(float64))

	// Assign the player to the team using first_name/last_name (the correct approach).
	body := fmt.Sprintf(`{"first_name":"Jane","last_name":"Doe","handicap":1.5,"team_id":%d}`, teamID)
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/players/%d", srv.URL, playerID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT player: want 200, got %d", resp.StatusCode)
	}

	// Verify the name is still intact after assignment.
	getResp, _ := http.Get(fmt.Sprintf("%s/api/players/%d", srv.URL, playerID))
	var updated map[string]any
	json.NewDecoder(getResp.Body).Decode(&updated)
	getResp.Body.Close()

	if fn, _ := updated["first_name"].(string); fn != "Jane" {
		t.Errorf("first_name: want %q, got %q", "Jane", fn)
	}
	if ln, _ := updated["last_name"].(string); ln != "Doe" {
		t.Errorf("last_name: want %q, got %q", "Doe", ln)
	}
	if tid, _ := updated["team_id"].(float64); int64(tid) != teamID {
		t.Errorf("team_id: want %d, got %v", teamID, updated["team_id"])
	}

	// Verify no duplicate player was created.
	allResp, _ := http.Get(srv.URL + "/api/players")
	var all []map[string]any
	json.NewDecoder(allResp.Body).Decode(&all)
	allResp.Body.Close()
	if len(all) != 1 {
		t.Errorf("expected 1 player, got %d -- assignment must not create duplicates", len(all))
	}
}

// --- Bye request conflict and scope enforcement ---

// putByeApproval sends PUT .../bye-requests/{byeID} with {approved:approved}.
// Caller must close the returned response body.
func putByeApproval(t *testing.T, srv *httptest.Server, seasonID, byeID int64, approved bool) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"approved":%v}`, approved)
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, seasonID, byeID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT bye-approval: %v", err)
	}
	return resp
}

// deleteByeReq sends DELETE .../seasons/{sid}/bye-requests/{bid}.
func deleteByeReq(t *testing.T, srv *httptest.Server, seasonID, byeID int64) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, seasonID, byeID),
		nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE bye-request: %v", err)
	}
	return resp
}

// listByes returns the decoded bye requests for the given season.
func listByes(t *testing.T, srv *httptest.Server, seasonID int64) []map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET bye-requests: %v", err)
	}
	defer resp.Body.Close()
	var byes []map[string]any
	json.NewDecoder(resp.Body).Decode(&byes)
	return byes
}

// TestByeRequest_ConflictPreventsApproval verifies that a second approved bye
// for the same week is rejected even though the request itself was accepted.
func TestByeRequest_ConflictPreventsApproval(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	// Approve Alpha for week 1.
	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("create Alpha bye: want 201, got %d", r1.StatusCode)
	}
	resp := putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve Alpha: want 200, got %d", resp.StatusCode)
	}

	// Record Bravo for week 1 -- creation must succeed.
	r2 := postByeRequest(t, srv, seasonID, teamIDs[1], 1)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()
	if r2.StatusCode != http.StatusCreated {
		t.Fatalf("create Bravo bye: want 201, got %d", r2.StatusCode)
	}

	// Approving Bravo must be rejected because Alpha already holds week 1.
	resp2 := putByeApproval(t, srv, seasonID, int64(b2["id"].(float64)), true)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("approve conflict: want 400, got %d", resp2.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp2.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected a non-empty error message")
	}
}

// TestByeRequest_RejectedApprovalRemainsUnapproved verifies the bye request stays
// unapproved after a conflict rejection.
func TestByeRequest_RejectedApprovalRemainsUnapproved(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true).Body.Close()

	r2 := postByeRequest(t, srv, seasonID, teamIDs[1], 1)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()
	bravoID := int64(b2["id"].(float64))

	// Attempt (rejected) approval.
	putByeApproval(t, srv, seasonID, bravoID, true).Body.Close()

	// Confirm Bravo's request is still unapproved.
	byes := listByes(t, srv, seasonID)
	for _, b := range byes {
		if int64(b["id"].(float64)) == bravoID {
			if app, _ := b["approved"].(bool); app {
				t.Error("Bravo bye should remain unapproved after conflict rejection")
			}
			return
		}
	}
	t.Fatal("Bravo bye request not found in list")
}

// TestByeRequest_DifferentWeeksCanBothBeApproved verifies independent week slots.
func TestByeRequest_DifferentWeeksCanBothBeApproved(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()

	r2 := postByeRequest(t, srv, seasonID, teamIDs[1], 2)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()

	resp1 := putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("approve Alpha week 1: want 200, got %d", resp1.StatusCode)
	}

	resp2 := putByeApproval(t, srv, seasonID, int64(b2["id"].(float64)), true)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("approve Bravo week 2: want 200, got %d", resp2.StatusCode)
	}
}

// TestByeRequest_Week0CannotBeApproved verifies TBD requests are not approvable.
func TestByeRequest_Week0CannotBeApproved(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r := postByeRequest(t, srv, seasonID, teamIDs[0], 0)
	var b map[string]any
	json.NewDecoder(r.Body).Decode(&b)
	r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("create week-0 bye: want 201, got %d", r.StatusCode)
	}

	resp := putByeApproval(t, srv, seasonID, int64(b["id"].(float64)), true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("approve week-0: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message for week-0 approval")
	}
}

// TestByeRequest_WrongSeasonUpdateRejected verifies season scope on updates.
func TestByeRequest_WrongSeasonUpdateRejected(t *testing.T) {
	srv := testServer(t)

	// Season 1: 3-team league with a bye request.
	_, season1ID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, season1ID, teamIDs)
	r := postByeRequest(t, srv, season1ID, teamIDs[0], 1)
	var b map[string]any
	json.NewDecoder(r.Body).Decode(&b)
	r.Body.Close()
	byeID := int64(b["id"].(float64))

	// Season 2: a separate season in a different league.
	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Other Season"}`, int64(lg["id"].(float64)))))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	season2ID := int64(s2["id"].(float64))

	// Try to update bye via season 2's URL -- must be 404.
	resp := putByeApproval(t, srv, season2ID, byeID, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-season update: want 404, got %d", resp.StatusCode)
	}
}

// TestByeRequest_WrongSeasonDeleteRejected verifies season scope on deletes.
func TestByeRequest_WrongSeasonDeleteRejected(t *testing.T) {
	srv := testServer(t)

	_, season1ID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, season1ID, teamIDs)
	r := postByeRequest(t, srv, season1ID, teamIDs[0], 1)
	var b map[string]any
	json.NewDecoder(r.Body).Decode(&b)
	r.Body.Close()
	byeID := int64(b["id"].(float64))

	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Other Season"}`, int64(lg["id"].(float64)))))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	season2ID := int64(s2["id"].(float64))

	resp := deleteByeReq(t, srv, season2ID, byeID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-season delete: want 404, got %d", resp.StatusCode)
	}

	// The request must still exist under season 1.
	byes := listByes(t, srv, season1ID)
	found := false
	for _, by := range byes {
		if int64(by["id"].(float64)) == byeID {
			found = true
		}
	}
	if !found {
		t.Error("bye request was incorrectly deleted via wrong season URL")
	}
}

// TestByeRequest_DeterministicScheduleHonorsApproved confirms schedule generation
// honors the single approved request when a competing unapproved request exists
// for the same week.
func TestByeRequest_DeterministicScheduleHonorsApproved(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	// 3 teams: Week 1 natural bye = Alpha, Week 2 = Bravo, Week 3 = Charlie.
	// Approve Alpha for week 1 (no change); also record Bravo for week 1 (unapproved).
	// Schedule must still give week 1 to Alpha (the approved one).
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	alphaID := teamIDs[0]
	bravoID := teamIDs[1]

	r1 := postByeRequest(t, srv, seasonID, alphaID, 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true).Body.Close()

	// Bravo requests week 1 but is NOT approved.
	r2 := postByeRequest(t, srv, seasonID, bravoID, 1)
	r2.Body.Close()

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	week1 := matchesInWeek(matches, 1)
	if len(week1) == 0 {
		t.Fatal("no matches in week 1")
	}
	// Alpha should have the bye -- approved request wins.
	if teamAppearsInMatches(alphaID, week1) {
		t.Error("Alpha should have the week-1 bye (approved request) but appears in a match")
	}
	// Bravo must play (unapproved request ignored).
	if !teamAppearsInMatches(bravoID, week1) {
		t.Error("Bravo should play in week 1 (unapproved request ignored)")
	}
}

// TestByeRequest_TwoApprovedRequestsDifferentWeeks verifies that two approved
// bye requests for different weeks are both honoured in the generated schedule.
// This is the multi-request regression test for the pairwise-swap bug where the
// second request was silently dropped when the displaced source week had already
// been used in an earlier swap.
//
// 5 teams, single_rr natural byes:
//
//	week 1 = Alpha   week 2 = Delta   week 3 = Bravo   week 4 = Echo   week 5 = Charlie
//
// Approved: Echo   -> week 1  (Echo natural week 4)
//
//	Bravo  -> week 2  (Bravo natural week 3)
func TestByeRequest_TwoApprovedRequestsDifferentWeeks(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate,
		"Alpha", "Bravo", "Charlie", "Delta", "Echo")
	ensureSeasonTeams(t, seasonID, teamIDs)
	echoID := teamIDs[4]
	bravoID := teamIDs[1]

	// Create and approve Echo for week 1.
	r1 := postByeRequest(t, srv, seasonID, echoID, 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("create Echo bye: want 201, got %d", r1.StatusCode)
	}
	resp1 := putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("approve Echo: want 200, got %d", resp1.StatusCode)
	}

	// Create and approve Bravo for week 2.
	r2 := postByeRequest(t, srv, seasonID, bravoID, 2)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()
	if r2.StatusCode != http.StatusCreated {
		t.Fatalf("create Bravo bye: want 201, got %d", r2.StatusCode)
	}
	resp2 := putByeApproval(t, srv, seasonID, int64(b2["id"].(float64)), true)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("approve Bravo: want 200, got %d", resp2.StatusCode)
	}

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)

	// Total: single_rr with 5 teams = 10 matches.
	if len(matches) != 10 {
		t.Errorf("total matches: want 10, got %d", len(matches))
	}

	week1 := matchesInWeek(matches, 1)
	if len(week1) == 0 {
		t.Fatal("no matches in week 1")
	}
	if teamAppearsInMatches(echoID, week1) {
		t.Error("Echo should have the week-1 bye but appears in a match")
	}

	week2 := matchesInWeek(matches, 2)
	if len(week2) == 0 {
		t.Fatal("no matches in week 2")
	}
	if teamAppearsInMatches(bravoID, week2) {
		t.Error("Bravo should have the week-2 bye but appears in a match")
	}

	// Regenerating should produce identical bye assignments (deterministic).
	matches2 := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	if teamAppearsInMatches(echoID, matchesInWeek(matches2, 1)) {
		t.Error("regeneration: Echo should still have week-1 bye")
	}
	if teamAppearsInMatches(bravoID, matchesInWeek(matches2, 2)) {
		t.Error("regeneration: Bravo should still have week-2 bye")
	}
}

// TestGenerateSchedule_Returns409WhenClosedWeeksExist verifies that regenerating
// a schedule is blocked when the season has at least one closed week.
func TestGenerateSchedule_Returns409WhenClosedWeeksExist(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	generateAndGetMatches(t, srv, seasonID, startDate, nil)

	// Simulate closing week 1 by inserting a league_weeks row directly.
	if _, err := db.DB.Exec(
		`INSERT INTO league_weeks (season_id, week_number, status) VALUES (?,1,'closed')`,
		seasonID,
	); err != nil {
		t.Fatalf("seed closed week: %v", err)
	}

	body := fmt.Sprintf(`{"season_id":%d,"schedule_type":"double_rr","start_date":%q}`,
		seasonID, startDate)
	resp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /matches/generate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 Conflict when closed weeks exist, got %d", resp.StatusCode)
	}

	var body2 map[string]any
	json.NewDecoder(resp.Body).Decode(&body2)
	errMsg, _ := body2["error"].(string)
	if errMsg == "" {
		t.Error("want non-empty error message in response body")
	}
}
