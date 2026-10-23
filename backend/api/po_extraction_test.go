package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vera-app/backend/apicontract"
	"vera-app/backend/extract"
)

// Issue #477: the authenticated extraction surface.
//
// The real server and disposable Postgres are used throughout; only the
// restricted engine is replaced by a stub, so the default suite needs no
// Poppler and no Docker. The tests hold the promises the UI depends on: a
// start needs an attached PDF, progress is readable, a replaced PDF discards
// the result instead of applying it, failures are typed, and no path writes a
// purchase-order field.

type stubExtractionEngine struct {
	err     error
	output  apicontract.ExtractionOutput
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (stub *stubExtractionEngine) Extract(ctx context.Context, pdf []byte) (apicontract.ExtractionOutput, error) {
	if stub.started != nil {
		stub.once.Do(func() { close(stub.started) })
	}
	if stub.release != nil {
		select {
		case <-stub.release:
		case <-ctx.Done():
			return apicontract.ExtractionOutput{}, ctx.Err()
		}
	}
	if stub.err != nil {
		return apicontract.ExtractionOutput{}, stub.err
	}
	return stub.output, nil
}

func extractionPage() *int {
	page := 1
	return &page
}

func extractionFixture() apicontract.ExtractionOutput {
	return apicontract.ExtractionOutput{
		SchemaVersion:    apicontract.ExtractionSchemaVersion,
		Engine:           "stub",
		EngineVersion:    "1.0-test",
		PageCount:        1,
		PagesWithText:    1,
		PagesWithoutText: 0,
		Pages:            []apicontract.ExtractionPageInfo{{Number: 1, Width: 595, Height: 842}},
		PageModes:        []apicontract.ExtractionPageMode{{Number: 1, Mode: apicontract.ExtractionPageModeText, TextWords: 12, TextChars: 120}},
		DurationMs:       9,
		Fields: []apicontract.ExtractionField{
			{Key: apicontract.ExtractionFieldPONumber, ExtractionCell: apicontract.ExtractionCell{
				Value: "4500123456", Raw: "4500123456", Status: apicontract.ExtractionStatusExtracted,
				Page: extractionPage(), BBox: &apicontract.ExtractionBBox{X: 86, Y: 82, Width: 56, Height: 9},
			}},
			{Key: apicontract.ExtractionFieldPODate, ExtractionCell: apicontract.ExtractionCell{
				Value: "2026-10-12", Raw: "2026-10-12", Status: apicontract.ExtractionStatusExtracted,
				Page: extractionPage(), BBox: &apicontract.ExtractionBBox{X: 95, Y: 97, Width: 50, Height: 9},
			}},
			{Key: apicontract.ExtractionFieldRequestedDeliveryDate, ExtractionCell: apicontract.ExtractionCell{
				Status: apicontract.ExtractionStatusAbsent,
			}},
			{Key: apicontract.ExtractionFieldCurrency, ExtractionCell: apicontract.ExtractionCell{
				Value: "USD", Raw: "USD", Status: apicontract.ExtractionStatusExtracted,
				Page: extractionPage(),
			}},
			{Key: apicontract.ExtractionFieldPaymentTerms, ExtractionCell: apicontract.ExtractionCell{
				Value: "30 days after delivery", Raw: "30 days after delivery",
				Status: apicontract.ExtractionStatusExtracted, Page: extractionPage(),
			}},
			{Key: apicontract.ExtractionFieldIncoterms, ExtractionCell: apicontract.ExtractionCell{
				Value: "CIF Batam", Raw: "CIF Batam", Status: apicontract.ExtractionStatusExtracted,
				Page: extractionPage(),
			}},
		},
		Rows: []apicontract.ExtractionRow{
			{
				LineNumber:  apicontract.ExtractionCell{Value: "1", Status: apicontract.ExtractionStatusExtracted, Page: extractionPage()},
				Description: apicontract.ExtractionCell{Value: "Electrical panel", Status: apicontract.ExtractionStatusExtracted, Page: extractionPage()},
				Quantity:    apicontract.ExtractionCell{Value: "2", Status: apicontract.ExtractionStatusExtracted, Page: extractionPage()},
				UoM:         apicontract.ExtractionCell{Value: "Pc", Status: apicontract.ExtractionStatusExtracted, Page: extractionPage()},
				UnitPrice:   apicontract.ExtractionCell{Value: "1250.00", Status: apicontract.ExtractionStatusExtracted, Page: extractionPage()},
				Amount:      apicontract.ExtractionCell{Value: "2500.00", Status: apicontract.ExtractionStatusExtracted, Page: extractionPage()},
				Status:      apicontract.ExtractionStatusExtracted,
				Page:        1,
				BBox:        &apicontract.ExtractionBBox{X: 50, Y: 225, Width: 490, Height: 10},
			},
		},
	}
}

func resultPresent(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func extractionState(t *testing.T, body map[string]any) apicontract.PurchaseOrderExtractionStateRow {
	t.Helper()
	var state apicontract.PurchaseOrderExtractionStateRow
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := json.Unmarshal(encoded, &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return state
}

func waitForExtraction(t *testing.T, server *Server, token string, id int64) apicontract.PurchaseOrderExtractionStateRow {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		code, body := doJSON(t, server, http.MethodGet,
			"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/extractions/latest", token, nil)
		if code != http.StatusOK {
			t.Fatalf("latest = %d; want 200 (%v)", code, body)
		}
		state := extractionState(t, body)
		if state.Run != nil {
			switch state.Run.Status {
			case "succeeded", "failed", "stale":
				return state
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("extraction did not reach a terminal state: %+v", state.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func startExtraction(t *testing.T, server *Server, token string, id int64) (int, map[string]any) {
	t.Helper()
	return doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(id, 10)+"/extractions", token, nil)
}

func createPODraftWithPDF(t *testing.T, server *Server, token string, quotationID int64, schema string) purchaseOrderRow {
	t.Helper()
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	code, body = uploadPurchaseOrderDocument(t, server, token, draft.ID, "Customer PO.pdf", documentTestPDF(schema))
	if code != http.StatusCreated {
		t.Fatalf("upload document = %d (%v)", code, body)
	}
	return draft
}

func TestExtractionStartsFromTheAttachedPDFAndNeverWritesTheDraft(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_flow")
	server.SetExtractor(&stubExtractionEngine{output: extractionFixture()})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "flow")

	beforeContext, err := server.organizationContext(context.Background())
	if err != nil {
		t.Fatalf("organization context: %v", err)
	}
	before, err := server.database.GetPurchaseOrder(beforeContext, draft.ID)
	if err != nil {
		t.Fatalf("read draft before: %v", err)
	}

	code, body := startExtraction(t, server, token, draft.ID)
	if code != http.StatusAccepted {
		t.Fatalf("start = %d; want 202 (%v)", code, body)
	}
	state := waitForExtraction(t, server, token, draft.ID)
	if state.Run == nil || state.Run.Status != "succeeded" {
		t.Fatalf("run = %+v; want succeeded", state.Run)
	}
	if state.Run.Engine != "stub" || state.Run.EngineVersion != "1.0-test" || state.Run.PageCount != 1 {
		t.Errorf("run envelope = %+v", state.Run)
	}
	if state.Document == nil || state.Document.SHA256 != state.Run.DocumentSHA256 {
		t.Errorf("document generation = %+v; run = %+v", state.Document, state.Run)
	}
	if !resultPresent(state.Run.Result) {
		t.Fatal("succeeded run has no result")
	}
	var result apicontract.ExtractionOutput
	if err := json.Unmarshal(state.Run.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Fields) != 6 || len(result.Rows) != 1 {
		t.Errorf("result = %+v", result)
	}
	if result.Fields[0].Page == nil || *result.Fields[0].Page != 1 || result.Fields[0].BBox == nil {
		t.Errorf("poNumber evidence = %+v", result.Fields[0])
	}

	afterContext, err := server.organizationContext(context.Background())
	if err != nil {
		t.Fatalf("organization context: %v", err)
	}
	after, err := server.database.GetPurchaseOrder(afterContext, draft.ID)
	if err != nil {
		t.Fatalf("read draft after: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("extraction wrote the purchase order\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func TestExtractionRequiresAnAttachedPDFAndReportsNoRun(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_no_pdf")
	server.SetExtractor(&stubExtractionEngine{output: extractionFixture()})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)

	code, body = startExtraction(t, server, token, draft.ID)
	if code != http.StatusConflict || body["code"] != "po_extraction_document_required" {
		t.Fatalf("start without PDF = %d %v; want 409 document required", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/extractions/latest", token, nil)
	if code != http.StatusOK {
		t.Fatalf("latest = %d; want 200", code)
	}
	state := extractionState(t, body)
	if state.Run != nil {
		t.Errorf("run = %+v; want null before the first extraction", state.Run)
	}
	if state.Document != nil {
		t.Errorf("document = %+v; want null when no PDF is attached", state.Document)
	}
}

func TestExtractionResultIsDiscardedWhenThePDFIsReplaced(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_stale")
	server.SetExtractor(&stubExtractionEngine{output: extractionFixture()})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "stale")

	if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
		t.Fatalf("start = %d (%v)", code, body)
	}
	first := waitForExtraction(t, server, token, draft.ID)
	if first.Run == nil || first.Run.Status != "succeeded" {
		t.Fatalf("first run = %+v", first.Run)
	}

	code, body := uploadPurchaseOrderDocument(t, server, token, draft.ID, "Replacement.pdf",
		documentTestPDF("replacement"))
	if code != http.StatusCreated {
		t.Fatalf("replace = %d (%v)", code, body)
	}
	stale := waitForExtraction(t, server, token, draft.ID)
	if stale.Run == nil || stale.Run.Status != "stale" {
		t.Fatalf("run after replacement = %+v; want stale", stale.Run)
	}
	if resultPresent(stale.Run.Result) {
		t.Error("a stale run still returned proposals from the replaced PDF")
	}
	if stale.Run.ErrorCode == nil || *stale.Run.ErrorCode != "document_changed" {
		t.Errorf("error code = %v; want document_changed", stale.Run.ErrorCode)
	}

	code, body = startExtraction(t, server, token, draft.ID)
	if code != http.StatusAccepted {
		t.Fatalf("restart = %d (%v)", code, body)
	}
	second := waitForExtraction(t, server, token, draft.ID)
	if second.Run == nil || second.Run.ID == first.Run.ID {
		t.Fatalf("second run = %+v; want a new run for the replacement", second.Run)
	}
	if second.Run.Status != "succeeded" || second.Run.DocumentSHA256 == first.Run.DocumentSHA256 {
		t.Errorf("second run = %+v; want success on the replacement generation", second.Run)
	}
}

func TestExtractionIsUnavailableUntilAnIsolatedExtractorIsConfigured(t *testing.T) {
	// The default server configuration has no extractor. A same-UID child in
	// the server container is not isolation (it can read the parent's /proc
	// environment), so failing closed is the only safe default.
	t.Setenv("VERA_EXTRACTOR_CMD", "")
	server, token := testQuotationServer(t, "vera_api_po_extract_unconfigured")
	if server.extractions.Engine != nil {
		t.Fatalf("default server configured an extraction engine: %+v", server.extractions.Engine)
	}
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "unconfigured")

	code, body := startExtraction(t, server, token, draft.ID)
	if code != http.StatusServiceUnavailable || body["code"] != "po_extraction_unavailable" {
		t.Fatalf("start without a configured extractor = %d %v; want 503 po_extraction_unavailable", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/extractions/latest", token, nil)
	if code != http.StatusOK {
		t.Fatalf("latest = %d; want 200 (%v)", code, body)
	}
	state := extractionState(t, body)
	if state.Run != nil {
		t.Errorf("a refused start queued run %+v; the operator must configure isolation first", state.Run)
	}
}

func TestExtractionStartRefusesAnUnavailableCommandBeforeQueueing(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_probe")
	server.SetExtractor(&extract.CommandEngine{
		Command: []string{"vera-extractor-that-does-not-exist"},
	})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "probe")

	code, body := startExtraction(t, server, token, draft.ID)
	if code != http.StatusServiceUnavailable || body["code"] != "po_extraction_unavailable" {
		t.Fatalf("start with a missing extractor = %d %v; want 503 po_extraction_unavailable", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/extractions/latest", token, nil)
	if code != http.StatusOK {
		t.Fatalf("latest = %d; want 200 (%v)", code, body)
	}
	if state := extractionState(t, body); state.Run != nil {
		t.Errorf("a preflight refusal queued run %+v; it must store nothing", state.Run)
	}

	// The operator opts in explicitly. Without this the endpoint stays 503;
	// with it, the normal 202 path and typed run failures are unchanged.
	server.SetExtractor(&stubExtractionEngine{output: extractionFixture()})
	if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
		t.Fatalf("start after an explicit engine = %d (%v); want 202", code, body)
	}
	if state := waitForExtraction(t, server, token, draft.ID); state.Run == nil || state.Run.Status != "succeeded" {
		t.Fatalf("run after an explicit engine = %+v; want succeeded", state.Run)
	}
}

func TestExtractionFailuresAreTypedAndKeepTheManualPath(t *testing.T) {
	cases := []struct {
		name string
		run  *stubExtractionEngine
		want string
	}{
		{"page limit", &stubExtractionEngine{err: extract.ErrPageLimit}, "page_limit"},
		{"ocr page limit", &stubExtractionEngine{err: extract.ErrOCRLimit}, "ocr_page_limit"},
		{"page too large", &stubExtractionEngine{err: extract.ErrPixelLimit}, "page_too_large"},
		{"ocr unavailable", &stubExtractionEngine{err: extract.ErrOCREngine}, "ocr_unavailable"},
		{"invalid output", &stubExtractionEngine{output: apicontract.ExtractionOutput{}}, "invalid_output"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			schema := "vera_api_po_extract_fail_" + strings.ReplaceAll(testCase.name, " ", "_")
			server, token := testQuotationServer(t, schema)
			server.SetExtractor(testCase.run)
			quotationID, _ := exportForSelector(t, server, token, quotationInput())
			draft := createPODraftWithPDF(t, server, token, quotationID, "failure")

			if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
				t.Fatalf("start = %d (%v)", code, body)
			}
			state := waitForExtraction(t, server, token, draft.ID)
			if state.Run == nil || state.Run.Status != "failed" {
				t.Fatalf("run = %+v; want failed", state.Run)
			}
			if state.Run.ErrorCode == nil || *state.Run.ErrorCode != testCase.want {
				t.Errorf("error code = %v; want %s", state.Run.ErrorCode, testCase.want)
			}
			if resultPresent(state.Run.Result) {
				t.Error("a failed run carried a result")
			}
			if state.Document == nil {
				t.Error("the failure removed the attached PDF; the manual path must remain")
			}
		})
	}
}

func TestExtractionTimeoutIsTyped(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_timeout")
	server.SetExtractor(&stubExtractionEngine{
		release: make(chan struct{}),
		output:  extractionFixture(),
	})
	server.extractions.Timeout = 200 * time.Millisecond
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "timeout")

	if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
		t.Fatalf("start = %d (%v)", code, body)
	}
	state := waitForExtraction(t, server, token, draft.ID)
	if state.Run == nil || state.Run.Status != "failed" ||
		state.Run.ErrorCode == nil || *state.Run.ErrorCode != "timeout" {
		t.Fatalf("run = %+v; want a typed timeout", state.Run)
	}
}

func TestExtractionStartIsIdempotentWhileTheRunIsActive(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_active")
	release := make(chan struct{})
	server.SetExtractor(&stubExtractionEngine{
		started: make(chan struct{}),
		release: release,
		output:  extractionFixture(),
	})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "active")

	code, body := startExtraction(t, server, token, draft.ID)
	if code != http.StatusAccepted {
		t.Fatalf("first start = %d (%v)", code, body)
	}
	first := extractionState(t, body)
	<-server.extractions.Engine.(*stubExtractionEngine).started

	code, body = startExtraction(t, server, token, draft.ID)
	if code != http.StatusAccepted {
		t.Fatalf("second start = %d (%v)", code, body)
	}
	second := extractionState(t, body)
	if first.Run == nil || second.Run == nil || first.Run.ID != second.Run.ID {
		t.Fatalf("second start = %+v; want the active run %+v", second.Run, first.Run)
	}
	close(release)
	waitForExtraction(t, server, token, draft.ID)
}

func TestInterruptedExtractionFailsAfterRecovery(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_interrupted")
	release := make(chan struct{})
	server.SetExtractor(&stubExtractionEngine{
		started: make(chan struct{}),
		release: release,
		output:  extractionFixture(),
	})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "interrupted")
	if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
		t.Fatalf("start = %d (%v)", code, body)
	}
	<-server.extractions.Engine.(*stubExtractionEngine).started

	if count, err := server.extractions.Recover(context.Background()); err != nil || count != 1 {
		t.Fatalf("recover = %d, %v; want 1", count, err)
	}
	state := waitForExtraction(t, server, token, draft.ID)
	if state.Run == nil || state.Run.Status != "failed" ||
		state.Run.ErrorCode == nil || *state.Run.ErrorCode != "interrupted" {
		t.Fatalf("run = %+v; want interrupted failure", state.Run)
	}
	close(release)
	time.Sleep(50 * time.Millisecond)
	after := waitForExtraction(t, server, token, draft.ID)
	if after.Run == nil || after.Run.ErrorCode == nil || *after.Run.ErrorCode != "interrupted" {
		t.Errorf("late worker overwrote the interrupted state: %+v", after.Run)
	}
}

func TestExtractionRequiresASessionAndAKnownRecord(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_auth")
	server.SetExtractor(&stubExtractionEngine{output: extractionFixture()})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "auth")

	code, _ := doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/extractions", "", nil)
	if code != http.StatusUnauthorized {
		t.Errorf("start without a session = %d; want 401", code)
	}
	code, _ = startExtraction(t, server, token, draft.ID+9999)
	if code != http.StatusNotFound {
		t.Errorf("start on an unknown record = %d; want 404", code)
	}
}

