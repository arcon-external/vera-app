package poreport

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Issue #657: a fulfillment page is read by staff and crosses the desktop's JS
// runtime, so no supplier unit cost may exist on any of its types. Aggregate
// realized margin is deliberately allowed: the report already publishes it.

// fulfillmentTypes is every wire type the drill-down returns. A new type must be
// added here, or the leak guard silently stops covering the page.
func fulfillmentTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(FulfillmentQuery{}),
		reflect.TypeOf(FulfillmentContribution{}),
		reflect.TypeOf(FulfillmentItem{}),
		reflect.TypeOf(FulfillmentExtraExpense{}),
		reflect.TypeOf(FulfillmentQuotation{}),
		reflect.TypeOf(FulfillmentPage{}),
	}
}

// TestFulfillmentTypesCarryNoSupplierUnitCost walks every field, including
// nested structs and slices, and refuses a name or JSON tag that could expose a
// per-unit supplier cost. It asserts the aggregate margin is present, so the
// guard cannot pass because the page carries no money at all.
func TestFulfillmentTypesCarryNoSupplierUnitCost(t *testing.T) {
	visited := map[reflect.Type]bool{}
	var walk func(reflect.Type, string)
	walk = func(fieldType reflect.Type, path string) {
		switch fieldType.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() != reflect.Struct {
			return
		}
		if visited[fieldType] {
			return
		}
		visited[fieldType] = true
		for index := 0; index < fieldType.NumField(); index++ {
			field := fieldType.Field(index)
			name := strings.ToLower(field.Name)
			tag := strings.ToLower(field.Tag.Get("json"))
			for _, forbidden := range []string{"costunit", "cost_unit", "suppliercost", "supplier_cost"} {
				if strings.Contains(name, forbidden) || strings.Contains(tag, forbidden) {
					t.Errorf("%s.%s (%s) exposes a supplier unit cost",
						path, field.Name, field.Tag.Get("json"))
				}
			}
			walk(field.Type, path+"."+field.Name)
		}
	}
	for _, fulfillmentType := range fulfillmentTypes() {
		walk(fulfillmentType, fulfillmentType.Name())
	}
	if !visited[reflect.TypeOf(FulfillmentItem{})] {
		t.Fatal("the walk never reached FulfillmentItem; the leak guard would be vacuous")
	}

	subNo := 2
	page := FulfillmentPage{
		AsOf:   "2026-11-19",
		Filter: FulfillmentQuery{Currency: "IDR", Status: "all", Page: 1, PageSize: 25},
		Quotations: []FulfillmentQuotation{{
			QuotationID: 1, QuoteNumber: "1501260001", CustomerName: "PT. Leak",
			Currency: "IDR", LatestRevision: 0, IssuedAt: "2026-11-01",
			ValidUntil: "2026-12-01", Status: "open",
			Items: []FulfillmentItem{{
				QuotationItemNo: 1, QuotationSubNo: &subNo, Label: "1.2",
				Description: "Mixed child", GroupTitle: "Combined",
				QuotedQuantityScaled: 2 * 10000, BoughtQuantityScaled: 3 * 10000,
				OverBoughtQuantityScaled:       1 * 10000,
				RealizedAmountMinor:            33_000,
				RealizedMarginMinor:            15_000,
				QuantityExtraIncomeAmountMinor: 10_000,
				QuantityExtraIncomeMarginMinor: 4_000,
				PurchaseOrders: []FulfillmentContribution{{
					ID: 9, PONumber: "PO-1", State: "saved",
					ReceivedDate: "2026-03-10", AllocatedQuantityScaled: 30000,
				}},
			}},
			ExtraExpenses: []FulfillmentExtraExpense{{
				PurchaseOrderID: 9, PONumber: "PO-1", LineNumber: 2,
				Description: "Freight", QuantityScaled: 20000, UoM: "Lot",
				AmountMinor: 1_000_000, GroupTitle: "Combined",
			}},
		}},
		Page: 1, PageSize: 25, Total: 1, TotalPages: 1,
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal fulfillment page: %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{"costUnitMinor", "cost_unit", "costUnit"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("fulfillment JSON contains %q: %s", forbidden, text)
		}
	}
	// The aggregate margin is the intended figure; its presence proves the scan
	// above is not passing on an empty page.
	if !strings.Contains(text, "realizedMarginMinor") ||
		!strings.Contains(text, `"realizedMarginMinor":15000`) {
		t.Errorf("fulfillment JSON lacks the aggregate realized margin: %s", text)
	}
}
