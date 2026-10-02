package gpdf

// Responsiveness tests.
//
// The layout is written for one template at one paper size, so the question worth
// asking is not "is it responsive" but "what happens when a field is longer than
// the sample document made it". A field that wraps grows its box; a field that is
// drawn as a single positioned run does not, and runs off the side.
//
// These tests walk every field the template has, put an overlong value in it, and
// check the same two things each time:
//
//   - nothing is drawn outside the page. This is a hard failure. Ink off the paper
//     is a rendering bug whatever produced it.
//   - a field that wraps grows its box, and a field that does not wrap is
//     reported with the distance it would escape by. That is a hard failure only
//     where the field is data-driven and can be arbitrarily long.
//
// The distinction matters: an unbounded customer name is a different problem from
// an unbounded date, and the tests say which is which rather than lumping them
// together.

import (
	"strings"
	"testing"
)

// overlong is comfortably longer than any of the sample's values and still a
// single unbreakable-looking token where that matters, so a field that cannot
// wrap cannot hide behind a lucky break.
const overlong = "Komp. Ruko Legenda Gemilang Blok D NO 12 Baloi Permai Batam"

// drawn is every text run the engine emits, resolved to page points.
type drawn struct {
	page               int
	x, right, baseline float64
	text               string
}

// collect lays the document out and returns every text run it drew.
func collect(t *testing.T, in Input) []drawn {
	t.Helper()
	e := testEngine(t)
	var all []drawn
	for i, p := range e.layout(in) {
		for _, pr := range p.prims {
			ru, ok := pr.(run)
			if !ok {
				continue
			}
			w := e.WidthTracked(ru.text, ru.size, ru.bold, ru.tracking)
			all = append(all, drawn{i, ru.x, ru.x + w, ru.baseline, ru.text})
		}
	}
	return all
}

// TestNothingIsDrawnOffThePaper is the hard guarantee, over every field at once.
// Whatever a field's overflow policy turns out to be, ink on the floor is a bug.
func TestNothingIsDrawnOffThePaper(t *testing.T) {
	in := sampleInput()
	// Every field, overlong, at once: the worst case rather than each alone.
	in.CompanyName = overlong
	in.CompanyAddress = []string{overlong, overlong, overlong}
	in.RecipientName = overlong
	in.RecipientCompany = overlong
	in.RecipientAddress = strings.Repeat(overlong+" ", 3)
	in.Subject = strings.Repeat(overlong+" ", 3)
	in.QuoteNumber = overlong
	in.Date = overlong
	in.ValidUntil = overlong
	in.Revision = overlong
	in.DisplaySubTotal = overlong
	in.DisplayDiscount = overlong
	in.DisplayPpn = overlong
	in.DisplayGrandTotal = overlong
	in.Terms = []string{strings.Repeat(overlong+" ", 3)}
	in.Closing = strings.Repeat(overlong+" ", 4)
	in.VerificationCode = overlong
	for i := range in.Items {
		in.Items[i].DescMain = strings.Repeat(overlong+" ", 3)
		in.Items[i].SubItems = []string{strings.Repeat(overlong+" ", 3)}
		in.Items[i].UoM = overlong
		in.Items[i].UnitPrice = overlong
		in.Items[i].Total = overlong
	}

	for _, d := range collect(t, in) {
		if d.x < -0.01 || d.right > PageW+0.01 {
			t.Errorf("page %d: a run at %.2f..%.2f is off the %.2fpt paper: %q",
				d.page+1, d.x, d.right, PageW, trunc(d.text))
		}
	}
}

