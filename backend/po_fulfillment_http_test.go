package backend

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"vera-app/backend/apiclient"
	"vera-app/backend/apicontract"
	"vera-app/backend/poreport"
)

// Issue #657 phase B: the desktop binding for the PO Fulfillment drill-down.
// The API integration suite proves the server's status derivation and figures;
// this proves the wrapper sends the promised query to the shared path, returns
// the server page unchanged, and surfaces the server's typed refusal code for
// the screen to branch on.

func TestGetPurchaseOrderFulfillmentSendsTheQueryAndReturnsThePage(t *testing.T) {
	subNo := 2
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet ||
			request.URL.Path != apicontract.PurchaseOrderFulfillmentPath {
			t.Errorf("request = %s %s; want GET %s",
				request.Method, request.URL.Path, apicontract.PurchaseOrderFulfillmentPath)
		}
		query := request.URL.Query()
		if query.Get("currency") != "USD" {
			t.Errorf("currency = %q; want USD", query.Get("currency"))
		}
		if query.Get("status") != "closed" {
			t.Errorf("status = %q; want closed", query.Get("status"))
		}
		if query.Get("search") != "infineon" {
			t.Errorf("search = %q; want the trimmed term", query.Get("search"))
		}
		if query.Get("page") != "2" {
			t.Errorf("page = %q; want 2", query.Get("page"))
		}
		if query.Get(apicontract.PurchaseOrderFulfillmentPageSizeParam) != "25" {
			t.Errorf("page_size = %q; want 25",
				query.Get(apicontract.PurchaseOrderFulfillmentPageSizeParam))
		}
		_ = json.NewEncoder(writer).Encode(poreport.FulfillmentPage{
			AsOf: "2026-11-19",
			Filter: poreport.FulfillmentQuery{
				Currency: "USD", Status: "closed", Search: "infineon",
				Page: 2, PageSize: 25,
			},
			Quotations: []poreport.FulfillmentQuotation{{
				QuotationID: 7, QuoteNumber: "1502260007",
				CustomerName: "PT. Infineon Technologies Batam",
				Currency:     "USD", LatestRevision: 1,
				IssuedAt: "2026-10-10", ValidUntil: "2026-11-09",
				Status: "closed",
				Items: []poreport.FulfillmentItem{{
					QuotationItemNo: 1, QuotationSubNo: &subNo,
					Label: "1.2", Description: "WIP rack",
					GroupTitle:           "Delivered package",
					QuotedQuantityScaled: 50000, BoughtQuantityScaled: 30000,
					UnsoldQuantityScaled: 0, OverBoughtQuantityScaled: 0,
					RealizedAmountMinor: 60_000, RealizedMarginMinor: 30_000,
					UnsoldAmountMinor: 0, UnsoldMarginMinor: 0,
					ClosedLossAmountMinor: 40_000, ClosedLossMarginMinor: 20_000,
					PurchaseOrders: []poreport.FulfillmentContribution{{
						ID: 11, PONumber: "PO-657-1", State: "saved",
						ReceivedDate: "2026-03-10", AllocatedQuantityScaled: 30000,
					}},
				}},
				ExtraExpenses: []poreport.FulfillmentExtraExpense{{
					PurchaseOrderID: 11, PONumber: "PO-657-1", LineNumber: 2,
					Description: "Freight surcharge", QuantityScaled: 20000,
					UoM: "Lot", AmountMinor: 1_000_000,
					GroupTitle: "Delivered package",
				}},
			}},
			Page: 2, PageSize: 25, Total: 26, TotalPages: 1,
		})
	})

	q := newTestQuotation()
	page, err := q.GetPurchaseOrderFulfillment(poreport.FulfillmentQuery{
		Currency: "USD", Status: "closed", Search: "  infineon  ",
		Page: 2, PageSize: 25,
	})
	if err != nil {
		t.Fatalf("GetPurchaseOrderFulfillment failed: %v", err)
	}
	if page.AsOf != "2026-11-19" || page.Page != 2 || page.Total != 26 {
		t.Errorf("page metadata = %+v; want the server page", page)
	}
	if len(page.Quotations) != 1 {
		t.Fatalf("quotations = %d; want 1", len(page.Quotations))
	}
	quotation := page.Quotations[0]
	if quotation.Status != "closed" || quotation.Currency != "USD" ||
		quotation.ValidUntil != "2026-11-09" {
		t.Errorf("quotation = %+v; want the closed USD row", quotation)
	}
	if len(quotation.Items) != 1 {
		t.Fatalf("items = %d; want 1", len(quotation.Items))
	}
	item := quotation.Items[0]
	if item.GroupTitle != "Delivered package" || item.Label != "1.2" ||
		item.QuotationSubNo == nil || *item.QuotationSubNo != 2 {
		t.Errorf("item identity = %+v; want the grouped 1.2 child", item)
	}
	if item.QuotedQuantityScaled != 50000 || item.BoughtQuantityScaled != 30000 ||
		item.ClosedLossAmountMinor != 40_000 || item.RealizedMarginMinor != 30_000 {
		t.Errorf("item figures = %+v; want the server figures preserved", item)
	}
	if len(item.PurchaseOrders) != 1 ||
		item.PurchaseOrders[0].PONumber != "PO-657-1" ||
		item.PurchaseOrders[0].AllocatedQuantityScaled != 30000 {
		t.Errorf("contributions = %+v; want the saved PO", item.PurchaseOrders)
	}
	if len(quotation.ExtraExpenses) != 1 ||
		quotation.ExtraExpenses[0].AmountMinor != 1_000_000 ||
		quotation.ExtraExpenses[0].GroupTitle != "Delivered package" {
		t.Errorf("extras = %+v; want the grouped surcharge", quotation.ExtraExpenses)
	}

	// Story 42: the page that crossed the binding carries no supplier unit cost.
	// The aggregate realized margin is the report's own figure and is expected.
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal returned page: %v", err)
	}
	wire := strings.ToLower(string(encoded))
	for _, banned := range []string{"costunit", "cost_unit", "unitcost", "unit_cost", "supplier"} {
		if strings.Contains(wire, banned) {
			t.Errorf("desktop page JSON contains %q; no supplier cost may cross the binding", banned)
		}
	}
	if !strings.Contains(wire, "realizedmarginminor") {
		t.Error("desktop page JSON lost realizedMarginMinor; the aggregate margin is expected")
	}
}

