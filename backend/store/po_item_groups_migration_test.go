package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Issue #654: the PO-only aggregate layer. An aggregate line is a presentation
// header in purchase_order_item_groups; its children stay ordinary priced
// purchase_order_items rows that carry an aggregate_group_id. There is one
// aggregate level only, and a composite foreign key refuses a child from
// another purchase order.
//
// These tests prove the schema the store helpers rely on: the forward migration
// from 053 leaves every existing line ungrouped, the constraints refuse what the
// application must never write, and the correction snapshot stays immutable.

// seedItemGroupPurchaseOrder writes one pre-054 purchase order with two priced
// lines through raw SQL, so the migration under test is the only code that adds
// the aggregate columns.
func seedItemGroupPurchaseOrder(t *testing.T, database *Store) (orderID, firstLineID, secondLineID int64) {
	t.Helper()
	ctx := context.Background()
	quotation := seedBackfillQuotation(t, database)
	user := poTestUser(t, database)
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	schema := database.configSchema()
	var customerID int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s.customers WHERE customer_code = $1`, schema),
		quotation.CustomerCode).Scan(&customerID); err != nil {
		t.Fatalf("read fixture customer: %v", err)
	}
	var subtotal int64 = 300
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id, customer_name,
			received_date, currency, tax_zone, created_by, updated_by,
			subtotal_minor, ppn_bps, ppn_minor, grand_total_minor
		) VALUES ($1,$2,$3,'draft',$4,$5,$6,'IDR','FTZ',$7,$7,$8,0,0,$8)
		RETURNING id`, schema),
		org.ID, quotation.ID, quotation.RevisionNumber, customerID,
		"PT. Backfill Buyer", time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		user.ID, subtotal).Scan(&orderID); err != nil {
		t.Fatalf("insert fixture purchase order: %v", err)
	}
	insertLine := func(lineNumber int, net int64) int64 {
		t.Helper()
		var id int64
		if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_items (
				purchase_order_id, line_number, description,
				quantity_scaled, uom, unit_price_minor,
				gross_minor, discount_minor, net_minor)
			VALUES ($1,$2,$3,$4,'Pc',$5,$5,0,$5)
			RETURNING id`, schema),
			orderID, lineNumber, fmt.Sprintf("Fixture line %d", lineNumber),
			QuantityScale, net).Scan(&id); err != nil {
			t.Fatalf("insert fixture line %d: %v", lineNumber, err)
		}
		return id
	}
	firstLineID = insertLine(1, 200)
	secondLineID = insertLine(2, 100)
	return orderID, firstLineID, secondLineID
}

// TestItemGroupMigrationFrom053AddsTheAggregateLayer proves the additive
// upgrade: the tables and the membership column appear, and no existing line is
// silently grouped.
func TestItemGroupMigrationFrom053AddsTheAggregateLayer(t *testing.T) {
	database := po518Unmigrated(t, "vera_po_654_migration")
	po518ApplyThrough(t, database, 53)
	ctx := context.Background()

	orderID, _, _ := seedItemGroupPurchaseOrder(t, database)
	schema := database.configSchema()
	// configSchema() returns a quoted identifier for interpolation; the
	// information_schema predicates need the bare name.
	schemaName := database.config.Schema

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("apply migration 054: %v", err)
	}

	var headerColumns int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'purchase_order_item_groups'
		  AND column_name IN (
			'id', 'org_id', 'purchase_order_id', 'display_order', 'title',
			'ordered_quantity_scaled', 'ordered_uom',
			'show_children_to_customer', 'created_at', 'updated_at')`,
		schemaName).Scan(&headerColumns); err != nil {
		t.Fatalf("read aggregate header columns: %v", err)
	}
	if headerColumns != 10 {
		t.Errorf("purchase_order_item_groups columns = %d; want 10", headerColumns)
	}

	var membershipColumn int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'purchase_order_items'
		  AND column_name = 'aggregate_group_id'`, schemaName).Scan(&membershipColumn); err != nil {
		t.Fatalf("read membership column: %v", err)
	}
	if membershipColumn != 1 {
		t.Errorf("purchase_order_items.aggregate_group_id columns = %d; want 1", membershipColumn)
	}

	// Proof the fixture rows survive the migration as ungrouped rows: a
	// backfill that invented a group would contradict §3.2's no-inference rule.
	var grouped int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(*) FROM %s.purchase_order_items
		WHERE purchase_order_id = $1 AND aggregate_group_id IS NOT NULL`, schema),
		orderID).Scan(&grouped); err != nil {
		t.Fatalf("count grouped fixture lines: %v", err)
	}
	if grouped != 0 {
		t.Errorf("grouped fixture lines = %d; want 0 (grouping is staff intent)", grouped)
	}
	var groups int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT count(*) FROM %s.purchase_order_item_groups`, schema)).Scan(&groups); err != nil {
		t.Fatalf("count fixture groups: %v", err)
	}
	if groups != 0 {
		t.Errorf("groups after migration = %d; want 0", groups)
	}

	var correctionTables int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = $1 AND table_name IN (
			'purchase_order_correction_item_groups')`, schemaName).Scan(&correctionTables); err != nil {
		t.Fatalf("read correction snapshot table: %v", err)
	}
	if correctionTables != 1 {
		t.Errorf("purchase_order_correction_item_groups tables = %d; want 1", correctionTables)
	}
}

