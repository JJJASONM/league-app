// Package handicaps owns the opponent-normalized handicap calculation formula.
// All functions in this package are pure (no DB access); callers supply the data.
//
// Formula status: draft -- not yet adopted as league policy.
// The formula inverts the FileMaker spot equation to derive each player's implied
// handicap from individual rack outcomes and their opponent's handicap snapshot.
package handicaps

import "math"

// RackSample is one eligible 8-ball game rack from a reviewed player's perspective.
// OpponentHC is the opponent's handicap snapshot (home_handicap_used or
// away_handicap_used) at the time the rack was played.
// RackDiff is (player_score - opponent_score) for this rack.
//
// Only racks where the opponent snapshot is non-NULL are included; racks with
// NULL opponent snapshots are excluded and counted separately by the caller.
// Samples must be ordered most-recent-first before being passed to
// ComputeImpliedHandicap so that window slicing takes the correct racks.
type RackSample struct {
	OpponentHC float64
	RackDiff   float64
}

// CalcResult holds the outcome of ComputeImpliedHandicap for one player.
// Both Lifetime and Window values use 0.01 precision.
// All fields are zero when the input samples slice is empty.
type CalcResult struct {
	LifetimeImplied float64 // average implied HC over all included racks
	LifetimeRacks   int
	WindowImplied   float64 // average implied HC over the most recent window racks
	WindowRacks     int
}

// ComputeImpliedHandicap applies the opponent-normalized rack formula to samples.
//
// Formula per rack (draft):
//
//	per_rack = opponent_hc + rack_diff / 0.85
//
// Derivation: the FileMaker spot formula is
//
//	spot_balls = round(abs(home_hc - away_hc) * 2.55)
//
// where 2.55 = 0.85 * 3 (per-game compression * 3 games). Working backwards,
// the opponent's handicap is the baseline from which the reviewed player's
// implied rating diverges by rack_diff / 0.85 (undoing the per-game factor).
//
// Precision: full float64 precision is retained during summation and averaging.
// The final average is rounded to the nearest 0.01 before returning.
// No intermediate rounding is applied. The configured cap (max_individual_handicap)
// is applied by the caller, not here.
//
// window controls how many of the most-recent samples form the current-window
// value. If window >= len(samples), all samples are used for both lifetime and
// window. An empty samples slice returns a zero CalcResult.
func ComputeImpliedHandicap(samples []RackSample, window int) CalcResult {
	if len(samples) == 0 {
		return CalcResult{}
	}

	avg := func(s []RackSample) float64 {
		var sum float64
		for _, r := range s {
			sum += r.OpponentHC + r.RackDiff/0.85
		}
		return math.Round(sum/float64(len(s))*100) / 100
	}

	n := window
	if n > len(samples) {
		n = len(samples)
	}

	return CalcResult{
		LifetimeImplied: avg(samples),
		LifetimeRacks:   len(samples),
		WindowImplied:   avg(samples[:n]),
		WindowRacks:     n,
	}
}
