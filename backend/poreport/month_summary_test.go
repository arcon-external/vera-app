package poreport

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"
)

// Issue #641: the current month's server-owned readout. These tests pin the
// arithmetic contract the screen and the PDF consume: actual-to-date comes from
// the elapsed-day sample, recorded future-dated evidence is separate, the daily
// remainder is the exact sum of the supplied rounded buckets, and an unavailable
// forecast never erases actual evidence.

func monthTestDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		t.Fatalf("parse %s: %v", value, err)
	}
	return date
}

func monthTestRatio(t *testing.T, numerator, denominator int64) *int64 {
	t.Helper()
	ratio, err := RatioBps(numerator, denominator)
	if err != nil {
		t.Fatalf("RatioBps(%d, %d): %v", numerator, denominator, err)
	}
	return ratio
}

func monthTestMetrics(t *testing.T, count, amount, costed, capital int64) Metrics {
	t.Helper()
	metrics, err := currentMetrics(count, amount, costed, capital)
	if err != nil {
		t.Fatalf("currentMetrics: %v", err)
	}
	return metrics
}

func monthDay(t *testing.T, date string, count, amount, costed, capital int64) HistoryDay {
	t.Helper()
	return HistoryDay{
		Date:    date,
		Metrics: monthTestMetrics(t, count, amount, costed, capital),
	}
}

func monthPeriod(t *testing.T, start string, count, amount, costed, capital int64) Period {
	t.Helper()
	return Period{
		PeriodStart: start,
		Metrics:     monthTestMetrics(t, count, amount, costed, capital),
	}
}

func monthSummaryHistory(t *testing.T, start string, count, amount, costed, capital int64) HistoryMonth {
	t.Helper()
	return HistoryMonth{
		MonthStart: start,
		Metrics:    monthTestMetrics(t, count, amount, costed, capital),
	}
}

func currentMonthDayQuery() Query {
	return Query{
		From: "2026-11-01", To: "2026-12-01", Bucket: BucketDay,
		Category: CategoryAll, Currency: "IDR",
	}
}

func currentYearMonthQuery() Query {
	return Query{
		From: "2026-01-01", To: "2027-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	}
}

// dailySample is three elapsed POs in November 2026, enough to pass the daily
// 5-day/3-PO guard. The as-of date makes 19 days elapsed.
func dailySample(t *testing.T) []HistoryDay {
	t.Helper()
	return []HistoryDay{
		monthDay(t, "2026-11-01", 1, 1_000_000, 1_000_000, 700_000),
		monthDay(t, "2026-11-02", 1, 2_500_000, 2_500_000, 1_800_000),
		monthDay(t, "2026-11-03", 1, 500_000, 500_000, 300_000),
	}
}

func TestBuildMonthSummarySumsTheSuppliedDailyBucketsExactly(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentMonthDayQuery()
	daily := dailySample(t)
	current := ProjectCurrent(nil, daily, query, asOf, false, false)
	if current.Mode != ProjectionModeDay || current.Status != ProjectionStatusAvailable {
		t.Fatalf("projection = %s/%s/%s; want day/available", current.Mode, current.Status, current.Reason)
	}

	summary, err := BuildMonthSummary([]Period{
		monthPeriod(t, "2026-11-20", 0, 0, 0, 0),
		monthPeriod(t, "2026-11-30", 0, 0, 0, 0),
	}, daily, current, query, asOf, false)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("monthSummary = nil; want the current month readout")
	}
	if summary.Month != "2026-11" || summary.From != "2026-11-01" || summary.To != "2026-12-01" {
		t.Errorf("month/from/to = %s/%s/%s", summary.Month, summary.From, summary.To)
	}
	if summary.AsOf != "2026-11-19" || summary.Mode != ProjectionModeDay {
		t.Errorf("asOf/mode = %s/%s", summary.AsOf, summary.Mode)
	}

	wantActual := monthTestMetrics(t, 3, 4_000_000, 4_000_000, 2_800_000)
	if !equalMonthMetrics(summary.Actual, wantActual) {
		t.Errorf("actual = %+v; want %+v", summary.Actual, wantActual)
	}
	if summary.RecordedFuture != nil {
		t.Errorf("recordedFuture = %+v; want omitted with no future-dated row", summary.RecordedFuture)
	}
	if summary.Projection.Status != ProjectionStatusAvailable {
		t.Fatalf("projection status = %s (%s); want available", summary.Projection.Status, summary.Projection.Reason)
	}
	if summary.Projection.Remainder == nil {
		t.Fatal("projection remainder = nil; want every remaining day summed")
	}

	var low, base, high, capital, margin int64
	for _, bucket := range current.Buckets {
		low += bucket.AmountLowMinor
		base += bucket.AmountBaseMinor
		high += bucket.AmountHighMinor
		capital += bucket.CapitalMinor
		margin += bucket.MarginMinor
	}
	remainder := summary.Projection.Remainder
	if remainder.LowMinor != low || remainder.BaseMinor != base ||
		remainder.HighMinor != high || remainder.CapitalMinor != capital ||
		remainder.MarginMinor != margin {
		t.Errorf("remainder = %+v; want the exact sum low=%d base=%d high=%d capital=%d margin=%d",
			remainder, low, base, high, capital, margin)
	}
	if remainder.SampleSize != 19 || remainder.Source != SourceElapsedDays {
		t.Errorf("remainder sample/source = %d/%s; want 19/%s",
			remainder.SampleSize, remainder.Source, SourceElapsedDays)
	}
	wantBps, err := RatioBps(margin, capital)
	if err != nil {
		t.Fatalf("RatioBps: %v", err)
	}
	if !sameOptionalInt64(remainder.MarginBps, wantBps) {
		t.Errorf("remainder marginBps = %s; want the exact ratio %s",
			showOptionalInt64(remainder.MarginBps), showOptionalInt64(wantBps))
	}
}

