package poreport

import (
	"fmt"
	"math/big"
	"time"
)

// Current-period projection guards. They are deliberately separate from the
// complete-history guards in Project: a current-period forecast is allowed with
// less history, but it needs enough elapsed days (or complete months) that one
// unusual order cannot set the whole base.
const (
	MinCurrentDayElapsedDays = 5
	MinCurrentDayPoCount     = 3
	MinCurrentMonthComplete  = 3
	MinCurrentMonthPoCount   = 5
)

// ProjectCurrent builds the current-period forecast for a report that is
// exactly the current calendar month (day bucket) or exactly the current
// calendar year (any non-day bucket). Any other range is mode none with
// ReasonNotCurrentPeriod; no trend is inferred for a range that is not the
// period still in progress.
//
// Day mode uses the elapsed days of the current month. The base is the
// elapsed-day amount total divided by the elapsed days, and the remaining days
// carry that base with the daily P25-P75 band (min-max under four elapsed
// days). Month mode uses the complete months in trailing history for the
// remaining months, and the current month's remainder uses the same daily rule.
// The current month's month-to-date actuals are returned beside that remainder.
//
// Both samples must pass their own guard. In month mode a failing daily sample
// makes the whole current projection unavailable: there is no truthful
// remainder to report, and a partial object would invite the screen to draw
// one. Incomplete cost suppresses any margin-bearing result with
// ReasonIncompleteCostBasis; margin is never invented from uncosted revenue.
// No bucket or remainder is ever added to Totals or Periods.
func ProjectCurrent(
	history []HistoryMonth,
	daily []HistoryDay,
	query Query,
	asOf time.Time,
	historyOutOfRange bool,
	dailyOutOfRange bool,
) CurrentProjection {
	normalized, err := NormalizeQuery(query)
	if err != nil {
		return unavailableCurrent(ProjectionModeNone, ReasonInvalidQuery, MethodCurrentNone,
			CurrentProjectionBasis{AsOf: companyDateString(asOf)})
	}
	asOfDate := companyDate(asOf)
	mode := currentProjectionMode(normalized, asOfDate)
	if mode == ProjectionModeNone {
		return unavailableCurrent(mode, ReasonNotCurrentPeriod, MethodCurrentNone,
			CurrentProjectionBasis{AsOf: asOfDate.Format(reportDateLayout)})
	}
	if mode == ProjectionModeDay {
		if dailyOutOfRange {
			return unavailableCurrent(mode, ReasonValueOutOfRange, MethodCurrentDay,
				CurrentProjectionBasis{AsOf: asOfDate.Format(reportDateLayout)})
		}
		return projectCurrentDay(daily, asOfDate)
	}
	if historyOutOfRange || dailyOutOfRange {
		return unavailableCurrent(mode, ReasonValueOutOfRange, MethodCurrentMonth,
			CurrentProjectionBasis{AsOf: asOfDate.Format(reportDateLayout)})
	}
	return projectCurrentMonth(history, daily, normalized, asOfDate)
}

// UnavailableCurrentProjection states one reason for a current projection whose
// input could not be represented exactly. The actual report is never suppressed
// by this; only the forecast is.
func UnavailableCurrentProjection(mode, reason string, asOf time.Time) CurrentProjection {
	method := MethodCurrentNone
	switch mode {
	case ProjectionModeDay:
		method = MethodCurrentDay
	case ProjectionModeMonth:
		method = MethodCurrentMonth
	}
	return unavailableCurrent(mode, reason, method,
		CurrentProjectionBasis{AsOf: companyDateString(asOf)})
}

