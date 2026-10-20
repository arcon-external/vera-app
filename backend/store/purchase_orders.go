package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Issue #468: a manual Purchase Order draft. It is born from one exact issued
// Quotation revision, owns a snapshot of the customer-facing header, and is
// editable only while it is a draft. Pricing, discounts, differences and
// attachments belong to later slices; this file stores entered values, never
// recalculated ones.
const (
	PurchaseOrderStateDraft = "draft"
	PurchaseOrderStateSaved = "saved"
)

// QuantityScale is the fixed four-decimal representation of a PO quantity.
// Every quantity travels as an integer count of 1/10000 units so no float and
// no locale-dependent decimal string enters the database.
const QuantityScale int64 = 10000

// maxPaymentTermsLength bounds the free-text terms to the migration's
// char_length CHECK. The unit is Unicode characters -- not UTF-8 bytes and not
// UTF-16 units -- so "500" means the same thing in Postgres, this store and
// the editor. Long enough for a real commercial sentence, short enough that a
// paste cannot store a page of text in a header field.
const maxPaymentTermsLength = 500

var (
	// ErrPurchaseOrderNotFound also answers a PO owned by another
	// organization: the row exists but is outside the caller's boundary, and
	// saying so would confirm another tenant's id.
	ErrPurchaseOrderNotFound = errors.New("purchase order not found")
	// ErrPurchaseOrderExists is the one-active-record-per-Quotation refusal.
	ErrPurchaseOrderExists = errors.New("quotation already has an active purchase order")
	// ErrPurchaseOrderNotDraft refuses a write to a finalized record.
	ErrPurchaseOrderNotDraft = errors.New("purchase order is not a draft")
	// ErrPurchaseOrderSourceNotFound means the selected Quotation or revision
	// is gone (or was never issued).
	ErrPurchaseOrderSourceNotFound = errors.New("purchase order source quotation or revision not found")
	// ErrPurchaseOrderBadReference refuses a line that names a Quotation item
	// or sub-item that the frozen source revision does not carry.
	ErrPurchaseOrderBadReference = errors.New("purchase order line references an unknown quotation item")
	// ErrPurchaseOrderInvalid refuses values the schema may not express
	// meaningfully (a zero quantity, a negative price, a malformed date).
	ErrPurchaseOrderInvalid = errors.New("purchase order draft is invalid")
)

// PurchaseOrderItem is one entered line. QuotationItemNo/QuotationSubNo are
// references by number into the frozen source revision; both are nil for an
// extra line the Quotation never carried.
type PurchaseOrderItem struct {
	QuotationItemNo *int
	QuotationSubNo  *int
	ID              int64
	LineNumber      int
	Description     string
	QuantityScaled  int64
	UoM             string
	UnitPriceMinor  int64
}

