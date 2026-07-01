package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/models"
)

var errRollbackTest = errors.New("intentional rollback")

// seedRoundTestData inserts the minimum rows needed for round store tests:
// league → team × 2 → player × 2 → season → match.
// Returns (matchID, homePlayerID, awayPlayerID, seasonID, homeTeamID, awayTeamID).
func seedRoundTestData(t *testing.T) (matchID, homePlayerID, awayPlayerID, seasonID, homeTeamID, awayTeamID int64) {
	t.Helper()
	res, _ := db.DB.Exec(`INSERT INTO leagues (name, game_format) VALUES ('Test League','8ball')`)
	leagueID, _ := res.LastInsertId()

	res, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,?)`, leagueID, "Home Team")
	homeTeamID, _ = res.LastInsertId()

	res, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,?)`, leagueID, "Away Team")
	awayTeamID, _ = res.LastInsertId()

	res, _ = db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Alice','A',?,1.0)`, homeTeamID)
	homePlayerID, _ = res.LastInsertId()

	res, _ = db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Bob','B',?,2.0)`, awayTeamID)
	awayPlayerID, _ = res.LastInsertId()

	res, _ = db.DB.Exec(`INSERT INTO seasons (league_id, name) VALUES (?,?)`, leagueID, "S1")
	seasonID, _ = res.LastInsertId()

	res, _ = db.DB.Exec(`
		INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, match_number)
		VALUES (?,?,?,1,1)`, seasonID, homeTeamID, awayTeamID)
	matchID, _ = res.LastInsertId()
	return
}

// newRoundStore returns a fresh RoundStore and test DB.
func newRoundStore(t *testing.T) *sqlite.RoundStore {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewRoundStore(db.DB)
}

// ─── IsWeekClosed ────────────────────────────────────────────────────────────

func TestRoundStore_IsWeekClosed_DefaultFalse(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	closed, err := s.IsWeekClosed(context.Background(), matchID)
	if err != nil {
		t.Fatalf("IsWeekClosed: %v", err)
	}
	if closed {
		t.Error("want false (not closed), got true")
	}
}

func TestRoundStore_IsWeekClosed_TrueAfterSet(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET week_closed=1 WHERE id=?`, matchID)
	closed, err := s.IsWeekClosed(context.Background(), matchID)
	if err != nil {
		t.Fatalf("IsWeekClosed: %v", err)
	}
	if !closed {
		t.Error("want true after week_closed=1, got false")
	}
}

// ─── SeasonRoundConfig ───────────────────────────────────────────────────────

func TestRoundStore_SeasonRoundConfig_Defaults(t *testing.T) {
	s := newRoundStore(t)
	_, _, _, seasonID, _, _ := seedRoundTestData(t)
	cfg, err := s.SeasonRoundConfig(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonRoundConfig: %v", err)
	}
	if cfg.Multiplier != 2.55 {
		t.Errorf("want default multiplier 2.55, got %v", cfg.Multiplier)
	}
	if cfg.MinBallHC != 0 {
		t.Errorf("want default MinBallHC 0, got %d", cfg.MinBallHC)
	}
}

func TestRoundStore_SeasonRoundConfig_ReadsStoredMultiplier(t *testing.T) {
	s := newRoundStore(t)
	_, _, _, seasonID, _, _ := seedRoundTestData(t)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		seasonID, "handicap_multiplier", "Multiplier", "3.00")
	cfg, err := s.SeasonRoundConfig(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("SeasonRoundConfig: %v", err)
	}
	if cfg.Multiplier != 3.00 {
		t.Errorf("want multiplier 3.00, got %v", cfg.Multiplier)
	}
}

func TestRoundStore_SeasonRoundConfig_InvalidMultiplier_ReturnsError(t *testing.T) {
	s := newRoundStore(t)
	_, _, _, seasonID, _, _ := seedRoundTestData(t)
	db.DB.Exec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		seasonID, "handicap_multiplier", "Multiplier", "not-a-number")
	_, err := s.SeasonRoundConfig(context.Background(), seasonID)
	if err == nil {
		t.Error("want error for invalid multiplier, got nil")
	}
}

// ─── RunTx rollback ──────────────────────────────────────────────────────────

func TestRoundStore_RunTx_RollbackOnError(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, _, _, _ := seedRoundTestData(t)

	insertErr := s.RunTx(context.Background(), func(tx matches.RoundStore) error {
		row := matches.RoundResultRow{
			MatchID: matchID, RoundNumber: 1,
			HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID,
			Game1Home: 10, Game1Away: 3,
		}
		_ = tx.InsertRoundResult(context.Background(), row)
		return errRollbackTest
	})
	if insertErr == nil {
		t.Fatal("want error from RunTx, got nil")
	}
	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM round_results WHERE match_id=?`, matchID).Scan(&count)
	if count != 0 {
		t.Errorf("want 0 rows after rollback, got %d", count)
	}
}

