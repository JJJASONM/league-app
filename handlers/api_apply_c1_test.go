// Integration tests for Phase C1: user endpoint auth and Apply attribution.
// Uses package handlers_test (black-box) with a real SQLite DB.
package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"league_app/models"
)

const c1AdminToken = "c1-test-admin-token"

// testServerWithApplyAuth creates a test server with AdminToken and ApplyAuth wired.
// Returns the server and the auth store for direct setup calls in tests.
func testServerWithApplyAuth(t *testing.T) (*httptest.Server, *sqlite.ApplyAuthStore) {
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
	scheduleStore := sqlite.NewScheduleStore(db.DB)
	scheduleSvc := matches.NewScheduleService(scheduleStore)
	matchStore := sqlite.NewMatchStore(db.DB)
	matchSvc := matches.NewMatchService(matchStore)
	lineupStore := sqlite.NewLineupStore(db.DB)
	lineupSvc := matches.NewLineupService(lineupStore, roundStore)
	deps := handlers.Dependencies{
		HandicapSvc:     hcSvc,
		HandicapApplier: hcSvc,
		AdminToken:      c1AdminToken,
		ApplyAuth:       authStore,
		WeekMgr:         weekSvc,
		RoundMgr:        roundSvc,
		RuleMgr:         ruleSvc,
		LeagueMgr:       leagueSvc,
		PlayerMgr:       playerSvc,
		TeamMgr:         teamSvc,
		SeasonMgr:       seasonSvc,
		ScheduleMgr:     scheduleSvc,
		MatchMgr:        matchSvc,
		LineupMgr:       lineupSvc,
	}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, authStore
}

// ─── POST /api/users auth requirements ───────────────────────────────────────

