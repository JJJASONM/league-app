package logic

import (
	"math"
	"testing"
)

// ─── CalcSpot tests ───────────────────────────────────────────────────────────

func TestCalcSpot_EqualHandicaps(t *testing.T) {
	r := CalcSpot(2.0, 2.0)
	if r.Pts != 0 {
		t.Errorf("equal handicaps: want Pts=0, got %d", r.Pts)
	}
	if r.To != "" {
		t.Errorf("equal handicaps: want To='', got %q", r.To)
	}
}

func TestCalcSpot_HomeLower_ReceivesSpot(t *testing.T) {
	// Home diff = -2.71, Away diff = -0.83  → home is lower-rated
	r := CalcSpot(-2.71, -0.83)
	if r.To != "home" {
		t.Errorf("home lower: want To='home', got %q", r.To)
	}
	if r.Pts <= 0 {
		t.Errorf("home lower: want Pts>0, got %d", r.Pts)
	}
	// Manual: abs(-2.71 - (-0.83)) * 2.55 = abs(-1.88) * 2.55 = 4.794 → round = 5
	want := 5
	if r.Pts != want {
		t.Errorf("home lower: want Pts=%d, got %d", want, r.Pts)
	}
}

func TestCalcSpot_AwayLower_ReceivesSpot(t *testing.T) {
	// Home diff = 3.40, Away diff = -0.83  → away is lower-rated
	r := CalcSpot(3.40, -0.83)
	if r.To != "away" {
		t.Errorf("away lower: want To='away', got %q", r.To)
	}
	// abs(3.40 - (-0.83)) * 2.55 = 4.23 * 2.55 = 10.7865 → round = 11
	want := 11
	if r.Pts != want {
		t.Errorf("away lower: want Pts=%d, got %d", want, r.Pts)
	}
}

func TestCalcSpot_UsesMultiplier255(t *testing.T) {
	if Multiplier != 2.55 {
		t.Errorf("Multiplier: want 2.55, got %v", Multiplier)
	}
	// Verify the multiplier is exactly 0.85 * 3
	want := 0.85 * 3
	if math.Abs(Multiplier-want) > 1e-10 {
		t.Errorf("Multiplier does not equal 0.85*3: want %v, got %v", want, Multiplier)
	}
}

func TestCalcSpot_RoundingBehavior(t *testing.T) {
	cases := []struct {
		homeHC, awayHC float64
		wantPts        int
		desc           string
	}{
		// abs(diff) * 2.55 = 1.0 * 2.55 = 2.55 → round to 3
		{1.0, 0.0, 3, "1.0 diff → 3 pts"},
		// abs(diff) * 2.55 = 2.0 * 2.55 = 5.1 → round to 5
		{2.0, 0.0, 5, "2.0 diff → 5 pts"},
		// abs(diff) * 2.55 = 0.5 * 2.55 = 1.275 → round to 1
		{0.5, 0.0, 1, "0.5 diff → 1 pt"},
		// abs(diff) * 2.55 = 0.196 * 2.55 = 0.4998, below the 0.5 rounding threshold
		{0.196, 0.0, 0, "~0.196 diff -> 0 pts"},
		// abs(diff) * 2.55 = 0.197 * 2.55 = 0.50235, above the 0.5 rounding threshold
		{0.197, 0.0, 1, "~0.197 diff -> 1 pt"},
		// exactly zero
		{0.0, 0.0, 0, "0 diff → 0 pts"},
	}
	for _, tc := range cases {
		r := CalcSpot(tc.homeHC, tc.awayHC)
		if r.Pts != tc.wantPts {
			t.Errorf("%s: homeHC=%.4f awayHC=%.4f want %d got %d",
				tc.desc, tc.homeHC, tc.awayHC, tc.wantPts, r.Pts)
		}
	}
}

func TestCalcSpot_ZeroVsNegative(t *testing.T) {
	// home = 0, away = -3.40 → home is higher → away receives spot
	r := CalcSpot(0, -3.40)
	if r.To != "away" {
		t.Errorf("zero vs negative: want To='away', got %q", r.To)
	}
	// abs(0 - (-3.40)) * 2.55 = 3.40 * 2.55 = 8.67 → round = 9
	want := 9
	if r.Pts != want {
		t.Errorf("zero vs negative: want Pts=%d, got %d", want, r.Pts)
	}
}

func TestCalcSpot_LargePositiveHandicaps(t *testing.T) {
	// e.g. 9-ball race-to numbers used as handicap
	r := CalcSpot(7.0, 3.0)
	if r.To != "away" {
		t.Errorf("7 vs 3: want To='away', got %q", r.To)
	}
	// abs(7-3) * 2.55 = 4 * 2.55 = 10.2 → round = 10
	want := 10
	if r.Pts != want {
		t.Errorf("7 vs 3: want Pts=%d, got %d", want, r.Pts)
	}
}

// ─── HandicapAdjustment (legacy) tests ───────────────────────────────────────

func TestHandicapAdjustment_NoFactor(t *testing.T) {
	h, a := HandicapAdjustment(5, 3, 0)
	if h != 0 || a != 0 {
		t.Errorf("factor=0: want (0,0), got (%d,%d)", h, a)
	}
}

func TestHandicapAdjustment_Equal(t *testing.T) {
	h, a := HandicapAdjustment(4, 4, 1.0)
	if h != 0 || a != 0 {
		t.Errorf("equal SL: want (0,0), got (%d,%d)", h, a)
	}
}

func TestHandicapAdjustment_HomeStronger(t *testing.T) {
	h, a := HandicapAdjustment(7, 3, 1.0)
	if h != 0 || a != 4 {
		t.Errorf("home stronger: want (0,4), got (%d,%d)", h, a)
	}
}

func TestHandicapAdjustment_AwayStronger(t *testing.T) {
	h, a := HandicapAdjustment(3, 7, 1.0)
	if h != 4 || a != 0 {
		t.Errorf("away stronger: want (4,0), got (%d,%d)", h, a)
	}
}
