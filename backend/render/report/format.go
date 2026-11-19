package report

import (
	"fmt"
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
	case poreport.ReasonIncompleteCostBasis:
		return "The history is not fully costed, so no projection is produced. Uncosted amount is never treated as profit."
	case poreport.ReasonValueOutOfRange:
		return "The forecast cannot be named exactly, so it is withheld rather than rounded."
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

// formatMoney renders a non-negative minor amount with the currency's own
// symbol placement, exponent and separators. An unresolved code is refused
// during validation, so this helper never invents IDR.
func formatMoney(code string, minor int64) string {
	if minor < 0 {
		return "-" + formatMoney(code, -minor)
	}
	return currency.Decorate(code, currency.FormatAmount(code, uint64(minor)))
}

// formatSignedMoney puts the minus before the symbol ("-Rp1.234") rather than
// between the symbol and the digits ("Rp-1.234"), which is how a loss reads in
// the ledger and in the screen's KPI strip.
func formatSignedMoney(code string, minor int64) string {
	if minor < 0 {
		return "-" + formatMoney(code, -minor)
	}
	return formatMoney(code, minor)
}

func formatCount(value int64) string {
	return strconv.FormatInt(value, 10)
}

// formatPct renders basis points as a percentage. A nil ratio is unknown and
// prints an em dash, never "0.0%": the report's whole point is that a margin
// with no capital is unknowable, not zero.
func formatPct(bps *int64) string {
	if bps == nil {
		return "—"
	}
	value := float64(*bps) / 100
	if value == 0 {
		value = 0
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + "%"
}

// compactMoney is the axis label form: one significant decimal and K/M/B/T,
// with the currency's own symbol placement. The KPI strip and the tables print
// exact figures; an axis tick does not need every rupiah.
func compactMoney(code string, value float64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	prefix, suffix := currency.Affixes(code)
	bare := compactNumber(value)
	if negative {
		return "-" + prefix + bare + suffix
	}
	return prefix + bare + suffix
}

func compactNumber(value float64) string {
	units := []struct {
		divisor float64
		suffix  string
	}{
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
// room for the year and the reader is looking up an exact bucket.
func periodTableLabel(periodStart, bucket string) string {
	date, err := time.Parse(time.DateOnly, periodStart)
	if err != nil {
		return periodStart
	}
	switch bucket {
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
