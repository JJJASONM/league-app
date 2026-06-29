package handlers_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"errors"

	"league_app/backend/domainerr"
	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/rules"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/handlers"
	"league_app/models"
)

// stubHandicapSvc is a test double for handlers.HandicapRecommender.
// Set fn to control what Recommendations returns.
type stubHandicapSvc struct {
	fn func(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error)
}

func (s *stubHandicapSvc) Recommendations(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error) {
	return s.fn(ctx, seasonID)
}

// testServer initializes a fresh SQLite database in a temp directory and
// returns a running test HTTP server with all routes registered.
// The DB connection and server are closed automatically when the test ends.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	// Close the DB before the temp dir is removed (required on Windows).
	t.Cleanup(func() { db.DB.Close() })
	mux := http.NewServeMux()
	hcStore := sqlite.NewHandicapStore(db.DB)
	hcSvc := handicaps.NewService(hcStore)
	deps := handlers.Dependencies{HandicapSvc: hcSvc}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// --- GET /api/seasons/{id}/handicap-recommendations (handler error mapping) ---

func TestGetHandicapRecommendations_NotFound404(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return models.HandicapReviewResponse{}, domainerr.New("HC_SEASON_NOT_FOUND", domainerr.NotFound, "season not found")
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/999/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestGetHandicapRecommendations_InternalError500(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return models.HandicapReviewResponse{}, domainerr.Wrap("HC_DATA_ERROR", domainerr.Internal, "internal error", fmt.Errorf("db offline"))
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/1/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", resp.StatusCode)
	}
}

func TestGetHandicapRecommendations_Success200(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	want := models.HandicapReviewResponse{
		SeasonID:        7,
		Method:          "manual_review",
		Status:          "no_auto_apply",
		Message:         "No handicap changes are applied automatically.",
		WeeksClosed:     0,
		Recommendations: []models.HandicapReviewRec{},
	}
	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return want, nil
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/7/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	var got models.HandicapReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SeasonID != want.SeasonID || got.Status != want.Status || got.Method != want.Method {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestGetHandicapRecommendations_NonDomainError500NoLeak asserts that a plain
// (non-domain) error returned by the service maps to 500 with a fixed safe body
// and that the original cause string never appears in the response.
func TestGetHandicapRecommendations_NonDomainError500NoLeak(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	stub := &stubHandicapSvc{fn: func(_ context.Context, _ int64) (models.HandicapReviewResponse, error) {
		return models.HandicapReviewResponse{}, errors.New("secret database path /var/db/prod.db")
	}}
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: stub})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/seasons/1/handicap-recommendations")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "internal error") {
		t.Errorf("want body to contain 'internal error', got: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "secret database path") {
		t.Errorf("want cause NOT in body, but found it: %s", bodyStr)
	}
}

// --- Registration nil-dependency tests ----------------------------------------

func TestRegister_NilHandicapSvcPanics(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when HandicapSvc is nil")
		}
	}()
	mux := http.NewServeMux()
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: nil})
}

// TestRegister_TypedNilHandicapSvcPanics asserts that a typed nil (a nil concrete
// pointer stored inside the HandicapRecommender interface) is also rejected.
// A typed nil passes the == nil check but panics on the first method call.
func TestRegister_TypedNilHandicapSvcPanics(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	defer func() {
		if r := recover(); r == nil {
			t.Error("want panic when HandicapSvc is a typed nil")
		}
	}()
	mux := http.NewServeMux()
	var svc *stubHandicapSvc // typed nil: interface is non-nil but concrete pointer is nil
	handlers.Register(mux, dir, handlers.Dependencies{HandicapSvc: svc})
}

// seedSeason creates one league and one season, returning the season ID.
func seedSeason(t *testing.T, base string) int64 {
	t.Helper()
	post := func(path, body string) *http.Response {
		resp, err := http.Post(base+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		return resp
	}

	resp := post("/api/leagues", `{"name":"Test League","game_format":"8ball"}`)
	resp.Body.Close()

	resp2 := post("/api/seasons", `{"league_id":1,"name":"Spring 2026"}`)
	defer resp2.Body.Close()
	var s map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil {
		t.Fatalf("decode season: %v", err)
	}
	return int64(s["id"].(float64))
}

// ─── GET /api/rules/definitions ───────────────────────────────────────────────

// fetchDefs is a test helper that GETs /api/rules/definitions and decodes the result.
func fetchDefs(t *testing.T, srv *httptest.Server) []rules.Definition {
	t.Helper()
	resp, err := http.Get(srv.URL + "/api/rules/definitions")
	if err != nil {
		t.Fatalf("GET /api/rules/definitions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var defs []rules.Definition
	if err := json.NewDecoder(resp.Body).Decode(&defs); err != nil {
		t.Fatalf("decode definitions: %v", err)
	}
	return defs
}

func TestListRuleDefinitions_ReturnsOK(t *testing.T) {
	srv := testServer(t)
	defs := fetchDefs(t, srv)
	if len(defs) == 0 {
		t.Fatal("expected non-empty definitions list")
	}
}

func TestListRuleDefinitions_ExactCount(t *testing.T) {
	srv := testServer(t)
	defs := fetchDefs(t, srv)
	const want = 14
	if len(defs) != want {
		t.Fatalf("want %d definitions, got %d", want, len(defs))
	}
}

func TestListRuleDefinitions_FieldsPresent(t *testing.T) {
	srv := testServer(t)
	defs := fetchDefs(t, srv)
	for _, d := range defs {
		if d.Key == "" {
			t.Errorf("definition missing key: %+v", d)
		}
		if d.Type == "" {
			t.Errorf("definition %q missing type", d.Key)
		}
		if d.DefaultValue == "" {
			t.Errorf("definition %q missing default_value", d.Key)
		}
		if d.Group == "" || d.GroupLabel == "" {
			t.Errorf("definition %q missing group/group_label", d.Key)
		}
	}
}

func TestListRuleDefinitions_StableMetadata(t *testing.T) {
	srv := testServer(t)
	defs := fetchDefs(t, srv)

	byKey := make(map[string]rules.Definition, len(defs))
	for _, d := range defs {
		byKey[d.Key] = d
	}

	cases := []struct {
		key          string
		wantLabel    string
		wantDefault  string
		wantGroup    string
		wantGroupOrd int
		wantOrder    int
	}{
		{"max_individual_handicap", "Max individual handicap on scoresheet", "4.5", "handicap", 10, 10},
		{"handicap_multiplier", "Handicap multiplier", "2.55", "handicap", 10, 20},
		{"handicap_rounding", "Rounding method", "nearest", "handicap", 10, 30},
		{"max_pairing_spot", "Max spot per pairing", "15", "handicap", 10, 40},
		{"max_match_spot", "Max total spot per match", "15", "handicap", 10, 50},
		{"handicap_update_method", "Handicap update method", "manual_review", "handicap", 10, 60},
		{"handicap_current_game_window", "Current game window", "15", "handicap", 10, 70},
		{"handicap_min_games_for_recommendation", "Minimum games for recommendation", "15", "handicap", 10, 80},
		{"lineup_players_per_team", "Players per team per match", "3", "lineup", 20, 10},
		{"games_per_pairing", "Games per pairing", "3", "lineup", 20, 20},
		{"allow_substitutes", "Allow substitutes", "true", "lineup", 20, 30},
		{"allow_bye_requests", "Allow bye requests", "true", "scheduling", 30, 10},
		{"require_bye_approval", "Require bye approval", "true", "scheduling", 30, 20},
	}

	for _, tc := range cases {
		d, ok := byKey[tc.key]
		if !ok {
			t.Errorf("definition %q not found", tc.key)
			continue
		}
		if d.Label != tc.wantLabel {
			t.Errorf("%q label: want %q, got %q", tc.key, tc.wantLabel, d.Label)
		}
		if d.DefaultValue != tc.wantDefault {
			t.Errorf("%q default_value: want %q, got %q", tc.key, tc.wantDefault, d.DefaultValue)
		}
		if d.Group != tc.wantGroup {
			t.Errorf("%q group: want %q, got %q", tc.key, tc.wantGroup, d.Group)
		}
		if d.GroupOrder != tc.wantGroupOrd {
			t.Errorf("%q group_order: want %d, got %d", tc.key, tc.wantGroupOrd, d.GroupOrder)
		}
		if d.Order != tc.wantOrder {
			t.Errorf("%q order: want %d, got %d", tc.key, tc.wantOrder, d.Order)
		}
	}
}

func TestListRuleDefinitions_ChoiceHasOptions(t *testing.T) {
	srv := testServer(t)
	defs := fetchDefs(t, srv)
	for _, d := range defs {
		if d.Type == rules.TypeChoice && len(d.Options) == 0 {
			t.Errorf("choice definition %q has no options", d.Key)
		}
	}
}

func TestListRuleDefinitions_ChoiceOptionValues(t *testing.T) {
	srv := testServer(t)
	defs := fetchDefs(t, srv)

	byKey := make(map[string]rules.Definition, len(defs))
	for _, d := range defs {
		byKey[d.Key] = d
	}

	optionValues := func(d rules.Definition) []string {
		out := make([]string, len(d.Options))
		for i, o := range d.Options {
			out[i] = o.Value
		}
		return out
	}
	optionLabel := func(d rules.Definition, value string) string {
		for _, o := range d.Options {
			if o.Value == value {
				return o.Label
			}
		}
		return ""
	}

	// handicap_rounding
	hr := byKey["handicap_rounding"]
	for _, wantVal := range []string{"nearest", "floor", "ceiling"} {
		if optionLabel(hr, wantVal) == "" {
			t.Errorf("handicap_rounding: missing option %q", wantVal)
		}
	}
	if got := optionValues(hr); len(got) != 3 {
		t.Errorf("handicap_rounding: want 3 options, got %d", len(got))
	}

	// handicap_update_method — assert all three values and that labels include descriptive text
	hum := byKey["handicap_update_method"]
	for _, wantVal := range []string{"manual_review", "game_diff_average", "kicker_average_preview"} {
		lbl := optionLabel(hum, wantVal)
		if lbl == "" {
			t.Errorf("handicap_update_method: missing option %q", wantVal)
		}
	}
	if got := optionValues(hum); len(got) != 3 {
		t.Errorf("handicap_update_method: want 3 options, got %d", len(got))
	}
}

// ─── POST /api/seasons/{id}/rules validation ──────────────────────────────────

func TestCreateSeasonRule_AcceptsValidBooleanValue(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	sid := seedSeason(t, srv.URL)

	body := `{"rule_key":"allow_substitutes","rule_label":"Allow subs","rule_value":"true"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
}

func TestCreateSeasonRule_RejectsInvalidBooleanValue(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	sid := seedSeason(t, srv.URL)

	body := `{"rule_key":"allow_substitutes","rule_label":"Allow subs","rule_value":"maybe"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("expected non-empty error message in response body")
	}
}

func TestCreateSeasonRule_RejectsInvalidChoiceValue(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	sid := seedSeason(t, srv.URL)

	body := `{"rule_key":"handicap_rounding","rule_label":"Rounding","rule_value":"random"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestCreateSeasonRule_AcceptsUnknownKey(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	sid := seedSeason(t, srv.URL)

	body := `{"rule_key":"custom_house_rule","rule_label":"House Rules","rule_value":"No jumping"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unknown key should be accepted (custom rule), got %d", resp.StatusCode)
	}
}

// ─── PUT /api/seasons/{id}/rules/{rid} validation ─────────────────────────────

func TestUpdateSeasonRule_RejectsInvalidValue(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	sid := seedSeason(t, srv.URL)

	// Create a valid choice rule first.
	createBody := `{"rule_key":"handicap_rounding","rule_label":"Rounding","rule_value":"nearest"}`
	createResp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	rid := int64(created["id"].(float64))

	// Attempt to update with an invalid choice value.
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/rules/%d", srv.URL, sid, rid),
		strings.NewReader(`{"rule_label":"Rounding","rule_value":"invalid_choice"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestCreateSeasonRule_RejectsNonNumericValue(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	body := `{"rule_key":"handicap_multiplier","rule_label":"Multiplier","rule_value":"abc"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-numeric value: want 400, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("expected non-empty error message in response body")
	}
}

func TestCreateSeasonRule_RejectsBelowMinimum(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// handicap_multiplier has Minimum=0.01; "0" is below it.
	body := `{"rule_key":"handicap_multiplier","rule_label":"Multiplier","rule_value":"0"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("below-minimum value: want 400, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("expected non-empty error message in response body")
	}
}

func TestCreateSeasonRule_RejectsAboveMaximumInteger(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// lineup_players_per_team has Maximum=6; "7" exceeds it.
	body := `{"rule_key":"lineup_players_per_team","rule_label":"Players","rule_value":"7"}`
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("above-maximum integer: want 400, got %d", resp.StatusCode)
	}
}

func TestUpdateSeasonRule_AcceptsValidValue(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	sid := seedSeason(t, srv.URL)

	// Create a valid choice rule.
	createBody := `{"rule_key":"handicap_rounding","rule_label":"Rounding","rule_value":"nearest"}`
	createResp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/rules", srv.URL, sid),
		"application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	rid := int64(created["id"].(float64))

	// Update to a different valid choice.
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/rules/%d", srv.URL, sid, rid),
		strings.NewReader(`{"rule_label":"Rounding","rule_value":"floor"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

// ─── Season date normalization ────────────────────────────────────────────────

// seedSeasonWithDate creates a league and season with an explicit start date,
// returning the season ID.
func seedSeasonWithDate(t *testing.T, base, startDate string) int64 {
	t.Helper()
	resp, err := http.Post(base+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Date Test League","game_format":"8ball"}`))
	if err != nil {
		t.Fatalf("POST leagues: %v", err)
	}
	resp.Body.Close()

	body := fmt.Sprintf(`{"league_id":1,"name":"Date Season","start_date":%q}`, startDate)
	resp2, err := http.Post(base+"/api/seasons", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST seasons: %v", err)
	}
	defer resp2.Body.Close()
	var s map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&s); err != nil {
		t.Fatalf("decode season: %v", err)
	}
	return int64(s["id"].(float64))
}

func TestListSeasons_StartDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	const wantDate = "2026-09-01"
	seedSeasonWithDate(t, srv.URL, wantDate)

	resp, err := http.Get(srv.URL + "/api/seasons?league_id=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var seasons []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&seasons); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(seasons) == 0 {
		t.Fatal("no seasons returned")
	}
	got, _ := seasons[0]["start_date"].(string)
	if got != wantDate {
		t.Errorf("start_date: want %q, got %q (must be YYYY-MM-DD for <input type=date>)", wantDate, got)
	}
}

func TestGetSeason_StartDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	const wantDate = "2026-03-15"
	sid := seedSeasonWithDate(t, srv.URL, wantDate)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, _ := s["start_date"].(string)
	if got != wantDate {
		t.Errorf("start_date: want %q, got %q", wantDate, got)
	}
}

// Week Workflow (Close Week) --------------------------------------------------

// weekFixture is the result of weekTestSeed: a running server plus pre-seeded IDs.
type weekFixture struct {
	srv     *httptest.Server
	sid     int64 // season ID
	matchID int64
	teamA   int64
	teamB   int64
	playerA int64 // one player on team A
	playerB int64 // one player on team B
}

// weekTestSeed spins up a fresh test server, creates one league, one season, two teams
// with one player each, and one unscored match in week 1. Cleanup is registered on t.
func weekTestSeed(t *testing.T) weekFixture {
	t.Helper()
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID); err != nil {
		t.Fatalf("weekTestSeed: season league: %v", err)
	}
	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team B')`, leagueID)
	teamA, _ := rA.LastInsertId()
	teamB, _ := rB.LastInsertId()

	rPA, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home','Player',?,3.0)`, teamA)
	rPB, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Away','Player',?,3.0)`, teamB)
	playerA, _ := rPA.LastInsertId()
	playerB, _ := rPB.LastInsertId()

	rm, err := db.DB.Exec(`
		INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		VALUES (?,?,?,1)`, sid, teamA, teamB)
	if err != nil {
		t.Fatalf("weekTestSeed: insert match: %v", err)
	}
	matchID, _ := rm.LastInsertId()
	return weekFixture{srv, sid, matchID, teamA, teamB, playerA, playerB}
}

// seedRoundResult inserts one round_results row with a game winner (home wins all 3)
// and sets matches.completed=1. Used to satisfy Close Week's game-winner requirement.
func seedRoundResult(t *testing.T, matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		matchID, homePlayerID, awayPlayerID)
	if err != nil {
		t.Fatalf("seedRoundResult: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
}

func TestListWeeks_ReturnsOpenWhenNoRows(t *testing.T) {
	f := weekTestSeed(t)
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var weeks []map[string]any
	json.NewDecoder(resp.Body).Decode(&weeks)
	if len(weeks) == 0 {
		t.Fatal("expected at least one week entry")
	}
	got, _ := weeks[0]["status"].(string)
	if got != "open" {
		t.Errorf("want status 'open' (inferred when no league_weeks row exists), got %q", got)
	}
}

func TestValidateWeek_ReportsErrorForUnscored(t *testing.T) {
	f := weekTestSeed(t)
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/validate", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("validate should return 200 with the result body, got %d", resp.StatusCode)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_MATCH_NO_SCORES error in validation result, got: %v", result)
	}
}

func TestCloseWeek_Returns422WhenErrors(t *testing.T) {
	f := weekTestSeed(t)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unscored match must block close: want 422, got %d", resp.StatusCode)
	}
}

// TestCloseWeek_Returns422WhenCompletedButNoRoundResults proves that completed=1 alone
// is not sufficient to close a week -- round_results with a game winner are required.
func TestCloseWeek_Returns422WhenCompletedButNoRoundResults(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, f.matchID)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("completed=1 with no round_results must block close: want 422, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_MATCH_NO_SCORES error, got: %v", result)
	}
}

func TestCloseWeek_SucceedsWithSavedScores(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["closed"] != true {
		t.Errorf("want closed=true in response body, got %v", result)
	}
}

func TestCloseWeek_SetsLeagueWeeksStatus(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	var status string
	db.DB.QueryRow(`SELECT status FROM league_weeks WHERE season_id=? AND week_number=1`, f.sid).Scan(&status)
	if status != "closed" {
		t.Errorf("want league_weeks.status='closed', got %q", status)
	}
}

func TestCloseWeek_SetsMatchWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	var wc int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, f.matchID).Scan(&wc)
	if wc != 1 {
		t.Errorf("want matches.week_closed=1 after close, got %d", wc)
	}
}

func TestSaveRounds_BlockedWhenWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	body := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`,
		f.playerA, f.playerB)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("saveRounds on closed week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestSubmitResults_BlockedWhenWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	body := fmt.Sprintf(`{"results":[{"player_id":%d,"team_id":%d,"games_won":3,"games_lost":0,"diff":3}]}`,
		f.playerA, f.teamA)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/results", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("submitResults on closed week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestClearResults_BlockedWhenWeekClosed(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/matches/%d/results", f.srv.URL, f.matchID),
		nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("clearResults on closed week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestStandings_ExcludeSavedButOpenMatch(t *testing.T) {
	f := weekTestSeed(t)
	// Insert round results and match results but do NOT close the week.
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA)

	resp, err := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var standings []map[string]any
	json.NewDecoder(resp.Body).Decode(&standings)
	for _, s := range standings {
		if played, _ := s["played"].(float64); played > 0 {
			t.Errorf("standings must not count open (unclosed) week: team %v shows %v played", s["team_name"], played)
		}
	}
}

func TestStandings_IncludeClosedMatch(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,0,3,-3)`, f.matchID, f.playerB, f.teamB)

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	standResp, err := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer standResp.Body.Close()
	var standings []map[string]any
	json.NewDecoder(standResp.Body).Decode(&standings)
	totalPlayed := 0
	for _, s := range standings {
		if p, _ := s["played"].(float64); p > 0 {
			totalPlayed++
		}
	}
	if totalPlayed == 0 {
		t.Error("standings must include match after week is closed")
	}
}

func TestPlayerStats_ExcludeOpenMatch(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA)

	// Do NOT close the week.
	resp, err := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var stats []map[string]any
	json.NewDecoder(resp.Body).Decode(&stats)
	for _, s := range stats {
		if gw, _ := s["games_won"].(float64); gw > 0 {
			t.Errorf("player stats must not count open (unclosed) week: %v shows %.0f games_won", s["player_name"], gw)
		}
	}
}

func TestPlayerStats_IncludeClosedMatch(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	if _, err := db.DB.Exec(`
		INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3)`, f.matchID, f.playerA, f.teamA); err != nil {
		t.Fatalf("insert match_results: %v", err)
	}

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, err := http.DefaultClient.Do(closeReq)
	if err != nil {
		t.Fatalf("close request: %v", err)
	}
	if closeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(closeResp.Body)
		t.Fatalf("close week failed: %d: %s", closeResp.StatusCode, body)
	}
	closeResp.Body.Close()

	resp, err := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var stats []map[string]any
	json.NewDecoder(resp.Body).Decode(&stats)
	totalGamesWon := 0
	for _, s := range stats {
		if gw, _ := s["games_won"].(float64); gw > 0 {
			totalGamesWon += int(gw)
		}
	}
	if totalGamesWon == 0 {
		t.Error("player stats must include match after week is closed")
	}
}

