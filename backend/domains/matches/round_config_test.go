package matches_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/domains/rules"
	"league_app/logic"
	"league_app/models"
)

// stubRuleStore is shared across all matches_test package files.
type stubRuleStore struct {
	values    map[string]string
	getValErr error
}

func (s *stubRuleStore) GetValue(_ context.Context, _ int64, key string) (string, bool, error) {
	if s.getValErr != nil {
		return "", false, s.getValErr
	}
	v, ok := s.values[key]
	return v, ok, nil
}

func (s *stubRuleStore) ListBySeasonID(_ context.Context, _ int64) ([]models.SeasonRule, error) {
	return nil, nil
}
func (s *stubRuleStore) Upsert(_ context.Context, r models.SeasonRule) (models.SeasonRule, error) {
	return r, nil
}
func (s *stubRuleStore) GetByID(_ context.Context, _ int64) (models.SeasonRule, error) {
	return models.SeasonRule{}, nil
}
func (s *stubRuleStore) UpdateByID(_ context.Context, _ int64, _, _ string) error { return nil }
func (s *stubRuleStore) DeleteByID(_ context.Context, _ int64) error               { return nil }

var _ rules.RuleStore = (*stubRuleStore)(nil)

func TestResolveRoundConfig_DefaultsWhenNoRules(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{}}
	cfg, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Multiplier != logic.Multiplier {
		t.Errorf("want Multiplier=%v, got %v", logic.Multiplier, cfg.Multiplier)
	}
	if cfg.MinBallHC != 0 {
		t.Errorf("want MinBallHC=0, got %d", cfg.MinBallHC)
	}
}

func TestResolveRoundConfig_StoredMultiplierIsUsed(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"handicap_multiplier": "3.00"}}
	cfg, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Multiplier != 3.00 {
		t.Errorf("want Multiplier=3.00, got %v", cfg.Multiplier)
	}
}

func TestResolveRoundConfig_StoredMinBallIsUsed(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"min_ball_handicap": "2"}}
	cfg, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinBallHC != 2 {
		t.Errorf("want MinBallHC=2, got %d", cfg.MinBallHC)
	}
}

func TestResolveRoundConfig_NonNumberMultiplier_ReturnsError(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"handicap_multiplier": "banana"}}
	_, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err == nil {
		t.Fatal("want error for non-number multiplier, got nil")
	}
}

func TestResolveRoundConfig_ZeroMultiplier_ReturnsError(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"handicap_multiplier": "0"}}
	_, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err == nil {
		t.Fatal("want error for zero multiplier, got nil")
	}
}

func TestResolveRoundConfig_NegativeMultiplier_ReturnsError(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"handicap_multiplier": "-1.5"}}
	_, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err == nil {
		t.Fatal("want error for negative multiplier, got nil")
	}
}

func TestResolveRoundConfig_NonIntegerMinBall_ReturnsError(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"min_ball_handicap": "1.5"}}
	_, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err == nil {
		t.Fatal("want error for non-integer min_ball_handicap, got nil")
	}
}

func TestResolveRoundConfig_NegativeMinBall_ReturnsError(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"min_ball_handicap": "-1"}}
	_, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err == nil {
		t.Fatal("want error for negative min_ball_handicap, got nil")
	}
}

func TestResolveRoundConfig_DBErrorOnMultiplier_Propagates(t *testing.T) {
	dbErr := errors.New("db exploded")
	rs := &stubRuleStore{getValErr: dbErr}
	_, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if !errors.Is(err, dbErr) {
		t.Errorf("want wrapped db error, got %v", err)
	}
}

func TestResolveRoundConfig_BlankStoredMultiplier_UsesDefault(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"handicap_multiplier": ""}}
	cfg, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Multiplier != logic.Multiplier {
		t.Errorf("want default Multiplier=%v, got %v", logic.Multiplier, cfg.Multiplier)
	}
}

func TestResolveRoundConfig_BlankStoredMinBall_UsesDefault(t *testing.T) {
	rs := &stubRuleStore{values: map[string]string{"min_ball_handicap": ""}}
	cfg, err := matches.ResolveRoundConfig(context.Background(), rs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinBallHC != 0 {
		t.Errorf("want default MinBallHC=0, got %d", cfg.MinBallHC)
	}
}
