package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"vera-app/backend/poreport"
)

// Issue #650: a quotation-backed PO line owns exactly one allocation, and the
// report resolves cost from that allocation before falling back to the legacy
// quotation-reference join.

type storedAllocation struct {
	QuotationID          int64
	SourceRevision       int16
	QuotationItemNo      int
	QuotationSubNo       int
	QuantityScaled       int64
	SourceUoM            string
	OrderedUoM           string
	QuotedUnitPriceMinor int64
	RevenueMinor         int64
	CostUnitMinor        int64
}

func readAllocations(t *testing.T, database *Store, ctx context.Context, orderID int64) map[string]storedAllocation {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT quotation_id, source_revision, quotation_item_no,
		       COALESCE(quotation_sub_no, 0), quantity_scaled, source_uom,
		       ordered_uom, quoted_unit_price_minor, revenue_minor,
		       COALESCE(cost_unit_minor, -1)
		FROM %s.purchase_order_allocations
		WHERE purchase_order_id = $1
		ORDER BY quotation_item_no, quotation_sub_no NULLS FIRST`, database.configSchema()), orderID)
	if err != nil {
		t.Fatalf("read allocations: %v", err)
	}
	defer rows.Close()
	result := map[string]storedAllocation{}
	for rows.Next() {
		var row storedAllocation
		if err := rows.Scan(&row.QuotationID, &row.SourceRevision, &row.QuotationItemNo,
			&row.QuotationSubNo, &row.QuantityScaled, &row.SourceUoM,
			&row.OrderedUoM, &row.QuotedUnitPriceMinor, &row.RevenueMinor,
			&row.CostUnitMinor); err != nil {
			t.Fatalf("scan allocation: %v", err)
		}
		result[fmt.Sprintf("%d.%d", row.QuotationItemNo, row.QuotationSubNo)] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate allocations: %v", err)
	}
	return result
}

func TestCreateFromQuotationWritesOneAllocationPerLine(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_create")
	quotation := seedPOQuotation(t, database, "PT. Allocation Buyer", "Allocation source")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO draft: %v", err)
	}
	allocations := readAllocations(t, database, ctx, draft.ID)
	if len(allocations) != 3 {
		t.Fatalf("allocations = %d; want 3 (one per priced source line)", len(allocations))
	}

	tests := []struct {
		key        string
		quantity   int64
		sourceUoM  string
		orderedUoM string
		quoted     int64
		cost       int64
	}{
		{key: "1.0", quantity: 30000, sourceUoM: "Pc", orderedUoM: "Pc", quoted: 600, cost: 500},
		{key: "2.1", quantity: 20000, sourceUoM: "Unit", orderedUoM: "Unit", quoted: 400, cost: 300},
		{key: "2.2", quantity: 10000, sourceUoM: "Lot", orderedUoM: "Lot", quoted: 450, cost: 200},
	}
	for _, test := range tests {
		allocation, ok := allocations[test.key]
		if !ok {
			t.Errorf("missing allocation %s", test.key)
			continue
		}
		if allocation.QuotationID != quotation.ID || allocation.SourceRevision != quotation.RevisionNumber {
			t.Errorf("%s source = %d/%d; want %d/%d", test.key,
				allocation.QuotationID, allocation.SourceRevision,
				quotation.ID, quotation.RevisionNumber)
		}
		if allocation.QuantityScaled != test.quantity {
			t.Errorf("%s quantity = %d; want %d", test.key, allocation.QuantityScaled, test.quantity)
		}
		if allocation.SourceUoM != test.sourceUoM || allocation.OrderedUoM != test.orderedUoM {
			t.Errorf("%s UoM = %q/%q; want %q/%q", test.key,
				allocation.SourceUoM, allocation.OrderedUoM, test.sourceUoM, test.orderedUoM)
		}
		if allocation.QuotedUnitPriceMinor != test.quoted {
			t.Errorf("%s quoted baseline = %d; want %d", test.key,
				allocation.QuotedUnitPriceMinor, test.quoted)
		}
		if allocation.CostUnitMinor != test.cost {
			t.Errorf("%s cost = %d; want %d", test.key, allocation.CostUnitMinor, test.cost)
		}
		// The allocation revenue is the PO line's own net, not the quotation's
		// list total: the final save may change quantity or price.
		if allocation.RevenueMinor <= 0 {
			t.Errorf("%s revenue = %d; want a positive stored line net", test.key,
				allocation.RevenueMinor)
		}
	}

	// The database, not the writer, enforces one allocation per line and one
	// source leaf per PO. Each attempt below is a direct fixture INSERT, so
	// deleting either unique index makes this test fail: the assertions are the
	// SQLSTATE and the exact constraint, not a count of rows the writer chose
	// not to add.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if len(draft.Items) != 3 {
		t.Fatalf("draft items = %d; want 3", len(draft.Items))
	}
	firstLine := draft.Items[0]
	secondLine := draft.Items[1]
	spareLine := draft.Items[2]

	refuse := func(query, constraint string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, query, args...)
		if err == nil {
			t.Fatalf("duplicate allocation was accepted; want %s", constraint)
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("duplicate allocation error = %v; want *pgconn.PgError", err)
		}
		if pgErr.Code != "23505" {
			t.Fatalf("duplicate allocation SQLSTATE = %s; want 23505 (%v)", pgErr.Code, err)
		}
		if pgErr.ConstraintName != constraint {
			t.Fatalf("duplicate allocation constraint = %q; want %q",
				pgErr.ConstraintName, constraint)
		}
	}

	// 1. A second allocation on the same PO line. The source number is changed
	// too, so only po_allocations_item_uidx can refuse it.
	refuse(fmt.Sprintf(`
		INSERT INTO %[1]s.purchase_order_allocations (
			org_id, purchase_order_id, purchase_order_item_id,
			quotation_id, source_revision, source_revision_id,
			quotation_item_no, quotation_sub_no,
			quantity_scaled, source_uom, ordered_uom,
			quoted_unit_price_minor, revenue_minor, cost_unit_minor)
		SELECT org_id, purchase_order_id, purchase_order_item_id,
		       quotation_id, source_revision, source_revision_id,
		       99, NULL, quantity_scaled, source_uom, ordered_uom,
		       quoted_unit_price_minor, revenue_minor, cost_unit_minor
		FROM %[1]s.purchase_order_allocations
		WHERE purchase_order_item_id = $1`, database.configSchema()),
		"po_allocations_item_uidx", firstLine.ID)

	// Free the spare line so the source index, not the line index, decides the
	// next two attempts.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_allocations WHERE purchase_order_item_id = $1`,
		database.configSchema()), spareLine.ID); err != nil {
		t.Fatalf("free the spare line allocation: %v", err)
	}

	// 2. A different line naming the same (item 1, null sub) leaf. The two null
	// sub-items are one leaf because the index is NULLS NOT DISTINCT.
	refuse(fmt.Sprintf(`
		INSERT INTO %[1]s.purchase_order_allocations (
			org_id, purchase_order_id, purchase_order_item_id,
			quotation_id, source_revision, source_revision_id,
			quotation_item_no, quotation_sub_no,
			quantity_scaled, source_uom, ordered_uom,
			quoted_unit_price_minor, revenue_minor, cost_unit_minor)
		SELECT org_id, purchase_order_id, $2,
		       quotation_id, source_revision, source_revision_id,
		       quotation_item_no, quotation_sub_no,
		       quantity_scaled, source_uom, ordered_uom,
		       quoted_unit_price_minor, revenue_minor, cost_unit_minor
		FROM %[1]s.purchase_order_allocations
		WHERE purchase_order_item_id = $1`, database.configSchema()),
		"po_allocations_po_source_uidx", firstLine.ID, spareLine.ID)

	// 3. The same duplicate for a non-null sub-item.
	refuse(fmt.Sprintf(`
		INSERT INTO %[1]s.purchase_order_allocations (
			org_id, purchase_order_id, purchase_order_item_id,
			quotation_id, source_revision, source_revision_id,
			quotation_item_no, quotation_sub_no,
			quantity_scaled, source_uom, ordered_uom,
			quoted_unit_price_minor, revenue_minor, cost_unit_minor)
		SELECT org_id, purchase_order_id, $2,
		       quotation_id, source_revision, source_revision_id,
		       quotation_item_no, quotation_sub_no,
		       quantity_scaled, source_uom, ordered_uom,
		       quoted_unit_price_minor, revenue_minor, cost_unit_minor
		FROM %[1]s.purchase_order_allocations
		WHERE purchase_order_item_id = $1`, database.configSchema()),
		"po_allocations_po_source_uidx", secondLine.ID, spareLine.ID)
}

