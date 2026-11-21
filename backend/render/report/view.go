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

// ViewCategories is the selected category section. In this ticket it is the
// legacy-equivalent aggregate row set from the overview source.
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

	if view.Periods.Layout != ViewLayoutIndividual {
		return fmt.Errorf("purchase order report view: periods layout %q is not supported by this build", view.Periods.Layout)
	}
	if view.Categories.Layout != ViewLayoutAggregate {
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

type resolvedView struct {
	View       ViewEnvelope
	Overview   poreport.Report
	Months     []resolvedMonth
	Categories poreport.Report
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
	if len(view.Periods.Sources) == 0 {
		return resolvedView{}, fmt.Errorf("purchase order report view: periods.sources is empty")
	}
	if len(view.Periods.Months) == 0 {
		return resolvedView{}, fmt.Errorf("purchase order report view: periods.months is empty")
	}
	if len(view.Periods.Months) > MaxViewMonths {
		return resolvedView{}, fmt.Errorf("purchase order report view: %d months; the cap is %d",
			len(view.Periods.Months), MaxViewMonths)
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
	resolved := make([]resolvedMonth, 0, len(view.Periods.Months))
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

	// The 120-month span is inclusive of both endpoint months.
	first, _ := parseViewMonth(view.Periods.Months[0])
	last, _ := parseViewMonth(view.Periods.Months[len(view.Periods.Months)-1])
	if last-first+1 > MaxViewMonthSpan {
		return resolvedView{}, fmt.Errorf("purchase order report view: selected months span %d months; the cap is %d",
			last-first+1, MaxViewMonthSpan)
	}
	if view.Periods.CurrentMonthSource != nil {
		if _, selected := seenMonths[currentKey]; !selected {
			return resolvedView{}, fmt.Errorf("purchase order report view: currentMonthSource is set but the current month %s is not selected",
				currentKey)
		}
	}

	categories := view.Sources[view.Categories.Source]
	seenCodes := make(map[string]struct{}, len(view.Categories.Codes))
	for index, code := range view.Categories.Codes {
		if !canonicalCategoryCode(code) {
			return resolvedView{}, fmt.Errorf("purchase order report view: categories.codes[%d] = %q is not a canonical business code",
				index, code)
		}
		if _, duplicate := seenCodes[code]; duplicate {
			return resolvedView{}, fmt.Errorf("purchase order report view: category %s is selected twice", code)
		}
		seenCodes[code] = struct{}{}
		if !categoryPresent(categories, code) {
			return resolvedView{}, fmt.Errorf("purchase order report view: category %s is not present in source %d",
				code, view.Categories.Source)
		}
	}
	if len(view.Categories.Codes) != len(categories.Categories) {
		return resolvedView{}, fmt.Errorf("purchase order report view: aggregate categories select %d codes for %d rows",
			len(view.Categories.Codes), len(categories.Categories))
	}
	for index, category := range categories.Categories {
		if view.Categories.Codes[index] != category.BusinessCategory {
			return resolvedView{}, fmt.Errorf("purchase order report view: aggregate category %d is %s; want %s",
				index, view.Categories.Codes[index], category.BusinessCategory)
		}
	}

	return resolvedView{
		View:       view,
		Overview:   overview,
		Months:     resolved,
		Categories: categories,
	}, nil
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

func categoryPresent(source poreport.Report, code string) bool {
	for _, category := range source.Categories {
		if category.BusinessCategory == code {
			return true
		}
	}
	return false
}
