package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
)

func (s *Server) getPOReconciliation(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	session, err := s.database.GetPOReconciliation(requestContext, id)
	if err != nil {
		if errors.Is(err, store.ErrPOReconciliationNotFound) {
			return context.JSON(http.StatusNotFound, map[string]string{
				"error": "no reconciliation recorded",
				"code":  "po_reconciliation_not_found",
			})
		}
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, session)
}

func (s *Server) savePOReconciliation(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	var session store.POReconciliationSession
	if err := context.Bind(&session); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	session.PurchaseOrderID = id
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	updated, err := s.database.SavePOReconciliation(requestContext, session, requestUser(context).ID)
	if err != nil {
		if errors.Is(err, store.ErrPOReconciliationNotFound) {
			return context.JSON(http.StatusNotFound, map[string]string{
				"error": "no reconciliation recorded",
				"code":  "po_reconciliation_not_found",
			})
		}
		if errors.Is(err, store.ErrPOReconciliationReadOnly) {
			return context.JSON(http.StatusConflict, map[string]string{
				"error": err.Error(),
				"code":  "po_reconciliation_readonly",
			})
		}
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, updated)
}

// getPOReconciliationSessions is the #518 archive read: the current PO version
// plus every retained generation, in one consistent snapshot. A known record
// with no session is 200 with an empty list; an unknown or foreign record is
// 404 and leaks nothing.
func (s *Server) getPOReconciliationSessions(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	history, err := s.database.ListPOReconciliationSessions(requestContext, id)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, history)
}
