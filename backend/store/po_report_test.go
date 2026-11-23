package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// Issue #629: the purchase-order report aggregate against disposable Postgres.
// These tests seed real issued revisions and finalize real saved records, then
// read the report through the store seam. The point is the arithmetic and the
// invariants staff rely on, not SQL text: a report that foots, keeps cost and
// sell apart, rounds capital once per PO, never turns an unknown cost into
// profit, and isolates organization, currency, category and date.

func reportNow() time.Time {
	// Fixed so partial/in-progress flags and the projection basis are stable.
	// 05:00 UTC is noon in the company's WIB calendar, 2026-11-19.
	return time.Date(2026, 11, 19, 5, 0, 0, 0, time.UTC)
}

type reportQuotationSub struct {
	Description     string
	UoM             string
	Qty             int64
	Cost            int64
	Sell            int64
	DiscountEnabled bool
	DiscountPercent int16
}

type reportQuotationLine struct {
	Description     string
	UoM             string
	Qty             int64
	Cost            int64
	Sell            int64
	DiscountEnabled bool
	DiscountPercent int16
	SubItems        []reportQuotationSub
}

// seedReportQuotation writes one issued revision with the exact cost and sell
// fields the report must tell apart: unit_price is the pre-margin cost, while
// list/marginalized_unit_price is what the customer paid. A package parent's
// own cost is deliberately not used by the PO line mapping, so sub-items carry
// their own unit_cost.
func seedReportQuotation(
	t *testing.T,
	database *Store,
	customerCode int,
	customerName, subject, currency, taxZone string,
	lines []reportQuotationLine,
) QuotationRecord {
	t.Helper()
	items := make([]RevisionItem, 0, len(lines))
	var original, marginalized int64
	for index, line := range lines {
		item := RevisionItem{
			ItemNo:                index + 1,
			Description:           line.Description,
			UoM:                   line.UoM,
			Qty:                   line.Qty,
			UnitPrice:             line.Cost,
			MarginalizedUnitPrice: line.Sell,
			ListUnitPrice:         line.Sell,
			DiscountEnabled:       line.DiscountEnabled,
			DiscountPercent:       line.DiscountPercent,
			ShowSubPrices:         len(line.SubItems) > 0,
		}
		if len(line.SubItems) == 0 {
			item.OriginalTotal = line.Qty * line.Cost
			item.MarginalizedTotal = line.Qty * line.Sell
			original += item.OriginalTotal
			marginalized += item.MarginalizedTotal
		} else {
			subs := make([]RevisionSubItem, 0, len(line.SubItems))
			for subIndex, sub := range line.SubItems {
				stored := RevisionSubItem{
					SubNo:           subIndex + 1,
					Description:     sub.Description,
					UoM:             sub.UoM,
					Qty:             sub.Qty,
					UnitCost:        sub.Cost,
					UnitPrice:       sub.Sell,
					ListUnitPrice:   sub.Sell,
					Total:           sub.Qty * sub.Sell,
					DiscountEnabled: sub.DiscountEnabled,
					DiscountPercent: sub.DiscountPercent,
				}
				item.OriginalTotal += sub.Qty * sub.Cost
				item.MarginalizedTotal += stored.Total
				subs = append(subs, stored)
			}
			item.UnitPrice = item.OriginalTotal
			item.MarginalizedUnitPrice = item.MarginalizedTotal
			original += item.OriginalTotal
			marginalized += item.MarginalizedTotal
			item.SubItems = subs
		}
		items = append(items, item)
	}

	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     int16(customerCode),
		CustomerName:     customerName,
		CustomerAddress:  "Jl. Report Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Report Recipient 9",
		Subject:          subject,
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		DiscountPercent:  0,
		TaxZone:          taxZone,
		TaxType:          "Goods",
		Currency:         currency,
		HTMLSnapshot:     "<html>report fixture</html>",
		Items:            items,
		Totals: RevisionTotals{
			OriginalSubtotal:          original,
			OriginalGrand:             original,
			MarginalizedSubtotal:      marginalized,
			MarginalizedGrand:         marginalized,
			MarginalizedGrandAfterTax: marginalized,
			CustomerGrandTotal:        marginalized,
		},
	})
	if err != nil {
		t.Fatalf("seed report quotation %q: %v", subject, err)
	}
	return record
}

// finalizeReportPO creates a draft from a real revision, finalizes it at an
// explicit received date and category, and lets the test change the entered
// lines before the final save (quantity and row discount are the two levers).
func finalizeReportPO(
	t *testing.T,
	database *Store,
	ctx context.Context,
	user AuthUser,
	quotation QuotationRecord,
	number, received, category string,
	mutate func(*PurchaseOrderFinalize),
) PurchaseOrder {
	t.Helper()
	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:      quotation.ID,
		RevisionNumber:   quotation.RevisionNumber,
		BusinessCategory: category,
		CreatedBy:        user.ID,
	})
	if err != nil {
		t.Fatalf("create report PO draft: %v", err)
	}
	input := finalizeInput(draft, user, number)
	receivedDate, err := time.Parse(time.DateOnly, received)
	if err != nil {
		t.Fatalf("received date %q: %v", received, err)
	}
	input.ReceivedDate = &receivedDate
	input.BusinessCategory = category
	if mutate != nil {
		mutate(&input)
	}
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize report PO %s: %v", number, err)
	}
	return saved
}

func reportQuery(from, to, bucket, category, currency string) poreport.Query {
	return poreport.Query{
		From: from, To: to, Bucket: bucket, Category: category, Currency: currency,
	}
}

func reportPeriod(t *testing.T, report PurchaseOrderReportSnapshot, start string) poreport.Period {
	t.Helper()
	for _, period := range report.Periods {
		if period.PeriodStart == start {
			return period
		}
	}
	t.Fatalf("report has no period %s: %+v", start, report.Periods)
	return poreport.Period{}
}

func reportCategory(t *testing.T, report PurchaseOrderReportSnapshot, code string) poreport.Category {
	t.Helper()
	for _, category := range report.Categories {
		if category.BusinessCategory == code {
			return category
		}
	}
	t.Fatalf("report has no category %s: %+v", code, report.Categories)
	return poreport.Category{}
}

func mustReport(t *testing.T, database *Store, ctx context.Context, query poreport.Query) PurchaseOrderReportSnapshot {
	t.Helper()
	report, err := database.ReportPurchaseOrders(ctx, query, reportNow())
	if err != nil {
		t.Fatalf("ReportPurchaseOrders(%+v): %v", query, err)
	}
	return report
}

