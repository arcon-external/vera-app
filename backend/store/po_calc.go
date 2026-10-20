package store

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"
)

// Issue #470: the exact customer-facing calculation of a Purchase Order.
//
// The PO is entered against a customer document, so every figure here is a
// customer-facing one. Supplier cost, margin, PPH22 and profit have no place in
// this file: a PO is compared against the quotation's customer prices,
// discounts and PPN only (parent story 42).
//
// # Quantity
//
// Quantity travels as an exact count of 1/10000 units (`QuantityScale`). Input
// parsing rejects a fifth decimal place rather than rounding it; this package
// only ever sees the scaled integer, and refuses a non-positive one.
//
// # Money and rounding
//
// Money is int64 minor units. No float appears at any stage. Each stage names
// its own rounding:
//
//  1. line gross    = round-half-away-from-zero(qty_scaled * unit_price / 10^4)
//  2. row discount  = truncate(gross * row_percent / 100); net = gross - discount
//  3. subtotal      = exact sum of row nets
//  4. PO discount   = truncate(subtotal * po_percent / 100); after = subtotal - discount
//  5. PPN           = truncate(after * ppn_bps / 10000), added on top
//  6. adjustment    = signed integer, reason required, added last
//  7. grand total   = after + PPN + adjustment, never negative
//
// Stage 1 is the only half-up stage, because it is the only one that turns a
// fractional quantity into money: 0.5 unit at 1 minor unit rounds to 1. Stages
// 2 and 4 truncate so the customer is never charged less than the discount
// stated, and they follow the quotation's existing discount arithmetic
// (`applyDiscount` in backend/builder.go) exactly. Stage 5 truncates because
// that is what the issued quotation does (`calcTax` in backend/tax.go).
//
// A discount is a price cut only: it never touches a cost side, because this
// document has no cost side.
const (
	// MaxDiscountPercent is the ceiling on a row or whole-PO discount. 100% is
	// a free line; above it is a price rather than a discount. It mirrors the
	// quotation's MaxDiscountPercent.
	MaxDiscountPercent = 100

	// MaxAdjustmentReasonLength bounds the free-text reason in Unicode
	// characters, the unit the schema's char_length CHECK uses.
	MaxAdjustmentReasonLength = 200

	// ppnRateBps is the customer-facing PPN rate for Non-FTZ goods, in basis
	// points. It mirrors ppnRateBps in backend/tax.go; a test pins the two
	// together rather than letting the pair drift.
	ppnRateBps int16 = 1100
)

// PurchaseOrderCalcLine is one row's inputs and, after calculation, its exact
// money figures. A caller passes the inputs; CalculatePurchaseOrder fills the
// calculated fields in a copy.
type PurchaseOrderCalcLine struct {
	QuantityScaled  int64
	UnitPriceMinor  int64
	DiscountEnabled bool
	DiscountPercent int16
	GrossMinor      int64
	DiscountMinor   int64
	NetMinor        int64
}

// PurchaseOrderCalcInput is everything the calculation reads. Printed source
// totals are deliberately absent: they are evidence copied from the issued
// revision, never an input to this arithmetic.
type PurchaseOrderCalcInput struct {
	Lines            []PurchaseOrderCalcLine
	DiscountPercent  int16
	TaxZone          string
	TaxCategory      string
	AdjustmentMinor  int64
	AdjustmentReason string
}

// PurchaseOrderCalc is the explained calculation: every stage is visible, so
// the editor, the API and a later difference review can show where a total
// came from instead of only its final figure.
type PurchaseOrderCalc struct {
	Lines              []PurchaseOrderCalcLine
	SubtotalMinor      int64
	DiscountMinor      int64
	AfterDiscountMinor int64
	PpnBps             int16
	PpnMinor           int64
	AdjustmentMinor    int64
	AdjustmentReason   string
	GrandTotalMinor    int64
}

