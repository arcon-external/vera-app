package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Issue #473: traceable corrections and recoverable overwrites.
//
// A draft may be replaced by a normal save. A saved record may only be replaced
// through CorrectPurchaseOrder, which requires the reason staff typed and keeps
// an immutable snapshot of the values it replaced. Both paths state the version
// they loaded; a stale base is refused by name rather than applied.
//
// Nothing here can change the record's identity: the source link, customer,
// currency, tax basis, UUID and confirmation number are not writable through a
// correction, and the SQL never names them. A correction of a row that no
// longer exists is not found -- there is no upsert anywhere in this file.

const (
	// PurchaseOrderCorrectionKindCorrection is a staff correction of a saved
	// record; its reason is required.
	PurchaseOrderCorrectionKindCorrection = "correction"
	// PurchaseOrderCorrectionKindDraftOverwrite is the recoverable prior draft
	// an explicit overwrite kept. Its reason is the fixed label below, because
	// the warning is about the concurrency, not about a commercial change.
	PurchaseOrderCorrectionKindDraftOverwrite = "draft_overwrite"
	// PurchaseOrderDraftOverwriteReason is what the history shows for a prior
	// draft kept by an explicit overwrite.
	PurchaseOrderDraftOverwriteReason = "Draft overwritten after a concurrent save"
	// maxCorrectionReasonLength bounds the reason to the migration's
	// char_length CHECK. It is shown in the history panel, not kept as an
	// internal note.
	maxCorrectionReasonLength = 500
)

var (
	// ErrPurchaseOrderVersionRequired refuses a write that did not state the
	// version it loaded. Without it a stale editor would silently overwrite a
	// newer save, which is exactly what this slice exists to prevent.
	ErrPurchaseOrderVersionRequired = errors.New("purchase order version is required")
	// ErrPurchaseOrderVersionConflict is the named refusal a stale write gets.
	// The concrete *PurchaseOrderVersionConflictError carries who saved, when
	// and which version is current, so the warning can name them.
	ErrPurchaseOrderVersionConflict = errors.New("purchase order was saved since it was loaded")
	// ErrPurchaseOrderCorrectionReasonRequired is the correction rule: a saved
	// record is replaced only with a reason staff can read later.
	ErrPurchaseOrderCorrectionReasonRequired = errors.New("a correction reason is required")
)

// PurchaseOrderVersionConflictError is the stale-write refusal. The store reads
// the current version and the last writer inside the locked transaction, so the
// caller can warn without a second, raceable read.
type PurchaseOrderVersionConflictError struct {
	UpdatedAt      time.Time
	CurrentState   string
	UpdatedByName  string
	CurrentVersion int64
	UpdatedBy      int64
}

func (e *PurchaseOrderVersionConflictError) Error() string {
	actor := strings.TrimSpace(e.UpdatedByName)
	if actor == "" {
		actor = fmt.Sprintf("user %d", e.UpdatedBy)
	}
	return fmt.Sprintf(
		"%s: %s saved version %d at %s",
		ErrPurchaseOrderVersionConflict, actor, e.CurrentVersion,
		e.UpdatedAt.Format(time.RFC3339))
}

func (e *PurchaseOrderVersionConflictError) Unwrap() error {
	return ErrPurchaseOrderVersionConflict
}

// PurchaseOrderCorrectionInput replaces a saved record. The embedded update is
// the same payload a draft save carries (and deliberately has no field for the
// source link, customer, currency, tax basis, UUID or confirmation number);
// Reason is the staff member's explanation. Overwrite records that the caller
// deliberately replaced a version it was warned about, so the replaced values
// are kept as history.
type PurchaseOrderCorrectionInput struct {
	PurchaseOrderUpdate
	Reason string
}

// PurchaseOrderCorrectionAttachment is the customer PO PDF version that was
// active when the snapshot was taken. It identifies a retained document row by
// the pair the schema never reuses.
type PurchaseOrderCorrectionAttachment struct {
	SHA256   string
	Filename string
	Version  int32
}

