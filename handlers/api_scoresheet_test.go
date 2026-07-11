package handlers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"league_app/db"
)

// --- Scoresheet round validation (HTTP level) ---

// seedRoundFixture creates a legacy (teams_managed=0) league/season/teams/players
// and inserts one match. The legacy season bypasses the RosterEligible roster check
// so the test can reach round validation directly.
// Returns (matchID, homePlayerID, awayPlayerID).
func seedRoundFixture(t *testing.T, srv *httptest.Server) (matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	pd := func(path, body string) map[string]any {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return m
	}

	lg := pd("/api/leagues", `{"name":"Round Test League","game_format":"8ball"}`)
	lgID := int64(lg["id"].(float64))

	tm1 := pd("/api/teams", fmt.Sprintf(`{"league_id":%d,"name":"Home Team"}`, lgID))
	tm2 := pd("/api/teams", fmt.Sprintf(`{"league_id":%d,"name":"Away Team"}`, lgID))
	homeTeamID := int64(tm1["id"].(float64))
	awayTeamID := int64(tm2["id"].(float64))

	p1 := pd("/api/players", `{"first_name":"Home","last_name":"Player","handicap":0}`)
	p2 := pd("/api/players", `{"first_name":"Away","last_name":"Player","handicap":0}`)
	homePlayerID = int64(p1["id"].(float64))
	awayPlayerID = int64(p2["id"].(float64))

	// Create a season and immediately downgrade to legacy (teams_managed=0).
	// POST /api/seasons always sets teams_managed=1; legacy mode bypasses RosterEligible.
	s := pd("/api/seasons", fmt.Sprintf(`{"league_id":%d,"name":"Test Season"}`, lgID))
	seasonID := int64(s["id"].(float64))
	if _, err := db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, seasonID); err != nil {
		t.Fatalf("downgrade season to legacy: %v", err)
	}

	res, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,1)`,
		seasonID, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	matchID, _ = res.LastInsertId()
	return
}

// TestSaveRounds_ValidationError_Returns422 confirms that submitting an impossible
// game score (both sides = 10) returns HTTP 422 with a structured validation.Result body.
func TestSaveRounds_ValidationError_Returns422(t *testing.T) {
	srv := testServer(t)
	matchID, homeP, awayP := seedRoundFixture(t, srv)

	body := fmt.Sprintf(`{"rounds":[{
		"round_number":1,
		"home_player_id":%d,"away_player_id":%d,
		"game1_home":10,"game1_away":10,
		"game2_home":0,"game2_away":0,
		"game3_home":0,"game3_away":0
	}]}`, homeP, awayP)

	resp, err := http.Post(
		fmt.Sprintf("%s/api/matches/%d/rounds", srv.URL, matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}

	var result struct {
		Messages []struct {
			Code  string `json:"code"`
			Level string `json:"level"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode validation result: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected validation messages in 422 response body")
	}
	found := false
	for _, m := range result.Messages {
		if m.Code == "SCORESHEET_GAME_BOTH_WINNERS" && m.Level == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SCORESHEET_GAME_BOTH_WINNERS error in response, got: %+v", result.Messages)
	}
}


