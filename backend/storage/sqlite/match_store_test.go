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

func newMatchStore(t *testing.T) *sqlite.MatchStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewMatchStore(db.DB)
}

// msseedMatchID inserts a match and returns its ID.
func msseedMatchID(t *testing.T, seasonID, homeID, awayID int64, weekNum int) int64 {
	t.Helper()
	res, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed)
		 VALUES (?,?,?,?,0)`, seasonID, homeID, awayID, weekNum)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// msseedResult inserts a match_results row for a player.
func msseedResult(t *testing.T, matchID, playerID, teamID int64) {
	t.Helper()
	_, err := db.DB.Exec(
		`INSERT INTO match_results (match_id, player_id, team_id, sets_won, sets_lost, games_won, games_lost, diff)
		 VALUES (?,?,?,1,0,3,0,1.0)`, matchID, playerID, teamID)
	if err != nil {
		t.Fatalf("insert match_result: %v", err)
	}
}

// ── ListMatches ───────────────────────────────────────────────────────────────

func TestMatchStore_ListMatches_BySeasonID(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	msseedMatchID(t, sid, tid, tid, 1)
	msseedMatchID(t, sid, tid, tid, 2)

	ms, err := store.ListMatches(ctx, matches.ListMatchesRequest{SeasonID: sid})
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if len(ms) != 2 {
		t.Errorf("want 2 matches, got %d", len(ms))
	}
	for _, m := range ms {
		if m.SeasonID != sid {
			t.Errorf("want season_id=%d, got %d", sid, m.SeasonID)
		}
	}
}

func TestMatchStore_ListMatches_ByLeagueID(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	msseedMatchID(t, sid, tid, tid, 1)

	// Another league (different name to avoid UNIQUE constraint) — should not appear.
	res2, _ := db.DB.Exec(`INSERT INTO leagues (name, game_format) VALUES ('L2','8ball')`)
	lid2, _ := res2.LastInsertId()
	sid2 := sseedSeason(t, lid2, "S2", "", "", true)
	tid2 := sseedTeam(t, lid2, "T2")
	msseedMatchID(t, sid2, tid2, tid2, 1)

	ms, err := store.ListMatches(ctx, matches.ListMatchesRequest{LeagueID: lid})
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if len(ms) != 1 {
		t.Errorf("want 1 match for league %d, got %d", lid, len(ms))
	}
}

func TestMatchStore_ListMatches_NoFilter(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	msseedMatchID(t, sid, tid, tid, 1)
	msseedMatchID(t, sid, tid, tid, 2)

	ms, err := store.ListMatches(ctx, matches.ListMatchesRequest{})
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if len(ms) < 2 {
		t.Errorf("want at least 2 matches, got %d", len(ms))
	}
}

func TestMatchStore_ListMatches_BlankSlot(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	// Insert a blanket slot with null team IDs.
	_, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed)
		 VALUES (?,NULL,NULL,1,0)`, sid)
	if err != nil {
		t.Fatalf("insert blank match: %v", err)
	}

	ms, err := store.ListMatches(ctx, matches.ListMatchesRequest{SeasonID: sid})
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if len(ms) != 1 {
		t.Fatalf("want 1 match, got %d", len(ms))
	}
	if ms[0].HomeTeamID != 0 {
		t.Errorf("want home_team_id=0 for blank slot, got %d", ms[0].HomeTeamID)
	}
	if ms[0].HomeTeamName != "(unassigned)" {
		t.Errorf("want home_team_name='(unassigned)', got %q", ms[0].HomeTeamName)
	}
}

func TestMatchStore_ListMatches_OrderedByWeekThenID(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	// Insert week 2 before week 1 to verify ordering.
	msseedMatchID(t, sid, tid, tid, 2)
	msseedMatchID(t, sid, tid, tid, 1)

	ms, err := store.ListMatches(ctx, matches.ListMatchesRequest{SeasonID: sid})
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("want 2 matches, got %d", len(ms))
	}
	if ms[0].WeekNumber != 1 {
		t.Errorf("want first match week=1, got %d", ms[0].WeekNumber)
	}
}

// ── GetMatch ──────────────────────────────────────────────────────────────────

func TestMatchStore_GetMatch_Found(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	mid := msseedMatchID(t, sid, tid, tid, 1)
	pid := sseedPlayer(t, tid)
	msseedResult(t, mid, pid, tid)

	detail, err := store.GetMatch(ctx, mid)
	if err != nil {
		t.Fatalf("GetMatch: %v", err)
	}
	if detail.Match.ID != mid {
		t.Errorf("want match id=%d, got %d", mid, detail.Match.ID)
	}
	if len(detail.Results) != 1 {
		t.Errorf("want 1 result, got %d", len(detail.Results))
	}
	if detail.Results[0].PlayerID != pid {
		t.Errorf("want player_id=%d, got %d", pid, detail.Results[0].PlayerID)
	}
}

func TestMatchStore_GetMatch_EmptyResults(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	mid := msseedMatchID(t, sid, tid, tid, 1)

	detail, err := store.GetMatch(ctx, mid)
	if err != nil {
		t.Fatalf("GetMatch: %v", err)
	}
	if detail.Results == nil {
		t.Error("want non-nil empty results slice, got nil")
	}
	if len(detail.Results) != 0 {
		t.Errorf("want 0 results, got %d", len(detail.Results))
	}
}

func TestMatchStore_GetMatch_NotFound(t *testing.T) {
	store := newMatchStore(t)
	_, err := store.GetMatch(context.Background(), 9999)
	if !errors.Is(err, matches.ErrMatchNotFound) {
		t.Errorf("want ErrMatchNotFound, got %v", err)
	}
}

// ── AssignMatchTeams ──────────────────────────────────────────────────────────

func TestMatchStore_AssignMatchTeams_UpdatesBothTeams(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid1 := sseedTeam(t, lid, "Home")
	tid2 := sseedTeam(t, lid, "Away")
	// Insert blank slot.
	res, _ := db.DB.Exec(
		`INSERT INTO matches (season_id, week_number, completed) VALUES (?,1,0)`, sid)
	mid, _ := res.LastInsertId()

	if err := store.AssignMatchTeams(ctx, mid, &tid1, &tid2); err != nil {
		t.Fatalf("AssignMatchTeams: %v", err)
	}

	var gotHome, gotAway int64
	db.DB.QueryRow(`SELECT COALESCE(home_team_id,0), COALESCE(away_team_id,0) FROM matches WHERE id=?`, mid).
		Scan(&gotHome, &gotAway)
	if gotHome != tid1 {
		t.Errorf("want home_team_id=%d, got %d", tid1, gotHome)
	}
	if gotAway != tid2 {
		t.Errorf("want away_team_id=%d, got %d", tid2, gotAway)
	}
}

func TestMatchStore_AssignMatchTeams_NilNullsColumns(t *testing.T) {
	store := newMatchStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	mid := msseedMatchID(t, sid, tid, tid, 1)

	if err := store.AssignMatchTeams(ctx, mid, nil, nil); err != nil {
		t.Fatalf("AssignMatchTeams: %v", err)
	}

	var gotHome, gotAway *int64
	db.DB.QueryRow(`SELECT home_team_id, away_team_id FROM matches WHERE id=?`, mid).
		Scan(&gotHome, &gotAway)
	if gotHome != nil || gotAway != nil {
		t.Errorf("want both team IDs NULL, got home=%v away=%v", gotHome, gotAway)
	}
}
