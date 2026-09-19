package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domains/auth"
)

// AuthUserStore implements auth.UserStore against the users table's
// Phase-1 columns (email, password_hash). It is a separate small store
// from ApplyAuthStore -- that one owns API-key creation/resolution for the
// existing personal-key flow; this one owns password-login identity,
// which is a genuinely new concern layered on the same table.
type AuthUserStore struct {
	db *sql.DB
}

// NewAuthUserStore returns an AuthUserStore backed by db.
func NewAuthUserStore(db *sql.DB) *AuthUserStore {
	return &AuthUserStore{db: db}
}

const authUserSelectCols = `
	SELECT u.id, u.username, u.email, u.password_hash, u.active,
	       u.player_id, COALESCE(p.first_name || ' ' || p.last_name, ''),
	       u.must_reset_password
	FROM users u
	LEFT JOIN players p ON p.id = u.player_id`

func scanAuthUser(row interface{ Scan(...any) error }) (*auth.UserRecord, error) {
	var u auth.UserRecord
	var email, passwordHash sql.NullString
	var active, mustReset int
	var playerID sql.NullInt64
	if err := row.Scan(&u.ID, &u.Username, &email, &passwordHash, &active, &playerID, &u.PlayerName, &mustReset); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if email.Valid {
		u.Email = &email.String
	}
	if passwordHash.Valid {
		u.PasswordHash = &passwordHash.String
	}
	u.Active = active == 1
	u.MustResetPassword = mustReset == 1
	if playerID.Valid {
		u.PlayerID = &playerID.Int64
	}
	return &u, nil
}

// GetByEmail looks up a user by normalized email. Returns nil, nil when
// no user has that email -- callers must not distinguish this from any
// other login failure (see auth.ErrInvalidCredentials).
func (s *AuthUserStore) GetByEmail(ctx context.Context, normalizedEmail string) (*auth.UserRecord, error) {
	row := s.db.QueryRowContext(ctx, authUserSelectCols+` WHERE u.email = ?`, normalizedEmail)
	u, err := scanAuthUser(row)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// GetByID looks up a user by id, regardless of active state (callers that
// need to reject inactive accounts do so explicitly).
func (s *AuthUserStore) GetByID(ctx context.Context, id int64) (*auth.UserRecord, error) {
	row := s.db.QueryRowContext(ctx, authUserSelectCols+` WHERE u.id = ?`, id)
	u, err := scanAuthUser(row)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// SetPassword stores a new encoded password hash for userID and clears
// must_reset_password (setting a password, however it happened, satisfies
// any pending forced-reset requirement).
func (s *AuthUserStore) SetPassword(ctx context.Context, userID int64, encodedHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, password_updated_at = CURRENT_TIMESTAMP,
		       must_reset_password = 0, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, encodedHash, userID)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	return nil
}

// SetActive sets the account's active flag.
func (s *AuthUserStore) SetActive(ctx context.Context, userID int64, active bool) error {
	activeInt := 0
	if active {
		activeInt = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET active = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, activeInt, userID)
	if err != nil {
		return fmt.Errorf("set active: %w", err)
	}
	return nil
}

// ProvisionUser creates a new user row with email set and no password yet
// (a password_setup_token is issued separately) -- the system_admin
// "create/provision user" account-administration action. username is
// caller-derived (auto-generated from the email's local part) purely to
// satisfy the legacy NOT NULL UNIQUE column; it is never the login
// identity or shown as the account identity in the UI.
func (s *AuthUserStore) ProvisionUser(ctx context.Context, username, normalizedEmail string, playerID *int64) (auth.UserRecord, error) {
	var u auth.UserRecord
	var active int
	var pid sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (username, email, player_id, active)
		VALUES (?, ?, ?, 1)
		RETURNING id, username, active, player_id
	`, username, normalizedEmail, playerID).Scan(&u.ID, &u.Username, &active, &pid)
	if err != nil {
		return auth.UserRecord{}, fmt.Errorf("provision user: %w", err)
	}
	u.Active = active == 1
	u.Email = &normalizedEmail
	if pid.Valid {
		u.PlayerID = &pid.Int64
	}
	return u, nil
}
