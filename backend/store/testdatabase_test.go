package store

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/testdb"
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
			`SELECT COUNT(*) FROM `+quoteIdentifier(first.Schema)+`.customers
			WHERE customer_code BETWEEN 2 AND 6`).Scan(&seeded); err != nil {
			t.Fatalf("read the migrated seed: %v", err)
		}
		if seeded != 5 {
			t.Fatalf("seeded clients = %d; want the five migration 011 rows", seeded)
		}

		if _, err := first.Store.pool.Exec(ctx,
			`INSERT INTO `+quoteIdentifier(first.Schema)+`.customers (customer_code, legal_name)
			VALUES (99, 'PT. First Test')`); err != nil {
			t.Fatalf("write the first test's row: %v", err)
		}

		second := openTestDatabase(t, "vera_probe_isolation_b")
		if second.Database == first.Database {
			t.Fatalf("both tests were given the database %q; they are not isolated", first.Database)
		}
		var leaked int
		if err := second.Store.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM `+quoteIdentifier(second.Schema)+`.customers
			WHERE customer_code = 99`).Scan(&leaked); err != nil {
			t.Fatalf("read across the clone boundary: %v", err)
		}
		if leaked != 0 {
			t.Errorf("the second test sees %d row(s) the first test wrote; the clones share data", leaked)
		}
		if _, err := second.Store.pool.Exec(ctx,
			`INSERT INTO `+quoteIdentifier(second.Schema)+`.customers (customer_code, legal_name)
			VALUES (98, 'PT. Second Test')`); err != nil {
			t.Fatalf("write the second test's row: %v", err)
		}
		var back int
		if err := first.Store.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM `+quoteIdentifier(first.Schema)+`.customers
			WHERE customer_code = 98`).Scan(&back); err != nil {
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

	renamed, err := testdb.DSNFor(base, "vera_probe_clone_1a2b3c4d")
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
		keyword, err := testdb.DSNFor(dsn, "vera_probe_keyword")
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

	if testdb.Builds() != 1 {
		t.Errorf("templates built = %d after two tests; want the process-wide one (a per-test template pays the chain again)",
			testdb.Builds())
	}

	migrations, err := loadMigrations(first.Schema)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	ctx := context.Background()
	for _, handle := range []*testDatabase{first, second} {
		var applied int
		if err := handle.Store.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM `+quoteIdentifier(handle.Schema)+`.schema_migrations`).Scan(&applied); err != nil {
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
		name := testdb.CloneNameFor(schema, testdb.BinaryName(), t.Name())
		if len(name) > testDatabaseMaxIdentifier {
			t.Errorf("name for %q is %d characters; Postgres truncates past %d: %q",
				schema, len(name), testDatabaseMaxIdentifier, name)
		}
		if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(name) {
			t.Errorf("name for %q is not a bare identifier: %q", schema, name)
		}
		if again := testdb.CloneNameFor(schema, testdb.BinaryName(), t.Name()); again != name {
			t.Errorf("name for %q is not stable: %q then %q", schema, name, again)
		}
	}

	// Truncation must not make two long names collide: a 60-character name with
	// a different tail still has to get its own database.
	long := strings.Repeat("b", 60)
	if testdb.CloneNameFor(long, testdb.BinaryName(), t.Name()) == testdb.CloneNameFor(long+"x", testdb.BinaryName(), t.Name()) {
		t.Error("two different long schema names map to one database; a clone would be shared")
	}
	if !strings.HasPrefix(testdb.CloneNameFor("vera_po_report_history_amount_overflow_empty", testdb.BinaryName(), t.Name()),
		"vera_po_report_history_amount_overflow_empty_") {
		t.Error("a 44-character schema lost its readable name; the clone should be identifiable in psql")
	}
}

// TestMissingCreatedbPrivilegeIsNamed pins the failure a developer actually
// hits: a DSN whose role may not create databases. It must say what is missing
// and how to fix it, and it must not drift back to a schema replay, which is
// what this helper replaced.
func TestMissingCreatedbPrivilegeIsNamed(t *testing.T) {
	err := testdb.CreatedbError("vera_reader")
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
	if handle.Identity != "vera_probe_dsn" || handle.Schema != testdb.Schema {
		t.Errorf("handle identity = %q/%q; want the requested name and the app schema",
			handle.Identity, handle.Schema)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, handle.DSN)
	if err != nil {
		t.Fatalf("open the clone directly: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx,
		`INSERT INTO `+quoteIdentifier(handle.Schema)+`.customers (customer_code, legal_name)
		VALUES (97, 'PT. Raw Connection')`); err != nil {
		t.Fatalf("write through the clone DSN: %v", err)
	}
	var seen int
	if err := handle.Store.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM `+quoteIdentifier(handle.Schema)+`.customers
		WHERE customer_code = 97`).Scan(&seen); err != nil {
		t.Fatalf("read the raw connection's row through the store: %v", err)
	}
	if seen != 1 {
		t.Errorf("the store sees %d of the raw connection's rows; the DSN does not point at the clone", seen)
	}
}

