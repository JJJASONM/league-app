package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/players"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

func newPlayerStore(t *testing.T) *sqlite.PlayerStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewPlayerStore(db.DB)
}

// seedPlayerRow inserts a minimal player and returns its ID.
func seedPlayerRow(t *testing.T, firstName, lastName string) int64 {
	t.Helper()
	res, err := db.DB.Exec(
		`INSERT INTO players (first_name, last_name, handicap) VALUES (?,?,0)`,
		firstName, lastName)
	if err != nil {
		t.Fatalf("seedPlayerRow: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ── ListPlayers ───────────────────────────────────────────────────────────────

func TestPlayerStore_ListPlayers_ReturnsAll(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	if _, err := store.CreatePlayer(ctx, players.CreatePlayerInput{FirstName: "Alice", LastName: "A"}); err != nil {
		t.Fatalf("CreatePlayer Alice: %v", err)
	}
	if _, err := store.CreatePlayer(ctx, players.CreatePlayerInput{FirstName: "Bob", LastName: "B"}); err != nil {
		t.Fatalf("CreatePlayer Bob: %v", err)
	}

	got, err := store.ListPlayers(ctx, nil)
	if err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 players, got %d", len(got))
	}
}

func TestPlayerStore_ListPlayers_EmptyWhenNone(t *testing.T) {
	store := newPlayerStore(t)
	got, err := store.ListPlayers(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 players, got %d", len(got))
	}
}

func TestPlayerStore_ListPlayers_LeagueFilter(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	// Insert league and two teams.
	var leagueID int64
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L','8ball') RETURNING id`).Scan(&leagueID)
	var team1ID, team2ID int64
	db.DB.QueryRow(`INSERT INTO teams (league_id, name) VALUES (?,?) RETURNING id`, leagueID, "Team1").Scan(&team1ID)
	db.DB.QueryRow(`INSERT INTO teams (league_id, name) VALUES (?,?) RETURNING id`, leagueID, "Team2").Scan(&team2ID)

	// Insert another league with its own team.
	var otherLeagueID int64
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('Other','8ball') RETURNING id`).Scan(&otherLeagueID)
	var otherTeamID int64
	db.DB.QueryRow(`INSERT INTO teams (league_id, name) VALUES (?,?) RETURNING id`, otherLeagueID, "OtherTeam").Scan(&otherTeamID)

	// Players in target league.
	db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('P1','X',?)`, team1ID)
	db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('P2','X',?)`, team2ID)
	// Player in other league.
	db.DB.Exec(`INSERT INTO players (first_name, last_name, team_id) VALUES ('P3','X',?)`, otherTeamID)
	// Player with no team.
	db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('P4','X')`)

	got, err := store.ListPlayers(ctx, &leagueID)
	if err != nil {
		t.Fatalf("ListPlayers with leagueID: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 players for leagueID=%d, got %d", leagueID, len(got))
	}
	for _, p := range got {
		if p.LeagueID != leagueID {
			t.Errorf("want all results in leagueID=%d, got leagueID=%d", leagueID, p.LeagueID)
		}
	}
}

// ── GetPlayer ─────────────────────────────────────────────────────────────────

func TestPlayerStore_GetPlayer_ReturnsRecord(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	created, err := store.CreatePlayer(ctx, players.CreatePlayerInput{
		PlayerNumber: "07",
		FirstName:    "Carol",
		LastName:     "Jones",
		Handicap:     1.5,
	})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}

	got, err := store.GetPlayer(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPlayer: %v", err)
	}
	if got.FirstName != "Carol" {
		t.Errorf("want FirstName=Carol, got %q", got.FirstName)
	}
	if got.PlayerNumber != "07" {
		t.Errorf("want PlayerNumber=07, got %q", got.PlayerNumber)
	}
	if got.Handicap != 1.5 {
		t.Errorf("want Handicap=1.5, got %v", got.Handicap)
	}
	if got.CreatedAt.IsZero() {
		t.Error("want non-zero CreatedAt")
	}
}

