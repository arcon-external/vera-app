package store

import (
	"context"
	"fmt"
	"testing"

	"vera-app/backend/poreport"
)

// Issue #650: a quotation-backed PO line owns exactly one allocation, and the
// report resolves cost from that allocation before falling back to the legacy
// quotation-reference join.

type storedAllocation struct {
	QuotationID          int64
	SourceRevision       int16
	QuotationItemNo      int
	QuotationSubNo       int
	QuantityScaled       int64
	SourceUoM            string
	OrderedUoM           string
	QuotedUnitPriceMinor int64
	RevenueMinor         int64
	CostUnitMinor        int64
}

func readAllocations(t *testing.T, database *Store, ctx context.Context, orderID int64) map[string]storedAllocation {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT quotation_id, source_revision, quotation_item_no,
		       COALESCE(quotation_sub_no, 0), quantity_scaled, source_uom,
		       ordered_uom, quoted_unit_price_minor, revenue_minor,
		       COALESCE(cost_unit_minor, -1)
		FROM %s.purchase_order_allocations
		WHERE purchase_order_id = $1
		ORDER BY quotation_item_no, quotation_sub_no NULLS FIRST`, database.configSchema()), orderID)
	if err != nil {
		t.Fatalf("read allocations: %v", err)
	}
	defer rows.Close()
	result := map[string]storedAllocation{}
	for rows.Next() {
		var row storedAllocation
		if err := rows.Scan(&row.QuotationID, &row.SourceRevision, &row.QuotationItemNo,
			&row.QuotationSubNo, &row.QuantityScaled, &row.SourceUoM,
			&row.OrderedUoM, &row.QuotedUnitPriceMinor, &row.RevenueMinor,
			&row.CostUnitMinor); err != nil {
			t.Fatalf("scan allocation: %v", err)
		}
		result[fmt.Sprintf("%d.%d", row.QuotationItemNo, row.QuotationSubNo)] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate allocations: %v", err)
	}
	return result
}

func TestCreateFromQuotationWritesOneAllocationPerLine(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_create")
	quotation := seedPOQuotation(t, database, "PT. Allocation Buyer", "Allocation source")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO draft: %v", err)
	}
	allocations := readAllocations(t, database, ctx, draft.ID)
	if len(allocations) != 3 {
		t.Fatalf("allocations = %d; want 3 (one per priced source line)", len(allocations))
	}

	tests := []struct {
		key        string
		quantity   int64
		sourceUoM  string
		orderedUoM string
		quoted     int64
		cost       int64
	}{
		{key: "1.0", quantity: 30000, sourceUoM: "Pc", orderedUoM: "Pc", quoted: 600, cost: 500},
		{key: "2.1", quantity: 20000, sourceUoM: "Unit", orderedUoM: "Unit", quoted: 400, cost: 300},
		{key: "2.2", quantity: 10000, sourceUoM: "Lot", orderedUoM: "Lot", quoted: 450, cost: 200},
	}
	for _, test := range tests {
		allocation, ok := allocations[test.key]
		if !ok {
			t.Errorf("missing allocation %s", test.key)
			continue
		}
		if allocation.QuotationID != quotation.ID || allocation.SourceRevision != quotation.RevisionNumber {
			t.Errorf("%s source = %d/%d; want %d/%d", test.key,
				allocation.QuotationID, allocation.SourceRevision,
				quotation.ID, quotation.RevisionNumber)
		}
		if allocation.QuantityScaled != test.quantity {
			t.Errorf("%s quantity = %d; want %d", test.key, allocation.QuantityScaled, test.quantity)
		}
		if allocation.SourceUoM != test.sourceUoM || allocation.OrderedUoM != test.orderedUoM {
			t.Errorf("%s UoM = %q/%q; want %q/%q", test.key,
				allocation.SourceUoM, allocation.OrderedUoM, test.sourceUoM, test.orderedUoM)
		}
		if allocation.QuotedUnitPriceMinor != test.quoted {
			t.Errorf("%s quoted baseline = %d; want %d", test.key,
				allocation.QuotedUnitPriceMinor, test.quoted)
		}
		if allocation.CostUnitMinor != test.cost {
			t.Errorf("%s cost = %d; want %d", test.key, allocation.CostUnitMinor, test.cost)
		}
		// The allocation revenue is the PO line's own net, not the quotation's
		// list total: the final save may change quantity or price.
		if allocation.RevenueMinor <= 0 {
			t.Errorf("%s revenue = %d; want a positive stored line net", test.key,
				allocation.RevenueMinor)
		}
	}

	// The uniqueness guard: a PO line cannot own two allocations.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var duplicate int
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM (
			SELECT purchase_order_item_id FROM %s.purchase_order_allocations
			WHERE purchase_order_id = $1
			GROUP BY purchase_order_item_id HAVING COUNT(*) > 1
		) duplicates`, database.configSchema()), draft.ID).Scan(&duplicate); err != nil {
		t.Fatalf("read duplicate allocations: %v", err)
	}
	if duplicate != 0 {
		t.Errorf("duplicate allocation lines = %d; want 0", duplicate)
	}
}

