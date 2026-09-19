package auth

import (
	"context"
	"time"
)

// UserRecord is the subset of a users row this domain needs, independent
// of the existing models.User (which is shaped around the personal-API-key
// Apply flow and does not carry password/email fields).
type UserRecord struct {
	ID                int64
	Username          string
	Email             *string
	PasswordHash      *string
	Active            bool
	PlayerID          *int64
	PlayerName        string
	MustResetPassword bool
}

// UserStore is the identity/credential persistence this domain needs.
type UserStore interface {
	GetByEmail(ctx context.Context, normalizedEmail string) (*UserRecord, error)
	GetByID(ctx context.Context, id int64) (*UserRecord, error)
	SetPassword(ctx context.Context, userID int64, encodedHash string) error
	SetActive(ctx context.Context, userID int64, active bool) error
	// ProvisionUser creates a new user row for system-admin-driven account
	// provisioning (Users Admin: "create/provision user with email and
	// optional player link"). No password is set here -- a password_setup
	// token is issued separately. username is auto-derived by the caller
	// to satisfy the legacy NOT NULL UNIQUE column; it is never shown as
	// the account's identity (email is).
	ProvisionUser(ctx context.Context, username, normalizedEmail string, playerID *int64) (UserRecord, error)
}

// RoleAssignmentStore is the role_assignments persistence this domain
// needs. Grant/Revoke only ever touch system_admin/league_admin rows --
// "player" is never accepted (also enforced by the table's own CHECK
// constraint as a second, authoritative line of defense).
type RoleAssignmentStore interface {
	ListForUser(ctx context.Context, userID int64) ([]Assignment, error)
	Grant(ctx context.Context, userID int64, roleCode RoleCode, leagueID *int64, createdByUserID *int64) error
	Revoke(ctx context.Context, userID int64, roleCode RoleCode, leagueID *int64) error
}

// Session is one browser login session. TokenHash/CSRFTokenHash are
// SHA-256 of the two independently-generated random values the browser
// holds as cookies -- neither cleartext value is ever persisted.
type Session struct {
	ID            int64
	UserID        int64
	TokenHash     string
	CSRFTokenHash string
	CreatedAt     time.Time
	LastSeenAt    time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	UserAgent     string
	IPAddress     string
}

// SessionStore is the sessions persistence this domain needs.
type SessionStore interface {
	Create(ctx context.Context, s Session) (Session, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	Touch(ctx context.Context, id int64, lastSeenAt, expiresAt time.Time) error
	Revoke(ctx context.Context, id int64) error
	RevokeAllForUser(ctx context.Context, userID int64) error
	DeleteExpiredForUser(ctx context.Context, userID int64) error
}

// PasswordSetupTokenStore is the password_setup_tokens persistence this
// domain needs. ConsumeAndSetPassword performs verification, the password
// update, and marking the token used all in one transaction -- PM
// decision: "setting the password and consuming the token happen
// atomically," so a failure partway can never burn a token without
// actually changing the password.
type PasswordSetupTokenStore interface {
	Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	RevokeActiveForUser(ctx context.Context, userID int64) error
	ConsumeAndSetPassword(ctx context.Context, tokenHash, encodedPasswordHash string) (userID int64, ok bool, err error)
}

// APIKeyStore is the user_api_keys persistence needed for account
// administration (Users Admin: "rotate/revoke API keys"). Creation of new
// keys for the existing Apply flow continues to go through
// ApplyAuthStore/ApplyAuthResolver unchanged; this interface only adds the
// lifecycle actions that flow did not previously need.
type APIKeyStore interface {
	RevokeAllForUser(ctx context.Context, userID int64) error
}
