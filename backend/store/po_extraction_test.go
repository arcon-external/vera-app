package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #477: extraction runs are evidence about one *generation* of the one
// customer PO PDF. These tests drive the real store against a disposable
// schema: the run records the document, version and SHA it read; a second
// start while one is active is idempotent; a replaced PDF discards the result
// instead of serving proposals from the wrong file; and a restart fails an
// interrupted run cleanly rather than leaving it "running" forever.

func extractionResultJSON(t *testing.T) []byte {
	t.Helper()
	page := 1
	output := apicontract.ExtractionOutput{
		SchemaVersion: apicontract.ExtractionSchemaVersion,
		Engine:        "stub",
		EngineVersion: "1.0",
		PageCount:     1,
		PagesWithText: 1,
		Pages:         []apicontract.ExtractionPageInfo{{Number: 1, Width: 595, Height: 842}},
		Fields:        make([]apicontract.ExtractionField, 0, len(apicontract.ExtractionFieldKeys)),
		Rows:          []apicontract.ExtractionRow{},
		DurationMs:    12,
	}
	for _, key := range apicontract.ExtractionFieldKeys {
		cell := apicontract.ExtractionCell{Status: apicontract.ExtractionStatusAbsent}
		if key == apicontract.ExtractionFieldPONumber {
			cell = apicontract.ExtractionCell{
				Value: "4500123456", Raw: "4500123456",
				Status: apicontract.ExtractionStatusExtracted, Page: &page,
			}
		}
		output.Fields = append(output.Fields, apicontract.ExtractionField{Key: key, ExtractionCell: cell})
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return encoded
}

func createExtractionRun(t *testing.T, database *Store, ctx context.Context, purchaseOrderID, actorID int64) PurchaseOrderExtraction {
	t.Helper()
	run, err := database.CreatePurchaseOrderExtraction(ctx, PurchaseOrderExtractionCreate{
		PurchaseOrderID: purchaseOrderID,
		RequestedBy:     actorID,
	})
	if err != nil {
		t.Fatalf("create extraction: %v", err)
	}
	return run
}

func finishExtractionSuccess(t *testing.T, database *Store, ctx context.Context, run PurchaseOrderExtraction) PurchaseOrderExtraction {
	t.Helper()
	finished, err := database.FinishPurchaseOrderExtraction(ctx, PurchaseOrderExtractionFinish{
		ID:            run.ID,
		Status:        PurchaseOrderExtractionStatusSucceeded,
		Engine:        "stub",
		EngineVersion: "1.0",
		PageCount:     1,
		Result:        extractionResultJSON(t),
		DurationMS:    34,
	})
	if err != nil {
		t.Fatalf("finish extraction: %v", err)
	}
	return finished
}

func TestCreateExtractionRequiresAnAttachedDocument(t *testing.T) {
	database := openTestSchema(t, "vera_po_extract_no_document")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "No document")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	_, err := database.CreatePurchaseOrderExtraction(ctx, PurchaseOrderExtractionCreate{
		PurchaseOrderID: draft.ID,
		RequestedBy:     user.ID,
	})
	if !errors.Is(err, ErrPurchaseOrderExtractionDocumentRequired) {
		t.Fatalf("error = %v; want document required", err)
	}
}

func TestCreateExtractionBindsTheDocumentGenerationAndIsIdempotent(t *testing.T) {
	database := openTestSchema(t, "vera_po_extract_create")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Create")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	document := attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	run := createExtractionRun(t, database, ctx, draft.ID, user.ID)
	if run.Status != PurchaseOrderExtractionStatusQueued {
		t.Errorf("status = %q; want queued", run.Status)
	}
	if run.DocumentID != document.ID || run.DocumentVersion != document.Version ||
		run.DocumentSHA256 != document.SHA256 {
		t.Errorf("run generation = %d/v%d/%s; want %d/v%d/%s",
			run.DocumentID, run.DocumentVersion, run.DocumentSHA256,
			document.ID, document.Version, document.SHA256)
	}
	if run.RequestedBy != user.ID || run.CreatedAt.IsZero() || run.StartedAt != nil {
		t.Errorf("run metadata = %+v", run)
	}

	again := createExtractionRun(t, database, ctx, draft.ID, user.ID)
	if again.ID != run.ID {
		t.Errorf("second start created run %d; want the active run %d", again.ID, run.ID)
	}
}

