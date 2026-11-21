package report

import (
	"fmt"
	"math"
	"strings"
	"time"

	"vera-app/backend/poreport"
)

// Issue #642: the native individual comparison grid and the one-calendar-year
// Aggregate table. A selected set prints at most two cards per row; a final odd
// card spans the content width. Each card is measured and kept on one page, and
// the section context is repeated when the grid continues. Aggregate keeps the
// original wide six-metric comparison table and reads its one total from the
// annual server snapshot, never from a row sum or an estimate.

const (
	cardGap         = 12.0
	cardPad         = 8.0
	cardLabelWidth  = 78.0
	cardValueSize   = 8.0
	cardLabelSize   = 7.6
	cardLineHeight  = 9.6
	cardScopeSize   = 6.8
	cardScopeLine   = 8.8
	cardNoteSize    = 6.9
	cardNoteLine    = 8.4
	cardEvidencePad = 3.0
)

// Comparison-card sections. One grid engine draws both; the section decides
// which layout-evidence slice records the box and is never printed.
const (
	cardSectionPeriod   = "period"
	cardSectionCategory = "category"
)

type cardBox struct {
	// Section and Key identify what the card is. Month is retained for the
	// selected-month evidence tests; a category card leaves it empty and names
	// its code in Key.
	Section   string
	Key       string
	Month     string
	Row       int
	Page      int
	X         float64
	Y         float64
	Width     float64
	Height    float64
	FullWidth bool
}

type cardMetricPlan struct {
	Label      string
	ValueLines []string
	Height     float64
}

type cardEvidencePlan struct {
	Lines     []string
	Estimated bool
	Height    float64
}

type comparisonCardPlan struct {
	Key      string
	Month    string
	Heading  string
	Scope    []string
	Metrics  []cardMetricPlan
	Note     []string
	Evidence []cardEvidencePlan
	Height   float64
}

func (plan comparisonCardPlan) measuredHeight() float64 {
	height := 2*cardPad + 12.5 + float64(len(plan.Scope))*cardScopeLine + 3
	for _, metric := range plan.Metrics {
		height += metric.Height
	}
	height += 3 + float64(len(plan.Note))*cardNoteLine + 2
	for _, evidence := range plan.Evidence {
		height += evidence.Height
	}
	return height
}

func (r *reportRenderer) titleBlockHeight(subtitle string) float64 {
	height := 16.0 + 6.0
	if subtitle != "" {
		height += float64(len(r.faces.wrap(subtitle, 7.2, contentWidth, false))) * 9.5
	}
	return height
}

func (r *reportRenderer) monthCardScope(month resolvedMonth) string {
	return fmt.Sprintf("%s  ·  %s  ·  As of %s  ·  Snapshot %s",
		categoryFilterLabel(month.Source.Filter.Category),
		month.Source.Filter.Currency,
		month.Source.AsOf,
		generatedLabel(month.Source.GeneratedAt))
}

func (r *reportRenderer) selectedMonthsScope() string {
	months := r.resolved.Months
	first, last := months[0], months[len(months)-1]
	rangeText := monthHeading(first.Month)
	if first.Month != last.Month {
		rangeText += " – " + monthHeading(last.Month)
	}
	var snapshots []string
	seen := make(map[string]struct{}, len(months))
	for _, month := range months {
		label := generatedLabel(month.Source.GeneratedAt)
		if _, duplicate := seen[label]; duplicate {
			continue
		}
		seen[label] = struct{}{}
		snapshots = append(snapshots, label)
	}
	prefix := "Snapshot "
	if len(snapshots) > 1 {
		prefix = "Snapshots "
	}
	return fmt.Sprintf("%s  ·  %s  ·  %s  ·  As of %s  ·  %s%s",
		rangeText,
		categoryFilterLabel(first.Source.Filter.Category),
		first.Source.Filter.Currency,
		first.Source.AsOf,
		prefix,
		strings.Join(snapshots, " / "))
}

