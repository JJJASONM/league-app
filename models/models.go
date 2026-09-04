package models

import "time"

// League is the top-level container (e.g. "Monday 8-Ball", "Tuesday 9-Ball").
type League struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	GameFormat string    `json:"game_format"` // "8ball","9ball","10ball","straight"
	DayOfWeek  string    `json:"day_of_week"` // "Monday","Tuesday", etc.
	CreatedAt  time.Time `json:"created_at"`
}

// Player represents a league member.
type Player struct {
	ID           int64     `json:"id"`
	PlayerNumber string    `json:"player_number"` // two-digit code e.g. "42"; locked once set
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	Name         string    `json:"name"`            // computed: FirstName + " " + LastName
	Phone        string    `json:"phone,omitempty"`
	Email        string    `json:"email,omitempty"`
	TeamID       *int64    `json:"team_id"`
	TeamName     string    `json:"team_name,omitempty"`
	LeagueID     int64     `json:"league_id,omitempty"`
	// Handicap meaning depends on game format:
	//   8-ball: Diff rating = (games won − games lost) / matches played
	//   9-ball: race-to number (e.g. 5, 7)
	Handicap  float64   `json:"handicap"`
	AdminHold bool      `json:"admin_hold"` // 9-ball: locked at Administrative Discretion
	Active    bool      `json:"active"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Team represents a group of players competing together.
type Team struct {
	ID         int64     `json:"id"`
	LeagueID   int64     `json:"league_id"`
	Name       string    `json:"name"`
	TeamNumber string    `json:"team_number,omitempty"`
	CaptainID  *int64    `json:"captain_id"`
	Players    []Player  `json:"players,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Season represents a bounded league season.
// EndDate is computed from the last scheduled match date (not user-entered).
// ScheduleType: "single_rr" | "double_rr" | "split" | "custom" | "blanket"
type Season struct {
	ID            int64     `json:"id"`
	LeagueID      int64     `json:"league_id"`
	Name          string    `json:"name"`
	StartDate     *string   `json:"start_date"`
	EndDate       *string   `json:"end_date"`       // computed after schedule generation
	Active        bool      `json:"active"`
	ScheduleType  string    `json:"schedule_type"`
	NumWeeks      int       `json:"num_weeks"`       // used for "custom" and "blanket" types
	ScheduleStale bool      `json:"schedule_stale"`  // true when season_teams changed after generation
	TeamsManaged  bool      `json:"teams_managed"`   // false = legacy season; true = explicit team management
	ActivatedAt   *string   `json:"activated_at,omitempty"` // set once on first activation; persistent setup lock
	ClosedAt      *string   `json:"closed_at,omitempty"`    // set when season is officially closed
	CreatedAt     time.Time `json:"created_at"`
}

// SeasonTeam is a team explicitly selected to participate in a season.
// SeasonName is an editable draft snapshot of the team name for this season.
// CaptainID must reference a player on this team's season roster.
type SeasonTeam struct {
	ID          int64   `json:"id"`
	SeasonID    int64   `json:"season_id"`
	TeamID      int64   `json:"team_id"`
	TeamName    string  `json:"team_name"`              // from teams table (permanent)
	TeamNumber  string  `json:"team_number,omitempty"`  // from teams table
	SeasonName  string  `json:"season_name"`            // season-specific snapshot
	CaptainID   *int64  `json:"captain_id"`
	CaptainName string  `json:"captain_name,omitempty"`
	RosterCount int     `json:"roster_count"`
}

// SeasonRosterEntry is one player on a team's season roster.
type SeasonRosterEntry struct {
	ID           int64   `json:"id"`
	SeasonID     int64   `json:"season_id"`
	TeamID       int64   `json:"team_id"`
	TeamName     string  `json:"team_name,omitempty"`
	PlayerID     int64   `json:"player_id"`
	PlayerName   string  `json:"player_name,omitempty"`
	PlayerNumber string  `json:"player_number,omitempty"`
	Handicap     float64 `json:"handicap"`
}

// ChecklistItem is one structured issue in a season setup checklist.
// Code is stable and machine-readable; Message is human-readable.
// TeamID is non-zero when the issue is specific to one team.
type ChecklistItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TeamID  int64  `json:"team_id,omitempty"`
}

