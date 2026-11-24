package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"vera-app/backend/poreport"
)

// Issue #683: the Excel export. The workbook is built from the same validated
// ExportDocument as the PDF, so these tests drive the public RenderXLSX seam
// only: they open the produced bytes with a real spreadsheet reader, read cells
// back, and inspect the chart parts. No test reaches inside the builder.

func renderXLSXSnapshot(t *testing.T, snapshot poreport.Report) []byte {
	t.Helper()
	raw, err := RenderXLSX(ExportDocument{Legacy: &snapshot})
	if err != nil {
		t.Fatalf("RenderXLSX: %v", err)
	}
	return raw
}

func renderXLSXView(t *testing.T, view ViewEnvelope) []byte {
	t.Helper()
	raw, err := RenderXLSX(ExportDocument{View: &view})
	if err != nil {
		t.Fatalf("RenderXLSX(view): %v", err)
	}
	return raw
}

// rowNumberWithFirstCell returns the 1-based row whose first cell starts with
// prefix. Period rows are keyed by their ISO period start.
func rowNumberWithFirstCell(t *testing.T, workbook *excelize.File, sheet, prefix string) int {
	t.Helper()
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		t.Fatalf("read %s rows: %v", sheet, err)
	}
	for index, row := range rows {
		if len(row) > 0 && strings.HasPrefix(strings.TrimSpace(row[0]), prefix) {
			return index + 1
		}
	}
	t.Fatalf("%s has no row starting with %q", sheet, prefix)
	return 0
}

func openWorkbook(t *testing.T, raw []byte) *excelize.File {
	t.Helper()
	workbook, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	t.Cleanup(func() { _ = workbook.Close() })
	return workbook
}

// workbookParts reads every zip entry. It is the raw evidence for the parts a
// workbook must carry (worksheets, charts) and for the absence of cached
// formula errors.
func workbookParts(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("read workbook zip: %v", err)
	}
	parts := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("open workbook part %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			t.Fatalf("read workbook part %s: %v", file.Name, err)
		}
		parts[file.Name] = string(content)
	}
	return parts
}

func chartParts(parts map[string]string) []string {
	var names []string
	for name := range parts {
		if strings.HasPrefix(name, "xl/charts/chart") && strings.HasSuffix(name, ".xml") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// rowWithLabel finds the row whose first cell is exactly label. Summary and
// Forecast are label/value tables, so this keeps the assertions independent of
// row numbers that formatting changes could shift.
func rowWithLabel(t *testing.T, workbook *excelize.File, sheet, label string) []string {
	t.Helper()
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		t.Fatalf("read %s rows: %v", sheet, err)
	}
	for _, row := range rows {
		if len(row) > 0 && strings.TrimSpace(row[0]) == label {
			return row
		}
	}
	t.Fatalf("%s has no row labelled %q", sheet, label)
	return nil
}

// rowWithFirstCellContaining finds a row by a prefix, for the period rows whose
// first cell is the ISO period start.
func rowWithFirstCellContaining(t *testing.T, workbook *excelize.File, sheet, prefix string) []string {
	t.Helper()
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		t.Fatalf("read %s rows: %v", sheet, err)
	}
	for _, row := range rows {
		if len(row) > 0 && strings.HasPrefix(strings.TrimSpace(row[0]), prefix) {
			return row
		}
	}
	t.Fatalf("%s has no row starting with %q", sheet, prefix)
	return nil
}

