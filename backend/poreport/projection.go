package poreport

import (
	"errors"
	"math/big"
	"time"
)

// companyLocation is the company's commercial date context: WIB (Asia/Jakarta,
// UTC+7). The API server may run in UTC, but a report's "as of" month is the
// month the office is working in. There is no company timezone setting yet, so
// this fixed zone is the single source of truth until one exists.
var companyLocation = time.FixedZone("WIB", 7*60*60)

// Project builds the monthly and yearly forecast series from complete monthly
// history. It never fails the actual report: every branch returns a series with
// a status and, when unavailable, a stable reason. Actual figures are not part
// of this function and are never mixed into its buckets.
//
// The caller supplies history already filtered to the report's organization,
// currency and category. The window used is the complete months before both the
// report's To month and the as-of month in WIB. Zero months between the first
// observed historical PO month and that end are filled so seasonality sees the
// real gaps; months before the first observed PO are not invented. The first
// observed month is part of the complete window and is counted in CompleteMonths.
//
// An unavailable series states one reason, in this order: no_data_in_range when
// the actual population is empty; insufficient_history when the sample guard
// fails; incomplete_cost_basis when some history month's amount is not fully
// costed; value_out_of_range when an input or forecast cannot be named exactly.
// A forecast never becomes actual evidence and is never added to a total.
func Project(history []HistoryMonth, query Query, asOf time.Time, actualPoCount int64) Projection {
	normalized, err := NormalizeQuery(query)
	if err != nil {
		return Projection{
			Monthly: unavailableSeries(MethodMonthly, ReasonInvalidQuery, ProjectionBasis{AsOf: companyDateString(asOf)}),
			Yearly:  unavailableSeries(MethodYearly, ReasonInvalidQuery, ProjectionBasis{AsOf: companyDateString(asOf)}),
		}
	}

	sample := buildHistorySample(history, normalized, asOf)
	monthly := seriesForGuard(MethodMonthly, sample, 6)
	if monthly.Status == ProjectionStatusAvailable {
		buckets, err := monthlyBuckets(sample, normalized)
		if err != nil {
			monthly = unavailableSeries(MethodMonthly, projectionReason(err), sample.basis)
		} else {
			monthly.Buckets = buckets
		}
	}
	yearly := seriesForGuard(MethodYearly, sample, 12)
	if yearly.Status == ProjectionStatusAvailable {
		buckets, err := yearlyBuckets(sample, normalized)
		if err != nil {
			yearly = unavailableSeries(MethodYearly, projectionReason(err), sample.basis)
		} else {
			yearly.Buckets = buckets
		}
	}

	if actualPoCount <= 0 {
		monthly = unavailableSeries(MethodMonthly, ReasonNoDataInRange, sample.basis)
		yearly = unavailableSeries(MethodYearly, ReasonNoDataInRange, sample.basis)
		return Projection{Monthly: monthly, Yearly: yearly}
	}

	return Projection{Monthly: monthly, Yearly: yearly}
}

