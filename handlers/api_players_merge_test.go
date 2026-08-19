// Tests for POST /api/players/{id}/merge: route auth wiring, request
// parsing, and domainerr-to-HTTP-status mapping. players.PlayerService's own
// merge behavior (blockers, transaction, repointing) is covered by
// backend/domains/players and backend/storage/sqlite tests -- these tests
// only exercise the handler/route layer, using a stub PlayerManager.
package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domainerr"
	"league_app/models"
)

// stubMergePlayerMgr satisfies PlayerManager with configurable MergePlayers
// behavior; all other methods are the existing noopPlayerMgr no-ops.
type stubMergePlayerMgr struct {
	noopPlayerMgr
	mergeFn func(ctx context.Context, sourceID, targetID int64) error
}

func (s *stubMergePlayerMgr) MergePlayers(ctx context.Context, sourceID, targetID int64) error {
	if s.mergeFn != nil {
		return s.mergeFn(ctx, sourceID, targetID)
	}
	return nil
}

func mergePlayerDeps(auth ApplyAuthResolver, mgr *stubMergePlayerMgr) Dependencies {
	return Dependencies{
		HandicapSvc: &noopRecommender{},
		RuleMgr:     &noopRuleManager{},
		LeagueMgr:   &noopLeagueMgr{},
		PlayerMgr:   mgr,
		TeamMgr:     &noopTeamMgr{},
		SeasonMgr:   &noopSeasonMgr{},
		ApplyAuth:   auth,
	}
}

func TestMergePlayerRoute_NoAuthHeader_Returns401(t *testing.T) {
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: &models.User{ID: 1, Role: "league_admin"}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), mergePlayerDeps(auth, &stubMergePlayerMgr{}))

	req := httptest.NewRequest(http.MethodPost, "/api/players/1/merge", strings.NewReader(`{"target_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMergePlayerRoute_ScoreKeeperRole_Returns403(t *testing.T) {
	scorer := &models.User{ID: 2, Role: "score_keeper", Active: true}
	auth := &stubApplyAuth{resolveKey: "scorer-key", resolveUser: scorer}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), mergePlayerDeps(auth, &stubMergePlayerMgr{}))

	req := httptest.NewRequest(http.MethodPost, "/api/players/1/merge", strings.NewReader(`{"target_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer scorer-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", w.Code)
	}
}

func TestMergePlayerRoute_LeagueAdmin_Success_Returns200(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	var gotSource, gotTarget int64
	mgr := &stubMergePlayerMgr{mergeFn: func(_ context.Context, sourceID, targetID int64) error {
		gotSource, gotTarget = sourceID, targetID
		return nil
	}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), mergePlayerDeps(auth, mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/players/1/merge", strings.NewReader(`{"target_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotSource != 1 || gotTarget != 2 {
		t.Errorf("want handler to call MergePlayers(1, 2), got (%d, %d)", gotSource, gotTarget)
	}
	if !strings.Contains(w.Body.String(), `"status":"merged"`) {
		t.Errorf("want response body to report status merged, got: %s", w.Body.String())
	}
}

func TestMergePlayerRoute_MissingTargetID_Returns400(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), mergePlayerDeps(auth, &stubMergePlayerMgr{}))

	req := httptest.NewRequest(http.MethodPost, "/api/players/1/merge", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for missing target_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMergePlayerRoute_SamePlayerConflict_Returns400(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mgr := &stubMergePlayerMgr{mergeFn: func(_ context.Context, _, _ int64) error {
		return domainerr.New("PLAYER_MERGE_SAME_PLAYER", domainerr.InvalidInput,
			"source and target player must be different")
	}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), mergePlayerDeps(auth, mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/players/1/merge", strings.NewReader(`{"target_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for source==target, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMergePlayerRoute_SourceNotFound_Returns404(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mgr := &stubMergePlayerMgr{mergeFn: func(_ context.Context, _, _ int64) error {
		return domainerr.New("PLAYER_MERGE_SOURCE_NOT_FOUND", domainerr.NotFound, "source player not found")
	}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), mergePlayerDeps(auth, mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/players/999/merge", strings.NewReader(`{"target_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing source player, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMergePlayerRoute_UnsafeMergeConflict_Returns409(t *testing.T) {
	user := &models.User{ID: 1, Role: "league_admin", Active: true}
	auth := &stubApplyAuth{resolveKey: "my-key", resolveUser: user}
	mgr := &stubMergePlayerMgr{mergeFn: func(_ context.Context, _, _ int64) error {
		return domainerr.New("PLAYER_MERGE_SEASON_ROSTER_CONFLICT", domainerr.Conflict,
			"source and target are both rostered in the same season; resolve the roster before merging")
	}}
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), mergePlayerDeps(auth, mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/players/1/merge", strings.NewReader(`{"target_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("want 409 for unsafe-merge conflict, got %d: %s", w.Code, w.Body.String())
	}
}
