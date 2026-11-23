package apicontract

import (
	"encoding/json"
	"fmt"
)

// This file defines the shared HTTP contract between veraserver and the
// desktop client. Field names are stable API; the desktop code-generates or
// mirrors these shapes in #80.

// MaxNoteLength is the maximum length of a quotation-level note, in
// characters. The migration's CHECK, the store's validation and Prepare all
// enforce this one number, and the editor's counter shows it.
const MaxNoteLength = 1000

// FormatQuoteNumber renders a plain-digit quotation number: 15 + 2-digit
// customer code + 2-digit issue year + sequence. The normal width is 10
// digits (0001-9999); past 9999 the sequence widens to 5 digits instead of
// wrapping, so overflow years print 11 digits. No grouped display anywhere.
// It lives here (not backend) so store can build numbers without importing
// backend, which would be an import cycle.
func FormatQuoteNumber(customerCode, yy int, seq int64) string {
	return fmt.Sprintf("15%02d%02d%04d", customerCode, yy, seq)
}

// POConfirmationNumberPrefix replaces the quotation's "15" with "23". It is
// the only part of the number that is not shared with the quotation format.
const POConfirmationNumberPrefix = "23"

// FormatPOConfirmationNumber renders a purchase order's optional
// customer-system confirmation reference: 23 + 2-digit customer code +
// 2-digit assignment year + sequence. The normal width is 10 digits
// (0001-9999); past 9999 the sequence widens to 5 digits instead of wrapping,
// so an overflow cycle prints 11 digits. The quotation's prefix and
// independent customer-year counter are what keep the two series apart; the
// customer code, year rule and warning threshold are shared.
func FormatPOConfirmationNumber(customerCode, yy int, seq int64) string {
	return fmt.Sprintf("%s%02d%02d%04d", POConfirmationNumberPrefix, customerCode, yy, seq)
}

// This file defines the shared HTTP contract between veraserver and the
// desktop client. Field names are stable API; the desktop code-generates or
// mirrors these shapes in #80.

// ExportResponse identifies the persisted revision.
type ExportResponse struct {
	QuoteNumber  string `json:"quoteNumber"`
	UUID         string `json:"uuid,omitempty"`
	ShortID      string `json:"shortId"`
	ID           int64  `json:"id,omitempty"`
	GrandTotal   uint64 `json:"grandTotal"`
	Revision     int16  `json:"revision"`
	SequenceWarn bool   `json:"sequenceWarn,omitempty"`
}

// PeekNumberResponse previews the next allocatable quote number without
// consuming it. Warn turns true once the customer-year cycle reaches the
// alert threshold.
type PeekNumberResponse struct {
	Number string `json:"number"`
	Warn   bool   `json:"warn"`
}

// DraftSummaryRow is one row of the drafts list. The payload is absent by
// design: the list renders four columns and should not pull every saved
// quotation's editor state over the wire to do it.
type DraftSummaryRow struct {
	Label        string `json:"label"`
	Subject      string `json:"subject"`
	CustomerName string `json:"customerName"`
	UpdatedAt    string `json:"updatedAt"`
	DisplayName  string `json:"displayName"`
	ID           int64  `json:"id"`
	ItemCount    int    `json:"itemCount"`
}

// DraftRow is one draft with its editor state. Payload stays raw JSON rather
// than being typed here: a draft is unvalidated scratchpad state, so the
// server has no business knowing the editor's shape.
type DraftRow struct {
	Label        string          `json:"label"`
	Subject      string          `json:"subject"`
	CustomerName string          `json:"customerName"`
	UpdatedAt    string          `json:"updatedAt"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    string          `json:"createdAt"`
	ID           int64           `json:"id"`
	ItemCount    int             `json:"itemCount"`
	CustomerID   *int64          `json:"customerId,omitempty"`
}

// QuotationSummaryRow is one archive row: always the latest revision.
type QuotationSummaryRow struct {
	QuoteNumber    string `json:"quoteNumber"`
	CustomerName   string `json:"customerName"`
	Subject        string `json:"subject"`
	Currency       string `json:"currency,omitempty"`
	CreatedAt      string `json:"createdAt"`
	DisplayGrand   string `json:"displayGrand"`
	ID             int64  `json:"id"`
	GrandTotal     int64  `json:"grandTotal"`
	RevisionNumber int16  `json:"revision"`
	// Closed is the archive badge (#656). Closure is quotation-level, so every
	// revision of a closed quotation reads closed.
	Closed bool `json:"closed"`
}

// POSourceActivePORow is the active Purchase Order occupying a Quotation
// (issue #468). A draft offers "Open draft"; a saved record stays disabled.
type POSourceActivePORow struct {
	ID    int64  `json:"id"`
	State string `json:"state,omitempty"`
}

// POSourceQuotationRow is one selectable Quotation in the PO source selector
// (issue #467). A row always describes the latest issued revision, with the
// number of issued revisions beside it so the second step knows whether a
// choice exists. ActivePO is null when no PO occupies the Quotation.
type POSourceQuotationRow struct {
	ActivePO            *POSourceActivePORow `json:"activePO"`
	ID                  int64                `json:"id"`
	QuoteNumber         string               `json:"quoteNumber"`
	Subject             string               `json:"subject"`
	CustomerName        string               `json:"customerName"`
	LatestRevision      int16                `json:"latestRevision"`
	IssuedRevisionCount int                  `json:"issuedRevisionCount"`
	// Bought-state badges (#651). AllBought disables a new PO; a partial
	// quotation stays selectable and shows its remaining leaf count.
	AllBought            bool `json:"allBought"`
	LeafCount            int  `json:"leafCount"`
	RemainingLeafCount   int  `json:"remainingLeafCount"`
	SavedPOCount         int  `json:"savedPoCount"`
	DraftPOCount         int  `json:"draftPoCount"`
	LegacyHeaderDiscount bool `json:"legacyHeaderDiscount"`
	// Closure and expiry (#656). A closed row stays visible and disabled; its
	// unbought remainder is Closed Loss, not Unsold. ExpiredUnclosed is an
	// advisory badge only: expiry alone never becomes a loss.
	Closed          bool    `json:"closed"`
	ClosedAt        *string `json:"closedAt"`
	ClosedReason    string  `json:"closedReason"`
	ValidUntil      *string `json:"validUntil"`
	ExpiredUnclosed bool    `json:"expiredUnclosed"`
}

// POSourceListResponse is one page of the selector's first step. PageSize is
// fixed by the contract (15), not chosen by the caller.
type POSourceListResponse struct {
	Quotations []POSourceQuotationRow `json:"quotations"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	Total      int                    `json:"total"`
	TotalPages int                    `json:"totalPages"`
}

// POSourceRevisionRow is one issued revision of the selected Quotation.
type POSourceRevisionRow struct {
	Revision int16  `json:"revision"`
	IssuedAt string `json:"issuedAt"`
}

// POSourceRevisionsResponse is the second step's data: issued revisions only,
// newest first. A list of one is the sole-revision case the selector
// auto-selects.
type POSourceRevisionsResponse struct {
	QuotationID  int64                 `json:"quotationId"`
	QuoteNumber  string                `json:"quoteNumber"`
	Subject      string                `json:"subject"`
	CustomerName string                `json:"customerName"`
	Revisions    []POSourceRevisionRow `json:"revisions"`
}

