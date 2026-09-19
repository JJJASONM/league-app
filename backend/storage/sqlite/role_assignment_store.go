package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domains/auth"
)

// RoleAssignmentStore implements auth.RoleAssignmentStore against the
// role_assignments table. The table's own CHECK constraint and partial
// unique indexes (see db/db.go) are the authoritative invariant
// enforcement -- this store does not duplicate that validation, it simply
// surfaces whatever error SQLite returns if a caller somehow attempts an
// invalid combination.
type RoleAssignmentStore struct {
	db *sql.DB
}

// NewRoleAssignmentStore returns a RoleAssignmentStore backed by db.
func NewRoleAssignmentStore(db *sql.DB) *RoleAssignmentStore {
	return &RoleAssignmentStore{db: db}
}

// ListForUser returns every role_assignments row for userID.
func (s *RoleAssignmentStore) ListForUser(ctx context.Context, userID int64) ([]auth.Assignment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT role_code, league_id FROM role_assignments WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("list role assignments: %w", err)
	}
	defer rows.Close()

	var out []auth.Assignment
	for rows.Next() {
		var roleCode string
		var leagueID sql.NullInt64
		if err := rows.Scan(&roleCode, &leagueID); err != nil {
			return nil, fmt.Errorf("scan role assignment: %w", err)
		}
		a := auth.Assignment{RoleCode: auth.RoleCode(roleCode)}
		if leagueID.Valid {
			a.LeagueID = &leagueID.Int64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Grant inserts a new role_assignments row. Inserting a duplicate
// (user_id, role_code, league_id) combination, or an invalid role_code/
// league_id pairing, fails with the underlying SQLite constraint error --
// callers should treat any error here as "grant rejected," not retry.
func (s *RoleAssignmentStore) Grant(ctx context.Context, userID int64, roleCode auth.RoleCode, leagueID *int64, createdByUserID *int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO role_assignments (user_id, role_code, league_id, created_by_user_id)
		VALUES (?, ?, ?, ?)
	`, userID, string(roleCode), leagueID, createdByUserID)
	if err != nil {
		return fmt.Errorf("grant role assignment: %w", err)
	}
	return nil
}

// Revoke removes a matching role_assignments row, if any. Revoking a
// grant that does not exist is not an error, matching this codebase's
// existing delete-is-idempotent convention.
func (s *RoleAssignmentStore) Revoke(ctx context.Context, userID int64, roleCode auth.RoleCode, leagueID *int64) error {
	var err error
	if leagueID == nil {
		_, err = s.db.ExecContext(ctx, `
			DELETE FROM role_assignments WHERE user_id = ? AND role_code = ? AND league_id IS NULL
		`, userID, string(roleCode))
	} else {
		_, err = s.db.ExecContext(ctx, `
			DELETE FROM role_assignments WHERE user_id = ? AND role_code = ? AND league_id = ?
		`, userID, string(roleCode), *leagueID)
	}
	if err != nil {
		return fmt.Errorf("revoke role assignment: %w", err)
	}
	return nil
}
