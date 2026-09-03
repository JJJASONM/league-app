// Tests for Substitute Workflow Phase 1:
//
//	POST   /api/lineup-plans/{id}/substitute
//	DELETE /api/lineup-plans/{id}/substitute
//
// Uses testServerWithApplyAuth (api_apply_c1_test.go), which already wires a
// real ApplyAuthStore plus LineupMgr/RoundMgr/MatchMgr/SeasonMgr -- everything
// this workflow's auth and lock-check enforcement needs.
package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

// lineupSubFixture is the result of lineupSubSeed: a league/season/team/match
// with one lineup_plans row (week 1, home team, original player) ready to
// substitute.
type lineupSubFixture struct {
	seasonID int64
	teamID   int64
	matchID  int64
	original int64
	sub      int64
	planID   int64
}

// lineupSubSeed creates one league, one season, a home and away team, a
// week-1 match between them, one lineup_plans row for the home team's first
// slot (original player), and a second player on the same team who is not
// yet in the lineup (usable as a substitute).
func lineupSubSeed(t *testing.T) lineupSubFixture {
	t.Helper()
	d := db.DB

	r, err := d.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('Sub League','8ball','Monday')`)
	if err != nil {
		t.Fatalf("seed league: %v", err)
	}
	leagueID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?,?,?,?,?)`,
		leagueID, "Sub Season", "2026-01-01", "single_rr", 3)
	if err != nil {
		t.Fatalf("seed season: %v", err)
	}
	seasonID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Home Team')`, leagueID)
	if err != nil {
		t.Fatalf("seed home team: %v", err)
	}
	homeTeamID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Away Team')`, leagueID)
	if err != nil {
		t.Fatalf("seed away team: %v", err)
	}
	awayTeamID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, match_number) VALUES (?,?,?,1,1)`,
		seasonID, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("seed match: %v", err)
	}
	matchID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Original','Player',?,2.0)`, homeTeamID)
	if err != nil {
		t.Fatalf("seed original player: %v", err)
	}
	original, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Sub','Player',?,3.0)`, homeTeamID)
	if err != nil {
		t.Fatalf("seed sub player: %v", err)
	}
	sub, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO lineup_plans (season_id, team_id, week_number, player_id, is_sub) VALUES (?,?,1,?,0)`,
		seasonID, homeTeamID, original)
	if err != nil {
		t.Fatalf("seed lineup plan: %v", err)
	}
	planID, _ := r.LastInsertId()

	return lineupSubFixture{seasonID: seasonID, teamID: homeTeamID, matchID: matchID, original: original, sub: sub, planID: planID}
}

func lineupSubURL(base string, planID int64) string {
	return fmt.Sprintf("%s/api/lineup-plans/%d/substitute", base, planID)
}

// -- Auth ----------------------------------------------------------------------

func TestLineupSubstitute_NoHeader_Returns401(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)

	resp, err := http.Post(lineupSubURL(srv.URL, f.planID), "application/json",
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("want WWW-Authenticate header on 401")
	}
}

func TestLineupSubstitute_InvalidToken_Returns403(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer not-a-real-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestLineupSubstitute_StaticAdminToken_Returns403(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c1AdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize this route), got %d", resp.StatusCode)
	}
}

func TestLineupSubstitute_ScoreKeeperRole_Returns403(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-scorer", "score_keeper")
	if err != nil {
		t.Fatalf("create score_keeper user: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", resp.StatusCode)
	}
}

func TestLineupSubstitute_LeagueAdmin_Succeeds(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-league-admin", "league_admin")
	if err != nil {
		t.Fatalf("create league_admin user: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (league_admin reaches handler), got %d", resp.StatusCode)
	}
	var plan map[string]any
	json.NewDecoder(resp.Body).Decode(&plan)
	if pid, _ := plan["player_id"].(float64); int64(pid) != f.sub {
		t.Errorf("want player_id=%d (substitute), got %v", f.sub, plan["player_id"])
	}
	if isSub, _ := plan["is_sub"].(bool); !isSub {
		t.Error("want is_sub=true in response")
	}
}

func TestLineupSubstitute_Admin_Succeeds(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-admin", "admin")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (admin reaches handler), got %d", resp.StatusCode)
	}
}

func TestLineupSubstitute_SystemAdmin_Succeeds(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-system-admin", "system_admin")
	if err != nil {
		t.Fatalf("create system_admin user: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches handler), got %d", resp.StatusCode)
	}
}

// TestLineupSubstitute_PlayerAlreadyInMatch_Returns409 is a regression test
// for the staging finding: selecting a substitute who is already playing on
// the *other* team in the same match must be rejected, end to end over real
// HTTP, not just at the service layer.
func TestLineupSubstitute_PlayerAlreadyInMatch_Returns409(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)
	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-dup-match", "league_admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// f.sub already has team_id = f.teamID (the home team) from lineupSubSeed,
	// but is not yet in any lineup_plans row. Give them one on the *away*
	// side of the same match, then try to substitute them into the home slot.
	var awayTeamID int64
	if err := db.DB.QueryRow(`SELECT away_team_id FROM matches WHERE id=?`, f.matchID).Scan(&awayTeamID); err != nil {
		t.Fatalf("look up away team: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO lineup_plans (season_id, team_id, week_number, player_id, is_sub) VALUES (?,?,1,?,0)`,
		f.seasonID, awayTeamID, f.sub); err != nil {
		t.Fatalf("seed away-side lineup row for the would-be substitute: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (substitute already in this match), got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "that player is already in this match" {
		t.Errorf("want error message %q, got %v", "that player is already in this match", body["error"])
	}
}

