package report

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"vera-app/backend/poreport"
)

const (
	periodsPerChartPanel = 18
	chartPanelHeight     = 198.0
	chartPlotHeight      = 112.0
	chartTitleHeight     = 22.0
)

func (r *reportRenderer) drawLegend() {
	c := r.canvas
	pal := r.pal
	c.ensure(30)
	x := marginX
	y := c.y + 8
	c.text("KEY", x, y, 6.2, true, pal.muted)
	x += 24

	item := func(width float64, draw func(float64)) {
		if x+width > marginX+contentWidth {
			x = marginX
			y += 12
		}
		draw(x)
		x += width
	}
	box := func(color rgb, label string) {
		width := 12 + c.faces.width(label, 7, false) + 12
		item(width, func(at float64) {
			c.rect(at, y-6.5, 7, 7, &color, nil, 0)
			c.text(label, at+10, y, 7, false, pal.secondary)
		})
	}
	box(pal.amount, "Amount")
	box(pal.capital, "Capital")
	box(pal.marginPos, "Margin")
	if r.hasNegativeMargin() {
		box(pal.marginNeg, "Negative margin")
	}
	lineLabel := "Margin % (right axis)"
	item(20+c.faces.width(lineLabel, 7, false)+12, func(at float64) {
		c.line(at, y-3, at+15, y-3, pal.marginLine, 1.4)
		c.text(lineLabel, at+19, y, 7, false, pal.secondary)
	})
	hatchLabel := "Uncosted part of amount"
	item(12+c.faces.width(hatchLabel, 7, false)+12, func(at float64) {
		c.hatch(at, y-7, 7, 7, 2.2, pal.amount, 0.5)
		c.text(hatchLabel, at+10, y, 7, false, pal.secondary)
	})
	if _, ok := currentChartBuckets(r.snapshot); ok {
		ghostLabel := "Projected (ghost)"
		item(12+c.faces.width(ghostLabel, 7, false)+12, func(at float64) {
			band := pal.band
			c.rect(at, y-7, 7, 7, &band, nil, 0)
			c.hatch(at, y-7, 7, 7, 2.2, pal.amount, 0.4)
			c.text(ghostLabel, at+10, y, 7, false, pal.secondary)
		})
		rangeLabel := "Projected low-high"
		item(12+c.faces.width(rangeLabel, 7, false)+12, func(at float64) {
			c.whisker(at+4, y-8, y+1, pal.secondary, 0.8)
			c.text(rangeLabel, at+10, y, 7, false, pal.secondary)
		})
	}
	c.y = y + 8
}

func (r *reportRenderer) hasNegativeMargin() bool {
	for _, period := range r.snapshot.Periods {
		if period.MarginMinor < 0 {
			return true
		}
	}
	if r.snapshot.Totals.MarginMinor < 0 {
		return true
	}
	for _, category := range r.snapshot.Categories {
		if category.MarginMinor < 0 {
			return true
		}
	}
	return false
}

func (r *reportRenderer) drawActualChartPanels() {
	periods := r.snapshot.Periods
	total := (len(periods) + periodsPerChartPanel - 1) / periodsPerChartPanel
	for start := 0; start < len(periods); start += periodsPerChartPanel {
		end := start + periodsPerChartPanel
		if end > len(periods) {
			end = len(periods)
		}
		index := start/periodsPerChartPanel + 1
		title := "Actual periods"
		if total > 1 {
			title = fmt.Sprintf("Actual periods  ·  %d of %d", index, total)
		}
		r.drawActualChartPanel(title, periods[start:end])
	}
}

