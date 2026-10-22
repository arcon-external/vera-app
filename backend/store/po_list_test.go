package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Issue #469: the PO archive reads purchase_orders, not quotations. These
// tests drive the real store against a disposable schema: the default 15-row
// page and a caller-chosen one (#492), newest received first, search over the
// three promised fields, the customer/date/state filters, the source-missing
// predicate, and the #465 organization boundary.
//
// They observe stored behaviour only: the returned rows and their count. The
// SQL behind them may change entirely.

// seedPOListQuotation writes one issued revision for a chosen client code. The
// customer code is a parameter because the multi-customer filter needs two
// distinct customer ids, and the code is what resolves them.
func seedPOListQuotation(t *testing.T, database *Store, code int16, customerName, subject string) QuotationRecord {
	t.Helper()
	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     code,
		CustomerName:     customerName,
		CustomerAddress:  "Jl. List " + customerName,
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          subject,
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     10,
		TaxZone:          "NonFTZ",
		TaxCategory:      "SupplyInstall",
		Currency:         "IDR",
		HTMLSnapshot:     "<html>snapshot</html>",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "List item",
			Qty:                   1,
			UoM:                   "Lot",
			UnitPrice:             1000,
			OriginalTotal:         1000,
			MarginalizedUnitPrice: 1000,
			MarginalizedTotal:     1000,
			ListUnitPrice:         1000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:          1000,
			OriginalGrand:             1000,
			MarginalizedSubtotal:      1000,
			MarginalizedGrand:         1000,
			MarginalizedGrandAfterTax: 1000,
			CustomerGrandTotal:        1000,
		},
	})
	if err != nil {
		t.Fatalf("seed list quotation %q: %v", subject, err)
	}
	return record
}

// createPOForList creates one PO from a seeded revision and an explicit
// received date, so ordering and date-range assertions are deterministic.
func createPOForList(t *testing.T, database *Store, ctx context.Context, quotation QuotationRecord, user AuthUser, received string) PurchaseOrder {
	t.Helper()
	var receivedDate *time.Time
	if received != "" {
		parsed, err := time.Parse("2006-01-02", received)
		if err != nil {
			t.Fatalf("parse received date %q: %v", received, err)
		}
		receivedDate = &parsed
	}
	order, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		ReceivedDate:   receivedDate,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO for quotation %d: %v", quotation.ID, err)
	}
	return order
}

// setPOListNumber stores the customer PO number, the field the search promises
// to cover. Update replaces the lines; the copied ones are passed back.
func setPOListNumber(t *testing.T, database *Store, ctx context.Context, order PurchaseOrder, user AuthUser, number string) PurchaseOrder {
	t.Helper()
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID:           order.ID,
		PONumber:     number,
		UpdatedBy:    user.ID,
		PaymentTerms: order.PaymentTerms,
		Items:        order.Items,
	})
	if err != nil {
		t.Fatalf("set PO number %q: %v", number, err)
	}
	return updated
}

