package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// Issue #657: the PO Fulfillment drill-down. These tests drive the store seam
// against disposable Postgres: one quotation in each derived status, aggregate
// decomposition, extra expense lines, pagination and filters, the currency
// partition, a leaf removed by a later revision, and the exclusions the store
// document states.

// fulfillmentNow is the report's fixed as-of instant: noon WIB on 2026-11-19.
// Status depends on the company calendar day, so the test pins the clock instead
// of depending on the day it runs.
func fulfillmentNow() time.Time {
	return reportNow()
}

func fulfillmentQuery(currency, status, search string, page, pageSize int) poreport.FulfillmentQuery {
	return poreport.FulfillmentQuery{
		Currency: currency,
		Status:   status,
		Search:   search,
		Page:     page,
		PageSize: pageSize,
	}
}

func mustFulfillment(
	t *testing.T, database *Store, query poreport.FulfillmentQuery,
) poreport.FulfillmentPage {
	t.Helper()
	page, err := database.ReportPurchaseOrderFulfillment(poContext(t, database), query, fulfillmentNow())
	if err != nil {
		t.Fatalf("ReportPurchaseOrderFulfillment(%+v): %v", query, err)
	}
	return page
}

func fulfillmentQuotation(
	t *testing.T, page poreport.FulfillmentPage, quoteNumber string,
) poreport.FulfillmentQuotation {
	t.Helper()
	for _, quotation := range page.Quotations {
		if quotation.QuoteNumber == quoteNumber {
			return quotation
		}
	}
	t.Fatalf("fulfillment page has no quotation %s: %+v", quoteNumber, page.Quotations)
	return poreport.FulfillmentQuotation{}
}

