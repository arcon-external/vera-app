package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/poreport"
)

// Issue #641: the current month's readout over the authenticated endpoint. The
// domain arithmetic is pinned in backend/poreport; these tests prove the API
// wires the same snapshot, keeps actual-to-date independent of the forecast
// guard, and never lets a future-dated saved order enter the elapsed sample.

// Issue #646: the month readout's failure path is a typed refusal like every
// other report failure, not a bare {"error": ...} body. The mapping is exercised
// directly because reaching it through the endpoint needs a corrupt snapshot
// shape the public surface never produces.
func TestPurchaseOrderReportMonthSummaryFailureCarriesItsCode(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "internal shape failure",
			err:        fmt.Errorf("%w: current projection status %q", poreport.ErrInvalid, "mystery"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "report_month_summary_failed",
		},
		{
			name:       "value out of range",
			err:        fmt.Errorf("current month actual: %w", poreport.ErrValueOutOfRange),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "report_value_out_of_range",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			instance := echo.New()
			recorder := httptest.NewRecorder()
			context := instance.NewContext(
				httptest.NewRequest(http.MethodGet, "/", nil), recorder)
			if err := reportMonthSummaryError(context, test.err); err != nil {
				t.Fatalf("reportMonthSummaryError returned %v", err)
			}
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d; want %d", recorder.Code, test.wantStatus)
			}
			var body map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode refusal body: %v", err)
			}
			if body["code"] != test.wantCode {
				t.Errorf("code = %q; want %q", body["code"], test.wantCode)
			}
			if body["error"] == "" {
				t.Error("error = empty; want the human explanation")
			}
		})
	}
}

func TestPurchaseOrderReportMonthSummaryFootsTheDailyBuckets(t *testing.T) {
	today := apiReportToday()
	if today.Day() < 5 {
		t.Skip("the daily forecast needs five elapsed days")
	}
	server, token := testQuotationServer(t, "vera_api_po_report_month_summary_day")
	from, to := apiReportCurrentMonthRange(today)
	for day := 1; day <= 3; day++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-MONTH-SUMMARY-%d", day), apiReportDay(today, day), "FM")
	}

	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("day report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	summary := report.MonthSummary
	if summary == nil {
		t.Fatal("monthSummary = nil; want the current month readout")
	}
	if summary.Month != report.AsOf[:7] || summary.Mode != poreport.ProjectionModeDay {
		t.Errorf("month/mode = %s/%s; want %s/day", summary.Month, summary.Mode, report.AsOf[:7])
	}
	if summary.Actual.PoCount != 3 {
		t.Errorf("actual poCount = %d; want the three elapsed POs", summary.Actual.PoCount)
	}
	var actualCount, actualAmount int64
	for _, period := range report.Periods {
		if period.PeriodStart <= report.AsOf {
			actualCount += period.PoCount
			actualAmount += period.AmountMinor
		}
	}
	if summary.Actual.PoCount != actualCount || summary.Actual.AmountMinor != actualAmount {
		t.Errorf("actual = %+v; want the elapsed period rows %d/%d",
			summary.Actual, actualCount, actualAmount)
	}
	if summary.RecordedFuture != nil {
		t.Errorf("recordedFuture = %+v; want omitted with no future-dated row", summary.RecordedFuture)
	}
	if summary.Projection.Status != poreport.ProjectionStatusAvailable ||
		summary.Projection.Remainder == nil {
		t.Fatalf("projection = %+v; want an available remainder", summary.Projection)
	}
	var low, base, high, capital, margin int64
	for _, bucket := range report.CurrentProjection.Buckets {
		low += bucket.AmountLowMinor
		base += bucket.AmountBaseMinor
		high += bucket.AmountHighMinor
		capital += bucket.CapitalMinor
		margin += bucket.MarginMinor
	}
	remainder := summary.Projection.Remainder
	if remainder.LowMinor != low || remainder.BaseMinor != base || remainder.HighMinor != high ||
		remainder.CapitalMinor != capital || remainder.MarginMinor != margin {
		t.Errorf("remainder = %+v; want the exact sum of the daily buckets", remainder)
	}
}