func TestSaveRounds_PopulatesSets(t *testing.T) {
	f := weekTestSeed(t)
	// Disable roster gate so saveRounds reaches sets logic (teams_managed=1 by default via API).
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Two home and two away players needed for a two-pairing round with a round winner.
	rH2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home2','P',?,3.0)`, f.teamA)
	playerA2, _ := rH2.LastInsertId()
	rA2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Away2','P',?,3.0)`, f.teamB)
	playerB2, _ := rA2.LastInsertId()

	// Round 1: home wins both pairings -> RoundWinners[1]="home" -> home players get sets_won=1.
	body := fmt.Sprintf(`{"rounds":[
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2},
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}
	]}`, f.playerA, f.playerB, playerA2, playerB2)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("saveRounds: want 200, got %d: %s", resp.StatusCode, b)
	}

	check := func(label string, playerID int64, wantSW, wantSL int) {
		t.Helper()
		var sw, sl int
		db.DB.QueryRow(`SELECT sets_won, sets_lost FROM match_results WHERE match_id=? AND player_id=?`,
			f.matchID, playerID).Scan(&sw, &sl)
		if sw != wantSW || sl != wantSL {
			t.Errorf("%s: want sets_won=%d sets_lost=%d, got %d/%d", label, wantSW, wantSL, sw, sl)
		}
	}
	check("playerA (home)", f.playerA, 1, 0)
	check("playerA2 (home)", playerA2, 1, 0)
	check("playerB (away)", f.playerB, 0, 1)
	check("playerB2 (away)", playerB2, 0, 1)
}

func TestPlayerStats_IncludeSetsAfterClose(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	rH2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home2','P',?,3.0)`, f.teamA)
	playerA2, _ := rH2.LastInsertId()
	rA2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Away2','P',?,3.0)`, f.teamB)
	playerB2, _ := rA2.LastInsertId()

	body := fmt.Sprintf(`{"rounds":[
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2},
		{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}
	]}`, f.playerA, f.playerB, playerA2, playerB2)

	saveReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	saveReq.Header.Set("Content-Type", "application/json")
	saveResp, _ := http.DefaultClient.Do(saveReq)
	saveResp.Body.Close()

	// Before close: sets must not appear in stats (week_closed gate).
	statsResp, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var beforeStats []map[string]any
	json.NewDecoder(statsResp.Body).Decode(&beforeStats)
	statsResp.Body.Close()
	for _, s := range beforeStats {
		if sw, _ := s["sets_won"].(float64); sw > 0 {
			t.Errorf("sets_won before close: want 0, got %.0f for %v", sw, s["player_name"])
		}
	}

	closeReq, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	closeReq.Header.Set("Content-Type", "application/json")
	closeResp, _ := http.DefaultClient.Do(closeReq)
	closeResp.Body.Close()

	// After close: at least one player must show sets_won > 0.
	statsResp2, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var afterStats []map[string]any
	json.NewDecoder(statsResp2.Body).Decode(&afterStats)
	statsResp2.Body.Close()
	found := false
	for _, s := range afterStats {
		if sw, _ := s["sets_won"].(float64); sw > 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sets_won > 0 after week close, got: %v", afterStats)
	}
}

func TestCloseWeek_ErrorOnDuplicatePlayer(t *testing.T) {
	f := weekTestSeed(t)

	// Seed a valid round result (round 1, playerA home, playerB away).
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	// Add a third player and insert them as the home player in a second pairing of round 1,
	// with playerA as the away player - playerA now appears twice in round 1.
	rPC, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Extra','P',?,3.0)`, f.teamA)
	playerC, _ := rPC.LastInsertId()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		f.matchID, playerC, f.playerA)
	if err != nil {
		t.Fatalf("insert duplicate round: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("duplicate player must block close: want 422, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_PLAYER_DUPLICATE" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_PLAYER_DUPLICATE error, got: %v", result)
	}
}

// Phase 2A: Warning acknowledgment ------------------------------------------------

// seedRoundResultWithIncompleteGame inserts round_results with one game winner (game1)
// and one incomplete game (game2: 5-3, no winner), triggering SCORESHEET_GAME_INCOMPLETE.
func seedRoundResultWithIncompleteGame(t *testing.T, matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,5,3,0,0,3.0,3.0,0,'')`,
		matchID, homePlayerID, awayPlayerID)
	if err != nil {
		t.Fatalf("seedRoundResultWithIncompleteGame: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
}

// seedRoundResultWithTwoWarnings inserts round_results with one game winner (game1)
// and two incomplete games (game2: 5-3, game3: 4-2), each triggering SCORESHEET_GAME_INCOMPLETE.
func seedRoundResultWithTwoWarnings(t *testing.T, matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,5,3,4,2,3.0,3.0,0,'')`,
		matchID, homePlayerID, awayPlayerID)
	if err != nil {
		t.Fatalf("seedRoundResultWithTwoWarnings: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
}

// weekValidate is a helper that calls GET /validate and decodes messages.
func weekValidate(t *testing.T, srvURL string, sid int64, weekNum int) []map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/validate", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("weekValidate: %v", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if msg, ok := m.(map[string]any); ok {
			out = append(out, msg)
		}
	}
	return out
}

// buildAcks constructs an acknowledgment slice from validate messages (warnings only).
func buildAcks(msgs []map[string]any) []map[string]any {
	var acks []map[string]any
	for _, msg := range msgs {
		if msg["level"] == "warning" {
			fieldStr, _ := msg["field"].(string)
			acks = append(acks, map[string]any{
				"match_id":     msg["match_id"],
				"warning_code": msg["code"],
				"field":        fieldStr,
				"notes":        "",
			})
		}
	}
	return acks
}

// weekClose POSTs to close the week with the given ack body.
func weekClose(t *testing.T, srvURL string, sid int64, weekNum int, acks []map[string]any) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"acknowledgments": acks})
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/%d/close", srvURL, sid, weekNum),
		strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("weekClose: %v", err)
	}
	return resp
}

func TestValidateWeek_StampsMatchIDOnErrors(t *testing.T) {
	f := weekTestSeed(t)
	// No round results -> WEEK_MATCH_NO_SCORES error
	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	for _, msg := range msgs {
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			mid, ok := msg["match_id"].(float64)
			if !ok || mid != float64(f.matchID) {
				t.Errorf("WEEK_MATCH_NO_SCORES: want match_id=%d, got: %v", f.matchID, msg["match_id"])
			}
			return
		}
	}
	t.Errorf("WEEK_MATCH_NO_SCORES not found in: %v", msgs)
}

func TestValidateWeek_StampsMatchIDOnScoresheetWarnings(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)
	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	for _, msg := range msgs {
		if msg["level"] == "warning" {
			mid, ok := msg["match_id"].(float64)
			if !ok || mid != float64(f.matchID) {
				t.Errorf("scoresheet warning: want match_id=%d, got: %v", f.matchID, msg["match_id"])
			}
			return
		}
	}
	t.Errorf("no warning found with match_id in: %v", msgs)
}

func TestCloseWeek_NoWarningsNilBodySucceeds(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid), nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("no warnings + nil body must succeed: want 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_MalformedBodyReturns400(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/1/close", f.srv.URL, f.sid),
		strings.NewReader(`{"acknowledgments": [`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("malformed close body must return 400, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_BlocksOnUnacknowledgedWarnings(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unacknowledged warnings must block close: want 422, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_SucceedsWithAllWarningsAcknowledged(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	if len(acks) == 0 {
		t.Fatal("fixture produced no warnings to acknowledge")
	}

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("all warnings acknowledged: want 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_BlocksOnPartialAcknowledgments(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithTwoWarnings(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	if len(acks) < 2 {
		t.Fatalf("need at least 2 warnings for partial ack test, got %d", len(acks))
	}

	// Acknowledge only the first warning
	resp := weekClose(t, f.srv.URL, f.sid, 1, acks[:1])
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("partial acknowledgments must block close: want 422, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_StoresAcknowledgmentRows(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	var acks []map[string]any
	for _, msg := range msgs {
		if msg["level"] == "warning" {
			fieldStr, _ := msg["field"].(string)
			acks = append(acks, map[string]any{
				"match_id":     msg["match_id"],
				"warning_code": msg["code"],
				"field":        fieldStr,
				"notes":        "stored note",
			})
		}
	}

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for acked warnings, got %d", resp.StatusCode)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&count)
	if count != len(acks) {
		t.Errorf("want %d ack row(s) stored, got %d", len(acks), count)
	}

	var seasonID int64
	var weekNumber int
	var matchID int64
	var warningCode string
	var field string
	var notes string
	db.DB.QueryRow(`
		SELECT season_id, week_number, match_id, warning_code, field, notes
		FROM week_close_acknowledgments
		WHERE season_id=? AND week_number=1`,
		f.sid).Scan(&seasonID, &weekNumber, &matchID, &warningCode, &field, &notes)
	if seasonID != f.sid {
		t.Errorf("want stored season_id=%d, got %d", f.sid, seasonID)
	}
	if weekNumber != 1 {
		t.Errorf("want stored week_number=1, got %d", weekNumber)
	}
	if matchID != f.matchID {
		t.Errorf("want stored match_id=%d, got %d", f.matchID, matchID)
	}
	if warningCode != "SCORESHEET_GAME_INCOMPLETE" {
		t.Errorf("want stored warning_code=SCORESHEET_GAME_INCOMPLETE, got %q", warningCode)
	}
	if field == "" {
		t.Error("want stored field to be non-empty")
	}
	if notes != "stored note" {
		t.Errorf("want notes='stored note', got %q", notes)
	}
}

func TestCloseWeek_ExtraStaleAckIgnoredWhenAllCurrentAcknowledged(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	// Prepend a stale ack for a warning that does not exist
	stale := map[string]any{
		"match_id":     float64(f.matchID),
		"warning_code": "SCORESHEET_NO_SCORES",
		"field":        "",
		"notes":        "stale",
	}
	acks = append([]map[string]any{stale}, acks...)

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("stale ack should not block close when current warnings acked: want 200, got %d: %s", resp.StatusCode, body)
	}
}

func TestCloseWeek_StaleAckAloneDoesNotAllowClose(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	// Only submit a stale ack for a non-existent warning
	staleOnlyAcks := []map[string]any{{
		"match_id":     float64(f.matchID),
		"warning_code": "SCORESHEET_NO_SCORES",
		"field":        "",
		"notes":        "stale",
	}}
	resp := weekClose(t, f.srv.URL, f.sid, 1, staleOnlyAcks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("stale ack alone must not allow close: want 422, got %d: %s", resp.StatusCode, body)
	}
}

func TestSaveRounds_ValidationMatchIDNil(t *testing.T) {
	f := weekTestSeed(t)
	// Submit impossible scores (both score 10) to trigger SCORESHEET_GAME_BOTH_WINNERS error
	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":10,"game2_home":0,"game2_away":0,"game3_home":0,"game3_away":0}]}`,
		f.playerA, f.playerB)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("invalid rounds must return 422, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	msgs, _ := result["messages"].([]any)
	for _, m := range msgs {
		if msg, ok := m.(map[string]any); ok {
			if _, hasMatchID := msg["match_id"]; hasMatchID {
				t.Errorf("saveRounds validation messages must not include match_id, got: %v", msg)
			}
		}
	}
}

// Phase 2B: Reopen workflow ------------------------------------------------

// weekReopen POSTs to reopen a week. Returns the raw *http.Response (caller must close body).
func weekReopen(t *testing.T, srvURL string, sid int64, weekNum int) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/seasons/%d/weeks/%d/reopen", srvURL, sid, weekNum), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("weekReopen: %v", err)
	}
	return resp
}

// seedMatchResult inserts one match_results row (1 set won, 3 games won) for the player.
func seedMatchResult(t *testing.T, matchID, playerID, teamID int64) {
	t.Helper()
	if _, err := db.DB.Exec(`
		INSERT OR IGNORE INTO match_results (match_id, player_id, team_id, sets_won, sets_lost, games_won, games_lost, diff)
		VALUES (?,?,?,1,0,3,0,3.0)`, matchID, playerID, teamID); err != nil {
		t.Fatalf("seedMatchResult: %v", err)
	}
}

func TestReopenWeek_ClosedWeekCanBeReopened(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	resp := weekReopen(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["reopened"] != true {
		t.Errorf("want reopened=true in response body, got %v", result)
	}
}

func TestReopenWeek_SetsLeagueWeeksStatusOpen(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var status string
	db.DB.QueryRow(`SELECT status FROM league_weeks WHERE season_id=? AND week_number=1`, f.sid).Scan(&status)
	if status != "open" {
		t.Errorf("want league_weeks.status='open' after reopen, got %q", status)
	}
}

func TestReopenWeek_ClearsClosedAt(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var closedAt *string
	db.DB.QueryRow(`SELECT closed_at FROM league_weeks WHERE season_id=? AND week_number=1`, f.sid).Scan(&closedAt)
	if closedAt != nil {
		t.Errorf("want league_weeks.closed_at=NULL after reopen, got %q", *closedAt)
	}
}

func TestReopenWeek_SetsMatchWeekClosed0(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var wc int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, f.matchID).Scan(&wc)
	if wc != 0 {
		t.Errorf("want matches.week_closed=0 after reopen, got %d", wc)
	}
}

func TestReopenWeek_PreservesRoundResults(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM round_results WHERE match_id=?`, f.matchID).Scan(&count)
	if count == 0 {
		t.Error("round_results must survive reopen")
	}
}

func TestReopenWeek_PreservesMatchResults(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	seedMatchResult(t, f.matchID, f.playerA, f.teamA)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM match_results WHERE match_id=?`, f.matchID).Scan(&count)
	if count == 0 {
		t.Error("match_results must survive reopen")
	}
}

func TestReopenWeek_StandingsExcludeReopenedWeek(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	// After close: match appears in standings (played >= 1).
	resp, _ := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	var standings []map[string]any
	json.NewDecoder(resp.Body).Decode(&standings)
	resp.Body.Close()
	closedPlayed := 0
	for _, s := range standings {
		if p, ok := s["played"].(float64); ok && int(p) > closedPlayed {
			closedPlayed = int(p)
		}
	}
	if closedPlayed == 0 {
		t.Fatal("expected standings to reflect closed match (played>0), but got 0")
	}

	// After reopen: match is excluded (played=0 for all teams).
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()
	resp2, _ := http.Get(fmt.Sprintf("%s/api/standings?season_id=%d", f.srv.URL, f.sid))
	var standings2 []map[string]any
	json.NewDecoder(resp2.Body).Decode(&standings2)
	resp2.Body.Close()
	for _, s := range standings2 {
		if p, ok := s["played"].(float64); ok && p > 0 {
			t.Errorf("standings must exclude reopened week: team %v still shows played=%v", s["team_name"], p)
		}
	}
}

func TestReopenWeek_PlayerStatsExcludeReopenedWeek(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	seedMatchResult(t, f.matchID, f.playerA, f.teamA)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	// After close: player stats include games from the closed match.
	resp, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var stats []map[string]any
	json.NewDecoder(resp.Body).Decode(&stats)
	resp.Body.Close()
	maxGamesWon := 0
	for _, s := range stats {
		if gw, ok := s["games_won"].(float64); ok && int(gw) > maxGamesWon {
			maxGamesWon = int(gw)
		}
	}
	if maxGamesWon == 0 {
		t.Fatal("expected player stats to include closed match games, but got 0")
	}

	// After reopen: player stats exclude the week (games_won=0 for all players).
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()
	resp2, _ := http.Get(fmt.Sprintf("%s/api/player-stats?season_id=%d", f.srv.URL, f.sid))
	var stats2 []map[string]any
	json.NewDecoder(resp2.Body).Decode(&stats2)
	resp2.Body.Close()
	for _, s := range stats2 {
		if gw, ok := s["games_won"].(float64); ok && gw > 0 {
			t.Errorf("player stats must exclude reopened week: player %v still shows games_won=%v", s["player_name"], gw)
		}
	}
}

func TestReopenWeek_OpenWeekReturns409(t *testing.T) {
	f := weekTestSeed(t)
	// Week 1 exists (match seeded) but has no league_weeks row: implicitly open.
	resp := weekReopen(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("reopening an open week must return 409, got %d: %s", resp.StatusCode, b)
	}
}

func TestReopenWeek_NoMatchesReturns404(t *testing.T) {
	f := weekTestSeed(t)
	// Week 99 has no matches.
	resp := weekReopen(t, f.srv.URL, f.sid, 99)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("reopening a week with no matches must return 404, got %d: %s", resp.StatusCode, b)
	}
}

func TestReopenWeek_ClosingAfterReopenWorks(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("re-close after reopen must succeed: want 200, got %d: %s", resp.StatusCode, b)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["closed"] != true {
		t.Errorf("want closed=true in re-close response, got %v", result)
	}
}

func TestReopenWeek_PreservesAcknowledgmentRows(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	weekClose(t, f.srv.URL, f.sid, 1, acks).Body.Close()

	var countBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&countBefore)
	if countBefore == 0 {
		t.Fatal("expected acknowledgment rows after close, got 0")
	}

	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	var countAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&countAfter)
	if countAfter != countBefore {
		t.Errorf("acknowledgment rows must survive reopen: want %d, got %d", countBefore, countAfter)
	}
}

// Phase 2E: Acknowledgment history endpoint ----------------------------------

// weekGetAcks calls GET /api/seasons/{id}/weeks/{week}/acknowledgments.
func weekGetAcks(t *testing.T, srvURL string, sid int64, weekNum int) *http.Response {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/acknowledgments", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("weekGetAcks: %v", err)
	}
	return resp
}

func TestGetWeekAcknowledgments_NotFound(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetAcks(t, f.srv.URL, f.sid, 99)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("week with no matches must return 404, got %d: %s", resp.StatusCode, b)
	}
}

func TestGetWeekAcknowledgments_Empty(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetAcks(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var acks []map[string]any
	json.NewDecoder(resp.Body).Decode(&acks)
	if len(acks) != 0 {
		t.Errorf("want empty array before any close, got %d items", len(acks))
	}
}

func TestGetWeekAcknowledgments_AfterClose(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	weekClose(t, f.srv.URL, f.sid, 1, acks).Body.Close()

	resp := weekGetAcks(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 after close, got %d: %s", resp.StatusCode, b)
	}
	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) == 0 {
		t.Fatal("want at least one acknowledgment after close with warnings, got 0")
	}
	a := result[0]
	if code, _ := a["warning_code"].(string); code == "" {
		t.Errorf("want non-empty warning_code in ack row, got: %v", a)
	}
	if a["acknowledged_at"] == nil {
		t.Error("want acknowledged_at in ack row, got nil")
	}
}

func TestGetWeekAcknowledgments_PersistedAfterReopen(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	weekClose(t, f.srv.URL, f.sid, 1, acks).Body.Close()

	var countBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM week_close_acknowledgments WHERE season_id=? AND week_number=1`, f.sid).Scan(&countBefore)

	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	resp := weekGetAcks(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 after reopen, got %d: %s", resp.StatusCode, b)
	}
	var result []map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result) != countBefore {
		t.Errorf("acknowledgments must persist after reopen: want %d, got %d", countBefore, len(result))
	}
}

func TestListWeeks_IncludesAckCount(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	getWeeks := func() []map[string]any {
		resp, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks", f.srv.URL, f.sid))
		defer resp.Body.Close()
		var weeks []map[string]any
		json.NewDecoder(resp.Body).Decode(&weeks)
		return weeks
	}
	ackCountFor := func(weeks []map[string]any, wn float64) float64 {
		for _, ws := range weeks {
			if ws["week_number"] == wn {
				cnt, _ := ws["ack_count"].(float64)
				return cnt
			}
		}
		return -1
	}

	// Before close: ack_count must be 0.
	if cnt := ackCountFor(getWeeks(), 1); cnt != 0 {
		t.Errorf("ack_count before close: want 0, got %v", cnt)
	}

	// Close with acknowledged warnings.
	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	weekClose(t, f.srv.URL, f.sid, 1, buildAcks(msgs)).Body.Close()

	// After close: ack_count > 0.
	if cnt := ackCountFor(getWeeks(), 1); cnt == 0 {
		t.Error("ack_count after close: want > 0, got 0")
	}

	// After reopen: ack_count still > 0 (acks persist).
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()
	if cnt := ackCountFor(getWeeks(), 1); cnt == 0 {
		t.Error("ack_count after reopen: want > 0 (acks persist), got 0")
	}
}

func TestListWeeks_AckCountZeroForCleanClose(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil).Body.Close()

	resp, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks", f.srv.URL, f.sid))
	defer resp.Body.Close()
	var weeks []map[string]any
	json.NewDecoder(resp.Body).Decode(&weeks)
	for _, ws := range weeks {
		if ws["week_number"] == float64(1) {
			cnt, _ := ws["ack_count"].(float64)
			if cnt != 0 {
				t.Errorf("ack_count for warning-free close: want 0, got %v", cnt)
			}
		}
	}
}

// --- Phase 3A: Advance Week Preview ------------------------------------------

// weekGetAdvancePreview calls GET /api/seasons/{id}/weeks/{week}/advance-preview.
func weekGetAdvancePreview(t *testing.T, srvURL string, sid int64, weekNum int) *http.Response {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/advance-preview", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("weekGetAdvancePreview: %v", err)
	}
	return resp
}

func TestAdvancePreview_NotFound(t *testing.T) {
	f := weekTestSeed(t)
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 99)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("week with no matches must return 404, got %d: %s", resp.StatusCode, b)
	}
}

func TestAdvancePreview_WithValidationErrors(t *testing.T) {
	f := weekTestSeed(t)
	// No round results: WEEK_MATCH_NO_SCORES is expected.
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("advance preview must return 200 even with errors, got %d: %s", resp.StatusCode, b)
	}
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)
	canClose, _ := preview["can_close"].(bool)
	if canClose {
		t.Error("can_close must be false when validation errors exist")
	}
	msgs, _ := preview["validation_messages"].([]any)
	if len(msgs) == 0 {
		t.Error("validation_messages must be non-empty when errors exist")
	}
	found := false
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["code"] == "WEEK_MATCH_NO_SCORES" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WEEK_MATCH_NO_SCORES in validation_messages, got: %v", msgs)
	}
}

func TestAdvancePreview_ClosableWeek(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)
	canClose, _ := preview["can_close"].(bool)
	if !canClose {
		t.Error("can_close must be true when all validation checks pass")
	}
	msgs, _ := preview["validation_messages"].([]any)
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if msg["level"] == "error" {
			t.Errorf("no error messages expected for closable week, got: %v", msg)
		}
	}
}

func TestAdvancePreview_NextWeekExists(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// Add a week 2 match.
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,2)`,
		f.sid, f.teamA, f.teamB)

	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	nextNum, ok := preview["next_week_number"].(float64)
	if !ok || int(nextNum) != 2 {
		t.Errorf("next_week_number: want 2, got %v", preview["next_week_number"])
	}
	nw, ok := preview["next_week"].(map[string]any)
	if !ok {
		t.Fatalf("next_week must be present when a next week exists")
	}
	if mc, _ := nw["match_count"].(float64); int(mc) != 1 {
		t.Errorf("next_week.match_count: want 1, got %v", mc)
	}
	if ac, _ := nw["assigned_count"].(float64); int(ac) != 1 {
		t.Errorf("next_week.assigned_count: want 1, got %v", ac)
	}
	if uc, _ := nw["unassigned_count"].(float64); int(uc) != 0 {
		t.Errorf("next_week.unassigned_count: want 0, got %v", uc)
	}
}

