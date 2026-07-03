package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/seasons"
	"league_app/db"
)

func sseedBye(t *testing.T, seasonID, teamID int64, weekNum int, approved bool) int64 {
	t.Helper()
	app := 0
	if approved {
		app = 1
	}
	res, err := db.DB.Exec(
		`INSERT INTO bye_requests (season_id, team_id, week_number, reason, approved) VALUES (?,?,?,'',?)`,
		seasonID, teamID, weekNum, app)
	if err != nil {
		t.Fatalf("insert bye_request: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ── CountParticipatingTeams ───────────────────────────────────────────────────

func TestSeasonStore_CountParticipatingTeams_ManagedUsesSeasonTeams(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	t3 := sseedTeam(t, lid, "C")
	sseedSeasonTeam(t, sid, t1, nil)
	sseedSeasonTeam(t, sid, t2, nil)
	sseedSeasonTeam(t, sid, t3, nil)

	n, err := store.CountParticipatingTeams(ctx, sid, lid, true)
	if err != nil {
		t.Fatalf("CountParticipatingTeams: %v", err)
	}
	if n != 3 {
		t.Errorf("want 3, got %d", n)
	}
}

func TestSeasonStore_CountParticipatingTeams_LegacyFallsBackToLeague(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	sseedTeam(t, lid, "A")
	sseedTeam(t, lid, "B")
	// No season_teams rows — falls back to league teams

	n, err := store.CountParticipatingTeams(ctx, sid, lid, false)
	if err != nil {
		t.Fatalf("CountParticipatingTeams: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 from league fallback, got %d", n)
	}
}

// ── CheckTeamInSeason ─────────────────────────────────────────────────────────

func TestSeasonStore_CheckTeamInSeason_TrueWhenRegistered(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Alpha")
	sseedSeasonTeam(t, sid, tid, nil)

	ok, err := store.CheckTeamInSeason(ctx, sid, tid)
	if err != nil {
		t.Fatalf("CheckTeamInSeason: %v", err)
	}
	if !ok {
		t.Error("want true when team is in season")
	}
}

func TestSeasonStore_CheckTeamInSeason_FalseWhenAbsent(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Beta")

	ok, err := store.CheckTeamInSeason(ctx, sid, tid)
	if err != nil {
		t.Fatalf("CheckTeamInSeason: %v", err)
	}
	if ok {
		t.Error("want false when team is not in season")
	}
}

// ── HasDuplicateBye ───────────────────────────────────────────────────────────

func TestSeasonStore_HasDuplicateBye_TrueWhenExists(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	tid := sseedTeam(t, lid, "A")
	sseedBye(t, sid, tid, 3, false)

	dup, err := store.HasDuplicateBye(ctx, sid, tid, 3)
	if err != nil {
		t.Fatalf("HasDuplicateBye: %v", err)
	}
	if !dup {
		t.Error("want true for existing bye")
	}
}

func TestSeasonStore_HasDuplicateBye_FalseWhenAbsent(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	tid := sseedTeam(t, lid, "B")

	dup, err := store.HasDuplicateBye(ctx, sid, tid, 4)
	if err != nil {
		t.Fatalf("HasDuplicateBye: %v", err)
	}
	if dup {
		t.Error("want false when no bye exists")
	}
}

// ── InsertByeRequest ──────────────────────────────────────────────────────────

func TestSeasonStore_InsertByeRequest_CreatesAndReturns(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	tid := sseedTeam(t, lid, "Gamma")

	b, err := store.InsertByeRequest(ctx, sid, tid, 5, "vacation")
	if err != nil {
		t.Fatalf("InsertByeRequest: %v", err)
	}
	if b.ID == 0 {
		t.Error("want non-zero ID")
	}
	if b.WeekNumber != 5 {
		t.Errorf("want WeekNumber=5, got %d", b.WeekNumber)
	}
	if b.TeamName == "" {
		t.Error("want TeamName populated from JOIN")
	}
	if b.Approved {
		t.Error("want Approved=false on insert")
	}
}

// ── GetByeRequest ─────────────────────────────────────────────────────────────

func TestSeasonStore_GetByeRequest_ReturnsRecord(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	tid := sseedTeam(t, lid, "Delta")
	bid := sseedBye(t, sid, tid, 2, false)

	b, err := store.GetByeRequest(ctx, sid, bid)
	if err != nil {
		t.Fatalf("GetByeRequest: %v", err)
	}
	if b.ID != bid {
		t.Errorf("want id=%d, got %d", bid, b.ID)
	}
	if b.WeekNumber != 2 {
		t.Errorf("want WeekNumber=2, got %d", b.WeekNumber)
	}
}

func TestSeasonStore_GetByeRequest_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	_, err := store.GetByeRequest(ctx, sid, 9999)
	if !errors.Is(err, seasons.ErrByeNotFound) {
		t.Errorf("want ErrByeNotFound, got %v", err)
	}
}

func TestSeasonStore_GetByeRequest_WrongSeason_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid1 := sseedSeason(t, lid, "S1", "", "", false)
	sid2 := sseedSeason(t, lid, "S2", "", "", false)
	tid := sseedTeam(t, lid, "Epsilon")
	bid := sseedBye(t, sid1, tid, 1, false)

	_, err := store.GetByeRequest(ctx, sid2, bid) // wrong season
	if !errors.Is(err, seasons.ErrByeNotFound) {
		t.Errorf("want ErrByeNotFound for wrong season, got %v", err)
	}
}

// ── HasByeConflict ────────────────────────────────────────────────────────────

func TestSeasonStore_HasByeConflict_TrueWhenOtherApproved(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	t1 := sseedTeam(t, lid, "A")
	t2 := sseedTeam(t, lid, "B")
	bid1 := sseedBye(t, sid, t1, 3, true)  // approved
	bid2 := sseedBye(t, sid, t2, 3, false) // the one being approved

	conflict, err := store.HasByeConflict(ctx, sid, 3, bid2)
	if err != nil {
		t.Fatalf("HasByeConflict: %v", err)
	}
	if !conflict {
		t.Errorf("want conflict when bid1=%d is approved for same week", bid1)
	}
}

func TestSeasonStore_HasByeConflict_FalseForSameRecord(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	tid := sseedTeam(t, lid, "C")
	bid := sseedBye(t, sid, tid, 4, true) // same record excluded

	conflict, err := store.HasByeConflict(ctx, sid, 4, bid)
	if err != nil {
		t.Fatalf("HasByeConflict: %v", err)
	}
	if conflict {
		t.Error("want no conflict when only the same record is approved")
	}
}

// ── SetByeApproval ────────────────────────────────────────────────────────────

func TestSeasonStore_SetByeApproval_ApprovesAndReturns(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)
	tid := sseedTeam(t, lid, "Zeta")
	bid := sseedBye(t, sid, tid, 2, false)

	b, err := store.SetByeApproval(ctx, sid, bid, true)
	if err != nil {
		t.Fatalf("SetByeApproval: %v", err)
	}
	if !b.Approved {
		t.Error("want Approved=true")
	}
}

func TestSeasonStore_SetByeApproval_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false)

	_, err := store.SetByeApproval(ctx, sid, 9999, true)
	if !errors.Is(err, seasons.ErrByeNotFound) {
		t.Errorf("want ErrByeNotFound, got %v", err)
	}
}
