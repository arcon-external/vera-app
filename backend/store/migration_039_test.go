package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Issue #496 migration compatibility. The migration is applied to a schema
// that already holds pre-039 rows, exactly as a developer's or staff member's
// database does: the backfill must fill the new tax triple from the frozen
// source, must NOT move the optimistic-concurrency version (035's UPDATE
// trigger would otherwise bump every row), and must leave a former-source row
// alone. The three row shapes are then checked directly against the schema.
func TestMigration039BackfillsBasisWithoutBumpingExistingVersions(t *testing.T) {
	dsn := testDSN(t)
	schema := "vera_po_migration_039"
	resetTestSchema(t, dsn, schema)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for migration test: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoteIdentifier(schema)); err != nil {
		t.Fatalf("create test schema: %v", err)
	}

	migrations, err := loadMigrations(schema)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var migration039 migration
	for _, item := range migrations {
		switch {
		case item.version < 39:
			if _, err := pool.Exec(ctx, item.sql); err != nil {
				t.Fatalf("apply migration %d (%s): %v", item.version, item.name, err)
			}
		case item.version == 39:
			migration039 = item
		}
	}
	if migration039.version != 39 {
		t.Fatal("migration 039 is missing")
	}

	quoted := quoteIdentifier(schema)
	var customerID, userID, quotationID, revisionID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s.customers WHERE customer_code = 2`, quoted)).Scan(&customerID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.users (email, display_name, password_hash)
		VALUES ('migration-039@example.com', 'Migration', 'x') RETURNING id`, quoted)).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotations (quote_number, customer_id)
		VALUES ('MIG-039', $1) RETURNING id`, quoted), customerID).Scan(&quotationID); err != nil {
		t.Fatalf("seed quotation: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revisions (
			quotation_id, revision_number, recipient_name, recipient_address,
			subject, incoterm, validity_days, payment_term_days, lead_time_days,
			discount_percent, tax_zone, tax_category, tax_rate_bps,
			tax_type, service_category
		) VALUES ($1, 0, 'Recipient', 'Address', 'Subject', 'EXW', 30, 30, 10,
			0, 'NonFTZ', '', 0, 'Services', 'Construction') RETURNING id`, quoted),
		quotationID).Scan(&revisionID); err != nil {
		t.Fatalf("seed revision: %v", err)
	}
	var savedQuotationID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotations (quote_number, customer_id)
		VALUES ('MIG-039-SAVED', $1) RETURNING id`, quoted), customerID).Scan(&savedQuotationID); err != nil {
		t.Fatalf("seed saved quotation: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revisions (
			quotation_id, revision_number, recipient_name, recipient_address,
			subject, incoterm, validity_days, payment_term_days, lead_time_days,
			discount_percent, tax_zone, tax_category, tax_rate_bps,
			tax_type, service_category
		) VALUES ($1, 0, 'Recipient', 'Address', 'Subject', 'EXW', 30, 30, 10,
			0, 'NonFTZ', '', 0, 'Services', 'Construction')`, quoted),
		savedQuotationID); err != nil {
		t.Fatalf("seed saved revision: %v", err)
	}

	var draftID, savedID, formerID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id,
			customer_name, received_date, currency, tax_zone, tax_category,
			created_by, updated_by, version
		)
		SELECT o.id, $1, 0, 'draft', $2, 'PT. Migration', DATE '2026-10-01',
		       'IDR', 'NonFTZ', '', $3, $3, 7
		FROM %s.organizations o LIMIT 1
		RETURNING id`, quoted, quoted), quotationID, customerID, userID).Scan(&draftID); err != nil {
		t.Fatalf("seed pre-039 draft: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id,
			customer_name, received_date, currency, tax_zone, tax_category,
			po_number, po_number_key, saved_at, saved_by,
			created_by, updated_by, version
		)
		SELECT o.id, $1, 0, 'saved', $2, 'PT. Migration', DATE '2026-10-01',
		       'IDR', 'NonFTZ', '', 'MIG-PO-039', 'mig-po-039', NOW(), $3,
		       $3, $3, 11
		FROM %s.organizations o LIMIT 1
		RETURNING id`, quoted, quoted), savedQuotationID, customerID, userID).Scan(&savedID); err != nil {
		t.Fatalf("seed pre-039 saved record: %v", err)
	}
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id,
			customer_name, received_date, currency, tax_zone, tax_category,
			former_quotation_id, former_quotation_uuid, former_quote_number,
			former_source_revision, created_by, updated_by, version
		)
		SELECT o.id, NULL, 0, 'draft', $1, 'PT. Former Migration',
		       DATE '2026-10-01', 'IDR', 'NonFTZ', '',
		       999, gen_random_uuid(), 'FORMER-039', 0, $2, $2, 13
		FROM %s.organizations o LIMIT 1
		RETURNING id`, quoted, quoted), customerID, userID).Scan(&formerID); err != nil {
		t.Fatalf("seed pre-039 former record: %v", err)
	}

	if _, err := pool.Exec(ctx, migration039.sql); err != nil {
		t.Fatalf("apply migration 039: %v", err)
	}

	for _, want := range []struct {
		name    string
		id      int64
		version int
		taxType string
		service string
	}{
		{"linked draft", draftID, 7, "Services", "Construction"},
		{"saved record", savedID, 11, "Services", "Construction"},
		{"former record", formerID, 13, "", ""},
	} {
		var version int
		var taxType, serviceCategory string
		if err := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT version, tax_type, service_category
			FROM %s.purchase_orders WHERE id = $1`, quoted), want.id).Scan(
			&version, &taxType, &serviceCategory); err != nil {
			t.Fatalf("read %s: %v", want.name, err)
		}
		if version != want.version {
			t.Errorf("%s version = %d; want %d (the backfill must not bump it)",
				want.name, version, want.version)
		}
		if taxType != want.taxType || serviceCategory != want.service {
			t.Errorf("%s basis = %q/%q; want %q/%q",
				want.name, taxType, serviceCategory, want.taxType, want.service)
		}
	}
	if revisionID < 1 {
		t.Error("seed revision was not stored")
	}

	// 035's snapshot identities are nullable after 039, so a never-linked
	// overwrite can keep SQL NULL rather than a fabricated zero.
	var nullable int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = 'purchase_order_corrections'
		  AND column_name IN ('quotation_id', 'source_revision', 'customer_id')
		  AND is_nullable = 'YES'`, schema).Scan(&nullable); err != nil {
		t.Fatalf("read correction nullability: %v", err)
	}
	if nullable != 3 {
		t.Errorf("correction identity columns nullable = %d; want all three", nullable)
	}

	// The third shape exists only for a draft.
	var neverID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id,
			customer_name, received_date, currency, tax_zone, tax_category,
			created_by, updated_by
		)
		SELECT o.id, NULL, NULL, 'draft', NULL, '', DATE '2026-10-01',
		       'IDR', 'FTZ', '', $1, $1
		FROM %s.organizations o LIMIT 1
		RETURNING id`, quoted, quoted), userID).Scan(&neverID); err != nil {
		t.Fatalf("insert a never-linked draft: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET state = 'saved' WHERE id = $1`, quoted),
		neverID); err == nil {
		t.Error("a never-linked row was allowed into the saved state; the shape CHECK did not hold")
	}
}
