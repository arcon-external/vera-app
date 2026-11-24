// Package testdb provisions isolated, fully migrated Postgres databases for a
// test binary.
//
// It exists so that every database-backed package can share one provisioning
// seam. Provisioning happens once per test process and once per test:
//
//   - a template database is created and migrated once per process, then
//     closed (a database with a live connection cannot be cloned);
//   - each test gets a clone of it via CREATE DATABASE ... TEMPLATE, which is
//     already migrated;
//   - every clone is dropped in the test's cleanup, so a run leaves nothing
//     behind.
//
// A clone is keyed by its top-level test, not by the *testing.T that happens to
// ask first: a parent test boots a server and a subtest asserts against it, and
// both must reach one database. Two top-level tests that reuse an identity get
// separate clones, because a clone carries one test's rows.
//
// The template is named after the test binary, not after this package: `go
// test ./...` runs store.test and api.test in parallel against one test
// database, and a shared template name would let one process drop and rebuild
// the template while the other is cloning it. Two processes of the *same*
// binary still share a template, so the template's lifecycle and every clone
// run under a Postgres session-level advisory lock keyed on the template name;
// a second process waits rather than dropping the template underneath the
// first. The clone's name is reserved for the test's own lifetime for the same
// reason: two processes running the same test derive one clone name, and the
// second must wait for the first to drop it, not force-drop a database the
// first is still using.
//
// The package deliberately imports no application package. backend/store's
// in-package test files import this package, so a dependency on store would be
// an import cycle; the migration is supplied by the caller as a Migrate
// function.
//
// It carries no test files of its own on purpose: the CI step that runs the
// database-backed packages is the one place a test database exists, and this
// package's contract is proven by its consumers there (backend/store and
// backend/api both assert isolation, one template per process and the drop in
// cleanup). A test file here would move a database-backed suite into the
// step documented as never opening one.
package testdb

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// Schema is the schema the template is migrated with and every clone keeps.
	// It is deliberately not "public": Postgres does not dump the built-in
	// "public" schema, so the restore drill could neither restore a dump of its
	// own clone after ResetSchema drops the schema, nor prove the dump names
	// the test's schema. Migrations are applied with this name, so the trigger
	// functions the chain creates keep a schema that survives cloning (they
	// bake the name in; renaming "public" afterwards would break them).
	Schema = "vera_test"
	// MaxIdentifierLength is Postgres's NAMEDATALEN-1: longer identifiers are
	// truncated by the server, and two names that differ only past this point
	// would land on one database.
	MaxIdentifierLength = 63
	// Timeout bounds the DDL a test performs on databases rather than on rows,
	// so a stuck lock fails the test instead of hanging the suite.
	Timeout = 2 * time.Minute
)

// Migrate applies the application's migrations to an empty database named by
// dsn. It runs once per process, on the template database.
type Migrate func(ctx context.Context, dsn string) error

// Database is one test's isolated Postgres database.
type Database struct {
	// DSN names the clone. Use it for a test that opens its own connection or
	// shells out (pg_dump); the base DSN would point at the base database.
	DSN string
	// Name is the clone's physical database name.
	Name string
	// Identity is the name the test asked for. Tests kept their existing
	// argument, so this is what a test body may still key on.
	Identity string
	// Schema is the schema the clone carries, always Schema.
	Schema string
	// reservation is the session holding the clone-name lock. It stays open
	// until the clone is dropped, so another process running the same test
	// waits instead of force-dropping a database this test still uses.
	reservation *pgx.Conn
}

// process is the state shared by every test in one test binary.
var process struct {
	once   sync.Once
	admin  *pgxpool.Pool
	err    error
	builds atomic.Int64
}

// databases maps a top-level test to the clones it has asked for. Keying on the
// top-level test rather than only on the identity is what makes two tests that
// reuse a name (the suites do reuse a few) get separate databases, while a
// parent and its subtests share one.
var databases sync.Map // string -> *registry

