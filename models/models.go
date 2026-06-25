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

// AdvancePreviewHandicap summarizes the handicap update mode for an advance preview.
type AdvancePreviewHandicap struct {
	Method  string `json:"method"`
	Status  string `json:"status"`
	Message string `json:"message"`
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

// SaveRoundsRequest is the body for POST /api/matches/{id}/rounds.
type SaveRoundsRequest struct {
	Rounds []RoundResult `json:"rounds"`
}

// GenerateScheduleRequest is the body for POST /api/matches/generate.
type GenerateScheduleRequest struct {
	SeasonID     int64    `json:"season_id"`
	StartDate    string   `json:"start_date"`    // YYYY-MM-DD
	ScheduleType string   `json:"schedule_type"` // "single_rr"|"double_rr"|"split"|"custom"|"blanket"
	NumWeeks     int      `json:"num_weeks"`     // for "custom" and "blanket"
	MatchesPerWeek int    `json:"matches_per_week"` // for "blanket" only
	SkipDates    []string `json:"skip_dates"`    // YYYY-MM-DD dates to skip
	FromSeasonID int64    `json:"from_season_id"` // use teams from this season's schedule (0 = all league teams)
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
