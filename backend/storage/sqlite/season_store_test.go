package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/seasons"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// ── setup helpers ─────────────────────────────────────────────────────────────

func newSeasonStore(t *testing.T) *sqlite.SeasonStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewSeasonStore(db.DB)
}

func sseedLeague(t *testing.T) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO leagues (name, game_format) VALUES ('L','8ball')`)
	if err != nil {
		t.Fatalf("insert league: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func sseedSeason(t *testing.T, leagueID int64, name, start, end string, managed bool) int64 {
	t.Helper()
	var endArg any
	if end != "" {
		endArg = end
	}
	managed01 := 0
	if managed {
		managed01 = 1
	}
	res, err := db.DB.Exec(
		`INSERT INTO seasons (league_id, name, start_date, end_date, schedule_type, teams_managed)
		 VALUES (?,?,?,?,'single_rr',?)`, leagueID, name, start, endArg, managed01)
	if err != nil {
		t.Fatalf("insert season %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func sseedTeam(t *testing.T, leagueID int64, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,?)`, leagueID, name)
	if err != nil {
		t.Fatalf("insert team %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func sseedPlayer(t *testing.T, teamID int64) int64 {
	t.Helper()
	res, err := db.DB.Exec(
		`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('F','L',?,0)`, teamID)
	if err != nil {
		t.Fatalf("insert player: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func sseedSeasonTeam(t *testing.T, seasonID, teamID int64, capID *int64) {
	t.Helper()
	_, err := db.DB.Exec(
		`INSERT INTO season_teams (season_id, team_id, season_name, captain_id)
		 SELECT ?, id, name, ? FROM teams WHERE id=?`, seasonID, capID, teamID)
	if err != nil {
		t.Fatalf("insert season_team: %v", err)
	}
}

func sseedRoster(t *testing.T, seasonID, teamID, playerID int64) {
	t.Helper()
	_, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamID, playerID)
	if err != nil {
		t.Fatalf("insert season_roster: %v", err)
	}
}

func sseedMatch(t *testing.T, seasonID, homeID, awayID int64, completed bool) {
	t.Helper()
	c := 0
	if completed {
		c = 1
	}
	_, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed)
		 VALUES (?,?,?,1,?)`, seasonID, homeID, awayID, c)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
}

// ── IsDraft ───────────────────────────────────────────────────────────────────

func TestSeasonStore_IsDraft_TrueWhenNeverActivated(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "2026-09-01", "", false)

	draft, err := store.IsDraft(ctx, sid)
	if err != nil {
		t.Fatalf("IsDraft: %v", err)
	}
	if !draft {
		t.Error("want true before activation")
	}
}

func TestSeasonStore_IsDraft_FalseAfterActivatedAtSet(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "2026-09-01", "", false)
	db.DB.Exec(`UPDATE seasons SET activated_at=CURRENT_TIMESTAMP WHERE id=?`, sid)

	draft, err := store.IsDraft(ctx, sid)
	if err != nil {
		t.Fatalf("IsDraft: %v", err)
	}
	if draft {
		t.Error("want false after activated_at is set")
	}
}

func TestSeasonStore_IsDraft_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	_, err := store.IsDraft(context.Background(), 999)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── GetMeta ───────────────────────────────────────────────────────────────────

func TestSeasonStore_GetMeta_ReturnsFields(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "2026-09-01", "2027-05-01", true)
	db.DB.Exec(`UPDATE seasons SET schedule_stale=1 WHERE id=?`, sid)

	meta, err := store.GetMeta(ctx, sid)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if meta.LeagueID != lid {
		t.Errorf("want LeagueID=%d, got %d", lid, meta.LeagueID)
	}
	if !meta.TeamsManaged {
		t.Error("want TeamsManaged=true")
	}
	if !meta.ScheduleStale {
		t.Error("want ScheduleStale=true")
	}
	if meta.StartDate == nil || *meta.StartDate != "2026-09-01" {
		t.Errorf("want StartDate=2026-09-01, got %v", meta.StartDate)
	}
	if meta.EndDate == nil || *meta.EndDate != "2027-05-01" {
		t.Errorf("want EndDate=2027-05-01, got %v", meta.EndDate)
	}
}

func TestSeasonStore_GetMeta_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	_, err := store.GetMeta(context.Background(), 999)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── GetTeamSummaries ──────────────────────────────────────────────────────────

func TestSeasonStore_GetTeamSummaries_EmptyWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	ts, err := store.GetTeamSummaries(ctx, sid)
	if err != nil {
		t.Fatalf("GetTeamSummaries: %v", err)
	}
	if len(ts) != 0 {
		t.Errorf("want empty, got %v", ts)
	}
}

func TestSeasonStore_GetTeamSummaries_CaptainOnRoster(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Alpha")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, &pid)
	sseedRoster(t, sid, tid, pid)

	ts, err := store.GetTeamSummaries(ctx, sid)
	if err != nil {
		t.Fatalf("GetTeamSummaries: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("want 1 team, got %d", len(ts))
	}
	if ts[0].RosterCount != 1 {
		t.Errorf("want RosterCount=1, got %d", ts[0].RosterCount)
	}
	if ts[0].CaptainID == nil || *ts[0].CaptainID != pid {
		t.Errorf("want CaptainID=%d, got %v", pid, ts[0].CaptainID)
	}
	if !ts[0].CaptainOnRoster {
		t.Error("want CaptainOnRoster=true")
	}
}

func TestSeasonStore_GetTeamSummaries_CaptainNotOnRoster(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Alpha")
	pid1 := sseedPlayer(t, tid) // on roster
	pid2 := sseedPlayer(t, tid) // captain but NOT on roster
	sseedSeasonTeam(t, sid, tid, &pid2)
	sseedRoster(t, sid, tid, pid1)

	ts, err := store.GetTeamSummaries(ctx, sid)
	if err != nil {
		t.Fatalf("GetTeamSummaries: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("want 1 team, got %d", len(ts))
	}
	if ts[0].CaptainOnRoster {
		t.Error("want CaptainOnRoster=false when captain absent from roster")
	}
}

// ── GetMatchCount ─────────────────────────────────────────────────────────────

func TestSeasonStore_GetMatchCount_ZeroWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	n, err := store.GetMatchCount(ctx, sid)
	if err != nil {
		t.Fatalf("GetMatchCount: %v", err)
	}
	if n != 0 {
		t.Errorf("want 0, got %d", n)
	}
}

func TestSeasonStore_GetMatchCount_ReturnsCount(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sseedMatch(t, sid, t1, t2, false)
	sseedMatch(t, sid, t2, t1, true)

	n, err := store.GetMatchCount(ctx, sid)
	if err != nil {
		t.Fatalf("GetMatchCount: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2, got %d", n)
	}
}

// ── Activate ──────────────────────────────────────────────────────────────────

func TestSeasonStore_Activate_SetsActiveAndActivatedAt(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	if err := store.Activate(ctx, sid, lid); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	var active int
	var activatedAt *string
	db.DB.QueryRow(`SELECT active, activated_at FROM seasons WHERE id=?`, sid).
		Scan(&active, &activatedAt)
	if active != 1 {
		t.Error("want active=1")
	}
	if activatedAt == nil {
		t.Error("want activated_at to be set")
	}
}

func TestSeasonStore_Activate_ActivatedAtNotReset(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	// First activation sets activated_at.
	if err := store.Activate(ctx, sid, lid); err != nil {
		t.Fatalf("first Activate: %v", err)
	}
	var first *string
	db.DB.QueryRow(`SELECT activated_at FROM seasons WHERE id=?`, sid).Scan(&first)

	// Deactivate and reactivate — activated_at must not change.
	db.DB.Exec(`UPDATE seasons SET active=0 WHERE id=?`, sid)
	if err := store.Activate(ctx, sid, lid); err != nil {
		t.Fatalf("second Activate: %v", err)
	}
	var second *string
	db.DB.QueryRow(`SELECT activated_at FROM seasons WHERE id=?`, sid).Scan(&second)

	if first == nil || second == nil || *first != *second {
		t.Errorf("activated_at changed between activations: %v → %v", first, second)
	}
}

func TestSeasonStore_Activate_DeactivatesOthersInLeague(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid1 := sseedSeason(t, lid, "Old", "", "", false)
	db.DB.Exec(`UPDATE seasons SET active=1 WHERE id=?`, sid1)
	sid2 := sseedSeason(t, lid, "New", "", "", false)

	if err := store.Activate(ctx, sid2, lid); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	var a1, a2 int
	db.DB.QueryRow(`SELECT active FROM seasons WHERE id=?`, sid1).Scan(&a1)
	db.DB.QueryRow(`SELECT active FROM seasons WHERE id=?`, sid2).Scan(&a2)
	if a1 != 0 {
		t.Error("want old season deactivated")
	}
	if a2 != 1 {
		t.Error("want new season active")
	}
}

// ── MarkStaleIfScheduled ──────────────────────────────────────────────────────

func TestSeasonStore_MarkStaleIfScheduled_SetsFlag(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sseedMatch(t, sid, t1, t2, false) // unplayed

	if err := store.MarkStaleIfScheduled(ctx, sid); err != nil {
		t.Fatalf("MarkStaleIfScheduled: %v", err)
	}

	var stale int
	db.DB.QueryRow(`SELECT COALESCE(schedule_stale,0) FROM seasons WHERE id=?`, sid).Scan(&stale)
	if stale != 1 {
		t.Error("want schedule_stale=1")
	}
}

func TestSeasonStore_MarkStaleIfScheduled_NoOpWhenNoUnplayed(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sseedMatch(t, sid, t1, t2, true) // completed

	if err := store.MarkStaleIfScheduled(ctx, sid); err != nil {
		t.Fatalf("MarkStaleIfScheduled: %v", err)
	}

	var stale int
	db.DB.QueryRow(`SELECT COALESCE(schedule_stale,0) FROM seasons WHERE id=?`, sid).Scan(&stale)
	if stale != 0 {
		t.Error("want schedule_stale=0 when all matches completed")
	}
}

// ── FindActiveWithNoEndDate ───────────────────────────────────────────────────

func TestSeasonStore_FindActiveWithNoEndDate_ReturnsNilWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	got, err := store.FindActiveWithNoEndDate(ctx, lid, sid)
	if err != nil {
		t.Fatalf("FindActiveWithNoEndDate: %v", err)
	}
	if got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}

func TestSeasonStore_FindActiveWithNoEndDate_FindsSeason(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	activeID := sseedSeason(t, lid, "Active", "2025-09-01", "", false)
	db.DB.Exec(`UPDATE seasons SET active=1 WHERE id=?`, activeID)
	draftID := sseedSeason(t, lid, "Draft", "2026-09-01", "", false)

	got, err := store.FindActiveWithNoEndDate(ctx, lid, draftID)
	if err != nil {
		t.Fatalf("FindActiveWithNoEndDate: %v", err)
	}
	if got == nil {
		t.Fatal("want season, got nil")
	}
	if got.ID != activeID {
		t.Errorf("want id=%d, got %d", activeID, got.ID)
	}
}

// ── FindClosestPriorByEndDate ─────────────────────────────────────────────────

func TestSeasonStore_FindClosestPriorByEndDate_PicksClosest(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sseedSeason(t, lid, "Old", "2024-01-01", "2024-06-01", false)
	recentID := sseedSeason(t, lid, "Recent", "2025-09-01", "2025-12-15", false)
	draftID := sseedSeason(t, lid, "Draft", "2026-09-01", "", false)

	start := "2026-09-01"
	got, err := store.FindClosestPriorByEndDate(ctx, lid, draftID, &start)
	if err != nil {
		t.Fatalf("FindClosestPriorByEndDate: %v", err)
	}
	if got == nil {
		t.Fatal("want season, got nil")
	}
	if got.ID != recentID {
		t.Errorf("want id=%d, got %d", recentID, got.ID)
	}
}

func TestSeasonStore_FindClosestPriorByEndDate_ReturnsNilWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	draftID := sseedSeason(t, lid, "Draft", "2026-09-01", "", false)

	got, err := store.FindClosestPriorByEndDate(ctx, lid, draftID, nil)
	if err != nil {
		t.Fatalf("FindClosestPriorByEndDate: %v", err)
	}
	if got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}

// ── GetSeasonTeams ────────────────────────────────────────────────────────────

func TestSeasonStore_GetSeasonTeams_ReturnsList(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	tid := sseedTeam(t, lid, "Alpha")
	sseedSeasonTeam(t, sid, tid, nil)

	teams, err := store.GetSeasonTeams(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeasonTeams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("want 1 team, got %d", len(teams))
	}
	if teams[0].TeamID != tid {
		t.Errorf("want team id=%d, got %d", tid, teams[0].TeamID)
	}
}

func TestSeasonStore_GetSeasonTeams_EmptyWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	teams, err := store.GetSeasonTeams(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeasonTeams: %v", err)
	}
	if len(teams) != 0 {
		t.Errorf("want empty, got %v", teams)
	}
}

// ── GetMatchTeams ─────────────────────────────────────────────────────────────

func TestSeasonStore_GetMatchTeams_ReturnsList(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "Alpha")
	t2 := sseedTeam(t, lid, "Beta")
	sseedMatch(t, sid, t1, t2, false)

	teams, err := store.GetMatchTeams(ctx, sid)
	if err != nil {
		t.Fatalf("GetMatchTeams: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("want 2 teams, got %d", len(teams))
	}
}

// ── FindActiveSeasonByLeague ──────────────────────────────────────────────────

func TestSeasonStore_FindActiveSeasonByLeague_ReturnsIDWhenActive(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "2026-09-01", "", false)
	db.DB.Exec(`UPDATE seasons SET active=1 WHERE id=?`, sid)

	got, found, err := store.FindActiveSeasonByLeague(ctx, lid)
	if err != nil {
		t.Fatalf("FindActiveSeasonByLeague: %v", err)
	}
	if !found {
		t.Fatal("want found=true for active season")
	}
	if got != sid {
		t.Errorf("want id=%d, got %d", sid, got)
	}
}

func TestSeasonStore_FindActiveSeasonByLeague_NotFoundWhenInactive(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sseedSeason(t, lid, "S", "2026-09-01", "", false) // active=0 by default

	_, found, err := store.FindActiveSeasonByLeague(ctx, lid)
	if err != nil {
		t.Fatalf("FindActiveSeasonByLeague: %v", err)
	}
	if found {
		t.Error("want found=false when no active season")
	}
}

func TestSeasonStore_FindActiveSeasonByLeague_NotFoundWhenNoSeasons(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)

	_, found, err := store.FindActiveSeasonByLeague(ctx, lid)
	if err != nil {
		t.Fatalf("FindActiveSeasonByLeague: %v", err)
	}
	if found {
		t.Error("want found=false when league has no seasons")
	}
}

// ── RosterEligible ────────────────────────────────────────────────────────────

func TestSeasonStore_RosterEligible_LegacySeason_AlwaysTrue(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sid := sseedSeason(t, lid, "Legacy", "", "", false) // teams_managed=0
	var matchID int64
	db.DB.QueryRow(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		 VALUES (?,?,?,1) RETURNING id`, sid, t1, t2).Scan(&matchID)

	ok, msg, err := store.RosterEligible(ctx, matchID, 3)
	if err != nil {
		t.Fatalf("RosterEligible: %v", err)
	}
	if !ok {
		t.Errorf("want eligible for legacy season, got msg=%q", msg)
	}
}

