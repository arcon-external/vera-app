package gpdf

// Guard for the totals stack: the totals card hugs its own rows and the
// decoration below it flexes into the footer row's leftover, the same shape as
// the metadata stack above it.
//
// Without it the card was stretched to the footer row and the surplus shared
// across its rows (`align-content: stretch`), so a quotation with no PPN row
// spread two rows across a four-row-tall card. There was no test that would
// catch a filler in one renderer and not the other, and the equivalence
// comparison renders the 2-row shape -- its numeric DiscountAmount/PpnAmount
// are 0, so neither conditional row prints there either -- which is why the
// PPN, discount and combined shapes are pinned here instead.
//
// The footer row counts the whole stack -- card plus gap plus the decoration's
// border-box floor -- mirroring infoHeight's `maxF(recv, metaHeight +
// metaStackGap + metaDecorFloor)`. Counting only the taller card put the closing 12px apart
// between the two renderers in the 4-row state: the browser's row grows past
// the terms card while the native row stayed.
//
// sampleInput carries three term strings, while production always renders
// four (termsLines emits the four fixed terms, matching the template),
// so the three-row shapes below use fourTerms: with the sample's three terms
// the terms card is shorter than any real document's, which is what once made
// a three-row card look filler-free here while real documents painted one.

import (
	"math"
	"testing"
)

// fourTerms is the production terms shape: the four fixed one-line terms the
// template hard-codes (template-fixed.html) and termsLines emits. Each wraps
// to one line at the terms track width, so the terms card is 98.4pt.
//
// The lead time term is the composed sentence, not a bare number: "60 Days"
// for a day-only lead time, exactly what the application hands both
// renderers.
func fourTerms() []string {
	return []string{
		"1. Payment terms agreed for this quotation is 90 days.",
		"2. This quotation is valid for 30 days.",
		"3. Lead time is 60 Days.",
		"4. Incoterms: FOB",
	}
}

// noPPNInput is the PPH22 / FTZ shape: services carry no PPN row, so the
// totals card is two rows (Sub Total, Grand Total) beside a taller terms card.
func noPPNInput() Input {
	in := sampleInput()
	in.Terms = fourTerms()
	in.PpnAmount = 0
	in.DisplayPpn = ""
	in.DisplayGrandTotal = "270.000"
	return in
}

// ppnInput is the three-row PPN shape beside the production terms card: the
// sample fixture carries a non-zero PpnAmount, so the PPN row prints -- the
// condition reads the numeric, the same value the template's
// {{if .PpnAmount}} tests (issue #377) -- and no whole-quotation discount
// means no discount row.
func ppnInput() Input {
	in := sampleInput() // PpnAmount 76592203: the PPN row prints.
	in.Terms = fourTerms()
	return in
}

// ppnDiscountInput is the four-row state: a whole-quotation discount plus PPN
// beside the production terms card. The card plus the stack gap already
// exceeds the terms card here, so the row is decided by the stack.
func ppnDiscountInput() Input {
	in := ppnInput()
	in.DiscountAmount = 5000000
	in.DisplayDiscount = "13.500"
	in.DiscountPercent = "5"
	return in
}

// footerBoxes runs drawFooterCards alone on a fresh page and returns its boxes.
func footerBoxes(e *Engine, in Input, m metrics, top float64) []box {
	p := &page{}
	e.drawFooterCards(p, in, m, top)
	var out []box
	for _, pr := range p.prims {
		if b, ok := pr.(box); ok {
			out = append(out, b)
		}
	}
	return out
}

// footerRuns runs drawFooterCards alone on a fresh page and returns its runs.
func footerRuns(e *Engine, in Input, m metrics, top float64) []run {
	p := &page{}
	e.drawFooterCards(p, in, m, top)
	var out []run
	for _, pr := range p.prims {
		if r, ok := pr.(run); ok {
			out = append(out, r)
		}
	}
	return out
}

func baselineOf(runs []run, text string) (float64, bool) {
	for _, r := range runs {
		if r.text == text {
			return r.baseline, true
		}
	}
	return 0, false
}

