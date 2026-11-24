package gpdf

import "testing"

// The customer template bands rows per item: tbody.item:nth-of-type(even) gets
// the raised surface, so every part of a package shares its parent's band.
// Numbering is stable identity now, which means a document can carry gaps where
// a line was removed (#648). The band must follow the item's position in the
// document, exactly like nth-of-type; banding by number parity would paint two
// adjacent items the same and lose the alternation the browser keeps.
func TestItemBandingFollowsPositionNotNumber(t *testing.T) {
	e := testEngine(t)
	in := Input{Items: []Item{
		{No: 2, DescMain: "Line two", Qty: "1", UoM: "Lot", UnitPrice: "1.000", Total: "1.000"},
		{No: 4, DescMain: "Line four", Qty: "1", UoM: "Lot", UnitPrice: "2.000", Total: "2.000"},
	}}

	rows := e.measureRows(in, e.derive(in))
	if len(rows) != 2 {
		t.Fatalf("measured %d rows; want 2", len(rows))
	}
	if rows[0].stripe {
		t.Errorf("item at position 1 (No 2) is banded; the template leaves the first item unbanded")
	}
	if !rows[1].stripe {
		t.Errorf("item at position 2 (No 4) is not banded; the template bands the second item. " +
			"Two adjacent unbanded rows are what number-parity banding produces once a number is missing")
	}
}
