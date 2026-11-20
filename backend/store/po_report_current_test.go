package store

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// Issue #629's day bucket and the elapsed-day sample behind the current-period
// projection. The store returns one continuous calendar day per requested day
// and only the elapsed days of the current month as daily history; it never
// invents a forecast of its own.

func TestPurchaseOrderReportDayBucketAndDailyHistory(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_day_bucket")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	first := seedReportQuotation(t, database, 2, "PT. Buyer", "Day bucket one", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Day line one", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250}})
	second := seedReportQuotation(t, database, 3, "PT. Buyer Two", "Day bucket two", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Day line two", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250}})
	finalizeReportPO(t, database, ctx, user, first, "PO-REPORT-DAY-1", "2026-11-02",
		PurchaseOrderBusinessCategoryFm, nil)
	finalizeReportPO(t, database, ctx, user, second, "PO-REPORT-DAY-2", "2026-11-04",
		PurchaseOrderBusinessCategoryFm, nil)

	report := mustReport(t, database, ctx,
		reportQuery("2026-11-01", "2026-12-01", poreport.BucketDay, poreport.CategoryAll, "IDR"))
	if len(report.Periods) != 30 {
		t.Fatalf("day periods = %d; want the 30 days of November", len(report.Periods))
	}
	if first := report.Periods[0]; first.PeriodStart != "2026-11-01" {
		t.Errorf("first day = %s; want 2026-11-01", first.PeriodStart)
	}
	if last := report.Periods[len(report.Periods)-1]; last.PeriodStart != "2026-11-30" {
		t.Errorf("last day = %s; want 2026-11-30", last.PeriodStart)
	}
	dayTwo := reportPeriod(t, report, "2026-11-02")
	if dayTwo.PoCount != 1 || dayTwo.AmountMinor != 500 || dayTwo.CapitalMinor != 200 {
		t.Errorf("2 November = %+v; want one PO at 500/200", dayTwo.Metrics)
	}
	if dayThree := reportPeriod(t, report, "2026-11-03"); dayThree.PoCount != 0 || dayThree.AmountMinor != 0 {
		t.Errorf("3 November = %+v; want the zero gap day", dayThree.Metrics)
	}
	if dayFour := reportPeriod(t, report, "2026-11-04"); dayFour.PoCount != 1 || dayFour.AmountMinor != 500 {
		t.Errorf("4 November = %+v; want one PO at 500", dayFour.Metrics)
	}
	if today := reportPeriod(t, report, "2026-11-19"); !today.InProgress {
		t.Errorf("19 November inProgress = false; want the as-of day marked")
	}
	if before := reportPeriod(t, report, "2026-11-18"); before.InProgress {
		t.Errorf("18 November inProgress = true; want only the as-of day marked")
	}

	if len(report.Daily) != 2 {
		t.Fatalf("daily history = %d rows; want the two observed days", len(report.Daily))
	}
	if report.Daily[0].Date != "2026-11-02" || report.Daily[1].Date != "2026-11-04" {
		t.Errorf("daily dates = %s, %s; want 2026-11-02, 2026-11-04",
			report.Daily[0].Date, report.Daily[1].Date)
	}
	for _, day := range report.Daily {
		if day.PoCount != 1 || day.AmountMinor != 500 || day.CostedMinor != 500 ||
			day.UncostedMinor != 0 || day.CapitalMinor != 200 {
			t.Errorf("daily %s = %+v; want one fully costed PO at 500/200", day.Date, day.Metrics)
		}
	}
	if report.HistoryOutOfRange || report.DailyOutOfRange {
		t.Errorf("report marked an exact sample out of range: history=%t daily=%t",
			report.HistoryOutOfRange, report.DailyOutOfRange)
	}
}

func TestPurchaseOrderReportDayRangeMustStayInOneCalendarMonth(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_day_range")
	ctx := poContext(t, database)

	_, err := database.ReportPurchaseOrders(ctx,
		reportQuery("2026-11-15", "2026-12-02", poreport.BucketDay, poreport.CategoryAll, "IDR"),
		reportNow())
	if !errors.Is(err, poreport.ErrInvalid) {
		t.Fatalf("day range across a month boundary error = %v; want ErrInvalid", err)
	}
}

func TestPurchaseOrderReportDailyHistoryRecomputesAsPOsArrive(t *testing.T) {
	database := openTestSchema(t, "vera_po_report_day_recompute")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	first := seedReportQuotation(t, database, 2, "PT. Buyer", "First day PO", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "First line", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250}})
	finalizeReportPO(t, database, ctx, user, first, "PO-REPORT-DAY-RECOMPUTE-1", "2026-11-02",
		PurchaseOrderBusinessCategoryFm, nil)

	query := reportQuery("2026-11-01", "2026-12-01", poreport.BucketDay, poreport.CategoryAll, "IDR")
	before := mustReport(t, database, ctx, query)
	if len(before.Daily) != 1 || before.Daily[0].AmountMinor != 500 {
		t.Fatalf("before daily = %+v; want one 500 day", before.Daily)
	}

	// A second, larger PO on a later day must change both the observed-day
	// count and the sample total; this is the input the projection recomputes
	// from, and the store never caches it.
	second := seedReportQuotation(t, database, 3, "PT. Buyer Two", "Second day PO", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Second line", UoM: "Pc", Qty: 4, Cost: 100, Sell: 250}})
	finalizeReportPO(t, database, ctx, user, second, "PO-REPORT-DAY-RECOMPUTE-2", "2026-11-03",
		PurchaseOrderBusinessCategoryFm, nil)

	after := mustReport(t, database, ctx, query)
	if len(after.Daily) != 2 {
		t.Fatalf("after daily = %d rows; want two observed days", len(after.Daily))
	}
	if after.Daily[0].AmountMinor != 500 || after.Daily[1].AmountMinor != 1000 {
		t.Errorf("after daily amounts = %d, %d; want 500, 1000",
			after.Daily[0].AmountMinor, after.Daily[1].AmountMinor)
	}
	if after.Totals.PoCount != 2 || after.Totals.AmountMinor != 1500 {
		t.Errorf("after totals = %+v; want two POs at 1500", after.Totals)
	}
}

func TestPurchaseOrderReportDailyRefusesAnOutOfRangeAggregate(t *testing.T) {
	raw := []byte(fmt.Sprintf(
		`[{"date":"2026-11-02","poCount":1,"amountMinor":%d,"costedMinor":0,"capitalMinor":0}]`,
		poreport.MaxExactMinor+1))
	sample, err := reportDaily(raw, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("reportDaily() error = %v", err)
	}
	if !sample.outOfRange {
		t.Fatal("daily sample with an inexact aggregate = in range; want it withheld")
	}
	if len(sample.days) != 0 {
		t.Errorf("daily sample kept %d days; want none from a truncated aggregate", len(sample.days))
	}
}
