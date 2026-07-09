// Package logic is a minimal pure-calculation package.
// It provides the 8-ball scoresheet spot formula (CalcSpot, CalcSpotM) and
// legacy skill-level helpers retained for future 9-ball support.
// Domain-specific logic lives in backend/domains/; this package has no domain imports.
package logic

import "math"

// --- 8-Ball Scoresheet Spot Formula -----------------------------------------
//
// Derived from the FileMaker Scoresheet_Gameday calculation fields:
//
//	R1G1 Hndcp = Abs(((H1_Hndcp - V1_Hndcp) * .85) * 3)
//
// Which simplifies to:
//
//	spot = Abs(player_a_handicap - player_b_handicap) * 2.55
//	              (because  0.85 x 3 = 2.55)
//
// The app rounds to the nearest whole ball because partial spots are not
// operationally meaningful on a scoresheet.

// Multiplier is the FileMaker-equivalent handicap multiplier: 0.85 * 3.
const Multiplier = 2.55

// SpotResult holds the output of the 8-ball scoresheet spot calculation.
type SpotResult struct {
	// Pts is the number of bonus ball-points spotted to the lower-rated player.
	// Zero when both players have equal handicaps.
	Pts int

	// To is "home", "away", or "" (equal).
	// "home" means the home player receives the spot (they are the lower-rated).
	// "away" means the away player receives the spot.
	To string
}

// CalcSpot computes the 8-ball scoresheet spot for a single pairing.
//
// homeHC and awayHC are the players' diff-rating handicaps
// (positive = wins more than loses; negative = loses more than wins).
//
// The lower-rated player (lower handicap value) receives the spot.
func CalcSpot(homeHC, awayHC float64) SpotResult {
	diff := homeHC - awayHC
	pts := int(math.Round(math.Abs(diff) * Multiplier))

	var to string
	switch {
	case homeHC < awayHC:
		to = "home"
	case awayHC < homeHC:
		to = "away"
	default:
		to = ""
	}

	return SpotResult{Pts: pts, To: to}
}


// CalcSpotM is like CalcSpot but accepts a custom multiplier, allowing
// per-season handicap_multiplier rule values to override the default 2.55.
// Pass logic.Multiplier as the multiplier to get default behaviour.
func CalcSpotM(homeHC, awayHC, multiplier float64) SpotResult {
	diff := homeHC - awayHC
	pts := int(math.Round(math.Abs(diff) * multiplier))

	var to string
	switch {
	case homeHC < awayHC:
		to = "home"
	case awayHC < homeHC:
		to = "away"
	default:
		to = ""
	}

	return SpotResult{Pts: pts, To: to}
}

// --- Legacy skill-level helper (kept for backward compatibility) -------------

// HandicapAdjustment returns the bonus games awarded to the weaker player using
// an integer skill-level system. Not used for 8-ball scoresheets; retained for
// any 9-ball code that may reference it.
//
// factor:
//   - 0.0  no handicap (straight play)
//   - 1.0  1 bonus game per skill-level difference
//   - 0.5  half-game per difference (fractional, rounded down)
//
// Returns (homeBonus, awayBonus). The higher-skilled side gets 0 bonus.
func HandicapAdjustment(homeSL, awaySL int, factor float64) (homeBonus, awayBonus int) {
	if factor == 0 || homeSL == awaySL {
		return 0, 0
	}
	diff := homeSL - awaySL
	if diff > 0 {
		// Home is stronger -- away gets bonus
		return 0, int(float64(diff) * factor)
	}
	// Away is stronger -- home gets bonus
	return int(float64(-diff) * factor), 0
}

// EffectiveWins returns the handicap-adjusted win total for a player.
func EffectiveWins(gamesWon, handicapApplied int) int {
	return gamesWon + handicapApplied
}
