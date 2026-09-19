package handlers_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// This file covers PM's final Users/Roles Phase 1 correction round, item 1:
// resolveIdentity (handlers/api_auth_middleware.go) previously tried a
// Bearer API key before a session cookie, so a request carrying BOTH a
// valid session cookie and a stale/different Bearer key (e.g. an Admin Key
// left in a browser tab's sessionStorage from an earlier test) executed as
// the Bearer key's identity, not the session's -- even though GET
// /api/auth/me (what the shell displays) already preferred the session.
// The fix reorders resolveIdentity to check the session cookie FIRST,
// falling back to Bearer only when no session resolves. These tests prove
// that reordering directly at the HTTP layer, independent of the frontend
// api-client.js fix (which stops a browser from ever attaching a stale key
// once a session is active -- that half has no Go-testable surface).

// seedApplyBearerKey creates a personal Bearer API key resolvable via
// ApplyAuth AND (when role is "system_admin") granted a matching global
// role_assignments row, so it actually carries power under the
// RoleAssignmentMgr-wired path every test server in this package uses --
// see sqlite.ApplyAuthStore.CreateApplyUser's own doc comment for why
// "system_admin" is the one role it grants automatically.
func seedApplyBearerKey(t *testing.T, username, role string) string {
	t.Helper()
	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	_, key, err := applyAuthStore.CreateApplyUser(context.Background(), username, role)
	if err != nil {
		t.Fatalf("CreateApplyUser(%s, %s): %v", username, role, err)
	}
	return key
}

// seedApplyBearerKeyScopedToLeague creates a personal Bearer API key with a
// real league_admin role_assignments row scoped to leagueID -- CreateApplyUser
// itself only auto-grants a role_assignments row for system_admin, so a
// scoped league_admin key needs an explicit Grant against the same
// RoleAssignmentStore the test server wires as RoleAssignmentMgr.
func seedApplyBearerKeyScopedToLeague(t *testing.T, username string, leagueID int64) string {
	t.Helper()
	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	ctx := context.Background()
	user, key, err := applyAuthStore.CreateApplyUser(ctx, username, "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser(%s, league_admin): %v", username, err)
	}
	if err := roleStore.Grant(ctx, user.ID, auth.RoleLeagueAdmin, &leagueID, nil); err != nil {
		t.Fatalf("Grant league_admin to %s: %v", username, err)
	}
	return key
}

// TestCredentialPrecedence_SessionWinsOverDifferentSystemAdminBearerKey
// proves: a non-system_admin session (here, a league_admin scoped to
// League A) plus a stale system_admin Bearer key attached to the SAME
// request must NOT execute as system_admin. POST /api/backup is genuinely
// system_admin-only (rejects league_admin outright, unlike league
// creation's self-grant path -- see its own comment in handlers/api.go),
// so this exercises the sharpest possible case: if the old Bearer-first
// order were still in effect, this request would succeed using the stale
// key's power.
func TestCredentialPrecedence_SessionWinsOverDifferentSystemAdminBearerKey(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "Precedence League A")
	seedPasswordUser(t, "precedence-adminA@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueA)
	cookies, csrf := loginCookies(t, srv, "precedence-adminA@example.com", "hunter22")
	staleSysAdminKey := seedApplyBearerKey(t, "stale-sysadmin-key", "system_admin")

	code := statusCode(t, srv, "POST", "/api/backup", "{}",
		cookies, map[string]string{"X-CSRF-Token": csrf, "Authorization": "Bearer " + staleSysAdminKey})
	if code != http.StatusForbidden {
		t.Errorf("league_admin session + stale system_admin Bearer key calling backup: want 403 (session identity, not the key's), got %d", code)
	}
}