// TestBuildMonthSummaryRollsOverAtTheWIBMonthBoundary crosses 23:59:59 to
// 00:00:01 WIB through the existing asOf/project/summary seam. Both instants
// fall on 30 November in UTC, so only the WIB business date can explain the
// December readout; no fake clock API is involved.
func TestBuildMonthSummaryRollsOverAtTheWIBMonthBoundary(t *testing.T) {
	before := time.Date(2026, time.November, 30, 23, 59, 59, 0, companyLocation)
	after := time.Date(2026, time.December, 1, 0, 0, 1, 0, companyLocation)
	if got := before.UTC().Format(reportDateLayout); got != "2026-11-30" {
		t.Fatalf("pre-boundary UTC date = %s; want 2026-11-30 for this WIB boundary", got)
	}
	if got := after.UTC().Format(reportDateLayout); got != "2026-11-30" {
		t.Fatalf("post-boundary UTC date = %s; want 2026-11-30 for this WIB boundary", got)
	}

	daily := dailySample(t)
	daily = append(daily, monthDay(t, "2026-12-01", 1, 2_000_000, 1_800_000, 1_200_000))

	novemberQuery := currentMonthDayQuery()
	novemberCurrent := ProjectCurrent(nil, daily, novemberQuery, before.UTC(), false, false)
	novemberSummary, err := BuildMonthSummary(nil, daily, novemberCurrent, novemberQuery, before.UTC(), false)
	if err != nil {
		t.Fatalf("BuildMonthSummary(November): %v", err)
	}
	if novemberSummary == nil {
		t.Fatal("monthSummary = nil at 23:59:59 WIB; want the closing November month")
	}
	if novemberSummary.Month != "2026-11" || novemberSummary.AsOf != "2026-11-30" {
		t.Errorf("pre-boundary summary month/asOf = %s/%s; want 2026-11/2026-11-30",
			novemberSummary.Month, novemberSummary.AsOf)
	}
	if novemberSummary.Actual.PoCount != 3 || novemberSummary.Actual.AmountMinor != 4_000_000 {
		t.Errorf("pre-boundary actual = %+v; want the three November POs", novemberSummary.Actual)
	}
	if novemberCurrent.Status != ProjectionStatusAvailable {
		t.Fatalf("pre-boundary projection = %s/%s; want available on the month's last day",
			novemberCurrent.Status, novemberCurrent.Reason)
	}
	// The last day of a month has no remaining day, but a zero remainder is an
	// available forecast, not an unavailable one.
	if novemberSummary.Projection.Status != ProjectionStatusAvailable ||
		novemberSummary.Projection.Remainder == nil {
		t.Fatalf("pre-boundary month forecast = %s/%+v; want an available zero remainder",
			novemberSummary.Projection.Status, novemberSummary.Projection.Remainder)
	}
	remainder := *novemberSummary.Projection.Remainder
	if remainder.LowMinor != 0 || remainder.BaseMinor != 0 || remainder.HighMinor != 0 ||
		remainder.CapitalMinor != 0 || remainder.MarginMinor != 0 {
		t.Errorf("pre-boundary remainder = %+v; want an all-zero month-end remainder", remainder)
	}
	if remainder.SampleSize != 30 || remainder.Source != SourceElapsedDays {
		t.Errorf("pre-boundary remainder sample/source = %d/%s; want 30/%s",
			remainder.SampleSize, remainder.Source, SourceElapsedDays)
	}

	decemberQuery := Query{
		From: "2026-12-01", To: "2027-01-01", Bucket: BucketDay,
		Category: CategoryAll, Currency: "IDR",
	}
	decemberCurrent := ProjectCurrent(nil, daily, decemberQuery, after.UTC(), false, false)
	decemberSummary, err := BuildMonthSummary(nil, daily, decemberCurrent, decemberQuery, after.UTC(), false)
	if err != nil {
		t.Fatalf("BuildMonthSummary(December): %v", err)
	}
	if decemberSummary == nil {
		t.Fatal("monthSummary = nil at 00:00:01 WIB; want the opening December month")
	}
	if decemberSummary.Month != "2026-12" || decemberSummary.AsOf != "2026-12-01" {
		t.Errorf("post-boundary summary month/asOf = %s/%s; want 2026-12/2026-12-01",
			decemberSummary.Month, decemberSummary.AsOf)
	}
	if decemberSummary.Actual.PoCount != 1 || decemberSummary.Actual.AmountMinor != 2_000_000 {
		t.Errorf("post-boundary actual = %+v; want only the 1 December PO", decemberSummary.Actual)
	}
	if decemberCurrent.Status != ProjectionStatusUnavailable ||
		decemberCurrent.Reason != ReasonInsufficientDays {
		t.Fatalf("post-boundary projection = %s/%s; want unavailable/%s on the first day",
			decemberCurrent.Status, decemberCurrent.Reason, ReasonInsufficientDays)
	}
	if decemberSummary.Projection.Status != ProjectionStatusUnavailable ||
		decemberSummary.Projection.Reason != ReasonInsufficientDays ||
		decemberSummary.Projection.Remainder != nil {
		t.Errorf("post-boundary month forecast = %+v; want unavailable/%s with no remainder",
			decemberSummary.Projection, ReasonInsufficientDays)
	}
}