func (r *reportRenderer) drawActualChartPanel(title string, periods []poreport.Period) {
	c := r.canvas
	pal := r.pal
	c.ensure(chartPanelHeight)
	top := c.y
	c.text(title, marginX, top+10, 8.8, true, pal.emphasis)

	plotX := marginX + 48
	plotRight := marginX + contentWidth - 54
	plotW := plotRight - plotX
	plotY := top + chartTitleHeight + 6
	plotH := chartPlotHeight

	minValue, maxValue := chartExtent(periods)
	scale := NiceScale(minValue, maxValue, 5)
	zeroY := scaleY(scale, 0, plotY, plotH)
	valueY := func(value float64) float64 { return scaleY(scale, value, plotY, plotH) }

	for _, tick := range scale.Ticks {
		y := valueY(tick)
		if y < plotY-0.1 || y > plotY+plotH+0.1 {
			continue
		}
		color := pal.rule
		lw := 0.3
		if tick == 0 {
			color = pal.secondary
			lw = 0.9
		}
		c.line(plotX, y, plotRight, y, color, lw)
		c.textRight(compactMoney(r.currency(), tick), plotX-5, y+2.2, 5.9, false, pal.muted)
	}
	c.line(plotX, plotY+plotH, plotRight, plotY+plotH, pal.edge, 0.5)

	groupW := plotW / float64(len(periods))
	barW := math.Min(24, math.Max(3, groupW*0.18))
	for index, period := range periods {
		center := plotX + (float64(index)+0.5)*groupW
		amountX := center - barW*1.25
		capitalX := center
		marginX := center + barW*1.25

		r.drawColumn(amountX, barW, float64(period.AmountMinor), zeroY, valueY, pal.amount)
		if period.UncostedMinor > 0 && period.AmountMinor > 0 {
			costed := period.AmountMinor - period.UncostedMinor
			if costed < 0 {
				costed = 0
			}
			topY := valueY(float64(period.AmountMinor))
			costedY := valueY(float64(costed))
			height := costedY - topY
			if height > 0.5 {
				c.hatch(amountX, topY, barW, height, 3.0, pal.paper, 0.7)
			}
		}
		r.drawColumn(capitalX, barW, float64(period.CapitalMinor), zeroY, valueY, pal.capital)
		marginColor := pal.marginPos
		if period.MarginMinor < 0 {
			marginColor = pal.marginNeg
		}
		r.drawColumn(marginX, barW, float64(period.MarginMinor), zeroY, valueY, marginColor)
	}

	r.drawMarginLine(periods, plotX, plotRight, plotY, plotH, groupW)
	r.drawChartPeriodLabels(periods, plotX, plotY+plotH, groupW)

	c.y = plotY + plotH + 20
}

// drawColumn draws one money column from the zero baseline. A negative value
// grows downward, which is what lets a discounted-below-cost month read as a
// loss rather than as a small positive bar.
func (r *reportRenderer) drawColumn(x, width, value, zeroY float64, valueY func(float64) float64, color rgb) {
	if value == 0 {
		return
	}
	y := valueY(value)
	top := math.Min(y, zeroY)
	height := math.Abs(y - zeroY)
	if height < 0.4 {
		return
	}
	r.canvas.rect(x, top, width, height, &color, nil, 0)
}

func (r *reportRenderer) drawMarginLine(periods []poreport.Period, plotX, plotRight, plotY, plotH, groupW float64) {
	pal := r.pal
	var values []float64
	for _, period := range periods {
		if period.MarginBps != nil {
			values = append(values, float64(*period.MarginBps)/100)
		}
	}
	if len(values) == 0 {
		r.canvas.text("margin % n/a", plotRight+5, plotY+plotH/2, 5.8, false, pal.muted)
		return
	}
	scale := NiceScale(minFloat(values), maxFloat(values), 4)
	scaleYFor := func(value float64) float64 {
		if scale.Max == scale.Min {
			return plotY + plotH
		}
		return plotY + plotH - (value-scale.Min)/(scale.Max-scale.Min)*plotH
	}
	for _, tick := range scale.Ticks {
		y := scaleYFor(tick)
		if y < plotY-0.1 || y > plotY+plotH+0.1 {
			continue
		}
		r.canvas.text(percentTick(tick), plotRight+5, y+2.2, 5.9, false, pal.muted)
	}

	var previousIndex = -10
	var previousY float64
	var lastX, lastY float64
	lastKnown := -1
	for index, period := range periods {
		if period.MarginBps == nil {
			previousIndex = -10
			continue
		}
		value := float64(*period.MarginBps) / 100
		x := plotX + (float64(index)+0.5)*groupW
		y := scaleYFor(value)
		if index == previousIndex+1 {
			r.canvas.line(x-groupW, previousY, x, y, pal.marginLine, 1.4)
		}
		r.canvas.rect(x-1.2, y-1.2, 2.4, 2.4, &pal.marginLine, nil, 0)
		previousIndex = index
		previousY = y
		lastX, lastY = x, y
		lastKnown = index
	}
	if lastKnown >= 0 {
		label := percentTick(float64(*periods[lastKnown].MarginBps) / 100)
		labelX := lastX + 3
		if labelX+r.canvas.faces.width(label, 6.2, true) > plotRight {
			labelX = lastX - r.canvas.faces.width(label, 6.2, true) - 3
		}
		r.canvas.text(label, labelX, lastY-3, 6.2, true, pal.marginLine)
	}
}

