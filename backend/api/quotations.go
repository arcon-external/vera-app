package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"vera-app/backend"
	"vera-app/backend/apicontract"
	"vera-app/backend/currency"
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

func (s *Server) prepareQuotation(ctx context.Context, user store.AuthUser, input backend.QuotationInput) (*backend.Quotation, error) {
	quotation := backend.NewQuotationForRequest(s.database, user, s.templates)
	if err := quotation.Prepare(input); err != nil {
		return nil, err
	}
	// An uploaded logo replaces the baked asset on every server render, so
	// previews, snapshots, and PDFs agree. Failures fall through to the
	// baked asset rather than failing the quotation.
	if profile, err := s.database.GetCompanyProfile(ctx); err == nil && profile.HasLogo {
		quotation.Data.LogoBase64 = base64.StdEncoding.EncodeToString(profile.LogoBytes)
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
	quotation, err := s.prepareQuotation(context.Request().Context(), requestUser(context), input)
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
	var sourceDraftID int64
	if nested, ok := envelope["input"].(map[string]any); ok {
		rawInput = nested
		quoteNumber, _ = envelope["quoteNumber"].(string)
		if value, ok := envelope["revision"].(float64); ok {
			revision = int16(value)
		}
		// The draft this export came from, if any. It is consumed only after
		// the revision is safely stored, so a failed export always leaves the
		// draft intact and nothing is lost to a part-finished request.
		if value, ok := envelope["draftId"].(float64); ok {
			sourceDraftID = int64(value)
		}
	}
	input, err := decodeInput(rawInput)
	if err != nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	quotation, err := s.prepareQuotation(context.Request().Context(), requestUser(context), input)
	if err != nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}

	sequenceWarn := false
	if quoteNumber != "" {
		quotation.SetQuoteNumber(quoteNumber, uint16(revision))
	} else {
		yy := store.CurrentYY()
		sequence, warn, err := s.database.NextQuoteSequence(context.Request().Context(), int(input.CustomerID), yy)
		if err != nil {
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "allocate quote number"})
		}
		sequenceWarn = warn
		quotation.SetQuoteNumber(backend.FormatQuoteNumber(input.CustomerID, uint8(yy), sequence), 0)
	}

	// Resolve the revision's public verification handle before rendering:
	// the QR code printed on the document must be in the stored snapshot
	// too, so both artifacts show the same address. A brand-new quotation
	// needs its external identity now, which is why it is generated here
	// rather than left to the column default.
	var draftUUID string
	shortID := ""
	if quoteNumber != "" {
		uuid, err := s.database.GetQuotationUUIDByNumber(context.Request().Context(), quoteNumber)
		if err != nil {
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "unknown quotation number"})
		}
		shortID = store.ShortIDForRevision(uuid, int16(revision))
	} else {
		uuid, err := store.NewQuotationUUID()
		if err != nil {
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "allocate quotation identity"})
		}
		draftUUID = uuid
		shortID = store.ShortIDForRevision(uuid, 0)
	}
	if shortID != "" {
		if err := quotation.SetVerifyURL(s.publicBaseURL() + "/v/" + shortID); err != nil {
			return context.JSON(http.StatusInternalServerError, map[string]string{"error": "build verification code"})
		}
	}

	snapshot, err := quotation.GetQuotationPreviewHTML()
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "render quotation snapshot"})
	}
	draft, err := quotation.DraftRevision(snapshot, nil, "")
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "build revision draft"})
	}
	draft.QuotationUUID = draftUUID
	// Sign the canonical field set at export time. The signature travels
	// with the revision and is what the public verification portal checks,
	// so it must cover exactly what was issued.
	if s.signer != nil {
		signature, digest := s.signer.SignRevision(backend.SignableFromData(quotation.Data))
		draft.Signature = signature
		draft.SigKeyID = s.signer.KeyID()
		draft.CanonicalSHA256 = digest
		draft.IssuerName = quotation.Data.CompanyName
	}
	record, err := s.database.SaveRevision(context.Request().Context(), draft)
	if err != nil {
		return context.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	}
	// The draft has become a quotation, so it has nothing left to be. Scoped
	// to the exporting user: a draft id that is not theirs is simply not found,
	// and the export stands either way. Deliberately best-effort -- the
	// quotation is already safely stored, and failing the whole request
	// because a stale draft would not delete would be worse than leaving one.
	if sourceDraftID > 0 {
		if err := s.database.DeleteDraft(context.Request().Context(), sourceDraftID, requestUser(context).ID); err != nil &&
			!errors.Is(err, store.ErrDraftNotFound) {
			log.Printf("consume draft %d after export: %v", sourceDraftID, err)
		}
	}
	return context.JSON(http.StatusCreated, apicontract.ExportResponse{
		ID:           record.ID,
		QuoteNumber:  record.QuoteNumber,
		UUID:         record.UUID,
		ShortID:      record.ShortID,
		Revision:     record.RevisionNumber,
		GrandTotal:   uint64(record.Totals.MarginalizedGrand),
		SequenceWarn: sequenceWarn,
	})
}

