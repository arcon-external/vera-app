package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedQuotationNoteColumnOnLegacySchema adds migration 046's column to a schema
// deliberately held at an earlier migration. Those tests seed their fixtures
// through the current SaveRevision, which writes every current column; without
// this the old schema rejects the write for a reason unrelated to the
// migration under test. Applying 046 later is then a no-op for the column.
func seedQuotationNoteColumnOnLegacySchema(t *testing.T, pool *pgxpool.Pool, schema string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(
		`ALTER TABLE %s.quotation_revisions
		 ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT ''`,
		quoteIdentifier(schema))); err != nil {
		t.Fatalf("seed the note column on the legacy schema: %v", err)
	}
}

// TestQuotationNoteMigration pins migration 046: the note is a NOT NULL column
// defaulting to empty, capped at 1000 characters. Old rows keep the empty
// default rather than being backfilled, so their canonical bytes and printed
// verification codes are unchanged.
func TestQuotationNoteMigration(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var noteMigration string
	for _, migration := range migrations {
		if migration.version == 46 {
			noteMigration = migration.sql
			break
		}
	}
	if noteMigration == "" {
		t.Fatal("migration 46 is missing")
	}
	for _, want := range []string{
		"note", "ADD COLUMN IF NOT EXISTS", "NOT NULL DEFAULT ''",
		"CHECK", "char_length(note) <= 1000",
	} {
		if !strings.Contains(noteMigration, want) {
			t.Errorf("migration 46 missing %q:\n%s", want, noteMigration)
		}
	}

	database := openTestSchema(t, "vera_test_quotation_note_migration")
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
		VALUES ('15029996', $1) RETURNING id`, schema), customerID).Scan(&quotationID); err != nil {
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
		t.Fatalf("insert revision without a note: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT note FROM %s.quotation_revisions WHERE id = $1`, schema),
		revisionID).Scan(&stored); err != nil {
		t.Fatalf("read note: %v", err)
	}
	if stored != "" {
		t.Errorf("legacy revision note = %q; want the empty default", stored)
	}

	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.quotation_revisions SET note = $2 WHERE id = $1`, schema),
		revisionID, strings.Repeat("a", 1001)); err == nil {
		t.Fatal("the note CHECK accepted 1001 characters")
	} else if !isSQLState(t, err, "23514") {
		t.Fatalf("over-limit note error = %v; want SQLSTATE 23514", err)
	}

	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.quotation_revisions SET note = $2 WHERE id = $1`, schema),
		revisionID, strings.Repeat("a", 1000)); err != nil {
		t.Fatalf("the note CHECK refused exactly 1000 characters: %v", err)
	}
}
