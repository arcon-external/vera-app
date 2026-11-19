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

	if _, err := poreport.NormalizeQuery(snapshot.Filter); err != nil {
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
		if err := validateDate(name+".periodStart", period.PeriodStart, true); err != nil {
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
		if err := validateDate(label+".periodStart", bucket.PeriodStart, true); err != nil {
			return err
		}
		start, _ := time.Parse(time.DateOnly, bucket.PeriodStart)
		if i > 0 && !start.After(previous) {
			return fmt.Errorf("purchase order report snapshot: %s starts at %s, not after %s",
				label, bucket.PeriodStart, previous.Format(time.DateOnly))
		}
		previous = start
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
	if date.Day() != 1 && (name == "periodStart" || strings.HasSuffix(name, ".periodStart")) {
		return fmt.Errorf("purchase order report snapshot: %s %q is not the first day of a period", name, value)
	}
	return nil
}
