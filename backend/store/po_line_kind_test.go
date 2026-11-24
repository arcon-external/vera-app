package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Issue #655: line_kind discriminates a quotation-backed line from an explicit
// extra expense line. The extra line is a pass-through: it has no quotation
// reference, owns no allocation, and reports as Extra Expense instead of
// uncosted revenue. Everything the quotation owns (ghosts, Unsold, Pipeline,
// variance) is untouched by it.

// readStoredLineKinds reads the live line_kind column by line number, so a test
// can prove what the writer stored rather than only what the reader returned.
func readStoredLineKinds(t *testing.T, database *Store, orderID int64) map[int]string {
	t.Helper()
	rows, err := database.pool.Query(t.Context(), fmt.Sprintf(`
		SELECT line_number, line_kind
		FROM %s.purchase_order_items
		WHERE purchase_order_id = $1
		ORDER BY line_number`, database.configSchema()), orderID)
	if err != nil {
		t.Fatalf("read stored line kinds: %v", err)
	}
	defer rows.Close()
	kinds := map[int]string{}
	for rows.Next() {
		var line int
		var kind string
		if err := rows.Scan(&line, &kind); err != nil {
			t.Fatalf("scan stored line kind: %v", err)
		}
		kinds[line] = kind
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate stored line kinds: %v", err)
	}
	return kinds
}

// TestPurchaseOrderLineKindRoundTripsAndDefaultsToQuotation proves three
// things at once: a legacy payload with no kind writes `quotation` (never the
// empty string, which would bypass the column default), the kind round-trips
// through save and read, and an extra line owns no allocation row.
func TestPurchaseOrderLineKindRoundTripsAndDefaultsToQuotation(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_kind")
	quotation := seedReportQuotation(t, database, 2, "PT. Kind Buyer", "Line kind source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250,
		}})
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:      quotation.ID,
		RevisionNumber:   quotation.RevisionNumber,
		BusinessCategory: PurchaseOrderBusinessCategoryFm,
		CreatedBy:        user.ID,
	})
	if err != nil {
		t.Fatalf("create line-kind draft: %v", err)
	}
	if len(draft.Items) != 1 || draft.Items[0].LineKind != PurchaseOrderLineKindQuotation {
		t.Fatalf("created source line kind = %q; want %q",
			itemLineKind(draft.Items), PurchaseOrderLineKindQuotation)
	}

	// An older editor sends no kind at all. The store must normalize the empty
	// string before the INSERT names the NOT NULL column: the database default
	// does not apply to an explicitly supplied empty value.
	legacy := finalizeInput(draft, user, "").PurchaseOrderUpdate
	for index := range legacy.Items {
		legacy.Items[index].LineKind = ""
	}
	draft, err = updateDraft(t, database, ctx, legacy)
	if err != nil {
		t.Fatalf("save the legacy draft payload: %v", err)
	}
	if kinds := readStoredLineKinds(t, database, draft.ID); kinds[1] != PurchaseOrderLineKindQuotation {
		t.Fatalf("legacy payload stored line kind %q; want %q",
			kinds[1], PurchaseOrderLineKindQuotation)
	}
	if draft.Items[0].LineKind != PurchaseOrderLineKindQuotation {
		t.Errorf("legacy payload read line kind %q; want %q",
			draft.Items[0].LineKind, PurchaseOrderLineKindQuotation)
	}

	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(draft, user, "PO-655-KIND"))
	if err != nil {
		t.Fatalf("finalize line-kind PO: %v", err)
	}

	correction := correctionFromSavedOrder(saved, user, "add a pass-through freight line")
	correction.Items = append(correction.Items, PurchaseOrderItem{
		Description:    "Freight surcharge",
		QuantityScaled: 2 * QuantityScale,
		UoM:            "Lot",
		UnitPriceMinor: 500_000,
		LineKind:       PurchaseOrderLineKindExtra,
	})
	corrected, err := database.CorrectPurchaseOrder(ctx, correction)
	if err != nil {
		t.Fatalf("add the extra line: %v", err)
	}
	if len(corrected.Items) != 2 {
		t.Fatalf("corrected lines = %d; want the quoted line and the extra", len(corrected.Items))
	}
	if corrected.Items[1].LineKind != PurchaseOrderLineKindExtra {
		t.Fatalf("corrected extra line kind = %q; want %q",
			corrected.Items[1].LineKind, PurchaseOrderLineKindExtra)
	}
	kinds := readStoredLineKinds(t, database, corrected.ID)
	if kinds[1] != PurchaseOrderLineKindQuotation || kinds[2] != PurchaseOrderLineKindExtra {
		t.Errorf("stored kinds = %v; want line 1 quotation and line 2 extra", kinds)
	}
	if corrected.Items[1].QuotationItemNo != nil || corrected.Items[1].QuotationSubNo != nil {
		t.Errorf("extra line kept a quotation reference: %+v", corrected.Items[1])
	}

	// The extra must not own an allocation: an allocation would make it a
	// quotation-backed line in the report and in the bought-state read.
	allocations := readAllocations(t, database, ctx, corrected.ID)
	if len(allocations) != 1 {
		t.Fatalf("allocations = %d; want only the quoted line's", len(allocations))
	}
	if _, ok := allocations["1.0"]; !ok {
		t.Errorf("the quoted line lost its allocation: %v", allocations)
	}
	if allocationCountForItem(t, database, corrected.Items[1].ID) != 0 {
		t.Errorf("the extra line owns an allocation row")
	}

	// A correction snapshots the kind beside the line, so history can still tell
	// a deliberate extra from a legacy null-reference line. The first snapshot
	// predates the extra; the second was taken with it present.
	if _, err := database.CorrectPurchaseOrder(ctx,
		correctionFromSavedOrder(corrected, user, "recheck the freight line")); err != nil {
		t.Fatalf("second correction: %v", err)
	}
	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read line-kind history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %d snapshots; want 2", len(history))
	}
	if len(history[1].Items) != 1 || history[1].Items[0].LineKind != PurchaseOrderLineKindQuotation {
		t.Errorf("pre-extra snapshot kinds = %v; want one quotation line",
			correctionItemLineKinds(history[1].Items))
	}
	if len(history[0].Items) != 2 ||
		history[0].Items[0].LineKind != PurchaseOrderLineKindQuotation ||
		history[0].Items[1].LineKind != PurchaseOrderLineKindExtra {
		t.Errorf("snapshot kinds = %v; want quotation then extra",
			correctionItemLineKinds(history[0].Items))
	}
}

