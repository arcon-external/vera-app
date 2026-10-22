package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
)

// Issue #480: the authenticated deletion surface. These tests drive the real
// server against disposable Postgres: the admin gate, the preview's exact
// connected records, the typed number and reason, the ten-letter chain
// challenge, the changed-chain 409 that forces renewal, and the surviving
// source-missing record.

type deletionPreviewBody struct {
	TargetType          string `json:"targetType"`
	TargetUUID          string `json:"targetUuid"`
	DocumentNumber      string `json:"documentNumber"`
	DocumentNumberLabel string `json:"documentNumberLabel"`
	QuoteNumber         string `json:"quoteNumber"`
	PONumber            string `json:"poNumber"`
	CustomerName        string `json:"customerName"`
	State               string `json:"state"`
	Fingerprint         string `json:"fingerprint"`
	Scopes              []struct {
		Scope             string `json:"scope"`
		Available         bool   `json:"available"`
		UnavailableReason string `json:"unavailableReason"`
		RequiresChallenge bool   `json:"requiresChallenge"`
		Removes           []struct {
			Type           string  `json:"type"`
			ID             int64   `json:"id"`
			Label          string  `json:"label"`
			Revisions      []int16 `json:"revisions"`
			Version        int64   `json:"version"`
			DocumentNumber string  `json:"documentNumber"`
		} `json:"removes"`
		Preserves []struct {
			Type  string `json:"type"`
			ID    int64  `json:"id"`
			Label string `json:"label"`
		} `json:"preserves"`
	} `json:"scopes"`
	TargetID      int64 `json:"targetId"`
	Version       int64 `json:"version"`
	SourceMissing bool  `json:"sourceMissing"`
}

func decodeDeletionPreview(t *testing.T, body map[string]any) deletionPreviewBody {
	t.Helper()
	var preview deletionPreviewBody
	encodeDecode(t, body, &preview)
	return preview
}

func deletionScope(t *testing.T, preview deletionPreviewBody, scope string) int {
	t.Helper()
	for index, plan := range preview.Scopes {
		if plan.Scope == scope {
			return index
		}
	}
	t.Fatalf("preview has no %q scope: %+v", scope, preview.Scopes)
	return -1
}

func previewDeletionOverAPI(t *testing.T, server *Server, token, targetType string, targetID int64) deletionPreviewBody {
	t.Helper()
	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions/preview", token, map[string]any{
		"targetType": targetType, "targetId": targetID,
	})
	if code != http.StatusOK {
		t.Fatalf("preview = %d (%v)", code, body)
	}
	return decodeDeletionPreview(t, body)
}

func challengeDeletionOverAPI(t *testing.T, server *Server, token, targetType string, targetID int64, scope, fingerprint string) (string, string) {
	t.Helper()
	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions/challenge", token, map[string]any{
		"targetType": targetType, "targetId": targetID, "scope": scope,
		"expectedFingerprint": fingerprint,
	})
	if code != http.StatusCreated {
		t.Fatalf("challenge = %d (%v)", code, body)
	}
	id, _ := body["challengeId"].(string)
	letters, _ := body["letters"].(string)
	if id == "" || letters == "" {
		t.Fatalf("challenge body = %v", body)
	}
	return id, letters
}

func TestDeletionEndpointsRefuseStaff(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_staff")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)
	seedStaff(t, server.database, "delete-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "delete-staff@example.com", "staff-password")

	code, _ := doJSON(t, server, http.MethodPost, "/v1/deletions/preview", staffToken, map[string]any{
		"targetType": "purchase_order", "targetId": saved.ID,
	})
	if code != http.StatusForbidden {
		t.Fatalf("staff preview = %d; want 403", code)
	}
	code, _ = doJSON(t, server, http.MethodPost, "/v1/deletions/challenge", staffToken, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "chain",
	})
	if code != http.StatusForbidden {
		t.Fatalf("staff challenge = %d; want 403", code)
	}
	code, _ = doJSON(t, server, http.MethodPost, "/v1/deletions", staffToken, map[string]any{
		"targetType": "purchase_order", "targetId": saved.ID, "scope": "record",
		"reason": "cleanup", "typedNumber": saved.PONumber,
		"expectedFingerprint": "v1",
	})
	if code != http.StatusForbidden {
		t.Fatalf("staff confirm = %d; want 403", code)
	}
	// The record is still there: a refused caller saw nothing and deleted
	// nothing.
	code, _ = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("purchase order after staff attempts = %d; want 200", code)
	}
}

func TestDeletePurchaseOrderRecordOverAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_record")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)

	preview := previewDeletionOverAPI(t, server, token, "purchase_order", saved.ID)
	if preview.DocumentNumber != saved.PONumber || preview.DocumentNumberLabel != "Customer PO number" {
		t.Errorf("preview number = %q (%s); want the customer number", preview.DocumentNumber, preview.DocumentNumberLabel)
	}
	record := deletionScope(t, preview, "record")
	if !preview.Scopes[record].Available || preview.Scopes[record].RequiresChallenge {
		t.Errorf("record scope = %+v; want available without a challenge", preview.Scopes[record])
	}
	if len(preview.Scopes[record].Removes) != 1 || preview.Scopes[record].Removes[0].Type != "purchase_order" {
		t.Errorf("record removes = %+v", preview.Scopes[record].Removes)
	}
	if len(preview.Scopes[record].Preserves) != 1 || preview.Scopes[record].Preserves[0].Type != "quotation" {
		t.Errorf("record preserves = %+v; want the quotation", preview.Scopes[record].Preserves)
	}

	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "purchase_order", "targetId": saved.ID, "scope": "record",
		"reason": "duplicate entry", "typedNumber": saved.PONumber,
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("delete record = %d (%v)", code, body)
	}
	removed, _ := body["removed"].([]any)
	if len(removed) != 1 {
		t.Errorf("removed = %v; want one record", body["removed"])
	}
	code, _ = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusNotFound {
		t.Fatalf("deleted purchase order = %d; want 404", code)
	}
	code, _ = doJSON(t, server, http.MethodGet,
		"/v1/quotations/"+strconv.FormatInt(quotationID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("quotation after record deletion = %d; want 200", code)
	}
}

func TestDeleteQuotationRecordOverAPILeavesThePOReadable(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_source_missing")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)

	preview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	record := deletionScope(t, preview, "record")
	if len(preview.Scopes[record].Preserves) != 1 || preview.Scopes[record].Preserves[0].ID != saved.ID {
		t.Fatalf("record preserves = %+v; want the surviving PO", preview.Scopes[record].Preserves)
	}
	chain := deletionScope(t, preview, "chain")
	if !preview.Scopes[chain].RequiresChallenge {
		t.Error("chain scope does not require the challenge")
	}
	if len(preview.Scopes[chain].Removes) != 2 {
		t.Errorf("chain removes = %+v; want quotation and PO", preview.Scopes[chain].Removes)
	}
	if len(preview.Scopes[record].Removes[0].Revisions) == 0 {
		t.Error("quotation removal lists no revisions")
	}

	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "record",
		"reason": "customer cancelled", "typedNumber": quoteNumber,
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("delete quotation record = %d (%v)", code, body)
	}
	code, _ = doJSON(t, server, http.MethodGet,
		"/v1/quotations/"+strconv.FormatInt(quotationID, 10), token, nil)
	if code != http.StatusNotFound {
		t.Fatalf("deleted quotation = %d; want 404", code)
	}

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("surviving purchase order = %d (%v); want 200", code, body)
	}
	surviving := decodePurchaseOrder(t, body)
	if !surviving.SourceMissing {
		t.Error("surviving purchase order is not marked source missing")
	}
	if surviving.QuoteNumber != quoteNumber || surviving.PONumber != saved.PONumber {
		t.Errorf("surviving values = %q / %q; want the former source and the PO number",
			surviving.QuoteNumber, surviving.PONumber)
	}
	if surviving.GrandTotalMinor != saved.GrandTotalMinor || len(surviving.Items) != len(saved.Items) {
		t.Errorf("surviving economics drifted: %+v", surviving)
	}

	code, body = doJSON(t, server, http.MethodGet, "/v1/purchase-orders?sourceMissing=only", token, nil)
	if code != http.StatusOK {
		t.Fatalf("source-missing list = %d (%v)", code, body)
	}
	rows, _ := body["purchaseOrders"].([]any)
	if len(rows) != 1 {
		t.Fatalf("sourceMissing=only rows = %v; want the surviving record", body["purchaseOrders"])
	}
}

