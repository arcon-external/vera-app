package poreport

import (
	"testing"
	"time"
)

func dayOf(year int, month int, day int) string {
	return monthStart(year, month)[:8] + twoDigits(day)
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return string(rune('0'+value/10)) + string(rune('0'+value%10))
}

func hd(date string, poCount, amount, costed, capital int64) HistoryDay {
	return HistoryDay{
		Date: date,
		Metrics: Metrics{
			PoCount:       poCount,
			AmountMinor:   amount,
			CostedMinor:   costed,
			UncostedMinor: amount - costed,
			CapitalMinor:  capital,
		},
	}
}

// elapsedDays is one PO and 100 more amount each day, with capital at 10% of
// amount. Ten days have an average of 550 and daily quartiles of 225 and 775,
// so every assertion below has a distinct value.
func elapsedDays(year, month, count int) []HistoryDay {
	days := make([]HistoryDay, 0, count)
	for day := 1; day <= count; day++ {
		amount := int64(day * 100)
		days = append(days, hd(dayOf(year, month, day), 1, amount, amount, amount/10))
	}
	return days
}

func tenElapsedDays() []HistoryDay {
	return elapsedDays(2026, 11, 10)
}

func juneElapsedDays() []HistoryDay {
	return elapsedDays(2026, 6, 10)
}

func monthHistory() []HistoryMonth {
	return []HistoryMonth{
		hm("2026-01-01", 1, 1000, 1000, 100),
		hm("2026-02-01", 1, 2000, 2000, 200),
		hm("2026-03-01", 1, 3000, 3000, 300),
		hm("2026-04-01", 1, 4000, 4000, 400),
		hm("2026-05-01", 1, 5000, 5000, 500),
	}
}

func dayQuery() Query {
	return Query{From: "2026-11-01", To: "2026-12-01", Bucket: BucketDay, Category: CategoryAll, Currency: "IDR"}
}

func monthQuery() Query {
	return Query{From: "2026-01-01", To: "2027-01-01", Bucket: BucketMonth, Category: CategoryAll, Currency: "IDR"}
}

func TestProjectCurrentDayUsesElapsedDayAverageAndDailyBand(t *testing.T) {
	t.Parallel()

	projection := ProjectCurrent(nil, tenElapsedDays(), dayQuery(), at(2026, 11, 10), false, false)
	if projection.Mode != ProjectionModeDay || projection.Status != ProjectionStatusAvailable {
		t.Fatalf("current projection = %s/%s/%s; want day/available",
			projection.Mode, projection.Status, projection.Reason)
	}
	if projection.Basis.From != "2026-11-01" || projection.Basis.To != "2026-11-11" {
		t.Errorf("basis window = %s..%s; want 2026-11-01..2026-11-11",
			projection.Basis.From, projection.Basis.To)
	}
	if projection.Basis.Units != 10 || projection.Basis.PoCount != 10 ||
		projection.Basis.AsOf != "2026-11-10" {
		t.Errorf("basis = %+v; want 10 days, 10 POs, asOf 2026-11-10", projection.Basis)
	}
	if len(projection.Buckets) != 20 {
		t.Fatalf("buckets = %d; want the 20 remaining days of November", len(projection.Buckets))
	}
	first := projection.Buckets[0]
	if first.PeriodStart != "2026-11-11" {
		t.Errorf("first bucket = %s; want 2026-11-11", first.PeriodStart)
	}
	if first.AmountLowMinor != 325 || first.AmountBaseMinor != 550 || first.AmountHighMinor != 775 {
		t.Errorf("first band = %d/%d/%d; want 325/550/775",
			first.AmountLowMinor, first.AmountBaseMinor, first.AmountHighMinor)
	}
	if first.CapitalMinor != 55 || first.MarginMinor != 495 {
		t.Errorf("first capital/margin = %d/%d; want 55/495", first.CapitalMinor, first.MarginMinor)
	}
	if first.MarginBps == nil || *first.MarginBps != 90000 {
		t.Errorf("first marginBps = %v; want 90000", ratioValue(first.MarginBps))
	}
	if first.SampleSize != 10 || first.Source != SourceElapsedDays {
		t.Errorf("first sample/source = %d/%s; want 10/%s",
			first.SampleSize, first.Source, SourceElapsedDays)
	}
	if last := projection.Buckets[len(projection.Buckets)-1]; last.PeriodStart != "2026-11-30" {
		t.Errorf("last bucket = %s; want 2026-11-30", last.PeriodStart)
	}
	if projection.CurrentMonth != nil {
		t.Errorf("day mode carries currentMonth %+v; want nil", projection.CurrentMonth)
	}
}