// TestPurchaseOrderReportUsesPinnedCostAtThePOQuantity is the core formula
// pin: amount is the customer sell total excluding PPN, costed is the same
// lines' net, and capital is the source revision's pre-margin cost multiplied by
// the PO's entered quantity. The source sell field must never become capital.
func TestPurchaseOrderReportUsesPinnedCostAtThePOQuantity(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_cost")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Cost basis", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Widget", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250}})
	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-REPORT-COST", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	// #573 retired the whole-order discount and adjustment: a current save
	// writes zero, which is what lets the amount formula stay simple.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var discount, adjustment int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT discount_minor, adjustment_minor FROM %s.purchase_orders WHERE id = $1`,
		database.configSchema()), saved.ID).Scan(&discount, &adjustment); err != nil {
		t.Fatalf("read retired fields: %v", err)
	}
	if discount != 0 || adjustment != 0 {
		t.Fatalf("a current save wrote retired fields: discount=%d adjustment=%d", discount, adjustment)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 {
		t.Fatalf("poCount = %d; want 1", report.Totals.PoCount)
	}
	if report.Totals.AmountMinor != 500 {
		t.Errorf("amount = %d; want 2 x 250 = 500", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 500 {
		t.Errorf("costed = %d; want the resolved lines' net = 500", report.Totals.CostedMinor)
	}
	if report.Totals.CapitalMinor != 200 {
		t.Errorf("capital = %d; want 2 x 100 cost = 200 (never the 250 sell field)",
			report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 300 {
		t.Errorf("margin = %d; want costed 500 - capital 200 = 300", report.Totals.MarginMinor)
	}
	if report.Totals.UncostedMinor != 0 {
		t.Errorf("uncosted = %d; want 0 when every line resolved", report.Totals.UncostedMinor)
	}
	if report.Totals.MarginBps == nil || *report.Totals.MarginBps != 15000 {
		t.Errorf("margin bps = %v; want 15000", report.Totals.MarginBps)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 10000 {
		t.Errorf("coverage bps = %v; want 10000", report.Totals.CoverageBps)
	}
}

// TestPurchaseOrderReportRoundsCapitalOncePerPO pins the two different
// rounding rules: within one PO the lines sum before the single division and
// round, while across POs each PO's capital is rounded first and the rounded
// values are summed. Rounding per line would turn two half units into 2 instead
// of 1; rounding across POs would turn two half-unit POs into 1 instead of 2.
func TestPurchaseOrderReportRoundsCapitalOncePerPO(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_rounding")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Fractional capital", "IDR", "FTZ",
		[]reportQuotationLine{
			{Description: "Half A", UoM: "Pc", Qty: 1, Cost: 1, Sell: 10},
			{Description: "Half B", UoM: "Pc", Qty: 1, Cost: 1, Sell: 10},
		})
	finalizeReportPO(t, database, ctx, user, quotation, "PO-REPORT-HALF-ONE", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			for index := range input.Items {
				input.Items[index].QuantityScaled = 5000
				input.Decisions = append(input.Decisions, PurchaseOrderDecision{
					Key: fmt.Sprintf("line:%d:quantity", index+1), Choice: PurchaseOrderChoicePO,
				})
			}
		})

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if report.Totals.CapitalMinor != 1 {
		t.Fatalf("one PO with 0.5 + 0.5 at cost 1 has capital %d; want 1 (sum before rounding)",
			report.Totals.CapitalMinor)
	}

	// A second PO in the same month carries the same half unit. Its own capital
	// rounds to 1, and the report sums the rounded PO figures: 2, not 1.
	single := seedReportQuotation(t, database, 3, "PT. Buyer Two", "Half PO", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Half", UoM: "Pc", Qty: 1, Cost: 1, Sell: 10}})
	finalizeReportPO(t, database, ctx, user, single, "PO-REPORT-HALF-TWO", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 5000
			input.Decisions = []PurchaseOrderDecision{{
				Key: "line:1:quantity", Choice: PurchaseOrderChoicePO,
			}}
		})
	report = mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if report.Totals.CapitalMinor != 2 {
		t.Fatalf("two POs with half-unit capital have total %d; want 2 (round once per PO)",
			report.Totals.CapitalMinor)
	}
}

// TestPurchaseOrderReportPinsTheSourceRevisionAndCurrency proves cost comes
// from the exact revision the PO followed, not the quotation's latest one, and
// that a revision in another currency is not converted into this report.
func TestPurchaseOrderReportPinsTheSourceRevisionAndCurrency(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_pinning")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	rev0 := seedReportQuotation(t, database, 2, "PT. Buyer", "Pinned revision", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Pinned line", UoM: "Pc", Qty: 2, Cost: 100, Sell: 300}})
	saved := finalizeReportPO(t, database, ctx, user, rev0, "PO-REPORT-PIN", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	// The quotation moves on. A report that joined the latest revision would use
	// the 700 cost or the 900 sell; the PO must keep the 100 cost and 300 sell.
	if _, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "PT. Buyer",
		CustomerAddress:  "Jl. Report Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Report Recipient 9",
		Subject:          "Pinned revision moved on",
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		DiscountPercent:  0,
		TaxZone:          "FTZ",
		TaxType:          "Goods",
		Currency:         "IDR",
		QuoteNumber:      rev0.QuoteNumber,
		RevisionNumber:   1,
		HTMLSnapshot:     "<html>revision one</html>",
		Items: []RevisionItem{{
			ItemNo: 1, Description: "Pinned line", UoM: "Pc", Qty: 2,
			UnitPrice: 700, MarginalizedUnitPrice: 900, ListUnitPrice: 900,
			OriginalTotal: 1400, MarginalizedTotal: 1800,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal: 1400, OriginalGrand: 1400,
			MarginalizedSubtotal: 1800, MarginalizedGrand: 1800,
			MarginalizedGrandAfterTax: 1800, CustomerGrandTotal: 1800,
		},
	}); err != nil {
		t.Fatalf("issue revision 1: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if report.Totals.CapitalMinor != 200 {
		t.Errorf("capital = %d; want the pinned revision's 100 x 2", report.Totals.CapitalMinor)
	}
	if report.Totals.CostedMinor != 600 {
		t.Errorf("costed = %d; want the pinned revision's sell 300 x 2", report.Totals.CostedMinor)
	}

	// A PO whose source revision is in another currency has no resolvable cost
	// in this partition. It must not be converted, and its revenue stays.
	foreign := seedReportQuotation(t, database, 3, "PT. Foreign", "Foreign currency", "USD", "FTZ",
		[]reportQuotationLine{{Description: "Dollar line", UoM: "Pc", Qty: 1, Cost: 400, Sell: 900}})
	foreignPO := finalizeReportPO(t, database, ctx, user, foreign, "PO-REPORT-FOREIGN", "2026-03-11",
		PurchaseOrderBusinessCategoryFm, nil)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_orders SET currency = 'IDR' WHERE id = $1`,
		database.configSchema()), foreignPO.ID); err != nil {
		t.Fatalf("move PO into the IDR partition: %v", err)
	}
	report = mustReport(t, database, ctx,
		reportQuery("2026-01-31", "2026-03-12", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 2 || report.Totals.AmountMinor != 600+900 {
		t.Fatalf("mixed-currency report = %+v; want both revenues in the IDR partition", report.Totals)
	}
	if report.Totals.CostedMinor != 600 || report.Totals.CapitalMinor != 200 {
		t.Errorf("foreign-source PO resolved a cost across currencies: costed=%d capital=%d",
			report.Totals.CostedMinor, report.Totals.CapitalMinor)
	}
	if report.Totals.UncostedMinor != 900 {
		t.Errorf("uncosted = %d; want the foreign PO's revenue 900", report.Totals.UncostedMinor)
	}
	_ = saved
}

// TestPurchaseOrderReportKeepsDeletedSourceCostFromAllocation proves that a
// surviving allocated PO keeps its frozen cost snapshot after its Quotation is
// permanently deleted. The old model lost the cost with the source; the
// allocation is now the historical basis, so revenue, cost and margin survive.
func TestPurchaseOrderReportKeepsDeletedSourceCostFromAllocation(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_deleted_source")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Survives", "PO-REPORT-DELETED")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetQuotation,
		TargetID:            quotation.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "report fixture deletes the source",
		TypedNumber:         quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             admin.ID,
	}); err != nil {
		t.Fatalf("delete quotation record: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "USD"))
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != saved.GrandTotalMinor {
		t.Fatalf("deleted-source report = %+v; want one PO worth %d",
			report.Totals, saved.GrandTotalMinor)
	}
	if report.Totals.CostedMinor != saved.GrandTotalMinor || report.Totals.CapitalMinor != saved.GrandTotalMinor {
		t.Errorf("deleted source lost its allocation snapshot: costed=%d capital=%d; want %d/%d",
			report.Totals.CostedMinor, report.Totals.CapitalMinor,
			saved.GrandTotalMinor, saved.GrandTotalMinor)
	}
	if report.Totals.MarginMinor != 0 {
		t.Errorf("deleted source margin = %d; want 0 when cost equals sell", report.Totals.MarginMinor)
	}
	if report.Totals.UncostedMinor != 0 {
		t.Errorf("uncosted = %d; want 0 with the allocation snapshot", report.Totals.UncostedMinor)
	}
	if report.SourceMissingPoCount != 1 {
		t.Errorf("sourceMissingPoCount = %d; want 1", report.SourceMissingPoCount)
	}
	if report.Totals.MarginBps == nil || *report.Totals.MarginBps != 0 {
		t.Errorf("margin bps = %v; want 0 with positive capital and zero margin", report.Totals.MarginBps)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 10000 {
		t.Errorf("coverage bps = %v; want 10000", report.Totals.CoverageBps)
	}
}

// TestPurchaseOrderReportCountsACorrectionOnceAtItsCurrentValues covers the
// correction rule: the live row is the report row, so a corrected order is one
// PO at its new value and may move to another period, never two POs and never
// its old value.
func TestPurchaseOrderReportCountsACorrectionOnceAtItsCurrentValues(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_correction")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Correction source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Corrected line", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250}})
	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-REPORT-CORRECT", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	before := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if before.Totals.PoCount != 1 || before.Totals.AmountMinor != 500 {
		t.Fatalf("before correction = %+v; want one 500 PO", before.Totals)
	}

	received := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	corrected, err := database.CorrectPurchaseOrder(ctx, PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: PurchaseOrderUpdate{
			ID:               saved.ID,
			ExpectedVersion:  saved.Version,
			UpdatedBy:        user.ID,
			PONumber:         saved.PONumber,
			ReceivedDate:     &received,
			BusinessCategory: PurchaseOrderBusinessCategoryFm,
			Items: []PurchaseOrderItem{{
				QuotationItemNo: saved.Items[0].QuotationItemNo,
				Description:     saved.Items[0].Description,
				QuantityScaled:  40000,
				UoM:             saved.Items[0].UoM,
				UnitPriceMinor:  saved.Items[0].UnitPriceMinor,
			}},
		},
		Reason: "customer increased the order",
	})
	if err != nil {
		t.Fatalf("correct purchase order: %v", err)
	}
	if corrected.ID != saved.ID {
		t.Fatalf("correction created a new PO %d; want the same row %d", corrected.ID, saved.ID)
	}

	oldPeriod := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if oldPeriod.Totals.PoCount != 0 || oldPeriod.Totals.AmountMinor != 0 {
		t.Errorf("March after correction = %+v; want the record moved out", oldPeriod.Totals)
	}
	newPeriod := mustReport(t, database, ctx,
		reportQuery("2026-04-01", "2026-05-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if newPeriod.Totals.PoCount != 1 {
		t.Fatalf("April poCount = %d; want the corrected record counted once", newPeriod.Totals.PoCount)
	}
	if newPeriod.Totals.AmountMinor != 1000 {
		t.Errorf("April amount = %d; want 4 x 250 = 1000 after correction", newPeriod.Totals.AmountMinor)
	}
	if newPeriod.Totals.CapitalMinor != 400 {
		t.Errorf("April capital = %d; want 4 x 100 = 400", newPeriod.Totals.CapitalMinor)
	}
}

// TestPurchaseOrderReportExcludesDraftsAndDatesHalfOpen pins the population
// boundary: a draft is not an order, From is included and To is excluded.
func TestPurchaseOrderReportExcludesDraftsAndDatesHalfOpen(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_population")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Population source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Dated line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID: quotation.ID, RevisionNumber: quotation.RevisionNumber, CreatedBy: user.ID,
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if draft.State != PurchaseOrderStateDraft {
		t.Fatalf("fixture state = %q; want draft", draft.State)
	}

	edgeIn := seedReportQuotation(t, database, 3, "PT. Edge", "Edge in source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Edge line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	edgeOut := seedReportQuotation(t, database, 3, "PT. Edge", "Edge out source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Edge line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	finalizeReportPO(t, database, ctx, user, edgeIn, "PO-REPORT-EDGE-IN", "2026-03-01",
		PurchaseOrderBusinessCategoryFm, nil)
	finalizeReportPO(t, database, ctx, user, edgeOut, "PO-REPORT-EDGE-OUT", "2026-04-01",
		PurchaseOrderBusinessCategoryFm, nil)

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != 200 {
		t.Fatalf("half-open report = %+v; want only the 2026-03-01 PO", report.Totals)
	}
	if len(report.Periods) != 1 || report.Periods[0].PeriodStart != "2026-03-01" {
		t.Errorf("periods = %+v; want one March bucket", report.Periods)
	}
}

// TestPurchaseOrderReportFillsContinuousPeriodsAndProgress proves the report
// is a complete calendar axis: empty months stay present as zeros, clipping the
// first or last bucket marks it partial, and the bucket holding the as-of date
// is in progress.
func TestPurchaseOrderReportFillsContinuousPeriodsAndProgress(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_periods")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	januaryQuote := seedReportQuotation(t, database, 2, "PT. Buyer", "Period Jan source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Period line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	marchQuote := seedReportQuotation(t, database, 2, "PT. Buyer", "Period Mar source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Period line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	finalizeReportPO(t, database, ctx, user, januaryQuote, "PO-REPORT-JAN", "2026-01-31",
		PurchaseOrderBusinessCategoryFm, nil)
	finalizeReportPO(t, database, ctx, user, marchQuote, "PO-REPORT-MAR", "2026-03-01",
		PurchaseOrderBusinessCategoryFm, nil)

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-15", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if len(report.Periods) != 3 {
		t.Fatalf("periods = %d; want Jan, Feb, Mar continuous", len(report.Periods))
	}
	january := reportPeriod(t, report, "2026-01-01")
	if january.PoCount != 1 || january.AmountMinor != 200 {
		t.Errorf("January = %+v; want the Jan 31 PO", january.Metrics)
	}
	if !january.Partial || january.InProgress {
		t.Errorf("January partial=%v inProgress=%v; want partial, not in progress at Nov 19",
			january.Partial, january.InProgress)
	}
	february := reportPeriod(t, report, "2026-02-01")
	if february.PoCount != 0 || february.AmountMinor != 0 || february.MarginBps != nil {
		t.Errorf("February = %+v; want a zero actual month with null ratios", february.Metrics)
	}
	if february.Partial {
		t.Error("February is a complete month inside the range; want partial=false")
	}
	march := reportPeriod(t, report, "2026-03-01")
	if march.PoCount != 1 || march.Partial {
		t.Errorf("March = %+v; want the Mar 1 PO in a complete last bucket", march.Metrics)
	}

	// While March is the current company month, it is marked in progress; April
	// is still included as an actual zero bucket but is not in progress.
	marchNow := time.Date(2026, 3, 15, 5, 0, 0, 0, time.UTC)
	inProgress, err := database.ReportPurchaseOrders(ctx,
		reportQuery("2026-01-01", "2026-05-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"),
		marchNow)
	if err != nil {
		t.Fatalf("report at March as-of: %v", err)
	}
	if !reportPeriod(t, inProgress, "2026-03-01").InProgress {
		t.Error("March at as-of 2026-03-15 is not marked in progress")
	}
	if reportPeriod(t, inProgress, "2026-04-01").InProgress {
		t.Error("April is future relative to 2026-03-15; it must not be in progress")
	}
}

// TestPurchaseOrderReportFiltersCategoriesUnassignedAndFacets covers the
// category selector and the facets that must not move with it: All includes
// legacy Unassigned records, a single code filters to exactly that row, the
// currency facet is date-scoped and the unassigned flag ignores the category.
func TestPurchaseOrderReportFiltersCategoriesUnassignedAndFacets(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_categories")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	fm := seedReportQuotation(t, database, 2, "PT. Buyer", "FM source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "FM line", UoM: "Pc", Qty: 1, Cost: 10, Sell: 100}})
	fab := seedReportQuotation(t, database, 3, "PT. Buyer Two", "FAB source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "FAB line", UoM: "Pc", Qty: 1, Cost: 20, Sell: 200}})
	legacy := seedReportQuotation(t, database, 4, "PT. Buyer Three", "Legacy source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Legacy line", UoM: "Pc", Qty: 1, Cost: 30, Sell: 300}})
	usd := seedReportQuotation(t, database, 5, "PT. Buyer Four", "USD source", "USD", "FTZ",
		[]reportQuotationLine{{Description: "USD line", UoM: "Pc", Qty: 1, Cost: 40, Sell: 400}})

	finalizeReportPO(t, database, ctx, user, fm, "PO-REPORT-FM", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)
	finalizeReportPO(t, database, ctx, user, fab, "PO-REPORT-FAB", "2026-03-10",
		PurchaseOrderBusinessCategoryFab, nil)
	legacyPO := finalizeReportPO(t, database, ctx, user, legacy, "PO-REPORT-LEGACY", "2026-03-10",
		PurchaseOrderBusinessCategoryOthers, nil)
	finalizeReportPO(t, database, ctx, user, usd, "PO-REPORT-USD", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)
	// A record saved before #628 carries the empty category: Unassigned is a
	// real row, never inferred and never dropped from All.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_orders SET business_category = '' WHERE id = $1`,
		database.configSchema()), legacyPO.ID); err != nil {
		t.Fatalf("clear legacy category: %v", err)
	}

	all := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if all.Totals.PoCount != 3 || all.Totals.AmountMinor != 600 {
		t.Fatalf("All IDR = %+v; want three POs worth 600", all.Totals)
	}
	wantOrder := []string{"FM", "FAB", "SUPPLY_GEN", "OTHERS", "UNASSIGNED"}
	if len(all.Categories) != len(wantOrder) {
		t.Fatalf("categories = %+v; want five rows", all.Categories)
	}
	for index, code := range wantOrder {
		if all.Categories[index].BusinessCategory != code {
			t.Errorf("category %d = %s; want %s in fixed order",
				index, all.Categories[index].BusinessCategory, code)
		}
	}
	if got := reportCategory(t, all, "UNASSIGNED"); got.PoCount != 1 || got.AmountMinor != 300 {
		t.Errorf("unassigned row = %+v; want the legacy 300", got.Metrics)
	}
	if !all.HasUnassigned {
		t.Error("HasUnassigned = false; want true")
	}
	if len(all.AvailableCurrencies) != 2 ||
		all.AvailableCurrencies[0] != "IDR" || all.AvailableCurrencies[1] != "USD" {
		t.Errorf("currencies = %v; want date-scoped IDR and USD", all.AvailableCurrencies)
	}

	fmOnly := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryFm, "IDR"))
	if fmOnly.Totals.PoCount != 1 || fmOnly.Totals.AmountMinor != 100 {
		t.Errorf("FM filter = %+v; want one 100 PO", fmOnly.Totals)
	}
	if len(fmOnly.Categories) != 1 || fmOnly.Categories[0].BusinessCategory != "FM" {
		t.Errorf("FM categories = %+v; want exactly one FM row", fmOnly.Categories)
	}
	if !fmOnly.HasUnassigned {
		t.Error("HasUnassigned must ignore the category filter; want true")
	}
	if len(fmOnly.AvailableCurrencies) != 2 {
		t.Errorf("FM currencies = %v; want the date-scoped IDR and USD regardless of category",
			fmOnly.AvailableCurrencies)
	}
	unassignedOnly := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryUnassigned, "IDR"))
	if unassignedOnly.Totals.PoCount != 1 || unassignedOnly.Totals.AmountMinor != 300 {
		t.Errorf("UNASSIGNED filter = %+v; want one 300 PO", unassignedOnly.Totals)
	}

	// The currency facet ignores the category selector but not the date: a PO
	// outside the range must not offer its currency.
	outside := mustReport(t, database, ctx,
		reportQuery("2026-05-01", "2026-06-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if len(outside.AvailableCurrencies) != 1 || outside.AvailableCurrencies[0] != "IDR" {
		t.Errorf("out-of-range currencies = %v; want only the selected IDR", outside.AvailableCurrencies)
	}
	if outside.HasUnassigned {
		t.Error("HasUnassigned is date-scoped; an empty range must not report true")
	}
}