func TestPurchaseOrderLineKindValidationRefusesInvalidKinds(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_kind_refusals")
	quotation := seedReportQuotation(t, database, 2, "PT. Kind Refusal", "Kind refusal source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250,
		}})
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-655-KIND-REFUSE",
		"2026-03-10", PurchaseOrderBusinessCategoryFm, nil)

	itemNo := 1
	cases := []struct {
		name  string
		item  PurchaseOrderItem
		label string
	}{
		{
			name: "extra with a quotation item",
			item: PurchaseOrderItem{
				Description: "Impossible extra", QuantityScaled: QuantityScale,
				UoM: "Lot", UnitPriceMinor: 1_000,
				QuotationItemNo: &itemNo, LineKind: PurchaseOrderLineKindExtra,
			},
			label: "line 2",
		},
		{
			name: "extra with a quotation sub-item",
			item: PurchaseOrderItem{
				Description: "Impossible extra sub", QuantityScaled: QuantityScale,
				UoM: "Lot", UnitPriceMinor: 1_000,
				QuotationItemNo: &itemNo, QuotationSubNo: &itemNo,
				LineKind: PurchaseOrderLineKindExtra,
			},
			label: "line 2",
		},
		{
			name: "unknown kind",
			item: PurchaseOrderItem{
				Description: "Mystery line", QuantityScaled: QuantityScale,
				UoM: "Lot", UnitPriceMinor: 1_000, LineKind: "pass_through",
			},
			label: "line 2",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := correctionFromSavedOrder(saved, user, "invalid line kind")
			input.Items = append(input.Items, test.item)
			_, err := database.CorrectPurchaseOrder(ctx, input)
			if !errors.Is(err, ErrPurchaseOrderLineKindInvalid) {
				t.Fatalf("error = %v; want ErrPurchaseOrderLineKindInvalid", err)
			}
			if !strings.Contains(err.Error(), test.label) {
				t.Errorf("error %q does not name %s", err.Error(), test.label)
			}
		})
	}

	// The compatibility window keeps a legacy null-reference line legal while
	// its kind says quotation: only #658's strict cutover retires it.
	legacy := correctionFromSavedOrder(saved, user, "legacy free line remains legal")
	legacy.Items = append(legacy.Items, PurchaseOrderItem{
		Description: "Legacy free line", QuantityScaled: QuantityScale,
		UoM: "Lot", UnitPriceMinor: 1_000,
	})
	if _, err := database.CorrectPurchaseOrder(ctx, legacy); err != nil {
		t.Fatalf("nil-reference quotation line = %v; want it accepted before #658", err)
	}
}

