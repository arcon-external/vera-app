package api

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Issue #468: the authenticated HTTP surface for a manual PO draft. These
// tests drive the real server and disposable Postgres: source revalidation,
// snapshot copying, create/reopen/update, the one-active-record rule, and the
// selector occupancy answer the frontend consumes.

type purchaseOrderRow struct {
	PODate                *string `json:"poDate"`
	RequestedDeliveryDate *string `json:"requestedDeliveryDate"`
	UUID                  string  `json:"uuid"`
	State                 string  `json:"state"`
	QuoteNumber           string  `json:"quoteNumber"`
	CustomerName          string  `json:"customerName"`
	BillingAddress        string  `json:"billingAddress"`
	DeliveryAddress       string  `json:"deliveryAddress"`
	ShipToCode            string  `json:"shipToCode"`
	ContactEmail          string  `json:"contactEmail"`
	ContactPhone          string  `json:"contactPhone"`
	PONumber              string  `json:"poNumber"`
	ReceivedDate          string  `json:"receivedDate"`
	Incoterms             string  `json:"incoterms"`
	Currency              string  `json:"currency"`
	TaxZone               string  `json:"taxZone"`
	TaxCategory           string  `json:"taxCategory"`
	Items                 []struct {
		QuotationItemNo *int   `json:"quotationItemNo"`
		QuotationSubNo  *int   `json:"quotationSubNo"`
		LineNumber      int    `json:"lineNumber"`
		Description     string `json:"description"`
		QuantityScaled  int64  `json:"quantityScaled"`
		UoM             string `json:"uom"`
		UnitPriceMinor  int64  `json:"unitPriceMinor"`
		DiscountEnabled bool   `json:"discountEnabled"`
		DiscountPercent int16  `json:"discountPercent"`
		GrossMinor      int64  `json:"grossMinor"`
		DiscountMinor   int64  `json:"discountMinor"`
		NetMinor        int64  `json:"netMinor"`
	} `json:"items"`
	DiscountPercent        int16  `json:"discountPercent"`
	PpnBps                 int16  `json:"ppnBps"`
	AdjustmentReason       string `json:"adjustmentReason"`
	ID                     int64  `json:"id"`
	QuotationID            int64  `json:"quotationId"`
	SourceRevision         int16  `json:"sourceRevision"`
	CustomerID             int64  `json:"customerId"`
	PaymentTerms           string `json:"paymentTerms"`
	SubtotalMinor          int64  `json:"subtotalMinor"`
	DiscountMinor          int64  `json:"discountMinor"`
	PpnMinor               int64  `json:"ppnMinor"`
	AdjustmentMinor        int64  `json:"adjustmentMinor"`
	GrandTotalMinor        int64  `json:"grandTotalMinor"`
	PrintedSubtotalMinor   int64  `json:"printedSubtotalMinor"`
	PrintedDiscountMinor   int64  `json:"printedDiscountMinor"`
	PrintedPpnMinor        int64  `json:"printedPpnMinor"`
	PrintedGrandTotalMinor int64  `json:"printedGrandTotalMinor"`
}

func decodePurchaseOrder(t *testing.T, body map[string]any) purchaseOrderRow {
	t.Helper()
	var row purchaseOrderRow
	encodeDecode(t, body, &row)
	return row
}

// setPurchaseOrderStateForTest moves a stored record into the saved state.
// Only #471 may save through the API, so #468's read-side occupancy test
// reaches the column directly, exactly as a finalized record will look then.
func setPurchaseOrderStateForTest(t *testing.T, schema string, id int64, state string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("VERA_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("set VERA_POSTGRES_TEST_DSN to run API integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect for state change: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		"UPDATE "+quoteIdentForTest(schema)+".purchase_orders SET state = $2 WHERE id = $1",
		id, state); err != nil {
		t.Fatalf("set purchase order %d state %s: %v", id, state, err)
	}
}

func createPODraft(t *testing.T, server *Server, token string, quotationID int64, revision int16, extra map[string]any) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"quotationId":    quotationID,
		"revisionNumber": revision,
	}
	for key, value := range extra {
		body[key] = value
	}
	return doJSON(t, server, http.MethodPost, "/v1/purchase-orders", token, body)
}