func TestBuildMonthSummaryKeepsActualWhenTheDayGuardFails(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentMonthDayQuery()
	daily := []HistoryDay{
		monthDay(t, "2026-11-01", 1, 1_000_000, 1_000_000, 700_000),
		monthDay(t, "2026-11-02", 1, 2_500_000, 2_500_000, 1_800_000),
	}
	current := ProjectCurrent(nil, daily, query, asOf, false, false)
	if current.Status != ProjectionStatusUnavailable || current.Reason != ReasonInsufficientDays {
		t.Fatalf("projection = %s/%s; want unavailable/insufficient_days", current.Status, current.Reason)
	}

	summary, err := BuildMonthSummary(nil, daily, current, query, asOf, false)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("monthSummary = nil; actual-to-date must survive a failed forecast guard")
	}
	if summary.Actual.PoCount != 2 || summary.Actual.AmountMinor != 3_500_000 {
		t.Errorf("actual = %+v; want the two elapsed POs", summary.Actual)
	}
	if summary.Projection.Status != ProjectionStatusUnavailable ||
		summary.Projection.Reason != ReasonInsufficientDays {
		t.Errorf("projection = %s/%s/%s; want unavailable/%s",
			summary.Projection.Status, summary.Projection.Reason, summary.Projection.Method, ReasonInsufficientDays)
	}
	if summary.Projection.Remainder != nil {
		t.Errorf("remainder = %+v; want omitted when the forecast is unavailable", summary.Projection.Remainder)
	}
}

