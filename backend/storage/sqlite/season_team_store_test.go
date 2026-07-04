package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/seasons"
	"league_app/db"
)

// ── GetTeamLeagueID ───────────────────────────────────────────────────────────

func TestSeasonStore_GetTeamLeagueID_ReturnsLeague(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	tid := sseedTeam(t, lid, "Alpha")

	got, err := store.GetTeamLeagueID(ctx, tid)
	if err != nil {
		t.Fatalf("GetTeamLeagueID: %v", err)
	}
	if got != lid {
		t.Errorf("want league_id=%d, got %d", lid, got)
	}
}

func TestSeasonStore_GetTeamLeagueID_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	_, err := store.GetTeamLeagueID(context.Background(), 9999)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── GetSeasonTeam ─────────────────────────────────────────────────────────────

func TestSeasonStore_GetSeasonTeam_ReturnsRecord(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Bravo")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, &pid)
	sseedRoster(t, sid, tid, pid)

	st, err := store.GetSeasonTeam(ctx, sid, tid)
	if err != nil {
		t.Fatalf("GetSeasonTeam: %v", err)
	}
	if st.TeamID != tid {
		t.Errorf("want TeamID=%d, got %d", tid, st.TeamID)
	}
	if st.RosterCount != 1 {
		t.Errorf("want RosterCount=1, got %d", st.RosterCount)
	}
}

func TestSeasonStore_GetSeasonTeam_NotInSeason(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Charlie")

	_, err := store.GetSeasonTeam(ctx, sid, tid)
	if !errors.Is(err, seasons.ErrTeamNotInSeason) {
		t.Errorf("want ErrTeamNotInSeason, got %v", err)
	}
}

// ── AddSeasonTeamCopy ─────────────────────────────────────────────────────────

func TestSeasonStore_AddSeasonTeamCopy_CopiesFromPriorSeason(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	prevSID := sseedSeason(t, lid, "Prev", "2025-01-01", "2025-12-01", true)
	currSID := sseedSeason(t, lid, "Curr", "2026-01-01", "", true)
	tid := sseedTeam(t, lid, "Delta")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, prevSID, tid, nil)
	sseedRoster(t, prevSID, tid, pid)

	if err := store.AddSeasonTeamCopy(ctx, currSID, tid, prevSID, true); err != nil {
		t.Fatalf("AddSeasonTeamCopy: %v", err)
	}

	// Team registered in current season
	st, err := store.GetSeasonTeam(ctx, currSID, tid)
	if err != nil {
		t.Fatalf("GetSeasonTeam after copy: %v", err)
	}
	if st.TeamID != tid {
		t.Errorf("want TeamID=%d, got %d", tid, st.TeamID)
	}
	// Roster copied
	if st.RosterCount != 1 {
		t.Errorf("want RosterCount=1, got %d", st.RosterCount)
	}
}

func TestSeasonStore_AddSeasonTeamCopy_DuplicateReturnsErr(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Echo")
	sseedSeasonTeam(t, sid, tid, nil)

	err := store.AddSeasonTeamCopy(ctx, sid, tid, 0, true)
	if !errors.Is(err, seasons.ErrTeamAlreadyInSeason) {
		t.Errorf("want ErrTeamAlreadyInSeason, got %v", err)
	}
}

func TestSeasonStore_AddSeasonTeamCopy_TeamNotInPriorManagedSeason(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	prevSID := sseedSeason(t, lid, "Prev", "2025-01-01", "2025-12-01", true) // managed
	currSID := sseedSeason(t, lid, "Curr", "2026-01-01", "", true)
	tid := sseedTeam(t, lid, "Foxtrot")
	// team NOT in prevSID season_teams

	err := store.AddSeasonTeamCopy(ctx, currSID, tid, prevSID, true)
	if !errors.Is(err, seasons.ErrTeamNotInPriorSeason) {
		t.Errorf("want ErrTeamNotInPriorSeason, got %v", err)
	}
}

func TestSeasonStore_AddSeasonTeamCopy_LegacyFallbackCopiesActivePlayers(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", false) // not managed
	tid := sseedTeam(t, lid, "Golf")
	sseedPlayer(t, tid)

	if err := store.AddSeasonTeamCopy(ctx, sid, tid, 0, false); err != nil {
		t.Fatalf("AddSeasonTeamCopy legacy: %v", err)
	}
	st, err := store.GetSeasonTeam(ctx, sid, tid)
	if err != nil {
		t.Fatalf("GetSeasonTeam: %v", err)
	}
	if st.RosterCount != 1 {
		t.Errorf("want RosterCount=1 from legacy fallback, got %d", st.RosterCount)
	}
}

// ── AddSeasonTeamNew ──────────────────────────────────────────────────────────

func TestSeasonStore_AddSeasonTeamNew_CreatesAndRegisters(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	teamID, err := store.AddSeasonTeamNew(ctx, sid, lid, "Hotel")
	if err != nil {
		t.Fatalf("AddSeasonTeamNew: %v", err)
	}
	if teamID == 0 {
		t.Error("want non-zero teamID")
	}

	st, err := store.GetSeasonTeam(ctx, sid, teamID)
	if err != nil {
		t.Fatalf("GetSeasonTeam: %v", err)
	}
	if st.SeasonName != "Hotel" {
		t.Errorf("want SeasonName=Hotel, got %q", st.SeasonName)
	}
}

