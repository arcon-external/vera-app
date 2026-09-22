package backend

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func testSignable() SignableRevision {
	return SignableRevision{
		QuoteNumber:      "15020005",
		RevisionNumber:   0,
		RecipientName:    "Jesicha",
		RecipientCompany: "PT. Infineon Technologies Batam",
		RecipientAddress: "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam",
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
		Items: []SignableItem{
			{Description: "Scrubber control panel", Qty: 2, UoM: "Lot", UnitPrice: 120000, Total: 240000},
			{Description: "Commissioning", Qty: 1, UoM: "Lot", UnitPrice: 50000, Total: 50000},
		},
	}
}

// TestCanonicalFormatIsPinned freezes the byte format. Changing it silently
// would invalidate every signature ever printed, so the expected literal is
// deliberately verbose — editing it is a conscious migration, not a tweak.
func TestCanonicalFormatIsPinned(t *testing.T) {
	want := `VERA1:QUOTE
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
	got := string(CanonicalRevision(testSignable()))
	if got != want {
		t.Errorf("canonical revision drifted:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestCanonicalFlattensNewlines(t *testing.T) {
	revision := testSignable()
	revision.Items[0].Description = "Line one\n- bullet\r\n- bullet two"
	canonical := string(CanonicalRevision(revision))
	if strings.Contains(canonical, "\r") {
		t.Error("carriage return survived into the canonical form")
	}
	for _, line := range strings.Split(canonical, "\n") {
		if strings.HasPrefix(line, "- bullet") {
			t.Errorf("embedded newline broke line framing: %q", line)
		}
	}
	if !strings.Contains(canonical, "Line one - bullet - bullet two") {
		t.Error("multi-line description was not flattened to one field value")
	}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	key := ed25519.NewKeyFromSeed(seed)
	canonical := CanonicalRevision(testSignable())
	signature := SignCanonical(key, canonical)

	if !VerifyCanonical(key.Public().(ed25519.PublicKey), canonical, signature) {
		t.Fatal("genuine signature failed verification")
	}
}

// TestEverySignedFieldIsTamperProof walks the signed set and proves each
// field is covered: one mutation per field, all must fail.
func TestEverySignedFieldIsTamperProof(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(255 - i)
	}
	key := ed25519.NewKeyFromSeed(seed)
	publicKey := key.Public().(ed25519.PublicKey)
	base := testSignable()
	canonical := CanonicalRevision(base)
	signature := SignCanonical(key, canonical)

	mutations := map[string]func(r *SignableRevision){
		"number":            func(r *SignableRevision) { r.QuoteNumber = "15020006" },
		"revision":          func(r *SignableRevision) { r.RevisionNumber = 1 },
		"recipient_name":    func(r *SignableRevision) { r.RecipientName = "Someone Else" },
		"recipient_company": func(r *SignableRevision) { r.RecipientCompany = "PT Rival" },
		"recipient_address": func(r *SignableRevision) { r.RecipientAddress = "Elsewhere" },
		"subject":           func(r *SignableRevision) { r.Subject = "Different scope" },
		"incoterm":          func(r *SignableRevision) { r.Incoterm = "DDP" },
		"validity_days":     func(r *SignableRevision) { r.ValidityDays = 365 },
		"payment_term_days": func(r *SignableRevision) { r.PaymentTermDays = 7 },
		"lead_time_days":    func(r *SignableRevision) { r.LeadTimeDays = 1 },
		"discount_percent":  func(r *SignableRevision) { r.DiscountPercent = 0 },
		"issuer":            func(r *SignableRevision) { r.IssuerName = "PT Impostor" },
		"item_count":        func(r *SignableRevision) { r.Items = r.Items[:1] },
		"item_unit_price":   func(r *SignableRevision) { r.Items[0].UnitPrice = 1 },
		"item_total":        func(r *SignableRevision) { r.Items[0].Total = 1 },
		"item_qty":          func(r *SignableRevision) { r.Items[0].Qty = 99 },
		"item_uom":          func(r *SignableRevision) { r.Items[0].UoM = "EA" },
		"item_description":  func(r *SignableRevision) { r.Items[1].Description = "Extra free item" },
		"subtotal":          func(r *SignableRevision) { r.Subtotal = 1 },
		"discount_amount":   func(r *SignableRevision) { r.DiscountAmount = 0 },
		"ppn_amount":        func(r *SignableRevision) { r.PpnAmount = 0 },
		"grand_total":       func(r *SignableRevision) { r.GrandTotal = 200000 },
	}
	for field, mutate := range mutations {
		t.Run(field, func(t *testing.T) {
			tampered := base
			tampered.Items = append([]SignableItem(nil), base.Items...)
			mutate(&tampered)
			if VerifyCanonical(publicKey, CanonicalRevision(tampered), signature) {
				t.Errorf("mutation of %s still verified: field is not covered by the signature", field)
			}
		})
	}
}

func TestSigningKeyLoading(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	key := ed25519.NewKeyFromSeed(seed)

	for name, encoded := range map[string]string{
		"hex seed":       hex.EncodeToString(seed),
		"base64 seed":    base64.StdEncoding.EncodeToString(seed),
		"base64url seed": base64.RawURLEncoding.EncodeToString(seed),
		"hex expanded":   hex.EncodeToString(key),
	} {
		t.Run(name, func(t *testing.T) {
			loaded, err := LoadSigningKey(encoded)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if !loaded.Equal(key) {
				t.Error("loaded key differs from the source key")
			}
		})
	}

	if _, err := LoadSigningKey(""); err == nil {
		t.Error("empty key accepted; want a configuration error")
	}
	if _, err := LoadSigningKey("not-a-key"); err == nil {
		t.Error("garbage key accepted; want an error")
	}
	if _, err := LoadSigningKey(hex.EncodeToString([]byte{1, 2, 3})); err == nil {
		t.Error("wrong-length key accepted; want an error")
	}
}

func TestSigningKeyIDIsStableAndDistinct(t *testing.T) {
	seedA := make([]byte, ed25519.SeedSize)
	seedB := make([]byte, ed25519.SeedSize)
	seedB[0] = 9
	keyA := ed25519.NewKeyFromSeed(seedA)
	keyB := ed25519.NewKeyFromSeed(seedB)
	idA := SigningKeyID(keyA.Public().(ed25519.PublicKey))
	idB := SigningKeyID(keyB.Public().(ed25519.PublicKey))
	if idA != SigningKeyID(keyA.Public().(ed25519.PublicKey)) {
		t.Error("key id is not stable across calls")
	}
	if idA == idB {
		t.Error("different keys share an id")
	}
	if len(idA) != 8 {
		t.Errorf("key id = %q; want 8 hex characters", idA)
	}
}

func TestCanonicalDigestChangesWithAnyField(t *testing.T) {
	base := CanonicalDigest(CanonicalRevision(testSignable()))
	mutated := testSignable()
	mutated.GrandTotal++
	if CanonicalDigest(CanonicalRevision(mutated)) == base {
		t.Error("digest unchanged after a field mutation")
	}
}
