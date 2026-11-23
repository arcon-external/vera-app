package store

import (
	"fmt"
	"testing"
	"time"
)

// Issue #656: an explicit quotation close moves the unbought remainder out of
// Unsold and into Closed Loss. The worked example is the one the parent plan
// pins: five units quoted at 10.000 with a 6.000 cost, three bought, then the
// quotation closed. Actual stays the three sold units; the remaining two are a
// realized loss of 20.000 revenue / 8.000 margin, and the net realized variance
// is exactly that loss.

func assertClosedLossReport(
	t *testing.T, report PurchaseOrderReportSnapshot,
	unsoldAmount, unsoldMargin, closedLossAmount, closedLossMargin, netAmount, netMargin int64,
) {
	t.Helper()
	if report.Pipeline.UnsoldAmountMinor != unsoldAmount ||
		report.Pipeline.UnsoldMarginMinor != unsoldMargin {
		t.Errorf("pipeline unsold = %d/%d; want %d/%d",
			report.Pipeline.UnsoldAmountMinor, report.Pipeline.UnsoldMarginMinor,
			unsoldAmount, unsoldMargin)
	}
	// The variance object mirrors Unsold for the screen that shows both. It must
	// mirror the pipeline figure, never the pre-close one.
	if report.Variance.UnsoldAmountMinor != unsoldAmount ||
		report.Variance.UnsoldMarginMinor != unsoldMargin {
		t.Errorf("variance unsold mirror = %d/%d; want %d/%d",
			report.Variance.UnsoldAmountMinor, report.Variance.UnsoldMarginMinor,
			unsoldAmount, unsoldMargin)
	}
	if report.Variance.ClosedLossAmountMinor != closedLossAmount ||
		report.Variance.ClosedLossMarginMinor != closedLossMargin {
		t.Errorf("closed loss = %d/%d; want %d/%d",
			report.Variance.ClosedLossAmountMinor, report.Variance.ClosedLossMarginMinor,
			closedLossAmount, closedLossMargin)
	}
	if report.Variance.NetRealizedVarianceAmountMinor != netAmount ||
		report.Variance.NetRealizedVarianceMarginMinor != netMargin {
		t.Errorf("net realized variance = %d/%d; want %d/%d",
			report.Variance.NetRealizedVarianceAmountMinor,
			report.Variance.NetRealizedVarianceMarginMinor, netAmount, netMargin)
	}
}

func TestPurchaseOrderReportClosedLoss(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_closed_loss")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Closed Loss", "Closed loss source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-CLOSED-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})

	if _, err := database.CloseQuotation(ctx, quotation.ID,
		"customer cancelled the remaining two units", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}

	report := varianceReport(t, database)
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != 30_000 {
		t.Fatalf("actual = %d POs / %d; want the three sold units at 30.000",
			report.Totals.PoCount, report.Totals.AmountMinor)
	}
	if report.Totals.CapitalMinor != 18_000 || report.Totals.MarginMinor != 12_000 {
		t.Errorf("actual capital/margin = %d/%d; want 18.000/12.000",
			report.Totals.CapitalMinor, report.Totals.MarginMinor)
	}
	assertClosedLossReport(t, report, 0, 0, 20_000, 8_000, -20_000, -8_000)
	if report.Variance.QuantityExtraIncomeAmountMinor != 0 ||
		report.Variance.PriceExtraIncomeMinor != 0 ||
		report.Variance.PriceLossMinor != 0 {
		t.Errorf("closed loss invented another variance component: %+v", report.Variance)
	}
}