func cellAt(row []string, index int) string {
	if index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

// rawCell reads the stored value rather than the display text. A numeric cell
// with a currency format reads back as "Rp1,234,567,890" through GetRows; the
// stored number is the figure the report owns.
func rawCell(t *testing.T, workbook *excelize.File, sheet string, row, column int) string {
	t.Helper()
	name, err := excelize.ColumnNumberToName(column + 1)
	if err != nil {
		t.Fatalf("column name for %d: %v", column, err)
	}
	value, err := workbook.GetCellValue(sheet, fmt.Sprintf("%s%d", name, row), excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("read %s!%s%d: %v", sheet, name, row, err)
	}
	return strings.TrimSpace(value)
}

func numberAt(t *testing.T, workbook *excelize.File, sheet string, row, column int) float64 {
	t.Helper()
	text := rawCell(t, workbook, sheet, row, column)
	var value float64
	if _, err := fmt.Sscanf(text, "%g", &value); err != nil {
		t.Fatalf("%s row %d cell %d = %q is not a number: %v", sheet, row, column, text, err)
	}
	return value
}

// labelNumber reads the value column of the Summary's label/value rows.
func labelNumber(t *testing.T, workbook *excelize.File, label string) float64 {
	t.Helper()
	return numberAt(t, workbook, "Summary", rowNumberWithLabel(t, workbook, "Summary", label), 1)
}

func assertNumber(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func assertSheetList(t *testing.T, workbook *excelize.File, want []string) {
	t.Helper()
	got := workbook.GetSheetList()
	if len(got) != len(want) {
		t.Fatalf("sheets = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("sheets = %v, want %v", got, want)
		}
	}
}

// rowNumberWithLabel returns the 1-based row whose first cell is exactly label.
func rowNumberWithLabel(t *testing.T, workbook *excelize.File, sheet, label string) int {
	t.Helper()
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		t.Fatalf("read %s rows: %v", sheet, err)
	}
	for index, row := range rows {
		if len(row) > 0 && strings.TrimSpace(row[0]) == label {
			return index + 1
		}
	}
	t.Fatalf("%s has no row labelled %q", sheet, label)
	return 0
}

// cellNumberFormat reads the stored format code of a cell. It is the real
// evidence for the workbook's display contract: Excel applies this code, and a
// test that only compared raw values could not tell 0 from "-" or a loss from a
// gain.
func cellNumberFormat(t *testing.T, workbook *excelize.File, sheet, cell string) string {
	t.Helper()
	styleID, err := workbook.GetCellStyle(sheet, cell)
	if err != nil {
		t.Fatalf("read %s!%s style: %v", sheet, cell, err)
	}
	style, err := workbook.GetStyle(styleID)
	if err != nil {
		t.Fatalf("read %s!%s style record: %v", sheet, cell, err)
	}
	if style.CustomNumFmt == nil {
		return ""
	}
	return *style.CustomNumFmt
}

func summaryValue(t *testing.T, workbook *excelize.File, label string) []string {
	t.Helper()
	return rowWithLabel(t, workbook, "Summary", label)
}

// The Summary sheet is the answer page. Its figures are the frozen
// server totals, its percentages are fractions (percent formats) and its money
// is the currency's own exponent, so IDR prints whole rupiah and USD prints
// cents. A nil ratio is a dash, never zero percent.
func TestRenderXLSXSummaryCarriesTheServerFiguresAndFormats(t *testing.T) {
	snapshot := sampleReport()
	snapshot.Pipeline = poreport.PipelineSummary{
		UnsoldAmountMinor: 4_000_000, UnsoldMarginMinor: 1_000_000, FulfillmentBps: 6_250,
	}
	snapshot.Variance = poreport.VarianceSummary{
		QuantityExtraIncomeAmountMinor: 2_000_000,
		QuantityExtraIncomeMarginMinor: 500_000,
		PriceExtraIncomeMinor:          300_000,
		PriceLossMinor:                 250_000,
		ClosedLossAmountMinor:          1_000_000,
		ClosedLossMarginMinor:          400_000,
		NetRealizedVarianceAmountMinor: -1_500_000,
		NetRealizedVarianceMarginMinor: -800_000,
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("fixture no longer validates: %v", err)
	}
	workbook := openWorkbook(t, renderXLSXSnapshot(t, snapshot))

	header := rowWithLabel(t, workbook, "Summary", "Period")
	if got := cellAt(header, 1); got != "2025-01-01 → 2025-12-20 (exclusive)" {
		t.Errorf("summary period = %q", got)
	}
	if got := cellAt(summaryValue(t, workbook, "Bucket"), 1); got != "Monthly" {
		t.Errorf("summary bucket = %q", got)
	}
	if got := cellAt(summaryValue(t, workbook, "Category"), 1); got != "All categories (all)" {
		t.Errorf("summary category = %q", got)
	}
	if got := cellAt(summaryValue(t, workbook, "Currency"), 1); got != "IDR" {
		t.Errorf("summary currency = %q", got)
	}
	if got := cellAt(summaryValue(t, workbook, "As of"), 1); got != "2025-12-20" {
		t.Errorf("summary as-of = %q", got)
	}

	// The totals are the fixture's own figures, computed independently by the
	// fixture builder: 12 months of 100,000,000 plus December's 134,567,890.
	assertNumber(t, "PO count", labelNumber(t, workbook, "PO count"), 24)
	assertNumber(t, "PO amount", labelNumber(t, workbook, "PO amount"), 1_234_567_890)
	assertNumber(t, "Costed amount", labelNumber(t, workbook, "Costed amount"), 910_000_000)
	assertNumber(t, "Capital", labelNumber(t, workbook, "Capital"), 810_000_000)
	assertNumber(t, "PO margin", labelNumber(t, workbook, "PO margin"), 100_000_000)
	assertNumber(t, "PO margin %", labelNumber(t, workbook, "PO margin %"), 0.1235)
	assertNumber(t, "Coverage %", labelNumber(t, workbook, "Coverage %"), 0.7371)
	assertNumber(t, "Uncosted", labelNumber(t, workbook, "Uncosted"), 324_567_890)

	assertNumber(t, "Unsold amount", labelNumber(t, workbook, "Unsold amount"), 4_000_000)
	assertNumber(t, "Unsold margin", labelNumber(t, workbook, "Unsold margin"), 1_000_000)
	assertNumber(t, "Fulfillment %", labelNumber(t, workbook, "Fulfillment %"), 0.625)
	assertNumber(t, "Quantity extra amount", labelNumber(t, workbook, "Quantity extra amount"), 2_000_000)
	assertNumber(t, "Price loss", labelNumber(t, workbook, "Price loss"), 250_000)
	assertNumber(t, "Net realized amount", labelNumber(t, workbook, "Net realized amount"), -1_500_000)

	if got := cellAt(summaryValue(t, workbook, "Available currencies"), 1); got != "IDR, USD" {
		t.Errorf("available currencies = %q", got)
	}
	if got := cellAt(summaryValue(t, workbook, "Has unassigned"), 1); got != "Yes" {
		t.Errorf("has-unassigned = %q", got)
	}
	assertNumber(t, "Source-missing POs", labelNumber(t, workbook, "Source-missing POs"), 2)

	// The stored formats are the display contract: the currency's own symbol
	// and exponent, a parenthesised loss, a dash for zero, and a two-decimal
	// percent.
	amountFormat := cellNumberFormat(t, workbook, "Summary",
		fmt.Sprintf("B%d", rowNumberWithLabel(t, workbook, "Summary", "PO amount")))
	if !strings.Contains(amountFormat, `"Rp"#,##0`) || strings.Contains(amountFormat, ".00") {
		t.Errorf("IDR money format = %q; want whole rupiah", amountFormat)
	}
	if !strings.Contains(amountFormat, "(") || !strings.Contains(amountFormat, `"-"`) {
		t.Errorf("IDR money format = %q; want a parenthesised loss and a dash for zero", amountFormat)
	}
	percentCell := fmt.Sprintf("B%d", rowNumberWithLabel(t, workbook, "Summary", "PO margin %"))
	if got := cellNumberFormat(t, workbook, "Summary", percentCell); got != `0.00%;(0.00%);"-"` {
		t.Errorf("percent format = %q", got)
	}

	// An exponent-2 currency stores cents: the same minor figures become major
	// units and the format gains two decimals.
	usd := snapshot
	usd.Filter.Currency = "USD"
	usdBook := openWorkbook(t, renderXLSXSnapshot(t, usd))
	assertNumber(t, "USD PO amount", labelNumber(t, usdBook, "PO amount"), 12_345_678.90)
	usdFormat := cellNumberFormat(t, usdBook, "Summary",
		fmt.Sprintf("B%d", rowNumberWithLabel(t, usdBook, "Summary", "PO amount")))
	if !strings.Contains(usdFormat, `"$"#,##0.00`) {
		t.Errorf("USD money format = %q; want two decimals", usdFormat)
	}
}

// The evidence pages. Period and category rows are the server's rows,
// and the workbook carries native charts — not pictures — so the reader can
// retitle, restyle or re-range them in Excel.
func TestRenderXLSXPeriodsAndCategoriesCarryRowsAndNativeCharts(t *testing.T) {
	raw := renderXLSXSnapshot(t, sampleReport())
	workbook := openWorkbook(t, raw)

	periodsHeader := rowWithLabel(t, workbook, "Periods", "Period")
	for column, want := range []string{
		"Period", "PO count", "Amount", "Costed", "Capital", "Margin", "Margin %", "Coverage %", "Uncosted", "Status",
	} {
		if got := cellAt(periodsHeader, column); got != want {
			t.Errorf("Periods header column %d = %q, want %q", column, got, want)
		}
	}

	january := rowNumberWithFirstCell(t, workbook, "Periods", "2025-01-01")
	assertNumber(t, "January PO count", numberAt(t, workbook, "Periods", january, 1), 2)
	assertNumber(t, "January amount", numberAt(t, workbook, "Periods", january, 2), 100_000_000)
	assertNumber(t, "January costed", numberAt(t, workbook, "Periods", january, 3), 90_000_000)
	assertNumber(t, "January capital", numberAt(t, workbook, "Periods", january, 4), 70_000_000)
	assertNumber(t, "January margin", numberAt(t, workbook, "Periods", january, 5), 20_000_000)
	assertNumber(t, "January margin %", numberAt(t, workbook, "Periods", january, 6), 0.2857)
	assertNumber(t, "January coverage %", numberAt(t, workbook, "Periods", january, 7), 0.9)
	assertNumber(t, "January uncosted", numberAt(t, workbook, "Periods", january, 8), 10_000_000)

	december := rowNumberWithFirstCell(t, workbook, "Periods", "2025-12-01")
	assertNumber(t, "December amount", numberAt(t, workbook, "Periods", december, 2), 134_567_890)
	assertNumber(t, "December margin", numberAt(t, workbook, "Periods", december, 5), -50_000_000)
	if got := rawCell(t, workbook, "Periods", december, 5); !strings.HasPrefix(got, "-") {
		t.Errorf("December margin stored as %q; want a negative value", got)
	}

	totalRow := rowNumberWithLabel(t, workbook, "Periods", "Report total")
	assertNumber(t, "report total amount", numberAt(t, workbook, "Periods", totalRow, 2), 1_234_567_890)
	assertNumber(t, "report total margin %", numberAt(t, workbook, "Periods", totalRow, 6), 0.1235)

	categoriesHeader := rowWithLabel(t, workbook, "Categories", "Code")
	for column, want := range []string{
		"Code", "Category", "PO count", "Amount", "Costed", "Capital", "Margin", "Margin %", "Coverage %", "Uncosted",
	} {
		if got := cellAt(categoriesHeader, column); got != want {
			t.Errorf("Categories header column %d = %q, want %q", column, got, want)
		}
	}
	fm := rowNumberWithLabel(t, workbook, "Categories", "FM")
	if got := cellAt(rowWithLabel(t, workbook, "Categories", "FM"), 1); got != "Facility Management" {
		t.Errorf("FM label = %q", got)
	}
	assertNumber(t, "FM amount", numberAt(t, workbook, "Categories", fm, 3), 493_827_156)
	assertNumber(t, "FM costed", numberAt(t, workbook, "Categories", fm, 4), 364_000_000)
	assertNumber(t, "FM capital", numberAt(t, workbook, "Categories", fm, 5), 291_600_000)
	assertNumber(t, "FM margin", numberAt(t, workbook, "Categories", fm, 6), 72_400_000)
	assertNumber(t, "FM margin %", numberAt(t, workbook, "Categories", fm, 7), 0.2483)

	unassigned := rowNumberWithLabel(t, workbook, "Categories", "UNASSIGNED")
	if got := rawCell(t, workbook, "Categories", unassigned, 7); got != "-" {
		t.Errorf("UNASSIGNED margin %% = %q; want an unknown-ratio dash", got)
	}
	categoryTotal := rowNumberWithLabel(t, workbook, "Categories", "Report total")
	assertNumber(t, "category total amount", numberAt(t, workbook, "Categories", categoryTotal, 3), 1_234_567_890)

	parts := workbookParts(t, raw)
	charts := chartParts(parts)
	if len(charts) < 3 {
		t.Fatalf("workbook has %d chart parts (%v); want at least three", len(charts), charts)
	}
	var columns, bars, lines int
	for _, name := range charts {
		content := parts[name]
		if !strings.Contains(content, "<f>") {
			t.Errorf("chart %s has no cell-range reference", name)
		}
		switch {
		case strings.Contains(content, `<barDir val="col"`):
			columns++
		case strings.Contains(content, `<barDir val="bar"`):
			bars++
		case strings.Contains(content, "<lineChart"):
			lines++
		}
	}
	if columns < 1 {
		t.Errorf("no clustered-column chart in %v", charts)
	}
	if bars < 1 {
		t.Errorf("no category bar chart in %v", charts)
	}
	if lines < 1 {
		t.Errorf("no margin-%% line chart in %v", charts)
	}

	// The whole workbook is value-only: a cached formula error anywhere would
	// mean a figure Excel could not evaluate, which this export never emits.
	for name, content := range parts {
		for _, marker := range []string{"#REF!", "#VALUE!", "#DIV/0!"} {
			if strings.Contains(content, marker) {
				t.Errorf("workbook part %s contains %s", name, marker)
			}
		}
	}
}

// The Forecast sheet keeps the projection in its own place and never mixes it
// into the actual tables. When the monthly series is available the workbook
// carries a projection chart of the low/base/high band; when it is not, the
// sheet states the stable reason and draws no forecast chart at all.
func TestRenderXLSXForecastWritesBucketsAndTheProjectionChart(t *testing.T) {
	raw := renderXLSXSnapshot(t, sampleReport())
	workbook := openWorkbook(t, raw)

	if got := cellAt(rowWithLabel(t, workbook, "Forecast", "Status"), 1); got != "Available" {
		t.Errorf("forecast status = %q; want Available", got)
	}
	if got := cellAt(rowWithLabel(t, workbook, "Forecast", "Method"), 1); !strings.Contains(got, "Median of the same calendar month") {
		t.Errorf("forecast method = %q", got)
	}
	if got := cellAt(rowWithLabel(t, workbook, "Forecast", "Basis"), 1); !strings.Contains(got, "17 complete months") {
		t.Errorf("forecast basis = %q; want 17 complete months", got)
	}

	header := rowWithLabel(t, workbook, "Forecast", "Period")
	for column, want := range []string{
		"Period", "Amount low", "Amount base", "Amount high", "Capital", "Margin", "Margin %",
	} {
		if got := cellAt(header, column); got != want {
			t.Errorf("Forecast header column %d = %q, want %q", column, got, want)
		}
	}
	january := rowNumberWithFirstCell(t, workbook, "Forecast", "2026-01-01")
	assertNumber(t, "forecast low", numberAt(t, workbook, "Forecast", january, 1), 8_000_000)
	assertNumber(t, "forecast base", numberAt(t, workbook, "Forecast", january, 2), 10_000_000)
	assertNumber(t, "forecast high", numberAt(t, workbook, "Forecast", january, 3), 13_000_000)
	assertNumber(t, "forecast capital", numberAt(t, workbook, "Forecast", january, 4), 7_500_000)
	assertNumber(t, "forecast margin", numberAt(t, workbook, "Forecast", january, 5), 2_500_000)
	assertNumber(t, "forecast margin %", numberAt(t, workbook, "Forecast", january, 6), 0.3333)

	parts := workbookParts(t, raw)
	charts := chartParts(parts)
	if len(charts) != 4 {
		t.Fatalf("workbook has %d chart parts (%v); want four with an available forecast", len(charts), charts)
	}
	forecastChart := ""
	for _, name := range charts {
		if strings.Contains(parts[name], "Forecast!") {
			forecastChart = name
			break
		}
	}
	if forecastChart == "" {
		t.Fatalf("no chart references the forecast sheet: %v", charts)
	}
	content := parts[forecastChart]
	if !strings.Contains(content, "Forecast amount") {
		t.Errorf("forecast chart has no forecast title: %s", forecastChart)
	}
	for _, column := range []string{"$B$", "$C$", "$D$"} {
		if !strings.Contains(content, "Forecast!"+column) {
			t.Errorf("forecast chart does not plot the %s bucket column", column)
		}
	}
}

func TestRenderXLSXUnavailableForecastWritesTheReasonAndNoChart(t *testing.T) {
	raw := renderXLSXSnapshot(t, unavailableProjectionReport())
	workbook := openWorkbook(t, raw)

	if got := cellAt(rowWithLabel(t, workbook, "Forecast", "Status"), 1); got != "Unavailable" {
		t.Errorf("forecast status = %q; want Unavailable", got)
	}
	if got := cellAt(rowWithLabel(t, workbook, "Forecast", "Reason"), 1); !strings.Contains(got, "Not enough complete history") {
		t.Errorf("forecast reason = %q", got)
	}

	parts := workbookParts(t, raw)
	charts := chartParts(parts)
	if len(charts) != 3 {
		t.Fatalf("workbook has %d chart parts (%v); want three with no forecast", len(charts), charts)
	}
	for _, name := range charts {
		if strings.Contains(parts[name], "Forecast!") {
			t.Errorf("chart %s references the forecast sheet although the series is unavailable", name)
		}
	}
}

// The same validated export document feeds both renderers: a version-1 view is
// accepted and its overview snapshot owns the workbook, exactly as it owns the
// PDF. An invalid document is refused before a workbook exists.
func TestRenderXLSXAcceptsAValidatedViewAndRefusesInvalidDocuments(t *testing.T) {
	view := validPastMonthView(t)
	raw := renderXLSXView(t, view)
	workbook := openWorkbook(t, raw)
	if got := cellAt(rowWithLabel(t, workbook, "Summary", "As of"), 1); got != view.Sources[view.Overview].AsOf {
		t.Errorf("view workbook as-of = %q; want the overview's %q", got, view.Sources[view.Overview].AsOf)
	}

	if _, err := RenderXLSX(ExportDocument{}); err == nil {
		t.Error("an empty export document produced a workbook")
	}
	badCurrency := sampleReport()
	badCurrency.Filter.Currency = "XYZ"
	if _, err := RenderXLSX(ExportDocument{Legacy: &badCurrency}); err == nil {
		t.Error("an unresolved currency produced a workbook")
	}
	badView := view
	badView.Version = EnvelopeVersion(2)
	if _, err := RenderXLSX(ExportDocument{View: &badView}); err == nil {
		t.Error("an unsupported view version produced a workbook")
	}
}

// An empty report is a valid report. It must still open: every sheet states the
// empty shape and no chart is anchored to an empty range.
func TestRenderXLSXRendersAnEmptyReportWithoutCharts(t *testing.T) {
	snapshot := unavailableProjectionReport()
	snapshot.Periods = nil
	snapshot.Categories = nil
	snapshot.CategorySeries = nil
	snapshot.Totals = poreport.Metrics{}
	snapshot.Notes = nil
	raw := renderXLSXSnapshot(t, snapshot)
	workbook := openWorkbook(t, raw)
	assertSheetList(t, workbook, []string{"Summary", "Periods", "Categories", "Forecast", "Notes"})
	if got := cellAt(rowWithLabel(t, workbook, "Periods", "Period"), 0); got != "Period" {
		t.Errorf("empty workbook has no period header: %q", got)
	}
	parts := workbookParts(t, raw)
	if charts := chartParts(parts); len(charts) != 0 {
		t.Errorf("empty report drew charts: %v", charts)
	}
	for name, content := range parts {
		if strings.HasPrefix(name, "xl/worksheets/") && strings.Contains(content, "<f>") {
			t.Errorf("worksheet %s carries a formula; the workbook is value-only", name)
		}
	}
}

// The period table stops where the PDF stops: a current-month/day report
// materializes future days as zero buckets, and those are forecast territory,
// never actual rows. Printing them would claim a zero actual for a day that has
// not happened.
func TestRenderXLSXPeriodTableStopsAtTheAsOfDay(t *testing.T) {
	raw := renderXLSXSnapshot(t, dayReport())
	workbook := openWorkbook(t, raw)

	rows, err := workbook.GetRows("Periods")
	if err != nil {
		t.Fatalf("read Periods: %v", err)
	}
	lastPeriod := ""
	for _, row := range rows {
		cell := cellAt(row, 0)
		if strings.HasPrefix(cell, "2026-11-") {
			lastPeriod = cell
		}
	}
	if lastPeriod != "2026-11-19" {
		t.Errorf("last period row = %q; want the as-of day 2026-11-19", lastPeriod)
	}
	for _, row := range rows {
		if cellAt(row, 0) == "2026-11-20" {
			t.Errorf("workbook printed a future day as an actual period row")
		}
	}
}

// The workbook is a real zip with the five named sheets and the notes
// the snapshot carries. A reader can find the notes by name, not by position.

func TestRenderXLSXWritesTheFiveSheetWorkbook(t *testing.T) {
	raw := renderXLSXSnapshot(t, sampleReport())
	parts := workbookParts(t, raw)
	for _, required := range []string{"xl/workbook.xml", "xl/worksheets/sheet1.xml"} {
		if _, ok := parts[required]; !ok {
			t.Fatalf("workbook has no part %s", required)
		}
	}

	workbook := openWorkbook(t, raw)
	assertSheetList(t, workbook,
		[]string{"Summary", "Periods", "Categories", "Forecast", "Notes"})

	rows, err := workbook.GetRows("Notes")
	if err != nil {
		t.Fatalf("read Notes: %v", err)
	}
	foundCode, foundText := false, false
	for _, row := range rows {
		if cellAt(row, 0) == "legacy_order_adjustment" {
			foundCode = true
			if strings.Contains(cellAt(row, 1), "retired whole-order discount") {
				foundText = true
			}
		}
	}
	if !foundCode || !foundText {
		t.Errorf("Notes rows = %v; want the legacy adjustment note and its text", rows)
	}
}

// isISODate reports whether text is a zero-padded YYYY-MM-DD date. Period rows
// are the only dated rows the workbook prints, so this is how a test tells a
// data row from a heading.
func isISODate(text string) bool {
	if len(text) != len("2006-01-02") {
		return false
	}
	_, err := time.Parse(time.DateOnly, text)
	return err == nil
}

// datedRows returns the first cell of every dated row on a sheet.
func datedRows(t *testing.T, workbook *excelize.File, sheet string) []string {
	t.Helper()
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		t.Fatalf("read %s rows: %v", sheet, err)
	}
	dates := make([]string, 0, len(rows))
	for _, row := range rows {
		if first := cellAt(row, 0); isISODate(first) {
			dates = append(dates, first)
		}
	}
	return dates
}