// CalculatePurchaseOrder applies the pipeline above to one PO. It returns
// ErrPurchaseOrderInvalid (wrapped, with the offending stage named) for an
// out-of-range input, a forbidden adjustment or any intermediate that would
// exceed int64: an overflowing PO is refused rather than wrapped.
func CalculatePurchaseOrder(input PurchaseOrderCalcInput) (PurchaseOrderCalc, error) {
	ppnBps, err := PurchaseOrderPPNBps(input.TaxZone, input.TaxCategory)
	if err != nil {
		return PurchaseOrderCalc{}, err
	}
	if input.DiscountPercent < 0 || input.DiscountPercent > MaxDiscountPercent {
		return PurchaseOrderCalc{}, fmt.Errorf(
			"%w: whole purchase order discount must be between 0 and %d percent",
			ErrPurchaseOrderInvalid, MaxDiscountPercent)
	}
	adjustment, reason, err := normalizePurchaseOrderAdjustment(input.AdjustmentMinor, input.AdjustmentReason)
	if err != nil {
		return PurchaseOrderCalc{}, err
	}

	lines := make([]PurchaseOrderCalcLine, len(input.Lines))
	subtotal := new(big.Int)
	for index, item := range input.Lines {
		if item.QuantityScaled <= 0 {
			return PurchaseOrderCalc{}, fmt.Errorf(
				"%w: line %d quantity must be positive", ErrPurchaseOrderInvalid, index+1)
		}
		if item.UnitPriceMinor < 0 {
			return PurchaseOrderCalc{}, fmt.Errorf(
				"%w: line %d unit price must not be negative", ErrPurchaseOrderInvalid, index+1)
		}
		if item.DiscountPercent < 0 || item.DiscountPercent > MaxDiscountPercent {
			return PurchaseOrderCalc{}, fmt.Errorf(
				"%w: line %d discount must be between 0 and %d percent",
				ErrPurchaseOrderInvalid, index+1, MaxDiscountPercent)
		}

		gross, err := poMulDiv(item.QuantityScaled, item.UnitPriceMinor, QuantityScale, true)
		if err != nil {
			return PurchaseOrderCalc{}, fmt.Errorf("line %d gross: %w", index+1, err)
		}
		discount := int64(0)
		if item.DiscountEnabled && item.DiscountPercent > 0 {
			discount, err = poMulDiv(gross, int64(item.DiscountPercent), 100, false)
			if err != nil {
				return PurchaseOrderCalc{}, fmt.Errorf("line %d discount: %w", index+1, err)
			}
		}
		net := gross - discount
		lines[index] = PurchaseOrderCalcLine{
			QuantityScaled:  item.QuantityScaled,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
			GrossMinor:      gross,
			DiscountMinor:   discount,
			NetMinor:        net,
		}
		subtotal.Add(subtotal, big.NewInt(net))
	}

	subtotalMinor, err := poCheckedInt64(subtotal, "subtotal")
	if err != nil {
		return PurchaseOrderCalc{}, err
	}
	discountMinor, err := poMulDiv(subtotalMinor, int64(input.DiscountPercent), 100, false)
	if err != nil {
		return PurchaseOrderCalc{}, fmt.Errorf("whole purchase order discount: %w", err)
	}
	afterDiscount := subtotalMinor - discountMinor
	ppnMinor, err := poMulDiv(afterDiscount, int64(ppnBps), 10000, false)
	if err != nil {
		return PurchaseOrderCalc{}, fmt.Errorf("PPN: %w", err)
	}

	grand := new(big.Int).SetInt64(afterDiscount)
	grand.Add(grand, big.NewInt(ppnMinor))
	grand.Add(grand, big.NewInt(adjustment))
	if grand.Sign() < 0 {
		return PurchaseOrderCalc{}, fmt.Errorf(
			"%w: the adjustment would make the purchase order total negative", ErrPurchaseOrderInvalid)
	}
	grandTotal, err := poCheckedInt64(grand, "grand total")
	if err != nil {
		return PurchaseOrderCalc{}, err
	}

	return PurchaseOrderCalc{
		Lines:              lines,
		SubtotalMinor:      subtotalMinor,
		DiscountMinor:      discountMinor,
		AfterDiscountMinor: afterDiscount,
		PpnBps:             ppnBps,
		PpnMinor:           ppnMinor,
		AdjustmentMinor:    adjustment,
		AdjustmentReason:   reason,
		GrandTotalMinor:    grandTotal,
	}, nil
}

