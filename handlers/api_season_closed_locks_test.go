package handlers_test

// Handler integration tests for SEASON_CLOSED (HTTP 409) edit locks.
// Covers representative endpoints per access pattern:
//   - season-direct  : POST /api/seasons/{id}/skipped-weeks
//   - match-ID       : POST /api/matches/{id}/rounds  (primary)
//                      PATCH /api/matches/{id}/assign  (additional)
//   - schedule body  : POST /api/matches/generate
//   - season update  : PUT  /api/seasons/{id}
//   - week ops       : POST /api/seasons/{id}/weeks/{w}/close
//                      POST /api/seasons/{id}/weeks/{w}/reopen

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

// closeSeasonDirect stamps closed_at on a season, simulating Phase 1 closure.
func closeSeasonDirect(t *testing.T, seasonID int64) {
	t.Helper()
	if _, err := db.DB.Exec(
		`UPDATE seasons SET closed_at=CURRENT_TIMESTAMP, active=0 WHERE id=?`, seasonID,
	); err != nil {
		t.Fatalf("closeSeasonDirect: %v", err)
	}
}

// ---- helper: assert 409 Conflict -----------------------------------------------

func assertSeasonClosed409(t *testing.T, resp *http.Response) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 Conflict, got %d", resp.StatusCode)
	}
}

// ---- season-direct: POST /api/seasons/{id}/skipped-weeks -----------------------

func TestAddSkippedWeek_ClosedSeason_Returns409(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid)
	closeSeasonDirect(t, sid)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid),
		strings.NewReader(`{"skip_date":"2026-01-01","reason":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertSeasonClosed409(t, resp)
}

// ---- season-direct: PUT /api/seasons/{id} ---------------------------------------

func TestUpdateSeason_ClosedSeason_Returns409(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid)
	closeSeasonDirect(t, sid)

	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d", srv.URL, sid),
		strings.NewReader(`{"name":"Renamed","num_weeks":10,"schedule_type":"single_rr"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertSeasonClosed409(t, resp)
}

// ---- week ops: POST /api/seasons/{id}/weeks/{w}/close ---------------------------

func TestCloseWeek_ClosedSeason_Returns409(t *testing.T) {
	f := weekTestSeed(t)
	closeSeasonDirect(t, f.sid)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertSeasonClosed409(t, resp)
}

// ---- week ops: POST /api/seasons/{id}/weeks/{w}/reopen --------------------------

func TestReopenWeek_ClosedSeason_Returns409(t *testing.T) {
	f := weekTestSeed(t)
	// Mark week 1 as closed so reopen has something to act on, then close the season.
	db.DB.Exec(`INSERT INTO league_weeks (season_id, week_number, status) VALUES (?,1,'closed')`, f.sid)
	closeSeasonDirect(t, f.sid)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/reopen", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertSeasonClosed409(t, resp)
}

// ---- match-ID: POST /api/matches/{id}/rounds ------------------------------------
// The handler checks season-closed before RosterEligible, so this returns 409
// even when the season has no roster players (which would otherwise return 422).

func TestSaveRounds_ClosedSeason_Returns409(t *testing.T) {
	f := weekTestSeed(t)
	closeSeasonDirect(t, f.sid)

	body := fmt.Sprintf(`{"match_id":%d,"rounds":[]}`, f.matchID)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertSeasonClosed409(t, resp)
}

// ---- match-ID: PATCH /api/matches/{id}/assign -----------------------------------

func TestAssignMatchTeams_ClosedSeason_Returns409(t *testing.T) {
	f := weekTestSeed(t)
	closeSeasonDirect(t, f.sid)

	body := fmt.Sprintf(`{"home_team_id":%d,"away_team_id":%d}`, f.teamA, f.teamB)
	req, _ := http.NewRequest(http.MethodPatch,
		fmt.Sprintf("%s/api/matches/%d/assign", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertSeasonClosed409(t, resp)
}

// ---- schedule body: POST /api/matches/generate ----------------------------------

func TestGenerateSchedule_ClosedSeason_Returns409(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid)
	closeSeasonDirect(t, sid)

	body := fmt.Sprintf(`{"season_id":%d,"schedule_type":"single_rr"}`, sid)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/generate", srv.URL),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertSeasonClosed409(t, resp)
}
