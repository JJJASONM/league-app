package handicaps

import (
	"math"
	"testing"
)

// TestComputeRequestHash_NaN_AssignedHC_ReturnsError verifies defense-in-depth:
// computeRequestHash rejects NaN in ExpectedAssignedHC before marshalling.
func TestComputeRequestHash_NaN_AssignedHC_ReturnsError(t *testing.T) {
	_, err := computeRequestHash(requestHashInput{
		SeasonID: 1,
		Entries: []requestHashEntry{{
			PlayerID:              1,
			ExpectedAssignedHC:    math.NaN(),
			ExpectedRecommendedHC: 3.0,
			RecToken:              "tok",
		}},
	})
	if err == nil {
		t.Error("want error for NaN expected_assigned_hc, got nil")
	}
}

// TestComputeRequestHash_Inf_RecommendedHC_ReturnsError verifies defense-in-depth:
// computeRequestHash rejects +Inf in ExpectedRecommendedHC before marshalling.
func TestComputeRequestHash_Inf_RecommendedHC_ReturnsError(t *testing.T) {
	_, err := computeRequestHash(requestHashInput{
		SeasonID: 1,
		Entries: []requestHashEntry{{
			PlayerID:              1,
			ExpectedAssignedHC:    2.5,
			ExpectedRecommendedHC: math.Inf(1),
			RecToken:              "tok",
		}},
	})
	if err == nil {
		t.Error("want error for +Inf expected_recommended_hc, got nil")
	}
}

// TestComputeRequestHash_ValidInput_IsDeterministic proves that two calls with
// identical inputs produce identical hashes (no randomness, no timestamp).
func TestComputeRequestHash_ValidInput_IsDeterministic(t *testing.T) {
	in := requestHashInput{
		SeasonID: 1,
		Entries: []requestHashEntry{{
			PlayerID:              1,
			ExpectedAssignedHC:    2.5,
			ExpectedRecommendedHC: 3.0,
			RecToken:              "abc123",
		}},
	}
	h1, err1 := computeRequestHash(in)
	h2, err2 := computeRequestHash(in)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v / %v", err1, err2)
	}
	if h1 != h2 {
		t.Errorf("hash is not deterministic: %q vs %q", h1, h2)
	}
	if h1 == "" {
		t.Error("want non-empty hash")
	}
}

// TestComputeRequestHash_EntryOrder_IsHashIndependent proves that entries in
// different order produce the same hash (entries are sorted by PlayerID before
// marshalling).
func TestComputeRequestHash_EntryOrder_IsHashIndependent(t *testing.T) {
	e1 := requestHashEntry{PlayerID: 1, ExpectedAssignedHC: 2.5, ExpectedRecommendedHC: 3.0, RecToken: "tok1"}
	e2 := requestHashEntry{PlayerID: 2, ExpectedAssignedHC: 1.5, ExpectedRecommendedHC: 2.0, RecToken: "tok2"}

	h1, err1 := computeRequestHash(requestHashInput{SeasonID: 1, Entries: []requestHashEntry{e1, e2}})
	h2, err2 := computeRequestHash(requestHashInput{SeasonID: 1, Entries: []requestHashEntry{e2, e1}})
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v / %v", err1, err2)
	}
	if h1 != h2 {
		t.Errorf("hash changed with entry order:\n  forward:  %q\n  reversed: %q", h1, h2)
	}
}

// TestComputeRequestHash_CentEquivalentAssignedHC_ProducesSameHash proves that
// float noise in expected_assigned_hc does not change the hash.
func TestComputeRequestHash_CentEquivalentAssignedHC_ProducesSameHash(t *testing.T) {
	base := requestHashEntry{PlayerID: 1, ExpectedAssignedHC: 2.50, ExpectedRecommendedHC: 3.0, RecToken: "tok"}
	noisy := requestHashEntry{PlayerID: 1, ExpectedAssignedHC: 2.4999999999, ExpectedRecommendedHC: 3.0, RecToken: "tok"}

	h1, err1 := computeRequestHash(requestHashInput{SeasonID: 1, Entries: []requestHashEntry{base}})
	h2, err2 := computeRequestHash(requestHashInput{SeasonID: 1, Entries: []requestHashEntry{noisy}})
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v / %v", err1, err2)
	}
	if h1 != h2 {
		t.Errorf("cent-equivalent expected_assigned_hc must produce same hash:\n  2.50:         %q\n  2.4999999999: %q", h1, h2)
	}
}

// TestComputeRequestHash_CentEquivalentRecommendedHC_ProducesSameHash proves that
// float noise in expected_recommended_hc does not change the hash.
func TestComputeRequestHash_CentEquivalentRecommendedHC_ProducesSameHash(t *testing.T) {
	base := requestHashEntry{PlayerID: 1, ExpectedAssignedHC: 2.5, ExpectedRecommendedHC: 3.00, RecToken: "tok"}
	noisy := requestHashEntry{PlayerID: 1, ExpectedAssignedHC: 2.5, ExpectedRecommendedHC: 3.0000000001, RecToken: "tok"}

	h1, err1 := computeRequestHash(requestHashInput{SeasonID: 1, Entries: []requestHashEntry{base}})
	h2, err2 := computeRequestHash(requestHashInput{SeasonID: 1, Entries: []requestHashEntry{noisy}})
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v / %v", err1, err2)
	}
	if h1 != h2 {
		t.Errorf("cent-equivalent expected_recommended_hc must produce same hash:\n  3.00:         %q\n  3.0000000001: %q", h1, h2)
	}
}
