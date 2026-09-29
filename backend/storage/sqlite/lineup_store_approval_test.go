package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// lsseedMatch inserts a match for seasonID/homeID/awayID at week_number=1
// (matching lsseedPlan's convention) and returns its id.
func lsseedMatch(t *testing.T, seasonID, homeID, awayID int64, completed bool) int64 {
	t.Helper()
	c := 0
	if completed {
		c = 1
	}
	res, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed) VALUES (?,?,?,1,?)`,
		seasonID, homeID, awayID, c)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// --- Substitute swap: Player Score Approval Phase 1A revision/clear behavior ---

func TestSetSubstitute_OnScoredMatch_IncrementsRevisionAndClearsApprovals(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, true) // already scored

	roundStore := sqlite.NewRoundStore(db.DB)
	uid, pid := int64(1), int64(2)
	if err := roundStore.ApproveTeamSide(ctx, matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}

	if _, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	}); err != nil {
		t.Fatalf("SetSubstitute: %v", err)
	}

	state, err := roundStore.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatchApprovalState: %v", err)
	}
	if state.ScoreRevision != 1 {
		t.Errorf("want ScoreRevision=1 after a substitute swap on a scored match, got %d", state.ScoreRevision)
	}
	if state.HomeSide.State != matches.ApprovalStatePending {
		t.Errorf("want home approval cleared by the substitute swap, got %s", state.HomeSide.State)
	}
}

func TestSetSubstitute_OnUnscoredMatch_DoesNotBumpRevision(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, false) // not scored yet

	if _, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	}); err != nil {
		t.Fatalf("SetSubstitute: %v", err)
	}

	roundStore := sqlite.NewRoundStore(db.DB)
	state, err := roundStore.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatchApprovalState: %v", err)
	}
	if state.ScoreRevision != 0 {
		t.Errorf("want ScoreRevision unchanged (0) when the match has no scores yet, got %d", state.ScoreRevision)
	}
}

func TestSetSubstitute_NoMatchScheduledYet_DoesNotError(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "T")
	original := sseedPlayer(t, tid)
	sub := sseedPlayer(t, tid)
	planID := lsseedPlan(t, sid, tid, original, 1) // no match seeded for this team/week

	if _, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	}); err != nil {
		t.Fatalf("want SetSubstitute to succeed with nothing to invalidate, got: %v", err)
	}
}

func TestClearSubstitute_OnScoredMatch_IncrementsRevisionAndClearsApprovals(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, true)

	if _, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	}); err != nil {
		t.Fatalf("SetSubstitute: %v", err)
	}
	roundStore := sqlite.NewRoundStore(db.DB)
	uid, pid := int64(1), int64(2)
	if err := roundStore.ApproveTeamSide(ctx, matchID, "away", 1, &uid, &pid, "Bob"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}

	if _, err := store.ClearSubstitute(ctx, planID); err != nil {
		t.Fatalf("ClearSubstitute: %v", err)
	}
	state, err := roundStore.GetMatchApprovalState(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatchApprovalState: %v", err)
	}
	if state.ScoreRevision != 2 {
		t.Errorf("want ScoreRevision=2 (one bump per substitute change), got %d", state.ScoreRevision)
	}
	if state.AwaySide.State != matches.ApprovalStatePending {
		t.Errorf("want away approval cleared by ClearSubstitute, got %s", state.AwaySide.State)
	}
}

// --- Correction 4 (substitute mutations): the guard runs BEFORE the lineup
// update, and a locked match rolls back the WHOLE transaction -- the
// lineup_plans row must be left exactly as it was, never partially updated.

func TestSetSubstitute_RejectsAndRollsBackWhenAdminAccepted(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, true)

	roundStore := sqlite.NewRoundStore(db.DB)
	uid := int64(1)
	if err := roundStore.ApproveMatch(ctx, matchID, &uid, "admin ok"); err != nil {
		t.Fatalf("ApproveMatch: %v", err)
	}

	_, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	})
	if err == nil {
		t.Fatal("want SetSubstitute rejected once the match is admin-accepted, got nil")
	}
	plan, getErr := store.GetLineupPlan(ctx, planID)
	if getErr != nil {
		t.Fatalf("GetLineupPlan: %v", getErr)
	}
	if plan.PlayerID != original || plan.IsSub {
		t.Errorf("want the lineup_plans row left exactly as it was (player_id=%d, is_sub=false), got player_id=%d is_sub=%v",
			original, plan.PlayerID, plan.IsSub)
	}
}

func TestSetSubstitute_RejectsAndRollsBackWhenProcessed(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, true)

	roundStore := sqlite.NewRoundStore(db.DB)
	uid := int64(1)
	roundStore.ApproveMatch(ctx, matchID, &uid, "admin ok")
	if err := roundStore.ProcessMatch(ctx, matchID, &uid); err != nil {
		t.Fatalf("ProcessMatch: %v", err)
	}

	_, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	})
	if err == nil {
		t.Fatal("want SetSubstitute rejected once the match is processed, got nil")
	}
	plan, _ := store.GetLineupPlan(ctx, planID)
	if plan.PlayerID != original || plan.IsSub {
		t.Errorf("want the lineup_plans row left untouched, got player_id=%d is_sub=%v", plan.PlayerID, plan.IsSub)
	}
}

func TestSetSubstitute_RejectsAndRollsBackWhenWeekClosed(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, true)
	db.DB.Exec(`UPDATE matches SET week_closed = 1 WHERE id = ?`, matchID)

	_, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	})
	if err == nil {
		t.Fatal("want SetSubstitute rejected once the week is closed, got nil")
	}
	plan, _ := store.GetLineupPlan(ctx, planID)
	if plan.PlayerID != original || plan.IsSub {
		t.Errorf("want the lineup_plans row left untouched, got player_id=%d is_sub=%v", plan.PlayerID, plan.IsSub)
	}
}

func TestSetSubstitute_RejectsAndRollsBackWhenSeasonClosed(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, true)
	db.DB.Exec(`UPDATE seasons SET closed_at = CURRENT_TIMESTAMP WHERE id = ?`, sid)

	_, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	})
	if err == nil {
		t.Fatal("want SetSubstitute rejected once the season is closed, got nil")
	}
	plan, _ := store.GetLineupPlan(ctx, planID)
	if plan.PlayerID != original || plan.IsSub {
		t.Errorf("want the lineup_plans row left untouched, got player_id=%d is_sub=%v", plan.PlayerID, plan.IsSub)
	}
	_ = matchID
}

func TestClearSubstitute_RejectsAndRollsBackWhenAdminAccepted(t *testing.T) {
	store := newLineupStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	homeTeam := sseedTeam(t, lid, "Home")
	awayTeam := sseedTeam(t, lid, "Away")
	original := sseedPlayer(t, homeTeam)
	sub := sseedPlayer(t, homeTeam)
	planID := lsseedPlan(t, sid, homeTeam, original, 1)
	matchID := lsseedMatch(t, sid, homeTeam, awayTeam, true)

	if _, err := store.SetSubstitute(ctx, matches.SetSubstituteRequest{
		LineupPlanID: planID, SubstitutePlayerID: sub, OriginalPlayerID: original,
	}); err != nil {
		t.Fatalf("SetSubstitute: %v", err)
	}
	roundStore := sqlite.NewRoundStore(db.DB)
	uid := int64(1)
	if err := roundStore.ApproveMatch(ctx, matchID, &uid, "admin ok"); err != nil {
		t.Fatalf("ApproveMatch: %v", err)
	}

	_, err := store.ClearSubstitute(ctx, planID)
	if err == nil {
		t.Fatal("want ClearSubstitute rejected once the match is admin-accepted, got nil")
	}
	plan, _ := store.GetLineupPlan(ctx, planID)
	if plan.PlayerID != sub || !plan.IsSub {
		t.Errorf("want the lineup_plans row left exactly as the prior substitute left it (player_id=%d, is_sub=true), got player_id=%d is_sub=%v",
			sub, plan.PlayerID, plan.IsSub)
	}
}