// SetupChecklist is the response for GET /api/seasons/{id}/checklist.
// CanActivate is true when Blockers is empty.
type SetupChecklist struct {
	Blockers    []ChecklistItem `json:"blockers"`
	Warnings    []ChecklistItem `json:"warnings"`
	CanActivate bool            `json:"can_activate"`
}

// SeasonRule is a configurable rule for a season (e.g. max scoresheet handicap).
type SeasonRule struct {
	ID        int64  `json:"id"`
	SeasonID  int64  `json:"season_id"`
	RuleKey   string `json:"rule_key"`
	RuleLabel string `json:"rule_label"`
	RuleValue string `json:"rule_value"`
}

// SkippedWeek is a calendar date excluded from scheduling (holiday, break, etc.).
type SkippedWeek struct {
	ID       int64  `json:"id"`
	SeasonID int64  `json:"season_id"`
	SkipDate string `json:"skip_date"` // YYYY-MM-DD
	Reason   string `json:"reason"`
}

// ByeRequest records a team's request to not play a given week.
type ByeRequest struct {
	ID         int64  `json:"id"`
	SeasonID   int64  `json:"season_id"`
	TeamID     int64  `json:"team_id"`
	TeamName   string `json:"team_name,omitempty"`
	WeekNumber int    `json:"week_number"` // 0 = TBD/any
	Reason     string `json:"reason"`
	Approved   bool   `json:"approved"`
}

// Match represents a scheduled contest between two teams.
type Match struct {
	ID           int64     `json:"id"`
	SeasonID     int64     `json:"season_id"`
	LeagueID     int64     `json:"league_id,omitempty"`
	HomeTeamID   int64     `json:"home_team_id"`
	HomeTeamName string    `json:"home_team_name,omitempty"`
	AwayTeamID   int64     `json:"away_team_id"`
	AwayTeamName string    `json:"away_team_name,omitempty"`
	MatchDate    *string   `json:"match_date"`
	WeekNumber   int       `json:"week_number"`
	MatchNumber  *int      `json:"match_number,omitempty"`
	TableNumbers string    `json:"table_numbers,omitempty"`
	Completed    bool      `json:"completed"`
	CreatedAt    time.Time `json:"created_at"`

	// WeekClosed mirrors matches.week_closed, an existing column that was
	// not previously serialized on this model. Weekly Score Processing
	// Phase 1C exposes it (API-shape addition only -- no new column, no
	// migration) so the frontend can tell a closed-week match apart from
	// an open one without a second request, since the backend's own
	// approve/process/unprocess/unapprove guards all reject a closed week.
	WeekClosed bool `json:"week_closed"`

	// Weekly Score Processing Phase 1A: admin-attested approval/processing
	// state. Non-nil *At fields mean the corresponding action has happened.
	// UserID fields are nullable (admin-attested approval does not require
	// a personal-key user in this phase; see doc/domains/matches/README.md).
	ApprovedAt        *string `json:"approved_at,omitempty"`
	ApprovedByUserID  *int64  `json:"approved_by_user_id,omitempty"`
	ApprovalNote      string  `json:"approval_note,omitempty"`
	ProcessedAt       *string `json:"processed_at,omitempty"`
	ProcessedByUserID *int64  `json:"processed_by_user_id,omitempty"`
}

// MatchResult is a single player's performance within a match.
type MatchResult struct {
	ID         int64     `json:"id"`
	MatchID    int64     `json:"match_id"`
	PlayerID   int64     `json:"player_id"`
	PlayerName string    `json:"player_name,omitempty"`
	TeamID     int64     `json:"team_id"`
	SetsWon    int       `json:"sets_won"`
	SetsLost   int       `json:"sets_lost"`
	GamesWon   int       `json:"games_won"`
	GamesLost  int       `json:"games_lost"`
	Diff       float64   `json:"diff"` // point differential (8-ball)
	CreatedAt  time.Time `json:"created_at"`
}

