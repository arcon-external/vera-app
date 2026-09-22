package backend

import (
	"testing"

	"vera-app/backend/auth"
)

func TestResolveTaxMatrix(t *testing.T) {
	cases := []struct {
		name         string
		zone         string
		taxType      string
		category     string
		wantPpn      uint16
		wantServices uint16
		wantErr      bool
	}{
		{"FTZ goods zero", TaxZoneFTZ, TaxTypeGoods, "", 0, 0, false},
		{"FTZ services non-construction 2%", TaxZoneFTZ, TaxTypeServices, ServiceCategoryNonConstruction, 0, 200, false},
		{"FTZ services construction 1.75%", TaxZoneFTZ, TaxTypeServices, ServiceCategoryConstruction, 0, 175, false},
		{"NonFTZ goods PPN 11%", TaxZoneNonFTZ, TaxTypeGoods, "", 1100, 0, false},
		{"NonFTZ services non-construction 2%", TaxZoneNonFTZ, TaxTypeServices, ServiceCategoryNonConstruction, 0, 200, false},
		{"NonFTZ services construction 1.75%", TaxZoneNonFTZ, TaxTypeServices, ServiceCategoryConstruction, 0, 175, false},
		{"unknown zone rejected", "Moon", TaxTypeGoods, "", 0, 0, true},
		{"unknown type rejected", TaxZoneFTZ, "Barter", "", 0, 0, true},
		{"services without category rejected", TaxZoneNonFTZ, TaxTypeServices, "", 0, 0, true},
		{"goods with category rejected", TaxZoneNonFTZ, TaxTypeGoods, ServiceCategoryConstruction, 0, 0, true},
		{"unknown service category rejected", TaxZoneFTZ, TaxTypeServices, "Bogus", 0, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ppn, services, _, err := resolveTax(tc.zone, tc.taxType, tc.category)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ppn != tc.wantPpn || services != tc.wantServices {
				t.Errorf("resolveTax = (%d, %d); want (%d, %d)", ppn, services, tc.wantPpn, tc.wantServices)
			}
		})
	}
}

func TestPrepareGoodsPPN(t *testing.T) {
	// Cost 100.000, no margin, no discount, NonFTZ goods:
	// PPN 11.000, customer grand 111.000, nett 11.000.
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	err := q.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 100000},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		LeadTime:      30,
		CustomerID:    2,
		Incoterm:      "EXW",
		TaxZone:       TaxZoneNonFTZ,
		TaxType:       TaxTypeGoods,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if q.Data.PpnAmount != 11000 {
		t.Errorf("PpnAmount = %d; want 11000", q.Data.PpnAmount)
	}
	if q.Data.GrandTotal != 111000 {
		t.Errorf("GrandTotal = %d; want 111000", q.Data.GrandTotal)
	}
	if q.Data.NettProfit != 11000 {
		t.Errorf("NettProfit = %d; want 11000", q.Data.NettProfit)
	}
}

func TestPrepareGoodsPPNAfterDiscount(t *testing.T) {
	// Cost 100.000, no margin, discount 10% (applies symmetrically to cost
	// and sell): sell grand 90.000, PPN 9.900, customer 99.900;
	// nett 99.900 - 90.000 = 9.900 (the collected PPN).
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	err := q.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 100000},
		},
		Subject:         "Test",
		RecipientName:   "Test",
		Validity:        30,
		PaymentTerm:     30,
		LeadTime:        30,
		CustomerID:      2,
		Incoterm:        "EXW",
		DiscountPercent: 10,
		TaxZone:         TaxZoneNonFTZ,
		TaxType:         TaxTypeGoods,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if q.Data.PpnAmount != 9900 {
		t.Errorf("PpnAmount = %d; want 9900", q.Data.PpnAmount)
	}
	if q.Data.GrandTotal != 99900 {
		t.Errorf("GrandTotal = %d; want 99900", q.Data.GrandTotal)
	}
	if q.Data.NettProfit != 9900 {
		t.Errorf("NettProfit = %d; want 9900", q.Data.NettProfit)
	}
}

func TestLegacyCategoryCompat(t *testing.T) {
	// Old desktop builds send TaxCategory; the server maps them forward so
	// mixed-version rollouts keep working.
	cases := []struct {
		legacy       string
		wantType     string
		wantCategory string
		wantOK       bool
	}{
		{TaxCategorySupplyOnly, TaxTypeGoods, "", true},
		{TaxCategorySupplyInstall, TaxTypeServices, ServiceCategoryNonConstruction, true},
		{TaxCategorySupplyInstallConstruction, TaxTypeServices, ServiceCategoryConstruction, true},
		{"", "", "", false},
		{"Bogus", "", "", false},
	}
	for _, tc := range cases {
		taxType, category, ok := legacyTaxCategoryToNew(tc.legacy)
		if ok != tc.wantOK || taxType != tc.wantType || category != tc.wantCategory {
			t.Errorf("legacyTaxCategoryToNew(%q) = (%q, %q, %v); want (%q, %q, %v)",
				tc.legacy, taxType, category, ok, tc.wantType, tc.wantCategory, tc.wantOK)
		}
	}
}