// PurchaseOrder is a draft or finalized header plus its lines.
type PurchaseOrder struct {
	UUID                  string
	OrgID                 string
	QuoteNumber           string
	State                 string
	CustomerName          string
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	Incoterms             string
	Currency              string
	TaxZone               string
	TaxCategory           string
	Items                 []PurchaseOrderItem
	ID                    int64
	QuotationID           int64
	SourceRevision        int16
	CustomerID            int64
	PaymentTerms          string
	CreatedBy             int64
	UpdatedBy             int64
	PODate                *time.Time
	RequestedDeliveryDate *time.Time
	ReceivedDate          time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// PurchaseOrderCreate is the server-side creation input: the exact source pair
// and, optionally, a caller-chosen received date. Everything else is copied
// from the revision inside the transaction.
type PurchaseOrderCreate struct {
	ReceivedDate   *time.Time
	QuotationID    int64
	RevisionNumber int16
	CreatedBy      int64
}

// PurchaseOrderUpdate replaces the editable draft header and every line. The
// source link, snapshots, currency and tax basis are not editable here.
type PurchaseOrderUpdate struct {
	ID                    int64
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	Incoterms             string
	UpdatedBy             int64
	PaymentTerms          string
	PODate                *time.Time
	ReceivedDate          *time.Time
	RequestedDeliveryDate *time.Time
	Items                 []PurchaseOrderItem
}

type purchaseOrderQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// CreatePurchaseOrderDraft creates the one active PO for a Quotation from the
// exact issued revision. The unique index on quotation_id is the concurrency
// guard: two simultaneous creations race in Postgres, one wins, and the other
// is refused with ErrPurchaseOrderExists rather than silently overwritten.
func (s *Store) CreatePurchaseOrderDraft(ctx context.Context, input PurchaseOrderCreate) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.QuotationID < 1 {
		return PurchaseOrder{}, fmt.Errorf("%w: quotation id is required", ErrPurchaseOrderInvalid)
	}
	if input.RevisionNumber < 0 {
		return PurchaseOrder{}, fmt.Errorf("%w: revision must not be negative", ErrPurchaseOrderInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	schema := s.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("begin purchase order transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	source, err := loadPurchaseOrderSource(ctx, tx, schema, input.QuotationID, input.RevisionNumber)
	if err != nil {
		return PurchaseOrder{}, err
	}

	received := normalizeDate(input.ReceivedDate)
	if received.IsZero() {
		received = companyToday(time.Now())
	}

	var id int64
	var uuid string
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id,
			customer_name, billing_address, delivery_address, ship_to_code,
			contact_email, contact_phone, po_number, po_date, received_date,
			requested_delivery_date, payment_terms, incoterms, currency,
			tax_zone, tax_category, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'','','',NULL,NULL,$9,NULL,$10,$11,$12,$13,$14,$15,$15)
		RETURNING id, uuid::text`, schema),
		org.ID, input.QuotationID, input.RevisionNumber, PurchaseOrderStateDraft,
		source.CustomerID, source.CustomerName, source.BillingAddress,
		source.BillingAddress, received, source.PaymentTerms,
		source.Incoterm, source.Currency, source.TaxZone, source.TaxCategory,
		input.CreatedBy,
	).Scan(&id, &uuid)
	if err != nil {
		if isActivePurchaseOrderConflict(err) {
			return PurchaseOrder{}, ErrPurchaseOrderExists
		}
		return PurchaseOrder{}, fmt.Errorf("insert purchase order: %w", err)
	}

	lines, err := copyPurchaseOrderLines(ctx, tx, schema, source.RevisionID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := replacePurchaseOrderItems(ctx, tx, schema, id, lines); err != nil {
		return PurchaseOrder{}, err
	}

	created, err := getPurchaseOrder(ctx, tx, schema, org.ID, id)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit purchase order draft: %w", err)
	}
	return created, nil
}

// GetPurchaseOrder reopens one PO by id inside the caller's organization.
func (s *Store) GetPurchaseOrder(ctx context.Context, id int64) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	return getPurchaseOrder(ctx, pool, s.configSchema(), org.ID, id)
}

// UpdatePurchaseOrderDraft replaces the editable header and every line of a
// draft. Only drafts may be written; a saved record is immutable here because
// its corrections belong to the explicit correction slice (#473).
func (s *Store) UpdatePurchaseOrderDraft(ctx context.Context, input PurchaseOrderUpdate) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.ID < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	input.PaymentTerms = strings.TrimSpace(input.PaymentTerms)
	if utf8.RuneCountInString(input.PaymentTerms) > maxPaymentTermsLength {
		return PurchaseOrder{}, fmt.Errorf("%w: payment terms must be at most %d characters",
			ErrPurchaseOrderInvalid, maxPaymentTermsLength)
	}
	for index, item := range input.Items {
		if item.QuantityScaled <= 0 {
			return PurchaseOrder{}, fmt.Errorf("%w: line %d quantity must be positive", ErrPurchaseOrderInvalid, index+1)
		}
		if item.UnitPriceMinor < 0 {
			return PurchaseOrder{}, fmt.Errorf("%w: line %d unit price must not be negative", ErrPurchaseOrderInvalid, index+1)
		}
		if item.QuotationSubNo != nil && item.QuotationItemNo == nil {
			return PurchaseOrder{}, fmt.Errorf("%w: line %d names a sub-item without its item", ErrPurchaseOrderInvalid, index+1)
		}
	}

	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	schema := s.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("begin purchase order transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var state string
	var quotationID int64
	var sourceRevision int16
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT state, quotation_id, source_revision
		FROM %s.purchase_orders
		WHERE id = $1 AND org_id = $2
		FOR UPDATE`, schema), input.ID, org.ID).Scan(&state, &quotationID, &sourceRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("lock purchase order %d: %w", input.ID, err)
	}
	if state != PurchaseOrderStateDraft {
		return PurchaseOrder{}, ErrPurchaseOrderNotDraft
	}
	if err := validatePurchaseOrderReferences(ctx, tx, schema, quotationID, sourceRevision, input.Items); err != nil {
		return PurchaseOrder{}, err
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET
			billing_address = $3,
			delivery_address = $4,
			ship_to_code = $5,
			contact_email = $6,
			contact_phone = $7,
			po_number = $8,
			po_date = $9,
			received_date = COALESCE($10, received_date),
			requested_delivery_date = $11,
			payment_terms = $12,
			incoterms = $13,
			updated_by = $14,
			updated_at = NOW()
		WHERE id = $1 AND org_id = $2`, schema),
		input.ID, org.ID, input.BillingAddress, input.DeliveryAddress,
		input.ShipToCode, input.ContactEmail, input.ContactPhone,
		nilIfEmptyString(input.PONumber), dateOrNil(input.PODate),
		dateOrNil(input.ReceivedDate), dateOrNil(input.RequestedDeliveryDate),
		input.PaymentTerms, input.Incoterms, input.UpdatedBy,
	); err != nil {
		return PurchaseOrder{}, fmt.Errorf("update purchase order %d: %w", input.ID, err)
	}
	if err := replacePurchaseOrderItems(ctx, tx, schema, input.ID, input.Items); err != nil {
		return PurchaseOrder{}, err
	}

	updated, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit purchase order draft: %w", err)
	}
	return updated, nil
}

type purchaseOrderSource struct {
	QuoteNumber    string
	CustomerName   string
	BillingAddress string
	Incoterm       string
	Currency       string
	TaxZone        string
	TaxCategory    string
	CustomerID     int64
	RevisionID     int64
	PaymentTerms   string
}

// loadPurchaseOrderSource reads the exact issued revision and the snapshots a
// new PO inherits. Only stored revisions exist here: an unsaved Quotation
// draft never reaches quotation_revisions, so it can never become a source.
func loadPurchaseOrderSource(ctx context.Context, q purchaseOrderQuerier, schema string, quotationID int64, revision int16) (purchaseOrderSource, error) {
	var source purchaseOrderSource
	var paymentTermDays int16
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT q.quote_number, c.id, c.legal_name, r.id, r.recipient_address,
		       r.incoterm, r.payment_term_days, r.tax_zone, r.tax_category, r.currency
		FROM %s.quotations q
		JOIN %s.customers c ON c.id = q.customer_id
		JOIN %s.quotation_revisions r
		  ON r.quotation_id = q.id AND r.revision_number = $2
		WHERE q.id = $1`, schema, schema, schema),
		quotationID, revision).Scan(
		&source.QuoteNumber,
		&source.CustomerID,
		&source.CustomerName,
		&source.RevisionID,
		&source.BillingAddress,
		&source.Incoterm,
		&paymentTermDays,
		&source.TaxZone,
		&source.TaxCategory,
		&source.Currency,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return purchaseOrderSource{}, ErrPurchaseOrderSourceNotFound
	}
	if err != nil {
		return purchaseOrderSource{}, fmt.Errorf("load purchase order source %d/%d: %w", quotationID, revision, err)
	}
	source.PaymentTerms = quotationPaymentTerms(paymentTermDays)
	return source, nil
}