// PurchaseOrderPPNBps resolves the customer-facing PPN rate a PO carries. It
// follows the quotation's tax model exactly: 11% for Non-FTZ goods, zero for
// FTZ and for services. PPH22 (2% / 1.75%, internal only) never applies here,
// which is why the legacy service categories resolve to zero rather than to
// their internal rate.
func PurchaseOrderPPNBps(zone, category string) (int16, error) {
	switch zone {
	case "FTZ":
		if category != "" {
			return 0, fmt.Errorf("%w: an FTZ purchase order must not carry a tax category",
				ErrPurchaseOrderInvalid)
		}
		return 0, nil
	case "NonFTZ":
		switch category {
		case "SupplyOnly":
			return ppnRateBps, nil
		case "SupplyInstall", "SupplyInstallConstruction":
			return 0, nil
		default:
			return 0, fmt.Errorf(
				"%w: a Non-FTZ purchase order needs a goods or services tax category",
				ErrPurchaseOrderInvalid)
		}
	default:
		return 0, fmt.Errorf("%w: unknown tax zone %q", ErrPurchaseOrderInvalid, zone)
	}
}

// normalizePurchaseOrderAdjustment enforces the one rule a signed adjustment
// has: it is visible, and it is explained. A zero adjustment carries no
// reason, so a half-typed explanation cannot outlive the amount it explained.
func normalizePurchaseOrderAdjustment(amount int64, reason string) (int64, string, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > MaxAdjustmentReasonLength {
		return 0, "", fmt.Errorf("%w: adjustment reason must be at most %d characters",
			ErrPurchaseOrderInvalid, MaxAdjustmentReasonLength)
	}
	if amount == 0 {
		return 0, "", nil
	}
	if reason == "" {
		return 0, "", fmt.Errorf("%w: an adjustment requires a reason", ErrPurchaseOrderInvalid)
	}
	return amount, reason, nil
}

// poMulDiv multiplies two int64 values exactly and divides by a positive
// denominator, rounding half away from zero when asked and truncating otherwise.
// The multiplication happens in a big.Int so the intermediate cannot wrap; only
// a result outside int64 is refused.
func poMulDiv(left, right, denominator int64, halfUp bool) (int64, error) {
	if denominator <= 0 {
		return 0, errors.New("non-positive denominator")
	}
	product := new(big.Int).Mul(big.NewInt(left), big.NewInt(right))
	divisor := big.NewInt(denominator)
	quotient, remainder := new(big.Int).QuoRem(product, divisor, new(big.Int))
	if halfUp {
		twiceRemainder := new(big.Int).Abs(remainder)
		twiceRemainder.Lsh(twiceRemainder, 1)
		if twiceRemainder.Cmp(divisor) >= 0 {
			if product.Sign() < 0 {
				quotient.Sub(quotient, big.NewInt(1))
			} else {
				quotient.Add(quotient, big.NewInt(1))
			}
		}
	}
	return poCheckedInt64(quotient, "amount")
}

// poCheckedInt64 refuses a value outside int64 with the stage that produced it
// named, so an overflow reports "line 2 gross" rather than a bare range error.
func poCheckedInt64(value *big.Int, stage string) (int64, error) {
	if !value.IsInt64() {
		return 0, fmt.Errorf("%w: %s exceeds the supported amount range",
			ErrPurchaseOrderInvalid, stage)
	}
	return value.Int64(), nil
}
