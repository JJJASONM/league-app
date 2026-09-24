package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/models"
)

// This file covers PM's staging-UI-isolation correction round, item 3:
// the Users screen must show each account's REAL, current access (its
// role_assignments rows) rather than the legacy flat users.role column,
// which no longer reflects reality once scoped roles are in play (a
// league_admin created via the legacy endpoint with no grant yet has
// role="league_admin" but zero actual access; a role=player user shows
// role="admin"/whatever legacy value with no admin access at all).
// GET /api/users (listUsers -> ApplyAuthStore.ListApplyUsers) is the one
// backend boundary this data flows through -- these tests prove it now
// returns each user's real role_assignments rows over real HTTP, so the
// frontend has a coherent, structured shape to render from instead of
// inferring anything from users.role itself.

func findAccessUser(t *testing.T, users []models.User, username string) models.User {
	t.Helper()
	for _, u := range users {
		if u.Username == username {
			return u
		}
	}
	t.Fatalf("user %q not found in response", username)
	return models.User{}
}

func TestUsersList_ReturnsRealRoleAssignments_OverHTTP(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "Access Display League A")
	leagueB := seedTestLeague(t, "Access Display League B")

	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	ctx := context.Background()

	// A real system_admin, via the legacy endpoint -- CreateApplyUser
	// auto-grants a global system_admin role_assignments row for this
	// role, so its Assignments should reflect that directly.
	if _, _, err := applyAuthStore.CreateApplyUser(ctx, "access-sysadmin", "system_admin"); err != nil {
		t.Fatalf("CreateApplyUser(system_admin): %v", err)
	}

	// A league_admin explicitly granted two leagues (multiple assignments).
	multiUser, _, err := applyAuthStore.CreateApplyUser(ctx, "access-multileague", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser(league_admin): %v", err)
	}
	if err := roleStore.Grant(ctx, multiUser.ID, auth.RoleLeagueAdmin, &leagueA, nil); err != nil {
		t.Fatalf("Grant league A: %v", err)
	}
	if err := roleStore.Grant(ctx, multiUser.ID, auth.RoleLeagueAdmin, &leagueB, nil); err != nil {
		t.Fatalf("Grant league B: %v", err)
	}

	// A legacy-created league_admin with NO grant yet -- role="league_admin"
	// but genuinely zero access; the legacy Role field must still be
	// present so the UI can render an explicit no-access label (rather
	// than presenting the legacy role as if it granted current access).
	if _, _, err := applyAuthStore.CreateApplyUser(ctx, "access-legacy-noaccess", "league_admin"); err != nil {
		t.Fatalf("CreateApplyUser(legacy league_admin, no grant): %v", err)
	}

	// A role=player user -- no role_assignments row is ever created for
	// this role; access comes entirely from player_id.
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Access', 'Player')`)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	playerID, _ := res.LastInsertId()
	if _, _, err := applyAuthStore.CreateApplyPlayerUser(ctx, "access-player", playerID); err != nil {
		t.Fatalf("CreateApplyPlayerUser: %v", err)
	}

	seedPasswordUser(t, "access-caller-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, srv, "access-caller-sysadmin@example.com", "hunter22")

	resp := authDo(t, srv, "GET", "/api/users", "", cookies, nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /api/users: want 200, got %d", resp.StatusCode)
	}
	users := decodeJSON[[]models.User](t, resp)

	sysAdmin := findAccessUser(t, users, "access-sysadmin")
	if len(sysAdmin.Assignments) != 1 || sysAdmin.Assignments[0].RoleCode != "system_admin" || sysAdmin.Assignments[0].LeagueID != nil {
		t.Errorf("want one global system_admin assignment for access-sysadmin, got %+v", sysAdmin.Assignments)
	}

	multiLeague := findAccessUser(t, users, "access-multileague")
	if len(multiLeague.Assignments) != 2 {
		t.Errorf("want 2 league_admin assignments for access-multileague, got %+v", multiLeague.Assignments)
	}

	legacyNoAccess := findAccessUser(t, users, "access-legacy-noaccess")
	if len(legacyNoAccess.Assignments) != 0 {
		t.Errorf("want zero assignments for access-legacy-noaccess, got %+v", legacyNoAccess.Assignments)
	}
	if legacyNoAccess.Role != "league_admin" {
		t.Errorf("want legacy Role field preserved so the UI can render an explicit no-access label, got %q", legacyNoAccess.Role)
	}

	player := findAccessUser(t, users, "access-player")
	if len(player.Assignments) != 0 {
		t.Errorf("want zero role_assignments for access-player (access comes from player_id), got %+v", player.Assignments)
	}
	if player.PlayerID == nil || *player.PlayerID != playerID {
		t.Errorf("want player.PlayerID %d, got %v", playerID, player.PlayerID)
	}
}

func TestUsersList_LeagueAdminAndPlayerSessions_CannotAccessAPI(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "Access Denied League A")
	seedPasswordUser(t, "access-denied-leagueadmin@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueA)
	playerUserID := seedPasswordUser(t, "access-denied-player@example.com", "hunter22", "", nil)
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Denied', 'Player')`)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	playerID, _ := res.LastInsertId()
	if _, err := db.DB.Exec(`UPDATE users SET player_id=? WHERE id=?`, playerID, playerUserID); err != nil {
		t.Fatalf("link player: %v", err)
	}

	leagueAdminCookies, _ := loginCookies(t, srv, "access-denied-leagueadmin@example.com", "hunter22")
	if code := getCode(t, srv, "/api/users", leagueAdminCookies); code != http.StatusForbidden {
		t.Errorf("league_admin GET /api/users: want 403, got %d", code)
	}

	playerCookies, _ := loginCookies(t, srv, "access-denied-player@example.com", "hunter22")
	if code := getCode(t, srv, "/api/users", playerCookies); code != http.StatusForbidden {
		t.Errorf("player GET /api/users: want 403, got %d", code)
	}
}
