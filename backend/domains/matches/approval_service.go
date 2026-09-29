package matches

import (
	"context"
	"errors"
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

// Player Score Approval Phase 1A error codes. See doc/domains/matches/README.md.
const (
	// CodeApprovalRevisionStale fires when a caller's expected score
	// revision no longer matches the match's current score_revision --
	// either on a score-editing route that opted in by supplying one, or
	// on any team-approval action, which always requires one.
	CodeApprovalRevisionStale = "APPROVAL_REVISION_STALE"

	// CodeApprovalIneligiblePlayer fires when the acting identity has no
	// linked player, or that player does not satisfy
	// RoundStore.ResolveApprovalEligibility for the given side.
	CodeApprovalIneligiblePlayer = "APPROVAL_INELIGIBLE_PLAYER"

	// CodeApprovalInvalidSide fires when a caller supplies a side other
	// than "home" or "away".
	CodeApprovalInvalidSide = "APPROVAL_INVALID_SIDE"

	// CodeApprovalInvalidTransition fires when a team-approval action is
	// attempted from a state it does not apply to -- e.g. withdrawing a
	// side that is not currently "approved".
	CodeApprovalInvalidTransition = "APPROVAL_INVALID_TRANSITION"

	// CodeApprovalNoteRequired fires when RequestCorrection is called with
	// an empty or whitespace-only note.
	CodeApprovalNoteRequired = "APPROVAL_NOTE_REQUIRED"

	// CodeApprovalOverrideRequired fires when AdminAcceptMatch is called
	// without override=true while one or both team sides are not
	// "approved" at the match's current score revision.
	CodeApprovalOverrideRequired = "APPROVAL_OVERRIDE_REQUIRED"

	// CodeMatchAdminAccepted fires when a team-approval action (approve,
	// withdraw, request correction) is attempted after an admin has
	// already accepted the match (approved_at is set) -- distinct from
	// CodeMatchApproved, which fires for a normal score-edit attempt on
	// the same state, since the two callers need different guidance.
	CodeMatchAdminAccepted = "MATCH_ADMIN_ACCEPTED"

	// CodeMatchAlreadyAccepted fires when AdminAcceptMatch's atomic guard
	// finds approved_at already set at write time -- a second admin
	// accepted the same match in the narrow window between this call's
	// own decision and its write. Distinct from CodeMatchAdminAccepted
	// (which is about a team action colliding with an existing acceptance)
	// and from CodeMatchApproved (a score-edit colliding with one).
	CodeMatchAlreadyAccepted = "MATCH_ALREADY_ACCEPTED"

	// CodeApprovalTeamStateChanged fires when AdminAcceptMatch's atomic
	// guard finds either team side's approval_state no longer matches the
	// state its accept-vs-override decision was based on.
	CodeApprovalTeamStateChanged = "APPROVAL_TEAM_STATE_CHANGED"
)

// mapScoreEditGuardErr translates a sentinel error from
// RoundStore.AdvanceScoreRevisionForEdit into the matching domainerr, for
// SaveRounds/SubmitResults/ClearResults. Any other error is wrapped as-is.
func mapScoreEditGuardErr(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrGuardMatchNotFound):
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	case errors.Is(err, ErrApprovalRevisionStale):
		return domainerr.New(CodeApprovalRevisionStale, domainerr.Conflict,
			"score revision is stale; reload the match before making this change")
	case errors.Is(err, ErrGuardSeasonClosed):
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	case errors.Is(err, ErrGuardWeekClosed):
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	case errors.Is(err, ErrGuardProcessed):
		return domainerr.New(CodeMatchProcessed, domainerr.Conflict,
			"match scores are processed; unprocess before editing")
	case errors.Is(err, ErrGuardAdminAccepted):
		return domainerr.New(CodeMatchApproved, domainerr.Conflict,
			"match scores are approved; unapprove before editing")
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

// mapAdminAcceptGuardErr translates a sentinel error from
// RoundStore.GuardedAdminAccept into the matching domainerr.
func mapAdminAcceptGuardErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrGuardMatchNotFound):
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	case errors.Is(err, ErrApprovalRevisionStale):
		return domainerr.New(CodeApprovalRevisionStale, domainerr.Conflict,
			"score revision is stale; reload the match before accepting")
	case errors.Is(err, ErrGuardNotScored):
		return domainerr.New(CodeMatchNotScored, domainerr.Unprocessable,
			"match has no saved scores; enter scores before approving")
	case errors.Is(err, ErrGuardSeasonClosed):
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	case errors.Is(err, ErrGuardWeekClosed):
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	case errors.Is(err, ErrGuardProcessed):
		return domainerr.New(CodeMatchAlreadyProcessed, domainerr.Conflict,
			"match is already processed; unprocess before re-approving")
	case errors.Is(err, ErrGuardAdminAccepted):
		return domainerr.New(CodeMatchAlreadyAccepted, domainerr.Conflict,
			"match has already been accepted by an administrator")
	case errors.Is(err, ErrGuardTeamStateChanged):
		return domainerr.New(CodeApprovalTeamStateChanged, domainerr.Conflict,
			"team approval state changed since this decision was made; reload and try again")
	default:
		return fmt.Errorf("admin accept match: %w", err)
	}
}