// POSourceValidateRequest is the confirm-time revalidation: the exact pair the
// selector is about to return.
//
// RevisionNumber is a pointer because revision 0 is a real revision: an
// absent or null field cannot mean zero, so nil is what tells the handler the
// request is incomplete (400). A pointer lets `{revisionNumber: 0}` stay valid.
type POSourceValidateRequest struct {
	QuotationID    int64  `json:"quotationId"`
	RevisionNumber *int16 `json:"revisionNumber"`
}

// POSourceValidationResponse answers whether the pair still exists and is
// issued. A well-formed request always answers 200; the outcome is Valid, and
// Reason names why not when it is false.
type POSourceValidationResponse struct {
	Quotation      *POSourceQuotationRow `json:"quotation,omitempty"`
	Reason         string                `json:"reason,omitempty"`
	QuotationID    int64                 `json:"quotationId"`
	RevisionNumber int16                 `json:"revisionNumber"`
	Valid          bool                  `json:"valid"`
}

// POSourceOccupancyResponse is the archived Quotation detail's occupancy read
// (issue #499). It answers for one Quotation, not one revision: ActivePO is the
// one record occupying the whole Quotation, or null when it is free for a
// creation. The detail page uses it to choose between Create PO from this
// Quotation, Open PO draft, and View PO.
type POSourceOccupancyResponse struct {
	ActivePO             *POSourceActivePORow `json:"activePO"`
	QuotationID          int64                `json:"quotationId"`
	AllBought            bool                 `json:"allBought"`
	RemainingLeafCount   int                  `json:"remainingLeafCount"`
	SavedPOCount         int                  `json:"savedPoCount"`
	DraftPOCount         int                  `json:"draftPoCount"`
	LegacyHeaderDiscount bool                 `json:"legacyHeaderDiscount"`
	// Closure and expiry (#656). Closed disables creation; ClosedByName is the
	// detail page's "who", which the selector rows do not need.
	Closed          bool    `json:"closed"`
	ClosedAt        *string `json:"closedAt"`
	ClosedReason    string  `json:"closedReason"`
	ClosedByName    string  `json:"closedByName"`
	ValidUntil      *string `json:"validUntil"`
	ExpiredUnclosed bool    `json:"expiredUnclosed"`
}

// POSourceAvailabilityItemRow is one priced leaf of the selected revision.
// QuotedUnitPriceMinor is the row-discounted customer price; supplier cost is
// never part of this shape (Story 42).
type POSourceAvailabilityItemRow struct {
	QuotationItemNo          int      `json:"quotationItemNo"`
	QuotationSubNo           *int     `json:"quotationSubNo"`
	Label                    string   `json:"label"`
	Description              string   `json:"description"`
	ParentDescription        string   `json:"parentDescription,omitempty"`
	GroupKey                 string   `json:"groupKey,omitempty"`
	DescriptionLines         []string `json:"descriptionLines,omitempty"`
	UoM                      string   `json:"uom"`
	QuotedQuantityScaled     int64    `json:"quotedQuantityScaled"`
	BoughtQuantityScaled     int64    `json:"boughtQuantityScaled"`
	DraftQuantityScaled      int64    `json:"draftQuantityScaled"`
	RemainingQuantityScaled  int64    `json:"remainingQuantityScaled"`
	OverBoughtQuantityScaled int64    `json:"overBoughtQuantityScaled"`
	QuotedUnitPriceMinor     int64    `json:"quotedUnitPriceMinor"`
}

// POSourceAvailabilityResponse is the server-side bought state for one
// quotation revision. AllBought is derived from the listed leaves; a draft
// quantity is advisory and never reduces RemainingQuantityScaled.
type POSourceAvailabilityResponse struct {
	QuotationID          int64 `json:"quotationId"`
	RevisionNumber       int16 `json:"revisionNumber"`
	LatestRevision       int16 `json:"latestRevision"`
	AllBought            bool  `json:"allBought"`
	LegacyHeaderDiscount bool  `json:"legacyHeaderDiscount"`
	SavedPOCount         int   `json:"savedPoCount"`
	DraftPOCount         int   `json:"draftPoCount"`
	// Closure and expiry (#656). A closed revision's remaining leaves are
	// Closed Loss; the workspace disables ordering rather than hiding them.
	Closed          bool                          `json:"closed"`
	ClosedAt        *string                       `json:"closedAt"`
	ClosedReason    string                        `json:"closedReason"`
	ValidUntil      *string                       `json:"validUntil"`
	ExpiredUnclosed bool                          `json:"expiredUnclosed"`
	Items           []POSourceAvailabilityItemRow `json:"items"`
}

// PurchaseOrderItemRow is one entered PO line. QuotationItemNo and
// QuotationSubNo are null for an extra line the source revision never carried.
// QuantityScaled is an exact count of 1/10000 units; UnitPriceMinor is in the
// PO currency's minor units.
//
// GrossMinor, DiscountMinor and NetMinor are the stored result of this row's
// own calculation. They are returned, not recomputed, so a reopened order shows
// the arithmetic it was saved with.
type PurchaseOrderItemRow struct {
	QuotationItemNo *int `json:"quotationItemNo"`
	QuotationSubNo  *int `json:"quotationSubNo"`
	// LineKind is quotation or extra (#655). A quotation line fulfils a leaf of
	// the source revision and owns an allocation; an extra was never on the
	// quotation, carries no reference and no allocation, and reports as a
	// pass-through expense. The read always names it, so an older server's
	// empty value is the only "absent quotation" a reader ever sees.
	LineKind          string   `json:"lineKind"`
	ID                int64    `json:"id"`
	ClientKey         *string  `json:"clientKey"`
	LineNumber        int      `json:"lineNumber"`
	Description       string   `json:"description"`
	QuantityScaled    int64    `json:"quantityScaled"`
	UoM               string   `json:"uom"`
	UnitPriceMinor    int64    `json:"unitPriceMinor"`
	DiscountEnabled   bool     `json:"discountEnabled"`
	DiscountPercent   int16    `json:"discountPercent"`
	GrossMinor        int64    `json:"grossMinor"`
	DiscountMinor     int64    `json:"discountMinor"`
	NetMinor          int64    `json:"netMinor"`
	GroupKey          string   `json:"groupKey"`
	ParentDescription string   `json:"parentDescription"`
	DescriptionLines  []string `json:"descriptionLines"`
	// QuotedUnitPriceMinor is the frozen row-discounted quotation baseline.
	// The editor compares the negotiated unit price against it for the price
	// loss / extra-income badge. It is customer-facing and never supplier cost.
	QuotedUnitPriceMinor *int64 `json:"quotedUnitPriceMinor,omitempty"`
	// RemainingQuantityScaled is what is still orderable on this leaf after
	// other saved purchase orders. Absent when there is no live source answer.
	RemainingQuantityScaled *int64 `json:"remainingQuantityScaled,omitempty"`
	// AggregateGroupID names the aggregate presentation group this line belongs
	// to. Null means the line is ungrouped. The id is internal and not stable
	// across saves; the correction snapshot keys groups by display order.
	AggregateGroupID *int64 `json:"aggregateGroupId,omitempty"`
}

