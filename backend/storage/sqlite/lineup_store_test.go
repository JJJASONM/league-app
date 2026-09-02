package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// ── setup helper ──────────────────────────────────────────────────────────────

func newLineupStore(t *testing.T) *sqlite.LineupStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewLineupStore(db.DB)
}

// lsseedPlan inserts a lineup plan row and returns its ID.
func lsseedPlan(t *testing.T, seasonID, teamID, playerID, weekNum int64) int64 {
	t.Helper()
	res, err := db.DB.Exec(
		`INSERT INTO lineup_plans (season_id, team_id, week_number, player_id, is_sub) VALUES (?,?,?,?,0)`,
		seasonID, teamID, weekNum, playerID)
	if err != nil {
		t.Fatalf("insert lineup_plan: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ── ListLineupPlans ───────────────────────────────────────────────────────────

func TestLineupStore_ListLineupPlans_BySeason(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid := sseedPlayer(t, tid)
	lsseedPlan(t, sid, tid, pid, 1)
	lsseedPlan(t, sid, tid, pid, 2)

	plans, err := store.ListLineupPlans(ctx, matches.ListLineupPlansRequest{SeasonID: sid})
	if err != nil {
		t.Fatalf("ListLineupPlans: %v", err)
	}
	if len(plans) != 2 {
		t.Errorf("want 2 plans, got %d", len(plans))
	}
	for _, p := range plans {
		if p.SeasonID != sid {
			t.Errorf("want season_id=%d, got %d", sid, p.SeasonID)
		}
	}
}

func TestLineupStore_ListLineupPlans_ByWeek(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid := sseedPlayer(t, tid)
	lsseedPlan(t, sid, tid, pid, 1)
	lsseedPlan(t, sid, tid, pid, 2)

	plans, err := store.ListLineupPlans(ctx, matches.ListLineupPlansRequest{SeasonID: sid, WeekNumber: 1})
	if err != nil {
		t.Fatalf("ListLineupPlans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("want 1 plan for week 1, got %d", len(plans))
	}
	if plans[0].WeekNumber != 1 {
		t.Errorf("want week_number=1, got %d", plans[0].WeekNumber)
	}
}

func TestLineupStore_ListLineupPlans_ByTeam(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid1 := sseedTeam(t, lid, "A")
	tid2 := sseedTeam(t, lid, "B")
	pid1 := sseedPlayer(t, tid1)
	pid2 := sseedPlayer(t, tid2)
	lsseedPlan(t, sid, tid1, pid1, 1)
	lsseedPlan(t, sid, tid2, pid2, 1)

	plans, err := store.ListLineupPlans(ctx, matches.ListLineupPlansRequest{SeasonID: sid, TeamID: tid1})
	if err != nil {
		t.Fatalf("ListLineupPlans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("want 1 plan for team %d, got %d", tid1, len(plans))
	}
	if plans[0].TeamID != tid1 {
		t.Errorf("want team_id=%d, got %d", tid1, plans[0].TeamID)
	}
}

func TestLineupStore_ListLineupPlans_EmptySlice(t *testing.T) {
	store := newLineupStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	plans, err := store.ListLineupPlans(context.Background(), matches.ListLineupPlansRequest{SeasonID: sid})
	if err != nil {
		t.Fatalf("ListLineupPlans: %v", err)
	}
	if plans != nil {
		t.Errorf("want nil from store (service wraps to empty), got %v", plans)
	}
}

// ── SaveTeamLineup ────────────────────────────────────────────────────────────

func TestLineupStore_SaveTeamLineup_InsertsRows(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid1 := sseedPlayer(t, tid)
	pid2 := sseedPlayer(t, tid)

	err := store.SaveTeamLineup(ctx, matches.SaveLineupRequest{
		SeasonID:   sid,
		TeamID:     tid,
		WeekNumber: 1,
		PlayerIDs:  []int64{pid1, pid2},
	})
	if err != nil {
		t.Fatalf("SaveTeamLineup: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=? AND team_id=? AND week_number=1`, sid, tid).Scan(&count)
	if count != 2 {
		t.Errorf("want 2 rows inserted, got %d", count)
	}
}

func TestLineupStore_SaveTeamLineup_ReplacesExisting(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid1 := sseedPlayer(t, tid)
	pid2 := sseedPlayer(t, tid)
	pid3 := sseedPlayer(t, tid)
	lsseedPlan(t, sid, tid, pid1, 1)
	lsseedPlan(t, sid, tid, pid2, 1)

	// Replace with a single player.
	err := store.SaveTeamLineup(ctx, matches.SaveLineupRequest{
		SeasonID:   sid,
		TeamID:     tid,
		WeekNumber: 1,
		PlayerIDs:  []int64{pid3},
	})
	if err != nil {
		t.Fatalf("SaveTeamLineup: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=? AND team_id=? AND week_number=1`, sid, tid).Scan(&count)
	if count != 1 {
		t.Errorf("want 1 row after replace, got %d", count)
	}
	var gotPID int64
	db.DB.QueryRow(`SELECT player_id FROM lineup_plans WHERE season_id=? AND team_id=? AND week_number=1`, sid, tid).Scan(&gotPID)
	if gotPID != pid3 {
		t.Errorf("want player_id=%d after replace, got %d", pid3, gotPID)
	}
}

func TestLineupStore_SaveTeamLineup_SkipsZeroPlayerID(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid := sseedPlayer(t, tid)

	err := store.SaveTeamLineup(ctx, matches.SaveLineupRequest{
		SeasonID:   sid,
		TeamID:     tid,
		WeekNumber: 1,
		PlayerIDs:  []int64{0, pid, 0},
	})
	if err != nil {
		t.Fatalf("SaveTeamLineup: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=? AND team_id=? AND week_number=1`, sid, tid).Scan(&count)
	if count != 1 {
		t.Errorf("want 1 row (zero IDs skipped), got %d", count)
	}
}

// ── DeleteLineupPlan ──────────────────────────────────────────────────────────

func TestLineupStore_DeleteLineupPlan_DeletesRow(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid := sseedPlayer(t, tid)
	planID := lsseedPlan(t, sid, tid, pid, 1)

	if err := store.DeleteLineupPlan(ctx, planID); err != nil {
		t.Fatalf("DeleteLineupPlan: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE id=?`, planID).Scan(&count)
	if count != 0 {
		t.Errorf("want row gone, got count=%d", count)
	}
}

func TestLineupStore_DeleteLineupPlan_NonExistentNoError(t *testing.T) {
	store := newLineupStore(t)
	if err := store.DeleteLineupPlan(context.Background(), 9999); err != nil {
		t.Errorf("want no error deleting non-existent plan, got: %v", err)
	}
}

// -- GetLineupPlan -----------------------------------------------------------

func TestLineupStore_GetLineupPlan_ReturnsRow(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid := sseedPlayer(t, tid)
	planID := lsseedPlan(t, sid, tid, pid, 1)

	got, err := store.GetLineupPlan(ctx, planID)
	if err != nil {
		t.Fatalf("GetLineupPlan: %v", err)
	}
	if got.ID != planID || got.PlayerID != pid || got.SeasonID != sid || got.TeamID != tid {
		t.Errorf("want matching row, got %+v", got)
	}
	if got.IsSub {
		t.Error("want is_sub=false for a freshly-seeded plan")
	}
}

func TestLineupStore_GetLineupPlan_NotFound(t *testing.T) {
	store := newLineupStore(t)
	_, err := store.GetLineupPlan(context.Background(), 9999)
	if err == nil {
		t.Fatal("want error for non-existent plan")
	}
}

// -- FindMatchID -------------------------------------------------------------

func TestLineupStore_FindMatchID_FoundAsHomeOrAway(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tidHome := sseedTeam(t, lid, "Home")
	tidAway := sseedTeam(t, lid, "Away")
	res, err := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, match_number) VALUES (?,?,?,1,1)`,
		sid, tidHome, tidAway)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	wantMatchID, _ := res.LastInsertId()

	gotHome, found, err := store.FindMatchID(ctx, sid, tidHome, 1)
	if err != nil {
		t.Fatalf("FindMatchID (home): %v", err)
	}
	if !found || gotHome != wantMatchID {
		t.Errorf("want found=true matchID=%d, got found=%v matchID=%d", wantMatchID, found, gotHome)
	}

	gotAway, found, err := store.FindMatchID(ctx, sid, tidAway, 1)
	if err != nil {
		t.Fatalf("FindMatchID (away): %v", err)
	}
	if !found || gotAway != wantMatchID {
		t.Errorf("want found=true matchID=%d, got found=%v matchID=%d", wantMatchID, found, gotAway)
	}
}

func TestLineupStore_FindMatchID_NotFound(t *testing.T) {
	store := newLineupStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")

	_, found, err := store.FindMatchID(context.Background(), sid, tid, 1)
	if err != nil {
		t.Fatalf("FindMatchID: %v", err)
	}
	if found {
		t.Error("want found=false when no match is scheduled for this team/week")
	}
}

// -- SetSubstitute / ClearSubstitute -------------------------------------------

func TestLineupStore_SetSubstitute_UpdatesRowAndRecordsOriginal(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	original := sseedPlayer(t, tid)
	sub := sseedPlayer(t, tid)
	planID := lsseedPlan(t, sid, tid, original, 1)

	got, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID:       planID,
		SubstitutePlayerID: sub,
		OriginalPlayerID:   original,
	})
	if err != nil {
		t.Fatalf("SetSubstitute: %v", err)
	}
	if got.PlayerID != sub {
		t.Errorf("want player_id=%d (substitute), got %d", sub, got.PlayerID)
	}
	if !got.IsSub {
		t.Error("want is_sub=true after substitution")
	}
	if got.SubForID == nil || *got.SubForID != original {
		t.Errorf("want sub_for_id=%d, got %v", original, got.SubForID)
	}
	// Same row id, season/team/week context preserved.
	if got.ID != planID || got.SeasonID != sid || got.TeamID != tid || got.WeekNumber != 1 {
		t.Errorf("want season/team/week context preserved, got %+v", got)
	}
}

func TestLineupStore_SetSubstitute_DuplicatePlayerInLineup_ReturnsUniqueError(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	playerA := sseedPlayer(t, tid)
	playerB := sseedPlayer(t, tid)
	planA := lsseedPlan(t, sid, tid, playerA, 1)
	lsseedPlan(t, sid, tid, playerB, 1) // playerB already has a slot this week

	// Substituting playerB into playerA's slot collides with playerB's own
	// existing row under the same (season_id, team_id, week_number, player_id)
	// UNIQUE constraint.
	_, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID:       planA,
		SubstitutePlayerID: playerB,
		OriginalPlayerID:   playerA,
	})
	if err == nil {
		t.Fatal("want a UNIQUE constraint error when the substitute is already in this lineup")
	}
}

func TestLineupStore_ClearSubstitute_RevertsToOriginalPlayer(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	original := sseedPlayer(t, tid)
	sub := sseedPlayer(t, tid)
	planID := lsseedPlan(t, sid, tid, original, 1)
	if _, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	}); err != nil {
		t.Fatalf("SetSubstitute: %v", err)
	}

	got, err := store.ClearSubstitute(ctx, planID)
	if err != nil {
		t.Fatalf("ClearSubstitute: %v", err)
	}
	if got.PlayerID != original {
		t.Errorf("want player_id=%d (reverted to original), got %d", original, got.PlayerID)
	}
	if got.IsSub {
		t.Error("want is_sub=false after clearing")
	}
	if got.SubForID != nil {
		t.Errorf("want sub_for_id=nil after clearing, got %v", got.SubForID)
	}
}

func TestLineupStore_ClearSubstitute_NotCurrentlySubstituted_ReturnsError(t *testing.T) {
	store := newLineupStore(t)
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	pid := sseedPlayer(t, tid)
	planID := lsseedPlan(t, sid, tid, pid, 1)

	_, err := store.ClearSubstitute(context.Background(), planID)
	if err == nil {
		t.Fatal("want error clearing a slot that was never substituted")
	}
}
