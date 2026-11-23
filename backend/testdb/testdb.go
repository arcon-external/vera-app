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
// the template while the other is cloning it.
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

	name := CloneNameFor(identity, BinaryName(), scope)
	cloneDSN, err := DSNFor(dsn, name)
	if err != nil {
		tb.Fatalf("test database %s: %v", name, err)
	}
	// A run that was interrupted can leave its clone behind; dropping first
	// keeps a rerun on a warm database behaving like the first run.
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(name)+" WITH (FORCE)"); err != nil {
		tb.Fatalf("drop the previous test database %s: %v", name, err)
	}
	if _, err := admin.Exec(ctx,
		"CREATE DATABASE "+quoteIdentifier(name)+" TEMPLATE "+quoteIdentifier(TemplateNameFor(BinaryName()))); err != nil {
		tb.Fatalf("clone the test database template into %s: %v", name, err)
	}

	database := &Database{
		DSN:      cloneDSN,
		Name:     name,
		Identity: identity,
		Schema:   Schema,
	}
	registry.byIdent[identity] = database
	registry.order = append(registry.order, identity)
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
		name := r.byIdent[r.order[i]].Name
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoteIdentifier(name)+" WITH (FORCE)"); err != nil {
			tb.Errorf("drop test database %s: %v", name, err)
		}
		cancel()
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
