package store

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The store suite is the largest consumer of the test database: every
// integration test used to drop a schema and replay all 53 migrations, which
// costs ~0.75s per test and left one ~42-table schema behind per test, because
// nothing dropped a schema at the end of a run. A full run therefore left
// hundreds of schemas and a database too large for pg_dump to read (#670).
//
// Provisioning instead happens once per test process and once per test:
//
//   - a template database is created and migrated once per process (all 53
//     migrations and their seeds), then closed;
//   - each test gets a clone of it via CREATE DATABASE ... TEMPLATE, opened
//     with Schema: "public", which is already migrated;
//   - the clone is dropped in t.Cleanup, so a run leaves nothing behind.
//
// Measured on Postgres 18: a clone costs ~0.1s all in (create ~78ms, open an
// already-migrated clone ~31ms, drop 35-100ms) against ~0.75s for a schema
// replay.
//
// The exception is a test that asserts how a migration behaves: it must replay
// the chain from zero, so it keeps a genuinely fresh scratch schema on the base
// database (resetTestSchema + loadMigrations) rather than a clone that is
// already migrated. Those live in migration_039_test.go, migration_044_test.go,
// the 031 backfill in purchase_orders_test.go and the 047 backfill in
// session_expiry_test.go, and they are the only callers of resetTestSchema.

const (
	// testDatabaseMaxIdentifier is Postgres's NAMEDATALEN-1: longer identifiers
	// are truncated by the server, and two long schema names could then collide.
	testDatabaseMaxIdentifier = 63
	// testTemplateDatabase is the process-wide migrated template. It is named,
	// not random, so a run that crashed cannot leave an unreferenced template
	// behind: the next run drops and rebuilds it.
	testTemplateDatabase = "vera_store_test_template"
	// testDatabaseSchema is the schema every clone is configured with. The
	// template migrates "public", and a clone copies it wholesale.
	testDatabaseSchema = "public"
	// testDatabaseTimeout bounds the DDL a test performs on databases rather
	// than on rows, so a stuck lock fails the test instead of hanging the suite.
	testDatabaseTimeout = 2 * time.Minute
)

// testDatabase is one test's isolated Postgres database.
type testDatabase struct {
	// Store is the clone's store, configured with Schema: "public".
	Store *Store
	// DSN names the clone. Use it for a test that opens its own connection or
	// shells out (pg_dump); testDSN would point at the base database.
	DSN string
	// Database is the clone's physical name, derived from Identity and at most
	// 63 characters.
	Database string
	// Identity is the schema name the test asked for. Tests kept their existing
	// argument, so this is what a test body may still key on.
	Identity string
	// Schema is the store's schema, always "public" on a clone.
	Schema string
}

// testDatabases carries the process-wide template and the admin connection used
// to create and drop clones. A test binary is one process, so one template
// serves every test in it.
var testDatabases struct {
	once   sync.Once
	admin  *pgxpool.Pool
	err    error
	builds int // how many templates this process built; a test asserts it is 1
}

// openTestSchema opens a fresh, fully migrated database for one test. The
// argument is the schema name the test used to ask for; it is now the identity
// used to name the clone, not a schema that gets created.
func openTestSchema(t *testing.T, schema string) *Store {
	t.Helper()
	return openTestDatabase(t, schema).Store
}

// openTestDatabase is openTestSchema plus the clone's DSN and name, for a test
// that needs to reach the same database outside the store.
func openTestDatabase(t *testing.T, schema string) *testDatabase {
	t.Helper()
	return openTestDatabaseWithTracer(t, schema, nil)
}

