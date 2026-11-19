package poreport

import (
	"testing"
)

func TestProjectYearlyHoldsTrailingTwelveFlatAndExpandsTheBandToIncludeTheBase(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 24)
	for year, amount := range map[int]int64{2024: 100, 2025: 200} {
		for month := 1; month <= 12; month++ {
			history = append(history, hm(
				monthStart(year, month), 1, amount, amount, amount/10,
			))
		}
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2026, 1, 15), 24)
	series := projection.Yearly
	if series.Status != ProjectionStatusAvailable {
		t.Fatalf("yearly = %s/%s; want available; basis = %+v", series.Status, series.Reason, series.Basis)
	}
	if len(series.Buckets) != 3 {
		t.Fatalf("yearly buckets = %d; want 3", len(series.Buckets))
	}

	for index, year := range []string{"2026-01-01", "2027-01-01", "2028-01-01"} {
		bucket := series.Buckets[index]
		if bucket.PeriodStart != year {
			t.Errorf("yearly bucket %d = %s; want %s", index, bucket.PeriodStart, year)
		}
		if bucket.AmountLowMinor != 1500 || bucket.AmountBaseMinor != 2400 || bucket.AmountHighMinor != 2400 {
			t.Errorf("%s amount band = %d/%d/%d; want 1500/2400/2400",
				year, bucket.AmountLowMinor, bucket.AmountBaseMinor, bucket.AmountHighMinor)
		}
		if bucket.CapitalMinor != 240 {
			t.Errorf("%s capital = %d; want 240", year, bucket.CapitalMinor)
		}
		if bucket.MarginMinor != 2160 {
			t.Errorf("%s margin = %d; want 2160", year, bucket.MarginMinor)
		}
		if bucket.MarginBps == nil || *bucket.MarginBps != 90000 {
			t.Errorf("%s marginBps = %v; want 90000", year, ratioValue(bucket.MarginBps))
		}
		if bucket.SampleSize != 12 {
			t.Errorf("%s sample = %d; want 12", year, bucket.SampleSize)
		}
		if bucket.Source != SourceTrailingTwelveCompleteMonths {
			t.Errorf("%s source = %s; want %s", year, bucket.Source, SourceTrailingTwelveCompleteMonths)
		}
	}
}

func TestProjectYearlyRequiresTwelveCompleteMonths(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 11)
	for month := 1; month <= 11; month++ {
		history = append(history, hm(monthStart(2025, month), 1, 100, 100, 10))
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2025-12-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2025, 12, 15), 11)
	if projection.Monthly.Status != ProjectionStatusAvailable {
		t.Fatalf("monthly = %s/%s; want available", projection.Monthly.Status, projection.Monthly.Reason)
	}
	if projection.Yearly.Status != ProjectionStatusUnavailable ||
		projection.Yearly.Reason != ReasonInsufficientHistory {
		t.Fatalf("yearly = %s/%s; want unavailable/%s",
			projection.Yearly.Status, projection.Yearly.Reason, ReasonInsufficientHistory)
	}
	if len(projection.Yearly.Buckets) != 0 {
		t.Errorf("yearly buckets = %d; want 0", len(projection.Yearly.Buckets))
	}
}

func TestProjectIncompleteCostBasisSuppressesBothForecastSeries(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 12)
	for month := 1; month <= 12; month++ {
		history = append(history, hm(monthStart(2025, month), 1, 100, 100, 10))
	}
	history[6] = hm("2025-07-01", 1, 1000, 500, 200)

	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})
	projection := Project(history, query, at(2026, 1, 15), 12)

	for name, series := range map[string]ProjectionSeries{
		"monthly": projection.Monthly,
		"yearly":  projection.Yearly,
	} {
		if series.Status != ProjectionStatusUnavailable || series.Reason != ReasonIncompleteCostBasis {
			t.Errorf("%s = %s/%s; want unavailable/%s",
				name, series.Status, series.Reason, ReasonIncompleteCostBasis)
		}
		if len(series.Buckets) != 0 {
			t.Errorf("%s buckets = %d; want 0", name, len(series.Buckets))
		}
	}
}

func TestProjectKeepsAFullyCostedLossAndSignsItsMargin(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 12)
	for month := 1; month <= 12; month++ {
		history = append(history, hm(monthStart(2025, month), 1, 100, 100, 10))
	}
	// July sells for 1,000 against 1,300 of pinned source cost. Every line is
	// costed (amount == costed), so the basis is complete; margin alone is negative.
	history[6] = hm("2025-07-01", 1, 1000, 1000, 1300)

	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})
	projection := Project(history, query, at(2026, 1, 15), 12)

	for name, series := range map[string]ProjectionSeries{
		"monthly": projection.Monthly,
		"yearly":  projection.Yearly,
	} {
		if series.Status != ProjectionStatusAvailable {
			t.Errorf("%s = %s/%s; want available (a cost overrun is a known cost)", name, series.Status, series.Reason)
		}
	}
	july := projection.Monthly.Buckets[6]
	if july.PeriodStart != "2026-07-01" {
		t.Fatalf("July bucket = %s; want 2026-07-01", july.PeriodStart)
	}
	if july.MarginMinor != -300 {
		t.Errorf("July margin = %d; want -300", july.MarginMinor)
	}
	if july.MarginBps == nil || *july.MarginBps != -2308 {
		t.Errorf("July marginBps = %v; want -2308", ratioValue(july.MarginBps))
	}
}