// Standing is computed standings for a team in a season.
type Standing struct {
	TeamID    int64   `json:"team_id"`
	TeamName  string  `json:"team_name"`
	Played    int     `json:"played"`
	Wins      int     `json:"wins"`
	Losses    int     `json:"losses"`
	Ties      int     `json:"ties"`
	Points    int     `json:"points"`
	GamesWon  int     `json:"games_won"`
	GamesLost int     `json:"games_lost"`
	WinPct    float64 `json:"win_pct"`
}

// PlayerStat aggregates individual stats across a season.
type PlayerStat struct {
	PlayerID     int64   `json:"player_id"`
	PlayerNumber string  `json:"player_number"`
	PlayerName   string  `json:"player_name"`
	TeamName     string  `json:"team_name"`
	Handicap     float64 `json:"handicap"`
	SetsWon      int     `json:"sets_won"`
	SetsLost     int     `json:"sets_lost"`
	GamesWon     int     `json:"games_won"`
	GamesLost    int     `json:"games_lost"`
	WinPct       float64 `json:"win_pct"`
}

// MatchDetail bundles a match with its results for the entry screen.
type MatchDetail struct {
	Match   Match         `json:"match"`
	Results []MatchResult `json:"results"`
}

// PlayerOverview aggregates one player's identity, season/team context,
// schedule, stats, current handicap, and a money placeholder into one
// read-only response for the Player Overview screen (Player Overview
// Phase 1). Assembled by handler-level composition across existing
// managers -- not a persisted table or a new domain-owned service.
type PlayerOverview struct {
	Player   Player                 `json:"player"`
	Season   PlayerOverviewSeason   `json:"season"`
	Team     *PlayerOverviewTeam    `json:"team"`
	Handicap PlayerOverviewHandicap `json:"handicap"`
	Schedule []PlayerOverviewMatch  `json:"schedule"`
	Stats    PlayerOverviewStats    `json:"stats"`
	Money    PlayerOverviewMoney    `json:"money"`
}

// PlayerOverviewSeason is the minimal season context shown on the overview.
type PlayerOverviewSeason struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PlayerOverviewTeam is the minimal team context shown on the overview.
// Nil when the player has no resolvable team for the season (not on the
// season roster and no direct players.team_id).
type PlayerOverviewTeam struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PlayerOverviewHandicap carries the player's current handicap value.
// History/trend is out of scope for Phase 1.
type PlayerOverviewHandicap struct {
	Current float64 `json:"current"`
}

// PlayerOverviewMatch is one schedule row: a season match involving the
// player's resolved team, described from that team's perspective.
type PlayerOverviewMatch struct {
	MatchID          int64   `json:"match_id"`
	WeekNumber       int     `json:"week_number"`
	MatchDate        *string `json:"match_date"`
	OpponentTeamName string  `json:"opponent_team_name"`
	HomeOrAway       string  `json:"home_or_away"` // "home" | "away"
	Completed        bool    `json:"completed"`
}

// PlayerOverviewStats is the player's season stat totals. Zero-valued
// when the player has no match_results rows for the season yet.
type PlayerOverviewStats struct {
	SetsWon   int     `json:"sets_won"`
	SetsLost  int     `json:"sets_lost"`
	GamesWon  int     `json:"games_won"`
	GamesLost int     `json:"games_lost"`
	WinPct    float64 `json:"win_pct"`
}

// PlayerOverviewMoney is the player's season dues status (Player Overview
// Phase 2, backed by the finances domain added in Financial Phase 1).
// Tracked is true whenever a FinanceManager is wired; it is false only in
// test-only setups that omit one, in which case Message explains why
// money data is unavailable rather than showing a real Phase 1 "not
// tracked yet" claim. Paid is true whenever Payments is non-empty --
// there is no partial-payment/balance math. DuesAmount is read from the
// season_rules "dues_amount" freeform key (informational display only)
// and is empty when unset. Payout display is intentionally not included
// here -- payouts are team-level, not player-level, and are out of scope
// for Player Overview per PM decision.
type PlayerOverviewMoney struct {
	Tracked    bool          `json:"tracked"`
	Paid       bool          `json:"paid"`
	TotalPaid  float64       `json:"total_paid"`
	DuesAmount string        `json:"dues_amount,omitempty"`
	Payments   []DuesPayment `json:"payments"`
	Message    string        `json:"message,omitempty"`
}

