package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"vera-app/backend/currency"
	"vera-app/backend/poreport"
)

// SnapshotLimit is the largest report JSON the export accepts. The complete
// 120-period report (with five aligned category series, twelve monthly
// projections and three yearly ones) is well under half a megabyte, so this
// leaves room for a company address and disclosure notes without letting a
// caller hand the renderer an unbounded document.
const SnapshotLimit = 2 << 20

// DecodeSnapshot validates and decodes the report snapshot the desktop hands
// the export. It refuses malformed JSON, unknown fields, trailing data, an
// oversized document and every shape or figure the renderer cannot print
// truthfully.
//
// The snapshot is the sole source of the figures: this function never fetches
// a report, recalculates a total, or falls back to a default currency. An
// unresolved currency is refused rather than labelled IDR, because a report
// that says IDR while its numbers are USD is worse than no report.
func DecodeSnapshot(raw []byte) (poreport.Report, error) {
	if len(raw) > SnapshotLimit {
		return poreport.Report{}, fmt.Errorf(
			"purchase order report snapshot is %d bytes; the limit is %d", len(raw), SnapshotLimit)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot poreport.Report
	if err := decoder.Decode(&snapshot); err != nil {
		return poreport.Report{}, fmt.Errorf("decode purchase order report snapshot: %w", err)
	}
	// A second value after the report is not a report with a comment; it is a
	// caller sending two documents and assuming one wins.
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return poreport.Report{}, fmt.Errorf("decode purchase order report snapshot: trailing data")
		}
		return poreport.Report{}, fmt.Errorf("decode purchase order report snapshot: %w", err)
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		return poreport.Report{}, err
	}
	return snapshot, nil
}

