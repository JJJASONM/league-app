package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/handicaps"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// ============================================================================
// Test helpers
// ============================================================================

func initDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
}

// seedLeague inserts one league with the given game_format, returns league ID.
func seedLeague(t *testing.T, gameFormat string) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('Test', ?) RETURNING id`, gameFormat).Scan(&id); err != nil {
		t.Fatalf("insert league: %v", err)
	}
	return id
}

// seedSeason inserts a season for the given league, returns season ID.
func seedSeason(t *testing.T, leagueID int64) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?, 'S1', '2026-01-01', 'single_rr', 8) RETURNING id`, leagueID).Scan(&id); err != nil {
		t.Fatalf("insert season: %v", err)
	}
	return id
}

// seedTeam inserts a team for the given league, returns team ID.
func seedTeam(t *testing.T, leagueID int64, name string) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(`INSERT INTO teams (league_id, name) VALUES (?, ?) RETURNING id`, leagueID, name).Scan(&id); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	return id
}

// seedPlayer inserts a player for the given team, returns player ID.
func seedPlayer(t *testing.T, teamID int64, first, last string, hc float64) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES (?, ?, ?, ?) RETURNING id`, first, last, teamID, hc).Scan(&id); err != nil {
		t.Fatalf("insert player: %v", err)
	}
	return id
}

// seedMatch inserts a match, returns match ID.
func seedMatch(t *testing.T, seasonID, homeTeam, awayTeam int64, date string, weekNum, completed, weekClosed int) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number, match_number, completed, week_closed) VALUES (?, ?, ?, ?, ?, 1, ?, ?) RETURNING id`,
		seasonID, homeTeam, awayTeam, date, weekNum, completed, weekClosed).Scan(&id); err != nil {
		t.Fatalf("insert match: %v", err)
	}
	return id
}

// seedRound inserts one round_results row.
func seedRound(t *testing.T, matchID, homePID, awayPID int64, g1h, g1a, g2h, g2a, g3h, g3a int, homeHC, awayHC *float64) {
	t.Helper()
	var homeHCVal, awayHCVal interface{}
	if homeHC != nil {
		homeHCVal = *homeHC
	}
	if awayHC != nil {
		awayHCVal = *awayHC
	}
	if _, err := db.DB.Exec(`INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id, game1_home, game1_away, game2_home, game2_away, game3_home, game3_away, home_handicap_used, away_handicap_used) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		matchID, homePID, awayPID, g1h, g1a, g2h, g2a, g3h, g3a, homeHCVal, awayHCVal); err != nil {
		t.Fatalf("insert round_result: %v", err)
	}
}

func fp(v float64) *float64 { return &v }

// ============================================================================
// SeasonExists
// ============================================================================

func TestSeasonExists_PresentReturnsTrue(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	store := sqlite.NewHandicapStore(db.DB)
	ok, err := store.SeasonExists(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonExists: %v", err)
	}
	if !ok {
		t.Error("want true for existing season")
	}
}

func TestSeasonExists_AbsentReturnsFalse(t *testing.T) {
	initDB(t)
	store := sqlite.NewHandicapStore(db.DB)
	ok, err := store.SeasonExists(context.Background(), 9999)
	if err != nil {
		t.Fatalf("SeasonExists: %v", err)
	}
	if ok {
		t.Error("want false for absent season")
	}
}

// ============================================================================
// ClosedWeekCount
// ============================================================================

func TestClosedWeekCount_NoMatches_ReturnsZero(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	store := sqlite.NewHandicapStore(db.DB)
	n, err := store.ClosedWeekCount(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("ClosedWeekCount: %v", err)
	}
	if n != 0 {
		t.Errorf("want 0, got %d", n)
	}
}

func TestClosedWeekCount_OneClosedWeek_ReturnsOne(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "A")
	seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1)

	store := sqlite.NewHandicapStore(db.DB)
	n, err := store.ClosedWeekCount(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("ClosedWeekCount: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1, got %d", n)
	}
}

func TestClosedWeekCount_TwoMatchesSameWeek_ReturnsOne(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "A")
	// Two matches in week 1 -- distinct week count should be 1.
	seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1)
	seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1)

	store := sqlite.NewHandicapStore(db.DB)
	n, err := store.ClosedWeekCount(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("ClosedWeekCount: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1 distinct week, got %d", n)
	}
}

// ============================================================================
// SeasonHandicapRules -- nil, blank, partial, invalid-raw
// ============================================================================

func TestSeasonHandicapRules_AllAbsent_AllNil(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	store := sqlite.NewHandicapStore(db.DB)
	row, err := store.SeasonHandicapRules(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonHandicapRules: %v", err)
	}
	if row.UpdateMethod != nil || row.WindowSize != nil || row.Threshold != nil || row.MaxHC != nil {
		t.Errorf("want all nil for absent rules, got %+v", row)
	}
}

func TestSeasonHandicapRules_PresentKeys_ReturnRawValues(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'handicap_update_method', '', 'game_diff_average')`, seasonID)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'max_individual_handicap', '', '4.5')`, seasonID)

	store := sqlite.NewHandicapStore(db.DB)
	row, err := store.SeasonHandicapRules(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonHandicapRules: %v", err)
	}
	if row.UpdateMethod == nil || *row.UpdateMethod != "game_diff_average" {
		t.Errorf("want UpdateMethod=game_diff_average, got %v", row.UpdateMethod)
	}
	if row.MaxHC == nil || *row.MaxHC != "4.5" {
		t.Errorf("want MaxHC=4.5, got %v", row.MaxHC)
	}
	// Absent keys remain nil.
	if row.WindowSize != nil {
		t.Errorf("want WindowSize nil, got %v", row.WindowSize)
	}
}

// Blank stored value: row present, rule_value="". Adapter returns non-nil ptr to "".
// Service treats nil and blank the same (default), so the adapter must not hide the row.
func TestSeasonHandicapRules_BlankValue_NonNilPtrToEmptyString(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'handicap_current_game_window', '', '')`, seasonID)

	store := sqlite.NewHandicapStore(db.DB)
	row, err := store.SeasonHandicapRules(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonHandicapRules: %v", err)
	}
	if row.WindowSize == nil {
		t.Error("want non-nil ptr for present but blank window rule")
	} else if *row.WindowSize != "" {
		t.Errorf("want empty string, got %q", *row.WindowSize)
	}
}

