package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"vera-app/backend/apicontract"
	"vera-app/backend/currency"
)

type RevisionItem struct {
	Description           string
	UoM                   string
	ItemNo                int
	Qty                   int64
	UnitPrice             int64
	MarginAmount          int64
	OriginalTotal         int64
	MarginalizedUnitPrice int64
	MarginalizedTotal     int64
	ListUnitPrice         int64
	DiscountAmount        int64
	MarginPercent         int16
	DiscountPercent       int16
	MarginEnabled         bool
	DiscountEnabled       bool
	ShowSubPrices         bool
	SubItems              []RevisionSubItem
}

// RevisionSubItem is one priced part of a package line, stored as part of the
// immutable revision snapshot. The parent line's stored price is already the
// sum of these; the sub rows carry the composition.
//
// SubItems are the part's detail lines, description-only: they have no
// standalone values, so the table holds nothing to sum.
type RevisionSubItem struct {
	SubNo           int
	Description     string
	UoM             string
	UnitCost        int64
	UnitPrice       int64
	MarginAmount    int64
	Total           int64
	Qty             int64
	ListUnitPrice   int64
	DiscountAmount  int64
	MarginPercent   int16
	DiscountPercent int16
	MarginEnabled   bool
	DiscountEnabled bool
	SubSubItems     []RevisionSubSubItem
}

// RevisionSubSubItem is one detail line beneath a stored sub-item. Description
// only and numbered within its parent; there is nothing here to price.
type RevisionSubSubItem struct {
	SubSubNo    int
	Description string
}

type RevisionTotals struct {
	OriginalSubtotal          int64
	OriginalDiscount          int64
	OriginalGrand             int64
	MarginalizedSubtotal      int64
	MarginalizedDiscount      int64
	MarginalizedGrand         int64
	TaxAmount                 int64
	MarginalizedGrandAfterTax int64
	ProfitAmount              int64
	NettProfit                int64
	PpnAmount                 int64
	CustomerGrandTotal        int64
}

// RevisionDraft is a complete immutable snapshot. QuoteNumber may be empty
// for a new quotation; the store allocates its sequence in the same
// transaction and starts that quotation at revision 0.
type RevisionDraft struct {
	TaxCategory      string
	PDFPath          string
	CustomerName     string
	CustomerAddress  string
	QuotationUUID    string
	RecipientName    string
	RecipientAddress string
	Subject          string
	Incoterm         string
	IssuerName       string
	// Currency is the company's ISO 4217 code at issue time, stored per
	// revision beside the issuer name: a later company change must not
	// rewrite history or break old signatures.
	Currency        string
	CanonicalSHA256 string
	SigKeyID        string
	Signature       string
	TaxZone         string
	DocumentHash    string
	ServiceCategory string
	QuoteNumber     string
	HTMLSnapshot    string
	TaxType         string
	Items           []RevisionItem
	PDFData         []byte
	Totals          RevisionTotals
	TaxRateBps      int16
	CustomerCode    int16
	DiscountPercent int16
	LeadTimeDays    int16
	LeadTimeYears   int16
	LeadTimeMonths  int16
	LeadTimeWeeks   int16
	PaymentTermDays int16
	ValidityDays    int16
	RevisionNumber  int16
}

type QuotationSummary struct {
	CreatedAt      time.Time
	QuoteNumber    string
	CustomerName   string
	Subject        string
	Currency       string
	ID             int64
	GrandTotal     int64
	RevisionNumber int16
}

type QuotationRecord struct {
	UUID             string
	ServiceCategory  string
	PDFPath          string
	CustomerAddress  string
	RecipientName    string
	RecipientAddress string
	Incoterm         string
	CanonicalSHA256  string
	SigKeyID         string
	Signature        string
	ShortID          string
	TaxZone          string
	TaxCategory      string
	HTMLSnapshot     string
	// HasDocument reports whether the revision has a stored document: PDF bytes
	// or a path. It is scanned from the same expression the separate
	// HasDocumentPDF query used, so the answer is identical -- including for an
	// empty-but-not-null pdf_data, which that check counted as present and
	// len(PDFData) > 0 would not. The point of carrying it is that GetQuotation
	// already selects both columns, so asking again was a round trip for a fact
	// already in hand.
	HasDocument        bool
	TaxType            string
	AvailableRevisions []int16
	// Currency is the code the revision was issued in, for money displays
	// built from this record. History, not the live profile.
	Currency string
	Items    []RevisionItem
	PDFData  []byte
	QuotationSummary
	Totals          RevisionTotals
	CustomerCode    int16
	TaxRateBps      int16
	DiscountPercent int16
	LeadTimeDays    int16
	LeadTimeYears   int16
	LeadTimeMonths  int16
	LeadTimeWeeks   int16
	PaymentTermDays int16
	ValidityDays    int16
}

