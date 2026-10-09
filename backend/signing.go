package backend

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CanonicalPrefix versions the signed payload format. It must never change
// for existing codes: bump it and every printed signature stops verifying.
const CanonicalPrefix = "VERA1:QUOTE"

// SignableRevision is the complete field set a forger would want to touch.
// The list is deliberately generous: verification happens server-side, so
// coverage costs nothing at print time. Both mappers below MUST produce
// identical canonical bytes for the same revision — that equality is what
// lets an exported signature verify against stored rows later.
type SignableRevision struct {
	IssuerName       string
	RecipientName    string
	RecipientCompany string
	RecipientAddress string
	Subject          string
	Incoterm         string
	QuoteNumber      string
	Items            []SignableItem
	Subtotal         int64
	DiscountAmount   int64
	PpnAmount        int64
	GrandTotal       int64
	LeadTimeDays     int16
	DiscountPercent  int16
	PaymentTermDays  int16
	ValidityDays     int16
	RevisionNumber   int16
}

// SignableItem carries the values printed on the customer document (the
// marginalized side), never cost or margin.
//
// Subs is the item's composition when it is a main item with sub-items. The
// parent line's own fields already carry the rollup; the subs are covered
// because they print (state C shows their descriptions, quantities and
// prices; state B shows their descriptions as scope rows). A revision with
// no sub-items emits no sub lines at all, so every payload signed before this
// field existed remains byte-identical and keeps verifying.
type SignableItem struct {
	Description string
	UoM         string
	Qty         int64
	UnitPrice   int64
	Total       int64
	// DiscountPercent is the row's own discount. UnitPrice and Total above are
	// already the post-discount figures, so a discount that changes the money
	// is covered without this field; this covers the printed percentage
	// itself, which is content the customer reads.
	//
	// Emitted only when non-zero, exactly like Subs: a revision whose rows
	// carry no per-row discount produces byte-identical canonical bytes to
	// every revision signed before this field existed, so every printed
	// verification code keeps verifying. Never bump CanonicalPrefix for this.
	DiscountPercent int16
	Subs            []SignableSub
}

// SignableSub is one printed sub-item row: what the customer sees, never the
// cost side and never a margin rate. Details are the sub's scope lines --
// description-only text that prints beneath it in both display states, so it
// is signed content like any other printed string. The B/C display choice is
// deliberately not covered here: it selects which of these signed values
// print, it never changes a value.
type SignableSub struct {
	Description     string
	UoM             string
	Qty             int64
	UnitPrice       int64
	Total           int64
	DiscountPercent int16
	Details         []string
}

