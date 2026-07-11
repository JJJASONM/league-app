package handlers_test

// Handicap handler integration tests covering:
//   GET  /api/seasons/{id}/weeks/{w}/advance-preview  (handicap preview slice)
//   GET  /api/seasons/{id}/handicap-recommendations   - TestHandicapRecs_*
//   GET  /api/seasons/{id}/handicap-review (season-level review)
//       - TestHandicapPreview_*
//       - TestHandicapReview_*
//       - TestCloseWeek_ReturnsHandicapRecommendations
//       - TestPreviewAdvance_HandicapRecommendations
//       - TestAdvancePreview_DoesNotMutateHandicapTables
//       - TestCloseWeek_AdvanceResultHandicapShape
//
// Shared infrastructure (weekTestSeed, seedRoundResult, weekClose, weekFixture)
// lives in api_test.go because those helpers are used by tests in other files.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/db"
)

// --- Phase 3C: Handicap Recommendation Preview ---

// setHandicapMethod sets the handicap_update_method season rule.
func setHandicapMethod(t *testing.T, sid int64, method string) {
	t.Helper()
	if _, err := db.DB.Exec(`
		INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_update_method', 'Handicap Update Method', ?)`, sid, method); err != nil {
		t.Fatalf("setHandicapMethod: %v", err)
	}
}

// getHandicapPreviewHC calls GET /advance-preview and returns the decoded handicap section.
func getHandicapPreviewHC(t *testing.T, srvURL string, sid, weekNum int64) map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/advance-preview", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("getHandicapPreviewHC: %v", err)
	}
	defer resp.Body.Close()
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)
	hc, _ := preview["handicap"].(map[string]any)
	return hc
}

// closeWeek1ForHC closes week 1 by setting week_closed and league_weeks directly (no API).
// Used in handicap preview tests where the API path is not under test.
func closeWeek1ForHC(t *testing.T, sid, matchID int64) {
	t.Helper()
	if _, err := db.DB.Exec(`UPDATE matches SET week_closed=1 WHERE id=?`, matchID); err != nil {
		t.Fatalf("closeWeek1ForHC update matches: %v", err)
	}
	if _, err := db.DB.Exec(`
		INSERT OR IGNORE INTO league_weeks (season_id, week_number, status, closed_at)
		VALUES (?, 1, 'closed', CURRENT_TIMESTAMP)`, sid); err != nil {
		t.Fatalf("closeWeek1ForHC insert league_weeks: %v", err)
	}
}

func TestHandicapPreview_ManualReview(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// No handicap rule set -> defaults to manual_review.

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if method, _ := hc["method"].(string); method != "manual_review" {
		t.Errorf("want method=manual_review, got %q", method)
	}
	if status, _ := hc["status"].(string); status != "no_auto_apply" {
		t.Errorf("want status=no_auto_apply, got %q", status)
	}
	if _, ok := hc["recommendations"]; ok {
		t.Error("manual_review must not include recommendations field")
	}
}

