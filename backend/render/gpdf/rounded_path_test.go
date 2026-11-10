package gpdf

import (
	"math"
	"testing"

	"github.com/gpdf-dev/gpdf/document"
)

// pathHasCurveEndingAt reports whether p has a cubic ending at (x, y). A
// rounded corner's curve ends where the straight edge resumes, so this is how
// the tests tell a square corner from a rounded one.
func pathHasCurveEndingAt(p document.Path, x, y float64) bool {
	for _, seg := range p.Segments {
		if seg.Op != document.PathCurveTo || len(seg.Points) == 0 {
			continue
		}
		end := seg.Points[len(seg.Points)-1]
		if math.Abs(end.X-x) < 0.01 && math.Abs(end.Y-y) < 0.01 {
			return true
		}
	}
	return false
}

// TestRoundedPathCornerOrder pins the argument order. Four distinct radii make
// a swap visible: top-left 1, top-right 2, bottom-right 3, bottom-left 4.
func TestRoundedPathCornerOrder(t *testing.T) {
	p := roundedPath(0, 0, 100, 100, 1, 2, 3, 4)

	want := []struct {
		name string
		x, y float64
	}{
		{"top-left", 1, 0},
		{"top-right", 100, 2},
		{"bottom-right", 97, 100},
		{"bottom-left", 0, 96},
	}
	for _, w := range want {
		if !pathHasCurveEndingAt(p, w.x, w.y) {
			t.Errorf("%s corner has no curve ending at (%.1f, %.1f)", w.name, w.x, w.y)
		}
	}
}
