package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/apicontract"
	"vera-app/backend/poreport"
)

// Issue #629: the purchase-order report over the authenticated HTTP surface.
// These tests drive the real server and disposable Postgres through public
// behaviour: the report's shape and arithmetic footing, the facet defaults, the
// typed validation refusals, the session gate, and the two disclosures the API
// adds on top of the aggregate (legacy adjustment and overflow).

func reportURL(values url.Values) string {
	return apicontract.PurchaseOrderReportPath + "?" + values.Encode()
}

func reportQueryParams(from, to, bucket, category, currency string) url.Values {
	values := url.Values{}
	values.Set("from", from)
	values.Set("to", to)
	values.Set("bucket", bucket)
	if category != "" {
		values.Set("category", category)
	}
	if currency != "" {
		values.Set("currency", currency)
	}
	return values
}

func decodeReport(t *testing.T, body map[string]any) apicontract.PurchaseOrderReport {
	t.Helper()
	var report apicontract.PurchaseOrderReport
	encodeDecode(t, body, &report)
	return report
}

// apiReportPool opens the disposable database directly for the two fixtures
// that cannot be produced from the public API: a restored retired adjustment and
// an aggregate above the exact range.
func apiReportPool(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect for report fixture: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// finalizeOneReportPO drives a real export -> draft -> final save chain and
// returns the saved row so the report test knows its exact amount.
func finalizeOneReportPO(t *testing.T, server *Server, token, number, received, category string) purchaseOrderRow {
	t.Helper()
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, map[string]any{
		"receivedDate":     received,
		"businessCategory": category,
	})
	if code != http.StatusCreated {
		t.Fatalf("create report PO = %d (%v); want 201", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token,
		finalizeBody(draft, number))
	if code != http.StatusOK {
		t.Fatalf("finalize report PO = %d (%v); want 200", code, body)
	}
	return decodePurchaseOrder(t, body)
}

func TestPurchaseOrderReportValidatesEveryFilterTyped(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_validation")

	cases := []struct {
		name       string
		values     url.Values
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing bucket",
			values:     reportQueryParams("2026-01-01", "2027-01-01", "", "", "IDR"),
			wantStatus: http.StatusBadRequest, wantCode: "report_invalid",
		},
		{
			name:       "malformed from",
			values:     reportQueryParams("2026-1-01", "2027-01-01", "month", "", "IDR"),
			wantStatus: http.StatusBadRequest, wantCode: "report_invalid",
		},
		{
			name:       "reversed range",
			values:     reportQueryParams("2027-01-01", "2026-01-01", "month", "", "IDR"),
			wantStatus: http.StatusBadRequest, wantCode: "report_invalid",
		},
		{
			name:       "unknown bucket",
			values:     reportQueryParams("2026-01-01", "2027-01-01", "week", "", "IDR"),
			wantStatus: http.StatusBadRequest, wantCode: "report_invalid",
		},
		{
			name:       "unknown category",
			values:     reportQueryParams("2026-01-01", "2027-01-01", "month", "NOPE", "IDR"),
			wantStatus: http.StatusBadRequest, wantCode: "report_invalid",
		},
		{
			name:       "unknown currency",
			values:     reportQueryParams("2026-01-01", "2027-01-01", "month", "", "XYZ"),
			wantStatus: http.StatusBadRequest, wantCode: "report_invalid",
		},
		{
			name:       "121 month buckets",
			values:     reportQueryParams("2015-12-01", "2026-01-01", "month", "", "IDR"),
			wantStatus: http.StatusUnprocessableEntity, wantCode: "report_range_too_large",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			code, body := doJSON(t, server, http.MethodGet, reportURL(test.values), token, nil)
			if code != test.wantStatus {
				t.Fatalf("status = %d (%v); want %d", code, body, test.wantStatus)
			}
			if got, _ := body["code"].(string); got != test.wantCode {
				t.Errorf("code = %q; want %q", got, test.wantCode)
			}
		})
	}

	// The endpoint is authenticated like the PO archive: no session is a 401.
	code, _ := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams("2026-01-01", "2027-01-01", "month", "", "IDR")), "", nil)
	if code != http.StatusUnauthorized {
		t.Errorf("unauthenticated report = %d; want 401", code)
	}

	// A catalogue code is case-insensitive on input and canonical on output, so
	// "usd" and "USD" name the same partition in the filter selector.
	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams("2026-01-01", "2027-01-01", "month", "", "usd")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("lower-case currency = %d (%v); want 200", code, body)
	}
	if got := decodeReport(t, body).Filter.Currency; got != "USD" {
		t.Errorf("canonical currency = %q; want USD", got)
	}
}

