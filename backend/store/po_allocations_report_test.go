package store

import (
	"fmt"
	"testing"

	"vera-app/backend/poreport"
)

// Issue #650: the Actual report resolves each item independently. One saved PO
// can hold an allocated line, a legacy matched line that predates the backfill,
// and a null-reference line that has no cost identity at all; capital still
// rounds once per PO. A live allocation that no longer matches the PO's source
// must be ignored, not trusted for its revenue.

func TestPurchaseOrderReportResolvesMixedAllocationAndLegacyLines(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_mixed")
	quotation := seedReportQuotation(t, database, 2, "PT. Mixed Buyer", "Mixed allocation report",
		"IDR", "FTZ", []reportQuotationLine{
			{Description: "Allocated line", UoM: "Pc", Qty: 2, Cost: 333, Sell: 400},
			{Description: "Legacy matched line", UoM: "Pc", Qty: 2, Cost: 333, Sell: 400},
			{Description: "Legacy null line", UoM: "Pc", Qty: 1, Cost: 10, Sell: 30},
		})
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := finalizeReportPO(t, database, ctx, user, quotation,
		"PO-ALLOC-MIXED", "2026-03-10", PurchaseOrderBusinessCategoryFm, nil)

	// Half-unit quantities make capital expose the rounding seam: each of the
	// two sourced lines owns 499.5 minor units, and only one round per PO gives
	// 999 instead of the per-line 500 + 500.
	correction := correctionFromSavedOrder(saved, user, "mixed report fixture quantities")
	for index := range correction.Items {
		if index < 2 {
			correction.Items[index].QuantityScaled = 15000
		} else {
			correction.Items[index].QuantityScaled = 10000
		}
	}
	corrected, err := database.CorrectPurchaseOrder(ctx, correction)
	if err != nil {
		t.Fatalf("set mixed fixture quantities: %v", err)
	}
	if len(corrected.Items) != 3 {
		t.Fatalf("mixed fixture items = %d; want 3", len(corrected.Items))
	}

	// Line 1: keep the allocation, but make its revenue deliberately different
	// from the quotation's sell so the report can only get 555 from the
	// allocation row.
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_allocations SET revenue_minor = 555
		WHERE purchase_order_item_id = $1`, database.configSchema()),
		corrected.Items[0].ID); err != nil {
		t.Fatalf("set the allocated line revenue: %v", err)
	}
	// Line 2: drop the allocation to simulate a saved line from before the
	// backfill. Its quotation reference still resolves through the legacy join.
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_allocations WHERE purchase_order_item_id = $1`,
		database.configSchema()), corrected.Items[1].ID); err != nil {
		t.Fatalf("drop the legacy matched allocation: %v", err)
	}
	// Line 3: a null-reference line never had a cost identity, so it carries no
	// allocation either.
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_items
		SET quotation_item_no = NULL, quotation_sub_no = NULL
		WHERE id = $1`, database.configSchema()), corrected.Items[2].ID); err != nil {
		t.Fatalf("detach the legacy null line: %v", err)
	}
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_allocations WHERE purchase_order_item_id = $1`,
		database.configSchema()), corrected.Items[2].ID); err != nil {
		t.Fatalf("drop the legacy null line allocation: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 {
		t.Fatalf("poCount = %d; want 1", report.Totals.PoCount)
	}
	if report.Totals.AmountMinor != 1230 {
		t.Errorf("amount = %d; want 600 + 600 + 30 = 1230", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 1155 {
		t.Errorf("costed = %d; want allocation 555 + legacy net 600 = 1155", report.Totals.CostedMinor)
	}
	if report.Totals.CapitalMinor != 999 {
		t.Errorf("capital = %d; want one per-PO round of 499.5 + 499.5 = 999", report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 156 {
		t.Errorf("margin = %d; want 1155 - 999 = 156", report.Totals.MarginMinor)
	}
	if report.Totals.UncostedMinor != 75 {
		t.Errorf("uncosted = %d; want 1230 - 1155 = 75", report.Totals.UncostedMinor)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 9390 {
		t.Errorf("coverage = %v; want 9390 bps", report.Totals.CoverageBps)
	}
	if report.Totals.MarginBps == nil || *report.Totals.MarginBps != 1562 {
		t.Errorf("margin bps = %v; want 1562", report.Totals.MarginBps)
	}
}

func TestPurchaseOrderReportIgnoresAStaleLiveAllocation(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_stale")
	quotation := seedReportQuotation(t, database, 2, "PT. Stale Buyer", "Stale allocation report",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Live line", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250,
		}})
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := finalizeReportPO(t, database, ctx, user, quotation,
		"PO-ALLOC-STALE", "2026-03-10", PurchaseOrderBusinessCategoryFm, nil)

	// The PO stays live and keeps its currency, but the allocation loses the
	// revision pointer that proves it. NB3 says a live PO must not bypass the
	// live proof: the report ignores this row and resolves the same line through
	// the legacy quotation join. If the bypass were trusted, costed would be
	// 999.999 and capital 2 instead of the legacy 500 / 200.
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_allocations
		SET source_revision_id = NULL, revenue_minor = 999999, cost_unit_minor = 1
		WHERE purchase_order_id = $1`, database.configSchema()), saved.ID); err != nil {
		t.Fatalf("detach the live allocation: %v", err)
	}
	var live int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM %s.purchase_orders
		WHERE id = $1 AND quotation_id IS NOT NULL`, database.configSchema()),
		saved.ID).Scan(&live); err != nil {
		t.Fatalf("check the live source: %v", err)
	}
	if live != 1 {
		t.Fatalf("fixture purchase order is not live")
	}
	if got := countRows(t, database, "purchase_order_allocations",
		"purchase_order_id = $1", saved.ID); got != 1 {
		t.Fatalf("fixture allocations = %d; want the stale row still present", got)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != 500 {
		t.Fatalf("stale-allocation report = %+v; want one 500 PO", report.Totals)
	}
	if report.Totals.CostedMinor != 500 {
		t.Errorf("costed = %d; want the legacy net 500, not the stale 999999",
			report.Totals.CostedMinor)
	}
	if report.Totals.CapitalMinor != 200 {
		t.Errorf("capital = %d; want the legacy 2 x 100 = 200, not the stale 2",
			report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 300 {
		t.Errorf("margin = %d; want 500 - 200 = 300", report.Totals.MarginMinor)
	}
	if report.Totals.UncostedMinor != 0 {
		t.Errorf("uncosted = %d; want 0", report.Totals.UncostedMinor)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 10000 {
		t.Errorf("coverage = %v; want 10000 bps", report.Totals.CoverageBps)
	}
}
