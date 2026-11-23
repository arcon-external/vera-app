package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Issue #650, acceptance criterion 4: migration 052 backfills only the lines
// that already name a quotation leaf, keeps their stored PO revenue, freezes the
// row-discounted quotation baseline and the supplier cost, and leaves a
// null-reference line alone. The fixture is a genuine pre-052 schema so the
// migration's own SELECT is what writes the allocation.

// seedBackfillQuotation issues one revision whose list price is 250 and whose
// row-discounted sell is 200, with a 100 supplier cost. The PO below is priced
// at 150, so a backfill that used the quotation total (or the list price) is
// distinguishable from the stored PO revenue and from the discounted baseline.
func seedBackfillQuotation(t *testing.T, database *Store) QuotationRecord {
	t.Helper()
	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "PT. Backfill Buyer",
		CustomerAddress:  "Jl. Backfill 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Backfill Recipient 9",
		Subject:          "Backfill source",
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		DiscountPercent:  0,
		TaxZone:          "FTZ",
		TaxType:          "Goods",
		Currency:         "IDR",
		HTMLSnapshot:     "<html>backfill fixture</html>",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "Discounted line",
			UoM:                   "Pc",
			Qty:                   3,
			UnitPrice:             100,
			MarginEnabled:         true,
			MarginPercent:         150,
			MarginAmount:          150,
			OriginalTotal:         300,
			MarginalizedUnitPrice: 200,
			MarginalizedTotal:     600,
			ListUnitPrice:         250,
			DiscountEnabled:       true,
			DiscountPercent:       20,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:          300,
			OriginalGrand:             300,
			MarginalizedSubtotal:      600,
			MarginalizedGrand:         600,
			MarginalizedGrandAfterTax: 600,
			CustomerGrandTotal:        600,
		},
	})
	if err != nil {
		t.Fatalf("seed backfill quotation: %v", err)
	}
	return record
}

func TestAllocationBackfillFromMigration052(t *testing.T) {
	database := po518Unmigrated(t, "vera_po_052_backfill")
	po518ApplyThrough(t, database, 51)
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
		t.Fatalf("read backfill customer: %v", err)
	}

	// Three lines: one duplicate source leaf, the matched historical line, and
	// the null-reference legacy line. The duplicate is inserted first so
	// physical row order disagrees with line_number order: a backfill that
	// followed heap order would give item 1 to line 3, while the ordered
	// backfill must choose line 1. Line 1's stored net (300) differs from the
	// quotation's sell total (2 x 200 = 400) because the PO negotiated 150 a
	// unit.
	subtotal := int64(300 + 50 + 999)
	var orderID int64
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
		t.Fatalf("insert pre-052 purchase order: %v", err)
	}

	insertLine := func(lineNumber int, itemNo *int, subNo *int, quantity, price, net int64) int64 {
		t.Helper()
		var id int64
		if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_items (
				purchase_order_id, line_number, description,
				quotation_item_no, quotation_sub_no,
				quantity_scaled, uom, unit_price_minor,
				gross_minor, discount_minor, net_minor)
			VALUES ($1,$2,'Backfill line',$3,$4,$5,'Pc',$6,$7,0,$7)
			RETURNING id`, schema),
			orderID, lineNumber, itemNo, subNo, quantity, price, net).Scan(&id); err != nil {
			t.Fatalf("insert pre-052 line %d: %v", lineNumber, err)
		}
		return id
	}
	itemNo := 1
	duplicateLineID := insertLine(3, &itemNo, nil, QuantityScale, 999, 999)
	matchedLineID := insertLine(1, &itemNo, nil, 2*QuantityScale, 150, 300)
	nullLineID := insertLine(2, nil, nil, QuantityScale, 50, 50)

	// Apply the real migration: its backfill SELECT is the code under test.
	po518ApplyOnly(t, database, 52)

	type backfilledAllocation struct {
		ItemID               int64
		QuantityScaled       int64
		SourceUoM            string
		OrderedUoM           string
		QuotedUnitPriceMinor int64
		RevenueMinor         int64
		CostUnitMinor        int64
	}
	rows, err := database.pool.Query(ctx, fmt.Sprintf(`
		SELECT purchase_order_item_id, quantity_scaled, source_uom, ordered_uom,
		       quoted_unit_price_minor, revenue_minor, COALESCE(cost_unit_minor, -1)
		FROM %s.purchase_order_allocations
		WHERE purchase_order_id = $1
		ORDER BY purchase_order_item_id`, schema), orderID)
	if err != nil {
		t.Fatalf("read backfilled allocations: %v", err)
	}
	defer rows.Close()
	allocations := make([]backfilledAllocation, 0, 2)
	for rows.Next() {
		var row backfilledAllocation
		if err := rows.Scan(&row.ItemID, &row.QuantityScaled, &row.SourceUoM,
			&row.OrderedUoM, &row.QuotedUnitPriceMinor, &row.RevenueMinor,
			&row.CostUnitMinor); err != nil {
			t.Fatalf("scan backfilled allocation: %v", err)
		}
		allocations = append(allocations, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate backfilled allocations: %v", err)
	}
	if len(allocations) != 1 {
		t.Fatalf("backfilled allocations = %d; want exactly the matched line", len(allocations))
	}
	allocation := allocations[0]
	if allocation.ItemID != matchedLineID {
		t.Errorf("backfilled line = %d; want the lowest line number %d (not %d or %d)",
			allocation.ItemID, matchedLineID, nullLineID, duplicateLineID)
	}
	if allocation.RevenueMinor != 300 {
		t.Errorf("revenue = %d; want the stored PO net 300", allocation.RevenueMinor)
	}
	if allocation.QuotedUnitPriceMinor != 200 {
		t.Errorf("quoted baseline = %d; want the row-discounted 200 (list is 250)",
			allocation.QuotedUnitPriceMinor)
	}
	if allocation.CostUnitMinor != 100 {
		t.Errorf("cost = %d; want the frozen supplier cost 100", allocation.CostUnitMinor)
	}
	if allocation.QuantityScaled != 2*QuantityScale {
		t.Errorf("quantity = %d; want %d", allocation.QuantityScaled, 2*QuantityScale)
	}
	if allocation.SourceUoM != "Pc" || allocation.OrderedUoM != "Pc" {
		t.Errorf("units = %q/%q; want Pc/Pc", allocation.SourceUoM, allocation.OrderedUoM)
	}
}