// quotationPaymentTerms renders the issued revision's payment term as the
// wording a PO starts from. A Quotation stores a day count, so the seed is
// that count in words ("45 days"); the PO field itself is free text because
// the commercial wording can be "50% advance" or "30 days after delivery" and
// a day count cannot represent either. The draft is editable from there.
func quotationPaymentTerms(days int16) string {
	if days <= 0 {
		return ""
	}
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

type purchaseOrderSourceSub struct {
	Description    string
	UoM            string
	SubNo          int
	Quantity       int64
	UnitPriceMinor int64
}

// copyPurchaseOrderLines turns the customer-facing composition of one issued
// revision into starting lines. A package line whose parts are printed
// (show_sub_prices) becomes one line per part; every other line is copied as
// itself. Prices are the frozen list prices the customer document shows, not
// a recalculation -- exact PO maths is #470's slice.
func copyPurchaseOrderLines(ctx context.Context, q purchaseOrderQuerier, schema string, revisionID int64) ([]PurchaseOrderItem, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT id, item_no, description, qty, uom, list_unit_price, show_sub_prices
		FROM %s.quotation_revision_items
		WHERE revision_id = $1
		ORDER BY item_no`, schema), revisionID)
	if err != nil {
		return nil, fmt.Errorf("load purchase order source items: %w", err)
	}
	defer rows.Close()

	type sourceItem struct {
		Description    string
		UoM            string
		ID             int64
		ItemNo         int
		Quantity       int64
		UnitPriceMinor int64
		ShowSubPrices  bool
	}
	var items []sourceItem
	for rows.Next() {
		var item sourceItem
		if err := rows.Scan(&item.ID, &item.ItemNo, &item.Description, &item.Quantity,
			&item.UoM, &item.UnitPriceMinor, &item.ShowSubPrices); err != nil {
			return nil, fmt.Errorf("scan purchase order source item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order source items: %w", err)
	}

	subRows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT s.revision_item_id, s.sub_no, s.description, s.qty, s.uom, s.list_unit_price
		FROM %s.quotation_revision_sub_items s
		JOIN %s.quotation_revision_items i ON i.id = s.revision_item_id
		WHERE i.revision_id = $1
		ORDER BY i.item_no, s.sub_no`, schema, schema), revisionID)
	if err != nil {
		return nil, fmt.Errorf("load purchase order source sub-items: %w", err)
	}
	defer subRows.Close()
	subs := map[int64][]purchaseOrderSourceSub{}
	for subRows.Next() {
		var itemID int64
		var sub purchaseOrderSourceSub
		if err := subRows.Scan(&itemID, &sub.SubNo, &sub.Description, &sub.Quantity,
			&sub.UoM, &sub.UnitPriceMinor); err != nil {
			return nil, fmt.Errorf("scan purchase order source sub-item: %w", err)
		}
		subs[itemID] = append(subs[itemID], sub)
	}
	if err := subRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order source sub-items: %w", err)
	}

	lines := make([]PurchaseOrderItem, 0, len(items))
	for _, item := range items {
		if item.ShowSubPrices && len(subs[item.ID]) > 0 {
			for _, sub := range subs[item.ID] {
				quantity, err := scaledQuantity(sub.Quantity)
				if err != nil {
					return nil, err
				}
				itemNo, subNo := item.ItemNo, sub.SubNo
				lines = append(lines, PurchaseOrderItem{
					Description:     sub.Description,
					QuotationItemNo: &itemNo,
					QuotationSubNo:  &subNo,
					QuantityScaled:  quantity,
					UoM:             sub.UoM,
					UnitPriceMinor:  sub.UnitPriceMinor,
				})
			}
			continue
		}
		quantity, err := scaledQuantity(item.Quantity)
		if err != nil {
			return nil, err
		}
		itemNo := item.ItemNo
		lines = append(lines, PurchaseOrderItem{
			Description:     item.Description,
			QuotationItemNo: &itemNo,
			QuantityScaled:  quantity,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
		})
	}
	return lines, nil
}

