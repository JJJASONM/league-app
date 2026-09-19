package auth_test

import (
	"context"
	"testing"
	"time"

	"league_app/backend/domains/auth"
)

// --- in-memory fakes -------------------------------------------------------

type fakeUserStore struct {
	byID    map[int64]*auth.UserRecord
	byEmail map[string]int64
	nextID  int64
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{byID: map[int64]*auth.UserRecord{}, byEmail: map[string]int64{}, nextID: 1}
}

func (f *fakeUserStore) add(email string, passwordHash *string, active bool) *auth.UserRecord {
	id := f.nextID
	f.nextID++
	u := &auth.UserRecord{ID: id, Username: email, Email: &email, PasswordHash: passwordHash, Active: active}
	f.byID[id] = u
	f.byEmail[email] = id
	return u
}

func (f *fakeUserStore) GetByEmail(_ context.Context, normalizedEmail string) (*auth.UserRecord, error) {
	id, ok := f.byEmail[normalizedEmail]
	if !ok {
		return nil, nil
	}
	return f.byID[id], nil
}
func (f *fakeUserStore) GetByID(_ context.Context, id int64) (*auth.UserRecord, error) {
	return f.byID[id], nil
}
func (f *fakeUserStore) SetPassword(_ context.Context, userID int64, encodedHash string) error {
	f.byID[userID].PasswordHash = &encodedHash
	return nil
}
func (f *fakeUserStore) SetActive(_ context.Context, userID int64, active bool) error {
	f.byID[userID].Active = active
	return nil
}
func (f *fakeUserStore) ProvisionUser(_ context.Context, username, normalizedEmail string, playerID *int64) (auth.UserRecord, error) {
	u := f.add(normalizedEmail, nil, true)
	u.PlayerID = playerID
	return *u, nil
}

type fakeRoleStore struct{ assignments map[int64][]auth.Assignment }

func newFakeRoleStore() *fakeRoleStore { return &fakeRoleStore{assignments: map[int64][]auth.Assignment{}} }
func (f *fakeRoleStore) ListForUser(_ context.Context, userID int64) ([]auth.Assignment, error) {
	return f.assignments[userID], nil
}
func (f *fakeRoleStore) Grant(_ context.Context, userID int64, roleCode auth.RoleCode, leagueID *int64, _ *int64) error {
	f.assignments[userID] = append(f.assignments[userID], auth.Assignment{RoleCode: roleCode, LeagueID: leagueID})
	return nil
}
func (f *fakeRoleStore) Revoke(_ context.Context, userID int64, roleCode auth.RoleCode, leagueID *int64) error {
	var kept []auth.Assignment
	for _, a := range f.assignments[userID] {
		if a.RoleCode == roleCode && ((a.LeagueID == nil) == (leagueID == nil)) && (leagueID == nil || *a.LeagueID == *leagueID) {
			continue
		}
		kept = append(kept, a)
	}
	f.assignments[userID] = kept
	return nil
}

type fakeSessionStore struct {
	byHash map[string]*auth.Session
	nextID int64
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{byHash: map[string]*auth.Session{}, nextID: 1}
}
func (f *fakeSessionStore) Create(_ context.Context, s auth.Session) (auth.Session, error) {
	s.ID = f.nextID
	f.nextID++
	cp := s
	f.byHash[s.TokenHash] = &cp
	return s, nil
}
func (f *fakeSessionStore) GetByTokenHash(_ context.Context, tokenHash string) (*auth.Session, error) {
	s, ok := f.byHash[tokenHash]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}