// TestCredentialPrecedence_PlayerSessionCannotGainSystemAdminFromStaleKey
// proves the same precedence for a player session specifically -- PM's
// exact wording: "A player session cannot receive system-admin behavior
// merely because an old system-admin key remains in sessionStorage."
func TestCredentialPrecedence_PlayerSessionCannotGainSystemAdminFromStaleKey(t *testing.T) {
	srv := testServerWithAuth(t)
	playerUserID := seedPasswordUser(t, "precedence-player@example.com", "hunter22", "", nil)
	leagueA := seedTestLeague(t, "Precedence Player League")
	teamA := seedTestTeam(t, leagueA, "Precedence Player Team")
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('Precedence', 'Player', ?)`, teamA)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	playerID, _ := res.LastInsertId()
	if _, err := db.DB.Exec(`UPDATE users SET player_id=? WHERE id=?`, playerID, playerUserID); err != nil {
		t.Fatalf("link player: %v", err)
	}
	cookies, csrf := loginCookies(t, srv, "precedence-player@example.com", "hunter22")
	staleSysAdminKey := seedApplyBearerKey(t, "stale-sysadmin-key-2", "system_admin")

	code := statusCode(t, srv, "POST", "/api/backup", "{}",
		cookies, map[string]string{"X-CSRF-Token": csrf, "Authorization": "Bearer " + staleSysAdminKey})
	if code != http.StatusForbidden {
		t.Errorf("player session + stale system_admin Bearer key calling backup: want 403, got %d", code)
	}
}

// TestCredentialPrecedence_LeagueAdminSessionScopedByOwnAssignment_NotStaleKey
// proves: a league_admin session scoped to League A, plus a stale Bearer
// key granting league_admin over League B (or system_admin), still gets
// League A's own access and is still denied League B's -- the session's
// own assignment governs, never the stored key's.
func TestCredentialPrecedence_LeagueAdminSessionScopedByOwnAssignment_NotStaleKey(t *testing.T) {
	srv := testServerWithAuth(t)
	leagueA := seedTestLeague(t, "Precedence Scoped League A")
	leagueB := seedTestLeague(t, "Precedence Scoped League B")
	seedPasswordUser(t, "precedence-scopedA@example.com", "hunter22", auth.RoleLeagueAdmin, &leagueA)
	cookies, csrf := loginCookies(t, srv, "precedence-scopedA@example.com", "hunter22")
	headersNoBearer := map[string]string{"X-CSRF-Token": csrf}

	staleLeagueBKey := seedApplyBearerKeyScopedToLeague(t, "stale-leagueB-key", leagueB)
	headersWithStaleBearer := map[string]string{"X-CSRF-Token": csrf, "Authorization": "Bearer " + staleLeagueBKey}

	leagueBIDStr := strconv.FormatInt(leagueB, 10)
	teamBBody := `{"name":"League B Team via A session","league_id":` + leagueBIDStr + `}`
	if code := statusCode(t, srv, "POST", "/api/teams", teamBBody, cookies, headersWithStaleBearer); code != http.StatusForbidden {
		t.Errorf("League A session + stale League B Bearer key creating a team in League B: want 403 (own session scope only), got %d", code)
	}

	leagueAIDStr := strconv.FormatInt(leagueA, 10)
	teamABody := `{"name":"League A Team via A session","league_id":` + leagueAIDStr + `}`
	if code := statusCode(t, srv, "POST", "/api/teams", teamABody, cookies, headersNoBearer); code == http.StatusForbidden {
		t.Errorf("League A session creating a team in its own league (no Bearer attached): want non-403, got %d", code)
	}
}

// TestCredentialPrecedence_BearerFallbackStillWorksWithNoSession proves the
// other half of the contract: "The Admin Key remains the fallback when no
// session exists." No session cookie at all, only a Bearer key -- must
// still authenticate exactly as before this round's reordering.
func TestCredentialPrecedence_BearerFallbackStillWorksWithNoSession(t *testing.T) {
	srv := testServerWithAuth(t)
	key := seedApplyBearerKey(t, "fallback-sysadmin-key", "system_admin")

	code := statusCode(t, srv, "POST", "/api/backup", "{}", nil, map[string]string{"Authorization": "Bearer " + key})
	if code != http.StatusOK {
		t.Errorf("Bearer key with no session cookie present: want 200 (fallback still works), got %d", code)
	}
}

// TestCredentialPrecedence_SessionMutationStillRequiresCSRF_EvenWithBearerAttached
// proves: "Session mutations still require and send CSRF" holds even when a
// Bearer header also happens to be present on the request -- the session
// path is still the one taken (per the new precedence), so CSRF is still
// enforced, not bypassed because a Bearer header was seen.
func TestCredentialPrecedence_SessionMutationStillRequiresCSRF_EvenWithBearerAttached(t *testing.T) {
	srv := testServerWithAuth(t)
	seedPasswordUser(t, "precedence-csrf-sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, srv, "precedence-csrf-sysadmin@example.com", "hunter22")
	staleKey := seedApplyBearerKey(t, "stale-csrf-key", "system_admin")

	// No X-CSRF-Token header at all, but a valid Bearer key IS attached --
	// if Bearer still won the identity race, no CSRF would be required and
	// this would succeed (200). It must not.
	code := statusCode(t, srv, "POST", "/api/backup", "{}",
		cookies, map[string]string{"Authorization": "Bearer " + staleKey})
	if code != http.StatusForbidden {
		t.Errorf("session-authenticated mutation with no CSRF header (Bearer also attached): want 403, got %d", code)
	}
}
