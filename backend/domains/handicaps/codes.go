package handicaps

// Method codes for the handicap_update_method season rule.
// These values are stored in season_rules.rule_value and written to
// handicap_history.method when Apply commits changes.
const (
	MethodManualReview         = "manual_review"
	MethodGameDiffAverage      = "game_diff_average"
	MethodKickerAveragePreview = "kicker_average_preview"
)

// Reason codes for HandicapReviewRec.Reason and PlayerHandicapRec.Reason.
// An empty reason string means the player is actionable (eligible for Apply).
const (
	ReasonNoData         = "no_data"
	ReasonAdminHold      = "admin_hold"
	ReasonBelowThreshold = "below_threshold"
	ReasonCapped         = "capped"
	ReasonNoChange       = "no_change"
)

// ReviewStatus codes for HandicapReviewResponse.Status and
// AdvancePreviewHandicap.Status.
const (
	ReviewStatusNoAutoApply = "no_auto_apply"
	ReviewStatusUnsupported = "unsupported"
	ReviewStatusNoData      = "no_data"
	ReviewStatusPreview     = "preview"
)
