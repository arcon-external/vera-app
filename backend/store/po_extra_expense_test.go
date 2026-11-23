package store

import (
	"reflect"
	"testing"

	"vera-app/backend/poreport"
)

// Issue #655: an extra line is a pass-through expense. Its amount is real
// revenue on the PO, so it belongs to Actual, but it has no supplier cost and
// no margin: it is reported in its own Extra Expense bucket, never as uncosted
// revenue, and it is subtracted from the cost-coverage denominator. It never
// enters the quotation population, so Pipeline, Unsold and price variance are
// identical with or without it.

// extraLine is the submitted extra the tests append to a real quotation draft:
// two units of freight at 500.000, charged through at zero margin.
func extraLine() PurchaseOrderItem {
	return PurchaseOrderItem{
		Description:    "Freight surcharge",
		QuantityScaled: 2 * QuantityScale,
		UoM:            "Lot",
		UnitPriceMinor: 500_000,
		LineKind:       PurchaseOrderLineKindExtra,
	}
}

// TestPurchaseOrderReportBucketsAnExtraLineAsPassThroughExpense is the dedicated
// bucket proof: the quoted line keeps its cost, capital and margin exactly, the
// extra adds amount and nothing else, uncosted stays zero, and coverage is
// computed on the non-extra amount.
func TestPurchaseOrderReportBucketsAnExtraLineAsPassThroughExpense(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_report_bucket")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Extra Buyer", "Extra bucket source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000,
		}})

	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-655-EXTRA", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})
	if len(saved.Items) != 2 {
		t.Fatalf("saved lines = %d; want the quoted line and the extra", len(saved.Items))
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	totals := report.Totals
	if totals.PoCount != 1 {
		t.Errorf("poCount = %d; want 1", totals.PoCount)
	}
	if totals.AmountMinor != 1_050_000 {
		t.Errorf("amount = %d; want 5 x 10.000 + 2 x 500.000 = 1.050.000", totals.AmountMinor)
	}
	if totals.ExtraExpenseMinor != 1_000_000 {
		t.Errorf("extra expense = %d; want 2 x 500.000 = 1.000.000", totals.ExtraExpenseMinor)
	}
	// The quoted line's cost, capital and margin must be exactly what they are
	// without the extra.
	if totals.CostedMinor != 50_000 {
		t.Errorf("costed = %d; want the quoted line's 50.000", totals.CostedMinor)
	}
	if totals.CapitalMinor != 30_000 {
		t.Errorf("capital = %d; want 5 x 6.000 = 30.000", totals.CapitalMinor)
	}
	if totals.MarginMinor != 20_000 {
		t.Errorf("margin = %d; want 50.000 - 30.000 = 20.000 (the extra adds none)",
			totals.MarginMinor)
	}
	if totals.UncostedMinor != 0 {
		t.Errorf("uncosted = %d; want 0: the extra is expense, not unknown cost", totals.UncostedMinor)
	}
	// Coverage is costed / (amount - extra) = 50.000 / 50.000, not 50.000 / 1.050.000.
	if totals.CoverageBps == nil || *totals.CoverageBps != 10000 {
		t.Errorf("coverage = %v; want 10000 bps on the non-extra amount", totals.CoverageBps)
	}
	if totals.MarginBps == nil || *totals.MarginBps != 6667 {
		t.Errorf("margin bps = %v; want 20.000 / 30.000 = 6667", totals.MarginBps)
	}

	// Every grouped row foots to the total, so the bucket is not only a totals
	// figure that the period and category tables cannot explain.
	period := reportPeriod(t, report, "2026-03-01")
	if period.ExtraExpenseMinor != 1_000_000 || period.AmountMinor != 1_050_000 ||
		period.CostedMinor != 50_000 || period.UncostedMinor != 0 {
		t.Errorf("period March = %+v; want the same extra split as totals", period.Metrics)
	}
	category := reportCategory(t, report, poreport.CategoryFm)
	if category.ExtraExpenseMinor != 1_000_000 || category.AmountMinor != 1_050_000 ||
		category.CostedMinor != 50_000 || category.UncostedMinor != 0 {
		t.Errorf("category FM = %+v; want the same extra split as totals", category.Metrics)
	}

	// The quotation was fully bought: the extra does not invent or consume a
	// leaf, and no variance is attributed to it.
	if report.Pipeline.UnsoldAmountMinor != 0 || report.Pipeline.UnsoldMarginMinor != 0 {
		t.Errorf("pipeline = %+v; want no unsold quantity", report.Pipeline)
	}
	if report.Variance.NetRealizedVarianceAmountMinor != 0 ||
		report.Variance.NetRealizedVarianceMarginMinor != 0 {
		t.Errorf("variance = %+v; want no realized variance", report.Variance)
	}
}

