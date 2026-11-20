package backend

import (
	"encoding/json"
	"net/http"
	"testing"

	"vera-app/backend/apiclient"
	"vera-app/backend/poreport"
)

// Issue #629: the desktop binding for the purchase-order report. The API
// integration suite proves the server's arithmetic; this proves the wrapper
// sends the promised query, returns the server snapshot unchanged, and surfaces
// the server's typed refusal code for the screen to branch on.

func TestGetPurchaseOrderReportSendsTheFilterAndReturnsTheSnapshot(t *testing.T) {
	marginBps := int64(15000)
	coverageBps := int64(10000)
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/reports/purchase-orders" {
			t.Errorf("request = %s %s; want GET /v1/reports/purchase-orders",
				request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("from") != "2026-01-01" || query.Get("to") != "2027-01-01" {
			t.Errorf("range = %q..%q; want 2026-01-01..2027-01-01",
				query.Get("from"), query.Get("to"))
		}
		if query.Get("bucket") != "quarter" {
			t.Errorf("bucket = %q; want quarter", query.Get("bucket"))
		}
		if query.Get("category") != "FAB" {
			t.Errorf("category = %q; want FAB", query.Get("category"))
		}
		if query.Get("currency") != "USD" {
			t.Errorf("currency = %q; want USD", query.Get("currency"))
		}
		_ = json.NewEncoder(writer).Encode(poreport.Report{
			GeneratedAt: "2026-11-19T10:00:00Z",
			AsOf:        "2026-11-19",
			Filter: poreport.Query{
				From: "2026-01-01", To: "2027-01-01", Bucket: "quarter",
				Category: "FAB", Currency: "USD",
			},
			Company: poreport.Company{Name: "PT Arcon Penta Persada", Address: "Batam"},
			Population: poreport.Population{
				AvailableCurrencies:  []string{"IDR", "USD"},
				HasUnassigned:        true,
				SourceMissingPoCount: 1,
			},
			Totals: poreport.Metrics{
				PoCount: 2, AmountMinor: 500, CostedMinor: 500,
				UncostedMinor: 0, CapitalMinor: 200, MarginMinor: 300,
				MarginBps: &marginBps, CoverageBps: &coverageBps,
			},
			Periods: []poreport.Period{{
				PeriodStart: "2026-01-01", Partial: false, InProgress: false,
				Metrics: poreport.Metrics{PoCount: 2, AmountMinor: 500, CostedMinor: 500,
					CapitalMinor: 200, MarginMinor: 300},
			}},
			Categories: []poreport.Category{{
				BusinessCategory: "FAB", Label: "Fabrication",
				Metrics: poreport.Metrics{PoCount: 2, AmountMinor: 500, CostedMinor: 500,
					CapitalMinor: 200, MarginMinor: 300},
			}},
			CategorySeries: []poreport.CategorySeries{{
				BusinessCategory: "FAB", MarginBps: []*int64{nil, &marginBps},
			}},
			Projection: poreport.Projection{
				Monthly: poreport.ProjectionSeries{
					Status:  poreport.ProjectionStatusUnavailable,
					Reason:  poreport.ReasonInsufficientHistory,
					Method:  poreport.MethodMonthly,
					Basis:   poreport.ProjectionBasis{AsOf: "2026-11-19"},
					Buckets: []poreport.ProjectedBucket{},
				},
				Yearly: poreport.ProjectionSeries{
					Status:  poreport.ProjectionStatusUnavailable,
					Reason:  poreport.ReasonInsufficientHistory,
					Method:  poreport.MethodYearly,
					Basis:   poreport.ProjectionBasis{AsOf: "2026-11-19"},
					Buckets: []poreport.ProjectedBucket{},
				},
			},
			Notes: []poreport.Note{{Code: "legacy_order_adjustment", Text: "residual"}},
		})
	})

	q := newTestQuotation()
	report, err := q.GetPurchaseOrderReport(poreport.Query{
		From: " 2026-01-01 ", To: "2027-01-01", Bucket: "quarter",
		Category: "FAB", Currency: "USD",
	})
	if err != nil {
		t.Fatalf("GetPurchaseOrderReport failed: %v", err)
	}
	if report.GeneratedAt != "2026-11-19T10:00:00Z" || report.AsOf != "2026-11-19" {
		t.Errorf("timestamps = %q/%q; want the snapshot pair", report.GeneratedAt, report.AsOf)
	}
	if report.Filter.Currency != "USD" || report.Filter.Category != "FAB" {
		t.Errorf("filter = %+v; want the echoed USD FAB query", report.Filter)
	}
	if report.Company.Name != "PT Arcon Penta Persada" || report.Company.Address != "Batam" {
		t.Errorf("company = %+v; want the server profile", report.Company)
	}
	if report.Population.SourceMissingPoCount != 1 || !report.Population.HasUnassigned {
		t.Errorf("population = %+v; want the facet values preserved", report.Population)
	}
	if report.Totals.MarginBps == nil || *report.Totals.MarginBps != 15000 {
		t.Errorf("margin bps = %v; want 15000", report.Totals.MarginBps)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 10000 {
		t.Errorf("coverage bps = %v; want 10000", report.Totals.CoverageBps)
	}
	if len(report.Periods) != 1 || report.Periods[0].PeriodStart != "2026-01-01" {
		t.Errorf("periods = %+v; want one period", report.Periods)
	}
	if len(report.Categories) != 1 || report.Categories[0].Label != "Fabrication" {
		t.Errorf("categories = %+v; want the labelled FAB row", report.Categories)
	}
	if len(report.CategorySeries) != 1 || len(report.CategorySeries[0].MarginBps) != 2 {
		t.Fatalf("series = %+v; want aligned FAB values", report.CategorySeries)
	}
	if report.CategorySeries[0].MarginBps[0] != nil {
		t.Error("first series value is non-nil; want the null zero-capital period preserved")
	}
	if report.CategorySeries[0].MarginBps[1] == nil ||
		*report.CategorySeries[0].MarginBps[1] != 15000 {
		t.Errorf("second series value = %v; want 15000", report.CategorySeries[0].MarginBps[1])
	}
	if report.Projection.Monthly.Reason != poreport.ReasonInsufficientHistory {
		t.Errorf("projection reason = %q; want insufficient_history", report.Projection.Monthly.Reason)
	}
	if len(report.Notes) != 1 || report.Notes[0].Code != "legacy_order_adjustment" {
		t.Errorf("notes = %+v; want the legacy disclosure", report.Notes)
	}
}