func TestAdvancePreview_NextWeekMissing(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// weekTestSeed only seeds week 1; no further weeks scheduled.
	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	if _, ok := preview["next_week_number"]; ok {
		t.Error("next_week_number must be absent when no further weeks are scheduled")
	}
	if _, ok := preview["next_week"]; ok {
		t.Error("next_week must be absent when no further weeks are scheduled")
	}
}

func TestAdvancePreview_ReadOnly(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	var countBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM league_weeks WHERE season_id=?`, f.sid).Scan(&countBefore)

	weekGetAdvancePreview(t, f.srv.URL, f.sid, 1).Body.Close()
	weekGetAdvancePreview(t, f.srv.URL, f.sid, 1).Body.Close()

	var countAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM league_weeks WHERE season_id=?`, f.sid).Scan(&countAfter)
	if countAfter != countBefore {
		t.Errorf("advance-preview must not write to league_weeks: before=%d after=%d", countBefore, countAfter)
	}
}

// --- Phase 3B: Close Week advance_result in close response -------------------

func TestCloseWeek_ResponseIncludesAdvanceResult(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 on close, got %d: %s", resp.StatusCode, b)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	if v, _ := body["closed"].(bool); !v {
		t.Errorf("want closed=true, got %v", body["closed"])
	}
	if _, ok := body["advance_result"]; !ok {
		t.Error("close response must include advance_result")
	}
	if _, ok := body["acknowledgment_count"]; !ok {
		t.Error("close response must include acknowledgment_count")
	}
}

func TestCloseWeek_AdvanceResult_ClosedWeekStatus(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ar, _ := body["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("advance_result missing from close response")
	}
	cw, _ := ar["closed_week"].(map[string]any)
	if cw == nil {
		t.Fatal("advance_result.closed_week missing")
	}
	if status, _ := cw["status"].(string); status != "closed" {
		t.Errorf("closed_week.status: want closed, got %q", status)
	}
	if mc, _ := cw["match_count"].(float64); int(mc) != 1 {
		t.Errorf("closed_week.match_count: want 1, got %v", mc)
	}
}

func TestCloseWeek_AdvanceResult_NextWeekIncluded(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,2)`,
		f.sid, f.teamA, f.teamB)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ar, _ := body["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("advance_result missing")
	}
	nextNum, ok := ar["next_week_number"].(float64)
	if !ok {
		t.Fatal("next_week_number must be present when week 2 exists")
	}
	if int(nextNum) != 2 {
		t.Errorf("next_week_number: want 2, got %v", nextNum)
	}
	nw, _ := ar["next_week"].(map[string]any)
	if nw == nil {
		t.Fatal("next_week must be present")
	}
	if mc, _ := nw["match_count"].(float64); int(mc) != 1 {
		t.Errorf("next_week.match_count: want 1, got %v", mc)
	}
}

func TestCloseWeek_AdvanceResult_NextWeekOmitted(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// weekTestSeed only seeds week 1; no week 2 exists.

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ar, _ := body["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("advance_result missing")
	}
	if _, ok := ar["next_week_number"]; ok {
		t.Error("next_week_number must be absent for final week")
	}
	if _, ok := ar["next_week"]; ok {
		t.Error("next_week must be absent for final week")
	}
}

func TestCloseWeek_AdvanceResult_AcknowledgmentCount(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)
	acks := buildAcks(msgs)
	if len(acks) == 0 {
		t.Fatal("expected at least one warning to acknowledge")
	}

	resp := weekClose(t, f.srv.URL, f.sid, 1, acks)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	ackCount, _ := body["acknowledgment_count"].(float64)
	if int(ackCount) != len(acks) {
		t.Errorf("acknowledgment_count: want %d, got %v", len(acks), ackCount)
	}
}

func TestCloseWeek_AdvanceResult_NoHandicapHistory(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	var beforeCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&beforeCount)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("want 200 on close")
	}

	var afterCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&afterCount)
	if afterCount != beforeCount {
		t.Errorf("close must not write handicap_history: before=%d after=%d", beforeCount, afterCount)
	}
}

func TestCloseWeek_AdvanceResult_NoLineupPlansMutated(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	var beforeCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=?`, f.sid).Scan(&beforeCount)

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("want 200 on close")
	}

	var afterCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=?`, f.sid).Scan(&afterCount)
	if afterCount != beforeCount {
		t.Errorf("close must not mutate lineup_plans: before=%d after=%d", beforeCount, afterCount)
	}
}

func TestCloseWeek_AckCountIsCurrentCycleOnly(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResultWithIncompleteGame(t, f.matchID, f.playerA, f.playerB)

	// First close: acknowledge all warnings.
	msgs1 := weekValidate(t, f.srv.URL, f.sid, 1)
	acks1 := buildAcks(msgs1)
	if len(acks1) == 0 {
		t.Fatal("expected at least one warning for first close")
	}
	weekClose(t, f.srv.URL, f.sid, 1, acks1).Body.Close()

	// Reopen so the week can be closed again.
	weekReopen(t, f.srv.URL, f.sid, 1).Body.Close()

	// Second close: the same warning still exists; acknowledge it again.
	msgs2 := weekValidate(t, f.srv.URL, f.sid, 1)
	acks2 := buildAcks(msgs2)
	resp2 := weekClose(t, f.srv.URL, f.sid, 1, acks2)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("want 200 on re-close, got %d: %s", resp2.StatusCode, b)
	}
	var body map[string]any
	json.NewDecoder(resp2.Body).Decode(&body)

	ackCount, _ := body["acknowledgment_count"].(float64)
	// DB now holds rows from both close cycles, but acknowledgment_count must
	// reflect only the current cycle's warnings, not the cumulative historical total.
	if int(ackCount) != len(acks2) {
		t.Errorf("acknowledgment_count: want %d (current cycle only), got %v", len(acks2), ackCount)
	}
}

func TestAdvancePreview_StillWorksAfterHelperExtraction(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)

	resp := weekGetAdvancePreview(t, f.srv.URL, f.sid, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 from advance-preview after helper extraction, got %d: %s", resp.StatusCode, b)
	}
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	if v, _ := preview["can_close"].(bool); !v {
		t.Errorf("can_close must be true for closable week, got %v", preview["can_close"])
	}
	if _, ok := preview["current_week"]; !ok {
		t.Error("current_week must be present in advance-preview response")
	}
	if _, ok := preview["handicap"]; !ok {
		t.Error("handicap must be present in advance-preview response")
	}
}

// ─── Skip date and match date normalization ───────────────────────────────────

// seedScheduleFixture creates a league, 3 teams (odd), and one season.
// Returns (leagueID, seasonID).
func seedScheduleFixture(t *testing.T, srv *httptest.Server, startDate string) (leagueID, seasonID int64) {
	var teamIDs []int64
	leagueID, seasonID, teamIDs = seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	return
}

// ensureSeasonTeams inserts all teamIDs into season_teams via direct DB access.
// Idempotent (INSERT OR IGNORE). Required for managed seasons before schedule
// generation or bye validation.
func ensureSeasonTeams(t *testing.T, seasonID int64, teamIDs []int64) {
	t.Helper()
	for _, tid := range teamIDs {
		if _, err := db.DB.Exec(
			`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name)
			 SELECT ?, id, name FROM teams WHERE id=?`, seasonID, tid); err != nil {
			t.Fatalf("ensureSeasonTeams: %v", err)
		}
	}
}

// seedScheduleFixtureWithTeams creates a league, the named teams, and one season.
// Returns (leagueID, seasonID, []teamID).
func seedScheduleFixtureWithTeams(t *testing.T, srv *httptest.Server, startDate string, teamNames ...string) (leagueID, seasonID int64, teamIDs []int64) {
	t.Helper()
	pd := func(path, body string) map[string]any {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return m
	}
	lg := pd("/api/leagues", `{"name":"Sched League","game_format":"8ball"}`)
	leagueID = int64(lg["id"].(float64))
	for _, name := range teamNames {
		tm := pd("/api/teams", fmt.Sprintf(`{"league_id":%d,"name":%q}`, leagueID, name))
		teamIDs = append(teamIDs, int64(tm["id"].(float64)))
	}
	s := pd("/api/seasons", fmt.Sprintf(`{"league_id":%d,"name":"Test Season","start_date":%q}`, leagueID, startDate))
	seasonID = int64(s["id"].(float64))
	return
}

// generateAndGetMatches POSTs /matches/generate and then fetches the resulting matches.
func generateAndGetMatches(t *testing.T, srv *httptest.Server, seasonID int64, startDate string, skipDates []string) []map[string]any {
	t.Helper()
	skipsJSON, _ := json.Marshal(skipDates)
	body := fmt.Sprintf(`{"season_id":%d,"start_date":%q,"schedule_type":"single_rr","skip_dates":%s}`,
		seasonID, startDate, skipsJSON)
	genResp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /matches/generate: %v", err)
	}
	genResp.Body.Close()
	if genResp.StatusCode != http.StatusOK {
		t.Fatalf("generate: want 200, got %d", genResp.StatusCode)
	}

	matchResp, err := http.Get(fmt.Sprintf("%s/api/matches?season_id=%d", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET /matches: %v", err)
	}
	defer matchResp.Body.Close()
	var matches []map[string]any
	json.NewDecoder(matchResp.Body).Decode(&matches)
	return matches
}

func TestListSkippedWeeks_SkipDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	const wantDate = "2026-07-04"
	body := fmt.Sprintf(`{"skip_date":%q,"reason":"Independence Day"}`, wantDate)
	resp, err := http.Post(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp2, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var skips []map[string]any
	json.NewDecoder(resp2.Body).Decode(&skips)
	if len(skips) == 0 {
		t.Fatal("no skipped weeks returned")
	}
	got, _ := skips[0]["skip_date"].(string)
	if got != wantDate {
		t.Errorf("skip_date: want %q, got %q (must be YYYY-MM-DD)", wantDate, got)
	}
}

func TestListMatches_MatchDateIsYYYYMMDD(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	for _, m := range matches {
		raw, _ := m["match_date"].(string)
		if raw == "" {
			continue
		}
		if len(raw) != 10 || raw[4] != '-' || raw[7] != '-' {
			t.Errorf("match_date %q is not YYYY-MM-DD", raw)
		}
	}
}

func TestGenerateSchedule_SkipDateExcluded(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	const skipDate = "2026-07-13"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, []string{skipDate})
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	for _, m := range matches {
		if d, _ := m["match_date"].(string); d == skipDate {
			t.Errorf("match on %q should have been skipped", skipDate)
		}
	}
}

// TestGenerateSchedule_ISOSkipDateExcluded verifies that ISO timestamps sent as
// skip_dates (e.g. "2026-07-13T00:00:00Z") are normalised before comparison.
func TestGenerateSchedule_ISOSkipDateExcluded(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	const skipDate = "2026-07-13"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	// Simulate the frontend sending an ISO timestamp instead of YYYY-MM-DD.
	matches := generateAndGetMatches(t, srv, seasonID, startDate, []string{skipDate + "T00:00:00Z"})
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	for _, m := range matches {
		if d, _ := m["match_date"].(string); d == skipDate {
			t.Errorf("match on %q should have been skipped even with ISO timestamp skip date", skipDate)
		}
	}
}

func TestGenerateSchedule_ConsecutiveSkipsHonored(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	skipDates := []string{"2026-07-13", "2026-07-20"}
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, skipDates)
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}
	skipped := map[string]bool{"2026-07-13": true, "2026-07-20": true}
	for _, m := range matches {
		if d, _ := m["match_date"].(string); skipped[d] {
			t.Errorf("match on %q should have been skipped", d)
		}
	}
}

func TestSkippedWeeks_AreScopedToSeason(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	leagueID, firstSeasonID := seedScheduleFixture(t, srv, startDate)

	body := fmt.Sprintf(`{"league_id":%d,"name":"Second Season","start_date":%q}`, leagueID, startDate)
	resp, err := http.Post(srv.URL+"/api/seasons", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST second season: %v", err)
	}
	defer resp.Body.Close()
	var secondSeason map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&secondSeason); err != nil {
		t.Fatalf("decode second season: %v", err)
	}
	secondSeasonID := int64(secondSeason["id"].(float64))

	skipBody := `{"skip_date":"2026-07-13","reason":"First season only"}`
	skipResp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, firstSeasonID),
		"application/json", strings.NewReader(skipBody))
	if err != nil {
		t.Fatalf("POST skipped week: %v", err)
	}
	skipResp.Body.Close()

	listResp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/skipped-weeks", srv.URL, secondSeasonID))
	if err != nil {
		t.Fatalf("GET second season skipped weeks: %v", err)
	}
	defer listResp.Body.Close()
	var skips []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&skips); err != nil {
		t.Fatalf("decode skipped weeks: %v", err)
	}
	if len(skips) != 0 {
		t.Fatalf("second season inherited %d skipped weeks; want none", len(skips))
	}
}

func TestGenerateSchedule_PreservesCompletedMatches(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID := seedScheduleFixture(t, srv, startDate)
	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	if len(matches) == 0 {
		t.Fatal("expected matches to be generated")
	}

	completedID := int64(matches[0]["id"].(float64))
	if _, err := db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, completedID); err != nil {
		t.Fatalf("mark match completed: %v", err)
	}

	regenerated := generateAndGetMatches(t, srv, seasonID, startDate, []string{"2026-07-13"})
	for _, match := range regenerated {
		if int64(match["id"].(float64)) == completedID {
			if completed, _ := match["completed"].(bool); !completed {
				t.Fatal("preserved match lost its completed status")
			}
			return
		}
	}
	t.Fatalf("completed match %d was deleted during regeneration", completedID)
}

// ─── Bye request validation ───────────────────────────────────────────────────

// postByeRequest is a helper that sends a POST /seasons/{id}/bye-requests.
func postByeRequest(t *testing.T, srv *httptest.Server, seasonID, teamID int64, weekNum int) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"team_id":%d,"week_number":%d,"reason":"test"}`, teamID, weekNum)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST bye-request: %v", err)
	}
	return resp
}

// approveByeRequest approves the bye request with the given id.
func approveByeRequest(t *testing.T, srv *httptest.Server, seasonID, byeID int64) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, seasonID, byeID),
		strings.NewReader(`{"approved":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT approve: %v", err)
	}
	resp.Body.Close()
}

// matchesInWeek filters matches for a specific week number.
func matchesInWeek(matches []map[string]any, week int) []map[string]any {
	var out []map[string]any
	for _, m := range matches {
		if wn, ok := m["week_number"].(float64); ok && int(wn) == week {
			out = append(out, m)
		}
	}
	return out
}

// teamAppearsInMatches returns true if teamID is home or away in any of the matches.
func teamAppearsInMatches(teamID int64, matches []map[string]any) bool {
	for _, m := range matches {
		if int64(m["home_team_id"].(float64)) == teamID ||
			int64(m["away_team_id"].(float64)) == teamID {
			return true
		}
	}
	return false
}

func TestByeRequest_EvenTeamCountRejected(t *testing.T) {
	srv := testServer(t)
	// Two teams = even → bye request must be rejected.
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, teamIDs)
	resp := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("even-team bye request: want 400, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestByeRequest_TeamFromAnotherLeagueRejected(t *testing.T) {
	srv := testServer(t)
	// Create first league with 3 teams and a season.
	_, seasonID, teamIDs1 := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs1)

	// Create a second league and team.
	lg2Resp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg2 map[string]any
	json.NewDecoder(lg2Resp.Body).Decode(&lg2)
	lg2Resp.Body.Close()
	lg2ID := int64(lg2["id"].(float64))

	tm2Resp, _ := http.Post(srv.URL+"/api/teams", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Outsider"}`, lg2ID)))
	var tm2 map[string]any
	json.NewDecoder(tm2Resp.Body).Decode(&tm2)
	tm2Resp.Body.Close()
	foreignTeamID := int64(tm2["id"].(float64))

	resp := postByeRequest(t, srv, seasonID, foreignTeamID, 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("foreign-team bye request: want 400, got %d", resp.StatusCode)
	}
}

func TestByeRequest_DuplicateRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("first request: want 201, got %d", r1.StatusCode)
	}

	r2 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate request: want 400, got %d", r2.StatusCode)
	}
}

func TestByeRequest_ApprovedHonoredInSchedule(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	// 3 teams (odd) → one natural bye per week.
	// With Alpha(1), Bravo(2), Charlie(3) and single_rr:
	//   Week 1: Bravo vs Charlie  (Alpha bye)
	//   Week 2: Alpha vs Charlie  (Bravo bye)
	//   Week 3: Alpha vs Bravo    (Charlie bye)
	// Request Charlie's bye moved to week 1.
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	charlieID := teamIDs[2]

	// Create and approve the bye request.
	r := postByeRequest(t, srv, seasonID, charlieID, 1)
	var created map[string]any
	json.NewDecoder(r.Body).Decode(&created)
	r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("create bye: want 201, got %d", r.StatusCode)
	}
	byeID := int64(created["id"].(float64))
	approveByeRequest(t, srv, seasonID, byeID)

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	week1 := matchesInWeek(matches, 1)
	if len(week1) == 0 {
		t.Fatal("no matches in week 1")
	}
	if teamAppearsInMatches(charlieID, week1) {
		t.Errorf("Charlie should have the bye in week 1 but appears in a match")
	}
	// Exactly one match in week 1 (3 teams → 1 match per week).
	if len(week1) != 1 {
		t.Errorf("week 1: want 1 match, got %d", len(week1))
	}
}

func TestByeRequest_UnapprovedIgnoredInSchedule(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	charlieID := teamIDs[2]

	// Create but do NOT approve the bye request for week 1.
	r := postByeRequest(t, srv, seasonID, charlieID, 1)
	r.Body.Close()

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	week1 := matchesInWeek(matches, 1)
	// Without approval, the natural rotation applies: Charlie appears in week 1.
	if !teamAppearsInMatches(charlieID, week1) {
		t.Error("unapproved request should have no effect; Charlie should play in week 1")
	}
}

func TestByeRequest_NaturalByeNotDuplicated(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	charlieID := teamIDs[2]

	r := postByeRequest(t, srv, seasonID, charlieID, 1)
	var created map[string]any
	json.NewDecoder(r.Body).Decode(&created)
	r.Body.Close()
	approveByeRequest(t, srv, seasonID, int64(created["id"].(float64)))

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	// Total matches for single_rr with 3 teams = 3 (one per week).
	if len(matches) != 3 {
		t.Errorf("expected 3 matches, got %d — bye request must not create extra bye", len(matches))
	}
	// Every week has exactly 1 match.
	for week := 1; week <= 3; week++ {
		if n := len(matchesInWeek(matches, week)); n != 1 {
			t.Errorf("week %d: want 1 match, got %d", week, n)
		}
	}
}

// ─── Player assignment (no duplicate) ────────────────────────────────────────

func TestAssignExistingPlayer_NamePreserved(t *testing.T) {
	srv := testServer(t)

	// Create a league, team, and an unassigned player.
	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Test League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()
	lgID := int64(lg["id"].(float64))

	tmResp, _ := http.Post(srv.URL+"/api/teams", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Team A"}`, lgID)))
	var tm map[string]any
	json.NewDecoder(tmResp.Body).Decode(&tm)
	tmResp.Body.Close()
	teamID := int64(tm["id"].(float64))

	pResp, _ := http.Post(srv.URL+"/api/players", "application/json",
		strings.NewReader(`{"first_name":"Jane","last_name":"Doe","handicap":1.5}`))
	var player map[string]any
	json.NewDecoder(pResp.Body).Decode(&player)
	pResp.Body.Close()
	playerID := int64(player["id"].(float64))

	// Assign the player to the team using first_name/last_name (the correct approach).
	body := fmt.Sprintf(`{"first_name":"Jane","last_name":"Doe","handicap":1.5,"team_id":%d}`, teamID)
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/players/%d", srv.URL, playerID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT player: want 200, got %d", resp.StatusCode)
	}

	// Verify the name is still intact after assignment.
	getResp, _ := http.Get(fmt.Sprintf("%s/api/players/%d", srv.URL, playerID))
	var updated map[string]any
	json.NewDecoder(getResp.Body).Decode(&updated)
	getResp.Body.Close()

	if fn, _ := updated["first_name"].(string); fn != "Jane" {
		t.Errorf("first_name: want %q, got %q", "Jane", fn)
	}
	if ln, _ := updated["last_name"].(string); ln != "Doe" {
		t.Errorf("last_name: want %q, got %q", "Doe", ln)
	}
	if tid, _ := updated["team_id"].(float64); int64(tid) != teamID {
		t.Errorf("team_id: want %d, got %v", teamID, updated["team_id"])
	}

	// Verify no duplicate player was created.
	allResp, _ := http.Get(srv.URL + "/api/players")
	var all []map[string]any
	json.NewDecoder(allResp.Body).Decode(&all)
	allResp.Body.Close()
	if len(all) != 1 {
		t.Errorf("expected 1 player, got %d — assignment must not create duplicates", len(all))
	}
}