// Issue #683, F2. The Forecast sheet is projection territory: its chart plots
// the projected buckets only. A future day is never a zero actual, and one row
// never carries both a daily actual and a monthly forecast, because the sheet
// has no actual series at all. The Periods sheet carries the actual chart.
func TestRenderXLSXForecastChartPlotsOnlyTheProjectedBuckets(t *testing.T) {
	cases := []struct {
		name     string
		snapshot poreport.Report
	}{
		{name: "day report", snapshot: dayReport()},
		{name: "current year report", snapshot: currentYearReport()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			raw := renderXLSXSnapshot(t, test.snapshot)
			workbook := openWorkbook(t, raw)

			header := rowWithLabel(t, workbook, "Forecast", "Period")
			for _, cell := range header {
				if strings.EqualFold(strings.TrimSpace(cell), "Actual") {
					t.Errorf("Forecast header carries an Actual column: %v", header)
				}
			}

			dates := datedRows(t, workbook, "Forecast")
			want := len(test.snapshot.Projection.Monthly.Buckets)
			if len(dates) != want {
				t.Fatalf("Forecast dated rows = %v; want the %d projected buckets", dates, want)
			}
			buckets := make(map[string]struct{}, want)
			for _, bucket := range test.snapshot.Projection.Monthly.Buckets {
				buckets[bucket.PeriodStart] = struct{}{}
			}
			for _, date := range dates {
				if _, ok := buckets[date]; !ok {
					t.Errorf("Forecast plotted %q, which is not a projected bucket", date)
				}
				if !strings.HasSuffix(date, "-01") {
					t.Errorf("Forecast plotted %q on a month axis; want a month start", date)
				}
			}

			// The old actual-versus-forecast block lived in J:N with a blank
			// first cell. Nothing may sit past the bucket table any more.
			rows, err := workbook.GetRows("Forecast")
			if err != nil {
				t.Fatalf("read Forecast rows: %v", err)
			}
			for index, row := range rows {
				for column := 9; column < len(row); column++ {
					if strings.TrimSpace(row[column]) != "" {
						t.Errorf("Forecast row %d has a value in column %d (%q); the chart data block is gone",
							index+1, column+1, row[column])
					}
				}
				if cellAt(row, 0) == "" && len(row) > 1 && strings.TrimSpace(row[1]) != "" {
					t.Errorf("Forecast row %d carries a value with no period (%v)", index+1, row)
				}
			}

			parts := workbookParts(t, raw)
			forecastChart := ""
			for _, name := range chartParts(parts) {
				if strings.Contains(parts[name], "Forecast!") {
					forecastChart = name
				}
			}
			if forecastChart == "" {
				t.Fatalf("no chart references the Forecast sheet")
			}
			content := parts[forecastChart]
			if !strings.Contains(content, "Forecast amount") {
				t.Errorf("forecast chart is not titled as a projection: %s", forecastChart)
			}
			for _, column := range []string{"$B$", "$C$", "$D$"} {
				if !strings.Contains(content, "Forecast!"+column) {
					t.Errorf("forecast chart does not plot the %s bucket column", column)
				}
			}
			for _, column := range []string{"$J$", "$K$", "$L$", "$M$", "$N$"} {
				if strings.Contains(content, "Forecast!"+column) {
					t.Errorf("forecast chart plots %s, the retired actual-versus-forecast block", column)
				}
			}
			if strings.Contains(content, "Actual") {
				t.Errorf("forecast chart still names an actual series")
			}
			// An actual series that read the Periods sheet would reintroduce the
			// collision the old combo chart had: one row whose category is a
			// month start carrying both a daily actual and a monthly forecast.
			if strings.Contains(content, "Periods!") {
				t.Errorf("forecast chart plots actual rows from the Periods sheet")
			}
		})
	}
}

