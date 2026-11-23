package store

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestEachTestGetsItsOwnMigratedClone pins the contract of the provisioning
// helper the whole store suite funnels through:
//
//   - two tests see different data, even when they write the same rows;
//   - every clone is already fully migrated, so a test body never pays for the
//     migration chain;
//   - the clone is gone once the test that asked for it has finished.
//
// It observes the databases themselves rather than which SQL the helper ran: a
// clone that leaked one test's rows into another would pass any internal
// assertion and still be wrong.
func TestEachTestGetsItsOwnMigratedClone(t *testing.T) {
	dsn := testDSN(t)
	var first testDatabase

	t.Run("first test", func(t *testing.T) {
		first = *openTestDatabase(t, "vera_probe_isolation_a")
		ctx := context.Background()

		// The clone carries the shipped seed: five client rows from migration
		// 011. A clone of an unmigrated database would have no table at all.
		var seeded int
		if err := first.Store.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM public.customers WHERE customer_code BETWEEN 2 AND 6`).Scan(&seeded); err != nil {
			t.Fatalf("read the migrated seed: %v", err)
		}
		if seeded != 5 {
			t.Fatalf("seeded clients = %d; want the five migration 011 rows", seeded)
		}

		if _, err := first.Store.pool.Exec(ctx,
			`INSERT INTO public.customers (customer_code, legal_name) VALUES (99, 'PT. First Test')`); err != nil {
			t.Fatalf("write the first test's row: %v", err)
		}

		second := openTestDatabase(t, "vera_probe_isolation_b")
		if second.Database == first.Database {
			t.Fatalf("both tests were given the database %q; they are not isolated", first.Database)
		}
		var leaked int
		if err := second.Store.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM public.customers WHERE customer_code = 99`).Scan(&leaked); err != nil {
			t.Fatalf("read across the clone boundary: %v", err)
		}
		if leaked != 0 {
			t.Errorf("the second test sees %d row(s) the first test wrote; the clones share data", leaked)
		}
		if _, err := second.Store.pool.Exec(ctx,
			`INSERT INTO public.customers (customer_code, legal_name) VALUES (98, 'PT. Second Test')`); err != nil {
			t.Fatalf("write the second test's row: %v", err)
		}
		var back int
		if err := first.Store.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM public.customers WHERE customer_code = 98`).Scan(&back); err != nil {
			t.Fatalf("read back into the first clone: %v", err)
		}
		if back != 0 {
			t.Errorf("the first test sees %d row(s) the second test wrote; the clones share data", back)
		}
	})

	t.Run("after cleanup", func(t *testing.T) {
		if testDatabaseExists(t, dsn, first.Database) {
			t.Errorf("database %q still exists after its test finished; a run would grow without bound", first.Database)
		}
	})
}

// TestDatabaseCloneDSNRenamesTheDatabase pins the one piece of the helper that
// is easy to get silently wrong: pgxpool.Config.ConnString() returns the string
// it parsed, not the config it holds, so mutating ConnConfig.Database produces
// a DSN that still names the base database. Every clone would then silently be
// the base database, and the isolation test above would be the only thing that
// noticed.
func TestDatabaseCloneDSNRenamesTheDatabase(t *testing.T) {
	const base = "postgres://vera:secret@127.0.0.1:55433/veratest?sslmode=disable&application_name=vera"

	renamed, err := dsnForDatabase(base, "vera_probe_clone_1a2b3c4d")
	if err != nil {
		t.Fatalf("rename the DSN: %v", err)
	}
	parsed, err := pgxpool.ParseConfig(renamed)
	if err != nil {
		t.Fatalf("parse the renamed DSN: %v", err)
	}
	if parsed.ConnConfig.Database != "vera_probe_clone_1a2b3c4d" {
		t.Errorf("renamed DSN still names %q; want the clone", parsed.ConnConfig.Database)
	}
	if parsed.ConnConfig.User != "vera" || parsed.ConnConfig.Password != "secret" {
		t.Errorf("renamed DSN lost the credentials: user %q", parsed.ConnConfig.User)
	}
	// sslmode=disable is a parse-time setting: pgx keeps it as a nil TLSConfig
	// and does not repeat it in RuntimeParams, so that is what survives or not.
	if parsed.ConnConfig.TLSConfig != nil {
		t.Error("renamed DSN lost sslmode=disable; it would now attempt TLS")
	}
	if parsed.ConnConfig.RuntimeParams["application_name"] != "vera" {
		t.Errorf("renamed DSN lost application_name: %v", parsed.ConnConfig.RuntimeParams)
	}

	// A keyword/value DSN carries no URL to rewrite, and libpq accepts both
	// spellings of the database setting.
	for _, dsn := range []string{
		"host=127.0.0.1 port=55433 user=vera password=secret dbname=veratest sslmode=disable",
		"host=127.0.0.1 port=55433 user=vera password=secret database=veratest sslmode=disable",
		"host=127.0.0.1 port=55433 user=vera password=secret sslmode=disable",
	} {
		keyword, err := dsnForDatabase(dsn, "vera_probe_keyword")
		if err != nil {
			t.Fatalf("rename %q: %v", dsn, err)
		}
		config, err := pgxpool.ParseConfig(keyword)
		if err != nil {
			t.Fatalf("parse %q: %v", keyword, err)
		}
		if config.ConnConfig.Database != "vera_probe_keyword" {
			t.Errorf("%q renamed to database %q; want vera_probe_keyword",
				dsn, config.ConnConfig.Database)
		}
		if config.ConnConfig.Host != "127.0.0.1" || config.ConnConfig.Port != 55433 {
			t.Errorf("%q lost the host: %s:%d", dsn, config.ConnConfig.Host, config.ConnConfig.Port)
		}
	}
}

// TestTheTemplateIsBuiltOncePerProcessAndReused pins the cost model: the
// migration chain runs once for the whole binary, not once per test. The ledger
// assertion is the external half -- every clone carries all 53 migrations, so a
// clone never needs to replay anything -- and the build counter is the internal
// half, because "built once" is otherwise indistinguishable from "built again
// quickly".
func TestTheTemplateIsBuiltOncePerProcessAndReused(t *testing.T) {
	first := openTestDatabase(t, "vera_probe_template_a")
	second := openTestDatabase(t, "vera_probe_template_b")

	if testDatabases.builds != 1 {
		t.Errorf("templates built = %d after two tests; want the process-wide one (a per-test template pays the chain again)",
			testDatabases.builds)
	}

	migrations, err := loadMigrations(testDatabaseSchema)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	ctx := context.Background()
	for _, handle := range []*testDatabase{first, second} {
		var applied int
		if err := handle.Store.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM public.schema_migrations`).Scan(&applied); err != nil {
			t.Fatalf("read %s's migration ledger: %v", handle.Database, err)
		}
		if applied != len(migrations) {
			t.Errorf("%s applied %d migrations; want all %d", handle.Database, applied, len(migrations))
		}
	}
}

