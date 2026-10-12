package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ShortIDLength is the public verification handle width. The handle must
// identify one *revision* — two revisions of the same quotation are
// different documents and must never share a code — so it composes the
// first 14 hex characters of the quotation UUID (56 bits: not enumerable)
// with the revision number in 2 hex digits.
const ShortIDLength = 16

// maxEncodableRevision bounds the 2-hex-digit revision field. Exceeding it
// would silently shorten the handle, so the store refuses instead.
const maxEncodableRevision = 255

// ShortIDForRevision derives the public verification handle for one
// revision. Empty when the UUID is malformed or the revision number cannot
// be encoded; callers treat that as "no verification handle" rather than
// guessing.
func ShortIDForRevision(uuid string, revision int16) string {
	compact := strings.ReplaceAll(strings.TrimSpace(uuid), "-", "")
	if len(compact) < 14 {
		return ""
	}
	if revision < 0 || revision > maxEncodableRevision {
		return ""
	}
	return strings.ToLower(compact[:14]) + fmt.Sprintf("%02x", revision)
}

// SigningKeyRecord is a public key in the verification registry. Private
// keys never live here.
type SigningKeyRecord struct {
	CreatedAt time.Time
	RetiredAt *time.Time
	KeyID     string
	PublicKey string
	Active    bool
}

// RegisterSigningKey adds a public key to the registry. Re-registering the
// same key id is a no-op, so boot is idempotent across restarts.
func (s *Store) RegisterSigningKey(ctx context.Context, keyID, publicKey string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if strings.TrimSpace(keyID) == "" || strings.TrimSpace(publicKey) == "" {
		return fmt.Errorf("signing key id and public key are required")
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.signing_keys (key_id, public_key)
		VALUES ($1, $2)
		ON CONFLICT (key_id) DO NOTHING`, s.configSchema()),
		keyID, publicKey); err != nil {
		return fmt.Errorf("register signing key: %w", err)
	}
	return nil
}

// GetSigningKey returns one registered public key.
func (s *Store) GetSigningKey(ctx context.Context, keyID string) (SigningKeyRecord, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return SigningKeyRecord{}, err
	}
	var record SigningKeyRecord
	var retiredAt *time.Time
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT key_id, public_key, active, created_at, retired_at
		FROM %s.signing_keys WHERE key_id = $1`, s.configSchema()),
		keyID).Scan(&record.KeyID, &record.PublicKey, &record.Active, &record.CreatedAt, &retiredAt)
	if err != nil {
		return SigningKeyRecord{}, fmt.Errorf("signing key %q not found", keyID)
	}
	record.RetiredAt = retiredAt
	return record, nil
}

// RevisionEvidence is the minimal verification surface for one revision.
// It carries exactly what the portal needs to render a verdict — no line
// items, no internal economics, no customer identifiers beyond the
// recipient already printed on the document.
type RevisionEvidence struct {
	CreatedAt        time.Time
	RecipientAddress string
	QuoteNumber      string
	ShortID          string
	RecipientName    string
	RecipientCompany string
	IssuerName       string
	// Currency is the code the revision was issued in, so SignableFromEvidence
	// reproduces the signed bytes. Revisions written before the column read
	// back as IDR, which is what they were signed as.
	Currency        string
	Subject         string
	Incoterm        string
	CanonicalSHA256 string
	SigKeyID        string
	Signature       string
	Items           []EvidenceItem
	Subtotal        int64
	DiscountAmount  int64
	PpnAmount       int64
	GrandTotal      int64
	ItemCount       int
	QuotationID     int64
	DiscountPercent int16
	LeadTimeDays    int16
	PaymentTermDays int16
	ValidityDays    int16
	RevisionNumber  int16
}

// EvidenceItem is the customer-visible slice of a line: description, qty,
// unit, and the marginalized prices that print on the document. Cost and
// margin never enter the verification surface.
type EvidenceItem struct {
	Description string
	UoM         string
	Qty         int64
	UnitPrice   int64
	Total       int64
	// DiscountPercent is the row's own discount, which prints on the
	// customer document when it is non-zero. Zero for every revision written
	// before row discounts existed, which is what keeps their canonical bytes
	// -- and their signatures -- byte-identical.
	DiscountPercent int16
	// Subs is the item's printed composition when it is a main item with
	// sub-items. Empty for every revision written before sub-items existed,
	// which is what keeps their canonical bytes -- and their signatures --
	// identical.
	Subs []EvidenceSub
}