func TestSeasonStore_RosterEligible_NotFound_ReturnsTrue(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	// Match 999 does not exist; should return true so other validation catches it.
	ok, msg, err := store.RosterEligible(ctx, 999, 3)
	if err != nil {
		t.Fatalf("RosterEligible: %v", err)
	}
	if !ok {
		t.Errorf("want true (skip) for missing match, got msg=%q", msg)
	}
}

func TestSeasonStore_RosterEligible_InsufficientRoster_ReturnsFalse(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sid := sseedSeason(t, lid, "Current", "", "", true) // teams_managed=1
	p1 := sseedPlayer(t, t1)
	p2 := sseedPlayer(t, t2)
	sseedSeasonTeam(t, sid, t1, &p1)
	sseedSeasonTeam(t, sid, t2, &p2)
	sseedRoster(t, sid, t1, p1) // only 1 player on home (< 3 required)
	sseedRoster(t, sid, t2, p2)
	sseedRoster(t, sid, t2, sseedPlayer(t, t2))
	sseedRoster(t, sid, t2, sseedPlayer(t, t2))

	var matchID int64
	db.DB.QueryRow(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		 VALUES (?,?,?,1) RETURNING id`, sid, t1, t2).Scan(&matchID)

	ok, msg, err := store.RosterEligible(ctx, matchID, 3)
	if err != nil {
		t.Fatalf("RosterEligible: %v", err)
	}
	if ok {
		t.Error("want ineligible when home team has < 3 players")
	}
	if msg == "" {
		t.Error("want non-empty message")
	}
}

func TestSeasonStore_RosterEligible_SufficientRoster_ReturnsTrue(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sid := sseedSeason(t, lid, "Current", "", "", true) // teams_managed=1
	p1 := sseedPlayer(t, t1)
	sseedSeasonTeam(t, sid, t1, &p1)
	sseedRoster(t, sid, t1, p1)
	sseedRoster(t, sid, t1, sseedPlayer(t, t1))
	sseedRoster(t, sid, t1, sseedPlayer(t, t1))
	p2 := sseedPlayer(t, t2)
	sseedSeasonTeam(t, sid, t2, &p2)
	sseedRoster(t, sid, t2, p2)
	sseedRoster(t, sid, t2, sseedPlayer(t, t2))
	sseedRoster(t, sid, t2, sseedPlayer(t, t2))

	var matchID int64
	db.DB.QueryRow(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		 VALUES (?,?,?,1) RETURNING id`, sid, t1, t2).Scan(&matchID)

	ok, _, err := store.RosterEligible(ctx, matchID, 3)
	if err != nil {
		t.Fatalf("RosterEligible: %v", err)
	}
	if !ok {
		t.Error("want eligible when both teams have >= 3 players")
	}
}