// TestAcquireReturnsOneClonePerTestAndIdentity pins the contract the API
// helpers depend on: one test asks for the same identity from several helpers
// (a server, a direct store, a signing key), and every one of them must land in
// the same physical database. Two clones would give the test two empty copies
// of the same schema, so the second helper would assert against data the first
// never wrote.
func TestAcquireReturnsOneClonePerTestAndIdentity(t *testing.T) {
	dsn := testDSN(t)
	first := testdb.Acquire(t, dsn, "vera_probe_shared_identity", migrateTestDatabase)
	second := testdb.Acquire(t, dsn, "vera_probe_shared_identity", migrateTestDatabase)

	if first.Name != second.Name {
		t.Errorf("one test's two lookups got databases %q and %q; the helpers would not share data",
			first.Name, second.Name)
	}
	if first.DSN != second.DSN {
		t.Error("one test's two lookups got different DSNs; the helpers would not share data")
	}
	if first.Identity != "vera_probe_shared_identity" || first.Schema != testdb.Schema {
		t.Errorf("clone identity = %q/%q; want the requested name and the app schema %q",
			first.Identity, first.Schema, testdb.Schema)
	}
}

// TestTemplateNamesArePerTestBinary pins why the template is named after the
// test binary: `go test ./...` runs store.test and api.test in parallel against
// one test database, and a shared template name would let one process drop and
// recreate the template while the other is cloning it.
func TestTemplateNamesArePerTestBinary(t *testing.T) {
	api := testdb.TemplateNameFor("api.test")
	store := testdb.TemplateNameFor("store.test")
	if api == store {
		t.Fatalf("api.test and store.test share the template %q; a parallel run would drop it under the other", api)
	}
	if again := testdb.TemplateNameFor("/tmp/go-build123/b001/api.test"); again != api {
		t.Errorf("template name depends on the build path: %q then %q", api, again)
	}
	for _, name := range []string{api, store} {
		if len(name) > testdb.MaxIdentifierLength {
			t.Errorf("template %q is %d characters; Postgres truncates past %d",
				name, len(name), testdb.MaxIdentifierLength)
		}
		if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(name) {
			t.Errorf("template %q is not a bare identifier", name)
		}
	}
}

// TestCloneNamesAreScopedToTheTestBinary pins the other half: even if two test
// binaries picked the same identity, their clones may not collide.
func TestCloneNamesAreScopedToTheTestBinary(t *testing.T) {
	api := testdb.CloneNameFor("vera_api_shared_identity", "api.test", "TestX")
	store := testdb.CloneNameFor("vera_api_shared_identity", "store.test", "TestX")
	if api == store {
		t.Fatalf("one identity under two test binaries maps to one clone %q", api)
	}
}

// TestAdvisoryLockKeysAreStableAndScoped pins the derivation the locks
// depend on: every process must meet on one key for one template, and two
// templates must never share a key; the clone key space is separate too.
func TestAdvisoryLockKeysAreStableAndScoped(t *testing.T) {
	template := testdb.TemplateNameFor("store.test")
	key := testdb.TemplateLockKey(template)
	if again := testdb.TemplateLockKey(template); again != key {
		t.Fatalf("the same template produced keys %d then %d; two processes would not meet", key, again)
	}
	if other := testdb.TemplateLockKey(testdb.TemplateNameFor("api.test")); other == key {
		t.Fatalf("store.test and api.test share the lock key %d; their templates are independent", key)
	}

	// Pin the number. Changing the derivation moves every process to a new
	// lock, so a process still running the previous binary and one running the
	// new one would hold two different locks over the same template -- exactly
	// the race the lock exists to remove.
	if got, want := key, int64(8525218076598627597); got != want {
		t.Errorf("TemplateLockKey(%q) = %d; want the pinned %d", template, got, want)
	}

	// The clone reservation is a second key space: a clone and a template with
	// the same text must not meet on one lock.
	clone := "vera_test_clients_e61e5a20_10d7aeb6"
	if got, want := testdb.CloneLockKey(clone), int64(-8992055891338330083); got != want {
		t.Errorf("CloneLockKey(%q) = %d; want the pinned %d", clone, got, want)
	}
	if testdb.CloneLockKey(clone) == testdb.TemplateLockKey(clone) {
		t.Error("a clone and a template with the same name share one lock key")
	}
}

