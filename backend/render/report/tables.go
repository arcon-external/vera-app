package report

import "vera-app/backend/poreport"

type tableAlign int

const (
	alignLeft tableAlign = iota
	alignRight
)

type tableColumn struct {
	title string
	width float64
	align tableAlign
}

type tableRow struct {
	cells []string
	bold  bool
	fill  *rgb
}

// The body face is deliberately readable: an accepted report may carry a
// maximum exact amount, so the renderer wraps a cell onto measured lines
// rather than shrinking it to a 5pt floor that still overflows into the next
// column. A typical amount fits on one line at 8pt; only the widest accepted
// values and the longest label take two or more.
const (
	tableBodySize      = 8.0
	tableLineHeight    = 9.8
	tableCellPadX      = 4.0
	tableCellPadTop    = 3.0
	tableCellPadBottom = 3.0
)

// drawTable prints a titled table, repeating its header on every page it
// spans. A 120-period report is the reason this exists: the period rows are
// never clipped, never reduced to an unreadable font, and never dropped, and a
// reader on page six still knows what each column means.
func (r *reportRenderer) drawTable(title, note string, columns []tableColumn, rows []tableRow, total *tableRow) {
	c := r.canvas
	noteLines := r.faces.wrap(note, 7, contentWidth, false)
	firstRowHeight := 18.0
	if len(rows) > 0 {
		firstRowHeight = r.tableRowHeight(columns, rows[0], false)
	} else if total != nil {
		firstRowHeight = r.tableRowHeight(columns, *total, true)
	}
	// Keep the title, the header and at least one row together. A header
	// alone at the bottom of a page is not a table.
	c.ensure(15 + float64(len(noteLines))*9 + 3 + 18 + firstRowHeight)
	c.text(title, marginX, c.y+10, 9.5, true, r.pal.emphasis)
	c.y += 15
	if note != "" {
		for _, line := range noteLines {
			c.text(line, marginX, c.y+7, 7, false, r.pal.muted)
			c.y += 9
		}
	}
	c.y += 3
	r.drawTableHeader(columns)
	for _, row := range rows {
		height := r.tableRowHeight(columns, row, false)
		if c.y+height > contentBot {
			c.newPage()
			c.text(title+" (continued)", marginX, c.y+9, 8.8, true, r.pal.emphasis)
			c.y += 14
			r.drawTableHeader(columns)
		}
		r.drawTableRow(columns, row, false)
	}
	if total != nil {
		height := r.tableRowHeight(columns, *total, true)
		if c.y+height > contentBot {
			c.newPage()
			c.text(title+" (continued)", marginX, c.y+9, 8.8, true, r.pal.emphasis)
			c.y += 14
			r.drawTableHeader(columns)
		}
		r.drawTableRow(columns, *total, true)
	}
	c.y += 14
}

func (r *reportRenderer) drawTableHeader(columns []tableColumn) {
	c := r.canvas
	c.rect(marginX, c.y, contentWidth, 18, &r.pal.emphasis, nil, 0)
	x := marginX
	for _, column := range columns {
		size := fittedSize(r.faces.width, column.title, column.width-8, 6.6, 5.6, true)
		if column.align == alignRight {
			c.textRight(column.title, x+column.width-4, c.y+12.2, size, true, r.pal.paper)
		} else {
			c.text(column.title, x+4, c.y+12.2, size, true, r.pal.paper)
		}
		x += column.width
	}
	c.y += 18
}

