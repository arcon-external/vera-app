package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type RevisionItem struct {
	ItemNo                int
	Description           string
	Qty                   int64
	UoM                   string
	UnitPrice             int64
	MarginEnabled         bool
	MarginPercent         int16
	MarginAmount          int64
	OriginalTotal         int64
	MarginalizedUnitPrice int64
	MarginalizedTotal     int64
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
}

// RevisionDraft is a complete immutable snapshot. QuoteNumber may be empty
// for a new quotation; the store allocates its sequence in the same
// transaction and starts that quotation at revision 0.
type RevisionDraft struct {
	QuoteNumber     string
	CustomerCode    int16
	CustomerName    string
	CustomerAddress string
	RevisionNumber  int16

	RecipientName    string
	RecipientAddress string
	Subject          string
	Incoterm         string
	ValidityDays     int16
	PaymentTermDays  int16
	LeadTimeDays     int16
	DiscountPercent  int16
	TaxZone          string
	TaxCategory      string
	TaxRateBps       int16

	Items        []RevisionItem
	Totals       RevisionTotals
	HTMLSnapshot string
	PDFData      []byte
	PDFPath      string
	DocumentHash string
}

type QuotationSummary struct {
	ID             int64
	QuoteNumber    string
	RevisionNumber int16
	CustomerName   string
	Subject        string
	CreatedAt      time.Time
	GrandTotal     int64
}