// registry holds one top-level test's clones, created lazily and dropped in
// the cleanup of the TB that created it. A parent creates its clones in its own
// body, so the cleanup runs after every subtest that borrowed them.
type registry struct {
	mu      sync.Mutex
	byIdent map[string]*Database
	order   []string
}

// Builds reports how many templates this process built: zero before the first
// Acquire, one afterwards. It exists so a test can distinguish "built once" from
// "built again quickly".
func Builds() int {
	return int(process.builds.Load())
}

// The lock namespaces keep the test-database locks in their own key space, so
// an unrelated advisory lock that happens to hash to the same number cannot
// block a test run. They must never change: two processes meet on the same lock
// only when both derive the key the same way.
const (
	templateLockNamespace = "vera-app/testdb/template"
	cloneLockNamespace    = "vera-app/testdb/clone"
)

// TemplateLockKey is the stable 64-bit advisory-lock key for a template's
// lifecycle. It depends only on the template name, so every process sharing a
// test database -- and only those -- contends on the same session lock.
func TemplateLockKey(template string) int64 {
	return advisoryKey(templateLockNamespace, template)
}

// CloneLockKey is the stable 64-bit advisory-lock key that reserves one clone
// name for its test's lifetime.
func CloneLockKey(clone string) int64 {
	return advisoryKey(cloneLockNamespace, clone)
}

// advisoryKey is the stable 64-bit hash of a namespaced object name.
func advisoryKey(namespace, name string) int64 {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(namespace + "\x00" + name))
	return int64(digest.Sum64())
}

// LockConn is the connection surface the advisory locks use. Both *pgx.Conn
// (a template build or a clone reservation) and *pgxpool.Conn (a clone) implement
// it. A session-level lock belongs to its session, so the same connection must
// take it and release it.
type LockConn interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// AcquireTemplateLock takes the session-level advisory lock that serializes one
// template's lifecycle across every process sharing the test database. It waits
// until the lock is granted or ctx is done; a timeout names the template rather
// than letting the caller proceed into DROP DATABASE.
func AcquireTemplateLock(ctx context.Context, conn LockConn, template string) error {
	if err := acquireLock(ctx, conn, TemplateLockKey(template)); err != nil {
		return lockError(ctx, err, "the test database template", template)
	}
	return nil
}

// ReleaseTemplateLock releases the session lock AcquireTemplateLock took on
// conn. It reports a failed unlock rather than hiding it: a lock left behind
// blocks every later process until the session ends.
func ReleaseTemplateLock(ctx context.Context, conn LockConn, template string) error {
	return releaseLock(ctx, conn, TemplateLockKey(template), "the test database template", template)
}

// AcquireCloneLock reserves one clone name for the test's lifetime. Two
// processes running the same test derive the same clone name; the second waits
// here instead of force-dropping a database the first is still using. The lock
// is held until the clone is dropped, so it must be taken on a connection that
// lives as long as the clone.
func AcquireCloneLock(ctx context.Context, conn LockConn, clone string) error {
	if err := acquireLock(ctx, conn, CloneLockKey(clone)); err != nil {
		return lockError(ctx, err, "the test database clone", clone)
	}
	return nil
}

// ReleaseCloneLock releases the reservation AcquireCloneLock took on conn. It
// is called after the clone is dropped, so the next process can create it.
func ReleaseCloneLock(ctx context.Context, conn LockConn, clone string) error {
	return releaseLock(ctx, conn, CloneLockKey(clone), "the test database clone", clone)
}

// acquireLock blocks until the session holds key or ctx is done.
func acquireLock(ctx context.Context, conn LockConn, key int64) error {
	_, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", key)
	return err
}

// releaseLock reports a failed or impossible unlock rather than hiding it: a
// lock left behind blocks every later process until the session ends.
func releaseLock(ctx context.Context, conn LockConn, key int64, kind, name string) error {
	var released bool
	if err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&released); err != nil {
		return fmt.Errorf("release the lock on %s %q: %w", kind, name, err)
	}
	if !released {
		return fmt.Errorf("release the lock on %s %q: the session did not hold it", kind, name)
	}
	return nil
}

