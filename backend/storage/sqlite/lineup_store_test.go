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