// TestPurchaseOrderFulfillmentDecomposesAnAggregateGroup pins #654's read: a
// grouped depth-1 line and a depth-3 split child each appear as their own row
// carrying the group title, the header is never a third row, and every child
// keeps its own realized margin.
func TestPurchaseOrderFulfillmentDecomposesAnAggregateGroup(t *testing.T) {
	database := openTestSchema(t, "vera_po_657_group")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedPOQuotation(t, database, "PT. Aggregate Fulfillment", "Aggregate fulfillment")
	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-657-GROUP", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, nil)
	if len(saved.Items) != 3 {
		t.Fatalf("saved lines = %d; want the plain line and two package children", len(saved.Items))
	}

	grouped := correctionInput(saved, user, "one customer line over the mixed depths", saved.Version)
	grouped.Items = indexedItems(saved.Items, 1, 1, 1)
	grouped.AggregateGroups = []PurchaseOrderAggregateGroupInput{
		{Title: "Combined items", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"},
	}
	if _, err := database.CorrectPurchaseOrder(ctx, grouped); err != nil {
		t.Fatalf("group the saved record: %v", err)
	}

	page := mustFulfillment(t, database, fulfillmentQuery("USD", "all", "", 1, 100))
	fulfilled := fulfillmentQuotation(t, page, quotation.QuoteNumber)
	if len(fulfilled.Items) != 3 {
		t.Fatalf("fulfillment items = %d; want the three priced children only:\n%+v",
			len(fulfilled.Items), fulfilled.Items)
	}
	// The quotation's source costs, in the same order as the saved lines: the
	// plain line, then the package's two children.
	costs := []int64{500, 300, 200}
	for index, item := range fulfilled.Items {
		if item.GroupTitle != "Combined items" {
			t.Errorf("item %s group title = %q; want Combined items",
				item.Label, item.GroupTitle)
		}
		wantCapital, err := poMulDiv(saved.Items[index].QuantityScaled, costs[index], QuantityScale, true)
		if err != nil {
			t.Fatalf("expected capital of %s: %v", item.Label, err)
		}
		wantMargin := saved.Items[index].NetMinor - wantCapital
		if item.RealizedMarginMinor != wantMargin {
			t.Errorf("item %s realized margin = %d; want %d (revenue %d - capital %d)",
				item.Label, item.RealizedMarginMinor, wantMargin,
				saved.Items[index].NetMinor, wantCapital)
		}
		if item.Removed {
			t.Errorf("item %s reads removed; the latest revision still carries it", item.Label)
		}
	}
}

// TestPurchaseOrderFulfillmentListsExtrasSeparately pins #655 at the
// fulfillment seam: an extra expense line has its own entry and amount, never a
// quoted/bought/unsold quantity, and it carries the aggregate title when it was
// grouped with a quotation child.
func TestPurchaseOrderFulfillmentListsExtrasSeparately(t *testing.T) {
	database := openTestSchema(t, "vera_po_657_extras")
	ctx := poContext(t, database)
	user := poTestUser(t, database)

	standalone := seedReportQuotation(t, database, 2, "PT. Extra Standalone", "Standalone extra",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
		}})
	savedStandalone := finalizeReportPO(t, database, ctx, user, standalone, "PO-657-EXTRA", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})

	grouped := seedReportQuotation(t, database, 3, "PT. Extra Grouped", "Grouped extra",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000,
		}})
	savedGrouped := finalizeReportPO(t, database, ctx, user, grouped, "PO-657-EXTRA-GROUP", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, extraLine())
			input.AcknowledgedExtra = []int{2}
		})
	groupedInput := correctionInput(savedGrouped, user, "group the customer line with its freight",
		savedGrouped.Version)
	groupedInput.Items = indexedItems(savedGrouped.Items, 1, 1)
	groupedInput.AggregateGroups = []PurchaseOrderAggregateGroupInput{
		{Title: "Combined line and freight", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"},
	}
	if _, err := database.CorrectPurchaseOrder(ctx, groupedInput); err != nil {
		t.Fatalf("group the quoted line with the extra: %v", err)
	}

	page := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 100))

	simple := fulfillmentQuotation(t, page, standalone.QuoteNumber)
	if len(simple.Items) != 1 {
		t.Fatalf("standalone items = %d; want only the quoted line", len(simple.Items))
	}
	if got := fulfillmentItem(t, simple, 1, nil); got.QuotedQuantityScaled != 2*QuantityScale ||
		got.BoughtQuantityScaled != 2*QuantityScale {
		t.Errorf("standalone quoted line = %+v; want 2/2 scaled and no extra quantity", got)
	}
	if len(simple.ExtraExpenses) != 1 {
		t.Fatalf("standalone extras = %d; want the one freight line", len(simple.ExtraExpenses))
	}
	extra := simple.ExtraExpenses[0]
	if extra.AmountMinor != 1_000_000 || extra.QuantityScaled != 2*QuantityScale ||
		extra.Description != "Freight surcharge" || extra.UoM != "Lot" ||
		extra.PONumber != "PO-657-EXTRA" || extra.LineNumber != 2 ||
		extra.PurchaseOrderID != savedStandalone.ID {
		t.Errorf("standalone extra = %+v; want 1.000.000 at line 2 of the saved PO", extra)
	}
	if extra.GroupTitle != "" {
		t.Errorf("standalone extra group title = %q; want none", extra.GroupTitle)
	}

	combined := fulfillmentQuotation(t, page, grouped.QuoteNumber)
	if len(combined.Items) != 1 {
		t.Fatalf("grouped quotation items = %d; want only the quoted child", len(combined.Items))
	}
	child := fulfillmentItem(t, combined, 1, nil)
	if child.GroupTitle != "Combined line and freight" {
		t.Errorf("grouped quoted child title = %q; want the aggregate title", child.GroupTitle)
	}
	if child.QuotedQuantityScaled != 2*QuantityScale || child.BoughtQuantityScaled != 2*QuantityScale {
		t.Errorf("grouped quoted child = %+v; the extra must not change its quantities", child)
	}
	if len(combined.ExtraExpenses) != 1 {
		t.Fatalf("grouped extras = %d; want the extra listed separately", len(combined.ExtraExpenses))
	}
	if combined.ExtraExpenses[0].GroupTitle != "Combined line and freight" {
		t.Errorf("grouped extra title = %q; want the aggregate title",
			combined.ExtraExpenses[0].GroupTitle)
	}
	if combined.ExtraExpenses[0].AmountMinor != 1_000_000 {
		t.Errorf("grouped extra amount = %d; want 1.000.000",
			combined.ExtraExpenses[0].AmountMinor)
	}
}

