package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"vera-app/backend"
	"vera-app/backend/apicontract"
	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// closureTestServer is testQuotationServer plus the store handle: closure needs
// a staff actor to prove the admin gate, and the shared helper does not expose
// the database.
func closureTestServer(t *testing.T, schema string) (*Server, *store.Store, string) {
	t.Helper()
	ctx := context.Background()
	// The clone seam shared with every API test since #673. The store opens on
	// the clone's own database and app schema, so a run can never inherit rows
	// from a schema that survived an earlier one.
	database := openTestDatabaseStore(t, schema)
	templates := &backend.Quotation{}
	if err := backend.Startup(templates, ctx); err != nil {
		t.Fatalf("load templates: %v", err)
	}
	service := auth.NewService(database)
	if _, err := service.Bootstrap(ctx, "api-admin@example.com", "API Admin", "test-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	server := NewServer(database, service, templates)
	seedQuotationClient(t, database, 2, "Test Customer")
	token := doLogin(t, server, "api-admin@example.com", "test-password")
	return server, database, token
}

// Issue #656 phase B: closure over the authenticated HTTP surface. The store
// suite proves the decision; these tests prove what a client sees — the admin
// gate, the stored who/when/why, the typed refusals, and the closure/expiry
// fields on every read the selector, the workspace and the detail page use.

func closeQuotationOverHTTP(t *testing.T, server *Server, token string, id int64, reason string) (int, map[string]any) {
	t.Helper()
	return doJSON(t, server, http.MethodPost,
		"/v1/quotations/"+strconv.FormatInt(id, 10)+"/close",
		token, map[string]any{"reason": reason})
}

func reopenQuotationOverHTTP(t *testing.T, server *Server, token string, id int64) (int, map[string]any) {
	t.Helper()
	return doJSON(t, server, http.MethodPost,
		"/v1/quotations/"+strconv.FormatInt(id, 10)+"/reopen", token, nil)
}

func decodeQuotationDetail(t *testing.T, body map[string]any) apicontract.QuotationDetail {
	t.Helper()
	var detail apicontract.QuotationDetail
	encodeDecode(t, body, &detail)
	return detail
}

// findPOSourceRow locates one selector row by id.
func findPOSourceRow(t *testing.T, list poSourceList, id int64) poSourceRow {
	t.Helper()
	for _, row := range list.Quotations {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("selector has no row for quotation %d: %+v", id, list.Quotations)
	return poSourceRow{}
}

// assertNoCustomerFacingEconomics keeps Story 42: the selector, occupancy and
// availability shapes are customer-facing and must never carry a supplier
// cost, margin or profit figure.
func assertNoCustomerFacingEconomics(t *testing.T, label string, body map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("%s: marshal response: %v", label, err)
	}
	lower := strings.ToLower(string(encoded))
	for _, name := range []string{
		"costunit", "suppliercost", "unitcost", "cost_minor",
		"marginamount", "marginalizedunit", "nettprofit", "pphamount",
	} {
		if strings.Contains(lower, strings.ToLower(name)) {
			t.Errorf("%s carries %q; it is customer-facing and must not leak internal economics: %s",
				label, name, encoded)
		}
	}
}

func TestCloseQuotationRequiresAnAdminAndStoresWhoWhenWhy(t *testing.T) {
	server, database, adminToken := closureTestServer(t, "vera_api_quotation_closure")
	id, quoteNumber := exportForSelector(t, server, adminToken,
		poSourceInput("Closure subject", "Closure item"))

	seedStaff(t, database, "closure-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "closure-staff@example.com", "staff-password")

	code, body := closeQuotationOverHTTP(t, server, staffToken, id, "staff tried to close")
	if code != http.StatusForbidden {
		t.Fatalf("staff close = %d; want 403 (%v)", code, body)
	}
	if body["code"] != "quotation_close_forbidden" {
		t.Errorf("staff close code = %v; want quotation_close_forbidden", body["code"])
	}

	code, body = closeQuotationOverHTTP(t, server, adminToken, id,
		"  customer cancelled the remainder  ")
	if code != http.StatusOK {
		t.Fatalf("admin close = %d; want 200 (%v)", code, body)
	}
	detail := decodeQuotationDetail(t, body)
	if !detail.Closed {
		t.Error("close response is not closed")
	}
	if detail.ClosedReason != "customer cancelled the remainder" {
		t.Errorf("closed reason = %q; want the trimmed sentence", detail.ClosedReason)
	}
	if detail.ClosedByName != "API Admin" {
		t.Errorf("closed by = %q; want the actor's display name", detail.ClosedByName)
	}
	if detail.ClosedAt == nil || *detail.ClosedAt == "" {
		t.Error("close response has no closedAt")
	}
	if detail.ValidUntil == nil {
		t.Error("close response has no validUntil; the status line needs it")
	}
	if detail.ExpiredUnclosed {
		t.Error("a freshly closed quotation reports expiredUnclosed; closure wins")
	}

	// The archive detail read and the close answer must agree.
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/quotations/"+strconv.FormatInt(id, 10), adminToken, nil)
	if code != http.StatusOK {
		t.Fatalf("detail read = %d; want 200 (%v)", code, body)
	}
	reread := decodeQuotationDetail(t, body)
	if !reread.Closed || reread.ClosedReason != detail.ClosedReason ||
		reread.ClosedByName != detail.ClosedByName {
		t.Errorf("detail closure = %v/%q/%q; want the close answer",
			reread.Closed, reread.ClosedReason, reread.ClosedByName)
	}

	// The selector row, its availability and the occupancy read all carry it.
	selectorBody := func() map[string]any {
		code, body := doJSON(t, server, http.MethodGet, "/v1/po-sources", adminToken, nil)
		if code != http.StatusOK {
			t.Fatalf("selector read = %d; want 200 (%v)", code, body)
		}
		return body
	}()
	selector := findPOSourceRow(t, decodePOList(t, selectorBody), id)
	if selector.QuoteNumber != quoteNumber {
		t.Errorf("selector quote number = %q; want %q", selector.QuoteNumber, quoteNumber)
	}
	if !selector.Closed || selector.ClosedReason != detail.ClosedReason ||
		selector.ClosedAt == nil || selector.ValidUntil == nil {
		t.Errorf("selector closure = %v/%q/%v/%v; want the stored close and its expiry",
			selector.Closed, selector.ClosedReason, selector.ClosedAt, selector.ValidUntil)
	}
	if selector.ExpiredUnclosed {
		t.Error("a closed selector row reports expiredUnclosed")
	}
	assertNoCustomerFacingEconomics(t, "selector", selectorBody)

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/po-sources/"+strconv.FormatInt(id, 10)+"/occupancy", adminToken, nil)
	if code != http.StatusOK {
		t.Fatalf("occupancy read = %d; want 200 (%v)", code, body)
	}
	var occupancy apicontract.POSourceOccupancyResponse
	encodeDecode(t, body, &occupancy)
	if !occupancy.Closed || occupancy.ClosedReason != detail.ClosedReason ||
		occupancy.ClosedByName != detail.ClosedByName || occupancy.ClosedAt == nil {
		t.Errorf("occupancy closure = %v/%q/%q/%v; want the stored close",
			occupancy.Closed, occupancy.ClosedReason, occupancy.ClosedByName, occupancy.ClosedAt)
	}
	assertNoCustomerFacingEconomics(t, "occupancy", body)

	code, body = doJSON(t, server, http.MethodGet,
		"/v1/po-sources/"+strconv.FormatInt(id, 10)+"/availability", adminToken, nil)
	if code != http.StatusOK {
		t.Fatalf("availability read = %d; want 200 (%v)", code, body)
	}
	var availability apicontract.POSourceAvailabilityResponse
	encodeDecode(t, body, &availability)
	if !availability.Closed || availability.ClosedReason != detail.ClosedReason ||
		availability.ClosedAt == nil || availability.ValidUntil == nil {
		t.Errorf("availability closure = %v/%q/%v/%v; want the stored close",
			availability.Closed, availability.ClosedReason,
			availability.ClosedAt, availability.ValidUntil)
	}
	assertNoCustomerFacingEconomics(t, "availability", body)

	// Reopen restores the open state and clears the live columns.
	code, body = reopenQuotationOverHTTP(t, server, adminToken, id)
	if code != http.StatusOK {
		t.Fatalf("reopen = %d; want 200 (%v)", code, body)
	}
	reopened := decodeQuotationDetail(t, body)
	if reopened.Closed || reopened.ClosedAt != nil || reopened.ClosedReason != "" ||
		reopened.ClosedByName != "" {
		t.Errorf("reopened = %v/%v/%q/%q; want an open quotation",
			reopened.Closed, reopened.ClosedAt, reopened.ClosedReason, reopened.ClosedByName)
	}
	if reopened.ValidUntil == nil {
		t.Error("reopened detail lost validUntil")
	}
}

func TestQuotationClosureValidationAndIdempotence(t *testing.T) {
	server, _, token := closureTestServer(t, "vera_api_quotation_closure_rules")
	id, _ := exportForSelector(t, server, token,
		poSourceInput("Closure rules", "Closure rule item"))

	code, body := closeQuotationOverHTTP(t, server, token, id, "   ")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("blank reason = %d; want 422 (%v)", code, body)
	}
	if body["code"] != "quotation_close_reason_required" {
		t.Errorf("blank reason code = %v; want quotation_close_reason_required", body["code"])
	}

	code, body = closeQuotationOverHTTP(t, server, token, id, strings.Repeat("界", 501))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("501-rune reason = %d; want 422 (%v)", code, body)
	}
	if body["code"] != "quotation_close_reason_too_long" {
		t.Errorf("over-long code = %v; want quotation_close_reason_too_long", body["code"])
	}

	code, body = reopenQuotationOverHTTP(t, server, token, id)
	if code != http.StatusConflict {
		t.Fatalf("reopen of an open quotation = %d; want 409 (%v)", code, body)
	}
	if body["code"] != "quotation_not_closed" {
		t.Errorf("reopen-open code = %v; want quotation_not_closed", body["code"])
	}

	if code, body = closeQuotationOverHTTP(t, server, token, id, "customer cancelled"); code != http.StatusOK {
		t.Fatalf("close = %d; want 200 (%v)", code, body)
	}
	code, body = closeQuotationOverHTTP(t, server, token, id, "second close")
	if code != http.StatusConflict {
		t.Fatalf("second close = %d; want 409 (%v)", code, body)
	}
	if body["code"] != "quotation_already_closed" {
		t.Errorf("second close code = %v; want quotation_already_closed", body["code"])
	}

	code, body = doJSON(t, server, http.MethodPost,
		"/v1/quotations/999999/close", token,
		map[string]any{"reason": "nowhere"})
	if code != http.StatusNotFound {
		t.Fatalf("close of an unknown quotation = %d; want 404 (%v)", code, body)
	}
	if body["code"] != "quotation_not_found" {
		t.Errorf("unknown quotation code = %v; want quotation_not_found", body["code"])
	}
}

