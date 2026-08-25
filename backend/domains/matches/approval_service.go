package matches

import (
	"context"
	"fmt"

	"league_app/backend/domainerr"
)

// Weekly Score Processing Phase 1A error codes. See doc/domains/matches/README.md.
const (
	// CodeMatchNotFound fires when the given match ID does not exist.
	CodeMatchNotFound = "MATCH_NOT_FOUND"

	// CodeMatchNotScored fires when Approve is attempted on a match with completed=0.
	CodeMatchNotScored = "MATCH_NOT_SCORED"

	// CodeMatchAlreadyProcessed fires when Approve or Unapprove is attempted
	// on a match that already has processed_at set.
	CodeMatchAlreadyProcessed = "MATCH_ALREADY_PROCESSED"

	// CodeMatchNotApproved fires when Process is attempted on a match with
	// approved_at IS NULL.
	CodeMatchNotApproved = "MATCH_NOT_APPROVED"

	// CodeMatchApproved fires when a normal score edit (SaveRounds,
	// SubmitResults, ClearResults) is attempted on an approved match.
	CodeMatchApproved = "MATCH_APPROVED"

	// CodeMatchProcessed fires when a normal score edit is attempted on a
	// processed match.
	CodeMatchProcessed = "MATCH_PROCESSED"
)

// checkMatchEditable returns a domainerr.Conflict when the match is approved
// or processed, blocking normal score edits (SaveRounds, SubmitResults,
// ClearResults). Processed is checked first since it is the stricter state.
// Returns nil when the match does not exist so the caller's own not-found
// handling (if any) still applies; existing callers do not currently check
// existence before editing, so this preserves that behavior.
func checkMatchEditable(ctx context.Context, store RoundStore, matchID int64) error {
	state, err := store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("check match editable: %w", err)
	}
	if !state.Exists {
		return nil
	}
	if state.ProcessedAt != nil {
		return domainerr.New(CodeMatchProcessed, domainerr.Conflict,
			"match scores are processed; unprocess before editing")
	}
	if state.ApprovedAt != nil {
		return domainerr.New(CodeMatchApproved, domainerr.Conflict,
			"match scores are approved; unapprove before editing")
	}
	return nil
}

// ApproveMatch records admin-attested approval of a match's scores.
// approvedByUserID is the resolved personal-key user's ID, or nil when
// approved via a credential that does not resolve to a user record.
//
// Validation order: match exists -> season not closed -> week not closed ->
// match completed -> not already processed.
func (s *RoundService) ApproveMatch(ctx context.Context, matchID int64, approvedByUserID *int64, note string) error {
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("approve match: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	if sc, err := s.store.IsSeasonClosedForMatch(ctx, matchID); err != nil {
		return fmt.Errorf("approve match: season-closed check: %w", err)
	} else if sc {
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	}
	if closed, err := s.store.IsWeekClosed(ctx, matchID); err != nil {
		return fmt.Errorf("approve match: week-closed check: %w", err)
	} else if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	}
	if !state.Completed {
		return domainerr.New(CodeMatchNotScored, domainerr.Unprocessable,
			"match has no saved scores; enter scores before approving")
	}
	if state.ProcessedAt != nil {
		return domainerr.New(CodeMatchAlreadyProcessed, domainerr.Conflict,
			"match is already processed; unprocess before re-approving")
	}
	if err := s.store.ApproveMatch(ctx, matchID, approvedByUserID, note); err != nil {
		return fmt.Errorf("approve match: %w", err)
	}
	return nil
}

// ProcessMatch records that an approved match's results are official enough
// to count toward handicap recommendation eligibility, ahead of the full
// week closing. Does not write handicap_history and does not itself change
// any player's handicap -- Handicap Apply remains the only writer of
// handicap_history.
//
// Validation order: match exists -> season not closed -> week not closed ->
// approved_at IS NOT NULL.
func (s *RoundService) ProcessMatch(ctx context.Context, matchID int64, processedByUserID *int64) error {
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("process match: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	if sc, err := s.store.IsSeasonClosedForMatch(ctx, matchID); err != nil {
		return fmt.Errorf("process match: season-closed check: %w", err)
	} else if sc {
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	}
	if closed, err := s.store.IsWeekClosed(ctx, matchID); err != nil {
		return fmt.Errorf("process match: week-closed check: %w", err)
	} else if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	}
	if state.ApprovedAt == nil {
		return domainerr.New(CodeMatchNotApproved, domainerr.Unprocessable,
			"match has not been approved; approve before processing")
	}
	if err := s.store.ProcessMatch(ctx, matchID, processedByUserID); err != nil {
		return fmt.Errorf("process match: %w", err)
	}
	return nil
}

// UnapproveMatch clears a match's approval, used as an admin correction path
// before scores can be edited again. Rejected when the match is already
// processed (unprocess first) or the week is closed (reopen first).
//
// Validation order: match exists -> season not closed -> week not closed ->
// not already processed.
func (s *RoundService) UnapproveMatch(ctx context.Context, matchID int64) error {
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("unapprove match: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	if sc, err := s.store.IsSeasonClosedForMatch(ctx, matchID); err != nil {
		return fmt.Errorf("unapprove match: season-closed check: %w", err)
	} else if sc {
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	}
	if closed, err := s.store.IsWeekClosed(ctx, matchID); err != nil {
		return fmt.Errorf("unapprove match: week-closed check: %w", err)
	} else if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	}
	if state.ProcessedAt != nil {
		return domainerr.New(CodeMatchAlreadyProcessed, domainerr.Conflict,
			"match is already processed; unprocess before unapproving")
	}
	if err := s.store.UnapproveMatch(ctx, matchID); err != nil {
		return fmt.Errorf("unapprove match: %w", err)
	}
	return nil
}

// UnprocessMatch clears a match's processed state, used as an admin
// correction path. Leaves approval intact -- the admin can separately
// unapprove afterward if the correction requires editing scores.
//
// Validation order: match exists -> season not closed -> week not closed.
func (s *RoundService) UnprocessMatch(ctx context.Context, matchID int64) error {
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("unprocess match: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	if sc, err := s.store.IsSeasonClosedForMatch(ctx, matchID); err != nil {
		return fmt.Errorf("unprocess match: season-closed check: %w", err)
	} else if sc {
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	}
	if closed, err := s.store.IsWeekClosed(ctx, matchID); err != nil {
		return fmt.Errorf("unprocess match: week-closed check: %w", err)
	} else if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	}
	if err := s.store.UnprocessMatch(ctx, matchID); err != nil {
		return fmt.Errorf("unprocess match: %w", err)
	}
	return nil
}