func TestHandicapPreview_GameDiffAverage_TwoPlayers(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerB)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// playerA wins 3 games, diff=3.0; playerB loses 3 games, diff=-3.0.
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,0,3,-3.0)`, f.matchID, f.playerB, f.teamB)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if method, _ := hc["method"].(string); method != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", method)
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Fatal("want recommendations, got none")
	}
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		pid := int64(rec["player_id"].(float64))
		recHC, _ := rec["recommended_handicap"].(float64)
		skipped, _ := rec["skipped"].(bool)
		if pid == f.playerA {
			if recHC != 3.0 {
				t.Errorf("playerA: want recommended_handicap=3.0, got %v", recHC)
			}
			if skipped {
				t.Error("playerA: want skipped=false")
			}
		}
		if pid == f.playerB {
			if recHC != -3.0 {
				t.Errorf("playerB: want recommended_handicap=-3.0, got %v", recHC)
			}
		}
	}
}

func TestHandicapPreview_OpenWeeksExcluded(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=0.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// Add playerA to season_rosters so they appear as a candidate even with no closed data.
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id) VALUES (?,?)`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	// Insert match_results but leave week_closed=0 (open week).
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,5.0)`, f.matchID, f.playerA, f.teamA)
	// Do NOT close the week.
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			mp, _ := rec["matches_played"].(float64)
			if int(mp) != 0 {
				t.Errorf("open week must not count: want matches_played=0, got %v", mp)
			}
			if rec["skipped"] != true {
				t.Errorf("player with no closed data must be skipped")
			}
			return
		}
	}
	// playerA not found in recs is also acceptable: open weeks produce no candidates
	// unless season_rosters exists. Either outcome proves open weeks are excluded.
}

func TestHandicapPreview_NoMatchData(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// playerA is on season_rosters but has no closed match_results.
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id) VALUES (?,?)`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if rec["skipped"] != true {
				t.Errorf("player with no closed data: want skipped=true")
			}
			reason, _ := rec["reason"].(string)
			if reason != "no_data" {
				t.Errorf("want reason=no_data, got %q", reason)
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_AdminHold(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET admin_hold=1 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if rec["skipped"] != true {
				t.Errorf("admin_hold player: want skipped=true")
			}
			reason, _ := rec["reason"].(string)
			if reason != "admin_hold" {
				t.Errorf("want reason=admin_hold, got %q", reason)
			}
			if ah, _ := rec["admin_hold"].(bool); !ah {
				t.Errorf("want admin_hold=true in response")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_NoChange(t *testing.T) {
	f := weekTestSeed(t)
	// playerA: current=2.0, 1 match diff=2.0 -> recommended=2.0 -> no_change.
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,2.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			reason, _ := rec["reason"].(string)
			if reason != "no_change" {
				t.Errorf("want reason=no_change when current==recommended, got %q", reason)
			}
			if rec["skipped"] == true {
				t.Error("no_change player must not be skipped")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_MaxIndividualCapApplied(t *testing.T) {
	f := weekTestSeed(t)
	// Set max_individual_handicap=3.0; playerA diff=5.0 -> recommended capped to 3.0.
	db.DB.Exec(`INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		f.sid, "max_individual_handicap", "Max Individual Handicap", "3.0")
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,5.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			reason, _ := rec["reason"].(string)
			if reason != "capped" {
				t.Errorf("want reason=capped when avg exceeds max, got %q", reason)
			}
			recHC, _ := rec["recommended_handicap"].(float64)
			if recHC != 3.0 {
				t.Errorf("want recommended_handicap=3.0 (capped from 5.0), got %v", recHC)
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_KickerUnsupported(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	setHandicapMethod(t, f.sid, "kicker_average_preview")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if status, _ := hc["status"].(string); status != "unsupported" {
		t.Errorf("kicker_average_preview: want status=unsupported, got %q", status)
	}
	if _, ok := hc["recommendations"]; ok {
		t.Error("kicker_average_preview must not include recommendations field")
	}
	if msg, _ := hc["message"].(string); msg == "" {
		t.Error("kicker_average_preview: want non-empty message")
	}
}

func TestCloseWeek_ReturnsHandicapRecommendations(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	ar, _ := result["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("want advance_result in close response")
	}
	hc, _ := ar["handicap"].(map[string]any)
	if hc == nil {
		t.Fatal("want advance_result.handicap in close response")
	}
	if method, _ := hc["method"].(string); method != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", method)
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Error("want non-empty recommendations in close response advance_result.handicap")
	}
}

func TestPreviewAdvance_HandicapRecommendations(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if method, _ := hc["method"].(string); method != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", method)
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Error("want non-empty recommendations in advance-preview response")
	}
}

// TestHandicapPreview_DBError_Returns500 verifies that a real DB failure in the
// recommendation helper path surfaces as HTTP 500 rather than silently returning
// an empty recommendation set.
func TestHandicapPreview_DBError_Returns500(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	setHandicapMethod(t, f.sid, "game_diff_average")

	// Drop season_rosters so the candidate UNION query in computeGameDiffAverageRecs
	// fails with a real SQL error. This DB is isolated to this test's temp dir.
	if _, err := db.DB.Exec(`DROP TABLE season_rosters`); err != nil {
		t.Fatalf("DROP TABLE season_rosters: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/advance-preview", f.srv.URL, f.sid))
	if err != nil {
		t.Fatalf("GET advance-preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("want 500 on DB error, got %d: %s", resp.StatusCode, body)
	}
}

// --- Phase 3D: Handicap Review Screen ---

// getHandicapRecs calls GET /api/seasons/{id}/handicap-recommendations and
// returns the decoded response body as a map.
func getHandicapRecs(t *testing.T, srvURL string, sid int64) (map[string]any, int) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", srvURL, sid))
	if err != nil {
		t.Fatalf("getHandicapRecs: %v", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, resp.StatusCode
}

func TestHandicapRecs_GameDiffAverage(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team B')`, f.sid, f.teamB)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, f.playerB)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")
	// Threshold=3 so playerA's 3 included racks meet the minimum.
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "3")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	if m, _ := data["method"].(string); m != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", m)
	}
	if s, _ := data["status"].(string); s != "preview" {
		t.Errorf("want status=preview, got %q", s)
	}
	wc, _ := data["weeks_closed"].(float64)
	if int(wc) != 1 {
		t.Errorf("want weeks_closed=1, got %v", wc)
	}
	recs, _ := data["recommendations"].([]any)
	if len(recs) == 0 {
		t.Fatal("want recommendations, got none")
	}
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		pid := int64(rec["player_id"].(float64))
		if pid == f.playerA {
			if rec["recommended_hc"] == nil {
				t.Error("playerA: want non-nil recommended_hc")
			}
			if rec["change_amount"] == nil {
				t.Error("playerA: want non-nil change_amount")
			}
			if _, ok := rec["team_name"]; !ok {
				t.Error("want team_name field in recommendation")
			}
		}
	}
}

func TestHandicapRecs_AdminHold(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET admin_hold=1 WHERE id=?`, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team B')`, f.sid, f.teamB)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, f.playerB)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	recs, _ := data["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if reason, _ := rec["reason"].(string); reason != "admin_hold" {
				t.Errorf("want reason=admin_hold, got %q", reason)
			}
			if ah, _ := rec["admin_hold"].(bool); !ah {
				t.Error("want admin_hold=true in response")
			}
			if rec["recommended_hc"] != nil {
				t.Error("admin_hold player must have nil recommended_hc")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations")
}

