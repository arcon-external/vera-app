package store

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

// openTestSchema drops the schema and opens it empty, so a rerun on a warm
// database behaves exactly like the first run. The store tests create clients,
// quotations and profile rows that they do not all remove, and a leftover row
// fails the next run in ways that look like real regressions (a delete blocked
// by a foreign key, a seeded name already customised). CI gets a fresh Postgres
// per run and never sees it; a developer's reused database sees nothing else.
func openTestSchema(t *testing.T, schema string) *Store {
	t.Helper()
	resetTestSchema(t, testDSN(t), schema)
	database, err := Open(context.Background(), Config{DSN: testDSN(t), Schema: schema})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)
	return database
}

// resetTestSchema drops one schema outright, migrations included.
func resetTestSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect for schema reset: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`DROP SCHEMA IF EXISTS "`+strings.ReplaceAll(schema, `"`, `""`)+`" CASCADE`); err != nil {
		t.Fatalf("reset schema %s: %v", schema, err)
	}
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
