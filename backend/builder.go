package backend

import (
	"fmt"
	"time"
)

// applyMarkup adds a percentage margin to a unit cost.
//
// The multiply is in uint64 and the division truncates, so a margin of 50 on a
// cost of 333 adds 166 and yields 499 rather than 499.5. The frontend mirrors
// this exactly (see applyMargin in frontend/src/format.ts) because the two must
// not disagree about a number the customer is quoted.
//
// margin is uint16 rather than uint8 because the ceiling is maxMarginPercent, and
// uint8 tops out at 255 — below the limit the app actually allows.
func applyMarkup(cost uint64, margin uint16) uint64 {
	return cost + cost*uint64(margin)/100
}

type PricingSummary struct {
	OriginalSubtotal          uint64
	OriginalDiscount          uint64
	OriginalGrand             uint64
	MarginalizedSubtotal      uint64
	MarginalizedDiscount      uint64
	MarginalizedGrand         uint64
	TaxAmount                 uint64
	MarginalizedGrandAfterTax uint64
	ProfitAmount              int64
	NettProfit                int64
	PpnAmount                 uint64
	CustomerGrandTotal        uint64
}

func buildItems(inputItems []QuotationItemInput) []QuotationItem {
	items := make([]QuotationItem, 0, len(inputItems))
	for key, itemInput := range inputItems {
		item := newQuotationItem(
			itemInput.Description,
			itemInput.Qty,
			itemInput.UoM,
			itemInput.UnitPrice,
		)
		item.MarginEnabled = itemInput.MarginEnabled
		item.MarginPercent = itemInput.MarginPercent
		item.ShowSubPrices = itemInput.ShowSubPrices
		if item.MarginEnabled {
			item.UnitPrice = applyMarkup(item.OriginalUnitPrice, item.MarginPercent)
			item.MarginAmount = item.UnitPrice - item.OriginalUnitPrice
			item.Total = uint64(item.Qty) * item.UnitPrice
			item.DisplayUnitPrice = formatRupiah(item.UnitPrice)
			item.DisplayTotal = formatRupiah(item.Total)
			item.DisplayMarginAmount = formatRupiah(item.MarginAmount)
		}

		item.No = uint32(key + 1)
		if len(itemInput.SubItems) > 0 {
			item.SubItems = buildSubItems(item.No, itemInput.SubItems)
			// The parent is a package rollup: its cost and price are the sums
			// of its parts. Qty 1 by validation, so the totals below carry the
			// package once. MarginAmount keeps its per-unit meaning (sell
			// minus cost of the one package unit); per-sub margins were
			// applied inside buildSubItems, never to the summed cost.
			var costSum, sellSum uint64
			for _, sub := range item.SubItems {
				costSum += sub.OriginalTotal
				sellSum += sub.Total
			}
			item.OriginalUnitPrice = costSum
			item.OriginalTotal = costSum
			item.UnitPrice = sellSum
			item.Total = sellSum
			item.MarginAmount = sellSum - costSum
			item.DisplayOriginalUnitPrice = formatRupiah(costSum)
			item.DisplayOriginalTotal = formatRupiah(costSum)
			item.DisplayUnitPrice = formatRupiah(sellSum)
			item.DisplayTotal = formatRupiah(sellSum)
			item.DisplayMarginAmount = formatRupiah(item.MarginAmount)
		}
		items = append(items, item)
	}

	return items
}

// buildSubItems prices one part of a package on its own, exactly as a
// stand-alone line would be priced: the margin is applied per sub-item unit,
// so truncation happens at the same places it would on a single line, and the
// package total is the exact sum of the priced parts.
//
// Sub-sub-items attach numbering only: they carry no values, so they sum to
// nothing and leave every total untouched.
func buildSubItems(parentNo uint32, subs []QuotationSubItemInput) []QuotationSubItem {
	out := make([]QuotationSubItem, 0, len(subs))
	for i, sub := range subs {
		sellUnit := sub.UnitCost
		if sub.MarginEnabled {
			sellUnit = applyMarkup(sub.UnitCost, sub.MarginPercent)
		}
		item := QuotationSubItem{
			Description:       sub.Description,
			SubNo:             fmt.Sprintf("%d.%d", parentNo, i+1),
			UoM:               sub.UoM,
			SubSubItems:       buildSubSubItems(sub.SubSubItems),
			OriginalUnitPrice: sub.UnitCost,
			OriginalTotal:     uint64(sub.Qty) * sub.UnitCost,
			UnitPrice:         sellUnit,
			Total:             uint64(sub.Qty) * sellUnit,
			Qty:               sub.Qty,
		}
		item.MarginAmount = item.UnitPrice - item.OriginalUnitPrice
		item.DisplayUnitPrice = formatRupiah(item.UnitPrice)
		item.DisplayTotal = formatRupiah(item.Total)
		item.MarginEnabled = sub.MarginEnabled
		item.MarginPercent = sub.MarginPercent
		out = append(out, item)
	}
	return out
}

