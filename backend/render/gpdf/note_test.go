package gpdf

// Guards for the native quotation note card (#534/#540).
//
// The HTML template prints the note; this engine must print the same note in
// the same place. The tests below pin the card's measured height, its wrapping
// and paragraph breaks, bold and italic runs, bullet and numbered rows, the
// editable label, its position between the items and the terms, its atomicity
// across a page break, and that it carries no clip from the items table.

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// noteInput is the sample document carrying one note.
func noteInput(note string) Input {
	in := sampleInput()
	in.Note = note
	return in
}

// noteTexts joins each laid-out note line's spans, so a test can assert the
// printed text and the line structure without depending on span splitting.
func noteTexts(lines []noteLine) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		var b strings.Builder
		for _, span := range line.spans {
			b.WriteString(span.Text)
		}
		out[i] = b.String()
	}
	return out
}

// findRun returns the page index and baseline of the first run whose text is
// exactly want.
func findRun(pages []*page, want string) (int, float64, bool) {
	for i, pg := range pages {
		for _, prim := range pg.prims {
			if r, ok := prim.(run); ok && r.text == want {
				return i, r.baseline, true
			}
		}
	}
	return -1, 0, false
}

// findStyledRun locates the first run with the given text and style.
func findStyledRun(pages []*page, want string, bold, italic bool) (run, bool) {
	for _, pg := range pages {
		for _, prim := range pg.prims {
			if r, ok := prim.(run); ok && r.text == want && r.bold == bold && r.italic == italic {
				return r, true
			}
		}
	}
	return run{}, false
}

// TestNoteCardHeightIsTheStylesheetBox pins the card's height for a two-line
// note against the stylesheet's own box model: 1px borders, 12px vertical
// padding, one 12px label line at 1.45, a 4px gap, and two 11px body lines at
// 1.45. The browser value is the same arithmetic at 0.75pt per CSS pixel; no
// browser was available where this was written, so the literal is the box
// model rather than a fresh reading (the equivalence test re-measures it when
// Chrome is present).
func TestNoteCardHeightIsTheStylesheetBox(t *testing.T) {
	e := testEngine(t)
	in := noteInput("Delivery is ex-works.\nInstallation by the customer.")
	m := e.derive(in)

	if got := len(e.noteLines(in, m)); got != 2 {
		t.Fatalf("note wraps into %d lines; want 2", got)
	}
	// 2 + 24 + 17.4 + 4 + 2*15.95 = 79.3 CSS px = 59.475pt.
	assertNear(t, "two-line note card height", m.noteH, 59.475, 0.01)
}

// TestFormattedNoteCardHeightCountsBlocksAndListRows proves the measured
// height follows the parsed structure: one paragraph line, one line-box gap,
// two bullet rows, another gap, and two numbered rows -- seven line boxes.
func TestFormattedNoteCardHeightCountsBlocksAndListRows(t *testing.T) {
	e := testEngine(t)
	in := noteInput("Intro **bold** *italic*.\n- First\n- Second\n1. One\n2. Two")
	m := e.derive(in)

	texts := noteTexts(e.noteLines(in, m))
	want := []string{"Intro bold italic.", "", "First", "Second", "", "One", "Two"}
	if len(texts) != len(want) {
		t.Fatalf("formatted note laid out %q; want %q", texts, want)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Errorf("note line %d = %q; want %q", i+1, texts[i], want[i])
		}
	}
	wantH := 2*px(1) + 2*px(12) + px(12)*bodyLH + px(4) + 7*px(11)*bodyLH
	assertNear(t, "formatted note card height", m.noteH, wantH, 0.001)
}

