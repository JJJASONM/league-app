package sqlite_test

import (
	"context"
	"testing"

	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/models"
)

func newFinanceStore(t *testing.T) *sqlite.FinanceStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewFinanceStore(db.DB)
}

// financeStoreSeed creates one league, one season, one team, and one player
// on that team. Returns (seasonID, teamID, playerID). Call financeStoreSeed2
// instead when a test needs a second, independent league/season/team/player
// set in the same database (leagues.name is globally unique).
func financeStoreSeed(t *testing.T) (seasonID, teamID, playerID int64) {
	t.Helper()
	return financeStoreSeedNamed(t, "L")
}

func financeStoreSeed2(t *testing.T) (seasonID, teamID, playerID int64) {
	t.Helper()
	return financeStoreSeedNamed(t, "L2")
}

func financeStoreSeedNamed(t *testing.T, leagueName string) (seasonID, teamID, playerID int64) {
	t.Helper()
	d := db.DB

	r, err := d.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES (?,'8ball','Monday')`, leagueName)
	if err != nil {
		t.Fatalf("seed league: %v", err)
	}
	lgID, _ := r.LastInsertId()

	r, err = d.Exec(`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?,?,?,?,?)`,
		lgID, "S1", "2026-01-01", "single_rr", 3)
	if err != nil {
		t.Fatalf("seed season: %v", err)
	}
	seasonID, _ = r.LastInsertId()

	r, err = d.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Team A')`, lgID)
	if err != nil {
		t.Fatalf("seed team: %v", err)
	}
	teamID, _ = r.LastInsertId()

	r, err = d.Exec(`INSERT INTO players (first_name, last_name, team_id, handicap) VALUES ('Home','Player',?,3.0)`, teamID)
	if err != nil {
		t.Fatalf("seed player: %v", err)
	}
	playerID, _ = r.LastInsertId()

	return seasonID, teamID, playerID
}

// -- InsertDuesPayment / ListDuesPayments -----------------------------------

func TestFinanceStore_InsertDuesPayment_ReturnsStoredRowWithNames(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, teamID, playerID := financeStoreSeed(t)

	got, err := store.InsertDuesPayment(context.Background(), models.DuesPayment{
		SeasonID: seasonID, PlayerID: playerID, TeamID: &teamID,
		Amount: 25.0, PaidAt: "2026-01-05", Note: "cash",
	})
	if err != nil {
		t.Fatalf("InsertDuesPayment: %v", err)
	}
	if got.ID == 0 {
		t.Error("want non-zero ID")
	}
	if got.CreatedAt == "" {
		t.Error("want non-empty CreatedAt")
	}
	if got.PlayerName != "Home Player" {
		t.Errorf("want player_name='Home Player', got %q", got.PlayerName)
	}
	if got.TeamName != "Team A" {
		t.Errorf("want team_name='Team A', got %q", got.TeamName)
	}
	if got.Amount != 25.0 {
		t.Errorf("want amount=25.0, got %v", got.Amount)
	}
}

func TestFinanceStore_InsertDuesPayment_NilTeamID_NoTeamNameLookupError(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, _, playerID := financeStoreSeed(t)

	got, err := store.InsertDuesPayment(context.Background(), models.DuesPayment{
		SeasonID: seasonID, PlayerID: playerID, TeamID: nil,
		Amount: 10.0, PaidAt: "2026-01-05",
	})
	if err != nil {
		t.Fatalf("InsertDuesPayment: %v", err)
	}
	if got.TeamID != nil {
		t.Errorf("want TeamID=nil, got %v", got.TeamID)
	}
	if got.TeamName != "" {
		t.Errorf("want empty TeamName, got %q", got.TeamName)
	}
}