// TestTemplateLockWaitsAndTimesOut pins both halves of the lock contract on
// real Postgres sessions: a second session waits while the lock is held, and a
// bounded wait fails with the template's name instead of continuing into DROP
// DATABASE.
func TestTemplateLockWaitsAndTimesOut(t *testing.T) {
	dsn := testDSN(t)
	template := testdb.TemplateNameFor(testdb.BinaryName())
	ctx, cancel := context.WithTimeout(context.Background(), testdb.Timeout)
	defer cancel()

	holder := connectForLock(t, ctx, dsn)
	defer func() { _ = holder.Close(context.Background()) }()
	if err := testdb.AcquireTemplateLock(ctx, holder, template); err != nil {
		t.Fatalf("hold the template lock: %v", err)
	}

	// A bounded attempt on a second session has to give up with the template's
	// name rather than report a lock it never got.
	timedOut := connectForLock(t, ctx, dsn)
	defer func() { _ = timedOut.Close(context.Background()) }()
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer shortCancel()
	err := testdb.AcquireTemplateLock(shortCtx, timedOut, template)
	if err == nil {
		t.Fatal("a second session acquired a lock the first session holds")
	}
	for _, want := range []string{template, context.DeadlineExceeded.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout error %q does not name %q", err, want)
		}
	}

	// The same wait without the short deadline blocks until the holder
	// releases, then succeeds on the session that asked for it.
	waiter := connectForLock(t, ctx, dsn)
	defer func() { _ = waiter.Close(context.Background()) }()
	waited := make(chan error, 1)
	go func() {
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer waitCancel()
		waited <- testdb.AcquireTemplateLock(waitCtx, waiter, template)
	}()
	select {
	case err := <-waited:
		t.Fatalf("the waiter returned before the lock was released: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := testdb.ReleaseTemplateLock(ctx, holder, template); err != nil {
		t.Fatalf("release the template lock: %v", err)
	}
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("the waiter failed after the lock was released: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the waiter never acquired the released lock")
	}
	if err := testdb.ReleaseTemplateLock(ctx, waiter, template); err != nil {
		t.Fatalf("release the waiter's lock: %v", err)
	}
}

// templateLockChildEnv marks the child half of
// TestTemplateLockSerializesAcquireAcrossProcesses: the test binary re-runs
// itself, and only that process calls Acquire.
const templateLockChildEnv = "VERA_TESTDB_TEMPLATE_LOCK_CHILD"

