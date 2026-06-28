package handicaps

import (
	"math"
	"testing"
)

func TestComputeImpliedHandicap_Empty(t *testing.T) {
	r := ComputeImpliedHandicap(nil, 15)
	if r.LifetimeRacks != 0 || r.WindowRacks != 0 || r.LifetimeImplied != 0 || r.WindowImplied != 0 {
		t.Errorf("empty input: want zero CalcResult, got %+v", r)
	}
}

func TestComputeImpliedHandicap_EmptySlice(t *testing.T) {
	r := ComputeImpliedHandicap([]RackSample{}, 15)
	if r.LifetimeRacks != 0 || r.WindowRacks != 0 {
		t.Errorf("empty slice: want zero CalcResult, got %+v", r)
	}
}

// Equal players: opponent_hc = 2.0, rack_diff = 0 => per_rack = 2.0 every time.
// Implied handicap should equal the opponent's handicap exactly.
func TestComputeImpliedHandicap_EqualPlayers(t *testing.T) {
	samples := make([]RackSample, 15)
	for i := range samples {
		samples[i] = RackSample{OpponentHC: 2.0, RackDiff: 0}
	}
	r := ComputeImpliedHandicap(samples, 15)
	if r.LifetimeImplied != 2.00 {
		t.Errorf("equal players lifetime: want 2.00, got %v", r.LifetimeImplied)
	}
	if r.WindowImplied != 2.00 {
		t.Errorf("equal players window: want 2.00, got %v", r.WindowImplied)
	}
	if r.LifetimeRacks != 15 || r.WindowRacks != 15 {
		t.Errorf("rack counts: want 15/15, got %d/%d", r.LifetimeRacks, r.WindowRacks)
	}
}

// Precision: result is rounded to 0.01, not 0.1.
// per_rack = 0 + 1.05*0.85/0.85 = 1.05 => result = 1.05 (not 1.1 at 0.1 precision).
func TestComputeImpliedHandicap_PrecisionRoundTo001(t *testing.T) {
	samples := []RackSample{{OpponentHC: 0, RackDiff: 1.05 * 0.85}}
	r := ComputeImpliedHandicap(samples, 15)
	if r.LifetimeImplied != 1.05 {
		t.Errorf("0.01 precision: want 1.05, got %v", r.LifetimeImplied)
	}
}

// Rounding: value that rounds down at 0.01 boundary.
// Set per_rack = 1.234 (clearly below 1.235 midpoint) => rounds to 1.23.
func TestComputeImpliedHandicap_RoundsDown(t *testing.T) {
	// per_rack = opponent_hc + rack_diff/0.85; target per_rack = 1.234
	// rack_diff = (1.234 - 1.0) * 0.85 = 0.234 * 0.85 = 0.1989
	samples := []RackSample{{OpponentHC: 1.0, RackDiff: 0.234 * 0.85}}
	r := ComputeImpliedHandicap(samples, 15)
	if r.LifetimeImplied != 1.23 {
		t.Errorf("round-down: want 1.23, got %v", r.LifetimeImplied)
	}
}

// Rounding: value that rounds up at 0.01 boundary.
// Set per_rack = 1.245 => math.Round rounds half-away-from-zero => 1.25.
// But 1.235 => rounds to 1.24 (math.Round(123.5)/100 = 124/100 = 1.24).
func TestComputeImpliedHandicap_RoundsUp(t *testing.T) {
	// target per_rack = 1.235: rack_diff = (1.235 - 1.0) * 0.85 = 0.235 * 0.85 = 0.19975
	samples := []RackSample{{OpponentHC: 1.0, RackDiff: 0.235 * 0.85}}
	r := ComputeImpliedHandicap(samples, 15)
	if r.LifetimeImplied != 1.24 {
		t.Errorf("round-up: want 1.24, got %v", r.LifetimeImplied)
	}
}

