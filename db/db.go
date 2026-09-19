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
	// foreign_keys is a per-connection SQLite setting -- unlike journal_mode,
	// SQLite does not persist it in the database file, so it defaults back to
	// OFF on every new connection. database/sql maintains a pool of
	// connections and opens more of them on demand (e.g. under concurrent
	// load); a single *sql.DB.Exec("PRAGMA foreign_keys=ON") call after Open
	// below only configures the one connection that happened to run it,
	// leaving every other pooled connection's ON DELETE CASCADE/SET NULL
	// enforcement silently disabled (confirmed empirically: Known Gap #19,
	// reproduced in TestForeignKeysPragma_EnabledOnEveryPooledConnection).
	// modernc.org/sqlite's driver re-applies a DSN's "_pragma=..." query
	// parameter to every connection it opens (see that package's Driver.Open
	// doc comment), so foreign_keys is enabled here, in the DSN itself,
	// instead of via a post-Open Exec call.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// Remaining SQLite pragmas for performance and safety. journal_mode is
	// persisted in the database file itself (WAL mode, once set, applies to
	// every future connection automatically), so a single startup Exec is
	// sufficient for it; synchronous is not correctness-critical the way
	// foreign_keys is, so it is left as a startup-only pragma too, unchanged
	// from before.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
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
// WAL mode is enabled (see Init), so recently committed writes may still be
// sitting in league.db-wal rather than league.db itself. A TRUNCATE
// checkpoint forces all WAL content back into league.db (and empties the
// WAL file) before the copy, so the resulting file is a complete, restorable
// snapshot on its own.
func Backup(dataDir string) (string, error) {
	if _, err := DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return "", fmt.Errorf("checkpoint before backup: %w", err)
	}

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
    week_number   INTEGER,
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

-- League weeks: tracks official Close Week status for each week of a season.
-- A row is created the first time a week is closed. Absence implies 'open' status.
CREATE TABLE IF NOT EXISTS league_weeks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id   INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    week_number INTEGER NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'open', -- 'open' | 'closed'
    closed_at   DATETIME,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(season_id, week_number)
);
CREATE INDEX IF NOT EXISTS idx_league_weeks_season ON league_weeks(season_id);

-- Warning acknowledgments: one row per warning acknowledged by admin at Close Week time.
-- match_id is nullable (ON DELETE SET NULL) so history survives match deletion.
CREATE TABLE IF NOT EXISTS week_close_acknowledgments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id       INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    week_number     INTEGER NOT NULL,
    match_id        INTEGER REFERENCES matches(id) ON DELETE SET NULL,
    warning_code    TEXT    NOT NULL,
    field           TEXT    NOT NULL DEFAULT '',
    notes           TEXT    NOT NULL DEFAULT '',
    acknowledged_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_wca_season_week ON week_close_acknowledgments(season_id, week_number);

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

-- Season teams: explicit team participation per season, with name/captain snapshots
CREATE TABLE IF NOT EXISTS season_teams (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id   INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    team_id     INTEGER NOT NULL REFERENCES teams(id),
    season_name TEXT    NOT NULL DEFAULT '',  -- season-specific team name snapshot (editable draft)
    captain_id  INTEGER REFERENCES players(id),
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(season_id, team_id)
);

-- Season rosters: players assigned to a team for one season
-- UNIQUE(season_id, player_id) enforces one team per player per season
-- FK (season_id, team_id) ensures the team is registered in season_teams
CREATE TABLE IF NOT EXISTS season_rosters (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id  INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    team_id    INTEGER NOT NULL REFERENCES teams(id),
    player_id  INTEGER NOT NULL REFERENCES players(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(season_id, player_id),
    FOREIGN KEY (season_id, team_id) REFERENCES season_teams(season_id, team_id)
);

-- Trigger enforces the same FK on databases created before the FK was added.
CREATE TRIGGER IF NOT EXISTS trg_season_roster_team_exists
BEFORE INSERT ON season_rosters
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'team is not participating in this season')
    WHERE NOT EXISTS (
        SELECT 1 FROM season_teams
        WHERE season_id = NEW.season_id AND team_id = NEW.team_id
    );
END;

