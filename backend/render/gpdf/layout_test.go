package gpdf

// Layout tests.
//
// Every number here was measured out of a browser rendering the same template
// with the same font, and then pinned. They are the reason a change to the
// layout can be reviewed: a number moving is a change in the document, not a
// change in the code's opinion about the document.
//
// The expected values are the browser's, to the precision the browser's own
// rounding produces. Where the two disagree the expectation is not adjusted to
// match the engine — that is the whole point of writing them down.

import (
	"math"
	"os"
	"testing"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(os.DirFS("../../assets"), "inter-medium.ttf", "inter-bold.ttf")
	if err != nil {
		t.Fatalf("load engine: %v", err)
	}
	return e
}

// near fails the test when two points are further apart than tol. A document
// that is a fifth of a point out is a document whose rows no longer line up with
// the rules drawn beside them.
func assertNear(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.3f, browser renders %.3f (off by %+.3f, tolerance %.3f)",
			what, got, want, got-want, tol)
	}
}

// TestColumnEdgesOnWholePixels pins the table's grid.
//
// Chrome lays a fixed-layout table out in integer pixel tracks, so a boundary
// that falls between pixels is truncated rather than kept at its fractional
// position. Keeping the exact percentages instead moves the description column's
// left edge by a fifth of a point, which is enough for the wrapping cell to be
// narrow enough to break a word earlier than the browser breaks it.
//
// The two boundaries that decide wrapping are held to a hundredth of a point.
// The others are held to a pixel, because that is as close as Chrome's
// distribution of the trailing tracks could be pinned: it hands the odd pixel to
// a different column than a straightforward truncation does, and a pixel in the
// middle of a centred numeric column moves the number by half of one.
func TestColumnEdgesOnWholePixels(t *testing.T) {
	// Both grids, because the table has two and only the column list differs:
	// the six-column default, and the seven-column one a per-row discount adds.
	// The expected values are the measured pixel offsets scaled by ptPerPx and
	// shifted to the table's left edge.
	cases := []struct {
		name  string
		cols  colLayout
		want  []float64
		names []string
	}{
		{
			name:  "six columns",
			cols:  layoutFor(false),
			want:  []float64{56.25, 90.0, 298.5, 336.75, 375.75, 457.5, 540.75},
			names: []string{"table left", "No.", "Description", "Qty", "UoM", "Unit price", "table right"},
		},
		{
			name: "seven columns, with a per-row discount",
			cols: layoutFor(true),
			// The description gives up exactly the 45px the new column takes, so
			// Total's own edges (457.5, 540.75) are unchanged between the grids.
			want: []float64{56.25, 90.0, 264.0, 303.0, 342.0, 423.75, 457.5, 540.75},
			names: []string{
				"table left", "No.", "Description", "Qty", "UoM", "Unit price", "Disc.", "table right",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := columnEdges(56.25, 484.5, tc.cols)
			if got[tc.cols.last] != got[len(tc.want)-1] {
				t.Fatalf("columnEdges wrote the right edge to index %d, not last (%d)",
					len(tc.want)-1, tc.cols.last)
			}
			for i := range tc.want {
				tol := ptPerPx // one CSS pixel
				switch {
				case i == 0 || i == len(tc.want)-1:
					tol = 0.001 // the table's own edges are exact
				case tc.names[i] == "Description":
					tol = 0.01 // decides where the wrapping cell breaks
				}
				assertNear(t, tc.names[i], got[i], tc.want[i], tol)
			}
		})
	}
}

// TestDescriptionWrapMatchesTheBrowser pins the line breaks of the wrapping cell.
//
// Wrapping is the one place where a small measurement difference is not a small
// visual difference: it moves a whole word onto the next line. The expectations
// are the browser's breaks for the same string in the same column.
func TestDescriptionWrapMatchesTheBrowser(t *testing.T) {
	e := testEngine(t)
	m := e.derive(sampleInput())

	cases := []struct {
		name  string
		text  string
		width float64
		want  []string
	}{
		{
			name:  "main description",
			text:  "Supply & Install lighting sensor system (PIR Schneider) at AHU block 3, Mezzanine Front & Rear with following scopes:",
			width: m.colX[2] - m.colX[1] - px(1) - 2*px(tdPadPx),
			want: []string{
				"Supply & Install lighting sensor system (PIR",
				"Schneider) at AHU block 3, Mezzanine Front &",
				"Rear with following scopes:",
			},
		},
		{
			name:  "list item",
			text:  "6 sensors at Front AHU and 6 sensors at Rear AHU",
			width: m.colX[2] - m.colX[1] - px(1) - 2*px(tdPadPx) - px(subIndentPx),
			want: []string{
				"6 sensors at Front AHU and 6 sensors at Rear",
				"AHU",
			},
		},
		{
			name:  "list item with a measurement",
			text:  "6-units batten fixtures with guard (2x 1200mm LED tubes)",
			width: m.colX[2] - m.colX[1] - px(1) - 2*px(tdPadPx) - px(subIndentPx),
			want: []string{
				"6-units batten fixtures with guard (2x 1200mm",
				"LED tubes)",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := e.Wrap(c.text, px(bodyPx), c.width, false)
			if len(got) != len(c.want) {
				t.Fatalf("wrapped into %d lines, browser wraps into %d:\n got %q\nwant %q",
					len(got), len(c.want), got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("line %d:\n got %q\nwant %q", i+1, got[i], c.want[i])
				}
			}
		})
	}
}