// DuesPayment is one recorded dues payment for a player in a season
// (Financial Phase 1). Append-only history -- "paid" for a player/season
// means at least one row exists here; there is no update/delete path and
// no partial-payment/balance math. TeamID is a denormalized snapshot of
// the player's roster team at payment time.
type DuesPayment struct {
	ID               int64   `json:"id"`
	SeasonID         int64   `json:"season_id"`
	PlayerID         int64   `json:"player_id"`
	PlayerName       string  `json:"player_name,omitempty"`
	TeamID           *int64  `json:"team_id"`
	TeamName         string  `json:"team_name,omitempty"`
	Amount           float64 `json:"amount"`
	PaidAt           string  `json:"paid_at"`
	RecordedByUserID *int64  `json:"recorded_by_user_id,omitempty"`
	Note             string  `json:"note,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// Payout is one recorded payout to a team for a season (Financial Phase 1).
// Append-only history; the amount is always admin-entered -- standings are
// shown for reference only and never used to compute it automatically.
type Payout struct {
	ID               int64   `json:"id"`
	SeasonID         int64   `json:"season_id"`
	TeamID           int64   `json:"team_id"`
	TeamName         string  `json:"team_name,omitempty"`
	Amount           float64 `json:"amount"`
	RecordedByUserID *int64  `json:"recorded_by_user_id,omitempty"`
	Note             string  `json:"note,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// SeasonDuesResponse is the response for GET /api/seasons/{id}/finances/dues
// (Financial Phase 1). DuesAmount is read from the season_rules "dues_amount"
// freeform key (informational display only -- not enforced, no partial-payment
// math against it in Phase 1); empty when the rule has not been set.
type SeasonDuesResponse struct {
	SeasonID   int64           `json:"season_id"`
	DuesAmount string          `json:"dues_amount,omitempty"`
	Players    []PlayerDuesRow `json:"players"`
}

// PlayerDuesRow is one rostered player's dues status for the season.
// Paid is true when Payments is non-empty. Payments is ordered newest first.
type PlayerDuesRow struct {
	PlayerID     int64         `json:"player_id"`
	PlayerName   string        `json:"player_name"`
	PlayerNumber string        `json:"player_number,omitempty"`
	TeamID       int64         `json:"team_id"`
	TeamName     string        `json:"team_name,omitempty"`
	Paid         bool          `json:"paid"`
	TotalPaid    float64       `json:"total_paid"`
	Payments     []DuesPayment `json:"payments"`
}

// SeasonPayoutsResponse is the response for GET /api/seasons/{id}/finances/payouts
// (Financial Phase 1).
type SeasonPayoutsResponse struct {
	SeasonID int64           `json:"season_id"`
	Teams    []TeamPayoutRow `json:"teams"`
}

// TeamPayoutRow is one season team's payout history plus its current
// standing, shown for reference only -- Standing never determines Payouts.
type TeamPayoutRow struct {
	TeamID    int64     `json:"team_id"`
	TeamName  string    `json:"team_name"`
	TotalPaid float64   `json:"total_paid"`
	Payouts   []Payout  `json:"payouts"`
	Standing  *Standing `json:"standing,omitempty"`
}

// RoundResult stores point-per-game results for one player pairing within a match.
// Scoring: winner of each game gets 10 pts (7 balls × 1 pt + 8-ball × 3 pt).
// Loser gets however many balls they pocketed (0–7). All 3 games always played.
// Handicap = round(abs(homeHandicap − awayHandicap) × 2.55), given to lower-rated player.
// Computed fields (HandicapPts…PairingWinner) are derived on read, not stored.
type RoundResult struct {
	ID             int64   `json:"id"`
	MatchID        int64   `json:"match_id"`
	RoundNumber    int     `json:"round_number"`
	HomePlayerID   int64   `json:"home_player_id"`
	HomePlayerName string  `json:"home_player_name,omitempty"`
	HomeHandicap   float64 `json:"home_handicap,omitempty"`
	AwayPlayerID   int64   `json:"away_player_id"`
	AwayPlayerName string  `json:"away_player_name,omitempty"`
	AwayHandicap   float64 `json:"away_handicap,omitempty"`
	Game1Home      int     `json:"game1_home"` // pts scored by home player (0–10)
	Game1Away      int     `json:"game1_away"` // pts scored by away player (0–10)
	Game2Home      int     `json:"game2_home"`
	Game2Away      int     `json:"game2_away"`
	Game3Home      int     `json:"game3_home"`
	Game3Away      int     `json:"game3_away"`
	// Snapshot of handicap values at the time the round was played.
	// Prefer these over current player handicap for historical scoresheets.
	HomeHandicapUsed *float64 `json:"home_handicap_used,omitempty"`
	AwayHandicapUsed *float64 `json:"away_handicap_used,omitempty"`
	HandicapPtsUsed  *int     `json:"handicap_pts_used,omitempty"`
	HandicapToUsed   *string  `json:"handicap_to_used,omitempty"`
	// Computed on read — not stored:
	HandicapPts   int    `json:"handicap_pts,omitempty"`  // balls spotted
	HandicapTo    string `json:"handicap_to,omitempty"`   // "home"|"away"|""
	HomeTotalPts  int    `json:"home_total_pts,omitempty"` // raw + handicap if applicable
	AwayTotalPts  int    `json:"away_total_pts,omitempty"`
	PairingWinner string `json:"pairing_winner,omitempty"` // "home"|"away"|""
}

// WeekSummary is one entry in the GET /api/seasons/{id}/weeks response.
// Status is "open" when no league_weeks row exists (inferred) or the row has status "open".
// Status is "closed" after a successful POST /api/seasons/{id}/weeks/{week}/close.
type WeekSummary struct {
	WeekNumber     int     `json:"week_number"`
	Status         string  `json:"status"`          // "open" | "closed"
	ClosedAt       *string `json:"closed_at,omitempty"`
	MatchCount     int     `json:"match_count"`
	CompletedCount int     `json:"completed_count"` // matches with completed=1 (scores entered)
	ClosedCount    int     `json:"closed_count"`    // matches with week_closed=1 (officially closed)
	AckCount       int     `json:"ack_count"`       // total acknowledgment rows ever written for this week
}

// CloseAck is one row from week_close_acknowledgments.
// Returned by GET /api/seasons/{id}/weeks/{week}/acknowledgments.
type CloseAck struct {
	ID             int64  `json:"id"`
	SeasonID       int64  `json:"season_id"`
	WeekNumber     int    `json:"week_number"`
	MatchID        *int64 `json:"match_id,omitempty"`
	WarningCode    string `json:"warning_code"`
	Field          string `json:"field,omitempty"`
	Notes          string `json:"notes,omitempty"`
	AcknowledgedAt string `json:"acknowledged_at"`
}

// AdvancePreviewMessage mirrors validation.Message in the advance-preview response.
type AdvancePreviewMessage struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Level   string `json:"level"`
	MatchID *int64 `json:"match_id,omitempty"`
}

