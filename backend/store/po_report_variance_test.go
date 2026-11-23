package store

import (
	"context"
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

// TestPurchaseOrderReportPriceExtraIncome pins the positive side of realized
// price variance: 5 -> 3 at 11.000 against a 10.000 baseline leaves the two
// unsold units at the quotation price and realizes 3.000 of price extra
// income on the three sold units.
func TestPurchaseOrderReportPriceExtraIncome(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_price_extra")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Price Extra", "Price extra source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-PRICE-EXTRA-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Items[0].UnitPriceMinor = 11_000
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
				varianceDecision("line:1:unitPrice"),
			}
		})

	report := varianceReport(t, database)
	assertVarianceTotals(t, report, 33_000, 20_000, 8_000, 0, 0, 3_000, 0, 3_000, 3_000)
	if report.Totals.CapitalMinor != 18_000 || report.Totals.MarginMinor != 15_000 {
		t.Errorf("actual capital/margin = %d/%d; want 18.000/15.000",
			report.Totals.CapitalMinor, report.Totals.MarginMinor)
	}
	if report.Totals.AmountMinor+report.Pipeline.UnsoldAmountMinor-
		report.Variance.PriceExtraIncomeMinor != 50_000 {
		t.Errorf("actual + unsold - price extra = %d; want the 50.000 quoted population",
			report.Totals.AmountMinor+report.Pipeline.UnsoldAmountMinor-
				report.Variance.PriceExtraIncomeMinor)
	}
}

// addFreeLeafRevision replaces a quotation's latest revision with one free
// placeholder leaf, so the bought item from revision 0 has no current leaf.
func addFreeLeafRevision(t *testing.T, database *Store, quotation QuotationRecord) {
	t.Helper()
	_, err := database.SaveRevision(context.Background(), RevisionDraft{
		QuoteNumber:      quotation.QuoteNumber,
		RevisionNumber:   quotation.RevisionNumber + 1,
		CustomerCode:     quotation.CustomerCode,
		CustomerName:     quotation.CustomerName,
		CustomerAddress:  quotation.CustomerAddress,
		RecipientName:    quotation.RecipientName,
		RecipientAddress: quotation.RecipientAddress,
		Subject:          quotation.Subject,
		Incoterm:         quotation.Incoterm,
		ValidityDays:     quotation.ValidityDays,
		PaymentTermDays:  quotation.PaymentTermDays,
		LeadTimeDays:     quotation.LeadTimeDays,
		DiscountPercent:  0,
		TaxZone:          quotation.TaxZone,
		TaxType:          quotation.TaxType,
		Currency:         quotation.Currency,
		HTMLSnapshot:     "<html>removed leaf revision</html>",
		Items: []RevisionItem{{
			ItemNo:                2,
			Description:           "Free placeholder",
			Qty:                   1,
			UoM:                   "Pc",
			UnitPrice:             0,
			OriginalTotal:         0,
			MarginalizedUnitPrice: 0,
			MarginalizedTotal:     0,
			ListUnitPrice:         0,
		}},
		Totals: RevisionTotals{},
	})
	if err != nil {
		t.Fatalf("add the removed-leaf revision: %v", err)
	}
}

// TestPurchaseOrderReportKeepsRealizedVarianceWhenALeafIsRemoved pins the
// FULL OUTER JOIN half of the variance read: a later revision may remove a
// bought item, but its realized price loss is history and must stay. The
// removed leaf contributes no Unsold and no quantity extra.
func TestPurchaseOrderReportKeepsRealizedVarianceWhenALeafIsRemoved(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_variance_removed")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Removed Leaf", "Removed leaf source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-REMOVED-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Items[0].UnitPriceMinor = 9_000
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
				varianceDecision("line:1:unitPrice"),
			}
		})
	addFreeLeafRevision(t, database, quotation)

	report := varianceReport(t, database)
	if report.Totals.AmountMinor != 27_000 || report.Totals.CapitalMinor != 18_000 {
		t.Errorf("actual = %d/%d; want 27000/18000",
			report.Totals.AmountMinor, report.Totals.CapitalMinor)
	}
	if report.Pipeline.UnsoldAmountMinor != 0 || report.Pipeline.UnsoldMarginMinor != 0 {
		t.Errorf("removed-leaf pipeline = %d/%d; want 0/0",
			report.Pipeline.UnsoldAmountMinor, report.Pipeline.UnsoldMarginMinor)
	}
	if report.Variance.PriceLossMinor != 3_000 || report.Variance.PriceExtraIncomeMinor != 0 {
		t.Errorf("removed-leaf price variance = loss %d / extra %d; want 3000/0",
			report.Variance.PriceLossMinor, report.Variance.PriceExtraIncomeMinor)
	}
	if report.Variance.QuantityExtraIncomeAmountMinor != 0 ||
		report.Variance.QuantityExtraIncomeMarginMinor != 0 {
		t.Errorf("removed-leaf quantity extra = %d/%d; want 0/0",
			report.Variance.QuantityExtraIncomeAmountMinor,
			report.Variance.QuantityExtraIncomeMarginMinor)
	}
	if report.Variance.NetRealizedVarianceAmountMinor != -3_000 ||
		report.Variance.NetRealizedVarianceMarginMinor != -3_000 {
		t.Errorf("removed-leaf net realized = %d/%d; want -3000/-3000",
			report.Variance.NetRealizedVarianceAmountMinor,
			report.Variance.NetRealizedVarianceMarginMinor)
	}
}

