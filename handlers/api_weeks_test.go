package handlers_test

// Week workflow handler integration tests:
//   GET  /api/seasons/{id}/weeks              - TestListWeeks_*
//   GET  /api/seasons/{id}/weeks/{w}/validate - TestValidateWeek_*
//   POST /api/seasons/{id}/weeks/{w}/close    - TestCloseWeek_*
//   POST /api/seasons/{id}/weeks/{w}/reopen   - TestReopenWeek_*
//   GET  /api/seasons/{id}/weeks/{w}/advance-preview    - TestAdvancePreview_*
//   GET  /api/seasons/{id}/weeks/{w}/acknowledgments    - TestGetWeekAcknowledgments_*
//
// Shared infrastructure (weekFixture, weekTestSeed, seedRoundResult) lives in
// api_test.go because those helpers are also used by tests in other files.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

func TestListWeeks_ReturnsOpenWhenNoRows(t *testing.T) {
	f := weekTestSeed(t)
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var weeks []map[string]any
	json.NewDecoder(resp.Body).Decode(&weeks)
	if len(weeks) == 0 {
		t.Fatal("expected at least one week entry")
	}
	got, _ := weeks[0]["status"].(string)
	if got != "open" {
		t.Errorf("want status 'open' (inferred when no league_weeks row exists), got %q", got)
	}
}

func TestValidateWeek_ReportsErrorForUnscored(t *testing.T) {
	f := weekTestSeed(t)
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/validate", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("validate should return 200 with the result body, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_MATCH_NO_SCORES error in validation result, got: %v", result)
	}
}

func TestCloseWeek_Returns422WhenErrors(t *testing.T) {
	f := weekTestSeed(t)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unscored match must block close: want 422, got %d", resp.StatusCode)
	}
}

// TestCloseWeek_Returns422WhenCompletedButNoRoundResults proves that completed=1 alone
// is not sufficient to close a week -- round_results with a game winner are required.
func TestCloseWeek_Returns422WhenCompletedButNoRoundResults(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, f.matchID)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("completed=1 with no round_results must block close: want 422, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_MATCH_NO_SCORES error, got: %v", result)
	}
}

func TestCloseWeek_SucceedsWithSavedScores(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["closed"] != true {
		t.Errorf("want closed=true in response body, got %v", result)
	}
}

func TestCloseWeek_SetsLeagueWeeksStatus(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	var status string
	db.DB.QueryRow(`SELECT status FROM league_weeks WHERE season_id=? AND week_number=1`, f.sid).Scan(&status)
	if status != "closed" {
		t.Errorf("want league_weeks.status='closed', got %q", status)
	}
}

func TestCloseWeek_SetsMatchWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	var wc int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, f.matchID).Scan(&wc)
	if wc != 1 {
		t.Errorf("want matches.week_closed=1 after close, got %d", wc)
	}
}

func TestSaveRounds_BlockedWhenWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	// Disable roster gate so the week-closed check is reached (teams_managed=1 by default via API).
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	body := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`,
		f.playerA, f.playerB)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("saveRounds on closed week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestSubmitResults_BlockedWhenWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	body := fmt.Sprintf(`{"results":[{"player_id":%d,"team_id":%d,"games_won":3,"games_lost":0,"diff":3}]}`,
		f.playerA, f.teamA)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/results", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("submitResults on closed week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestClearResults_BlockedWhenWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/matches/%d/results", f.srv.URL, f.matchID),
		nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("clearResults on closed week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestStandings_ExcludeSavedButOpenMatch(t *testing.T) {
	f := weekTestSeed(t)
	// Insert round results and match results but do NOT close the week.
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA)

	resp, err := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var standings []map[string]any
	json.NewDecoder(resp.Body).Decode(&standings)
	for _, s := range standings {
		if played, _ := s["played"].(float64); played > 0 {
			t.Errorf("standings must not count open (unclosed) week: team %v shows %v played", s["team_name"], played)
		}
	}
}

func TestStandings_IncludeClosedMatch(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,0,3,-3)`, f.matchID, f.playerB, f.teamB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	standResp, err := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer standResp.Body.Close()
	var standings []map[string]any
	json.NewDecoder(standResp.Body).Decode(&standings)
	totalPlayed := 0
	for _, s := range standings {
		if p, _ := s["played"].(float64); p > 0 {
			totalPlayed++
		}
	}
	if totalPlayed == 0 {
		t.Error("standings must include match after week is closed")
	}
}

