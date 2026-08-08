// Global CRUD route auth tests (Phase 5).
//
// Protected routes (9 total):
//   POST/PUT/DELETE /api/leagues/{id?}
//   POST/PUT/DELETE /api/players/{id?}
//   POST/PUT/DELETE /api/teams/{id?}
//
// Unprotected reads (no changes):
//   GET /api/leagues, GET /api/leagues/{id}, GET /api/players,
//   GET /api/players/{id}, GET /api/teams, GET /api/teams/{id}
//
// Auth rejection cases are demonstrated on POST /api/leagues as the
// representative route -- the clearanceAuth chain is identical for all
// nine routes. Success cases cover all three allowed roles distributed
// across the three CRUD families.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/models"
)

// globalCrudDeps returns Dependencies with personal-key auth wired and all
// managers needed for global CRUD routes set to noops.
// noopLeagueMgr, noopPlayerMgr, and noopTeamMgr are defined in
// api_apply_auth_test.go within this package.
func globalCrudDeps(auth ApplyAuthResolver) Dependencies {
	return Dependencies{
		HandicapSvc: &noopRecommender{},
		RuleMgr:     &noopRuleManager{},
		LeagueMgr:   &noopLeagueMgr{},
		PlayerMgr:   &noopPlayerMgr{},
		TeamMgr:     &noopTeamMgr{},
		SeasonMgr:   &noopSeasonMgr{},
		ApplyAuth:   auth,
	}
}

// --- Auth rejection tests (representative route: POST /api/leagues) ---

func TestGlobalCrudRoute_CreateLeague_NoHeader_Returns401(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/leagues",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("want WWW-Authenticate header on 401")
	}
}

func TestGlobalCrudRoute_CreateLeague_InvalidToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/leagues",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

// TestGlobalCrudRoute_CreateLeague_StaticAdminToken_Returns403 verifies that
// the static LEAGUE_ADMIN_TOKEN does not authorize global CRUD routes.
func TestGlobalCrudRoute_CreateLeague_StaticAdminToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "valid-user-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/leagues",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize global CRUD routes), got %d", w.Code)
	}
}

func TestGlobalCrudRoute_CreateLeague_ScoreKeeperRole_Returns403(t *testing.T) {
	scorer := &models.User{ID: 2, Role: "score_keeper", Active: true}
	auth := &stubApplyAuth{resolveKey: "scorer-key", resolveUser: scorer}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/leagues",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer scorer-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

// --- League CRUD ---

func TestGlobalCrudRoute_CreateLeague_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/leagues",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (league_admin creates league), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalCrudRoute_UpdateLeague_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPut, "/api/leagues/1",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin compat updates league), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalCrudRoute_DeleteLeague_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/leagues/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin deletes league), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Player CRUD ---

func TestGlobalCrudRoute_CreatePlayer_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/players",
		strings.NewReader(`{"first_name":"Test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (system_admin creates player), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalCrudRoute_UpdatePlayer_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPut, "/api/players/1",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin updates player), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalCrudRoute_DeletePlayer_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/players/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (admin compat deletes player), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Team CRUD ---

func TestGlobalCrudRoute_CreateTeam_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/teams",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (admin compat creates team), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalCrudRoute_UpdateTeam_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodPut, "/api/teams/1",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin updates team), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGlobalCrudRoute_DeleteTeam_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/teams/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin deletes team), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Unprotected reads confirmation ---

func TestGlobalCrudRoute_ListLeagues_NoAuth_ReachesHandler(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), globalCrudDeps(auth))

	req := httptest.NewRequest(http.MethodGet, "/api/leagues", nil)
	// No Authorization header -- GET reads remain unprotected.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (GET /api/leagues is unprotected), got %d: %s", w.Code, w.Body.String())
	}
}
