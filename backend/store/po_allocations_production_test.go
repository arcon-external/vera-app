package store

import (
	"context"
	"fmt"
	"testing"

	"vera-app/backend/poreport"
)

// Issue #650, acceptance criterion 6: the three production purchase orders
// carried Rp151.230.000 of revenue with no quotation allocation, so the report
// could not resolve their cost. This test rebuilds that shape from saved legacy
// records (no line reference, no allocation row), backfills each through the
// audited correction path, and pins the exact Actual report the operator saw
// after the corrections.

// legacyizeSavedPurchaseOrder turns a saved record into the pre-allocation
// production shape: the customer-facing values stay, but no line names a
// quotation leaf and no allocation row exists to resolve cost.
func legacyizeSavedPurchaseOrder(t *testing.T, database *Store, ctx context.Context, orderID int64) {
	t.Helper()
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_items
		SET quotation_item_no = NULL, quotation_sub_no = NULL
		WHERE purchase_order_id = $1`, database.configSchema()), orderID); err != nil {
		t.Fatalf("strip purchase order %d line references: %v", orderID, err)
	}
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_allocations WHERE purchase_order_id = $1`,
		database.configSchema()), orderID); err != nil {
		t.Fatalf("remove purchase order %d allocations: %v", orderID, err)
	}
	if allocations := readAllocations(t, database, ctx, orderID); len(allocations) != 0 {
		t.Fatalf("legacy fixture kept %d allocations; want none", len(allocations))
	}
	var referenced int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM %s.purchase_order_items
		WHERE purchase_order_id = $1 AND quotation_item_no IS NOT NULL`,
		database.configSchema()), orderID).Scan(&referenced); err != nil {
		t.Fatalf("count legacy references: %v", err)
	}
	if referenced != 0 {
		t.Fatalf("legacy fixture kept %d line references; want none", referenced)
	}
}

func TestAllocationBackfillReproducesTheThreeProductionPOs(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_production")
	ctx := poContext(t, database)
	user := poTestUser(t, database)

	fixtures := []struct {
		label          string
		customerCode   int
		customerName   string
		quoteQty       int64
		quantity       int64
		sell           int64
		cost           int64
		received       string
		number         string
		chosenQuantity bool
	}{
		{
			label: "Quote 70 item 1", customerCode: 2, customerName: "PT. Production 70",
			quoteQty: 10, quantity: 4, sell: 17_600_000, cost: 11_000_000,
			received: "2026-11-16", number: "PO-PROD-70", chosenQuantity: true,
		},
		{
			label: "Quote 61 item 1", customerCode: 3, customerName: "PT. Production 61",
			quoteQty: 10, quantity: 10, sell: 7_488_000, cost: 3_600_000,
			received: "2026-11-11", number: "PO-PROD-61",
		},
		{
			label: "Quote 45 item 1", customerCode: 4, customerName: "PT. Production 45",
			quoteQty: 1, quantity: 1, sell: 5_950_000, cost: 3_400_000,
			received: "2026-11-04", number: "PO-PROD-45",
		},
	}

	for _, fixture := range fixtures {
		quotation := seedReportQuotation(t, database, fixture.customerCode,
			fixture.customerName, fixture.label, "IDR", "FTZ",
			[]reportQuotationLine{{
				Description: fixture.label,
				UoM:         "Pc",
				Qty:         fixture.quoteQty,
				Cost:        fixture.cost,
				Sell:        fixture.sell,
			}})

		var mutate func(*PurchaseOrderFinalize)
		if fixture.chosenQuantity {
			mutate = func(input *PurchaseOrderFinalize) {
				input.Items[0].QuantityScaled = fixture.quantity * QuantityScale
				input.Decisions = []PurchaseOrderDecision{{
					Key: "line:1:quantity", Choice: PurchaseOrderChoicePO,
				}}
			}
		}
		saved := finalizeReportPO(t, database, ctx, user, quotation, fixture.number,
			fixture.received, PurchaseOrderBusinessCategoryFm, mutate)
		legacyizeSavedPurchaseOrder(t, database, ctx, saved.ID)

		current, err := database.GetPurchaseOrder(ctx, saved.ID)
		if err != nil {
			t.Fatalf("%s: read the legacy record: %v", fixture.label, err)
		}
		correction := correctionFromSavedOrder(current, user,
			"Backfill allocation for line 1 to Quotation item 1")
		itemNo := 1
		correction.Items[0].QuotationItemNo = &itemNo
		correction.Items[0].QuotationSubNo = nil
		corrected, err := database.CorrectPurchaseOrder(ctx, correction)
		if err != nil {
			t.Fatalf("%s: backfill the allocation: %v", fixture.label, err)
		}

		allocations := readAllocations(t, database, ctx, corrected.ID)
		if len(allocations) != 1 {
			t.Fatalf("%s: allocations = %d; want one", fixture.label, len(allocations))
		}
		allocation, ok := allocations["1.0"]
		if !ok {
			t.Fatalf("%s: no allocation for quotation item 1", fixture.label)
		}
		if allocation.QuotationID != quotation.ID ||
			allocation.SourceRevision != quotation.RevisionNumber {
			t.Errorf("%s: source = %d/%d; want %d/%d", fixture.label,
				allocation.QuotationID, allocation.SourceRevision,
				quotation.ID, quotation.RevisionNumber)
		}
		if allocation.QuantityScaled != fixture.quantity*QuantityScale {
			t.Errorf("%s: quantity = %d; want %d", fixture.label,
				allocation.QuantityScaled, fixture.quantity*QuantityScale)
		}
		if allocation.RevenueMinor != fixture.quantity*fixture.sell {
			t.Errorf("%s: revenue = %d; want %d", fixture.label,
				allocation.RevenueMinor, fixture.quantity*fixture.sell)
		}
		if allocation.QuotedUnitPriceMinor != fixture.sell {
			t.Errorf("%s: quoted baseline = %d; want %d", fixture.label,
				allocation.QuotedUnitPriceMinor, fixture.sell)
		}
		if allocation.CostUnitMinor != fixture.cost {
			t.Errorf("%s: cost = %d; want %d", fixture.label,
				allocation.CostUnitMinor, fixture.cost)
		}
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 3 {
		t.Errorf("poCount = %d; want 3", report.Totals.PoCount)
	}
	if report.Totals.AmountMinor != 151_230_000 {
		t.Errorf("amount = %d; want 151.230.000", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 151_230_000 {
		t.Errorf("costed = %d; want 151.230.000", report.Totals.CostedMinor)
	}
	if report.Totals.UncostedMinor != 0 {
		t.Errorf("uncosted = %d; want 0", report.Totals.UncostedMinor)
	}
	if report.Totals.CapitalMinor != 83_400_000 {
		t.Errorf("capital = %d; want 83.400.000", report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 67_830_000 {
		t.Errorf("margin = %d; want 67.830.000", report.Totals.MarginMinor)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 10000 {
		t.Errorf("coverage = %v; want 10000 bps", report.Totals.CoverageBps)
	}
	// RatioBps rounds half away from zero: 67.830.000 x 10000 / 83.400.000.
	if report.Totals.MarginBps == nil || *report.Totals.MarginBps != 8133 {
		t.Errorf("margin bps = %v; want 8133", report.Totals.MarginBps)
	}
}
