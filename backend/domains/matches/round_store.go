package matches

import (
	"context"
	"database/sql"

	"league_app/models"
)

// RoundStore is the persistence interface for the round read/write service.
// Implementations must be safe for concurrent use by multiple goroutines.
// The tx-scoped variant (returned by RunTx) operates within a single transaction.
type RoundStore interface {
	// IsWeekClosed returns true when matches.week_closed=1 for the given matchID.
	// Returns false when no match row exists.
	IsWeekClosed(ctx context.Context, matchID int64) (bool, error)

	// SeasonRoundConfig reads handicap_multiplier and min_ball_handicap from
	// season_rules for the given season. Returns defaults when rules are absent.
	// Returns an error when a stored value is present but malformed.
	SeasonRoundConfig(ctx context.Context, seasonID int64) (RoundConfig, error)

	// RunTx executes fn inside a single read/write transaction.
	// The RoundStore passed to fn is tx-scoped. Panics and errors both roll back.
	RunTx(ctx context.Context, fn func(RoundStore) error) error

	// LoadMatchContext returns season_id, home_team_id, and away_team_id for a match.
	LoadMatchContext(ctx context.Context, matchID int64) (MatchContext, error)

	// LoadPlayerHandicap returns the current handicap for the given player.
	LoadPlayerHandicap(ctx context.Context, playerID int64) (float64, error)

	// LoadPriorSnapshots returns the stored HC snapshots for existing round_results rows,
	// grouped by round_number. Used to preserve history when a scoresheet is re-saved.
	LoadPriorSnapshots(ctx context.Context, matchID int64) ([]PriorSnapshotRow, error)

	// DeleteRoundResults deletes all round_results rows for the match.
	DeleteRoundResults(ctx context.Context, matchID int64) error

	// InsertRoundResult inserts one round_results row with full snapshot columns.
	InsertRoundResult(ctx context.Context, row RoundResultRow) error

	// DeleteMatchResults deletes all match_results rows for the match.
	DeleteMatchResults(ctx context.Context, matchID int64) error

	// InsertMatchResult inserts one match_results row.
	InsertMatchResult(ctx context.Context, row MatchResultRow) error

	// MarkMatchCompleted sets matches.completed=1 for the match.
	MarkMatchCompleted(ctx context.Context, matchID int64) error

	// MarkMatchIncomplete sets matches.completed=0 for the match.
	MarkMatchIncomplete(ctx context.Context, matchID int64) error

	// GetRoundResults returns all round_results rows for the match joined to player
	// names and current handicaps, ordered by round_number then id.
	GetRoundResults(ctx context.Context, matchID int64) ([]models.RoundResult, error)

	// GetStandingsData returns teams, completed+closed matches, and per-match results
	// for the given season in the shape needed by logic.ComputeStandings.
	GetStandingsData(ctx context.Context, seasonID int64) (StandingsData, error)

	// GetPlayerStats returns aggregated match_results for the given season or league scope.
	GetPlayerStats(ctx context.Context, req PlayerStatsRequest) ([]models.PlayerStat, error)

	// SubmitMatchResults replaces match_results for a match and marks it completed,
	// wrapped in a transaction.
	SubmitMatchResults(ctx context.Context, matchID int64, results []models.MatchResult) error

	// ClearMatchResults deletes match_results for a match and marks it incomplete.
	ClearMatchResults(ctx context.Context, matchID int64) error
}

// MatchContext holds the season and team IDs for a match.
type MatchContext struct {
	SeasonID   int64
	HomeTeamID int64
	AwayTeamID int64
}

// PriorSnapshotRow holds the HC snapshot from an existing round_results row.
// Used by SaveRounds to preserve handicap history on re-save.
type PriorSnapshotRow struct {
	RoundNumber      int
	HomePlayerID     int64
	AwayPlayerID     int64
	HomeHandicapUsed sql.NullFloat64
	AwayHandicapUsed sql.NullFloat64
}

// RoundResultRow is one row to be inserted into round_results, including full HC snapshots.
type RoundResultRow struct {
	MatchID         int64
	RoundNumber     int
	HomePlayerID    int64
	AwayPlayerID    int64
	Game1Home       int
	Game1Away       int
	Game2Home       int
	Game2Away       int
	Game3Home       int
	Game3Away       int
	HomeHCUsed      float64
	AwayHCUsed      float64
	HandicapPtsUsed int
	HandicapTo      string
}

// MatchResultRow is one row to be inserted into match_results.
type MatchResultRow struct {
	MatchID   int64
	PlayerID  int64
	TeamID    int64
	GamesWon  int
	GamesLost int
	Diff      float64
	SetsWon   int
	SetsLost  int
}

// StandingsData is the raw data returned from GetStandingsData,
// ready to be passed to logic.ComputeStandings.
type StandingsData struct {
	Teams     []models.Team
	Matches   []models.Match
	ResultMap map[int64][]models.MatchResult
}

// PlayerStatsRequest parameterizes GetPlayerStats.
// Exactly one of SeasonID or LeagueID should be non-zero.
type PlayerStatsRequest struct {
	SeasonID int64
	LeagueID int64
}

// SaveRoundsInput is the domain-level input for RoundService.SaveRounds.
type SaveRoundsInput struct {
	MatchID int64
	Rounds  []models.RoundResult
}
