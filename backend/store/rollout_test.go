package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vera-app/backend/poreport"
)

// Issue #658: the allocation cutover has two modes. Compatibility mode is
// today's server and must keep accepting every payload an older desktop sends.
// Strict mode is the post-cutover server: a source-less blank draft, a new free
// quotation line, and any new purchase-order discount are refused, while every
// record created before the flip stays readable, reportable and correctable.
//
// The two modes share one store; VERA_PO_ALLOCATIONS_STRICT is the only switch,
// read once by LoadConfig. These tests drive the real store against a
// disposable schema, exactly as the two server deployments would.

// strictStore returns a second handle on the same database with the cutover
// flag on. Copying the Store shares its pool; the original cleanup closes it.
func strictStore(database *Store) *Store {
	strict := *database
	strict.config.StrictPOAllocations = true
	return &strict
}

// TestCompatibilityModeAcceptsTheLegacyPurchaseOrderPayload pins the window:
// with the flag absent (the zero value) a source-less blank draft, a new free
// line without a reference, a row discount, and a header discount/adjustment
// all save, because that is what a pre-v0.24.0 desktop sends today.
func TestCompatibilityModeAcceptsTheLegacyPurchaseOrderPayload(t *testing.T) {
	database := openTestSchema(t, "vera_rollout_658_compat")
	ctx := poContext(t, database)
	user := poTestUser(t, database)

	blank := createBlankDraft(t, database, ctx, user)
	update := blankDraftUpdate(blank, blank.Version, user)
	update.DiscountPercent = 10
	update.AdjustmentMinor = 5_000
	update.AdjustmentReason = "legacy adjustment"
	update.Items[0].DiscountEnabled = true
	update.Items[0].DiscountPercent = 5
	update.Items[0].LineKind = ""

	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("compatibility mode refused the legacy payload: %v", err)
	}
	if len(saved.Items) != 1 || saved.Items[0].QuotationItemNo != nil {
		t.Fatalf("saved items = %+v; want the free line without a reference", saved.Items)
	}
	if !saved.Items[0].DiscountEnabled || saved.Items[0].DiscountPercent != 5 {
		t.Errorf("row discount = %v/%d; want the submitted 5%%",
			saved.Items[0].DiscountEnabled, saved.Items[0].DiscountPercent)
	}

	// The header discount and adjustment are retired by #573, so the save
	// accepts the fields and stores zero. What matters here is that the write
	// was not refused.
	if saved.DiscountPercent != 0 || saved.AdjustmentMinor != 0 {
		t.Errorf("retired header fields stored %d/%d; want zero",
			saved.DiscountPercent, saved.AdjustmentMinor)
	}
}

