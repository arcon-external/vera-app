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

// packageDiscountPercent is the effective discount rate across a package's
// parts: the summed discount over the summed list price the parts would have
// carried before their discounts. Integer arithmetic, multiply before divide,
// truncating -- the same rule applyMarkup and applyDiscount follow -- with a
// guard for a zero list sum so a package with no list price cannot divide by
// zero. The result cannot exceed 100: no part sells above its list price, so
// the summed discount never exceeds the summed list.
func packageDiscountPercent(listSum, discountSum uint64) uint16 {
	if listSum == 0 {
		return 0
	}
	return uint16(discountSum * 100 / listSum)
}

// applyDiscount takes a percentage off an already-marginalised price.
//
// The order is margin first, then discount, and that order is the point: the
// marginalised figure is the list price the customer would otherwise pay, and
// the discount comes off it. Reversing the two would mean the printed list
// price no longer matched the figure staff see in the editor, and a quotation
// already issued would be quoted differently from how it was built.
//
// Truncates for the same reason applyMarkup does, and for the same reason the
// frontend mirrors it: a discount of 33 on 100 is 33 off, not 33.33.
func applyDiscount(price uint64, discount uint16) uint64 {
	return price - price*uint64(discount)/100
}

type PricingSummary struct {
	OriginalSubtotal          uint64
	OriginalDiscount          uint64
	OriginalGrand             uint64
	MarginalizedSubtotal      uint64
	MarginalizedDiscount      uint64
	RowDiscountTotal          uint64
	MarginalizedGrand         uint64
	TaxAmount                 uint64
	MarginalizedGrandAfterTax uint64
	ProfitAmount              int64
	NettProfit                int64
	PpnAmount                 uint64
	CustomerGrandTotal        uint64
}

