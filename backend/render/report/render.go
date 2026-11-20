package report

import (
	"fmt"
	"io/fs"
	"strings"

	"vera-app/backend/poreport"
)

// Render lays the supplied snapshot out as an A4 portrait PDF.
//
// It validates the snapshot first, so a caller can never print a document the
// decoder would refuse. The renderer reads no clock, makes no network call and
// computes no report figure: every number on the page came from the snapshot.
func Render(snapshot poreport.Report, fonts fs.FS) ([]byte, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return nil, err
	}
	loaded, err := loadFaces(fonts)
	if err != nil {
		return nil, err
	}
	r := &reportRenderer{
		snapshot: snapshot,
		faces:    loaded,
		pal:      defaultPalette(),
		canvas:   newCanvas(loaded),
	}
	r.draw()
	return r.canvas.render("Purchase Order Performance Report")
}

type reportRenderer struct {
	snapshot poreport.Report
	faces    *faces
	pal      palette
	canvas   *canvas
}

func (r *reportRenderer) draw() {
	r.drawHeader()
	r.drawKPIs()
	r.drawCharts()
	r.drawCategoryTable()
	r.drawPeriodTable()
	r.drawProjectionDetails()
	r.drawReportNotes()
	r.drawDefinitions()
	r.drawFooters()
}

func (r *reportRenderer) currency() string {
	return r.snapshot.Filter.Currency
}

func (r *reportRenderer) money(minor int64) string {
	return formatMoney(r.currency(), minor)
}

func (r *reportRenderer) signedMoney(minor int64) string {
	return formatSignedMoney(r.currency(), minor)
}

func (r *reportRenderer) sectionTitle(title, subtitle string) {
	c := r.canvas
	c.ensure(38)
	c.text(title, marginX, c.y+11, 10, true, r.pal.emphasis)
	c.y += 16
	if subtitle != "" {
		for _, line := range r.faces.wrap(subtitle, 7.2, contentWidth, false) {
			c.ensure(10)
			c.text(line, marginX, c.y+7.5, 7.2, false, r.pal.muted)
			c.y += 9.5
		}
	}
	c.y += 6
}

func (r *reportRenderer) drawHeader() {
	c := r.canvas
	pal := r.pal
	top := c.y

	// The thin accent rule is the report's one piece of brand decoration. It
	// is drawn before anything else so the page reads as a Vera document at a
	// glance, without pretending to be the customer quotation.
	c.rect(marginX, top, contentWidth, 2.5, &pal.accent, nil, 0)
	c.text("INTERNAL REPORT  ·  PURCHASE ORDERS", marginX, top+16, 7, true, pal.accent)
	c.text("Purchase Order Performance Report", marginX, top+41, 17, true, pal.emphasis)

	leftWidth := 300.0
	companyBaseline := top + 60
	companyLines := r.faces.wrapCell(r.snapshot.Company.Name, 10.5, leftWidth, true)
	if len(companyLines) == 0 {
		companyLines = []string{""}
	}
	for index, line := range companyLines {
		c.text(line, marginX, companyBaseline+float64(index)*12, 10.5, true, pal.emphasis)
	}
	addressBaseline := companyBaseline + float64(len(companyLines))*12
	addressLines := r.faces.wrapCell(r.snapshot.Company.Address, 8, leftWidth, false)
	if len(addressLines) == 0 {
		addressLines = []string{"Company address not recorded"}
	}
	for _, line := range addressLines {
		c.text(line, marginX, addressBaseline, 8, false, pal.secondary)
		addressBaseline += 10
	}

	rightX := marginX + leftWidth + 24
	valueX := rightX + 58
	metaBaseline := top + 20
	meta := []struct{ label, value string }{
		{"PERIOD", r.snapshot.Filter.From + " → " + r.snapshot.Filter.To + " (exclusive)"},
		{"BUCKET", bucketLabel(r.snapshot.Filter.Bucket)},
		{"CATEGORY", categoryFilterLabel(r.snapshot.Filter.Category)},
		{"CURRENCY", r.snapshot.Filter.Currency},
		{"GENERATED", generatedLabel(r.snapshot.GeneratedAt)},
		{"AS OF", r.snapshot.AsOf},
	}
	for _, item := range meta {
		c.text(item.label, rightX, metaBaseline, 6.2, true, pal.muted)
		size := fittedSize(r.faces.width, item.value, contentWidth-(valueX-marginX)-4, 7.6, 6.2, false)
		c.text(item.value, valueX, metaBaseline, size, false, pal.emphasis)
		metaBaseline += 12.5
	}

	noteTop := addressBaseline
	if metaBaseline > noteTop {
		noteTop = metaBaseline
	}
	noteTop += 6
	c.rect(marginX, noteTop, contentWidth, 24, &pal.surface, &pal.edge, 0.5)
	c.rect(marginX, noteTop, 3, 24, &pal.accent, nil, 0)
	c.text("Internal report: not a customer quotation. PPN is excluded from every figure.",
		marginX+10, noteTop+15.5, 7.8, true, pal.emphasis)
	c.y = noteTop + 36
}