// lockError names what was being locked and, for a bounded wait, says outright
// that the wait expired rather than leaking a driver error.
func lockError(ctx context.Context, err error, kind, name string) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("wait for the lock on %s %q: %w", kind, name, ctxErr)
	}
	return fmt.Errorf("lock %s %q: %w", kind, name, err)
}

// Acquire returns the test's clone of the migrated template database for
// identity, creating the process template and the clone on first use.
//
// Repeated calls within one top-level test and identity return the same clone:
// a test that boots a server and then opens a second store for direct
// assertions must see one database, not two empty copies of the same schema,
// and a subtest must be able to assert against the server its parent booted.
func Acquire(tb testing.TB, dsn, identity string, migrate Migrate) *Database {
	tb.Helper()
	if strings.TrimSpace(dsn) == "" {
		tb.Fatal("test database: VERA_POSTGRES_TEST_DSN is empty")
	}
	if migrate == nil {
		tb.Fatal("test database: no migration function was supplied")
	}
	admin, err := adminPool(dsn, migrate)
	if err != nil {
		tb.Fatalf("test database template: %v", err)
	}

	scope := topLevelTestName(tb)
	registry := registryFor(tb, scope)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if database, ok := registry.byIdent[identity]; ok {
		return database
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	template := TemplateNameFor(BinaryName())
	name := CloneNameFor(identity, BinaryName(), scope)
	cloneDSN, err := DSNFor(dsn, name)
	if err != nil {
		tb.Fatalf("test database %s: %v", name, err)
	}
	// Two processes running the same test derive the same clone name, so the
	// name is reserved for this test's whole lifetime, not only while CREATE
	// DATABASE runs: the next process would otherwise FORCE-drop a database
	// this test is still using. The reservation connection stays open until
	// cleanup has dropped the clone.
	reservation, err := pgx.Connect(ctx, dsn)
	if err != nil {
		tb.Fatalf("test database clone %s: connect to reserve the name: %v", name, err)
	}
	reserved := false
	defer func() {
		if reserved {
			unlockCtx, cancel := context.WithTimeout(context.Background(), Timeout)
			defer cancel()
			_ = ReleaseCloneLock(unlockCtx, reservation, name)
			_ = reservation.Close(context.Background())
		}
	}()
	if err := AcquireCloneLock(ctx, reservation, name); err != nil {
		_ = reservation.Close(context.Background())
		tb.Fatalf("test database clone %s: %v", name, err)
	}
	reserved = true

	// The clone is created with the template locked: another process building
	// the same template must not DROP it between the catalog lookup and the
	// copy. The held connection is also the one that unlocks, because an
	// advisory lock belongs to its session.
	adminConn, err := admin.Acquire(ctx)
	if err != nil {
		tb.Fatalf("test database clone %s: acquire an admin connection: %v", name, err)
	}
	locked := false
	defer func() {
		if locked {
			unlockCtx, cancel := context.WithTimeout(context.Background(), Timeout)
			defer cancel()
			if err := ReleaseTemplateLock(unlockCtx, adminConn, template); err != nil {
				tb.Errorf("test database clone %s: %v", name, err)
				destroyPoolConn(adminConn)
			}
		}
		adminConn.Release()
	}()
	if err := AcquireTemplateLock(ctx, adminConn, template); err != nil {
		destroyPoolConn(adminConn)
		tb.Fatalf("test database clone %s: %v", name, err)
	}
	locked = true
	// A run that was interrupted can leave its clone behind; dropping first
	// keeps a rerun on a warm database behaving like the first run.
	if _, err := adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(name)+" WITH (FORCE)"); err != nil {
		tb.Fatalf("drop the previous test database %s: %v", name, err)
	}
	if _, err := adminConn.Exec(ctx,
		"CREATE DATABASE "+quoteIdentifier(name)+" TEMPLATE "+quoteIdentifier(template)); err != nil {
		tb.Fatalf("clone the test database template into %s: %v", name, err)
	}
	if err := ReleaseTemplateLock(ctx, adminConn, template); err != nil {
		tb.Fatalf("test database clone %s: %v", name, err)
	}
	locked = false

	database := &Database{
		DSN:         cloneDSN,
		Name:        name,
		Identity:    identity,
		Schema:      Schema,
		reservation: reservation,
	}
	registry.byIdent[identity] = database
	registry.order = append(registry.order, identity)
	// Ownership of the reservation moves to the registry, whose cleanup drops
	// the clone and only then releases the name.
	reserved = false
	return database
}

