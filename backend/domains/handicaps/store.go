package handicaps

import "context"

// HandicapRuleRow holds raw stored values for the four handicap rule keys.
// A nil field means the row is absent from season_rules.
// A non-nil pointer to an empty string means the row is present but stored blank.
// The adapter applies no defaults, type parsing, or range validation.
type HandicapRuleRow struct {
	UpdateMethod *string // rule_key='handicap_update_method'
	WindowSize   *string // rule_key='handicap_current_game_window'
	Threshold    *string // rule_key='handicap_min_games_for_recommendation'
	MaxHC        *string // rule_key='max_individual_handicap'
}

// RosterEntry is one season-rostered player. All fields are plain Go types;
// no database/sql types cross this boundary.
type RosterEntry struct {
	PlayerID   int64
	PlayerName string
	TeamName   string  // season_teams.season_name (season snapshot)
	AssignedHC float64 // players.handicap at query time
	AdminHold  bool
}

// RackRow is one round_results row reduced to rack-accumulation columns.
// HomeHCUsed and AwayHCUsed are nil when the DB column is NULL (no snapshot).
type RackRow struct {
	HomePlayerID int64
	AwayPlayerID int64
	G1H, G1A     int
	G2H, G2A     int
	G3H, G3A     int
	HomeHCUsed   *float64
	AwayHCUsed   *float64
}

// Store is the data-access contract for the handicaps domain.
// Implementations own all SQL. Service code contains no SQL.
// No database/sql types appear in any method signature.
type Store interface {
	// RunTx executes fn inside a single database transaction.
	// The Store passed to fn is scoped to that transaction; all reads
	// inside fn share a consistent snapshot. The adapter owns BeginTx,
	// Commit, and Rollback. Callback errors and panics both cause rollback;
	// panics are re-propagated after rollback.
	// The service must call RunTx exactly once per Recommendations call.
	RunTx(ctx context.Context, fn func(Store) error) error

	// SeasonExists returns true when a season with this ID exists.
	SeasonExists(ctx context.Context, seasonID int64) (bool, error)

	// ClosedWeekCount returns the number of distinct week_numbers in this season
	// that have at least one match with week_closed=1.
	ClosedWeekCount(ctx context.Context, seasonID int64) (int, error)

	// SeasonHandicapRules returns raw stored values for the four handicap rule keys
	// in a single query. Nil fields mean the row is absent.
	SeasonHandicapRules(ctx context.Context, seasonID int64) (HandicapRuleRow, error)

	// SeasonRoster returns all players in season_rosters for this season joined to
	// season_teams.season_name, ordered by season_name, last_name, first_name.
	SeasonRoster(ctx context.Context, seasonID int64) ([]RosterEntry, error)

	// EligibleRacks returns all round_results rows where the home or away player is
	// in playerIDs, the match has completed=1 AND week_closed=1, and the league
	// game_format is '8ball'. Ordered most-recent-first (match_date DESC, match.id DESC,
	// round_number DESC).
	EligibleRacks(ctx context.Context, playerIDs []int64) ([]RackRow, error)
}
