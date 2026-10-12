package api

import (
	"vera-app/backend/currency"
)

// formatMoney renders a non-negative minor-unit amount as a bare number in
// the given currency's home convention: the item table's cells stay bare
// while the totals carry the symbol. Negatives clamp to zero, which is the
// long-standing display rule for these archive views (a margin below cost
// reads as no margin here; the signed views carry the loss).
func formatMoney(code string, value int64) string {
	if value < 0 {
		value = 0
	}
	return currency.FormatAmount(code, uint64(value))
}

// formatSignedMoney renders a signed minor-unit amount the same way: a loss
// prints with a leading minus.
func formatSignedMoney(code string, value int64) string {
	return currency.FormatSignedAmount(code, value)
}

// moneyWithSymbol wraps a bare amount with its symbol placement: what the
// portal prints for a total. For IDR this is "Rp" + the bare figure, exactly
// as before.
func moneyWithSymbol(code string, value int64) string {
	return currency.Decorate(code, formatMoney(code, value))
}
