package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

// Issue #658: the production backfill is #659's human run, never this ticket's
// write. What #658 must prove is that the real migration chain, applied on top
// of the three production purchase-order shapes, reaches the figures the
// operator saw after the audited corrections -- using a disposable schema that
// is dropped with the test.
//
// The rehearsal replays the exact order: build a schema through migration 053,
// seed the pre-allocation shapes, then let the store's own Migrate apply
// 054-056 before any correction runs. Seeding before those migrations is the
// point: it proves the defaulted columns read the legacy rows as quotation
// lines, not as extra expenses, and that the audited correction path can still
// backfill them afterwards.

// seedPreAllocationPurchaseOrder writes the production shape directly: a saved
// record whose only line names no quotation leaf, so no allocation exists. It
// runs at migration 053, where the post-allocation columns do not exist yet, so
// raw SQL is the only writer that can produce a genuine legacy row. The shape
// is the shared #650 fixture.
func seedPreAllocationPurchaseOrder(
	t *testing.T,
	database *Store,
	ctx context.Context,
	user AuthUser,
	orgID string,
	quotation QuotationRecord,
	shape productionAllocationFixture,
) int64 {
	t.Helper()
	schema := database.configSchema()

	var customerID int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT id FROM %s.customers WHERE customer_code = $1`, schema),
		quotation.CustomerCode).Scan(&customerID); err != nil {
		t.Fatalf("%s: read the customer: %v", shape.label, err)
	}
	received, err := time.Parse(time.DateOnly, shape.received)
	if err != nil {
		t.Fatalf("%s: received date %q: %v", shape.label, shape.received, err)
	}
	subtotal := shape.quantity * shape.sell

	var orderID int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_id, customer_name,
			received_date, currency, tax_zone, po_number, po_number_key,
			saved_at, saved_by, subtotal_minor, grand_total_minor,
			created_by, updated_by, business_category
		) VALUES ($1,$2,$3,'saved',$4,$5,$6,'IDR','FTZ',$7,$8,
			NOW(),$9,$10,$10,$9,$9,'FM')
		RETURNING id`, schema),
		orgID, quotation.ID, quotation.RevisionNumber, customerID,
		shape.customerName, received, shape.number,
		PurchaseOrderNumberKey(shape.number), user.ID, subtotal,
	).Scan(&orderID); err != nil {
		t.Fatalf("%s: insert the pre-allocation record: %v", shape.label, err)
	}

	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_order_items (
			purchase_order_id, line_number, description,
			quotation_item_no, quotation_sub_no, quantity_scaled, uom,
			unit_price_minor, gross_minor, discount_minor, net_minor
		) VALUES ($1,1,$2,NULL,NULL,$3,'Pc',$4,$5,0,$5)`, schema),
		orderID, shape.label, shape.quantity*QuantityScale, shape.sell, subtotal,
	); err != nil {
		t.Fatalf("%s: insert the pre-allocation line: %v", shape.label, err)
	}
	return orderID
}

func readRolloutLineKinds(t *testing.T, database *Store, orderID int64) []string {
	t.Helper()
	rows, err := database.pool.Query(t.Context(), fmt.Sprintf(`
		SELECT line_kind FROM %s.purchase_order_items
		WHERE purchase_order_id = $1 ORDER BY line_number`, database.configSchema()), orderID)
	if err != nil {
		t.Fatalf("read line kinds of purchase order %d: %v", orderID, err)
	}
	defer rows.Close()
	kinds := make([]string, 0, 1)
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("scan line kind: %v", err)
		}
		kinds = append(kinds, kind)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate line kinds: %v", err)
	}
	return kinds
}

func TestAllocationRolloutRehearsalOnADisposableSchema(t *testing.T) {
	database := po518Unmigrated(t, "vera_rollout_658_rehearsal")
	po518ApplyThrough(t, database, 53)
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	org, err := database.DefaultOrganization(context.Background())
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}

	// The disposable schema stops at 053: no item groups, no line kind, no
	// closure. Pin that so the rehearsal cannot silently seed a later shape.
	for _, probe := range []struct {
		table  string
		column string
	}{
		{"purchase_order_allocations", "line_kind"},
		{"purchase_order_item_groups", "id"},
		{"purchase_order_items", "line_kind"},
		{"quotations", "closed_at"},
	} {
		var exists bool
		if err := database.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = $1 AND table_name = $2 AND column_name = $3)`,
			database.config.Schema, probe.table, probe.column).Scan(&exists); err != nil {
			t.Fatalf("probe %s.%s: %v", probe.table, probe.column, err)
		}
		if exists {
			t.Fatalf("%s.%s exists before migration 054-056; the rehearsal is not pre-cutover",
				probe.table, probe.column)
		}
	}

	shapes := productionAllocationFixtures()
	orderIDs := make([]int64, 0, len(shapes))
	for _, shape := range shapes {
		quotation := seedReportQuotation(t, database, shape.customerCode,
			shape.customerName, shape.label, "IDR", "FTZ",
			[]reportQuotationLine{{
				Description: shape.label,
				UoM:         "Pc",
				Qty:         shape.quoteQty,
				Cost:        shape.cost,
				Sell:        shape.sell,
			}})
		orderIDs = append(orderIDs, seedPreAllocationPurchaseOrder(
			t, database, ctx, user, org.ID, quotation, shape))
	}

	// This is the production deployment step: the store applies only the
	// migrations the ledger says are pending.
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("apply migrations 054-056 over the production shapes: %v", err)
	}
	for _, orderID := range orderIDs {
		kinds := readRolloutLineKinds(t, database, orderID)
		if len(kinds) != 1 || kinds[0] != PurchaseOrderLineKindQuotation {
			t.Fatalf("purchase order %d line kinds after 055 = %v; want the defaulted quotation",
				orderID, kinds)
		}
	}

	// Before the audited correction, the three records are exactly the
	// production hole: full revenue, no resolved cost, no margin.
	before := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if before.Totals.PoCount != 3 {
		t.Fatalf("pre-correction poCount = %d; want 3", before.Totals.PoCount)
	}
	if before.Totals.AmountMinor != 151_230_000 {
		t.Errorf("pre-correction amount = %d; want 151.230.000", before.Totals.AmountMinor)
	}
	if before.Totals.CostedMinor != 0 || before.Totals.UncostedMinor != 151_230_000 {
		t.Errorf("pre-correction costed/uncosted = %d/%d; want 0/151.230.000",
			before.Totals.CostedMinor, before.Totals.UncostedMinor)
	}

	// #659's human-approved correction, replayed as the store's audited path.
	for index, orderID := range orderIDs {
		current, err := database.GetPurchaseOrder(ctx, orderID)
		if err != nil {
			t.Fatalf("%s: read the legacy record: %v", shapes[index].label, err)
		}
		correction := correctionFromSavedOrder(current, user,
			"Backfill allocation for line 1 to Quotation item 1")
		itemNo := 1
		correction.Items[0].QuotationItemNo = &itemNo
		correction.Items[0].QuotationSubNo = nil
		if _, err := database.CorrectPurchaseOrder(ctx, correction); err != nil {
			t.Fatalf("%s: backfill the allocation: %v", shapes[index].label, err)
		}
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 3 {
		t.Errorf("poCount = %d; want 3", report.Totals.PoCount)
	}
	if report.Totals.AmountMinor != 151_230_000 {
		t.Errorf("amount = %d; want 151.230.000", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 151_230_000 {
		t.Errorf("costed = %d; want 151.230.000 (100%% coverage)", report.Totals.CostedMinor)
	}
	if report.Totals.UncostedMinor != 0 {
		t.Errorf("uncosted = %d; want 0", report.Totals.UncostedMinor)
	}
	if report.Totals.CapitalMinor != 83_400_000 {
		t.Errorf("capital = %d; want 83.400.000", report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 67_830_000 {
		t.Errorf("margin = %d; want 67.830.000", report.Totals.MarginMinor)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 10000 {
		t.Errorf("coverage = %v; want 10000 bps", report.Totals.CoverageBps)
	}
	if report.Totals.MarginBps == nil || *report.Totals.MarginBps != 8133 {
		t.Errorf("margin bps = %v; want 8133", report.Totals.MarginBps)
	}

	// Quotation 70 still has six units unbought, valued at its row baseline;
	// 61 and 45 are fully bought. Nothing was closed, and nothing was bought at
	// a price that differs from the quotation.
	if report.Pipeline.UnsoldAmountMinor != 105_600_000 ||
		report.Pipeline.UnsoldMarginMinor != 39_600_000 {
		t.Errorf("unsold = %d/%d; want 105.600.000/39.600.000",
			report.Pipeline.UnsoldAmountMinor, report.Pipeline.UnsoldMarginMinor)
	}
	if report.Variance.PriceExtraIncomeMinor != 0 || report.Variance.PriceLossMinor != 0 {
		t.Errorf("price variance = +%d/-%d; want 0",
			report.Variance.PriceExtraIncomeMinor, report.Variance.PriceLossMinor)
	}
	if report.Variance.ClosedLossAmountMinor != 0 || report.Variance.ClosedLossMarginMinor != 0 {
		t.Errorf("closed loss = %d/%d; want 0",
			report.Variance.ClosedLossAmountMinor, report.Variance.ClosedLossMarginMinor)
	}
	if report.Variance.QuantityExtraIncomeAmountMinor != 0 {
		t.Errorf("quantity extra income = %d; want 0", report.Variance.QuantityExtraIncomeAmountMinor)
	}

	// The drill-down is the per-quotation view: 6 Pc unsold on the quote, and
	// the other two quotations fully bought.
	page, err := database.ReportPurchaseOrderFulfillment(ctx, poreport.FulfillmentQuery{
		Currency: "IDR", Status: string(poreport.FulfillmentStatusAll),
	}, time.Now())
	if err != nil {
		t.Fatalf("read the fulfillment page: %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("fulfillment total = %d; want 3 quotations", page.Total)
	}
	byQuoteNumber := make(map[string]poreport.FulfillmentQuotation, len(page.Quotations))
	for _, quotation := range page.Quotations {
		byQuoteNumber[quotation.QuoteNumber] = quotation
	}
	var unsoldQuote *poreport.FulfillmentQuotation
	var allBought int
	for _, quotation := range page.Quotations {
		switch quotation.CustomerName {
		case "PT. Production 70":
			copy := quotation
			unsoldQuote = &copy
		case "PT. Production 61", "PT. Production 45":
			if quotation.Status != string(poreport.FulfillmentStatusAllBought) {
				t.Errorf("%s status = %q; want all_bought", quotation.CustomerName, quotation.Status)
			}
			allBought++
		}
	}
	if len(byQuoteNumber) != 3 {
		t.Errorf("fulfillment rows = %d distinct quotations; want 3", len(byQuoteNumber))
	}
	if allBought != 2 {
		t.Errorf("all-bought quotations = %d; want quotations 61 and 45", allBought)
	}
	if unsoldQuote == nil {
		t.Fatal("fulfillment page has no row for quotation 70")
	}
	if unsoldQuote.Status != string(poreport.FulfillmentStatusOpen) {
		t.Errorf("quotation 70 status = %q; want open", unsoldQuote.Status)
	}
	if len(unsoldQuote.Items) != 1 {
		t.Fatalf("quotation 70 items = %d; want one", len(unsoldQuote.Items))
	}
	item := unsoldQuote.Items[0]
	if item.UnsoldQuantityScaled != 6*QuantityScale {
		t.Errorf("quotation 70 unsold quantity = %d; want 6 Pc", item.UnsoldQuantityScaled)
	}
	if item.UnsoldAmountMinor != 105_600_000 || item.UnsoldMarginMinor != 39_600_000 {
		t.Errorf("quotation 70 unsold = %d/%d; want 105.600.000/39.600.000",
			item.UnsoldAmountMinor, item.UnsoldMarginMinor)
	}
	if item.BoughtQuantityScaled != 4*QuantityScale {
		t.Errorf("quotation 70 bought quantity = %d; want 4 Pc", item.BoughtQuantityScaled)
	}
}