// topLevelTestName is the test name without its subtest path: the parent and
// every subtest of one test share one clone registry, so a subtest can assert
// against the server its parent booted.
func topLevelTestName(tb testing.TB) string {
	name := tb.Name()
	if slash := strings.IndexByte(name, '/'); slash >= 0 {
		return name[:slash]
	}
	return name
}

// registryFor returns the top-level test's registry, registering the one
// cleanup that drops everything the test cloned.
func registryFor(tb testing.TB, scope string) *registry {
	if value, ok := databases.Load(scope); ok {
		return value.(*registry)
	}
	fresh := &registry{byIdent: make(map[string]*Database)}
	value, loaded := databases.LoadOrStore(scope, fresh)
	if loaded {
		return value.(*registry)
	}
	tb.Cleanup(func() {
		databases.Delete(scope)
		fresh.dropAll(tb)
	})
	return fresh
}

// dropAll drops the test's clones. Cleanup has no live t.Context, so each drop
// gets its own deadline; a stuck backend then still names the database it
// blocked instead of hanging the suite.
func (r *registry) dropAll(tb testing.TB) {
	r.mu.Lock()
	defer r.mu.Unlock()
	admin := process.admin
	if admin == nil {
		return
	}
	for i := len(r.order) - 1; i >= 0; i-- {
		database := r.byIdent[r.order[i]]
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(database.Name)+" WITH (FORCE)"); err != nil {
			tb.Errorf("drop test database %s: %v", database.Name, err)
		}
		cancel()
		// Release the name only after the drop: a process waiting on it must
		// not create the clone before this one is really gone. Closing the
		// session releases the lock even if the unlock statement fails.
		if database.reservation != nil {
			unlockCtx, unlockCancel := context.WithTimeout(context.Background(), Timeout)
			if err := ReleaseCloneLock(unlockCtx, database.reservation, database.Name); err != nil {
				tb.Errorf("release the reservation on test database %s: %v", database.Name, err)
			}
			unlockCancel()
			_ = database.reservation.Close(context.Background())
		}
	}
}

// adminPool returns the process-wide admin connection, building the template
// first if this process has not done so. The error is cached, so a missing
// CREATEDB privilege fails every test with the same sentence instead of
// falling back to a schema replay.
func adminPool(dsn string, migrate Migrate) (*pgxpool.Pool, error) {
	process.once.Do(func() {
		process.err = buildTemplate(dsn, migrate)
	})
	if process.err != nil {
		return nil, process.err
	}
	return process.admin, nil
}

