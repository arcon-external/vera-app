package store

import (
	"context"
	"errors"
	"fmt"
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
	// ErrPurchaseOrderSourceIncomplete means the revision exists but has no
	// stored totals row. A PO copies those printed figures as evidence; without
	// them there is nothing to copy, and creating the PO would invent zeros.
	ErrPurchaseOrderSourceIncomplete = errors.New("purchase order source revision is incomplete")
	// ErrPurchaseOrderSourceOutOfRange means a source amount or printed total
	// is outside 0..MaxExactMinor and could not cross into the editor exactly.
	ErrPurchaseOrderSourceOutOfRange = errors.New("purchase order source amount is outside the supported amount range")
	// ErrPurchaseOrderBadReference refuses a line that names a Quotation item
	// or sub-item that the frozen source revision does not carry.
	ErrPurchaseOrderBadReference = errors.New("purchase order line references an unknown quotation item")
	// ErrPurchaseOrderInvalid refuses values the schema may not express
	// meaningfully (a zero quantity, a negative price, a malformed date).
	ErrPurchaseOrderInvalid = errors.New("purchase order draft is invalid")
	// ErrPurchaseOrderIncomplete is the final-save minimum: a source revision,
	// customer PO number, received date, customer, currency and at least one
	// valid item. A draft may be missing any of them; a saved record may not.
	ErrPurchaseOrderIncomplete = errors.New("purchase order is not ready to be saved")
	// ErrPurchaseOrderNumberTaken is the per-customer uniqueness refusal. The
	// partial unique index is what decides it under concurrency; the store
	// maps that violation here.
	ErrPurchaseOrderNumberTaken = errors.New("this customer already has a purchase order with that number")
	// ErrPurchaseOrderUnresolvedDifference means the final-save review still
	// holds a differing value with no explicit choice, a missing or extra line
	// with no acknowledgement, or two lines claiming one quotation line.
	ErrPurchaseOrderUnresolvedDifference = errors.New("purchase order differences are unresolved")
	// ErrPurchaseOrderPDFConfirmationRequired is the deliberate confirmation a
	// final save needs while no customer PDF is attached (#472 has not
	// shipped). It is not asked of drafts.
	ErrPurchaseOrderPDFConfirmationRequired = errors.New("confirm saving the purchase order without its customer PDF")
	// ErrPurchaseOrderSourceCurrencyMismatch refuses a comparison whose two
	// sides are not in one currency. The PO's currency is copied from the
	// source revision and cannot be edited, so this can only mean a broken
	// source link -- never a conversion.
	ErrPurchaseOrderSourceCurrencyMismatch = errors.New("purchase order currency does not match its source revision")
	// ErrPurchaseOrderSourceMismatch means the stored record no longer agrees
	// with the revision it names (a customer, for instance). Like the currency
	// refusal, it is a broken link rather than something staff entered.
	ErrPurchaseOrderSourceMismatch = errors.New("purchase order source does not match the record")
)

// PurchaseOrderDecision is one explicit difference-review choice for a review
// key. Choice is PurchaseOrderChoicePO or PurchaseOrderChoiceQuotation.
type PurchaseOrderDecision struct {
	Key    string
	Choice string
}

// PurchaseOrderItem is one entered line. QuotationItemNo/QuotationSubNo are
// references by number into the frozen source revision; both are nil for an
// extra line the Quotation never carried.
//
// GrossMinor, DiscountMinor and NetMinor are the stored results of the row's
// own calculation (issue #470): the input is quantity, unit price and the row
// discount, and the three figures are persisted beside them so a reopened PO
// keeps saying what it said. They are not recomputed on read.
type PurchaseOrderItem struct {
	QuotationItemNo *int
	QuotationSubNo  *int
	ID              int64
	LineNumber      int
	Description     string
	QuantityScaled  int64
	UoM             string
	UnitPriceMinor  int64
	DiscountEnabled bool
	DiscountPercent int16
	GrossMinor      int64
	DiscountMinor   int64
	NetMinor        int64
}

