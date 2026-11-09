package backend

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"vera-app/backend/store"
)

// noteCanonicalGolden is the canonical payload exactly as it was printed and
// signed before the quotation note existed. It is copied from
// TestCanonicalFormatIsPinned rather than recomputed: this test's whole point
// is that an empty note changes nothing, so the expected bytes must come from
// the pre-note format, not from the code under test.
const noteCanonicalGolden = `VERA1:QUOTE
number: 15020005
revision: 00
recipient_name: Jesicha
recipient_company: PT. Infineon Technologies Batam
recipient_address: Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam
subject: Supply and install scrubber control system
incoterm: EXW
validity_days: 30
payment_term_days: 90
lead_time_days: 60
discount_percent: 5
issuer: PT Arcon Penta Persada
currency: IDR
item_count: 2
item_1: Scrubber control panel|2|Lot|120000|240000
item_2: Commissioning|1|Lot|50000|50000
subtotal: 290000
discount_amount: 14500
ppn_amount: 30305
grand_total: 305805`

// TestCanonicalOmitsAnAbsentNote is the compatibility guarantee for the
// quotation note: a revision with no note must produce byte-identical
// canonical bytes to every revision signed before the field existed, because
// every printed verification code out there depends on it.
//
// If this ever fails, do NOT bump CanonicalPrefix to "fix" it -- that stops
// every code already printed from verifying. Find what started emitting the
// field instead.
func TestCanonicalOmitsAnAbsentNote(t *testing.T) {
	revision := testSignable()
	revision.Note = ""
	if got := string(CanonicalRevision(revision)); got != noteCanonicalGolden {
		t.Errorf("an empty note changed the canonical bytes:\n--- got ---\n%s\n--- want ---\n%s",
			got, noteCanonicalGolden)
	}
}

// TestCanonicalCarriesANoteBetweenCurrencyAndItemCount pins the one place the
// note enters the signed payload: after the currency and before the item
// count, as one flattened line. The printed document preserves line breaks
// (#534); the signature covers the commercial text, not its presentation.
func TestCanonicalCarriesANoteBetweenCurrencyAndItemCount(t *testing.T) {
	revision := testSignable()
	revision.Note = "Delivery is ex-works.\r\nInstallation by the customer."
	want := strings.Replace(noteCanonicalGolden,
		"currency: IDR\nitem_count: 2",
		"currency: IDR\nnote: Delivery is ex-works. Installation by the customer.\nitem_count: 2", 1)
	got := string(CanonicalRevision(revision))
	if got != want {
		t.Errorf("a note-bearing revision drifted:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if strings.Contains(got, "\r") {
		t.Error("a carriage return survived into the canonical form")
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "Installation") {
			t.Errorf("the note's line break broke canonical framing: %q", line)
		}
	}
}

// TestNoteBearingRevisionSignsAndVerifies proves the note is covered content:
// a genuine signature verifies, and editing the note breaks it exactly like
// editing a price.
func TestNoteBearingRevisionSignsAndVerifies(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := key.Public().(ed25519.PublicKey)

	revision := testSignable()
	revision.Note = "Please allow two weeks for delivery."
	signature := SignCanonical(key, CanonicalRevision(revision))
	if !VerifyCanonical(publicKey, CanonicalRevision(revision), signature) {
		t.Fatal("a genuine note-bearing revision failed verification")
	}

	tampered := revision
	tampered.Note = "Please allow two months for delivery."
	if VerifyCanonical(publicKey, CanonicalRevision(tampered), signature) {
		t.Error("a tampered note still verified: the note is not covered by the signature")
	}
}

// TestOldPrintedCodeStillVerifiesAfterTheNoteShips signs the literal pre-note
// bytes -- what a code printed before this feature was signed over -- and
// verifies it against the current canonical renderer with no note. This is the
// migration story: old printed codes must keep verifying.
func TestOldPrintedCodeStillVerifiesAfterTheNoteShips(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := key.Public().(ed25519.PublicKey)
	signature := SignCanonical(key, []byte(noteCanonicalGolden))

	revision := testSignable()
	revision.Note = ""
	if !VerifyCanonical(publicKey, CanonicalRevision(revision), signature) {
		t.Error("a code printed before the note existed no longer verifies")
	}
}

// TestSignableMappersAgreeOnNote extends TestSignableMappersAgree to the note:
// the mapper that signs at export and the mapper that recomputes from stored
// evidence must carry it identically, or a note-bearing document reports
// tampering against an untouched revision.
func TestSignableMappersAgreeOnNote(t *testing.T) {
	const note = "Delivery is ex-works.\nInstallation by the customer."
	data := QuotationData{
		QuoteNumber: "15020005", RevisionID: 1,
		RecipientName: "Jesicha", RecipientCompany: "PT. Infineon",
		RecipientAddress: "Batam", Subject: "Scope", Incoterm: "EXW",
		Validity: 30, PaymentTerm: 90, LeadTimeDays: 60, DiscountPercent: 5,
		CompanyName: "PT Arcon Penta Persada", Currency: "IDR", Note: note,
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
		IssuerName: "PT Arcon Penta Persada", Currency: "IDR", Note: note,
		Subtotal: 290000, DiscountAmount: 14500, PpnAmount: 30305, GrandTotal: 305805,
		Items: []store.EvidenceItem{
			{Description: "Panel", Qty: 1, UoM: "Lot", UnitPrice: 240000, Total: 240000},
		},
	}
	fromData := string(CanonicalRevision(SignableFromData(data)))
	fromEvidence := string(CanonicalRevision(SignableFromEvidence(evidence)))
	if fromData != fromEvidence {
		t.Errorf("mappers disagree on the note:\n--- exported ---\n%s\n--- stored ---\n%s",
			fromData, fromEvidence)
	}
	if !strings.Contains(fromData, "\nnote: Delivery is ex-works. Installation by the customer.\n") {
		t.Errorf("neither mapper carried the note:\n%s", fromData)
	}
}