const kpiValueSize = 8.5

func (r *reportRenderer) drawKPIs() {
	c := r.canvas
	pal := r.pal
	gap := 8.0
	width := (contentWidth - 4*gap) / 5
	totals := r.snapshot.Totals

	kpis := []struct {
		label string
		value string
		color rgb
	}{
		{"PO COUNT", formatCount(totals.PoCount), pal.emphasis},
		{"PO AMOUNT", r.money(totals.AmountMinor), pal.emphasis},
		{"CAPITAL", r.money(totals.CapitalMinor), pal.emphasis},
		{"PO MARGIN", r.signedMoney(totals.MarginMinor), marginColor(pal, totals.MarginMinor)},
		{"PO MARGIN %", formatPct(totals.MarginBps), marginColor(pal, totals.MarginMinor)},
	}
	linesByKPI := make([][]string, len(kpis))
	maxLines := 1
	for index, kpi := range kpis {
		linesByKPI[index] = r.faces.wrapCell(kpi.value, kpiValueSize, width-16, true)
		if count := len(linesByKPI[index]); count > maxLines {
			maxLines = count
		}
	}
	cardHeight := 54.0
	if maxLines > 1 {
		cardHeight = 30 + float64(maxLines)*12.5 + 6
	}
	c.ensure(cardHeight + 10)
	top := c.y
	for index, kpi := range kpis {
		x := marginX + float64(index)*(width+gap)
		c.rect(x, top, width, cardHeight, &pal.surface, &pal.edge, 0.5)
		c.text(kpi.label, x+8, top+14, 6.1, true, pal.muted)
		lines := linesByKPI[index]
		for lineIndex, line := range lines {
			baseline := top + 37
			if len(lines) > 1 {
				baseline = top + 30 + float64(lineIndex)*12.5
			}
			c.text(line, x+8, baseline, kpiValueSize, true, kpi.color)
		}
	}
	c.y = top + cardHeight + 10

	coverage := ""
	if totals.UncostedMinor != 0 {
		coverage = fmt.Sprintf("Costed %s of %s (%s)  ·  Uncosted %s",
			r.money(totals.CostedMinor), r.money(totals.AmountMinor),
			formatPct(totals.CoverageBps), r.money(totals.UncostedMinor))
	} else if totals.AmountMinor != 0 {
		coverage = fmt.Sprintf("Costed %s of %s (%s)  ·  every amount in this range has a known source cost",
			r.money(totals.CostedMinor), r.money(totals.AmountMinor), formatPct(totals.CoverageBps))
	}
	if coverage != "" {
		for _, line := range r.faces.wrap(coverage, 7.5, contentWidth, false) {
			c.ensure(10)
			c.text(line, marginX, c.y+7, 7.5, false, pal.secondary)
			c.y += 9.5
		}
	}
	if r.snapshot.Population.SourceMissingPoCount > 0 {
		line := fmt.Sprintf("%d purchase order(s) in this range have a deleted source quotation; their amount is uncosted.",
			r.snapshot.Population.SourceMissingPoCount)
		for _, text := range r.faces.wrap(line, 7.2, contentWidth, false) {
			c.ensure(10)
			c.text(text, marginX, c.y+7, 7.2, false, pal.muted)
			c.y += 9.5
		}
	}
	c.y += 10
}

func marginColor(pal palette, margin int64) rgb {
	if margin < 0 {
		return pal.marginNeg
	}
	return pal.emphasis
}

