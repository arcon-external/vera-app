package gpdf

// Guard for the metadata stack's border-box floor: `.meta-stack` is a flex
// column whose last child (`.meta-decor`) flexes, and that child carries
// `border: 1px solid`, so under `box-sizing: border-box` its border-box floor
// is 2px even at zero height -- the stack is never shorter than the card plus
// the gap plus those 2px. infoHeight counts all three, mirroring the totals
// row (see totalsDecorFloor): omitting the floor puts everything below the
// block 2px high while the browser paints the 2px sliver under the card.
//
// Measured in Chrome at the print content width: the browser's metadata row
// is 131.75px (card 117.75 + 12 gap + 2 floor) against a native infoH that
// was 130.00px. After the fix the native row is 132.00px; the remaining
// quarter pixel is the metadata card's own rounding (four 16px line boxes
// against the browser's four 15.9375px ones), which belongs to the card's
// contents and not to this stack.

import (
	"math"
	"testing"
)

// TestMetaDecorFloorIsTwoPixels pins the constant itself: the same 2px the
// totals decoration carries, because the two stacks are the same shape --
// same gap, same border, same radius (see
// TestTotalsFillerIsTheMetadataFillersTwin for the shape contract).
func TestMetaDecorFloorIsTwoPixels(t *testing.T) {
	if metaDecorFloor != 2*ptPerPx {
		t.Errorf("metadata decoration floor is %.3fpt, a 1px top and bottom border is %.3fpt",
			float64(metaDecorFloor), 2*ptPerPx)
	}
	if metaDecorFloor != totalsDecorFloor {
		t.Errorf("metadata floor %.3fpt != totals floor %.3fpt: the two stacks are the same shape",
			float64(metaDecorFloor), float64(totalsDecorFloor))
	}
}

// stackDecorBoxes runs drawInfoRow alone on a fresh page and returns the
// metadata card box and the decoration box below it.
func stackDecorBoxes(e *Engine, in Input, m metrics) (card, decor *box, top float64) {
	p := &page{}
	e.drawInfoRow(p, in, m)
	top = m.contentTopY
	rightX := m.innerX + m.infoLeftW + px(metaGapX)
	for _, pr := range p.prims {
		b, ok := pr.(box)
		if !ok || b.x != rightX {
			continue
		}
		// b is a fresh copy each iteration (declared in the loop body), so
		// keeping its address is safe.
		switch {
		case b.gradient != nil:
			decor = &b
		case b.fill == e.pal.Surface:
			card = &b
		}
	}
	return card, decor, top
}

// TestInfoRowCountsTheFloorWhenTheStackDecides pins the stack-deciding state:
// the fixture's one-line recipient address leaves the recipient card shorter
// than the metadata stack, so the row is the stack's -- card plus gap plus
// floor -- and the decoration draws as exactly the floor, closing the row.
func TestInfoRowCountsTheFloorWhenTheStackDecides(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	m := e.derive(in)

	metaH := e.metaHeight(in, m)
	if metaH != 88.5 {
		t.Errorf("metadata card is %.3fpt, its measured height is 88.500pt", metaH)
	}
	// The row is the whole stack: card plus gap plus the decoration's
	// border-box floor. Dropping the floor from infoHeight puts it back at
	// 97.500pt (130px) against the browser's 131.75px.
	stack := metaH + metaStackGap + metaDecorFloor
	if m.infoH != stack {
		t.Errorf("info row is %.3fpt, the stack is %.3fpt: the row must follow the stack here",
			m.infoH, stack)
	}
	// Exact equality is the precondition: had the recipient card dominated,
	// the row would exceed the stack and this would fail rather than pass
	// vacuously.
	d := m.infoH - metaH - metaStackGap
	if d != metaDecorFloor {
		t.Errorf("leftover is %.3fpt, the decoration floor is %.3fpt",
			d, float64(metaDecorFloor))
	}

	card, decor, top := stackDecorBoxes(e, in, m)
	if card == nil {
		t.Fatal("no metadata card box at the metadata column")
	}
	if card.h != metaH {
		t.Errorf("drawn card is %.3fpt, derived card height is %.3fpt",
			card.h, metaH)
	}
	if decor == nil {
		t.Fatal("no floor decoration drawn below the metadata card")
	}
	if decor.y != top+metaH+metaStackGap {
		t.Errorf("decoration starts at %.3f, card plus gap ends at %.3f",
			decor.y, top+metaH+metaStackGap)
	}
	if bottom := decor.y + decor.h; math.Abs(bottom-(top+m.infoH)) > 1e-9 {
		t.Errorf("decoration ends at %.3f, info row ends at %.3f: the column does not close",
			bottom, top+m.infoH)
	}
	if decor.h != metaDecorFloor {
		t.Errorf("decoration is %.3fpt, the floor is %.3fpt: not the sliver the browser paints",
			decor.h, float64(metaDecorFloor))
	}
	t.Logf("  card %.3fpt, row %.3fpt, decoration %.3fpt (floor): sliver paints, as drawn",
		metaH, m.infoH, d)
}

// TestInfoRowIgnoresTheFloorWhenTheRecipientDominates pins the other deciding
// state: a two-line recipient address makes the recipient card taller than
// the stack even with the floor counted, so the row is the card's and the
// floor changes nothing -- 105.000pt before and after.
func TestInfoRowIgnoresTheFloorWhenTheRecipientDominates(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	in.RecipientAddress = "Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam\n" +
		"Kota, Kota Batam, Provinsi Kepulauan Riau, 29431, Indonesia"
	m := e.derive(in)

	metaH := e.metaHeight(in, m)
	stack := metaH + metaStackGap + metaDecorFloor
	if m.infoH <= stack {
		t.Fatalf("recipient row %.3fpt does not exceed the floored stack %.3fpt: not the state where the card dominates",
			m.infoH, stack)
	}
	if m.infoH != 105.0 {
		t.Errorf("info row is %.3fpt, the recipient card is 105.000pt: the floor must not move this state",
			m.infoH)
	}
	d := m.infoH - metaH - metaStackGap
	if d <= metaDecorFloor {
		t.Errorf("leftover is %.3fpt, the floor is %.3fpt: nothing distinguishes this state from the stack's",
			d, float64(metaDecorFloor))
	}

	_, decor, top := stackDecorBoxes(e, in, m)
	if decor == nil {
		t.Fatal("no decoration drawn below the metadata card")
	}
	if bottom := decor.y + decor.h; math.Abs(bottom-(top+m.infoH)) > 1e-9 {
		t.Errorf("decoration ends at %.3f, info row ends at %.3f: the column does not close",
			bottom, top+m.infoH)
	}
	t.Logf("  card %.3fpt, row %.3fpt, decoration %.3fpt: card decides, floor changes nothing",
		metaH, m.infoH, d)
}