// buildSubSubItems carries one part's detail lines through. They are plain
// bullets below their sub row -- no numbering, nothing priced.
func buildSubSubItems(lows []QuotationSubSubItemInput) []QuotationSubSubItem {
	out := make([]QuotationSubSubItem, 0, len(lows))
	for _, low := range lows {
		out = append(out, QuotationSubSubItem{Description: low.Description})
	}
	return out
}

func buildDates(validity uint8) (string, string) {
	now := time.Now()
	date := now.Format("January 02, 2006")
	validUntil := now.AddDate(0, 0, int(validity)).Format("January 02, 2006")

	return date, validUntil
}

func buildPrices(items []QuotationItem, discountPercent uint8) (uint64, uint64, uint64) {
	var subTotal uint64
	for _, item := range items {
		subTotal += item.Total
	}

	discountAmount := subTotal * uint64(discountPercent) / 100

	grandTotal := subTotal - discountAmount

	return subTotal, discountAmount, grandTotal
}

func buildPricing(items []QuotationItem, discountPercent uint8, ppnBps, servicesBps uint16) PricingSummary {
	var originalSubtotal uint64
	var marginalizedSubtotal uint64
	for _, item := range items {
		originalSubtotal += item.OriginalTotal
		marginalizedSubtotal += item.Total
	}

	// A discount is a price cut, not a cost cut: what we pay suppliers does not
	// move when the customer is given a lower price. So the cost side carries no
	// discount at all, and original grand equals the cost subtotal.
	//
	// Both sides used to be discounted by the same percentage, which left profit
	// scaled by (1 - discount) instead of losing the discount. See issue #270.
	originalDiscount := uint64(0)
	originalGrand := originalSubtotal
	marginalizedDiscount := marginalizedSubtotal * uint64(discountPercent) / 100
	marginalizedGrand := marginalizedSubtotal - marginalizedDiscount
	// PPN applies to the sell grand after discount (non-FTZ goods only) and
	// is part of the customer document. It is collected on top of the price and
	// passed to the state, so it is never profit: both figures below read the
	// pre-PPN sell grand, which is why PPN cannot leak into them. Services tax
	// stays internal and does come out of nett.
	ppnAmount := calcTax(marginalizedGrand, ppnBps)
	customerGrand := marginalizedGrand + ppnAmount
	servicesTax := calcTax(marginalizedGrand, servicesBps)
	marginalizedGrandAfterTax := marginalizedGrand - servicesTax

	return PricingSummary{
		OriginalSubtotal:          originalSubtotal,
		OriginalDiscount:          originalDiscount,
		OriginalGrand:             originalGrand,
		MarginalizedSubtotal:      marginalizedSubtotal,
		MarginalizedDiscount:      marginalizedDiscount,
		MarginalizedGrand:         marginalizedGrand,
		TaxAmount:                 servicesTax,
		MarginalizedGrandAfterTax: marginalizedGrandAfterTax,
		// Profit and nett both start from the sell grand after discount, never
		// the customer total, so the PPN the customer paid is excluded. They
		// differ only by the services tax that comes out of nett.
		ProfitAmount:       signedDifference(marginalizedGrand, originalGrand),
		NettProfit:         int64(marginalizedGrandAfterTax) - int64(originalGrand),
		PpnAmount:          ppnAmount,
		CustomerGrandTotal: customerGrand,
	}
}

func signedDifference(left, right uint64) int64 {
	if left >= right {
		return int64(left - right)
	}
	return -int64(right - left)
}

func buildViews(subTotal, discountAmount, grandTotal uint64) (string, string, string) {
	return formatRupiah(subTotal), formatRupiah(discountAmount), formatRupiah(grandTotal)
}

func newQuotationItem(desc string, qty uint32, uom string, unitPrice uint64) QuotationItem {
	total := uint64(qty) * unitPrice
	return QuotationItem{
		Description:              desc,
		DisplayDesc:              formatDescription(desc),
		UoM:                      uom,
		DisplayOriginalUnitPrice: formatRupiah(unitPrice),
		DisplayOriginalTotal:     formatRupiah(total),
		DisplayUnitPrice:         formatRupiah(unitPrice),
		DisplayTotal:             formatRupiah(total),
		DisplayMarginAmount:      formatRupiah(0),
		OriginalUnitPrice:        unitPrice,
		OriginalTotal:            total,
		UnitPrice:                unitPrice,
		Total:                    total,
		Qty:                      qty,
	}
}