func TestBuildMonthSummarySeparatesRecordedFutureFromActual(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentMonthDayQuery()
	daily := dailySample(t)
	current := ProjectCurrent(nil, daily, query, asOf, false, false)

	// One future-dated saved PO, already inside the report's half-open month.
	future := monthPeriod(t, "2026-11-25", 1, 777_000, 700_000, 600_000)
	periods := []Period{
		monthPeriod(t, "2026-11-01", 1, 1_000_000, 1_000_000, 700_000),
		monthPeriod(t, "2026-11-02", 1, 2_500_000, 2_500_000, 1_800_000),
		monthPeriod(t, "2026-11-03", 1, 500_000, 500_000, 300_000),
		future,
	}

	summary, err := BuildMonthSummary(periods, daily, current, query, asOf, false)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("monthSummary = nil; want current month readout")
	}
	if summary.Actual.PoCount != 3 || summary.Actual.AmountMinor != 4_000_000 {
		t.Errorf("actual = %+v; future-dated evidence leaked into actual", summary.Actual)
	}
	if summary.RecordedFuture == nil {
		t.Fatal("recordedFuture = nil; want the future-dated saved PO as separate evidence")
	}
	if !equalMonthMetrics(*summary.RecordedFuture, future.Metrics) {
		t.Errorf("recordedFuture = %+v; want the future period's exact metrics %+v",
			*summary.RecordedFuture, future.Metrics)
	}
	// The forecast remainder is not the recorded row, and no field adds them.
	if summary.Projection.Remainder == nil {
		t.Fatal("remainder = nil; want the daily forecast")
	}
	if summary.Projection.Remainder.BaseMinor == summary.RecordedFuture.AmountMinor {
		t.Errorf("remainder base equals the recorded future amount; the two quantities are not interchangeable")
	}
}

func TestBuildMonthSummaryMonthModeUsesTheRemainderAndRecordedDifference(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentYearMonthQuery()
	history := []HistoryMonth{
		monthSummaryHistory(t, "2026-08-01", 2, 4_000_000, 4_000_000, 3_000_000),
		monthSummaryHistory(t, "2026-09-01", 2, 6_000_000, 6_000_000, 4_500_000),
		monthSummaryHistory(t, "2026-10-01", 2, 5_000_000, 5_000_000, 4_000_000),
	}
	daily := dailySample(t)
	current := ProjectCurrent(history, daily, query, asOf, false, false)
	if current.Mode != ProjectionModeMonth || current.Status != ProjectionStatusAvailable ||
		current.CurrentMonth == nil {
		t.Fatalf("projection = %s/%s/%s currentMonth=%v; want month/available",
			current.Mode, current.Status, current.Reason, current.CurrentMonth)
	}

	// The raw current-month period row includes the three elapsed POs and one
	// future-dated row. Actual must be month-to-date; the difference is recorded
	// future evidence.
	raw := monthPeriod(t, "2026-11-01", 4, 4_777_000, 4_700_000, 3_400_000)
	periods := []Period{raw}

	summary, err := BuildMonthSummary(periods, daily, current, query, asOf, false)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("monthSummary = nil; want current month readout")
	}
	wantActual := monthTestMetrics(t, 3, 4_000_000, 4_000_000, 2_800_000)
	if !equalMonthMetrics(summary.Actual, wantActual) {
		t.Errorf("actual = %+v; want %+v", summary.Actual, wantActual)
	}
	wantFuture := monthTestMetrics(t, 1, 777_000, 700_000, 600_000)
	if summary.RecordedFuture == nil || !equalMonthMetrics(*summary.RecordedFuture, wantFuture) {
		t.Errorf("recordedFuture = %+v; want %+v", summary.RecordedFuture, wantFuture)
	}
	if summary.Projection.Remainder == nil {
		t.Fatal("projection remainder = nil; want the month-mode remainder")
	}
	if !equalRemainder(*summary.Projection.Remainder, current.CurrentMonth.Remainder) {
		t.Errorf("remainder = %+v; want the existing month-mode remainder %+v",
			*summary.Projection.Remainder, current.CurrentMonth.Remainder)
	}
}