// TestTotalsCardHugsItsContent pins the derive half: with no PPN row the card
// is its content height, not the terms card's, and the decoration is the row's
// leftover after the card and the stack gap.
func TestTotalsCardHugsItsContent(t *testing.T) {
	e := testEngine(t)
	in := noPPNInput()
	m := e.derive(in)
	rows := e.totalsRows(in)

	if len(rows) != 2 {
		t.Fatalf("no-PPN totals card has %d rows, want 2 (Sub Total, Grand Total)", len(rows))
	}
	content := e.totalsHeight(rows)
	if m.footerTotalsH != content {
		t.Errorf("totals card is %.3fpt, content height is %.3fpt: the card is not hugging its rows",
			m.footerTotalsH, content)
	}
	if m.footerTotalsH >= m.footerCardsH {
		t.Errorf("totals card %.3fpt fills the %.3fpt footer row: no gap case to fill",
			m.footerTotalsH, m.footerCardsH)
	}
	// The row height counts the whole stack -- card plus gap plus the
	// decoration's floor -- mirroring infoHeight. The terms card dominates
	// here, so the value is the terms card either way; the formula is what
	// keeps the 4-row state from staying behind.
	tncH := e.tncHeight(in, m.footerTncW)
	if want := maxF(tncH, content+totalsStackGap+totalsDecorFloor); m.footerCardsH != want {
		t.Errorf("footer row is %.3fpt, max of the terms card and the stack is %.3fpt: pagination moved",
			m.footerCardsH, want)
	}
	if m.footerTncH != m.footerCardsH {
		t.Errorf("terms card is %.3fpt, footer row is %.3fpt: the grid must stretch it",
			m.footerTncH, m.footerCardsH)
	}
	wantDecor := m.footerCardsH - m.footerTotalsH - totalsStackGap
	if m.footerTotalsDecorH != wantDecor {
		t.Errorf("filler is %.3fpt, row minus card minus gap is %.3fpt",
			m.footerTotalsDecorH, wantDecor)
	}
	if m.footerTotalsDecorH <= 0 {
		t.Errorf("filler is %.3fpt, not positive: nothing would draw below the card",
			m.footerTotalsDecorH)
	}
	t.Logf("  card %.3fpt, row %.3fpt, filler %.3fpt (gap %.3fpt)",
		m.footerTotalsH, m.footerCardsH, m.footerTotalsDecorH, float64(totalsStackGap))
}

// TestTotalsFillerClosesTheRow pins the drawing half: the filler starts below
// the card plus the gap and ends exactly at the footer row's bottom edge.
func TestTotalsFillerClosesTheRow(t *testing.T) {
	e := testEngine(t)
	in := noPPNInput()
	m := e.derive(in)

	const top = 100.0
	totX := m.innerX + m.footerTncW + px(footGapPx)
	var filler *box
	var card *box
	for _, b := range footerBoxes(e, in, m, top) {
		if b.gradient == nil || b.x != totX {
			continue
		}
		b := b
		filler = &b
	}
	for _, b := range footerBoxes(e, in, m, top) {
		if b.gradient != nil || b.fill != e.pal.Surface || b.x != totX {
			continue
		}
		b := b
		card = &b
	}
	if card == nil {
		t.Fatal("no totals card box at the totals column")
	}
	if card.h != m.footerTotalsH {
		t.Errorf("drawn card is %.3fpt, derived card height is %.3fpt",
			card.h, m.footerTotalsH)
	}
	if filler == nil {
		t.Fatal("no filler drawn below the totals card")
	}
	if filler.y != top+m.footerTotalsH+totalsStackGap {
		t.Errorf("filler starts at %.3f, card plus gap ends at %.3f",
			filler.y, top+m.footerTotalsH+totalsStackGap)
	}
	if bottom := filler.y + filler.h; math.Abs(bottom-(top+m.footerCardsH)) > 1e-9 {
		t.Errorf("filler ends at %.3f, footer row ends at %.3f: the column does not close",
			bottom, top+m.footerCardsH)
	}
	if filler.w != m.footerTotW {
		t.Errorf("filler is %.3fpt wide, totals track is %.3fpt", filler.w, m.footerTotW)
	}
	t.Logf("  filler y=%.3f h=%.3f ends %.3f, row ends %.3f",
		filler.y, filler.h, filler.y+filler.h, top+m.footerCardsH)
}

// TestTotalsFillerIsTheMetadataFillersTwin pins the shape contract: same gap,
// same border, same radius, same gradient stops. The brand theme drives both
// through the same tokens, so a rebrand cannot split them.
func TestTotalsFillerIsTheMetadataFillersTwin(t *testing.T) {
	if totalsStackGap != metaStackGap {
		t.Errorf("totals stack gap %.3fpt != metadata stack gap %.3fpt",
			float64(totalsStackGap), float64(metaStackGap))
	}

	e := testEngine(t)
	in := noPPNInput()
	m := e.derive(in)

	const top = 100.0
	totX := m.innerX + m.footerTncW + px(footGapPx)
	var filler *box
	for _, b := range footerBoxes(e, in, m, top) {
		if b.gradient != nil && b.x == totX {
			b := b
			filler = &b
		}
	}
	if filler == nil {
		t.Fatal("no filler drawn below the totals card")
	}
	if filler.stroke != e.pal.Edge {
		t.Errorf("filler border %q, metadata filler border %q (Edge)", filler.stroke, e.pal.Edge)
	}
	if filler.radius != px(cardRadiusPx) {
		t.Errorf("filler radius %.3f, metadata filler radius %.3f", filler.radius, px(cardRadiusPx))
	}
	if filler.gradient == nil {
		t.Fatal("filler carries no gradient")
	}
	if filler.gradient.c0 != e.pal.Decor0 || filler.gradient.c1 != e.pal.Decor1 {
		t.Errorf("filler gradient %v, metadata filler gradient {%s %s}",
			filler.gradient, e.pal.Decor0, e.pal.Decor1)
	}
}

