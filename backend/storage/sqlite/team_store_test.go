package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domains/teams"
	"league_app/backend/storage/sqlite"
	"league_app/db"
)

func newTeamStore(t *testing.T) *sqlite.TeamStore {
	t.Helper()
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return sqlite.NewTeamStore(db.DB)
}

// seedLeagueForTeams inserts a minimal league row and returns its ID.
func seedLeagueForTeams(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(
		`INSERT INTO leagues (name, game_format) VALUES ('Test League','8ball') RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatalf("seedLeagueForTeams: %v", err)
	}
	return id
}

// ── ListTeams ─────────────────────────────────────────────────────────────────

func TestTeamStore_ListTeams_ReturnsAll(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()
	leagueID := seedLeagueForTeams(t)

	if _, err := store.CreateTeam(ctx, teams.CreateTeamInput{Name: "Alpha", LeagueID: leagueID}); err != nil {
		t.Fatalf("CreateTeam Alpha: %v", err)
	}
	if _, err := store.CreateTeam(ctx, teams.CreateTeamInput{Name: "Beta", LeagueID: leagueID}); err != nil {
		t.Fatalf("CreateTeam Beta: %v", err)
	}

	got, err := store.ListTeams(ctx, nil)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 teams, got %d", len(got))
	}
}

func TestTeamStore_ListTeams_EmptyWhenNone(t *testing.T) {
	store := newTeamStore(t)
	got, err := store.ListTeams(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 teams, got %d", len(got))
	}
}

func TestTeamStore_ListTeams_LeagueFilter(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()

	var lid1, lid2 int64
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L1','8ball') RETURNING id`).Scan(&lid1)
	db.DB.QueryRow(`INSERT INTO leagues (name, game_format) VALUES ('L2','8ball') RETURNING id`).Scan(&lid2)

	store.CreateTeam(ctx, teams.CreateTeamInput{Name: "T1", LeagueID: lid1})
	store.CreateTeam(ctx, teams.CreateTeamInput{Name: "T2", LeagueID: lid1})
	store.CreateTeam(ctx, teams.CreateTeamInput{Name: "T3", LeagueID: lid2})

	got, err := store.ListTeams(ctx, &lid1)
	if err != nil {
		t.Fatalf("ListTeams with leagueID: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 teams for league 1, got %d", len(got))
	}
	for _, tm := range got {
		if tm.LeagueID != lid1 {
			t.Errorf("want all results in leagueID=%d, got leagueID=%d", lid1, tm.LeagueID)
		}
	}
}

// ── GetTeam ───────────────────────────────────────────────────────────────────