// PurchaseOrderCorrectionItem is one line of a retained snapshot. The fields
// mirror PurchaseOrderItem: this is what the record said, not a recalculation.
type PurchaseOrderCorrectionItem struct {
	QuotationItemNo *int
	QuotationSubNo  *int
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

// PurchaseOrderCorrection is one immutable snapshot plus who replaced it, when
// and why. It is self-contained: quote number, customer name and currency are
// copied, so a later repair of the live record (#475) cannot change what the
// history says the corrected version was.
type PurchaseOrderCorrection struct {
	Attachment *PurchaseOrderCorrectionAttachment
	// Repair is the old and new source one source-link repair recorded (#475).
	// Nil for every other kind; the snapshot fields above remain the values the
	// record carried before the write either way.
	Repair                *PurchaseOrderSourceRepair
	CreatedAt             time.Time
	UUID                  string
	Kind                  string
	State                 string
	Reason                string
	QuoteNumber           string
	CustomerName          string
	Currency              string
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	PaymentTerms          string
	Incoterms             string
	TaxZone               string
	TaxCategory           string
	AdjustmentReason      string
	ActorName             string
	Items                 []PurchaseOrderCorrectionItem
	ID                    int64
	QuotationID           int64
	SourceRevision        int16
	CustomerID            int64
	PreviousVersion       int64
	ResultingVersion      int64
	ActorID               int64
	DiscountPercent       int16
	SubtotalMinor         int64
	DiscountMinor         int64
	PpnBps                int16
	PpnMinor              int64
	AdjustmentMinor       int64
	GrandTotalMinor       int64
	PODate                *time.Time
	RequestedDeliveryDate *time.Time
	ReceivedDate          time.Time
}

// purchaseOrderWriteLock is the row facts every versioned write needs. It is
// read FOR UPDATE in the caller's organization, so the version check and the
// write cannot be separated by another save.
type purchaseOrderWriteLock struct {
	UpdatedAt      time.Time
	ReceivedDate   time.Time
	State          string
	CustomerName   string
	Currency       string
	TaxZone        string
	TaxCategory    string
	Version        int64
	QuotationID    int64
	CustomerID     int64
	SourceRevision int16
	PpnBps         int16
	UpdatedBy      int64
	// SourceMissing is #480: the live quotation link was released by a
	// deletion. Reads still work; writes that need the frozen source revision
	// are refused with a named error instead of a missing-row surprise.
	SourceMissing bool
}

// lockPurchaseOrderWrite locks the record and enforces the optimistic
// concurrency contract. A missing row is ErrPurchaseOrderNotFound (never an
// invitation to insert); a stale ExpectedVersion is the named conflict, even
// when the intervening write also changed the record's state (a draft
// finalized by someone else), because the caller must see who moved it; and a
// current base in the wrong state is refused with the state rule.
func lockPurchaseOrderWrite(
	ctx context.Context,
	tx pgx.Tx,
	schema, orgID string,
	id, expectedVersion int64,
	requiredState string,
) (purchaseOrderWriteLock, error) {
	if expectedVersion < 1 {
		return purchaseOrderWriteLock{}, ErrPurchaseOrderVersionRequired
	}
	var lock purchaseOrderWriteLock
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT state, version, COALESCE(quotation_id, former_quotation_id, 0),
		       source_revision, customer_id,
		       customer_name, currency, tax_zone, tax_category, ppn_bps,
		       received_date, updated_by, updated_at, (quotation_id IS NULL)
		FROM %s.purchase_orders
		WHERE id = $1 AND org_id = $2
		FOR UPDATE`, schema), id, orgID).Scan(
		&lock.State, &lock.Version, &lock.QuotationID, &lock.SourceRevision,
		&lock.CustomerID, &lock.CustomerName, &lock.Currency, &lock.TaxZone,
		&lock.TaxCategory, &lock.PpnBps, &lock.ReceivedDate, &lock.UpdatedBy,
		&lock.UpdatedAt, &lock.SourceMissing)
	if errors.Is(err, pgx.ErrNoRows) {
		return purchaseOrderWriteLock{}, ErrPurchaseOrderNotFound
	}
	if err != nil {
		return purchaseOrderWriteLock{}, fmt.Errorf("lock purchase order %d: %w", id, err)
	}
	// The version is checked first: a record that moved since the editor loaded
	// is a conflict the caller can see and overwrite deliberately, even when the
	// intervening write also changed its state (a draft finalized by someone
	// else). Only when the base is current does the state rule decide.
	if lock.Version != expectedVersion {
		name := ""
		if err := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT COALESCE(NULLIF(btrim(display_name), ''), email)
			FROM %s.users WHERE id = $1`, schema), lock.UpdatedBy).Scan(&name); err != nil {
			return purchaseOrderWriteLock{}, fmt.Errorf(
				"read the writer of purchase order %d: %w", id, err)
		}
		return purchaseOrderWriteLock{}, &PurchaseOrderVersionConflictError{
			CurrentVersion: lock.Version,
			CurrentState:   lock.State,
			UpdatedBy:      lock.UpdatedBy,
			UpdatedByName:  name,
			UpdatedAt:      lock.UpdatedAt,
		}
	}
	if requiredState != "" && lock.State != requiredState {
		if requiredState == PurchaseOrderStateSaved {
			return purchaseOrderWriteLock{}, ErrPurchaseOrderNotSaved
		}
		return purchaseOrderWriteLock{}, ErrPurchaseOrderNotDraft
	}
	return lock, nil
}

