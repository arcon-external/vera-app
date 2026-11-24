package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// Issue #648: the report's population counters. #659's post-backfill check
// reads these instead of inferring the population's health from an uncosted
// total, so each one is pinned at the store seam: the three production records
// that carried no allocation are exactly three unallocated and three legacy
// fallbacks until the audited correction, and zero after it. The counters are
// population facts, not range figures: a saved record outside the selected
// range must still be visible to the check.

func TestPurchaseOrderReportPopulationCountsUnallocatedAndLegacyFallback(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_population_unallocated")
	ctx := poContext(t, database)
	user := poTestUser(t, database)

	// The three production shapes: revenue, no line reference, no allocation.
	legacyIDs := make([]int64, 0, 3)
	for _, fixture := range productionAllocationFixtures() {
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
		legacyIDs = append(legacyIDs, saved.ID)
	}

	// A current save owns its allocation and is not a legacy fallback. A
	// deliberate extra line has no quotation reference either, but it is not a
	// legacy line: it must never be reported as one.
	current := seedReportQuotation(t, database, 2, "PT. Population Current",
		"Current source", "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "Current line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
		}})
	finalizeReportPO(t, database, ctx, user, current, "PO-POP-CURRENT", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)
	withExtra := seedReportQuotation(t, database, 3, "PT. Population Extra",
		"Extra source", "IDR", "FTZ",
		[]reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
		}})
	finalizeReportPO(t, database, ctx, user, withExtra, "PO-POP-EXTRA", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.UnallocatedPoCount != 3 {
		t.Errorf("unallocated = %d; want the three pre-correction production records",
			report.UnallocatedPoCount)
	}
	if report.LegacyFallbackPoCount != 3 {
		t.Errorf("legacy fallback = %d; want the three nil-reference production lines",
			report.LegacyFallbackPoCount)
	}

	// The counters describe the population, not the selected range: a range
	// that contains no purchase order still reports the same health figures,
	// because an unallocated record outside it is still unallocated.
	narrow := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2026-01-02", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if narrow.Totals.PoCount != 0 {
		t.Fatalf("narrow range poCount = %d; want 0", narrow.Totals.PoCount)
	}
	if narrow.UnallocatedPoCount != 3 || narrow.LegacyFallbackPoCount != 3 {
		t.Errorf("narrow-range population = %d/%d; want 3/3 regardless of the range",
			narrow.UnallocatedPoCount, narrow.LegacyFallbackPoCount)
	}

	// Currency is the partition: a USD report never counts the IDR records.
	usd := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "USD"))
	if usd.UnallocatedPoCount != 0 || usd.LegacyFallbackPoCount != 0 {
		t.Errorf("USD population = %d/%d; want 0/0 (the records are IDR)",
			usd.UnallocatedPoCount, usd.LegacyFallbackPoCount)
	}

	// #659's audited correction: each legacy line is linked back to its
	// quotation leaf through the normal correction path, which rebuilds the
	// allocation. Both counters must fall to zero.
	for index, orderID := range legacyIDs {
		order, err := database.GetPurchaseOrder(ctx, orderID)
		if err != nil {
			t.Fatalf("read legacy record %d: %v", index, err)
		}
		correction := correctionFromSavedOrder(order, user,
			"Backfill allocation for line 1 to Quotation item 1")
		itemNo := 1
		correction.Items[0].QuotationItemNo = &itemNo
		correction.Items[0].QuotationSubNo = nil
		if _, err := database.CorrectPurchaseOrder(ctx, correction); err != nil {
			t.Fatalf("backfill legacy record %d: %v", index, err)
		}
	}

	report = mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.UnallocatedPoCount != 0 || report.LegacyFallbackPoCount != 0 {
		t.Errorf("after the audited correction population = %d/%d; want 0/0",
			report.UnallocatedPoCount, report.LegacyFallbackPoCount)
	}
}

