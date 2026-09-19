package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Session lifetime, per PM decision: a 7-day idle timeout refreshed on
// every authenticated request (sliding window), capped by a 30-day
// absolute maximum measured from the session's own creation time
// regardless of activity.
const (
	SessionIdleTimeout      = 7 * 24 * time.Hour
	SessionAbsoluteLifetime = 30 * 24 * time.Hour
)

// PasswordSetupTokenTTL is how long a system-admin-issued password setup
// token remains valid before it must be reissued.
const PasswordSetupTokenTTL = 72 * time.Hour

var (
	// ErrInvalidCredentials is returned for every login failure mode that
	// must not be distinguishable from another (unknown email, wrong
	// password, no password set yet) -- PM decision: "use generic
	// authentication/registration errors where practical," specifically to
	// avoid email enumeration.
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountInactive    = errors.New("account is inactive")
	ErrInvalidSetupToken  = errors.New("invalid or expired setup token")
)

// SessionService implements login, logout, session resolution, and
// password setup -- the full credential/session lifecycle for Users/Roles
// Phase 1. It depends only on the store interfaces in store.go, so it can
// be unit-tested against in-memory fakes without a real database.
type SessionService struct {
	users       UserStore
	roles       RoleAssignmentStore
	sessions    SessionStore
	setupTokens PasswordSetupTokenStore
	argon2      Argon2Params
	now         func() time.Time
}

// NewSessionService returns a SessionService using the given stores and
// the Argon2id parameters new/rehashed passwords should target (see
// CalibrateArgon2).
func NewSessionService(users UserStore, roles RoleAssignmentStore, sessions SessionStore, setupTokens PasswordSetupTokenStore, argon2Target Argon2Params) *SessionService {
	return &SessionService{
		users:       users,
		roles:       roles,
		sessions:    sessions,
		setupTokens: setupTokens,
		argon2:      argon2Target,
		now:         time.Now,
	}
}

// LoginResult carries the two cleartext, one-time-visible token values the
// caller (the HTTP handler) must set as cookies -- SessionService never
// touches HTTP directly, keeping it testable without a server.
type LoginResult struct {
	Identity     Identity
	SessionToken string
	CSRFToken    string
	ExpiresAt    time.Time
}

// Login verifies email+password and, on success, creates a new session.
// Every failure path (unknown email, no password set, wrong password,
// inactive account) returns ErrInvalidCredentials except ErrAccountInactive,
// which is deliberately distinguishable -- an inactive account is not a
// secret the way "does this email exist" is, and telling a legitimately
// deactivated user why they cannot log in is not an enumeration risk.
func (s *SessionService) Login(ctx context.Context, email, password, userAgent, ipAddress string) (LoginResult, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	user, err := s.users.GetByEmail(ctx, normalized)
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: %w", err)
	}
	if user == nil || user.PasswordHash == nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	if !user.Active {
		return LoginResult{}, ErrAccountInactive
	}
	ok, err := VerifyPassword(password, *user.PasswordHash)
	if err != nil || !ok {
		return LoginResult{}, ErrInvalidCredentials
	}

	// Rehash-on-login: if this account's stored hash falls below the
	// current target parameters, upgrade it transparently now that we
	// have the cleartext password in hand (the only time it is ever
	// available to do so).
	if needs, err := NeedsRehash(*user.PasswordHash, s.argon2); err == nil && needs {
		if newHash, err := HashPassword(password, s.argon2); err == nil {
			_ = s.users.SetPassword(ctx, user.ID, newHash)
		}
	}

	sessionToken, err := generateRandomToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: %w", err)
	}
	csrfToken, err := generateRandomToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: %w", err)
	}

	now := s.now()
	expires := now.Add(SessionIdleTimeout)
	if _, err := s.sessions.Create(ctx, Session{
		UserID:        user.ID,
		TokenHash:     hashToken(sessionToken),
		CSRFTokenHash: hashToken(csrfToken),
		CreatedAt:     now,
		LastSeenAt:    now,
		ExpiresAt:     expires,
		UserAgent:     userAgent,
		IPAddress:     ipAddress,
	}); err != nil {
		return LoginResult{}, fmt.Errorf("login: create session: %w", err)
	}

	// Opportunistic cleanup: this user's own expired sessions, cheapest
	// possible cleanup strategy given SQLite/single-process (no cron).
	_ = s.sessions.DeleteExpiredForUser(ctx, user.ID)

	identity, err := s.buildIdentity(ctx, user)
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: %w", err)
	}
	return LoginResult{Identity: identity, SessionToken: sessionToken, CSRFToken: csrfToken, ExpiresAt: expires}, nil
}

// ResolvedSession pairs the resolved Identity with the underlying Session
// row -- callers (auth middleware) need the row's CSRFTokenHash to verify
// the X-CSRF-Token header on mutating requests.
type ResolvedSession struct {
	Identity Identity
	Session  Session
}

