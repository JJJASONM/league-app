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

// -- MergePlayers ------------------------------------------------------------

func seedLeagueRow(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(
		`INSERT INTO leagues (name, game_format) VALUES ('Merge League','8ball') RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatalf("seedLeagueRow: %v", err)
	}
	return id
}

func seedTeamRow(t *testing.T, leagueID int64, name string) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(
		`INSERT INTO teams (league_id, name) VALUES (?,?) RETURNING id`, leagueID, name,
	).Scan(&id); err != nil {
		t.Fatalf("seedTeamRow: %v", err)
	}
	return id
}

func seedSeasonRow(t *testing.T, leagueID int64) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(
		`INSERT INTO seasons (league_id, name) VALUES (?,'Merge Season') RETURNING id`, leagueID,
	).Scan(&id); err != nil {
		t.Fatalf("seedSeasonRow: %v", err)
	}
	return id
}

func seedMatchRow(t *testing.T, seasonID, homeTeamID, awayTeamID int64, week int) int64 {
	t.Helper()
	var id int64
	if err := db.DB.QueryRow(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number)
		 VALUES (?,?,?,?) RETURNING id`, seasonID, homeTeamID, awayTeamID, week,
	).Scan(&id); err != nil {
		t.Fatalf("seedMatchRow: %v", err)
	}
	return id
}

// seedRoundResultRow inserts a round_results row with distinct handicap
// snapshot values so tests can verify they survive a merge untouched.
func seedRoundResultRow(t *testing.T, matchID int64, round int, homePlayerID, awayPlayerID int64) {
	t.Helper()
	_, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,?,?,?,10,4,10,3,10,2,3.25,1.75,4,'away')`,
		matchID, round, homePlayerID, awayPlayerID)
	if err != nil {
		t.Fatalf("seedRoundResultRow: %v", err)
	}
}

func playerIDExists(t *testing.T, id int64) bool {
	t.Helper()
	var n int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM players WHERE id=?`, id).Scan(&n); err != nil {
		t.Fatalf("playerIDExists: %v", err)
	}
	return n > 0
}

