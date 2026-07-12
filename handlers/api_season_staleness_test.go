package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

// scheduleStale reads schedule_stale for the season directly from the DB.
func scheduleStale(t *testing.T, sid int64) int {
	t.Helper()
	var v int
	db.DB.QueryRow(`SELECT COALESCE(schedule_stale,0) FROM seasons WHERE id=?`, sid).Scan(&v)
	return v
}

// seedUnplayedMatchForSeason inserts two teams and one unplayed match for sid
// so MarkStaleIfScheduled has a row to act on. Returns the two team IDs.
func seedUnplayedMatchForSeason(t *testing.T, sid int64) (tA, tB int64) {
	t.Helper()
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'StaleA')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'StaleB')`, leagueID)
	tA, _ = rA.LastInsertId()
	tB, _ = rB.LastInsertId()
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed) VALUES (?,?,?,1,0)`, sid, tA, tB)
	return tA, tB
}

// -- UpdateSeason staleness ---------------------------------------------------

func TestUpdateSeason_MarksStale(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	seedUnplayedMatchForSeason(t, sid)

	body := `{"name":"Spring 2026","start_date":"2026-09-01","schedule_type":"double_rr","num_weeks":10}`
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d", srv.URL, sid),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT season: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 on update, got %d", resp.StatusCode)
	}

	if scheduleStale(t, sid) != 1 {
		t.Error("want schedule_stale=1 after season update when unplayed matches exist")
	}
}

// -- UpdateByeRequest staleness -----------------------------------------------

func TestUpdateByeRequest_MarksStale(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	tA, tB := seedUnplayedMatchForSeason(t, sid)

	// Need a 3rd team (odd count) and season_teams rows for bye validation.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rC, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'StaleC')`, leagueID)
	tC, _ := rC.LastInsertId()
	for _, tid := range []int64{tA, tB, tC} {
		db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) SELECT ?,id,name FROM teams WHERE id=?`, sid, tid)
	}

	// Create a bye request (unapproved by default).
	body := fmt.Sprintf(`{"team_id":%d,"week_number":2,"reason":"Test"}`, tC)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST bye-request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	var br map[string]any
	json.NewDecoder(resp.Body).Decode(&br)
	byeID := int64(br["id"].(float64))

	// Reset staleness so the approve call is the trigger we measure.
	db.DB.Exec(`UPDATE seasons SET schedule_stale=0 WHERE id=?`, sid)

	// Approve the bye request.
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, sid, byeID),
		strings.NewReader(`{"approved":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT bye-request: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp2.StatusCode)
	}

	if scheduleStale(t, sid) != 1 {
		t.Error("want schedule_stale=1 after bye request approval when unplayed matches exist")
	}
}

// -- DeleteByeRequest staleness -----------------------------------------------

func TestDeleteByeRequest_MarksStale(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	tA, tB := seedUnplayedMatchForSeason(t, sid)

	// Seed a 3rd team and register all three in season_teams.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rC, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'DeleteC')`, leagueID)
	tC, _ := rC.LastInsertId()
	for _, tid := range []int64{tA, tB, tC} {
		db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) SELECT ?,id,name FROM teams WHERE id=?`, sid, tid)
	}

	// Create a bye request.
	body := fmt.Sprintf(`{"team_id":%d,"week_number":1,"reason":"Test"}`, tC)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST bye-request: %v", err)
	}
	defer resp.Body.Close()
	var br map[string]any
	json.NewDecoder(resp.Body).Decode(&br)
	byeID := int64(br["id"].(float64))

	// Approve it.
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, sid, byeID),
		strings.NewReader(`{"approved":true}`))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req) //nolint

	// Reset staleness so the delete call is the trigger we measure.
	db.DB.Exec(`UPDATE seasons SET schedule_stale=0 WHERE id=?`, sid)

	// Delete the bye request.
	req2, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, sid, byeID), nil)
	resp3, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("DELETE bye-request: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("want 200 on delete, got %d", resp3.StatusCode)
	}

	if scheduleStale(t, sid) != 1 {
		t.Error("want schedule_stale=1 after bye request deletion when unplayed matches exist")
	}
}