func TestPurchaseOrderReportPopulationCountsExpiredUnclosedQuotations(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_population_expired")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)

	expiryQuotation := func(code int, name, subject, currency string) QuotationRecord {
		t.Helper()
		return seedReportQuotation(t, database, code, name, subject, currency, "FTZ",
			[]reportQuotationLine{{
				Description: "Expiry line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
			}})
	}

	// The report's as-of day is 2026-11-19. A 30-day offer issued on the WIB
	// day 2026-10-20 lapses on 2026-11-19; the last valid day itself is still
	// valid, so the boundary quotation is not counted.
	boundary := expiryQuotation(2, "PT. Expiry Boundary", "Expiry boundary", "IDR")
	setQuotationIssueWindow(t, database, boundary.ID,
		time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC), 30)
	valid := expiryQuotation(3, "PT. Expiry Valid", "Expiry valid", "IDR")
	setQuotationIssueWindow(t, database, valid.ID,
		time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC), 30)
	expired := expiryQuotation(4, "PT. Expiry Expired", "Expiry expired", "IDR")
	setQuotationIssueWindow(t, database, expired.ID,
		time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC), 30)
	closed := expiryQuotation(5, "PT. Expiry Closed", "Expiry closed", "IDR")
	setQuotationIssueWindow(t, database, closed.ID,
		time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC), 30)
	if _, err := database.CloseQuotation(ctx, closed.ID, "customer cancelled the remainder", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	// Another currency's lapsed offer is its own partition.
	usd := expiryQuotation(6, "PT. Expiry USD", "Expiry USD", "USD")
	setQuotationIssueWindow(t, database, usd.ID,
		time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC), 30)

	idr := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if idr.ExpiredUnclosedQuotationCount != 1 {
		t.Errorf("IDR expired-unclosed = %d; want only the lapsed open quotation",
			idr.ExpiredUnclosedQuotationCount)
	}
	usdReport := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "USD"))
	if usdReport.ExpiredUnclosedQuotationCount != 1 {
		t.Errorf("USD expired-unclosed = %d; want its own lapsed quotation",
			usdReport.ExpiredUnclosedQuotationCount)
	}

	// The next day the boundary quotation lapses too. The counter follows the
	// same clock the rest of the report uses, so the boundary moves once.
	nextDay, err := database.ReportPurchaseOrders(ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"),
		reportNow().AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("ReportPurchaseOrders one day later: %v", err)
	}
	if nextDay.ExpiredUnclosedQuotationCount != 2 {
		t.Errorf("expired-unclosed one day later = %d; want the boundary quotation included",
			nextDay.ExpiredUnclosedQuotationCount)
	}
}

// TestPurchaseOrderReportPopulationWireNames pins the Go JSON contract the
// report screen reads, because the population counters have no other consumer
// yet. A renamed tag is a silent zero on the screen, so the exact names are
// asserted here rather than left to review.
func TestPurchaseOrderReportPopulationWireNames(t *testing.T) {
	population, err := json.Marshal(poreport.Population{
		UnallocatedPoCount:            3,
		LegacyFallbackPoCount:         2,
		ExpiredUnclosedQuotationCount: 1,
	})
	if err != nil {
		t.Fatalf("marshal population: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(population, &fields); err != nil {
		t.Fatalf("unmarshal population: %v", err)
	}
	for name, want := range map[string]float64{
		"unallocatedPoCount":            3,
		"legacyFallbackPoCount":         2,
		"expiredUnclosedQuotationCount": 1,
	} {
		if got := fields[name]; got != want {
			t.Errorf("population JSON %q = %v; want %v", name, got, want)
		}
	}

	pipeline, err := json.Marshal(poreport.PipelineSummary{FulfillmentBps: 6000})
	if err != nil {
		t.Fatalf("marshal pipeline: %v", err)
	}
	if err := json.Unmarshal(pipeline, &fields); err != nil {
		t.Fatalf("unmarshal pipeline: %v", err)
	}
	if got := fields["fulfillmentBps"]; got != float64(6000) {
		t.Errorf("pipeline JSON %q = %v; want 6000", "fulfillmentBps", got)
	}
}

// setQuotationIssueWindow pins a quotation's latest revision issue instant and
// validity so a WIB expiry boundary can be exercised without waiting for it.
// The instant is a real UTC time whose WIB calendar day is one day later, which
// is how a document issued at 03:00 in Jakarta is dated.
func setQuotationIssueWindow(
	t *testing.T, database *Store, quotationID int64, issuedAt time.Time, validityDays int16,
) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(`
		UPDATE %s.quotation_revisions
		SET created_at = $2, validity_days = $3
		WHERE quotation_id = $1`, database.configSchema()),
		quotationID, issuedAt, validityDays); err != nil {
		t.Fatalf("set quotation %d issue window: %v", quotationID, err)
	}
}
