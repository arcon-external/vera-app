package backend

import (
	"encoding/base64"
	"fmt"
	"mime"
	"path"
	"strconv"
	"strings"

	"vera-app/backend/apiclient"
	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #468: desktop bindings for a manual Purchase Order draft. Both PO
// entry paths reach the same authenticated API; these wrappers keep the
// desktop free of HTTP shape knowledge and give Wails one typed surface.

// PurchaseOrderItemDTO is one entered line. QuantityScaled is an exact count
// of 1/10000 units; unit prices are minor units of the PO currency. The three
// money fields are the row's stored calculation, returned rather than
// recomputed.
type PurchaseOrderItemDTO struct {
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

// PurchaseOrderDTO is a draft or saved PO with its editable header, its
// explained calculation, and the source document's printed figures. The two
// sets of totals are deliberately separate: the calculated ones are Vera's
// arithmetic, the printed ones are evidence.
type PurchaseOrderDTO struct {
	PODate                 *string
	RequestedDeliveryDate  *string
	UUID                   string
	State                  string
	QuoteNumber            string
	CustomerName           string
	BillingAddress         string
	DeliveryAddress        string
	ShipToCode             string
	ContactEmail           string
	ContactPhone           string
	PONumber               string
	ReceivedDate           string
	Incoterms              string
	Currency               string
	TaxZone                string
	TaxType                string
	ServiceCategory        string
	TaxCategory            string
	AdjustmentReason       string
	CreatedAt              string
	UpdatedAt              string
	Items                  []PurchaseOrderItemDTO
	ID                     int64
	QuotationID            int64
	SourceRevision         int16
	CustomerID             int64
	PaymentTerms           string
	DiscountPercent        int16
	PpnBps                 int16
	SubtotalMinor          int64
	DiscountMinor          int64
	PpnMinor               int64
	AdjustmentMinor        int64
	GrandTotalMinor        int64
	PrintedSubtotalMinor   int64
	PrintedDiscountMinor   int64
	PrintedPpnMinor        int64
	PrintedGrandTotalMinor int64
	SavedAt                *string
	SavedBy                *int64
	PdfAbsentConfirmed     bool
	// SourceMissing is true for a never-linked draft and for a record whose
	// Quotation was deleted. SourceState names which one it is (#496).
	SourceMissing bool
	// SourceState is "live", "never" or "former". It is the only presence
	// signal: the numeric ids stay numeric for old readers and revision 00 is
	// a real link.
	SourceState string
	// PrintedTotalsKnown is false exactly for a never-linked draft, whose
	// printed source figures do not exist yet.
	PrintedTotalsKnown bool
	// The optional customer-system confirmation reference (#474). Null until
	// assigned, then immutable; ConfirmationWarn follows the stored sequence.
	ConfirmationNumber     *string
	ConfirmationAssignedAt *string
	ConfirmationWarn       bool
	// Version is the optimistic-concurrency revision (#473): the editor sends
	// the version it loaded, and every write moves it forward.
	Version int64
}

// PurchaseOrderSourceLineDTO is one customer-facing line of the exact source
// revision. LineLabel is the customer document's number: "3" or "3.1".
type PurchaseOrderSourceLineDTO struct {
	QuotationItemNo int
	QuotationSubNo  *int
	LineLabel       string
	Description     string
	UoM             string
	QuantityScaled  int64
	UnitPriceMinor  int64
	DiscountEnabled bool
	DiscountPercent int16
}

// PurchaseOrderSourceDTO is the exact revision the final-save review compares
// against, read fresh on every review.
type PurchaseOrderSourceDTO struct {
	QuotationID            int64
	RevisionNumber         int16
	QuoteNumber            string
	CustomerID             int64
	CustomerName           string
	Currency               string
	PaymentTerms           string
	Incoterms              string
	DiscountPercent        int16
	Lines                  []PurchaseOrderSourceLineDTO
	PrintedSubtotalMinor   int64
	PrintedDiscountMinor   int64
	PrintedPpnMinor        int64
	PrintedGrandTotalMinor int64
}

// PurchaseOrderReviewDTO is one record beside its source plus the document
// branch. DocumentState is "none" until #472 can attach a PDF.
type PurchaseOrderReviewDTO struct {
	Order         PurchaseOrderDTO
	Source        PurchaseOrderSourceDTO
	DocumentState string
}

// PurchaseOrderDocumentDTO is one stored version of the optional customer PO
// PDF. It is metadata only; the bytes come from a content call.
type PurchaseOrderDocumentDTO struct {
	UUID             string
	SHA256           string
	ContentType      string
	OriginalFilename string
	UploadedAt       string
	ReplacedAt       *string
	RemovedAt        *string
	RemovedBy        *int64
	ID               int64
	PurchaseOrderID  int64
	Version          int32
	SizeBytes        int64
	UploadedBy       int64
	Active           bool
}

// PurchaseOrderDocumentStateDTO is the active PDF plus retained history,
// newest first. Document is nil when no PDF is attached.
type PurchaseOrderDocumentStateDTO struct {
	Document *PurchaseOrderDocumentDTO
	Versions []PurchaseOrderDocumentDTO
}

// PurchaseOrderDocumentContentDTO is the PDF content, base64-encoded, and the
// headers that identify which version was served.
//
// Base64 is explicit rather than an implicit []byte because the two sides
// disagreed once already: Wails serializes []byte as base64 text, while the
// generated TypeScript called it number[] and the panel built a zero-length
// array. A string field makes the representation part of the contract, and
// frontend/src/purchaseOrderDocument.ts is the only decoder.
type PurchaseOrderDocumentContentDTO struct {
	DataBase64  string
	ContentType string
	Filename    string
	SHA256      string
	Version     int32
}

// PurchaseOrderDocumentUploadInput attaches or replaces the one customer PO
// PDF. The editor uses its file picker, reads the file, and sends the bytes
// base64-encoded; there is no path variant, because the desktop picker never
// supplied one and a path the panel does not use is a second untested input.
type PurchaseOrderDocumentUploadInput struct {
	PurchaseOrderID int64
	DataBase64      string
	Filename        string
}

// PurchaseOrderDecisionInput is one explicit difference choice ("po" or
// "quotation"). Neither is preselected.
type PurchaseOrderDecisionInput struct {
	Key    string
	Choice string
}

// PurchaseOrderFinalizeInput carries the same values a draft save replaces plus
// the review decisions, the explicit line acknowledgements and the document
// confirmation.
type PurchaseOrderFinalizeInput struct {
	ID                    int64
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	PODate                string
	ReceivedDate          string
	RequestedDeliveryDate string
	Incoterms             string
	PaymentTerms          string
	AdjustmentReason      string
	Items                 []PurchaseOrderItemInput
	Decisions             []PurchaseOrderDecisionInput
	AcknowledgedMissing   []string
	AcknowledgedExtra     []int
	DiscountPercent       int16
	AdjustmentMinor       int64
	ExpectedVersion       int64
	Overwrite             bool
	DocumentConfirmed     bool
	// AssignConfirmationNumber is the final-save checkbox (#474). False leaves
	// the record unassigned and consumes no sequence.
	AssignConfirmationNumber bool
}

// PurchaseOrderCreateInput is the selector's confirmed pair plus an optional
// received-date override. An empty date asks the server for today.
type PurchaseOrderCreateInput struct {
	ReceivedDate   string
	QuotationID    int64
	RevisionNumber int16
}

// PurchaseOrderBlankCreateInput starts a source-less draft (#496). The server
// seeds the company currency; the tax triple is the user's Quotation default
// and is editable again on the draft until a quotation is linked.
type PurchaseOrderBlankCreateInput struct {
	ReceivedDate    string
	TaxZone         string
	TaxType         string
	ServiceCategory string
}

// PurchaseOrderItemInput is one replace-lines entry. Line numbers come from
// array order; a row discount is a flag plus a 0-100 percentage, mirroring the
// quotation.
type PurchaseOrderItemInput struct {
	QuotationItemNo *int
	QuotationSubNo  *int
	Description     string
	QuantityScaled  int64
	UoM             string
	UnitPriceMinor  int64
	DiscountEnabled bool
	DiscountPercent int16
}

// PurchaseOrderUpdateInput replaces the editable draft header, the
// whole-order discount and adjustment, and every line.
//
// ExpectedVersion is mandatory (#473): a save without the version it loaded is
// refused, never treated as "no check". Overwrite records that the caller chose
// Overwrite after a version warning, which keeps the replaced draft.
type PurchaseOrderUpdateInput struct {
	ID                    int64
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	PODate                string
	ReceivedDate          string
	RequestedDeliveryDate string
	Incoterms             string
	PaymentTerms          string
	AdjustmentReason      string
	Items                 []PurchaseOrderItemInput
	DiscountPercent       int16
	AdjustmentMinor       int64
	// Currency and the tax triple are editable only while a draft has never
	// been linked (#496). A linked row keeps its frozen basis.
	Currency        string
	TaxZone         string
	TaxType         string
	ServiceCategory string
	ExpectedVersion int64
	Overwrite       bool
}

// PurchaseOrderCorrectionInput replaces a saved record. It carries the same
// editable values as a draft save plus the reason staff typed, which the server
// stores in the immutable snapshot. The source link, customer, currency, tax
// basis, identity and confirmation number have no field here at all.
type PurchaseOrderCorrectionInput struct {
	ID                    int64
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	PODate                string
	ReceivedDate          string
	RequestedDeliveryDate string
	Incoterms             string
	PaymentTerms          string
	AdjustmentReason      string
	Reason                string
	Items                 []PurchaseOrderItemInput
	DiscountPercent       int16
	AdjustmentMinor       int64
	ExpectedVersion       int64
	Overwrite             bool
}

// PurchaseOrderSourceReviewInput asks for a saved record's difference review
// against a candidate revision (#475). It writes nothing: the admin sees the
// values that would need a choice before the repair is saved.
type PurchaseOrderSourceReviewInput struct {
	ID             int64
	QuotationID    int64
	RevisionNumber int16
}

// PurchaseOrderSourceRepairInput moves a saved record to another issued
// revision (#475). It carries the same editable values as a correction plus
// the target pair, the required reason and the #471 difference review. The
// protected fields still have no entry here at all.
type PurchaseOrderSourceRepairInput struct {
	ID                    int64
	TargetQuotationID     int64
	TargetRevisionNumber  int16
	BillingAddress        string
	DeliveryAddress       string
	ShipToCode            string
	ContactEmail          string
	ContactPhone          string
	PONumber              string
	PODate                string
	ReceivedDate          string
	RequestedDeliveryDate string
	Incoterms             string
	PaymentTerms          string
	AdjustmentReason      string
	Reason                string
	Items                 []PurchaseOrderItemInput
	Decisions             []PurchaseOrderDecisionInput
	AcknowledgedMissing   []string
	AcknowledgedExtra     []int
	DiscountPercent       int16
	AdjustmentMinor       int64
	ExpectedVersion       int64
}

// PurchaseOrderSourceRepairDecisionDTO is one recorded difference choice.
type PurchaseOrderSourceRepairDecisionDTO struct {
	Key    string
	Choice string
}

// PurchaseOrderSourceRepairDTO is the old and new link a repair kept in the
// history, plus the choices made against the new revision.
type PurchaseOrderSourceRepairDTO struct {
	PreviousQuoteNumber    string
	PreviousCustomerName   string
	NewQuoteNumber         string
	NewCustomerName        string
	Decisions              []PurchaseOrderSourceRepairDecisionDTO
	PreviousQuotationID    int64
	PreviousSourceRevision int16
	PreviousCustomerID     int64
	NewQuotationID         int64
	NewSourceRevision      int16
	NewCustomerID          int64
}

// PurchaseOrderConflictDTO is the current state a stale save was refused
// against, passed to the editor as typed data rather than error prose.
type PurchaseOrderConflictDTO struct {
	CurrentState   string
	UpdatedByName  string
	UpdatedAt      string
	CurrentVersion int64
	UpdatedBy      int64
}

// PurchaseOrderWriteResult is the answer to a versioned write. Exactly one side
// is set: Order when the write succeeded, Conflict when the server refused it
// because the record moved on, or SavedRecord when the record is already saved
// or was finalized by the intervening save. SavedRecord is not an error: the
// caller keeps its working copy and continues through the correction flow,
// which requires a reason, instead of promising a draft overwrite that the
// server would refuse. A real failure is still the second return value, so the
// editor never has to guess from an error string.
type PurchaseOrderWriteResult struct {
	Order       *PurchaseOrderDTO
	Conflict    *PurchaseOrderConflictDTO
	SavedRecord bool
}

// PurchaseOrderCorrectionAttachmentDTO is the customer PO PDF version that was
// active when a snapshot was taken.
type PurchaseOrderCorrectionAttachmentDTO struct {
	SHA256   string
	Filename string
	Version  int32
}

// PurchaseOrderCorrectionItemDTO is one line of a retained snapshot.
type PurchaseOrderCorrectionItemDTO struct {
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

// PurchaseOrderCorrectionDTO is one immutable snapshot: the values a
// correction replaced, its reason, actor and time, and the attachment version
// the record referenced at that moment.
type PurchaseOrderCorrectionDTO struct {
	Attachment *PurchaseOrderCorrectionAttachmentDTO
	// Repair is present only for a source repair (#475): the old and new link
	// and the admin's choices. Nil for every other history kind.
	Repair                *PurchaseOrderSourceRepairDTO
	PODate                *string
	RequestedDeliveryDate *string
	UUID                  string
	Kind                  string
	State                 string
	Reason                string
	SourceMissing         bool
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
	ReceivedDate          string
	CreatedAt             string
	Items                 []PurchaseOrderCorrectionItemDTO
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
}

// CreatePurchaseOrderDraft creates the one active PO for a Quotation from the
// confirmed issued revision.
func (q *Quotation) CreatePurchaseOrderDraft(input PurchaseOrderCreateInput) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
	}
	revision := input.RevisionNumber
	var response apicontract.PurchaseOrderRow
	if err := client.Post("/v1/purchase-orders", apicontract.PurchaseOrderCreateRequest{
		QuotationID:    input.QuotationID,
		RevisionNumber: &revision,
		ReceivedDate:   input.ReceivedDate,
	}, &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

// CreatePurchaseOrderBlankDraft starts a draft with no source and no Customer
// (#496). The server supplies the company currency, validates the user's tax
// defaults, and returns the stored draft. Opening the editor does not call
// this; the first Save draft, PDF upload or extraction does.
func (q *Quotation) CreatePurchaseOrderBlankDraft(input PurchaseOrderBlankCreateInput) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Post("/v1/purchase-orders", apicontract.PurchaseOrderCreateRequest{
		ReceivedDate:    input.ReceivedDate,
		TaxZone:         input.TaxZone,
		TaxType:         input.TaxType,
		ServiceCategory: input.ServiceCategory,
	}, &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

// GetPurchaseOrder reopens one PO by id.
func (q *Quotation) GetPurchaseOrder(id int64) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Get("/v1/purchase-orders/"+strconv.FormatInt(id, 10), &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

// UpdatePurchaseOrderDraft replaces the editable draft values. The result is a
// typed outcome: a stale version comes back as Conflict with no error, so the
// editor can offer Cancel/Overwrite without parsing a message.
func (q *Quotation) UpdatePurchaseOrderDraft(input PurchaseOrderUpdateInput) (PurchaseOrderWriteResult, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	items := make([]apicontract.PurchaseOrderItemInput, 0, len(input.Items))
	for _, item := range input.Items {
		items = append(items, apicontract.PurchaseOrderItemInput{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
		})
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Put("/v1/purchase-orders/"+strconv.FormatInt(input.ID, 10),
		apicontract.PurchaseOrderUpdateRequest{
			PurchaseOrderProtectedFieldsRequest: apicontract.PurchaseOrderProtectedFieldsRequest{
				Currency:        stringPointer(input.Currency),
				TaxZone:         stringPointer(input.TaxZone),
				TaxType:         stringPointer(input.TaxType),
				ServiceCategory: stringPointer(input.ServiceCategory),
			},
			BillingAddress:        input.BillingAddress,
			DeliveryAddress:       input.DeliveryAddress,
			ShipToCode:            input.ShipToCode,
			ContactEmail:          input.ContactEmail,
			ContactPhone:          input.ContactPhone,
			PONumber:              input.PONumber,
			PODate:                input.PODate,
			ReceivedDate:          input.ReceivedDate,
			RequestedDeliveryDate: input.RequestedDeliveryDate,
			PaymentTerms:          input.PaymentTerms,
			Incoterms:             input.Incoterms,
			Items:                 items,
			DiscountPercent:       input.DiscountPercent,
			AdjustmentMinor:       input.AdjustmentMinor,
			AdjustmentReason:      input.AdjustmentReason,
			ExpectedVersion:       input.ExpectedVersion,
			Overwrite:             input.Overwrite,
		}, &response); err != nil {
		return purchaseOrderWriteFailure(err)
	}
	return purchaseOrderSaved(response), nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// CorrectPurchaseOrder replaces a saved record. Like a draft save, the version
// is mandatory and a stale one returns a typed Conflict; unlike it, the reason
// is required and the server keeps the replaced values.
func (q *Quotation) CorrectPurchaseOrder(input PurchaseOrderCorrectionInput) (PurchaseOrderWriteResult, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	items := make([]apicontract.PurchaseOrderItemInput, 0, len(input.Items))
	for _, item := range input.Items {
		items = append(items, apicontract.PurchaseOrderItemInput{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
		})
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Post("/v1/purchase-orders/"+strconv.FormatInt(input.ID, 10)+"/correction",
		apicontract.PurchaseOrderCorrectionRequest{
			PurchaseOrderUpdateRequest: apicontract.PurchaseOrderUpdateRequest{
				BillingAddress:        input.BillingAddress,
				DeliveryAddress:       input.DeliveryAddress,
				ShipToCode:            input.ShipToCode,
				ContactEmail:          input.ContactEmail,
				ContactPhone:          input.ContactPhone,
				PONumber:              input.PONumber,
				PODate:                input.PODate,
				ReceivedDate:          input.ReceivedDate,
				RequestedDeliveryDate: input.RequestedDeliveryDate,
				PaymentTerms:          input.PaymentTerms,
				Incoterms:             input.Incoterms,
				Items:                 items,
				DiscountPercent:       input.DiscountPercent,
				AdjustmentMinor:       input.AdjustmentMinor,
				AdjustmentReason:      input.AdjustmentReason,
				ExpectedVersion:       input.ExpectedVersion,
				Overwrite:             input.Overwrite,
			},
			Reason: input.Reason,
		}, &response); err != nil {
		return purchaseOrderWriteFailure(err)
	}
	return purchaseOrderSaved(response), nil
}

// ReviewPurchaseOrderSource compares a saved record against a candidate
// revision without writing anything (#475). The response is the same review
// shape the final save reads, with the candidate as the source.
func (q *Quotation) ReviewPurchaseOrderSource(input PurchaseOrderSourceReviewInput) (PurchaseOrderReviewDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderReviewDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderReviewDTO{}, err
	}
	revision := input.RevisionNumber
	var response apicontract.PurchaseOrderReviewRow
	if err := client.Post("/v1/purchase-orders/"+strconv.FormatInt(input.ID, 10)+"/source-repair/review",
		apicontract.PurchaseOrderSourceReviewRequest{
			TargetQuotationID:    input.QuotationID,
			TargetRevisionNumber: &revision,
		}, &response); err != nil {
		return PurchaseOrderReviewDTO{}, err
	}
	return mapPurchaseOrderReviewDTO(response), nil
}

// RepairPurchaseOrderSource moves a saved record to another issued revision
// (#475). A stale version is a typed Conflict, exactly as a correction's is;
// every other refusal stays an error so the panel names the server's answer.
func (q *Quotation) RepairPurchaseOrderSource(input PurchaseOrderSourceRepairInput) (PurchaseOrderWriteResult, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	items := make([]apicontract.PurchaseOrderItemInput, 0, len(input.Items))
	for _, item := range input.Items {
		items = append(items, apicontract.PurchaseOrderItemInput{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
		})
	}
	decisions := make([]apicontract.PurchaseOrderDecisionRequest, 0, len(input.Decisions))
	for _, decision := range input.Decisions {
		decisions = append(decisions, apicontract.PurchaseOrderDecisionRequest{
			Key:    decision.Key,
			Choice: decision.Choice,
		})
	}
	revision := input.TargetRevisionNumber
	var response apicontract.PurchaseOrderRow
	if err := client.Post("/v1/purchase-orders/"+strconv.FormatInt(input.ID, 10)+"/source-repair",
		apicontract.PurchaseOrderSourceRepairRequest{
			PurchaseOrderUpdateRequest: apicontract.PurchaseOrderUpdateRequest{
				BillingAddress:        input.BillingAddress,
				DeliveryAddress:       input.DeliveryAddress,
				ShipToCode:            input.ShipToCode,
				ContactEmail:          input.ContactEmail,
				ContactPhone:          input.ContactPhone,
				PONumber:              input.PONumber,
				PODate:                input.PODate,
				ReceivedDate:          input.ReceivedDate,
				RequestedDeliveryDate: input.RequestedDeliveryDate,
				PaymentTerms:          input.PaymentTerms,
				Incoterms:             input.Incoterms,
				Items:                 items,
				DiscountPercent:       input.DiscountPercent,
				AdjustmentMinor:       input.AdjustmentMinor,
				AdjustmentReason:      input.AdjustmentReason,
				ExpectedVersion:       input.ExpectedVersion,
			},
			TargetQuotationID:    input.TargetQuotationID,
			TargetRevisionNumber: &revision,
			Reason:               input.Reason,
			Decisions:            decisions,
			AcknowledgedMissing:  input.AcknowledgedMissing,
			AcknowledgedExtra:    input.AcknowledgedExtra,
		}, &response); err != nil {
		return purchaseOrderWriteFailure(err)
	}
	return purchaseOrderSaved(response), nil
}

// purchaseOrderSaved wraps one successful write.
func purchaseOrderSaved(row apicontract.PurchaseOrderRow) PurchaseOrderWriteResult {
	order := mapPurchaseOrderDTO(row)
	return PurchaseOrderWriteResult{Order: &order}
}

// purchaseOrderWriteFailure turns a named version conflict into a typed result,
// a saved-record refusal into the correction-flow answer, and leaves every
// other failure as an error. The server's message is not parsed: the details
// come from the response body through apiclient.
func purchaseOrderWriteFailure(err error) (PurchaseOrderWriteResult, error) {
	conflict := apiclient.ConflictOf(err)
	if conflict != nil {
		return PurchaseOrderWriteResult{Conflict: &PurchaseOrderConflictDTO{
			CurrentVersion: conflict.CurrentVersion,
			CurrentState:   conflict.CurrentState,
			UpdatedBy:      conflict.UpdatedBy,
			UpdatedByName:  conflict.UpdatedByName,
			UpdatedAt:      conflict.UpdatedAt,
		}}, nil
	}
	// The version check passed and the state rule refused: the record is saved,
	// so the working copy belongs in a correction, not a draft write. It is a
	// typed answer rather than an error so the editor keeps every value and
	// opens the correction flow instead of dead-ending on a message.
	if apiclient.CodeOf(err) == "po_not_draft" {
		return PurchaseOrderWriteResult{SavedRecord: true}, nil
	}
	return PurchaseOrderWriteResult{}, err
}

// GetPurchaseOrderHistory reads the immutable snapshots of one record, newest
// first. It is a read: a missing or deleted record is an error, not an empty
// success, so the panel never claims a history for a record that is gone.
func (q *Quotation) GetPurchaseOrderHistory(id int64) ([]PurchaseOrderCorrectionDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return nil, err
	}
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		History []apicontract.PurchaseOrderCorrectionRow `json:"history"`
	}
	if err := client.Get("/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/history", &response); err != nil {
		return nil, err
	}
	history := make([]PurchaseOrderCorrectionDTO, 0, len(response.History))
	for _, row := range response.History {
		history = append(history, mapPurchaseOrderCorrectionDTO(row))
	}
	return history, nil
}

func mapPurchaseOrderCorrectionDTO(row apicontract.PurchaseOrderCorrectionRow) PurchaseOrderCorrectionDTO {
	entry := PurchaseOrderCorrectionDTO{
		ID:                    row.ID,
		UUID:                  row.UUID,
		Kind:                  row.Kind,
		State:                 row.State,
		Reason:                row.Reason,
		SourceMissing:         row.SourceMissing,
		ActorID:               row.ActorID,
		ActorName:             row.ActorName,
		CreatedAt:             row.CreatedAt,
		PreviousVersion:       row.PreviousVersion,
		ResultingVersion:      row.ResultingVersion,
		QuotationID:           row.QuotationID,
		SourceRevision:        row.SourceRevision,
		QuoteNumber:           row.QuoteNumber,
		CustomerID:            row.CustomerID,
		CustomerName:          row.CustomerName,
		Currency:              row.Currency,
		TaxZone:               row.TaxZone,
		TaxCategory:           row.TaxCategory,
		BillingAddress:        row.BillingAddress,
		DeliveryAddress:       row.DeliveryAddress,
		ShipToCode:            row.ShipToCode,
		ContactEmail:          row.ContactEmail,
		ContactPhone:          row.ContactPhone,
		PONumber:              row.PONumber,
		PODate:                row.PODate,
		ReceivedDate:          row.ReceivedDate,
		RequestedDeliveryDate: row.RequestedDeliveryDate,
		PaymentTerms:          row.PaymentTerms,
		Incoterms:             row.Incoterms,
		DiscountPercent:       row.DiscountPercent,
		SubtotalMinor:         row.SubtotalMinor,
		DiscountMinor:         row.DiscountMinor,
		PpnBps:                row.PpnBps,
		PpnMinor:              row.PpnMinor,
		AdjustmentMinor:       row.AdjustmentMinor,
		AdjustmentReason:      row.AdjustmentReason,
		GrandTotalMinor:       row.GrandTotalMinor,
		Items:                 make([]PurchaseOrderCorrectionItemDTO, 0, len(row.Items)),
	}
	if row.Attachment != nil {
		entry.Attachment = &PurchaseOrderCorrectionAttachmentDTO{
			Version:  row.Attachment.Version,
			SHA256:   row.Attachment.SHA256,
			Filename: row.Attachment.Filename,
		}
	}
	if row.Repair != nil {
		repair := &PurchaseOrderSourceRepairDTO{
			PreviousQuotationID:    row.Repair.PreviousQuotationID,
			PreviousSourceRevision: row.Repair.PreviousSourceRevision,
			PreviousQuoteNumber:    row.Repair.PreviousQuoteNumber,
			PreviousCustomerID:     row.Repair.PreviousCustomerID,
			PreviousCustomerName:   row.Repair.PreviousCustomerName,
			NewQuotationID:         row.Repair.NewQuotationID,
			NewSourceRevision:      row.Repair.NewSourceRevision,
			NewQuoteNumber:         row.Repair.NewQuoteNumber,
			NewCustomerID:          row.Repair.NewCustomerID,
			NewCustomerName:        row.Repair.NewCustomerName,
			Decisions:              make([]PurchaseOrderSourceRepairDecisionDTO, 0, len(row.Repair.Decisions)),
		}
		for _, decision := range row.Repair.Decisions {
			repair.Decisions = append(repair.Decisions, PurchaseOrderSourceRepairDecisionDTO{
				Key:    decision.Key,
				Choice: decision.Choice,
			})
		}
		entry.Repair = repair
	}
	for _, item := range row.Items {
		entry.Items = append(entry.Items, PurchaseOrderCorrectionItemDTO{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			LineNumber:      item.LineNumber,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
			GrossMinor:      item.GrossMinor,
			DiscountMinor:   item.DiscountMinor,
			NetMinor:        item.NetMinor,
		})
	}
	return entry
}

// purchaseOrderSourceStateOf normalizes the wire state. An older server had
// only two shapes: a live link or a deleted (former) one, so an absent enum is
// derived from the fields it did send. A new server always sends the enum.
func purchaseOrderSourceStateOf(row apicontract.PurchaseOrderRow) string {
	switch row.SourceState {
	case "live", "never", "former":
		return row.SourceState
	}
	if row.SourceMissing {
		return "former"
	}
	if row.QuotationID > 0 {
		return "live"
	}
	return "never"
}

func mapPurchaseOrderDTO(row apicontract.PurchaseOrderRow) PurchaseOrderDTO {
	sourceState := purchaseOrderSourceStateOf(row)
	dto := PurchaseOrderDTO{
		ID:                     row.ID,
		UUID:                   row.UUID,
		State:                  row.State,
		QuotationID:            row.QuotationID,
		SourceRevision:         row.SourceRevision,
		QuoteNumber:            row.QuoteNumber,
		CustomerID:             row.CustomerID,
		CustomerName:           row.CustomerName,
		BillingAddress:         row.BillingAddress,
		DeliveryAddress:        row.DeliveryAddress,
		ShipToCode:             row.ShipToCode,
		ContactEmail:           row.ContactEmail,
		ContactPhone:           row.ContactPhone,
		PONumber:               row.PONumber,
		PODate:                 row.PODate,
		ReceivedDate:           row.ReceivedDate,
		RequestedDeliveryDate:  row.RequestedDeliveryDate,
		PaymentTerms:           row.PaymentTerms,
		Incoterms:              row.Incoterms,
		Currency:               row.Currency,
		TaxZone:                row.TaxZone,
		TaxType:                row.TaxType,
		ServiceCategory:        row.ServiceCategory,
		TaxCategory:            row.TaxCategory,
		DiscountPercent:        row.DiscountPercent,
		PpnBps:                 row.PpnBps,
		SubtotalMinor:          row.SubtotalMinor,
		DiscountMinor:          row.DiscountMinor,
		PpnMinor:               row.PpnMinor,
		AdjustmentMinor:        row.AdjustmentMinor,
		AdjustmentReason:       row.AdjustmentReason,
		GrandTotalMinor:        row.GrandTotalMinor,
		PrintedSubtotalMinor:   row.PrintedSubtotalMinor,
		PrintedDiscountMinor:   row.PrintedDiscountMinor,
		PrintedPpnMinor:        row.PrintedPpnMinor,
		PrintedGrandTotalMinor: row.PrintedGrandTotalMinor,
		SavedAt:                row.SavedAt,
		SavedBy:                row.SavedBy,
		PdfAbsentConfirmed:     row.PdfAbsentConfirmed,
		SourceMissing:          sourceState != "live",
		SourceState:            sourceState,
		PrintedTotalsKnown:     sourceState != "never",
		Version:                row.Version,
		ConfirmationNumber:     row.ConfirmationNumber,
		ConfirmationAssignedAt: row.ConfirmationAssignedAt,
		ConfirmationWarn:       row.ConfirmationWarn,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}
	dto.Items = make([]PurchaseOrderItemDTO, 0, len(row.Items))
	for _, item := range row.Items {
		dto.Items = append(dto.Items, PurchaseOrderItemDTO{
			LineNumber:      item.LineNumber,
			Description:     item.Description,
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
			GrossMinor:      item.GrossMinor,
			DiscountMinor:   item.DiscountMinor,
			NetMinor:        item.NetMinor,
		})
	}
	return dto
}

// GetPurchaseOrderReview reads one record beside the exact source revision for
// the final-save difference review.
func (q *Quotation) GetPurchaseOrderReview(id int64) (PurchaseOrderReviewDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderReviewDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderReviewDTO{}, err
	}
	var response apicontract.PurchaseOrderReviewRow
	if err := client.Get("/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/review", &response); err != nil {
		return PurchaseOrderReviewDTO{}, err
	}
	return mapPurchaseOrderReviewDTO(response), nil
}

// FinalizePurchaseOrder is the one-way draft -> saved save. A stale version
// returns a typed Conflict exactly as a draft save does.
func (q *Quotation) FinalizePurchaseOrder(input PurchaseOrderFinalizeInput) (PurchaseOrderWriteResult, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderWriteResult{}, err
	}
	items := make([]apicontract.PurchaseOrderItemInput, 0, len(input.Items))
	for _, item := range input.Items {
		items = append(items, apicontract.PurchaseOrderItemInput{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
		})
	}
	decisions := make([]apicontract.PurchaseOrderDecisionRequest, 0, len(input.Decisions))
	for _, decision := range input.Decisions {
		decisions = append(decisions, apicontract.PurchaseOrderDecisionRequest{
			Key:    decision.Key,
			Choice: decision.Choice,
		})
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Post("/v1/purchase-orders/"+strconv.FormatInt(input.ID, 10)+"/finalize",
		apicontract.PurchaseOrderFinalizeRequest{
			PurchaseOrderUpdateRequest: apicontract.PurchaseOrderUpdateRequest{
				BillingAddress:        input.BillingAddress,
				DeliveryAddress:       input.DeliveryAddress,
				ShipToCode:            input.ShipToCode,
				ContactEmail:          input.ContactEmail,
				ContactPhone:          input.ContactPhone,
				PONumber:              input.PONumber,
				PODate:                input.PODate,
				ReceivedDate:          input.ReceivedDate,
				RequestedDeliveryDate: input.RequestedDeliveryDate,
				PaymentTerms:          input.PaymentTerms,
				Incoterms:             input.Incoterms,
				Items:                 items,
				DiscountPercent:       input.DiscountPercent,
				AdjustmentMinor:       input.AdjustmentMinor,
				AdjustmentReason:      input.AdjustmentReason,
				ExpectedVersion:       input.ExpectedVersion,
				Overwrite:             input.Overwrite,
			},
			Decisions:           decisions,
			AcknowledgedMissing: input.AcknowledgedMissing,
			AcknowledgedExtra:   input.AcknowledgedExtra,
			DocumentConfirmed:   input.DocumentConfirmed,
			// #474: the client asks; it never sends a number.
			AssignConfirmationNumber: input.AssignConfirmationNumber,
		}, &response); err != nil {
		return purchaseOrderWriteFailure(err)
	}
	return purchaseOrderSaved(response), nil
}

// AssignPurchaseOrderConfirmationNumber issues the optional customer-system
// reference on a saved record. It is idempotent: the server returns the
// existing number when one is already assigned.
func (q *Quotation) AssignPurchaseOrderConfirmationNumber(id int64) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
	}
	var response apicontract.PurchaseOrderRow
	if err := client.Post(
		"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/confirmation-number",
		struct{}{}, &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

// AttachPurchaseOrderDocument stores the one customer PO PDF as the next
// version. DataBase64 is the exact file the editor read; the decoded bytes are
// bounded again here so a hand-built caller cannot push more than the server
// and schema allow.
func (q *Quotation) AttachPurchaseOrderDocument(input PurchaseOrderDocumentUploadInput) (PurchaseOrderDocumentDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDocumentDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDocumentDTO{}, err
	}
	data, err := decodePurchaseOrderPDFBase64(input.DataBase64)
	if err != nil {
		return PurchaseOrderDocumentDTO{}, err
	}
	filename := strings.TrimSpace(input.Filename)
	if len(data) == 0 {
		return PurchaseOrderDocumentDTO{}, fmt.Errorf("choose a PDF to attach")
	}
	if filename == "" || filename == "." || filename == "/" {
		filename = "customer-po.pdf"
	}
	var response apicontract.PurchaseOrderDocumentRow
	if err := client.Upload(
		"/v1/purchase-orders/"+strconv.FormatInt(input.PurchaseOrderID, 10)+"/document",
		"document", filename, data, &response); err != nil {
		return PurchaseOrderDocumentDTO{}, err
	}
	return mapPurchaseOrderDocumentDTO(response), nil
}

// GetPurchaseOrderDocumentState reads the active PDF metadata and its retained
// versions.
func (q *Quotation) GetPurchaseOrderDocumentState(id int64) (PurchaseOrderDocumentStateDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDocumentStateDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDocumentStateDTO{}, err
	}
	var response apicontract.PurchaseOrderDocumentStateRow
	if err := client.Get(
		"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/document", &response); err != nil {
		return PurchaseOrderDocumentStateDTO{}, err
	}
	state := PurchaseOrderDocumentStateDTO{
		Versions: make([]PurchaseOrderDocumentDTO, 0, len(response.Versions)),
	}
	if response.Document != nil {
		active := mapPurchaseOrderDocumentDTO(*response.Document)
		state.Document = &active
	}
	for _, document := range response.Versions {
		state.Versions = append(state.Versions, mapPurchaseOrderDocumentDTO(document))
	}
	return state, nil
}

// GetActivePurchaseOrderDocumentContent returns the current PDF bytes.
func (q *Quotation) GetActivePurchaseOrderDocumentContent(id int64) (PurchaseOrderDocumentContentDTO, error) {
	return q.getPurchaseOrderDocumentContent(
		"/v1/purchase-orders/" + strconv.FormatInt(id, 10) + "/document/content")
}

// GetPurchaseOrderDocumentVersionContent returns one retained version, so a
// replaced or removed PDF can still be inspected.
func (q *Quotation) GetPurchaseOrderDocumentVersionContent(id int64, version int32) (PurchaseOrderDocumentContentDTO, error) {
	if version < 1 {
		return PurchaseOrderDocumentContentDTO{}, fmt.Errorf("document version must be positive")
	}
	return q.getPurchaseOrderDocumentContent(
		"/v1/purchase-orders/" + strconv.FormatInt(id, 10) + "/documents/" +
			strconv.FormatInt(int64(version), 10) + "/content")
}

func (q *Quotation) getPurchaseOrderDocumentContent(endpoint string) (PurchaseOrderDocumentContentDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDocumentContentDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDocumentContentDTO{}, err
	}
	data, header, err := client.GetBytes(endpoint)
	if err != nil {
		return PurchaseOrderDocumentContentDTO{}, err
	}
	content := PurchaseOrderDocumentContentDTO{
		DataBase64:  base64.StdEncoding.EncodeToString(data),
		ContentType: header.Get("Content-Type"),
		Filename:    "customer-po.pdf",
		SHA256:      header.Get("X-Document-SHA256"),
	}
	if content.ContentType == "" {
		content.ContentType = "application/pdf"
	}
	if version, parseErr := strconv.ParseInt(header.Get("X-Document-Version"), 10, 32); parseErr == nil {
		content.Version = int32(version)
	}
	if _, params, parseErr := mime.ParseMediaType(header.Get("Content-Disposition")); parseErr == nil {
		if name := params["filename"]; name != "" {
			content.Filename = path.Base(strings.ReplaceAll(name, "\\", "/"))
		}
	}
	return content, nil
}

// RemovePurchaseOrderDocument removes the current PDF after the caller's
// explicit confirmation. The removed version stays retrievable.
func (q *Quotation) RemovePurchaseOrderDocument(id int64, confirmed bool) (PurchaseOrderDocumentDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDocumentDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDocumentDTO{}, err
	}
	var response apicontract.PurchaseOrderDocumentRow
	if err := client.DeleteWithBody(
		"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/document",
		apicontract.PurchaseOrderDocumentRemovalRequest{Confirmed: confirmed},
		&response); err != nil {
		return PurchaseOrderDocumentDTO{}, err
	}
	return mapPurchaseOrderDocumentDTO(response), nil
}

