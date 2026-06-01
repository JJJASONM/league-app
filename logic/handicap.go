// Package logic implements the business rules for the pool league.
package logic

// HandicapAdjustment returns the bonus games awarded to the weaker player.
//
// The formula is: adjustment = (higherSL - lowerSL) * factor
//
// factor comes from the season's handicap_factor field.
//   - 0.0  → no handicap (straight play)
//   - 1.0  → 1 bonus game per skill-level difference
//   - 0.5  → half-game per difference (fractional, rounded down)
//
// Returns (homeBonus, awayBonus). The higher-skilled side gets 0 bonus.
func HandicapAdjustment(homeSL, awaySL int, factor float64) (homeBonus, awayBonus int) {
	if factor == 0 || homeSL == awaySL {
		return 0, 0
	}
	diff := homeSL - awaySL
	if diff > 0 {
		// Home is stronger → away gets bonus
		return 0, int(float64(diff) * factor)
	}
	// Away is stronger → home gets bonus
	return int(float64(-diff) * factor), 0
}

// EffectiveWins returns the handicap-adjusted win total for a player.
// The handicap bonus is added to games_won before comparison.
func EffectiveWins(gamesWon, handicapApplied int) int {
	return gamesWon + handicapApplied
}