func TestPurchaseOrderDraftCreateReopenAndUpdate(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_draft")
	quotationID, quoteNumber := exportForSelector(t, server, token, quotationInput())

	// The API defaults the received date in the company date context (WIB).
	today := time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02")
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create = %d; want 201 (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	if draft.State != "draft" || draft.QuotationID != quotationID || draft.SourceRevision != 0 {
		t.Errorf("draft identity = %+v; want draft of quotation %d revision 0", draft, quotationID)
	}
	if draft.QuoteNumber != quoteNumber ||
		draft.CustomerName != "PT. Infineon Technologies Batam" {
		t.Errorf("snapshot = %q / %q; want %q / the revision customer",
			draft.QuoteNumber, draft.CustomerName, quoteNumber)
	}
	// The issued revision copied its customer-facing basis, not the live
	// company currency: this quotation was issued in IDR.
	if draft.Currency != "IDR" || draft.TaxZone != "FTZ" ||
		draft.PaymentTerms != "30 days" || draft.Incoterms != "EXW" {
		t.Errorf("basis = %s/%s/%q/%s; want IDR/FTZ/%q/EXW",
			draft.Currency, draft.TaxZone, draft.PaymentTerms, draft.Incoterms, "30 days")
	}
	if draft.ReceivedDate != today {
		t.Errorf("received date = %q; want today", draft.ReceivedDate)
	}
	billing := "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia"
	if draft.BillingAddress != billing || draft.DeliveryAddress != billing {
		t.Errorf("address snapshots = %q / %q; want the revision address on both",
			draft.BillingAddress, draft.DeliveryAddress)
	}
	if draft.PONumber != "" || draft.PODate != nil || draft.RequestedDeliveryDate != nil {
		t.Errorf("optional customer fields were invented: %+v", draft)
	}
	// Two customer-facing lines, from the quotation's two items. The first
	// was margin-enabled at 20% and keeps its marginalised list price.
	if len(draft.Items) != 2 {
		t.Fatalf("lines = %d; want 2", len(draft.Items))
	}
	if draft.Items[0].Description != "Test item" || draft.Items[0].QuantityScaled != 20000 ||
		draft.Items[0].UnitPriceMinor != 120000 || draft.Items[0].QuotationItemNo == nil ||
		*draft.Items[0].QuotationItemNo != 1 {
		t.Errorf("line 1 = %+v; want Test item 2.0000 at 120000 item 1", draft.Items[0])
	}
	if draft.Items[1].Description != "Plain item" || draft.Items[1].QuantityScaled != 10000 ||
		draft.Items[1].UnitPriceMinor != 50000 {
		t.Errorf("line 2 = %+v; want Plain item 1.0000 at 50000", draft.Items[1])
	}

	// Reopen.
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen = %d; want 200 (%v)", code, body)
	}
	reopened := decodePurchaseOrder(t, body)
	if reopened.ID != draft.ID || reopened.UUID != draft.UUID || len(reopened.Items) != 2 {
		t.Errorf("reopened = %+v; want the stored draft", reopened)
	}

	// Update replaces the header and lines. The source stays fixed.
	code, body = doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"billingAddress":        "Jl. Billing edited",
			"deliveryAddress":       "Jl. Delivery",
			"shipToCode":            "SHIP-1",
			"contactEmail":          "buyer@example.com",
			"contactPhone":          "+62 21 555",
			"poNumber":              "PO-9001",
			"poDate":                "2026-10-01",
			"receivedDate":          "2026-10-02",
			"requestedDeliveryDate": "2026-11-20",
			"paymentTerms":          "50% advance",
			"incoterms":             "FOB",
			"items": []map[string]any{
				{
					"quotationItemNo": 1,
					"quotationSubNo":  nil,
					"description":     "Edited test item",
					"quantityScaled":  25000,
					"uom":             "Lot",
					"unitPriceMinor":  125000,
				},
				{
					"description":    "Extra freight",
					"quantityScaled": 10000,
					"uom":            "Lot",
					"unitPriceMinor": 7500,
				},
			},
		})
	if code != http.StatusOK {
		t.Fatalf("update = %d; want 200 (%v)", code, body)
	}
	updated := decodePurchaseOrder(t, body)
	if updated.PONumber != "PO-9001" || updated.ShipToCode != "SHIP-1" ||
		updated.ContactEmail != "buyer@example.com" || updated.PaymentTerms != "50% advance" ||
		updated.Incoterms != "FOB" || *updated.PODate != "2026-10-01" ||
		updated.ReceivedDate != "2026-10-02" || *updated.RequestedDeliveryDate != "2026-11-20" {
		t.Errorf("updated header = %+v", updated)
	}
	if updated.QuotationID != quotationID || updated.SourceRevision != 0 ||
		updated.Currency != draft.Currency || updated.CustomerName != draft.CustomerName {
		t.Errorf("an update moved the immutable source snapshot: %+v", updated)
	}
	if len(updated.Items) != 2 || updated.Items[0].Description != "Edited test item" ||
		updated.Items[0].QuotationItemNo == nil || *updated.Items[0].QuotationItemNo != 1 ||
		updated.Items[1].QuotationItemNo != nil {
		t.Errorf("updated lines = %+v", updated.Items)
	}
}

