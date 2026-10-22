package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Issue #496: authenticated HTTP coverage for PO-first drafts. These tests
// drive the real server and a disposable Postgres schema: a source-less draft
// is created, saved incomplete, reopened, listed under the existing Source
// missing filter, and refused by review/finalize at the server boundary. They
// also prove the editable pre-link basis and the frozen basis of a linked row.

func createBlankPODraft(t *testing.T, server *Server, token string, extra map[string]any) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"taxZone": "FTZ",
		"taxType": "Goods",
	}
	for key, value := range extra {
		body[key] = value
	}
	return doJSON(t, server, http.MethodPost, "/v1/purchase-orders", token, body)
}

func TestBlankPurchaseOrderDraftSavesAndReopensOverHTTP(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_first_draft")

	code, body := createBlankPODraft(t, server, token, nil)
	if code != http.StatusCreated {
		t.Fatalf("blank create = %d; want 201 (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	if draft.SourceState != "never" || !draft.SourceMissing || draft.PrintedTotalsKnown {
		t.Errorf("source state = %q missing %v printedKnown %v; want never/true/false",
			draft.SourceState, draft.SourceMissing, draft.PrintedTotalsKnown)
	}
	if draft.QuotationID != 0 || draft.CustomerID != 0 || draft.SourceRevision != 0 {
		t.Errorf("identity = %d/%d/%d; want numeric zeros with sourceState carrying presence",
			draft.QuotationID, draft.CustomerID, draft.SourceRevision)
	}
	if draft.QuoteNumber != "" || draft.CustomerName != "" {
		t.Errorf("fabricated identity = %q / %q", draft.QuoteNumber, draft.CustomerName)
	}
	if draft.Currency != "IDR" || draft.TaxZone != "FTZ" || draft.TaxType != "Goods" {
		t.Errorf("basis = %q/%q/%q; want company currency and supplied FTZ/Goods",
			draft.Currency, draft.TaxZone, draft.TaxType)
	}
	if len(draft.Items) != 0 {
		t.Errorf("lines = %d; a blank draft invents none", len(draft.Items))
	}

	code, body = doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"expectedVersion": draft.Version,
			"billingAddress":  "Jl. Entered before the source",
			"poNumber":        "CUSTOMER-PO-496",
			"receivedDate":    "2026-10-20",
			"paymentTerms":    "50% advance",
			"incoterms":       "FOB",
			"items": []map[string]any{{
				"description":    "Incomplete line",
				"quantityScaled": 15000,
				"uom":            "Pc",
				"unitPriceMinor": 12500,
			}},
		})
	if code != http.StatusOK {
		t.Fatalf("save blank draft = %d; want 200 (%v)", code, body)
	}
	saved := decodePurchaseOrder(t, body)
	if saved.PONumber != "CUSTOMER-PO-496" || saved.BillingAddress != "Jl. Entered before the source" ||
		len(saved.Items) != 1 || saved.Items[0].UnitPriceMinor != 12500 {
		t.Errorf("saved = %+v; want every typed value kept", saved)
	}
	if saved.SourceState != "never" || saved.CustomerID != 0 {
		t.Errorf("save invented a source/customer: %+v", saved)
	}

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen = %d; want 200 (%v)", code, body)
	}
	reopened := decodePurchaseOrder(t, body)
	if reopened.PONumber != saved.PONumber || len(reopened.Items) != 1 ||
		reopened.SourceState != "never" {
		t.Errorf("reopened = %+v; want the stored source-less draft", reopened)
	}

	// A second save updates the same row; the address confirms it landed.
	code, body = doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"expectedVersion": reopened.Version,
			"billingAddress":  "Jl. Second save",
			"poNumber":        reopened.PONumber,
			"receivedDate":    reopened.ReceivedDate,
			"items": []map[string]any{{
				"description":    "Incomplete line",
				"quantityScaled": 15000,
				"uom":            "Pc",
				"unitPriceMinor": 12500,
			}},
		})
	if code != http.StatusOK {
		t.Fatalf("second save = %d; want 200 (%v)", code, body)
	}
	if decodePurchaseOrder(t, body).ID != draft.ID {
		t.Error("the second save created another record")
	}
}