func TestPlayerStats_ExcludeOpenMatch(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA)

	// Do NOT close the week.
	resp, err := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var stats []map[string]any
	json.NewDecoder(resp.Body).Decode(&stats)
	for _, s := range stats {
		if gw, _ := s["games_won"].(float64); gw > 0 {
			t.Errorf("player stats must not count open (unclosed) week: %v shows %.0f games_won", s["player_name"], gw)
		}
	}
}

func TestPlayerStats_IncludeClosedMatch(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	if _, err := db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA); err != nil {
		t.Fatalf("insert match_results: %v", err)
	}

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, err := http.DefaultClient.Do(closeReq)
	if err != nil {
		t.Fatalf("close request: %v", err)
	}
	if closeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(closeResp.Body)
		t.Fatalf("close week failed: %d: %s", closeResp.StatusCode, body)
	}
	closeResp.Body.Close()

	resp, err := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var stats []map[string]any
	json.NewDecoder(resp.Body).Decode(&stats)
	totalGamesWon := 0
	for _, s := range stats {
		if gw, _ := s["games_won"].(float64); gw > 0 {
			totalGamesWon += int(gw)
		}
	}
	if totalGamesWon == 0 {
		t.Error("player stats must include match after week is closed")
	}
}

func TestSaveRounds_PopulatesSets(t *testing.T) {
	f := weekTestSeed(t)
	// Disable roster gate so saveRounds reaches sets logic (teams_managed=1 by default via API).
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Two home and two away players needed for a two-pairing round with a round winner.
	rH2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home2','P',?,3.0)`, f.teamA)
	playerA2, _ := rH2.LastInsertId()
	rA2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Away2','P',?,3.0)`, f.teamB)
	playerB2, _ := rA2.LastInsertId()

	// Round 1: home wins both pairings -> RoundWinners[1]="home" -> home players get sets_won=1.
	body := fmt.Sprintf(`{"rounds":[
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2},
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}
	]}`, f.playerA, f.playerB, playerA2, playerB2)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("saveRounds: want 200, got %d: %s", resp.StatusCode, b)
	}

	check := func(label string, playerID int64, wantSW, wantSL int) {
		t.Helper()
		var sw, sl int
		db.DB.QueryRow(`SELECT sets_won, sets_lost FROM match_results WHERE match_id=? AND player_id=?`,
			f.matchID, playerID).Scan(&sw, &sl)
		if sw != wantSW || sl != wantSL {
			t.Errorf("%s: want sets_won=%d sets_lost=%d, got %d/%d", label, wantSW, wantSL, sw, sl)
		}
	}
	check("playerA (home)", f.playerA, 1, 0)
	check("playerA2 (home)", playerA2, 1, 0)
	check("playerB (away)", f.playerB, 0, 1)
	check("playerB2 (away)", playerB2, 0, 1)
}

func TestPlayerStats_IncludeSetsAfterClose(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	rH2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home2','P',?,3.0)`, f.teamA)
	playerA2, _ := rH2.LastInsertId()
	rA2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Away2','P',?,3.0)`, f.teamB)
	playerB2, _ := rA2.LastInsertId()

	body := fmt.Sprintf(`{"rounds":[
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2},
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}
	]}`, f.playerA, f.playerB, playerA2, playerB2)

	saveReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	saveReq.Header.Set("Content-Type", "application/json")
	saveResp, _ := http.DefaultClient.Do(saveReq)
	saveResp.Body.Close()

	// Before close: sets must not appear in stats (week_closed gate).
	statsResp, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var beforeStats []map[string]any
	json.NewDecoder(statsResp.Body).Decode(&beforeStats)
	statsResp.Body.Close()
	for _, s := range beforeStats {
		if sw, _ := s["sets_won"].(float64); sw > 0 {
			t.Errorf("sets_won before close: want 0, got %.0f for %v", sw, s["player_name"])
		}
	}

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	// After close: at least one player must show sets_won > 0.
	statsResp2, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var afterStats []map[string]any
	json.NewDecoder(statsResp2.Body).Decode(&afterStats)
	statsResp2.Body.Close()
	found := false
	for _, s := range afterStats {
		if sw, _ := s["sets_won"].(float64); sw > 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sets_won > 0 after week close, got: %v", afterStats)
	}
}

func TestCloseWeek_ErrorOnDuplicatePlayer(t *testing.T) {
	f := weekTestSeed(t)

	// Seed a valid round result (round 1, playerA home, playerB away).
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	// Add a third player and insert them as the home player in a second pairing of round 1,
	// with playerA as the away player - playerA now appears twice in round 1.
	rPC, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Extra','P',?,3.0)`, f.teamA)
	playerC, _ := rPC.LastInsertId()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		f.matchID, playerC, f.playerA)
	if err != nil {
		t.Fatalf("insert duplicate round: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("duplicate player must block close: want 422, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_PLAYER_DUPLICATE" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_PLAYER_DUPLICATE error, got: %v", result)
	}
}

