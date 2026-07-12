package handlers

import (
	"context"

	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/leagues"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/players"
	"league_app/backend/domains/seasons"
	"league_app/backend/domains/teams"
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

// MatchManager handles match listing, detail retrieval, and team assignment.
// Routes are registered only when non-nil.
type MatchManager interface {
	ListMatches(ctx context.Context, req matches.ListMatchesRequest) ([]models.Match, error)
	GetMatch(ctx context.Context, id int64) (models.MatchDetail, error)
	AssignMatchTeams(ctx context.Context, id int64, homeTeamID, awayTeamID *int64) error
}

// ScheduleManager handles schedule generation for a season.
type ScheduleManager interface {
	GenerateSchedule(ctx context.Context, req matches.GenerateRequest) (matches.GenerateResult, error)
}

// LineupManager handles lineup plan listing, atomic save, and deletion.
// Routes are registered only when non-nil.
type LineupManager interface {
	ListLineupPlans(ctx context.Context, req matches.ListLineupPlansRequest) ([]models.LineupPlan, error)
	SaveTeamLineup(ctx context.Context, req matches.SaveLineupRequest) error
	DeleteLineupPlan(ctx context.Context, id int64) error
}

// LeagueManager handles league CRUD operations.
// Required: Register panics if nil.
type LeagueManager interface {
	ListLeagues(ctx context.Context) ([]models.League, error)
	GetLeague(ctx context.Context, id int64) (models.League, error)
	CreateLeague(ctx context.Context, input leagues.CreateLeagueInput) (models.League, error)
	UpdateLeague(ctx context.Context, id int64, input leagues.UpdateLeagueInput) error
	DeleteLeague(ctx context.Context, id int64) error
}

// PlayerManager handles player CRUD operations.
// Required: Register panics if nil.
type PlayerManager interface {
	ListPlayers(ctx context.Context, leagueID *int64) ([]models.Player, error)
	GetPlayer(ctx context.Context, id int64) (models.Player, error)
	CreatePlayer(ctx context.Context, input players.CreatePlayerInput) (models.Player, error)
	UpdatePlayer(ctx context.Context, id int64, input players.UpdatePlayerInput) error
	DeletePlayer(ctx context.Context, id int64) error
}

// TeamManager handles team CRUD operations.
// Required: Register panics if nil.
type TeamManager interface {
	ListTeams(ctx context.Context, leagueID *int64) ([]models.Team, error)
	GetTeam(ctx context.Context, id int64) (models.Team, error)
	CreateTeam(ctx context.Context, input teams.CreateTeamInput) (models.Team, error)
	UpdateTeam(ctx context.Context, id int64, input teams.UpdateTeamInput) error
	DeleteTeam(ctx context.Context, id int64) error
}

// SeasonManager handles season lifecycle: activation, checklist evaluation,
// previous-season lookup, draft/stale checks, team management, bye requests,
// and season roster operations.
type SeasonManager interface {
	Activate(ctx context.Context, seasonID int64) error
	Checklist(ctx context.Context, seasonID int64) (models.SetupChecklist, error)
	PreviousSeason(ctx context.Context, seasonID int64) (seasons.PreviousSeasonResult, error)
	IsDraft(ctx context.Context, seasonID int64) (bool, error)
	MarkStaleIfScheduled(ctx context.Context, seasonID int64) error
	AddTeam(ctx context.Context, seasonID int64, req seasons.AddTeamRequest) (models.SeasonTeam, error)
	RemoveTeam(ctx context.Context, seasonID, teamID int64) error
	UpdateTeam(ctx context.Context, seasonID, teamID int64, req seasons.UpdateTeamRequest) (models.SeasonTeam, error)
	CreateByeRequest(ctx context.Context, seasonID int64, req seasons.CreateByeRequestInput) (models.ByeRequest, error)
	UpdateByeRequest(ctx context.Context, seasonID, byeID int64, approve bool) (models.ByeRequest, error)
	ListRoster(ctx context.Context, seasonID, teamID int64) ([]models.SeasonRosterEntry, error)
	AddRosterPlayer(ctx context.Context, seasonID, teamID, playerID int64) (models.SeasonRosterEntry, error)
	RemoveRosterPlayer(ctx context.Context, seasonID, teamID, playerID int64) error
	ListAvailablePlayers(ctx context.Context, seasonID int64) ([]models.Player, error)
	ListSeasonTeams(ctx context.Context, seasonID int64) ([]models.SeasonTeam, error)
	ListSeasons(ctx context.Context, leagueID *int64) ([]models.Season, error)
	GetSeason(ctx context.Context, seasonID int64) (models.Season, error)
	CreateSeason(ctx context.Context, input seasons.CreateSeasonInput) (models.Season, error)
	UpdateSeason(ctx context.Context, seasonID int64, input seasons.UpdateSeasonInput) (models.Season, error)
	DeleteSeason(ctx context.Context, seasonID int64) error
	ListSkippedWeeks(ctx context.Context, seasonID int64) ([]models.SkippedWeek, error)
	CreateSkippedWeek(ctx context.Context, seasonID int64, skipDate, reason string) (models.SkippedWeek, error)
	DeleteSkippedWeek(ctx context.Context, seasonID, id int64) error
	ListByeRequests(ctx context.Context, seasonID int64) ([]models.ByeRequest, error)
	DeleteByeRequest(ctx context.Context, seasonID, byeID int64) error
	// FindActiveSeasonByLeague returns the ID of the active season in leagueID.
	// Returns (0, false, nil) when no active season exists.
	FindActiveSeasonByLeague(ctx context.Context, leagueID int64) (int64, bool, error)
	// RosterEligible returns (true, "") when both teams in a match have at least
	// minPlayers season-roster players, or when the season is not managed.
	RosterEligible(ctx context.Context, matchID int64, minPlayers int) (bool, string, error)
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
	// LeagueMgr handles league CRUD.
	// Required: Register panics if nil.
	LeagueMgr LeagueManager
	// PlayerMgr handles player CRUD.
	// Required: Register panics if nil.
	PlayerMgr PlayerManager
	// TeamMgr handles team CRUD.
	// Required: Register panics if nil.
	TeamMgr TeamManager
	// SeasonMgr handles season lifecycle: activation, checklist, previous-season.
	// Required: Register panics if nil.
	SeasonMgr SeasonManager
	// MatchMgr handles match listing, detail retrieval, and team assignment.
	// Routes are registered only when non-nil.
	MatchMgr MatchManager
	// ScheduleMgr handles schedule generation.
	// Routes are registered only when non-nil.
	ScheduleMgr ScheduleManager
	// LineupMgr handles lineup plan listing, atomic save, and deletion.
	// Routes are registered only when non-nil.
	LineupMgr LineupManager
}