// PurchaseOrderAggregateGroupRow is one PO-only presentation group. It groups
// one or more existing PO lines for the customer document; the lines remain the
// priced truth and carry their own allocations. TotalMinor and
// DisplayUnitPriceMinor are derived on read, never stored, and no group row
// carries cost, margin or variance (Story 42).
type PurchaseOrderAggregateGroupRow struct {
	ID                     int64  `json:"id"`
	DisplayOrder           int    `json:"displayOrder"`
	Title                  string `json:"title"`
	OrderedQuantityScaled  int64  `json:"orderedQuantityScaled"`
	OrderedUoM             string `json:"orderedUom"`
	ShowChildrenToCustomer bool   `json:"showChildrenToCustomer"`
	TotalMinor             int64  `json:"totalMinor"`
	// DisplayUnitPriceMinor is round_half_away(TotalMinor/qty). Null when the
	// exact value cannot be represented safely; the total stays authoritative.
	DisplayUnitPriceMinor *int64 `json:"displayUnitPriceMinor"`
}

// PurchaseOrderAggregateGroupInput is one submitted group. Its position in the
// request slice is the handle items reference through AggregateGroupIndex;
// display order is derived from the first child's line number by the server.
type PurchaseOrderAggregateGroupInput struct {
	Title                  string `json:"title"`
	OrderedQuantityScaled  int64  `json:"orderedQuantityScaled"`
	OrderedUoM             string `json:"orderedUom"`
	ShowChildrenToCustomer bool   `json:"showChildrenToCustomer"`
}

// PurchaseOrderRow is one PO draft or saved record. Dates are plain
// YYYY-MM-DD; the optional customer PO date and requested delivery date are
// null when unset.
//
// The calculated figures (SubtotalMinor .. GrandTotalMinor) and the printed
// source figures are separate on purpose: the first pair is Vera's arithmetic,
// the second is what the issued quotation printed. A PO never overwrites the
// evidence it is checked against.
type PurchaseOrderRow struct {
	PODate                *string                `json:"poDate"`
	RequestedDeliveryDate *string                `json:"requestedDeliveryDate"`
	UUID                  string                 `json:"uuid"`
	State                 string                 `json:"state"`
	QuoteNumber           string                 `json:"quoteNumber"`
	CustomerName          string                 `json:"customerName"`
	BillingAddress        string                 `json:"billingAddress"`
	DeliveryAddress       string                 `json:"deliveryAddress"`
	ShipToCode            string                 `json:"shipToCode"`
	ContactEmail          string                 `json:"contactEmail"`
	ContactPhone          string                 `json:"contactPhone"`
	PONumber              string                 `json:"poNumber"`
	ReceivedDate          string                 `json:"receivedDate"`
	Incoterms             string                 `json:"incoterms"`
	Currency              string                 `json:"currency"`
	TaxZone               string                 `json:"taxZone,omitempty"`
	TaxType               string                 `json:"taxType,omitempty"`
	ServiceCategory       string                 `json:"serviceCategory,omitempty"`
	TaxCategory           string                 `json:"taxCategory,omitempty"`
	AdjustmentReason      string                 `json:"adjustmentReason"`
	CreatedAt             string                 `json:"createdAt"`
	UpdatedAt             string                 `json:"updatedAt"`
	Items                 []PurchaseOrderItemRow `json:"items"`
	// AggregateGroups are the record's PO-only presentation headers, ordered by
	// display order. Absent on a record with no groups, so an old reader sees
	// exactly the payload it always did.
	AggregateGroups        []PurchaseOrderAggregateGroupRow `json:"aggregateGroups,omitempty"`
	ID                     int64                            `json:"id"`
	QuotationID            int64                            `json:"quotationId"`
	SourceRevision         int16                            `json:"sourceRevision,omitempty"`
	CustomerID             int64                            `json:"customerId"`
	PaymentTerms           string                           `json:"paymentTerms"`
	DiscountPercent        int16                            `json:"discountPercent"`
	PpnBps                 int16                            `json:"ppnBps"`
	SubtotalMinor          int64                            `json:"subtotalMinor"`
	DiscountMinor          int64                            `json:"discountMinor"`
	PpnMinor               int64                            `json:"ppnMinor"`
	AdjustmentMinor        int64                            `json:"adjustmentMinor"`
	GrandTotalMinor        int64                            `json:"grandTotalMinor"`
	PrintedSubtotalMinor   int64                            `json:"printedSubtotalMinor"`
	PrintedDiscountMinor   int64                            `json:"printedDiscountMinor"`
	PrintedPpnMinor        int64                            `json:"printedPpnMinor"`
	PrintedGrandTotalMinor int64                            `json:"printedGrandTotalMinor"`
	// Finalization (#471). SavedAt/SavedBy are null on a draft; they are set
	// exactly once by the draft -> saved transition. PdfAbsentConfirmed records
	// the deliberate confirmation a save needs while no customer PDF exists.
	SavedAt            *string `json:"savedAt"`
	SavedBy            *int64  `json:"savedBy"`
	PdfAbsentConfirmed bool    `json:"pdfAbsentConfirmed"`
	// SourceMissing is true for both a never-linked draft and a record whose
	// source Quotation was permanently deleted. SourceState names which one it
	// is; it is the only presence signal, because revision 00 is a real link
	// and the numeric fields stay numeric for older readers.
	SourceMissing bool `json:"sourceMissing"`
	// SourceState is "live", "never" or "former" (#496).
	SourceState string `json:"sourceState"`
	// PrintedTotalsKnown is false exactly for a never-linked draft, whose
	// printed source figures do not exist yet. It is never inferred from a
	// zero total: a real quotation may print 0.
	PrintedTotalsKnown bool `json:"printedTotalsKnown"`
	// The optional customer-system confirmation reference (#474). It is null
	// until assigned, and then immutable. ConfirmationWarn turns true once the
	// customer-year cycle reaches the shared 8000 warning threshold.
	ConfirmationNumber     *string `json:"confirmationNumber"`
	ConfirmationAssignedAt *string `json:"confirmationAssignedAt"`
	ConfirmationWarn       bool    `json:"confirmationWarn"`
	// ConfirmationRequested is the draft's request for a confirmation number
	// (#581). It allocates nothing and is independent of any assigned number.
	ConfirmationRequested bool `json:"confirmationRequested"`
	// Version is the optimistic-concurrency revision (#473). It starts at 1
	// and moves forward on every write; a save must state the version it
	// loaded.
	Version int64 `json:"version"`

	// BusinessCategory is the #628 reporting category (FM, FAB, SUPPLY_GEN,
	// OTHERS). Empty is Unassigned: a draft that has not chosen one, or a
	// record saved before the category existed. It is required by the final
	// save and never inferred.
	BusinessCategory string `json:"businessCategory"`
}