func scaledQuantity(quantity int64) (int64, error) {
	if quantity <= 0 {
		return 0, fmt.Errorf("%w: source quantity must be positive", ErrPurchaseOrderInvalid)
	}
	if quantity > math.MaxInt64/QuantityScale {
		return 0, fmt.Errorf("%w: source quantity is too large", ErrPurchaseOrderInvalid)
	}
	return quantity * QuantityScale, nil
}

// validatePurchaseOrderReferences ensures every non-nil line reference exists
// in the frozen source revision. Extras are allowed; a stale or invented link
// is not.
func validatePurchaseOrderReferences(ctx context.Context, q purchaseOrderQuerier, schema string, quotationID int64, revision int16, items []PurchaseOrderItem) error {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT ri.item_no, 0 AS sub_no
		FROM %s.quotation_revision_items ri
		JOIN %s.quotation_revisions r ON r.id = ri.revision_id
		WHERE r.quotation_id = $1 AND r.revision_number = $2
		UNION ALL
		SELECT ri.item_no, s.sub_no
		FROM %s.quotation_revision_sub_items s
		JOIN %s.quotation_revision_items ri ON ri.id = s.revision_item_id
		JOIN %s.quotation_revisions r ON r.id = ri.revision_id
		WHERE r.quotation_id = $1 AND r.revision_number = $2`, schema, schema, schema, schema, schema),
		quotationID, revision)
	if err != nil {
		return fmt.Errorf("load purchase order source references: %w", err)
	}
	defer rows.Close()
	known := map[int]map[int]bool{}
	for rows.Next() {
		var itemNo, subNo int
		if err := rows.Scan(&itemNo, &subNo); err != nil {
			return fmt.Errorf("scan purchase order source reference: %w", err)
		}
		if known[itemNo] == nil {
			known[itemNo] = map[int]bool{}
		}
		known[itemNo][subNo] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate purchase order source references: %w", err)
	}

	for index, item := range items {
		if item.QuotationItemNo == nil {
			continue
		}
		subs := known[*item.QuotationItemNo]
		if subs == nil {
			return fmt.Errorf("%w: line %d names item %d", ErrPurchaseOrderBadReference, index+1, *item.QuotationItemNo)
		}
		if item.QuotationSubNo == nil {
			if !subs[0] {
				return fmt.Errorf("%w: line %d names item %d, which is not a line of the revision",
					ErrPurchaseOrderBadReference, index+1, *item.QuotationItemNo)
			}
			continue
		}
		if !subs[*item.QuotationSubNo] {
			return fmt.Errorf("%w: line %d names item %d sub-item %d",
				ErrPurchaseOrderBadReference, index+1, *item.QuotationItemNo, *item.QuotationSubNo)
		}
	}
	return nil
}

func replacePurchaseOrderItems(ctx context.Context, q purchaseOrderQuerier, schema string, orderID int64, items []PurchaseOrderItem) error {
	if _, err := q.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %s.purchase_order_items WHERE purchase_order_id = $1", schema), orderID); err != nil {
		return fmt.Errorf("clear purchase order lines: %w", err)
	}
	for index, item := range items {
		lineNumber := index + 1
		if _, err := q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_items (
				purchase_order_id, line_number, description, quotation_item_no,
				quotation_sub_no, quantity_scaled, uom, unit_price_minor
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, schema),
			orderID, lineNumber, item.Description, item.QuotationItemNo,
			item.QuotationSubNo, item.QuantityScaled, item.UoM, item.UnitPriceMinor,
		); err != nil {
			return fmt.Errorf("insert purchase order line %d: %w", lineNumber, err)
		}
	}
	return nil
}

func getPurchaseOrder(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, id int64) (PurchaseOrder, error) {
	var order PurchaseOrder
	var poDate, requestedDelivery pgtype.Date
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT po.id, po.uuid::text, po.org_id::text, po.quotation_id,
		       po.source_revision, q.quote_number, po.state, po.customer_id,
		       po.customer_name, po.billing_address, po.delivery_address,
		       po.ship_to_code, po.contact_email, po.contact_phone,
		       COALESCE(po.po_number, ''), po.po_date, po.received_date,
		       po.requested_delivery_date, po.payment_terms, po.incoterms,
		       po.currency, po.tax_zone, po.tax_category, po.created_by,
		       po.updated_by, po.created_at, po.updated_at
		FROM %s.purchase_orders po
		JOIN %s.quotations q ON q.id = po.quotation_id
		WHERE po.id = $1 AND po.org_id = $2`, schema, schema), id, orgID).Scan(
		&order.ID,
		&order.UUID,
		&order.OrgID,
		&order.QuotationID,
		&order.SourceRevision,
		&order.QuoteNumber,
		&order.State,
		&order.CustomerID,
		&order.CustomerName,
		&order.BillingAddress,
		&order.DeliveryAddress,
		&order.ShipToCode,
		&order.ContactEmail,
		&order.ContactPhone,
		&order.PONumber,
		&poDate,
		&order.ReceivedDate,
		&requestedDelivery,
		&order.PaymentTerms,
		&order.Incoterms,
		&order.Currency,
		&order.TaxZone,
		&order.TaxCategory,
		&order.CreatedBy,
		&order.UpdatedBy,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("load purchase order %d: %w", id, err)
	}
	if poDate.Valid {
		value := poDate.Time
		order.PODate = &value
	}
	if requestedDelivery.Valid {
		value := requestedDelivery.Time
		order.RequestedDeliveryDate = &value
	}

	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT id, line_number, description, quotation_item_no, quotation_sub_no,
		       quantity_scaled, uom, unit_price_minor
		FROM %s.purchase_order_items
		WHERE purchase_order_id = $1
		ORDER BY line_number`, schema), id)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("load purchase order lines: %w", err)
	}
	defer rows.Close()
	order.Items = make([]PurchaseOrderItem, 0, 8)
	for rows.Next() {
		var item PurchaseOrderItem
		var itemNo, subNo pgtype.Int4
		if err := rows.Scan(&item.ID, &item.LineNumber, &item.Description, &itemNo,
			&subNo, &item.QuantityScaled, &item.UoM, &item.UnitPriceMinor); err != nil {
			return PurchaseOrder{}, fmt.Errorf("scan purchase order line: %w", err)
		}
		if itemNo.Valid {
			value := int(itemNo.Int32)
			item.QuotationItemNo = &value
		}
		if subNo.Valid {
			value := int(subNo.Int32)
			item.QuotationSubNo = &value
		}
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		return PurchaseOrder{}, fmt.Errorf("iterate purchase order lines: %w", err)
	}
	return order, nil
}

// normalizeDate turns a caller's date into a midnight value Postgres stores as
// DATE. A nil stays nil; the SQL keeps the existing received date when an
// update omits it.
func normalizeDate(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

// dateOrNil passes NULL rather than Go's zero time when a caller omits an
// optional DATE; the zero value would store year 1.
func dateOrNil(value *time.Time) any {
	if value == nil {
		return nil
	}
	return normalizeDate(value)
}

func isActivePurchaseOrderConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "purchase_orders_active_quotation_idx"
}