func TestProjectSampleOverflowMarksBothSeriesUnavailable(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 12)
	for month := 1; month <= 12; month++ {
		history = append(history, hm(monthStart(2025, month), 1, MaxExactMinor+1, MaxExactMinor+1, 10))
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})
	projection := Project(history, query, at(2026, 1, 15), 12)

	for name, series := range map[string]ProjectionSeries{
		"monthly": projection.Monthly,
		"yearly":  projection.Yearly,
	} {
		if series.Status != ProjectionStatusUnavailable || series.Reason != ReasonValueOutOfRange {
			t.Errorf("%s = %s/%s; want unavailable/%s",
				name, series.Status, series.Reason, ReasonValueOutOfRange)
		}
	}
}

func TestProjectRatioOverflowMarksTheMonthlySeriesUnavailable(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 12)
	for month := 1; month <= 12; month++ {
		history = append(history, hm(monthStart(2025, month), 1, MaxExactMinor, MaxExactMinor, 1))
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})
	projection := Project(history, query, at(2026, 1, 15), 12)

	if projection.Monthly.Status != ProjectionStatusUnavailable || projection.Monthly.Reason != ReasonValueOutOfRange {
		t.Fatalf("monthly = %s/%s; want unavailable/%s",
			projection.Monthly.Status, projection.Monthly.Reason, ReasonValueOutOfRange)
	}
	if len(projection.Monthly.Buckets) != 0 {
		t.Errorf("monthly buckets = %d; want 0", len(projection.Monthly.Buckets))
	}
}

func TestProjectYearlyOverflowMarksOnlyTheYearlySeriesUnavailable(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 12)
	for month := 1; month <= 12; month++ {
		history = append(history, hm(monthStart(2025, month), 1, MaxExactMinor, MaxExactMinor, 0))
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})
	projection := Project(history, query, at(2026, 1, 15), 12)

	if projection.Monthly.Status != ProjectionStatusAvailable {
		t.Fatalf("monthly = %s/%s; want available at the exact ceiling",
			projection.Monthly.Status, projection.Monthly.Reason)
	}
	if len(projection.Monthly.Buckets) != 12 {
		t.Fatalf("monthly buckets = %d; want 12", len(projection.Monthly.Buckets))
	}
	january := projection.Monthly.Buckets[0]
	if january.AmountBaseMinor != MaxExactMinor || january.MarginMinor != MaxExactMinor {
		t.Errorf("January base/margin = %d/%d; want %d/%d",
			january.AmountBaseMinor, january.MarginMinor, MaxExactMinor, MaxExactMinor)
	}

	if projection.Yearly.Status != ProjectionStatusUnavailable || projection.Yearly.Reason != ReasonValueOutOfRange {
		t.Fatalf("yearly = %s/%s; want unavailable/%s",
			projection.Yearly.Status, projection.Yearly.Reason, ReasonValueOutOfRange)
	}
	if len(projection.Yearly.Buckets) != 0 {
		t.Errorf("yearly buckets = %d; want 0", len(projection.Yearly.Buckets))
	}
}

func TestProjectFillsZeroMonthsWithoutInventingPreVeraHistory(t *testing.T) {
	t.Parallel()

	history := []HistoryMonth{
		hm("2025-03-01", 3, 300, 300, 30),
		hm("2025-08-01", 3, 600, 600, 60),
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2025-09-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})
	projection := Project(history, query, at(2025, 9, 15), 6)
	series := projection.Monthly
	if series.Status != ProjectionStatusAvailable {
		t.Fatalf("monthly = %s/%s; want available", series.Status, series.Reason)
	}
	if series.Basis.HistoryFrom != "2025-03-01" || series.Basis.CompleteMonths != 6 {
		t.Errorf("basis = %s / %d months; want 2025-03-01 / 6",
			series.Basis.HistoryFrom, series.Basis.CompleteMonths)
	}
	if series.Basis.PoCount != 6 {
		t.Errorf("basis poCount = %d; want 6", series.Basis.PoCount)
	}
	october := series.Buckets[0]
	if october.Source != SourceTrailingThreeCompleteMonths || october.SampleSize != 3 {
		t.Errorf("October source/sample = %s/%d; want %s/3",
			october.Source, october.SampleSize, SourceTrailingThreeCompleteMonths)
	}
	if october.AmountLowMinor != 0 || october.AmountBaseMinor != 0 || october.AmountHighMinor != 600 {
		t.Errorf("October amount band = %d/%d/%d; want zero-filled 0/0/600",
			october.AmountLowMinor, october.AmountBaseMinor, october.AmountHighMinor)
	}
}

func TestProjectAggregatesUnsortedDuplicateHistoryMonths(t *testing.T) {
	t.Parallel()

	history := []HistoryMonth{
		hm("2025-12-01", 1, 100, 100, 10),
		hm("2025-01-01", 1, 40, 40, 5),
		hm("2025-01-01", 2, 60, 60, 10),
	}
	for month := 2; month <= 11; month++ {
		history = append(history, hm(monthStart(2025, month), 1, 100, 100, 10))
	}
	query := mustQuery(t, Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})
	projection := Project(history, query, at(2026, 1, 15), 14)
	series := projection.Monthly
	if series.Status != ProjectionStatusAvailable {
		t.Fatalf("monthly = %s/%s; want available", series.Status, series.Reason)
	}
	if series.Basis.PoCount != 14 {
		t.Errorf("basis poCount = %d; want duplicates summed to 14", series.Basis.PoCount)
	}
	january := series.Buckets[0]
	if january.AmountBaseMinor != 100 || january.CapitalMinor != 15 || january.SampleSize != 1 {
		t.Errorf("January base/capital/sample = %d/%d/%d; want 100/15/1",
			january.AmountBaseMinor, january.CapitalMinor, january.SampleSize)
	}
}