// decodePurchaseOrderPDFBase64 turns the wire value back into bytes. The bound
// is applied to the encoded length before decoding, so an oversized string is
// refused without first materializing its decoded copy.
func decodePurchaseOrderPDFBase64(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, nil
	}
	// EncodedLen is the exact text length of a file at the bound, so a longer
	// string cannot decode to an accepted size. DecodedLen would over-count
	// the padding and refuse a file of exactly 20 MiB.
	if int64(len(trimmed)) > int64(base64.StdEncoding.EncodedLen(int(store.MaxPurchaseOrderDocumentBytes))) {
		return nil, fmt.Errorf("the customer PO PDF is larger than 20 MB")
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("the customer PO PDF content is not valid base64: %w", err)
	}
	if int64(len(decoded)) > store.MaxPurchaseOrderDocumentBytes {
		return nil, fmt.Errorf("the customer PO PDF is larger than 20 MB")
	}
	return decoded, nil
}

func mapPurchaseOrderDocumentDTO(row apicontract.PurchaseOrderDocumentRow) PurchaseOrderDocumentDTO {
	return PurchaseOrderDocumentDTO{
		ID:               row.ID,
		UUID:             row.UUID,
		PurchaseOrderID:  row.PurchaseOrderID,
		Version:          row.Version,
		SHA256:           row.SHA256,
		SizeBytes:        row.SizeBytes,
		ContentType:      row.ContentType,
		OriginalFilename: row.OriginalFilename,
		UploadedBy:       row.UploadedBy,
		UploadedAt:       row.UploadedAt,
		Active:           row.Active,
		ReplacedAt:       row.ReplacedAt,
		RemovedAt:        row.RemovedAt,
		RemovedBy:        row.RemovedBy,
	}
}

