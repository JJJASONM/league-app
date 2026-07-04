// Auth-layer tests for requireAdminToken, requireApplyAuth, and Apply route mounting.
// Uses package handlers (internal) to access unexported helpers and Register().
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/domains/seasons"
	"league_app/models"
)

// noopRecommender satisfies HandicapRecommender for tests that only exercise
// auth or route-mounting logic and do not need real recommendation data.
type noopRecommender struct{}

func (n *noopRecommender) Recommendations(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
	return models.HandicapReviewResponse{}, nil
}

// noopRuleManager satisfies RuleManager for tests that only exercise auth or
// route-mounting logic and do not exercise the season-rules endpoints.
type noopRuleManager struct{}

func (n *noopRuleManager) List(_ context.Context, _ int64) ([]models.SeasonRule, error) {
	return nil, nil
}
func (n *noopRuleManager) Upsert(_ context.Context, r models.SeasonRule) (models.SeasonRule, error) {
	return r, nil
}
func (n *noopRuleManager) Update(_ context.Context, _ int64, _, _ string) error { return nil }
func (n *noopRuleManager) Delete(_ context.Context, _ int64) error               { return nil }

// noopSeasonMgr satisfies SeasonManager for tests that only exercise auth or
// route-mounting logic and do not exercise season lifecycle endpoints.
type noopSeasonMgr struct{}

func (n *noopSeasonMgr) Activate(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) Checklist(_ context.Context, _ int64) (models.SetupChecklist, error) {
	return models.SetupChecklist{CanActivate: true}, nil
}
func (n *noopSeasonMgr) PreviousSeason(_ context.Context, _ int64) (seasons.PreviousSeasonResult, error) {
	return seasons.PreviousSeasonResult{Teams: []seasons.SeasonTeamEntry{}}, nil
}
func (n *noopSeasonMgr) IsDraft(_ context.Context, _ int64) (bool, error) { return true, nil }
func (n *noopSeasonMgr) MarkStaleIfScheduled(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) AddTeam(_ context.Context, _ int64, _ seasons.AddTeamRequest) (models.SeasonTeam, error) {
	return models.SeasonTeam{}, nil
}
func (n *noopSeasonMgr) RemoveTeam(_ context.Context, _, _ int64) error { return nil }
func (n *noopSeasonMgr) UpdateTeam(_ context.Context, _, _ int64, _ seasons.UpdateTeamRequest) (models.SeasonTeam, error) {
	return models.SeasonTeam{}, nil
}
func (n *noopSeasonMgr) CreateByeRequest(_ context.Context, _ int64, _ seasons.CreateByeRequestInput) (models.ByeRequest, error) {
	return models.ByeRequest{}, nil
}
func (n *noopSeasonMgr) UpdateByeRequest(_ context.Context, _, _ int64, _ bool) (models.ByeRequest, error) {
	return models.ByeRequest{}, nil
}
func (n *noopSeasonMgr) ListRoster(_ context.Context, _, _ int64) ([]models.SeasonRosterEntry, error) {
	return []models.SeasonRosterEntry{}, nil
}
func (n *noopSeasonMgr) AddRosterPlayer(_ context.Context, _, _, _ int64) (models.SeasonRosterEntry, error) {
	return models.SeasonRosterEntry{}, nil
}
func (n *noopSeasonMgr) RemoveRosterPlayer(_ context.Context, _, _, _ int64) error { return nil }
func (n *noopSeasonMgr) ListAvailablePlayers(_ context.Context, _ int64) ([]models.Player, error) {
	return []models.Player{}, nil
}
func (n *noopSeasonMgr) ListSeasonTeams(_ context.Context, _ int64) ([]models.SeasonTeam, error) {
	return []models.SeasonTeam{}, nil
}
func (n *noopSeasonMgr) ListSeasons(_ context.Context, _ *int64) ([]models.Season, error) {
	return []models.Season{}, nil
}
func (n *noopSeasonMgr) GetSeason(_ context.Context, _ int64) (models.Season, error) {
	return models.Season{}, nil
}
func (n *noopSeasonMgr) CreateSeason(_ context.Context, _ seasons.CreateSeasonInput) (models.Season, error) {
	return models.Season{}, nil
}
func (n *noopSeasonMgr) UpdateSeason(_ context.Context, _ int64, _ seasons.UpdateSeasonInput) (models.Season, error) {
	return models.Season{}, nil
}
func (n *noopSeasonMgr) DeleteSeason(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) ListSkippedWeeks(_ context.Context, _ int64) ([]models.SkippedWeek, error) {
	return []models.SkippedWeek{}, nil
}
func (n *noopSeasonMgr) CreateSkippedWeek(_ context.Context, _ int64, _, _ string) (models.SkippedWeek, error) {
	return models.SkippedWeek{}, nil
}
func (n *noopSeasonMgr) DeleteSkippedWeek(_ context.Context, _ int64) error { return nil }
func (n *noopSeasonMgr) ListByeRequests(_ context.Context, _ int64) ([]models.ByeRequest, error) {
	return []models.ByeRequest{}, nil
}
func (n *noopSeasonMgr) DeleteByeRequest(_ context.Context, _, _ int64) error { return nil }

// noopScheduleMgr satisfies ScheduleManager for tests that only exercise auth
// or route-mounting logic and do not exercise schedule generation endpoints.
type noopScheduleMgr struct{}