func itemLineKind(items []PurchaseOrderItem) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].LineKind
}

func correctionItemLineKinds(items []PurchaseOrderCorrectionItem) []string {
	kinds := make([]string, len(items))
	for index := range items {
		kinds[index] = items[index].LineKind
	}
	return kinds
}

func allocationCountForItem(t *testing.T, database *Store, itemID int64) int {
	t.Helper()
	var count int
	if err := database.pool.QueryRow(t.Context(), fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.purchase_order_allocations WHERE purchase_order_item_id = $1`,
		database.configSchema()), itemID).Scan(&count); err != nil {
		t.Fatalf("count allocations for line %d: %v", itemID, err)
	}
	return count
}

// TestExtraLineRefusesAnAllocationBeforeItClearsTheOrder reaches the defensive
// guard directly: validation refuses an extra line with a reference before any
// writer runs, so the allocation writer must not assume it was checked. It
// refuses before the DELETE, leaving the existing allocation intact inside the
// still-open transaction instead of half-clearing the record.
func TestExtraLineRefusesAnAllocationBeforeItClearsTheOrder(t *testing.T) {
	database := openTestSchema(t, "vera_po_655_kind_defensive")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Defensive Kind", "Defensive source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 2, Cost: 100, Sell: 250,
		}})
	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-655-DEFENSIVE",
		"2026-03-10", PurchaseOrderBusinessCategoryFm, nil)
	if len(readAllocations(t, database, ctx, saved.ID)) != 1 {
		t.Fatalf("fixture allocations = %d; want the quoted line's one",
			len(readAllocations(t, database, ctx, saved.ID)))
	}

	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	tx, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin defensive transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	source, err := loadPurchaseOrderSourceDetail(
		ctx, tx, database.configSchema(), quotation.ID, quotation.RevisionNumber)
	if err != nil {
		t.Fatalf("load defensive source: %v", err)
	}
	itemNo := 1
	item := PurchaseOrderItem{
		ID:              saved.Items[0].ID,
		LineNumber:      1,
		Description:     saved.Items[0].Description,
		QuantityScaled:  saved.Items[0].QuantityScaled,
		UoM:             saved.Items[0].UoM,
		UnitPriceMinor:  saved.Items[0].UnitPriceMinor,
		QuotationItemNo: &itemNo,
		LineKind:        PurchaseOrderLineKindExtra,
	}
	err = replacePurchaseOrderAllocations(ctx, tx, database.configSchema(), org.ID,
		saved.ID, source, []PurchaseOrderItem{item}, false)
	if !errors.Is(err, ErrPurchaseOrderLineKindInvalid) {
		t.Fatalf("extra-with-reference allocation error = %v; want ErrPurchaseOrderLineKindInvalid", err)
	}
	// The refusal runs before the DELETE, so the row is still visible inside the
	// transaction that refused it.
	var remaining int
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.purchase_order_allocations WHERE purchase_order_id = $1`,
		database.configSchema()), saved.ID).Scan(&remaining); err != nil {
		t.Fatalf("count allocations inside the refused transaction: %v", err)
	}
	if remaining != 1 {
		t.Errorf("allocations after the refusal = %d; want the original 1 (refused before the DELETE)",
			remaining)
	}
}

