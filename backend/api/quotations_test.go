package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"vera-app/backend"
	"vera-app/backend/auth"
	"vera-app/backend/store"
)

func testQuotationServer(t *testing.T, schema string) (*Server, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	ctx := context.Background()
	database, err := store.Open(ctx, store.Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(database.Close)

	templates := &backend.Quotation{}
	if err := backend.Startup(templates, ctx); err != nil {
		t.Fatalf("load templates: %v", err)
	}
	service := auth.NewService(database)
	if _, err := service.Bootstrap(ctx, "api-quote@example.com", "API Quotes", "test-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	server := NewServer(database, service, templates)
	seedQuotationClient(t, database, 2, "Test Customer")

	token, _, err := service.Login(ctx, "api-quote@example.com", "test-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return server, token
}

func quotationInput() map[string]any {
	return map[string]any{
		"Items": []map[string]any{
			{"Description": "Test item", "Qty": 2, "UoM": "Lot", "UnitPrice": 100000, "MarginEnabled": true, "MarginPercent": 20},
			{"Description": "Plain item", "Qty": 1, "UoM": "Unit", "UnitPrice": 50000, "MarginEnabled": false, "MarginPercent": 0},
		},
		"Subject":         "Test quotation",
		"RecipientName":   "Test Recipient",
		"DiscountPercent": 10,
		"TaxZone":         "FTZ",
		"Validity":        30,
		"PaymentTerm":     30,
		"LeadTime":        10,
		"CustomerID":      2,
		"Incoterm":        "EXW",
	}
}

func doJSON(t *testing.T, server *Server, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	var decoded map[string]any
	if len(bytes.TrimSpace(recorder.Body.Bytes())) > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s: decode response: %v (%s)", method, path, err, recorder.Body.String())
		}
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	return recorder.Code, decoded
}

func TestPrepareEndpointPricesQuotation(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_prepare")

	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/prepare", token, quotationInput())
	if code != http.StatusOK {
		t.Fatalf("prepare = %d; want 200 (%v)", code, body)
	}
	customer, _ := body["customer"].(map[string]any)
	internal, _ := body["internal"].(map[string]any)
	// Sell: 2x120000 + 1x50000 = 290000; discount 10% -> grand 261000.
	if customer["GrandTotal"] != 261000.0 {
		t.Errorf("customer grand = %v; want 261000", customer["GrandTotal"])
	}
	// Cost: 2x100000 + 1x50000 = 250000; discount 10% -> 225000; FTZ nett profit 36000.
	if internal["NettProfit"] != 36000.0 {
		t.Errorf("nett profit = %v; want 36000", internal["NettProfit"])
	}
}

func TestPrepareEndpointRejectsInvalidInput(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_invalid")

	code, _ := doJSON(t, server, http.MethodPost, "/v1/quotations/prepare", token, map[string]any{
		"Items":         []any{},
		"Subject":       "Test",
		"RecipientName": "Test",
		"Validity":      30,
		"PaymentTerm":   30,
		"LeadTime":      10,
		"CustomerID":    2,
		"Incoterm":      "EXW",
		"TaxZone":       "FTZ",
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid prepare = %d; want 422", code)
	}
}

func TestExportHeaderTracksCompanyProfile(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_company")
	database, err := store.Open(context.Background(), store.Config{
		DSN:    strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN")),
		Schema: "vera_api_quote_company",
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	if _, err := database.UpdateCompanyProfile(context.Background(), store.CompanyProfile{
		LegalName: "PT Live Header",
		Address:   "Live Street 9",
	}); err != nil {
		t.Fatalf("update company profile: %v", err)
	}

	code, exported := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput())
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201 (%v)", code, exported)
	}
	id, _ := exported["id"].(float64)
	quotePath := "/v1/quotations/" + strconv.Itoa(int(id))

	code, detail := doJSON(t, server, http.MethodGet, quotePath, token, nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d; want 200", code)
	}
	snapshot, _ := detail["htmlSnapshot"].(string)
	for _, want := range []string{"PT Live Header", "Live Street 9"} {
		if !strings.Contains(snapshot, want) {
			t.Errorf("snapshot missing stored %q", want)
		}
	}
}

func TestExportCreatesImmutableRevisions(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_export")

	code, first := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput())
	if code != http.StatusCreated {
		t.Fatalf("export rev0 = %d; want 201 (%v)", code, first)
	}
	if first["revision"] != 0.0 {
		t.Errorf("first revision = %v; want 0", first["revision"])
	}
	quoteNumber, _ := first["quoteNumber"].(string)
	if quoteNumber == "" {
		t.Fatal("empty quote number")
	}
	uuid, _ := first["uuid"].(string)
	if uuid == "" {
		t.Fatal("empty quotation uuid")
	}
	quoteID, _ := first["id"].(float64)
	if quoteID == 0 {
		t.Fatal("empty quotation id")
	}
	quotePath := "/v1/quotations/" + strconv.Itoa(int(quoteID))

	secondInput := quotationInput()
	code, second := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":       secondInput,
		"quoteNumber": quoteNumber,
		"revision":    1,
	})
	if code != http.StatusCreated {
		t.Fatalf("export rev1 = %d; want 201 (%v)", code, second)
	}
	if second["revision"] != 1.0 {
		t.Errorf("second revision = %v; want 1", second["revision"])
	}
	if second["uuid"] != uuid {
		t.Errorf("revision uuid = %v; want stable %s", second["uuid"], uuid)
	}

	code, revisions := doJSON(t, server, http.MethodGet, quotePath+"/revisions", token, nil)
	if code != http.StatusOK {
		t.Fatalf("revisions = %d; want 200", code)
	}
	list, _ := revisions["revisions"].([]any)
	if len(list) != 2 || list[0] != 1.0 || list[1] != 0.0 {
		t.Errorf("revisions = %v; want [1 0]", list)
	}

	code, historical := doJSON(t, server, http.MethodGet, quotePath+"?revision=0", token, nil)
	if code != http.StatusOK {
		t.Fatalf("historical detail = %d; want 200", code)
	}
	if historical["RevisionNumber"] != 0.0 && historical["revision"] != 0.0 {
		t.Errorf("historical revision markers = %v / %v; want 0", historical["RevisionNumber"], historical["revision"])
	}
}

