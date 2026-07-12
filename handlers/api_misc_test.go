package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/leagues"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/players"
	"league_app/backend/domains/seasons"
	"league_app/backend/domains/teams"
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
func (n *noopSeasonMgr) DeleteSkippedWeek(_ context.Context, _, _ int64) error { return nil }
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

// --- Registration nil-dependency tests ---

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

// --- Season date normalization ---

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