// TestPurchaseOrderReportClosedLossIsNotUnsold keeps the no-double-count rule:
// the closed remainder belongs to exactly one bucket, and the full quoted
// population is still accounted for across Actual and Closed Loss.
func TestPurchaseOrderReportClosedLossIsNotUnsold(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_closed_loss_once")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Closed Once", "Closed once source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-CLOSED-ONCE-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
	if _, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}

	report := varianceReport(t, database)
	population := report.Totals.AmountMinor + report.Pipeline.UnsoldAmountMinor +
		report.Variance.ClosedLossAmountMinor
	if population != 50_000 {
		t.Errorf("actual + unsold + closed loss = %d; want the 5 x 10.000 quoted population once",
			population)
	}
	if report.Pipeline.UnsoldAmountMinor != 0 && report.Variance.ClosedLossAmountMinor != 0 {
		t.Errorf("the remainder is both unsold (%d) and closed loss (%d)",
			report.Pipeline.UnsoldAmountMinor, report.Variance.ClosedLossAmountMinor)
	}
	if report.Pipeline.UnsoldAmountMinor == 0 && report.Variance.ClosedLossAmountMinor == 0 {
		t.Error("the closed remainder is in neither bucket")
	}
}

// TestPurchaseOrderReportReopenRestoresUnsold pins the reversal: reopening the
// quotation moves the same remainder back to Unsold and the net back to zero,
// without touching the allocation the saved PO already owns.
func TestPurchaseOrderReportReopenRestoresUnsold(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_closed_reopen")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Reopened", "Reopened source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-REOPEN-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})

	if _, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	closed := varianceReport(t, database)
	assertClosedLossReport(t, closed, 0, 0, 20_000, 8_000, -20_000, -8_000)

	if _, err := database.ReopenQuotation(ctx, quotation.ID, admin.ID); err != nil {
		t.Fatalf("ReopenQuotation: %v", err)
	}
	reopened := varianceReport(t, database)
	assertClosedLossReport(t, reopened, 20_000, 8_000, 0, 0, 0, 0)
	if reopened.Totals.AmountMinor != closed.Totals.AmountMinor {
		t.Errorf("actual amount moved across reopen: %d -> %d",
			closed.Totals.AmountMinor, reopened.Totals.AmountMinor)
	}

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var allocations, events int
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT (SELECT COUNT(*) FROM %s.purchase_order_allocations WHERE quotation_id = $1),
		       (SELECT COUNT(*) FROM %s.quotation_closure_events WHERE quotation_id = $1)`,
		database.configSchema(), database.configSchema()),
		quotation.ID).Scan(&allocations, &events); err != nil {
		t.Fatalf("count allocations and events: %v", err)
	}
	if allocations != 1 {
		t.Errorf("allocations after reopen = %d; want the one saved PO line untouched", allocations)
	}
	if events != 2 {
		t.Errorf("closure events after reopen = %d; want close + reopen", events)
	}
}

// TestPurchaseOrderReportExpiredUnclosedStaysUnsold is the proof that expiry is
// never a loss by itself: the offer lapses, the badge says so, and the remaining
// quantity still reports as Unsold until an admin closes the quotation.
func TestPurchaseOrderReportExpiredUnclosedStaysUnsold(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_expired_unclosed")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Expired", "Expired source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-EXPIRED-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revisions SET created_at = $2 WHERE quotation_id = $1`,
		database.configSchema()), quotation.ID, time.Now().AddDate(0, 0, -45)); err != nil {
		t.Fatalf("age the issued revision: %v", err)
	}

	availability := availabilityFor(t, database, ctx, quotation.ID)
	if !availability.ExpiredUnclosed || availability.Closed {
		t.Fatalf("availability = expired %v / closed %v; want an expired open quotation",
			availability.ExpiredUnclosed, availability.Closed)
	}

	report := varianceReport(t, database)
	assertClosedLossReport(t, report, 20_000, 8_000, 0, 0, 0, 0)
}

