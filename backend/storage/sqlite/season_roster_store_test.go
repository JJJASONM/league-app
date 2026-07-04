package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/seasons"
	"league_app/db"
)

// ── ListRoster ────────────────────────────────────────────────────────────────

func TestSeasonRosterStore_ListRoster_ReturnsEntries(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Alpha")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	entries, err := store.ListRoster(ctx, sid, tid)
	if err != nil {
		t.Fatalf("ListRoster: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].PlayerID != pid {
		t.Errorf("want PlayerID=%d, got %d", pid, entries[0].PlayerID)
	}
	if entries[0].TeamID != tid {
		t.Errorf("want TeamID=%d, got %d", tid, entries[0].TeamID)
	}
}

func TestSeasonRosterStore_ListRoster_EmptyWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Bravo")
	sseedSeasonTeam(t, sid, tid, nil)

	entries, err := store.ListRoster(ctx, sid, tid)
	if err != nil {
		t.Fatalf("ListRoster: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("want 0 entries, got %d", len(entries))
	}
}

// ── GetPlayerRosterTeam ───────────────────────────────────────────────────────

func TestSeasonRosterStore_GetPlayerRosterTeam_FoundWhenRostered(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Charlie")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	gotTeam, found, err := store.GetPlayerRosterTeam(ctx, sid, pid)
	if err != nil {
		t.Fatalf("GetPlayerRosterTeam: %v", err)
	}
	if !found {
		t.Fatal("want found=true, got false")
	}
	if gotTeam != tid {
		t.Errorf("want teamID=%d, got %d", tid, gotTeam)
	}
}

func TestSeasonRosterStore_GetPlayerRosterTeam_NotFoundWhenAbsent(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Delta")
	pid := sseedPlayer(t, tid)

	_, found, err := store.GetPlayerRosterTeam(ctx, sid, pid)
	if err != nil {
		t.Fatalf("GetPlayerRosterTeam: %v", err)
	}
	if found {
		t.Error("want found=false, got true")
	}
}

// ── InsertOrGetRosterPlayer ───────────────────────────────────────────────────

func TestSeasonRosterStore_InsertOrGetRosterPlayer_InsertsAndReturnsEntry(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Echo")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)

	entry, err := store.InsertOrGetRosterPlayer(ctx, sid, tid, pid)
	if err != nil {
		t.Fatalf("InsertOrGetRosterPlayer: %v", err)
	}
	if entry.PlayerID != pid {
		t.Errorf("want PlayerID=%d, got %d", pid, entry.PlayerID)
	}
	if entry.ID == 0 {
		t.Error("want non-zero ID")
	}
}

func TestSeasonRosterStore_InsertOrGetRosterPlayer_IdempotentOnDuplicate(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Foxtrot")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	entry, err := store.InsertOrGetRosterPlayer(ctx, sid, tid, pid)
	if err != nil {
		t.Fatalf("InsertOrGetRosterPlayer (duplicate): %v", err)
	}
	if entry.PlayerID != pid {
		t.Errorf("want PlayerID=%d, got %d", pid, entry.PlayerID)
	}
}

// ── DeleteRosterPlayer ────────────────────────────────────────────────────────

func TestSeasonRosterStore_DeleteRosterPlayer_DeletesEntry(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Golf")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid)

	if err := store.DeleteRosterPlayer(ctx, sid, tid, pid); err != nil {
		t.Fatalf("DeleteRosterPlayer: %v", err)
	}

	var n int
	db.DB.QueryRow(
		`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND team_id=? AND player_id=?`,
		sid, tid, pid).Scan(&n)
	if n != 0 {
		t.Errorf("want 0 rows after delete, got %d", n)
	}
}

func TestSeasonRosterStore_DeleteRosterPlayer_ClearsCaptainID(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Hotel")
	pid := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, &pid)
	sseedRoster(t, sid, tid, pid)

	if err := store.DeleteRosterPlayer(ctx, sid, tid, pid); err != nil {
		t.Fatalf("DeleteRosterPlayer: %v", err)
	}

	var capID *int64
	db.DB.QueryRow(
		`SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`,
		sid, tid).Scan(&capID)
	if capID != nil {
		t.Errorf("want captain_id=NULL after removing captain, got %d", *capID)
	}
}

func TestSeasonRosterStore_DeleteRosterPlayer_NotFoundReturnsError(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "India")
	pid := sseedPlayer(t, tid)

	err := store.DeleteRosterPlayer(ctx, sid, tid, pid)
	if !errors.Is(err, seasons.ErrRosterEntryNotFound) {
		t.Errorf("want ErrRosterEntryNotFound, got %v", err)
	}
}

// ── ListAvailablePlayers ──────────────────────────────────────────────────────

func TestSeasonRosterStore_ListAvailablePlayers_ExcludesRostered(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	tid := sseedTeam(t, lid, "Juliet")
	pid1 := sseedPlayer(t, tid)
	pid2 := sseedPlayer(t, tid)
	sseedSeasonTeam(t, sid, tid, nil)
	sseedRoster(t, sid, tid, pid1)

	players, err := store.ListAvailablePlayers(ctx, sid)
	if err != nil {
		t.Fatalf("ListAvailablePlayers: %v", err)
	}
	for _, p := range players {
		if p.ID == pid1 {
			t.Errorf("rostered player %d should not appear in available list", pid1)
		}
	}
	found := false
	for _, p := range players {
		if p.ID == pid2 {
			found = true
		}
	}
	if !found {
		t.Errorf("unrostered player %d should appear in available list", pid2)
	}
}

func TestSeasonRosterStore_ListAvailablePlayers_SeasonNotFound(t *testing.T) {
	store := newSeasonStore(t)
	_, err := store.ListAvailablePlayers(context.Background(), 9999)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
