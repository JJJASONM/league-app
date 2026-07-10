package handicaps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"league_app/backend/domainerr"
	"league_app/models"
)

// Domain error codes specific to Apply.
const (
	CodeInvalidRequest = "HC_INVALID_REQUEST" // domainerr.InvalidInput
	CodeMethodNotApply = "HC_METHOD_NOT_APPLY" // domainerr.Unprocessable
)

// ConflictCode identifies the reason an ApplyEntry was rejected due to a
// stale or inconsistent condition that must be resolved before re-applying.
type ConflictCode string

const (
	ConflictTokenMismatch         ConflictCode = "token_mismatch"
	ConflictAssignedHCChanged     ConflictCode = "assigned_hc_changed"
	ConflictRecommendedHCChanged  ConflictCode = "recommended_hc_changed"
	ConflictNotInRoster           ConflictCode = "not_in_roster"
	ConflictConcurrentWrite       ConflictCode = "concurrent_write"
	ConflictIdempotencyKeyReused  ConflictCode = "idempotency_key_reused"
)

// RejectionCode identifies the reason a player entry is ineligible for Apply.
// Rejections are pre-condition failures, not stale-data conflicts.
type RejectionCode string

const (
	RejectionAdminHold       RejectionCode = "admin_hold"
	RejectionBelowThreshold  RejectionCode = "below_threshold"
	RejectionNoData          RejectionCode = "no_data"
	RejectionNoChange        RejectionCode = "no_change"
	RejectionNotEligible     RejectionCode = "not_eligible"
	RejectionMethodNotApply  RejectionCode = "method_not_apply"
)

// ApplyConflict is one conflict entry returned in ApplyResult.Conflicts.
type ApplyConflict struct {
	PlayerID int64        `json:"player_id"`
	Code     ConflictCode `json:"code"`
	Message  string       `json:"message"`
}

// ApplyRejection is one rejection entry returned in ApplyResult.Rejections.
type ApplyRejection struct {
	PlayerID int64         `json:"player_id"`
	Code     RejectionCode `json:"code"`
	Message  string        `json:"message"`
}

// ApplyConflictErr is a domain error carrying a conflict payload.
// Apply returns this as the error when there are unresolved conflicts.
type ApplyConflictErr struct {
	Conflicts []ApplyConflict
}

func (e *ApplyConflictErr) Error() string {
	return fmt.Sprintf("apply: %d conflict(s)", len(e.Conflicts))
}

// ApplyRejectionErr is a domain error carrying a rejection payload.
// Apply returns this when all entries were rejected (no conflicts, no writes).
type ApplyRejectionErr struct {
	Rejections []ApplyRejection
}

func (e *ApplyRejectionErr) Error() string {
	return fmt.Sprintf("apply: %d rejection(s)", len(e.Rejections))
}

// ApplyEntry is one player entry in an Apply request.
// AppliedByUserID is not decoded from the browser request; it is injected from
// auth context in Phase B2. It is nil throughout Phase B1.
type ApplyEntry struct {
	PlayerID              int64
	ExpectedAssignedHC    float64
	ExpectedRecommendedHC float64
	RecToken              string
	AppliedByUserID       *int64
}

// ApplyRequest is the validated domain request for Service.Apply.
// Handler-layer DTOs are decoded separately and converted to this type.
type ApplyRequest struct {
	ApplyRequestID string
	Entries        []ApplyEntry
}

// AppliedChange is one successfully applied handicap update.
type AppliedChange struct {
	PlayerID    int64   `json:"player_id"`
	PlayerName  string  `json:"player_name"`
	OldHandicap float64 `json:"old_handicap"`
	NewHandicap float64 `json:"new_handicap"`
}

// ApplyResult is the successful response from Service.Apply.
type ApplyResult struct {
	ApplyRequestID string          `json:"apply_request_id"`
	Applied        []AppliedChange `json:"applied"`
	Replayed       bool            `json:"replayed,omitempty"`
}

