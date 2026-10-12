package currency

import (
	"testing"
)

// The catalogue is the validation boundary: every listed code must carry all
// six fields, and the two separators must differ within an entry. A partial
// entry formats numbers with an empty separator, which reads as a different
// number to whoever is quoted.
func TestCatalogueIsComplete(t *testing.T) {
	want := []string{"IDR", "USD", "EUR", "SGD", "MYR", "JPY", "CNY", "HKD",
		"AUD", "GBP", "THB", "PHP", "VND", "KRW", "INR", "AED"}
	if got := Codes(); len(got) != len(want) {
		t.Fatalf("codes = %v; want %d entries %v", got, len(want), want)
	}
	for _, code := range want {
		info, ok := Lookup(code)
		if !ok {
			t.Errorf("code %s is listed but has no catalogue entry", code)
			continue
		}
		if info.Code != code {
			t.Errorf("entry code = %q; want %q", info.Code, code)
		}
		if info.Symbol == "" {
			t.Errorf("code %s carries no symbol", code)
		}
		if info.GroupSep == "" || info.DecimalSep == "" {
			t.Errorf("code %s carries separators %q/%q; want both set",
				code, info.GroupSep, info.DecimalSep)
		}
		if info.GroupSep == info.DecimalSep {
			t.Errorf("code %s carries identical separators %q; they must differ",
				code, info.GroupSep)
		}
		if info.Exponent < 0 {
			t.Errorf("code %s carries exponent %d; want >= 0", code, info.Exponent)
		}
	}
}

