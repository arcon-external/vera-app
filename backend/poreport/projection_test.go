package poreport

import (
	"testing"
	"time"
)

func TestProjectEmptyActualPopulationIsNoData(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 12)
	for month := 1; month <= 12; month++ {
		history = append(history, hm(monthStart(2025, month), 1, 100, 100, 10))
	}
	query := mustQuery(t, Query{
		From: "2026-01-01", To: "2026-07-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2026, 7, 15), 0)

	for name, series := range map[string]ProjectionSeries{
		"monthly": projection.Monthly,
		"yearly":  projection.Yearly,
	} {
		if series.Status != ProjectionStatusUnavailable || series.Reason != ReasonNoDataInRange {
			t.Errorf("%s = %s/%s; want unavailable/%s", name, series.Status, series.Reason, ReasonNoDataInRange)
		}
		if series.Buckets == nil {
			t.Errorf("%s buckets = nil; want an empty slice", name)
		}
		if len(series.Buckets) != 0 {
			t.Errorf("%s buckets = %d; want 0", name, len(series.Buckets))
		}
		if series.Basis.CompleteMonths != 18 {
			t.Errorf("%s basis completeMonths = %d; want 18 (through the last complete month)", name, series.Basis.CompleteMonths)
		}
		if series.Basis.HistoryFrom != "2025-01-01" || series.Basis.HistoryTo != "2026-07-01" {
			t.Errorf("%s basis history = %s..%s; want 2025-01-01..2026-07-01",
				name, series.Basis.HistoryFrom, series.Basis.HistoryTo)
		}
		if series.Basis.PoCount != 12 {
			t.Errorf("%s basis poCount = %d; want 12", name, series.Basis.PoCount)
		}
		if series.Basis.AsOf != "2026-07-15" {
			t.Errorf("%s basis asOf = %s; want 2026-07-15", name, series.Basis.AsOf)
		}
		if series.Method == "" {
			t.Errorf("%s method is empty; want the stated method", name)
		}
	}
}

func TestProjectInsufficientCompleteMonths(t *testing.T) {
	t.Parallel()

	history := []HistoryMonth{
		hm("2026-01-01", 1, 100, 100, 10),
		hm("2026-02-01", 1, 100, 100, 10),
		hm("2026-03-01", 1, 100, 100, 10),
		hm("2026-04-01", 1, 100, 100, 10),
		hm("2026-05-01", 1, 100, 100, 10),
	}
	query := mustQuery(t, Query{
		From: "2026-01-01", To: "2026-06-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2026, 6, 15), 5)

	for name, series := range map[string]ProjectionSeries{
		"monthly": projection.Monthly,
		"yearly":  projection.Yearly,
	} {
		if series.Status != ProjectionStatusUnavailable || series.Reason != ReasonInsufficientHistory {
			t.Errorf("%s = %s/%s; want unavailable/%s", name, series.Status, series.Reason, ReasonInsufficientHistory)
		}
		if series.Basis.CompleteMonths != 5 || series.Basis.PoCount != 5 {
			t.Errorf("%s basis = %d months / %d POs; want 5 / 5",
				name, series.Basis.CompleteMonths, series.Basis.PoCount)
		}
	}
}

func TestProjectInsufficientPurchaseOrders(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 6)
	history = append(history, hm("2026-01-01", 5, 500, 500, 50))
	for month := 2; month <= 6; month++ {
		history = append(history, hm(monthStart(2026, month), 0, 0, 0, 0))
	}
	query := mustQuery(t, Query{
		From: "2026-01-01", To: "2026-07-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	projection := Project(history, query, at(2026, 7, 15), 5)

	for name, series := range map[string]ProjectionSeries{
		"monthly": projection.Monthly,
		"yearly":  projection.Yearly,
	} {
		if series.Status != ProjectionStatusUnavailable || series.Reason != ReasonInsufficientHistory {
			t.Errorf("%s = %s/%s; want unavailable/%s", name, series.Status, series.Reason, ReasonInsufficientHistory)
		}
		if series.Basis.CompleteMonths != 6 || series.Basis.PoCount != 5 {
			t.Errorf("%s basis = %d months / %d POs; want 6 / 5",
				name, series.Basis.CompleteMonths, series.Basis.PoCount)
		}
	}
}

func TestProjectUsesWIBForTheAsOfMonth(t *testing.T) {
	t.Parallel()

	history := make([]HistoryMonth, 0, 8)
	for month := 1; month <= 8; month++ {
		history = append(history, hm(monthStart(2026, month), 1, 100, 100, 10))
	}
	query := mustQuery(t, Query{
		From: "2026-01-01", To: "2027-01-01", Bucket: BucketMonth,
		Category: CategoryAll, Currency: "IDR",
	})

	tests := []struct {
		name      string
		asOf      time.Time
		complete  int
		historyTo string
		asOfDate  string
	}{
		{
			name:      "half past midnight in Jakarta is already September",
			asOf:      time.Date(2026, 8, 31, 17, 30, 0, 0, time.UTC),
			complete:  8,
			historyTo: "2026-09-01",
			asOfDate:  "2026-09-01",
		},
		{
			name:      "one minute before September in Jakarta is still August",
			asOf:      time.Date(2026, 8, 31, 16, 59, 0, 0, time.UTC),
			complete:  7,
			historyTo: "2026-08-01",
			asOfDate:  "2026-08-31",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			projection := Project(history, query, test.asOf, 8)
			for name, series := range map[string]ProjectionSeries{
				"monthly": projection.Monthly,
				"yearly":  projection.Yearly,
			} {
				if series.Basis.CompleteMonths != test.complete {
					t.Errorf("%s completeMonths = %d; want %d",
						name, series.Basis.CompleteMonths, test.complete)
				}
				if series.Basis.HistoryTo != test.historyTo {
					t.Errorf("%s historyTo = %s; want %s",
						name, series.Basis.HistoryTo, test.historyTo)
				}
			}
			if projection.Monthly.Basis.AsOf != test.asOfDate {
				t.Errorf("asOf = %s; want %s", projection.Monthly.Basis.AsOf, test.asOfDate)
			}
		})
	}
}

func TestProjectInvalidQueryReturnsReasonedUnavailableSeries(t *testing.T) {
	t.Parallel()

	projection := Project(nil, Query{From: "nope", To: "also-nope", Bucket: "week"}, at(2026, 7, 15), 1)

	for name, series := range map[string]ProjectionSeries{
		"monthly": projection.Monthly,
		"yearly":  projection.Yearly,
	} {
		if series.Status != ProjectionStatusUnavailable || series.Reason != ReasonInvalidQuery {
			t.Errorf("%s = %s/%s; want unavailable/%s", name, series.Status, series.Reason, ReasonInvalidQuery)
		}
		if len(series.Buckets) != 0 {
			t.Errorf("%s buckets = %d; want 0", name, len(series.Buckets))
		}
	}
}

func hm(month string, poCount, amount, costed, capital int64) HistoryMonth {
	return HistoryMonth{
		MonthStart: month,
		Metrics: Metrics{
			PoCount:       poCount,
			AmountMinor:   amount,
			CostedMinor:   costed,
			UncostedMinor: amount - costed,
			CapitalMinor:  capital,
			MarginMinor:   costed - capital,
		},
	}
}

func monthStart(year, month int) string {
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Format(reportDateLayout)
}

func at(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 12, 0, 0, 0, time.UTC)
}

func mustQuery(t *testing.T, query Query) Query {
	t.Helper()
	normalized, err := NormalizeQuery(query)
	if err != nil {
		t.Fatalf("NormalizeQuery(%+v) error = %v", query, err)
	}
	return normalized
}