func (r *reportRenderer) tableCellLines(cell string, width float64, bold bool) []string {
	lines := r.faces.wrapCell(cell, tableBodySize, width-2*tableCellPadX, bold)
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// tableRowHeight is measured from the wrapped cell lines, not assumed. A
// wrapped amount or a 128-character category label therefore pushes the next
// row down instead of colliding with it.
func (r *reportRenderer) tableRowHeight(columns []tableColumn, row tableRow, total bool) float64 {
	bold := row.bold || total
	maxLines := 1
	for index, column := range columns {
		cell := ""
		if index < len(row.cells) {
			cell = row.cells[index]
		}
		if lines := len(r.tableCellLines(cell, column.width, bold)); lines > maxLines {
			maxLines = lines
		}
	}
	return float64(maxLines)*tableLineHeight + tableCellPadTop + tableCellPadBottom
}

func (r *reportRenderer) drawTableRow(columns []tableColumn, row tableRow, total bool) {
	c := r.canvas
	if total {
		c.line(marginX, c.y, marginX+contentWidth, c.y, r.pal.emphasis, 0.8)
	}
	height := r.tableRowHeight(columns, row, total)
	fill := row.fill
	if total {
		surface := r.pal.surface
		fill = &surface
	}
	if fill != nil {
		c.rect(marginX, c.y, contentWidth, height, fill, nil, 0)
	}
	x := marginX
	bold := row.bold || total
	for index, column := range columns {
		cell := ""
		if index < len(row.cells) {
			cell = row.cells[index]
		}
		lines := r.tableCellLines(cell, column.width, bold)
		for lineIndex, line := range lines {
			baseline := c.y + tableCellPadTop + tableBodySize + float64(lineIndex)*tableLineHeight
			if column.align == alignRight {
				c.textRight(line, x+column.width-tableCellPadX, baseline, tableBodySize, bold, r.pal.emphasis)
			} else {
				c.text(line, x+tableCellPadX, baseline, tableBodySize, bold, r.pal.emphasis)
			}
		}
		x += column.width
	}
	c.line(marginX, c.y+height, marginX+contentWidth, c.y+height, r.pal.rule, 0.3)
	c.y += height
}

func (r *reportRenderer) drawCategoryTable() {
	if len(r.snapshot.Categories) == 0 {
		r.sectionTitle("By business category", "")
		r.emptyState("No category rows",
			"The selected range has no saved purchase orders to break down.")
		return
	}
	columns := []tableColumn{
		{title: "CATEGORY", width: 86, align: alignLeft},
		{title: "POs", width: 28, align: alignRight},
		{title: "AMOUNT", width: 88, align: alignRight},
		{title: "COSTED", width: 86, align: alignRight},
		{title: "CAPITAL", width: 78, align: alignRight},
		{title: "MARGIN", width: 78, align: alignRight},
		{title: "MARGIN %", width: contentWidth - 444, align: alignRight},
	}
	rows := make([]tableRow, 0, len(r.snapshot.Categories))
	for index, category := range r.snapshot.Categories {
		var fill *rgb
		if index%2 == 1 {
			surface := r.pal.surface
			fill = &surface
		}
		rows = append(rows, tableRow{fill: fill, cells: []string{
			categoryRowLabel(category.BusinessCategory, category.Label),
			formatCount(category.PoCount),
			r.money(category.AmountMinor),
			r.money(category.CostedMinor),
			r.money(category.CapitalMinor),
			r.signedMoney(category.MarginMinor),
			formatPct(category.MarginBps),
		}})
	}
	total := r.totalRow("Total")
	r.drawTable("By business category",
		"All amounts in "+r.currency()+". The category rows foot to the total for the selected filter.",
		columns, rows, &total)
}

func (r *reportRenderer) drawPeriodTable() {
	columns := []tableColumn{
		{title: "PERIOD", width: 86, align: alignLeft},
		{title: "POs", width: 28, align: alignRight},
		{title: "AMOUNT", width: 88, align: alignRight},
		{title: "COSTED", width: 86, align: alignRight},
		{title: "CAPITAL", width: 78, align: alignRight},
		{title: "MARGIN", width: 78, align: alignRight},
		{title: "MARGIN %", width: contentWidth - 444, align: alignRight},
	}
	rows := make([]tableRow, 0, len(r.snapshot.Periods))
	for index, period := range r.snapshot.Periods {
		var fill *rgb
		if index%2 == 1 {
			surface := r.pal.surface
			fill = &surface
		}
		label := periodTableLabel(period.PeriodStart, r.snapshot.Filter.Bucket)
		if marker := periodMarker(period); marker != "" {
			label += " " + marker
		}
		rows = append(rows, tableRow{fill: fill, cells: []string{
			label,
			formatCount(period.PoCount),
			r.money(period.AmountMinor),
			r.money(period.CostedMinor),
			r.money(period.CapitalMinor),
			r.signedMoney(period.MarginMinor),
			formatPct(period.MarginBps),
		}})
	}
	total := r.totalRow("Total")
	r.drawTable("By period",
		"All amounts in "+r.currency()+". P = partial period; IP = period still in progress. "+
			"Projected periods never appear in this table and never enter the total.",
		columns, rows, &total)
}

func (r *reportRenderer) totalRow(label string) tableRow {
	return tableRow{bold: true, cells: []string{
		label,
		formatCount(r.snapshot.Totals.PoCount),
		r.money(r.snapshot.Totals.AmountMinor),
		r.money(r.snapshot.Totals.CostedMinor),
		r.money(r.snapshot.Totals.CapitalMinor),
		r.signedMoney(r.snapshot.Totals.MarginMinor),
		formatPct(r.snapshot.Totals.MarginBps),
	}}
}

func (r *reportRenderer) drawProjectionTable(series poreport.ProjectionSeries, bucket string) {
	columns := []tableColumn{
		{title: "PERIOD", width: 62, align: alignLeft},
		{title: "LOW", width: 76, align: alignRight},
		{title: "BASE", width: 76, align: alignRight},
		{title: "HIGH", width: 76, align: alignRight},
		{title: "CAPITAL", width: 68, align: alignRight},
		{title: "MARGIN", width: 68, align: alignRight},
		{title: "MARGIN %", width: 45, align: alignRight},
		{title: "n", width: contentWidth - 471, align: alignRight},
	}
	rows := make([]tableRow, 0, len(series.Buckets))
	for index, projected := range series.Buckets {
		var fill *rgb
		if index%2 == 1 {
			surface := r.pal.surface
			fill = &surface
		}
		rows = append(rows, tableRow{fill: fill, cells: []string{
			periodTableLabel(projected.PeriodStart, bucket),
			r.money(projected.AmountLowMinor),
			r.money(projected.AmountBaseMinor),
			r.money(projected.AmountHighMinor),
			r.money(projected.CapitalMinor),
			r.signedMoney(projected.MarginMinor),
			formatPct(projected.MarginBps),
			formatCount(int64(projected.SampleSize)),
		}})
	}
	r.drawTable("Projected range",
		"Estimates, not actuals. Low and high bound the amount band; the base is the single planning figure. "+
			"n is the number of observations behind the range.",
		columns, rows, nil)
}