// Issue #683, F3. The workbook is the PDF's document: a view that selects one
// month prints one period row, not the overview's whole range.
func TestRenderXLSXViewPrintsTheSelectedMonths(t *testing.T) {
	view := validPastMonthView(t) // October 2026 only
	workbook := openWorkbook(t, renderXLSXView(t, view))

	got := datedRows(t, workbook, "Periods")
	if len(got) != 1 || got[0] != "2026-10-01" {
		t.Fatalf("Periods rows = %v; want the single selected month 2026-10-01", got)
	}
	october := rowNumberWithFirstCell(t, workbook, "Periods", "2026-10-01")
	assertNumber(t, "October PO count", numberAt(t, workbook, "Periods", october, 1), 2)
	if count := cellAt(rowWithFirstCellContaining(t, workbook, "Periods", "2026-10-01"), 1); count != "2" {
		t.Errorf("October PO count cell = %q, want 2", count)
	}
	for _, absent := range []string{"2026-01-01", "2026-06-01", "2026-11-01", "2026-12-01"} {
		for _, date := range got {
			if date == absent {
				t.Errorf("Periods printed %s, which the view did not select", date)
			}
		}
	}

	// The one-calendar-year aggregate reads its twelve rows from its own
	// annual source and labels them exactly as the PDF's aggregate table does.
	source := annualViewReport(2025, "2025-12-20", nil)
	aggregate := annualAggregateView(source)
	aggregateBook := openWorkbook(t, renderXLSXView(t, aggregate))
	labels := periodLabels(t, aggregateBook)
	if len(labels) != 12 || labels[0] != "Jan" || labels[11] != "Dec (recorded)" {
		t.Errorf("aggregate Periods labels = %v; want Jan..Dec (recorded)", labels)
	}
}

