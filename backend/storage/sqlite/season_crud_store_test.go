package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"league_app/backend/domains/seasons"
	"league_app/db"
)

// ── ListSeasons ───────────────────────────────────────────────────────────────

func TestSeasonCRUDStore_ListSeasons_ReturnsAll(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sseedSeason(t, lid, "A", "", "", true)
	sseedSeason(t, lid, "B", "", "", true)

	seasons, err := store.ListSeasons(ctx, nil)
	if err != nil {
		t.Fatalf("ListSeasons: %v", err)
	}
	if len(seasons) != 2 {
		t.Errorf("want 2 seasons, got %d", len(seasons))
	}
}

func TestSeasonCRUDStore_ListSeasons_FiltersByLeague(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid1 := sseedLeague(t)
	// Insert second league directly to avoid the fixed 'L' name collision.
	res, err := db.DB.Exec(`INSERT INTO leagues (name, game_format) VALUES ('L2','8ball')`)
	if err != nil {
		t.Fatalf("insert league2: %v", err)
	}
	lid2, _ := res.LastInsertId()
	sseedSeason(t, lid1, "Alpha", "", "", true)
	sseedSeason(t, lid2, "Beta", "", "", true)

	seasons, err := store.ListSeasons(ctx, &lid1)
	if err != nil {
		t.Fatalf("ListSeasons: %v", err)
	}
	if len(seasons) != 1 {
		t.Fatalf("want 1 season for league, got %d", len(seasons))
	}
	if seasons[0].Name != "Alpha" {
		t.Errorf("want Name=Alpha, got %q", seasons[0].Name)
	}
}

func TestSeasonCRUDStore_ListSeasons_EmptyWhenNone(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)

	seasons, err := store.ListSeasons(ctx, &lid)
	if err != nil {
		t.Fatalf("ListSeasons: %v", err)
	}
	if len(seasons) != 0 {
		t.Errorf("want 0 seasons, got %d", len(seasons))
	}
}

// ── GetSeason ─────────────────────────────────────────────────────────────────

func TestSeasonCRUDStore_GetSeason_ReturnsRecord(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "Gamma", "2026-01-01", "", true)

	s, err := store.GetSeason(ctx, sid)
	if err != nil {
		t.Fatalf("GetSeason: %v", err)
	}
	if s.ID != sid {
		t.Errorf("want ID=%d, got %d", sid, s.ID)
	}
	if s.Name != "Gamma" {
		t.Errorf("want Name=Gamma, got %q", s.Name)
	}
	if s.StartDate == nil || *s.StartDate != "2026-01-01" {
		t.Errorf("want StartDate=2026-01-01, got %v", s.StartDate)
	}
	if !s.TeamsManaged {
		t.Error("want TeamsManaged=true")
	}
}

func TestSeasonCRUDStore_GetSeason_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	_, err := store.GetSeason(context.Background(), 9999)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── CreateSeason ──────────────────────────────────────────────────────────────

func TestSeasonCRUDStore_CreateSeason_InsertsRow(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)

	s, err := store.CreateSeason(ctx, seasons.CreateSeasonInput{
		LeagueID:     lid,
		Name:         "Delta",
		ScheduleType: "single_rr",
		NumWeeks:     10,
	})
	if err != nil {
		t.Fatalf("CreateSeason: %v", err)
	}
	if s.ID == 0 {
		t.Error("want non-zero ID")
	}
	if s.Name != "Delta" {
		t.Errorf("want Name=Delta, got %q", s.Name)
	}
	if s.ScheduleType != "single_rr" {
		t.Errorf("want ScheduleType=single_rr, got %q", s.ScheduleType)
	}
	if s.NumWeeks != 10 {
		t.Errorf("want NumWeeks=10, got %d", s.NumWeeks)
	}
}

func TestSeasonCRUDStore_CreateSeason_ForcesTeamsManaged(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)

	s, err := store.CreateSeason(ctx, seasons.CreateSeasonInput{
		LeagueID:     lid,
		Name:         "Epsilon",
		ScheduleType: "double_rr",
	})
	if err != nil {
		t.Fatalf("CreateSeason: %v", err)
	}
	if !s.TeamsManaged {
		t.Error("want TeamsManaged=true on all new seasons")
	}
}

func TestSeasonCRUDStore_CreateSeason_WithStartDate(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	startDate := "2026-09-01"

	s, err := store.CreateSeason(ctx, seasons.CreateSeasonInput{
		LeagueID:     lid,
		Name:         "Zeta",
		StartDate:    &startDate,
		ScheduleType: "double_rr",
	})
	if err != nil {
		t.Fatalf("CreateSeason: %v", err)
	}
	if s.StartDate == nil || *s.StartDate != "2026-09-01" {
		t.Errorf("want StartDate=2026-09-01, got %v", s.StartDate)
	}
}

// ── UpdateSeason ──────────────────────────────────────────────────────────────

func TestSeasonCRUDStore_UpdateSeason_UpdatesFields(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "Eta", "", "", true)
	newStart := "2026-10-01"

	s, err := store.UpdateSeason(ctx, sid, seasons.UpdateSeasonInput{
		Name:         "Eta Updated",
		StartDate:    &newStart,
		ScheduleType: "blanket",
		NumWeeks:     12,
	})
	if err != nil {
		t.Fatalf("UpdateSeason: %v", err)
	}
	if s.Name != "Eta Updated" {
		t.Errorf("want Name=Eta Updated, got %q", s.Name)
	}
	if s.ScheduleType != "blanket" {
		t.Errorf("want ScheduleType=blanket, got %q", s.ScheduleType)
	}
	if s.NumWeeks != 12 {
		t.Errorf("want NumWeeks=12, got %d", s.NumWeeks)
	}
	if s.StartDate == nil || *s.StartDate != "2026-10-01" {
		t.Errorf("want StartDate=2026-10-01, got %v", s.StartDate)
	}
}

func TestSeasonCRUDStore_UpdateSeason_ReturnsFullRow(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "Theta", "", "", true)

	s, err := store.UpdateSeason(ctx, sid, seasons.UpdateSeasonInput{
		Name:         "Theta",
		ScheduleType: "double_rr",
	})
	if err != nil {
		t.Fatalf("UpdateSeason: %v", err)
	}
	if s.LeagueID != lid {
		t.Errorf("want LeagueID=%d in full row, got %d", lid, s.LeagueID)
	}
	if s.CreatedAt.IsZero() {
		t.Error("want non-zero CreatedAt in full row")
	}
	_ = time.Now() // ensure time package used
}

func TestSeasonCRUDStore_UpdateSeason_NotFound(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()

	_, err := store.UpdateSeason(ctx, 9999, seasons.UpdateSeasonInput{
		Name:         "X",
		ScheduleType: "double_rr",
	})
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── DeleteSeason ──────────────────────────────────────────────────────────────

func TestSeasonCRUDStore_DeleteSeason_DeletesRow(t *testing.T) {
	store := newSeasonStore(t)
	ctx := context.Background()
	lid := sseedLeague(t)
	sid := sseedSeason(t, lid, "Iota", "", "", true)

	if err := store.DeleteSeason(ctx, sid); err != nil {
		t.Fatalf("DeleteSeason: %v", err)
	}
	_, err := store.GetSeason(ctx, sid)
	if !errors.Is(err, seasons.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestSeasonCRUDStore_DeleteSeason_MissingRowNoError(t *testing.T) {
	store := newSeasonStore(t)
	if err := store.DeleteSeason(context.Background(), 9999); err != nil {
		t.Errorf("want nil error for non-existent season, got %v", err)
	}
}