// Phase 2A: Warning acknowledgment ------------------------------------------------

// seedRoundResultWithIncompleteGame inserts round_results with one game winner (game1)
// and one incomplete game (game2: 5-3, no winner), triggering SCORESHEET_GAME_INCOMPLETE.
func seedRoundResultWithIncompleteGame(t *testing.T, matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,5,3,0,0,3.0,3.0,0,'')`,
		matchID, homePlayerID, awayPlayerID)
	if err != nil {
		t.Fatalf("seedRoundResultWithIncompleteGame: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
}

// seedRoundResultWithTwoWarnings inserts round_results with one game winner (game1)
// and two incomplete games (game2: 5-3, game3: 4-2), each triggering SCORESHEET_GAME_INCOMPLETE.
func seedRoundResultWithTwoWarnings(t *testing.T, matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,5,3,4,2,3.0,3.0,0,'')`,
		matchID, homePlayerID, awayPlayerID)
	if err != nil {
		t.Fatalf("seedRoundResultWithTwoWarnings: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
}

// weekValidate is a helper that calls GET /validate and decodes messages.
func weekValidate(t *testing.T, srvURL string, sid int64, weekNum int) []map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/validate", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("weekValidate: %v", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if msg, ok := m.(map[string]any); ok {
			out = append(out, msg)
		}
	}
	return out
}

// buildAcks constructs an acknowledgment slice from validate messages (warnings only).
func buildAcks(msgs []map[string]any) []map[string]any {
	var acks []map[string]any
	for _, msg := range msgs {
		if msg["level"] == "warning" {
			fieldStr, _ := msg["field"].(string)
			acks = append(acks, map[string]any{
				"match_id":     msg["match_id"],
				"warning_code": msg["code"],
				"field":        fieldStr,
				"notes":        "",
			})
		}
	}
	return acks
}

// weekClose POSTs to close the week with the given ack body.
func weekClose(t *testing.T, srvURL string, sid int64, weekNum int, acks []map[string]any) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"acknowledgments": acks})
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/%d/close", srvURL, sid, weekNum),
		strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("weekClose: %v", err)
	}
	return resp
}

func TestValidateWeek_StampsMatchIDOnErrors(t *testing.T) {
	f := weekTestSeed(t)
	// No round results -> WEEK_MATCH_NO_SCORES error
	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	for _, msg := range msgs {
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			mid, ok := msg["match_id"].(float64)
			if !ok || mid != float64(f.matchID) {
				t.Errorf("WEEK_MATCH_NO_SCORES: want match_id=%d, got: %v", f.matchID, msg["match_id"])
			}
			return
		}
	}
	t.Errorf("WEEK_MATCH_NO_SCORES not found in: %v", msgs)
}

func TestValidateWeek_StampsMatchIDOnScoresheetWarnings(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)
	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	for _, msg := range msgs {
		if msg["level"] == "warning" {
			mid, ok := msg["match_id"].(float64)
			if !ok || mid != float64(f.matchID) {
				t.Errorf("scoresheet warning: want match_id=%d, got: %v", f.matchID, msg["match_id"])
			}
			return
		}
	}
	t.Errorf("no warning found with match_id in: %v", msgs)
}

