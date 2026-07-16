package matches

import (
	"context"
	"fmt"
	"time"

	"league_app/backend/domainerr"
)

// PushbackPreviewRequest is the input for both Preview and Apply.
type PushbackPreviewRequest struct {
	SeasonID   int64 `json:"season_id"`
	CutoffWeek int   `json:"cutoff_week"`
	WeeksToAdd int   `json:"weeks_to_add"`
}

// ShiftedMatch is one unplayed match that would move if the pushback were applied.
type ShiftedMatch struct {
	ID            int64   `json:"id"`
	WeekNumber    int     `json:"week_number"`
	NewWeekNumber int     `json:"new_week_number"`
	MatchDate     *string `json:"match_date,omitempty"`
	NewMatchDate  *string `json:"new_match_date,omitempty"`
	HomeTeamID    int64   `json:"home_team_id"`
	AwayTeamID    int64   `json:"away_team_id"`
}

// PreservedMatch is one completed match at or after the cutoff that would not move.
type PreservedMatch struct {
	ID         int64   `json:"id"`
	WeekNumber int     `json:"week_number"`
	MatchDate  *string `json:"match_date,omitempty"`
	Completed  bool    `json:"completed"`
	HomeTeamID int64   `json:"home_team_id"`
	AwayTeamID int64   `json:"away_team_id"`
}

// PushbackPreviewResult is the response shape for both preview and apply.
// Shifted and Preserved only contain matches at or after the cutoff week.
// Matches before the cutoff are outside the preview range and are omitted.
type PushbackPreviewResult struct {
	Shifted    []ShiftedMatch   `json:"shifted"`
	Preserved  []PreservedMatch `json:"preserved"`
	NewEndDate *string          `json:"new_end_date,omitempty"`
}

// PushbackService computes pushback previews and applies them.
type PushbackService struct {
	store PushbackStore
}

// NewPushbackService returns a PushbackService backed by the given store.
func NewPushbackService(store PushbackStore) *PushbackService {
	return &PushbackService{store: store}
}

// Preview computes what a pushback would affect without writing any data.
func (s *PushbackService) Preview(ctx context.Context, req PushbackPreviewRequest) (PushbackPreviewResult, error) {
	return s.compute(ctx, req)
}

// Apply validates the request, computes the shift plan, writes it atomically,
// and returns the same result shape as Preview.
// Audit write is deferred until the audit system is implemented.
func (s *PushbackService) Apply(ctx context.Context, req PushbackPreviewRequest) (PushbackPreviewResult, error) {
	result, err := s.compute(ctx, req)
	if err != nil {
		return PushbackPreviewResult{}, err
	}
	ids := make([]int64, len(result.Shifted))
	for i, sm := range result.Shifted {
		ids[i] = sm.ID
	}
	applyInput := PushbackApplyInput{
		SeasonID:   req.SeasonID,
		ShiftedIDs: ids,
		WeeksToAdd: req.WeeksToAdd,
		DayShift:   req.WeeksToAdd * 7,
		NewEndDate: result.NewEndDate,
	}
	if err := s.store.ApplyPushback(ctx, applyInput); err != nil {
		return PushbackPreviewResult{}, fmt.Errorf("pushback apply: %w", err)
	}
	return result, nil
}

// compute performs validation, season/closed-week checks, and match partitioning.
// Both Preview and Apply use this to avoid duplicating logic.
func (s *PushbackService) compute(ctx context.Context, req PushbackPreviewRequest) (PushbackPreviewResult, error) {
	if req.CutoffWeek < 1 {
		return PushbackPreviewResult{}, domainerr.New("PUSHBACK_INVALID_CUTOFF",
			domainerr.InvalidInput, "cutoff_week must be at least 1")
	}
	if req.WeeksToAdd < 1 {
		return PushbackPreviewResult{}, domainerr.New("PUSHBACK_INVALID_WEEKS_TO_ADD",
			domainerr.InvalidInput, "weeks_to_add must be at least 1")
	}

	exists, err := s.store.SeasonExists(ctx, req.SeasonID)
	if err != nil {
		return PushbackPreviewResult{}, fmt.Errorf("pushback compute: check season: %w", err)
	}
	if !exists {
		return PushbackPreviewResult{}, domainerr.New("PUSHBACK_SEASON_NOT_FOUND",
			domainerr.NotFound, "season not found")
	}

	closed, err := s.store.HasClosedWeeksAtOrAfter(ctx, req.SeasonID, req.CutoffWeek)
	if err != nil {
		return PushbackPreviewResult{}, fmt.Errorf("pushback compute: check closed weeks: %w", err)
	}
	if closed {
		return PushbackPreviewResult{}, domainerr.New("PUSHBACK_HAS_CLOSED_WEEKS",
			domainerr.Conflict,
			"cannot pushback: one or more weeks at or after the cutoff are already closed")
	}

	allMatches, err := s.store.GetPushbackMatches(ctx, req.SeasonID)
	if err != nil {
		return PushbackPreviewResult{}, fmt.Errorf("pushback compute: get matches: %w", err)
	}

	shiftDays := req.WeeksToAdd * 7
	var shifted []ShiftedMatch
	var preserved []PreservedMatch
	var latestDate string

	for _, m := range allMatches {
		if m.WeekNumber < req.CutoffWeek {
			// Before the cutoff - outside the preview range. Track date for end-date calc.
			if m.MatchDate != nil && *m.MatchDate > latestDate {
				latestDate = *m.MatchDate
			}
			continue
		}

		if m.Completed {
			p := PreservedMatch{
				ID:         m.ID,
				WeekNumber: m.WeekNumber,
				MatchDate:  m.MatchDate,
				Completed:  true,
				HomeTeamID: m.HomeTeamID,
				AwayTeamID: m.AwayTeamID,
			}
			preserved = append(preserved, p)
			if m.MatchDate != nil && *m.MatchDate > latestDate {
				latestDate = *m.MatchDate
			}
			continue
		}

		// Unplayed at or after cutoff - this match shifts.
		sm := ShiftedMatch{
			ID:            m.ID,
			WeekNumber:    m.WeekNumber,
			NewWeekNumber: m.WeekNumber + req.WeeksToAdd,
			HomeTeamID:    m.HomeTeamID,
			AwayTeamID:    m.AwayTeamID,
		}
		if m.MatchDate != nil {
			sm.MatchDate = m.MatchDate
			newDate := shiftDate(*m.MatchDate, shiftDays)
			sm.NewMatchDate = &newDate
			if newDate > latestDate {
				latestDate = newDate
			}
		}
		shifted = append(shifted, sm)
	}

	result := PushbackPreviewResult{
		Shifted:   shifted,
		Preserved: preserved,
	}
	if latestDate != "" {
		result.NewEndDate = &latestDate
	}

	// Return empty slices rather than JSON null when nothing qualifies.
	if result.Shifted == nil {
		result.Shifted = []ShiftedMatch{}
	}
	if result.Preserved == nil {
		result.Preserved = []PreservedMatch{}
	}

	return result, nil
}

// shiftDate adds days to a YYYY-MM-DD string. Returns the original on parse error.
func shiftDate(date string, days int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}
