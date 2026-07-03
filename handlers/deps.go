package handlers

import (
	"context"

	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/seasons"
	"league_app/backend/validation"
	"league_app/models"
)

// RuleManager is the subset of rules.RuleService used by the season-rules handlers.
// Accepting an interface allows stub injection in tests.
type RuleManager interface {
	List(ctx context.Context, seasonID int64) ([]models.SeasonRule, error)
	Upsert(ctx context.Context, rule models.SeasonRule) (models.SeasonRule, error)
	Update(ctx context.Context, ruleID int64, label, value string) error
	Delete(ctx context.Context, ruleID int64) error
}

// HandicapRecommender is the subset of handicaps.Service used by the read handler.
// Accepting an interface (rather than the concrete type) allows stub injection in tests.
type HandicapRecommender interface {
	Recommendations(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error)
}

// HandicapApplier is the subset of handicaps.Service used by the write handler.
// The route is registered in Register() when deps.AdminToken is non-empty.
type HandicapApplier interface {
	Apply(ctx context.Context, seasonID int64, req handicaps.ApplyRequest) (handicaps.ApplyResult, error)
}

// ApplyAuthResolver resolves and manages application users for Apply attribution.
// This is a purpose-built interface — not a generic user store.
type ApplyAuthResolver interface {
	// ResolveApplyUserByAPIKey returns the active user matching SHA-256(apiKey),
	// or nil, nil when no match is found.
	ResolveApplyUserByAPIKey(ctx context.Context, apiKey string) (*models.User, error)
	// CreateApplyUser creates a new user and returns the user plus the one-time cleartext key.
	CreateApplyUser(ctx context.Context, username string) (models.User, string, error)
	// ListApplyUsers returns all users. The api_key_hash column is never exposed.
	ListApplyUsers(ctx context.Context) ([]models.User, error)
}

// applyUserIDKey is an unexported type used as a context key for the resolved
// user ID on authenticated Apply requests. A struct type avoids collisions with
// other packages that use string keys.
type applyUserIDKey struct{}

// WeekManager is the subset of matches.WeekService used by the week-workflow handlers.
// Accepting an interface allows stub injection in tests.
type WeekManager interface {
	ListWeeks(ctx context.Context, seasonID int64) ([]models.WeekSummary, error)
	ValidateWeek(ctx context.Context, seasonID, weekNum int64) (validation.Result, error)
	CloseWeek(ctx context.Context, req matches.CloseWeekRequest) (matches.CloseWeekResult, error)
	ReopenWeek(ctx context.Context, seasonID, weekNum int64) error
	ListAcknowledgments(ctx context.Context, seasonID, weekNum int64) ([]models.CloseAck, error)
	AdvanceData(ctx context.Context, seasonID, weekNum int64) (models.AdvanceResult, error)
	AdvancePreview(ctx context.Context, seasonID, weekNum int64) (models.AdvancePreview, error)
}

// RoundManager is the subset of matches.RoundService used by the round/standings/stats handlers.
// Accepting an interface allows stub injection in tests.
type RoundManager interface {
	SaveRounds(ctx context.Context, input matches.SaveRoundsInput) error
	GetRounds(ctx context.Context, matchID int64) ([]models.RoundResult, error)
	GetStandings(ctx context.Context, seasonID int64) ([]models.Standing, error)
	GetPlayerStats(ctx context.Context, req matches.PlayerStatsRequest) ([]models.PlayerStat, error)
	SubmitResults(ctx context.Context, matchID int64, results []models.MatchResult) error
	ClearResults(ctx context.Context, matchID int64) error
}

// SeasonManager handles season lifecycle: activation, checklist evaluation,
// previous-season lookup, and draft/stale checks for team and roster mutations.
type SeasonManager interface {
	Activate(ctx context.Context, seasonID int64) error
	Checklist(ctx context.Context, seasonID int64) (models.SetupChecklist, error)
	PreviousSeason(ctx context.Context, seasonID int64) (seasons.PreviousSeasonResult, error)
	IsDraft(ctx context.Context, seasonID int64) (bool, error)
	MarkStaleIfScheduled(ctx context.Context, seasonID int64) error
}

// Dependencies holds domain services injected into handlers at startup.
// Add new service fields here as additional domains are migrated.
type Dependencies struct {
	HandicapSvc     HandicapRecommender
	HandicapApplier HandicapApplier
	// AdminToken is the static bearer token for LEAGUE_ADMIN_TOKEN fallback auth.
	// When empty the Apply route is not mounted.
	// Personal API keys (via ApplyAuth) are checked first; this token is the fallback.
	AdminToken string
	// ApplyAuth resolves personal API keys for Apply attribution.
	// When nil, only the AdminToken static fallback is used.
	ApplyAuth ApplyAuthResolver
	// WeekMgr handles the week-workflow: list, validate, close, reopen, ack-history.
	// When nil, week routes are not registered.
	WeekMgr WeekManager
	// RoundMgr handles scoresheet save/read, standings, and player stats.
	// When nil, round/standings/stats routes are not registered.
	RoundMgr RoundManager
	// RuleMgr handles per-season rule CRUD.
	// Required: Register panics if nil.
	RuleMgr RuleManager
	// SeasonMgr handles season lifecycle: activation, checklist, previous-season.
	// Required: Register panics if nil.
	SeasonMgr SeasonManager
}
