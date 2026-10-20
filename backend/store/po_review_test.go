package store

import (
	"errors"
	"strings"
	"testing"
)

// Issue #471: the pure comparison between an entered Purchase Order and the
// exact Quotation revision it was born from. Matching is by the stored
// item/sub reference -- never by position -- and every differing value is a
// named review slot. Missing (quotation line with no PO line) and extra (PO
// line with no match) lines are review items of their own.
//
// What these tests hold: the comparison, the decision validation and the
// monetary gap rule. The store's finalize transaction is covered separately
// against real Postgres.

func intPointer(value int) *int { return &value }

func reviewSource() PurchaseOrderSource {
	return PurchaseOrderSource{
		Currency:               "USD",
		PaymentTerms:           "45 days",
		Incoterms:              "CIF",
		PrintedGrandTotalMinor: 305000,
		Lines: []PurchaseOrderSourceLine{
			{
				QuotationItemNo: 1,
				Description:     "Plain widget",
				UoM:             "Pc",
				QuantityScaled:  30000,
				UnitPriceMinor:  650,
				DiscountEnabled: true,
				DiscountPercent: 5,
			},
			{
				QuotationItemNo: 2,
				QuotationSubNo:  intPointer(1),
				Description:     "Mounting",
				UoM:             "Unit",
				QuantityScaled:  20000,
				UnitPriceMinor:  420,
			},
		},
	}
}

func matchedReviewState() PurchaseOrderReviewState {
	return PurchaseOrderReviewState{
		PaymentTerms: "45 days",
		Incoterms:    "CIF",
		Lines: []PurchaseOrderItem{
			{
				LineNumber:      1,
				QuotationItemNo: intPointer(1),
				Description:     "Plain widget",
				UoM:             "Pc",
				QuantityScaled:  30000,
				UnitPriceMinor:  650,
				DiscountEnabled: true,
				DiscountPercent: 5,
			},
			{
				LineNumber:      2,
				QuotationItemNo: intPointer(2),
				QuotationSubNo:  intPointer(1),
				Description:     "Mounting",
				UoM:             "Unit",
				QuantityScaled:  20000,
				UnitPriceMinor:  420,
			},
		},
	}
}

func TestComparePurchaseOrderFindsNoDifferenceWhenValuesAgree(t *testing.T) {
	state := matchedReviewState()
	// Surrounding whitespace is not a difference; the commercial text is.
	state.PaymentTerms = "  45 days\t"
	state.Lines[0].Description = " Plain widget "

	comparison := ComparePurchaseOrder(reviewSource(), state)
	if len(comparison.Differences) != 0 {
		t.Fatalf("differences = %+v; want none", comparison.Differences)
	}
	if len(comparison.Missing) != 0 || len(comparison.ExtraLineNumbers) != 0 {
		t.Fatalf("missing/extra = %+v/%v; want none", comparison.Missing, comparison.ExtraLineNumbers)
	}
	if len(comparison.DuplicateLineNumbers) != 0 {
		t.Fatalf("duplicates = %v; want none", comparison.DuplicateLineNumbers)
	}
}

func TestComparePurchaseOrderNamesEveryDifferingValue(t *testing.T) {
	state := matchedReviewState()
	state.PaymentTerms = "30 days after delivery"
	state.Incoterms = "FOB"
	state.Lines[0].Description = "Plain widget, revised"
	state.Lines[0].QuantityScaled = 25000
	state.Lines[0].UoM = "Box"
	state.Lines[0].UnitPriceMinor = 700
	state.Lines[0].DiscountEnabled = false
	state.Lines[0].DiscountPercent = 0

	comparison := ComparePurchaseOrder(reviewSource(), state)
	keys := map[string]bool{}
	for _, difference := range comparison.Differences {
		if difference.Key == "" || difference.Label == "" {
			t.Errorf("difference %+v has no key/label", difference)
		}
		keys[difference.Key] = true
	}
	for _, want := range []string{
		"header:paymentTerms",
		"header:incoterms",
		"line:1:description",
		"line:1:quantity",
		"line:1:uom",
		"line:1:unitPrice",
		"line:1:discount",
	} {
		if !keys[want] {
			t.Errorf("missing difference key %q; got %v", want, keys)
		}
	}
	if len(comparison.Differences) != 7 {
		t.Errorf("differences = %d; want 7", len(comparison.Differences))
	}
	if keys["line:2:description"] {
		t.Error("an equal line produced a difference")
	}
}

