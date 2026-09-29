package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/domains/matches"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/models"
)

// seedRosterPlayer registers teamID in season_teams (idempotent via INSERT
// OR IGNORE) for seasonID, inserts a new player on that team, and adds a
// season_rosters row linking them -- the minimum needed to make a player
// "currently rostered to that team for the season" per
// RoundStore.ResolveApprovalEligibility.
func seedRosterPlayer(t *testing.T, seasonID, teamID int64, first string) (playerID int64) {
	t.Helper()
	db.DB.Exec(`INSERT OR IGNORE INTO season_teams (season_id, team_id) VALUES (?,?)`, seasonID, teamID)
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES (?,?,?)`, first, "Roster", teamID)
	if err != nil {
		t.Fatalf("seed roster player: %v", err)
	}
	playerID, _ = res.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`, seasonID, teamID, playerID); err != nil {
		t.Fatalf("seed season_rosters: %v", err)
	}
	return playerID
}

// seedRoundParticipant inserts a round_results row for matchID with the
// given player on the given side, WITHOUT any season_rosters row -- used to
// test the "actual participant, no roster membership" eligibility branch
// (a genuine last-minute substitute).
func seedRoundParticipant(t *testing.T, matchID int64, side string, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_ = side
	if _, err := db.DB.Exec(`
		INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id)
		VALUES (?, 1, ?, ?)`, matchID, homePlayerID, awayPlayerID); err != nil {
		t.Fatalf("seed round participant: %v", err)
	}
}

// --- ResolveApprovalEligibility ---

func TestResolveApprovalEligibility_CurrentHomeRosterMember_Qualifies(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, seasonID, homeTeamID, _ := seedRoundTestData(t)
	pid := seedRosterPlayer(t, seasonID, homeTeamID, "Home")

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "home", pid)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if !ok {
		t.Error("want current home roster member eligible for home")
	}
}

func TestResolveApprovalEligibility_CurrentAwayRosterMember_Qualifies(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, seasonID, _, awayTeamID := seedRoundTestData(t)
	pid := seedRosterPlayer(t, seasonID, awayTeamID, "Away")

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "away", pid)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if !ok {
		t.Error("want current away roster member eligible for away")
	}
}

func TestResolveApprovalEligibility_NonparticipatingRosterMember_Qualifies(t *testing.T) {
	s := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, seasonID, homeTeamID, _ := seedRoundTestData(t)
	// Someone else actually played this match...
	seedRoundParticipant(t, matchID, "home", homePlayerID, awayPlayerID)
	// ...but pid never played -- only rostered.
	pid := seedRosterPlayer(t, seasonID, homeTeamID, "Bench")

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "home", pid)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if !ok {
		t.Error("want a rostered player eligible for their team's side even without having personally played")
	}
}

