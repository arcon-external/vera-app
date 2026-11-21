package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"vera-app/backend/poreport"
)

// Issue #641's versioned export view. A payload without a `version` property
// stays the legacy report-only snapshot; a payload with any version property is
// decoded only as this envelope, and a version that is not exactly the integer
// 1 is refused. There is no permissive fallback between the two: a malformed
// view must never print as a legacy report that silently ignores the selection.
const (
	MaxViewSources       = 16
	MaxViewMonths        = 120
	MaxViewMonthSpan     = 120
	ViewLayoutAggregate  = "aggregate"
	ViewLayoutIndividual = "individual"
)

// EnvelopeVersion is the exact JSON integer 1. It is a named type so a string,
// null, zero, fraction or later version is refused by decoding rather than
// quietly compared.
type EnvelopeVersion int

// ViewVersion is the first (and, for this ticket, only) envelope version.
const ViewVersion EnvelopeVersion = 1

func (version *EnvelopeVersion) UnmarshalJSON(data []byte) error {
	text := string(bytes.TrimSpace(data))
	if text != "1" {
		return fmt.Errorf("purchase order report view: version %s is not supported", text)
	}
	*version = ViewVersion
	return nil
}

// ViewEnvelope references a bounded set of immutable report snapshots and the
// per-section selections that describe one export. Sources are referenced by
// index so identical snapshots are never duplicated per card.
type ViewEnvelope struct {
	Version    EnvelopeVersion   `json:"version"`
	Sources    []poreport.Report `json:"sources"`
	Overview   int               `json:"overview"`
	Periods    ViewPeriods       `json:"periods"`
	Categories ViewCategories    `json:"categories"`
}

// ViewPeriods is the selected month section. Months are canonical YYYY-MM
// identifiers, unique and chronological. CurrentMonthSource names the
// full-current-month/day snapshot whose server monthSummary owns the current
// month, when one is used.
type ViewPeriods struct {
	Sources            []int    `json:"sources"`
	Months             []string `json:"months"`
	Layout             string   `json:"layout"`
	CurrentMonthSource *int     `json:"currentMonthSource,omitempty"`
}

// ViewCategories is the selected category section. Source names the immutable
// snapshot the rows are read from; it must share the overview's from/to/bucket
// and is either the unrestricted all-category source or the overview's own
// restricted category (the #641 compatibility envelope). Layout is the display:
// "aggregate" prints the wide table over every supplied row, "individual" prints
// one card per selected row. Codes are canonical business codes in the source's
// supplied order; an empty list is an explicit empty section, not a zero-valued
// comparison.
type ViewCategories struct {
	Source int      `json:"source"`
	Codes  []string `json:"codes"`
	Layout string   `json:"layout"`
}

// ExportDocument is exactly one of the two accepted export payloads. It exists
// so the binding can open the save dialog only after the shape is known and so
// tests can prove the legacy path never becomes a view or vice versa.
type ExportDocument struct {
	Legacy *poreport.Report
	View   *ViewEnvelope
}

// Overview returns the report whose header, KPI strip, chart and notes the
// document prints. A legacy payload is its own overview; a view names one
// source.
func (document ExportDocument) Overview() poreport.Report {
	if document.Legacy != nil {
		return *document.Legacy
	}
	return document.View.Sources[document.View.Overview]
}

// DecodeExport picks the path from the presence of the version property, then
// applies only that path's decoder. The size cap is the legacy snapshot cap;
// a view is larger only in its bounded source array and must still fit.
func DecodeExport(raw []byte) (ExportDocument, error) {
	if len(raw) > SnapshotLimit {
		return ExportDocument{}, fmt.Errorf(
			"purchase order report export is %d bytes; the limit is %d", len(raw), SnapshotLimit)
	}
	var header struct {
		Version json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return ExportDocument{}, fmt.Errorf("decode purchase order report export: %w", err)
	}
	if len(header.Version) == 0 {
		snapshot, err := DecodeSnapshot(raw)
		if err != nil {
			return ExportDocument{}, err
		}
		return ExportDocument{Legacy: &snapshot}, nil
	}
	view, err := DecodeView(raw)
	if err != nil {
		return ExportDocument{}, err
	}
	return ExportDocument{View: &view}, nil
}