// uuidV4Re validates UUID v4 format: xxxxxxxx-xxxx-4xxx-[89ab]xxx-xxxxxxxxxxxx
var uuidV4Re = regexp.MustCompile(
	`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

// validateApplyRequest performs structural validation before any store access.
func validateApplyRequest(req ApplyRequest) error {
	if req.ApplyRequestID == "" {
		return domainerr.New(CodeInvalidRequest, domainerr.InvalidInput, "apply_request_id is required")
	}
	if !uuidV4Re.MatchString(req.ApplyRequestID) {
		return domainerr.New(CodeInvalidRequest, domainerr.InvalidInput, "apply_request_id must be a UUID v4")
	}
	if len(req.Entries) == 0 {
		return domainerr.New(CodeInvalidRequest, domainerr.InvalidInput, "entries must not be empty")
	}
	seen := make(map[int64]bool, len(req.Entries))
	for i, e := range req.Entries {
		if seen[e.PlayerID] {
			return domainerr.New(CodeInvalidRequest, domainerr.InvalidInput,
				fmt.Sprintf("duplicate player_id %d in entries", e.PlayerID))
		}
		seen[e.PlayerID] = true
		if !isFiniteHC(e.ExpectedAssignedHC) {
			return domainerr.New(CodeInvalidRequest, domainerr.InvalidInput,
				fmt.Sprintf("entry[%d]: expected_assigned_hc must be finite", i))
		}
		if !isFiniteHC(e.ExpectedRecommendedHC) {
			return domainerr.New(CodeInvalidRequest, domainerr.InvalidInput,
				fmt.Sprintf("entry[%d]: expected_recommended_hc must be finite", i))
		}
		if e.RecToken == "" {
			return domainerr.New(CodeInvalidRequest, domainerr.InvalidInput,
				fmt.Sprintf("entry[%d]: rec_token is required", i))
		}
	}
	return nil
}

// roundHC rounds a handicap value to the nearest cent (2 decimal places).
func roundHC(v float64) float64 {
	return math.Round(v*100) / 100
}

// Apply validates and applies a set of handicap changes for the given season.
//
// Error return contract:
//   - *domainerr.Err{Category: InvalidInput}  — structural validation failure (400)
//   - *domainerr.Err{Category: NotFound}      — season not found (404)
//   - *domainerr.Err{Category: Conflict}      — idempotency key reused with different hash (409)
//   - *domainerr.Err{Category: Unprocessable} — method is not game_diff_average (422)
//   - *ApplyConflictErr                       — per-entry conflicts (409 in handler)
//   - *ApplyRejectionErr                      — all entries rejected (422 in handler)
//   - *domainerr.Err{Category: Internal}      — unexpected data or store errors (500)
func (s *Service) Apply(ctx context.Context, seasonID int64, req ApplyRequest) (ApplyResult, error) {
	if err := validateApplyRequest(req); err != nil {
		return ApplyResult{}, err
	}

	// Compute the request hash before entering the write transaction so the hash
	// function's error is surfaced before we acquire any lock.
	hashEntries := make([]requestHashEntry, len(req.Entries))
	for i, e := range req.Entries {
		hashEntries[i] = requestHashEntry{
			PlayerID:              e.PlayerID,
			ExpectedAssignedHC:    e.ExpectedAssignedHC,
			ExpectedRecommendedHC: e.ExpectedRecommendedHC,
			RecToken:              e.RecToken,
		}
	}
	requestHash, err := computeRequestHash(requestHashInput{
		SeasonID: seasonID,
		Entries:  hashEntries,
	})
	if err != nil {
		return ApplyResult{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
	}

	var result ApplyResult

	txErr := s.store.RunWriteTx(ctx, func(tx Store) error {
		// --- Replay check -------------------------------------------------
		prior, err := tx.AppliedChangesByRequestID(ctx, req.ApplyRequestID)
		if err != nil {
			return domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
		}

		switch {
		case len(prior) == 0:
			// Case 1: no prior rows — proceed with fresh apply.

		case len(prior) > 0:
			// Check that all stored hashes match.
			hashes := make(map[string]struct{}, len(prior))
			for _, p := range prior {
				hashes[p.RequestHash] = struct{}{}
			}
			if len(hashes) > 1 {
				// Case 3: inconsistent stored hashes — data integrity failure.
				return domainerr.New(CodeDataError, domainerr.Internal,
					"internal error: inconsistent stored request hashes for this apply_request_id")
			}
			storedHash := prior[0].RequestHash

			if storedHash != requestHash {
				// Case 4: same request ID, different hash — idempotency key reused.
				return domainerr.New(CodeInvalidRequest, domainerr.Conflict,
					"apply_request_id was already used with a different set of changes")
			}

			// Same hash — verify player set matches (Case 2 vs Case 5).
			storedIDs := make(map[int64]struct{}, len(prior))
			for _, p := range prior {
				storedIDs[p.PlayerID] = struct{}{}
			}
			reqIDs := make(map[int64]struct{}, len(req.Entries))
			for _, e := range req.Entries {
				reqIDs[e.PlayerID] = struct{}{}
			}
			if len(storedIDs) != len(reqIDs) {
				// Case 5: same hash, different player set — SHA-256 collision (astronomically unlikely).
				return domainerr.New(CodeDataError, domainerr.Internal,
					"internal error: request hash collision detected")
			}
			for id := range reqIDs {
				if _, ok := storedIDs[id]; !ok {
					return domainerr.New(CodeDataError, domainerr.Internal,
						"internal error: request hash collision detected")
				}
			}

			// Case 2: identical replay — return prior result without re-writing.
			applied := make([]AppliedChange, len(prior))
			for i, p := range prior {
				applied[i] = AppliedChange{
					PlayerID:    p.PlayerID,
					PlayerName:  p.PlayerNameSnapshot,
					OldHandicap: p.OldHandicap,
					NewHandicap: p.NewHandicap,
				}
			}
			result = ApplyResult{
				ApplyRequestID: req.ApplyRequestID,
				Applied:        applied,
				Replayed:       true,
			}
			return nil
		}

		// --- Compute live recommendations ---------------------------------
		liveResp, err := s.compute(ctx, tx, seasonID)
		if err != nil {
			return err
		}

		// Method gate: Apply is only supported for game_diff_average.
		if liveResp.Method != MethodGameDiffAverage {
			return domainerr.New(CodeMethodNotApply, domainerr.Unprocessable,
				fmt.Sprintf("apply is not supported for method %q; use game_diff_average", liveResp.Method))
		}

		// Build a lookup map: playerID → live recommendation.
		liveMap := make(map[int64]models.HandicapReviewRec, len(liveResp.Recommendations))
		for _, rec := range liveResp.Recommendations {
			liveMap[rec.PlayerID] = rec
		}

		// --- Per-entry validation -----------------------------------------
		// Order per PM spec: not_in_roster → admin_hold → ineligible reason →
		//   nil RecommendedHC → token_mismatch → assigned_hc_changed → recommended_hc_changed.
		// All rejections and conflicts are collected before any write.
		var rejections []ApplyRejection
		var conflicts []ApplyConflict

		type writeIntent struct {
			entry   ApplyEntry
			liveRec models.HandicapReviewRec
		}
		intents := make([]writeIntent, 0, len(req.Entries))

		for _, e := range req.Entries {
			liveRec, inRoster := liveMap[e.PlayerID]
			if !inRoster {
				conflicts = append(conflicts, ApplyConflict{
					PlayerID: e.PlayerID,
					Code:     ConflictNotInRoster,
					Message:  fmt.Sprintf("player %d is not in the season roster", e.PlayerID),
				})
				continue
			}

			// Rejection checks.
			if liveRec.AdminHold {
				rejections = append(rejections, ApplyRejection{
					PlayerID: e.PlayerID,
					Code:     RejectionAdminHold,
					Message:  "player is on admin hold; remove hold before applying",
				})
				continue
			}

			switch liveRec.Reason {
			case ReasonNoData:
				rejections = append(rejections, ApplyRejection{
					PlayerID: e.PlayerID, Code: RejectionNoData,
					Message: "player has no eligible rack data",
				})
				continue
			case ReasonBelowThreshold:
				rejections = append(rejections, ApplyRejection{
					PlayerID: e.PlayerID, Code: RejectionBelowThreshold,
					Message: "player has not met the minimum rack threshold",
				})
				continue
			case ReasonNoChange:
				rejections = append(rejections, ApplyRejection{
					PlayerID: e.PlayerID, Code: RejectionNoChange,
					Message: "recommended handicap equals current handicap; no change to apply",
				})
				continue
			case "", ReasonCapped:
				// Actionable: fall through to conflict checks.
			default:
				rejections = append(rejections, ApplyRejection{
					PlayerID: e.PlayerID, Code: RejectionNotEligible,
					Message: fmt.Sprintf("player is not eligible for apply (reason: %s)", liveRec.Reason),
				})
				continue
			}

			// Nil RecommendedHC check (should not happen for actionable, but guard for safety).
			if liveRec.RecommendedHC == nil {
				rejections = append(rejections, ApplyRejection{
					PlayerID: e.PlayerID, Code: RejectionNotEligible,
					Message: "live recommendation has no recommended value",
				})
				continue
			}

			// Conflict checks.
			if liveRec.RecToken != e.RecToken {
				conflicts = append(conflicts, ApplyConflict{
					PlayerID: e.PlayerID,
					Code:     ConflictTokenMismatch,
					Message:  "recommendation inputs have changed since this token was issued; refresh and retry",
				})
				continue
			}

			if roundHC(liveRec.AssignedHC) != roundHC(e.ExpectedAssignedHC) {
				conflicts = append(conflicts, ApplyConflict{
					PlayerID: e.PlayerID,
					Code:     ConflictAssignedHCChanged,
					Message:  fmt.Sprintf("assigned handicap changed from %.2f to %.2f; refresh and retry", e.ExpectedAssignedHC, liveRec.AssignedHC),
				})
				continue
			}

			if roundHC(*liveRec.RecommendedHC) != roundHC(e.ExpectedRecommendedHC) {
				conflicts = append(conflicts, ApplyConflict{
					PlayerID: e.PlayerID,
					Code:     ConflictRecommendedHCChanged,
					Message:  fmt.Sprintf("recommended handicap changed from %.2f to %.2f; refresh and retry", e.ExpectedRecommendedHC, *liveRec.RecommendedHC),
				})
				continue
			}

			intents = append(intents, writeIntent{entry: e, liveRec: liveRec})
		}

		// All-or-nothing: rejections first, then conflicts.
		if len(rejections) > 0 {
			return &ApplyRejectionErr{Rejections: rejections}
		}
		if len(conflicts) > 0 {
			return &ApplyConflictErr{Conflicts: conflicts}
		}

		// --- Write phase --------------------------------------------------
		effectiveDate := time.Now().UTC().Format("2006-01-02")
		applied := make([]AppliedChange, 0, len(intents))

		for _, intent := range intents {
			e := intent.entry
			liveRec := intent.liveRec
			newHC := *liveRec.RecommendedHC

			updated, err := tx.UpdatePlayerHandicap(ctx, e.PlayerID, newHC, liveRec.AssignedHC)
			if err != nil {
				return domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
			}
			if !updated {
				// The conditional update failed inside the write transaction — the
				// assigned HC changed between our read and write in this very tx.
				// This should be extremely rare (requires concurrent writer inside
				// the same BEGIN IMMEDIATE, which SQLite prevents — but guard for safety).
				return domainerr.New(CodeDataError, domainerr.Internal,
					fmt.Sprintf("optimistic update failed for player %d inside write transaction", e.PlayerID))
			}

			row := HandicapHistoryRow{
				PlayerID:           e.PlayerID,
				PlayerNameSnapshot: liveRec.PlayerName,
				OldHandicap:        liveRec.AssignedHC,
				NewHandicap:        newHC,
				EffectiveDate:      effectiveDate,
				AdminHold:          0,
				ApplyRequestID:     req.ApplyRequestID,
				RequestHash:        requestHash,
				SeasonID:           seasonID,
				Method:             MethodGameDiffAverage,
				WindowSize:         liveRec.WindowSize,
				WindowRacks:        liveRec.WindowRacks,
				LifetimeRacks:      liveRec.LifetimeRacks,
				RecToken:           liveRec.RecToken,
				AppliedByUserID:    e.AppliedByUserID,
			}
			if err := tx.InsertHandicapHistory(ctx, row); err != nil {
				return domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
			}

			applied = append(applied, AppliedChange{
				PlayerID:    e.PlayerID,
				PlayerName:  liveRec.PlayerName,
				OldHandicap: liveRec.AssignedHC,
				NewHandicap: newHC,
			})
		}

		result = ApplyResult{
			ApplyRequestID: req.ApplyRequestID,
			Applied:        applied,
		}
		return nil
	})

	if txErr != nil {
		// Unwrap error types in priority order.
		var conflictErr *ApplyConflictErr
		if errors.As(txErr, &conflictErr) {
			return ApplyResult{}, conflictErr
		}
		var rejectionErr *ApplyRejectionErr
		if errors.As(txErr, &rejectionErr) {
			return ApplyResult{}, rejectionErr
		}
		if errors.Is(txErr, ErrConcurrentWrite) {
			return ApplyResult{}, &ApplyConflictErr{Conflicts: []ApplyConflict{{
				Code:    ConflictConcurrentWrite,
				Message: "another request is applying handicap changes; retry in a moment",
			}}}
		}
		var de *domainerr.Err
		if errors.As(txErr, &de) {
			return ApplyResult{}, txErr
		}
		return ApplyResult{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", txErr)
	}

	return result, nil
}