// TestNotePreservesParagraphBreaks proves an explicit blank line survives:
// the line is empty, but it still occupies a line box, exactly as the
// stylesheet's block gap prints it.
func TestNotePreservesParagraphBreaks(t *testing.T) {
	e := testEngine(t)
	in := noteInput("First paragraph.\n\nSecond paragraph.")
	m := e.derive(in)

	want := []string{"First paragraph.", "", "Second paragraph."}
	got := noteTexts(e.noteLines(in, m))
	if len(got) != len(want) {
		t.Fatalf("note wraps into %q; want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note line %d = %q; want %q", i+1, got[i], want[i])
		}
	}
	wantH := 2*px(1) + 2*px(12) + px(12)*bodyLH + px(4) + 3*px(11)*bodyLH
	assertNear(t, "paragraph-break note card height", m.noteH, wantH, 0.001)
}

// TestNoteWrapsToTheCardWidth checks the wrapping property rather than a
// particular break: every line fits the card's content width and no word is
// lost between them. The content width is the card less its 3px left accent,
// its 1px right border and 14px of padding on each side.
func TestNoteWrapsToTheCardWidth(t *testing.T) {
	e := testEngine(t)
	text := strings.TrimSpace(strings.Repeat("Please confirm site access before dispatch. ", 6))
	in := noteInput(text)
	m := e.derive(in)

	lines := noteTexts(e.noteLines(in, m))
	if len(lines) < 3 {
		t.Fatalf("the long note wrapped into %d lines; want at least 3: %q", len(lines), lines)
	}
	width := m.innerW - px(3) - px(1) - 2*px(14)
	for i, line := range lines {
		if w := e.Width(line, px(bodyPx), false); w > width+0.01 {
			t.Errorf("note line %d measures %.2fpt, wider than the %.2fpt content box: %q",
				i+1, w, width, line)
		}
	}
	if got := strings.Join(lines, " "); got != text {
		t.Errorf("wrapping lost or reordered words:\n got %q\nwant %q", got, text)
	}
}

// TestNoteRendersBoldAndItalicRuns proves each inline style reaches the draw
// layer as its own run with the right face selected.
func TestNoteRendersBoldAndItalicRuns(t *testing.T) {
	in := noteInput("Please **confirm** access *before* 09:00.")
	pages := testEngine(t).layout(in)

	for _, want := range []struct {
		text   string
		bold   bool
		italic bool
	}{
		{"confirm", true, false},
		{"before", false, true},
	} {
		if _, ok := findStyledRun(pages, want.text, want.bold, want.italic); !ok {
			t.Errorf("the note did not draw %q as bold=%v italic=%v", want.text, want.bold, want.italic)
		}
	}
}

// TestNoteRendersBulletRows proves a bullet list prints one disc marker per
// item inside the note card, indented from the card's text edge.
func TestNoteRendersBulletRows(t *testing.T) {
	e := testEngine(t)
	in := noteInput("- First item\n- Second item")
	m := e.derive(in)
	pages := e.layout(in)

	notePage, noteY, ok := findRun(pages, "Note")
	if !ok {
		t.Fatal("the engine did not print the note label")
	}
	termsPage, termsY, ok := findRun(pages, "Terms and Conditions")
	if !ok {
		t.Fatal("the engine did not print the terms heading")
	}

	discs := 0
	for _, prim := range pages[notePage].prims {
		b, isBox := prim.(box)
		if !isBox || b.path == nil {
			continue
		}
		if b.y > noteY && (termsPage != notePage || b.y < termsY) {
			discs++
			if b.x <= m.innerX {
				t.Errorf("bullet disc sits at x %.2f, not indented past the card edge %.2f", b.x, m.innerX)
			}
		}
	}
	if discs != 2 {
		t.Errorf("the note drew %d bullet discs; want 2", discs)
	}
}

// TestNoteRendersNumberedRows proves a numbered list prints its numbers in the
// marker gutter and the item text to the right of them.
func TestNoteRendersNumberedRows(t *testing.T) {
	in := noteInput("1. Notify security\n2. Log the visit")
	pages := testEngine(t).layout(in)

	for _, want := range []struct {
		marker string
		text   string
	}{
		{"1.", "Notify security"},
		{"2.", "Log the visit"},
	} {
		r, ok := findStyledRun(pages, want.marker, false, false)
		if !ok {
			t.Errorf("the note did not draw the number marker %q", want.marker)
			continue
		}
		_, textX, ok := findRun(pages, want.text)
		if !ok {
			t.Errorf("the note did not draw the text %q for marker %q", want.text, want.marker)
			continue
		}
		if r.x >= textX {
			t.Errorf("number marker %q sits at x %.2f, not left of its text at x %.2f", want.marker, r.x, textX)
		}
	}
}