func TestPurchaseOrderReportMonthSummarySeparatesAFutureDatedSavedPO(t *testing.T) {
	today := apiReportToday()
	if today.Day() < 5 || today.Day()+1 > apiReportDaysInMonth(today) {
		t.Skip("needs five elapsed days and a future date in this month")
	}
	server, token := testQuotationServer(t, "vera_api_po_report_month_summary_future")
	from, to := apiReportCurrentMonthRange(today)
	for day := 1; day <= 3; day++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-MONTH-FUTURE-%d", day), apiReportDay(today, day), "FM")
	}
	future := finalizeOneReportPO(t, server, token, "PO-REPORT-MONTH-FUTURE-4",
		apiReportDay(today, today.Day()+1), "FM")

	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("day report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)

	// The raw report includes the future-dated saved order: the acceptance
	// preserves that total/date policy and this ticket does not migrate it.
	if report.Totals.PoCount != 4 {
		t.Errorf("totals poCount = %d; want the future-dated saved order counted", report.Totals.PoCount)
	}
	var futurePeriod *poreport.Period
	for index := range report.Periods {
		if report.Periods[index].PeriodStart == apiReportDay(today, today.Day()+1) {
			futurePeriod = &report.Periods[index]
			break
		}
	}
	if futurePeriod == nil || futurePeriod.AmountMinor != future.SubtotalMinor {
		t.Fatalf("no future-dated period row carrying the saved PO amount %d", future.SubtotalMinor)
	}

	summary := report.MonthSummary
	if summary == nil {
		t.Fatal("monthSummary = nil; want the current month readout")
	}
	if summary.Actual.PoCount != 3 {
		t.Errorf("actual poCount = %d; future-dated evidence leaked into actual-to-date", summary.Actual.PoCount)
	}
	if summary.RecordedFuture == nil {
		t.Fatal("recordedFuture = nil; want the future-dated saved order as separate evidence")
	}
	if summary.RecordedFuture.PoCount != 1 ||
		summary.RecordedFuture.AmountMinor != future.SubtotalMinor {
		t.Errorf("recordedFuture = %+v; want the saved future-dated order", summary.RecordedFuture)
	}
	if summary.Actual.AmountMinor+summary.RecordedFuture.AmountMinor != report.Totals.AmountMinor {
		t.Errorf("actual + recorded future = %d; want the report total %d",
			summary.Actual.AmountMinor+summary.RecordedFuture.AmountMinor, report.Totals.AmountMinor)
	}
	if report.CurrentProjection.Basis.PoCount != 3 {
		t.Errorf("projection basis poCount = %d; want only the three elapsed POs",
			report.CurrentProjection.Basis.PoCount)
	}
}

func TestPurchaseOrderReportMonthSummaryKeepsActualWhenTheYearGuardFails(t *testing.T) {
	today := apiReportToday()
	if today.Day() < 5 {
		t.Skip("the daily sample needs five elapsed days")
	}
	server, token := testQuotationServer(t, "vera_api_po_report_month_summary_year")

	// One complete month with two POs is short of the three-month guard, while
	// the current month's elapsed sample has three POs and enough days. The day
	// view is forecastable; the year view is not. Its actual month-to-date must
	// still be present.
	historyMonth := apiReportMonthOffset(today, -1)
	for index := 1; index <= 2; index++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-MONTH-YEAR-HISTORY-%d", index),
			historyMonth.Format(time.DateOnly), "FM")
	}
	for day := 1; day <= 3; day++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-MONTH-YEAR-CURRENT-%d", day), apiReportDay(today, day), "FM")
	}

	yearFrom := time.Date(today.Year(), time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	yearTo := time.Date(today.Year()+1, time.January, 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(yearFrom, yearTo, "month", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("year report = %d (%v); want 200", code, body)
	}
	yearReport := decodeReport(t, body)
	if yearReport.CurrentProjection.Status != poreport.ProjectionStatusUnavailable ||
		yearReport.CurrentProjection.Reason != poreport.ReasonInsufficientMonths {
		t.Fatalf("year projection = %s/%s; want unavailable/%s",
			yearReport.CurrentProjection.Status, yearReport.CurrentProjection.Reason,
			poreport.ReasonInsufficientMonths)
	}
	yearSummary := yearReport.MonthSummary
	if yearSummary == nil {
		t.Fatal("year monthSummary = nil; actual-to-date must survive the failed year guard")
	}
	if yearSummary.Actual.PoCount != 3 || yearSummary.Actual.AmountMinor == 0 {
		t.Errorf("year monthSummary actual = %+v; want the three current-month POs", yearSummary.Actual)
	}
	if yearSummary.Projection.Status != poreport.ProjectionStatusUnavailable ||
		yearSummary.Projection.Reason != poreport.ReasonInsufficientMonths ||
		yearSummary.Projection.Remainder != nil {
		t.Errorf("year monthSummary projection = %+v; want unavailable/%s with no remainder",
			yearSummary.Projection, poreport.ReasonInsufficientMonths)
	}

	// The day view for the same current month is available, so the current-month
	// table must take the daily summary rather than the annual source's
	// availability.
	from, to := apiReportCurrentMonthRange(today)
	code, body = doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams(from, to, "day", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("day report = %d (%v); want 200", code, body)
	}
	daySummary := decodeReport(t, body).MonthSummary
	if daySummary == nil || daySummary.Projection.Status != poreport.ProjectionStatusAvailable ||
		daySummary.Projection.Remainder == nil {
		t.Fatalf("day monthSummary = %+v; want an available remainder", daySummary)
	}
	if daySummary.Actual.PoCount != yearSummary.Actual.PoCount ||
		daySummary.Actual.AmountMinor != yearSummary.Actual.AmountMinor {
		t.Errorf("day actual = %+v; want the same month-to-date figures as the year readout %+v",
			daySummary.Actual, yearSummary.Actual)
	}
}
