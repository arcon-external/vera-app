package apicontract

import "vera-app/backend/poreport"

// Issue #657: the PO Fulfillment drill-down's HTTP contract. The domain package
// owns the one declaration of the page shape, because the desktop binding
// returns poreport.FulfillmentPage directly and Wails generates its model from
// that Go type. These aliases let the server name the wire type without copying
// it, so a JSON tag can never drift between client and server.
//
// PurchaseOrderFulfillmentPath is shared so the server route and the desktop
// wrapper cannot disagree about the URL. The query parameters are the
// FulfillmentQuery struct's own names (currency, status, search, page) with
// page_size as the one archive-compatible spelling, because the archive has
// always spelled that field in snake case.
const PurchaseOrderFulfillmentPath = "/v1/reports/purchase-orders/fulfillment"

// PurchaseOrderFulfillmentPageSizeParam is the page-size query parameter. It
// matches the archive's spelling so the two paginated reads cannot disagree
// about what "page_size" means.
const PurchaseOrderFulfillmentPageSizeParam = "page_size"

// PurchaseOrderFulfillmentPage is one filtered, paginated page of quotation
// rows. Total is the filtered count before paging; TotalPages is never below
// one, so an empty page still has working pagination controls.
type PurchaseOrderFulfillmentPage = poreport.FulfillmentPage

// PurchaseOrderFulfillmentQuery is the validated filter: one currency, one of
// the four derived statuses or all, a quote-number/customer search term, and
// one page. There is no date-range filter.
type PurchaseOrderFulfillmentQuery = poreport.FulfillmentQuery

// PurchaseOrderFulfillmentQuotation is one quotation row: its status, dates,
// priced leaves and separately listed extra expenses.
type PurchaseOrderFulfillmentQuotation = poreport.FulfillmentQuotation

// PurchaseOrderFulfillmentItem is one priced leaf's quoted/bought position, its
// realized and unsold figures, and the saved purchase orders that bought it.
// It carries no supplier unit cost, by construction: only aggregate realized
// margin, which the report already publishes.
type PurchaseOrderFulfillmentItem = poreport.FulfillmentItem

// PurchaseOrderFulfillmentContribution is one saved purchase order's
// allocation to one leaf. Drafts never contribute.
type PurchaseOrderFulfillmentContribution = poreport.FulfillmentContribution

// PurchaseOrderFulfillmentExtraExpense is one explicit extra expense line,
// listed separately and never counted in any quantity.
type PurchaseOrderFulfillmentExtraExpense = poreport.FulfillmentExtraExpense

// PurchaseOrderFulfillmentStatus is the stable status vocabulary: open,
// expired_unclosed, all_bought or closed.
type PurchaseOrderFulfillmentStatus = poreport.FulfillmentStatus

// The four derived statuses, aliased so the API and a client can name them
// without re-typing the strings.
const (
	FulfillmentStatusOpen            = poreport.FulfillmentStatusOpen
	FulfillmentStatusExpiredUnclosed = poreport.FulfillmentStatusExpiredUnclosed
	FulfillmentStatusAllBought       = poreport.FulfillmentStatusAllBought
	FulfillmentStatusClosed          = poreport.FulfillmentStatusClosed
	FulfillmentStatusAll             = poreport.FulfillmentStatusAll
)
