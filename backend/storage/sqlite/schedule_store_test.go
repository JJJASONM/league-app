package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// ── setup helper ──────────────────────────────────────────────────────────────

func newScheduleStore(t *testing.T) *sqlite.ScheduleStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewScheduleStore(db.DB)
}

// ── GetScheduleSeasonMeta ─────────────────────────────────────────────────────

func TestScheduleStore_GetScheduleSeasonMeta_Found(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	meta, err := store.GetScheduleSeasonMeta(ctx, sid)
	if err != nil {
		t.Fatalf("GetScheduleSeasonMeta: %v", err)
	}
	if meta.LeagueID != lid {
		t.Errorf("want LeagueID=%d, got %d", lid, meta.LeagueID)
	}
	if !meta.TeamsManaged {
		t.Error("want TeamsManaged=true for managed season")
	}
}

func TestScheduleStore_GetScheduleSeasonMeta_NotFound(t *testing.T) {
	store := newScheduleStore(t)
	_, err := store.GetScheduleSeasonMeta(context.Background(), 9999)
	if !errors.Is(err, matches.ErrSeasonNotFound) {
		t.Errorf("want ErrSeasonNotFound, got %v", err)
	}
}

// ── LoadByeRequests ───────────────────────────────────────────────────────────

func TestScheduleStore_LoadByeRequests_Empty(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	byes, err := store.LoadByeRequests(ctx, sid)
	if err != nil {
		t.Fatalf("LoadByeRequests: %v", err)
	}
	if len(byes) != 0 {
		t.Errorf("want empty map, got %v", byes)
	}
}

func TestScheduleStore_LoadByeRequests_ApprovedOnly(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sseedBye(t, sid, t1, 3, true)  // approved with specific week → included
	sseedBye(t, sid, t2, 4, false) // unapproved → excluded
	sseedBye(t, sid, t1, 0, true)  // week=0 → excluded

	byes, err := store.LoadByeRequests(ctx, sid)
	if err != nil {
		t.Fatalf("LoadByeRequests: %v", err)
	}
	if len(byes) != 1 {
		t.Errorf("want 1 approved specific-week bye, got %d", len(byes))
	}
	if byes[3] != t1 {
		t.Errorf("want week 3 → team %d, got %d", t1, byes[3])
	}
}

// ── LoadTeamIDsFromHistory ────────────────────────────────────────────────────