// CanonicalRevision renders the byte string the signature covers:
// fixed-order labeled fields, LF-joined, no trailing newline.
func CanonicalRevision(revision SignableRevision) []byte {
	var b strings.Builder
	b.WriteString(CanonicalPrefix)
	writeField(&b, "number", revision.QuoteNumber)
	writeField(&b, "revision", fmt.Sprintf("%02d", revision.RevisionNumber))
	writeField(&b, "recipient_name", revision.RecipientName)
	writeField(&b, "recipient_company", revision.RecipientCompany)
	writeField(&b, "recipient_address", revision.RecipientAddress)
	writeField(&b, "subject", revision.Subject)
	writeField(&b, "incoterm", revision.Incoterm)
	writeField(&b, "validity_days", strconv.Itoa(int(revision.ValidityDays)))
	writeField(&b, "payment_term_days", strconv.Itoa(int(revision.PaymentTermDays)))
	writeField(&b, "lead_time_days", strconv.Itoa(int(revision.LeadTimeDays)))
	writeField(&b, "discount_percent", strconv.Itoa(int(revision.DiscountPercent)))
	writeField(&b, "issuer", revision.IssuerName)
	writeField(&b, "currency", "IDR")
	writeField(&b, "item_count", strconv.Itoa(len(revision.Items)))
	for i, item := range revision.Items {
		writeField(&b, fmt.Sprintf("item_%d", i+1), strings.Join([]string{
			item.Description,
			strconv.FormatInt(item.Qty, 10),
			item.UoM,
			strconv.FormatInt(item.UnitPrice, 10),
			strconv.FormatInt(item.Total, 10),
		}, "|"))
		// Zero means the row carried no discount of its own, which is every
		// row of every revision signed before #274. Emitting nothing then is
		// what keeps those signatures byte-identical.
		if item.DiscountPercent > 0 {
			writeField(&b, fmt.Sprintf("item_%d_discount_percent", i+1),
				strconv.Itoa(int(item.DiscountPercent)))
		}
		if len(item.Subs) == 0 {
			continue
		}
		writeField(&b, fmt.Sprintf("item_%d_subs", i+1), strconv.Itoa(len(item.Subs)))
		for j, sub := range item.Subs {
			writeField(&b, fmt.Sprintf("item_%d_sub_%d", i+1, j+1), strings.Join([]string{
				sub.Description,
				strconv.FormatInt(sub.Qty, 10),
				sub.UoM,
				strconv.FormatInt(sub.UnitPrice, 10),
				strconv.FormatInt(sub.Total, 10),
			}, "|"))
			if sub.DiscountPercent > 0 {
				writeField(&b, fmt.Sprintf("item_%d_sub_%d_discount_percent", i+1, j+1),
					strconv.Itoa(int(sub.DiscountPercent)))
			}
			for k, detail := range sub.Details {
				writeField(&b, fmt.Sprintf("item_%d_sub_%d_detail_%d", i+1, j+1, k+1), detail)
			}
		}
	}
	writeField(&b, "subtotal", strconv.FormatInt(revision.Subtotal, 10))
	writeField(&b, "discount_amount", strconv.FormatInt(revision.DiscountAmount, 10))
	writeField(&b, "ppn_amount", strconv.FormatInt(revision.PpnAmount, 10))
	writeField(&b, "grand_total", strconv.FormatInt(revision.GrandTotal, 10))
	return []byte(b.String())
}

// writeField appends one labeled line. Newlines inside values would break
// line framing, so they become spaces — descriptions are multi-line.
func writeField(b *strings.Builder, label, value string) {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", " "), "\n", " ")
	b.WriteString("\n")
	b.WriteString(label)
	b.WriteString(": ")
	b.WriteString(value)
}

// CanonicalDigest is the SHA-256 of the canonical bytes, stored beside the
// signature so serializer drift is distinguishable from tampering.
func CanonicalDigest(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// SigningKeyID fingerprints a public key. It travels with each signature so
// retired keys keep verifying old codes after a rotation.
func SigningKeyID(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:4])
}

// LoadSigningKey reads the Ed25519 private key from a hex or base64 string
// (32-byte seed or 64-byte expanded key). The key never lives in source,
// logs, or the database: it arrives through the environment only.
func LoadSigningKey(encoded string) (ed25519.PrivateKey, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, fmt.Errorf("signing key is required: set VERA_SIGNING_KEY to the base64 or hex Ed25519 private key")
	}
	raw, err := decodeKeyMaterial(trimmed)
	if err != nil {
		return nil, err
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	default:
		return nil, fmt.Errorf("signing key must be %d bytes (seed) or %d bytes (expanded), got %d",
			ed25519.SeedSize, ed25519.PrivateKeySize, len(raw))
	}
}

func decodeKeyMaterial(encoded string) ([]byte, error) {
	if raw, err := hex.DecodeString(encoded); err == nil {
		return raw, nil
	}
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		if raw, err := encoding.DecodeString(encoded); err == nil {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("signing key is neither hex nor base64")
}

// SigningKeyFromEnvironment loads the key from VERA_SIGNING_KEY.
func SigningKeyFromEnvironment() (ed25519.PrivateKey, error) {
	return LoadSigningKey(os.Getenv("VERA_SIGNING_KEY"))
}

// SignCanonical returns the base64url signature for canonical bytes.
func SignCanonical(key ed25519.PrivateKey, canonical []byte) string {
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, canonical))
}

// VerifyCanonical reports whether signature authenticates the canonical
// bytes under publicKey. Any tampering with any signed field fails here.
func VerifyCanonical(publicKey ed25519.PublicKey, canonical []byte, signature string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	return ed25519.Verify(publicKey, canonical, raw)
}
