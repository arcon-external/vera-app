package backend

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #655: the desktop wrapper is the only layer that knows the wire shape.
// These tests prove it sends each line's kind, maps the server's kind back on
// every read (record and correction history), and keeps the compatibility
// meaning of an absent value.

func TestUpdatePurchaseOrderDraftCarriesLineKindAndMapsItBack(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/v1/purchase-orders/7" {
			t.Errorf("request = %s %s; want PUT /v1/purchase-orders/7",
				request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		items, _ := body["items"].([]any)
		if len(items) != 2 {
			t.Fatalf("items payload = %v; want 2", body["items"])
		}
		extra, _ := items[0].(map[string]any)
		if extra["lineKind"] != "extra" || extra["description"] != "Freight surcharge" {
			t.Errorf("extra payload = %v; want lineKind extra", extra)
		}
		// An empty kind is the compatibility quotation: the field is omitted,
		// not sent as "", so the server's default applies.
		plain, _ := items[1].(map[string]any)
		if _, present := plain["lineKind"]; present {
			t.Errorf("an empty kind was sent on the wire: %v", plain)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderRow{
			ID: 7, State: "draft", Version: 4, ReceivedDate: "2026-10-20",
			Items: []apicontract.PurchaseOrderItemRow{
				{LineNumber: 1, Description: "Freight surcharge", LineKind: "extra"},
				{LineNumber: 2, Description: "Widget", LineKind: "quotation"},
			},
		})
	})

	q := newTestQuotation()
	result, err := q.UpdatePurchaseOrderDraft(PurchaseOrderUpdateInput{
		ID: 7, ExpectedVersion: 3,
		Items: []PurchaseOrderItemInput{
			{
				Description: "Freight surcharge", QuantityScaled: 20000,
				UoM: "Lot", UnitPriceMinor: 500000, LineKind: "extra",
			},
			{Description: "Widget", QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 100},
		},
	})
	if err != nil {
		t.Fatalf("UpdatePurchaseOrderDraft failed: %v", err)
	}
	if len(result.Order.Items) != 2 {
		t.Fatalf("mapped items = %d; want 2", len(result.Order.Items))
	}
	if result.Order.Items[0].LineKind != "extra" ||
		result.Order.Items[1].LineKind != "quotation" {
		t.Errorf("mapped kinds = %q / %q; want extra / quotation",
			result.Order.Items[0].LineKind, result.Order.Items[1].LineKind)
	}
}

func TestGetPurchaseOrderHistoryMapsTheLineKind(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/purchase-orders/7/history" {
			t.Errorf("request = %s %s; want GET /v1/purchase-orders/7/history",
				request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"history": []apicontract.PurchaseOrderCorrectionRow{{
				ID: 3, UUID: "corr-3", Kind: "correction", State: "saved",
				Reason: "Restate the freight line", CreatedAt: "2026-10-21T08:00:00Z",
				Items: []apicontract.PurchaseOrderCorrectionItemRow{
					{LineNumber: 1, Description: "Widget", LineKind: "quotation"},
					{LineNumber: 2, Description: "Freight surcharge", LineKind: "extra"},
				},
			}},
		})
	})

	q := newTestQuotation()
	history, err := q.GetPurchaseOrderHistory(7)
	if err != nil {
		t.Fatalf("GetPurchaseOrderHistory failed: %v", err)
	}
	if len(history) != 1 || len(history[0].Items) != 2 {
		t.Fatalf("history = %+v; want one two-line snapshot", history)
	}
	if history[0].Items[0].LineKind != "quotation" ||
		history[0].Items[1].LineKind != "extra" {
		t.Errorf("snapshot kinds = %q / %q; want quotation / extra",
			history[0].Items[0].LineKind, history[0].Items[1].LineKind)
	}
}

// TestPurchaseOrderLineKindWireNamesAreFrozen pins the two directions: a read
// always names the kind, and an input omits an empty kind so an older client's
// payload keeps its exact compatibility meaning.
func TestPurchaseOrderLineKindWireNamesAreFrozen(t *testing.T) {
	encoded, err := json.Marshal(apicontract.PurchaseOrderItemRow{LineNumber: 1})
	if err != nil {
		t.Fatalf("marshal item row: %v", err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &row); err != nil {
		t.Fatalf("decode item row: %v", err)
	}
	if raw, present := row["lineKind"]; !present || string(raw) != `""` {
		t.Errorf("item row lineKind = %s (present %v); want an always-present empty value: %s",
			raw, present, encoded)
	}

	encoded, err = json.Marshal(apicontract.PurchaseOrderItemInput{Description: "Widget"})
	if err != nil {
		t.Fatalf("marshal item input: %v", err)
	}
	if strings.Contains(string(encoded), "lineKind") {
		t.Errorf("an absent kind was sent on the wire: %s", encoded)
	}

	encoded, err = json.Marshal(apicontract.PurchaseOrderItemInput{
		Description: "Freight surcharge", LineKind: "extra",
	})
	if err != nil {
		t.Fatalf("marshal extra input: %v", err)
	}
	if !strings.Contains(string(encoded), `"lineKind":"extra"`) {
		t.Errorf("the explicit extra kind is missing: %s", encoded)
	}
}

// TestPurchaseOrderLineKindPacketsCarryNoInternalEconomics is Story 42 at the
// wire: a record or a correction holding an extra line and an aggregate group
// must not sprout a cost, margin, variance, PPH or supplier key.
func TestPurchaseOrderLineKindPacketsCarryNoInternalEconomics(t *testing.T) {
	groupID := int64(5)
	packets := map[string]any{
		"record": PurchaseOrderDTO{
			ID: 7, Currency: "IDR",
			Items: []PurchaseOrderItemDTO{
				{LineNumber: 1, Description: "Widget", LineKind: "quotation", AggregateGroupID: groupID},
				{LineNumber: 2, Description: "Freight surcharge", LineKind: "extra", AggregateGroupID: groupID},
			},
			AggregateGroups: []PurchaseOrderAggregateGroupDTO{
				{ID: groupID, DisplayOrder: 1, Title: "Delivered package", TotalMinor: 1000},
			},
		},
		"correction": PurchaseOrderCorrectionDTO{
			ID: 3, Reason: "Restate the freight line",
			Items: []PurchaseOrderCorrectionItemDTO{
				{LineNumber: 1, Description: "Freight surcharge", LineKind: "extra"},
			},
		},
		"input": PurchaseOrderUpdateInput{
			ID: 7,
			Items: []PurchaseOrderItemInput{
				{Description: "Freight surcharge", LineKind: "extra"},
			},
		},
	}
	forbidden := []string{"cost", "margin", "variance", "pph", "supplier", "profit", "nett"}
	for name, packet := range packets {
		encoded, err := json.Marshal(packet)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		assertNoInternalEconomicsKeys(t, name, decoded, forbidden)
	}
}

func assertNoInternalEconomicsKeys(t *testing.T, path string, value any, forbidden []string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			for _, word := range forbidden {
				if strings.Contains(lower, word) {
					t.Errorf("%s carries the internal-economics key %q", path, key)
				}
			}
			assertNoInternalEconomicsKeys(t, path+"."+key, child, forbidden)
		}
	case []any:
		for _, child := range typed {
			assertNoInternalEconomicsKeys(t, path, child, forbidden)
		}
	}
}