func TestBuildMonthSummaryKeepsActualWhenTheMonthGuardFails(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentYearMonthQuery()
	history := []HistoryMonth{
		monthSummaryHistory(t, "2026-10-01", 2, 5_000_000, 5_000_000, 4_000_000),
	}
	daily := dailySample(t)
	current := ProjectCurrent(history, daily, query, asOf, false, false)
	if current.Status != ProjectionStatusUnavailable || current.Reason != ReasonInsufficientMonths {
		t.Fatalf("projection = %s/%s; want unavailable/insufficient_months", current.Status, current.Reason)
	}

	summary, err := BuildMonthSummary(
		[]Period{monthPeriod(t, "2026-11-01", 3, 4_000_000, 4_000_000, 2_800_000)},
		daily, current, query, asOf, false)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary == nil || summary.Actual.PoCount != 3 {
		t.Fatalf("summary = %+v; want actual-to-date even with an unavailable forecast", summary)
	}
	if summary.Projection.Status != ProjectionStatusUnavailable ||
		summary.Projection.Reason != ReasonInsufficientMonths ||
		summary.Projection.Remainder != nil {
		t.Errorf("projection = %+v; want unavailable/insufficient_months and no remainder", summary.Projection)
	}
}

func TestBuildMonthSummaryIsAbsentOutsideTheExactCurrentPeriods(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	daily := dailySample(t)
	cases := []struct {
		name  string
		query Query
	}{
		{
			name: "current year bucketed by quarter",
			query: Query{
				From: "2026-01-01", To: "2027-01-01", Bucket: BucketQuarter,
				Category: CategoryAll, Currency: "IDR",
			},
		},
		{
			name: "current month bucketed by month",
			query: Query{
				From: "2026-11-01", To: "2026-12-01", Bucket: BucketMonth,
				Category: CategoryAll, Currency: "IDR",
			},
		},
		{
			name: "historical year",
			query: Query{
				From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth,
				Category: CategoryAll, Currency: "IDR",
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			current := ProjectCurrent(nil, daily, test.query, asOf, false, false)
			summary, err := BuildMonthSummary(nil, daily, current, test.query, asOf, false)
			if err != nil {
				t.Fatalf("BuildMonthSummary: %v", err)
			}
			if summary != nil {
				t.Errorf("summary = %+v; want nil outside the exact current month/day or year/month shape", summary)
			}
		})
	}
}

func TestBuildMonthSummaryIsAbsentWhenTheDailySampleIsCorrupt(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentMonthDayQuery()
	daily := dailySample(t)
	current := ProjectCurrent(nil, daily, query, asOf, false, true)
	summary, err := BuildMonthSummary(nil, daily, current, query, asOf, true)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary != nil {
		t.Errorf("summary = %+v; want nil when the daily sample could not be represented exactly", summary)
	}
}

func TestBuildMonthSummaryDowngradesAnInexactDailyRemainder(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentMonthDayQuery()
	daily := dailySample(t)
	current := CurrentProjection{
		Mode:   ProjectionModeDay,
		Status: ProjectionStatusAvailable,
		Method: MethodCurrentDay,
		Basis:  CurrentProjectionBasis{From: "2026-11-01", To: "2026-11-20", Units: 19, PoCount: 3, AsOf: "2026-11-19"},
		Buckets: []ProjectedBucket{
			{
				PeriodStart: "2026-11-20", AmountLowMinor: MaxExactMinor, AmountBaseMinor: MaxExactMinor,
				AmountHighMinor: MaxExactMinor, CapitalMinor: 0, MarginMinor: MaxExactMinor,
				SampleSize: 19, Source: SourceElapsedDays,
			},
			{
				PeriodStart: "2026-11-21", AmountLowMinor: MaxExactMinor, AmountBaseMinor: MaxExactMinor,
				AmountHighMinor: MaxExactMinor, CapitalMinor: 0, MarginMinor: MaxExactMinor,
				SampleSize: 19, Source: SourceElapsedDays,
			},
		},
	}

	summary, err := BuildMonthSummary(nil, daily, current, query, asOf, false)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("summary = nil; actual evidence must survive an inexact forecast sum")
	}
	if summary.Actual.PoCount != 3 {
		t.Errorf("actual = %+v; want the elapsed sample unchanged", summary.Actual)
	}
	if summary.Projection.Status != ProjectionStatusUnavailable ||
		summary.Projection.Reason != ReasonValueOutOfRange ||
		summary.Projection.Remainder != nil {
		t.Errorf("projection = %+v; want unavailable/value_out_of_range with no remainder", summary.Projection)
	}
}

