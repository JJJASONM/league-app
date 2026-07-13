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

// --- Standings via league_id (FindActiveSeasonByLeague boundary) ---

// TestStandings_LeagueID_NoActiveSeason returns empty standings when there is no
// active season for the given league_id, exercising FindActiveSeasonByLeague.
func TestStandings_LeagueID_NoActiveSeason(t *testing.T) {
	f := weekTestSeed(t)
	// weekTestSeed activates the season; revert to draft to test the inactive path.
	db.DB.Exec(`UPDATE seasons SET active=0, activated_at=NULL WHERE id=?`, f.sid)
	resp, err := http.Get(fmt.Sprintf("%s/api/standings?league_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var standings []map[string]any
	json.NewDecoder(resp.Body).Decode(&standings)
	if len(standings) != 0 {
		t.Errorf("want empty standings for inactive season, got %d entries", len(standings))
	}
}

// TestStandings_LeagueID_ResolvesActiveSeasonAndReturnsStandings activates the
// season, closes a week with results, and confirms standings are returned when
// calling GET /api/standings?league_id=X (no explicit season_id).
func TestStandings_LeagueID_ResolvesActiveSeasonAndReturnsStandings(t *testing.T) {
	f := weekTestSeed(t)

	// Get the league_id for this season.
	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, f.sid).Scan(&leagueID); err != nil {
		t.Fatalf("get league_id: %v", err)
	}

	// Activate the season and add match results.
	db.DB.Exec(`UPDATE seasons SET active=1 WHERE id=?`, f.sid)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,0,3,-3)`, f.matchID, f.playerB, f.teamB)

	// Close the week so standings count the match.
	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	// Request standings by league_id -- no season_id in query.
	resp, err := http.Get(fmt.Sprintf("%s/api/standings?league_id=%d", f.srv.URL, leagueID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var standings []map[string]any
	json.NewDecoder(resp.Body).Decode(&standings)
	totalPlayed := 0
	for _, s := range standings {
		if p, _ := s["played"].(float64); p > 0 {
			totalPlayed++
		}
	}
	if totalPlayed == 0 {
		t.Error("want standings via league_id to include closed match")
	}
}