// PurchaseOrder is a draft or finalized header plus its lines and the explained
// calculation those lines produce. The calculation is stored, not derived:
// see backend/store/po_calc.go for the stage order and rounding.
type PurchaseOrder struct {
	UUID                   string
	OrgID                  string
	QuoteNumber            string
	State                  string
	CustomerName           string
	BillingAddress         string
	DeliveryAddress        string
	ShipToCode             string
	ContactEmail           string
	ContactPhone           string
	PONumber               string
	Incoterms              string
	Currency               string
	TaxZone                string
	TaxCategory            string
	AdjustmentReason       string
	Items                  []PurchaseOrderItem
	PrintedSubtotalMinor   int64
	PrintedDiscountMinor   int64
	PrintedPpnMinor        int64
	PrintedGrandTotalMinor int64
	SubtotalMinor          int64
	DiscountMinor          int64
	PpnMinor               int64
	GrandTotalMinor        int64
	AdjustmentMinor        int64
	ID                     int64
	QuotationID            int64
	SourceRevision         int16
	CustomerID             int64
	PaymentTerms           string
	CreatedBy              int64
	UpdatedBy              int64
	DiscountPercent        int16
	PpnBps                 int16
	PODate                 *time.Time
	RequestedDeliveryDate  *time.Time
	ReceivedDate           time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	// Finalization (#471). SavedAt/SavedBy are set exactly once, by the
	// transition from draft to saved. PDFAbsentConfirmed records the deliberate
	// confirmation a save needs while no customer PDF exists.
	SavedAt            *time.Time
	SavedBy            *int64
	PDFAbsentConfirmed bool
}

// PurchaseOrderReview is one draft or saved record beside the exact source
// revision it is checked against. DocumentState says which document branch the
// review is in; today every record is without its customer PDF because #472
// has not shipped. The final save asks for the no-PDF confirmation only in
// that branch, so #472 can add an "attached" branch without rewriting it.
type PurchaseOrderReview struct {
	Order         PurchaseOrder
	Source        PurchaseOrderSource
	DocumentState string
}

// The document-state values the review understands. "none" is the only one
// this slice can produce: there is no attachment table yet.
const (
	PurchaseOrderDocumentNone     = "none"
	PurchaseOrderDocumentAttached = "attached"
)