// -- Lock enforcement ------------------------------------------------------

func TestLineupSubstitute_SeasonClosed_Returns409(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)
	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-lock-1", "league_admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE seasons SET closed_at = CURRENT_TIMESTAMP WHERE id=?`, f.seasonID); err != nil {
		t.Fatalf("close season: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (season closed), got %d", resp.StatusCode)
	}
}

func TestLineupSubstitute_WeekClosed_Returns409(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)
	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-lock-2", "league_admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE matches SET week_closed = 1 WHERE id=?`, f.matchID); err != nil {
		t.Fatalf("close week: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (week closed), got %d", resp.StatusCode)
	}
}

func TestLineupSubstitute_MatchApproved_Returns409(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)
	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-lock-3", "league_admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE matches SET approved_at = CURRENT_TIMESTAMP WHERE id=?`, f.matchID); err != nil {
		t.Fatalf("approve match: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (match approved), got %d", resp.StatusCode)
	}
}

func TestLineupSubstitute_MatchProcessed_Returns409(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)
	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-lock-4", "league_admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE matches SET approved_at = CURRENT_TIMESTAMP, processed_at = CURRENT_TIMESTAMP WHERE id=?`, f.matchID); err != nil {
		t.Fatalf("process match: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (match processed), got %d", resp.StatusCode)
	}
}

// -- Clear substitute --------------------------------------------------------

func TestLineupSubstitute_ClearAfterSet_RevertsToOriginal(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)
	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-clear-1", "league_admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	setReq, _ := http.NewRequest(http.MethodPost, lineupSubURL(srv.URL, f.planID),
		strings.NewReader(fmt.Sprintf(`{"substitute_player_id":%d}`, f.sub)))
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Header.Set("Authorization", "Bearer "+key)
	setResp, err := http.DefaultClient.Do(setReq)
	if err != nil {
		t.Fatalf("set request: %v", err)
	}
	setResp.Body.Close()
	if setResp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 on set, got %d", setResp.StatusCode)
	}

	clearReq, _ := http.NewRequest(http.MethodDelete, lineupSubURL(srv.URL, f.planID), nil)
	clearReq.Header.Set("Authorization", "Bearer "+key)
	clearResp, err := http.DefaultClient.Do(clearReq)
	if err != nil {
		t.Fatalf("clear request: %v", err)
	}
	defer clearResp.Body.Close()
	if clearResp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 on clear, got %d", clearResp.StatusCode)
	}
	var plan map[string]any
	json.NewDecoder(clearResp.Body).Decode(&plan)
	if pid, _ := plan["player_id"].(float64); int64(pid) != f.original {
		t.Errorf("want player_id=%d (reverted), got %v", f.original, plan["player_id"])
	}
	if isSub, _ := plan["is_sub"].(bool); isSub {
		t.Error("want is_sub=false after clearing")
	}
}

func TestLineupSubstitute_ClearWithoutSet_Returns400(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	f := lineupSubSeed(t)
	_, key, err := authStore.CreateApplyUser(context.Background(), "sub-clear-2", "league_admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	clearReq, _ := http.NewRequest(http.MethodDelete, lineupSubURL(srv.URL, f.planID), nil)
	clearReq.Header.Set("Authorization", "Bearer "+key)
	clearResp, err := http.DefaultClient.Do(clearReq)
	if err != nil {
		t.Fatalf("clear request: %v", err)
	}
	defer clearResp.Body.Close()
	if clearResp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 (slot was never substituted), got %d", clearResp.StatusCode)
	}
}