func mapPurchaseOrderReviewDTO(row apicontract.PurchaseOrderReviewRow) PurchaseOrderReviewDTO {
	review := PurchaseOrderReviewDTO{
		Order:         mapPurchaseOrderDTO(row.Order),
		DocumentState: row.DocumentState,
		Source: PurchaseOrderSourceDTO{
			QuotationID:            row.Source.QuotationID,
			RevisionNumber:         row.Source.RevisionNumber,
			QuoteNumber:            row.Source.QuoteNumber,
			CustomerID:             row.Source.CustomerID,
			CustomerName:           row.Source.CustomerName,
			Currency:               row.Source.Currency,
			PaymentTerms:           row.Source.PaymentTerms,
			Incoterms:              row.Source.Incoterms,
			DiscountPercent:        row.Source.DiscountPercent,
			PrintedSubtotalMinor:   row.Source.PrintedSubtotalMinor,
			PrintedDiscountMinor:   row.Source.PrintedDiscountMinor,
			PrintedPpnMinor:        row.Source.PrintedPpnMinor,
			PrintedGrandTotalMinor: row.Source.PrintedGrandTotalMinor,
			Lines:                  make([]PurchaseOrderSourceLineDTO, 0, len(row.Source.Lines)),
		},
	}
	for _, line := range row.Source.Lines {
		review.Source.Lines = append(review.Source.Lines, PurchaseOrderSourceLineDTO{
			QuotationItemNo: line.QuotationItemNo,
			QuotationSubNo:  line.QuotationSubNo,
			LineLabel:       line.LineLabel,
			Description:     line.Description,
			UoM:             line.UoM,
			QuantityScaled:  line.QuantityScaled,
			UnitPriceMinor:  line.UnitPriceMinor,
			DiscountEnabled: line.DiscountEnabled,
			DiscountPercent: line.DiscountPercent,
		})
	}
	return review
}