// ─── Full write cycle ─────────────────────────────────────────────────────────

func TestRoundStore_FullWriteCycle(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, _, homeTeamID, awayTeamID := seedRoundTestData(t)

	err := s.RunTx(context.Background(), func(tx matches.RoundStore) error {
		row := matches.RoundResultRow{
			MatchID: matchID, RoundNumber: 1,
			HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID,
			Game1Home: 10, Game1Away: 3,
			Game2Home: 10, Game2Away: 5,
			Game3Home: 10, Game3Away: 2,
			HomeHCUsed: 1.0, AwayHCUsed: 2.0, HandicapPtsUsed: 3, HandicapTo: "home",
		}
		if err := tx.InsertRoundResult(context.Background(), row); err != nil {
			return err
		}
		if err := tx.InsertMatchResult(context.Background(), matches.MatchResultRow{
			MatchID: matchID, PlayerID: homePlayerID, TeamID: homeTeamID,
			GamesWon: 3, GamesLost: 0, Diff: 3, SetsWon: 1, SetsLost: 0,
		}); err != nil {
			return err
		}
		if err := tx.InsertMatchResult(context.Background(), matches.MatchResultRow{
			MatchID: matchID, PlayerID: awayPlayerID, TeamID: awayTeamID,
			GamesWon: 0, GamesLost: 3, Diff: -3, SetsWon: 0, SetsLost: 1,
		}); err != nil {
			return err
		}
		return tx.MarkMatchCompleted(context.Background(), matchID)
	})
	if err != nil {
		t.Fatalf("RunTx: %v", err)
	}

	var rrCount, mrCount, completed int
	db.DB.QueryRow(`SELECT COUNT(*) FROM round_results WHERE match_id=?`, matchID).Scan(&rrCount)
	db.DB.QueryRow(`SELECT COUNT(*) FROM match_results WHERE match_id=?`, matchID).Scan(&mrCount)
	db.DB.QueryRow(`SELECT completed FROM matches WHERE id=?`, matchID).Scan(&completed)

	if rrCount != 1 {
		t.Errorf("want 1 round_result, got %d", rrCount)
	}
	if mrCount != 2 {
		t.Errorf("want 2 match_results, got %d", mrCount)
	}
	if completed != 1 {
		t.Errorf("want completed=1, got %d", completed)
	}
}

// ─── GetRoundResults ─────────────────────────────────────────────────────────

func TestRoundStore_GetRoundResults_ReturnsRows(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, _, _, _ := seedRoundTestData(t)

	db.DB.Exec(`
		INSERT INTO round_results
		  (match_id, round_number, home_player_id, away_player_id,
		   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		   home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,3,10,5,10,2,1.0,2.0,3,'home')`,
		matchID, homePlayerID, awayPlayerID)

	rows, err := s.GetRoundResults(context.Background(), matchID)
	if err != nil {
		t.Fatalf("GetRoundResults: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].HomePlayerID != homePlayerID {
		t.Errorf("want home_player_id=%d, got %d", homePlayerID, rows[0].HomePlayerID)
	}
	if rows[0].HandicapPtsUsed == nil || *rows[0].HandicapPtsUsed != 3 {
		t.Errorf("want HandicapPtsUsed=3, got %v", rows[0].HandicapPtsUsed)
	}
}

// ─── GetStandingsData week_closed gate ───────────────────────────────────────

func TestRoundStore_GetStandingsData_OnlyClosedMatches(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, seasonID, homeTeamID, awayTeamID := seedRoundTestData(t)

	// Insert match result but do NOT set week_closed=1 — should not appear in standings.
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3)`,
		matchID, homePlayerID, homeTeamID)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,0,3,-3)`,
		matchID, awayPlayerID, awayTeamID)

	data, err := s.GetStandingsData(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("GetStandingsData: %v", err)
	}
	if len(data.Matches) != 0 {
		t.Errorf("want 0 matches (not closed), got %d", len(data.Matches))
	}

	// Now close the week and confirm the match appears.
	db.DB.Exec(`UPDATE matches SET week_closed=1 WHERE id=?`, matchID)
	data, err = s.GetStandingsData(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("GetStandingsData (closed): %v", err)
	}
	if len(data.Matches) != 1 {
		t.Errorf("want 1 match after week_closed, got %d", len(data.Matches))
	}
}

