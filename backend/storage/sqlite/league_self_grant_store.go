package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domains/leagues"
	"league_app/models"
)

// LeagueSelfGrantStore performs the one Users/Roles Phase 1 workflow that
// PM required to be truly atomic across the leagues and role_assignments
// tables: creating a league and granting its creator league_admin scope
// for it. LeagueService/LeagueStore and RoleAssignmentStore are normally
// kept as separate domains by this codebase's convention (see AGENTS.md),
// and the original Phase 1 implementation of this workflow respected that
// by composing the two stores in the handler with a compensating delete on
// grant failure -- but that left a real, if narrow, crash window where a
// league could exist with no admin able to manage it. Since both tables
// live in the same physical SQLite database, a single transaction spanning
// both is possible without a generic cross-domain transaction framework;
// this store is a narrowly-scoped, deliberate exception for this one
// workflow, not a precedent for merging the two domains generally.
type LeagueSelfGrantStore struct {
	db *sql.DB
}

// NewLeagueSelfGrantStore returns a LeagueSelfGrantStore backed by db.
func NewLeagueSelfGrantStore(db *sql.DB) *LeagueSelfGrantStore {
	return &LeagueSelfGrantStore{db: db}
}

// CreateLeagueWithLeagueAdminGrant creates a league and grants
// creatorUserID league_admin scope for it in a single transaction: both
// rows are committed together, or neither is. A grant failure (e.g. the
// creator somehow already holds a conflicting role_assignments row) rolls
// back the league insert too, so the request never leaves behind a league
// with zero admins.
func (s *LeagueSelfGrantStore) CreateLeagueWithLeagueAdminGrant(ctx context.Context, input leagues.CreateLeagueInput, creatorUserID int64) (models.League, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.League{}, fmt.Errorf("begin league creation transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO leagues (name, game_format, day_of_week) VALUES (?,?,?)`,
		input.Name, input.GameFormat, input.DayOfWeek)
	if err != nil {
		return models.League{}, fmt.Errorf("create league: %w", err)
	}
	leagueID, _ := res.LastInsertId()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO role_assignments (user_id, role_code, league_id, created_by_user_id)
		VALUES (?, 'league_admin', ?, ?)
	`, creatorUserID, leagueID, creatorUserID); err != nil {
		return models.League{}, fmt.Errorf("grant creator league_admin access: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return models.League{}, fmt.Errorf("commit league creation: %w", err)
	}

	return models.League{
		ID:         leagueID,
		Name:       input.Name,
		GameFormat: input.GameFormat,
		DayOfWeek:  input.DayOfWeek,
	}, nil
}
