package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// Issue #469: the authenticated HTTP surface of the PO archive. These tests
// drive the real server and disposable Postgres through public behaviour: the
// page shape, the three search fields, the customer/date/state filters, the
// inert source-missing predicate, the customer facet, validation and the
// session gate. The two-organization proof lives at the store seam, where a
// second organization can exist without breaking the single seeded one.

type poListRow struct {
	UUID           string `json:"uuid"`
	State          string `json:"state"`
	PONumber       string `json:"poNumber"`
	QuoteNumber    string `json:"quoteNumber"`
	CustomerName   string `json:"customerName"`
	ReceivedDate   string `json:"receivedDate"`
	Currency       string `json:"currency"`
	ID             int64  `json:"id"`
	CustomerID     int64  `json:"customerId"`
	SourceRevision int16  `json:"sourceRevision"`
	SourceMissing  bool   `json:"sourceMissing"`
}

type poListCustomer struct {
	Name string `json:"name"`
	ID   int64  `json:"id"`
}

type poListResponse struct {
	PurchaseOrders []poListRow      `json:"purchaseOrders"`
	Customers      []poListCustomer `json:"customers"`
	Page           int              `json:"page"`
	PageSize       int              `json:"pageSize"`
	Total          int              `json:"total"`
	TotalPages     int              `json:"totalPages"`
}

func listPurchaseOrders(t *testing.T, server *Server, token, query string) poListResponse {
	t.Helper()
	code, body := doJSON(t, server, http.MethodGet, "/v1/purchase-orders"+query, token, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /v1/purchase-orders%s = %d; want 200 (%v)", query, code, body)
	}
	var page poListResponse
	encodeDecode(t, body, &page)
	return page
}

// exportForPOList issues one Quotation for a chosen client code. The archive
// reads the PO's own customer snapshot, so the code is what makes two customer
// ids selectable.
func exportForPOList(t *testing.T, server *Server, token string, customerCode int, subject string) (int64, string) {
	t.Helper()
	input := poSourceInput(subject, "Archive item")
	input["CustomerID"] = customerCode
	return exportForSelector(t, server, token, input)
}

// createPOForAPITest creates one draft with an explicit received date so the
// date range and ordering are deterministic.
func createPOForAPITest(t *testing.T, server *Server, token string, quotationID int64, received string) purchaseOrderRow {
	t.Helper()
	code, body := createPODraft(t, server, token, quotationID, 0, map[string]any{"receivedDate": received})
	if code != http.StatusCreated {
		t.Fatalf("create PO = %d; want 201 (%v)", code, body)
	}
	return decodePurchaseOrder(t, body)
}

func setPOForAPITestNumber(t *testing.T, server *Server, token string, id int64, number string) {
	t.Helper()
	path := "/v1/purchase-orders/" + strconv.FormatInt(id, 10)
	code, body := doJSON(t, server, http.MethodGet, path, token, nil)
	if code != http.StatusOK {
		t.Fatalf("reopen PO %d = %d; want 200 (%v)", id, code, body)
	}
	var current struct {
		Version int64 `json:"version"`
	}
	encodeDecode(t, body, &current)
	code, body = doJSON(t, server, http.MethodPut, path, token,
		map[string]any{"poNumber": number, "expectedVersion": current.Version})
	if code != http.StatusOK {
		t.Fatalf("set PO number = %d; want 200 (%v)", code, body)
	}
}