func TestProjectCurrentDayBandUsesMinMaxUnderFourObservations(t *testing.T) {
	t.Parallel()

	low, high, err := dailyBand([]int64{100, 500, 300}, 300)
	if err != nil {
		t.Fatalf("dailyBand() error = %v", err)
	}
	if low != 100 || high != 500 {
		t.Errorf("dailyBand(3) = %d/%d; want min-max 100/500", low, high)
	}

	// Four observations switch to the interpolated quartiles.
	low, high, err = dailyBand([]int64{100, 200, 300, 400}, 250)
	if err != nil {
		t.Fatalf("dailyBand(4) error = %v", err)
	}
	if low != 175 || high != 325 {
		t.Errorf("dailyBand(4) = %d/%d; want 175/325", low, high)
	}
}

func TestProjectCurrentUsesWIBToday(t *testing.T) {
	t.Parallel()

	// 2026-12-04 20:00 UTC is 2026-12-05 03:00 in WIB. The current month is
	// therefore December, not November, and five days have elapsed.
	projection := ProjectCurrent(nil, elapsedDays(2026, 12, 10),
		Query{From: "2026-12-01", To: "2027-01-01", Bucket: BucketDay, Category: CategoryAll, Currency: "IDR"},
		time.Date(2026, 12, 4, 20, 0, 0, 0, time.UTC), false, false)
	if projection.Mode != ProjectionModeDay || projection.Status != ProjectionStatusAvailable {
		t.Fatalf("WIB current month = %s/%s/%s; want day/available",
			projection.Mode, projection.Status, projection.Reason)
	}
	if projection.Basis.AsOf != "2026-12-05" {
		t.Errorf("basis asOf = %s; want the WIB date 2026-12-05", projection.Basis.AsOf)
	}
}

func TestProjectCurrentOutOfRangeSampleIsUnavailable(t *testing.T) {
	t.Parallel()

	day := ProjectCurrent(nil, tenElapsedDays(), dayQuery(), at(2026, 11, 10), false, true)
	if day.Reason != ReasonValueOutOfRange || len(day.Buckets) != 0 {
		t.Errorf("overflowed day sample = %+v; want unavailable/%s with no buckets",
			day, ReasonValueOutOfRange)
	}
	month := ProjectCurrent(monthHistory(), juneElapsedDays(), monthQuery(), at(2026, 6, 10), true, false)
	if month.Reason != ReasonValueOutOfRange || month.CurrentMonth != nil {
		t.Errorf("overflowed month sample = %+v; want unavailable/%s without currentMonth",
			month, ReasonValueOutOfRange)
	}
}

func TestProjectCurrentDayGuardsElapsedDaysAndPurchaseOrders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		days   []HistoryDay
		day    int
		reason string
	}{
		{
			name: "four elapsed days",
			days: []HistoryDay{
				hd("2026-11-01", 1, 100, 100, 10),
				hd("2026-11-02", 1, 100, 100, 10),
				hd("2026-11-03", 1, 100, 100, 10),
				hd("2026-11-04", 1, 100, 100, 10),
			},
			day:    4,
			reason: ReasonInsufficientDays,
		},
		{
			name: "two purchase orders on five days",
			days: []HistoryDay{
				hd("2026-11-01", 1, 100, 100, 10),
				hd("2026-11-02", 1, 100, 100, 10),
			},
			day:    5,
			reason: ReasonInsufficientDays,
		},
		{
			name:   "no purchase orders",
			days:   nil,
			day:    19,
			reason: ReasonNoData,
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			projection := ProjectCurrent(nil, test.days, dayQuery(), at(2026, 11, test.day), false, false)
			if projection.Status != ProjectionStatusUnavailable ||
				projection.Reason != test.reason {
				t.Fatalf("projection = %s/%s; want unavailable/%s",
					projection.Status, projection.Reason, test.reason)
			}
			if len(projection.Buckets) != 0 {
				t.Errorf("buckets = %d; want none below the guard", len(projection.Buckets))
			}
		})
	}

	// The fifth day and the third order are what make it available.
	days := append(tenElapsedDays()[:3], hd("2026-11-04", 0, 0, 0, 0), hd("2026-11-05", 0, 0, 0, 0))
	projection := ProjectCurrent(nil, days, dayQuery(), at(2026, 11, 5), false, false)
	if projection.Status != ProjectionStatusAvailable {
		t.Fatalf("five days and three POs = %s/%s; want available",
			projection.Status, projection.Reason)
	}
}