func TestClosedQuotationRefusesANewPurchaseOrderDraftOverHTTP(t *testing.T) {
	server, _, token := closureTestServer(t, "vera_api_quotation_closure_ordering")
	id, _ := exportForSelector(t, server, token,
		poSourceInput("Closure ordering", "Closure ordering item"))

	if code, body := closeQuotationOverHTTP(t, server, token, id, "customer cancelled"); code != http.StatusOK {
		t.Fatalf("close = %d; want 200 (%v)", code, body)
	}
	code, body := doJSON(t, server, http.MethodPost, "/v1/purchase-orders", token, map[string]any{
		"quotationId":      id,
		"revisionNumber":   0,
		"businessCategory": "FM",
	})
	if code != http.StatusConflict {
		t.Fatalf("create against a closed quotation = %d; want 409 (%v)", code, body)
	}
	if body["code"] != "quotation_closed" {
		t.Errorf("closed create code = %v; want quotation_closed", body["code"])
	}
}

// TestQuotationClosureRefusalIsATypedConflict pins the mapper itself: the
// finalize path shares it, and a stale draft must be refused with the same
// stable code a client can branch on.
func TestQuotationClosureRefusalIsATypedConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	context := echo.New().NewContext(
		httptest.NewRequest(http.MethodPost, "/v1/purchase-orders/1/finalize", nil),
		recorder)
	if err := purchaseOrderError(context, store.ErrQuotationClosed); err != nil {
		t.Fatalf("purchaseOrderError: %v", err)
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d; want 409", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["code"] != "quotation_closed" {
		t.Errorf("code = %q; want quotation_closed", body["code"])
	}
}