func (n *noopScheduleMgr) GenerateSchedule(_ context.Context, _ matches.GenerateRequest) (matches.GenerateResult, error) {
	return matches.GenerateResult{}, nil
}

// noopMatchMgr satisfies MatchManager for tests that only exercise auth or
// route-mounting logic and do not exercise match listing or assignment endpoints.
type noopMatchMgr struct{}

func (n *noopMatchMgr) ListMatches(_ context.Context, _ matches.ListMatchesRequest) ([]models.Match, error) {
	return []models.Match{}, nil
}
func (n *noopMatchMgr) GetMatch(_ context.Context, _ int64) (models.MatchDetail, error) {
	return models.MatchDetail{}, nil
}
func (n *noopMatchMgr) AssignMatchTeams(_ context.Context, _ int64, _, _ *int64) error {
	return nil
}

// noopLineupMgr satisfies LineupManager for tests that don't exercise lineup routes.
type noopLineupMgr struct{}

func (n *noopLineupMgr) ListLineupPlans(_ context.Context, _ matches.ListLineupPlansRequest) ([]models.LineupPlan, error) {
	return []models.LineupPlan{}, nil
}
func (n *noopLineupMgr) SaveTeamLineup(_ context.Context, _ matches.SaveLineupRequest) error {
	return nil
}
func (n *noopLineupMgr) DeleteLineupPlan(_ context.Context, _ int64) error { return nil }

// stubApplyAuth satisfies ApplyAuthResolver for auth middleware tests.
// ResolveKey maps a cleartext key to a user; zero-value returns nil (no match).
type stubApplyAuth struct {
	resolveKey  string      // cleartext key that resolves successfully
	resolveUser *models.User
}

func (s *stubApplyAuth) ResolveApplyUserByAPIKey(_ context.Context, key string) (*models.User, error) {
	if s.resolveKey != "" && key == s.resolveKey {
		return s.resolveUser, nil
	}
	return nil, nil
}

func (s *stubApplyAuth) CreateApplyUser(_ context.Context, username string) (models.User, string, error) {
	return models.User{ID: 1, Username: username, Role: "admin", Active: true}, "fake-key-64chars-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", nil
}

func (s *stubApplyAuth) ListApplyUsers(_ context.Context) ([]models.User, error) {
	return nil, nil
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

// ─── requireApplyAuth unit tests ──────────────────────────────────────────────

func TestRequireApplyAuth_NoHeader_Returns401(t *testing.T) {
	wrapped := requireApplyAuth("secret", nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	wrapped(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestRequireApplyAuth_PersonalKey_SetsUserIDInContext(t *testing.T) {
	user := &models.User{ID: 42, Username: "alice", Role: "admin", Active: true}
	resolver := &stubApplyAuth{resolveKey: "mykey", resolveUser: user}

	var gotID *int64
	wrapped := requireApplyAuth("admin-token", resolver, func(w http.ResponseWriter, r *http.Request) {
		gotID = applyUserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer mykey")
	w := httptest.NewRecorder()
	wrapped(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if gotID == nil || *gotID != 42 {
		t.Errorf("want user ID 42 in context, got %v", gotID)
	}
}

func TestRequireApplyAuth_StaticToken_NilUserIDInContext(t *testing.T) {
	resolver := &stubApplyAuth{} // no matching key

	var gotID *int64
	wrapped := requireApplyAuth("admin-token", resolver, func(w http.ResponseWriter, r *http.Request) {
		gotID = applyUserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	wrapped(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if gotID != nil {
		t.Errorf("want nil user ID for static-token path, got %v", gotID)
	}
}

func TestRequireApplyAuth_WrongToken_Returns403(t *testing.T) {
	resolver := &stubApplyAuth{} // no matching key

	wrapped := requireApplyAuth("admin-token", resolver, func(w http.ResponseWriter, r *http.Request) {
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

func TestRequireApplyAuth_NilResolver_StaticTokenAllowed(t *testing.T) {
	wrapped := requireApplyAuth("admin-token", nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	wrapped(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 with nil resolver + correct static token, got %d", w.Code)
	}
}

// TestRequireApplyAuth_InactiveKey_Returns403 verifies that a key belonging to
// an inactive user is rejected. The store filters inactive users at the SQL
// level and returns nil, so the middleware falls through to the static-token
// check; when that also fails, 403 is returned.
func TestRequireApplyAuth_InactiveKey_Returns403(t *testing.T) {
	// stubApplyAuth with empty resolveKey always returns nil (simulates inactive/missing user).
	resolver := &stubApplyAuth{}

	wrapped := requireApplyAuth("admin-token", resolver, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer inactive-user-key")
	w := httptest.NewRecorder()
	wrapped(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for inactive user key, got %d", w.Code)
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
		RuleMgr:         &noopRuleManager{},
		SeasonMgr:       &noopSeasonMgr{},
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
		RuleMgr:         &noopRuleManager{},
		SeasonMgr:       &noopSeasonMgr{},
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
		RuleMgr:         &noopRuleManager{},
		SeasonMgr:       &noopSeasonMgr{},
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
		RuleMgr:         &noopRuleManager{},
		SeasonMgr:       &noopSeasonMgr{},
	})
}
