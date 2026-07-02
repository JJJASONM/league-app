package rules

import (
	"context"

	"league_app/models"
)

// RuleStore is the persistence interface for per-season rule CRUD.
// Implementations must be safe for concurrent use.
// GetByID returns domainerr.NotFound when no row exists for the given ID.
type RuleStore interface {
	// ListBySeasonID returns all season_rules rows for the season, ordered by id.
	// Returns an empty (non-nil) slice when no rows exist.
	ListBySeasonID(ctx context.Context, seasonID int64) ([]models.SeasonRule, error)

	// Upsert inserts or replaces a season_rules row (INSERT OR REPLACE semantics).
	// Returns the saved row with its assigned ID.
	Upsert(ctx context.Context, rule models.SeasonRule) (models.SeasonRule, error)

	// GetByID returns the rule with the given ID.
	// Returns domainerr.NotFound when no row exists.
	GetByID(ctx context.Context, ruleID int64) (models.SeasonRule, error)

	// UpdateByID updates the rule_label and rule_value for the given ID.
	UpdateByID(ctx context.Context, ruleID int64, label, value string) error

	// DeleteByID removes the rule with the given ID.
	DeleteByID(ctx context.Context, ruleID int64) error

	// GetValue returns the stored text value for a season+key pair.
	// exists=false when no row is present; err is non-nil only on DB error.
	GetValue(ctx context.Context, seasonID int64, key string) (value string, exists bool, err error)
}
