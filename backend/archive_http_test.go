package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/apiclient"
	"vera-app/backend/apicontract"
)

// stubAPIClient points the package client at a stub server for the test.
func stubAPIClient(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	t.Setenv("VERA_NO_KEYRING", "1")
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	previous := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previous })
	if err := SetSessionToken("test-token"); err != nil {
		t.Fatalf("set test session: %v", err)
	}
	t.Cleanup(func() { _ = ClearSessionToken() })
}

func TestListQuotationsMapsContract(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/quotations" {
			t.Errorf("path = %s; want /v1/quotations", request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"quotations": []apicontract.QuotationSummaryRow{
				{ID: 3, QuoteNumber: "15020003", RevisionNumber: 2, CustomerName: "Acme",
					Subject: "Test", CreatedAt: "2026-09-12T00:00:00Z",
					GrandTotal: 261000, DisplayGrand: "261.000"},
			},
		})
	})

	q := newTestQuotation()
	rows, err := q.ListQuotations()
	if err != nil {
		t.Fatalf("ListQuotations failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(rows))
	}
	row := rows[0]
	if row.ID != 3 || row.QuoteNumber != "15020003" || row.RevisionNumber != 2 {
		t.Errorf("row = %+v; want id 3 number 15020003 rev 2", row)
	}
	if row.DisplayGrand != "261.000" || row.GrandTotal != 261000 {
		t.Errorf("row totals = %d/%q; want 261000/261.000", row.GrandTotal, row.DisplayGrand)
	}
}

func TestListQuotationsRequiresSignIn(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	_ = ClearSessionToken()
	q := newTestQuotation()
	if _, err := q.ListQuotations(); err == nil {
		t.Fatal("ListQuotations without a session succeeded; want denial")
	}
}

func TestGetQuotationMapsDetail(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(apicontract.QuotationDetail{
			ID: 5, QuoteNumber: "15020005", Revision: 1,
			AvailableRevisions:       []int16{1, 0},
			CustomerName:             "Acme",
			Subject:                  "Detail",
			RecipientName:            "Jane",
			Incoterm:                 "EXW",
			DiscountPercent:          10,
			TaxZone:                  "FTZ",
			OriginalGrand:            225000,
			MarginalizedGrand:        270000,
			GrandTotal:               270000,
			TaxAmount:                0,
			NettProfit:               45000,
			DisplayOriginalGrand:     "225.000",
			DisplayMarginalizedGrand: "270.000",
			DisplayTax:               "0",
			DisplayNett:              "45.000",
			Items: []apicontract.QuotationItemRow{
				{ItemNo: 1, Description: "Line", Qty: 1, UoM: "Lot",
					OriginalUnitPrice: 225000, MarginalizedUnitPrice: 270000,
					MarginalizedTotal: 270000, MarginEnabled: true, MarginPercent: 20,
					MarginAmount: 45000, DisplayMarginalizedTotal: "270.000",
					DisplayMarginAmount: "45.000"},
			},
			HTMLSnapshot: "<title>Quotation</title>",
			HasPDF:       true,
		})
	})

	q := newTestQuotation()
	detail, err := q.GetQuotationRevision(5, 1)
	if err != nil {
		t.Fatalf("GetQuotationRevision failed: %v", err)
	}
	if detail.QuoteNumber != "15020005" || detail.RevisionNumber != 1 {
		t.Errorf("detail = %q rev %d; want 15020005 rev 1", detail.QuoteNumber, detail.RevisionNumber)
	}
	if len(detail.AvailableRevisions) != 2 || detail.AvailableRevisions[0] != 1 {
		t.Errorf("revisions = %v; want [1 0]", detail.AvailableRevisions)
	}
	if detail.NettProfit != 45000 || detail.DisplayNett != "45.000" {
		t.Errorf("nett = %d/%q; want 45000/45.000", detail.NettProfit, detail.DisplayNett)
	}
	if len(detail.Items) != 1 || detail.Items[0].MarginalizedTotal != 270000 {
		t.Errorf("items = %+v; want one 270000 line", detail.Items)
	}
	if !strings.Contains(detail.HTMLSnapshot, "Quotation") {
		t.Error("HTML snapshot missing")
	}
}

// TestGetQuotationMapsEntryTerms pins the desktop leg of the copy contract
// (#485): the entry values the server emits for the revision reach the
// archived quotation the copy path reads -- the customer code the editor
// pastes by, the address snapshot, and the revision's own terms. Before the
// mapping these read as the zero values, which is why the editor opened
// every revision with no customer and blank terms.
func TestGetQuotationMapsEntryTerms(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(apicontract.QuotationDetail{
			ID: 8, QuoteNumber: "15020008", Revision: 0,
			AvailableRevisions: []int16{0},
			CustomerName:       "Acme",
			CustomerCode:       4,
			CustomerAddress:    "Jl. Klien 12",
			RecipientAddress:   "Gedung Penerima 3",
			ValidityDays:       45,
			PaymentTermDays:    60,
		})
	})

	q := newTestQuotation()
	detail, err := q.GetQuotationRevision(8, 0)
	if err != nil {
		t.Fatalf("GetQuotationRevision failed: %v", err)
	}
	if detail.CustomerCode != 4 {
		t.Errorf("customerCode = %d; want 4", detail.CustomerCode)
	}
	if detail.CustomerAddress != "Jl. Klien 12" {
		t.Errorf("customerAddress = %q; want the stored client address", detail.CustomerAddress)
	}
	if detail.RecipientAddress != "Gedung Penerima 3" {
		t.Errorf("recipientAddress = %q; want the stored recipient address", detail.RecipientAddress)
	}
	if detail.ValidityDays != 45 {
		t.Errorf("validityDays = %d; want 45", detail.ValidityDays)
	}
	if detail.PaymentTermDays != 60 {
		t.Errorf("paymentTermDays = %d; want 60", detail.PaymentTermDays)
	}
}