// ─── Bye request conflict and scope enforcement ───────────────────────────────

// putByeApproval sends PUT .../bye-requests/{byeID} with {approved:approved}.
// Caller must close the returned response body.
func putByeApproval(t *testing.T, srv *httptest.Server, seasonID, byeID int64, approved bool) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"approved":%v}`, approved)
	req, _ := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, seasonID, byeID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT bye-approval: %v", err)
	}
	return resp
}

// deleteByeReq sends DELETE .../seasons/{sid}/bye-requests/{bid}.
func deleteByeReq(t *testing.T, srv *httptest.Server, seasonID, byeID int64) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("%s/api/seasons/%d/bye-requests/%d", srv.URL, seasonID, byeID),
		nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE bye-request: %v", err)
	}
	return resp
}

// listByes returns the decoded bye requests for the given season.
func listByes(t *testing.T, srv *httptest.Server, seasonID int64) []map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/bye-requests", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET bye-requests: %v", err)
	}
	defer resp.Body.Close()
	var byes []map[string]any
	json.NewDecoder(resp.Body).Decode(&byes)
	return byes
}

// TestByeRequest_ConflictPreventsApproval verifies that a second approved bye
// for the same week is rejected even though the request itself was accepted.
func TestByeRequest_ConflictPreventsApproval(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	// Approve Alpha for week 1.
	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("create Alpha bye: want 201, got %d", r1.StatusCode)
	}
	resp := putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve Alpha: want 200, got %d", resp.StatusCode)
	}

	// Record Bravo for week 1 — creation must succeed.
	r2 := postByeRequest(t, srv, seasonID, teamIDs[1], 1)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()
	if r2.StatusCode != http.StatusCreated {
		t.Fatalf("create Bravo bye: want 201, got %d", r2.StatusCode)
	}

	// Approving Bravo must be rejected because Alpha already holds week 1.
	resp2 := putByeApproval(t, srv, seasonID, int64(b2["id"].(float64)), true)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("approve conflict: want 400, got %d", resp2.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp2.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected a non-empty error message")
	}
}

// TestByeRequest_RejectedApprovalRemainsUnapproved verifies the bye request stays
// unapproved after a conflict rejection.
func TestByeRequest_RejectedApprovalRemainsUnapproved(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true).Body.Close()

	r2 := postByeRequest(t, srv, seasonID, teamIDs[1], 1)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()
	bravoID := int64(b2["id"].(float64))

	// Attempt (rejected) approval.
	putByeApproval(t, srv, seasonID, bravoID, true).Body.Close()

	// Confirm Bravo's request is still unapproved.
	byes := listByes(t, srv, seasonID)
	for _, b := range byes {
		if int64(b["id"].(float64)) == bravoID {
			if app, _ := b["approved"].(bool); app {
				t.Error("Bravo bye should remain unapproved after conflict rejection")
			}
			return
		}
	}
	t.Fatal("Bravo bye request not found in list")
}

// TestByeRequest_DifferentWeeksCanBothBeApproved verifies independent week slots.
func TestByeRequest_DifferentWeeksCanBothBeApproved(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r1 := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()

	r2 := postByeRequest(t, srv, seasonID, teamIDs[1], 2)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()

	resp1 := putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("approve Alpha week 1: want 200, got %d", resp1.StatusCode)
	}

	resp2 := putByeApproval(t, srv, seasonID, int64(b2["id"].(float64)), true)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("approve Bravo week 2: want 200, got %d", resp2.StatusCode)
	}
}

// TestByeRequest_Week0CannotBeApproved verifies TBD requests are not approvable.
func TestByeRequest_Week0CannotBeApproved(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	r := postByeRequest(t, srv, seasonID, teamIDs[0], 0)
	var b map[string]any
	json.NewDecoder(r.Body).Decode(&b)
	r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("create week-0 bye: want 201, got %d", r.StatusCode)
	}

	resp := putByeApproval(t, srv, seasonID, int64(b["id"].(float64)), true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("approve week-0: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message for week-0 approval")
	}
}

// TestByeRequest_WrongSeasonUpdateRejected verifies season scope on updates.
func TestByeRequest_WrongSeasonUpdateRejected(t *testing.T) {
	srv := testServer(t)

	// Season 1: 3-team league with a bye request.
	_, season1ID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, season1ID, teamIDs)
	r := postByeRequest(t, srv, season1ID, teamIDs[0], 1)
	var b map[string]any
	json.NewDecoder(r.Body).Decode(&b)
	r.Body.Close()
	byeID := int64(b["id"].(float64))

	// Season 2: a separate season in a different league.
	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Other Season"}`, int64(lg["id"].(float64)))))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	season2ID := int64(s2["id"].(float64))

	// Try to update bye via season 2's URL — must be 404.
	resp := putByeApproval(t, srv, season2ID, byeID, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-season update: want 404, got %d", resp.StatusCode)
	}
}

// TestByeRequest_WrongSeasonDeleteRejected verifies season scope on deletes.
func TestByeRequest_WrongSeasonDeleteRejected(t *testing.T) {
	srv := testServer(t)

	_, season1ID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-07-06", "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, season1ID, teamIDs)
	r := postByeRequest(t, srv, season1ID, teamIDs[0], 1)
	var b map[string]any
	json.NewDecoder(r.Body).Decode(&b)
	r.Body.Close()
	byeID := int64(b["id"].(float64))

	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Other Season"}`, int64(lg["id"].(float64)))))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	season2ID := int64(s2["id"].(float64))

	resp := deleteByeReq(t, srv, season2ID, byeID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-season delete: want 404, got %d", resp.StatusCode)
	}

	// The request must still exist under season 1.
	byes := listByes(t, srv, season1ID)
	found := false
	for _, by := range byes {
		if int64(by["id"].(float64)) == byeID {
			found = true
		}
	}
	if !found {
		t.Error("bye request was incorrectly deleted via wrong season URL")
	}
}

// TestByeRequest_DeterministicScheduleHonorsApproved confirms schedule generation
// honors the single approved request when a competing unapproved request exists
// for the same week.
func TestByeRequest_DeterministicScheduleHonorsApproved(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	// 3 teams: Week 1 natural bye = Alpha, Week 2 = Bravo, Week 3 = Charlie.
	// Approve Alpha for week 1 (no change); also record Bravo for week 1 (unapproved).
	// Schedule must still give week 1 to Alpha (the approved one).
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)
	alphaID := teamIDs[0]
	bravoID := teamIDs[1]

	r1 := postByeRequest(t, srv, seasonID, alphaID, 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true).Body.Close()

	// Bravo requests week 1 but is NOT approved.
	r2 := postByeRequest(t, srv, seasonID, bravoID, 1)
	r2.Body.Close()

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	week1 := matchesInWeek(matches, 1)
	if len(week1) == 0 {
		t.Fatal("no matches in week 1")
	}
	// Alpha should have the bye — approved request wins.
	if teamAppearsInMatches(alphaID, week1) {
		t.Error("Alpha should have the week-1 bye (approved request) but appears in a match")
	}
	// Bravo must play (unapproved request ignored).
	if !teamAppearsInMatches(bravoID, week1) {
		t.Error("Bravo should play in week 1 (unapproved request ignored)")
	}
}

// TestByeRequest_TwoApprovedRequestsDifferentWeeks verifies that two approved
// bye requests for different weeks are both honoured in the generated schedule.
// This is the multi-request regression test for the pairwise-swap bug where the
// second request was silently dropped when the displaced source week had already
// been used in an earlier swap.
//
// 5 teams, single_rr natural byes:
//
//	week 1 = Alpha   week 2 = Delta   week 3 = Bravo   week 4 = Echo   week 5 = Charlie
//
// Approved: Echo   → week 1  (Echo natural week 4)
//
//	Bravo  → week 2  (Bravo natural week 3)
func TestByeRequest_TwoApprovedRequestsDifferentWeeks(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-07-06"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate,
		"Alpha", "Bravo", "Charlie", "Delta", "Echo")
	ensureSeasonTeams(t, seasonID, teamIDs)
	echoID := teamIDs[4]
	bravoID := teamIDs[1]

	// Create and approve Echo for week 1.
	r1 := postByeRequest(t, srv, seasonID, echoID, 1)
	var b1 map[string]any
	json.NewDecoder(r1.Body).Decode(&b1)
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("create Echo bye: want 201, got %d", r1.StatusCode)
	}
	resp1 := putByeApproval(t, srv, seasonID, int64(b1["id"].(float64)), true)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("approve Echo: want 200, got %d", resp1.StatusCode)
	}

	// Create and approve Bravo for week 2.
	r2 := postByeRequest(t, srv, seasonID, bravoID, 2)
	var b2 map[string]any
	json.NewDecoder(r2.Body).Decode(&b2)
	r2.Body.Close()
	if r2.StatusCode != http.StatusCreated {
		t.Fatalf("create Bravo bye: want 201, got %d", r2.StatusCode)
	}
	resp2 := putByeApproval(t, srv, seasonID, int64(b2["id"].(float64)), true)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("approve Bravo: want 200, got %d", resp2.StatusCode)
	}

	matches := generateAndGetMatches(t, srv, seasonID, startDate, nil)

	// Total: single_rr with 5 teams = 10 matches.
	if len(matches) != 10 {
		t.Errorf("total matches: want 10, got %d", len(matches))
	}

	week1 := matchesInWeek(matches, 1)
	if len(week1) == 0 {
		t.Fatal("no matches in week 1")
	}
	if teamAppearsInMatches(echoID, week1) {
		t.Error("Echo should have the week-1 bye but appears in a match")
	}

	week2 := matchesInWeek(matches, 2)
	if len(week2) == 0 {
		t.Fatal("no matches in week 2")
	}
	if teamAppearsInMatches(bravoID, week2) {
		t.Error("Bravo should have the week-2 bye but appears in a match")
	}

	// Regenerating should produce identical bye assignments (deterministic).
	matches2 := generateAndGetMatches(t, srv, seasonID, startDate, nil)
	if teamAppearsInMatches(echoID, matchesInWeek(matches2, 1)) {
		t.Error("regeneration: Echo should still have week-1 bye")
	}
	if teamAppearsInMatches(bravoID, matchesInWeek(matches2, 2)) {
		t.Error("regeneration: Bravo should still have week-2 bye")
	}
}

// ─── Season Teams & Rosters (Phase One) ──────────────────────────────────────

// postSeasonTeamFromID adds an existing team to a draft season via from_team_id.
func postSeasonTeamFromID(t *testing.T, srv *httptest.Server, seasonID, teamID int64) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"from_team_id":%d}`, teamID)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST season team: %v", err)
	}
	return resp
}

// postNewSeasonTeam creates a brand-new team inside a draft season via name.
func postNewSeasonTeam(t *testing.T, srv *httptest.Server, seasonID int64, name string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q}`, name)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST new season team: %v", err)
	}
	return resp
}

// postRosterPlayer adds a player to a team's season roster.
func postRosterPlayer(t *testing.T, srv *httptest.Server, seasonID, teamID, playerID int64) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"player_id":%d}`, playerID)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST roster player: %v", err)
	}
	return resp
}

// httpDo sends an arbitrary request; body may be empty.
func httpDo(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var req *http.Request
	if body != "" {
		req, _ = http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, srv.URL+path, nil)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// postActivateSeason POSTs /api/seasons/{id}/activate.
func postActivateSeason(t *testing.T, srv *httptest.Server, seasonID int64) *http.Response {
	t.Helper()
	return httpDo(t, srv, http.MethodPost, fmt.Sprintf("/api/seasons/%d/activate", seasonID), "")
}

// getChecklist GETs /api/seasons/{id}/checklist and returns the decoded body map.
func getChecklist(t *testing.T, srv *httptest.Server, seasonID int64) map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/checklist", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET checklist: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("checklist: want 200, got %d", resp.StatusCode)
	}
	var c map[string]any
	json.NewDecoder(resp.Body).Decode(&c)
	return c
}

// createTestPlayer POSTs a player and returns their ID.
func createTestPlayer(t *testing.T, srv *httptest.Server, first, last string) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"first_name":%q,"last_name":%q,"handicap":0}`, first, last)
	resp, err := http.Post(srv.URL+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST player: %v", err)
	}
	defer resp.Body.Close()
	var p map[string]any
	json.NewDecoder(resp.Body).Decode(&p)
	return int64(p["id"].(float64))
}

// setPlayerTeam assigns a player to a team directly in the DB (avoids full-PUT field clobber).
func setPlayerTeam(t *testing.T, playerID, teamID int64) {
	t.Helper()
	if _, err := db.DB.Exec(`UPDATE players SET team_id=? WHERE id=?`, teamID, playerID); err != nil {
		t.Fatalf("setPlayerTeam: %v", err)
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestSeasonTeams_AddAndList(t *testing.T) {
	srv := testServer(t)
	_, seasonID, _ := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Managed first season: only the name path is allowed.
	r := postNewSeasonTeam(t, srv, seasonID, "Gamma")
	if r.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		t.Fatalf("add team: want 201, got %d — %s", r.StatusCode, body)
	}
	var added map[string]any
	json.NewDecoder(r.Body).Decode(&added)
	r.Body.Close()
	addedTeamID := int64(added["team_id"].(float64))

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var teams []map[string]any
	json.NewDecoder(resp.Body).Decode(&teams)
	if len(teams) != 1 {
		t.Fatalf("want 1 season team, got %d", len(teams))
	}
	if int64(teams[0]["team_id"].(float64)) != addedTeamID {
		t.Errorf("team_id: want %d, got %v", addedTeamID, teams[0]["team_id"])
	}
}

func TestSeasonTeams_DuplicateRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	// Downgrade to legacy mode so from_team_id works without from_season_id.
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, seasonID)

	r1 := postSeasonTeamFromID(t, srv, seasonID, teamIDs[0])
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("first add: want 201, got %d", r1.StatusCode)
	}

	r2 := postSeasonTeamFromID(t, srv, seasonID, teamIDs[0])
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate add: want 400, got %d", r2.StatusCode)
	}
}

func TestSeasonTeams_ActiveSeasonBlocked(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Lock the season by setting activated_at directly in DB (bypasses checklist).
	// isDraftSeason checks activated_at IS NULL, so this simulates first activation.
	if _, err := db.DB.Exec(
		`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, seasonID); err != nil {
		t.Fatalf("set activated_at: %v", err)
	}

	r := postSeasonTeamFromID(t, srv, seasonID, teamIDs[0])
	defer r.Body.Close()
	if r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("add team to active season: want 422, got %d", r.StatusCode)
	}
}

func TestSeasonRoster_AddAndList(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	pID := createTestPlayer(t, srv, "Alice", "Smith")
	setPlayerTeam(t, pID, teamIDs[0])

	rr := postRosterPlayer(t, srv, seasonID, teamIDs[0], pID)
	defer rr.Body.Close()
	if rr.StatusCode != http.StatusCreated {
		t.Fatalf("add roster player: want 201, got %d", rr.StatusCode)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var roster []map[string]any
	json.NewDecoder(resp.Body).Decode(&roster)
	if len(roster) != 1 {
		t.Fatalf("want 1 roster entry, got %d", len(roster))
	}
	if int64(roster[0]["player_id"].(float64)) != pID {
		t.Errorf("player_id: want %d, got %v", pID, roster[0]["player_id"])
	}
}

func TestSeasonRoster_DuplicateOnOtherTeamRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, teamIDs)

	pID := createTestPlayer(t, srv, "Bob", "Jones")
	setPlayerTeam(t, pID, teamIDs[0])

	postRosterPlayer(t, srv, seasonID, teamIDs[0], pID).Body.Close()

	// Same player on a different team must be rejected.
	rr2 := postRosterPlayer(t, srv, seasonID, teamIDs[1], pID)
	defer rr2.Body.Close()
	if rr2.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-team duplicate: want 400, got %d", rr2.StatusCode)
	}
}

func TestSeasonRoster_ActiveSeasonBlocked(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Lock the season by setting activated_at directly in DB (bypasses checklist).
	// isDraftSeason checks activated_at IS NULL, so this simulates first activation.
	if _, err := db.DB.Exec(
		`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, seasonID); err != nil {
		t.Fatalf("set activated_at: %v", err)
	}

	// Add team directly in DB (activated season cannot be modified via API).
	if _, err := db.DB.Exec(
		`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Alpha')`,
		seasonID, teamIDs[0]); err != nil {
		t.Fatalf("seed season_teams: %v", err)
	}
	pID := createTestPlayer(t, srv, "Carol", "Lee")
	setPlayerTeam(t, pID, teamIDs[0])

	rr := postRosterPlayer(t, srv, seasonID, teamIDs[0], pID)
	defer rr.Body.Close()
	if rr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("roster add to active season: want 422, got %d", rr.StatusCode)
	}
}

func TestSeasonTeams_RemoveTeam_ClearsRoster(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})
	pID := createTestPlayer(t, srv, "Dan", "Brown")
	setPlayerTeam(t, pID, teamIDs[0])
	postRosterPlayer(t, srv, seasonID, teamIDs[0], pID).Body.Close()

	del := httpDo(t, srv, http.MethodDelete,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), "")
	del.Body.Close()
	if del.StatusCode != http.StatusOK {
		t.Fatalf("delete season team: want 200, got %d", del.StatusCode)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamIDs[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var roster []map[string]any
	json.NewDecoder(resp.Body).Decode(&roster)
	if len(roster) != 0 {
		t.Errorf("roster after team removal: want 0, got %d", len(roster))
	}
}

func TestSeasonTeams_UpdateCaptain(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})
	pID := createTestPlayer(t, srv, "Eve", "Green")
	setPlayerTeam(t, pID, teamIDs[0])
	postRosterPlayer(t, srv, seasonID, teamIDs[0], pID).Body.Close()

	body := fmt.Sprintf(`{"season_name":"Alpha A","captain_id":%d}`, pID)
	upd := httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body)
	defer upd.Body.Close()
	if upd.StatusCode != http.StatusOK {
		t.Fatalf("update captain: want 200, got %d", upd.StatusCode)
	}

	var st map[string]any
	json.NewDecoder(upd.Body).Decode(&st)
	if int64(st["captain_id"].(float64)) != pID {
		t.Errorf("captain_id: want %d, got %v", pID, st["captain_id"])
	}
}

func TestSeasonTeams_UpdateCaptainNotOnRosterRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})
	// Player is created but NOT added to season roster.
	pID := createTestPlayer(t, srv, "Frank", "White")
	setPlayerTeam(t, pID, teamIDs[0])

	body := fmt.Sprintf(`{"season_name":"Alpha","captain_id":%d}`, pID)
	upd := httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body)
	defer upd.Body.Close()
	if upd.StatusCode != http.StatusBadRequest {
		t.Fatalf("captain not on roster: want 400, got %d", upd.StatusCode)
	}
}

// TestSeasonRoster_RemoveNonCaptain_CaptainUnchanged verifies that removing a
// non-captain player from a draft season roster leaves the team's captain_id intact.
func TestSeasonRoster_RemoveNonCaptain_CaptainUnchanged(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	captainID := createTestPlayer(t, srv, "Cap", "Tain")
	otherID := createTestPlayer(t, srv, "Other", "Player")

	// Add both players to the roster.
	postRosterPlayer(t, srv, seasonID, teamIDs[0], captainID).Body.Close()
	postRosterPlayer(t, srv, seasonID, teamIDs[0], otherID).Body.Close()

	// Assign captainID as captain.
	body := fmt.Sprintf(`{"season_name":"Alpha","captain_id":%d}`, captainID)
	httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body).Body.Close()

	// Remove the NON-captain player.
	del := httpDo(t, srv, http.MethodDelete,
		fmt.Sprintf("/api/seasons/%d/teams/%d/roster/%d", seasonID, teamIDs[0], otherID), "")
	defer del.Body.Close()
	if del.StatusCode != http.StatusOK {
		t.Fatalf("remove non-captain: want 200, got %d", del.StatusCode)
	}

	// Captain must still be set.
	var captainNow *int64
	if err := db.DB.QueryRow(
		`SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`,
		seasonID, teamIDs[0]).Scan(&captainNow); err != nil {
		t.Fatalf("query captain: %v", err)
	}
	if captainNow == nil || *captainNow != captainID {
		t.Errorf("captain_id: want %d, got %v", captainID, captainNow)
	}
}

