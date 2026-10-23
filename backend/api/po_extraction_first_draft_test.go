package api

import (
	"context"
	"net/http"
	"strconv"
	"testing"
)

// Issue #498: extraction runs before a source exists. A stored Source missing
// draft with an attached PDF is extractable; applying its proposals still
// leaves the record source-less, creates no Customer, and never unlocks the
// finalize review. A replaced PDF discards the result as stale.
func TestSourceLessDraftExtractsBeforeLinkingAndStillCannotFinalize(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_extract_source_less")
	server.SetExtractor(&stubExtractionEngine{output: extractionFixture()})

	_, created := createBlankPODraft(t, server, token, nil)
	draft := decodePurchaseOrder(t, created)
	if code, body := uploadPurchaseOrderDocument(t, server, token, draft.ID,
		"Customer PO.pdf", documentTestPDF("source-less")); code != http.StatusCreated {
		t.Fatalf("upload = %d (%v); want 201", code, body)
	}

	orgContext, err := server.organizationContext(context.Background())
	if err != nil {
		t.Fatalf("organization context: %v", err)
	}
	clientsBefore, err := server.database.ListClients(orgContext)
	if err != nil {
		t.Fatalf("list clients before: %v", err)
	}

	if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
		t.Fatalf("start before linking = %d (%v); want 202", code, body)
	}
	state := waitForExtraction(t, server, token, draft.ID)
	if state.Run == nil || state.Run.Status != "succeeded" || !resultPresent(state.Run.Result) {
		t.Fatalf("run = %+v; want a succeeded result on the source-less document", state.Run)
	}
	if state.Document == nil || state.Document.SHA256 != state.Run.DocumentSHA256 {
		t.Errorf("document generation = %+v; run = %+v", state.Document, state.Run)
	}

	// Apply one proposal through the ordinary draft save: the exact path the
	// editor's Apply uses. It must not invent identity.
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	code, body := doJSON(t, server, http.MethodPut, path, token, map[string]any{
		"expectedVersion": draft.Version,
		"poNumber":        "4500123456",
		"items": []map[string]any{{
			"description":    "Electrical panel",
			"quantityScaled": 20000,
			"uom":            "Pc",
			"unitPriceMinor": 125000,
		}},
	})
	if code != http.StatusOK {
		t.Fatalf("save applied proposal = %d (%v); want 200", code, body)
	}
	saved := decodePurchaseOrder(t, body)
	if saved.PONumber != "4500123456" || len(saved.Items) != 1 {
		t.Errorf("saved = %+v; want the applied proposal stored", saved)
	}
	if saved.SourceState != "never" || saved.CustomerID != 0 || saved.CustomerName != "" ||
		saved.QuotationID != 0 || saved.SourceRevision != 0 {
		t.Errorf("extraction supplied identity: %+v", saved)
	}
	clientsAfter, err := server.database.ListClients(orgContext)
	if err != nil {
		t.Fatalf("list clients after: %v", err)
	}
	if len(clientsAfter) != len(clientsBefore) {
		t.Errorf("client records = %d after extraction; want %d (extraction cannot create a Customer)",
			len(clientsAfter), len(clientsBefore))
	}

	// The extraction review is not the finalize review, and it is not a source.
	if code, body := doJSON(t, server, http.MethodGet, path+"/review", token, nil); code != http.StatusConflict ||
		body["code"] != "po_source_missing" {
		t.Fatalf("review after extraction = %d (%v); want 409 po_source_missing", code, body)
	}
	if code, body := doJSON(t, server, http.MethodPost, path+"/finalize", token, map[string]any{
		"expectedVersion": saved.Version,
	}); code != http.StatusConflict || body["code"] != "po_source_missing" {
		t.Fatalf("finalize after extraction = %d (%v); want 409 po_source_missing", code, body)
	}

	// Replacing the PDF discards the stored proposals for the old generation.
	if code, body := uploadPurchaseOrderDocument(t, server, token, draft.ID,
		"Replacement.pdf", documentTestPDF("replacement")); code != http.StatusCreated {
		t.Fatalf("replace = %d (%v); want 201", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet, path+"/extractions/latest", token, nil)
	if code != http.StatusOK {
		t.Fatalf("latest after replacement = %d (%v); want 200", code, body)
	}
	replaced := extractionState(t, body)
	if replaced.Run == nil || replaced.Run.Status != "stale" {
		t.Errorf("run after replacement = %+v; want stale", replaced.Run)
	}
	if resultPresent(replaced.Run.Result) {
		t.Error("the replaced document still serves the old proposals")
	}
	if replaced.Document == nil || replaced.Document.SHA256 == state.Run.DocumentSHA256 {
		t.Errorf("active document after replacement = %+v; want a new generation", replaced.Document)
	}
}