func TestDeleteSourceMissingPOStopsWhenADocumentArrivesAfterThePreview(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_source_missing_late_document")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)

	// The Judge's reproduction: remove the source alone, preview the surviving
	// record, then upload a customer PDF before the confirmation.
	quotationPreview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "record",
		"reason": "customer cancelled", "typedNumber": quoteNumber,
		"expectedFingerprint": quotationPreview.Fingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("delete quotation record = %d (%v)", code, body)
	}

	preview := previewDeletionOverAPI(t, server, token, "purchase_order", saved.ID)
	code, body = uploadPurchaseOrderDocument(t, server, token, saved.ID,
		"arrived-after-preview.pdf", documentTestPDF("late"))
	if code != http.StatusCreated {
		t.Fatalf("upload after the preview = %d (%v)", code, body)
	}

	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "purchase_order", "targetId": saved.ID, "scope": "record",
		"reason": "delete the record", "typedNumber": saved.PONumber,
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusConflict || body["code"] != "deletion_chain_changed" {
		t.Fatalf("stale confirmation = %d (%v); want 409/deletion_chain_changed", code, body)
	}
	field := "purchase_order_" + strconv.FormatInt(saved.ID, 10) + "_documents"
	changes, _ := body["changes"].([]any)
	named := false
	for _, entry := range changes {
		change, _ := entry.(map[string]any)
		if change["field"] == field {
			named = true
			if change["after"] != "1" {
				t.Errorf("changed document count after = %v; want 1", change["after"])
			}
		}
	}
	if !named {
		t.Fatalf("changes = %v; want %s named", changes, field)
	}
	fresh, _ := body["preview"].(map[string]any)
	if fresh == nil || fresh["fingerprint"] == preview.Fingerprint {
		t.Fatalf("stale confirmation carries no fresh preview: %v", body["preview"])
	}

	// Nothing was deleted: the record and the unconfirmed document remain.
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("record after a refused confirmation = %d (%v); want 200", code, body)
	}
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10)+"/document", token, nil)
	if code != http.StatusOK {
		t.Fatalf("document after a refused confirmation = %d (%v); want 200", code, body)
	}

	// The fresh preview is the confirmation the modal can renew with.
	freshFingerprint, _ := fresh["fingerprint"].(string)
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "purchase_order", "targetId": saved.ID, "scope": "record",
		"reason": "delete the record", "typedNumber": saved.PONumber,
		"expectedFingerprint": freshFingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("renewed confirmation = %d (%v)", code, body)
	}
}

// TestDeletionErrorMapsACrossOrganizationLinkTo409 pins the HTTP shape of the
// store's cross-organization refusal. A full HTTP fixture cannot carry a second
// organization (the single-tenant resolver refuses two rows), so the store test
// owns the end-to-end proof and this owns the response code.
func TestDeletionErrorMapsACrossOrganizationLinkTo409(t *testing.T) {
	server := &Server{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/deletions", nil)
	context := echo.New().NewContext(request, recorder)
	if err := server.deletionError(context, "quotation", 1, store.ErrDeletionCrossOrgLinked); err != nil {
		t.Fatalf("map cross-organization refusal: %v", err)
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d; want 409", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode refusal: %v (%s)", err, recorder.Body.String())
	}
	if body["code"] != "deletion_cross_org_linked" {
		t.Fatalf("code = %v; want deletion_cross_org_linked", body["code"])
	}
	if message, _ := body["error"].(string); !strings.Contains(strings.ToLower(message), "another organization") {
		t.Errorf("error = %q; want it to name the other organization", message)
	}
}

