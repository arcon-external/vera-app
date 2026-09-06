package backend

import "testing"

func TestTaxRateFor(t *testing.T) {
	cases := []struct {
		name     string
		zone     string
		category string
		wantBps  uint16
		wantErr  bool
	}{
		{"FTZ no category", TaxZoneFTZ, "", 0, false},
		{"FTZ ignores category is rejected", TaxZoneFTZ, TaxCategorySupplyOnly, 0, true},
		{"NonFTZ supply only 11%", TaxZoneNonFTZ, TaxCategorySupplyOnly, 1100, false},
		{"NonFTZ supply+install 2%", TaxZoneNonFTZ, TaxCategorySupplyInstall, 200, false},
		{"NonFTZ construction 1.75%", TaxZoneNonFTZ, TaxCategorySupplyInstallConstruction, 175, false},
		{"NonFTZ without category rejected", TaxZoneNonFTZ, "", 0, true},
		{"unknown zone rejected", "Moon", "", 0, true},
		{"unknown category rejected", TaxZoneNonFTZ, "Bogus", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bps, _, err := taxRateFor(tc.zone, tc.category)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bps != tc.wantBps {
				t.Errorf("bps = %d; want %d", bps, tc.wantBps)
			}
		})
	}
}

func TestCalcTax(t *testing.T) {
	cases := []struct {
		name  string
		grand uint64
		bps   uint16
		want  uint64
	}{
		{"zero rate", 1000000, 0, 0},
		{"2% of 1.000.000", 1000000, 200, 20000},
		{"11% of 1.000.000", 1000000, 1100, 110000},
		{"1.75% of 100.000", 100000, 175, 1750},
		// Integer truncation: 999 x 175 / 10000 = 17 (not 17.48)
		{"truncates fractional rupiah", 999, 175, 17},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calcTax(tc.grand, tc.bps); got != tc.want {
				t.Errorf("calcTax(%d, %d) = %d; want %d", tc.grand, tc.bps, got, tc.want)
			}
		})
	}
}

func TestPrepareTaxValidation(t *testing.T) {
	validItem := QuotationItemInput{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 1000}
	base := func() QuotationInput {
		return QuotationInput{
			Items:         []QuotationItemInput{validItem},
			Subject:       "Test",
			RecipientName: "Test",
			Validity:      30,
			PaymentTerm:   30,
			LeadTime:      30,
			CustomerID:    2,
			Incoterm:      "EXW",
		}
	}

	t.Run("NonFTZ without category fails", func(t *testing.T) {
		in := base()
		in.TaxZone = TaxZoneNonFTZ
		if err := (&Quotation{}).Prepare(in); err == nil {
			t.Fatal("expected error but got nil")
		}
	})

	t.Run("FTZ with category fails", func(t *testing.T) {
		in := base()
		in.TaxZone = TaxZoneFTZ
		in.TaxCategory = TaxCategorySupplyOnly
		if err := (&Quotation{}).Prepare(in); err == nil {
			t.Fatal("expected error but got nil")
		}
	})

	t.Run("unknown zone fails", func(t *testing.T) {
		in := base()
		in.TaxZone = "Moon"
		if err := (&Quotation{}).Prepare(in); err == nil {
			t.Fatal("expected error but got nil")
		}
	})
}

func TestPrepareMarginDiscountTaxChain(t *testing.T) {
	// Cost 100.000, margin 20% -> sell 120.000; discount 10% -> grand
	// 108.000; NonFTZ 2% -> tax 2.160, nett 105.840.
	q := &Quotation{}
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
		ProfitMargin:    20,
		DiscountPercent: 10,
		TaxZone:         TaxZoneNonFTZ,
		TaxCategory:     TaxCategorySupplyInstall,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if q.Data.SubTotal != 120000 {
		t.Errorf("SubTotal = %d; want 120000", q.Data.SubTotal)
	}
	if q.Data.DiscountAmount != 12000 {
		t.Errorf("DiscountAmount = %d; want 12000", q.Data.DiscountAmount)
	}
	if q.Data.GrandTotal != 108000 {
		t.Errorf("GrandTotal = %d; want 108000", q.Data.GrandTotal)
	}
	if q.Data.TaxAmount != 2160 {
		t.Errorf("TaxAmount = %d; want 2160", q.Data.TaxAmount)
	}
	if q.Data.NettProfit != 105840 {
		t.Errorf("NettProfit = %d; want 105840", q.Data.NettProfit)
	}
}

func TestPrepareComputesNett(t *testing.T) {
	q := &Quotation{}
	err := q.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 1000000},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      30,
		PaymentTerm:   30,
		LeadTime:      30,
		CustomerID:    2,
		Incoterm:      "EXW",
		TaxZone:       TaxZoneNonFTZ,
		TaxCategory:   TaxCategorySupplyInstall,
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if q.Data.GrandTotal != 1000000 {
		t.Errorf("GrandTotal = %d; want 1000000", q.Data.GrandTotal)
	}
	if q.Data.TaxAmount != 20000 {
		t.Errorf("TaxAmount = %d; want 20000", q.Data.TaxAmount)
	}
	if q.Data.NettProfit != 980000 {
		t.Errorf("NettProfit = %d; want 980000", q.Data.NettProfit)
	}

	ftz := &Quotation{}
	err = ftz.Prepare(QuotationInput{
		Items: []QuotationItemInput{
			{Description: "Item", Qty: 1, UoM: "Lot", UnitPrice: 1000000},
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
	if err != nil {
		t.Fatalf("FTZ Prepare failed: %v", err)
	}
	if ftz.Data.TaxAmount != 0 {
		t.Errorf("FTZ TaxAmount = %d; want 0", ftz.Data.TaxAmount)
	}
	if ftz.Data.NettProfit != ftz.Data.GrandTotal {
		t.Errorf("FTZ NettProfit = %d; want GrandTotal %d", ftz.Data.NettProfit, ftz.Data.GrandTotal)
	}
}