// ValidateSnapshot applies the renderer's structural and numeric limits. It
// is exported so a caller can check a decoded report before choosing a save
// path, and so a test can pin each refusal without constructing JSON.
//
// It deliberately performs no report arithmetic: a figure is either inside the
// exact range or it is refused, but no total is rebuilt to see whether it
// "adds up". The server owns the arithmetic; this package must print what it
// was given or print nothing.
func ValidateSnapshot(snapshot poreport.Report) error {
	if strings.TrimSpace(snapshot.GeneratedAt) == "" {
		return fmt.Errorf("purchase order report snapshot: generatedAt is required")
	}
	if _, err := time.Parse(time.RFC3339, snapshot.GeneratedAt); err != nil {
		return fmt.Errorf("purchase order report snapshot: generatedAt %q is not RFC3339", snapshot.GeneratedAt)
	}
	if err := validateDate("asOf", snapshot.AsOf, true); err != nil {
		return err
	}
	if len(snapshot.Company.Name) > 256 {
		return fmt.Errorf("purchase order report snapshot: company name is %d characters; the cap is 256", len(snapshot.Company.Name))
	}
	if len(snapshot.Company.Address) > 1024 {
		return fmt.Errorf("purchase order report snapshot: company address is %d characters; the cap is 1024", len(snapshot.Company.Address))
	}

	filter, err := poreport.NormalizeQuery(snapshot.Filter)
	if err != nil {
		return fmt.Errorf("purchase order report snapshot filter: %w", err)
	}
	if _, ok := currency.Lookup(snapshot.Filter.Currency); !ok {
		return fmt.Errorf("purchase order report snapshot: currency %q is not in the supported catalogue",
			snapshot.Filter.Currency)
	}
	if len(snapshot.Population.AvailableCurrencies) > 64 {
		return fmt.Errorf("purchase order report snapshot: %d available currencies; the cap is 64",
			len(snapshot.Population.AvailableCurrencies))
	}
	if snapshot.Population.SourceMissingPoCount < 0 {
		return fmt.Errorf("purchase order report snapshot: sourceMissingPoCount is negative")
	}

	if err := validateMetrics("totals", snapshot.Totals); err != nil {
		return err
	}
	if len(snapshot.Periods) > poreport.MaxPeriods {
		return fmt.Errorf("purchase order report snapshot: %d actual periods; the cap is %d",
			len(snapshot.Periods), poreport.MaxPeriods)
	}
	if filter.Bucket == poreport.BucketDay && len(snapshot.Periods) > poreport.MaxDayPeriods {
		return fmt.Errorf("purchase order report snapshot: %d day periods; the cap is %d",
			len(snapshot.Periods), poreport.MaxDayPeriods)
	}
	if len(snapshot.Categories) > 5 {
		return fmt.Errorf("purchase order report snapshot: %d category rows; the cap is 5", len(snapshot.Categories))
	}
	if len(snapshot.CategorySeries) > 5 {
		return fmt.Errorf("purchase order report snapshot: %d category series; the cap is 5", len(snapshot.CategorySeries))
	}
	if len(snapshot.Notes) > 32 {
		return fmt.Errorf("purchase order report snapshot: %d notes; the cap is 32", len(snapshot.Notes))
	}

	var previous time.Time
	for i, period := range snapshot.Periods {
		name := fmt.Sprintf("periods[%d]", i)
		if err := validatePeriodStart(name+".periodStart", period.PeriodStart, filter.Bucket); err != nil {
			return err
		}
		start, _ := time.Parse(time.DateOnly, period.PeriodStart)
		if i > 0 && !start.After(previous) {
			return fmt.Errorf("purchase order report snapshot: %s starts at %s, not after %s",
				name, period.PeriodStart, previous.Format(time.DateOnly))
		}
		previous = start
		if err := validateMetrics(name, period.Metrics); err != nil {
			return err
		}
	}

	for i, category := range snapshot.Categories {
		name := fmt.Sprintf("categories[%d]", i)
		if strings.TrimSpace(category.BusinessCategory) == "" {
			return fmt.Errorf("purchase order report snapshot: %s has no business category", name)
		}
		if len(category.BusinessCategory) > 32 || len(category.Label) > 128 {
			return fmt.Errorf("purchase order report snapshot: %s label is too long", name)
		}
		if err := validateMetrics(name, category.Metrics); err != nil {
			return err
		}
	}

	for i, series := range snapshot.CategorySeries {
		name := fmt.Sprintf("categorySeries[%d]", i)
		if strings.TrimSpace(series.BusinessCategory) == "" {
			return fmt.Errorf("purchase order report snapshot: %s has no business category", name)
		}
		if len(series.MarginBps) != len(snapshot.Periods) {
			return fmt.Errorf("purchase order report snapshot: %s has %d margin points for %d periods",
				name, len(series.MarginBps), len(snapshot.Periods))
		}
		for j, bps := range series.MarginBps {
			if bps == nil {
				continue
			}
			if err := validateExact(fmt.Sprintf("%s.marginBps[%d]", name, j), *bps); err != nil {
				return err
			}
		}
	}

	for i, note := range snapshot.Notes {
		if strings.TrimSpace(note.Code) == "" {
			return fmt.Errorf("purchase order report snapshot: notes[%d].code is required", i)
		}
		if len(note.Code) > 64 || len(note.Text) > 2048 {
			return fmt.Errorf("purchase order report snapshot: notes[%d] is too long", i)
		}
	}

	if err := validateProjectionSeries("projection.monthly", snapshot.Projection.Monthly, 12); err != nil {
		return err
	}
	if err := validateProjectionSeries("projection.yearly", snapshot.Projection.Yearly, 3); err != nil {
		return err
	}
	if err := validateCurrentProjection("currentProjection", snapshot.CurrentProjection); err != nil {
		return err
	}
	if err := validateMonthSummary("monthSummary", snapshot.MonthSummary, filter, snapshot.AsOf); err != nil {
		return err
	}
	return nil
}

