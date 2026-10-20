package backend

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"vera-app/backend/apicontract"
	"vera-app/backend/store"
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
			PaymentTerms: "45 days", CreatedAt: "2026-10-20T10:00:00Z",
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
		draft.Currency != "USD" || draft.PaymentTerms != "45 days" ||
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
			body["paymentTerms"] != "50% advance" {
			t.Errorf("header payload = %v", body)
		}
		if body["discountPercent"] != float64(5) ||
			body["adjustmentMinor"] != float64(-250) ||
			body["adjustmentReason"] != "Printed total rounding" {
			t.Errorf("calculation payload = %v", body)
		}
		items, _ := body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("items payload = %v; want 1", body["items"])
		}
		item, _ := items[0].(map[string]any)
		if item["quantityScaled"] != float64(25000) || item["unitPriceMinor"] != float64(1250) ||
			item["quotationItemNo"] != float64(1) || item["discountEnabled"] != true ||
			item["discountPercent"] != float64(10) {
			t.Errorf("item payload = %v", item)
		}
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderRow{
			ID: 7, State: "draft", ReceivedDate: "2026-10-02",
			DiscountPercent: 5, SubtotalMinor: 23750, DiscountMinor: 1187,
			PpnBps: 1100, PpnMinor: 2482, AdjustmentMinor: -250,
			AdjustmentReason: "Printed total rounding", GrandTotalMinor: 24795,
			PrintedSubtotalMinor: 25000, PrintedDiscountMinor: 0,
			PrintedPpnMinor: 2750, PrintedGrandTotalMinor: 27750,
			Items: []apicontract.PurchaseOrderItemRow{{
				LineNumber: 1, Description: "Widget", QuantityScaled: 25000,
				UoM: "Pc", UnitPriceMinor: 1250, DiscountEnabled: true,
				DiscountPercent: 10, GrossMinor: 31250, DiscountMinor: 3125,
				NetMinor: 28125,
			}},
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
		PaymentTerms:     "50% advance",
		DiscountPercent:  5,
		AdjustmentMinor:  -250,
		AdjustmentReason: "Printed total rounding",
		Items: []PurchaseOrderItemInput{{
			QuotationItemNo: &itemNo,
			Description:     "Widget",
			QuantityScaled:  25000,
			UoM:             "Pc",
			UnitPriceMinor:  1250,
			DiscountEnabled: true,
			DiscountPercent: 10,
		}},
	})
	if err != nil {
		t.Fatalf("UpdatePurchaseOrderDraft failed: %v", err)
	}
	if draft.ID != 7 || draft.ReceivedDate != "2026-10-02" {
		t.Errorf("draft = %+v; want the updated record", draft)
	}
	if draft.DiscountPercent != 5 || draft.SubtotalMinor != 23750 ||
		draft.DiscountMinor != 1187 || draft.PpnBps != 1100 || draft.PpnMinor != 2482 ||
		draft.AdjustmentMinor != -250 || draft.AdjustmentReason != "Printed total rounding" ||
		draft.GrandTotalMinor != 24795 || draft.PrintedSubtotalMinor != 25000 ||
		draft.PrintedDiscountMinor != 0 || draft.PrintedPpnMinor != 2750 ||
		draft.PrintedGrandTotalMinor != 27750 {
		t.Errorf("mapped calculation = %+v; want every stored and printed figure", draft)
	}
	if len(draft.Items) != 1 || !draft.Items[0].DiscountEnabled ||
		draft.Items[0].DiscountPercent != 10 || draft.Items[0].GrossMinor != 31250 ||
		draft.Items[0].DiscountMinor != 3125 || draft.Items[0].NetMinor != 28125 {
		t.Errorf("mapped line calculation = %+v", draft.Items)
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

// TestPurchaseOrderPPNRateMatchesQuotationTaxResolution pins the PO calculator
// against the quotation's own tax model. The two live in different packages --
// `store` cannot import `backend` without a cycle -- so the resolver is written
// twice and this test is what keeps the pair from drifting: a change to
// resolveTax's customer-facing PPN must break here.
//
// Both tax models are covered: the legacy category-only revision and the
// current tax_type/service_category one whose legacy category is empty.
func TestPurchaseOrderPPNRateMatchesQuotationTaxResolution(t *testing.T) {
	cases := []struct {
		name            string
		zone            string
		taxType         string
		serviceCategory string
		legacyCategory  string
	}{
		{"legacy FTZ", "FTZ", "", "", ""},
		{"legacy goods", "NonFTZ", "", "", "SupplyOnly"},
		{"legacy service", "NonFTZ", "", "", "SupplyInstall"},
		{"legacy construction service", "NonFTZ", "", "", "SupplyInstallConstruction"},
		{"current non-FTZ goods", "NonFTZ", TaxTypeGoods, "", ""},
		{"current FTZ goods", "FTZ", TaxTypeGoods, "", ""},
		{"current service", "NonFTZ", TaxTypeServices, ServiceCategoryNonConstruction, ""},
		{"current construction service", "NonFTZ", TaxTypeServices, ServiceCategoryConstruction, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			quotationType := testCase.taxType
			quotationCategory := testCase.serviceCategory
			if quotationType == "" {
				if testCase.zone == TaxZoneFTZ {
					quotationType = TaxTypeGoods
				} else {
					quotationType, quotationCategory, _ = legacyTaxCategoryToNew(testCase.legacyCategory)
				}
			}
			ppnBps, _, _, err := resolveTax(testCase.zone, quotationType, quotationCategory)
			if err != nil {
				t.Fatalf("resolveTax: %v", err)
			}
			poBps, err := store.PurchaseOrderPPNBps(
				testCase.zone, testCase.taxType, testCase.serviceCategory, testCase.legacyCategory)
			if err != nil {
				t.Fatalf("PurchaseOrderPPNBps: %v", err)
			}
			if poBps != int16(ppnBps) {
				t.Errorf("PO PPN = %d bps, quotation PPN = %d bps", poBps, ppnBps)
			}
		})
	}
}