func TestFinalizeRewritesAllocationToTheFinalLine(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_finalize")
	quotation := seedPOQuotation(t, database, "PT. Finalize Buyer", "Finalize allocation")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO draft: %v", err)
	}
	input := finalizeInput(draft, user, "PO-ALLOC-1")
	input.Items[0].QuantityScaled = 20000
	input.Decisions = []PurchaseOrderDecision{
		{Key: "line:1:quantity", Choice: PurchaseOrderChoicePO},
	}
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize PO: %v", err)
	}

	allocation, ok := readAllocations(t, database, ctx, saved.ID)["1.0"]
	if !ok {
		t.Fatalf("no allocation for the finalized item")
	}
	if allocation.QuantityScaled != 20000 {
		t.Errorf("allocation quantity = %d; want the finalized 2.0000", allocation.QuantityScaled)
	}
	if allocation.RevenueMinor != saved.Items[0].NetMinor {
		t.Errorf("allocation revenue = %d; want the finalized line net %d",
			allocation.RevenueMinor, saved.Items[0].NetMinor)
	}
}

func TestReportUsesAllocationRevenueNotQuotationPrice(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_report")
	quotation := seedReportQuotation(t, database, 2, "PT. Report Buyer", "Allocation report",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Allocated line",
			UoM:         "Pc",
			Qty:         2,
			Cost:        100,
			Sell:        250,
		}})
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := finalizeReportPO(t, database, ctx, user, quotation,
		"PO-ALLOC-REPORT", "2026-11-15", PurchaseOrderBusinessCategoryOthers, nil)

	// Make the stored allocation deliberately different from the quotation's
	// sell price. The report must use the allocation revenue and cost snapshot,
	// not re-price the line from the quotation.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_allocations
		SET revenue_minor = 400, cost_unit_minor = 50
		WHERE purchase_order_id = $1`, database.configSchema()), saved.ID); err != nil {
		t.Fatalf("update allocation: %v", err)
	}

	report := mustReport(t, database, ctx,
		reportQuery("2026-11-01", "2026-12-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.PoCount != 1 {
		t.Fatalf("poCount = %d; want 1", report.Totals.PoCount)
	}
	if report.Totals.AmountMinor != 500 {
		t.Errorf("amount = %d; want the PO's 2 x 250 = 500", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 400 {
		t.Errorf("costed = %d; want the allocation revenue 400", report.Totals.CostedMinor)
	}
	if report.Totals.CapitalMinor != 100 {
		t.Errorf("capital = %d; want 2 x 50 allocation cost = 100", report.Totals.CapitalMinor)
	}
	if report.Totals.MarginMinor != 300 {
		t.Errorf("margin = %d; want 400 - 100 = 300", report.Totals.MarginMinor)
	}
	if report.Totals.UncostedMinor != 100 {
		t.Errorf("uncosted = %d; want 500 - 400 = 100", report.Totals.UncostedMinor)
	}
	if report.Totals.CoverageBps == nil || *report.Totals.CoverageBps != 8000 {
		t.Errorf("coverage = %v; want 8000 bps", report.Totals.CoverageBps)
	}
}

// correctionFromSavedOrder builds the payload a saved-record correction carries:
// every editable header and line as stored, with the current version as the
// optimistic base. Tests then change only the field they exercise.
func correctionFromSavedOrder(order PurchaseOrder, user AuthUser, reason string) PurchaseOrderCorrectionInput {
	items := make([]PurchaseOrderItem, len(order.Items))
	copy(items, order.Items)
	for index := range items {
		items[index].LineNumber = index + 1
	}
	received := order.ReceivedDate
	return PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: PurchaseOrderUpdate{
			ID:                    order.ID,
			BillingAddress:        order.BillingAddress,
			DeliveryAddress:       order.DeliveryAddress,
			ShipToCode:            order.ShipToCode,
			ContactEmail:          order.ContactEmail,
			ContactPhone:          order.ContactPhone,
			PONumber:              order.PONumber,
			Incoterms:             order.Incoterms,
			PaymentTerms:          order.PaymentTerms,
			PODate:                order.PODate,
			ReceivedDate:          &received,
			RequestedDeliveryDate: order.RequestedDeliveryDate,
			BusinessCategory:      order.BusinessCategory,
			Items:                 items,
			DiscountPercent:       order.DiscountPercent,
			AdjustmentMinor:       order.AdjustmentMinor,
			AdjustmentReason:      order.AdjustmentReason,
			UpdatedBy:             user.ID,
			ExpectedVersion:       order.Version,
		},
		Reason: reason,
	}
}

type correctionAllocationSnapshot struct {
	LineNumber           int
	QuantityScaled       int64
	RevenueMinor         int64
	QuotedUnitPriceMinor int64
	CostUnitMinor        int64
}

func readCorrectionAllocations(
	t *testing.T, database *Store, ctx context.Context, correctionID int64,
) []correctionAllocationSnapshot {
	t.Helper()
	rows, err := database.pool.Query(ctx, fmt.Sprintf(`
		SELECT line_number, quantity_scaled, revenue_minor,
		       quoted_unit_price_minor, COALESCE(cost_unit_minor, -1)
		FROM %s.purchase_order_correction_allocations
		WHERE correction_id = $1
		ORDER BY line_number`, database.configSchema()), correctionID)
	if err != nil {
		t.Fatalf("read correction allocations: %v", err)
	}
	defer rows.Close()
	result := make([]correctionAllocationSnapshot, 0, 4)
	for rows.Next() {
		var row correctionAllocationSnapshot
		if err := rows.Scan(&row.LineNumber, &row.QuantityScaled, &row.RevenueMinor,
			&row.QuotedUnitPriceMinor, &row.CostUnitMinor); err != nil {
			t.Fatalf("scan correction allocation: %v", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate correction allocations: %v", err)
	}
	return result
}

// TestCorrectionSnapshotsAndRewritesAllocationsAtomically proves the correction
// path keeps the old allocation picture beside the immutable correction while
// replacing the live allocations with the corrected lines, and that a refused
// correction rolls both back with no partial snapshot.
func TestCorrectionSnapshotsAndRewritesAllocationsAtomically(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_correction")
	quotation := seedPOQuotation(t, database, "PT. Correction Buyer", "Correction allocation")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO draft: %v", err)
	}
	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(draft, user, "PO-ALLOC-CORRECT"))
	if err != nil {
		t.Fatalf("finalize PO: %v", err)
	}
	before := readAllocations(t, database, ctx, saved.ID)
	if len(before) != 3 {
		t.Fatalf("allocations before correction = %d; want 3", len(before))
	}

	// A correction that changes only one quantity.
	correction := correctionFromSavedOrder(saved, user, "customer increased the order")
	correction.Items[0].QuantityScaled = 25000
	corrected, err := database.CorrectPurchaseOrder(ctx, correction)
	if err != nil {
		t.Fatalf("correct PO: %v", err)
	}

	after := readAllocations(t, database, ctx, corrected.ID)
	if len(after) != 3 {
		t.Fatalf("allocations after correction = %d; want 3", len(after))
	}
	rewritten, ok := after["1.0"]
	if !ok {
		t.Fatalf("the corrected line lost its allocation")
	}
	if rewritten.QuantityScaled != 25000 {
		t.Errorf("corrected allocation quantity = %d; want 25000", rewritten.QuantityScaled)
	}
	if rewritten.RevenueMinor != corrected.Items[0].NetMinor {
		t.Errorf("corrected allocation revenue = %d; want the corrected line net %d",
			rewritten.RevenueMinor, corrected.Items[0].NetMinor)
	}
	// The other lines were not part of the correction's changed value and keep
	// their allocation exactly.
	for _, key := range []string{"2.1", "2.2"} {
		if after[key] != before[key] {
			t.Errorf("untouched allocation %s changed: %+v -> %+v", key, before[key], after[key])
		}
	}

	var correctionID int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.purchase_order_corrections
		WHERE purchase_order_id = $1 ORDER BY id DESC LIMIT 1`,
		database.configSchema()), saved.ID).Scan(&correctionID); err != nil {
		t.Fatalf("read correction id: %v", err)
	}
	snapshot := readCorrectionAllocations(t, database, ctx, correctionID)
	if len(snapshot) != 3 {
		t.Fatalf("correction allocation snapshot = %d rows; want 3", len(snapshot))
	}
	oldLine := before["1.0"]
	if snapshot[0].LineNumber != 1 ||
		snapshot[0].QuantityScaled != oldLine.QuantityScaled ||
		snapshot[0].RevenueMinor != oldLine.RevenueMinor ||
		snapshot[0].QuotedUnitPriceMinor != oldLine.QuotedUnitPriceMinor ||
		snapshot[0].CostUnitMinor != oldLine.CostUnitMinor {
		t.Errorf("correction snapshot line 1 = %+v; want the old allocation %+v",
			snapshot[0], oldLine)
	}

	// A duplicate source leaf is refused by the shared allocation guard, after
	// the correction row and snapshot were written inside the transaction. The
	// refusal must roll all of it back.
	bad := correctionFromSavedOrder(corrected, user, "duplicate leaf")
	firstNo := *bad.Items[0].QuotationItemNo
	bad.Items[1].QuotationItemNo = &firstNo
	bad.Items[1].QuotationSubNo = nil
	if _, err := database.CorrectPurchaseOrder(ctx, bad); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("duplicate-leaf correction error = %v; want ErrPurchaseOrderInvalid", err)
	} else if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("duplicate-leaf refusal = %q; want both line numbers named", err.Error())
	}

	if got := countRows(t, database, "purchase_order_corrections",
		"purchase_order_id = $1", saved.ID); got != 1 {
		t.Errorf("corrections after the refused write = %d; want only the committed one", got)
	}
	if got := len(readCorrectionAllocations(t, database, ctx, correctionID)); got != 3 {
		t.Errorf("correction snapshot after the refused write = %d rows; want 3", got)
	}
	recheck := readAllocations(t, database, ctx, saved.ID)
	if len(recheck) != len(after) {
		t.Fatalf("allocations after the refused write = %d; want %d", len(recheck), len(after))
	}
	for key, want := range after {
		if got, ok := recheck[key]; !ok || got != want {
			t.Errorf("allocation %s after the refused write = %+v/%v; want %+v",
				key, got, ok, want)
		}
	}
	reloaded, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload corrected PO: %v", err)
	}
	if reloaded.Items[0].QuantityScaled != 25000 {
		t.Errorf("refused correction changed the line quantity to %d; want the committed 25000",
			reloaded.Items[0].QuantityScaled)
	}
}

