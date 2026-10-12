package currency_test

import (
	"strings"
	"testing"

	"vera-app/backend"
	"vera-app/backend/currency"
)

// The AED defect: a leading code-symbol printed flush ("AED15,000.00")
// misreads as a single token, so AED leads with one space.
func TestSpacedLeadingSymbol(t *testing.T) {
	if got := currency.Decorate("AED", "15,000.00"); got != "AED 15,000.00" {
		t.Errorf("Decorate(AED) = %q; want %q", got, "AED 15,000.00")
	}
	prefix, suffix := currency.Affixes("AED")
	if prefix != "AED " || suffix != "" {
		t.Errorf("Affixes(AED) = %q/%q; want %q/%q", prefix, suffix, "AED ", "")
	}
}

// Trailing symbols stay spaced, exactly as the catalogue has always
// rendered them.
func TestSpacedTrailingSymbols(t *testing.T) {
	cases := []struct{ code, bare, want string }{
		{"EUR", "1.234,56", "1.234,56 €"},
		{"VND", "15.000", "15.000 ₫"},
	}
	for _, tc := range cases {
		if got := currency.Decorate(tc.code, tc.bare); got != tc.want {
			t.Errorf("Decorate(%s) = %q; want %q", tc.code, got, tc.want)
		}
	}
}

// Every other entry sits flush: no space anywhere in the decorated total,
// matching the Rp the documents print today.
func TestUnspacedSymbolsSitFlush(t *testing.T) {
	spaced := map[string]bool{"AED": true, "EUR": true, "VND": true}
	for _, code := range currency.Codes() {
		if spaced[code] {
			continue
		}
		if got := currency.Decorate(code, "1"); strings.Contains(got, " ") {
			t.Errorf("Decorate(%s) = %q; want no space", code, got)
		}
	}
	if got := currency.Decorate("USD", "15,000.00"); got != "$15,000.00" {
		t.Errorf("Decorate(USD) = %q; want %q", got, "$15,000.00")
	}
	if got := currency.Decorate("IDR", "15.000.000"); got != "Rp15.000.000" {
		t.Errorf("Decorate(IDR) = %q; want %q", got, "Rp15.000.000")
	}
}

// The bridge shape carries all seven fields per entry, in Codes() order,
// and agrees with the catalogue lookups field for field.
func TestCatalogueBridgeShape(t *testing.T) {
	assertCatalogue(t, currency.Catalogue())
}

// The bound method serves the same entries Round 2 consumes: all 16, all
// seven fields, with no server round-trip (it reads the compiled-in
// catalogue, so it works with a bare AdminAPI).
func TestGetCurrencyCatalogueBoundMethod(t *testing.T) {
	assertCatalogue(t, backend.NewAdminAPI().GetCurrencyCatalogue())
}

func assertCatalogue(t *testing.T, entries []currency.CurrencyInfo) {
	t.Helper()
	codes := currency.Codes()
	if len(entries) != len(codes) {
		t.Fatalf("catalogue holds %d entries; want %d", len(entries), len(codes))
	}
	wantSpaced := map[string]bool{"AED": true, "EUR": true, "VND": true}
	for i, entry := range entries {
		if entry.Code != codes[i] {
			t.Errorf("entry %d code = %q; want Codes() order %q", i, entry.Code, codes[i])
		}
		info, ok := currency.Lookup(entry.Code)
		if !ok {
			t.Errorf("entry %d code %q has no catalogue lookup", i, entry.Code)
			continue
		}
		if entry.Symbol == "" || entry.GroupSep == "" || entry.DecimalSep == "" {
			t.Errorf("entry %s carries an empty symbol/separator: %+v", entry.Code, entry)
		}
		if entry.Exponent != info.Exponent {
			t.Errorf("entry %s exponent = %d; want %d", entry.Code, entry.Exponent, info.Exponent)
		}
		if entry.Symbol != info.Symbol {
			t.Errorf("entry %s symbol = %q; want %q", entry.Code, entry.Symbol, info.Symbol)
		}
		if entry.GroupSep != info.GroupSep || entry.DecimalSep != info.DecimalSep {
			t.Errorf("entry %s separators = %q/%q; want %q/%q",
				entry.Code, entry.GroupSep, entry.DecimalSep, info.GroupSep, info.DecimalSep)
		}
		wantPosition := "before"
		if info.SymbolAfter {
			wantPosition = "after"
		}
		if entry.Position != wantPosition {
			t.Errorf("entry %s position = %q; want %q", entry.Code, entry.Position, wantPosition)
		}
		if entry.Position != "before" && entry.Position != "after" {
			t.Errorf("entry %s position = %q; want before|after", entry.Code, entry.Position)
		}
		if entry.SymbolSpaced != wantSpaced[entry.Code] {
			t.Errorf("entry %s symbolSpaced = %v; want %v", entry.Code, entry.SymbolSpaced, wantSpaced[entry.Code])
		}
	}
}