// TestPurchaseOrderReportKeepsLossesDiscountsAndPPNOutOfTheFigures proves three
// independently locked rules: a negotiated price below cost can make PO margin
// negative, PPN is excluded from amount, costed, capital and margin, and the
// signed figures stay signed rather than clamping at zero.
func TestPurchaseOrderReportKeepsLossesDiscountsAndPPNOutOfTheFigures(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_loss_ppn")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	loss := seedReportQuotation(t, database, 2, "PT. Buyer", "Loss source", "IDR", "NonFTZ",
		[]reportQuotationLine{{
			Description: "Negotiated below cost", UoM: "Pc", Qty: 1, Cost: 100, Sell: 120,
		}})
	// #653: the variance is the negotiated price against the quotation, not a
	// hidden row discount. A PO at 60 against a 120 quote and a 100 cost is a
	// real price loss of 60 and a margin of -40.
	saved := finalizeReportPO(t, database, ctx, user, loss, "PO-REPORT-LOSS", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].UnitPriceMinor = 60
			input.Decisions = []PurchaseOrderDecision{{
				Key: "line:1:unitPrice", Choice: PurchaseOrderChoicePO,
			}}
		})
	if saved.PpnMinor <= 0 {
		t.Fatalf("fixture PPN = %d; want a NonFTZ goods order to charge PPN", saved.PpnMinor)
	}
	if saved.GrandTotalMinor != saved.SubtotalMinor+saved.PpnMinor-saved.DiscountMinor+saved.AdjustmentMinor {
		t.Fatalf("fixture grand total did not use PPN: %+v", saved)
	}

	before := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if before.Totals.AmountMinor != 60 {
		t.Errorf("amount = %d; want the discounted 60, excluding PPN", before.Totals.AmountMinor)
	}
	if before.Totals.CostedMinor != 60 {
		t.Errorf("costed = %d; want 60", before.Totals.CostedMinor)
	}
	if before.Totals.CapitalMinor != 100 {
		t.Errorf("capital = %d; want the undiscounted cost 100", before.Totals.CapitalMinor)
	}
	if before.Totals.MarginMinor != -40 {
		t.Errorf("margin = %d; want the signed loss -40", before.Totals.MarginMinor)
	}
	if before.Totals.MarginBps == nil || *before.Totals.MarginBps != -4000 {
		t.Errorf("margin bps = %v; want -4000", before.Totals.MarginBps)
	}

	// Change only the PPN snapshot. No report figure may move, because PPN is
	// collected for the state and is not part of any PO performance number.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET ppn_bps = 1100, ppn_minor = ppn_minor + 777,
		    grand_total_minor = grand_total_minor + 777
		WHERE id = $1`, database.configSchema()), saved.ID); err != nil {
		t.Fatalf("raise PPN: %v", err)
	}
	after := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if after.Totals.AmountMinor != before.Totals.AmountMinor ||
		after.Totals.CostedMinor != before.Totals.CostedMinor ||
		after.Totals.CapitalMinor != before.Totals.CapitalMinor ||
		after.Totals.MarginMinor != before.Totals.MarginMinor {
		t.Errorf("PPN change moved the report: before %+v after %+v", before.Totals, after.Totals)
	}
}

// TestPurchaseOrderReportUsesSubItemCostWithoutFanout proves a package PO line
// resolves the sub-item's unit_cost once, at the PO's quantity, and that the
// parent reference does not multiply the row across its sub-items. The parent's
// own item cost is deliberately absurd: it must never be used.
func TestPurchaseOrderReportUsesSubItemCostWithoutFanout(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_package")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Package source", "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "Package",
			UoM:         "Lot",
			Qty:         1,
			Cost:        999999,
			Sell:        500,
			SubItems: []reportQuotationSub{
				{Description: "Part A", UoM: "Pc", Qty: 1, Cost: 100, Sell: 300},
				{Description: "Part B", UoM: "Pc", Qty: 1, Cost: 50, Sell: 200},
			},
		}})
	finalizeReportPO(t, database, ctx, user, quotation, "PO-REPORT-PACKAGE", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			if len(input.Items) != 2 {
				t.Fatalf("package draft has %d lines; want one per priced sub-item", len(input.Items))
			}
			for index := range input.Items {
				input.Items[index].QuantityScaled = 20000
				input.Decisions = append(input.Decisions, PurchaseOrderDecision{
					Key: fmt.Sprintf("line:%d:quantity", index+1), Choice: PurchaseOrderChoicePO,
				})
			}
		})

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 {
		t.Fatalf("package poCount = %d; want 1", report.Totals.PoCount)
	}
	if report.Totals.AmountMinor != 1000 {
		t.Errorf("amount = %d; want 2 x (300 + 200) = 1000", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 1000 {
		t.Errorf("costed = %d; want the two sub-item nets = 1000", report.Totals.CostedMinor)
	}
	if report.Totals.CapitalMinor != 300 {
		t.Errorf("capital = %d; want 2 x 100 + 2 x 50 = 300 from sub-item unit_cost",
			report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 700 {
		t.Errorf("margin = %d; want 1000 - 300 = 700", report.Totals.MarginMinor)
	}
	// #653: the package's priced parts are the variance leaves. Buying two of
	// each part leaves nothing Unsold and earns one extra unit of each part:
	// 300 + 200 at the quoted baseline, less 100 + 50 of frozen cost.
	if report.Pipeline.UnsoldAmountMinor != 0 || report.Pipeline.UnsoldMarginMinor != 0 {
		t.Errorf("package pipeline = %d/%d; want 0/0",
			report.Pipeline.UnsoldAmountMinor, report.Pipeline.UnsoldMarginMinor)
	}
	if report.Variance.QuantityExtraIncomeAmountMinor != 500 ||
		report.Variance.QuantityExtraIncomeMarginMinor != 350 {
		t.Errorf("package quantity extra = %d/%d; want 500/350",
			report.Variance.QuantityExtraIncomeAmountMinor,
			report.Variance.QuantityExtraIncomeMarginMinor)
	}
	if report.Variance.PriceLossMinor != 0 || report.Variance.PriceExtraIncomeMinor != 0 {
		t.Errorf("package price variance = loss %d / extra %d; want 0/0",
			report.Variance.PriceLossMinor, report.Variance.PriceExtraIncomeMinor)
	}
}

// TestPurchaseOrderReportIsOrganizationScoped proves the report never crosses
// the #465 tenant boundary, including through the source-cost join and the
// facets.
func TestPurchaseOrderReportIsOrganizationScoped(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_org")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Tenant source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Tenant line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	finalizeReportPO(t, database, ctx, user, quotation, "PO-REPORT-ORG", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var otherOrg string
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.organizations (name) VALUES ('Other Tenant') RETURNING id::text`,
		database.configSchema())).Scan(&otherOrg); err != nil {
		t.Fatalf("create second organization: %v", err)
	}
	// A second issued revision gives the other tenant a real source cost. Its PO
	// is inserted directly because the normal path is organization-scoped: this
	// fixture exists precisely to prove the other organization's rows are
	// invisible to the caller's report.
	otherQuotation := seedReportQuotation(t, database, 3, "PT. Other Tenant", "Other tenant source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Tenant line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200,
		}})
	var otherPO int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id, customer_name,
			received_date, currency, tax_zone, tax_type, service_category,
			subtotal_minor, discount_minor, discount_percent, ppn_bps, ppn_minor,
			adjustment_minor, adjustment_reason, grand_total_minor,
			printed_subtotal_minor, printed_discount_minor, printed_ppn_minor,
			printed_grand_total_minor, po_number, po_number_key, saved_at, saved_by,
			created_by, updated_by, business_category
		)
		SELECT $1, q.id, 0, 'saved', q.customer_id, 'PT. Other Tenant',
		       DATE '2026-03-10', 'IDR', 'FTZ', 'Goods', '',
		       200, 0, 0, 0, 0, 0, '', 200, 200, 0, 0, 200,
		       'PO-REPORT-OTHER-ORG', 'po-report-other-org', NOW(), $2, $2, $2, 'FM'
		FROM %[1]s.quotations q
		WHERE q.id = $3
		RETURNING id`, database.configSchema()), otherOrg, user.ID, otherQuotation.ID).Scan(&otherPO); err != nil {
		t.Fatalf("insert the other tenant's saved PO: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %[1]s.purchase_order_items (
			purchase_order_id, line_number, description, quotation_item_no,
			quantity_scaled, uom, unit_price_minor, gross_minor, discount_minor, net_minor
		) VALUES ($1, 1, 'Tenant line', 1, 10000, 'Pc', 200, 200, 0, 200)`,
		database.configSchema()), otherPO); err != nil {
		t.Fatalf("insert the other tenant's PO line: %v", err)
	}

	defaultOrg := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if defaultOrg.Totals.PoCount != 1 || defaultOrg.Totals.AmountMinor != 200 {
		t.Errorf("default org report = %+v; want only its own 200 PO", defaultOrg.Totals)
	}
	if len(defaultOrg.AvailableCurrencies) != 1 || defaultOrg.AvailableCurrencies[0] != "IDR" {
		t.Errorf("default org currencies = %v; want only the selected currency", defaultOrg.AvailableCurrencies)
	}

	otherReport, err := database.ReportPurchaseOrders(ContextWithOrg(ctx, otherOrg),
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"),
		reportNow())
	if err != nil {
		t.Fatalf("other org report: %v", err)
	}
	if otherReport.Totals.PoCount != 1 || otherReport.Totals.AmountMinor != 200 {
		t.Errorf("other org report = %+v; want its own 200 PO", otherReport.Totals)
	}
	if otherReport.Totals.CapitalMinor != 100 {
		t.Errorf("other org capital = %d; want the source cost 100 resolved within the tenant",
			otherReport.Totals.CapitalMinor)
	}
}