// openTestDatabaseWithTracer is openTestDatabase with a query tracer attached,
// for the tests that assert what a code path asks Postgres for.
func openTestDatabaseWithTracer(t *testing.T, schema string, tracer pgx.QueryTracer) *testDatabase {
	t.Helper()
	dsn := testDSN(t)
	admin, err := testDatabaseAdmin(dsn)
	if err != nil {
		t.Fatalf("test database template: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testDatabaseTimeout)
	defer cancel()

	name := testDatabaseName(schema)
	cloneDSN, err := dsnForDatabase(dsn, name)
	if err != nil {
		t.Fatalf("test database %s: %v", name, err)
	}
	// A run that was interrupted can leave its clone behind; dropping first
	// keeps a rerun on a warm database behaving like the first run, exactly as
	// the schema reset did.
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(name)+" WITH (FORCE)"); err != nil {
		t.Fatalf("drop the previous test database %s: %v", name, err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoteIdentifier(name)+" TEMPLATE "+quoteIdentifier(testTemplateDatabase)); err != nil {
		t.Fatalf("clone the test database template into %s: %v", name, err)
	}

	database, err := Open(context.Background(), Config{DSN: cloneDSN, Schema: testDatabaseSchema, Tracer: tracer})
	if err != nil {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(name)+" WITH (FORCE)")
		t.Fatalf("open test database %s: %v", name, err)
	}

	handle := &testDatabase{
		Store:    database,
		DSN:      cloneDSN,
		Database: name,
		Identity: schema,
		Schema:   testDatabaseSchema,
	}
	t.Cleanup(func() {
		database.Close()
		// Cleanup has no live t.Context; the drop gets its own deadline so a
		// stuck backend still names the database it blocked.
		dropCtx, cancel := context.WithTimeout(context.Background(), testDatabaseTimeout)
		defer cancel()
		if _, err := admin.Exec(dropCtx, "DROP DATABASE IF EXISTS "+quoteIdentifier(name)+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database %s: %v", name, err)
		}
	})
	return handle
}

// testDatabaseAdmin returns the process-wide admin connection, building the
// template first if this process has not done so. The error is cached, so a
// missing CREATEDB privilege fails every test with the same sentence instead of
// falling back to a schema replay.
func testDatabaseAdmin(dsn string) (*pgxpool.Pool, error) {
	testDatabases.once.Do(func() {
		testDatabases.err = buildTestDatabaseTemplate(dsn)
	})
	if testDatabases.err != nil {
		return nil, testDatabases.err
	}
	return testDatabases.admin, nil
}

// buildTestDatabaseTemplate creates and migrates the template database once for
// the process, then closes it: a database with a live connection cannot be
// cloned.
func buildTestDatabaseTemplate(dsn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), testDatabaseTimeout)
	defer cancel()

	templateDSN, err := dsnForDatabase(dsn, testTemplateDatabase)
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to the test database: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	var role string
	var canCreate bool
	if err := conn.QueryRow(ctx,
		`SELECT current_user, rolsuper OR rolcreatedb FROM pg_roles WHERE rolname = current_user`,
	).Scan(&role, &canCreate); err != nil {
		return fmt.Errorf("read the test role's privileges: %w", err)
	}
	if !canCreate {
		return testDatabaseCreatedbError(role)
	}

	if _, err := conn.Exec(ctx,
		"DROP DATABASE IF EXISTS "+quoteIdentifier(testTemplateDatabase)+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("drop the previous test database template: %w", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+quoteIdentifier(testTemplateDatabase)); err != nil {
		return fmt.Errorf("create the test database template: %w", err)
	}

	template, err := Open(ctx, Config{DSN: templateDSN, Schema: testDatabaseSchema})
	if err != nil {
		return fmt.Errorf("migrate the test database template: %w", err)
	}
	template.Close()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open the test database admin connection: %w", err)
	}
	testDatabases.admin = admin
	testDatabases.builds++
	return nil
}

// testDatabaseCreatedbError names the privilege that is missing rather than
// letting CREATE DATABASE fail with a bare SQLSTATE, and it never suggests
// falling back to the schema replay: that path is what this change removes.
func testDatabaseCreatedbError(role string) error {
	return fmt.Errorf(
		"the store test databases need a Postgres role that can CREATE DATABASE, but %q cannot: "+
			"run `ALTER ROLE %q CREATEDB` as a superuser, or point VERA_POSTGRES_TEST_DSN at a role that can",
		role, role)
}

// testDatabaseName derives the clone's name from the schema name the test used
// to ask for. Postgres identifiers stop at 63 characters and several of these
// names are 44 characters already, so a name carries a stable digest of the full
// schema: two different schemas never share a clone, and rerunning a test
// derives the same name it used before.
func testDatabaseName(schema string) string {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(schema))
	suffix := "_" + hex.EncodeToString(digest.Sum(nil)[:4])

	var builder strings.Builder
	for _, character := range strings.ToLower(schema) {
		switch {
		case character >= 'a' && character <= 'z',
			character >= '0' && character <= '9',
			character == '_':
			builder.WriteRune(character)
		default:
			builder.WriteByte('_')
		}
	}
	base := builder.String()
	if base == "" {
		base = "vera_test"
	}
	if limit := testDatabaseMaxIdentifier - len(suffix); len(base) > limit {
		base = base[:limit]
	}
	return base + suffix
}

// dsnForDatabase is the same connection string with another database name. The
// result carries the credentials, so it is never logged.
//
// It rewrites the string rather than pgxpool.Config.ConnString(): that method
// returns the string it parsed, so mutating ConnConfig.Database and asking for
// the string back yields the base DSN unchanged -- every clone would then be the
// base database.
func dsnForDatabase(dsn, database string) (string, error) {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return "", fmt.Errorf("VERA_POSTGRES_TEST_DSN is empty")
	}
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("parse VERA_POSTGRES_TEST_DSN: %w", err)
		}
		parsed.Path = "/" + database
		parsed.RawPath = ""
		return parsed.String(), nil
	}

	// libpq keyword/value form. `dbname` and `database` are the same setting,
	// and a DSN may name neither (libpq then uses the role name), so a missing
	// key is appended rather than assumed present.
	if keywordDatabasePattern.MatchString(trimmed) {
		return keywordDatabasePattern.ReplaceAllString(trimmed, "${1}database="+database), nil
	}
	return trimmed + " database=" + database, nil
}

// keywordDatabasePattern matches the leading whitespace and the database setting
// in a libpq keyword/value DSN. The value is either a single-quoted string or a
// bare word.
var keywordDatabasePattern = regexp.MustCompile(`(?i)(^|\s)(database|dbname)\s*=\s*('(?:[^']|'')*'|[^\s]+)`)

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
