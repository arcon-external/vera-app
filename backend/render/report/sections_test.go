package report

import (
	"strings"
	"testing"

	"vera-app/backend/poreport"
)

func renderPageText(t *testing.T, snapshot poreport.Report) string {
	t.Helper()
	renderer, err := testReportRenderer(snapshot)
	if err != nil {
		t.Fatalf("layout report: %v", err)
	}
	return pageText(renderer.canvas.pages)
}

// pdfText collapses whitespace runs. pdftotext -layout collapses the double
// spaces the renderer draws around a separator, while PyMuPDF preserves them;
// the document's words are what the test owns, not an extractor's spacing.
func pdfText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func requireText(t *testing.T, text string, wants ...string) {
	t.Helper()
	haystack := pdfText(text)
	for _, want := range wants {
		if !strings.Contains(haystack, pdfText(want)) {
			t.Errorf("rendered report does not print %q", want)
		}
	}
}

func requireNoText(t *testing.T, name, text string, unwanted ...string) {
	t.Helper()
	haystack := pdfText(text)
	for _, value := range unwanted {
		if strings.Contains(haystack, pdfText(value)) {
			t.Errorf("%s prints %q; it must not appear there", name, value)
		}
	}
}

// periodTableText isolates the printed period table from the chart and the
// projection summary. The same date can legitimately name a ghost in the chart
// or a remaining day in the summary, so a row assertion has to read the table
// alone: the slice runs from the "By period" heading to the projection heading
// that follows the total.
func periodTableText(text string) string {
	start := strings.Index(text, "By period")
	if start < 0 {
		return ""
	}
	end := strings.Index(text[start:], "Current-period projection")
	if end < 0 {
		return text[start:]
	}
	return text[start : start+end]
}

func requireInOrder(t *testing.T, text string, wants ...string) {
	t.Helper()
	at := 0
	for _, want := range wants {
		index := strings.Index(text[at:], want)
		if index < 0 {
			t.Errorf("rendered report does not print %q after %q", want, text[at:])
			return
		}
		at += index + len(want)
	}
}

// TestSampleReportPrintsTheFrozenSnapshot pins the words a reader uses to
// judge the document: the filter, the KPI figures, the negative margin, the
// unknown percentage and the internal nature of the report.
func TestSampleReportPrintsTheFrozenSnapshot(t *testing.T) {
	text := renderPageText(t, sampleReport())
	requireText(t, text,
		"Purchase Order Performance Report",
		"INTERNAL REPORT",
		"Internal report: not a customer quotation. PPN is excluded from every figure.",
		"PT. Arcon Penta Persada",
		"2025-01-01 → 2025-12-20 (exclusive)",
		"Monthly",
		"All categories (all)",
		"IDR",
		"20 Dec 2025 08:30:00 UTC",
		"2025-12-20",
		"PO COUNT",
		"24",
		"PO AMOUNT",
		"Rp1.234.567.890",
		"CAPITAL",
		"Rp810.000.000",
		"PO MARGIN",
		"Rp100.000.000",
		"PO MARGIN %",
		"12.35%",
		"Costed Rp910.000.000 of Rp1.234.567.890 (73.71%)",
		"Uncosted Rp324.567.890",
		"2 purchase order(s) in this range have no live source link; unresolved cost is shown as uncosted.",
	)
}

func TestSampleReportPrintsCategoryAndPeriodTables(t *testing.T) {
	text := renderPageText(t, sampleReport())
	requireText(t, text,
		"By business category",
		"Facility",
		"Management",
		"(FM)",
		"Fabrication",
		"(FAB)",
		"Goods Supply &",
		"General",
		"(SUPPLY_GEN)",
		"Others",
		"(OTHERS)",
		"Unassigned",
		"(UNASSIGNED)",
		"By period",
		"Jan 2025",
		"Dec 2025 IP",
		"Rp-30.000.000",
		"-",
		"Total",
		"P = partial period; IP = period still in progress.",
	)
	// A historical range keeps every actual period: mode none withholds no row.
	table := periodTableText(text)
	requireText(t, table, "Jan 2025", "Dec 2025 IP", "Total")
}

