package store

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Issue #470: the exact customer-facing PO calculation. These are pure worked
// examples with literal expectations -- never a second implementation of the
// same formula -- so a change in the arithmetic cannot make a test agree with
// it by construction.

func line(qtyScaled, unitPrice int64) PurchaseOrderCalcLine {
	return PurchaseOrderCalcLine{QuantityScaled: qtyScaled, UnitPriceMinor: unitPrice}
}

func discountedLine(qtyScaled, unitPrice int64, percent int16) PurchaseOrderCalcLine {
	return PurchaseOrderCalcLine{
		QuantityScaled:  qtyScaled,
		UnitPriceMinor:  unitPrice,
		DiscountEnabled: true,
		DiscountPercent: percent,
	}
}

func TestPurchaseOrderLineGrossRoundsHalfUpAwayFromZero(t *testing.T) {
	cases := []struct {
		name     string
		qty      int64
		price    int64
		want     int64
		wantName string
	}{
		{"half a minor unit rounds up", 5000, 1, 1, "0.5 -> 1"},
		{"just below half rounds down", 1666, 3, 0, "0.4998 -> 0"},
		{"just above half rounds up", 1667, 3, 1, "0.5001 -> 1"},
		{"four nines rounds up", 13333, 3, 4, "3.9999 -> 4"},
		{"a fraction below the half rounds down", 12345, 100, 123, "123.45 -> 123"},
		{"an exact product keeps its value", 25000, 125050, 312625, "2.5 x 125050"},
		{"a whole quantity is unit price times quantity", 30000, 500, 1500, "3 x 500"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			calculation, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
				Lines:   []PurchaseOrderCalcLine{line(testCase.qty, testCase.price)},
				TaxZone: "FTZ",
			})
			if err != nil {
				t.Fatalf("calculate: %v", err)
			}
			if got := calculation.Lines[0].GrossMinor; got != testCase.want {
				t.Errorf("%s: gross = %d; want %d", testCase.wantName, got, testCase.want)
			}
		})
	}
}

func TestPurchaseOrderDiscountsTruncateLikeTheQuotation(t *testing.T) {
	// 101 x 50% is 50.5 in exact arithmetic. The quotation truncates the
	// discount amount, so the customer keeps the extra half: 101 - 50 = 51.
	calculation, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
		Lines:   []PurchaseOrderCalcLine{discountedLine(10000, 101, 50)},
		TaxZone: "FTZ",
	})
	if err != nil {
		t.Fatalf("calculate row discount: %v", err)
	}
	row := calculation.Lines[0]
	if row.GrossMinor != 101 || row.DiscountMinor != 50 || row.NetMinor != 51 {
		t.Errorf("row = gross %d / discount %d / net %d; want 101/50/51",
			row.GrossMinor, row.DiscountMinor, row.NetMinor)
	}
	if calculation.SubtotalMinor != 51 || calculation.DiscountMinor != 0 ||
		calculation.GrandTotalMinor != 51 {
		t.Errorf("totals = subtotal %d / po discount %d / grand %d; want 51/0/51",
			calculation.SubtotalMinor, calculation.DiscountMinor, calculation.GrandTotalMinor)
	}

	// The whole-PO discount follows the same rule on the summed subtotal.
	whole, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
		Lines:           []PurchaseOrderCalcLine{line(10000, 101)},
		DiscountPercent: 50,
		TaxZone:         "FTZ",
	})
	if err != nil {
		t.Fatalf("calculate whole discount: %v", err)
	}
	if whole.SubtotalMinor != 101 || whole.DiscountMinor != 50 ||
		whole.AfterDiscountMinor != 51 || whole.GrandTotalMinor != 51 {
		t.Errorf("whole discount = subtotal %d / discount %d / after %d / grand %d; want 101/50/51/51",
			whole.SubtotalMinor, whole.DiscountMinor, whole.AfterDiscountMinor, whole.GrandTotalMinor)
	}

	// A disabled row discount is not applied even if a percentage is present.
	disabled := discountedLine(10000, 101, 50)
	disabled.DiscountEnabled = false
	ignored, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
		Lines:   []PurchaseOrderCalcLine{disabled},
		TaxZone: "FTZ",
	})
	if err != nil {
		t.Fatalf("calculate disabled discount: %v", err)
	}
	if ignored.Lines[0].DiscountMinor != 0 || ignored.Lines[0].NetMinor != 101 {
		t.Errorf("disabled row discount was applied: %+v", ignored.Lines[0])
	}
}