func TestCloseWeek_NoWarningsNilBodySucceeds(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid), nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("no warnings + nil body must succeed: want 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_MalformedBodyReturns400(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader(`{"acknowledgments": [`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("malformed close body must return 400, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_BlocksOnUnacknowledgedWarnings(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unacknowledged warnings must block close: want 422, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_SucceedsWithAllWarningsAcknowledged(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	if len(acks) == 0 {
		t.Fatal("fixture produced no warnings to acknowledge")
	}

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("all warnings acknowledged: want 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_BlocksOnPartialAcknowledgments(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithTwoWarnings(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	if len(acks) < 2 {
		t.Fatalf("need at least 2 warnings for partial ack test, got %d", len(acks))
	}

	// Acknowledge only the first warning
	resp := weekClose(t, f.srv.URL, f.sid, 1, acks[:1])
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("partial acknowledgments must block close: want 422, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_StoresAcknowledgmentRows(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	var acks []map[string]any
	for _, msg := range msgs {
		if msg["level"] == "warning" {
			fieldStr, _ := msg["field"].(string)
			acks = append(acks, map[string]any{
				"match_id":     msg["match_id"],
				"warning_code": msg["code"],
				"field":        fieldStr,
				"notes":        "stored note",
			})
		}
	}

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for acked warnings, got %d", resp.StatusCode)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&count)
	if count != len(acks) {
		t.Errorf("want %d ack row(s) stored, got %d", len(acks), count)
	}

	var seasonID int64
	var weekNumber int
	var matchID int64
	var warningCode string
	var field string
	var notes string
	db.DB.QueryRow(`
		SELECT season_id, week_number, match_id, warning_code, field, notes
		FROM week_close_acknowledgments
		WHERE season_id=? AND week_number=1`,
		f.sid).Scan(&seasonID, &weekNumber, &matchID, &warningCode, &field, &notes)
	if seasonID != f.sid {
		t.Errorf("want stored season_id=%d, got %d", f.sid, seasonID)
	}
	if weekNumber != 1 {
		t.Errorf("want stored week_number=1, got %d", weekNumber)
	}
	if matchID != f.matchID {
		t.Errorf("want stored match_id=%d, got %d", f.matchID, matchID)
	}
	if warningCode != "SCORESHEET_GAME_INCOMPLETE" {
		t.Errorf("want stored warning_code=SCORESHEET_GAME_INCOMPLETE, got %q", warningCode)
	}
	if field == "" {
		t.Error("want stored field to be non-empty")
	}
	if notes != "stored note" {
		t.Errorf("want notes='stored note', got %q", notes)
	}
}

func TestCloseWeek_ExtraStaleAckIgnoredWhenAllCurrentAcknowledged(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	// Prepend a stale ack for a warning that does not exist
	stale := map[string]any{
		"match_id":     float64(f.matchID),
		"warning_code": "SCORESHEET_NO_SCORES",
		"field":        "",
		"notes":        "stale",
	}
	acks = append([]map[string]any{stale}, acks...)

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("stale ack should not block close when current warnings acked: want 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_StaleAckAloneDoesNotAllowClose(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	// Only submit a stale ack for a non-existent warning
	staleOnlyAcks := []map[string]any{{
		"match_id":     float64(f.matchID),
		"warning_code": "SCORESHEET_NO_SCORES",
		"field":        "",
		"notes":        "stale",
	}}
	resp := weekClose(t, f.srv.URL, f.sid, 1, staleOnlyAcks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("stale ack alone must not allow close: want 422, got %d: %s", resp.StatusCode, body)
	}
}

func TestSaveRounds_ValidationMatchIDNil(t *testing.T) {
	f := weekTestSeed(t)
	// Submit impossible scores (both score 10) to trigger SCORESHEET_GAME_BOTH_WINNERS error
	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":10,"game2_home":0,"game2_away":0,"game3_home":0,"game3_away":0}]}`,
		f.playerA, f.playerB)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("invalid rounds must return 422, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	for _, m := range msgs {
		if msg, ok := m.(map[string]any); ok {
			if _, hasMatchID := msg["match_id"]; hasMatchID {
				t.Errorf("saveRounds validation messages must not include match_id, got: %v", msg)
			}
		}
	}
}

// Phase 2B: Reopen workflow ------------------------------------------------

// weekReopen POSTs to reopen a week. Returns the raw *http.Response (caller must close body).
func weekReopen(t *testing.T, srvURL string, sid int64, weekNum int) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/%d/reopen", srvURL, sid, weekNum), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("weekReopen: %v", err)
	}
	return resp
}

// seedMatchResult inserts one match_results row (1 set won, 3 games won) for the player.
func seedMatchResult(t *testing.T, matchID, playerID, teamID int64) {
	t.Helper()
	if _, err := db.DB.Exec(`
		INSERT OR IGNORE INTO match_results (match_id, player_id, team_id, sets_won, sets_lost, games_won, games_lost, diff)
		VALUES (?,?,?,1,0,3,0,3.0)`, matchID, playerID, teamID); err != nil {
		t.Fatalf("seedMatchResult: %v", err)
	}
}

func TestReopenWeek_ClosedWeekCanBeReopened(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	resp := weekReopen(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["reopened"] != true {
		t.Errorf("want reopened=true in response body, got %v", result)
	}
}

func TestReopenWeek_SetsLeagueWeeksStatusOpen(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var status string
	db.DB.QueryRow(`SELECT status FROM league_weeks WHERE season_id=? AND week_number=1`, f.sid).Scan(&status)
	if status != "open" {
		t.Errorf("want league_weeks.status='open' after reopen, got %q", status)
	}
}

func TestReopenWeek_ClearsClosedAt(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var closedAt *string
	db.DB.QueryRow(`SELECT closed_at FROM league_weeks WHERE season_id=? AND week_number=1`, f.sid).Scan(&closedAt)
	if closedAt != nil {
		t.Errorf("want league_weeks.closed_at=NULL after reopen, got %q", *closedAt)
	}
}

func TestReopenWeek_SetsMatchWeekClosed0(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var wc int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, f.matchID).Scan(&wc)
	if wc != 0 {
		t.Errorf("want matches.week_closed=0 after reopen, got %d", wc)
	}
}

func TestReopenWeek_PreservesRoundResults(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM round_results WHERE match_id=?`, f.matchID).Scan(&count)
	if count == 0 {
		t.Error("round_results must survive reopen")
	}
}

func TestReopenWeek_PreservesMatchResults(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	seedMatchResult(t, f.matchID, f.playerA, f.teamA)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM match_results WHERE match_id=?`, f.matchID).Scan(&count)
	if count == 0 {
		t.Error("match_results must survive reopen")
	}
}

