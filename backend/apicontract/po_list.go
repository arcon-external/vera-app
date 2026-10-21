package apicontract

// Issue #469: the Purchase Order archive's HTTP contract. These shapes are
// stable API for the desktop binding and any later client. PageSize is fixed
// by the server; the caller only names a page.

// PurchaseOrderListRow is one archive row. PONumber is the customer's own
// number and stays empty until it is entered; QuoteNumber identifies the exact
// source Quotation, or the former one when SourceMissing is true. Deletion
// (#480) may remove the Quotation while the retained PO survives.
type PurchaseOrderListRow struct {
	UUID           string `json:"uuid"`
	State          string `json:"state"`
	PONumber       string `json:"poNumber"`
	QuoteNumber    string `json:"quoteNumber"`
	CustomerName   string `json:"customerName"`
	ReceivedDate   string `json:"receivedDate"`
	Currency       string `json:"currency"`
	ID             int64  `json:"id"`
	CustomerID     int64  `json:"customerId"`
	SourceRevision int16  `json:"sourceRevision"`
	// SourceMissing is independent of the stored draft/saved state: true when
	// the followed Quotation was permanently deleted (#480) and the row now
	// carries a former-source reference until an admin repairs it (#481).
	SourceMissing bool `json:"sourceMissing"`
}

// PurchaseOrderCustomerRow is one option of the customer multi-select. The
// facet lists every customer that owns a PO in the organization, unaffected by
// the current page's other filters.
type PurchaseOrderCustomerRow struct {
	Name string `json:"name"`
	ID   int64  `json:"id"`
}

// PurchaseOrderListResponse is one page of the archive plus the org-wide
// customer facet. An empty result still carries one page and empty arrays.
type PurchaseOrderListResponse struct {
	PurchaseOrders []PurchaseOrderListRow     `json:"purchaseOrders"`
	Customers      []PurchaseOrderCustomerRow `json:"customers"`
	Page           int                        `json:"page"`
	PageSize       int                        `json:"pageSize"`
	Total          int                        `json:"total"`
	TotalPages     int                        `json:"totalPages"`
}