func TestHandicapRecs_NoData(t *testing.T) {
	f := weekTestSeed(t)
	// playerA is rostered but has no round_results at all => included_racks=0 => no_data.
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	recs, _ := data["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if reason, _ := rec["reason"].(string); reason != "no_data" {
				t.Errorf("want reason=no_data, got %q", reason)
			}
			if rec["recommended_hc"] != nil {
				t.Error("no_data player must have nil recommended_hc")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations")
}

func TestHandicapRecs_ManualReview(t *testing.T) {
	f := weekTestSeed(t)
	// Default method is manual_review (no rule set).

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	if m, _ := data["method"].(string); m != "manual_review" {
		t.Errorf("want method=manual_review, got %q", m)
	}
	if s, _ := data["status"].(string); s != "no_auto_apply" {
		t.Errorf("want status=no_auto_apply, got %q", s)
	}
	recs, _ := data["recommendations"].([]any)
	if len(recs) != 0 {
		t.Errorf("manual_review: want empty recommendations, got %d", len(recs))
	}
	if msg, _ := data["message"].(string); msg == "" {
		t.Error("want non-empty message for manual_review")
	}
}

func TestHandicapRecs_NoClosedWeeks(t *testing.T) {
	f := weekTestSeed(t)
	// Scores saved and completed=1, but week NOT closed (week_closed=0).
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,5.0)`, f.matchID, f.playerA, f.teamA)
	// Do NOT call closeWeek1ForHC -- week remains open.
	setHandicapMethod(t, f.sid, "game_diff_average")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	if s, _ := data["status"].(string); s != "no_data" {
		t.Errorf("want status=no_data when no closed weeks, got %q", s)
	}
	recs, _ := data["recommendations"].([]any)
	if len(recs) != 0 {
		t.Errorf("no closed weeks: want empty recommendations, got %d", len(recs))
	}
}

func TestHandicapRecs_SeasonNotFound(t *testing.T) {
	f := weekTestSeed(t)
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/9999/handicap-recommendations", f.srv.URL))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandicapRecs_DBError(t *testing.T) {
	f := weekTestSeed(t)
	setHandicapMethod(t, f.sid, "game_diff_average")
	// Insert a closed week so we get past the weeksClosed==0 gate.
	closeWeek1ForHC(t, f.sid, f.matchID)

	// Drop season_rosters so computeHandicapReviewRecs fails with a real SQL error.
	if _, err := db.DB.Exec(`DROP TABLE season_rosters`); err != nil {
		t.Fatalf("DROP TABLE season_rosters: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", f.srv.URL, f.sid))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("want 500 on DB error, got %d: %s", resp.StatusCode, body)
	}
}

func TestHandicapRecs_ReadOnly(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,4.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	var hcBefore float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcBefore)
	var histBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&histBefore)

	http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", f.srv.URL, f.sid))

	var hcAfter float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcAfter)
	if hcAfter != hcBefore {
		t.Errorf("handicap-recommendations must not modify players.handicap: was %v, now %v", hcBefore, hcAfter)
	}
	var histAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&histAfter)
	if histAfter != histBefore {
		t.Errorf("handicap-recommendations must not write handicap_history: was %d, now %d", histBefore, histAfter)
	}
}

func TestAdvancePreview_DoesNotMutateHandicapTables(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	// Snapshot state before any Phase 3C operation.
	var hcBefore float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcBefore)
	var hcHistBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&hcHistBefore)

	// Trigger buildHandicapPreview via close (writes week_closed=1, then calls buildAdvanceResult).
	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	resp.Body.Close()

	// Also call advance-preview directly.
	http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/advance-preview", f.srv.URL, f.sid))

	var hcAfter float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcAfter)
	if hcAfter != hcBefore {
		t.Errorf("advance-preview must not modify players.handicap: was %v, now %v", hcBefore, hcAfter)
	}

	var hcHistAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&hcHistAfter)
	if hcHistAfter != hcHistBefore {
		t.Errorf("advance-preview must not write handicap_history: was %d, now %d", hcHistBefore, hcHistAfter)
	}
}

// --- Phase 3E: Handicap Review (opponent-normalized) ----------------------------

// hrFixture is a running server with a seeded 8-ball league/season and two
// teams registered in season_teams. Per-test helpers add players and rack data.
type hrFixture struct {
	srv      *httptest.Server
	sid      int64
	leagueID int64
	teamA    int64
	teamB    int64
}

// hrTestSeed spins up a fresh test server, creates an 8-ball league and season,
// two teams, and registers both teams in season_teams.
func hrTestSeed(t *testing.T) hrFixture {
	t.Helper()
	srv := testServer(t)

	resp, err := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"HR League","game_format":"8ball"}`))
	if err != nil {
		t.Fatalf("hrTestSeed: POST leagues: %v", err)
	}
	var lg map[string]any
	json.NewDecoder(resp.Body).Decode(&lg)
	resp.Body.Close()
	leagueID := int64(lg["id"].(float64))

	resp2, err := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"HR Season"}`, leagueID)))
	if err != nil {
		t.Fatalf("hrTestSeed: POST seasons: %v", err)
	}
	var ss map[string]any
	json.NewDecoder(resp2.Body).Decode(&ss)
	resp2.Body.Close()
	sid := int64(ss["id"].(float64))

	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team B')`, leagueID)
	teamA, _ := rA.LastInsertId()
	teamB, _ := rB.LastInsertId()

	if _, err := db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, sid, teamA); err != nil {
		t.Fatalf("hrTestSeed: season_teams A: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team B')`, sid, teamB); err != nil {
		t.Fatalf("hrTestSeed: season_teams B: %v", err)
	}
	return hrFixture{srv: srv, sid: sid, leagueID: leagueID, teamA: teamA, teamB: teamB}
}