// TestTotalsRowsDrawAtNaturalPitch pins the stretch removal: the rows sit at
// the stylesheet's own pitch with no surplus shared across them. The old code
// added ~15pt of stretch per row in the no-PPN fixture, so this trips on any
// return of the spreading.
func TestTotalsRowsDrawAtNaturalPitch(t *testing.T) {
	e := testEngine(t)
	in := noPPNInput()
	m := e.derive(in)

	const top = 100.0
	runs := footerRuns(e, in, m, top)
	sub, ok := baselineOf(runs, "Sub Total")
	if !ok {
		t.Fatal(`no "Sub Total" run drawn`)
	}
	grand, ok := baselineOf(runs, "Grand Total")
	if !ok {
		t.Fatal(`no "Grand Total" run drawn`)
	}
	// One body line box, the row gap, the grand rule and its top padding: the
	// stylesheet's own pitch, with both baselines carrying the same offset.
	want := px(bodyPx)*bodyLH + px(totalsGap) + px(2) + px(grandPadTop)
	if got := grand - sub; math.Abs(got-want) > 0.01 {
		t.Errorf("Sub Total -> Grand Total spans %.3fpt, natural pitch is %.3fpt: rows are spreading",
			got, want)
	}
}

// TestTotalsWithPPNLeavesAFiller pins the three-row production shape: with
// the template's four terms the PPN card is shorter than the terms card, so
// the leftover after the card and the gap is positive and a filler draws.
// The sample fixture's three terms made this look filler-free; real documents
// were never that shape.
func TestTotalsWithPPNLeavesAFiller(t *testing.T) {
	e := testEngine(t)
	in := ppnInput()
	m := e.derive(in)
	rows := e.totalsRows(in)

	if len(rows) != 3 {
		t.Fatalf("PPN totals card has %d rows, want 3 (Sub Total, PPN 11%%, Grand Total)", len(rows))
	}
	content := e.totalsHeight(rows)
	if m.footerTotalsH != content {
		t.Errorf("totals card is %.3fpt, content height is %.3fpt", m.footerTotalsH, content)
	}
	tncH := e.tncHeight(in, m.footerTncW)
	if want := maxF(tncH, content+totalsStackGap+totalsDecorFloor); m.footerCardsH != want {
		t.Errorf("footer row is %.3fpt, max of the terms card and the stack is %.3fpt", m.footerCardsH, want)
	}
	if m.footerCardsH != tncH {
		t.Errorf("footer row is %.3fpt, terms card is %.3fpt: the terms side must decide this state",
			m.footerCardsH, tncH)
	}
	if m.footerTotalsDecorH <= 0 {
		t.Fatalf("three-row production shape leaves a %.3fpt filler: nothing would draw below the card",
			m.footerTotalsDecorH)
	}

	const top = 100.0
	totX := m.innerX + m.footerTncW + px(footGapPx)
	var filler *box
	for _, b := range footerBoxes(e, in, m, top) {
		if b.gradient != nil && b.x == totX {
			b := b
			filler = &b
		}
	}
	if filler == nil {
		t.Fatal("no filler drawn below the three-row totals card")
	}
	if bottom := filler.y + filler.h; math.Abs(bottom-(top+m.footerCardsH)) > 1e-9 {
		t.Errorf("filler ends at %.3f, footer row ends at %.3f: the column does not close",
			bottom, top+m.footerCardsH)
	}
	t.Logf("  card %.3fpt, row %.3fpt, filler %.3fpt: filler paints, as drawn",
		m.footerTotalsH, m.footerCardsH, m.footerTotalsDecorH)
}