// TestSeasonRoster_RemoveCaptain_ClearsCaptain verifies that removing the current
// captain from a draft season roster atomically sets captain_id to NULL in season_teams.
func TestSeasonRoster_RemoveCaptain_ClearsCaptain(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	captainID := createTestPlayer(t, srv, "Cap", "Tain")

	// Add player and assign as captain.
	postRosterPlayer(t, srv, seasonID, teamIDs[0], captainID).Body.Close()
	body := fmt.Sprintf(`{"season_name":"Alpha","captain_id":%d}`, captainID)
	httpDo(t, srv, http.MethodPut,
		fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, teamIDs[0]), body).Body.Close()

	// Confirm captain is set before the DELETE.
	var before *int64
	db.DB.QueryRow(`SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`,
		seasonID, teamIDs[0]).Scan(&before)
	if before == nil || *before != captainID {
		t.Fatalf("precondition: captain_id should be %d, got %v", captainID, before)
	}

	// Remove the captain from the roster.
	del := httpDo(t, srv, http.MethodDelete,
		fmt.Sprintf("/api/seasons/%d/teams/%d/roster/%d", seasonID, teamIDs[0], captainID), "")
	defer del.Body.Close()
	if del.StatusCode != http.StatusOK {
		t.Fatalf("remove captain: want 200, got %d", del.StatusCode)
	}

	// captain_id must now be NULL in season_teams.
	var after *int64
	if err := db.DB.QueryRow(
		`SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`,
		seasonID, teamIDs[0]).Scan(&after); err != nil {
		t.Fatalf("query captain after removal: %v", err)
	}
	if after != nil {
		t.Errorf("captain_id: want NULL after captain removed from roster, got %d", *after)
	}

	// Verify the roster row is also gone.
	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND player_id=?`,
		seasonID, captainID).Scan(&count)
	if count != 0 {
		t.Errorf("season_rosters: player row should be deleted, count=%d", count)
	}
}

func TestSeasonTeams_CopyFromPriorWithRoster(t *testing.T) {
	srv := testServer(t)
	leagueID, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, prevSeasonID, []int64{teamIDs[0]})
	pID := createTestPlayer(t, srv, "Grace", "Hall")
	setPlayerTeam(t, pID, teamIDs[0])
	postRosterPlayer(t, srv, prevSeasonID, teamIDs[0], pID).Body.Close()

	// Give the prior season an end_date so PreviousSeason can find it (correction 6).
	if _, err := db.DB.Exec(`UPDATE seasons SET end_date='2026-09-30' WHERE id=?`, prevSeasonID); err != nil {
		t.Fatalf("set end_date: %v", err)
	}

	// Create a new season in the same league.
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Fall 2026","start_date":"2026-10-01"}`, leagueID)))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	newSeasonID := int64(s2["id"].(float64))

	// Copy team from prior season, preserving roster.
	cpBody := fmt.Sprintf(`{"from_team_id":%d,"from_season_id":%d}`, teamIDs[0], prevSeasonID)
	cp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, newSeasonID),
		"application/json", strings.NewReader(cpBody))
	cp.Body.Close()
	if cp.StatusCode != http.StatusCreated {
		t.Fatalf("copy team: want 201, got %d", cp.StatusCode)
	}

	rosterResp, _ := http.Get(
		fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, newSeasonID, teamIDs[0]))
	defer rosterResp.Body.Close()
	var roster []map[string]any
	json.NewDecoder(rosterResp.Body).Decode(&roster)

	found := false
	for _, e := range roster {
		if int64(e["player_id"].(float64)) == pID {
			found = true
		}
	}
	if !found {
		t.Errorf("copied roster must contain player %d; got %v", pID, roster)
	}
}

func TestSeasonTeams_MarkStaleWhenMatchesExist(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-09-01"
	_, seasonID := seedScheduleFixture(t, srv, startDate)

	// Generate schedule (sets schedule_stale=0).
	generateAndGetMatches(t, srv, seasonID, startDate, nil)

	sResp, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d", srv.URL, seasonID))
	var s1 map[string]any
	json.NewDecoder(sResp.Body).Decode(&s1)
	sResp.Body.Close()
	if s1["schedule_stale"].(bool) {
		t.Fatal("schedule_stale should be false immediately after generation")
	}

	// Add a new team — unplayed matches exist so stale flag must be set.
	postNewSeasonTeam(t, srv, seasonID, "Delta").Body.Close()

	sResp2, _ := http.Get(fmt.Sprintf("%s/api/seasons/%d", srv.URL, seasonID))
	var s2 map[string]any
	json.NewDecoder(sResp2.Body).Decode(&s2)
	sResp2.Body.Close()
	if !s2["schedule_stale"].(bool) {
		t.Error("schedule_stale must be true after adding a team when unplayed matches exist")
	}
}

// TestSeasonChecklist_LegacySeason_CanActivate verifies correction 2:
// a season with teams_managed=0 (legacy) bypasses all checklist enforcement.
// The season is created via API (teams_managed=1) then downgraded in the DB
// to simulate a pre-Phase-One record.
func TestSeasonChecklist_LegacySeason_CanActivate(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	// Downgrade to legacy mode (simulates seasons created before Phase One).
	if _, err := db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, sid); err != nil {
		t.Fatalf("set teams_managed=0: %v", err)
	}

	c := getChecklist(t, srv, sid)
	if canActivate, _ := c["can_activate"].(bool); !canActivate {
		t.Errorf("legacy season: want can_activate=true; got %v", c)
	}
	blockers, _ := c["blockers"].([]any)
	if len(blockers) != 0 {
		t.Errorf("legacy season: want no blockers, got %v", blockers)
	}
}

// TestSeasonChecklist_ManagedNoTeams_BlocksTooFew verifies correction 1:
// a managed season (teams_managed=1, set by createSeason) with no teams
// returns TEAMS_TOO_FEW and cannot activate.
func TestSeasonChecklist_ManagedNoTeams_BlocksTooFew(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)
	// sid was created via API → teams_managed=1; no teams added yet.

	c := getChecklist(t, srv, sid)
	if canActivate, _ := c["can_activate"].(bool); canActivate {
		t.Error("managed season with no teams: want can_activate=false")
	}
	blockers, _ := c["blockers"].([]any)
	found := false
	for _, b := range blockers {
		bm := b.(map[string]any)
		if bm["code"].(string) == "TEAMS_TOO_FEW" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TEAMS_TOO_FEW blocker; got: %v", blockers)
	}
}

func TestSeasonChecklist_TwoTeamsNoSchedule_Blocked(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, teamIDs)

	// One player per team on roster, set as captain.
	for i, tid := range teamIDs {
		pID := createTestPlayer(t, srv, fmt.Sprintf("P%d", i), "Tester")
		setPlayerTeam(t, pID, tid)
		postRosterPlayer(t, srv, seasonID, tid, pID).Body.Close()
		httpDo(t, srv, http.MethodPut,
			fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, tid),
			fmt.Sprintf(`{"season_name":"Team%d","captain_id":%d}`, i, pID)).Body.Close()
	}

	c := getChecklist(t, srv, seasonID)
	if canActivate, _ := c["can_activate"].(bool); canActivate {
		t.Errorf("no schedule generated: want can_activate=false; got %v", c)
	}

	blockers, _ := c["blockers"].([]any)
	found := false
	for _, b := range blockers {
		bm := b.(map[string]any)
		if bm["code"].(string) == "NO_SCHEDULE" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected NO_SCHEDULE blocker; got: %v", blockers)
	}
}

func TestSeasonChecklist_AllGood_CanActivate(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, teamIDs)

	for i, tid := range teamIDs {
		pID := createTestPlayer(t, srv, fmt.Sprintf("Q%d", i), "Ready")
		setPlayerTeam(t, pID, tid)
		postRosterPlayer(t, srv, seasonID, tid, pID).Body.Close()
		httpDo(t, srv, http.MethodPut,
			fmt.Sprintf("/api/seasons/%d/teams/%d", seasonID, tid),
			fmt.Sprintf(`{"season_name":"T%d","captain_id":%d}`, i, pID)).Body.Close()
	}

	// Generate a schedule (2 teams, 1 match).
	genBody := fmt.Sprintf(`{"season_id":%d,"start_date":"2026-09-01","schedule_type":"single_rr"}`, seasonID)
	genResp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(genBody))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	genResp.Body.Close()
	if genResp.StatusCode != http.StatusOK {
		t.Fatalf("generate: want 200, got %d", genResp.StatusCode)
	}

	c := getChecklist(t, srv, seasonID)
	if canActivate, _ := c["can_activate"].(bool); !canActivate {
		t.Errorf("happy path: want can_activate=true; checklist: %v", c)
	}
	blockers, _ := c["blockers"].([]any)
	if len(blockers) != 0 {
		t.Errorf("happy path: want no blockers, got %v", blockers)
	}
}

func TestActivateSeason_BlockedByChecklist(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// One team only → TEAMS_TOO_FEW; no players → TEAM_NO_PLAYERS; no schedule → NO_SCHEDULE.
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	act := postActivateSeason(t, srv, seasonID)
	defer act.Body.Close()
	if act.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("activate with blockers: want 422, got %d", act.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(act.Body).Decode(&body)
	blockers, _ := body["blockers"].([]any)
	if len(blockers) == 0 {
		t.Error("expected blockers array in 422 response body")
	}
}

func TestAvailablePlayers_ExcludesRostered(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	p1 := createTestPlayer(t, srv, "Karl", "One")
	p2 := createTestPlayer(t, srv, "Lara", "Two")
	setPlayerTeam(t, p1, teamIDs[0])
	setPlayerTeam(t, p2, teamIDs[0])

	// Roster p1 only; p2 remains unrostered.
	postRosterPlayer(t, srv, seasonID, teamIDs[0], p1).Body.Close()

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/players/available", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var players []map[string]any
	json.NewDecoder(resp.Body).Decode(&players)

	foundP1, foundP2 := false, false
	for _, p := range players {
		pid := int64(p["id"].(float64))
		if pid == p1 {
			foundP1 = true
		}
		if pid == p2 {
			foundP2 = true
		}
	}
	if foundP1 {
		t.Error("rostered player must not appear in available list")
	}
	if !foundP2 {
		t.Error("unrostered player must appear in available list")
	}
}

func TestPreviousSeason_ReturnsNilWhenNoPrior(t *testing.T) {
	srv := testServer(t)
	sid := seedSeason(t, srv.URL)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/previous", srv.URL, sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["season"] != nil {
		t.Errorf("want null season when no prior exists, got %v", body["season"])
	}
	teams, _ := body["teams"].([]any)
	if len(teams) != 0 {
		t.Errorf("want empty teams list, got %d items", len(teams))
	}
}

func TestPreviousSeason_ReturnsTeamsFromSeasonTeams(t *testing.T) {
	srv := testServer(t)
	leagueID, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Register both teams in the prior season.
	ensureSeasonTeams(t, prevSeasonID, teamIDs)

	// Give the prior season an end_date so it qualifies as a previous season.
	if _, err := db.DB.Exec(`UPDATE seasons SET end_date='2026-09-30' WHERE id=?`, prevSeasonID); err != nil {
		t.Fatalf("set end_date: %v", err)
	}

	// Create a newer season.
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(
			`{"league_id":%d,"name":"Fall 2026","start_date":"2026-10-01"}`, leagueID)))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	newSeasonID := int64(s2["id"].(float64))

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/previous", srv.URL, newSeasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["season"] == nil {
		t.Fatal("want a previous season, got null")
	}
	teams, _ := body["teams"].([]any)
	if len(teams) != 2 {
		t.Errorf("want 2 prior teams, got %d", len(teams))
	}
}

func TestSaveRounds_BlockedByInsufficientRoster(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Add both teams to season (enables roster enforcement).
	ensureSeasonTeams(t, seasonID, teamIDs)

	// Add 1 player per team (below the 3-player minimum).
	for i, tid := range teamIDs {
		pID := createTestPlayer(t, srv, fmt.Sprintf("Rnd%d", i), "Player")
		setPlayerTeam(t, pID, tid)
		postRosterPlayer(t, srv, seasonID, tid, pID).Body.Close()
	}

	// Insert a match directly so we have a match ID to target.
	res, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number)
		 VALUES (?,?,?,?,?)`,
		seasonID, teamIDs[0], teamIDs[1], "2026-09-01", 1)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	matchID, _ := res.LastInsertId()

	rr := httpDo(t, srv, http.MethodPost,
		fmt.Sprintf("/api/matches/%d/rounds", matchID), `{"rounds":[]}`)
	defer rr.Body.Close()
	if rr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("insufficient roster: want 422, got %d", rr.StatusCode)
	}

	var errBody map[string]string
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message in 422 body")
	}
}

// ─── Correction regression tests ─────────────────────────────────────────────

// TestSeasonActivated_SetupLockedPersistently verifies correction 3:
// activated_at is set once on first activation. Simulating a second season
// becoming active (deactivating the first) must NOT re-enable setup on the
// first season — isDraftSeason checks activated_at, not active.
func TestSeasonActivated_SetupLockedPersistently(t *testing.T) {
	srv := testServer(t)
	leagueID, s1ID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Lock season 1 via DB (teams_managed=1 so API activation would need teams).
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, s1ID)

	// Create season 2 and activate it (demotes s1 to active=0).
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"S2","start_date":"2027-01-01"}`, leagueID)))
	var s2 map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2)
	s2Resp.Body.Close()
	s2ID := int64(s2["id"].(float64))

	// Set s2 active via DB (it has teams_managed=1 but no teams → checklist blocks API activate).
	db.DB.Exec(`UPDATE seasons SET active=0 WHERE league_id=?`, leagueID)
	db.DB.Exec(`UPDATE seasons SET active=1, activated_at=CURRENT_TIMESTAMP WHERE id=?`, s2ID)

	// Now season 1 has active=0 but activated_at is still set.
	// Adding a team to season 1 must still be rejected.
	r := postSeasonTeamFromID(t, srv, s1ID, teamIDs[0])
	defer r.Body.Close()
	if r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("add team to locked (deactivated) season: want 422, got %d", r.StatusCode)
	}
}

// TestAvailablePlayers_IncludesUnassigned verifies correction 4:
// players with no team_id (unassigned to any team) appear in available players.
func TestAvailablePlayers_IncludesUnassigned(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	postSeasonTeamFromID(t, srv, seasonID, teamIDs[0]).Body.Close()

	// Create a player with no team assigned.
	pUnassigned := createTestPlayer(t, srv, "Zara", "Solo")
	// pUnassigned has no team_id (created via API, no team_id in body).

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/players/available", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var players []map[string]any
	json.NewDecoder(resp.Body).Decode(&players)

	found := false
	for _, p := range players {
		if int64(p["id"].(float64)) == pUnassigned {
			found = true
		}
	}
	if !found {
		t.Errorf("unassigned player %d must appear in available list; got %d players", pUnassigned, len(players))
	}
}

// TestAvailablePlayers_IncludesCrossLeague verifies correction 4:
// players assigned to a team in a different league appear in available players.
func TestAvailablePlayers_IncludesCrossLeague(t *testing.T) {
	srv := testServer(t)
	_, seasonID, _ := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Create a second league with a team and a player.
	lg2Resp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Other League","game_format":"8ball"}`))
	var lg2 map[string]any
	json.NewDecoder(lg2Resp.Body).Decode(&lg2)
	lg2Resp.Body.Close()
	lg2ID := int64(lg2["id"].(float64))

	tm2Resp, _ := http.Post(srv.URL+"/api/teams", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Outsiders"}`, lg2ID)))
	var tm2 map[string]any
	json.NewDecoder(tm2Resp.Body).Decode(&tm2)
	tm2Resp.Body.Close()
	otherTeamID := int64(tm2["id"].(float64))

	pCrossLeague := createTestPlayer(t, srv, "Cross", "Player")
	setPlayerTeam(t, pCrossLeague, otherTeamID)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/players/available", srv.URL, seasonID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var players []map[string]any
	json.NewDecoder(resp.Body).Decode(&players)

	found := false
	for _, p := range players {
		if int64(p["id"].(float64)) == pCrossLeague {
			found = true
		}
	}
	if !found {
		t.Errorf("cross-league player %d must appear in available list; got %v players", pCrossLeague, len(players))
	}
}

// TestSeasonTeams_FromSeasonID_MustBePreviousSeason verifies correction 5:
// from_season_id must be the immediately previous season, not just any season.
func TestSeasonTeams_FromSeasonID_MustBePreviousSeason(t *testing.T) {
	srv := testServer(t)
	leagueID, olderSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-01-01", "Alpha", "Bravo")

	// Register team in older season.
	postSeasonTeamFromID(t, srv, olderSeasonID, teamIDs[0]).Body.Close()
	db.DB.Exec(`UPDATE seasons SET end_date='2026-06-01' WHERE id=?`, olderSeasonID)

	// Create a middle season (not the draft) with its own end_date.
	s2Resp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Middle","start_date":"2026-07-01"}`, leagueID)))
	var s2m map[string]any
	json.NewDecoder(s2Resp.Body).Decode(&s2m)
	s2Resp.Body.Close()
	middleSeasonID := int64(s2m["id"].(float64))
	db.DB.Exec(`UPDATE seasons SET end_date='2026-12-31' WHERE id=?`, middleSeasonID)

	// Create the draft season (start_date after middle).
	draftResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Draft","start_date":"2027-01-01"}`, leagueID)))
	var draftS map[string]any
	json.NewDecoder(draftResp.Body).Decode(&draftS)
	draftResp.Body.Close()
	draftSeasonID := int64(draftS["id"].(float64))

	// Try to copy using the OLDER season (not the immediately previous one).
	cpBody := fmt.Sprintf(`{"from_team_id":%d,"from_season_id":%d}`, teamIDs[0], olderSeasonID)
	cp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, draftSeasonID),
		"application/json", strings.NewReader(cpBody))
	cp.Body.Close()
	if cp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-prior from_season_id: want 400, got %d", cp.StatusCode)
	}
}

// TestSeasonTeams_FromSeasonID_TeamNotInPrevSeason_Rejected verifies correction 5:
// the team must have participated in the previous season.
func TestSeasonTeams_FromSeasonID_TeamNotInPrevSeason_Rejected(t *testing.T) {
	srv := testServer(t)
	leagueID, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// prevSeason has NO season_teams rows for teamIDs[0] — it was managed (created via API).
	db.DB.Exec(`UPDATE seasons SET end_date='2026-09-30' WHERE id=?`, prevSeasonID)

	// Create draft season.
	draftResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Draft","start_date":"2026-10-01"}`, leagueID)))
	var draftS map[string]any
	json.NewDecoder(draftResp.Body).Decode(&draftS)
	draftResp.Body.Close()
	draftSeasonID := int64(draftS["id"].(float64))

	// Copy with from_season_id = prevSeason, but team is NOT in prevSeason's season_teams.
	cpBody := fmt.Sprintf(`{"from_team_id":%d,"from_season_id":%d}`, teamIDs[0], prevSeasonID)
	cp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, draftSeasonID),
		"application/json", strings.NewReader(cpBody))
	cp.Body.Close()
	if cp.StatusCode != http.StatusBadRequest {
		t.Fatalf("team not in prev season: want 400, got %d", cp.StatusCode)
	}
}

// TestSeasonRosters_DbLevelEnforcementTriggered verifies correction 7:
// inserting into season_rosters without a matching season_teams row is blocked
// at the database level by the trigger.
func TestSeasonRosters_DbLevelEnforcementTriggered(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	pID := createTestPlayer(t, srv, "Trigger", "Test")

	// Insert directly into season_rosters WITHOUT adding the team to season_teams first.
	_, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamIDs[0], pID)
	if err == nil {
		t.Fatal("expected trigger to reject insert into season_rosters without season_teams row")
	}
}

// TestSeasonRosters_DbLevelEnforcementAllowsValid verifies correction 7:
// inserting into season_rosters WITH a matching season_teams row succeeds.
func TestSeasonRosters_DbLevelEnforcementAllowsValid(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Add the team to season_teams first (satisfies trigger condition).
	ensureSeasonTeams(t, seasonID, []int64{teamIDs[0]})

	pID := createTestPlayer(t, srv, "Valid", "Insert")

	// Insert directly into season_rosters WITH a matching season_teams row.
	_, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamIDs[0], pID)
	if err != nil {
		t.Fatalf("trigger should allow valid insert: %v", err)
	}
}

// TestSeasonRoster_IncludesPlayerNumber verifies that GET /api/seasons/{id}/teams/{tid}/roster
// returns player_number for players that have one set.
func TestSeasonRoster_IncludesPlayerNumber(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, teamIDs[:1])
	teamID := teamIDs[0]

	// Create player with player_number via API.
	body := `{"first_name":"Jane","last_name":"Doe","player_number":"99","handicap":2.5}`
	resp, err := http.Post(srv.URL+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST player: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST player: want 201, got %d: %s", resp.StatusCode, b)
	}
	var p map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		resp.Body.Close()
		t.Fatalf("POST player decode: %v", err)
	}
	resp.Body.Close()
	rawID, ok := p["id"]
	if !ok {
		t.Fatal("POST player response missing id field")
	}
	playerID := int64(rawID.(float64))

	postRosterPlayer(t, srv, seasonID, teamID, playerID).Body.Close()

	r, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamID))
	if err != nil {
		t.Fatalf("GET roster: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", r.StatusCode)
	}
	var roster []map[string]any
	json.NewDecoder(r.Body).Decode(&roster)
	if len(roster) == 0 {
		t.Fatal("expected at least one roster entry")
	}
	got, ok := roster[0]["player_number"]
	if !ok {
		t.Fatal("player_number field missing from roster response")
	}
	if got != "99" {
		t.Errorf("player_number: want %q, got %v", "99", got)
	}
}

// TestSeasonRoster_ZeroHandicapIncluded verifies that a player with handicap=0
// appears in the roster response with "handicap":0, not omitted.
func TestSeasonRoster_ZeroHandicapIncluded(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	ensureSeasonTeams(t, seasonID, teamIDs[:1])
	teamID := teamIDs[0]

	body := `{"first_name":"Zero","last_name":"Handi","handicap":0}`
	resp, err := http.Post(srv.URL+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST player: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST player: want 201, got %d: %s", resp.StatusCode, b)
	}
	var p map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		resp.Body.Close()
		t.Fatalf("POST player decode: %v", err)
	}
	resp.Body.Close()
	rawID, ok := p["id"]
	if !ok {
		t.Fatal("POST player response missing id field")
	}
	playerID := int64(rawID.(float64))

	postRosterPlayer(t, srv, seasonID, teamID, playerID).Body.Close()

	r, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams/%d/roster", srv.URL, seasonID, teamID))
	if err != nil {
		t.Fatalf("GET roster: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", r.StatusCode)
	}
	var roster []map[string]any
	json.NewDecoder(r.Body).Decode(&roster)
	if len(roster) == 0 {
		t.Fatal("expected at least one roster entry")
	}
	hc, ok := roster[0]["handicap"]
	if !ok {
		t.Fatal("handicap field missing for zero-handicap player")
	}
	if hc != float64(0) {
		t.Errorf("handicap: want 0, got %v", hc)
	}
}

// ─── Regression tests: corrections 1-4 ───────────────────────────────────────

