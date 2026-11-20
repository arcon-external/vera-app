package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
	if got.CurrentProjection.Mode != want.CurrentProjection.Mode ||
		got.CurrentProjection.Reason != want.CurrentProjection.Reason {
		t.Errorf("decoded current projection differs: %+v", got.CurrentProjection)
	}
}

func TestDecodeSnapshotAcceptsADayReport(t *testing.T) {
	raw := marshalReport(t, dayReport())
	got, err := DecodeSnapshot(raw)
	if err != nil {
		t.Fatalf("DecodeSnapshot(day report): %v", err)
	}
	if len(got.Periods) != 30 {
		t.Errorf("day report decoded %d periods; want 30", len(got.Periods))
	}
	if got.Periods[0].PeriodStart != "2026-11-01" || got.Periods[29].PeriodStart != "2026-11-30" {
		t.Errorf("day periods span %s..%s", got.Periods[0].PeriodStart, got.Periods[29].PeriodStart)
	}
	if got.CurrentProjection.Mode != poreport.ProjectionModeDay ||
		len(got.CurrentProjection.Buckets) != 11 {
		t.Errorf("decoded current projection = %+v; want 11 remaining days", got.CurrentProjection)
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
			for len(r.CategorySeries) <= 5 {
				r.CategorySeries = append(r.CategorySeries, r.CategorySeries[0])
			}
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
	if err := ValidateSnapshot(dayReport()); err != nil {
		t.Fatalf("30-day report refused: %v", err)
	}
	full := dayReport()
	full.Periods = append(full.Periods, poreport.Period{PeriodStart: "2026-12-01"})
	for index := range full.CategorySeries {
		full.CategorySeries[index].MarginBps = append(full.CategorySeries[index].MarginBps, nil)
	}
	if err := ValidateSnapshot(full); err != nil {
		t.Fatalf("31-day report refused: %v", err)
	}
	over := full
	over.Periods = append(over.Periods, poreport.Period{PeriodStart: "2026-12-02"})
	for index := range over.CategorySeries {
		over.CategorySeries[index].MarginBps = append(over.CategorySeries[index].MarginBps, nil)
	}
	if err := ValidateSnapshot(over); err == nil || !strings.Contains(err.Error(), "day periods") {
		t.Fatalf("32-day report error = %v; want the 31-day cap", err)
	}
}

func TestValidateSnapshotAcceptsALegacySnapshotWithoutCurrentProjection(t *testing.T) {
	snapshot := sampleReport()
	snapshot.CurrentProjection = poreport.CurrentProjection{}
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("legacy snapshot without currentProjection refused: %v", err)
	}
	text := renderPageText(t, snapshot)
	requireText(t, text, "Current-period projection", "No current-period projection is available in this snapshot.")
	// A snapshot with no current projection is not current-period trimmed: it
	// keeps every actual period.
	requireText(t, periodTableText(text), "Jan 2025", "Dec 2025 IP")
}

func TestValidateSnapshotRefusesMalformedCurrentProjection(t *testing.T) {
	day := func() poreport.Report { return dayReport() }
	year := func() poreport.Report { return currentYearReport() }
	cases := []struct {
		name   string
		base   func() poreport.Report
		mutate func(*poreport.Report)
		want   string
	}{
		{"unknown mode", day, func(r *poreport.Report) {
			r.CurrentProjection.Mode = "week"
		}, "mode"},
		{"unknown status", day, func(r *poreport.Report) {
			r.CurrentProjection.Status = "maybe"
		}, "status"},
		{"available in mode none", func() poreport.Report { return sampleReport() }, func(r *poreport.Report) {
			r.CurrentProjection.Status = poreport.ProjectionStatusAvailable
		}, "available in mode none"},
		{"unavailable without reason", day, func(r *poreport.Report) {
			r.CurrentProjection.Status = poreport.ProjectionStatusUnavailable
			r.CurrentProjection.Reason = ""
		}, "without a reason"},
		{"unknown reason", day, func(r *poreport.Report) {
			r.CurrentProjection.Status = poreport.ProjectionStatusUnavailable
			r.CurrentProjection.Reason = poreport.ReasonInsufficientHistory
		}, "stable current-period code"},
		{"too many day buckets", day, func(r *poreport.Report) {
			buckets := make([]poreport.ProjectedBucket, 32)
			for index := range buckets {
				buckets[index] = r.CurrentProjection.Buckets[0]
			}
			r.CurrentProjection.Buckets = buckets
		}, "buckets; the cap is 31"},
		{"too many month buckets", year, func(r *poreport.Report) {
			r.CurrentProjection.Buckets = make([]poreport.ProjectedBucket, 13)
			for index := range r.CurrentProjection.Buckets {
				r.CurrentProjection.Buckets[index] = year().CurrentProjection.Buckets[0]
				r.CurrentProjection.Buckets[index].PeriodStart = time.Date(2026, time.December+time.Month(index), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
			}
		}, "buckets; the cap is 12"},
		{"inverted bucket band", day, func(r *poreport.Report) {
			r.CurrentProjection.Buckets[0].AmountLowMinor = 11_000_000
		}, "low <= base <= high"},
		{"inverted remainder band", year, func(r *poreport.Report) {
			r.CurrentProjection.CurrentMonth.Remainder.LowMinor = 60_000_000
		}, "low <= base <= high"},
		{"month bucket not on the first", year, func(r *poreport.Report) {
			r.CurrentProjection.Buckets[0].PeriodStart = "2026-12-02"
		}, "first day of a month"},
		{"available without a sample window", day, func(r *poreport.Report) {
			r.CurrentProjection.Basis.From = ""
		}, "without a sample window"},
		{"negative basis units", day, func(r *poreport.Report) {
			r.CurrentProjection.Basis.Units = -1
		}, "units is out of range"},
		{"remainder outside exact range", year, func(r *poreport.Report) {
			r.CurrentProjection.CurrentMonth.Remainder.HighMinor = poreport.MaxExactMinor + 1
		}, "outside the exact range"},
		{"bucket outside exact range", day, func(r *poreport.Report) {
			r.CurrentProjection.Buckets[0].AmountHighMinor = poreport.MaxExactMinor + 1
		}, "outside the exact range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := tc.base()
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