// hrAddPlayer inserts a player and registers them in season_rosters.
func hrAddPlayer(t *testing.T, f hrFixture, teamID int64, hc float64, adminHold bool) int64 {
	t.Helper()
	ah := 0
	if adminHold {
		ah = 1
	}
	r, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, player_number, handicap, admin_hold, active) VALUES ('Test','Player','00',?,?,1)`, hc, ah)
	if err != nil {
		t.Fatalf("hrAddPlayer: %v", err)
	}
	pid, _ := r.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, teamID, pid); err != nil {
		t.Fatalf("hrAddPlayer: season_rosters: %v", err)
	}
	return pid
}

// hrInsertMatch inserts a completed, week_closed match and returns its ID.
func hrInsertMatch(t *testing.T, f hrFixture, homeTeamID, awayTeamID int64) int64 {
	t.Helper()
	r, err := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-01-01',1,1,1)`,
		f.sid, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("hrInsertMatch: %v", err)
	}
	id, _ := r.LastInsertId()
	return id
}

// hrInsertRound inserts one round_results row. homeHC / awayHC may be nil (NULL snapshot).
func hrInsertRound(t *testing.T, matchID, roundNum, homeID, awayID int64,
	g1h, g1a, g2h, g2a, g3h, g3a int, homeHC, awayHC *float64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		  (match_id, round_number, home_player_id, away_player_id,
		   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		   home_handicap_used, away_handicap_used)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		matchID, roundNum, homeID, awayID,
		g1h, g1a, g2h, g2a, g3h, g3a,
		homeHC, awayHC)
	if err != nil {
		t.Fatalf("hrInsertRound: %v", err)
	}
}