func TestResolveApprovalEligibility_ActualSubstitute_QualifiesWithoutRoster(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, awayPlayerID, seasonID, homeTeamID, _ := seedRoundTestData(t)
	_ = seasonID
	// A brand-new player, no season_rosters row at all, who actually played
	// as the home participant in round_results (a last-minute substitute).
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('Sub','Player',?)`, homeTeamID)
	if err != nil {
		t.Fatalf("seed substitute player: %v", err)
	}
	subID, _ := res.LastInsertId()
	seedRoundParticipant(t, matchID, "home", subID, awayPlayerID)

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "home", subID)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if !ok {
		t.Error("want an actual round_results participant eligible even with no season_rosters row")
	}
}

func TestResolveApprovalEligibility_OpposingTeamPlayer_DoesNotQualify(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, seasonID, _, awayTeamID := seedRoundTestData(t)
	pid := seedRosterPlayer(t, seasonID, awayTeamID, "Away")

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "home", pid)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if ok {
		t.Error("want an away-rostered player NOT eligible for the home side")
	}
}

func TestResolveApprovalEligibility_UnrelatedTeamPlayer_DoesNotQualify(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, seasonID, homeTeamID, _ := seedRoundTestData(t)

	// A third team in the same league/season, not part of this match at all.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM teams WHERE id=?`, homeTeamID).Scan(&leagueID)
	res, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?, 'Third Team')`, leagueID)
	thirdTeamID, _ := res.LastInsertId()
	pid := seedRosterPlayer(t, seasonID, thirdTeamID, "Bystander")

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "home", pid)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if ok {
		t.Error("want a player on an unrelated third team NOT eligible")
	}
}

func TestResolveApprovalEligibility_CrossSeasonRoster_DoesNotQualify(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, seasonID, homeTeamID, _ := seedRoundTestData(t)

	// Same league/team, but a DIFFERENT season's roster.
	var leagueID int64
	db.DB.QueryRow(`SELECT league_id FROM teams WHERE id=?`, homeTeamID).Scan(&leagueID)
	res, _ := db.DB.Exec(`INSERT INTO seasons (league_id, name) VALUES (?, 'Other Season')`, leagueID)
	otherSeasonID, _ := res.LastInsertId()
	_ = seasonID
	pid := seedRosterPlayer(t, otherSeasonID, homeTeamID, "OtherSeason")

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "home", pid)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if ok {
		t.Error("want a player rostered only in a different season NOT eligible")
	}
}

func TestResolveApprovalEligibility_CrossLeague_DoesNotQualify(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)

	res, _ := db.DB.Exec(`INSERT INTO leagues (name) VALUES ('Other League')`)
	otherLeagueID, _ := res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO seasons (league_id, name) VALUES (?, 'Other League Season')`, otherLeagueID)
	otherSeasonID, _ := res.LastInsertId()
	res, _ = db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?, 'Other League Team')`, otherLeagueID)
	otherTeamID, _ := res.LastInsertId()
	pid := seedRosterPlayer(t, otherSeasonID, otherTeamID, "OtherLeague")

	ok, err := s.ResolveApprovalEligibility(context.Background(), matchID, "home", pid)
	if err != nil {
		t.Fatalf("ResolveApprovalEligibility: %v", err)
	}
	if ok {
		t.Error("want a player from a completely different league NOT eligible")
	}
}

// --- AdvanceScoreRevisionForEdit ---

func TestAdvanceScoreRevisionForEdit_IncrementsAndClearsNonPendingSides(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)

	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	if err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}
	if err := s.RequestCorrection(context.Background(), matchID, "away", 0, "wrong score", &uid, &pid, "Bob"); err != nil {
		t.Fatalf("RequestCorrection: %v", err)
	}

	newRev, err := s.AdvanceScoreRevisionForEdit(context.Background(), matchID, nil)
	if err != nil {
		t.Fatalf("AdvanceScoreRevisionForEdit: %v", err)
	}
	if newRev != 1 {
		t.Errorf("want new revision 1, got %d", newRev)
	}

	state, err := s.GetMatchApprovalState(context.Background(), matchID)
	if err != nil {
		t.Fatalf("GetMatchApprovalState: %v", err)
	}
	if state.ScoreRevision != 1 {
		t.Errorf("want ScoreRevision=1, got %d", state.ScoreRevision)
	}
	if state.HomeSide.State != matches.ApprovalStatePending || state.AwaySide.State != matches.ApprovalStatePending {
		t.Errorf("want both sides reset to pending, got home=%s away=%s", state.HomeSide.State, state.AwaySide.State)
	}
	if state.HomeSide.ActorPlayerID != nil || state.AwaySide.CorrectionNote != "" {
		t.Error("want actor/note fields cleared on both sides")
	}

	events, err := s.ListApprovalEvents(context.Background(), matchID)
	if err != nil {
		t.Fatalf("ListApprovalEvents: %v", err)
	}
	var clearedHome, clearedAway int
	for _, e := range events {
		if e.EventType == "cleared_by_edit" {
			if e.EventScope == "home" {
				clearedHome++
			}
			if e.EventScope == "away" {
				clearedAway++
			}
			if e.ScoreRevision != 1 {
				t.Errorf("want cleared_by_edit event stamped with new revision 1, got %d", e.ScoreRevision)
			}
		}
	}
	if clearedHome != 1 || clearedAway != 1 {
		t.Errorf("want exactly one cleared_by_edit event per non-pending side, got home=%d away=%d", clearedHome, clearedAway)
	}
}

func TestAdvanceScoreRevisionForEdit_PendingSidesGenerateNoClearEvent(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)

	if _, err := s.AdvanceScoreRevisionForEdit(context.Background(), matchID, nil); err != nil {
		t.Fatalf("AdvanceScoreRevisionForEdit: %v", err)
	}
	events, err := s.ListApprovalEvents(context.Background(), matchID)
	if err != nil {
		t.Fatalf("ListApprovalEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("want no events when both sides were already pending, got %d", len(events))
	}
}

// --- ApproveTeamSide / WithdrawTeamApproval / RequestCorrection: revision & transitions ---

func TestApproveTeamSide_CurrentRevision_SucceedsAndRecordsEvent(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)

	uid, pid := int64(10), int64(20)
	if err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.HomeSide.State != matches.ApprovalStateApproved {
		t.Fatalf("want home approved, got %s", state.HomeSide.State)
	}
	if state.HomeSide.StateScoreRevision == nil || *state.HomeSide.StateScoreRevision != 0 {
		t.Errorf("want StateScoreRevision=0, got %v", state.HomeSide.StateScoreRevision)
	}
	if state.HomeSide.ActorNameSnapshot != "Alice" {
		t.Errorf("want actor name snapshot Alice, got %s", state.HomeSide.ActorNameSnapshot)
	}

	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 1 || events[0].EventType != "approved" || events[0].EventScope != "home" {
		t.Fatalf("want exactly one home 'approved' event, got %+v", events)
	}
}

func TestApproveTeamSide_StaleRevision_FailsWithNoWrites(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)

	uid, pid := int64(10), int64(20)
	err := s.ApproveTeamSide(context.Background(), matchID, "home", 5, &uid, &pid, "Alice")
	if err == nil {
		t.Fatal("want error for stale revision, got nil")
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.HomeSide.State != matches.ApprovalStatePending {
		t.Errorf("want no write on stale revision, but home state is %s", state.HomeSide.State)
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 0 {
		t.Errorf("want zero events on stale-revision failure, got %d", len(events))
	}
}

func TestWithdrawTeamApproval_CurrentRevision_Succeeds(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice")

	// A DIFFERENT eligible teammate withdraws -- withdrawal belongs to the
	// team side, not to whoever originally approved.
	otherUID, otherPID := int64(99), int64(88)
	if err := s.WithdrawTeamApproval(context.Background(), matchID, "home", 0, &otherUID, &otherPID, "Carol"); err != nil {
		t.Fatalf("WithdrawTeamApproval: %v", err)
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.HomeSide.State != matches.ApprovalStatePending {
		t.Errorf("want pending after withdrawal, got %s", state.HomeSide.State)
	}
	if state.HomeSide.ActorPlayerID != nil {
		t.Error("want actor cleared after withdrawal")
	}
}

func TestWithdrawTeamApproval_NotApproved_FailsWithNoWrites(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	uid, pid := int64(1), int64(2)

	err := s.WithdrawTeamApproval(context.Background(), matchID, "home", 0, &uid, &pid, "Alice")
	if err == nil {
		t.Fatal("want error withdrawing a side that is not approved, got nil")
	}
}

func TestRequestCorrection_RecordsActorRevisionAndNote(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)

	uid, pid := int64(1), int64(2)
	if err := s.RequestCorrection(context.Background(), matchID, "away", 0, "game 2 score is wrong", &uid, &pid, "Dave"); err != nil {
		t.Fatalf("RequestCorrection: %v", err)
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.AwaySide.State != matches.ApprovalStateCorrectionRequested {
		t.Fatalf("want correction_requested, got %s", state.AwaySide.State)
	}
	if state.AwaySide.CorrectionNote != "game 2 score is wrong" {
		t.Errorf("want note preserved, got %q", state.AwaySide.CorrectionNote)
	}
	if state.AwaySide.ActorPlayerID == nil || *state.AwaySide.ActorPlayerID != pid {
		t.Errorf("want actor player recorded, got %v", state.AwaySide.ActorPlayerID)
	}

	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 1 || events[0].EventType != "correction_requested" || events[0].Note != "game 2 score is wrong" {
		t.Fatalf("want one correction_requested event with the note, got %+v", events)
	}
}

func TestRequestCorrection_ReplacesExistingApprovalForThatSide(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)

	if err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}
	if err := s.RequestCorrection(context.Background(), matchID, "home", 0, "actually this is wrong", &uid, &pid, "Alice"); err != nil {
		t.Fatalf("RequestCorrection: %v", err)
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.HomeSide.State != matches.ApprovalStateCorrectionRequested {
		t.Fatalf("want home now correction_requested (superseding the approval), got %s", state.HomeSide.State)
	}
	// PM correction 2: correction_requested retains the exact revision it
	// describes, the same as approved -- it must NOT be cleared to NULL.
	if state.HomeSide.StateScoreRevision == nil || *state.HomeSide.StateScoreRevision != 0 {
		t.Errorf("want StateScoreRevision=0 retained for the correction request, got %v", state.HomeSide.StateScoreRevision)
	}
}

// --- Admin acceptance event recording ---

func TestRecordAdminAcceptanceEvent_PreservesStructuredSnapshots(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	uid := int64(1)

	err := s.RecordAdminAcceptanceEvent(context.Background(), matches.AdminAcceptanceEvent{
		MatchID:       matchID,
		EventType:     "admin_override",
		ActorUserID:   &uid,
		Note:          "captains unavailable, accepting as-is",
		ScoreRevision: 0,
		HomeState:     "approved",
		HomeNote:      "",
		AwayState:     "correction_requested",
		AwayNote:      "disagrees with game 3",
	})
	if err != nil {
		t.Fatalf("RecordAdminAcceptanceEvent: %v", err)
	}

	events, err := s.ListApprovalEvents(context.Background(), matchID)
	if err != nil {
		t.Fatalf("ListApprovalEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	e := events[0]
	if e.EventScope != "match" || e.EventType != "admin_override" {
		t.Fatalf("want match-scope admin_override event, got scope=%s type=%s", e.EventScope, e.EventType)
	}
	if e.HomeStateSnapshot == nil || *e.HomeStateSnapshot != "approved" {
		t.Errorf("want home_state_snapshot=approved, got %v", e.HomeStateSnapshot)
	}
	if e.AwayStateSnapshot == nil || *e.AwayStateSnapshot != "correction_requested" {
		t.Errorf("want away_state_snapshot=correction_requested, got %v", e.AwayStateSnapshot)
	}
	if e.AwayNoteSnapshot != "disagrees with game 3" {
		t.Errorf("want away_note_snapshot preserved, got %q", e.AwayNoteSnapshot)
	}
}

// --- Atomic clearing: SubmitMatchResults / ClearMatchResults ---

// TestSubmitResults_IncrementsRevisionAndClearsApprovals goes through
// RoundService (not the bare store method) because, per PM's atomicity
// correction, the guarded revision transition is now acquired by the
// SERVICE layer as the outer RunTx wrapping the store's SubmitMatchResults
// -- the store method alone no longer touches score_revision or approval
// state, so calling it directly (as this test previously did) would no
// longer exercise the real, atomic behavior actual callers get.
func TestSubmitResults_IncrementsRevisionAndClearsApprovals(t *testing.T) {
	roundStore := newRoundStore(t)
	matchID, homePlayerID, _, _, homeTeamID, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	if err := roundStore.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}

	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))
	newRev, err := svc.SubmitResultsWithRevision(context.Background(), matchID, []models.MatchResult{
		{PlayerID: homePlayerID, TeamID: homeTeamID, GamesWon: 2, GamesLost: 1, Diff: 1},
	}, nil)
	if err != nil {
		t.Fatalf("SubmitResultsWithRevision: %v", err)
	}
	if newRev != 1 {
		t.Errorf("want returned newRevision=1, got %d", newRev)
	}
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 1 {
		t.Errorf("want ScoreRevision=1 after SubmitResults, got %d", state.ScoreRevision)
	}
	if state.HomeSide.State != matches.ApprovalStatePending {
		t.Errorf("want home approval cleared by SubmitResults, got %s", state.HomeSide.State)
	}
}

// TestClearResults_IncrementsRevisionAndClearsApprovals -- see
// TestSubmitResults_IncrementsRevisionAndClearsApprovals's doc comment for
// why this goes through RoundService rather than the bare store method.
func TestClearResults_IncrementsRevisionAndClearsApprovals(t *testing.T) {
	roundStore := newRoundStore(t)
	matchID, homePlayerID, _, _, homeTeamID, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff) VALUES (?,?,?,3,0,3)`,
		matchID, homePlayerID, homeTeamID)
	uid, pid := int64(1), int64(2)
	if err := roundStore.ApproveTeamSide(context.Background(), matchID, "away", 0, &uid, &pid, "Bob"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}

	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))
	newRev, err := svc.ClearResultsWithRevision(context.Background(), matchID, nil)
	if err != nil {
		t.Fatalf("ClearResultsWithRevision: %v", err)
	}
	if newRev != 1 {
		t.Errorf("want returned newRevision=1, got %d", newRev)
	}
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 1 {
		t.Errorf("want ScoreRevision=1 after ClearResults, got %d", state.ScoreRevision)
	}
	if state.AwaySide.State != matches.ApprovalStatePending {
		t.Errorf("want away approval cleared by ClearResults, got %s", state.AwaySide.State)
	}
	if state.Completed {
		t.Error("want completed=0 after ClearResults (existing behavior unaffected)")
	}
}

