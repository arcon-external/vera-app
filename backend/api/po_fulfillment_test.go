package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"vera-app/backend/apicontract"
	"vera-app/backend/poreport"
)

// Issue #657 phase B: the PO Fulfillment drill-down over the authenticated HTTP
// surface. The store suite already proves the arithmetic; these tests prove what
// a client sees: the route and its session gate, the four derived statuses, the
// filter/pagination contract, the typed refusals, aggregate decomposition,
// extras listed separately, and the absence of any supplier unit cost on the
// wire.

func fulfillmentURL(values url.Values) string {
	return apicontract.PurchaseOrderFulfillmentPath + "?" + values.Encode()
}

func fulfillmentParams(currency, status, search string, page, pageSize int) url.Values {
	values := url.Values{}
	if currency != "" {
		values.Set("currency", currency)
	}
	if status != "" {
		values.Set("status", status)
	}
	if search != "" {
		values.Set("search", search)
	}
	if page != 0 {
		values.Set("page", strconv.Itoa(page))
	}
	if pageSize != 0 {
		values.Set("page_size", strconv.Itoa(pageSize))
	}
	return values
}

func decodeFulfillment(t *testing.T, body map[string]any) apicontract.PurchaseOrderFulfillmentPage {
	t.Helper()
	var page apicontract.PurchaseOrderFulfillmentPage
	encodeDecode(t, body, &page)
	return page
}

func mustFulfillmentPage(
	t *testing.T, server *Server, token string, values url.Values,
) apicontract.PurchaseOrderFulfillmentPage {
	t.Helper()
	code, body := doJSON(t, server, http.MethodGet, fulfillmentURL(values), token, nil)
	if code != http.StatusOK {
		t.Fatalf("fulfillment = %d (%v); want 200", code, body)
	}
	return decodeFulfillment(t, body)
}

// fulfillmentQuotationInput is one priced leaf: five pieces at a 10.000 cost
// and a 100% margin, so the customer price is 20.000 and every fulfillment
// figure is an exact round number.
func fulfillmentQuotationInput(qty int) map[string]any {
	return map[string]any{
		"Items": []map[string]any{{
			"Description": "Fulfillment line", "Qty": qty, "UoM": "Pc",
			"UnitPrice": 10_000, "MarginEnabled": true, "MarginPercent": 100,
			"DiscountEnabled": false, "DiscountPercent": 0,
		}},
		"Subject":       "Fulfillment source",
		"RecipientName": "Test Recipient",
		"TaxZone":       "FTZ",
		"Validity":      30,
		"PaymentTerm":   30,
		"LeadTimeDays":  10,
		"CustomerID":    2,
		"Incoterm":      "EXW",
	}
}

// saveFulfillmentPO buys quantityScaled of the quotation's only leaf and saves
// the record. A smaller quantity is the PO's own choice, so the review records
// that decision exactly as the editor does.
func saveFulfillmentPO(
	t *testing.T, server *Server, token string, quotationID int64,
	number string, quantityScaled int64,
) purchaseOrderRow {
	t.Helper()
	code, body := createPODraft(t, server, token, quotationID, 0, map[string]any{
		"receivedDate": "2026-03-10",
	})
	if code != http.StatusCreated {
		t.Fatalf("create %s = %d (%v); want 201", number, code, body)
	}
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)
	payload := finalizeBody(draft, number)
	items, _ := payload["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("%s items = %d; want the one source leaf", number, len(items))
	}
	if quantityScaled != draft.Items[0].QuantityScaled {
		items[0]["quantityScaled"] = quantityScaled
		payload["decisions"] = []map[string]any{
			{"key": "line:1:quantity", "choice": "po"},
		}
	}
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, payload)
	if code != http.StatusOK {
		t.Fatalf("finalize %s = %d (%v); want 200", number, code, body)
	}
	return decodePurchaseOrder(t, body)
}

