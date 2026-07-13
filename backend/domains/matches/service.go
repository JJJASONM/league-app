package matches

import (
	"context"
	"fmt"

	"league_app/backend/domainerr"
	"league_app/backend/domains/rules"
	"league_app/backend/validation"
	"league_app/models"
)

// CloseWeekRequest carries inputs for the CloseWeek service call.
type CloseWeekRequest struct {
	SeasonID        int64
	WeekNumber      int64
	Acknowledgments []AckEntry
}

// CloseWeekResult carries the result of a successful CloseWeek call.
type CloseWeekResult struct {
	AckCount int
}

// WeekCloseErr is returned when CloseWeek is blocked by validation errors or
// missing acknowledgments. The handler maps it to HTTP 422.
type WeekCloseErr struct {
	Result validation.Result
}

func (e *WeekCloseErr) Error() string {
	return fmt.Sprintf("week close blocked: %d error(s)", len(e.Result.Errors()))
}

// WeekService orchestrates list, validate, close, reopen, acknowledgment-history,
// and advance-preview for the week-workflow.
// It implements the WeekManager interface declared in handlers/deps.go.
type WeekService struct {
	store     WeekStore
	hcPreview HandicapPreviewer // nil-safe: advance preview returns empty Handicap when nil
	ruleStore rules.RuleStore
}

// NewWeekService returns a WeekService backed by the given store and rule store.
// hcPreview is optional (nil disables handicap preview in AdvanceData/AdvancePreview).
func NewWeekService(store WeekStore, hcPreview HandicapPreviewer, ruleStore rules.RuleStore) *WeekService {
	return &WeekService{store: store, hcPreview: hcPreview, ruleStore: ruleStore}
}

// ListWeeks returns the current status and match counts for all scheduled weeks
// in the season.
func (s *WeekService) ListWeeks(ctx context.Context, seasonID int64) ([]models.WeekSummary, error) {
	return s.store.ListWeekSummaries(ctx, seasonID)
}

// ValidateWeek runs week validation and returns the result. Config and match data
// are fetched from the store; no direct DB connection is required.
func (s *WeekService) ValidateWeek(ctx context.Context, seasonID, weekNum int64) (validation.Result, error) {
	cfg, err := ResolveRoundConfig(ctx, s.ruleStore, seasonID)
	if err != nil {
		return validation.Result{}, fmt.Errorf("validate week: config: %w", err)
	}
	data, err := s.store.GetWeekValidationData(ctx, seasonID, weekNum)
	if err != nil {
		return validation.Result{}, fmt.Errorf("validate week: data: %w", err)
	}
	return validateWeekData(data, cfg), nil
}

// CloseWeek validates the week, checks that all warnings are acknowledged, then
// commits the close. Returns *WeekCloseErr for validation/ack failures (HTTP 422).
// Returns domainerr.Conflict (WEEK_CLOSE_SEASON_DRAFT) when the season is still draft.
func (s *WeekService) CloseWeek(ctx context.Context, req CloseWeekRequest) (CloseWeekResult, error) {
	if draft, err := s.store.IsSeasonDraft(ctx, req.SeasonID); err != nil {
		return CloseWeekResult{}, fmt.Errorf("close week: draft check: %w", err)
	} else if draft {
		return CloseWeekResult{}, domainerr.New("WEEK_CLOSE_SEASON_DRAFT", domainerr.Conflict,
			"cannot close a week for a draft season: activate the season before closing weeks")
	}

	cfg, err := ResolveRoundConfig(ctx, s.ruleStore, req.SeasonID)
	if err != nil {
		return CloseWeekResult{}, fmt.Errorf("close week: config: %w", err)
	}
	data, err := s.store.GetWeekValidationData(ctx, req.SeasonID, req.WeekNumber)
	if err != nil {
		return CloseWeekResult{}, fmt.Errorf("close week: validation data: %w", err)
	}
	result := validateWeekData(data, cfg)
	if result.HasErrors() {
		return CloseWeekResult{}, &WeekCloseErr{Result: result}
	}

	type ackKey struct {
		matchID int64
		code    string
		field   string
	}
	ackSet := make(map[ackKey]string, len(req.Acknowledgments))
	for _, a := range req.Acknowledgments {
		ackSet[ackKey{a.MatchID, a.WarningCode, a.Field}] = a.Notes
	}

	var unacked []validation.Message
	for _, msg := range result.Warnings() {
		var mid int64
		if msg.MatchID != nil {
			mid = *msg.MatchID
		}
		if _, ok := ackSet[ackKey{mid, msg.Code, msg.Field}]; !ok {
			unacked = append(unacked, msg)
		}
	}
	if len(unacked) > 0 {
		var ackResult validation.Result
		for _, msg := range unacked {
			ackResult.AddError(msg.Code, msg.Field,
				"warning requires acknowledgment before close: "+msg.Message)
			if msg.MatchID != nil {
				id := *msg.MatchID
				ackResult.Messages[len(ackResult.Messages)-1].MatchID = &id
			}
		}
		return CloseWeekResult{}, &WeekCloseErr{Result: ackResult}
	}

	acksToWrite := make([]AckEntry, 0, len(result.Warnings()))
	for _, msg := range result.Warnings() {
		var mid int64
		if msg.MatchID != nil {
			mid = *msg.MatchID
		}
		notes := ackSet[ackKey{mid, msg.Code, msg.Field}]
		acksToWrite = append(acksToWrite, AckEntry{
			MatchID:     mid,
			WarningCode: msg.Code,
			Field:       msg.Field,
			Notes:       notes,
		})
	}

	if err := s.store.CloseWeek(ctx, req.SeasonID, req.WeekNumber, acksToWrite); err != nil {
		return CloseWeekResult{}, fmt.Errorf("close week: %w", err)
	}
	return CloseWeekResult{AckCount: len(result.Warnings())}, nil
}

