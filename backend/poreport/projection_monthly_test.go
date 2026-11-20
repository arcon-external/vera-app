package poreport

import (
	"testing"
)

func TestProjectMonthlySameCalendarMonthUsesMedianAndQuartiles(t *testing.T) {
	t.Parallel()

	history := []HistoryMonth{
		hm("2022-01-01", 2, 100, 100, 10),
		hm("2023-01-01", 2, 300, 300, 30),
		hm("2024-01-01", 2, 500, 500, 50),
		hm("2025-01-01", 2, 700, 700, 70),
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2026, 1, 15), 8)
	series := projection.Monthly
	if series.Status != ProjectionStatusAvailable {
		t.Fatalf("monthly = %s/%s; want available; basis = %+v", series.Status, series.Reason, series.Basis)
	}
	if len(series.Buckets) != 12 {
		t.Fatalf("monthly buckets = %d; want 12", len(series.Buckets))
	}

	january := series.Buckets[0]
	if january.PeriodStart != "2026-01-01" {
		t.Errorf("first monthly period = %s; want 2026-01-01", january.PeriodStart)
	}
	if january.Source != SourceSameCalendarMonth || january.SampleSize != 4 {
		t.Errorf("January source/sample = %s/%d; want %s/4",
			january.Source, january.SampleSize, SourceSameCalendarMonth)
	}
	if january.AmountLowMinor != 250 || january.AmountBaseMinor != 400 || january.AmountHighMinor != 550 {
		t.Errorf("January amount band = %d/%d/%d; want 250/400/550",
			january.AmountLowMinor, january.AmountBaseMinor, january.AmountHighMinor)
	}
	if january.CapitalMinor != 40 {
		t.Errorf("January capital = %d; want 40", january.CapitalMinor)
	}
	if january.MarginMinor != 360 {
		t.Errorf("January margin = %d; want 360", january.MarginMinor)
	}
	if january.MarginBps == nil || *january.MarginBps != 90000 {
		t.Errorf("January marginBps = %v; want 90000", ratioValue(january.MarginBps))
	}

	february := series.Buckets[1]
	if february.AmountBaseMinor != 0 || february.AmountLowMinor != 0 || february.AmountHighMinor != 0 {
		t.Errorf("February amount band = %d/%d/%d; want zero observations",
			february.AmountLowMinor, february.AmountBaseMinor, february.AmountHighMinor)
	}
	if february.MarginBps != nil {
		t.Errorf("February marginBps = %d; want nil at zero capital", *february.MarginBps)
	}
	if february.Source != SourceSameCalendarMonth || february.SampleSize != 4 {
		t.Errorf("February source/sample = %s/%d; want %s/4",
			february.Source, february.SampleSize, SourceSameCalendarMonth)
	}
}

func TestProjectMonthlyFallsBackToTrailingThreeCompleteMonths(t *testing.T) {
	t.Parallel()

	history := []HistoryMonth{
		hm("2025-05-01", 1, 10, 10, 1),
		hm("2025-06-01", 1, 20, 20, 2),
		hm("2025-07-01", 1, 30, 30, 3),
		hm("2025-08-01", 1, 30, 30, 3),
		hm("2025-09-01", 1, 60, 60, 6),
		hm("2025-10-01", 1, 90, 90, 9),
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2025-11-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2025, 11, 15), 6)
	series := projection.Monthly
	if series.Status != ProjectionStatusAvailable {
		t.Fatalf("monthly = %s/%s; want available; basis = %+v", series.Status, series.Reason, series.Basis)
	}

	november := series.Buckets[0]
	if november.PeriodStart != "2025-11-01" {
		t.Errorf("first monthly period = %s; want 2025-11-01", november.PeriodStart)
	}
	if november.Source != SourceTrailingThreeCompleteMonths || november.SampleSize != 3 {
		t.Errorf("November source/sample = %s/%d; want %s/3",
			november.Source, november.SampleSize, SourceTrailingThreeCompleteMonths)
	}
	if november.AmountLowMinor != 30 || november.AmountBaseMinor != 60 || november.AmountHighMinor != 90 {
		t.Errorf("November amount band = %d/%d/%d; want 30/60/90",
			november.AmountLowMinor, november.AmountBaseMinor, november.AmountHighMinor)
	}
	if november.CapitalMinor != 6 || november.MarginMinor != 54 {
		t.Errorf("November capital/margin = %d/%d; want 6/54", november.CapitalMinor, november.MarginMinor)
	}
	if november.MarginBps == nil || *november.MarginBps != 90000 {
		t.Errorf("November marginBps = %v; want 90000", ratioValue(november.MarginBps))
	}
}

