package gpdf

import (
	"math"
	"testing"

	"vera-app/backend/render/quotation"
)

// Guards for the After Disc. cell after issue #398: it is one line -- the net
// price, drawn through the shared price-size rule -- and the row is tall
// enough to hold it.

// afterDiscInput builds a one-row document whose After Disc. cell carries the
// net price.
func afterDiscInput() Input {
	return Input{
		CompanyName: "C", CompanyAddress: []string{"A"},
		RecipientName: "R", RecipientCompany: "RC", RecipientAddress: "RA",
		Subject: "S", QuoteNumber: "1", Date: "d", ValidUntil: "v",
		Terms: []string{"1. a"}, Closing: "c",
		Items: []Item{
			{No: 1, DescMain: "Widget", Qty: "2", UoM: "Lot",
				UnitPrice: "120.000", Total: "216.000", Disc: "108.000"},
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

// TestAfterDiscDrawsTheNetPrice pins the cell's single line: the net price,
// centred in its track like the numeric cells beside it, at the shared
// price-size rule's size for its text (11px for a short figure).
func TestAfterDiscDrawsTheNetPrice(t *testing.T) {
	e := testEngine(t)
	runs := runsOnFirstPage(e, afterDiscInput())

	price := findRuns(t, runs, "108.000")
	if len(price) != 1 {
		t.Fatalf("want 1 run printing the net price, got %d", len(price))
	}
	wantSize := px(quotation.PriceSizePx("108.000"))
	if price[0].size != wantSize {
		t.Errorf("net price size %.3fpt; want %.3fpt (PriceSizePx)", price[0].size, wantSize)
	}

	// The percentage line is gone: nothing prints a "%" in the item table.
	for _, r := range runs {
		if r.text == "10%" || r.text == "0%" || r.text == "—" {
			t.Errorf("second-line run %q still drawn in the After Disc. cell", r.text)
		}
	}

	// Centred in the same track the Unit Price and Total cells use.
	m := e.derive(afterDiscInput())
	col := m.cols.after
	trackW := m.colX[col+1] - m.colX[col] - px(1) - 2*px(tdPadPx)
	w := e.Width("108.000", wantSize, false)
	wantX := m.colX[col] + px(1) + px(tdPadPx) + (trackW-w)/2
	if d := math.Abs(price[0].x - wantX); d > 0.01 {
		t.Errorf("net price x %.3f; want centred at %.3f (d %.3f)", price[0].x, wantX, d)
	}
}

// TestRowHeightsFitTheAfterCell pins the floor: a one-line priced row is at
// least as tall as its one-line money cells, not merely as tall as its
// one-line description.
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

	// A part row carries the same cell with the sub-row's padding.
	part := afterDiscInput()
	part.Items[0].Package = &Package{ShowPrices: true, Parts: []Part{
		{SubNo: "1.1", Desc: "part a", Qty: "1", UoM: "Pc",
			UnitPrice: "220.000", Total: "209.000", Disc: "209.000"},
	}}
	mp := e.derive(part)
	prows := e.measureRows(part, mp)
	if len(prows) != 2 {
		t.Fatalf("laid out %d blocks, want title + part", len(prows))
	}
	wantPart := wantPx*ptPerPx + 2*px(subRowPadPx) + 2*px(1)
	assertNear(t, "one-line part row height", prows[1].height, wantPart, 0.001)
}

// TestPriceSizeStepsFitTheColumn pins the calibration of the shared price-size
// rule (quotation.PriceSizePx, issue #398): the worst string each step can
// receive must fit the price column's content box in the engine's own
// metrics. The strings are all digits and separators -- digits are the widest
// glyphs money uses, and these are the widest strings at each length the rule
// can see.
//
// The column's content box is 79px (the 15.5% track at the 646px table, less
// the 1px collapsed border and the 10px padding each side). Both engines'
// measured widths are quoted below: the engine's (Inter parsed by gpdf) and
// Chrome 155's (the embedded Inter, canvas measureText, after
// document.fonts.load). Chrome renders this font slightly narrower than the
// engine, so the engine's number is the binding one -- but both are held
// under the box, because the two renderers print the same document. The rule
// guarantees a fit through 18 characters; a 20-character figure (86.11px in
// the engine; the 19-character worst is 83.82px) overflows the content box
// but stays inside the cell, exactly as it did before this rule existed.
func TestPriceSizeStepsFitTheColumn(t *testing.T) {
	e := testEngine(t)
	in := afterDiscInput()
	m := e.derive(in)
	col := m.cols.unit
	content := m.colX[col+1] - m.colX[col] - px(1) - 2*px(tdPadPx)

	cases := []struct {
		text     string
		wantPx   float64
		engineW  float64 // the engine's own measured width, CSS px
		browserW float64 // Chrome 155, embedded Inter, canvas measureText
	}{
		{"999.999.999,9", 11, 77.89, 76.71},     // 13 chars: the 11px step's worst
		{"1.234.567.890", 11, 74.92, 72.83},     // 13 chars: still 11px
		{"999.999.999,99", 10, 77.03, 75.96},    // 14 chars: the 10px step's worst
		{"9.999.999.999.99", 9, 77.50, 76.22},   // 16 chars: the 9px step's worst
		{"1.234.567.890.12", 9, 72.99, 70.22},   // 16 chars: still 9px
		{"9.999.999.999.999", 8, 73.87, 72.73},  // 17 chars: the 8px floor
		{"999.999.999.999.99", 8, 78.84, 77.70}, // 18 chars: the floor's worst
	}
	for _, tc := range cases {
		if got := quotation.PriceSizePx(tc.text); got != tc.wantPx {
			t.Errorf("PriceSizePx(%q) = %v; want %v", tc.text, got, tc.wantPx)
		}
		w := e.Width(tc.text, px(tc.wantPx), false)
		if w > content {
			t.Errorf("PriceSizePx(%q) at %vpx measures %.2fpx, wider than the %.2fpx content box",
				tc.text, tc.wantPx, w/ptPerPx, content/ptPerPx)
		}
		if got := w / ptPerPx; math.Abs(got-tc.engineW) > 0.5 {
			t.Errorf("%q at %vpx: engine now measures %.2fpx; the calibration recorded %.2fpx -- re-measure both engines",
				tc.text, tc.wantPx, got, tc.engineW)
		}
		if tc.browserW > content/ptPerPx {
			t.Errorf("%q at %vpx measures %.2fpx in Chrome, wider than the %.2fpx content box",
				tc.text, tc.wantPx, tc.browserW, content/ptPerPx)
		}
		// The rule is only useful if it actually shrinks what would overflow.
		if bodyW := e.Width(tc.text, px(bodyPx), false); bodyW <= content && tc.wantPx != bodyPx {
			t.Errorf("%q fits at body size (%.2fpx) but steps down to %vpx", tc.text, bodyW/ptPerPx, tc.wantPx)
		}
	}
}
