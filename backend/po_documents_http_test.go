package backend

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #472: the desktop binding shapes for the customer PO PDF. The API
// suite proves the server; these prove the wrapper sends a real multipart file
// from either bytes or a local path, returns the metadata the panel renders,
// and asks for content with the version/SHA headers the viewer needs.

func documentTestBytes(label string) []byte {
	return []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Root 1 0 R /Label (" + label + ") >>\nendobj\n" +
		"trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n9\n%%EOF\n")
}

func readMultipartDocument(t *testing.T, request *http.Request) (filename string, data []byte) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q; want multipart/form-data (%v)", request.Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(request.Body, params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		t.Fatalf("read multipart part: %v", err)
	}
	if part.FormName() != "document" {
		t.Errorf("field name = %q; want document", part.FormName())
	}
	data, err = io.ReadAll(part)
	if err != nil {
		t.Fatalf("read multipart data: %v", err)
	}
	return part.FileName(), data
}

func TestAttachPurchaseOrderDocumentSendsBytesOrAPath(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		data := documentTestBytes("bytes")
		stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodPost || request.URL.Path != "/v1/purchase-orders/7/document" {
				t.Errorf("request = %s %s; want POST /v1/purchase-orders/7/document", request.Method, request.URL.Path)
			}
			filename, received := readMultipartDocument(t, request)
			if filename != "Customer PO.pdf" {
				t.Errorf("filename = %q; want Customer PO.pdf", filename)
			}
			if string(received) != string(data) {
				t.Error("uploaded bytes differ from the supplied bytes")
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderDocumentRow{
				ID: 3, UUID: "doc-3", PurchaseOrderID: 7, Version: 1,
				SHA256: "abc", SizeBytes: int64(len(data)),
				ContentType: "application/pdf", OriginalFilename: "Customer PO.pdf",
				UploadedBy: 4, UploadedAt: "2026-10-21T10:00:00Z", Active: true,
			})
		})
		q := newTestQuotation()
		document, err := q.AttachPurchaseOrderDocument(PurchaseOrderDocumentUploadInput{
			PurchaseOrderID: 7,
			Data:            data,
			Filename:        "Customer PO.pdf",
		})
		if err != nil {
			t.Fatalf("attach: %v", err)
		}
		if document.ID != 3 || document.Version != 1 || !document.Active ||
			document.OriginalFilename != "Customer PO.pdf" ||
			document.UploadedAt != "2026-10-21T10:00:00Z" {
			t.Errorf("document = %+v; want the mapped metadata", document)
		}
	})

	t.Run("path", func(t *testing.T) {
		data := documentTestBytes("path")
		filePath := filepath.Join(t.TempDir(), "from-disk.pdf")
		if err := os.WriteFile(filePath, data, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
			filename, received := readMultipartDocument(t, request)
			if filename != "from-disk.pdf" {
				t.Errorf("filename = %q; want the path's base name", filename)
			}
			if string(received) != string(data) {
				t.Error("uploaded bytes differ from the file on disk")
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderDocumentRow{
				ID: 4, PurchaseOrderID: 7, Version: 1, Active: true,
			})
		})
		q := newTestQuotation()
		document, err := q.AttachPurchaseOrderDocument(PurchaseOrderDocumentUploadInput{
			PurchaseOrderID: 7,
			FilePath:        filePath,
		})
		if err != nil {
			t.Fatalf("attach from path: %v", err)
		}
		if document.ID != 4 {
			t.Errorf("document = %+v; want id 4", document)
		}
	})

	t.Run("neither", func(t *testing.T) {
		q := newTestQuotation()
		if _, err := q.AttachPurchaseOrderDocument(PurchaseOrderDocumentUploadInput{PurchaseOrderID: 7}); err == nil {
			t.Fatal("an empty upload was accepted")
		}
	})
}

func TestGetPurchaseOrderDocumentStateMapsActiveAndHistory(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/purchase-orders/7/document" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		replaced := "2026-10-21T09:00:00Z"
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderDocumentStateRow{
			Document: &apicontract.PurchaseOrderDocumentRow{
				ID: 2, Version: 2, Active: true, OriginalFilename: "replacement.pdf",
				SizeBytes: 120, SHA256: "sha-2", ContentType: "application/pdf",
				UploadedBy: 4, UploadedAt: "2026-10-21T10:00:00Z",
			},
			Versions: []apicontract.PurchaseOrderDocumentRow{
				{ID: 2, Version: 2, Active: true},
				{ID: 1, Version: 1, Active: false, ReplacedAt: &replaced},
			},
		})
	})
	q := newTestQuotation()
	state, err := q.GetPurchaseOrderDocumentState(7)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.Document == nil || state.Document.Version != 2 || !state.Document.Active {
		t.Fatalf("active = %+v; want version 2", state.Document)
	}
	if len(state.Versions) != 2 || state.Versions[1].Version != 1 ||
		state.Versions[1].ReplacedAt == nil ||
		*state.Versions[1].ReplacedAt != "2026-10-21T09:00:00Z" {
		t.Errorf("history = %+v", state.Versions)
	}
}

func TestGetPurchaseOrderDocumentContentMapsHeaders(t *testing.T) {
	data := documentTestBytes("content")
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/purchase-orders/7/documents/2/content" {
			t.Errorf("path = %s; want version 2 content", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/pdf")
		writer.Header().Set("Content-Disposition", `inline; filename="Customer PO.pdf"`)
		writer.Header().Set("X-Document-Version", "2")
		writer.Header().Set("X-Document-SHA256", "sha-content")
		_, _ = writer.Write(data)
	})
	q := newTestQuotation()
	content, err := q.GetPurchaseOrderDocumentVersionContent(7, 2)
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	if string(content.Data) != string(data) || content.Version != 2 ||
		content.SHA256 != "sha-content" || content.Filename != "Customer PO.pdf" ||
		content.ContentType != "application/pdf" {
		t.Errorf("content = %+v; want bytes with version 2 headers", content)
	}
}

func TestRemovePurchaseOrderDocumentSendsTheConfirmation(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/v1/purchase-orders/7/document" {
			t.Errorf("request = %s %s; want DELETE the document", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode removal body: %v", err)
		}
		if body["confirmed"] != true {
			t.Errorf("body = %v; want confirmed true", body)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderDocumentRow{
			ID: 2, Version: 2, Active: false, RemovedBy: int64Ptr(4),
			RemovedAt: stringPtr("2026-10-21T11:00:00Z"),
		})
	})
	q := newTestQuotation()
	document, err := q.RemovePurchaseOrderDocument(7, true)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if document.Active || document.RemovedBy == nil || document.RemovedAt == nil {
		t.Errorf("removed = %+v; want removal metadata", document)
	}
}

func int64Ptr(value int64) *int64    { return &value }
func stringPtr(value string) *string { return &value }
