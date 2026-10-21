package gpdf

// Unsplit description lines (issue #485, feature 2) on the native path.
//
// A plain item's description lines arrive as structured DescriptionLines and
// lay out as one scope row each -- the same merged row the split/prices-off
// side draws -- while the main row keeps its own values and no bullet list.

import (
	"testing"

	"vera-app/backend/render/quotation"
)

// descLinesInput is one plain item with two description lines and a second
// plain item without any. Prices are distinct per column so a leaked price
// cell on a line row reads as the wrong figure, not a duplicate.
func descLinesInput() Input {
	in := sampleInput()
	in.Items = []Item{{
		No: 1, DescMain: "Overhaul of pump P-101:",
		DescLines: []DescLine{
			{No: "1.1", Text: "Replace mechanical seal"},
			{No: "1.2", Text: "Realign coupling"},
		},
		Qty: "2", UoM: "Lot", UnitPrice: "12.500.000", Disc: "11.250.000", Total: "22.500.000",
	}, {
		No: 2, DescMain: "Freight to site",
		Qty: "1", UoM: "Lot", UnitPrice: "1.500.000", Disc: "1.500.000", Total: "1.500.000",
	}}
	in.QRPNG = nil
	return in
}

// TestEngineItemsMapsDescriptionLines pins the seam mapping: the plain
// bullets never return to SubItems, and the main text carries no list.
func TestEngineItemsMapsDescriptionLines(t *testing.T) {
	items := engineItems([]quotation.Item{{
		No:          1,
		DisplayDesc: `<div class="desc-main">Overhaul of pump P-101:</div>`,
		DescriptionLines: []quotation.DescriptionLine{
			{No: "1.1", Text: "Replace mechanical seal"},
			{No: "1.2", Text: "Realign coupling"},
		},
		Qty: 2, UoM: "Lot", UnitPrice: "12.500.000", Total: "22.500.000", Disc: "11.250.000",
	}})
	if len(items) != 1 {
		t.Fatalf("engine items = %d, want 1", len(items))
	}
	got := items[0]
	if got.DescMain != "Overhaul of pump P-101:" {
		t.Errorf("DescMain = %q, want the main text without markup", got.DescMain)
	}
	if len(got.SubItems) != 0 {
		t.Errorf("SubItems = %q, want empty: plain bullets must not return to the main row", got.SubItems)
	}
	if len(got.DescLines) != 2 || got.DescLines[0] != (DescLine{No: "1.1", Text: "Replace mechanical seal"}) ||
		got.DescLines[1] != (DescLine{No: "1.2", Text: "Realign coupling"}) {
		t.Errorf("DescLines = %+v, want the numbered lines", got.DescLines)
	}
	if got.UnitPrice != "12.500.000" || got.Total != "22.500.000" {
		t.Errorf("main figures moved: unit %q total %q", got.UnitPrice, got.Total)
	}
}

// TestDescriptionLinesLayOutAsScopeRows pins the row model: a plain item with
// two lines measures as its main row plus two scope rows carrying the X.Y
// numbers, and the scope rows carry no numeric cells of their own.
func TestDescriptionLinesLayOutAsScopeRows(t *testing.T) {
	e := testEngine(t)
	in := descLinesInput()
	m := e.derive(in)
	rows := e.measureRows(in, m)

	if len(rows) != 4 {
		t.Fatalf("row blocks = %d, want 4 (two mains plus two line rows)", len(rows))
	}
	if rows[0].kind != rowMain || rows[3].kind != rowMain {
		t.Errorf("kinds = %v, %v, %v, %v; want main, scope, scope, main",
			rows[0].kind, rows[1].kind, rows[2].kind, rows[3].kind)
	}
	for i, want := range []string{"1.1", "1.2"} {
		rb := rows[1+i]
		if rb.kind != rowScope {
			t.Errorf("block %d kind = %v, want rowScope", 1+i, rb.kind)
		}
		if rb.part == nil || rb.part.SubNo != want {
			t.Errorf("block %d sub number = %+v, want %s", 1+i, rb.part, want)
		}
		if len(rb.subs) != 0 {
			t.Errorf("block %d carries %d bullet subs, want none", 1+i, len(rb.subs))
		}
	}
}

// TestDescriptionLinesRenderOnce pins the draw: Render completes, each line
// appears exactly once, the main row keeps its own figures, and the line
// rows carry no price cells.
func TestDescriptionLinesRenderOnce(t *testing.T) {
	in := descLinesInput()
	data := renderBounded(t, in)
	if len(data) == 0 {
		t.Fatal("Render returned no bytes")
	}

	pages := layoutBounded(t, in)
	texts := runTexts(pages)
	for _, want := range []string{"Overhaul of pump P-101:", "Replace mechanical seal", "Realign coupling", "Freight to site"} {
		if got := countRuns(texts, want); got != 1 {
			t.Errorf("text %q appears %d times, want exactly once", want, got)
		}
	}
	for _, want := range []string{"1.1", "1.2"} {
		if got := countRuns(texts, want); got != 1 {
			t.Errorf("line number %q appears %d times, want exactly once", want, got)
		}
	}
	if got := countRuns(texts, "22.500.000"); got != 1 {
		t.Errorf("main total appears %d times, want exactly once (on the main row)", got)
	}
}

// TestManyDescriptionLinesFragment pins the pagination path: 200 long
// description lines fragment across pages with no lost or duplicated line.
func TestManyDescriptionLinesFragment(t *testing.T) {
	in := sampleInput()
	lines := make([]DescLine, 0, 200)
	for i := 1; i <= 200; i++ {
		lines = append(lines, DescLine{
			No:   "1." + itoa(i),
			Text: "Filter inspection tour number " + itoa(i) + " across the facility",
		})
	}
	in.Items = []Item{{
		No: 1, DescMain: "Maintenance scope", DescLines: lines,
		Qty: "7", UoM: "Lot", UnitPrice: "9.999", Disc: "—", Total: "9.999",
	}}
	in.QRPNG = nil

	data := renderBounded(t, in)
	if len(data) == 0 {
		t.Fatal("Render returned no bytes")
	}
	pages := layoutBounded(t, in)
	if len(pages) <= 1 {
		t.Fatalf("200 description lines laid out to %d page(s), want more than one", len(pages))
	}
	texts := runTexts(pages)
	for _, dl := range lines {
		if got := countRuns(texts, dl.Text); got != 1 {
			t.Errorf("line %q appears %d times, want exactly once", dl.Text, got)
		}
	}
	if got := countRuns(texts, "Maintenance scope"); got != 1 {
		t.Errorf("item heading appears %d times, want exactly once", got)
	}
}