func TestPurchaseOrderDraftRequiresAnExplicitRevisionAndIssuedSource(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_source")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())

	// Revision 0 is a real revision, not a missing value.
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("revision 0 create = %d; want 201 (%v)", code, body)
	}
	// But an absent revision field is incomplete.
	code, _ = doJSON(t, server, http.MethodPost, "/v1/purchase-orders", token, map[string]any{
		"quotationId": quotationID,
	})
	if code != http.StatusBadRequest {
		t.Errorf("missing revision = %d; want 400", code)
	}
	// A revision that was never issued is not a source.
	otherID, _ := exportForSelector(t, server, token, poSourceInput("Another", "Item"))
	code, _ = createPODraft(t, server, token, otherID, 1, nil)
	if code != http.StatusNotFound {
		t.Errorf("unissued revision = %d; want 404", code)
	}
	code, _ = createPODraft(t, server, token, otherID+9999, 0, nil)
	if code != http.StatusNotFound {
		t.Errorf("unknown quotation = %d; want 404", code)
	}
}

func TestPurchaseOrderDraftRefusesASecondActiveRecord(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_unique")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())

	if code, body := createPODraft(t, server, token, quotationID, 0, nil); code != http.StatusCreated {
		t.Fatalf("first create = %d; want 201 (%v)", code, body)
	}
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusConflict {
		t.Fatalf("second create = %d; want 409 (%v)", code, body)
	}
	if body["code"] != "po_already_exists" {
		t.Errorf("conflict code = %v; want po_already_exists", body["code"])
	}
}

func TestPurchaseOrderDraftRejectsInvalidLinesAndSavedWrites(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_invalid")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	_, body := createPODraft(t, server, token, quotationID, 0, nil)
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	// A line naming an item the source revision never carried is refused.
	code, response := doJSON(t, server, http.MethodPut, path, token, map[string]any{
		"paymentTerms": "30 days after delivery",
		"items": []map[string]any{{
			"quotationItemNo": 99,
			"description":     "Bogus",
			"quantityScaled":  10000,
			"uom":             "Lot",
			"unitPriceMinor":  1,
		}},
	})
	if code != http.StatusUnprocessableEntity {
		t.Errorf("bad reference = %d; want 422 (%v)", code, response)
	}

	code, response = doJSON(t, server, http.MethodPut, path, token, map[string]any{
		"paymentTerms": "30 days after delivery",
		"items": []map[string]any{{
			"description":    "Zero quantity",
			"quantityScaled": 0,
			"uom":            "Lot",
			"unitPriceMinor": 1,
		}},
	})
	if code != http.StatusUnprocessableEntity {
		t.Errorf("zero quantity = %d; want 422 (%v)", code, response)
	}

	// A finalized record is not writable through the draft endpoint.
	setPurchaseOrderStateForTest(t, "vera_api_po_invalid", draft.ID, "saved")
	code, response = doJSON(t, server, http.MethodPut, path, token, map[string]any{
		"paymentTerms": "30 days after delivery",
	})
	if code != http.StatusConflict {
		t.Errorf("saved update = %d; want 409 (%v)", code, response)
	}
}

func TestPurchaseOrderOccupancyFeedsTheSelector(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_occupancy")
	quotationID, _ := exportForSelector(t, server, token, poSourceInput("Occupied quotation", "Item"))
	_, body := createPODraft(t, server, token, quotationID, 0, nil)
	draft := decodePurchaseOrder(t, body)

	list := listPOSources(t, server, token, "")
	if len(list.Quotations) != 1 {
		t.Fatalf("rows = %d; want 1", len(list.Quotations))
	}
	active := list.Quotations[0].ActivePO
	if active == nil || active.ID != draft.ID || active.State != "draft" {
		t.Errorf("activePO = %+v; want draft %d", active, draft.ID)
	}
	if hidden := listPOSources(t, server, token, "?hideUsed=true"); len(hidden.Quotations) != 0 {
		t.Errorf("hideUsed rows = %d; want the occupied quotation hidden", len(hidden.Quotations))
	}

	// A saved record still occupies the Quotation; only the action differs.
	setPurchaseOrderStateForTest(t, "vera_api_po_occupancy", draft.ID, "saved")
	list = listPOSources(t, server, token, "")
	if list.Quotations[0].ActivePO == nil || list.Quotations[0].ActivePO.State != "saved" {
		t.Errorf("saved activePO = %+v; want state saved", list.Quotations[0].ActivePO)
	}
	if hidden := listPOSources(t, server, token, "?hideUsed=true"); len(hidden.Quotations) != 0 {
		t.Errorf("hideUsed rows = %d; want the saved record hidden too", len(hidden.Quotations))
	}
}