// TestSaveRounds_SnapshotPreservedOnResave verifies that re-saving a scoresheet
// with the same players preserves the original HC snapshots even after a player's
// current handicap changes.
func TestSaveRounds_SnapshotPreservedOnResave(t *testing.T) {
	f := weekTestSeed(t)
	// Disable roster gate so saveRounds reaches the snapshot logic (teams_managed=1 by default via API).
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	body := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`, f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var origHomeHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1`, f.matchID).Scan(&origHomeHC)
	if !origHomeHC.Valid {
		t.Fatal("home_handicap_used should be set after first save")
	}

	// Change the player's current handicap.
	db.DB.Exec(`UPDATE players SET handicap=9.99 WHERE id=?`, f.playerA)

	// Re-save same round with same players.
	resp2, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	var resavedHomeHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1`, f.matchID).Scan(&resavedHomeHC)
	if !resavedHomeHC.Valid {
		t.Fatal("home_handicap_used should still be set after re-save")
	}
	if resavedHomeHC.Float64 != origHomeHC.Float64 {
		t.Errorf("snapshot should be preserved on re-save with same player: orig=%v, resaved=%v",
			origHomeHC.Float64, resavedHomeHC.Float64)
	}
}

// TestSaveRounds_SubstitutionPreservesUnchangedSide verifies that when a player
// is substituted on one side, the unchanged side's snapshot is preserved while
// the new player receives a fresh snapshot from their current players.handicap.
func TestSaveRounds_SubstitutionPreservesUnchangedSide(t *testing.T) {
	f := weekTestSeed(t)
	// Disable roster gate so saveRounds reaches the snapshot logic (teams_managed=1 by default via API).
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	rSub, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Sub','Player',?,1.5,1)`, f.teamB)
	subID, _ := rSub.LastInsertId()

	body1 := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`, f.playerA, f.playerB)
	resp, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID), "application/json", strings.NewReader(body1))
	resp.Body.Close()

	var origHomeHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1`, f.matchID).Scan(&origHomeHC)

	// Change home player handicap (should NOT affect preserved snapshot).
	db.DB.Exec(`UPDATE players SET handicap=9.99 WHERE id=?`, f.playerA)

	// Re-save: same home player, substitute on away side.
	body2 := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`, f.playerA, subID)
	resp2, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID), "application/json", strings.NewReader(body2))
	resp2.Body.Close()

	var newHomeHC, newAwayHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used, away_handicap_used FROM round_results WHERE match_id=? AND round_number=1`,
		f.matchID).Scan(&newHomeHC, &newAwayHC)

	if newHomeHC.Float64 != origHomeHC.Float64 {
		t.Errorf("home snapshot should be preserved (same player): orig=%v, now=%v",
			origHomeHC.Float64, newHomeHC.Float64)
	}
	if !newAwayHC.Valid || newAwayHC.Float64 != 1.5 {
		t.Errorf("away snapshot should be sub player's current hc (1.5): got valid=%v value=%v",
			newAwayHC.Valid, newAwayHC.Float64)
	}
}

// --- Phase 3E: Corrections (PM Corrections 1, 2, and 3) ----------------------

// TestSaveRounds_HomeSubstitutionPreservesAwaySnapshot is a regression test for
// PM Correction 2: when the home player is substituted (new home player C replaces A),
// the prior row is matched by away player identity, not by (round, home) key.
// B's stored away_handicap_used must be preserved even though A is no longer home.
func TestSaveRounds_HomeSubstitutionPreservesAwaySnapshot(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerB)

	// Initial save: A (home, HC=1.0) vs B (away, HC=2.0).
	body1 := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body1))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Add substitute home player C; change B's current HC so it differs from stored snapshot.
	rC, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Home','Sub',?,3.5,1)`, f.teamA)
	playerC, _ := rC.LastInsertId()
	db.DB.Exec(`UPDATE players SET handicap=5.0 WHERE id=?`, f.playerB)

	// Re-save: C (home, fresh HC=3.5) vs B (away, must preserve stored HC=2.0).
	body2 := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		playerC, f.playerB)
	resp2, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body2))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	var homeHCStored, awayHCStored float64
	db.DB.QueryRow(
		`SELECT home_handicap_used, away_handicap_used FROM round_results WHERE match_id=? AND round_number=1`,
		f.matchID).Scan(&homeHCStored, &awayHCStored)

	if awayHCStored != 2.0 {
		t.Errorf("away_handicap_used: want 2.0 (B's preserved snapshot), got %g (current HC used instead)", awayHCStored)
	}
	if homeHCStored != 3.5 {
		t.Errorf("home_handicap_used: want 3.5 (C's fresh HC), got %g", homeHCStored)
	}
}