// insertItemGroupHeader writes one aggregate header through raw SQL; the tests
// below exercise the constraints rather than the store helper.
func insertItemGroupHeader(t *testing.T, database *Store, orderID int64, displayOrder int, title string, quantity int64, uom string) int64 {
	t.Helper()
	ctx := context.Background()
	schema := database.configSchema()
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	var id int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_item_groups (
			org_id, purchase_order_id, display_order, title,
			ordered_quantity_scaled, ordered_uom
		) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id`, schema),
		org.ID, orderID, displayOrder, title, quantity, uom).Scan(&id); err != nil {
		t.Fatalf("insert aggregate header: %v", err)
	}
	return id
}

// TestItemGroupCrossPurchaseOrderAttachmentIsRefused proves the composite key
// is the guard, not the application: a raw statement that moves a line of one
// purchase order onto another record's group is refused with a foreign-key
// violation. The subtest removes the constraint and shows the same statement
// succeed, so the refusal is attributable to the key (§10.6 red proof 1).
func TestItemGroupCrossPurchaseOrderAttachmentIsRefused(t *testing.T) {
	database := openTestSchema(t, "vera_po_654_cross_po")
	ctx := context.Background()
	firstOrder, firstLine, _ := seedItemGroupPurchaseOrder(t, database)
	_, secondLine, _ := seedItemGroupPurchaseOrder(t, database)
	groupID := insertItemGroupHeader(t, database, firstOrder, 1, "Package A", 10000, "Lot")
	schema := database.configSchema()

	attach := `UPDATE ` + schema + `.purchase_order_items
		SET aggregate_group_id = $1 WHERE id = $2`

	// The child's own purchase order is not the group's: refused.
	_, err := database.pool.Exec(ctx, attach, groupID, secondLine)
	if !isSQLState(t, err, "23503") {
		t.Fatalf("cross-PO attach error = %v; want SQLSTATE 23503", err)
	}

	// The child's own purchase order is the group's: accepted.
	if _, err := database.pool.Exec(ctx, attach, groupID, firstLine); err != nil {
		t.Fatalf("same-PO attach: %v", err)
	}
	var grouped int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(*) FROM %s.purchase_order_items
		WHERE aggregate_group_id = $1`, schema), groupID).Scan(&grouped); err != nil {
		t.Fatalf("count grouped children: %v", err)
	}
	if grouped != 1 {
		t.Errorf("grouped children = %d; want 1", grouped)
	}

	t.Run("without the key the same statement succeeds", func(t *testing.T) {
		tx, err := database.pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin red-proof transaction: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`ALTER TABLE %s.purchase_order_items
			 DROP CONSTRAINT purchase_order_items_aggregate_group_fkey`, schema)); err != nil {
			t.Fatalf("drop membership key: %v", err)
		}
		if _, err := tx.Exec(ctx, attach, groupID, secondLine); err != nil {
			t.Fatalf("attach without the key = %v; want success (the key is the guard)", err)
		}
	})
}

