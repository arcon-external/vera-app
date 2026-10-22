package backend

import (
	"encoding/json"
	"net/http"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #469: the desktop reaches the PO archive through this binding. The API
// integration suite proves server behaviour; this proves the wrapper sends the
// promised query and maps every field the list renders.

func TestListPurchaseOrdersSendsTheFiltersAndMapsThePage(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/purchase-orders" {
			t.Errorf("request = %s %s; want GET /v1/purchase-orders", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("page") != "3" {
			t.Errorf("page = %q; want 3", query.Get("page"))
		}
		if query.Get("page_size") != "25" {
			t.Errorf("page_size = %q; want the caller's 25", query.Get("page_size"))
		}
		if query.Get("search") != "  infineon  " {
			t.Errorf("search = %q; want the caller's term", query.Get("search"))
		}
		if got := query["customerId"]; len(got) != 2 || got[0] != "2" || got[1] != "4" {
			t.Errorf("customerId = %v; want repeated 2 and 4", got)
		}
		if query.Get("receivedFrom") != "2026-01-01" || query.Get("receivedTo") != "2026-12-31" {
			t.Errorf("received range = %q..%q", query.Get("receivedFrom"), query.Get("receivedTo"))
		}
		if query.Get("state") != "saved" {
			t.Errorf("state = %q; want saved", query.Get("state"))
		}
		if query.Get("sourceMissing") != "only" {
			t.Errorf("sourceMissing = %q; want only", query.Get("sourceMissing"))
		}
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderListResponse{
			Page:       3,
			PageSize:   25,
			Total:      61,
			TotalPages: 3,
			PurchaseOrders: []apicontract.PurchaseOrderListRow{{
				ID: 7, UUID: "uuid-7", State: "draft", PONumber: "PO-INF-1",
				QuoteNumber: "1502260012", SourceRevision: 2, CustomerID: 2,
				CustomerName: "PT. Infineon Technologies Batam",
				ReceivedDate: "2026-02-10", Currency: "USD", SourceMissing: false,
			}},
			Customers: []apicontract.PurchaseOrderCustomerRow{
				{ID: 2, Name: "PT. Infineon Technologies Batam"},
				{ID: 4, Name: "PT. Datalec Global Solution"},
			},
		})
	})

	q := newTestQuotation()
	page, err := q.ListPurchaseOrders(PurchaseOrderListQuery{
		Page:          3,
		PageSize:      25,
		Search:        "  infineon  ",
		CustomerIDs:   []int64{2, 4},
		ReceivedFrom:  "2026-01-01",
		ReceivedTo:    "2026-12-31",
		State:         "saved",
		SourceMissing: "only",
	})
	if err != nil {
		t.Fatalf("ListPurchaseOrders failed: %v", err)
	}
	if page.Page != 3 || page.PageSize != 25 || page.Total != 61 || page.TotalPages != 3 {
		t.Errorf("page metadata = %+v; want 3/25/61/3", page)
	}
	if len(page.PurchaseOrders) != 1 {
		t.Fatalf("rows = %d; want 1", len(page.PurchaseOrders))
	}
	row := page.PurchaseOrders[0]
	if row.ID != 7 || row.UUID != "uuid-7" || row.State != "draft" ||
		row.PONumber != "PO-INF-1" || row.QuoteNumber != "1502260012" ||
		row.SourceRevision != 2 || row.CustomerID != 2 ||
		row.CustomerName != "PT. Infineon Technologies Batam" ||
		row.ReceivedDate != "2026-02-10" || row.Currency != "USD" ||
		row.SourceMissing {
		t.Errorf("row = %+v; want every list field mapped", row)
	}
	if len(page.Customers) != 2 || page.Customers[0].ID != 2 ||
		page.Customers[1].Name != "PT. Datalec Global Solution" {
		t.Errorf("customers = %+v; want the mapped facet", page.Customers)
	}
}

func TestListPurchaseOrdersOmitsEmptyFiltersAndDefaultsThePage(t *testing.T) {
	stubAPIClient(t, func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("page") != "1" {
			t.Errorf("page = %q; want the default 1", query.Get("page"))
		}
		for _, key := range []string{"search", "state", "sourceMissing", "receivedFrom", "receivedTo", "customerId", "page_size"} {
			if _, present := query[key]; present {
				t.Errorf("%s = %v; an empty filter must not be sent", key, query[key])
			}
		}
		_ = json.NewEncoder(writer).Encode(apicontract.PurchaseOrderListResponse{
			Page: 1, PageSize: 15, TotalPages: 1,
			PurchaseOrders: []apicontract.PurchaseOrderListRow{},
			Customers:      []apicontract.PurchaseOrderCustomerRow{},
		})
	})

	q := newTestQuotation()
	page, err := q.ListPurchaseOrders(PurchaseOrderListQuery{})
	if err != nil {
		t.Fatalf("ListPurchaseOrders failed: %v", err)
	}
	if page.PurchaseOrders == nil || page.Customers == nil {
		t.Errorf("empty page = %+v; want non-nil empty slices", page)
	}
}

func TestListPurchaseOrdersRequiresSignIn(t *testing.T) {
	q := newTestQuotation()
	if _, err := q.ListPurchaseOrders(PurchaseOrderListQuery{}); err == nil {
		t.Error("ListPurchaseOrders without a session succeeded; want refusal")
	}
}