// requirePurchaseOrderSource refuses a write whose rules need the frozen source
// revision when #480 released the link. Reads and the #475 repair stay open: the
// record is not broken, it is waiting for a new source.
func requirePurchaseOrderSource(lock purchaseOrderWriteLock) error {
	if lock.SourceMissing {
		return ErrPurchaseOrderSourceMissing
	}
	return nil
}

// CorrectPurchaseOrder replaces a saved record with the submitted values and
// keeps the values it replaced. The stored source link, customer, currency, tax
// basis, UUID, saved identity and confirmation number are never written here.
// A draft is refused: it is replaced by UpdatePurchaseOrderDraft, not corrected.
func (s *Store) CorrectPurchaseOrder(ctx context.Context, input PurchaseOrderCorrectionInput) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.ID < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	input.PONumber = strings.TrimSpace(input.PONumber)
	input.PaymentTerms = strings.TrimSpace(input.PaymentTerms)
	input.Incoterms = strings.TrimSpace(input.Incoterms)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return PurchaseOrder{}, ErrPurchaseOrderCorrectionReasonRequired
	}
	if utf8.RuneCountInString(input.Reason) > maxCorrectionReasonLength {
		return PurchaseOrder{}, fmt.Errorf(
			"%w: the correction reason must be at most %d characters",
			ErrPurchaseOrderInvalid, maxCorrectionReasonLength)
	}
	if err := validateSavedPurchaseOrderUpdateInput(input.PurchaseOrderUpdate); err != nil {
		return PurchaseOrder{}, err
	}

	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	schema := s.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("begin purchase order correction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lock, err := lockPurchaseOrderWrite(ctx, tx, schema, org.ID, input.ID,
		input.ExpectedVersion, PurchaseOrderStateSaved)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := requirePurchaseOrderSource(lock); err != nil {
		return PurchaseOrder{}, err
	}

	stored, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if _, err := insertPurchaseOrderCorrection(ctx, tx, schema, stored, org.ID,
		PurchaseOrderCorrectionKindCorrection, input.Reason, input.UpdatedBy); err != nil {
		return PurchaseOrder{}, err
	}
	if err := validatePurchaseOrderReferences(ctx, tx, schema,
		lock.QuotationID, lock.SourceRevision, input.Items); err != nil {
		return PurchaseOrder{}, err
	}
	lines, calculation, err := calculatePurchaseOrderLines(
		input.Items, input.DiscountPercent, input.AdjustmentMinor,
		input.AdjustmentReason, lock.PpnBps)
	if err != nil {
		return PurchaseOrder{}, err
	}
	numberKey := PurchaseOrderNumberKey(input.PONumber)
	if err := updatePurchaseOrderEditable(ctx, tx, schema, org.ID,
		input.PurchaseOrderUpdate, calculation, numberKey); err != nil {
		return PurchaseOrder{}, err
	}
	if err := replacePurchaseOrderItems(ctx, tx, schema, input.ID, lines); err != nil {
		return PurchaseOrder{}, err
	}

	corrected, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit purchase order correction: %w", err)
	}
	return corrected, nil
}

