package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"errors"

	"league_app/backend/domainerr"
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
	"league_app/models"
)

// stubHandicapSvc is a test double for handlers.HandicapRecommender.
// Set fn to control what Recommendations returns.
type stubHandicapSvc struct {
	fn func(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error)
}

func (s *stubHandicapSvc) Recommendations(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error) {
	return s.fn(ctx, seasonID)
}

// noopLeagueMgr satisfies handlers.LeagueManager for tests that only exercise
// non-league handler logic.
type noopLeagueMgr struct{}

func (n *noopLeagueMgr) ListLeagues(_ context.Context) ([]models.League, error) {
	return []models.League{}, nil
}
func (n *noopLeagueMgr) GetLeague(_ context.Context, _ int64) (models.League, error) {
	return models.League{}, nil
}
func (n *noopLeagueMgr) CreateLeague(_ context.Context, _ leagues.CreateLeagueInput) (models.League, error) {
	return models.League{}, nil
}
func (n *noopLeagueMgr) UpdateLeague(_ context.Context, _ int64, _ leagues.UpdateLeagueInput) error {
	return nil
}
func (n *noopLeagueMgr) DeleteLeague(_ context.Context, _ int64) error { return nil }

// noopRuleMgr satisfies handlers.RuleManager for tests that only exercise other
// handler logic and do not exercise the season-rules endpoints.
type noopRuleMgr struct{}

func (n *noopRuleMgr) List(_ context.Context, _ int64) ([]models.SeasonRule, error) {
	return nil, nil
}
func (n *noopRuleMgr) Upsert(_ context.Context, r models.SeasonRule) (models.SeasonRule, error) {
	return r, nil
}
func (n *noopRuleMgr) Update(_ context.Context, _ int64, _, _ string) error { return nil }
func (n *noopRuleMgr) Delete(_ context.Context, _ int64) error               { return nil }

// noopSeasonMgr satisfies handlers.SeasonManager for tests that only exercise
// non-season handler logic.
type noopSeasonMgr struct{}

func (n *noopSeasonMgr) Activate(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) Checklist(_ context.Context, _ int64) (models.SetupChecklist, error) {
	return models.SetupChecklist{CanActivate: true}, nil
}
func (n *noopSeasonMgr) PreviousSeason(_ context.Context, _ int64) (seasons.PreviousSeasonResult, error) {
	return seasons.PreviousSeasonResult{Teams: []seasons.SeasonTeamEntry{}}, nil
}
func (n *noopSeasonMgr) IsDraft(_ context.Context, _ int64) (bool, error) { return true, nil }
func (n *noopSeasonMgr) MarkStaleIfScheduled(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) AddTeam(_ context.Context, _ int64, _ seasons.AddTeamRequest) (models.SeasonTeam, error) {
	return models.SeasonTeam{}, nil
}
func (n *noopSeasonMgr) RemoveTeam(_ context.Context, _, _ int64) error { return nil }
func (n *noopSeasonMgr) UpdateTeam(_ context.Context, _, _ int64, _ seasons.UpdateTeamRequest) (models.SeasonTeam, error) {
	return models.SeasonTeam{}, nil
}
func (n *noopSeasonMgr) CreateByeRequest(_ context.Context, _ int64, _ seasons.CreateByeRequestInput) (models.ByeRequest, error) {
	return models.ByeRequest{}, nil
}
func (n *noopSeasonMgr) UpdateByeRequest(_ context.Context, _, _ int64, _ bool) (models.ByeRequest, error) {
	return models.ByeRequest{}, nil
}
func (n *noopSeasonMgr) ListRoster(_ context.Context, _, _ int64) ([]models.SeasonRosterEntry, error) {
	return []models.SeasonRosterEntry{}, nil
}
func (n *noopSeasonMgr) AddRosterPlayer(_ context.Context, _, _, _ int64) (models.SeasonRosterEntry, error) {
	return models.SeasonRosterEntry{}, nil
}
func (n *noopSeasonMgr) RemoveRosterPlayer(_ context.Context, _, _, _ int64) error { return nil }
func (n *noopSeasonMgr) ListAvailablePlayers(_ context.Context, _ int64) ([]models.Player, error) {
	return []models.Player{}, nil
}
func (n *noopSeasonMgr) ListSeasonTeams(_ context.Context, _ int64) ([]models.SeasonTeam, error) {
	return []models.SeasonTeam{}, nil
}
func (n *noopSeasonMgr) ListSeasons(_ context.Context, _ *int64) ([]models.Season, error) {
	return []models.Season{}, nil
}
func (n *noopSeasonMgr) GetSeason(_ context.Context, _ int64) (models.Season, error) {
	return models.Season{}, nil
}
func (n *noopSeasonMgr) CreateSeason(_ context.Context, _ seasons.CreateSeasonInput) (models.Season, error) {
	return models.Season{}, nil
}
func (n *noopSeasonMgr) UpdateSeason(_ context.Context, _ int64, _ seasons.UpdateSeasonInput) (models.Season, error) {
	return models.Season{}, nil
}
func (n *noopSeasonMgr) DeleteSeason(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) ListSkippedWeeks(_ context.Context, _ int64) ([]models.SkippedWeek, error) {
	return []models.SkippedWeek{}, nil
}
func (n *noopSeasonMgr) CreateSkippedWeek(_ context.Context, _ int64, _, _ string) (models.SkippedWeek, error) {
	return models.SkippedWeek{}, nil
}
func (n *noopSeasonMgr) DeleteSkippedWeek(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) ListByeRequests(_ context.Context, _ int64) ([]models.ByeRequest, error) {
	return []models.ByeRequest{}, nil
}
func (n *noopSeasonMgr) DeleteByeRequest(_ context.Context, _, _ int64) error { return nil }
func (n *noopSeasonMgr) FindActiveSeasonByLeague(_ context.Context, _ int64) (int64, bool, error) {
	return 0, false, nil
}
func (n *noopSeasonMgr) RosterEligible(_ context.Context, _ int64, _ int) (bool, string, error) {
	return true, "", nil
}

