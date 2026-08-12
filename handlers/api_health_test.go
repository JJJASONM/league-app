package handlers_test

// Health-check route integration tests: GET /healthz.

import (
	"encoding/json"
	"net/http"
	"testing"

	"league_app/db"
)

func TestHealthz_DatabaseUp_Returns200(t *testing.T) {
	srv := testServer(t)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("want status=ok, got %v", body)
	}
}

// TestHealthz_DatabaseDown_Returns503 simulates an unusable database
// connection by closing db.DB before the request, and verifies the health
// route reports 503 rather than a false 200 or a panic.
func TestHealthz_DatabaseDown_Returns503(t *testing.T) {
	srv := testServer(t)

	if err := db.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}