// TestPurchaseOrderFulfillmentPaginatesAndFiltersBeforePaging pins the paging
// contract: 26 quotations page as 25/1, a status filter lowers the total before
// the page is cut, search matches the quote number and the customer snapshot,
// and an unknown status is a typed invalid query.
func TestPurchaseOrderFulfillmentPaginatesAndFiltersBeforePaging(t *testing.T) {
	database := openTestSchema(t, "vera_po_657_paging")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)

	records := make([]QuotationRecord, 0, 26)
	for index := 0; index < 26; index++ {
		records = append(records, seedReportQuotation(t, database, 2+index,
			fmt.Sprintf("PT. Filter %02d", index), fmt.Sprintf("Filter source %02d", index),
			"IDR", "FTZ", []reportQuotationLine{{
				Description: "Filtered line", UoM: "Pc", Qty: 1, Cost: 6_000, Sell: 10_000,
			}}))
	}

	first := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 0))
	if first.Total != 26 || first.TotalPages != 2 || first.PageSize != 25 ||
		first.Page != 1 || len(first.Quotations) != 25 {
		t.Fatalf("first page = total %d / pages %d / size %d / page %d / rows %d; want 26/2/25/1/25",
			first.Total, first.TotalPages, first.PageSize, first.Page, len(first.Quotations))
	}
	second := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 2, 0))
	if second.Total != 26 || len(second.Quotations) != 1 {
		t.Fatalf("second page = total %d / rows %d; want 26/1",
			second.Total, len(second.Quotations))
	}
	seen := make(map[string]bool, 26)
	for _, quotation := range append(append([]poreport.FulfillmentQuotation{}, first.Quotations...),
		second.Quotations...) {
		if seen[quotation.QuoteNumber] {
			t.Errorf("quotation %s appears on both pages", quotation.QuoteNumber)
		}
		seen[quotation.QuoteNumber] = true
	}
	if len(seen) != 26 {
		t.Fatalf("distinct paged quotations = %d; want 26", len(seen))
	}

	// A filter is applied before the page is cut: closing one quotation makes
	// the open total 25 and the closed total 1, not a page of 25 then a filter.
	closed := records[7]
	if _, err := database.CloseQuotation(ctx, closed.ID, "filter proof", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	openPage := mustFulfillment(t, database, fulfillmentQuery("IDR", "open", "", 1, 0))
	if openPage.Total != 25 || len(openPage.Quotations) != 25 {
		t.Errorf("open filter = total %d / rows %d; want 25/25",
			openPage.Total, len(openPage.Quotations))
	}
	closedPage := mustFulfillment(t, database, fulfillmentQuery("IDR", "closed", "", 1, 0))
	if closedPage.Total != 1 || len(closedPage.Quotations) != 1 ||
		closedPage.Quotations[0].QuoteNumber != closed.QuoteNumber {
		t.Fatalf("closed filter = total %d / %+v; want the one closed quotation",
			closedPage.Total, closedPage.Quotations)
	}

	// Search runs over the same filtered population.
	byNumber := mustFulfillment(t, database,
		fulfillmentQuery("IDR", "all", records[17].QuoteNumber, 1, 0))
	if byNumber.Total != 1 || byNumber.Quotations[0].QuoteNumber != records[17].QuoteNumber {
		t.Errorf("search by quote number = total %d / %+v; want that one quotation",
			byNumber.Total, byNumber.Quotations)
	}
	byCustomer := mustFulfillment(t, database,
		fulfillmentQuery("IDR", "all", "PT. Filter 17", 1, 0))
	if byCustomer.Total != 1 || byCustomer.Quotations[0].QuoteNumber != records[17].QuoteNumber {
		t.Errorf("search by customer = total %d / %+v; want that one quotation",
			byCustomer.Total, byCustomer.Quotations)
	}

	// A page past the end keeps the true total and returns no rows.
	pastTheEnd := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 3, 25))
	if pastTheEnd.Total != 26 || len(pastTheEnd.Quotations) != 0 {
		t.Errorf("page past the end = total %d / rows %d; want 26/0",
			pastTheEnd.Total, len(pastTheEnd.Quotations))
	}

	if _, err := database.ReportPurchaseOrderFulfillment(ctx,
		fulfillmentQuery("IDR", "bogus", "", 1, 0), fulfillmentNow()); !errors.Is(err, poreport.ErrInvalid) {
		t.Errorf("unknown status error = %v; want poreport.ErrInvalid", err)
	}
	if _, err := database.ReportPurchaseOrderFulfillment(ctx,
		fulfillmentQuery("IDR", "all", "", 1, 101), fulfillmentNow()); !errors.Is(err, poreport.ErrInvalid) {
		t.Errorf("page size 101 error = %v; want poreport.ErrInvalid", err)
	}
}

