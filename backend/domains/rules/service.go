package rules

import (
	"context"
	"fmt"
	"time"

	"league_app/backend/domainerr"
	"league_app/models"
)

// RuleService orchestrates per-season rule CRUD.
// Validation (via ValidateValue) runs in the service before any persistence call.
type RuleService struct {
	store RuleStore
}

// NewRuleService returns a RuleService backed by the given store.
func NewRuleService(store RuleStore) *RuleService {
	return &RuleService{store: store}
}

// List returns all season_rules for the given season.
func (s *RuleService) List(ctx context.Context, seasonID int64) ([]models.SeasonRule, error) {
	return s.store.ListBySeasonID(ctx, seasonID)
}

// Upsert inserts or replaces a season rule.
// When rule.RuleKey is blank, a unique key is generated.
// Returns domainerr.InvalidInput when the value fails ValidateValue.
func (s *RuleService) Upsert(ctx context.Context, rule models.SeasonRule) (models.SeasonRule, error) {
	if rule.RuleKey == "" {
		rule.RuleKey = fmt.Sprintf("rule_%d", time.Now().UnixMilli())
	}
	if err := ValidateValue(rule.RuleKey, rule.RuleValue); err != nil {
		return models.SeasonRule{}, domainerr.New("RULE_INVALID_VALUE", domainerr.InvalidInput, err.Error())
	}
	return s.store.Upsert(ctx, rule)
}

// Update validates a new value against the existing rule's key, then persists.
// Returns domainerr.NotFound when ruleID does not exist.
// Returns domainerr.InvalidInput when the value fails ValidateValue.
func (s *RuleService) Update(ctx context.Context, ruleID int64, label, value string) error {
	existing, err := s.store.GetByID(ctx, ruleID)
	if err != nil {
		return err // propagates domainerr.NotFound from store
	}
	if err := ValidateValue(existing.RuleKey, value); err != nil {
		return domainerr.New("RULE_INVALID_VALUE", domainerr.InvalidInput, err.Error())
	}
	return s.store.UpdateByID(ctx, ruleID, label, value)
}

// Delete removes the rule with the given ID.
func (s *RuleService) Delete(ctx context.Context, ruleID int64) error {
	return s.store.DeleteByID(ctx, ruleID)
}
