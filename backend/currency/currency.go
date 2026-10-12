// Package currency is the shipped catalogue of company currencies.
//
// It is tenant-selected but never tenant-defined: each entry carries its ISO
// 4217 exponent (minor units per major), its home-convention symbol, the
// symbol's position, and its grouping/decimal separators. A tenant cannot typo
// an exponent into malformed money because there is no field to typo one
// into — only a code from this list.
//
// This package imports nothing local, so the store (validation at the write
// boundary), the application (display formatting), the API (portal and detail
// views), the CLI (flag validation) and the native renderer (totals and
// headers) can all import it without an import cycle. Same shape as
// backend/theme for the same reason.
//
// Exponents follow ISO 4217 minor units: IDR, JPY, KRW and VND have exponent
// 0 (no fractional display); every other listed currency has exponent 2.
//
// Separators and positions follow each currency's home convention, not a
// global locale. That is deliberate (issue #336): a USD quotation grouping
// with dots reads as wrong to its customer even when the office sits in
// Indonesia. Where the home convention has a well-known ambiguity the choice
// is documented on the entry:
//
//   - EUR takes the German/EU-core form (1.234,56 €, symbol after with a
//     space). Other Eurozone countries differ; one form has to win.
//   - CNY renders as CN¥, not ¥, so a total cannot be mistaken for JPY.
//   - INR uses plain 3-digit grouping, not the lakh/crore grouping used in
//     India (1,00,000). The formatter groups uniformly; implementing one
//     currency's irregular grouping is not worth the second code path.
//   - AED renders as the Latin code "AED", not the Arabic-script symbol,
//     so totals stay inside the typeface the documents embed.
//   - Symbols after the amount (EUR, VND) are separated by one space;
//     symbols before it sit flush, matching the Rp the documents print today.
package currency

import (
	"fmt"
	"strconv"
	"strings"
)

// DefaultCode is the currency every revision written before this catalogue
// existed carries, and what every normalisation falls back to. The stored
// integer for an IDR amount is whole rupiah, so exponent 0 leaves every
// existing figure byte-identical.
const DefaultCode = "IDR"

// Info is one catalogue entry: everything display and input need.
type Info struct {
	// Code is the ISO 4217 alphabetic code, always three ASCII capitals.
	Code string
	// Exponent is the minor units per major (10^Exponent minor = 1 major).
	Exponent int
	// Symbol is the home-convention mark printed with totals.
	Symbol string
	// SymbolAfter reports whether the symbol follows the amount (EUR, VND).
	// Otherwise it leads, flush against the digits.
	SymbolAfter bool
	// GroupSep separates thousands groups; DecimalSep opens the fraction.
	// The two always differ within an entry.
	GroupSep   string
	DecimalSep string
}

// catalogue is the full shipped set. Codes are the keys; entries repeat
// their own code so a lookup result is self-describing.
var catalogue = map[string]Info{
	"IDR": {Code: "IDR", Exponent: 0, Symbol: "Rp", GroupSep: ".", DecimalSep: ","},
	"USD": {Code: "USD", Exponent: 2, Symbol: "$", GroupSep: ",", DecimalSep: "."},
	"EUR": {Code: "EUR", Exponent: 2, Symbol: "€", SymbolAfter: true, GroupSep: ".", DecimalSep: ","},
	"SGD": {Code: "SGD", Exponent: 2, Symbol: "S$", GroupSep: ",", DecimalSep: "."},
	"MYR": {Code: "MYR", Exponent: 2, Symbol: "RM", GroupSep: ",", DecimalSep: "."},
	"JPY": {Code: "JPY", Exponent: 0, Symbol: "¥", GroupSep: ",", DecimalSep: "."},
	"CNY": {Code: "CNY", Exponent: 2, Symbol: "CN¥", GroupSep: ",", DecimalSep: "."},
	"HKD": {Code: "HKD", Exponent: 2, Symbol: "HK$", GroupSep: ",", DecimalSep: "."},
	"AUD": {Code: "AUD", Exponent: 2, Symbol: "A$", GroupSep: ",", DecimalSep: "."},
	"GBP": {Code: "GBP", Exponent: 2, Symbol: "£", GroupSep: ",", DecimalSep: "."},
	"THB": {Code: "THB", Exponent: 2, Symbol: "฿", GroupSep: ",", DecimalSep: "."},
	"PHP": {Code: "PHP", Exponent: 2, Symbol: "₱", GroupSep: ",", DecimalSep: "."},
	"VND": {Code: "VND", Exponent: 0, Symbol: "₫", SymbolAfter: true, GroupSep: ".", DecimalSep: ","},
	"KRW": {Code: "KRW", Exponent: 0, Symbol: "₩", GroupSep: ",", DecimalSep: "."},
	"INR": {Code: "INR", Exponent: 2, Symbol: "₹", GroupSep: ",", DecimalSep: "."},
	"AED": {Code: "AED", Exponent: 2, Symbol: "AED", GroupSep: ",", DecimalSep: "."},
}