// ── CheckPlayerOnSeasonRoster ─────────────────────────────────────────────────

func TestSeasonStore_CheckPlayerOnSeasonRoster_TrueWhenPresent(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "India")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	ok, err := store.CheckPlayerOnSeasonRoster(ctx, sid, tid, pid)
	if err != nil {
		t.Fatalf("CheckPlayerOnSeasonRoster: %v", err)
	}
	if !ok {
		t.Error("want true when player is on roster")
	}
}

func TestSeasonStore_CheckPlayerOnSeasonRoster_FalseWhenAbsent(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Juliet")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)

	ok, err := store.CheckPlayerOnSeasonRoster(ctx, sid, tid, pid)
	if err != nil {
		t.Fatalf("CheckPlayerOnSeasonRoster: %v", err)
	}
	if ok {
		t.Error("want false when player not on roster")
	}
}

// ── UpdateSeasonTeamMeta ──────────────────────────────────────────────────────

func TestSeasonStore_UpdateSeasonTeamMeta_UpdatesNameAndCaptain(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Kilo")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	if err := store.UpdateSeasonTeamMeta(ctx, sid, tid, "Kilo Updated", &pid); err != nil {
		t.Fatalf("UpdateSeasonTeamMeta: %v", err)
	}
	st, err := store.GetSeasonTeam(ctx, sid, tid)
	if err != nil {
		t.Fatalf("GetSeasonTeam: %v", err)
	}
	if st.SeasonName != "Kilo Updated" {
		t.Errorf("want SeasonName=Kilo Updated, got %q", st.SeasonName)
	}
	if st.CaptainID == nil || *st.CaptainID != pid {
		t.Errorf("want CaptainID=%d, got %v", pid, st.CaptainID)
	}
}

func TestSeasonStore_UpdateSeasonTeamMeta_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	err := store.UpdateSeasonTeamMeta(ctx, sid, 9999, "X", nil)
	if !errors.Is(err, seasons.ErrTeamNotInSeason) {
		t.Errorf("want ErrTeamNotInSeason, got %v", err)
	}
}

// ── RemoveSeasonTeam ──────────────────────────────────────────────────────────

func TestSeasonStore_RemoveSeasonTeam_RemovesFromSeason(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Lima")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	if err := store.RemoveSeasonTeam(ctx, sid, tid); err != nil {
		t.Fatalf("RemoveSeasonTeam: %v", err)
	}

	// Team no longer in season
	_, err := store.GetSeasonTeam(ctx, sid, tid)
	if !errors.Is(err, seasons.ErrTeamNotInSeason) {
		t.Errorf("want ErrTeamNotInSeason after removal, got %v", err)
	}

	// Team + players cleaned up (no match history, no other seasons)
	var teamCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM teams WHERE id=?`, tid).Scan(&teamCount)
	if teamCount != 0 {
		t.Error("want team record deleted when no match history")
	}
}

func TestSeasonStore_RemoveSeasonTeam_PreservesTeamWithMatchHistory(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	t1 := sseedTeam(t, lid, "Mike")
	t2 := sseedTeam(t, lid, "November")
	sseedSeasonTeam(t, sid, t1, nil)
	sseedMatch(t, sid, t1, t2, true)

	if err := store.RemoveSeasonTeam(ctx, sid, t1); err != nil {
		t.Fatalf("RemoveSeasonTeam: %v", err)
	}

	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM teams WHERE id=?`, t1).Scan(&count)
	if count != 1 {
		t.Error("want team preserved when it has match history")
	}
}

func TestSeasonStore_RemoveSeasonTeam_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	err := store.RemoveSeasonTeam(ctx, sid, 9999)
	if !errors.Is(err, seasons.ErrTeamNotInSeason) {
		t.Errorf("want ErrTeamNotInSeason, got %v", err)
	}
}

// ── ListSeasonTeams ───────────────────────────────────────────────────────────

func TestSeasonStore_ListSeasonTeams_ReturnsTeams(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid1 := sseedTeam(t, lid, "Alpha")
	tid2 := sseedTeam(t, lid, "Bravo")
	sseedSeasonTeam(t, sid, tid1, nil)
	sseedSeasonTeam(t, sid, tid2, nil)

	teams, err := store.ListSeasonTeams(ctx, sid)
	if err != nil {
		t.Fatalf("ListSeasonTeams: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("want 2 teams, got %d", len(teams))
	}
	if teams[0].TeamID != tid1 {
		t.Errorf("want first team=%d, got %d", tid1, teams[0].TeamID)
	}
}

func TestSeasonStore_ListSeasonTeams_EmptyWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	teams, err := store.ListSeasonTeams(ctx, sid)
	if err != nil {
		t.Fatalf("ListSeasonTeams: %v", err)
	}
	if len(teams) != 0 {
		t.Errorf("want 0 teams, got %d", len(teams))
	}
}

func TestSeasonStore_ListSeasonTeams_IncludesRosterCount(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Charlie")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	teams, err := store.ListSeasonTeams(ctx, sid)
	if err != nil {
		t.Fatalf("ListSeasonTeams: %v", err)
	}
	if len(teams) != 1 || teams[0].RosterCount != 1 {
		t.Errorf("want RosterCount=1, got %+v", teams)
	}
}
