package store

import (
	"testing"

	"vera-app/backend/poreport"
)

// Issue #653: Unsold is pipeline, not loss; price decrease is realized loss;
// quantity and price increases are extra income. These tests drive the exact
// store read with one quotation of five units at 10.000 each, cost 6.000.

func seedVarianceQuotation(
	t *testing.T, database *Store, customerCode int, customerName, subject string,
) QuotationRecord {
	t.Helper()
	return seedReportQuotation(t, database, customerCode, customerName, subject, "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "WIP rack", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000,
		}})
}

func varianceDecision(key string) PurchaseOrderDecision {
	return PurchaseOrderDecision{Key: key, Choice: PurchaseOrderChoicePO}
}

func varianceReport(t *testing.T, database *Store) PurchaseOrderReportSnapshot {
	t.Helper()
	return mustReport(t, database, poContext(t, database),
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
}

func assertVarianceTotals(
	t *testing.T, report PurchaseOrderReportSnapshot,
	actualAmount, unsoldAmount, unsoldMargin int64,
	quantityExtraAmount, quantityExtraMargin, priceExtra, priceLoss, netAmount, netMargin int64,
) {
	t.Helper()
	if report.Totals.AmountMinor != actualAmount {
		t.Errorf("actual amount = %d; want %d", report.Totals.AmountMinor, actualAmount)
	}
	if report.Pipeline.UnsoldAmountMinor != unsoldAmount ||
		report.Pipeline.UnsoldMarginMinor != unsoldMargin {
		t.Errorf("pipeline = %d/%d; want %d/%d",
			report.Pipeline.UnsoldAmountMinor, report.Pipeline.UnsoldMarginMinor,
			unsoldAmount, unsoldMargin)
	}
	if report.Variance.UnsoldAmountMinor != unsoldAmount ||
		report.Variance.UnsoldMarginMinor != unsoldMargin {
		t.Errorf("variance unsold mirror = %d/%d; want %d/%d",
			report.Variance.UnsoldAmountMinor, report.Variance.UnsoldMarginMinor,
			unsoldAmount, unsoldMargin)
	}
	if report.Variance.QuantityExtraIncomeAmountMinor != quantityExtraAmount ||
		report.Variance.QuantityExtraIncomeMarginMinor != quantityExtraMargin {
		t.Errorf("quantity extra income = %d/%d; want %d/%d",
			report.Variance.QuantityExtraIncomeAmountMinor,
			report.Variance.QuantityExtraIncomeMarginMinor,
			quantityExtraAmount, quantityExtraMargin)
	}
	if report.Variance.PriceExtraIncomeMinor != priceExtra {
		t.Errorf("price extra income = %d; want %d",
			report.Variance.PriceExtraIncomeMinor, priceExtra)
	}
	if report.Variance.PriceLossMinor != priceLoss {
		t.Errorf("price loss = %d; want %d", report.Variance.PriceLossMinor, priceLoss)
	}
	if report.Variance.ClosedLossAmountMinor != 0 || report.Variance.ClosedLossMarginMinor != 0 {
		t.Errorf("closed loss = %d/%d; want the #656 placeholder 0/0",
			report.Variance.ClosedLossAmountMinor, report.Variance.ClosedLossMarginMinor)
	}
	if report.Variance.NetRealizedVarianceAmountMinor != netAmount ||
		report.Variance.NetRealizedVarianceMarginMinor != netMargin {
		t.Errorf("net realized variance = %d/%d; want %d/%d",
			report.Variance.NetRealizedVarianceAmountMinor,
			report.Variance.NetRealizedVarianceMarginMinor,
			netAmount, netMargin)
	}
}

// TestPurchaseOrderReportUnsoldIsNotALoss pins the 5 -> 3 same-price case:
// Actual is the three sold units, Unsold is the two still orderable units, and
// no quantity loss exists.
func TestPurchaseOrderReportUnsoldIsNotALoss(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_unsold")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Unsold", "Unsold source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-UNSOLD-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})

	report := varianceReport(t, database)
	if report.Totals.PoCount != 1 {
		t.Fatalf("poCount = %d; want 1", report.Totals.PoCount)
	}
	assertVarianceTotals(t, report, 30_000, 20_000, 8_000, 0, 0, 0, 0, 0, 0)
	if report.Totals.CapitalMinor != 18_000 || report.Totals.MarginMinor != 12_000 {
		t.Errorf("actual capital/margin = %d/%d; want 18.000/12.000",
			report.Totals.CapitalMinor, report.Totals.MarginMinor)
	}
	// Actual (at the PO's price) plus Unsold (at the quotation baseline) is
	// the full quoted population when the price did not move.
	if report.Totals.AmountMinor+report.Pipeline.UnsoldAmountMinor != 50_000 {
		t.Errorf("actual + unsold = %d; want the 5 x 10.000 quoted population",
			report.Totals.AmountMinor+report.Pipeline.UnsoldAmountMinor)
	}
}

// TestPurchaseOrderReportPriceLossPersistsAfterALaterBuy pins the 5 -> 3 at
// 9.000 case and then the remaining two bought at 10.000: Unsold clears, but
// the realized price loss stays on the sold units.
func TestPurchaseOrderReportPriceLossPersistsAfterALaterBuy(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_price_loss")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Price Loss", "Price loss source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-PRICE-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Items[0].UnitPriceMinor = 9_000
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
				varianceDecision("line:1:unitPrice"),
			}
		})

	report := varianceReport(t, database)
	assertVarianceTotals(t, report, 27_000, 20_000, 8_000, 0, 0, 0, 3_000, -3_000, -3_000)
	if report.Totals.CapitalMinor != 18_000 || report.Totals.MarginMinor != 9_000 {
		t.Errorf("actual capital/margin = %d/%d; want 18.000/9.000",
			report.Totals.CapitalMinor, report.Totals.MarginMinor)
	}
	// The price loss explains the gap between the PO's actual revenue, the
	// Unsold baseline and the full quoted population.
	if report.Totals.AmountMinor+report.Pipeline.UnsoldAmountMinor+
		report.Variance.PriceLossMinor != 50_000 {
		t.Errorf("actual + unsold + price loss = %d; want the 50.000 quoted population",
			report.Totals.AmountMinor+report.Pipeline.UnsoldAmountMinor+
				report.Variance.PriceLossMinor)
	}

	// A later PO buys the remaining two at the quotation price. The draft is
	// seeded with exactly the remaining two, but the source still says five,
	// so the final save records the explicit quantity choice.
	finalizeReportPO(t, database, ctx, user, quotation, "PO-PRICE-2", "2026-03-11",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})

	report = varianceReport(t, database)
	if report.Totals.PoCount != 2 {
		t.Fatalf("poCount after the second buy = %d; want 2", report.Totals.PoCount)
	}
	assertVarianceTotals(t, report, 47_000, 0, 0, 0, 0, 0, 3_000, -3_000, -3_000)
	if report.Totals.CapitalMinor != 30_000 || report.Totals.MarginMinor != 17_000 {
		t.Errorf("actual capital/margin after the second buy = %d/%d; want 30.000/17.000",
			report.Totals.CapitalMinor, report.Totals.MarginMinor)
	}
}

