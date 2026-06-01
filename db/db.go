package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// Init opens (or creates) the SQLite database and runs migrations.
func Init(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "league.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// SQLite pragmas for performance and safety
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("pragma %s: %w", p, err)
		}
	}

	DB = db
	return migrate()
}

// Seed runs the provided SQL (the embedded seed.sql) to insert starter data.
// Safe to run on an existing database — all inserts use INSERT OR IGNORE.
func Seed(sql string) error {
	_, err := DB.Exec(sql)
	return err
}

// Backup copies the database to a timestamped file in dataDir.
func Backup(dataDir string) (string, error) {
	src := filepath.Join(dataDir, "league.db")
	stamp := time.Now().Format("2006-01-02_150405")
	dst := filepath.Join(dataDir, fmt.Sprintf("league_backup_%s.db", stamp))

	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("reading db for backup: %w", err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", fmt.Errorf("writing backup: %w", err)
	}
	log.Printf("backup saved: %s", dst)
	return dst, nil
}

// migrate creates all tables if they don't exist.
func migrate() error {
	schema := `
-- Leagues: top-level container (e.g. "Monday 8-Ball", "Tuesday 9-Ball")
CREATE TABLE IF NOT EXISTS leagues (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    game_format TEXT    NOT NULL DEFAULT '8ball', -- '8ball','9ball','10ball','straight'
    day_of_week TEXT,                              -- 'Monday', 'Tuesday', etc.
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS teams (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    league_id  INTEGER REFERENCES leagues(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    captain_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(league_id, name)
);

CREATE TABLE IF NOT EXISTS players (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    player_number  TEXT,            -- two-digit code, e.g. "42"; locked once set in UI
    first_name     TEXT    NOT NULL DEFAULT '',
    last_name      TEXT    NOT NULL DEFAULT '',
    phone          TEXT    NOT NULL DEFAULT '',
    email          TEXT    NOT NULL DEFAULT '',
    team_id        INTEGER REFERENCES teams(id) ON DELETE SET NULL,
    -- handicap meaning depends on game format:
    --   8-ball: Diff rating = (games won − games lost) / matches played
    --   9-ball: race-to number (e.g. 5, 7)
    handicap       REAL    NOT NULL DEFAULT 0.0,
    admin_hold     INTEGER NOT NULL DEFAULT 0, -- 1 = locked at Admin Discretion (9-ball only)
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS seasons (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    league_id     INTEGER REFERENCES leagues(id) ON DELETE CASCADE,
    name          TEXT    NOT NULL,
    start_date    DATE,
    end_date      DATE,           -- computed from last match date after schedule generation
    active        INTEGER NOT NULL DEFAULT 0,
    schedule_type TEXT    NOT NULL DEFAULT 'double_rr', -- 'single_rr'|'double_rr'|'split'|'custom'|'blanket'
    num_weeks     INTEGER NOT NULL DEFAULT 0,            -- used for 'custom' and 'blanket'
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Season rules: configurable per-season rule values (e.g. max handicap on scoresheet)
CREATE TABLE IF NOT EXISTS season_rules (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id   INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    rule_key    TEXT    NOT NULL,
    rule_label  TEXT    NOT NULL,
    rule_value  TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(season_id, rule_key)
);

-- Skipped weeks: calendar dates excluded from scheduling (holidays, breaks, etc.)
CREATE TABLE IF NOT EXISTS skipped_weeks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id   INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    skip_date   DATE    NOT NULL,
    reason      TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(season_id, skip_date)
);

-- Bye requests: a team's request to not play a given week
CREATE TABLE IF NOT EXISTS bye_requests (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id   INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    team_id     INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    week_number INTEGER NOT NULL DEFAULT 0,  -- 0 = TBD
    reason      TEXT    NOT NULL DEFAULT '',
    approved    INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(season_id, team_id, week_number)
);

CREATE TABLE IF NOT EXISTS matches (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id     INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    home_team_id  INTEGER REFERENCES teams(id),  -- nullable: unassigned blanket slot
    away_team_id  INTEGER REFERENCES teams(id),  -- nullable: unassigned blanket slot
    match_date    DATE,
    week_number   INTEGER NOT NULL DEFAULT 1,
    match_number  INTEGER,       -- sequential match # for the season
    table_numbers TEXT,          -- e.g. "1&2", "5&6"
    completed     INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS match_results (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    match_id         INTEGER NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    player_id        INTEGER NOT NULL REFERENCES players(id),
    team_id          INTEGER NOT NULL REFERENCES teams(id),
    sets_won         INTEGER NOT NULL DEFAULT 0,
    sets_lost        INTEGER NOT NULL DEFAULT 0,
    games_won        INTEGER NOT NULL DEFAULT 0,
    games_lost       INTEGER NOT NULL DEFAULT 0,
    diff             REAL    NOT NULL DEFAULT 0, -- point differential (8-ball)
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Handicap history: tracks every change per player with effective date
CREATE TABLE IF NOT EXISTS handicap_history (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    player_id     INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    old_handicap  REAL    NOT NULL,
    new_handicap  REAL    NOT NULL,
    effective_date DATE   NOT NULL,
    admin_hold    INTEGER NOT NULL DEFAULT 0,
    note          TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 8-ball round results: point-per-game scoring for each player pairing within a match.
-- Winner of each game always scores 10 (7 object balls × 1 pt + 8-ball × 3 pt).
-- Loser scores however many balls they pocketed (0–7).
-- Pairing winner is determined by adjusted totals after handicap applied.
CREATE TABLE IF NOT EXISTS round_results (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    match_id         INTEGER NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    round_number     INTEGER NOT NULL,  -- 1, 2, or 3
    home_player_id   INTEGER NOT NULL REFERENCES players(id),
    away_player_id   INTEGER NOT NULL REFERENCES players(id),
    game1_home       INTEGER NOT NULL DEFAULT 0,  -- points scored by home player in game 1 (0–10)
    game1_away       INTEGER NOT NULL DEFAULT 0,  -- points scored by away player in game 1 (0–10)
    game2_home       INTEGER NOT NULL DEFAULT 0,
    game2_away       INTEGER NOT NULL DEFAULT 0,
    game3_home       INTEGER NOT NULL DEFAULT 0,
    game3_away       INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(match_id, round_number, home_player_id)
);
CREATE INDEX IF NOT EXISTS idx_round_results_match ON round_results(match_id);

-- Lineup planning: pre-scheduled who plays each week per team
CREATE TABLE IF NOT EXISTS lineup_plans (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id     INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    player_id   INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    week_number INTEGER NOT NULL,
    season_id   INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    is_sub      INTEGER NOT NULL DEFAULT 0,
    sub_for_id  INTEGER REFERENCES players(id),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(team_id, week_number, season_id, player_id)
);

CREATE INDEX IF NOT EXISTS idx_players_team      ON players(team_id);
CREATE INDEX IF NOT EXISTS idx_teams_league      ON teams(league_id);
CREATE INDEX IF NOT EXISTS idx_seasons_league    ON seasons(league_id);
CREATE INDEX IF NOT EXISTS idx_matches_season    ON matches(season_id);
CREATE INDEX IF NOT EXISTS idx_results_match     ON match_results(match_id);
CREATE INDEX IF NOT EXISTS idx_results_player    ON match_results(player_id);
CREATE INDEX IF NOT EXISTS idx_hc_history_player ON handicap_history(player_id);
CREATE INDEX IF NOT EXISTS idx_lineup_team_week  ON lineup_plans(team_id, week_number, season_id);
`
	if _, err := DB.Exec(schema); err != nil {
		return err
	}

	// Additive migrations for existing databases — errors are intentionally ignored
	// because the column may already exist (SQLite has no IF NOT EXISTS for ALTER).
	additiveMigrations := []string{
		`ALTER TABLE seasons ADD COLUMN schedule_type TEXT NOT NULL DEFAULT 'double_rr'`,
		`ALTER TABLE seasons ADD COLUMN num_weeks     INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range additiveMigrations {
		DB.Exec(stmt) // ignore error — column already exists on fresh DBs
	}
	return nil
}
