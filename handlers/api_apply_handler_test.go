// Internal (white-box) test file for the postHandicapApply handler.
// Uses package handlers (not handlers_test) to access the unexported handler function.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/domains/handicaps"
)

// stubApplier implements HandicapApplier for handler tests.
type stubApplier struct {
	result handicaps.ApplyResult
	err    error
}

func (s *stubApplier) Apply(_ context.Context, _ int64, _ handicaps.ApplyRequest) (handicaps.ApplyResult, error) {
	return s.result, s.err
}

// capturingApplier records the last ApplyRequest passed to Apply for inspection.
type capturingApplier struct {
	lastReq *handicaps.ApplyRequest
	result  handicaps.ApplyResult
	err     error
}

func (c *capturingApplier) Apply(_ context.Context, _ int64, req handicaps.ApplyRequest) (handicaps.ApplyResult, error) {
	c.lastReq = &req
	return c.result, c.err
}

// doApply posts the given JSON body to the handler and returns the response.
func doApply(t *testing.T, svc HandicapApplier, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doApplyWithContext(t, context.Background(), svc, body)
}

// doApplyWithContext posts the given body with a pre-set context (for auth injection tests).
func doApplyWithContext(t *testing.T, ctx context.Context, svc HandicapApplier, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/seasons/1/handicap-apply", strings.NewReader(body))
	req = req.WithContext(ctx)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	postHandicapApply(w, req, svc)
	return w
}

// ─── Request validation at handler boundary ────────────────────────────────

func TestPostHandicapApply_MissingApplyRequestID_Returns400(t *testing.T) {
	svc := &stubApplier{}
	w := doApply(t, svc, `{"entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPostHandicapApply_MissingEntries_Returns400(t *testing.T) {
	svc := &stubApplier{}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPostHandicapApply_EntryMissingPlayerID_Returns400(t *testing.T) {
	svc := &stubApplier{}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPostHandicapApply_InvalidBody_Returns400(t *testing.T) {
	svc := &stubApplier{}
	w := doApply(t, svc, `not-json`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

// ─── Service error mapping ─────────────────────────────────────────────────

func TestPostHandicapApply_NotFound_Returns404(t *testing.T) {
	svc := &stubApplier{
		err: domainerr.New("HC_SEASON_NOT_FOUND", domainerr.NotFound, "season not found"),
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestPostHandicapApply_InvalidInput_Returns400(t *testing.T) {
	svc := &stubApplier{
		err: domainerr.New("HC_INVALID_REQUEST", domainerr.InvalidInput, "apply_request_id must be a UUID v4"),
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestPostHandicapApply_Unprocessable_Returns422(t *testing.T) {
	svc := &stubApplier{
		err: domainerr.New("HC_METHOD_NOT_APPLY", domainerr.Unprocessable, "apply is not supported for method \"manual_review\""),
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", w.Code)
	}
}

func TestPostHandicapApply_DomainConflict_Returns409(t *testing.T) {
	svc := &stubApplier{
		err: domainerr.New("HC_INVALID_REQUEST", domainerr.Conflict, "apply_request_id was already used with a different set of changes"),
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
}

func TestPostHandicapApply_Internal_Returns500(t *testing.T) {
	svc := &stubApplier{
		err: domainerr.Wrap("HC_DATA_ERROR", domainerr.Internal, "internal error", errors.New("db offline")),
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestPostHandicapApply_ApplyConflictErr_Returns409WithPayload(t *testing.T) {
	svc := &stubApplier{
		err: &handicaps.ApplyConflictErr{
			Conflicts: []handicaps.ApplyConflict{{
				PlayerID: 1,
				Code:     handicaps.ConflictTokenMismatch,
				Message:  "token mismatch",
			}},
		},
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["conflicts"]; !ok {
		t.Error("want 'conflicts' key in response body")
	}
}

func TestPostHandicapApply_ApplyRejectionErr_Returns422WithPayload(t *testing.T) {
	svc := &stubApplier{
		err: &handicaps.ApplyRejectionErr{
			Rejections: []handicaps.ApplyRejection{{
				PlayerID: 1,
				Code:     handicaps.RejectionBelowThreshold,
				Message:  "below threshold",
			}},
		},
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["rejections"]; !ok {
		t.Error("want 'rejections' key in response body")
	}
}

func TestPostHandicapApply_NonDomainError_Returns500Safe(t *testing.T) {
	svc := &stubApplier{
		err: errors.New("secret db path /var/db/prod.db"),
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"t"}]}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "secret db path") {
		t.Error("want cause not leaked in response body")
	}
}

// ─── Success ───────────────────────────────────────────────────────────────

func TestPostHandicapApply_Success_Returns200WithApplied(t *testing.T) {
	svc := &stubApplier{
		result: handicaps.ApplyResult{
			ApplyRequestID: "550e8400-e29b-41d4-a716-446655440000",
			Applied: []handicaps.AppliedChange{{
				PlayerID:    1,
				PlayerName:  "Alice",
				OldHandicap: 1.0,
				NewHandicap: 2.5,
			}},
		},
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.5,"rec_token":"tok"}]}`)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var result handicaps.ApplyResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("want 1 applied, got %d", len(result.Applied))
	}
	if result.Applied[0].NewHandicap != 2.5 {
		t.Errorf("new_handicap: want 2.5, got %f", result.Applied[0].NewHandicap)
	}
}