// TestTemplateLockSerializesAcquireAcrossProcesses runs a second copy of this
// test binary against the same template -- the local multi-agent case -- and
// proves Acquire makes it wait instead of letting it drop the template. The
// child announces itself, then blocks inside Acquire's template build while
// this process holds the lock; pg_locks shows the wait, and the template's OID
// proves it was not rebuilt underneath a clone.
func TestTemplateLockSerializesAcquireAcrossProcesses(t *testing.T) {
	if os.Getenv(templateLockChildEnv) == "1" {
		fmt.Println("LOCK-CHILD READY")
		handle := testdb.Acquire(t, testDSN(t), "vera_probe_lock_child", migrateTestDatabase)
		fmt.Println("LOCK-CHILD DONE " + handle.Name)
		return
	}

	dsn := testDSN(t)
	// Build this process's template and keep a clone, the state a running test
	// binary is in when another checkout starts the same binary.
	parentClone := testdb.Acquire(t, dsn, "vera_probe_lock_parent", migrateTestDatabase)
	template := testdb.TemplateNameFor(testdb.BinaryName())
	ctx, cancel := context.WithTimeout(context.Background(), testdb.Timeout)
	defer cancel()

	holder := connectForLock(t, ctx, dsn)
	defer func() { _ = holder.Close(context.Background()) }()
	if err := testdb.AcquireTemplateLock(ctx, holder, template); err != nil {
		t.Fatalf("hold the template lock: %v", err)
	}
	templateOID := testDatabaseOID(t, dsn, template)

	child := startSelfTestChild(t, t.Name(), templateLockChildEnv+"=1", "LOCK-CHILD READY")
	waitForAdvisoryLockWait(t, dsn, testdb.TemplateLockKey(template), 30*time.Second, "the template lock")
	if got := testDatabaseOID(t, dsn, template); got != templateOID {
		t.Fatalf("the waiting child rebuilt the template (oid %d -> %d); Acquire is not holding the lock",
			templateOID, got)
	}
	if !testDatabaseExists(t, dsn, parentClone.Name) {
		t.Fatalf("the parent's clone %s vanished while the child was blocked", parentClone.Name)
	}

	if err := testdb.ReleaseTemplateLock(ctx, holder, template); err != nil {
		t.Fatalf("release the template lock: %v", err)
	}
	if err := child.wait(t); err != nil {
		t.Fatalf("the second test process failed after the lock was released: %v\nstderr:\n%s",
			err, child.stderr.String())
	}

	// The child rebuilding the template is fine now: this process's clone is
	// independent, and both survive because the child serialized behind the
	// lock rather than dropping the template under a live clone.
	if !testDatabaseExists(t, dsn, template) {
		t.Errorf("template %q is gone after the second process finished", template)
	}
	if !testDatabaseExists(t, dsn, parentClone.Name) {
		t.Errorf("clone %q is gone after the second process rebuilt the template", parentClone.Name)
	}
}

// cloneLockChildEnv marks the child half of
// TestCloneLockSerializesAcquireAcrossProcesses.
const cloneLockChildEnv = "VERA_TESTDB_CLONE_LOCK_CHILD"

// TestCloneLockSerializesAcquireAcrossProcesses covers the collision a template
// lock alone cannot fix: two processes running the same test derive the same
// clone name, so the second would force-drop a database the first is using.
// The child blocks inside Acquire on the clone reservation; this process proves
// the wait in pg_locks, that the clone does not exist while the name is
// reserved, and that the child creates and then drops it after the release.
func TestCloneLockSerializesAcquireAcrossProcesses(t *testing.T) {
	if os.Getenv(cloneLockChildEnv) == "1" {
		fmt.Println("CLONE-CHILD READY")
		handle := testdb.Acquire(t, testDSN(t), "vera_probe_clone_reserved", migrateTestDatabase)
		fmt.Println("CLONE-CHILD DONE " + handle.Name)
		return
	}

	dsn := testDSN(t)
	// Build this process's template and keep its own clone, so the child's
	// template build does not disturb the state under test.
	testdb.Acquire(t, dsn, "vera_probe_clone_reservation_parent", migrateTestDatabase)
	childClone := testdb.CloneNameFor("vera_probe_clone_reserved", testdb.BinaryName(), t.Name())

	ctx, cancel := context.WithTimeout(context.Background(), testdb.Timeout)
	defer cancel()
	holder := connectForLock(t, ctx, dsn)
	defer func() { _ = holder.Close(context.Background()) }()
	if err := testdb.AcquireCloneLock(ctx, holder, childClone); err != nil {
		t.Fatalf("reserve the child's clone name: %v", err)
	}
	// A clone left by an interrupted run would be visible while the child is
	// blocked; remove it under the reservation this test already holds.
	if _, err := holder.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(childClone)+" WITH (FORCE)"); err != nil {
		t.Fatalf("remove the stale clone %s: %v", childClone, err)
	}

	child := startSelfTestChild(t, t.Name(), cloneLockChildEnv+"=1", "CLONE-CHILD READY")
	waitForAdvisoryLockWait(t, dsn, testdb.CloneLockKey(childClone), 60*time.Second, "the clone reservation")
	if testDatabaseExists(t, dsn, childClone) {
		t.Fatalf("clone %s exists while its name is reserved and the child is waiting", childClone)
	}

	if err := testdb.ReleaseCloneLock(ctx, holder, childClone); err != nil {
		t.Fatalf("release the clone reservation: %v", err)
	}
	if err := child.wait(t); err != nil {
		t.Fatalf("the second test process failed after the reservation was released: %v\nstderr:\n%s",
			err, child.stderr.String())
	}
	if testDatabaseExists(t, dsn, childClone) {
		t.Errorf("clone %s still exists after the child's test finished; its reservation was released early",
			childClone)
	}
}

