package db_test

import (
	"context"
	"fmt"
	"testing"

	"league_app/backend/domains/leagues"
	"league_app/backend/domains/seasons"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// assertForeignKeyCheckClean runs PRAGMA foreign_key_check and fails the
// test if it reports any violation. This is the authoritative SQLite
// integrity check for dangling foreign-key references -- distinct from
// (and a stronger guarantee than) merely confirming specific rows are gone,
// since it inspects every FK-declaring table in the schema.
func assertForeignKeyCheckClean(t *testing.T) {
	t.Helper()
	rows, err := db.DB.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_check: %v", err)
	}
	defer rows.Close()
	var violations []string
	for rows.Next() {
		var table string
		var rowid *int64
		var parent string
		var fkid int64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("scan foreign_key_check row: %v", err)
		}
		violations = append(violations, fmt.Sprintf("table=%s rowid=%v parent=%s fkid=%d", table, rowid, parent, fkid))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign_key_check iteration: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("want no foreign_key_check violations, got %d: %v", len(violations), violations)
	}
}

func rowCount(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

// forceNonInitConnection checks out and holds one *sql.Conn immediately
// after db.Init, forcing every subsequent *sql.DB call in the test
// (fixture setup, the delete under test, and the assertions) onto a
// different, freshly-opened pooled connection instead of reusing the one
// idle connection db.Init's startup pragmas happened to run on.
//
// This is not optional decoration: without it, these tests still pass
// against the pre-fix db.go (verified empirically while writing them,
// by reverting the fix and re-running) -- database/sql has no concurrent
// demand forcing a second connection open, so every call in a
// straight-line sequential test reuses that one already-correctly-
// configured connection, and the regression these tests exist to catch
// never gets exercised. Holding a connection open here guarantees the
// delete under test runs on a connection db.Init never touched.
func forceNonInitConnection(t *testing.T) {
	t.Helper()
	c, err := db.DB.Conn(context.Background())
	if err != nil {
		t.Fatalf("check out a connection to force pool growth: %v", err)
	}
	t.Cleanup(func() { c.Close() })
}

// TestForeignKeyEnforcement_SeasonDelete_CascadesSeasonOwnedRows guards
// Known Gap #19's "seasons -> lineup_plans and other season-owned rows: ON
// DELETE CASCADE" contract. Deletes a season through the real
// SeasonService/SeasonStore path (not a raw DELETE) and confirms every
// season-owned child row is gone, while the parent league and its teams
// (season-independent) survive untouched.
func TestForeignKeyEnforcement_SeasonDelete_CascadesSeasonOwnedRows(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	forceNonInitConnection(t)
	ctx := context.Background()

	var leagueID, teamID, seasonID, playerID int64
	mustExec := func(query string, args ...any) int64 {
		t.Helper()
		res, err := db.DB.Exec(query, args...)
		if err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	leagueID = mustExec(`INSERT INTO leagues (name) VALUES ('FK Season Cascade League')`)
	teamID = mustExec(`INSERT INTO teams (league_id, name) VALUES (?, 'FK Season Team')`, leagueID)
	playerID = mustExec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('FK', 'Player', ?)`, teamID)
	seasonID = mustExec(`INSERT INTO seasons (league_id, name) VALUES (?, 'FK Season Cascade Season')`, leagueID)
	mustExec(`INSERT INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?, 'k', 'Label', 'v')`, seasonID)
	mustExec(`INSERT INTO skipped_weeks (season_id, skip_date) VALUES (?, '2026-01-01')`, seasonID)
	mustExec(`INSERT INTO bye_requests (season_id, team_id) VALUES (?, ?)`, seasonID, teamID)
	matchID := mustExec(`INSERT INTO matches (season_id, home_team_id, week_number) VALUES (?, ?, 1)`, seasonID, teamID)
	mustExec(`INSERT INTO lineup_plans (team_id, player_id, week_number, season_id) VALUES (?, ?, 0, ?)`, teamID, playerID, seasonID)

	svc := seasons.NewSeasonService(sqlite.NewSeasonStore(db.DB))
	if err := svc.DeleteSeason(ctx, seasonID); err != nil {
		t.Fatalf("DeleteSeason: %v", err)
	}

	if n := rowCount(t, `SELECT COUNT(*) FROM seasons WHERE id=?`, seasonID); n != 0 {
		t.Errorf("want season row gone, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM season_rules WHERE season_id=?`, seasonID); n != 0 {
		t.Errorf("want season_rules cascade-deleted, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM skipped_weeks WHERE season_id=?`, seasonID); n != 0 {
		t.Errorf("want skipped_weeks cascade-deleted, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM bye_requests WHERE season_id=?`, seasonID); n != 0 {
		t.Errorf("want bye_requests cascade-deleted, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM matches WHERE id=?`, matchID); n != 0 {
		t.Errorf("want matches cascade-deleted, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM lineup_plans WHERE season_id=?`, seasonID); n != 0 {
		t.Errorf("want lineup_plans cascade-deleted, found %d", n)
	}

	// Season-independent rows must survive a season delete.
	if n := rowCount(t, `SELECT COUNT(*) FROM leagues WHERE id=?`, leagueID); n != 1 {
		t.Errorf("want the parent league to survive a season delete, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM teams WHERE id=?`, teamID); n != 1 {
		t.Errorf("want the league's team to survive a season delete, found %d", n)
	}

	assertForeignKeyCheckClean(t)
}

// TestForeignKeyEnforcement_LeagueDelete_CascadesSeasonsTeamsAndOwnedRows
// guards Known Gap #19's "leagues -> seasons: ON DELETE CASCADE" and
// "leagues -> teams: ON DELETE CASCADE" contracts together, exercised via
// the real LeagueService/LeagueStore delete path. A season under the
// league, that season's lineup_plans, and a match all cascade away
// transitively through the season; the team cascades away directly.
func TestForeignKeyEnforcement_LeagueDelete_CascadesSeasonsTeamsAndOwnedRows(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	forceNonInitConnection(t)
	ctx := context.Background()

	mustExec := func(query string, args ...any) int64 {
		t.Helper()
		res, err := db.DB.Exec(query, args...)
		if err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	leagueID := mustExec(`INSERT INTO leagues (name) VALUES ('FK League Cascade League')`)
	teamID := mustExec(`INSERT INTO teams (league_id, name) VALUES (?, 'FK League Team')`, leagueID)
	seasonID := mustExec(`INSERT INTO seasons (league_id, name) VALUES (?, 'FK League Cascade Season')`, leagueID)
	matchID := mustExec(`INSERT INTO matches (season_id, home_team_id, week_number) VALUES (?, ?, 1)`, seasonID, teamID)

	svc := leagues.NewLeagueService(sqlite.NewLeagueStore(db.DB))
	if err := svc.DeleteLeague(ctx, leagueID); err != nil {
		t.Fatalf("DeleteLeague: %v", err)
	}

	if n := rowCount(t, `SELECT COUNT(*) FROM leagues WHERE id=?`, leagueID); n != 0 {
		t.Errorf("want league row gone, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM seasons WHERE id=?`, seasonID); n != 0 {
		t.Errorf("want season cascade-deleted via league, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM teams WHERE id=?`, teamID); n != 0 {
		t.Errorf("want team cascade-deleted via league, found %d", n)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM matches WHERE id=?`, matchID); n != 0 {
		t.Errorf("want match cascade-deleted via league->season, found %d", n)
	}

	assertForeignKeyCheckClean(t)
}

// TestForeignKeyEnforcement_TeamDelete_PlayersSurviveWithTeamIDCleared
// guards the schema's deliberate exception: "teams -> players.team_id: ON
// DELETE SET NULL", not CASCADE. A player is never deleted when their team
// is -- only detached (team_id becomes NULL). This is the opposite failure
// mode from the other two tests: here, a *survivor* is the correct outcome
// and disappearing would itself be the bug.
func TestForeignKeyEnforcement_TeamDelete_PlayersSurviveWithTeamIDCleared(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	forceNonInitConnection(t)

	mustExec := func(query string, args ...any) int64 {
		t.Helper()
		res, err := db.DB.Exec(query, args...)
		if err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	leagueID := mustExec(`INSERT INTO leagues (name) VALUES ('FK Team Delete League')`)
	teamID := mustExec(`INSERT INTO teams (league_id, name) VALUES (?, 'FK Team Delete Team')`, leagueID)
	playerID := mustExec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('Set', 'Null', ?)`, teamID)

	if _, err := db.DB.Exec(`DELETE FROM teams WHERE id=?`, teamID); err != nil {
		t.Fatalf("delete team: %v", err)
	}

	var gotTeamID *int64
	if err := db.DB.QueryRow(`SELECT team_id FROM players WHERE id=?`, playerID).Scan(&gotTeamID); err != nil {
		t.Fatalf("want player row to survive team deletion, query failed: %v", err)
	}
	if gotTeamID != nil {
		t.Errorf("want player.team_id=NULL after team delete, got %v", *gotTeamID)
	}
	if n := rowCount(t, `SELECT COUNT(*) FROM players WHERE id=?`, playerID); n != 1 {
		t.Errorf("want exactly 1 surviving player row, found %d", n)
	}

	assertForeignKeyCheckClean(t)
}
