package matches

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domainerr"
	"league_app/backend/validation"
	"league_app/models"
)

// CloseWeekRequest carries inputs for the CloseWeek service call.
type CloseWeekRequest struct {
	SeasonID        int64
	WeekNumber      int64
	Acknowledgments []AckEntry
	Cfg             RoundConfig
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

// WeekService orchestrates list, validate, close, reopen, and acknowledgment-history
// for the week-workflow. It implements the WeekManager interface declared in handlers/deps.go.
type WeekService struct {
	store WeekStore
	db    *sql.DB // temporary: passed to ValidateWeek until B4 moves validation to a store method
}

// NewWeekService returns a WeekService backed by the given store and database connection.
// The db argument is used only to call the package-level ValidateWeek function.
func NewWeekService(store WeekStore, db *sql.DB) *WeekService {
	return &WeekService{store: store, db: db}
}

// ListWeeks returns the current status and match counts for all scheduled weeks
// in the season.
func (s *WeekService) ListWeeks(ctx context.Context, seasonID int64) ([]models.WeekSummary, error) {
	return s.store.ListWeekSummaries(ctx, seasonID)
}

// ValidateWeek runs week validation and returns the result. The error return is
// always nil in the current implementation; the signature satisfies WeekManager.
func (s *WeekService) ValidateWeek(ctx context.Context, seasonID, weekNum int64, cfg RoundConfig) (validation.Result, error) {
	result := ValidateWeek(s.db, seasonID, int(weekNum), cfg)
	return result, nil
}

// CloseWeek validates the week, checks that all warnings are acknowledged, then
// commits the close. Returns *WeekCloseErr for validation/ack failures (HTTP 422).
func (s *WeekService) CloseWeek(ctx context.Context, req CloseWeekRequest) (CloseWeekResult, error) {
	result := ValidateWeek(s.db, req.SeasonID, int(req.WeekNumber), req.Cfg)
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
	if status != "closed" {
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