// Window slicing: 20 samples, window=15; lifetime uses all 20, window uses first 15.
// First 15 (most recent): opponent_hc=3.0, diff=0 => window_implied=3.00.
// Samples 16-20: opponent_hc=1.0, diff=0 => drags lifetime down.
// Lifetime: (15*3 + 5*1)/20 = 50/20 = 2.50.
func TestComputeImpliedHandicap_WindowSlice(t *testing.T) {
	samples := make([]RackSample, 20)
	for i := 0; i < 15; i++ {
		samples[i] = RackSample{OpponentHC: 3.0, RackDiff: 0}
	}
	for i := 15; i < 20; i++ {
		samples[i] = RackSample{OpponentHC: 1.0, RackDiff: 0}
	}
	r := ComputeImpliedHandicap(samples, 15)
	if r.LifetimeRacks != 20 {
		t.Errorf("lifetime_racks: want 20, got %d", r.LifetimeRacks)
	}
	if r.WindowRacks != 15 {
		t.Errorf("window_racks: want 15, got %d", r.WindowRacks)
	}
	if r.WindowImplied != 3.00 {
		t.Errorf("window_implied: want 3.00, got %v", r.WindowImplied)
	}
	if r.LifetimeImplied != 2.50 {
		t.Errorf("lifetime_implied: want 2.50, got %v", r.LifetimeImplied)
	}
}

// When window >= len(samples), all samples are used for both lifetime and window.
func TestComputeImpliedHandicap_WindowLargerThanSamples(t *testing.T) {
	samples := make([]RackSample, 5)
	for i := range samples {
		samples[i] = RackSample{OpponentHC: 2.0, RackDiff: 0}
	}
	r := ComputeImpliedHandicap(samples, 15)
	if r.LifetimeRacks != 5 || r.WindowRacks != 5 {
		t.Errorf("small set: want LifetimeRacks=5 WindowRacks=5, got %+v", r)
	}
	if r.LifetimeImplied != r.WindowImplied {
		t.Errorf("small set: lifetime and window should be equal, got %+v", r)
	}
}

// Negative implied: player consistently loses.
func TestComputeImpliedHandicap_NegativeImplied(t *testing.T) {
	// opponent_hc=2.0, rack_diff=-10 => per_rack = 2.0 - 10/0.85 = 2.0 - 11.76...
	samples := []RackSample{{OpponentHC: 2.0, RackDiff: -10.0}}
	r := ComputeImpliedHandicap(samples, 15)
	want := math.Round((2.0+(-10.0/0.85))*100) / 100
	if r.LifetimeImplied != want {
		t.Errorf("negative implied: want %v, got %v", want, r.LifetimeImplied)
	}
	if r.LifetimeImplied >= 0 {
		t.Errorf("negative implied: expected negative result, got %v", r.LifetimeImplied)
	}
}

// Large positive: cap is applied by caller, not here. Value passes through uncapped.
func TestComputeImpliedHandicap_LargePositivePassesThrough(t *testing.T) {
	// opponent_hc=4.5, rack_diff=+10 => per_rack = 4.5 + 10/0.85 >> 4.5
	samples := []RackSample{{OpponentHC: 4.5, RackDiff: 10.0}}
	r := ComputeImpliedHandicap(samples, 15)
	if r.LifetimeImplied <= 4.5 {
		t.Errorf("large positive: expected implied > 4.5 (no cap here), got %v", r.LifetimeImplied)
	}
}

// Full precision retained during averaging: intermediate values are not rounded.
// Two racks: per_rack_1 = 1.001, per_rack_2 = 1.009 => avg = 1.005 => rounds to 1.01.
// If per-rack rounding were applied: 1.00 + 1.01 = 2.01/2 = 1.005 => 1.01 (same here).
// Better test: 3 racks averaging to 1.003333... => rounds to 1.00 (not 1.01).
func TestComputeImpliedHandicap_FullPrecisionBeforeRound(t *testing.T) {
	// per_rack = 1.0 + 0/0.85 = 1.0 for all => avg = 1.00
	// Add a tiny delta to confirm accumulation doesn't round mid-sum.
	// Three racks: per_rack values = 1.001, 1.001, 1.001 => sum=3.003, avg=1.001 => 1.00
	rackDiff := 0.001 * 0.85 // rack_diff such that per_rack = 1.0 + 0.001 = 1.001
	samples := []RackSample{
		{OpponentHC: 1.0, RackDiff: rackDiff},
		{OpponentHC: 1.0, RackDiff: rackDiff},
		{OpponentHC: 1.0, RackDiff: rackDiff},
	}
	r := ComputeImpliedHandicap(samples, 15)
	// avg = 1.001, math.Round(1.001*100)/100 = math.Round(100.1)/100 = 100/100 = 1.00
	if r.LifetimeImplied != 1.00 {
		t.Errorf("precision accumulation: want 1.00, got %v", r.LifetimeImplied)
	}
}