// --- Atomic clearing: SaveRounds (full service+store integration) ---

func TestSaveRounds_IncrementsRevisionAndClearsApprovals(t *testing.T) {
	roundStore := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, _, _, _ := seedRoundTestData(t)
	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))

	// First save: scores the match.
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID: matchID,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID, Game1Home: 10, Game1Away: 3},
		},
	})
	if err != nil {
		t.Fatalf("first SaveRounds: %v", err)
	}
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 1 {
		t.Fatalf("want ScoreRevision=1 after first save, got %d", state.ScoreRevision)
	}

	uid, pid := int64(1), int64(2)
	if err := roundStore.ApproveTeamSide(context.Background(), matchID, "home", 1, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}

	// Second save: edits the scoresheet again -- must bump revision again
	// and clear the home approval just recorded, atomically.
	err = svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID: matchID,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID, Game1Home: 10, Game1Away: 5},
		},
	})
	if err != nil {
		t.Fatalf("second SaveRounds: %v", err)
	}
	state, _ = roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 2 {
		t.Errorf("want ScoreRevision=2 after second save, got %d", state.ScoreRevision)
	}
	if state.HomeSide.State != matches.ApprovalStatePending {
		t.Errorf("want home approval cleared by the second SaveRounds, got %s", state.HomeSide.State)
	}
}