// TestPurchaseOrderReportExtraLineLeavesPipelineUnchanged keeps the open Unsold
// remainder of a partially bought quotation: buying 3 of 5 leaves 2 Pc and
// 20.000 / 8.000 unsold whether or not a pass-through extra rides on the same
// purchase order.
func TestPurchaseOrderReportExtraLineLeavesPipelineUnchanged(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_report_pipeline")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Extra Pipeline", "Extra pipeline source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000,
		}})

	finalizeReportPO(t, database, ctx, user, quotation, "PO-655-PIPELINE", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Items = append(input.Items, extraLine())
			input.Decisions = []PurchaseOrderDecision{
				{Key: "line:1:quantity", Choice: PurchaseOrderChoicePO},
			}
			input.AcknowledgedExtra = []int{2}
		})

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Pipeline.UnsoldAmountMinor != 20_000 || report.Pipeline.UnsoldMarginMinor != 8_000 {
		t.Errorf("pipeline = %+v; want the two unbought pieces at 20.000 / 8.000",
			report.Pipeline)
	}
	if report.Variance.QuantityExtraIncomeAmountMinor != 0 ||
		report.Variance.QuantityExtraIncomeMarginMinor != 0 ||
		report.Variance.PriceExtraIncomeMinor != 0 ||
		report.Variance.PriceLossMinor != 0 ||
		report.Variance.ClosedLossAmountMinor != 0 ||
		report.Variance.ClosedLossMarginMinor != 0 {
		t.Errorf("variance = %+v; want every realized component at zero", report.Variance)
	}
	if report.Totals.AmountMinor != 30_000+1_000_000 {
		t.Errorf("amount = %d; want the three bought units plus the extra", report.Totals.AmountMinor)
	}
	if report.Totals.ExtraExpenseMinor != 1_000_000 {
		t.Errorf("extra expense = %d; want 1.000.000", report.Totals.ExtraExpenseMinor)
	}
}

