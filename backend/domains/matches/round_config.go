package matches

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"league_app/backend/domains/rules"
	"league_app/logic"
)

// ResolveRoundConfig reads handicap_multiplier and min_ball_handicap from the
// rule store for the given season. Returns documented defaults when a key is
// absent. Returns an error when a stored value is present but malformed:
// non-positive, NaN, or Inf for the multiplier; non-integer or negative for
// min_ball_handicap.
func ResolveRoundConfig(ctx context.Context, rs rules.RuleStore, seasonID int64) (RoundConfig, error) {
	multStr, multExists, err := rs.GetValue(ctx, seasonID, "handicap_multiplier")
	if err != nil {
		return RoundConfig{}, fmt.Errorf("season %d: handicap_multiplier: %w", seasonID, err)
	}
	mult := logic.Multiplier
	if multExists && multStr != "" {
		f, parseErr := strconv.ParseFloat(multStr, 64)
		if parseErr != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
			return RoundConfig{}, fmt.Errorf("season %d: handicap_multiplier %q is not a positive finite number", seasonID, multStr)
		}
		mult = f
	}

	minBallStr, minBallExists, err := rs.GetValue(ctx, seasonID, "min_ball_handicap")
	if err != nil {
		return RoundConfig{}, fmt.Errorf("season %d: min_ball_handicap: %w", seasonID, err)
	}
	minBallHC := 0
	if minBallExists && minBallStr != "" {
		n, parseErr := strconv.Atoi(minBallStr)
		if parseErr != nil {
			return RoundConfig{}, fmt.Errorf("season %d: min_ball_handicap %q is not an integer", seasonID, minBallStr)
		}
		if n < 0 {
			return RoundConfig{}, fmt.Errorf("season %d: min_ball_handicap %d must be >= 0", seasonID, n)
		}
		minBallHC = n
	}
	return RoundConfig{Multiplier: mult, MinBallHC: minBallHC}, nil
}