// TestSaveRounds_AmbiguousSubstitutionReturns422 is a regression test for PM Correction 2
// (ambiguity rejection): when the home player is substituted and the unchanged away player
// appears in more than one prior row of the same round, the server must reject the save
// with 422 rather than silently replacing both snapshots with current values.
func TestSaveRounds_AmbiguousSubstitutionReturns422(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Second and third home players on teamA (C and D).
	rC, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home2','Sub',?,1.0)`, f.teamA)
	playerC, _ := rC.LastInsertId()
	rD, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home3','New',?,1.0)`, f.teamA)
	playerD, _ := rD.LastInsertId()

	// Insert two prior rows in round 1 with DIFFERENT home players but the SAME away player.
	// The schema UNIQUE constraint is (match_id, round_number, home_player_id) so this is allowed.
	// This creates the ambiguous state: playerA and playerC both faced playerB in round 1.
	db.DB.Exec(`INSERT INTO round_results
		(match_id, round_number, home_player_id, away_player_id,
		 game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		 home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO round_results
		(match_id, round_number, home_player_id, away_player_id,
		 game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		 home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		f.matchID, playerC, f.playerB)

	// Re-save: playerD (new home, not in any prior row) vs playerB (away).
	// priorByRound[1] has two prior rows with awayPlayerID=playerB -> awayCount=2 -> expect 422.
	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`,
		playerD, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("ambiguous substitution: want 422, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_SamePlayerMultiplePairingsDistinctSnapshots verifies that when
// the same player appears as home in multiple rounds with different prior snapshots,
// each round preserves its own snapshot independently (pairing-level, not player-level).
func TestSaveRounds_SamePlayerMultiplePairingsDistinctSnapshots(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	rY, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Away','R1',?,3.0,1)`, f.teamB)
	playerY, _ := rY.LastInsertId()
	rZ, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Away','R2',?,3.0,1)`, f.teamB)
	playerZ, _ := rZ.LastInsertId()

	// First save: playerA as home in round 1 only, HC=2.0; snapshot 2.0 stored.
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerA)
	body1 := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, playerY)
	resp, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body1))
	resp.Body.Close()

	// Second save: playerA in round 1 (prior snapshot 2.0 preserved) and round 2
	// (no prior row, so fresh HC=4.0 is stored). Both rounds present.
	db.DB.Exec(`UPDATE players SET handicap=4.0 WHERE id=?`, f.playerA)
	body2 := fmt.Sprintf(
		`{"rounds":[`+
			`{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5},`+
			`{"round_number":2,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}`+
			`]}`,
		f.playerA, playerY, f.playerA, playerZ)
	resp2, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body2))
	resp2.Body.Close()

	var r1HC, r2HC float64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r1HC)
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=2 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r2HC)
	if r1HC != 2.0 {
		t.Fatalf("after second save: round 1 home_handicap_used want 2.0 (preserved), got %g", r1HC)
	}
	if r2HC != 4.0 {
		t.Fatalf("after second save: round 2 home_handicap_used want 4.0 (fresh), got %g", r2HC)
	}

	// Third save: change A's HC to 1.0; re-save same body. Both rounds must preserve
	// their distinct snapshots (2.0 for round 1, 4.0 for round 2) via pairing-level matching.
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	resp3, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body2))
	resp3.Body.Close()

	var r1HCAfter, r2HCAfter float64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r1HCAfter)
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=2 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r2HCAfter)
	if r1HCAfter != 2.0 {
		t.Errorf("after re-save: round 1 home_handicap_used want 2.0, got %g", r1HCAfter)
	}
	if r2HCAfter != 4.0 {
		t.Errorf("after re-save: round 2 home_handicap_used want 4.0, got %g", r2HCAfter)
	}
}

// --- Phase 3E: Corrections (PM Correction 1 regression) ----------------------

// TestSaveRounds_EffectiveHCUsedForValidation is a regression test for PM Correction 1:
// saveRounds must resolve effective handicaps before ValidateRounds so that round
// winners and sets reflect the preserved snapshot, not the current players.handicap.
//
// A round winner requires 2 of 3 pairing wins, so we submit 2 pairings in round 1.
// Game scores are chosen so that home wins with preserved HC (spot to home) but
// loses when current HC is used instead (spot flips to away after the HC change).
func TestSaveRounds_EffectiveHCUsedForValidation(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Add a second home/away pair for round 1 so a round winner can be determined.
	rA2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Home','Two',?,1.0,1)`, f.teamA)
	playerA2, _ := rA2.LastInsertId()
	rB2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Away','Two',?,2.0,1)`, f.teamB)
	playerB2, _ := rB2.LastInsertId()

	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerB)

	// Games chosen so: with HC 1.0(home) vs 2.0(away), spot=3 to home.
	// adjH=25+3=28, adjA=24 -> home wins pairing.
	// After changing home HC to 3.0: spot=3 to away (away<home).
	// adjH=25, adjA=24+3=27 -> away wins pairing (ValidateRounds uses wrong HC without fix).
	body := fmt.Sprintf(
		`{"rounds":[`+
			`{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":7,"game2_home":10,"game2_away":7,"game3_home":5,"game3_away":10},`+
			`{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":7,"game2_home":10,"game2_away":7,"game3_home":5,"game3_away":10}`+
			`]}`,
		f.playerA, f.playerB, playerA2, playerB2)

	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// After first save: home wins 2 pairings -> RoundWinners[1]="home" -> sets_won=1.
	var swInit int
	db.DB.QueryRow(`SELECT sets_won FROM match_results WHERE match_id=? AND player_id=?`, f.matchID, f.playerA).Scan(&swInit)
	if swInit != 1 {
		t.Fatalf("initial save: sets_won want 1, got %d", swInit)
	}

	// Change both home players' HCs. Current HC (3.0 > 2.0) would flip spot to away,
	// making away win both pairings and giving RoundWinners[1]="away" (sets_won=0).
	db.DB.Exec(`UPDATE players SET handicap=3.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=3.0 WHERE id=?`, playerA2)

	resp2, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	// Effective HC must use preserved snapshot (1.0 vs 2.0): home wins -> sets_won=1.
	var swResaved int
	db.DB.QueryRow(`SELECT sets_won FROM match_results WHERE match_id=? AND player_id=?`, f.matchID, f.playerA).Scan(&swResaved)
	if swResaved != 1 {
		t.Errorf("re-save: sets_won want 1 (effective HC preserved, home wins), got %d (current HC used instead)", swResaved)
	}
}

