package gpdf

// Regression tests for issue #485: a table row taller than a page hung the
// native renderer forever.
//
// The layout loop placed whole rows or nothing: a row that did not fit the
// remaining space and did not fit an empty page either left the cursor
// unmoved while appending a fresh page, and the same row was retried forever.
// A single item with enough bullets reaches that state; the 20-bullet control
// below completes in milliseconds while the 30-bullet shape never returns.
//
// Every test that touches an oversized row runs the engine in a goroutine
// behind a bounded wait, so a regression fails the test instead of hanging
// the suite.

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// renderBounded runs Render behind a bounded wait: on current main an
// oversized row hangs inside layout, and this turns that hang into a test
// failure instead of a stuck suite.
func renderBounded(t *testing.T, in Input) []byte {
	t.Helper()
	e := testEngine(t)
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := e.Render(in)
		done <- result{data, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Render: %v", r.err)
		}
		return r.data
	case <-time.After(5 * time.Second):
		t.Fatalf("Render did not return within 5s (issue #485: oversized row hangs layout)")
		return nil
	}
}

// layoutBounded runs the layout behind the same bound and hands back its
// pages, so content assertions read the display list rather than parsing PDF.
func layoutBounded(t *testing.T, in Input) []*page {
	t.Helper()
	e := testEngine(t)
	type result struct {
		pages []*page
	}
	done := make(chan result, 1)
	go func() { done <- result{e.layout(in)} }()
	select {
	case r := <-done:
		return r.pages
	case <-time.After(5 * time.Second):
		t.Fatalf("layout did not return within 5s (issue #485: oversized row hangs layout)")
		return nil
	}
}

// oversizedInput is one asset-free item with n distinct single-line bullets.
// The bullets are distinct so each must appear exactly once; they are pinned
// single-line (each wraps to one line at the bullet width) so a wrapped
// bullet cannot read as a duplicated or missing string.
func oversizedInput(n int) Input {
	in := sampleInput()
	bullets := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		bullets = append(bullets, fmt.Sprintf("Filter inspection tour %03d", i))
	}
	in.Items = []Item{{
		No: 1, DescMain: "Maintenance scope",
		SubItems:  bullets,
		Qty:       "7",
		UoM:       "Lot",
		UnitPrice: "9.999",
		Disc:      "—",
		Total:     "9.999",
	}}
	in.QRPNG = nil
	return in
}

// runTexts gathers every text run on every page, in order.
func runTexts(pages []*page) []string {
	var out []string
	for _, pg := range pages {
		for _, prim := range pg.prims {
			if r, ok := prim.(run); ok {
				out = append(out, r.text)
			}
		}
	}
	return out
}

// countRuns counts exact matches of want across the run texts.
func countRuns(texts []string, want string) int {
	n := 0
	for _, t := range texts {
		if t == want {
			n++
		}
	}
	return n
}

// TestOversizedRowFragmentsAcrossPages is the public regression for #485: one
// item with 200 ordinary bullets must render to a multi-page PDF with every
// bullet and the heading present exactly once.
func TestOversizedRowFragmentsAcrossPages(t *testing.T) {
	in := oversizedInput(200)
	for _, b := range in.Items[0].SubItems {
		e := testEngine(t)
		m := e.derive(sampleInput())
		subW := m.colX[m.cols.qty] - m.colX[m.cols.desc] - px(1) - 2*px(tdPadPx) - px(subIndentPx)
		if got := e.Wrap(b, px(bodyPx), subW, false); len(got) != 1 {
			t.Fatalf("fixture bullet %q wraps to %d lines, want 1: %q", b, len(got), got)
		}
	}

	data := renderBounded(t, in)
	if len(data) == 0 {
		t.Fatal("Render returned no bytes")
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("Render returned %d bytes without a %%PDF header", len(data))
	}

	pages := layoutBounded(t, in)
	if len(pages) <= 1 {
		t.Fatalf("oversized row laid out to %d page(s), want more than one", len(pages))
	}

	texts := runTexts(pages)
	for _, b := range in.Items[0].SubItems {
		if got := countRuns(texts, b); got != 1 {
			t.Errorf("bullet %q appears %d times, want exactly once", b, got)
		}
	}
	if got := countRuns(texts, "Maintenance scope"); got != 1 {
		t.Errorf("item heading appears %d times, want exactly once", got)
	}
}

