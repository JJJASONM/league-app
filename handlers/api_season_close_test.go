package handlers_test

// Season close/reopen handler integration tests:
//   GET  /api/seasons/{id}/close-preview  - TestClosePreview_*
//   POST /api/seasons/{id}/close           - TestCloseSeason_*
//   POST /api/seasons/{id}/reopen          - TestReopenSeason_*

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

// --- POST /api/seasons/{id}/reopen ---

func TestReopenSeason_NotClosed_Returns409(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL) // active, not closed

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/reopen", srv.URL, sid), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409, got %d", resp.StatusCode)
	}
}

func TestReopenSeason_NotFound_Returns404(t *testing.T) {
	srv := testServer(t)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/9999/reopen", srv.URL), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestReopenSeason_Success_ClearsClosedAt(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL)
	closeWeekDirect(t, sid)
	closeSeasonDirect(t, sid)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/reopen", srv.URL, sid), nil)
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
	// closed_at must be cleared.
	if season["closed_at"] != nil && season["closed_at"] != "" {
		t.Errorf("want closed_at=null after reopen, got %v", season["closed_at"])
	}
	// active must remain false (Historical state, not Active).
	if season["active"] != false {
		t.Errorf("want active=false after reopen, got %v", season["active"])
	}
	// activated_at must be preserved.
	if season["activated_at"] == nil || season["activated_at"] == "" {
		t.Errorf("want activated_at preserved after reopen, got %v", season["activated_at"])
	}
}

// TestCloseSeason_SnapshotPersistedAndPreservedOnReopen closes a season through
// the real HTTP route with DB-backed standings data, verifies
// final_standings_snapshot is persisted with expected content, then reopens
// through the HTTP route and verifies the snapshot is preserved unchanged.
// final_standings_snapshot is intentionally not part of any JSON response
// (see doc/domains/seasons/README.md Phase 1 note); this test reads it
// directly from the database, consistent with the existing store-level
// snapshot tests.
func TestCloseSeason_SnapshotPersistedAndPreservedOnReopen(t *testing.T) {
	srv := testServer(t)
	sid := closeTestActiveSeason(t, srv.URL)
	closeWeekDirect(t, sid)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/close", srv.URL, sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, err := http.DefaultClient.Do(closeReq)
	if err != nil {
		t.Fatal(err)
	}
	defer closeResp.Body.Close()
	if closeResp.StatusCode != http.StatusOK {
		t.Fatalf("close: want 200, got %d", closeResp.StatusCode)
	}

	var snapshotAfterClose string
	if err := db.DB.QueryRow(
		`SELECT final_standings_snapshot FROM seasons WHERE id=?`, sid,
	).Scan(&snapshotAfterClose); err != nil {
		t.Fatalf("query snapshot after close: %v", err)
	}
	if snapshotAfterClose == "" {
		t.Fatal("want non-empty final_standings_snapshot after close")
	}
	for _, want := range []string{"schema_version", "Alpha"} {
		if !strings.Contains(snapshotAfterClose, want) {
			t.Errorf("snapshot after close missing %q; snapshot: %s", want, snapshotAfterClose)
		}
	}

	reopenReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/reopen", srv.URL, sid), nil)
	reopenResp, err := http.DefaultClient.Do(reopenReq)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenResp.Body.Close()
	if reopenResp.StatusCode != http.StatusOK {
		t.Fatalf("reopen: want 200, got %d", reopenResp.StatusCode)
	}

	var snapshotAfterReopen string
	if err := db.DB.QueryRow(
		`SELECT final_standings_snapshot FROM seasons WHERE id=?`, sid,
	).Scan(&snapshotAfterReopen); err != nil {
		t.Fatalf("query snapshot after reopen: %v", err)
	}
	if snapshotAfterReopen != snapshotAfterClose {
		t.Errorf("want snapshot unchanged after reopen\nafter close:  %s\nafter reopen: %s",
			snapshotAfterClose, snapshotAfterReopen)
	}
}