func TestTeamStore_GetTeam_ReturnsRecord(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()
	leagueID := seedLeagueForTeams(t)

	created, err := store.CreateTeam(ctx, teams.CreateTeamInput{Name: "Gamma", LeagueID: leagueID})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	got, err := store.GetTeam(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.Name != "Gamma" {
		t.Errorf("want Name=Gamma, got %q", got.Name)
	}
	if got.LeagueID != leagueID {
		t.Errorf("want LeagueID=%d, got %d", leagueID, got.LeagueID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("want non-zero CreatedAt")
	}
}

// TestTeamStore_GetTeam_EmbedPlayers_COALESCE verifies that a player with a NULL
// player_number scans without error. This guards the COALESCE fix for the nullable column.
func TestTeamStore_GetTeam_EmbedPlayers_COALESCE(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()
	leagueID := seedLeagueForTeams(t)

	created, err := store.CreateTeam(ctx, teams.CreateTeamInput{Name: "Delta", LeagueID: leagueID})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	// Insert a player with no player_number (leaves column NULL).
	if _, err := db.DB.Exec(
		`INSERT INTO players (first_name, last_name, team_id) VALUES ('Null','Number',?)`, created.ID,
	); err != nil {
		t.Fatalf("insert player: %v", err)
	}

	got, err := store.GetTeam(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTeam with NULL player_number: %v", err)
	}
	if len(got.Players) != 1 {
		t.Fatalf("want 1 embedded player, got %d", len(got.Players))
	}
	if got.Players[0].PlayerNumber != "" {
		t.Errorf("want empty string for NULL player_number, got %q", got.Players[0].PlayerNumber)
	}
}

func TestTeamStore_GetTeam_NotFound(t *testing.T) {
	store := newTeamStore(t)
	_, err := store.GetTeam(context.Background(), 9999)
	if !errors.Is(err, teams.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── CreateTeam ────────────────────────────────────────────────────────────────

func TestTeamStore_CreateTeam_InsertsRow(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()
	leagueID := seedLeagueForTeams(t)

	tm, err := store.CreateTeam(ctx, teams.CreateTeamInput{
		Name:     "Epsilon",
		LeagueID: leagueID,
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if tm.ID == 0 {
		t.Error("want non-zero ID")
	}
	if tm.Name != "Epsilon" {
		t.Errorf("want Name=Epsilon, got %q", tm.Name)
	}
	if tm.LeagueID != leagueID {
		t.Errorf("want LeagueID=%d, got %d", leagueID, tm.LeagueID)
	}
}

// ── UpdateTeam ────────────────────────────────────────────────────────────────

func TestTeamStore_UpdateTeam_UpdatesFields(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()
	leagueID := seedLeagueForTeams(t)

	// Insert a player to use as captain.
	var playerID int64
	db.DB.QueryRow(`INSERT INTO players (first_name, last_name) VALUES ('Cap','Tain') RETURNING id`).Scan(&playerID)

	created, err := store.CreateTeam(ctx, teams.CreateTeamInput{Name: "Zeta", LeagueID: leagueID})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if err := store.UpdateTeam(ctx, created.ID, teams.UpdateTeamInput{
		Name:      "Zeta Renamed",
		CaptainID: &playerID,
	}); err != nil {
		t.Fatalf("UpdateTeam: %v", err)
	}

	got, err := store.GetTeam(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTeam after update: %v", err)
	}
	if got.Name != "Zeta Renamed" {
		t.Errorf("want Name=Zeta Renamed, got %q", got.Name)
	}
	if got.CaptainID == nil || *got.CaptainID != playerID {
		t.Errorf("want CaptainID=%d, got %v", playerID, got.CaptainID)
	}
}

func TestTeamStore_UpdateTeam_MissingRowNoError(t *testing.T) {
	store := newTeamStore(t)
	if err := store.UpdateTeam(context.Background(), 9999, teams.UpdateTeamInput{Name: "X"}); err != nil {
		t.Errorf("want nil error for non-existent team, got %v", err)
	}
}

// ── DeleteTeam ────────────────────────────────────────────────────────────────

func TestTeamStore_DeleteTeam_DeletesRow(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()
	leagueID := seedLeagueForTeams(t)

	created, err := store.CreateTeam(ctx, teams.CreateTeamInput{Name: "Eta", LeagueID: leagueID})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if err := store.DeleteTeam(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}
	_, err = store.GetTeam(ctx, created.ID)
	if !errors.Is(err, teams.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

// TestTeamStore_DeleteTeam_NullsPlayerTeamID verifies that DeleteTeam nulls the
// team_id on any players assigned to the team before deleting the team row.
func TestTeamStore_DeleteTeam_NullsPlayerTeamID(t *testing.T) {
	store := newTeamStore(t)
	ctx := context.Background()
	leagueID := seedLeagueForTeams(t)

	created, err := store.CreateTeam(ctx, teams.CreateTeamInput{Name: "Theta", LeagueID: leagueID})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	// Insert a player assigned to this team.
	var playerID int64
	db.DB.QueryRow(
		`INSERT INTO players (first_name, last_name, team_id) VALUES ('Joe','Smith',?) RETURNING id`,
		created.ID,
	).Scan(&playerID)

	if err := store.DeleteTeam(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}

	// Verify player.team_id is now NULL.
	var teamID *int64
	if err := db.DB.QueryRow(`SELECT team_id FROM players WHERE id=?`, playerID).Scan(&teamID); err != nil {
		t.Fatalf("select player: %v", err)
	}
	if teamID != nil {
		t.Errorf("want team_id=NULL after team delete, got %d", *teamID)
	}
}

func TestTeamStore_DeleteTeam_MissingRowNoError(t *testing.T) {
	store := newTeamStore(t)
	if err := store.DeleteTeam(context.Background(), 9999); err != nil {
		t.Errorf("want nil error for non-existent team, got %v", err)
	}
}
