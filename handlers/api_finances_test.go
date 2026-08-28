// Tests for the Financial Phase 1 routes:
//
//	GET  /api/seasons/{id}/finances/dues
//	POST /api/seasons/{id}/finances/dues-payments
//	GET  /api/seasons/{id}/finances/payouts
//	POST /api/seasons/{id}/finances/payouts
//
// Per PM decision, ALL FOUR routes (reads and writes) require personal-key
// auth with league_admin/admin/system_admin -- unlike most other domains,
// finance GETs are not open reads.
package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domains/finances"
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

// financeTestServer builds the full stack, including FinanceMgr and a real
// ApplyAuthStore, so clearanceAuth genuinely enforces role checks (unlike
// the plain testServer() helper, which leaves ApplyAuth nil and so takes
// clearanceAuth's nil-resolver passthrough).
func financeTestServer(t *testing.T) (*httptest.Server, *sqlite.ApplyAuthStore) {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	mux := http.NewServeMux()
	hcStore := sqlite.NewHandicapStore(db.DB)
	hcSvc := handicaps.NewService(hcStore)
	authStore := sqlite.NewApplyAuthStore(db.DB)
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
	matchStore := sqlite.NewMatchStore(db.DB)
	matchSvc := matches.NewMatchService(matchStore)
	financeStore := sqlite.NewFinanceStore(db.DB)
	financeSvc := finances.NewFinanceService(financeStore)
	deps := handlers.Dependencies{
		HandicapSvc:     hcSvc,
		HandicapApplier: hcSvc,
		AdminToken:      "finance-test-admin-token",
		ApplyAuth:       authStore,
		WeekMgr:         weekSvc,
		RoundMgr:        roundSvc,
		RuleMgr:         ruleSvc,
		LeagueMgr:       leagueSvc,
		PlayerMgr:       playerSvc,
		TeamMgr:         teamSvc,
		SeasonMgr:       seasonSvc,
		MatchMgr:        matchSvc,
		FinanceMgr:      financeSvc,
	}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, authStore
}

// financeSeed creates a league, an active season, one team registered for
// the season (season_teams), and one player rostered on that team for the
// season (season_rosters). Returns (seasonID, teamID, playerID).
func financeSeed(t *testing.T) (seasonID, teamID, playerID int64) {
	t.Helper()
	d := db.DB

	r, err := d.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('Finance League','8ball','Monday')`)
	if err != nil {
		t.Fatalf("seed league: %v", err)
	}
	lgID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?,?,?,?,?)`,
		lgID, "Finance Season", "2026-01-01", "single_rr", 3)
	if err != nil {
		t.Fatalf("seed season: %v", err)
	}
	seasonID, _ = r.LastInsertId()

	r, err = d.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Finance Team')`, lgID)
	if err != nil {
		t.Fatalf("seed team: %v", err)
	}
	teamID, _ = r.LastInsertId()

	if _, err := d.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Finance Team')`, seasonID, teamID); err != nil {
		t.Fatalf("seed season_teams: %v", err)
	}

	r, err = d.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Pat','Payer',?,2.0)`, teamID)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	playerID, _ = r.LastInsertId()

	if _, err := d.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, seasonID, teamID, playerID); err != nil {
		t.Fatalf("seed season_rosters: %v", err)
	}

	return seasonID, teamID, playerID
}

// createFinanceUser bootstraps a personal-key user with the given role via
// the static admin token and returns the cleartext key.
func createFinanceUser(t *testing.T, base, role string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/api/users",
		strings.NewReader(fmt.Sprintf(`{"username":"finance-%s","role":"%s"}`, role, role)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer finance-test-admin-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: want 201, got %d", resp.StatusCode)
	}
	var got models.CreateUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode create user: %v", err)
	}
	return got.APIKey
}

// -- Auth: GET /finances/dues -------------------------------------------------

func TestFinanceDues_NoToken_Returns401(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, _ := financeSeed(t)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/finances/dues", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestFinanceDues_WrongToken_Returns403(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, _ := financeSeed(t)

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/seasons/%d/finances/dues", srv.URL, seasonID), nil)
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

func TestFinanceDues_LeagueAdminKey_Returns200(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, _ := financeSeed(t)
	key := createFinanceUser(t, srv.URL, "league_admin")

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/seasons/%d/finances/dues", srv.URL, seasonID), nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (league_admin can read dues), got %d: %s", resp.StatusCode, resp.Status)
	}
}

// -- Auth: POST /finances/dues-payments --------------------------------------

