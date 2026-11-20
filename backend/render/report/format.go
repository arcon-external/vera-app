package report

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"vera-app/backend/currency"
	"vera-app/backend/poreport"
)

// categoryLabels is the workbook vocabulary. The report payload carries the
// same label on each category row, but the filter carries only a code, and the
// PDF must not print a bare SUPPLY_GEN where the screen says "Goods Supply &
// General". The Unassigned row is the report sentinel for POs saved before the
// business category existed; it is never inferred from a subject.
var categoryLabels = map[string]string{
	poreport.CategoryFm:            "Facility Management",
	poreport.CategoryFab:           "Fabrication",
	poreport.CategorySupplyGeneral: "Goods Supply & General",
	poreport.CategoryOthers:        "Others",
	poreport.CategoryUnassigned:    "Unassigned",
}

func categoryLabel(code string) string {
	if code == "" || code == poreport.CategoryAll {
		return "All categories"
	}
	if label, ok := categoryLabels[code]; ok {
		return label
	}
	return code
}

// categoryRowLabel prints the row's own label when the server supplied one,
// with its stable code beside it. The code is what the workbook and the filter
// use, so a reader can move between the PDF and the app without guessing.
func categoryRowLabel(code, label string) string {
	if strings.TrimSpace(label) == "" {
		label = categoryLabel(code)
	}
	if code == "" {
		return label
	}
	return label + " (" + code + ")"
}

func categoryFilterLabel(code string) string {
	if code == "" || code == poreport.CategoryAll {
		return "All categories (all)"
	}
	return categoryLabel(code) + " (" + code + ")"
}

func bucketLabel(bucket string) string {
	switch bucket {
	case poreport.BucketDay:
		return "Daily"
	case poreport.BucketMonth:
		return "Monthly"
	case poreport.BucketQuarter:
		return "Quarterly"
	case poreport.BucketYear:
		return "Yearly"
	default:
		return bucket
	}
}

func projectionReasonLabel(reason string) string {
	switch reason {
	case poreport.ReasonNoDataInRange:
		return "No completed purchase orders in the selected range, so there is nothing to forecast."
	case poreport.ReasonInsufficientHistory:
		return "Not enough complete history: the monthly forecast needs six complete months and six POs; the yearly forecast needs twelve complete months."
	case poreport.ReasonNoData:
		return "No purchase orders in the current period yet, so there is nothing to project."
	case poreport.ReasonInsufficientDays:
		return "Not enough elapsed days: the current month needs five elapsed days and three purchase orders."
	case poreport.ReasonInsufficientMonths:
		return "Not enough complete months: the current year needs three complete months and five purchase orders."
	case poreport.ReasonIncompleteCostBasis:
		return "The history is not fully costed, so no margin-bearing projection is produced. Uncosted amount is never treated as profit."
	case poreport.ReasonValueOutOfRange:
		return "The forecast cannot be named exactly, so it is withheld rather than rounded."
	case poreport.ReasonNotCurrentPeriod:
		return "This range is not the current month or year. A projection applies to the current month or year."
	case poreport.ReasonInvalidQuery:
		return "The report query was invalid, so no projection is produced."
	default:
		if strings.TrimSpace(reason) == "" {
			return "No projection is available."
		}
		return reason
	}
}

func projectionSourceLabel(source string) string {
	switch source {
	case poreport.SourceSameCalendarMonth:
		return "Same calendar month across complete history"
	case poreport.SourceTrailingThreeCompleteMonths:
		return "Trailing three complete months"
	case poreport.SourceTrailingTwelveCompleteMonths:
		return "Trailing twelve complete months"
	case poreport.SourceElapsedDays:
		return "Elapsed days of the current month"
	case poreport.SourceTrailingCompleteMonths:
		return "Trailing complete months"
	default:
		return source
	}
}

func projectionStatusLabel(status string) string {
	if status == poreport.ProjectionStatusAvailable {
		return "Available"
	}
	return "Unavailable"
}

// formatMoney renders a signed minor amount with the currency's own symbol
// placement, exponent and separators. The sign goes inside the affixes
// ("Rp-30.000.000", "$-1,234.56", "-1.234,56 €"), which is the established
// app-wide convention (`formatMoney` in frontend/src/format.ts). An unresolved
// code is refused during validation, so this helper never invents IDR.
func formatMoney(code string, minor int64) string {
	negative := minor < 0
	magnitude := uint64(minor)
	if negative {
		magnitude = uint64(-minor)
	}
	bare := currency.FormatAmount(code, magnitude)
	if negative {
		bare = "-" + bare
	}
	return currency.Decorate(code, bare)
}

// formatSignedMoney exists for call sites that read a value as profit or loss.
// The established convention is the same string as formatMoney: the minus sits
// inside the currency affixes, never between the symbol and the digits.
func formatSignedMoney(code string, minor int64) string {
	return formatMoney(code, minor)
}

// formatCount renders a count with en-US grouping, matching the screen's
// formatReportCount. A count is never abbreviated in a table: "1,234" is exact.
func formatCount(value int64) string {
	negative := value < 0
	magnitude := value
	if negative {
		magnitude = -magnitude
	}
	digits := strconv.FormatInt(magnitude, 10)
	if len(digits) <= 3 {
		if negative {
			return "-" + digits
		}
		return digits
	}
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	grouped := strings.Join(parts, ",")
	if negative {
		return "-" + grouped
	}
	return grouped
}