// validatePurchaseOrderUpdateInput is the shape check every write applies:
// payment terms, positive quantities, nonnegative prices and a sub-item that
// names its item. A draft may be saved incomplete, so it stops here.
func validatePurchaseOrderUpdateInput(input PurchaseOrderUpdate) error {
	if utf8.RuneCountInString(input.PaymentTerms) > maxPaymentTermsLength {
		return fmt.Errorf("%w: payment terms must be at most %d characters",
			ErrPurchaseOrderInvalid, maxPaymentTermsLength)
	}
	for index, item := range input.Items {
		if item.QuantityScaled <= 0 {
			return fmt.Errorf("%w: line %d quantity must be positive", ErrPurchaseOrderInvalid, index+1)
		}
		if item.UnitPriceMinor < 0 {
			return fmt.Errorf("%w: line %d unit price must not be negative", ErrPurchaseOrderInvalid, index+1)
		}
		if item.QuotationSubNo != nil && item.QuotationItemNo == nil {
			return fmt.Errorf("%w: line %d names a sub-item without its item", ErrPurchaseOrderInvalid, index+1)
		}
	}
	return nil
}

// validateSavedPurchaseOrderUpdateInput is the final-save minimum, shared by
// finalization and a saved-record correction so the two can never drift: a
// saved record needs its customer PO number and at least one valid line, and
// every line must pass the same shape check a draft line does. The caller has
// already trimmed the header values it submits.
func validateSavedPurchaseOrderUpdateInput(input PurchaseOrderUpdate) error {
	if err := validatePurchaseOrderUpdateInput(input); err != nil {
		return err
	}
	if input.PONumber == "" {
		return fmt.Errorf(
			"%w: a saved purchase order needs its customer PO number", ErrPurchaseOrderIncomplete)
	}
	if len(input.Items) == 0 {
		return fmt.Errorf("%w: at least one valid item is required", ErrPurchaseOrderIncomplete)
	}
	return nil
}

