package backend

import "time"

func buildItems(inputItems []QuotationItemInput) []QuotationItem {
	items := make([]QuotationItem, 0, len(inputItems))
	for key, itemInput := range inputItems {
		items = append(items, NewQuotationItem(
			itemInput.Description,
			itemInput.Qty,
			itemInput.UoM,
			itemInput.UnitPrice,
		))

		items[key].No = uint32(key + 1)
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

func buildViews(subTotal, discountAmount, grandTotal uint64) (string, string, string) {
	return formatRupiah(subTotal), formatRupiah(discountAmount), formatRupiah(grandTotal)
}

func NewQuotationItem(desc string, qty uint32, uom string, unitPrice uint64) QuotationItem {
	total := uint64(qty) * unitPrice
	return QuotationItem{
		Description:      desc,
		DisplayDesc:      formatDescription(desc),
		UoM:              uom,
		DisplayUnitPrice: formatRupiah(unitPrice),
		DisplayTotal:     formatRupiah(total),
		UnitPrice:        unitPrice,
		Total:            total,
		Qty:              qty,
	}
}