// PurchaseOrderSourceLineRow is one customer-facing line of the exact source
// revision. LineLabel is how the customer document numbers it: "3" for an
// item, "3.1" for a priced part of a package.
type PurchaseOrderSourceLineRow struct {
	QuotationItemNo   int      `json:"quotationItemNo"`
	QuotationSubNo    *int     `json:"quotationSubNo"`
	LineLabel         string   `json:"lineLabel"`
	Description       string   `json:"description"`
	UoM               string   `json:"uom"`
	QuantityScaled    int64    `json:"quantityScaled"`
	UnitPriceMinor    int64    `json:"unitPriceMinor"`
	DiscountEnabled   bool     `json:"discountEnabled"`
	DiscountPercent   int16    `json:"discountPercent"`
	ParentDescription string   `json:"parentDescription"`
	DescriptionLines  []string `json:"descriptionLines"`
	// QuotedUnitPriceMinor is the frozen row-discounted quotation baseline.
	// UnitPriceMinor above is the same customer-facing price for a new PO; the
	// explicit field lets the workspace state the baseline without re-deriving
	// it from the printed list price.
	QuotedUnitPriceMinor int64 `json:"quotedUnitPriceMinor"`
}

// PurchaseOrderSourceRow is the exact source revision the review compares
// against. It is read from the frozen revision on every review, never from a
// cache, so a corrected match is always checked against the issued document.
type PurchaseOrderSourceRow struct {
	QuotationID    int64  `json:"quotationId"`
	RevisionNumber int16  `json:"revisionNumber"`
	QuoteNumber    string `json:"quoteNumber"`
	CustomerID     int64  `json:"customerId"`
	CustomerName   string `json:"customerName"`
	Currency       string `json:"currency"`
	PaymentTerms   string `json:"paymentTerms"`
	Incoterms      string `json:"incoterms"`
	// TaxZone, TaxType and ServiceCategory are the exact revision's
	// normalized semantic triple (#502 ticket 05, B4). Additive surface for
	// the link-time tax evidence; no billing comparison is added.
	TaxZone                string                       `json:"taxZone"`
	TaxType                string                       `json:"taxType"`
	ServiceCategory        string                       `json:"serviceCategory"`
	DiscountPercent        int16                        `json:"discountPercent"`
	Lines                  []PurchaseOrderSourceLineRow `json:"lines"`
	PrintedSubtotalMinor   int64                        `json:"printedSubtotalMinor"`
	PrintedDiscountMinor   int64                        `json:"printedDiscountMinor"`
	PrintedPpnMinor        int64                        `json:"printedPpnMinor"`
	PrintedGrandTotalMinor int64                        `json:"printedGrandTotalMinor"`
}

// PurchaseOrderReviewRow is one record beside its source plus the document
// branch the final save is in. DocumentState is "none" while no attachment
// exists (#472 adds "attached"); the client shows the no-PDF confirmation only
// in that branch.
type PurchaseOrderReviewRow struct {
	Order         PurchaseOrderRow       `json:"order"`
	Source        PurchaseOrderSourceRow `json:"source"`
	DocumentState string                 `json:"documentState"`
}

// PurchaseOrderDocumentRow is one stored version of the single customer PO
// PDF. Active is true for the one current document; ReplacedAt marks a version
// superseded by a newer upload, and RemovedAt/RemovedBy mark a deliberate
// removal. The bytes are never part of this row: content is served by its own
// endpoint so a history listing cannot copy every version into memory.
type PurchaseOrderDocumentRow struct {
	UUID             string  `json:"uuid"`
	SHA256           string  `json:"sha256"`
	ContentType      string  `json:"contentType"`
	OriginalFilename string  `json:"originalFilename"`
	UploadedAt       string  `json:"uploadedAt"`
	ReplacedAt       *string `json:"replacedAt"`
	RemovedAt        *string `json:"removedAt"`
	RemovedBy        *int64  `json:"removedBy"`
	ID               int64   `json:"id"`
	PurchaseOrderID  int64   `json:"purchaseOrderId"`
	Version          int32   `json:"version"`
	SizeBytes        int64   `json:"sizeBytes"`
	UploadedBy       int64   `json:"uploadedBy"`
	Active           bool    `json:"active"`
}

// PurchaseOrderDocumentStateRow is the current document plus retained history,
// newest first. Document is null when no PDF is attached; Versions is empty
// then, not null.
type PurchaseOrderDocumentStateRow struct {
	Document *PurchaseOrderDocumentRow  `json:"document"`
	Versions []PurchaseOrderDocumentRow `json:"versions"`
}

// PurchaseOrderDocumentRemovalRequest is the deliberate removal. Confirmed
// must be true; an empty body is not consent.
type PurchaseOrderDocumentRemovalRequest struct {
	Confirmed bool `json:"confirmed"`
}

// PurchaseOrderDecisionRequest is one explicit final-save choice. Choice is
// "po" or "quotation"; neither is preselected by the client.
type PurchaseOrderDecisionRequest struct {
	Key    string `json:"key"`
	Choice string `json:"choice"`
}

// PurchaseOrderFinalizeRequest replaces the editable fields exactly as a draft
// save does, and adds the difference-review decisions, the explicit
// missing/extra acknowledgements, and the document confirmation.
type PurchaseOrderFinalizeRequest struct {
	PurchaseOrderEditableRequest
	Decisions           []PurchaseOrderDecisionRequest `json:"decisions"`
	AcknowledgedMissing []string                       `json:"acknowledgedMissing"`
	AcknowledgedExtra   []int                          `json:"acknowledgedExtra"`
	DocumentConfirmed   bool                           `json:"documentConfirmed"`
	// AssignConfirmationNumber asks the server to allocate the optional
	// customer-system reference inside the same transaction as the save. The
	// client never sends the number itself: only the server allocates, and an
	// already-assigned value is never regenerated (#474). It is also the one
	// source of the stored confirmation request (#579).
	AssignConfirmationNumber bool `json:"assignConfirmationNumber"`
}

// PurchaseOrderCreateRequest starts a draft from one exact issued revision.
// RevisionNumber is a pointer because revision 0 is real and an absent field
// must not read as it. ReceivedDate is optional; the server defaults it to
// today.
//
// #496: a request with no QuotationID (zero or absent) creates a source-less
// blank draft instead. RevisionNumber must then be absent. The tax fields are
// the user's Quotation defaults for a blank draft; the server validates them,
// seeds the currency from the company profile, and computes PPN from the
// persisted basis. A linked create ignores them because the revision owns the
// basis.
type PurchaseOrderCreateRequest struct {
	QuotationID     int64  `json:"quotationId"`
	RevisionNumber  *int16 `json:"revisionNumber"`
	ReceivedDate    string `json:"receivedDate,omitempty"`
	TaxZone         string `json:"taxZone,omitempty"`
	TaxType         string `json:"taxType,omitempty"`
	ServiceCategory string `json:"serviceCategory,omitempty"`
	TaxCategory     string `json:"taxCategory,omitempty"`
	// BusinessCategory is optional at creation: a draft may start Unassigned.
	// A supplied value must be one of the four codes.
	BusinessCategory string `json:"businessCategory,omitempty"`
	// Items selects the remaining quotation leaves for this draft. Empty means
	// every leaf that still has quantity (#651).
	Items []PurchaseOrderCreateItemRow `json:"items,omitempty"`
}

// PurchaseOrderCreateItemRow selects one remaining quotation leaf.
type PurchaseOrderCreateItemRow struct {
	QuotationItemNo int  `json:"quotationItemNo"`
	QuotationSubNo  *int `json:"quotationSubNo"`
}

