package matches

import (
	"context"
	"strings"

	"league_app/backend/domainerr"
	"league_app/models"
)

// MatchLockChecker is the subset of RoundStore's read-only lock checks
// LineupService needs to enforce the same approval/processing/week-closed/
// season-closed guards RoundService enforces for scoring (Substitute
// Workflow Phase 1) -- swapping in a substitute changes who is credited for
// a match just like a score edit would, so it must respect the same locks.
// RoundStore already satisfies this interface structurally; callers pass
// the same RoundStore instance already constructed for RoundService rather
// than building a second one.
type MatchLockChecker interface {
	IsSeasonClosedForMatch(ctx context.Context, matchID int64) (bool, error)
	IsWeekClosed(ctx context.Context, matchID int64) (bool, error)
	GetMatchApprovalState(ctx context.Context, matchID int64) (MatchApprovalState, error)
}

// LineupService provides lineup plan read and write operations.
type LineupService struct {
	store      LineupStore
	matchLocks MatchLockChecker
}

// NewLineupService returns a LineupService backed by the given store and
// match-lock checker.
func NewLineupService(store LineupStore, matchLocks MatchLockChecker) *LineupService {
	return &LineupService{store: store, matchLocks: matchLocks}
}

// ListLineupPlans returns lineup plans filtered by the request.
// Returns an empty (non-nil) slice when no plans exist.
func (s *LineupService) ListLineupPlans(ctx context.Context, req ListLineupPlansRequest) ([]models.LineupPlan, error) {
	plans, err := s.store.ListLineupPlans(ctx, req)
	if err != nil {
		return nil, domainerr.New("LINEUP_LIST_FAILED", domainerr.Internal, "list lineup plans failed")
	}
	if plans == nil {
		plans = []models.LineupPlan{}
	}
	return plans, nil
}

// SaveTeamLineup atomically replaces all lineup slots for one team/week.
func (s *LineupService) SaveTeamLineup(ctx context.Context, req SaveLineupRequest) error {
	if err := s.store.SaveTeamLineup(ctx, req); err != nil {
		return domainerr.New("LINEUP_SAVE_FAILED", domainerr.Internal, "save lineup failed")
	}
	return nil
}

// DeleteLineupPlan removes a lineup plan by ID. Deleting a non-existent plan is not an error.
func (s *LineupService) DeleteLineupPlan(ctx context.Context, id int64) error {
	if err := s.store.DeleteLineupPlan(ctx, id); err != nil {
		return domainerr.New("LINEUP_DELETE_FAILED", domainerr.Internal, "delete lineup plan failed")
	}
	return nil
}

// SetSubstitute replaces a lineup slot's player with a substitute, recording
// the original player via sub_for_id. Rejects the change when the match for
// this team/week is season-closed, week-closed, approved, or processed --
// the same lock set RoundService enforces for score edits, since a
// substitute swap changes who is credited for a match just as much as a
// score edit would.
func (s *LineupService) SetSubstitute(ctx context.Context, req SetSubstituteRequest) (models.LineupPlan, error) {
	if req.SubstitutePlayerID == 0 {
		return models.LineupPlan{}, domainerr.New("SUB_PLAYER_REQUIRED", domainerr.InvalidInput, "substitute_player_id is required")
	}
	plan, err := s.store.GetLineupPlan(ctx, req.LineupPlanID)
	if err != nil {
		return models.LineupPlan{}, err
	}
	if req.SubstitutePlayerID == plan.PlayerID {
		return models.LineupPlan{}, domainerr.New("SUB_SAME_PLAYER", domainerr.InvalidInput, "substitute must be a different player than the one currently in this slot")
	}
	if err := s.checkEditable(ctx, plan.SeasonID, plan.TeamID, int64(plan.WeekNumber)); err != nil {
		return models.LineupPlan{}, err
	}
	updated, err := s.store.SetSubstitute(ctx, SetSubstituteRequest{
		LineupPlanID:       req.LineupPlanID,
		SubstitutePlayerID: req.SubstitutePlayerID,
		OriginalPlayerID:   plan.PlayerID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return models.LineupPlan{}, domainerr.New("SUB_ALREADY_IN_LINEUP", domainerr.Conflict, "that player is already in this team's lineup for this week")
		}
		return models.LineupPlan{}, domainerr.New("LINEUP_SUB_SET_FAILED", domainerr.Internal, "set substitute failed")
	}
	return updated, nil
}

// ClearSubstitute reverts a substituted slot back to its original player.
// Subject to the same lock checks as SetSubstitute.
func (s *LineupService) ClearSubstitute(ctx context.Context, id int64) (models.LineupPlan, error) {
	plan, err := s.store.GetLineupPlan(ctx, id)
	if err != nil {
		return models.LineupPlan{}, err
	}
	if !plan.IsSub {
		return models.LineupPlan{}, domainerr.New("SUB_NOT_ACTIVE", domainerr.InvalidInput, "this slot is not currently substituted")
	}
	if err := s.checkEditable(ctx, plan.SeasonID, plan.TeamID, int64(plan.WeekNumber)); err != nil {
		return models.LineupPlan{}, err
	}
	updated, err := s.store.ClearSubstitute(ctx, id)
	if err != nil {
		return models.LineupPlan{}, domainerr.New("LINEUP_SUB_CLEAR_FAILED", domainerr.Internal, "clear substitute failed")
	}
	return updated, nil
}

// checkEditable rejects substitute changes once the team's match for this
// season/week is season-closed, week-closed, approved, or processed. When no
// match has been scheduled yet for this team/week, there is nothing to lock
// against, so the change is allowed.
func (s *LineupService) checkEditable(ctx context.Context, seasonID, teamID, weekNumber int64) error {
	matchID, found, err := s.store.FindMatchID(ctx, seasonID, teamID, weekNumber)
	if err != nil {
		return domainerr.New("LINEUP_SUB_LOCK_CHECK_FAILED", domainerr.Internal, "checking match lock state failed")
	}
	if !found {
		return nil
	}
	if closed, err := s.matchLocks.IsSeasonClosedForMatch(ctx, matchID); err != nil {
		return domainerr.New("LINEUP_SUB_LOCK_CHECK_FAILED", domainerr.Internal, "checking season lock state failed")
	} else if closed {
		return domainerr.New("SEASON_CLOSED", domainerr.Conflict, "season is closed; substitutes cannot be changed")
	}
	if closed, err := s.matchLocks.IsWeekClosed(ctx, matchID); err != nil {
		return domainerr.New("LINEUP_SUB_LOCK_CHECK_FAILED", domainerr.Internal, "checking week lock state failed")
	} else if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict, "week is closed; substitutes cannot be changed")
	}
	state, err := s.matchLocks.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		return domainerr.New("LINEUP_SUB_LOCK_CHECK_FAILED", domainerr.Internal, "checking match approval state failed")
	}
	if state.ProcessedAt != nil {
		return domainerr.New("MATCH_PROCESSED", domainerr.Conflict, "match scores are processed; substitutes cannot be changed")
	}
	if state.ApprovedAt != nil {
		return domainerr.New("MATCH_APPROVED", domainerr.Conflict, "match scores are approved; substitutes cannot be changed")
	}
	return nil
}