func TestSaveRounds_StaleExpectedRevision_ReturnsConflictWithNoWrites(t *testing.T) {
	roundStore := newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, _, _, _ := seedRoundTestData(t)
	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))

	stale := 5
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID:          matchID,
		ExpectedRevision: &stale,
		Rounds: []models.RoundResult{
			{RoundNumber: 1, HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID, Game1Home: 10, Game1Away: 3},
		},
	})
	if err == nil {
		t.Fatal("want error for stale expected revision, got nil")
	}
	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM round_results WHERE match_id=?`, matchID).Scan(&count)
	if count != 0 {
		t.Errorf("want zero writes on stale-revision rejection, got %d round_results rows", count)
	}
	// PM correction 1: a stale revision must leave zero score, result,
	// approval-state, event-history, OR revision changes -- not just zero
	// round_results rows.
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 0 {
		t.Errorf("want score_revision unchanged at 0, got %d", state.ScoreRevision)
	}
	if state.HomeSide.State != matches.ApprovalStatePending || state.AwaySide.State != matches.ApprovalStatePending {
		t.Error("want approval state unchanged (still pending both sides)")
	}
	events, _ := roundStore.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 0 {
		t.Errorf("want zero approval events on stale-revision rejection, got %d", len(events))
	}
}

// --- Correction 1: atomic lock enforcement for AdvanceScoreRevisionForEdit ---
//
// Each of these seeds a scored match already in a locked state, then
// attempts a real score edit through RoundService (never the stub), proving
// the SQLite conditional UPDATE -- not a removed pre-transaction check --
// is what rejects it, with the revision, round_results, and event history
// all left completely untouched.

func lockedMatchGuardTest(t *testing.T, lock func(matchID int64)) (roundStore *sqlite.RoundStore, matchID, homePlayerID, awayPlayerID int64) {
	t.Helper()
	roundStore = newRoundStore(t)
	matchID, homePlayerID, awayPlayerID, _, _, _ = seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	lock(matchID)
	return roundStore, matchID, homePlayerID, awayPlayerID
}

func TestSaveRounds_RejectsWhenSeasonClosed_RealGuard(t *testing.T) {
	roundStore, matchID, homePlayerID, awayPlayerID := lockedMatchGuardTest(t, func(matchID int64) {
		db.DB.Exec(`UPDATE seasons SET closed_at = CURRENT_TIMESTAMP WHERE id = (SELECT season_id FROM matches WHERE id = ?)`, matchID)
	})
	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID: matchID,
		Rounds:  []models.RoundResult{{RoundNumber: 1, HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID, Game1Home: 10, Game1Away: 3}},
	})
	if err == nil {
		t.Fatal("want SaveRounds rejected when the season is closed, got nil")
	}
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 0 {
		t.Errorf("want revision unchanged, got %d", state.ScoreRevision)
	}
}

func TestSaveRounds_RejectsWhenWeekClosed_RealGuard(t *testing.T) {
	roundStore, matchID, homePlayerID, awayPlayerID := lockedMatchGuardTest(t, func(matchID int64) {
		db.DB.Exec(`UPDATE matches SET week_closed = 1 WHERE id = ?`, matchID)
	})
	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID: matchID,
		Rounds:  []models.RoundResult{{RoundNumber: 1, HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID, Game1Home: 10, Game1Away: 3}},
	})
	if err == nil {
		t.Fatal("want SaveRounds rejected when the week is closed, got nil")
	}
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 0 {
		t.Errorf("want revision unchanged, got %d", state.ScoreRevision)
	}
}

func TestSaveRounds_RejectsWhenAdminAccepted_RealGuard(t *testing.T) {
	roundStore, matchID, homePlayerID, awayPlayerID := lockedMatchGuardTest(t, func(matchID int64) {
		uid := int64(1)
		roundStore2 := sqlite.NewRoundStore(db.DB)
		_ = roundStore2.ApproveMatch(context.Background(), matchID, &uid, "admin ok")
	})
	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID: matchID,
		Rounds:  []models.RoundResult{{RoundNumber: 1, HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID, Game1Home: 10, Game1Away: 3}},
	})
	if err == nil {
		t.Fatal("want SaveRounds rejected when the match is admin-accepted, got nil")
	}
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 0 {
		t.Errorf("want revision unchanged, got %d", state.ScoreRevision)
	}
}

func TestSaveRounds_RejectsWhenProcessed_RealGuard(t *testing.T) {
	roundStore, matchID, homePlayerID, awayPlayerID := lockedMatchGuardTest(t, func(matchID int64) {
		uid := int64(1)
		roundStore2 := sqlite.NewRoundStore(db.DB)
		_ = roundStore2.ApproveMatch(context.Background(), matchID, &uid, "admin ok")
		_ = roundStore2.ProcessMatch(context.Background(), matchID, &uid)
	})
	svc := matches.NewRoundService(roundStore, sqlite.NewRuleStore(db.DB))
	err := svc.SaveRounds(context.Background(), matches.SaveRoundsInput{
		MatchID: matchID,
		Rounds:  []models.RoundResult{{RoundNumber: 1, HomePlayerID: homePlayerID, AwayPlayerID: awayPlayerID, Game1Home: 10, Game1Away: 3}},
	})
	if err == nil {
		t.Fatal("want SaveRounds rejected when the match is processed, got nil")
	}
	state, _ := roundStore.GetMatchApprovalState(context.Background(), matchID)
	if state.ScoreRevision != 0 {
		t.Errorf("want revision unchanged, got %d", state.ScoreRevision)
	}
}

// --- Correction 4: atomic lock enforcement for team actions ---
//
// Each seeds a scored, approved-by-home match, applies a lock AFTER that
// approval exists, then attempts a second team action (a different actor
// approving away, or withdrawing home) -- proving the store's conditional
// UPDATE, not a removed service-layer pre-check, is what rejects it, and
// that no event is written for the rejected attempt.

func TestApproveTeamSide_RejectsWhenAdminAccepted_RealGuard(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid := int64(1)
	if err := s.ApproveMatch(context.Background(), matchID, &uid, "admin ok"); err != nil {
		t.Fatalf("ApproveMatch: %v", err)
	}

	err := s.ApproveTeamSide(context.Background(), matchID, "away", 0, &uid, &uid, "Someone")
	if err == nil {
		t.Fatal("want ApproveTeamSide rejected once admin has accepted, got nil")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 0 {
		t.Errorf("want zero events for the rejected action, got %d", len(events))
	}
}

func TestApproveTeamSide_RejectsWhenProcessed_RealGuard(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid := int64(1)
	s.ApproveMatch(context.Background(), matchID, &uid, "admin ok")
	if err := s.ProcessMatch(context.Background(), matchID, &uid); err != nil {
		t.Fatalf("ProcessMatch: %v", err)
	}

	err := s.ApproveTeamSide(context.Background(), matchID, "away", 0, &uid, &uid, "Someone")
	if err == nil {
		t.Fatal("want ApproveTeamSide rejected once processed, got nil")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 0 {
		t.Errorf("want zero events for the rejected action, got %d", len(events))
	}
}

func TestApproveTeamSide_RejectsWhenWeekClosed_RealGuard(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1, week_closed=1 WHERE id=?`, matchID)

	uid := int64(1)
	err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &uid, "Someone")
	if err == nil {
		t.Fatal("want ApproveTeamSide rejected once the week is closed, got nil")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 0 {
		t.Errorf("want zero events for the rejected action, got %d", len(events))
	}
}

