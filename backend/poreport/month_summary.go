package poreport

import (
	"errors"
	"fmt"
	"math/big"
	"time"
)

// BuildMonthSummary derives the current calendar month's readout from the same
// report snapshot the API already holds. It is the one place the actual-to-date,
// recorded future-dated and projected-remainder quantities are separated; the
// screen and the PDF select these values and never recompute them.
//
// The summary exists only for a query that is exactly the current month bucketed
// by day, or exactly the current year bucketed by month. Any other valid shape,
// and a corrupt daily sample, yields nil: a readout invented from a different
// scope would be a second report, not this one. A query that cannot be
// normalized is refused with an error instead, so a corrupt filter never reads
// as "no summary".
//
// Actual comes from the elapsed-day sample, so it excludes a future-dated saved
// order even though the report's Totals include it. RecordedFuture carries that
// order separately. Projection is copied from the current-period forecast; a
// failed guard leaves Actual intact. In day mode the remainder must foot to the
// supplied rounded daily ghost buckets exactly, so the table and the chart can
// never disagree; in month mode the existing CurrentMonth remainder is reused.
func BuildMonthSummary(
	periods []Period,
	daily []HistoryDay,
	current CurrentProjection,
	query Query,
	asOf time.Time,
	dailyOutOfRange bool,
) (*MonthSummary, error) {
	if dailyOutOfRange {
		return nil, nil
	}
	normalized, err := NormalizeQuery(query)
	if err != nil {
		return nil, fmt.Errorf("current month summary query: %w", err)
	}
	asOfDate := companyDate(asOf)
	monthStart := firstOfMonth(asOfDate)
	monthEnd := firstOfNextMonth(asOfDate)
	if !summaryScopeMatches(normalized, current.Mode, asOfDate, monthStart) {
		return nil, nil
	}

	day := buildDaySample(daily, asOfDate)
	if day.elapsed <= 0 {
		return nil, nil
	}
	actual, err := currentMetrics(day.poCount, day.amount, day.costed, day.capital, day.extra)
	if err != nil {
		return nil, fmt.Errorf("current month actual: %w", err)
	}

	var recorded *Metrics
	if current.Mode == ProjectionModeDay {
		recorded, err = futureDayMetrics(periods, asOfDate, monthStart, monthEnd)
	} else {
		recorded, err = recordedDifference(periods, monthStart, actual)
	}
	if err != nil {
		return nil, err
	}

	projection := MonthSummaryProjection{
		Status: current.Status,
		Reason: current.Reason,
		Method: current.Method,
	}
	switch current.Status {
	case ProjectionStatusAvailable:
		if current.Mode == ProjectionModeDay {
			remainder, err := sumDailyBuckets(current)
			switch {
			case errors.Is(err, ErrValueOutOfRange):
				// The buckets are individually exact but their rounded total is
				// not. The actual figures remain printable; only the forecast
				// total is refused rather than emitted rounded.
				projection = MonthSummaryProjection{
					Status: ProjectionStatusUnavailable,
					Reason: ReasonValueOutOfRange,
					Method: current.Method,
				}
			case err != nil:
				return nil, err
			default:
				projection.Remainder = remainder
			}
		} else {
			if current.CurrentMonth == nil {
				return nil, fmt.Errorf("%w: month projection is available without a current month", ErrInvalid)
			}
			remainder := current.CurrentMonth.Remainder
			projection.Remainder = &remainder
		}
	case ProjectionStatusUnavailable:
		// The reason is copied verbatim; the UI owns the human sentence.
	default:
		return nil, fmt.Errorf("%w: current projection status %q", ErrInvalid, current.Status)
	}

	return &MonthSummary{
		Month:          monthStart.Format("2006-01"),
		From:           monthStart.Format(reportDateLayout),
		To:             monthEnd.Format(reportDateLayout),
		AsOf:           asOfDate.Format(reportDateLayout),
		Mode:           current.Mode,
		Actual:         actual,
		RecordedFuture: recorded,
		Projection:     projection,
	}, nil
}