// TestPurchaseOrderBindingsModelCarriesEveryField guards the generated Wails
// model shape for this slice. The repository's bindings test covers method
// names; a field the generator dropped would still compile on both sides and
// silently disappear from the update payload (the generated constructor copies
// only the fields its class declares). The class list here is exactly the PO
// types #470 changed.
func TestPurchaseOrderBindingsModelCarriesEveryField(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "frontend", "wailsjs", "go", "models.ts"))
	if err != nil {
		t.Fatalf("read generated models: %v", err)
	}
	source := string(raw)
	targets := []struct {
		name  string
		value any
	}{
		{"PurchaseOrderDTO", PurchaseOrderDTO{}},
		{"PurchaseOrderItemDTO", PurchaseOrderItemDTO{}},
		{"PurchaseOrderCreateInput", PurchaseOrderCreateInput{}},
		{"PurchaseOrderItemInput", PurchaseOrderItemInput{}},
		{"PurchaseOrderUpdateInput", PurchaseOrderUpdateInput{}},
	}
	for _, target := range targets {
		start := strings.Index(source, "export class "+target.name+" {")
		if start < 0 {
			t.Errorf("models.ts has no class %s", target.name)
			continue
		}
		block := source[start:]
		if end := strings.Index(block, "\n\texport class "); end >= 0 {
			block = block[:end]
		}
		structType := reflect.TypeOf(target.value)
		for index := 0; index < structType.NumField(); index++ {
			structField := structType.Field(index)
			field := structField.Name
			// Nested classes are rebuilt through convertValues; the generator
			// does not copy them as plain values, so accept that form too.
			want := `this.` + field + ` = source["` + field + `"];`
			nested := structField.Type.Kind() == reflect.Struct ||
				(structField.Type.Kind() == reflect.Slice &&
					structField.Type.Elem().Kind() == reflect.Struct)
			if nested && strings.Contains(block, `this.`+field+` = this.convertValues(source["`+field+`"]`) {
				continue
			}
			if !strings.Contains(block, want) {
				t.Errorf("models.ts %s is missing %s", target.name, want)
			}
		}
	}
}
