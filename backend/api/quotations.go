package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"vera-app/backend"
	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// decodeInput converts the generic JSON input into a QuotationInput via a
// JSON round-trip so the API and Wails paths share validation rules.
func decodeInput(raw map[string]any) (backend.QuotationInput, error) {
	var input backend.QuotationInput
	encoded, err := json.Marshal(raw)
	if err != nil {
		return input, fmt.Errorf("encode quotation input: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("decode quotation input: %w", err)
	}
	return input, nil
}

func (s *Server) prepareQuotation(user store.AuthUser, input backend.QuotationInput) (*backend.Quotation, error) {
	quotation := backend.NewQuotationForRequest(s.database, user, s.templates)
	if err := quotation.Prepare(input); err != nil {
		return nil, err
	}
	return quotation, nil
}

func requestUser(context echo.Context) store.AuthUser {
	user, _ := context.Get("user").(store.AuthUser)
	return user
}

func (s *Server) prepareEndpoint(context echo.Context) error {
	var raw map[string]any
	if err := context.Bind(&raw); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	input, err := decodeInput(raw)
	if err != nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	quotation, err := s.prepareQuotation(requestUser(context), input)
	if err != nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	customer, err := quotation.GetCustomerPreview()
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "build customer preview"})
	}
	internal, err := quotation.GetInternalBreakdown()
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "build internal breakdown"})
	}
	return context.JSON(http.StatusOK, map[string]any{"customer": customer, "internal": internal})
}

func (s *Server) exportEndpoint(context echo.Context) error {
	var envelope map[string]any
	if err := context.Bind(&envelope); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	// Accept both {"input": {...}, "quoteNumber": ..., "revision": ...} and
	// a bare input object for brand-new quotations.
	rawInput := envelope
	var quoteNumber string
	var revision int16
	if nested, ok := envelope["input"].(map[string]any); ok {
		rawInput = nested
		quoteNumber, _ = envelope["quoteNumber"].(string)
		if value, ok := envelope["revision"].(float64); ok {
			revision = int16(value)
		}
	}
	input, err := decodeInput(rawInput)
	if err != nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	quotation, err := s.prepareQuotation(requestUser(context), input)
	if err != nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}

	if quoteNumber != "" {
		quotation.SetQuoteNumber(quoteNumber, uint16(revision))
	} else {
		sequence, err := s.database.NextQuoteSequence(context.Request().Context())
		if err != nil {
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "allocate quote number"})
		}
		quotation.SetQuoteNumber(backend.FormatQuoteNumber(quotation.CustomerID(), sequence), 0)
	}

	snapshot, err := quotation.GetQuotationPreviewHTML()
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "render quotation snapshot"})
	}
	draft, err := quotation.DraftRevision(snapshot, nil, "")
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "build revision draft"})
	}
	record, err := s.database.SaveRevision(context.Request().Context(), draft)
	if err != nil {
		return context.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	}
	return context.JSON(http.StatusCreated, apicontract.ExportResponse{
		ID:          record.ID,
		QuoteNumber: record.QuoteNumber,
		Revision:    record.RevisionNumber,
		GrandTotal:  uint64(record.Totals.MarginalizedGrand),
	})
}

func (s *Server) listEndpoint(context echo.Context) error {
	rows, err := s.database.ListQuotations(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list quotations"})
	}
	summaries := make([]apicontract.QuotationSummaryRow, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, apicontract.QuotationSummaryRow{
			ID:             row.ID,
			QuoteNumber:    row.QuoteNumber,
			RevisionNumber: row.RevisionNumber,
			CustomerName:   row.CustomerName,
			Subject:        row.Subject,
			CreatedAt:      row.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			GrandTotal:     row.GrandTotal,
			DisplayGrand:   formatAmount(row.GrandTotal),
		})
	}
	return context.JSON(http.StatusOK, map[string]any{"quotations": summaries})
}

func (s *Server) detailEndpoint(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	var revision *int16
	if raw := context.QueryParam("revision"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 16)
		if err != nil || parsed < 0 {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid revision"})
		}
		value := int16(parsed)
		revision = &value
	}
	record, err := s.database.GetQuotation(context.Request().Context(), id, revision)
	if err != nil {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "quotation not found"})
	}
	hasPDF, err := s.database.HasDocumentPDF(context.Request().Context(), id, record.RevisionNumber)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "check quotation document"})
	}
	return context.JSON(http.StatusOK, mapDetail(record, hasPDF))
}

func (s *Server) revisionsEndpoint(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	record, err := s.database.GetQuotation(context.Request().Context(), id, nil)
	if err != nil {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "quotation not found"})
	}
	revisions := record.AvailableRevisions
	if revisions == nil {
		revisions = []int16{}
	}
	return context.JSON(http.StatusOK, map[string]any{"revisions": revisions})
}

