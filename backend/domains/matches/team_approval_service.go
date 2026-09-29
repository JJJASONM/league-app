package matches

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// ApprovalActor identifies who is performing a team-approval action.
// PlayerID is required for ApproveTeamSide, WithdrawTeamApproval, and
// RequestCorrection -- these are inherently player-side actions, and
// eligibility is always evaluated against a linked player, never against a
// bare user identity. UserID and NameSnapshot are attribution only,
// recorded on the resulting state and event but never used to decide
// eligibility.
type ApprovalActor struct {
	UserID       *int64
	PlayerID     *int64
	NameSnapshot string
}

func isValidSide(side string) bool {
	return side == ApprovalSideHome || side == ApprovalSideAway
}

// sideApprovedAtRevision reports whether side counts as a valid approval
// for expectedRevision: the state label alone is not sufficient -- a side
// labeled "approved" whose StateScoreRevision is nil or does not equal
// expectedRevision describes a stale or otherwise invalid approval and
// must not count. Both BothSidesApproved and AdminAcceptMatch use this
// exact rule so the two can never disagree about what "approved" means.
func sideApprovedAtRevision(side TeamSideApprovalState, expectedRevision int) bool {
	return side.State == ApprovalStateApproved &&
		side.StateScoreRevision != nil &&
		*side.StateScoreRevision == expectedRevision
}

// mapTeamActionGuardErr translates a sentinel error returned by the SQLite
// store's transaction-scoped team-approval write methods (ApproveTeamSide,
// WithdrawTeamApproval, RequestCorrection) into the matching domainerr. Any
// other error is wrapped as-is. This is the AUTHORITATIVE mapping -- the
// store's conditional UPDATE re-verifies every one of these conditions
// atomically at write time, so this covers a real race, not just whatever
// the service layer's own (non-authoritative, fast-fail only) reads above
// already saw.
func mapTeamActionGuardErr(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrGuardMatchNotFound):
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	case errors.Is(err, ErrApprovalRevisionStale):
		return domainerr.New(CodeApprovalRevisionStale, domainerr.Conflict,
			"score revision is stale; reload the match before making this change")
	case errors.Is(err, ErrGuardNotScored):
		return domainerr.New(CodeMatchNotScored, domainerr.Unprocessable,
			"match has no saved scores; enter scores before approving")
	case errors.Is(err, ErrGuardSeasonClosed):
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict,
			"season is closed; this action is not allowed")
	case errors.Is(err, ErrGuardWeekClosed):
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; this action is not allowed")
	case errors.Is(err, ErrGuardProcessed):
		return domainerr.New(CodeMatchProcessed, domainerr.Conflict,
			"match is processed; team approval actions are no longer available")
	case errors.Is(err, ErrGuardAdminAccepted):
		return domainerr.New(CodeMatchAdminAccepted, domainerr.Conflict,
			"match has been accepted by an administrator; team approval actions are no longer available")
	case errors.Is(err, ErrGuardInvalidTransition):
		return domainerr.New(CodeApprovalInvalidTransition, domainerr.Conflict,
			"this side is not currently approved")
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

