package api

// This file defines the shared HTTP contract between veraserver and the
// desktop client. Field names are stable API; the desktop code-generates or
// mirrors these shapes in #80.

// ExportResponse identifies the persisted revision.
type ExportResponse struct {
	ID          int64  `json:"id"`
	QuoteNumber string `json:"quoteNumber"`
	Revision    int16  `json:"revision"`
	GrandTotal  uint64 `json:"grandTotal"`
}

// QuotationSummaryRow is one archive row: always the latest revision.
type QuotationSummaryRow struct {
	ID             int64  `json:"id"`
	QuoteNumber    string `json:"quoteNumber"`
	RevisionNumber int16  `json:"revision"`
	CustomerName   string `json:"customerName"`
	Subject        string `json:"subject"`
	CreatedAt      string `json:"createdAt"`
	GrandTotal     int64  `json:"grandTotal"`
	DisplayGrand   string `json:"displayGrand"`
}

// QuotationItemRow mirrors one internal line. PDF bytes are never included.
type QuotationItemRow struct {
	ItemNo                   int    `json:"itemNo"`
	Description              string `json:"description"`
	Qty                      int64  `json:"qty"`
	UoM                      string `json:"uom"`
	OriginalUnitPrice        int64  `json:"originalUnitPrice"`
	MarginEnabled            bool   `json:"marginEnabled"`
	MarginPercent            int16  `json:"marginPercent"`
	MarginAmount             int64  `json:"marginAmount"`
	OriginalTotal            int64  `json:"originalTotal"`
	MarginalizedUnitPrice    int64  `json:"marginalizedUnitPrice"`
	MarginalizedTotal        int64  `json:"marginalizedTotal"`
	DisplayOriginalUnit      string `json:"displayOriginalUnit"`
	DisplayOriginalTotal     string `json:"displayOriginalTotal"`
	DisplayMarginalizedUnit  string `json:"displayMarginalizedUnit"`
	DisplayMarginalizedTotal string `json:"displayMarginalizedTotal"`
	DisplayMarginAmount      string `json:"displayMarginAmount"`
}

// QuotationDetail is one revision: internal economics plus the customer HTML
// snapshot. Raw PDF bytes are intentionally absent; HasPDF reports attachment.
type QuotationDetail struct {
	ID                          int64              `json:"id"`
	QuoteNumber                 string             `json:"quoteNumber"`
	Revision                    int16              `json:"revision"`
	AvailableRevisions          []int16            `json:"availableRevisions"`
	CustomerName                string             `json:"customerName"`
	Subject                     string             `json:"subject"`
	RecipientName               string             `json:"recipientName"`
	Incoterm                    string             `json:"incoterm"`
	DiscountPercent             int16              `json:"discountPercent"`
	TaxZone                     string             `json:"taxZone"`
	TaxCategory                 string             `json:"taxCategory"`
	TaxLabel                    string             `json:"taxLabel"`
	OriginalSubtotal            int64              `json:"originalSubtotal"`
	OriginalDiscount            int64              `json:"originalDiscount"`
	OriginalGrand               int64              `json:"originalGrand"`
	MarginalizedSubtotal        int64              `json:"marginalizedSubtotal"`
	MarginalizedDiscount        int64              `json:"marginalizedDiscount"`
	MarginalizedGrand           int64              `json:"marginalizedGrand"`
	GrandTotal                  int64              `json:"grandTotal"`
	TaxAmount                   int64              `json:"taxAmount"`
	MarginalizedGrandAfterTax   int64              `json:"marginalizedGrandAfterTax"`
	ProfitAmount                int64              `json:"profitAmount"`
	NettProfit                  int64              `json:"nettProfit"`
	DisplayOriginalSubtotal     string             `json:"displayOriginalSubtotal"`
	DisplayOriginalDiscount     string             `json:"displayOriginalDiscount"`
	DisplayOriginalGrand        string             `json:"displayOriginalGrand"`
	DisplayMarginalizedSubtotal string             `json:"displayMarginalizedSubtotal"`
	DisplayMarginalizedDiscount string             `json:"displayMarginalizedDiscount"`
	DisplayMarginalizedGrand    string             `json:"displayMarginalizedGrand"`
	DisplayTax                  string             `json:"displayTax"`
	DisplayMarginalizedAfterTax string             `json:"displayMarginalizedAfterTax"`
	DisplayProfit               string             `json:"displayProfit"`
	DisplayNett                 string             `json:"displayNett"`
	Items                       []QuotationItemRow `json:"items"`
	HTMLSnapshot                string             `json:"htmlSnapshot"`
	HasPDF                      bool               `json:"hasPdf"`
}

// AttachDocumentRequest uploads locally rendered PDF bytes for a revision.
type AttachDocumentRequest struct {
	PDFBase64 string `json:"pdfBase64"`
	PDFPath   string `json:"pdfPath"`
}
