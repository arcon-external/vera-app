package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Issue #497: Staff link an issued Quotation revision to a Purchase Order
// draft through the existing difference review.
//
// A draft may be entered before its source exists (#496): never-linked or,
// after a Quotation was deleted, formerly-linked. Linking is an ordinary draft
// mutation -- a version bump and nothing else -- so it writes no correction
// snapshot, no 036 repair satellite and no reason. The admin-only source repair
// stays the only operation that moves a finalized record, and it keeps its
// reason, history and confirmation-number rules.
//
// The source owns identity: customer, quote number, source revision, quotation
// line references and printed totals. Every PO-owned value -- number, dates,
// addresses, ship-to, customer contact, payment terms, Incoterm, lines,
// quantities, prices, discounts and the adjustment -- stays exactly as staff
// entered it. The one deliberate exception is the billing address, which follows
// a changed customer exactly as the saved-record repair already does; it is
// never replaced for a same-customer link.
//
// Currency is structural, not convertible: the draft and the revision must be
// in one currency or the link is refused. The remedy is editing a never-linked
// draft's currency (or choosing another revision) before retrying -- never a
// conversion by the store.
//
// Tax is the one basis the link may move, and only by explicit choice: when the
// draft and source bases differ, staff must choose "keep" or "source". The
// server then recomputes PPN from the chosen basis. There is no silent
// adoption, and the basis is frozen again as soon as the link commits.

const (
	// PurchaseOrderDraftLinkTaxKeep keeps the draft's persisted tax basis.
	PurchaseOrderDraftLinkTaxKeep = "keep"
	// PurchaseOrderDraftLinkTaxSource adopts the source revision's tax basis.
	PurchaseOrderDraftLinkTaxSource = "source"
)

var (
	// ErrPurchaseOrderDraftLinkNotEligible refuses a draft that already follows
	// a live revision. This slice links never/former drafts only; re-linking a
	// live draft is deliberately not an action here.
	ErrPurchaseOrderDraftLinkNotEligible = errors.New("only a draft without a live source can be linked")
	// ErrPurchaseOrderDraftLinkTaxChoiceRequired refuses a link whose draft and
	// source tax bases differ while no keep/source choice was made. The basis
	// is never adopted silently.
	ErrPurchaseOrderDraftLinkTaxChoiceRequired = errors.New("choose whether to keep the draft tax basis or use the source revision's")
)

// PurchaseOrderDraftLinkInput is one Staff draft link. The target pair is named
// separately from any editable payload because a link carries no PO values:
// the working copy is saved as an ordinary draft first, and the link commits
// only identity, the chosen tax basis and the recorded review.
//
// AcknowledgedMissing and AcknowledgedExtra are the same explicit line
// acknowledgements a final save requires; a manual line is extra and an
// unclaimed source line is missing until the reviewer decides each one.
type PurchaseOrderDraftLinkInput struct {
	ID                  int64
	ExpectedVersion     int64
	QuotationID         int64
	RevisionNumber      int16
	Decisions           []PurchaseOrderDecision
	AcknowledgedMissing []string
	AcknowledgedExtra   []int
	UpdatedBy           int64
	// TaxChoice is "keep", "source", or empty. It is required only when the
	// draft and source tax bases differ; empty then is refused.
	TaxChoice string
}

// PurchaseOrderDraftTaxBasis is one side of the tax decision: the persisted
// basis and the PPN rate the server derives from it.
type PurchaseOrderDraftTaxBasis struct {
	TaxZone         string
	TaxType         string
	ServiceCategory string
	TaxCategory     string
	PpnBps          int16
}

// PurchaseOrderDraftLinkReview is the difference review's source plus the tax
// decision the link needs. The embedded PurchaseOrderReview is the same shape
// the final save and the admin repair use, so the client reuses one review
// implementation rather than a second comparison.
type PurchaseOrderDraftLinkReview struct {
	PurchaseOrderReview
	DraftTax          PurchaseOrderDraftTaxBasis
	SourceTax         PurchaseOrderDraftTaxBasis
	TaxChoiceRequired bool
}