// TestPurchaseOrderReportFailsClosedAboveTheExactRange proves aggregate
// overflow is a typed refusal, not a wrapped figure. Two POs each at
// MaxExactMinor sum above the exact JSON range while every row is individually
// valid.
func TestPurchaseOrderReportFailsClosedAboveTheExactRange(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_overflow")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	first := seedReportQuotation(t, database, 2, "PT. Buyer", "Overflow one", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	second := seedReportQuotation(t, database, 3, "PT. Buyer Two", "Overflow two", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	one := finalizeReportPO(t, database, ctx, user, first, "PO-REPORT-OVERFLOW-1", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)
	two := finalizeReportPO(t, database, ctx, user, second, "PO-REPORT-OVERFLOW-2", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	for _, id := range []int64{one.ID, two.ID} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_orders
			SET subtotal_minor = $2, grand_total_minor = $2
			WHERE id = $1`, database.configSchema()), id, poreport.MaxExactMinor); err != nil {
			t.Fatalf("raise subtotal on %d: %v", id, err)
		}
	}
	if _, err := database.ReportPurchaseOrders(ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"),
		reportNow()); !errors.Is(err, poreport.ErrValueOutOfRange) {
		t.Fatalf("overflow report = %v; want ErrValueOutOfRange", err)
	}
}

// TestPurchaseOrderReportReportsLegacyAdjustmentsAsAResidual records what a
// restored pre-#573 dataset looks like: the amount keeps the retired field, the
// snapshot counts it, and the caller can disclose it rather than call it
// supplier cost.
func TestPurchaseOrderReportReportsLegacyAdjustmentsAsAResidual(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_legacy_adjustment")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Legacy adjustment", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 300}})
	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-REPORT-LEGACY-ADJ", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET adjustment_minor = 50,
		    adjustment_reason = 'restored legacy adjustment',
		    grand_total_minor = grand_total_minor + 50
		WHERE id = $1`,
		database.configSchema()), saved.ID); err != nil {
		t.Fatalf("restore legacy adjustment: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.LegacyAdjustmentPoCount != 1 {
		t.Fatalf("legacy adjustment count = %d; want 1", report.LegacyAdjustmentPoCount)
	}
	if report.Totals.AmountMinor != 350 {
		t.Errorf("amount = %d; want subtotal 300 + the restored adjustment 50", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 300 || report.Totals.CapitalMinor != 100 {
		t.Errorf("legacy adjustment leaked into cost: costed=%d capital=%d",
			report.Totals.CostedMinor, report.Totals.CapitalMinor)
	}
	if report.Totals.UncostedMinor != 50 {
		t.Errorf("uncosted = %d; want the 50 residual disclosed as uncosted, not supplier cost",
			report.Totals.UncostedMinor)
	}
}

// TestPurchaseOrderReportMultipliesTheCostAsNumeric proves the cost x scaled
// quantity product is evaluated exactly before the division. 9,007,199,254,740,991
// cost for one unit is valid on each side, but its scaled product overflows
// int64; Postgres numeric carries it and the per-PO capital lands exactly at the
// allowed bound instead of wrapping or rounding through a float.
func TestPurchaseOrderReportMultipliesTheCostAsNumeric(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_exact_product")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Buyer", "Exact product", "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "Large exact line", UoM: "Pc", Qty: 1,
			Cost: poreport.MaxExactMinor, Sell: poreport.MaxExactMinor,
		}})
	finalizeReportPO(t, database, ctx, user, quotation, "PO-REPORT-EXACT", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.AmountMinor != poreport.MaxExactMinor {
		t.Errorf("amount = %d; want the exact bound", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != poreport.MaxExactMinor {
		t.Errorf("costed = %d; want the exact bound", report.Totals.CostedMinor)
	}
	if report.Totals.CapitalMinor != poreport.MaxExactMinor {
		t.Errorf("capital = %d; want the exact bound after exact product and division",
			report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 0 {
		t.Errorf("margin = %d; want zero at equal cost and sell", report.Totals.MarginMinor)
	}
}

// TestPurchaseOrderReportMigrationCreatesThePartialIndex pins the performance
// shape issue #629 asked for: org and received date, saved rows only. A new
// migration must not replace it with a full-table index that drafts maintain on
// every edit.
func TestPurchaseOrderReportMigrationCreatesThePartialIndex(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_index")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var definition string
	if err := pool.QueryRow(context.Background(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = $1 AND tablename = 'purchase_orders'
		  AND indexname = 'purchase_orders_report_idx'`,
		database.config.Schema).Scan(&definition); err != nil {
		t.Fatalf("read report index: %v", err)
	}
	for _, part := range []string{"(org_id, received_date)", "WHERE (state = 'saved'::text)"} {
		if !strings.Contains(definition, part) {
			t.Errorf("report index = %q; want %q", definition, part)
		}
	}
	var applied bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM `+database.configSchema()+`.schema_migrations WHERE version = 51
		)`).Scan(&applied); err != nil {
		t.Fatalf("read migration 051 ledger: %v", err)
	}
	if !applied {
		t.Error("migration 051 is not in the ledger")
	}
}

// TestPurchaseOrderReportKeepsActualsWhenHistoryAmountExceedsTheExactRange is
// the Judge regression: two individually valid historical POs sum above the
// exact JSON range in January 2025. That month is not an actual figure for the
// March 2026 report, so the actual snapshot must survive and only the forecast
// is suppressed with value_out_of_range and the truthful basis.
func TestPurchaseOrderReportKeepsActualsWhenHistoryAmountExceedsTheExactRange(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_history_amount_overflow")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	historyOne := seedReportQuotation(t, database, 2, "PT. Buyer", "History overflow one", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	historyTwo := seedReportQuotation(t, database, 3, "PT. Buyer Two", "History overflow two", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	actualQuote := seedReportQuotation(t, database, 4, "PT. Buyer Three", "Safe actual", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})

	one := finalizeReportPO(t, database, ctx, user, historyOne, "PO-HIST-OVERFLOW-1", "2025-01-10",
		PurchaseOrderBusinessCategoryFm, nil)
	two := finalizeReportPO(t, database, ctx, user, historyTwo, "PO-HIST-OVERFLOW-2", "2025-01-10",
		PurchaseOrderBusinessCategoryFm, nil)
	actual := finalizeReportPO(t, database, ctx, user, actualQuote, "PO-HIST-OVERFLOW-ACTUAL", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	for _, id := range []int64{one.ID, two.ID} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_orders
			SET subtotal_minor = $2, grand_total_minor = $2
			WHERE id = $1`, database.configSchema()), id, poreport.MaxExactMinor); err != nil {
			t.Fatalf("raise historical amount on %d: %v", id, err)
		}
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != actual.SubtotalMinor {
		t.Fatalf("actual totals = %+v; want the safe March PO preserved", report.Totals)
	}
	if !report.HistoryOutOfRange {
		t.Error("history out-of-range = false; want the oversized January 2025 aggregate flagged")
	}
	if len(report.History) != 0 {
		t.Errorf("history = %+v; want no sample passed to the projection", report.History)
	}
	basis := report.HistoryBasis
	if basis.HistoryFrom != "2025-01-01" || basis.HistoryTo != "2026-04-01" ||
		basis.CompleteMonths != 15 || basis.PoCount != 3 || basis.AsOf != "2026-11-19" {
		t.Errorf("history basis = %+v; want 2025-01-01..2026-04-01, 15 months, 3 POs, as of 2026-11-19", basis)
	}
}

// TestPurchaseOrderReportKeepsAnEmptyActualRangeWhenHistoryAmountExceedsTheExactRange
// is the same defect with no actual PO at all: the report is still a valid
// empty report, not a 422. The forecast states value_out_of_range rather than
// no_data_in_range because the stored history it would have used is corrupt.
func TestPurchaseOrderReportKeepsAnEmptyActualRangeWhenHistoryAmountExceedsTheExactRange(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_history_amount_overflow_empty")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	first := seedReportQuotation(t, database, 2, "PT. Buyer", "Empty history overflow one", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	second := seedReportQuotation(t, database, 3, "PT. Buyer Two", "Empty history overflow two", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	one := finalizeReportPO(t, database, ctx, user, first, "PO-HIST-OVERFLOW-EMPTY-1", "2025-01-10",
		PurchaseOrderBusinessCategoryFm, nil)
	two := finalizeReportPO(t, database, ctx, user, second, "PO-HIST-OVERFLOW-EMPTY-2", "2025-01-10",
		PurchaseOrderBusinessCategoryFm, nil)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	for _, id := range []int64{one.ID, two.ID} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_orders
			SET subtotal_minor = $2, grand_total_minor = $2
			WHERE id = $1`, database.configSchema()), id, poreport.MaxExactMinor); err != nil {
			t.Fatalf("raise historical amount on %d: %v", id, err)
		}
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 0 || report.Totals.AmountMinor != 0 {
		t.Errorf("actual totals = %+v; want an empty March 2026 actual range", report.Totals)
	}
	if !report.HistoryOutOfRange {
		t.Error("history out-of-range = false; want the oversized January 2025 aggregate flagged")
	}
	if len(report.History) != 0 {
		t.Errorf("history = %+v; want no sample passed to the projection", report.History)
	}
	if basis := report.HistoryBasis; basis.CompleteMonths != 15 || basis.PoCount != 2 {
		t.Errorf("history basis = %+v; want 15 complete months and 2 historical POs", basis)
	}
}

// TestPurchaseOrderReportDoesNotComputeUnusedHistoryRatios is the ratio half of
// the Judge regression. A restored whole-order discount makes January 2025's
// amount 1 against a costed MaxExactMinor; every stored money field is exact,
// but the unused coverage percentage would overflow. The history the projection
// actually reads is amount/costed/capital, so the actual report and the sample
// must remain usable. A forecast-derived ratio that genuinely overflows is
// still caught by the domain projection, not here.
func TestPurchaseOrderReportDoesNotComputeUnusedHistoryRatios(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_history_ratio_overflow")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	ratioQuote := seedReportQuotation(t, database, 2, "PT. Buyer", "History ratio overflow", "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "Line", UoM: "Pc", Qty: 1,
			Cost: 100, Sell: poreport.MaxExactMinor,
		}})
	actualQuote := seedReportQuotation(t, database, 3, "PT. Buyer Two", "Safe actual", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	ratioPO := finalizeReportPO(t, database, ctx, user, ratioQuote, "PO-HIST-RATIO", "2025-01-10",
		PurchaseOrderBusinessCategoryFm, nil)
	actual := finalizeReportPO(t, database, ctx, user, actualQuote, "PO-HIST-RATIO-ACTUAL", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET discount_minor = subtotal_minor - 1, grand_total_minor = 1
		WHERE id = $1`, database.configSchema()), ratioPO.ID); err != nil {
		t.Fatalf("restore legacy whole-order discount: %v", err)
	}
	// The line net is an exact history value; only the coverage percentage
	// derived from amount=1 and costed=MaxExactMinor is out of range.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_items SET net_minor = $2 WHERE purchase_order_id = $1`,
		database.configSchema()), ratioPO.ID, poreport.MaxExactMinor); err != nil {
		t.Fatalf("raise historical line net: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-03-01", "2026-04-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != actual.SubtotalMinor {
		t.Fatalf("actual totals = %+v; want the safe March PO preserved", report.Totals)
	}
	if report.HistoryOutOfRange {
		t.Fatal("history out-of-range = true; the unused ratio must not invalidate exact history")
	}
	var january poreport.HistoryMonth
	found := false
	for _, month := range report.History {
		if month.MonthStart == "2025-01-01" {
			january = month
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("history = %+v; want the January 2025 observation", report.History)
	}
	if january.AmountMinor != 1 || january.CostedMinor != poreport.MaxExactMinor ||
		january.CapitalMinor != 100 || january.UncostedMinor != 1-poreport.MaxExactMinor {
		t.Errorf("January history metrics = %+v; want exact amount/cost/capital with no truncation", january.Metrics)
	}
	if january.MarginBps != nil || january.CoverageBps != nil {
		t.Errorf("January history ratios = %v/%v; want the unused ratios not computed",
			january.MarginBps, january.CoverageBps)
	}
}

func TestPurchaseOrderReportHistoryIsContinuousAndEndsAtTheCompleteMonth(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_history")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	januaryQuote := seedReportQuotation(t, database, 2, "PT. Buyer", "History Jan source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "History line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	marchQuote := seedReportQuotation(t, database, 2, "PT. Buyer", "History Mar source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "History line", UoM: "Pc", Qty: 1, Cost: 100, Sell: 200}})
	finalizeReportPO(t, database, ctx, user, januaryQuote, "PO-REPORT-HISTORY-JAN", "2026-01-10",
		PurchaseOrderBusinessCategoryFm, nil)
	finalizeReportPO(t, database, ctx, user, marchQuote, "PO-REPORT-HISTORY-MAR", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2026-05-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if len(report.History) != 4 {
		t.Fatalf("history months = %d; want Jan through the complete April", len(report.History))
	}
	if report.History[0].MonthStart != "2026-01-01" || report.History[0].PoCount != 1 {
		t.Errorf("history Jan = %+v; want the observed January", report.History[0])
	}
	if report.History[1].MonthStart != "2026-02-01" || report.History[1].PoCount != 0 {
		t.Errorf("history Feb = %+v; want a zero-filled month", report.History[1])
	}
	if report.History[2].MonthStart != "2026-03-01" || report.History[2].PoCount != 1 {
		t.Errorf("history Mar = %+v; want the observed March", report.History[2])
	}
	if report.History[2].AmountMinor != 200 || report.History[2].CapitalMinor != 100 {
		t.Errorf("history March metrics = %+v; want 200 amount and 100 capital", report.History[2].Metrics)
	}
	if report.History[3].MonthStart != "2026-04-01" || report.History[3].PoCount != 0 {
		t.Errorf("history Apr = %+v; want the complete empty April", report.History[3])
	}
}