// monthlyBuckets builds the next twelve months. Each bucket reads the same
// calendar month from every complete history month; when that month has no
// history at all, it reads the last three complete months instead. Amount and
// capital share the same observations, and margin is derived from the two
// medians rather than forecast separately.
func monthlyBuckets(sample historySample, query Query) ([]ProjectedBucket, error) {
	to, err := parseReportDate(query.To)
	if err != nil {
		return nil, err
	}
	start := firstOfNextMonth(to.AddDate(0, 0, -1))
	buckets := make([]ProjectedBucket, 0, 12)
	for offset := 0; offset < 12; offset++ {
		target := start.AddDate(0, offset, 0)
		amounts := make([]int64, 0, len(sample.months))
		capitals := make([]int64, 0, len(sample.months))
		for _, month := range sample.months {
			if month.month.Month() != target.Month() {
				continue
			}
			amounts = append(amounts, month.metrics.AmountMinor)
			capitals = append(capitals, month.metrics.CapitalMinor)
		}

		source := SourceSameCalendarMonth
		if len(amounts) == 0 {
			source = SourceTrailingThreeCompleteMonths
			fallback := sample.months[len(sample.months)-3:]
			for _, month := range fallback {
				amounts = append(amounts, month.metrics.AmountMinor)
				capitals = append(capitals, month.metrics.CapitalMinor)
			}
		}

		bucket, err := projectedBucket(target, source, amounts, capitals)
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, nil
}

// projectedBucket holds the one shape both series use: an amount band from the
// observations, a capital base from the same window, and a derived signed
// margin. A passing sample is fully costed (costed == amount), so the amount
// base is also the costed amount base; margin is that base minus capital.
// Values are exact int64 and the band is ordered low <= base <= high.
func projectedBucket(period time.Time, source string, amounts, capitals []int64) (ProjectedBucket, error) {
	base, err := percentileInc(amounts, 1, 2)
	if err != nil {
		return ProjectedBucket{}, err
	}
	capital, err := percentileInc(capitals, 1, 2)
	if err != nil {
		return ProjectedBucket{}, err
	}
	low, high := minMax(amounts)
	if len(amounts) >= 4 {
		if low, err = percentileInc(amounts, 1, 4); err != nil {
			return ProjectedBucket{}, err
		}
		if high, err = percentileInc(amounts, 3, 4); err != nil {
			return ProjectedBucket{}, err
		}
	}
	if low > base {
		low = base
	}
	if high < base {
		high = base
	}
	return makeProjectedBucket(period, source, low, base, high, capital, len(amounts))
}

// makeProjectedBucket derives the single margin figure and its percentage from
// an already chosen amount base and capital base. The band is only ever on the
// amount; capital uncertainty is not invented.
func makeProjectedBucket(period time.Time, source string, low, base, high, capital int64, sampleSize int) (ProjectedBucket, error) {
	if err := CheckExact(low); err != nil {
		return ProjectedBucket{}, err
	}
	if err := CheckExact(base); err != nil {
		return ProjectedBucket{}, err
	}
	if err := CheckExact(high); err != nil {
		return ProjectedBucket{}, err
	}
	if err := CheckExact(capital); err != nil {
		return ProjectedBucket{}, err
	}

	margin, err := subtractExact(base, capital)
	if err != nil {
		return ProjectedBucket{}, err
	}
	marginBps, err := RatioBps(margin, capital)
	if err != nil {
		return ProjectedBucket{}, err
	}

	return ProjectedBucket{
		PeriodStart:     period.Format(reportDateLayout),
		AmountLowMinor:  low,
		AmountBaseMinor: base,
		AmountHighMinor: high,
		CapitalMinor:    capital,
		MarginMinor:     margin,
		MarginBps:       marginBps,
		SampleSize:      sampleSize,
		Source:          source,
	}, nil
}

// yearlyBuckets projects three calendar years from the trailing twelve complete
// months. The base is that trailing sum held flat; the band is the P25-P75 of
// every rolling twelve-month total, widened only when it would exclude the base.
// The capital base comes from the same trailing window, so margin is derived
// from a pair that saw the same months.
func yearlyBuckets(sample historySample, query Query) ([]ProjectedBucket, error) {
	if len(sample.months) < 12 {
		return nil, ErrInvalid
	}
	to, err := parseReportDate(query.To)
	if err != nil {
		return nil, err
	}
	firstYear := to.AddDate(0, 0, -1).Year() + 1

	var rollingAmounts, rollingCapitals []int64
	for start := 0; start+12 <= len(sample.months); start++ {
		amount := new(big.Int)
		capital := new(big.Int)
		for _, month := range sample.months[start : start+12] {
			amount.Add(amount, big.NewInt(month.metrics.AmountMinor))
			capital.Add(capital, big.NewInt(month.metrics.CapitalMinor))
		}
		exactAmount, err := exactInt64(amount)
		if err != nil {
			return nil, err
		}
		exactCapital, err := exactInt64(capital)
		if err != nil {
			return nil, err
		}
		rollingAmounts = append(rollingAmounts, exactAmount)
		rollingCapitals = append(rollingCapitals, exactCapital)
	}

	low, err := percentileInc(rollingAmounts, 1, 4)
	if err != nil {
		return nil, err
	}
	high, err := percentileInc(rollingAmounts, 3, 4)
	if err != nil {
		return nil, err
	}
	base := rollingAmounts[len(rollingAmounts)-1]
	capital := rollingCapitals[len(rollingCapitals)-1]
	if low > base {
		low = base
	}
	if high < base {
		high = base
	}

	buckets := make([]ProjectedBucket, 0, 3)
	for offset := 0; offset < 3; offset++ {
		bucket, err := makeProjectedBucket(
			time.Date(firstYear+offset, time.January, 1, 0, 0, 0, 0, time.UTC),
			SourceTrailingTwelveCompleteMonths, low, base, high, capital, 12)
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, nil
}

func minMax(values []int64) (int64, int64) {
	low, high := values[0], values[0]
	for _, value := range values[1:] {
		if value < low {
			low = value
		}
		if value > high {
			high = value
		}
	}
	return low, high
}

func subtractExact(left, right int64) (int64, error) {
	return exactInt64(new(big.Int).Sub(big.NewInt(left), big.NewInt(right)))
}

func projectionReason(err error) string {
	if errors.Is(err, ErrValueOutOfRange) {
		return ReasonValueOutOfRange
	}
	return ReasonInsufficientHistory
}

// historySample is the complete-month window every forecast reads. basis is
// shared by both series so the screen shows one sample window.
type historySample struct {
	basis      ProjectionBasis
	months     []completeMonth
	incomplete bool
	overflow   bool
}

// completeMonth is one observed or zero-filled calendar month, already converted
// to exact int64 metrics. An overflow in the source keeps the month position but
// invalidates the forecast, never the actual report.
type completeMonth struct {
	month   time.Time
	metrics Metrics
}

type aggregateMonth struct {
	poCount  *big.Int
	amount   *big.Int
	costed   *big.Int
	uncosted *big.Int
	capital  *big.Int
}

func buildHistorySample(history []HistoryMonth, query Query, asOf time.Time) historySample {
	asOfDate := companyDate(asOf)
	to, _ := parseReportDate(query.To) // NormalizeQuery already proved it.
	historyEnd := firstOfMonth(to)
	if asOfMonth := firstOfMonth(asOfDate); asOfMonth.Before(historyEnd) {
		historyEnd = asOfMonth
	}

	aggregated := make(map[time.Time]*aggregateMonth)
	var firstObserved time.Time
	for _, entry := range history {
		month, err := time.Parse(reportDateLayout, entry.MonthStart)
		if err != nil || month.Day() != 1 || month.Format(reportDateLayout) != entry.MonthStart {
			continue
		}
		month = month.UTC()
		if !month.Before(historyEnd) {
			continue
		}
		current := aggregated[month]
		if current == nil {
			current = &aggregateMonth{
				poCount: new(big.Int), amount: new(big.Int), costed: new(big.Int),
				uncosted: new(big.Int), capital: new(big.Int),
			}
			aggregated[month] = current
		}
		current.poCount.Add(current.poCount, big.NewInt(entry.PoCount))
		current.amount.Add(current.amount, big.NewInt(entry.AmountMinor))
		current.costed.Add(current.costed, big.NewInt(entry.CostedMinor))
		current.uncosted.Add(current.uncosted, big.NewInt(entry.UncostedMinor))
		current.capital.Add(current.capital, big.NewInt(entry.CapitalMinor))

		if isObserved(current) && (firstObserved.IsZero() || month.Before(firstObserved)) {
			firstObserved = month
		}
	}

	basis := ProjectionBasis{AsOf: asOfDate.Format(reportDateLayout)}
	if firstObserved.IsZero() || !firstObserved.Before(historyEnd) {
		return historySample{basis: basis}
	}

	basis.HistoryFrom = firstObserved.Format(reportDateLayout)
	basis.HistoryTo = historyEnd.Format(reportDateLayout)

	var months []completeMonth
	poCount := new(big.Int)
	incomplete := false
	overflow := false
	for month := firstObserved; month.Before(historyEnd); month = firstOfNextMonth(month) {
		metrics := Metrics{}
		if current := aggregated[month]; current != nil {
			metrics = Metrics{
				PoCount:       bigIntValue(current.poCount),
				AmountMinor:   bigIntValue(current.amount),
				CostedMinor:   bigIntValue(current.costed),
				UncostedMinor: bigIntValue(current.uncosted),
				CapitalMinor:  bigIntValue(current.capital),
			}
			if _, err := exactInt64(current.poCount); err != nil {
				overflow = true
			} else if _, err := exactInt64(current.amount); err != nil {
				overflow = true
			} else if _, err := exactInt64(current.costed); err != nil {
				overflow = true
			} else if _, err := exactInt64(current.uncosted); err != nil {
				overflow = true
			} else if _, err := exactInt64(current.capital); err != nil {
				overflow = true
			}
			if current.costed.Cmp(current.amount) < 0 || current.uncosted.Sign() > 0 {
				incomplete = true
			}
			poCount.Add(poCount, current.poCount)
		}
		months = append(months, completeMonth{month: month, metrics: metrics})
	}

	basis.CompleteMonths = len(months)
	if count, err := exactInt64(poCount); err == nil {
		basis.PoCount = count
	} else {
		overflow = true
	}

	return historySample{
		basis:      basis,
		months:     months,
		incomplete: incomplete,
		overflow:   overflow,
	}
}

func isObserved(month *aggregateMonth) bool {
	return month.poCount.Sign() != 0 ||
		month.amount.Sign() != 0 ||
		month.costed.Sign() != 0 ||
		month.uncosted.Sign() != 0 ||
		month.capital.Sign() != 0
}

// bigIntValue converts an aggregate to int64 for storage. Any value outside
// int64 is already flagged as overflow by the caller; it cannot be represented
// in the DTO, and the whole series becomes unavailable rather than losing a
// digit silently.
func bigIntValue(value *big.Int) int64 {
	if !value.IsInt64() {
		return 0
	}
	return value.Int64()
}

// seriesForGuard applies the sample-size guard and, for a passing sample,
// leaves the series for the monthly or yearly builder. The builders replace the
// empty placeholder before Project returns.
func seriesForGuard(method string, sample historySample, minimumMonths int) ProjectionSeries {
	series := ProjectionSeries{
		Status:  ProjectionStatusUnavailable,
		Reason:  ReasonInsufficientHistory,
		Method:  method,
		Basis:   sample.basis,
		Buckets: []ProjectedBucket{},
	}
	if sample.basis.CompleteMonths < minimumMonths || sample.basis.PoCount < 6 {
		return series
	}
	if sample.incomplete {
		series.Reason = ReasonIncompleteCostBasis
		return series
	}
	if sample.overflow {
		series.Reason = ReasonValueOutOfRange
		return series
	}
	series.Status = ProjectionStatusAvailable
	series.Reason = ""
	return series
}

func unavailableSeries(method, reason string, basis ProjectionBasis) ProjectionSeries {
	return ProjectionSeries{
		Status:  ProjectionStatusUnavailable,
		Reason:  reason,
		Method:  method,
		Basis:   basis,
		Buckets: []ProjectedBucket{},
	}
}

func companyDate(value time.Time) time.Time {
	local := value.In(companyLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func companyDateString(value time.Time) string {
	return companyDate(value).Format(reportDateLayout)
}
