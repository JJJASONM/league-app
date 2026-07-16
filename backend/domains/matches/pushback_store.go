package matches

import "context"

// PushbackMatchRow is one match record fetched for pushback analysis.
// Only the columns required to compute the preview are included.
type PushbackMatchRow struct {
	ID         int64
	WeekNumber int
	MatchDate  *string // nil when the match has no scheduled date
	Completed  bool
	HomeTeamID int64
	AwayTeamID int64
}

// PushbackApplyInput carries the precomputed shift plan for an atomic apply.
// The service computes this from preview results and passes it to the store.
type PushbackApplyInput struct {
	SeasonID   int64
	ShiftedIDs []int64 // IDs of unplayed matches to shift; may be empty
	WeeksToAdd int     // added to week_number for each shifted match
	DayShift   int     // WeeksToAdd * 7; added to match_date when non-null
	NewEndDate *string // new value for seasons.end_date; nil leaves it unchanged
}

// PushbackStore is the persistence interface for pushback preview and apply.
// All methods accept a context and are safe for concurrent use.
type PushbackStore interface {
	// GetPushbackMatches returns all matches for the season with the columns
	// needed to compute a pushback preview, ordered by week_number, id.
	GetPushbackMatches(ctx context.Context, seasonID int64) ([]PushbackMatchRow, error)

	// HasClosedWeeksAtOrAfter reports whether any league_weeks row for the
	// season has status "closed" with week_number >= cutoffWeek.
	HasClosedWeeksAtOrAfter(ctx context.Context, seasonID int64, cutoffWeek int) (bool, error)

	// SeasonExists reports whether the season row exists.
	SeasonExists(ctx context.Context, seasonID int64) (bool, error)

	// ApplyPushback writes the precomputed shift plan atomically.
	// It shifts unplayed matches by ShiftedIDs, updates seasons.end_date,
	// and clears seasons.schedule_stale to 0. No validation is performed;
	// callers must validate before calling.
	ApplyPushback(ctx context.Context, input PushbackApplyInput) error
}
