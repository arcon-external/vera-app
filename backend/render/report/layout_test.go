package report

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"vera-app/backend/currency"
	"vera-app/backend/poreport"
)

// extremeMoneyRow is the widest accepted money row that still obeys the
// domain identities: amount is the exact maximum, all of it except one minor
// unit is uncosted, capital is the exact maximum, and the margin is the signed
// difference. The percentage and coverage are therefore derived, not the
// synthetic ±MaxExactMinor placeholders this fixture used to carry. Only the
// standalone formatting/width guard below uses a maximum percentage string.
func extremeMoneyRow() poreport.Metrics {
	costed := int64(1)
	capital := poreport.MaxExactMinor
	margin := costed - capital
	return poreport.Metrics{
		PoCount:       poreport.MaxExactMinor,
		AmountMinor:   poreport.MaxExactMinor,
		CostedMinor:   costed,
		UncostedMinor: poreport.MaxExactMinor - costed,
		CapitalMinor:  capital,
		MarginMinor:   margin,
		MarginBps:     fixtureRatio(margin, capital),
		CoverageBps:   fixtureRatio(costed, poreport.MaxExactMinor),
	}
}

func extremeReport() poreport.Report {
	snapshot := sampleReport()
	snapshot.Company.Name = strings.Repeat("W", 256)
	snapshot.Company.Address = strings.Repeat("A", 1024)
	snapshot.Filter.Currency = "IDR"
	snapshot.Totals = extremeMoneyRow()
	snapshot.Categories[0].Label = strings.Repeat("W", 128)
	for index := range snapshot.Categories {
		snapshot.Categories[index].Metrics = extremeMoneyRow()
	}
	for index := range snapshot.Periods {
		snapshot.Periods[index].Metrics = extremeMoneyRow()
	}
	if len(snapshot.Projection.Monthly.Buckets) > 0 {
		snapshot.Projection.Monthly.Buckets[0].AmountLowMinor = -poreport.MaxExactMinor
		snapshot.Projection.Monthly.Buckets[0].AmountBaseMinor = 0
		snapshot.Projection.Monthly.Buckets[0].AmountHighMinor = poreport.MaxExactMinor
		snapshot.Projection.Monthly.Buckets[0].CapitalMinor = poreport.MaxExactMinor
		snapshot.Projection.Monthly.Buckets[0].MarginMinor = -poreport.MaxExactMinor
		snapshot.Projection.Monthly.Buckets[0].MarginBps = fixtureRatio(-poreport.MaxExactMinor, poreport.MaxExactMinor)
	}
	return snapshot
}

func assertLinesFit(t *testing.T, name string, lines []string, size, maxWidth float64, bold bool) {
	t.Helper()
	faces := testFonts()
	for index, line := range lines {
		if width := faces.width(line, size, bold); width > maxWidth+0.01 {
			t.Errorf("%s line %d %q is %.2f wide, box is %.2f", name, index, line, width, maxWidth)
		}
	}
}

func TestWrapCellKeepsEveryCharacterAndFits(t *testing.T) {
	faces := testFonts()
	cases := []struct {
		name     string
		text     string
		size     float64
		maxWidth float64
		bold     bool
	}{
		{"max IDR", formatMoney("IDR", poreport.MaxExactMinor), tableBodySize, 84, true},
		{"negative max IDR", formatMoney("IDR", -poreport.MaxExactMinor), tableBodySize, 70, true},
		{"max EUR", formatMoney("EUR", poreport.MaxExactMinor), tableBodySize, 70, true},
		{"supply label", "Goods Supply & General (SUPPLY_GEN)", tableBodySize, 78, false},
		{"128-char category", strings.Repeat("W", 128), tableBodySize, 78, false},
		{"256-char company", strings.Repeat("W", 256), 10.5, 300, true},
		{"1024-char address", strings.Repeat("A", 1024), 8, 300, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := faces.wrapCell(tc.text, tc.size, tc.maxWidth, tc.bold)
			assertLinesFit(t, tc.name, lines, tc.size, tc.maxWidth, tc.bold)
			normalized := func(value string) string {
				return strings.Join(strings.Fields(value), "")
			}
			if got := normalized(strings.Join(lines, " ")); got != normalized(tc.text) {
				t.Errorf("%s lost characters:\n got %q\nwant %q", tc.name, got, normalized(tc.text))
			}
		})
	}
}

// productionTableColumns is every column the report tables actually draw,
// including the 68pt projected capital/margin columns and the 45pt projected
// percentage column. Reading the shared definitions means a new production
// width cannot be added without this guard seeing it.
func productionTableColumns() []tableColumn {
	columns := append([]tableColumn{}, actualTableColumns("CATEGORY")...)
	return append(columns, projectionTableColumns()...)
}

// TestEveryAcceptedTableAmountFitsItsColumn pins the width half of the extreme
// snapshot contract: for every supported currency and every production column
// width, the largest exact amount is split into lines that each fit a table
// cell. No figure is truncated and no line crosses into the next column.
func TestEveryAcceptedTableAmountFitsItsColumn(t *testing.T) {
	faces := testFonts()
	for _, code := range currency.Codes() {
		for _, minor := range []int64{poreport.MaxExactMinor, -poreport.MaxExactMinor} {
			text := formatMoney(code, minor)
			for _, column := range productionTableColumns() {
				width := column.width - 2*tableCellPadX
				lines := faces.wrapCell(text, tableBodySize, width, true)
				assertLinesFit(t, code, lines, tableBodySize, width, true)
				joined := strings.Join(strings.Fields(strings.Join(lines, "")), "")
				want := strings.Join(strings.Fields(text), "")
				if joined != want {
					t.Errorf("%s amount split lost characters in %s: %q from %q",
						code, column.title, joined, want)
				}
			}
		}
	}
}