// EvidenceSub is one printed sub-item row: the customer-visible side only.
// Details are its scope lines, description-only and printed beneath it.
type EvidenceSub struct {
	Description     string
	UoM             string
	Qty             int64
	UnitPrice       int64
	Total           int64
	DiscountPercent int16
	Details         []string
}

// GetRevisionEvidenceByShortID resolves a verification handle to its
// revision evidence. Unknown or malformed handles return the same error, so
// callers cannot distinguish "never existed" from "revoked".
func (s *Store) GetRevisionEvidenceByShortID(ctx context.Context, shortID string) (RevisionEvidence, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return RevisionEvidence{}, err
	}
	normalized := strings.ToLower(strings.TrimSpace(shortID))
	if len(normalized) != ShortIDLength {
		return RevisionEvidence{}, fmt.Errorf("revision not found")
	}
	schema := s.configSchema()
	var evidence RevisionEvidence
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT r.short_id, q.id, q.quote_number, r.revision_number,
		       r.recipient_name, r.recipient_address, c.legal_name,
		       r.subject, r.incoterm, r.currency,
		       r.validity_days, r.payment_term_days, r.lead_time_days,
		       r.discount_percent, r.created_at,
		       r.signature, r.sig_key_id, r.canonical_sha256, r.issuer_name,
		       t.marginalized_subtotal, t.marginalized_discount,
		       t.ppn_amount, t.customer_grand_total
		FROM %s.quotation_revisions r
		JOIN %s.quotations q ON q.id = r.quotation_id
		JOIN %s.customers c ON c.id = q.customer_id
		JOIN %s.quotation_revision_totals t ON t.revision_id = r.id
		WHERE r.short_id = $1`, schema, schema, schema, schema),
		normalized).Scan(
		&evidence.ShortID, &evidence.QuotationID, &evidence.QuoteNumber, &evidence.RevisionNumber,
		&evidence.RecipientName, &evidence.RecipientAddress, &evidence.RecipientCompany,
		&evidence.Subject, &evidence.Incoterm, &evidence.Currency,
		&evidence.ValidityDays, &evidence.PaymentTermDays, &evidence.LeadTimeDays,
		&evidence.DiscountPercent, &evidence.CreatedAt,
		&evidence.Signature, &evidence.SigKeyID, &evidence.CanonicalSHA256, &evidence.IssuerName,
		&evidence.Subtotal, &evidence.DiscountAmount, &evidence.PpnAmount, &evidence.GrandTotal,
	)
	if err != nil {
		return RevisionEvidence{}, fmt.Errorf("revision not found")
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT i.item_no, i.description, i.qty, i.uom,
		       i.marginalized_unit_price, i.marginalized_total, i.discount_percent
		FROM %s.quotation_revision_items i
		WHERE i.revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE short_id = $1)
		ORDER BY i.item_no`, schema, schema),
		normalized)
	if err != nil {
		return RevisionEvidence{}, fmt.Errorf("get revision items: %w", err)
	}
	defer rows.Close()
	itemNos := make([]int, 0, len(evidence.Items))
	for rows.Next() {
		var (
			itemNo int
			item   EvidenceItem
		)
		if err := rows.Scan(&itemNo, &item.Description, &item.Qty, &item.UoM,
			&item.UnitPrice, &item.Total, &item.DiscountPercent); err != nil {
			return RevisionEvidence{}, fmt.Errorf("scan revision item: %w", err)
		}
		itemNos = append(itemNos, itemNo)
		evidence.Items = append(evidence.Items, item)
	}
	if err := rows.Err(); err != nil {
		return RevisionEvidence{}, fmt.Errorf("iterate revision items: %w", err)
	}

	// Sub-item rows attach to their parent item by number, the same key the
	// quotations read uses. A revision written before sub-items existed simply
	// returns none, and its canonical payload is unchanged.
	subRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT s.id, ri.item_no, s.description, s.qty, s.uom, s.unit_price, s.total,
		       s.discount_percent
		FROM %s.quotation_revision_sub_items s
		JOIN %s.quotation_revision_items ri ON ri.id = s.revision_item_id
		WHERE ri.revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE short_id = $1)
		ORDER BY ri.item_no, s.sub_no`, schema, schema, schema),
		normalized)
	if err != nil {
		return RevisionEvidence{}, fmt.Errorf("get revision sub-items: %w", err)
	}
	defer subRows.Close()
	subAt := make(map[int64][2]int)
	for subRows.Next() {
		var (
			subID  int64
			itemNo int
			sub    EvidenceSub
		)
		if err := subRows.Scan(&subID, &itemNo, &sub.Description, &sub.Qty, &sub.UoM,
			&sub.UnitPrice, &sub.Total, &sub.DiscountPercent); err != nil {
			return RevisionEvidence{}, fmt.Errorf("scan revision sub-item: %w", err)
		}
		attached := false
		for i, no := range itemNos {
			if no == itemNo {
				subAt[subID] = [2]int{i, len(evidence.Items[i].Subs)}
				evidence.Items[i].Subs = append(evidence.Items[i].Subs, sub)
				attached = true
				break
			}
		}
		if !attached {
			return RevisionEvidence{}, fmt.Errorf("revision sub-item %d has no parent item", itemNo)
		}
	}
	if err := subRows.Err(); err != nil {
		return RevisionEvidence{}, fmt.Errorf("iterate revision sub-items: %w", err)
	}

	// Detail lines print beneath their sub in both display states, so they
	// are signed content and round-trip through evidence the same way.
	detailRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT ss.revision_sub_item_id, ss.description
		FROM %s.quotation_revision_sub_sub_items ss
		JOIN %s.quotation_revision_sub_items s ON s.id = ss.revision_sub_item_id
		JOIN %s.quotation_revision_items ri ON ri.id = s.revision_item_id
		WHERE ri.revision_id = (
			SELECT id FROM %s.quotation_revisions
			WHERE short_id = $1)
		ORDER BY ss.revision_sub_item_id, ss.sub_sub_no`, schema, schema, schema, schema),
		normalized)
	if err != nil {
		return RevisionEvidence{}, fmt.Errorf("get revision sub-item details: %w", err)
	}
	defer detailRows.Close()
	for detailRows.Next() {
		var (
			subID  int64
			detail string
		)
		if err := detailRows.Scan(&subID, &detail); err != nil {
			return RevisionEvidence{}, fmt.Errorf("scan revision sub-item detail: %w", err)
		}
		at, ok := subAt[subID]
		if !ok {
			return RevisionEvidence{}, fmt.Errorf("revision sub-item detail for unknown sub %d", subID)
		}
		sub := &evidence.Items[at[0]].Subs[at[1]]
		sub.Details = append(sub.Details, detail)
	}
	if err := detailRows.Err(); err != nil {
		return RevisionEvidence{}, fmt.Errorf("iterate revision sub-item details: %w", err)
	}
	evidence.ItemCount = len(evidence.Items)
	return evidence, nil
}

// LatestRevisionNumber reports the newest revision number for a quotation,
// so the portal can flag a superseded document.
func (s *Store) LatestRevisionNumber(ctx context.Context, quotationID int64) (int16, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return 0, err
	}
	var latest int16
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT MAX(revision_number) FROM %s.quotation_revisions WHERE quotation_id = $1`,
		s.configSchema()), quotationID).Scan(&latest); err != nil {
		return 0, fmt.Errorf("latest revision: %w", err)
	}
	return latest, nil
}

// SaveRevisionSignature backfills the signature columns on a revision that
// predates signing (short_id already exists from migration). It refuses to
// overwrite an existing signature: stored evidence is immutable.
func (s *Store) SaveRevisionSignature(ctx context.Context, shortID, signature, keyID, canonicalSHA256 string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_revisions
		SET signature = $2, sig_key_id = $3, canonical_sha256 = $4
		WHERE short_id = $1 AND signature = ''`, s.configSchema()),
		shortID, signature, keyID, canonicalSHA256)
	if err != nil {
		return fmt.Errorf("save revision signature: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("revision %q already signed or missing", shortID)
	}
	return nil
}
