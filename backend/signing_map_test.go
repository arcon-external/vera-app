package backend

import (
	"crypto/ed25519"
	"testing"

	"vera-app/backend/store"
)

// TestSignableMappersAgree is the load-bearing invariant of the whole
// verification scheme: the bytes signed at export must equal the bytes
// recomputed from stored rows. If these ever drift, every printed code
// reports tampering against an untouched document.
func TestSignableMappersAgree(t *testing.T) {
	data := QuotationData{
		QuoteNumber:      "15020005",
		RevisionID:       1,
		RecipientName:    "Jesicha",
		RecipientCompany: "PT. Infineon Technologies Batam",
		RecipientAddress: "Batamindo Industrial Park, Lot 317",
		Subject:          "Supply and install scrubber control system",
		Incoterm:         "EXW",
		Validity:         30,
		PaymentTerm:      90,
		LeadTime:         60,
		DiscountPercent:  5,
		CompanyName:      "PT Arcon Penta Persada",
		SubTotal:         290000,
		DiscountAmount:   14500,
		PpnAmount:        30305,
		GrandTotal:       305805,
		Items: []QuotationItem{
			{Description: "Scrubber panel", Qty: 2, UoM: "Lot", UnitPrice: 120000, Total: 240000},
			{Description: "Commissioning", Qty: 1, UoM: "Lot", UnitPrice: 50000, Total: 50000},
		},
	}
	evidence := store.RevisionEvidence{
		QuoteNumber:      "15020005",
		RevisionNumber:   1,
		RecipientName:    "Jesicha",
		RecipientCompany: "PT. Infineon Technologies Batam",
		RecipientAddress: "Batamindo Industrial Park, Lot 317",
		Subject:          "Supply and install scrubber control system",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  90,
		LeadTimeDays:     60,
		DiscountPercent:  5,
		IssuerName:       "PT Arcon Penta Persada",
		Subtotal:         290000,
		DiscountAmount:   14500,
		PpnAmount:        30305,
		GrandTotal:       305805,
		Items: []store.EvidenceItem{
			{Description: "Scrubber panel", Qty: 2, UoM: "Lot", UnitPrice: 120000, Total: 240000},
			{Description: "Commissioning", Qty: 1, UoM: "Lot", UnitPrice: 50000, Total: 50000},
		},
	}

	fromData := CanonicalRevision(SignableFromData(data))
	fromEvidence := CanonicalRevision(SignableFromEvidence(evidence))
	if string(fromData) != string(fromEvidence) {
		t.Errorf("mappers disagree:\n--- exported ---\n%s\n--- stored ---\n%s", fromData, fromEvidence)
	}
}

// TestUnsignedLegacyEvidenceStillVerifies proves the documented migration
// story: a revision stored before signing existed (empty signature) must
// self-heal to a signature that verifies against the same evidence.
func TestUnsignedLegacyEvidenceStillVerifies(t *testing.T) {
	key := testPrivateKey()
	signer := NewSigner(key)
	evidence := store.RevisionEvidence{
		QuoteNumber:      "15029999",
		RevisionNumber:   0,
		RecipientName:    "Legacy Recipient",
		RecipientCompany: "PT Legacy",
		RecipientAddress: "Legacy Street",
		Subject:          "Legacy quotation",
		Incoterm:         "FOB",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     14,
		DiscountPercent:  0,
		IssuerName:       "PT Arcon Penta Persada",
		Subtotal:         100000,
		DiscountAmount:   0,
		PpnAmount:        0,
		GrandTotal:       100000,
		Items: []store.EvidenceItem{
			{Description: "Legacy item", Qty: 1, UoM: "Unit", UnitPrice: 100000, Total: 100000},
		},
	}

	signature, digest := signer.SignRevision(SignableFromEvidence(evidence))
	if signature == "" || digest == "" {
		t.Fatal("self-heal produced an empty signature")
	}
	if !VerifyRevision(signer.publicKey, SignableFromEvidence(evidence), signature) {
		t.Error("self-healed signature does not verify against its own evidence")
	}
	if digest != CanonicalDigest(CanonicalRevision(SignableFromEvidence(evidence))) {
		t.Error("self-healed digest does not match the canonical bytes")
	}
}

// TestIssuerRenameDoesNotBreakOldSignatures documents the reason issuer_name
// is stored per revision rather than read from the live company profile.
func TestIssuerRenameDoesNotBreakOldSignatures(t *testing.T) {
	key := testPrivateKey()
	signer := NewSigner(key)
	revision := testSignable()
	revision.IssuerName = "PT Arcon Penta Persada"
	signature, _ := signer.SignRevision(revision)

	// The company is renamed; a stored revision keeps its original issuer,
	// so verification still succeeds.
	stored := revision
	stored.IssuerName = "PT Arcon Penta Persada"
	if !VerifyRevision(signer.publicKey, stored, signature) {
		t.Error("historical signature failed after a company rename")
	}

	// Had we recomputed the issuer from the live profile instead, the
	// signature would break — this is the regression the column prevents.
	rewritten := revision
	rewritten.IssuerName = "PT Arcon Penta Persada (New Name)"
	if VerifyRevision(signer.publicKey, rewritten, signature) {
		t.Error("signature verified against a different issuer; issuer is not covered")
	}
}

func testPrivateKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i * 7)
	}
	return ed25519.NewKeyFromSeed(seed)
}