func (r *reportRenderer) drawChartPeriodLabels(periods []poreport.Period, plotX, baseline, groupW float64) {
	if len(periods) == 0 {
		return
	}
	// A day-bucketed month packs 31 groups into the plot. Printing every date
	// at five points would overlap; thin the labels and always keep the last
	// day, which is the boundary a reader looks for.
	step := 1
	if groupW < 22 {
		step = 2
	}
	if groupW < 12 {
		step = 3
	}
	label := func(index int, period poreport.Period) {
		text := periodLabel(period.PeriodStart, r.snapshot.Filter.Bucket)
		size := fittedSize(r.canvas.faces.width, text, groupW*float64(step)-2, 6.2, 5, false)
		center := plotX + (float64(index)+0.5)*groupW
		r.canvas.textCenter(text, center, baseline+11, size, false, r.pal.secondary)
	}
	drawn := make([]int, 0, len(periods)/step+1)
	for index := 0; index < len(periods); index += step {
		drawn = append(drawn, index)
	}
	if last := len(periods) - 1; last%step != 0 {
		// Keep the final label, but drop a previous one that would sit closer
		// than one step. Without this a 30-day November printed 29 and 30 on
		// top of each other.
		if count := len(drawn); count > 0 && last-drawn[count-1] < step {
			drawn = drawn[:count-1]
		}
		drawn = append(drawn, last)
	}
	for _, index := range drawn {
		label(index, periods[index])
	}
}

// currentChartBucket is one column group of the combined current-period
// chart. An actual bucket carries evidence; a ghost bucket carries a forecast.
// The current month of a year report carries both: its actual columns and the
// stacked ghost remainder for the rest of the month.
type currentChartBucket struct {
	start     string
	actual    *poreport.Metrics
	ghost     *poreport.ProjectedBucket
	remainder *poreport.CurrentMonthProjection
}

// currentChartBuckets builds the ordered groups the ghost chart draws. It
// returns false when there is no available current-period forecast or no
// actual period to anchor it: a ghost with no actuals behind it is not a chart,
// it is an invented report.
func currentChartBuckets(snapshot poreport.Report) ([]currentChartBucket, bool) {
	current := snapshot.CurrentProjection
	if current.Status != poreport.ProjectionStatusAvailable {
		return nil, false
	}
	if current.Mode != poreport.ProjectionModeDay && current.Mode != poreport.ProjectionModeMonth {
		return nil, false
	}
	// The ghost is drawn at the report's own resolution. A day forecast on a
	// month-bucketed report (or a month forecast on a quarterly one) would mix
	// two axis meanings, so those reports keep their actual chart and the
	// summary alone states the forecast.
	if current.Mode == poreport.ProjectionModeDay && snapshot.Filter.Bucket != poreport.BucketDay {
		return nil, false
	}
	if current.Mode == poreport.ProjectionModeMonth && snapshot.Filter.Bucket != poreport.BucketMonth {
		return nil, false
	}
	if len(snapshot.Periods) == 0 {
		return nil, false
	}

	actualByStart := make(map[string]*poreport.Metrics, len(snapshot.Periods))
	for index := range snapshot.Periods {
		actualByStart[snapshot.Periods[index].PeriodStart] = &snapshot.Periods[index].Metrics
	}
	ghostByStart := make(map[string]*poreport.ProjectedBucket, len(current.Buckets))
	for index := range current.Buckets {
		ghostByStart[current.Buckets[index].PeriodStart] = &current.Buckets[index]
	}

	currentMonthStart := ""
	if current.Mode == poreport.ProjectionModeMonth && current.CurrentMonth != nil {
		asOfValue := current.Basis.AsOf
		if asOfValue == "" {
			asOfValue = snapshot.AsOf
		}
		if asOf, err := time.Parse(time.DateOnly, asOfValue); err == nil {
			currentMonthStart = firstOfMonthDate(asOf).Format(time.DateOnly)
		}
	}

	seen := make(map[string]bool, len(snapshot.Periods)+len(current.Buckets)+1)
	for _, period := range snapshot.Periods {
		seen[period.PeriodStart] = true
	}
	for _, bucket := range current.Buckets {
		seen[bucket.PeriodStart] = true
	}
	if currentMonthStart != "" {
		seen[currentMonthStart] = true
	}
	starts := make([]string, 0, len(seen))
	for start := range seen {
		starts = append(starts, start)
	}
	sort.Strings(starts)

	buckets := make([]currentChartBucket, 0, len(starts))
	for _, start := range starts {
		item := currentChartBucket{start: start}
		if actual := actualByStart[start]; actual != nil {
			item.actual = actual
		}
		if ghost := ghostByStart[start]; ghost != nil {
			// A forecast for a bucket replaces that bucket's zero actual
			// placeholder. The actual table still carries the placeholder row;
			// the chart must not draw an empty actual column under a ghost.
			item.actual = nil
			item.ghost = ghost
		}
		if start == currentMonthStart {
			item.remainder = current.CurrentMonth
			if item.actual == nil {
				actual := current.CurrentMonth.Actual
				item.actual = &actual
			}
		}
		if item.actual != nil || item.ghost != nil || item.remainder != nil {
			buckets = append(buckets, item)
		}
	}
	return buckets, len(buckets) > 0
}

