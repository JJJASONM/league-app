package handicaps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// isFiniteHC reports whether v is a usable handicap value.
// Go's math package has no IsFinite; this is the canonical substitute.
func isFiniteHC(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// recTokenInput is the versioned struct marshalled to produce a recommendation token.
// Version allows future schema evolution without silently invalidating prior tokens.
// AllSamples covers ALL included eligible racks (full lifetime set, not just the window slice)
// so that adding new racks always changes the token even when the window is full.
type recTokenInput struct {
	Version       int           `json:"v"`
	SeasonID      int64         `json:"season_id"`
	PlayerID      int64         `json:"player_id"`
	AssignedHC    float64       `json:"assigned_hc"`
	Method        string        `json:"method"`
	WindowSize    int           `json:"window_size"`
	Threshold     int           `json:"threshold"`
	MaxHC         float64       `json:"max_hc"`
	AllSamples    []tokenSample `json:"all_samples"`
	LifetimeRacks int           `json:"lifetime_racks"`
	WindowRacks   int           `json:"window_racks"`
	RecommendedHC float64       `json:"recommended_hc"`
}

type tokenSample struct {
	OppHC    float64 `json:"opp_hc"`
	RackDiff float64 `json:"rack_diff"`
}

// computeRecToken returns a hex-encoded SHA-256 hash of the versioned token input.
// All numeric inputs are validated for finiteness before marshalling. Non-finite
// values return an error rather than producing a malformed or colliding hash
// (json.Marshal on a NaN/Inf produces an error whose error bytes would be nil,
// making every non-finite input collapse to sha256("")).
func computeRecToken(in recTokenInput) (string, error) {
	if !isFiniteHC(in.AssignedHC) {
		return "", fmt.Errorf("rec token: assigned_hc is not finite (%v)", in.AssignedHC)
	}
	if !isFiniteHC(in.MaxHC) {
		return "", fmt.Errorf("rec token: max_hc is not finite (%v)", in.MaxHC)
	}
	if !isFiniteHC(in.RecommendedHC) {
		return "", fmt.Errorf("rec token: recommended_hc is not finite (%v)", in.RecommendedHC)
	}
	for i, s := range in.AllSamples {
		if !isFiniteHC(s.OppHC) {
			return "", fmt.Errorf("rec token: sample[%d].opp_hc is not finite (%v)", i, s.OppHC)
		}
		if !isFiniteHC(s.RackDiff) {
			return "", fmt.Errorf("rec token: sample[%d].rack_diff is not finite (%v)", i, s.RackDiff)
		}
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("rec token: marshal: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// requestHashInput is marshalled to produce the per-request idempotency hash.
// Entries are sorted ascending by PlayerID before marshalling so the hash is
// independent of request entry order; two requests with the same entries in
// different order produce identical hashes.
type requestHashInput struct {
	SeasonID int64              `json:"season_id"`
	Entries  []requestHashEntry `json:"entries"`
}

type requestHashEntry struct {
	PlayerID              int64   `json:"player_id"`
	ExpectedAssignedHC    float64 `json:"expected_assigned_hc"`
	ExpectedRecommendedHC float64 `json:"expected_recommended_hc"`
	RecToken              string  `json:"rec_token"`
}

// computeRequestHash returns a hex-encoded SHA-256 hash of the canonical request payload.
// It sorts entries by PlayerID before marshalling to guarantee order-independence.
// Float inputs are validated for finiteness before marshalling: non-finite values
// (NaN, ±Inf) cause json.Marshal to produce an error whose byte representation
// would be nil, making all non-finite inputs collapse to sha256("").
func computeRequestHash(in requestHashInput) (string, error) {
	entries := make([]requestHashEntry, len(in.Entries))
	copy(entries, in.Entries)
	for i, e := range entries {
		if !isFiniteHC(e.ExpectedAssignedHC) {
			return "", fmt.Errorf("request hash: entries[%d].expected_assigned_hc is not finite (%v)", i, e.ExpectedAssignedHC)
		}
		if !isFiniteHC(e.ExpectedRecommendedHC) {
			return "", fmt.Errorf("request hash: entries[%d].expected_recommended_hc is not finite (%v)", i, e.ExpectedRecommendedHC)
		}
		// Normalize to 0.01 precision so cent-equivalent floats (e.g. 2.50 vs
		// 2.4999999999) produce the same hash and are correctly treated as the
		// same idempotent request.
		entries[i].ExpectedAssignedHC    = math.Round(e.ExpectedAssignedHC*100) / 100
		entries[i].ExpectedRecommendedHC = math.Round(e.ExpectedRecommendedHC*100) / 100
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].PlayerID < entries[j].PlayerID
	})
	canonical := requestHashInput{SeasonID: in.SeasonID, Entries: entries}
	b, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("request hash: marshal: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