// TestTestDatabaseNameIsSafeUniqueAndBounded covers the derivation rule directly
// because a name that is too long is not a flaky test: Postgres truncates it,
// two long schema names can land on one database, and the collision shows up as
// one test corrupting another.
func TestTestDatabaseNameIsSafeUniqueAndBounded(t *testing.T) {
	for _, schema := range []string{"vera_test_units_seeds", strings.Repeat("a", 80), ""} {
		name := testDatabaseName(schema)
		if len(name) > testDatabaseMaxIdentifier {
			t.Errorf("name for %q is %d characters; Postgres truncates past %d: %q",
				schema, len(name), testDatabaseMaxIdentifier, name)
		}
		if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(name) {
			t.Errorf("name for %q is not a bare identifier: %q", schema, name)
		}
		if again := testDatabaseName(schema); again != name {
			t.Errorf("name for %q is not stable: %q then %q", schema, name, again)
		}
	}

	// Truncation must not make two long names collide: a 60-character name with
	// a different tail still has to get its own database.
	long := strings.Repeat("b", 60)
	if testDatabaseName(long) == testDatabaseName(long+"x") {
		t.Error("two different long schema names map to one database; a clone would be shared")
	}
	if !strings.HasPrefix(testDatabaseName("vera_po_report_history_amount_overflow_empty"),
		"vera_po_report_history_amount_overflow_empty_") {
		t.Error("a 44-character schema lost its readable name; the clone should be identifiable in psql")
	}
}

// TestMissingCreatedbPrivilegeIsNamed pins the failure a developer actually
// hits: a DSN whose role may not create databases. It must say what is missing
// and how to fix it, and it must not drift back to a schema replay, which is
// what this helper replaced.
func TestMissingCreatedbPrivilegeIsNamed(t *testing.T) {
	err := testDatabaseCreatedbError("vera_reader")
	if err == nil {
		t.Fatal("a role without CREATEDB produced no error; the helper would fall back to the slow path")
	}
	for _, want := range []string{"CREATE DATABASE", `"vera_reader"`, "CREATEDB", "VERA_POSTGRES_TEST_DSN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestTheCloneDSNIsUsable opens its own connection to the clone: a test that
// shells out to pg_dump, or needs a second backend to hold a lock, gets the
// clone's DSN rather than the base database's.
func TestTheCloneDSNIsUsable(t *testing.T) {
	handle := openTestDatabase(t, "vera_probe_dsn")

	parsed, err := pgxpool.ParseConfig(handle.DSN)
	if err != nil {
		t.Fatalf("parse the clone DSN: %v", err)
	}
	if parsed.ConnConfig.Database != handle.Database {
		t.Errorf("the clone DSN names %q; want the clone %q", parsed.ConnConfig.Database, handle.Database)
	}
	if handle.Identity != "vera_probe_dsn" || handle.Schema != testDatabaseSchema {
		t.Errorf("handle identity = %q/%q; want the requested name and the public schema",
			handle.Identity, handle.Schema)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, handle.DSN)
	if err != nil {
		t.Fatalf("open the clone directly: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx,
		`INSERT INTO public.customers (customer_code, legal_name) VALUES (97, 'PT. Raw Connection')`); err != nil {
		t.Fatalf("write through the clone DSN: %v", err)
	}
	var seen int
	if err := handle.Store.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM public.customers WHERE customer_code = 97`).Scan(&seen); err != nil {
		t.Fatalf("read the raw connection's row through the store: %v", err)
	}
	if seen != 1 {
		t.Errorf("the store sees %d of the raw connection's rows; the DSN does not point at the clone", seen)
	}
}

// testDatabaseExists asks the server directly whether a database is there. It
// deliberately does not use the helper's own bookkeeping: a cleanup that
// released a name without dropping the database would still be visible here.
func testDatabaseExists(t *testing.T, dsn, name string) bool {
	t.Helper()
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
	return exists
}
