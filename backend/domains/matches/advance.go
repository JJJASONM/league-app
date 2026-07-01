package matches

import (
	"context"

	"league_app/models"
)

// HandicapPreviewer is the consumer-side interface for read-only handicap
// preview data. Defined in the matches package (the consumer) and implemented
// by handicaps.Service. No import of matches into handicaps; no import cycle.
type HandicapPreviewer interface {
	HandicapPreview(ctx context.Context, seasonID int64) (models.AdvancePreviewHandicap, error)
}

// WeekAdvanceSummary is the matches-domain portion of the advance-preview
// response. It carries week status and next-week readiness data only.
// The Handicap field is populated separately by WeekService using HandicapPreviewer.
type WeekAdvanceSummary struct {
	ClosedWeek     models.AdvancePreviewWeekSummary
	NextWeekNumber *int
	NextWeek       *models.AdvancePreviewNextWeek
}
