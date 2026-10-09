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
	CreatedAt      string `json:"createdAt"`
	DisplayGrand   string `json:"displayGrand"`
	ID             int64  `json:"id"`
	GrandTotal     int64  `json:"grandTotal"`
	RevisionNumber int16  `json:"revision"`
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
	Revision                    int16              `json:"revision"`
	HasPDF                      bool               `json:"hasPdf"`
}

// AttachDocumentRequest uploads locally rendered PDF bytes for a revision.
type AttachDocumentRequest struct {
	PDFBase64 string `json:"pdfBase64"`
	PDFPath   string `json:"pdfPath"`
}
