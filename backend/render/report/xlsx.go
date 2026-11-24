package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"vera-app/backend/currency"
	"vera-app/backend/poreport"
)

// Issue #683: the Excel export of one purchase-order report document.
//
// RenderXLSX accepts exactly the two export payloads the PDF accepts and prints
// the same document: the overview owns the Summary, Forecast and Notes sheets,
// while the Periods and Categories sheets follow the view's own selected months
// and categories exactly as the PDF's selected sections do. It makes no network
// call, reads no clock and computes no report figure: every number in the
// workbook is a frozen server value. The workbook carries no formulas at all,
// because a spreadsheet that recalculated a server figure could disagree with
// the signed report it was exported from.

// The workbook's fixed sheet vocabulary. The order is the reading order: the
// answer first, then the evidence, then the disclosures.
const (
	xlsxSummarySheet    = "Summary"
	xlsxPeriodsSheet    = "Periods"
	xlsxCategoriesSheet = "Categories"
	xlsxForecastSheet   = "Forecast"
	xlsxNotesSheet      = "Notes"
)

// RenderXLSX renders the export document as an .xlsx workbook.
func RenderXLSX(document ExportDocument) ([]byte, error) {
	var (
		overview poreport.Report
		resolved *resolvedView
	)
	switch {
	case document.Legacy != nil:
		if err := ValidateSnapshot(*document.Legacy); err != nil {
			return nil, err
		}
		overview = *document.Legacy
	case document.View != nil:
		if err := ValidateView(*document.View); err != nil {
			return nil, err
		}
		// The same resolver the PDF prints through: the Periods and Categories
		// sheets must show the view's selection, not the overview's full range.
		view, err := resolveView(*document.View)
		if err != nil {
			return nil, err
		}
		resolved = &view
		overview = view.Overview
	default:
		return nil, fmt.Errorf("purchase order report export has no document")
	}

	book, err := newReportWorkbook(overview)
	if err != nil {
		return nil, err
	}
	if err := book.render(overview, resolved); err != nil {
		return nil, err
	}
	raw, err := book.file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write purchase order report workbook: %w", err)
	}
	return raw.Bytes(), nil
}

type workbookStyles struct {
	title    int
	subtitle int
	section  int
	header   int
	label    int
	strong   int
	text     int
	wrapped  int
	money    int
	percent  int
	count    int
	total    int
}

type reportWorkbook struct {
	file       *excelize.File
	info       currency.Info
	styles     workbookStyles
	moneyFmt   string
	percentFmt string
	countFmt   string
}

func newReportWorkbook(snapshot poreport.Report) (*reportWorkbook, error) {
	info, ok := currency.Lookup(snapshot.Filter.Currency)
	if !ok {
		return nil, fmt.Errorf("purchase order report workbook: currency %q is not in the supported catalogue",
			snapshot.Filter.Currency)
	}
	file := excelize.NewFile()
	book := &reportWorkbook{
		file:       file,
		info:       info,
		moneyFmt:   moneyNumberFormat(info),
		percentFmt: "0.00%;(0.00%);\"-\"",
		countFmt:   "#,##0;(#,##0);\"-\"",
	}
	if err := file.SetSheetName("Sheet1", xlsxSummarySheet); err != nil {
		return nil, fmt.Errorf("name the summary sheet: %w", err)
	}
	for _, name := range []string{xlsxPeriodsSheet, xlsxCategoriesSheet, xlsxForecastSheet, xlsxNotesSheet} {
		if _, err := file.NewSheet(name); err != nil {
			return nil, fmt.Errorf("create the %s sheet: %w", name, err)
		}
	}
	styles, err := book.newStyles()
	if err != nil {
		return nil, err
	}
	book.styles = styles
	if err := book.setWidths(); err != nil {
		return nil, err
	}
	return book, nil
}

