package handlers_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

// -- Skip Weeks ---------------------------------------------------------------

func TestSkippedWeeks_CRUD(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// GET list -- empty
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid))
	if err != nil {
		t.Fatalf("GET skipped-weeks: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 on empty list, got %d", resp.StatusCode)
	}

	// POST create
	body := `{"skip_date":"2026-08-15","reason":"Summer break"}`
	resp2, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST skipped-weeks: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp2.Body)
		t.Fatalf("want 201 on create, got %d: %s", resp2.StatusCode, raw)
	}
	var created map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	swID := int64(created["id"].(float64))
	if swID == 0 {
		t.Fatal("want non-zero skip week ID")
	}

	// GET list -- one row
	resp3, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid))
	if err != nil {
		t.Fatalf("GET skipped-weeks after create: %v", err)
	}
	defer resp3.Body.Close()
	var list []map[string]any
	json.NewDecoder(resp3.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("want 1 skip week, got %d", len(list))
	}

	// DELETE
	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks/%d", srv.URL, sid, swID), nil)
	resp4, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE skipped-week: %v", err)
	}
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Errorf("want 200 on delete, got %d", resp4.StatusCode)
	}

	// GET list -- empty again
	resp5, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid))
	if err != nil {
		t.Fatalf("GET after delete: %v", err)
	}
	defer resp5.Body.Close()
	var list2 []map[string]any
	json.NewDecoder(resp5.Body).Decode(&list2)
	if len(list2) != 0 {
		t.Errorf("want 0 skip weeks after delete, got %d", len(list2))
	}
}

func TestSkippedWeeks_CreateMarksStale(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// Seed an unplayed match so MarkStaleIfScheduled has something to act on.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'B')`, leagueID)
	tA, _ := rA.LastInsertId()
	tB, _ := rB.LastInsertId()
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed) VALUES (?,?,?,1,0)`, sid, tA, tB)

	// POST create skip week
	body := `{"skip_date":"2026-09-01","reason":"Holiday"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}

	var stale int
	db.DB.QueryRow(`SELECT COALESCE(schedule_stale,0) FROM seasons WHERE id=?`, sid).Scan(&stale)
	if stale != 1 {
		t.Error("want schedule_stale=1 after skip week creation when unplayed matches exist")
	}
}

func TestSkippedWeeks_DeleteMarksStale(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// Seed an unplayed match.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'B')`, leagueID)
	tA, _ := rA.LastInsertId()
	tB, _ := rB.LastInsertId()
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed) VALUES (?,?,?,1,0)`, sid, tA, tB)

	// Create a skip week first.
	body := `{"skip_date":"2026-10-01","reason":"Bye"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	var sw map[string]any
	json.NewDecoder(resp.Body).Decode(&sw)
	swID := int64(sw["id"].(float64))

	// Reset staleness flag to distinguish from the create call above.
	db.DB.Exec(`UPDATE seasons SET schedule_stale=0 WHERE id=?`, sid)

	// DELETE the skip week.
	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks/%d", srv.URL, sid, swID), nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("want 200 on delete, got %d", resp2.StatusCode)
	}

	var stale int
	db.DB.QueryRow(`SELECT COALESCE(schedule_stale,0) FROM seasons WHERE id=?`, sid).Scan(&stale)
	if stale != 1 {
		t.Error("want schedule_stale=1 after skip week deletion when unplayed matches exist")
	}
}

// -- Bye Requests -------------------------------------------------------------

func TestByeRequests_CRUD(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// Seed an odd number of teams (3) with season_teams so bye validation passes.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'B')`, leagueID)
	rC, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'C')`, leagueID)
	tA, _ := rA.LastInsertId()
	tB, _ := rB.LastInsertId()
	tC, _ := rC.LastInsertId()
	for _, tid := range []int64{tA, tB, tC} {
		db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name)
			SELECT ?,id,name FROM teams WHERE id=?`, sid, tid)
	}

	// GET list -- empty
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, sid))
	if err != nil {
		t.Fatalf("GET bye-requests: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}

	// POST create
	body := fmt.Sprintf(`{"team_id":%d,"week_number":2,"reason":"Tournament"}`, tA)
	resp2, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST bye-requests: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp2.Body)
		t.Fatalf("want 201, got %d: %s", resp2.StatusCode, raw)
	}
	var created map[string]any
	json.NewDecoder(resp2.Body).Decode(&created)
	byeID := int64(created["id"].(float64))
	if byeID == 0 {
		t.Fatal("want non-zero bye request ID")
	}

	// PUT update (approve)
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, sid, byeID),
		strings.NewReader(`{"approved":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT bye-request: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp3.Body)
		t.Fatalf("want 200 on approve, got %d: %s", resp3.StatusCode, raw)
	}

	// DELETE
	req2, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, sid, byeID), nil)
	resp4, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("DELETE bye-request: %v", err)
	}
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Errorf("want 200 on delete, got %d", resp4.StatusCode)
	}

	// GET list -- empty again
	resp5, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, sid))
	if err != nil {
		t.Fatalf("GET after delete: %v", err)
	}
	defer resp5.Body.Close()
	var list []map[string]any
	json.NewDecoder(resp5.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("want 0 bye requests after delete, got %d", len(list))
	}
}
