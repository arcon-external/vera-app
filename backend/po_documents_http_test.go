package backend

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"
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
			DataBase64:      base64.StdEncoding.EncodeToString(data),
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
	decoded, decodeErr := base64.StdEncoding.DecodeString(content.DataBase64)
	if decodeErr != nil || string(decoded) != string(data) || content.Version != 2 ||
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

// Problem: Wails serializes a Go []byte as base64 *text*, but the generated
// TypeScript declared number[] and the panel built a zero-length array from
// it. The wire representation is therefore made explicit as a base64 string on
// both directions, and these tests pin the JSON shapes rather than the Go
// fields, because the JSON shape is what crosses the boundary.
const pdfHeaderBase64 = "JVBERi0xLjcK" // %PDF-1.7\n

func TestPurchaseOrderDocumentContentWireValueIsBase64(t *testing.T) {
	encoded, err := json.Marshal(PurchaseOrderDocumentContentDTO{DataBase64: pdfHeaderBase64})
	if err != nil {
		t.Fatalf("marshal content DTO: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode content DTO: %v", err)
	}
	if got := string(fields["DataBase64"]); got != `"`+pdfHeaderBase64+`"` {
		t.Errorf("DataBase64 = %s; want the base64 text %q", got, pdfHeaderBase64)
	}
	decoded, err := base64.StdEncoding.DecodeString(pdfHeaderBase64)
	if err != nil || string(decoded) != "%PDF-1.7\n" {
		t.Fatalf("the pinned wire literal does not decode to the PDF header (%v)", err)
	}
}

func TestPurchaseOrderDocumentContentWireFieldIsBase64(t *testing.T) {
	encoded, err := json.Marshal(PurchaseOrderDocumentContentDTO{})
	if err != nil {
		t.Fatalf("marshal content DTO: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode content DTO: %v", err)
	}
	if _, ok := fields["Data"]; ok {
		t.Errorf("content DTO still exposes Data (the raw-byte field Wails encodes as base64 anyway)")
	}
	if _, ok := fields["DataBase64"]; !ok {
		t.Errorf("content DTO has no DataBase64 field; fields = %v", keysOf(fields))
	}
}

func TestDecodePurchaseOrderPDFBase64(t *testing.T) {
	decoded, err := decodePurchaseOrderPDFBase64(pdfHeaderBase64)
	if err != nil || string(decoded) != "%PDF-1.7\n" {
		t.Fatalf("decode header = %q (%v); want %%PDF-1.7\n", decoded, err)
	}
	if decoded, err := decodePurchaseOrderPDFBase64("   "); err != nil || decoded != nil {
		t.Errorf("blank input = %q (%v); want nil and no error", decoded, err)
	}
	if _, err := decodePurchaseOrderPDFBase64("not base64!"); err == nil ||
		!strings.Contains(err.Error(), "base64") {
		t.Fatalf("malformed base64 error = %v; want a decode refusal", err)
	}
	// The boundary belongs to the accepted side: a file of exactly 20 MiB is
	// encoded, accepted, and decoded intact.
	exactData := bytes.Repeat([]byte("A"), int(20<<20))
	exact := base64.StdEncoding.EncodeToString(exactData)
	decodedExact, err := decodePurchaseOrderPDFBase64(exact)
	if err != nil || len(decodedExact) != len(exactData) {
		t.Fatalf("exactly 20 MiB base64 = %d bytes (%v); want %d", len(decodedExact), err, len(exactData))
	}
	// The encoded bound is checked before decoding, so an oversized string is
	// refused without first building its decoded copy.
	oversized := base64.StdEncoding.EncodeToString(make([]byte, int(20<<20)+1))
	if _, err := decodePurchaseOrderPDFBase64(oversized); err == nil ||
		!strings.Contains(err.Error(), "20 MB") {
		t.Fatalf("oversized base64 error = %v; want the 20 MB refusal", err)
	}
}

func TestPurchaseOrderDocumentUploadAcceptsTheBase64JSONShape(t *testing.T) {
	data := documentTestBytes("base64-wire")
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		filename, received := readMultipartDocument(t, request)
		if filename != "Customer PO.pdf" {
			t.Errorf("filename = %q; want Customer PO.pdf", filename)
		}
		if string(received) != string(data) {
			t.Errorf("uploaded %q; want %q", received, data)
		}
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderDocumentRow{
			ID: 5, PurchaseOrderID: 7, Version: 1, Active: true,
		})
	})
	// The exact JSON a Wails client sends for the argument struct.
	wire := fmt.Sprintf(`{"PurchaseOrderID":7,"DataBase64":%q,"Filename":"Customer PO.pdf"}`,
		base64.StdEncoding.EncodeToString(data))
	var input PurchaseOrderDocumentUploadInput
	if err := json.Unmarshal([]byte(wire), &input); err != nil {
		t.Fatalf("decode upload input: %v", err)
	}
	q := newTestQuotation()
	document, err := q.AttachPurchaseOrderDocument(input)
	if err != nil {
		t.Fatalf("attach from base64: %v", err)
	}
	if document.ID != 5 {
		t.Errorf("document = %+v; want id 5", document)
	}
}

func keysOf(fields map[string]json.RawMessage) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