func TestPurchaseOrderEndpointsRequireASession(t *testing.T) {
	server, _ := testQuotationServer(t, "vera_api_po_guard")
	for _, target := range []struct{ method, path string }{
		{http.MethodPost, "/v1/purchase-orders"},
		{http.MethodGet, "/v1/purchase-orders/1"},
		{http.MethodPut, "/v1/purchase-orders/1"},
	} {
		code, _ := doJSON(t, server, target.method, target.path, "", nil)
		if code != http.StatusUnauthorized {
			t.Errorf("%s %s without token = %d; want 401", target.method, target.path, code)
		}
	}
}

// Issue #470: the explained calculation over the authenticated API. These
// assertions are the HTTP-visible contract the desktop binds to: every stage
// of the calculation, the printed source snapshot, and the refusal codes.

func TestPurchaseOrderCalculationRoundTripsThroughTheAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_calc")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())

	// The source revision printed 290000 before its own 10% discount and 261000
	// after it (FTZ, no PPN). The PO starts from the customer-facing list
	// prices, with no discount of its own.
	_, body := createPODraft(t, server, token, quotationID, 0, nil)
	draft := decodePurchaseOrder(t, body)
	if draft.SubtotalMinor != 290000 || draft.GrandTotalMinor != 290000 ||
		draft.DiscountMinor != 0 || draft.PpnBps != 0 || draft.PpnMinor != 0 {
		t.Fatalf("created calculation = %+v; want subtotal/grand 290000 and no tax", draft)
	}
	if draft.PrintedSubtotalMinor != 290000 || draft.PrintedDiscountMinor != 29000 ||
		draft.PrintedPpnMinor != 0 || draft.PrintedGrandTotalMinor != 261000 {
		t.Errorf("printed snapshot = %d/%d/%d/%d; want 290000/29000/0/261000",
			draft.PrintedSubtotalMinor, draft.PrintedDiscountMinor,
			draft.PrintedPpnMinor, draft.PrintedGrandTotalMinor)
	}

	code, body := doJSON(t, server, http.MethodPut,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, map[string]any{
			"paymentTerms":     "45 days",
			"discountPercent":  5,
			"adjustmentMinor":  -700,
			"adjustmentReason": "Match the printed total",
			"items": []map[string]any{
				{
					"quotationItemNo": 1, "description": "Test item",
					"quantityScaled": 20000, "uom": "Lot", "unitPriceMinor": 120000,
					"discountEnabled": true, "discountPercent": 10,
				},
				{
					"description": "Extra freight", "quantityScaled": 10000,
					"uom": "Lot", "unitPriceMinor": 50000,
				},
			},
		})
	if code != http.StatusOK {
		t.Fatalf("update = %d; want 200 (%v)", code, body)
	}
	updated := decodePurchaseOrder(t, body)

	// 2 x 120000 = 240000, 10% is 24000 -> net 216000. 1 x 50000 = 50000.
	// Subtotal 266000; 5% whole is 13300 -> 252700; adjustment -700 -> 252000.
	if len(updated.Items) != 2 {
		t.Fatalf("lines = %d; want 2", len(updated.Items))
	}
	first, second := updated.Items[0], updated.Items[1]
	if first.GrossMinor != 240000 || first.DiscountMinor != 24000 || first.NetMinor != 216000 ||
		!first.DiscountEnabled || first.DiscountPercent != 10 {
		t.Errorf("line 1 calculation = %+v; want 240000/24000/216000 at 10%%", first)
	}
	if second.GrossMinor != 50000 || second.DiscountMinor != 0 || second.NetMinor != 50000 {
		t.Errorf("line 2 calculation = %+v; want 50000/0/50000", second)
	}
	if updated.SubtotalMinor != 266000 || updated.DiscountPercent != 5 ||
		updated.DiscountMinor != 13300 || updated.AdjustmentMinor != -700 ||
		updated.AdjustmentReason != "Match the printed total" ||
		updated.GrandTotalMinor != 252000 {
		t.Errorf("updated totals = %+v; want 266000/5/13300/-700/252000", updated)
	}

	// GET returns the stored calculation, not a fresh one.
	code, body = doJSON(t, server, http.MethodGet,
		"/v1/purchase-orders/"+strconv.FormatInt(draft.ID, 10), token, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen = %d; want 200 (%v)", code, body)
	}
	reopened := decodePurchaseOrder(t, body)
	if reopened.SubtotalMinor != updated.SubtotalMinor ||
		reopened.DiscountMinor != updated.DiscountMinor ||
		reopened.AdjustmentMinor != updated.AdjustmentMinor ||
		reopened.GrandTotalMinor != updated.GrandTotalMinor ||
		reopened.Items[0].NetMinor != 216000 {
		t.Errorf("reopened calculation = %+v; want the stored figures", reopened)
	}
}