// monthPrimary selects the six supplied figures for one card. It moves no
// money: a dashed value is the absence of evidence, never a zero.
func (r *reportRenderer) monthPrimary(month resolvedMonth) (*poreport.Metrics, string) {
	switch {
	case month.IsCurrent:
		if month.Summary == nil {
			return nil, ""
		}
		actual := month.Summary.Actual
		return &actual, "Actual to date (month-to-date). Actual figures only; future-dated records and estimates are separate lines below and are never added to these figures."
	case month.IsFuture:
		if month.Period != nil && month.Period.PoCount > 0 {
			recorded := month.Period.Metrics
			return &recorded, "Recorded evidence in the selected future month. The full-month forecast is a separate alternative below and is never added to it."
		}
		return nil, "No recorded purchase orders in this future month. The full-month forecast is shown separately below as an estimate, never as actual zero."
	default:
		if month.Period != nil {
			recorded := month.Period.Metrics
			return &recorded, "Recorded figures for the selected month. A zero row is a loaded month with no purchase orders, not missing data."
		}
		return nil, ""
	}
}

func (r *reportRenderer) planMonthCard(month resolvedMonth, width float64, heading string) comparisonCardPlan {
	inner := width - 2*cardPad
	plan := comparisonCardPlan{Key: month.Month, Month: month.Month, Heading: heading}
	plan.Scope = r.faces.wrapCell(r.monthCardScope(month), cardScopeSize, inner, false)

	primary, note := r.monthPrimary(month)
	values := []string{"-", "-", "-", "-", "-", "-"}
	if primary != nil {
		values = []string{
			formatCount(primary.PoCount),
			r.money(primary.AmountMinor),
			r.money(primary.CostedMinor),
			r.money(primary.CapitalMinor),
			r.signedMoney(primary.MarginMinor),
			formatPct(primary.MarginBps),
		}
	}
	labels := []string{"POs", "PO amount", "Costed amount", "Capital", "PO margin", "PO margin %"}
	plan.Metrics = make([]cardMetricPlan, 0, len(labels))
	for index, label := range labels {
		lines := r.faces.wrapCell(values[index], cardValueSize, inner-cardLabelWidth, false)
		if len(lines) == 0 {
			lines = []string{""}
		}
		plan.Metrics = append(plan.Metrics, cardMetricPlan{
			Label:      label,
			ValueLines: lines,
			Height:     float64(len(lines))*cardLineHeight + 2.4,
		})
	}
	plan.Note = r.faces.wrapCell(note, cardNoteSize, inner, false)
	for _, item := range r.monthEvidenceItems(month) {
		prefix := item.label
		if item.estimated {
			prefix = "Estimated (not actual) — " + prefix
		}
		lines := r.faces.wrapCell(prefix+": "+item.body, cardNoteSize, inner, false)
		plan.Evidence = append(plan.Evidence, cardEvidencePlan{
			Lines:     lines,
			Estimated: item.estimated,
			Height:    float64(len(lines))*cardNoteLine + cardEvidencePad,
		})
	}
	plan.Height = plan.measuredHeight()
	return plan
}

func (r *reportRenderer) drawIndividualMonths() {
	months := r.resolved.Months
	if len(months) == 1 {
		plan := r.planMonthCard(months[0], contentWidth, "By period — "+monthHeading(months[0].Month))
		r.canvas.ensure(plan.Height)
		top := r.canvas.y
		r.drawComparisonCard(plan, marginX, top, contentWidth, plan.Height, 0, true, cardSectionPeriod)
		r.canvas.y = top + plan.Height + cardGap
		return
	}

	title := fmt.Sprintf("By period — %d selected months", len(months))
	subtitle := r.selectedMonthsScope()
	half := (contentWidth - cardGap) / 2
	plans := make([]comparisonCardPlan, len(months))
	for index, month := range months {
		plans[index] = r.planMonthCard(month, half, monthHeading(month.Month))
	}
	r.drawComparisonCardGrid(title, subtitle, cardSectionPeriod, plans,
		func(index int, width float64) comparisonCardPlan {
			return r.planMonthCard(months[index], width, monthHeading(months[index].Month))
		})
}

