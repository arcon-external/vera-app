package report

import (
	"strconv"

	"vera-app/backend/poreport"
)

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

// drawTable prints a titled table, repeating its header on every page it
// spans. A 120-period report is the reason this exists: the period rows are
// never clipped, never reduced to an unreadable font, and never dropped, and a
// reader on page six still knows what each column means.
func (r *reportRenderer) drawTable(title, note string, columns []tableColumn, rows []tableRow, total *tableRow) {
	c := r.canvas
	noteLines := r.faces.wrap(note, 7, contentWidth, false)
	// Keep the title, the header and at least two rows together. A header
	// alone at the bottom of a page is not a table.
	c.ensure(15 + float64(len(noteLines))*9 + 3 + 18 + 31)
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
		if c.y+15.5 > contentBot {
			c.newPage()
			c.text(title+" (continued)", marginX, c.y+9, 8.8, true, r.pal.emphasis)
			c.y += 14
			r.drawTableHeader(columns)
		}
		r.drawTableRow(columns, row, false)
	}
	if total != nil {
		if c.y+17 > contentBot {
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

func (r *reportRenderer) drawTableRow(columns []tableColumn, row tableRow, total bool) {
	c := r.canvas
	if total {
		c.line(marginX, c.y, marginX+contentWidth, c.y, r.pal.emphasis, 0.8)
	}
	fill := row.fill
	if total {
		surface := r.pal.surface
		fill = &surface
	}
	if fill != nil {
		c.rect(marginX, c.y, contentWidth, 15.5, fill, nil, 0)
	}
	x := marginX
	for index, column := range columns {
		cell := ""
		if index < len(row.cells) {
			cell = row.cells[index]
		}
		bold := row.bold || total
		size := fittedSize(r.faces.width, cell, column.width-8, 7.1, 5.5, bold)
		if column.align == alignRight {
			c.textRight(cell, x+column.width-4, c.y+10.9, size, bold, r.pal.emphasis)
		} else {
			c.text(cell, x+4, c.y+10.9, size, bold, r.pal.emphasis)
		}
		x += column.width
	}
	c.line(marginX, c.y+15.5, marginX+contentWidth, c.y+15.5, r.pal.rule, 0.3)
	c.y += 15.5
}

func (r *reportRenderer) drawCategoryTable() {
	if len(r.snapshot.Categories) == 0 {
		r.sectionTitle("By business category", "")
		r.emptyState("No category rows",
			"The selected range has no saved purchase orders to break down.")
		return
	}
	columns := []tableColumn{
		{title: "CATEGORY", width: 100, align: alignLeft},
		{title: "POs", width: 30, align: alignRight},
		{title: "AMOUNT", width: 93, align: alignRight},
		{title: "COSTED", width: 88, align: alignRight},
		{title: "CAPITAL", width: 76, align: alignRight},
		{title: "MARGIN", width: 76, align: alignRight},
		{title: "MARGIN %", width: contentWidth - 463, align: alignRight},
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
		{title: "PERIOD", width: 88, align: alignLeft},
		{title: "POs", width: 32, align: alignRight},
		{title: "AMOUNT", width: 93, align: alignRight},
		{title: "COSTED", width: 88, align: alignRight},
		{title: "CAPITAL", width: 78, align: alignRight},
		{title: "MARGIN", width: 78, align: alignRight},
		{title: "MARGIN %", width: contentWidth - 457, align: alignRight},
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
			strconv.Itoa(projected.SampleSize),
		}})
	}
	r.drawTable("Projected range",
		"Estimates, not actuals. Low and high bound the amount band; the base is the single planning figure. "+
			"n is the number of observations behind the range.",
		columns, rows, nil)
}