func TestPurchaseOrderListPaginatesFifteenPerPage(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_page")

	for index := 1; index <= 16; index++ {
		customerCode := 2
		if index%2 == 0 {
			customerCode = 3
			seedQuotationClient(t, server.database, 3, "PT. Beta")
		}
		quotationID, _ := exportForPOList(t, server, token, customerCode, fmt.Sprintf("Archive quotation %02d", index))
		createPOForAPITest(t, server, token, quotationID, fmt.Sprintf("2026-01-%02d", index))
	}

	first := listPurchaseOrders(t, server, token, "?page=1")
	if first.Page != 1 || first.PageSize != 15 || first.Total != 16 || first.TotalPages != 2 {
		t.Errorf("page meta = %d/%d/%d/%d; want 1/15/16/2",
			first.Page, first.PageSize, first.Total, first.TotalPages)
	}
	if len(first.PurchaseOrders) != 15 {
		t.Fatalf("page 1 rows = %d; want 15", len(first.PurchaseOrders))
	}
	if got := first.PurchaseOrders[0].ReceivedDate; got != "2026-01-16" {
		t.Errorf("first received date = %s; want the newest 2026-01-16", got)
	}
	row := first.PurchaseOrders[0]
	if row.ID < 1 || row.UUID == "" || row.QuoteNumber == "" || row.CustomerName == "" {
		t.Errorf("row identity = %+v; want id, uuid, quote number and customer", row)
	}
	if row.State != "draft" || row.SourceRevision != 0 || row.SourceMissing {
		t.Errorf("row state = %+v; want a live draft at revision 0", row)
	}
	if row.Currency != "IDR" {
		t.Errorf("row currency = %q; want the issued revision's IDR", row.Currency)
	}

	second := listPurchaseOrders(t, server, token, "?page=2")
	if len(second.PurchaseOrders) != 1 {
		t.Fatalf("page 2 rows = %d; want 1", len(second.PurchaseOrders))
	}
	if got := second.PurchaseOrders[0].ReceivedDate; got != "2026-01-01" {
		t.Errorf("page 2 received date = %s; want the oldest 2026-01-01", got)
	}
}

func TestPurchaseOrderListSearchCoversTheThreePromisedFields(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_search")

	alphaID, alphaNumber := exportForPOList(t, server, token, 2, "Electrical")
	alpha := createPOForAPITest(t, server, token, alphaID, "2026-02-10")
	setPOForAPITestNumber(t, server, token, alpha.ID, "PO-ALPHA-1")

	seedQuotationClient(t, server.database, 3, "PT. Beta")
	betaID, _ := exportForPOList(t, server, token, 3, "Mechanical")
	beta := createPOForAPITest(t, server, token, betaID, "2026-02-11")
	setPOForAPITestNumber(t, server, token, beta.ID, "PO 100% REAL")

	underscoreID, _ := exportForPOList(t, server, token, 2, "Instrumentation")
	underscore := createPOForAPITest(t, server, token, underscoreID, "2026-02-12")
	setPOForAPITestNumber(t, server, token, underscore.ID, "PO_UNDERSCORE")

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"PO number, case-insensitive", "?search=po-alpha", 1},
		{"quotation number", "?search=" + alphaNumber, 1},
		{"customer name", "?search=" + url.QueryEscape(strings.ToUpper(beta.CustomerName)), 1},
		{"customer name matches both", "?search=INFINEON", 2},
		{"literal percent is not a wildcard", "?search=%25", 1},
		{"literal underscore is not a wildcard", "?search=_", 1},
		{"no match", "?search=zzz-nothing", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := listPurchaseOrders(t, server, token, tc.query)
			if len(page.PurchaseOrders) != tc.want || page.Total != tc.want {
				t.Fatalf("search %q rows = %d total = %d; want %d (%+v)",
					tc.query, len(page.PurchaseOrders), page.Total, tc.want, page.PurchaseOrders)
			}
			if page.TotalPages < 1 {
				t.Errorf("search %q total pages = %d; want at least 1", tc.query, page.TotalPages)
			}
		})
	}
}