func hrGetRecs(t *testing.T, srv *httptest.Server, sid int64) []map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", srv.URL, sid))
	if err != nil {
		t.Fatalf("hrGetRecs: GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("hrGetRecs: want 200, got %d: %s", resp.StatusCode, body)
	}
	var data map[string]any
	json.NewDecoder(resp.Body).Decode(&data)
	recs, _ := data["recommendations"].([]any)
	out := make([]map[string]any, len(recs))
	for i, r := range recs {
		out[i], _ = r.(map[string]any)
	}
	return out
}

func hrSetRule(t *testing.T, sid int64, key, value string) {
	t.Helper()
	_, err := db.DB.Exec(
		`INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		sid, key, key, value)
	if err != nil {
		t.Fatalf("hrSetRule: %v", err)
	}
}

func ptr64(v float64) *float64 { return &v }

// TestHandicapReview_HomePlayerUsesAwaySnapshot verifies that when the reviewed
// player was HOME, the opponent HC baseline is away_handicap_used.
func TestHandicapReview_HomePlayerUsesAwaySnapshot(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	// home wins all 3 games 10 vs 5; away_handicap_used=3.0, home_handicap_used=2.0.
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5,
		ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("home player not found in recommendations")
	}

	windowHC, ok := homeRec["window_hc"].(float64)
	if !ok {
		t.Fatalf("home player window_hc is nil or not float64: %v", homeRec["window_hc"])
	}
	// per_rack = 3.0 + (10-5)/0.85; 3 racks => avg rounds to nearest 0.01
	wantApprox := 3.0 + 5.0/0.85
	want := math.Round(wantApprox*100) / 100
	if math.Abs(windowHC-want) > 0.005 {
		t.Errorf("home player window_hc: want ~%v (away_hc=3.0 baseline), got %v", want, windowHC)
	}
}

// TestHandicapReview_AwayPlayerUsesHomeSnapshot verifies that when the reviewed
// player was AWAY, the opponent HC baseline is home_handicap_used.
func TestHandicapReview_AwayPlayerUsesHomeSnapshot(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	// away wins all 3 games 10 vs 5; opponent for away player = home_handicap_used=2.0.
	hrInsertRound(t, matchID, 1, homeID, awayID,
		5, 10, 5, 10, 5, 10,
		ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var awayRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == awayID {
			awayRec = r
		}
	}
	if awayRec == nil {
		t.Fatal("away player not found in recommendations")
	}

	windowHC, ok := awayRec["window_hc"].(float64)
	if !ok {
		t.Fatalf("away player window_hc is nil: %v", awayRec["window_hc"])
	}
	// per_rack = 2.0 + (10-5)/0.85
	wantApprox := 2.0 + 5.0/0.85
	want := math.Round(wantApprox*100) / 100
	if math.Abs(windowHC-want) > 0.005 {
		t.Errorf("away player window_hc: want ~%v (home_hc=2.0 baseline), got %v", want, windowHC)
	}
}

// TestHandicapReview_NullSnapshotExcluded verifies that a rack with NULL
// away_handicap_used is excluded from calculation and counted in missing_snapshot_racks.
func TestHandicapReview_NullSnapshotExcluded(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5,
		ptr64(2.0), nil) // NULL away snapshot

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("home player not found in recommendations")
	}

	if int(homeRec["score_eligible_racks"].(float64)) == 0 {
		t.Error("score_eligible_racks should be > 0")
	}
	if int(homeRec["missing_snapshot_racks"].(float64)) == 0 {
		t.Error("missing_snapshot_racks should be > 0")
	}
	if int(homeRec["included_racks"].(float64)) != 0 {
		t.Errorf("included_racks should be 0, got %v", homeRec["included_racks"])
	}
	if homeRec["reason"] != "no_data" {
		t.Errorf("reason: want no_data, got %v", homeRec["reason"])
	}
}

// TestHandicapReview_ExcludedRacksNotCountedTowardThreshold verifies that
// NULL-snapshot racks do not count toward the eligibility threshold.
func TestHandicapReview_ExcludedRacksNotCountedTowardThreshold(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "10")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// 2 matches with NULL snapshots (6 slots excluded) + 1 match with valid snapshots (3 slots).
	for i := 0; i < 2; i++ {
		mid := hrInsertMatch(t, f, f.teamA, f.teamB)
		hrInsertRound(t, mid, 1, homeID, awayID,
			10, 5, 10, 5, 10, 5, ptr64(2.0), nil)
	}
	midValid := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, midValid, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("home player not found")
	}

	included := int(homeRec["included_racks"].(float64))
	if included >= 10 {
		t.Errorf("included_racks should be < 10 (NULL-snapshot racks excluded), got %d", included)
	}
	if homeRec["reason"] != "below_threshold" {
		t.Errorf("reason: want below_threshold, got %v", homeRec["reason"])
	}
	if homeRec["recommended_hc"] != nil {
		t.Errorf("recommended_hc must be nil for below_threshold player, got %v", homeRec["recommended_hc"])
	}
}

// TestHandicapReview_AdminHoldShowsCalculationsNoRecommendation verifies that
// an admin hold player with included racks still has lifetime_hc and window_hc
// populated, but recommended_hc and change_amount are nil.
func TestHandicapReview_AdminHoldShowsCalculationsNoRecommendation(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, true) // admin_hold=true
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("admin hold player not found in recommendations")
	}

	if homeRec["reason"] != "admin_hold" {
		t.Errorf("reason: want admin_hold, got %v", homeRec["reason"])
	}
	if homeRec["lifetime_hc"] == nil {
		t.Error("lifetime_hc should be non-nil for admin hold player with included racks")
	}
	if homeRec["window_hc"] == nil {
		t.Error("window_hc should be non-nil for admin hold player with included racks")
	}
	if homeRec["recommended_hc"] != nil {
		t.Errorf("recommended_hc must be nil for admin hold player, got %v", homeRec["recommended_hc"])
	}
	if homeRec["change_amount"] != nil {
		t.Errorf("change_amount must be nil for admin hold player, got %v", homeRec["change_amount"])
	}
}

// TestHandicapReview_BelowThresholdShowsProvisionalNoRecommendation verifies
// that a below-threshold player has calculated values but no recommendation.
func TestHandicapReview_BelowThresholdShowsProvisionalNoRecommendation(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "5")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	// 3 included racks; threshold=5 => below_threshold.
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("player not found in recommendations")
	}

	if homeRec["reason"] != "below_threshold" {
		t.Errorf("reason: want below_threshold, got %v", homeRec["reason"])
	}
	if homeRec["lifetime_hc"] == nil {
		t.Error("lifetime_hc should be non-nil for player with included racks")
	}
	if homeRec["window_hc"] == nil {
		t.Error("window_hc should be non-nil for player with included racks")
	}
	if homeRec["recommended_hc"] != nil {
		t.Errorf("recommended_hc must be nil for below_threshold player, got %v", homeRec["recommended_hc"])
	}
}

// TestHandicapReview_InvalidRuleReturns500 verifies that a stored rule value of
// "0" (invalid -- below minimum 1) causes the endpoint to return HTTP 500.
func TestHandicapReview_InvalidRuleReturns500(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")

	// Seed one closed week so the handler does not early-return with no_data.
	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)
	mid := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	// "0" is below minimum of 1 -- invalid.
	hrSetRule(t, f.sid, "handicap_current_game_window", "0")

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 for invalid rule value, got %d", resp.StatusCode)
	}
}

// TestHandicapReview_DuplicatePlayerRecordsNotCombined verifies that two separate
// players.id rows are returned as independent entries with independent rack counts.
func TestHandicapReview_DuplicatePlayerRecordsNotCombined(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")

	p1 := hrAddPlayer(t, f, f.teamA, 2.0, false)
	p2 := hrAddPlayer(t, f, f.teamB, 2.0, false)

	mid := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid, 1, p1, p2,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(2.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	if len(recs) != 2 {
		t.Errorf("want 2 separate player entries (no merging), got %d", len(recs))
	}
	ids := map[float64]bool{}
	for _, r := range recs {
		ids[r["player_id"].(float64)] = true
	}
	if len(ids) != 2 {
		t.Errorf("want 2 distinct player_id values, got %d", len(ids))
	}
}

// TestHandicapReview_CrossLeague8BallParticipates verifies that a player's racks
// from a second 8-ball league contribute to their lifetime rack count in the
// Handicap Review endpoint.
func TestHandicapReview_CrossLeague8BallParticipates(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// Main season: one closed match (3 racks).
	mid1 := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid1, 1, homeID, awayID, 10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	// Second 8-ball league and season -- same player appears as home (3 more racks).
	var league2ID, season2ID, teamC, teamD, opp2ID, match2ID int64
	res, _ := db.DB.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('League2','8ball','Tuesday')`)
	league2ID, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO seasons (league_id, name, schedule_type, num_weeks, active) VALUES (?,'S2','single_rr',8,0)`, league2ID)
	season2ID, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamC')`, league2ID)
	teamC, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamD')`, league2ID)
	teamD, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Opp','Two',?,3.0,1)`, teamD)
	opp2ID, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-02-01',1,1,1)`, season2ID, teamC, teamD)
	match2ID, _ = res.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		match2ID, homeID, opp2ID)

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("player not found in recommendations")
	}
	// 3 racks from season1 + 3 racks from season2 = 6 total lifetime racks.
	if got := int(homeRec["lifetime_racks"].(float64)); got != 6 {
		t.Errorf("cross-league 8-ball: want lifetime_racks=6, got %d", got)
	}
	_ = awayID
}