func TestProjectCurrentMonthUsesTrailingHistoryAndDailyRemainder(t *testing.T) {
	t.Parallel()

	projection := ProjectCurrent(monthHistory(), juneElapsedDays(), monthQuery(), at(2026, 6, 10), false, false)
	if projection.Mode != ProjectionModeMonth || projection.Status != ProjectionStatusAvailable {
		t.Fatalf("current projection = %s/%s/%s; want month/available",
			projection.Mode, projection.Status, projection.Reason)
	}
	if projection.Basis.From != "2026-01-01" || projection.Basis.To != "2026-06-01" {
		t.Errorf("basis window = %s..%s; want 2026-01-01..2026-06-01",
			projection.Basis.From, projection.Basis.To)
	}
	if projection.Basis.Units != 5 || projection.Basis.PoCount != 5 {
		t.Errorf("basis = %+v; want five complete months and five POs", projection.Basis)
	}
	if len(projection.Buckets) != 6 {
		t.Fatalf("buckets = %d; want July through December", len(projection.Buckets))
	}
	first := projection.Buckets[0]
	if first.PeriodStart != "2026-07-01" {
		t.Errorf("first bucket = %s; want 2026-07-01", first.PeriodStart)
	}
	if first.AmountLowMinor != 2000 || first.AmountBaseMinor != 3000 || first.AmountHighMinor != 4000 {
		t.Errorf("first band = %d/%d/%d; want 2000/3000/4000",
			first.AmountLowMinor, first.AmountBaseMinor, first.AmountHighMinor)
	}
	if first.CapitalMinor != 300 || first.MarginMinor != 2700 ||
		first.MarginBps == nil || *first.MarginBps != 90000 {
		t.Errorf("first capital/margin/bps = %d/%d/%v; want 300/2700/90000",
			first.CapitalMinor, first.MarginMinor, ratioValue(first.MarginBps))
	}
	if first.SampleSize != 5 || first.Source != SourceTrailingCompleteMonths {
		t.Errorf("first sample/source = %d/%s; want 5/%s",
			first.SampleSize, first.Source, SourceTrailingCompleteMonths)
	}
	if projection.CurrentMonth == nil {
		t.Fatal("month mode has no currentMonth")
	}
	actual := projection.CurrentMonth.Actual
	if actual.PoCount != 10 || actual.AmountMinor != 5500 || actual.CostedMinor != 5500 ||
		actual.CapitalMinor != 550 || actual.MarginMinor != 4950 ||
		actual.MarginBps == nil || *actual.MarginBps != 90000 ||
		actual.CoverageBps == nil || *actual.CoverageBps != 10000 {
		t.Errorf("currentMonth actual = %+v; want month-to-date 10 POs at 5500/550/4950", actual)
	}
	remainder := projection.CurrentMonth.Remainder
	if remainder.LowMinor != 6500 || remainder.BaseMinor != 11000 || remainder.HighMinor != 15500 {
		t.Errorf("remainder = %d/%d/%d; want 6500/11000/15500",
			remainder.LowMinor, remainder.BaseMinor, remainder.HighMinor)
	}
	if remainder.CapitalMinor != 1100 || remainder.MarginMinor != 9900 ||
		remainder.MarginBps == nil || *remainder.MarginBps != 90000 {
		t.Errorf("remainder capital/margin/bps = %d/%d/%v; want 1100/9900/90000",
			remainder.CapitalMinor, remainder.MarginMinor, ratioValue(remainder.MarginBps))
	}
	if remainder.SampleSize != 10 || remainder.Source != SourceElapsedDays {
		t.Errorf("remainder sample/source = %d/%s; want 10/%s",
			remainder.SampleSize, remainder.Source, SourceElapsedDays)
	}
}

