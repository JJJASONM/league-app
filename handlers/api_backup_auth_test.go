// Backup route auth tests (Phase 6).
//
// Protected route:
//   POST /api/backup
//
// Unlike the clearanceAuth-wrapped routes from Phases 1-5, this route uses
// systemAdminAuth: only "system_admin" and the legacy "admin" alias are
// allowed. "league_admin" is rejected, which is the key behavioral
// difference verified here.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"league_app/db"
	"league_app/models"
)

// backupDeps returns Dependencies with personal-key auth wired and all
// managers required by Register set to noops. Only the backup route is
// exercised by these tests.
func backupDeps(auth ApplyAuthResolver) Dependencies {
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

func TestBackupRoute_NoHeader_Returns401(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "system_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), backupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/backup", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("want WWW-Authenticate header on 401")
	}
}

func TestBackupRoute_InvalidToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "system_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), backupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/backup", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

// TestBackupRoute_StaticAdminToken_Returns403 verifies that the static
// LEAGUE_ADMIN_TOKEN does not authorize the backup route. Personal-key-only
// auth has no static-token fallback.
func TestBackupRoute_StaticAdminToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "valid-user-key", resolveUser: &models.User{ID: 1, Role: "system_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), backupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/backup", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize backup), got %d", w.Code)
	}
}

func TestBackupRoute_ScoreKeeperRole_Returns403(t *testing.T) {
	scorer := &models.User{ID: 2, Role: "score_keeper", Active: true}
	auth := &stubApplyAuth{resolveKey: "scorer-key", resolveUser: scorer}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), backupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/backup", nil)
	req.Header.Set("Authorization", "Bearer scorer-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

// TestBackupRoute_LeagueAdminRole_Returns403 verifies the key behavioral
// difference from Phases 1-5: league_admin is allowed on clearance/schedule/
// match/season/CRUD routes but is rejected here. Backup is system-admin only.
func TestBackupRoute_LeagueAdminRole_Returns403(t *testing.T) {
	leagueAdmin := &models.User{ID: 3, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "league-admin-key", resolveUser: leagueAdmin}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), backupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/backup", nil)
	req.Header.Set("Authorization", "Bearer league-admin-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for league_admin role (backup is system-admin only), got %d", w.Code)
	}
}

// --- Success path ---

func TestBackupRoute_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	dataDir := t.TempDir()
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	Register(mux, dataDir, backupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/backup", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin treated as system-admin-compatible alias), got %d: %s", w.Code, w.Body.String())
	}
}

func TestBackupRoute_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	dataDir := t.TempDir()
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	Register(mux, dataDir, backupDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/backup", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches backup handler), got %d: %s", w.Code, w.Body.String())
	}
}
