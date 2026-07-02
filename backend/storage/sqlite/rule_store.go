package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"league_app/backend/domainerr"
	"league_app/backend/domains/rules"
	"league_app/models"
)

// RuleStore implements rules.RuleStore against a SQLite database.
type RuleStore struct {
	db *sql.DB
}

// NewRuleStore returns a RuleStore backed by db.
func NewRuleStore(db *sql.DB) *RuleStore {
	return &RuleStore{db: db}
}

// Compile-time interface check.
var _ rules.RuleStore = (*RuleStore)(nil)

func (s *RuleStore) ListBySeasonID(ctx context.Context, seasonID int64) ([]models.SeasonRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, season_id, rule_key, rule_label, rule_value
		 FROM season_rules WHERE season_id=? ORDER BY id`,
		seasonID)
	if err != nil {
		return nil, fmt.Errorf("list season rules: %w", err)
	}
	defer rows.Close()
	out := []models.SeasonRule{}
	for rows.Next() {
		var r models.SeasonRule
		if err := rows.Scan(&r.ID, &r.SeasonID, &r.RuleKey, &r.RuleLabel, &r.RuleValue); err != nil {
			return nil, fmt.Errorf("list season rules: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *RuleStore) Upsert(ctx context.Context, rule models.SeasonRule) (models.SeasonRule, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value)
		 VALUES (?,?,?,?)`,
		rule.SeasonID, rule.RuleKey, rule.RuleLabel, rule.RuleValue)
	if err != nil {
		return models.SeasonRule{}, fmt.Errorf("upsert season rule: %w", err)
	}
	id, _ := res.LastInsertId()
	rule.ID = id
	return rule, nil
}

func (s *RuleStore) GetByID(ctx context.Context, ruleID int64) (models.SeasonRule, error) {
	var r models.SeasonRule
	err := s.db.QueryRowContext(ctx,
		`SELECT id, season_id, rule_key, rule_label, rule_value
		 FROM season_rules WHERE id=?`, ruleID).
		Scan(&r.ID, &r.SeasonID, &r.RuleKey, &r.RuleLabel, &r.RuleValue)
	if errors.Is(err, sql.ErrNoRows) {
		return models.SeasonRule{}, domainerr.New("RULE_NOT_FOUND", domainerr.NotFound, "rule not found")
	}
	if err != nil {
		return models.SeasonRule{}, fmt.Errorf("get season rule %d: %w", ruleID, err)
	}
	return r, nil
}

func (s *RuleStore) UpdateByID(ctx context.Context, ruleID int64, label, value string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE season_rules SET rule_label=?, rule_value=? WHERE id=?`,
		label, value, ruleID)
	if err != nil {
		return fmt.Errorf("update season rule %d: %w", ruleID, err)
	}
	return nil
}

func (s *RuleStore) DeleteByID(ctx context.Context, ruleID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM season_rules WHERE id=?`, ruleID)
	if err != nil {
		return fmt.Errorf("delete season rule %d: %w", ruleID, err)
	}
	return nil
}

func (s *RuleStore) GetValue(ctx context.Context, seasonID int64, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT rule_value FROM season_rules WHERE season_id=? AND rule_key=?`,
		seasonID, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get rule value %q season %d: %w", key, seasonID, err)
	}
	return value, true, nil
}