// --- Phase 3E: Final Corrections (PM Correction - rule helpers and close-week query) ---------

// TestSaveRounds_InvalidMultiplierReturns500 is a regression test for the rule-helper
// error-return correction: when season_rules contains an unparseable handicap_multiplier,
// txSeasonRoundConfig must return an error and saveRounds must respond 500 rather than
// silently defaulting.
func TestSaveRounds_InvalidMultiplierReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', 'not-a-number')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("invalid multiplier: want 500, got %d", resp.StatusCode)
	}
}

// TestValidateWeek_RoundQueryFailureReturns500 verifies that when the underlying
// data fetch fails (round_results table dropped), ValidateWeek propagates the error
// as HTTP 500 rather than silently returning empty validation messages.
func TestValidateWeek_RoundQueryFailureReturns500(t *testing.T) {
	f := weekTestSeed(t)

	// Drop round_results so GetWeekValidationData fails for every match in the week.
	db.DB.Exec(`DROP TABLE round_results`)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/validate", f.srv.URL, f.sid, 1))
	if err != nil {
		t.Fatalf("validate request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 when round_results unavailable, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_NaNMultiplierReturns500 verifies that a stored NaN value for
// handicap_multiplier is rejected (strconv.ParseFloat accepts NaN without error
// but it is not a finite positive number) and saveRounds returns HTTP 500.
func TestSaveRounds_NaNMultiplierReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', 'NaN')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("NaN multiplier: want 500, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_InfMultiplierReturns500 verifies that a stored +Inf value for
// handicap_multiplier is rejected (strconv.ParseFloat accepts +Inf without error
// but it is not a finite positive number) and saveRounds returns HTTP 500.
func TestSaveRounds_InfMultiplierReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', '+Inf')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("+Inf multiplier: want 500, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_NegativeMinBallHCReturns500 verifies that a negative integer
// stored for min_ball_handicap is rejected and saveRounds returns HTTP 500.
// strconv.Atoi accepts "-1" without error; the explicit < 0 guard catches it.
func TestSaveRounds_NegativeMinBallHCReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'min_ball_handicap', 'MinBall', '-1')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("negative min_ball_handicap: want 500, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_InvalidConfigPreservesData verifies that a round save rejected
// by an invalid rule leaves existing round_results and match_results unchanged.
// Content is compared field-by-field so a delete-and-reinsert-with-different-values
// bug cannot hide behind a matching row count.
func TestSaveRounds_InvalidConfigPreservesData(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Seed an existing round_results row (snapshots deliberately NULL to verify NULL is preserved).
	db.DB.Exec(`
		INSERT INTO round_results
		  (match_id, round_number, home_player_id, away_player_id,
		   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away)
		VALUES (?,1,?,?,10,5,10,5,10,5)`,
		f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)

	// Capture round_results content before the rejected request.
	type rrSnapshot struct {
		homePlayerID, awayPlayerID    int64
		g1h, g1a, g2h, g2a, g3h, g3a int
		homeHC, awayHC                sql.NullFloat64
	}
	var rrBefore rrSnapshot
	db.DB.QueryRow(`
		SELECT home_player_id, away_player_id,
		       game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		       home_handicap_used, away_handicap_used
		FROM round_results WHERE match_id=?`, f.matchID).Scan(
		&rrBefore.homePlayerID, &rrBefore.awayPlayerID,
		&rrBefore.g1h, &rrBefore.g1a, &rrBefore.g2h, &rrBefore.g2a,
		&rrBefore.g3h, &rrBefore.g3a,
		&rrBefore.homeHC, &rrBefore.awayHC)

	// Capture match_results content before the rejected request.
	type mrSnapshot struct {
		playerID            int64
		gamesWon, gamesLost int
		diff                float64
		setsWon, setsLost   int
	}
	var mrBefore mrSnapshot
	db.DB.QueryRow(`
		SELECT player_id, games_won, games_lost, diff,
		       COALESCE(sets_won,0), COALESCE(sets_lost,0)
		FROM match_results WHERE match_id=?`, f.matchID).Scan(
		&mrBefore.playerID, &mrBefore.gamesWon, &mrBefore.gamesLost, &mrBefore.diff,
		&mrBefore.setsWon, &mrBefore.setsLost)

	// Store NaN as the multiplier to force config rejection before any writes.
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', 'NaN')`, f.sid)

	// Attempt a save with inverted scores -- must be rejected; existing data must survive.
	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":5,"game1_away":10,"game2_home":5,"game2_away":10,"game3_home":5,"game3_away":10}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 for invalid config, got %d", resp.StatusCode)
	}

	// Verify round_results row content is identical -- not merely the same count.
	var rrAfter rrSnapshot
	db.DB.QueryRow(`
		SELECT home_player_id, away_player_id,
		       game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		       home_handicap_used, away_handicap_used
		FROM round_results WHERE match_id=?`, f.matchID).Scan(
		&rrAfter.homePlayerID, &rrAfter.awayPlayerID,
		&rrAfter.g1h, &rrAfter.g1a, &rrAfter.g2h, &rrAfter.g2a,
		&rrAfter.g3h, &rrAfter.g3a,
		&rrAfter.homeHC, &rrAfter.awayHC)
	if rrAfter != rrBefore {
		t.Errorf("round_results row changed after rejected save:\n  before: %+v\n  after:  %+v", rrBefore, rrAfter)
	}

	// Verify match_results row content is identical.
	var mrAfter mrSnapshot
	db.DB.QueryRow(`
		SELECT player_id, games_won, games_lost, diff,
		       COALESCE(sets_won,0), COALESCE(sets_lost,0)
		FROM match_results WHERE match_id=?`, f.matchID).Scan(
		&mrAfter.playerID, &mrAfter.gamesWon, &mrAfter.gamesLost, &mrAfter.diff,
		&mrAfter.setsWon, &mrAfter.setsLost)
	if mrAfter != mrBefore {
		t.Errorf("match_results row changed after rejected save:\n  before: %+v\n  after:  %+v", mrBefore, mrAfter)
	}
}

// TestSaveRounds_ValidNonDefaultConfigSavesNormally verifies that explicitly stored
// valid non-default handicap_multiplier and min_ball_handicap values do not block
// a round save. min_ball_handicap=2 suppresses spots below 2 to 0 (threshold semantics);
// equal-handicap players produce spot=0, so no suppression occurs and the save proceeds.
func TestSaveRounds_ValidNonDefaultConfigSavesNormally(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', '3.0')`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'min_ball_handicap', 'MinBall', '2')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("valid non-default config: want 200, got %d: %s", resp.StatusCode, b)
	}
}

// --- Roster eligibility guard (RosterEligible boundary) ---

// TestSaveRounds_BlockedByRosterGate verifies that POST /api/matches/{id}/rounds
// returns 422 when the managed season's home team has fewer than 3 roster players.
func TestSaveRounds_BlockedByRosterGate(t *testing.T) {
	f := weekTestSeed(t)

	// Mark the season as managed so the roster gate applies.
	db.DB.Exec(`UPDATE seasons SET teams_managed=1 WHERE id=?`, f.sid)

	// Add team records but only 1 player on the home team roster (< 3 required).
	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team B')`, f.sid, f.teamB)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	// Away team has 3 players -- only home is short.
	rP2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('B2','P',?,3.0)`, f.teamB)
	rP3, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('B3','P',?,3.0)`, f.teamB)
	p2, _ := rP2.LastInsertId()
	p3, _ := rP3.LastInsertId()
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, f.playerB)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, p2)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, p3)

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
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("roster gate must return 422 when home team has < 3 players, got %d: %s", resp.StatusCode, b)
	}
}
