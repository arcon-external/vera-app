package backend

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

// prepareMixedStates and the tests below execute the real customer template
// over one mixed document: the numbering, the rollup arithmetic and the
// customer-visible split all come from the same path the export uses.
func prepareMixedStates(t *testing.T) *Quotation {
	t.Helper()
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{
			{ // state B: subs priced internally, main carries the values
				Description: "Supply & install exhaust fan system at CDA block 2:",
				UoM:         "Lot",
				UnitPrice:   31_625_000,
				Qty:         1,
				SubItems: []QuotationSubItemInput{
					{Description: "4 units axial exhaust fan 18-in 1.5HP", UoM: "Unit", UnitCost: 2_500_000, Qty: 4, MarginEnabled: true, MarginPercent: 25,
						SubSubItems: []QuotationSubSubItemInput{
							{Description: "Concrete pad and anti-vibration mounting rails"},
							{Description: "Roof penetration and flashing kit"},
						}},
					{Description: "Galvanized ducting complete with damper and grille", UoM: "LS", UnitCost: 11_062_500, Qty: 1, MarginEnabled: true, MarginPercent: 50},
					{Description: "Electrical wiring and control panel", UoM: "LS", UnitCost: 10_562_500, Qty: 1},
				},
			},
			{ // state A: plain full-value line
				Description: "Monthly preventive maintenance for HVAC system at main office building",
				UoM:         "Month",
				UnitPrice:   9_180_000,
				Qty:         12,
			},
			{ // state C: parent is a title, every sub priced; one part carries
				// detail lines numbered X.Y.Z
				Description:   "Supply and install aircond automation system at Mesjid Batam 1:",
				UoM:           "Lot",
				UnitPrice:     38_136_000,
				Qty:           1,
				ShowSubPrices: true,
				SubItems: []QuotationSubItemInput{
					{Description: "Metal enclosure panel with timers", UoM: "LS", UnitCost: 12_450_000, Qty: 1, MarginEnabled: true, MarginPercent: 15},
					{Description: "Zelio Schneider programmable controller", UoM: "EA", UnitCost: 9_500_000, Qty: 1, MarginEnabled: true, MarginPercent: 25,
						SubSubItems: []QuotationSubSubItemInput{
							{Description: "24V control wiring from the AHU panel"},
						}},
					{Description: "Wiring and commissioning", UoM: "LS", UnitCost: 16_186_000, Qty: 1},
				},
			},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTime:      45,
		Incoterm:      "FOB",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return q
}

// TestPackageRollupPricesPerSub pins the arithmetic the whole feature rests
// on: margins apply per sub-item before summing, the parent is the rollup, and
// the per-unit margin amount keeps its per-unit meaning on the package.
func TestPackageRollupPricesPerSub(t *testing.T) {
	q := prepareMixedStates(t)

	exhaust := q.Data.Items[0]
	// Cost: (2,500,000 x 4) + 11,062,500 + 10,562,500 = 31,625,000.
	// Sell: 2,500,000 x 1.25 x 4 = 12,500,000; 11,062,500 x 1.5 = 16,593,750
	// (truncated from 16,593,750.0? 11,062,500 * 150 / 100 = 16,593,750 exactly);
	// wiring has no margin: 10,562,500. Package sell = 39,656,250.
	if exhaust.OriginalUnitPrice != 31_625_000 {
		t.Errorf("package cost = %d, want 31625000", exhaust.OriginalUnitPrice)
	}
	if exhaust.UnitPrice != 39_656_250 {
		t.Errorf("package sell = %d, want 39656250", exhaust.UnitPrice)
	}
	if exhaust.Total != 39_656_250 {
		t.Errorf("package total = %d, want 39656250 (Qty is 1: the parent is the rollup)", exhaust.Total)
	}
	if exhaust.Qty != 1 {
		t.Errorf("package Qty = %d, want 1", exhaust.Qty)
	}
	if exhaust.UoM != "Lot" {
		t.Errorf("package UoM = %q, want Lot", exhaust.UoM)
	}
	// Per unit, per the line spec: the package's margin amount is per package.
	if exhaust.MarginAmount != 8_031_250 {
		t.Errorf("package MarginAmount = %d, want 8031250", exhaust.MarginAmount)
	}

	// Sub rows carry their own numbers, priced per unit with the same
	// truncation a stand-alone line would see.
	subs := exhaust.SubItems
	if len(subs) != 3 {
		t.Fatalf("sub count = %d, want 3", len(subs))
	}
	if subs[0].SubNo != "1.1" || subs[1].SubNo != "1.2" || subs[2].SubNo != "1.3" {
		t.Errorf("sub numbering = %q, %q, %q; want 1.1, 1.2, 1.3", subs[0].SubNo, subs[1].SubNo, subs[2].SubNo)
	}
	if subs[1].UnitPrice != 16_593_750 {
		t.Errorf("sub 1.2 sell unit = %d, want 16593750 (exactly 150%% of cost, no truncation needed)", subs[1].UnitPrice)
	}
	if subs[1].Total != 16_593_750 {
		t.Errorf("sub 1.2 total = %d, want 16593750", subs[1].Total)
	}
	// An unpriced sub sells for its cost.
	if subs[2].UnitPrice != 10_562_500 {
		t.Errorf("sub 1.3 sell unit = %d, want its cost 10562500", subs[2].UnitPrice)
	}

	// Detail lines are plain bullets and change no money at all: they
	// describe what a part includes, so the parent rollup above must be
	// exactly what it was before they existed.
	lows := subs[0].SubSubItems
	if len(lows) != 2 {
		t.Fatalf("sub 1.1 detail count = %d, want 2", len(lows))
	}
	if lows[0].Description != "Concrete pad and anti-vibration mounting rails" {
		t.Errorf("detail text drifted: %q", lows[0].Description)
	}
	// The C side carries one detail under 3.2, carried through unlabelled.
	aircond := q.Data.Items[2]
	if got := len(aircond.SubItems[1].SubSubItems); got != 1 {
		t.Errorf("state C detail count = %d, want 1", got)
	}

	// Every package's sub total sums to the parent: the invariant that makes
	// state B and state C renderings of the same line print identical money.
	var sum uint64
	for _, sub := range subs {
		sum += sub.Total
	}
	if sum != exhaust.Total {
		t.Errorf("sub totals sum to %d, parent total is %d", sum, exhaust.Total)
	}
}

// TestPackageTruncatesPerSubNotAtTheSum proves margins truncate per sub-unit,
// the same place a stand-alone line truncates -- never at the sum, which would
// let one part's remainder pay for another's.
func TestPackageTruncatesPerSubNotAtTheSum(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())

	if err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{{
			Description: "Package:",
			UoM:         "Lot",
			Qty:         1,
			SubItems: []QuotationSubItemInput{
				{Description: "A part with a third-based margin", UoM: "EA", UnitCost: 333, Qty: 1, MarginEnabled: true, MarginPercent: 50},
				{Description: "Another part", UoM: "EA", UnitCost: 333, Qty: 1, MarginEnabled: true, MarginPercent: 50},
			},
		}},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTime:      45,
		Incoterm:      "FOB",
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	item := q.Data.Items[0]
	// applyMarkup(333, 50) = 499 (499.5 truncated) per part; selling at the
	// summed 998, rather than 333*2 + 499.5*2 = 1000 rounded at the end.
	if item.UnitPrice != 998 {
		t.Errorf("package sell = %d, want 998 (499 per part: truncation is per sub, never at the sum)", item.UnitPrice)
	}
}