func (r *reportRenderer) drawCurrentChartPanel(buckets []currentChartBucket) {
	c := r.canvas
	pal := r.pal
	c.ensure(chartPanelHeight)
	top := c.y
	title := "Actual and projected periods"
	subtitle := "Hatched columns are forecasts; the low-high whisker bounds the amount."
	mode := r.snapshot.CurrentProjection.Mode
	if mode == poreport.ProjectionModeDay {
		title = "Actual and projected days  ·  " + monthYear(buckets[0].start)
		subtitle = "Days after the last actual day are hatched ghosts with a daily low-high amount whisker."
	} else if mode == poreport.ProjectionModeMonth {
		title = "Actual and projected months  ·  " + yearLabel(buckets[0].start)
		subtitle = "Future months are hatched ghosts; the current month stacks its actual columns with the ghost remainder."
	}
	c.text(title, marginX, top+10, 8.8, true, pal.emphasis)
	c.text(subtitle, marginX, top+20, 6.4, false, pal.muted)

	plotX := marginX + 48
	plotRight := marginX + contentWidth - 54
	plotW := plotRight - plotX
	plotY := top + chartTitleHeight + 10
	plotH := chartPlotHeight

	minValue, maxValue := currentChartExtent(buckets)
	scale := NiceScale(minValue, maxValue, 5)
	zeroY := scaleY(scale, 0, plotY, plotH)
	valueY := func(value float64) float64 { return scaleY(scale, value, plotY, plotH) }

	for _, tick := range scale.Ticks {
		y := valueY(tick)
		if y < plotY-0.1 || y > plotY+plotH+0.1 {
			continue
		}
		color := pal.rule
		lw := 0.3
		if tick == 0 {
			color = pal.secondary
			lw = 0.9
		}
		c.line(plotX, y, plotRight, y, color, lw)
		c.textRight(compactMoney(r.currency(), tick), plotX-5, y+2.2, 5.9, false, pal.muted)
	}
	c.line(plotX, plotY+plotH, plotRight, plotY+plotH, pal.edge, 0.5)

	groupW := plotW / float64(len(buckets))
	barW := math.Min(24, math.Max(2.2, groupW*0.18))
	linePeriods := make([]poreport.Period, len(buckets))
	for index, bucket := range buckets {
		center := plotX + (float64(index)+0.5)*groupW
		amountX := center - barW*1.25
		capitalX := center
		marginX := center + barW*1.25
		linePeriods[index].PeriodStart = bucket.start

		if bucket.actual != nil {
			actual := *bucket.actual
			r.drawColumn(amountX, barW, float64(actual.AmountMinor), zeroY, valueY, pal.amount)
			if actual.UncostedMinor > 0 && actual.AmountMinor > 0 {
				costed := actual.AmountMinor - actual.UncostedMinor
				if costed < 0 {
					costed = 0
				}
				topY := valueY(float64(actual.AmountMinor))
				costedY := valueY(float64(costed))
				height := costedY - topY
				if height > 0.5 {
					c.hatch(amountX, topY, barW, height, 3.0, pal.paper, 0.7)
				}
			}
			r.drawColumn(capitalX, barW, float64(actual.CapitalMinor), zeroY, valueY, pal.capital)
			marginColor := pal.marginPos
			if actual.MarginMinor < 0 {
				marginColor = pal.marginNeg
			}
			r.drawColumn(marginX, barW, float64(actual.MarginMinor), zeroY, valueY, marginColor)
			linePeriods[index] = poreport.Period{
				PeriodStart: bucket.start,
				Partial:     false,
				InProgress:  false,
				Metrics:     actual,
			}
		}
		if bucket.ghost != nil {
			ghost := *bucket.ghost
			r.drawGhostColumn(amountX, barW, 0, float64(ghost.AmountBaseMinor), valueY, pal.amount)
			r.drawGhostColumn(capitalX, barW, 0, float64(ghost.CapitalMinor), valueY, pal.capital)
			marginInk := pal.marginPos
			if ghost.MarginMinor < 0 {
				marginInk = pal.marginNeg
			}
			r.drawGhostColumn(marginX, barW, 0, float64(ghost.MarginMinor), valueY, marginInk)
			c.whisker(amountX+barW/2, valueY(float64(ghost.AmountLowMinor)),
				valueY(float64(ghost.AmountHighMinor)), pal.secondary, 0.8)
		}
		if bucket.remainder != nil {
			actual := remainderActual(bucket.remainder, bucket.actual)
			amountTotal, capitalTotal, marginTotal := remainderTotals(actual, bucket.remainder.Remainder)
			marginInk := pal.marginPos
			if marginTotal < 0 {
				marginInk = pal.marginNeg
			}
			r.drawGhostColumn(amountX, barW, float64(actual.AmountMinor), float64(amountTotal), valueY, pal.amount)
			r.drawGhostColumn(capitalX, barW, float64(actual.CapitalMinor), float64(capitalTotal), valueY, pal.capital)
			r.drawGhostColumn(marginX, barW, float64(actual.MarginMinor), float64(marginTotal), valueY, marginInk)
			c.whisker(amountX+barW/2, valueY(float64(actual.AmountMinor+bucket.remainder.Remainder.LowMinor)),
				valueY(float64(actual.AmountMinor+bucket.remainder.Remainder.HighMinor)), pal.secondary, 0.8)
		}
	}

	r.drawMarginLine(linePeriods, plotX, plotRight, plotY, plotH, groupW)
	r.drawChartPeriodLabels(linePeriods, plotX, plotY+plotH, groupW)
	c.y = plotY + plotH + 22
}

