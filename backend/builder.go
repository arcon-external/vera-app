package backend

import "time"

func applyMarkup(cost uint64, margin uint8) uint64 {
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
		if item.MarginEnabled {
			item.UnitPrice = applyMarkup(item.OriginalUnitPrice, item.MarginPercent)
			item.MarginAmount = item.UnitPrice - item.OriginalUnitPrice
			item.Total = uint64(item.Qty) * item.UnitPrice
			item.DisplayUnitPrice = formatRupiah(item.UnitPrice)
			item.DisplayTotal = formatRupiah(item.Total)
			item.DisplayMarginAmount = formatRupiah(item.MarginAmount)
		}

		item.No = uint32(key + 1)
		items = append(items, item)
	}

	return items
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

	originalDiscount := originalSubtotal * uint64(discountPercent) / 100
	originalGrand := originalSubtotal - originalDiscount
	marginalizedDiscount := marginalizedSubtotal * uint64(discountPercent) / 100
	marginalizedGrand := marginalizedSubtotal - marginalizedDiscount
	// PPN applies to the sell grand after discount (non-FTZ goods only) and
	// is part of the customer document. Services tax stays internal.
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
		ProfitAmount:              signedDifference(marginalizedGrand, originalGrand),
		NettProfit:                int64(customerGrand) - int64(originalGrand) - int64(servicesTax),
		PpnAmount:                 ppnAmount,
		CustomerGrandTotal:        customerGrand,
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