func TestExtractionLifecycleStoresTheResultAndReadsItBack(t *testing.T) {
	database := openTestSchema(t, "vera_po_extract_lifecycle")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lifecycle")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	run := createExtractionRun(t, database, ctx, draft.ID, user.ID)
	running, err := database.BeginPurchaseOrderExtraction(ctx, run.ID)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if running.Status != PurchaseOrderExtractionStatusRunning || running.StartedAt == nil {
		t.Errorf("running = %+v; want running with a start time", running)
	}
	finished := finishExtractionSuccess(t, database, ctx, run)
	if finished.Status != PurchaseOrderExtractionStatusSucceeded ||
		finished.FinishedAt == nil || finished.DurationMS == nil ||
		finished.Engine != "stub" || finished.EngineVersion != "1.0" ||
		finished.PageCount != 1 || len(finished.Result) == 0 {
		t.Errorf("finished = %+v; want a complete succeeded row", finished)
	}
	if finished.ErrorCode != nil || finished.ErrorMessage != nil {
		t.Errorf("succeeded row carries an error: %+v", finished)
	}

	state, err := database.LatestPurchaseOrderExtraction(ctx, draft.ID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if state.Run == nil || state.Run.ID != run.ID || state.Run.Status != PurchaseOrderExtractionStatusSucceeded {
		t.Errorf("latest run = %+v", state.Run)
	}
	if state.Document == nil || state.Document.SHA256 != finished.DocumentSHA256 {
		t.Errorf("latest document = %+v; want the same generation", state.Document)
	}
	if len(state.Run.Result) == 0 {
		t.Error("latest succeeded run lost its result")
	}
}

func TestFinishExtractionDiscardsTheResultWhenTheDocumentChanged(t *testing.T) {
	database := openTestSchema(t, "vera_po_extract_finish_stale")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Finish stale")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	run := createExtractionRun(t, database, ctx, draft.ID, user.ID)
	if _, err := database.BeginPurchaseOrderExtraction(ctx, run.ID); err != nil {
		t.Fatalf("begin: %v", err)
	}
	attachTestDocument(t, database, ctx, draft.ID, user,
		minimalPDF([]string{"<< /Type /Metadata /Label (replacement) >>"}, ""))

	finished := finishExtractionSuccess(t, database, ctx, run)
	if finished.Status != PurchaseOrderExtractionStatusStale {
		t.Fatalf("status = %q; want stale", finished.Status)
	}
	if finished.Result != nil {
		t.Error("a stale run kept its result; the proposals belong to the replaced PDF")
	}
	if finished.ErrorCode == nil || *finished.ErrorCode != PurchaseOrderExtractionErrorDocumentChanged {
		t.Errorf("error code = %v; want document_changed", finished.ErrorCode)
	}
}

func TestLatestExtractionMarksASuccessStaleAfterReplacement(t *testing.T) {
	database := openTestSchema(t, "vera_po_extract_read_stale")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Read stale")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	run := createExtractionRun(t, database, ctx, draft.ID, user.ID)
	if _, err := database.BeginPurchaseOrderExtraction(ctx, run.ID); err != nil {
		t.Fatalf("begin: %v", err)
	}
	finishExtractionSuccess(t, database, ctx, run)
	attachTestDocument(t, database, ctx, draft.ID, user,
		minimalPDF([]string{"<< /Type /Metadata /Label (v2) >>"}, ""))

	state, err := database.LatestPurchaseOrderExtraction(ctx, draft.ID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if state.Run == nil || state.Run.Status != PurchaseOrderExtractionStatusStale {
		t.Fatalf("run = %+v; want stale", state.Run)
	}
	if state.Run.Result != nil {
		t.Error("a stale read returned the discarded result")
	}

	// A new start is allowed once the old run is stale and is bound to the
	// replacement generation, never to the old one.
	replacement := createExtractionRun(t, database, ctx, draft.ID, user.ID)
	if replacement.ID == run.ID {
		t.Fatal("start after replacement reused the stale run")
	}
	if replacement.DocumentSHA256 == run.DocumentSHA256 {
		t.Error("replacement run is still bound to the replaced PDF")
	}
}

func TestRecoverInterruptedExtractionsFailsOnlyActiveRuns(t *testing.T) {
	database := openTestSchema(t, "vera_po_extract_recover")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recover")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	queued := createExtractionRun(t, database, ctx, draft.ID, user.ID)
	if _, err := database.BeginPurchaseOrderExtraction(ctx, queued.ID); err != nil {
		t.Fatalf("begin: %v", err)
	}

	// A second record with a completed run proves recovery leaves terminal
	// rows alone.
	secondQuotation := seedFinalizeQuotation(t, database, 3, "PT. Other", "Recover finished")
	secondDraft := createFinalizeDraft(t, database, ctx, user, secondQuotation)
	attachTestDocument(t, database, ctx, secondDraft.ID, user, minimalPDF(nil, ""))
	completed := createExtractionRun(t, database, ctx, secondDraft.ID, user.ID)
	if _, err := database.BeginPurchaseOrderExtraction(ctx, completed.ID); err != nil {
		t.Fatalf("begin completed: %v", err)
	}
	finishExtractionSuccess(t, database, ctx, completed)

	count, err := database.RecoverInterruptedPurchaseOrderExtractions(ctx)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if count != 1 {
		t.Errorf("recovered = %d; want 1", count)
	}
	state, err := database.LatestPurchaseOrderExtraction(ctx, draft.ID)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if state.Run == nil || state.Run.Status != PurchaseOrderExtractionStatusFailed {
		t.Fatalf("interrupted run = %+v; want failed", state.Run)
	}
	if state.Run.ErrorCode == nil || *state.Run.ErrorCode != PurchaseOrderExtractionErrorInterrupted {
		t.Errorf("error code = %v; want interrupted", state.Run.ErrorCode)
	}
	if state.Run.FinishedAt == nil || state.Run.DurationMS == nil {
		t.Error("interrupted run is not terminal: no finish time or duration")
	}
	finishedState, err := database.LatestPurchaseOrderExtraction(ctx, secondDraft.ID)
	if err != nil {
		t.Fatalf("latest completed: %v", err)
	}
	if finishedState.Run == nil || finishedState.Run.Status != PurchaseOrderExtractionStatusSucceeded {
		t.Errorf("completed run = %+v; recovery changed a terminal row", finishedState.Run)
	}
}

func TestExtractionOperationsRequireOrganizationContext(t *testing.T) {
	database := openTestSchema(t, "vera_po_extract_org")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Org")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	if _, err := database.CreatePurchaseOrderExtraction(context.Background(), PurchaseOrderExtractionCreate{
		PurchaseOrderID: draft.ID, RequestedBy: user.ID,
	}); err == nil {
		t.Fatal("create without an organization context succeeded")
	}
	if _, err := database.LatestPurchaseOrderExtraction(context.Background(), draft.ID); err == nil {
		t.Fatal("latest without an organization context succeeded")
	}
}