// drawComparisonCardGrid is the one comparison layout engine: at most two cards
// per row, a final odd card spanning the content width, the section title kept
// with its first row, and each pair or full-width card kept on one page with the
// section context repeated when the grid continues. planAt re-plans a card at a
// new width, which is what makes the odd final card honest at full width.
func (r *reportRenderer) drawComparisonCardGrid(
	title, subtitle, section string,
	plans []comparisonCardPlan,
	planAt func(index int, width float64) comparisonCardPlan,
) {
	if len(plans) == 0 {
		return
	}
	half := (contentWidth - cardGap) / 2
	firstHeight := plans[0].Height
	if len(plans) > 1 {
		firstHeight = math.Max(plans[0].Height, plans[1].Height)
	}
	// Keep the section title and its first card row together: a heading alone at
	// the foot of a page is the orphan #641 F3 showed, not a grid.
	r.canvas.ensure(r.titleBlockHeight(subtitle) + firstHeight)
	r.sectionTitle(title, subtitle)

	row := 0
	for index := 0; index < len(plans); {
		if len(plans)-index == 1 {
			plan := planAt(index, contentWidth)
			if r.canvas.y+plan.Height > contentBot {
				r.newComparisonPage(title, subtitle)
			}
			top := r.canvas.y
			r.drawComparisonCard(plan, marginX, top, contentWidth, plan.Height, row, true, section)
			r.canvas.y = top + plan.Height + cardGap
			index++
			row++
			continue
		}
		left, right := plans[index], plans[index+1]
		height := math.Max(left.Height, right.Height)
		if r.canvas.y+height > contentBot {
			r.newComparisonPage(title, subtitle)
		}
		top := r.canvas.y
		r.drawComparisonCard(left, marginX, top, half, height, row, false, section)
		r.drawComparisonCard(right, marginX+half+cardGap, top, half, height, row, false, section)
		r.canvas.y = top + height + cardGap
		index += 2
		row++
	}
}

func (r *reportRenderer) newComparisonPage(title, subtitle string) {
	r.canvas.newPage()
	r.sectionTitle(title+" (continued)", subtitle)
}

func (r *reportRenderer) drawComparisonCard(plan comparisonCardPlan, x, y, width, surfaceHeight float64, row int, fullWidth bool, section string) {
	c := r.canvas
	c.rect(x, y, width, surfaceHeight, &r.pal.surface, &r.pal.edge, 0.5)
	inner := width - 2*cardPad

	headingSize := fittedSize(r.faces.width, plan.Heading, inner, 10, 6.2, true)
	c.text(plan.Heading, x+cardPad, y+cardPad+9.5, headingSize, true, r.pal.emphasis)
	cursor := y + cardPad + 12.5
	for _, line := range plan.Scope {
		c.text(line, x+cardPad, cursor+6.2, cardScopeSize, false, r.pal.muted)
		cursor += cardScopeLine
	}
	cursor += 3
	for _, metric := range plan.Metrics {
		c.text(metric.Label, x+cardPad, cursor+7.4, cardLabelSize, false, r.pal.secondary)
		for index, line := range metric.ValueLines {
			c.textRight(line, x+width-cardPad, cursor+7.4+float64(index)*cardLineHeight,
				cardValueSize, false, r.pal.emphasis)
		}
		cursor += metric.Height
	}
	cursor += 3
	for _, line := range plan.Note {
		c.text(line, x+cardPad, cursor+6.2, cardNoteSize, false, r.pal.secondary)
		cursor += cardNoteLine
	}
	cursor += 2
	for _, evidence := range plan.Evidence {
		if evidence.Estimated {
			band := r.pal.band
			c.rect(x+4, cursor-1, inner, evidence.Height-1, &band, nil, 0)
		}
		for index, line := range evidence.Lines {
			c.text(line, x+cardPad, cursor+6.2+float64(index)*cardNoteLine,
				cardNoteSize, false, r.pal.muted)
		}
		cursor += evidence.Height
	}

	box := cardBox{
		Section:   section,
		Key:       plan.Key,
		Month:     plan.Month,
		Row:       row,
		Page:      len(c.pages) - 1,
		X:         x,
		Y:         y,
		Width:     width,
		Height:    surfaceHeight,
		FullWidth: fullWidth,
	}
	if section == cardSectionCategory {
		r.categoryCards = append(r.categoryCards, box)
		return
	}
	r.cards = append(r.cards, box)
}

