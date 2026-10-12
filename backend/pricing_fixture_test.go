package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This test and frontend/src/pricing.test.ts read the same file.
//
// Pricing is implemented twice — buildPricing here and computePricing in
// SummaryRail — because the editor shows totals before Prepare runs. Two
// implementations of one rule is the setup for drift, and drift here is a number
// the customer is quoted.
//
// The TypeScript test used to claim it would catch that: "these cases are the
// ones Go asserts ... If one side changes a rule, the other side fails here
// rather than quietly disagreeing." It would not. Both sides asserted their own
// literals, so a Go change failed the Go test, someone updated the Go literal,
// and the TypeScript test carried on passing with the old number.
//
// A shared fixture is what makes the claim true: the expectation lives in one
// place, so changing a rule on either side breaks whichever side no longer
// matches it.
//
// The expected numbers were derived from buildPricing, which is canonical — the
// server does the pricing that reaches a customer. The fixture pins current
// behaviour; it does not assert what the behaviour should be. That is what the
// rule-specific tests alongside it are for.

type fixtureLine struct {
	Cost            uint64        `json:"cost"`
	Qty             uint32        `json:"qty"`
	MarginEnabled   bool          `json:"marginEnabled"`
	MarginPercent   uint16        `json:"marginPercent"`
	DiscountEnabled bool          `json:"discountEnabled"`
	DiscountPercent uint16        `json:"discountPercent"`
	SubItems        []fixtureLine `json:"subItems"`
}

type fixtureCase struct {
	Name                   string           `json:"name"`
	Why                    string           `json:"why"`
	Lines                  []fixtureLine    `json:"lines"`
	OverallDiscountPercent uint8            `json:"overallDiscountPercent"`
	TaxZone                string           `json:"taxZone"`
	TaxType                string           `json:"taxType"`
	ServiceCategory        string           `json:"serviceCategory"`
	Want                   map[string]int64 `json:"want"`
}

// pricingFixture is loaded once; the file is small and read-only.
func pricingFixture(t *testing.T) []fixtureCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "pricing_cases.json"))
	if err != nil {
		t.Fatalf("read the shared pricing fixture: %v", err)
	}
	var doc struct {
		Cases []fixtureCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse the shared pricing fixture: %v", err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("the shared pricing fixture has no cases, so it proves nothing")
	}
	return doc.Cases
}

func (l fixtureLine) toInput() QuotationItemInput {
	in := QuotationItemInput{
		Description:     "line",
		UoM:             "Lot",
		Qty:             l.Qty,
		UnitPrice:       l.Cost,
		MarginEnabled:   l.MarginEnabled,
		MarginPercent:   l.MarginPercent,
		DiscountEnabled: l.DiscountEnabled,
		DiscountPercent: l.DiscountPercent,
	}
	for _, sub := range l.SubItems {
		in.SubItems = append(in.SubItems, QuotationSubItemInput{
			Description:     "part",
			UoM:             "Pc",
			Qty:             sub.Qty,
			UnitCost:        sub.Cost,
			MarginEnabled:   sub.MarginEnabled,
			MarginPercent:   sub.MarginPercent,
			DiscountEnabled: sub.DiscountEnabled,
			DiscountPercent: sub.DiscountPercent,
		})
	}
	return in
}

// actual reads the summary through the same field names the fixture uses, so the
// two sides are compared on meaning rather than on Go's struct naming.
func actual(s PricingSummary) map[string]int64 {
	return map[string]int64{
		"costSubtotal":    int64(s.OriginalSubtotal),
		"sellSubtotal":    int64(s.MarginalizedSubtotal),
		"sellDiscount":    int64(s.MarginalizedDiscount),
		"sellGrand":       int64(s.MarginalizedGrand),
		"ppnAmount":       int64(s.PpnAmount),
		"customerGrand":   int64(s.CustomerGrandTotal),
		"taxAmount":       int64(s.TaxAmount),
		"grandAfterTax":   int64(s.MarginalizedGrandAfterTax),
		"profitBeforeTax": s.ProfitAmount,
		"netProfit":       s.NettProfit,
	}
}