// AdminAcceptMatch is the Phase 1B-ready admin acceptance/override
// primitive for Player Score Approval. It is NOT wired to any HTTP route in
// Phase 1A -- the existing ApproveMatch method below and its
// POST /api/matches/{id}/approve route are completely unchanged and remain
// the only externally reachable admin-approval path this phase. Phase 1B
// will switch the handler to this method once its request contract carries
// an explicit override flag and an expected revision.
//
// An initial read decides what to do: whether both team sides are
// "approved" at the current revision (records an "admin_accepted" event) or
// not (override must be true, or this returns CodeApprovalOverrideRequired
// with zero writes; records an "admin_override" event capturing a
// structured snapshot of both sides' state and note). That decision is then
// re-verified atomically: RoundStore.GuardedAdminAccept performs one
// conditional UPDATE, inside the same transaction as the event write, that
// pins expectedRevision and the exact home/away states this decision was
// based on alongside completed/season-open/week-open/approved_at-IS-NULL/
// processed_at-IS-NULL -- so a score edit, team-state change, process, week
// close, or season close racing this call between the read and the write is
// caught and rejected (zero writes; the event and the acceptance either
// both commit or neither does, since both happen in one transaction). A
// failure at either the event write or the guarded write rolls back the
// other, whichever ran first.
//
// Fast-fail validation order (informs the decision above; the atomic guard
// is the real protection): match exists -> revision matches -> season not
// closed -> week not closed -> match completed -> not already processed ->
// not already accepted.
func (s *RoundService) AdminAcceptMatch(ctx context.Context, matchID int64, approvedByUserID *int64, note string, override bool, expectedRevision int) error {
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("admin accept match: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	if expectedRevision != state.ScoreRevision {
		return domainerr.New(CodeApprovalRevisionStale, domainerr.Conflict,
			"score revision is stale; reload the match before accepting")
	}
	if sc, err := s.store.IsSeasonClosedForMatch(ctx, matchID); err != nil {
		return fmt.Errorf("admin accept match: season-closed check: %w", err)
	} else if sc {
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	}
	if closed, err := s.store.IsWeekClosed(ctx, matchID); err != nil {
		return fmt.Errorf("admin accept match: week-closed check: %w", err)
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
	if state.ApprovedAt != nil {
		return domainerr.New(CodeMatchAlreadyAccepted, domainerr.Conflict,
			"match has already been accepted by an administrator")
	}

	// PM correction: a side counts as approved only when its state label
	// is "approved" AND its StateScoreRevision equals expectedRevision --
	// the same rule BothSidesApproved applies (sideApprovedAtRevision), so
	// the two can never disagree about what "approved" means. A side
	// labeled "approved" with a nil or stale StateScoreRevision must not
	// let normal acceptance proceed; override remains available.
	bothApproved := sideApprovedAtRevision(state.HomeSide, expectedRevision) &&
		sideApprovedAtRevision(state.AwaySide, expectedRevision)
	eventType := "admin_accepted"
	if !bothApproved {
		if !override {
			return domainerr.New(CodeApprovalOverrideRequired, domainerr.Conflict,
				"one or both team approvals are missing; pass override=true to accept anyway")
		}
		eventType = "admin_override"
	}
	// pinnedHome/pinnedAway carry the COMPLETE decision snapshot -- state,
	// state-score-revision (including nil), and correction note -- not
	// just the state label. GuardedAdminAccept re-verifies all three
	// atomically, and the event below is written from these same pinned
	// values, so the guarded write and the event always describe one
	// consistent snapshot: a correction note rewritten at the same
	// revision with the same label would otherwise slip past a
	// label-only guard while being recorded with a stale note.
	pinnedHome, pinnedAway := state.HomeSide, state.AwaySide

	return s.store.RunTx(ctx, func(tx RoundStore) error {
		if err := tx.GuardedAdminAccept(ctx, matchID, expectedRevision, pinnedHome, pinnedAway, approvedByUserID, note); err != nil {
			return mapAdminAcceptGuardErr(err)
		}
		if err := tx.RecordAdminAcceptanceEvent(ctx, AdminAcceptanceEvent{
			MatchID:       matchID,
			EventType:     eventType,
			ActorUserID:   approvedByUserID,
			Note:          note,
			ScoreRevision: expectedRevision,
			HomeState:     pinnedHome.State,
			HomeNote:      pinnedHome.CorrectionNote,
			AwayState:     pinnedAway.State,
			AwayNote:      pinnedAway.CorrectionNote,
		}); err != nil {
			return fmt.Errorf("admin accept match: record event: %w", err)
		}
		return nil
	})
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