// selfTestChild is a second copy of this test binary restricted to one test,
// started so the cross-process locks can be observed from both sides.
type selfTestChild struct {
	cmd    *exec.Cmd
	stderr bytes.Buffer
	waited bool
}

// startSelfTestChild starts the child and waits for its ready line. The child
// is killed at test end unless the test already waited for it.
func startSelfTestChild(t *testing.T, testName, env, ready string) *selfTestChild {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate the test binary: %v", err)
	}
	child := &selfTestChild{}
	child.cmd = exec.Command(executable,
		"-test.run=^"+testName+"$",
		"-test.count=1",
		"-test.timeout=5m")
	child.cmd.Env = append(os.Environ(), env)
	stdout, err := child.cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe the child's stdout: %v", err)
	}
	child.cmd.Stderr = &child.stderr
	if err := child.cmd.Start(); err != nil {
		t.Fatalf("start the second test process: %v", err)
	}
	t.Cleanup(func() {
		if !child.waited {
			_ = child.cmd.Process.Kill()
			_ = child.cmd.Wait()
		}
	})
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	select {
	case line := <-lines:
		if line != ready {
			t.Fatalf("the child said %q before its Acquire; want %q", line, ready)
		}
	case <-time.After(30 * time.Second):
		// Stop the child before reading its buffer: Stderr is copied from a
		// goroutine, so reading it while the process runs is a data race.
		_ = child.cmd.Process.Kill()
		_ = child.cmd.Wait()
		child.waited = true
		t.Fatalf("the child never announced itself; stderr:\n%s", child.stderr.String())
	}
	return child
}

// wait waits for the child and reports its exit error. It bounds the wait and
// kills the child on timeout so a wedged lock cannot hang the suite.
func (c *selfTestChild) wait(t *testing.T) error {
	t.Helper()
	finished := make(chan error, 1)
	go func() { finished <- c.cmd.Wait() }()
	select {
	case err := <-finished:
		c.waited = true
		return err
	case <-time.After(2 * time.Minute):
		_ = c.cmd.Process.Kill()
		err := <-finished
		c.waited = true
		t.Fatalf("the second test process never finished: %v\nstderr:\n%s", err, c.stderr.String())
		return err
	}
}

// connectForLock opens a dedicated session: an advisory lock belongs to its
// session, so the connection that takes it is the one that must release it.
func connectForLock(t *testing.T, ctx context.Context, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for the test database lock: %v", err)
	}
	return conn
}

// waitForAdvisoryLockWait waits until pg_locks shows an ungranted advisory lock
// for key, which is the child sitting inside pg_advisory_lock. It observes the
// server, not the child's stdout: "READY" only says the child started, and the
// point is that it is waiting on the named lock.
func waitForAdvisoryLockWait(t *testing.T, dsn string, key int64, limit time.Duration, what string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to read pg_locks: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	// A 64-bit advisory key is reported as classid = high 32 bits and
	// objid = low 32 bits, with objsubid 1 for the single-key form.
	classid := int64(uint32(uint64(key) >> 32))
	objid := int64(uint32(uint64(key)))
	deadline := time.Now().Add(limit)
	for {
		var waiting int
		err := conn.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted
			  AND classid = $1::oid AND objid = $2::oid AND objsubid = 1`,
			classid, objid).Scan(&waiting)
		if err != nil {
			if ctx.Err() != nil {
				t.Fatalf("the child never waited on %s within %s", what, limit)
			}
			t.Fatalf("read pg_locks for %s: %v", what, err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child never waited on %s within %s", what, limit)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// testDatabaseOID is the template's identity on the server. A DROP and CREATE
// gives it a new OID, so an unchanged OID proves the template was not rebuilt.
func testDatabaseOID(t *testing.T, dsn, name string) uint32 {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to read %s's oid: %v", name, err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var oid uint32
	if err := conn.QueryRow(ctx, `SELECT oid FROM pg_database WHERE datname = $1`, name).Scan(&oid); err != nil {
		t.Fatalf("read %s's oid: %v", name, err)
	}
	return oid
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