func TestComparePurchaseOrderClassifiesMissingExtraAndDuplicates(t *testing.T) {
	state := matchedReviewState()
	// Line 2 stops pointing at the source; line 3 claims the same source line
	// twice and line 4 is a real extra.
	state.Lines[1].QuotationItemNo = nil
	state.Lines[1].QuotationSubNo = nil
	state.Lines[1].LineNumber = 2
	state.Lines = append(state.Lines,
		PurchaseOrderItem{
			LineNumber:      3,
			QuotationItemNo: intPointer(1),
			Description:     "Plain widget duplicate",
			UoM:             "Pc",
			QuantityScaled:  10000,
			UnitPriceMinor:  650,
		},
		PurchaseOrderItem{
			LineNumber:     4,
			Description:    "Freight",
			UoM:            "Lot",
			QuantityScaled: 10000,
			UnitPriceMinor: 5000,
		},
	)

	comparison := ComparePurchaseOrder(reviewSource(), state)
	if len(comparison.Missing) != 1 || comparison.Missing[0].Label() != "2.1" {
		t.Errorf("missing = %+v; want source line 2.1", comparison.Missing)
	}
	if len(comparison.ExtraLineNumbers) != 2 ||
		comparison.ExtraLineNumbers[0] != 2 || comparison.ExtraLineNumbers[1] != 4 {
		t.Errorf("extra = %v; want lines 2 and 4", comparison.ExtraLineNumbers)
	}
	if len(comparison.DuplicateLineNumbers) != 1 || comparison.DuplicateLineNumbers[0] != 3 {
		t.Errorf("duplicates = %v; want line 3 (item 1 already matched by line 1)",
			comparison.DuplicateLineNumbers)
	}
}

func TestValidatePurchaseOrderDecisionsRequiresOneExplicitChoicePerDifference(t *testing.T) {
	state := matchedReviewState()
	state.PaymentTerms = "30 days after delivery"
	state.Incoterms = "FOB"
	state.Lines[0].UnitPriceMinor = 700
	comparison := ComparePurchaseOrder(reviewSource(), state)
	if len(comparison.Differences) != 3 {
		t.Fatalf("differences = %d; want 3", len(comparison.Differences))
	}

	err := ValidatePurchaseOrderDecisions(comparison, nil, nil, nil)
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("no decisions error = %v; want ErrPurchaseOrderUnresolvedDifference", err)
	}
	for _, key := range []string{"header:paymentTerms", "header:incoterms", "line:1:unitPrice"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %q", err, key)
		}
	}

	decisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
		{Key: "line:1:unitPrice", Choice: PurchaseOrderChoicePO},
	}
	if err := ValidatePurchaseOrderDecisions(comparison, decisions, nil, nil); err != nil {
		t.Fatalf("explicit PO choices refused: %v", err)
	}
}

func TestValidatePurchaseOrderDecisionsRefusesALyingQuotationChoice(t *testing.T) {
	state := matchedReviewState()
	state.Incoterms = "FOB"
	comparison := ComparePurchaseOrder(reviewSource(), state)

	err := ValidatePurchaseOrderDecisions(comparison, []PurchaseOrderDecision{
		{Key: "header:incoterms", Choice: PurchaseOrderChoiceQuotation},
	}, nil, nil)
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("lying quotation choice error = %v; want refusal", err)
	}
	if !strings.Contains(err.Error(), "header:incoterms") {
		t.Errorf("error %q does not name the key", err)
	}
}

func TestValidatePurchaseOrderDecisionsRefusesUnknownAndDuplicateChoices(t *testing.T) {
	comparison := ComparePurchaseOrder(reviewSource(), matchedReviewState())

	err := ValidatePurchaseOrderDecisions(comparison, []PurchaseOrderDecision{
		{Key: "line:99:description", Choice: PurchaseOrderChoicePO},
	}, nil, nil)
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) || !strings.Contains(err.Error(), "line:99:description") {
		t.Fatalf("unknown key error = %v; want refusal naming it", err)
	}

	state := matchedReviewState()
	state.Incoterms = "FOB"
	comparison = ComparePurchaseOrder(reviewSource(), state)
	err = ValidatePurchaseOrderDecisions(comparison, []PurchaseOrderDecision{
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoiceQuotation},
	}, nil, nil)
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("duplicate choice error = %v; want refusal", err)
	}
}