CREATE INDEX IF NOT EXISTS idx_players_team       ON players(team_id);
CREATE INDEX IF NOT EXISTS idx_teams_league       ON teams(league_id);
CREATE INDEX IF NOT EXISTS idx_seasons_league     ON seasons(league_id);
CREATE INDEX IF NOT EXISTS idx_matches_season     ON matches(season_id);
CREATE INDEX IF NOT EXISTS idx_results_match      ON match_results(match_id);
CREATE INDEX IF NOT EXISTS idx_results_player     ON match_results(player_id);
CREATE INDEX IF NOT EXISTS idx_hc_history_player  ON handicap_history(player_id);
CREATE INDEX IF NOT EXISTS idx_lineup_team_week   ON lineup_plans(team_id, week_number, season_id);
CREATE INDEX IF NOT EXISTS idx_season_teams_sid   ON season_teams(season_id);
CREATE INDEX IF NOT EXISTS idx_season_rosters_sid ON season_rosters(season_id);
CREATE INDEX IF NOT EXISTS idx_season_rosters_pid ON season_rosters(player_id);

-- Application users: identity for Apply attribution and, as of Users/Roles
-- Phase 1, real email+password browser login. username is a legacy
-- identifier (still NOT NULL UNIQUE for compatibility with every existing
-- row) but is never accepted by the password-login endpoint -- email is
-- the sole password-login identifier. API-key credentials live in the
-- separate user_api_keys table (see below), not on this row, so a
-- password-only user can exist with zero keys and a key-only legacy user
-- can exist with no password. A database created before this phase had an
-- additional api_key_hash TEXT NOT NULL UNIQUE column directly on users;
-- migrateUsersAndAPIKeys performs a one-time, guarded table rebuild that
-- moves those hashes into user_api_keys and drops that column, since
-- SQLite cannot relax a NOT NULL/UNIQUE constraint via ALTER TABLE.
CREATE TABLE IF NOT EXISTS users (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    username             TEXT    NOT NULL UNIQUE,
    role                 TEXT    NOT NULL DEFAULT 'admin',
    active               INTEGER NOT NULL DEFAULT 1,
    created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
    player_id            INTEGER REFERENCES players(id),
    email                TEXT,
    password_hash        TEXT,
    password_updated_at  DATETIME,
    must_reset_password  INTEGER NOT NULL DEFAULT 0,
    updated_at           DATETIME
);
-- idx_users_player_id and idx_users_email are created after
-- migrateUsersAndAPIKeys runs below, not here -- a pre-Phase-1 database's
-- users table already exists (so this CREATE TABLE is skipped) and has
-- neither an email column nor the final player_id placement yet, so an
-- index on either column would fail against it before the rebuild runs.

-- user_api_keys, role_assignments, sessions, and password_setup_tokens
-- (Users/Roles Phase 1 auth child tables, all FK-children of users(id))
-- are deliberately NOT created here. On a legacy database, users still
-- has its pre-Phase-1 shape at this point in migrate() (the CREATE TABLE
-- IF NOT EXISTS above is a no-op against an existing table) and is about
-- to be dropped and rebuilt by migrateUsersAndAPIKeys -- creating FK
-- children of it here, before that rebuild, would mean they briefly
-- reference a table that is about to be dropped. See
-- authChildTablesSchema below and migrateUsersAndAPIKeys's doc comment
-- for the required order: rebuild users FIRST, create these children
-- SECOND, against the final table.

