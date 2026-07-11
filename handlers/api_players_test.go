package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"league_app/db"
)

// --- DELETE /api/players/{id} -- handicap history guard ---

// seedPlayerViaAPI creates a player via the API and returns its numeric ID.
func seedPlayerViaAPI(t *testing.T, base string) int64 {
	t.Helper()
	body := `{"first_name":"Test","last_name":"Player","handicap":1.5,"team_id":null}`
	resp, err := http.Post(base+"/api/players", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/players: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create player: want 201, got %d", resp.StatusCode)
	}
	var p map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode player: %v", err)
	}
	return int64(p["id"].(float64))
}

// insertHandicapHistory inserts a raw handicap_history row directly into the DB.
func insertHandicapHistory(t *testing.T, playerID int64) {
	t.Helper()
	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?, 1.0, 2.0, '2026-01-01')`,
		playerID,
	); err != nil {
		t.Fatalf("insertHandicapHistory: %v", err)
	}
}

// TestDeletePlayer_NoHistory_Succeeds verifies that a player with no
// handicap history records can be deleted normally (200 OK).
func TestDeletePlayer_NoHistory_Succeeds(t *testing.T) {
	srv := testServer(t)
	playerID := seedPlayerViaAPI(t, srv.URL)

	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/players/%d", srv.URL, playerID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 for player with no history, got %d", resp.StatusCode)
	}
}

// TestDeletePlayer_WithHandicapHistory_Returns409 verifies that a player
// with at least one handicap_history row cannot be deleted (409 Conflict).
func TestDeletePlayer_WithHandicapHistory_Returns409(t *testing.T) {
	srv := testServer(t)
	playerID := seedPlayerViaAPI(t, srv.URL)
	insertHandicapHistory(t, playerID)

	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/players/%d", srv.URL, playerID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 for player with history, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(body["error"], "handicap history") {
		t.Errorf("want error message mentioning handicap history, got: %q", body["error"])
	}
}

// TestDeletePlayer_NonExistent_Returns200 verifies that deleting a player
// that doesn't exist still returns 200 (idempotent DELETE).
func TestDeletePlayer_NonExistent_Returns200(t *testing.T) {
	srv := testServer(t)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/players/999999", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (idempotent delete), got %d", resp.StatusCode)
	}
}