// remainderActual is the month-to-date figure a remainder stacks on. The
// period table already carries it; CurrentMonth.Actual is the fallback for a
// supplied snapshot whose periods omit the current month.
func remainderActual(current *poreport.CurrentMonthProjection, actual *poreport.Metrics) poreport.Metrics {
	if actual != nil {
		return *actual
	}
	return current.Actual
}

// remainderTotals is the stack the current month's ghost is drawn to. It is
// the only arithmetic the renderer performs on a projection, and it is a
// direct sum of two supplied figures, never a rebuilt report total.
func remainderTotals(actual poreport.Metrics, remainder poreport.CurrentProjectionRemainder) (amount, capital, margin int64) {
	return actual.AmountMinor + remainder.BaseMinor,
		actual.CapitalMinor + remainder.CapitalMinor,
		actual.MarginMinor + remainder.MarginMinor
}

func currentChartExtent(buckets []currentChartBucket) (float64, float64) {
	minValue, maxValue := 0.0, 0.0
	for _, bucket := range buckets {
		if bucket.actual != nil {
			for _, value := range []int64{
				bucket.actual.AmountMinor, bucket.actual.CapitalMinor, bucket.actual.MarginMinor,
			} {
				minValue = math.Min(minValue, float64(value))
				maxValue = math.Max(maxValue, float64(value))
			}
		}
		if bucket.ghost != nil {
			for _, value := range []int64{
				bucket.ghost.AmountLowMinor, bucket.ghost.AmountBaseMinor, bucket.ghost.AmountHighMinor,
				bucket.ghost.CapitalMinor, bucket.ghost.MarginMinor,
			} {
				minValue = math.Min(minValue, float64(value))
				maxValue = math.Max(maxValue, float64(value))
			}
		}
		if bucket.remainder != nil {
			actual := remainderActual(bucket.remainder, bucket.actual)
			amount, capital, margin := remainderTotals(actual, bucket.remainder.Remainder)
			for _, value := range []int64{
				actual.AmountMinor + bucket.remainder.Remainder.LowMinor,
				actual.AmountMinor + bucket.remainder.Remainder.HighMinor,
				amount, capital, margin,
			} {
				minValue = math.Min(minValue, float64(value))
				maxValue = math.Max(maxValue, float64(value))
			}
		}
	}
	return minValue, maxValue
}