// TestPurchaseOrderFulfillmentPartitionsByCurrency proves the page is a
// currency partition, not a converted total: an IDR page never lists a USD
// quotation or its leaves, even when both were issued from the same customer.
func TestPurchaseOrderFulfillmentPartitionsByCurrency(t *testing.T) {
	database := openTestSchema(t, "vera_po_657_currency")
	ctx := poContext(t, database)
	user := poTestUser(t, database)

	idr := seedReportQuotation(t, database, 2, "PT. Rupiah", "Rupiah source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Rupiah line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000}})
	buyQuotedQuantity(t, database, ctx, user, idr, "PO-657-IDR", 1)
	usd := seedReportQuotation(t, database, 3, "PT. Dollar", "Dollar source", "USD", "FTZ",
		[]reportQuotationLine{{Description: "Dollar line", UoM: "Pc", Qty: 2, Cost: 6, Sell: 10}})
	buyQuotedQuantity(t, database, ctx, user, usd, "PO-657-USD", 1)

	idrPage := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 100))
	if idrPage.Total != 1 || len(idrPage.Quotations) != 1 {
		t.Fatalf("IDR page = total %d / rows %+v; want only the IDR quotation",
			idrPage.Total, idrPage.Quotations)
	}
	if idrPage.Quotations[0].QuoteNumber != idr.QuoteNumber ||
		idrPage.Quotations[0].Currency != "IDR" {
		t.Errorf("IDR page row = %+v; want the IDR quotation", idrPage.Quotations[0])
	}
	if len(idrPage.Quotations[0].Items) != 1 ||
		idrPage.Quotations[0].Items[0].RealizedAmountMinor != 10_000 {
		t.Errorf("IDR items = %+v; want the one IDR leaf at 10.000",
			idrPage.Quotations[0].Items)
	}

	usdPage := mustFulfillment(t, database, fulfillmentQuery("USD", "all", "", 1, 100))
	if usdPage.Total != 1 || usdPage.Quotations[0].QuoteNumber != usd.QuoteNumber {
		t.Fatalf("USD page = %+v; want only the USD quotation", usdPage.Quotations)
	}
}

// TestPurchaseOrderFulfillmentKeepsARemovedBoughtLeaf proves the cross-revision
// half: a leaf bought on revision 0 and removed by revision 1 stays listed with
// its frozen realized variance, reports zero quoted/unsold/over-bought, and can
// never read as a false over-buy.
func TestPurchaseOrderFulfillmentKeepsARemovedBoughtLeaf(t *testing.T) {
	database := openTestSchema(t, "vera_po_657_removed")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Removed Fulfillment", "Removed source")

	finalizeReportPO(t, database, ctx, user, quotation, "PO-657-REMOVED", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Items[0].UnitPriceMinor = 9_000
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
				varianceDecision("line:1:unitPrice"),
			}
		})
	addFreeLeafRevision(t, database, quotation)

	page := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 100))
	fulfilled := fulfillmentQuotation(t, page, quotation.QuoteNumber)
	if fulfilled.Status != "open" {
		t.Fatalf("status = %q; want the free placeholder leaf to keep the quotation open",
			fulfilled.Status)
	}
	removed := fulfillmentItem(t, fulfilled, 1, nil)
	if !removed.Removed {
		t.Fatalf("item 1 = %+v; want it marked removed", removed)
	}
	if removed.QuotedQuantityScaled != 0 || removed.UnsoldQuantityScaled != 0 ||
		removed.OverBoughtQuantityScaled != 0 || removed.UnsoldAmountMinor != 0 ||
		removed.ClosedLossAmountMinor != 0 {
		t.Errorf("removed leaf = %+v; want every quoted/unsold/over-bought figure at zero", removed)
	}
	if removed.BoughtQuantityScaled != 3*QuantityScale {
		t.Errorf("removed bought quantity = %d; want the three units bought on revision 0",
			removed.BoughtQuantityScaled)
	}
	if removed.RealizedAmountMinor != 27_000 || removed.RealizedMarginMinor != 9_000 {
		t.Errorf("removed realized = %d/%d; want 27.000/9.000",
			removed.RealizedAmountMinor, removed.RealizedMarginMinor)
	}
	if removed.PriceLossMinor != 3_000 || removed.PriceExtraIncomeMinor != 0 {
		t.Errorf("removed price variance = extra %d / loss %d; want 0/3.000",
			removed.PriceExtraIncomeMinor, removed.PriceLossMinor)
	}
	if removed.Description != "WIP rack" {
		t.Errorf("removed description = %q; want the bought line's description", removed.Description)
	}
	placeholder := fulfillmentItem(t, fulfilled, 2, nil)
	if placeholder.Removed || placeholder.QuotedQuantityScaled != QuantityScale ||
		placeholder.UnsoldQuantityScaled != QuantityScale {
		t.Errorf("placeholder item = %+v; want the new ungrouped leaf at 1 quoted / 1 unsold",
			placeholder)
	}

	// The report still owns the removed leaf's realized variance; fulfillment
	// lists it under the quotation, and the price loss must appear in both.
	report := varianceReport(t, database)
	if report.Variance.PriceLossMinor != 3_000 {
		t.Errorf("report price loss = %d; want the removed leaf's 3.000",
			report.Variance.PriceLossMinor)
	}
}