// periodLabels returns column A of the Periods sheet's month rows: everything
// below the header up to, but not including, the total row.
func periodLabels(t *testing.T, workbook *excelize.File) []string {
	t.Helper()
	rows, err := workbook.GetRows("Periods")
	if err != nil {
		t.Fatalf("read Periods rows: %v", err)
	}
	labels := make([]string, 0, len(rows))
	started := false
	for _, row := range rows {
		label := cellAt(row, 0)
		if label == "Period" {
			started = true
			continue
		}
		if !started || label == "" {
			continue
		}
		if strings.Contains(strings.ToLower(label), "total") {
			break
		}
		labels = append(labels, label)
	}
	return labels
}

// Issue #683, F3. A category selection prints exactly those rows in the
// source's supplied order, and a subset carries no total that would read as the
// subset's own.
func TestRenderXLSXViewPrintsOnlyTheSelectedCategories(t *testing.T) {
	codes := []string{poreport.CategorySupplyGeneral, poreport.CategoryOthers}
	view := categoryComparisonView(t, ViewLayoutIndividual, codes, false)
	workbook := openWorkbook(t, renderXLSXView(t, view))

	rows, err := workbook.GetRows("Categories")
	if err != nil {
		t.Fatalf("read Categories rows: %v", err)
	}
	var printed []string
	inTable := false
	for _, row := range rows {
		code := cellAt(row, 0)
		if code == "Code" {
			inTable = true
			continue
		}
		if !inTable || code == "" {
			continue
		}
		if strings.Contains(strings.ToLower(code), "total") {
			t.Errorf("a selected subset printed a total row %q", code)
			continue
		}
		printed = append(printed, code)
	}
	if len(printed) != len(codes) {
		t.Fatalf("Categories rows = %v; want %v", printed, codes)
	}
	for index := range codes {
		if printed[index] != codes[index] {
			t.Fatalf("Categories rows = %v; want the source order %v", printed, codes)
		}
	}
	for _, absent := range []string{poreport.CategoryFm, poreport.CategoryFab, poreport.CategoryUnassigned} {
		for _, code := range printed {
			if code == absent {
				t.Errorf("Categories printed %s, which the view did not select", code)
			}
		}
	}

	// The aggregate layout is the whole supplied row set, so it keeps the
	// source's own server total under an honest label.
	all := viewCodes(categoryComparisonSource(t))
	aggregate := categoryComparisonView(t, ViewLayoutAggregate, all, false)
	aggregateBook := openWorkbook(t, renderXLSXView(t, aggregate))
	totalRows, err := aggregateBook.GetRows("Categories")
	if err != nil {
		t.Fatalf("read aggregate Categories: %v", err)
	}
	found := false
	for _, row := range totalRows {
		if cellAt(row, 0) == "Source total" {
			found = true
			assertNumber(t, "aggregate source total", numberAt(t, aggregateBook, "Categories",
				rowNumberWithLabel(t, aggregateBook, "Categories", "Source total"), 3), 1_100_000_000)
		}
	}
	if !found {
		t.Errorf("aggregate category workbook has no source total row")
	}
}