// ApproveTeamSide records that an eligible player has approved the given
// side's scores at expectedRevision. Eligibility (RoundStore.
// ResolveApprovalEligibility) requires either current season-roster
// membership on that side's team or actual round_results participation on
// that side for this specific match -- never a captain requirement, and no
// requirement to have personally played when rostered.
//
// Fast-fail order (non-authoritative; informs eligibility resolution and
// gives a friendly error for the common case): side valid -> actor has a
// linked player -> match exists -> player eligible. The store write that
// follows is the AUTHORITATIVE guard: one conditional UPDATE atomically
// re-verifies match exists, expected revision, completed, season open,
// week open, not admin-accepted, and not processed, all at write time --
// see RoundStore.ApproveTeamSide.
func (s *RoundService) ApproveTeamSide(ctx context.Context, matchID int64, side string, expectedRevision int, actor ApprovalActor) error {
	if !isValidSide(side) {
		return domainerr.New(CodeApprovalInvalidSide, domainerr.InvalidInput, "side must be \"home\" or \"away\"")
	}
	if actor.PlayerID == nil {
		return domainerr.New(CodeApprovalIneligiblePlayer, domainerr.Unprocessable,
			"no linked player to evaluate approval eligibility")
	}
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("approve team side: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	eligible, err := s.store.ResolveApprovalEligibility(ctx, matchID, side, *actor.PlayerID)
	if err != nil {
		return fmt.Errorf("approve team side: %w", err)
	}
	if !eligible {
		return domainerr.New(CodeApprovalIneligiblePlayer, domainerr.Unprocessable,
			"player is not eligible to approve for this team side")
	}
	err = s.store.ApproveTeamSide(ctx, matchID, side, expectedRevision, actor.UserID, actor.PlayerID, actor.NameSnapshot)
	return mapTeamActionGuardErr("approve team side", err)
}

// WithdrawTeamApproval reverts the given side's state from "approved" back
// to "pending" for expectedRevision. Any player currently eligible for that
// side may withdraw it -- not only the player who originally approved --
// since the approval belongs to the team side, not to one individual.
//
// Fast-fail order mirrors ApproveTeamSide. The store write's conditional
// UPDATE additionally re-verifies, atomically, that the side is still
// "approved" -- see RoundStore.WithdrawTeamApproval.
func (s *RoundService) WithdrawTeamApproval(ctx context.Context, matchID int64, side string, expectedRevision int, actor ApprovalActor) error {
	if !isValidSide(side) {
		return domainerr.New(CodeApprovalInvalidSide, domainerr.InvalidInput, "side must be \"home\" or \"away\"")
	}
	if actor.PlayerID == nil {
		return domainerr.New(CodeApprovalIneligiblePlayer, domainerr.Unprocessable,
			"no linked player to evaluate approval eligibility")
	}
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("withdraw team approval: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	eligible, err := s.store.ResolveApprovalEligibility(ctx, matchID, side, *actor.PlayerID)
	if err != nil {
		return fmt.Errorf("withdraw team approval: %w", err)
	}
	if !eligible {
		return domainerr.New(CodeApprovalIneligiblePlayer, domainerr.Unprocessable,
			"player is not eligible to act for this team side")
	}
	err = s.store.WithdrawTeamApproval(ctx, matchID, side, expectedRevision, actor.UserID, actor.PlayerID, actor.NameSnapshot)
	return mapTeamActionGuardErr("withdraw team approval", err)
}

// RequestCorrection records that an eligible player believes the given
// side's scores need correction, at expectedRevision, with a required
// note. This is allowed from either "pending" or "approved" (it supersedes
// an existing approval for that side only -- the other side's state is
// untouched) but not once an admin has accepted or processed the match.
// Requesting a correction on an unscored match is rejected the same way
// approving one is -- there is nothing yet to correct.
//
// Fast-fail order mirrors ApproveTeamSide. The store write's conditional
// UPDATE is the authoritative guard -- see RoundStore.RequestCorrection.
func (s *RoundService) RequestCorrection(ctx context.Context, matchID int64, side string, expectedRevision int, note string, actor ApprovalActor) error {
	if !isValidSide(side) {
		return domainerr.New(CodeApprovalInvalidSide, domainerr.InvalidInput, "side must be \"home\" or \"away\"")
	}
	if strings.TrimSpace(note) == "" {
		return domainerr.New(CodeApprovalNoteRequired, domainerr.InvalidInput, "a note explaining the requested correction is required")
	}
	if actor.PlayerID == nil {
		return domainerr.New(CodeApprovalIneligiblePlayer, domainerr.Unprocessable,
			"no linked player to evaluate approval eligibility")
	}
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("request correction: %w", err)
	}
	if !state.Exists {
		return domainerr.New(CodeMatchNotFound, domainerr.NotFound, "match not found")
	}
	eligible, err := s.store.ResolveApprovalEligibility(ctx, matchID, side, *actor.PlayerID)
	if err != nil {
		return fmt.Errorf("request correction: %w", err)
	}
	if !eligible {
		return domainerr.New(CodeApprovalIneligiblePlayer, domainerr.Unprocessable,
			"player is not eligible to act for this team side")
	}
	err = s.store.RequestCorrection(ctx, matchID, side, expectedRevision, note, actor.UserID, actor.PlayerID, actor.NameSnapshot)
	return mapTeamActionGuardErr("request correction", err)
}

// GetApprovalSnapshot returns matchID's full current approval state (both
// sides plus score_revision) for read-only display, eligibility decisions,
// and Phase 1B's future override-confirmation payload. Exists is false
// when the match does not exist.
func (s *RoundService) GetApprovalSnapshot(ctx context.Context, matchID int64) (MatchApprovalState, error) {
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return MatchApprovalState{}, fmt.Errorf("get approval snapshot: %w", err)
	}
	return state, nil
}

// ListApprovalEvents returns the full match_approval_events history for
// matchID, oldest first. Returns a non-nil empty slice when no events
// exist or the match does not exist.
func (s *RoundService) ListApprovalEvents(ctx context.Context, matchID int64) ([]models.MatchApprovalEvent, error) {
	events, err := s.store.ListApprovalEvents(ctx, matchID)
	if err != nil {
		return nil, fmt.Errorf("list approval events: %w", err)
	}
	if events == nil {
		events = []models.MatchApprovalEvent{}
	}
	return events, nil
}

// BothSidesApproved reports whether both team sides are "approved" at the
// match's current score_revision -- the precondition AdminAcceptMatch uses
// to decide between a normal "admin_accepted" event and one requiring
// override.
func (s *RoundService) BothSidesApproved(ctx context.Context, matchID int64) (bool, error) {
	state, err := s.store.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return false, fmt.Errorf("both sides approved: %w", err)
	}
	return sideApprovedAtRevision(state.HomeSide, state.ScoreRevision) &&
		sideApprovedAtRevision(state.AwaySide, state.ScoreRevision), nil
}

// ResolveApprovalEligibility exposes RoundStore.ResolveApprovalEligibility
// at the service layer so Phase 1B's authorization scope resolvers can
// reuse the exact same eligibility rule this package enforces internally,
// rather than re-implementing it.
func (s *RoundService) ResolveApprovalEligibility(ctx context.Context, matchID int64, side string, playerID int64) (bool, error) {
	if !isValidSide(side) {
		return false, domainerr.New(CodeApprovalInvalidSide, domainerr.InvalidInput, "side must be \"home\" or \"away\"")
	}
	eligible, err := s.store.ResolveApprovalEligibility(ctx, matchID, side, playerID)
	if err != nil {
		return false, fmt.Errorf("resolve approval eligibility: %w", err)
	}
	return eligible, nil
}
