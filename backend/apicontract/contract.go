package apicontract

import (
	"encoding/json"
	"fmt"
)

// This file defines the shared HTTP contract between veraserver and the
// desktop client. Field names are stable API; the desktop code-generates or
// mirrors these shapes in #80.

// FormatQuoteNumber renders a plain-digit quotation number: 15 + 2-digit
// customer code + 2-digit issue year + sequence. The normal width is 10
// digits (0001-9999); past 9999 the sequence widens to 5 digits instead of
// wrapping, so overflow years print 11 digits. No grouped display anywhere.
// It lives here (not backend) so store can build numbers without importing
// backend, which would be an import cycle.
func FormatQuoteNumber(customerCode, yy int, seq int64) string {
	return fmt.Sprintf("15%02d%02d%04d", customerCode, yy, seq)
}

// This file defines the shared HTTP contract between veraserver and the
// desktop client. Field names are stable API; the desktop code-generates or
// mirrors these shapes in #80.

// ExportResponse identifies the persisted revision.
type ExportResponse struct {
	QuoteNumber  string `json:"quoteNumber"`
	UUID         string `json:"uuid"`
	ShortID      string `json:"shortId"`
	ID           int64  `json:"id"`
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
	CustomerID   *int64          `json:"customerId"`
}

// QuotationSummaryRow is one archive row: always the latest revision.
type QuotationSummaryRow struct {
	QuoteNumber    string `json:"quoteNumber"`
	CustomerName   string `json:"customerName"`
	Subject        string `json:"subject"`
	Currency       string `json:"currency"`
	CreatedAt      string `json:"createdAt"`
	DisplayGrand   string `json:"displayGrand"`
	ID             int64  `json:"id"`
	GrandTotal     int64  `json:"grandTotal"`
	RevisionNumber int16  `json:"revision"`
}

// POSourceActivePORow is the active Purchase Order occupying a Quotation
// (issue #468). A draft offers "Open draft"; a saved record stays disabled.
type POSourceActivePORow struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
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

// PurchaseOrderItemRow is one entered PO line. QuotationItemNo and
// QuotationSubNo are null for an extra line the source revision never carried.
// QuantityScaled is an exact count of 1/10000 units; UnitPriceMinor is in the
// PO currency's minor units.
type PurchaseOrderItemRow struct {
	QuotationItemNo *int   `json:"quotationItemNo"`
	QuotationSubNo  *int   `json:"quotationSubNo"`
	LineNumber      int    `json:"lineNumber"`
	Description     string `json:"description"`
	QuantityScaled  int64  `json:"quantityScaled"`
	UoM             string `json:"uom"`
	UnitPriceMinor  int64  `json:"unitPriceMinor"`
}

// PurchaseOrderRow is one PO draft. Dates are plain YYYY-MM-DD; the optional
// customer PO date and requested delivery date are null when unset.
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
	TaxZone               string                 `json:"taxZone"`
	TaxCategory           string                 `json:"taxCategory"`
	CreatedAt             string                 `json:"createdAt"`
	UpdatedAt             string                 `json:"updatedAt"`
	Items                 []PurchaseOrderItemRow `json:"items"`
	ID                    int64                  `json:"id"`
	QuotationID           int64                  `json:"quotationId"`
	SourceRevision        int16                  `json:"sourceRevision"`
	CustomerID            int64                  `json:"customerId"`
	PaymentTermsDays      int16                  `json:"paymentTermsDays"`
}

// PurchaseOrderCreateRequest starts a draft from one exact issued revision.
// RevisionNumber is a pointer because revision 0 is real and an absent field
// must not read as it. ReceivedDate is optional; the server defaults it to
// today.
type PurchaseOrderCreateRequest struct {
	QuotationID    int64  `json:"quotationId"`
	RevisionNumber *int16 `json:"revisionNumber"`
	ReceivedDate   string `json:"receivedDate,omitempty"`
}

// PurchaseOrderItemInput is one replace-lines entry. LineNumber is assigned by
// the server from array order, so drafts always carry 1..n without gaps.
type PurchaseOrderItemInput struct {
	QuotationItemNo *int   `json:"quotationItemNo"`
	QuotationSubNo  *int   `json:"quotationSubNo"`
	Description     string `json:"description"`
	QuantityScaled  int64  `json:"quantityScaled"`
	UoM             string `json:"uom"`
	UnitPriceMinor  int64  `json:"unitPriceMinor"`
}

// PurchaseOrderUpdateRequest replaces the editable draft fields. Source,
// customer snapshots, currency and tax basis are not writable.
type PurchaseOrderUpdateRequest struct {
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
	Items                 []PurchaseOrderItemInput `json:"items"`
	PaymentTermsDays      int16                    `json:"paymentTermsDays"`
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
	DisplayMarginalizedSubtotal string             `json:"displayMarginalizedSubtotal"`
	DisplayMarginalizedDiscount string             `json:"displayMarginalizedDiscount"`
	UUID                        string             `json:"uuid"`
	HTMLSnapshot                string             `json:"htmlSnapshot"`
	DisplayNett                 string             `json:"displayNett"`
	CustomerName                string             `json:"customerName"`
	Subject                     string             `json:"subject"`
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
}

// AttachDocumentRequest uploads locally rendered PDF bytes for a revision.
type AttachDocumentRequest struct {
	PDFBase64 string `json:"pdfBase64"`
	PDFPath   string `json:"pdfPath"`
}
