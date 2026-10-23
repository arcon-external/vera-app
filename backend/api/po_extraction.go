package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #477: start and read the text-PDF extraction run.
//
// Starting is a write of intent only: it creates a queued run and returns. The
// worker is asynchronous so manual entry never waits for Poppler, and the
// result is evidence: no purchase-order field is written here, at success, or
// by any automatic path. The client applies chosen proposals through its own
// explicit action.

func (s *Server) startPurchaseOrderExtraction(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	user := requestUser(context)
	run, err := s.extractions.Start(requestContext, id, user.ID)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	state := store.PurchaseOrderExtractionState{Run: &run}
	if document, documentErr := s.database.GetActivePurchaseOrderDocument(requestContext, id); documentErr == nil {
		state.Document = &document
	}
	// 202: the run is accepted, not finished. A fast engine may already have a
	// result by the time the client reads, which the state below reports
	// truthfully.
	return context.JSON(http.StatusAccepted, mapPurchaseOrderExtractionState(state))
}

// getPurchaseOrderExtractionAvailability answers whether this deployment can
// extract at all. It is authenticated but organization-independent: the
// preflight describes the server, not a tenant. It writes nothing and is the
// typed signal behind the desktop's disabled Extract and banner.
func (s *Server) getPurchaseOrderExtractionAvailability(context echo.Context) error {
	available := s.extractions != nil && s.extractions.Available(context.Request().Context())
	return context.JSON(http.StatusOK, apicontract.PurchaseOrderExtractionAvailabilityRow{
		Available: available,
	})
}

func (s *Server) getPurchaseOrderExtraction(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	state, err := s.extractions.Latest(requestContext, id)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrderExtractionState(state))
}

func mapPurchaseOrderExtractionState(state store.PurchaseOrderExtractionState) apicontract.PurchaseOrderExtractionStateRow {
	row := apicontract.PurchaseOrderExtractionStateRow{}
	if state.Document != nil {
		row.Document = &apicontract.PurchaseOrderExtractionDocumentRow{
			ID:      state.Document.ID,
			Version: state.Document.Version,
			SHA256:  state.Document.SHA256,
		}
	}
	if state.Run != nil {
		run := mapPurchaseOrderExtraction(*state.Run)
		row.Run = &run
	}
	return row
}

func mapPurchaseOrderExtraction(run store.PurchaseOrderExtraction) apicontract.PurchaseOrderExtractionRow {
	row := apicontract.PurchaseOrderExtractionRow{
		ID:              run.ID,
		UUID:            run.UUID,
		PurchaseOrderID: run.PurchaseOrderID,
		DocumentID:      run.DocumentID,
		DocumentVersion: run.DocumentVersion,
		DocumentSHA256:  run.DocumentSHA256,
		Status:          run.Status,
		Engine:          run.Engine,
		EngineVersion:   run.EngineVersion,
		PageCount:       run.PageCount,
		Result:          run.Result,
		ErrorCode:       run.ErrorCode,
		ErrorMessage:    run.ErrorMessage,
		RequestedBy:     run.RequestedBy,
		CreatedAt:       run.CreatedAt.Format(time.RFC3339),
		DurationMS:      run.DurationMS,
	}
	if run.StartedAt != nil {
		formatted := run.StartedAt.Format(time.RFC3339)
		row.StartedAt = &formatted
	}
	if run.FinishedAt != nil {
		formatted := run.FinishedAt.Format(time.RFC3339)
		row.FinishedAt = &formatted
	}
	return row
}