// TestHandicapReview_Non8BallRacksExcluded verifies that a player's racks from
// a non-8-ball league do not contribute to their lifetime rack count.
func TestHandicapReview_Non8BallRacksExcluded(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// 8-ball season: one closed match (3 racks).
	mid1 := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid1, 1, homeID, awayID, 10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	// 9-ball league and season -- same player; these racks must NOT be counted.
	var league9ID, season9ID, teamE, teamF, opp9ID, match9ID int64
	res9, _ := db.DB.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('NineBall','9ball','Wednesday')`)
	league9ID, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO seasons (league_id, name, schedule_type, num_weeks, active) VALUES (?,'S9','single_rr',8,0)`, league9ID)
	season9ID, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamE')`, league9ID)
	teamE, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamF')`, league9ID)
	teamF, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Opp','Nine',?,3.0,1)`, teamF)
	opp9ID, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-03-01',1,1,1)`, season9ID, teamE, teamF)
	match9ID, _ = res9.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		match9ID, homeID, opp9ID)

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("player not found in recommendations")
	}
	// Only the 3 racks from the 8-ball season count; 9-ball racks excluded by game_format filter.
	if got := int(homeRec["lifetime_racks"].(float64)); got != 3 {
		t.Errorf("non-8ball excluded: want lifetime_racks=3, got %d (9-ball racks leaked)", got)
	}
	_, _ = awayID, season9ID
}

