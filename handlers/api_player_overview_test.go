// Tests for GET /api/players/{id}/overview (Player Overview Phase 1).
package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/db"
	"league_app/models"
)

// playerOverviewFixture is the result of playerOverviewSeed: a running
// server plus pre-seeded league/season/teams/players/roster/match data.
type playerOverviewFixture struct {
	srv      *httptest.Server
	leagueID int64
	seasonID int64
	teamA    int64
	teamB    int64
	playerA  int64 // rostered on teamA for seasonID, has a completed/closed match result
	playerB  int64 // rostered on teamB for seasonID
	matchID  int64 // teamA (home) vs teamB (away), week 1, completed and week_closed
}

// playerOverviewSeed creates one active season with two teams, two players
// rostered via season_rosters, one completed+week_closed match between the
// teams, and one match_results row for playerA so stats are non-zero.
func playerOverviewSeed(t *testing.T) playerOverviewFixture {
	t.Helper()
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID); err != nil {
		t.Fatalf("playerOverviewSeed: season league: %v", err)
	}
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Overview Team A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Overview Team B')`, leagueID)
	teamA, _ := rA.LastInsertId()
	teamB, _ := rB.LastInsertId()

	rPA, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home','Player',?,2.5)`, teamA)
	rPB, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Away','Player',?,1.5)`, teamB)
	playerA, _ := rPA.LastInsertId()
	playerB, _ := rPB.LastInsertId()

	// season_rosters requires the team to already be in season_teams.
	ensureSeasonTeams(t, sid, []int64{teamA, teamB})

	if _, err := db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, sid, teamA, playerA); err != nil {
		t.Fatalf("playerOverviewSeed: roster A: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, sid, teamB, playerB); err != nil {
		t.Fatalf("playerOverviewSeed: roster B: %v", err)
	}

	rm, err := db.DB.Exec(`
		INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed, week_closed)
		VALUES (?,?,?,1,1,1)`, sid, teamA, teamB)
	if err != nil {
		t.Fatalf("playerOverviewSeed: insert match: %v", err)
	}
	matchID, _ := rm.LastInsertId()

	if _, err := db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, sets_won, sets_lost, games_won, games_lost, diff)
		VALUES (?,?,?,2,1,20,15,5)`, matchID, playerA, teamA); err != nil {
		t.Fatalf("playerOverviewSeed: insert match_results: %v", err)
	}

	if _, err := db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid); err != nil {
		t.Fatalf("playerOverviewSeed: activate season: %v", err)
	}

	return playerOverviewFixture{srv, leagueID, sid, teamA, teamB, playerA, playerB, matchID}
}

func getPlayerOverview(t *testing.T, base string, playerID int64, query string) (*http.Response, models.PlayerOverview) {
	t.Helper()
	url := fmt.Sprintf("%s/api/players/%d/overview%s", base, playerID, query)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	var overview models.PlayerOverview
	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&overview); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp, overview
}

func TestPlayerOverview_ExplicitSeasonID_Returns200(t *testing.T) {
	f := playerOverviewSeed(t)
	resp, overview := getPlayerOverview(t, f.srv.URL, f.playerA, fmt.Sprintf("?season_id=%d", f.seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if overview.Player.ID != f.playerA {
		t.Errorf("want player.id=%d, got %d", f.playerA, overview.Player.ID)
	}
	if overview.Season.ID != f.seasonID {
		t.Errorf("want season.id=%d, got %d", f.seasonID, overview.Season.ID)
	}
	if overview.Team == nil || overview.Team.ID != f.teamA {
		t.Errorf("want team.id=%d, got %+v", f.teamA, overview.Team)
	}
	if overview.Handicap.Current != 2.5 {
		t.Errorf("want handicap.current=2.5, got %v", overview.Handicap.Current)
	}
	if len(overview.Schedule) != 1 {
		t.Fatalf("want 1 schedule row, got %d", len(overview.Schedule))
	}
	row := overview.Schedule[0]
	if row.MatchID != f.matchID {
		t.Errorf("want match_id=%d, got %d", f.matchID, row.MatchID)
	}
	if row.HomeOrAway != "home" {
		t.Errorf("want home_or_away=home, got %q", row.HomeOrAway)
	}
	if row.OpponentTeamName != "Overview Team B" {
		t.Errorf("want opponent_team_name='Overview Team B', got %q", row.OpponentTeamName)
	}
	if overview.Stats.SetsWon != 2 || overview.Stats.GamesWon != 20 {
		t.Errorf("want sets_won=2 games_won=20, got %+v", overview.Stats)
	}
	if overview.Money.Tracked {
		t.Error("want money.tracked=false")
	}
	if overview.Money.Message == "" {
		t.Error("want a non-empty money.message explaining dues/payouts are not tracked")
	}
}

func TestPlayerOverview_OmittedSeasonID_UsesActiveSeason(t *testing.T) {
	f := playerOverviewSeed(t) // seed already activates the season
	resp, overview := getPlayerOverview(t, f.srv.URL, f.playerA, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if overview.Season.ID != f.seasonID {
		t.Errorf("want season.id=%d (the active season), got %d", f.seasonID, overview.Season.ID)
	}
}

func TestPlayerOverview_MissingPlayer_Returns404(t *testing.T) {
	f := playerOverviewSeed(t)
	resp, _ := getPlayerOverview(t, f.srv.URL, 9_999_999, fmt.Sprintf("?season_id=%d", f.seasonID))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

// TestPlayerOverview_NotRosteredFallsBackToDirectTeam covers a player with
// no season_rosters entry for the selected season but a direct
// players.team_id -- the fallback path should still resolve their team and
// schedule, with zeroed stats since they have no match_results row.
func TestPlayerOverview_NotRosteredFallsBackToDirectTeam(t *testing.T) {
	f := playerOverviewSeed(t)
	r, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('NotRostered','Player',?,0)`, f.teamA)
	playerC, _ := r.LastInsertId()

	resp, overview := getPlayerOverview(t, f.srv.URL, playerC, fmt.Sprintf("?season_id=%d", f.seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if overview.Team == nil || overview.Team.ID != f.teamA {
		t.Errorf("want fallback to direct team_id=%d, got %+v", f.teamA, overview.Team)
	}
	if len(overview.Schedule) != 1 {
		t.Errorf("want 1 schedule row (same team's match), got %d", len(overview.Schedule))
	}
	if overview.Stats.SetsWon != 0 {
		t.Errorf("want zeroed stats (no match_results row for this player), got %+v", overview.Stats)
	}
	if overview.Money.Tracked {
		t.Error("want money.tracked=false")
	}
}

// TestPlayerOverview_SeasonFromDifferentLeague_Returns404 verifies the
// player/season league-coherence guard: an explicit season_id belonging
// to a different league than the player must be rejected, not silently
// composed via a direct-team_id fallback into a nonsensical cross-league
// overview.
func TestPlayerOverview_SeasonFromDifferentLeague_Returns404(t *testing.T) {
	f := playerOverviewSeed(t)

	resp, err := http.Post(f.srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	if err != nil {
		t.Fatalf("create other league: %v", err)
	}
	resp.Body.Close()

	resp2, err := http.Post(f.srv.URL+"/api/seasons", "application/json",
		strings.NewReader(`{"league_id":2,"name":"Other Season"}`))
	if err != nil {
		t.Fatalf("create other season: %v", err)
	}
	defer resp2.Body.Close()
	var s map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil {
		t.Fatalf("decode other season: %v", err)
	}
	otherSeasonID := int64(s["id"].(float64))

	resp3, _ := getPlayerOverview(t, f.srv.URL, f.playerA, fmt.Sprintf("?season_id=%d", otherSeasonID))
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 (season belongs to a different league than the player), got %d", resp3.StatusCode)
	}
}

// TestPlayerOverview_NoTeamAtAll_TeamIsNilAndScheduleEmpty covers a player
// with neither a season_rosters entry nor a direct team_id -- the handler
// must not error, just return an honestly empty overview.
func TestPlayerOverview_NoTeamAtAll_TeamIsNilAndScheduleEmpty(t *testing.T) {
	f := playerOverviewSeed(t)
	r, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, handicap) VALUES ('NoTeam','Player',0)`)
	playerD, _ := r.LastInsertId()

	resp, overview := getPlayerOverview(t, f.srv.URL, playerD, fmt.Sprintf("?season_id=%d", f.seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if overview.Team != nil {
		t.Errorf("want team=nil, got %+v", overview.Team)
	}
	if len(overview.Schedule) != 0 {
		t.Errorf("want empty schedule, got %d rows", len(overview.Schedule))
	}
	if overview.Stats.SetsWon != 0 {
		t.Errorf("want zeroed stats, got %+v", overview.Stats)
	}
	if overview.Money.Tracked {
		t.Error("want money.tracked=false")
	}
}
