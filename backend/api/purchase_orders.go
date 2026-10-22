package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/extract"
	"vera-app/backend/store"
)

// Issue #468: create, reopen and update a manual Purchase Order draft. The
// source is always one exact issued Quotation revision; the server revalidates
// it, copies the customer-facing snapshot inside the same transaction, and
// refuses a second active record for the same Quotation.

// createPurchaseOrderDraft creates either the one active record for a
// Quotation or, when no Quotation is named, a source-less blank draft (#496).
// A linked create keeps its exact source and occupancy guard; a blank create
// never occupies a Quotation and seeds the company currency.
func (s *Server) createPurchaseOrderDraft(context echo.Context) error {
	var request apicontract.PurchaseOrderCreateRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	received, err := parsePurchaseOrderDate(request.ReceivedDate, "receivedDate")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	user := requestUser(context)

	if request.QuotationID == 0 {
		if request.RevisionNumber != nil {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "a revision needs a quotation source"})
		}
		order, err := s.database.CreatePurchaseOrderBlankDraft(requestContext, store.PurchaseOrderBlankCreate{
			ReceivedDate:    received,
			TaxZone:         request.TaxZone,
			TaxType:         request.TaxType,
			ServiceCategory: request.ServiceCategory,
			TaxCategory:     request.TaxCategory,
			CreatedBy:       user.ID,
		})
		if err != nil {
			return purchaseOrderError(context, err)
		}
		return context.JSON(http.StatusCreated, mapPurchaseOrder(order))
	}
	if request.QuotationID < 0 || request.RevisionNumber == nil || *request.RevisionNumber < 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation source"})
	}
	order, err := s.database.CreatePurchaseOrderDraft(requestContext, store.PurchaseOrderCreate{
		QuotationID:    request.QuotationID,
		RevisionNumber: *request.RevisionNumber,
		ReceivedDate:   received,
		CreatedBy:      user.ID,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusCreated, mapPurchaseOrder(order))
}

// getPurchaseOrder reopens one draft by id inside the caller's organization.
func (s *Server) getPurchaseOrder(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	order, err := s.database.GetPurchaseOrder(requestContext, id)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
}

// updatePurchaseOrderDraft replaces the editable header and lines of a draft.
func (s *Server) updatePurchaseOrderDraft(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	var request apicontract.PurchaseOrderUpdateRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if protected := request.NamedExceptSourceBasis(); protected != "" {
		return purchaseOrderProtectedField(context, protected)
	}
	update, err := purchaseOrderUpdateFromRequest(request, requestUser(context).ID)
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	update.ID = id
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	order, err := s.database.UpdatePurchaseOrderDraft(requestContext, update)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
}

// correctPurchaseOrder replaces a saved record. The reason is required, the
// expected version is mandatory, and the payload can never name the source
// link, customer, currency, tax basis, identity or confirmation number.
func (s *Server) correctPurchaseOrder(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	var request apicontract.PurchaseOrderCorrectionRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if protected := request.Named(); protected != "" {
		return purchaseOrderProtectedField(context, protected)
	}
	update, err := purchaseOrderUpdateFromRequest(request.PurchaseOrderUpdateRequest, requestUser(context).ID)
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	update.ID = id
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	order, err := s.database.CorrectPurchaseOrder(requestContext, store.PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: update,
		Reason:              request.Reason,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
}

// reviewPurchaseOrderSource compares a saved record against a candidate
// revision without writing anything (#475). The admin gate is the route's; the
// review itself applies the same eligibility rules the save will apply, so an
// ineligible target is named before the admin spends time deciding values.
func (s *Server) reviewPurchaseOrderSource(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	var request apicontract.PurchaseOrderSourceReviewRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if request.TargetQuotationID < 1 || request.TargetRevisionNumber == nil || *request.TargetRevisionNumber < 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid target quotation source"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	review, err := s.database.ReviewPurchaseOrderSource(
		requestContext, id, request.TargetQuotationID, *request.TargetRevisionNumber)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrderReview(review))
}