// PurchaseOrderItemInput is one replace-lines entry. LineNumber is assigned by
// the server from array order, so drafts always carry 1..n without gaps. A row
// discount mirrors the quotation's: a flag plus a 0-100 percentage.
type PurchaseOrderItemInput struct {
	QuotationItemNo *int `json:"quotationItemNo"`
	QuotationSubNo  *int `json:"quotationSubNo"`
	// LineKind is quotation or extra (#655). It is omitempty on purpose: an
	// absent value is the compatibility quotation, so a client that predates
	// #655 keeps writing exactly the payload it always did.
	LineKind          string   `json:"lineKind,omitempty"`
	ID                *int64   `json:"id"`
	ClientKey         *string  `json:"clientKey"`
	Description       string   `json:"description"`
	QuantityScaled    int64    `json:"quantityScaled"`
	UoM               string   `json:"uom"`
	UnitPriceMinor    int64    `json:"unitPriceMinor"`
	DiscountEnabled   bool     `json:"discountEnabled"`
	DiscountPercent   int16    `json:"discountPercent"`
	GroupKey          string   `json:"groupKey"`
	ParentDescription string   `json:"parentDescription"`
	DescriptionLines  []string `json:"descriptionLines"`
	// AggregateGroupIndex is the 1-based position of the line's group in the
	// request's AggregateGroups; 0 or absent means ungrouped. It is an input
	// handle only and is never persisted.
	AggregateGroupIndex int `json:"aggregateGroupIndex,omitempty"`
}

// PurchaseOrderEditableRequest is the editable payload every replace-style
// purchase order request shares. ConfirmationRequested is deliberately absent:
// a draft save owns that preference (#581), while the final save derives the
// stored request from its own checkbox (#579).
type PurchaseOrderEditableRequest struct {
	PurchaseOrderProtectedFieldsRequest
	PODate                string                   `json:"poDate,omitempty"`
	RequestedDeliveryDate string                   `json:"requestedDeliveryDate,omitempty"`
	BillingAddress        string                   `json:"billingAddress"`
	DeliveryAddress       string                   `json:"deliveryAddress"`
	ShipToCode            string                   `json:"shipToCode"`
	ContactEmail          string                   `json:"contactEmail"`
	ContactPhone          string                   `json:"contactPhone"`
	PONumber              string                   `json:"poNumber"`
	ReceivedDate          string                   `json:"receivedDate,omitempty"`
	Incoterms             string                   `json:"incoterms"`
	AdjustmentReason      string                   `json:"adjustmentReason"`
	Items                 []PurchaseOrderItemInput `json:"items"`
	// AggregateGroups is the submitted aggregate presentation. It has no
	// omitempty on purpose: a client that understands groups always sends an
	// array (empty when the record has none), while a client that predates
	// #654 sends nothing and decodes to nil. The server refuses nil on a
	// grouped record (409 po_client_outdated) instead of silently ungrouping
	// it, so nil and empty are different contracts, not the same one.
	AggregateGroups []PurchaseOrderAggregateGroupInput `json:"aggregateGroups"`
	PaymentTerms    string                             `json:"paymentTerms"`
	DiscountPercent int16                              `json:"discountPercent"`
	AdjustmentMinor int64                              `json:"adjustmentMinor"`
	ExpectedVersion int64                              `json:"expectedVersion"`
	Overwrite       bool                               `json:"overwrite"`

	// BusinessCategory is the #628 reporting category. It is editable on a
	// draft and on a saved-record correction; a source repair preserves the
	// stored value when the field is absent.
	BusinessCategory string `json:"businessCategory"`
}

// PurchaseOrderUpdateRequest replaces the editable draft fields. Source,
// customer snapshots, currency and tax basis are not writable.
//
// ExpectedVersion is the version the editor loaded (#473); the server refuses a
// stale save by name. Overwrite records that the caller deliberately replaced a
// version it was warned about, so the replaced values are kept.
//
// The embedded protected names exist only so a client that submits one can be
// refused explicitly. None of them is ever written by an update or correction;
// the confirmation number, source link, customer, currency and identity have
// their own rules (#474, #475) and are not reachable here.
type PurchaseOrderUpdateRequest struct {
	PurchaseOrderEditableRequest
	// ConfirmationRequested remembers the editor's draft choice (#581). It
	// allocates nothing; an update writes it, and the final save does not
	// accept it at all (#579).
	ConfirmationRequested bool `json:"confirmationRequested"`
}

// PurchaseOrderProtectedFieldsRequest names the values that are protected from
// an edit. The fields are pointers so "absent or null" means "not submitted";
// a non-null value is refused with po_protected_field rather than ignored.
type PurchaseOrderProtectedFieldsRequest struct {
	ConfirmationNumber *string `json:"confirmationNumber,omitempty"`
	ConfirmationWarn   *bool   `json:"confirmationWarn,omitempty"`
	UUID               *string `json:"uuid,omitempty"`
	CustomerID         *int64  `json:"customerId,omitempty"`
	QuotationID        *int64  `json:"quotationId,omitempty"`
	SourceRevision     *int16  `json:"sourceRevision,omitempty"`
	Currency           *string `json:"currency,omitempty"`
	TaxZone            *string `json:"taxZone,omitempty"`
	TaxType            *string `json:"taxType,omitempty"`
	ServiceCategory    *string `json:"serviceCategory,omitempty"`
	TaxCategory        *string `json:"taxCategory,omitempty"`
	OrgID              *string `json:"orgId,omitempty"`
	State              *string `json:"state,omitempty"`
	SavedAt            *string `json:"savedAt,omitempty"`
	SavedBy            *int64  `json:"savedBy,omitempty"`
	PdfAbsentConfirmed *bool   `json:"pdfAbsentConfirmed,omitempty"`
	Version            *int64  `json:"version,omitempty"`
	ID                 *int64  `json:"id,omitempty"`
	PONumberKey        *string `json:"poNumberKey,omitempty"`
}

// purchaseOrderProtectedFields is the one ordered source of protected field
// names. Both accessors read it, so a new protected column is added in exactly
// one visible place and cannot be silently writable through the draft variant.
var purchaseOrderProtectedFields = []struct {
	name    string
	present func(PurchaseOrderProtectedFieldsRequest) bool
}{
	{"confirmationNumber", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.ConfirmationNumber != nil }},
	{"confirmationWarn", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.ConfirmationWarn != nil }},
	{"uuid", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.UUID != nil }},
	{"customerId", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.CustomerID != nil }},
	{"quotationId", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.QuotationID != nil }},
	{"sourceRevision", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.SourceRevision != nil }},
	{"currency", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.Currency != nil }},
	{"taxZone", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.TaxZone != nil }},
	{"taxType", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.TaxType != nil }},
	{"serviceCategory", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.ServiceCategory != nil }},
	{"taxCategory", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.TaxCategory != nil }},
	{"orgId", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.OrgID != nil }},
	{"state", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.State != nil }},
	{"savedAt", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.SavedAt != nil }},
	{"savedBy", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.SavedBy != nil }},
	{"pdfAbsentConfirmed", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.PdfAbsentConfirmed != nil }},
	{"version", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.Version != nil }},
	{"id", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.ID != nil }},
	{"poNumberKey", func(r PurchaseOrderProtectedFieldsRequest) bool { return r.PONumberKey != nil }},
}