func (r *reportRenderer) drawCharts() {
	r.sectionTitle("Performance chart",
		"All amounts in "+r.currency()+". Columns start at zero; the margin % line uses the right-hand percentage axis. "+
			"An uncosted part of the amount is hatched, so the amount column still foots.")
	if len(r.snapshot.Periods) == 0 {
		r.emptyState("No purchase orders in the selected range.",
			"The report still prints its filter, its definitions and the projection status so the empty result is unambiguous.")
	} else {
		r.drawLegend()
		r.drawActualChartPanels()
	}
	r.drawProjectionCharts()
	r.canvas.y += 8
}

func (r *reportRenderer) emptyState(title, body string) {
	c := r.canvas
	c.ensure(52)
	c.rect(marginX, c.y, contentWidth, 46, &r.pal.surface, &r.pal.edge, 0.5)
	c.text(title, marginX+12, c.y+18, 9, true, r.pal.emphasis)
	c.text(body, marginX+12, c.y+32, 7.2, false, r.pal.secondary)
	c.y += 56
}

func (r *reportRenderer) drawReportNotes() {
	if len(r.snapshot.Notes) == 0 {
		return
	}
	r.sectionTitle("Report notes", "")
	for _, note := range r.snapshot.Notes {
		lines := r.faces.wrap(note.Text, 7.5, contentWidth-10, false)
		for index, line := range lines {
			r.canvas.ensure(11)
			if index == 0 {
				r.canvas.text("•", marginX, r.canvas.y+8, 7.5, true, r.pal.accent)
			}
			r.canvas.text(line, marginX+10, r.canvas.y+8, 7.5, false, r.pal.secondary)
			r.canvas.y += 10.5
		}
		r.canvas.y += 3
	}
	r.canvas.y += 4
}

func (r *reportRenderer) drawDefinitions() {
	r.sectionTitle("Definitions", "")
	definitions := []string{
		"PPN is excluded from every amount, costed amount, capital and margin figure in this report. The customer pays PPN on top of a quotation; it is collected for the state and is never profit.",
		"Capital is the pinned source revision's pre-margin unit cost at the purchase order's quantity, rounded once per purchase order. It is not the quotation's sell price and it is not the latest revision's cost.",
		"PO margin is costed amount minus capital. It covers only purchase orders whose source cost resolved; uncosted amount is never counted as profit. PPH23 is not subtracted: this is a PO margin, not the quotation's nett profit.",
		"Corrections are counted once at their current values. A correction can move a purchase order between periods, so this report reflects current records, not the values as they stood when each order was received.",
		"Projected periods are estimates from complete history. They are drawn as a low–high range with a base, never styled as actuals, and never included in an actual total.",
		"Partial and in-progress periods are marked in the period table. They should not be compared with complete periods as if the same amount of time had passed.",
		"All money is in " + r.currency() + " and no figure is converted between currencies. Counts may combine currencies only within the selected one; money never does.",
	}
	for _, definition := range definitions {
		lines := r.faces.wrap(definition, 7.5, contentWidth-12, false)
		r.canvas.ensure(float64(len(lines))*10.5 + 4)
		r.canvas.text("•", marginX, r.canvas.y+8, 7.5, true, r.pal.accent)
		for _, line := range lines {
			r.canvas.text(line, marginX+11, r.canvas.y+8, 7.5, false, r.pal.secondary)
			r.canvas.y += 10.5
		}
		r.canvas.y += 4
	}
}

func (r *reportRenderer) drawFooters() {
	pages := r.canvas.pages
	total := len(pages)
	for index, pg := range pages {
		r.canvas.current = pg
		r.canvas.line(marginX, footerY-9, PageWidth-marginX, footerY-9, r.pal.rule, 0.4)
		left := strings.TrimSpace(r.snapshot.Company.Name) + "  ·  Purchase Order Performance Report  ·  Internal"
		maxWidth := contentWidth - 110
		lines := r.faces.wrapCell(left, 6.2, maxWidth, false)
		if len(lines) > 2 {
			joined := strings.Join(lines[2:], " ")
			lines = lines[:2]
			lines[1] = r.faces.truncateToWidth(lines[1]+" "+joined, 6.2, maxWidth, false)
		}
		if len(lines) == 0 {
			lines = []string{""}
		}
		top := footerY - float64(len(lines)-1)*7
		for lineIndex, line := range lines {
			r.canvas.text(line, marginX, top+float64(lineIndex)*7, 6.2, false, r.pal.muted)
		}
		r.canvas.textRight(fmt.Sprintf("Page %d of %d", index+1, total),
			PageWidth-marginX, footerY, 6.6, false, r.pal.muted)
	}
	r.canvas.current = pages[len(pages)-1]
}