// TestItalicFaceIsLoadedAndUsed proves the new asset is a face the engine can
// parse, measure with and embed, not just a flag on a run. A regular and an
// italic measurement of the same word must differ, and a document that uses
// the italic face must render through the subsetter without error.
func TestItalicFaceIsLoadedAndUsed(t *testing.T) {
	e := testEngine(t)
	if e.italic == nil || e.italicShape == nil || len(e.italicData) == 0 {
		t.Fatal("the engine did not load the italic face")
	}
	regular := e.widthFor("before", px(bodyPx), false, false)
	italic := e.widthFor("before", px(bodyPx), false, true)
	if math.Abs(italic-regular) < 0.01 {
		t.Errorf("italic and regular measure the same (%.4fpt); the italic face is not being used", regular)
	}
	if _, err := e.Render(noteInput("Please *confirm* access before 09:00.")); err != nil {
		t.Fatalf("render a note with italic: %v", err)
	}
}

// TestNoteLabelComesFromTheSeam proves the card's heading is the editable
// label the application handed over, not a literal in this package.
func TestNoteLabelComesFromTheSeam(t *testing.T) {
	in := noteInput("Delivery is ex-works.")
	in.Labels.Note = "Catatan"
	pages := testEngine(t).layout(in)

	texts := runTexts(pages)
	if got := countRuns(texts, "Catatan"); got != 1 {
		t.Errorf("the engine printed the reworded note label %d times; want once", got)
	}
	if got := countRuns(texts, "Note"); got != 0 {
		t.Errorf("the engine still printed the built-in Note label %d times", got)
	}
}

// TestNotePrintsBetweenTheItemsAndTheTerms pins the position both renderers
// share: below the last line row and above the terms.
func TestNotePrintsBetweenTheItemsAndTheTerms(t *testing.T) {
	in := noteInput("Delivery is ex-works.\nInstallation by the customer.")
	pages := testEngine(t).layout(in)

	notePage, noteY, ok := findRun(pages, "Note")
	if !ok {
		t.Fatal("the engine did not print the note label")
	}
	bodyPage, bodyY, ok := findRun(pages, "Delivery is ex-works.")
	if !ok {
		t.Fatal("the engine did not print the note body")
	}
	termsPage, termsY, ok := findRun(pages, "Terms and Conditions")
	if !ok {
		t.Fatal("the engine did not print the terms heading")
	}
	if bodyPage != notePage || bodyY <= noteY {
		t.Errorf("the note body is not below its label: label page %d y %.2f, body page %d y %.2f",
			notePage+1, noteY, bodyPage+1, bodyY)
	}
	if notePage > termsPage || (notePage == termsPage && noteY >= termsY) {
		t.Errorf("the note is not above the terms: note page %d y %.2f, terms page %d y %.2f",
			notePage+1, noteY, termsPage+1, termsY)
	}
}