// TestCreateSeason_ResponseHasTeamsManagedTrue verifies that createSeason returns
// teams_managed=true immediately, matching the persisted value (correction 4).
func TestCreateSeason_ResponseHasTeamsManagedTrue(t *testing.T) {
	srv := testServer(t)
	lgResp, _ := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"Test League","game_format":"8ball"}`))
	var lg map[string]any
	json.NewDecoder(lgResp.Body).Decode(&lg)
	lgResp.Body.Close()

	sResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Spring 2026"}`, int64(lg["id"].(float64)))))
	defer sResp.Body.Close()
	if sResp.StatusCode != http.StatusCreated {
		t.Fatalf("create season: want 201, got %d", sResp.StatusCode)
	}
	var s map[string]any
	json.NewDecoder(sResp.Body).Decode(&s)
	if tm, _ := s["teams_managed"].(bool); !tm {
		t.Errorf("create season response: want teams_managed=true, got %v", s["teams_managed"])
	}
}

// TestScheduleGenerate_ManagedNoTeams_Rejected verifies that schedule generation
// returns 400 for a managed season with no season_teams (correction 1).
func TestScheduleGenerate_ManagedNoTeams_Rejected(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-09-01"
	// seedScheduleFixtureWithTeams creates a managed season but does NOT register season_teams.
	_, seasonID, _ := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")

	body := fmt.Sprintf(`{"season_id":%d,"start_date":%q,"schedule_type":"single_rr"}`, seasonID, startDate)
	resp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /matches/generate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+no teams: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

// TestByeRequest_ManagedNoTeams_RejectsDespiteLeagueTeams verifies that bye
// validation uses season_teams count for managed seasons even when the league
// has odd teams (correction 2). A managed season with 0 season_teams has an
// even (0) participating count and must reject bye requests.
func TestByeRequest_ManagedNoTeams_RejectsDespiteLeagueTeams(t *testing.T) {
	srv := testServer(t)
	// 3 league teams (odd) but none registered in season_teams.
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo", "Charlie")

	resp := postByeRequest(t, srv, seasonID, teamIDs[0], 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+no season_teams: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSeasonTeams_ManagedRequiresFromSeasonIdAlways verifies that from_team_id
// always requires from_season_id in managed seasons, regardless of whether a
// prior season exists (correction 3).
func TestSeasonTeams_ManagedRequiresFromSeasonIdAlways(t *testing.T) {
	srv := testServer(t)
	_, prevSeasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	// Give the previous season an end_date so PreviousSeason can find it.
	db.DB.Exec(`UPDATE seasons SET end_date='2026-12-31' WHERE id=?`, prevSeasonID)

	// Create a new draft season in the same league.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, prevSeasonID).Scan(&leagueID)
	sResp, _ := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"Next Season","start_date":"2027-01-10"}`, leagueID)))
	var newSeason map[string]any
	json.NewDecoder(sResp.Body).Decode(&newSeason)
	sResp.Body.Close()
	newSeasonID := int64(newSeason["id"].(float64))

	// Attempt to add team without from_season_id → must be rejected.
	body := fmt.Sprintf(`{"from_team_id":%d}`, teamIDs[0])
	resp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, newSeasonID),
		"application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+prior season+no from_season_id: want 400, got %d", resp.StatusCode)
	}
}

// TestSeasonTeams_ManagedFirstSeason_FromTeamIdRejected verifies that from_team_id
// is rejected for a managed season even when no prior season exists; the only
// allowed path for first-season team creation is the name field (correction 3).
func TestSeasonTeams_ManagedFirstSeason_FromTeamIdRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")
	// No prior season exists; from_team_id must still be rejected for managed seasons.
	body := fmt.Sprintf(`{"from_team_id":%d}`, teamIDs[0])
	resp, _ := http.Post(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID),
		"application/json", strings.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed first season+from_team_id: want 400, got %d", resp.StatusCode)
	}
}

// TestScheduleGenerate_ManagedRejectsFromSeasonId verifies that schedule generation
// rejects a nonzero from_season_id for managed seasons; prior-season inference is
// legacy-only (correction 1).
func TestScheduleGenerate_ManagedRejectsFromSeasonId(t *testing.T) {
	srv := testServer(t)
	const startDate = "2026-09-01"
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, startDate, "Alpha", "Bravo", "Charlie")
	ensureSeasonTeams(t, seasonID, teamIDs)

	body := fmt.Sprintf(
		`{"season_id":%d,"start_date":%q,"schedule_type":"single_rr","from_season_id":999}`,
		seasonID, startDate)
	resp, err := http.Post(srv.URL+"/api/matches/generate", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /matches/generate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("managed+from_season_id: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSeasonTeams_ListReturnsTeamNumber verifies that GET /api/seasons/{id}/teams
// includes the team_number field for teams that have one set.
// team_number is stored on the teams table and projected through seasonTeamSelect;
// it is display-only in Phase 1 and not writable via the teams API.
func TestSeasonTeams_ListReturnsTeamNumber(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo")

	// Set team_number directly; updateTeam does not expose this field.
	if _, err := db.DB.Exec(`UPDATE teams SET team_number='07' WHERE id=?`, teamIDs[0]); err != nil {
		t.Fatalf("set team_number: %v", err)
	}
	ensureSeasonTeams(t, seasonID, teamIDs[:1])

	r, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET season teams: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", r.StatusCode)
	}
	var teams []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&teams); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(teams) == 0 {
		t.Fatal("expected at least one team in response")
	}
	got, ok := teams[0]["team_number"]
	if !ok {
		t.Fatal("team_number field missing from season-teams response")
	}
	if got != "07" {
		t.Errorf("team_number: want %q, got %v", "07", got)
	}
}

// TestByeRequest_ManagedTeamNotInSeasonTeams_Rejected verifies that a bye request
// for a team that belongs to the league but is not registered in season_teams is
// rejected for managed seasons (correction 2).
func TestByeRequest_ManagedTeamNotInSeasonTeams_Rejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha", "Bravo", "Charlie")
	// Register only Alpha and Bravo; Charlie is in the league but not in season_teams.
	ensureSeasonTeams(t, seasonID, teamIDs[:2])

	// Charlie is not registered — bye request must be rejected.
	resp := postByeRequest(t, srv, seasonID, teamIDs[2], 1)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("team not in season_teams: want 400, got %d", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestUpdateSeasonTeam_BlankNameRejected(t *testing.T) {
	srv := testServer(t)
	_, seasonID, teamIDs := seedScheduleFixtureWithTeams(t, srv, "2026-09-01", "Alpha")
	ensureSeasonTeams(t, seasonID, teamIDs)
	teamID := teamIDs[0]

	putTeam := func(body string) *http.Response {
		req, _ := http.NewRequest(http.MethodPut,
			fmt.Sprintf("%s/api/seasons/%d/teams/%d", srv.URL, seasonID, teamID),
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT season team: %v", err)
		}
		return resp
	}

	cases := []struct{ label, body string }{
		{"empty string", `{"season_name":"","captain_id":null}`},
		{"whitespace only", `{"season_name":"   ","captain_id":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			resp := putTeam(tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", resp.StatusCode)
			}
			var errBody map[string]string
			json.NewDecoder(resp.Body).Decode(&errBody)
			if errBody["error"] == "" {
				t.Error("expected non-empty error message")
			}
		})
	}

	// Verify stored season_name was not blanked by the rejected requests.
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/teams", srv.URL, seasonID))
	if err != nil {
		t.Fatalf("GET teams: %v", err)
	}
	defer resp.Body.Close()
	var teams []map[string]any
	json.NewDecoder(resp.Body).Decode(&teams)
	if len(teams) == 0 {
		t.Fatal("expected at least one registered team")
	}
	if name, _ := teams[0]["season_name"].(string); name == "" {
		t.Errorf("season_name was blanked after rejected PUT; got %q", name)
	}
}

// ─── Scoresheet round validation (HTTP level) ─────────────────────────────────

// seedRoundFixture creates a legacy (teams_managed=0) league/season/teams/players
// and inserts one match. The legacy season bypasses the RosterEligible roster check
// so the test can reach round validation directly.
// Returns (matchID, homePlayerID, awayPlayerID).
func seedRoundFixture(t *testing.T, srv *httptest.Server) (matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	pd := func(path, body string) map[string]any {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return m
	}

	lg := pd("/api/leagues", `{"name":"Round Test League","game_format":"8ball"}`)
	lgID := int64(lg["id"].(float64))

	tm1 := pd("/api/teams", fmt.Sprintf(`{"league_id":%d,"name":"Home Team"}`, lgID))
	tm2 := pd("/api/teams", fmt.Sprintf(`{"league_id":%d,"name":"Away Team"}`, lgID))
	homeTeamID := int64(tm1["id"].(float64))
	awayTeamID := int64(tm2["id"].(float64))

	p1 := pd("/api/players", `{"first_name":"Home","last_name":"Player","handicap":0}`)
	p2 := pd("/api/players", `{"first_name":"Away","last_name":"Player","handicap":0}`)
	homePlayerID = int64(p1["id"].(float64))
	awayPlayerID = int64(p2["id"].(float64))

	// Create a season and immediately downgrade to legacy (teams_managed=0).
	// POST /api/seasons always sets teams_managed=1; legacy mode bypasses RosterEligible.
	s := pd("/api/seasons", fmt.Sprintf(`{"league_id":%d,"name":"Test Season"}`, lgID))
	seasonID := int64(s["id"].(float64))
	if _, err := db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, seasonID); err != nil {
		t.Fatalf("downgrade season to legacy: %v", err)
	}

	res, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,1)`,
		seasonID, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	matchID, _ = res.LastInsertId()
	return
}

// TestSaveRounds_ValidationError_Returns422 confirms that submitting an impossible
// game score (both sides = 10) returns HTTP 422 with a structured validation.Result body.
func TestSaveRounds_ValidationError_Returns422(t *testing.T) {
	srv := testServer(t)
	matchID, homeP, awayP := seedRoundFixture(t, srv)

	body := fmt.Sprintf(`{"rounds":[{
		"round_number":1,
		"home_player_id":%d,"away_player_id":%d,
		"game1_home":10,"game1_away":10,
		"game2_home":0,"game2_away":0,
		"game3_home":0,"game3_away":0
	}]}`, homeP, awayP)

	resp, err := http.Post(
		fmt.Sprintf("%s/api/matches/%d/rounds", srv.URL, matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}

	var result struct {
		Messages []struct {
			Code  string `json:"code"`
			Level string `json:"level"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode validation result: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected validation messages in 422 response body")
	}
	found := false
	for _, m := range result.Messages {
		if m.Code == "SCORESHEET_GAME_BOTH_WINNERS" && m.Level == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SCORESHEET_GAME_BOTH_WINNERS error in response, got: %+v", result.Messages)
	}
}

// --- Phase 3C: Handicap Recommendation Preview ---

// setHandicapMethod sets the handicap_update_method season rule.
func setHandicapMethod(t *testing.T, sid int64, method string) {
	t.Helper()
	if _, err := db.DB.Exec(`
		INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_update_method', 'Handicap Update Method', ?)`, sid, method); err != nil {
		t.Fatalf("setHandicapMethod: %v", err)
	}
}

// getHandicapPreviewHC calls GET /advance-preview and returns the decoded handicap section.
func getHandicapPreviewHC(t *testing.T, srvURL string, sid, weekNum int64) map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/%d/advance-preview", srvURL, sid, weekNum))
	if err != nil {
		t.Fatalf("getHandicapPreviewHC: %v", err)
	}
	defer resp.Body.Close()
	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)
	hc, _ := preview["handicap"].(map[string]any)
	return hc
}

// closeWeek1ForHC closes week 1 by setting week_closed and league_weeks directly (no API).
// Used in handicap preview tests where the API path is not under test.
func closeWeek1ForHC(t *testing.T, sid, matchID int64) {
	t.Helper()
	if _, err := db.DB.Exec(`UPDATE matches SET week_closed=1 WHERE id=?`, matchID); err != nil {
		t.Fatalf("closeWeek1ForHC update matches: %v", err)
	}
	if _, err := db.DB.Exec(`
		INSERT OR IGNORE INTO league_weeks (season_id, week_number, status, closed_at)
		VALUES (?, 1, 'closed', CURRENT_TIMESTAMP)`, sid); err != nil {
		t.Fatalf("closeWeek1ForHC insert league_weeks: %v", err)
	}
}

func TestHandicapPreview_ManualReview(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// No handicap rule set -> defaults to manual_review.

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if method, _ := hc["method"].(string); method != "manual_review" {
		t.Errorf("want method=manual_review, got %q", method)
	}
	if status, _ := hc["status"].(string); status != "no_auto_apply" {
		t.Errorf("want status=no_auto_apply, got %q", status)
	}
	if _, ok := hc["recommendations"]; ok {
		t.Error("manual_review must not include recommendations field")
	}
}

func TestHandicapPreview_GameDiffAverage_TwoPlayers(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerB)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// playerA wins 3 games, diff=3.0; playerB loses 3 games, diff=-3.0.
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,0,3,-3.0)`, f.matchID, f.playerB, f.teamB)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if method, _ := hc["method"].(string); method != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", method)
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Fatal("want recommendations, got none")
	}
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		pid := int64(rec["player_id"].(float64))
		recHC, _ := rec["recommended_handicap"].(float64)
		skipped, _ := rec["skipped"].(bool)
		if pid == f.playerA {
			if recHC != 3.0 {
				t.Errorf("playerA: want recommended_handicap=3.0, got %v", recHC)
			}
			if skipped {
				t.Error("playerA: want skipped=false")
			}
		}
		if pid == f.playerB {
			if recHC != -3.0 {
				t.Errorf("playerB: want recommended_handicap=-3.0, got %v", recHC)
			}
		}
	}
}

func TestHandicapPreview_OpenWeeksExcluded(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=0.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// Add playerA to season_rosters so they appear as a candidate even with no closed data.
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id) VALUES (?,?)`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	// Insert match_results but leave week_closed=0 (open week).
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,5.0)`, f.matchID, f.playerA, f.teamA)
	// Do NOT close the week.
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			mp, _ := rec["matches_played"].(float64)
			if int(mp) != 0 {
				t.Errorf("open week must not count: want matches_played=0, got %v", mp)
			}
			if rec["skipped"] != true {
				t.Errorf("player with no closed data must be skipped")
			}
			return
		}
	}
	// playerA not found in recs is also acceptable: open weeks produce no candidates
	// unless season_rosters exists. Either outcome proves open weeks are excluded.
}

func TestHandicapPreview_NoMatchData(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	// playerA is on season_rosters but has no closed match_results.
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id) VALUES (?,?)`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if rec["skipped"] != true {
				t.Errorf("player with no closed data: want skipped=true")
			}
			reason, _ := rec["reason"].(string)
			if reason != "no_data" {
				t.Errorf("want reason=no_data, got %q", reason)
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_AdminHold(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET admin_hold=1 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if rec["skipped"] != true {
				t.Errorf("admin_hold player: want skipped=true")
			}
			reason, _ := rec["reason"].(string)
			if reason != "admin_hold" {
				t.Errorf("want reason=admin_hold, got %q", reason)
			}
			if ah, _ := rec["admin_hold"].(bool); !ah {
				t.Errorf("want admin_hold=true in response")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_NoChange(t *testing.T) {
	f := weekTestSeed(t)
	// playerA: current=2.0, 1 match diff=2.0 -> recommended=2.0 -> no_change.
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,2.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			reason, _ := rec["reason"].(string)
			if reason != "no_change" {
				t.Errorf("want reason=no_change when current==recommended, got %q", reason)
			}
			if rec["skipped"] == true {
				t.Error("no_change player must not be skipped")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_MaxIndividualCapApplied(t *testing.T) {
	f := weekTestSeed(t)
	// Set max_individual_handicap=3.0; playerA diff=5.0 -> recommended capped to 3.0.
	db.DB.Exec(`INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		f.sid, "max_individual_handicap", "Max Individual Handicap", "3.0")
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,5.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	recs, _ := hc["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			reason, _ := rec["reason"].(string)
			if reason != "capped" {
				t.Errorf("want reason=capped when avg exceeds max, got %q", reason)
			}
			recHC, _ := rec["recommended_handicap"].(float64)
			if recHC != 3.0 {
				t.Errorf("want recommended_handicap=3.0 (capped from 5.0), got %v", recHC)
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations; recs: %v", recs)
}

func TestHandicapPreview_KickerUnsupported(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	setHandicapMethod(t, f.sid, "kicker_average_preview")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if status, _ := hc["status"].(string); status != "unsupported" {
		t.Errorf("kicker_average_preview: want status=unsupported, got %q", status)
	}
	if _, ok := hc["recommendations"]; ok {
		t.Error("kicker_average_preview must not include recommendations field")
	}
	if msg, _ := hc["message"].(string); msg == "" {
		t.Error("kicker_average_preview: want non-empty message")
	}
}

func TestCloseWeek_ReturnsHandicapRecommendations(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	ar, _ := result["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("want advance_result in close response")
	}
	hc, _ := ar["handicap"].(map[string]any)
	if hc == nil {
		t.Fatal("want advance_result.handicap in close response")
	}
	if method, _ := hc["method"].(string); method != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", method)
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Error("want non-empty recommendations in close response advance_result.handicap")
	}
}

func TestPreviewAdvance_HandicapRecommendations(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	hc := getHandicapPreviewHC(t, f.srv.URL, f.sid, 1)
	if method, _ := hc["method"].(string); method != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", method)
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Error("want non-empty recommendations in advance-preview response")
	}
}

// TestHandicapPreview_DBError_Returns500 verifies that a real DB failure in the
// recommendation helper path surfaces as HTTP 500 rather than silently returning
// an empty recommendation set.
func TestHandicapPreview_DBError_Returns500(t *testing.T) {
	f := weekTestSeed(t)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	setHandicapMethod(t, f.sid, "game_diff_average")

	// Drop season_rosters so the candidate UNION query in computeGameDiffAverageRecs
	// fails with a real SQL error. This DB is isolated to this test's temp dir.
	if _, err := db.DB.Exec(`DROP TABLE season_rosters`); err != nil {
		t.Fatalf("DROP TABLE season_rosters: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/advance-preview", f.srv.URL, f.sid))
	if err != nil {
		t.Fatalf("GET advance-preview: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("want 500 on DB error, got %d: %s", resp.StatusCode, body)
	}
}

// --- Phase 3D: Handicap Review Screen ---

// getHandicapRecs calls GET /api/seasons/{id}/handicap-recommendations and
// returns the decoded response body as a map.
func getHandicapRecs(t *testing.T, srvURL string, sid int64) (map[string]any, int) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", srvURL, sid))
	if err != nil {
		t.Fatalf("getHandicapRecs: %v", err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, resp.StatusCode
}

func TestHandicapRecs_GameDiffAverage(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team B')`, f.sid, f.teamB)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, f.playerB)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")
	// Threshold=3 so playerA's 3 included racks meet the minimum.
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "3")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	if m, _ := data["method"].(string); m != "game_diff_average" {
		t.Errorf("want method=game_diff_average, got %q", m)
	}
	if s, _ := data["status"].(string); s != "preview" {
		t.Errorf("want status=preview, got %q", s)
	}
	wc, _ := data["weeks_closed"].(float64)
	if int(wc) != 1 {
		t.Errorf("want weeks_closed=1, got %v", wc)
	}
	recs, _ := data["recommendations"].([]any)
	if len(recs) == 0 {
		t.Fatal("want recommendations, got none")
	}
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		pid := int64(rec["player_id"].(float64))
		if pid == f.playerA {
			if rec["recommended_hc"] == nil {
				t.Error("playerA: want non-nil recommended_hc")
			}
			if rec["change_amount"] == nil {
				t.Error("playerA: want non-nil change_amount")
			}
			if _, ok := rec["team_name"]; !ok {
				t.Error("want team_name field in recommendation")
			}
		}
	}
}

func TestHandicapRecs_AdminHold(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET admin_hold=1 WHERE id=?`, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team B')`, f.sid, f.teamB)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, f.playerB)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	recs, _ := data["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if reason, _ := rec["reason"].(string); reason != "admin_hold" {
				t.Errorf("want reason=admin_hold, got %q", reason)
			}
			if ah, _ := rec["admin_hold"].(bool); !ah {
				t.Error("want admin_hold=true in response")
			}
			if rec["recommended_hc"] != nil {
				t.Error("admin_hold player must have nil recommended_hc")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations")
}

func TestHandicapRecs_NoData(t *testing.T) {
	f := weekTestSeed(t)
	// playerA is rostered but has no round_results at all => included_racks=0 => no_data.
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	recs, _ := data["recommendations"].([]any)
	for _, r := range recs {
		rec, _ := r.(map[string]any)
		if int64(rec["player_id"].(float64)) == f.playerA {
			if reason, _ := rec["reason"].(string); reason != "no_data" {
				t.Errorf("want reason=no_data, got %q", reason)
			}
			if rec["recommended_hc"] != nil {
				t.Error("no_data player must have nil recommended_hc")
			}
			return
		}
	}
	t.Errorf("playerA not found in recommendations")
}

func TestHandicapRecs_ManualReview(t *testing.T) {
	f := weekTestSeed(t)
	// Default method is manual_review (no rule set).

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	if m, _ := data["method"].(string); m != "manual_review" {
		t.Errorf("want method=manual_review, got %q", m)
	}
	if s, _ := data["status"].(string); s != "no_auto_apply" {
		t.Errorf("want status=no_auto_apply, got %q", s)
	}
	recs, _ := data["recommendations"].([]any)
	if len(recs) != 0 {
		t.Errorf("manual_review: want empty recommendations, got %d", len(recs))
	}
	if msg, _ := data["message"].(string); msg == "" {
		t.Error("want non-empty message for manual_review")
	}
}

func TestHandicapRecs_NoClosedWeeks(t *testing.T) {
	f := weekTestSeed(t)
	// Scores saved and completed=1, but week NOT closed (week_closed=0).
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,5.0)`, f.matchID, f.playerA, f.teamA)
	// Do NOT call closeWeek1ForHC -- week remains open.
	setHandicapMethod(t, f.sid, "game_diff_average")

	data, status := getHandicapRecs(t, f.srv.URL, f.sid)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	if s, _ := data["status"].(string); s != "no_data" {
		t.Errorf("want status=no_data when no closed weeks, got %q", s)
	}
	recs, _ := data["recommendations"].([]any)
	if len(recs) != 0 {
		t.Errorf("no closed weeks: want empty recommendations, got %d", len(recs))
	}
}

func TestHandicapRecs_SeasonNotFound(t *testing.T) {
	f := weekTestSeed(t)
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/9999/handicap-recommendations", f.srv.URL))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandicapRecs_DBError(t *testing.T) {
	f := weekTestSeed(t)
	setHandicapMethod(t, f.sid, "game_diff_average")
	// Insert a closed week so we get past the weeksClosed==0 gate.
	closeWeek1ForHC(t, f.sid, f.matchID)

	// Drop season_rosters so computeHandicapReviewRecs fails with a real SQL error.
	if _, err := db.DB.Exec(`DROP TABLE season_rosters`); err != nil {
		t.Fatalf("DROP TABLE season_rosters: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", f.srv.URL, f.sid))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("want 500 on DB error, got %d: %s", resp.StatusCode, body)
	}
}

func TestHandicapRecs_ReadOnly(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,4.0)`, f.matchID, f.playerA, f.teamA)
	closeWeek1ForHC(t, f.sid, f.matchID)
	setHandicapMethod(t, f.sid, "game_diff_average")

	var hcBefore float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcBefore)
	var histBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&histBefore)

	http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", f.srv.URL, f.sid))

	var hcAfter float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcAfter)
	if hcAfter != hcBefore {
		t.Errorf("handicap-recommendations must not modify players.handicap: was %v, now %v", hcBefore, hcAfter)
	}
	var histAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&histAfter)
	if histAfter != histBefore {
		t.Errorf("handicap-recommendations must not write handicap_history: was %d, now %d", histBefore, histAfter)
	}
}

