package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
)

// stubPushbackMgr is an injectable stub for PushbackPreviewer.
type stubPushbackMgr struct {
	result matches.PushbackPreviewResult
	err    error
}

func (s *stubPushbackMgr) Preview(_ context.Context, _ matches.PushbackPreviewRequest) (matches.PushbackPreviewResult, error) {
	return s.result, s.err
}

// pushbackDeps returns a minimal Dependencies with only PushbackMgr set.
func pushbackDeps(mgr PushbackPreviewer) Dependencies {
	return Dependencies{
		HandicapSvc: &noopRecommender{},
		RuleMgr:     &noopRuleManager{},
		LeagueMgr:   &noopLeagueMgr{},
		PlayerMgr:   &noopPlayerMgr{},
		TeamMgr:     &noopTeamMgr{},
		SeasonMgr:   &noopSeasonMgr{},
		PushbackMgr: mgr,
	}
}

func doPushbackPreview(t *testing.T, mgr PushbackPreviewer, seasonID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/seasons/"+seasonID+"/schedule/pushback-preview",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux, t.TempDir(), pushbackDeps(mgr))
	mux.ServeHTTP(w, req)
	return w
}

func TestPushbackPreview_200_ReturnsResult(t *testing.T) {
	date := "2026-10-13"
	mgr := &stubPushbackMgr{
		result: matches.PushbackPreviewResult{
			Shifted: []matches.ShiftedMatch{
				{ID: 1, WeekNumber: 5, NewWeekNumber: 6, HomeTeamID: 1, AwayTeamID: 2, NewMatchDate: &date},
			},
			Preserved:  []matches.PreservedMatch{},
			NewEndDate: &date,
		},
	}
	w := doPushbackPreview(t, mgr, "1", map[string]int{"cutoff_week": 5, "weeks_to_add": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var got matches.PushbackPreviewResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Shifted) != 1 {
		t.Errorf("want 1 shifted match, got %d", len(got.Shifted))
	}
}

func TestPushbackPreview_400_InvalidCutoff(t *testing.T) {
	mgr := &stubPushbackMgr{
		err: domainerr.New("PUSHBACK_INVALID_CUTOFF", domainerr.InvalidInput, "cutoff_week must be at least 1"),
	}
	w := doPushbackPreview(t, mgr, "1", map[string]int{"cutoff_week": 0, "weeks_to_add": 1})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPushbackPreview_400_InvalidWeeksToAdd(t *testing.T) {
	mgr := &stubPushbackMgr{
		err: domainerr.New("PUSHBACK_INVALID_WEEKS_TO_ADD", domainerr.InvalidInput, "weeks_to_add must be at least 1"),
	}
	w := doPushbackPreview(t, mgr, "1", map[string]int{"cutoff_week": 3, "weeks_to_add": 0})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPushbackPreview_409_ClosedWeeks(t *testing.T) {
	mgr := &stubPushbackMgr{
		err: domainerr.New("PUSHBACK_HAS_CLOSED_WEEKS", domainerr.Conflict, "cannot pushback: closed weeks at or after cutoff"),
	}
	w := doPushbackPreview(t, mgr, "1", map[string]int{"cutoff_week": 5, "weeks_to_add": 1})
	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
}

func TestPushbackPreview_404_SeasonNotFound(t *testing.T) {
	mgr := &stubPushbackMgr{
		err: domainerr.New("PUSHBACK_SEASON_NOT_FOUND", domainerr.NotFound, "season not found"),
	}
	w := doPushbackPreview(t, mgr, "99", map[string]int{"cutoff_week": 1, "weeks_to_add": 1})
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestPushbackPreview_400_InvalidSeasonID(t *testing.T) {
	mgr := &stubPushbackMgr{}
	w := doPushbackPreview(t, mgr, "notanid", map[string]int{"cutoff_week": 1, "weeks_to_add": 1})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for non-numeric season id, got %d", w.Code)
	}
}