// TestStrictModeRefusesABlankPurchaseOrderDraft proves the first strict gate:
// a source-less draft cannot start after the cutover, and the refusal leaves no
// row behind.
func TestStrictModeRefusesABlankPurchaseOrderDraft(t *testing.T) {
	database := openTestSchema(t, "vera_rollout_658_blank")
	strict := strictStore(database)
	ctx := poContext(t, database)
	user := poTestUser(t, database)

	_, err := strict.CreatePurchaseOrderBlankDraft(ctx, PurchaseOrderBlankCreate{
		TaxZone:   "FTZ",
		TaxType:   "Goods",
		CreatedBy: user.ID,
	})
	if !errors.Is(err, ErrPurchaseOrderBlankDraftRetired) {
		t.Fatalf("strict blank draft error = %v; want ErrPurchaseOrderBlankDraftRetired", err)
	}
	var count int
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s.purchase_orders`, database.configSchema())).Scan(&count); err != nil {
		t.Fatalf("count purchase orders: %v", err)
	}
	if count != 0 {
		t.Errorf("refused blank draft wrote %d row(s)", count)
	}
}

// TestStrictModeRefusesANewFreeLineAndAcceptsAnExtra covers the free-line gate
// from both sides: a new quotation-kind line with no reference is retired, and
// the explicit extra line (#655) that replaces it still saves.
func TestStrictModeRefusesANewFreeLineAndAcceptsAnExtra(t *testing.T) {
	database := openTestSchema(t, "vera_rollout_658_free_line")
	strict := strictStore(database)
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Rollout Buyer", "Rollout source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 3, Cost: 100, Sell: 250,
		}})
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	legacy := blankDraftUpdate(draft, draft.Version, user)
	legacy.Items = []PurchaseOrderItem{
		draft.Items[0],
		{
			Description:    "Free line",
			QuantityScaled: QuantityScale,
			UoM:            "Pc",
			UnitPriceMinor: 40,
			LineKind:       PurchaseOrderLineKindQuotation,
		},
	}
	_, err := strict.UpdatePurchaseOrderDraft(ctx, legacy)
	if !errors.Is(err, ErrPurchaseOrderFreeLineRetired) {
		t.Fatalf("new free line error = %v; want ErrPurchaseOrderFreeLineRetired", err)
	}

	// An older desktop sends no lineKind at all; absence is quotation, so the
	// same new line is still refused.
	legacy.Items[1].LineKind = ""
	if _, err := strict.UpdatePurchaseOrderDraft(ctx, legacy); !errors.Is(err, ErrPurchaseOrderFreeLineRetired) {
		t.Fatalf("kind-less free line error = %v; want ErrPurchaseOrderFreeLineRetired", err)
	}

	// The explicit extra is the supported way to add a pass-through line.
	extra := legacy
	extra.Items = []PurchaseOrderItem{
		draft.Items[0],
		{
			Description:    "Freight",
			QuantityScaled: QuantityScale,
			UoM:            "Lot",
			UnitPriceMinor: 40,
			LineKind:       PurchaseOrderLineKindExtra,
		},
	}
	saved, err := strict.UpdatePurchaseOrderDraft(ctx, extra)
	if err != nil {
		t.Fatalf("strict refused an extra line: %v", err)
	}
	if len(saved.Items) != 2 || saved.Items[1].LineKind != PurchaseOrderLineKindExtra {
		t.Errorf("saved items = %+v; want the quoted line and an extra", saved.Items)
	}
}

// TestStrictModeRefusesAStrippedReference proves "new or changed": a new line
// is refused, and so is an existing referenced line whose reference the save
// removes. Editing the line without touching the reference is not this gate.
func TestStrictModeRefusesAStrippedReference(t *testing.T) {
	database := openTestSchema(t, "vera_rollout_658_stripped")
	strict := strictStore(database)
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Rollout Buyer", "Stripped source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 3, Cost: 100, Sell: 250,
		}})
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	update := finalizeInput(draft, user, "").PurchaseOrderUpdate
	update.Items[0].QuotationItemNo = nil
	if _, err := strict.UpdatePurchaseOrderDraft(ctx, update); !errors.Is(err, ErrPurchaseOrderFreeLineRetired) {
		t.Fatalf("stripped reference error = %v; want ErrPurchaseOrderFreeLineRetired", err)
	}

	// The same line, reference intact, only reworded: allowed.
	update.Items[0] = draft.Items[0]
	update.Items[0].Description = "Quoted line, reworded on the purchase order"
	reworded, err := strict.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("strict refused a referenced line edit: %v", err)
	}
	if reworded.Items[0].QuotationItemNo == nil {
		t.Errorf("the referenced line lost its reference")
	}
}

// TestStrictModeRefusesRowAndHeaderDiscounts covers the discount gate on both
// levels: a row discount on a new line, and a header discount or adjustment.
func TestStrictModeRefusesRowAndHeaderDiscounts(t *testing.T) {
	database := openTestSchema(t, "vera_rollout_658_discount")
	strict := strictStore(database)
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Rollout Buyer", "Discount source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 3, Cost: 100, Sell: 250,
		}})
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	base := finalizeInput(draft, user, "").PurchaseOrderUpdate

	rowDiscount := base
	rowDiscount.Items = append([]PurchaseOrderItem(nil), base.Items...)
	rowDiscount.Items[0].DiscountEnabled = true
	rowDiscount.Items[0].DiscountPercent = 10
	if _, err := strict.UpdatePurchaseOrderDraft(ctx, rowDiscount); !errors.Is(err, ErrPurchaseOrderDiscountForbidden) {
		t.Fatalf("row discount error = %v; want ErrPurchaseOrderDiscountForbidden", err)
	}

	headerDiscount := base
	headerDiscount.DiscountPercent = 10
	if _, err := strict.UpdatePurchaseOrderDraft(ctx, headerDiscount); !errors.Is(err, ErrPurchaseOrderDiscountForbidden) {
		t.Fatalf("header discount error = %v; want ErrPurchaseOrderDiscountForbidden", err)
	}

	adjustment := base
	adjustment.AdjustmentMinor = 5_000
	adjustment.AdjustmentReason = "rounding"
	if _, err := strict.UpdatePurchaseOrderDraft(ctx, adjustment); !errors.Is(err, ErrPurchaseOrderDiscountForbidden) {
		t.Fatalf("header adjustment error = %v; want ErrPurchaseOrderDiscountForbidden", err)
	}

	// The untouched payload still saves: the gate refuses new discounts, not
	// every save of a quoted draft.
	if _, err := strict.UpdatePurchaseOrderDraft(ctx, base); err != nil {
		t.Fatalf("strict refused an undiscounted draft save: %v", err)
	}
}

// TestStrictModeKeepsHistoricalLegacyLinesReadableReportableAndCorrectable is
// the anti-data-loss proof. A saved pre-cutover record carries a free line and
// a row discount. After the flip it must read back, report the same way, and
// still accept a correction that leaves those historical values unchanged.
func TestStrictModeKeepsHistoricalLegacyLinesReadableReportableAndCorrectable(t *testing.T) {
	database := openTestSchema(t, "vera_rollout_658_historical")
	strict := strictStore(database)
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Rollout Buyer", "Historical source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 3, Cost: 100, Sell: 250,
		}})

	// Compatibility mode creates today's shape: the quoted line (with a row
	// discount) plus a free pass-through line.
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	legacy := finalizeInput(draft, user, "").PurchaseOrderUpdate
	legacy.Items = append([]PurchaseOrderItem(nil), legacy.Items...)
	legacy.Items[0].DiscountEnabled = true
	legacy.Items[0].DiscountPercent = 10
	legacy.Items = append(legacy.Items, PurchaseOrderItem{
		Description:    "Legacy free line",
		QuantityScaled: QuantityScale,
		UoM:            "Lot",
		UnitPriceMinor: 40,
	})
	updated, err := database.UpdatePurchaseOrderDraft(ctx, legacy)
	if err != nil {
		t.Fatalf("compatibility mode refused the historical shape: %v", err)
	}

	finalize := finalizeInput(updated, user, "PO-658-HISTORICAL")
	finalize.ExpectedVersion = updated.Version
	finalize.Decisions = []PurchaseOrderDecision{{
		Key: "line:1:discount", Choice: PurchaseOrderChoicePO,
	}}
	finalize.AcknowledgedExtra = []int{2}
	saved, err := database.FinalizePurchaseOrder(ctx, finalize)
	if err != nil {
		t.Fatalf("finalize the historical shape: %v", err)
	}
	if len(saved.Items) != 2 || saved.Items[1].QuotationItemNo != nil {
		t.Fatalf("saved items = %+v; want the quoted line and a nil-reference line", saved.Items)
	}

	// Readable under strict.
	read, err := strict.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("strict read of the historical record: %v", err)
	}
	if read.PONumber != "PO-658-HISTORICAL" || len(read.Items) != 2 {
		t.Fatalf("strict read = %q with %d lines; want the stored identity", read.PONumber, len(read.Items))
	}

	// Reportable under strict: the free line stays uncosted, never invented
	// into profit.
	report := mustReport(t, strict, ctx,
		reportQuery("2026-01-01", "2027-01-01", poreport.BucketMonth, poreport.CategoryAll, "IDR"))
	if report.Totals.AmountMinor != 715 {
		t.Errorf("amount = %d; want the discounted 675 + free 40 = 715", report.Totals.AmountMinor)
	}
	if report.Totals.CostedMinor != 675 {
		t.Errorf("costed = %d; want the quoted line's 675", report.Totals.CostedMinor)
	}
	if report.Totals.UncostedMinor != 40 {
		t.Errorf("uncosted = %d; want the free line's 40", report.Totals.UncostedMinor)
	}

	// Correctable under strict when the historical values are unchanged.
	unchanged := correctionFromSavedOrder(saved, user, "keep the historical shape")
	corrected, err := strict.CorrectPurchaseOrder(ctx, unchanged)
	if err != nil {
		t.Fatalf("strict refused an unchanged historical correction: %v", err)
	}
	if len(corrected.Items) != 2 || corrected.Items[0].DiscountPercent != 10 ||
		corrected.Items[1].QuotationItemNo != nil {
		t.Errorf("corrected items = %+v; want the historical values preserved", corrected.Items)
	}

	// A correction may reword the legacy free line; it may not remove an
	// existing reference, and it may not invent a new free line.
	current, err := strict.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("re-read after the correction: %v", err)
	}
	errata := correctionFromSavedOrder(current, user, "reword the legacy line")
	errata.Items[1].Description = "Legacy free line, reworded"
	if _, err := strict.CorrectPurchaseOrder(ctx, errata); err != nil {
		t.Fatalf("strict refused rewording a legacy free line: %v", err)
	}

	current, err = strict.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("re-read after the reword: %v", err)
	}
	stripped := correctionFromSavedOrder(current, user, "strip the reference")
	stripped.Items[0].QuotationItemNo = nil
	if _, err := strict.CorrectPurchaseOrder(ctx, stripped); !errors.Is(err, ErrPurchaseOrderFreeLineRetired) {
		t.Fatalf("strict correction stripping a reference = %v; want ErrPurchaseOrderFreeLineRetired", err)
	}

	added := correctionFromSavedOrder(current, user, "add another free line")
	added.Items = append(added.Items, PurchaseOrderItem{
		Description:    "New free line",
		QuantityScaled: QuantityScale,
		UoM:            "Lot",
		UnitPriceMinor: 1_000,
	})
	if _, err := strict.CorrectPurchaseOrder(ctx, added); !errors.Is(err, ErrPurchaseOrderFreeLineRetired) {
		t.Fatalf("strict correction adding a free line = %v; want ErrPurchaseOrderFreeLineRetired", err)
	}

	changedDiscount := correctionFromSavedOrder(current, user, "move the historical discount")
	changedDiscount.Items[0].DiscountPercent = 5
	if _, err := strict.CorrectPurchaseOrder(ctx, changedDiscount); !errors.Is(err, ErrPurchaseOrderDiscountForbidden) {
		t.Fatalf("strict correction changing a discount = %v; want ErrPurchaseOrderDiscountForbidden", err)
	}
}

// TestStrictPOAllocationsRefusesAQuotationWithMoreThanOneLivePO runs the
// rollback precheck the release runbook ships. A destructive schema rollback
// would have to restore one purchase order per quotation; it is refused while
// any quotation carries more than one live record.
func TestStrictPOAllocationsRefusesAQuotationWithMoreThanOneLivePO(t *testing.T) {
	precheck := readRollbackPrecheck(t)
	database := openTestSchema(t, "vera_rollout_658_precheck")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Precheck Buyer", "Precheck source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Quoted line", UoM: "Pc", Qty: 3, Cost: 100, Sell: 250,
		}})

	createFinalizeDraft(t, database, ctx, user, quotation)
	if rows := runRollbackPrecheck(t, database, precheck); len(rows) != 0 {
		t.Fatalf("one live PO reported as unsafe: %v", rows)
	}

	// A second live record for the same quotation is legal under 053, and is
	// exactly what a destructive rollback cannot represent.
	second, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("second draft for one quotation: %v", err)
	}
	rows := runRollbackPrecheck(t, database, precheck)
	if len(rows) != 1 {
		t.Fatalf("precheck rows = %v; want exactly the quotation with two live POs", rows)
	}
	if rows[0].QuotationID != quotation.ID || rows[0].LivePurchaseOrders != 2 {
		t.Errorf("precheck row = %+v; want quotation %d with 2 POs", rows[0], quotation.ID)
	}

	// Deleting the second draft restores the single-PO shape the rollback
	// needs, and the precheck stops refusing.
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_orders WHERE id = $1`, database.configSchema()),
		second.ID); err != nil {
		t.Fatalf("delete the second draft: %v", err)
	}
	if rows := runRollbackPrecheck(t, database, precheck); len(rows) != 0 {
		t.Fatalf("precheck still refuses after one PO remains: %v", rows)
	}
}

