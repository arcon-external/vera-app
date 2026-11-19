package report

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gpdf-dev/gpdf/document"

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
		label := formatPct(periods[lastKnown].MarginBps)
		labelX := lastX + 3
		if labelX+r.canvas.faces.width(label, 6.2, true) > plotRight {
			labelX = lastX - r.canvas.faces.width(label, 6.2, true) - 3
		}
		r.canvas.text(label, labelX, lastY-3, 6.2, true, pal.marginLine)
	}
}

func (r *reportRenderer) drawChartPeriodLabels(periods []poreport.Period, plotX, baseline, groupW float64) {
	for index, period := range periods {
		label := periodLabel(period.PeriodStart, r.snapshot.Filter.Bucket)
		size := fittedSize(r.canvas.faces.width, label, groupW-2, 6.2, 5, false)
		center := plotX + (float64(index)+0.5)*groupW
		r.canvas.textCenter(label, center, baseline+11, size, false, r.pal.secondary)
	}
}

func (r *reportRenderer) drawProjectionCharts() {
	monthly := r.snapshot.Projection.Monthly
	yearly := r.snapshot.Projection.Yearly
	if monthly.Status != poreport.ProjectionStatusAvailable && yearly.Status != poreport.ProjectionStatusAvailable {
		return
	}
	c := r.canvas
	c.ensure(34)
	c.line(marginX, c.y+10, marginX+contentWidth, c.y+10, r.pal.accent, 0.8, 4, 3)
	c.text("PROJECTION  ·  ESTIMATES, NEVER INCLUDED IN ACTUAL TOTALS",
		marginX, c.y+24, 7.4, true, r.pal.accent)
	c.y += 32

	if monthly.Status == poreport.ProjectionStatusAvailable {
		r.drawProjectionPanel("Monthly projection  ·  next 12 months", monthly.Buckets, poreport.BucketMonth)
	} else {
		r.unavailableProjectionLine("Monthly projection", monthly.Reason)
	}
	if yearly.Status == poreport.ProjectionStatusAvailable {
		r.drawProjectionPanel("Yearly projection  ·  next 3 years", yearly.Buckets, poreport.BucketYear)
	} else {
		r.unavailableProjectionLine("Yearly projection", yearly.Reason)
	}
}

func (r *reportRenderer) unavailableProjectionLine(label, reason string) {
	c := r.canvas
	c.ensure(20)
	text := label + " unavailable: " + reason
	c.text(text, marginX, c.y+9, 7, false, r.pal.muted)
	c.y += 14
}

func (r *reportRenderer) drawProjectionPanel(title string, buckets []poreport.ProjectedBucket, bucket string) {
	c := r.canvas
	pal := r.pal
	c.ensure(chartPanelHeight)
	top := c.y
	c.text(title, marginX, top+10, 8.8, true, pal.emphasis)
	c.text("Shaded band = low–high amount range  ·  line = base", marginX, top+20, 6.4, false, pal.muted)

	plotX := marginX + 48
	plotRight := marginX + contentWidth - 18
	plotW := plotRight - plotX
	plotY := top + chartTitleHeight + 8
	plotH := chartPlotHeight

	minValue, maxValue := 0.0, 0.0
	for _, bucket := range buckets {
		for _, value := range []int64{
			bucket.AmountLowMinor, bucket.AmountBaseMinor, bucket.AmountHighMinor,
			bucket.CapitalMinor, bucket.MarginMinor,
		} {
			minValue = math.Min(minValue, float64(value))
			maxValue = math.Max(maxValue, float64(value))
		}
	}
	scale := NiceScale(minValue, maxValue, 5)
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
	if len(buckets) > 1 {
		points := make([]document.Point, 0, len(buckets)*2)
		for index, bucket := range buckets {
			x := plotX + (float64(index)+0.5)*groupW
			points = append(points, document.Point{X: x, Y: valueY(float64(bucket.AmountLowMinor))})
		}
		for index := len(buckets) - 1; index >= 0; index-- {
			x := plotX + (float64(index)+0.5)*groupW
			points = append(points, document.Point{X: x, Y: valueY(float64(buckets[index].AmountHighMinor))})
		}
		band := pal.band
		edge := pal.edge
		c.polygon(points, &band, &edge, 0.5)
	} else if len(buckets) == 1 {
		x := plotX + groupW*0.2
		lowY := valueY(float64(buckets[0].AmountLowMinor))
		highY := valueY(float64(buckets[0].AmountHighMinor))
		band := pal.band
		c.rect(x, highY, groupW*0.6, lowY-highY, &band, nil, 0)
	}

	for index, projected := range buckets {
		x := plotX + (float64(index)+0.5)*groupW
		y := valueY(float64(projected.AmountBaseMinor))
		if index > 0 {
			previousX := plotX + (float64(index)-0.5)*groupW
			previousY := valueY(float64(buckets[index-1].AmountBaseMinor))
			c.line(previousX, previousY, x, y, pal.accent, 1.5)
		}
		c.rect(x-1.4, y-1.4, 2.8, 2.8, &pal.accent, nil, 0)
		label := periodLabel(projected.PeriodStart, bucket)
		size := fittedSize(c.faces.width, label, groupW-2, 6.1, 5, false)
		c.textCenter(label, x, plotY+plotH+11, size, false, pal.secondary)
	}
	c.text("Estimates from complete history; not actual evidence.",
		plotX, plotY+plotH+21, 6.2, false, pal.muted)
	c.y = plotY + plotH + 28
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
