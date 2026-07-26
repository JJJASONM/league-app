package handlers_test

// Season close handler integration tests:
//   GET  /api/seasons/{id}/close-preview  - TestClosePreview_*
//   POST /api/seasons/{id}/close           - TestCloseSeason_*

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

// closeTestActiveSeason returns an activated season ID seeded with one match
// in week 1 (completed). The week is NOT closed in league_weeks yet.
func closeTestActiveSeason(t *testing.T, srv string) int64 {
	t.Helper()
	sid := seedSeason(t, srv)

	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID); err != nil {
		t.Fatalf("closeTestActiveSeason: league: %v", err)
	}
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Alpha')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Bravo')`, leagueID)
	tA, _ := rA.LastInsertId()
	tB, _ := rB.LastInsertId()

	if _, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed) VALUES (?,?,?,1,1)`,
		sid, tA, tB,
	); err != nil {
		t.Fatalf("closeTestActiveSeason: match: %v", err)
	}
	if _, err := db.DB.Exec(
		`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid,
	); err != nil {
		t.Fatalf("closeTestActiveSeason: activate: %v", err)
	}
	return sid
}

// closeWeekDirect inserts a league_weeks row marking week 1 as closed.
func closeWeekDirect(t *testing.T, seasonID int64) {
	t.Helper()
	if _, err := db.DB.Exec(
		`INSERT INTO league_weeks (season_id, week_number, status) VALUES (?,1,'closed')`, seasonID,
	); err != nil {
		t.Fatalf("closeWeekDirect: %v", err)
	}
}

// ── GET /api/seasons/{id}/close-preview ───────────────────────────────────────

func TestClosePreview_DraftSeason_ReturnsCanCloseFalse(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL) // never activated → draft

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/close-preview", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["can_close"] != false {
		t.Errorf("want can_close=false, got %v", body["can_close"])
	}
	blockers, _ := body["blockers"].([]any)
	if len(blockers) == 0 {
		t.Fatal("expected at least one blocker for draft season")
	}
	b0 := blockers[0].(map[string]any)
	if b0["code"] != "SEASON_CLOSE_DRAFT" {
		t.Errorf("want SEASON_CLOSE_DRAFT, got %v", b0["code"])
	}
}

func TestClosePreview_NoSchedule_ReturnsCanCloseFalse(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	// Activate but add no matches.
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/close-preview", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["can_close"] != false {
		t.Errorf("want can_close=false, got %v", body["can_close"])
	}
	blockers, _ := body["blockers"].([]any)
	found := false
	for _, bl := range blockers {
		if bl.(map[string]any)["code"] == "SEASON_CLOSE_NO_SCHEDULE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SEASON_CLOSE_NO_SCHEDULE blocker, got %v", blockers)
	}
}

func TestClosePreview_UnclosedWeeks_ReturnsBlocker(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL) // has match, week NOT closed

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/close-preview", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["can_close"] != false {
		t.Errorf("want can_close=false, got %v", body["can_close"])
	}
	blockers, _ := body["blockers"].([]any)
	found := false
	for _, bl := range blockers {
		if bl.(map[string]any)["code"] == "SEASON_CLOSE_UNCLOSED" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SEASON_CLOSE_UNCLOSED blocker, got %v", blockers)
	}
}

func TestClosePreview_AllClear_ReturnsCanCloseTrue(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL)
	closeWeekDirect(t, sid)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/close-preview", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["can_close"] != true {
		t.Errorf("want can_close=true, got %v; blockers: %v", body["can_close"], body["blockers"])
	}
}

func TestClosePreview_NotFound_Returns404(t *testing.T) {
	srv := testServer(t)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/9999/close-preview", srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

// ── POST /api/seasons/{id}/close ──────────────────────────────────────────────

func TestCloseSeason_DraftSeason_Returns422(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL) // never activated

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/close", srv.URL, sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", resp.StatusCode)
	}
}

func TestCloseSeason_AlreadyClosed_Returns409(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL)
	closeWeekDirect(t, sid)

	// Close it once.
	req1, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/close", srv.URL, sid),
		strings.NewReader("{}"))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first close: want 200, got %d", resp1.StatusCode)
	}

	// Try to close again.
	req2, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/close", srv.URL, sid),
		strings.NewReader("{}"))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("second close: want 409, got %d", resp2.StatusCode)
	}
}

func TestCloseSeason_UnclosedWeeks_Returns409(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL) // week NOT closed

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/close", srv.URL, sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409, got %d", resp.StatusCode)
	}
}

func TestCloseSeason_Success_SetsClosedAt(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL)
	closeWeekDirect(t, sid)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/close", srv.URL, sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var season map[string]any
	json.NewDecoder(resp.Body).Decode(&season)
	if season["closed_at"] == nil || season["closed_at"] == "" {
		t.Errorf("want closed_at set in response, got %v", season["closed_at"])
	}
	// Active season should now be inactive.
	if season["active"] != false {
		t.Errorf("want active=false after close, got %v", season["active"])
	}
}
