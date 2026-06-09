package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domains/rules"
	"league_app/db"
	"league_app/handlers"
)

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
	handlers.Register(mux, dir)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
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
	const want = 11
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