// repairPurchaseOrderSource moves a saved record to another issued revision.
// The reason, the loaded version and the full #471 difference review are all
// required; the old and new link and the choices become history. Only an admin
// reaches this handler, and the store re-reads the actor's stored role.
func (s *Server) repairPurchaseOrderSource(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	var request apicontract.PurchaseOrderSourceRepairRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if protected := request.Named(); protected != "" {
		return purchaseOrderProtectedField(context, protected)
	}
	if request.TargetQuotationID < 1 || request.TargetRevisionNumber == nil || *request.TargetRevisionNumber < 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid target quotation source"})
	}
	update, err := purchaseOrderUpdateFromRequest(request.PurchaseOrderUpdateRequest, requestUser(context).ID)
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	update.ID = id
	decisions := make([]store.PurchaseOrderDecision, 0, len(request.Decisions))
	for _, decision := range request.Decisions {
		decisions = append(decisions, store.PurchaseOrderDecision{
			Key:    decision.Key,
			Choice: decision.Choice,
		})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	order, err := s.database.RepairPurchaseOrderSource(requestContext, store.PurchaseOrderSourceRepairInput{
		PurchaseOrderUpdate: update,
		Reason:              request.Reason,
		QuotationID:         request.TargetQuotationID,
		RevisionNumber:      *request.TargetRevisionNumber,
		Decisions:           decisions,
		AcknowledgedMissing: request.AcknowledgedMissing,
		AcknowledgedExtra:   request.AcknowledgedExtra,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
}

// getPurchaseOrderHistory returns the immutable snapshots of one record,
// newest first.
func (s *Server) getPurchaseOrderHistory(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	history, err := s.database.GetPurchaseOrderHistory(requestContext, id)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	rows := make([]apicontract.PurchaseOrderCorrectionRow, 0, len(history))
	for _, entry := range history {
		rows = append(rows, mapPurchaseOrderCorrection(entry))
	}
	return context.JSON(http.StatusOK, map[string]any{"history": rows})
}

// getPurchaseOrderReview opens one record beside the exact source revision it
// is checked against. The final-save review reads this, not a cached copy.
func (s *Server) getPurchaseOrderReview(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	review, err := s.database.GetPurchaseOrderReview(requestContext, id)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrderReview(review))
}

// finalizePurchaseOrder is the one-way draft -> saved transition. The server
// revalidates the source, the minimum fields, every difference decision and
// the printed-total explanation inside one transaction.
func (s *Server) finalizePurchaseOrder(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	var request apicontract.PurchaseOrderFinalizeRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if protected := request.Named(); protected != "" {
		return purchaseOrderProtectedField(context, protected)
	}
	user := requestUser(context)
	update, err := purchaseOrderUpdateFromRequest(request.PurchaseOrderUpdateRequest, user.ID)
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	update.ID = id
	decisions := make([]store.PurchaseOrderDecision, 0, len(request.Decisions))
	for _, decision := range request.Decisions {
		decisions = append(decisions, store.PurchaseOrderDecision{
			Key:    decision.Key,
			Choice: decision.Choice,
		})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	order, err := s.database.FinalizePurchaseOrder(requestContext, store.PurchaseOrderFinalize{
		PurchaseOrderUpdate: update,
		Decisions:           decisions,
		AcknowledgedMissing: request.AcknowledgedMissing,
		AcknowledgedExtra:   request.AcknowledgedExtra,
		DocumentConfirmed:   request.DocumentConfirmed,
		// #474: only the checkbox travels; the number itself is allocated by
		// the server inside the final-save transaction.
		AssignConfirmationNumber: request.AssignConfirmationNumber,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
}

// assignPurchaseOrderConfirmation issues the optional customer-system
// reference on a saved record. It is idempotent: an assigned record returns its
// existing number, and a draft is refused. No request body carries a number.
func (s *Server) assignPurchaseOrderConfirmation(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	user := requestUser(context)
	order, err := s.database.AssignPurchaseOrderConfirmationNumber(requestContext, id, user.ID)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
}

func purchaseOrderError(context echo.Context, err error) error {
	var versionConflict *store.PurchaseOrderVersionConflictError
	if errors.As(err, &versionConflict) {
		return context.JSON(http.StatusConflict, apicontract.PurchaseOrderConflictResponse{
			Error:          versionConflict.Error(),
			Code:           "po_version_conflict",
			CurrentState:   versionConflict.CurrentState,
			CurrentVersion: versionConflict.CurrentVersion,
			UpdatedBy:      versionConflict.UpdatedBy,
			UpdatedByName:  versionConflict.UpdatedByName,
			UpdatedAt:      versionConflict.UpdatedAt.Format(time.RFC3339),
		})
	}
	switch {
	case errors.Is(err, store.ErrPurchaseOrderNotFound):
		return context.JSON(http.StatusNotFound, map[string]string{"error": "purchase order not found"})
	case errors.Is(err, store.ErrPurchaseOrderVersionRequired):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": "reload the purchase order before saving; the version it was loaded at is required",
			"code":  "po_version_required",
		})
	case errors.Is(err, store.ErrPurchaseOrderRepairForbidden):
		return context.JSON(http.StatusForbidden, map[string]string{
			"error": "repairing a purchase order source link requires an administrator",
			"code":  "po_repair_forbidden",
		})
	case errors.Is(err, store.ErrPurchaseOrderRepairReasonRequired):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": "a source repair needs a reason",
			"code":  "po_repair_reason_required",
		})
	case errors.Is(err, store.ErrPurchaseOrderSourceUnchanged):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "po_source_unchanged",
		})
	case errors.Is(err, store.ErrPurchaseOrderTargetOccupied):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "po_target_occupied",
		})
	case errors.Is(err, store.ErrPurchaseOrderConfirmationCustomerLocked):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "po_confirmation_customer_locked",
		})
	case errors.Is(err, store.ErrPurchaseOrderCorrectionReasonRequired):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": "a correction needs a reason",
			"code":  "po_correction_reason_required",
		})
	case errors.Is(err, store.ErrPurchaseOrderExists):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": "this quotation already has a purchase order",
			"code":  "po_already_exists",
		})
	case errors.Is(err, store.ErrPurchaseOrderNotDraft):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": "only a draft can be edited",
			"code":  "po_not_draft",
		})
	case errors.Is(err, store.ErrPurchaseOrderSourceNotFound):
		return context.JSON(http.StatusNotFound, map[string]string{"error": "quotation revision not found"})
	case errors.Is(err, store.ErrPurchaseOrderSourceMissing):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "po_source_missing",
		})
	case errors.Is(err, store.ErrPurchaseOrderSourceIncomplete):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_source_incomplete",
		})
	case errors.Is(err, store.ErrPurchaseOrderSourceOutOfRange):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_source_out_of_range",
		})
	case errors.Is(err, store.ErrPurchaseOrderBadReference):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrPurchaseOrderIncomplete):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_incomplete",
		})
	case errors.Is(err, store.ErrPurchaseOrderUnresolvedDifference):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_differences_unresolved",
		})
	case errors.Is(err, store.ErrPurchaseOrderPDFConfirmationRequired):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_pdf_confirmation_required",
		})
	case errors.Is(err, store.ErrPurchaseOrderNumberTaken):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "po_number_taken",
		})
	case errors.Is(err, store.ErrPurchaseOrderSourceCurrencyMismatch):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_currency_mismatch",
		})
	case errors.Is(err, store.ErrPurchaseOrderSourceMismatch):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_source_mismatch",
		})
	case errors.Is(err, store.ErrPurchaseOrderNotSaved):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": "save the purchase order first; this action needs a saved record",
			"code":  "po_not_saved",
		})
	case errors.Is(err, store.ErrPOConfirmationExhausted):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "po_confirmation_exhausted",
		})
	case errors.Is(err, store.ErrPurchaseOrderInvalid):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrPurchaseOrderDocumentNotFound):
		return context.JSON(http.StatusNotFound, map[string]string{
			"error": "customer PO document not found",
			"code":  "po_document_not_found",
		})
	case errors.Is(err, store.ErrPurchaseOrderDocumentTooLarge):
		return context.JSON(http.StatusRequestEntityTooLarge, map[string]string{
			"error": err.Error(),
			"code":  "po_document_too_large",
		})
	case errors.Is(err, store.ErrPurchaseOrderDocumentEncrypted):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_document_encrypted",
		})
	case errors.Is(err, store.ErrPurchaseOrderDocumentInvalid):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_document_invalid",
		})
	case errors.Is(err, store.ErrPurchaseOrderDocumentRemovalNotConfirmed):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": "confirm removing the customer PO PDF",
			"code":  "po_document_removal_unconfirmed",
		})
	case errors.Is(err, store.ErrPurchaseOrderExtractionDocumentRequired):
		return context.JSON(http.StatusConflict, map[string]string{
			"error": err.Error(),
			"code":  "po_extraction_document_required",
		})
	case errors.Is(err, store.ErrPurchaseOrderExtractionNotFound):
		return context.JSON(http.StatusNotFound, map[string]string{
			"error": "purchase order extraction not found",
			"code":  "po_extraction_not_found",
		})
	case errors.Is(err, extract.ErrExtractorUnavailable):
		return context.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": err.Error(),
			"code":  "po_extraction_unavailable",
		})
	default:
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "purchase order operation failed"})
	}
}