// noopScheduleMgr satisfies handlers.ScheduleManager for tests that don't
// exercise schedule generation.
type noopScheduleMgr struct{}

func (n *noopScheduleMgr) GenerateSchedule(_ context.Context, _ matches.GenerateRequest) (matches.GenerateResult, error) {
	return matches.GenerateResult{}, nil
}

// noopMatchMgr satisfies handlers.MatchManager for tests that don't exercise
// match listing, detail, or team assignment.
type noopMatchMgr struct{}

func (n *noopMatchMgr) ListMatches(_ context.Context, _ matches.ListMatchesRequest) ([]models.Match, error) {
	return []models.Match{}, nil
}
func (n *noopMatchMgr) GetMatch(_ context.Context, _ int64) (models.MatchDetail, error) {
	return models.MatchDetail{}, nil
}
func (n *noopMatchMgr) AssignMatchTeams(_ context.Context, _ int64, _, _ *int64) error {
	return nil
}

// noopPlayerMgr satisfies handlers.PlayerManager for tests that only exercise
// non-player handler logic.
type noopPlayerMgr struct{}

func (n *noopPlayerMgr) ListPlayers(_ context.Context, _ *int64) ([]models.Player, error) {
	return []models.Player{}, nil
}
func (n *noopPlayerMgr) GetPlayer(_ context.Context, _ int64) (models.Player, error) {
	return models.Player{}, nil
}
func (n *noopPlayerMgr) CreatePlayer(_ context.Context, _ players.CreatePlayerInput) (models.Player, error) {
	return models.Player{}, nil
}
func (n *noopPlayerMgr) UpdatePlayer(_ context.Context, _ int64, _ players.UpdatePlayerInput) error {
	return nil
}
func (n *noopPlayerMgr) DeletePlayer(_ context.Context, _ int64) error { return nil }

// noopTeamMgr satisfies handlers.TeamManager for tests that only exercise
// non-team handler logic.
type noopTeamMgr struct{}

func (n *noopTeamMgr) ListTeams(_ context.Context, _ *int64) ([]models.Team, error) {
	return []models.Team{}, nil
}
func (n *noopTeamMgr) GetTeam(_ context.Context, _ int64) (models.Team, error) {
	return models.Team{}, nil
}
func (n *noopTeamMgr) CreateTeam(_ context.Context, _ teams.CreateTeamInput) (models.Team, error) {
	return models.Team{}, nil
}
func (n *noopTeamMgr) UpdateTeam(_ context.Context, _ int64, _ teams.UpdateTeamInput) error {
	return nil
}
func (n *noopTeamMgr) DeleteTeam(_ context.Context, _ int64) error { return nil }

