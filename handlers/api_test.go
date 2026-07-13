package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/leagues"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/players"
	"league_app/backend/domains/rules"
	"league_app/backend/domains/seasons"
	"league_app/backend/domains/teams"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/handlers"
)


func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	// Close the DB before the temp dir is removed (required on Windows).
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	hcStore := sqlite.NewHandicapStore(db.DB)
	hcSvc := handicaps.NewService(hcStore)
	weekStore := sqlite.NewWeekStore(db.DB)
	ruleStore := sqlite.NewRuleStore(db.DB)
	weekSvc := matches.NewWeekService(weekStore, hcSvc, ruleStore)
	roundStore := sqlite.NewRoundStore(db.DB)
	roundSvc := matches.NewRoundService(roundStore, ruleStore)
	ruleSvc := rules.NewRuleService(ruleStore)
	seasonStore := sqlite.NewSeasonStore(db.DB)
	seasonSvc := seasons.NewSeasonService(seasonStore)
	leagueStore := sqlite.NewLeagueStore(db.DB)
	leagueSvc := leagues.NewLeagueService(leagueStore)
	playerStore := sqlite.NewPlayerStore(db.DB)
	playerSvc := players.NewPlayerService(playerStore)
	teamStore := sqlite.NewTeamStore(db.DB)
	teamSvc := teams.NewTeamService(teamStore)
	scheduleStore := sqlite.NewScheduleStore(db.DB)
	scheduleSvc := matches.NewScheduleService(scheduleStore)
	matchStore := sqlite.NewMatchStore(db.DB)
	matchSvc := matches.NewMatchService(matchStore)
	lineupStore := sqlite.NewLineupStore(db.DB)
	lineupSvc := matches.NewLineupService(lineupStore)
	deps := handlers.Dependencies{HandicapSvc: hcSvc, WeekMgr: weekSvc, RoundMgr: roundSvc, RuleMgr: ruleSvc, LeagueMgr: leagueSvc, PlayerMgr: playerSvc, TeamMgr: teamSvc, SeasonMgr: seasonSvc, ScheduleMgr: scheduleSvc, MatchMgr: matchSvc, LineupMgr: lineupSvc}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// seedSeason creates one league and one season, returning the season ID.
