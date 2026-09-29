package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/storage/sqlite"
	"league_app/db"
)

// TestPlayerStore_MergePlayers_RepointsActiveApprovalActorPlayerID verifies
// Player Score Approval Phase 1A's extension to mergeRepointStmts: a
// player's CURRENT team-side approval attribution follows them into the
// merge target, exactly like round_results/season_rosters already do,
// while the permanent match_approval_events history (a separate table,
// deliberately outside mergeRepointStmts) is left completely untouched --
// including its actor_player_id and actor_name_snapshot, which must remain
// readable and unchanged after the merge.
func TestPlayerStore_MergePlayers_RepointsActiveApprovalActorPlayerID(t *testing.T) {
	playerStore := newPlayerStore(t)
	roundStore := sqlite.NewRoundStore(db.DB)
	ctx := context.Background()

	leagueID := seedLeagueRow(t)
	teamID := seedTeamRow(t, leagueID, "Team One")
	seasonID := seedSeasonRow(t, leagueID)
	source := seedPlayerRow(t, "Source", "Player")
	target := seedPlayerRow(t, "Target", "Player")

	matchHome := seedMatchRow(t, seasonID, teamID, teamID, 1)
	matchAway := seedMatchRow(t, seasonID, teamID, teamID, 2)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id IN (?,?)`, matchHome, matchAway)

	// source currently holds the active home approval on matchHome and the
	// active away approval on matchAway.
	if err := roundStore.ApproveTeamSide(ctx, matchHome, "home", 0, nil, &source, "Source Player"); err != nil {
		t.Fatalf("ApproveTeamSide home: %v", err)
	}
	if err := roundStore.ApproveTeamSide(ctx, matchAway, "away", 0, nil, &source, "Source Player"); err != nil {
		t.Fatalf("ApproveTeamSide away: %v", err)
	}

	if err := playerStore.MergePlayers(ctx, source, target); err != nil {
		t.Fatalf("MergePlayers: %v", err)
	}

	stateHome, err := roundStore.GetMatchApprovalState(ctx, matchHome)
	if err != nil {
		t.Fatalf("GetMatchApprovalState(matchHome): %v", err)
	}
	if stateHome.HomeSide.ActorPlayerID == nil || *stateHome.HomeSide.ActorPlayerID != target {
		t.Errorf("want matchHome's active home approval actor repointed to target %d, got %v", target, stateHome.HomeSide.ActorPlayerID)
	}
	if stateHome.HomeSide.State != "approved" {
		t.Errorf("want matchHome home side still approved after merge, got %s", stateHome.HomeSide.State)
	}

	stateAway, err := roundStore.GetMatchApprovalState(ctx, matchAway)
	if err != nil {
		t.Fatalf("GetMatchApprovalState(matchAway): %v", err)
	}
	if stateAway.AwaySide.ActorPlayerID == nil || *stateAway.AwaySide.ActorPlayerID != target {
		t.Errorf("want matchAway's active away approval actor repointed to target %d, got %v", target, stateAway.AwaySide.ActorPlayerID)
	}

	// Historical match_approval_events rows are NOT repointed -- they keep
	// the pre-merge source id, readable via their own name snapshot.
	homeEvents, err := roundStore.ListApprovalEvents(ctx, matchHome)
	if err != nil {
		t.Fatalf("ListApprovalEvents(matchHome): %v", err)
	}
	if len(homeEvents) != 1 {
		t.Fatalf("want exactly 1 event for matchHome, got %d", len(homeEvents))
	}
	if homeEvents[0].ActorPlayerID == nil || *homeEvents[0].ActorPlayerID != source {
		t.Errorf("want historical event actor_player_id left as the pre-merge source id %d, got %v", source, homeEvents[0].ActorPlayerID)
	}
	if homeEvents[0].ActorNameSnapshot != "Source Player" {
		t.Errorf("want historical actor_name_snapshot unchanged, got %q", homeEvents[0].ActorNameSnapshot)
	}

	// The now-merged-away source player must actually be gone (proves the
	// new mergeRepointStmts entries did not introduce a delete blocker).
	if playerIDExists(t, source) {
		t.Error("want source player deleted after merge")
	}
}