// updatePurchaseOrderEditable writes the header fields every replace operation
// owns. The record's identity, state and confirmation columns are absent by
// design; the version trigger bumps the row. numberKey is nil for a draft and
// the saved comparison key for a correction.
func updatePurchaseOrderEditable(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	input PurchaseOrderUpdate,
	calculation PurchaseOrderCalc,
	numberKey any,
) error {
	_, err := q.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET
			billing_address = $3,
			delivery_address = $4,
			ship_to_code = $5,
			contact_email = $6,
			contact_phone = $7,
			po_number = $8,
			po_number_key = $9,
			po_date = $10,
			received_date = COALESCE($11, received_date),
			requested_delivery_date = $12,
			payment_terms = $13,
			incoterms = $14,
			updated_by = $15,
			discount_percent = $16,
			subtotal_minor = $17,
			discount_minor = $18,
			ppn_bps = $19,
			ppn_minor = $20,
			adjustment_minor = $21,
			adjustment_reason = $22,
			grand_total_minor = $23,
			updated_at = NOW()
		WHERE id = $1 AND org_id = $2`, schema),
		input.ID, orgID, input.BillingAddress, input.DeliveryAddress,
		input.ShipToCode, input.ContactEmail, input.ContactPhone,
		nilIfEmptyString(input.PONumber), numberKey, dateOrNil(input.PODate),
		dateOrNil(input.ReceivedDate), dateOrNil(input.RequestedDeliveryDate),
		input.PaymentTerms, input.Incoterms, input.UpdatedBy,
		input.DiscountPercent, calculation.SubtotalMinor,
		calculation.DiscountMinor, calculation.PpnBps, calculation.PpnMinor,
		calculation.AdjustmentMinor, calculation.AdjustmentReason,
		calculation.GrandTotalMinor,
	)
	if err != nil {
		if isSavedPurchaseOrderNumberConflict(err) {
			return ErrPurchaseOrderNumberTaken
		}
		return fmt.Errorf("update purchase order %d: %w", input.ID, err)
	}
	return nil
}

// insertPurchaseOrderCorrection writes one immutable snapshot of the values a
// write is about to replace, including the attachment version that was active.
// It runs in the writer's transaction, so a failed write leaves no snapshot.
func insertPurchaseOrderCorrection(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema string,
	order PurchaseOrder,
	orgID, kind, reason string,
	actor int64,
) (int64, error) {
	if actor < 1 {
		return 0, fmt.Errorf("%w: the actor is required", ErrPurchaseOrderInvalid)
	}
	attachment, err := loadActivePurchaseOrderAttachment(ctx, q, schema, order.ID)
	if err != nil {
		return 0, err
	}
	var attachmentVersion any
	var attachmentSHA, attachmentFilename any
	if attachment != nil {
		attachmentVersion = attachment.Version
		attachmentSHA = attachment.SHA256
		attachmentFilename = attachment.Filename
	}

	var correctionID int64
	err = q.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_corrections (
			org_id, purchase_order_id, previous_version, resulting_version,
			kind, state, reason, actor,
			quotation_id, source_revision, quote_number, customer_id,
			customer_name, currency, tax_zone, tax_category,
			billing_address, delivery_address, ship_to_code, contact_email,
			contact_phone, po_number, po_date, received_date,
			requested_delivery_date, payment_terms, incoterms,
			discount_percent, subtotal_minor, discount_minor, ppn_bps,
			ppn_minor, adjustment_minor, adjustment_reason, grand_total_minor,
			attachment_version, attachment_sha256, attachment_filename
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,
			$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,
			$33,$34,$35,$36,$37,$38
		) RETURNING id`, schema),
		orgID, order.ID, order.Version, order.Version+1, kind, order.State,
		reason, actor, order.QuotationID, order.SourceRevision, order.QuoteNumber,
		order.CustomerID, order.CustomerName, order.Currency, order.TaxZone,
		order.TaxCategory, order.BillingAddress, order.DeliveryAddress,
		order.ShipToCode, order.ContactEmail, order.ContactPhone,
		nilIfEmptyString(order.PONumber), dateOrNil(order.PODate),
		order.ReceivedDate, dateOrNil(order.RequestedDeliveryDate),
		order.PaymentTerms, order.Incoterms, order.DiscountPercent,
		order.SubtotalMinor, order.DiscountMinor, order.PpnBps, order.PpnMinor,
		order.AdjustmentMinor, order.AdjustmentReason, order.GrandTotalMinor,
		attachmentVersion, attachmentSHA, attachmentFilename,
	).Scan(&correctionID)
	if err != nil {
		return 0, fmt.Errorf("store purchase order correction snapshot: %w", err)
	}
	for index, item := range order.Items {
		lineNumber := item.LineNumber
		if lineNumber < 1 {
			lineNumber = index + 1
		}
		if _, err := q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_correction_items (
				correction_id, line_number, description, quotation_item_no,
				quotation_sub_no, quantity_scaled, uom, unit_price_minor,
				discount_enabled, discount_percent, gross_minor, discount_minor,
				net_minor
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, schema),
			correctionID, lineNumber, item.Description, item.QuotationItemNo,
			item.QuotationSubNo, item.QuantityScaled, item.UoM,
			item.UnitPriceMinor, item.DiscountEnabled, item.DiscountPercent,
			item.GrossMinor, item.DiscountMinor, item.NetMinor,
		); err != nil {
			return 0, fmt.Errorf("store purchase order correction line %d: %w", lineNumber, err)
		}
	}
	return correctionID, nil
}

