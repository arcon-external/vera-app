package store

import (
	"context"
	"strings"
	"testing"
)

// TestMarginAmountAcceptsALoss pins the floor on margin_amount.
//
// Since migration 020 a row can carry its own discount, and a row discounted
// below what we paid for it has a negative margin amount -- the honest figure,
// and the one the editor shows. The column still carried CHECK (>= 0) from 001,
// so exporting any quotation with such a line failed outright:
//
//	new row for relation "quotation_revision_items" violates check
//	constraint "quotation_revision_items_margin_amount_check"
//
// The floor is now -cost, which still catches nonsense (a margin impossibly far
// below free) while allowing a loss.
func TestMarginAmountAcceptsALoss(t *testing.T) {
	database := openTestSchema(t, "vera_test_signed_margin")
	ctx := context.Background()
	schema := "vera_test_signed_margin"

	// Seed the parent rows a revision needs: a customer, a quotation, and one
	// revision. Direct SQL because the constraint is what is under test, not
	// the draft plumbing above it.
	var customerID int64
	if err := database.pool.QueryRow(ctx,
		`SELECT id FROM `+schema+`.customers ORDER BY id LIMIT 1`).Scan(&customerID); err != nil {
		t.Fatalf("no seeded customer to attach a quotation to: %v", err)
	}
	var quotationID int64
	if err := database.pool.QueryRow(ctx, `
		INSERT INTO `+schema+`.quotations (quote_number, customer_id)
		VALUES ('9999000001', $1) RETURNING id`, customerID).Scan(&quotationID); err != nil {
		t.Fatalf("insert quotation: %v", err)
	}
	var revisionID int64
	if err := database.pool.QueryRow(ctx, `
		INSERT INTO `+schema+`.quotation_revisions
			(quotation_id, revision_number, recipient_name, recipient_address, subject,
			 incoterm, validity_days, payment_term_days, lead_time_days,
			 discount_percent, tax_zone, tax_category, tax_rate_bps)
		VALUES ($1, 0, 'r', 'a', 's', 'EXW', 30, 30, 30, 0, 'FTZ', '', 0)
		RETURNING id`, quotationID).Scan(&revisionID); err != nil {
		t.Fatalf("insert revision: %v", err)
	}

	// One line per case: (revision_id, item_no) is unique.
	nextItemNo := 1
	insertItem := func(margin int64) error {
		itemNo := nextItemNo
		nextItemNo++
		_, err := database.pool.Exec(ctx, `
			INSERT INTO `+schema+`.quotation_revision_items
				(revision_id, item_no, description, qty, uom, unit_price, margin_enabled,
				 margin_percent, margin_amount, original_total, marginalized_unit_price,
				 marginalized_total, show_sub_prices)
			VALUES ($1, $2, 'probe', 1, 'Lot', 10000, TRUE, 0, $3, 10000, 7500, 7500, FALSE)`,
			revisionID, itemNo, margin)
		return err
	}

	// A loss: sold at 7.500 against a 10.000 cost, so the margin is -2.500.
	if err := insertItem(-2500); err != nil {
		t.Fatalf("a line discounted below cost must store its negative margin, got: %v", err)
	}
	// A line given away entirely is still a real quotation.
	if err := insertItem(-10000); err != nil {
		t.Fatalf("a free line (margin -cost) must store, got: %v", err)
	}
	// Below free is not a price at all, and the constraint must still say so.
	if err := insertItem(-10001); err == nil {
		t.Fatal("a margin below -cost was accepted; the constraint no longer guards anything")
	} else if !strings.Contains(err.Error(), "margin_amount_check") {
		t.Fatalf("rejected for the wrong reason: %v", err)
	}

	// The same floor on the sub-item table, which spells cost and sell apart and
	// so needs -unit_cost rather than -unit_price.
	var itemID int64
	if err := database.pool.QueryRow(ctx,
		`SELECT id FROM `+schema+`.quotation_revision_items WHERE revision_id = $1 ORDER BY id LIMIT 1`,
		revisionID).Scan(&itemID); err != nil {
		t.Fatalf("read back item: %v", err)
	}
	insertSub := func(margin int64) error {
		_, err := database.pool.Exec(ctx, `
			INSERT INTO `+schema+`.quotation_revision_sub_items
				(revision_item_id, sub_no, description, qty, uom, unit_cost, margin_enabled,
				 margin_percent, unit_price, margin_amount, total)
			VALUES ($1, 1, 'probe part', 1, 'Pc', 10000, TRUE, 0, 7500, $2, 7500)`,
			itemID, margin)
		return err
	}
	if err := insertSub(-2500); err != nil {
		t.Fatalf("a sub-item discounted below cost must store its negative margin, got: %v", err)
	}
	if err := insertSub(-10001); err == nil {
		t.Fatal("a sub-item margin below -cost was accepted")
	} else if !strings.Contains(err.Error(), "margin_amount_check") {
		t.Fatalf("sub-item rejected for the wrong reason: %v", err)
	}
}