// PurchaseOrderFinalize is the final-save request: the same replaced values a
// draft save carries, plus the difference-review decisions, the explicit
// missing/extra acknowledgements, and the document confirmation.
type PurchaseOrderFinalize struct {
	PurchaseOrderUpdate
	Decisions           []PurchaseOrderDecision
	AcknowledgedMissing []string
	AcknowledgedExtra   []int
	DocumentConfirmed   bool
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

// PurchaseOrderUpdate replaces the editable draft header, the whole-order
// discount and adjustment, and every line. The source link, snapshots, currency
// and tax basis are not editable here.
type PurchaseOrderUpdate struct {
	ID                    int64
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	Incoterms             string
	AdjustmentReason      string
	UpdatedBy             int64
	PaymentTerms          string
	PODate                *time.Time
	ReceivedDate          *time.Time
	RequestedDeliveryDate *time.Time
	Items                 []PurchaseOrderItem
	DiscountPercent       int16
	AdjustmentMinor       int64
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

	lines, err := loadPurchaseOrderSourceLines(ctx, tx, schema, source.RevisionID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	ppnBps, err := PurchaseOrderPPNBps(
		source.TaxZone, source.TaxType, source.ServiceCategory, source.TaxCategory)
	if err != nil {
		return PurchaseOrder{}, err
	}
	items := make([]PurchaseOrderItem, len(lines))
	for index, line := range lines {
		items[index] = line.PurchaseOrderItem()
	}
	items, calculation, err := calculatePurchaseOrderLines(items, source.DiscountPercent, 0, "", ppnBps)
	if err != nil {
		return PurchaseOrder{}, err
	}

	var id int64
	var uuid string
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id,
			customer_name, billing_address, delivery_address, ship_to_code,
			contact_email, contact_phone, po_number, po_date, received_date,
			requested_delivery_date, payment_terms, incoterms, currency,
			tax_zone, tax_category, discount_percent, subtotal_minor,
			discount_minor, ppn_bps, ppn_minor, adjustment_minor,
			adjustment_reason, grand_total_minor, printed_subtotal_minor,
			printed_discount_minor, printed_ppn_minor, printed_grand_total_minor,
			created_by, updated_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,'','','',NULL,NULL,$9,NULL,
			$10,$11,$12,$13,$14,
			$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,
			$27,$27)
		RETURNING id, uuid::text`, schema),
		org.ID, input.QuotationID, input.RevisionNumber, PurchaseOrderStateDraft,
		source.CustomerID, source.CustomerName, source.BillingAddress,
		source.BillingAddress, received, source.PaymentTerms,
		source.Incoterms, source.Currency, source.TaxZone, source.TaxCategory,
		source.DiscountPercent, calculation.SubtotalMinor,
		calculation.DiscountMinor, calculation.PpnBps, calculation.PpnMinor,
		calculation.AdjustmentMinor, calculation.AdjustmentReason,
		calculation.GrandTotalMinor, source.PrintedSubtotalMinor,
		source.PrintedDiscountMinor, source.PrintedPpnMinor,
		source.PrintedGrandTotalMinor, input.CreatedBy,
	).Scan(&id, &uuid)
	if err != nil {
		if isActivePurchaseOrderConflict(err) {
			return PurchaseOrder{}, ErrPurchaseOrderExists
		}
		return PurchaseOrder{}, fmt.Errorf("insert purchase order: %w", err)
	}

	if err := replacePurchaseOrderItems(ctx, tx, schema, id, items); err != nil {
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

// UpdatePurchaseOrderDraft replaces the editable header, the whole-order
// discount and adjustment, and every line of a draft. The exact calculation is
// recomputed from the new inputs and stored with them; only drafts may be
// written, because a saved record's corrections belong to #473.
func (s *Store) UpdatePurchaseOrderDraft(ctx context.Context, input PurchaseOrderUpdate) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.ID < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	input.PONumber = strings.TrimSpace(input.PONumber)
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
	var ppnBps int16
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT state, quotation_id, source_revision, ppn_bps
		FROM %s.purchase_orders
		WHERE id = $1 AND org_id = $2
		FOR UPDATE`, schema), input.ID, org.ID).Scan(
		&state, &quotationID, &sourceRevision, &ppnBps)
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
	lines, calculation, err := calculatePurchaseOrderLines(
		input.Items, input.DiscountPercent, input.AdjustmentMinor,
		input.AdjustmentReason, ppnBps)
	if err != nil {
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
			po_number_key = NULL,
			po_date = $9,
			received_date = COALESCE($10, received_date),
			requested_delivery_date = $11,
			payment_terms = $12,
			incoterms = $13,
			updated_by = $14,
			discount_percent = $15,
			subtotal_minor = $16,
			discount_minor = $17,
			ppn_bps = $18,
			ppn_minor = $19,
			adjustment_minor = $20,
			adjustment_reason = $21,
			grand_total_minor = $22,
			updated_at = NOW()
		WHERE id = $1 AND org_id = $2`, schema),
		input.ID, org.ID, input.BillingAddress, input.DeliveryAddress,
		input.ShipToCode, input.ContactEmail, input.ContactPhone,
		nilIfEmptyString(input.PONumber), dateOrNil(input.PODate),
		dateOrNil(input.ReceivedDate), dateOrNil(input.RequestedDeliveryDate),
		input.PaymentTerms, input.Incoterms, input.UpdatedBy,
		input.DiscountPercent, calculation.SubtotalMinor,
		calculation.DiscountMinor, calculation.PpnBps, calculation.PpnMinor,
		calculation.AdjustmentMinor, calculation.AdjustmentReason,
		calculation.GrandTotalMinor,
	); err != nil {
		return PurchaseOrder{}, fmt.Errorf("update purchase order %d: %w", input.ID, err)
	}
	if err := replacePurchaseOrderItems(ctx, tx, schema, input.ID, lines); err != nil {
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

// loadPurchaseOrderSource reads the exact issued revision and the snapshots a
// new PO inherits. Only stored revisions exist here: an unsaved Quotation
// draft never reaches quotation_revisions, so it can never become a source.
//
// The printed totals are the revision's customer-facing figures. They are
// copied once, as evidence of what the source document said, and never
// recalculated: a PO that disagrees with them is a difference to review (#471),
// not a reason to overwrite the document (parent story 45).
func loadPurchaseOrderSource(ctx context.Context, q purchaseOrderQuerier, schema string, quotationID int64, revision int16) (PurchaseOrderSource, error) {
	source := PurchaseOrderSource{QuotationID: quotationID, RevisionNumber: revision}
	var paymentTermDays int16
	var printedSubtotal, printedDiscount, printedPpn, printedGrand pgtype.Int8
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT q.quote_number, c.id, c.legal_name, r.id, r.recipient_address,
		       r.incoterm, r.payment_term_days, r.tax_zone, r.tax_category, r.currency,
		       r.tax_type, r.service_category, r.discount_percent,
		       t.marginalized_subtotal,
		       t.marginalized_discount,
		       t.ppn_amount,
		       t.customer_grand_total
		FROM %s.quotations q
		JOIN %s.customers c ON c.id = q.customer_id
		JOIN %s.quotation_revisions r
		  ON r.quotation_id = q.id AND r.revision_number = $2
		LEFT JOIN %s.quotation_revision_totals t ON t.revision_id = r.id
		WHERE q.id = $1`, schema, schema, schema, schema),
		quotationID, revision).Scan(
		&source.QuoteNumber,
		&source.CustomerID,
		&source.CustomerName,
		&source.RevisionID,
		&source.BillingAddress,
		&source.Incoterms,
		&paymentTermDays,
		&source.TaxZone,
		&source.TaxCategory,
		&source.Currency,
		&source.TaxType,
		&source.ServiceCategory,
		&source.DiscountPercent,
		&printedSubtotal,
		&printedDiscount,
		&printedPpn,
		&printedGrand,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrderSource{}, ErrPurchaseOrderSourceNotFound
	}
	if err != nil {
		return PurchaseOrderSource{}, fmt.Errorf("load purchase order source %d/%d: %w", quotationID, revision, err)
	}
	if !printedSubtotal.Valid || !printedDiscount.Valid || !printedPpn.Valid || !printedGrand.Valid {
		return PurchaseOrderSource{}, fmt.Errorf(
			"%w: revision %d has no printed totals to copy",
			ErrPurchaseOrderSourceIncomplete, revision)
	}
	source.PrintedSubtotalMinor = printedSubtotal.Int64
	source.PrintedDiscountMinor = printedDiscount.Int64
	source.PrintedPpnMinor = printedPpn.Int64
	source.PrintedGrandTotalMinor = printedGrand.Int64
	for _, printed := range []struct {
		name  string
		value int64
	}{
		{"subtotal", source.PrintedSubtotalMinor},
		{"discount", source.PrintedDiscountMinor},
		{"PPN", source.PrintedPpnMinor},
		{"grand total", source.PrintedGrandTotalMinor},
	} {
		if printed.value < 0 || printed.value > MaxExactMinor {
			return PurchaseOrderSource{}, fmt.Errorf(
				"%w: printed %s %d is outside 0..%d",
				ErrPurchaseOrderSourceOutOfRange, printed.name, printed.value, MaxExactMinor)
		}
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
	Description     string
	UoM             string
	Quantity        int64
	UnitPriceMinor  int64
	SubNo           int
	DiscountPercent int16
	DiscountEnabled bool
}