func TestPurchaseOrderTaxIsOnTheDiscountedSubtotalAndStaysSeparate(t *testing.T) {
	// 1000, 10% whole discount -> 900. PPN 11% of 900 is 99.00 exactly, and
	// the customer pays 999. Taxing before the discount would give 1010.
	calculation, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
		Lines:           []PurchaseOrderCalcLine{line(10000, 1000)},
		DiscountPercent: 10,
		TaxZone:         "NonFTZ",
		TaxCategory:     "SupplyOnly",
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if calculation.PpnBps != 1100 || calculation.PpnMinor != 99 {
		t.Errorf("PPN = %d bps / %d minor; want 1100/99", calculation.PpnBps, calculation.PpnMinor)
	}
	if calculation.GrandTotalMinor != 999 {
		t.Errorf("grand = %d; want 999 (900 + 99)", calculation.GrandTotalMinor)
	}
	// PPN is reported separately, never folded into the discounted subtotal.
	if calculation.AfterDiscountMinor != 900 {
		t.Errorf("after discount = %d; want 900", calculation.AfterDiscountMinor)
	}

	// A fractional tax amount truncates, exactly like calcTax.
	fractional, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
		Lines:       []PurchaseOrderCalcLine{line(10000, 905)},
		TaxZone:     "NonFTZ",
		TaxCategory: "SupplyOnly",
	})
	if err != nil {
		t.Fatalf("calculate fractional tax: %v", err)
	}
	if fractional.PpnMinor != 99 || fractional.GrandTotalMinor != 1004 {
		t.Errorf("fractional tax = %d / grand %d; want 99/1004",
			fractional.PpnMinor, fractional.GrandTotalMinor)
	}

	// FTZ and services carry no customer-facing PPN.
	for _, taxCase := range []struct{ zone, category string }{
		{"FTZ", ""},
		{"NonFTZ", "SupplyInstall"},
		{"NonFTZ", "SupplyInstallConstruction"},
	} {
		noPpn, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
			Lines:       []PurchaseOrderCalcLine{line(10000, 1000)},
			TaxZone:     taxCase.zone,
			TaxCategory: taxCase.category,
		})
		if err != nil {
			t.Fatalf("%s/%s: %v", taxCase.zone, taxCase.category, err)
		}
		if noPpn.PpnMinor != 0 || noPpn.GrandTotalMinor != 1000 {
			t.Errorf("%s/%s PPN = %d / grand %d; want 0/1000",
				taxCase.zone, taxCase.category, noPpn.PpnMinor, noPpn.GrandTotalMinor)
		}
	}
}

