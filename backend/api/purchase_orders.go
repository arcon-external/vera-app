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
	"vera-app/backend/store"
)

// Issue #468: create, reopen and update a manual Purchase Order draft. The
// source is always one exact issued Quotation revision; the server revalidates
// it, copies the customer-facing snapshot inside the same transaction, and
// refuses a second active record for the same Quotation.

// createPurchaseOrderDraft creates the one active record for a Quotation.
func (s *Server) createPurchaseOrderDraft(context echo.Context) error {
	var request apicontract.PurchaseOrderCreateRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if request.QuotationID < 1 || request.RevisionNumber == nil || *request.RevisionNumber < 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation source"})
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
	poDate, err := parsePurchaseOrderDate(request.PODate, "poDate")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	received, err := parsePurchaseOrderDate(request.ReceivedDate, "receivedDate")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	requested, err := parsePurchaseOrderDate(request.RequestedDeliveryDate, "requestedDeliveryDate")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	user := requestUser(context)
	order, err := s.database.UpdatePurchaseOrderDraft(requestContext, store.PurchaseOrderUpdate{
		ID:                    id,
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
		UpdatedBy:             user.ID,
		Items:                 items,
		DiscountPercent:       request.DiscountPercent,
		AdjustmentMinor:       request.AdjustmentMinor,
		AdjustmentReason:      request.AdjustmentReason,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
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
	poDate, err := parsePurchaseOrderDate(request.PODate, "poDate")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	received, err := parsePurchaseOrderDate(request.ReceivedDate, "receivedDate")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	requested, err := parsePurchaseOrderDate(request.RequestedDeliveryDate, "requestedDeliveryDate")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	user := requestUser(context)
	order, err := s.database.FinalizePurchaseOrder(requestContext, store.PurchaseOrderFinalize{
		PurchaseOrderUpdate: store.PurchaseOrderUpdate{
			ID:                    id,
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
			UpdatedBy:             user.ID,
			Items:                 items,
			DiscountPercent:       request.DiscountPercent,
			AdjustmentMinor:       request.AdjustmentMinor,
			AdjustmentReason:      request.AdjustmentReason,
		},
		Decisions:           decisions,
		AcknowledgedMissing: request.AcknowledgedMissing,
		AcknowledgedExtra:   request.AcknowledgedExtra,
		DocumentConfirmed:   request.DocumentConfirmed,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrder(order))
}

func purchaseOrderError(context echo.Context, err error) error {
	switch {
	case errors.Is(err, store.ErrPurchaseOrderNotFound):
		return context.JSON(http.StatusNotFound, map[string]string{"error": "purchase order not found"})
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
	case errors.Is(err, store.ErrPurchaseOrderTotalMismatch):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error": err.Error(),
			"code":  "po_total_unexplained",
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
	case errors.Is(err, store.ErrPurchaseOrderInvalid):
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	default:
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "purchase order operation failed"})
	}
}

func mapPurchaseOrder(order store.PurchaseOrder) apicontract.PurchaseOrderRow {
	row := apicontract.PurchaseOrderRow{
		ID:                     order.ID,
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
		SavedBy:                order.SavedBy,
		CreatedAt:              order.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              order.UpdatedAt.Format(time.RFC3339),
	}
	if order.SavedAt != nil {
		formatted := order.SavedAt.Format(time.RFC3339)
		row.SavedAt = &formatted
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
