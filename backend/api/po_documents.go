package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
)

// Issue #472: the authenticated customer PO document surface.
//
// One optional PDF per purchase order, uploaded as a multipart file so the
// request bound is enforced on the wire rather than after a base64 expansion.
// The current document and every retained version are read with the caller's
// organization context; the store's join is the actual tenant filter.
//
// Documents are independent of the PO fields: an upload, replacement or
// removal never runs a purchase-order update.

// uploadPurchaseOrderDocument stores the uploaded PDF as the next version.
// The extra MiB above the document bound is multipart envelope headroom; the
// decoded file is checked again against the exact bound.
func (s *Server) uploadPurchaseOrderDocument(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	request := context.Request()
	request.Body = http.MaxBytesReader(context.Response(), request.Body,
		store.MaxPurchaseOrderDocumentBytes+(1<<20))
	if err := request.ParseMultipartForm(store.MaxPurchaseOrderDocumentBytes); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) ||
			strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			return purchaseOrderError(context, store.ErrPurchaseOrderDocumentTooLarge)
		}
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "the upload is not a readable PDF file",
			"code":  "po_document_upload_invalid",
		})
	}
	if request.MultipartForm != nil {
		defer func() { _ = request.MultipartForm.RemoveAll() }()
	}
	file, header, err := request.FormFile("document")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "choose a PDF to attach",
			"code":  "po_document_missing",
		})
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, store.MaxPurchaseOrderDocumentBytes+1))
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "the uploaded PDF could not be read",
			"code":  "po_document_upload_invalid",
		})
	}
	if int64(len(data)) > store.MaxPurchaseOrderDocumentBytes {
		return purchaseOrderError(context, store.ErrPurchaseOrderDocumentTooLarge)
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	user := requestUser(context)
	document, err := s.database.AttachPurchaseOrderDocument(requestContext, store.PurchaseOrderDocumentUpload{
		PurchaseOrderID: id,
		Data:            data,
		Filename:        header.Filename,
		UploadedBy:      user.ID,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusCreated, mapPurchaseOrderDocument(document))
}

// getPurchaseOrderDocument returns the active document's metadata and the
// retained versions. A purchase order with no PDF is a successful empty state,
// not an error.
func (s *Server) getPurchaseOrderDocument(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	state, err := s.database.GetPurchaseOrderDocumentState(requestContext, id)
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrderDocumentState(state))
}

// getPurchaseOrderDocumentContent serves the active PDF bytes.
func (s *Server) getPurchaseOrderDocumentContent(context echo.Context) error {
	return s.servePurchaseOrderDocument(context, 0)
}

// getPurchaseOrderDocumentVersionContent serves one retained version, so a
// replaced or removed PDF remains inspectable as correction history.
func (s *Server) getPurchaseOrderDocumentVersionContent(context echo.Context) error {
	version, err := store.ParsePurchaseOrderDocumentVersion(context.Param("version"))
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{
			"error": "document version must be a positive number",
		})
	}
	return s.servePurchaseOrderDocument(context, version)
}

func (s *Server) servePurchaseOrderDocument(context echo.Context, version int32) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	var document store.PurchaseOrderDocument
	if version == 0 {
		document, err = s.database.GetActivePurchaseOrderDocument(requestContext, id)
	} else {
		document, err = s.database.GetPurchaseOrderDocumentVersion(requestContext, id, version)
	}
	if err != nil {
		return purchaseOrderError(context, err)
	}
	disposition := mime.FormatMediaType("inline", map[string]string{"filename": document.OriginalFilename})
	if disposition == "" {
		disposition = "inline"
	}
	response := context.Response()
	response.Header().Set(echo.HeaderContentType, store.PurchaseOrderDocumentContentType)
	response.Header().Set(echo.HeaderContentDisposition, disposition)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Cache-Control", "private, no-store")
	response.Header().Set("X-Document-Version", strconv.Itoa(int(document.Version)))
	response.Header().Set("X-Document-SHA256", document.SHA256)
	return context.Blob(http.StatusOK, store.PurchaseOrderDocumentContentType, document.Data)
}

// removePurchaseOrderDocument marks the active PDF removed. The confirmation
// is part of the request contract, not only the editor's UI: an accidental or
// automated DELETE without it is refused before any row changes.
func (s *Server) removePurchaseOrderDocument(context echo.Context) error {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid purchase order id"})
	}
	var request apicontract.PurchaseOrderDocumentRemovalRequest
	if err := json.NewDecoder(context.Request().Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if !request.Confirmed {
		return purchaseOrderError(context, store.ErrPurchaseOrderDocumentRemovalNotConfirmed)
	}
	requestContext, err := s.organizationContext(context.Request().Context())
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "resolve organization"})
	}
	user := requestUser(context)
	document, err := s.database.RemovePurchaseOrderDocument(requestContext, store.PurchaseOrderDocumentRemoval{
		PurchaseOrderID: id,
		RemovedBy:       user.ID,
		Confirmed:       true,
	})
	if err != nil {
		return purchaseOrderError(context, err)
	}
	return context.JSON(http.StatusOK, mapPurchaseOrderDocument(document))
}

func mapPurchaseOrderDocument(document store.PurchaseOrderDocument) apicontract.PurchaseOrderDocumentRow {
	row := apicontract.PurchaseOrderDocumentRow{
		ID:               document.ID,
		UUID:             document.UUID,
		PurchaseOrderID:  document.PurchaseOrderID,
		Version:          document.Version,
		SHA256:           document.SHA256,
		SizeBytes:        document.SizeBytes,
		ContentType:      document.ContentType,
		OriginalFilename: document.OriginalFilename,
		UploadedBy:       document.UploadedBy,
		UploadedAt:       document.UploadedAt.Format(time.RFC3339),
		Active:           document.Active,
		RemovedBy:        document.RemovedBy,
	}
	if document.ReplacedAt != nil {
		formatted := document.ReplacedAt.Format(time.RFC3339)
		row.ReplacedAt = &formatted
	}
	if document.RemovedAt != nil {
		formatted := document.RemovedAt.Format(time.RFC3339)
		row.RemovedAt = &formatted
	}
	return row
}

func mapPurchaseOrderDocumentState(state store.PurchaseOrderDocumentState) apicontract.PurchaseOrderDocumentStateRow {
	row := apicontract.PurchaseOrderDocumentStateRow{
		Versions: make([]apicontract.PurchaseOrderDocumentRow, 0, len(state.Versions)),
	}
	if state.Active != nil {
		active := mapPurchaseOrderDocument(*state.Active)
		row.Document = &active
	}
	for _, document := range state.Versions {
		row.Versions = append(row.Versions, mapPurchaseOrderDocument(document))
	}
	return row
}
