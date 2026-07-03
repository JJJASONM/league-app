package seasons_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"league_app/backend/domains/seasons"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/models"
)

// setupDB initialises a fresh in-disk (temp-dir) SQLite database and returns
// it. The global db.DB is set as a side effect so db.Init's full schema runs.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return db.DB
}

// makeSvc builds a SeasonService backed by a real sqlite store.
func makeSvc(d *sql.DB) *seasons.SeasonService {
	return seasons.NewSeasonService(sqlite.NewSeasonStore(d))
}

// seed helpers ─────────────────────────────────────────────────────────────────

func seedLeague(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	res, err := d.Exec(`INSERT INTO leagues (name, game_format) VALUES ('Test League','8ball')`)
	if err != nil {
		t.Fatalf("insert league: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedTeam(t *testing.T, d *sql.DB, leagueID int64, name string) int64 {
	t.Helper()
	res, err := d.Exec(`INSERT INTO teams (league_id, name) VALUES (?,?)`, leagueID, name)
	if err != nil {
		t.Fatalf("insert team %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedPlayer(t *testing.T, d *sql.DB, teamID int64, first, last string) int64 {
	t.Helper()
	res, err := d.Exec(
		`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES (?,?,?,0)`,
		first, last, teamID)
	if err != nil {
		t.Fatalf("insert player %s %s: %v", first, last, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedSeason(t *testing.T, d *sql.DB, leagueID int64, name, start, end string) int64 {
	t.Helper()
	var endArg any
	if end != "" {
		endArg = end
	}
	res, err := d.Exec(
		`INSERT INTO seasons (league_id, name, start_date, end_date, schedule_type)
		 VALUES (?,?,?,?,'single_rr')`, leagueID, name, start, endArg)
	if err != nil {
		t.Fatalf("insert season %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedManagedSeason inserts a season with teams_managed=1 so the Checklist
// and RosterEligible functions apply full enforcement (no legacy bypass).
func seedManagedSeason(t *testing.T, d *sql.DB, leagueID int64, name, start, end string) int64 {
	t.Helper()
	var endArg any
	if end != "" {
		endArg = end
	}
	res, err := d.Exec(
		`INSERT INTO seasons (league_id, name, start_date, end_date, schedule_type, teams_managed)
		 VALUES (?,?,?,?,'single_rr',1)`, leagueID, name, start, endArg)
	if err != nil {
		t.Fatalf("insert managed season %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func addSeasonTeam(t *testing.T, d *sql.DB, seasonID, teamID int64, capID *int64) int64 {
	t.Helper()
	res, err := d.Exec(
		`INSERT INTO season_teams (season_id, team_id, season_name, captain_id)
		 SELECT ?, id, name, ? FROM teams WHERE id=?`, seasonID, capID, teamID)
	if err != nil {
		t.Fatalf("insert season_team: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func addRosterPlayer(t *testing.T, d *sql.DB, seasonID, teamID, playerID int64) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamID, playerID); err != nil {
		t.Fatalf("insert season_roster: %v", err)
	}
}

func seedMatch(t *testing.T, d *sql.DB, seasonID, homeID, awayID int64) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,1)`,
		seasonID, homeID, awayID); err != nil {
		t.Fatalf("insert match: %v", err)
	}
	// also set end_date so NO_END_DATE doesn't fire
	d.Exec(`UPDATE seasons SET end_date='2026-10-01' WHERE id=?`, seasonID)
}

// ── Checklist tests ───────────────────────────────────────────────────────────

// TestChecklist_LegacySeason_CanActivate verifies that seasons with teams_managed=0
// (the DEFAULT for rows created before Phase One) bypass all enforcement and can
// always activate, regardless of whether they have teams or a schedule.
func TestChecklist_LegacySeason_CanActivate(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	// seedSeason inserts without teams_managed → DEFAULT 0 → legacy mode.
	sid := seedSeason(t, d, lid, "Draft", "2026-09-01", "")

	c, err := makeSvc(d).Checklist(context.Background(), sid)
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}
	if len(c.Blockers) != 0 {
		t.Errorf("want 0 blockers for legacy season, got %v", c.Blockers)
	}
	if !c.CanActivate {
		t.Error("want CanActivate=true for legacy season")
	}
}

// TestChecklist_ManagedSeason_NoTeams_BlocksTooFew verifies that a managed season
// (teams_managed=1) with no teams gets TEAMS_TOO_FEW and cannot activate.
func TestChecklist_ManagedSeason_NoTeams_BlocksTooFew(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")

	c, err := makeSvc(d).Checklist(context.Background(), sid)
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}
	assertCode(t, c.Blockers, "TEAMS_TOO_FEW")
	if c.CanActivate {
		t.Error("want CanActivate=false for managed season with no teams")
	}
}

func TestChecklist_OnlyOneTeam_BlocksTooFew(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	tid := seedTeam(t, d, lid, "Alpha")
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")
	addSeasonTeam(t, d, sid, tid, nil)

	c, _ := makeSvc(d).Checklist(context.Background(), sid)
	assertCode(t, c.Blockers, "TEAMS_TOO_FEW")
	assertCode(t, c.Blockers, "TEAM_NO_CAPTAIN")
	assertCode(t, c.Blockers, "TEAM_NO_PLAYERS")
}

func TestChecklist_TeamNoPlayers_Blocker(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")
	addSeasonTeam(t, d, sid, t1, nil)
	addSeasonTeam(t, d, sid, t2, nil)

	c, _ := makeSvc(d).Checklist(context.Background(), sid)
	assertCode(t, c.Blockers, "TEAM_NO_PLAYERS")
}

func TestChecklist_TeamFewPlayers_Warning(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")
	p1 := seedPlayer(t, d, t1, "Ann", "A")
	p2 := seedPlayer(t, d, t1, "Bob", "B")
	p3 := seedPlayer(t, d, t2, "Cal", "C")
	p4 := seedPlayer(t, d, t2, "Dan", "D")
	p5 := seedPlayer(t, d, t2, "Eve", "E")

	addSeasonTeam(t, d, sid, t1, &p1)
	addRosterPlayer(t, d, sid, t1, p1)
	addRosterPlayer(t, d, sid, t1, p2) // 2 players — warning

	addSeasonTeam(t, d, sid, t2, &p3)
	addRosterPlayer(t, d, sid, t2, p3)
	addRosterPlayer(t, d, sid, t2, p4)
	addRosterPlayer(t, d, sid, t2, p5)

	seedMatch(t, d, sid, t1, t2)

	c, _ := makeSvc(d).Checklist(context.Background(), sid)
	assertCode(t, c.Warnings, "TEAM_FEW_PLAYERS")
	// TEAM_NO_PLAYERS should NOT appear (team has players)
	assertNotCode(t, c.Blockers, "TEAM_NO_PLAYERS")
}

func TestChecklist_CaptainNotOnRoster_Blocker(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")

	p1 := seedPlayer(t, d, t1, "Ann", "A")
	p2 := seedPlayer(t, d, t1, "Bob", "B")
	p3 := seedPlayer(t, d, t1, "Cal", "C")
	p4 := seedPlayer(t, d, t2, "Dan", "D")
	p5 := seedPlayer(t, d, t2, "Eve", "E")
	p6 := seedPlayer(t, d, t2, "Fay", "F")
	outsider := seedPlayer(t, d, t1, "Ghost", "G") // not on roster

	addSeasonTeam(t, d, sid, t1, &outsider) // captain not on roster
	addRosterPlayer(t, d, sid, t1, p1)
	addRosterPlayer(t, d, sid, t1, p2)
	addRosterPlayer(t, d, sid, t1, p3)

	addSeasonTeam(t, d, sid, t2, &p4)
	addRosterPlayer(t, d, sid, t2, p4)
	addRosterPlayer(t, d, sid, t2, p5)
	addRosterPlayer(t, d, sid, t2, p6)

	seedMatch(t, d, sid, t1, t2)

	c, _ := makeSvc(d).Checklist(context.Background(), sid)
	assertCode(t, c.Blockers, "CAPTAIN_NOT_ON_ROSTER")
}

func TestChecklist_NoSchedule_Blocker(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")
	p1 := seedPlayer(t, d, t1, "Ann", "A")
	p2 := seedPlayer(t, d, t1, "Bob", "B")
	p3 := seedPlayer(t, d, t1, "Cal", "C")
	p4 := seedPlayer(t, d, t2, "Dan", "D")
	p5 := seedPlayer(t, d, t2, "Eve", "E")
	p6 := seedPlayer(t, d, t2, "Fay", "F")

	addSeasonTeam(t, d, sid, t1, &p1)
	addRosterPlayer(t, d, sid, t1, p1)
	addRosterPlayer(t, d, sid, t1, p2)
	addRosterPlayer(t, d, sid, t1, p3)
	addSeasonTeam(t, d, sid, t2, &p4)
	addRosterPlayer(t, d, sid, t2, p4)
	addRosterPlayer(t, d, sid, t2, p5)
	addRosterPlayer(t, d, sid, t2, p6)
	// no match inserted → NO_SCHEDULE blocker

	c, _ := makeSvc(d).Checklist(context.Background(), sid)
	assertCode(t, c.Blockers, "NO_SCHEDULE")
	if c.CanActivate {
		t.Error("want CanActivate=false when no schedule")
	}
}

func TestChecklist_StaleSchedule_Blocker(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")
	p1 := seedPlayer(t, d, t1, "Ann", "A")
	p2 := seedPlayer(t, d, t1, "Bob", "B")
	p3 := seedPlayer(t, d, t1, "Cal", "C")
	p4 := seedPlayer(t, d, t2, "Dan", "D")
	p5 := seedPlayer(t, d, t2, "Eve", "E")
	p6 := seedPlayer(t, d, t2, "Fay", "F")

	addSeasonTeam(t, d, sid, t1, &p1)
	addRosterPlayer(t, d, sid, t1, p1)
	addRosterPlayer(t, d, sid, t1, p2)
	addRosterPlayer(t, d, sid, t1, p3)
	addSeasonTeam(t, d, sid, t2, &p4)
	addRosterPlayer(t, d, sid, t2, p4)
	addRosterPlayer(t, d, sid, t2, p5)
	addRosterPlayer(t, d, sid, t2, p6)
	seedMatch(t, d, sid, t1, t2)

	d.Exec(`UPDATE seasons SET schedule_stale=1 WHERE id=?`, sid)

	c, _ := makeSvc(d).Checklist(context.Background(), sid)
	assertCode(t, c.Blockers, "SCHEDULE_STALE")
}

func TestChecklist_AllGood_CanActivate(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Draft", "2026-09-01", "")
	p1 := seedPlayer(t, d, t1, "Ann", "A")
	p2 := seedPlayer(t, d, t1, "Bob", "B")
	p3 := seedPlayer(t, d, t1, "Cal", "C")
	p4 := seedPlayer(t, d, t2, "Dan", "D")
	p5 := seedPlayer(t, d, t2, "Eve", "E")
	p6 := seedPlayer(t, d, t2, "Fay", "F")

	addSeasonTeam(t, d, sid, t1, &p1)
	addRosterPlayer(t, d, sid, t1, p1)
	addRosterPlayer(t, d, sid, t1, p2)
	addRosterPlayer(t, d, sid, t1, p3)
	addSeasonTeam(t, d, sid, t2, &p4)
	addRosterPlayer(t, d, sid, t2, p4)
	addRosterPlayer(t, d, sid, t2, p5)
	addRosterPlayer(t, d, sid, t2, p6)
	seedMatch(t, d, sid, t1, t2)

	c, err := makeSvc(d).Checklist(context.Background(), sid)
	if err != nil {
		t.Fatalf("Checklist: %v", err)
	}
	if !c.CanActivate {
		t.Errorf("want CanActivate=true, got blockers=%v warnings=%v", c.Blockers, c.Warnings)
	}
}

// ── PreviousSeason tests ──────────────────────────────────────────────────────

func TestPreviousSeason_ByStartDate(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	// Previous (ended before draft start)
	prevID := seedSeason(t, d, lid, "Fall 2025", "2025-09-01", "2025-12-15")
	// Draft (start_date stored in DB; service reads it via GetMeta)
	draftID := seedSeason(t, d, lid, "Spring 2026", "2026-02-01", "")

	result, err := makeSvc(d).PreviousSeason(context.Background(), draftID)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if result.Season == nil {
		t.Fatal("want previous season, got nil")
	}
	if result.Season.ID != prevID {
		t.Errorf("want prevID=%d, got %d", prevID, result.Season.ID)
	}
}

func TestPreviousSeason_NoStartDate_MostRecent(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	seedSeason(t, d, lid, "Old", "2024-01-01", "2024-06-01")
	recentID := seedSeason(t, d, lid, "Recent", "2025-09-01", "2025-12-15")
	draftID := seedSeason(t, d, lid, "Draft", "", "")

	result, err := makeSvc(d).PreviousSeason(context.Background(), draftID)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if result.Season == nil {
		t.Fatal("want previous season")
	}
	if result.Season.ID != recentID {
		t.Errorf("want recentID=%d, got %d", recentID, result.Season.ID)
	}
}

func TestPreviousSeason_NoneExists_ReturnsNil(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	draftID := seedSeason(t, d, lid, "Draft", "2026-09-01", "")

	result, err := makeSvc(d).PreviousSeason(context.Background(), draftID)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if result.Season != nil {
		t.Errorf("want nil season, got %+v", result.Season)
	}
}

// TestPreviousSeason_PrefersActiveSeasonWithNoEndDate verifies that an active
// season with no end_date is returned before a completed season.
func TestPreviousSeason_PrefersActiveSeasonWithNoEndDate(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	// An older completed season.
	seedSeason(t, d, lid, "Old", "2025-01-01", "2025-06-01")
	// Active season without end_date (currently playing — not yet closed).
	activeID := seedSeason(t, d, lid, "Active", "2025-09-01", "")
	d.Exec(`UPDATE seasons SET active=1 WHERE id=?`, activeID)
	// The draft season being set up.
	draftID := seedSeason(t, d, lid, "Draft", "2026-09-01", "")

	result, err := makeSvc(d).PreviousSeason(context.Background(), draftID)
	if err != nil {
		t.Fatalf("PreviousSeason: %v", err)
	}
	if result.Season == nil {
		t.Fatal("want a previous season, got nil")
	}
	if result.Season.ID != activeID {
		t.Errorf("want active season (id=%d), got id=%d", activeID, result.Season.ID)
	}
}

func TestPreviousSeason_NotFromOtherLeague(t *testing.T) {
	d := setupDB(t)
	lid1 := seedLeague(t, d)
	// Second league needs a distinct name to avoid the leagues.name UNIQUE constraint.
	res, err := d.Exec(`INSERT INTO leagues (name, game_format) VALUES ('Other League','8ball')`)
	if err != nil {
		t.Fatalf("insert other league: %v", err)
	}
	var lid2 int64
	lid2, _ = res.LastInsertId()
	seedSeason(t, d, lid2, "Other League Season", "2025-01-01", "2025-06-01")
	draftID := seedSeason(t, d, lid1, "Draft", "2026-09-01", "")

	result, _ := makeSvc(d).PreviousSeason(context.Background(), draftID)
	if result.Season != nil {
		t.Error("should not return seasons from a different league")
	}
}

// ── RosterEligible tests ──────────────────────────────────────────────────────

func TestRosterEligible_LegacySeason_AlwaysTrue(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedSeason(t, d, lid, "Legacy", "2026-01-01", "")
	var matchID int64
	d.QueryRow(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		 VALUES (?,?,?,1) RETURNING id`, sid, t1, t2).Scan(&matchID)

	// No season_teams → skip check
	ok, msg := seasons.RosterEligible(d, matchID, 3)
	if !ok {
		t.Errorf("want eligible for legacy season, got msg=%q", msg)
	}
}

func TestRosterEligible_InsufficientRoster_ReturnsFalse(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Current", "2026-09-01", "")
	p1 := seedPlayer(t, d, t1, "Ann", "A")
	p2 := seedPlayer(t, d, t2, "Bob", "B")
	p3 := seedPlayer(t, d, t2, "Cal", "C")
	p4 := seedPlayer(t, d, t2, "Dan", "D")

	addSeasonTeam(t, d, sid, t1, &p1)
	addRosterPlayer(t, d, sid, t1, p1) // only 1 player on home team

	addSeasonTeam(t, d, sid, t2, &p2)
	addRosterPlayer(t, d, sid, t2, p2)
	addRosterPlayer(t, d, sid, t2, p3)
	addRosterPlayer(t, d, sid, t2, p4)

	var matchID int64
	d.QueryRow(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		 VALUES (?,?,?,1) RETURNING id`, sid, t1, t2).Scan(&matchID)

	ok, msg := seasons.RosterEligible(d, matchID, 3)
	if ok {
		t.Error("want ineligible when home team has < 3 players")
	}
	if msg == "" {
		t.Error("want non-empty error message")
	}
}

func TestRosterEligible_SufficientRoster_ReturnsTrue(t *testing.T) {
	d := setupDB(t)
	lid := seedLeague(t, d)
	t1 := seedTeam(t, d, lid, "A")
	t2 := seedTeam(t, d, lid, "B")
	sid := seedManagedSeason(t, d, lid, "Current", "2026-09-01", "")
	players := make([]int64, 6)
	for i := 0; i < 3; i++ {
		players[i] = seedPlayer(t, d, t1, fmt.Sprintf("H%d", i), "X")
		players[i+3] = seedPlayer(t, d, t2, fmt.Sprintf("A%d", i), "Y")
	}

	addSeasonTeam(t, d, sid, t1, &players[0])
	addSeasonTeam(t, d, sid, t2, &players[3])
	for i, pid := range players {
		tid := t1
		if i >= 3 {
			tid = t2
		}
		addRosterPlayer(t, d, sid, tid, pid)
	}

	var matchID int64
	d.QueryRow(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		 VALUES (?,?,?,1) RETURNING id`, sid, t1, t2).Scan(&matchID)

	ok, _ := seasons.RosterEligible(d, matchID, 3)
	if !ok {
		t.Error("want eligible when both teams have >= 3 players")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertCode(t *testing.T, items []models.ChecklistItem, code string) {
	t.Helper()
	for _, it := range items {
		if it.Code == code {
			return
		}
	}
	t.Errorf("expected checklist code %q, got %v", code, items)
}

func assertNotCode(t *testing.T, items []models.ChecklistItem, code string) {
	t.Helper()
	for _, it := range items {
		if it.Code == code {
			t.Errorf("unexpected checklist code %q in %v", code, items)
			return
		}
	}
}
