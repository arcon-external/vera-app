package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Issue #472: the authenticated document surface. These tests use the real
// server and disposable Postgres: a multipart upload is validated and stored,
// metadata/history/content are org-scoped reads, a replacement keeps the prior
// version addressable, and removal needs its own explicit confirmation.

type purchaseOrderDocumentRow struct {
	ID               int64   `json:"id"`
	UUID             string  `json:"uuid"`
	PurchaseOrderID  int64   `json:"purchaseOrderId"`
	Version          int32   `json:"version"`
	SHA256           string  `json:"sha256"`
	SizeBytes        int64   `json:"sizeBytes"`
	ContentType      string  `json:"contentType"`
	OriginalFilename string  `json:"originalFilename"`
	UploadedBy       int64   `json:"uploadedBy"`
	UploadedAt       string  `json:"uploadedAt"`
	Active           bool    `json:"active"`
	ReplacedAt       *string `json:"replacedAt"`
	RemovedAt        *string `json:"removedAt"`
	RemovedBy        *int64  `json:"removedBy"`
}

type purchaseOrderDocumentStateRow struct {
	Document *purchaseOrderDocumentRow  `json:"document"`
	Versions []purchaseOrderDocumentRow `json:"versions"`
}

func documentTestPDF(suffix string) []byte {
	return []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
		"2 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n" +
		"3 0 obj\n<< /Type /Metadata /Label (" + suffix + ") >>\nendobj\n" +
		"trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n200\n%%EOF\n")
}

func uploadPurchaseOrderDocument(t *testing.T, server *Server, token string, id int64, filename string, data []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("document", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/document", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	var decoded map[string]any
	if len(bytes.TrimSpace(recorder.Body.Bytes())) > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("decode upload response: %v (%s)", err, recorder.Body.String())
		}
	}
	return recorder.Code, decoded
}

