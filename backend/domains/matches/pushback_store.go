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

// PushbackStore is the read-only persistence interface for the pushback preview.
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
}