func mapPurchaseOrder(order store.PurchaseOrder) apicontract.PurchaseOrderRow {
	row := apicontract.PurchaseOrderRow{
		ID:                     order.ID,
		Version:                order.Version,
		UUID:                   order.UUID,
		State:                  order.State,
		QuotationID:            order.QuotationID,
		SourceRevision:         order.SourceRevision,
		QuoteNumber:            order.QuoteNumber,
		CustomerID:             order.CustomerID,
		CustomerName:           order.CustomerName,
		BillingAddress:         order.BillingAddress,
		DeliveryAddress:        order.DeliveryAddress,
		ShipToCode:             order.ShipToCode,
		ContactEmail:           order.ContactEmail,
		ContactPhone:           order.ContactPhone,
		PONumber:               order.PONumber,
		PODate:                 formatPurchaseOrderDate(order.PODate),
		ReceivedDate:           order.ReceivedDate.Format("2006-01-02"),
		RequestedDeliveryDate:  formatPurchaseOrderDate(order.RequestedDeliveryDate),
		PaymentTerms:           order.PaymentTerms,
		Incoterms:              order.Incoterms,
		Currency:               order.Currency,
		TaxZone:                order.TaxZone,
		TaxType:                order.TaxType,
		ServiceCategory:        order.ServiceCategory,
		TaxCategory:            order.TaxCategory,
		DiscountPercent:        order.DiscountPercent,
		SubtotalMinor:          order.SubtotalMinor,
		DiscountMinor:          order.DiscountMinor,
		PpnBps:                 order.PpnBps,
		PpnMinor:               order.PpnMinor,
		AdjustmentMinor:        order.AdjustmentMinor,
		AdjustmentReason:       order.AdjustmentReason,
		GrandTotalMinor:        order.GrandTotalMinor,
		PrintedSubtotalMinor:   order.PrintedSubtotalMinor,
		PrintedDiscountMinor:   order.PrintedDiscountMinor,
		PrintedPpnMinor:        order.PrintedPpnMinor,
		PrintedGrandTotalMinor: order.PrintedGrandTotalMinor,
		PdfAbsentConfirmed:     order.PDFAbsentConfirmed,
		SourceMissing:          order.SourceMissing,
		SourceState:            order.SourceState,
		PrintedTotalsKnown:     order.PrintedTotalsKnown,
		SavedBy:                order.SavedBy,
		ConfirmationNumber:     order.ConfirmationNumber,
		ConfirmationWarn:       order.ConfirmationWarn,
		CreatedAt:              order.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              order.UpdatedAt.Format(time.RFC3339),
	}
	if order.SavedAt != nil {
		formatted := order.SavedAt.Format(time.RFC3339)
		row.SavedAt = &formatted
	}
	if order.ConfirmationAssignedAt != nil {
		formatted := order.ConfirmationAssignedAt.Format(time.RFC3339)
		row.ConfirmationAssignedAt = &formatted
	}
	row.Items = make([]apicontract.PurchaseOrderItemRow, 0, len(order.Items))
	for _, item := range order.Items {
		row.Items = append(row.Items, apicontract.PurchaseOrderItemRow{
			LineNumber:      item.LineNumber,
			Description:     item.Description,
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
			GrossMinor:      item.GrossMinor,
			DiscountMinor:   item.DiscountMinor,
			NetMinor:        item.NetMinor,
		})
	}
	return row
}