// purchaseOrderSourceBasisFields is the exact set a draft save may carry:
// currency and the tax triple are editable before a source is linked (#496).
// Every other protected value is still refused, and the store freezes the
// basis on a row that already has a live source.
var purchaseOrderSourceBasisFields = map[string]struct{}{
	"currency":        {},
	"taxZone":         {},
	"taxType":         {},
	"serviceCategory": {},
	"taxCategory":     {},
}

// Named returns the first protected value the request carried, or "". The
// caller refuses it by name; the list is deliberately read in a fixed order so
// the answer is deterministic.
func (request PurchaseOrderProtectedFieldsRequest) Named() string {
	return request.firstProtected(nil)
}

// NamedExceptSourceBasis is the draft-update variant: the source-basis names
// are removed from the one ordered list rather than repeated as a second list.
func (request PurchaseOrderProtectedFieldsRequest) NamedExceptSourceBasis() string {
	return request.firstProtected(purchaseOrderSourceBasisFields)
}

func (request PurchaseOrderProtectedFieldsRequest) firstProtected(excluded map[string]struct{}) string {
	for _, field := range purchaseOrderProtectedFields {
		if _, skip := excluded[field.name]; skip {
			continue
		}
		if field.present(request) {
			return field.name
		}
	}
	return ""
}

// PurchaseOrderCorrectionRequest replaces a saved record. The payload is the
// same editable set a draft save carries plus the reason staff typed; the
// source link, customer, currency, tax basis, identity and confirmation number
// are protected exactly as they are for a draft save.
type PurchaseOrderCorrectionRequest struct {
	PurchaseOrderUpdateRequest
	Reason string `json:"reason"`
}

// PurchaseOrderSourceReviewRequest asks for the difference review of one
// saved record against a candidate revision (#475). It writes nothing; the
// target is named separately from the editable payload on purpose.
type PurchaseOrderSourceReviewRequest struct {
	TargetQuotationID    int64  `json:"targetQuotationId"`
	TargetRevisionNumber *int16 `json:"targetRevisionNumber"`
}

// PurchaseOrderDraftLinkReviewRequest asks for the Staff draft-link review
// (#497). It writes nothing; the target is the candidate source revision.
type PurchaseOrderDraftLinkReviewRequest struct {
	TargetQuotationID    int64  `json:"targetQuotationId"`
	TargetRevisionNumber *int16 `json:"targetRevisionNumber"`
}

// PurchaseOrderDraftLinkRequest commits one Staff draft link (#497). It carries
// no editable PO values: the working copy is saved as an ordinary draft first,
// and this request changes only the source link, the chosen tax basis and the
// recorded review. Protected identity values are still refused by name.
type PurchaseOrderDraftLinkRequest struct {
	PurchaseOrderProtectedFieldsRequest
	Decisions            []PurchaseOrderDecisionRequest `json:"decisions"`
	AcknowledgedMissing  []string                       `json:"acknowledgedMissing"`
	AcknowledgedExtra    []int                          `json:"acknowledgedExtra"`
	TargetQuotationID    int64                          `json:"targetQuotationId"`
	TargetRevisionNumber *int16                         `json:"targetRevisionNumber"`
	// ExpectedVersion is the version the editor loaded (#473). Like a draft
	// save, a link with a stale version is a named conflict.
	ExpectedVersion int64 `json:"expectedVersion"`
	// TaxChoice is "keep" or "source"; required only when the draft and source
	// tax bases differ. Empty is refused in that case, never adopted silently.
	TaxChoice string `json:"taxChoice"`
}

// PurchaseOrderDraftTaxBasisRow is one side of the link's tax decision: the
// persisted basis and the PPN rate the server derives from it.
type PurchaseOrderDraftTaxBasisRow struct {
	TaxZone         string `json:"taxZone"`
	TaxType         string `json:"taxType"`
	ServiceCategory string `json:"serviceCategory"`
	TaxCategory     string `json:"taxCategory"`
	PpnBps          int16  `json:"ppnBps"`
}

// PurchaseOrderDraftLinkReviewRow is the same review the final save reads plus
// the tax decision a draft link needs.
type PurchaseOrderDraftLinkReviewRow struct {
	Order             PurchaseOrderRow              `json:"order"`
	Source            PurchaseOrderSourceRow        `json:"source"`
	DocumentState     string                        `json:"documentState"`
	DraftTax          PurchaseOrderDraftTaxBasisRow `json:"draftTax"`
	SourceTax         PurchaseOrderDraftTaxBasisRow `json:"sourceTax"`
	TaxChoiceRequired bool                          `json:"taxChoiceRequired"`
}

// PurchaseOrderSourceRepairRequest moves a saved record to another issued
// revision (#475). The embedded payload is the same editable set a correction
// carries and still refuses every protected value; the target pair and the
// reason are the repair's own inputs, and decisions/acknowledgements repeat
// the #471 difference review against the new source.
type PurchaseOrderSourceRepairRequest struct {
	PurchaseOrderUpdateRequest
	Decisions            []PurchaseOrderDecisionRequest `json:"decisions"`
	AcknowledgedMissing  []string                       `json:"acknowledgedMissing"`
	AcknowledgedExtra    []int                          `json:"acknowledgedExtra"`
	TargetQuotationID    int64                          `json:"targetQuotationId"`
	TargetRevisionNumber *int16                         `json:"targetRevisionNumber"`
	Reason               string                         `json:"reason"`
}

// PurchaseOrderSourceRepairDecisionRow is one difference choice a repair
// recorded. The key is the stable review key ("line:2:unitPrice").
type PurchaseOrderSourceRepairDecisionRow struct {
	Key    string `json:"key"`
	Choice string `json:"choice"`
}

// PurchaseOrderSourceRepairRow is the old and new link one repair recorded,
// plus the choices made against the new revision. It hangs off the history
// snapshot, which keeps the values the record carried before the repair.
type PurchaseOrderSourceRepairRow struct {
	PreviousQuoteNumber    string                                 `json:"previousQuoteNumber"`
	PreviousCustomerName   string                                 `json:"previousCustomerName"`
	NewQuoteNumber         string                                 `json:"newQuoteNumber"`
	NewCustomerName        string                                 `json:"newCustomerName"`
	Decisions              []PurchaseOrderSourceRepairDecisionRow `json:"decisions"`
	PreviousQuotationID    int64                                  `json:"previousQuotationId"`
	PreviousSourceRevision int16                                  `json:"previousSourceRevision"`
	PreviousCustomerID     int64                                  `json:"previousCustomerId"`
	NewQuotationID         int64                                  `json:"newQuotationId"`
	NewSourceRevision      int16                                  `json:"newSourceRevision"`
	NewCustomerID          int64                                  `json:"newCustomerId"`
}

// PurchaseOrderConflictResponse is the 409 body a stale save receives (#473).
// It carries the current version and the last writer so the desktop can warn
// with real values instead of asking the user to guess or reload blindly.
type PurchaseOrderConflictResponse struct {
	Error          string `json:"error"`
	Code           string `json:"code"`
	CurrentState   string `json:"currentState"`
	UpdatedByName  string `json:"updatedByName"`
	UpdatedAt      string `json:"updatedAt"`
	CurrentVersion int64  `json:"currentVersion"`
	UpdatedBy      int64  `json:"updatedBy"`
}