// moneyNumberFormat is the workbook's currency format: the currency's own
// symbol and ISO exponent, grouping, parentheses for a loss and a dash for
// zero. The stored format code uses the invariant "," and "." placeholders;
// Excel localizes the displayed separators itself.
func moneyNumberFormat(info currency.Info) string {
	digits := "#,##0"
	if info.Exponent > 0 {
		digits += "." + strings.Repeat("0", info.Exponent)
	}
	prefix, suffix := currency.Affixes(info.Code)
	quote := func(value string) string {
		if value == "" {
			return ""
		}
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	positive := quote(prefix) + digits + quote(suffix)
	return positive + ";(" + positive + ");\"-\""
}

func (b *reportWorkbook) newStyles() (workbookStyles, error) {
	style := func(font *excelize.Font, numFmt string, fill *excelize.Fill, border []excelize.Border, alignment *excelize.Alignment) (int, error) {
		value := &excelize.Style{Font: font, Border: border, Alignment: alignment}
		if numFmt != "" {
			value.CustomNumFmt = &numFmt
		}
		if fill != nil {
			value.Fill = *fill
		}
		return b.file.NewStyle(value)
	}
	base := func(bold bool, size float64, color string) *excelize.Font {
		return &excelize.Font{Family: "Arial", Size: size, Bold: bold, Color: color}
	}
	sectionFill := &excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F2937"}}
	headerBorder := []excelize.Border{{Type: "bottom", Color: "9CA3AF", Style: 1}}

	var styles workbookStyles
	var err error
	if styles.title, err = style(base(true, 14, "111827"), "", nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.subtitle, err = style(&excelize.Font{Family: "Arial", Size: 9, Italic: true, Color: "6B7280"}, "", nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.section, err = style(base(true, 10, "FFFFFF"), "", sectionFill, nil, nil); err != nil {
		return styles, err
	}
	if styles.header, err = style(base(true, 10, "111827"), "", &excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"F3F4F6"}}, headerBorder, nil); err != nil {
		return styles, err
	}
	if styles.label, err = style(base(false, 10, "374151"), "", nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.strong, err = style(base(true, 10, "111827"), "", nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.text, err = style(base(false, 10, "111827"), "", nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.wrapped, err = style(base(false, 10, "111827"), "", nil, nil, &excelize.Alignment{WrapText: true, Vertical: "top"}); err != nil {
		return styles, err
	}
	if styles.money, err = style(base(false, 10, "111827"), b.moneyFmt, nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.percent, err = style(base(false, 10, "111827"), b.percentFmt, nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.count, err = style(base(false, 10, "111827"), b.countFmt, nil, nil, nil); err != nil {
		return styles, err
	}
	if styles.total, err = style(base(true, 10, "111827"), b.moneyFmt, nil, headerBorder, nil); err != nil {
		return styles, err
	}
	return styles, nil
}

func (b *reportWorkbook) setWidths() error {
	widths := map[string]struct {
		start, end string
		width      float64
	}{
		xlsxSummarySheet:    {"A", "A", 38},
		xlsxPeriodsSheet:    {"A", "A", 14},
		xlsxCategoriesSheet: {"A", "A", 12},
		xlsxForecastSheet:   {"A", "A", 14},
		xlsxNotesSheet:      {"A", "A", 22},
	}
	for sheet, width := range widths {
		if err := b.file.SetColWidth(sheet, width.start, width.end, width.width); err != nil {
			return fmt.Errorf("size the %s sheet: %w", sheet, err)
		}
	}
	// The value column and the worksheet's evidence columns are wider than a
	// label column but not so wide that a figure is lost in whitespace.
	if err := b.file.SetColWidth(xlsxSummarySheet, "B", "B", 46); err != nil {
		return fmt.Errorf("size the summary values: %w", err)
	}
	if err := b.file.SetColWidth(xlsxPeriodsSheet, "B", "J", 14); err != nil {
		return fmt.Errorf("size the period columns: %w", err)
	}
	if err := b.file.SetColWidth(xlsxCategoriesSheet, "B", "B", 28); err != nil {
		return fmt.Errorf("size the category label column: %w", err)
	}
	if err := b.file.SetColWidth(xlsxCategoriesSheet, "C", "J", 14); err != nil {
		return fmt.Errorf("size the category columns: %w", err)
	}
	if err := b.file.SetColWidth(xlsxForecastSheet, "B", "G", 15); err != nil {
		return fmt.Errorf("size the forecast columns: %w", err)
	}
	if err := b.file.SetColWidth(xlsxNotesSheet, "B", "B", 110); err != nil {
		return fmt.Errorf("size the notes column: %w", err)
	}
	return nil
}

func (b *reportWorkbook) render(overview poreport.Report, resolved *resolvedView) error {
	if err := b.renderSummary(overview); err != nil {
		return err
	}
	if err := b.renderPeriods(newPeriodSection(overview, resolved)); err != nil {
		return err
	}
	if err := b.renderCategories(newCategorySection(overview, resolved)); err != nil {
		return err
	}
	if err := b.renderForecast(overview); err != nil {
		return err
	}
	if err := b.renderNotes(overview); err != nil {
		return err
	}
	summary, err := b.file.GetSheetIndex(xlsxSummarySheet)
	if err != nil {
		return fmt.Errorf("read the summary sheet index: %w", err)
	}
	b.file.SetActiveSheet(summary)
	return nil
}

func (b *reportWorkbook) set(sheet, cell string, value any, style int) error {
	if err := b.file.SetCellValue(sheet, cell, value); err != nil {
		return fmt.Errorf("write %s!%s: %w", sheet, cell, err)
	}
	if err := b.file.SetCellStyle(sheet, cell, cell, style); err != nil {
		return fmt.Errorf("style %s!%s: %w", sheet, cell, err)
	}
	return nil
}

func (b *reportWorkbook) renderSummary(snapshot poreport.Report) error {
	const sheet = xlsxSummarySheet
	if err := b.set(sheet, "A1", "Purchase Order Performance Report", b.styles.title); err != nil {
		return err
	}
	if err := b.set(sheet, "A2",
		"Internal report: not a customer quotation. PPN is excluded from every figure.", b.styles.subtitle); err != nil {
		return err
	}

	row := 4
	if err := b.labelValue(sheet, row, "Company", snapshot.Company.Name, b.styles.strong); err != nil {
		return err
	}
	if err := b.labelValue(sheet, row+1, "Address", snapshot.Company.Address, b.styles.text); err != nil {
		return err
	}

	row = 7
	if err := b.sectionRow(sheet, row, "Report", "B"); err != nil {
		return err
	}
	reportRows := []struct{ label, value string }{
		{"Period", snapshot.Filter.From + " → " + snapshot.Filter.To + " (exclusive)"},
		{"Bucket", bucketLabel(snapshot.Filter.Bucket)},
		{"Category", categoryFilterLabel(snapshot.Filter.Category)},
		{"Currency", snapshot.Filter.Currency},
		{"Generated", generatedLabel(snapshot.GeneratedAt)},
		{"As of", snapshot.AsOf},
	}
	for index, item := range reportRows {
		if err := b.labelValue(sheet, row+1+index, item.label, item.value, b.styles.text); err != nil {
			return err
		}
	}

	row += len(reportRows) + 2
	if err := b.sectionRow(sheet, row, "Totals", "B"); err != nil {
		return err
	}
	totals := snapshot.Totals
	if err := b.countRow(sheet, row+1, "PO count", totals.PoCount); err != nil {
		return err
	}
	if err := b.moneyRow(sheet, row+2, "PO amount", totals.AmountMinor); err != nil {
		return err
	}
	if err := b.moneyRow(sheet, row+3, "Costed amount", totals.CostedMinor); err != nil {
		return err
	}
	if err := b.moneyRow(sheet, row+4, "Capital", totals.CapitalMinor); err != nil {
		return err
	}
	if err := b.moneyRow(sheet, row+5, "PO margin", totals.MarginMinor); err != nil {
		return err
	}
	if err := b.percentRow(sheet, row+6, "PO margin %", totals.MarginBps); err != nil {
		return err
	}
	if err := b.percentRow(sheet, row+7, "Coverage %", totals.CoverageBps); err != nil {
		return err
	}
	if err := b.moneyRow(sheet, row+8, "Uncosted", totals.UncostedMinor); err != nil {
		return err
	}
	if err := b.moneyRow(sheet, row+9, "Extra expense", totals.ExtraExpenseMinor); err != nil {
		return err
	}
	row += 10
	if totals.ExtraExpenseMinor != 0 {
		if err := b.labelValue(sheet, row, "Coverage note",
			"Coverage is costed over the non-extra amount; extra expense is pass-through with no cost and no margin.",
			b.styles.wrapped); err != nil {
			return err
		}
		row++
	}

	row += 1
	if err := b.sectionRow(sheet, row, "Pipeline", "B"); err != nil {
		return err
	}
	pipeline := snapshot.Pipeline
	if err := b.moneyRow(sheet, row+1, "Unsold amount", pipeline.UnsoldAmountMinor); err != nil {
		return err
	}
	if err := b.moneyRow(sheet, row+2, "Unsold margin", pipeline.UnsoldMarginMinor); err != nil {
		return err
	}
	if err := b.percentBpsRow(sheet, row+3, "Fulfillment %", pipeline.FulfillmentBps); err != nil {
		return err
	}
	row += 4

	row += 1
	if err := b.sectionRow(sheet, row, "Variance", "B"); err != nil {
		return err
	}
	variance := snapshot.Variance
	varianceRows := []struct {
		label string
		minor int64
	}{
		{"Quantity extra amount", variance.QuantityExtraIncomeAmountMinor},
		{"Quantity extra margin", variance.QuantityExtraIncomeMarginMinor},
		{"Price extra income", variance.PriceExtraIncomeMinor},
		{"Price loss", variance.PriceLossMinor},
		{"Closed loss amount", variance.ClosedLossAmountMinor},
		{"Closed loss margin", variance.ClosedLossMarginMinor},
		{"Net realized amount", variance.NetRealizedVarianceAmountMinor},
		{"Net realized margin", variance.NetRealizedVarianceMarginMinor},
	}
	for index, item := range varianceRows {
		if err := b.moneyRow(sheet, row+1+index, item.label, item.minor); err != nil {
			return err
		}
	}
	row += len(varianceRows) + 1

	row += 1
	if err := b.sectionRow(sheet, row, "Population", "B"); err != nil {
		return err
	}
	currencies := strings.Join(snapshot.Population.AvailableCurrencies, ", ")
	if currencies == "" {
		currencies = "-"
	}
	hasUnassigned := "No"
	if snapshot.Population.HasUnassigned {
		hasUnassigned = "Yes"
	}
	populationRows := []struct {
		label string
		value any
		style int
	}{
		{"Available currencies", currencies, b.styles.text},
		{"Has unassigned", hasUnassigned, b.styles.text},
		{"Source-missing POs", snapshot.Population.SourceMissingPoCount, b.styles.count},
		{"Unallocated POs", snapshot.Population.UnallocatedPoCount, b.styles.count},
		{"Legacy fallback POs", snapshot.Population.LegacyFallbackPoCount, b.styles.count},
		{"Expired unclosed quotations", snapshot.Population.ExpiredUnclosedQuotationCount, b.styles.count},
	}
	for index, item := range populationRows {
		if err := b.labelValue(sheet, row+1+index, item.label, item.value, item.style); err != nil {
			return err
		}
	}
	return nil
}

// sectionRow is the dark band that separates the Summary's blocks. Merging the
// value column into it keeps the band one continuous rule.
func (b *reportWorkbook) sectionRow(sheet string, row int, title, endColumn string) error {
	cell := fmt.Sprintf("A%d", row)
	if err := b.set(sheet, cell, title, b.styles.section); err != nil {
		return err
	}
	if err := b.file.MergeCell(sheet, cell, fmt.Sprintf("%s%d", endColumn, row)); err != nil {
		return fmt.Errorf("merge the %s section header: %w", title, err)
	}
	if err := b.file.SetRowHeight(sheet, row, 20); err != nil {
		return fmt.Errorf("size the %s section header: %w", title, err)
	}
	return nil
}

func (b *reportWorkbook) labelValue(sheet string, row int, label string, value any, style int) error {
	if err := b.set(sheet, fmt.Sprintf("A%d", row), label, b.styles.label); err != nil {
		return err
	}
	return b.set(sheet, fmt.Sprintf("B%d", row), value, style)
}

func (b *reportWorkbook) moneyRow(sheet string, row int, label string, minor int64) error {
	return b.labelValue(sheet, row, label, b.major(minor), b.styles.money)
}

func (b *reportWorkbook) countRow(sheet string, row int, label string, count int64) error {
	return b.labelValue(sheet, row, label, count, b.styles.count)
}

func (b *reportWorkbook) percentRow(sheet string, row int, label string, bps *int64) error {
	if bps == nil {
		return b.labelValue(sheet, row, label, "-", b.styles.text)
	}
	return b.labelValue(sheet, row, label, float64(*bps)/10000, b.styles.percent)
}

func (b *reportWorkbook) percentBpsRow(sheet string, row int, label string, bps int64) error {
	return b.labelValue(sheet, row, label, float64(bps)/10000, b.styles.percent)
}

// major converts stored minor units to the major-unit number a spreadsheet
// stores, using the currency's ISO exponent. Every figure in one workbook is in
// the same currency, so nothing is converted between currencies.
func (b *reportWorkbook) major(minor int64) float64 {
	if b.info.Exponent == 0 {
		return float64(minor)
	}
	divisor := 1.0
	for index := 0; index < b.info.Exponent; index++ {
		divisor *= 10
	}
	return float64(minor) / divisor
}

// periodSection is the Periods sheet's row set and the total that belongs to
// it. A legacy snapshot prints its own actual rows and total; a versioned view
// prints the selected months and keeps the overview's total, labelled for what
// it is. A section with no rows prints its empty message and no total, exactly
// as the PDF's empty selection does.
type periodSection struct {
	rows       []poreport.Period
	total      *poreport.Metrics
	totalLabel string
	note       string
	empty      string
	// status overrides the Status cell for a period start. A view names the
	// current month "Month to date" and a recorded future month "Recorded
	// future"; a legacy row keeps its own partial/in-progress flags.
	status map[string]string
}

// newPeriodSection builds the Periods sheet's rows from the document: the
// snapshot's own rows for a legacy payload, the resolved selected months for a
// view.
func newPeriodSection(overview poreport.Report, resolved *resolvedView) periodSection {
	if resolved == nil {
		note := "Actual purchase orders in the report range. PPN is excluded; an uncosted amount is never counted as profit."
		if _, limited := periodTableLimit(overview); limited {
			note += " The table stops at the period containing the as-of date; later buckets are forecast territory."
		}
		totals := overview.Totals
		return periodSection{
			rows:       xlsxVisiblePeriods(overview),
			total:      &totals,
			totalLabel: "Report total",
			note:       note,
			empty:      "No purchase orders in the selected range.",
		}
	}

	rows, status, omitted := xlsxSelectedPeriods(*resolved)
	note := "Selected months only, each read from its frozen source. PPN is excluded; an uncosted amount is never counted as profit."
	if omitted > 0 {
		note += " A selected future month with no recorded purchase order is left out rather than printed as a zero actual; its projection is on the Forecast sheet."
	}
	section := periodSection{
		rows:   rows,
		note:   note,
		status: status,
		empty:  "No recorded purchase orders in the selected months.",
	}
	if len(resolved.View.Periods.Months) == 0 {
		section.empty = "No months selected. Choose at least one month to print a period comparison; no figure and no total are shown here."
		return section
	}
	if len(rows) == 0 {
		return section
	}
	totals := resolved.Overview.Totals
	section.total = &totals
	section.totalLabel = "Overview total"
	section.note += " The total is the overview report's own total, not a sum of the selected months."
	return section
}

// xlsxSelectedPeriods reads the resolved view's month rows. It mirrors the
// PDF's monthPrimary: a current month prints its server actual-to-date readout,
// a past month or a future month with recorded purchase orders prints its
// supplied row, and a future month with no recorded purchase order has no
// actual row at all. A month without evidence is counted in omitted, never
// printed as a zero.
func xlsxSelectedPeriods(resolved resolvedView) ([]poreport.Period, map[string]string, int) {
	status := make(map[string]string)
	if resolved.Aggregate != nil {
		rows := make([]poreport.Period, 0, len(resolved.Aggregate.Months))
		omitted := 0
		for _, month := range resolved.Aggregate.Months {
			if !month.Recorded || month.Row == nil {
				omitted++
				continue
			}
			period := *month.Row
			period.PeriodStart = month.Month + "-01"
			switch {
			case month.IsCurrent:
				status[period.PeriodStart] = "Month to date"
				period.InProgress = true
				period.Partial = false
			case month.IsFuture:
				status[period.PeriodStart] = "Recorded future"
				period.InProgress = false
				period.Partial = false
			}
			rows = append(rows, period)
		}
		return rows, status, omitted
	}

	rows := make([]poreport.Period, 0, len(resolved.Months))
	omitted := 0
	for _, month := range resolved.Months {
		period, label, ok := xlsxSelectedMonthPeriod(month)
		if !ok {
			omitted++
			continue
		}
		if label != "" {
			status[period.PeriodStart] = label
		}
		rows = append(rows, period)
	}
	return rows, status, omitted
}

// xlsxSelectedMonthPeriod is one resolved month as an actual row. A dashed
// card in the PDF has no row here: the absence of evidence is not a zero.
func xlsxSelectedMonthPeriod(month resolvedMonth) (poreport.Period, string, bool) {
	period := poreport.Period{PeriodStart: month.Month + "-01"}
	switch {
	case month.IsCurrent:
		if month.Summary == nil {
			return poreport.Period{}, "", false
		}
		period.Metrics = month.Summary.Actual
		return period, "Month to date", true
	case month.IsFuture:
		if month.Period == nil || month.Period.PoCount == 0 {
			return poreport.Period{}, "", false
		}
		period.Metrics = month.Period.Metrics
		return period, "Recorded future", true
	default:
		if month.Period == nil {
			return poreport.Period{}, "", false
		}
		period.Metrics = month.Period.Metrics
		period.Partial = month.Period.Partial
		period.InProgress = month.Period.InProgress
		return period, "", true
	}
}

func (b *reportWorkbook) renderPeriods(section periodSection) error {
	const sheet = xlsxPeriodsSheet
	if err := b.set(sheet, "A1", "Periods", b.styles.title); err != nil {
		return err
	}
	if err := b.set(sheet, "A2", section.note, b.styles.subtitle); err != nil {
		return err
	}

	headerRow := 4
	headers := []string{
		"Period", "PO count", "Amount", "Costed", "Capital", "Margin", "Margin %", "Coverage %", "Uncosted", "Status",
	}
	for index, header := range headers {
		if err := b.set(sheet, fmt.Sprintf("%s%d", columnLetter(index), headerRow), header, b.styles.header); err != nil {
			return err
		}
	}

	if len(section.rows) == 0 {
		return b.set(sheet, fmt.Sprintf("A%d", headerRow+1), section.empty, b.styles.subtitle)
	}
	row := headerRow + 1
	for _, period := range section.rows {
		if err := b.set(sheet, fmt.Sprintf("A%d", row), period.PeriodStart, b.styles.text); err != nil {
			return err
		}
		if err := b.set(sheet, fmt.Sprintf("B%d", row), period.PoCount, b.styles.count); err != nil {
			return err
		}
		for index, minor := range []int64{
			period.AmountMinor, period.CostedMinor, period.CapitalMinor, period.MarginMinor,
		} {
			column := columnLetter(2 + index)
			if err := b.set(sheet, fmt.Sprintf("%s%d", column, row), b.major(minor), b.styles.money); err != nil {
				return err
			}
		}
		if err := b.setPercentCell(sheet, fmt.Sprintf("G%d", row), period.MarginBps); err != nil {
			return err
		}
		if err := b.setPercentCell(sheet, fmt.Sprintf("H%d", row), period.CoverageBps); err != nil {
			return err
		}
		if err := b.set(sheet, fmt.Sprintf("I%d", row), b.major(period.UncostedMinor), b.styles.money); err != nil {
			return err
		}
		status := section.status[period.PeriodStart]
		if status == "" {
			switch {
			case period.InProgress:
				status = "In progress"
			case period.Partial:
				status = "Partial"
			}
		}
		if err := b.set(sheet, fmt.Sprintf("J%d", row), status, b.styles.text); err != nil {
			return err
		}
		row++
	}

	// An empty section and a selection with no recorded evidence carry no
	// total: a total with no rows beside it would be an invented answer.
	if section.total == nil {
		return b.addPeriodCharts(sheet, headerRow+1, row-1)
	}
	total := *section.total
	totalRow := row
	if err := b.set(sheet, fmt.Sprintf("A%d", totalRow), section.totalLabel, b.styles.strong); err != nil {
		return err
	}
	if err := b.set(sheet, fmt.Sprintf("B%d", totalRow), total.PoCount, b.styles.count); err != nil {
		return err
	}
	for index, minor := range []int64{
		total.AmountMinor, total.CostedMinor, total.CapitalMinor, total.MarginMinor,
	} {
		column := columnLetter(2 + index)
		if err := b.set(sheet, fmt.Sprintf("%s%d", column, totalRow), b.major(minor), b.styles.total); err != nil {
			return err
		}
	}
	if err := b.setPercentCell(sheet, fmt.Sprintf("G%d", totalRow), total.MarginBps); err != nil {
		return err
	}
	if err := b.setPercentCell(sheet, fmt.Sprintf("H%d", totalRow), total.CoverageBps); err != nil {
		return err
	}
	if err := b.set(sheet, fmt.Sprintf("I%d", totalRow), b.major(total.UncostedMinor), b.styles.total); err != nil {
		return err
	}

	return b.addPeriodCharts(sheet, headerRow+1, totalRow-1)
}

// xlsxVisiblePeriods is the actual row set: the workbook stops where the PDF's
// period table stops, so a future day or month is never printed as a zero
// actual. The chart and the summary still read the snapshot's own totals.
func xlsxVisiblePeriods(snapshot poreport.Report) []poreport.Period {
	limit, hasLimit := periodTableLimit(snapshot)
	if !hasLimit {
		return snapshot.Periods
	}
	periods := make([]poreport.Period, 0, len(snapshot.Periods))
	for _, period := range snapshot.Periods {
		start, err := time.Parse(time.DateOnly, period.PeriodStart)
		if err != nil || start.After(limit) {
			break
		}
		periods = append(periods, period)
	}
	return periods
}

// addPeriodCharts draws the two native charts beside the period table: one
// clustered column chart for the money and one line chart for margin %, which
// is a percentage and cannot share the money axis.
func (b *reportWorkbook) addPeriodCharts(sheet string, firstRow, lastRow int) error {
	if lastRow < firstRow {
		return nil
	}
	categories := fmt.Sprintf("%s!$A$%d:$A$%d", sheet, firstRow, lastRow)
	money := &excelize.Chart{
		Type: excelize.Col,
		Series: []excelize.ChartSeries{
			{Name: fmt.Sprintf("%s!$C$4", sheet), Categories: categories,
				Values: fmt.Sprintf("%s!$C$%d:$C$%d", sheet, firstRow, lastRow)},
			{Name: fmt.Sprintf("%s!$D$4", sheet), Categories: categories,
				Values: fmt.Sprintf("%s!$D$%d:$D$%d", sheet, firstRow, lastRow)},
			{Name: fmt.Sprintf("%s!$E$4", sheet), Categories: categories,
				Values: fmt.Sprintf("%s!$E$%d:$E$%d", sheet, firstRow, lastRow)},
			{Name: fmt.Sprintf("%s!$F$4", sheet), Categories: categories,
				Values: fmt.Sprintf("%s!$F$%d:$F$%d", sheet, firstRow, lastRow)},
		},
		Title:        chartTitle("Amount, costed, capital and margin by period"),
		Legend:       excelize.ChartLegend{Position: "bottom"},
		Dimension:    excelize.ChartDimension{Width: 900, Height: 320},
		XAxis:        excelize.ChartAxis{Title: chartTitle("Period")},
		YAxis:        excelize.ChartAxis{Title: chartTitle("Amount (" + b.info.Code + ")"), MajorGridLines: true},
		ShowBlanksAs: "gap",
	}
	margin := &excelize.Chart{
		Type: excelize.Line,
		Series: []excelize.ChartSeries{{
			Name:       fmt.Sprintf("%s!$G$4", sheet),
			Categories: categories,
			Values:     fmt.Sprintf("%s!$G$%d:$G$%d", sheet, firstRow, lastRow),
			Marker:     excelize.ChartMarker{Symbol: "circle", Size: 5},
		}},
		Title:     chartTitle("Margin % by period"),
		Legend:    excelize.ChartLegend{Position: "bottom"},
		Dimension: excelize.ChartDimension{Width: 900, Height: 320},
		XAxis:     excelize.ChartAxis{Title: chartTitle("Period")},
		YAxis: excelize.ChartAxis{
			Title:          chartTitle("Margin %"),
			NumFmt:         excelize.ChartNumFmt{CustomNumFmt: "0.00%", SourceLinked: false},
			MajorGridLines: true,
		},
		ShowBlanksAs: "gap",
	}
	if err := b.file.AddChart(sheet, "K4", money); err != nil {
		return fmt.Errorf("add the period column chart: %w", err)
	}
	if err := b.file.AddChart(sheet, "K22", margin); err != nil {
		return fmt.Errorf("add the period margin chart: %w", err)
	}
	return nil
}

// categorySection is the Categories sheet's row set. A legacy snapshot prints
// every supplied row and its own total; a view prints exactly the selected
// codes in the source's supplied order. Only an aggregate selection is the whole
// supplied row set, so only that one carries the source's total: a subset with
// the source total beside it would read as a subset total.
type categorySection struct {
	rows       []poreport.Category
	total      *poreport.Metrics
	totalLabel string
	note       string
	empty      string
}

// newCategorySection builds the Categories sheet's rows from the document.
func newCategorySection(overview poreport.Report, resolved *resolvedView) categorySection {
	if resolved == nil {
		totals := overview.Totals
		return categorySection{
			rows:       overview.Categories,
			total:      &totals,
			totalLabel: "Report total",
			empty:      "No purchase orders in the selected range.",
		}
	}

	section := categorySection{
		note:  "Selected business categories only, in the source's supplied order.",
		empty: "No categories selected. Choose at least one business category to print a category comparison; no figure and no total are shown here.",
	}
	if len(resolved.View.Categories.Codes) == 0 {
		return section
	}
	labels := make(map[string]string, len(resolved.Categories.Categories))
	for _, row := range resolved.Categories.Categories {
		labels[row.BusinessCategory] = row.Label
	}
	section.rows = make([]poreport.Category, 0, len(resolved.ResolvedCategories))
	for _, item := range resolved.ResolvedCategories {
		section.rows = append(section.rows, poreport.Category{
			BusinessCategory: item.Code,
			Label:            labels[item.Code],
			Metrics:          item.Metrics,
		})
	}
	if resolved.View.Categories.Layout == ViewLayoutAggregate {
		totals := resolved.Categories.Totals
		section.total = &totals
		section.totalLabel = "Source total"
		section.note += " The total is the category source's own total, not a sum of the printed rows."
	}
	return section
}

func (b *reportWorkbook) renderCategories(section categorySection) error {
	const sheet = xlsxCategoriesSheet
	if err := b.set(sheet, "A1", "Categories", b.styles.title); err != nil {
		return err
	}
	subtitle := "Actual purchase orders by business category. Capital is the pinned source cost; margin is costed amount minus capital."
	if section.note != "" {
		subtitle += " " + section.note
	}
	if err := b.set(sheet, "A2", subtitle, b.styles.subtitle); err != nil {
		return err
	}

	headerRow := 4
	headers := []string{
		"Code", "Category", "PO count", "Amount", "Costed", "Capital", "Margin", "Margin %", "Coverage %", "Uncosted",
	}
	for index, header := range headers {
		if err := b.set(sheet, fmt.Sprintf("%s%d", columnLetter(index), headerRow), header, b.styles.header); err != nil {
			return err
		}
	}
	if len(section.rows) == 0 {
		return b.set(sheet, fmt.Sprintf("A%d", headerRow+1), section.empty, b.styles.subtitle)
	}

	row := headerRow + 1
	for _, category := range section.rows {
		label := category.Label
		if strings.TrimSpace(label) == "" {
			label = categoryLabel(category.BusinessCategory)
		}
		if err := b.set(sheet, fmt.Sprintf("A%d", row), category.BusinessCategory, b.styles.text); err != nil {
			return err
		}
		if err := b.set(sheet, fmt.Sprintf("B%d", row), label, b.styles.text); err != nil {
			return err
		}
		if err := b.set(sheet, fmt.Sprintf("C%d", row), category.PoCount, b.styles.count); err != nil {
			return err
		}
		for index, minor := range []int64{
			category.AmountMinor, category.CostedMinor, category.CapitalMinor, category.MarginMinor,
		} {
			column := columnLetter(3 + index)
			if err := b.set(sheet, fmt.Sprintf("%s%d", column, row), b.major(minor), b.styles.money); err != nil {
				return err
			}
		}
		if err := b.setPercentCell(sheet, fmt.Sprintf("H%d", row), category.MarginBps); err != nil {
			return err
		}
		if err := b.setPercentCell(sheet, fmt.Sprintf("I%d", row), category.CoverageBps); err != nil {
			return err
		}
		if err := b.set(sheet, fmt.Sprintf("J%d", row), b.major(category.UncostedMinor), b.styles.money); err != nil {
			return err
		}
		row++
	}

	chart := &excelize.Chart{
		Type: excelize.Bar,
		Series: []excelize.ChartSeries{{
			Name:       fmt.Sprintf("%s!$D$4", sheet),
			Categories: fmt.Sprintf("%s!$A$%d:$A$%d", sheet, headerRow+1, row-1),
			Values:     fmt.Sprintf("%s!$D$%d:$D$%d", sheet, headerRow+1, row-1),
		}},
		Title:        chartTitle("Amount by business category"),
		Legend:       excelize.ChartLegend{Position: "bottom"},
		Dimension:    excelize.ChartDimension{Width: 720, Height: 300},
		XAxis:        excelize.ChartAxis{Title: chartTitle("Category")},
		YAxis:        excelize.ChartAxis{Title: chartTitle("Amount (" + b.info.Code + ")"), MajorGridLines: true},
		ShowBlanksAs: "gap",
	}
	if err := b.file.AddChart(sheet, "L4", chart); err != nil {
		return fmt.Errorf("add the category bar chart: %w", err)
	}

	if section.total == nil {
		return nil
	}
	total := *section.total
	if err := b.set(sheet, fmt.Sprintf("A%d", row), section.totalLabel, b.styles.strong); err != nil {
		return err
	}
	if err := b.set(sheet, fmt.Sprintf("C%d", row), total.PoCount, b.styles.count); err != nil {
		return err
	}
	for index, minor := range []int64{
		total.AmountMinor, total.CostedMinor, total.CapitalMinor, total.MarginMinor,
	} {
		column := columnLetter(3 + index)
		if err := b.set(sheet, fmt.Sprintf("%s%d", column, row), b.major(minor), b.styles.total); err != nil {
			return err
		}
	}
	if err := b.setPercentCell(sheet, fmt.Sprintf("H%d", row), total.MarginBps); err != nil {
		return err
	}
	if err := b.setPercentCell(sheet, fmt.Sprintf("I%d", row), total.CoverageBps); err != nil {
		return err
	}
	if err := b.set(sheet, fmt.Sprintf("J%d", row), b.major(total.UncostedMinor), b.styles.total); err != nil {
		return err
	}
	return nil
}

// setPercentCell writes a ratio as a fraction with the workbook's percent
// format. A nil ratio is unknown, not zero percent, and is written as a dash.
func (b *reportWorkbook) setPercentCell(sheet, cell string, bps *int64) error {
	if bps == nil {
		return b.set(sheet, cell, "-", b.styles.text)
	}
	return b.set(sheet, cell, float64(*bps)/10000, b.styles.percent)
}

// columnLetter numbers columns from zero: 0 is A. The workbook never addresses a
// column past N, so the single-letter form is the whole vocabulary.
func columnLetter(index int) string {
	if index < 0 || index > 25 {
		return "A"
	}
	return string(rune('A' + index))
}

func chartTitle(text string) excelize.ChartTitle {
	return excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: text}}}
}

func (b *reportWorkbook) renderForecast(snapshot poreport.Report) error {
	const sheet = xlsxForecastSheet
	if err := b.set(sheet, "A1", "Forecast", b.styles.title); err != nil {
		return err
	}
	if err := b.set(sheet, "A2",
		"Projections are estimates from complete history. They are not actual evidence and are never added to a total.",
		b.styles.subtitle); err != nil {
		return err
	}

	monthly := snapshot.Projection.Monthly
	if err := b.labelValue(sheet, 4, "Status", projectionStatusLabel(monthly.Status), b.styles.text); err != nil {
		return err
	}
	if err := b.labelValue(sheet, 5, "Method", monthly.Method, b.styles.wrapped); err != nil {
		return err
	}
	basis := monthly.Basis
	basisText := fmt.Sprintf("History from %s to %s (exclusive); %d complete months; %d purchase orders.",
		basis.HistoryFrom, basis.HistoryTo, basis.CompleteMonths, basis.PoCount)
	if err := b.labelValue(sheet, 6, "Basis", basisText, b.styles.text); err != nil {
		return err
	}
	if monthly.Status != poreport.ProjectionStatusAvailable {
		return b.labelValue(sheet, 7, "Reason", projectionReasonLabel(monthly.Reason), b.styles.wrapped)
	}

	headerRow := 8
	headers := []string{"Period", "Amount low", "Amount base", "Amount high", "Capital", "Margin", "Margin %"}
	for index, header := range headers {
		if err := b.set(sheet, fmt.Sprintf("%s%d", columnLetter(index), headerRow), header, b.styles.header); err != nil {
			return err
		}
	}
	if len(monthly.Buckets) == 0 {
		return b.set(sheet, fmt.Sprintf("A%d", headerRow+1),
			"No projected buckets are available.", b.styles.subtitle)
	}

	row := headerRow + 1
	for _, bucket := range monthly.Buckets {
		if err := b.set(sheet, fmt.Sprintf("A%d", row), bucket.PeriodStart, b.styles.text); err != nil {
			return err
		}
		for index, minor := range []int64{
			bucket.AmountLowMinor, bucket.AmountBaseMinor, bucket.AmountHighMinor,
			bucket.CapitalMinor, bucket.MarginMinor,
		} {
			column := columnLetter(1 + index)
			if err := b.set(sheet, fmt.Sprintf("%s%d", column, row), b.major(minor), b.styles.money); err != nil {
				return err
			}
		}
		if err := b.setPercentCell(sheet, fmt.Sprintf("G%d", row), bucket.MarginBps); err != nil {
			return err
		}
		row++
	}

	return b.addForecastChart(sheet, headerRow+1, row-1)
}

// addForecastChart places the projection-only chart below the bucket table.
//
// It plots the projected buckets and nothing else. The Forecast sheet has no
// actual series at all: a current-period report materializes future days as
// zero placeholders, and a zero on this sheet would claim an actual that has
// not happened. The retired actual-versus-forecast combo also put a monthly
// projection bucket on a day-bucketed report's monthly axis beside the daily
// actual of the same date, so one row carried both. The Periods sheet's chart
// is the actual series; a forecast is never evidence and never shares its axis
// with one.
func (b *reportWorkbook) addForecastChart(sheet string, firstRow, lastRow int) error {
	if lastRow < firstRow {
		return nil
	}
	categories := fmt.Sprintf("%s!$A$%d:$A$%d", sheet, firstRow, lastRow)
	series := make([]excelize.ChartSeries, 0, 3)
	for _, column := range []string{"B", "C", "D"} {
		series = append(series, excelize.ChartSeries{
			Name:       fmt.Sprintf("%s!$%s$%d", sheet, column, firstRow-1),
			Categories: categories,
			Values:     fmt.Sprintf("%s!$%s$%d:$%s$%d", sheet, column, firstRow, column, lastRow),
		})
	}
	chart := &excelize.Chart{
		Type:         excelize.Col,
		Series:       series,
		Title:        chartTitle("Forecast amount (low, base, high) by projected bucket"),
		Legend:       excelize.ChartLegend{Position: "bottom"},
		Dimension:    excelize.ChartDimension{Width: 900, Height: 340},
		XAxis:        excelize.ChartAxis{Title: chartTitle("Projected bucket")},
		YAxis:        excelize.ChartAxis{Title: chartTitle("Amount (" + b.info.Code + ")"), MajorGridLines: true},
		ShowBlanksAs: "gap",
	}
	// The bucket table sits above the chart, so the chart never covers a cell a
	// reader might want to inspect.
	anchor := fmt.Sprintf("A%d", lastRow+2)
	if err := b.file.AddChart(sheet, anchor, chart); err != nil {
		return fmt.Errorf("add the forecast chart: %w", err)
	}
	return nil
}

func (b *reportWorkbook) renderNotes(snapshot poreport.Report) error {
	row := 1
	if err := b.set(xlsxNotesSheet, fmt.Sprintf("A%d", row), "Notes", b.styles.title); err != nil {
		return err
	}
	row += 1
	if err := b.set(xlsxNotesSheet, fmt.Sprintf("A%d", row),
		"Internal report: not a customer quotation. PPN is excluded from every figure.", b.styles.subtitle); err != nil {
		return err
	}
	row += 2
	if err := b.set(xlsxNotesSheet, fmt.Sprintf("A%d", row), "Code", b.styles.header); err != nil {
		return err
	}
	if err := b.set(xlsxNotesSheet, fmt.Sprintf("B%d", row), "Note", b.styles.header); err != nil {
		return err
	}
	row++
	if len(snapshot.Notes) == 0 {
		return b.set(xlsxNotesSheet, fmt.Sprintf("A%d", row), "No report notes.", b.styles.subtitle)
	}
	for _, note := range snapshot.Notes {
		if err := b.set(xlsxNotesSheet, fmt.Sprintf("A%d", row), note.Code, b.styles.text); err != nil {
			return err
		}
		if err := b.set(xlsxNotesSheet, fmt.Sprintf("B%d", row), note.Text, b.styles.wrapped); err != nil {
			return err
		}
		row++
	}
	return nil
}