// TestSyntheticThirtyBoundary pins the exact reported shape: 30 copies of the
// archived bullet complete, and the 20-bullet control still completes.
func TestSyntheticThirtyBoundary(t *testing.T) {
	thirty := sampleInput()
	thirty.Items = []Item{{
		No: 1, DescMain: "Maintenance scope",
		SubItems: func() []string {
			out := make([]string, 0, 30)
			for i := 0; i < 30; i++ {
				out = append(out, "Synthetic maintenance task for the facility.")
			}
			return out
		}(),
		Qty: "1", UoM: "Lot", UnitPrice: "1", Disc: "1", Total: "1",
	}}
	thirty.QRPNG = nil
	if data := renderBounded(t, thirty); len(data) == 0 {
		t.Fatal("30-bullet Render returned no bytes")
	}

	// The 30 bullets wrap to two lines each, so the split falls inside
	// list items: every wrapped line must survive exactly 30 times and
	// every item must keep exactly one disc, wherever the fragments land.
	thirtyPages := layoutBounded(t, thirty)
	if len(thirtyPages) <= 1 {
		t.Fatalf("30-bullet row laid out to %d page(s), want more than one", len(thirtyPages))
	}
	var thirtyTexts []string
	discs := 0
	for _, pg := range thirtyPages {
		for _, prim := range pg.prims {
			switch v := prim.(type) {
			case run:
				thirtyTexts = append(thirtyTexts, v.text)
			case box:
				if v.path != nil {
					discs++
				}
			}
		}
	}
	// discPath is drawn only by the bullet list, so every path box is one
	// item marker; cards draw through the radius field, not a path.
	if discs != 30 {
		t.Errorf("laid out %d bullet discs, the data has 30 items", discs)
	}
	for _, line := range []string{"Synthetic maintenance task for", "the facility."} {
		if got := countRuns(thirtyTexts, line); got != 30 {
			t.Errorf("wrapped line %q appears %d times, want 30", line, got)
		}
	}

	twenty := sampleInput()
	twenty.Items = []Item{{
		No: 1, DescMain: "Maintenance scope",
		SubItems: func() []string {
			out := make([]string, 0, 20)
			for i := 0; i < 20; i++ {
				out = append(out, "Synthetic maintenance task for the facility.")
			}
			return out
		}(),
		Qty: "1", UoM: "Lot", UnitPrice: "1", Disc: "1", Total: "1",
	}}
	twenty.QRPNG = nil
	if data := renderBounded(t, twenty); len(data) == 0 {
		t.Fatal("20-bullet Render returned no bytes")
	}
}

// TestSpilledRowDoesNotRepeatTheHeader pins the header-once rule on the
// non-fragmented spill path: the 20-bullet row never fit the first page, so
// it moves whole to page two, and the header must not follow it there.
//
// Before the fix the loop keyed the header off the row cursor, and a first
// page that placed zero rows repeated the header on the next page (the
// cursor was still zero). The clip comment already stated the header does
// not repeat; the code just failed to uphold it on an empty first page.
func TestSpilledRowDoesNotRepeatTheHeader(t *testing.T) {
	in := sampleInput()
	in.Items = []Item{{
		No: 1, DescMain: "Maintenance scope",
		SubItems: func() []string {
			out := make([]string, 0, 20)
			for i := 0; i < 20; i++ {
				out = append(out, "Synthetic maintenance task for the facility.")
			}
			return out
		}(),
		Qty: "1", UoM: "Lot", UnitPrice: "1", Disc: "1", Total: "1",
	}}
	in.QRPNG = nil
	pages := layoutBounded(t, in)
	if len(pages) <= 1 {
		t.Fatalf("spilled row laid out to %d page(s), want more than one", len(pages))
	}
	var all []string
	for i, pg := range pages {
		var texts []string
		for _, prim := range pg.prims {
			if r, ok := prim.(run); ok {
				texts = append(texts, r.text)
			}
		}
		all = append(all, texts...)
		want := 0
		if i == 0 {
			want = 1
		}
		if got := countRuns(texts, "No."); got != want {
			t.Errorf("page %d: header label %q appears %d times, want %d", i+1, "No.", got, want)
		}
	}
	if got := countRuns(all, "No."); got != 1 {
		t.Errorf("header label prints %d times in the document, want once", got)
	}
}

