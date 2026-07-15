package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/storage/sqlite"
	"league_app/db"
)

func newPushbackStore(t *testing.T) *sqlite.PushbackStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewPushbackStore(db.DB)
}

// pseedMatch inserts a match with a specified week_number, match_date, and completed flag.
func pseedMatch(t *testing.T, seasonID, homeID, awayID int64, weekNum int, date string, completed bool) int64 {
	t.Helper()
	c := 0
	if completed {
		c = 1
	}
	var dateArg any
	if date != "" {
		dateArg = date
	}
	res, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, match_date, completed)
		 VALUES (?,?,?,?,?,?)`,
		seasonID, homeID, awayID, weekNum, dateArg, c)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// --- SeasonExists ---

func TestPushbackStore_SeasonExists_True(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	ok, err := store.SeasonExists(context.Background(), sid)
	if err != nil {
		t.Fatalf("SeasonExists: %v", err)
	}
	if !ok {
		t.Error("want true for existing season")
	}
}

func TestPushbackStore_SeasonExists_False(t *testing.T) {
	store := newPushbackStore(t)

	ok, err := store.SeasonExists(context.Background(), 9999)
	if err != nil {
		t.Fatalf("SeasonExists: %v", err)
	}
	if ok {
		t.Error("want false for non-existent season")
	}
}

// --- HasClosedWeeksAtOrAfter ---

func TestPushbackStore_HasClosedWeeksAtOrAfter_TrueWhenClosedAtCutoff(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	sseedLeagueWeek(t, sid, 5, "closed")

	got, err := store.HasClosedWeeksAtOrAfter(context.Background(), sid, 5)
	if err != nil {
		t.Fatalf("HasClosedWeeksAtOrAfter: %v", err)
	}
	if !got {
		t.Error("want true when closed week exists at cutoff")
	}
}

func TestPushbackStore_HasClosedWeeksAtOrAfter_TrueWhenClosedAfterCutoff(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	sseedLeagueWeek(t, sid, 7, "closed")

	got, err := store.HasClosedWeeksAtOrAfter(context.Background(), sid, 5)
	if err != nil {
		t.Fatalf("HasClosedWeeksAtOrAfter: %v", err)
	}
	if !got {
		t.Error("want true when closed week exists after cutoff")
	}
}

func TestPushbackStore_HasClosedWeeksAtOrAfter_FalseWhenClosedOnlyBefore(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	sseedLeagueWeek(t, sid, 3, "closed") // before cutoff of 5

	got, err := store.HasClosedWeeksAtOrAfter(context.Background(), sid, 5)
	if err != nil {
		t.Fatalf("HasClosedWeeksAtOrAfter: %v", err)
	}
	if got {
		t.Error("want false when closed week only before cutoff")
	}
}

func TestPushbackStore_HasClosedWeeksAtOrAfter_FalseWhenNoneExist(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	got, err := store.HasClosedWeeksAtOrAfter(context.Background(), sid, 1)
	if err != nil {
		t.Fatalf("HasClosedWeeksAtOrAfter: %v", err)
	}
	if got {
		t.Error("want false when no league_weeks rows exist")
	}
}

// --- GetPushbackMatches ---

func TestPushbackStore_GetPushbackMatches_ReturnsAllForSeason(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	pseedMatch(t, sid, t1, t2, 1, "2026-09-01", false)
	pseedMatch(t, sid, t2, t1, 2, "2026-09-08", true)

	rows, err := store.GetPushbackMatches(context.Background(), sid)
	if err != nil {
		t.Fatalf("GetPushbackMatches: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 matches, got %d", len(rows))
	}
}

func TestPushbackStore_GetPushbackMatches_CompletedFlagCorrect(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	pseedMatch(t, sid, t1, t2, 1, "2026-09-01", false)
	pseedMatch(t, sid, t2, t1, 2, "2026-09-08", true)

	rows, err := store.GetPushbackMatches(context.Background(), sid)
	if err != nil {
		t.Fatalf("GetPushbackMatches: %v", err)
	}
	if rows[0].Completed {
		t.Error("want week 1 match not completed")
	}
	if !rows[1].Completed {
		t.Error("want week 2 match completed")
	}
}

func TestPushbackStore_GetPushbackMatches_NullDateReturnsNilPointer(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	pseedMatch(t, sid, t1, t2, 1, "", false) // no date

	rows, err := store.GetPushbackMatches(context.Background(), sid)
	if err != nil {
		t.Fatalf("GetPushbackMatches: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].MatchDate != nil {
		t.Errorf("want nil MatchDate for undated match, got %q", *rows[0].MatchDate)
	}
}

func TestPushbackStore_GetPushbackMatches_ExcludesOtherSeasons(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid1 := sseedSeason(t, lid, "S1", "", "", false)
	sid2 := sseedSeason(t, lid, "S2", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	pseedMatch(t, sid1, t1, t2, 1, "2026-09-01", false)
	pseedMatch(t, sid2, t1, t2, 1, "2026-09-01", false) // other season

	rows, err := store.GetPushbackMatches(context.Background(), sid1)
	if err != nil {
		t.Fatalf("GetPushbackMatches: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("want 1 match for sid1 only, got %d", len(rows))
	}
}