// TestPlayerStore_MergePlayers_RepointsAllReferences seeds one row in every
// FK column that references players.id and verifies a successful merge moves
// every one of them from source to target, then deletes source.
func TestPlayerStore_MergePlayers_RepointsAllReferences(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	leagueID := seedLeagueRow(t)
	teamID := seedTeamRow(t, leagueID, "Team One")
	seasonID := seedSeasonRow(t, leagueID)

	source := seedPlayerRow(t, "Source", "Player")
	target := seedPlayerRow(t, "Target", "Player")
	other1 := seedPlayerRow(t, "Other", "One") // opponent in the home-side round
	other2 := seedPlayerRow(t, "Other", "Two") // opponent in the away-side round
	subFor := seedPlayerRow(t, "Sub", "Player")

	matchHome := seedMatchRow(t, seasonID, teamID, teamID, 1)
	seedRoundResultRow(t, matchHome, 1, source, other1) // exercises home_player_id

	matchAway := seedMatchRow(t, seasonID, teamID, teamID, 2)
	seedRoundResultRow(t, matchAway, 1, other2, source) // exercises away_player_id

	if _, err := db.DB.Exec(
		`INSERT INTO match_results (match_id, player_id, team_id) VALUES (?,?,?)`,
		matchHome, source, teamID); err != nil {
		t.Fatalf("seed match_results: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?,1.0,2.0,'2026-01-01')`, source); err != nil {
		t.Fatalf("seed handicap_history: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO lineup_plans (team_id, player_id, week_number, season_id) VALUES (?,?,1,?)`,
		teamID, source, seasonID); err != nil {
		t.Fatalf("seed lineup_plans.player_id: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO lineup_plans (team_id, player_id, week_number, season_id, is_sub, sub_for_id)
		 VALUES (?,?,2,?,1,?)`, teamID, subFor, seasonID, source); err != nil {
		t.Fatalf("seed lineup_plans.sub_for_id: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO season_teams (season_id, team_id, season_name, captain_id) VALUES (?,?,'Team One',?)`,
		seasonID, teamID, source); err != nil {
		t.Fatalf("seed season_teams.captain_id: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, teamID, source); err != nil {
		t.Fatalf("seed season_rosters: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE teams SET captain_id=? WHERE id=?`, source, teamID); err != nil {
		t.Fatalf("seed teams.captain_id: %v", err)
	}

	if err := store.MergePlayers(ctx, source, target); err != nil {
		t.Fatalf("MergePlayers: %v", err)
	}

	// Source is gone; target still exists.
	if playerIDExists(t, source) {
		t.Error("want source player deleted after merge, still exists")
	}
	if !playerIDExists(t, target) {
		t.Error("want target player to still exist after merge")
	}

	assertInt64 := func(label string, query string, args ...any) {
		t.Helper()
		var got int64
		if err := db.DB.QueryRow(query, args...).Scan(&got); err != nil {
			t.Fatalf("%s: query: %v", label, err)
		}
		if got != target {
			t.Errorf("%s: want %d, got %d", label, target, got)
		}
	}

	assertInt64("match_results.player_id", `SELECT player_id FROM match_results WHERE match_id=?`, matchHome)
	assertInt64("handicap_history.player_id", `SELECT player_id FROM handicap_history WHERE effective_date='2026-01-01'`)
	assertInt64("round_results.home_player_id", `SELECT home_player_id FROM round_results WHERE match_id=? AND round_number=1`, matchHome)
	assertInt64("round_results.away_player_id", `SELECT away_player_id FROM round_results WHERE match_id=? AND round_number=1`, matchAway)
	assertInt64("lineup_plans.player_id", `SELECT player_id FROM lineup_plans WHERE team_id=? AND week_number=1 AND season_id=?`, teamID, seasonID)
	assertInt64("lineup_plans.sub_for_id", `SELECT sub_for_id FROM lineup_plans WHERE team_id=? AND week_number=2 AND season_id=?`, teamID, seasonID)
	assertInt64("season_teams.captain_id", `SELECT captain_id FROM season_teams WHERE season_id=? AND team_id=?`, seasonID, teamID)
	assertInt64("season_rosters.player_id", `SELECT player_id FROM season_rosters WHERE season_id=? AND team_id=?`, seasonID, teamID)
	assertInt64("teams.captain_id", `SELECT captain_id FROM teams WHERE id=?`, teamID)
}

// TestPlayerStore_MergePlayers_SnapshotColumnsUnchanged verifies that merging
// only moves the player-ID column on round_results -- the handicap snapshot
// columns and game scores are byte-for-byte unchanged.
func TestPlayerStore_MergePlayers_SnapshotColumnsUnchanged(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	leagueID := seedLeagueRow(t)
	teamID := seedTeamRow(t, leagueID, "Team One")
	seasonID := seedSeasonRow(t, leagueID)
	source := seedPlayerRow(t, "Source", "Player")
	target := seedPlayerRow(t, "Target", "Player")
	opponent := seedPlayerRow(t, "Opp", "Onent")
	match := seedMatchRow(t, seasonID, teamID, teamID, 1)
	seedRoundResultRow(t, match, 1, source, opponent)

	if err := store.MergePlayers(ctx, source, target); err != nil {
		t.Fatalf("MergePlayers: %v", err)
	}

	var homePlayerID int64
	var homeHC, awayHC float64
	var hcPts int
	var hcTo string
	var g1h, g1a, g2h, g2a, g3h, g3a int
	err := db.DB.QueryRow(`
		SELECT home_player_id, home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to,
		       game1_home, game1_away, game2_home, game2_away, game3_home, game3_away
		FROM round_results WHERE match_id=? AND round_number=1`, match,
	).Scan(&homePlayerID, &homeHC, &awayHC, &hcPts, &hcTo, &g1h, &g1a, &g2h, &g2a, &g3h, &g3a)
	if err != nil {
		t.Fatalf("query round_results: %v", err)
	}

	if homePlayerID != target {
		t.Errorf("want home_player_id=%d (repointed), got %d", target, homePlayerID)
	}
	if homeHC != 3.25 || awayHC != 1.75 || hcPts != 4 || hcTo != "away" {
		t.Errorf("want snapshot columns unchanged (3.25, 1.75, 4, away), got (%v, %v, %v, %v)",
			homeHC, awayHC, hcPts, hcTo)
	}
	if g1h != 10 || g1a != 4 || g2h != 10 || g2a != 3 || g3h != 10 || g3a != 2 {
		t.Errorf("want game scores unchanged (10/4, 10/3, 10/2), got (%d/%d, %d/%d, %d/%d)",
			g1h, g1a, g2h, g2a, g3h, g3a)
	}
}

// TestPlayerStore_MergePlayers_HandicapHistorySurvivesRepointedNotRewritten
// verifies that both players' handicap_history rows survive a merge with
// their old/new handicap values unchanged -- only player_id moves.
func TestPlayerStore_MergePlayers_HandicapHistorySurvivesRepointedNotRewritten(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	source := seedPlayerRow(t, "Source", "Player")
	target := seedPlayerRow(t, "Target", "Player")

	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?,1.0,2.0,'2026-01-01')`, source); err != nil {
		t.Fatalf("seed source handicap_history: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?,5.0,6.0,'2026-02-01')`, target); err != nil {
		t.Fatalf("seed target handicap_history: %v", err)
	}

	if err := store.MergePlayers(ctx, source, target); err != nil {
		t.Fatalf("MergePlayers: %v", err)
	}

	rows, err := db.DB.Query(
		`SELECT old_handicap, new_handicap, strftime('%Y-%m-%d', effective_date)
		 FROM handicap_history WHERE player_id=? ORDER BY effective_date`,
		target)
	if err != nil {
		t.Fatalf("query handicap_history: %v", err)
	}
	defer rows.Close()

	type row struct {
		old, new float64
		date     string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.old, &r.new, &r.date); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 handicap_history rows repointed to target, got %d", len(got))
	}
	if got[0].old != 1.0 || got[0].new != 2.0 || got[0].date != "2026-01-01" {
		t.Errorf("want source's row preserved as (1.0, 2.0, 2026-01-01), got %+v", got[0])
	}
	if got[1].old != 5.0 || got[1].new != 6.0 || got[1].date != "2026-02-01" {
		t.Errorf("want target's own row preserved as (5.0, 6.0, 2026-02-01), got %+v", got[1])
	}
}

// TestPlayerStore_MergePlayers_SeasonRosterConflict_Blocks verifies that
// merging is refused, with nothing changed, when source and target are both
// already rostered in the same season (regardless of team).
func TestPlayerStore_MergePlayers_SeasonRosterConflict_Blocks(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	leagueID := seedLeagueRow(t)
	team1 := seedTeamRow(t, leagueID, "Team One")
	team2 := seedTeamRow(t, leagueID, "Team Two")
	seasonID := seedSeasonRow(t, leagueID)
	source := seedPlayerRow(t, "Source", "Player")
	target := seedPlayerRow(t, "Target", "Player")

	// season_rosters has FK(season_id, team_id) -> season_teams(season_id, team_id).
	if _, err := db.DB.Exec(
		`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team One')`,
		seasonID, team1); err != nil {
		t.Fatalf("seed season_teams team1: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,'Team Two')`,
		seasonID, team2); err != nil {
		t.Fatalf("seed season_teams team2: %v", err)
	}

	if _, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, team1, source); err != nil {
		t.Fatalf("seed source roster: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		seasonID, team2, target); err != nil {
		t.Fatalf("seed target roster: %v", err)
	}
	// Unrelated reference to prove rollback leaves it untouched too.
	if _, err := db.DB.Exec(
		`INSERT INTO handicap_history (player_id, old_handicap, new_handicap, effective_date)
		 VALUES (?,1.0,2.0,'2026-01-01')`, source); err != nil {
		t.Fatalf("seed handicap_history: %v", err)
	}

	err := store.MergePlayers(ctx, source, target)
	if !errors.Is(err, players.ErrMergeSeasonRosterConflict) {
		t.Fatalf("want ErrMergeSeasonRosterConflict, got %v", err)
	}

	if !playerIDExists(t, source) {
		t.Error("want source player to still exist after blocked merge (rollback)")
	}
	var hcPlayerID int64
	if err := db.DB.QueryRow(`SELECT player_id FROM handicap_history WHERE effective_date='2026-01-01'`).Scan(&hcPlayerID); err != nil {
		t.Fatalf("query handicap_history: %v", err)
	}
	if hcPlayerID != source {
		t.Errorf("want handicap_history still referencing source=%d after rollback, got %d", source, hcPlayerID)
	}
	var team1Count, team2Count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND team_id=? AND player_id=?`, seasonID, team1, source).Scan(&team1Count)
	db.DB.QueryRow(`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND team_id=? AND player_id=?`, seasonID, team2, target).Scan(&team2Count)
	if team1Count != 1 || team2Count != 1 {
		t.Errorf("want both original season_rosters rows unchanged, got counts (%d, %d)", team1Count, team2Count)
	}
}

// TestPlayerStore_MergePlayers_RoundResultConflict_Blocks verifies that
// merging is refused whenever source and target already both participate
// (home or away, in any row) in the same (match_id, round_number) --
// covering every combination except the same-row, opposing-players case,
// which is TestPlayerStore_MergePlayers_SelfOpponentConflict_Blocks instead.
func TestPlayerStore_MergePlayers_RoundResultConflict_Blocks(t *testing.T) {
	cases := []struct {
		name                   string
		sourceHome             bool // source's role in row 1; false = away
		targetHome             bool // target's role in row 2; false = away
	}{
		{"both home", true, true},
		{"both away", false, false},
		{"source home, target away (different rows)", true, false},
		{"source away, target home (different rows)", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newPlayerStore(t)
			ctx := context.Background()

			leagueID := seedLeagueRow(t)
			teamID := seedTeamRow(t, leagueID, "Team One")
			seasonID := seedSeasonRow(t, leagueID)
			source := seedPlayerRow(t, "Source", "Player")
			target := seedPlayerRow(t, "Target", "Player")
			other1 := seedPlayerRow(t, "Other", "One")
			other2 := seedPlayerRow(t, "Other", "Two")
			match := seedMatchRow(t, seasonID, teamID, teamID, 1)

			row1Home, row1Away := other1, source
			if tc.sourceHome {
				row1Home, row1Away = source, other1
			}
			row2Home, row2Away := other2, target
			if tc.targetHome {
				row2Home, row2Away = target, other2
			}

			if _, err := db.DB.Exec(`
				INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id)
				VALUES (?,1,?,?)`, match, row1Home, row1Away); err != nil {
				t.Fatalf("seed row1: %v", err)
			}
			if _, err := db.DB.Exec(`
				INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id)
				VALUES (?,1,?,?)`, match, row2Home, row2Away); err != nil {
				t.Fatalf("seed row2: %v", err)
			}

			err := store.MergePlayers(ctx, source, target)
			if !errors.Is(err, players.ErrMergeRoundResultConflict) {
				t.Fatalf("want ErrMergeRoundResultConflict, got %v", err)
			}
			if !playerIDExists(t, source) {
				t.Error("want source player to still exist after blocked merge (rollback)")
			}
			var count int
			db.DB.QueryRow(`SELECT COUNT(*) FROM round_results WHERE match_id=? AND round_number=1`, match).Scan(&count)
			if count != 2 {
				t.Errorf("want both original round_results rows unchanged, got %d rows", count)
			}
		})
	}
}

// TestPlayerStore_MergePlayers_SelfOpponentConflict_Blocks verifies that
// merging is refused when source and target already played against each
// other in a recorded round -- merging would make a player play themselves.
func TestPlayerStore_MergePlayers_SelfOpponentConflict_Blocks(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	leagueID := seedLeagueRow(t)
	teamID := seedTeamRow(t, leagueID, "Team One")
	seasonID := seedSeasonRow(t, leagueID)
	source := seedPlayerRow(t, "Source", "Player")
	target := seedPlayerRow(t, "Target", "Player")
	match := seedMatchRow(t, seasonID, teamID, teamID, 1)

	if _, err := db.DB.Exec(`
		INSERT INTO round_results (match_id, round_number, home_player_id, away_player_id)
		VALUES (?,1,?,?)`, match, source, target); err != nil {
		t.Fatalf("seed opposing round: %v", err)
	}

	err := store.MergePlayers(ctx, source, target)
	if !errors.Is(err, players.ErrMergeSelfOpponentConflict) {
		t.Fatalf("want ErrMergeSelfOpponentConflict, got %v", err)
	}
	if !playerIDExists(t, source) {
		t.Error("want source player to still exist after blocked merge (rollback)")
	}
	var homeID, awayID int64
	db.DB.QueryRow(`SELECT home_player_id, away_player_id FROM round_results WHERE match_id=? AND round_number=1`, match).
		Scan(&homeID, &awayID)
	if homeID != source || awayID != target {
		t.Errorf("want round_results row unchanged (home=%d, away=%d), got (home=%d, away=%d)",
			source, target, homeID, awayID)
	}
}

// TestPlayerStore_MergePlayers_LineupPlanConflict_Blocks verifies that
// merging is refused when source and target already both have a lineup_plans
// row for the same team/week/season.
func TestPlayerStore_MergePlayers_LineupPlanConflict_Blocks(t *testing.T) {
	store := newPlayerStore(t)
	ctx := context.Background()

	leagueID := seedLeagueRow(t)
	teamID := seedTeamRow(t, leagueID, "Team One")
	seasonID := seedSeasonRow(t, leagueID)
	source := seedPlayerRow(t, "Source", "Player")
	target := seedPlayerRow(t, "Target", "Player")

	if _, err := db.DB.Exec(
		`INSERT INTO lineup_plans (team_id, player_id, week_number, season_id) VALUES (?,?,1,?)`,
		teamID, source, seasonID); err != nil {
		t.Fatalf("seed source lineup_plans: %v", err)
	}
	if _, err := db.DB.Exec(
		`INSERT INTO lineup_plans (team_id, player_id, week_number, season_id) VALUES (?,?,1,?)`,
		teamID, target, seasonID); err != nil {
		t.Fatalf("seed target lineup_plans: %v", err)
	}

	err := store.MergePlayers(ctx, source, target)
	if !errors.Is(err, players.ErrMergeLineupPlanConflict) {
		t.Fatalf("want ErrMergeLineupPlanConflict, got %v", err)
	}
	if !playerIDExists(t, source) {
		t.Error("want source player to still exist after blocked merge (rollback)")
	}
	var count int
	db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE team_id=? AND week_number=1 AND season_id=?`, teamID, seasonID).Scan(&count)
	if count != 2 {
		t.Errorf("want both original lineup_plans rows unchanged, got %d rows", count)
	}
}
