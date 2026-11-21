package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// Issue #641: the versioned export view. These tests pin the two-path decode
// (strict legacy versus version-1 view, never a fallback), the scope/reference
// rules, and the PDF composition of a selected month. The financial arithmetic
// is pinned in backend/poreport; this package only selects frozen values.

func viewCodes(report poreport.Report) []string {
	codes := make([]string, 0, len(report.Categories))
	for _, category := range report.Categories {
		codes = append(codes, category.BusinessCategory)
	}
	return codes
}

func validPastMonthView(t *testing.T) ViewEnvelope {
	t.Helper()
	source := currentYearReport()
	return ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{source},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0},
			Months:  []string{"2026-10"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{
			Source: 0,
			Codes:  viewCodes(source),
			Layout: ViewLayoutAggregate,
		},
	}
}

func intPointer(value int) *int { return &value }

func TestDecodeExportKeepsTheLegacyReportOnlyPath(t *testing.T) {
	legacy := sampleReport()
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	document, err := DecodeExport(raw)
	if err != nil {
		t.Fatalf("DecodeExport(legacy): %v", err)
	}
	if document.Legacy == nil || document.View != nil {
		t.Fatalf("document = %+v; want the legacy path only", document)
	}
	if document.Legacy.Filter != legacy.Filter {
		t.Errorf("legacy filter = %+v; want the decoded snapshot unchanged", document.Legacy.Filter)
	}

	pdf, err := RenderExport(document, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderExport(legacy): %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text, "By period", "PERIOD")
	// The legacy composition still prints its bucket-following table.
	requireText(t, text, "Partial and in-progress periods are marked in the period table.")
	if strings.Contains(text, "By period —") {
		t.Error("legacy export printed the versioned selected-month heading")
	}
}

