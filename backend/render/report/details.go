package report

import (
	"fmt"
	"strings"
	"time"

	"vera-app/backend/poreport"
)

// drawProjectionSummary replaces the retired monthly/yearly projection tables.
// A reader needs to know what the ghost on the chart is, how far it reaches and
// what sample produced it; the old two tables repeated every projected bucket
// and made a forecast look like a second set of actuals.
func (r *reportRenderer) drawProjectionSummary() {
	current := r.snapshot.CurrentProjection
	r.sectionTitle("Current-period projection",
		"A forecast for the period still in progress. It is drawn as a hatched ghost on the chart and never enters an actual table or total.")

	if currentProjectionAbsent(current) {
		r.paragraph("Status", "No current-period projection is available in this snapshot.")
		return
	}

	c := r.canvas
	c.ensure(22)
	c.text(projectionModeText(current), marginX, c.y+10, 7.6, true, r.pal.emphasis)
	statusColor := r.pal.muted
	if current.Status == poreport.ProjectionStatusAvailable {
		statusColor = r.pal.accent
	}
	c.textRight(projectionStatusLabel(current.Status), marginX+contentWidth, c.y+10, 7, true, statusColor)
	c.y += 18

	if current.Status == poreport.ProjectionStatusAvailable {
		r.drawCurrentProjectionFigures(current)
	} else {
		r.reasonBox(current.Reason)
	}

	r.paragraph("Method", current.Method)
	r.paragraph("Basis", currentProjectionBasisText(current))
	if sources := currentProjectionSources(current); sources != "" {
		r.paragraph("Sources", sources)
	}
	r.paragraph("Updates", "This forecast updates as data arrives. Projected figures never enter an actual table or total.")
	c.y += 6
}

// projectionModeText names the period the forecast belongs to using the
// snapshot's AsOf date, not the machine's clock.
func projectionModeText(current poreport.CurrentProjection) string {
	asOf, err := time.Parse(time.DateOnly, current.Basis.AsOf)
	switch current.Mode {
	case poreport.ProjectionModeDay:
		if err == nil {
			return "Mode: Day  ·  " + asOf.Format("January 2006") + " day by day"
		}
		return "Mode: Day  ·  current month day by day"
	case poreport.ProjectionModeMonth:
		if err == nil {
			return fmt.Sprintf("Mode: Month  ·  %d month by month", asOf.Year())
		}
		return "Mode: Month  ·  current year month by month"
	case poreport.ProjectionModeNone:
		return "Mode: None  ·  not the current month or year"
	default:
		return "Mode: " + current.Mode
	}
}

func (r *reportRenderer) drawCurrentProjectionFigures(current poreport.CurrentProjection) {
	switch current.Mode {
	case poreport.ProjectionModeDay:
		r.drawDayProjectionFigures(current)
	case poreport.ProjectionModeMonth:
		r.drawMonthProjectionFigures(current)
	}
}

func (r *reportRenderer) drawDayProjectionFigures(current poreport.CurrentProjection) {
	if len(current.Buckets) == 0 {
		r.paragraph("Remaining days", "No days remain in the current month.")
		return
	}
	first := current.Buckets[0]
	last := current.Buckets[len(current.Buckets)-1]
	r.paragraph("Remaining days", fmt.Sprintf("%s  ·  %d %s",
		daySpanLabel(first.PeriodStart, last.PeriodStart),
		len(current.Buckets), plural(len(current.Buckets), "day", "days")))
	if sameProjectedFigures(current.Buckets) {
		r.paragraph("Per remaining day", r.projectedFigures(first))
	} else {
		r.paragraph("Per remaining day", "Each remaining day carries its own low, base and high on the chart.")
	}
}

func (r *reportRenderer) drawMonthProjectionFigures(current poreport.CurrentProjection) {
	if current.CurrentMonth != nil {
		actual := current.CurrentMonth.Actual
		r.paragraph("Month to date", fmt.Sprintf("%s amount  ·  %s capital  ·  %s margin",
			r.money(actual.AmountMinor), r.money(actual.CapitalMinor), r.signedMoney(actual.MarginMinor)))
		remainder := current.CurrentMonth.Remainder
		r.paragraph("Current month remainder", fmt.Sprintf("low %s  ·  base %s  ·  high %s  ·  capital %s  ·  margin %s",
			r.money(remainder.LowMinor), r.money(remainder.BaseMinor), r.money(remainder.HighMinor),
			r.money(remainder.CapitalMinor), r.signedMoney(remainder.MarginMinor)))
	}
	if len(current.Buckets) == 0 {
		r.paragraph("Future months", "No months remain in the current year.")
		return
	}
	first := current.Buckets[0]
	last := current.Buckets[len(current.Buckets)-1]
	r.paragraph("Future months", fmt.Sprintf("%s  ·  %d ghost %s",
		monthSpanLabel(first.PeriodStart, last.PeriodStart),
		len(current.Buckets), plural(len(current.Buckets), "bar", "bars")))
	if sameProjectedFigures(current.Buckets) {
		r.paragraph("Per future month", r.projectedFigures(first))
	} else {
		r.paragraph("Per future month", "Each future month carries its own low, base and high on the chart.")
	}
}