// DecodeView decodes and validates a version-1 envelope only. It refuses a
// missing version, unknown fields, trailing data, an unknown version and every
// semantic mismatch the renderer cannot print truthfully.
func DecodeView(raw []byte) (ViewEnvelope, error) {
	if len(raw) > SnapshotLimit {
		return ViewEnvelope{}, fmt.Errorf(
			"purchase order report view is %d bytes; the limit is %d", len(raw), SnapshotLimit)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var view ViewEnvelope
	if err := decoder.Decode(&view); err != nil {
		return ViewEnvelope{}, fmt.Errorf("decode purchase order report view: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return ViewEnvelope{}, fmt.Errorf("decode purchase order report view: trailing data")
		}
		return ViewEnvelope{}, fmt.Errorf("decode purchase order report view: %w", err)
	}
	if err := ValidateView(view); err != nil {
		return ViewEnvelope{}, err
	}
	return view, nil
}

// ValidateView applies the envelope's structural, scope and reference rules.
// It performs no financial arithmetic: every figure is already a frozen server
// value inside one of the sources.
func ValidateView(view ViewEnvelope) error {
	if view.Version != ViewVersion {
		return fmt.Errorf("purchase order report view: version %d is not supported", view.Version)
	}
	if len(view.Sources) == 0 {
		return fmt.Errorf("purchase order report view: at least one source is required")
	}
	if len(view.Sources) > MaxViewSources {
		return fmt.Errorf("purchase order report view: %d sources; the cap is %d",
			len(view.Sources), MaxViewSources)
	}
	if view.Overview < 0 || view.Overview >= len(view.Sources) {
		return fmt.Errorf("purchase order report view: overview source %d is out of range", view.Overview)
	}

	seenQueries := make(map[string]int, len(view.Sources))
	for index := range view.Sources {
		if err := ValidateSnapshot(view.Sources[index]); err != nil {
			return fmt.Errorf("purchase order report view: sources[%d]: %w", index, err)
		}
		query, err := poreport.NormalizeQuery(view.Sources[index].Filter)
		if err != nil {
			return fmt.Errorf("purchase order report view: sources[%d]: %w", index, err)
		}
		key := viewQueryKey(query)
		if previous, ok := seenQueries[key]; ok {
			return fmt.Errorf("purchase order report view: sources[%d] duplicates the query of sources[%d]; reuse the index",
				index, previous)
		}
		seenQueries[key] = index
	}

	baseline := view.Sources[view.Overview]
	baselineQuery, err := poreport.NormalizeQuery(baseline.Filter)
	if err != nil {
		return fmt.Errorf("purchase order report view: overview filter: %w", err)
	}
	for index := range view.Sources {
		source := view.Sources[index]
		query, err := poreport.NormalizeQuery(source.Filter)
		if err != nil {
			return fmt.Errorf("purchase order report view: sources[%d]: %w", index, err)
		}
		if query.Currency != baselineQuery.Currency {
			return fmt.Errorf("purchase order report view: sources[%d] currency %s differs from the overview's %s",
				index, query.Currency, baselineQuery.Currency)
		}
		if source.AsOf != baseline.AsOf {
			return fmt.Errorf("purchase order report view: sources[%d] as-of %s differs from the overview's %s; refresh before exporting",
				index, source.AsOf, baseline.AsOf)
		}
	}

	if view.Periods.Layout != ViewLayoutIndividual && view.Periods.Layout != ViewLayoutAggregate {
		return fmt.Errorf("purchase order report view: periods layout %q is not supported by this build", view.Periods.Layout)
	}
	switch view.Categories.Layout {
	case ViewLayoutAggregate, ViewLayoutIndividual:
	default:
		return fmt.Errorf("purchase order report view: categories layout %q is not supported by this build", view.Categories.Layout)
	}
	if _, err := resolveView(view); err != nil {
		return err
	}
	return nil
}

type resolvedMonth struct {
	Month       string
	SourceIndex int
	Source      poreport.Report
	// Period is the supplied monthly row when the resolved source is a
	// month-bucketed annual source. A current-month/day source has no monthly
	// row; its server monthSummary carries the actual evidence instead.
	Period    *poreport.Period
	Summary   *poreport.MonthSummary
	IsCurrent bool
	IsFuture  bool
}

type resolvedAggregateMonth struct {
	Month string
	Row   *poreport.Period
	// Recorded is true for every past and current month row and for a future
	// month whose supplied row carries a recorded purchase order. A future
	// month with no recorded purchase order prints dashes, never a zero actual.
	Recorded    bool
	IsCurrent   bool
	IsFuture    bool
	FutureDated bool
}

// resolvedAggregate is the one-calendar-year All view. It reads its month rows
// and its total from one immutable annual snapshot; it never sums rows and
// never combines an estimate with a recorded figure.
type resolvedAggregate struct {
	Year                string
	SourceIndex         int
	Source              poreport.Report
	Months              []resolvedAggregateMonth
	IncludesFutureDated bool
}

type resolvedCategory struct {
	Code    string
	Metrics poreport.Metrics
}

type resolvedView struct {
	View       ViewEnvelope
	Overview   poreport.Report
	Months     []resolvedMonth
	Categories poreport.Report
	// ResolvedCategories is the selected category rows in the category source's
	// supplied order. It is empty for an explicit empty selection and holds one
	// entry per code otherwise; the renderer prints these rows and never
	// re-orders them.
	ResolvedCategories []resolvedCategory
	Aggregate          *resolvedAggregate
}

func (resolved resolvedView) currentMonthKey() string {
	return firstSeven(resolved.Overview.AsOf)
}

func firstSeven(date string) string {
	if len(date) < 7 {
		return date
	}
	return date[:7]
}

func resolveView(view ViewEnvelope) (resolvedView, error) {
	if view.Overview < 0 || view.Overview >= len(view.Sources) {
		return resolvedView{}, fmt.Errorf("purchase order report view: overview source %d is out of range", view.Overview)
	}
	if view.Categories.Source < 0 || view.Categories.Source >= len(view.Sources) {
		return resolvedView{}, fmt.Errorf("purchase order report view: categories source %d is out of range", view.Categories.Source)
	}
	if len(view.Periods.Months) > MaxViewMonths {
		return resolvedView{}, fmt.Errorf("purchase order report view: %d months; the cap is %d",
			len(view.Periods.Months), MaxViewMonths)
	}
	if len(view.Periods.Months) == 0 && view.Periods.CurrentMonthSource != nil {
		return resolvedView{}, fmt.Errorf("purchase order report view: an empty selection cannot set currentMonthSource")
	}

	refs := make(map[int]struct{}, len(view.Periods.Sources))
	overviewQuery, err := poreport.NormalizeQuery(view.Sources[view.Overview].Filter)
	if err != nil {
		return resolvedView{}, fmt.Errorf("purchase order report view: overview filter: %w", err)
	}
	for _, ref := range view.Periods.Sources {
		if ref < 0 || ref >= len(view.Sources) {
			return resolvedView{}, fmt.Errorf("purchase order report view: period source %d is out of range", ref)
		}
		if _, duplicate := refs[ref]; duplicate {
			return resolvedView{}, fmt.Errorf("purchase order report view: period source %d is referenced twice", ref)
		}
		query, err := poreport.NormalizeQuery(view.Sources[ref].Filter)
		if err != nil {
			return resolvedView{}, fmt.Errorf("purchase order report view: period source %d filter: %w", ref, err)
		}
		if query.Category != overviewQuery.Category {
			return resolvedView{}, fmt.Errorf("purchase order report view: period source %d category %s differs from the overview's %s",
				ref, query.Category, overviewQuery.Category)
		}
		refs[ref] = struct{}{}
	}

	overview := view.Sources[view.Overview]
	currentKey := firstSeven(overview.AsOf)
	var currentMonthSource *poreport.Report
	if view.Periods.CurrentMonthSource != nil {
		ref := *view.Periods.CurrentMonthSource
		if ref < 0 || ref >= len(view.Sources) {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource %d is out of range", ref)
		}
		if _, referenced := refs[ref]; !referenced {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource %d is not referenced by periods.sources", ref)
		}
		source := view.Sources[ref]
		summary := source.MonthSummary
		if summary == nil {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource %d has no monthSummary", ref)
		}
		if summary.Month != currentKey || summary.Mode != poreport.ProjectionModeDay {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource %d is not this month's daily readout", ref)
		}
		query, err := poreport.NormalizeQuery(source.Filter)
		if err != nil {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource filter: %w", err)
		}
		if query.Bucket != poreport.BucketDay ||
			query.From != currentKey+"-01" ||
			query.To != nextMonthFirst(currentKey) ||
			firstSeven(source.AsOf) != currentKey {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource %d is not a full-current-month/day snapshot", ref)
		}
		currentMonthSource = &view.Sources[ref]
	}

	seenMonths := make(map[string]struct{}, len(view.Periods.Months))
	previousIndex := -1
	for _, month := range view.Periods.Months {
		index, ok := parseViewMonth(month)
		if !ok {
			return resolvedView{}, fmt.Errorf("purchase order report view: month %q is not a canonical YYYY-MM identifier", month)
		}
		if _, duplicate := seenMonths[month]; duplicate {
			return resolvedView{}, fmt.Errorf("purchase order report view: month %s is selected twice", month)
		}
		seenMonths[month] = struct{}{}
		if previousIndex >= 0 && index <= previousIndex {
			return resolvedView{}, fmt.Errorf("purchase order report view: months are not strictly chronological at %s", month)
		}
		previousIndex = index
	}

	var resolved []resolvedMonth
	var aggregate *resolvedAggregate
	switch {
	case len(view.Periods.Months) == 0:
		// An explicit empty selection prints an explicit empty section. It has
		// no financial total and requires no period source.
	case view.Periods.Layout == ViewLayoutAggregate:
		found, err := resolveAggregateMonths(view, currentKey)
		if err != nil {
			return resolvedView{}, err
		}
		aggregate = found
	default:
		resolved = make([]resolvedMonth, 0, len(view.Periods.Months))
		for _, month := range view.Periods.Months {
			isCurrent := month == currentKey
			isFuture := month > currentKey
			var sourceIndex int
			var periodRow *poreport.Period
			var summary *poreport.MonthSummary

			switch {
			case isCurrent && currentMonthSource != nil:
				sourceIndex = *view.Periods.CurrentMonthSource
				summary = currentMonthSource.MonthSummary
			default:
				found, err := resolveAnnualMonth(view, month, refs)
				if err != nil {
					return resolvedView{}, err
				}
				sourceIndex = found.sourceIndex
				periodRow = found.period
				if isCurrent && view.Sources[sourceIndex].MonthSummary != nil {
					summary = view.Sources[sourceIndex].MonthSummary
				}
			}

			source := view.Sources[sourceIndex]
			if summary != nil && summary.Month != month {
				return resolvedView{}, fmt.Errorf("purchase order report view: month %s resolved to a monthSummary for %s",
					month, summary.Month)
			}
			if isCurrent && summary == nil {
				return resolvedView{}, fmt.Errorf("purchase order report view: current month %s has no server monthSummary", month)
			}
			resolved = append(resolved, resolvedMonth{
				Month:       month,
				SourceIndex: sourceIndex,
				Source:      source,
				Period:      periodRow,
				Summary:     summary,
				IsCurrent:   isCurrent,
				IsFuture:    isFuture,
			})
		}
	}

	if len(view.Periods.Months) > 0 {
		// The 120-month span is inclusive of both endpoint months.
		first, _ := parseViewMonth(view.Periods.Months[0])
		last, _ := parseViewMonth(view.Periods.Months[len(view.Periods.Months)-1])
		if last-first+1 > MaxViewMonthSpan {
			return resolvedView{}, fmt.Errorf("purchase order report view: selected months span %d months; the cap is %d",
				last-first+1, MaxViewMonthSpan)
		}
	}
	if view.Periods.CurrentMonthSource != nil {
		if _, selected := seenMonths[currentKey]; !selected {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource is set but the current month %s is not selected",
				currentKey)
		}
	}

	categories := view.Sources[view.Categories.Source]
	categoriesQuery, err := poreport.NormalizeQuery(categories.Filter)
	if err != nil {
		return resolvedView{}, fmt.Errorf("purchase order report view: categories source %d filter: %w",
			view.Categories.Source, err)
	}
	if categoriesQuery.From != overviewQuery.From ||
		categoriesQuery.To != overviewQuery.To ||
		categoriesQuery.Bucket != overviewQuery.Bucket {
		return resolvedView{}, fmt.Errorf(
			"purchase order report view: categories source %d is %s..%s by %s; the overview is %s..%s by %s",
			view.Categories.Source,
			categoriesQuery.From, categoriesQuery.To, categoriesQuery.Bucket,
			overviewQuery.From, overviewQuery.To, overviewQuery.Bucket)
	}
	// The category section may read the unrestricted all-category universe under
	// a Performance-restricted overview, or inherit the overview's own category
	// (#641 compatibility). Any other category would print rows that answer a
	// different question than the caption claims, so it is refused.
	if categoriesQuery.Category != poreport.CategoryAll &&
		categoriesQuery.Category != overviewQuery.Category {
		return resolvedView{}, fmt.Errorf(
			"purchase order report view: categories source %d category %s is neither all nor the overview's %s",
			view.Categories.Source, categoriesQuery.Category, overviewQuery.Category)
	}

	rowByCode := make(map[string]int, len(categories.Categories))
	for index, category := range categories.Categories {
		if _, duplicate := rowByCode[category.BusinessCategory]; duplicate {
			return resolvedView{}, fmt.Errorf("purchase order report view: categories source %d lists %s twice",
				view.Categories.Source, category.BusinessCategory)
		}
		rowByCode[category.BusinessCategory] = index
	}

	selected := make([]resolvedCategory, 0, len(view.Categories.Codes))
	seenCodes := make(map[string]struct{}, len(view.Categories.Codes))
	previousRow := -1
	for index, code := range view.Categories.Codes {
		if !canonicalCategoryCode(code) {
			return resolvedView{}, fmt.Errorf("purchase order report view: categories.codes[%d] = %q is not a canonical business code",
				index, code)
		}
		if _, duplicate := seenCodes[code]; duplicate {
			return resolvedView{}, fmt.Errorf("purchase order report view: category %s is selected twice", code)
		}
		seenCodes[code] = struct{}{}
		row, present := rowByCode[code]
		if !present {
			return resolvedView{}, fmt.Errorf("purchase order report view: category %s is not present in source %d",
				code, view.Categories.Source)
		}
		// The supplied order is the report's own order. A caller that reorders a
		// selection is refused rather than silently re-sorted: the export must
		// print what the source says, not an order the reader cannot find again.
		if row <= previousRow {
			return resolvedView{}, fmt.Errorf(
				"purchase order report view: categories are not in the source's supplied order at %s", code)
		}
		previousRow = row
		category := categories.Categories[row]
		selected = append(selected, resolvedCategory{
			Code:    code,
			Metrics: category.Metrics,
		})
	}
	// An aggregate is the whole supplied row set or nothing. A non-empty subset
	// under aggregate would print a wide table whose total is the server's
	// whole-source total; that combination would read as a subset total.
	if len(view.Categories.Codes) > 0 && view.Categories.Layout == ViewLayoutAggregate &&
		len(selected) != len(categories.Categories) {
		return resolvedView{}, fmt.Errorf("purchase order report view: aggregate categories select %d codes for %d rows",
			len(selected), len(categories.Categories))
	}

	return resolvedView{
		View:               view,
		Overview:           overview,
		Months:             resolved,
		Categories:         categories,
		ResolvedCategories: selected,
		Aggregate:          aggregate,
	}, nil
}