// TestItemGroupDeleteClearsMembershipAndPurchaseOrderDeleteCascades proves the
// two deletion paths: removing a group returns its children to normal lines,
// and removing the purchase order removes its groups.
func TestItemGroupDeleteClearsMembershipAndPurchaseOrderDeleteCascades(t *testing.T) {
	database := openTestSchema(t, "vera_po_654_cascade")
	ctx := context.Background()
	orderID, lineID, _ := seedItemGroupPurchaseOrder(t, database)
	schema := database.configSchema()
	groupID := insertItemGroupHeader(t, database, orderID, 1, "Package", 10000, "Lot")
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_order_items SET aggregate_group_id = $1 WHERE id = $2`, schema),
		groupID, lineID); err != nil {
		t.Fatalf("attach child: %v", err)
	}

	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_item_groups WHERE id = $1`, schema), groupID); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	var membership *int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT aggregate_group_id FROM %s.purchase_order_items WHERE id = $1`, schema),
		lineID).Scan(&membership); err != nil {
		t.Fatalf("read child membership: %v", err)
	}
	if membership != nil {
		t.Errorf("child membership = %d after group delete; want NULL", *membership)
	}
	var lineCount int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT count(*) FROM %s.purchase_order_items WHERE purchase_order_id = $1`, schema),
		orderID).Scan(&lineCount); err != nil {
		t.Fatalf("count surviving lines: %v", err)
	}
	if lineCount != 2 {
		t.Errorf("lines after group delete = %d; want 2 (deleting a group never deletes a priced line)", lineCount)
	}

	secondGroup := insertItemGroupHeader(t, database, orderID, 2, "Another", 20000, "Pc")
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_orders WHERE id = $1`, schema), orderID); err != nil {
		t.Fatalf("delete purchase order: %v", err)
	}
	var groups int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(*) FROM %s.purchase_order_item_groups
		WHERE id = $1`, schema), secondGroup).Scan(&groups); err != nil {
		t.Fatalf("count cascaded groups: %v", err)
	}
	if groups != 0 {
		t.Errorf("groups after purchase order delete = %d; want 0", groups)
	}
}

// TestItemGroupSchemaChecksAndUniqueness proves the header's own bounds and the
// one-level shape: no parent column exists, so a group cannot be a child.
func TestItemGroupSchemaChecksAndUniqueness(t *testing.T) {
	database := openTestSchema(t, "vera_po_654_checks")
	ctx := context.Background()
	orderID, _, _ := seedItemGroupPurchaseOrder(t, database)
	schema := database.configSchema()

	insert := func(displayOrder int, title string, quantity int64, uom string) error {
		t.Helper()
		_, err := database.pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_item_groups (
				org_id, purchase_order_id, display_order, title,
				ordered_quantity_scaled, ordered_uom
			)
			SELECT org_id, $1, $2, $3, $4, $5
			FROM %s.purchase_orders WHERE id = $1`, schema, schema),
			orderID, displayOrder, title, quantity, uom)
		return err
	}

	cases := []struct {
		name         string
		displayOrder int
		title        string
		quantity     int64
		uom          string
		state        string
	}{
		{"blank title", 1, "   ", 10000, "Lot", "23514"},
		{"zero quantity", 1, "Package", 0, "Lot", "23514"},
		{"negative quantity", 1, "Package", -1, "Lot", "23514"},
		{"quantity above the exact range", 1, "Package", 1 << 53, "Lot", "23514"},
		{"blank unit", 1, "Package", 10000, "  ", "23514"},
		{"unit above 80 characters", 1, "Package", 10000, strings.Repeat("x", 81), "23514"},
		{"zero display order", 0, "Package", 10000, "Lot", "23514"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := insert(tc.displayOrder, tc.title, tc.quantity, tc.uom)
			if !isSQLState(t, err, tc.state) {
				t.Fatalf("insert %s error = %v; want SQLSTATE %s", tc.name, err, tc.state)
			}
		})
	}

	// A valid header, then a second one at the same display_order: the uniqueness
	// that keeps the read order deterministic refuses it.
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_item_groups (
			org_id, purchase_order_id, display_order, title)
		SELECT org_id, $1, 1, 'Package'
		FROM %s.purchase_orders WHERE id = $1`, schema, schema), orderID); err != nil {
		t.Fatalf("insert first group: %v", err)
	}
	if err := insert(1, "Duplicate position", 10000, "Lot"); !isSQLState(t, err, "23505") {
		t.Fatalf("duplicate display_order error = %v; want SQLSTATE 23505", err)
	}

	// One level is structural: the header table has no parent pointer at all.
	var parentColumns int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'purchase_order_item_groups'
		  AND column_name LIKE 'parent%%'`, database.config.Schema).Scan(&parentColumns); err != nil {
		t.Fatalf("read parent columns: %v", err)
	}
	if parentColumns != 0 {
		t.Errorf("group parent columns = %d; want 0 (an aggregate cannot contain an aggregate)", parentColumns)
	}
	var groupToGroupKeys int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN pg_class rt ON rt.oid = c.confrelid
		WHERE n.nspname = $1 AND t.relname = 'purchase_order_item_groups'
		  AND c.contype = 'f' AND rt.relname = 'purchase_order_item_groups'`,
		database.config.Schema).Scan(&groupToGroupKeys); err != nil {
		t.Fatalf("read group-to-group keys: %v", err)
	}
	if groupToGroupKeys != 0 {
		t.Errorf("group-to-group foreign keys = %d; want 0", groupToGroupKeys)
	}
}