func TestPurchaseOrderReportDefaultsCurrencyAndReturnsTheEmptyShape(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_empty")
	values := reportQueryParams("2026-01-01", "2027-01-01", "month", "", "")
	code, body := doJSON(t, server, http.MethodGet, reportURL(values), token, nil)
	if code != http.StatusOK {
		t.Fatalf("empty report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	if report.Filter.Currency != "IDR" {
		t.Errorf("default currency = %q; want the company IDR", report.Filter.Currency)
	}
	if report.Filter.Category != poreport.CategoryAll {
		t.Errorf("default category = %q; want all", report.Filter.Category)
	}
	if report.GeneratedAt == "" || report.AsOf == "" {
		t.Errorf("empty report lacks generatedAt/asOf: %+v", report)
	}
	if report.Company.Name == "" {
		t.Error("empty report has no company name")
	}
	if report.Totals.PoCount != 0 || report.Totals.AmountMinor != 0 {
		t.Errorf("empty totals = %+v; want zero", report.Totals)
	}
	if len(report.Periods) != 12 {
		t.Errorf("empty periods = %d; want a continuous 12-month axis", len(report.Periods))
	}
	if len(report.Categories) != 5 {
		t.Errorf("empty categories = %d; want the fixed five rows", len(report.Categories))
	}
	if len(report.CategorySeries) != 0 {
		t.Errorf("empty series = %+v; want no all-null series", report.CategorySeries)
	}
	if report.Projection.Monthly.Status != poreport.ProjectionStatusUnavailable ||
		report.Projection.Monthly.Reason != poreport.ReasonNoDataInRange {
		t.Errorf("empty projection = %+v; want unavailable/no_data_in_range", report.Projection.Monthly)
	}
	if len(report.Population.AvailableCurrencies) != 1 ||
		report.Population.AvailableCurrencies[0] != "IDR" {
		t.Errorf("empty currencies = %v; want the selected IDR only", report.Population.AvailableCurrencies)
	}
	if report.Notes == nil {
		t.Error("notes = null; want an empty array")
	}
}

// TestPurchaseOrderReportResponseFootsToItsActuals is the endpoint-level
// footing proof: the period rows, category rows and totals are three views of
// one population and must agree exactly. Projections are separate and never
// enter any of them.
func TestPurchaseOrderReportResponseFootsToItsActuals(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_response")
	saved := finalizeOneReportPO(t, server, token, "PO-REPORT-API-1", "2026-03-10", "FM")

	values := reportQueryParams("2026-01-01", "2027-01-01", "month", "", "IDR")
	code, body := doJSON(t, server, http.MethodGet, reportURL(values), token, nil)
	if code != http.StatusOK {
		t.Fatalf("report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	wantAmount := saved.SubtotalMinor - saved.DiscountMinor + saved.AdjustmentMinor
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != wantAmount {
		t.Fatalf("totals = %+v; want one PO worth %d", report.Totals, wantAmount)
	}
	if report.Totals.UncostedMinor != report.Totals.AmountMinor-report.Totals.CostedMinor {
		t.Errorf("uncosted = %d; want amount - costed", report.Totals.UncostedMinor)
	}
	if report.Totals.MarginMinor != report.Totals.CostedMinor-report.Totals.CapitalMinor {
		t.Errorf("margin = %d; want costed - capital", report.Totals.MarginMinor)
	}
	periodAmount, periodCosted, periodCapital, periodCount := int64(0), int64(0), int64(0), int64(0)
	for _, period := range report.Periods {
		periodAmount += period.AmountMinor
		periodCosted += period.CostedMinor
		periodCapital += period.CapitalMinor
		periodCount += period.PoCount
	}
	if periodAmount != report.Totals.AmountMinor ||
		periodCosted != report.Totals.CostedMinor ||
		periodCapital != report.Totals.CapitalMinor ||
		periodCount != report.Totals.PoCount {
		t.Errorf("periods do not foot: %d/%d/%d/%d vs totals %+v",
			periodAmount, periodCosted, periodCapital, periodCount, report.Totals)
	}
	categoryAmount, categoryCount := int64(0), int64(0)
	for _, category := range report.Categories {
		categoryAmount += category.AmountMinor
		categoryCount += category.PoCount
	}
	if categoryAmount != report.Totals.AmountMinor || categoryCount != report.Totals.PoCount {
		t.Errorf("categories do not foot: %d/%d vs totals %+v",
			categoryAmount, categoryCount, report.Totals)
	}
	// The category series is aligned to the actual periods, carries a null for
	// a zero-capital period and omits categories that never had capital.
	if len(report.CategorySeries) != 1 || report.CategorySeries[0].BusinessCategory != "FM" {
		t.Fatalf("category series = %+v; want the FM series only", report.CategorySeries)
	}
	if len(report.CategorySeries[0].MarginBps) != len(report.Periods) {
		t.Errorf("series length = %d; want the %d actual periods",
			len(report.CategorySeries[0].MarginBps), len(report.Periods))
	}
	for index, period := range report.Periods {
		value := report.CategorySeries[0].MarginBps[index]
		if period.PeriodStart == "2026-03-01" {
			if value == nil {
				t.Error("March series value is null; want the FM margin percentage")
			}
		} else if value != nil {
			t.Errorf("series value for empty %s = %d; want null", period.PeriodStart, *value)
		}
	}
	if report.Filter.From != "2026-01-01" || report.Filter.To != "2027-01-01" ||
		report.Filter.Bucket != "month" || report.Filter.Category != "all" {
		t.Errorf("filter echo = %+v; want the normalized query", report.Filter)
	}
	// The projection is a forecast, not actual: its buckets must never be
	// counted in the totals above (the type separates them, and this pins the
	// response keeps them separate).
	if len(report.Projection.Monthly.Buckets) > 0 {
		if report.Totals.PoCount != 1 {
			t.Errorf("projection leaked into actual poCount: %+v", report.Totals)
		}
	}
}

// TestPurchaseOrderReportWiresAvailableProjectionsFromRealHistory proves the
// endpoint hands the store's complete monthly history to the domain projection
// and keeps the result outside every actual total. Twelve complete months and
// twelve POs clear both guards, so both series are available and start after the
// final actual bucket.
func TestPurchaseOrderReportWiresAvailableProjectionsFromRealHistory(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_projection")
	for month := 1; month <= 12; month++ {
		finalizeOneReportPO(t, server, token,
			fmt.Sprintf("PO-REPORT-PROJECTION-%02d", month),
			fmt.Sprintf("2025-%02d-10", month), "FM")
	}
	values := reportQueryParams("2025-01-01", "2026-01-01", "month", "", "IDR")
	code, body := doJSON(t, server, http.MethodGet, reportURL(values), token, nil)
	if code != http.StatusOK {
		t.Fatalf("projection report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	if report.Totals.PoCount != 12 {
		t.Fatalf("actual poCount = %d; want the 12 seeded POs", report.Totals.PoCount)
	}
	periodCount := int64(0)
	for _, period := range report.Periods {
		periodCount += period.PoCount
	}
	if periodCount != report.Totals.PoCount {
		t.Errorf("period counts = %d; want the actual total %d", periodCount, report.Totals.PoCount)
	}
	if report.Projection.Monthly.Status != poreport.ProjectionStatusAvailable {
		t.Fatalf("monthly projection = %+v; want available from 12 complete months",
			report.Projection.Monthly)
	}
	if len(report.Projection.Monthly.Buckets) != 12 {
		t.Errorf("monthly buckets = %d; want the next 12 months", len(report.Projection.Monthly.Buckets))
	}
	if got := report.Projection.Monthly.Buckets[0].PeriodStart; got != "2026-01-01" {
		t.Errorf("first monthly forecast = %s; want the month after the final actual bucket", got)
	}
	if report.Projection.Yearly.Status != poreport.ProjectionStatusAvailable {
		t.Fatalf("yearly projection = %+v; want available from 12 complete months",
			report.Projection.Yearly)
	}
	if len(report.Projection.Yearly.Buckets) != 3 {
		t.Errorf("yearly buckets = %d; want the next 3 years", len(report.Projection.Yearly.Buckets))
	}
	if got := report.Projection.Yearly.Buckets[0].PeriodStart; got != "2026-01-01" {
		t.Errorf("first yearly forecast = %s; want the January after the final actual year", got)
	}
	for _, bucket := range report.Projection.Monthly.Buckets {
		if bucket.AmountLowMinor > bucket.AmountBaseMinor ||
			bucket.AmountBaseMinor > bucket.AmountHighMinor {
			t.Errorf("monthly band is unordered: %+v", bucket)
		}
	}
	if report.Projection.Monthly.Basis.CompleteMonths != 12 ||
		report.Projection.Monthly.Basis.PoCount != 12 {
		t.Errorf("monthly basis = %+v; want 12 months and 12 POs", report.Projection.Monthly.Basis)
	}
}

func TestPurchaseOrderReportDisclosesARestoredLegacyAdjustment(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_legacy")
	saved := finalizeOneReportPO(t, server, token, "PO-REPORT-API-LEGACY", "2026-03-10", "FM")

	pool := apiReportPool(t, "vera_api_po_report_legacy")
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET adjustment_minor = 50,
		    adjustment_reason = 'restored legacy adjustment',
		    grand_total_minor = grand_total_minor + 50
		WHERE id = $1`, "vera_api_po_report_legacy"),
		saved.ID); err != nil {
		t.Fatalf("restore legacy adjustment: %v", err)
	}
	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams("2026-03-01", "2026-04-01", "month", "", "IDR")), token, nil)
	if code != http.StatusOK {
		t.Fatalf("legacy report = %d (%v); want 200", code, body)
	}
	report := decodeReport(t, body)
	found := false
	for _, note := range report.Notes {
		if note.Code == "legacy_order_adjustment" {
			found = true
			if !strings.Contains(note.Text, "not supplier cost") {
				t.Errorf("legacy note = %q; want it to state the residual is not supplier cost", note.Text)
			}
		}
	}
	if !found {
		t.Fatalf("notes = %+v; want legacy_order_adjustment", report.Notes)
	}
	if report.Totals.AmountMinor != saved.SubtotalMinor+50 {
		t.Errorf("amount = %d; want the restored residual included", report.Totals.AmountMinor)
	}
}

func TestPurchaseOrderReportFailsClosedOnAggregateOverflow(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_report_overflow")
	first := finalizeOneReportPO(t, server, token, "PO-REPORT-API-OVERFLOW-1", "2026-03-10", "FM")
	second := finalizeOneReportPO(t, server, token, "PO-REPORT-API-OVERFLOW-2", "2026-03-10", "FM")

	schema := "vera_api_po_report_overflow"
	pool := apiReportPool(t, schema)
	for _, id := range []int64{first.ID, second.ID} {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(`
			UPDATE %s.purchase_orders
			SET subtotal_minor = $2, grand_total_minor = $2
			WHERE id = $1`, schema),
			id, poreport.MaxExactMinor); err != nil {
			t.Fatalf("raise subtotal on %d: %v", id, err)
		}
	}
	code, body := doJSON(t, server, http.MethodGet,
		reportURL(reportQueryParams("2026-03-01", "2026-04-01", "month", "", "IDR")), token, nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("overflow report = %d (%v); want 422", code, body)
	}
	if got, _ := body["code"].(string); got != "report_value_out_of_range" {
		t.Errorf("overflow code = %q; want report_value_out_of_range", got)
	}
}
