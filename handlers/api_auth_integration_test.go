package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"league_app/backend/domains/auth"
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
)

// fastArgon2 keeps these integration tests fast -- never used for a real
// account, only this test server's login flow.
var fastArgon2 = auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

// testServerWithAuth wires the full Users/Roles Phase 1 auth stack
// (real sqlite-backed AuthMgr/RoleAssignmentMgr/AuthUserMgr/APIKeyMgr)
// alongside every existing domain manager, mirroring testServer in
// api_test.go plus the new pieces. InsecureLocalCookies is true because
// httptest.Server serves plain HTTP.
func testServerWithAuth(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
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
	lineupSvc := matches.NewLineupService(lineupStore, roundStore)
	pushbackStore := sqlite.NewPushbackStore(db.DB)
	pushbackSvc := matches.NewPushbackService(pushbackStore)
	financeStore := sqlite.NewFinanceStore(db.DB)
	financeSvc := finances.NewFinanceService(financeStore)

	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	authUserStore := sqlite.NewAuthUserStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	sessionStore := sqlite.NewSessionStore(db.DB)
	setupTokenStore := sqlite.NewPasswordSetupTokenStore(db.DB)
	apiKeyAdminStore := sqlite.NewUserAPIKeyAdminStore(db.DB)
	sessionSvc := auth.NewSessionService(authUserStore, roleStore, sessionStore, setupTokenStore, fastArgon2)

	deps := handlers.Dependencies{
		HandicapSvc: hcSvc, WeekMgr: weekSvc, RoundMgr: roundSvc, RuleMgr: ruleSvc,
		LeagueMgr: leagueSvc, PlayerMgr: playerSvc, TeamMgr: teamSvc, SeasonMgr: seasonSvc,
		ScheduleMgr: scheduleSvc, MatchMgr: matchSvc, LineupMgr: lineupSvc,
		PushbackMgr: pushbackSvc, PushbackApplyMgr: pushbackSvc, FinanceMgr: financeSvc,
		ApplyAuth:            applyAuthStore,
		AuthMgr:              sessionSvc,
		RoleAssignmentMgr:    roleStore,
		AuthUserMgr:          authUserStore,
		APIKeyMgr:            apiKeyAdminStore,
		LeagueSelfGrantMgr:   sqlite.NewLeagueSelfGrantStore(db.DB),
		InsecureLocalCookies: true,
	}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// --- small test-only HTTP helpers ------------------------------------------

func authDo(t *testing.T, srv *httptest.Server, method, path, body string, cookies []*http.Cookie, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

// loginCookies logs in via POST /api/auth/login and returns the session
// and CSRF cookies the server set, plus the raw CSRF cookie value (for
// attaching as the X-CSRF-Token header on subsequent mutating requests).
func loginCookies(t *testing.T, srv *httptest.Server, email, password string) ([]*http.Cookie, string) {
	t.Helper()
	resp := authDo(t, srv, "POST", "/api/auth/login", `{"email":"`+email+`","password":"`+password+`"}`, nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: want 200, got %d", email, resp.StatusCode)
	}
	var cookies []*http.Cookie
	var csrf string
	for _, c := range resp.Cookies() {
		cookies = append(cookies, c)
		if c.Name == "csrf_token" {
			csrf = c.Value
		}
	}
	if csrf == "" {
		t.Fatal("want a csrf_token cookie set on login")
	}
	return cookies, csrf
}

// seedPasswordUser creates a user directly via the auth stores (bootstrap
// path: this phase has no self-registration, so a test/ops flow creating
// the very first accounts this way is expected) with a real Argon2id
// password hash and, optionally, a role assignment.
func seedPasswordUser(t *testing.T, email, password string, roleCode auth.RoleCode, leagueID *int64) int64 {
	t.Helper()
	authUserStore := sqlite.NewAuthUserStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	ctx := context.Background()

	normalized, err := auth.NormalizeEmail(email)
	if err != nil {
		t.Fatalf("NormalizeEmail: %v", err)
	}
	u, err := authUserStore.ProvisionUser(ctx, strings.SplitN(normalized, "@", 2)[0], normalized, nil)
	if err != nil {
		t.Fatalf("ProvisionUser: %v", err)
	}
	hash, err := auth.HashPassword(password, fastArgon2)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := authUserStore.SetPassword(ctx, u.ID, hash); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if roleCode != "" {
		if err := roleStore.Grant(ctx, u.ID, roleCode, leagueID, nil); err != nil {
			t.Fatalf("Grant %s: %v", roleCode, err)
		}
	}
	return u.ID
}

func seedTestLeague(t *testing.T, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO leagues (name) VALUES (?)`, name)
	if err != nil {
		t.Fatalf("seed league %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// --- tests ------------------------------------------------------------

func TestAuthIntegration_LoginSetsSessionAndCSRFCookies(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleSystemAdmin, nil)

	resp := authDo(t, srv, "POST", "/api/auth/login", `{"email":"admin@example.com","password":"hunter22"}`, nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var sawSession, sawCSRF bool
	for _, c := range resp.Cookies() {
		if c.Name == "session_token" {
			sawSession = true
			if !c.HttpOnly {
				t.Error("want session_token cookie HttpOnly")
			}
		}
		if c.Name == "csrf_token" {
			sawCSRF = true
			if c.HttpOnly {
				t.Error("want csrf_token cookie NOT HttpOnly (must be JS-readable)")
			}
		}
	}
	if !sawSession || !sawCSRF {
		t.Fatalf("want both session_token and csrf_token cookies set, session=%v csrf=%v", sawSession, sawCSRF)
	}
}

func TestAuthIntegration_LoginWrongPassword_GenericError(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleSystemAdmin, nil)

	resp := authDo(t, srv, "POST", "/api/auth/login", `{"email":"admin@example.com","password":"wrong"}`, nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
	body := decodeJSON[map[string]string](t, resp)
	if !strings.Contains(strings.ToLower(body["error"]), "invalid email or password") {
		t.Errorf("want a generic error message, got %q", body["error"])
	}
}

func TestAuthIntegration_MeReflectsIdentity(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, srv, "admin@example.com", "hunter22")

	resp := authDo(t, srv, "GET", "/api/auth/me", "", cookies, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
		RoleAssignments []map[string]any `json:"role_assignments"`
		Workspaces      []string         `json:"workspaces"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.User.Email != "admin@example.com" {
		t.Errorf("want email admin@example.com, got %q", body.User.Email)
	}
	if len(body.RoleAssignments) != 1 {
		t.Errorf("want 1 role assignment, got %d", len(body.RoleAssignments))
	}
	found := false
	for _, w := range body.Workspaces {
		if w == "admin" {
			found = true
		}
	}
	if !found {
		t.Errorf("want 'admin' workspace available, got %v", body.Workspaces)
	}
}

func TestAuthIntegration_LogoutRevokesSessionButNotOthers(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleSystemAdmin, nil)

	cookiesA, _ := loginCookies(t, srv, "admin@example.com", "hunter22")
	cookiesB, _ := loginCookies(t, srv, "admin@example.com", "hunter22")

	resp := authDo(t, srv, "POST", "/api/auth/logout", "", cookiesA, nil)
	resp.Body.Close()

	respA := authDo(t, srv, "GET", "/api/auth/me", "", cookiesA, nil)
	respA.Body.Close()
	if respA.StatusCode != http.StatusUnauthorized {
		t.Errorf("want session A to be logged out (401), got %d", respA.StatusCode)
	}

	respB := authDo(t, srv, "GET", "/api/auth/me", "", cookiesB, nil)
	respB.Body.Close()
	if respB.StatusCode != http.StatusOK {
		t.Errorf("want session B to remain valid (logout affects only the current session), got %d", respB.StatusCode)
	}
}

func TestAuthIntegration_LeagueCreate_RequiresCSRFForSessionAuth(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, csrf := loginCookies(t, srv, "admin@example.com", "hunter22")

	// Without the CSRF header: rejected.
	resp := authDo(t, srv, "POST", "/api/leagues", `{"name":"CSRF Test League","game_format":"8ball"}`, cookies, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 without CSRF header, got %d", resp.StatusCode)
	}

	// With the correct CSRF header: succeeds.
	resp2 := authDo(t, srv, "POST", "/api/leagues", `{"name":"CSRF Test League","game_format":"8ball"}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("want 201 with correct CSRF header, got %d", resp2.StatusCode)
	}
}

func TestAuthIntegration_LeagueCreate_WrongCSRFRejected(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, srv, "admin@example.com", "hunter22")

	resp := authDo(t, srv, "POST", "/api/leagues", `{"name":"CSRF Test League 2","game_format":"8ball"}`, cookies, map[string]string{"X-CSRF-Token": "not-the-real-token"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 with a wrong CSRF token, got %d", resp.StatusCode)
	}
}

func TestAuthIntegration_LeagueCreate_AtomicSelfGrant(t *testing.T) {
	srv := testServerWithAuth(t)
	seedTestLeague(t, "Seed League So League Admin Can Exist") // system_admin needs no seed league, but keep ids realistic
	userID := seedPasswordUser(t, "newadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	_ = userID
	cookies, csrf := loginCookies(t, srv, "newadmin@example.com", "hunter22")

	resp := authDo(t, srv, "POST", "/api/leagues", `{"name":"Atomic Grant League","game_format":"8ball"}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	created := decodeJSON[map[string]any](t, resp)
	newLeagueID := int64(created["id"].(float64))

	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	assignments, err := roleStore.ListForUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	foundGrant := false
	for _, a := range assignments {
		if a.RoleCode == auth.RoleLeagueAdmin && a.LeagueID != nil && *a.LeagueID == newLeagueID {
			foundGrant = true
		}
	}
	if !foundGrant {
		t.Errorf("want the creator (already system_admin) to ALSO receive an explicit league_admin grant for the new league %d, got %+v", newLeagueID, assignments)
	}
}

func TestAuthIntegration_LeagueAdmin_ScopedToOwnLeagueOnly(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "League A")
	leagueB := seedTestLeague(t, "League B")
	seedPasswordUser(t, "la@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueA)
	cookies, csrf := loginCookies(t, srv, "la@example.com", "hunter22")
	headers := map[string]string{"X-CSRF-Token": csrf}

	// Allowed: create a season in League A.
	respA := authDo(t, srv, "POST", "/api/seasons", `{"league_id":`+strconv.FormatInt(leagueA, 10)+`,"name":"Season in A"}`, cookies, headers)
	defer respA.Body.Close()
	if respA.StatusCode != http.StatusCreated {
		t.Fatalf("want 201 creating a season in the admin's own league A, got %d", respA.StatusCode)
	}

	// Denied: create a season in League B.
	respB := authDo(t, srv, "POST", "/api/seasons", `{"league_id":`+strconv.FormatInt(leagueB, 10)+`,"name":"Season in B"}`, cookies, headers)
	defer respB.Body.Close()
	if respB.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 creating a season in a league this admin is NOT scoped to, got %d", respB.StatusCode)
	}

	// Denied: update League B's own metadata directly.
	respUpd := authDo(t, srv, "PUT", "/api/leagues/"+strconv.FormatInt(leagueB, 10), `{"name":"Renamed B","game_format":"8ball"}`, cookies, headers)
	defer respUpd.Body.Close()
	if respUpd.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 updating a different league, got %d", respUpd.StatusCode)
	}

	// Denied: system-admin-only actions.
	respSys := authDo(t, srv, "POST", "/api/auth/admin/users", `{"email":"someone@example.com"}`, cookies, headers)
	defer respSys.Body.Close()
	if respSys.StatusCode != http.StatusForbidden {
		t.Fatalf("want league_admin denied system_user_admin action, got %d", respSys.StatusCode)
	}
}

