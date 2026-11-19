package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"vera-app/backend/poreport"
)

func marshalReport(t *testing.T, snapshot poreport.Report) []byte {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	return raw
}

func TestDecodeSnapshotRoundTripsTheReport(t *testing.T) {
	want := sampleReport()
	got, err := DecodeSnapshot(marshalReport(t, want))
	if err != nil {
		t.Fatalf("DecodeSnapshot: %v", err)
	}
	if got.Filter != want.Filter || got.GeneratedAt != want.GeneratedAt || got.AsOf != want.AsOf {
		t.Errorf("decoded filter/metadata differs: %+v", got.Filter)
	}
	if len(got.Periods) != len(want.Periods) || got.Periods[5].MarginBps != nil {
		t.Errorf("decoded periods lost the unknown percentage")
	}
}

func TestDecodeSnapshotRefusesMalformedInput(t *testing.T) {
	valid := marshalReport(t, sampleReport())
	var generic map[string]any
	if err := json.Unmarshal(valid, &generic); err != nil {
		t.Fatal(err)
	}
	generic["unexpected"] = true
	withUnknown, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{"empty", nil, "decode"},
		{"truncated", valid[:len(valid)/2], "decode"},
		{"unknown field", withUnknown, "unknown field"},
		{"trailing data", append(append([]byte{}, valid...), []byte(` {}`)...), "trailing data"},
		{"float into integer", []byte(`{"generatedAt":"2026-01-05T08:30:00Z","asOf":"2026-01-04","filter":{"from":"2025-01-01","to":"2026-01-01","bucket":"month","category":"all","currency":"IDR"},"totals":{"poCount":1.5}}`), "decode"},
		{"oversized", bytes.Repeat([]byte("x"), SnapshotLimit+1), "limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeSnapshot(tc.raw)
			if err == nil {
				t.Fatalf("DecodeSnapshot accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateSnapshotRefusesShapesTheRendererCannotPrint(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*poreport.Report)
		want   string
	}{
		{"too many periods", func(r *poreport.Report) {
			r.Periods = append(longReport(120).Periods, poreport.Period{PeriodStart: "2026-01-01"})
		}, "actual periods"},
		{"too many categories", func(r *poreport.Report) {
			r.Categories = append(r.Categories, poreport.Category{BusinessCategory: "SIX", Label: "Six"})
		}, "category rows"},
		{"too many category series", func(r *poreport.Report) {
			r.CategorySeries = append(r.CategorySeries, poreport.CategorySeries{BusinessCategory: "SIX"})
		}, "category series"},
		{"too many monthly buckets", func(r *poreport.Report) {
			r.Projection.Monthly.Buckets = append(r.Projection.Monthly.Buckets, r.Projection.Monthly.Buckets[0])
		}, "buckets"},
		{"too many yearly buckets", func(r *poreport.Report) {
			r.Projection.Yearly.Buckets = append(r.Projection.Yearly.Buckets, r.Projection.Yearly.Buckets[0])
		}, "buckets"},
		{"amount outside exact range", func(r *poreport.Report) {
			r.Totals.AmountMinor = poreport.MaxExactMinor + 1
		}, "outside the exact range"},
		{"unknown currency", func(r *poreport.Report) {
			r.Filter.Currency = "XYZ"
		}, "supported catalogue"},
		{"bad period start", func(r *poreport.Report) {
			r.Periods[0].PeriodStart = "2025-01-02"
		}, "first day of a period"},
		{"unordered periods", func(r *poreport.Report) {
			r.Periods[0], r.Periods[1] = r.Periods[1], r.Periods[0]
		}, "not after"},
		{"misaligned category series", func(r *poreport.Report) {
			r.CategorySeries[0].MarginBps = r.CategorySeries[0].MarginBps[:3]
		}, "margin points"},
		{"inverted band", func(r *poreport.Report) {
			r.Projection.Monthly.Buckets[0].AmountLowMinor = 99_000_000
		}, "low <= base <= high"},
		{"bad projection status", func(r *poreport.Report) {
			r.Projection.Monthly.Status = "maybe"
		}, "status"},
		{"available without buckets", func(r *poreport.Report) {
			r.Projection.Monthly.Buckets = nil
		}, "available with no buckets"},
		{"unavailable without reason", func(r *poreport.Report) {
			r.Projection.Monthly = poreport.ProjectionSeries{
				Status: poreport.ProjectionStatusUnavailable, Method: poreport.MethodMonthly,
			}
		}, "without a reason"},
		{"bad generatedAt", func(r *poreport.Report) {
			r.GeneratedAt = "yesterday"
		}, "RFC3339"},
		{"negative count", func(r *poreport.Report) {
			r.Totals.PoCount = -1
		}, "negative"},
		{"too many notes", func(r *poreport.Report) {
			for len(r.Notes) <= 32 {
				r.Notes = append(r.Notes, poreport.Note{Code: "x", Text: "y"})
			}
		}, "notes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := sampleReport()
			tc.mutate(&snapshot)
			err := ValidateSnapshot(snapshot)
			if err == nil {
				t.Fatalf("ValidateSnapshot accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateSnapshotAcceptsTheFullPeriodCap(t *testing.T) {
	if err := ValidateSnapshot(longReport(poreport.MaxPeriods)); err != nil {
		t.Fatalf("120-period report refused: %v", err)
	}
	if err := ValidateSnapshot(longReport(poreport.MaxPeriods + 1)); err == nil {
		t.Fatalf("121-period report accepted")
	}
}