func TestReopenWeek_StandingsExcludeReopenedWeek(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	// After close: match appears in standings (played >= 1).
	resp, _ := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	var standings []map[string]any
	json.NewDecoder(resp.Body).Decode(&standings)
	resp.Body.Close()
	closedPlayed := 0
	for _, s := range standings {
		if p, ok := s["played"].(float64); ok && int(p) > closedPlayed {
			closedPlayed = int(p)
		}
	}
	if closedPlayed == 0 {
		t.Fatal("expected standings to reflect closed match (played>0), but got 0")
	}

	// After reopen: match is excluded (played=0 for all teams).
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()
	resp2, _ := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	var standings2 []map[string]any
	json.NewDecoder(resp2.Body).Decode(&standings2)
	resp2.Body.Close()
	for _, s := range standings2 {
		if p, ok := s["played"].(float64); ok && p > 0 {
			t.Errorf("standings must exclude reopened week: team %v still shows played=%v", s["team_name"], p)
		}
	}
}

func TestReopenWeek_PlayerStatsExcludeReopenedWeek(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	seedMatchResult(t, f.matchID, f.playerA, f.teamA)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	// After close: player stats include games from the closed match.
	resp, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var stats []map[string]any
	json.NewDecoder(resp.Body).Decode(&stats)
	resp.Body.Close()
	maxGamesWon := 0
	for _, s := range stats {
		if gw, ok := s["games_won"].(float64); ok && int(gw) > maxGamesWon {
			maxGamesWon = int(gw)
		}
	}
	if maxGamesWon == 0 {
		t.Fatal("expected player stats to include closed match games, but got 0")
	}

	// After reopen: player stats exclude the week (games_won=0 for all players).
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()
	resp2, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var stats2 []map[string]any
	json.NewDecoder(resp2.Body).Decode(&stats2)
	resp2.Body.Close()
	for _, s := range stats2 {
		if gw, ok := s["games_won"].(float64); ok && gw > 0 {
			t.Errorf("player stats must exclude reopened week: player %v still shows games_won=%v", s["player_name"], gw)
		}
	}
}

func TestReopenWeek_OpenWeekReturns409(t *testing.T) {
	f := weekTestSeed(t)
	// Week 1 exists (match seeded) but has no league_weeks row: implicitly open.
	resp := weekReopen(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("reopening an open week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestReopenWeek_NoMatchesReturns404(t *testing.T) {
	f := weekTestSeed(t)
	// Week 99 has no matches.
	resp := weekReopen(t, f.srv.URL, f.sid, 99)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("reopening a week with no matches must return 404, got %d: %s", resp.StatusCode, b)
	}
}

func TestReopenWeek_ClosingAfterReopenWorks(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("re-close after reopen must succeed: want 200, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["closed"] != true {
		t.Errorf("want closed=true in re-close response, got %v", result)
	}
}

func TestReopenWeek_PreservesAcknowledgmentRows(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	weekClose(t, f.srv.URL, f.sid, 1, acks).Body.Close()

	var countBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&countBefore)
	if countBefore == 0 {
		t.Fatal("expected acknowledgment rows after close, got 0")
	}

	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var countAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&countAfter)
	if countAfter != countBefore {
		t.Errorf("acknowledgment rows must survive reopen: want %d, got %d", countBefore, countAfter)
	}
}

// Phase 2E: Acknowledgment history endpoint ----------------------------------

// weekGetAcks calls GET /api/seasons/{id}/weeks/{week}/acknowledgments.
func weekGetAcks(t *testing.T, srvURL string, sid int64, weekNum int) *http.Response {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/acknowledgments", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("weekGetAcks: %v", err)
	}
	return resp
}