func TestAuthIntegration_PlayerCannotCallAdminMutationRoute(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "League A")
	userID := seedPasswordUser(t, "player@example.com", "hunter22", "", nil)
	// Link this user to a player record directly (no self-registration in
	// this phase) -- a plain SQL insert + link, matching how a system_admin
	// would provision a player-linked account.
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Test', 'Player')`)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	playerID, _ := res.LastInsertId()
	if _, err := db.DB.Exec(`UPDATE users SET player_id=? WHERE id=?`, playerID, userID); err != nil {
		t.Fatalf("link player: %v", err)
	}

	cookies, csrf := loginCookies(t, srv, "player@example.com", "hunter22")
	resp := authDo(t, srv, "POST", "/api/seasons", `{"league_id":`+strconv.FormatInt(leagueA, 10)+`,"name":"Player Attempt"}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 for a plain player attempting an admin mutation route, got %d", resp.StatusCode)
	}
}

func TestAuthIntegration_Deactivation_InvalidatesSessionImmediately(t *testing.T) {
	srv := testServerWithAuth(t)
	userID := seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, srv, "admin@example.com", "hunter22")

	resp := authDo(t, srv, "GET", "/api/auth/me", "", cookies, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 before deactivation, got %d", resp.StatusCode)
	}

	sessionSvc := auth.NewSessionService(
		sqlite.NewAuthUserStore(db.DB), sqlite.NewRoleAssignmentStore(db.DB),
		sqlite.NewSessionStore(db.DB), sqlite.NewPasswordSetupTokenStore(db.DB), fastArgon2,
	)
	if err := sessionSvc.Deactivate(context.Background(), userID, sqlite.NewUserAPIKeyAdminStore(db.DB)); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	resp2 := authDo(t, srv, "GET", "/api/auth/me", "", cookies, nil)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 immediately after deactivation, got %d", resp2.StatusCode)
	}
}

func TestAuthIntegration_ExistingAPIKeyStillWorksAfterMigration(t *testing.T) {
	srv := testServerWithAuth(t)
	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	_, cleartext, err := applyAuthStore.CreateApplyUser(context.Background(), "legacy-admin", "system_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	resp := authDo(t, srv, "POST", "/api/leagues", `{"name":"Legacy Key League","game_format":"8ball"}`, nil, map[string]string{"Authorization": "Bearer " + cleartext})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201 creating a league with a legacy Bearer API key (no CSRF header needed), got %d", resp.StatusCode)
	}
}