func TestDeleteChainOverAPIRequiresTheTenLetterChallenge(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_chain")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)
	preview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)

	challengeID, letters := challengeDeletionOverAPI(t, server, token, "quotation", quotationID, "chain", preview.Fingerprint)
	if len(letters) != 10 {
		t.Fatalf("letters = %q; want 10", letters)
	}
	for _, letter := range letters {
		if strings.ContainsRune("ILO", letter) || letter < 'A' || letter > 'Z' {
			t.Errorf("letters = %q contain an excluded or lower-case character %q", letters, letter)
		}
	}

	confirm := map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "chain",
		"reason": "entered by mistake", "typedNumber": quoteNumber,
		"expectedFingerprint": preview.Fingerprint,
		"challengeId":         challengeID,
		"challengeLetters":    "0000000000",
	}
	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions", token, confirm)
	if code != http.StatusConflict || body["code"] != "deletion_challenge_invalid" {
		t.Fatalf("wrong letters = %d (%v); want 409/invalid", code, body)
	}

	confirm["challengeLetters"] = strings.ToLower(letters)
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, confirm)
	if code != http.StatusOK {
		t.Fatalf("lower-case challenge = %d (%v); want 200", code, body)
	}
	removed, _ := body["removed"].([]any)
	if len(removed) != 2 {
		t.Errorf("removed = %v; want the quotation and its PO", body["removed"])
	}
	code, _ = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusNotFound {
		t.Fatalf("deleted purchase order = %d; want 404", code)
	}
}

func TestDeleteChainOverAPIStopsOnAChangedChainAndRenews(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_changed")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)
	preview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	challengeID, letters := challengeDeletionOverAPI(t, server, token, "quotation", quotationID, "chain", preview.Fingerprint)

	// Move the connected PO between the challenge and the confirmation.
	code, body := doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10)+"/correction", token,
		correctionBody(saved, "changed payment terms after the challenge"))
	if code != http.StatusOK {
		t.Fatalf("correction = %d (%v)", code, body)
	}

	// A challenge request bound to the old chain is refused too: no letters are
	// issued against a chain the admin has not seen.
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions/challenge", token, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "chain",
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusConflict || body["code"] != "deletion_chain_changed" {
		t.Fatalf("stale challenge request = %d (%v); want 409/changed", code, body)
	}

	confirm := map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "chain",
		"reason": "entered by mistake", "typedNumber": quoteNumber,
		"expectedFingerprint": preview.Fingerprint,
		"challengeId":         challengeID, "challengeLetters": letters,
	}
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, confirm)
	if code != http.StatusConflict || body["code"] != "deletion_chain_changed" {
		t.Fatalf("changed chain = %d (%v); want 409/changed", code, body)
	}
	changes, _ := body["changes"].([]any)
	if len(changes) == 0 {
		t.Fatal("changed chain response carries no changes")
	}
	fresh, _ := body["preview"].(map[string]any)
	if fresh == nil || fresh["fingerprint"] == preview.Fingerprint {
		t.Fatalf("changed chain response has no fresh preview: %v", body["preview"])
	}
	// Nothing was deleted and the old challenge was not consumed: the admin
	// renews with a fresh preview and a fresh challenge.
	code, _ = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("record after refused chain deletion = %d; want 200", code)
	}
	renewed := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	newChallengeID, newLetters := challengeDeletionOverAPI(t, server, token, "quotation", quotationID, "chain", renewed.Fingerprint)
	confirm["expectedFingerprint"] = renewed.Fingerprint
	confirm["challengeId"] = newChallengeID
	confirm["challengeLetters"] = newLetters
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, confirm)
	if code != http.StatusOK {
		t.Fatalf("renewed chain deletion = %d (%v)", code, body)
	}
}