// AdvancePreviewWeekSummary holds match counts for a week in an advance preview.
type AdvancePreviewWeekSummary struct {
	MatchCount     int    `json:"match_count"`
	CompletedCount int    `json:"completed_count"`
	ClosedCount    int    `json:"closed_count"`
	Status         string `json:"status"`
}

// AdvancePreviewNextWeek holds readiness counts for the next scheduled week.
type AdvancePreviewNextWeek struct {
	MatchCount           int     `json:"match_count"`
	AssignedCount        int     `json:"assigned_count"`
	UnassignedCount      int     `json:"unassigned_count"`
	LineupPlanCount      int     `json:"lineup_plan_count"`
	MissingLineupTeamIDs []int64 `json:"missing_lineup_team_ids"`
}

// PlayerHandicapRec is one player's read-only handicap recommendation in an advance preview.
// The recommendation is computed from closed official match data only, using the same
// rack-windowed, eligibility-threshold-gated engine as the Handicap Review screen
// (handicaps.Service.Recommendations) so Week Recap and the pre-close advance preview
// can never disagree with the Handicap tab about whether a player is eligible.
// No changes are written anywhere; this is informational draft output only.
type PlayerHandicapRec struct {
	PlayerID            int64   `json:"player_id"`
	PlayerName          string  `json:"player_name"`
	CurrentHandicap     float64 `json:"current_handicap"`
	RecommendedHandicap float64 `json:"recommended_handicap"`
	// IncludedRacks is the count of eligible rack samples used in the calculation
	// (window-limited), matching HandicapReviewRec.IncludedRacks. Renamed from the
	// older "matches_played" -- the prior game-diff-average implementation counted
	// whole matches, but the unified rack-based engine counts individual racks, so
	// the old field name no longer described what was in it.
	IncludedRacks int    `json:"included_racks"`
	AdminHold     bool   `json:"admin_hold"`
	Skipped       bool   `json:"skipped"`
	Reason        string `json:"reason,omitempty"` // "no_data"|"admin_hold"|"below_threshold"|"no_change"|"capped"|"unsupported_method"
}

