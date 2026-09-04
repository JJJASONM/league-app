package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// newApplyAuthStore initialises a fresh DB in a temp dir and returns an
// ApplyAuthStore backed by it. db.DB is left open for the test.
func newApplyAuthStore(t *testing.T) *sqlite.ApplyAuthStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewApplyAuthStore(db.DB)
}

// seedPlayerForUser inserts a minimal player row and returns its id, for
// tests linking a role=player user to a real player.
func seedPlayerForUser(t *testing.T, first, last string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, handicap) VALUES (?,?,0)`, first, last)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ─── CreateApplyUser ──────────────────────────────────────────────────────────

func TestApplyAuthStore_Create_ReturnsUser(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	u, key, err := store.CreateApplyUser(ctx, "alice", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}
	if u.ID == 0 {
		t.Error("want non-zero user ID")
	}
	if u.Username != "alice" {
		t.Errorf("want username alice, got %q", u.Username)
	}
	if u.Role != "league_admin" {
		t.Errorf("want role league_admin, got %q", u.Role)
	}
	if !u.Active {
		t.Error("want active=true")
	}
	if len(key) != 64 {
		t.Errorf("want 64-char hex key, got len=%d", len(key))
	}
}

// TestApplyAuthStore_Create_PersistsGivenRole verifies the store persists
// whatever role it is given (Users Admin Screen Phase 1: role is no longer
// hardcoded to "admin") for both roles the handler is allowed to assign.
func TestApplyAuthStore_Create_PersistsGivenRole(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	sysAdmin, _, err := store.CreateApplyUser(ctx, "sys-admin-user", "system_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser(system_admin): %v", err)
	}
	if sysAdmin.Role != "system_admin" {
		t.Errorf("want role system_admin, got %q", sysAdmin.Role)
	}

	leagueAdmin, _, err := store.CreateApplyUser(ctx, "league-admin-user", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser(league_admin): %v", err)
	}
	if leagueAdmin.Role != "league_admin" {
		t.Errorf("want role league_admin, got %q", leagueAdmin.Role)
	}
}

func TestApplyAuthStore_Create_DuplicateUsername_Errors(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	if _, _, err := store.CreateApplyUser(ctx, "bob", "league_admin"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, _, err := store.CreateApplyUser(ctx, "bob", "league_admin"); err == nil {
		t.Error("want error on duplicate username, got nil")
	}
}

func TestApplyAuthStore_Create_KeysAreUnique(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	_, k1, _ := store.CreateApplyUser(ctx, "user1", "league_admin")
	_, k2, _ := store.CreateApplyUser(ctx, "user2", "league_admin")
	if k1 == k2 {
		t.Error("want distinct keys for distinct users")
	}
}

// ─── ResolveApplyUserByAPIKey ─────────────────────────────────────────────────

// --- CreateApplyPlayerUser (Player Account Access Phase 1) -----------------

func TestApplyAuthStore_CreateApplyPlayerUser_ReturnsLinkedUser(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()
	playerID := seedPlayerForUser(t, "Sam", "Player")

	u, key, err := store.CreateApplyPlayerUser(ctx, "sam-player", playerID)
	if err != nil {
		t.Fatalf("CreateApplyPlayerUser: %v", err)
	}
	if u.Role != "player" {
		t.Errorf("want role=player, got %q", u.Role)
	}
	if u.PlayerID == nil || *u.PlayerID != playerID {
		t.Errorf("want player_id=%d, got %v", playerID, u.PlayerID)
	}
	if len(key) != 64 {
		t.Errorf("want 64-char hex key, got len=%d", len(key))
	}
}

func TestApplyAuthStore_Resolve_LinkedPlayerUser_ReturnsPlayerID(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()
	playerID := seedPlayerForUser(t, "Resolved", "Player")

	_, key, err := store.CreateApplyPlayerUser(ctx, "resolved-player", playerID)
	if err != nil {
		t.Fatalf("CreateApplyPlayerUser: %v", err)
	}

	resolved, err := store.ResolveApplyUserByAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("ResolveApplyUserByAPIKey: %v", err)
	}
	if resolved == nil {
		t.Fatal("want non-nil user, got nil")
	}
	if resolved.PlayerID == nil || *resolved.PlayerID != playerID {
		t.Errorf("want player_id=%d resolved from the key, got %v", playerID, resolved.PlayerID)
	}
}

// TestApplyAuthStore_Resolve_AdminUser_HasNilPlayerID confirms the existing
// admin-role creation path leaves player_id NULL -- the new column must not
// change behavior for every pre-existing user.
func TestApplyAuthStore_Resolve_AdminUser_HasNilPlayerID(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	_, key, err := store.CreateApplyUser(ctx, "plain-admin", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	resolved, err := store.ResolveApplyUserByAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("ResolveApplyUserByAPIKey: %v", err)
	}
	if resolved.PlayerID != nil {
		t.Errorf("want nil player_id for an admin-role user, got %v", resolved.PlayerID)
	}
}

func TestApplyAuthStore_List_ShowsLinkedPlayerName(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()
	playerID := seedPlayerForUser(t, "Listed", "Player")

	if _, _, err := store.CreateApplyPlayerUser(ctx, "listed-player", playerID); err != nil {
		t.Fatalf("CreateApplyPlayerUser: %v", err)
	}
	if _, _, err := store.CreateApplyUser(ctx, "listed-admin", "league_admin"); err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	users, err := store.ListApplyUsers(ctx)
	if err != nil {
		t.Fatalf("ListApplyUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("want 2 users, got %d", len(users))
	}
	for _, u := range users {
		switch u.Username {
		case "listed-player":
			if u.PlayerName != "Listed Player" {
				t.Errorf("want player_name=%q for the linked user, got %q", "Listed Player", u.PlayerName)
			}
			if u.PlayerID == nil || *u.PlayerID != playerID {
				t.Errorf("want player_id=%d, got %v", playerID, u.PlayerID)
			}
		case "listed-admin":
			if u.PlayerName != "" {
				t.Errorf("want empty player_name for an admin-role user, got %q", u.PlayerName)
			}
			if u.PlayerID != nil {
				t.Errorf("want nil player_id for an admin-role user, got %v", u.PlayerID)
			}
		}
	}
}

func TestApplyAuthStore_Resolve_MatchesCreatedKey(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	created, key, err := store.CreateApplyUser(ctx, "carol", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	resolved, err := store.ResolveApplyUserByAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("ResolveApplyUserByAPIKey: %v", err)
	}
	if resolved == nil {
		t.Fatal("want non-nil user, got nil")
	}
	if resolved.ID != created.ID {
		t.Errorf("want id=%d, got %d", created.ID, resolved.ID)
	}
	if resolved.Username != "carol" {
		t.Errorf("want username carol, got %q", resolved.Username)
	}
}

func TestApplyAuthStore_Resolve_WrongKey_ReturnsNil(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	if _, _, err := store.CreateApplyUser(ctx, "dave", "league_admin"); err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	got, err := store.ResolveApplyUserByAPIKey(ctx, "not-the-right-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for wrong key, got user %+v", got)
	}
}

func TestApplyAuthStore_Resolve_InactiveUser_ReturnsNil(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	_, key, err := store.CreateApplyUser(ctx, "eve", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}

	// Deactivate the user directly.
	if _, err := db.DB.ExecContext(ctx, `UPDATE users SET active=0 WHERE username='eve'`); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	got, err := store.ResolveApplyUserByAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for inactive user, got %+v", got)
	}
}

// ─── ListApplyUsers ───────────────────────────────────────────────────────────

func TestApplyAuthStore_List_ReturnsAllUsers(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	for _, name := range []string{"frank", "grace", "henry"} {
		if _, _, err := store.CreateApplyUser(ctx, name, "league_admin"); err != nil {
			t.Fatalf("CreateApplyUser(%q): %v", name, err)
		}
	}

	users, err := store.ListApplyUsers(ctx)
	if err != nil {
		t.Fatalf("ListApplyUsers: %v", err)
	}
	if len(users) != 3 {
		t.Errorf("want 3 users, got %d", len(users))
	}
}

func TestApplyAuthStore_List_DoesNotExposeHash(t *testing.T) {
	// The API key hash must never appear in the JSON-serialisable User struct.
	// Verify by ensuring no field on User holds a 64-char hex string after listing.
	store := newApplyAuthStore(t)
	ctx := context.Background()

	_, key, _ := store.CreateApplyUser(ctx, "iris", "league_admin")
	users, err := store.ListApplyUsers(ctx)
	if err != nil {
		t.Fatalf("ListApplyUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("want 1 user, got %d", len(users))
	}
	u := users[0]
	// None of the exported string fields should equal the key or its hash.
	for _, field := range []string{u.Username, u.Role, u.CreatedAt} {
		if field == key {
			t.Error("want no field equal to cleartext key")
		}
		if len(field) == 64 {
			t.Errorf("want no 64-char field (possible hash leak), got %q", field)
		}
	}
}

func TestApplyAuthStore_List_EmptyDB_ReturnsNilSlice(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	users, err := store.ListApplyUsers(ctx)
	if err != nil {
		t.Fatalf("ListApplyUsers: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("want 0 users on empty db, got %d", len(users))
	}
}
