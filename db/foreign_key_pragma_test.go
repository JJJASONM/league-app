package db_test

import (
	"context"
	"database/sql"
	"testing"

	"league_app/db"
)

// TestForeignKeysPragma_EnabledOnEveryPooledConnection guards Known Gap #19:
// SQLite's foreign_keys pragma is a per-connection setting (SQLite does not
// persist it in the database file, unlike journal_mode). db.Init previously
// applied "PRAGMA foreign_keys=ON" via a single *sql.DB.Exec call, which
// database/sql satisfies using exactly one underlying driver connection --
// any other connection the pool later opens (e.g. under concurrent load)
// defaulted to SQLite's own foreign_keys=OFF default, silently disabling
// every ON DELETE CASCADE/SET NULL clause in the schema for that connection.
//
// This test deliberately checks out more than one connection at once via
// *sql.DB.Conn (which forces the pool to open a genuinely new underlying
// connection whenever no idle one is available, unlike plain Query/Exec
// calls which may reuse the same connection the whole test) and queries
// PRAGMA foreign_keys directly on each -- so it exercises connections other
// than whichever one db.Init happened to run its startup pragmas on, and
// would fail under the old startup-only PRAGMA behavior.
func TestForeignKeysPragma_EnabledOnEveryPooledConnection(t *testing.T) {
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	ctx := context.Background()

	const n = 3
	conns := make([]*sql.Conn, 0, n)
	for i := 0; i < n; i++ {
		c, err := db.DB.Conn(ctx)
		if err != nil {
			t.Fatalf("checkout connection %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})

	for i, c := range conns {
		var enabled int
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			t.Fatalf("connection %d: PRAGMA foreign_keys: %v", i, err)
		}
		if enabled != 1 {
			t.Errorf("connection %d: want PRAGMA foreign_keys=1, got %d", i, enabled)
		}
	}
}