func getPurchaseOrderDocumentRaw(t *testing.T, server *Server, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func decodeDocumentState(t *testing.T, body map[string]any) purchaseOrderDocumentStateRow {
	t.Helper()
	var state purchaseOrderDocumentStateRow
	if err := json.Unmarshal(mustJSON(t, body), &state); err != nil {
		t.Fatalf("decode document state: %v", err)
	}
	return state
}

func mustJSON(t *testing.T, body map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return encoded
}

func TestPurchaseOrderDocumentUploadReadReplaceAndHistory(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_document_flow")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d; want 201 (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)

	firstData := documentTestPDF("first")
	code, body = uploadPurchaseOrderDocument(t, server, token, draft.ID, "Customer PO.pdf", firstData)
	if code != http.StatusCreated {
		t.Fatalf("upload = %d; want 201 (%v)", code, body)
	}
	first := decodePurchaseOrderDocument(t, body)
	if first.Version != 1 || !first.Active || first.PurchaseOrderID != draft.ID {
		t.Errorf("first document = %+v; want active version 1", first)
	}
	if first.OriginalFilename != "Customer PO.pdf" || first.ContentType != "application/pdf" ||
		first.SizeBytes != int64(len(firstData)) || first.SHA256 == "" {
		t.Errorf("first metadata = %+v", first)
	}

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/document", token, nil)
	if code != http.StatusOK {
		t.Fatalf("document state = %d; want 200 (%v)", code, body)
	}
	state := decodeDocumentState(t, body)
	if state.Document == nil || state.Document.Version != 1 || len(state.Versions) != 1 {
		t.Errorf("state = %+v; want the active document and one version", state)
	}

	content := getPurchaseOrderDocumentRaw(t, server, token,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/document/content")
	if content.Code != http.StatusOK {
		t.Fatalf("content = %d; want 200 (%s)", content.Code, content.Body.String())
	}
	if !bytes.Equal(content.Body.Bytes(), firstData) {
		t.Error("served content is not the uploaded bytes")
	}
	if got := content.Header().Get("Content-Type"); got != "application/pdf" {
		t.Errorf("content type = %q; want application/pdf", got)
	}
	if got := content.Header().Get("X-Document-Version"); got != "1" {
		t.Errorf("version header = %q; want 1", got)
	}
	if got := content.Header().Get("Content-Disposition"); !strings.Contains(got, "Customer PO.pdf") {
		t.Errorf("disposition = %q; want the original filename", got)
	}

	secondData := documentTestPDF("second")
	code, body = uploadPurchaseOrderDocument(t, server, token, draft.ID, "replacement.pdf", secondData)
	if code != http.StatusCreated {
		t.Fatalf("replace = %d; want 201 (%v)", code, body)
	}
	second := decodePurchaseOrderDocument(t, body)
	if second.Version != 2 || !second.Active {
		t.Errorf("replacement = %+v; want active version 2", second)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/document", token, nil)
	if code != http.StatusOK {
		t.Fatalf("state after replace = %d (%v)", code, body)
	}
	state = decodeDocumentState(t, body)
	if state.Document == nil || state.Document.Version != 2 || len(state.Versions) != 2 {
		t.Fatalf("state after replace = %+v", state)
	}
	if state.Versions[0].Version != 2 || state.Versions[1].Version != 1 ||
		state.Versions[1].ReplacedAt == nil || state.Versions[1].Active {
		t.Errorf("history = %+v; want newest first with version 1 replaced", state.Versions)
	}

	oldContent := getPurchaseOrderDocumentRaw(t, server, token,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/documents/1/content")
	if oldContent.Code != http.StatusOK || !bytes.Equal(oldContent.Body.Bytes(), firstData) {
		t.Errorf("version 1 content = %d, equal=%v; want the original bytes",
			oldContent.Code, bytes.Equal(oldContent.Body.Bytes(), firstData))
	}
	activeContent := getPurchaseOrderDocumentRaw(t, server, token,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/document/content")
	if !bytes.Equal(activeContent.Body.Bytes(), secondData) {
		t.Error("active content is not the replacement")
	}

	// The document attach never touched a PO field.
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen PO = %d (%v)", code, body)
	}
	reopened := decodePurchaseOrder(t, body)
	if reopened.PONumber != draft.PONumber || len(reopened.Items) != len(draft.Items) {
		t.Errorf("document upload changed the purchase order: %+v", reopened)
	}
}

func decodePurchaseOrderDocument(t *testing.T, body map[string]any) purchaseOrderDocumentRow {
	t.Helper()
	var row purchaseOrderDocumentRow
	if err := json.Unmarshal(mustJSON(t, body), &row); err != nil {
		t.Fatalf("decode document: %v", err)
	}
	return row
}

func TestPurchaseOrderDocumentUploadRefusesInvalidEncryptedAndOversized(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_document_validate")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)

	cases := []struct {
		name        string
		data        []byte
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"garbage", []byte("not a pdf"), http.StatusUnprocessableEntity, "po_document_invalid", "valid PDF"},
		{"encrypted", []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Root 1 0 R >>\nendobj\ntrailer\n<< /Size 2 /Root 1 0 R /Encrypt 9 0 R >>\nstartxref\n9\n%%EOF\n"), http.StatusUnprocessableEntity, "po_document_encrypted", "unlocked"},
		{"oversized", bytes.Repeat([]byte("x"), int(20<<20)+1), http.StatusRequestEntityTooLarge, "po_document_too_large", "20 MB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := uploadPurchaseOrderDocument(t, server, token, draft.ID, "bad.pdf", tc.data)
			if code != tc.wantStatus {
				t.Fatalf("status = %d; want %d (%v)", code, tc.wantStatus, body)
			}
			if got := fmt.Sprint(body["code"]); got != tc.wantCode {
				t.Errorf("code = %q; want %q", got, tc.wantCode)
			}
			if message := fmt.Sprint(body["error"]); !strings.Contains(message, tc.wantMessage) {
				t.Errorf("message = %q; want it to contain %q", message, tc.wantMessage)
			}
		})
	}

	// Nothing refused was stored.
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/document", token, nil)
	if code != http.StatusOK {
		t.Fatalf("document state = %d (%v)", code, body)
	}
	state := decodeDocumentState(t, body)
	if state.Document != nil || len(state.Versions) != 0 {
		t.Errorf("refused uploads were stored: %+v", state)
	}
}

func TestFinalizeWithAttachedPDFSkipsTheAbsenceConfirmation(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_document_finalize")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	if code, body := uploadPurchaseOrderDocument(t, server, token, draft.ID, "attached.pdf", documentTestPDF("attached")); code != http.StatusCreated {
		t.Fatalf("upload = %d (%v)", code, body)
	}

	// The review sees the attached branch.
	code, body = doJSON(t, server, http.MethodGet, path+"/review", token, nil)
	if code != http.StatusOK || fmt.Sprint(body["documentState"]) != "attached" {
		t.Fatalf("review = %d %v; want documentState attached", code, body)
	}

	// A final save with no absence confirmation succeeds because the PDF is
	// attached, and it must not record an absence.
	payload := finalizeBody(draft, "PO-ATTACHED")
	payload["documentConfirmed"] = false
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, payload)
	if code != http.StatusOK {
		t.Fatalf("attached finalize = %d (%v); want 200 without the checkbox", code, body)
	}
	saved := decodePurchaseOrder(t, body)
	if saved.State != "saved" || saved.PdfAbsentConfirmed {
		t.Errorf("saved = %+v; want saved with no absence confirmation", saved)
	}

	// Removing the PDF afterwards changes only the document table; the saved
	// record still does not claim a deliberate absence.
	code, body = doJSON(t, server, http.MethodDelete, path+"/document", token, map[string]any{"confirmed": true})
	if code != http.StatusOK {
		t.Fatalf("remove after save = %d (%v)", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet, path, token, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen saved = %d (%v)", code, body)
	}
	reopened := decodePurchaseOrder(t, body)
	if reopened.PdfAbsentConfirmed {
		t.Error("removing an attached PDF rewrote the saved absence confirmation")
	}
}

func TestPurchaseOrderDocumentRemovalNeedsConfirmationAndKeepsHistory(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_document_remove")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	data := documentTestPDF("remove")
	code, body = uploadPurchaseOrderDocument(t, server, token, draft.ID, "remove.pdf", data)
	if code != http.StatusCreated {
		t.Fatalf("upload = %d (%v)", code, body)
	}
	document := decodePurchaseOrderDocument(t, body)

	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10) + "/document"
	code, body = doJSON(t, server, http.MethodDelete, path, token, map[string]any{})
	if code != http.StatusUnprocessableEntity || fmt.Sprint(body["code"]) != "po_document_removal_unconfirmed" {
		t.Fatalf("unconfirmed removal = %d %v; want 422 po_document_removal_unconfirmed", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet, path, token, nil)
	if state := decodeDocumentState(t, body); code != http.StatusOK || state.Document == nil {
		t.Fatalf("unconfirmed removal removed the document: %d %+v", code, state)
	}

	code, body = doJSON(t, server, http.MethodDelete, path, token, map[string]any{"confirmed": true})
	if code != http.StatusOK {
		t.Fatalf("confirmed removal = %d (%v)", code, body)
	}
	removed := decodePurchaseOrderDocument(t, body)
	if removed.Active || removed.RemovedAt == nil || removed.RemovedBy == nil {
		t.Errorf("removal metadata = %+v", removed)
	}
	code, body = doJSON(t, server, http.MethodGet, path, token, nil)
	if state := decodeDocumentState(t, body); code != http.StatusOK || state.Document != nil || len(state.Versions) != 1 {
		t.Errorf("state after removal = %d %+v; want no active document and one retained version", code, state)
	}
	history := getPurchaseOrderDocumentRaw(t, server, token,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/documents/"+
			strconv.Itoa(int(document.Version))+"/content")
	if history.Code != http.StatusOK || !bytes.Equal(history.Body.Bytes(), data) {
		t.Errorf("retained version = %d; want the original bytes", history.Code)
	}
}

func TestPurchaseOrderDocumentReadsRequireASessionAndARealDocument(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_document_guards")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)

	if code, _ := uploadPurchaseOrderDocument(t, server, "", draft.ID, "x.pdf", documentTestPDF("auth")); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated upload = %d; want 401", code)
	}
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10) + "/document"
	if code, _ := doJSON(t, server, http.MethodGet, path, "", nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated state = %d; want 401", code)
	}
	if recorder := getPurchaseOrderDocumentRaw(t, server, "", path+"/content"); recorder.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated content = %d; want 401", recorder.Code)
	}

	// An empty state is legitimate: the document is optional.
	code, body = doJSON(t, server, http.MethodGet, path, token, nil)
	if code != http.StatusOK {
		t.Fatalf("empty state = %d (%v)", code, body)
	}
	state := decodeDocumentState(t, body)
	if state.Document != nil || state.Versions == nil || len(state.Versions) != 0 {
		t.Errorf("empty state = %+v; want null document and an empty versions list", state)
	}

	if code, _ := uploadPurchaseOrderDocument(t, server, token, draft.ID+9999, "x.pdf", documentTestPDF("missing")); code != http.StatusNotFound {
		t.Errorf("upload to a missing PO = %d; want 404", code)
	}
	if recorder := getPurchaseOrderDocumentRaw(t, server, token, path+"/content"); recorder.Code != http.StatusNotFound {
		t.Errorf("content without a document = %d; want 404", recorder.Code)
	}
	if recorder := getPurchaseOrderDocumentRaw(t, server, token,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/documents/99/content"); recorder.Code != http.StatusNotFound {
		t.Errorf("unknown version = %d; want 404", recorder.Code)
	}
	if recorder := getPurchaseOrderDocumentRaw(t, server, token,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/documents/0/content"); recorder.Code != http.StatusBadRequest {
		t.Errorf("version 0 = %d; want 400", recorder.Code)
	}
}
