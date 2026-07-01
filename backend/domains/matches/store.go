package matches

import (
	"context"

	"league_app/models"
)

// AckEntry is one warning acknowledgment. The handler uses it to decode the
// HTTP request body; the service computes the rows to write and passes them
// here to WeekStore.CloseWeek. MatchID is 0 for non-match-specific warnings
// (stored as NULL in the DB).
type AckEntry struct {
	MatchID     int64  `json:"match_id"`
	WarningCode string `json:"warning_code"`
	Field       string `json:"field"`
	Notes       string `json:"notes"`
}

// WeekStore is the persistence interface for the week-workflow service.
// Implementations must be safe for concurrent use by multiple goroutines.
type WeekStore interface {
	// ListWeekSummaries returns one WeekSummary per week that has matches in
	// the season, merging match counts, league_weeks status, and ack counts.
	ListWeekSummaries(ctx context.Context, seasonID int64) ([]models.WeekSummary, error)

	// WeekMatchCount returns the number of matches for the season/week.
	// Returns 0, nil when no matches exist.
	WeekMatchCount(ctx context.Context, seasonID, weekNum int64) (int, error)

	// GetWeekStatus returns league_weeks.status for the season/week.
	// Returns "", nil when no row exists (implicitly "open").
	GetWeekStatus(ctx context.Context, seasonID, weekNum int64) (string, error)

	// CloseWeek atomically upserts the league_weeks row to "closed", sets
	// matches.week_closed=1, and inserts one ack row per entry in acks.
	CloseWeek(ctx context.Context, seasonID, weekNum int64, acks []AckEntry) error

	// ReopenWeek atomically sets league_weeks.status to "open" and clears
	// matches.week_closed for the season/week.
	ReopenWeek(ctx context.Context, seasonID, weekNum int64) error

	// ListAcknowledgments returns all close acknowledgments for the week,
	// ordered by acknowledged_at DESC.
	ListAcknowledgments(ctx context.Context, seasonID, weekNum int64) ([]models.CloseAck, error)
}