// ReopenWeek sets the week back to open. Returns domainerr.NotFound (404) when no
// matches exist for the week, and domainerr.Conflict (409) when the week is not closed.
func (s *WeekService) ReopenWeek(ctx context.Context, seasonID, weekNum int64) error {
	count, err := s.store.WeekMatchCount(ctx, seasonID, weekNum)
	if err != nil {
		return fmt.Errorf("reopen week: %w", err)
	}
	if count == 0 {
		return domainerr.New("WEEK_NOT_FOUND", domainerr.NotFound,
			"week not found: no matches for this season and week")
	}

	status, err := s.store.GetWeekStatus(ctx, seasonID, weekNum)
	if err != nil {
		return fmt.Errorf("reopen week: %w", err)
	}
	if status != WeekStatusClosed {
		return domainerr.New("WEEK_NOT_CLOSED", domainerr.Conflict, "week is not closed")
	}

	if err := s.store.ReopenWeek(ctx, seasonID, weekNum); err != nil {
		return fmt.Errorf("reopen week: %w", err)
	}
	return nil
}

// ListAcknowledgments returns all close acknowledgments for the week.
// Returns domainerr.NotFound (404) when no matches exist for the week.
func (s *WeekService) ListAcknowledgments(ctx context.Context, seasonID, weekNum int64) ([]models.CloseAck, error) {
	count, err := s.store.WeekMatchCount(ctx, seasonID, weekNum)
	if err != nil {
		return nil, fmt.Errorf("list acknowledgments: %w", err)
	}
	if count == 0 {
		return nil, domainerr.New("WEEK_NOT_FOUND", domainerr.NotFound,
			"week not found: no matches for this season and week")
	}

	acks, err := s.store.ListAcknowledgments(ctx, seasonID, weekNum)
	if err != nil {
		return nil, fmt.Errorf("list acknowledgments: %w", err)
	}
	if acks == nil {
		acks = []models.CloseAck{}
	}
	return acks, nil
}

// AdvanceData assembles the advance-result response for a season/week.
// It reads match counts, week status, and next-week readiness from the store,
// then calls the HandicapPreviewer (if wired) for the handicap portion.
// Called from closeWeekHandler after a successful commit, and from AdvancePreview.
func (s *WeekService) AdvanceData(ctx context.Context, seasonID, weekNum int64) (models.AdvanceResult, error) {
	summary, err := s.store.GetWeekAdvanceSummary(ctx, seasonID, weekNum)
	if err != nil {
		return models.AdvanceResult{}, fmt.Errorf("advance data: %w", err)
	}

	var hc models.AdvancePreviewHandicap
	if s.hcPreview != nil {
		hc, err = s.hcPreview.HandicapPreview(ctx, seasonID)
		if err != nil {
			return models.AdvanceResult{}, fmt.Errorf("advance data: handicap preview: %w", err)
		}
	}

	return models.AdvanceResult{
		ClosedWeek:     summary.ClosedWeek,
		NextWeekNumber: summary.NextWeekNumber,
		NextWeek:       summary.NextWeek,
		Handicap:       hc,
	}, nil
}

// AdvancePreview builds the full advance-preview response for a season/week.
// Returns domainerr.NotFound (404) when no matches exist for the week.
// Called from the getAdvancePreview handler.
func (s *WeekService) AdvancePreview(ctx context.Context, seasonID, weekNum int64) (models.AdvancePreview, error) {
	count, err := s.store.WeekMatchCount(ctx, seasonID, weekNum)
	if err != nil {
		return models.AdvancePreview{}, fmt.Errorf("advance preview: %w", err)
	}
	if count == 0 {
		return models.AdvancePreview{}, domainerr.New("WEEK_NOT_FOUND", domainerr.NotFound,
			"week not found: no matches for this season and week")
	}

	cfg, err := ResolveRoundConfig(ctx, s.ruleStore, seasonID)
	if err != nil {
		return models.AdvancePreview{}, fmt.Errorf("advance preview: config: %w", err)
	}
	data, err := s.store.GetWeekValidationData(ctx, seasonID, weekNum)
	if err != nil {
		return models.AdvancePreview{}, fmt.Errorf("advance preview: validation data: %w", err)
	}
	result := validateWeekData(data, cfg)

	msgs := make([]models.AdvancePreviewMessage, 0, len(result.Messages))
	for _, msg := range result.Messages {
		msgs = append(msgs, models.AdvancePreviewMessage{
			Code:    msg.Code,
			Field:   msg.Field,
			Message: msg.Message,
			Level:   string(msg.Level),
			MatchID: msg.MatchID,
		})
	}

	ar, err := s.AdvanceData(ctx, seasonID, weekNum)
	if err != nil {
		return models.AdvancePreview{}, fmt.Errorf("advance preview: %w", err)
	}

	return models.AdvancePreview{
		SeasonID:           seasonID,
		WeekNumber:         int(weekNum),
		CanClose:           !result.HasErrors(),
		ValidationMessages: msgs,
		CurrentWeek:        ar.ClosedWeek,
		NextWeekNumber:     ar.NextWeekNumber,
		NextWeek:           ar.NextWeek,
		Handicap:           ar.Handicap,
	}, nil
}