// resolveAggregateMonths validates the one-calendar-year All/aggregate shape
// and reads its twelve rows from the single exact annual snapshot. The month
// count, order and year are enforced here because the layout has no year field:
// twelve chronological months are one year only when they are January through
// December of the same year.
func resolveAggregateMonths(view ViewEnvelope, currentKey string) (*resolvedAggregate, error) {
	months := view.Periods.Months
	if len(months) != 12 {
		return nil, fmt.Errorf("purchase order report view: aggregate needs twelve months; got %d", len(months))
	}
	year := months[0][:4]
	for index, month := range months {
		want := fmt.Sprintf("%s-%02d", year, index+1)
		if month != want {
			return nil, fmt.Errorf("purchase order report view: aggregate months must be the twelve calendar months of one year; month %d is %s, want %s",
				index, month, want)
		}
	}
	from := year + "-01-01"
	to := nextYearFirst(year)
	sourceIndex := -1
	for _, ref := range view.Periods.Sources {
		query, err := poreport.NormalizeQuery(view.Sources[ref].Filter)
		if err != nil {
			return nil, fmt.Errorf("purchase order report view: aggregate source %d filter: %w", ref, err)
		}
		if query.Bucket != poreport.BucketMonth || query.From != from || query.To != to {
			continue
		}
		if sourceIndex >= 0 {
			return nil, fmt.Errorf("purchase order report view: aggregate year %s is covered by more than one source", year)
		}
		sourceIndex = ref
	}
	if sourceIndex < 0 {
		return nil, fmt.Errorf("purchase order report view: aggregate year %s has no referenced annual/monthly source", year)
	}
	source := view.Sources[sourceIndex]
	byStart := make(map[string]*poreport.Period, len(source.Periods))
	for index := range source.Periods {
		byStart[source.Periods[index].PeriodStart] = &source.Periods[index]
	}
	resolved := resolvedAggregate{
		Year:        year,
		SourceIndex: sourceIndex,
		Source:      source,
		Months:      make([]resolvedAggregateMonth, 0, 12),
	}
	for _, month := range months {
		row := byStart[month+"-01"]
		if row == nil {
			return nil, fmt.Errorf("purchase order report view: aggregate source %d has no row for %s", sourceIndex, month)
		}
		isCurrent := month == currentKey
		isFuture := month > currentKey
		recorded := !isFuture || row.PoCount > 0
		futureDated := isCurrent && currentMonthRowCarriesFutureDated(source, *row)
		if isFuture && row.PoCount > 0 {
			resolved.IncludesFutureDated = true
		}
		if futureDated {
			resolved.IncludesFutureDated = true
		}
		resolved.Months = append(resolved.Months, resolvedAggregateMonth{
			Month:       month,
			Row:         row,
			Recorded:    recorded,
			IsCurrent:   isCurrent,
			IsFuture:    isFuture,
			FutureDated: futureDated,
		})
	}
	return &resolved, nil
}

