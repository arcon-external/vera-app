package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedRecipientCompanyColumnOnLegacySchema adds migration 045's column to a
// schema deliberately held at an earlier migration. Those tests seed their
// fixtures through the current SaveRevision, which writes every current
// column; without this the old schema rejects the write for a reason unrelated
// to the migration under test. Applying 045 later is then a no-op.
func seedRecipientCompanyColumnOnLegacySchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(
		`ALTER TABLE %s.quotation_revisions
		 ADD COLUMN IF NOT EXISTS recipient_company TEXT NOT NULL DEFAULT ''`,
		quoteIdentifier(schema))); err != nil {
		t.Fatalf("seed the recipient_company column on the legacy schema: %v", err)
	}
}

// TestRecipientCompanySnapshotMigration pins the migration that adds the
// immutable recipient company snapshot. Old revision rows must read back empty
// -- not inherit today's customer name -- because an empty snapshot is the
// documented signal to fall back to the live customer row.
func TestRecipientCompanySnapshotMigration(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var snapshot string
	for _, migration := range migrations {
		if migration.version == 45 {
			snapshot = migration.sql
			break
		}
	}
	if snapshot == "" {
		t.Fatal("migration 45 is missing")
	}
	for _, want := range []string{"recipient_company", "ADD COLUMN IF NOT EXISTS", "NOT NULL DEFAULT ''"} {
		if !strings.Contains(snapshot, want) {
			t.Errorf("migration 45 missing %q:\n%s", want, snapshot)
		}
	}

	database := openTestSchema(t, "vera_test_recipient_company_migration")
	ctx := context.Background()
	schema := database.configSchema()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}

	var customerID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s.customers WHERE customer_code = 2`, schema)).Scan(&customerID); err != nil {
		t.Fatalf("seeded customer: %v", err)
	}
	var quotationID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotations (quote_number, customer_id)
		VALUES ('15029997', $1) RETURNING id`, schema), customerID).Scan(&quotationID); err != nil {
		t.Fatalf("insert quotation: %v", err)
	}
	var revisionID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revisions
			(quotation_id, revision_number, recipient_name, recipient_address, subject,
			 incoterm, validity_days, payment_term_days, lead_time_days,
			 discount_percent, tax_zone, tax_category, tax_rate_bps)
		VALUES ($1, 0, 'r', 'a', 's', 'EXW', 30, 30, 30, 0, 'FTZ', '', 0)
		RETURNING id`, schema), quotationID).Scan(&revisionID); err != nil {
		t.Fatalf("insert revision without the snapshot column: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT recipient_company FROM %s.quotation_revisions WHERE id = $1`, schema),
		revisionID).Scan(&stored); err != nil {
		t.Fatalf("read recipient_company: %v", err)
	}
	if stored != "" {
		t.Errorf("legacy revision snapshot = %q; want the empty default", stored)
	}
}