// TestGetQuotationCarriesCurrency pins the revision side of the B4 contract
// at the desktop mapping: the code the server emits for the revision reaches
// the archived quotation the editor compares against the live profile before
// it may be opened for editing.
func TestGetQuotationCarriesCurrency(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(apicontract.QuotationDetail{
			ID: 6, QuoteNumber: "15020006", Revision: 2, Currency: "USD",
			AvailableRevisions: []int16{2, 1, 0},
		})
	})

	q := newTestQuotation()
	detail, err := q.GetQuotationRevision(6, 2)
	if err != nil {
		t.Fatalf("GetQuotationRevision failed: %v", err)
	}
	if detail.Currency != "USD" {
		t.Errorf("revision currency = %q; want USD", detail.Currency)
	}
}

// TestGetQuotationWithoutCurrencyReadsDefault covers a revision that predates
// the column: the server normalises to the default it was signed as, and the
// mapping keeps that rather than an empty string beside IDR figures.
func TestGetQuotationWithoutCurrencyReadsDefault(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(apicontract.QuotationDetail{
			ID: 7, QuoteNumber: "15020007", Revision: 0,
			AvailableRevisions: []int16{0},
		})
	})

	q := newTestQuotation()
	detail, err := q.GetQuotationRevision(7, 0)
	if err != nil {
		t.Fatalf("GetQuotationRevision failed: %v", err)
	}
	if detail.Currency != "IDR" {
		t.Errorf("revision currency = %q; want the IDR default", detail.Currency)
	}
}

func TestExportPersistsUploadsAndWritesPDF(t *testing.T) {
	if os.Getenv("VERA_RUN_CHROME_TESTS") == "" {
		t.Skip("set VERA_RUN_CHROME_TESTS=1 to exercise the chromedp PDF build")
	}
	t.Setenv("VERA_NO_KEYRING", "1")

	var putPaths []string
	var putBodies []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/quotations/export", func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(apicontract.ExportResponse{
			ID: 11, QuoteNumber: "15020011", Revision: 0, GrandTotal: 90000,
		})
	})
	mux.HandleFunc("/v1/quotations/11/revisions/0/document", func(writer http.ResponseWriter, request *http.Request) {
		putPaths = append(putPaths, request.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		putBodies = append(putBodies, body)
		_ = json.NewEncoder(writer).Encode(map[string]any{"hasPdf": true})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	previous := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previous })
	if err := SetSessionToken("test-token"); err != nil {
		t.Fatalf("set test session: %v", err)
	}
	t.Cleanup(func() { _ = ClearSessionToken() })

	q := newTestQuotation()
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	if err := q.Prepare(gatedPrepareInput()); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	target := filepath.Join(t.TempDir(), "quotation.pdf")
	previousDialog := saveFileDialog
	var gotOptions runtime.SaveDialogOptions
	saveFileDialog = func(ctx context.Context, options runtime.SaveDialogOptions) (string, error) {
		gotOptions = options
		return target, nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	if err := q.Export(t.TempDir()); err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if gotOptions.DefaultDirectory != t.TempDir() {
		t.Errorf("dialog directory = %q; want the configured export folder", gotOptions.DefaultDirectory)
	}
	for _, dir := range []string{"   ", filepath.Join(t.TempDir(), "gone")} {
		if err := q.Export(dir); err != nil {
			t.Fatalf("Export(%q) failed: %v", dir, err)
		}
		if gotOptions.DefaultDirectory != "" {
			t.Errorf("Export(%q) dialog directory = %q; want empty (OS default)", dir, gotOptions.DefaultDirectory)
		}
	}
	if q.Data.QuoteNumber != "15020011" || q.Data.RevisionID != 0 {
		t.Errorf("number/revision = %q/%d; want 15020011/0", q.Data.QuoteNumber, q.Data.RevisionID)
	}
	if len(putPaths) != 1 {
		t.Fatalf("document PUTs = %d; want 1", len(putPaths))
	}
	pdfBase64, _ := putBodies[0]["pdfBase64"].(string)
	if pdfBase64 == "" {
		t.Error("uploaded document has no PDF bytes")
	}
	if putBodies[0]["pdfPath"] != target {
		t.Errorf("uploaded path = %v; want %s", putBodies[0]["pdfPath"], target)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("PDF file missing: %v", err)
	}
}