func TestPricingMatchesTheSharedFixture(t *testing.T) {
	for _, tc := range pricingFixture(t) {
		t.Run(tc.Name, func(t *testing.T) {
			inputs := make([]QuotationItemInput, 0, len(tc.Lines))
			for _, line := range tc.Lines {
				inputs = append(inputs, line.toInput())
			}
			ppnBps, servicesBps, _, err := resolveTax(tc.TaxZone, tc.TaxType, tc.ServiceCategory)
			if err != nil {
				t.Fatalf("%s: resolveTax(%q, %q, %q): %v",
					tc.Why, tc.TaxZone, tc.TaxType, tc.ServiceCategory, err)
			}
			got := actual(buildPricing(buildItems(inputs, "IDR"), tc.OverallDiscountPercent, ppnBps, servicesBps))

			// Every field the fixture states must match, and every field the
			// fixture omits is simply not asserted -- so adding a field to the
			// fixture is how a new value gets pinned, with no code change here.
			for field, want := range tc.Want {
				gotValue, ok := got[field]
				if !ok {
					t.Fatalf("the fixture asserts %q, which this test does not read; "+
						"add it to actual() so the assertion is real", field)
				}
				if gotValue != want {
					t.Errorf("%s: %s = %d; want %d", tc.Why, field, gotValue, want)
				}
			}
		})
	}
}

// TestPricingFixtureCoversTheRulesThatWereWrong keeps the fixture honest.
//
// A fixture that drifts into testing one easy case eight times would still pass
// while the interesting rules went unpinned. These are the cases that have
// actually been got wrong here, so the fixture must keep one of each.
func TestPricingFixtureCoversTheRulesThatWereWrong(t *testing.T) {
	cases := pricingFixture(t)

	wanted := map[string]string{
		"a row discounted below cost earns a negative margin":                                  "the signed-margin rule",
		"PPN is charged on top of the price for non-FTZ goods":                                 "PPN on top, never profit",
		"PPH22 is withheld on services, and is internal":                                       "PPH22 withheld, never printed",
		"FTZ goods carry no PPN and no PPH22, so nett equals the profit":                       "the no-tax baseline",
		"the whole-quotation discount applies to the summed subtotal, on top of row discounts": "discount ordering",
		"a split line prices through its parts, not itself":                                    "the sub-item rollup",
		"truncation lands in the same place on both sides":                                     "truncation, not rounding",
	}

	names := map[string]bool{}
	for _, c := range cases {
		names[c.Name] = true
	}
	for name, rule := range wanted {
		if !names[name] {
			t.Errorf("the fixture no longer covers %q, which pins %s", name, rule)
		}
	}
}

// TestNegativeMarginIsSigned pins the rule the unsigned-arithmetic bug would
// break, in a form that reads as the invariant rather than as a number.
//
// A row sold below cost must show a loss. The struct carries int64 precisely so
// it can, and a future change to uint64 would wrap the loss into a huge gain --
// which is why this asserts the sign rather than only the total.
func TestNegativeMarginIsSigned(t *testing.T) {
	for _, tc := range pricingFixture(t) {
		summary := func() PricingSummary {
			inputs := make([]QuotationItemInput, 0, len(tc.Lines))
			for _, line := range tc.Lines {
				inputs = append(inputs, line.toInput())
			}
			ppnBps, servicesBps, _, err := resolveTax(tc.TaxZone, tc.TaxType, tc.ServiceCategory)
			if err != nil {
				t.Fatal(err)
			}
			return buildPricing(buildItems(inputs, "IDR"), tc.OverallDiscountPercent, ppnBps, servicesBps)
		}()

		if summary.MarginalizedGrand < summary.OriginalGrand {
			if summary.ProfitAmount >= 0 {
				t.Errorf("%s: the customer pays less than the cost side, but profit is %d; "+
					"a loss must be negative", tc.Name, summary.ProfitAmount)
			}
		}
	}
}
