package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Issue #475: repairing a saved Purchase Order's source link.
//
// A saved record follows one exact issued Quotation revision. #473 deliberately
// made that link unwritable through a correction: changing the customer, the
// currency or the source silently would rewrite what the order is. Repair is
// the one admin-only operation that moves it, and it is not a shortcut around
// the difference review: the submitted values are compared against the new
// revision with the same rules a final save uses, every difference needs an
// explicit choice, and the reason staff typed is stored.
//
// The old link, the new link and the selected choices are kept as history
// through #473's snapshot tables (migration 036 adds the satellite rows). The
// record's version moves forward exactly as any other write does, so a stale
// editor is refused rather than applied over someone else's save.

const (
	// PurchaseOrderCorrectionKindSourceRepair is the history kind a repair
	// records. The snapshot holds the values the record carried before the
	// repair; migration 036's satellite rows hold the old and new source.
	PurchaseOrderCorrectionKindSourceRepair = "source_repair"
	// maxRepairReasonLength bounds the reason to the snapshot table's
	// char_length CHECK, the same 500 Unicode characters a correction allows.
	maxRepairReasonLength = 500
)

var (
	// ErrPurchaseOrderRepairForbidden refuses a repair by a non-admin. The
	// route already requires an admin; this re-reads the actor's stored role in
	// the write transaction, so a mistaken route or a future caller cannot
	// open the operation by forgetting a middleware.
	ErrPurchaseOrderRepairForbidden = errors.New("repairing a purchase order source link requires an administrator")
	// ErrPurchaseOrderRepairReasonRequired is the repair's reason rule, the
	// same deliberate explanation a correction requires.
	ErrPurchaseOrderRepairReasonRequired = errors.New("a source repair needs a reason")
	// ErrPurchaseOrderSourceUnchanged refuses a repair that selects the link
	// the record already follows. It changes nothing, so recording a repair
	// would only claim work that did not happen.
	ErrPurchaseOrderSourceUnchanged = errors.New("the purchase order already follows that quotation revision")
	// ErrPurchaseOrderTargetOccupied refuses a target Quotation that another
	// active Purchase Order already occupies. The pre-check inside the
	// transaction answers it normally; the unique quotation index is the
	// concurrency backstop and maps here too.
	ErrPurchaseOrderTargetOccupied = errors.New("that quotation already has an active purchase order")
	// ErrPurchaseOrderConfirmationCustomerLocked refuses a cross-customer
	// repair once a confirmation number is assigned: the number carries the
	// original customer's code, so moving the record would make the identifier
	// contradict its own customer.
	ErrPurchaseOrderConfirmationCustomerLocked = errors.New("a purchase order with an assigned confirmation number cannot move to another customer")
)

// PurchaseOrderSourceRepairInput moves one saved record to a new source
// revision. The embedded update is the values the record should carry after the
// repair -- starting from its current values, with the review's explicit
// "Use Quotation" choices applied -- and Reason is what the admin typed.
//
// The target is named separately from the editable payload on purpose: the
// editable set has no source field, and the API's protected-field guard must
// keep refusing a quotation link submitted as an ordinary value.
type PurchaseOrderSourceRepairInput struct {
	PurchaseOrderUpdate
	Reason              string
	QuotationID         int64
	RevisionNumber      int16
	Decisions           []PurchaseOrderDecision
	AcknowledgedMissing []string
	AcknowledgedExtra   []int
}

// PurchaseOrderSourceRepair is the old and new link one repair recorded, plus
// the difference choices the admin made. It is read from the history satellite
// rows, never derived from the live record, so a later repair cannot rewrite
// what an earlier one said.
type PurchaseOrderSourceRepair struct {
	PreviousQuoteNumber    string
	PreviousCustomerName   string
	NewQuoteNumber         string
	NewCustomerName        string
	Decisions              []PurchaseOrderDecision
	PreviousQuotationID    int64
	PreviousSourceRevision int16
	PreviousCustomerID     int64
	NewQuotationID         int64
	NewSourceRevision      int16
	NewCustomerID          int64
}