func TestFinanceStore_ListDuesPayments_EmptySeason_ReturnsEmptySlice(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, _, _ := financeStoreSeed(t)

	got, err := store.ListDuesPayments(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("ListDuesPayments: %v", err)
	}
	if got == nil {
		t.Error("want non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("want 0 payments, got %d", len(got))
	}
}

func TestFinanceStore_ListDuesPayments_NewestFirst(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, teamID, playerID := financeStoreSeed(t)
	ctx := context.Background()

	first, err := store.InsertDuesPayment(ctx, models.DuesPayment{
		SeasonID: seasonID, PlayerID: playerID, TeamID: &teamID, Amount: 10, PaidAt: "2026-01-01",
	})
	if err != nil {
		t.Fatalf("insert first: %v", err)
	}
	second, err := store.InsertDuesPayment(ctx, models.DuesPayment{
		SeasonID: seasonID, PlayerID: playerID, TeamID: &teamID, Amount: 15, PaidAt: "2026-01-02",
	})
	if err != nil {
		t.Fatalf("insert second: %v", err)
	}

	got, err := store.ListDuesPayments(ctx, seasonID)
	if err != nil {
		t.Fatalf("ListDuesPayments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 payments, got %d", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Errorf("want newest-first order [%d,%d], got [%d,%d]", second.ID, first.ID, got[0].ID, got[1].ID)
	}
}

func TestFinanceStore_ListDuesPayments_ScopedBySeason(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, teamID, playerID := financeStoreSeed(t)
	otherSeasonID, otherTeamID, otherPlayerID := financeStoreSeed2(t)
	ctx := context.Background()

	if _, err := store.InsertDuesPayment(ctx, models.DuesPayment{
		SeasonID: seasonID, PlayerID: playerID, TeamID: &teamID, Amount: 10, PaidAt: "2026-01-01",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := store.InsertDuesPayment(ctx, models.DuesPayment{
		SeasonID: otherSeasonID, PlayerID: otherPlayerID, TeamID: &otherTeamID, Amount: 99, PaidAt: "2026-01-01",
	}); err != nil {
		t.Fatalf("insert other season: %v", err)
	}

	got, err := store.ListDuesPayments(ctx, seasonID)
	if err != nil {
		t.Fatalf("ListDuesPayments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 payment scoped to season, got %d", len(got))
	}
	if got[0].Amount != 10 {
		t.Errorf("want amount=10 from the correct season, got %v", got[0].Amount)
	}
}

// -- InsertPayout / ListPayouts ----------------------------------------------

func TestFinanceStore_InsertPayout_ReturnsStoredRowWithTeamName(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, teamID, _ := financeStoreSeed(t)

	got, err := store.InsertPayout(context.Background(), models.Payout{
		SeasonID: seasonID, TeamID: teamID, Amount: 150.0, Note: "1st place",
	})
	if err != nil {
		t.Fatalf("InsertPayout: %v", err)
	}
	if got.ID == 0 {
		t.Error("want non-zero ID")
	}
	if got.CreatedAt == "" {
		t.Error("want non-empty CreatedAt")
	}
	if got.TeamName != "Team A" {
		t.Errorf("want team_name='Team A', got %q", got.TeamName)
	}
	if got.Amount != 150.0 {
		t.Errorf("want amount=150.0, got %v", got.Amount)
	}
}

func TestFinanceStore_ListPayouts_EmptySeason_ReturnsEmptySlice(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, _, _ := financeStoreSeed(t)

	got, err := store.ListPayouts(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("ListPayouts: %v", err)
	}
	if got == nil {
		t.Error("want non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("want 0 payouts, got %d", len(got))
	}
}

func TestFinanceStore_ListPayouts_NewestFirst(t *testing.T) {
	store := newFinanceStore(t)
	seasonID, teamID, _ := financeStoreSeed(t)
	ctx := context.Background()

	first, err := store.InsertPayout(ctx, models.Payout{SeasonID: seasonID, TeamID: teamID, Amount: 50})
	if err != nil {
		t.Fatalf("insert first: %v", err)
	}
	second, err := store.InsertPayout(ctx, models.Payout{SeasonID: seasonID, TeamID: teamID, Amount: 75})
	if err != nil {
		t.Fatalf("insert second: %v", err)
	}

	got, err := store.ListPayouts(ctx, seasonID)
	if err != nil {
		t.Fatalf("ListPayouts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 payouts, got %d", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Errorf("want newest-first order [%d,%d], got [%d,%d]", second.ID, first.ID, got[0].ID, got[1].ID)
	}
}