// TestPurchaseOrderReportKeepsRealizedVarianceAfterQuotationDeletion pins the
// other FULL OUTER JOIN arm: deletion removes the latest leaves but preserves
// the surviving saved PO's frozen allocation, so its realized price loss must
// still reach the report.
func TestPurchaseOrderReportKeepsRealizedVarianceAfterQuotationDeletion(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_variance_deleted")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Deleted Source", "Deleted source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-DELETED-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Items[0].UnitPriceMinor = 9_000
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
				varianceDecision("line:1:unitPrice"),
			}
		})
	admin := deletionUser(t, database, "admin")
	deleteQuotationRecordWithFreshPreview(t, database, admin, ctx, quotation)

	report := varianceReport(t, database)
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != 27_000 {
		t.Errorf("surviving actual = %d POs / %d amount; want 1/27000",
			report.Totals.PoCount, report.Totals.AmountMinor)
	}
	if report.Pipeline.UnsoldAmountMinor != 0 || report.Pipeline.UnsoldMarginMinor != 0 {
		t.Errorf("deleted-source pipeline = %d/%d; want 0/0",
			report.Pipeline.UnsoldAmountMinor, report.Pipeline.UnsoldMarginMinor)
	}
	if report.Variance.PriceLossMinor != 3_000 || report.Variance.PriceExtraIncomeMinor != 0 {
		t.Errorf("deleted-source price variance = loss %d / extra %d; want 3000/0",
			report.Variance.PriceLossMinor, report.Variance.PriceExtraIncomeMinor)
	}
	if report.Variance.QuantityExtraIncomeAmountMinor != 0 {
		t.Errorf("deleted-source quantity extra = %d; want 0",
			report.Variance.QuantityExtraIncomeAmountMinor)
	}
	if report.Variance.NetRealizedVarianceAmountMinor != -3_000 {
		t.Errorf("deleted-source net realized = %d; want -3000",
			report.Variance.NetRealizedVarianceAmountMinor)
	}
}

// TestPurchaseOrderReportFulfillmentCoverageIsBoughtOverQuoted pins the second
// coverage ratio the report contract separates from cost coverage: the bought
// quantity over the quoted quantity of the latest revision, in basis points,
// rounded half away from zero. It is measured on the latest revision's leaves,
// so a bought leaf a later revision removed is realized history, not current
// fulfillment. An over-buy is a real over-buy and reads above 10000; a
// partition with no quoted quantity reads 0 instead of dividing.
func TestPurchaseOrderReportFulfillmentCoverageIsBoughtOverQuoted(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_fulfillment_bps")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Fulfillment", "Fulfillment source")

	empty := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "USD"))
	if empty.Pipeline.FulfillmentBps != 0 {
		t.Errorf("an empty USD partition reads %d bps; want 0 with no quoted quantity",
			empty.Pipeline.FulfillmentBps)
	}

	finalizeReportPO(t, database, ctx, user, quotation, "PO-FULFILL-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
	three := varianceReport(t, database)
	if three.Pipeline.FulfillmentBps != 6000 {
		t.Errorf("3 of 5 bought = %d bps; want 6000", three.Pipeline.FulfillmentBps)
	}
	// The ratio is a separate figure: Unsold keeps its exact two-unit value.
	if three.Pipeline.UnsoldAmountMinor != 20_000 || three.Pipeline.UnsoldMarginMinor != 8_000 {
		t.Errorf("unsold moved with the ratio: %d/%d; want 20000/8000",
			three.Pipeline.UnsoldAmountMinor, three.Pipeline.UnsoldMarginMinor)
	}

	// The two remaining units plus two extra: seven bought of five quoted is an
	// over-buy and must read 14000, never a silent cap at 10000.
	finalizeReportPO(t, database, ctx, user, quotation, "PO-FULFILL-7", "2026-03-11",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 4 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
	seven := varianceReport(t, database)
	if seven.Pipeline.FulfillmentBps != 14000 {
		t.Errorf("7 of 5 bought = %d bps; want 14000 (an over-buy is never capped)",
			seven.Pipeline.FulfillmentBps)
	}
	if seven.Pipeline.UnsoldAmountMinor != 0 || seven.Pipeline.UnsoldMarginMinor != 0 {
		t.Errorf("over-bought unsold = %d/%d; want 0/0",
			seven.Pipeline.UnsoldAmountMinor, seven.Pipeline.UnsoldMarginMinor)
	}

	// Half away from zero: 1 of 32 is exactly 312.5 bps and must read 313.
	tie := seedReportQuotation(t, database, 3, "PT. Fulfillment Tie", "Tie source", "USD", "FTZ",
		[]reportQuotationLine{{
			Description: "Tie line", UoM: "Pc", Qty: 32, Cost: 6_000, Sell: 10_000,
		}})
	finalizeReportPO(t, database, ctx, user, tie, "PO-FULFILL-TIE", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 1 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
	tied := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "USD"))
	if tied.Pipeline.FulfillmentBps != 313 {
		t.Errorf("1 of 32 bought = %d bps; want 313 (half away from zero)",
			tied.Pipeline.FulfillmentBps)
	}
}