func (r *reportRenderer) drawGhostColumn(x, width, from, to float64, valueY func(float64) float64, ink rgb) {
	if from == to {
		return
	}
	fromY := valueY(from)
	toY := valueY(to)
	top := math.Min(fromY, toY)
	height := math.Abs(toY - fromY)
	if height < 0.4 {
		return
	}
	band := r.pal.band
	r.canvas.rect(x, top, width, height, &band, nil, 0)
	// The hatch keeps the series colour, so a ghost amount, capital and margin
	// are still three different columns rather than one grey block.
	r.canvas.hatch(x, top, width, height, 2.6, ink, 0.45)
	edge := r.pal.edge
	r.canvas.rect(x, top, width, height, nil, &edge, 0.3)
}

func (c *canvas) whisker(x, lowY, highY float64, color rgb, lw float64) {
	if math.Abs(highY-lowY) < 0.2 {
		return
	}
	c.line(x, lowY, x, highY, color, lw)
	c.line(x-2, lowY, x+2, lowY, color, lw)
	c.line(x-2, highY, x+2, highY, color, lw)
}

func chartExtent(periods []poreport.Period) (float64, float64) {
	minValue, maxValue := 0.0, 0.0
	for _, period := range periods {
		for _, value := range []int64{
			period.AmountMinor, period.CapitalMinor, period.MarginMinor,
		} {
			minValue = math.Min(minValue, float64(value))
			maxValue = math.Max(maxValue, float64(value))
		}
	}
	return minValue, maxValue
}

func scaleY(scale Scale, value, top, height float64) float64 {
	if scale.Max == scale.Min {
		return top + height
	}
	return top + height - (value-scale.Min)/(scale.Max-scale.Min)*height
}

func minFloat(values []float64) float64 {
	minValue := values[0]
	for _, value := range values[1:] {
		minValue = math.Min(minValue, value)
	}
	return minValue
}

func maxFloat(values []float64) float64 {
	maxValue := values[0]
	for _, value := range values[1:] {
		maxValue = math.Max(maxValue, value)
	}
	return maxValue
}

func percentTick(value float64) string {
	// A chart tick is a visual guide, not a table figure. Compact very large
	// percentages so an accepted extreme value does not run off the axis; the
	// exact figure remains in the KPI strip and the projection table. Normal
	// percentages keep every digit the scale produced.
	negative := value < 0
	magnitude := value
	if negative {
		magnitude = -magnitude
	}
	if magnitude >= 10000 {
		label := compactNumber(magnitude) + "%"
		if negative {
			return "-" + label
		}
		return label
	}
	return strconv.FormatFloat(value, 'f', -1, 64) + "%"
}

// hatch fills a rectangle with parallel diagonal strokes. gpdf has no clip
// primitive, so the strokes are computed to their rectangle intersections
// instead of being clipped afterwards; the uncosted segment is then a real
// pattern on the page rather than a rectangle in a second colour.
func (c *canvas) hatch(x, y, width, height, step float64, color rgb, lw float64) {
	if width <= 0 || height <= 0 || step <= 0 {
		return
	}
	for offset := -height; offset <= width; offset += step {
		u1 := math.Max(0, -offset)
		v1 := offset + u1
		u2 := math.Min(width, height-offset)
		v2 := offset + u2
		if u1 >= u2 || v1 < -0.01 || v1 > height+0.01 || v2 < -0.01 || v2 > height+0.01 {
			continue
		}
		c.line(x+u1, y+v1, x+u2, y+v2, color, lw)
	}
}