// TestPurchaseOrderReportClosedLossAndQuantityExtraDoNotDoubleCount seeds both
// directions at once: one leaf is over-bought (quantity extra income) and the
// other is under-bought on a closed quotation (closed loss). Each unit belongs
// to exactly one component, so the net is extra income minus closed loss.
func TestPurchaseOrderReportClosedLossAndQuantityExtraDoNotDoubleCount(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_closed_mixed")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Mixed", "Mixed variance source",
		"IDR", "FTZ", []reportQuotationLine{
			{Description: "Over-bought line", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000},
			{Description: "Under-bought line", UoM: "Pc", Qty: 4, Cost: 6_000, Sell: 10_000},
		})

	finalizeReportPO(t, database, ctx, user, quotation, "PO-MIXED-8", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 7 * QuantityScale
			input.Items[1].QuantityScaled = 1 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
				varianceDecision("line:2:quantity"),
			}
		})
	if _, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}

	report := varianceReport(t, database)
	if report.Totals.AmountMinor != 80_000 ||
		report.Totals.CapitalMinor != 48_000 ||
		report.Totals.MarginMinor != 32_000 {
		t.Errorf("actual = %d/%d/%d; want 80.000/48.000/32.000",
			report.Totals.AmountMinor, report.Totals.CapitalMinor, report.Totals.MarginMinor)
	}
	if report.Variance.QuantityExtraIncomeAmountMinor != 20_000 ||
		report.Variance.QuantityExtraIncomeMarginMinor != 8_000 {
		t.Errorf("quantity extra income = %d/%d; want 20.000/8.000",
			report.Variance.QuantityExtraIncomeAmountMinor,
			report.Variance.QuantityExtraIncomeMarginMinor)
	}
	assertClosedLossReport(t, report, 0, 0, 30_000, 12_000, -10_000, -4_000)
}

// TestPurchaseOrderReportClosedLossIgnoresARemovedBoughtLeaf pins the FULL
// OUTER JOIN's honesty: a bought allocation whose leaf the latest revision
// removed has no quoted quantity to close. Closing the quotation must not turn
// those bought units into a loss; only the current revision's unbought one unit
// is closed.
func TestPurchaseOrderReportClosedLossIgnoresARemovedBoughtLeaf(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_closed_removed")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Removed", "Removed leaf source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-REMOVED-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
	// Revision 1 drops the bought item and quotes one different unit instead.
	if _, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode: 2, CustomerName: "PT. Removed", CustomerAddress: "Jl. Report Billing 1",
		RecipientName: "Budi Santoso", RecipientAddress: "Jl. Report Recipient 9",
		Subject: "Removed leaf", Incoterm: "CIF", ValidityDays: 30, PaymentTermDays: 45,
		LeadTimeDays: 10, TaxZone: "FTZ", TaxType: "Goods", Currency: "IDR",
		RevisionNumber: 1, QuoteNumber: quotation.QuoteNumber,
		HTMLSnapshot: "<html>removed leaf</html>",
		Items: []RevisionItem{{
			ItemNo: 2, Description: "Replacement line", Qty: 1, UoM: "Pc",
			UnitPrice: 6_000, OriginalTotal: 6_000, MarginalizedUnitPrice: 10_000,
			MarginalizedTotal: 10_000, ListUnitPrice: 10_000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal: 6_000, OriginalGrand: 6_000,
			MarginalizedSubtotal: 10_000, MarginalizedGrand: 10_000,
			MarginalizedGrandAfterTax: 10_000, CustomerGrandTotal: 10_000,
		},
	}); err != nil {
		t.Fatalf("issue the revision that removes the bought leaf: %v", err)
	}
	if _, err := database.CloseQuotation(ctx, quotation.ID, "customer cancelled", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}

	report := varianceReport(t, database)
	// 30.000 of bought revenue survives as price variance zero; the removed
	// leaf contributes neither unsold nor closed loss. Only revision 1's one
	// unbought unit is closed: 10.000 revenue / 4.000 margin.
	assertClosedLossReport(t, report, 0, 0, 10_000, 4_000, -10_000, -4_000)
	if report.Totals.AmountMinor != 30_000 {
		t.Errorf("actual amount = %d; want the 30.000 bought before the revision",
			report.Totals.AmountMinor)
	}
}
