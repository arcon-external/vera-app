package api

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"vera-app/backend"
	"vera-app/backend/auth"
	"vera-app/backend/store"
)

// Issue #658: strict mode is the post-cutover server. These tests drive the
// real HTTP surface with VERA_PO_ALLOCATIONS_STRICT on, because the codes and
// statuses are the contract an older desktop displays: 409 when a blank draft
// is retired, 422 when a free line or a discount is refused.

// strictQuotationServer boots the ordinary quotation server against a second
// store handle on the same database with the cutover flag on. The desktop's
// route, session and templates are the same; only the store's mode differs.
func strictQuotationServer(t *testing.T, identity string) (*Server, string) {
	t.Helper()
	ctx := context.Background()
	clone := testDatabase(t, identity)
	database, err := store.Open(ctx, store.Config{
		DSN:                 clone.DSN,
		Schema:              clone.Schema,
		StrictPOAllocations: true,
	})
	if err != nil {
		t.Fatalf("open the strict store: %v", err)
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

	token, _, err := service.Login(ctx, "api-quote@example.com", "test-password", false)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return server, token
}

// strictDraftSaveBody is the shared editable payload an editor sends: the
// stored header and one line per item, with no item ids -- the compatibility
// shape every pre-#502 desktop submits.
func strictDraftSaveBody(row purchaseOrderRow) map[string]any {
	items := make([]map[string]any, 0, len(row.Items))
	for _, item := range row.Items {
		items = append(items, map[string]any{
			"quotationItemNo": item.QuotationItemNo,
			"quotationSubNo":  item.QuotationSubNo,
			"lineKind":        item.LineKind,
			"description":     item.Description,
			"quantityScaled":  item.QuantityScaled,
			"uom":             item.UoM,
			"unitPriceMinor":  item.UnitPriceMinor,
			"discountEnabled": item.DiscountEnabled,
			"discountPercent": item.DiscountPercent,
		})
	}
	return map[string]any{
		"billingAddress":   row.BillingAddress,
		"deliveryAddress":  row.DeliveryAddress,
		"shipToCode":       row.ShipToCode,
		"contactEmail":     row.ContactEmail,
		"contactPhone":     row.ContactPhone,
		"poNumber":         row.PONumber,
		"receivedDate":     row.ReceivedDate,
		"paymentTerms":     row.PaymentTerms,
		"incoterms":        row.Incoterms,
		"businessCategory": row.BusinessCategory,
		"aggregateGroups":  []map[string]any{},
		"expectedVersion":  row.Version,
		"items":            items,
	}
}

func strictSourceDraft(t *testing.T, server *Server, token string) purchaseOrderRow {
	t.Helper()
	quotationID, _ := exportForSelector(t, server, token, poSourceInput("Strict rollout", "Strict line"))
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create the source draft = %d (%v); want 201", code, body)
	}
	return decodePurchaseOrder(t, body)
}

func TestStrictModeRefusesABlankDraftOverHTTP(t *testing.T) {
	server, token := strictQuotationServer(t, "vera_api_po_658_blank")

	code, body := createBlankPODraft(t, server, token, nil)
	if code != http.StatusConflict || body["code"] != "po_blank_draft_retired" {
		t.Fatalf("blank draft = %d (%v); want 409/po_blank_draft_retired", code, body)
	}
}

func TestStrictModeStillRefusesALegacyHeaderDiscountRevision(t *testing.T) {
	server, token := strictQuotationServer(t, "vera_api_po_658_legacy_header")
	quotationID, _ := exportForSelector(t, server, token,
		poSourceInput("Strict legacy header", "Strict legacy line"))
	setQuotationRevisionHeaderDiscountForTest(t, "vera_api_po_658_legacy_header",
		quotationID, 0, 10)

	// #652's gate is independent of the #658 cutover: a revision whose stored
	// whole-order discount is nonzero is refused in either mode, so strict
	// never becomes a way around it.
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusConflict || body["code"] != "po_legacy_header_discount" {
		t.Fatalf("create on a legacy header revision = %d (%v); want 409/po_legacy_header_discount",
			code, body)
	}
}

func TestStrictModeRefusesANewFreeLineOverHTTP(t *testing.T) {
	server, token := strictQuotationServer(t, "vera_api_po_658_free_line")
	draft := strictSourceDraft(t, server, token)

	payload := strictDraftSaveBody(draft)
	payload["items"] = append(payload["items"].([]map[string]any), map[string]any{
		"description":    "Free line",
		"quantityScaled": 10000,
		"uom":            "Pc",
		"unitPriceMinor": 12500,
	})
	code, body := doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, payload)
	if code != http.StatusUnprocessableEntity || body["code"] != "po_free_line_retired" {
		t.Fatalf("new free line = %d (%v); want 422/po_free_line_retired", code, body)
	}
}

func TestStrictModeRefusesDiscountsOverHTTP(t *testing.T) {
	server, token := strictQuotationServer(t, "vera_api_po_658_discount")
	draft := strictSourceDraft(t, server, token)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	rowDiscount := strictDraftSaveBody(draft)
	item := rowDiscount["items"].([]map[string]any)[0]
	item["discountEnabled"] = true
	item["discountPercent"] = 10
	code, body := doJSON(t, server, http.MethodPut, path, token, rowDiscount)
	if code != http.StatusUnprocessableEntity || body["code"] != "po_discount_forbidden" {
		t.Fatalf("row discount = %d (%v); want 422/po_discount_forbidden", code, body)
	}

	headerDiscount := strictDraftSaveBody(draft)
	headerDiscount["discountPercent"] = 10
	code, body = doJSON(t, server, http.MethodPut, path, token, headerDiscount)
	if code != http.StatusUnprocessableEntity || body["code"] != "po_discount_forbidden" {
		t.Fatalf("header discount = %d (%v); want 422/po_discount_forbidden", code, body)
	}

	adjustment := strictDraftSaveBody(draft)
	adjustment["adjustmentMinor"] = 5000
	adjustment["adjustmentReason"] = "rounding"
	code, body = doJSON(t, server, http.MethodPut, path, token, adjustment)
	if code != http.StatusUnprocessableEntity || body["code"] != "po_discount_forbidden" {
		t.Fatalf("header adjustment = %d (%v); want 422/po_discount_forbidden", code, body)
	}

	// The unchanged payload still saves: the cutover refuses new discounts,
	// not every save.
	code, body = doJSON(t, server, http.MethodPut, path, token, strictDraftSaveBody(draft))
	if code != http.StatusOK {
		t.Fatalf("unchanged strict save = %d (%v); want 200", code, body)
	}
}