// TestPurchaseOrderFulfillmentExcludesDraftsAndUnlinkedSources pins the two
// documented exclusions: a draft buys nothing, a source-less blank draft has no
// quotation to list, and a deleted source has no quotation row (while its
// realized variance still lives in the Actual report).
func TestPurchaseOrderFulfillmentExcludesDraftsAndUnlinkedSources(t *testing.T) {
	database := openTestSchema(t, "vera_po_657_exclusions")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)

	live := seedReportQuotation(t, database, 2, "PT. Draft Only", "Draft only source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Drafted line", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000}})
	if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    live.ID,
		RevisionNumber: live.RevisionNumber,
		CreatedBy:      user.ID,
	}); err != nil {
		t.Fatalf("create the un-finalized draft: %v", err)
	}
	blank := createBlankDraft(t, database, ctx, user)

	page := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 100))
	if page.Total != 1 || len(page.Quotations) != 1 {
		t.Fatalf("page with a draft and a blank draft = total %d / %+v; want only the live quotation",
			page.Total, page.Quotations)
	}
	item := fulfillmentItem(t, page.Quotations[0], 1, nil)
	if item.BoughtQuantityScaled != 0 || item.UnsoldQuantityScaled != 5*QuantityScale ||
		len(item.PurchaseOrders) != 0 {
		t.Errorf("draft contributed to fulfillment: %+v", item)
	}
	if blank.QuotationID != 0 || page.Quotations[0].QuotationID != live.ID {
		t.Errorf("blank draft %d leaked into the page: %+v", blank.ID, page.Quotations)
	}

	deleted := seedReportQuotation(t, database, 3, "PT. Deleted Source", "Deleted source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Deleted line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000}})
	buyQuotedQuantity(t, database, ctx, user, deleted, "PO-657-DELETED", 2)

	before := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 100))
	if before.Total != 2 {
		t.Fatalf("before deletion total = %d; want the two live quotations", before.Total)
	}
	deleteQuotationRecordWithFreshPreview(t, database, admin, ctx, deleted)

	after := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 100))
	if after.Total != 1 || after.Quotations[0].QuoteNumber != live.QuoteNumber {
		t.Fatalf("after deletion = total %d / %+v; want only the surviving quotation",
			after.Total, after.Quotations)
	}
	if _, found := fulfillmentPageByNumber(after, deleted.QuoteNumber); found {
		t.Errorf("deleted quotation %s is still listed", deleted.QuoteNumber)
	}

	// The exclusion is disclosed, not lost: the Actual report still counts the
	// surviving purchase order whose source is gone.
	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketYear, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 || report.Totals.AmountMinor != 20_000 {
		t.Errorf("report after source deletion = %d POs / %d; want the survivor at 20.000",
			report.Totals.PoCount, report.Totals.AmountMinor)
	}
}

func fulfillmentPageByNumber(
	page poreport.FulfillmentPage, quoteNumber string,
) (poreport.FulfillmentQuotation, bool) {
	for _, quotation := range page.Quotations {
		if quotation.QuoteNumber == quoteNumber {
			return quotation, true
		}
	}
	return poreport.FulfillmentQuotation{}, false
}

