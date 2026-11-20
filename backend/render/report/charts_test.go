package report

import (
	"math"
	"testing"
)

// TestNegativeMarginDrawsBelowTheZeroBaseline pins the one chart rule that is
// easiest to get wrong: a signed margin column grows from zero in both
// directions, so a discounted-below-cost period is a bar below the baseline,
// not a small positive bar or an absolute value.
func TestNegativeMarginDrawsBelowTheZeroBaseline(t *testing.T) {
	renderer, err := testReportRenderer(sampleReport())
	if err != nil {
		t.Fatalf("layout report: %v", err)
	}
	pal := defaultPalette()
	var zeroLines []float64
	var negative *rectPrim
	for _, pg := range renderer.canvas.pages {
		for index := range pg.prims {
			switch value := pg.prims[index].(type) {
			case pathPrim:
				if value.stroke != nil && *value.stroke == pal.secondary && len(value.path.Segments) == 2 {
					from := value.path.Segments[0].Points[0]
					to := value.path.Segments[1].Points[0]
					if math.Abs(from.Y-to.Y) < 0.01 {
						zeroLines = append(zeroLines, from.Y)
					}
				}
			case rectPrim:
				if value.fill != nil && *value.fill == pal.marginNeg && value.h > 8 {
					copy := value
					negative = &copy
				}
			}
		}
	}
	if len(zeroLines) == 0 {
		t.Fatal("no zero baseline found in the chart")
	}
	if negative == nil {
		t.Fatal("no negative-margin column drawn")
	}
	onBaseline := false
	for _, zeroY := range zeroLines {
		if math.Abs(negative.y-zeroY) <= 0.6 {
			onBaseline = true
		}
	}
	if !onBaseline {
		t.Errorf("negative margin column starts at y=%.2f, not on any zero baseline %v", negative.y, zeroLines)
	}
	if negative.h <= 0 {
		t.Errorf("negative margin column has height %.2f", negative.h)
	}
}

func TestCurrentChartBucketsPutEveryGhostAfterTheLastActualDay(t *testing.T) {
	buckets, ok := currentChartBuckets(dayReport())
	if !ok {
		t.Fatal("day report did not build a ghost chart")
	}
	if len(buckets) != 31 {
		t.Fatalf("day chart has %d groups; want 31 days", len(buckets))
	}
	for index, bucket := range buckets {
		day := index + 1
		if day <= 19 {
			if bucket.actual == nil || bucket.ghost != nil {
				t.Fatalf("day %d must be actual only", day)
			}
			continue
		}
		if bucket.ghost == nil || bucket.actual != nil {
			t.Fatalf("day %d must be a ghost only", day)
		}
		if got := bucket.ghost.AmountLowMinor; got != 8_000_000 {
			t.Errorf("day %d low = %d", day, got)
		}
	}
}

func TestCurrentMonthChartStacksTheGhostRemainder(t *testing.T) {
	buckets, ok := currentChartBuckets(currentYearReport())
	if !ok {
		t.Fatal("current-year report did not build a ghost chart")
	}
	if len(buckets) != 12 {
		t.Fatalf("current-year chart has %d groups; want 12 months", len(buckets))
	}
	november := buckets[10]
	if november.start != "2026-11-01" || november.actual == nil || november.remainder == nil {
		t.Fatalf("November bucket = %+v; want actual plus remainder", november)
	}
	amount, capital, margin := remainderTotals(*november.actual, november.remainder.Remainder)
	if amount != 155_000_000 || capital != 114_000_000 || margin != 31_000_000 {
		t.Errorf("stacked remainder = %d/%d/%d; want 155M/114M/31M", amount, capital, margin)
	}
	december := buckets[11]
	if december.ghost == nil || december.actual != nil {
		t.Fatalf("December bucket = %+v; want a ghost only", december)
	}
}

