package sqlite_test

import (
	"context"
	"testing"
	"time"

	"league_app/backend/domains/auth"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

func newAuthTestDB(t *testing.T) {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
}

func seedAuthUser(t *testing.T, username string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO users (username, active) VALUES (?, 1)`, username)
	if err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedAuthLeague(t *testing.T, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO leagues (name) VALUES (?)`, name)
	if err != nil {
		t.Fatalf("seed league %s: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// --- AuthUserStore -----------------------------------------------------

func TestAuthUserStore_ProvisionAndGetByEmail(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewAuthUserStore(db.DB)
	ctx := context.Background()

	u, err := store.ProvisionUser(ctx, "auto-alice-1", "alice@example.com", nil)
	if err != nil {
		t.Fatalf("ProvisionUser: %v", err)
	}
	if u.Email == nil || *u.Email != "alice@example.com" {
		t.Fatalf("want email set, got %+v", u)
	}

	got, err := store.GetByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got == nil || got.ID != u.ID {
		t.Fatalf("want to resolve the provisioned user by email, got %+v", got)
	}
	if got.PasswordHash != nil {
		t.Error("want no password set yet for a freshly provisioned user")
	}
}

func TestAuthUserStore_SetPasswordAndSetActive(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewAuthUserStore(db.DB)
	ctx := context.Background()
	id := seedAuthUser(t, "bob")

	if err := store.SetPassword(ctx, id, "$argon2id$fake$hash$for$test"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	u, err := store.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if u.PasswordHash == nil || *u.PasswordHash != "$argon2id$fake$hash$for$test" {
		t.Errorf("want stored password hash, got %+v", u.PasswordHash)
	}

	if err := store.SetActive(ctx, id, false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	u, _ = store.GetByID(ctx, id)
	if u.Active {
		t.Error("want active=false after SetActive(false)")
	}
}

// --- RoleAssignmentStore -------------------------------------------------

func TestRoleAssignmentStore_GrantListRevoke(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewRoleAssignmentStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "carol")
	leagueID := seedAuthLeague(t, "League A")

	if err := store.Grant(ctx, userID, auth.RoleLeagueAdmin, &leagueID, nil); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	assignments, err := store.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(assignments) != 1 || assignments[0].RoleCode != auth.RoleLeagueAdmin || assignments[0].LeagueID == nil || *assignments[0].LeagueID != leagueID {
		t.Fatalf("want 1 league_admin assignment for league %d, got %+v", leagueID, assignments)
	}

	if err := store.Revoke(ctx, userID, auth.RoleLeagueAdmin, &leagueID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	assignments, _ = store.ListForUser(ctx, userID)
	if len(assignments) != 0 {
		t.Errorf("want 0 assignments after revoke, got %d", len(assignments))
	}
}

func TestRoleAssignmentStore_CHECKConstraintRejectsInvalidCombination(t *testing.T) {
	newAuthTestDB(t)
	userID := seedAuthUser(t, "dave")
	leagueID := seedAuthLeague(t, "League B")

	// system_admin with a non-NULL league_id must be rejected by the
	// table's own CHECK constraint -- a direct raw INSERT, not the store's
	// Grant method, to prove the database itself enforces this, not just
	// application code.
	_, err := db.DB.Exec(`INSERT INTO role_assignments (user_id, role_code, league_id) VALUES (?, 'system_admin', ?)`, userID, leagueID)
	if err == nil {
		t.Fatal("want the database to reject system_admin with a non-NULL league_id")
	}

	// league_admin with a NULL league_id must also be rejected.
	_, err = db.DB.Exec(`INSERT INTO role_assignments (user_id, role_code, league_id) VALUES (?, 'league_admin', NULL)`, userID)
	if err == nil {
		t.Fatal("want the database to reject league_admin with a NULL league_id")
	}

	// role_code='player' must never be insertable at all.
	_, err = db.DB.Exec(`INSERT INTO role_assignments (user_id, role_code, league_id) VALUES (?, 'player', NULL)`, userID)
	if err == nil {
		t.Fatal("want the database to reject role_code='player' in role_assignments")
	}
}

func TestRoleAssignmentStore_DuplicateAssignmentRejected(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewRoleAssignmentStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "erin")
	leagueID := seedAuthLeague(t, "League C")

	if err := store.Grant(ctx, userID, auth.RoleLeagueAdmin, &leagueID, nil); err != nil {
		t.Fatalf("first Grant: %v", err)
	}
	if err := store.Grant(ctx, userID, auth.RoleLeagueAdmin, &leagueID, nil); err == nil {
		t.Error("want a duplicate (user, league_admin, league) grant to be rejected")
	}

	// A second system_admin grant for the same user must also be rejected
	// (the partial unique index on user_id alone for that role_code).
	if err := store.Grant(ctx, userID, auth.RoleSystemAdmin, nil, nil); err != nil {
		t.Fatalf("first system_admin grant: %v", err)
	}
	if err := store.Grant(ctx, userID, auth.RoleSystemAdmin, nil, nil); err == nil {
		t.Error("want a duplicate system_admin grant for the same user to be rejected")
	}
}

// TestRoleAssignmentStore_LeagueDeletionCascadesAssignments follows the
// exact pattern proven necessary in db/cascade_delete_test.go: without
// forcing a non-init connection, this test would still pass against a
// version of db.go that only configured PRAGMA foreign_keys on the
// startup connection, giving false confidence. See that file's
// forceNonInitConnection doc comment for the full empirical justification.
func TestRoleAssignmentStore_LeagueDeletionCascadesAssignments(t *testing.T) {
	newAuthTestDB(t)
	conn, err := db.DB.Conn(context.Background())
	if err != nil {
		t.Fatalf("checkout non-init connection: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	store := sqlite.NewRoleAssignmentStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "frank")
	leagueID := seedAuthLeague(t, "League D")

	if err := store.Grant(ctx, userID, auth.RoleLeagueAdmin, &leagueID, nil); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	if _, err := db.DB.Exec(`DELETE FROM leagues WHERE id = ?`, leagueID); err != nil {
		t.Fatalf("delete league: %v", err)
	}

	assignments, err := store.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(assignments) != 0 {
		t.Errorf("want role_assignments cascade-deleted with its league, got %+v", assignments)
	}
}

// --- SessionStore --------------------------------------------------------

func TestSessionStore_CreateGetTouchRevoke(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewSessionStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "grace")

	now := time.Now().UTC().Truncate(time.Second)
	created, err := store.Create(ctx, auth.Session{
		UserID: userID, TokenHash: "tok-hash-1", CSRFTokenHash: "csrf-hash-1",
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
		UserAgent: "test-agent", IPAddress: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("want a non-zero session id")
	}

	got, err := store.GetByTokenHash(ctx, "tok-hash-1")
	if err != nil {
		t.Fatalf("GetByTokenHash: %v", err)
	}
	if got == nil || got.UserID != userID || got.CSRFTokenHash != "csrf-hash-1" {
		t.Fatalf("want matching session, got %+v", got)
	}
	if got.RevokedAt != nil {
		t.Error("want RevokedAt nil for a fresh session")
	}
	if !got.ExpiresAt.After(now) {
		t.Errorf("want ExpiresAt after creation time, got %v vs %v", got.ExpiresAt, now)
	}

	newExpiry := now.Add(14 * 24 * time.Hour)
	if err := store.Touch(ctx, created.ID, now.Add(time.Hour), newExpiry); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, _ = store.GetByTokenHash(ctx, "tok-hash-1")
	if !got.ExpiresAt.Equal(newExpiry) {
		t.Errorf("want ExpiresAt updated to %v, got %v", newExpiry, got.ExpiresAt)
	}

	if err := store.Revoke(ctx, created.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, _ = store.GetByTokenHash(ctx, "tok-hash-1")
	if got.RevokedAt == nil {
		t.Error("want RevokedAt set after Revoke")
	}
}

func TestSessionStore_RevokeAllForUserAndDeleteExpired(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewSessionStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "henry")
	now := time.Now().UTC()

	if _, err := store.Create(ctx, auth.Session{UserID: userID, TokenHash: "h1", CSRFTokenHash: "c1", CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if _, err := store.Create(ctx, auth.Session{UserID: userID, TokenHash: "h2", CSRFTokenHash: "c2", CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(-time.Hour)}); err != nil {
		t.Fatalf("Create 2 (already-expired): %v", err)
	}

	if err := store.RevokeAllForUser(ctx, userID); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	s1, _ := store.GetByTokenHash(ctx, "h1")
	if s1.RevokedAt == nil {
		t.Error("want session 1 revoked")
	}

	if err := store.DeleteExpiredForUser(ctx, userID); err != nil {
		t.Fatalf("DeleteExpiredForUser: %v", err)
	}
	s1After, _ := store.GetByTokenHash(ctx, "h1")
	if s1After != nil {
		t.Error("want revoked session removed by DeleteExpiredForUser")
	}
}

// --- PasswordSetupTokenStore ---------------------------------------------

func TestPasswordSetupTokenStore_ConsumeAndSetPassword(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewPasswordSetupTokenStore(db.DB)
	userStore := sqlite.NewAuthUserStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "ivan")

	if err := store.Create(ctx, userID, "setup-hash-1", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotUserID, ok, err := store.ConsumeAndSetPassword(ctx, "setup-hash-1", "$argon2id$new$hash")
	if err != nil {
		t.Fatalf("ConsumeAndSetPassword: %v", err)
	}
	if !ok || gotUserID != userID {
		t.Fatalf("want ok=true userID=%d, got ok=%v userID=%d", userID, ok, gotUserID)
	}

	u, _ := userStore.GetByID(ctx, userID)
	if u.PasswordHash == nil || *u.PasswordHash != "$argon2id$new$hash" {
		t.Errorf("want password hash set by the atomic consume, got %+v", u.PasswordHash)
	}

	// Single-use.
	_, ok, err = store.ConsumeAndSetPassword(ctx, "setup-hash-1", "$argon2id$another$hash")
	if err != nil {
		t.Fatalf("second ConsumeAndSetPassword: %v", err)
	}
	if ok {
		t.Error("want second consumption of the same token to fail")
	}
}

func TestPasswordSetupTokenStore_ExpiredTokenRejected(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewPasswordSetupTokenStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "judy")

	if err := store.Create(ctx, userID, "expired-hash", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, ok, err := store.ConsumeAndSetPassword(ctx, "expired-hash", "$argon2id$x")
	if err != nil {
		t.Fatalf("ConsumeAndSetPassword: %v", err)
	}
	if ok {
		t.Error("want an expired token to be rejected")
	}
}

func TestPasswordSetupTokenStore_RevokeActiveForUser(t *testing.T) {
	newAuthTestDB(t)
	store := sqlite.NewPasswordSetupTokenStore(db.DB)
	ctx := context.Background()
	userID := seedAuthUser(t, "karl")

	if err := store.Create(ctx, userID, "will-be-revoked", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.RevokeActiveForUser(ctx, userID); err != nil {
		t.Fatalf("RevokeActiveForUser: %v", err)
	}
	_, ok, err := store.ConsumeAndSetPassword(ctx, "will-be-revoked", "$argon2id$x")
	if err != nil {
		t.Fatalf("ConsumeAndSetPassword: %v", err)
	}
	if ok {
		t.Error("want a revoked token to be rejected")
	}
}

// --- UserAPIKeyAdminStore -------------------------------------------------

func TestUserAPIKeyAdminStore_RevokeAllForUser(t *testing.T) {
	newAuthTestDB(t)
	applyStore := sqlite.NewApplyAuthStore(db.DB)
	adminStore := sqlite.NewUserAPIKeyAdminStore(db.DB)
	ctx := context.Background()

	_, cleartext, err := applyStore.CreateApplyUser(ctx, "leo", "league_admin")
	if err != nil {
		t.Fatalf("CreateApplyUser: %v", err)
	}
	if u, err := applyStore.ResolveApplyUserByAPIKey(ctx, cleartext); err != nil || u == nil {
		t.Fatalf("expected key to resolve before revocation: user=%v err=%v", u, err)
	}

	var userID int64
	if err := db.DB.QueryRow(`SELECT id FROM users WHERE username='leo'`).Scan(&userID); err != nil {
		t.Fatalf("lookup user id: %v", err)
	}
	if err := adminStore.RevokeAllForUser(ctx, userID); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}

	u, err := applyStore.ResolveApplyUserByAPIKey(ctx, cleartext)
	if err != nil {
		t.Fatalf("ResolveApplyUserByAPIKey after revocation: %v", err)
	}
	if u != nil {
		t.Error("want the key to no longer resolve after revocation")
	}
}