func (f *fakeSessionStore) Touch(_ context.Context, id int64, lastSeenAt, expiresAt time.Time) error {
	for _, s := range f.byHash {
		if s.ID == id {
			s.LastSeenAt, s.ExpiresAt = lastSeenAt, expiresAt
		}
	}
	return nil
}
func (f *fakeSessionStore) Revoke(_ context.Context, id int64) error {
	for _, s := range f.byHash {
		if s.ID == id {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	return nil
}
func (f *fakeSessionStore) RevokeAllForUser(_ context.Context, userID int64) error {
	now := time.Now()
	for _, s := range f.byHash {
		if s.UserID == userID {
			s.RevokedAt = &now
		}
	}
	return nil
}
func (f *fakeSessionStore) DeleteExpiredForUser(_ context.Context, userID int64) error {
	for h, s := range f.byHash {
		if s.UserID == userID && (time.Now().After(s.ExpiresAt) || s.RevokedAt != nil) {
			delete(f.byHash, h)
		}
	}
	return nil
}

type fakeSetupTokenStore struct {
	byHash map[string]*setupTokenRow
	users  *fakeUserStore // mimics a real store's single transaction spanning both tables
}
type setupTokenRow struct {
	userID    int64
	expiresAt time.Time
	used      bool
	revoked   bool
}

func newFakeSetupTokenStore(users *fakeUserStore) *fakeSetupTokenStore {
	return &fakeSetupTokenStore{byHash: map[string]*setupTokenRow{}, users: users}
}
func (f *fakeSetupTokenStore) Create(_ context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	f.byHash[tokenHash] = &setupTokenRow{userID: userID, expiresAt: expiresAt}
	return nil
}
func (f *fakeSetupTokenStore) RevokeActiveForUser(_ context.Context, userID int64) error {
	for _, r := range f.byHash {
		if r.userID == userID && !r.used && !r.revoked {
			r.revoked = true
		}
	}
	return nil
}
func (f *fakeSetupTokenStore) ConsumeAndSetPassword(_ context.Context, tokenHash, encodedPasswordHash string) (int64, bool, error) {
	r, ok := f.byHash[tokenHash]
	if !ok || r.used || r.revoked || time.Now().After(r.expiresAt) {
		return 0, false, nil
	}
	r.used = true
	if u := f.users.byID[r.userID]; u != nil {
		u.PasswordHash = &encodedPasswordHash
	}
	return r.userID, true, nil
}

type fakeAPIKeyStore struct{ revokedFor map[int64]bool }

func newFakeAPIKeyStore() *fakeAPIKeyStore { return &fakeAPIKeyStore{revokedFor: map[int64]bool{}} }
func (f *fakeAPIKeyStore) RevokeAllForUser(_ context.Context, userID int64) error {
	f.revokedFor[userID] = true
	return nil
}

func newTestService() (*auth.SessionService, *fakeUserStore, *fakeSessionStore, *fakeSetupTokenStore) {
	u := newFakeUserStore()
	r := newFakeRoleStore()
	s := newFakeSessionStore()
	p := newFakeSetupTokenStore(u)
	fastParams := auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	return auth.NewSessionService(u, r, s, p, fastParams), u, s, p
}

// --- tests -------------------------------------------------------------

func TestSessionService_Login_Success(t *testing.T) {
	svc, users, _, _ := newTestService()
	hash, _ := auth.HashPassword("hunter2", auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	users.add("alice@example.com", &hash, true)

	res, err := svc.Login(context.Background(), "Alice@Example.com", "hunter2", "ua", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.SessionToken == "" || res.CSRFToken == "" {
		t.Error("want non-empty session and CSRF tokens")
	}
	if res.Identity.Email != "alice@example.com" {
		t.Errorf("want normalized email in identity, got %q", res.Identity.Email)
	}
}

func TestSessionService_Login_WrongPassword(t *testing.T) {
	svc, users, _, _ := newTestService()
	hash, _ := auth.HashPassword("hunter2", auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	users.add("alice@example.com", &hash, true)

	_, err := svc.Login(context.Background(), "alice@example.com", "wrong", "ua", "127.0.0.1")
	if err != auth.ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestSessionService_Login_UnknownEmail(t *testing.T) {
	svc, _, _, _ := newTestService()
	_, err := svc.Login(context.Background(), "nobody@example.com", "whatever", "ua", "127.0.0.1")
	if err != auth.ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials for unknown email (not a distinguishable error), got %v", err)
	}
}

func TestSessionService_Login_NoPasswordSetYet(t *testing.T) {
	svc, users, _, _ := newTestService()
	users.add("keyonly@example.com", nil, true) // legacy/API-key-only account
	_, err := svc.Login(context.Background(), "keyonly@example.com", "anything", "ua", "127.0.0.1")
	if err != auth.ErrInvalidCredentials {
		t.Errorf("want ErrInvalidCredentials when no password is set, got %v", err)
	}
}

func TestSessionService_Login_InactiveAccount(t *testing.T) {
	svc, users, _, _ := newTestService()
	hash, _ := auth.HashPassword("hunter2", auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	users.add("inactive@example.com", &hash, false)

	_, err := svc.Login(context.Background(), "inactive@example.com", "hunter2", "ua", "127.0.0.1")
	if err != auth.ErrAccountInactive {
		t.Errorf("want ErrAccountInactive, got %v", err)
	}
}

func TestSessionService_ResolveSession_ValidThenLogout(t *testing.T) {
	svc, users, _, _ := newTestService()
	hash, _ := auth.HashPassword("hunter2", auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	users.add("alice@example.com", &hash, true)

	res, err := svc.Login(context.Background(), "alice@example.com", "hunter2", "", "")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	resolved, err := svc.ResolveSession(context.Background(), res.SessionToken)
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	if resolved == nil {
		t.Fatal("want session to resolve")
	}
	if resolved.Session.CSRFTokenHash == "" {
		t.Error("want resolved session to carry its CSRF token hash for the caller to verify against")
	}

	if err := svc.Logout(context.Background(), res.SessionToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	resolved, err = svc.ResolveSession(context.Background(), res.SessionToken)
	if err != nil {
		t.Fatalf("ResolveSession after logout: %v", err)
	}
	if resolved != nil {
		t.Error("want session to no longer resolve after logout")
	}
}

func TestSessionService_ResolveSession_UnknownTokenReturnsNilNotError(t *testing.T) {
	svc, _, _, _ := newTestService()
	resolved, err := svc.ResolveSession(context.Background(), "not-a-real-token")
	if err != nil {
		t.Fatalf("want no error for an unknown token, got %v", err)
	}
	if resolved != nil {
		t.Error("want nil for an unknown token")
	}
}

func TestSessionService_Deactivate_RevokesSessionsAndKeys(t *testing.T) {
	svc, users, sessions, _ := newTestService()
	hash, _ := auth.HashPassword("hunter2", auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	u := users.add("alice@example.com", &hash, true)

	res, err := svc.Login(context.Background(), "alice@example.com", "hunter2", "", "")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	keys := newFakeAPIKeyStore()
	if err := svc.Deactivate(context.Background(), u.ID, keys); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if !keys.revokedFor[u.ID] {
		t.Error("want Deactivate to revoke all API keys for the user")
	}
	resolved, err := svc.ResolveSession(context.Background(), res.SessionToken)
	if err != nil {
		t.Fatalf("ResolveSession: %v", err)
	}
	if resolved != nil {
		t.Error("want the session to no longer resolve after deactivation")
	}
	if len(sessions.byHash) == 0 {
		t.Fatal("expected the session row to still exist (revoked, not deleted)")
	}
}

func TestSessionService_PasswordSetup_IssueAndConsume(t *testing.T) {
	svc, users, _, _ := newTestService()
	u := users.add("newadmin@example.com", nil, true)

	token, err := svc.IssuePasswordSetupToken(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("IssuePasswordSetupToken: %v", err)
	}
	if token == "" {
		t.Fatal("want non-empty setup token")
	}

	if err := svc.CompletePasswordSetup(context.Background(), token, "newpassword123"); err != nil {
		t.Fatalf("CompletePasswordSetup: %v", err)
	}
	if u.PasswordHash == nil {
		t.Fatal("want password hash set after completing setup")
	}
	ok, err := auth.VerifyPassword("newpassword123", *u.PasswordHash)
	if err != nil || !ok {
		t.Errorf("want the new password to verify, ok=%v err=%v", ok, err)
	}

	// Single-use: consuming again must fail.
	if err := svc.CompletePasswordSetup(context.Background(), token, "another-password"); err != auth.ErrInvalidSetupToken {
		t.Errorf("want ErrInvalidSetupToken on second use, got %v", err)
	}
}

func TestSessionService_PasswordSetup_IssuingNewTokenRevokesPrior(t *testing.T) {
	svc, users, _, setup := newTestService()
	u := users.add("newadmin@example.com", nil, true)

	first, err := svc.IssuePasswordSetupToken(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("first IssuePasswordSetupToken: %v", err)
	}
	if _, err := svc.IssuePasswordSetupToken(context.Background(), u.ID); err != nil {
		t.Fatalf("second IssuePasswordSetupToken: %v", err)
	}

	if err := svc.CompletePasswordSetup(context.Background(), first, "whatever-password"); err != auth.ErrInvalidSetupToken {
		t.Errorf("want the first token revoked once a second is issued, got %v", err)
	}
	_ = setup
}