// TestTailsOnlyRowHeightBandsTheList pins the #485 follow-up the Judge
// caught: a continuation fragment that starts mid-bullet holds only wrapped
// tails (first=false on every line), and its band must still carry the list
// it draws. Before the fix the height gated on an item start existing in the
// slice, so such a fragment collapsed to padding: compose took the fits
// branch and drew the whole tail into a band far too short, losing it past
// the card (or overlapping the trailer) in the PDF.
func TestTailsOnlyRowHeightBandsTheList(t *testing.T) {
	padY := 2 * px(tdPadPx)
	// One bullet wrapping to three lines, exactly as measureRows builds it:
	// the opening line carries the disc, the wrapped tails do not.
	whole := []subLine{
		{text: "first line", first: true},
		{text: "wrapped tail one", first: false},
		{text: "wrapped tail two", first: false},
	}
	// Whole rows band exactly what they always have. The expectation is the
	// pre-fix formula restated (content: lines, the 6px list gap, one 3px
	// margin per item, resolved down, plus padding and borders), so a drift
	// in either the code or this restatement fails loudly.
	oldWhole := math.Floor(3*descLinePx+6+3*1)*ptPerPx + padY + 2*px(1)
	if got := rowHeightWithMin(nil, whole, padY, 0); got != oldWhole {
		t.Errorf("whole-row height %.3f, want the long-standing %.3f", got, oldWhole)
	}
	// A tails-only slice: the continuation after the head kept line one.
	tails := whole[1:]
	if n := countItems(tails); n != 0 {
		t.Fatalf("tails-only slice opens %d items, want none", n)
	}
	oldTails := padY + 2*px(1) // pre-fix: content collapsed to padding
	got := rowHeightWithMin(nil, tails, padY, 0)
	if got <= oldTails {
		t.Errorf("tails-only height %.3f, want strictly more than the padding-only %.3f", got, oldTails)
	}
	// The band carries the 6px gap drawBullets draws for any non-empty list
	// plus every line's box, and no item margin where no item starts.
	wantTails := math.Floor(2*descLinePx+6)*ptPerPx + padY + 2*px(1)
	if got != wantTails {
		t.Errorf("tails-only height %.3f, want gap plus every line %.3f", got, wantTails)
	}
	// Tails that carry later item starts band one 3px margin per start --
	// exactly the margins drawBullets draws for them, because every start
	// sits past the slice's first line and each takes the interior margin.
	mixed := []subLine{
		{text: "wrapped tail", first: false},
		{text: "second bullet", first: true},
		{text: "its tail", first: false},
	}
	wantMixed := math.Floor(3*descLinePx+6+3*1)*ptPerPx + padY + 2*px(1)
	if got := rowHeightWithMin(nil, mixed, padY, 0); got != wantMixed {
		t.Errorf("mixed-tails height %.3f, want %.3f", got, wantMixed)
	}
}