// setPOListState moves a stored record into the saved state. Only #471 may
// save through the API, so the state filter reaches the column directly.
func setPOListState(t *testing.T, database *Store, ctx context.Context, id int64, state string) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	// A saved row must carry its final identity (migration 032). The archive
	// test only needs the state, so seed a synthetic number and actor rather
	// than pretending the #471 final-save rules do not exist.
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET
			state = $2,
			po_number = CASE WHEN $2 = 'saved'
				THEN COALESCE(NULLIF(btrim(po_number), ''), 'PO-LIST-' || id)
				ELSE po_number END,
			po_number_key = CASE WHEN $2 = 'saved'
				THEN lower(COALESCE(NULLIF(btrim(po_number), ''), 'PO-LIST-' || id))
				ELSE NULL END,
			saved_at = CASE WHEN $2 = 'saved' THEN NOW() ELSE NULL END,
			saved_by = CASE WHEN $2 = 'saved'
				THEN (SELECT id FROM %s.users ORDER BY id LIMIT 1)
				ELSE NULL END
		WHERE id = $1`, database.configSchema(), database.configSchema()), id, state); err != nil {
		t.Fatalf("set PO %d state %s: %v", id, state, err)
	}
}

func TestListPurchaseOrdersPaginatesFifteenPerPageNewestReceivedFirst(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_page")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	orders := make([]PurchaseOrder, 0, 16)
	for index := 1; index <= 16; index++ {
		code := int16(2)
		name := "PT. Alpha"
		if index%2 == 0 {
			code = 3
			name = "PT. Beta"
		}
		quotation := seedPOListQuotation(t, database, code, name, fmt.Sprintf("List quotation %02d", index))
		received := fmt.Sprintf("2026-01-%02d", index)
		orders = append(orders, createPOForList(t, database, ctx, quotation, user, received))
	}

	first, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{Page: 1})
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if first.PageSize != PurchaseOrderListPageSize || PurchaseOrderListPageSize != 15 {
		t.Errorf("default page size = %d (constant %d); want 15", first.PageSize, PurchaseOrderListPageSize)
	}
	if first.Page != 1 || first.Total != 16 || first.TotalPages != 2 {
		t.Errorf("page meta = page %d total %d pages %d; want 1/16/2",
			first.Page, first.Total, first.TotalPages)
	}
	if len(first.Rows) != 15 {
		t.Fatalf("page 1 rows = %d; want 15", len(first.Rows))
	}
	// Newest received first: the last created PO carries 2026-01-16.
	if got := dateOnly(first.Rows[0].ReceivedDate); got != "2026-01-16" {
		t.Errorf("first row received date = %s; want 2026-01-16", got)
	}
	if got := dateOnly(first.Rows[14].ReceivedDate); got != "2026-01-02" {
		t.Errorf("fifteenth row received date = %s; want 2026-01-02", got)
	}

	row := first.Rows[0]
	newest := orders[15]
	if row.ID != newest.ID {
		t.Errorf("first row id = %d; want newest PO %d", row.ID, newest.ID)
	}
	if row.UUID == "" || row.QuoteNumber == "" || row.CustomerName != "PT. Beta" {
		t.Errorf("row identity = %+v; want uuid, quote number and customer", row)
	}
	if row.State != PurchaseOrderStateDraft {
		t.Errorf("row state = %q; want draft", row.State)
	}
	if row.CustomerID != newest.CustomerID || row.SourceRevision != newest.SourceRevision {
		t.Errorf("row source = customer %d revision %d; want %d/%d",
			row.CustomerID, row.SourceRevision, newest.CustomerID, newest.SourceRevision)
	}
	if row.SourceMissing {
		t.Error("a live PO claims a missing source; deletion has not shipped")
	}
	if row.Currency != "IDR" {
		t.Errorf("row currency = %q; want the PO snapshot IDR", row.Currency)
	}

	second, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{Page: 2})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(second.Rows) != 1 {
		t.Fatalf("page 2 rows = %d; want 1", len(second.Rows))
	}
	if got := dateOnly(second.Rows[0].ReceivedDate); got != "2026-01-01" {
		t.Errorf("page 2 row received date = %s; want the oldest 2026-01-01", got)
	}

	// The customer facet is org-wide and alphabetical, independent of the
	// page: both customers are offered even though page 1 only names the one
	// whose PO is newest.
	if len(first.Customers) != 2 {
		t.Fatalf("customer facet = %+v; want the two PO customers", first.Customers)
	}
	if first.Customers[0].Name != "PT. Alpha" || first.Customers[1].Name != "PT. Beta" {
		t.Errorf("customer facet order = %+v; want alphabetical", first.Customers)
	}
}

func TestListPurchaseOrdersHonoursARequestedPageSize(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_page_size")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	const total = 21
	for index := 1; index <= total; index++ {
		quotation := seedPOListQuotation(t, database, 2, "PT. Alpha",
			fmt.Sprintf("Page-size quotation %02d", index))
		createPOForList(t, database, ctx, quotation, user, fmt.Sprintf("2026-01-%02d", index))
	}

	// Each case pins one thing: the effective size the response reports, the
	// rows LIMIT actually returned, and the page count derived from the same
	// size. 21 rows make 15, 20, 8, 1 and 100 all distinguishable.
	cases := []struct {
		name      string
		pageSize  int
		wantSize  int
		wantRows  int
		wantPages int
	}{
		{"absent keeps the default", 0, 15, 15, 2},
		{"custom twenty", 20, 20, 20, 2},
		{"custom eight", 8, 8, 8, 3},
		{"custom one", 1, 1, 1, 21},
		{"upper bound is accepted", PurchaseOrderListMaxPageSize, 100, 21, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{Page: 1, PageSize: tc.pageSize})
			if err != nil {
				t.Fatalf("page size %d: %v", tc.pageSize, err)
			}
			if page.PageSize != tc.wantSize {
				t.Errorf("reported page size = %d; want the effective %d", page.PageSize, tc.wantSize)
			}
			if len(page.Rows) != tc.wantRows {
				t.Errorf("rows = %d; want %d", len(page.Rows), tc.wantRows)
			}
			if page.Total != total || page.TotalPages != tc.wantPages {
				t.Errorf("total = %d pages = %d; want %d/%d",
					page.Total, page.TotalPages, total, tc.wantPages)
			}
		})
	}

	// Offset uses the same effective size as LIMIT: at 20 a page, page 2 holds
	// exactly the oldest PO instead of repeating page 1's rows.
	second, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{Page: 2, PageSize: 20})
	if err != nil {
		t.Fatalf("page 2 at size 20: %v", err)
	}
	if len(second.Rows) != 1 || dateOnly(second.Rows[0].ReceivedDate) != "2026-01-01" {
		t.Fatalf("page 2 at size 20 = %+v; want the oldest row only", second.Rows)
	}

	// Out-of-range sizes are refused, never clamped to the nearest bound: a
	// silently shortened page would look like a correct answer.
	for _, invalid := range []int{-1, PurchaseOrderListMaxPageSize + 1} {
		if _, err := database.ListPurchaseOrders(ctx,
			PurchaseOrderListQuery{Page: 1, PageSize: invalid}); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("page size %d error = %v; want ErrPurchaseOrderInvalid", invalid, err)
		}
	}
}

func TestListPurchaseOrdersSearchesPONumberQuoteNumberAndCustomerName(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_search")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	alphaQuote := seedPOListQuotation(t, database, 2, "PT. Alpha", "Electrical")
	alpha := setPOListNumber(t, database, ctx,
		createPOForList(t, database, ctx, alphaQuote, user, "2026-02-10"), user, "PO-ALPHA-1")

	betaQuote := seedPOListQuotation(t, database, 3, "PT. Beta", "Mechanical")
	setPOListNumber(t, database, ctx,
		createPOForList(t, database, ctx, betaQuote, user, "2026-02-11"), user, "PO 100% REAL")

	underscoreQuote := seedPOListQuotation(t, database, 2, "PT. Alpha", "Instrumentation")
	setPOListNumber(t, database, ctx,
		createPOForList(t, database, ctx, underscoreQuote, user, "2026-02-12"), user, "PO_UNDERSCORE")

	cases := []struct {
		name   string
		search string
		want   int
	}{
		{"PO number, case-insensitive", "po-alpha", 1},
		{"quotation number", alphaQuote.QuoteNumber, 1},
		{"customer name", "beta", 1},
		{"customer name matches both Alphas", "ALPHA", 2},
		{"literal percent is not a wildcard", "%", 1},
		{"literal underscore is not a wildcard", "_", 1},
		{"surrounding whitespace is ignored", "  po-alpha  ", 1},
		{"no match", "zzz-nothing", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{Search: tc.search, Page: 1})
			if err != nil {
				t.Fatalf("search %q: %v", tc.search, err)
			}
			if len(page.Rows) != tc.want || page.Total != tc.want {
				t.Fatalf("search %q rows = %d total = %d; want %d (%+v)",
					tc.search, len(page.Rows), page.Total, tc.want, page.Rows)
			}
			if page.TotalPages != 1 {
				t.Errorf("search %q total pages = %d; want 1", tc.search, page.TotalPages)
			}
		})
	}

	// The search finds the selected PO by its customer number, not only its
	// snapshot name.
	found, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{Search: "PO-ALPHA-1", Page: 1})
	if err != nil {
		t.Fatalf("search by exact PO number: %v", err)
	}
	if len(found.Rows) != 1 || found.Rows[0].ID != alpha.ID {
		t.Errorf("exact PO search = %+v; want PO %d", found.Rows, alpha.ID)
	}
}

func TestListPurchaseOrdersFiltersByCustomersReceivedRangeAndState(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_filters")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	alphaQuote := seedPOListQuotation(t, database, 2, "PT. Alpha", "First")
	first := setPOListNumber(t, database, ctx,
		createPOForList(t, database, ctx, alphaQuote, user, "2026-02-10"), user, "PO-A")

	betaQuote := seedPOListQuotation(t, database, 3, "PT. Beta", "Second")
	second := setPOListNumber(t, database, ctx,
		createPOForList(t, database, ctx, betaQuote, user, "2026-02-15"), user, "PO-B")
	setPOListState(t, database, ctx, second.ID, PurchaseOrderStateSaved)

	thirdQuote := seedPOListQuotation(t, database, 2, "PT. Alpha", "Third")
	third := setPOListNumber(t, database, ctx,
		createPOForList(t, database, ctx, thirdQuote, user, "2026-03-01"), user, "PO-C")

	from := func(value string) *time.Time {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		return &parsed
	}

	cases := []struct {
		name  string
		query PurchaseOrderListQuery
		want  []int64
	}{
		{
			name:  "one customer",
			query: PurchaseOrderListQuery{CustomerIDs: []int64{first.CustomerID}},
			want:  []int64{third.ID, first.ID},
		},
		{
			name:  "two customers",
			query: PurchaseOrderListQuery{CustomerIDs: []int64{first.CustomerID, second.CustomerID}},
			want:  []int64{third.ID, second.ID, first.ID},
		},
		{
			name:  "received from is inclusive",
			query: PurchaseOrderListQuery{ReceivedFrom: from("2026-02-12")},
			want:  []int64{third.ID, second.ID},
		},
		{
			name:  "received to is inclusive",
			query: PurchaseOrderListQuery{ReceivedTo: from("2026-02-15")},
			want:  []int64{second.ID, first.ID},
		},
		{
			name: "received range includes both boundaries",
			query: PurchaseOrderListQuery{
				ReceivedFrom: from("2026-02-10"),
				ReceivedTo:   from("2026-02-15"),
			},
			want: []int64{second.ID, first.ID},
		},
		{
			name:  "saved state",
			query: PurchaseOrderListQuery{State: PurchaseOrderStateSaved},
			want:  []int64{second.ID},
		},
		{
			name: "draft state plus customer",
			query: PurchaseOrderListQuery{
				State:       PurchaseOrderStateDraft,
				CustomerIDs: []int64{first.CustomerID},
			},
			want: []int64{third.ID, first.ID},
		},
		{
			name: "search and state combine",
			query: PurchaseOrderListQuery{
				Search:       "PO-",
				State:        PurchaseOrderStateDraft,
				ReceivedFrom: from("2026-02-20"),
			},
			want: []int64{third.ID},
		},
		{
			name:  "no match keeps one empty page",
			query: PurchaseOrderListQuery{State: PurchaseOrderStateSaved, CustomerIDs: []int64{first.CustomerID}},
			want:  []int64{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.query.Page = 1
			page, err := database.ListPurchaseOrders(ctx, tc.query)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			got := make([]int64, 0, len(page.Rows))
			for _, row := range page.Rows {
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
			if page.TotalPages != 1 {
				t.Errorf("total pages = %d; want 1", page.TotalPages)
			}
			if page.Rows == nil {
				t.Error("rows are nil; an empty page must still be an empty list")
			}
		})
	}

	// Page below one is the first page, not an empty result.
	page, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{Page: 0})
	if err != nil {
		t.Fatalf("page 0: %v", err)
	}
	if page.Page != 1 || len(page.Rows) != 3 {
		t.Errorf("page 0 = page %d rows %d; want 1/3", page.Page, len(page.Rows))
	}
}

func TestListPurchaseOrdersIsOrganizationScoped(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_org")
	user := poTestUser(t, database)
	ctxA := poContext(t, database)

	quotation := seedPOListQuotation(t, database, 2, "PT. Alpha", "Scoped")
	order := createPOForList(t, database, ctxA, quotation, user, "2026-02-10")

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var orgB string
	if err := pool.QueryRow(ctxA, fmt.Sprintf(
		`INSERT INTO %s.organizations (name) VALUES ('Other Org') RETURNING id::text`,
		database.configSchema())).Scan(&orgB); err != nil {
		t.Fatalf("insert second org: %v", err)
	}
	ctxB := ContextWithOrg(context.Background(), orgB)

	page, err := database.ListPurchaseOrders(ctxB, PurchaseOrderListQuery{Page: 1})
	if err != nil {
		t.Fatalf("list org B: %v", err)
	}
	if len(page.Rows) != 0 || page.Total != 0 {
		t.Errorf("org B sees org A's PO: rows %+v total %d", page.Rows, page.Total)
	}
	if len(page.Customers) != 0 {
		t.Errorf("org B customer facet = %+v; want none", page.Customers)
	}
	searched, err := database.ListPurchaseOrders(ctxB, PurchaseOrderListQuery{Search: quotation.QuoteNumber, Page: 1})
	if err != nil {
		t.Fatalf("search org B: %v", err)
	}
	if len(searched.Rows) != 0 {
		t.Errorf("org B searched org A's quotation into view: %+v", searched.Rows)
	}

	// The owner still sees it.
	owner, err := database.ListPurchaseOrders(ctxA, PurchaseOrderListQuery{Page: 1})
	if err != nil {
		t.Fatalf("list org A: %v", err)
	}
	if len(owner.Rows) != 1 || owner.Rows[0].ID != order.ID {
		t.Errorf("org A rows = %+v; want PO %d", owner.Rows, order.ID)
	}

	// No organization context is refused rather than silently reading every
	// row.
	if _, err := database.ListPurchaseOrders(context.Background(), PurchaseOrderListQuery{Page: 1}); err == nil {
		t.Error("listing without an organization succeeded; want refusal")
	}
}

func TestListPurchaseOrdersSourceMissingPredicateOnLiveRows(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_source_missing")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	quotation := seedPOListQuotation(t, database, 2, "PT. Alpha", "Live source")
	order := createPOForList(t, database, ctx, quotation, user, "2026-02-10")

	// A live link is never source missing: "only" answers zero for it. The
	// reachable state after #480 deletion is covered by the repair suite.
	only, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{SourceMissing: PurchaseOrderSourceMissingOnly, Page: 1})
	if err != nil {
		t.Fatalf("sourceMissing=only: %v", err)
	}
	if len(only.Rows) != 0 || only.Total != 0 {
		t.Errorf("sourceMissing=only returned %+v; a live link is not missing", only.Rows)
	}
	excluded, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{SourceMissing: PurchaseOrderSourceMissingExclude, Page: 1})
	if err != nil {
		t.Fatalf("sourceMissing=exclude: %v", err)
	}
	if len(excluded.Rows) != 1 || excluded.Rows[0].ID != order.ID {
		t.Errorf("sourceMissing=exclude rows = %+v; want the live PO", excluded.Rows)
	}
	for _, mode := range []string{"", PurchaseOrderSourceMissingAny} {
		all, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{SourceMissing: mode, Page: 1})
		if err != nil {
			t.Fatalf("sourceMissing=%q: %v", mode, err)
		}
		if len(all.Rows) != 1 || all.Rows[0].SourceMissing {
			t.Errorf("sourceMissing=%q = %+v; want the live PO with sourceMissing false", mode, all.Rows)
		}
	}

	if _, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{SourceMissing: "maybe", Page: 1}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("unknown source-missing filter = %v; want ErrPurchaseOrderInvalid", err)
	}
}

func TestListPurchaseOrdersRejectsInvalidFilters(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_invalid")
	ctx := poContext(t, database)

	parsed, err := time.Parse("2006-01-02", "2026-02-10")
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	later, err := time.Parse("2006-01-02", "2026-02-11")
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}

	cases := []struct {
		name  string
		query PurchaseOrderListQuery
	}{
		{"unknown state", PurchaseOrderListQuery{State: "fulfilled", Page: 1}},
		{"received from after to", PurchaseOrderListQuery{ReceivedFrom: &later, ReceivedTo: &parsed, Page: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := database.ListPurchaseOrders(ctx, tc.query); !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Errorf("query %+v error = %v; want ErrPurchaseOrderInvalid", tc.query, err)
			}
		})
	}
}

func TestListPurchaseOrdersRejectsAPageWhoseOffsetOverflows(t *testing.T) {
	database := openTestSchema(t, "vera_po_list_page_overflow")
	ctx := poContext(t, database)

	// 614891469123651722 = floor(MaxInt64/15)+2, so (page-1)*15 wraps
	// negative and Postgres rejects the OFFSET. That must be invalid input,
	// never a database error the caller sees as a server failure.
	if _, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		Page: 614891469123651722,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("overflowing page error = %v; want ErrPurchaseOrderInvalid", err)
	}

	// One less is representable: an empty page, not a refusal. This pins that
	// the guard rejects only the unrepresentable offset and never clamps.
	boundary, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		Page: 614891469123651721,
	})
	if err != nil {
		t.Fatalf("largest representable page: %v", err)
	}
	if boundary.Page != 614891469123651721 || len(boundary.Rows) != 0 {
		t.Errorf("boundary page = %+v; want an empty page it echoes", boundary)
	}
}

func TestEscapeLikeStillCoversTheListSearch(t *testing.T) {
	// The archive reuses the selector's escape helper. This pins that the
	// helper is still the one both queries call, so a future change cannot
	// silently widen the archive search.
	if got := escapeLike(`50%_done\now`); got != `50\%\_done\\now` {
		t.Errorf("escapeLike = %q; want every wildcard neutralised", got)
	}
	if strings.Contains(escapeLike("abc"), "%") {
		t.Error("escapeLike added a wildcard to a plain term")
	}
}
