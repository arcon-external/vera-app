package api

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var amountPrinter = message.NewPrinter(language.Indonesian)

// formatAmount renders a non-negative IDR amount with Indonesian grouping.
func formatAmount(value int64) string {
	if value < 0 {
		value = 0
	}
	return amountPrinter.Sprintf("%d", value)
}

// formatSignedAmount renders a signed IDR amount with Indonesian grouping.
func formatSignedAmount(value int64) string {
	return amountPrinter.Sprintf("%d", value)
}