// loadPurchaseOrderSourceLines turns the customer-facing composition of one
// issued revision into its lines. A package line whose parts are printed
// (show_sub_prices) becomes one line per part; every other line is itself.
// Prices are the frozen list prices the customer document shows, not a
// recalculation. Row discounts come across too, because the printed row's net
// is the list price less its discount, and starting from the list alone would
// silently raise the PO above the quotation.
func loadPurchaseOrderSourceLines(ctx context.Context, q purchaseOrderQuerier, schema string, revisionID int64) ([]PurchaseOrderSourceLine, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT id, item_no, description, qty, uom, list_unit_price, show_sub_prices,
		       discount_enabled, discount_percent
		FROM %s.quotation_revision_items
		WHERE revision_id = $1
		ORDER BY item_no`, schema), revisionID)
	if err != nil {
		return nil, fmt.Errorf("load purchase order source items: %w", err)
	}
	defer rows.Close()

	type sourceItem struct {
		Description     string
		UoM             string
		ID              int64
		ItemNo          int
		Quantity        int64
		UnitPriceMinor  int64
		DiscountPercent int16
		ShowSubPrices   bool
		DiscountEnabled bool
	}
	var items []sourceItem
	for rows.Next() {
		var item sourceItem
		if err := rows.Scan(&item.ID, &item.ItemNo, &item.Description, &item.Quantity,
			&item.UoM, &item.UnitPriceMinor, &item.ShowSubPrices,
			&item.DiscountEnabled, &item.DiscountPercent); err != nil {
			return nil, fmt.Errorf("scan purchase order source item: %w", err)
		}
		if err := checkSourceUnitPrice(item.UnitPriceMinor); err != nil {
			return nil, fmt.Errorf("source item %d: %w", item.ItemNo, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order source items: %w", err)
	}

	subRows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT s.revision_item_id, s.sub_no, s.description, s.qty, s.uom,
		       s.list_unit_price, s.discount_enabled, s.discount_percent
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
			&sub.UoM, &sub.UnitPriceMinor, &sub.DiscountEnabled, &sub.DiscountPercent); err != nil {
			return nil, fmt.Errorf("scan purchase order source sub-item: %w", err)
		}
		if err := checkSourceUnitPrice(sub.UnitPriceMinor); err != nil {
			return nil, fmt.Errorf("source sub-item %d: %w", sub.SubNo, err)
		}
		subs[itemID] = append(subs[itemID], sub)
	}
	if err := subRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order source sub-items: %w", err)
	}

	lines := make([]PurchaseOrderSourceLine, 0, len(items))
	for _, item := range items {
		if item.ShowSubPrices && len(subs[item.ID]) > 0 {
			for _, sub := range subs[item.ID] {
				quantity, err := scaledQuantity(sub.Quantity)
				if err != nil {
					return nil, err
				}
				subNo := sub.SubNo
				lines = append(lines, PurchaseOrderSourceLine{
					QuotationItemNo: item.ItemNo,
					QuotationSubNo:  &subNo,
					Description:     sub.Description,
					QuantityScaled:  quantity,
					UoM:             sub.UoM,
					UnitPriceMinor:  sub.UnitPriceMinor,
					DiscountEnabled: sub.DiscountEnabled,
					DiscountPercent: sub.DiscountPercent,
				})
			}
			continue
		}
		quantity, err := scaledQuantity(item.Quantity)
		if err != nil {
			return nil, err
		}
		lines = append(lines, PurchaseOrderSourceLine{
			QuotationItemNo: item.ItemNo,
			Description:     item.Description,
			QuantityScaled:  quantity,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
		})
	}
	return lines, nil
}

// loadPurchaseOrderSourceDetail is the review's source: the header wording and
// printed totals plus the exact customer-facing lines.
func loadPurchaseOrderSourceDetail(ctx context.Context, q purchaseOrderQuerier, schema string, quotationID int64, revision int16) (PurchaseOrderSource, error) {
	source, err := loadPurchaseOrderSource(ctx, q, schema, quotationID, revision)
	if err != nil {
		return PurchaseOrderSource{}, err
	}
	source.Lines, err = loadPurchaseOrderSourceLines(ctx, q, schema, source.RevisionID)
	if err != nil {
		return PurchaseOrderSource{}, err
	}
	return source, nil
}

// PurchaseOrderItem is the starting line a fresh draft stores. The match
// reference is the source line's own item/sub number, so a draft begins
// explicitly matched rather than inferred from position.
func (line PurchaseOrderSourceLine) PurchaseOrderItem() PurchaseOrderItem {
	itemNo := line.QuotationItemNo
	item := PurchaseOrderItem{
		QuotationItemNo: &itemNo,
		Description:     line.Description,
		QuantityScaled:  line.QuantityScaled,
		UoM:             line.UoM,
		UnitPriceMinor:  line.UnitPriceMinor,
		DiscountEnabled: line.DiscountEnabled,
		DiscountPercent: line.DiscountPercent,
	}
	if line.QuotationSubNo != nil {
		subNo := *line.QuotationSubNo
		item.QuotationSubNo = &subNo
	}
	return item
}

func scaledQuantity(quantity int64) (int64, error) {
	if quantity <= 0 {
		return 0, fmt.Errorf("%w: source quantity must be positive", ErrPurchaseOrderInvalid)
	}
	if quantity > MaxExactMinor/QuantityScale {
		return 0, fmt.Errorf(
			"%w: source quantity %d exceeds the supported amount range",
			ErrPurchaseOrderSourceOutOfRange, quantity)
	}
	return quantity * QuantityScale, nil
}

// checkSourceUnitPrice keeps a frozen revision's list price inside the range
// the editor can read back exactly. It is a source-integrity refusal rather
// than a user-input error: nothing the staff member typed caused it.
func checkSourceUnitPrice(value int64) error {
	if value < 0 || value > MaxExactMinor {
		return fmt.Errorf(
			"%w: source unit price %d is outside 0..%d",
			ErrPurchaseOrderSourceOutOfRange, value, MaxExactMinor)
	}
	return nil
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

// calculatePurchaseOrderLines runs the exact calculator (po_calc.go) over the
// entered rows and returns those rows with their stored money figures filled,
// alongside the header totals. The inputs are mutated in place-ish (a new slice
// is returned) so the caller can persist exactly what was calculated; nothing
// here recalculates on read.
func calculatePurchaseOrderLines(items []PurchaseOrderItem, discountPercent int16, adjustment int64, reason string, ppnBps int16) ([]PurchaseOrderItem, PurchaseOrderCalc, error) {
	input := PurchaseOrderCalcInput{
		Lines:            make([]PurchaseOrderCalcLine, len(items)),
		DiscountPercent:  discountPercent,
		PpnBps:           ppnBps,
		AdjustmentMinor:  adjustment,
		AdjustmentReason: reason,
	}
	for index, item := range items {
		input.Lines[index] = PurchaseOrderCalcLine{
			QuantityScaled:  item.QuantityScaled,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
		}
	}
	calculation, err := CalculatePurchaseOrder(input)
	if err != nil {
		return nil, PurchaseOrderCalc{}, err
	}
	lines := make([]PurchaseOrderItem, len(items))
	for index, item := range items {
		calculated := calculation.Lines[index]
		item.GrossMinor = calculated.GrossMinor
		item.DiscountMinor = calculated.DiscountMinor
		item.NetMinor = calculated.NetMinor
		lines[index] = item
	}
	return lines, calculation, nil
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
				quotation_sub_no, quantity_scaled, uom, unit_price_minor,
				discount_enabled, discount_percent, gross_minor,
				discount_minor, net_minor
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, schema),
			orderID, lineNumber, item.Description, item.QuotationItemNo,
			item.QuotationSubNo, item.QuantityScaled, item.UoM, item.UnitPriceMinor,
			item.DiscountEnabled, item.DiscountPercent, item.GrossMinor,
			item.DiscountMinor, item.NetMinor,
		); err != nil {
			return fmt.Errorf("insert purchase order line %d: %w", lineNumber, err)
		}
	}
	return nil
}

func getPurchaseOrder(ctx context.Context, q purchaseOrderQuerier, schema, orgID string, id int64) (PurchaseOrder, error) {
	var order PurchaseOrder
	var poDate, requestedDelivery pgtype.Date
	var savedAt pgtype.Timestamptz
	var savedBy pgtype.Int8
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT po.id, po.uuid::text, po.org_id::text, po.quotation_id,
		       po.source_revision, q.quote_number, po.state, po.customer_id,
		       po.customer_name, po.billing_address, po.delivery_address,
		       po.ship_to_code, po.contact_email, po.contact_phone,
		       COALESCE(po.po_number, ''), po.po_date, po.received_date,
		       po.requested_delivery_date, po.payment_terms, po.incoterms,
		       po.currency, po.tax_zone, po.tax_category, po.created_by,
		       po.updated_by, po.created_at, po.updated_at,
		       po.subtotal_minor, po.discount_minor, po.ppn_bps, po.ppn_minor,
		       po.adjustment_minor, po.adjustment_reason, po.grand_total_minor,
		       po.discount_percent, po.printed_subtotal_minor,
		       po.printed_discount_minor, po.printed_ppn_minor,
		       po.printed_grand_total_minor, po.saved_at, po.saved_by,
		       (po.pdf_absent_confirmed_at IS NOT NULL
		        AND po.pdf_absent_confirmed_by IS NOT NULL)
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
		&order.SubtotalMinor,
		&order.DiscountMinor,
		&order.PpnBps,
		&order.PpnMinor,
		&order.AdjustmentMinor,
		&order.AdjustmentReason,
		&order.GrandTotalMinor,
		&order.DiscountPercent,
		&order.PrintedSubtotalMinor,
		&order.PrintedDiscountMinor,
		&order.PrintedPpnMinor,
		&order.PrintedGrandTotalMinor,
		&savedAt,
		&savedBy,
		&order.PDFAbsentConfirmed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("load purchase order %d: %w", id, err)
	}
	if savedAt.Valid {
		value := savedAt.Time
		order.SavedAt = &value
	}
	if savedBy.Valid {
		value := savedBy.Int64
		order.SavedBy = &value
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
		       quantity_scaled, uom, unit_price_minor, discount_enabled,
		       discount_percent, gross_minor, discount_minor, net_minor
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
			&subNo, &item.QuantityScaled, &item.UoM, &item.UnitPriceMinor,
			&item.DiscountEnabled, &item.DiscountPercent, &item.GrossMinor,
			&item.DiscountMinor, &item.NetMinor); err != nil {
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