// buildTemplate creates and migrates the template database once for the
// process, then closes it: a database with a live connection cannot be cloned.
func buildTemplate(dsn string, migrate Migrate) error {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	template := TemplateNameFor(BinaryName())
	templateDSN, err := DSNFor(dsn, template)
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to the test database: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	// The template's whole lifecycle -- drop, create, migrate -- happens under
	// one advisory lock, so two store.test processes sharing a database
	// serialize instead of dropping the template under each other. Closing the
	// session releases the lock even if the unlock statement itself failed.
	if err := AcquireTemplateLock(ctx, conn, template); err != nil {
		return err
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), Timeout)
		defer cancel()
		_ = ReleaseTemplateLock(unlockCtx, conn, template)
	}()

	var role string
	var canCreate bool
	if err := conn.QueryRow(ctx,
		`SELECT current_user, rolsuper OR rolcreatedb FROM pg_roles WHERE rolname = current_user`,
	).Scan(&role, &canCreate); err != nil {
		return fmt.Errorf("read the test role's privileges: %w", err)
	}
	if !canCreate {
		return CreatedbError(role)
	}

	if _, err := conn.Exec(ctx,
		"DROP DATABASE IF EXISTS "+quoteIdentifier(template)+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("drop the previous test database template: %w", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+quoteIdentifier(template)); err != nil {
		return fmt.Errorf("create the test database template: %w", err)
	}

	if err := migrate(ctx, templateDSN); err != nil {
		return fmt.Errorf("migrate the test database template: %w", err)
	}

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open the test database admin connection: %w", err)
	}
	process.admin = admin
	process.builds.Add(1)
	return nil
}

// destroyPoolConn closes a pooled connection instead of returning it to the
// pool. A session whose lock state is uncertain must not be reused: if the
// unlock failed, a pooled session holding the template lock would deadlock
// every later clone in this process.
func destroyPoolConn(conn *pgxpool.Conn) {
	_ = conn.Hijack().Close(context.Background())
}

// CreatedbError names the privilege that is missing rather than letting CREATE
// DATABASE fail with a bare SQLSTATE, and it never suggests falling back to the
// schema replay: that path is what this helper removes.
func CreatedbError(role string) error {
	return fmt.Errorf(
		"the test databases need a Postgres role that can CREATE DATABASE, but %q cannot: "+
			"run `ALTER ROLE %q CREATEDB` as a superuser, or point VERA_POSTGRES_TEST_DSN at a role that can",
		role, role)
}

// BinaryName is the test binary's file name, the discriminator between the
// templates of two packages run in parallel against one database.
func BinaryName() string {
	return filepath.Base(os.Args[0])
}

// TemplateNameFor names the template database for a test binary. The path is
// reduced to its base name so the same binary gets the same template from any
// build directory.
func TemplateNameFor(binary string) string {
	return "vera_test_template_" + binaryTag(binary)
}

// CloneNameFor derives a clone's name from the identity the test asked for,
// the test binary and the top-level test. Postgres identifiers stop at 63
// characters and several identities are 44 characters already, so a name
// carries a stable digest of all three: two identities never share a clone, two
// test binaries never share a clone, two tests that reuse an identity never
// share a clone, and rerunning a test derives the same name it used before.
func CloneNameFor(identity, binary, scope string) string {
	suffix := "_" + binaryTag(binary) + "_" + digestOf(identity+"\x00"+scope)
	return identifier(identity, "vera_test", MaxIdentifierLength-len(suffix)) + suffix
}

// identifier reduces value to a bare Postgres identifier of at most limit
// characters, falling back to name when nothing usable remains.
func identifier(value, name string, limit int) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		switch {
		case character >= 'a' && character <= 'z',
			character >= '0' && character <= '9',
			character == '_':
			builder.WriteRune(character)
		default:
			builder.WriteByte('_')
		}
	}
	result := builder.String()
	if result == "" {
		result = name
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// binaryTag is the stable 8-hex discriminator for a test binary.
func binaryTag(binary string) string {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(filepath.Base(binary)))
	return hex.EncodeToString(digest.Sum(nil)[:4])
}

// digestOf is the stable 8-hex digest of an identity.
func digestOf(identity string) string {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(identity))
	return hex.EncodeToString(digest.Sum(nil)[:4])
}

// DSNFor is the same connection string with another database name. The result
// carries the credentials, so it is never logged.
//
// It rewrites the string rather than pgxpool.Config.ConnString(): that method
// returns the string it parsed, so mutating ConnConfig.Database and asking for
// the string back yields the base DSN unchanged -- every clone would then be the
// base database.
func DSNFor(dsn, database string) (string, error) {
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

// quoteIdentifier quotes a database name for DDL. Names here are derived from
// test-selected strings, never user input.
func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