func TestGetWeekAcknowledgments_NotFound(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetAcks(t, f.srv.URL, f.sid, 99)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("week with no matches must return 404, got %d: %s", resp.StatusCode, b)
	}
}

func TestGetWeekAcknowledgments_Empty(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetAcks(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var acks []map[string]any
	json.NewDecoder(resp.Body).Decode(&acks)
	if len(acks) != 0 {
		t.Errorf("want empty array before any close, got %d items", len(acks))
	}
}

func TestGetWeekAcknowledgments_AfterClose(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	weekClose(t, f.srv.URL, f.sid, 1, acks).Body.Close()

	resp := weekGetAcks(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 after close, got %d: %s", resp.StatusCode, b)
	}
	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) == 0 {
		t.Fatal("want at least one acknowledgment after close with warnings, got 0")
	}
	a := result[0]
	if code, _ := a["warning_code"].(string); code == "" {
		t.Errorf("want non-empty warning_code in ack row, got: %v", a)
	}
	if a["acknowledged_at"] == nil {
		t.Error("want acknowledged_at in ack row, got nil")
	}
}

func TestGetWeekAcknowledgments_PersistedAfterReopen(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	weekClose(t, f.srv.URL, f.sid, 1, acks).Body.Close()

	var countBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&countBefore)

	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	resp := weekGetAcks(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 after reopen, got %d: %s", resp.StatusCode, b)
	}
	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != countBefore {
		t.Errorf("acknowledgments must persist after reopen: want %d, got %d", countBefore, len(result))
	}
}

func TestListWeeks_IncludesAckCount(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	getWeeks := func() []map[string]any {
		resp, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks", f.srv.URL, f.sid))
		defer resp.Body.Close()
		var weeks []map[string]any
		json.NewDecoder(resp.Body).Decode(&weeks)
		return weeks
	}
	ackCountFor := func(weeks []map[string]any, wn float64) float64 {
		for _, ws := range weeks {
			if ws["week_number"] == wn {
				cnt, _ := ws["ack_count"].(float64)
				return cnt
			}
		}
		return -1
	}

	// Before close: ack_count must be 0.
	if cnt := ackCountFor(getWeeks(), 1); cnt != 0 {
		t.Errorf("ack_count before close: want 0, got %v", cnt)
	}

	// Close with acknowledged warnings.
	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	weekClose(t, f.srv.URL, f.sid, 1, buildAcks(msgs)).Body.Close()

	// After close: ack_count > 0.
	if cnt := ackCountFor(getWeeks(), 1); cnt == 0 {
		t.Error("ack_count after close: want > 0, got 0")
	}

	// After reopen: ack_count still > 0 (acks persist).
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()
	if cnt := ackCountFor(getWeeks(), 1); cnt == 0 {
		t.Error("ack_count after reopen: want > 0 (acks persist), got 0")
	}
}

func TestListWeeks_AckCountZeroForCleanClose(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	resp, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks", f.srv.URL, f.sid))
	defer resp.Body.Close()
	var weeks []map[string]any
	json.NewDecoder(resp.Body).Decode(&weeks)
	for _, ws := range weeks {
		if ws["week_number"] == float64(1) {
			cnt, _ := ws["ack_count"].(float64)
			if cnt != 0 {
				t.Errorf("ack_count for warning-free close: want 0, got %v", cnt)
			}
		}
	}
}

// --- Phase 3A: Advance Week Preview ------------------------------------------

// weekGetAdvancePreview calls GET /api/seasons/{id}/weeks/{week}/advance-preview.
func weekGetAdvancePreview(t *testing.T, srvURL string, sid int64, weekNum int) *http.Response {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/advance-preview", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("weekGetAdvancePreview: %v", err)
	}
	return resp
}

func weekGetRecap(t *testing.T, srvURL string, sid int64, weekNum int) *http.Response {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/recap", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("weekGetRecap: %v", err)
	}
	return resp
}

func TestAdvancePreview_NotFound(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 99)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("week with no matches must return 404, got %d: %s", resp.StatusCode, b)
	}
}

func TestAdvancePreview_WithValidationErrors(t *testing.T) {
	f := weekTestSeed(t)
	// No round results: WEEK_MATCH_NO_SCORES is expected.
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("advance preview must return 200 even with errors, got %d: %s", resp.StatusCode, b)
	}
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)
	canClose, _ := preview["can_close"].(bool)
	if canClose {
		t.Error("can_close must be false when validation errors exist")
	}
	msgs, _ := preview["validation_messages"].([]any)
	if len(msgs) == 0 {
		t.Error("validation_messages must be non-empty when errors exist")
	}
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_MATCH_NO_SCORES in validation_messages, got: %v", msgs)
	}
}