// TestSingleLongBulletSurvivesFragmentation is the Render regression for the
// #485 follow-up: one item with one very long single bullet (300 repeats of
// a mid-bullet token plus a unique terminator, ~6.9k characters). The bullet
// wraps to one token per line, so every continuation but the last starts
// mid-bullet with no item start in the slice -- the shape whose height the
// pre-fix formula collapsed. The PDF must keep every line: the marker exactly
// once, the mid token its full 300 times, and no bullet line past its page's
// card bottom, where the table clip would remove it from the document.
func TestSingleLongBulletSurvivesFragmentation(t *testing.T) {
	const mid = "FILTER-INSPECTION-TOUR"
	const marker = "TERMINUS-UNIQUE-XYZ123"
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		sb.WriteString(mid + " ")
	}
	sb.WriteString(marker)
	bullet := sb.String()
	t.Logf("single bullet length: %d characters", len(bullet))

	in := sampleInput()
	in.Items = []Item{{
		No: 1, DescMain: "Maintenance scope",
		SubItems: []string{bullet},
		Qty:      "1", UoM: "Lot", UnitPrice: "1", Disc: "—", Total: "1",
	}}
	in.QRPNG = nil

	data := renderBounded(t, in)
	if len(data) == 0 {
		t.Fatal("Render returned no bytes")
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("Render returned %d bytes without a %%PDF header", len(data))
	}

	pages := layoutBounded(t, in)
	if len(pages) <= 1 {
		t.Fatalf("long bullet laid out to %d page(s), want more than one", len(pages))
	}
	mids, marks := 0, 0
	for pi, pg := range pages {
		for _, prim := range pg.prims {
			r, ok := prim.(run)
			if !ok {
				continue
			}
			mids += strings.Count(r.text, mid)
			marks += strings.Count(r.text, marker)
			if strings.Contains(r.text, mid) || strings.Contains(r.text, marker) {
				// The trailer fit carries a two-pixel tolerance; a past-the-card
				// tail misses by thousands of points, so this stays strict.
				if r.baseline > pg.bottom+2*ptPerPx {
					t.Errorf("page %d: bullet run %q at %.2fpt, past the %.2fpt card bottom",
						pi+1, r.text, r.baseline, pg.bottom)
				}
			}
		}
	}
	if mids != 300 {
		t.Errorf("mid-bullet token appears %d times in the display list, want 300", mids)
	}
	if marks != 1 {
		t.Errorf("terminating marker appears %d times in the display list, want once", marks)
	}
}

// TestPlanFragmentRejectsAnUnplaceableLine is the no-progress guard: a planner
// that cannot consume even one description or sub line returns a clear error
// instead of an empty fragment, so the layout loop fails rather than
// allocating a zero-progress page.
func TestPlanFragmentRejectsAnUnplaceableLine(t *testing.T) {
	e := testEngine(t)
	lines := []string{"Maintenance scope"}
	subs := []subLine{{text: "Filter inspection tour 001", first: true}}

	// One line needs its line box plus the cell padding and borders; far
	// below that nothing can be consumed, head or continuation alike.
	for _, kind := range []rowKind{rowMain, rowParentTitle, rowScope, rowPart} {
		for _, head := range []bool{true, false} {
			if _, _, _, err := e.planFragment(lines, subs, kind, head, 1.0); err == nil {
				t.Errorf("kind %d head %v: planner accepted 1pt, want a clear error", kind, head)
			}
		}
	}

	// Empty content consumes nothing either way.
	if _, _, _, err := e.planFragment(nil, nil, rowMain, true, 1e6); err == nil {
		t.Error("planner accepted empty content, want an error")
	}
}

// TestPlanFragmentKeepsTheWholeRowWhenItFits pins the planner's other edge:
// content that fits is returned whole, at exactly the height the drawers band
// a short row with, so a fitting row never fragments.
func TestPlanFragmentKeepsTheWholeRowWhenItFits(t *testing.T) {
	e := testEngine(t)
	lines := []string{"Maintenance scope", "second line"}
	subs := []subLine{
		{text: "first bullet", first: true},
		{text: "wrapped tail", first: false},
		{text: "second bullet", first: true},
	}
	for _, kind := range []rowKind{rowMain, rowParentTitle, rowScope, rowPart} {
		for _, head := range []bool{true, false} {
			padY, minPx := fragPadMin(kind, head)
			want := rowHeightWithMin(lines, subs, padY, minPx)
			nl, ns, got, err := e.planFragment(lines, subs, kind, head, 1e6)
			if err != nil {
				t.Errorf("kind %d head %v: %v", kind, head, err)
				continue
			}
			if nl != len(lines) || ns != len(subs) {
				t.Errorf("kind %d head %v: took %d/%d lines and %d/%d subs, want all",
					kind, head, nl, len(lines), ns, len(subs))
			}
			if got != want {
				t.Errorf("kind %d head %v: height %.3f, want %.3f", kind, head, got, want)
			}
		}
	}
}