type QuotationRecord struct {
	QuotationSummary
	CustomerCode     int16
	CustomerAddress  string
	RecipientName    string
	RecipientAddress string
	Incoterm         string
	ValidityDays     int16
	PaymentTermDays  int16
	LeadTimeDays     int16
	DiscountPercent  int16
	TaxZone          string
	TaxCategory      string
	TaxRateBps       int16
	Items            []RevisionItem
	Totals           RevisionTotals
	HTMLSnapshot     string
	PDFData          []byte
	PDFPath          string
	// AvailableRevisions lists every revision number for the quotation,
	// newest first, so the UI can offer a revision selector.
	AvailableRevisions []int16
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
		       r.subject, r.created_at, t.marginalized_grand_total
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
		SELECT q.id, q.quote_number, r.revision_number, c.customer_code,
		       c.legal_name, ca.address, r.recipient_name, r.recipient_address,
		       r.subject, r.incoterm, r.validity_days, r.payment_term_days,
		       r.lead_time_days, r.discount_percent, r.tax_zone, r.tax_category,
		       r.tax_rate_bps, r.created_at,
		       d.html_snapshot, d.pdf_data, COALESCE(d.pdf_path, ''),
		       t.original_subtotal, t.original_discount, t.original_grand_total,
		       t.marginalized_subtotal, t.marginalized_discount,
		       t.marginalized_grand_total, t.tax_amount,
		       t.marginalized_grand_after_tax, t.profit_amount, t.nett_profit
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
		&record.DiscountPercent,
		&record.TaxZone,
		&record.TaxCategory,
		&record.TaxRateBps,
		&record.CreatedAt,
		&record.HTMLSnapshot,
		&record.PDFData,
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
	)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("get quotation %d: %w", quotationID, err)
	}
	record.GrandTotal = record.Totals.MarginalizedGrand

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT item_no, description, qty, uom, unit_price, margin_enabled,
		       margin_percent, margin_amount, original_total,
		       marginalized_unit_price, marginalized_total
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
		); err != nil {
			return QuotationRecord{}, fmt.Errorf("scan quotation item: %w", err)
		}
		record.Items = append(record.Items, item)
	}
	if err := rows.Err(); err != nil {
		return QuotationRecord{}, fmt.Errorf("iterate quotation items: %w", err)
	}

	revisionRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT revision_number FROM %s.quotation_revisions
		WHERE quotation_id = $1
		ORDER BY revision_number DESC`, schema), quotationID)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("list quotation revisions: %w", err)
	}
	defer revisionRows.Close()
	for revisionRows.Next() {
		var revisionNumber int16
		if err := revisionRows.Scan(&revisionNumber); err != nil {
			return QuotationRecord{}, fmt.Errorf("scan quotation revision: %w", err)
		}
		record.AvailableRevisions = append(record.AvailableRevisions, revisionNumber)
	}
	if err := revisionRows.Err(); err != nil {
		return QuotationRecord{}, fmt.Errorf("iterate quotation revisions: %w", err)
	}

	return record, nil
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

	quoteID, quoteNumber, revisionNumber, err := ensureQuotation(ctx, tx, schema, draft, customerID)
	if err != nil {
		return QuotationRecord{}, err
	}

	var revisionID int64
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revisions (
			quotation_id, revision_number, recipient_name, recipient_address,
			subject, incoterm, validity_days, payment_term_days, lead_time_days,
			discount_percent, tax_zone, tax_category, tax_rate_bps
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
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
		draft.DiscountPercent,
		draft.TaxZone,
		draft.TaxCategory,
		draft.TaxRateBps,
	).Scan(&revisionID)
	if err != nil {
		return QuotationRecord{}, fmt.Errorf("insert quotation revision %d: %w", revisionNumber, err)
	}

	for _, item := range draft.Items {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.quotation_revision_items (
				revision_id, item_no, description, qty, uom, unit_price,
				margin_enabled, margin_percent, margin_amount, original_total,
				marginalized_unit_price, marginalized_total
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, schema),
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
		); err != nil {
			return QuotationRecord{}, fmt.Errorf("insert quotation revision item %d: %w", item.ItemNo, err)
		}
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.quotation_revision_totals (
			revision_id, original_subtotal, original_discount, original_grand_total,
			marginalized_subtotal, marginalized_discount, marginalized_grand_total,
			tax_amount, marginalized_grand_after_tax, profit_amount, nett_profit
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, schema),
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
		CustomerCode:     draft.CustomerCode,
		CustomerAddress:  draft.CustomerAddress,
		RecipientName:    draft.RecipientName,
		RecipientAddress: draft.RecipientAddress,
		Incoterm:         draft.Incoterm,
		ValidityDays:     draft.ValidityDays,
		PaymentTermDays:  draft.PaymentTermDays,
		LeadTimeDays:     draft.LeadTimeDays,
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

func ensureQuotation(ctx context.Context, tx pgx.Tx, schema string, draft RevisionDraft, customerID int64) (int64, string, int16, error) {
	if draft.QuoteNumber == "" {
		if draft.RevisionNumber != 0 {
			return 0, "", 0, fmt.Errorf("new quotation must start at revision 0")
		}
		sequence, err := nextQuoteSequenceTx(ctx, tx, schema)
		if err != nil {
			return 0, "", 0, err
		}
		quoteNumber := fmt.Sprintf("15%02d%04d", draft.CustomerCode, sequence)
		var quoteID int64
		if err := tx.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO %s.quotations (quote_number, customer_id)
			VALUES ($1, $2) RETURNING id`, schema), quoteNumber, customerID).Scan(&quoteID); err != nil {
			return 0, "", 0, fmt.Errorf("insert quotation: %w", err)
		}
		return quoteID, quoteNumber, 0, nil
	}

	var quoteID int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		"SELECT id FROM %s.quotations WHERE quote_number = $1 FOR UPDATE", schema,
	), draft.QuoteNumber).Scan(&quoteID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) && draft.RevisionNumber == 0 {
			if err := tx.QueryRow(ctx, fmt.Sprintf(`
				INSERT INTO %s.quotations (quote_number, customer_id)
				VALUES ($1, $2) RETURNING id`, schema), draft.QuoteNumber, customerID).Scan(&quoteID); err != nil {
				return 0, "", 0, fmt.Errorf("insert quotation %q: %w", draft.QuoteNumber, err)
			}
			return quoteID, draft.QuoteNumber, 0, nil
		}
		return 0, "", 0, fmt.Errorf("find quotation %q: %w", draft.QuoteNumber, err)
	}
	var expectedRevision int16
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(MAX(revision_number), -1) + 1
		FROM %s.quotation_revisions WHERE quotation_id = $1`, schema), quoteID).Scan(&expectedRevision); err != nil {
		return 0, "", 0, fmt.Errorf("find next revision for quotation %q: %w", draft.QuoteNumber, err)
	}
	if draft.RevisionNumber != expectedRevision {
		return 0, "", 0, fmt.Errorf("revision %d is not the next revision; want %d", draft.RevisionNumber, expectedRevision)
	}
	return quoteID, draft.QuoteNumber, draft.RevisionNumber, nil
}

func nextQuoteSequenceTx(ctx context.Context, tx pgx.Tx, schema string) (int64, error) {
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.sequence_counters (name, next_value)
		VALUES ('quotation', 1)
		ON CONFLICT (name) DO NOTHING`, schema)); err != nil {
		return 0, fmt.Errorf("initialize quote sequence: %w", err)
	}
	var sequence int64
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE %s.sequence_counters
		SET next_value = next_value + 1
		WHERE name = 'quotation' AND next_value <= 9999
		RETURNING next_value - 1`, schema)).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("allocate quote sequence: %w", err)
	}
	return sequence, nil
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
