// Season setup route auth tests (Phase 4).
//
// Protected routes (19 total):
//   POST/PUT/DELETE /api/seasons, POST /api/seasons/{id}/activate
//   POST/PUT/DELETE /api/seasons/{id}/rules/{rid?}
//   POST/DELETE /api/seasons/{id}/skipped-weeks/{sid?}
//   POST/PUT/DELETE /api/seasons/{id}/bye-requests/{bid?}
//   POST/PUT/DELETE /api/seasons/{id}/teams/{tid?}
//   POST/DELETE /api/seasons/{id}/teams/{tid}/roster/{pid?}
//   POST/DELETE /api/lineup-plans/{id?}
//
// Auth rejection cases are demonstrated on POST /api/seasons as the
// representative route -- the clearanceAuth chain is identical for all routes.
// Success cases cover all three allowed roles distributed across the 19 routes.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/models"
)

// seasonSetupDeps returns Dependencies with personal-key auth wired and all
// managers needed for season setup routes set to noops.
// noopSeasonMgr, noopRuleManager, and noopLineupMgr are defined in
// api_apply_auth_test.go within this package.
func seasonSetupDeps(auth ApplyAuthResolver) Dependencies {
	return Dependencies{
		HandicapSvc: &noopRecommender{},
		RuleMgr:     &noopRuleManager{},
		LeagueMgr:   &noopLeagueMgr{},
		PlayerMgr:   &noopPlayerMgr{},
		TeamMgr:     &noopTeamMgr{},
		SeasonMgr:   &noopSeasonMgr{},
		LineupMgr:   &noopLineupMgr{},
		ApplyAuth:   auth,
	}
}

// --- Auth rejection tests (representative route: POST /api/seasons) ---

func TestSeasonSetupRoute_CreateSeason_NoHeader_Returns401(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons",
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

func TestSeasonSetupRoute_CreateSeason_InvalidToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

// TestSeasonSetupRoute_CreateSeason_StaticAdminToken_Returns403 verifies that
// the static LEAGUE_ADMIN_TOKEN does not authorize season setup routes.
func TestSeasonSetupRoute_CreateSeason_StaticAdminToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "valid-user-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize season setup routes), got %d", w.Code)
	}
}

func TestSeasonSetupRoute_CreateSeason_ScoreKeeperRole_Returns403(t *testing.T) {
	scorer := &models.User{ID: 2, Role: "score_keeper", Active: true}
	auth := &stubApplyAuth{resolveKey: "scorer-key", resolveUser: scorer}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer scorer-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

// --- Season CRUD and activate ---

func TestSeasonSetupRoute_CreateSeason_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (league_admin creates season), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_UpdateSeason_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPut, "/api/seasons/1",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin compat for update season), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_DeleteSeason_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/seasons/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin deletes season), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_ActivateSeason_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/activate", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin activates season), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Season rules ---

func TestSeasonSetupRoute_CreateRule_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/rules",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (admin compat creates rule), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_UpdateRule_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPut, "/api/seasons/1/rules/1",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin updates rule), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_DeleteRule_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/seasons/1/rules/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin deletes rule), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Skipped weeks ---

func TestSeasonSetupRoute_CreateSkippedWeek_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/skipped-weeks",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (admin compat creates skipped week), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_DeleteSkippedWeek_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/seasons/1/skipped-weeks/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin deletes skipped week), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Bye requests ---

func TestSeasonSetupRoute_CreateByeRequest_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/bye-requests",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (league_admin creates bye request), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_UpdateByeRequest_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPut, "/api/seasons/1/bye-requests/1",
		strings.NewReader(`{"approved":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (admin compat updates bye request), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_DeleteByeRequest_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/seasons/1/bye-requests/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin deletes bye request), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Season teams ---

func TestSeasonSetupRoute_AddSeasonTeam_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/teams",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (league_admin adds season team), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_UpdateSeasonTeam_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPut, "/api/seasons/1/teams/1",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (admin compat updates season team), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_RemoveSeasonTeam_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/seasons/1/teams/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin removes season team), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Season roster ---

func TestSeasonSetupRoute_AddRosterPlayer_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/teams/1/roster",
		strings.NewReader(`{"player_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (league_admin adds roster player), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_RemoveRosterPlayer_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/seasons/1/teams/1/roster/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (admin compat removes roster player), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Lineup plans ---

func TestSeasonSetupRoute_SaveLineupPlan_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/lineup-plans",
		strings.NewReader(`{"season_id":1,"team_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (admin compat saves lineup plan), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSeasonSetupRoute_DeleteLineupPlan_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), seasonSetupDeps(auth))

	req := httptest.NewRequest(http.MethodDelete, "/api/lineup-plans/1", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin deletes lineup plan), got %d: %s", w.Code, w.Body.String())
	}
}