func TestBlankPurchaseOrderDraftRefusesReviewAndFinalizeOverHTTP(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_first_refuse")
	_, body := createBlankPODraft(t, server, token, nil)
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	code, body := doJSON(t, server, http.MethodGet, path+"/review", token, nil)
	if code != http.StatusConflict || body["code"] != "po_source_missing" {
		t.Fatalf("review = %d (%v); want 409 po_source_missing", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, map[string]any{
		"expectedVersion": draft.Version,
	})
	if code != http.StatusConflict || body["code"] != "po_source_missing" {
		t.Fatalf("finalize = %d (%v); want 409 po_source_missing", code, body)
	}
	// The server's wording is neutral: a never-linked draft was not deleted.
	if message, _ := body["error"].(string); message == "" ||
		strings.Contains(strings.ToLower(message), "deleted") {
		t.Errorf("refusal = %q; want neutral link guidance", message)
	}
}

func TestFormerlyLinkedDraftSavesButReviewAndFinalizeStillRefuseOverHTTP(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_first_former")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("linked create = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)

	preview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "record",
		"reason": "issued by mistake", "typedNumber": quoteNumber,
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("delete quotation = %d (%v)", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("read former draft = %d (%v)", code, body)
	}
	former := decodePurchaseOrder(t, body)
	if former.SourceState != "former" || !former.PrintedTotalsKnown {
		t.Fatalf("former state = %q printedKnown %v; want former/true",
			former.SourceState, former.PrintedTotalsKnown)
	}

	// #496 deliberately moved the old refusal: the draft still saves.
	code, body = doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"expectedVersion": former.Version,
			"billingAddress":  "Jl. Edited after deletion",
			"receivedDate":    former.ReceivedDate,
			"items": []map[string]any{{
				"description":    "line kept",
				"quantityScaled": 10000,
				"uom":            "Pc",
				"unitPriceMinor": 1,
			}},
		})
	if code != http.StatusOK {
		t.Fatalf("save former draft = %d (%v); want 200", code, body)
	}
	saved := decodePurchaseOrder(t, body)
	if saved.BillingAddress != "Jl. Edited after deletion" || saved.SourceState != "former" {
		t.Errorf("saved former draft = %+v; want the edited value and former state", saved)
	}

	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	code, body = doJSON(t, server, http.MethodGet, path+"/review", token, nil)
	if code != http.StatusConflict || body["code"] != "po_source_missing" {
		t.Errorf("former review = %d (%v); want 409 po_source_missing", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, map[string]any{
		"expectedVersion": saved.Version,
	})
	if code != http.StatusConflict || body["code"] != "po_source_missing" {
		t.Errorf("former finalize = %d (%v); want 409 po_source_missing", code, body)
	}
}