-- Financial Phase 1: dues and payouts, both append-only history tables
-- (no update/delete path -- mirrors handicap_history's shape). "Paid" for
-- a player/season means at least one dues_payments row exists; there is
-- no partial-payment/balance math. team_id on dues_payments is a
-- denormalized snapshot of the player's roster team at payment time, so
-- history stays accurate even if the player's roster team changes later.
-- recorded_by_user_id is nullable with no FK constraint, matching the
-- existing attribution columns handicap_history.applied_by_user_id and
-- matches.approved_by_user_id.
CREATE TABLE IF NOT EXISTS dues_payments (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id           INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    player_id           INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    team_id             INTEGER REFERENCES teams(id),
    amount              REAL    NOT NULL,
    paid_at             DATE    NOT NULL,
    recorded_by_user_id INTEGER,
    note                TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_dues_payments_season_player ON dues_payments(season_id, player_id);

-- Payouts: admin-entered amounts per team per season. Standings are shown
-- for reference when recording a payout but are never used to compute the
-- amount automatically (Financial Phase 1 -- no payout formulas).
CREATE TABLE IF NOT EXISTS payouts (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id           INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    team_id             INTEGER NOT NULL REFERENCES teams(id),
    amount              REAL    NOT NULL,
    recorded_by_user_id INTEGER,
    note                TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_payouts_season_team ON payouts(season_id, team_id);
`
	if _, err := DB.Exec(schema); err != nil {
		return err
	}

	// Additive migrations for existing databases — errors are intentionally ignored
	// because the column may already exist (SQLite has no IF NOT EXISTS for ALTER).
	additiveMigrations := []string{
		`ALTER TABLE seasons ADD COLUMN schedule_type TEXT NOT NULL DEFAULT 'double_rr'`,
		`ALTER TABLE seasons ADD COLUMN num_weeks     INTEGER NOT NULL DEFAULT 0`,
		// Handicap snapshots on round_results (stores handicap values at match time)
		`ALTER TABLE round_results ADD COLUMN home_handicap_used REAL`,
		`ALTER TABLE round_results ADD COLUMN away_handicap_used REAL`,
		`ALTER TABLE round_results ADD COLUMN handicap_pts_used  INTEGER`,
		`ALTER TABLE round_results ADD COLUMN handicap_to        TEXT`,
		// Legacy / import fields
		`ALTER TABLE teams   ADD COLUMN team_number TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE players ADD COLUMN active      INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE players ADD COLUMN note        TEXT    NOT NULL DEFAULT ''`,
		// Schedule staleness flag: set when season_teams change after schedule generation
		`ALTER TABLE seasons ADD COLUMN schedule_stale INTEGER NOT NULL DEFAULT 0`,
		// teams_managed=1 marks new seasons as using explicit team management;
		// DEFAULT 0 keeps all pre-Phase-One seasons in legacy (bypass) mode.
		`ALTER TABLE seasons ADD COLUMN teams_managed INTEGER NOT NULL DEFAULT 0`,
		// activated_at is the persistent setup lock — set once on first activation.
		`ALTER TABLE seasons ADD COLUMN activated_at DATETIME`,
		// Close Week gate: set to 1 when the match's week has been officially closed.
		// Standings filter on week_closed=1 so only official results count.
		`ALTER TABLE matches ADD COLUMN week_closed INTEGER NOT NULL DEFAULT 0`,
		// Phase B: Apply endpoint audit columns on handicap_history.
		// All errors ignored — fresh DBs have these columns from the CREATE TABLE schema.
		`ALTER TABLE handicap_history ADD COLUMN apply_request_id     TEXT`,
		`ALTER TABLE handicap_history ADD COLUMN request_hash         TEXT`,
		`ALTER TABLE handicap_history ADD COLUMN player_name_snapshot TEXT`,
		`ALTER TABLE handicap_history ADD COLUMN season_id            INTEGER REFERENCES seasons(id)`,
		`ALTER TABLE handicap_history ADD COLUMN method               TEXT`,
		`ALTER TABLE handicap_history ADD COLUMN window_size          INTEGER`,
		`ALTER TABLE handicap_history ADD COLUMN window_racks         INTEGER`,
		`ALTER TABLE handicap_history ADD COLUMN lifetime_racks       INTEGER`,
		`ALTER TABLE handicap_history ADD COLUMN rec_token            TEXT`,
		`ALTER TABLE handicap_history ADD COLUMN applied_by_user_id   INTEGER`,
		// Idempotency index: one history row per (apply_request_id, player_id).
		// WHERE clause excludes legacy rows where apply_request_id IS NULL.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_hc_history_apply_idempotent
		 ON handicap_history(apply_request_id, player_id)
		 WHERE apply_request_id IS NOT NULL`,
		// Phase D: week_number links an apply batch to the recap week being processed.
		`ALTER TABLE handicap_history ADD COLUMN week_number INTEGER`,
		// Season-end clearance Phase 1: close season lifecycle columns.
		`ALTER TABLE seasons ADD COLUMN closed_at DATETIME`,
		`ALTER TABLE seasons ADD COLUMN final_standings_snapshot TEXT`,
		// Weekly Score Processing Phase 1A: match-level approval/processing state.
		// NULL means not-yet-approved / not-yet-processed. approved_by_user_id and
		// processed_by_user_id are nullable since admin-attested approval in this
		// phase does not require a personal-key user (mirrors handicap_history's
		// applied_by_user_id nullability for the same reason).
		`ALTER TABLE matches ADD COLUMN approved_at          DATETIME`,
		`ALTER TABLE matches ADD COLUMN approved_by_user_id  INTEGER`,
		`ALTER TABLE matches ADD COLUMN approval_note        TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE matches ADD COLUMN processed_at         DATETIME`,
		`ALTER TABLE matches ADD COLUMN processed_by_user_id INTEGER`,
		// Player Account Access Phase 1: optional one-to-one link from a user
		// to a player. NULL for every existing system_admin/league_admin user;
		// required (enforced at the application layer, not by NOT NULL, since
		// SQLite can't add a NOT NULL column without a default) for new
		// role=player users. The one-to-one part was already documented as
		// intended in doc/domains/users/README.md's "Provisional
		// Relationship" section before this phase; enforced below via a
		// partial unique index (SQLite's ALTER TABLE ADD COLUMN cannot carry
		// a UNIQUE constraint directly) rather than a UNIQUE column
		// constraint, so multiple NULLs (every non-player user) remain
		// allowed and only non-null values are required to be distinct.
		`ALTER TABLE users ADD COLUMN player_id INTEGER REFERENCES players(id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_player_id ON users(player_id) WHERE player_id IS NOT NULL`,
	}
	for _, stmt := range additiveMigrations {
		DB.Exec(stmt) // ignore error — column already exists on fresh DBs
	}

	// Clean up stale rule keys seeded by older versions (now handled by frontend defaults)
	DB.Exec(`DELETE FROM season_rules WHERE rule_key IN ('max_scoresheet_handicap','max_match_handicap')`)

	if err := migrateUsersAndAPIKeys(DB); err != nil {
		return fmt.Errorf("migrating users/api keys: %w", err)
	}

	// Safe to create unconditionally now: users has the final Phase-1 shape
	// either way -- freshly created above, or just rebuilt into it by
	// migrateUsersAndAPIKeys. migrateUsersAndAPIKeys also creates all of
	// these itself, against the rebuilt table, as part of its own
	// transaction on the legacy-database path; IF NOT EXISTS makes
	// re-creating them here harmless for that path and necessary for the
	// fresh-install path, which never goes through the rebuild at all (a
	// fresh database's users table has the final shape from the moment it
	// is created, so there is no ordering hazard to guard against here).
	if _, err := DB.Exec(authChildTablesSchema); err != nil {
		return fmt.Errorf("create auth child tables: %w", err)
	}
	if _, err := DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_player_id ON users(player_id) WHERE player_id IS NOT NULL`); err != nil {
		return fmt.Errorf("create idx_users_player_id: %w", err)
	}
	if _, err := DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE email IS NOT NULL`); err != nil {
		return fmt.Errorf("create idx_users_email: %w", err)
	}

	return nil
}

