package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #469: the PO archive. One authenticated read lists the caller's
// organization's purchase orders 15 at a time by default (#492 lets the caller
// ask for 1..100), newest received first, with search, one-or-more customers,
// an inclusive received-date range, the stored Draft/Saved state and the
// independent source-missing predicate.
//
// Filters are validated here before any query runs: an unknown state or
// wildcard filter is a 400, not an empty page that quietly means "everything".

func (s *Server) listPurchaseOrders(context echo.Context) error {
	page := 1
	if raw := context.QueryParam("page"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid page"})
		}
		page = parsed
	}

	// page_size is optional (#492). Its range is the store's guard, exactly like
	// the page-offset guard, so malformed input surfaces as ErrPurchaseOrderInvalid
	// and a 400; only "not a number" is refused here, where the parse happens.
	pageSize := 0
	if raw := context.QueryParam(apicontract.PurchaseOrderListPageSizeParam); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid page_size"})
		}
		pageSize = parsed
	}

	customerIDs := make([]int64, 0, 4)
	seen := map[int64]bool{}
	for _, raw := range context.QueryParams()["customerId"] {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || id < 1 {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid customerId"})
		}
		if !seen[id] {
			seen[id] = true
			customerIDs = append(customerIDs, id)
		}
	}

	state := strings.TrimSpace(context.QueryParam("state"))
	switch state {
	case "", store.PurchaseOrderStateDraft, store.PurchaseOrderStateSaved:
	default:
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid state"})
	}

	sourceMissing := strings.TrimSpace(context.QueryParam("sourceMissing"))
	if sourceMissing == "" {
		sourceMissing = store.PurchaseOrderSourceMissingAny
	}
	switch sourceMissing {
	case store.PurchaseOrderSourceMissingAny,
		store.PurchaseOrderSourceMissingOnly,
		store.PurchaseOrderSourceMissingExclude:
	default:
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid sourceMissing"})
	}

	receivedFrom, err := parsePurchaseOrderDate(context.QueryParam("receivedFrom"), "receivedFrom")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	receivedTo, err := parsePurchaseOrderDate(context.QueryParam("receivedTo"), "receivedTo")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if receivedFrom != nil && receivedTo != nil && receivedFrom.After(*receivedTo) {
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "receivedFrom must be on or before receivedTo",
		})
	}

	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	result, err := s.database.ListPurchaseOrders(requestContext, store.PurchaseOrderListQuery{
		Search:        context.QueryParam("search"),
		State:         state,
		CustomerIDs:   customerIDs,
		ReceivedFrom:  receivedFrom,
		ReceivedTo:    receivedTo,
		SourceMissing: sourceMissing,
		Page:          page,
		PageSize:      pageSize,
	})
	if errors.Is(err, store.ErrPurchaseOrderInvalid) {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list purchase orders"})
	}

	rows := make([]apicontract.PurchaseOrderListRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		rows = append(rows, mapPurchaseOrderListRow(row))
	}
	customers := make([]apicontract.PurchaseOrderCustomerRow, 0, len(result.Customers))
	for _, customer := range result.Customers {
		customers = append(customers, apicontract.PurchaseOrderCustomerRow{
			ID:   customer.ID,
			Name: customer.Name,
		})
	}
	return context.JSON(http.StatusOK, apicontract.PurchaseOrderListResponse{
		PurchaseOrders: rows,
		Customers:      customers,
		Page:           result.Page,
		PageSize:       result.PageSize,
		Total:          result.Total,
		TotalPages:     result.TotalPages,
	})
}

func mapPurchaseOrderListRow(row store.PurchaseOrderListRow) apicontract.PurchaseOrderListRow {
	return apicontract.PurchaseOrderListRow{
		ID:             row.ID,
		UUID:           row.UUID,
		State:          row.State,
		PONumber:       row.PONumber,
		QuoteNumber:    row.QuoteNumber,
		SourceRevision: row.SourceRevision,
		CustomerID:     row.CustomerID,
		CustomerName:   row.CustomerName,
		ReceivedDate:   row.ReceivedDate.Format(time.DateOnly),
		Currency:       row.Currency,
		SourceMissing:  row.SourceMissing,
		SourceState:    row.SourceState,
	}
}
