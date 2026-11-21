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
		view.Periods.Layout = ViewLayoutAggregate
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an unsupported aggregate period layout")
		}
		view = base
		view.Categories.Layout = ViewLayoutIndividual
		if err := ValidateView(view); err == nil {
			t.Fatal("accepted an unsupported individual category layout")
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
	requireText(t, text, "Category section: All categories (all)")
	if !strings.Contains(text, "Facility Management (FM)") ||
		!strings.Contains(text, "Fabrication (FAB)") {
		t.Error("the category section did not print its own category-scoped source rows")
	}
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