func daySpanLabel(first, last string) string {
	if first == last {
		return humanDate(first)
	}
	return humanDate(first) + " to " + humanDate(last)
}

func monthSpanLabel(first, last string) string {
	if first == last {
		return monthYear(first)
	}
	return monthYear(first) + " to " + monthYear(last)
}

func (r *reportRenderer) projectedFigures(bucket poreport.ProjectedBucket) string {
	return fmt.Sprintf("low %s  ·  base %s  ·  high %s  ·  capital %s  ·  margin %s",
		r.money(bucket.AmountLowMinor), r.money(bucket.AmountBaseMinor), r.money(bucket.AmountHighMinor),
		r.money(bucket.CapitalMinor), r.signedMoney(bucket.MarginMinor))
}

// sameProjectedFigures is the summary's honesty check: it prints one
// low/base/high line only when every remaining bucket actually carries that
// line. When a supplied snapshot varies the buckets, the reader is sent to the
// chart instead of being shown the first bucket as if it spoke for all.
func sameProjectedFigures(buckets []poreport.ProjectedBucket) bool {
	if len(buckets) < 2 {
		return true
	}
	first := buckets[0]
	for _, bucket := range buckets[1:] {
		if bucket.AmountLowMinor != first.AmountLowMinor ||
			bucket.AmountBaseMinor != first.AmountBaseMinor ||
			bucket.AmountHighMinor != first.AmountHighMinor ||
			bucket.CapitalMinor != first.CapitalMinor ||
			bucket.MarginMinor != first.MarginMinor {
			return false
		}
	}
	return true
}

func currentProjectionBasisText(current poreport.CurrentProjection) string {
	basis := current.Basis
	asOf := humanDate(basis.AsOf)
	if asOf == "" {
		asOf = "not stated"
	}
	if basis.From == "" || basis.To == "" {
		return fmt.Sprintf("No sample window is stated; as of %s.", asOf)
	}
	unit := "elapsed days"
	if current.Mode == poreport.ProjectionModeMonth {
		unit = "complete months"
	}
	return fmt.Sprintf("%s to %s (exclusive)  ·  %d %s  ·  %d %s  ·  as of %s.",
		humanDate(basis.From), humanDate(basis.To), basis.Units, unit,
		basis.PoCount, plural(int(basis.PoCount), "purchase order", "purchase orders"), asOf)
}

func currentProjectionSources(current poreport.CurrentProjection) string {
	seen := map[string]bool{}
	var labels []string
	add := func(source string) {
		if source == "" || seen[source] {
			return
		}
		seen[source] = true
		labels = append(labels, projectionSourceLabel(source)+" ["+source+"]")
	}
	for _, bucket := range current.Buckets {
		add(bucket.Source)
	}
	if current.CurrentMonth != nil {
		add(current.CurrentMonth.Remainder.Source)
	}
	return strings.Join(labels, "; ")
}

func (r *reportRenderer) reasonBox(reason string) {
	c := r.canvas
	lines := r.faces.wrap(projectionReasonLabel(reason), 7.4, contentWidth-24, false)
	height := float64(len(lines))*10 + 18
	c.ensure(height + 4)
	c.rect(marginX, c.y, contentWidth, height, &r.pal.surface, &r.pal.edge, 0.5)
	c.text("REASON  "+reason, marginX+10, c.y+13, 6.4, true, r.pal.muted)
	baseline := c.y + 25
	for _, line := range lines {
		c.text(line, marginX+10, baseline, 7.4, false, r.pal.secondary)
		baseline += 10
	}
	c.y += height + 8
}

func (r *reportRenderer) paragraph(label, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	c := r.canvas
	prefix := label + ": "
	prefixWidth := r.faces.width(prefix, 7.3, true)
	drawnPrefix := strings.TrimRight(prefix, " ")
	if prefixWidth > 70 {
		// A long label gets its own line. Capping the indent at 70 without
		// shortening the drawn label put the value underneath its own heading,
		// which is what "Current month remainder" did before this branch.
		lines := r.faces.wrap(text, 7.3, contentWidth, false)
		c.ensure(10 + float64(len(lines))*10 + 4)
		c.text(drawnPrefix, marginX, c.y+8, 7.3, true, r.pal.emphasis)
		c.y += 10
		for _, line := range lines {
			c.text(line, marginX, c.y+8, 7.3, false, r.pal.secondary)
			c.y += 10
		}
		c.y += 3
		return
	}
	lines := r.faces.wrap(text, 7.3, contentWidth-prefixWidth, false)
	if len(lines) == 0 {
		return
	}
	c.ensure(float64(len(lines))*10 + 6)
	c.text(drawnPrefix, marginX, c.y+8, 7.3, true, r.pal.emphasis)
	for _, line := range lines {
		c.text(line, marginX+prefixWidth, c.y+8, 7.3, false, r.pal.secondary)
		c.y += 10
	}
	c.y += 3
}
