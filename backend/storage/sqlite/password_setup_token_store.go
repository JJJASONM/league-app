package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PasswordSetupTokenStore implements auth.PasswordSetupTokenStore against
// the password_setup_tokens table.
type PasswordSetupTokenStore struct {
	db *sql.DB
}

// NewPasswordSetupTokenStore returns a PasswordSetupTokenStore backed by db.
func NewPasswordSetupTokenStore(db *sql.DB) *PasswordSetupTokenStore {
	return &PasswordSetupTokenStore{db: db}
}

func (s *PasswordSetupTokenStore) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO password_setup_tokens (user_id, token_hash, expires_at) VALUES (?, ?, ?)
	`, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("create password setup token: %w", err)
	}
	return nil
}

func (s *PasswordSetupTokenStore) RevokeActiveForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE password_setup_tokens SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND used_at IS NULL AND revoked_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("revoke active password setup tokens: %w", err)
	}
	return nil
}

// ConsumeAndSetPassword verifies the token (exists, unused, unrevoked,
// unexpired), updates the owning user's password_hash, and marks the
// token used -- all inside one transaction, so a failure partway can
// never burn a token without actually changing the password (PM
// requirement: "setting the password and consuming the token happen
// atomically").
func (s *PasswordSetupTokenStore) ConsumeAndSetPassword(ctx context.Context, tokenHash, encodedPasswordHash string) (int64, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("consume password setup token: %w", err)
	}
	defer tx.Rollback()

	var userID int64
	var usedAt, revokedAt sql.NullTime
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, expires_at, used_at, revoked_at FROM password_setup_tokens WHERE token_hash = ?
	`, tokenHash).Scan(&userID, &expiresAt, &usedAt, &revokedAt)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("consume password setup token: %w", err)
	}
	if usedAt.Valid || revokedAt.Valid || time.Now().After(expiresAt) {
		return 0, false, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, password_updated_at = CURRENT_TIMESTAMP,
		       must_reset_password = 0, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, encodedPasswordHash, userID); err != nil {
		return 0, false, fmt.Errorf("consume password setup token: set password: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE password_setup_tokens SET used_at = CURRENT_TIMESTAMP WHERE token_hash = ?
	`, tokenHash); err != nil {
		return 0, false, fmt.Errorf("consume password setup token: mark used: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("consume password setup token: %w", err)
	}
	return userID, true, nil
}