// taxBasisFromOrder is the persisted side of the tax decision.
func taxBasisFromOrder(order PurchaseOrder) PurchaseOrderDraftTaxBasis {
	return PurchaseOrderDraftTaxBasis{
		TaxZone:         order.TaxZone,
		TaxType:         order.TaxType,
		ServiceCategory: order.ServiceCategory,
		TaxCategory:     order.TaxCategory,
		PpnBps:          order.PpnBps,
	}
}

// taxBasisFromSource derives the source side, including the rate the server
// would compute from that basis.
func taxBasisFromSource(source PurchaseOrderSource) (PurchaseOrderDraftTaxBasis, error) {
	ppnBps, err := PurchaseOrderPPNBps(
		source.TaxZone, source.TaxType, source.ServiceCategory, source.TaxCategory)
	if err != nil {
		return PurchaseOrderDraftTaxBasis{}, err
	}
	return PurchaseOrderDraftTaxBasis{
		TaxZone:         source.TaxZone,
		TaxType:         source.TaxType,
		ServiceCategory: source.ServiceCategory,
		TaxCategory:     source.TaxCategory,
		PpnBps:          ppnBps,
	}, nil
}

// taxBasisDiffers compares the commercial basis, not the derived rate. A stored
// legacy rate is corrected by the recompute rather than treated as a silent
// basis move.
func taxBasisDiffers(a, b PurchaseOrderDraftTaxBasis) bool {
	return strings.TrimSpace(a.TaxZone) != strings.TrimSpace(b.TaxZone) ||
		strings.TrimSpace(a.TaxType) != strings.TrimSpace(b.TaxType) ||
		strings.TrimSpace(a.ServiceCategory) != strings.TrimSpace(b.ServiceCategory) ||
		strings.TrimSpace(a.TaxCategory) != strings.TrimSpace(b.TaxCategory)
}

// PreviewPurchaseOrderDraftLink reads the candidate revision and the record
// beside it without writing anything. It is the Staff draft-link review: it
// applies the eligibility rules that do not depend on the submitted values
// (draft, no live source, free target) and reports which tax decision the link
// needs. Currency is reported, not refused, so the client can explain the
// remedy instead of showing a bare error; the commit is what refuses it.
func (s *Store) PreviewPurchaseOrderDraftLink(
	ctx context.Context, id, quotationID int64, revision int16,
) (PurchaseOrderDraftLinkReview, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrderDraftLinkReview{}, err
	}
	if id < 1 {
		return PurchaseOrderDraftLinkReview{}, ErrPurchaseOrderNotFound
	}
	if quotationID < 1 || revision < 0 {
		return PurchaseOrderDraftLinkReview{}, fmt.Errorf("%w: invalid target quotation revision", ErrPurchaseOrderInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrderDraftLinkReview{}, err
	}
	schema := s.configSchema()

	order, err := getPurchaseOrder(ctx, pool, schema, org.ID, id)
	if err != nil {
		return PurchaseOrderDraftLinkReview{}, err
	}
	if order.State != PurchaseOrderStateDraft {
		return PurchaseOrderDraftLinkReview{}, ErrPurchaseOrderNotDraft
	}
	if order.SourceState == PurchaseOrderSourceStateLive {
		return PurchaseOrderDraftLinkReview{}, ErrPurchaseOrderDraftLinkNotEligible
	}
	source, err := loadPurchaseOrderSourceDetail(ctx, pool, schema, quotationID, revision)
	if err != nil {
		return PurchaseOrderDraftLinkReview{}, err
	}
	if err := checkDraftLinkTargetAvailable(ctx, pool, schema, org.ID, order.ID, source.QuotationID); err != nil {
		return PurchaseOrderDraftLinkReview{}, err
	}
	documentState, err := purchaseOrderDocumentState(ctx, pool, schema, order.ID)
	if err != nil {
		return PurchaseOrderDraftLinkReview{}, err
	}
	draftTax := taxBasisFromOrder(order)
	sourceTax, err := taxBasisFromSource(source)
	if err != nil {
		return PurchaseOrderDraftLinkReview{}, err
	}
	return PurchaseOrderDraftLinkReview{
		PurchaseOrderReview: PurchaseOrderReview{
			Order:         order,
			Source:        source,
			DocumentState: documentState,
		},
		DraftTax:          draftTax,
		SourceTax:         sourceTax,
		TaxChoiceRequired: taxBasisDiffers(draftTax, sourceTax),
	}, nil
}

