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

// ResolveApplyUserByAPIKey looks up an active user with a non-revoked
// user_api_keys row whose key_hash matches SHA-256(apiKey). Returns nil,
// nil when no matching active user/key is found. PlayerID is scanned
// (Player Account Access Phase 1) so downstream ownership checks (e.g.
// Player Overview) can compare it against a requested player id;
// PlayerName is left empty here (only ListApplyUsers populates it, for
// the Users Admin screen's display).
//
// Users/Roles Phase 1: API-key credentials moved off users.api_key_hash
// (which could not support a password-only user while it remained NOT
// NULL UNIQUE) into this separate user_api_keys table -- see
// db.migrateUsersAndAPIKeys for the one-time migration. This query's
// external behavior (and this method's signature) is unchanged for every
// existing caller; only the underlying table changed.
func (s *ApplyAuthStore) ResolveApplyUserByAPIKey(ctx context.Context, apiKey string) (*models.User, error) {
	hash := hashAPIKey(apiKey)
	row := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.role, u.player_id, u.active, u.created_at
		FROM users u
		JOIN user_api_keys k ON k.user_id = u.id
		WHERE k.key_hash = ? AND k.revoked_at IS NULL AND u.active = 1
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.User{}, "", fmt.Errorf("create apply user: %w", err)
	}
	defer tx.Rollback()

	var u models.User
	var active int
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO users (username, role, active)
		VALUES (?, ?, 1)
		RETURNING id, username, role, active, created_at
	`, username, role).Scan(&u.ID, &u.Username, &u.Role, &active, &u.CreatedAt); err != nil {
		return models.User{}, "", fmt.Errorf("create apply user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_api_keys (user_id, key_hash, label) VALUES (?, ?, 'primary')
	`, u.ID, hash); err != nil {
		return models.User{}, "", fmt.Errorf("create apply user key: %w", err)
	}
	// Users/Roles Phase 1: this endpoint has no concept of league scope
	// (its body is just {username, role}), so a role='system_admin' or
	// legacy 'admin' user -- both global by definition -- gets a matching
	// global role_assignments row immediately, keeping this the working
	// bootstrap path for the very first system_admin on a fresh install
	// (nothing else can grant that first row). A role='league_admin' user
	// created here gets NO role_assignments row: league_admin now
	// requires a concrete league, which this endpoint cannot supply --
	// use POST /api/auth/admin/users/{id}/roles afterward (system_admin
	// only) to grant a specific league. This is a real, disclosed
	// behavior change for this legacy endpoint, not a bug: creating a
	// "league_admin" here no longer grants any scoped access on its own.
	if role == "system_admin" || role == "admin" {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO role_assignments (user_id, role_code, league_id) VALUES (?, 'system_admin', NULL)
		`, u.ID); err != nil {
			return models.User{}, "", fmt.Errorf("create apply user role assignment: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.User{}, "", fmt.Errorf("create apply player user: %w", err)
	}
	defer tx.Rollback()

	var u models.User
	var active int
	var pid sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO users (username, role, player_id, active)
		VALUES (?, 'player', ?, 1)
		RETURNING id, username, role, player_id, active, created_at
	`, username, playerID).Scan(&u.ID, &u.Username, &u.Role, &pid, &active, &u.CreatedAt); err != nil {
		return models.User{}, "", fmt.Errorf("create apply player user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_api_keys (user_id, key_hash, label) VALUES (?, ?, 'primary')
	`, u.ID, hash); err != nil {
		return models.User{}, "", fmt.Errorf("create apply player user key: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.User{}, "", fmt.Errorf("create apply player user: %w", err)
	}
	u.Active = active == 1
	if pid.Valid {
		u.PlayerID = &pid.Int64
	}
	return u, cleartext, nil
}

// ListApplyUsers returns all users, ordered by id, each with its real
// role_assignments rows attached (User.Assignments -- Users/Roles Phase 1
// UI correction: the Users Admin screen must show actual current access,
// not the legacy flat Role column, once scoped assignments exist). The
// api_key_hash column is never included in the result. PlayerName is
// resolved via a LEFT JOIN for linked (role=player) users -- a display
// convenience for the Users Admin screen, empty for every other user.
//
// This is a single query (LEFT JOIN role_assignments, one row per
// assignment, collapsed back into each user's Assignments slice below) --
// deliberately not one role_assignments query per user, which would be an
// N+1 pattern against a screen that already lists every user at once.
func (s *ApplyAuthStore) ListApplyUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.role, u.player_id,
		       COALESCE(p.first_name || ' ' || p.last_name, ''),
		       u.active, u.created_at, u.email,
		       ra.role_code, ra.league_id
		FROM users u
		LEFT JOIN players p ON p.id = u.player_id
		LEFT JOIN role_assignments ra ON ra.user_id = u.id
		ORDER BY u.id, ra.role_code, ra.league_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list apply users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	var current *models.User
	for rows.Next() {
		var u models.User
		var active int
		var playerID sql.NullInt64
		var email sql.NullString
		var roleCode sql.NullString
		var assignmentLeagueID sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &playerID, &u.PlayerName, &active, &u.CreatedAt, &email, &roleCode, &assignmentLeagueID); err != nil {
			return nil, fmt.Errorf("scan apply user: %w", err)
		}
		if current == nil || current.ID != u.ID {
			u.Active = active == 1
			if playerID.Valid {
				u.PlayerID = &playerID.Int64
			}
			if email.Valid {
				u.Email = &email.String
			}
			users = append(users, u)
			current = &users[len(users)-1]
		}
		if roleCode.Valid {
			assignment := models.UserRoleAssignment{RoleCode: roleCode.String}
			if assignmentLeagueID.Valid {
				assignment.LeagueID = &assignmentLeagueID.Int64
			}
			current.Assignments = append(current.Assignments, assignment)
		}
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