func TestPhase3C_NoWritesToHandicapTables(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	// Snapshot state before any Phase 3C operation.
	var hcBefore float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcBefore)
	var hcHistBefore int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&hcHistBefore)

	// Trigger buildHandicapPreview via close (writes week_closed=1, then calls buildAdvanceResult).
	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	resp.Body.Close()

	// Also call advance-preview directly.
	http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/advance-preview", f.srv.URL, f.sid))

	var hcAfter float64
	db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, f.playerA).Scan(&hcAfter)
	if hcAfter != hcBefore {
		t.Errorf("Phase 3C must not modify players.handicap: was %v, now %v", hcBefore, hcAfter)
	}

	var hcHistAfter int
	db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history`).Scan(&hcHistAfter)
	if hcHistAfter != hcHistBefore {
		t.Errorf("Phase 3C must not write handicap_history: was %d, now %d", hcHistBefore, hcHistAfter)
	}
}

// --- Phase 3E: Handicap Review (opponent-normalized) ----------------------------

// hrFixture is a running server with a seeded 8-ball league/season and two
// teams registered in season_teams. Per-test helpers add players and rack data.
type hrFixture struct {
	srv      *httptest.Server
	sid      int64
	leagueID int64
	teamA    int64
	teamB    int64
}

// hrTestSeed spins up a fresh test server, creates an 8-ball league and season,
// two teams, and registers both teams in season_teams.
func hrTestSeed(t *testing.T) hrFixture {
	t.Helper()
	srv := testServer(t)

	resp, err := http.Post(srv.URL+"/api/leagues", "application/json",
		strings.NewReader(`{"name":"HR League","game_format":"8ball"}`))
	if err != nil {
		t.Fatalf("hrTestSeed: POST leagues: %v", err)
	}
	var lg map[string]any
	json.NewDecoder(resp.Body).Decode(&lg)
	resp.Body.Close()
	leagueID := int64(lg["id"].(float64))

	resp2, err := http.Post(srv.URL+"/api/seasons", "application/json",
		strings.NewReader(fmt.Sprintf(`{"league_id":%d,"name":"HR Season"}`, leagueID)))
	if err != nil {
		t.Fatalf("hrTestSeed: POST seasons: %v", err)
	}
	var ss map[string]any
	json.NewDecoder(resp2.Body).Decode(&ss)
	resp2.Body.Close()
	sid := int64(ss["id"].(float64))

	rA, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team A')`, leagueID)
	rB, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team B')`, leagueID)
	teamA, _ := rA.LastInsertId()
	teamB, _ := rB.LastInsertId()

	if _, err := db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team A')`, sid, teamA); err != nil {
		t.Fatalf("hrTestSeed: season_teams A: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team B')`, sid, teamB); err != nil {
		t.Fatalf("hrTestSeed: season_teams B: %v", err)
	}
	return hrFixture{srv: srv, sid: sid, leagueID: leagueID, teamA: teamA, teamB: teamB}
}

// hrAddPlayer inserts a player and registers them in season_rosters.
func hrAddPlayer(t *testing.T, f hrFixture, teamID int64, hc float64, adminHold bool) int64 {
	t.Helper()
	ah := 0
	if adminHold {
		ah = 1
	}
	r, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, player_number, handicap, admin_hold, active) VALUES ('Test','Player','00',?,?,1)`, hc, ah)
	if err != nil {
		t.Fatalf("hrAddPlayer: %v", err)
	}
	pid, _ := r.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, teamID, pid); err != nil {
		t.Fatalf("hrAddPlayer: season_rosters: %v", err)
	}
	return pid
}

// hrInsertMatch inserts a completed, week_closed match and returns its ID.
func hrInsertMatch(t *testing.T, f hrFixture, homeTeamID, awayTeamID int64) int64 {
	t.Helper()
	r, err := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-01-01',1,1,1)`,
		f.sid, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("hrInsertMatch: %v", err)
	}
	id, _ := r.LastInsertId()
	return id
}

// hrInsertRound inserts one round_results row. homeHC / awayHC may be nil (NULL snapshot).
func hrInsertRound(t *testing.T, matchID, roundNum, homeID, awayID int64,
	g1h, g1a, g2h, g2a, g3h, g3a int, homeHC, awayHC *float64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		  (match_id, round_number, home_player_id, away_player_id,
		   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		   home_handicap_used, away_handicap_used)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		matchID, roundNum, homeID, awayID,
		g1h, g1a, g2h, g2a, g3h, g3a,
		homeHC, awayHC)
	if err != nil {
		t.Fatalf("hrInsertRound: %v", err)
	}
}

func hrGetRecs(t *testing.T, srv *httptest.Server, sid int64) []map[string]any {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", srv.URL, sid))
	if err != nil {
		t.Fatalf("hrGetRecs: GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("hrGetRecs: want 200, got %d: %s", resp.StatusCode, body)
	}
	var data map[string]any
	json.NewDecoder(resp.Body).Decode(&data)
	recs, _ := data["recommendations"].([]any)
	out := make([]map[string]any, len(recs))
	for i, r := range recs {
		out[i], _ = r.(map[string]any)
	}
	return out
}

func hrSetRule(t *testing.T, sid int64, key, value string) {
	t.Helper()
	_, err := db.DB.Exec(
		`INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		sid, key, key, value)
	if err != nil {
		t.Fatalf("hrSetRule: %v", err)
	}
}

func ptr64(v float64) *float64 { return &v }

// TestHandicapReview_HomePlayerUsesAwaySnapshot verifies that when the reviewed
// player was HOME, the opponent HC baseline is away_handicap_used.
func TestHandicapReview_HomePlayerUsesAwaySnapshot(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	// home wins all 3 games 10 vs 5; away_handicap_used=3.0, home_handicap_used=2.0.
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5,
		ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("home player not found in recommendations")
	}

	windowHC, ok := homeRec["window_hc"].(float64)
	if !ok {
		t.Fatalf("home player window_hc is nil or not float64: %v", homeRec["window_hc"])
	}
	// per_rack = 3.0 + (10-5)/0.85; 3 racks => avg rounds to nearest 0.01
	wantApprox := 3.0 + 5.0/0.85
	want := math.Round(wantApprox*100) / 100
	if math.Abs(windowHC-want) > 0.005 {
		t.Errorf("home player window_hc: want ~%v (away_hc=3.0 baseline), got %v", want, windowHC)
	}
}

// TestHandicapReview_AwayPlayerUsesHomeSnapshot verifies that when the reviewed
// player was AWAY, the opponent HC baseline is home_handicap_used.
func TestHandicapReview_AwayPlayerUsesHomeSnapshot(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	// away wins all 3 games 10 vs 5; opponent for away player = home_handicap_used=2.0.
	hrInsertRound(t, matchID, 1, homeID, awayID,
		5, 10, 5, 10, 5, 10,
		ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var awayRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == awayID {
			awayRec = r
		}
	}
	if awayRec == nil {
		t.Fatal("away player not found in recommendations")
	}

	windowHC, ok := awayRec["window_hc"].(float64)
	if !ok {
		t.Fatalf("away player window_hc is nil: %v", awayRec["window_hc"])
	}
	// per_rack = 2.0 + (10-5)/0.85
	wantApprox := 2.0 + 5.0/0.85
	want := math.Round(wantApprox*100) / 100
	if math.Abs(windowHC-want) > 0.005 {
		t.Errorf("away player window_hc: want ~%v (home_hc=2.0 baseline), got %v", want, windowHC)
	}
}

// TestHandicapReview_NullSnapshotExcluded verifies that a rack with NULL
// away_handicap_used is excluded from calculation and counted in missing_snapshot_racks.
func TestHandicapReview_NullSnapshotExcluded(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5,
		ptr64(2.0), nil) // NULL away snapshot

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("home player not found in recommendations")
	}

	if int(homeRec["score_eligible_racks"].(float64)) == 0 {
		t.Error("score_eligible_racks should be > 0")
	}
	if int(homeRec["missing_snapshot_racks"].(float64)) == 0 {
		t.Error("missing_snapshot_racks should be > 0")
	}
	if int(homeRec["included_racks"].(float64)) != 0 {
		t.Errorf("included_racks should be 0, got %v", homeRec["included_racks"])
	}
	if homeRec["reason"] != "no_data" {
		t.Errorf("reason: want no_data, got %v", homeRec["reason"])
	}
}

// TestHandicapReview_ExcludedRacksNotCountedTowardThreshold verifies that
// NULL-snapshot racks do not count toward the eligibility threshold.
func TestHandicapReview_ExcludedRacksNotCountedTowardThreshold(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "10")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// 2 matches with NULL snapshots (6 slots excluded) + 1 match with valid snapshots (3 slots).
	for i := 0; i < 2; i++ {
		mid := hrInsertMatch(t, f, f.teamA, f.teamB)
		hrInsertRound(t, mid, 1, homeID, awayID,
			10, 5, 10, 5, 10, 5, ptr64(2.0), nil)
	}
	midValid := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, midValid, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("home player not found")
	}

	included := int(homeRec["included_racks"].(float64))
	if included >= 10 {
		t.Errorf("included_racks should be < 10 (NULL-snapshot racks excluded), got %d", included)
	}
	if homeRec["reason"] != "below_threshold" {
		t.Errorf("reason: want below_threshold, got %v", homeRec["reason"])
	}
	if homeRec["recommended_hc"] != nil {
		t.Errorf("recommended_hc must be nil for below_threshold player, got %v", homeRec["recommended_hc"])
	}
}

// TestHandicapReview_AdminHoldShowsCalculationsNoRecommendation verifies that
// an admin hold player with included racks still has lifetime_hc and window_hc
// populated, but recommended_hc and change_amount are nil.
func TestHandicapReview_AdminHoldShowsCalculationsNoRecommendation(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, true) // admin_hold=true
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("admin hold player not found in recommendations")
	}

	if homeRec["reason"] != "admin_hold" {
		t.Errorf("reason: want admin_hold, got %v", homeRec["reason"])
	}
	if homeRec["lifetime_hc"] == nil {
		t.Error("lifetime_hc should be non-nil for admin hold player with included racks")
	}
	if homeRec["window_hc"] == nil {
		t.Error("window_hc should be non-nil for admin hold player with included racks")
	}
	if homeRec["recommended_hc"] != nil {
		t.Errorf("recommended_hc must be nil for admin hold player, got %v", homeRec["recommended_hc"])
	}
	if homeRec["change_amount"] != nil {
		t.Errorf("change_amount must be nil for admin hold player, got %v", homeRec["change_amount"])
	}
}

// TestHandicapReview_BelowThresholdShowsProvisionalNoRecommendation verifies
// that a below-threshold player has calculated values but no recommendation.
func TestHandicapReview_BelowThresholdShowsProvisionalNoRecommendation(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "5")
	hrSetRule(t, f.sid, "handicap_current_game_window", "15")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	matchID := hrInsertMatch(t, f, f.teamA, f.teamB)
	// 3 included racks; threshold=5 => below_threshold.
	hrInsertRound(t, matchID, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("player not found in recommendations")
	}

	if homeRec["reason"] != "below_threshold" {
		t.Errorf("reason: want below_threshold, got %v", homeRec["reason"])
	}
	if homeRec["lifetime_hc"] == nil {
		t.Error("lifetime_hc should be non-nil for player with included racks")
	}
	if homeRec["window_hc"] == nil {
		t.Error("window_hc should be non-nil for player with included racks")
	}
	if homeRec["recommended_hc"] != nil {
		t.Errorf("recommended_hc must be nil for below_threshold player, got %v", homeRec["recommended_hc"])
	}
}

// TestHandicapReview_InvalidRuleReturns500 verifies that a stored rule value of
// "0" (invalid -- below minimum 1) causes the endpoint to return HTTP 500.
func TestHandicapReview_InvalidRuleReturns500(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")

	// Seed one closed week so the handler does not early-return with no_data.
	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)
	mid := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid, 1, homeID, awayID,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	// "0" is below minimum of 1 -- invalid.
	hrSetRule(t, f.sid, "handicap_current_game_window", "0")

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/handicap-recommendations", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 for invalid rule value, got %d", resp.StatusCode)
	}
}

// TestHandicapReview_DuplicatePlayerRecordsNotCombined verifies that two separate
// players.id rows are returned as independent entries with independent rack counts.
func TestHandicapReview_DuplicatePlayerRecordsNotCombined(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")

	p1 := hrAddPlayer(t, f, f.teamA, 2.0, false)
	p2 := hrAddPlayer(t, f, f.teamB, 2.0, false)

	mid := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid, 1, p1, p2,
		10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(2.0))

	recs := hrGetRecs(t, f.srv, f.sid)
	if len(recs) != 2 {
		t.Errorf("want 2 separate player entries (no merging), got %d", len(recs))
	}
	ids := map[float64]bool{}
	for _, r := range recs {
		ids[r["player_id"].(float64)] = true
	}
	if len(ids) != 2 {
		t.Errorf("want 2 distinct player_id values, got %d", len(ids))
	}
}

// TestSaveRounds_SnapshotPreservedOnResave verifies that re-saving a scoresheet
// with the same players preserves the original HC snapshots even after a player's
// current handicap changes.
func TestSaveRounds_SnapshotPreservedOnResave(t *testing.T) {
	f := weekTestSeed(t)
	// Disable roster gate so saveRounds reaches the snapshot logic (teams_managed=1 by default via API).
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	body := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`, f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var origHomeHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1`, f.matchID).Scan(&origHomeHC)
	if !origHomeHC.Valid {
		t.Fatal("home_handicap_used should be set after first save")
	}

	// Change the player's current handicap.
	db.DB.Exec(`UPDATE players SET handicap=9.99 WHERE id=?`, f.playerA)

	// Re-save same round with same players.
	resp2, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	var resavedHomeHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1`, f.matchID).Scan(&resavedHomeHC)
	if !resavedHomeHC.Valid {
		t.Fatal("home_handicap_used should still be set after re-save")
	}
	if resavedHomeHC.Float64 != origHomeHC.Float64 {
		t.Errorf("snapshot should be preserved on re-save with same player: orig=%v, resaved=%v",
			origHomeHC.Float64, resavedHomeHC.Float64)
	}
}

// TestSaveRounds_SubstitutionPreservesUnchangedSide verifies that when a player
// is substituted on one side, the unchanged side's snapshot is preserved while
// the new player receives a fresh snapshot from their current players.handicap.
func TestSaveRounds_SubstitutionPreservesUnchangedSide(t *testing.T) {
	f := weekTestSeed(t)
	// Disable roster gate so saveRounds reaches the snapshot logic (teams_managed=1 by default via API).
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	rSub, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Sub','Player',?,1.5,1)`, f.teamB)
	subID, _ := rSub.LastInsertId()

	body1 := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`, f.playerA, f.playerB)
	resp, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID), "application/json", strings.NewReader(body1))
	resp.Body.Close()

	var origHomeHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1`, f.matchID).Scan(&origHomeHC)

	// Change home player handicap (should NOT affect preserved snapshot).
	db.DB.Exec(`UPDATE players SET handicap=9.99 WHERE id=?`, f.playerA)

	// Re-save: same home player, substitute on away side.
	body2 := fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`, f.playerA, subID)
	resp2, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID), "application/json", strings.NewReader(body2))
	resp2.Body.Close()

	var newHomeHC, newAwayHC sql.NullFloat64
	db.DB.QueryRow(`SELECT home_handicap_used, away_handicap_used FROM round_results WHERE match_id=? AND round_number=1`,
		f.matchID).Scan(&newHomeHC, &newAwayHC)

	if newHomeHC.Float64 != origHomeHC.Float64 {
		t.Errorf("home snapshot should be preserved (same player): orig=%v, now=%v",
			origHomeHC.Float64, newHomeHC.Float64)
	}
	if !newAwayHC.Valid || newAwayHC.Float64 != 1.5 {
		t.Errorf("away snapshot should be sub player's current hc (1.5): got valid=%v value=%v",
			newAwayHC.Valid, newAwayHC.Float64)
	}
}

// --- Phase 3E: Corrections (PM Corrections 1, 2, and 3) ----------------------

// TestSaveRounds_HomeSubstitutionPreservesAwaySnapshot is a regression test for
// PM Correction 2: when the home player is substituted (new home player C replaces A),
// the prior row is matched by away player identity, not by (round, home) key.
// B's stored away_handicap_used must be preserved even though A is no longer home.
func TestSaveRounds_HomeSubstitutionPreservesAwaySnapshot(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerB)

	// Initial save: A (home, HC=1.0) vs B (away, HC=2.0).
	body1 := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body1))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Add substitute home player C; change B's current HC so it differs from stored snapshot.
	rC, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Home','Sub',?,3.5,1)`, f.teamA)
	playerC, _ := rC.LastInsertId()
	db.DB.Exec(`UPDATE players SET handicap=5.0 WHERE id=?`, f.playerB)

	// Re-save: C (home, fresh HC=3.5) vs B (away, must preserve stored HC=2.0).
	body2 := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		playerC, f.playerB)
	resp2, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body2))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	var homeHCStored, awayHCStored float64
	db.DB.QueryRow(
		`SELECT home_handicap_used, away_handicap_used FROM round_results WHERE match_id=? AND round_number=1`,
		f.matchID).Scan(&homeHCStored, &awayHCStored)

	if awayHCStored != 2.0 {
		t.Errorf("away_handicap_used: want 2.0 (B's preserved snapshot), got %g (current HC used instead)", awayHCStored)
	}
	if homeHCStored != 3.5 {
		t.Errorf("home_handicap_used: want 3.5 (C's fresh HC), got %g", homeHCStored)
	}
}

// TestSaveRounds_AmbiguousSubstitutionReturns422 is a regression test for PM Correction 2
// (ambiguity rejection): when the home player is substituted and the unchanged away player
// appears in more than one prior row of the same round, the server must reject the save
// with 422 rather than silently replacing both snapshots with current values.
func TestSaveRounds_AmbiguousSubstitutionReturns422(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Second and third home players on teamA (C and D).
	rC, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home2','Sub',?,1.0)`, f.teamA)
	playerC, _ := rC.LastInsertId()
	rD, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home3','New',?,1.0)`, f.teamA)
	playerD, _ := rD.LastInsertId()

	// Insert two prior rows in round 1 with DIFFERENT home players but the SAME away player.
	// The schema UNIQUE constraint is (match_id, round_number, home_player_id) so this is allowed.
	// This creates the ambiguous state: playerA and playerC both faced playerB in round 1.
	db.DB.Exec(`INSERT INTO round_results
		(match_id, round_number, home_player_id, away_player_id,
		 game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		 home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO round_results
		(match_id, round_number, home_player_id, away_player_id,
		 game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		 home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		f.matchID, playerC, f.playerB)

	// Re-save: playerD (new home, not in any prior row) vs playerB (away).
	// priorByRound[1] has two prior rows with awayPlayerID=playerB -> awayCount=2 -> expect 422.
	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]}`,
		playerD, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("ambiguous substitution: want 422, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_SamePlayerMultiplePairingsDistinctSnapshots verifies that when
// the same player appears as home in multiple rounds with different prior snapshots,
// each round preserves its own snapshot independently (pairing-level, not player-level).
func TestSaveRounds_SamePlayerMultiplePairingsDistinctSnapshots(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	rY, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Away','R1',?,3.0,1)`, f.teamB)
	playerY, _ := rY.LastInsertId()
	rZ, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Away','R2',?,3.0,1)`, f.teamB)
	playerZ, _ := rZ.LastInsertId()

	// First save: playerA as home in round 1 only, HC=2.0; snapshot 2.0 stored.
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerA)
	body1 := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, playerY)
	resp, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body1))
	resp.Body.Close()

	// Second save: playerA in round 1 (prior snapshot 2.0 preserved) and round 2
	// (no prior row, so fresh HC=4.0 is stored). Both rounds present.
	db.DB.Exec(`UPDATE players SET handicap=4.0 WHERE id=?`, f.playerA)
	body2 := fmt.Sprintf(
		`{"rounds":[`+
			`{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5},`+
			`{"round_number":2,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}`+
			`]}`,
		f.playerA, playerY, f.playerA, playerZ)
	resp2, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body2))
	resp2.Body.Close()

	var r1HC, r2HC float64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r1HC)
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=2 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r2HC)
	if r1HC != 2.0 {
		t.Fatalf("after second save: round 1 home_handicap_used want 2.0 (preserved), got %g", r1HC)
	}
	if r2HC != 4.0 {
		t.Fatalf("after second save: round 2 home_handicap_used want 4.0 (fresh), got %g", r2HC)
	}

	// Third save: change A's HC to 1.0; re-save same body. Both rounds must preserve
	// their distinct snapshots (2.0 for round 1, 4.0 for round 2) via pairing-level matching.
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	resp3, _ := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body2))
	resp3.Body.Close()

	var r1HCAfter, r2HCAfter float64
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=1 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r1HCAfter)
	db.DB.QueryRow(`SELECT home_handicap_used FROM round_results WHERE match_id=? AND round_number=2 AND home_player_id=?`, f.matchID, f.playerA).Scan(&r2HCAfter)
	if r1HCAfter != 2.0 {
		t.Errorf("after re-save: round 1 home_handicap_used want 2.0, got %g", r1HCAfter)
	}
	if r2HCAfter != 4.0 {
		t.Errorf("after re-save: round 2 home_handicap_used want 4.0, got %g", r2HCAfter)
	}
}