func TestProjectCurrentMonthGuardsBothSamples(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		history []HistoryMonth
		daily   []HistoryDay
		day     int
		reason  string
	}{
		{
			name:    "two complete months",
			history: monthHistory()[:2],
			daily:   juneElapsedDays(),
			day:     10,
			reason:  ReasonInsufficientMonths,
		},
		{
			name: "three months but four POs",
			history: []HistoryMonth{
				hm("2026-01-01", 1, 1000, 1000, 100),
				hm("2026-02-01", 1, 2000, 2000, 200),
				hm("2026-03-01", 2, 3000, 3000, 300),
			},
			daily:  juneElapsedDays(),
			day:    10,
			reason: ReasonInsufficientMonths,
		},
		{
			name:    "history passes but the daily sample is short",
			history: monthHistory(),
			daily:   juneElapsedDays(),
			day:     4,
			reason:  ReasonInsufficientDays,
		},
		{
			name:    "no history and no current month",
			history: nil,
			daily:   nil,
			day:     10,
			reason:  ReasonNoData,
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			projection := ProjectCurrent(test.history, test.daily, monthQuery(), at(2026, 6, test.day), false, false)
			if projection.Status != ProjectionStatusUnavailable ||
				projection.Reason != test.reason {
				t.Fatalf("projection = %s/%s; want unavailable/%s",
					projection.Status, projection.Reason, test.reason)
			}
			if len(projection.Buckets) != 0 || projection.CurrentMonth != nil {
				t.Errorf("unavailable projection carries buckets/currentMonth: %+v", projection)
			}
		})
	}
}

func TestProjectCurrentRecomputesWhenADayIsAdded(t *testing.T) {
	t.Parallel()

	before := ProjectCurrent(nil, tenElapsedDays(), dayQuery(), at(2026, 11, 10), false, false)
	if before.Status != ProjectionStatusAvailable || before.Buckets[0].AmountBaseMinor != 550 {
		t.Fatalf("before = %s/%d; want available at 550",
			before.Status, before.Buckets[0].AmountBaseMinor)
	}

	daily := append(tenElapsedDays(), hd("2026-11-11", 1, 1000, 1000, 100))
	after := ProjectCurrent(nil, daily, dayQuery(), at(2026, 11, 11), false, false)
	if after.Status != ProjectionStatusAvailable {
		t.Fatalf("after = %s/%s; want available", after.Status, after.Reason)
	}
	if after.Basis.Units != 11 || after.Basis.PoCount != 11 {
		t.Errorf("after basis = %+v; want 11 days and 11 POs", after.Basis)
	}
	if len(after.Buckets) != 19 {
		t.Errorf("after buckets = %d; want 19 remaining days", len(after.Buckets))
	}
	if after.Buckets[0].AmountBaseMinor != 591 {
		t.Errorf("after base = %d; want 6500/11 rounded to 591",
			after.Buckets[0].AmountBaseMinor)
	}
}

func TestProjectCurrentSuppressesIncompleteCost(t *testing.T) {
	t.Parallel()

	incompleteDay := tenElapsedDays()
	incompleteDay[4].CostedMinor = incompleteDay[4].AmountMinor - 1
	incompleteDay[4].UncostedMinor = 1

	incompleteHistory := monthHistory()
	incompleteHistory[2].CostedMinor = incompleteHistory[2].AmountMinor - 1
	incompleteHistory[2].UncostedMinor = 1

	incompleteJuneDay := juneElapsedDays()
	incompleteJuneDay[4].CostedMinor = incompleteJuneDay[4].AmountMinor - 1
	incompleteJuneDay[4].UncostedMinor = 1

	cases := []struct {
		name    string
		history []HistoryMonth
		daily   []HistoryDay
		query   Query
		month   int
		day     int
	}{
		{
			name:  "incomplete elapsed day",
			daily: incompleteDay,
			query: dayQuery(),
			month: 11,
			day:   10,
		},
		{
			name:    "incomplete completed month",
			history: incompleteHistory,
			daily:   juneElapsedDays(),
			query:   monthQuery(),
			month:   6,
			day:     10,
		},
		{
			name:    "incomplete current month in month mode",
			history: monthHistory(),
			daily:   incompleteJuneDay,
			query:   monthQuery(),
			month:   6,
			day:     10,
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			projection := ProjectCurrent(test.history, test.daily, test.query, at(2026, test.month, test.day), false, false)
			if projection.Status != ProjectionStatusUnavailable ||
				projection.Reason != ReasonIncompleteCostBasis {
				t.Fatalf("projection = %s/%s; want unavailable/%s",
					projection.Status, projection.Reason, ReasonIncompleteCostBasis)
			}
			if len(projection.Buckets) != 0 || projection.CurrentMonth != nil {
				t.Errorf("incomplete cost carries a margin-bearing projection: %+v", projection)
			}
		})
	}
}