func buildItems(inputItems []QuotationItemInput, currencyCode string) []QuotationItem {
	items := make([]QuotationItem, 0, len(inputItems))
	for key, itemInput := range inputItems {
		item := newQuotationItem(
			itemInput.Description,
			itemInput.Qty,
			itemInput.UoM,
			itemInput.UnitPrice,
			currencyCode,
		)
		item.ShowSubPrices = itemInput.ShowSubPrices
		// Margin first, then the row's own discount. newQuotationItem seeded
		// the cost side; priceLine derives everything the sell side needs.
		if itemInput.MarginEnabled || itemInput.DiscountEnabled {
			priced := priceLine(item.OriginalUnitPrice, item.Qty,
				itemInput.MarginPercent, itemInput.MarginEnabled,
				itemInput.DiscountPercent, itemInput.DiscountEnabled,
				currencyCode)
			item.ListUnitPrice = priced.ListUnitPrice
			item.ListTotal = priced.ListTotal
			item.UnitPrice = priced.UnitPrice
			item.Total = priced.Total
			item.DiscountAmount = priced.DiscountAmount
			item.DisplayUnitPrice = priced.DisplayUnitPrice
			item.DisplayTotal = priced.DisplayTotal
			item.DisplayListUnitPrice = priced.DisplayListUnitPrice
			item.DisplayDiscount = priced.DisplayDiscount
			item.DisplayMarginAmount = priced.DisplayMarginAmount
			item.MarginAmount = priced.MarginAmount
			item.MarginEnabled = priced.MarginEnabled
			item.MarginPercent = priced.MarginPercent
			item.DiscountEnabled = priced.DiscountEnabled
			item.DiscountPercent = priced.DiscountPercent
		} else {
			item.MarginEnabled = false
			item.MarginPercent = itemInput.MarginPercent
		}

		item.No = uint32(key + 1)
		if len(itemInput.SubItems) > 0 {
			item.SubItems = buildSubItems(item.No, itemInput.SubItems, currencyCode)
			// The parent is a package rollup: its cost and price are the sums
			// of its parts. Qty 1 by validation, so the totals below carry the
			// package once. MarginAmount keeps its per-unit meaning (sell
			// minus cost of the one package unit); per-sub margins were
			// applied inside buildSubItems, never to the summed cost.
			var costSum, sellSum, listSum, discountSum uint64
			for _, sub := range item.SubItems {
				costSum += sub.OriginalTotal
				sellSum += sub.Total
				listSum += sub.ListUnitPrice * uint64(sub.Qty)
				discountSum += sub.DiscountAmount
			}
			item.OriginalUnitPrice = costSum
			item.OriginalTotal = costSum
			item.ListUnitPrice = listSum
			item.ListTotal = listSum
			item.UnitPrice = sellSum
			item.Total = sellSum
			item.DiscountAmount = discountSum
			item.DiscountEnabled = discountSum > 0
			// The package's own percentage is derived from its parts, the same
			// way its amount is: the summed discount over the summed list price
			// the parts would have carried before their discounts. Integer
			// arithmetic, multiply before divide, truncating -- the same rule
			// applyMarkup and applyDiscount follow -- with a guard for a zero
			// list sum so a package with no list price cannot divide by zero.
			// Without this the flag says "discounted" while the percentage
			// keeps its zero value, and the parent row prints 0% beside a
			// visibly reduced price (issue #383). Zeroed explicitly when there
			// is no sub-discount so a stale parent input cannot linger: the
			// parent's own discount fields are ignored once it has sub-items,
			// and DiscountEnabled above is already reset the same way.
			//
			// MarginPercent is deliberately not derived here. It is
			// internal-only and never printed, and on a plain line it means
			// "the rate that was applied" -- a package has no single applied
			// rate, and its effective margin already has a derived home with
			// its own rounding (blendedMarginPercent, half-up with an
			// approximate flag). A truncated effective rate in this field
			// would disagree with that display while no consumer reads it.
			item.DiscountPercent = packageDiscountPercent(listSum, discountSum)
			item.MarginAmount = signedDifference(sellSum, costSum)
			item.DisplayOriginalUnitPrice = formatMoney(currencyCode, costSum)
			item.DisplayOriginalTotal = formatMoney(currencyCode, costSum)
			item.DisplayUnitPrice = formatMoney(currencyCode, sellSum)
			item.DisplayTotal = formatMoney(currencyCode, sellSum)
			item.DisplayListUnitPrice = formatMoney(currencyCode, listSum)
			item.DisplayDiscount = formatMoney(currencyCode, discountSum)
			item.DisplayMarginAmount = formatSignedMoney(currencyCode, item.MarginAmount)
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
func buildSubItems(parentNo uint32, subs []QuotationSubItemInput, currencyCode string) []QuotationSubItem {
	out := make([]QuotationSubItem, 0, len(subs))
	for i, sub := range subs {
		listUnit := sub.UnitCost
		if sub.MarginEnabled {
			listUnit = applyMarkup(sub.UnitCost, sub.MarginPercent)
		}
		sellUnit := listUnit
		if sub.DiscountEnabled {
			sellUnit = applyDiscount(listUnit, sub.DiscountPercent)
		}
		qty := uint64(sub.Qty)
		item := QuotationSubItem{
			Description:       sub.Description,
			SubNo:             fmt.Sprintf("%d.%d", parentNo, i+1),
			UoM:               sub.UoM,
			SubSubItems:       buildSubSubItems(sub.SubSubItems),
			OriginalUnitPrice: sub.UnitCost,
			OriginalTotal:     qty * sub.UnitCost,
			ListUnitPrice:     listUnit,
			UnitPrice:         sellUnit,
			Total:             qty * sellUnit,
			DiscountAmount:    qty * (listUnit - sellUnit),
			Qty:               sub.Qty,
		}
		item.MarginAmount = signedDifference(sellUnit, sub.UnitCost)
		item.DisplayUnitPrice = formatMoney(currencyCode, sellUnit)
		item.DisplayTotal = formatMoney(currencyCode, qty*sellUnit)
		item.DisplayListPrice = formatMoney(currencyCode, listUnit)
		item.DisplayDiscount = formatMoney(currencyCode, qty*(listUnit-sellUnit))
		item.DisplayMarginAmount = formatSignedMoney(currencyCode, item.MarginAmount)
		item.MarginEnabled = sub.MarginEnabled
		item.MarginPercent = sub.MarginPercent
		item.DiscountEnabled = sub.DiscountEnabled
		item.DiscountPercent = sub.DiscountPercent
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
	var rowDiscountTotal uint64
	for _, item := range items {
		originalSubtotal += item.OriginalTotal
		marginalizedSubtotal += item.Total
		// The row-discount total is leaf-aware, and the reason is double
		// counting. A package -- any item carrying sub-items, whether its parts
		// are priced (state C) or shown as unbilled scope (state B) -- already
		// holds the rollup of its parts' discounts in its own DiscountAmount:
		// buildItems set it to that sum. Each priced sub-item then carries the
		// same amount again as its own leaf discount. Summing a package's own
		// DiscountAmount AND its parts' would count the package twice, so a
		// package contributes its parts' discounts and its parent rollup is
		// never added. A plain row has no parts: its own DiscountAmount is the
		// leaf, so it is added directly.
		if len(item.SubItems) > 0 {
			for _, sub := range item.SubItems {
				rowDiscountTotal += sub.DiscountAmount
			}
		} else {
			rowDiscountTotal += item.DiscountAmount
		}
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
		RowDiscountTotal:          rowDiscountTotal,
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

func buildViews(subTotal, discountAmount, grandTotal uint64, currencyCode string) (string, string, string) {
	return formatMoney(currencyCode, subTotal), formatMoney(currencyCode, discountAmount), formatMoney(currencyCode, grandTotal)
}

// newQuotationItem seeds an unpriced line: cost and sell are the same figure,
// and the list price starts equal to the sell price so an undiscounted line
// reads identically to a line whose discount happens to be zero.
func newQuotationItem(desc string, qty uint32, uom string, unitPrice uint64, currencyCode string) QuotationItem {
	total := uint64(qty) * unitPrice
	return QuotationItem{
		Description:              desc,
		DisplayDesc:              formatDescription(desc),
		UoM:                      uom,
		DisplayOriginalUnitPrice: formatMoney(currencyCode, unitPrice),
		DisplayOriginalTotal:     formatMoney(currencyCode, total),
		DisplayUnitPrice:         formatMoney(currencyCode, unitPrice),
		DisplayTotal:             formatMoney(currencyCode, total),
		DisplayMarginAmount:      formatSignedMoney(currencyCode, 0),
		DisplayListUnitPrice:     formatMoney(currencyCode, unitPrice),
		DisplayDiscount:          formatMoney(currencyCode, 0),
		OriginalUnitPrice:        unitPrice,
		OriginalTotal:            total,
		UnitPrice:                unitPrice,
		Total:                    total,
		ListUnitPrice:            unitPrice,
		ListTotal:                total,
		Qty:                      qty,
	}
}

// priceLine applies a line's margin then its discount, in that order, and
// records every figure the editor, the customer document and the internal
// breakdown each need.
//
// The cost side never moves: what we pay suppliers does not change when the
// customer is given a lower price. That is the same rule the whole-quotation
// discount follows (issue #270), applied per row here.
func priceLine(costUnit uint64, qty uint32, margin uint16, marginOn bool,
	discount uint16, discountOn bool, currencyCode string) QuotationItem {
	sellUnit := costUnit
	if marginOn {
		sellUnit = applyMarkup(costUnit, margin)
	}
	finalUnit := sellUnit
	if discountOn {
		finalUnit = applyDiscount(sellUnit, discount)
	}
	return QuotationItem{
		ListUnitPrice: sellUnit,
		ListTotal:     uint64(qty) * sellUnit,
		UnitPrice:     finalUnit,
		Total:         uint64(qty) * finalUnit,
		DiscountAmount: (uint64(qty) * sellUnit) -
			(uint64(qty) * finalUnit),
		DisplayUnitPrice:     formatMoney(currencyCode, finalUnit),
		DisplayTotal:         formatMoney(currencyCode, uint64(qty)*finalUnit),
		DisplayListUnitPrice: formatMoney(currencyCode, sellUnit),
		DisplayDiscount:      formatMoney(currencyCode, (uint64(qty)*sellUnit)-(uint64(qty)*finalUnit)),
		// Per unit, like every other line: a plain line is sell-unit minus
		// cost-unit, and a package is pinned to qty 1 so the two agree. Going
		// through signedDifference is what lets a discount below cost show a
		// loss instead of wrapping around as a huge uint64 gain.
		MarginAmount:        signedDifference(finalUnit, costUnit),
		DisplayMarginAmount: formatSignedMoney(currencyCode, signedDifference(finalUnit, costUnit)),
		DiscountEnabled:     discountOn,
		DiscountPercent:     discount,
		MarginEnabled:       marginOn,
		MarginPercent:       margin,
	}
}
