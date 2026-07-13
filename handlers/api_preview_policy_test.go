package handlers_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

// TestCloseWeek_DraftSeasonReturns409 verifies that attempting to close a week
// for a season that has never been activated returns 409 Conflict.
// Uses a raw season (not weekTestSeed, which activates the season).
func TestCloseWeek_DraftSeasonReturns409(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL) // draft: active=0, activated_at=NULL

	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'DraftA')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'DraftB')`, leagueID)
	tA, _ := rA.LastInsertId()
	tB, _ := rB.LastInsertId()
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,1)`, sid, tA, tB)

	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", srv.URL, sid),
		"application/json",
		strings.NewReader(`{}`),
	)
	if err != nil {
		t.Fatalf("POST close week: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 Conflict for draft season, got %d", resp.StatusCode)
	}
}

// TestCloseWeek_ActiveSeasonAllowed verifies that close-week validation proceeds
// normally (no 409) for an active season. The week will still fail validation
// (no round results → validation errors), returning 422, not 409.
func TestCloseWeek_ActiveSeasonAllowed(t *testing.T) {
	// weekTestSeed activates the season, so this exercises the non-draft path.
	f := weekTestSeed(t)

	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		"application/json",
		strings.NewReader(`{}`),
	)
	if err != nil {
		t.Fatalf("POST close week: %v", err)
	}
	resp.Body.Close()
	// With no round results the validation layer fires → 422 (not the draft 409).
	if resp.StatusCode == http.StatusConflict {
		t.Error("want non-409 for active season: draft guard must not fire")
	}
}

// TestGenerateSchedule_ActiveWithCompleted_Returns409 verifies that attempting to
// regenerate a schedule for an active season that already has completed matches
// returns 409 Conflict.
func TestGenerateSchedule_ActiveWithCompleted_Returns409(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// Activate the season and insert a completed match directly.
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid)

	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'GuardA')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'GuardB')`, leagueID)
	tA, _ := rA.LastInsertId()
	tB, _ := rB.LastInsertId()
	db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed) VALUES (?,?,?,1,1)`,
		sid, tA, tB,
	)

	body := fmt.Sprintf(`{"season_id":%d,"schedule_type":"double_rr"}`, sid)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/matches/generate", srv.URL),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST generate: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 Conflict for active season with completed matches, got %d", resp.StatusCode)
	}
}

// TestGenerateSchedule_ActiveNoCompleted_Succeeds verifies that an active season
// with no completed matches can still have its schedule regenerated.
// New seasons are managed (teams_managed=1), so teams must be in season_teams.
func TestGenerateSchedule_ActiveNoCompleted_Succeeds(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID)
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'ActiveA')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'ActiveB')`, leagueID)
	tA, _ := rA.LastInsertId()
	tB, _ := rB.LastInsertId()

	// Register both teams in season_teams (required for managed seasons).
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) SELECT ?,id,name FROM teams WHERE id=?`, sid, tA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) SELECT ?,id,name FROM teams WHERE id=?`, sid, tB)

	// Activate the season without seeding any completed matches.
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid)

	body := fmt.Sprintf(`{"season_id":%d,"schedule_type":"double_rr"}`, sid)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/matches/generate", srv.URL),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST generate: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 for active season with no completed matches, got %d", resp.StatusCode)
	}
}
