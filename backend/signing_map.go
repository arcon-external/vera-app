package backend

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"

	"vera-app/backend/store"
)

// SignableFromData maps prepared quotation data to the canonical field set.
// It reads the customer-visible (marginalized) side only: exactly what the
// printed document shows.
func SignableFromData(data QuotationData) SignableRevision {
	items := make([]SignableItem, 0, len(data.Items))
	for _, item := range data.Items {
		items = append(items, SignableItem{
			Description: item.Description,
			Qty:         int64(item.Qty),
			UoM:         item.UoM,
			UnitPrice:   int64(item.UnitPrice),
			Total:       int64(item.Total),
		})
	}
	return SignableRevision{
		QuoteNumber:      data.QuoteNumber,
		RevisionNumber:   int16(data.RevisionID),
		RecipientName:    data.RecipientName,
		RecipientCompany: data.RecipientCompany,
		RecipientAddress: data.RecipientAddress,
		Subject:          data.Subject,
		Incoterm:         data.Incoterm,
		ValidityDays:     int16(data.Validity),
		PaymentTermDays:  int16(data.PaymentTerm),
		LeadTimeDays:     int16(data.LeadTime),
		DiscountPercent:  int16(data.DiscountPercent),
		IssuerName:       data.CompanyName,
		Subtotal:         int64(data.SubTotal),
		DiscountAmount:   int64(data.DiscountAmount),
		PpnAmount:        int64(data.PpnAmount),
		GrandTotal:       int64(data.GrandTotal),
		Items:            items,
	}
}

// SignableFromEvidence maps stored verification evidence back to the same
// canonical field set. If this and SignableFromData ever disagree, exported
// signatures stop verifying — TestSignableMappersAgree guards that.
//
// The issuer comes from the stored revision, not the live profile: renaming
// the company later must not rewrite history or break old signatures.
func SignableFromEvidence(evidence store.RevisionEvidence) SignableRevision {
	items := make([]SignableItem, 0, len(evidence.Items))
	for _, item := range evidence.Items {
		items = append(items, SignableItem{
			Description: item.Description,
			Qty:         item.Qty,
			UoM:         item.UoM,
			UnitPrice:   item.UnitPrice,
			Total:       item.Total,
		})
	}
	return SignableRevision{
		QuoteNumber:      evidence.QuoteNumber,
		RevisionNumber:   evidence.RevisionNumber,
		RecipientName:    evidence.RecipientName,
		RecipientCompany: evidence.RecipientCompany,
		RecipientAddress: evidence.RecipientAddress,
		Subject:          evidence.Subject,
		Incoterm:         evidence.Incoterm,
		ValidityDays:     evidence.ValidityDays,
		PaymentTermDays:  evidence.PaymentTermDays,
		LeadTimeDays:     evidence.LeadTimeDays,
		DiscountPercent:  evidence.DiscountPercent,
		IssuerName:       evidence.IssuerName,
		Subtotal:         evidence.Subtotal,
		DiscountAmount:   evidence.DiscountAmount,
		PpnAmount:        evidence.PpnAmount,
		GrandTotal:       evidence.GrandTotal,
		Items:            items,
	}
}

// Signer binds the Ed25519 private key used to sign revisions. Callers get
// one at boot; a deploy without VERA_SIGNING_KEY fails there rather than
// silently issuing unsigned documents.
type Signer struct {
	key       ed25519.PrivateKey
	publicKey ed25519.PublicKey
	keyID     string
}

// NewSigner builds the signer for a private key.
func NewSigner(privateKey ed25519.PrivateKey) *Signer {
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return &Signer{key: privateKey, publicKey: publicKey, keyID: SigningKeyID(publicKey)}
}

// KeyID reports the fingerprint recorded beside every signature, so retired
// keys keep verifying the codes they signed.
func (s *Signer) KeyID() string { return s.keyID }

// PublicKeyHex renders the public key for the registry table.
func (s *Signer) PublicKeyHex() string { return hex.EncodeToString(s.publicKey) }

// SignRevision returns the signature and canonical digest for a revision.
func (s *Signer) SignRevision(revision SignableRevision) (signature, digest string) {
	canonical := CanonicalRevision(revision)
	return SignCanonical(s.key, canonical), CanonicalDigest(canonical)
}

// VerifyRevision checks a stored signature against canonical bytes.
func VerifyRevision(publicKey ed25519.PublicKey, revision SignableRevision, signature string) bool {
	return VerifyCanonical(publicKey, CanonicalRevision(revision), signature)
}

// PublicKeyFromHex parses a registered public key.
func PublicKeyFromHex(encoded string) (ed25519.PublicKey, error) {
	raw, err := hex.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("registered public key is malformed")
	}
	return ed25519.PublicKey(raw), nil
}

// LoadSignerFromEnvironment builds the signer and registers its public key
// so verification can resolve it later.
func LoadSignerFromEnvironment(ctx context.Context, database *store.Store) (*Signer, error) {
	key, err := SigningKeyFromEnvironment()
	if err != nil {
		return nil, err
	}
	signer := NewSigner(key)
	if err := database.RegisterSigningKey(ctx, signer.KeyID(), signer.PublicKeyHex()); err != nil {
		return nil, err
	}
	return signer, nil
}
