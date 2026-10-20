package backend

import (
	"encoding/json"
	"net/http"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #468: the desktop reaches PO drafts through these bindings. The API
// integration suite proves server behaviour; these prove the wrappers send the
// promised payload and map every field the editor renders.

func TestCreatePurchaseOrderDraftPostsTheExactPairAndMapsTheDraft(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/purchase-orders" {
			t.Errorf("request = %s %s; want POST /v1/purchase-orders", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["quotationId"] != float64(12) || body["revisionNumber"] != float64(0) {
			t.Errorf("source payload = %v; want quotation 12 revision 0", body)
		}
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderRow{
			ID: 7, UUID: "uuid-7", State: "draft", QuotationID: 12, SourceRevision: 0,
			QuoteNumber: "1502260012", CustomerID: 2, CustomerName: "PT. Buyer",
			BillingAddress: "Jl. Billing", DeliveryAddress: "Jl. Delivery",
			ShipToCode: "SHIP", ContactEmail: "buyer@example.com", ContactPhone: "+62",
			PONumber: "PO-1", ReceivedDate: "2026-10-20", Incoterms: "CIF",
			Currency: "USD", TaxZone: "NonFTZ", TaxCategory: "SupplyInstall",
			PaymentTermsDays: 45, CreatedAt: "2026-10-20T10:00:00Z",
			UpdatedAt: "2026-10-20T10:00:00Z",
			Items: []apicontract.PurchaseOrderItemRow{{
				LineNumber: 1, Description: "Widget", QuantityScaled: 25000,
				UoM: "Pc", UnitPriceMinor: 1250,
			}},
		})
	})

	q := newTestQuotation()
	draft, err := q.CreatePurchaseOrderDraft(PurchaseOrderCreateInput{
		QuotationID: 12, RevisionNumber: 0,
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrderDraft failed: %v", err)
	}
	if draft.ID != 7 || draft.State != "draft" || draft.SourceRevision != 0 ||
		draft.Currency != "USD" || draft.PaymentTermsDays != 45 ||
		draft.CustomerName != "PT. Buyer" {
		t.Errorf("draft = %+v; want the full mapped header", draft)
	}
	if len(draft.Items) != 1 || draft.Items[0].QuantityScaled != 25000 ||
		draft.Items[0].UnitPriceMinor != 1250 {
		t.Errorf("items = %+v; want the exact scaled line", draft.Items)
	}
}

func TestGetPurchaseOrderReadsOneDraftByID(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/purchase-orders/7" {
			t.Errorf("request = %s %s; want GET /v1/purchase-orders/7", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderRow{
			ID: 7, State: "draft", ReceivedDate: "2026-10-20",
			Items: []apicontract.PurchaseOrderItemRow{},
		})
	})

	q := newTestQuotation()
	draft, err := q.GetPurchaseOrder(7)
	if err != nil {
		t.Fatalf("GetPurchaseOrder failed: %v", err)
	}
	if draft.ID != 7 || draft.ReceivedDate != "2026-10-20" || draft.Items == nil {
		t.Errorf("draft = %+v; want id 7 with an empty line list", draft)
	}
}

func TestUpdatePurchaseOrderDraftPutsTheEditableFields(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/v1/purchase-orders/7" {
			t.Errorf("request = %s %s; want PUT /v1/purchase-orders/7", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["billingAddress"] != "Jl. Edited" || body["poNumber"] != "PO-9" ||
			body["receivedDate"] != "2026-10-02" || body["poDate"] != "2026-10-01" ||
			body["paymentTermsDays"] != float64(60) {
			t.Errorf("header payload = %v", body)
		}
		items, _ := body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("items payload = %v; want 1", body["items"])
		}
		item, _ := items[0].(map[string]any)
		if item["quantityScaled"] != float64(25000) || item["unitPriceMinor"] != float64(1250) ||
			item["quotationItemNo"] != float64(1) {
			t.Errorf("item payload = %v", item)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderRow{
			ID: 7, State: "draft", ReceivedDate: "2026-10-02",
		})
	})

	itemNo := 1
	q := newTestQuotation()
	draft, err := q.UpdatePurchaseOrderDraft(PurchaseOrderUpdateInput{
		ID:               7,
		BillingAddress:   "Jl. Edited",
		PONumber:         "PO-9",
		PODate:           "2026-10-01",
		ReceivedDate:     "2026-10-02",
		PaymentTermsDays: 60,
		Items: []PurchaseOrderItemInput{{
			QuotationItemNo: &itemNo,
			Description:     "Widget",
			QuantityScaled:  25000,
			UoM:             "Pc",
			UnitPriceMinor:  1250,
		}},
	})
	if err != nil {
		t.Fatalf("UpdatePurchaseOrderDraft failed: %v", err)
	}
	if draft.ID != 7 || draft.ReceivedDate != "2026-10-02" {
		t.Errorf("draft = %+v; want the updated record", draft)
	}
}

func TestPurchaseOrderBindingsRequireSignIn(t *testing.T) {
	q := newTestQuotation()
	if _, err := q.CreatePurchaseOrderDraft(PurchaseOrderCreateInput{QuotationID: 1}); err == nil {
		t.Error("CreatePurchaseOrderDraft without a session succeeded; want refusal")
	}
	if _, err := q.GetPurchaseOrder(1); err == nil {
		t.Error("GetPurchaseOrder without a session succeeded; want refusal")
	}
	if _, err := q.UpdatePurchaseOrderDraft(PurchaseOrderUpdateInput{ID: 1}); err == nil {
		t.Error("UpdatePurchaseOrderDraft without a session succeeded; want refusal")
	}
}
