// Package poreport holds the purchase-order report domain: the query contract,
// exact percentage arithmetic, and the deterministic complete-month projection.
//
// The report is a read-only snapshot. Actual figures come from one store
// aggregate over saved purchase orders; projections are derived here from the
// monthly history that aggregate returns and are never mixed into totals.
//
// All money is int64 minor units in one currency. No figure is ever converted
// between currencies, and no float is used in a calculation. Values that cross
// JSON are bounded by MaxExactMinor so a JavaScript number can name them
// exactly; a value outside that bound is refused rather than allowed to read
// back one minor unit wrong.
package poreport

import "errors"

// MaxPeriods is the approved ceiling on intersected actual calendar buckets in
// one report. A range that touches more than 120 months, quarters or years is
// refused with ErrRangeTooLarge; the payload target is measured, never a reason
// to silently reduce this range.
const MaxPeriods = 120

// MaxExactMinor is the largest amount, ratio or count a report may emit:
// 2^53-1, JavaScript's Number.MAX_SAFE_INTEGER. int64 is wider, but every
// figure crosses JSON into the desktop's JS runtime as a number, which cannot
// name every integer above this bound.
const MaxExactMinor int64 = 1<<53 - 1

var (
	// ErrInvalid reports a malformed date, enum or range. The HTTP handler maps
	// it to 400 report_invalid.
	ErrInvalid = errors.New("purchase order report query is invalid")
	// ErrRangeTooLarge reports a well-formed range that intersects more than
	// MaxPeriods calendar buckets. The HTTP handler maps it to 422
	// report_range_too_large.
	ErrRangeTooLarge = errors.New("purchase order report range exceeds the period cap")
	// ErrValueOutOfRange reports a numeric result outside the exact JSON range.
	// The HTTP handler maps actual-report overflow to 422
	// report_value_out_of_range; a projection marks its series unavailable and
	// keeps the actual figures.
	ErrValueOutOfRange = errors.New("purchase order report value is outside the exact range")
)

// Report query enums. They are the wire vocabulary; a caller may not invent a
// bucket or category.
const (
	BucketMonth   = "month"
	BucketQuarter = "quarter"
	BucketYear    = "year"

	CategoryAll           = "all"
	CategoryFm            = "FM"
	CategoryFab           = "FAB"
	CategorySupplyGeneral = "SUPPLY_GEN"
	CategoryOthers        = "OTHERS"
	CategoryUnassigned    = "UNASSIGNED"
)

// Projection availability. A projected bucket is never actual evidence.
const (
	ProjectionStatusAvailable   = "available"
	ProjectionStatusUnavailable = "unavailable"
)

// Projection reason codes. They are stable identifiers, not prose: the screen,
// the PDF and the API notes all render the same code.
const (
	ReasonNoDataInRange       = "no_data_in_range"
	ReasonInsufficientHistory = "insufficient_history"
	ReasonIncompleteCostBasis = "incomplete_cost_basis"
	ReasonValueOutOfRange     = "value_out_of_range"
	// ReasonInvalidQuery is defensive only: a report endpoint normalizes its
	// query before projection, so a client never receives this code.
	ReasonInvalidQuery = "invalid_query"
)

// Projected bucket sources. They name the observations behind the base value.
const (
	SourceSameCalendarMonth            = "same_calendar_month"
	SourceTrailingThreeCompleteMonths  = "trailing_three_complete_months"
	SourceTrailingTwelveCompleteMonths = "trailing_twelve_complete_months"
)

// Projection methods state how the series was built. They deliberately do not
// call the band a confidence interval.
const (
	MethodMonthly = "Median of the same calendar month across complete history; P25-P75 range (min-max under four observations), trailing three complete months when that calendar month has no history."
	MethodYearly  = "The last twelve complete months held flat; P25-P75 range of rolling twelve-month totals, widened only to include the base."
)

// Query is the validated report filter. From is inclusive and To exclusive.
// Category is one of CategoryAll, the four business codes or
// CategoryUnassigned; currency is one partition and is never converted.
type Query struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Bucket   string `json:"bucket"`
	Category string `json:"category"`
	Currency string `json:"currency"`
}