// TestPurchaseOrderReportQuantityExtraIncome pins the 5 -> 7 case: the two
// over-quoted units are realized extra income at the quotation price, with the
// frozen cost leaving an 8.000 margin contribution.
func TestPurchaseOrderReportQuantityExtraIncome(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_quantity_extra")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Quantity Extra", "Quantity extra source")

	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-EXTRA-7", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 7 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
	// Reopening the over-bought saved record must compare its seven units with
	// the five-unit quotation, so the badge still says +2 extra rather than
	// treating the record as if it had not consumed the room itself.
	if len(saved.Items) != 1 {
		t.Fatalf("saved lines = %d; want 1", len(saved.Items))
	}
	if saved.Items[0].RemainingQuantityScaled == nil ||
		*saved.Items[0].RemainingQuantityScaled != 5*QuantityScale {
		t.Fatalf("saved over-bought line remaining = %v; want 50000",
			saved.Items[0].RemainingQuantityScaled)
	}

	report := varianceReport(t, database)
	assertVarianceTotals(t, report, 70_000, 0, 0, 20_000, 8_000, 0, 0, 20_000, 8_000)
	if report.Totals.CapitalMinor != 42_000 || report.Totals.MarginMinor != 28_000 {
		t.Errorf("actual capital/margin = %d/%d; want 42.000/28.000",
			report.Totals.CapitalMinor, report.Totals.MarginMinor)
	}
	// The actual amount is the whole seven units, while the quoted population
	// is five: the two extra units are the 20.000 quantity extra income.
	if report.Totals.AmountMinor-report.Variance.QuantityExtraIncomeAmountMinor != 50_000 {
		t.Errorf("actual - quantity extra = %d; want the 50.000 quoted population",
			report.Totals.AmountMinor-report.Variance.QuantityExtraIncomeAmountMinor)
	}
}