// currentProjectionMode names the only two ranges that earn a current-period
// forecast. A day-bucketed report must be the whole current month; a non-day
// report must be the whole current year. A current-month report bucketed by
// month is deliberately not month mode: the day rule is the one the contract
// asks for at that range.
func currentProjectionMode(query Query, asOfDate time.Time) string {
	from, err := parseReportDate(query.From)
	if err != nil {
		return ProjectionModeNone
	}
	to, err := parseReportDate(query.To)
	if err != nil {
		return ProjectionModeNone
	}
	if query.Bucket == BucketDay {
		if from.Equal(firstOfMonth(asOfDate)) && to.Equal(firstOfNextMonth(asOfDate)) {
			return ProjectionModeDay
		}
		return ProjectionModeNone
	}
	yearStart := time.Date(asOfDate.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := yearStart.AddDate(1, 0, 0)
	if from.Equal(yearStart) && to.Equal(yearEnd) {
		return ProjectionModeMonth
	}
	return ProjectionModeNone
}

// projectCurrentDay forecasts the remaining days of the month from the elapsed
// days. Every elapsed calendar day is an observation, including a day with no
// order, because the base is an average over calendar days.
func projectCurrentDay(daily []HistoryDay, asOfDate time.Time) CurrentProjection {
	sample := buildDaySample(daily, asOfDate)
	basis := CurrentProjectionBasis{
		From:    firstOfMonth(asOfDate).Format(reportDateLayout),
		To:      asOfDate.AddDate(0, 0, 1).Format(reportDateLayout),
		Units:   sample.elapsed,
		PoCount: sample.poCount,
		AsOf:    asOfDate.Format(reportDateLayout),
	}
	if sample.elapsed < MinCurrentDayElapsedDays || sample.poCount < MinCurrentDayPoCount {
		return unavailableCurrent(ProjectionModeDay, ReasonInsufficientHistory, MethodCurrentDay, basis)
	}
	if sample.incomplete {
		return unavailableCurrent(ProjectionModeDay, ReasonIncompleteCostBasis, MethodCurrentDay, basis)
	}
	if sample.overflow {
		return unavailableCurrent(ProjectionModeDay, ReasonValueOutOfRange, MethodCurrentDay, basis)
	}

	base, err := averageExact(sample.amount, sample.elapsed)
	if err != nil {
		return unavailableCurrent(ProjectionModeDay, projectionReason(err), MethodCurrentDay, basis)
	}
	capital, err := averageExact(sample.capital, sample.elapsed)
	if err != nil {
		return unavailableCurrent(ProjectionModeDay, projectionReason(err), MethodCurrentDay, basis)
	}
	low, high, err := dailyBand(sample.amounts, base)
	if err != nil {
		return unavailableCurrent(ProjectionModeDay, projectionReason(err), MethodCurrentDay, basis)
	}

	buckets := make([]ProjectedBucket, 0, daysInMonth(asOfDate)-sample.elapsed)
	for day := asOfDate.AddDate(0, 0, 1); day.Before(firstOfNextMonth(asOfDate)); day = day.AddDate(0, 0, 1) {
		bucket, err := makeProjectedBucket(day, SourceElapsedDays, low, base, high, capital, sample.elapsed)
		if err != nil {
			return unavailableCurrent(ProjectionModeDay, projectionReason(err), MethodCurrentDay, basis)
		}
		buckets = append(buckets, bucket)
	}
	return CurrentProjection{
		Mode:    ProjectionModeDay,
		Status:  ProjectionStatusAvailable,
		Method:  MethodCurrentDay,
		Basis:   basis,
		Buckets: buckets,
	}
}

// projectCurrentMonth forecasts the remaining months from the complete months
// in trailing history and the current month's remainder from the elapsed days.
// The two guards both have to pass; a missing daily sample means the current
// month object would be a lie, so the whole current projection is unavailable.
func projectCurrentMonth(history []HistoryMonth, daily []HistoryDay, query Query, asOfDate time.Time) CurrentProjection {
	sample := buildHistorySample(history, query, asOfDate)
	day := buildDaySample(daily, asOfDate)
	monthStart := firstOfMonth(asOfDate)
	basis := CurrentProjectionBasis{
		From:    sample.basis.HistoryFrom,
		To:      monthStart.Format(reportDateLayout),
		Units:   len(sample.months),
		PoCount: sample.basis.PoCount,
		AsOf:    asOfDate.Format(reportDateLayout),
	}
	if basis.Units < MinCurrentMonthComplete || basis.PoCount < MinCurrentMonthPoCount ||
		day.elapsed < MinCurrentDayElapsedDays || day.poCount < MinCurrentDayPoCount {
		return unavailableCurrent(ProjectionModeMonth, ReasonInsufficientHistory, MethodCurrentMonth, basis)
	}
	if sample.incomplete || day.incomplete {
		return unavailableCurrent(ProjectionModeMonth, ReasonIncompleteCostBasis, MethodCurrentMonth, basis)
	}
	if sample.overflow || day.overflow {
		return unavailableCurrent(ProjectionModeMonth, ReasonValueOutOfRange, MethodCurrentMonth, basis)
	}

	amounts := make([]int64, 0, len(sample.months))
	capitals := make([]int64, 0, len(sample.months))
	for _, month := range sample.months {
		amounts = append(amounts, month.metrics.AmountMinor)
		capitals = append(capitals, month.metrics.CapitalMinor)
	}
	base, err := percentileInc(amounts, 1, 2)
	if err != nil {
		return unavailableCurrent(ProjectionModeMonth, projectionReason(err), MethodCurrentMonth, basis)
	}
	capital, err := percentileInc(capitals, 1, 2)
	if err != nil {
		return unavailableCurrent(ProjectionModeMonth, projectionReason(err), MethodCurrentMonth, basis)
	}
	low, high, err := monthlyBand(amounts, base)
	if err != nil {
		return unavailableCurrent(ProjectionModeMonth, projectionReason(err), MethodCurrentMonth, basis)
	}

	buckets := make([]ProjectedBucket, 0, 12)
	for month := firstOfNextMonth(asOfDate); month.Year() == asOfDate.Year(); month = month.AddDate(0, 1, 0) {
		bucket, err := makeProjectedBucket(month, SourceTrailingCompleteMonths, low, base, high, capital, len(sample.months))
		if err != nil {
			return unavailableCurrent(ProjectionModeMonth, projectionReason(err), MethodCurrentMonth, basis)
		}
		buckets = append(buckets, bucket)
	}

	remainder, err := currentMonthRemainder(day, asOfDate)
	if err != nil {
		return unavailableCurrent(ProjectionModeMonth, projectionReason(err), MethodCurrentMonth, basis)
	}
	actual, err := currentMetrics(day.poCount, day.amount, day.costed, day.capital)
	if err != nil {
		return unavailableCurrent(ProjectionModeMonth, projectionReason(err), MethodCurrentMonth, basis)
	}
	return CurrentProjection{
		Mode:    ProjectionModeMonth,
		Status:  ProjectionStatusAvailable,
		Method:  MethodCurrentMonth,
		Basis:   basis,
		Buckets: buckets,
		CurrentMonth: &CurrentMonthProjection{
			Actual:    actual,
			Remainder: remainder,
		},
	}
}

// currentMonthRemainder projects the rest of the month at the elapsed-day
// average. The band is the daily P25-P75 multiplied by the remaining days, and
// capital is the same daily average. Margin is derived from the base and the
// capital, never forecast on its own.
func currentMonthRemainder(day daySample, asOfDate time.Time) (CurrentProjectionRemainder, error) {
	remaining := daysInMonth(asOfDate) - day.elapsed
	if remaining < 0 {
		remaining = 0
	}
	baseDaily, err := averageExact(day.amount, day.elapsed)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	capitalDaily, err := averageExact(day.capital, day.elapsed)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	lowDaily, highDaily, err := dailyBand(day.amounts, baseDaily)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	low, err := multiplyExact(lowDaily, remaining)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	base, err := multiplyExact(baseDaily, remaining)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	high, err := multiplyExact(highDaily, remaining)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	capital, err := multiplyExact(capitalDaily, remaining)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	margin, err := subtractExact(base, capital)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	marginBps, err := RatioBps(margin, capital)
	if err != nil {
		return CurrentProjectionRemainder{}, err
	}
	return CurrentProjectionRemainder{
		LowMinor:     low,
		BaseMinor:    base,
		HighMinor:    high,
		CapitalMinor: capital,
		MarginMinor:  margin,
		MarginBps:    marginBps,
		SampleSize:   day.elapsed,
		Source:       SourceElapsedDays,
	}, nil
}

// daySample is the elapsed portion of the current month. amounts and capitals
// are indexed by elapsed day and include zeros, so an average is over calendar
// days and the P25-P75 band can see a slow day.
type daySample struct {
	elapsed    int
	poCount    int64
	amount     int64
	costed     int64
	capital    int64
	amounts    []int64
	capitals   []int64
	incomplete bool
	overflow   bool
}

func buildDaySample(daily []HistoryDay, asOfDate time.Time) daySample {
	monthStart := firstOfMonth(asOfDate)
	elapsed := int(asOfDate.Sub(monthStart)/(24*time.Hour)) + 1
	if elapsed < 1 {
		return daySample{}
	}
	byDate := make(map[string]Metrics, len(daily))
	for _, entry := range daily {
		date, err := time.Parse(reportDateLayout, entry.Date)
		if err != nil || date.Format(reportDateLayout) != entry.Date {
			continue
		}
		if date.Before(monthStart) || date.After(asOfDate) {
			continue
		}
		current := byDate[entry.Date]
		current.PoCount += entry.PoCount
		current.AmountMinor += entry.AmountMinor
		current.CostedMinor += entry.CostedMinor
		current.UncostedMinor += entry.UncostedMinor
		current.CapitalMinor += entry.CapitalMinor
		byDate[entry.Date] = current
	}

	sample := daySample{
		elapsed:  elapsed,
		amounts:  make([]int64, 0, elapsed),
		capitals: make([]int64, 0, elapsed),
	}
	for index := 0; index < elapsed; index++ {
		day := monthStart.AddDate(0, 0, index).Format(reportDateLayout)
		metrics := byDate[day]
		sample.amounts = append(sample.amounts, metrics.AmountMinor)
		sample.capitals = append(sample.capitals, metrics.CapitalMinor)
		sample.poCount += metrics.PoCount
		sample.amount += metrics.AmountMinor
		sample.costed += metrics.CostedMinor
		sample.capital += metrics.CapitalMinor
		if metrics.CostedMinor < metrics.AmountMinor || metrics.UncostedMinor > 0 {
			sample.incomplete = true
		}
	}
	for _, value := range []int64{sample.poCount, sample.amount, sample.costed, sample.capital} {
		if err := CheckExact(value); err != nil {
			sample.overflow = true
		}
	}
	return sample
}

// dailyBand is the daily P25-P75 after at least four elapsed days, and the
// min-max under four. The band is widened only when it would exclude the base;
// a band that excludes its own base reads as a contradiction.
func dailyBand(values []int64, base int64) (int64, int64, error) {
	if len(values) == 0 {
		return 0, 0, fmt.Errorf("%w: a daily band needs at least one day", ErrInvalid)
	}
	low, high := minMax(values)
	if len(values) >= 4 {
		var err error
		if low, err = percentileInc(values, 1, 4); err != nil {
			return 0, 0, err
		}
		if high, err = percentileInc(values, 3, 4); err != nil {
			return 0, 0, err
		}
	}
	if low > base {
		low = base
	}
	if high < base {
		high = base
	}
	return low, high, nil
}

// monthlyBand is always the monthly P25-P75, widened only to include the base.
// Month mode has at least three complete months by its guard, so the
// interpolation is defined.
func monthlyBand(values []int64, base int64) (int64, int64, error) {
	low, err := percentileInc(values, 1, 4)
	if err != nil {
		return 0, 0, err
	}
	high, err := percentileInc(values, 3, 4)
	if err != nil {
		return 0, 0, err
	}
	if low > base {
		low = base
	}
	if high < base {
		high = base
	}
	return low, high, nil
}

func averageExact(total int64, count int) (int64, error) {
	if count <= 0 {
		return 0, fmt.Errorf("%w: an average needs a positive count", ErrInvalid)
	}
	return exactInt64(roundQuotientAwayFromZero(big.NewInt(total), big.NewInt(int64(count))))
}

func multiplyExact(value int64, factor int) (int64, error) {
	return exactInt64(new(big.Int).Mul(big.NewInt(value), big.NewInt(int64(factor))))
}

func currentMetrics(poCount, amount, costed, capital int64) (Metrics, error) {
	for name, value := range map[string]int64{
		"po count": poCount,
		"amount":   amount,
		"costed":   costed,
		"capital":  capital,
	} {
		if err := CheckExact(value); err != nil {
			return Metrics{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	margin, err := subtractExact(costed, capital)
	if err != nil {
		return Metrics{}, fmt.Errorf("margin: %w", err)
	}
	uncosted, err := subtractExact(amount, costed)
	if err != nil {
		return Metrics{}, fmt.Errorf("uncosted: %w", err)
	}
	marginBps, err := RatioBps(margin, capital)
	if err != nil {
		return Metrics{}, fmt.Errorf("margin percentage: %w", err)
	}
	coverageBps, err := RatioBps(costed, amount)
	if err != nil {
		return Metrics{}, fmt.Errorf("coverage percentage: %w", err)
	}
	return Metrics{
		PoCount:       poCount,
		AmountMinor:   amount,
		CostedMinor:   costed,
		UncostedMinor: uncosted,
		CapitalMinor:  capital,
		MarginMinor:   margin,
		MarginBps:     marginBps,
		CoverageBps:   coverageBps,
	}, nil
}

func daysInMonth(date time.Time) int {
	return int(firstOfNextMonth(date).Sub(firstOfMonth(date)) / (24 * time.Hour))
}

func unavailableCurrent(mode, reason, method string, basis CurrentProjectionBasis) CurrentProjection {
	return CurrentProjection{
		Mode:    mode,
		Status:  ProjectionStatusUnavailable,
		Reason:  reason,
		Method:  method,
		Basis:   basis,
		Buckets: []ProjectedBucket{},
	}
}