// TestNoteCardIsNotClippedByTheItemsTable guards the drawing seam: the note's
// card boxes must not carry the items table's clip, or the card would be cut
// off where the table's outline ends. Its accent is clipped to the card's own
// rounded outline, which is a different region.
func TestNoteCardIsNotClippedByTheItemsTable(t *testing.T) {
	e := testEngine(t)
	in := noteInput("Delivery is ex-works.\nInstallation by the customer.")
	m := e.derive(in)
	pages := e.layout(in)

	notePage, noteY, ok := findRun(pages, "Note")
	if !ok {
		t.Fatal("the engine did not print the note label")
	}

	cardBoxes, accents := 0, 0
	for _, prim := range pages[notePage].prims {
		b, isBox := prim.(box)
		if !isBox || b.x != m.innerX {
			continue
		}
		if b.w == m.innerW && b.y <= noteY && noteY <= b.y+b.h {
			cardBoxes++
			if b.clip != nil {
				t.Errorf("a note card box is clipped (x %.2f y %.2f w %.2f h %.2f); "+
					"the note must not inherit the items table's clip", b.x, b.y, b.w, b.h)
			}
		}
		if b.w == px(3) && b.clip != nil {
			accents++
			if math.Abs(b.clip.h-m.noteH) > 0.01 {
				t.Errorf("the note accent clip is %.2fpt tall, want the note card's %.2fpt",
					b.clip.h, m.noteH)
			}
		}
	}
	if cardBoxes < 2 {
		t.Errorf("found %d unclipped note card boxes; want the fill and the border", cardBoxes)
	}
	if accents != 1 {
		t.Errorf("found %d clipped note accents; want exactly one", accents)
	}
}

// TestNoteStartsAFreshPageWhenItDoesNotFit proves the card is atomic: a note
// too tall for what the last table page has left moves whole to the next page
// rather than being split or squeezed.
func TestNoteStartsAFreshPageWhenItDoesNotFit(t *testing.T) {
	// 55 lines are about 693pt: too tall for what the last table page has
	// left after its rows, but comfortably shorter than an empty page.
	lines := make([]string, 55)
	for i := range lines {
		lines[i] = "Note line " + itoa(i+1) + "."
	}
	in := noteInput(strings.Join(lines, "\n"))
	pages := testEngine(t).layout(in)

	notePage, _, ok := findRun(pages, "Note")
	if !ok {
		t.Fatal("the engine did not print the note label")
	}
	if notePage == 0 {
		t.Fatalf("the note shared page 1 with the items table; it must start a fresh page when it does not fit")
	}
	// A continuation page carries no chrome, so a fresh note page begins with
	// the note label itself.
	if texts := runTexts([]*page{pages[notePage]}); len(texts) == 0 || texts[0] != "Note" {
		t.Errorf("the fresh note page does not begin with the note card; first runs: %q", texts)
	}
	for i, line := range lines {
		if got := countRuns(runTexts(pages), line); got != 1 {
			t.Errorf("note line %d %q printed %d times; want once", i+1, line, got)
		}
	}
}

// TestFormattedNoteStartsAFreshPageWhenItDoesNotFit extends the atomicity
// guard to the rich-text layout: many bold bullet rows still move as one card.
func TestFormattedNoteStartsAFreshPageWhenItDoesNotFit(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "- **Note** line " + itoa(i+1) + "."
	}
	in := noteInput("Intro paragraph.\n" + strings.Join(lines, "\n"))
	pages := testEngine(t).layout(in)

	notePage, _, ok := findRun(pages, "Note")
	if !ok {
		t.Fatal("the engine did not print the note label")
	}
	if notePage == 0 {
		t.Fatalf("the formatted note shared page 1 with the items table; it must start a fresh page when it does not fit")
	}
	for i := range lines {
		want := " line " + itoa(i+1) + "."
		if got := countRuns(runTexts(pages), want); got != 1 {
			t.Errorf("formatted note line %d %q printed %d times; want once", i+1, want, got)
		}
	}
}

// TestEmptyNoteLeavesTheDocumentUnchanged is the "most quotations are
// unchanged" rule at the strongest seam available without a browser: the PDF
// bytes of a note-less document and of a whitespace-only note are identical.
func TestEmptyNoteLeavesTheDocumentUnchanged(t *testing.T) {
	base := sampleInput()

	explicitEmpty := sampleInput()
	explicitEmpty.Note = ""

	spaces := sampleInput()
	spaces.Note = " \n\t "

	basePDF := renderBounded(t, base)
	if !bytes.Equal(basePDF, renderBounded(t, explicitEmpty)) {
		t.Error("an explicit empty note changed the rendered document")
	}
	if !bytes.Equal(basePDF, renderBounded(t, spaces)) {
		t.Error("a whitespace-only note changed the rendered document; it must print nothing")
	}
}
