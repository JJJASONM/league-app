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

// ─── CreateApplyUser ──────────────────────────────────────────────────────────

func TestApplyAuthStore_Create_ReturnsUser(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	u, key, err := store.CreateApplyUser(ctx, "alice")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}
	if u.ID == 0 {
		t.Error("want non-zero user ID")
	}
	if u.Username != "alice" {
		t.Errorf("want username alice, got %q", u.Username)
	}
	if u.Role != "admin" {
		t.Errorf("want role admin, got %q", u.Role)
	}
	if !u.Active {
		t.Error("want active=true")
	}
	if len(key) != 64 {
		t.Errorf("want 64-char hex key, got len=%d", len(key))
	}
}

func TestApplyAuthStore_Create_DuplicateUsername_Errors(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	if _, _, err := store.CreateApplyUser(ctx, "bob"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, _, err := store.CreateApplyUser(ctx, "bob"); err == nil {
		t.Error("want error on duplicate username, got nil")
	}
}

func TestApplyAuthStore_Create_KeysAreUnique(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	_, k1, _ := store.CreateApplyUser(ctx, "user1")
	_, k2, _ := store.CreateApplyUser(ctx, "user2")
	if k1 == k2 {
		t.Error("want distinct keys for distinct users")
	}
}

// ─── ResolveApplyUserByAPIKey ─────────────────────────────────────────────────

func TestApplyAuthStore_Resolve_MatchesCreatedKey(t *testing.T) {
	store := newApplyAuthStore(t)
	ctx := context.Background()

	created, key, err := store.CreateApplyUser(ctx, "carol")
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

	if _, _, err := store.CreateApplyUser(ctx, "dave"); err != nil {
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

	_, key, err := store.CreateApplyUser(ctx, "eve")
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
		if _, _, err := store.CreateApplyUser(ctx, name); err != nil {
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

	_, key, _ := store.CreateApplyUser(ctx, "iris")
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