// summaryScopeMatches names the only two report shapes that earn a month
// summary. The projection mode is the domain's own statement that the query is
// exactly current; the bucket check keeps a current-year quarter/year report
// from pretending it has a monthly row, and a current-month month-bucket report
// (deliberately mode none) from doing the same.
func summaryScopeMatches(query Query, mode string, asOfDate, monthStart time.Time) bool {
	if mode == ProjectionModeDay {
		return query.Bucket == BucketDay &&
			query.From == monthStart.Format(reportDateLayout) &&
			query.To == firstOfNextMonth(asOfDate).Format(reportDateLayout)
	}
	if mode == ProjectionModeMonth {
		yearStart := time.Date(asOfDate.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
		return query.Bucket == BucketMonth &&
			query.From == yearStart.Format(reportDateLayout) &&
			query.To == yearStart.AddDate(1, 0, 0).Format(reportDateLayout)
	}
	return false
}

// futureDayMetrics sums the supplied day rows after the as-of day and inside
// the current month. A day-bucketed report materializes the whole month, so
// these rows are the recorded future-dated evidence. Only the core figures are
// summed; the ratios are derived once from the sums, never averaged.
func futureDayMetrics(periods []Period, asOfDate, monthStart, monthEnd time.Time) (*Metrics, error) {
	var count, amount, costed, extra, capital int64
	has := false
	for _, period := range periods {
		start, err := time.Parse(reportDateLayout, period.PeriodStart)
		if err != nil || start.Format(reportDateLayout) != period.PeriodStart {
			return nil, fmt.Errorf("%w: period start %q", ErrInvalid, period.PeriodStart)
		}
		if !start.After(asOfDate) || start.Before(monthStart) || !start.Before(monthEnd) {
			continue
		}
		if period.PoCount == 0 && period.AmountMinor == 0 &&
			period.CostedMinor == 0 && period.ExtraExpenseMinor == 0 &&
			period.CapitalMinor == 0 {
			continue
		}
		has = true
		if count, err = addExact(count, period.PoCount); err != nil {
			return nil, fmt.Errorf("recorded future PO count: %w", err)
		}
		if amount, err = addExact(amount, period.AmountMinor); err != nil {
			return nil, fmt.Errorf("recorded future amount: %w", err)
		}
		if costed, err = addExact(costed, period.CostedMinor); err != nil {
			return nil, fmt.Errorf("recorded future costed amount: %w", err)
		}
		if extra, err = addExact(extra, period.ExtraExpenseMinor); err != nil {
			return nil, fmt.Errorf("recorded future extra expense: %w", err)
		}
		if capital, err = addExact(capital, period.CapitalMinor); err != nil {
			return nil, fmt.Errorf("recorded future capital: %w", err)
		}
	}
	if !has {
		return nil, nil
	}
	metrics, err := currentMetrics(count, amount, costed, capital, extra)
	if err != nil {
		return nil, fmt.Errorf("recorded future metrics: %w", err)
	}
	return &metrics, nil
}

// recordedDifference is the month-mode future-dated evidence: the supplied
// current-month row is the whole month, while Actual is only the elapsed part.
// The difference is computed field by field with checked subtraction, then the
// ratios are derived from those differences. It is omitted when every
// difference is zero.
func recordedDifference(periods []Period, monthStart time.Time, actual Metrics) (*Metrics, error) {
	start := monthStart.Format(reportDateLayout)
	var raw *Metrics
	for index := range periods {
		if periods[index].PeriodStart == start {
			metrics := periods[index].Metrics
			raw = &metrics
			break
		}
	}
	if raw == nil {
		return nil, nil
	}
	count, err := subtractExact(raw.PoCount, actual.PoCount)
	if err != nil {
		return nil, fmt.Errorf("recorded future PO count: %w", err)
	}
	amount, err := subtractExact(raw.AmountMinor, actual.AmountMinor)
	if err != nil {
		return nil, fmt.Errorf("recorded future amount: %w", err)
	}
	costed, err := subtractExact(raw.CostedMinor, actual.CostedMinor)
	if err != nil {
		return nil, fmt.Errorf("recorded future costed amount: %w", err)
	}
	extra, err := subtractExact(raw.ExtraExpenseMinor, actual.ExtraExpenseMinor)
	if err != nil {
		return nil, fmt.Errorf("recorded future extra expense: %w", err)
	}
	capital, err := subtractExact(raw.CapitalMinor, actual.CapitalMinor)
	if err != nil {
		return nil, fmt.Errorf("recorded future capital: %w", err)
	}
	if count == 0 && amount == 0 && costed == 0 && extra == 0 && capital == 0 {
		return nil, nil
	}
	metrics, err := currentMetrics(count, amount, costed, capital, extra)
	if err != nil {
		return nil, fmt.Errorf("recorded future metrics: %w", err)
	}
	return &metrics, nil
}

// sumDailyBuckets foots the supplied rounded daily ghost buckets into the one
// remainder total the table prints. All arithmetic is checked; a total outside
// the exact JSON range is refused rather than rounded.
func sumDailyBuckets(current CurrentProjection) (*CurrentProjectionRemainder, error) {
	var low, base, high, capital, margin int64
	for _, bucket := range current.Buckets {
		var err error
		if low, err = addExact(low, bucket.AmountLowMinor); err != nil {
			return nil, fmt.Errorf("remainder low: %w", err)
		}
		if base, err = addExact(base, bucket.AmountBaseMinor); err != nil {
			return nil, fmt.Errorf("remainder base: %w", err)
		}
		if high, err = addExact(high, bucket.AmountHighMinor); err != nil {
			return nil, fmt.Errorf("remainder high: %w", err)
		}
		if capital, err = addExact(capital, bucket.CapitalMinor); err != nil {
			return nil, fmt.Errorf("remainder capital: %w", err)
		}
		if margin, err = addExact(margin, bucket.MarginMinor); err != nil {
			return nil, fmt.Errorf("remainder margin: %w", err)
		}
	}
	marginBps, err := RatioBps(margin, capital)
	if err != nil {
		return nil, fmt.Errorf("remainder margin percentage: %w", err)
	}
	return &CurrentProjectionRemainder{
		LowMinor:     low,
		BaseMinor:    base,
		HighMinor:    high,
		CapitalMinor: capital,
		MarginMinor:  margin,
		MarginBps:    marginBps,
		SampleSize:   current.Basis.Units,
		Source:       SourceElapsedDays,
	}, nil
}

// addExact adds two exact-report integers without wrapping. The caller's values
// are each inside the JavaScript-exact range; the sum may not be, and that is
// exactly what this refuses.
func addExact(left, right int64) (int64, error) {
	return exactInt64(new(big.Int).Add(big.NewInt(left), big.NewInt(right)))
}