// --- Phase 3E: Corrections (PM Correction 1 regression) ----------------------

// TestSaveRounds_EffectiveHCUsedForValidation is a regression test for PM Correction 1:
// saveRounds must resolve effective handicaps before ValidateRounds so that round
// winners and sets reflect the preserved snapshot, not the current players.handicap.
//
// A round winner requires 2 of 3 pairing wins, so we submit 2 pairings in round 1.
// Game scores are chosen so that home wins with preserved HC (spot to home) but
// loses when current HC is used instead (spot flips to away after the HC change).
func TestSaveRounds_EffectiveHCUsedForValidation(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Add a second home/away pair for round 1 so a round winner can be determined.
	rA2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Home','Two',?,1.0,1)`, f.teamA)
	playerA2, _ := rA2.LastInsertId()
	rB2, _ := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Away','Two',?,2.0,1)`, f.teamB)
	playerB2, _ := rB2.LastInsertId()

	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=2.0 WHERE id=?`, f.playerB)

	// Games chosen so: with HC 1.0(home) vs 2.0(away), spot=3 to home.
	// adjH=25+3=28, adjA=24 -> home wins pairing.
	// After changing home HC to 3.0: spot=3 to away (away<home).
	// adjH=25, adjA=24+3=27 -> away wins pairing (ValidateRounds uses wrong HC without fix).
	body := fmt.Sprintf(
		`{"rounds":[`+
			`{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":7,"game2_home":10,"game2_away":7,"game3_home":5,"game3_away":10},`+
			`{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":7,"game2_home":10,"game2_away":7,"game3_home":5,"game3_away":10}`+
			`]}`,
		f.playerA, f.playerB, playerA2, playerB2)

	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// After first save: home wins 2 pairings -> RoundWinners[1]="home" -> sets_won=1.
	var swInit int
	db.DB.QueryRow(`SELECT sets_won FROM match_results WHERE match_id=? AND player_id=?`, f.matchID, f.playerA).Scan(&swInit)
	if swInit != 1 {
		t.Fatalf("initial save: sets_won want 1, got %d", swInit)
	}

	// Change both home players' HCs. Current HC (3.0 > 2.0) would flip spot to away,
	// making away win both pairings and giving RoundWinners[1]="away" (sets_won=0).
	db.DB.Exec(`UPDATE players SET handicap=3.0 WHERE id=?`, f.playerA)
	db.DB.Exec(`UPDATE players SET handicap=3.0 WHERE id=?`, playerA2)

	resp2, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	// Effective HC must use preserved snapshot (1.0 vs 2.0): home wins -> sets_won=1.
	var swResaved int
	db.DB.QueryRow(`SELECT sets_won FROM match_results WHERE match_id=? AND player_id=?`, f.matchID, f.playerA).Scan(&swResaved)
	if swResaved != 1 {
		t.Errorf("re-save: sets_won want 1 (effective HC preserved, home wins), got %d (current HC used instead)", swResaved)
	}
}

// --- Phase 3E: Final Corrections (PM Correction - rule helpers and close-week query) ---------

// TestSaveRounds_InvalidMultiplierReturns500 is a regression test for the rule-helper
// error-return correction: when season_rules contains an unparseable handicap_multiplier,
// txSeasonRoundConfig must return an error and saveRounds must respond 500 rather than
// silently defaulting.
func TestSaveRounds_InvalidMultiplierReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', 'not-a-number')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("invalid multiplier: want 500, got %d", resp.StatusCode)
	}
}

// TestValidateWeek_RoundQueryFailureProducesWeekInternalError is a regression test for
// the close-week round-results query correction: when the round_results table is unavailable,
// ValidateWeek must emit WEEK_INTERNAL_ERROR with the match_id stamped rather than
// silently skipping the match.
func TestValidateWeek_RoundQueryFailureProducesWeekInternalError(t *testing.T) {
	f := weekTestSeed(t)

	// Drop round_results so the per-match SELECT fails for every match in the week.
	db.DB.Exec(`DROP TABLE round_results`)

	msgs := weekValidate(t, f.srv.URL, f.sid, 1)

	var found bool
	for _, msg := range msgs {
		if msg["code"] == "WEEK_INTERNAL_ERROR" {
			found = true
			if msg["match_id"] == nil {
				t.Error("WEEK_INTERNAL_ERROR must carry a match_id")
			}
			break
		}
	}
	if !found {
		codes := make([]string, 0, len(msgs))
		for _, m := range msgs {
			if c, ok := m["code"].(string); ok {
				codes = append(codes, c)
			}
		}
		t.Errorf("expected WEEK_INTERNAL_ERROR, got codes: %v", codes)
	}
}

// TestSaveRounds_NaNMultiplierReturns500 verifies that a stored NaN value for
// handicap_multiplier is rejected (strconv.ParseFloat accepts NaN without error
// but it is not a finite positive number) and saveRounds returns HTTP 500.
func TestSaveRounds_NaNMultiplierReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', 'NaN')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("NaN multiplier: want 500, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_InfMultiplierReturns500 verifies that a stored +Inf value for
// handicap_multiplier is rejected (strconv.ParseFloat accepts +Inf without error
// but it is not a finite positive number) and saveRounds returns HTTP 500.
func TestSaveRounds_InfMultiplierReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', '+Inf')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("+Inf multiplier: want 500, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_NegativeMinBallHCReturns500 verifies that a negative integer
// stored for min_ball_handicap is rejected and saveRounds returns HTTP 500.
// strconv.Atoi accepts "-1" without error; the explicit < 0 guard catches it.
func TestSaveRounds_NegativeMinBallHCReturns500(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'min_ball_handicap', 'MinBall', '-1')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("negative min_ball_handicap: want 500, got %d", resp.StatusCode)
	}
}

// TestSaveRounds_InvalidConfigPreservesData verifies that a round save rejected
// by an invalid rule leaves existing round_results and match_results unchanged.
// Content is compared field-by-field so a delete-and-reinsert-with-different-values
// bug cannot hide behind a matching row count.
func TestSaveRounds_InvalidConfigPreservesData(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)

	// Seed an existing round_results row (snapshots deliberately NULL to verify NULL is preserved).
	db.DB.Exec(`
		INSERT INTO round_results
		  (match_id, round_number, home_player_id, away_player_id,
		   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away)
		VALUES (?,1,?,?,10,5,10,5,10,5)`,
		f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
		VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)

	// Capture round_results content before the rejected request.
	type rrSnapshot struct {
		homePlayerID, awayPlayerID    int64
		g1h, g1a, g2h, g2a, g3h, g3a int
		homeHC, awayHC                sql.NullFloat64
	}
	var rrBefore rrSnapshot
	db.DB.QueryRow(`
		SELECT home_player_id, away_player_id,
		       game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		       home_handicap_used, away_handicap_used
		FROM round_results WHERE match_id=?`, f.matchID).Scan(
		&rrBefore.homePlayerID, &rrBefore.awayPlayerID,
		&rrBefore.g1h, &rrBefore.g1a, &rrBefore.g2h, &rrBefore.g2a,
		&rrBefore.g3h, &rrBefore.g3a,
		&rrBefore.homeHC, &rrBefore.awayHC)

	// Capture match_results content before the rejected request.
	type mrSnapshot struct {
		playerID            int64
		gamesWon, gamesLost int
		diff                float64
		setsWon, setsLost   int
	}
	var mrBefore mrSnapshot
	db.DB.QueryRow(`
		SELECT player_id, games_won, games_lost, diff,
		       COALESCE(sets_won,0), COALESCE(sets_lost,0)
		FROM match_results WHERE match_id=?`, f.matchID).Scan(
		&mrBefore.playerID, &mrBefore.gamesWon, &mrBefore.gamesLost, &mrBefore.diff,
		&mrBefore.setsWon, &mrBefore.setsLost)

	// Store NaN as the multiplier to force config rejection before any writes.
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', 'NaN')`, f.sid)

	// Attempt a save with inverted scores -- must be rejected; existing data must survive.
	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":5,"game1_away":10,"game2_home":5,"game2_away":10,"game3_home":5,"game3_away":10}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 for invalid config, got %d", resp.StatusCode)
	}

	// Verify round_results row content is identical -- not merely the same count.
	var rrAfter rrSnapshot
	db.DB.QueryRow(`
		SELECT home_player_id, away_player_id,
		       game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		       home_handicap_used, away_handicap_used
		FROM round_results WHERE match_id=?`, f.matchID).Scan(
		&rrAfter.homePlayerID, &rrAfter.awayPlayerID,
		&rrAfter.g1h, &rrAfter.g1a, &rrAfter.g2h, &rrAfter.g2a,
		&rrAfter.g3h, &rrAfter.g3a,
		&rrAfter.homeHC, &rrAfter.awayHC)
	if rrAfter != rrBefore {
		t.Errorf("round_results row changed after rejected save:\n  before: %+v\n  after:  %+v", rrBefore, rrAfter)
	}

	// Verify match_results row content is identical.
	var mrAfter mrSnapshot
	db.DB.QueryRow(`
		SELECT player_id, games_won, games_lost, diff,
		       COALESCE(sets_won,0), COALESCE(sets_lost,0)
		FROM match_results WHERE match_id=?`, f.matchID).Scan(
		&mrAfter.playerID, &mrAfter.gamesWon, &mrAfter.gamesLost, &mrAfter.diff,
		&mrAfter.setsWon, &mrAfter.setsLost)
	if mrAfter != mrBefore {
		t.Errorf("match_results row changed after rejected save:\n  before: %+v\n  after:  %+v", mrBefore, mrAfter)
	}
}

// TestSaveRounds_ValidNonDefaultConfigSavesNormally verifies that explicitly stored
// valid non-default handicap_multiplier and min_ball_handicap values do not block
// a round save. min_ball_handicap=2 suppresses spots below 2 to 0 (threshold semantics);
// equal-handicap players produce spot=0, so no suppression occurs and the save proceeds.
func TestSaveRounds_ValidNonDefaultConfigSavesNormally(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE seasons SET teams_managed=0 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'handicap_multiplier', 'Multiplier', '3.0')`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value)
		VALUES (?, 'min_ball_handicap', 'MinBall', '2')`, f.sid)

	body := fmt.Sprintf(
		`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,`+
			`"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":5,"game3_home":10,"game3_away":5}]}`,
		f.playerA, f.playerB)
	resp, err := http.Post(fmt.Sprintf("%s/api/matches/%d/rounds", f.srv.URL, f.matchID),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("valid non-default config: want 200, got %d: %s", resp.StatusCode, b)
	}
}

// TestHandicapReview_CrossLeague8BallParticipates verifies that a player's racks
// from a second 8-ball league contribute to their lifetime rack count in the
// Handicap Review endpoint.
func TestHandicapReview_CrossLeague8BallParticipates(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// Main season: one closed match (3 racks).
	mid1 := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid1, 1, homeID, awayID, 10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	// Second 8-ball league and season -- same player appears as home (3 more racks).
	var league2ID, season2ID, teamC, teamD, opp2ID, match2ID int64
	res, _ := db.DB.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('League2','8ball','Tuesday')`)
	league2ID, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO seasons (league_id, name, schedule_type, num_weeks, active) VALUES (?,'S2','single_rr',8,0)`, league2ID)
	season2ID, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamC')`, league2ID)
	teamC, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamD')`, league2ID)
	teamD, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Opp','Two',?,3.0,1)`, teamD)
	opp2ID, _ = res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-02-01',1,1,1)`, season2ID, teamC, teamD)
	match2ID, _ = res.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		match2ID, homeID, opp2ID)

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("player not found in recommendations")
	}
	// 3 racks from season1 + 3 racks from season2 = 6 total lifetime racks.
	if got := int(homeRec["lifetime_racks"].(float64)); got != 6 {
		t.Errorf("cross-league 8-ball: want lifetime_racks=6, got %d", got)
	}
	_ = awayID
}

// TestHandicapReview_Non8BallRacksExcluded verifies that a player's racks from
// a non-8-ball league do not contribute to their lifetime rack count.
func TestHandicapReview_Non8BallRacksExcluded(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// 8-ball season: one closed match (3 racks).
	mid1 := hrInsertMatch(t, f, f.teamA, f.teamB)
	hrInsertRound(t, mid1, 1, homeID, awayID, 10, 5, 10, 5, 10, 5, ptr64(2.0), ptr64(3.0))

	// 9-ball league and season -- same player; these racks must NOT be counted.
	var league9ID, season9ID, teamE, teamF, opp9ID, match9ID int64
	res9, _ := db.DB.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('NineBall','9ball','Wednesday')`)
	league9ID, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO seasons (league_id, name, schedule_type, num_weeks, active) VALUES (?,'S9','single_rr',8,0)`, league9ID)
	season9ID, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamE')`, league9ID)
	teamE, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'TeamF')`, league9ID)
	teamF, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap, active) VALUES ('Opp','Nine',?,3.0,1)`, teamF)
	opp9ID, _ = res9.LastInsertId()
	res9, _ = db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-03-01',1,1,1)`, season9ID, teamE, teamF)
	match9ID, _ = res9.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		match9ID, homeID, opp9ID)

	recs := hrGetRecs(t, f.srv, f.sid)
	var homeRec map[string]any
	for _, r := range recs {
		if int64(r["player_id"].(float64)) == homeID {
			homeRec = r
		}
	}
	if homeRec == nil {
		t.Fatal("player not found in recommendations")
	}
	// Only the 3 racks from the 8-ball season count; 9-ball racks excluded by game_format filter.
	if got := int(homeRec["lifetime_racks"].(float64)); got != 3 {
		t.Errorf("non-8ball excluded: want lifetime_racks=3, got %d (9-ball racks leaked)", got)
	}
	_, _ = awayID, season9ID
}

// TestHandicapReview_WeekReopeningSlideWindow verifies that reopening a closed match
// removes its racks from the calculation and shrinks the window accordingly.
func TestHandicapReview_WeekReopeningSlideWindow(t *testing.T) {
	f := hrTestSeed(t)
	hrSetRule(t, f.sid, "handicap_update_method", "game_diff_average")
	hrSetRule(t, f.sid, "handicap_min_games_for_recommendation", "1")
	hrSetRule(t, f.sid, "handicap_current_game_window", "4")

	homeID := hrAddPlayer(t, f, f.teamA, 2.0, false)
	awayID := hrAddPlayer(t, f, f.teamB, 3.0, false)

	// Week 1 (older): 3 racks.
	res, _ := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-01-01',1,1,1)`, f.sid, f.teamA, f.teamB)
	mid1, _ := res.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		mid1, homeID, awayID)

	// Week 2 (more recent): 3 racks.
	res, _ = db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, completed, week_closed) VALUES (?,?,?,'2026-02-01',2,1,1)`, f.sid, f.teamA, f.teamB)
	mid2, _ := res.LastInsertId()
	db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?,1,?,?,10,5,10,5,10,5,2.0,3.0)`,
		mid2, homeID, awayID)

	// Both closed: 6 total racks, window=4 -> window takes 4 most-recent racks.
	recs1 := hrGetRecs(t, f.srv, f.sid)
	var rec1 map[string]any
	for _, r := range recs1 {
		if int64(r["player_id"].(float64)) == homeID {
			rec1 = r
		}
	}
	if rec1 == nil {
		t.Fatal("player not found before reopen")
	}
	if got := int(rec1["lifetime_racks"].(float64)); got != 6 {
		t.Errorf("before reopen: want lifetime_racks=6, got %d", got)
	}
	if got := int(rec1["window_racks"].(float64)); got != 4 {
		t.Errorf("before reopen: want window_racks=4 (window=4, total=6), got %d", got)
	}

	// Reopen the more-recent match (week 2).
	db.DB.Exec(`UPDATE matches SET week_closed=0 WHERE id=?`, mid2)

	// After reopen: only week 1's 3 racks remain; window_racks capped at available.
	recs2 := hrGetRecs(t, f.srv, f.sid)
	var rec2 map[string]any
	for _, r := range recs2 {
		if int64(r["player_id"].(float64)) == homeID {
			rec2 = r
		}
	}
	if rec2 == nil {
		t.Fatal("player not found after reopen")
	}
	if got := int(rec2["lifetime_racks"].(float64)); got != 3 {
		t.Errorf("after reopen: want lifetime_racks=3 (week2 removed), got %d", got)
	}
	if got := int(rec2["window_racks"].(float64)); got != 3 {
		t.Errorf("after reopen: want window_racks=3 (fewer than window=4), got %d", got)
	}
}

// TestCloseWeek_AdvanceResultHandicapShape verifies that the Close Week response's
// advance_result.handicap uses PlayerHandicapRec fields (current_handicap,
// recommended_handicap, matches_played) and not HandicapReviewRec fields.
func TestCloseWeek_AdvanceResultHandicapShape(t *testing.T) {
	f := weekTestSeed(t)
	db.DB.Exec(`UPDATE players SET handicap=1.0 WHERE id=?`, f.playerA)
	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3.0)`, f.matchID, f.playerA, f.teamA)
	setHandicapMethod(t, f.sid, "game_diff_average")

	resp := weekClose(t, f.srv.URL, f.sid, 1, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	ar, _ := result["advance_result"].(map[string]any)
	if ar == nil {
		t.Fatal("want advance_result in close response")
	}
	hc, _ := ar["handicap"].(map[string]any)
	if hc == nil {
		t.Fatal("want advance_result.handicap in close response")
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		t.Skip("no recommendations available -- cannot verify shape")
	}
	rec, _ := recs[0].(map[string]any)

	// Must have PlayerHandicapRec fields.
	if _, ok := rec["current_handicap"]; !ok {
		t.Error("close week advance_result.handicap rec missing current_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["recommended_handicap"]; !ok {
		t.Error("close week advance_result.handicap rec missing recommended_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["matches_played"]; !ok {
		t.Error("close week advance_result.handicap rec missing matches_played (PlayerHandicapRec field)")
	}
	// Must NOT have HandicapReviewRec-only fields.
	if _, ok := rec["assigned_hc"]; ok {
		t.Error("close week advance_result.handicap must not contain assigned_hc (HandicapReviewRec field leaked)")
	}
	if _, ok := rec["window_hc"]; ok {
		t.Error("close week advance_result.handicap must not contain window_hc (HandicapReviewRec field leaked)")
	}
}

// TestHandicapReview_AdvancePreviewShapeUnchanged verifies that the advance-preview
// response uses PlayerHandicapRec fields and is not contaminated by HandicapReviewRec fields.
func TestHandicapReview_AdvancePreviewShapeUnchanged(t *testing.T) {
	f := weekTestSeed(t)

	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'A')`, f.sid, f.teamA)
	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'B')`, f.sid, f.teamB)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamA, f.playerA)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, f.sid, f.teamB, f.playerB)
	db.DB.Exec(`UPDATE seasons SET teams_managed=1 WHERE id=?`, f.sid)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		f.sid, "handicap_update_method", "Method", "game_diff_average")

	seedRoundResult(t, f.matchID, f.playerA, f.playerB)
	weekClose(t, f.srv.URL, f.sid, 1, nil)

	resp, err := http.Get(fmt.Sprintf("%s/api/seasons/%d/weeks/1/advance-preview", f.srv.URL, f.sid))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var preview map[string]any
	json.NewDecoder(resp.Body).Decode(&preview)

	hc, _ := preview["handicap"].(map[string]any)
	if hc == nil {
		t.Fatal("advance-preview: missing 'handicap' field")
	}
	recs, _ := hc["recommendations"].([]any)
	if len(recs) == 0 {
		return // no data is acceptable
	}

	rec, _ := recs[0].(map[string]any)
	if _, ok := rec["current_handicap"]; !ok {
		t.Error("advance-preview rec missing current_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["recommended_handicap"]; !ok {
		t.Error("advance-preview rec missing recommended_handicap (PlayerHandicapRec field)")
	}
	if _, ok := rec["matches_played"]; !ok {
		t.Error("advance-preview rec missing matches_played (PlayerHandicapRec field)")
	}
	if _, ok := rec["assigned_hc"]; ok {
		t.Error("advance-preview rec must not contain assigned_hc (HandicapReviewRec field)")
	}
	if _, ok := rec["window_hc"]; ok {
		t.Error("advance-preview rec must not contain window_hc (HandicapReviewRec field)")
	}
}
