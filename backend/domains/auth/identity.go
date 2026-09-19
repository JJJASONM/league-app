// Package auth implements Users/Roles Phase 1: email+password identity,
// server-managed sessions, scoped role assignments, and the single
// centralized authorization policy entry point (Authorize). It is deliberately
// separate from the existing handicap-Apply personal-API-key mechanism
// (backend/storage/sqlite/apply_auth_store.go), which remains a compatible,
// continuing bootstrap/automation path per PM decision, not something this
// package replaces.
package auth

// RoleCode identifies a grantable scoped role. "player" is deliberately not
// a RoleCode here -- player identity comes exclusively from a user's
// player_id link, never from a role_assignments row (see
// db/db.go's role_assignments CHECK constraint, which enforces this at the
// database layer too).
type RoleCode string

const (
	RoleSystemAdmin RoleCode = "system_admin"
	RoleLeagueAdmin RoleCode = "league_admin"
)

// Assignment is one scoped role grant. LeagueID is always nil for
// RoleSystemAdmin (global) and always non-nil for RoleLeagueAdmin (scoped
// to exactly one league) -- mirroring the role_assignments CHECK constraint.
type Assignment struct {
	RoleCode RoleCode
	LeagueID *int64
}

// Identity is the full, resolved set of facts about who is making a
// request: their account, their linked player (if any), and every scoped
// role they hold. It is built once per request (at login, or when a
// session/API key resolves) and is the only input Authorize needs beyond
// the action and its target scope.
type Identity struct {
	UserID      int64
	Username    string
	Email       string
	Active      bool
	PlayerID    *int64
	PlayerName  string
	Assignments []Assignment
}

// IsSystemAdmin reports whether this identity holds the global system_admin
// role.
func (id Identity) IsSystemAdmin() bool {
	for _, a := range id.Assignments {
		if a.RoleCode == RoleSystemAdmin {
			return true
		}
	}
	return false
}

// IsLeagueAdminFor reports whether this identity holds league_admin scoped
// to exactly leagueID.
func (id Identity) IsLeagueAdminFor(leagueID int64) bool {
	for _, a := range id.Assignments {
		if a.RoleCode == RoleLeagueAdmin && a.LeagueID != nil && *a.LeagueID == leagueID {
			return true
		}
	}
	return false
}

// IsAnyLeagueAdmin reports whether this identity holds league_admin for at
// least one league, regardless of which. Used only to decide eligibility to
// create a NEW league (a brand-new league_admin's very first scope must
// come from a system_admin grant against an existing league; from then on,
// being a league_admin anywhere is what unlocks creating additional
// leagues on one's own -- see Authorize's ActionLeagueCreate case).
func (id Identity) IsAnyLeagueAdmin() bool {
	for _, a := range id.Assignments {
		if a.RoleCode == RoleLeagueAdmin {
			return true
		}
	}
	return false
}

// LeagueIDs returns every league this identity is league_admin for.
func (id Identity) LeagueIDs() []int64 {
	var out []int64
	for _, a := range id.Assignments {
		if a.RoleCode == RoleLeagueAdmin && a.LeagueID != nil {
			out = append(out, *a.LeagueID)
		}
	}
	return out
}

// IsPlayer reports whether this identity has a linked player record.
func (id Identity) IsPlayer() bool {
	return id.PlayerID != nil
}

// Workspaces lists the presentation-only workspaces available to this
// identity, for the frontend's Player View / Admin View chooser. This has
// no bearing on Authorize's outcome -- permissions are always the full
// Identity regardless of which workspace is currently selected client-side.
func (id Identity) Workspaces() []string {
	var out []string
	if id.IsSystemAdmin() || id.IsAnyLeagueAdmin() {
		out = append(out, "admin")
	}
	if id.IsPlayer() {
		out = append(out, "player")
	}
	return out
}