func validateProjectionSeries(name string, series poreport.ProjectionSeries, bucketCap int) error {
	switch series.Status {
	case poreport.ProjectionStatusAvailable, poreport.ProjectionStatusUnavailable:
	default:
		return fmt.Errorf("purchase order report snapshot: %s status %q is not available or unavailable",
			name, series.Status)
	}
	if len(series.Buckets) > bucketCap {
		return fmt.Errorf("purchase order report snapshot: %s has %d buckets; the cap is %d",
			name, len(series.Buckets), bucketCap)
	}
	if series.Status == poreport.ProjectionStatusAvailable && len(series.Buckets) == 0 {
		return fmt.Errorf("purchase order report snapshot: %s is available with no buckets", name)
	}
	if series.Status == poreport.ProjectionStatusUnavailable && strings.TrimSpace(series.Reason) == "" {
		return fmt.Errorf("purchase order report snapshot: %s is unavailable without a reason", name)
	}
	if len(series.Reason) > 64 || len(series.Method) > 1024 {
		return fmt.Errorf("purchase order report snapshot: %s method or reason is too long", name)
	}
	if err := validateDate(name+".basis.historyFrom", series.Basis.HistoryFrom, false); err != nil {
		return err
	}
	if err := validateDate(name+".basis.historyTo", series.Basis.HistoryTo, false); err != nil {
		return err
	}
	if err := validateDate(name+".basis.asOf", series.Basis.AsOf, false); err != nil {
		return err
	}
	if series.Basis.CompleteMonths < 0 || int64(series.Basis.CompleteMonths) > poreport.MaxExactMinor {
		return fmt.Errorf("purchase order report snapshot: %s completeMonths is out of range", name)
	}
	if series.Basis.PoCount < 0 {
		return fmt.Errorf("purchase order report snapshot: %s basis PoCount is negative", name)
	}

	var previous time.Time
	for i, bucket := range series.Buckets {
		label := fmt.Sprintf("%s.buckets[%d]", name, i)
		if err := validatePeriodStart(label+".periodStart", bucket.PeriodStart, poreport.BucketMonth); err != nil {
			return err
		}
		start, _ := time.Parse(time.DateOnly, bucket.PeriodStart)
		if i > 0 && !start.After(previous) {
			return fmt.Errorf("purchase order report snapshot: %s starts at %s, not after %s",
				label, bucket.PeriodStart, previous.Format(time.DateOnly))
		}
		previous = start
		if err := validateProjectedBucket(label, bucket); err != nil {
			return err
		}
	}
	return nil
}

// validateProjectedBucket applies the bounds every projected bucket shares,
// whether it comes from the retired monthly/yearly series or the current-period
// forecast. It checks shape and range, never whether the figures add up: the
// renderer prints what it was given and does not rebuild a total.
func validateProjectedBucket(label string, bucket poreport.ProjectedBucket) error {
	if bucket.AmountLowMinor > bucket.AmountBaseMinor || bucket.AmountBaseMinor > bucket.AmountHighMinor {
		return fmt.Errorf("purchase order report snapshot: %s amount band is not low <= base <= high", label)
	}
	for _, value := range []struct {
		name  string
		value int64
	}{
		{"amountLowMinor", bucket.AmountLowMinor},
		{"amountBaseMinor", bucket.AmountBaseMinor},
		{"amountHighMinor", bucket.AmountHighMinor},
		{"capitalMinor", bucket.CapitalMinor},
		{"marginMinor", bucket.MarginMinor},
	} {
		if err := validateExact(label+"."+value.name, value.value); err != nil {
			return err
		}
	}
	if bucket.MarginBps != nil {
		if err := validateExact(label+".marginBps", *bucket.MarginBps); err != nil {
			return err
		}
	}
	if bucket.SampleSize < 0 || int64(bucket.SampleSize) > poreport.MaxExactMinor {
		return fmt.Errorf("purchase order report snapshot: %s sampleSize is out of range", label)
	}
	if len(bucket.Source) > 64 {
		return fmt.Errorf("purchase order report snapshot: %s source is too long", label)
	}
	return nil
}

// currentProjectionAbsent reports the zero value encoding/json leaves when a
// snapshot predates the current-period forecast. Such a snapshot is still
// accepted and printed; the summary says the forecast is unavailable.
func currentProjectionAbsent(current poreport.CurrentProjection) bool {
	return current.Mode == "" && current.Status == "" && current.Reason == "" &&
		current.Method == "" && current.Basis == (poreport.CurrentProjectionBasis{}) &&
		len(current.Buckets) == 0 && current.CurrentMonth == nil
}