// PurchaseOrderCorrectionAttachmentRow is the customer PO PDF version that was
// active when a snapshot was taken. Version and SHA identify the retained
// document row exactly.
type PurchaseOrderCorrectionAttachmentRow struct {
	SHA256   string `json:"sha256"`
	Filename string `json:"filename"`
	Version  int32  `json:"version"`
}

// PurchaseOrderCorrectionItemRow is one line of a retained snapshot.
type PurchaseOrderCorrectionItemRow struct {
	QuotationItemNo *int `json:"quotationItemNo"`
	QuotationSubNo  *int `json:"quotationSubNo"`
	// LineKind is the kind the snapshot line carried (#655). A snapshot taken
	// before the column existed reads quotation, so history can always tell an
	// intentional extra from a legacy free line.
	LineKind          string   `json:"lineKind"`
	LineNumber        int      `json:"lineNumber"`
	Description       string   `json:"description"`
	QuantityScaled    int64    `json:"quantityScaled"`
	UoM               string   `json:"uom"`
	UnitPriceMinor    int64    `json:"unitPriceMinor"`
	DiscountEnabled   bool     `json:"discountEnabled"`
	DiscountPercent   int16    `json:"discountPercent"`
	GrossMinor        int64    `json:"grossMinor"`
	DiscountMinor     int64    `json:"discountMinor"`
	NetMinor          int64    `json:"netMinor"`
	GroupKey          string   `json:"groupKey"`
	ParentDescription string   `json:"parentDescription"`
	DescriptionLines  []string `json:"descriptionLines"`
	// AggregateGroupOrder is the snapshot group's display order this line
	// belonged to; 0 means ungrouped. Group ids churn across saves, so the
	// snapshot keys membership by the derived order instead.
	AggregateGroupOrder int `json:"aggregateGroupOrder,omitempty"`
}

// PurchaseOrderCorrectionGroupRow is one aggregate header frozen into a
// retained snapshot. The snapshot total is derived from the snapshot lines with
// the same display order and is never stored.
type PurchaseOrderCorrectionGroupRow struct {
	DisplayOrder           int    `json:"displayOrder"`
	Title                  string `json:"title"`
	OrderedQuantityScaled  int64  `json:"orderedQuantityScaled"`
	OrderedUoM             string `json:"orderedUom"`
	ShowChildrenToCustomer bool   `json:"showChildrenToCustomer"`
}

// PurchaseOrderCorrectionRow is one immutable snapshot in a record's history:
// previous values, reason, actor and time, and the attachment reference. It is
// what the history panel reads; no line or field is derived from the live
// record, so a later repair cannot rewrite what the history says.
type PurchaseOrderCorrectionRow struct {
	Attachment *PurchaseOrderCorrectionAttachmentRow `json:"attachment"`
	// Repair is present only for kind "source_repair" (#475): the old and new
	// link and the choices the admin made. Every other kind leaves it nil, so
	// an older reader sees exactly the snapshot it always did.
	Repair                *PurchaseOrderSourceRepairRow `json:"repair,omitempty"`
	PODate                *string                       `json:"poDate"`
	RequestedDeliveryDate *string                       `json:"requestedDeliveryDate"`
	UUID                  string                        `json:"uuid"`
	Kind                  string                        `json:"kind"`
	State                 string                        `json:"state"`
	Reason                string                        `json:"reason"`
	// SourceMissing is true when the snapshot had no live source: a
	// never-linked draft overwrite or a formerly-linked record. The numeric
	// QuotationID/SourceRevision stay 0 for older readers, so the history
	// panel must branch on this rather than print a fabricated revision 0.
	SourceMissing   bool   `json:"sourceMissing"`
	QuoteNumber     string `json:"quoteNumber"`
	CustomerName    string `json:"customerName"`
	Currency        string `json:"currency"`
	BillingAddress  string `json:"billingAddress"`
	DeliveryAddress string `json:"deliveryAddress"`
	ShipToCode      string `json:"shipToCode"`
	ContactEmail    string `json:"contactEmail"`
	ContactPhone    string `json:"contactPhone"`
	PONumber        string `json:"poNumber"`
	PaymentTerms    string `json:"paymentTerms"`
	Incoterms       string `json:"incoterms"`
	TaxZone         string `json:"taxZone"`
	TaxCategory     string `json:"taxCategory"`
	// BusinessCategory is the category the snapshot carried. Empty means
	// Unassigned: a record corrected before the category existed.
	BusinessCategory string                           `json:"businessCategory"`
	AdjustmentReason string                           `json:"adjustmentReason"`
	ActorName        string                           `json:"actorName"`
	ReceivedDate     string                           `json:"receivedDate"`
	CreatedAt        string                           `json:"createdAt"`
	Items            []PurchaseOrderCorrectionItemRow `json:"items"`
	// Groups are the aggregate headers the snapshot record carried, ordered by
	// display order. Absent on a snapshot taken before #654.
	Groups           []PurchaseOrderCorrectionGroupRow `json:"groups,omitempty"`
	ID               int64                             `json:"id"`
	QuotationID      int64                             `json:"quotationId"`
	SourceRevision   int16                             `json:"sourceRevision"`
	CustomerID       int64                             `json:"customerId"`
	PreviousVersion  int64                             `json:"previousVersion"`
	ResultingVersion int64                             `json:"resultingVersion"`
	ActorID          int64                             `json:"actorId"`
	DiscountPercent  int16                             `json:"discountPercent"`
	SubtotalMinor    int64                             `json:"subtotalMinor"`
	DiscountMinor    int64                             `json:"discountMinor"`
	PpnBps           int16                             `json:"ppnBps"`
	PpnMinor         int64                             `json:"ppnMinor"`
	AdjustmentMinor  int64                             `json:"adjustmentMinor"`
	GrandTotalMinor  int64                             `json:"grandTotalMinor"`
}

// QuotationItemRow mirrors one internal line. PDF bytes are never included.
type QuotationItemRow struct {
	DisplayOriginalUnit      string                `json:"displayOriginalUnit"`
	Description              string                `json:"description"`
	DisplayMarginAmount      string                `json:"displayMarginAmount"`
	UoM                      string                `json:"uom"`
	DisplayMarginalizedTotal string                `json:"displayMarginalizedTotal"`
	DisplayMarginalizedUnit  string                `json:"displayMarginalizedUnit"`
	DisplayOriginalTotal     string                `json:"displayOriginalTotal"`
	Subs                     []QuotationSubItemRow `json:"subs"`
	OriginalUnitPrice        int64                 `json:"originalUnitPrice"`
	OriginalTotal            int64                 `json:"originalTotal"`
	MarginalizedUnitPrice    int64                 `json:"marginalizedUnitPrice"`
	MarginalizedTotal        int64                 `json:"marginalizedTotal"`
	MarginAmount             int64                 `json:"marginAmount"`
	ListUnitPrice            int64                 `json:"listUnitPrice"`
	DiscountAmount           int64                 `json:"discountAmount"`
	ItemNo                   int                   `json:"itemNo"`
	Qty                      int64                 `json:"qty"`
	MarginPercent            int16                 `json:"marginPercent"`
	DiscountPercent          int16                 `json:"discountPercent"`
	MarginEnabled            bool                  `json:"marginEnabled"`
	DiscountEnabled          bool                  `json:"discountEnabled"`
	ShowSubPrices            bool                  `json:"showSubPrices"`
}

