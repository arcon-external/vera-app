package backend

import (
	"testing"

	"vera-app/backend/auth"
)

func TestBuildItemsUsesPerItemMargins(t *testing.T) {
	items := buildItems([]QuotationItemInput{
		{
			Description:   "Margined item",
			Qty:           2,
			UoM:           "Lot",
			UnitPrice:     100000,
			MarginEnabled: true,
			MarginPercent: 20,
		},
		{
			Description:   "Unmargined item",
			Qty:           3,
			UoM:           "Unit",
			UnitPrice:     50000,
			MarginEnabled: false,
			MarginPercent: 50,
		},
	}, "IDR")

	if len(items) != 2 {
		t.Fatalf("buildItems returned %d items; want 2", len(items))
	}

	if items[0].OriginalUnitPrice != 100000 || items[0].OriginalTotal != 200000 {
		t.Errorf("margined original values = %d/%d; want 100000/200000", items[0].OriginalUnitPrice, items[0].OriginalTotal)
	}
	if items[0].UnitPrice != 120000 || items[0].Total != 240000 {
		t.Errorf("margined values = %d/%d; want 120000/240000", items[0].UnitPrice, items[0].Total)
	}
	if items[0].MarginAmount != 20000 {
		t.Errorf("margin amount = %d; want 20000", items[0].MarginAmount)
	}

	if items[1].UnitPrice != 50000 || items[1].Total != 150000 {
		t.Errorf("disabled margin values = %d/%d; want 50000/150000", items[1].UnitPrice, items[1].Total)
	}
	if items[1].MarginAmount != 0 {
		t.Errorf("disabled margin amount = %d; want 0", items[1].MarginAmount)
	}
}

func TestBuildPricingUsesOriginalAndMarginalizedGrandTotals(t *testing.T) {
	items := buildItems([]QuotationItemInput{
		{
			Description:   "Margined item",
			Qty:           2,
			UoM:           "Lot",
			UnitPrice:     100000,
			MarginEnabled: true,
			MarginPercent: 20,
		},
		{
			Description: "Unmargined item",
			Qty:         1,
			UoM:         "Unit",
			UnitPrice:   50000,
		},
	}, "IDR")

	pricing := buildPricing(items, 10, 0, 200)

	if pricing.OriginalSubtotal != 250000 {
		t.Errorf("original subtotal = %d; want 250000", pricing.OriginalSubtotal)
	}
	// The cost side carries no discount: a price cut to the customer does not
	// move what we pay suppliers, so original grand is the cost subtotal.
	if pricing.OriginalDiscount != 0 || pricing.OriginalGrand != 250000 {
		t.Errorf("original discount/grand = %d/%d; want 0/250000", pricing.OriginalDiscount, pricing.OriginalGrand)
	}
	if pricing.MarginalizedSubtotal != 290000 {
		t.Errorf("marginalized subtotal = %d; want 290000", pricing.MarginalizedSubtotal)
	}
	if pricing.MarginalizedDiscount != 29000 || pricing.MarginalizedGrand != 261000 {
		t.Errorf("marginalized discount/grand = %d/%d; want 29000/261000", pricing.MarginalizedDiscount, pricing.MarginalizedGrand)
	}
	if pricing.TaxAmount != 5220 || pricing.MarginalizedGrandAfterTax != 255780 {
		t.Errorf("tax/after-tax grand = %d/%d; want 5220/255780", pricing.TaxAmount, pricing.MarginalizedGrandAfterTax)
	}
	// Profit 261.000 - 250.000; nett takes the services tax off first.
	if pricing.ProfitAmount != 11000 {
		t.Errorf("profit amount = %d; want 11000", pricing.ProfitAmount)
	}
	if pricing.NettProfit != 5780 {
		t.Errorf("nett profit = %d; want 5780", pricing.NettProfit)
	}
}

func TestPrepareRejectsItemMarginOverCeiling(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	err := q.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{
				Description:   "Item",
				Qty:           1,
				UoM:           "Lot",
				UnitPrice:     1000,
				MarginEnabled: true,
				MarginPercent: MaxMarginPercent + 1,
			},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		LeadTime:      30,
		CustomerID:    2,
		Incoterm:      "EXW",
		TaxZone:       TaxZoneFTZ,
	})
	if err == nil {
		t.Fatalf("expected error for item margin > %d but got nil", MaxMarginPercent)
	}
}

