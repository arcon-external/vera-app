package backend

import (
	"testing"

	"vera-app/backend/auth"
)

// TestRowDiscountTotalMatchesTheReportShape is the headline guard, built the way
// the reviewed document was: a discounted plain row (500.000) sitting beside a
// priced package whose second part carries its own 120.000 discount. The genuine
// per-row total is 620.000, and before this total existed the internal table
// showed the operator a zero for exactly this shape.
//
// The figures come from real QuotationInput values through buildItems and
// buildPricing -- nothing is asserted against a literal the arithmetic did not
// produce -- and the total is checked against the two rows the report names.
func TestRowDiscountTotalMatchesTheReportShape(t *testing.T) {
	items := buildItems([]QuotationItemInput{
		// Item 1: a plain margined line with no row discount. It keeps the shape
		// a real quotation, and it must contribute nothing to the total.
		{Description: "Main switchboard", Qty: 1, UoM: "Lot", UnitPrice: 3000000,
			MarginEnabled: true, MarginPercent: 20},
		// Item 2: a discounted plain row. No margin, so the list price is the
		// cost (1.000.000); 50% off sells 500.000 and leaves a 500.000 discount
		// on the row itself (buildItems takes its own DiscountAmount no matter
		// which of margin/discount is on).
		{Description: "Cable tray run", Qty: 1, UoM: "LS", UnitPrice: 1000000,
			DiscountEnabled: true, DiscountPercent: 50},
		// Item 3: a priced package (state C). Part 3.1 is undiscounted; part 3.2
		// is 10% off a 1.200.000 list, a 120.000 discount that lives on the part.
		{Description: "Trauma system", ShowSubPrices: true, Qty: 1, UoM: "Lot",
			SubItems: []QuotationSubItemInput{
				{Description: "Panel beds", Qty: 1, UoM: "Pc", UnitCost: 2000000,
					MarginEnabled: true, MarginPercent: 20},
				{Description: "Monitor arms", Qty: 1, UoM: "LS", UnitCost: 1200000,
					DiscountEnabled: true, DiscountPercent: 10},
			}},
	}, "IDR")

	if len(items) != 3 {
		t.Fatalf("buildItems returned %d items; want 3", len(items))
	}

	// The two rows the report names, pinned so the total below is checked
	// against what the arithmetic actually produced rather than a hand-typed
	// 620.000.
	rowDiscount := items[1].DiscountAmount              // item 2, its own discount
	partDiscount := items[2].SubItems[1].DiscountAmount // part 3.2, its own discount
	if rowDiscount != 500000 {
		t.Fatalf("item 2 discount = %d; want 500000 (1.000.000 at 50%% off)", rowDiscount)
	}
	if partDiscount != 120000 {
		t.Fatalf("part 3.2 discount = %d; want 120000 (1.200.000 at 10%% off)", partDiscount)
	}

	// The package parent carries the rollup of its parts; the discounted part
	// is the whole of it here, so parent rollup equals the part's own discount.
	if parent := items[2].DiscountAmount; parent != partDiscount {
		t.Fatalf("package parent rollup = %d; want %d (the single discounted part)",
			parent, partDiscount)
	}

	// A whole-quotation discount of 0 keeps this the report's exact shape: the
	// only discount on the document is the per-row one.
	pricing := buildPricing(items, 0, 0, 0)

	// The total is the sum of the contributing leaves: 500.000 + 120.000.
	wantTotal := rowDiscount + partDiscount
	if wantTotal != 620000 {
		t.Fatalf("the report's arithmetic moved: leaves sum to %d; want 620000", wantTotal)
	}
	if pricing.RowDiscountTotal != wantTotal {
		t.Errorf("RowDiscountTotal = %d; want %d (the sum of every row's own discount)",
			pricing.RowDiscountTotal, wantTotal)
	}

	// The whole-quotation discount is a separate figure: with no whole-quotation
	// percent it is zero, which is exactly the value the operator used to see on
	// the "Row discounts" line while the real 620.000 went unshown.
	if pricing.MarginalizedDiscount != 0 {
		t.Errorf("MarginalizedDiscount = %d; want 0 (no whole-quotation discount here)",
			pricing.MarginalizedDiscount)
	}
}