func TestPurchaseOrderPPNIsCustomerFacingOnlyOverTheAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_ppn")
	input := poSourceInput("Taxed goods", "Widget")
	input["TaxZone"] = "NonFTZ"
	input["TaxCategory"] = "SupplyOnly"
	quotationID, _ := exportForSelector(t, server, token, input)

	_, body := createPODraft(t, server, token, quotationID, 0, nil)
	draft := decodePurchaseOrder(t, body)
	// One widget at 100000, Non-FTZ goods: PPN 11% is added on top and shown
	// separately, so the customer pays 111000.
	if draft.PpnBps != 1100 || draft.PpnMinor != 11000 ||
		draft.SubtotalMinor != 100000 || draft.GrandTotalMinor != 111000 {
		t.Errorf("PPN calculation = %+v; want 1100 bps / 11000 / grand 111000", draft)
	}
	if draft.PrintedGrandTotalMinor != 111000 {
		t.Errorf("printed source grand = %d; want 111000 (the quotation printed PPN)",
			draft.PrintedGrandTotalMinor)
	}

	// PPH22 never reaches a PO: services carry no customer-facing tax at all.
	services := poSourceInput("Installation services", "Installation")
	services["TaxZone"] = "NonFTZ"
	services["TaxCategory"] = "SupplyInstall"
	servicesID, _ := exportForSelector(t, server, token, services)
	_, body = createPODraft(t, server, token, servicesID, 0, nil)
	servicePO := decodePurchaseOrder(t, body)
	if servicePO.PpnBps != 0 || servicePO.PpnMinor != 0 ||
		servicePO.GrandTotalMinor != 100000 {
		t.Errorf("services PO = %+v; want no PPN and grand 100000", servicePO)
	}
}

func TestPurchaseOrderCalculationRefusesInvalidInputOverTheAPI(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_calc_invalid")
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	_, body := createPODraft(t, server, token, quotationID, 0, nil)
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	validItem := map[string]any{
		"description": "Widget", "quantityScaled": 10000,
		"uom": "Pc", "unitPriceMinor": 100000,
	}

	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{
			name: "whole discount above 100",
			body: map[string]any{
				"paymentTerms": "45 days", "discountPercent": 101,
				"items": []map[string]any{validItem},
			},
			field: "discount",
		},
		{
			name: "row discount above 100",
			body: map[string]any{
				"paymentTerms": "45 days",
				"items": []map[string]any{{
					"description": "Widget", "quantityScaled": 10000, "uom": "Pc",
					"unitPriceMinor": 100000, "discountEnabled": true, "discountPercent": 101,
				}},
			},
			field: "discount",
		},
		{
			name: "unexplained adjustment",
			body: map[string]any{
				"paymentTerms": "45 days", "adjustmentMinor": -1,
				"items": []map[string]any{validItem},
			},
			field: "reason",
		},
		{
			name: "adjustment below zero total",
			body: map[string]any{
				"paymentTerms": "45 days", "adjustmentMinor": -100001,
				"adjustmentReason": "Too far",
				"items":            []map[string]any{validItem},
			},
			field: "negative",
		},
		{
			name: "overflowing line",
			body: map[string]any{
				"paymentTerms": "45 days",
				"items": []map[string]any{{
					"description": "Impossible", "quantityScaled": int64(9223372036854775807),
					"uom": "Pc", "unitPriceMinor": int64(9223372036854775807),
				}},
			},
			field: "range",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			code, response := doJSON(t, server, http.MethodPut, path, token, testCase.body)
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d; want 422 (%v)", code, response)
			}
			message, _ := response["error"].(string)
			if !strings.Contains(strings.ToLower(message), testCase.field) {
				t.Errorf("error = %q; want it to name %q", message, testCase.field)
			}
		})
	}

	// Nothing refused may have been stored.
	_, body = doJSON(t, server, http.MethodGet, path, token, nil)
	reopened := decodePurchaseOrder(t, body)
	if reopened.GrandTotalMinor != 290000 || reopened.DiscountMinor != 0 {
		t.Errorf("a refused update changed the PO: %+v", reopened)
	}
}