// ResolveSession looks up a session by its cleartext cookie token, checks
// idle and absolute expiry, checks the user is still active, and -- on
// success -- slides the idle window forward (never past the absolute cap).
// Returns nil, nil for any invalid/expired/revoked/inactive-user case
// (never an error for those -- an expired session is not exceptional).
func (s *SessionService) ResolveSession(ctx context.Context, sessionToken string) (*ResolvedSession, error) {
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(sessionToken))
	if err != nil {
		return nil, fmt.Errorf("resolve session: %w", err)
	}
	if sess == nil || sess.RevokedAt != nil {
		return nil, nil
	}
	now := s.now()
	if now.After(sess.ExpiresAt) {
		return nil, nil
	}
	absoluteCap := sess.CreatedAt.Add(SessionAbsoluteLifetime)
	if now.After(absoluteCap) {
		return nil, nil
	}

	user, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, fmt.Errorf("resolve session: %w", err)
	}
	if user == nil || !user.Active {
		return nil, nil
	}

	newExpiry := now.Add(SessionIdleTimeout)
	if newExpiry.After(absoluteCap) {
		newExpiry = absoluteCap
	}
	_ = s.sessions.Touch(ctx, sess.ID, now, newExpiry)
	sess.LastSeenAt, sess.ExpiresAt = now, newExpiry

	identity, err := s.buildIdentity(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("resolve session: %w", err)
	}
	return &ResolvedSession{Identity: identity, Session: *sess}, nil
}

// Logout revokes the session identified by its cleartext cookie token.
// Revoking an already-invalid/unknown token is not an error -- logout is
// idempotent, matching the existing DeleteLineupPlan-style convention in
// this codebase ("deleting something already gone is not an error").
func (s *SessionService) Logout(ctx context.Context, sessionToken string) error {
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(sessionToken))
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	if sess == nil {
		return nil
	}
	return s.sessions.Revoke(ctx, sess.ID)
}

// Deactivate marks the account inactive and immediately revokes every
// session and API key it holds -- deactivation must take effect
// instantly, not merely be caught by the next request re-checking
// `active` (though that check remains in place too, as defense in depth).
func (s *SessionService) Deactivate(ctx context.Context, userID int64, apiKeys APIKeyStore) error {
	if err := s.users.SetActive(ctx, userID, false); err != nil {
		return fmt.Errorf("deactivate: %w", err)
	}
	if err := s.sessions.RevokeAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("deactivate: revoke sessions: %w", err)
	}
	if apiKeys != nil {
		if err := apiKeys.RevokeAllForUser(ctx, userID); err != nil {
			return fmt.Errorf("deactivate: revoke api keys: %w", err)
		}
	}
	return nil
}

// Reactivate marks the account active again. It does not restore any
// previously-revoked session or API key -- the user must log in (or be
// issued a new key) again, which is the correct behavior after a
// deactivation.
func (s *SessionService) Reactivate(ctx context.Context, userID int64) error {
	return s.users.SetActive(ctx, userID, true)
}

// IssuePasswordSetupToken generates a new one-time setup token for
// userID, revoking every other still-active token for that user first
// (PM decision: "issuing another token revokes prior active setup
// tokens"). Returns the cleartext token, shown once to the system_admin
// who issued it and communicated out of band -- never stored or logged.
func (s *SessionService) IssuePasswordSetupToken(ctx context.Context, userID int64) (string, error) {
	if err := s.setupTokens.RevokeActiveForUser(ctx, userID); err != nil {
		return "", fmt.Errorf("issue password setup token: %w", err)
	}
	token, err := generateRandomToken()
	if err != nil {
		return "", fmt.Errorf("issue password setup token: %w", err)
	}
	if err := s.setupTokens.Create(ctx, userID, hashToken(token), s.now().Add(PasswordSetupTokenTTL)); err != nil {
		return "", fmt.Errorf("issue password setup token: %w", err)
	}
	return token, nil
}

// CompletePasswordSetup atomically consumes a one-time setup token and
// sets the account's password (via PasswordSetupTokenStore.
// ConsumeAndSetPassword, which does both in one transaction). Returns
// ErrInvalidSetupToken for any invalid/expired/already-used/revoked token.
func (s *SessionService) CompletePasswordSetup(ctx context.Context, token, newPassword string) error {
	encoded, err := HashPassword(newPassword, s.argon2)
	if err != nil {
		return fmt.Errorf("complete password setup: %w", err)
	}
	_, ok, err := s.setupTokens.ConsumeAndSetPassword(ctx, hashToken(token), encoded)
	if err != nil {
		return fmt.Errorf("complete password setup: %w", err)
	}
	if !ok {
		return ErrInvalidSetupToken
	}
	return nil
}

// Identity resolves the current Identity for userID directly, without a
// session -- used by API-key-authenticated requests (Users/Roles Phase 1
// keeps the existing personal-API-key path fully functional; this lets it
// share the same Authorize policy and Identity shape as session auth).
func (s *SessionService) Identity(ctx context.Context, userID int64) (Identity, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return Identity{}, fmt.Errorf("identity: %w", err)
	}
	if user == nil {
		return Identity{}, fmt.Errorf("identity: user %d not found", userID)
	}
	return s.buildIdentity(ctx, user)
}

func (s *SessionService) buildIdentity(ctx context.Context, u *UserRecord) (Identity, error) {
	assignments, err := s.roles.ListForUser(ctx, u.ID)
	if err != nil {
		return Identity{}, fmt.Errorf("list role assignments: %w", err)
	}
	email := ""
	if u.Email != nil {
		email = *u.Email
	}
	return Identity{
		UserID:      u.ID,
		Username:    u.Username,
		Email:       email,
		Active:      u.Active,
		PlayerID:    u.PlayerID,
		PlayerName:  u.PlayerName,
		Assignments: assignments,
	}, nil
}

// generateRandomToken returns a cryptographically random 32-byte value,
// hex-encoded (64 chars) -- the same shape as the existing API-key
// generation in apply_auth_store.go, for consistency.
func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns SHA-256(token) as 64-char lowercase hex -- the
// cleartext token is never stored, only this hash, exactly matching the
// existing API-key hashing convention.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