func TestArchiveEndpointsServeLatestAndHistory(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_archive")

	if code, _ := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput()); code != http.StatusCreated {
		t.Fatalf("export = %d; want 201", code)
	}

	code, list := doJSON(t, server, http.MethodGet, "/v1/quotations", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d; want 200", code)
	}
	items, _ := list["quotations"].([]any)
	if len(items) != 1 {
		t.Fatalf("archive rows = %d; want 1 (latest only)", len(items))
	}
	first, _ := items[0].(map[string]any)
	id, _ := first["id"].(float64)
	quotePath := "/v1/quotations/" + strconv.Itoa(int(id))

	code, detail := doJSON(t, server, http.MethodGet, quotePath, token, nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d; want 200", code)
	}
	if detail["grandTotal"] != 261000.0 {
		t.Errorf("detail grand = %v; want 261000", detail["grandTotal"])
	}
	if _, hasPDF := detail["pdfData"]; hasPDF {
		t.Error("detail exposes raw PDF bytes")
	}
	if _, hasPDF := detail["PDFData"]; hasPDF {
		t.Error("detail exposes raw PDF bytes (capitalized)")
	}
	snapshot, _ := detail["htmlSnapshot"].(string)
	if !strings.Contains(snapshot, "261.000") {
		t.Error("detail HTML snapshot missing customer grand total")
	}
	for _, forbidden := range []string{"NettProfit", "TaxAmount", "MarginPercent", "nett profit"} {
		if strings.Contains(snapshot, forbidden) {
			t.Errorf("customer snapshot leaks %q", forbidden)
		}
	}
}

