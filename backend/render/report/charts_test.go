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