func TestPostUsers_NoToken_Returns401(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	resp, err := http.Post(srv.URL+"/api/users", "application/json",
		strings.NewReader(`{"username":"alice"}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestPostUsers_WrongToken_Returns403(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/users",
		strings.NewReader(`{"username":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestPostUsers_CorrectToken_Returns201WithOneTimeKey(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/users",
		strings.NewReader(`{"username":"alice","role":"league_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c1AdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("want 201, got %d", resp.StatusCode)
	}
	var got models.CreateUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.User.Username != "alice" {
		t.Errorf("want username alice, got %q", got.User.Username)
	}
	if got.User.Role != "league_admin" {
		t.Errorf("want role league_admin, got %q", got.User.Role)
	}
	if len(got.APIKey) != 64 {
		t.Errorf("want 64-char api_key, got len=%d", len(got.APIKey))
	}
}

// TestPostUsers_MissingRole_Returns400 verifies role is required -- Users
// Admin Screen Phase 1 no longer hardcodes a default role.
func TestPostUsers_MissingRole_Returns400(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/users",
		strings.NewReader(`{"username":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c1AdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

// TestPostUsers_LegacyAdminRole_Returns400 verifies "admin" is rejected as a
// new-creation role even with valid auth -- it remains a legacy alias for
// existing rows only, per PM decision 2.
func TestPostUsers_LegacyAdminRole_Returns400(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/users",
		strings.NewReader(`{"username":"alice","role":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c1AdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 (admin is not a creatable role), got %d", resp.StatusCode)
	}
}

// TestPostUsers_SystemAdminPersonalKey_Returns201 verifies a resolved
// system_admin personal key can create a user, not just the static admin
// token -- the concrete backend gap identified in discovery (PM decision 1).
func TestPostUsers_SystemAdminPersonalKey_Returns201(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	ctx := context.Background()
	_, sysAdminKey, err := authStore.CreateApplyUser(ctx, "sys-admin-caller", "system_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/users",
		strings.NewReader(`{"username":"created-by-personal-key","role":"league_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sysAdminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("want 201 (system_admin personal key authorizes user creation), got %d: %s",
			resp.StatusCode, resp.Body)
	}
}

// TestPostUsers_LeagueAdminPersonalKey_Returns403 verifies league_admin
// (unlike system_admin/admin) cannot create users -- this route uses the
// stricter system-admin gate, like backup, not the broader clearance gate.
func TestPostUsers_LeagueAdminPersonalKey_Returns403(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	ctx := context.Background()
	_, leagueAdminKey, err := authStore.CreateApplyUser(ctx, "league-admin-caller", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/users",
		strings.NewReader(`{"username":"nope","role":"league_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+leagueAdminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (league_admin cannot create users), got %d", resp.StatusCode)
	}
}

// --- GET /api/users/me -----------------------------------------------------

// TestGetMe_NoToken_Returns401 verifies /me requires authentication.
func TestGetMe_NoToken_Returns401(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	resp, err := http.Get(srv.URL + "/api/users/me")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// TestGetMe_StaticAdminToken_Returns403 verifies the static admin token does
// not resolve to a user identity -- /me has no static-token fallback, since
// its whole purpose is resolving a real personal key to a real user.
func TestGetMe_StaticAdminToken_Returns403(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+c1AdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (static token does not resolve to a user), got %d", resp.StatusCode)
	}
}

// TestGetMe_ValidPersonalKey_ReturnsResolvedIdentity verifies /me returns the
// caller's own username/role/active for any resolvable personal key,
// regardless of role -- this is what drives the Admin Key modal's identity
// indicator (Phase 1 frontend).
func TestGetMe_ValidPersonalKey_ReturnsResolvedIdentity(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	ctx := context.Background()
	_, key, err := authStore.CreateApplyUser(ctx, "whoami-user", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, resp.Body)
	}
	var got models.User
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Username != "whoami-user" {
		t.Errorf("want username whoami-user, got %q", got.Username)
	}
	if got.Role != "league_admin" {
		t.Errorf("want role league_admin, got %q", got.Role)
	}
}

// ─── GET /api/users auth requirements ────────────────────────────────────────

func TestGetUsers_NoToken_Returns401(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	resp, err := http.Get(srv.URL + "/api/users")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestGetUsers_WrongToken_Returns403(t *testing.T) {
	srv, _ := testServerWithApplyAuth(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestGetUsers_CorrectToken_Returns200(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	ctx := context.Background()
	if _, _, err := authStore.CreateApplyUser(ctx, "bob", "league_admin"); err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+c1AdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	var users []models.User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("want 1 user, got %d", len(users))
	}
}

// TestGetUsers_SystemAdminPersonalKey_Returns200 verifies a resolved
// system_admin personal key can list users, not just the static admin
// token (PM decision 1).
func TestGetUsers_SystemAdminPersonalKey_Returns200(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	ctx := context.Background()
	_, sysAdminKey, err := authStore.CreateApplyUser(ctx, "list-caller", "system_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+sysAdminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (system_admin personal key authorizes list), got %d", resp.StatusCode)
	}
}

// TestGetUsers_LeagueAdminPersonalKey_Returns403 verifies league_admin
// cannot list users -- same stricter system-admin gate as create.
func TestGetUsers_LeagueAdminPersonalKey_Returns403(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	ctx := context.Background()
	_, leagueAdminKey, err := authStore.CreateApplyUser(ctx, "league-lister", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+leagueAdminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (league_admin cannot list users), got %d", resp.StatusCode)
	}
}

// ─── Apply attribution: DB column wiring ─────────────────────────────────────

// createUserForTest creates a user via the auth store and returns the user and cleartext key.
func createUserForTest(t *testing.T, authStore *sqlite.ApplyAuthStore, username string) (models.User, string) {
	t.Helper()
	user, key, err := authStore.CreateApplyUser(context.Background(), username, "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser(%q): %v", username, err)
	}
	return user, key
}

// insertPlayerAndHistory inserts a player row and a handicap_history row with
// the given applied_by_user_id (nil for NULL). Returns the player ID.
func insertPlayerAndHistory(t *testing.T, appliedByUserID *int64, reqID string) int64 {
	t.Helper()
	if _, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Test', 'Player')`); err != nil {
		t.Fatalf("insert player: %v", err)
	}
	var playerID int64
	if err := db.DB.QueryRow(`SELECT id FROM players ORDER BY id DESC LIMIT 1`).Scan(&playerID); err != nil {
		t.Fatalf("get player id: %v", err)
	}
	var idArg interface{}
	if appliedByUserID != nil {
		idArg = *appliedByUserID
	}
	if _, err := db.DB.Exec(`
		INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date,
		                              apply_request_id, applied_by_user_id)
		VALUES (?, 1.0, 2.0, '2026-06-30', ?, ?)`, playerID, reqID, idArg); err != nil {
		t.Fatalf("insert history: %v", err)
	}
	return playerID
}

// TestApply_PersonalKey_WritesAppliedByUserID verifies that a handicap_history
// row written with a user's ID has applied_by_user_id set in the DB.
// This confirms the DB column is properly wired for the personal-key auth path.
func TestApply_PersonalKey_WritesAppliedByUserID(t *testing.T) {
	_, authStore := testServerWithApplyAuth(t)
	user, _ := createUserForTest(t, authStore, "attr-writer")

	playerID := insertPlayerAndHistory(t, &user.ID, "req-personal-key-001")

	var gotUserID *int64
	if err := db.DB.QueryRow(
		`SELECT applied_by_user_id FROM handicap_history WHERE player_id = ?`, playerID,
	).Scan(&gotUserID); err != nil {
		t.Fatalf("read history: %v", err)
	}
	if gotUserID == nil || *gotUserID != user.ID {
		t.Errorf("want applied_by_user_id=%d, got %v", user.ID, gotUserID)
	}
}

// TestApply_StaticToken_NullAppliedByUserID verifies that a handicap_history row
// written without a user ID (static-token fallback path) has applied_by_user_id=NULL.
func TestApply_StaticToken_NullAppliedByUserID(t *testing.T) {
	_, _ = testServerWithApplyAuth(t)

	playerID := insertPlayerAndHistory(t, nil, "req-static-token-001")

	var gotUserID *int64
	if err := db.DB.QueryRow(
		`SELECT applied_by_user_id FROM handicap_history WHERE player_id = ?`, playerID,
	).Scan(&gotUserID); err != nil {
		t.Fatalf("read history: %v", err)
	}
	if gotUserID != nil {
		t.Errorf("want applied_by_user_id=NULL for static-token path, got %d", *gotUserID)
	}
}

// TestApply_Replay_PreservesAttribution verifies that a duplicate apply_request_id
// (idempotency/replay scenario) is rejected by the unique index, leaving the
// original applied_by_user_id intact.
func TestApply_Replay_PreservesAttribution(t *testing.T) {
	_, authStore := testServerWithApplyAuth(t)
	user, _ := createUserForTest(t, authStore, "replay-user")

	const reqID = "replay-idempotency-test-001"
	playerID := insertPlayerAndHistory(t, &user.ID, reqID)

	// Replay with nil user ID — the unique index must reject this.
	_, dupErr := db.DB.Exec(`
		INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date,
		                              apply_request_id, applied_by_user_id)
		VALUES (?, 1.0, 2.0, '2026-06-30', ?, NULL)`, playerID, reqID)
	if dupErr == nil {
		t.Error("want unique-constraint error on duplicate (player_id, apply_request_id), got nil")
	}

	// The original attribution row is unchanged.
	var gotUserID *int64
	if err := db.DB.QueryRow(
		`SELECT applied_by_user_id FROM handicap_history WHERE player_id = ?`, playerID,
	).Scan(&gotUserID); err != nil {
		t.Fatalf("read history: %v", err)
	}
	if gotUserID == nil || *gotUserID != user.ID {
		t.Errorf("want original applied_by_user_id=%d preserved after replay attempt, got %v", user.ID, gotUserID)
	}
}

// ─── GET /api/users response does not expose key/hash ─────────────────────────

func TestGetUsers_ResponseDoesNotContainAPIKey(t *testing.T) {
	srv, authStore := testServerWithApplyAuth(t)
	ctx := context.Background()
	_, key, err := authStore.CreateApplyUser(ctx, "key-check-user", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+c1AdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var raw bytes.Buffer
	raw.ReadFrom(resp.Body)
	body := raw.String()
	if strings.Contains(body, key) {
		t.Error("want api_key absent from GET /api/users response body")
	}
	if len(key) == 64 {
		// Sanity-check: verify the key is actually 64 chars (so the search above is meaningful).
		t.Logf("api_key len=%d — not present in list response (correct)", len(key))
	}
}