// HandicapReviewRec is one player's read-only entry for the Handicap Review screen.
// Population comes from season_teams + season_rosters (season-specific membership).
// Historical rack calculations are player-ID based and cross all 8-ball leagues/seasons.
// Only racks from completed, week_closed matches with a valid opponent HC snapshot are included.
// No changes are written anywhere; this is informational draft output only.
//
// Reason priority: no_data > admin_hold > below_threshold > capped > no_change > "" (actionable).
// RecommendedHC and ChangeAmount are nil for non-actionable players (no_data/admin_hold/below_threshold).
// LifetimeHC and WindowHC are nil when IncludedRacks == 0.
// All calculated HC values use 0.01 precision.
type HandicapReviewRec struct {
	// Identity
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	TeamName   string `json:"team_name"` // season-specific name from season_teams.season_name
	AdminHold  bool   `json:"admin_hold"`

	// Assigned handicap from players.handicap at query time
	AssignedHC float64 `json:"assigned_hc"`

	// Rack inventory
	ScoreEligibleRacks   int `json:"score_eligible_racks"`   // valid 8-ball slots before snapshot check
	MissingSnapshotRacks int `json:"missing_snapshot_racks"` // excluded: opponent snapshot NULL
	IncludedRacks        int `json:"included_racks"`         // used in calculation

	// Configuration echoed per-record for UI rendering
	WindowSize           int `json:"window_size"`
	EligibilityThreshold int `json:"eligibility_threshold"`

	// Calculated values -- nil when IncludedRacks == 0
	LifetimeHC    *float64 `json:"lifetime_hc"`
	LifetimeRacks int      `json:"lifetime_racks"`
	WindowHC      *float64 `json:"window_hc"`    // raw window value, before cap
	WindowRacks   int      `json:"window_racks"`

	// Recommendation -- nil when non-actionable (no_data/admin_hold/below_threshold)
	RecommendedHC *float64 `json:"recommended_hc"` // capped window value
	ChangeAmount  *float64 `json:"change_amount"`   // recommended_hc - assigned_hc

	// Reason: "" | "no_data" | "admin_hold" | "below_threshold" | "capped" | "no_change"
	Reason string `json:"reason"`

	// RecToken is an opaque versioned hash committing to the full recommendation
	// inputs. Populated only for actionable players (RecommendedHC != nil).
	// Required by the Apply endpoint to detect stale recommendations.
	RecToken string `json:"rec_token,omitempty"`
}

// HandicapReviewResponse is the response for GET /api/seasons/{id}/handicap-recommendations.
// Read-only; recommendations recompute live from completed=1 AND week_closed=1 data only.
// No writes are performed to players, handicap_history, or any other table.
type HandicapReviewResponse struct {
	SeasonID        int64               `json:"season_id"`
	Method          string              `json:"method"`
	Status          string              `json:"status"`
	Message         string              `json:"message"`
	WeeksClosed     int                 `json:"weeks_closed"`
	Recommendations []HandicapReviewRec `json:"recommendations"`
}

