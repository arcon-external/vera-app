package backend

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"vera-app/backend/store"
)

// TestCurrencyDefaultsToIDRByteIdentical is the compatibility guarantee for
// #336: the canonical line is unconditional and defaults to IDR, so a
// revision that never set a currency produces byte-identical canonical bytes
// to every revision signed before the field existed. The three pinned format
// tests already assert this with their `currency: IDR` literals; this pins
// the defaulting itself — empty, lowercase and unknown codes alike.
//
// If this ever fails, do NOT bump CanonicalPrefix to "fix" it — that stops
// every code already printed from verifying. Find what started emitting a
// different line instead.
func TestCurrencyDefaultsToIDRByteIdentical(t *testing.T) {
	pinned := string(CanonicalRevision(testSignable()))
	if !strings.Contains(pinned, "\ncurrency: IDR\n") {
		t.Fatalf("pinned canonical carries no unconditional currency line:\n%s", pinned)
	}
	for name, code := range map[string]string{
		"empty":     "",
		"lowercase": "idr",
		"unknown":   "XX",
	} {
		revision := testSignable()
		revision.Currency = code
		if got := string(CanonicalRevision(revision)); got != pinned {
			t.Errorf("%s currency %q changed the canonical bytes:\n got %q\nwant %q",
				name, code, got, pinned)
		}
	}
}

// TestCanonicalCarriesNonDefaultCurrency is the other half: a revision in a
// non-default currency carries it in its signed payload.
func TestCanonicalCarriesNonDefaultCurrency(t *testing.T) {
	revision := testSignable()
	revision.Currency = "USD"
	got := string(CanonicalRevision(revision))
	if !strings.Contains(got, "\ncurrency: USD\n") {
		t.Errorf("a USD revision carries no currency line:\n%s", got)
	}
	if strings.Contains(got, "\ncurrency: IDR\n") {
		t.Errorf("a USD revision still carries the default line:\n%s", got)
	}
	// Only the currency line moves: every other line is the pinned golden.
	pinned := string(CanonicalRevision(testSignable()))
	if strings.Replace(got, "\ncurrency: USD\n", "\ncurrency: IDR\n", 1) != pinned {
		t.Errorf("a USD revision differs beyond its currency line:\n got %q\nwant %q", got, pinned)
	}
}

// TestSignableMappersAgreeOnCurrency extends TestSignableMappersAgree to a
// non-default currency: both mappers must set it identically, or exported
// signatures stop verifying.
func TestSignableMappersAgreeOnCurrency(t *testing.T) {
	data := QuotationData{
		QuoteNumber: "15020005", RevisionID: 1,
		RecipientName: "Jesicha", RecipientCompany: "PT. Infineon",
		RecipientAddress: "Batam", Subject: "Scope", Incoterm: "EXW",
		Validity: 30, PaymentTerm: 90, LeadTimeDays: 60, DiscountPercent: 5,
		CompanyName: "PT Arcon Penta Persada", Currency: "USD",
		SubTotal: 290000, DiscountAmount: 14500, PpnAmount: 30305, GrandTotal: 305805,
		Items: []QuotationItem{
			{Description: "Panel", Qty: 1, UoM: "Lot", UnitPrice: 240000, Total: 240000},
		},
	}
	evidence := store.RevisionEvidence{
		QuoteNumber: "15020005", RevisionNumber: 1,
		RecipientName: "Jesicha", RecipientCompany: "PT. Infineon",
		RecipientAddress: "Batam", Subject: "Scope", Incoterm: "EXW",
		ValidityDays: 30, PaymentTermDays: 90, LeadTimeDays: 60, DiscountPercent: 5,
		IssuerName: "PT Arcon Penta Persada", Currency: "USD",
		Subtotal: 290000, DiscountAmount: 14500, PpnAmount: 30305, GrandTotal: 305805,
		Items: []store.EvidenceItem{
			{Description: "Panel", Qty: 1, UoM: "Lot", UnitPrice: 240000, Total: 240000},
		},
	}
	fromData := string(CanonicalRevision(SignableFromData(data)))
	fromEvidence := string(CanonicalRevision(SignableFromEvidence(evidence)))
	if fromData != fromEvidence {
		t.Errorf("mappers disagree on currency:\n--- exported ---\n%s\n--- stored ---\n%s",
			fromData, fromEvidence)
	}
	if !strings.Contains(fromData, "\ncurrency: USD\n") {
		t.Errorf("neither mapper carried the currency:\n%s", fromData)
	}

	// And a tampered currency fails verification, end to end.
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 11)
	}
	key := ed25519.NewKeyFromSeed(seed)
	signature := SignCanonical(key, CanonicalRevision(SignableFromData(data)))
	tampered := evidence
	tampered.Currency = "EUR"
	if VerifyCanonical(key.Public().(ed25519.PublicKey),
		CanonicalRevision(SignableFromEvidence(tampered)), signature) {
		t.Error("a tampered currency still verified: the field is not covered by the signature")
	}
}
