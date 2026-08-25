package handlers_test

// Weekly Score Processing Phase 1A: match-level approve/process/unapprove/
// unprocess integration tests, plus the score-edit-blocked-after-approval/
// processing regression and the handicap-recommendations-include-processed-
// but-open-match regression.
//
//   POST /api/matches/{id}/approve
//   POST /api/matches/{id}/process
//   POST /api/matches/{id}/unapprove
//   POST /api/matches/{id}/unprocess
//
// Shared infrastructure (weekTestSeed, seedRoundResult, seedRoundResultsN)
// lives in api_test.go / api_handicap_test.go.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

func postNoBody(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestApproveMatch_Success_SetsApprovedAt(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var approvedAt sql.NullString
	db.DB.QueryRow(`SELECT approved_at FROM matches WHERE id=?`, f.matchID).Scan(&approvedAt)
	if !approvedAt.Valid || approvedAt.String == "" {
		t.Error("want approved_at set after approve")
	}
}

func TestApproveMatch_WithNote_StoresNote(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(`{"note":"captain confirmed via text"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var note string
	db.DB.QueryRow(`SELECT approval_note FROM matches WHERE id=?`, f.matchID).Scan(&note)
	if note != "captain confirmed via text" {
		t.Errorf("want approval_note=%q, got %q", "captain confirmed via text", note)
	}
}

func TestApproveMatch_NotScored_Returns422(t *testing.T) {
	f := weekTestSeed(t)
	// No round result saved -- match.completed stays 0.
	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", resp.StatusCode)
	}
}

func TestApproveMatch_NotFound_Returns404(t *testing.T) {
	f := weekTestSeed(t)
	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/999999/approve", f.srv.URL))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestApproveMatch_AlreadyProcessed_Returns409(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/process", f.srv.URL, f.matchID)).Body.Close()

	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409, got %d", resp.StatusCode)
	}
}

func TestProcessMatch_Success_SetsProcessedAt(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()

	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/process", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var processedAt sql.NullString
	db.DB.QueryRow(`SELECT processed_at FROM matches WHERE id=?`, f.matchID).Scan(&processedAt)
	if !processedAt.Valid || processedAt.String == "" {
		t.Error("want processed_at set after process")
	}
}

func TestProcessMatch_NotApproved_Returns422(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/process", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", resp.StatusCode)
	}
}

func TestUnapproveMatch_Success_ClearsApproval(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()

	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/unapprove", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var approvedAt sql.NullString
	db.DB.QueryRow(`SELECT approved_at FROM matches WHERE id=?`, f.matchID).Scan(&approvedAt)
	if approvedAt.Valid {
		t.Error("want approved_at cleared after unapprove")
	}
}

func TestUnapproveMatch_AlreadyProcessed_Returns409(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/process", f.srv.URL, f.matchID)).Body.Close()

	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/unapprove", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409, got %d", resp.StatusCode)
	}
}

func TestUnprocessMatch_Success_ClearsProcessingPreservesApproval(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/process", f.srv.URL, f.matchID)).Body.Close()

	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/unprocess", f.srv.URL, f.matchID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var approvedAt, processedAt sql.NullString
	db.DB.QueryRow(`SELECT approved_at, processed_at FROM matches WHERE id=?`, f.matchID).
		Scan(&approvedAt, &processedAt)
	if processedAt.Valid {
		t.Error("want processed_at cleared after unprocess")
	}
	if !approvedAt.Valid {
		t.Error("want approved_at to remain set after unprocess (approval is preserved)")
	}
}

// --- Score edits blocked after approval/processing ---

func TestSaveRounds_ReturnsConflict_AfterApproval(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()

	body := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (approved match must not accept a normal score edit), got %d", resp.StatusCode)
	}
}

func TestSubmitResults_ReturnsConflict_AfterProcessing(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/process", f.srv.URL, f.matchID)).Body.Close()

	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/results", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(`{"results":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (processed match must not accept a normal score edit), got %d", resp.StatusCode)
	}
}

func TestClearResults_ReturnsConflict_AfterApproval(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/matches/%d/results", f.srv.URL, f.matchID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (approved match must not accept clear-results), got %d", resp.StatusCode)
	}
}

// --- Handicap recommendations include processed-but-open matches ---

// TestHandicapRecommendations_IncludeProcessedButOpenMatch proves the Phase
// 1A compatibility eligibility gate end to end: a match that is processed
// but whose week was never closed still contributes to
// GET /handicap-recommendations.
func TestHandicapRecommendations_IncludeProcessedButOpenMatch(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id) VALUES (?,?)`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	setHandicapMethod(t, f.sid, "game_diff_average")
	// 5 rounds (15 racks, at the default eligibility threshold) of home=10/away=7.
	seedRoundResultsN(t, f.matchID, f.playerA, f.playerB, 5, 10, 7, 0.0, 0.0)

	postNoBody(t, fmt.Sprintf("%s/api/matches/%d/approve", f.srv.URL, f.matchID)).Body.Close()
	resp := postNoBody(t, fmt.Sprintf("%s/api/matches/%d/process", f.srv.URL, f.matchID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("process match: want 200, got %d", resp.StatusCode)
	}

	// Week was never closed -- confirm that directly before asserting on recs.
	var weekClosed int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, f.matchID).Scan(&weekClosed)
	if weekClosed != 0 {
		t.Fatal("test setup error: week must remain open for this assertion to be meaningful")
	}

	httpResp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer httpResp.Body.Close()
	var data map[string]any
	json.NewDecoder(httpResp.Body).Decode(&data)

	if status, _ := data["status"].(string); status != "preview" {
		t.Fatalf("want status=preview (processed match should count as real data), got %q: %v", status, data)
	}
	recs, _ := data["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			includedRacks, _ := rec["included_racks"].(float64)
			if includedRacks != 15 {
				t.Errorf("want included_racks=15 for the processed-but-open match, got %v", includedRacks)
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}
