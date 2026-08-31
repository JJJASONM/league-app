// Tests for Player Overview Phase 2: money status backed by the finances
// domain added in Financial Phase 1. Reuses financeTestServer/financeSeed
// from api_finances_test.go, which already wires FinanceMgr (the shared
// testServer() helper in api_test.go deliberately does not, so the
// existing api_player_overview_test.go assertions of money.tracked=false
// continue to exercise that no-FinanceMgr fallback path unchanged).
//
// financeTestServer wires a real ApplyAuthStore, so -- as of the Phase 2
// auth correction -- these requests need a league_admin/admin/system_admin
// personal key, unlike the plain testServer()-based tests in
// api_player_overview_test.go (which leave ApplyAuth nil and so exercise
// clearanceAuth's nil-resolver passthrough instead). See
// api_player_overview_auth_test.go for the dedicated auth-enforcement
// coverage.
package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/models"
)

// getPlayerOverviewAuth is getPlayerOverview (declared in
// api_player_overview_test.go) with an Authorization header attached.
func getPlayerOverviewAuth(t *testing.T, base, token string, playerID int64, query string) (*http.Response, models.PlayerOverview) {
	t.Helper()
	url := fmt.Sprintf("%s/api/players/%d/overview%s", base, playerID, query)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	var overview models.PlayerOverview
	if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&overview); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp, overview
}

// financeAdminKey creates a league_admin personal-key user directly via the
// auth store (bypassing POST /api/users' creatable-role restriction, which
// would also accept league_admin but this keeps every money test's admin
// bootstrap uniform with the dedicated auth tests) and returns the key.
func financeAdminKey(t *testing.T, authStore *sqlite.ApplyAuthStore, username string) string {
	t.Helper()
	_, key, err := authStore.CreateApplyUser(context.Background(), username, "league_admin")
	if err != nil {
		t.Fatalf("create league_admin user: %v", err)
	}
	return key
}

func TestPlayerOverviewMoney_NoPayments_TrackedTrueUnpaid(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)
	key := financeAdminKey(t, authStore, "po-money-1")

	resp, overview := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if !overview.Money.Tracked {
		t.Error("want money.tracked=true")
	}
	if overview.Money.Paid {
		t.Error("want money.paid=false")
	}
	if overview.Money.TotalPaid != 0 {
		t.Errorf("want total_paid=0, got %v", overview.Money.TotalPaid)
	}
	if len(overview.Money.Payments) != 0 {
		t.Errorf("want 0 payments, got %d", len(overview.Money.Payments))
	}
	if overview.Money.Message == "" {
		t.Error("want a non-empty message when unpaid")
	}
}

func TestPlayerOverviewMoney_OnePayment_TrackedTruePaid(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, teamID, playerID := financeSeed(t)
	key := financeAdminKey(t, authStore, "po-money-2")

	if _, err := db.DB.Exec(`INSERT INTO dues_payments (season_id, player_id, team_id, amount, paid_at) VALUES (?,?,?,?,?)`,
		seasonID, playerID, teamID, 25.0, "2026-01-05"); err != nil {
		t.Fatalf("seed dues payment: %v", err)
	}

	resp, overview := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if !overview.Money.Tracked {
		t.Error("want money.tracked=true")
	}
	if !overview.Money.Paid {
		t.Error("want money.paid=true")
	}
	if overview.Money.TotalPaid != 25.0 {
		t.Errorf("want total_paid=25, got %v", overview.Money.TotalPaid)
	}
	if len(overview.Money.Payments) != 1 {
		t.Fatalf("want 1 payment, got %d", len(overview.Money.Payments))
	}
	if overview.Money.Message != "" {
		t.Errorf("want empty message when paid, got %q", overview.Money.Message)
	}
}

func TestPlayerOverviewMoney_DuesAmountRule_ShownInResponse(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)
	key := financeAdminKey(t, authStore, "po-money-3")

	if _, err := db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'dues_amount', 'Dues Amount', '25')`, seasonID); err != nil {
		t.Fatalf("seed dues_amount rule: %v", err)
	}

	resp, overview := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if overview.Money.DuesAmount != "25" {
		t.Errorf("want dues_amount=25, got %q", overview.Money.DuesAmount)
	}
}

// TestPlayerOverviewMoney_NotRosteredPlayer_StillGetsMoneyStatus verifies
// money composition uses playerID directly (not the resolved team/roster),
// so a player with no season roster entry still gets a real dues status
// rather than silently falling back to the untracked placeholder.
func TestPlayerOverviewMoney_NotRosteredPlayer_StillGetsMoneyStatus(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, _ := financeSeed(t)
	key := financeAdminKey(t, authStore, "po-money-4")

	r, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, handicap) VALUES ('NotRostered','Player',0)`)
	if err != nil {
		t.Fatalf("seed unrostered player: %v", err)
	}
	playerID, _ := r.LastInsertId()

	resp, overview := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if !overview.Money.Tracked {
		t.Error("want money.tracked=true even when not rostered")
	}
	if overview.Money.Paid {
		t.Error("want money.paid=false")
	}
}
