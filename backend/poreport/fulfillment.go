package poreport

import (
	"fmt"
	"strings"
)

// Issue #657: the PO Fulfillment drill-down.
//
// Fulfillment answers, per quotation and per priced leaf, what the customer
// asked for, what saved purchase orders actually bought, and what is still
// unsold, over-bought, closed or removed. It is a read-only, paginated
// projection over the same facts the report aggregates: a quotation's latest
// issued revision supplies the quoted leaves and their customer baseline, and
// cross-revision saved allocations supply the bought side.
//
// # Money
//
// Every amount is int64 minor units in one currency. No supplier unit cost is
// exposed here -- only realized margin, which is the same aggregate figure the
// report already publishes. Quantities use the store's 1/10000 scale.
//
// # Status vocabulary
//
// The status is derived, never stored: closed > all_bought > expired_unclosed >
// open. A closed quotation's remainder is Closed Loss, not Unsold; a fully
// bought quotation is never "expired"; expiry is a warning about an open offer,
// never a loss.

// FulfillmentStatus is the stable wire vocabulary of a quotation's fulfillment
// state. The values are the only ones a query or a page may carry.
type FulfillmentStatus string

const (
	// FulfillmentStatusOpen is a quotation whose latest revision is still
	// valid, open, and has at least one leaf with room to buy.
	FulfillmentStatusOpen FulfillmentStatus = "open"
	// FulfillmentStatusExpiredUnclosed is an open quotation whose latest
	// revision's validity window has passed. Its remainder is still Unsold.
	FulfillmentStatusExpiredUnclosed FulfillmentStatus = "expired_unclosed"
	// FulfillmentStatusAllBought is a quotation whose every latest leaf has
	// been bought up (or over-bought). A newer issue resets this.
	FulfillmentStatusAllBought FulfillmentStatus = "all_bought"
	// FulfillmentStatusClosed is a quotation an admin explicitly closed. Its
	// unbought remainder is Closed Loss, and closure wins over every other
	// state.
	FulfillmentStatusClosed FulfillmentStatus = "closed"
	// FulfillmentStatusAll is the query's "every status" value. It is not a
	// quotation state and never appears on a page row.
	FulfillmentStatusAll FulfillmentStatus = "all"
)

const (
	// FulfillmentDefaultPageSize is the approved page size of the drill-down:
	// 25 quotations, never item rows.
	FulfillmentDefaultPageSize = 25
	// FulfillmentMaxPageSize bounds a caller-supplied page size. It mirrors the
	// archive's ceiling so two paginated reads cannot disagree about what is a
	// reasonable page.
	FulfillmentMaxPageSize = 100
)

// IsFulfillmentStatus reports whether value is one of the four derived
// quotation states. The "all" filter value is deliberately not a state.
func IsFulfillmentStatus(value FulfillmentStatus) bool {
	switch value {
	case FulfillmentStatusOpen, FulfillmentStatusExpiredUnclosed,
		FulfillmentStatusAllBought, FulfillmentStatusClosed:
		return true
	default:
		return false
	}
}

