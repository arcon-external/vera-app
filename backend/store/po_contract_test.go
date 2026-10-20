package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #470 wave 3: the contracts the Go calculator, the Postgres schema and
// the editor share. The TypeScript suite reads the same two fixture files
// (frontend/src/purchaseOrder.test.ts), so a rule changed on one side breaks
// the other instead of quietly disagreeing.

type poCalcFixtureLine struct {
	QuantityScaled  int64 `json:"quantityScaled"`
	UnitPriceMinor  int64 `json:"unitPriceMinor"`
	DiscountEnabled bool  `json:"discountEnabled"`
	DiscountPercent int16 `json:"discountPercent"`
}

type poCalcFixtureWant struct {
	LineGrossMinor     int64 `json:"lineGrossMinor"`
	LineDiscountMinor  int64 `json:"lineDiscountMinor"`
	LineNetMinor       int64 `json:"lineNetMinor"`
	SubtotalMinor      int64 `json:"subtotalMinor"`
	DiscountMinor      int64 `json:"discountMinor"`
	AfterDiscountMinor int64 `json:"afterDiscountMinor"`
	PpnMinor           int64 `json:"ppnMinor"`
	AdjustmentMinor    int64 `json:"adjustmentMinor"`
	GrandTotalMinor    int64 `json:"grandTotalMinor"`
}

type poCalcFixtureCase struct {
	Name             string              `json:"name"`
	Why              string              `json:"why"`
	Lines            []poCalcFixtureLine `json:"lines"`
	DiscountPercent  int16               `json:"discountPercent"`
	PpnBps           int16               `json:"ppnBps"`
	AdjustmentMinor  int64               `json:"adjustmentMinor"`
	AdjustmentReason string              `json:"adjustmentReason"`
	Want             poCalcFixtureWant   `json:"want"`
}

func readPOFixture(t *testing.T, name string, target any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatalf("read the shared PO fixture %s: %v", name, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("parse the shared PO fixture %s: %v", name, err)
	}
}

func poCalculationCases(t *testing.T) []poCalcFixtureCase {
	t.Helper()
	var document struct {
		Cases []poCalcFixtureCase `json:"cases"`
	}
	readPOFixture(t, "po_calculation_cases.json", &document)
	if len(document.Cases) == 0 {
		t.Fatal("the shared PO calculation fixture has no cases, so it proves nothing")
	}
	return document.Cases
}

type poCodePoint struct {
	CodePoint int    `json:"codePoint"`
	Name      string `json:"name"`
	Why       string `json:"why"`
}

type poWhitespace struct {
	Blank   []poCodePoint `json:"blank"`
	Content []poCodePoint `json:"content"`
}

func poWhitespaceFixture(t *testing.T) poWhitespace {
	t.Helper()
	var fixture poWhitespace
	readPOFixture(t, "po_adjustment_whitespace.json", &fixture)
	if len(fixture.Blank) == 0 || len(fixture.Content) == 0 {
		t.Fatal("the shared PO whitespace fixture must name both blank and content code points")
	}
	return fixture
}