func TestRenderedTableTotalsMatchTheFootedFixture(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot poreport.Report
		total    []string
	}{
		{
			name:     "sample",
			snapshot: sampleReport(),
			total: []string{
				"Total", "24", "Rp1.234.567.890", "Rp910.000.000",
				"Rp810.000.000", "Rp100.000.000", "12.35%",
			},
		},
		{
			name:     "long120",
			snapshot: longReport(poreport.MaxPeriods),
			total: []string{
				"Total", "120", "Rp1.200.000.000", "Rp1.080.000.000",
				"Rp960.000.000", "Rp120.000.000", "12.50%",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireInOrder(t, renderPageText(t, tc.snapshot), tc.total...)
		})
	}
}

func TestHistoricalReportPrintsTheNoProjectionReason(t *testing.T) {
	text := renderPageText(t, sampleReport())
	requireText(t, text,
		"Current-period projection",
		"Mode: None  ·  not the current month or year",
		"Unavailable",
		"REASON  not_current_period",
		"This range is not the current month or year.",
		"A projection applies to the current month or year.",
		"This forecast updates as data arrives.",
		"Projected figures never enter an actual table or total.",
	)
	for _, retired := range []string{
		"Monthly projection",
		"Yearly projection",
		"Projection detail",
		"Projected range",
		"PROJECTION  ·  ESTIMATES",
	} {
		if strings.Contains(text, retired) {
			t.Errorf("historical report still prints the retired projection section %q", retired)
		}
	}
}

func TestDayReportPrintsDailyRowsAndGhostSummary(t *testing.T) {
	text := renderPageText(t, dayReport())
	requireText(t, text,
		"By period",
		"1 Nov 2026",
		"19 Nov 2026 IP",
		"Current-period projection",
		"Mode: Day  ·  November 2026 day by day",
		"Available",
		"20 Nov 2026 to 30 Nov 2026 · 11 days",
		"low Rp8.000.000",
		"base Rp10.000.000",
		"high Rp12.000.000",
		"1 Nov 2026 to 20 Nov 2026 (exclusive) · 19 elapsed days",
		"19 purchase orders",
		"as of 19 Nov 2026.",
		"Elapsed days of the current month [elapsed_days]",
		"Actual and projected days  ·  November 2026",
		"Projected (ghost)",
		"Projected low-high",
		"updates as data arrives",
	)
	// The actual table stops at the as-of date. Every later day is ghost
	// territory: named by the summary and drawn on the chart, never a zero row.
	table := periodTableText(text)
	requireText(t, table, "1 Nov 2026", "19 Nov 2026 IP")
	requireNoText(t, "the day report's period table", table,
		"20 Nov 2026", "21 Nov 2026", "30 Nov 2026")
	requireText(t, text, "20 Nov 2026 to 30 Nov 2026 · 11 days")
	for _, retired := range []string{"Monthly projection", "Yearly projection", "Projected range"} {
		if strings.Contains(text, retired) {
			t.Errorf("day report still prints the retired projection section %q", retired)
		}
	}
	if !strings.Contains(text, "Rp190.000.000") {
		t.Errorf("day report total does not print the 19 actual days")
	}
	if strings.Contains(text, "Rp300.000.000") {
		t.Errorf("day report total includes the 11 ghost days; projections must not enter a total")
	}
}

func TestCurrentYearReportPrintsTheStackedMonthRemainder(t *testing.T) {
	text := renderPageText(t, currentYearReport())
	requireText(t, text,
		"Current-period projection",
		"Mode: Month  ·  2026 month by month",
		"Available",
		"Rp100.000.000 amount",
		"Rp70.000.000 capital",
		"Rp20.000.000 margin",
		"Current month remainder:",
		"low Rp44.000.000",
		"base Rp55.000.000",
		"high Rp66.000.000",
		"December 2026 · 1 ghost bar",
		"low Rp80.000.000",
		"1 Dec 2025 to 1 Nov 2026 (exclusive) · 11 complete months",
		"Actual and projected months  ·  2026",
		"Future months are hatched ghosts; the current month stacks its actual columns with the ghost remainder.",
		"Projected (ghost)",
	)
	// The table stops at the current month. December is a ghost on the chart and
	// a line in the summary, never a zero actual row.
	table := periodTableText(text)
	requireText(t, table, "Jan 2026", "Nov 2026 IP")
	requireNoText(t, "the current-year period table", table, "Dec 2026")
	requireText(t, text, "December 2026 · 1 ghost bar")
	if !strings.Contains(text, "Rp1.100.000.000") {
		t.Errorf("current-year report total does not print the eleven actual months")
	}
	if strings.Contains(text, "Rp1.200.000.000") {
		t.Errorf("current-year report total includes the December ghost; projections must not enter a total")
	}
}