func TestCurrentChartExtentIncludesActualsAndGhosts(t *testing.T) {
	dayBuckets, _ := currentChartBuckets(dayReport())
	minDay, maxDay := currentChartExtent(dayBuckets)
	if maxDay < 12_000_000 || minDay > 0 {
		t.Errorf("day extent = %.0f..%.0f; want the ghost high and the zero baseline", minDay, maxDay)
	}
	yearBuckets, _ := currentChartBuckets(currentYearReport())
	_, maxYear := currentChartExtent(yearBuckets)
	if maxYear < 155_000_000 {
		t.Errorf("year extent max = %.0f; want at least the stacked November total", maxYear)
	}
}

func TestGhostBarsAndWhiskersAreDrawn(t *testing.T) {
	renderer, err := testReportRenderer(dayReport())
	if err != nil {
		t.Fatalf("layout day report: %v", err)
	}
	pal := defaultPalette()
	ghostColumns := 0
	whiskerLines := 0
	for _, pg := range renderer.canvas.pages {
		for _, prim := range pg.prims {
			switch value := prim.(type) {
			case rectPrim:
				if value.fill != nil && *value.fill == pal.band {
					ghostColumns++
				}
			case pathPrim:
				if value.stroke != nil && *value.stroke == pal.secondary && len(value.path.Segments) == 2 {
					from := value.path.Segments[0].Points[0]
					to := value.path.Segments[1].Points[0]
					if math.Abs(from.X-to.X) < 0.01 && math.Abs(from.Y-to.Y) > 0.2 {
						whiskerLines++
					}
				}
			}
		}
	}
	if ghostColumns < 12*3 {
		t.Errorf("ghost columns = %d; want three ghost series for each of the 12 future days", ghostColumns)
	}
	// Each of the 12 ghosts draws a vertical whisker plus two caps.
	if whiskerLines < 12 {
		t.Errorf("vertical whisker lines = %d; want at least one per future day", whiskerLines)
	}
}

func TestCurrentMonthDrawsTheStackedGhostColumnsAndLegend(t *testing.T) {
	renderer, err := testReportRenderer(currentYearReport())
	if err != nil {
		t.Fatalf("layout current-year report: %v", err)
	}
	pal := defaultPalette()
	bandRects := 0
	for _, pg := range renderer.canvas.pages {
		for _, prim := range pg.prims {
			value, ok := prim.(rectPrim)
			if !ok || value.fill == nil || *value.fill != pal.band {
				continue
			}
			if value.h > 0.4 {
				bandRects++
			}
		}
	}
	// Three stacked remainder columns in November and three ghost columns in
	// December. A chart that drew only the future month, or only one series,
	// fails this count.
	if bandRects < 6 {
		t.Errorf("ghost columns = %d; want the November remainder and December ghost for amount, capital and margin", bandRects)
	}
	text := pageText(renderer.canvas.pages)
	requireText(t, text, "Projected (ghost)", "Projected low-high",
		"Future months are hatched ghosts; the current month stacks its actual columns with the ghost remainder.")
}

// TestUnknownMarginPercentLeavesAGapInTheLine pins that a nil percentage is a
// gap: the line is drawn as one segment between each pair of adjacent known
// periods, and no segment jumps across the unknown June value.
func TestUnknownMarginPercentLeavesAGapInTheLine(t *testing.T) {
	renderer, err := testReportRenderer(sampleReport())
	if err != nil {
		t.Fatalf("layout report: %v", err)
	}
	pal := defaultPalette()
	segments := 0
	for _, pg := range renderer.canvas.pages {
		for _, prim := range pg.prims {
			value, ok := prim.(pathPrim)
			if !ok || value.stroke == nil || *value.stroke != pal.marginLine {
				continue
			}
			if len(value.path.Segments) == 2 {
				from := value.path.Segments[0].Points[0]
				to := value.path.Segments[1].Points[0]
				if math.Abs(from.X-to.X) > 20 {
					segments++
				}
			}
		}
	}
	// Twelve periods, June unknown: January-May is four segments and
	// July-December is five. A line across the gap would make ten.
	if segments != 9 {
		t.Errorf("margin %% line has %d segments, want 9 with one unknown month", segments)
	}
}