// TestExtraLineRefusesAnAllocationRow is the database's own guard, proven by a
// genuine upgrade: on 054 a raw allocation insert for a null-reference line
// succeeds, because nothing distinguishes it from a legacy line. After 055 the
// same row is refused by the composite kind foreign key. The store's typed
// refusal runs first on every write path; this test is what catches a path that
// bypasses the store.
func TestExtraLineRefusesAnAllocationRow(t *testing.T) {
	database := po518Unmigrated(t, "vera_po_655_kind_guard")
	po518ApplyThrough(t, database, 54)
	ctx := context.Background()
	quotation := seedBackfillQuotation(t, database)
	user := poTestUser(t, database)
	org, err := database.DefaultOrganization(ctx)
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	schema := database.configSchema()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}

	var orderID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, state, customer_name,
			received_date, currency, tax_zone, created_by, updated_by,
			subtotal_minor, ppn_bps, ppn_minor, grand_total_minor
		) VALUES ($1,$2,$3,'draft',$4,$5,'IDR','FTZ',$6,$6,$7,0,0,$7)
		RETURNING id`, schema),
		org.ID, quotation.ID, quotation.RevisionNumber, "PT. Kind Guard",
		time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), user.ID, 500).Scan(&orderID); err != nil {
		t.Fatalf("insert the guard fixture purchase order: %v", err)
	}

	insertLine := func(lineNumber int, description string) int64 {
		t.Helper()
		var itemID int64
		if err := pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_items (
				purchase_order_id, line_number, description, quotation_item_no,
				quotation_sub_no, quantity_scaled, uom, unit_price_minor,
				gross_minor, discount_minor, net_minor)
			VALUES ($1,$2,$3,NULL,NULL,10000,'Lot',500,500,0,500)
			RETURNING id`, schema), orderID, lineNumber, description).Scan(&itemID); err != nil {
			t.Fatalf("insert guard fixture line %d: %v", lineNumber, err)
		}
		return itemID
	}

	// INSERT with source_revision_id NULL: the revision foreign key is skipped,
	// so only the line-kind key can decide whether an extra line may own this
	// row. That is exactly the bypass this guard exists for.
	insertAllocation := func(itemID int64) error {
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_allocations (
				org_id, purchase_order_id, purchase_order_item_id, quotation_id,
				source_revision, source_revision_id, quotation_item_no,
				quotation_sub_no, quantity_scaled, source_uom, ordered_uom,
				quoted_unit_price_minor, revenue_minor, cost_unit_minor)
			VALUES ($1,$2,$3,$4,$5,NULL,1,NULL,10000,'Pc','Pc',500,500,100)`, schema),
			org.ID, orderID, itemID, quotation.ID, quotation.RevisionNumber)
		return err
	}

	extraItemID := insertLine(1, "Null-reference line")
	if err := insertAllocation(extraItemID); err != nil {
		t.Fatalf("pre-055 schema refused the allocation: %v (the red proof would not hold)", err)
	}

	// 055 is the migration under test. The pre-existing allocation defaults to
	// quotation kind and still satisfies the new key.
	po518ApplyOnly(t, database, 55)
	var survivingKind string
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT line_kind FROM %s.purchase_order_allocations
		WHERE purchase_order_item_id = $1`, schema), extraItemID).Scan(&survivingKind); err != nil {
		t.Fatalf("read the upgraded allocation kind: %v", err)
	}
	if survivingKind != PurchaseOrderLineKindQuotation {
		t.Fatalf("upgraded allocation kind = %q; want quotation", survivingKind)
	}

	// Make the line a deliberate extra, as the store does once a match is
	// cleared. The old allocation must go first: the key it names is changing.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_allocations WHERE purchase_order_item_id = $1`,
		schema), extraItemID); err != nil {
		t.Fatalf("clear the upgraded allocation: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_order_items SET line_kind = 'extra' WHERE id = $1`,
		schema), extraItemID); err != nil {
		t.Fatalf("mark the line extra: %v", err)
	}

	err = insertAllocation(extraItemID)
	if err == nil {
		t.Fatal("the database accepted an allocation for an extra line")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("extra allocation error = %v; want *pgconn.PgError", err)
	}
	if pgErr.Code != "23503" {
		t.Fatalf("extra allocation SQLSTATE = %s; want 23503 (%v)", pgErr.Code, err)
	}
	if pgErr.ConstraintName != "po_allocations_line_kind_fkey" {
		t.Fatalf("extra allocation constraint = %q; want po_allocations_line_kind_fkey",
			pgErr.ConstraintName)
	}

	// The refusal is the kind, not the fixture: the same insert for a
	// quotation-kind line still succeeds.
	quotationItemID := insertLine(2, "Quotation-kind line")
	if err := insertAllocation(quotationItemID); err != nil {
		t.Fatalf("quotation-kind allocation = %v; want it accepted", err)
	}
}