// TestProjectCurrentDayTreatsAnExtraExpenseDayAsFullyCosted is the day-mode
// counterpart of the history gate: a day whose amount includes a pass-through
// extra has no missing cost, so it must not suppress the forecast.
func TestProjectCurrentDayTreatsAnExtraExpenseDayAsFullyCosted(t *testing.T) {
	t.Parallel()

	days := tenElapsedDays()
	days[4].AmountMinor += 1_000_000
	days[4].ExtraExpenseMinor = 1_000_000

	projection := ProjectCurrent(nil, days, dayQuery(), at(2026, 11, 10), false, false)
	if projection.Mode != ProjectionModeDay || projection.Status != ProjectionStatusAvailable {
		t.Fatalf("projection = %s/%s/%s; want day/available",
			projection.Mode, projection.Status, projection.Reason)
	}
	if len(projection.Buckets) != 20 {
		t.Errorf("buckets = %d; want the 20 remaining days", len(projection.Buckets))
	}

	// The same amount with no extra kind is genuinely uncosted revenue and keeps
	// the guard working.
	unpriced := tenElapsedDays()
	unpriced[4].AmountMinor += 1_000_000
	blocked := ProjectCurrent(nil, unpriced, dayQuery(), at(2026, 11, 10), false, false)
	if blocked.Status != ProjectionStatusUnavailable || blocked.Reason != ReasonIncompleteCostBasis {
		t.Errorf("unpriced day = %s/%s; want unavailable/%s",
			blocked.Status, blocked.Reason, ReasonIncompleteCostBasis)
	}
}

func TestProjectCurrentHistoricalRangeIsModeNone(t *testing.T) {
	t.Parallel()

	projection := ProjectCurrent(monthHistory(), tenElapsedDays(),
		Query{From: "2025-01-01", To: "2026-01-01", Bucket: BucketMonth, Category: CategoryAll, Currency: "IDR"},
		at(2026, 11, 19), false, false)
	if projection.Mode != ProjectionModeNone {
		t.Errorf("mode = %s; want none", projection.Mode)
	}
	if projection.Status != ProjectionStatusUnavailable || projection.Reason != ReasonNotCurrentPeriod {
		t.Errorf("projection = %s/%s; want unavailable/%s",
			projection.Status, projection.Reason, ReasonNotCurrentPeriod)
	}
	if len(projection.Buckets) != 0 || projection.CurrentMonth != nil {
		t.Errorf("mode none carries a forecast: %+v", projection)
	}
	if projection.Basis.AsOf != "2026-11-19" {
		t.Errorf("basis asOf = %s; want the WIB today", projection.Basis.AsOf)
	}
	if projection.Method == "" {
		t.Error("mode none has no method text")
	}
}

func TestProjectCurrentCurrentMonthBucketMonthIsModeNone(t *testing.T) {
	t.Parallel()

	query := Query{From: "2026-11-01", To: "2026-12-01", Bucket: BucketMonth, Category: CategoryAll, Currency: "IDR"}
	projection := ProjectCurrent(monthHistory(), tenElapsedDays(), query, at(2026, 11, 10), false, false)
	if projection.Mode != ProjectionModeNone || projection.Reason != ReasonNotCurrentPeriod {
		t.Fatalf("current month by month = %s/%s; want none/%s",
			projection.Mode, projection.Reason, ReasonNotCurrentPeriod)
	}
}

func TestProjectCurrentCurrentYearByQuarterIsMonthMode(t *testing.T) {
	t.Parallel()

	query := Query{From: "2026-01-01", To: "2027-01-01", Bucket: BucketQuarter, Category: CategoryAll, Currency: "IDR"}
	projection := ProjectCurrent(monthHistory(), juneElapsedDays(), query, at(2026, 6, 10), false, false)
	if projection.Mode != ProjectionModeMonth || projection.Status != ProjectionStatusAvailable {
		t.Fatalf("current year by quarter = %s/%s/%s; want month/available",
			projection.Mode, projection.Status, projection.Reason)
	}
}