// Adapter must return invalid values raw and unmodified. Validation is the
// service's responsibility; the adapter must not silently alter stored values.
func TestSeasonHandicapRules_InvalidValue_ReturnedRawUnmodified(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	const storedVal = "not-a-number"
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'handicap_current_game_window', '', ?)`, seasonID, storedVal)

	store := sqlite.NewHandicapStore(db.DB)
	row, err := store.SeasonHandicapRules(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonHandicapRules: %v", err)
	}
	if row.WindowSize == nil || *row.WindowSize != storedVal {
		t.Errorf("want raw value %q, got %v", storedVal, row.WindowSize)
	}
}

// ============================================================================
// SeasonRoster -- empty, one player, ordering
// ============================================================================

func TestSeasonRoster_Empty_ReturnsNil(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	store := sqlite.NewHandicapStore(db.DB)
	roster, err := store.SeasonRoster(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonRoster: %v", err)
	}
	if len(roster) != 0 {
		t.Errorf("want empty roster, got %d", len(roster))
	}
}

func TestSeasonRoster_OnePlayer_Returned(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "Rack City")
	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?, ?, 'Rack City S1')`, seasonID, teamID)
	playerID := seedPlayer(t, teamID, "Jane", "Doe", 2.5)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?, ?, ?)`, seasonID, teamID, playerID)

	store := sqlite.NewHandicapStore(db.DB)
	roster, err := store.SeasonRoster(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonRoster: %v", err)
	}
	if len(roster) != 1 {
		t.Fatalf("want 1 entry, got %d", len(roster))
	}
	e := roster[0]
	if e.PlayerID != playerID {
		t.Errorf("want player_id=%d, got %d", playerID, e.PlayerID)
	}
	if e.PlayerName != "Jane Doe" {
		t.Errorf("want name=Jane Doe, got %q", e.PlayerName)
	}
	if e.TeamName != "Rack City S1" {
		t.Errorf("want team=Rack City S1, got %q", e.TeamName)
	}
	if e.AssignedHC != 2.5 {
		t.Errorf("want hc=2.5, got %v", e.AssignedHC)
	}
}

// Roster is ordered by (season_teams.season_name, last_name, first_name).
// Alpha team comes before Beta team regardless of insertion order.
func TestSeasonRoster_OrderedByTeamThenLastFirst(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)

	betaTeam := seedTeam(t, leagueID, "Beta Raw")
	alphaTeam := seedTeam(t, leagueID, "Alpha Raw")
	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?, ?, 'Beta')`, seasonID, betaTeam)
	db.DB.Exec(`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?, ?, 'Alpha')`, seasonID, alphaTeam)

	// Seed Beta player first (insertion order is intentionally reversed).
	betaPlayer := seedPlayer(t, betaTeam, "Zach", "Smith", 1.0)
	alphaPlayer := seedPlayer(t, alphaTeam, "Alice", "Jones", 2.0)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?, ?, ?)`, seasonID, betaTeam, betaPlayer)
	db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?, ?, ?)`, seasonID, alphaTeam, alphaPlayer)

	store := sqlite.NewHandicapStore(db.DB)
	roster, err := store.SeasonRoster(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonRoster: %v", err)
	}
	if len(roster) != 2 {
		t.Fatalf("want 2 entries, got %d", len(roster))
	}
	// Alpha comes before Beta.
	if roster[0].TeamName != "Alpha" {
		t.Errorf("want first entry team=Alpha (season_name order), got %q", roster[0].TeamName)
	}
	if roster[1].TeamName != "Beta" {
		t.Errorf("want second entry team=Beta, got %q", roster[1].TeamName)
	}
}