func TestProjectMonthlyUsesMinMaxUnderFourSameMonthObservations(t *testing.T) {
	t.Parallel()

	history := []HistoryMonth{
		hm("2024-01-01", 3, 100, 100, 10),
		hm("2025-01-01", 3, 500, 500, 50),
	}
	query := mustQuery(t, Query{
		From: "2024-01-01", To: "2025-07-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2025, 7, 15), 6)
	series := projection.Monthly
	if series.Status != ProjectionStatusAvailable {
		t.Fatalf("monthly = %s/%s; want available; basis = %+v", series.Status, series.Reason, series.Basis)
	}

	january2026 := series.Buckets[6]
	if january2026.PeriodStart != "2026-01-01" {
		t.Fatalf("bucket 6 = %s; want 2026-01-01", january2026.PeriodStart)
	}
	if january2026.Source != SourceSameCalendarMonth || january2026.SampleSize != 2 {
		t.Errorf("January source/sample = %s/%d; want %s/2",
			january2026.Source, january2026.SampleSize, SourceSameCalendarMonth)
	}
	if january2026.AmountLowMinor != 100 || january2026.AmountBaseMinor != 300 || january2026.AmountHighMinor != 500 {
		t.Errorf("January amount band = %d/%d/%d; want min/median/max 100/300/500",
			january2026.AmountLowMinor, january2026.AmountBaseMinor, january2026.AmountHighMinor)
	}
	if january2026.CapitalMinor != 30 {
		t.Errorf("January capital = %d; want 30", january2026.CapitalMinor)
	}
}

func TestProjectMonthlyStartsAfterTheLastIncludedMonthForEveryBucket(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 6)
	for month := 1; month <= 6; month++ {
		history = append(history, hm(monthStart(2025, month), 1, 100, 100, 10))
	}

	for _, bucket := range []string{BucketMonth, BucketQuarter, BucketYear} {
		bucket := bucket
		t.Run(bucket, func(t *testing.T) {
			t.Parallel()
			query := mustQuery(t, Query{
				From: "2025-01-01", To: "2026-08-15", Bucket: bucket,
				Category: CategoryAll, Currency: "IDR",
			})
			projection := Project(history, query, at(2026, 8, 20), 6)
			series := projection.Monthly
			if series.Status != ProjectionStatusAvailable {
				t.Fatalf("monthly = %s/%s; want available; basis = %+v", series.Status, series.Reason, series.Basis)
			}
			if len(series.Buckets) != 12 {
				t.Fatalf("monthly buckets = %d; want 12", len(series.Buckets))
			}
			if series.Buckets[0].PeriodStart != "2026-09-01" {
				t.Errorf("first monthly period = %s; want 2026-09-01", series.Buckets[0].PeriodStart)
			}
			if series.Buckets[11].PeriodStart != "2027-08-01" {
				t.Errorf("last monthly period = %s; want 2027-08-01", series.Buckets[11].PeriodStart)
			}
			yearly := projection.Yearly
			if yearly.Status != ProjectionStatusAvailable || len(yearly.Buckets) != 3 {
				t.Fatalf("yearly = %s/%s with %d buckets; want available with 3",
					yearly.Status, yearly.Reason, len(yearly.Buckets))
			}
			if yearly.Buckets[0].PeriodStart != "2027-01-01" {
				t.Errorf("first yearly period = %s; want 2027-01-01", yearly.Buckets[0].PeriodStart)
			}
		})
	}
}

func ratioValue(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