func TestPurchaseOrderListFiltersByCustomerReceivedRangeAndState(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_filters")

	firstID, _ := exportForPOList(t, server, token, 2, "First")
	first := createPOForAPITest(t, server, token, firstID, "2026-02-10")
	setPOForAPITestNumber(t, server, token, first.ID, "PO-A")

	seedQuotationClient(t, server.database, 3, "PT. Beta")
	secondID, _ := exportForPOList(t, server, token, 3, "Second")
	second := createPOForAPITest(t, server, token, secondID, "2026-02-15")
	setPOForAPITestNumber(t, server, token, second.ID, "PO-B")
	setPurchaseOrderStateForTest(t, "vera_api_po_list_filters", second.ID, "saved")

	thirdID, _ := exportForPOList(t, server, token, 2, "Third")
	third := createPOForAPITest(t, server, token, thirdID, "2026-03-01")
	setPOForAPITestNumber(t, server, token, third.ID, "PO-C")

	customer := func(id int64) string { return "&customerId=" + strconv.FormatInt(id, 10) }
	cases := []struct {
		name  string
		query string
		want  []int64
	}{
		{"one customer", "?customerId=" + strconv.FormatInt(first.CustomerID, 10), []int64{third.ID, first.ID}},
		{"two customers", "?customerId=" + strconv.FormatInt(first.CustomerID, 10) + customer(second.CustomerID), []int64{third.ID, second.ID, first.ID}},
		{"received from inclusive", "?receivedFrom=2026-02-12", []int64{third.ID, second.ID}},
		{"received to inclusive", "?receivedTo=2026-02-15", []int64{second.ID, first.ID}},
		{"received range", "?receivedFrom=2026-02-10&receivedTo=2026-02-15", []int64{second.ID, first.ID}},
		{"saved state", "?state=saved", []int64{second.ID}},
		{"draft state plus customer", "?state=draft" + customer(first.CustomerID), []int64{third.ID, first.ID}},
		{"search and state combine", "?search=PO-&state=draft&receivedFrom=2026-02-20", []int64{third.ID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := listPurchaseOrders(t, server, token, tc.query)
			got := make([]int64, 0, len(page.PurchaseOrders))
			for _, row := range page.PurchaseOrders {
				got = append(got, row.ID)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ids = %v; want %v", got, tc.want)
			}
			for index := range tc.want {
				if got[index] != tc.want[index] {
					t.Fatalf("ids = %v; want %v", got, tc.want)
				}
			}
			if page.Total != len(tc.want) {
				t.Errorf("total = %d; want %d", page.Total, len(tc.want))
			}
		})
	}

	// A page past the end is an empty page, not a 404 or a wrapped page.
	past := listPurchaseOrders(t, server, token, "?page=99")
	if len(past.PurchaseOrders) != 0 || past.Page != 99 {
		t.Errorf("page 99 = %d rows page %d; want an empty page 99", len(past.PurchaseOrders), past.Page)
	}
}

func TestPurchaseOrderListCustomerFacetIsOrgWideAndIndependentOfThePage(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_customers")
	alphaID, _ := exportForPOList(t, server, token, 2, "Alpha")
	createPOForAPITest(t, server, token, alphaID, "2026-02-10")
	seedQuotationClient(t, server.database, 3, "PT. Beta")
	betaID, _ := exportForPOList(t, server, token, 3, "Beta")
	createPOForAPITest(t, server, token, betaID, "2026-02-11")

	// Search narrows the rows but the picker still offers both customers: a
	// filter that hides the other option would make multi-select unusable.
	page := listPurchaseOrders(t, server, token, "?search=INFINEON")
	if len(page.PurchaseOrders) != 1 {
		t.Fatalf("search rows = %d; want 1", len(page.PurchaseOrders))
	}
	if len(page.Customers) != 2 {
		t.Fatalf("customer facet = %+v; want both PO customers", page.Customers)
	}
	if page.Customers[0].Name > page.Customers[1].Name {
		t.Errorf("customer facet order = %+v; want alphabetical", page.Customers)
	}
	if page.Customers[0].ID == 0 || page.Customers[1].ID == 0 {
		t.Errorf("customer facet ids = %+v; want real ids for the filter", page.Customers)
	}
	if page.Customers[0].ID == page.Customers[1].ID {
		t.Errorf("customer facet ids = %+v; want two distinct customers", page.Customers)
	}
}

func TestPurchaseOrderListSourceMissingIsAcceptedButInert(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_source_missing")
	quotationID, _ := exportForPOList(t, server, token, 2, "Live source")
	order := createPOForAPITest(t, server, token, quotationID, "2026-02-10")

	only := listPurchaseOrders(t, server, token, "?sourceMissing=only")
	if len(only.PurchaseOrders) != 0 || only.Total != 0 {
		t.Errorf("sourceMissing=only = %+v; no record can be missing its source today", only.PurchaseOrders)
	}
	excluded := listPurchaseOrders(t, server, token, "?sourceMissing=exclude")
	if len(excluded.PurchaseOrders) != 1 || excluded.PurchaseOrders[0].ID != order.ID {
		t.Errorf("sourceMissing=exclude = %+v; want the live PO", excluded.PurchaseOrders)
	}
	if excluded.PurchaseOrders[0].SourceMissing {
		t.Error("a live PO claims a missing source")
	}
	any := listPurchaseOrders(t, server, token, "")
	if len(any.PurchaseOrders) != 1 || any.PurchaseOrders[0].SourceMissing {
		t.Errorf("default filter = %+v; want the live PO with sourceMissing false", any.PurchaseOrders)
	}
}