func TestDecodeExportRejectsEveryUnsupportedVersionShape(t *testing.T) {
	valid := validPastMonthView(t)
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal view object: %v", err)
	}
	cases := []struct {
		name    string
		version string
	}{
		{name: "zero", version: "0"},
		{name: "two", version: "2"},
		{name: "string", version: `"1"`},
		{name: "null", version: "null"},
		{name: "fraction", version: "1.5"},
		{name: "object", version: `{"n":1}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			object["version"] = json.RawMessage(test.version)
			mutated, err := json.Marshal(object)
			if err != nil {
				t.Fatalf("marshal mutated view: %v", err)
			}
			if _, err := DecodeExport(mutated); err == nil {
				t.Fatal("DecodeExport accepted an unsupported version")
			}
		})
	}

	// A missing version is not a view: it must be decoded as strict legacy and
	// refused because a view's sources/periods are unknown legacy fields.
	delete(object, "version")
	mutated, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal versionless view: %v", err)
	}
	if _, err := DecodeExport(mutated); err == nil {
		t.Fatal("DecodeExport routed a versionless envelope to legacy without refusing its fields")
	}
}

func TestDecodeViewRefusesUnknownFieldsAndTrailingData(t *testing.T) {
	valid := validPastMonthView(t)
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	object["selection"] = json.RawMessage(`{"x":1}`)
	unknown, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal unknown-field view: %v", err)
	}
	if _, err := DecodeView(unknown); err == nil {
		t.Fatal("DecodeView accepted an unknown envelope field")
	}

	var source map[string]json.RawMessage
	var sources []map[string]json.RawMessage
	if err := json.Unmarshal(object["sources"], &sources); err != nil {
		t.Fatalf("unmarshal sources: %v", err)
	}
	source = sources[0]
	source["selection"] = json.RawMessage(`"x"`)
	sources[0] = source
	rewritten, err := json.Marshal(sources)
	if err != nil {
		t.Fatalf("marshal sources: %v", err)
	}
	object["sources"] = rewritten
	delete(object, "selection")
	nested, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal nested unknown field: %v", err)
	}
	if _, err := DecodeView(nested); err == nil {
		t.Fatal("DecodeView accepted an unknown field inside a source")
	}

	trailing := append(append([]byte{}, raw...), []byte(` {"second":true}`)...)
	if _, err := DecodeView(trailing); err == nil {
		t.Fatal("DecodeView accepted trailing data")
	}
}

func TestValidateViewEnforcesScopeAndReferences(t *testing.T) {
	base := validPastMonthView(t)
	if err := ValidateView(base); err != nil {
		t.Fatalf("valid past-month view refused: %v", err)
	}

	t.Run("duplicate source query", func(t *testing.T) {
		view := base
		view.Sources = []poreport.Report{base.Sources[0], base.Sources[0]}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted two sources with the same exact query")
		}
	})
	t.Run("different as-of", func(t *testing.T) {
		view := base
		other := currentYearReport()
		other.AsOf = "2026-11-20"
		other.Filter.From = "2026-01-01"
		other.CurrentProjection = unavailableCurrentProjection(
			poreport.ProjectionModeNone, poreport.ReasonNotCurrentPeriod, poreport.MethodCurrentNone,
			poreport.CurrentProjectionBasis{AsOf: "2026-11-20"})
		view.Sources = []poreport.Report{base.Sources[0], other}
		view.Periods.Sources = []int{0}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted sources with different as-of dates")
		}
	})
	t.Run("different currency", func(t *testing.T) {
		view := base
		other := currentYearReport()
		other.Filter.Currency = "USD"
		view.Sources = []poreport.Report{base.Sources[0], other}
		view.Periods.Sources = []int{0}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted sources with different currencies")
		}
	})
	t.Run("period source category differs", func(t *testing.T) {
		view := base
		overview := base.Sources[0]
		overview.Filter.Category = poreport.CategoryFm
		view.Sources[0] = overview
		if err := ValidateView(view); err != nil {
			t.Fatalf("a period source in the inherited category should be valid: %v", err)
		}
		// The snapshot's rows still carry all categories, but the period source
		// itself is category=all under an FM overview: refused.
		other := currentYearReport()
		view.Sources = []poreport.Report{overview, other}
		view.Periods.Sources = []int{1}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a period source whose category differs from the overview")
		}
	})
	t.Run("category source may differ for the category section", func(t *testing.T) {
		// #643 regression: the category section will use category=all under an FM
		// overview. That must be a valid envelope now, not a permanent invariant.
		overview := currentYearReport()
		overview.Filter.Category = poreport.CategoryFm
		categories := currentYearReport()
		categories.Filter.Category = poreport.CategoryAll
		view := ViewEnvelope{
			Version:  ViewVersion,
			Sources:  []poreport.Report{overview, categories},
			Overview: 0,
			Periods: ViewPeriods{
				Sources: []int{0},
				Months:  []string{"2026-10"},
				Layout:  ViewLayoutIndividual,
			},
			Categories: ViewCategories{
				Source: 1,
				Codes:  viewCodes(categories),
				Layout: ViewLayoutAggregate,
			},
		}
		if err := ValidateView(view); err != nil {
			t.Fatalf("category-scoped source was refused: %v", err)
		}
	})
	t.Run("duplicate period source ref", func(t *testing.T) {
		view := base
		view.Periods.Sources = []int{0, 0}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a duplicate period source reference")
		}
	})
	t.Run("month ordering", func(t *testing.T) {
		view := base
		view.Periods.Months = []string{"2026-10", "2026-09"}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted non-chronological months")
		}
	})
	t.Run("duplicate month", func(t *testing.T) {
		view := base
		view.Periods.Months = []string{"2026-10", "2026-10"}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a duplicate month")
		}
	})
	t.Run("malformed month", func(t *testing.T) {
		view := base
		view.Periods.Months = []string{"2026-1"}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a non-canonical month")
		}
	})
	t.Run("month without annual source", func(t *testing.T) {
		view := base
		view.Periods.Months = []string{"2026-10", "2025-10"}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a month with no referenced annual source")
		}
	})
	t.Run("current month without summary", func(t *testing.T) {
		view := base
		view.Periods.Months = []string{"2026-11"}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted the current month without a server monthSummary")
		}
	})
	t.Run("out of range refs", func(t *testing.T) {
		view := base
		view.Periods.Sources = []int{3}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an out-of-range period source")
		}
		view = base
		view.Categories.Source = 3
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an out-of-range category source")
		}
		view = base
		view.Overview = 3
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an out-of-range overview source")
		}
	})
	t.Run("unsupported layouts", func(t *testing.T) {
		view := base
		view.Periods.Layout = "sideways"
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an unknown period layout")
		}
		// Aggregate is frozen vocabulary, but it requires all twelve months of
		// one calendar year; one month is not an aggregate.
		view = base
		view.Periods.Layout = ViewLayoutAggregate
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a single month as an aggregate")
		}
		view = base
		view.Categories.Layout = "sideways"
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an unknown category layout")
		}
	})
	t.Run("category codes", func(t *testing.T) {
		view := base
		view.Categories.Codes = append([]string{}, view.Categories.Codes...)
		view.Categories.Codes[0] = "NOPE"
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a non-canonical category code")
		}
		view = base
		view.Categories.Codes = append([]string{}, view.Categories.Codes...)
		view.Categories.Codes = view.Categories.Codes[:len(view.Categories.Codes)-1]
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an aggregate category list shorter than the source rows")
		}
	})
}

func TestRenderViewPrintsTheSelectedMonthAndSeparateEvidence(t *testing.T) {
	overview := dayMonthFixture(t)
	assertMonthSummaryFootsToSnapshot(t, "current month", overview)
	summary := overview.MonthSummary
	remainder := summary.Projection.Remainder
	recorded := *summary.RecordedFuture
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources:            []int{0},
			Months:             []string{"2026-11"},
			Layout:             ViewLayoutIndividual,
			CurrentMonthSource: intPointer(0),
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("valid current-month view refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text,
		"By period — November 2026",
		"Actual to date (month-to-date)",
		"Future-dated recorded",
		"Estimated (not actual) — Estimated remainder",
		fmt.Sprintf("PO amount %s (%s – %s)",
			formatMoney("IDR", remainder.BaseMinor),
			formatMoney("IDR", remainder.LowMinor),
			formatMoney("IDR", remainder.HighMinor)),
		formatMoney("IDR", recorded.AmountMinor),
		formatMoney("IDR", summary.Actual.AmountMinor),
		// The future-dated row is inside this snapshot's totals; the PDF must
		// print a total that the same supplied Periods foot to.
		formatMoney("IDR", overview.Totals.AmountMinor),
		"Actual figures only",
		"As of 2026-11-19",
		"Snapshot",
	)
	if strings.Contains(text, "Estimated (not actual) — Estimated remainder: POs 1") {
		t.Error("the estimated remainder printed a PO count it does not forecast")
	}
	// An estimate is never added to actual or recorded evidence.
	for _, mixed := range []int64{
		summary.Actual.AmountMinor + remainder.BaseMinor,
		recorded.AmountMinor + remainder.BaseMinor,
		summary.Actual.AmountMinor + recorded.AmountMinor + remainder.BaseMinor,
	} {
		requireNoText(t, "selected month", text, formatMoney("IDR", mixed))
	}
}

func TestRenderViewFutureMonthUsesOnlyTheCurrentYearForecast(t *testing.T) {
	overview := currentYearReport()
	otherYear := currentYearReport()
	otherYear.Filter = poreport.Query{
		From: "2027-01-01", To: "2028-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}
	periods := make([]poreport.Period, 0, 12)
	for month := 1; month <= 12; month++ {
		periods = append(periods, poreport.Period{
			PeriodStart: time.Date(2027, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly),
			Metrics: poreport.Metrics{
				PoCount: 1, AmountMinor: 12_345_000, CostedMinor: 12_000_000,
				UncostedMinor: 345_000, CapitalMinor: 10_000_000, MarginMinor: 2_000_000,
				MarginBps: fixtureRatio(2_000_000, 10_000_000), CoverageBps: fixtureRatio(12_000_000, 12_345_000),
			},
		})
	}
	otherYear.Periods = periods
	otherYear.Totals = poreport.Metrics{
		PoCount: 12, AmountMinor: 148_140_000, CostedMinor: 144_000_000,
		UncostedMinor: 4_140_000, CapitalMinor: 120_000_000, MarginMinor: 24_000_000,
		MarginBps: fixtureRatio(24_000_000, 120_000_000), CoverageBps: fixtureRatio(144_000_000, 148_140_000),
	}
	otherYear.CurrentProjection = unavailableCurrentProjection(
		poreport.ProjectionModeNone, poreport.ReasonNotCurrentPeriod, poreport.MethodCurrentNone,
		poreport.CurrentProjectionBasis{AsOf: "2026-11-19"})
	otherYear.Projection.Monthly.Buckets[1].AmountBaseMinor = 777_777_777
	otherYear.Projection.Monthly.Buckets[1].AmountHighMinor = 777_777_777
	otherYear.Projection.Monthly.Buckets[1].AmountLowMinor = 777_777_777

	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview, otherYear},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{1},
			Months:  []string{"2027-02"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("future-year view refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text,
		"By period — February 2027",
		"Recorded evidence in the selected future month",
		"Future months beyond the current year have no current-year forecast",
		formatMoney("IDR", 12_345_000),
	)
	if strings.Contains(text, "777.777.777") {
		t.Error("the selected-month section read the retired projection.monthly series as a fallback")
	}
}

// monthSectionText isolates one selected-month section so a value assertion
// cannot be satisfied by the KPI strip, the category table or the chart.
func monthSectionText(text, heading string) string {
	prefix := "By period — " + heading
	start := strings.Index(text, prefix)
	if start < 0 {
		return ""
	}
	rest := text[start+len(prefix):]
	end := len(rest)
	for _, marker := range []string{"Current-period projection", "Report notes", "Definitions"} {
		if index := strings.Index(rest, marker); index >= 0 && index < end {
			end = index
		}
	}
	return text[start : start+len(prefix)+end]
}

// TestRenderViewPrintsAPastMonthsSixRecordedMetrics is the rendered-PDF evidence
// for the past branch: all six metrics come from the supplied period row, and a
// past month never grows an estimated line.
func TestRenderViewPrintsAPastMonthsSixRecordedMetrics(t *testing.T) {
	overview := currentYearReport()
	row := overview.Periods[9].Metrics
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0},
			Months:  []string{"2026-10"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("past-month view refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	section := monthSectionText(strings.Join(binaryPDFText(t, pdf), "\n"), "October 2026")
	if section == "" {
		t.Fatal("the rendered PDF has no October 2026 selected-month section")
	}
	requireText(t, section,
		"Recorded figures for the selected month",
		fmt.Sprintf("POs %s", formatCount(row.PoCount)),
		fmt.Sprintf("PO amount %s", formatMoney("IDR", row.AmountMinor)),
		fmt.Sprintf("Costed amount %s", formatMoney("IDR", row.CostedMinor)),
		fmt.Sprintf("Capital %s", formatMoney("IDR", row.CapitalMinor)),
		fmt.Sprintf("PO margin %s", formatSignedMoney("IDR", row.MarginMinor)),
		fmt.Sprintf("PO margin %% %s", formatPct(row.MarginBps)),
	)
	requireNoText(t, "past month section", section, "Estimated", "Forecast unavailable")
}

// TestRenderViewPrintsACurrentYearFutureMonthForecast covers the in-year future
// branch with an available forecast: the supplied current-year bucket is the
// only value printed as an estimate, and nothing is invented for the missing
// recorded row.
func TestRenderViewPrintsACurrentYearFutureMonthForecast(t *testing.T) {
	overview := currentYearReport()
	forecast := overview.CurrentProjection.Buckets[0]
	overview.Projection.Monthly.Buckets[11].AmountLowMinor = 777_777_777
	overview.Projection.Monthly.Buckets[11].AmountBaseMinor = 777_777_777
	overview.Projection.Monthly.Buckets[11].AmountHighMinor = 777_777_777
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0},
			Months:  []string{"2026-12"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("future-month view refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	section := monthSectionText(strings.Join(binaryPDFText(t, pdf), "\n"), "December 2026")
	if section == "" {
		t.Fatal("the rendered PDF has no December 2026 selected-month section")
	}
	requireText(t, section,
		"Estimated (not actual) — Estimated forecast",
		fmt.Sprintf("PO amount %s (%s – %s)",
			formatMoney("IDR", forecast.AmountBaseMinor),
			formatMoney("IDR", forecast.AmountLowMinor),
			formatMoney("IDR", forecast.AmountHighMinor)),
	)
	// No recorded row was supplied, so the six actual metrics are dashes, not
	// an invented zero, and the retired series is never a fallback.
	requireNoText(t, "future month section", section,
		fmt.Sprintf("POs %s", formatCount(0)),
		formatMoney("IDR", 0),
		"777.777.777",
	)
}

// TestRenderViewPrintsAnUnavailableFutureForecastReason covers the in-year
// future branch when the annual source's guard fails: the stable reason prints
// and no estimate is invented.
func TestRenderViewPrintsAnUnavailableFutureForecastReason(t *testing.T) {
	overview := currentYearReport()
	overview.CurrentProjection = unavailableCurrentProjection(
		poreport.ProjectionModeMonth, poreport.ReasonInsufficientDays, poreport.MethodCurrentMonth,
		poreport.CurrentProjectionBasis{
			From: "2025-12-01", To: "2026-11-01", Units: 3, PoCount: 2, AsOf: "2026-11-19",
		})
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0},
			Months:  []string{"2026-12"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("unavailable future-month view refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	section := monthSectionText(strings.Join(binaryPDFText(t, pdf), "\n"), "December 2026")
	if section == "" {
		t.Fatal("the rendered PDF has no December 2026 selected-month section")
	}
	requireText(t, section,
		"Forecast unavailable",
		"Not enough elapsed days: the current month needs five elapsed days and three purchase orders.",
	)
	requireNoText(t, "unavailable future month section", section, "Estimated forecast")
}

// TestRenderViewReplacesTheLegacyPartialPeriodDefinition pins the view-only
// copy: a selected-month PDF has no partial/in-progress period table, so it must
// not claim one, while the legacy report keeps that sentence unchanged.
func TestRenderViewReplacesTheLegacyPartialPeriodDefinition(t *testing.T) {
	const (
		legacyDefinition = "Partial and in-progress periods are marked in the period table."
		viewDefinition   = "Selected-month rows are not marked partial or in progress."
	)

	viewPDF, err := RenderView(validPastMonthView(t), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	viewText := strings.Join(binaryPDFText(t, viewPDF), "\n")
	requireText(t, viewText, viewDefinition)
	requireNoText(t, "view definitions", viewText, legacyDefinition)

	legacyPDF, err := Render(sampleReport(), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render(legacy): %v", err)
	}
	legacyText := strings.Join(binaryPDFText(t, legacyPDF), "\n")
	requireText(t, legacyText, legacyDefinition)
	requireNoText(t, "legacy definitions", legacyText, viewDefinition)
}

func TestRenderViewCategorySectionUsesItsOwnSource(t *testing.T) {
	overview := currentYearReport()
	overview.Filter.Category = poreport.CategoryFm
	categories := currentYearReport()
	categories.Filter.Category = poreport.CategoryAll
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview, categories},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0},
			Months:  []string{"2026-10"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 1, Codes: viewCodes(categories), Layout: ViewLayoutAggregate},
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	section := categorySectionText(text)
	if section == "" {
		t.Fatal("the rendered PDF has no category section")
	}
	requireText(t, section,
		"All categories (5)",
		"Facility",
		"Fabrication",
		"Total",
		formatMoney("IDR", categories.Totals.AmountMinor),
	)
	// The section's caption names its own all-category source, never
	// Performance's restricted FM filter.
	requireNoText(t, "category section", section, "Facility Management (FM)")
}

func TestValidateSnapshotRefusesAnIncoherentMonthSummary(t *testing.T) {
	valid := dayReport()
	valid.MonthSummary = monthSummaryFromSnapshot(t, valid)
	remainder := *valid.MonthSummary.Projection.Remainder
	if err := ValidateSnapshot(valid); err != nil {
		t.Fatalf("valid monthSummary refused: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(summary *poreport.MonthSummary)
	}{
		{name: "wrong month", mutate: func(summary *poreport.MonthSummary) {
			summary.Month = "2026-10"
			summary.From = "2026-10-01"
			summary.To = "2026-11-01"
		}},
		{name: "wrong as-of", mutate: func(summary *poreport.MonthSummary) {
			summary.AsOf = "2026-11-18"
		}},
		{name: "available without remainder", mutate: func(summary *poreport.MonthSummary) {
			summary.Projection.Remainder = nil
		}},
		{name: "unavailable with remainder", mutate: func(summary *poreport.MonthSummary) {
			summary.Projection = poreport.MonthSummaryProjection{
				Status:    poreport.ProjectionStatusUnavailable,
				Reason:    poreport.ReasonInsufficientDays,
				Method:    poreport.MethodCurrentDay,
				Remainder: &remainder,
			}
		}},
		{name: "unknown reason", mutate: func(summary *poreport.MonthSummary) {
			summary.Projection = poreport.MonthSummaryProjection{
				Status: poreport.ProjectionStatusUnavailable,
				Reason: "made_up",
				Method: poreport.MethodCurrentDay,
			}
		}},
		{name: "inexact recorded future", mutate: func(summary *poreport.MonthSummary) {
			summary.RecordedFuture = &poreport.Metrics{
				PoCount: 1, AmountMinor: poreport.MaxExactMinor + 1,
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			snapshot := dayReport()
			summary := *valid.MonthSummary
			projection := summary.Projection
			summary.Projection = projection
			test.mutate(&summary)
			snapshot.MonthSummary = &summary
			if err := ValidateSnapshot(snapshot); err == nil {
				t.Fatal("ValidateSnapshot accepted an incoherent monthSummary")
			}
		})
	}
}

func TestValidateSnapshotRefusesADaySummaryOnAnotherFilter(t *testing.T) {
	year := currentYearReport()
	year.MonthSummary = &poreport.MonthSummary{
		Month: "2026-11", From: "2026-11-01", To: "2026-12-01", AsOf: "2026-11-19",
		Mode:   poreport.ProjectionModeDay,
		Actual: year.Periods[10].Metrics,
		Projection: poreport.MonthSummaryProjection{
			Status: poreport.ProjectionStatusUnavailable,
			Reason: poreport.ReasonInsufficientDays,
			Method: poreport.MethodCurrentDay,
		},
	}
	if err := ValidateSnapshot(year); err == nil {
		t.Fatal("a day monthSummary was accepted on a year/month filter")
	}
}

func TestLegacyReportWithMonthSummaryStillPrintsTheLegacyComposition(t *testing.T) {
	legacy := dayReport()
	legacy.MonthSummary = monthSummaryFromSnapshot(t, legacy)
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy with monthSummary: %v", err)
	}
	document, err := DecodeExport(raw)
	if err != nil {
		t.Fatalf("DecodeExport(legacy with monthSummary): %v", err)
	}
	if document.Legacy == nil || document.View != nil {
		t.Fatalf("document = %+v; a report with monthSummary must stay on the legacy path", document)
	}
	pdf, err := RenderExport(document, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderExport(legacy): %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text, "By period", "PERIOD")
	if strings.Contains(text, "By period —") {
		t.Error("legacy composition printed the versioned selected-month heading")
	}
}

func TestValidateViewAllowsDifferentGeneratedTimestamps(t *testing.T) {
	overview := currentYearReport()
	other := currentYearReport()
	other.GeneratedAt = "2026-11-19T09:45:00Z"
	other.Filter = poreport.Query{
		From: "2025-01-01", To: "2026-01-01", Bucket: poreport.BucketMonth,
		Category: poreport.CategoryAll, Currency: "IDR",
	}
	other.Periods = other.Periods[:10]
	for index := range other.Periods {
		other.Periods[index].PeriodStart = time.Date(
			2025, time.Month(index+1), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
	}
	other.MonthSummary = nil
	other.CategorySeries = nil
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview, other},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{1},
			Months:  []string{"2025-10"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("different generated timestamps should be allowed: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text, "19 Nov 2026 08:30:00 UTC", "19 Nov 2026 09:45:00 UTC")
}

func TestRenderViewCurrentMonthFromAnAnnualMonthSummary(t *testing.T) {
	overview := yearMonthFixture(t)
	assertMonthSummaryFootsToSnapshot(t, "annual current month", overview)
	summary := overview.MonthSummary
	remainder := summary.Projection.Remainder
	actual := summary.Actual
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0},
			Months:  []string{"2026-11"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("current-month annual view refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text,
		"By period — November 2026",
		"Actual to date (month-to-date)",
		formatCount(actual.PoCount),
		formatMoney("IDR", actual.AmountMinor),
		formatMoney("IDR", actual.CostedMinor),
		formatMoney("IDR", actual.CapitalMinor),
		formatSignedMoney("IDR", actual.MarginMinor),
		formatPct(actual.MarginBps),
		"Estimated (not actual) — Estimated remainder",
		fmt.Sprintf("PO amount %s (%s – %s)",
			formatMoney("IDR", remainder.BaseMinor),
			formatMoney("IDR", remainder.LowMinor),
			formatMoney("IDR", remainder.HighMinor)),
	)
	requireNoText(t, "annual current month", text,
		formatMoney("IDR", actual.AmountMinor+remainder.BaseMinor))
}

func TestRenderViewPrintsAZeroMonthUnderAnEmptyOverview(t *testing.T) {
	overview := emptyMonthReport(t)
	assertMonthSummaryFootsToSnapshot(t, "empty month", overview)
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources:            []int{0},
			Months:             []string{"2026-11"},
			Layout:             ViewLayoutIndividual,
			CurrentMonthSource: intPointer(0),
		},
		Categories: ViewCategories{Source: 0, Codes: []string{}, Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("zero month view refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text,
		"By period — November 2026",
		"Actual to date (month-to-date)",
		formatMoney("IDR", 0),
		"Forecast unavailable",
		"No purchase orders in the current period yet",
		"Unavailable",
		// The projection still knows how many calendar days elapsed; it knows
		// zero purchase orders.
		"19 elapsed days",
		"0 purchase orders",
	)
	// An empty month has no global 19-PO forecast and no missing-source
	// evidence beside its zero actuals: every surface describes the same empty
	// snapshot.
	requireNoText(t, "empty month overview", text,
		"19 purchase orders",
		"purchase order(s) in this range have a deleted source quotation",
		formatMoney("IDR", 152_000_000),
	)
}

// Issue #642: the v1 envelope now covers an empty, subset and cross-year
// selection plus one-calendar-year Aggregate, without a version bump. These
// helpers build immutable annual snapshots for the tests; no source is
// re-fetched and no figure is recalculated.
func annualViewReport(year int, asOf string, amounts []int64) poreport.Report {
	snapshot := currentYearReport()
	snapshot.AsOf = asOf
	snapshot.Filter = poreport.Query{
		From: fmt.Sprintf("%04d-01-01", year), To: fmt.Sprintf("%04d-01-01", year+1),
		Bucket: poreport.BucketMonth, Category: poreport.CategoryAll, Currency: "IDR",
	}
	periods := make([]poreport.Period, 0, 12)
	for month := 1; month <= 12; month++ {
		amount := int64(100_000_000)
		if month <= len(amounts) && amounts[month-1] > 0 {
			amount = amounts[month-1]
		}
		costed := amount - 1_000
		capital := amount / 2
		margin := costed - capital
		periods = append(periods, poreport.Period{
			PeriodStart: monthStart(year, time.Month(month)),
			Metrics: poreport.Metrics{
				PoCount: 2, AmountMinor: amount, CostedMinor: costed,
				UncostedMinor: 1_000, CapitalMinor: capital, MarginMinor: margin,
				MarginBps:   fixtureRatio(margin, capital),
				CoverageBps: fixtureRatio(costed, amount),
			},
		})
	}
	snapshot.Periods = periods
	snapshot.Categories = nil
	snapshot.CategorySeries = nil
	snapshot.MonthSummary = nil
	snapshot.Totals = periods[0].Metrics
	snapshot.Projection = poreport.Projection{
		Monthly: poreport.ProjectionSeries{
			Status: poreport.ProjectionStatusUnavailable, Reason: poreport.ReasonInsufficientHistory,
			Method: poreport.MethodMonthly, Basis: poreport.ProjectionBasis{AsOf: asOf},
		},
		Yearly: poreport.ProjectionSeries{
			Status: poreport.ProjectionStatusUnavailable, Reason: poreport.ReasonInsufficientHistory,
			Method: poreport.MethodYearly, Basis: poreport.ProjectionBasis{AsOf: asOf},
		},
	}
	snapshot.CurrentProjection = unavailableCurrentProjection(
		poreport.ProjectionModeNone, poreport.ReasonNotCurrentPeriod, poreport.MethodCurrentNone,
		poreport.CurrentProjectionBasis{AsOf: asOf})
	return snapshot
}

func monthsOfYear(year int) []string {
	months := make([]string, 0, 12)
	for month := 1; month <= 12; month++ {
		months = append(months, fmt.Sprintf("%04d-%02d", year, month))
	}
	return months
}

func annualAggregateView(source poreport.Report) ViewEnvelope {
	year := 0
	fmt.Sscanf(source.Filter.From, "%04d", &year)
	return ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{source},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0},
			Months:  monthsOfYear(year),
			Layout:  ViewLayoutAggregate,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(source), Layout: ViewLayoutAggregate},
	}
}

func viewPeriodSectionText(text string) string {
	start := strings.Index(text, "By period")
	if start < 0 {
		return ""
	}
	end := len(text)
	for _, marker := range []string{"Current-period projection", "Report notes", "Definitions"} {
		if index := strings.Index(text[start:], marker); index >= 0 && start+index < end {
			end = start + index
		}
	}
	return text[start:end]
}

func TestDecodeViewAllowsAnEmptyMonthSelection(t *testing.T) {
	overview := currentYearReport()
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{}, Months: []string{}, Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("empty selection refused: %v", err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal empty view: %v", err)
	}
	document, err := DecodeExport(raw)
	if err != nil {
		t.Fatalf("DecodeExport(empty view): %v", err)
	}
	if document.View == nil || document.Legacy != nil {
		t.Fatalf("empty view was not decoded through the v1 path: %+v", document)
	}
	pdf, err := RenderExport(document, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderExport(empty view): %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text, "By period", "No months selected", "0 months selected", "As of 2026-11-19")
	section := viewPeriodSectionText(text)
	if section == "" {
		t.Fatal("the rendered PDF has no By period section")
	}
	requireNoText(t, "empty selection section", section,
		"Recorded total", "Estimated", "POs ", formatMoney("IDR", 0))

	// The layout preference is independent of an empty selection: the same
	// explicit empty section prints whether the last preference was Individual
	// or Aggregate, and neither invents a year or a total.
	aggregateEmpty := view
	aggregateEmpty.Periods.Layout = ViewLayoutAggregate
	aggregatePDF, err := RenderView(aggregateEmpty, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(empty aggregate): %v", err)
	}
	aggregateSection := viewPeriodSectionText(strings.Join(binaryPDFText(t, aggregatePDF), "\n"))
	requireText(t, aggregateSection, "No months selected", "0 months selected")
	requireNoText(t, "empty aggregate section", aggregateSection, "Recorded total", "all months in")

	// An empty selection cannot claim that one month is the current-month
	// override, and it cannot carry a foreign source reference.
	bad := view
	bad.Periods.CurrentMonthSource = intPointer(0)
	if err := ValidateView(bad); err == nil {
		t.Fatal("accepted a current-month override in an empty selection")
	}
	bad = view
	bad.Periods.Sources = []int{2}
	if err := ValidateView(bad); err == nil {
		t.Fatal("accepted a foreign period source in an empty selection")
	}
}

func TestValidateViewAllowsOneCalendarYearAggregate(t *testing.T) {
	source := annualViewReport(2025, "2025-12-20", nil)
	view := annualAggregateView(source)
	if err := ValidateView(view); err != nil {
		t.Fatalf("valid aggregate refused: %v", err)
	}

	t.Run("eleven months", func(t *testing.T) {
		short := view
		short.Periods.Months = monthsOfYear(2025)[:11]
		if err := ValidateView(short); err == nil {
			t.Fatal("accepted an aggregate of eleven months")
		}
	})
	t.Run("cross-year twelve", func(t *testing.T) {
		other := annualViewReport(2026, "2025-12-20", nil)
		crossYear := ViewEnvelope{
			Version:  ViewVersion,
			Sources:  []poreport.Report{source, other},
			Overview: 0,
			Periods: ViewPeriods{
				Sources: []int{0, 1},
				Months:  append([]string{"2025-12"}, monthsOfYear(2026)[:11]...),
				Layout:  ViewLayoutAggregate,
			},
			Categories: ViewCategories{Source: 0, Codes: viewCodes(source), Layout: ViewLayoutAggregate},
		}
		if err := ValidateView(crossYear); err == nil {
			t.Fatal("accepted a cross-year twelve as an aggregate")
		}
	})
	t.Run("no annual source for the selected year", func(t *testing.T) {
		wrongYear := view
		wrongYear.Periods.Months = monthsOfYear(2026)
		if err := ValidateView(wrongYear); err == nil {
			t.Fatal("accepted aggregate months with no referenced annual source for that year")
		}
	})
	t.Run("empty aggregate is the explicit empty section", func(t *testing.T) {
		empty := view
		empty.Periods.Sources = []int{}
		empty.Periods.Months = []string{}
		if err := ValidateView(empty); err != nil {
			t.Fatalf("empty aggregate selection refused: %v", err)
		}
	})
	t.Run("current-month override is allowed but not the aggregate total source", func(t *testing.T) {
		year := currentYearReport()
		day := dayMonthFixture(t)
		withOverride := ViewEnvelope{
			Version:  ViewVersion,
			Sources:  []poreport.Report{year, day},
			Overview: 0,
			Periods: ViewPeriods{
				Sources: []int{0, 1}, Months: monthsOfYear(2026),
				Layout: ViewLayoutAggregate, CurrentMonthSource: intPointer(1),
			},
			Categories: ViewCategories{Source: 0, Codes: viewCodes(year), Layout: ViewLayoutAggregate},
		}
		if err := ValidateView(withOverride); err != nil {
			t.Fatalf("aggregate with a current-month override refused: %v", err)
		}
	})
}

func TestValidateViewExtendsToCrossYearSubsets(t *testing.T) {
	overview := currentYearReport()
	previous := annualViewReport(2025, "2026-11-19", nil)
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview, previous},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0, 1},
			Months:  []string{"2025-12", "2026-01"},
			Layout:  ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("cross-year subset refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView(cross-year): %v", err)
	}
	section := viewPeriodSectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section, "By period — 2 selected months", "December 2025", "January 2026")
	requireInOrder(t, section, "December 2025", "January 2026")
}

func TestValidateViewKeepsTheSourceAndSpanBounds(t *testing.T) {
	overview := currentYearReport()
	years := []int{2016, 2017, 2018, 2019, 2020, 2021, 2022, 2023, 2024, 2025}
	months := make([]string, 0, 120)
	sources := []poreport.Report{overview}
	refs := make([]int, 0, len(years))
	for index, year := range years {
		sources = append(sources, annualViewReport(year, "2026-11-19", nil))
		refs = append(refs, index+1)
		months = append(months, monthsOfYear(year)...)
	}
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  sources,
		Overview: 0,
		Periods: ViewPeriods{
			Sources: refs, Months: months, Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("120-month inclusive span refused: %v", err)
	}

	// Seventeen immutable sources are still refused by the existing cap.
	tooMany := view
	tooMany.Sources = append([]poreport.Report{}, sources...)
	for year := 2000; year <= 2006; year++ {
		tooMany.Sources = append(tooMany.Sources, annualViewReport(year, "2026-11-19", nil))
	}
	if err := ValidateView(tooMany); err == nil {
		t.Fatal("accepted more than sixteen sources")
	}

	// One hundred twenty selected months spanning 121 inclusive months is
	// refused: both the count cap and the inclusive span cap remain in force.
	spanSources := []poreport.Report{overview}
	spanRefs := make([]int, 0, 11)
	spanMonths := make([]string, 0, 120)
	for year := 2016; year <= 2025; year++ {
		spanSources = append(spanSources, annualViewReport(year, "2026-11-19", nil))
		spanRefs = append(spanRefs, len(spanSources)-1)
		spanMonths = append(spanMonths, monthsOfYear(year)...)
	}
	spanRefs = append(spanRefs, 0)
	spanMonths = append(spanMonths, "2026-01")
	// Drop one middle month so the count stays at 120 while the inclusive span
	// becomes 121 months.
	spanMonths = append(spanMonths[:54], spanMonths[55:]...)
	spanView := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  spanSources,
		Overview: 0,
		Periods: ViewPeriods{
			Sources: spanRefs, Months: spanMonths, Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(spanView); err == nil {
		t.Fatal("accepted a 121-month inclusive span")
	}
}

func TestResolveViewUsesTheCurrentMonthOverrideOnce(t *testing.T) {
	annual := currentYearReport()
	day := dayMonthFixture(t)
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{annual, day},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0, 1}, Months: []string{"2026-10", "2026-11"},
			Layout: ViewLayoutIndividual, CurrentMonthSource: intPointer(1),
		},
		Categories: ViewCategories{Source: 0, Codes: viewCodes(annual), Layout: ViewLayoutAggregate},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("dual current-month sources refused: %v", err)
	}
	renderer := viewRenderer(t, view)
	var november *cardBox
	for index := range renderer.cards {
		if renderer.cards[index].Month == "2026-11" {
			november = &renderer.cards[index]
		}
	}
	if november == nil {
		t.Fatal("the laid-out document has no November 2026 card")
	}
	actual := day.MonthSummary.Actual
	novemberText := pdfText(strings.Join(cardTexts(renderer, *november), " "))
	requireText(t, novemberText,
		"Actual to date (month-to-date)",
		formatMoney("IDR", actual.AmountMinor),
		"Future-dated recorded",
		formatMoney("IDR", day.MonthSummary.RecordedFuture.AmountMinor),
	)
	// The annual November row is the recorded month, including any
	// future-dated row; the override must not be printed as the actual card.
	annualRow := annual.Periods[10].Metrics
	if annualRow.AmountMinor == actual.AmountMinor {
		t.Fatal("fixture error: the annual and actual amounts are indistinguishable")
	}
	requireNoText(t, "current-month card", novemberText, formatMoney("IDR", annualRow.AmountMinor))

	mismatch := view
	mismatch.Sources = []poreport.Report{annual, annualViewReport(2025, "2026-11-19", nil)}
	if err := ValidateView(mismatch); err == nil {
		t.Fatal("accepted a current-month override with no matching summary")
	}
}

// Issue #643: the category section is one v1 envelope selection, not a version
// bump and not a second report. These tests pin the two category displays, the
// category source's inherited scope, the canonical supplied order and the
// rendered empty / individual / aggregate sections.

// categoryComparisonSource is the all-category snapshot the category section
// reads. It keeps the five supplied rows in their canonical order, makes OTHERS
// a supplied zero row and FAB a row discounted below cost, so exact money, a
// zero row and a negative margin are all printable from supplied evidence.
func categoryComparisonSource(t *testing.T) poreport.Report {
	t.Helper()
	source := currentYearReport()
	source.Categories[3].Metrics = poreport.Metrics{}
	fab := &source.Categories[1]
	fab.Metrics.CapitalMinor = fab.Metrics.CostedMinor + 5_000_000
	fab.Metrics.MarginMinor = -5_000_000
	fab.Metrics.MarginBps = fixtureRatio(-5_000_000, fab.Metrics.CapitalMinor)
	return source
}

// restrictedCategoryOverview is a Performance snapshot filtered to FM. Its
// category section still reads its own category=all source.
func restrictedCategoryOverview() poreport.Report {
	overview := currentYearReport()
	overview.Filter.Category = poreport.CategoryFm
	facility := overview.Categories[0]
	overview.Categories = []poreport.Category{facility}
	overview.Totals = facility.Metrics
	return overview
}

func categoryComparisonView(t *testing.T, layout string, codes []string, restrictedOverview bool) ViewEnvelope {
	t.Helper()
	source := categoryComparisonSource(t)
	overview := currentYearReport()
	sources := []poreport.Report{source}
	sourceIndex := 0
	if restrictedOverview {
		overview = restrictedCategoryOverview()
		sources = []poreport.Report{overview, source}
		sourceIndex = 1
	}
	return ViewEnvelope{
		Version:  ViewVersion,
		Sources:  sources,
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0}, Months: []string{"2026-10"}, Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: sourceIndex, Codes: codes, Layout: layout},
	}
}

// categorySectionText isolates the printed category section so a figure
// assertion cannot be satisfied by the KPI strip, the chart or the month cards.
func categorySectionText(text string) string {
	start := strings.Index(text, "By business category")
	if start < 0 {
		return ""
	}
	rest := text[start:]
	end := len(rest)
	for _, marker := range []string{"By period", "Current-period projection", "Report notes", "Definitions"} {
		if index := strings.Index(rest, marker); index >= 0 && index < end {
			end = index
		}
	}
	return rest[:end]
}

func TestValidateViewAcceptsCategoryComparisons(t *testing.T) {
	allCodes := viewCodes(categoryComparisonSource(t))
	cases := []struct {
		name     string
		layout   string
		codes    []string
		restrict bool
	}{
		{name: "empty aggregate", layout: ViewLayoutAggregate, codes: []string{}},
		{name: "empty individual under a restricted overview", layout: ViewLayoutIndividual, codes: []string{}, restrict: true},
		{name: "one individual", layout: ViewLayoutIndividual, codes: []string{poreport.CategoryFm}, restrict: true},
		{name: "three individual", layout: ViewLayoutIndividual,
			codes: []string{poreport.CategoryFm, poreport.CategoryFab, poreport.CategoryUnassigned}, restrict: true},
		{name: "four individual", layout: ViewLayoutIndividual,
			codes: []string{poreport.CategoryFm, poreport.CategoryFab, poreport.CategorySupplyGeneral, poreport.CategoryOthers}, restrict: true},
		{name: "five individual", layout: ViewLayoutIndividual, codes: allCodes, restrict: true},
		{name: "five aggregate under a restricted overview", layout: ViewLayoutAggregate, codes: allCodes, restrict: true},
		{name: "five aggregate on the overview source", layout: ViewLayoutAggregate, codes: allCodes},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			view := categoryComparisonView(t, test.layout, test.codes, test.restrict)
			if err := ValidateView(view); err != nil {
				t.Fatalf("valid category comparison refused: %v", err)
			}
			raw, err := json.Marshal(view)
			if err != nil {
				t.Fatalf("marshal view: %v", err)
			}
			document, err := DecodeExport(raw)
			if err != nil {
				t.Fatalf("DecodeExport: %v", err)
			}
			if document.View == nil || document.Legacy != nil {
				t.Fatalf("category comparison did not decode through the v1 path: %+v", document)
			}
		})
	}
}

func TestValidateViewRefusesCategoryScopeAndOrder(t *testing.T) {
	allCodes := viewCodes(categoryComparisonSource(t))
	base := categoryComparisonView(t, ViewLayoutIndividual,
		[]string{poreport.CategoryFm, poreport.CategoryFab, poreport.CategoryUnassigned}, true)

	t.Run("noncanonical order", func(t *testing.T) {
		view := base
		view.Categories.Codes = []string{poreport.CategoryFab, poreport.CategoryFm}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted category codes out of the supplied source order")
		}
	})
	t.Run("duplicate code", func(t *testing.T) {
		view := base
		view.Categories.Codes = []string{poreport.CategoryFm, poreport.CategoryFm}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a duplicate category code")
		}
	})
	t.Run("unknown code", func(t *testing.T) {
		view := base
		view.Categories.Codes = []string{poreport.CategoryFm, "NOPE"}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a non-canonical category code")
		}
	})
	t.Run("code absent from the source", func(t *testing.T) {
		view := base
		view.Sources = append([]poreport.Report{}, base.Sources...)
		source := view.Sources[1]
		kept := make([]poreport.Category, 0, len(source.Categories))
		for _, category := range source.Categories {
			if category.BusinessCategory == poreport.CategoryOthers {
				continue
			}
			kept = append(kept, category)
		}
		source.Categories = kept
		view.Sources[1] = source
		view.Categories.Codes = []string{poreport.CategoryOthers}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a category code the source does not supply")
		}
	})
	t.Run("subset aggregate", func(t *testing.T) {
		view := base
		view.Categories.Layout = ViewLayoutAggregate
		view.Categories.Codes = []string{poreport.CategoryFm, poreport.CategoryFab}
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an aggregate of a category subset")
		}
	})
	t.Run("category source range differs", func(t *testing.T) {
		view := base
		view.Sources = append([]poreport.Report{}, base.Sources...)
		source := view.Sources[1]
		source.Filter.From = "2024-01-01"
		view.Sources[1] = source
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a category source with a range other than the overview's")
		}
	})
	t.Run("category source bucket differs", func(t *testing.T) {
		view := base
		view.Sources = append([]poreport.Report{}, base.Sources...)
		source := view.Sources[1]
		source.Filter.Bucket = poreport.BucketYear
		source.Periods = nil
		source.CategorySeries = nil
		view.Sources[1] = source
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a category source with a bucket other than the overview's")
		}
	})
	t.Run("foreign restricted category", func(t *testing.T) {
		// The overview is FM; a FAB category source would make the section's
		// rows and its caption disagree with the report it was exported from.
		view := base
		view.Sources = append([]poreport.Report{}, base.Sources...)
		source := view.Sources[1]
		source.Filter.Category = poreport.CategoryFab
		view.Sources[1] = source
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted a category source restricted to a foreign category")
		}
	})
	t.Run("all codes in aggregate order", func(t *testing.T) {
		view := categoryComparisonView(t, ViewLayoutAggregate, allCodes, true)
		if err := ValidateView(view); err != nil {
			t.Fatalf("full aggregate refused: %v", err)
		}
	})
}

func TestRenderViewEmptyCategorySectionPrintsNoFiguresOrTotal(t *testing.T) {
	for _, layout := range []string{ViewLayoutIndividual, ViewLayoutAggregate} {
		t.Run(layout, func(t *testing.T) {
			view := categoryComparisonView(t, layout, []string{}, true)
			if err := ValidateView(view); err != nil {
				t.Fatalf("empty category selection refused: %v", err)
			}
			pdf, err := RenderView(view, os.DirFS("../../assets"))
			if err != nil {
				t.Fatalf("RenderView: %v", err)
			}
			section := categorySectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
			if section == "" {
				t.Fatal("the rendered PDF has no category section")
			}
			requireText(t, section,
				"By business category",
				"0 categories selected",
				"No categories selected",
				"Choose at least one business category to print a category comparison.",
				"2026-01-01 → 2027-01-01 (exclusive)",
				"IDR",
				"As of 2026-11-19",
				"Snapshot 19 Nov 2026 08:30:00 UTC",
			)
			requireNoText(t, "empty category section", section,
				"PO amount", "Costed amount", "Capital", "PO margin",
				"Total", "Estimated", "Forecast", "Projected", formatMoney("IDR", 0),
			)
		})
	}
}

func TestRenderViewPrintsOneIndividualCategoryCard(t *testing.T) {
	view := categoryComparisonView(t, ViewLayoutIndividual, []string{poreport.CategoryFm}, true)
	row := view.Sources[1].Categories[0]
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	section := categorySectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section,
		"By business category — Facility Management",
		"POs", formatCount(row.PoCount),
		"PO amount", formatMoney("IDR", row.AmountMinor),
		"Costed amount", formatMoney("IDR", row.CostedMinor),
		"Capital", formatMoney("IDR", row.CapitalMinor),
		"PO margin", formatSignedMoney("IDR", row.MarginMinor),
		"PO margin %", formatPct(row.MarginBps),
		"Uncosted "+formatMoney("IDR", row.UncostedMinor)+" is not counted as profit",
		"2026-01-01 → 2027-01-01 (exclusive)",
		"As of 2026-11-19",
		"Snapshot 19 Nov 2026 08:30:00 UTC",
	)
	// The card names the selected category, never Performance's restricted
	// filter, and it prints no total and no ghost estimate.
	requireNoText(t, "one-category card", section,
		"Facility Management (FM)", "Total", "Estimated", "Forecast", "Projected",
	)
}

func TestRenderViewPrintsAnIndividualCategorySubsetWithoutAnInventedTotal(t *testing.T) {
	codes := []string{poreport.CategoryFm, poreport.CategoryFab, poreport.CategoryUnassigned}
	view := categoryComparisonView(t, ViewLayoutIndividual, codes, true)
	source := view.Sources[1]
	var subsetAmount int64
	for _, code := range codes {
		for _, category := range source.Categories {
			if category.BusinessCategory == code {
				subsetAmount += category.AmountMinor
			}
		}
	}
	if subsetAmount == source.Totals.AmountMinor {
		t.Fatal("fixture error: the subset sum and the server total are indistinguishable")
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	section := categorySectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section,
		"By business category — 3 selected categories",
		"Facility Management", "Fabrication", "Unassigned",
		formatSignedMoney("IDR", -5_000_000),
		"PO margin %",
	)
	requireInOrder(t, section, "Facility Management", "Fabrication", "Unassigned")
	// A subset has no total of its own, invented or otherwise, and the section
	// never reprints the whole source's server total.
	requireNoText(t, "category subset", section,
		"Total", "Estimated", "Forecast", "Projected",
		formatMoney("IDR", subsetAmount), formatMoney("IDR", source.Totals.AmountMinor),
	)
}

func TestRenderViewAggregateCategoriesUseTheServerTotalNotARowSum(t *testing.T) {
	allCodes := viewCodes(categoryComparisonSource(t))
	view := categoryComparisonView(t, ViewLayoutAggregate, allCodes, true)
	source := view.Sources[1]
	source.Totals = poreport.Metrics{
		PoCount: 37, AmountMinor: 1_200_777_777, CostedMinor: 1_000_000_000,
		UncostedMinor: 200_777_777, CapitalMinor: 800_000_000, MarginMinor: 200_000_000,
		MarginBps:   fixtureRatio(200_000_000, 800_000_000),
		CoverageBps: fixtureRatio(1_000_000_000, 1_200_777_777),
	}
	view.Sources[1] = source
	var rowSum int64
	for _, category := range source.Categories {
		rowSum += category.AmountMinor
	}
	if rowSum == source.Totals.AmountMinor {
		t.Fatal("fixture error: the row sum and the server total are indistinguishable")
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	section := categorySectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section,
		"By business category",
		"All categories (5)",
		"2026-01-01 → 2027-01-01 (exclusive)",
		"IDR",
		"As of 2026-11-19",
		"Snapshot 19 Nov 2026 08:30:00 UTC",
		"Total",
		formatMoney("IDR", source.Totals.AmountMinor),
		"not a sum of the printed rows",
		"Facility", "Fabrication", "Goods Supply &", "Others", "Unassigned",
	)
	// The category column is 86pt wide, so the long labels wrap across lines and
	// a row's words are separated by the row's figures in extracted text. The
	// row order is still the source's supplied order.
	requireInOrder(t, section, "Facility", "Fabrication", "Goods Supply &", "Others", "Unassigned")
	// The caption names the all-category source, never Performance's FM filter;
	// and the printed total is the server's, not the row sum.
	requireNoText(t, "category aggregate", section,
		"Facility Management (FM)", formatMoney("IDR", rowSum),
	)
}

func TestRenderViewRestrictedCategorySourceNamesItsOwnFilter(t *testing.T) {
	// #641 compatibility: a category source may be restricted to the overview's
	// own category. It is printed, and the section names that filter honestly
	// instead of claiming to be an all-category comparison.
	overview := restrictedCategoryOverview()
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{0}, Months: []string{"2026-10"}, Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{
			Source: 0, Codes: []string{poreport.CategoryFm}, Layout: ViewLayoutAggregate,
		},
	}
	if err := ValidateView(view); err != nil {
		t.Fatalf("restricted category source refused: %v", err)
	}
	pdf, err := RenderView(view, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("RenderView: %v", err)
	}
	section := categorySectionText(strings.Join(binaryPDFText(t, pdf), "\n"))
	requireText(t, section, "Facility Management (FM)", "Total")
	requireNoText(t, "restricted category source", section, "All categories (5)")
}