func TestPurchaseOrderAdjustmentIsSignedAndRequiresAReason(t *testing.T) {
	base := PurchaseOrderCalcInput{
		Lines:       []PurchaseOrderCalcLine{line(10000, 1000)},
		TaxZone:     "NonFTZ",
		TaxCategory: "SupplyOnly",
	}

	negative := base
	negative.AdjustmentMinor = -10
	negative.AdjustmentReason = "Round the printed total down"
	calculation, err := CalculatePurchaseOrder(negative)
	if err != nil {
		t.Fatalf("negative adjustment: %v", err)
	}
	if calculation.GrandTotalMinor != 1100 { // 1000 + 110 - 10
		t.Errorf("negative adjustment grand = %d; want 1100", calculation.GrandTotalMinor)
	}
	if calculation.AdjustmentMinor != -10 ||
		calculation.AdjustmentReason != "Round the printed total down" {
		t.Errorf("stored adjustment = %d %q; want -10 with its reason",
			calculation.AdjustmentMinor, calculation.AdjustmentReason)
	}

	positive := base
	positive.AdjustmentMinor = 25
	positive.AdjustmentReason = "Freight printed separately"
	calculation, err = CalculatePurchaseOrder(positive)
	if err != nil {
		t.Fatalf("positive adjustment: %v", err)
	}
	if calculation.GrandTotalMinor != 1135 {
		t.Errorf("positive adjustment grand = %d; want 1135", calculation.GrandTotalMinor)
	}

	// The adjustment never changes row prices or the tax base.
	if calculation.Lines[0].GrossMinor != 1000 ||
		calculation.Lines[0].NetMinor != 1000 ||
		calculation.SubtotalMinor != 1000 || calculation.AfterDiscountMinor != 1000 ||
		calculation.PpnMinor != 110 {
		t.Errorf("adjustment moved the calculation: %+v", calculation)
	}

	for _, reason := range []string{"", "   ", "\t\n"} {
		missing := base
		missing.AdjustmentMinor = -10
		missing.AdjustmentReason = reason
		if _, err := CalculatePurchaseOrder(missing); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("adjustment with reason %q error = %v; want ErrPurchaseOrderInvalid", reason, err)
		}
	}

	// A reason without an amount is not an adjustment: it is dropped rather
	// than stored as an unexplained zero.
	empty := base
	empty.AdjustmentReason = "typed before the amount"
	calculation, err = CalculatePurchaseOrder(empty)
	if err != nil {
		t.Fatalf("zero adjustment with a reason: %v", err)
	}
	if calculation.AdjustmentMinor != 0 || calculation.AdjustmentReason != "" {
		t.Errorf("zero adjustment kept %d %q; want 0 with no reason",
			calculation.AdjustmentMinor, calculation.AdjustmentReason)
	}

	// An adjustment may zero the grand total but may not make it negative.
	exact := base
	exact.AdjustmentMinor = -1110
	exact.AdjustmentReason = "Goodwill"
	if _, err := CalculatePurchaseOrder(exact); err != nil {
		t.Fatalf("adjustment to exactly zero: %v", err)
	}
	over := base
	over.AdjustmentMinor = -1111
	over.AdjustmentReason = "Too much"
	if _, err := CalculatePurchaseOrder(over); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("negative grand total error = %v; want ErrPurchaseOrderInvalid", err)
	}

	// The reason is bounded in Unicode characters, the unit the schema's
	// char_length uses.
	long := base
	long.AdjustmentMinor = -1
	long.AdjustmentReason = strings.Repeat("界", MaxAdjustmentReasonLength+1)
	if _, err := CalculatePurchaseOrder(long); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("over-long reason error = %v; want ErrPurchaseOrderInvalid", err)
	}
	atLimit := base
	atLimit.AdjustmentMinor = -1
	atLimit.AdjustmentReason = strings.Repeat("界", MaxAdjustmentReasonLength)
	if _, err := CalculatePurchaseOrder(atLimit); err != nil {
		t.Errorf("reason at the %d-character limit: %v", MaxAdjustmentReasonLength, err)
	}
}

func TestPurchaseOrderCalculationRejectsInvalidRowsAndPercents(t *testing.T) {
	cases := []struct {
		name  string
		input PurchaseOrderCalcInput
	}{
		{"zero quantity", PurchaseOrderCalcInput{Lines: []PurchaseOrderCalcLine{line(0, 10)}}},
		{"negative quantity", PurchaseOrderCalcInput{Lines: []PurchaseOrderCalcLine{line(-10000, 10)}}},
		{"negative unit price", PurchaseOrderCalcInput{Lines: []PurchaseOrderCalcLine{line(10000, -1)}}},
		{"row discount above 100", PurchaseOrderCalcInput{Lines: []PurchaseOrderCalcLine{
			discountedLine(10000, 100, 101),
		}}},
		{"row discount below 0", PurchaseOrderCalcInput{Lines: []PurchaseOrderCalcLine{
			discountedLine(10000, 100, -1),
		}}},
		{"whole discount above 100", PurchaseOrderCalcInput{
			Lines: []PurchaseOrderCalcLine{line(10000, 100)}, DiscountPercent: 101,
		}},
		{"whole discount below 0", PurchaseOrderCalcInput{
			Lines: []PurchaseOrderCalcLine{line(10000, 100)}, DiscountPercent: -1,
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := CalculatePurchaseOrder(testCase.input); !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Errorf("error = %v; want ErrPurchaseOrderInvalid", err)
			}
		})
	}
}