// Issue #478: a mixed document stores its per-page OCR evidence in the run
// result, so the review panel can say which pages were OCRed and which values
// are below the confidence floor.
func extractionOCRFixure() apicontract.ExtractionOutput {
	output := extractionFixture()
	output.PageCount = 2
	output.Pages = append(output.Pages, apicontract.ExtractionPageInfo{Number: 2, Width: 595, Height: 842})
	meanConfidence := 88.0
	output.PageModes = append(output.PageModes, apicontract.ExtractionPageMode{
		Number: 2, Mode: apicontract.ExtractionPageModeOCR,
		TextWords: 3, TextChars: 14, OCRWords: 20, DPI: 300,
		Confidence: &meanConfidence, Reason: "below the text floor",
	})
	output.PagesWithoutText = 1
	output.PagesWithOCR = 1
	output.OCREngine = "tesseract"
	output.OCREngineVersion = "5.5.0-test"
	quantityConfidence := 55.0
	page := 2
	output.Rows = append(output.Rows, apicontract.ExtractionRow{
		LineNumber:  apicontract.ExtractionCell{Value: "2", Raw: "2", Status: apicontract.ExtractionStatusExtracted, Page: &page},
		Description: apicontract.ExtractionCell{Value: "Cable termination", Raw: "Cable termination", Status: apicontract.ExtractionStatusExtracted, Page: &page},
		Quantity: apicontract.ExtractionCell{Value: "10.5", Raw: "10.5", Status: apicontract.ExtractionStatusNeedsReview,
			Page: &page, Confidence: &quantityConfidence, Note: "low OCR confidence"},
		UoM:       apicontract.ExtractionCell{Value: "M", Raw: "M", Status: apicontract.ExtractionStatusExtracted, Page: &page},
		UnitPrice: apicontract.ExtractionCell{Value: "42.00", Raw: "42.00", Status: apicontract.ExtractionStatusExtracted, Page: &page},
		Amount:    apicontract.ExtractionCell{Value: "441.00", Raw: "441.00", Status: apicontract.ExtractionStatusExtracted, Page: &page},
		Status:    apicontract.ExtractionStatusNeedsReview,
		Page:      2,
		Note:      "low OCR confidence",
	})
	return output
}