func TestBuildMonthSummaryRecordedFutureKeepsSignsAndNullRatios(t *testing.T) {
	asOf := monthTestDate(t, "2026-11-19")
	query := currentMonthDayQuery()
	daily := dailySample(t)
	current := ProjectCurrent(nil, daily, query, asOf, false, false)

	future := monthPeriod(t, "2026-11-27", 1, 100_000, 100_000, 150_000)
	summary, err := BuildMonthSummary([]Period{future}, daily, current, query, asOf, false)
	if err != nil {
		t.Fatalf("BuildMonthSummary: %v", err)
	}
	if summary == nil || summary.RecordedFuture == nil {
		t.Fatalf("summary = %+v; want recorded future evidence", summary)
	}
	if summary.RecordedFuture.MarginMinor != -50_000 {
		t.Errorf("recorded future margin = %d; want the signed loss -50000",
			summary.RecordedFuture.MarginMinor)
	}
	if summary.RecordedFuture.MarginBps == nil || *summary.RecordedFuture.MarginBps != -3333 {
		t.Errorf("recorded future marginBps = %s; want -3333",
			showOptionalInt64(summary.RecordedFuture.MarginBps))
	}
}

func TestMonthSummaryJSONOmitsAbsentOptionalEvidence(t *testing.T) {
	summary := MonthSummary{
		Month: "2026-11", From: "2026-11-01", To: "2026-12-01",
		AsOf: "2026-11-19", Mode: ProjectionModeDay,
		Actual: Metrics{PoCount: 1, AmountMinor: 100, CostedMinor: 100, CapitalMinor: 50, MarginMinor: 50},
		Projection: MonthSummaryProjection{
			Status: ProjectionStatusUnavailable,
			Reason: ReasonInsufficientDays,
			Method: MethodCurrentDay,
		},
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{"month", "from", "to", "asOf", "mode", "actual", "projection"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("monthSummary JSON is missing %q", key)
		}
	}
	if _, ok := decoded["recordedFuture"]; ok {
		t.Error("absent recorded future evidence was serialized")
	}
	projection := decoded["projection"].(map[string]any)
	if projection["reason"] != ReasonInsufficientDays {
		t.Errorf("projection reason = %v; want %s", projection["reason"], ReasonInsufficientDays)
	}
	if _, ok := projection["remainder"]; ok {
		t.Error("absent projection remainder was serialized")
	}
}

func equalMonthMetrics(left, right Metrics) bool {
	if left.PoCount != right.PoCount || left.AmountMinor != right.AmountMinor ||
		left.CostedMinor != right.CostedMinor || left.UncostedMinor != right.UncostedMinor ||
		left.CapitalMinor != right.CapitalMinor || left.MarginMinor != right.MarginMinor {
		return false
	}
	return sameOptionalInt64(left.MarginBps, right.MarginBps) &&
		sameOptionalInt64(left.CoverageBps, right.CoverageBps)
}

func equalRemainder(left, right CurrentProjectionRemainder) bool {
	return left.LowMinor == right.LowMinor && left.BaseMinor == right.BaseMinor &&
		left.HighMinor == right.HighMinor && left.CapitalMinor == right.CapitalMinor &&
		left.MarginMinor == right.MarginMinor && left.SampleSize == right.SampleSize &&
		left.Source == right.Source && sameOptionalInt64(left.MarginBps, right.MarginBps)
}

func sameOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func showOptionalInt64(value *int64) string {
	if value == nil {
		return "null"
	}
	return new(big.Int).SetInt64(*value).String()
}
