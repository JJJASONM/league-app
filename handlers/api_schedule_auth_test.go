// Schedule mutation route auth tests: POST /api/matches/generate and
// POST /api/seasons/{id}/schedule/pushback-apply are gated by personal-key-only
// Bearer auth + league_admin role check (Phase 2).
//
// POST /api/seasons/{id}/schedule/pushback-preview is intentionally left
// unprotected (read-only semantics despite POST method) and is verified here
// to confirm it requires no Authorization header.
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/models"
)

// --- noopPushbackMgr satisfies PushbackPreviewer for schedule mutation auth tests ---

type noopPushbackMgr struct{}

func (n *noopPushbackMgr) Preview(_ context.Context, _ matches.PushbackPreviewRequest) (matches.PushbackPreviewResult, error) {
	return matches.PushbackPreviewResult{}, nil
}

// --- noopPushbackApplyMgr satisfies PushbackApplier for schedule mutation auth tests ---

type noopPushbackApplyMgr struct{}

func (n *noopPushbackApplyMgr) Apply(_ context.Context, _ matches.PushbackPreviewRequest) (matches.PushbackPreviewResult, error) {
	return matches.PushbackPreviewResult{}, nil
}

// scheduleMutationDeps returns Dependencies with personal-key auth wired and all
// schedule-related managers set to noops. Both protected schedule mutation routes
// (generate, pushback-apply) and the unprotected pushback-preview are mounted.
func scheduleMutationDeps(auth ApplyAuthResolver) Dependencies {
	return Dependencies{
		HandicapSvc:      &noopRecommender{},
		RuleMgr:          &noopRuleManager{},
		LeagueMgr:        &noopLeagueMgr{},
		PlayerMgr:        &noopPlayerMgr{},
		TeamMgr:          &noopTeamMgr{},
		SeasonMgr:        &noopSeasonMgr{},
		ScheduleMgr:      &noopScheduleMgr{},
		PushbackMgr:      &noopPushbackMgr{},
		PushbackApplyMgr: &noopPushbackApplyMgr{},
		ApplyAuth:        auth,
	}
}

// --- Auth rejection tests (representative route: POST /api/matches/generate) ---

func TestScheduleRoute_Generate_NoHeader_Returns401(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/generate",
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

func TestScheduleRoute_Generate_InvalidToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/generate",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

// TestScheduleRoute_Generate_StaticAdminToken_Returns403 verifies that the static
// LEAGUE_ADMIN_TOKEN does not grant access to the generate route. Personal-key-only
// auth has no static-token fallback.
func TestScheduleRoute_Generate_StaticAdminToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "valid-user-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/generate",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin-token") // static env-var token, not a personal key
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize schedule mutation routes), got %d", w.Code)
	}
}

func TestScheduleRoute_Generate_ScoreKeeperRole_Returns403(t *testing.T) {
	scorer := &models.User{ID: 2, Role: "score_keeper", Active: true}
	auth := &stubApplyAuth{resolveKey: "scorer-key", resolveUser: scorer}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/generate",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer scorer-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

// --- Success path: POST /api/matches/generate ---

func TestScheduleRoute_Generate_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/generate",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin reaches handler), got %d: %s", w.Code, w.Body.String())
	}
}

// TestScheduleRoute_Generate_AdminCompat_ReachesHandler verifies role="admin"
// is accepted as the backward-compatible alias for league_admin.
func TestScheduleRoute_Generate_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/generate",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin backward-compat alias), got %d: %s", w.Code, w.Body.String())
	}
}

func TestScheduleRoute_Generate_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/matches/generate",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches handler), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Success path: POST /api/seasons/{id}/schedule/pushback-apply ---

func TestScheduleRoute_PushbackApply_LeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/schedule/pushback-apply",
		strings.NewReader(`{"cutoff_week":1,"weeks_to_add":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (league_admin reaches pushback-apply), got %d: %s", w.Code, w.Body.String())
	}
}

func TestScheduleRoute_PushbackApply_AdminCompat_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/schedule/pushback-apply",
		strings.NewReader(`{"cutoff_week":1,"weeks_to_add":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin compat for pushback-apply), got %d: %s", w.Code, w.Body.String())
	}
}

func TestScheduleRoute_PushbackApply_SystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/schedule/pushback-apply",
		strings.NewReader(`{"cutoff_week":1,"weeks_to_add":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches pushback-apply), got %d: %s", w.Code, w.Body.String())
	}
}

// TestScheduleRoute_PushbackPreview_NoAuth_ReachesHandler confirms that
// POST /api/seasons/{id}/schedule/pushback-preview is intentionally unprotected.
// It accepts no Authorization header and returns 200.
func TestScheduleRoute_PushbackPreview_NoAuth_ReachesHandler(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), scheduleMutationDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/schedule/pushback-preview",
		strings.NewReader(`{"cutoff_week":1,"weeks_to_add":1}`))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header - this route is deliberately unprotected.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (pushback-preview is unprotected read-only route), got %d: %s", w.Code, w.Body.String())
	}
}
