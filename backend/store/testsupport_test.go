package store

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/testdb"
)

// The store suite is the largest consumer of the test database: every
// integration test used to drop a schema and replay all 53 migrations, which
// costs ~0.75s per test and left one ~42-table schema behind per test, because
// nothing dropped a schema at the end of a run. A full run therefore left
// hundreds of schemas and a database too large for pg_dump to read (#670).
//
// The provisioning itself lives in backend/testdb, shared with backend/api:
// one migrated template database per test binary, one CREATE DATABASE ...
// TEMPLATE clone per test, and a drop in t.Cleanup. Every clone carries the
// same app schema (testdb.Schema, "vera_test"), because the name is baked into
// the trigger functions the migration chain creates; the template uses it
// rather than "public" so the restore drill can dump and restore a clone.
//
// The exception is a test that asserts how a migration behaves: it must replay
// the chain from zero, so it keeps a genuinely fresh scratch schema on the
// base database (resetTestSchema + loadMigrations) rather than a clone that is
// already migrated. Those live in migration_039_test.go, migration_044_test.go,
// the 031 backfill in purchase_orders_test.go and the 047 backfill in
// session_expiry_test.go, and they are the only callers of resetTestSchema.

const (
	// testDatabaseMaxIdentifier is Postgres's NAMEDATALEN-1. It is the shared
	// helper's cap; the constant stays here because the name test reads it.
	testDatabaseMaxIdentifier = testdb.MaxIdentifierLength
)

// testDatabase is one test's isolated Postgres database.
type testDatabase struct {
	// Store is the clone's store, configured with Schema: "public".
	Store *Store
	// DSN names the clone. Use it for a test that opens its own connection or
	// shells out (pg_dump); testDSN would point at the base database.
	DSN string
	// Database is the clone's physical name.
	Database string
	// Identity is the name the test asked for. Tests kept their existing
	// argument, so this is what a test body may still key on.
	Identity string
	// Schema is the app schema every clone carries, testdb.Schema.
	Schema string
}

// openTestSchema opens a fresh, fully migrated database for one test. The
// argument is the name the test used to ask for; it is now the identity used to
// name the clone, not a schema that gets created.
func openTestSchema(t *testing.T, identity string) *Store {
	t.Helper()
	return openTestDatabase(t, identity).Store
}

// openTestDatabase is openTestSchema plus the clone's DSN and name, for a test
// that needs to reach the same database outside the store.
func openTestDatabase(t *testing.T, identity string) *testDatabase {
	t.Helper()
	return openTestDatabaseWithTracer(t, identity, nil)
}

// openTestDatabaseWithTracer is openTestDatabase with a query tracer attached,
// for the tests that assert what a code path asks Postgres for.
func openTestDatabaseWithTracer(t *testing.T, identity string, tracer pgx.QueryTracer) *testDatabase {
	t.Helper()
	clone := testdb.Acquire(t, testDSN(t), identity, migrateTestDatabase)

	database, err := Open(context.Background(), Config{
		DSN:    clone.DSN,
		Schema: clone.Schema,
		Tracer: tracer,
	})
	if err != nil {
		t.Fatalf("open test database %s: %v", clone.Name, err)
	}
	t.Cleanup(database.Close)

	return &testDatabase{
		Store:    database,
		DSN:      clone.DSN,
		Database: clone.Name,
		Identity: clone.Identity,
		Schema:   clone.Schema,
	}
}

// migrateTestDatabase is the Migrate callback backend/testdb runs once per
// process, on the template database. It applies every shipped migration and
// seed through the same Open path production uses, then closes the pool so the
// template can be cloned.
func migrateTestDatabase(ctx context.Context, dsn string) error {
	template, err := Open(ctx, Config{DSN: dsn, Schema: testdb.Schema})
	if err != nil {
		return err
	}
	template.Close()
	return nil
}

// testStoreDSN is the DSN a store is connected with, for a test that opens its
// own backend to the same database (a second connection is how the deletion
// tests hold a lock the store must wait on).
func testStoreDSN(database *Store) string {
	return database.config.DSN
}

// openTracedSchema is openTestSchema with a tracer attached.
func openTracedSchema(t *testing.T, schema string) (*Store, *recordingTracer) {
	t.Helper()
	tracer := &recordingTracer{}
	return openTestDatabaseWithTracer(t, schema, tracer).Store, tracer
}

// testDSN returns the integration-test DSN, skipping the test when it is unset
// so the suite is still runnable without Postgres.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run Postgres integration tests")
	}
	return dsn
}

// updateDraft performs a real draft save for tests that exercise a rule other
// than optimistic concurrency: it fills the version the record currently
// carries when the test did not state one. The versioning rule itself is
// exercised through UpdatePurchaseOrderDraft directly by po_corrections_test.go.
func updateDraft(t *testing.T, database *Store, ctx context.Context, input PurchaseOrderUpdate) (PurchaseOrder, error) {
	t.Helper()
	if input.ExpectedVersion == 0 {
		current, err := database.GetPurchaseOrder(ctx, input.ID)
		if err != nil {
			return PurchaseOrder{}, err
		}
		input.ExpectedVersion = current.Version
	}
	return database.UpdatePurchaseOrderDraft(ctx, input)
}

// resetTestSchema drops one schema on the base database outright, migrations
// included. It is what a migration-behaviour test uses to replay the chain from
// zero, which is the one thing a clone cannot do; openTestSchema no longer
// calls it.
func resetTestSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect for schema reset: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`DROP SCHEMA IF EXISTS `+quoteIdentifier(schema)+` CASCADE`); err != nil {
		t.Fatalf("reset schema %s: %v", schema, err)
	}
}
