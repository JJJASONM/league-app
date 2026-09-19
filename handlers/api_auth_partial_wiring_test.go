package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/leagues"
	"league_app/backend/domains/seasons"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/handlers"
	"league_app/models"
)

// stubHandicapRecommender, noopLeagueMgr, noopRuleMgr, noopPlayerMgr,
// noopTeamMgr, and noopSeasonMgr are all defined once in api_misc_test.go,
// in this same package (handlers_test), and reused here directly.

// This file covers PM's correction item 3: guardedAction previously fell
// back to "fully open" whenever deps.ApplyAuth was nil, regardless of
// whether AuthMgr/RoleAssignmentMgr were wired -- a session-only
// configuration (a real, if partial, auth stack) was silently exposing
// every mutation route with no authentication at all. The fix gates the
// fully-open fallback on ALL THREE of ApplyAuth/AuthMgr/RoleAssignmentMgr
// being nil; wiring any one of them is now treated as a real auth
// configuration that must fail closed.

// testServerSessionOnlyNoApplyAuth wires the real session/role-assignment
// auth stack (AuthMgr, RoleAssignmentMgr) exactly like testServerWithAuth,
// but deliberately leaves ApplyAuth nil -- the "AuthMgr and
// RoleAssignmentMgr wired, ApplyAuth absent" condition PM's item 3
// requires to still enforce session authentication rather than fall open.
func testServerSessionOnlyNoApplyAuth(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	mux := http.NewServeMux()
	leagueStore := sqlite.NewLeagueStore(db.DB)
	leagueSvc := leagues.NewLeagueService(leagueStore)

	authUserStore := sqlite.NewAuthUserStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	sessionStore := sqlite.NewSessionStore(db.DB)
	setupTokenStore := sqlite.NewPasswordSetupTokenStore(db.DB)
	sessionSvc := auth.NewSessionService(authUserStore, roleStore, sessionStore, setupTokenStore, fastArgon2)

	deps := handlers.Dependencies{
		HandicapSvc: &stubHandicapSvc{fn: func(context.Context, int64) (models.HandicapReviewResponse, error) {
			return models.HandicapReviewResponse{}, nil
		}},
		RuleMgr:   &noopRuleMgr{},
		LeagueMgr: leagueSvc,
		PlayerMgr: &noopPlayerMgr{},
		TeamMgr:   &noopTeamMgr{},
		SeasonMgr: &noopSeasonMgr{},

		// ApplyAuth deliberately left nil -- session support must still work.
		AuthMgr:              sessionSvc,
		RoleAssignmentMgr:    roleStore,
		LeagueSelfGrantMgr:   sqlite.NewLeagueSelfGrantStore(db.DB),
		InsecureLocalCookies: true,
	}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestPartialWiring_SessionOnlyNoApplyAuth_AuthorizedSessionSucceeds(t *testing.T) {
	srv := testServerSessionOnlyNoApplyAuth(t)
	seedPasswordUser(t, "partial-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, csrf := loginCookies(t, srv, "partial-sysadmin@example.com", "hunter22")

	resp := authDo(t, srv, "POST", "/api/leagues", `{"name":"Partial Wiring League","game_format":"8ball"}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201 for a session-authenticated system_admin with no ApplyAuth wired, got %d", resp.StatusCode)
	}
}

func TestPartialWiring_SessionOnlyNoApplyAuth_NoCredential_Returns401(t *testing.T) {
	srv := testServerSessionOnlyNoApplyAuth(t)

	resp := authDo(t, srv, "POST", "/api/leagues", `{"name":"No Credential League","game_format":"8ball"}`, nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 with no credential at all (not fully open), got %d", resp.StatusCode)
	}
}

func TestPartialWiring_SessionOnlyNoApplyAuth_BearerKeyCannotAuthenticate(t *testing.T) {
	srv := testServerSessionOnlyNoApplyAuth(t)

	// No ApplyAuth resolver is wired, so a Bearer key can never resolve --
	// this must be rejected, not silently treated as "no credential
	// presented" and let through.
	resp := authDo(t, srv, "POST", "/api/leagues", `{"name":"Bearer Attempt League","game_format":"8ball"}`, nil, map[string]string{"Authorization": "Bearer some-key"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 for an unresolvable Bearer key when ApplyAuth is unwired, got %d", resp.StatusCode)
	}
}

// TestRegister_ApplyRoute_Mounted_WhenSessionAuthOnly_NoToken proves PM's
// item 4: the Apply route (POST /api/seasons/{id}/handicap-apply) must
// mount whenever session/scoped-role authentication is available, even
// with no LEAGUE_ADMIN_TOKEN configured and no ApplyAuth (Bearer) resolver
// wired at all -- previously the route was gated on AdminToken alone and
// simply did not exist for a deployment relying purely on session login.
func TestRegister_ApplyRoute_Mounted_WhenSessionAuthOnly_NoToken(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	hcStore := sqlite.NewHandicapStore(db.DB)
	hcSvc := handicaps.NewService(hcStore)
	leagueStore := sqlite.NewLeagueStore(db.DB)
	leagueSvc := leagues.NewLeagueService(leagueStore)
	seasonStore := sqlite.NewSeasonStore(db.DB)
	seasonSvc := seasons.NewSeasonService(seasonStore)

	authUserStore := sqlite.NewAuthUserStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	sessionStore := sqlite.NewSessionStore(db.DB)
	setupTokenStore := sqlite.NewPasswordSetupTokenStore(db.DB)
	sessionSvc := auth.NewSessionService(authUserStore, roleStore, sessionStore, setupTokenStore, fastArgon2)

	mux := http.NewServeMux()
	deps := handlers.Dependencies{
		HandicapSvc:     hcSvc,
		HandicapApplier: hcSvc,
		RuleMgr:         &noopRuleMgr{},
		LeagueMgr:       leagueSvc,
		PlayerMgr:       &noopPlayerMgr{},
		TeamMgr:         &noopTeamMgr{},
		SeasonMgr:       seasonSvc,
		// AdminToken and ApplyAuth are BOTH deliberately left unset --
		// only session/scoped-role auth is available.
		AuthMgr:              sessionSvc,
		RoleAssignmentMgr:    roleStore,
		LeagueSelfGrantMgr:   sqlite.NewLeagueSelfGrantStore(db.DB),
		InsecureLocalCookies: true,
	}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// No credential at all: the route must exist (401, not 404).
	noCredResp := authDo(t, srv, "POST", "/api/seasons/1/handicap-apply", `{}`, nil, nil)
	noCredResp.Body.Close()
	if noCredResp.StatusCode == http.StatusNotFound {
		t.Fatal("want the Apply route to be mounted even with no AdminToken/ApplyAuth configured, got 404")
	}
	if noCredResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 with no credential, got %d", noCredResp.StatusCode)
	}

	// A session-authenticated system_admin reaches the real handler
	// (CSRF required, scoped to the season's league -- proven by getting
	// PAST the auth layer at all; the handler's own business validation
	// of the apply request body is out of scope for this test).
	seedPasswordUser(t, "handicap-apply-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, csrf := loginCookies(t, srv, "handicap-apply-sysadmin@example.com", "hunter22")
	leagueID := seedTestLeague(t, "Handicap Apply League")
	seasonID := seedTestSeason(t, leagueID, "Handicap Apply Season")

	resp := authDo(t, srv, "POST", "/api/seasons/"+strconv.FormatInt(seasonID, 10)+"/handicap-apply",
		`{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[]}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		t.Errorf("want a session-authenticated system_admin to reach the Apply handler, got %d", resp.StatusCode)
	}
}

func TestPartialWiring_EntireAuthSubsystemAbsent_RemainsFullyOpen(t *testing.T) {
	// This is the ONE condition that legitimately falls open: a
	// Dependencies with ApplyAuth, AuthMgr, AND RoleAssignmentMgr all nil
	// -- the shape every pre-session-support test in this package
	// (globalCrudDeps, clearanceDeps, etc.) already relies on. Confirmed
	// here directly against registerLeagueRoutes' own gate so a future
	// change to guardedAction's condition is caught if it ever drifts
	// from this documented, intentional legacy-test-only shape.
	mux := http.NewServeMux()
	handlers.Register(mux, t.TempDir(), handlers.Dependencies{
		HandicapSvc: &stubHandicapSvc{fn: func(context.Context, int64) (models.HandicapReviewResponse, error) {
			return models.HandicapReviewResponse{}, nil
		}},
		RuleMgr:   &noopRuleMgr{},
		LeagueMgr: &noopLeagueMgr{},
		PlayerMgr: &noopPlayerMgr{},
		TeamMgr:   &noopTeamMgr{},
		SeasonMgr: &noopSeasonMgr{},
		// ApplyAuth, AuthMgr, and RoleAssignmentMgr are ALL left nil.
	})

	req := httptest.NewRequest(http.MethodPost, "/api/leagues", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("want the route to remain open when the entire auth subsystem is absent (legacy minimal-test compatibility), got %d", w.Code)
	}
}