func TestPurchaseOrderListRejectsMalformedFilters(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_invalid")

	cases := []struct {
		name  string
		query string
	}{
		{"page zero", "?page=0"},
		{"page not a number", "?page=one"},
		{"unknown state", "?state=fulfilled"},
		{"unknown source-missing filter", "?sourceMissing=maybe"},
		{"customer not a number", "?customerId=alpha"},
		{"customer zero", "?customerId=0"},
		{"date not a date", "?receivedFrom=10-02-2026"},
		{"received from after to", "?receivedFrom=2026-02-11&receivedTo=2026-02-10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := doJSON(t, server, http.MethodGet, "/v1/purchase-orders"+tc.query, token, nil)
			if code != http.StatusBadRequest {
				t.Fatalf("GET %s = %d; want 400 (%v)", tc.query, code, body)
			}
		})
	}
}

// TestPurchaseOrderListRejectsAnUnrepresentablePage is the regression for a
// page whose (page-1)*15 offset overflows a signed 64-bit integer. The handler
// used to accept it and Postgres answered the negative OFFSET with an error,
// turning malformed input into a 500. 614891469123651722 is
// floor(MaxInt64/15)+2: one past the largest page with a representable offset.
func TestPurchaseOrderListRejectsAnUnrepresentablePage(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_page_overflow")

	const page = "614891469123651722"
	code, body := doJSON(t, server, http.MethodGet, "/v1/purchase-orders?page="+page, token, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("GET page=%s = %d; want 400, never 500 (%v)", page, code, body)
	}
}

// TestPurchaseOrderListAcceptsTheLargestRepresentablePage pins the other side
// of that boundary. The guard must reject only the pages whose offset cannot be
// represented; a page past the end stays an empty page, not a 404 or a clamp.
func TestPurchaseOrderListAcceptsTheLargestRepresentablePage(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_page_boundary")

	// floor(MaxInt64/15)+1. The exact echo is pinned at the store seam, where
	// the value stays an int64; this body-level helper decodes numbers as
	// float64 and would round a 19-digit page.
	const page = 614891469123651721
	result := listPurchaseOrders(t, server, token, "?page="+strconv.FormatInt(page, 10))
	if len(result.PurchaseOrders) != 0 {
		t.Errorf("rows = %d; want an empty page past the end", len(result.PurchaseOrders))
	}
	if result.PageSize != 15 || result.TotalPages < 1 {
		t.Errorf("page meta = %+v; want page size 15 and at least one total page", result)
	}
}

func TestPurchaseOrderListEmptyShape(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_empty")
	code, body := doJSON(t, server, http.MethodGet, "/v1/purchase-orders", token, nil)
	if code != http.StatusOK {
		t.Fatalf("empty list = %d; want 200 (%v)", code, body)
	}
	rows, ok := body["purchaseOrders"].([]any)
	if !ok || len(rows) != 0 {
		t.Errorf("purchaseOrders = %#v; want an empty JSON array", body["purchaseOrders"])
	}
	customers, ok := body["customers"].([]any)
	if !ok || len(customers) != 0 {
		t.Errorf("customers = %#v; want an empty JSON array", body["customers"])
	}
	var page poListResponse
	encodeDecode(t, body, &page)
	if page.Page != 1 || page.PageSize != 15 || page.Total != 0 || page.TotalPages != 1 {
		t.Errorf("empty page meta = %+v; want 1/15/0/1", page)
	}
}

func TestPurchaseOrderListRequiresASession(t *testing.T) {
	server, _ := testQuotationServer(t, "vera_api_po_list_auth")
	code, _ := doJSON(t, server, http.MethodGet, "/v1/purchase-orders", "", nil)
	if code != http.StatusUnauthorized {
		t.Errorf("unauthenticated list = %d; want 401", code)
	}
}

func TestPurchaseOrderListSearchEscapesLikeWildcardsAtTheHTTPSeam(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_list_escape")
	quotationID, _ := exportForPOList(t, server, token, 2, "Escaping")
	order := createPOForAPITest(t, server, token, quotationID, "2026-02-10")
	setPOForAPITestNumber(t, server, token, order.ID, `PO 50%_DONE\TEST`)

	literal := listPurchaseOrders(t, server, token, "?search="+url.QueryEscape(`50%_DONE\TEST`))
	if len(literal.PurchaseOrders) != 1 {
		t.Errorf("literal wildcard search rows = %d; want the one PO", len(literal.PurchaseOrders))
	}
	wild := listPurchaseOrders(t, server, token, "?search=%25")
	if len(wild.PurchaseOrders) != 1 {
		t.Errorf("bare percent rows = %d; want 1 (escaped, not all)", len(wild.PurchaseOrders))
	}
}