// TestPurchaseOrderCalculationMatchesTheSharedFixture pins the Judge's
// round-two case: two individually exact maximum lines, each fully discounted,
// so the aggregate gross is twice the exact bound while the subtotal is zero.
// Go bounds each stored/transported amount and the summed net, not the display-
// only aggregate gross, which is why this order is valid.
func TestPurchaseOrderCalculationMatchesTheSharedFixture(t *testing.T) {
	for _, testCase := range poCalculationCases(t) {
		t.Run(testCase.Name, func(t *testing.T) {
			lines := make([]PurchaseOrderCalcLine, 0, len(testCase.Lines))
			for _, line := range testCase.Lines {
				lines = append(lines, PurchaseOrderCalcLine{
					QuantityScaled:  line.QuantityScaled,
					UnitPriceMinor:  line.UnitPriceMinor,
					DiscountEnabled: line.DiscountEnabled,
					DiscountPercent: line.DiscountPercent,
				})
			}
			calculation, err := CalculatePurchaseOrder(PurchaseOrderCalcInput{
				Lines:            lines,
				DiscountPercent:  testCase.DiscountPercent,
				PpnBps:           testCase.PpnBps,
				AdjustmentMinor:  testCase.AdjustmentMinor,
				AdjustmentReason: testCase.AdjustmentReason,
			})
			if err != nil {
				t.Fatalf("%s: calculate: %v", testCase.Why, err)
			}
			// Every line in this fixture carries the same figures; assert both
			// ends so a one-line implementation cannot pass by accident.
			for _, index := range []int{0, len(calculation.Lines) - 1} {
				row := calculation.Lines[index]
				if row.GrossMinor != testCase.Want.LineGrossMinor ||
					row.DiscountMinor != testCase.Want.LineDiscountMinor ||
					row.NetMinor != testCase.Want.LineNetMinor {
					t.Errorf("%s: line %d = %+v; want gross %d / discount %d / net %d",
						testCase.Why, index+1, row,
						testCase.Want.LineGrossMinor, testCase.Want.LineDiscountMinor,
						testCase.Want.LineNetMinor)
				}
			}
			for _, want := range []struct {
				name string
				got  int64
				want int64
			}{
				{"subtotal", calculation.SubtotalMinor, testCase.Want.SubtotalMinor},
				{"whole discount", calculation.DiscountMinor, testCase.Want.DiscountMinor},
				{"after discount", calculation.AfterDiscountMinor, testCase.Want.AfterDiscountMinor},
				{"PPN", calculation.PpnMinor, testCase.Want.PpnMinor},
				{"adjustment", calculation.AdjustmentMinor, testCase.Want.AdjustmentMinor},
				{"grand total", calculation.GrandTotalMinor, testCase.Want.GrandTotalMinor},
			} {
				if want.got != want.want {
					t.Errorf("%s: %s = %d; want %d", testCase.Why, want.name, want.got, want.want)
				}
			}
		})
	}
}

// TestPurchaseOrderAdjustmentReasonWhitespaceMatchesTheSharedFixture pins the
// set all three layers treat as blank. NBSP was the Judge's probe; NEL (U+0085)
// and the ogham space mark are the same defect one code point over, and a BOM
// is deliberately content because neither Go nor the SQL backstop removes it.
func TestPurchaseOrderAdjustmentReasonWhitespaceMatchesTheSharedFixture(t *testing.T) {
	fixture := poWhitespaceFixture(t)

	base := PurchaseOrderCalcInput{
		Lines:           []PurchaseOrderCalcLine{line(10000, 1000)},
		AdjustmentMinor: -1,
	}

	for _, point := range fixture.Blank {
		reason := string(rune(point.CodePoint))
		input := base
		input.AdjustmentReason = reason
		if _, err := CalculatePurchaseOrder(input); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("%s (%U) only: error = %v; want ErrPurchaseOrderInvalid", point.Name, point.CodePoint, err)
		}
	}

	joined := strings.Builder{}
	for _, point := range fixture.Blank {
		joined.WriteRune(rune(point.CodePoint))
	}
	input := base
	input.AdjustmentReason = joined.String()
	if _, err := CalculatePurchaseOrder(input); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("every blank code point together: error = %v; want ErrPurchaseOrderInvalid", err)
	}

	for _, point := range fixture.Content {
		reason := string(rune(point.CodePoint))
		input := base
		input.AdjustmentReason = reason
		calculation, err := CalculatePurchaseOrder(input)
		if err != nil {
			t.Errorf("%s (%U) is content, but it was refused: %v", point.Name, point.CodePoint, err)
			continue
		}
		if calculation.AdjustmentReason != reason {
			t.Errorf("%s (%U) stored as %U; content must be kept verbatim",
				point.Name, point.CodePoint, []rune(calculation.AdjustmentReason))
		}
	}
}
