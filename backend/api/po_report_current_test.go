package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// Issue #629's current-period projection over the authenticated endpoint. The
// tests use the real server clock in WIB, because the contract is "exactly the
// current month or year"; dates are derived from today rather than hard-coded.
// The exact base and band arithmetic is pinned in the poreport tests. These
// tests pin the wiring: the right mode for the range, the duration guards, the
// current-month remainder, recomputation when a PO arrives, incomplete cost,
// and the rule that no forecast enters an actual total.

func apiReportToday() time.Time {
	now := time.Now().In(time.FixedZone("WIB", 7*60*60))
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func apiReportMonthStart(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func apiReportDaysInMonth(date time.Time) int {
	return time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func apiReportMonthOffset(date time.Time, months int) time.Time {
	return time.Date(date.Year(), date.Month()+time.Month(months), 10, 0, 0, 0, 0, time.UTC)
}

func apiReportDay(date time.Time, day int) string {
	return time.Date(date.Year(), date.Month(), day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

func apiReportCurrentMonthRange(today time.Time) (string, string) {
	start := apiReportMonthStart(today)
	return start.Format(time.DateOnly), start.AddDate(0, 1, 0).Format(time.DateOnly)
}

func TestPurchaseOrderReportDayBucketHasContinuousDays(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_day_bucket")
	today := apiReportToday()
	from, to := apiReportCurrentMonthRange(today)
	finalizeOneReportPO(t, server, token, "PO-REPORT-API-DAY-1", apiReportDay(today, 1), "FM")

	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("day report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	if report.Filter.Bucket != "day" {
		t.Errorf("filter bucket = %s; want day", report.Filter.Bucket)
	}
	if len(report.Periods) != apiReportDaysInMonth(today) {
		t.Fatalf("day periods = %d; want the %d days of the current month",
			len(report.Periods), apiReportDaysInMonth(today))
	}
	if report.Periods[0].PeriodStart != from {
		t.Errorf("first day = %s; want %s", report.Periods[0].PeriodStart, from)
	}
	if report.CurrentProjection.Mode != poreport.ProjectionModeDay {
		t.Errorf("current projection mode = %s; want day", report.CurrentProjection.Mode)
	}
	if report.CurrentProjection.Buckets == nil {
		t.Error("current projection buckets = null; want an empty array or the remaining days")
	}
}

func TestPurchaseOrderReportProjectsCurrentMonthAndRecomputes(t *testing.T) {
	today := apiReportToday()
	if today.Day() < 5 {
		t.Skip("the current-month projection needs five elapsed days")
	}
	server, token := testQuotationServer(t, "vera_api_po_report_current_day")
	from, to := apiReportCurrentMonthRange(today)
	for day := 1; day <= 3; day++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-API-CURRENT-%d", day), apiReportDay(today, day), "FM")
	}

	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("current day report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	current := report.CurrentProjection
	if current.Mode != poreport.ProjectionModeDay || current.Status != poreport.ProjectionStatusAvailable {
		t.Fatalf("current projection = %s/%s/%s; want day/available",
			current.Mode, current.Status, current.Reason)
	}
	if report.AsOf != today.Format(time.DateOnly) {
		t.Errorf("asOf = %s; want WIB today %s", report.AsOf, today.Format(time.DateOnly))
	}
	if current.Basis.AsOf != today.Format(time.DateOnly) {
		t.Errorf("projection asOf = %s; want WIB today", current.Basis.AsOf)
	}
	if current.Basis.Units != today.Day() || current.Basis.PoCount != 3 {
		t.Errorf("projection basis = %+v; want %d elapsed days and 3 POs", current.Basis, today.Day())
	}
	if len(current.Buckets) != apiReportDaysInMonth(today)-today.Day() {
		t.Errorf("projection buckets = %d; want the remaining days of the month", len(current.Buckets))
	}
	if len(current.Buckets) > 0 {
		wantStart := today.AddDate(0, 0, 1).Format(time.DateOnly)
		if current.Buckets[0].PeriodStart != wantStart {
			t.Errorf("first projected day = %s; want %s", current.Buckets[0].PeriodStart, wantStart)
		}
		if current.Buckets[0].AmountLowMinor > current.Buckets[0].AmountBaseMinor ||
			current.Buckets[0].AmountBaseMinor > current.Buckets[0].AmountHighMinor {
			t.Errorf("projected band is unordered: %+v", current.Buckets[0])
		}
	}
	if current.CurrentMonth != nil {
		t.Errorf("day mode currentMonth = %+v; want nil", current.CurrentMonth)
	}

	// The forecast is not actual evidence: the actual totals still count only
	// the POs in the report and foot across the day rows.
	periodCount := int64(0)
	periodAmount := int64(0)
	for _, period := range report.Periods {
		periodCount += period.PoCount
		periodAmount += period.AmountMinor
	}
	if report.Totals.PoCount != 3 || periodCount != report.Totals.PoCount ||
		periodAmount != report.Totals.AmountMinor {
		t.Errorf("totals/periods = %+v / %d/%d; want three actual POs footing",
			report.Totals, periodCount, periodAmount)
	}

	// A new PO changes the elapsed sample; the next read recomputes rather
	// than caching the previous forecast.
	if len(current.Buckets) == 0 {
		t.Skip("the current month has no remaining days to project")
	}
	beforeBase := current.Buckets[0].AmountBaseMinor
	finalizeOneReportPO(t, server, token, "PO-REPORT-API-CURRENT-4", apiReportDay(today, 4), "FM")
	code, body = doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("recomputed day report = %d (%v); want 200", code, body)
	}
	after := decodeReport(t, body).CurrentProjection
	if after.Basis.PoCount != 4 {
		t.Errorf("recomputed basis poCount = %d; want 4", after.Basis.PoCount)
	}
	if after.Buckets[0].AmountBaseMinor == beforeBase {
		t.Errorf("recomputed base = %d; want it to move with a fourth PO", beforeBase)
	}
}

func TestPurchaseOrderReportProjectsCurrentYearWithMonthRemainder(t *testing.T) {
	today := apiReportToday()
	if today.Day() < 5 {
		t.Skip("the current-month remainder needs five elapsed days")
	}
	server, token := testQuotationServer(t, "vera_api_po_report_current_month")
	today = apiReportToday()
	for months := 1; months <= 3; months++ {
		for index := 1; index <= 2; index++ {
			date := apiReportMonthOffset(today, -months)
			finalizeOneReportPO(t, server, token,
				fmt.Sprintf("PO-REPORT-API-HISTORY-%d-%d", months, index),
				date.Format(time.DateOnly), "FM")
		}
	}
	for day := 1; day <= 3; day++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-API-CURRENT-MONTH-%d", day), apiReportDay(today, day), "FM")
	}

	from := time.Date(today.Year(), time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	to := time.Date(today.Year()+1, time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "month", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("current year report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	current := report.CurrentProjection
	if current.Mode != poreport.ProjectionModeMonth || current.Status != poreport.ProjectionStatusAvailable {
		t.Fatalf("current projection = %s/%s/%s; want month/available",
			current.Mode, current.Status, current.Reason)
	}
	if current.Basis.Units != 3 || current.Basis.PoCount != 6 {
		t.Errorf("basis = %+v; want three complete months and six POs", current.Basis)
	}
	if len(current.Buckets) != 12-int(today.Month()) {
		t.Errorf("projection buckets = %d; want the months after %s",
			len(current.Buckets), today.Month())
	}
	if current.CurrentMonth == nil {
		t.Fatal("month mode has no currentMonth")
	}
	if current.CurrentMonth.Actual.PoCount != 3 {
		t.Errorf("currentMonth actual poCount = %d; want 3", current.CurrentMonth.Actual.PoCount)
	}
	if current.CurrentMonth.Remainder.SampleSize != today.Day() ||
		current.CurrentMonth.Remainder.Source != poreport.SourceElapsedDays {
		t.Errorf("currentMonth remainder = %+v; want %d elapsed days from %s",
			current.CurrentMonth.Remainder, today.Day(), poreport.SourceElapsedDays)
	}
	if current.CurrentMonth.Remainder.LowMinor > current.CurrentMonth.Remainder.BaseMinor ||
		current.CurrentMonth.Remainder.BaseMinor > current.CurrentMonth.Remainder.HighMinor {
		t.Errorf("remainder band is unordered: %+v", current.CurrentMonth.Remainder)
	}

	// The forecast, including the remainder, never enters the actual totals.
	periodCount := int64(0)
	periodAmount := int64(0)
	for _, period := range report.Periods {
		periodCount += period.PoCount
		periodAmount += period.AmountMinor
	}
	if periodCount != report.Totals.PoCount || periodAmount != report.Totals.AmountMinor {
		t.Errorf("periods do not foot to totals: %d/%d vs %+v",
			periodCount, periodAmount, report.Totals)
	}
	if report.Totals.PoCount == current.Basis.PoCount+current.CurrentMonth.Actual.PoCount &&
		report.Totals.PoCount != periodCount {
		t.Errorf("projection sample was added to the actual total: %+v", report.Totals)
	}
}

func TestPurchaseOrderReportCurrentProjectionThresholds(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_current_threshold")
	today := apiReportToday()
	from, to := apiReportCurrentMonthRange(today)
	for index := 1; index <= 2; index++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-API-THRESHOLD-%d", index), apiReportDay(today, 1), "FM")
	}

	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("threshold report = %d (%v); want 200", code, body)
	}
	current := decodeReport(t, body).CurrentProjection
	if current.Mode != poreport.ProjectionModeDay ||
		current.Status != poreport.ProjectionStatusUnavailable ||
		current.Reason != poreport.ReasonInsufficientDays {
		t.Fatalf("current projection = %s/%s/%s; want day/unavailable/%s",
			current.Mode, current.Status, current.Reason, poreport.ReasonInsufficientDays)
	}
	if len(current.Buckets) != 0 {
		t.Errorf("threshold failure carries %d buckets; want none", len(current.Buckets))
	}
}

func TestPurchaseOrderReportCurrentProjectionNoData(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_current_no_data")
	today := apiReportToday()
	from, to := apiReportCurrentMonthRange(today)

	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("empty day report = %d (%v); want 200", code, body)
	}
	day := decodeReport(t, body).CurrentProjection
	if day.Mode != poreport.ProjectionModeDay || day.Reason != poreport.ReasonNoData {
		t.Errorf("empty day projection = %s/%s; want day/%s",
			day.Mode, day.Reason, poreport.ReasonNoData)
	}
	if len(day.Buckets) != 0 {
		t.Errorf("empty day projection carries %d buckets; want none", len(day.Buckets))
	}

	yearFrom := time.Date(today.Year(), time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	yearTo := time.Date(today.Year()+1, time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	code, body = doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(yearFrom, yearTo, "month", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("empty year report = %d (%v); want 200", code, body)
	}
	month := decodeReport(t, body).CurrentProjection
	if month.Mode != poreport.ProjectionModeMonth || month.Reason != poreport.ReasonNoData {
		t.Errorf("empty year projection = %s/%s; want month/%s",
			month.Mode, month.Reason, poreport.ReasonNoData)
	}
	if month.CurrentMonth != nil {
		t.Errorf("empty year projection carries currentMonth: %+v", month.CurrentMonth)
	}
}

func TestPurchaseOrderReportHistoricalRangeHasNoCurrentProjection(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_current_historical")
	finalizeOneReportPO(t, server, token, "PO-REPORT-API-HISTORICAL", "2020-06-10", "FM")

	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams("2020-01-01", "2021-01-01", "year", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("historical report = %d (%v); want 200", code, body)
	}
	current := decodeReport(t, body).CurrentProjection
	if current.Mode != poreport.ProjectionModeNone ||
		current.Status != poreport.ProjectionStatusUnavailable ||
		current.Reason != poreport.ReasonNotCurrentPeriod {
		t.Fatalf("historical current projection = %s/%s/%s; want none/unavailable/%s",
			current.Mode, current.Status, current.Reason, poreport.ReasonNotCurrentPeriod)
	}
	if len(current.Buckets) != 0 || current.CurrentMonth != nil {
		t.Errorf("historical range carries a forecast: %+v", current)
	}
}

func TestPurchaseOrderReportIncompleteCostSuppressesCurrentProjection(t *testing.T) {
	today := apiReportToday()
	if today.Day() < 5 {
		t.Skip("the current-month remainder needs five elapsed days")
	}
	const schema = "vera_api_po_report_current_incomplete"
	server, token := testQuotationServer(t, schema)
	for months := 1; months <= 3; months++ {
		for index := 1; index <= 2; index++ {
			date := apiReportMonthOffset(today, -months)
			finalizeOneReportPO(t, server, token,
				fmt.Sprintf("PO-REPORT-API-INCOMPLETE-HISTORY-%d-%d", months, index),
				date.Format(time.DateOnly), "FM")
		}
	}
	var saved purchaseOrderRow
	for day := 1; day <= 3; day++ {
		po := finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-API-INCOMPLETE-CURRENT-%d", day), apiReportDay(today, day), "FM")
		if day == 1 {
			saved = po
		}
	}

	// Point the PO at a revision that does not exist. The amount stays, the
	// cost no longer resolves, and the row keeps its valid source shape, so
	// this isolates incomplete cost rather than the deleted-source path.
	pool := apiReportPool(t, schema)
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(
		`UPDATE %s.purchase_orders SET source_revision = 99 WHERE id = $1`, testSchemaIdent()), saved.ID); err != nil {
		t.Fatalf("detach the source revision: %v", err)
	}

	from := time.Date(today.Year(), time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	to := time.Date(today.Year()+1, time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "month", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("incomplete-cost report = %d (%v); want 200", code, body)
	}
	current := decodeReport(t, body).CurrentProjection
	if current.Mode != poreport.ProjectionModeMonth ||
		current.Status != poreport.ProjectionStatusUnavailable ||
		current.Reason != poreport.ReasonIncompleteCostBasis {
		t.Fatalf("current projection = %s/%s/%s; want month/unavailable/%s",
			current.Mode, current.Status, current.Reason, poreport.ReasonIncompleteCostBasis)
	}
	if len(current.Buckets) != 0 || current.CurrentMonth != nil {
		t.Errorf("incomplete cost invented a margin-bearing forecast: %+v", current)
	}
}