// TestTotalsWithPPNAndDiscountLeavesFloorFiller pins the four-row state, the
// one that split the renderers: PPN plus a whole-quotation discount makes the
// card plus the gap taller than the terms card, so the row is the stack's and
// the decoration is exactly its border-box floor -- a 2px sliver, drawn in
// both renderers. Dropping the gap or the floor from the row formula puts the
// row back at the terms card and the closing 12px apart again.
func TestTotalsWithPPNAndDiscountLeavesFloorFiller(t *testing.T) {
	e := testEngine(t)
	in := ppnDiscountInput()
	m := e.derive(in)
	rows := e.totalsRows(in)

	if len(rows) != 4 {
		t.Fatalf("PPN-plus-discount totals card has %d rows, want 4", len(rows))
	}
	content := e.totalsHeight(rows)
	if m.footerTotalsH != content {
		t.Errorf("totals card is %.3fpt, content height is %.3fpt", m.footerTotalsH, content)
	}
	tncH := e.tncHeight(in, m.footerTncW)
	stack := content + totalsStackGap + totalsDecorFloor
	if stack <= tncH {
		t.Fatalf("four-row stack %.3fpt does not exceed the terms card %.3fpt: not the state that split",
			stack, tncH)
	}
	if m.footerCardsH != stack {
		t.Errorf("footer row is %.3fpt, the stack is %.3fpt: the row must follow the stack here",
			m.footerCardsH, stack)
	}
	if m.footerTncH != m.footerCardsH {
		t.Errorf("terms card is %.3fpt, footer row is %.3fpt: the grid must stretch it",
			m.footerTncH, m.footerCardsH)
	}
	if m.footerTotalsDecorH != totalsDecorFloor {
		t.Errorf("filler is %.3fpt, the decoration floor is %.3fpt",
			m.footerTotalsDecorH, float64(totalsDecorFloor))
	}

	const top = 100.0
	totX := m.innerX + m.footerTncW + px(footGapPx)
	var filler *box
	for _, b := range footerBoxes(e, in, m, top) {
		if b.gradient != nil && b.x == totX {
			b := b
			filler = &b
		}
	}
	if filler == nil {
		t.Fatal("no floor filler drawn below the four-row totals card")
	}
	if filler.y != top+m.footerTotalsH+totalsStackGap {
		t.Errorf("filler starts at %.3f, card plus gap ends at %.3f",
			filler.y, top+m.footerTotalsH+totalsStackGap)
	}
	if bottom := filler.y + filler.h; math.Abs(bottom-(top+m.footerCardsH)) > 1e-9 {
		t.Errorf("filler ends at %.3f, footer row ends at %.3f: the column does not close",
			bottom, top+m.footerCardsH)
	}
	t.Logf("  card %.3fpt, row %.3fpt, filler %.3fpt (floor): sliver paints, as drawn",
		m.footerTotalsH, m.footerCardsH, m.footerTotalsDecorH)
}

// TestTotalsPPNRowFollowsTheNumeric pins issue #377: the PPN row is decided
// by the numeric PpnAmount -- the value the template's {{if .PpnAmount}}
// tests -- never by the display string's emptiness. A caller handing the
// engine a stray "0" display string with no PPN charged must not draw a
// "PPN 11% Rp0" row the browser never prints; under the old condition
// (DisplayPpn != "") it did, and the two renderers could disagree with no
// code path in the app to catch it.
//
// Both engine outputs are checked: the totalsRows list, which the layout
// derives its heights from, and the runs drawFooterCards paints, which is
// what reaches the page.
func TestTotalsPPNRowFollowsTheNumeric(t *testing.T) {
	e := testEngine(t)

	hasPPNRow := func(rows []totalRow) bool {
		for _, r := range rows {
			if r.label == "PPN 11%" {
				return true
			}
		}
		return false
	}
	ppnPainted := func(in Input) bool {
		m := e.derive(in)
		const top = 100.0
		for _, r := range footerRuns(e, in, m, top) {
			if r.text == "PPN 11%" {
				return true
			}
		}
		return false
	}

	t.Run("no PPN charged draws no row even with a display string", func(t *testing.T) {
		in := sampleInput()
		in.PpnAmount = 0
		in.DisplayPpn = "0" // the stray string: it must not decide anything
		rows := e.totalsRows(in)
		if len(rows) != 2 {
			t.Errorf("totals card has %d rows, want 2 (Sub Total, Grand Total)", len(rows))
		}
		if hasPPNRow(rows) {
			t.Errorf("PPN row drawn from display string %q with PpnAmount 0: the browser prints no such row", in.DisplayPpn)
		}
		if ppnPainted(in) {
			t.Error("drawFooterCards painted a PPN 11% run the template never prints")
		}
	})

	t.Run("PPN charged draws the row", func(t *testing.T) {
		in := sampleInput() // PpnAmount 76592203, as a quotation with PPN carries
		rows := e.totalsRows(in)
		if len(rows) != 3 {
			t.Errorf("totals card has %d rows, want 3 (Sub Total, PPN 11%%, Grand Total)", len(rows))
		}
		if !hasPPNRow(rows) {
			t.Error("PpnAmount > 0 dropped the PPN row the template prints")
		}
		if !ppnPainted(in) {
			t.Error("drawFooterCards drew no PPN 11% run for a quotation with PPN charged")
		}
	})
}