// The exponents are ISO 4217 minor units, verified rather than assumed: only
// IDR, JPY, KRW and VND carry 0; the rest carry 2. A wrong exponent stores a
// factor of a hundred into every figure.
func TestCatalogueExponentsMatchISO4217(t *testing.T) {
	for code, want := range map[string]int{
		"IDR": 0, "JPY": 0, "KRW": 0, "VND": 0,
		"USD": 2, "EUR": 2, "SGD": 2, "MYR": 2, "CNY": 2, "HKD": 2,
		"AUD": 2, "GBP": 2, "THB": 2, "PHP": 2, "INR": 2, "AED": 2,
	} {
		info, ok := Lookup(code)
		if !ok {
			t.Errorf("code %s has no entry", code)
			continue
		}
		if info.Exponent != want {
			t.Errorf("code %s exponent = %d; want ISO 4217 minor units %d",
				code, info.Exponent, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	for input, want := range map[string]string{
		"":       "IDR",
		"IDR":    "IDR",
		"idr":    "IDR",
		" usd ":  "USD",
		"Usd":    "USD",
		"XX":     "IDR",
		"Rupiah": "IDR",
	} {
		if got := Normalize(input); got != want {
			t.Errorf("Normalize(%q) = %q; want %q", input, got, want)
		}
	}
}

// The issue's own example, both from integers: the exponent is applied at
// display, so the same stored integer reads differently per currency.
func TestFormatFromMinorUnits(t *testing.T) {
	if got := FormatAmount("USD", 1500000); got != "15,000.00" {
		t.Errorf("USD 1500000 = %q; want %q", got, "15,000.00")
	}
	if got := FormatAmount("IDR", 15000000); got != "15.000.000" {
		t.Errorf("IDR 15000000 = %q; want %q", got, "15.000.000")
	}
}

func TestFormatAmount(t *testing.T) {
	cases := []struct {
		code  string
		minor uint64
		want  string
	}{
		{"IDR", 0, "0"},
		{"IDR", 999, "999"},
		{"IDR", 1000, "1.000"},
		{"IDR", 115725000, "115.725.000"},
		{"USD", 0, "0.00"},
		{"USD", 1, "0.01"},
		{"USD", 1500000, "15,000.00"},
		{"USD", 100, "1.00"},
		{"EUR", 123456, "1.234,56"},
		{"JPY", 1500, "1,500"},
		{"KRW", 1000000, "1,000,000"},
		{"VND", 15000, "15.000"},
		{"GBP", 199, "1.99"},
		// Unknown codes render as the default rather than breaking a document.
		{"XX", 15000000, "15.000.000"},
		{"", 1000, "1.000"},
	}
	for _, tc := range cases {
		if got := FormatAmount(tc.code, tc.minor); got != tc.want {
			t.Errorf("FormatAmount(%q, %d) = %q; want %q",
				tc.code, tc.minor, got, tc.want)
		}
	}
}

func TestFormatSignedAmount(t *testing.T) {
	if got := FormatSignedAmount("USD", -150000); got != "-1,500.00" {
		t.Errorf("signed USD = %q; want %q", got, "-1,500.00")
	}
	if got := FormatSignedAmount("IDR", -2500000); got != "-2.500.000" {
		t.Errorf("signed IDR = %q; want %q", got, "-2.500.000")
	}
	if got := FormatSignedAmount("IDR", 0); got != "0" {
		t.Errorf("signed IDR zero = %q; want %q", got, "0")
	}
}

// Symbol placement: leading symbols sit flush (as Rp always has), trailing
// ones are spaced. IDR decorates exactly as the template always has.
func TestDecorate(t *testing.T) {
	cases := []struct {
		code string
		bare string
		want string
	}{
		{"IDR", "15.000.000", "Rp15.000.000"},
		{"USD", "15,000.00", "$15,000.00"},
		{"EUR", "1.234,56", "1.234,56 €"},
		{"JPY", "1,500", "¥1,500"},
		{"VND", "15.000", "15.000 ₫"},
		{"XX", "1.000", "Rp1.000"},
	}
	for _, tc := range cases {
		if got := Decorate(tc.code, tc.bare); got != tc.want {
			t.Errorf("Decorate(%q, %q) = %q; want %q",
				tc.code, tc.bare, got, tc.want)
		}
	}
	for _, code := range Codes() {
		prefix, suffix := Affixes(code)
		if (prefix == "") == (suffix == "") {
			t.Errorf("code %s affixes = %q/%q; want exactly one set",
				code, prefix, suffix)
		}
	}
}

// The exponent round-trips: "15000.00" in stores 1500000 and prints
// "$15,000.00". Parse is the input half of the display contract.
func TestParseMinorRoundTrip(t *testing.T) {
	minor, err := ParseMinor("USD", "15000.00")
	if err != nil {
		t.Fatalf("parse USD 15000.00: %v", err)
	}
	if minor != 1500000 {
		t.Fatalf("parse USD 15000.00 = %d; want 1500000", minor)
	}
	if got := Decorate("USD", FormatAmount("USD", uint64(minor))); got != "$15,000.00" {
		t.Errorf("round trip = %q; want %q", got, "$15,000.00")
	}
}

func TestParseMinor(t *testing.T) {
	cases := []struct {
		code string
		text string
		want int64
	}{
		{"USD", "15000.00", 1500000},
		{"USD", "15,000.00", 1500000},
		{"USD", "15,000", 1500000},
		{"USD", "0.01", 1},
		{"USD", "1.5", 150},
		{"USD", "-1,500.00", -150000},
		{"IDR", "15.000.000", 15000000},
		{"IDR", "15000000", 15000000},
		{"IDR", "0", 0},
		{"EUR", "1.234,56", 123456},
		{"JPY", "1,500", 1500},
		{"VND", "15.000", 15000},
	}
	for _, tc := range cases {
		got, err := ParseMinor(tc.code, tc.text)
		if err != nil {
			t.Errorf("ParseMinor(%q, %q) failed: %v", tc.code, tc.text, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseMinor(%q, %q) = %d; want %d",
				tc.code, tc.text, got, tc.want)
		}
	}
}

func TestParseMinorRefuses(t *testing.T) {
	for _, tc := range []struct{ code, text string }{
		{"USD", ""},
		{"USD", "abc"},
		{"USD", "15,00.00"},
		{"USD", "1,50,00.00"},
		{"USD", "15.000,00"},
		{"USD", "15.000"},
		{"USD", "1.005"},
		{"IDR", "15,5"},
		{"IDR", "15.000,5"},
		{"JPY", "1500.5"},
		{"EUR", "1,234.56"},
	} {
		if got, err := ParseMinor(tc.code, tc.text); err == nil {
			t.Errorf("ParseMinor(%q, %q) = %d; want an error", tc.code, tc.text, got)
		}
	}
}

// Format and Parse are inverses across the catalogue: every entry formats a
// stored integer and parses it back to the same integer.
func TestFormatParseInverse(t *testing.T) {
	for _, code := range Codes() {
		info, _ := Lookup(code)
		minor := uint64(1500000)
		if info.Exponent == 0 {
			minor = 15000000
		}
		formatted := FormatAmount(code, minor)
		back, err := ParseMinor(code, formatted)
		if err != nil {
			t.Errorf("code %s: parse(%q) failed: %v", code, formatted, err)
			continue
		}
		if back != int64(minor) {
			t.Errorf("code %s: parse(format(%d)) = %d", code, minor, back)
		}
	}
}

// The separators are load-bearing for Parse: a formatter that grouped with
// the decimal separator could never round-trip. Pinned per entry.
func TestSeparatorsDiffer(t *testing.T) {
	for _, code := range Codes() {
		info, _ := Lookup(code)
		if info.GroupSep == info.DecimalSep {
			t.Errorf("code %s: separators both %q", code, info.GroupSep)
		}
	}
}