func TestExtractionStoresPageModesAndOCRProvenance(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_ocr")
	server.SetExtractor(&stubExtractionEngine{output: extractionOCRFixure()})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	draft := createPODraftWithPDF(t, server, token, quotationID, "ocr")

	if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
		t.Fatalf("start = %d (%v)", code, body)
	}
	state := waitForExtraction(t, server, token, draft.ID)
	if state.Run == nil || state.Run.Status != "succeeded" {
		t.Fatalf("run = %+v; want succeeded", state.Run)
	}
	var result apicontract.ExtractionOutput
	if err := json.Unmarshal(state.Run.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.OCREngine != "tesseract" || result.OCREngineVersion != "5.5.0-test" {
		t.Errorf("OCR engine = %q/%q; want the pinned engine", result.OCREngine, result.OCREngineVersion)
	}
	if result.PagesWithOCR != 1 || result.PagesWithoutText != 1 || len(result.PageModes) != 2 {
		t.Fatalf("page evidence = %+v", result)
	}
	if result.PageModes[1].Mode != apicontract.ExtractionPageModeOCR || result.PageModes[1].DPI != 300 {
		t.Errorf("OCR page mode = %+v", result.PageModes[1])
	}
	if len(result.Rows) != 2 || result.Rows[1].Quantity.Confidence == nil ||
		*result.Rows[1].Quantity.Confidence != 55 {
		t.Errorf("OCR row confidence = %+v; want the measured 55", result.Rows[1].Quantity)
	}
	if result.Rows[1].Status != apicontract.ExtractionStatusNeedsReview {
		t.Errorf("OCR row status = %q; want needs_review", result.Rows[1].Status)
	}
}