func TestGetPurchaseOrderFulfillmentOmitsDefaultedFilters(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		for _, key := range []string{
			"status", "search", "page", apicontract.PurchaseOrderFulfillmentPageSizeParam,
		} {
			if _, present := query[key]; present {
				t.Errorf("%s = %v; a defaulted value must not be sent", key, query[key])
			}
		}
		_ = json.NewEncoder(writer).Encode(poreport.FulfillmentPage{
			Filter:     poreport.FulfillmentQuery{Currency: "IDR", Status: "all", Page: 1, PageSize: 25},
			Quotations: []poreport.FulfillmentQuotation{},
		})
	})

	q := newTestQuotation()
	page, err := q.GetPurchaseOrderFulfillment(poreport.FulfillmentQuery{
		Currency: "IDR", Status: "all", Page: 0, PageSize: 0,
	})
	if err != nil {
		t.Fatalf("GetPurchaseOrderFulfillment failed: %v", err)
	}
	if page.Filter.Currency != "IDR" || page.Quotations == nil {
		t.Errorf("page = %+v; want the server defaults preserved", page)
	}
}

func TestGetPurchaseOrderFulfillmentKeepsTheTypedRefusal(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(
			`{"error":"status \"lost\" must be all, open, expired_unclosed, all_bought or closed",` +
				`"code":"purchase_order_fulfillment_invalid"}`))
	})

	q := newTestQuotation()
	_, err := q.GetPurchaseOrderFulfillment(poreport.FulfillmentQuery{
		Currency: "IDR", Status: "lost", Page: 1, PageSize: 25,
	})
	if err == nil {
		t.Fatal("malformed fulfillment succeeded; want the server refusal")
	}
	if got := apiclient.CodeOf(err); got != "purchase_order_fulfillment_invalid" {
		t.Errorf("refusal code = %q; want purchase_order_fulfillment_invalid", got)
	}
	if got := apiclient.StatusOf(err); got != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", got)
	}
}

func TestGetPurchaseOrderFulfillmentRequiresSignIn(t *testing.T) {
	q := newTestQuotation()
	if _, err := q.GetPurchaseOrderFulfillment(poreport.FulfillmentQuery{
		Currency: "IDR", Page: 1, PageSize: 25,
	}); err == nil {
		t.Error("GetPurchaseOrderFulfillment without a session succeeded; want refusal")
	}
}