func TestDeleteRequiresAdmin(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_delete")

	code, exported := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput())
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201", code)
	}
	id, _ := exported["id"].(float64)
	quotePath := "/v1/quotations/" + strconv.Itoa(int(id))

	staffToken := staffLogin(t, server, "vera_api_quote_delete")

	code, _ = doJSON(t, server, http.MethodDelete, quotePath, staffToken, nil)
	if code != http.StatusForbidden {
		t.Fatalf("staff delete = %d; want 403", code)
	}

	code, _ = doJSON(t, server, http.MethodDelete, quotePath, token, nil)
	if code != http.StatusNoContent {
		t.Fatalf("admin delete = %d; want 204", code)
	}
	code, _ = doJSON(t, server, http.MethodGet, quotePath, token, nil)
	if code != http.StatusNotFound {
		t.Fatalf("get after delete = %d; want 404", code)
	}
}

// staffLogin creates a staff user directly in the test schema and returns a
// bearer token for it.
func staffLogin(t *testing.T, server *Server, schema string) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	ctx := context.Background()
	database, err := store.Open(ctx, store.Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	hash, err := auth.HashPassword("staff-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := database.CreateUser(ctx, "api-staff@example.com", "API Staff", hash, "staff"); err != nil {
		t.Fatalf("create staff: %v", err)
	}
	service := auth.NewService(database)
	token, _, err := service.Login(ctx, "api-staff@example.com", "staff-password")
	if err != nil {
		t.Fatalf("staff login: %v", err)
	}
	return token
}

func TestAttachDocumentAfterExport(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_quote_document")

	code, exported := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, quotationInput())
	if code != http.StatusCreated {
		t.Fatalf("export = %d; want 201", code)
	}
	id, _ := exported["id"].(float64)
	quotePath := "/v1/quotations/" + strconv.Itoa(int(id))

	pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 drill"))
	code, attached := doJSON(t, server, http.MethodPut, quotePath+"/revisions/0/document", token,
		map[string]any{"pdfBase64": pdf, "pdfPath": "/tmp/drill.pdf"})
	if code != http.StatusOK {
		t.Fatalf("attach = %d; want 200 (%v)", code, attached)
	}
	if attached["hasPdf"] != true {
		t.Errorf("attach response = %v; want hasPdf true", attached)
	}
}

func TestNextSequencePreviewsWithoutConsuming(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_next_seq")
	seedQuotationClient(t, database, 2, "Test Customer")
	seedQuotationClient(t, database, 4, "Test Customer Four")

	wantNumber := backend.FormatQuoteNumber(2, uint8(store.CurrentYY()), 1)
	code, body := doAdminRequest(t, server, token, http.MethodGet, "/v1/quotations/next-sequence?customerId=2", "")
	if code != http.StatusOK {
		t.Fatalf("next-sequence = %d; want 200", code)
	}
	if body["number"] != wantNumber {
		t.Fatalf("number = %v; want %s on a fresh database", body["number"], wantNumber)
	}
	if warn, _ := body["warn"].(bool); warn {
		t.Errorf("warn = true; want false on a fresh cycle")
	}

	// A second preview observes the same number: nothing was consumed.
	code, again := doAdminRequest(t, server, token, http.MethodGet, "/v1/quotations/next-sequence?customerId=2", "")
	if code != http.StatusOK {
		t.Fatalf("second next-sequence = %d; want 200", code)
	}
	if again["number"] != body["number"] {
		t.Errorf("second preview = %v; want stable %v", again["number"], body["number"])
	}

	// Peek is per-customer: another customer starts at its own 1.
	code, other := doAdminRequest(t, server, token, http.MethodGet, "/v1/quotations/next-sequence?customerId=4", "")
	if code != http.StatusOK {
		t.Fatalf("other customer next-sequence = %d; want 200", code)
	}
	if want := backend.FormatQuoteNumber(4, uint8(store.CurrentYY()), 1); other["number"] != want {
		t.Errorf("other customer number = %v; want %s", other["number"], want)
	}

	for _, path := range []string{
		"/v1/quotations/next-sequence",
		"/v1/quotations/next-sequence?customerId=99",
		"/v1/quotations/next-sequence?customerId=abc",
	} {
		if code, _ := doAdminRequest(t, server, token, http.MethodGet, path, ""); code != http.StatusBadRequest {
			t.Errorf("GET %s = %d; want 400", path, code)
		}
	}

	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/quotations/next-sequence?customerId=2", ""); code != http.StatusUnauthorized {
		t.Errorf("anonymous next-sequence = %d; want 401", code)
	}
}