// RepairPurchaseOrderSource moves a saved record to another issued revision.
//
// Everything the write depends on is read and checked inside one transaction:
// the actor's stored role, the record's version and state, the target's
// existence, the occupancy of the target Quotation (with the unique index as
// the race backstop), the currency, the confirmation-number customer rule, the
// line references, and the full difference review. The replaced values plus the
// old/new link and the choices commit with the write or not at all.
func (s *Store) RepairPurchaseOrderSource(ctx context.Context, input PurchaseOrderSourceRepairInput) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.ID < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if input.QuotationID < 1 {
		return PurchaseOrder{}, fmt.Errorf("%w: a target quotation is required", ErrPurchaseOrderInvalid)
	}
	if input.RevisionNumber < 0 {
		return PurchaseOrder{}, fmt.Errorf("%w: the target revision must not be negative", ErrPurchaseOrderInvalid)
	}
	input.PONumber = strings.TrimSpace(input.PONumber)
	input.PaymentTerms = strings.TrimSpace(input.PaymentTerms)
	input.Incoterms = strings.TrimSpace(input.Incoterms)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return PurchaseOrder{}, ErrPurchaseOrderRepairReasonRequired
	}
	if utf8.RuneCountInString(input.Reason) > maxRepairReasonLength {
		return PurchaseOrder{}, fmt.Errorf(
			"%w: the repair reason must be at most %d characters",
			ErrPurchaseOrderInvalid, maxRepairReasonLength)
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
		return PurchaseOrder{}, fmt.Errorf("begin purchase order source repair: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := requirePurchaseOrderRepairAdmin(ctx, tx, schema, input.UpdatedBy); err != nil {
		return PurchaseOrder{}, err
	}
	if _, err := lockPurchaseOrderWrite(ctx, tx, schema, org.ID, input.ID,
		input.ExpectedVersion, PurchaseOrderStateSaved); err != nil {
		return PurchaseOrder{}, err
	}
	stored, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}

	source, err := loadPurchaseOrderSourceDetail(ctx, tx, schema,
		input.QuotationID, input.RevisionNumber)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := checkPurchaseOrderRepairTarget(ctx, tx, schema, org.ID, stored, source); err != nil {
		return PurchaseOrder{}, err
	}

	for index := range input.Items {
		input.Items[index].LineNumber = index + 1
	}
	if err := validatePurchaseOrderReferences(ctx, tx, schema,
		source.QuotationID, source.RevisionNumber, input.Items); err != nil {
		return PurchaseOrder{}, err
	}
	ppnBps, err := PurchaseOrderPPNBps(
		source.TaxZone, source.TaxType, source.ServiceCategory, source.TaxCategory)
	if err != nil {
		return PurchaseOrder{}, err
	}
	lines, calculation, err := calculatePurchaseOrderLines(
		input.Items, input.DiscountPercent, input.AdjustmentMinor,
		input.AdjustmentReason, ppnBps)
	if err != nil {
		return PurchaseOrder{}, err
	}
	comparison := ComparePurchaseOrder(source, PurchaseOrderReviewState{
		PaymentTerms:    input.PaymentTerms,
		Incoterms:       input.Incoterms,
		DiscountPercent: input.DiscountPercent,
		Lines:           lines,
	})
	if err := ValidatePurchaseOrderDecisions(
		comparison, input.Decisions, input.AcknowledgedMissing, input.AcknowledgedExtra); err != nil {
		return PurchaseOrder{}, err
	}

	correctionID, err := insertPurchaseOrderCorrection(ctx, tx, schema, stored, org.ID,
		PurchaseOrderCorrectionKindSourceRepair, input.Reason, input.UpdatedBy)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := insertPurchaseOrderSourceRepair(ctx, tx, schema, correctionID,
		stored, source, comparison, input.Decisions); err != nil {
		return PurchaseOrder{}, err
	}

	customerChanged := source.CustomerID != stored.CustomerID
	customerName := stored.CustomerName
	billingAddress := input.BillingAddress
	if customerChanged {
		// The customer identity is part of the link: the old customer's name
		// and billing address must not survive on a record that now follows
		// another customer's revision. Every other entered snapshot stays as
		// the current values the admin started from.
		customerName = source.CustomerName
		billingAddress = source.BillingAddress
	}
	numberKey := PurchaseOrderNumberKey(input.PONumber)
	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET
			quotation_id = $3,
			source_revision = $4,
			customer_id = $5,
			customer_name = $6,
			billing_address = $7,
			delivery_address = $8,
			ship_to_code = $9,
			contact_email = $10,
			contact_phone = $11,
			po_number = $12,
			po_number_key = $13,
			po_date = $14,
			received_date = COALESCE($15, received_date),
			requested_delivery_date = $16,
			payment_terms = $17,
			incoterms = $18,
			tax_zone = $19,
			tax_category = $20,
			ppn_bps = $21,
			discount_percent = $22,
			subtotal_minor = $23,
			discount_minor = $24,
			ppn_minor = $25,
			adjustment_minor = $26,
			adjustment_reason = $27,
			grand_total_minor = $28,
			printed_subtotal_minor = $29,
			printed_discount_minor = $30,
			printed_ppn_minor = $31,
			printed_grand_total_minor = $32,
			updated_by = $33,
			updated_at = NOW()
		WHERE id = $1 AND org_id = $2 AND state = 'saved'`, schema),
		input.ID, org.ID, source.QuotationID, source.RevisionNumber,
		source.CustomerID, customerName,
		billingAddress, input.DeliveryAddress, input.ShipToCode,
		input.ContactEmail, input.ContactPhone, input.PONumber, numberKey,
		dateOrNil(input.PODate), dateOrNil(input.ReceivedDate),
		dateOrNil(input.RequestedDeliveryDate), input.PaymentTerms, input.Incoterms,
		source.TaxZone, source.TaxCategory, calculation.PpnBps,
		input.DiscountPercent, calculation.SubtotalMinor, calculation.DiscountMinor,
		calculation.PpnMinor, calculation.AdjustmentMinor,
		calculation.AdjustmentReason, calculation.GrandTotalMinor,
		source.PrintedSubtotalMinor, source.PrintedDiscountMinor,
		source.PrintedPpnMinor, source.PrintedGrandTotalMinor, input.UpdatedBy,
	)
	if err != nil {
		if mapped := purchaseOrderRepairConflict(err); mapped != nil {
			return PurchaseOrder{}, mapped
		}
		return PurchaseOrder{}, fmt.Errorf("repair purchase order %d source: %w", input.ID, err)
	}
	if tag.RowsAffected() != 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotSaved
	}
	if err := replacePurchaseOrderItems(ctx, tx, schema, input.ID, lines); err != nil {
		return PurchaseOrder{}, err
	}

	repaired, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit purchase order source repair: %w", err)
	}
	return repaired, nil
}

// requirePurchaseOrderRepairAdmin re-reads the actor's stored role inside the
// write transaction. The route's middleware is the first gate; this is the one
// that cannot be bypassed by a caller that forgets it.
func requirePurchaseOrderRepairAdmin(ctx context.Context, q purchaseOrderQuerier, schema string, userID int64) error {
	if userID < 1 {
		return ErrPurchaseOrderRepairForbidden
	}
	var role string
	var active bool
	err := q.QueryRow(ctx, fmt.Sprintf(
		`SELECT role, is_active FROM %s.users WHERE id = $1`, schema), userID).Scan(&role, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPurchaseOrderRepairForbidden
	}
	if err != nil {
		return fmt.Errorf("read the actor of purchase order repair: %w", err)
	}
	if role != "admin" || !active {
		return ErrPurchaseOrderRepairForbidden
	}
	return nil
}

// checkPurchaseOrderRepairTarget applies every eligibility rule that does not
// depend on the submitted values: the link must move, the target Quotation must
// be free, the currency must match, and a number assigned to the old customer
// must not move to another one.
func checkPurchaseOrderRepairTarget(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	order PurchaseOrder,
	source PurchaseOrderSource,
) error {
	if order.QuotationID == source.QuotationID && order.SourceRevision == source.RevisionNumber {
		return ErrPurchaseOrderSourceUnchanged
	}
	var occupied bool
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s.purchase_orders
			WHERE org_id = $1 AND quotation_id = $2 AND id <> $3
		)`, schema), orgID, source.QuotationID, order.ID).Scan(&occupied); err != nil {
		return fmt.Errorf("check the target quotation %d occupancy: %w", source.QuotationID, err)
	}
	if occupied {
		return ErrPurchaseOrderTargetOccupied
	}
	if order.Currency != source.Currency {
		return fmt.Errorf("%w: record is %q, revision is %q",
			ErrPurchaseOrderSourceCurrencyMismatch, order.Currency, source.Currency)
	}
	if source.CustomerID != order.CustomerID && order.ConfirmationNumber != nil {
		return ErrPurchaseOrderConfirmationCustomerLocked
	}
	return nil
}