// AdvancePreviewHandicap summarizes the handicap update mode for an advance preview.
// Recommendations is populated only when method is "game_diff_average" and closed
// match data exists. It is absent (omitempty) for "manual_review" and "kicker_average_preview".
type AdvancePreviewHandicap struct {
	Method          string             `json:"method"`
	Status          string             `json:"status"`
	Message         string             `json:"message"`
	Recommendations []PlayerHandicapRec `json:"recommendations,omitempty"`
}

// AdvancePreview is the response for GET /api/seasons/{id}/weeks/{week}/advance-preview.
// Read-only; no data is modified by this endpoint.
type AdvancePreview struct {
	SeasonID           int64                     `json:"season_id"`
	WeekNumber         int                       `json:"week_number"`
	CanClose           bool                      `json:"can_close"`
	ValidationMessages []AdvancePreviewMessage   `json:"validation_messages"`
	CurrentWeek        AdvancePreviewWeekSummary `json:"current_week"`
	NextWeekNumber     *int                      `json:"next_week_number,omitempty"`
	NextWeek           *AdvancePreviewNextWeek   `json:"next_week,omitempty"`
	Handicap           AdvancePreviewHandicap    `json:"handicap"`
}

// AdvanceResult is embedded in the POST close response after a successful close.
// It summarizes the state immediately after the week transaction commits.
type AdvanceResult struct {
	Message        string                    `json:"message"`
	ClosedWeek     AdvancePreviewWeekSummary `json:"closed_week"`
	NextWeekNumber *int                      `json:"next_week_number,omitempty"`
	NextWeek       *AdvancePreviewNextWeek   `json:"next_week,omitempty"`
	Handicap       AdvancePreviewHandicap    `json:"handicap"`
}

// RecapMatchRow is one match entry in a week-end recap.
// HasResult is true when completed=1 (scores were entered). Set and game counts
// are 0 when HasResult is false. TeamIDs are nil for unassigned matches.
// ApprovedAt, ProcessedAt, and WeekClosed (Weekly Summary Phase 1) mirror the
// same fields already on models.Match, letting a recap consumer show the
// full unscored/scored/approved/processed/closed status ladder for each
// match without a second request against GET /api/matches.
type RecapMatchRow struct {
	MatchID      int64   `json:"match_id"`
	HomeTeamID   *int64  `json:"home_team_id"`
	HomeTeamName string  `json:"home_team_name,omitempty"`
	AwayTeamID   *int64  `json:"away_team_id"`
	AwayTeamName string  `json:"away_team_name,omitempty"`
	MatchDate    *string `json:"match_date,omitempty"`
	HasResult    bool    `json:"has_result"`
	HomeSetsWon  int     `json:"home_sets_won"`
	AwaySetsWon  int     `json:"away_sets_won"`
	HomeGamesWon int     `json:"home_games_won"`
	AwayGamesWon int     `json:"away_games_won"`
	ApprovedAt   *string `json:"approved_at,omitempty"`
	ProcessedAt  *string `json:"processed_at,omitempty"`
	WeekClosed   bool    `json:"week_closed"`
}

// RecapPlayerStat is one player's stat totals for a week in a week-end recap.
// Derived from match_results joined to matches by season_id and week_number.
// No schema changes are required; all fields come from existing columns.
type RecapPlayerStat struct {
	PlayerID   int64   `json:"player_id"`
	PlayerName string  `json:"player_name"`
	TeamName   string  `json:"team_name,omitempty"`
	SetsWon    int     `json:"sets_won"`
	SetsLost   int     `json:"sets_lost"`
	GamesWon   int     `json:"games_won"`
	GamesLost  int     `json:"games_lost"`
	Diff       float64 `json:"diff"`
	// IsSub and SubForName are Substitute Workflow Phase 1: true/populated
	// when this player has a lineup_plans row for this season/team/week with
	// is_sub=1. Resolved by matching mr.team_id/mr.player_id against
	// lineup_plans -- a player who subbed without ever being added to
	// lineup_plans (scored directly, no lineup row at all) shows IsSub=false
	// here even if they were a real substitute; this reflects the recorded
	// lineup, not just match_results.
	IsSub      bool   `json:"is_sub,omitempty"`
	SubForName string `json:"sub_for_name,omitempty"`
}