// Issue #683, F3. An explicit empty selection prints an empty section. It must
// not fall back to the overview's full months or categories, because the PDF
// prints none of them either.
func TestRenderXLSXViewEmptySelectionPrintsTheEmptySections(t *testing.T) {
	overview := currentYearReport()
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{overview},
		Overview: 0,
		Periods: ViewPeriods{
			Sources: []int{}, Months: []string{}, Layout: ViewLayoutIndividual,
		},
		Categories: ViewCategories{Source: 0, Codes: []string{}, Layout: ViewLayoutIndividual},
	}
	workbook := openWorkbook(t, renderXLSXView(t, view))

	if rows := datedRows(t, workbook, "Periods"); len(rows) != 0 {
		t.Errorf("an empty month selection printed %v", rows)
	}
	periodRows, err := workbook.GetRows("Periods")
	if err != nil {
		t.Fatalf("read Periods: %v", err)
	}
	if !sheetSays(periodRows, "No months selected") {
		t.Errorf("Periods does not state the empty month selection: %v", periodRows)
	}

	categoryRows, err := workbook.GetRows("Categories")
	if err != nil {
		t.Fatalf("read Categories: %v", err)
	}
	if !sheetSays(categoryRows, "No categories selected") {
		t.Errorf("Categories does not state the empty category selection: %v", categoryRows)
	}
	for _, row := range categoryRows {
		switch cellAt(row, 0) {
		case poreport.CategoryFm, poreport.CategoryFab, poreport.CategorySupplyGeneral,
			poreport.CategoryOthers, poreport.CategoryUnassigned, "Report total", "Source total":
			t.Errorf("an empty category selection printed %q", cellAt(row, 0))
		}
	}
}

