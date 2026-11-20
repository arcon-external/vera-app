package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Issue #628: the business category over the authenticated HTTP surface. A
// create and an update carry it, a read and the archive return it, the archive
// filters by exactly one value, an unknown value is a 400, and the final save
// refuses an empty category with a code that names the field.

func TestPurchaseOrderBusinessCategoryRoundTripsAndFiltersOverHTTP(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_category")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())

	code, body := createPODraft(t, server, token, quotationID, 0, map[string]any{
		"businessCategory": "FAB",
	})
	if code != http.StatusCreated {
		t.Fatalf("create with FAB = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	if draft.BusinessCategory != "FAB" {
		t.Fatalf("created category = %q; want FAB", draft.BusinessCategory)
	}
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	code, body = doJSON(t, server, http.MethodGet, path, token, nil)
	if code != http.StatusOK {
		t.Fatalf("read = %d (%v)", code, body)
	}
	if read := decodePurchaseOrder(t, body); read.BusinessCategory != "FAB" {
		t.Errorf("read category = %q; want FAB", read.BusinessCategory)
	}

	// The same editable payload a save sends carries the new category.
	update := finalizeBody(draft, "")
	update["businessCategory"] = "FM"
	code, body = doJSON(t, server, http.MethodPut, path, token, update)
	if code != http.StatusOK {
		t.Fatalf("update category = %d (%v)", code, body)
	}
	if updated := decodePurchaseOrder(t, body); updated.BusinessCategory != "FM" {
		t.Fatalf("updated category = %q; want FM", updated.BusinessCategory)
	}

	// The archive returns the category and filters by exactly one value.
	all := listPurchaseOrders(t, server, token, "?page=1")
	if len(all.PurchaseOrders) != 1 || all.PurchaseOrders[0].BusinessCategory != "FM" {
		t.Fatalf("all rows = %+v; want one FM row", all.PurchaseOrders)
	}
	fm := listPurchaseOrders(t, server, token, "?category=FM")
	if fm.Total != 1 || len(fm.PurchaseOrders) != 1 {
		t.Errorf("category=FM total = %d rows = %d; want 1/1", fm.Total, len(fm.PurchaseOrders))
	}
	fab := listPurchaseOrders(t, server, token, "?category=FAB")
	if fab.Total != 0 || len(fab.PurchaseOrders) != 0 {
		t.Errorf("category=FAB total = %d rows = %d; want an empty page", fab.Total, len(fab.PurchaseOrders))
	}

	// An unknown filter is a named 400, never "everything".
	code, body = doJSON(t, server, http.MethodGet, "/v1/purchase-orders?category=NOPE", token, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("unknown category filter = %d (%v); want 400", code, body)
	}

	// An unknown value on an update is refused by name too.
	invalid := finalizeBody(draft, "")
	invalid["businessCategory"] = "NOPE"
	invalid["expectedVersion"] = 2
	code, body = doJSON(t, server, http.MethodPut, path, token, invalid)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown category update = %d (%v); want 422", code, body)
	}
}

func TestPurchaseOrderFinalSaveRefusesAnEmptyBusinessCategoryOverHTTP(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_category_finalize")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())

	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	finalize := finalizeBody(draft, "PO-CATEGORY-API")
	finalize["businessCategory"] = ""
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, finalize)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("empty-category finalize = %d (%v); want 422", code, body)
	}
	if got := errorCode(t, body); got != "po_business_category_required" {
		t.Errorf("refusal code = %q; want po_business_category_required", got)
	}
	if message, _ := body["error"].(string); !strings.Contains(strings.ToLower(message), "business category") {
		t.Errorf("refusal message = %q; want it to name the business category", message)
	}

	// Left editable: the record is still a draft and a later final save with a
	// category succeeds.
	code, body = doJSON(t, server, http.MethodGet, path, token, nil)
	if code != http.StatusOK {
		t.Fatalf("reload after refusal = %d (%v)", code, body)
	}
	reloaded := decodePurchaseOrder(t, body)
	if reloaded.State != "draft" {
		t.Fatalf("state after refusal = %q; want draft", reloaded.State)
	}
	retry := finalizeBody(reloaded, "PO-CATEGORY-API")
	retry["businessCategory"] = "OTHERS"
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, retry)
	if code != http.StatusOK {
		t.Fatalf("finalize with OTHERS = %d (%v); want 200", code, body)
	}
	if saved := decodePurchaseOrder(t, body); saved.BusinessCategory != "OTHERS" {
		t.Errorf("saved category = %q; want OTHERS", saved.BusinessCategory)
	}
	saved := decodePurchaseOrder(t, body)

	// A correction stores the new category and keeps the previous one in the
	// immutable snapshot the history endpoint returns.
	correction := correctionBody(saved, "Reclassify the order")
	correction["businessCategory"] = "FM"
	code, body = doJSON(t, server, http.MethodPost, path+"/correction", token, correction)
	if code != http.StatusOK {
		t.Fatalf("correction to FM = %d (%v); want 200", code, body)
	}
	if corrected := decodePurchaseOrder(t, body); corrected.BusinessCategory != "FM" {
		t.Errorf("corrected category = %q; want FM", corrected.BusinessCategory)
	}
	code, body = doJSON(t, server, http.MethodGet, path+"/history", token, nil)
	if code != http.StatusOK {
		t.Fatalf("history = %d (%v); want 200", code, body)
	}
	history := decodePurchaseOrderHistory(t, body)
	if len(history) != 1 || history[0].BusinessCategory != "OTHERS" {
		t.Errorf("history categories = %+v; want one snapshot reading OTHERS", history)
	}
}

func errorCode(t *testing.T, body map[string]any) string {
	t.Helper()
	code, _ := body["code"].(string)
	return code
}