// authChildTablesSchema creates the four Users/Roles Phase 1 tables that
// are FK-children of users(id). Deliberately kept separate from the main
// schema string and only ever executed AFTER users has its final shape --
// either because it was just created fresh, or because migrateUsersAndAPIKeys
// already rebuilt it -- so these never reference a users table that is
// about to be dropped. migrateUsersAndAPIKeys creates its own copy of these
// (identical DDL) inside its rebuild transaction for the legacy-database
// path, since the role_assignments/user_api_keys backfill it performs must
// run in the same transaction as the rebuild; this copy's IF NOT EXISTS
// makes re-running it afterward a no-op there, and is what actually
// creates them on the fresh-install path.
const authChildTablesSchema = `
-- API credentials, separate from password credentials (Users/Roles Phase
-- 1). A user may have zero, one, or (in the future) several keys; "no
-- active key" is simply zero rows with revoked_at IS NULL, rather than a
-- sentinel value. key_hash stores SHA-256(cleartext_key) as 64-char
-- lowercase hex, exactly as the old users.api_key_hash column did -- the
-- cleartext key is still returned once at creation and never stored.
CREATE TABLE IF NOT EXISTS user_api_keys (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash    TEXT    NOT NULL UNIQUE,
    label       TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    revoked_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_user_api_keys_user ON user_api_keys(user_id);

-- Scoped role grants (Users/Roles Phase 1). A "player" identity is
-- deliberately NOT modeled here -- it comes exclusively from
-- users.player_id, a 1:1 identity link rather than a repeatable grant.
-- The CHECK constraint is the authoritative proof (not just a service-
-- layer convention) that system_admin is always global and league_admin
-- is always scoped to one league; the two partial unique indexes prove no
-- user can hold the same grant twice. This is a brand-new table, so it
-- can carry these constraints directly at creation -- no rebuild dance
-- like users.api_key_hash was needed.
CREATE TABLE IF NOT EXISTS role_assignments (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_code           TEXT    NOT NULL,
    league_id           INTEGER REFERENCES leagues(id) ON DELETE CASCADE,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_by_user_id  INTEGER REFERENCES users(id),
    CHECK (
        role_code IN ('system_admin','league_admin')
        AND (
            (role_code = 'system_admin' AND league_id IS NULL)
            OR (role_code = 'league_admin' AND league_id IS NOT NULL)
        )
    )
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_role_assignments_system_admin
    ON role_assignments(user_id) WHERE role_code = 'system_admin';
CREATE UNIQUE INDEX IF NOT EXISTS idx_role_assignments_league_admin
    ON role_assignments(user_id, league_id) WHERE role_code = 'league_admin';
CREATE INDEX IF NOT EXISTS idx_role_assignments_user ON role_assignments(user_id);

-- Browser sessions (Users/Roles Phase 1). token_hash is SHA-256 of the
-- opaque session cookie value; csrf_token_hash is SHA-256 of a second,
-- independently-generated value exposed to the browser as a separate,
-- JS-readable cookie (see doc/domains/users/README.md for the full
-- login/CSRF exchange) -- neither cleartext value is ever stored.
CREATE TABLE IF NOT EXISTS sessions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash        TEXT    NOT NULL UNIQUE,
    csrf_token_hash   TEXT    NOT NULL,
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at        DATETIME NOT NULL,
    revoked_at        DATETIME,
    user_agent        TEXT    NOT NULL DEFAULT '',
    ip_address        TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

-- One-time password setup tokens (Users/Roles Phase 1). There is no
-- self-service "forgot password" flow and no email delivery in this
-- phase: a system_admin generates a token here, shown once, and
-- communicates it out of band -- the same pattern already used for API
-- keys. Single-use (used_at) and revocable (revoked_at); issuing a new
-- token for a user revokes every other still-active one for that user.
CREATE TABLE IF NOT EXISTS password_setup_tokens (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT    NOT NULL UNIQUE,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at  DATETIME NOT NULL,
    used_at     DATETIME,
    revoked_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_password_setup_tokens_user ON password_setup_tokens(user_id);
`