func TestValidatePurchaseOrderDecisionsRequiresMissingAndExtraAcknowledgement(t *testing.T) {
	state := matchedReviewState()
	state.Lines[0].QuotationItemNo = nil
	state.Lines = append(state.Lines, PurchaseOrderItem{
		LineNumber:     3,
		Description:    "Freight",
		UoM:            "Lot",
		QuantityScaled: 10000,
		UnitPriceMinor: 5000,
	})
	comparison := ComparePurchaseOrder(reviewSource(), state)
	if len(comparison.Missing) != 1 || len(comparison.ExtraLineNumbers) != 2 {
		t.Fatalf("setup: missing=%d extra=%v", len(comparison.Missing), comparison.ExtraLineNumbers)
	}

	err := ValidatePurchaseOrderDecisions(comparison, nil, nil, nil)
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("unacknowledged error = %v; want refusal", err)
	}
	if !strings.Contains(err.Error(), "quotation line 1") || !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error %q does not name what must be acknowledged", err)
	}

	err = ValidatePurchaseOrderDecisions(comparison, nil, []string{"1"}, []int{1})
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("missing extra acknowledgement error = %v; want refusal", err)
	}

	err = ValidatePurchaseOrderDecisions(comparison, nil, []string{"9"}, []int{1, 3})
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("acknowledgement for a source line that is not missing error = %v; want refusal", err)
	}
}

func TestValidatePurchaseOrderDecisionsAcceptsExactAcknowledgements(t *testing.T) {
	state := matchedReviewState()
	state.Lines[0].QuotationItemNo = nil
	comparison := ComparePurchaseOrder(reviewSource(), state)
	// Lines 1 is now extra; source 1 is missing.
	if len(comparison.Missing) != 1 || len(comparison.ExtraLineNumbers) != 1 {
		t.Fatalf("setup: missing=%+v extra=%v", comparison.Missing, comparison.ExtraLineNumbers)
	}
	if err := ValidatePurchaseOrderDecisions(
		comparison, nil, []string{"1"}, []int{1}); err != nil {
		t.Fatalf("exact acknowledgements refused: %v", err)
	}
}

func TestPurchaseOrderRequiredAdjustmentCoversThePrintedTotalGap(t *testing.T) {
	calc := PurchaseOrderCalc{
		SubtotalMinor:   300000,
		DiscountMinor:   15000,
		PpnMinor:        31350,
		AdjustmentMinor: 0,
		GrandTotalMinor: 316350,
	}
	required, err := PurchaseOrderRequiredAdjustment(316350, calc)
	if err != nil {
		t.Fatalf("required adjustment: %v", err)
	}
	if required != 0 {
		t.Errorf("required = %d; want 0 when the totals already agree", required)
	}

	required, err = PurchaseOrderRequiredAdjustment(300000, calc)
	if err != nil {
		t.Fatalf("required adjustment: %v", err)
	}
	if required != -16350 {
		t.Errorf("required = %d; want -16350", required)
	}

	// An out-of-range printed total is refused rather than wrapped.
	if _, err := PurchaseOrderRequiredAdjustment(0, PurchaseOrderCalc{
		SubtotalMinor:   MaxExactMinor,
		AdjustmentMinor: MaxExactMinor,
		PpnMinor:        MaxExactMinor,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("out-of-range error = %v; want ErrPurchaseOrderInvalid", err)
	}
}

func TestComparePurchaseOrderRefusesTwoLinesForOneSourceLine(t *testing.T) {
	state := matchedReviewState()
	state.Lines[1].QuotationItemNo = intPointer(1)
	state.Lines[1].QuotationSubNo = nil
	comparison := ComparePurchaseOrder(reviewSource(), state)
	if len(comparison.DuplicateLineNumbers) != 1 || comparison.DuplicateLineNumbers[0] != 2 {
		t.Fatalf("duplicates = %v; want [2]", comparison.DuplicateLineNumbers)
	}
	err := ValidatePurchaseOrderDecisions(comparison, nil, nil, nil)
	if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Errorf("duplicate match error = %v; want refusal", err)
	}
}