// loadActivePurchaseOrderAttachment reads the current customer PO PDF's
// identifying pair. No active document is nil, not an error: most corrections
// happen before one is attached.
func loadActivePurchaseOrderAttachment(ctx context.Context, q purchaseOrderQuerier, schema string, purchaseOrderID int64) (*PurchaseOrderCorrectionAttachment, error) {
	var attachment PurchaseOrderCorrectionAttachment
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT version, sha256, original_filename
		FROM %s.purchase_order_documents
		WHERE purchase_order_id = $1 AND active`, schema), purchaseOrderID).Scan(
		&attachment.Version, &attachment.SHA256, &attachment.Filename)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the active document of purchase order %d: %w", purchaseOrderID, err)
	}
	return &attachment, nil
}

// GetPurchaseOrderHistory returns the immutable snapshots of one record,
// newest first, inside the caller's organization. A record that does not exist
// (including one deleted by #480) is not found, never an empty success.
func (s *Store) GetPurchaseOrderHistory(ctx context.Context, id int64) ([]PurchaseOrderCorrection, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return nil, err
	}
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	schema := s.configSchema()
	if err := requirePurchaseOrderInOrg(ctx, pool, schema, org.ID, id); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.uuid::text, c.kind, c.state, c.reason, c.actor,
		       COALESCE(NULLIF(btrim(u.display_name), ''), u.email),
		       c.created_at, c.previous_version, c.resulting_version,
		       c.quotation_id, c.source_revision, c.quote_number, c.customer_id,
		       c.customer_name, c.currency, c.tax_zone, c.tax_category,
		       c.billing_address, c.delivery_address, c.ship_to_code,
		       c.contact_email, c.contact_phone, COALESCE(c.po_number, ''),
		       c.po_date, c.received_date, c.requested_delivery_date,
		       c.payment_terms, c.incoterms, c.discount_percent,
		       c.subtotal_minor, c.discount_minor, c.ppn_bps, c.ppn_minor,
		       c.adjustment_minor, c.adjustment_reason, c.grand_total_minor,
		       c.attachment_version, COALESCE(c.attachment_sha256, ''),
		       COALESCE(c.attachment_filename, '')
		FROM %s.purchase_order_corrections c
		JOIN %s.users u ON u.id = c.actor
		WHERE c.purchase_order_id = $1 AND c.org_id = $2
		ORDER BY c.previous_version DESC`, schema, schema), id, org.ID)
	if err != nil {
		return nil, fmt.Errorf("load purchase order %d history: %w", id, err)
	}
	defer rows.Close()

	history := make([]PurchaseOrderCorrection, 0, 4)
	for rows.Next() {
		var entry PurchaseOrderCorrection
		var poDate, requestedDelivery pgtype.Date
		var attachmentVersion pgtype.Int4
		var attachmentSHA, attachmentFilename string
		if err := rows.Scan(
			&entry.ID, &entry.UUID, &entry.Kind, &entry.State, &entry.Reason,
			&entry.ActorID, &entry.ActorName, &entry.CreatedAt,
			&entry.PreviousVersion, &entry.ResultingVersion,
			&entry.QuotationID, &entry.SourceRevision, &entry.QuoteNumber,
			&entry.CustomerID, &entry.CustomerName, &entry.Currency,
			&entry.TaxZone, &entry.TaxCategory, &entry.BillingAddress,
			&entry.DeliveryAddress, &entry.ShipToCode, &entry.ContactEmail,
			&entry.ContactPhone, &entry.PONumber, &poDate, &entry.ReceivedDate,
			&requestedDelivery, &entry.PaymentTerms, &entry.Incoterms,
			&entry.DiscountPercent, &entry.SubtotalMinor, &entry.DiscountMinor,
			&entry.PpnBps, &entry.PpnMinor, &entry.AdjustmentMinor,
			&entry.AdjustmentReason, &entry.GrandTotalMinor, &attachmentVersion,
			&attachmentSHA, &attachmentFilename,
		); err != nil {
			return nil, fmt.Errorf("scan purchase order %d history: %w", id, err)
		}
		if poDate.Valid {
			value := poDate.Time
			entry.PODate = &value
		}
		if requestedDelivery.Valid {
			value := requestedDelivery.Time
			entry.RequestedDeliveryDate = &value
		}
		if attachmentVersion.Valid {
			entry.Attachment = &PurchaseOrderCorrectionAttachment{
				Version:  attachmentVersion.Int32,
				SHA256:   attachmentSHA,
				Filename: attachmentFilename,
			}
		}
		entry.Items = make([]PurchaseOrderCorrectionItem, 0, 4)
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order %d history: %w", id, err)
	}
	if len(history) == 0 {
		return history, nil
	}

	ids := make([]int64, len(history))
	for index := range history {
		ids[index] = history[index].ID
	}
	itemRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT correction_id, line_number, description, quotation_item_no,
		       quotation_sub_no, quantity_scaled, uom, unit_price_minor,
		       discount_enabled, discount_percent, gross_minor, discount_minor,
		       net_minor
		FROM %s.purchase_order_correction_items
		WHERE correction_id = ANY($1)
		ORDER BY correction_id, line_number`, schema), ids)
	if err != nil {
		return nil, fmt.Errorf("load purchase order %d correction lines: %w", id, err)
	}
	defer itemRows.Close()
	byCorrection := make(map[int64][]PurchaseOrderCorrectionItem, len(history))
	for itemRows.Next() {
		var correctionID int64
		var item PurchaseOrderCorrectionItem
		var itemNo, subNo pgtype.Int4
		if err := itemRows.Scan(&correctionID, &item.LineNumber, &item.Description,
			&itemNo, &subNo, &item.QuantityScaled, &item.UoM, &item.UnitPriceMinor,
			&item.DiscountEnabled, &item.DiscountPercent, &item.GrossMinor,
			&item.DiscountMinor, &item.NetMinor); err != nil {
			return nil, fmt.Errorf("scan purchase order %d correction line: %w", id, err)
		}
		if itemNo.Valid {
			value := int(itemNo.Int32)
			item.QuotationItemNo = &value
		}
		if subNo.Valid {
			value := int(subNo.Int32)
			item.QuotationSubNo = &value
		}
		byCorrection[correctionID] = append(byCorrection[correctionID], item)
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order %d correction lines: %w", id, err)
	}
	for index := range history {
		if items, ok := byCorrection[history[index].ID]; ok {
			history[index].Items = items
		}
	}

	// #475: the old/new source and the choices a repair recorded. The rows are
	// read separately rather than joined so a history with no repair (the
	// overwhelming majority) pays one extra, empty query instead of widening
	// every snapshot row.
	repairRows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT correction_id, previous_quotation_id, previous_source_revision,
		       previous_quote_number, previous_customer_id, previous_customer_name,
		       new_quotation_id, new_source_revision, new_quote_number,
		       new_customer_id, new_customer_name
		FROM %s.purchase_order_source_repairs
		WHERE correction_id = ANY($1)`, schema), ids)
	if err != nil {
		return nil, fmt.Errorf("load purchase order %d repair links: %w", id, err)
	}
	defer repairRows.Close()
	repairs := make(map[int64]*PurchaseOrderSourceRepair, len(history))
	for repairRows.Next() {
		var correctionID int64
		var repair PurchaseOrderSourceRepair
		if err := repairRows.Scan(&correctionID,
			&repair.PreviousQuotationID, &repair.PreviousSourceRevision,
			&repair.PreviousQuoteNumber, &repair.PreviousCustomerID,
			&repair.PreviousCustomerName, &repair.NewQuotationID,
			&repair.NewSourceRevision, &repair.NewQuoteNumber,
			&repair.NewCustomerID, &repair.NewCustomerName,
		); err != nil {
			return nil, fmt.Errorf("scan purchase order %d repair link: %w", id, err)
		}
		repair.Decisions = make([]PurchaseOrderDecision, 0, 4)
		repairs[correctionID] = &repair
	}
	if err := repairRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase order %d repair links: %w", id, err)
	}

	if len(repairs) > 0 {
		decisionRows, err := pool.Query(ctx, fmt.Sprintf(`
			SELECT correction_id, key, choice
			FROM %s.purchase_order_source_repair_decisions
			WHERE correction_id = ANY($1)
			ORDER BY correction_id, key`, schema), ids)
		if err != nil {
			return nil, fmt.Errorf("load purchase order %d repair choices: %w", id, err)
		}
		defer decisionRows.Close()
		for decisionRows.Next() {
			var correctionID int64
			var decision PurchaseOrderDecision
			if err := decisionRows.Scan(&correctionID, &decision.Key, &decision.Choice); err != nil {
				return nil, fmt.Errorf("scan purchase order %d repair choice: %w", id, err)
			}
			if repair, ok := repairs[correctionID]; ok {
				repair.Decisions = append(repair.Decisions, decision)
			}
		}
		if err := decisionRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate purchase order %d repair choices: %w", id, err)
		}
	}
	for index := range history {
		history[index].Repair = repairs[history[index].ID]
	}
	return history, nil
}
