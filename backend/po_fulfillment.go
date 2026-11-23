package backend

import (
	"net/url"
	"strconv"
	"strings"

	"vera-app/backend/apicontract"
	"vera-app/backend/poreport"
)

// Issue #657: the desktop's read of the PO Fulfillment drill-down. The desktop
// has no fulfillment arithmetic of its own: it calls the same authenticated,
// paginated endpoint the Reports screen uses, sends the validated filter, and
// returns the server's page unchanged. One payload is what keeps the screen and
// the server's status derivation from disagreeing.
//
// The query fields travel under the contract's own names (currency, status,
// search, page) with page_size as the one archive-compatible spelling. A
// defaulted value is omitted so the server applies its documented default:
// the company currency, every status, the first page and 25 rows.
func (q *Quotation) GetPurchaseOrderFulfillment(
	query poreport.FulfillmentQuery,
) (poreport.FulfillmentPage, error) {
	if err := q.requireUser(q.ctx); err != nil {
		return poreport.FulfillmentPage{}, err
	}
	client, err := api()
	if err != nil {
		return poreport.FulfillmentPage{}, err
	}
	params := url.Values{}
	if currency := strings.TrimSpace(query.Currency); currency != "" {
		params.Set("currency", currency)
	}
	if status := strings.TrimSpace(query.Status); status != "" &&
		status != string(poreport.FulfillmentStatusAll) {
		params.Set("status", status)
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		params.Set("search", search)
	}
	if query.Page > 0 {
		params.Set("page", strconv.Itoa(query.Page))
	}
	if query.PageSize > 0 {
		params.Set(apicontract.PurchaseOrderFulfillmentPageSizeParam,
			strconv.Itoa(query.PageSize))
	}

	var page poreport.FulfillmentPage
	if err := client.Get(
		apicontract.PurchaseOrderFulfillmentPath+"?"+params.Encode(), &page); err != nil {
		return poreport.FulfillmentPage{}, err
	}
	return page, nil
}