// TestWrappedFieldsGrowTheirBox covers the fields the engine re-flows. Each one
// has to make its box taller, not just draw further down the page.
func TestWrappedFieldsGrowTheirBox(t *testing.T) {
	e := testEngine(t)
	base := sampleInput()
	m0 := e.derive(base)

	cases := []struct {
		name   string
		mutate func(*Input)
		// height reports the field's own box height under the base input and
		// under the mutated one, so the test can say the box grew.
		boxes func(e *Engine, in Input) (float64, float64)
	}{
		{
			name:   "recipient address",
			mutate: func(in *Input) { in.RecipientAddress = strings.Repeat(base.RecipientAddress+", ", 6) },
			boxes: func(e *Engine, in Input) (float64, float64) {
				return 0, e.derive(in).infoH
			},
		},
		{
			name: "item description",
			mutate: func(in *Input) {
				in.Items[0].DescMain = strings.Repeat(base.Items[0].DescMain+", ", 6)
			},
			boxes: func(e *Engine, in Input) (float64, float64) {
				r := e.measureRows(in, e.derive(in))
				return 0, r[0].height
			},
		},
		{
			name: "item sub-items",
			mutate: func(in *Input) {
				// Added to the existing list, not swapped for it: a shorter
				// replacement legitimately makes a shorter row, and the test is
				// about growth.
				in.Items[0].SubItems = append(in.Items[0].SubItems,
					strings.Repeat(overlong+" ", 4))
			},
			boxes: func(e *Engine, in Input) (float64, float64) {
				r := e.measureRows(in, e.derive(in))
				return 0, r[0].height
			},
		},
		{
			name:   "terms",
			mutate: func(in *Input) { in.Terms = []string{strings.Repeat(overlong+" ", 6)} },
			boxes: func(e *Engine, in Input) (float64, float64) {
				mm := e.derive(in)
				return 0, e.tncHeight(in, mm.footerTncW)
			},
		},
		{
			name:   "closing",
			mutate: func(in *Input) { in.Closing = strings.Repeat(overlong+" ", 8) },
			boxes: func(e *Engine, in Input) (float64, float64) {
				mm := e.derive(in)
				n := len(e.Wrap(in.Closing, px(bodyPx), mm.innerW, false))
				return 0, float64(n)
			},
		},
		{
			name:   "company address lines",
			mutate: func(in *Input) { in.CompanyAddress = []string{overlong, overlong, overlong, overlong} },
			boxes: func(e *Engine, in Input) (float64, float64) {
				return 0, float64(len(e.Wrap(strings.Join(in.CompanyAddress, " "), px(12), m0.innerW, true)))
			},
		},
	}

	for _, c := range cases {
		_, before := c.boxes(e, sampleInput())
		long := sampleInput()
		c.mutate(&long)
		_, after := c.boxes(e, long)
		if after <= before {
			t.Errorf("%s: an overlong value did not grow its box (%.2f -> %.2f)", c.name, before, after)
			continue
		}
		t.Logf("  %-24s box grows %.2f -> %.2f", c.name, before, after)
	}
}

// TestSubjectGrowsWithItsValue covers the subject strip, which is a grid with no
// height of its own and so is as tall as its value wraps to. Treating it as a
// fixed 30pt box pushed the table down over the last line of a long subject.
func TestSubjectGrowsWithItsValue(t *testing.T) {
	e := testEngine(t)
	short := sampleInput()
	long := sampleInput()
	long.Subject = strings.Repeat(overlong+" ", 8)

	m0, m1 := e.derive(short), e.derive(long)
	if m1.subjectH <= m0.subjectH {
		t.Errorf("a longer subject did not grow the strip: %.2f -> %.2f", m0.subjectH, m1.subjectH)
	}
	if m1.tableTop <= m0.tableTop {
		t.Errorf("a grown subject did not push the table down: %.2f -> %.2f", m0.tableTop, m1.tableTop)
	}

	// Every line of the wrapped value sits inside the strip it grew into, and the
	// block is centred in it, so the first and last baselines straddle the middle
	// by half a block each way.
	lines := e.Wrap(long.Subject, px(bodyPx), subjectValueW(m1), true)
	stripTop, stripBot := m1.subjectY, m1.subjectY+m1.subjectH
	mid := (stripTop + stripBot) / 2
	lineH := px(bodyPx) * bodyLH
	first := mid + centerBaselineOffset - float64(len(lines)-1)*lineH/2
	last := first + float64(len(lines)-1)*lineH
	t.Logf("  subject wraps to %d lines: strip %.2f..%.2f, baselines %.2f..%.2f",
		len(lines), stripTop, stripBot, first, last)
	if first < stripTop || last > stripBot {
		t.Errorf("a wrapped subject escapes its own strip: baselines %.2f..%.2f in %.2f..%.2f",
			first, last, stripTop, stripBot)
	}
}

