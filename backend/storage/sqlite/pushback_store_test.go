package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"league_app/backend/domains/matches"
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

// --- ApplyPushback ---

// seasonRow reads end_date and schedule_stale for a season directly from DB.
func seasonRow(t *testing.T, sid int64) (endDate sql.NullString, stale int) {
	t.Helper()
	err := db.DB.QueryRow(
		`SELECT strftime('%Y-%m-%d', end_date), COALESCE(schedule_stale,0) FROM seasons WHERE id=?`, sid,
	).Scan(&endDate, &stale)
	if err != nil {
		t.Fatalf("seasonRow %d: %v", sid, err)
	}
	return
}

// matchRow reads week_number and match_date for a match directly from DB.
func matchRow(t *testing.T, mid int64) (weekNum int, matchDate sql.NullString) {
	t.Helper()
	err := db.DB.QueryRow(
		`SELECT week_number, strftime('%Y-%m-%d', match_date) FROM matches WHERE id=?`, mid,
	).Scan(&weekNum, &matchDate)
	if err != nil {
		t.Fatalf("matchRow %d: %v", mid, err)
	}
	return
}

func TestApplyPushback_ShiftsWeekNumberAndDate(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	mid := pseedMatch(t, sid, t1, t2, 5, "2026-10-06", false)

	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{mid},
		WeeksToAdd: 1,
		DayShift:   7,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	wn, md := matchRow(t, mid)
	if wn != 6 {
		t.Errorf("want week_number=6, got %d", wn)
	}
	if !md.Valid || md.String != "2026-10-13" {
		t.Errorf("want match_date=2026-10-13, got %v", md)
	}
}

func TestApplyPushback_PreservesCompletedMatches(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	mid := pseedMatch(t, sid, t1, t2, 5, "2026-10-06", true) // completed - NOT in ShiftedIDs

	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{}, // completed match excluded by service
		WeeksToAdd: 1,
		DayShift:   7,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	wn, md := matchRow(t, mid)
	if wn != 5 {
		t.Errorf("want week_number unchanged=5, got %d", wn)
	}
	if !md.Valid || md.String != "2026-10-06" {
		t.Errorf("want match_date unchanged=2026-10-06, got %v", md)
	}
}

func TestApplyPushback_LeavesBeforeCutoffUnchanged(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	// week 3 is before cutoff 5; service excludes it from ShiftedIDs
	mid := pseedMatch(t, sid, t1, t2, 3, "2026-09-22", false)

	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{},
		WeeksToAdd: 1,
		DayShift:   7,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	wn, md := matchRow(t, mid)
	if wn != 3 {
		t.Errorf("want week_number=3, got %d", wn)
	}
	if !md.Valid || md.String != "2026-09-22" {
		t.Errorf("want match_date=2026-09-22, got %v", md)
	}
}

func TestApplyPushback_NullDateStaysNull(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	mid := pseedMatch(t, sid, t1, t2, 5, "", false) // no date

	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{mid},
		WeeksToAdd: 1,
		DayShift:   7,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	wn, md := matchRow(t, mid)
	if wn != 6 {
		t.Errorf("want week_number=6, got %d", wn)
	}
	if md.Valid {
		t.Errorf("want match_date NULL, got %q", md.String)
	}
}

func TestApplyPushback_UpdatesSeasonEndDate(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	newEnd := "2026-11-03"
	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{},
		WeeksToAdd: 1,
		DayShift:   7,
		NewEndDate: &newEnd,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	ed, _ := seasonRow(t, sid)
	if !ed.Valid || ed.String != "2026-11-03" {
		t.Errorf("want end_date=2026-11-03, got %v", ed)
	}
}

func TestApplyPushback_ClearsScheduleStale(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	if _, err := db.DB.Exec(`UPDATE seasons SET schedule_stale = 1 WHERE id = ?`, sid); err != nil {
		t.Fatalf("set schedule_stale: %v", err)
	}

	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{},
		WeeksToAdd: 1,
		DayShift:   7,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	_, stale := seasonRow(t, sid)
	if stale != 0 {
		t.Errorf("want schedule_stale=0, got %d", stale)
	}
}

func TestApplyPushback_IgnoresCompletedMatchInShiftedIDs(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	mid := pseedMatch(t, sid, t1, t2, 5, "2026-10-06", true) // completed

	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{mid}, // defensive guard must reject this
		WeeksToAdd: 1,
		DayShift:   7,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	wn, md := matchRow(t, mid)
	if wn != 5 {
		t.Errorf("want week_number unchanged=5, got %d", wn)
	}
	if !md.Valid || md.String != "2026-10-06" {
		t.Errorf("want match_date unchanged=2026-10-06, got %v", md)
	}
}

func TestApplyPushback_DoesNotMutateSkippedWeeks(t *testing.T) {
	store := newPushbackStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	if _, err := db.DB.Exec(
		`INSERT INTO skipped_weeks (season_id, skip_date, reason) VALUES (?, '2026-10-06', 'test')`, sid,
	); err != nil {
		t.Fatalf("insert skipped_week: %v", err)
	}

	err := store.ApplyPushback(context.Background(), matches.PushbackApplyInput{
		SeasonID:   sid,
		ShiftedIDs: []int64{},
		WeeksToAdd: 1,
		DayShift:   7,
	})
	if err != nil {
		t.Fatalf("ApplyPushback: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM skipped_weeks WHERE season_id=?`, sid).Scan(&count)
	if count != 1 {
		t.Errorf("want skipped_weeks row unchanged (count=1), got %d", count)
	}
}