// defaultPublicBaseURL is the service address printed into verification
// codes when VERA_PUBLIC_BASE_URL is unset. A custom domain can replace it
// without invalidating already-printed codes: both hosts serve the portal.
const defaultPublicBaseURL = "https://verify.arconpentapersada.co.id"

// publicBaseURL returns the address printed on documents as the verification
// link. Configurable so a company domain can take over later.
func (s *Server) publicBaseURL() string {
	if base := strings.TrimSpace(os.Getenv("VERA_PUBLIC_BASE_URL")); base != "" {
		return strings.TrimRight(base, "/")
	}
	return defaultPublicBaseURL
}

// nextSequence previews the next full quote number for a customer without
// consuming it, so the desktop can pre-fill the save dialog filename. The
// server owns the format: callers pass ?customerId= and display the returned
// number verbatim. Allocation at export stays authoritative.
func (s *Server) nextSequence(context echo.Context) error {
	customerID, err := strconv.Atoi(context.QueryParam("customerId"))
	if err != nil || customerID < 1 || customerID > 99 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "valid customerId is required"})
	}
	if _, err := s.database.GetCustomerByCode(context.Request().Context(), customerID); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "unknown customer"})
	}
	yy := store.CurrentYY()
	sequence, warn, err := s.database.PeekQuoteSequence(context.Request().Context(), customerID, yy)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "preview quote number"})
	}
	return context.JSON(http.StatusOK, apicontract.PeekNumberResponse{
		Number: backend.FormatQuoteNumber(uint8(customerID), uint8(yy), sequence),
		Warn:   warn,
	})
}

func (s *Server) listEndpoint(context echo.Context) error {
	rows, err := s.database.ListQuotations(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "list quotations"})
	}
	summaries := make([]apicontract.QuotationSummaryRow, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, mapSummaryRow(row))
	}
	return context.JSON(http.StatusOK, map[string]any{"quotations": summaries})
}

// mapSummaryRow renders one archive row. The currency travels with the money
// it describes: a mixed-currency archive is unreadable without it, and the
// code is normalised so a row that predates the column reads as the default
// rather than an empty string beside an IDR-formatted figure.
func mapSummaryRow(row store.QuotationSummary) apicontract.QuotationSummaryRow {
	return apicontract.QuotationSummaryRow{
		ID:             row.ID,
		QuoteNumber:    row.QuoteNumber,
		RevisionNumber: row.RevisionNumber,
		CustomerName:   row.CustomerName,
		Subject:        row.Subject,
		Currency:       currency.Normalize(row.Currency),
		CreatedAt:      row.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		GrandTotal:     row.GrandTotal,
		DisplayGrand:   formatMoney(row.Currency, row.GrandTotal),
	}
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
	// The record already knows: GetQuotation scans pdf_data and pdf_path, so
	// asking the database again was a second round trip for a fact in hand.
	return context.JSON(http.StatusOK, mapDetail(record, record.HasDocument))
}

