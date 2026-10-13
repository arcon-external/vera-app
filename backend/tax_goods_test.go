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
		// The retired TaxCategory vocabulary, passed through the field that does
		// not use it. The CLI documented these as serviceCategory values and sent
		// callers here, where they are refused -- so this is the trap, named.
		{"retired SupplyOnly rejected", TaxZoneNonFTZ, TaxTypeServices, TaxCategorySupplyOnly, 0, 0, true},
		{"retired SupplyInstall rejected", TaxZoneNonFTZ, TaxTypeServices, TaxCategorySupplyInstall, 0, 0, true},
		{"retired SupplyInstallConstruction rejected", TaxZoneNonFTZ, TaxTypeServices, TaxCategorySupplyInstallConstruction, 0, 0, true},
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
	// PPN 11.000, customer grand 111.000, nett 0.
	//
	// Nett is zero because the item sells at cost and the 11.000 of PPN on top
	// is collected for the state: it never becomes our profit.
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
		LeadTimeDays:  30,
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
	if q.Data.NettProfit != 0 {
		t.Errorf("NettProfit = %d; want 0 (PPN is collected, not earned)", q.Data.NettProfit)
	}
}

func TestPrepareGoodsPPNAfterDiscount(t *testing.T) {
	// Cost 100.000, no margin, discount 10%: the discount cuts only the price,
	// so sell grand is 90.000 against a cost of 100.000. PPN 9.900 goes on top
	// for the customer (99.900) and is removed again before profit, so the item
	// is a loss of 10.000.
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
		LeadTimeDays:    30,
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
	if q.Data.NettProfit != -10000 {
		t.Errorf("NettProfit = %d; want -10000 (90.000 sell - 100.000 cost)", q.Data.NettProfit)
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

// TestProfitFollowsConfirmedTaxFlow pins the company's confirmed flow on one
// example per case: cost 10.000, a 10% discount and no margin, so the price
// becomes 9.000 and the cost stays 10.000.
//
//   - PPN is charged on top of the price for non-FTZ goods and is never
//     profit: the customer pays 9.990 and our profit is 9.000 - 10.000.
//   - FTZ goods carry no tax at all, so the customer pays 9.000 and the loss
//     is the same 1.000.
//   - Services carry a PPh22 withholding instead: the customer pays 9.000,
//     2% comes off internally, and the loss is 1.180.
//
// The discount cutting only the price is what makes the goods cases a loss
// rather than break-even; that is the point of the test.
func TestProfitFollowsConfirmedTaxFlow(t *testing.T) {
	cases := []struct {
		name            string
		taxZone         string
		taxType         string
		serviceCategory string
		wantPpn         uint64
		wantCustomer    uint64
		wantNett        int64
	}{
		{"goods non-FTZ pays PPN on top", TaxZoneNonFTZ, TaxTypeGoods, "", 990, 9990, -1000},
		{"goods FTZ pays no tax", TaxZoneFTZ, TaxTypeGoods, "", 0, 9000, -1000},
		{"services withhold PPh22", TaxZoneNonFTZ, TaxTypeServices, ServiceCategoryNonConstruction, 0, 9000, -1180},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := newTestQuotation()
			q.SetAuthService(auth.StaffTestService())
			if err := q.Prepare(QuotationInput{
				Items:           []QuotationItemInput{{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 10000}},
				Subject:         "Test",
				RecipientName:   "Test",
				Validity:        30,
				PaymentTerm:     30,
				LeadTimeDays:    30,
				CustomerID:      2,
				Incoterm:        "EXW",
				DiscountPercent: 10,
				TaxZone:         tc.taxZone,
				TaxType:         tc.taxType,
				ServiceCategory: tc.serviceCategory,
			}); err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if q.Data.PpnAmount != tc.wantPpn {
				t.Errorf("PpnAmount = %d; want %d", q.Data.PpnAmount, tc.wantPpn)
			}
			if q.Data.GrandTotal != tc.wantCustomer {
				t.Errorf("customer grand total = %d; want %d", q.Data.GrandTotal, tc.wantCustomer)
			}
			if q.Data.NettProfit != tc.wantNett {
				t.Errorf("NettProfit = %d; want %d", q.Data.NettProfit, tc.wantNett)
			}
			// The cost side never carries the discount.
			if q.Data.OriginalDiscountAmount != 0 || q.Data.OriginalGrandTotal != 10000 {
				t.Errorf("cost side = %d discount / %d grand; want 0/10000",
					q.Data.OriginalDiscountAmount, q.Data.OriginalGrandTotal)
			}
		})
	}
}
