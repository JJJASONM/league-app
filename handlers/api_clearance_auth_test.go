// Clearance route auth tests: requirePersonalKeyAuth, requireLeagueAdminRole,
// and route-level auth wiring for the four clearance routes:
//   POST /api/seasons/{id}/weeks/{week}/close
//   POST /api/seasons/{id}/weeks/{week}/reopen
//   POST /api/seasons/{id}/close
//   POST /api/seasons/{id}/reopen
//
// Coverage: auth rejection cases (401, 403, static-token rejection, wrong role) are
// demonstrated on /weeks/{week}/close as a representative route -- the clearanceAuth
// middleware chain is identical for all four routes. All four routes are verified for
// the success path (valid personal key + allowed role reaches the handler).
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/validation"
	"league_app/models"
)

// --- noopWeekMgr satisfies WeekManager for tests that only exercise auth ---

type noopWeekMgr struct{}

func (n *noopWeekMgr) ListWeeks(_ context.Context, _ int64) ([]models.WeekSummary, error) {
	return []models.WeekSummary{}, nil
}
func (n *noopWeekMgr) ValidateWeek(_ context.Context, _, _ int64) (validation.Result, error) {
	return validation.Result{}, nil
}
func (n *noopWeekMgr) CloseWeek(_ context.Context, _ matches.CloseWeekRequest) (matches.CloseWeekResult, error) {
	return matches.CloseWeekResult{}, nil
}
func (n *noopWeekMgr) ReopenWeek(_ context.Context, _, _ int64) error { return nil }
func (n *noopWeekMgr) ListAcknowledgments(_ context.Context, _, _ int64) ([]models.CloseAck, error) {
	return []models.CloseAck{}, nil
}
func (n *noopWeekMgr) AdvanceData(_ context.Context, _, _ int64) (models.AdvanceResult, error) {
	return models.AdvanceResult{}, nil
}
func (n *noopWeekMgr) AdvancePreview(_ context.Context, _, _ int64) (models.AdvancePreview, error) {
	return models.AdvancePreview{}, nil
}
func (n *noopWeekMgr) WeekRecap(_ context.Context, _, _ int64) (models.WeekRecap, error) {
	return models.WeekRecap{}, nil
}

// --- noopRoundMgr satisfies RoundManager for tests that only exercise auth ---

type noopRoundMgr struct{}

func (n *noopRoundMgr) IsSeasonClosedForMatch(_ context.Context, _ int64) (bool, error) {
	return false, nil
}
func (n *noopRoundMgr) SaveRounds(_ context.Context, _ matches.SaveRoundsInput) error { return nil }
func (n *noopRoundMgr) GetRounds(_ context.Context, _ int64) ([]models.RoundResult, error) {
	return []models.RoundResult{}, nil
}
func (n *noopRoundMgr) GetStandings(_ context.Context, _ int64) ([]models.Standing, error) {
	return []models.Standing{}, nil
}
func (n *noopRoundMgr) GetPlayerStats(_ context.Context, _ matches.PlayerStatsRequest) ([]models.PlayerStat, error) {
	return []models.PlayerStat{}, nil
}
func (n *noopRoundMgr) SubmitResults(_ context.Context, _ int64, _ []models.MatchResult) error {
	return nil
}
func (n *noopRoundMgr) ClearResults(_ context.Context, _ int64) error { return nil }

// clearanceDeps returns Dependencies with personal-key auth wired and all required
// managers set to noops. The four clearance routes are mounted with auth because
// ApplyAuth is non-nil.
func clearanceDeps(auth ApplyAuthResolver) Dependencies {
	return Dependencies{
		HandicapSvc: &noopRecommender{},
		RuleMgr:     &noopRuleManager{},
		LeagueMgr:   &noopLeagueMgr{},
		PlayerMgr:   &noopPlayerMgr{},
		TeamMgr:     &noopTeamMgr{},
		SeasonMgr:   &noopSeasonMgr{},
		WeekMgr:     &noopWeekMgr{},
		RoundMgr:    &noopRoundMgr{},
		ApplyAuth:   auth,
	}
}

// makeUserCtxRequest builds a request with a *models.User stored in clearanceUserKey context.
func makeUserCtxRequest(user *models.User) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if user != nil {
		ctx := context.WithValue(req.Context(), clearanceUserKey{}, user)
		req = req.WithContext(ctx)
	}
	return req
}

// --- requirePersonalKeyAuth unit tests ---