// ─── GetPlayerStats ──────────────────────────────────────────────────────────

func TestRoundStore_GetPlayerStats_SeasonScope_WeekClosedGate(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, _, seasonID, homeTeamID, _ := seedRoundTestData(t)

	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3)`,
		matchID, homePlayerID, homeTeamID)

	// Not closed — sets_won should remain 0.
	stats, err := s.GetPlayerStats(context.Background(), matches.PlayerStatsRequest{SeasonID: seasonID})
	if err != nil {
		t.Fatalf("GetPlayerStats: %v", err)
	}
	for _, st := range stats {
		if st.PlayerID == homePlayerID && st.GamesWon != 0 {
			t.Errorf("want games_won=0 for unclosed week, got %d", st.GamesWon)
		}
	}

	// Close and recheck.
	db.DB.Exec(`UPDATE matches SET week_closed=1 WHERE id=?`, matchID)
	stats, err = s.GetPlayerStats(context.Background(), matches.PlayerStatsRequest{SeasonID: seasonID})
	if err != nil {
		t.Fatalf("GetPlayerStats (closed): %v", err)
	}
	var found bool
	for _, st := range stats {
		if st.PlayerID == homePlayerID {
			found = true
			if st.GamesWon != 3 {
				t.Errorf("want games_won=3 after close, got %d", st.GamesWon)
			}
		}
	}
	if !found {
		t.Error("home player not found in stats")
	}
}

// ─── SubmitMatchResults ───────────────────────────────────────────────────────

func TestRoundStore_SubmitMatchResults_ReplacesAndCompletes(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, _, _, homeTeamID, _ := seedRoundTestData(t)

	results := []models.MatchResult{
		{PlayerID: homePlayerID, TeamID: homeTeamID, GamesWon: 2, GamesLost: 1, Diff: 1},
	}
	if err := s.SubmitMatchResults(context.Background(), matchID, results); err != nil {
		t.Fatalf("SubmitMatchResults: %v", err)
	}
	var count, completed int
	db.DB.QueryRow(`SELECT COUNT(*) FROM match_results WHERE match_id=?`, matchID).Scan(&count)
	db.DB.QueryRow(`SELECT completed FROM matches WHERE id=?`, matchID).Scan(&completed)
	if count != 1 {
		t.Errorf("want 1 match_result, got %d", count)
	}
	if completed != 1 {
		t.Errorf("want completed=1, got %d", completed)
	}
}

// ─── ClearMatchResults ───────────────────────────────────────────────────────

func TestRoundStore_ClearMatchResults_DeletesAndMarksIncomplete(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, _, _, homeTeamID, _ := seedRoundTestData(t)

	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3)`,
		matchID, homePlayerID, homeTeamID)

	if err := s.ClearMatchResults(context.Background(), matchID); err != nil {
		t.Fatalf("ClearMatchResults: %v", err)
	}
	var count, completed int
	db.DB.QueryRow(`SELECT COUNT(*) FROM match_results WHERE match_id=?`, matchID).Scan(&count)
	db.DB.QueryRow(`SELECT completed FROM matches WHERE id=?`, matchID).Scan(&completed)
	if count != 0 {
		t.Errorf("want 0 match_results after clear, got %d", count)
	}
	if completed != 0 {
		t.Errorf("want completed=0 after clear, got %d", completed)
	}
}

// ─── LoadPriorSnapshots ──────────────────────────────────────────────────────

func TestRoundStore_LoadPriorSnapshots_ReturnsSnapshotValues(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, _, _, _ := seedRoundTestData(t)

	db.DB.Exec(`
		INSERT INTO round_results
		  (match_id, round_number, home_player_id, away_player_id,
		   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		   home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,3,10,5,10,2,3.5,4.0,2,'home')`,
		matchID, homePlayerID, awayPlayerID)

	snaps, err := s.LoadPriorSnapshots(context.Background(), matchID)
	if err != nil {
		t.Fatalf("LoadPriorSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("want 1 snapshot, got %d", len(snaps))
	}
	if !snaps[0].HomeHandicapUsed.Valid || snaps[0].HomeHandicapUsed.Float64 != 3.5 {
		t.Errorf("want home_handicap_used=3.5, got %v", snaps[0].HomeHandicapUsed)
	}
}
