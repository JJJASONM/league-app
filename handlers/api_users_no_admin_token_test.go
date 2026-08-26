// User-management route tests for the Users Admin Screen Phase 1
// correction: GET/POST /api/users must work via a system_admin/admin
// personal key even when AdminToken (the static bootstrap token) is not
// configured at all -- route mounting must not depend on AdminToken, and
// the static-token path must never accidentally authorize a request when
// AdminToken is empty.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/db"
	"league_app/models"
)

func TestGetUsers_NoAdminToken_SystemAdminPersonalKey_Returns200(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "sys-admin-key", resolveUser: user}
	dataDir := t.TempDir()
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	Register(mux, dataDir, backupDeps(auth)) // backupDeps leaves AdminToken empty

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer sys-admin-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin personal key must work when AdminToken is not configured), got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostUsers_NoAdminToken_SystemAdminPersonalKey_Returns201(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "sys-admin-key", resolveUser: user}
	dataDir := t.TempDir()
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	Register(mux, dataDir, backupDeps(auth)) // backupDeps leaves AdminToken empty

	req := httptest.NewRequest(http.MethodPost, "/api/users",
		strings.NewReader(`{"username":"no-token-created","role":"league_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sys-admin-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want 201 (system_admin personal key must work when AdminToken is not configured), got %d: %s", w.Code, w.Body.String())
	}
}

// TestGetUsers_NoAdminToken_EmptyBearerDoesNotAuthorize verifies the
// adminToken != "" guard in requireAdminTokenOrSystemAdminAuth: when
// AdminToken is unconfigured (empty string), an empty bearer token must
// not accidentally equal it and slip through the static-token path.
func TestGetUsers_NoAdminToken_EmptyBearerDoesNotAuthorize(t *testing.T) {
	auth := &stubApplyAuth{} // no resolveKey configured -- nothing resolves
	dataDir := t.TempDir()
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	Register(mux, dataDir, backupDeps(auth)) // backupDeps leaves AdminToken empty

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer ") // empty token after "Bearer "
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (empty token must not match an unconfigured AdminToken), got %d", w.Code)
	}
}

// TestPostUsers_NoAdminToken_EmptyBearerDoesNotAuthorize is the POST
// counterpart of the guard above.
func TestPostUsers_NoAdminToken_EmptyBearerDoesNotAuthorize(t *testing.T) {
	auth := &stubApplyAuth{} // no resolveKey configured -- nothing resolves
	dataDir := t.TempDir()
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	Register(mux, dataDir, backupDeps(auth)) // backupDeps leaves AdminToken empty

	req := httptest.NewRequest(http.MethodPost, "/api/users",
		strings.NewReader(`{"username":"should-not-be-created","role":"league_admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ") // empty token after "Bearer "
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (empty token must not match an unconfigured AdminToken), got %d", w.Code)
	}
}

// TestGetUsers_NoAdminToken_LeagueAdminPersonalKey_Returns403 confirms the
// role gate is still enforced when AdminToken is unconfigured -- mounting
// the route unconditionally must not loosen who is allowed to use it.
func TestGetUsers_NoAdminToken_LeagueAdminPersonalKey_Returns403(t *testing.T) {
	user := &models.User{ID: 2, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "league-admin-key", resolveUser: user}
	dataDir := t.TempDir()
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	Register(mux, dataDir, backupDeps(auth)) // backupDeps leaves AdminToken empty

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Authorization", "Bearer league-admin-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (league_admin still rejected when AdminToken is unconfigured), got %d", w.Code)
	}
}
