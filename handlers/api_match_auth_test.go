// Match mutation route auth tests (Phase 3).
//
// Protected routes:
//   PATCH /api/matches/{id}/assign
//   POST  /api/matches/{id}/results
//   DELETE /api/matches/{id}/results
//   POST  /api/matches/{id}/rounds
//
// Unprotected reads (no changes):
//   GET /api/matches, GET /api/matches/{id}, GET /api/matches/{id}/rounds,
//   GET /api/standings, GET /api/player-stats
//
// Auth rejection cases are demonstrated on PATCH /api/matches/{id}/assign as
// the representative route -- the clearanceAuth chain is identical for all four.
// Success cases cover all three allowed roles distributed across the four routes.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/models"
)

// matchMutationDeps returns Dependencies with personal-key auth wired and all
// managers needed for match mutation routes set to noops.
// noopMatchMgr and noopRoundMgr are defined in other _test.go files in this package.
func matchMutationDeps(auth ApplyAuthResolver) Dependencies {
	return Dependencies{
		HandicapSvc: &noopRecommender{},
		RuleMgr:     &noopRuleManager{},
		LeagueMgr:   &noopLeagueMgr{},
		PlayerMgr:   &noopPlayerMgr{},
		TeamMgr:     &noopTeamMgr{},
		SeasonMgr:   &noopSeasonMgr{},
		MatchMgr:    &noopMatchMgr{},
		RoundMgr:    &noopRoundMgr{},
		ApplyAuth:   auth,
	}
}

// --- Auth rejection tests (representative route: PATCH /api/matches/{id}/assign) ---

func TestMatchRoute_Assign_NoHeader_Returns401(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPatch, "/api/matches/1/assign",
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

func TestMatchRoute_Assign_InvalidToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPatch, "/api/matches/1/assign",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

// TestMatchRoute_Assign_StaticAdminToken_Returns403 verifies that the static
// LEAGUE_ADMIN_TOKEN does not authorize match mutation routes.
// Personal-key-only auth has no static-token fallback.
func TestMatchRoute_Assign_StaticAdminToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "valid-user-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPatch, "/api/matches/1/assign",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize match mutation routes), got %d", w.Code)
	}
}

func TestMatchRoute_Assign_ScoreKeeperRole_Returns403(t *testing.T) {
	scorer := &models.User{ID: 2, Role: "score_keeper", Active: true}
	auth := &stubApplyAuth{resolveKey: "scorer-key", resolveUser: scorer}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPatch, "/api/matches/1/assign",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer scorer-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

// --- Success path: PATCH /api/matches/{id}/assign ---

func TestMatchRoute_Assign_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPatch, "/api/matches/1/assign",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin reaches assign handler), got %d: %s", w.Code, w.Body.String())
	}
}

func TestMatchRoute_Assign_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPatch, "/api/matches/1/assign",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin backward-compat alias for assign), got %d: %s", w.Code, w.Body.String())
	}
}

func TestMatchRoute_Assign_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPatch, "/api/matches/1/assign",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches assign handler), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Success path: POST /api/matches/{id}/results ---

func TestMatchRoute_SubmitResults_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/1/results",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin reaches submit-results handler), got %d: %s", w.Code, w.Body.String())
	}
}

func TestMatchRoute_SubmitResults_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/1/results",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin compat for submit-results), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Success path: DELETE /api/matches/{id}/results ---

func TestMatchRoute_ClearResults_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/matches/1/results", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin reaches clear-results handler), got %d: %s", w.Code, w.Body.String())
	}
}

func TestMatchRoute_ClearResults_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/matches/1/results", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches clear-results handler), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Success path: POST /api/matches/{id}/rounds ---

func TestMatchRoute_SaveRounds_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/1/rounds",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin reaches save-rounds handler), got %d: %s", w.Code, w.Body.String())
	}
}

func TestMatchRoute_SaveRounds_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), matchMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/1/rounds",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches save-rounds handler), got %d: %s", w.Code, w.Body.String())
	}
}
