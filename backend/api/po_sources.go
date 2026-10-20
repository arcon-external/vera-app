package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #467: the read surface shared by both PO entry paths. Step one lists
// issued Quotations (15 per page, searchable); step two lists one Quotation's
// issued revisions; confirm revalidates the exact pair. Every route reuses the
// session middleware and reads the store, so pagination, search and the final
// check all see the same rows the PO link will be built against.

// listPOSources serves the selector's first step.
func (s *Server) listPOSources(context echo.Context) error {
	page := 1
	if raw := context.QueryParam("page"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid page"})
		}
		page = parsed
	}
	hideUsed := false
	if raw := context.QueryParam("hideUsed"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid hideUsed"})
		}
		hideUsed = parsed
	}

	quotations, total, err := s.database.ListPOSourceQuotations(
		context.Request().Context(), page, context.QueryParam("search"), hideUsed)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list quotation sources"})
	}

	rows := make([]apicontract.POSourceQuotationRow, 0, len(quotations))
	for _, quotation := range quotations {
		rows = append(rows, mapPOSourceQuotation(quotation))
	}
	totalPages := (total + store.POSourcePageSize - 1) / store.POSourcePageSize
	if totalPages < 1 {
		// One empty page, not "page 1 of 0": the selector still shows its
		// empty state with working pagination controls.
		totalPages = 1
	}
	return context.JSON(http.StatusOK, apicontract.POSourceListResponse{
		Quotations: rows,
		Page:       page,
		PageSize:   store.POSourcePageSize,
		Total:      total,
		TotalPages: totalPages,
	})
}

func mapPOSourceQuotation(quotation store.POSourceQuotation) apicontract.POSourceQuotationRow {
	return apicontract.POSourceQuotationRow{
		ID:                  quotation.ID,
		QuoteNumber:         quotation.QuoteNumber,
		Subject:             quotation.Subject,
		CustomerName:        quotation.CustomerName,
		LatestRevision:      quotation.LatestRevision,
		IssuedRevisionCount: quotation.IssuedRevisionCount,
		HasActivePO:         quotation.HasActivePO,
	}
}

// listPOSourceRevisions serves the selector's second step. Every stored
// revision is issued: unsaved drafts never enter quotation_revisions.
func (s *Server) listPOSourceRevisions(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	detail, err := s.database.ListPOSourceRevisions(context.Request().Context(), id)
	if errors.Is(err, store.ErrPOSourceNotFound) {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "quotation not found"})
	}
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list quotation revisions"})
	}
	revisions := make([]apicontract.POSourceRevisionRow, 0, len(detail.Revisions))
	for _, revision := range detail.Revisions {
		revisions = append(revisions, apicontract.POSourceRevisionRow{
			Revision: revision.Revision,
			IssuedAt: revision.IssuedAt.Format(time.RFC3339),
		})
	}
	return context.JSON(http.StatusOK, apicontract.POSourceRevisionsResponse{
		QuotationID:  detail.ID,
		QuoteNumber:  detail.QuoteNumber,
		Subject:      detail.Subject,
		CustomerName: detail.CustomerName,
		Revisions:    revisions,
	})
}

// validatePOSource is the selector's confirm gate. The exact pair must still
// exist and be issued, checked after the user has spent time choosing. A
// well-formed request always answers 200 with the outcome in the body, so the
// modal can show "no longer available" without treating it as a transport
// failure.
func (s *Server) validatePOSource(context echo.Context) error {
	var request apicontract.POSourceValidateRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	// Revision 0 is issued revision zero, not a missing value: an absent or
	// null field arrives as nil and is rejected rather than read as zero.
	if request.QuotationID < 1 || request.RevisionNumber == nil || *request.RevisionNumber < 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation source"})
	}
	revisionNumber := *request.RevisionNumber
	validation, err := s.database.ValidatePOSource(
		context.Request().Context(), request.QuotationID, revisionNumber)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "validate quotation source"})
	}
	response := apicontract.POSourceValidationResponse{
		Valid:          validation.Valid,
		Reason:         validation.Reason,
		QuotationID:    request.QuotationID,
		RevisionNumber: revisionNumber,
	}
	if validation.Quotation.ID > 0 {
		row := mapPOSourceQuotation(validation.Quotation)
		response.Quotation = &row
	}
	return context.JSON(http.StatusOK, response)
}