// TestEveryAcceptedPercentageFitsItsColumn covers the second narrow column
// family: the actual tables' MARGIN % column and the projection's 45pt MARGIN %
// column. The maximum percentage text is the synthetic widest case for this
// width guard; the assembled reports themselves carry derived percentages.
func TestEveryAcceptedPercentageFitsItsColumn(t *testing.T) {
	faces := testFonts()
	widths := []float64{}
	for _, column := range productionTableColumns() {
		if column.title == "MARGIN %" {
			widths = append(widths, column.width)
		}
	}
	if len(widths) != 2 {
		t.Fatalf("MARGIN %% columns = %v; want the actual and projected widths", widths)
	}
	for _, value := range []*int64{
		pointer(poreport.MaxExactMinor),
		pointer(-poreport.MaxExactMinor),
		nil,
	} {
		text := formatPct(value)
		for _, width := range widths {
			available := width - 2*tableCellPadX
			lines := faces.wrapCell(text, tableBodySize, available, true)
			assertLinesFit(t, "pct "+text, lines, tableBodySize, available, true)
			joined := strings.Join(strings.Fields(strings.Join(lines, "")), "")
			want := strings.Join(strings.Fields(text), "")
			if joined != want {
				t.Errorf("percentage split lost characters at %.2fpt: %q from %q",
					width, joined, want)
			}
		}
	}
}

func TestUnknownPercentageFitsTheProjectedMarginColumn(t *testing.T) {
	faces := testFonts()
	text := formatPct(nil)
	width := 45 - 2*tableCellPadX
	lines := faces.wrapCell(text, tableBodySize, width, true)
	assertLinesFit(t, "unknown pct "+text, lines, tableBodySize, width, true)
	if len(lines) != 1 {
		t.Errorf("unknown percentage wrapped into %d lines in a 45pt column: %q", len(lines), lines)
	}
}

func TestTableRowHeightGrowsForWrappedCells(t *testing.T) {
	renderer := &reportRenderer{faces: testFonts()}
	columns := []tableColumn{
		{title: "CATEGORY", width: 86, align: alignLeft},
		{title: "AMOUNT", width: 88, align: alignRight},
	}
	oneLine := tableRow{cells: []string{"Fabrication (FAB)", formatMoney("IDR", 1_000_000)}}
	wrapped := tableRow{cells: []string{
		strings.Repeat("W", 128),
		formatMoney("IDR", poreport.MaxExactMinor),
	}}
	if got, want := renderer.tableRowHeight(columns, oneLine, false), tableLineHeight+tableCellPadTop+tableCellPadBottom; got != want {
		t.Errorf("single-line row height = %.2f, want %.2f", got, want)
	}
	if got := renderer.tableRowHeight(columns, wrapped, false); got <= tableLineHeight+tableCellPadTop+tableCellPadBottom {
		t.Errorf("wrapped row height = %.2f; it did not grow for wrapped cells", got)
	}
}

func TestExtremeAcceptedSnapshotRenders(t *testing.T) {
	snapshot := extremeReport()
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("extreme snapshot is refused: %v", err)
	}
	renderer, err := testReportRenderer(snapshot)
	if err != nil {
		t.Fatalf("layout extreme snapshot: %v", err)
	}
	text := pageText(renderer.canvas.pages)
	requireText(t, text, "SUPPLY_GEN", "PO AMOUNT", "Rp9.007.199.254.740.991")
	if got := strings.Count(text, "W"); got < 256+128 {
		t.Errorf("extreme report carries %d W characters; the 256-character company and 128-character category label lost text", got)
	}
	if got := strings.Count(text, "A"); got < 1024 {
		t.Errorf("extreme report carries %d A characters; the 1024-character address lost text", got)
	}
	if len(renderer.canvas.pages) < 3 {
		t.Errorf("extreme report has %d pages; it should still paginate", len(renderer.canvas.pages))
	}
	pdf, err := renderer.canvas.render("Purchase Order Performance Report")
	if err != nil {
		t.Fatalf("render extreme PDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) || len(pdf) < 10_000 {
		t.Errorf("extreme render is not a plausible PDF (%d bytes)", len(pdf))
	}
}

func TestExtremeLongReportKeepsEveryPeriod(t *testing.T) {
	snapshot := longReport(poreport.MaxPeriods)
	for index := range snapshot.Periods {
		snapshot.Periods[index].Metrics = extremeMoneyRow()
	}
	renderer, err := testReportRenderer(snapshot)
	if err != nil {
		t.Fatalf("layout 120-row extreme report: %v", err)
	}
	text := pageText(renderer.canvas.pages)
	if got := strings.Count(text, "Rp9.007.199.254."); got < 120 {
		t.Errorf("extreme 120-row report prints the amount prefix %d times; want at least one per period", got)
	}
	if got := strings.Count(text, "740.991"); got < 120 {
		t.Errorf("extreme 120-row report prints the amount suffix %d times; a period amount was truncated", got)
	}
	for index := 0; index < poreport.MaxPeriods; index++ {
		date := time.Date(2016, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, index, 0)
		label := date.Format("Jan 2006")
		if !strings.Contains(text, label) {
			t.Fatalf("period %s is missing from the extreme report", label)
		}
	}
}

func TestExtremePDFRenderDoesNotNeedChrome(t *testing.T) {
	pdf, err := Render(extremeReport(), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render extreme snapshot: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("Render did not produce a PDF")
	}
}