// TestQuotationClosureResponseCarriesNoSupplierCost is the explicit Story 42
// round-trip for the close answer: admin-editable data, never a cost sheet.
func TestQuotationClosureResponseCarriesNoSupplierCost(t *testing.T) {
	server, _, token := closureTestServer(t, "vera_api_quotation_closure_leak")
	id, _ := exportForSelector(t, server, token,
		poSourceInput("Closure leak", "Closure leak item"))
	code, body := closeQuotationOverHTTP(t, server, token, id, "customer cancelled")
	if code != http.StatusOK {
		t.Fatalf("close = %d; want 200 (%v)", code, body)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal close response: %v", err)
	}
	if bytes.Contains(bytes.ToLower(encoded), []byte("costunit")) ||
		bytes.Contains(bytes.ToLower(encoded), []byte("suppliercost")) {
		t.Errorf("close response carries a supplier cost: %s", encoded)
	}
}

// TestClosedQuotationRefusesANewIssuedRevisionOverHTTP proves the export path
// carries the same typed refusal as the store: a closed quotation cannot be
// reissued until it is reopened, so the closed population cannot change under a
// report that already counted it.
func TestClosedQuotationRefusesANewIssuedRevisionOverHTTP(t *testing.T) {
	server, _, token := closureTestServer(t, "vera_api_quotation_closure_revision")
	id, quoteNumber := exportForSelector(t, server, token,
		poSourceInput("Closure revision", "Closure revision item"))

	if code, body := closeQuotationOverHTTP(t, server, token, id, "customer cancelled"); code != http.StatusOK {
		t.Fatalf("close = %d; want 200 (%v)", code, body)
	}
	code, body := doJSON(t, server, http.MethodPost, "/v1/quotations/export", token, map[string]any{
		"input":       poSourceInput("Closure revision", "Closure revision item"),
		"quoteNumber": quoteNumber,
		"revision":    1,
	})
	if code != http.StatusConflict {
		t.Fatalf("export a revision on a closed quotation = %d; want 409 (%v)", code, body)
	}
	if body["code"] != "quotation_closed" {
		t.Errorf("closed export code = %v; want quotation_closed", body["code"])
	}
}