func TestPostHandicapApply_Success_Replayed_Returns200(t *testing.T) {
	svc := &stubApplier{
		result: handicaps.ApplyResult{
			ApplyRequestID: "550e8400-e29b-41d4-a716-446655440000",
			Applied:        []handicaps.AppliedChange{{PlayerID: 1, OldHandicap: 1.0, NewHandicap: 2.5}},
			Replayed:       true,
		},
	}
	w := doApply(t, svc, `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.5,"rec_token":"tok"}]}`)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	var result handicaps.ApplyResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !result.Replayed {
		t.Error("want replayed=true in response")
	}
}

// ─── Handler injection: AppliedByUserID from auth context ──────────────────

const oneEntryBody = `{"apply_request_id":"550e8400-e29b-41d4-a716-446655440000","entries":[{"player_id":1,"expected_assigned_hc":1.0,"expected_recommended_hc":2.0,"rec_token":"tok"}]}`

// TestPostHandicapApply_PersonalKeyContext_SetsAppliedByUserID verifies that
// when requireApplyAuth has placed a user ID in the context (personal key path),
// postHandicapApply propagates it to every ApplyEntry sent to the service.
func TestPostHandicapApply_PersonalKeyContext_SetsAppliedByUserID(t *testing.T) {
	svc := &capturingApplier{}
	ctx := context.WithValue(context.Background(), applyUserIDKey{}, int64(42))
	doApplyWithContext(t, ctx, svc, oneEntryBody)
	if svc.lastReq == nil {
		t.Fatal("want Apply to be called, was not")
	}
	for i, e := range svc.lastReq.Entries {
		if e.AppliedByUserID == nil {
			t.Errorf("entry[%d]: want AppliedByUserID=42, got nil", i)
		} else if *e.AppliedByUserID != 42 {
			t.Errorf("entry[%d]: want AppliedByUserID=42, got %d", i, *e.AppliedByUserID)
		}
	}
}

// TestPostHandicapApply_StaticTokenContext_NilAppliedByUserID verifies that
// when no user ID is in the context (static LEAGUE_ADMIN_TOKEN fallback path),
// every ApplyEntry has AppliedByUserID = nil.
func TestPostHandicapApply_StaticTokenContext_NilAppliedByUserID(t *testing.T) {
	svc := &capturingApplier{}
	// Plain background context: no user ID (simulates static-token fallback path).
	doApplyWithContext(t, context.Background(), svc, oneEntryBody)
	if svc.lastReq == nil {
		t.Fatal("want Apply to be called, was not")
	}
	for i, e := range svc.lastReq.Entries {
		if e.AppliedByUserID != nil {
			t.Errorf("entry[%d]: want AppliedByUserID=nil, got %d", i, *e.AppliedByUserID)
		}
	}
}