func validateCurrentProjection(name string, current poreport.CurrentProjection) error {
	if currentProjectionAbsent(current) {
		return nil
	}
	switch current.Mode {
	case poreport.ProjectionModeDay, poreport.ProjectionModeMonth, poreport.ProjectionModeNone:
	default:
		return fmt.Errorf("purchase order report snapshot: %s mode %q is not day, month or none",
			name, current.Mode)
	}
	switch current.Status {
	case poreport.ProjectionStatusAvailable, poreport.ProjectionStatusUnavailable:
	default:
		return fmt.Errorf("purchase order report snapshot: %s status %q is not available or unavailable",
			name, current.Status)
	}
	if len(current.Reason) > 64 || len(current.Method) > 1024 {
		return fmt.Errorf("purchase order report snapshot: %s method or reason is too long", name)
	}

	bucketCap := 0
	switch current.Mode {
	case poreport.ProjectionModeDay:
		bucketCap = poreport.MaxDayPeriods
	case poreport.ProjectionModeMonth:
		// The retired monthly series also used twelve; accepting a full year
		// keeps a supplied snapshot printable without weakening the day cap.
		bucketCap = 12
	case poreport.ProjectionModeNone:
		// Mode none is not drawn, but a snapshot may still carry the last
		// computed forecast. Bound it like a day series rather than refusing a
		// document the reader can safely ignore.
		bucketCap = poreport.MaxDayPeriods
	}
	if len(current.Buckets) > bucketCap {
		return fmt.Errorf("purchase order report snapshot: %s has %d buckets; the cap is %d",
			name, len(current.Buckets), bucketCap)
	}
	if current.CurrentMonth != nil {
		if err := validateMetrics(name+".currentMonth.actual", current.CurrentMonth.Actual); err != nil {
			return err
		}
		if err := validateCurrentRemainder(name+".currentMonth.remainder", current.CurrentMonth.Remainder); err != nil {
			return err
		}
	}
	if current.Status == poreport.ProjectionStatusAvailable {
		if current.Mode == poreport.ProjectionModeNone {
			return fmt.Errorf("purchase order report snapshot: %s is available in mode none", name)
		}
		// The last day of a month or the last month of a year legitimately has
		// no remaining bucket; an available forecast with an empty bucket list
		// is accepted so that day still renders its actuals and remainder.
	} else {
		if strings.TrimSpace(current.Reason) == "" {
			return fmt.Errorf("purchase order report snapshot: %s is unavailable without a reason", name)
		}
		if !knownCurrentProjectionReason(current.Reason) {
			return fmt.Errorf("purchase order report snapshot: %s reason %q is not a stable current-period code",
				name, current.Reason)
		}
	}

	basis := current.Basis
	if err := validateDate(name+".basis.from", basis.From, false); err != nil {
		return err
	}
	if err := validateDate(name+".basis.to", basis.To, false); err != nil {
		return err
	}
	if err := validateDate(name+".basis.asOf", basis.AsOf, false); err != nil {
		return err
	}
	if basis.Units < 0 || int64(basis.Units) > poreport.MaxExactMinor {
		return fmt.Errorf("purchase order report snapshot: %s basis units is out of range", name)
	}
	if basis.PoCount < 0 {
		return fmt.Errorf("purchase order report snapshot: %s basis PoCount is negative", name)
	}
	if current.Status == poreport.ProjectionStatusAvailable &&
		(strings.TrimSpace(basis.From) == "" || strings.TrimSpace(basis.To) == "") {
		return fmt.Errorf("purchase order report snapshot: %s is available without a sample window", name)
	}

	var previous time.Time
	for i, bucket := range current.Buckets {
		label := fmt.Sprintf("%s.buckets[%d]", name, i)
		start, err := validateCurrentBucketStart(label, bucket.PeriodStart, current.Mode)
		if err != nil {
			return err
		}
		if i > 0 && !start.After(previous) {
			return fmt.Errorf("purchase order report snapshot: %s starts at %s, not after %s",
				label, bucket.PeriodStart, previous.Format(time.DateOnly))
		}
		previous = start
		if err := validateProjectedBucket(label, bucket); err != nil {
			return err
		}
	}
	return nil
}