// noopLineupMgr satisfies handlers.LineupManager for tests that don't exercise
// lineup plan routes.
type noopLineupMgr struct{}

func (n *noopLineupMgr) ListLineupPlans(_ context.Context, _ matches.ListLineupPlansRequest) ([]models.LineupPlan, error) {
	return []models.LineupPlan{}, nil
}
func (n *noopLineupMgr) SaveTeamLineup(_ context.Context, _ matches.SaveLineupRequest) error {
	return nil
}
func (n *noopLineupMgr) DeleteLineupPlan(_ context.Context, _ int64) error { return nil }

// testServer initializes a fresh SQLite database in a temp directory and
// returns a running test HTTP server with all routes registered.
// The DB connection and server are closed automatically when the test ends.
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

// --- GET /api/seasons/{id}/handicap-recommendations (handler error mapping) ---

func TestGetHandicapRecommendations_NotFound404(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return models.HandicapReviewResponse{}, domainerr.New("HC_SEASON_NOT_FOUND", domainerr.NotFound, "season not found")
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub, RuleMgr: &noopRuleMgr{}, LeagueMgr: &noopLeagueMgr{}, PlayerMgr: &noopPlayerMgr{}, TeamMgr: &noopTeamMgr{}, SeasonMgr: &noopSeasonMgr{}})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/999/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestGetHandicapRecommendations_InternalError500(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return models.HandicapReviewResponse{}, domainerr.Wrap("HC_DATA_ERROR", domainerr.Internal, "internal error", fmt.Errorf("db offline"))
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub, RuleMgr: &noopRuleMgr{}, LeagueMgr: &noopLeagueMgr{}, PlayerMgr: &noopPlayerMgr{}, TeamMgr: &noopTeamMgr{}, SeasonMgr: &noopSeasonMgr{}})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/1/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", resp.StatusCode)
	}
}

func TestGetHandicapRecommendations_Success200(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	want := models.HandicapReviewResponse{
		SeasonID:        7,
		Method:          "manual_review",
		Status:          "no_auto_apply",
		Message:         "No handicap changes are applied automatically.",
		WeeksClosed:     0,
		Recommendations: []models.HandicapReviewRec{},
	}
	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return want, nil
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub, RuleMgr: &noopRuleMgr{}, LeagueMgr: &noopLeagueMgr{}, PlayerMgr: &noopPlayerMgr{}, TeamMgr: &noopTeamMgr{}, SeasonMgr: &noopSeasonMgr{}})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/7/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	var got models.HandicapReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SeasonID != want.SeasonID || got.Status != want.Status || got.Method != want.Method {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestGetHandicapRecommendations_NonDomainError500NoLeak asserts that a plain
// (non-domain) error returned by the service maps to 500 with a fixed safe body
// and that the original cause string never appears in the response.
func TestGetHandicapRecommendations_NonDomainError500NoLeak(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return models.HandicapReviewResponse{}, errors.New("secret database path /var/db/prod.db")
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub, RuleMgr: &noopRuleMgr{}, LeagueMgr: &noopLeagueMgr{}, PlayerMgr: &noopPlayerMgr{}, TeamMgr: &noopTeamMgr{}, SeasonMgr: &noopSeasonMgr{}})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/1/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "internal error") {
		t.Errorf("want body to contain 'internal error', got: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "secret database path") {
		t.Errorf("want cause NOT in body, but found it: %s", bodyStr)
	}
}

// --- Registration nil-dependency tests ----------------------------------------

func TestRegister_NilHandicapSvcPanics(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when HandicapSvc is nil")
		}
	}()
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: nil})
}

// TestRegister_TypedNilHandicapSvcPanics asserts that a typed nil (a nil concrete
// pointer stored inside the HandicapRecommender interface) is also rejected.
// A typed nil passes the == nil check but panics on the first method call.
func TestRegister_TypedNilHandicapSvcPanics(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when HandicapSvc is a typed nil")
		}
	}()
	mux := http.NewServeMux()
	var svc *stubHandicapSvc // typed nil: interface is non-nil but concrete pointer is nil
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: svc})
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

