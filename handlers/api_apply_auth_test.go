// Auth-layer tests for the requireAdminToken wrapper and Apply route mounting.
// Uses package handlers (internal) to access unexported helpers and Register().
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/models"
)

// noopRecommender satisfies HandicapRecommender for tests that only exercise
// auth or route-mounting logic and do not need real recommendation data.
type noopRecommender struct{}

func (n *noopRecommender) Recommendations(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
	return models.HandicapReviewResponse{}, nil
}

// ─── requireAdminToken unit tests ─────────────────────────────────────────────

func TestRequireAdminToken_NoHeader_Returns401(t *testing.T) {
	wrapped := requireAdminToken("secret", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	wrapped(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestRequireAdminToken_NoHeader_SetsWWWAuthenticate(t *testing.T) {
	wrapped := requireAdminToken("secret", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	wrapped(w, req)
	got := w.Header().Get("WWW-Authenticate")
	if got != `Bearer realm="league-admin"` {
		t.Errorf("want WWW-Authenticate: Bearer realm=\"league-admin\", got %q", got)
	}
}

func TestRequireAdminToken_WrongToken_Returns403(t *testing.T) {
	wrapped := requireAdminToken("secret", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	wrapped(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

func TestRequireAdminToken_CorrectToken_CallsNext(t *testing.T) {
	called := false
	wrapped := requireAdminToken("secret", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	wrapped(w, req)
	if !called {
		t.Error("want next handler to be called with correct token")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestRequireAdminToken_BearerPrefix_Required(t *testing.T) {
	// Raw token without "Bearer " prefix must be rejected (403, not 401 — header is present).
	wrapped := requireAdminToken("secret", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "secret") // no "Bearer " prefix
	w := httptest.NewRecorder()
	wrapped(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 when Bearer prefix absent, got %d", w.Code)
	}
}

func TestRequireAdminToken_NoHeader_BodyContainsError(t *testing.T) {
	wrapped := requireAdminToken("secret", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	wrapped(w, req)
	if !strings.Contains(w.Body.String(), "authentication required") {
		t.Errorf("want 'authentication required' in body, got: %s", w.Body.String())
	}
}

func TestRequireAdminToken_WrongToken_BodyContainsError(t *testing.T) {
	wrapped := requireAdminToken("secret", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	wrapped(w, req)
	if !strings.Contains(w.Body.String(), "forbidden") {
		t.Errorf("want 'forbidden' in body, got: %s", w.Body.String())
	}
}

// ─── Register() mounting tests ─────────────────────────────────────────────────

// TestRegister_ApplyRoute_NotMounted_WhenTokenEmpty verifies that the Apply
// route is absent when AdminToken is empty, returning 404 (not 401/403/405).
func TestRegister_ApplyRoute_NotMounted_WhenTokenEmpty(t *testing.T) {
	mux := http.NewServeMux()
	deps := Dependencies{
		HandicapSvc:     &noopRecommender{},
		HandicapApplier: &stubApplier{},
		AdminToken:      "",
	}
	Register(mux, t.TempDir(), deps)

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/handicap-apply",
		strings.NewReader(`{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 when token empty (route not mounted), got %d", w.Code)
	}
}

// TestRegister_ApplyRoute_Mounted_WhenTokenPresent verifies that the Apply route
// is registered when AdminToken is non-empty, confirmed by the auth layer firing
// (401 without Authorization header instead of 404).
func TestRegister_ApplyRoute_Mounted_WhenTokenPresent(t *testing.T) {
	mux := http.NewServeMux()
	deps := Dependencies{
		HandicapSvc:     &noopRecommender{},
		HandicapApplier: &stubApplier{},
		AdminToken:      "test-token",
	}
	Register(mux, t.TempDir(), deps)

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/handicap-apply",
		strings.NewReader(`{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Auth layer fires before handler body: 401 (not 404) proves route is mounted.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (auth layer) when route mounted without header, got %d", w.Code)
	}
}

// TestRegister_ApplyRoute_CorrectToken_ReachesHandler verifies that a correctly
// authorized request reaches postHandicapApply. The stub applier returns success,
// so we expect 200 with a body — not a 4xx from the auth layer.
func TestRegister_ApplyRoute_CorrectToken_ReachesHandler(t *testing.T) {
	mux := http.NewServeMux()
	deps := Dependencies{
		HandicapSvc:     &noopRecommender{},
		HandicapApplier: &stubApplier{},
		AdminToken:      "test-token",
	}
	Register(mux, t.TempDir(), deps)

	body := `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":` +
		`[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"tok"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/handicap-apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Auth passes; stubApplier returns zero-value ApplyResult with no error → 200.
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (handler reached), got %d: %s", w.Code, w.Body.String())
	}
}

// ─── HandicapApplier dependency guard tests ────────────────────────────────────

// TestRegister_NilApplier_WithToken_Panics verifies that Register panics at
// startup (not at first request) when the Apply route would be mounted but
// HandicapApplier is nil.
func TestRegister_NilApplier_WithToken_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when HandicapApplier is nil and AdminToken is set")
		}
	}()
	Register(http.NewServeMux(), t.TempDir(), Dependencies{
		HandicapSvc:     &noopRecommender{},
		HandicapApplier: nil,
		AdminToken:      "test-token",
	})
}

// TestRegister_TypedNilApplier_WithToken_Panics verifies that Register panics
// when HandicapApplier is a typed-nil (non-nil interface holding a nil pointer),
// which would panic on the first method call if allowed through.
func TestRegister_TypedNilApplier_WithToken_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when HandicapApplier is a typed-nil and AdminToken is set")
		}
	}()
	Register(http.NewServeMux(), t.TempDir(), Dependencies{
		HandicapSvc:     &noopRecommender{},
		HandicapApplier: (*stubApplier)(nil), // typed-nil: interface != nil, pointer == nil
		AdminToken:      "test-token",
	})
}

// TestRegister_NilApplier_NoToken_DoesNotPanic verifies that a nil
// HandicapApplier is acceptable when AdminToken is empty — the Apply route is
// not mounted, so the applier is never called.
func TestRegister_NilApplier_NoToken_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("want no panic when HandicapApplier is nil and AdminToken is empty, got: %v", r)
		}
	}()
	Register(http.NewServeMux(), t.TempDir(), Dependencies{
		HandicapSvc:     &noopRecommender{},
		HandicapApplier: nil,
		AdminToken:      "",
	})
}
