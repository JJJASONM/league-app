package sqlite

// ApplyAuthStore implements the ApplyAuthResolver interface for the
// handicap Apply endpoint. It stores SHA-256 hashes of API keys — the
// cleartext key is returned once at create time and never stored.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"league_app/models"
)

// ApplyAuthStore resolves and creates application users for Apply authorization.
type ApplyAuthStore struct {
	db *sql.DB
}

// NewApplyAuthStore returns an ApplyAuthStore backed by the given database.
func NewApplyAuthStore(db *sql.DB) *ApplyAuthStore {
	return &ApplyAuthStore{db: db}
}

// ResolveApplyUserByAPIKey looks up an active user whose api_key_hash matches
// SHA-256(apiKey). Returns nil, nil when no matching active user is found.
func (s *ApplyAuthStore) ResolveApplyUserByAPIKey(ctx context.Context, apiKey string) (*models.User, error) {
	hash := hashAPIKey(apiKey)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, role, active, created_at
		FROM users
		WHERE api_key_hash = ? AND active = 1
	`, hash)

	var u models.User
	var active int
	err := row.Scan(&u.ID, &u.Username, &u.Role, &active, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve apply user: %w", err)
	}
	u.Active = active == 1
	return &u, nil
}

// CreateApplyUser creates a new user with the given username, generates a
// random 32-byte API key, stores only its SHA-256 hash, and returns the user
// along with the cleartext key. The cleartext key is not stored anywhere and
// cannot be retrieved again.
func (s *ApplyAuthStore) CreateApplyUser(ctx context.Context, username string) (models.User, string, error) {
	cleartext, hash, err := generateAPIKey()
	if err != nil {
		return models.User{}, "", fmt.Errorf("generate api key: %w", err)
	}

	var u models.User
	var active int
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO users (username, api_key_hash, role, active)
		VALUES (?, ?, 'admin', 1)
		RETURNING id, username, role, active, created_at
	`, username, hash).Scan(&u.ID, &u.Username, &u.Role, &active, &u.CreatedAt)
	if err != nil {
		return models.User{}, "", fmt.Errorf("create apply user: %w", err)
	}
	u.Active = active == 1
	return u, cleartext, nil
}

// ListApplyUsers returns all users, ordered by id. The api_key_hash column is
// never included in the result.
func (s *ApplyAuthStore) ListApplyUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, role, active, created_at
		FROM users
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list apply users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var active int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &active, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan apply user: %w", err)
		}
		u.Active = active == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

// hashAPIKey returns the SHA-256 hash of the API key as a 64-char lowercase hex string.
func hashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

// generateAPIKey generates a cryptographically random 32-byte key (hex-encoded
// as 64 chars) and returns both the cleartext and its SHA-256 hash.
func generateAPIKey() (cleartext, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	cleartext = hex.EncodeToString(b)
	hash = hashAPIKey(cleartext)
	return cleartext, hash, nil
}