// ─── Season date normalization ────────────────────────────────────────────────

// seedSeasonWithDate creates a league and season with an explicit start date,
// returning the season ID.
func seedSeasonWithDate(t *testing.T, base, startDate string) int64 {
	t.Helper()
	resp, err := http.Post(base+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Date Test League","game_format":"8ball"}`))
	if err != nil {
		t.Fatalf("POST leagues: %v", err)
	}
	resp.Body.Close()

	body := fmt.Sprintf(`{"league_id":1,"name":"Date Season","start_date":%q}`, startDate)
	resp2, err := http.Post(base+"/api/seasons", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST seasons: %v", err)
	}
	defer resp2.Body.Close()
	var s map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil {
		t.Fatalf("decode season: %v", err)
	}
	return int64(s["id"].(float64))
}

func TestListSeasons_StartDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	const wantDate = "2026-09-01"
	seedSeasonWithDate(t, srv.URL, wantDate)

	resp, err := http.Get(srv.URL + "/api/seasons?league_id=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var seasons []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&seasons); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(seasons) == 0 {
		t.Fatal("no seasons returned")
	}
	got, _ := seasons[0]["start_date"].(string)
	if got != wantDate {
		t.Errorf("start_date: want %q, got %q (must be YYYY-MM-DD for <input type=date>)", wantDate, got)
	}
}

func TestGetSeason_StartDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	const wantDate = "2026-03-15"
	sid := seedSeasonWithDate(t, srv.URL, wantDate)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, _ := s["start_date"].(string)
	if got != wantDate {
		t.Errorf("start_date: want %q, got %q", wantDate, got)
	}
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




// ─── DELETE /api/players/{id} — handicap history guard ──────────────────────

// seedPlayerViaAPI creates a player via the API and returns its numeric ID.
func seedPlayerViaAPI(t *testing.T, base string) int64 {
	t.Helper()
	body := `{"first_name":"Test","last_name":"Player","handicap":1.5,"team_id":null}`
	resp, err := http.Post(base+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/players: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create player: want 201, got %d", resp.StatusCode)
	}
	var p map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode player: %v", err)
	}
	return int64(p["id"].(float64))
}

// insertHandicapHistory inserts a raw handicap_history row directly into the DB.
func insertHandicapHistory(t *testing.T, playerID int64) {
	t.Helper()
	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?, 1.0, 2.0, '2026-01-01')`,
		playerID,
	); err != nil {
		t.Fatalf("insertHandicapHistory: %v", err)
	}
}

// TestDeletePlayer_NoHistory_Succeeds verifies that a player with no
// handicap history records can be deleted normally (200 OK).
func TestDeletePlayer_NoHistory_Succeeds(t *testing.T) {
	srv := testServer(t)
	playerID := seedPlayerViaAPI(t, srv.URL)

	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/players/%d", srv.URL, playerID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 for player with no history, got %d", resp.StatusCode)
	}
}

// TestDeletePlayer_WithHandicapHistory_Returns409 verifies that a player
// with at least one handicap_history row cannot be deleted (409 Conflict).
func TestDeletePlayer_WithHandicapHistory_Returns409(t *testing.T) {
	srv := testServer(t)
	playerID := seedPlayerViaAPI(t, srv.URL)
	insertHandicapHistory(t, playerID)

	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/players/%d", srv.URL, playerID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 for player with history, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(body["error"], "handicap history") {
		t.Errorf("want error message mentioning handicap history, got: %q", body["error"])
	}
}

// TestDeletePlayer_NonExistent_Returns200 verifies that deleting a player
// that doesn't exist still returns 200 (idempotent DELETE).
func TestDeletePlayer_NonExistent_Returns200(t *testing.T) {
	srv := testServer(t)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/players/999999", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (idempotent delete), got %d", resp.StatusCode)
	}
}

// ─── Standings via league_id (FindActiveSeasonByLeague boundary) ──────────────

// TestStandings_LeagueID_NoActiveSeason returns empty standings when there is no
// active season for the given league_id, exercising FindActiveSeasonByLeague.
func TestStandings_LeagueID_NoActiveSeason(t *testing.T) {
	f := weekTestSeed(t)
	// Season is created but never activated — active=0.
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

	// Request standings by league_id — no season_id in query.
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