func TestGetPurchaseOrderReportOmitsEmptyOptionalFilters(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("from") != "2026-01-01" || query.Get("to") != "2026-02-01" ||
			query.Get("bucket") != "month" {
			t.Errorf("required query = %v; want from/to/bucket", query)
		}
		for _, key := range []string{"category", "currency"} {
			if _, present := query[key]; present {
				t.Errorf("%s = %v; an omitted filter must not be sent", key, query[key])
			}
		}
		_ = json.NewEncoder(writer).Encode(poreport.Report{
			Filter: poreport.Query{From: "2026-01-01", To: "2026-02-01", Bucket: "month",
				Category: "all", Currency: "IDR"},
			Periods: []poreport.Period{}, Categories: []poreport.Category{},
			CategorySeries: []poreport.CategorySeries{}, Notes: []poreport.Note{},
		})
	})

	q := newTestQuotation()
	report, err := q.GetPurchaseOrderReport(poreport.Query{
		From: "2026-01-01", To: "2026-02-01", Bucket: "month",
	})
	if err != nil {
		t.Fatalf("GetPurchaseOrderReport failed: %v", err)
	}
	if report.Filter.Currency != "IDR" || report.Filter.Category != "all" {
		t.Errorf("defaulted filter = %+v; want the server's IDR/all default", report.Filter)
	}
	if report.Periods == nil || report.Categories == nil ||
		report.CategorySeries == nil || report.Notes == nil {
		t.Errorf("empty report = %+v; want non-nil empty arrays", report)
	}
}

func TestGetPurchaseOrderReportKeepsTheTypedRangeRefusal(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{"error":"range is too large","code":"report_range_too_large"}`))
	})

	q := newTestQuotation()
	_, err := q.GetPurchaseOrderReport(poreport.Query{
		From: "2015-12-01", To: "2026-01-01", Bucket: "month", Currency: "IDR",
	})
	if err == nil {
		t.Fatal("oversized report succeeded; want the server refusal")
	}
	if got := apiclient.CodeOf(err); got != "report_range_too_large" {
		t.Errorf("refusal code = %q; want report_range_too_large", got)
	}
	if got := apiclient.StatusOf(err); got != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422", got)
	}
}

func TestGetPurchaseOrderReportRequiresSignIn(t *testing.T) {
	q := newTestQuotation()
	if _, err := q.GetPurchaseOrderReport(poreport.Query{
		From: "2026-01-01", To: "2026-02-01", Bucket: "month", Currency: "IDR",
	}); err == nil {
		t.Error("GetPurchaseOrderReport without a session succeeded; want refusal")
	}
}