func (r *reportRenderer) drawAggregateMonths() {
	aggregate := r.resolved.Aggregate
	if aggregate == nil {
		return
	}
	title := "By period — all months in " + aggregate.Year
	columns := aggregateTableColumns()
	rows := make([]tableRow, 0, len(aggregate.Months))
	for index, month := range aggregate.Months {
		var fill *rgb
		if index%2 == 1 {
			surface := r.pal.surface
			fill = &surface
		}
		label := aggregateMonthLabel(month)
		cells := []string{label, "-", "-", "-", "-", "-", "-"}
		if month.Recorded && month.Row != nil {
			metrics := month.Row.Metrics
			cells = []string{
				label,
				formatCount(metrics.PoCount),
				r.money(metrics.AmountMinor),
				r.money(metrics.CostedMinor),
				r.money(metrics.CapitalMinor),
				r.signedMoney(metrics.MarginMinor),
				formatPct(metrics.MarginBps),
			}
		}
		rows = append(rows, tableRow{fill: fill, cells: cells})
	}
	// The one total is the annual server snapshot's totals, labelled for what it
	// is. No row is summed, and no estimate enters it.
	total := r.totalRowFor(aggregate.Source, "Recorded total")
	// Keep the table title, note, header and several rows together rather than
	// leaving a lone month row at the foot of the previous section.
	r.canvas.ensure(170)
	r.drawTable(title, r.aggregateNote(aggregate), columns, rows, &total)
	r.drawAggregateForecasts(aggregate)
}

// aggregateTableColumns keeps the original six-metric comparison table. The
// month column is wide enough for a recorded/no-record marker; the money
// columns stay wide and wrap an exact maximum amount rather than shrinking it.
func aggregateTableColumns() []tableColumn {
	return []tableColumn{
		{title: "MONTH", width: 104, align: alignLeft},
		{title: "POs", width: 28, align: alignRight},
		{title: "AMOUNT", width: 80, align: alignRight},
		{title: "COSTED", width: 80, align: alignRight},
		{title: "CAPITAL", width: 78, align: alignRight},
		{title: "MARGIN", width: 78, align: alignRight},
		{title: "MARGIN %", width: contentWidth - 448, align: alignRight},
	}
}

func aggregateMonthLabel(month resolvedAggregateMonth) string {
	label := month.Month
	if parsed, err := time.Parse("2006-01", month.Month); err == nil {
		label = parsed.Format("Jan")
	}
	switch {
	case month.IsFuture && month.Recorded:
		return label + " (recorded)"
	case month.IsFuture:
		return label + " (no recorded POs)"
	case month.IsCurrent:
		return label + " (recorded)"
	default:
		return label
	}
}

func (r *reportRenderer) aggregateNote(aggregate *resolvedAggregate) string {
	first := aggregate.Months[0].Month
	last := aggregate.Months[len(aggregate.Months)-1].Month
	note := fmt.Sprintf("%s – %s  ·  %s  ·  %s  ·  As of %s  ·  Snapshot %s. ",
		monthHeading(first),
		monthHeading(last),
		categoryFilterLabel(aggregate.Source.Filter.Category),
		aggregate.Source.Filter.Currency,
		aggregate.Source.AsOf,
		generatedLabel(aggregate.Source.GeneratedAt))
	note += "Recorded month rows come from this annual snapshot. The Recorded total is the server's annual total, not a sum of the printed rows; no forecast or estimate is included in this total."
	for _, month := range aggregate.Months {
		if month.IsCurrent {
			note += " The current month row is the annual snapshot's recorded row, not an actual-to-date readout."
			break
		}
	}
	if aggregate.IncludesFutureDated {
		note += " Future-dated records are included as recorded evidence, never as a forecast."
	}
	return note
}

// drawAggregateForecasts prints estimates or stable unavailable reasons after
// the recorded table. Nothing here is added to the table or its Recorded total.
func (r *reportRenderer) drawAggregateForecasts(aggregate *resolvedAggregate) {
	future := make([]resolvedAggregateMonth, 0, len(aggregate.Months))
	for _, month := range aggregate.Months {
		if month.IsFuture {
			future = append(future, month)
		}
	}
	if len(future) == 0 {
		return
	}
	currentYear := firstSeven(r.resolved.Overview.AsOf)[:4]
	if future[0].Month[:4] != currentYear && len(future) == len(aggregate.Months) {
		r.drawEvidenceLine("Forecast unavailable for "+aggregate.Year,
			"Future months beyond the current year have no current-year forecast. No retired projection is used as a fallback.",
			false)
		return
	}
	for _, month := range future {
		item, ok := r.monthForecastEvidence(resolvedMonth{
			Month: month.Month, Source: aggregate.Source, IsFuture: true,
		})
		if !ok {
			continue
		}
		r.drawEvidenceLine(item.label+" ("+monthHeading(month.Month)+")", item.body, item.estimated)
	}
}