func seedSeason(t *testing.T, base string) int64 {
	t.Helper()
	post := func(path, body string) *http.Response {
		resp, err := http.Post(base+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		return resp
	}

	resp := post("/api/leagues", `{"name":"Test League","game_format":"8ball"}`)
	resp.Body.Close()

	resp2 := post("/api/seasons", `{"league_id":1,"name":"Spring 2026"}`)
	defer resp2.Body.Close()
	var s map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil {
		t.Fatalf("decode season: %v", err)
	}
	return int64(s["id"].(float64))
}


// Week Workflow (Close Week) --------------------------------------------------

// weekFixture is the result of weekTestSeed: a running server plus pre-seeded IDs.
type weekFixture struct {
	srv     *httptest.Server
	sid     int64 // season ID
	matchID int64
	teamA   int64
	teamB   int64
	playerA int64 // one player on team A
	playerB int64 // one player on team B
}

// weekTestSeed spins up a fresh test server, creates one league, one season, two teams
// with one player each, and one unscored match in week 1. Cleanup is registered on t.
func weekTestSeed(t *testing.T) weekFixture {
	t.Helper()
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID); err != nil {
		t.Fatalf("weekTestSeed: season league: %v", err)
	}
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team B')`, leagueID)
	teamA, _ := rA.LastInsertId()
	teamB, _ := rB.LastInsertId()

	rPA, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home','Player',?,3.0)`, teamA)
	rPB, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Away','Player',?,3.0)`, teamB)
	playerA, _ := rPA.LastInsertId()
	playerB, _ := rPB.LastInsertId()

	rm, err := db.DB.Exec(`
		INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		VALUES (?,?,?,1)`, sid, teamA, teamB)
	if err != nil {
		t.Fatalf("weekTestSeed: insert match: %v", err)
	}
	matchID, _ := rm.LastInsertId()

	// Activate the season: close-week requires an active (non-draft) season.
	if _, err := db.DB.Exec(
		`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid,
	); err != nil {
		t.Fatalf("weekTestSeed: activate season: %v", err)
	}

	return weekFixture{srv, sid, matchID, teamA, teamB, playerA, playerB}
}

// seedRoundResult inserts one round_results row with a game winner (home wins all 3)
// and sets matches.completed=1. Used to satisfy Close Week's game-winner requirement.
func seedRoundResult(t *testing.T, matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		matchID, homePlayerID, awayPlayerID)
	if err != nil {
		t.Fatalf("seedRoundResult: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
}

// ─── Skip date and match date normalization ───────────────────────────────────

// seedScheduleFixture creates a league, 3 teams (odd), and one season.
// Returns (leagueID, seasonID).
func seedScheduleFixture(t *testing.T, srv *httptest.Server, startDate string) (leagueID, seasonID int64) {
	var teamIDs []int64
	leagueID, seasonID, teamIDs = seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	return
}

// ensureSeasonTeams inserts all teamIDs into season_teams via direct DB access.
// Idempotent (INSERT OR IGNORE). Required for managed seasons before schedule
// generation or bye validation.
func ensureSeasonTeams(t *testing.T, seasonID int64, teamIDs []int64) {
	t.Helper()
	for _, tid := range teamIDs {
		if _, err := db.DB.Exec(
			`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name)
			 SELECT ?, id, name FROM teams WHERE id=?`, seasonID, tid); err != nil {
			t.Fatalf("ensureSeasonTeams: %v", err)
		}
	}
}

// seedScheduleFixtureWithTeams creates a league, the named teams, and one season.
// Returns (leagueID, seasonID, []teamID).
func seedScheduleFixtureWithTeams(t *testing.T, srv *httptest.Server, startDate string, teamNames ...string) (leagueID, seasonID int64, teamIDs []int64) {
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
	lg := pd("/api/leagues", `{"name":"Sched League","game_format":"8ball"}`)
	leagueID = int64(lg["id"].(float64))
	for _, name := range teamNames {
		tm := pd("/api/teams", fmt.Sprintf(`{"league_id":%d,"name":%q}`, leagueID, name))
		teamIDs = append(teamIDs, int64(tm["id"].(float64)))
	}
	s := pd("/api/seasons", fmt.Sprintf(`{"league_id":%d,"name":"Test Season","start_date":%q}`, leagueID, startDate))
	seasonID = int64(s["id"].(float64))
	return
}

// generateAndGetMatches POSTs /matches/generate and then fetches the resulting matches.
func generateAndGetMatches(t *testing.T, srv *httptest.Server, seasonID int64, startDate string, skipDates []string) []map[string]any {
	t.Helper()
	skipsJSON, _ := json.Marshal(skipDates)
	body := fmt.Sprintf(`{"season_id":%d,"start_date":%q,"schedule_type":"single_rr","skip_dates":%s}`,
		seasonID, startDate, skipsJSON)
	genResp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /matches/generate: %v", err)
	}
	genResp.Body.Close()
	if genResp.StatusCode != http.StatusOK {
		t.Fatalf("generate: want 200, got %d", genResp.StatusCode)
	}

	matchResp, err := http.Get(fmt.Sprintf("%s/api/matches?season_id=%d", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET /matches: %v", err)
	}
	defer matchResp.Body.Close()
	var matches []map[string]any
	json.NewDecoder(matchResp.Body).Decode(&matches)
	return matches
}

// ─── Bye request validation ───────────────────────────────────────────────────

// postByeRequest is a helper that sends a POST /seasons/{id}/bye-requests.
func postByeRequest(t *testing.T, srv *httptest.Server, seasonID, teamID int64, weekNum int) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"team_id":%d,"week_number":%d,"reason":"test"}`, teamID, weekNum)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST bye-request: %v", err)
	}
	return resp
}