func TestSampleReportPrintsDefinitionsAndNotes(t *testing.T) {
	text := renderPageText(t, sampleReport())
	requireText(t, text,
		"Report notes",
		"Some purchase orders in this range carry a retired whole-order discount or adjustment.",
		"whole-order discount or adjustment.",
		"Definitions",
		"PPN is excluded from every amount, costed amount, capital and margin figure in this report.",
		"Capital is the pinned source revision's pre-margin unit cost at the purchase order's quantity",
		"PO margin is costed amount minus capital.",
		"PPH23 is not subtracted: this is a PO margin, not the quotation's nett profit.",
		"Corrections are counted once at their current values.",
		"Current-period projections are estimates from elapsed days or complete months.",
		"Partial and in-progress periods are marked in the period table.",
		"All money is in IDR and no figure is converted between currencies.",
	)
}

func TestUnavailableCurrentProjectionPrintsReasonInsteadOfNumbers(t *testing.T) {
	text := renderPageText(t, unavailableCurrentReport())
	requireText(t, text,
		"Current-period projection",
		"Unavailable",
		"REASON  insufficient_days",
		"Not enough elapsed days: the current month needs five elapsed days and three purchase orders.",
		"1 Nov 2026 to 20 Nov 2026 (exclusive) · 3 elapsed days · 2 purchase orders · as of 19 Nov 2026.",
		"updates as data arrives",
	)
	if strings.Contains(text, "Per remaining day") {
		t.Errorf("an unavailable projection printed per-day projected figures")
	}
	// No ghost is drawn, but the month-bucketed day range still stops its actual
	// table at the as-of date: the trim follows the mode, not the status.
	table := periodTableText(text)
	requireText(t, table, "19 Nov 2026 IP")
	requireNoText(t, "an unavailable day report's period table", table, "20 Nov 2026", "30 Nov 2026")
}

func TestUnavailableProjectionIgnoresSuppliedBuckets(t *testing.T) {
	snapshot := dayReport()
	snapshot.CurrentProjection.Status = poreport.ProjectionStatusUnavailable
	snapshot.CurrentProjection.Reason = poreport.ReasonInsufficientDays
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("unavailable snapshot with stale buckets is refused: %v", err)
	}
	text := renderPageText(t, snapshot)
	requireText(t, text, "Unavailable", "REASON  insufficient_days")
	for _, forbidden := range []string{"Per remaining day", "Projected (ghost)", "Actual and projected days"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("an unavailable forecast still printed %q", forbidden)
		}
	}
}

func TestEmptyReportPrintsAnUnambiguousEmptyState(t *testing.T) {
	snapshot := unavailableProjectionReport()
	snapshot.Periods = nil
	snapshot.Categories = nil
	snapshot.CategorySeries = nil
	snapshot.Totals = poreport.Metrics{}
	snapshot.Population.SourceMissingPoCount = 0
	text := renderPageText(t, snapshot)
	requireText(t, text,
		"No purchase orders in the selected range.",
		"The report still prints its filter, its definitions and the projection status so the empty result is unambiguous.",
		"No category rows",
	)
}

func TestSinglePeriodChartDoesNotInventADataPoint(t *testing.T) {
	snapshot := longReport(1)
	text := renderPageText(t, snapshot)
	requireText(t, text, "Actual periods", "Jan 2016", "Total")
	if strings.Contains(text, "Actual periods  ·  1 of 2") {
		t.Errorf("one period was split across chart panels")
	}
}

func TestReportUsesTheFilterCurrencyAndNeverFallsBackToIDR(t *testing.T) {
	snapshot := sampleReport()
	snapshot.Filter.Currency = "USD"
	snapshot.Totals.AmountMinor = 123_456
	snapshot.Totals.CostedMinor = 100_000
	snapshot.Totals.UncostedMinor = 23_456
	snapshot.Totals.CapitalMinor = 80_000
	snapshot.Totals.MarginMinor = 20_000
	text := renderPageText(t, snapshot)
	requireText(t, text, "CURRENCY", "USD", "PO AMOUNT", "$1,234.56", "CAPITAL", "$800.00")
	if strings.Contains(text, "Rp1.234.567.890") {
		t.Errorf("a USD report printed an IDR figure")
	}

	unknown := sampleReport()
	unknown.Filter.Currency = "ZZZ"
	if err := ValidateSnapshot(unknown); err == nil {
		t.Errorf("an unknown currency was accepted instead of being refused")
	}
}