func TestAdvancePreview_ClosableWeek(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)
	canClose, _ := preview["can_close"].(bool)
	if !canClose {
		t.Error("can_close must be true when all validation checks pass")
	}
	msgs, _ := preview["validation_messages"].([]any)
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["level"] == "error" {
			t.Errorf("no error messages expected for closable week, got: %v", msg)
		}
	}
}

func TestAdvancePreview_NextWeekExists(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// Add a week 2 match.
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,2)`,
		f.sid, f.teamA, f.teamB)

	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	nextNum, ok := preview["next_week_number"].(float64)
	if !ok || int(nextNum) != 2 {
		t.Errorf("next_week_number: want 2, got %v", preview["next_week_number"])
	}
	nw, ok := preview["next_week"].(map[string]any)
	if !ok {
		t.Fatalf("next_week must be present when a next week exists")
	}
	if mc, _ := nw["match_count"].(float64); int(mc) != 1 {
		t.Errorf("next_week.match_count: want 1, got %v", mc)
	}
	if ac, _ := nw["assigned_count"].(float64); int(ac) != 1 {
		t.Errorf("next_week.assigned_count: want 1, got %v", ac)
	}
	if uc, _ := nw["unassigned_count"].(float64); int(uc) != 0 {
		t.Errorf("next_week.unassigned_count: want 0, got %v", uc)
	}
}

func TestAdvancePreview_NextWeekMissing(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// weekTestSeed only seeds week 1; no further weeks scheduled.
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	if _, ok := preview["next_week_number"]; ok {
		t.Error("next_week_number must be absent when no further weeks are scheduled")
	}
	if _, ok := preview["next_week"]; ok {
		t.Error("next_week must be absent when no further weeks are scheduled")
	}
}

func TestAdvancePreview_ReadOnly(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	var countBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM league_weeks WHERE season_id=?`, f.sid).Scan(&countBefore)

	weekGetAdvancePreview(t, f.srv.URL, f.sid, 1).Body.Close()
	weekGetAdvancePreview(t, f.srv.URL, f.sid, 1).Body.Close()

	var countAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM league_weeks WHERE season_id=?`, f.sid).Scan(&countAfter)
	if countAfter != countBefore {
		t.Errorf("advance-preview must not write to league_weeks: before=%d after=%d", countBefore, countAfter)
	}
}

// --- Phase 3B: Close Week advance_result in close response -------------------

func TestCloseWeek_ResponseIncludesAdvanceResult(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 on close, got %d: %s", resp.StatusCode, b)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	if v, _ := body["closed"].(bool); !v {
		t.Errorf("want closed=true, got %v", body["closed"])
	}
	if _, ok := body["advance_result"]; !ok {
		t.Error("close response must include advance_result")
	}
	if _, ok := body["acknowledgment_count"]; !ok {
		t.Error("close response must include acknowledgment_count")
	}
}

func TestCloseWeek_AdvanceResult_ClosedWeekStatus(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ar, _ := body["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("advance_result missing from close response")
	}
	cw, _ := ar["closed_week"].(map[string]any)
	if cw == nil {
		t.Fatal("advance_result.closed_week missing")
	}
	if status, _ := cw["status"].(string); status != "closed" {
		t.Errorf("closed_week.status: want closed, got %q", status)
	}
	if mc, _ := cw["match_count"].(float64); int(mc) != 1 {
		t.Errorf("closed_week.match_count: want 1, got %v", mc)
	}
}

func TestCloseWeek_AdvanceResult_NextWeekIncluded(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,2)`,
		f.sid, f.teamA, f.teamB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ar, _ := body["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("advance_result missing")
	}
	nextNum, ok := ar["next_week_number"].(float64)
	if !ok {
		t.Fatal("next_week_number must be present when week 2 exists")
	}
	if int(nextNum) != 2 {
		t.Errorf("next_week_number: want 2, got %v", nextNum)
	}
	nw, _ := ar["next_week"].(map[string]any)
	if nw == nil {
		t.Fatal("next_week must be present")
	}
	if mc, _ := nw["match_count"].(float64); int(mc) != 1 {
		t.Errorf("next_week.match_count: want 1, got %v", mc)
	}
}

func TestCloseWeek_AdvanceResult_NextWeekOmitted(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// weekTestSeed only seeds week 1; no week 2 exists.

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ar, _ := body["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("advance_result missing")
	}
	if _, ok := ar["next_week_number"]; ok {
		t.Error("next_week_number must be absent for final week")
	}
	if _, ok := ar["next_week"]; ok {
		t.Error("next_week must be absent for final week")
	}
}