func firstOfMonthDate(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func validateCurrentBucketStart(label, value, mode string) (time.Time, error) {
	if err := validateDate(label, value, true); err != nil {
		return time.Time{}, err
	}
	start, _ := time.Parse(time.DateOnly, value)
	if mode == poreport.ProjectionModeMonth && start.Day() != 1 {
		return time.Time{}, fmt.Errorf("purchase order report snapshot: %s %q is not the first day of a month",
			label, value)
	}
	return start, nil
}

func validateCurrentRemainder(name string, remainder poreport.CurrentProjectionRemainder) error {
	if remainder.LowMinor > remainder.BaseMinor || remainder.BaseMinor > remainder.HighMinor {
		return fmt.Errorf("purchase order report snapshot: %s band is not low <= base <= high", name)
	}
	for _, value := range []struct {
		name  string
		value int64
	}{
		{"low", remainder.LowMinor},
		{"base", remainder.BaseMinor},
		{"high", remainder.HighMinor},
		{"capital", remainder.CapitalMinor},
		{"margin", remainder.MarginMinor},
	} {
		if err := validateExact(name+"."+value.name, value.value); err != nil {
			return err
		}
	}
	if remainder.MarginBps != nil {
		if err := validateExact(name+".marginBps", *remainder.MarginBps); err != nil {
			return err
		}
	}
	if remainder.SampleSize < 0 || int64(remainder.SampleSize) > poreport.MaxExactMinor {
		return fmt.Errorf("purchase order report snapshot: %s sampleSize is out of range", name)
	}
	if len(remainder.Source) > 64 {
		return fmt.Errorf("purchase order report snapshot: %s source is too long", name)
	}
	return nil
}

func knownCurrentProjectionReason(reason string) bool {
	switch reason {
	case poreport.ReasonNotCurrentPeriod, poreport.ReasonNoData,
		poreport.ReasonInsufficientDays, poreport.ReasonInsufficientMonths,
		poreport.ReasonIncompleteCostBasis, poreport.ReasonValueOutOfRange,
		poreport.ReasonInvalidQuery:
		return true
	default:
		return false
	}
}

// validateMonthSummary pins the additive current-month readout. It is optional
// on a report, but when present every field must describe this exact report: a
// summary for a different month, a remainder alongside an unavailable forecast,
// or an inexact figure is refused rather than printed under the wrong heading.
func validateMonthSummary(name string, summary *poreport.MonthSummary, filter poreport.Query, asOf string) error {
	if summary == nil {
		return nil
	}
	month, err := time.Parse("2006-01", summary.Month)
	if err != nil || month.Format("2006-01") != summary.Month {
		return fmt.Errorf("purchase order report snapshot: %s.month %q is not a zero-padded YYYY-MM identifier",
			name, summary.Month)
	}
	nextMonth := month.AddDate(0, 1, 0).Format(time.DateOnly)
	if summary.From != summary.Month+"-01" || summary.To != nextMonth {
		return fmt.Errorf("purchase order report snapshot: %s from/to %q/%q is not the calendar month %s",
			name, summary.From, summary.To, summary.Month)
	}
	if summary.AsOf != asOf {
		return fmt.Errorf("purchase order report snapshot: %s.asOf %q differs from the report's %q",
			name, summary.AsOf, asOf)
	}
	if firstSeven(asOf) != summary.Month {
		return fmt.Errorf("purchase order report snapshot: %s.month %q is not the report's as-of month %s",
			name, summary.Month, firstSeven(asOf))
	}
	if err := validateDate(name+".asOf", summary.AsOf, true); err != nil {
		return err
	}

	switch summary.Mode {
	case poreport.ProjectionModeDay:
		if filter.Bucket != poreport.BucketDay ||
			filter.From != summary.From || filter.To != summary.To {
			return fmt.Errorf("purchase order report snapshot: %s day readout does not match the report's day filter", name)
		}
	case poreport.ProjectionModeMonth:
		yearStart := time.Date(month.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
		if filter.Bucket != poreport.BucketMonth ||
			filter.From != yearStart.Format(time.DateOnly) ||
			filter.To != yearStart.AddDate(1, 0, 0).Format(time.DateOnly) {
			return fmt.Errorf("purchase order report snapshot: %s month readout does not match the report's current-year filter", name)
		}
	default:
		return fmt.Errorf("purchase order report snapshot: %s.mode %q is not day or month", name, summary.Mode)
	}

	if err := validateMetrics(name+".actual", summary.Actual); err != nil {
		return err
	}
	if summary.RecordedFuture != nil {
		if err := validateMetrics(name+".recordedFuture", *summary.RecordedFuture); err != nil {
			return err
		}
	}

	projection := summary.Projection
	switch projection.Status {
	case poreport.ProjectionStatusAvailable:
		if strings.TrimSpace(projection.Reason) != "" {
			return fmt.Errorf("purchase order report snapshot: %s.projection is available with reason %q",
				name, projection.Reason)
		}
		if projection.Remainder == nil {
			return fmt.Errorf("purchase order report snapshot: %s.projection is available without a remainder", name)
		}
		if err := validateCurrentRemainder(name+".projection.remainder", *projection.Remainder); err != nil {
			return err
		}
	case poreport.ProjectionStatusUnavailable:
		if strings.TrimSpace(projection.Reason) == "" {
			return fmt.Errorf("purchase order report snapshot: %s.projection is unavailable without a reason", name)
		}
		if !knownCurrentProjectionReason(projection.Reason) {
			return fmt.Errorf("purchase order report snapshot: %s.projection reason %q is not a stable current-period code",
				name, projection.Reason)
		}
		if projection.Remainder != nil {
			return fmt.Errorf("purchase order report snapshot: %s.projection is unavailable with a remainder", name)
		}
	default:
		return fmt.Errorf("purchase order report snapshot: %s.projection status %q is not available or unavailable",
			name, projection.Status)
	}
	if len(projection.Reason) > 64 || len(projection.Method) > 1024 {
		return fmt.Errorf("purchase order report snapshot: %s.projection method or reason is too long", name)
	}
	return nil
}

func validateMetrics(name string, metrics poreport.Metrics) error {
	if metrics.PoCount < 0 {
		return fmt.Errorf("purchase order report snapshot: %s.poCount is negative", name)
	}
	for _, value := range []struct {
		name  string
		value int64
	}{
		{"poCount", metrics.PoCount},
		{"amountMinor", metrics.AmountMinor},
		{"costedMinor", metrics.CostedMinor},
		{"uncostedMinor", metrics.UncostedMinor},
		{"extraExpenseMinor", metrics.ExtraExpenseMinor},
		{"capitalMinor", metrics.CapitalMinor},
		{"marginMinor", metrics.MarginMinor},
	} {
		if err := validateExact(name+"."+value.name, value.value); err != nil {
			return err
		}
	}
	if metrics.MarginBps != nil {
		if err := validateExact(name+".marginBps", *metrics.MarginBps); err != nil {
			return err
		}
	}
	if metrics.CoverageBps != nil {
		if err := validateExact(name+".coverageBps", *metrics.CoverageBps); err != nil {
			return err
		}
	}
	return nil
}

// validateExact keeps every figure a report may emit inside the range a
// JavaScript number can name exactly, matching poreport.MaxExactMinor. The
// desktop carries the same snapshot through JSON, so a wider int64 here would
// be a value the screen quietly rounded before the PDF read it.
func validateExact(name string, value int64) error {
	if value < -poreport.MaxExactMinor || value > poreport.MaxExactMinor {
		return fmt.Errorf("purchase order report snapshot: %s = %d is outside the exact range ±%d",
			name, value, poreport.MaxExactMinor)
	}
	return nil
}

func validateDate(name, value string, required bool) error {
	if strings.TrimSpace(value) == "" {
		if required {
			return fmt.Errorf("purchase order report snapshot: %s is required", name)
		}
		return nil
	}
	date, err := time.Parse(time.DateOnly, value)
	if err != nil || date.Format(time.DateOnly) != value {
		return fmt.Errorf("purchase order report snapshot: %s %q is not a zero-padded YYYY-MM-DD date", name, value)
	}
	return nil
}

// validatePeriodStart applies the calendar anchor a bucket start must carry.
// A month, quarter or year bucket starts on the first day; a day bucket is any
// real calendar day, which is what lets a current-month report print 31 rows.
func validatePeriodStart(name, value, bucket string) error {
	if err := validateDate(name, value, true); err != nil {
		return err
	}
	if bucket == poreport.BucketDay {
		return nil
	}
	date, _ := time.Parse(time.DateOnly, value)
	if date.Day() != 1 {
		return fmt.Errorf("purchase order report snapshot: %s %q is not the first day of a period", name, value)
	}
	return nil
}