// TestHandicapReview_WeekReopeningSlideWindow verifies that reopening a closed match
// removes its racks from the calculation and shrinks the window accordingly.
func TestHandicapReview_WeekReopeningSlideWindow(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "4")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// Week 1 (older): 3 racks.
	res, _ := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-01-01',1,1,1)`, f.sid, f.teamA, f.teamB)
	mid1, _ := res.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		mid1, homeID, awayID)

	// Week 2 (more recent): 3 racks.
	res, _ = db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-02-01',2,1,1)`, f.sid, f.teamA, f.teamB)
	mid2, _ := res.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		mid2, homeID, awayID)

	// Both closed: 6 total racks, window=4 -> window takes 4 most-recent racks.
	recs1 := hrGetRecs(t, f.srv, f.sid)
	var rec1 map[string]any
	for _, r := range recs1 {
		if int64(r["player_id"].(float64)) == homeID {
			rec1 = r
		}
	}
	if rec1 == nil {
		t.Fatal("player not found before reopen")
	}
	if got := int(rec1["lifetime_racks"].(float64)); got != 6 {
		t.Errorf("before reopen: want lifetime_racks=6, got %d", got)
	}
	if got := int(rec1["window_racks"].(float64)); got != 4 {
		t.Errorf("before reopen: want window_racks=4 (window=4, total=6), got %d", got)
	}

	// Reopen the more-recent match (week 2).
	db.DB.Exec(`UPDATE matches SET week_closed=0 WHERE id=?`, mid2)

	// After reopen: only week 1's 3 racks remain; window_racks capped at available.
	recs2 := hrGetRecs(t, f.srv, f.sid)
	var rec2 map[string]any
	for _, r := range recs2 {
		if int64(r["player_id"].(float64)) == homeID {
			rec2 = r
		}
	}
	if rec2 == nil {
		t.Fatal("player not found after reopen")
	}
	if got := int(rec2["lifetime_racks"].(float64)); got != 3 {
		t.Errorf("after reopen: want lifetime_racks=3 (week2 removed), got %d", got)
	}
	if got := int(rec2["window_racks"].(float64)); got != 3 {
		t.Errorf("after reopen: want window_racks=3 (fewer than window=4), got %d", got)
	}
}