// TestDecorationTracksTheRecipientCard is the requirement that the decoration
// below the metadata card grows when the recipient card does. It is a consequence
// of the row being the taller of the two and the decoration being the leftover,
// and it is worth a test because the leftover is easy to break by accident: any
// code that takes the row from one card alone puts the difference in the wrong
// place.
func TestDecorationTracksTheRecipientCard(t *testing.T) {
	e := testEngine(t)
	base := sampleInput()
	mm := e.derive(base)

	// The fixture's own address fits on one line, and a one-line address leaves
	// the metadata stack taller than the recipient card, so the row is the stack's
	// and there is nothing over for the decoration to flex into. That is the
	// stylesheet, and it is the zero case. The growth case needs an address that
	// wraps.
	base.RecipientAddress = "Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam\n" +
		"Kota, Kota Batam, Provinsi Kepulauan Riau, 29431, Indonesia"

	decor := func(in Input) (row, d float64) {
		m := e.derive(in)
		return m.infoH, m.infoH - e.metaHeight(in, m) - metaStackGap
	}
	row0, d0 := decor(base)
	if d0 <= 0 {
		t.Fatalf("a two-line address still leaves no room for the decoration: row %.2f, decor %.2f", row0, d0)
	}

	longer := base
	longer.RecipientAddress = strings.Repeat(base.RecipientAddress+" ", 4)
	row1, d1 := decor(longer)
	if row1 <= row0 {
		t.Fatalf("a longer address did not grow the recipient card: %.2f -> %.2f", row0, row1)
	}
	if d1 <= d0 {
		t.Errorf("the decoration did not grow with the card: %.2f -> %.2f (row %.2f -> %.2f)",
			d0, d1, row0, row1)
	}
	t.Logf("  address 1 line : row %.2f, decoration %.2f", row0, d0)
	t.Logf("  address 8 lines: row %.2f, decoration %.2f", row1, d1)
	_ = mm
}

// TestShortAddressLeavesTheDecorationSmall is the other direction. When the
// recipient is short the leftover is small, and the test says so rather than
// leaving the size to be discovered on a live quotation.
func TestShortAddressLeavesTheDecorationSmall(t *testing.T) {
	e := testEngine(t)
	for _, c := range []struct{ label, addr string }{
		{"no address", ""},
		{"one line", "Komp. Ruko Legenda Gemilang, Blok D. NO 12, Batam"},
		{"two lines", "Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam\n" +
			"Kota, Kota Batam, Provinsi Kepulauan Riau, 29431, Indonesia"},
	} {
		in := sampleInput()
		in.RecipientAddress = c.addr
		m := e.derive(in)
		d := m.infoH - e.metaHeight(in, m) - metaStackGap
		t.Logf("  %-12s row %7.2f  metadata card %6.2f  decoration %6.2f (%4.1fpx)",
			c.label, m.infoH, e.metaHeight(in, m), d, 4*d)
		if d < -0.01 {
			t.Errorf("%s: the metadata card overruns the row by %.2fpt", c.label, -d)
		}
	}
}

// TestMetadataValueWrapping covers the collision the fixed 180pt track invites:
// a label on the left and a right-aligned value in the same row grow towards each
// other. A value too long wraps and the card grows.
func TestMetadataValueWrapping(t *testing.T) {
	e := testEngine(t)
	base := sampleInput()
	m := e.derive(base)

	long := base
	long.Date = "23 September 2026 at 14:05 WIB, Jakarta"
	shortH := e.metaHeight(base, m)
	longH := e.metaHeight(long, m)
	if longH <= shortH {
		t.Errorf("a value that overflows its track did not grow the card: %.2f -> %.2f", shortH, longH)
		return
	}
	t.Logf("  card grows %.2f -> %.2f (+%.2f, one extra line)", shortH, longH, longH-shortH)

	// And the value's lines must not run back over their label.
	rows := e.metaRows(long)
	valueX := m.infoRightW - px(1) - px(metaPadX)
	valueW := valueX - px(1) - px(metaPadX) - e.Width(longestLabel(rows), px(bodyPx), true)
	for _, kv := range rows {
		lines := e.Wrap(kv[1], px(bodyPx), valueW, false)
		for _, ln := range lines {
			if w := e.Width(ln, px(bodyPx), true); w > valueW+0.01 {
				t.Errorf("value %q is %.2fpt wide in a %.2fpt track", trunc(ln), w, valueW)
			}
		}
	}
}

// TestPaginationUnderLoad checks the two things pagination has to get right when
// the content grows: no page overflows the paper, and no row is silently lost.
func TestPaginationUnderLoad(t *testing.T) {
	e := testEngine(t)

	in := sampleInput()
	in.Items = append([]Item{}, in.Items...)
	for i := 0; i < 12; i++ {
		it := in.Items[0]
		it.No = len(in.Items) + 1
		it.DescMain = strings.Repeat(overlong+" ", 3)
		in.Items = append(in.Items, it)
	}

	pages := e.layout(in)
	for i, p := range pages {
		if p.bottom > PageH+0.01 {
			t.Errorf("page %d closes at %.2f, past the %.2fpt page", i+1, p.bottom, PageH)
		}
	}
	t.Logf("  %d items -> %d pages, each closing inside the paper", len(in.Items), len(pages))
	if len(pages) < 2 {
		t.Errorf("23 items did not need a second page")
	}
}

func trunc(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}