// TestItemGroupCorrectionSnapshotIsImmutable proves the snapshot tables exist,
// the composite key ties a snapshot line to a group in the same snapshot, and
// the existing immutability trigger refuses a rewrite.
func TestItemGroupCorrectionSnapshotIsImmutable(t *testing.T) {
	database := openTestSchema(t, "vera_po_654_correction")
	ctx := context.Background()
	orderID, _, _ := seedItemGroupPurchaseOrder(t, database)
	user := poTestUser(t, database)
	schema := database.configSchema()
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}

	var correctionID int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_corrections (
			org_id, purchase_order_id, previous_version, resulting_version, kind,
			state, reason, actor, quotation_id, source_revision, customer_id,
			currency, received_date, subtotal_minor, discount_minor, ppn_bps,
			ppn_minor, adjustment_minor, grand_total_minor
		)
		SELECT $1, $2, 1, 2, 'correction', 'draft', 'fixture', $3,
		       COALESCE(quotation_id, former_quotation_id, 0), source_revision,
		       customer_id, currency, received_date, 300, 0, 0, 0, 0, 300
		FROM %s.purchase_orders WHERE id = $2
		RETURNING id`, schema, schema), org.ID, orderID, user.ID).Scan(&correctionID); err != nil {
		t.Fatalf("insert correction header: %v", err)
	}
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_correction_item_groups (
			correction_id, display_order, title, ordered_quantity_scaled,
			ordered_uom, show_children_to_customer
		) VALUES ($1, 1, 'Package', 10000, 'Lot', FALSE)`, schema), correctionID); err != nil {
		t.Fatalf("insert snapshot group: %v", err)
	}
	insertSnapshottedLine := func(lineNumber int, groupOrder *int) error {
		_, err := database.pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_correction_items (
				correction_id, line_number, description, quantity_scaled, uom,
				unit_price_minor, gross_minor, discount_minor, net_minor,
				aggregate_group_order
			) VALUES ($1, $2, 'Snapshot line', 10000, 'Pc', 100, 100, 0, 100, $3)`, schema),
			correctionID, lineNumber, groupOrder)
		return err
	}
	order := 1
	if err := insertSnapshottedLine(1, &order); err != nil {
		t.Fatalf("insert grouped snapshot line: %v", err)
	}
	missing := 9
	if err := insertSnapshottedLine(2, &missing); !isSQLState(t, err, "23503") {
		t.Fatalf("snapshot line naming a missing group = %v; want SQLSTATE 23503", err)
	}
	if err := insertSnapshottedLine(3, nil); err != nil {
		t.Fatalf("insert ungrouped snapshot line: %v", err)
	}

	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_correction_item_groups SET title = 'Rewritten'
		WHERE correction_id = $1`, schema), correctionID); !isSQLState(t, err, "23514") {
		t.Fatalf("rewrite snapshot group = %v; want SQLSTATE 23514 (snapshots are evidence)", err)
	}
}