// currentMonthRowCarriesFutureDated reports whether the annual current-month
// row differs from the server's actual-to-date readout, which is the evidence
// that at least one record is dated after the as-of date. It compares supplied
// figures field by field; it performs no subtraction and invents no count.
func currentMonthRowCarriesFutureDated(source poreport.Report, row poreport.Period) bool {
	var actual *poreport.Metrics
	if source.MonthSummary != nil && source.MonthSummary.Month == firstSeven(source.AsOf) {
		value := source.MonthSummary.Actual
		actual = &value
	} else if source.CurrentProjection.Mode == poreport.ProjectionModeMonth &&
		source.CurrentProjection.CurrentMonth != nil {
		value := source.CurrentProjection.CurrentMonth.Actual
		actual = &value
	}
	if actual == nil {
		return false
	}
	return row.PoCount != actual.PoCount ||
		row.AmountMinor != actual.AmountMinor ||
		row.CostedMinor != actual.CostedMinor ||
		row.UncostedMinor != actual.UncostedMinor ||
		row.CapitalMinor != actual.CapitalMinor ||
		row.MarginMinor != actual.MarginMinor
}

type annualResolution struct {
	sourceIndex int
	period      *poreport.Period
}

// resolveAnnualMonth finds the one referenced month-bucketed snapshot whose
// half-open filter is exactly the month's calendar year and which supplies a
// row for that month. Equal year overlap is not enough: the range must be the
// complete year, which is what makes the current-year forecast legal and the
// retired partial-year snapshot useless here.
func resolveAnnualMonth(view ViewEnvelope, month string, refs map[int]struct{}) (annualResolution, error) {
	year := month[:4]
	from := year + "-01-01"
	next := nextYearFirst(year)
	var resolved *annualResolution
	for ref := range refs {
		source := view.Sources[ref]
		query, err := poreport.NormalizeQuery(source.Filter)
		if err != nil {
			return annualResolution{}, fmt.Errorf("purchase order report view: source %d filter: %w", ref, err)
		}
		if query.Bucket != poreport.BucketMonth || query.From != from || query.To != next {
			continue
		}
		var row *poreport.Period
		for index := range source.Periods {
			if source.Periods[index].PeriodStart == month+"-01" {
				row = &source.Periods[index]
				break
			}
		}
		if row == nil {
			continue
		}
		if resolved != nil {
			return annualResolution{}, fmt.Errorf("purchase order report view: month %s is covered by more than one source", month)
		}
		resolved = &annualResolution{sourceIndex: ref, period: row}
	}
	if resolved == nil {
		return annualResolution{}, fmt.Errorf("purchase order report view: month %s has no exact annual/monthly source", month)
	}
	return *resolved, nil
}

func viewQueryKey(query poreport.Query) string {
	return strings.Join([]string{
		query.From, query.To, query.Bucket, query.Category, query.Currency,
	}, "|")
}

func parseViewMonth(value string) (int, bool) {
	if len(value) != 7 || value[4] != '-' {
		return 0, false
	}
	parsed, err := time.Parse("2006-01", value)
	if err != nil || parsed.Format("2006-01") != value {
		return 0, false
	}
	return parsed.Year()*12 + int(parsed.Month()) - 1, true
}

func nextMonthFirst(month string) string {
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return ""
	}
	return parsed.AddDate(0, 1, 0).Format(time.DateOnly)
}

func nextYearFirst(year string) string {
	parsed, err := time.Parse("2006", year)
	if err != nil {
		return ""
	}
	return parsed.AddDate(1, 0, 0).Format(time.DateOnly)
}

func canonicalCategoryCode(code string) bool {
	switch code {
	case poreport.CategoryFm, poreport.CategoryFab, poreport.CategorySupplyGeneral,
		poreport.CategoryOthers, poreport.CategoryUnassigned:
		return true
	default:
		return false
	}
}