// ============================================================================
// EligibleRacks -- empty IDs, inclusion, exclusions, snapshots, ordering
// ============================================================================

func TestEligibleRacks_EmptyPlayerIDs_ReturnsNil(t *testing.T) {
	initDB(t)
	store := sqlite.NewHandicapStore(db.DB)
	rows, err := store.EligibleRacks(context.Background(), nil)
	if err != nil {
		t.Fatalf("EligibleRacks: %v", err)
	}
	if rows != nil {
		t.Errorf("want nil for empty playerIDs, got %v", rows)
	}
}

// A completed, week-closed, 8ball match is included.
func TestEligibleRacks_CompletedClosedEightBall_Included(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "T")
	homePID := seedPlayer(t, teamID, "H", "P", 1.5)
	awayPID := seedPlayer(t, teamID, "A", "P", 2.0)
	matchID := seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1) // completed=1, week_closed=1
	seedRound(t, matchID, homePID, awayPID, 10, 7, 10, 5, 10, 3, fp(1.5), fp(2.0))

	store := sqlite.NewHandicapStore(db.DB)
	racks, err := store.EligibleRacks(context.Background(), []int64{homePID})
	if err != nil {
		t.Fatalf("EligibleRacks: %v", err)
	}
	if len(racks) != 1 {
		t.Fatalf("want 1 rack, got %d", len(racks))
	}
	r := racks[0]
	if r.HomePlayerID != homePID || r.AwayPlayerID != awayPID {
		t.Errorf("want home=%d away=%d, got home=%d away=%d", homePID, awayPID, r.HomePlayerID, r.AwayPlayerID)
	}
}

// A match with week_closed=0 (open week) is excluded.
func TestEligibleRacks_OpenWeek_Excluded(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "T")
	homePID := seedPlayer(t, teamID, "H", "P", 1.5)
	awayPID := seedPlayer(t, teamID, "A", "P", 2.0)
	matchID := seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 0) // week_closed=0
	seedRound(t, matchID, homePID, awayPID, 10, 7, 10, 5, 10, 3, fp(1.5), fp(2.0))

	store := sqlite.NewHandicapStore(db.DB)
	racks, err := store.EligibleRacks(context.Background(), []int64{homePID})
	if err != nil {
		t.Fatalf("EligibleRacks: %v", err)
	}
	if len(racks) != 0 {
		t.Errorf("want 0 racks for open week, got %d", len(racks))
	}
}

// A match in a non-8ball league is excluded.
func TestEligibleRacks_NonEightBall_Excluded(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "9ball") // not 8ball
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "T")
	homePID := seedPlayer(t, teamID, "H", "P", 1.5)
	awayPID := seedPlayer(t, teamID, "A", "P", 2.0)
	matchID := seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1)
	seedRound(t, matchID, homePID, awayPID, 10, 7, 10, 5, 10, 3, fp(1.5), fp(2.0))

	store := sqlite.NewHandicapStore(db.DB)
	racks, err := store.EligibleRacks(context.Background(), []int64{homePID})
	if err != nil {
		t.Fatalf("EligibleRacks: %v", err)
	}
	if len(racks) != 0 {
		t.Errorf("want 0 racks for non-8ball, got %d", len(racks))
	}
}