func TestFinalizeRewritesAllocationToTheFinalLine(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_finalize")
	quotation := seedPOQuotation(t, database, "PT. Finalize Buyer", "Finalize allocation")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO draft: %v", err)
	}
	input := finalizeInput(draft, user, "PO-ALLOC-1")
	input.Items[0].QuantityScaled = 20000
	input.Decisions = []PurchaseOrderDecision{
		{Key: "line:1:quantity", Choice: PurchaseOrderChoicePO},
	}
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize PO: %v", err)
	}

	allocation, ok := readAllocations(t, database, ctx, saved.ID)["1.0"]
	if !ok {
		t.Fatalf("no allocation for the finalized item")
	}
	if allocation.QuantityScaled != 20000 {
		t.Errorf("allocation quantity = %d; want the finalized 2.0000", allocation.QuantityScaled)
	}
	if allocation.RevenueMinor != saved.Items[0].NetMinor {
		t.Errorf("allocation revenue = %d; want the finalized line net %d",
			allocation.RevenueMinor, saved.Items[0].NetMinor)
	}
}

func TestReportUsesAllocationRevenueNotQuotationPrice(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_report")
	quotation := seedReportQuotation(t, database, 2, "PT. Report Buyer", "Allocation report",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Allocated line",
			UoM:         "Pc",
			Qty:         2,
			Cost:        100,
			Sell:        250,
		}})
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := finalizeReportPO(t, database, ctx, user, quotation,
		"PO-ALLOC-REPORT", "2026-11-15", PurchaseOrderBusinessCategoryOthers, nil)

	// Make the stored allocation deliberately different from the quotation's
	// sell price. The report must use the allocation revenue and cost snapshot,
	// not re-price the line from the quotation.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_allocations
		SET revenue_minor = 400, cost_unit_minor = 50
		WHERE purchase_order_id = $1`, database.configSchema()), saved.ID); err != nil {
		t.Fatalf("update allocation: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-11-01", "2026-12-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 {
		t.Fatalf("poCount = %d; want 1", report.Totals.PoCount)
	}
	if report.Totals.AmountMinor != 500 {
		t.Errorf("amount = %d; want the PO's 2 x 250 = 500", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 400 {
		t.Errorf("costed = %d; want the allocation revenue 400", report.Totals.CostedMinor)
	}
	if report.Totals.CapitalMinor != 100 {
		t.Errorf("capital = %d; want 2 x 50 allocation cost = 100", report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 300 {
		t.Errorf("margin = %d; want 400 - 100 = 300", report.Totals.MarginMinor)
	}
	if report.Totals.UncostedMinor != 100 {
		t.Errorf("uncosted = %d; want 500 - 400 = 100", report.Totals.UncostedMinor)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 8000 {
		t.Errorf("coverage = %v; want 8000 bps", report.Totals.CoverageBps)
	}
}