func TestCloseWeek_AdvanceResult_AcknowledgmentCount(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	if len(acks) == 0 {
		t.Fatal("expected at least one warning to acknowledge")
	}

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ackCount, _ := body["acknowledgment_count"].(float64)
	if int(ackCount) != len(acks) {
		t.Errorf("acknowledgment_count: want %d, got %v", len(acks), ackCount)
	}
}

func TestCloseWeek_AdvanceResult_NoHandicapHistory(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	var beforeCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&beforeCount)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("want 200 on close")
	}

	var afterCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&afterCount)
	if afterCount != beforeCount {
		t.Errorf("close must not write handicap_history: before=%d after=%d", beforeCount, afterCount)
	}
}

func TestCloseWeek_AdvanceResult_NoLineupPlansMutated(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	var beforeCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=?`, f.sid).Scan(&beforeCount)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("want 200 on close")
	}

	var afterCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=?`, f.sid).Scan(&afterCount)
	if afterCount != beforeCount {
		t.Errorf("close must not mutate lineup_plans: before=%d after=%d", beforeCount, afterCount)
	}
}

func TestCloseWeek_AckCountIsCurrentCycleOnly(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	// First close: acknowledge all warnings.
	msgs1 := weekValidate(t, f.srv.URL, f.sid, 1)
	acks1 := buildAcks(msgs1)
	if len(acks1) == 0 {
		t.Fatal("expected at least one warning for first close")
	}
	weekClose(t, f.srv.URL, f.sid, 1, acks1).Body.Close()

	// Reopen so the week can be closed again.
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	// Second close: the same warning still exists; acknowledge it again.
	msgs2 := weekValidate(t, f.srv.URL, f.sid, 1)
	acks2 := buildAcks(msgs2)
	resp2 := weekClose(t, f.srv.URL, f.sid, 1, acks2)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("want 200 on re-close, got %d: %s", resp2.StatusCode, b)
	}
	var body map[string]any
	json.NewDecoder(resp2.Body).Decode(&body)

	ackCount, _ := body["acknowledgment_count"].(float64)
	// DB now holds rows from both close cycles, but acknowledgment_count must
	// reflect only the current cycle's warnings, not the cumulative historical total.
	if int(ackCount) != len(acks2) {
		t.Errorf("acknowledgment_count: want %d (current cycle only), got %v", len(acks2), ackCount)
	}
}

func TestAdvancePreview_ResponseIncludesCurrentWeekAndHandicap(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 from advance-preview after helper extraction, got %d: %s", resp.StatusCode, b)
	}
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	if v, _ := preview["can_close"].(bool); !v {
		t.Errorf("can_close must be true for closable week, got %v", preview["can_close"])
	}
	if _, ok := preview["current_week"]; !ok {
		t.Error("current_week must be present in advance-preview response")
	}
	if _, ok := preview["handicap"]; !ok {
		t.Error("handicap must be present in advance-preview response")
	}
}

// --- Recap Week ---

func TestRecapWeek_NotFoundWhenNoMatches(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetRecap(t, f.srv.URL, f.sid, 99)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("week with no matches must return 404, got %d: %s", resp.StatusCode, b)
	}
}

func TestRecapWeek_OpenStatusForUnclosedWeek(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetRecap(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var recap map[string]any
	json.NewDecoder(resp.Body).Decode(&recap)
	if recap["status"] != "open" {
		t.Errorf("want status=open for unclosed week, got %v", recap["status"])
	}
	matches, _ := recap["matches"].([]any)
	if len(matches) != 1 {
		t.Errorf("want 1 match in recap, got %d", len(matches))
	}
}

func TestRecapWeek_ClosedStatusAndMissingCount(t *testing.T) {
	f := weekTestSeed(t)
	// Close the week without entering results (match remains incomplete).
	// The seeded match has no round results, so it will block close via validation.
	// Instead, seed a valid round and close properly.
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	resp := weekGetRecap(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 after close, got %d: %s", resp.StatusCode, b)
	}
	var recap map[string]any
	json.NewDecoder(resp.Body).Decode(&recap)
	if recap["status"] != "closed" {
		t.Errorf("want status=closed after CloseWeek, got %v", recap["status"])
	}
	// The one match has a result (completed=1 after seedRoundResult).
	missing, _ := recap["missing_count"].(float64)
	if int(missing) != 0 {
		t.Errorf("want missing_count=0 when all matches have results, got %v", missing)
	}
	if _, ok := recap["acknowledgments"]; !ok {
		t.Error("acknowledgments must be present in recap response")
	}
	if _, ok := recap["handicap"]; !ok {
		t.Error("handicap must be present in recap response")
	}
}

