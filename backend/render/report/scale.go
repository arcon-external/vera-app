// Package report renders the purchase-order performance report as a native
// A4 PDF.
//
// The renderer draws only the snapshot the caller supplies: it never fetches a
// report, recomputes a figure, or signs anything. The screen and this document
// therefore read the same bytes, and a value that is not on the screen cannot
// appear here.
//
// Geometry is laid out explicitly, as the quotation engine does: gpdf's rect,
// path and text primitives, no HTML and no SVG. The only chart library is the
// nice-scale algorithm in this file, shared with the TypeScript chart through
// testdata/chart_scales.json.
package report

import "math"

// Scale is one axis: a domain rounded outward to a nice step and the ticks
// drawn between its bounds.
type Scale struct {
	Min   float64
	Max   float64
	Step  float64
	Ticks []float64
}

// DefaultTargetTicks is the tick count the report asks for. The algorithm may
// return a few more or fewer; it never returns zero ticks.
const DefaultTargetTicks = 6

// NiceScale returns a zero-inclusive axis over [min, max].
//
// Zero is always inside the domain because every figure the report charts is
// either money (where a column must start at zero to be a column) or a margin
// percentage (where the sign is the point). Steps are 1, 2 or 5 times a power
// of ten; the domain is rounded outward to whole steps, and both bounds are
// ticks. Exact zero is returned as 0 rather than -0.
//
// The same function is implemented in TypeScript for the on-screen chart and
// both are pinned by testdata/chart_scales.json. Change the fixture and both
// implementations together; a chart whose axis differs from the PDF's is the
// disagreement this function exists to prevent.
func NiceScale(min, max float64, targetTicks int) Scale {
	if targetTicks < 2 {
		targetTicks = 2
	}
	if math.IsNaN(min) || math.IsNaN(max) || math.IsInf(min, 0) || math.IsInf(max, 0) {
		return Scale{Min: 0, Max: 1, Step: 0.2, Ticks: []float64{0, 0.2, 0.4, 0.6, 0.8, 1}}
	}
	if min > max {
		min, max = max, min
	}

	// A money column and a percentage line are both read against zero, so the
	// axis always includes it.
	if min > 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if min == 0 && max == 0 {
		max = 1
	}

	intervals := float64(targetTicks - 1)
	step := niceStep((max - min) / intervals)
	if step <= 0 || math.IsInf(step, 0) || math.IsNaN(step) {
		step = 1
	}

	// The epsilon keeps a bound that sits exactly on a step from gaining an
	// extra empty step through floating-point division.
	niceMin := math.Floor(min/step+1e-9) * step
	niceMax := math.Ceil(max/step-1e-9) * step
	// Floor/ceil can produce -0; a zero domain edge is zero.
	if niceMin == 0 {
		niceMin = 0
	}
	if niceMax == 0 {
		niceMax = 0
	}

	count := int(math.Round((niceMax - niceMin) / step))
	if count < 1 {
		count = 1
	}
	ticks := make([]float64, 0, count+1)
	for i := 0; i <= count; i++ {
		value := roundTick(niceMin+float64(i)*step, step)
		if value == 0 {
			value = 0
		}
		ticks = append(ticks, value)
	}

	return Scale{Min: niceMin, Max: niceMax, Step: step, Ticks: ticks}
}

// niceStep rounds a raw step up to the nearest 1, 2 or 5 times a power of ten.
// The thresholds are the midpoints between those candidates on a log view, so
// the chosen step is the nearest one rather than always the next larger one.
func niceStep(raw float64) float64 {
	if raw <= 0 || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 1
	}
	exponent := math.Floor(math.Log10(raw))
	base := math.Pow(10, exponent)
	switch fraction := raw / base; {
	case fraction < 1.5:
		return base
	case fraction < 3:
		return 2 * base
	case fraction < 7:
		return 5 * base
	default:
		return 10 * base
	}
}

// roundTick removes the binary-noise tail a step like 0.2 leaves behind. The
// number of decimals is the step's own last significant digit, so both this
// implementation and the TypeScript one print the same clean values.
func roundTick(value, step float64) float64 {
	if step <= 0 || math.IsInf(step, 0) || math.IsNaN(step) {
		return value
	}
	decimals := -int(math.Floor(math.Log10(step)))
	if decimals < 0 {
		decimals = 0
	}
	if decimals > 15 {
		decimals = 15
	}
	factor := math.Pow(10, float64(decimals))
	return math.Round(value*factor) / factor
}