func TestScheduleStore_LoadTeamIDsFromHistory_Empty(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	ids, err := store.LoadTeamIDsFromHistory(ctx, sid)
	if err != nil {
		t.Fatalf("LoadTeamIDsFromHistory: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("want empty slice, got %v", ids)
	}
}

func TestScheduleStore_LoadTeamIDsFromHistory_DistinctFromMatches(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sseedMatch(t, sid, t1, t2, false)
	sseedMatch(t, sid, t1, t2, false) // duplicate — UNION dedups

	ids, err := store.LoadTeamIDsFromHistory(ctx, sid)
	if err != nil {
		t.Fatalf("LoadTeamIDsFromHistory: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 distinct team ids, got %d", len(ids))
	}
}

// ── LoadTeamIDsForSchedule ────────────────────────────────────────────────────

func TestScheduleStore_LoadTeamIDsForSchedule_ManagedFromSeasonTeams(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sseedSeasonTeam(t, sid, t1, nil)
	sseedSeasonTeam(t, sid, t2, nil)
	// Third team in league but not in season — must be excluded.
	sseedTeam(t, lid, "C")

	ids, err := store.LoadTeamIDsForSchedule(ctx, sid, lid, true)
	if err != nil {
		t.Fatalf("LoadTeamIDsForSchedule: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 season teams, got %d", len(ids))
	}
}

func TestScheduleStore_LoadTeamIDsForSchedule_LegacyFallsBackToLeague(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	// No season_teams rows — falls back to all league teams.
	sseedTeam(t, lid, "A")
	sseedTeam(t, lid, "B")
	sseedTeam(t, lid, "C")

	ids, err := store.LoadTeamIDsForSchedule(ctx, sid, lid, false)
	if err != nil {
		t.Fatalf("LoadTeamIDsForSchedule: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("want 3 league teams (legacy fallback), got %d", len(ids))
	}
}

func TestScheduleStore_LoadTeamIDsForSchedule_LegacyPrefersSeasonTeams(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	sseedTeam(t, lid, "B") // in league but not in season_teams
	sseedSeasonTeam(t, sid, t1, nil)

	ids, err := store.LoadTeamIDsForSchedule(ctx, sid, lid, false)
	if err != nil {
		t.Fatalf("LoadTeamIDsForSchedule: %v", err)
	}
	// season_teams exists, so use it (1 team), not league fallback (2 teams).
	if len(ids) != 1 {
		t.Errorf("want 1 season team (prefers season_teams over league), got %d", len(ids))
	}
}

// ── SaveGeneratedSchedule ─────────────────────────────────────────────────────

func TestScheduleStore_SaveGeneratedSchedule_CreatesMatches(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")

	req := matches.SaveScheduleRequest{
		SeasonID:     sid,
		ScheduleType: "double_rr",
		NumWeeks:     2,
		EndDate:      "2026-09-08",
		Entries: []matches.MatchEntry{
			{HomeTeamID: t1, AwayTeamID: t2, WeekNumber: 1, MatchDate: "2026-09-01"},
			{HomeTeamID: t2, AwayTeamID: t1, WeekNumber: 2, MatchDate: "2026-09-08"},
		},
	}
	if err := store.SaveGeneratedSchedule(ctx, req); err != nil {
		t.Fatalf("SaveGeneratedSchedule: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=?`, sid).Scan(&count)
	if count != 2 {
		t.Errorf("want 2 matches, got %d", count)
	}

	var schedType, endDate string
	db.DB.QueryRow(`SELECT schedule_type, COALESCE(end_date,'') FROM seasons WHERE id=?`, sid).Scan(&schedType, &endDate)
	if schedType != "double_rr" {
		t.Errorf("want schedule_type=double_rr, got %q", schedType)
	}
	if endDate != "2026-09-08" {
		t.Errorf("want end_date=2026-09-08, got %q", endDate)
	}
}

func TestScheduleStore_SaveGeneratedSchedule_PreservesCompletedMatches(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	sseedMatch(t, sid, t1, t2, true) // completed — must be preserved

	req := matches.SaveScheduleRequest{
		SeasonID:     sid,
		ScheduleType: "double_rr",
		NumWeeks:     1,
		Entries: []matches.MatchEntry{
			{HomeTeamID: t1, AwayTeamID: t2, WeekNumber: 1},
		},
	}
	if err := store.SaveGeneratedSchedule(ctx, req); err != nil {
		t.Fatalf("SaveGeneratedSchedule: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=?`, sid).Scan(&count)
	// 1 completed (preserved) + 1 new = 2
	if count != 2 {
		t.Errorf("want 2 matches (1 completed preserved + 1 new), got %d", count)
	}
}

func TestScheduleStore_SaveGeneratedSchedule_BlanketSlots(t *testing.T) {
	store := newScheduleStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	req := matches.SaveScheduleRequest{
		SeasonID:     sid,
		ScheduleType: "blanket",
		NumWeeks:     2,
		Entries: []matches.MatchEntry{
			{HomeTeamID: 0, AwayTeamID: 0, WeekNumber: 1},
			{HomeTeamID: 0, AwayTeamID: 0, WeekNumber: 2},
		},
	}
	if err := store.SaveGeneratedSchedule(ctx, req); err != nil {
		t.Fatalf("SaveGeneratedSchedule blanket: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=? AND home_team_id IS NULL`, sid).Scan(&count)
	if count != 2 {
		t.Errorf("want 2 blanket (null home) matches, got %d", count)
	}
}