func (s *Server) revisionsEndpoint(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quotation id"})
	}
	// Deliberately not GetQuotation. That selects html_snapshot and pdf_data,
	// so this endpoint used to pull the whole document out of Postgres -- the
	// largest column in the database -- to return a list of int16. The narrow
	// query is served by UNIQUE (quotation_id, revision_number).
	revisions, err := s.database.ListRevisionNumbers(context.Request().Context(), id)
	if err != nil {
		return context.JSON(http.StatusNotFound, map[string]string{"error": "quotation not found"})
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
		subs := make([]apicontract.QuotationSubItemRow, 0, len(item.SubItems))
		for _, sub := range item.SubItems {
			details := make([]string, 0, len(sub.SubSubItems))
			for _, detail := range sub.SubSubItems {
				details = append(details, detail.Description)
			}
			subs = append(subs, apicontract.QuotationSubItemRow{
				SubNo:                    sub.SubNo,
				Description:              sub.Description,
				Details:                  details,
				Qty:                      sub.Qty,
				UoM:                      sub.UoM,
				OriginalUnitPrice:        sub.UnitCost,
				MarginEnabled:            sub.MarginEnabled,
				MarginPercent:            sub.MarginPercent,
				MarginAmount:             sub.MarginAmount,
				DiscountEnabled:          sub.DiscountEnabled,
				DiscountPercent:          sub.DiscountPercent,
				ListUnitPrice:            sub.ListUnitPrice,
				DiscountAmount:           sub.DiscountAmount,
				OriginalTotal:            sub.Qty * sub.UnitCost,
				MarginalizedUnitPrice:    sub.UnitPrice,
				MarginalizedTotal:        sub.Total,
				DisplayOriginalUnit:      formatMoney(record.Currency, sub.UnitCost),
				DisplayOriginalTotal:     formatMoney(record.Currency, sub.Qty*sub.UnitCost),
				DisplayMarginalizedUnit:  formatMoney(record.Currency, sub.UnitPrice),
				DisplayMarginalizedTotal: formatMoney(record.Currency, sub.Total),
				DisplayMarginAmount:      formatMoney(record.Currency, sub.MarginAmount),
			})
		}
		items = append(items, apicontract.QuotationItemRow{
			ItemNo:                   item.ItemNo,
			Description:              item.Description,
			Qty:                      item.Qty,
			UoM:                      item.UoM,
			OriginalUnitPrice:        item.UnitPrice,
			MarginEnabled:            item.MarginEnabled,
			MarginPercent:            int16(item.MarginPercent),
			MarginAmount:             item.MarginAmount,
			DiscountEnabled:          item.DiscountEnabled,
			DiscountPercent:          item.DiscountPercent,
			ListUnitPrice:            item.ListUnitPrice,
			DiscountAmount:           item.DiscountAmount,
			OriginalTotal:            item.OriginalTotal,
			MarginalizedUnitPrice:    item.MarginalizedUnitPrice,
			MarginalizedTotal:        item.MarginalizedTotal,
			ShowSubPrices:            item.ShowSubPrices,
			Subs:                     subs,
			DisplayOriginalUnit:      formatMoney(record.Currency, item.UnitPrice),
			DisplayOriginalTotal:     formatMoney(record.Currency, item.OriginalTotal),
			DisplayMarginalizedUnit:  formatMoney(record.Currency, item.MarginalizedUnitPrice),
			DisplayMarginalizedTotal: formatMoney(record.Currency, item.MarginalizedTotal),
			DisplayMarginAmount:      formatMoney(record.Currency, item.MarginAmount),
		})
	}
	return apicontract.QuotationDetail{
		ID:                          record.ID,
		QuoteNumber:                 record.QuoteNumber,
		UUID:                        record.UUID,
		Revision:                    record.RevisionNumber,
		AvailableRevisions:          record.AvailableRevisions,
		CustomerName:                record.CustomerName,
		Subject:                     record.Subject,
		Currency:                    currency.Normalize(record.Currency),
		RecipientName:               record.RecipientName,
		Incoterm:                    record.Incoterm,
		DiscountPercent:             record.DiscountPercent,
		TaxZone:                     record.TaxZone,
		TaxCategory:                 record.TaxCategory,
		TaxType:                     record.TaxType,
		ServiceCategory:             record.ServiceCategory,
		OriginalSubtotal:            record.Totals.OriginalSubtotal,
		OriginalDiscount:            record.Totals.OriginalDiscount,
		OriginalGrand:               record.Totals.OriginalGrand,
		MarginalizedSubtotal:        record.Totals.MarginalizedSubtotal,
		MarginalizedDiscount:        record.Totals.MarginalizedDiscount,
		MarginalizedGrand:           record.Totals.MarginalizedGrand,
		GrandTotal:                  record.Totals.CustomerGrandTotal,
		PpnAmount:                   record.Totals.PpnAmount,
		TaxAmount:                   record.Totals.TaxAmount,
		MarginalizedGrandAfterTax:   record.Totals.MarginalizedGrandAfterTax,
		ProfitAmount:                record.Totals.ProfitAmount,
		NettProfit:                  record.Totals.NettProfit,
		DisplayOriginalSubtotal:     formatMoney(record.Currency, record.Totals.OriginalSubtotal),
		DisplayOriginalDiscount:     formatMoney(record.Currency, record.Totals.OriginalDiscount),
		DisplayOriginalGrand:        formatMoney(record.Currency, record.Totals.OriginalGrand),
		DisplayMarginalizedSubtotal: formatMoney(record.Currency, record.Totals.MarginalizedSubtotal),
		DisplayMarginalizedDiscount: formatMoney(record.Currency, record.Totals.MarginalizedDiscount),
		DisplayMarginalizedGrand:    formatMoney(record.Currency, record.Totals.MarginalizedGrand),
		DisplayPpn:                  formatMoney(record.Currency, record.Totals.PpnAmount),
		DisplayTax:                  formatMoney(record.Currency, record.Totals.TaxAmount),
		DisplayMarginalizedAfterTax: formatMoney(record.Currency, record.Totals.MarginalizedGrandAfterTax),
		DisplayProfit:               formatSignedMoney(record.Currency, record.Totals.ProfitAmount),
		DisplayNett:                 formatSignedMoney(record.Currency, record.Totals.NettProfit),
		Items:                       items,
		HTMLSnapshot:                record.HTMLSnapshot,
		HasPDF:                      hasPDF,
	}
}