// TestPurchaseOrderReportPipelineIsCurrencyIsolated proves the currency
// partition: an IDR report must never add a USD quotation's cents to Unsold,
// and a fully bought USD quotation must not appear as IDR Unsold merely
// because it has no IDR bought row. The USD report keeps its own pipeline and
// realized price loss.
func TestPurchaseOrderReportPipelineIsCurrencyIsolated(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_variance_currency")
	ctx := poContext(t, database)
	user := poTestUser(t, database)

	// IDR open, no PO: 5 x 10.000 at cost 6.000.
	seedReportQuotation(t, database, 2, "PT. IDR Open", "IDR open source", "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "IDR rack", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000,
		}})
	// USD open, no PO: 5 x 100 at cost 60.
	seedReportQuotation(t, database, 3, "PT. USD Open", "USD open source", "USD", "FTZ",
		[]reportQuotationLine{{
			Description: "USD rack", UoM: "Pc", Qty: 5, Cost: 60, Sell: 100,
		}})
	// USD fully bought at 150 against a 200 baseline: price loss 250.
	usdBought := seedReportQuotation(t, database, 4, "PT. USD Bought", "USD bought source", "USD", "FTZ",
		[]reportQuotationLine{{
			Description: "USD sold rack", UoM: "Pc", Qty: 5, Cost: 100, Sell: 200,
		}})
	finalizeReportPO(t, database, ctx, user, usdBought, "PO-USD-653", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].UnitPriceMinor = 150
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:unitPrice"),
			}
		})

	idr := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if idr.Totals.PoCount != 0 {
		t.Fatalf("IDR poCount = %d; want 0 (the only saved PO is USD)", idr.Totals.PoCount)
	}
	if idr.Pipeline.UnsoldAmountMinor != 50_000 || idr.Pipeline.UnsoldMarginMinor != 20_000 {
		t.Errorf("IDR pipeline = %d/%d; want only the IDR open 50000/20000",
			idr.Pipeline.UnsoldAmountMinor, idr.Pipeline.UnsoldMarginMinor)
	}
	if idr.Variance.PriceLossMinor != 0 || idr.Variance.PriceExtraIncomeMinor != 0 {
		t.Errorf("IDR price variance = loss %d / extra %d; want 0/0 (USD must not leak)",
			idr.Variance.PriceLossMinor, idr.Variance.PriceExtraIncomeMinor)
	}
	if idr.Variance.UnsoldAmountMinor != idr.Pipeline.UnsoldAmountMinor {
		t.Errorf("IDR variance unsold = %d; want the pipeline %d",
			idr.Variance.UnsoldAmountMinor, idr.Pipeline.UnsoldAmountMinor)
	}

	usd := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "USD"))
	if usd.Totals.PoCount != 1 || usd.Totals.AmountMinor != 750 ||
		usd.Totals.CapitalMinor != 500 || usd.Totals.MarginMinor != 250 {
		t.Errorf("USD actual = %+v; want one 750 PO at 500 capital and 250 margin", usd.Totals)
	}
	if usd.Pipeline.UnsoldAmountMinor != 500 || usd.Pipeline.UnsoldMarginMinor != 200 {
		t.Errorf("USD pipeline = %d/%d; want the USD open 500/200",
			usd.Pipeline.UnsoldAmountMinor, usd.Pipeline.UnsoldMarginMinor)
	}
	if usd.Variance.PriceLossMinor != 250 || usd.Variance.NetRealizedVarianceAmountMinor != -250 {
		t.Errorf("USD variance = loss %d / net %d; want 250/-250",
			usd.Variance.PriceLossMinor, usd.Variance.NetRealizedVarianceAmountMinor)
	}
}