func TestPostDuesPayment_NoToken_Returns401(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	resp, err := http.Post(fmt.Sprintf("%s/api/seasons/%d/finances/dues-payments", srv.URL, seasonID),
		"application/json", strings.NewReader(fmt.Sprintf(`{"player_id":%d,"amount":25,"paid_at":"2026-01-05"}`, playerID)))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// -- Auth: GET /finances/payouts ----------------------------------------------

func TestFinancePayouts_NoToken_Returns401(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, _ := financeSeed(t)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/finances/payouts", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// -- Auth: POST /finances/payouts ---------------------------------------------

func TestPostPayout_NoToken_Returns401(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, teamID, _ := financeSeed(t)

	resp, err := http.Post(fmt.Sprintf("%s/api/seasons/%d/finances/payouts", srv.URL, seasonID),
		"application/json", strings.NewReader(fmt.Sprintf(`{"team_id":%d,"amount":100}`, teamID)))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// -- Success path: dues --------------------------------------------------------

func TestFinanceDues_SuccessPath_RosterPlayerStartsUnpaidThenPaid(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, teamID, playerID := financeSeed(t)
	key := createFinanceUser(t, srv.URL, "league_admin")

	getDues := func() models.SeasonDuesResponse {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/seasons/%d/finances/dues", srv.URL, seasonID), nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get dues: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get dues: want 200, got %d", resp.StatusCode)
		}
		var got models.SeasonDuesResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode dues: %v", err)
		}
		return got
	}

	before := getDues()
	if len(before.Players) != 1 {
		t.Fatalf("want 1 rostered player, got %d", len(before.Players))
	}
	if before.Players[0].Paid {
		t.Error("want paid=false before any payment recorded")
	}
	if before.Players[0].PlayerID != playerID || before.Players[0].TeamID != teamID {
		t.Errorf("want player_id=%d team_id=%d, got %+v", playerID, teamID, before.Players[0])
	}

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/seasons/%d/finances/dues-payments", srv.URL, seasonID),
		strings.NewReader(fmt.Sprintf(`{"player_id":%d,"amount":25,"paid_at":"2026-01-05","note":"cash"}`, playerID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post payment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post payment: want 201, got %d", resp.StatusCode)
	}
	var payment models.DuesPayment
	if err := json.NewDecoder(resp.Body).Decode(&payment); err != nil {
		t.Fatalf("decode payment: %v", err)
	}
	if payment.TeamID == nil || *payment.TeamID != teamID {
		t.Errorf("want team_id=%d denormalized onto payment, got %v", teamID, payment.TeamID)
	}

	after := getDues()
	if !after.Players[0].Paid {
		t.Error("want paid=true after payment recorded")
	}
	if after.Players[0].TotalPaid != 25 {
		t.Errorf("want total_paid=25, got %v", after.Players[0].TotalPaid)
	}
	if len(after.Players[0].Payments) != 1 {
		t.Fatalf("want 1 payment in history, got %d", len(after.Players[0].Payments))
	}
}

func TestPostDuesPayment_PlayerNotRostered_Returns404(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, _ := financeSeed(t)
	key := createFinanceUser(t, srv.URL, "league_admin")

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/seasons/%d/finances/dues-payments", srv.URL, seasonID),
		strings.NewReader(`{"player_id":9999999,"amount":25,"paid_at":"2026-01-05"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 (player not rostered for this season), got %d", resp.StatusCode)
	}
}

func TestPostDuesPayment_InvalidAmount_Returns400(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)
	key := createFinanceUser(t, srv.URL, "league_admin")

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/seasons/%d/finances/dues-payments", srv.URL, seasonID),
		strings.NewReader(fmt.Sprintf(`{"player_id":%d,"amount":0,"paid_at":"2026-01-05"}`, playerID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 (amount must be positive), got %d", resp.StatusCode)
	}
}

// -- Success path: payouts -----------------------------------------------------

func TestFinancePayouts_SuccessPath_RecordAndList(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, teamID, _ := financeSeed(t)
	key := createFinanceUser(t, srv.URL, "league_admin")

	getPayouts := func() models.SeasonPayoutsResponse {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/seasons/%d/finances/payouts", srv.URL, seasonID), nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get payouts: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get payouts: want 200, got %d", resp.StatusCode)
		}
		var got models.SeasonPayoutsResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode payouts: %v", err)
		}
		return got
	}

	before := getPayouts()
	if len(before.Teams) != 1 {
		t.Fatalf("want 1 season team, got %d", len(before.Teams))
	}
	if before.Teams[0].TotalPaid != 0 || len(before.Teams[0].Payouts) != 0 {
		t.Errorf("want no payouts yet, got %+v", before.Teams[0])
	}

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/seasons/%d/finances/payouts", srv.URL, seasonID),
		strings.NewReader(fmt.Sprintf(`{"team_id":%d,"amount":150,"note":"1st place"}`, teamID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post payout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post payout: want 201, got %d", resp.StatusCode)
	}

	after := getPayouts()
	if after.Teams[0].TotalPaid != 150 {
		t.Errorf("want total_paid=150, got %v", after.Teams[0].TotalPaid)
	}
	if len(after.Teams[0].Payouts) != 1 {
		t.Fatalf("want 1 payout in history, got %d", len(after.Teams[0].Payouts))
	}
}

func TestPostPayout_TeamNotInSeason_Returns404(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, _ := financeSeed(t)
	key := createFinanceUser(t, srv.URL, "league_admin")

	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/seasons/%d/finances/payouts", srv.URL, seasonID),
		strings.NewReader(`{"team_id":9999999,"amount":100}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 (team not part of this season), got %d", resp.StatusCode)
	}
}