func (s *Store) ListQuotations(ctx context.Context) ([]QuotationSummary, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	schema := s.configSchema()
	// One row per quotation: DISTINCT ON collapses each quotation to its
	// highest revision, so the archive shows the latest state of every quote.
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT ON (q.id)
		       q.id, q.quote_number, r.revision_number, c.legal_name,
		       r.subject, r.created_at, t.customer_grand_total, r.currency
		FROM %s.quotations q
		JOIN %s.customers c ON c.id = q.customer_id
		JOIN %s.quotation_revisions r ON r.quotation_id = q.id
		JOIN %s.quotation_revision_totals t ON t.revision_id = r.id
		ORDER BY q.id DESC, r.revision_number DESC`, schema, schema, schema, schema))
	if err != nil {
		return nil, fmt.Errorf("list quotations: %w", err)
	}
	defer rows.Close()

	quotations := make([]QuotationSummary, 0)
	for rows.Next() {
		var quotation QuotationSummary
		if err := rows.Scan(
			&quotation.ID,
			&quotation.QuoteNumber,
			&quotation.RevisionNumber,
			&quotation.CustomerName,
			&quotation.Subject,
			&quotation.CreatedAt,
			&quotation.GrandTotal,
			&quotation.Currency,
		); err != nil {
			return nil, fmt.Errorf("scan quotation summary: %w", err)
		}
		quotations = append(quotations, quotation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quotation summaries: %w", err)
	}
	return quotations, nil
}

func (s *Store) GetQuotation(ctx context.Context, quotationID int64, revision *int16) (QuotationRecord, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return QuotationRecord{}, err
	}
	schema := s.configSchema()
	filter := fmt.Sprintf(`
		SELECT q.id, q.uuid::text, q.quote_number, r.revision_number, c.customer_code,
		       c.legal_name, ca.address, r.recipient_name, r.recipient_address,
		       r.subject, r.incoterm, r.validity_days, r.payment_term_days,
		       r.lead_time_days, r.lead_time_years, r.lead_time_months,
		       r.lead_time_weeks,
		       r.discount_percent, r.tax_zone, r.tax_category,
		       r.tax_rate_bps, r.tax_type, r.service_category, r.created_at,
		       r.currency,
		       d.html_snapshot, d.pdf_data,
		       (d.pdf_data IS NOT NULL OR d.pdf_path IS NOT NULL) AS has_document,
		       COALESCE(d.pdf_path, ''),
		       t.original_subtotal, t.original_discount, t.original_grand_total,
		       t.marginalized_subtotal, t.marginalized_discount,
		       t.marginalized_grand_total, t.tax_amount,
		       t.marginalized_grand_after_tax, t.profit_amount, t.nett_profit,
		       t.ppn_amount, t.customer_grand_total
		FROM %s.quotations q
		JOIN %s.customers c ON c.id = q.customer_id
		LEFT JOIN LATERAL (
			SELECT address FROM %s.customer_addresses
			WHERE customer_id = c.id AND is_primary = TRUE LIMIT 1
		) ca ON TRUE
		JOIN %s.quotation_revisions r ON r.quotation_id = q.id
		JOIN %s.quotation_revision_totals t ON t.revision_id = r.id
		JOIN %s.quotation_documents d ON d.revision_id = r.id
		WHERE q.id = $1`, schema, schema, schema, schema, schema, schema)
	args := []any{quotationID}
	if revision == nil {
		filter += " ORDER BY r.revision_number DESC LIMIT 1"
	} else {
		filter += " AND r.revision_number = $2"
		args = append(args, *revision)
	}

	var record QuotationRecord
	err = pool.QueryRow(ctx, filter, args...).Scan(
		&record.ID,
		&record.UUID,
		&record.QuoteNumber,
		&record.RevisionNumber,
		&record.CustomerCode,
		&record.CustomerName,
		&record.CustomerAddress,
		&record.RecipientName,
		&record.RecipientAddress,
		&record.Subject,
		&record.Incoterm,
		&record.ValidityDays,
		&record.PaymentTermDays,
		&record.LeadTimeDays,
		&record.LeadTimeYears,
		&record.LeadTimeMonths,
		&record.LeadTimeWeeks,
		&record.DiscountPercent,
		&record.TaxZone,
		&record.TaxCategory,
		&record.TaxRateBps,
		&record.TaxType,
		&record.ServiceCategory,
		&record.CreatedAt,
		&record.Currency,
		&record.HTMLSnapshot,
		&record.PDFData,
		&record.HasDocument,
		&record.PDFPath,
		&record.Totals.OriginalSubtotal,
		&record.Totals.OriginalDiscount,
		&record.Totals.OriginalGrand,
		&record.Totals.MarginalizedSubtotal,
		&record.Totals.MarginalizedDiscount,
		&record.Totals.MarginalizedGrand,
		&record.Totals.TaxAmount,
		&record.Totals.MarginalizedGrandAfterTax,
		&record.Totals.ProfitAmount,
		&record.Totals.NettProfit,
		&record.Totals.PpnAmount,
		&record.Totals.CustomerGrandTotal,
	)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("get quotation %d: %w", quotationID, err)
	}
	record.GrandTotal = record.Totals.CustomerGrandTotal

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT item_no, description, qty, uom, unit_price, margin_enabled,
		       margin_percent, margin_amount, original_total,
		       marginalized_unit_price, marginalized_total, show_sub_prices,
		       discount_enabled, discount_percent, list_unit_price
		FROM %s.quotation_revision_items
		WHERE revision_id = (
			SELECT r.id FROM %s.quotation_revisions r
			WHERE r.quotation_id = $1 AND ($2::SMALLINT IS NULL OR r.revision_number = $2)
			ORDER BY r.revision_number DESC LIMIT 1
		)
		ORDER BY item_no`, schema, schema), quotationID, revision)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("get quotation items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item RevisionItem
		if err := rows.Scan(
			&item.ItemNo,
			&item.Description,
			&item.Qty,
			&item.UoM,
			&item.UnitPrice,
			&item.MarginEnabled,
			&item.MarginPercent,
			&item.MarginAmount,
			&item.OriginalTotal,
			&item.MarginalizedUnitPrice,
			&item.MarginalizedTotal,
			&item.ShowSubPrices,
			&item.DiscountEnabled,
			&item.DiscountPercent,
			&item.ListUnitPrice,
		); err != nil {
			return QuotationRecord{}, fmt.Errorf("scan quotation item: %w", err)
		}
		item.DiscountAmount = rowDiscountAmount(item.DiscountEnabled, item.ListUnitPrice,
			item.UnitPrice, item.Qty)
		record.Items = append(record.Items, item)
	}
	if err := rows.Err(); err != nil {
		return QuotationRecord{}, fmt.Errorf("iterate quotation items: %w", err)
	}

	// Sub-item rows come back joined to their parent item's number. The sub
	// rows key to the storage row (revision item identity), but items within
	// one revision are unique by number, so attachment here goes by number.
	subRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT ri.item_no, s.sub_no, s.description, s.qty, s.uom, s.unit_cost,
		       s.margin_enabled, s.margin_percent, s.unit_price, s.margin_amount, s.total,
		       s.discount_enabled, s.discount_percent, s.list_unit_price
		FROM %s.quotation_revision_sub_items s
		JOIN %s.quotation_revision_items ri ON ri.id = s.revision_item_id
		WHERE ri.revision_id = (
			SELECT r.id FROM %s.quotation_revisions r
			WHERE r.quotation_id = $1 AND ($2::SMALLINT IS NULL OR r.revision_number = $2)
			ORDER BY r.revision_number DESC LIMIT 1
		)
		ORDER BY ri.item_no, s.sub_no`, schema, schema, schema), quotationID, revision)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("get quotation sub-items: %w", err)
	}
	defer subRows.Close()
	for subRows.Next() {
		var (
			itemNo int
			sub    RevisionSubItem
		)
		if err := subRows.Scan(
			&itemNo,
			&sub.SubNo,
			&sub.Description,
			&sub.Qty,
			&sub.UoM,
			&sub.UnitCost,
			&sub.MarginEnabled,
			&sub.MarginPercent,
			&sub.UnitPrice,
			&sub.MarginAmount,
			&sub.Total,
			&sub.DiscountEnabled,
			&sub.DiscountPercent,
			&sub.ListUnitPrice,
		); err != nil {
			return QuotationRecord{}, fmt.Errorf("scan quotation sub-item: %w", err)
		}
		sub.DiscountAmount = rowDiscountAmount(sub.DiscountEnabled, sub.ListUnitPrice,
			sub.UnitPrice, sub.Qty)
		attached := false
		for i := range record.Items {
			if record.Items[i].ItemNo == itemNo {
				record.Items[i].SubItems = append(record.Items[i].SubItems, sub)
				attached = true
				break
			}
		}
		if !attached {
			return QuotationRecord{}, fmt.Errorf("quotation sub-item %d.%d has no parent item", itemNo, sub.SubNo)
		}
	}
	if err := subRows.Err(); err != nil {
		return QuotationRecord{}, fmt.Errorf("iterate quotation sub-items: %w", err)
	}

	// Detail lines walk up through both levels: sub-sub-items key to a
	// sub-item row, which keys to an item row, so the join returns the two
	// parent numbers the record's in-memory tree hangs off.
	detailRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT ri.item_no, s.sub_no, ss.sub_sub_no, ss.description
		FROM %s.quotation_revision_sub_sub_items ss
		JOIN %s.quotation_revision_sub_items s ON s.id = ss.revision_sub_item_id
		JOIN %s.quotation_revision_items ri ON ri.id = s.revision_item_id
		WHERE ri.revision_id = (
			SELECT r.id FROM %s.quotation_revisions r
			WHERE r.quotation_id = $1 AND ($2::SMALLINT IS NULL OR r.revision_number = $2)
			ORDER BY r.revision_number DESC LIMIT 1
		)
		ORDER BY ri.item_no, s.sub_no, ss.sub_sub_no`, schema, schema, schema, schema), quotationID, revision)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("get quotation sub-item details: %w", err)
	}
	defer detailRows.Close()
	for detailRows.Next() {
		var (
			itemNo, subNo int
			low           RevisionSubSubItem
		)
		if err := detailRows.Scan(
			&itemNo,
			&subNo,
			&low.SubSubNo,
			&low.Description,
		); err != nil {
			return QuotationRecord{}, fmt.Errorf("scan quotation sub-item detail: %w", err)
		}
		attached := false
		for i := range record.Items {
			if record.Items[i].ItemNo != itemNo {
				continue
			}
			for j := range record.Items[i].SubItems {
				if record.Items[i].SubItems[j].SubNo == subNo {
					record.Items[i].SubItems[j].SubSubItems = append(record.Items[i].SubItems[j].SubSubItems, low)
					attached = true
					break
				}
			}
			break
		}
		if !attached {
			return QuotationRecord{}, fmt.Errorf("quotation detail %d.%d.%d has no parent sub-item", itemNo, subNo, low.SubSubNo)
		}
	}
	if err := detailRows.Err(); err != nil {
		return QuotationRecord{}, fmt.Errorf("iterate quotation sub-item details: %w", err)
	}

	revisions, err := s.ListRevisionNumbers(ctx, quotationID)
	if err != nil {
		return QuotationRecord{}, err
	}
	record.AvailableRevisions = revisions

	return record, nil
}