// RecapHandicapChange is one applied handicap change from handicap_history for a
// specific season and week. PlayerName comes from player_name_snapshot (the name
// at apply time). Returned in WeekRecap.HandicapChanges.
type RecapHandicapChange struct {
	PlayerName  string  `json:"player_name"`
	OldHandicap float64 `json:"old_handicap"`
	NewHandicap float64 `json:"new_handicap"`
}

// WeekRecap is the response for GET /api/seasons/{id}/weeks/{week}/recap.
// Read-only; no data is modified by this endpoint.
// Acknowledgments, next-week readiness, and handicap sections are included
// so the frontend requires no additional requests to render the recap view.
type WeekRecap struct {
	SeasonID        int64                   `json:"season_id"`
	WeekNumber      int                     `json:"week_number"`
	Status          string                  `json:"status"`
	ClosedAt        *string                 `json:"closed_at,omitempty"`
	Matches         []RecapMatchRow         `json:"matches"`
	MissingCount    int                     `json:"missing_count"`
	PlayerStats     []RecapPlayerStat       `json:"player_stats"`
	HandicapChanges []RecapHandicapChange   `json:"handicap_changes"`
	Acknowledgments []CloseAck              `json:"acknowledgments"`
	NextWeekNumber  *int                    `json:"next_week_number,omitempty"`
	NextWeek        *AdvancePreviewNextWeek `json:"next_week,omitempty"`
	Handicap        AdvancePreviewHandicap  `json:"handicap"`
}

// SaveRoundsRequest is the body for POST /api/matches/{id}/rounds.
type SaveRoundsRequest struct {
	Rounds []RoundResult `json:"rounds"`
}

// AssignMatchTeamsRequest is the body for PATCH /api/matches/{id}/assign.
type AssignMatchTeamsRequest struct {
	HomeTeamID *int64 `json:"home_team_id"` // nil clears the assignment
	AwayTeamID *int64 `json:"away_team_id"`
}

// SubmitResultsRequest is the body for POST /api/matches/{id}/results.
type SubmitResultsRequest struct {
	Results []MatchResult `json:"results"`
}

// LineupPlan records a player's planned lineup slot for a match week (pre-game).
type LineupPlan struct {
	ID         int64   `json:"id"`
	SeasonID   int64   `json:"season_id"`
	TeamID     int64   `json:"team_id"`
	TeamName   string  `json:"team_name,omitempty"`
	PlayerID   int64   `json:"player_id"`
	PlayerName string  `json:"player_name,omitempty"`
	Handicap   float64 `json:"handicap,omitempty"`
	WeekNumber int     `json:"week_number"`
	IsSub      bool    `json:"is_sub"`
	SubForID   *int64  `json:"sub_for_id,omitempty"`
}

// SaveTeamLineupRequest is the body for POST /api/lineup-plans.
// Replaces all slots for a team/week atomically.
type SaveTeamLineupRequest struct {
	SeasonID   int64   `json:"season_id"`
	TeamID     int64   `json:"team_id"`
	WeekNumber int     `json:"week_number"`
	PlayerIDs  []int64 `json:"player_ids"` // ordered: slot 1, 2, 3
}

// User is an application user with an API key for authenticated operations.
// APIKeyHash is intentionally excluded from JSON to prevent accidental exposure.
// The cleartext key is returned once at create time (via CreateUserResponse) and never stored.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
	// PlayerID and PlayerName are Player Account Access Phase 1: the
	// optional one-to-one link from a role=player user to their player
	// record. Both are nil/empty for every system_admin/league_admin user.
	// PlayerName is a display convenience populated only by ListApplyUsers
	// (a LEFT JOIN for the Users Admin screen) -- ResolveApplyUserByAPIKey
	// leaves it empty, since only PlayerID is needed for the auth/ownership
	// check on Player Overview and elsewhere.
	PlayerID   *int64 `json:"player_id,omitempty"`
	PlayerName string `json:"player_name,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// CreateUserResponse is the one-time response body for POST /api/users.
// APIKey is the cleartext key — show once, not re-retrievable.
type CreateUserResponse struct {
	User   User   `json:"user"`
	APIKey string `json:"api_key"`
}
