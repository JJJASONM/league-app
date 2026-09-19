package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/backend/domains/leagues"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// TestLeagueSelfGrantStore_Success proves a normal creation commits both
// the league row and the creator's league_admin grant together.
func TestLeagueSelfGrantStore_Success(t *testing.T) {
	newAuthTestDB(t)
	ctx := context.Background()
	store := sqlite.NewLeagueSelfGrantStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	creatorID := seedAuthUser(t, "self-grant-creator")

	l, err := store.CreateLeagueWithLeagueAdminGrant(ctx, leagues.CreateLeagueInput{
		Name: "Atomic League", GameFormat: "8ball", DayOfWeek: "Monday",
	}, creatorID)
	if err != nil {
		t.Fatalf("CreateLeagueWithLeagueAdminGrant: %v", err)
	}
	if l.ID == 0 {
		t.Fatal("want a non-zero league id")
	}

	var leagueCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM leagues WHERE id = ?`, l.ID).Scan(&leagueCount); err != nil {
		t.Fatalf("count leagues: %v", err)
	}
	if leagueCount != 1 {
		t.Fatalf("want the league to exist, got count=%d", leagueCount)
	}

	assignments, err := roleStore.ListForUser(ctx, creatorID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	found := false
	for _, a := range assignments {
		if a.RoleCode == auth.RoleLeagueAdmin && a.LeagueID != nil && *a.LeagueID == l.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("want creator granted league_admin for the new league, got assignments=%+v", assignments)
	}
}

// TestLeagueSelfGrantStore_ForcedGrantFailureRollsBackLeague proves the
// transaction is truly atomic: when the role_assignments insert fails (here,
// forced deterministically via a creatorUserID that does not exist in
// users, which violates role_assignments' own foreign key), the
// just-inserted league row is rolled back too -- no orphaned league is left
// behind with zero admins able to manage it.
func TestLeagueSelfGrantStore_ForcedGrantFailureRollsBackLeague(t *testing.T) {
	newAuthTestDB(t)
	ctx := context.Background()
	store := sqlite.NewLeagueSelfGrantStore(db.DB)

	const nonexistentUserID = 999999
	const leagueName = "Should Not Persist League"

	_, err := store.CreateLeagueWithLeagueAdminGrant(ctx, leagues.CreateLeagueInput{
		Name: leagueName, GameFormat: "8ball", DayOfWeek: "Tuesday",
	}, nonexistentUserID)
	if err == nil {
		t.Fatal("want an error when the grant insert violates a foreign key constraint")
	}

	var leagueCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM leagues WHERE name = ?`, leagueName).Scan(&leagueCount); err != nil {
		t.Fatalf("count leagues: %v", err)
	}
	if leagueCount != 0 {
		t.Errorf("want the league insert rolled back on grant failure, but found %d row(s)", leagueCount)
	}

	var assignmentCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM role_assignments WHERE user_id = ?`, nonexistentUserID).Scan(&assignmentCount); err != nil {
		t.Fatalf("count role_assignments: %v", err)
	}
	if assignmentCount != 0 {
		t.Errorf("want no role_assignments row left behind, got %d", assignmentCount)
	}
}

// TestLeagueSelfGrantStore_OtherLeagueAdminsNotAutoGranted proves the
// atomic self-grant only ever grants the creator -- an unrelated existing
// league_admin (for a different league) gets no automatic access to the
// newly created league.
func TestLeagueSelfGrantStore_OtherLeagueAdminsNotAutoGranted(t *testing.T) {
	newAuthTestDB(t)
	ctx := context.Background()
	store := sqlite.NewLeagueSelfGrantStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)

	creatorID := seedAuthUser(t, "self-grant-creator-2")
	otherAdminID := seedAuthUser(t, "unrelated-league-admin")
	otherLeagueID := seedAuthLeague(t, "Other Pre-Existing League")
	if err := roleStore.Grant(ctx, otherAdminID, auth.RoleLeagueAdmin, &otherLeagueID, nil); err != nil {
		t.Fatalf("seed other admin's grant: %v", err)
	}

	l, err := store.CreateLeagueWithLeagueAdminGrant(ctx, leagues.CreateLeagueInput{
		Name: "New League No Auto Grant", GameFormat: "8ball", DayOfWeek: "Wednesday",
	}, creatorID)
	if err != nil {
		t.Fatalf("CreateLeagueWithLeagueAdminGrant: %v", err)
	}

	assignments, err := roleStore.ListForUser(ctx, otherAdminID)
	if err != nil {
		t.Fatalf("ListForUser(otherAdminID): %v", err)
	}
	for _, a := range assignments {
		if a.LeagueID != nil && *a.LeagueID == l.ID {
			t.Errorf("want the unrelated league_admin to receive no automatic grant for the new league, got %+v", assignments)
		}
	}
}