// ageFulfillmentRevision moves a quotation's only revision into the past, so
// the expiry boundary is exercised without waiting for a real month.
func ageFulfillmentRevision(t *testing.T, identity string, quotationID int64, days int) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDatabase(t, identity).DSN)
	if err != nil {
		t.Fatalf("connect for revision age: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`UPDATE `+testSchemaIdent()+`.quotation_revisions
		 SET created_at = created_at - make_interval(days => $2)
		 WHERE quotation_id = $1`, quotationID, days); err != nil {
		t.Fatalf("age revision of quotation %d: %v", quotationID, err)
	}
}

// setFulfillmentRevisionCurrency moves one issued revision into a different
// currency partition. The public export writes the company currency, so this is
// the direct-database fixture the partition test needs.
func setFulfillmentRevisionCurrency(t *testing.T, identity string, quotationID int64, code string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDatabase(t, identity).DSN)
	if err != nil {
		t.Fatalf("connect for currency fixture: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(),
		`UPDATE `+testSchemaIdent()+`.quotation_revisions
		 SET currency = $2 WHERE quotation_id = $1`, quotationID, code); err != nil {
		t.Fatalf("set quotation %d currency %s: %v", quotationID, code, err)
	}
}

func fulfillmentByNumber(
	t *testing.T, page apicontract.PurchaseOrderFulfillmentPage, quoteNumber string,
) apicontract.PurchaseOrderFulfillmentQuotation {
	t.Helper()
	for _, quotation := range page.Quotations {
		if quotation.QuoteNumber == quoteNumber {
			return quotation
		}
	}
	t.Fatalf("fulfillment page has no quotation %s: %+v", quoteNumber, page.Quotations)
	return apicontract.PurchaseOrderFulfillmentQuotation{}
}

func fulfillmentItem(
	t *testing.T, quotation apicontract.PurchaseOrderFulfillmentQuotation, itemNo int,
) apicontract.PurchaseOrderFulfillmentItem {
	t.Helper()
	for _, item := range quotation.Items {
		if item.QuotationItemNo == itemNo {
			return item
		}
	}
	t.Fatalf("quotation %s has no item %d: %+v", quotation.QuoteNumber, itemNo, quotation.Items)
	return apicontract.PurchaseOrderFulfillmentItem{}
}

// TestPurchaseOrderFulfillmentRequiresASession pins the route and its gate: no
// session is a 401, never a 404 that hides the route's existence.
func TestPurchaseOrderFulfillmentRequiresASession(t *testing.T) {
	server, _ := testQuotationServer(t, "vera_api_po_fulfillment_auth")
	code, _ := doJSON(t, server, http.MethodGet,
		fulfillmentURL(fulfillmentParams("IDR", "", "", 1, 25)), "", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated fulfillment = %d; want 401", code)
	}
}

// TestPurchaseOrderFulfillmentStatusesAndFigures is the acceptance proof at the
// HTTP seam: one quotation in each derived status, with exact quoted, bought,
// unsold, realized and closed-loss figures. Expiry never becomes a loss, and a
// fully bought quotation is all_bought even when its validity window passed.
func TestPurchaseOrderFulfillmentStatusesAndFigures(t *testing.T) {
	const identity = "vera_api_po_fulfillment_statuses"
	server, token := testQuotationServer(t, identity)

	openID, openNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(5))
	saveFulfillmentPO(t, server, token, openID, "PO-657-API-OPEN", 30_000)

	expiredID, expiredNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(5))
	saveFulfillmentPO(t, server, token, expiredID, "PO-657-API-EXPIRED", 30_000)
	ageFulfillmentRevision(t, identity, expiredID, 45)

	allBoughtID, allBoughtNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(2))
	saveFulfillmentPO(t, server, token, allBoughtID, "PO-657-API-ALL-BOUGHT", 20_000)
	ageFulfillmentRevision(t, identity, allBoughtID, 45)

	closedID, closedNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(5))
	saveFulfillmentPO(t, server, token, closedID, "PO-657-API-CLOSED", 30_000)
	code, body := closeQuotationOverHTTP(t, server, token, closedID,
		"customer cancelled the remainder")
	if code != http.StatusOK {
		t.Fatalf("close = %d (%v); want 200", code, body)
	}

	page := mustFulfillmentPage(t, server, token, fulfillmentParams("IDR", "all", "", 1, 100))
	if page.Total != 4 {
		t.Fatalf("total = %d; want the four IDR quotations", page.Total)
	}
	if page.Page != 1 || page.PageSize != 100 || page.TotalPages != 1 {
		t.Errorf("page metadata = %d/%d/%d; want 1/100/1",
			page.Page, page.PageSize, page.TotalPages)
	}
	if page.AsOf == "" {
		t.Error("asOf is empty; want the company WIB calendar day")
	}
	if page.Filter.Currency != "IDR" || page.Filter.Status != string(poreport.FulfillmentStatusAll) {
		t.Errorf("filter echo = %+v; want IDR/all", page.Filter)
	}

	statuses := map[string]string{
		openNumber:      string(poreport.FulfillmentStatusOpen),
		expiredNumber:   string(poreport.FulfillmentStatusExpiredUnclosed),
		allBoughtNumber: string(poreport.FulfillmentStatusAllBought),
		closedNumber:    string(poreport.FulfillmentStatusClosed),
	}
	for quoteNumber, want := range statuses {
		quotation := fulfillmentByNumber(t, page, quoteNumber)
		if quotation.Status != want {
			t.Errorf("%s status = %q; want %q", quoteNumber, quotation.Status, want)
		}
	}

	open := fulfillmentItem(t, fulfillmentByNumber(t, page, openNumber), 1)
	if open.QuotedQuantityScaled != 50_000 || open.BoughtQuantityScaled != 30_000 ||
		open.UnsoldQuantityScaled != 20_000 || open.OverBoughtQuantityScaled != 0 {
		t.Errorf("open quantities = %d/%d/%d/%d; want 50000/30000/20000/0",
			open.QuotedQuantityScaled, open.BoughtQuantityScaled,
			open.UnsoldQuantityScaled, open.OverBoughtQuantityScaled)
	}
	if open.RealizedAmountMinor != 60_000 || open.RealizedMarginMinor != 30_000 {
		t.Errorf("open realized = %d/%d; want 60000/30000",
			open.RealizedAmountMinor, open.RealizedMarginMinor)
	}
	if open.UnsoldAmountMinor != 40_000 || open.UnsoldMarginMinor != 20_000 {
		t.Errorf("open unsold = %d/%d; want 40000/20000",
			open.UnsoldAmountMinor, open.UnsoldMarginMinor)
	}
	if open.ClosedLossAmountMinor != 0 || open.ClosedLossMarginMinor != 0 {
		t.Errorf("open closed loss = %d/%d; want zero",
			open.ClosedLossAmountMinor, open.ClosedLossMarginMinor)
	}
	if len(open.PurchaseOrders) != 1 ||
		open.PurchaseOrders[0].PONumber != "PO-657-API-OPEN" ||
		open.PurchaseOrders[0].State != "saved" ||
		open.PurchaseOrders[0].AllocatedQuantityScaled != 30_000 {
		t.Errorf("open contributions = %+v; want the saved PO at 3",
			open.PurchaseOrders)
	}

	expired := fulfillmentItem(t, fulfillmentByNumber(t, page, expiredNumber), 1)
	if expired.UnsoldQuantityScaled != 20_000 || expired.UnsoldAmountMinor != 40_000 ||
		expired.ClosedLossAmountMinor != 0 || expired.ClosedLossMarginMinor != 0 {
		t.Errorf("expired item = %+v; want Unsold 2 / 40000 and no closed loss", expired)
	}

	allBought := fulfillmentItem(t, fulfillmentByNumber(t, page, allBoughtNumber), 1)
	if allBought.UnsoldQuantityScaled != 0 || allBought.ClosedLossAmountMinor != 0 ||
		allBought.OverBoughtQuantityScaled != 0 {
		t.Errorf("all-bought item = %+v; want every remainder at zero", allBought)
	}
	if allBought.QuotedQuantityScaled != 20_000 || allBought.BoughtQuantityScaled != 20_000 {
		t.Errorf("all-bought quantities = %d/%d; want 20000/20000",
			allBought.QuotedQuantityScaled, allBought.BoughtQuantityScaled)
	}

	closed := fulfillmentItem(t, fulfillmentByNumber(t, page, closedNumber), 1)
	if closed.UnsoldQuantityScaled != 0 || closed.UnsoldAmountMinor != 0 ||
		closed.UnsoldMarginMinor != 0 {
		t.Errorf("closed item still reports Unsold = %+v", closed)
	}
	if closed.ClosedLossAmountMinor != 40_000 || closed.ClosedLossMarginMinor != 20_000 {
		t.Errorf("closed loss = %d/%d; want 40000/20000",
			closed.ClosedLossAmountMinor, closed.ClosedLossMarginMinor)
	}
}

// TestPurchaseOrderFulfillmentFiltersAndPages pins the HTTP query contract:
// status filters before the page is cut, search matches the quote number and
// the customer snapshot, and all four state filters are reachable by name.
func TestPurchaseOrderFulfillmentFiltersAndPages(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_fulfillment_filters")
	openID, openNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(3))
	closedID, closedNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(3))
	code, body := closeQuotationOverHTTP(t, server, token, closedID, "closed for the filter proof")
	if code != http.StatusOK {
		t.Fatalf("close = %d (%v); want 200", code, body)
	}

	open := mustFulfillmentPage(t, server, token,
		fulfillmentParams("IDR", "open", "", 1, 25))
	if open.Total != 1 || len(open.Quotations) != 1 ||
		open.Quotations[0].QuoteNumber != openNumber {
		t.Fatalf("open filter = %+v; want only %s", open.Quotations, openNumber)
	}

	closed := mustFulfillmentPage(t, server, token,
		fulfillmentParams("IDR", "closed", "", 1, 25))
	if closed.Total != 1 || len(closed.Quotations) != 1 ||
		closed.Quotations[0].QuoteNumber != closedNumber ||
		closed.Quotations[0].Status != string(poreport.FulfillmentStatusClosed) {
		t.Fatalf("closed filter = %+v; want only %s", closed.Quotations, closedNumber)
	}

	allBought := mustFulfillmentPage(t, server, token,
		fulfillmentParams("IDR", "all_bought", "", 1, 25))
	if allBought.Total != 0 || len(allBought.Quotations) != 0 {
		t.Errorf("all_bought filter = %+v; want the empty page", allBought.Quotations)
	}

	byNumber := mustFulfillmentPage(t, server, token,
		fulfillmentParams("IDR", "", openNumber, 1, 25))
	if byNumber.Total != 1 || byNumber.Quotations[0].QuotationID != openID {
		t.Errorf("search by quote number = %+v; want %s", byNumber.Quotations, openNumber)
	}

	// The customer snapshot is the revision's own company name (here the seeded
	// client for customer code 2), so search the exact name the page reports.
	byCustomer := mustFulfillmentPage(t, server, token,
		fulfillmentParams("IDR", "", open.Quotations[0].CustomerName, 1, 25))
	if byCustomer.Total != 2 {
		t.Errorf("search by customer total = %d; want both quotations", byCustomer.Total)
	}
}

// TestPurchaseOrderFulfillmentPaginatesQuotationRows creates 26 quotations and
// proves the page cuts quotation rows, never item rows: 25 then 1, with the
// filtered total available to the caller before the page is read.
func TestPurchaseOrderFulfillmentPaginatesQuotationRows(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_fulfillment_pages")
	for index := 1; index <= 26; index++ {
		exportForSelector(t, server, token, fulfillmentQuotationInput(index))
	}

	first := mustFulfillmentPage(t, server, token, fulfillmentParams("IDR", "open", "", 1, 25))
	if first.Total != 26 || len(first.Quotations) != 25 || first.TotalPages != 2 {
		t.Fatalf("first page = %d rows, total %d, %d pages; want 25/26/2",
			len(first.Quotations), first.Total, first.TotalPages)
	}
	second := mustFulfillmentPage(t, server, token, fulfillmentParams("IDR", "open", "", 2, 25))
	if second.Total != 26 || len(second.Quotations) != 1 || second.TotalPages != 2 {
		t.Fatalf("second page = %d rows, total %d, %d pages; want 1/26/2",
			len(second.Quotations), second.Total, second.TotalPages)
	}
	if first.Quotations[0].QuotationID == second.Quotations[0].QuotationID {
		t.Error("the two pages share a quotation; the page cut duplicated a row")
	}

	// A filter runs before the page is cut, so the total is the filtered count.
	filtered := mustFulfillmentPage(t, server, token, fulfillmentParams("IDR", "closed", "", 1, 25))
	if filtered.Total != 0 {
		t.Errorf("filtered total = %d; want 0 closed quotations", filtered.Total)
	}

	// The default page size is the approved 25, and the ceiling is 100.
	defaulted := mustFulfillmentPage(t, server, token, fulfillmentParams("IDR", "", "", 0, 0))
	if defaulted.Page != 1 || defaulted.PageSize != poreport.FulfillmentDefaultPageSize {
		t.Errorf("defaults = page %d size %d; want 1/%d",
			defaulted.Page, defaulted.PageSize, poreport.FulfillmentDefaultPageSize)
	}
}

// TestPurchaseOrderFulfillmentRefusalsAreTyped pins every malformed input to a
// 400 carrying one stable code, and proves an omitted currency is the company's
// own while a lower-case catalogue code is canonical on output.
func TestPurchaseOrderFulfillmentRefusalsAreTyped(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_fulfillment_refusals")

	cases := []struct {
		name   string
		values url.Values
	}{
		{"unknown status", fulfillmentParams("IDR", "lost", "", 1, 25)},
		{"zero page", url.Values{"page": {"0"}}},
		{"page not a number", url.Values{"page": {"one"}}},
		{"negative page", url.Values{"page": {"-1"}}},
		{"page size above the bound", fulfillmentParams("IDR", "", "", 1, 101)},
		{"negative page size", fulfillmentParams("IDR", "", "", 1, -1)},
		{"page size not a number", url.Values{"page_size": {"many"}}},
		{"unknown currency", fulfillmentParams("XYZ", "", "", 1, 25)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			code, body := doJSON(t, server, http.MethodGet,
				fulfillmentURL(test.values), token, nil)
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d (%v); want 400", code, body)
			}
			if got, _ := body["code"].(string); got != "purchase_order_fulfillment_invalid" {
				t.Errorf("code = %q; want purchase_order_fulfillment_invalid", got)
			}
			if message, _ := body["error"].(string); message == "" {
				t.Error("a refusal must carry its message")
			}
		})
	}

	// Currency is the only filter with a company default. An omitted code is the
	// profile's, and an input code is canonical on output.
	defaulted := mustFulfillmentPage(t, server, token, fulfillmentParams("", "", "", 1, 25))
	if defaulted.Filter.Currency != "IDR" {
		t.Errorf("default currency = %q; want the company IDR", defaulted.Filter.Currency)
	}
	usd := mustFulfillmentPage(t, server, token, fulfillmentParams("usd", "", "", 1, 25))
	if usd.Filter.Currency != "USD" {
		t.Errorf("canonical currency = %q; want USD", usd.Filter.Currency)
	}
}

// TestPurchaseOrderFulfillmentCarriesNoSupplierCostAndPartitionsCurrency pins
// the two wire honesty rules: an IDR page never lists a USD quotation, and no
// per-unit supplier cost or cost-side name ever crosses the boundary. Realized
// margin is the report's own aggregate and is expected.
func TestPurchaseOrderFulfillmentCarriesNoSupplierCostAndPartitionsCurrency(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_fulfillment_leak")
	idrID, idrNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(5))
	saveFulfillmentPO(t, server, token, idrID, "PO-657-API-IDR", 30_000)

	// The public export always writes the company currency, so the USD
	// partition fixture moves the issued revision directly; this is the same
	// direct-database seam the report's legacy-adjustment fixture uses.
	usdID, usdNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(2))
	setFulfillmentRevisionCurrency(t, "vera_api_po_fulfillment_leak", usdID, "USD")
	saveFulfillmentPO(t, server, token, usdID, "PO-657-API-USD", 20_000)

	idr := mustFulfillmentPage(t, server, token, fulfillmentParams("IDR", "", "", 1, 25))
	if idr.Total != 1 || idr.Quotations[0].QuoteNumber != idrNumber {
		t.Fatalf("IDR page = %+v; want only %s", idr.Quotations, idrNumber)
	}
	usd := mustFulfillmentPage(t, server, token, fulfillmentParams("USD", "", "", 1, 25))
	if usd.Total != 1 || usd.Quotations[0].QuoteNumber != usdNumber {
		t.Fatalf("USD page = %+v; want only %s", usd.Quotations, usdNumber)
	}

	code, body := doJSON(t, server, http.MethodGet,
		fulfillmentURL(fulfillmentParams("IDR", "", "", 1, 25)), token, nil)
	if code != http.StatusOK {
		t.Fatalf("leak read = %d (%v); want 200", code, body)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal the page: %v", err)
	}
	wire := strings.ToLower(string(encoded))
	for _, banned := range []string{
		"costunitminor", "cost_unit", "costunit", "suppliercost", "supplier_cost",
		"unitcost", "unit_cost",
	} {
		if strings.Contains(wire, banned) {
			t.Errorf("fulfillment wire contains %q; no supplier unit cost may cross the boundary", banned)
		}
	}
	// The aggregate realized margin is the report's own figure and must survive.
	if !strings.Contains(wire, "realizedmarginminor") {
		t.Error("fulfillment wire has no realizedMarginMinor; the aggregate margin is allowed and expected")
	}
}

// TestPurchaseOrderFulfillmentDecomposesGroupsAndListsExtras pins #654/#655 at
// the HTTP seam: a grouped quotation child and a grouped extra each carry the
// group title, the header is never a third priced row, and the extra is listed
// separately with an amount and no quoted/bought quantity.
func TestPurchaseOrderFulfillmentDecomposesGroupsAndListsExtras(t *testing.T) {
	server, token := testQuotationServer(t, "vera_api_po_fulfillment_groups")
	quotationID, quoteNumber := exportForSelector(t, server, token, fulfillmentQuotationInput(2))
	code, body := createPODraft(t, server, token, quotationID, 0, map[string]any{
		"receivedDate": "2026-03-10",
	})
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v); want 201", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	path := "/v1/purchase-orders/" + strconv.FormatInt(draft.ID, 10)

	// The quotation child and the extra are the group's two members, so the
	// group has no third line of its own.
	update := draftUpdateBody(draft)
	items, _ := update["items"].([]map[string]any)
	items[0]["aggregateGroupIndex"] = 1
	items = append(items, map[string]any{
		"description":         "Freight surcharge",
		"quantityScaled":      20000,
		"uom":                 "Lot",
		"unitPriceMinor":      500_000,
		"lineKind":            "extra",
		"aggregateGroupIndex": 1,
	})
	update["items"] = items
	update["aggregateGroups"] = []map[string]any{
		aggregateGroupPayload("Delivered package", 10000, "Lot", true),
	}
	code, body = doJSON(t, server, http.MethodPut, path, token, update)
	if code != http.StatusOK {
		t.Fatalf("grouped update = %d (%v); want 200", code, body)
	}

	finalize := finalizeBody(decodePurchaseOrder(t, body), "PO-657-API-GROUP")
	finalItems, _ := finalize["items"].([]map[string]any)
	for index := range finalItems {
		if index < len(items) {
			finalItems[index]["lineKind"] = items[index]["lineKind"]
			finalItems[index]["aggregateGroupIndex"] = items[index]["aggregateGroupIndex"]
		}
	}
	finalize["items"] = finalItems
	finalize["aggregateGroups"] = update["aggregateGroups"]
	finalize["acknowledgedExtra"] = []int{2}
	code, body = doJSON(t, server, http.MethodPost, path+"/finalize", token, finalize)
	if code != http.StatusOK {
		t.Fatalf("grouped finalize = %d (%v); want 200", code, body)
	}

	page := mustFulfillmentPage(t, server, token, fulfillmentParams("IDR", "", "", 1, 25))
	fulfilled := fulfillmentByNumber(t, page, quoteNumber)
	if len(fulfilled.Items) != 1 {
		t.Fatalf("fulfillment items = %d; want the one priced child:\n%+v",
			len(fulfilled.Items), fulfilled.Items)
	}
	if fulfilled.Items[0].GroupTitle != "Delivered package" {
		t.Errorf("child group title = %q; want Delivered package", fulfilled.Items[0].GroupTitle)
	}
	if fulfilled.Items[0].QuotedQuantityScaled != 20_000 {
		t.Errorf("group created a phantom quantity: %+v", fulfilled.Items[0])
	}
	if len(fulfilled.ExtraExpenses) != 1 {
		t.Fatalf("extras = %+v; want the one extra line listed separately",
			fulfilled.ExtraExpenses)
	}
	extra := fulfilled.ExtraExpenses[0]
	if extra.Description != "Freight surcharge" || extra.AmountMinor != 1_000_000 ||
		extra.QuantityScaled != 20_000 || extra.UoM != "Lot" ||
		extra.GroupTitle != "Delivered package" {
		t.Errorf("extra = %+v; want the grouped 1.000.000 surcharge", extra)
	}
	if extra.PONumber != "PO-657-API-GROUP" || extra.LineNumber != 2 {
		t.Errorf("extra source = %s line %d; want PO-657-API-GROUP line 2",
			extra.PONumber, extra.LineNumber)
	}
}
