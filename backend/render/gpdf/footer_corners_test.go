package gpdf

import (
	"math"
	"testing"

	"github.com/gpdf-dev/gpdf/document"
)

// TestFooterCardsSquareOnlyTheApprovedCorners pins the three-corner rule from
// the approved layout: the terms card keeps its top-left square, the totals
// card keeps its top-right square, and every other corner stays at the card
// radius. The boxes are found by their measured footer geometry; the radii
// themselves come from the two helpers the draw layer calls.
func TestFooterCardsSquareOnlyTheApprovedCorners(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	m := e.derive(in)
	pages := e.layout(in)

	var tnc, totals *box
	for _, pg := range pages {
		for _, prim := range pg.prims {
			b, ok := prim.(box)
			if !ok || b.path == nil {
				continue
			}
			bb := b
			switch {
			case b.x == m.innerX && b.w == m.footerTncW:
				tnc = &bb
			case b.x == m.innerX+m.footerTncW+px(footGapPx) && b.w == m.footerTotW:
				totals = &bb
			}
		}
	}
	if tnc == nil {
		t.Fatal("the terms card was not found with a per-corner path")
	}
	if totals == nil {
		t.Fatal("the totals card was not found with a per-corner path")
	}

	r := px(cardRadiusPx)
	wantTnc := [4]float64{0, r, r, r}
	wantTotals := [4]float64{r, 0, r, r}
	gotTnc := [4]float64{}
	gotTnc[0], gotTnc[1], gotTnc[2], gotTnc[3] = termsCardCorners(r)
	gotTotals := [4]float64{}
	gotTotals[0], gotTotals[1], gotTotals[2], gotTotals[3] = totalsCardCorners(r)

	for i, want := range wantTnc {
		assertNear(t, "terms corner", gotTnc[i], want, 0.001)
	}
	for i, want := range wantTotals {
		assertNear(t, "totals corner", gotTotals[i], want, 0.001)
	}

	// The helper values are not enough: the path itself must put the square
	// corner at the top-left. A rounded top-left starts at x+r and carries a
	// curve ending there; a square one starts at x.
	first := tnc.path.Segments[0]
	if first.Op != document.PathMoveTo || len(first.Points) != 1 {
		t.Fatalf("terms path starts with %v, want a MoveTo", first.Op)
	}
	if math.Abs(first.Points[0].X-tnc.x) > 0.01 || math.Abs(first.Points[0].Y-tnc.y) > 0.01 {
		t.Errorf("terms path starts at (%.2f, %.2f), want the square top-left (%.2f, %.2f)",
			first.Points[0].X, first.Points[0].Y, tnc.x, tnc.y)
	}
	if !pathHasCurveEndingAt(*tnc.path, tnc.x, tnc.y+tnc.h-px(cardRadiusPx)) {
		t.Error("terms bottom-left is not rounded")
	}
	if pathHasCurveEndingAt(*totals.path, totals.x+totals.w, totals.y+px(cardRadiusPx)) {
		t.Error("totals top-right is rounded; want square")
	}
	if !pathHasCurveEndingAt(*totals.path, totals.x+totals.w-px(cardRadiusPx), totals.y+totals.h) {
		t.Error("totals bottom-right is not rounded")
	}
}