// TestRowDiscountFollowsTheConfirmedFlow pins the three rules #274 landed:
// the discount comes off the marginalised price, the cost side never moves,
// and the line's margin is read after the discount so a discount below cost
// shows a loss instead of wrapping round as a huge uint64 gain.
func TestRowDiscountFollowsTheConfirmedFlow(t *testing.T) {
	cases := []struct {
		name           string
		cost           uint64
		qty            uint32
		margin         uint16
		discount       uint16
		wantListUnit   uint64
		wantUnit       uint64
		wantTotal      uint64
		wantDiscAmount uint64
		wantMargin     int64
	}{
		{
			// 100.000 cost, 20% margin -> 120.000 list, 10% off -> 108.000.
			// Profit per unit is +8.000, not the +20.000 the margin alone
			// would have suggested.
			name: "margin then discount, cost untouched", cost: 100000, qty: 2,
			margin: 20, discount: 10,
			wantListUnit: 120000, wantUnit: 108000, wantTotal: 216000,
			wantDiscAmount: 24000, wantMargin: 8000,
		},
		{
			// No margin: the list price IS the cost, so the discount is taken
			// straight off it and profit follows.
			name: "discount with no margin", cost: 100000, qty: 1,
			discount:     10,
			wantListUnit: 100000, wantUnit: 90000, wantTotal: 90000,
			wantDiscAmount: 10000, wantMargin: -10000,
		},
		{
			// A discount that exceeds the margin: the line is below cost.
			// That is a real answer and has to survive as a negative number.
			name: "discount below cost shows a loss", cost: 100000, qty: 1,
			margin: 10, discount: 30,
			wantListUnit: 110000, wantUnit: 77000, wantTotal: 77000,
			wantDiscAmount: 33000, wantMargin: -23000,
		},
		{
			// A 100% discount is a free line: legal, and the loss is the cost.
			name: "a hundred percent discount is a free line", cost: 50000, qty: 4,
			discount:     100,
			wantListUnit: 50000, wantUnit: 0, wantTotal: 0,
			wantDiscAmount: 200000, wantMargin: -50000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := buildItems([]QuotationItemInput{{
				Description:     "Line",
				Qty:             tc.qty,
				UoM:             "Lot",
				UnitPrice:       tc.cost,
				MarginEnabled:   tc.margin > 0,
				MarginPercent:   tc.margin,
				DiscountEnabled: true,
				DiscountPercent: tc.discount,
			}}, "IDR")
			if len(items) != 1 {
				t.Fatalf("buildItems returned %d items; want 1", len(items))
			}
			item := items[0]
			if item.ListUnitPrice != tc.wantListUnit {
				t.Errorf("list unit = %d; want %d", item.ListUnitPrice, tc.wantListUnit)
			}
			if item.UnitPrice != tc.wantUnit {
				t.Errorf("discounted unit = %d; want %d", item.UnitPrice, tc.wantUnit)
			}
			if item.Total != tc.wantTotal {
				t.Errorf("line total = %d; want %d", item.Total, tc.wantTotal)
			}
			if item.DiscountAmount != tc.wantDiscAmount {
				t.Errorf("discount amount = %d; want %d", item.DiscountAmount, tc.wantDiscAmount)
			}
			// The whole point of #270 applied per row: the cost side does not
			// move when the customer's price does.
			if item.OriginalUnitPrice != tc.cost || item.OriginalTotal != tc.cost*uint64(tc.qty) {
				t.Errorf("cost side = %d/%d; want %d/%d -- a discount is not a cost cut",
					item.OriginalUnitPrice, item.OriginalTotal, tc.cost, tc.cost*uint64(tc.qty))
			}
			if item.MarginAmount != tc.wantMargin {
				t.Errorf("margin = %d; want %d (read after the discount)", item.MarginAmount, tc.wantMargin)
			}
		})
	}
}

// TestUndiscountedLineIsUnchanged is the regression guard for the common case:
// a quotation with no row discounts has to price byte-for-byte as it did
// before this feature existed.
func TestUndiscountedLineIsUnchanged(t *testing.T) {
	items := buildItems([]QuotationItemInput{{
		Description: "Control panel", Qty: 2, UoM: "Lot", UnitPrice: 15000000,
		MarginEnabled: true, MarginPercent: 20,
	}}, "IDR")
	item := items[0]
	if item.UnitPrice != 18000000 || item.Total != 36000000 {
		t.Errorf("priced %d/%d; want 18000000/36000000", item.UnitPrice, item.Total)
	}
	if item.ListUnitPrice != item.UnitPrice {
		t.Errorf("list price %d differs from the sell price %d on an undiscounted line",
			item.ListUnitPrice, item.UnitPrice)
	}
	if item.DiscountAmount != 0 || item.DiscountEnabled {
		t.Errorf("an undiscounted line carries discount %d (enabled=%v); want none",
			item.DiscountAmount, item.DiscountEnabled)
	}
	if item.DisplayDiscount != "0" {
		t.Errorf("display discount = %q; want \"0\"", item.DisplayDiscount)
	}
}
