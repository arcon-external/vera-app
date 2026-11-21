package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/currency"
	"vera-app/backend/poreport"
)

// Issue #629: the purchase-order performance report. One authenticated read
// returns the complete snapshot the Reports screen and the later PDF render:
// the filter, the population facets, actual totals, continuous calendar
// buckets, the category breakdown, aligned category series, the monthly and
// yearly projections, the current-period projection, and any disclosure notes.
//
// The endpoint is organization-scoped through the same context the PO archive
// uses and adds no role restriction: staff who record the orders may read what
// they made. Currency is a partition, so an omitted currency defaults to the
// company's own and an unknown code is refused rather than silently treated as
// the default.

const (
	// reportErrorInvalid is the stable refusal code for a malformed date,
	// bucket, category or currency. It is a 400: the caller can fix the query.
	reportErrorInvalid = "report_invalid"
	// reportErrorRangeTooLarge is the 422 for a well-formed range above the
	// approved exact-period cap. The message names the cap; the server never
	// silently shortens the range.
	reportErrorRangeTooLarge = "report_range_too_large"
	// reportErrorValueOutOfRange is the 422 for actual aggregates outside the
	// exact JSON range. Failing closed is deliberate: a wrapped figure would be
	// a wrong report that looks valid.
	reportErrorValueOutOfRange = "report_value_out_of_range"
	// reportErrorMonthSummary is the 500 for a current-month readout the server
	// cannot derive from the snapshot it already read. The filter was validated
	// before the read, so no request field can fix it; the stable code still
	// separates it from an untyped internal failure.
	reportErrorMonthSummary = "report_month_summary_failed"
	// reportErrorLegacyAdjustment is the note code attached when a filtered PO
	// still carries a retired nonzero whole-order discount or adjustment.
	reportErrorLegacyAdjustment = "legacy_order_adjustment"
)

func (s *Server) getPurchaseOrderReport(context echo.Context) error {
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	profile, err := s.database.GetCompanyProfile(requestContext)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "read company profile"})
	}

	// Currency is the only filter with a company default, and it must be
	// resolved before normalization because the domain refuses an empty code.
	raw := poreport.Query{
		From:     context.QueryParam("from"),
		To:       context.QueryParam("to"),
		Bucket:   context.QueryParam("bucket"),
		Category: context.QueryParam("category"),
		Currency: context.QueryParam("currency"),
	}
	if strings.TrimSpace(raw.Currency) == "" {
		raw.Currency = profile.Currency
	}
	query, err := poreport.NormalizeQuery(raw)
	if err != nil {
		if errors.Is(err, poreport.ErrRangeTooLarge) {
			return reportError(context, http.StatusUnprocessableEntity, reportErrorRangeTooLarge, err)
		}
		return reportError(context, http.StatusBadRequest, reportErrorInvalid, err)
	}
	info, ok := currency.Lookup(query.Currency)
	if !ok {
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "currency " + query.Currency + " is not in the supported catalogue",
			"code":  reportErrorInvalid,
		})
	}
	// Store the catalogue's canonical spelling, so "usd" and "USD" name the
	// same partition and the response echoes the code the selector shows.
	query.Currency = info.Code

	now := time.Now()
	snapshot, err := s.database.ReportPurchaseOrders(requestContext, query, now)
	if err != nil {
		if errors.Is(err, poreport.ErrValueOutOfRange) {
			return reportError(context, http.StatusUnprocessableEntity, reportErrorValueOutOfRange, err)
		}
		if errors.Is(err, poreport.ErrRangeTooLarge) {
			// Defensive: the query above already passed normalization.
			return reportError(context, http.StatusUnprocessableEntity, reportErrorRangeTooLarge, err)
		}
		if errors.Is(err, poreport.ErrInvalid) {
			return reportError(context, http.StatusBadRequest, reportErrorInvalid, err)
		}
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "read purchase order report"})
	}

	notes := make([]poreport.Note, 0, 1)
	if snapshot.LegacyAdjustmentPoCount > 0 {
		notes = append(notes, poreport.Note{
			Code: reportErrorLegacyAdjustment,
			Text: "Some purchase orders in this range carry a retired whole-order discount or adjustment. " +
				"The amount includes it, but it is not supplier cost and no margin is inferred from it; " +
				"the uncosted figure is the part of the amount without a resolved source cost.",
		})
	}
	var projection poreport.Projection
	if snapshot.HistoryOutOfRange {
		// The empty actual population is the reason staff see first: there is
		// nothing to forecast from in the selected range. The history overflow
		// still determines the basis and suppresses the series, but it must not
		// replace the empty-range explanation. A nonempty range keeps
		// value_out_of_range because the corrupt history is then the only thing
		// preventing a forecast.
		reason := poreport.ReasonValueOutOfRange
		if snapshot.Totals.PoCount == 0 {
			reason = poreport.ReasonNoDataInRange
		}
		projection = poreport.UnavailableProjection(reason, snapshot.HistoryBasis)
	} else {
		projection = poreport.Project(
			snapshot.History, query, snapshot.AsOf, snapshot.Totals.PoCount)
	}
	currentProjection := poreport.ProjectCurrent(
		snapshot.History,
		snapshot.Daily,
		query,
		snapshot.AsOf,
		snapshot.HistoryOutOfRange,
		snapshot.DailyOutOfRange,
	)
	monthSummary, err := poreport.BuildMonthSummary(
		snapshot.Periods,
		snapshot.Daily,
		currentProjection,
		query,
		snapshot.AsOf,
		snapshot.DailyOutOfRange,
	)
	if err != nil {
		return reportMonthSummaryError(context, err)
	}
	report := poreport.Report{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		AsOf:        snapshot.AsOf.Format(time.DateOnly),
		Filter:      query,
		Company: poreport.Company{
			Name:    profile.LegalName,
			Address: profile.Address,
		},
		Population: poreport.Population{
			AvailableCurrencies:  snapshot.AvailableCurrencies,
			HasUnassigned:        snapshot.HasUnassigned,
			SourceMissingPoCount: snapshot.SourceMissingPoCount,
		},
		Totals:            snapshot.Totals,
		Periods:           snapshot.Periods,
		Categories:        snapshot.Categories,
		CategorySeries:    snapshot.CategorySeries,
		Projection:        projection,
		CurrentProjection: currentProjection,
		MonthSummary:      monthSummary,
		Notes:             notes,
	}
	return context.JSON(http.StatusOK, apicontract.PurchaseOrderReport(report))
}

// reportError writes the shared typed refusal body. The code is the stable
// machine identifier; error carries the human explanation and the offending
// value.
func reportError(context echo.Context, status int, code string, err error) error {
	return context.JSON(status, map[string]string{
		"error": err.Error(),
		"code":  code,
	})
}

// reportMonthSummaryError writes the typed refusal for a failed current-month
// readout. A value outside the exact range is the same 422 every other
// aggregate failure uses; any other failure is server-side, so it keeps the
// 500 while still carrying a stable code instead of a bare error body.
func reportMonthSummaryError(context echo.Context, err error) error {
	if errors.Is(err, poreport.ErrValueOutOfRange) {
		return reportError(context, http.StatusUnprocessableEntity, reportErrorValueOutOfRange, err)
	}
	return reportError(context, http.StatusInternalServerError, reportErrorMonthSummary, err)
}