// TestAllocationMigrationsShipNoDownFile pins the forward-only rule the
// runbook relies on. The migration runner applies every *.sql once and has no
// down path, so a _down file would be replayed as a new forward migration --
// which is why the rollback procedure is a server rollback plus corrections,
// and why a destructive schema rollback needs a new, hand-written migration.
func TestAllocationMigrationsShipNoDownFile(t *testing.T) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Name()), "_down") {
			t.Errorf("migration %q is a down file; the runner would apply it forward", entry.Name())
		}
	}
}

// The release runbook is the artifact an operator follows, and
// announce-release.sh is the one thing that announces a release. Pin the two
// facts the cutover depends on: the floor is raised by a named manual command
// and verified from the public endpoint, and the script only carries the live
// floor forward.
func TestAllocationRolloutRunbookNamesTheFloorAndUptakeChecks(t *testing.T) {
	runbook := readRepoFile(t, "docs", "agents", "release.md")
	for _, want := range []string{
		"## Allocation rollout",
		"Stage A",
		"Stage B",
		"VERA_PO_ALLOCATIONS_STRICT",
		"gcloud run services update vera-api",
		"VERA_APP_MIN_SUPPORTED=v0.24.0",
		"https://api.arconpentapersada.co.id/v1/app/latest",
		"rollback",
		"correction",
	} {
		if !strings.Contains(runbook, want) {
			t.Errorf("release runbook is missing %q", want)
		}
	}

	script := readRepoFile(t, "scripts", "announce-release.sh")
	if !strings.Contains(script, "VERA_APP_MIN_SUPPORTED=${LIVE_MIN}") {
		t.Error("announce-release.sh no longer carries the live minSupported forward")
	}
	if strings.Contains(script, "VERA_APP_MIN_SUPPORTED=v0.") {
		t.Error("announce-release.sh hard-codes a minimum version; the floor must be raised by hand")
	}
	if !strings.Contains(script, "refusing to guess a floor") {
		t.Error("announce-release.sh no longer refuses an unclear live floor")
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(contents)
}

// rollbackPrecheckRow is one quotation the destructive rollback precheck
// refuses.
type rollbackPrecheckRow struct {
	QuotationID        int64
	QuoteNumber        string
	LivePurchaseOrders int64
}

// readRollbackPrecheck reads the operator query the runbook ships, so the test
// runs the same SQL a human would.
func readRollbackPrecheck(t *testing.T) string {
	t.Helper()
	return readRepoFile(t, "docs", "agents", "allocation-rollback-precheck.sql")
}

// runRollbackPrecheck executes the runbook query with this test's schema
// substituted for the psql :"schema" variable.
func runRollbackPrecheck(t *testing.T, database *Store, precheck string) []rollbackPrecheckRow {
	t.Helper()
	sql := strings.ReplaceAll(precheck, `:"schema"`, quoteIdentifier(database.config.Schema))
	rows, err := database.pool.Query(context.Background(), sql)
	if err != nil {
		t.Fatalf("run the rollback precheck: %v", err)
	}
	defer rows.Close()
	found := make([]rollbackPrecheckRow, 0)
	for rows.Next() {
		var row rollbackPrecheckRow
		if err := rows.Scan(&row.QuotationID, &row.QuoteNumber, &row.LivePurchaseOrders); err != nil {
			t.Fatalf("scan rollback precheck row: %v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rollback precheck rows: %v", err)
	}
	return found
}
