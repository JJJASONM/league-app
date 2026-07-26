package sqlite_test

import (
	"context"
	"testing"

	"league_app/db"
)

// sseedSeasonActive inserts a season with activated_at set and active=1.
func sseedSeasonActive(t *testing.T, leagueID int64, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(
		`INSERT INTO seasons (league_id, name, schedule_type, teams_managed, active, activated_at)
		 VALUES (?,?,'single_rr',1,1,'2025-01-01T00:00:00Z')`,
		leagueID, name)
	if err != nil {
		t.Fatalf("insert active season %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// sseedSeasonHistorical inserts a season with activated_at set and active=0 (not yet closed).
func sseedSeasonHistorical(t *testing.T, leagueID int64, name string) int64 {
	t.Helper()
	res, err := db.DB.Exec(
		`INSERT INTO seasons (league_id, name, schedule_type, teams_managed, active, activated_at)
		 VALUES (?,?,'single_rr',1,0,'2025-01-01T00:00:00Z')`,
		leagueID, name)
	if err != nil {
		t.Fatalf("insert historical season %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// sseedMatchCompleted inserts a match with a given completed flag.
func sseedMatchCompleted(t *testing.T, seasonID, homeID, awayID int64, completed int) {
	t.Helper()
	_, err := db.DB.Exec(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed)
		 VALUES (?,?,?,1,?)`,
		seasonID, homeID, awayID, completed)
	if err != nil {
		t.Fatalf("insert match (completed=%d): %v", completed, err)
	}
}

// ── GetSeasonMissingCount ─────────────────────────────────────────────────────

func TestSeasonCloseStore_GetSeasonMissingCount_NoMatches(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)

	count, err := store.GetSeasonMissingCount(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeasonMissingCount: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0, got %d", count)
	}
}

func TestSeasonCloseStore_GetSeasonMissingCount_CountsIncompleteOnly(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	t1 := sseedTeam(t, lid, "Home")
	t2 := sseedTeam(t, lid, "Away")

	sseedMatchCompleted(t, sid, t1, t2, 0) // incomplete
	sseedMatchCompleted(t, sid, t1, t2, 0) // incomplete
	sseedMatchCompleted(t, sid, t1, t2, 1) // complete

	count, err := store.GetSeasonMissingCount(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeasonMissingCount: %v", err)
	}
	if count != 2 {
		t.Errorf("want 2 incomplete matches, got %d", count)
	}
}

func TestSeasonCloseStore_GetSeasonMissingCount_AllComplete(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "S", "", "", true)
	t1 := sseedTeam(t, lid, "Home")
	t2 := sseedTeam(t, lid, "Away")

	sseedMatchCompleted(t, sid, t1, t2, 1)
	sseedMatchCompleted(t, sid, t1, t2, 1)

	count, err := store.GetSeasonMissingCount(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeasonMissingCount: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0, got %d", count)
	}
}

// ── CloseSeasonRecord ─────────────────────────────────────────────────────────

func TestSeasonCloseStore_CloseSeasonRecord_SetsClosedAt(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeasonHistorical(t, lid, "H1")

	if err := store.CloseSeasonRecord(ctx, sid, `{"schema_version":1}`, false); err != nil {
		t.Fatalf("CloseSeasonRecord: %v", err)
	}

	season, err := store.GetSeason(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeason after close: %v", err)
	}
	if season.ClosedAt == nil || *season.ClosedAt == "" {
		t.Error("expected closed_at to be set")
	}
}

func TestSeasonCloseStore_CloseSeasonRecord_ActiveSeason_SetsInactive(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeasonActive(t, lid, "A1")

	if err := store.CloseSeasonRecord(ctx, sid, `{"schema_version":1}`, true); err != nil {
		t.Fatalf("CloseSeasonRecord: %v", err)
	}

	season, err := store.GetSeason(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeason after close: %v", err)
	}
	if season.ClosedAt == nil || *season.ClosedAt == "" {
		t.Error("expected closed_at to be set")
	}
	if season.Active {
		t.Error("expected active=false after close with setInactive=true")
	}
}

func TestSeasonCloseStore_CloseSeasonRecord_HistoricalSeason_ActiveUnchanged(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeasonHistorical(t, lid, "H2")

	if err := store.CloseSeasonRecord(ctx, sid, `{"schema_version":1}`, false); err != nil {
		t.Fatalf("CloseSeasonRecord: %v", err)
	}

	season, err := store.GetSeason(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeason after close: %v", err)
	}
	if season.ClosedAt == nil || *season.ClosedAt == "" {
		t.Error("expected closed_at to be set")
	}
	// Historical season was already active=0; it must remain so.
	if season.Active {
		t.Error("expected active=false to be preserved after close")
	}
}
