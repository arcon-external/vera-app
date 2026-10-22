package backend

import (
	"net/url"
	"strconv"
	"strings"

	"vera-app/backend/apicontract"
)

// Issue #469: desktop bindings for the Purchase Order archive. The desktop has
// no HTTP layer of its own -- it calls the same authenticated API as every
// other surface -- so this wrapper sends the typed query and maps the answer
// without dropping a field the list renders.
//
// PO binding methods live on *Quotation because that is the one bound object
// the PO slice extends (there is no separate PurchaseOrder binding): the same
// class already owns CreatePurchaseOrderDraft/GetPurchaseOrder from #468.

// PurchaseOrderListQuery is one archive page request. Empty strings mean "no
// filter"; State is the stored entry state (draft or saved); SourceMissing is
// "only", "exclude" or empty for any. CustomerIDs is a multi-select. PageSize 0
// keeps the server default (15); the server accepts 1..100 and refuses the rest.
type PurchaseOrderListQuery struct {
	Search        string
	State         string
	CustomerIDs   []int64
	ReceivedFrom  string
	ReceivedTo    string
	SourceMissing string
	Page          int
	PageSize      int
}

// PurchaseOrderListRowDTO is one archive row.
type PurchaseOrderListRowDTO struct {
	UUID           string
	State          string
	PONumber       string
	QuoteNumber    string
	CustomerName   string
	ReceivedDate   string
	Currency       string
	ID             int64
	CustomerID     int64
	SourceRevision int16
	SourceMissing  bool
	// SourceState is "live", "never" or "former" (#496).
	SourceState string
}

// PurchaseOrderListCustomerDTO is one option of the customer multi-select.
type PurchaseOrderListCustomerDTO struct {
	Name string
	ID   int64
}

// PurchaseOrderListPageDTO is one page plus the org-wide customer facet.
type PurchaseOrderListPageDTO struct {
	PurchaseOrders []PurchaseOrderListRowDTO
	Customers      []PurchaseOrderListCustomerDTO
	Page           int
	PageSize       int
	Total          int
	TotalPages     int
}

// ListPurchaseOrders fetches one page of the PO archive. The server defaults the
// page size to 15, honours a caller's 1..100 (#492), and scopes every query to
// the caller's organization.
func (q *Quotation) ListPurchaseOrders(query PurchaseOrderListQuery) (PurchaseOrderListPageDTO, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return PurchaseOrderListPageDTO{}, err
	}
	client, err := api()
	if err != nil {
		return PurchaseOrderListPageDTO{}, err
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	params := url.Values{}
	params.Set("page", strconv.Itoa(page))
	// An absent page size means "use the server default", so only a caller who
	// named one sends the field. Its range is the server's guard, not this
	// wrapper's: an out-of-range value comes back as the API's 400.
	if query.PageSize != 0 {
		params.Set(apicontract.PurchaseOrderListPageSizeParam, strconv.Itoa(query.PageSize))
	}
	if strings.TrimSpace(query.Search) != "" {
		params.Set("search", query.Search)
	}
	if state := strings.TrimSpace(query.State); state != "" {
		params.Set("state", state)
	}
	if from := strings.TrimSpace(query.ReceivedFrom); from != "" {
		params.Set("receivedFrom", from)
	}
	if to := strings.TrimSpace(query.ReceivedTo); to != "" {
		params.Set("receivedTo", to)
	}
	if mode := strings.TrimSpace(query.SourceMissing); mode != "" && mode != "any" {
		params.Set("sourceMissing", mode)
	}
	for _, id := range query.CustomerIDs {
		params.Add("customerId", strconv.FormatInt(id, 10))
	}

	var response apicontract.PurchaseOrderListResponse
	if err := client.Get("/v1/purchase-orders?"+params.Encode(), &response); err != nil {
		return PurchaseOrderListPageDTO{}, err
	}

	rows := make([]PurchaseOrderListRowDTO, 0, len(response.PurchaseOrders))
	for _, row := range response.PurchaseOrders {
		rows = append(rows, PurchaseOrderListRowDTO{
			ID:             row.ID,
			UUID:           row.UUID,
			State:          row.State,
			PONumber:       row.PONumber,
			QuoteNumber:    row.QuoteNumber,
			SourceRevision: row.SourceRevision,
			CustomerID:     row.CustomerID,
			CustomerName:   row.CustomerName,
			ReceivedDate:   row.ReceivedDate,
			Currency:       row.Currency,
			SourceMissing:  row.SourceMissing,
			SourceState:    row.SourceState,
		})
	}
	customers := make([]PurchaseOrderListCustomerDTO, 0, len(response.Customers))
	for _, customer := range response.Customers {
		customers = append(customers, PurchaseOrderListCustomerDTO{
			ID:   customer.ID,
			Name: customer.Name,
		})
	}
	return PurchaseOrderListPageDTO{
		PurchaseOrders: rows,
		Customers:      customers,
		Page:           response.Page,
		PageSize:       response.PageSize,
		Total:          response.Total,
		TotalPages:     response.TotalPages,
	}, nil
}