// FulfillmentQuery is one validated drill-down request. Currency is required;
// Status is "" or "all" for every status, otherwise one exact state. Search
// matches the quotation number or the customer name snapshot. There is no
// date-range filter: bought state spans revisions and time, so a date filter
// would answer half the question.
type FulfillmentQuery struct {
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Search   string `json:"search"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

// FulfillmentContribution is one saved purchase order's contribution to one
// quoted leaf. Drafts never contribute: they are advisory and reserve nothing.
type FulfillmentContribution struct {
	ID                      int64  `json:"id"`
	PONumber                string `json:"poNumber"`
	State                   string `json:"state"`
	ReceivedDate            string `json:"receivedDate"`
	AllocatedQuantityScaled int64  `json:"allocatedQuantityScaled"`
}

// FulfillmentItem is one priced leaf of the quotation's latest revision, or a
// bought leaf removed by a later revision. A removed leaf keeps its realized
// variance but reports zero quoted, unsold and over-bought quantity, so it can
// never read as a false over-buy. GroupTitle comes from the earliest received
// saved PO line that bought the leaf (the order contributions are listed in); a
// group header is never a row of its own, so two grouped children that share a
// title are two rows, not one group row.
//
// RealizedAmountMinor is the allocated revenue, and RealizedMarginMinor is the
// realized costed revenue minus its rounded capital. An unknown supplier cost
// contributes no margin rather than presenting the whole revenue as profit.
type FulfillmentItem struct {
	QuotationItemNo int    `json:"quotationItemNo"`
	QuotationSubNo  *int   `json:"quotationSubNo"`
	Label           string `json:"label"`
	Description     string `json:"description"`
	GroupTitle      string `json:"groupTitle,omitempty"`
	Removed         bool   `json:"removed,omitempty"`

	QuotedQuantityScaled     int64 `json:"quotedQuantityScaled"`
	BoughtQuantityScaled     int64 `json:"boughtQuantityScaled"`
	UnsoldQuantityScaled     int64 `json:"unsoldQuantityScaled"`
	OverBoughtQuantityScaled int64 `json:"overBoughtQuantityScaled"`

	RealizedAmountMinor int64 `json:"realizedAmountMinor"`
	RealizedMarginMinor int64 `json:"realizedMarginMinor"`
	UnsoldAmountMinor   int64 `json:"unsoldAmountMinor"`
	UnsoldMarginMinor   int64 `json:"unsoldMarginMinor"`

	QuantityExtraIncomeAmountMinor int64 `json:"quantityExtraIncomeAmountMinor"`
	QuantityExtraIncomeMarginMinor int64 `json:"quantityExtraIncomeMarginMinor"`
	PriceExtraIncomeMinor          int64 `json:"priceExtraIncomeMinor"`
	PriceLossMinor                 int64 `json:"priceLossMinor"`
	ClosedLossAmountMinor          int64 `json:"closedLossAmountMinor"`
	ClosedLossMarginMinor          int64 `json:"closedLossMarginMinor"`

	PurchaseOrders []FulfillmentContribution `json:"purchaseOrders"`
}

// FulfillmentExtraExpense is one explicit extra expense line on a saved PO for
// this quotation. It is listed separately and never appears in any quoted,
// bought, unsold or over-bought figure. GroupTitle carries the aggregate title
// when the extra was grouped with quotation children.
type FulfillmentExtraExpense struct {
	PurchaseOrderID int64  `json:"purchaseOrderId"`
	PONumber        string `json:"poNumber"`
	LineNumber      int    `json:"lineNumber"`
	Description     string `json:"description"`
	QuantityScaled  int64  `json:"quantityScaled"`
	UoM             string `json:"uom"`
	AmountMinor     int64  `json:"amountMinor"`
	GroupTitle      string `json:"groupTitle,omitempty"`
}

// FulfillmentQuotation is one quotation row of a fulfillment page. IssuedAt and
// ValidUntil are WIB calendar dates (YYYY-MM-DD); ValidUntil is the latest
// issued revision's last valid day.
type FulfillmentQuotation struct {
	QuotationID    int64                     `json:"quotationId"`
	QuoteNumber    string                    `json:"quoteNumber"`
	CustomerName   string                    `json:"customerName"`
	Currency       string                    `json:"currency"`
	LatestRevision int16                     `json:"latestRevision"`
	IssuedAt       string                    `json:"issuedAt"`
	ValidUntil     string                    `json:"validUntil"`
	Status         string                    `json:"status"`
	Items          []FulfillmentItem         `json:"items"`
	ExtraExpenses  []FulfillmentExtraExpense `json:"extraExpenses"`
}

// FulfillmentPage is one filtered, paginated page of quotation rows. Total is
// the filtered count before paging, so the page and the count can never
// disagree.
type FulfillmentPage struct {
	AsOf       string                 `json:"asOf"`
	Filter     FulfillmentQuery       `json:"filter"`
	Quotations []FulfillmentQuotation `json:"quotations"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	Total      int                    `json:"total"`
	TotalPages int                    `json:"totalPages"`
}

// NormalizeFulfillmentQuery trims and validates one request. An empty status
// means every status. A page below one is the first page; a page size of zero is
// the default; a page size outside 1..FulfillmentMaxPageSize is refused rather
// than clamped.
func NormalizeFulfillmentQuery(query FulfillmentQuery) (FulfillmentQuery, error) {
	query.Currency = strings.TrimSpace(query.Currency)
	if query.Currency == "" {
		return FulfillmentQuery{}, fmt.Errorf("%w: currency is required", ErrInvalid)
	}

	query.Status = strings.TrimSpace(query.Status)
	if query.Status == "" {
		query.Status = string(FulfillmentStatusAll)
	}
	status := FulfillmentStatus(query.Status)
	if status != FulfillmentStatusAll && !IsFulfillmentStatus(status) {
		return FulfillmentQuery{}, fmt.Errorf(
			"%w: status %q must be all, open, expired_unclosed, all_bought or closed",
			ErrInvalid, query.Status)
	}

	query.Search = strings.TrimSpace(query.Search)

	if query.Page < 1 {
		query.Page = 1
	}
	switch {
	case query.PageSize == 0:
		query.PageSize = FulfillmentDefaultPageSize
	case query.PageSize < 0 || query.PageSize > FulfillmentMaxPageSize:
		return FulfillmentQuery{}, fmt.Errorf(
			"%w: page size %d is outside 1..%d",
			ErrInvalid, query.PageSize, FulfillmentMaxPageSize)
	}
	return query, nil
}