func fulfillmentItem(
	t *testing.T, quotation poreport.FulfillmentQuotation, itemNo int, subNo *int,
) poreport.FulfillmentItem {
	t.Helper()
	wantSub := 0
	if subNo != nil {
		wantSub = *subNo
	}
	for _, item := range quotation.Items {
		haveSub := 0
		if item.QuotationSubNo != nil {
			haveSub = *item.QuotationSubNo
		}
		if item.QuotationItemNo == itemNo && haveSub == wantSub {
			return item
		}
	}
	t.Fatalf("quotation %s has no item %d/%d: %+v",
		quotation.QuoteNumber, itemNo, wantSub, quotation.Items)
	return poreport.FulfillmentItem{}
}

// ageFulfillmentRevision moves a quotation's only revision into the past, so the
// expiry boundary is exercised without waiting for a real month.
func ageFulfillmentRevision(t *testing.T, database *Store, quotationID int64, days int) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(poContext(t, database), fmt.Sprintf(`
		UPDATE %s.quotation_revisions SET created_at = $2 WHERE quotation_id = $1`,
		database.configSchema()), quotationID, time.Now().AddDate(0, 0, -days)); err != nil {
		t.Fatalf("age quotation %d revision: %v", quotationID, err)
	}
}

func buyQuotedQuantity(
	t *testing.T, database *Store, ctx context.Context, user AuthUser,
	quotation QuotationRecord, number string, quantity int64,
) PurchaseOrder {
	t.Helper()
	return finalizeReportPO(t, database, ctx, user, quotation, number, "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = quantity * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})
}

// TestPurchaseOrderFulfillmentStatusesAndItemFigures is the acceptance proof:
// one quotation in each status, with the exact quoted/bought/unsold/closed
// figures. Expiry never becomes a loss, and a fully bought quotation is
// all_bought even when its validity window has passed.
func TestPurchaseOrderFulfillmentStatusesAndItemFigures(t *testing.T) {
	database := openTestSchema(t, "vera_po_657_statuses")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)

	open := seedReportQuotation(t, database, 2, "PT. Open", "Open source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Open line", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000}})
	buyQuotedQuantity(t, database, ctx, user, open, "PO-657-OPEN", 3)

	expired := seedReportQuotation(t, database, 3, "PT. Expired", "Expired source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Expired line", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000}})
	buyQuotedQuantity(t, database, ctx, user, expired, "PO-657-EXPIRED", 3)
	ageFulfillmentRevision(t, database, expired.ID, 45)

	allBought := seedReportQuotation(t, database, 4, "PT. All bought", "All bought source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Bought line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000}})
	buyQuotedQuantity(t, database, ctx, user, allBought, "PO-657-ALL-BOUGHT", 2)
	// Expired AND fully bought: all_bought wins, because there is no open
	// remainder left to lapse.
	ageFulfillmentRevision(t, database, allBought.ID, 45)

	closed := seedReportQuotation(t, database, 5, "PT. Closed", "Closed source", "IDR", "FTZ",
		[]reportQuotationLine{{Description: "Closed line", UoM: "Pc", Qty: 5, Cost: 6_000, Sell: 10_000}})
	buyQuotedQuantity(t, database, ctx, user, closed, "PO-657-CLOSED", 3)
	if _, err := database.CloseQuotation(ctx, closed.ID, "customer cancelled the remainder", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}

	page := mustFulfillment(t, database, fulfillmentQuery("IDR", "all", "", 1, 100))
	if page.Total != 4 {
		t.Fatalf("total = %d; want the four IDR quotations", page.Total)
	}
	if page.AsOf != "2026-11-19" {
		t.Errorf("asOf = %q; want the WIB calendar day 2026-11-19", page.AsOf)
	}

	statuses := map[string]string{
		open.QuoteNumber:      "open",
		expired.QuoteNumber:   "expired_unclosed",
		allBought.QuoteNumber: "all_bought",
		closed.QuoteNumber:    "closed",
	}
	for quoteNumber, want := range statuses {
		quotation := fulfillmentQuotation(t, page, quoteNumber)
		if quotation.Status != want {
			t.Errorf("%s status = %q; want %q", quoteNumber, quotation.Status, want)
		}
	}

	// Open 5/3: two still orderable at the quotation baseline, never a loss.
	openItem := fulfillmentItem(t, fulfillmentQuotation(t, page, open.QuoteNumber), 1, nil)
	if openItem.QuotedQuantityScaled != 5*QuantityScale ||
		openItem.BoughtQuantityScaled != 3*QuantityScale ||
		openItem.UnsoldQuantityScaled != 2*QuantityScale ||
		openItem.OverBoughtQuantityScaled != 0 {
		t.Errorf("open quantities = quoted %d / bought %d / unsold %d / over %d; want 5/3/2/0 scaled",
			openItem.QuotedQuantityScaled, openItem.BoughtQuantityScaled,
			openItem.UnsoldQuantityScaled, openItem.OverBoughtQuantityScaled)
	}
	if openItem.RealizedAmountMinor != 30_000 || openItem.RealizedMarginMinor != 12_000 {
		t.Errorf("open realized = %d/%d; want 30.000/12.000",
			openItem.RealizedAmountMinor, openItem.RealizedMarginMinor)
	}
	if openItem.UnsoldAmountMinor != 20_000 || openItem.UnsoldMarginMinor != 8_000 {
		t.Errorf("open unsold = %d/%d; want 20.000/8.000",
			openItem.UnsoldAmountMinor, openItem.UnsoldMarginMinor)
	}
	if openItem.ClosedLossAmountMinor != 0 || openItem.ClosedLossMarginMinor != 0 {
		t.Errorf("an open quotation reports closed loss %d/%d",
			openItem.ClosedLossAmountMinor, openItem.ClosedLossMarginMinor)
	}
	if len(openItem.PurchaseOrders) != 1 ||
		openItem.PurchaseOrders[0].PONumber != "PO-657-OPEN" ||
		openItem.PurchaseOrders[0].AllocatedQuantityScaled != 3*QuantityScale ||
		openItem.PurchaseOrders[0].State != PurchaseOrderStateSaved {
		t.Errorf("open contributions = %+v; want the one saved PO at 3 scaled",
			openItem.PurchaseOrders)
	}

	// Expired and unclosed: the same Unsold remainder, never Closed Loss.
	expiredItem := fulfillmentItem(t, fulfillmentQuotation(t, page, expired.QuoteNumber), 1, nil)
	if expiredItem.UnsoldQuantityScaled != 2*QuantityScale ||
		expiredItem.UnsoldAmountMinor != 20_000 ||
		expiredItem.ClosedLossAmountMinor != 0 || expiredItem.ClosedLossMarginMinor != 0 {
		t.Errorf("expired item = %+v; want Unsold 2 / 20.000 and no closed loss", expiredItem)
	}

	allBoughtItem := fulfillmentItem(t, fulfillmentQuotation(t, page, allBought.QuoteNumber), 1, nil)
	if allBoughtItem.UnsoldQuantityScaled != 0 || allBoughtItem.UnsoldAmountMinor != 0 ||
		allBoughtItem.OverBoughtQuantityScaled != 0 || allBoughtItem.ClosedLossAmountMinor != 0 {
		t.Errorf("all-bought item = %+v; want every remainder at zero", allBoughtItem)
	}
	if allBoughtItem.QuotedQuantityScaled != 2*QuantityScale ||
		allBoughtItem.BoughtQuantityScaled != 2*QuantityScale {
		t.Errorf("all-bought quantities = %d/%d; want 2/2 scaled",
			allBoughtItem.QuotedQuantityScaled, allBoughtItem.BoughtQuantityScaled)
	}

	closedItem := fulfillmentItem(t, fulfillmentQuotation(t, page, closed.QuoteNumber), 1, nil)
	if closedItem.UnsoldQuantityScaled != 0 || closedItem.UnsoldAmountMinor != 0 ||
		closedItem.UnsoldMarginMinor != 0 {
		t.Errorf("closed item still reports Unsold = %+v", closedItem)
	}
	if closedItem.ClosedLossAmountMinor != 20_000 || closedItem.ClosedLossMarginMinor != 8_000 {
		t.Errorf("closed loss = %d/%d; want 20.000/8.000",
			closedItem.ClosedLossAmountMinor, closedItem.ClosedLossMarginMinor)
	}
}