// TestRowDiscountTotalIsLeafAwareAcrossShapes pins the rule the total follows
// for every row shape, and proves no shape is double-counted.
//
// A plain row contributes its own discount. A package -- priced (state C) or
// shown as scope (state B) -- contributes its parts' discounts once; its parent
// DiscountAmount is already the rollup of those same parts, so counting the
// parent too would be counting the package twice.
func TestRowDiscountTotalIsLeafAwareAcrossShapes(t *testing.T) {
	// twoPartPackage is a package with two discounted parts: 100.000 at 10% off
	// (10.000) plus 200.000 at 5% off (10.000), a 20.000 rollup. showSubPrices
	// selects state C (true) or state B (false); both price and roll up the
	// same way in buildItems, so the row-discount total must treat them alike.
	twoPartPackage := func(showSubPrices bool) QuotationItemInput {
		return QuotationItemInput{
			Description: "Package", ShowSubPrices: showSubPrices, Qty: 1, UoM: "Lot",
			SubItems: []QuotationSubItemInput{
				{Description: "part a", Qty: 1, UoM: "Pc", UnitCost: 100000,
					DiscountEnabled: true, DiscountPercent: 10},
				{Description: "part b", Qty: 1, UoM: "Pc", UnitCost: 200000,
					DiscountEnabled: true, DiscountPercent: 5},
			},
		}
	}

	cases := []struct {
		name  string
		items []QuotationItemInput
		want  uint64
	}{
		{
			// 800.000 at 25% off sells 600.000; the 200.000 is on the row.
			name:  "a plain row carries its own discount",
			items: []QuotationItemInput{{Description: "row", Qty: 1, UoM: "LS", UnitPrice: 800000, DiscountEnabled: true, DiscountPercent: 25}},
			want:  200000,
		},
		{
			name:  "a priced package counts its parts once",
			items: []QuotationItemInput{twoPartPackage(true)},
			want:  20000, // 10.000 + 10.000, not the 40.000 a double count would give
		},
		{
			name:  "a scope package counts its parts once",
			items: []QuotationItemInput{twoPartPackage(false)},
			want:  20000,
		},
		{
			// The parent rollup (20.000) plus a plain row (200.000): the package
			// is counted once through its parts, the row through itself.
			name: "a package and a plain row add their separate discounts",
			items: []QuotationItemInput{
				twoPartPackage(true),
				{Description: "row", Qty: 1, UoM: "LS", UnitPrice: 800000, DiscountEnabled: true, DiscountPercent: 25},
			},
			want: 220000,
		},
		{
			name: "undiscounted rows add nothing",
			items: []QuotationItemInput{
				{Description: "plain", Qty: 1, UoM: "Lot", UnitPrice: 1000000, MarginEnabled: true, MarginPercent: 10},
				{Description: "package", ShowSubPrices: true, Qty: 1, UoM: "Lot", SubItems: []QuotationSubItemInput{
					{Description: "part", Qty: 1, UoM: "Pc", UnitCost: 500000, MarginEnabled: true, MarginPercent: 10},
				}},
			},
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pricing := buildPricing(buildItems(tc.items, "IDR"), 0, 0, 0)
			if pricing.RowDiscountTotal != tc.want {
				t.Errorf("RowDiscountTotal = %d; want %d", pricing.RowDiscountTotal, tc.want)
			}
		})
	}
}

// TestRowDiscountTotalDoesNotDoubleCountPackageRollups names the exact trap the
// design warns about. The package parent's DiscountAmount is the rollup of its
// parts' discounts, so the total must equal that sum once -- never the parent
// rollup plus the parts a second time.
func TestRowDiscountTotalDoesNotDoubleCountPackageRollups(t *testing.T) {
	items := buildItems([]QuotationItemInput{{
		Description: "Package", ShowSubPrices: true, Qty: 1, UoM: "Lot",
		SubItems: []QuotationSubItemInput{
			{Description: "part a", Qty: 1, UoM: "Pc", UnitCost: 1000000, DiscountEnabled: true, DiscountPercent: 10}, // 100.000
			{Description: "part b", Qty: 1, UoM: "Pc", UnitCost: 500000, DiscountEnabled: true, DiscountPercent: 20},  // 100.000
		},
	}}, "IDR")

	if len(items) != 1 || len(items[0].SubItems) != 2 {
		t.Fatalf("buildItems did not produce the expected package: %+v", items)
	}
	parentRollup := items[0].DiscountAmount
	partsSum := items[0].SubItems[0].DiscountAmount + items[0].SubItems[1].DiscountAmount

	if parentRollup != partsSum {
		t.Fatalf("the package parent carries %d but its parts sum to %d; the "+
			"rollup assumption behind the leaf-aware total no longer holds",
			parentRollup, partsSum)
	}

	pricing := buildPricing(items, 0, 0, 0)

	// The total is the parts once.
	if pricing.RowDiscountTotal != partsSum {
		t.Errorf("RowDiscountTotal = %d; want %d (the parts, counted once)",
			pricing.RowDiscountTotal, partsSum)
	}
	// And it is never the parent rollup added on top of the parts a second time.
	if pricing.RowDiscountTotal == parentRollup+partsSum {
		t.Errorf("RowDiscountTotal = %d, which is the parent rollup (%d) plus the "+
			"parts (%d): the package was counted twice",
			pricing.RowDiscountTotal, parentRollup, partsSum)
	}
}

// TestInternalBreakdownReportsRowDiscountTotal proves the field survives the
// trip the CLI actually reads: buildPricing -> q.Data -> InternalBreakdown, both
// as the raw total and as the display string the table prints.
func TestInternalBreakdownReportsRowDiscountTotal(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{
			{Description: "Discounted row", Qty: 1, UoM: "LS", UnitPrice: 1000000, DiscountEnabled: true, DiscountPercent: 50},
			{Description: "Priced package", ShowSubPrices: true, Qty: 1, UoM: "Lot", SubItems: []QuotationSubItemInput{
				{Description: "Discounted part", Qty: 1, UoM: "LS", UnitCost: 1200000, DiscountEnabled: true, DiscountPercent: 10},
			}},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		LeadTime:      30,
		Incoterm:      "EXW",
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if q.Data.RowDiscountTotal != 620000 {
		t.Errorf("q.Data.RowDiscountTotal = %d; want 620000", q.Data.RowDiscountTotal)
	}

	bd, err := q.GetInternalBreakdown()
	if err != nil {
		t.Fatalf("GetInternalBreakdown failed: %v", err)
	}
	if bd.RowDiscountTotal != 620000 {
		t.Errorf("InternalBreakdown.RowDiscountTotal = %d; want 620000", bd.RowDiscountTotal)
	}
	if bd.DisplayRowDiscountTotal != "620.000" {
		t.Errorf("InternalBreakdown.DisplayRowDiscountTotal = %q; want \"620.000\"",
			bd.DisplayRowDiscountTotal)
	}
}