// QuotationSubItemRow is one sub-item of an itemized line: both sides, so the
// editor and the archive detail can show what the document chose to print and
// what it earned.
type QuotationSubItemRow struct {
	Description              string   `json:"description"`
	Details                  []string `json:"details"`
	UoM                      string   `json:"uom"`
	DisplayOriginalUnit      string   `json:"displayOriginalUnit"`
	DisplayOriginalTotal     string   `json:"displayOriginalTotal"`
	DisplayMarginalizedUnit  string   `json:"displayMarginalizedUnit"`
	DisplayMarginalizedTotal string   `json:"displayMarginalizedTotal"`
	DisplayMarginAmount      string   `json:"displayMarginAmount"`
	OriginalUnitPrice        int64    `json:"originalUnitPrice"`
	OriginalTotal            int64    `json:"originalTotal"`
	MarginalizedUnitPrice    int64    `json:"marginalizedUnitPrice"`
	MarginalizedTotal        int64    `json:"marginalizedTotal"`
	MarginAmount             int64    `json:"marginAmount"`
	ListUnitPrice            int64    `json:"listUnitPrice"`
	DiscountAmount           int64    `json:"discountAmount"`
	SubNo                    int      `json:"subNo"`
	Qty                      int64    `json:"qty"`
	MarginPercent            int16    `json:"marginPercent"`
	DiscountPercent          int16    `json:"discountPercent"`
	MarginEnabled            bool     `json:"marginEnabled"`
	DiscountEnabled          bool     `json:"discountEnabled"`
}

// QuotationDetail is one revision: internal economics plus the customer HTML
// snapshot. Raw PDF bytes are intentionally absent; HasPDF reports attachment.
type QuotationDetail struct {
	DisplayMarginalizedSubtotal string `json:"displayMarginalizedSubtotal"`
	DisplayMarginalizedDiscount string `json:"displayMarginalizedDiscount"`
	UUID                        string `json:"uuid"`
	HTMLSnapshot                string `json:"htmlSnapshot"`
	DisplayNett                 string `json:"displayNett"`
	CustomerName                string `json:"customerName"`
	Subject                     string `json:"subject"`
	// Note is the quotation-level note this revision printed and signed. Its
	// source is the restricted Markdown subset (**bold**, *italic*, "- "
	// bullets, "1. " numbered items, paragraphs and line breaks); other markup
	// prints literally. It is empty for revisions issued before the note
	// existed.
	Note                        string             `json:"note"`
	Currency                    string             `json:"currency"`
	RecipientName               string             `json:"recipientName"`
	Incoterm                    string             `json:"incoterm"`
	DisplayProfit               string             `json:"displayProfit"`
	TaxZone                     string             `json:"taxZone"`
	TaxCategory                 string             `json:"taxCategory"`
	TaxType                     string             `json:"taxType"`
	ServiceCategory             string             `json:"serviceCategory"`
	TaxLabel                    string             `json:"taxLabel"`
	DisplayMarginalizedAfterTax string             `json:"displayMarginalizedAfterTax"`
	DisplayTax                  string             `json:"displayTax"`
	DisplayPpn                  string             `json:"displayPpn"`
	DisplayMarginalizedGrand    string             `json:"displayMarginalizedGrand"`
	DisplayOriginalGrand        string             `json:"displayOriginalGrand"`
	QuoteNumber                 string             `json:"quoteNumber"`
	DisplayOriginalDiscount     string             `json:"displayOriginalDiscount"`
	DisplayOriginalSubtotal     string             `json:"displayOriginalSubtotal"`
	AvailableRevisions          []int16            `json:"availableRevisions"`
	Items                       []QuotationItemRow `json:"items"`
	MarginalizedSubtotal        int64              `json:"marginalizedSubtotal"`
	ProfitAmount                int64              `json:"profitAmount"`
	MarginalizedGrand           int64              `json:"marginalizedGrand"`
	GrandTotal                  int64              `json:"grandTotal"`
	MarginalizedDiscount        int64              `json:"marginalizedDiscount"`
	TaxAmount                   int64              `json:"taxAmount"`
	ID                          int64              `json:"id"`
	NettProfit                  int64              `json:"nettProfit"`
	OriginalGrand               int64              `json:"originalGrand"`
	OriginalDiscount            int64              `json:"originalDiscount"`
	OriginalSubtotal            int64              `json:"originalSubtotal"`
	MarginalizedGrandAfterTax   int64              `json:"marginalizedGrandAfterTax"`
	PpnAmount                   int64              `json:"ppnAmount"`
	DiscountPercent             int16              `json:"discountPercent"`
	// CustomerCode is the client-directory code the editor resolves: the copy
	// path pastes by code and refuses when the code is not in the served
	// list, so a revision must carry the code it was issued under.
	CustomerCode int16 `json:"customerCode"`
	// CustomerAddress and RecipientAddress are the client-directory snapshot
	// the revision stored. The editor has no address inputs -- Prepare
	// resolves both from the client on the next export -- so the copy payload
	// carries them as information, never as applied values.
	CustomerAddress  string `json:"customerAddress"`
	RecipientAddress string `json:"recipientAddress"`
	// ValidityDays and PaymentTermDays are the revision's own terms:
	// history, not the live profile's defaults.
	ValidityDays    int16 `json:"validityDays"`
	PaymentTermDays int16 `json:"paymentTermDays"`
	// LeadTimeYears/Months/Weeks/Days are the lead time as the revision stated
	// it, each 0..255, all zero meaning none was stated. They are the stored
	// figures rather than a display so the editor restores the units.
	LeadTimeYears  uint8 `json:"leadTimeYears"`
	LeadTimeMonths uint8 `json:"leadTimeMonths"`
	LeadTimeWeeks  uint8 `json:"leadTimeWeeks"`
	LeadTimeDays   uint8 `json:"leadTimeDays"`
	// DisplayLeadTime is the composed sentence the document printed
	// ("2 Years, 10 Months, 4 Weeks, and 5 Days", "-" when none was stated).
	DisplayLeadTime string `json:"displayLeadTime"`
	Revision        int16  `json:"revision"`
	HasPDF          bool   `json:"hasPdf"`
	// Closure and expiry (#656). Closure is quotation-level, so every revision
	// of a closed quotation reads closed. ValidUntil follows the latest issued
	// revision even when an older revision is requested; ExpiredUnclosed is
	// false while closed.
	Closed          bool    `json:"closed"`
	ClosedAt        *string `json:"closedAt"`
	ClosedReason    string  `json:"closedReason"`
	ClosedByName    string  `json:"closedByName"`
	ValidUntil      *string `json:"validUntil"`
	ExpiredUnclosed bool    `json:"expiredUnclosed"`
}

// QuotationCloseRequest carries the mandatory reason for closing a quotation
// (#656). The bound is 500 characters; the store trims and validates it again.
type QuotationCloseRequest struct {
	Reason string `json:"reason"`
}

// AttachDocumentRequest uploads locally rendered PDF bytes for a revision.
type AttachDocumentRequest struct {
	PDFBase64 string `json:"pdfBase64"`
	PDFPath   string `json:"pdfPath"`
}