// TestCloseWeek_AdvanceResultHandicapShape verifies that the Close Week response's
// advance_result.handicap uses PlayerHandicapRec fields (current_handicap,
// recommended_handicap, matches_played) and not HandicapReviewRec fields.
func TestCloseWeek_AdvanceResultHandicapShape(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	ar, _ := result["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("want advance_result in close response")
	}
	hc, _ := ar["handicap"].(map[string]any)
	if hc == nil {
		t.Fatal("want advance_result.handicap in close response")
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Skip("no recommendations available -- cannot verify shape")
	}
	rec, _ := recs[0].(map[string]any)

	// Must have PlayerHandicapRec fields.
	if _, ok := rec["current_handicap"]; !ok {
		t.Error("close week advance_result.handicap rec missing current_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["recommended_handicap"]; !ok {
		t.Error("close week advance_result.handicap rec missing recommended_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["matches_played"]; !ok {
		t.Error("close week advance_result.handicap rec missing matches_played (PlayerHandicapRec field)")
	}
	// Must NOT have HandicapReviewRec-only fields.
	if _, ok := rec["assigned_hc"]; ok {
		t.Error("close week advance_result.handicap must not contain assigned_hc (HandicapReviewRec field leaked)")
	}
	if _, ok := rec["window_hc"]; ok {
		t.Error("close week advance_result.handicap must not contain window_hc (HandicapReviewRec field leaked)")
	}
}

// TestHandicapReview_AdvancePreviewShapeUnchanged verifies that the advance-preview
// response uses PlayerHandicapRec fields and is not contaminated by HandicapReviewRec fields.
func TestHandicapReview_AdvancePreviewShapeUnchanged(t *testing.T) {
	f := weekTestSeed(t)

	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'B')`, f.sid, f.teamB)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, f.playerB)
	db.DB.Exec(`UPDATE seasons SET teams_managed=1 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		f.sid, "handicap_update_method", "Method", "game_diff_average")

	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/advance-preview", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	hc, _ := preview["handicap"].(map[string]any)
	if hc == nil {
		t.Fatal("advance-preview: missing 'handicap' field")
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		return // no data is acceptable
	}

	rec, _ := recs[0].(map[string]any)
	if _, ok := rec["current_handicap"]; !ok {
		t.Error("advance-preview rec missing current_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["recommended_handicap"]; !ok {
		t.Error("advance-preview rec missing recommended_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["matches_played"]; !ok {
		t.Error("advance-preview rec missing matches_played (PlayerHandicapRec field)")
	}
	if _, ok := rec["assigned_hc"]; ok {
		t.Error("advance-preview rec must not contain assigned_hc (HandicapReviewRec field)")
	}
	if _, ok := rec["window_hc"]; ok {
		t.Error("advance-preview rec must not contain window_hc (HandicapReviewRec field)")
	}
}