// sheetSays reports whether any cell on the sheet contains text.
func sheetSays(rows [][]string, text string) bool {
	for _, row := range rows {
		for _, cell := range row {
			if strings.Contains(cell, text) {
				return true
			}
		}
	}
	return false
}

// Issue #683, F3. The current month's row is the server's actual-to-date
// readout, not the annual month row that may also carry future-dated records;
// a future month with no recorded purchase order has no actual row at all.
func TestRenderXLSXViewCurrentAndFutureMonthsFollowThePdfEvidence(t *testing.T) {
	overview := yearMonthFixture(t)
	current := individualMonthsView(t, []string{"2026-11"})
	workbook := openWorkbook(t, renderXLSXView(t, current))
	rows := datedRows(t, workbook, "Periods")
	if len(rows) != 1 || rows[0] != "2026-11-01" {
		t.Fatalf("Periods rows = %v; want the selected current month only", rows)
	}
	row := rowNumberWithFirstCell(t, workbook, "Periods", "2026-11-01")
	if status := cellAt(rowWithFirstCellContaining(t, workbook, "Periods", "2026-11-01"), 9); status != "Month to date" {
		t.Errorf("current month status = %q; want Month to date", status)
	}
	actual := overview.MonthSummary.Actual
	assertNumber(t, "current month amount", numberAt(t, workbook, "Periods", row, 2), float64(actual.AmountMinor))
	assertNumber(t, "current month PO count", numberAt(t, workbook, "Periods", row, 1), float64(actual.PoCount))

	// December 2026 is a future month with a zero annual row and no recorded
	// purchase order: the PDF prints a dashed card, the workbook prints no
	// actual row and says so, rather than a zero.
	future := individualMonthsView(t, []string{"2026-12"})
	futureBook := openWorkbook(t, renderXLSXView(t, future))
	if rows := datedRows(t, futureBook, "Periods"); len(rows) != 0 {
		t.Errorf("a future month with no recorded purchase order printed %v", rows)
	}
	periodRows, err := futureBook.GetRows("Periods")
	if err != nil {
		t.Fatalf("read Periods: %v", err)
	}
	if !sheetSays(periodRows, "No recorded purchase orders in the selected months") {
		t.Errorf("the future month's empty section is not stated: %v", periodRows)
	}
	if !sheetSays(periodRows, "left out rather than printed as a zero actual") {
		t.Errorf("the workbook does not say the future month was left out: %v", periodRows)
	}
	for _, row := range periodRows {
		if cellAt(row, 0) == "Overview total" {
			t.Errorf("an empty future selection printed an overview total")
		}
	}
}