func TestRequirePersonalKeyAuth_NoHeader_Returns401(t *testing.T) {
	h := requirePersonalKeyAuth(&stubApplyAuth{}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestRequirePersonalKeyAuth_NoHeader_SetsWWWAuthenticate(t *testing.T) {
	h := requirePersonalKeyAuth(&stubApplyAuth{}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if got := w.Header().Get("WWW-Authenticate"); got != `Bearer realm="league-admin"` {
		t.Errorf("want WWW-Authenticate header, got %q", got)
	}
}

func TestRequirePersonalKeyAuth_InvalidToken_Returns403(t *testing.T) {
	h := requirePersonalKeyAuth(&stubApplyAuth{}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for unresolved token, got %d", w.Code)
	}
}

func TestRequirePersonalKeyAuth_ValidKey_SetsUserInContext(t *testing.T) {
	user := &models.User{ID: 7, Username: "alice", Role: "league_admin", Active: true}
	resolver := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	var gotUser *models.User
	h := requirePersonalKeyAuth(resolver, func(w http.ResponseWriter, r *http.Request) {
		gotUser = clearanceUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 for valid key, got %d", w.Code)
	}
	if gotUser == nil || gotUser.ID != 7 {
		t.Errorf("want user ID 7 in context, got %v", gotUser)
	}
}

// --- requireLeagueAdminRole unit tests ---

func TestRequireLeagueAdminRole_NoUser_Returns403(t *testing.T) {
	h := requireLeagueAdminRole(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 when no user in context, got %d", w.Code)
	}
}

func TestRequireLeagueAdminRole_ScoreKeeper_Returns403(t *testing.T) {
	h := requireLeagueAdminRole(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	h(w, makeUserCtxRequest(&models.User{ID: 1, Role: "score_keeper"}))
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

func TestRequireLeagueAdminRole_LeagueAdmin_Allowed(t *testing.T) {
	h := requireLeagueAdminRole(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	h(w, makeUserCtxRequest(&models.User{ID: 1, Role: "league_admin"}))
	if w.Code != http.StatusOK {
		t.Errorf("want 200 for league_admin, got %d", w.Code)
	}
}

func TestRequireLeagueAdminRole_AdminCompat_Allowed(t *testing.T) {
	h := requireLeagueAdminRole(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	// role="admin" is the backward-compatible alias for league_admin.
	h(w, makeUserCtxRequest(&models.User{ID: 1, Role: "admin"}))
	if w.Code != http.StatusOK {
		t.Errorf("want 200 for admin (backward-compatible alias), got %d", w.Code)
	}
}

func TestRequireLeagueAdminRole_SystemAdmin_Allowed(t *testing.T) {
	h := requireLeagueAdminRole(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	h(w, makeUserCtxRequest(&models.User{ID: 1, Role: "system_admin"}))
	if w.Code != http.StatusOK {
		t.Errorf("want 200 for system_admin, got %d", w.Code)
	}
}

// --- Route auth integration tests ---

func TestClearanceRoute_CloseWeek_NoHeader_Returns401(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/weeks/1/close", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("want WWW-Authenticate header on 401")
	}
}

func TestClearanceRoute_CloseWeek_InvalidToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/weeks/1/close", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}

// TestClearanceRoute_CloseWeek_StaticAdminToken_Returns403 verifies that the static
// LEAGUE_ADMIN_TOKEN (used as a fallback for handicap-apply only) does not grant
// access to clearance routes. requirePersonalKeyAuth has no static-token path.
func TestClearanceRoute_CloseWeek_StaticAdminToken_Returns403(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "valid-user-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/weeks/1/close", nil)
	req.Header.Set("Authorization", "Bearer admin-token") // static env-var token, not a personal key
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize clearance routes), got %d", w.Code)
	}
}

func TestClearanceRoute_CloseWeek_ScoreKeeperRole_Returns403(t *testing.T) {
	scorer := &models.User{ID: 2, Role: "score_keeper", Active: true}
	auth := &stubApplyAuth{resolveKey: "scorer-key", resolveUser: scorer}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/weeks/1/close", nil)
	req.Header.Set("Authorization", "Bearer scorer-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

// --- Success path: all four clearance routes ---

func TestClearanceRoute_CloseWeek_ValidLeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/weeks/1/close",
		strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer my-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (handler reached with league_admin key), got %d: %s", w.Code, w.Body.String())
	}
}

// TestClearanceRoute_ReopenWeek_ValidAdmin_ReachesHandler verifies that role="admin"
// is accepted as the backward-compatible alias for league_admin.
func TestClearanceRoute_ReopenWeek_ValidAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/weeks/1/reopen", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (role=admin treated as league_admin), got %d: %s", w.Code, w.Body.String())
	}
}

// TestClearanceRoute_CloseSeason_ValidSystemAdmin_ReachesHandler verifies that
// system_admin is allowed to perform league_admin operations.
func TestClearanceRoute_CloseSeason_ValidSystemAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "system_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/close", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (system_admin allowed for league_admin operations), got %d: %s", w.Code, w.Body.String())
	}
}

func TestClearanceRoute_ReopenSeason_ValidLeagueAdmin_ReachesHandler(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), clearanceDeps(auth))

	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/reopen", nil)
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 for reopen season with league_admin key, got %d: %s", w.Code, w.Body.String())
	}
}
