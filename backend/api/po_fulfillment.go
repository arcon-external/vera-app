package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/currency"
	"vera-app/backend/poreport"
)

// Issue #657: the PO Fulfillment drill-down. One authenticated, paginated read
// returns the per-quotation status and the per-leaf quoted/bought/unsold/
// over-bought position, the realized and unsold figures, every variance
// component, the saved purchase orders behind each leaf, the aggregate titles
// on their children, and the extra expense lines listed separately.
//
// It is a read of the same organization the PO archive and the report read, and
// it adds no role restriction beyond a session: staff who record the orders may
// see what was bought. Currency is a partition, so an omitted code defaults to
// the company's own and an unknown code is refused rather than silently
// treated as the default. There is no date filter by design: bought state spans
// revisions and time, so a date range would answer half the question.

const (
	// fulfillmentErrorInvalid is the stable refusal code for a malformed page,
	// page size, status or currency. It is a 400: the caller can fix the query.
	fulfillmentErrorInvalid = "purchase_order_fulfillment_invalid"
	// fulfillmentErrorValueOutOfRange is the 422 for a figure outside the exact
	// JSON range. Failing closed is deliberate: a wrapped amount would be a
	// wrong drill-down that looks valid.
	fulfillmentErrorValueOutOfRange = "purchase_order_fulfillment_value_out_of_range"
)

func (s *Server) getPurchaseOrderFulfillment(context echo.Context) error {
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	profile, err := s.database.GetCompanyProfile(requestContext)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "read company profile"})
	}

	// The page and page size are parsed here because they are ints on the wire
	// and the store's normalizer only sees the values that parsed. A page below
	// one is refused rather than silently treated as the first page: the
	// caller can fix it, and a silent default hides a client bug.
	page := 1
	if raw := context.QueryParam("page"); raw != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || parsed < 1 {
			return fulfillmentError(context, http.StatusBadRequest, fulfillmentErrorInvalid,
				"page must be a positive integer")
		}
		page = parsed
	}
	pageSize := 0
	if raw := context.QueryParam(apicontract.PurchaseOrderFulfillmentPageSizeParam); raw != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fulfillmentError(context, http.StatusBadRequest, fulfillmentErrorInvalid,
				"page_size must be a whole number")
		}
		pageSize = parsed
	}

	raw := poreport.FulfillmentQuery{
		Currency: context.QueryParam("currency"),
		Status:   context.QueryParam("status"),
		Search:   context.QueryParam("search"),
		Page:     page,
		PageSize: pageSize,
	}
	if strings.TrimSpace(raw.Currency) == "" {
		raw.Currency = profile.Currency
	}
	query, err := poreport.NormalizeFulfillmentQuery(raw)
	if err != nil {
		return fulfillmentError(context, http.StatusBadRequest, fulfillmentErrorInvalid, err.Error())
	}
	info, ok := currency.Lookup(query.Currency)
	if !ok {
		return fulfillmentError(context, http.StatusBadRequest, fulfillmentErrorInvalid,
			"currency "+query.Currency+" is not in the supported catalogue")
	}
	// Store the catalogue's canonical spelling, so "usd" and "USD" name the
	// same partition and the response echoes the code the selector shows.
	query.Currency = info.Code

	fulfillment, err := s.database.ReportPurchaseOrderFulfillment(requestContext, query, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, poreport.ErrInvalid):
			return fulfillmentError(context, http.StatusBadRequest, fulfillmentErrorInvalid, err.Error())
		case errors.Is(err, poreport.ErrValueOutOfRange):
			return fulfillmentError(context, http.StatusUnprocessableEntity,
				fulfillmentErrorValueOutOfRange, err.Error())
		default:
			return context.JSON(http.StatusInternalServerError,
				map[string]string{"error": "read purchase order fulfillment"})
		}
	}
	return context.JSON(http.StatusOK, apicontract.PurchaseOrderFulfillmentPage(fulfillment))
}

// fulfillmentError writes the shared typed refusal body. The code is the stable
// machine identifier; error carries the human explanation.
func fulfillmentError(context echo.Context, status int, code, message string) error {
	return context.JSON(status, map[string]string{
		"error": message,
		"code":  code,
	})
}
