package backend

import (
	"testing"

	"vera-app/backend/auth"
)

func TestApplyMarkup(t *testing.T) {
	cases := []struct {
		name   string
		cost   uint64
		margin uint8
		want   uint64
	}{
		{"zero margin unchanged", 100000, 0, 100000},
		{"twenty percent", 100000, 20, 120000},
		{"hundred percent doubles", 50000, 100, 100000},
		{"zero cost stays zero", 0, 50, 0},
		// Integer truncation: 999 * 15 / 100 = 149 (not 149.85)
		{"truncates fractional rupiah", 999, 15, 1148},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyMarkup(tc.cost, tc.margin); got != tc.want {
				t.Errorf("applyMarkup(%d, %d) = %d; want %d", tc.cost, tc.margin, got, tc.want)
			}
		})
	}
}

func TestBuildItemsAppliesMargin(t *testing.T) {
	inputs := []QuotationItemInput{
		{Description: "Item A", Qty: 2, UoM: "Lot", UnitPrice: 100000, MarginEnabled: true, MarginPercent: 20},
	}

	items := buildItems(inputs)
	if len(items) != 1 {
		t.Fatalf("buildItems returned %d items; want 1", len(items))
	}
	if items[0].UnitPrice != 120000 {
		t.Errorf("UnitPrice = %d; want 120000 (cost 100000 + 20%%)", items[0].UnitPrice)
	}
	if items[0].Total != 240000 {
		t.Errorf("Total = %d; want 240000 (120000 x 2)", items[0].Total)
	}
	if items[0].DisplayUnitPrice != "120.000" {
		t.Errorf("DisplayUnitPrice = %q; want %q", items[0].DisplayUnitPrice, "120.000")
	}
}

func TestBuildPricesDiscountsSellSide(t *testing.T) {
	// Items already carry sell prices (margin applied at build time).
	items := []QuotationItem{
		{Total: 240000},
		{Total: 60000},
	}

	subTotal, discount, grandTotal := buildPrices(items, 10)
	if subTotal != 300000 {
		t.Errorf("subTotal = %d; want 300000", subTotal)
	}
	if discount != 30000 {
		t.Errorf("discount = %d; want 30000", discount)
	}
	if grandTotal != 270000 {
		t.Errorf("grandTotal = %d; want 270000", grandTotal)
	}
}

func TestPrepareRejectsMarginOver100(t *testing.T) {
	q := &Quotation{}
	q.SetAuthService(auth.StaffTestService())
	err := q.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 1000, MarginEnabled: true, MarginPercent: 101},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		LeadTime:      30,
		CustomerID:    2,
		Incoterm:      "EXW",
	})
	if err == nil {
		t.Fatal("expected error for margin > 100 but got nil")
	}
}