func parsePurchaseOrderDate(raw, field string) (*time.Time, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, nil
	}
	value, err := time.Parse("2006-01-02", text)
	if err != nil {
		return nil, fmt.Errorf("%s must be a date like 2026-10-20", field)
	}
	return &value, nil
}

// purchaseOrderProtectedField refuses a request that carried a protected value.
// The name is repeated so the client knows which field it must not send; the
// value itself is never echoed.
func purchaseOrderProtectedField(context echo.Context, field string) error {
	return context.JSON(http.StatusUnprocessableEntity, map[string]string{
		"error": "the field " + field + " is protected and cannot be changed by a purchase order save or correction",
		"code":  "po_protected_field",
	})
}

// purchaseOrderUpdateFromRequest converts the shared editable payload. It only
// fails on a malformed date; every other rule belongs to the store so the API
// and the desktop cannot disagree.
func purchaseOrderUpdateFromRequest(request apicontract.PurchaseOrderUpdateRequest, updatedBy int64) (store.PurchaseOrderUpdate, error) {
	poDate, err := parsePurchaseOrderDate(request.PODate, "poDate")
	if err != nil {
		return store.PurchaseOrderUpdate{}, err
	}
	received, err := parsePurchaseOrderDate(request.ReceivedDate, "receivedDate")
	if err != nil {
		return store.PurchaseOrderUpdate{}, err
	}
	requested, err := parsePurchaseOrderDate(request.RequestedDeliveryDate, "requestedDeliveryDate")
	if err != nil {
		return store.PurchaseOrderUpdate{}, err
	}
	items := make([]store.PurchaseOrderItem, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, store.PurchaseOrderItem{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
		})
	}
	return store.PurchaseOrderUpdate{
		BillingAddress:        request.BillingAddress,
		DeliveryAddress:       request.DeliveryAddress,
		ShipToCode:            request.ShipToCode,
		ContactEmail:          request.ContactEmail,
		ContactPhone:          request.ContactPhone,
		PONumber:              request.PONumber,
		PODate:                poDate,
		ReceivedDate:          received,
		RequestedDeliveryDate: requested,
		PaymentTerms:          request.PaymentTerms,
		Incoterms:             request.Incoterms,
		Currency:              stringOrEmpty(request.Currency),
		TaxZone:               stringOrEmpty(request.TaxZone),
		TaxType:               stringOrEmpty(request.TaxType),
		ServiceCategory:       stringOrEmpty(request.ServiceCategory),
		TaxCategory:           stringOrEmpty(request.TaxCategory),
		UpdatedBy:             updatedBy,
		Items:                 items,
		DiscountPercent:       request.DiscountPercent,
		AdjustmentMinor:       request.AdjustmentMinor,
		AdjustmentReason:      request.AdjustmentReason,
		ExpectedVersion:       request.ExpectedVersion,
		Overwrite:             request.Overwrite,
	}, nil
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// mapPurchaseOrderCorrection exposes one retained snapshot. Nothing here reads
// the live record: the history is the values as they stood.
func mapPurchaseOrderCorrection(entry store.PurchaseOrderCorrection) apicontract.PurchaseOrderCorrectionRow {
	row := apicontract.PurchaseOrderCorrectionRow{
		ID:                    entry.ID,
		UUID:                  entry.UUID,
		Kind:                  entry.Kind,
		State:                 entry.State,
		Reason:                entry.Reason,
		SourceMissing:         entry.SourceMissing,
		ActorID:               entry.ActorID,
		ActorName:             entry.ActorName,
		CreatedAt:             entry.CreatedAt.Format(time.RFC3339),
		PreviousVersion:       entry.PreviousVersion,
		ResultingVersion:      entry.ResultingVersion,
		QuotationID:           entry.QuotationID,
		SourceRevision:        entry.SourceRevision,
		QuoteNumber:           entry.QuoteNumber,
		CustomerID:            entry.CustomerID,
		CustomerName:          entry.CustomerName,
		Currency:              entry.Currency,
		TaxZone:               entry.TaxZone,
		TaxCategory:           entry.TaxCategory,
		BillingAddress:        entry.BillingAddress,
		DeliveryAddress:       entry.DeliveryAddress,
		ShipToCode:            entry.ShipToCode,
		ContactEmail:          entry.ContactEmail,
		ContactPhone:          entry.ContactPhone,
		PONumber:              entry.PONumber,
		PODate:                formatPurchaseOrderDate(entry.PODate),
		ReceivedDate:          entry.ReceivedDate.Format("2006-01-02"),
		RequestedDeliveryDate: formatPurchaseOrderDate(entry.RequestedDeliveryDate),
		PaymentTerms:          entry.PaymentTerms,
		Incoterms:             entry.Incoterms,
		DiscountPercent:       entry.DiscountPercent,
		SubtotalMinor:         entry.SubtotalMinor,
		DiscountMinor:         entry.DiscountMinor,
		PpnBps:                entry.PpnBps,
		PpnMinor:              entry.PpnMinor,
		AdjustmentMinor:       entry.AdjustmentMinor,
		AdjustmentReason:      entry.AdjustmentReason,
		GrandTotalMinor:       entry.GrandTotalMinor,
		Items:                 make([]apicontract.PurchaseOrderCorrectionItemRow, 0, len(entry.Items)),
	}
	if entry.Attachment != nil {
		row.Attachment = &apicontract.PurchaseOrderCorrectionAttachmentRow{
			Version:  entry.Attachment.Version,
			SHA256:   entry.Attachment.SHA256,
			Filename: entry.Attachment.Filename,
		}
	}
	if entry.Repair != nil {
		repair := &apicontract.PurchaseOrderSourceRepairRow{
			PreviousQuotationID:    entry.Repair.PreviousQuotationID,
			PreviousSourceRevision: entry.Repair.PreviousSourceRevision,
			PreviousQuoteNumber:    entry.Repair.PreviousQuoteNumber,
			PreviousCustomerID:     entry.Repair.PreviousCustomerID,
			PreviousCustomerName:   entry.Repair.PreviousCustomerName,
			NewQuotationID:         entry.Repair.NewQuotationID,
			NewSourceRevision:      entry.Repair.NewSourceRevision,
			NewQuoteNumber:         entry.Repair.NewQuoteNumber,
			NewCustomerID:          entry.Repair.NewCustomerID,
			NewCustomerName:        entry.Repair.NewCustomerName,
			Decisions:              make([]apicontract.PurchaseOrderSourceRepairDecisionRow, 0, len(entry.Repair.Decisions)),
		}
		for _, decision := range entry.Repair.Decisions {
			repair.Decisions = append(repair.Decisions, apicontract.PurchaseOrderSourceRepairDecisionRow{
				Key:    decision.Key,
				Choice: decision.Choice,
			})
		}
		row.Repair = repair
	}
	for _, item := range entry.Items {
		row.Items = append(row.Items, apicontract.PurchaseOrderCorrectionItemRow{
			QuotationItemNo: item.QuotationItemNo,
			QuotationSubNo:  item.QuotationSubNo,
			LineNumber:      item.LineNumber,
			Description:     item.Description,
			QuantityScaled:  item.QuantityScaled,
			UoM:             item.UoM,
			UnitPriceMinor:  item.UnitPriceMinor,
			DiscountEnabled: item.DiscountEnabled,
			DiscountPercent: item.DiscountPercent,
			GrossMinor:      item.GrossMinor,
			DiscountMinor:   item.DiscountMinor,
			NetMinor:        item.NetMinor,
		})
	}
	return row
}

func formatPurchaseOrderDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02")
	return &formatted
}

// mapPurchaseOrderReview exposes the exact source beside the stored record. The
// source is the customer-facing composition, so internal cost, margin, PPH22
// and profit never cross this boundary (parent story 42).
func mapPurchaseOrderReview(review store.PurchaseOrderReview) apicontract.PurchaseOrderReviewRow {
	source := review.Source
	row := apicontract.PurchaseOrderReviewRow{
		Order:         mapPurchaseOrder(review.Order),
		DocumentState: review.DocumentState,
		Source: apicontract.PurchaseOrderSourceRow{
			QuotationID:            source.QuotationID,
			RevisionNumber:         source.RevisionNumber,
			QuoteNumber:            source.QuoteNumber,
			CustomerID:             source.CustomerID,
			CustomerName:           source.CustomerName,
			Currency:               source.Currency,
			PaymentTerms:           source.PaymentTerms,
			Incoterms:              source.Incoterms,
			DiscountPercent:        source.DiscountPercent,
			PrintedSubtotalMinor:   source.PrintedSubtotalMinor,
			PrintedDiscountMinor:   source.PrintedDiscountMinor,
			PrintedPpnMinor:        source.PrintedPpnMinor,
			PrintedGrandTotalMinor: source.PrintedGrandTotalMinor,
			Lines:                  make([]apicontract.PurchaseOrderSourceLineRow, 0, len(source.Lines)),
		},
	}
	for _, line := range source.Lines {
		row.Source.Lines = append(row.Source.Lines, apicontract.PurchaseOrderSourceLineRow{
			QuotationItemNo: line.QuotationItemNo,
			QuotationSubNo:  line.QuotationSubNo,
			LineLabel:       line.Label(),
			Description:     line.Description,
			UoM:             line.UoM,
			QuantityScaled:  line.QuantityScaled,
			UnitPriceMinor:  line.UnitPriceMinor,
			DiscountEnabled: line.DiscountEnabled,
			DiscountPercent: line.DiscountPercent,
		})
	}
	return row
}
