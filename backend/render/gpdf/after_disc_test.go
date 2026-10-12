package gpdf

import (
	"math"
	"testing"
)

// Guards for issue #380's refinement: the After Disc. cell is always two lines
// -- the net price, then the discount percentage or an em dash -- in both
// renderers, and the row is tall enough to hold them.

func TestSplitAfterDisc(t *testing.T) {
	cases := []struct {
		name       string
		packed     string
		wantPrice  string
		wantSecond string
	}{
		{"discounted", "108.000\n10%", "108.000", "10%"},
		{"undiscounted", "50.000\n—", "50.000", "—"},
		{"legacy bare price", "108.000", "108.000", "—"},
		{"legacy empty", "", "", "—"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			price, pct := splitAfterDisc(tc.packed)
			if price != tc.wantPrice || pct != tc.wantSecond {
				t.Errorf("splitAfterDisc(%q) = (%q, %q); want (%q, %q)",
					tc.packed, price, pct, tc.wantPrice, tc.wantSecond)
			}
		})
	}
}

// afterDiscInput builds a one-row document whose After Disc. cell packs net/pct.
func afterDiscInput() Input {
	return Input{
		CompanyName: "C", CompanyAddress: []string{"A"},
		RecipientName: "R", RecipientCompany: "RC", RecipientAddress: "RA",
		Subject: "S", QuoteNumber: "1", Date: "d", ValidUntil: "v",
		Terms: []string{"1. a"}, Closing: "c",
		Items: []Item{
			{No: 1, DescMain: "Widget", Qty: "2", UoM: "Lot",
				UnitPrice: "120.000", Total: "216.000", Disc: "108.000\n10%"},
		},
	}
}

// runsOnFirstPage collects the page's text runs.
func runsOnFirstPage(e *Engine, in Input) []run {
	pages := e.layout(in)
	var out []run
	for _, prim := range pages[0].prims {
		if r, ok := prim.(run); ok {
			out = append(out, r)
		}
	}
	return out
}

func findRuns(t *testing.T, runs []run, text string) []run {
	t.Helper()
	var out []run
	for _, r := range runs {
		if r.text == text {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no run prints %q", text)
	}
	return out
}

// TestAfterDiscDrawsTwoLines pins the cell's two printed lines: the net price
// at body size in the row's ink, and the percentage at 9px in the muted
// annotation ink, tracked per the stylesheet's letter-spacing.
func TestAfterDiscDrawsTwoLines(t *testing.T) {
	e := testEngine(t)
	runs := runsOnFirstPage(e, afterDiscInput())

	price := findRuns(t, runs, "108.000")
	if len(price) != 1 {
		t.Fatalf("want 1 run printing the net price, got %d", len(price))
	}
	if price[0].size != px(bodyPx) {
		t.Errorf("net price size %.3fpt; want %.3fpt (body)", price[0].size, px(bodyPx))
	}

	pct := findRuns(t, runs, "10%")
	if len(pct) != 1 {
		t.Fatalf("want 1 run printing the percentage, got %d", len(pct))
	}
	if pct[0].size != px(afterPctPx) {
		t.Errorf("percentage size %.3fpt; want %.3fpt (9px)", pct[0].size, px(afterPctPx))
	}
	if pct[0].tracking != px(0.02*afterPctPx) {
		t.Errorf("percentage tracking %.4fpt; want %.4fpt (0.02em)", pct[0].tracking, px(0.02*afterPctPx))
	}
	wantMuted := e.pal.Muted
	if pct[0].color != wantMuted {
		t.Errorf("percentage color %q; want muted %q", pct[0].color, wantMuted)
	}
	// The annotation sits below the price, inside the same track.
	if !(pct[0].baseline > price[0].baseline) {
		t.Errorf("percentage baseline %.2f not below price baseline %.2f", pct[0].baseline, price[0].baseline)
	}
	dx := pct[0].x - price[0].x
	// Both lines are centred in the same track, so their centres agree to
	// within the shaping difference of two different strings -- far less than
	// a track's width, far more than zero.
	pc := price[0].x + e.Width("108.000", px(bodyPx), false)/2
	qc := pct[0].x + e.WidthTracked("10%", px(afterPctPx), false, px(0.02*afterPctPx))/2
	if d := math.Abs(pc - qc); d > 1.0 {
		t.Errorf("after lines centred %.2fpt apart (price x %.2f, pct x %.2f, dx %.2f)", d, price[0].x, pct[0].x, dx)
	}
}

// TestAfterDiscUndiscountedDrawsDash pins the no-discount state: the price and
// an em dash, not a percentage.
func TestAfterDiscUndiscountedDrawsDash(t *testing.T) {
	e := testEngine(t)
	in := afterDiscInput()
	in.Items[0].Disc = "50.000\n—"
	runs := runsOnFirstPage(e, in)
	findRuns(t, runs, "50.000")
	dash := findRuns(t, runs, "—")
	if len(dash) != 1 {
		t.Fatalf("want 1 em-dash run, got %d", len(dash))
	}
	if dash[0].size != px(afterPctPx) || dash[0].color != e.pal.Muted {
		t.Errorf("dash run size %.3f color %q; want %.3f muted %q",
			dash[0].size, dash[0].color, px(afterPctPx), e.pal.Muted)
	}
}

// TestRowHeightsFitTheAfterCell pins the floor: a one-line priced row is as
// tall as its two-line after cell, not as tall as its one-line description.
func TestRowHeightsFitTheAfterCell(t *testing.T) {
	e := testEngine(t)

	in := afterDiscInput()
	m := e.derive(in)
	rows := e.measureRows(in, m)
	if len(rows) != 1 {
		t.Fatalf("laid out %d rows, want 1", len(rows))
	}
	wantPx := math.Floor(afterCellPx)
	want := wantPx*ptPerPx + 2*px(tdPadPx) + 2*px(1)
	assertNear(t, "one-line main row height", rows[0].height, want, 0.001)
	if rows[0].height/ptPerPx < wantPx+2*tdPadPx+2 {
		t.Errorf("row height %.2fpx fits no two-line cell", rows[0].height/ptPerPx)
	}

	// A part row carries the same cell with the sub-row's padding.
	part := afterDiscInput()
	part.Items[0].Package = &Package{ShowPrices: true, Parts: []Part{
		{SubNo: "1.1", Desc: "part a", Qty: "1", UoM: "Pc",
			UnitPrice: "220.000", Total: "209.000", Disc: "209.000\n5%"},
	}}
	mp := e.derive(part)
	prows := e.measureRows(part, mp)
	if len(prows) != 2 {
		t.Fatalf("laid out %d blocks, want title + part", len(prows))
	}
	wantPart := wantPx*ptPerPx + 2*px(subRowPadPx) + 2*px(1)
	assertNear(t, "one-line part row height", prows[1].height, wantPart, 0.001)
}