// NULL home_handicap_used is converted to nil HomeHCUsed.
func TestEligibleRacks_NullSnapshot_ConvertedToNil(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "T")
	homePID := seedPlayer(t, teamID, "H", "P", 1.5)
	awayPID := seedPlayer(t, teamID, "A", "P", 2.0)
	matchID := seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1)
	// Pass nil for both snapshots.
	seedRound(t, matchID, homePID, awayPID, 10, 7, 10, 5, 10, 3, nil, nil)

	store := sqlite.NewHandicapStore(db.DB)
	racks, err := store.EligibleRacks(context.Background(), []int64{homePID})
	if err != nil {
		t.Fatalf("EligibleRacks: %v", err)
	}
	if len(racks) != 1 {
		t.Fatalf("want 1 rack, got %d", len(racks))
	}
	if racks[0].HomeHCUsed != nil {
		t.Errorf("want HomeHCUsed nil for NULL DB value, got %v", racks[0].HomeHCUsed)
	}
	if racks[0].AwayHCUsed != nil {
		t.Errorf("want AwayHCUsed nil for NULL DB value, got %v", racks[0].AwayHCUsed)
	}
}

// Non-NULL snapshots are converted to non-nil pointers with the correct value.
func TestEligibleRacks_NonNullSnapshot_ConvertedToPointer(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "T")
	homePID := seedPlayer(t, teamID, "H", "P", 1.5)
	awayPID := seedPlayer(t, teamID, "A", "P", 2.0)
	matchID := seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1)
	seedRound(t, matchID, homePID, awayPID, 10, 7, 10, 5, 10, 3, fp(1.5), fp(2.0))

	store := sqlite.NewHandicapStore(db.DB)
	racks, err := store.EligibleRacks(context.Background(), []int64{homePID})
	if err != nil {
		t.Fatalf("EligibleRacks: %v", err)
	}
	if len(racks) != 1 {
		t.Fatalf("want 1 rack, got %d", len(racks))
	}
	if racks[0].HomeHCUsed == nil || *racks[0].HomeHCUsed != 1.5 {
		t.Errorf("want HomeHCUsed=1.5, got %v", racks[0].HomeHCUsed)
	}
	if racks[0].AwayHCUsed == nil || *racks[0].AwayHCUsed != 2.0 {
		t.Errorf("want AwayHCUsed=2.0, got %v", racks[0].AwayHCUsed)
	}
}

// Most-recent-first ordering: match with later date appears before earlier one.
func TestEligibleRacks_MostRecentFirst(t *testing.T) {
	initDB(t)
	leagueID := seedLeague(t, "8ball")
	seasonID := seedSeason(t, leagueID)
	teamID := seedTeam(t, leagueID, "T")
	homePID := seedPlayer(t, teamID, "H", "P", 1.5)
	awayPID := seedPlayer(t, teamID, "A", "P", 2.0)

	// Insert older match first so insertion order != expected result order.
	oldMatchID := seedMatch(t, seasonID, teamID, teamID, "2026-01-07", 1, 1, 1)
	newMatchID := seedMatch(t, seasonID, teamID, teamID, "2026-01-14", 2, 1, 1)
	seedRound(t, oldMatchID, homePID, awayPID, 7, 10, 5, 10, 3, 10, fp(2.0), fp(1.5)) // old: player scores 7
	seedRound(t, newMatchID, homePID, awayPID, 10, 7, 10, 5, 10, 3, fp(1.5), fp(2.0)) // new: player scores 10

	store := sqlite.NewHandicapStore(db.DB)
	racks, err := store.EligibleRacks(context.Background(), []int64{homePID})
	if err != nil {
		t.Fatalf("EligibleRacks: %v", err)
	}
	if len(racks) != 2 {
		t.Fatalf("want 2 racks, got %d", len(racks))
	}
	// Newer match (home wins) should be first.
	if racks[0].G1H != 10 {
		t.Errorf("want first rack from newer match (G1H=10), got G1H=%d", racks[0].G1H)
	}
	if racks[1].G1H != 7 {
		t.Errorf("want second rack from older match (G1H=7), got G1H=%d", racks[1].G1H)
	}
}

// ============================================================================
// RunTx basic contracts (no-write tests)
// ============================================================================

func TestRunTx_ErrorPropagated(t *testing.T) {
	initDB(t)
	store := sqlite.NewHandicapStore(db.DB)
	sentinel := errors.New("abort")
	err := store.RunTx(context.Background(), func(tx handicaps.Store) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("want sentinel error propagated, got %v", err)
	}
}

func TestRunTx_NilErrorCommits(t *testing.T) {
	initDB(t)
	seasonID := seedSeason(t, seedLeague(t, "8ball"))
	store := sqlite.NewHandicapStore(db.DB)

	var gotExists bool
	err := store.RunTx(context.Background(), func(tx handicaps.Store) error {
		ok, e := tx.SeasonExists(context.Background(), seasonID)
		gotExists = ok
		return e
	})
	if err != nil {
		t.Fatalf("RunTx: %v", err)
	}
	if !gotExists {
		t.Error("want SeasonExists=true inside committed transaction")
	}
}