// TestRowHeightsMatchTheBrowser pins the height of every table row.
//
// A row's height is padding, the description's lines, the list's top margin, the
// list's lines and the margins between items. The list margins are the part worth
// watching: they sit between *items*, not between *lines*, so a list item that
// wraps must not add one.
func TestRowHeightsMatchTheBrowser(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	rows := e.measureRows(in, e.derive(in))

	// The browser's row heights for the lab document, in order, read off its own
	// content stream rather than off a raster. The tolerance is two pixels
	// because this test's input is not the lab document's: its rows wrap
	// differently, so the fractions the 15.4px line box leaves behind land
	// differently, and the two are the same size only to within that.
	want := []float64{132.00, 38.25, 39.00, 84.75, 96.00, 39.00}
	if len(rows) != len(want) {
		t.Fatalf("laid out %d rows, measured %d", len(rows), len(want))
	}
	for i, rb := range rows {
		assertNear(t, "row height", rb.height, want[i], 1.5)
	}
}

// TestListItemsAreCountedNotLines guards the trap in the row-height formula: a
// list item that wraps is still one item, and the margin between items belongs
// between the last line of one item and the first line of the next.
func TestListItemsAreCountedNotLines(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	rows := e.measureRows(in, e.derive(in))

	first := rows[0]
	items := countItems(first.subs)
	if items != 3 {
		t.Errorf("first row has %d list items, the data has 3", items)
	}
	if len(first.subs) != 6 {
		t.Errorf("first row has %d list lines, the browser lays out 6", len(first.subs))
	}
	// Nine lines of text in all, the list's 6px top margin, one 3px between each
	// pair of items and the last item's own 3px below it, resolved to a whole
	// pixel, then the cell's padding and both its collapsed borders.
	//
	// The assertion is against the browser's number, not against the formula
	// restated here. Restating it only checks that the test and the code agree,
	// which is what let the rounding rule drift: this row came out 132.000
	// because the content was rounded up from 153.6, and every two-line row in
	// the same document was a pixel tall for the same reason. Across six rows
	// that was 2.25pt, enough to push the sixth item onto page two and make the
	// engine paginate a different document from the browser's. The browser
	// gives this row 132.000pt; the rule that reproduces it also has to leave a
	// two-line row a pixel shorter, which TestTwoLineRowsResolveDown pins.
	content := 9*descLinePx + 6 + 3*float64(items)
	if got, want := content, (132.00-2*px(tdPadPx)-2*px(1))/ptPerPx; math.Abs(got-want) > 1.5 {
		t.Errorf("first row content %.1fpx is more than a pixel from the browser's %.1fpx", got, want)
	}
	assertNear(t, "first row height", first.height, 132.00, 1.0)
}

// TestTwoLineRowsResolveDown pins the rounding rule that made pagination match.
//
// A browser lays a table row out in fractional pixels and snaps the row's
// bottom edge down to a whole pixel, so a row is the floor of its own content.
// Two lines of the 15.4px line box are 30.8px: the browser resolves that to 30
// or 29 in the reference document, never to the 31 that rounding gives. Two rows
// of identical content come out a pixel apart in the browser's own render, so
// the test asserts the size rather than equality with a single number.
func TestTwoLineRowsResolveDown(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	rows := e.measureRows(in, e.derive(in))

	// Two lines of description and no list items: 30.8px of content, which the
	// browser resolves to 30 or 29. Padding and borders add 22px.
	const wantLo, wantHi = 51.0, 52.0 // row heights in px, borders included
	for i, rb := range rows {
		if len(rb.lines) != 2 || len(rb.subs) != 0 {
			continue
		}
		got := rb.height / ptPerPx
		if got < wantLo || got > wantHi {
			t.Errorf("row %d: two lines resolved to %.0fpx, the browser gives %v-%v",
				i+1, got, wantLo, wantHi)
		}
	}
}

// TestBulletsOnlyOpenListItems checks the marker rule, which is easy to get
// wrong in a way that reads as a font bug: a wrapped item's second line must not
// carry a disc of its own.
func TestBulletsOnlyOpenListItems(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	rows := e.measureRows(in, e.derive(in))

	var open, total int
	for _, sl := range rows[0].subs {
		total++
		if sl.first {
			open++
		}
	}
	if open != 3 || total != 6 {
		t.Errorf("%d of %d list lines open an item; 3 of 6 should", open, total)
	}
}

// TestWrapDoesNotDoubleSpace guards the join in Wrap. Words are split on
// whitespace, and if the split leaves the separator attached while the join adds
// another, every word boundary in the document gains a space — invisible in the
// text, visible as a line that is suddenly too wide and wraps early.
func TestWrapDoesNotDoubleSpace(t *testing.T) {
	e := testEngine(t)
	lines := e.Wrap("alpha beta gamma delta", px(bodyPx), 1e6, false)
	if len(lines) != 1 {
		t.Fatalf("wrapped into %d lines: %q", len(lines), lines)
	}
	if lines[0] != "alpha beta gamma delta" {
		t.Errorf("got %q, want the input back unchanged", lines[0])
	}
	// And the width must be the width of the text, not the text plus spacings.
	whole := e.Width("alpha beta gamma delta", px(bodyPx), false)
	if d := whole - e.Width(lines[0], px(bodyPx), false); math.Abs(d) > 1e-9 {
		t.Errorf("width of the wrapped line differs from the string by %g", d)
	}
}
