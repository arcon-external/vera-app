package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

// The three live source shapes (#496). A row is exactly one of them:
//
//	live   quotation_id IS NOT NULL
//	former quotation_id IS NULL and every former_* column set
//	never  quotation_id IS NULL and every former_* column NULL (draft only)
//
// Machine logic keys off this state, never off a zero id or a missing field: a
// linked revision 00 is a real link and serialises its revision as absent.
const (
	PurchaseOrderSourceStateLive   = "live"
	PurchaseOrderSourceStateNever  = "never"
	PurchaseOrderSourceStateFormer = "former"
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
	// ErrPurchaseOrderSourceMissing is the #480/#496 state a draft can be in:
	// it was never linked, or the Quotation it followed was permanently
	// deleted. The rules that compare against the frozen revision cannot run
	// until a source is linked. The wording is deliberately neutral because a
	// never-linked draft was not deleted.
	ErrPurchaseOrderSourceMissing = errors.New("purchase order has no live source quotation; link a quotation before this action")
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
	TaxType                string
	ServiceCategory        string
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
	// Version is the optimistic-concurrency revision (#473). It starts at 1
	// and the row's trigger moves it forward on every write, so a save must
	// state the version it loaded.
	Version int64
	// SourceMissing is true for both a never-linked draft and a record whose
	// source quotation was deleted. SourceState names which one it is.
	SourceMissing bool
	// SourceState is "live", "never" or "former" (#496). It is the only
	// presence signal: QuotationID/CustomerID/SourceRevision stay numeric for
	// old readers and are zero when absent, which is also a real revision 00.
	SourceState string
	// PrintedTotalsKnown is false exactly for a never-linked draft, whose
	// printed source figures do not exist yet. Formerly-linked and live records
	// copied them when they were created.
	PrintedTotalsKnown bool
	// Finalization (#471). SavedAt/SavedBy are set exactly once, by the
	// transition from draft to saved. PDFAbsentConfirmed records the deliberate
	// confirmation a save needs while no customer PDF exists.
	SavedAt            *time.Time
	SavedBy            *int64
	PDFAbsentConfirmed bool
	// The optional customer-system confirmation reference (#474). It is null
	// until explicitly assigned, then immutable. ConfirmationWarn is derived
	// from the stored sequence; the customer code and year are kept beside the
	// number so the identifier can be audited without parsing it.
	ConfirmationNumber       *string
	ConfirmationSequence     *int64
	ConfirmationYear         *int16
	ConfirmationCustomerCode *int16
	ConfirmationAssignedAt   *time.Time
	ConfirmationAssignedBy   *int64
	ConfirmationWarn         bool
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
	// AssignConfirmationNumber asks for the optional customer-system reference
	// (#474). It is false unless staff ticked the option; an unrequested save
	// consumes no sequence.
	AssignConfirmationNumber bool
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

// PurchaseOrderBlankCreate starts a draft with no source and no Customer
// (#496). Currency is deliberately absent: the server seeds the company
// currency. The tax triple is the user's Quotation default carried by the
// desktop; the server validates it and computes PPN from the persisted basis.
type PurchaseOrderBlankCreate struct {
	ReceivedDate    *time.Time
	TaxZone         string
	TaxType         string
	ServiceCategory string
	TaxCategory     string
	CreatedBy       int64
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
	// Currency and the tax triple are editable only while a draft has never
	// been linked (#496). Empty means "keep what the row already carries"; a
	// linked row refuses any non-empty value that differs from its frozen
	// basis, so a stale client cannot move it by accident.
	Currency        string
	TaxZone         string
	TaxType         string
	ServiceCategory string
	TaxCategory     string
	// ExpectedVersion is the version the editor loaded (#473). It is required:
	// a missing base is refused rather than treated as "no check".
	ExpectedVersion int64
	// Overwrite records that the caller deliberately replaced a version it was
	// warned about, so the replaced values are kept as history. It never
	// weakens the version check: the stated version must still match.
	Overwrite bool
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
			created_by, updated_by, tax_type, service_category
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,'','','',NULL,NULL,$9,NULL,
			$10,$11,$12,$13,$14,
			$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,
			$27,$27,$28,$29)
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
		source.TaxType, source.ServiceCategory,
	).Scan(&id, &uuid)
	if err != nil {
		if isActivePurchaseOrderConflict(err) {
			return PurchaseOrder{}, ErrPurchaseOrderExists
		}
		// A concurrent quotation deletion holds the parent row FOR UPDATE; this
		// insert's foreign-key check then fails with 23503. The source is gone,
		// not the request malformed.
		if isMissingQuotationConflict(err) {
			return PurchaseOrder{}, ErrPurchaseOrderSourceNotFound
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

// CreatePurchaseOrderBlankDraft creates a draft with no source and no Customer
// (#496). The server owns the company currency; the desktop supplies the user's
// tax defaults, which are validated here and persisted as the full triple. The
// row is a never-linked draft: no Quotation, no revision, no Customer, no
// printed source totals and no invented former-source reference.
func (s *Store) CreatePurchaseOrderBlankDraft(ctx context.Context, input PurchaseOrderBlankCreate) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.CreatedBy < 1 {
		return PurchaseOrder{}, fmt.Errorf("%w: the creator is required", ErrPurchaseOrderInvalid)
	}
	pool, err := s.poolOrError()
	if err != nil {
		return PurchaseOrder{}, err
	}
	schema := s.configSchema()

	profile, err := s.GetCompanyProfile(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	basis, err := resolveBlankPurchaseOrderBasis(profile.Currency, input)
	if err != nil {
		return PurchaseOrder{}, err
	}

	received := normalizeDate(input.ReceivedDate)
	if received.IsZero() {
		received = companyToday(time.Now())
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("begin blank purchase order transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

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
			created_by, updated_by, tax_type, service_category
		) VALUES (
			$1,NULL,NULL,$2,NULL,
			'','','','','','',NULL,NULL,$3,
			NULL,'','',$4,
			$5,$10,0,0,
			0,$6,0,0,
			'',0,0,0,0,0,
			$7,$7,$8,$9)
		RETURNING id, uuid::text`, schema),
		org.ID, PurchaseOrderStateDraft, received, basis.Currency,
		basis.TaxZone, basis.PpnBps, input.CreatedBy,
		basis.TaxType, basis.ServiceCategory, basis.TaxCategory,
	).Scan(&id, &uuid)
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("insert blank purchase order: %w", err)
	}

	if err := replacePurchaseOrderItems(ctx, tx, schema, id, nil); err != nil {
		return PurchaseOrder{}, err
	}

	created, err := getPurchaseOrder(ctx, tx, schema, org.ID, id)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PurchaseOrder{}, fmt.Errorf("commit blank purchase order draft: %w", err)
	}
	return created, nil
}

// purchaseOrderBasis is the persisted currency and tax triple a write resolves.
type purchaseOrderBasis struct {
	Currency        string
	TaxZone         string
	TaxType         string
	ServiceCategory string
	TaxCategory     string
	PpnBps          int16
}

// resolveBlankPurchaseOrderBasis validates the desktop's tax defaults and
// seeds the currency from the company profile. An empty triple falls back to
// FTZ goods, the same safe default a quotation uses.
func resolveBlankPurchaseOrderBasis(companyCurrency string, input PurchaseOrderBlankCreate) (purchaseOrderBasis, error) {
	currencyCode, err := checkDraftCurrency(companyCurrency)
	if err != nil {
		return purchaseOrderBasis{}, err
	}
	zone := strings.TrimSpace(input.TaxZone)
	taxType := strings.TrimSpace(input.TaxType)
	serviceCategory := strings.TrimSpace(input.ServiceCategory)
	legacyCategory := strings.TrimSpace(input.TaxCategory)
	if zone == "" && taxType == "" && serviceCategory == "" && legacyCategory == "" {
		zone, taxType = "FTZ", "Goods"
	}
	if zone == "" {
		zone = "FTZ"
	}
	ppnBps, err := PurchaseOrderPPNBps(zone, taxType, serviceCategory, legacyCategory)
	if err != nil {
		return purchaseOrderBasis{}, err
	}
	return purchaseOrderBasis{
		Currency:        currencyCode,
		TaxZone:         zone,
		TaxType:         taxType,
		ServiceCategory: serviceCategory,
		TaxCategory:     legacyCategory,
		PpnBps:          ppnBps,
	}, nil
}

// keepsFormerLegacyPopulation reports whether a formerly-linked legacy row
// should keep the one-time legacy population instead of taking the editable
// path. It is true only when the stored tax type is empty (a pre-039 row) and
// the submission names no tax type, currency or zone change: that is the
// editor's display-derived legacy basis, which is not an edit. A submission
// that names a type takes the editable path; there the effective commercial
// basis is compared with the stored one so a derived label still keeps the
// frozen rate (sameFormerCommercialBasis).
func keepsFormerLegacyPopulation(lock purchaseOrderWriteLock, input PurchaseOrderUpdate) bool {
	if lock.SourceState != PurchaseOrderSourceStateFormer || strings.TrimSpace(lock.TaxType) != "" {
		return false
	}
	if strings.TrimSpace(input.TaxType) != "" {
		return false
	}
	if input.Currency != "" && !strings.EqualFold(strings.TrimSpace(input.Currency), lock.Currency) {
		return false
	}
	if input.TaxZone != "" && strings.TrimSpace(input.TaxZone) != lock.TaxZone {
		return false
	}
	if legacyCategory := strings.TrimSpace(input.TaxCategory); legacyCategory != "" &&
		legacyCategory != lock.TaxCategory {
		return false
	}
	return true
}

// sameFormerCommercialBasis reports whether an editable former-draft save
// leaves the commercial tax basis exactly as stored. The stored side derives
// its type/service labels from the legacy category when a pre-039 row has no
// tax type, because that is what the editor derives for display: sending those
// derived labels back is not a tax edit. The legacy category is compared too,
// since it is a stored label a client could otherwise move silently.
//
// An unrepresentable stored legacy category never matches: any save of it is a
// real basis change and recomputes.
func sameFormerCommercialBasis(lock purchaseOrderWriteLock, submitted purchaseOrderBasis) bool {
	zone := strings.TrimSpace(lock.TaxZone)
	if zone == "" {
		zone = "FTZ"
	}
	taxType := strings.TrimSpace(lock.TaxType)
	serviceCategory := strings.TrimSpace(lock.ServiceCategory)
	if taxType == "" {
		derivedType, derivedService, ok := legacyBasisFor(lock.TaxCategory)
		if !ok {
			return false
		}
		taxType = derivedType
		if serviceCategory == "" {
			serviceCategory = derivedService
		}
	}
	return zone == strings.TrimSpace(submitted.TaxZone) &&
		taxType == strings.TrimSpace(submitted.TaxType) &&
		serviceCategory == strings.TrimSpace(submitted.ServiceCategory) &&
		strings.TrimSpace(lock.TaxCategory) == strings.TrimSpace(submitted.TaxCategory)
}

// resolvePurchaseOrderUpdateBasis keeps only a LIVE row's currency and tax
// basis frozen. A never- or formerly-linked draft is still before a link, so
// its basis is editable within the supported options (#493 review); a link
// still needs an exact currency match and an explicit tax choice, so nothing is
// converted silently.
//
// Empty submitted values mean "keep the stored value"; a never-linked draft is
// allowed to clear its service category when it switches to Goods.
//
// A pre-007 source revision stored an empty tax_type/service_category and only
// the legacy tax_category. Such a row may be populated once from the matching
// legacy basis, and that population is also preserved for a formerly-linked
// row that submits no tax type: a stored non-empty basis remains frozen on a
// live row, and a legacy partial submission (the display-derived basis) still
// cannot drift the effective basis.
func resolvePurchaseOrderUpdateBasis(lock purchaseOrderWriteLock, input PurchaseOrderUpdate) (purchaseOrderBasis, error) {
	if lock.SourceState == PurchaseOrderSourceStateLive || keepsFormerLegacyPopulation(lock, input) {
		if input.Currency != "" && !strings.EqualFold(strings.TrimSpace(input.Currency), lock.Currency) {
			return purchaseOrderBasis{}, fmt.Errorf(
				"%w: the currency is fixed while a source is linked", ErrPurchaseOrderInvalid)
		}
		if input.TaxZone != "" && strings.TrimSpace(input.TaxZone) != lock.TaxZone {
			return purchaseOrderBasis{}, fmt.Errorf(
				"%w: the tax zone is fixed while a source is linked", ErrPurchaseOrderInvalid)
		}
		legacyCategory := strings.TrimSpace(input.TaxCategory)
		if legacyCategory != "" && legacyCategory != lock.TaxCategory {
			return purchaseOrderBasis{}, fmt.Errorf(
				"%w: the tax category is fixed while a source is linked", ErrPurchaseOrderInvalid)
		}

		basis := basisFromLock(lock)
		submittedType := strings.TrimSpace(input.TaxType)
		submittedService := strings.TrimSpace(input.ServiceCategory)
		if lock.TaxType == "" {
			// Legacy row: the stored type is truly empty. Derive the effective
			// basis from the stored legacy category and accept only an exact
			// match. A service-only submission must not drift the effective
			// basis just because the type was omitted.
			expectedType, expectedService, ok := legacyBasisFor(lock.TaxCategory)
			if !ok {
				if submittedType != "" || submittedService != "" {
					return purchaseOrderBasis{}, fmt.Errorf(
						"%w: the tax type is fixed while a source is linked", ErrPurchaseOrderInvalid)
				}
			} else {
				if submittedType != "" && submittedType != expectedType {
					return purchaseOrderBasis{}, fmt.Errorf(
						"%w: the tax type is fixed while a source is linked", ErrPurchaseOrderInvalid)
				}
				if submittedService != "" && submittedService != expectedService {
					return purchaseOrderBasis{}, fmt.Errorf(
						"%w: the service category is fixed while a source is linked", ErrPurchaseOrderInvalid)
				}
				// Populate only fields that are truly empty. A stored service is
				// never overwritten by the legacy fallback.
				basis.TaxType = expectedType
				if lock.ServiceCategory == "" {
					basis.ServiceCategory = expectedService
				}
			}
		} else {
			if submittedType != "" && submittedType != lock.TaxType {
				return purchaseOrderBasis{}, fmt.Errorf(
					"%w: the tax type is fixed while a source is linked", ErrPurchaseOrderInvalid)
			}
			if lock.ServiceCategory != "" {
				if submittedService != "" && submittedService != lock.ServiceCategory {
					return purchaseOrderBasis{}, fmt.Errorf(
						"%w: the service category is fixed while a source is linked", ErrPurchaseOrderInvalid)
				}
			} else if submittedService != "" {
				if lock.TaxType == "Goods" {
					return purchaseOrderBasis{}, fmt.Errorf(
						"%w: the service category is fixed while a source is linked", ErrPurchaseOrderInvalid)
				}
				basis.ServiceCategory = submittedService
			}
		}
		// A stored service remains frozen even when a legacy type population
		// also applies: a submitted value must match it as well.
		if lock.ServiceCategory != "" && submittedService != "" &&
			submittedService != lock.ServiceCategory {
			return purchaseOrderBasis{}, fmt.Errorf(
				"%w: the service category is fixed while a source is linked", ErrPurchaseOrderInvalid)
		}
		if _, err := PurchaseOrderPPNBps(
			basis.TaxZone, basis.TaxType, basis.ServiceCategory, basis.TaxCategory); err != nil {
			return purchaseOrderBasis{}, err
		}
		// The stored PPN rate was frozen at creation; populating a legacy label
		// never recalculates the money.
		basis.PpnBps = lock.PpnBps
		return basis, nil
	}

	currencyCode := lock.Currency
	if strings.TrimSpace(input.Currency) != "" {
		validated, err := checkDraftCurrency(input.Currency)
		if err != nil {
			return purchaseOrderBasis{}, err
		}
		currencyCode = validated
	}
	zone := strings.TrimSpace(input.TaxZone)
	if zone == "" {
		zone = lock.TaxZone
	}
	if zone == "" {
		zone = "FTZ"
	}
	taxType := strings.TrimSpace(input.TaxType)
	if taxType == "" {
		taxType = lock.TaxType
	}
	serviceCategory := strings.TrimSpace(input.ServiceCategory)
	if serviceCategory == "" && taxType == "Services" {
		serviceCategory = lock.ServiceCategory
		if serviceCategory == "" {
			serviceCategory = "NonConstruction"
		}
	}
	if taxType == "Goods" {
		serviceCategory = ""
	}
	legacyCategory := strings.TrimSpace(input.TaxCategory)
	if legacyCategory == "" {
		legacyCategory = lock.TaxCategory
	}
	basis := purchaseOrderBasis{
		Currency:        currencyCode,
		TaxZone:         zone,
		TaxType:         taxType,
		ServiceCategory: serviceCategory,
		TaxCategory:     legacyCategory,
	}
	// A former row keeps its stored PPN rate when the save leaves the effective
	// commercial basis alone: the editor submits the display-derived legacy
	// labels, and those are not a tax edit. Only a real basis or zone change
	// recomputes the money, so a pre-039 row cannot silently gain PPN.
	if lock.SourceState == PurchaseOrderSourceStateFormer && sameFormerCommercialBasis(lock, basis) {
		basis.PpnBps = lock.PpnBps
		return basis, nil
	}
	ppnBps, err := PurchaseOrderPPNBps(zone, taxType, serviceCategory, legacyCategory)
	if err != nil {
		return purchaseOrderBasis{}, err
	}
	basis.PpnBps = ppnBps
	return basis, nil
}

// legacyBasisFor resolves the stored legacy category to the type/service an
// editor derives for display. An empty category is legacy FTZ/Goods.
func legacyBasisFor(category string) (taxType, serviceCategory string, ok bool) {
	if strings.TrimSpace(category) == "" {
		return "Goods", "", true
	}
	return legacyPOToTaxType(strings.TrimSpace(category))
}

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
// recomputed from the new inputs and stored with them. Only drafts may be
// written here, because a saved record's corrections belong to
// CorrectPurchaseOrder (#473).
//
// The save states the version the editor loaded. A stale version is refused by
// name with who saved and when; when the caller chose to overwrite after that
// warning, the replaced draft -- including the attachment it referenced -- is
// kept as an immutable snapshot before the write.
func (s *Store) UpdatePurchaseOrderDraft(ctx context.Context, input PurchaseOrderUpdate) (PurchaseOrder, error) {
	org, err := s.RequireOrg(ctx)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if input.ID < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if input.ExpectedVersion < 1 {
		return PurchaseOrder{}, ErrPurchaseOrderVersionRequired
	}
	input.PONumber = strings.TrimSpace(input.PONumber)
	input.PaymentTerms = strings.TrimSpace(input.PaymentTerms)
	if err := validatePurchaseOrderUpdateInput(input); err != nil {
		return PurchaseOrder{}, err
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

	lock, err := lockPurchaseOrderWrite(ctx, tx, schema, org.ID, input.ID,
		input.ExpectedVersion, PurchaseOrderStateDraft)
	if err != nil {
		return PurchaseOrder{}, err
	}
	// #496: a never-linked or formerly-linked draft still saves its own
	// PO-owned values. Only the paths that need the frozen revision (review,
	// finalize, corrections, repairs) require a live source.
	basis, err := resolvePurchaseOrderUpdateBasis(lock, input)
	if err != nil {
		return PurchaseOrder{}, err
	}

	if input.Overwrite {
		stored, err := getPurchaseOrder(ctx, tx, schema, org.ID, input.ID)
		if err != nil {
			return PurchaseOrder{}, err
		}
		if _, err := insertPurchaseOrderCorrection(ctx, tx, schema, stored, org.ID,
			PurchaseOrderCorrectionKindDraftOverwrite, PurchaseOrderDraftOverwriteReason,
			input.UpdatedBy); err != nil {
			return PurchaseOrder{}, err
		}
	}

	// Reference validation needs a frozen revision to validate against. A
	// source-less draft has none; its preserved references stay as entered and
	// are checked again if and when a source is linked (#497).
	if lock.SourceState == PurchaseOrderSourceStateLive {
		if err := validatePurchaseOrderReferences(ctx, tx, schema,
			lock.QuotationID, lock.SourceRevision, input.Items); err != nil {
			return PurchaseOrder{}, err
		}
	}
	lines, calculation, err := calculatePurchaseOrderLines(
		input.Items, input.DiscountPercent, input.AdjustmentMinor,
		input.AdjustmentReason, basis.PpnBps)
	if err != nil {
		return PurchaseOrder{}, err
	}

	// A draft keeps its number free while staff prepare it: the key is cleared,
	// so two people may still compete for one order. Only the final save writes
	// it.
	if err := updatePurchaseOrderEditable(ctx, tx, schema, org.ID, input, calculation, basis, nil); err != nil {
		return PurchaseOrder{}, err
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
	var confirmationNumber pgtype.Text
	var confirmationSequence pgtype.Int8
	var confirmationYear, confirmationCustomerCode pgtype.Int2
	var confirmationAssignedAt pgtype.Timestamptz
	var confirmationAssignedBy pgtype.Int8
	var customerID pgtype.Int8
	var sourceRevision pgtype.Int2
	err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT po.id, po.version, po.uuid::text, po.org_id::text,
		       COALESCE(po.quotation_id, po.former_quotation_id, 0),
		       po.source_revision, COALESCE(q.quote_number, po.former_quote_number, ''),
		       po.state, po.customer_id,
		       po.customer_name, po.billing_address, po.delivery_address,
		       po.ship_to_code, po.contact_email, po.contact_phone,
		       COALESCE(po.po_number, ''), po.po_date, po.received_date,
		       po.requested_delivery_date, po.payment_terms, po.incoterms,
		       po.currency, po.tax_zone, po.tax_type, po.service_category,
		       po.tax_category, po.created_by,
		       po.updated_by, po.created_at, po.updated_at,
		       po.subtotal_minor, po.discount_minor, po.ppn_bps, po.ppn_minor,
		       po.adjustment_minor, po.adjustment_reason, po.grand_total_minor,
		       po.discount_percent, po.printed_subtotal_minor,
		       po.printed_discount_minor, po.printed_ppn_minor,
		       po.printed_grand_total_minor, po.saved_at, po.saved_by,
		       (po.pdf_absent_confirmed_at IS NOT NULL
		        AND po.pdf_absent_confirmed_by IS NOT NULL),
		       po.confirmation_number, po.confirmation_sequence,
		       po.confirmation_year, po.confirmation_customer_code,
		       po.confirmation_assigned_at, po.confirmation_assigned_by,
		       CASE WHEN po.quotation_id IS NOT NULL THEN 'live'
		            WHEN po.former_quotation_id IS NOT NULL THEN 'former'
		            ELSE 'never' END
		FROM %s.purchase_orders po
		LEFT JOIN %s.quotations q ON q.id = po.quotation_id
		WHERE po.id = $1 AND po.org_id = $2`, schema, schema), id, orgID).Scan(
		&order.ID,
		&order.Version,
		&order.UUID,
		&order.OrgID,
		&order.QuotationID,
		&sourceRevision,
		&order.QuoteNumber,
		&order.State,
		&customerID,
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
		&order.TaxType,
		&order.ServiceCategory,
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
		&confirmationNumber,
		&confirmationSequence,
		&confirmationYear,
		&confirmationCustomerCode,
		&confirmationAssignedAt,
		&confirmationAssignedBy,
		&order.SourceState,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PurchaseOrder{}, ErrPurchaseOrderNotFound
	}
	if err != nil {
		return PurchaseOrder{}, fmt.Errorf("load purchase order %d: %w", id, err)
	}
	if sourceRevision.Valid {
		order.SourceRevision = sourceRevision.Int16
	}
	if customerID.Valid {
		order.CustomerID = customerID.Int64
	}
	order.SourceMissing = order.SourceState != PurchaseOrderSourceStateLive
	order.PrintedTotalsKnown = order.SourceState != PurchaseOrderSourceStateNever
	if savedAt.Valid {
		value := savedAt.Time
		order.SavedAt = &value
	}
	if savedBy.Valid {
		value := savedBy.Int64
		order.SavedBy = &value
	}
	if confirmationNumber.Valid {
		value := confirmationNumber.String
		order.ConfirmationNumber = &value
	}
	if confirmationSequence.Valid {
		value := confirmationSequence.Int64
		order.ConfirmationSequence = &value
		order.ConfirmationWarn = value >= POConfirmationWarnAt
	}
	if confirmationYear.Valid {
		value := confirmationYear.Int16
		order.ConfirmationYear = &value
	}
	if confirmationCustomerCode.Valid {
		value := confirmationCustomerCode.Int16
		order.ConfirmationCustomerCode = &value
	}
	if confirmationAssignedAt.Valid {
		value := confirmationAssignedAt.Time
		order.ConfirmationAssignedAt = &value
	}
	if confirmationAssignedBy.Valid {
		value := confirmationAssignedBy.Int64
		order.ConfirmationAssignedBy = &value
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

// isMissingQuotationConflict names the foreign-key failure a creation race with
// a permanent Quotation deletion produces. It is the race guard for #480: the
// deletion holds the quotation row FOR UPDATE, so an insert either completes
// first (and the deletion stops on a changed chain) or waits and then finds no
// parent.
func isMissingQuotationConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23503" && pgErr.ConstraintName == "purchase_orders_quotation_id_fkey"
}