// Metrics are one period, category or whole-report set of figures. Amount is
// the sell total excluding PPN. Costed is the amount on lines whose exact source
// cost resolved, Capital is the pinned source cost at the ordered quantity, and
// Uncosted is amount-costed. Margin is costed-capital and is signed. MarginBps
// and CoverageBps are null, never zero, when their denominator is zero.
type Metrics struct {
	PoCount       int64  `json:"poCount"`
	AmountMinor   int64  `json:"amountMinor"`
	CostedMinor   int64  `json:"costedMinor"`
	UncostedMinor int64  `json:"uncostedMinor"`
	CapitalMinor  int64  `json:"capitalMinor"`
	MarginMinor   int64  `json:"marginMinor"`
	MarginBps     *int64 `json:"marginBps"`
	CoverageBps   *int64 `json:"coverageBps"`
}

// Company is the issuer snapshot the report was generated under.
type Company struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Population describes the facets a filter selector can offer without
// changing the selected currency or category.
type Population struct {
	AvailableCurrencies  []string `json:"availableCurrencies"`
	HasUnassigned        bool     `json:"hasUnassigned"`
	SourceMissingPoCount int64    `json:"sourceMissingPoCount"`
}

// Period is one actual calendar bucket. The metrics are embedded so the wire
// shape keeps the period metadata beside the figures.
type Period struct {
	PeriodStart string `json:"periodStart"`
	Partial     bool   `json:"partial"`
	InProgress  bool   `json:"inProgress"`
	Metrics
}

// Category is one actual business-category row.
type Category struct {
	BusinessCategory string `json:"businessCategory"`
	Label            string `json:"label"`
	Metrics
}

// CategorySeries carries one category's margin percentage aligned to the
// actual periods. All is not repeated here; a null entry means the category had
// zero capital in that period.
type CategorySeries struct {
	BusinessCategory string   `json:"businessCategory"`
	MarginBps        []*int64 `json:"marginBps"`
}

// Note is a report disclosure such as PPN exclusion, uncosted money or a
// projection that could not be produced.
type Note struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// Report is the complete read-only snapshot the screen and the PDF render.
// GeneratedAt and AsOf are one timestamp pair for the whole response.
type Report struct {
	GeneratedAt    string           `json:"generatedAt"`
	AsOf           string           `json:"asOf"`
	Filter         Query            `json:"filter"`
	Company        Company          `json:"company"`
	Population     Population       `json:"population"`
	Totals         Metrics          `json:"totals"`
	Periods        []Period         `json:"periods"`
	Categories     []Category       `json:"categories"`
	CategorySeries []CategorySeries `json:"categorySeries"`
	Projection     Projection       `json:"projection"`
	Notes          []Note           `json:"notes"`
}

// Projection holds the two independent forecast series. A quarterly actual
// view still receives monthly and yearly series; no quarterly forecast formula
// is invented.
type Projection struct {
	Monthly ProjectionSeries `json:"monthly"`
	Yearly  ProjectionSeries `json:"yearly"`
}

// ProjectionSeries is available with buckets, or unavailable with a stable
// reason. Basis is present either way so staff can judge the sample.
type ProjectionSeries struct {
	Status  string            `json:"status"`
	Reason  string            `json:"reason,omitempty"`
	Method  string            `json:"method"`
	Basis   ProjectionBasis   `json:"basis"`
	Buckets []ProjectedBucket `json:"buckets"`
}

// ProjectionBasis is the complete-month window and sample behind a series.
// HistoryFrom is the first observed PO month and is included in CompleteMonths;
// HistoryTo is exclusive. Zero months inside that window are real observations.
type ProjectionBasis struct {
	HistoryFrom    string `json:"historyFrom"`
	HistoryTo      string `json:"historyTo"`
	CompleteMonths int    `json:"completeMonths"`
	PoCount        int64  `json:"poCount"`
	AsOf           string `json:"asOf"`
}

// ProjectedBucket is one forecast month or year. Only the amount carries a
// band; capital and margin are single derived figures. Source names the
// observations behind the base.
type ProjectedBucket struct {
	PeriodStart     string `json:"periodStart"`
	AmountLowMinor  int64  `json:"amountLowMinor"`
	AmountBaseMinor int64  `json:"amountBaseMinor"`
	AmountHighMinor int64  `json:"amountHighMinor"`
	CapitalMinor    int64  `json:"capitalMinor"`
	MarginMinor     int64  `json:"marginMinor"`
	MarginBps       *int64 `json:"marginBps"`
	SampleSize      int    `json:"sampleSize"`
	Source          string `json:"source"`
}

// HistoryMonth is one complete calendar month of the report's population.
// MonthStart is YYYY-MM-01. The caller filters organization, currency and
// category before calling Project.
type HistoryMonth struct {
	MonthStart string `json:"monthStart"`
	Metrics
}