// insertPurchaseOrderSourceRepair writes the two satellite rows migration 036
// adds: the old/new link and the choices the admin made. Every submitted choice
// is kept, including one whose applied value now agrees with the new revision:
// the decision is what explains the value, and dropping it would leave the
// audit unable to say which side a changed value came from. The caller has
// already validated each key against the review.
func insertPurchaseOrderSourceRepair(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema string,
	correctionID int64,
	stored PurchaseOrder,
	source PurchaseOrderSource,
	comparison PurchaseOrderComparison,
	decisions []PurchaseOrderDecision,
) error {
	if _, err := q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_source_repairs (
			correction_id, previous_quotation_id, previous_source_revision,
			previous_quote_number, previous_customer_id, previous_customer_name,
			new_quotation_id, new_source_revision, new_quote_number,
			new_customer_id, new_customer_name
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, schema),
		correctionID, stored.QuotationID, stored.SourceRevision,
		stored.QuoteNumber, stored.CustomerID, stored.CustomerName,
		source.QuotationID, source.RevisionNumber, source.QuoteNumber,
		source.CustomerID, source.CustomerName,
	); err != nil {
		return fmt.Errorf("store purchase order source repair link: %w", err)
	}

	_ = comparison
	for _, decision := range decisions {
		if _, err := q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_source_repair_decisions (
				correction_id, key, choice
			) VALUES ($1,$2,$3)`, schema),
			correctionID, decision.Key, decision.Choice,
		); err != nil {
			return fmt.Errorf("store purchase order source repair decision %q: %w", decision.Key, err)
		}
	}
	return nil
}

// purchaseOrderRepairConflict maps the two unique indexes a repair can violate
// to the named refusals the caller can show. Any other unique violation is not
// this operation's answer and keeps its original error.
func purchaseOrderRepairConflict(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil
	}
	switch pgErr.ConstraintName {
	case "purchase_orders_active_quotation_idx":
		return ErrPurchaseOrderTargetOccupied
	case "purchase_orders_saved_number_idx":
		return ErrPurchaseOrderNumberTaken
	}
	return nil
}

// ReviewPurchaseOrderSource compares a saved record's current values against a
// candidate revision without writing anything. It is the repair's difference
// review: the same shape a final save reads, with the candidate as the source.
// Ineligible targets are refused here with the same errors the save uses, so an
// admin learns the reason before spending time on the review.
func (s *Store) ReviewPurchaseOrderSource(ctx context.Context, id, quotationID int64, revision int16) (PurchaseOrderReview, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	if id < 1 {
		return PurchaseOrderReview{}, ErrPurchaseOrderNotFound
	}
	if quotationID < 1 || revision < 0 {
		return PurchaseOrderReview{}, fmt.Errorf("%w: invalid target quotation revision", ErrPurchaseOrderInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	schema := s.configSchema()

	order, err := getPurchaseOrder(ctx, pool, schema, org.ID, id)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	if order.State != PurchaseOrderStateSaved {
		return PurchaseOrderReview{}, ErrPurchaseOrderNotSaved
	}
	source, err := loadPurchaseOrderSourceDetail(ctx, pool, schema, quotationID, revision)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	if err := checkPurchaseOrderRepairTarget(ctx, pool, schema, org.ID, order, source); err != nil {
		return PurchaseOrderReview{}, err
	}
	documentState, err := purchaseOrderDocumentState(ctx, pool, schema, order.ID)
	if err != nil {
		return PurchaseOrderReview{}, err
	}
	return PurchaseOrderReview{
		Order:         order,
		Source:        source,
		DocumentState: documentState,
	}, nil
}