// Issue #683, aggregate blocker. The PDF's aggregate table prints all twelve
// months of the year, and a month with no recorded purchase order keeps its own
// labelled row: the label says "no recorded POs" and no figure is printed. The
// workbook must print that same table, not drop the month.
func TestRenderXLSXAggregatePeriodsPrintEveryMonthLikeThePdf(t *testing.T) {
	source := currentYearReport() // as of 2026-11-19; December has no POs
	workbook := openWorkbook(t, renderXLSXView(t, annualAggregateView(source)))

	want := []string{
		"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov (recorded)", "Dec (no recorded POs)",
	}
	labels := periodLabels(t, workbook)
	if len(labels) != len(want) {
		t.Fatalf("aggregate Periods labels = %v; want the twelve months %v", labels, want)
	}
	for index := range want {
		if labels[index] != want[index] {
			t.Errorf("aggregate label %d = %q, want %q", index, labels[index], want[index])
		}
	}

	// December is printed, and every figure cell on it is empty: no amount, no
	// count and no zero standing in for evidence.
	december := rowNumberWithFirstCell(t, workbook, "Periods", "Dec (no recorded POs)")
	for column := 1; column <= 8; column++ {
		if got := rawCell(t, workbook, "Periods", december, column); got != "" {
			t.Errorf("December column %d = %q; want no figure", column+1, got)
		}
	}

	// A recorded month prints the annual source's own row.
	november := rowNumberWithFirstCell(t, workbook, "Periods", "Nov (recorded)")
	assertNumber(t, "November PO count", numberAt(t, workbook, "Periods", november, 1), 2)
	assertNumber(t, "November amount", numberAt(t, workbook, "Periods", november, 2), 100_000_000)
}

// Issue #683, aggregate blocker. The aggregate's one total is the annual
// source's own server total under the PDF's "Recorded total" label, even when
// the view carries several sources and the overview is a different one.
func TestRenderXLSXAggregateTotalIsTheAggregateSourceTotal(t *testing.T) {
	aggregateSource := currentYearReport() // 22 POs, Rp1,100,000,000
	overview := annualViewReport(2025, "2026-11-19", nil)
	view := ViewEnvelope{
		Version:  ViewVersion,
		Sources:  []poreport.Report{aggregateSource, overview},
		Overview: 1,
		Periods: ViewPeriods{
			Sources: []int{0}, Months: monthsOfYear(2026), Layout: ViewLayoutAggregate,
		},
		Categories: ViewCategories{Source: 1, Codes: viewCodes(overview), Layout: ViewLayoutAggregate},
	}
	workbook := openWorkbook(t, renderXLSXView(t, view))

	// The overview's own totals are 2 POs and Rp100,000,000, so a total taken
	// from the overview instead of the aggregate source cannot pass this.
	assertNumber(t, "summary PO count", labelNumber(t, workbook, "PO count"), 2)

	total := rowNumberWithLabel(t, workbook, "Periods", "Recorded total")
	if got := cellAt(rowWithLabel(t, workbook, "Periods", "Recorded total"), 0); got != "Recorded total" {
		t.Errorf("aggregate total label = %q; want Recorded total", got)
	}
	assertNumber(t, "aggregate total PO count", numberAt(t, workbook, "Periods", total, 1), 22)
	assertNumber(t, "aggregate total amount", numberAt(t, workbook, "Periods", total, 2), 1_100_000_000)
}

// Issue #683, aggregate blocker. The current month in an aggregate table is the
// annual source's recorded row, exactly as the PDF prints it. It is not the
// month-to-date readout an individual selection prints.
func TestRenderXLSXAggregateCurrentMonthIsRecordedNotMonthToDate(t *testing.T) {
	source := currentYearReport()
	workbook := openWorkbook(t, renderXLSXView(t, annualAggregateView(source)))

	if labels := periodLabels(t, workbook); !containsString(labels, "Nov (recorded)") {
		t.Errorf("aggregate labels = %v; want the current month labelled Nov (recorded)", labels)
	}
	rows, err := workbook.GetRows("Periods")
	if err != nil {
		t.Fatalf("read Periods rows: %v", err)
	}
	for index, row := range rows {
		for _, cell := range row {
			if strings.TrimSpace(cell) == "Month to date" {
				t.Errorf("Periods row %d prints %q; an aggregate month is a recorded row", index+1, cell)
			}
		}
	}
	november := rowNumberWithFirstCell(t, workbook, "Periods", "Nov (recorded)")
	assertNumber(t, "November amount", numberAt(t, workbook, "Periods", november, 2), 100_000_000)
	if got := cellAt(rowWithFirstCellContaining(t, workbook, "Periods", "Nov (recorded)"), 9); got != "" {
		t.Errorf("aggregate current-month status = %q; want no status override", got)
	}
}