// Codes lists every supported code in sorted order, for error messages and
// for any future picker. The sort keeps the output deterministic.
func Codes() []string {
	out := make([]string, 0, len(catalogue))
	for code := range catalogue {
		out = append(out, code)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Lookup reports the catalogue entry for a code, case-insensitively.
// Unknown codes report false; callers that must never break a document use
// Normalize instead.
func Lookup(code string) (Info, bool) {
	info, ok := catalogue[strings.ToUpper(strings.TrimSpace(code))]
	return info, ok
}

// Normalize resolves any stored or supplied code to the code the document
// uses: upper-cased and trimmed when supported, the default otherwise. This
// is the second fallback (see the theme's fallback): columns are validated
// on write, and this guards every render and signing path against anything
// that slipped through, so an unknown code renders as IDR rather than
// breaking a document.
func Normalize(code string) string {
	if info, ok := Lookup(code); ok {
		return info.Code
	}
	return DefaultCode
}

// pow10 returns 10^exp for the small exponents the catalogue carries.
func pow10(exp int) int64 {
	multiplier := int64(1)
	for i := 0; i < exp; i++ {
		multiplier *= 10
	}
	return multiplier
}

// groupThousands groups a digit string in threes with the separator.
func groupThousands(digits, sep string) string {
	if len(digits) <= 3 {
		return digits
	}
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	return strings.Join(parts, sep)
}

// FormatAmount renders a non-negative minor-unit amount as a bare number —
// no symbol — in the currency's home convention: the item table's cells stay
// bare while the totals carry the symbol, which is the template's existing
// shape. The exponent is applied here: 1500000 minor USD is "15,000.00"
// while 15000000 minor IDR is "15.000.000".
func FormatAmount(code string, minor uint64) string {
	info, ok := Lookup(code)
	if !ok {
		info = catalogue[DefaultCode]
	}
	if info.Exponent == 0 {
		return groupThousands(strconv.FormatUint(minor, 10), info.GroupSep)
	}
	divisor := uint64(pow10(info.Exponent))
	major, fraction := minor/divisor, minor%divisor
	padded := strconv.FormatUint(fraction, 10)
	for len(padded) < info.Exponent {
		padded = "0" + padded
	}
	return groupThousands(strconv.FormatUint(major, 10), info.GroupSep) + info.DecimalSep + padded
}

// FormatSignedAmount renders a signed minor-unit amount the same way.
// Profit and loss are signed; a loss prints with a leading minus.
func FormatSignedAmount(code string, minor int64) string {
	if minor < 0 {
		return "-" + FormatAmount(code, uint64(-minor))
	}
	return FormatAmount(code, uint64(minor))
}

// Affixes returns the symbol's placement around a bare formatted amount:
// a leading symbol sits flush ("$15,000.00", "Rp15.000.000"), a trailing
// one is spaced ("1.234,56 €"). Exactly one of the two is non-empty.
func Affixes(code string) (prefix, suffix string) {
	info, ok := Lookup(code)
	if !ok {
		info = catalogue[DefaultCode]
	}
	if info.SymbolAfter {
		return "", " " + info.Symbol
	}
	return info.Symbol, ""
}

// Decorate wraps a bare formatted amount with its symbol placement: what
// the totals card prints. IDR decorates exactly as the template always has
// ("Rp" + bare), so existing documents are byte-identical.
func Decorate(code, bare string) string {
	prefix, suffix := Affixes(code)
	return prefix + bare + suffix
}

// ParseMinor parses a numeric money text into minor units: the input half
// of the exponent contract. It accepts the currency's grouping separator
// (validated: leading group of 1-3 digits, then threes) and its decimal
// separator, with at most Exponent fraction digits, padded right. Symbols
// are not accepted: inputs hold numbers, never marks.
//
// "15000.00" in USD stores 1500000; "15.000.000" in IDR stores 15000000.
func ParseMinor(code, text string) (int64, error) {
	info, ok := Lookup(code)
	if !ok {
		info = catalogue[DefaultCode]
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf("amount is required")
	}
	negative := false
	if strings.HasPrefix(trimmed, "-") {
		negative = true
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	if trimmed == "" {
		return 0, fmt.Errorf("amount %q is not a number", text)
	}

	intPart, fracPart := trimmed, ""
	if info.Exponent > 0 {
		if i := strings.LastIndex(trimmed, info.DecimalSep); i >= 0 {
			intPart, fracPart = trimmed[:i], trimmed[i+len(info.DecimalSep):]
		}
	} else if strings.Contains(trimmed, info.DecimalSep) && info.DecimalSep != info.GroupSep {
		// An exponent-0 currency has no fraction: a decimal separator in the
		// text is a misplaced grouping from another convention, refused
		// rather than silently truncated.
		return 0, fmt.Errorf("amount %q carries a fraction %s has no minor unit for", text, info.Code)
	}
	if strings.Contains(fracPart, info.GroupSep) || strings.Contains(intPart, info.DecimalSep) {
		return 0, fmt.Errorf("amount %q mixes separators", text)
	}
	if len(fracPart) > info.Exponent {
		return 0, fmt.Errorf("amount %q carries more than %d decimal places", text, info.Exponent)
	}
	for len(fracPart) < info.Exponent {
		fracPart += "0"
	}

	digits := intPart
	if info.GroupSep != "" && strings.Contains(intPart, info.GroupSep) {
		groups := strings.Split(intPart, info.GroupSep)
		if len(groups[0]) < 1 || len(groups[0]) > 3 {
			return 0, fmt.Errorf("amount %q is not grouped in threes", text)
		}
		for _, group := range groups[1:] {
			if len(group) != 3 {
				return 0, fmt.Errorf("amount %q is not grouped in threes", text)
			}
		}
		digits = strings.Join(groups, "")
	}
	if digits == "" {
		return 0, fmt.Errorf("amount %q is not a number", text)
	}
	for _, r := range digits + fracPart {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("amount %q is not a number", text)
		}
	}
	major, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is out of range", text)
	}
	var fraction int64
	if fracPart != "" {
		fraction, err = strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("amount %q is out of range", text)
		}
	}
	minor := major*pow10(info.Exponent) + fraction
	if negative {
		minor = -minor
	}
	return minor, nil
}