func TestDeleteRejectsAReasonOrTypedNumberMismatchOverAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_confirm_fields")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)
	preview := previewDeletionOverAPI(t, server, token, "purchase_order", saved.ID)

	base := map[string]any{
		"targetType": "purchase_order", "targetId": saved.ID, "scope": "record",
		"reason": "cleanup", "typedNumber": saved.PONumber,
		"expectedFingerprint": preview.Fingerprint,
	}
	blankReason := map[string]any{}
	for key, value := range base {
		blankReason[key] = value
	}
	blankReason["reason"] = "   "
	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions", token, blankReason)
	if code != http.StatusUnprocessableEntity || body["code"] != "deletion_reason_required" {
		t.Fatalf("blank reason = %d (%v)", code, body)
	}
	wrongNumber := map[string]any{}
	for key, value := range base {
		wrongNumber[key] = value
	}
	wrongNumber["typedNumber"] = quoteNumber
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions", token, wrongNumber)
	if code != http.StatusUnprocessableEntity || body["code"] != "deletion_typed_number_mismatch" {
		t.Fatalf("wrong number = %d (%v)", code, body)
	}
	code, _ = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(saved.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("record after refused confirmations = %d; want 200", code)
	}
}

func TestDeleteRejectsAnUnavailableChainScopeOverAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_unavailable")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	saved := savedPOOverAPI(t, server, token, quotationID)
	preview := previewDeletionOverAPI(t, server, token, "quotation", quotationID)
	code, body := doJSON(t, server, http.MethodPost, "/v1/deletions", token, map[string]any{
		"targetType": "quotation", "targetId": quotationID, "scope": "record",
		"reason": "source cancellation", "typedNumber": quoteNumber,
		"expectedFingerprint": preview.Fingerprint,
	})
	if code != http.StatusOK {
		t.Fatalf("delete source = %d (%v)", code, body)
	}

	poPreview := previewDeletionOverAPI(t, server, token, "purchase_order", saved.ID)
	chain := deletionScope(t, poPreview, "chain")
	if poPreview.Scopes[chain].Available {
		t.Fatalf("chain scope = %+v; want unavailable without a source", poPreview.Scopes[chain])
	}
	code, body = doJSON(t, server, http.MethodPost, "/v1/deletions/challenge", token, map[string]any{
		"targetType": "purchase_order", "targetId": saved.ID, "scope": "chain",
		"expectedFingerprint": poPreview.Fingerprint,
	})
	if code != http.StatusUnprocessableEntity || body["code"] != "deletion_scope_unavailable" {
		t.Fatalf("challenge for an unavailable chain = %d (%v)", code, body)
	}
}

func TestSourceMissingDraftIsReadableButCannotBeSaved(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_delete_draft_survives")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
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
		t.Fatalf("source-missing draft = %d (%v); want 200", code, body)
	}
	surviving := decodePurchaseOrder(t, body)
	if !surviving.SourceMissing || surviving.State != "draft" {
		t.Fatalf("surviving draft = %+v; want a source-missing draft", surviving)
	}

	// #496 moved this expectation deliberately: the formerly-linked draft keeps
	// saving its own values. Review and finalize still require a live source,
	// and that server-boundary refusal is pinned below.
	code, body = doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"billingAddress": "Jl. New", "deliveryAddress": "Jl. New",
			"poNumber": "", "receivedDate": surviving.ReceivedDate,
			"items": []map[string]any{{
				"description": "line", "quantityScaled": 10000, "uom": "Pc",
				"unitPriceMinor": 1,
			}},
			"expectedVersion": surviving.Version,
		})
	if code != http.StatusOK {
		t.Fatalf("save source-missing draft = %d (%v); want 200", code, body)
	}
	saved := decodePurchaseOrder(t, body)
	if saved.BillingAddress != "Jl. New" || saved.SourceState != "former" {
		t.Errorf("saved source-missing draft = %+v; want the edited value and former state", saved)
	}

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/review", token, nil)
	if code != http.StatusConflict || body["code"] != "po_source_missing" {
		t.Fatalf("review source-missing draft = %d (%v); want 409/po_source_missing", code, body)
	}
	code, body = doJSON(t, server, http.MethodPost,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10)+"/finalize", token, map[string]any{
			"expectedVersion": saved.Version,
		})
	if code != http.StatusConflict || body["code"] != "po_source_missing" {
		t.Fatalf("finalize source-missing draft = %d (%v); want 409/po_source_missing", code, body)
	}
}
