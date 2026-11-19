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

func requireText(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("rendered report does not print %q", want)
		}
	}
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
		"Internal report — not a customer quotation. PPN is excluded from every figure.",
		"PT. Arcon Penta Persada",
		"2025-01-01 → 2026-01-01 (exclusive)",
		"Monthly",
		"All categories (all)",
		"IDR",
		"05 Jan 2026 08:30:00 UTC",
		"2026-01-04",
		"PO COUNT",
		"24",
		"PO AMOUNT",
		"Rp1.234.567.890",
		"CAPITAL",
		"Rp830.000.000",
		"PO MARGIN",
		"Rp170.000.000",
		"PO MARGIN %",
		"20.48%",
		"Costed Rp1.000.000.000 of Rp1.234.567.890 (81.00%)",
		"Uncosted Rp234.567.890",
		"2 purchase order(s) in this range have a deleted source quotation; their amount is uncosted.",
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
		"—",
		"Total",
		"P = partial period; IP = period still in progress.",
	)
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
				"Total", "24", "Rp1.234.567.890", "Rp1.000.000.000",
				"Rp830.000.000", "Rp170.000.000", "20.48%",
			},
		},
		{
			name:     "long120",
			snapshot: longReport(poreport.MaxPeriods),
			total: []string{
				"Total", "120", "Rp1.200.000.000", "Rp1.080.000.000",
				"Rp960.000.000", "Rp120.000.000", "11.11%",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireInOrder(t, renderPageText(t, tc.snapshot), tc.total...)
		})
	}
}

func TestSampleReportPrintsProjectionRangeBasisAndMethod(t *testing.T) {
	text := renderPageText(t, sampleReport())
	requireText(t, text,
		"PROJECTION  ·  ESTIMATES, NEVER INCLUDED IN ACTUAL TOTALS",
		"Monthly projection  ·  next 12 months",
		"Yearly projection  ·  next 3 years",
		"Projection detail",
		"Projected range",
		"LOW",
		"BASE",
		"HIGH",
		"Available",
		"Method: ",
		"Median of the same calendar month across complete history",
		"months when that calendar month has no history.",
		"Basis: ",
		"History 2024-07-01 to 2026-01-01 (exclusive)",
		"18 complete months",
		"24 purchase orders",
		"as of 2026-01-04.",
		"Same calendar month across complete history [same_calendar_month]",
		"Trailing twelve complete months [trailing_twelve_complete_months]",
	)
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
		"PPH22 is not subtracted: this is a PO margin, not the quotation's nett profit.",
		"Corrections are counted once at their current values.",
		"Projected periods are estimates from complete history.",
		"Partial and in-progress periods are marked in the period table.",
		"All money is in IDR and no figure is converted between currencies.",
	)
}

func TestUnavailableProjectionPrintsReasonInsteadOfNumbers(t *testing.T) {
	text := renderPageText(t, unavailableProjectionReport())
	requireText(t, text,
		"Unavailable",
		"REASON  insufficient_history",
		"Not enough complete history: the monthly forecast needs six complete months and six POs",
		"No complete history window is available; as of 2026-01-04.",
	)
	if strings.Contains(text, "Projected range") {
		t.Errorf("an unavailable projection printed a projected range table")
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