// TestPurchaseOrderReportExtraOnlyPeriodHasNoCoverage is the zero-denominator
// case: a record whose only line is an extra has amount == extra, no cost, no
// capital, zero margin, and null coverage. It must never report 0% or 100%
// profit.
func TestPurchaseOrderReportExtraOnlyPeriodHasNoCoverage(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_report_extra_only")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Extra Only", "Extra only source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 250,
		}})

	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-655-EXTRA-ONLY", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})
	// Removing the quoted line in a correction leaves a saved record that is
	// only the pass-through expense.
	correction := correctionFromSavedOrder(saved, user, "the customer dropped the quoted line")
	correction.Items = correction.Items[1:]
	if _, err := database.CorrectPurchaseOrder(ctx, correction); err != nil {
		t.Fatalf("leave only the extra line: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	totals := report.Totals
	if totals.PoCount != 1 || totals.AmountMinor != 1_000_000 {
		t.Fatalf("totals = %+v; want one 1.000.000 extra-only PO", totals)
	}
	if totals.ExtraExpenseMinor != 1_000_000 {
		t.Errorf("extra expense = %d; want amount == extra == 1.000.000", totals.ExtraExpenseMinor)
	}
	if totals.CostedMinor != 0 || totals.CapitalMinor != 0 || totals.MarginMinor != 0 {
		t.Errorf("costed/capital/margin = %d/%d/%d; want 0/0/0",
			totals.CostedMinor, totals.CapitalMinor, totals.MarginMinor)
	}
	if totals.UncostedMinor != 0 {
		t.Errorf("uncosted = %d; want 0: an extra is not unknown cost", totals.UncostedMinor)
	}
	if totals.CoverageBps != nil {
		t.Errorf("coverage = %d; want null on a zero non-extra denominator", *totals.CoverageBps)
	}
	if totals.MarginBps != nil {
		t.Errorf("margin bps = %d; want null with zero capital", *totals.MarginBps)
	}
	period := reportPeriod(t, report, "2026-03-01")
	if period.CoverageBps != nil || period.ExtraExpenseMinor != 1_000_000 || period.UncostedMinor != 0 {
		t.Errorf("period March = %+v; want the same extra-only shape", period.Metrics)
	}
}

// TestPurchaseOrderReportCarriesExtraThroughHistoryAndDaily proves the field is
// threaded through the two projection inputs as well as the wire tables: a
// complete history month and the current month's daily sample each keep the
// pass-through amount in Extra Expense rather than in uncosted.
func TestPurchaseOrderReportCarriesExtraThroughHistoryAndDaily(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_report_samples")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	lines := []reportQuotationLine{{
		Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
	}}
	march := seedReportQuotation(t, database, 2, "PT. Extra History", "Extra history source",
		"IDR", "FTZ", lines)
	finalizeReportPO(t, database, ctx, user, march, "PO-655-HISTORY", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})
	november := seedReportQuotation(t, database, 3, "PT. Extra Daily", "Extra daily source",
		"IDR", "FTZ", lines)
	finalizeReportPO(t, database, ctx, user, november, "PO-655-DAILY", "2026-11-05",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})

	yearly := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	historyFound := false
	for _, month := range yearly.History {
		if month.MonthStart != "2026-03-01" {
			continue
		}
		historyFound = true
		if month.ExtraExpenseMinor != 1_000_000 || month.AmountMinor != 1_020_000 ||
			month.CostedMinor != 20_000 || month.UncostedMinor != 0 {
			t.Errorf("history March = %+v; want amount 1.020.000 with 1.000.000 extra",
				month.Metrics)
		}
	}
	if !historyFound {
		t.Fatalf("history has no March row: %+v", yearly.History)
	}

	daily := mustReport(t, database, ctx,
		reportQuery("2026-11-01", "2026-12-01", poreport.BucketDay, poreport.CategoryAll, "IDR"))
	dailyFound := false
	for _, day := range daily.Daily {
		if day.Date != "2026-11-05" {
			continue
		}
		dailyFound = true
		if day.ExtraExpenseMinor != 1_000_000 || day.AmountMinor != 1_020_000 ||
			day.CostedMinor != 20_000 || day.UncostedMinor != 0 {
			t.Errorf("daily 5 November = %+v; want amount 1.020.000 with 1.000.000 extra",
				day.Metrics)
		}
	}
	if !dailyFound {
		t.Fatalf("daily sample has no 5 November row: %+v", daily.Daily)
	}
}

// TestPurchaseOrderReportDecomposesAnExtraInsideAnAggregate keeps the #654
// behaviour: an aggregate header is presentation only, so grouping a quoted
// child with an extra child changes no report figure, and the group total
// includes the pass-through amount.
func TestPurchaseOrderReportDecomposesAnExtraInsideAnAggregate(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_report_group")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Extra Group", "Extra group source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted child", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
		}})

	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-655-GROUP", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})
	before := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if before.Totals.ExtraExpenseMinor != 1_000_000 || before.Totals.CostedMinor != 20_000 {
		t.Fatalf("ungrouped totals = %+v; want extra 1.000.000 and costed 20.000", before.Totals)
	}

	grouped := correctionInput(saved, user, "one customer line over the child and freight", saved.Version)
	grouped.Items = indexedItems(saved.Items, 1, 1)
	grouped.AggregateGroups = []PurchaseOrderAggregateGroupInput{
		{Title: "Combined line and freight", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"},
	}
	corrected, err := database.CorrectPurchaseOrder(ctx, grouped)
	if err != nil {
		t.Fatalf("group the extra with its quoted sibling: %v", err)
	}
	if len(corrected.AggregateGroups) != 1 ||
		corrected.AggregateGroups[0].TotalMinor != 1_020_000 {
		t.Fatalf("live group = %+v; want one header totalling 20.000 + 1.000.000",
			corrected.AggregateGroups)
	}

	after := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if !reflect.DeepEqual(before, after) {
		t.Errorf("grouping the extra changed the report:\nbefore = %+v\nafter  = %+v", before, after)
	}
}
