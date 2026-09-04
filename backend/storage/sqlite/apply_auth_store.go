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
// PlayerID is scanned (Player Account Access Phase 1) so downstream
// ownership checks (e.g. Player Overview) can compare it against a
// requested player id; PlayerName is left empty here (only ListApplyUsers
// populates it, for the Users Admin screen's display).
func (s *ApplyAuthStore) ResolveApplyUserByAPIKey(ctx context.Context, apiKey string) (*models.User, error) {
	hash := hashAPIKey(apiKey)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, role, player_id, active, created_at
		FROM users
		WHERE api_key_hash = ? AND active = 1
	`, hash)

	var u models.User
	var active int
	var playerID sql.NullInt64
	err := row.Scan(&u.ID, &u.Username, &u.Role, &playerID, &active, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve apply user: %w", err)
	}
	u.Active = active == 1
	if playerID.Valid {
		u.PlayerID = &playerID.Int64
	}
	return &u, nil
}

// CreateApplyUser creates a new user with the given username and role,
// generates a random 32-byte API key, stores only its SHA-256 hash, and
// returns the user along with the cleartext key. The cleartext key is not
// stored anywhere and cannot be retrieved again. Role validation (which
// roles may be assigned to a new user) is the caller's responsibility --
// this store persists whatever role it is given. player_id is left NULL --
// use CreateApplyPlayerUser to create a role=player user linked to a player.
func (s *ApplyAuthStore) CreateApplyUser(ctx context.Context, username, role string) (models.User, string, error) {
	cleartext, hash, err := generateAPIKey()
	if err != nil {
		return models.User{}, "", fmt.Errorf("generate api key: %w", err)
	}

	var u models.User
	var active int
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO users (username, api_key_hash, role, active)
		VALUES (?, ?, ?, 1)
		RETURNING id, username, role, active, created_at
	`, username, hash, role).Scan(&u.ID, &u.Username, &u.Role, &active, &u.CreatedAt)
	if err != nil {
		return models.User{}, "", fmt.Errorf("create apply user: %w", err)
	}
	u.Active = active == 1
	return u, cleartext, nil
}

// CreateApplyPlayerUser creates a new role="player" user linked to playerID
// (Player Account Access Phase 1), generates a random 32-byte API key, and
// returns the user along with the cleartext key. playerID must reference an
// existing player -- the caller (postUser) validates this via PlayerManager
// before calling; the players(id) foreign key is a backstop, not the
// primary validation path (SQLite reports an FK violation without naming
// which row failed, which would make a poor API error message on its own).
func (s *ApplyAuthStore) CreateApplyPlayerUser(ctx context.Context, username string, playerID int64) (models.User, string, error) {
	cleartext, hash, err := generateAPIKey()
	if err != nil {
		return models.User{}, "", fmt.Errorf("generate api key: %w", err)
	}

	var u models.User
	var active int
	var pid sql.NullInt64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO users (username, api_key_hash, role, player_id, active)
		VALUES (?, ?, 'player', ?, 1)
		RETURNING id, username, role, player_id, active, created_at
	`, username, hash, playerID).Scan(&u.ID, &u.Username, &u.Role, &pid, &active, &u.CreatedAt)
	if err != nil {
		return models.User{}, "", fmt.Errorf("create apply player user: %w", err)
	}
	u.Active = active == 1
	if pid.Valid {
		u.PlayerID = &pid.Int64
	}
	return u, cleartext, nil
}

// ListApplyUsers returns all users, ordered by id. The api_key_hash column is
// never included in the result. PlayerName is resolved via a LEFT JOIN for
// linked (role=player) users -- a display convenience for the Users Admin
// screen, empty for every other user.
func (s *ApplyAuthStore) ListApplyUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.role, u.player_id,
		       COALESCE(p.first_name || ' ' || p.last_name, ''),
		       u.active, u.created_at
		FROM users u
		LEFT JOIN players p ON p.id = u.player_id
		ORDER BY u.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list apply users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var active int
		var playerID sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &playerID, &u.PlayerName, &active, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan apply user: %w", err)
		}
		u.Active = active == 1
		if playerID.Valid {
			u.PlayerID = &playerID.Int64
		}
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