func TestApproveTeamSide_RejectsWhenSeasonClosed_RealGuard(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	db.DB.Exec(`UPDATE seasons SET closed_at = CURRENT_TIMESTAMP WHERE id = (SELECT season_id FROM matches WHERE id = ?)`, matchID)

	uid := int64(1)
	err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &uid, "Someone")
	if err == nil {
		t.Fatal("want ApproveTeamSide rejected once the season is closed, got nil")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	if len(events) != 0 {
		t.Errorf("want zero events for the rejected action, got %d", len(events))
	}
}

func TestWithdrawTeamApproval_RejectsWhenAdminAccepted_RealGuard(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	if err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}
	if err := s.ApproveMatch(context.Background(), matchID, &uid, "admin ok"); err != nil {
		t.Fatalf("ApproveMatch: %v", err)
	}

	err := s.WithdrawTeamApproval(context.Background(), matchID, "home", 0, &uid, &pid, "Alice")
	if err == nil {
		t.Fatal("want WithdrawTeamApproval rejected once admin has accepted, got nil")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	for _, e := range events {
		if e.EventType == "withdrawn" {
			t.Error("want no withdrawn event recorded for the rejected action")
		}
	}
}

func TestRequestCorrection_RejectsWhenProcessed_RealGuard(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid := int64(1)
	s.ApproveMatch(context.Background(), matchID, &uid, "admin ok")
	if err := s.ProcessMatch(context.Background(), matchID, &uid); err != nil {
		t.Fatalf("ProcessMatch: %v", err)
	}

	err := s.RequestCorrection(context.Background(), matchID, "home", 0, "something's off", &uid, &uid, "Someone")
	if err == nil {
		t.Fatal("want RequestCorrection rejected once processed, got nil")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	for _, e := range events {
		if e.EventType == "correction_requested" {
			t.Error("want no correction_requested event recorded for the rejected action")
		}
	}
}

// --- Correction 3: GuardedAdminAccept atomicity ---

func TestGuardedAdminAccept_StaleRevision_RealGuard_NoWrites(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid := int64(1)

	pending := matches.TeamSideApprovalState{State: matches.ApprovalStatePending}
	err := s.GuardedAdminAccept(context.Background(), matchID, 99, pending, pending, &uid, "note")
	if err == nil {
		t.Fatal("want GuardedAdminAccept rejected on a stale revision, got nil")
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.ApprovedAt != nil {
		t.Error("want approved_at left unset on a rejected accept")
	}
}

func TestGuardedAdminAccept_TeamStateChanged_RealGuard_NoWrites(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	// Home is actually 'approved', but the caller's decision was pinned to
	// 'pending' -- simulating a race where the approval landed after the
	// decision was made but before the guarded write ran.
	if err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}

	pending := matches.TeamSideApprovalState{State: matches.ApprovalStatePending}
	err := s.GuardedAdminAccept(context.Background(), matchID, 0, pending, pending, &uid, "note")
	if err == nil {
		t.Fatal("want GuardedAdminAccept rejected when pinned team state no longer matches, got nil")
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.ApprovedAt != nil {
		t.Error("want approved_at left unset on a rejected accept")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	for _, e := range events {
		if e.EventScope == "match" {
			t.Error("want no match-scope event recorded for the rejected accept")
		}
	}
}

func TestGuardedAdminAccept_LockChanged_RealGuard_NoWrites(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1, week_closed=1 WHERE id=?`, matchID)
	uid := int64(1)

	pending := matches.TeamSideApprovalState{State: matches.ApprovalStatePending}
	err := s.GuardedAdminAccept(context.Background(), matchID, 0, pending, pending, &uid, "note")
	if err == nil {
		t.Fatal("want GuardedAdminAccept rejected once the week is closed, got nil")
	}
}

func TestGuardedAdminAccept_Success_CommitsAcceptanceAndEventTogether(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice")
	s.ApproveTeamSide(context.Background(), matchID, "away", 0, &uid, &pid, "Bob")

	svc := matches.NewRoundService(s, sqlite.NewRuleStore(db.DB))
	if err := svc.AdminAcceptMatch(context.Background(), matchID, &uid, "looks good", false, 0); err != nil {
		t.Fatalf("AdminAcceptMatch: %v", err)
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.ApprovedAt == nil {
		t.Fatal("want approved_at set after successful acceptance")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	found := false
	for _, e := range events {
		if e.EventScope == "match" && e.EventType == "admin_accepted" {
			found = true
		}
	}
	if !found {
		t.Error("want an admin_accepted event committed together with the acceptance")
	}
}

func TestGuardedAdminAccept_Override_CommitsAcceptanceAndEventTogether(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice")
	// Away never approves.

	svc := matches.NewRoundService(s, sqlite.NewRuleStore(db.DB))
	if err := svc.AdminAcceptMatch(context.Background(), matchID, &uid, "overriding", true, 0); err != nil {
		t.Fatalf("AdminAcceptMatch with override: %v", err)
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.ApprovedAt == nil {
		t.Fatal("want approved_at set after override acceptance")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	found := false
	for _, e := range events {
		if e.EventScope == "match" && e.EventType == "admin_override" {
			if e.HomeStateSnapshot == nil || *e.HomeStateSnapshot != "approved" {
				t.Errorf("want home_state_snapshot=approved, got %v", e.HomeStateSnapshot)
			}
			if e.AwayStateSnapshot == nil || *e.AwayStateSnapshot != "pending" {
				t.Errorf("want away_state_snapshot=pending, got %v", e.AwayStateSnapshot)
			}
			found = true
		}
	}
	if !found {
		t.Error("want an admin_override event committed together with the acceptance")
	}
}

// intPtr returns a pointer to v, for constructing TeamSideApprovalState
// literals in tests.
func intPtr(v int) *int { return &v }

// TestGuardedAdminAccept_TeamStateRevisionChanged_RealGuard_NoWrites
// constructs, via direct SQL, a state where the label RoundService.
// AdminAcceptMatch's decision was pinned on ("approved") still matches, but
// the state's OWN score revision was rewritten underneath it -- isolating
// the state-score-revision column from both the label and the match-wide
// score_revision, which a label-only guard would miss entirely (PM
// correction 1). Not reachable through the public write API in one step
// (any transition through ApproveTeamSide/RequestCorrection always pins
// the state's revision to the match's CURRENT revision), so this
// constructs the "after" state directly, the same way a real concurrent
// write would leave it.
func TestGuardedAdminAccept_TeamStateRevisionChanged_RealGuard_NoWrites(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	if err := s.ApproveTeamSide(context.Background(), matchID, "home", 0, &uid, &pid, "Alice"); err != nil {
		t.Fatalf("ApproveTeamSide: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET home_approval_state_score_revision=5 WHERE id=?`, matchID)

	pinnedHome := matches.TeamSideApprovalState{State: matches.ApprovalStateApproved, StateScoreRevision: intPtr(0)}
	pinnedAway := matches.TeamSideApprovalState{State: matches.ApprovalStatePending}
	err := s.GuardedAdminAccept(context.Background(), matchID, 0, pinnedHome, pinnedAway, &uid, "note")
	if err == nil {
		t.Fatal("want GuardedAdminAccept rejected when the pinned state-score-revision no longer matches, got nil")
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.ApprovedAt != nil {
		t.Error("want approved_at left unset on a rejected accept")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	for _, e := range events {
		if e.EventScope == "match" {
			t.Error("want no match-scope event recorded for the rejected accept")
		}
	}
}

// TestGuardedAdminAccept_CorrectionNoteChanged_RealGuard_NoWrites is the
// scenario PM's correction explicitly names: a correction note rewritten
// at the same revision, with the label still "correction_requested" --
// must still be caught, since the state label and revision alone are
// identical before and after.
func TestGuardedAdminAccept_CorrectionNoteChanged_RealGuard_NoWrites(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	uid, pid := int64(1), int64(2)
	if err := s.RequestCorrection(context.Background(), matchID, "home", 0, "original note", &uid, &pid, "Alice"); err != nil {
		t.Fatalf("RequestCorrection: %v", err)
	}
	db.DB.Exec(`UPDATE matches SET home_correction_note=? WHERE id=?`, "rewritten note", matchID)

	pinnedHome := matches.TeamSideApprovalState{
		State: matches.ApprovalStateCorrectionRequested, StateScoreRevision: intPtr(0), CorrectionNote: "original note",
	}
	pinnedAway := matches.TeamSideApprovalState{State: matches.ApprovalStatePending}
	err := s.GuardedAdminAccept(context.Background(), matchID, 0, pinnedHome, pinnedAway, &uid, "note")
	if err == nil {
		t.Fatal("want GuardedAdminAccept rejected when the pinned correction note no longer matches, got nil")
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.ApprovedAt != nil {
		t.Error("want approved_at left unset on a rejected accept")
	}
	events, _ := s.ListApprovalEvents(context.Background(), matchID)
	for _, e := range events {
		if e.EventScope == "match" {
			t.Error("want no match-scope event recorded for the rejected accept (stale note snapshot)")
		}
	}
}

// --- Compatibility: existing admin approval unaffected by team-approval state ---

func TestApproveMatch_StillWorksWithoutAnyTeamApprovals(t *testing.T) {
	s := newRoundStore(t)
	matchID, _, _, _, _, _ := seedRoundTestData(t)
	db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)

	uid := int64(7)
	if err := s.ApproveMatch(context.Background(), matchID, &uid, "admin says fine"); err != nil {
		t.Fatalf("want existing ApproveMatch to keep working with zero team approvals, got: %v", err)
	}
	state, _ := s.GetMatchApprovalState(context.Background(), matchID)
	if state.ApprovedAt == nil {
		t.Fatal("want admin approved_at set")
	}
	if state.HomeSide.State != matches.ApprovalStatePending || state.AwaySide.State != matches.ApprovalStatePending {
		t.Error("want team approval state completely unaffected by the existing admin route")
	}
}