func TestPurchaseOrderCalculationRejectsOverflowInsteadOfWrapping(t *testing.T) {
	// Every case names the stage in the error. "It failed" is not enough: a
	// wrapped value also fails later, as a negative grand total, and that
	// would let a removed range check pass unnoticed.
	cases := []struct {
		name  string
		input PurchaseOrderCalcInput
		stage string
	}{
		{
			// The line product is 10^4 times an int64, so only a big-integer
			// intermediate can decide -- a direct int64 multiply wraps.
			name:  "line gross",
			input: PurchaseOrderCalcInput{Lines: []PurchaseOrderCalcLine{line(math.MaxInt64, math.MaxInt64)}, TaxZone: "FTZ"},
			stage: "line 1 gross",
		},
		{
			// Two individually valid lines whose subtotal exceeds int64.
			name: "subtotal",
			input: PurchaseOrderCalcInput{
				Lines:   []PurchaseOrderCalcLine{line(10000, math.MaxInt64/2+1), line(10000, math.MaxInt64/2+1)},
				TaxZone: "FTZ",
			},
			stage: "subtotal",
		},
		{
			// The adjustment is added after tax, so it is checked there too.
			name: "grand total",
			input: PurchaseOrderCalcInput{
				Lines:            []PurchaseOrderCalcLine{line(10000, math.MaxInt64)},
				TaxZone:          "FTZ",
				AdjustmentMinor:  math.MaxInt64,
				AdjustmentReason: "Overflow",
			},
			stage: "grand total",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := CalculatePurchaseOrder(testCase.input)
			if !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Fatalf("error = %v; want ErrPurchaseOrderInvalid", err)
			}
			if !strings.Contains(err.Error(), testCase.stage) ||
				!strings.Contains(err.Error(), "exceeds the supported amount range") {
				t.Errorf("error = %v; want %q named as out of range", err, testCase.stage)
			}
		})
	}
}

func TestPurchaseOrderPPNFollowsQuotationTaxSemantics(t *testing.T) {
	cases := []struct {
		zone     string
		category string
		want     int16
	}{
		{"FTZ", "", 0},
		{"NonFTZ", "SupplyOnly", 1100},
		{"NonFTZ", "SupplyInstall", 0},
		{"NonFTZ", "SupplyInstallConstruction", 0},
	}
	for _, testCase := range cases {
		got, err := PurchaseOrderPPNBps(testCase.zone, testCase.category)
		if err != nil {
			t.Fatalf("%s/%s: %v", testCase.zone, testCase.category, err)
		}
		if got != testCase.want {
			t.Errorf("%s/%s = %d bps; want %d", testCase.zone, testCase.category, got, testCase.want)
		}
	}
	for _, bad := range []struct{ zone, category string }{
		{"NonFTZ", ""},
		{"NonFTZ", "Unknown"},
		{"FTZ", "SupplyOnly"},
		{"Nowhere", ""},
	} {
		if _, err := PurchaseOrderPPNBps(bad.zone, bad.category); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("%s/%s error = %v; want ErrPurchaseOrderInvalid", bad.zone, bad.category, err)
		}
	}
}

// TestPurchaseOrderCalculationCarriesNoInternalEconomics mirrors
// TestCustomerPreviewCarriesNoInternalEconomics: supplier cost, margin, PPH22
// and profit must never have a home in this calculation, because a customer
// PO is compared against customer-facing figures only.
func TestPurchaseOrderCalculationCarriesNoInternalEconomics(t *testing.T) {
	forbidden := []string{"cost", "margin", "pph22", "profit", "nett"}
	var inspect func(reflect.Type)
	inspect = func(structType reflect.Type) {
		for index := 0; index < structType.NumField(); index++ {
			field := structType.Field(index)
			lower := strings.ToLower(field.Name)
			for _, word := range forbidden {
				if strings.Contains(lower, word) {
					t.Errorf("%s.%s carries internal economics (%q)",
						structType.Name(), field.Name, word)
				}
			}
			if field.Type.Kind() == reflect.Struct {
				inspect(field.Type)
			}
			if field.Type.Kind() == reflect.Slice &&
				field.Type.Elem().Kind() == reflect.Struct {
				inspect(field.Type.Elem())
			}
		}
	}
	inspect(reflect.TypeOf(PurchaseOrderCalcInput{}))
	inspect(reflect.TypeOf(PurchaseOrderCalc{}))
	inspect(reflect.TypeOf(PurchaseOrderCalcLine{}))
}