// migrateUsersAndAPIKeys performs the one-time, guarded rebuild of `users`
// needed for Users/Roles Phase 1. A database created before this phase has
// an api_key_hash TEXT NOT NULL UNIQUE column directly on users; SQLite
// cannot relax a NOT NULL/UNIQUE constraint via ALTER TABLE, so a
// password-only user (no API key at all) cannot exist while that column
// remains. A brand-new database never has this column at all (the base
// CREATE TABLE above already omits it), so this is a no-op there --
// PRAGMA table_info(users) simply won't list api_key_hash and this
// function returns immediately.
//
// Sequence (deliberately in this order -- do not reorder; PM review of an
// earlier draft flagged that creating the auth child tables before this
// rebuild ran -- even though they were empty at that point -- relied on a
// temporal accident rather than a real guarantee, and contradicted this
// exact ordering):
//  1. Stash every existing api_key_hash value in a plain, foreign-key-less
//     temporary table. This must happen BEFORE user_api_keys exists: if
//     user_api_keys were created first as a child of the OLD users table
//     and that table were then dropped as part of the rebuild below, its
//     ON DELETE CASCADE could fire against the just-migrated rows and
//     silently destroy them. Keeping the stash table free of any foreign
//     key to `users` makes it immune to that risk regardless of exactly
//     how SQLite treats a DROP TABLE of an FK parent.
//  2. Rebuild `users` itself (create the new shape, copy every row's id/
//     username/role/active/created_at/player_id unchanged, drop the old
//     table, rename the new one into place) -- this is the one step in
//     this whole migration that is not a plain additive ALTER TABLE, since
//     removing a NOT NULL/UNIQUE constraint has no cheaper path in SQLite.
//     The old table's sqlite_sequence high-water mark is read before the
//     drop and restored onto the renamed table afterward (see the
//     "historical sequence" step below) -- copying only currently-existing
//     rows' ids is not enough, since a user deleted before migration could
//     have held a higher id than anything left to copy.
//  3. Only now create user_api_keys, role_assignments, sessions, and
//     password_setup_tokens, all as children of the freshly-rebuilt
//     `users` table -- none of them existed during step 2's drop, so none
//     were ever at risk. (sessions and password_setup_tokens have no data
//     to migrate into them, but are created here rather than left for the
//     caller so that every auth child table's creation follows the same
//     "only after users has its final shape" rule, with no exception.)
//  4. Backfill role_assignments from the legacy flat role column, and copy
//     the stashed hashes into user_api_keys.
//  5. Verify every stashed hash now resolves to an active (non-revoked)
//     user_api_keys row for the same user -- roll back the whole
//     migration rather than silently losing or duplicating a key.
//  6. Only after verification passes, drop the temporary stash table.
func migrateUsersAndAPIKeys(db *sql.DB) error {
	hasAPIKeyHash, err := columnExists(db, "users", "api_key_hash")
	if err != nil {
		return fmt.Errorf("checking users schema: %w", err)
	}
	if !hasAPIKeyHash {
		return nil // already migrated, or a fresh database that never had it
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE _migrate_api_key_hashes (
			user_id      INTEGER NOT NULL,
			api_key_hash TEXT    NOT NULL,
			created_at   DATETIME
		)`); err != nil {
		return fmt.Errorf("create stash table: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO _migrate_api_key_hashes (user_id, api_key_hash, created_at)
		SELECT id, api_key_hash, created_at FROM users`); err != nil {
		return fmt.Errorf("stash existing api key hashes: %w", err)
	}

	if _, err := tx.Exec(`
		CREATE TABLE users_new (
			id                   INTEGER PRIMARY KEY AUTOINCREMENT,
			username             TEXT    NOT NULL UNIQUE,
			role                 TEXT    NOT NULL DEFAULT 'admin',
			active               INTEGER NOT NULL DEFAULT 1,
			created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
			player_id            INTEGER REFERENCES players(id),
			email                TEXT,
			password_hash        TEXT,
			password_updated_at  DATETIME,
			must_reset_password  INTEGER NOT NULL DEFAULT 0,
			updated_at           DATETIME
		)`); err != nil {
		return fmt.Errorf("create users_new: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO users_new (id, username, role, active, created_at, player_id)
		SELECT id, username, role, active, created_at, player_id FROM users`); err != nil {
		return fmt.Errorf("copy users into users_new: %w", err)
	}

	// Read the OLD table's historical AUTOINCREMENT high-water mark before
	// dropping it. sqlite_sequence records the highest id ever inserted
	// into a table, independent of which rows currently still exist --
	// copying only currently-existing rows above would silently lose this
	// if the highest-id user had been deleted before migration ran, since
	// users_new's own sequence (set by the explicit-id INSERT above) only
	// reflects the max id actually copied. sql.ErrNoRows means the old
	// table never had any inserts recorded (a database with zero users),
	// which is not an error here.
	var oldSeq sql.NullInt64
	if err := tx.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'users'`).Scan(&oldSeq); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read historical users sequence: %w", err)
	}

	if _, err := tx.Exec(`DROP TABLE users`); err != nil {
		return fmt.Errorf("drop old users table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE users_new RENAME TO users`); err != nil {
		return fmt.Errorf("rename users_new to users: %w", err)
	}
	if oldSeq.Valid {
		// sqlite_sequence gets a row for a table lazily, on that table's
		// first AUTOINCREMENT insert -- not at CREATE TABLE time. If every
		// legacy user was deleted before migration ran, the copy above
		// inserts zero rows into users_new, so users_new (and therefore
		// the renamed users) has NO sqlite_sequence row at all yet, and a
		// plain UPDATE would silently match zero rows, losing the
		// historical high-water mark entirely (PM correction: the
		// original fix only handled "some users survived," not "every
		// user was deleted"). INSERT A row first when none exists yet,
		// then UPDATE to raise it when one already does (from a nonzero
		// copy) but is lower than the historical value -- covering both
		// cases with the same historical value either way.
		if _, err := tx.Exec(`
			INSERT INTO sqlite_sequence (name, seq)
			SELECT 'users', ? WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'users')
		`, oldSeq.Int64); err != nil {
			return fmt.Errorf("restore historical users sequence (insert): %w", err)
		}
		if _, err := tx.Exec(`UPDATE sqlite_sequence SET seq = ? WHERE name = 'users' AND seq < ?`, oldSeq.Int64, oldSeq.Int64); err != nil {
			return fmt.Errorf("restore historical users sequence (update): %w", err)
		}
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_player_id ON users(player_id) WHERE player_id IS NOT NULL`); err != nil {
		return fmt.Errorf("recreate idx_users_player_id: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE email IS NOT NULL`); err != nil {
		return fmt.Errorf("recreate idx_users_email: %w", err)
	}

	// Auth child tables, created now against the freshly-rebuilt users
	// table -- see this function's doc comment, step 3. Identical DDL to
	// authChildTablesSchema in migrate(); that copy's IF NOT EXISTS makes
	// re-running it after this transaction commits a no-op.
	if _, err := tx.Exec(authChildTablesSchema); err != nil {
		return fmt.Errorf("create auth child tables: %w", err)
	}

	// Backfill role_assignments from the legacy flat users.role column so
	// every existing API-key user keeps exactly the access they have
	// today -- without this, a pre-Phase-1 system_admin/league_admin would
	// resolve to an identity with zero role_assignments rows and lose all
	// authorization under the new centralized policy the moment it is
	// wired into any route. role='system_admin' or the legacy 'admin'
	// alias (which already satisfies the system_admin tier everywhere in
	// the old flat-role checks) both become one global system_admin row.
	// role='league_admin' becomes one row PER LEAGUE THAT EXISTS AT
	// MIGRATION TIME -- league_admin was global/unscoped under the old
	// model, and role_assignments' own CHECK constraint requires a
	// concrete league_id for league_admin (no nullable "all leagues"
	// escape hatch), so grandfathering per-existing-league is the closest
	// safe equivalent. A league created AFTER this migration is
	// deliberately NOT auto-granted to these grandfathered admins -- a
	// system_admin must explicitly extend scope to new leagues going
	// forward, a real, disclosed behavior change from the old unscoped
	// model. role='player' gets no row -- that identity already comes
	// from users.player_id. INSERT OR IGNORE makes this safe to run
	// against a users table that (in principle) already has some
	// role_assignments rows, though in practice this only ever runs
	// once per database, guarded by the api_key_hash check above.
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO role_assignments (user_id, role_code, league_id)
		SELECT id, 'system_admin', NULL FROM users WHERE role IN ('system_admin', 'admin')
	`); err != nil {
		return fmt.Errorf("backfill system_admin role_assignments: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO role_assignments (user_id, role_code, league_id)
		SELECT u.id, 'league_admin', l.id FROM users u, leagues l WHERE u.role = 'league_admin'
	`); err != nil {
		return fmt.Errorf("backfill league_admin role_assignments: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO user_api_keys (user_id, key_hash, label, created_at)
		SELECT user_id, api_key_hash, 'migrated', created_at FROM _migrate_api_key_hashes`); err != nil {
		return fmt.Errorf("copy hashes into user_api_keys: %w", err)
	}

	var mismatch int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM _migrate_api_key_hashes t
		WHERE NOT EXISTS (
			SELECT 1 FROM user_api_keys k
			WHERE k.user_id = t.user_id AND k.key_hash = t.api_key_hash AND k.revoked_at IS NULL
		)`).Scan(&mismatch); err != nil {
		return fmt.Errorf("verify migrated api keys: %w", err)
	}
	if mismatch != 0 {
		return fmt.Errorf("api key migration verification failed: %d hash(es) did not migrate correctly", mismatch)
	}

	if _, err := tx.Exec(`DROP TABLE _migrate_api_key_hashes`); err != nil {
		return fmt.Errorf("drop stash table: %w", err)
	}

	return tx.Commit()
}

// columnExists reports whether table has a column named column, using
// PRAGMA table_info -- the standard way to introspect a SQLite table's
// current shape without depending on driver-specific schema APIs.
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
