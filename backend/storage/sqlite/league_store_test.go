package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/leagues"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

func newLeagueStore(t *testing.T) *sqlite.LeagueStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewLeagueStore(db.DB)
}

// ── ListLeagues ───────────────────────────────────────────────────────────────

func TestLeagueStore_ListLeagues_ReturnsAll(t *testing.T) {
	store := newLeagueStore(t)
	ctx := context.Background()
	if _, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{Name: "Alpha", GameFormat: "8ball"}); err != nil {
		t.Fatalf("CreateLeague Alpha: %v", err)
	}
	if _, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{Name: "Beta", GameFormat: "9ball"}); err != nil {
		t.Fatalf("CreateLeague Beta: %v", err)
	}

	got, err := store.ListLeagues(ctx)
	if err != nil {
		t.Fatalf("ListLeagues: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 leagues, got %d", len(got))
	}
}

func TestLeagueStore_ListLeagues_EmptyWhenNone(t *testing.T) {
	store := newLeagueStore(t)
	got, err := store.ListLeagues(context.Background())
	if err != nil {
		t.Fatalf("ListLeagues: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 leagues, got %d", len(got))
	}
}

func TestLeagueStore_ListLeagues_OrderedByID(t *testing.T) {
	store := newLeagueStore(t)
	ctx := context.Background()
	if _, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{Name: "Z-League", GameFormat: "8ball"}); err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	if _, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{Name: "A-League", GameFormat: "8ball"}); err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}

	got, err := store.ListLeagues(ctx)
	if err != nil {
		t.Fatalf("ListLeagues: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("want ≥2 leagues, got %d", len(got))
	}
	if got[0].ID >= got[1].ID {
		t.Errorf("want ordered by id ASC, got ids %d then %d", got[0].ID, got[1].ID)
	}
}

// ── GetLeague ─────────────────────────────────────────────────────────────────

func TestLeagueStore_GetLeague_ReturnsRecord(t *testing.T) {
	store := newLeagueStore(t)
	ctx := context.Background()
	created, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{
		Name: "Gamma", GameFormat: "10ball", DayOfWeek: "Wednesday",
	})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}

	got, err := store.GetLeague(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetLeague: %v", err)
	}
	if got.Name != "Gamma" {
		t.Errorf("want Name=Gamma, got %q", got.Name)
	}
	if got.GameFormat != "10ball" {
		t.Errorf("want GameFormat=10ball, got %q", got.GameFormat)
	}
	if got.DayOfWeek != "Wednesday" {
		t.Errorf("want DayOfWeek=Wednesday, got %q", got.DayOfWeek)
	}
	if got.CreatedAt.IsZero() {
		t.Error("want non-zero CreatedAt")
	}
}

func TestLeagueStore_GetLeague_NotFound(t *testing.T) {
	store := newLeagueStore(t)
	_, err := store.GetLeague(context.Background(), 9999)
	if !errors.Is(err, leagues.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── CreateLeague ──────────────────────────────────────────────────────────────

func TestLeagueStore_CreateLeague_InsertsRow(t *testing.T) {
	store := newLeagueStore(t)
	ctx := context.Background()

	l, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{
		Name: "Delta", GameFormat: "8ball", DayOfWeek: "Thursday",
	})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	if l.ID == 0 {
		t.Error("want non-zero ID")
	}
	if l.Name != "Delta" {
		t.Errorf("want Name=Delta, got %q", l.Name)
	}
	if l.GameFormat != "8ball" {
		t.Errorf("want GameFormat=8ball, got %q", l.GameFormat)
	}
	if l.DayOfWeek != "Thursday" {
		t.Errorf("want DayOfWeek=Thursday, got %q", l.DayOfWeek)
	}
}

func TestLeagueStore_CreateLeague_NoDayOfWeek(t *testing.T) {
	store := newLeagueStore(t)
	ctx := context.Background()

	l, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{
		Name: "Epsilon", GameFormat: "8ball",
	})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	// DayOfWeek is optional; empty string is the stored form of NULL.
	if l.DayOfWeek != "" {
		t.Errorf("want empty DayOfWeek, got %q", l.DayOfWeek)
	}
}

// ── UpdateLeague ──────────────────────────────────────────────────────────────

func TestLeagueStore_UpdateLeague_UpdatesFields(t *testing.T) {
	store := newLeagueStore(t)
	ctx := context.Background()

	created, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{Name: "Zeta", GameFormat: "8ball"})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	if err := store.UpdateLeague(ctx, created.ID, leagues.UpdateLeagueInput{
		Name: "Zeta Updated", GameFormat: "9ball", DayOfWeek: "Friday",
	}); err != nil {
		t.Fatalf("UpdateLeague: %v", err)
	}

	got, err := store.GetLeague(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetLeague after update: %v", err)
	}
	if got.Name != "Zeta Updated" {
		t.Errorf("want Name=Zeta Updated, got %q", got.Name)
	}
	if got.GameFormat != "9ball" {
		t.Errorf("want GameFormat=9ball, got %q", got.GameFormat)
	}
	if got.DayOfWeek != "Friday" {
		t.Errorf("want DayOfWeek=Friday, got %q", got.DayOfWeek)
	}
}

func TestLeagueStore_UpdateLeague_MissingRowNoError(t *testing.T) {
	store := newLeagueStore(t)
	if err := store.UpdateLeague(context.Background(), 9999, leagues.UpdateLeagueInput{Name: "X"}); err != nil {
		t.Errorf("want nil error for non-existent league, got %v", err)
	}
}

// ── DeleteLeague ──────────────────────────────────────────────────────────────

func TestLeagueStore_DeleteLeague_DeletesRow(t *testing.T) {
	store := newLeagueStore(t)
	ctx := context.Background()

	created, err := store.CreateLeague(ctx, leagues.CreateLeagueInput{Name: "Eta", GameFormat: "8ball"})
	if err != nil {
		t.Fatalf("CreateLeague: %v", err)
	}
	if err := store.DeleteLeague(ctx, created.ID); err != nil {
		t.Fatalf("DeleteLeague: %v", err)
	}
	_, err = store.GetLeague(ctx, created.ID)
	if !errors.Is(err, leagues.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestLeagueStore_DeleteLeague_MissingRowNoError(t *testing.T) {
	store := newLeagueStore(t)
	if err := store.DeleteLeague(context.Background(), 9999); err != nil {
		t.Errorf("want nil error for non-existent league, got %v", err)
	}
}
