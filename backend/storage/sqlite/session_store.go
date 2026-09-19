package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"league_app/backend/domains/auth"
)

// SessionStore implements auth.SessionStore against the sessions table.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore returns a SessionStore backed by db.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) Create(ctx context.Context, sess auth.Session) (auth.Session, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, csrf_token_hash, created_at, last_seen_at, expires_at, user_agent, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sess.UserID, sess.TokenHash, sess.CSRFTokenHash, sess.CreatedAt, sess.LastSeenAt, sess.ExpiresAt, sess.UserAgent, sess.IPAddress)
	if err != nil {
		return auth.Session{}, fmt.Errorf("create session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return auth.Session{}, fmt.Errorf("create session: %w", err)
	}
	sess.ID = id
	return sess, nil
}

func (s *SessionStore) GetByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, csrf_token_hash, created_at, last_seen_at, expires_at, revoked_at, user_agent, ip_address
		FROM sessions WHERE token_hash = ?
	`, tokenHash)
	var sess auth.Session
	var revoked sql.NullTime
	err := row.Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.CSRFTokenHash,
		&sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt, &revoked, &sess.UserAgent, &sess.IPAddress)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if revoked.Valid {
		sess.RevokedAt = &revoked.Time
	}
	return &sess, nil
}

func (s *SessionStore) Touch(ctx context.Context, id int64, lastSeenAt, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE id = ?`, lastSeenAt, expiresAt, id)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (s *SessionStore) Revoke(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = ? AND revoked_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("revoke all sessions for user: %w", err)
	}
	return nil
}

func (s *SessionStore) DeleteExpiredForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE user_id = ? AND (expires_at < CURRENT_TIMESTAMP OR revoked_at IS NOT NULL)
	`, userID)
	if err != nil {
		return fmt.Errorf("delete expired sessions for user: %w", err)
	}
	return nil
}
