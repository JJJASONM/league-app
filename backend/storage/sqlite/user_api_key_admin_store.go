package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// UserAPIKeyAdminStore implements auth.APIKeyStore -- the lifecycle
// actions (revocation) account administration needs on top of
// user_api_keys, distinct from ApplyAuthStore's key creation/resolution
// for the existing personal-key Apply flow.
type UserAPIKeyAdminStore struct {
	db *sql.DB
}

// NewUserAPIKeyAdminStore returns a UserAPIKeyAdminStore backed by db.
func NewUserAPIKeyAdminStore(db *sql.DB) *UserAPIKeyAdminStore {
	return &UserAPIKeyAdminStore{db: db}
}

// RevokeAllForUser marks every currently-active key for userID revoked.
func (s *UserAPIKeyAdminStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_api_keys SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = ? AND revoked_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("revoke all api keys for user: %w", err)
	}
	return nil
}
