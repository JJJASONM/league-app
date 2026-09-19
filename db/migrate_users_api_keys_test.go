package db_test

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	"league_app/db"

	_ "modernc.org/sqlite"
)

// TestMigrateUsersAndAPIKeys_FromPrePhaseSchema seeds a database containing
// ONLY the exact pre-Users/Roles-Phase-1 `users` shape (api_key_hash TEXT
// NOT NULL UNIQUE directly on the row) with real data, before db.Init ever
// runs. migrate()'s CREATE TABLE IF NOT EXISTS leaves this pre-seeded table
// alone and creates every other table fresh, so the guarded rebuild
// migration is exercised against the actual historical schema, not a
// synthetic approximation of it -- and, critically, against a database
// where role_assignments/sessions/password_setup_tokens/user_api_keys (all
// freshly created, empty, with an ON DELETE CASCADE FK to users) already
// exist by the time the rebuild's DROP TABLE users runs, which is exactly
// the scenario the PM's migration-safety review was concerned about.
func TestMigrateUsersAndAPIKeys_FromPrePhaseSchema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "league.db")

	seed, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if _, err := seed.Exec(`
		CREATE TABLE users (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			username     TEXT    NOT NULL UNIQUE,
			api_key_hash TEXT    NOT NULL UNIQUE,
			role         TEXT    NOT NULL DEFAULT 'admin',
			active       INTEGER NOT NULL DEFAULT 1,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		t.Fatalf("seed old users table: %v", err)
	}
	if _, err := seed.Exec(`ALTER TABLE users ADD COLUMN player_id INTEGER REFERENCES players(id)`); err != nil {
		t.Fatalf("seed player_id column: %v", err)
	}
	// Two pre-existing leagues -- needed to prove the legacy league_admin
	// backfill grandfathers a role_assignments row per league that already
	// existed at migration time (role_assignments' own CHECK constraint
	// requires a concrete league, so there is no nullable "all leagues"
	// equivalent to fall back to).
	if _, err := seed.Exec(`
		CREATE TABLE leagues (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL UNIQUE,
			game_format TEXT NOT NULL DEFAULT '8ball',
			day_of_week TEXT,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		t.Fatalf("seed leagues table: %v", err)
	}
	if _, err := seed.Exec(`INSERT INTO leagues (name) VALUES ('Pre-Existing League 1'), ('Pre-Existing League 2')`); err != nil {
		t.Fatalf("seed leagues: %v", err)
	}

	type oldUser struct {
		username, hash, role string
		active               int
	}
	seeds := []oldUser{
		{"alice-admin", "hash-alice-aaaa", "system_admin", 1},
		{"bob-league", "hash-bob-bbbb", "league_admin", 1},
		{"carol-legacy", "hash-carol-cccc", "admin", 1},
		{"dave-inactive", "hash-dave-dddd", "league_admin", 0},
	}
	var seededIDs []int64
	for _, su := range seeds {
		res, err := seed.Exec(`INSERT INTO users (username, api_key_hash, role, active) VALUES (?,?,?,?)`,
			su.username, su.hash, su.role, su.active)
		if err != nil {
			t.Fatalf("seed user %s: %v", su.username, err)
		}
		id, _ := res.LastInsertId()
		seededIDs = append(seededIDs, id)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init (runs migrateUsersAndAPIKeys): %v", err)
	}

	// 1. Every seeded id, username, role, active, and (absence of) email is preserved exactly.
	for i, su := range seeds {
		var username, role string
		var active int
		var email sql.NullString
		err := db.DB.QueryRow(`SELECT username, role, active, email FROM users WHERE id=?`, seededIDs[i]).
			Scan(&username, &role, &active, &email)
		if err != nil {
			t.Fatalf("row for id %d missing after migration: %v", seededIDs[i], err)
		}
		if username != su.username || role != su.role || active != su.active {
			t.Errorf("user %d: want (%s,%s,%d), got (%s,%s,%d)",
				seededIDs[i], su.username, su.role, su.active, username, role, active)
		}
		if email.Valid {
			t.Errorf("user %d: want email NULL for a pre-existing legacy user, got %q", seededIDs[i], email.String)
		}
	}

	// 1b. Legacy role backfill: system_admin/admin (alice, carol) each get
	// exactly one global (league_id NULL) system_admin role_assignments
	// row; league_admin (bob) gets one row PER pre-existing league; the
	// inactive league_admin (dave) still gets backfilled too (backfill is
	// about preserving what the row WAS entitled to, independent of
	// active state, matching the migration's "preserve every row exactly"
	// principle -- ResolveApplyUserByAPIKey's own active=1 check is what
	// actually keeps an inactive account from authenticating, not the
	// presence or absence of role_assignments rows).
	assertGlobalSystemAdmin := func(userID int64, label string) {
		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM role_assignments WHERE user_id=? AND role_code='system_admin' AND league_id IS NULL`, userID).Scan(&count); err != nil {
			t.Fatalf("count system_admin role_assignments for %s: %v", label, err)
		}
		if count != 1 {
			t.Errorf("%s: want exactly 1 global system_admin role_assignments row, got %d", label, count)
		}
	}
	assertGlobalSystemAdmin(seededIDs[0], "alice (system_admin)")
	assertGlobalSystemAdmin(seededIDs[2], "carol (legacy admin alias)")

	assertLeagueAdminPerExistingLeague := func(userID int64, label string) {
		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM role_assignments WHERE user_id=? AND role_code='league_admin'`, userID).Scan(&count); err != nil {
			t.Fatalf("count league_admin role_assignments for %s: %v", label, err)
		}
		if count != 2 {
			t.Errorf("%s: want exactly 2 league_admin role_assignments rows (one per pre-existing league), got %d", label, count)
		}
	}
	assertLeagueAdminPerExistingLeague(seededIDs[1], "bob (league_admin)")
	assertLeagueAdminPerExistingLeague(seededIDs[3], "dave (inactive league_admin)")

	// 2. Every old api_key_hash migrated exactly once into user_api_keys,
	//    resolving to the same user_id, with no loss or duplication.
	for i, su := range seeds {
		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM user_api_keys WHERE user_id=? AND key_hash=? AND revoked_at IS NULL`,
			seededIDs[i], su.hash).Scan(&count); err != nil {
			t.Fatalf("query user_api_keys for %d: %v", seededIDs[i], err)
		}
		if count != 1 {
			t.Errorf("user %d: want exactly 1 active user_api_keys row for hash %q, got %d", seededIDs[i], su.hash, count)
		}
	}
	var totalKeys int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM user_api_keys`).Scan(&totalKeys); err != nil {
		t.Fatalf("count user_api_keys: %v", err)
	}
	if totalKeys != len(seeds) {
		t.Errorf("want exactly %d user_api_keys rows total (no duplication), got %d", len(seeds), totalKeys)
	}

	// 3. The temporary stash table is gone.
	var stashExists int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='_migrate_api_key_hashes'`).
		Scan(&stashExists); err != nil {
		t.Fatalf("check stash table: %v", err)
	}
	if stashExists != 0 {
		t.Error("temporary stash table _migrate_api_key_hashes was not dropped")
	}

	// 4. The old api_key_hash column is gone from the rebuilt table.
	if hasCol, err := testColumnExists(db.DB, "users", "api_key_hash"); err != nil {
		t.Fatalf("check api_key_hash column: %v", err)
	} else if hasCol {
		t.Error("want api_key_hash column removed from rebuilt users table")
	}

	// 5. role_assignments (created empty, with an ON DELETE CASCADE FK to
	//    users, before the rebuild's DROP TABLE users ran) still correctly
	//    references the renamed table -- a fresh insert against it must
	//    succeed and be visible. Using bob (already backfilled with 2
	//    league_admin rows) and granting a DIFFERENT role (system_admin,
	//    which he doesn't already hold) avoids colliding with the partial
	//    unique index on the backfilled rows.
	if _, err := db.DB.Exec(`INSERT INTO role_assignments (user_id, role_code) VALUES (?, 'system_admin')`, seededIDs[1]); err != nil {
		t.Fatalf("insert role_assignments referencing a migrated user id: %v", err)
	}
	var raCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM role_assignments WHERE user_id=?`, seededIDs[1]).Scan(&raCount); err != nil {
		t.Fatalf("count role_assignments: %v", err)
	}
	if raCount != 3 { // 2 backfilled league_admin rows + 1 just-inserted system_admin row
		t.Errorf("want 3 role_assignments rows for user %d, got %d", seededIDs[1], raCount)
	}

	// 6. Deleting that user cascades ALL of their role_assignments rows,
	//    proving the FK is live post-rebuild, not just schema text
	//    pointing at a table that no longer functionally enforces
	//    anything.
	if _, err := db.DB.Exec(`DELETE FROM users WHERE id=?`, seededIDs[1]); err != nil {
		t.Fatalf("delete migrated user: %v", err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM role_assignments WHERE user_id=?`, seededIDs[1]).Scan(&raCount); err != nil {
		t.Fatalf("count role_assignments after delete: %v", err)
	}
	if raCount != 0 {
		t.Errorf("want all role_assignments cascade-deleted with their user, got %d rows remaining", raCount)
	}

	// 7. AUTOINCREMENT sequence continues correctly -- a newly inserted row
	//    gets an id greater than every migrated id, with no collision.
	var maxSeeded int64
	for _, id := range seededIDs {
		if id > maxSeeded {
			maxSeeded = id
		}
	}
	res, err := db.DB.Exec(`INSERT INTO users (username, role, active) VALUES ('post-migration-user', 'league_admin', 1)`)
	if err != nil {
		t.Fatalf("insert post-migration user: %v", err)
	}
	newID, _ := res.LastInsertId()
	if newID <= maxSeeded {
		t.Errorf("want new user id > %d (max migrated id), got %d -- autoincrement sequence was not preserved", maxSeeded, newID)
	}

	if err := db.DB.Close(); err != nil {
		t.Fatalf("close db before rerun: %v", err)
	}

	// 8. Re-running migration (a second full db.Init on the same directory)
	//    is idempotent -- no error, no data loss, no duplication.
	if err := db.Init(dir); err != nil {
		t.Fatalf("second db.Init (idempotency check): %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var totalKeysAfterRerun int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM user_api_keys`).Scan(&totalKeysAfterRerun); err != nil {
		t.Fatalf("count user_api_keys after rerun: %v", err)
	}
	// One key was deleted along with seededIDs[0] in step 6 (cascade), and
	// one new key-less user was added in step 7 -- so the expected count
	// after rerun is the same as after step 6/7, not the original totalKeys.
	if totalKeysAfterRerun != totalKeys-1 {
		t.Errorf("re-running migration changed user_api_keys row count unexpectedly: want %d, got %d", totalKeys-1, totalKeysAfterRerun)
	}
}

// TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceAfterHighIDDeletion
// is the exact regression PM's review called for: copying only
// currently-existing rows' ids into the rebuilt table is not enough to
// preserve AUTOINCREMENT history, because SQLite's sqlite_sequence tracks
// the highest id ever inserted into a table, not the current max row --
// if the highest-id user was deleted before migration ran, the naive
// rebuild would let a future insert reuse that old, already-referenced id.
func TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceAfterHighIDDeletion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "league.db")

	seed, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if _, err := seed.Exec(`
		CREATE TABLE users (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			username     TEXT    NOT NULL UNIQUE,
			api_key_hash TEXT    NOT NULL UNIQUE,
			role         TEXT    NOT NULL DEFAULT 'admin',
			active       INTEGER NOT NULL DEFAULT 1,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		t.Fatalf("seed old users table: %v", err)
	}
	if _, err := seed.Exec(`ALTER TABLE users ADD COLUMN player_id INTEGER REFERENCES players(id)`); err != nil {
		t.Fatalf("seed player_id column: %v", err)
	}

	// Insert users up through a high id, then delete the highest-id one --
	// sqlite_sequence still remembers that high-water mark even though no
	// row with that id currently exists.
	var highestID int64
	for i := 1; i <= 5; i++ {
		res, err := seed.Exec(`INSERT INTO users (username, api_key_hash, role, active) VALUES (?,?,?,1)`,
			"user"+strconv.Itoa(i), "hash-"+strconv.Itoa(i), "admin")
		if err != nil {
			t.Fatalf("seed user %d: %v", i, err)
		}
		highestID, _ = res.LastInsertId()
	}
	if _, err := seed.Exec(`DELETE FROM users WHERE id = ?`, highestID); err != nil {
		t.Fatalf("delete highest-id user: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init (runs migrateUsersAndAPIKeys): %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	res, err := db.DB.Exec(`INSERT INTO users (username, role, active) VALUES ('post-migration-user', 'admin', 1)`)
	if err != nil {
		t.Fatalf("insert post-migration user: %v", err)
	}
	newID, _ := res.LastInsertId()
	if newID <= highestID {
		t.Errorf("want new user id > %d (historical high-water mark, even though that row was deleted before migration), got %d -- sqlite_sequence history was not preserved", highestID, newID)
	}
}

// TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceWhenAllUsersDeleted
// is the second regression PM's review called for: when EVERY legacy user
// is deleted before migration runs, the rebuild's
// "INSERT INTO users_new ... SELECT ... FROM users" copies ZERO rows, so
// users_new never receives an AUTOINCREMENT insert at all -- sqlite_sequence
// only gets a row for a table lazily, on its first insert, not at CREATE
// TABLE time. A plain UPDATE against a nonexistent sqlite_sequence row is a
// silent no-op, losing the historical high-water mark completely. This is
// distinct from (and more extreme than) the "highest user deleted but lower
// users remain" case above, where the copy still produces at least one row
// and therefore an existing sqlite_sequence row to update.
func TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceWhenAllUsersDeleted(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "league.db")

	seed, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if _, err := seed.Exec(`
		CREATE TABLE users (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			username     TEXT    NOT NULL UNIQUE,
			api_key_hash TEXT    NOT NULL UNIQUE,
			role         TEXT    NOT NULL DEFAULT 'admin',
			active       INTEGER NOT NULL DEFAULT 1,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		t.Fatalf("seed old users table: %v", err)
	}
	if _, err := seed.Exec(`ALTER TABLE users ADD COLUMN player_id INTEGER REFERENCES players(id)`); err != nil {
		t.Fatalf("seed player_id column: %v", err)
	}

	var highestID int64
	for i := 1; i <= 5; i++ {
		res, err := seed.Exec(`INSERT INTO users (username, api_key_hash, role, active) VALUES (?,?,?,1)`,
			"user"+strconv.Itoa(i), "hash-"+strconv.Itoa(i), "admin")
		if err != nil {
			t.Fatalf("seed user %d: %v", i, err)
		}
		highestID, _ = res.LastInsertId()
	}
	// Delete EVERY user, not just the highest-id one -- the rebuild's copy
	// step will have nothing at all to copy.
	if _, err := seed.Exec(`DELETE FROM users`); err != nil {
		t.Fatalf("delete all users: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init (runs migrateUsersAndAPIKeys): %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count users after migration: %v", err)
	}
	if count != 0 {
		t.Fatalf("want zero users after migrating an emptied table, got %d", count)
	}

	res, err := db.DB.Exec(`INSERT INTO users (username, role, active) VALUES ('post-migration-user', 'admin', 1)`)
	if err != nil {
		t.Fatalf("insert post-migration user: %v", err)
	}
	newID, _ := res.LastInsertId()
	if newID <= highestID {
		t.Errorf("want new user id > %d (historical high-water mark, even though users_new received zero copied rows), got %d -- sqlite_sequence history was not preserved for an emptied table", highestID, newID)
	}
}

func testColumnExists(conn *sql.DB, table, column string) (bool, error) {
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
