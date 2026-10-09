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
	})

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
	})

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
