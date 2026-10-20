package backend

import (
	"encoding/base64"
	"fmt"
	"mime"
	"path"
	"strconv"
	"strings"

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
	DocumentConfirmed     bool
}

// PurchaseOrderCreateInput is the selector's confirmed pair plus an optional
// received-date override. An empty date asks the server for today.
type PurchaseOrderCreateInput struct {
	ReceivedDate   string
	QuotationID    int64
	RevisionNumber int16
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

// UpdatePurchaseOrderDraft replaces the editable draft values.
func (q *Quotation) UpdatePurchaseOrderDraft(input PurchaseOrderUpdateInput) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
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
		}, &response); err != nil {
		return PurchaseOrderDTO{}, err
	}
	return mapPurchaseOrderDTO(response), nil
}

func mapPurchaseOrderDTO(row apicontract.PurchaseOrderRow) PurchaseOrderDTO {
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

// FinalizePurchaseOrder is the one-way draft -> saved save.
func (q *Quotation) FinalizePurchaseOrder(input PurchaseOrderFinalizeInput) (PurchaseOrderDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderDTO{}, err
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
			},
			Decisions:           decisions,
			AcknowledgedMissing: input.AcknowledgedMissing,
			AcknowledgedExtra:   input.AcknowledgedExtra,
			DocumentConfirmed:   input.DocumentConfirmed,
		}, &response); err != nil {
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
