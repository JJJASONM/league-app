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

// WeekValidationData holds all match and round data needed to validate a week
// for close. Fetched by WeekStore.GetWeekValidationData.
type WeekValidationData struct {
	Matches []MatchValidationRow
}

// MatchValidationRow holds one match's team assignment and resolved round results
// for use in week validation.
type MatchValidationRow struct {
	MatchID    int64
	HomeTeamID *int64 // nil when not assigned
	AwayTeamID *int64
	Rounds     []RoundValidationRow
}

// RoundValidationRow holds one round_results row with handicaps resolved to float64.
// HomeHC and AwayHC use the stored snapshot when available, falling back to the
// current player handicap.
type RoundValidationRow struct {
	RoundNumber  int
	HomePlayerID int64
	AwayPlayerID int64
	Game1Home    int
	Game1Away    int
	Game2Home    int
	Game2Away    int
	Game3Home    int
	Game3Away    int
	HomeHC       float64
	AwayHC       float64
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

	// GetWeekAdvanceSummary returns match counts, week status, and next-week
	// readiness for the advance-preview and close-result response. Read-only.
	// Returns an empty summary without error when no matches exist for the week.
	GetWeekAdvanceSummary(ctx context.Context, seasonID, weekNum int64) (WeekAdvanceSummary, error)

	// GetWeekValidationData returns all match and round data needed for week
	// validation. Handicap snapshots take priority; falls back to current player
	// handicap when the snapshot column is NULL.
	GetWeekValidationData(ctx context.Context, seasonID, weekNum int64) (WeekValidationData, error)
}