func TestBlankDraftEditsCurrencyAndTaxBasisBeforeLinking(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_first_basis")
	_, body := createBlankPODraft(t, server, token, nil)
	draft := decodePurchaseOrder(t, body)

	code, body := doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"expectedVersion": draft.Version,
			"currency":        "USD",
			"taxZone":         "NonFTZ",
			"taxType":         "Goods",
			"receivedDate":    draft.ReceivedDate,
			"items": []map[string]any{{
				"description":    "Priced line",
				"quantityScaled": 20000,
				"uom":            "Pc",
				"unitPriceMinor": 1250,
			}},
		})
	if code != http.StatusOK {
		t.Fatalf("edit basis = %d (%v)", code, body)
	}
	edited := decodePurchaseOrder(t, body)
	if edited.Currency != "USD" || edited.TaxZone != "NonFTZ" ||
		edited.TaxType != "Goods" || edited.PpnBps != 1100 {
		t.Errorf("edited basis = %s/%s/%s ppn %d; want USD/NonFTZ/Goods 1100",
			edited.Currency, edited.TaxZone, edited.TaxType, edited.PpnBps)
	}
	// The server computed PPN from the persisted basis; a client cannot send
	// the rate itself. A stray ppnBps is ignored and the persisted 1100 is
	// recomputed from the same basis.
	code, body = doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"expectedVersion": edited.Version,
			"ppnBps":          0,
			"receivedDate":    edited.ReceivedDate,
			"items": []map[string]any{{
				"description":    "Priced line again",
				"quantityScaled": 20000,
				"uom":            "Pc",
				"unitPriceMinor": 1250,
			}},
		})
	if code != http.StatusOK {
		t.Fatalf("stray client PPN = %d (%v); want 200 with the server rate", code, body)
	}
	recomputed := decodePurchaseOrder(t, body)
	if recomputed.PpnBps != 1100 || recomputed.PpnMinor == 0 {
		t.Errorf("PPN after a stray client bps = %d/%d; want the server's 1100 and a nonzero PPN",
			recomputed.PpnBps, recomputed.PpnMinor)
	}
}

func TestLinkedDraftRefusesAnEditedCurrencyOrTaxBasisOverHTTP(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_first_linked_frozen")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	_, body := createPODraft(t, server, token, quotationID, 0, nil)
	draft := decodePurchaseOrder(t, body)

	code, body := doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"expectedVersion": draft.Version,
			"taxZone":         "NonFTZ",
		})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("linked tax change = %d (%v); want 422", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("read linked draft = %d (%v)", code, body)
	}
	unchanged := decodePurchaseOrder(t, body)
	if unchanged.TaxZone != draft.TaxZone || unchanged.PpnBps != draft.PpnBps {
		t.Errorf("linked basis moved: %+v -> %+v", draft, unchanged)
	}
}

func TestPurchaseOrderListIncludesNeverAndFormerSourceMissing(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_first_list")
	_, body := createBlankPODraft(t, server, token, nil)
	never := decodePurchaseOrder(t, body)

	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("linked create = %d (%v)", code, body)
	}
	former := decodePurchaseOrder(t, body)
	preview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "record",
		"reason": "list coverage", "typedNumber": quoteNumber,
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("delete quotation = %d (%v)", code, body)
	}

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders?sourceMissing=only", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list source missing = %d (%v)", code, body)
	}
	rows, _ := body["purchaseOrders"].([]any)
	if len(rows) != 2 {
		t.Fatalf("source-missing rows = %d; want the never and former rows", len(rows))
	}
	states := map[float64]string{}
	for _, entry := range rows {
		row, _ := entry.(map[string]any)
		id, _ := row["id"].(float64)
		state, _ := row["sourceState"].(string)
		states[id] = state
	}
	if states[float64(never.ID)] != "never" {
		t.Errorf("never row state = %q; want never", states[float64(never.ID)])
	}
	if states[float64(former.ID)] != "former" {
		t.Errorf("former row state = %q; want former", states[float64(former.ID)])
	}
	customers, _ := body["customers"].([]any)
	for _, entry := range customers {
		customer, _ := entry.(map[string]any)
		if id, _ := customer["id"].(float64); id == 0 {
			t.Error("the facet invented a zero-id customer for the source-less row")
		}
	}

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders?sourceMissing=exclude", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list source present = %d (%v)", code, body)
	}
	if rows, _ := body["purchaseOrders"].([]any); len(rows) != 0 {
		t.Errorf("source-present rows = %d; want none", len(rows))
	}
}

func TestBlankDraftCreationRejectsARevisionWithoutAQuotation(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_first_bad_revision")
	code, body := doJSON(t, server, http.MethodPost, "/v1/purchase-orders", token, map[string]any{
		"revisionNumber": 1,
	})
	if code != http.StatusBadRequest {
		t.Errorf("revision without quotation = %d (%v); want 400", code, body)
	}
}