func TestPlayerStore_GetPlayer_NotFound(t *testing.T) {
	store := newPlayerStore(t)
	_, err := store.GetPlayer(context.Background(), 9999)
	if !errors.Is(err, players.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── CreatePlayer ──────────────────────────────────────────────────────────────

func TestPlayerStore_CreatePlayer_InsertsRow(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	p, err := store.CreatePlayer(ctx, players.CreatePlayerInput{
		PlayerNumber: "42",
		FirstName:    "Dan",
		LastName:     "Smith",
		Phone:        "555-0100",
		Email:        "dan@example.com",
		Handicap:     3.0,
	})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	if p.ID == 0 {
		t.Error("want non-zero ID")
	}
	if p.PlayerNumber != "42" {
		t.Errorf("want PlayerNumber=42, got %q", p.PlayerNumber)
	}
	if p.Name != "Dan Smith" {
		t.Errorf("want Name=Dan Smith, got %q", p.Name)
	}
}

func TestPlayerStore_CreatePlayer_AdminHold_StoredCorrectly(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	created, err := store.CreatePlayer(ctx, players.CreatePlayerInput{
		FirstName: "Eve",
		LastName:  "H",
		AdminHold: true,
	})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	got, err := store.GetPlayer(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPlayer: %v", err)
	}
	if !got.AdminHold {
		t.Error("want AdminHold=true, got false")
	}
}

// ── UpdatePlayer ──────────────────────────────────────────────────────────────

func TestPlayerStore_UpdatePlayer_UpdatesFields(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	created, err := store.CreatePlayer(ctx, players.CreatePlayerInput{
		FirstName: "Frank", LastName: "Old", Handicap: 1.0,
	})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	if err := store.UpdatePlayer(ctx, created.ID, players.UpdatePlayerInput{
		FirstName: "Frank", LastName: "New", Handicap: 2.5,
	}); err != nil {
		t.Fatalf("UpdatePlayer: %v", err)
	}

	got, err := store.GetPlayer(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPlayer after update: %v", err)
	}
	if got.LastName != "New" {
		t.Errorf("want LastName=New, got %q", got.LastName)
	}
	if got.Handicap != 2.5 {
		t.Errorf("want Handicap=2.5, got %v", got.Handicap)
	}
}

func TestPlayerStore_UpdatePlayer_DoesNotUpdatePlayerNumber(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	created, err := store.CreatePlayer(ctx, players.CreatePlayerInput{
		PlayerNumber: "77",
		FirstName:    "Gina",
		LastName:     "X",
	})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	// UpdatePlayerInput has no PlayerNumber field — update cannot change it.
	if err := store.UpdatePlayer(ctx, created.ID, players.UpdatePlayerInput{
		FirstName: "Gina",
		LastName:  "Y",
	}); err != nil {
		t.Fatalf("UpdatePlayer: %v", err)
	}

	got, err := store.GetPlayer(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetPlayer after update: %v", err)
	}
	if got.PlayerNumber != "77" {
		t.Errorf("want PlayerNumber=77 unchanged, got %q", got.PlayerNumber)
	}
}

func TestPlayerStore_UpdatePlayer_MissingRowNoError(t *testing.T) {
	store := newPlayerStore(t)
	if err := store.UpdatePlayer(context.Background(), 9999, players.UpdatePlayerInput{FirstName: "X"}); err != nil {
		t.Errorf("want nil error for non-existent player, got %v", err)
	}
}

// ── DeletePlayer ──────────────────────────────────────────────────────────────

func TestPlayerStore_DeletePlayer_DeletesRow(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	created, err := store.CreatePlayer(ctx, players.CreatePlayerInput{FirstName: "Hal", LastName: "D"})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	if err := store.DeletePlayer(ctx, created.ID); err != nil {
		t.Fatalf("DeletePlayer: %v", err)
	}
	_, err = store.GetPlayer(ctx, created.ID)
	if !errors.Is(err, players.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestPlayerStore_DeletePlayer_HasHistory_ReturnsErrHasHistory(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	created, err := store.CreatePlayer(ctx, players.CreatePlayerInput{FirstName: "Ivy", LastName: "H"})
	if err != nil {
		t.Fatalf("CreatePlayer: %v", err)
	}
	// Insert a handicap_history row for this player.
	_, err = db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?,1.0,2.0,'2026-01-01')`, created.ID)
	if err != nil {
		t.Fatalf("insert handicap_history: %v", err)
	}

	if err := store.DeletePlayer(ctx, created.ID); !errors.Is(err, players.ErrHasHistory) {
		t.Errorf("want ErrHasHistory, got %v", err)
	}
}

func TestPlayerStore_DeletePlayer_MissingRowNoError(t *testing.T) {
	store := newPlayerStore(t)
	if err := store.DeletePlayer(context.Background(), 9999); err != nil {
		t.Errorf("want nil error for non-existent player, got %v", err)
	}
}
