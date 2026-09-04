// Auth enforcement tests for GET /api/players/{id}/overview
// (Player Overview Phase 2 correction, 2026-08-30). The route now exposes
// per-player dues/payment status, so it is protected by the same
// clearanceAuth chain (league_admin/admin/system_admin) Financial Phase 1
// uses for its own routes -- resolves PLAYERS-Q002 in doc/roadmap.md. Uses
// financeTestServer/financeSeed from api_finances_test.go, which already
// wires a real ApplyAuthStore and every manager this route needs
// (PlayerMgr, SeasonMgr, TeamMgr, MatchMgr, RoundMgr, FinanceMgr, RuleMgr).
package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"league_app/db"
)

func TestPlayerOverview_NoHeader_Returns401(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	resp, err := http.Get(fmt.Sprintf("%s/api/players/%d/overview?season_id=%d", srv.URL, playerID, seasonID))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("want WWW-Authenticate header on 401")
	}
}

func TestPlayerOverview_InvalidToken_Returns403(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/players/%d/overview?season_id=%d", srv.URL, playerID, seasonID), nil)
	req.Header.Set("Authorization", "Bearer not-a-real-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

// TestPlayerOverview_StaticAdminToken_Returns403 verifies the static
// LEAGUE_ADMIN_TOKEN (financeTestServer's bootstrap token) does not
// authorize this route -- personal-key-only auth has no static-token
// fallback, matching every other clearanceAuth route.
func TestPlayerOverview_StaticAdminToken_Returns403(t *testing.T) {
	srv, _ := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/players/%d/overview?season_id=%d", srv.URL, playerID, seasonID), nil)
	req.Header.Set("Authorization", "Bearer finance-test-admin-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (static admin token must not authorize this route), got %d", resp.StatusCode)
	}
}

func TestPlayerOverview_ScoreKeeperRole_Returns403(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "po-scorer", "score_keeper")
	if err != nil {
		t.Fatalf("create score_keeper user: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/api/players/%d/overview?season_id=%d", srv.URL, playerID, seasonID), nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 for score_keeper role, got %d", resp.StatusCode)
	}
}

func TestPlayerOverview_LeagueAdmin_ReachesHandler(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "po-league-admin", "league_admin")
	if err != nil {
		t.Fatalf("create league_admin user: %v", err)
	}

	resp, _ := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (league_admin reaches handler), got %d", resp.StatusCode)
	}
}

func TestPlayerOverview_Admin_ReachesHandler(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "po-admin", "admin")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	resp, _ := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (admin reaches handler), got %d", resp.StatusCode)
	}
}

func TestPlayerOverview_SystemAdmin_ReachesHandler(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	_, key, err := authStore.CreateApplyUser(context.Background(), "po-system-admin", "system_admin")
	if err != nil {
		t.Fatalf("create system_admin user: %v", err)
	}

	resp, _ := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (system_admin reaches handler), got %d", resp.StatusCode)
	}
}

// -- Player Account Access Phase 1: role=player ownership check -------------

// TestPlayerOverview_PlayerRole_CanAccessOwnOverview confirms a role=player
// user linked to a player can view that player's own overview.
func TestPlayerOverview_PlayerRole_CanAccessOwnOverview(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	_, key, err := authStore.CreateApplyPlayerUser(context.Background(), "po-player-own", playerID)
	if err != nil {
		t.Fatalf("create player user: %v", err)
	}

	resp, _ := getPlayerOverviewAuth(t, srv.URL, key, playerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (player role reaches own overview), got %d", resp.StatusCode)
	}
}

// TestPlayerOverview_PlayerRole_CannotAccessOtherPlayerOverview is the
// core Player Account Access Phase 1 guard: a role=player user must not be
// able to view a *different* player's overview, even though the route
// otherwise accepts any authenticated personal key.
func TestPlayerOverview_PlayerRole_CannotAccessOtherPlayerOverview(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, handicap) VALUES ('Other','Player',0)`)
	if err != nil {
		t.Fatalf("seed other player: %v", err)
	}
	otherPlayerID, _ := res.LastInsertId()

	_, key, err := authStore.CreateApplyPlayerUser(context.Background(), "po-player-other", playerID)
	if err != nil {
		t.Fatalf("create player user: %v", err)
	}

	resp, _ := getPlayerOverviewAuth(t, srv.URL, key, otherPlayerID, fmt.Sprintf("?season_id=%d", seasonID))
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (player role cannot view another player's overview), got %d", resp.StatusCode)
	}
}

// TestPlayerOverview_PlayerRole_CannotAccessFinanceRoutes confirms role=player
// does not accidentally gain access to Financial Phase 1's clearanceAuth
// routes, which allow only league_admin/admin/system_admin.
func TestPlayerOverview_PlayerRole_CannotAccessFinanceRoutes(t *testing.T) {
	srv, authStore := financeTestServer(t)
	seasonID, _, playerID := financeSeed(t)

	_, key, err := authStore.CreateApplyPlayerUser(context.Background(), "po-player-finance", playerID)
	if err != nil {
		t.Fatalf("create player user: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/seasons/%d/finances/dues", srv.URL, seasonID), nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (player role cannot access finance routes), got %d", resp.StatusCode)
	}
}

// TestPlayerOverview_PlayerRole_CannotAccessUsersRoutes confirms role=player
// does not gain access to the Users Admin routes (system_admin/admin only,
// or the static admin token).
func TestPlayerOverview_PlayerRole_CannotAccessUsersRoutes(t *testing.T) {
	srv, authStore := financeTestServer(t)
	_, _, playerID := financeSeed(t)

	_, key, err := authStore.CreateApplyPlayerUser(context.Background(), "po-player-users", playerID)
	if err != nil {
		t.Fatalf("create player user: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (player role cannot access GET /api/users), got %d", resp.StatusCode)
	}
}