// ListRevisionNumbers returns a quotation's revision numbers, newest first.
//
// Split out of GetQuotation because the revisions endpoint needs this and
// nothing else. It used to call GetQuotation, which selects html_snapshot and
// pdf_data -- so every click pulled the whole document out of Postgres to return
// a handful of int16s, and PDFs are the largest column in the database. The
// UNIQUE (quotation_id, revision_number) index already serves this.
//
// The result is never nil, so a caller can serialise it without a nil check
// turning into a JSON null on the wire.
func (s *Store) ListRevisionNumbers(ctx context.Context, quotationID int64) ([]int16, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT revision_number FROM %s.quotation_revisions
		WHERE quotation_id = $1
		ORDER BY revision_number DESC`, s.configSchema()), quotationID)
	if err != nil {
		return nil, fmt.Errorf("list quotation revisions: %w", err)
	}
	defer rows.Close()

	revisions := []int16{}
	for rows.Next() {
		var revisionNumber int16
		if err := rows.Scan(&revisionNumber); err != nil {
			return nil, fmt.Errorf("scan quotation revision: %w", err)
		}
		revisions = append(revisions, revisionNumber)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quotation revisions: %w", err)
	}
	return revisions, nil
}

func (s *Store) DeleteQuotation(ctx context.Context, quotationID int64) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %s.quotations WHERE id = $1", s.configSchema(),
	), quotationID)
	if err != nil {
		return fmt.Errorf("delete quotation %d: %w", quotationID, err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("quotation %d not found", quotationID)
	}
	return nil
}

func (s *Store) SaveRevision(ctx context.Context, draft RevisionDraft) (QuotationRecord, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return QuotationRecord{}, err
	}
	if err := validateRevisionDraft(draft); err != nil {
		return QuotationRecord{}, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("begin quotation revision transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := s.configSchema()
	customerID, err := upsertCustomer(ctx, tx, schema, draft)
	if err != nil {
		return QuotationRecord{}, err
	}

	quoteID, quoteUUID, quoteNumber, revisionNumber, err := ensureQuotation(ctx, tx, schema, draft, customerID)
	if err != nil {
		return QuotationRecord{}, err
	}

	var revisionID int64
	// An empty draft currency stores as the default, exactly like the column
	// would: revisions written before this column existed read back as IDR,
	// and so does a draft that never set one.
	currencyCode := currency.Normalize(draft.Currency)
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revisions (
			quotation_id, revision_number, recipient_name, recipient_address,
			subject, incoterm, validity_days, payment_term_days, lead_time_days,
			lead_time_years, lead_time_months, lead_time_weeks,
			discount_percent, tax_zone, tax_category, tax_rate_bps,
			tax_type, service_category,
			short_id, signature, sig_key_id, canonical_sha256, issuer_name, currency
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		RETURNING id`, schema),
		quoteID,
		revisionNumber,
		draft.RecipientName,
		draft.RecipientAddress,
		draft.Subject,
		draft.Incoterm,
		draft.ValidityDays,
		draft.PaymentTermDays,
		draft.LeadTimeDays,
		draft.LeadTimeYears,
		draft.LeadTimeMonths,
		draft.LeadTimeWeeks,
		draft.DiscountPercent,
		draft.TaxZone,
		draft.TaxCategory,
		draft.TaxRateBps,
		draft.TaxType,
		draft.ServiceCategory,
		ShortIDForRevision(quoteUUID, revisionNumber),
		draft.Signature,
		draft.SigKeyID,
		draft.CanonicalSHA256,
		draft.IssuerName,
		currencyCode,
	).Scan(&revisionID)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("insert quotation revision %d: %w", revisionNumber, err)
	}

	for _, item := range draft.Items {
		var itemID int64
		// RETURNING id: sub-item rows key to the storage row, not the item
		// number, so the snapshot of one revision's composition hangs off the
		// same identity the totals hang off.
		if err := tx.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO %s.quotation_revision_items (
				revision_id, item_no, description, qty, uom, unit_price,
				margin_enabled, margin_percent, margin_amount, original_total,
				marginalized_unit_price, marginalized_total, show_sub_prices,
				discount_enabled, discount_percent, list_unit_price
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, schema),
			revisionID,
			item.ItemNo,
			item.Description,
			item.Qty,
			item.UoM,
			item.UnitPrice,
			item.MarginEnabled,
			item.MarginPercent,
			item.MarginAmount,
			item.OriginalTotal,
			item.MarginalizedUnitPrice,
			item.MarginalizedTotal,
			item.ShowSubPrices,
			item.DiscountEnabled,
			item.DiscountPercent,
			item.ListUnitPrice,
		).Scan(&itemID); err != nil {
			return QuotationRecord{}, fmt.Errorf("insert quotation revision item %d: %w", item.ItemNo, err)
		}

		for _, sub := range item.SubItems {
			var subID int64
			if err := tx.QueryRow(ctx, fmt.Sprintf(`
				INSERT INTO %s.quotation_revision_sub_items (
					revision_item_id, sub_no, description, qty, uom, unit_cost,
					margin_enabled, margin_percent, unit_price, margin_amount, total,
					discount_enabled, discount_percent, list_unit_price
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`, schema),
				itemID,
				sub.SubNo,
				sub.Description,
				sub.Qty,
				sub.UoM,
				sub.UnitCost,
				sub.MarginEnabled,
				sub.MarginPercent,
				sub.UnitPrice,
				sub.MarginAmount,
				sub.Total,
				sub.DiscountEnabled,
				sub.DiscountPercent,
				sub.ListUnitPrice,
			).Scan(&subID); err != nil {
				return QuotationRecord{}, fmt.Errorf("insert quotation revision item %d sub-item %d: %w", item.ItemNo, sub.SubNo, err)
			}

			for _, low := range sub.SubSubItems {
				if _, err := tx.Exec(ctx, fmt.Sprintf(`
					INSERT INTO %s.quotation_revision_sub_sub_items (
						revision_sub_item_id, sub_sub_no, description
					) VALUES ($1,$2,$3)`, schema),
					subID,
					low.SubSubNo,
					low.Description,
				); err != nil {
					return QuotationRecord{}, fmt.Errorf("insert quotation revision item %d sub-item %d detail %d: %w", item.ItemNo, sub.SubNo, low.SubSubNo, err)
				}
			}
		}
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revision_totals (
			revision_id, original_subtotal, original_discount, original_grand_total,
			marginalized_subtotal, marginalized_discount, marginalized_grand_total,
			tax_amount, marginalized_grand_after_tax, profit_amount, nett_profit,
			ppn_amount, customer_grand_total
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, schema),
		revisionID,
		draft.Totals.OriginalSubtotal,
		draft.Totals.OriginalDiscount,
		draft.Totals.OriginalGrand,
		draft.Totals.MarginalizedSubtotal,
		draft.Totals.MarginalizedDiscount,
		draft.Totals.MarginalizedGrand,
		draft.Totals.TaxAmount,
		draft.Totals.MarginalizedGrandAfterTax,
		draft.Totals.ProfitAmount,
		draft.Totals.NettProfit,
		draft.Totals.PpnAmount,
		draft.Totals.CustomerGrandTotal,
	); err != nil {
		return QuotationRecord{}, fmt.Errorf("insert quotation revision totals: %w", err)
	}

	documentHash := draft.DocumentHash
	if documentHash == "" {
		hash := sha256.Sum256(draft.PDFData)
		documentHash = hex.EncodeToString(hash[:])
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_documents (
			revision_id, html_snapshot, pdf_data, pdf_path, sha256
		) VALUES ($1,$2,$3,$4,$5)`, schema),
		revisionID,
		draft.HTMLSnapshot,
		nilIfEmptyBytes(draft.PDFData),
		nilIfEmptyString(draft.PDFPath),
		documentHash,
	); err != nil {
		return QuotationRecord{}, fmt.Errorf("insert quotation document: %w", err)
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.quotations SET updated_at = NOW() WHERE id = $1",
		schema,
	), quoteID); err != nil {
		return QuotationRecord{}, fmt.Errorf("update quotation timestamp: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return QuotationRecord{}, fmt.Errorf("commit quotation revision: %w", err)
	}

	return QuotationRecord{
		QuotationSummary: QuotationSummary{
			ID:             quoteID,
			QuoteNumber:    quoteNumber,
			RevisionNumber: revisionNumber,
			CustomerName:   draft.CustomerName,
			Subject:        draft.Subject,
			GrandTotal:     draft.Totals.MarginalizedGrand,
		},
		UUID:             quoteUUID,
		ShortID:          ShortIDForRevision(quoteUUID, revisionNumber),
		CustomerCode:     draft.CustomerCode,
		CustomerAddress:  draft.CustomerAddress,
		RecipientName:    draft.RecipientName,
		RecipientAddress: draft.RecipientAddress,
		Incoterm:         draft.Incoterm,
		ValidityDays:     draft.ValidityDays,
		PaymentTermDays:  draft.PaymentTermDays,
		LeadTimeDays:     draft.LeadTimeDays,
		LeadTimeYears:    draft.LeadTimeYears,
		LeadTimeMonths:   draft.LeadTimeMonths,
		LeadTimeWeeks:    draft.LeadTimeWeeks,
		DiscountPercent:  draft.DiscountPercent,
		TaxZone:          draft.TaxZone,
		TaxCategory:      draft.TaxCategory,
		TaxRateBps:       draft.TaxRateBps,
		Items:            draft.Items,
		Totals:           draft.Totals,
		HTMLSnapshot:     draft.HTMLSnapshot,
		PDFData:          draft.PDFData,
		PDFPath:          draft.PDFPath,
	}, nil
}

func validateRevisionDraft(draft RevisionDraft) error {
	if draft.CustomerCode < 1 || draft.CustomerName == "" {
		return fmt.Errorf("customer code and name are required")
	}
	if draft.Subject == "" || draft.RecipientName == "" || draft.RecipientAddress == "" {
		return fmt.Errorf("recipient and subject are required")
	}
	if len(draft.Items) == 0 {
		return fmt.Errorf("at least one quotation item is required")
	}
	if draft.HTMLSnapshot == "" {
		return fmt.Errorf("quotation HTML snapshot is required")
	}
	return nil
}

func upsertCustomer(ctx context.Context, tx pgx.Tx, schema string, draft RevisionDraft) (int64, error) {
	var customerID int64
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.customers (customer_code, legal_name)
		VALUES ($1, $2)
		ON CONFLICT (customer_code) DO UPDATE SET legal_name = EXCLUDED.legal_name, updated_at = NOW()
		RETURNING id`, schema), draft.CustomerCode, draft.CustomerName).Scan(&customerID)
	if err != nil {
		return 0, fmt.Errorf("upsert customer: %w", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.customer_addresses (customer_id, address, is_primary)
		SELECT $1, $2, TRUE
		WHERE NOT EXISTS (
			SELECT 1 FROM %s.customer_addresses WHERE customer_id = $1 AND is_primary = TRUE
		)`, schema, schema), customerID, draft.CustomerAddress); err != nil {
		return 0, fmt.Errorf("upsert customer address: %w", err)
	}
	return customerID, nil
}

func ensureQuotation(ctx context.Context, tx pgx.Tx, schema string, draft RevisionDraft, customerID int64) (int64, string, string, int16, error) {
	if draft.QuoteNumber == "" {
		if draft.RevisionNumber != 0 {
			return 0, "", "", 0, fmt.Errorf("new quotation must start at revision 0")
		}
		yy := CurrentYY()
		sequence, err := nextQuoteSequenceTx(ctx, tx, schema, int(draft.CustomerCode), yy)
		if err != nil {
			return 0, "", "", 0, err
		}
		quoteNumber := apicontract.FormatQuoteNumber(int(draft.CustomerCode), yy, sequence)
		var quoteID int64
		var quoteUUID string
		if draft.QuotationUUID != "" {
			if err := tx.QueryRow(ctx, fmt.Sprintf(`
				INSERT INTO %s.quotations (quote_number, customer_id, uuid)
				VALUES ($1, $2, $3) RETURNING id, uuid::text`, schema),
				quoteNumber, customerID, draft.QuotationUUID).Scan(&quoteID, &quoteUUID); err != nil {
				return 0, "", "", 0, fmt.Errorf("insert quotation: %w", err)
			}
			return quoteID, quoteUUID, quoteNumber, 0, nil
		}
		if err := tx.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO %s.quotations (quote_number, customer_id)
			VALUES ($1, $2) RETURNING id, uuid::text`, schema), quoteNumber, customerID).Scan(&quoteID, &quoteUUID); err != nil {
			return 0, "", "", 0, fmt.Errorf("insert quotation: %w", err)
		}
		return quoteID, quoteUUID, quoteNumber, 0, nil
	}

	var quoteID int64
	var quoteUUID string
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		"SELECT id, uuid::text FROM %s.quotations WHERE quote_number = $1 FOR UPDATE", schema,
	), draft.QuoteNumber).Scan(&quoteID, &quoteUUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) && draft.RevisionNumber == 0 {
			// First revision of a quotation the API already numbered: the
			// insert must reuse the caller's identity, never the column
			// default, or the printed handle and the stored handle diverge
			// (production incident: snapshot QR resolved to nothing).
			newUUID := draft.QuotationUUID
			if newUUID == "" {
				var genErr error
				newUUID, genErr = NewQuotationUUID()
				if genErr != nil {
					return 0, "", "", 0, genErr
				}
			}
			if err := tx.QueryRow(ctx, fmt.Sprintf(`
				INSERT INTO %s.quotations (quote_number, customer_id, uuid)
				VALUES ($1, $2, $3) RETURNING id, uuid::text`, schema), draft.QuoteNumber, customerID, newUUID).Scan(&quoteID, &quoteUUID); err != nil {
				return 0, "", "", 0, fmt.Errorf("insert quotation %q: %w", draft.QuoteNumber, err)
			}
			return quoteID, quoteUUID, draft.QuoteNumber, 0, nil
		}
		return 0, "", "", 0, fmt.Errorf("find quotation %q: %w", draft.QuoteNumber, err)
	}
	var expectedRevision int16
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(MAX(revision_number), -1) + 1
		FROM %s.quotation_revisions WHERE quotation_id = $1`, schema), quoteID).Scan(&expectedRevision); err != nil {
		return 0, "", "", 0, fmt.Errorf("find next revision for quotation %q: %w", draft.QuoteNumber, err)
	}
	if draft.RevisionNumber != expectedRevision {
		return 0, "", "", 0, fmt.Errorf("revision %d is not the next revision; want %d", draft.RevisionNumber, expectedRevision)
	}
	return quoteID, quoteUUID, draft.QuoteNumber, draft.RevisionNumber, nil
}

func nextQuoteSequenceTx(ctx context.Context, tx pgx.Tx, schema string, customerCode, yy int) (int64, error) {
	key := SequenceKey(customerCode, yy)
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.sequence_counters (name, next_value)
		VALUES ($1, 1)
		ON CONFLICT (name) DO NOTHING`, schema), key); err != nil {
		return 0, fmt.Errorf("initialize quote sequence: %w", err)
	}
	var sequence int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.sequence_counters
		SET next_value = next_value + 1
		WHERE name = $1 AND next_value <= $2
		RETURNING next_value - 1`, schema), key, SequenceMax).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("allocate quote sequence: %w", err)
	}
	return sequence, nil
}

// rowDiscountAmount is the money a row's discount came off, derived rather
// than stored (see migration 020) so there is only ever one copy of the truth.
//
// Gated on DiscountEnabled for a reason that is easy to miss: migration 020
// added list_unit_price with DEFAULT 0, so every revision written before it
// reads a list price of zero. Deriving unconditionally would turn those rows
// into a negative discount -- 0 - unit -- a figure that never meant anything.
// A row with no discount of its own has no discount money.
func rowDiscountAmount(enabled bool, listUnit, unit int64, qty int64) int64 {
	if !enabled {
		return 0
	}
	return (listUnit - unit) * qty
}

func nilIfEmptyBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nilIfEmptyString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// AttachDocument stores PDF bytes/path for an existing revision's document
// row. It is how the desktop uploads locally rendered PDFs after a
// server-side export persisted the revision with only the HTML snapshot.
func (s *Store) AttachDocument(ctx context.Context, quotationID int64, revision int16, pdfData []byte, pdfPath string) error {
	pool, err := s.poolOrError()
	if err != nil {
		return err
	}
	if len(pdfData) == 0 && pdfPath == "" {
		return fmt.Errorf("pdf bytes or path are required")
	}
	command, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.quotation_documents d
		SET pdf_data = $1, pdf_path = $2
		FROM %s.quotation_revisions r
		WHERE d.revision_id = r.id
		  AND r.quotation_id = $3 AND r.revision_number = $4`, s.configSchema(), s.configSchema()),
		nilIfEmptyBytes(pdfData), nilIfEmptyString(pdfPath), quotationID, revision)
	if err != nil {
		return fmt.Errorf("attach quotation document: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("quotation %d revision %d not found", quotationID, revision)
	}
	return nil
}