// TestPrepareSubItemValidation covers the boundary: what Prepare refuses, and
// why, in the order the rules exist.
func TestPrepareSubItemValidation(t *testing.T) {
	base := func(mod func(*QuotationItemInput)) QuotationInput {
		item := QuotationItemInput{
			Description: "Package:",
			UoM:         "Lot",
			Qty:         1,
			SubItems: []QuotationSubItemInput{
				{Description: "A part", UoM: "EA", UnitCost: 1_000, Qty: 1, MarginEnabled: true, MarginPercent: 100},
			},
		}
		mod(&item)
		return QuotationInput{
			CustomerID:    2,
			Items:         []QuotationItemInput{item},
			Subject:       "Test",
			RecipientName: "Test",
			Validity:      14,
			PaymentTerm:   30,
			LeadTime:      45,
			Incoterm:      "FOB",
		}
	}

	expectErr := func(name, wantContains string, mod func(*QuotationItemInput)) {
		t.Run(name, func(t *testing.T) {
			q := newTestQuotation()
			q.SetAuthService(auth.StaffTestService())
			err := q.Prepare(base(mod))
			if err == nil {
				t.Fatalf("was accepted; want refusal containing %q", wantContains)
			}
			if !strings.Contains(err.Error(), wantContains) {
				t.Errorf("error %q does not mention %q", err, wantContains)
			}
		})
	}

	expectErr("parent qty is not 1", "quantity 1", func(i *QuotationItemInput) { i.Qty = 2 })
	expectErr("parent uom is not Lot", "unit Lot", func(i *QuotationItemInput) { i.UoM = "EA" })
	expectErr("parent margin enabled", "clear the line margin", func(i *QuotationItemInput) { i.MarginEnabled = true })
	expectErr("bullets duplicate the subs", "duplicate the sub-items", func(i *QuotationItemInput) { i.Description = "Package:\n- scope bullet" })
	expectErr("breakdown flag without subs", "breakdown requested but no sub-items", func(i *QuotationItemInput) {
		i.ShowSubPrices = true
		i.SubItems = nil
	})
	expectErr("empty sub description", "description is required", func(i *QuotationItemInput) { i.SubItems[0].Description = "   " })
	expectErr("multi-line sub description", "single line", func(i *QuotationItemInput) { i.SubItems[0].Description = "two\nlines" })
	expectErr("sub qty zero", "quantity must be greater than zero", func(i *QuotationItemInput) { i.SubItems[0].Qty = 0 })
	expectErr("sub uom implausible (the paste bug's shape)", "is not a unit", func(i *QuotationItemInput) { i.SubItems[0].UoM = "Electric panel, delivered" })
	expectErr("sub margin over the ceiling", "greater than 1000", func(i *QuotationItemInput) { i.SubItems[0].MarginPercent = 1001 })
	expectErr("detail description empty", "detail 1: description is required", func(i *QuotationItemInput) {
		i.SubItems[0].SubSubItems = []QuotationSubSubItemInput{{Description: "   "}}
	})
	expectErr("detail description multi-line", "detail 1: description must be a single line", func(i *QuotationItemInput) {
		i.SubItems[0].SubSubItems = []QuotationSubSubItemInput{{Description: "two\nlines"}}
	})

	t.Run("lot accepted case-insensitively and canonicalized", func(t *testing.T) {
		q := newTestQuotation()
		q.SetAuthService(auth.StaffTestService())
		err := q.Prepare(base(func(i *QuotationItemInput) { i.UoM = "lot" }))
		if err != nil {
			t.Fatalf("Prepare with UoM \"lot\": %v", err)
		}
		if got := q.Data.Items[0].UoM; got != "Lot" {
			t.Errorf("parent UoM printed as %q, want the canonical Lot", got)
		}
	})
}