func (s *Server) deleteEndpoint(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	if err := s.database.DeleteQuotation(context.Request().Context(), id); err != nil {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "quotation not found"})
	}
	return context.NoContent(http.StatusNoContent)
}

func (s *Server) attachDocumentEndpoint(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	revision, err := strconv.ParseInt(context.Param("rev"), 10, 16)
	if err != nil || revision < 0 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid revision"})
	}
	var request apicontract.AttachDocumentRequest
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	var pdfData []byte
	if request.PDFBase64 != "" {
		pdfData, err = base64.StdEncoding.DecodeString(request.PDFBase64)
		if err != nil {
			return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid pdfBase64"})
		}
	}
	if err := s.database.AttachDocument(context.Request().Context(), id, int16(revision), pdfData, request.PDFPath); err != nil {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "quotation revision not found"})
	}
	return context.JSON(http.StatusOK, map[string]any{"hasPdf": true})
}

func mapDetail(record store.QuotationRecord, hasPDF bool) apicontract.QuotationDetail {
	items := make([]apicontract.QuotationItemRow, 0, len(record.Items))
	for _, item := range record.Items {
		items = append(items, apicontract.QuotationItemRow{
			ItemNo:                   item.ItemNo,
			Description:              item.Description,
			Qty:                      item.Qty,
			UoM:                      item.UoM,
			OriginalUnitPrice:        item.UnitPrice,
			MarginEnabled:            item.MarginEnabled,
			MarginPercent:            int16(item.MarginPercent),
			MarginAmount:             item.MarginAmount,
			OriginalTotal:            item.OriginalTotal,
			MarginalizedUnitPrice:    item.MarginalizedUnitPrice,
			MarginalizedTotal:        item.MarginalizedTotal,
			DisplayOriginalUnit:      formatAmount(item.UnitPrice),
			DisplayOriginalTotal:     formatAmount(item.OriginalTotal),
			DisplayMarginalizedUnit:  formatAmount(item.MarginalizedUnitPrice),
			DisplayMarginalizedTotal: formatAmount(item.MarginalizedTotal),
			DisplayMarginAmount:      formatAmount(item.MarginAmount),
		})
	}
	return apicontract.QuotationDetail{
		ID:                          record.ID,
		QuoteNumber:                 record.QuoteNumber,
		Revision:                    record.RevisionNumber,
		AvailableRevisions:          record.AvailableRevisions,
		CustomerName:                record.CustomerName,
		Subject:                     record.Subject,
		RecipientName:               record.RecipientName,
		Incoterm:                    record.Incoterm,
		DiscountPercent:             record.DiscountPercent,
		TaxZone:                     record.TaxZone,
		TaxCategory:                 record.TaxCategory,
		OriginalSubtotal:            record.Totals.OriginalSubtotal,
		OriginalDiscount:            record.Totals.OriginalDiscount,
		OriginalGrand:               record.Totals.OriginalGrand,
		MarginalizedSubtotal:        record.Totals.MarginalizedSubtotal,
		MarginalizedDiscount:        record.Totals.MarginalizedDiscount,
		MarginalizedGrand:           record.Totals.MarginalizedGrand,
		GrandTotal:                  record.Totals.MarginalizedGrand,
		TaxAmount:                   record.Totals.TaxAmount,
		MarginalizedGrandAfterTax:   record.Totals.MarginalizedGrandAfterTax,
		ProfitAmount:                record.Totals.ProfitAmount,
		NettProfit:                  record.Totals.NettProfit,
		DisplayOriginalSubtotal:     formatAmount(record.Totals.OriginalSubtotal),
		DisplayOriginalDiscount:     formatAmount(record.Totals.OriginalDiscount),
		DisplayOriginalGrand:        formatAmount(record.Totals.OriginalGrand),
		DisplayMarginalizedSubtotal: formatAmount(record.Totals.MarginalizedSubtotal),
		DisplayMarginalizedDiscount: formatAmount(record.Totals.MarginalizedDiscount),
		DisplayMarginalizedGrand:    formatAmount(record.Totals.MarginalizedGrand),
		DisplayTax:                  formatAmount(record.Totals.TaxAmount),
		DisplayMarginalizedAfterTax: formatAmount(record.Totals.MarginalizedGrandAfterTax),
		DisplayProfit:               formatSignedAmount(record.Totals.ProfitAmount),
		DisplayNett:                 formatSignedAmount(record.Totals.NettProfit),
		Items:                       items,
		HTMLSnapshot:                record.HTMLSnapshot,
		HasPDF:                      hasPDF,
	}
}
