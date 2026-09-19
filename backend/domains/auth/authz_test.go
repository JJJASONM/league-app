package auth_test

import (
	"testing"

	"league_app/backend/domains/auth"
)

func ptr(v int64) *int64 { return &v }

func systemAdmin() auth.Identity {
	return auth.Identity{UserID: 1, Active: true, Assignments: []auth.Assignment{{RoleCode: auth.RoleSystemAdmin}}}
}

func leagueAdmin(leagueID int64) auth.Identity {
	return auth.Identity{UserID: 2, Active: true, Assignments: []auth.Assignment{{RoleCode: auth.RoleLeagueAdmin, LeagueID: ptr(leagueID)}}}
}

func plainPlayer(playerID int64) auth.Identity {
	pid := playerID
	return auth.Identity{UserID: 3, Active: true, PlayerID: &pid}
}

func TestAuthorize_SystemAdminAllowedEverywhere(t *testing.T) {
	id := systemAdmin()
	actions := []auth.Action{
		auth.ActionSystemUserAdmin, auth.ActionBackup, auth.ActionLeagueCreate,
		auth.ActionLeagueAdminister, auth.ActionSeasonSetup, auth.ActionScheduleMutate,
		auth.ActionMatchScoreMutate, auth.ActionMatchApproval, auth.ActionWeekCloseReopen,
		auth.ActionFinanceRead, auth.ActionFinanceWrite, auth.ActionHandicapApply,
	}
	for _, a := range actions {
		if !auth.Authorize(id, a, auth.Scope{LeagueID: ptr(999)}) {
			t.Errorf("want system_admin authorized for %s", a)
		}
	}
	if !auth.Authorize(id, auth.ActionPlayerOverviewOther, auth.Scope{LeagueID: ptr(999)}) {
		t.Error("want system_admin authorized for player_overview_other")
	}
}

func TestAuthorize_InactiveIdentityNeverAuthorized(t *testing.T) {
	id := systemAdmin()
	id.Active = false
	if auth.Authorize(id, auth.ActionBackup, auth.Scope{}) {
		t.Error("want inactive identity denied even for system_admin actions")
	}
}

func TestAuthorize_LeagueAdminDeniedSystemUserAdminAndBackup(t *testing.T) {
	id := leagueAdmin(10)
	if auth.Authorize(id, auth.ActionSystemUserAdmin, auth.Scope{}) {
		t.Error("want league_admin denied system_user_admin")
	}
	if auth.Authorize(id, auth.ActionBackup, auth.Scope{}) {
		t.Error("want league_admin denied backup")
	}
}

func TestAuthorize_LeagueAdminAllowedOwnLeagueOnly(t *testing.T) {
	id := leagueAdmin(10)
	scopedActions := []auth.Action{
		auth.ActionLeagueAdminister, auth.ActionSeasonSetup, auth.ActionScheduleMutate,
		auth.ActionMatchScoreMutate, auth.ActionMatchApproval, auth.ActionWeekCloseReopen,
		auth.ActionFinanceRead, auth.ActionFinanceWrite, auth.ActionHandicapApply,
	}
	for _, a := range scopedActions {
		if !auth.Authorize(id, a, auth.Scope{LeagueID: ptr(10)}) {
			t.Errorf("want league_admin allowed %s for their own league", a)
		}
		if auth.Authorize(id, a, auth.Scope{LeagueID: ptr(20)}) {
			t.Errorf("want league_admin denied %s for a different league", a)
		}
		if auth.Authorize(id, a, auth.Scope{}) {
			t.Errorf("want league_admin denied %s with no league scope resolved", a)
		}
	}
}

func TestAuthorize_LeagueCreate_RequiresBeingALeagueAdminSomewhereOrSystemAdmin(t *testing.T) {
	if !auth.Authorize(leagueAdmin(10), auth.ActionLeagueCreate, auth.Scope{}) {
		t.Error("want an existing league_admin (any league) allowed to create a new league")
	}
	if !auth.Authorize(systemAdmin(), auth.ActionLeagueCreate, auth.Scope{}) {
		t.Error("want system_admin allowed to create a league")
	}
	if auth.Authorize(plainPlayer(1), auth.ActionLeagueCreate, auth.Scope{}) {
		t.Error("want a plain player (no league_admin anywhere) denied league creation")
	}
}

func TestAuthorize_PlayerOverviewOwn(t *testing.T) {
	id := plainPlayer(42)
	if !auth.Authorize(id, auth.ActionPlayerOverviewOwn, auth.Scope{PlayerID: ptr(42)}) {
		t.Error("want a player allowed to view their own overview")
	}
	if auth.Authorize(id, auth.ActionPlayerOverviewOwn, auth.Scope{PlayerID: ptr(43)}) {
		t.Error("want a player denied another player's overview via the 'own' action")
	}
}

func TestAuthorize_PlayerOverviewOther_RequiresLeagueAdminOrSystemAdmin(t *testing.T) {
	if !auth.Authorize(leagueAdmin(10), auth.ActionPlayerOverviewOther, auth.Scope{LeagueID: ptr(10)}) {
		t.Error("want league_admin allowed to view a player's overview in their own league")
	}
	if auth.Authorize(leagueAdmin(10), auth.ActionPlayerOverviewOther, auth.Scope{LeagueID: ptr(20)}) {
		t.Error("want league_admin denied a player's overview in a different league")
	}
	if auth.Authorize(plainPlayer(1), auth.ActionPlayerOverviewOther, auth.Scope{LeagueID: ptr(10)}) {
		t.Error("want a plain player denied viewing another player's overview")
	}
}

func TestAuthorize_DualRoleUnionsPermissions(t *testing.T) {
	// A dual-role identity (league_admin for league 10, also linked to
	// player 42) must be authorized for BOTH kinds of action regardless of
	// which "workspace" a frontend might currently be showing -- Authorize
	// has no concept of workspace at all, which is the point.
	dual := auth.Identity{
		UserID:      4,
		Active:      true,
		PlayerID:    ptr(42),
		Assignments: []auth.Assignment{{RoleCode: auth.RoleLeagueAdmin, LeagueID: ptr(10)}},
	}
	if !auth.Authorize(dual, auth.ActionSeasonSetup, auth.Scope{LeagueID: ptr(10)}) {
		t.Error("want dual-role identity authorized for its league_admin capability")
	}
	if !auth.Authorize(dual, auth.ActionPlayerOverviewOwn, auth.Scope{PlayerID: ptr(42)}) {
		t.Error("want dual-role identity authorized for its player capability")
	}
}
