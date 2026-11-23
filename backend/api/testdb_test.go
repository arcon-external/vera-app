package api

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/store"
	"vera-app/backend/testdb"
)

// The API suite provisions its databases through backend/testdb, shared with
// backend/store: one migrated template database per test binary, one clone per
// test, dropped in t.Cleanup. A clone carries the migrated tables under the
// test's own schema name, so a test that embeds its identity in SQL still reads
// rows from the clone.
//
// The helpers below keep the names and signatures every test already calls.
// testDatabase is the one seam behind them, and it caches per test and
// identity: a test that boots a server and then opens a second store for direct
// assertions reaches one database, not two empty copies of the same schema.

// testDatabase returns the test's clone for identity, creating the process
// template and the clone on first use. The argument is the name the test used
// to ask for; it is now the identity used to name the clone, not a schema that
// gets created.
func testDatabase(tb testing.TB, identity string) *testdb.Database {
	tb.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		tb.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	return testdb.Acquire(tb, dsn, identity, migrateTestDatabase)
}

// migrateTestDatabase is the Migrate callback backend/testdb runs once per
// process, on the template database. It applies every shipped migration and
// seed through the same Open path production uses, then closes the pool so the
// template can be cloned.
func migrateTestDatabase(ctx context.Context, dsn string) error {
	template, err := store.Open(ctx, store.Config{DSN: dsn, Schema: testdb.Schema})
	if err != nil {
		return err
	}
	template.Close()
	return nil
}

// openTestDatabaseStore opens a store on the test's clone. A test may call it
// more than once; every store lands on the same clone.
func openTestDatabaseStore(tb testing.TB, identity string) *store.Store {
	tb.Helper()
	clone := testDatabase(tb, identity)
	database, err := store.Open(context.Background(), store.Config{
		DSN:    clone.DSN,
		Schema: clone.Schema,
	})
	if err != nil {
		tb.Fatalf("open test store %s: %v", clone.Name, err)
	}
	tb.Cleanup(database.Close)
	return database
}

// resetTestSchema keeps the name the setups used to call before a clone
// existed. A clone is fresh by construction, so this only acquires it; there is
// no schema to drop, and dropping the clone's migrated schema would delete the
// tables it exists to provide.
func resetTestSchema(t *testing.T, identity string) {
	t.Helper()
	testDatabase(t, identity)
}

// openTestStore opens the clone's store for direct assertions and seeding.
func openTestStore(t *testing.T, identity string) *store.Store {
	t.Helper()
	return openTestDatabaseStore(t, identity)
}

// testSchemaIdent is the quoted app schema every clone carries, for fixtures
// that used to embed a schema name in SQL.
func testSchemaIdent() string {
	return quoteIdentForTest(testdb.Schema)
}

// TestAPITestHelpersShareOneClone pins the property the setup helpers rely on:
// a test that boots a server and then opens a second store for direct
// assertions must reach one database. Two clones would give the test two empty
// copies of the same schema, so a row written through one handle would be
// invisible through the other and the assertion would pass against nothing.
func TestAPITestHelpersShareOneClone(t *testing.T) {
	const identity = "vera_probe_api_shared"
	_, database, _ := adminTestSetup(t, identity)
	direct := openTestStore(t, identity)

	if database.DSN() != direct.DSN() {
		t.Fatalf("the server and the direct store opened different databases; they cannot share data")
	}

	clone := testDatabase(t, identity)
	opened, err := pgxpool.ParseConfig(database.DSN())
	if err != nil {
		t.Fatalf("parse the server's DSN: %v", err)
	}
	if opened.ConnConfig.Database != clone.Name {
		t.Fatalf("the server's store points at %q, not the test's clone %q; the restore drill would dump every test",
			opened.ConnConfig.Database, clone.Name)
	}

	ctx := context.Background()
	if _, err := direct.CreateUser(ctx, "probe-shared@example.com", "Probe", "test-hash", "staff"); err != nil {
		t.Fatalf("write through the direct store: %v", err)
	}
	if _, err := database.GetUserByEmail(ctx, "probe-shared@example.com"); err != nil {
		t.Fatalf("the server's store cannot see the row the direct store wrote: %v", err)
	}
}

// TestAPITestDatabaseIsDroppedAfterItsTest observes the database itself rather
// than the helper's bookkeeping: a cleanup that forgot the drop would leave one
// database per test behind, which is exactly the accumulation #670 measured.
func TestAPITestDatabaseIsDroppedAfterItsTest(t *testing.T) {
	var name string
	t.Run("inside the test", func(t *testing.T) {
		name = testDatabase(t, "vera_probe_api_dropped").Name
	})
	t.Run("after its cleanup", func(t *testing.T) {
		dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatalf("connect to check %s: %v", name, err)
		}
		defer func() { _ = conn.Close(context.Background()) }()
		var exists bool
		if err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
			t.Fatalf("read pg_database for %s: %v", name, err)
		}
		if exists {
			t.Errorf("database %q still exists after its test finished; a run would grow without bound", name)
		}
	})
}