// TestPlanFragmentSplitsLinesBeforeSubs pins the fragment order: description
// lines first, then sub lines, each scan stopping at the first line that no
// longer fits.
func TestPlanFragmentSplitsLinesBeforeSubs(t *testing.T) {
	e := testEngine(t)
	lines := []string{"line one", "line two", "line three"}
	var subs []subLine
	padY, minPx := fragPadMin(rowMain, true)
	one := rowHeightWithMin(lines[:1], nil, padY, minPx)
	two := rowHeightWithMin(lines[:2], nil, padY, minPx)
	if one >= two {
		t.Fatalf("fixture heights do not grow: %.3f vs %.3f", one, two)
	}
	nl, ns, _, err := e.planFragment(lines, subs, rowMain, true, (one+two)/2)
	if err != nil {
		t.Fatalf("planner: %v", err)
	}
	if nl != 1 || ns != 0 {
		t.Errorf("took %d lines and %d subs, want the 1 line that fits and no subs", nl, ns)
	}
}

// TestContinuationKeepsNumericsOnTheHead pins the fragment contract on the
// laid-out pages: the numeric cells print once, on the first fragment, every
// description line prints exactly once, and no page carries a fragment with
// no description in it.
func TestContinuationKeepsNumericsOnTheHead(t *testing.T) {
	in := oversizedInput(200)
	pages := layoutBounded(t, in)
	if len(pages) <= 1 {
		t.Fatalf("oversized row laid out to %d page(s), want more than one", len(pages))
	}

	perPage := make([][]string, len(pages))
	for i, pg := range pages {
		for _, prim := range pg.prims {
			if r, ok := prim.(run); ok {
				perPage[i] = append(perPage[i], r.text)
			}
		}
	}
	all := func() []string {
		var out []string
		for _, texts := range perPage {
			out = append(out, texts...)
		}
		return out
	}()

	// The numeric cells belong to the head fragment only: each prints once
	// in the whole document and never past the first page.
	for _, cell := range []string{"1", "7", "Lot", "9.999", "—"} {
		if got := countRuns(all, cell); got != countRuns(perPage[0], cell) {
			t.Errorf("cell %q appears %d times overall but %d on the first page",
				cell, got, countRuns(perPage[0], cell))
		}
		for i := 1; i < len(perPage); i++ {
			if got := countRuns(perPage[i], cell); got != 0 {
				t.Errorf("cell %q appears %d times on page %d, want none past the head",
					cell, got, i+1)
			}
		}
	}
	if got := countRuns(all, "1"); got != 1 {
		t.Errorf("row number prints %d times, want once", got)
	}
	if got := countRuns(all, "9.999"); got != 2 {
		t.Errorf("unit price prints %d times, want twice (unit and total columns)", got)
	}

	// Every description line exactly once across the document.
	descSet := map[string]bool{"Maintenance scope": true}
	for _, b := range in.Items[0].SubItems {
		descSet[b] = true
	}
	seen := map[string]int{}
	for _, text := range all {
		if descSet[text] {
			seen[text]++
		}
	}
	for line := range descSet {
		if seen[line] != 1 {
			t.Errorf("description line %q appears %d times, want exactly once", line, seen[line])
		}
	}

	// No page carries a fragment with no description in it: every page holds
	// at least one description run. (Page one also holds the chrome; the
	// continuation pages hold table fragments and, on the last page, the
	// trailer.)
	for i, texts := range perPage {
		found := false
		for _, text := range texts {
			if descSet[text] {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("page %d holds no description line", i+1)
		}
	}

	// The header prints once: the first page carries it and no continuation
	// repeats it.
	for i, texts := range perPage {
		want := 0
		if i == 0 {
			want = 1
		}
		if got := countRuns(texts, "No."); got != want {
			t.Errorf("page %d: header label %q appears %d times, want %d", i+1, "No.", got, want)
		}
	}
}