// TestFinalizeAndCorrectionStoreActualRevenueAgainstTheBaseline pins #653's
// allocation contract: the negotiated revenue moves with the PO, while the
// frozen row-discounted quotation baseline does not. The badge/report can then
// compute the price variance from stored facts instead of re-pricing history.
func TestFinalizeAndCorrectionStoreActualRevenueAgainstTheBaseline(t *testing.T) {
	database := openTestSchema(t, "vera_po_alloc_revenue_baseline")
	quotation := seedPOQuotation(t, database, "PT. Baseline Buyer", "Baseline revenue")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	input := finalizeInput(draft, user, "PO-653-BASELINE")
	input.Items[0].QuantityScaled = 2 * QuantityScale
	input.Items[0].UnitPriceMinor = 550
	input.Decisions = []PurchaseOrderDecision{
		{Key: "line:1:quantity", Choice: PurchaseOrderChoicePO},
		{Key: "line:1:unitPrice", Choice: PurchaseOrderChoicePO},
	}
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	allocations := readAllocations(t, database, ctx, saved.ID)
	first := allocations["1.0"]
	if first.QuantityScaled != 2*QuantityScale || first.RevenueMinor != 1_100 ||
		first.QuotedUnitPriceMinor != 600 {
		t.Fatalf("finalized allocation = qty %d / revenue %d / baseline %d; want 20000/1100/600",
			first.QuantityScaled, first.RevenueMinor, first.QuotedUnitPriceMinor)
	}

	correction := correctionInput(saved, user, "Renegotiate one unit", saved.Version)
	correction.Items[0].QuantityScaled = QuantityScale
	correction.Items[0].UnitPriceMinor = 500
	corrected, err := database.CorrectPurchaseOrder(ctx, correction)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	allocations = readAllocations(t, database, ctx, corrected.ID)
	first = allocations["1.0"]
	if first.QuantityScaled != QuantityScale || first.RevenueMinor != 500 ||
		first.QuotedUnitPriceMinor != 600 {
		t.Errorf("corrected allocation = qty %d / revenue %d / baseline %d; want 10000/500/600",
			first.QuantityScaled, first.RevenueMinor, first.QuotedUnitPriceMinor)
	}
}