// LinkPurchaseOrderDraftSource commits one Staff draft link.
//
// Everything the write depends on is read and checked inside one transaction:
// the record's version and draft state, the no-live-source rule, the target's
// existence and occupancy (with the unique index as the race backstop), the
// currency, the line references, the explicit tax choice, and the full
// difference review. The row's version moves forward through the same trigger
// every draft write uses, and no history row is written: this is not a
// correction and not the admin repair.
func (s *Store) LinkPurchaseOrderDraftSource(ctx context.Context, input PurchaseOrderDraftLinkInput) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.ID < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if input.QuotationID < 1 || input.RevisionNumber < 0 {
		return PurchaseOrder{}, fmt.Errorf("%w: invalid target quotation revision", ErrPurchaseOrderInvalid)
	}
	if input.UpdatedBy < 1 {
		return PurchaseOrder{}, fmt.Errorf("%w: the actor is required", ErrPurchaseOrderInvalid)
	}
	taxChoice := strings.TrimSpace(input.TaxChoice)

	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	schema := s.configSchema()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("begin draft source link: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lock, err := lockPurchaseOrderWrite(ctx, tx, schema, org.ID, input.ID,
		input.ExpectedVersion, PurchaseOrderStateDraft)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if lock.SourceState == PurchaseOrderSourceStateLive {
		return PurchaseOrder{}, ErrPurchaseOrderDraftLinkNotEligible
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
	if err := checkDraftLinkTargetAvailable(ctx, tx, schema, org.ID, stored.ID, source.QuotationID); err != nil {
		return PurchaseOrder{}, err
	}
	// Currency is structural: the two sides are only one money when they are
	// the same currency. No conversion exists anywhere in this path.
	if lock.Currency != source.Currency {
		return PurchaseOrder{}, fmt.Errorf("%w: record is %q, revision is %q",
			ErrPurchaseOrderSourceCurrencyMismatch, lock.Currency, source.Currency)
	}

	draftTax := taxBasisFromOrder(stored)
	sourceTax, err := taxBasisFromSource(source)
	if err != nil {
		return PurchaseOrder{}, err
	}
	basis := draftTax
	switch taxChoice {
	case "":
		if taxBasisDiffers(draftTax, sourceTax) {
			return PurchaseOrder{}, ErrPurchaseOrderDraftLinkTaxChoiceRequired
		}
	case PurchaseOrderDraftLinkTaxKeep:
		// Keep the PO-owned basis; PPN is still recomputed from it so a stale
		// stored rate cannot survive the link.
	case PurchaseOrderDraftLinkTaxSource:
		basis = sourceTax
	default:
		return PurchaseOrder{}, fmt.Errorf(
			"%w: the tax choice must be %q or %q",
			ErrPurchaseOrderInvalid, PurchaseOrderDraftLinkTaxKeep, PurchaseOrderDraftLinkTaxSource)
	}
	ppnBps, err := PurchaseOrderPPNBps(basis.TaxZone, basis.TaxType, basis.ServiceCategory, basis.TaxCategory)
	if err != nil {
		return PurchaseOrder{}, err
	}

	if err := validatePurchaseOrderReferences(ctx, tx, schema,
		source.QuotationID, source.RevisionNumber, stored.Items); err != nil {
		return PurchaseOrder{}, err
	}
	lines, calculation, err := calculatePurchaseOrderLines(
		stored.Items, stored.DiscountPercent, stored.AdjustmentMinor,
		stored.AdjustmentReason, ppnBps)
	if err != nil {
		return PurchaseOrder{}, err
	}
	comparison := ComparePurchaseOrder(source, PurchaseOrderReviewState{
		PaymentTerms:    stored.PaymentTerms,
		Incoterms:       stored.Incoterms,
		DiscountPercent: stored.DiscountPercent,
		Lines:           lines,
	})
	if err := ValidatePurchaseOrderDecisions(
		comparison, input.Decisions, input.AcknowledgedMissing, input.AcknowledgedExtra); err != nil {
		return PurchaseOrder{}, err
	}

	// The customer name is source-owned unconditionally: the revision's customer
	// is the authority even when the customer did not change but was renamed in
	// the directory. The billing address is the one conditional: a changed
	// customer brings the revision's address, while a same-customer link keeps
	// the PO-owned address (the saved-record repair's rule).
	customerName := source.CustomerName
	billingAddress := stored.BillingAddress
	if source.CustomerID != stored.CustomerID {
		billingAddress = source.BillingAddress
	}

	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET
			quotation_id = $3,
			source_revision = $4,
			former_quotation_id = NULL,
			former_quotation_uuid = NULL,
			former_quote_number = NULL,
			former_source_revision = NULL,
			customer_id = $5,
			customer_name = $6,
			billing_address = $7,
			tax_zone = $8,
			tax_type = $9,
			service_category = $10,
			tax_category = $11,
			ppn_bps = $12,
			subtotal_minor = $13,
			discount_minor = $14,
			ppn_minor = $15,
			adjustment_minor = $16,
			adjustment_reason = $17,
			grand_total_minor = $18,
			printed_subtotal_minor = $19,
			printed_discount_minor = $20,
			printed_ppn_minor = $21,
			printed_grand_total_minor = $22,
			updated_by = $23,
			updated_at = NOW()
		WHERE id = $1 AND org_id = $2 AND state = 'draft' AND quotation_id IS NULL`, schema),
		stored.ID, org.ID, source.QuotationID, source.RevisionNumber,
		source.CustomerID, customerName, billingAddress,
		basis.TaxZone, basis.TaxType, basis.ServiceCategory, basis.TaxCategory,
		calculation.PpnBps, calculation.SubtotalMinor, calculation.DiscountMinor,
		calculation.PpnMinor, calculation.AdjustmentMinor,
		calculation.AdjustmentReason, calculation.GrandTotalMinor,
		source.PrintedSubtotalMinor, source.PrintedDiscountMinor,
		source.PrintedPpnMinor, source.PrintedGrandTotalMinor, input.UpdatedBy,
	)
	if err != nil {
		if mapped := purchaseOrderRepairConflict(err); mapped != nil {
			return PurchaseOrder{}, mapped
		}
		return PurchaseOrder{}, fmt.Errorf("link purchase order %d source: %w", input.ID, err)
	}
	if tag.RowsAffected() != 1 {
		// The row lock makes this impossible; checking it means a future change
		// to the predicate cannot silently leave the link half-applied.
		return PurchaseOrder{}, ErrPurchaseOrderNotDraft
	}

	linked, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit draft source link: %w", err)
	}
	return linked, nil
}

// checkDraftLinkTargetAvailable applies the occupancy rule inside the caller's
// transaction. One Quotation has at most one active Purchase Order across every
// revision; the partial unique index is the concurrency backstop and maps to
// the same named refusal.
func checkDraftLinkTargetAvailable(
	ctx context.Context,
	q purchaseOrderQuerier,
	schema, orgID string,
	orderID, quotationID int64,
) error {
	var occupied bool
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s.purchase_orders
			WHERE org_id = $1 AND quotation_id = $2 AND id <> $3
		)`, schema), orgID, quotationID, orderID).Scan(&occupied); err != nil {
		return fmt.Errorf("check the target quotation %d occupancy: %w", quotationID, err)
	}
	if occupied {
		return ErrPurchaseOrderTargetOccupied
	}
	return nil
}