// formatPct renders basis points as an exact two-decimal percentage using
// integer arithmetic, matching the screen's formatReportBps. A nil ratio is
// unknown and prints "-", never "0.00%": the report's whole point is that a
// margin with no capital is unknowable, not zero. No floating-point division
// is involved, so 1077 is always 10.77%, never a rounded 10.8%.
func formatPct(bps *int64) string {
	if bps == nil {
		return "-"
	}
	value := *bps
	negative := value < 0
	magnitude := value
	if negative {
		magnitude = -magnitude
	}
	whole := magnitude / 100
	fraction := magnitude % 100
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%02d%%", sign, whole, fraction)
}

// compactMoney is the axis label form: one significant decimal and K/M/B/T,
// with the currency's own symbol placement. The KPI strip and the tables print
// exact figures; an axis tick does not need every rupiah.
//
// Axis values arrive in minor units, exactly like the table's money. A currency
// with an exponent must be converted to major units before compacting, or USD
// 10,000 minor would print "$10K" beside a $100 table figure -- a 100x lie on
// the same page. IDR's exponent is zero, so this is invisible there and easy to
// miss; the currency catalogue owns the exponent, never a hard-coded 100.
func compactMoney(code string, value float64) string {
	info, ok := currency.Lookup(code)
	if ok && info.Exponent > 0 {
		value /= math.Pow10(info.Exponent)
	}
	negative := value < 0
	if negative {
		value = -value
	}
	prefix, suffix := currency.Affixes(code)
	bare := compactNumber(value)
	if negative {
		return prefix + "-" + bare + suffix
	}
	return prefix + bare + suffix
}

func compactNumber(value float64) string {
	units := []struct {
		divisor float64
		suffix  string
	}{
		{1e18, "E"},
		{1e15, "P"},
		{1e12, "T"},
		{1e9, "B"},
		{1e6, "M"},
		{1e3, "K"},
	}
	for _, unit := range units {
		if value >= unit.divisor {
			return trimDecimal(value/unit.divisor) + unit.suffix
		}
	}
	return trimDecimal(value)
}

func trimDecimal(value float64) string {
	text := strconv.FormatFloat(value, 'f', 1, 64)
	return strings.TrimSuffix(text, ".0")
}

// periodLabel is the short label under a chart column and in a period table.
// The bucket decides whether "Jan 26" is a month, "Q1 26" a quarter or "2026"
// a year, so the same periodStart cannot be read at the wrong resolution.
func periodLabel(periodStart, bucket string) string {
	date, err := time.Parse(time.DateOnly, periodStart)
	if err != nil {
		return periodStart
	}
	switch bucket {
	case poreport.BucketDay:
		return date.Format("2 Jan")
	case poreport.BucketMonth:
		return date.Format("Jan 06")
	case poreport.BucketQuarter:
		return fmt.Sprintf("Q%d %02d", (int(date.Month())-1)/3+1, date.Year()%100)
	case poreport.BucketYear:
		return fmt.Sprintf("%d", date.Year())
	default:
		return periodStart
	}
}

// periodTableLabel is the long form used in the period table, where there is
// room for the year and the reader is looking up an exact bucket. A day bucket
// prints the full date because 1 Jan 2026 and 1 Dec 2026 are different rows.
func periodTableLabel(periodStart, bucket string) string {
	date, err := time.Parse(time.DateOnly, periodStart)
	if err != nil {
		return periodStart
	}
	switch bucket {
	case poreport.BucketDay:
		return date.Format("2 Jan 2006")
	case poreport.BucketMonth:
		return date.Format("Jan 2006")
	case poreport.BucketQuarter:
		return fmt.Sprintf("Q%d %d", (int(date.Month())-1)/3+1, date.Year())
	case poreport.BucketYear:
		return fmt.Sprintf("%d", date.Year())
	default:
		return periodStart
	}
}

// humanDate renders a wire date for prose, where the ISO spelling would read
// like an identifier rather than a day. An unparseable value is passed through
// unchanged so the renderer never invents a date.
func humanDate(value string) string {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return value
	}
	return date.Format("2 Jan 2006")
}

// monthYear renders the calendar month a period belongs to, for a chart
// subtitle such as "November 2026".
func monthYear(value string) string {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return ""
	}
	return date.Format("January 2006")
}

func yearLabel(value string) string {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d", date.Year())
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func periodMarker(period poreport.Period) string {
	switch {
	case period.InProgress:
		return "IP"
	case period.Partial:
		return "P"
	default:
		return ""
	}
}

// generatedLabel keeps the server's timestamp but replaces the T separator so
// a reader does not have to parse ISO 8601 to see when the report was made.
func generatedLabel(value string) string {
	if date, err := time.Parse(time.RFC3339, value); err == nil {
		return date.UTC().Format("02 Jan 2006 15:04:05 UTC")
	}
	return value
}

// fittedSize shrinks a single-line label to minimum. Its callers are labels
// whose content is already bounded (dates, currency codes, chart ticks); table
// cells and KPI values use wrapCell instead, because shrinking to a floor that
// is still wider than the box would put text into the next column.
func fittedSize(measure func(string, float64, bool) float64, text string, maxWidth, preferred, minimum float64, bold bool) float64 {
	size := preferred
	for size > minimum && measure(text, size, bold) > maxWidth {
		size -= 0.25
	}
	if size < minimum {
		size = minimum
	}
	return size
}
