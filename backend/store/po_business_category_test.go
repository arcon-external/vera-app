package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Issue #628: every purchase order carries a business category, chosen at
// entry and required before the final save. These tests drive the real store
// against a disposable schema: the round trip through create, draft save, read
// and the archive's category filter; the final-save refusal that leaves the
// draft editable; and the correction history keeping the category the corrected
// version carried.

// saveDraftWithCategory is a full draft save that keeps every stored header
// value and changes only the category, so the later final save compares equal
// to its source.
func saveDraftWithCategory(t *testing.T, database *Store, ctx context.Context, draft PurchaseOrder, category string, user AuthUser) PurchaseOrder {
	t.Helper()
	input := finalizeInput(draft, user, draft.PONumber).PurchaseOrderUpdate
	input.BusinessCategory = category
	input.ExpectedVersion = draft.Version
	saved, err := database.UpdatePurchaseOrderDraft(ctx, input)
	if err != nil {
		t.Fatalf("save draft with category %q: %v", category, err)
	}
	return saved
}

func TestPurchaseOrderBusinessCategoryRoundTripsThroughCreateSaveReadAndList(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_roundtrip")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Category round trip")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	// Create carries the category. A linked draft starts empty unless the
	// caller names one, and an empty category is a legal draft value.
	created, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:      quotation.ID,
		RevisionNumber:   quotation.RevisionNumber,
		BusinessCategory: PurchaseOrderBusinessCategoryFab,
		CreatedBy:        user.ID,
	})
	if err != nil {
		t.Fatalf("create draft with a category: %v", err)
	}
	if created.BusinessCategory != PurchaseOrderBusinessCategoryFab {
		t.Fatalf("created category = %q; want %q",
			created.BusinessCategory, PurchaseOrderBusinessCategoryFab)
	}

	// A draft save replaces the category with what the editor chose.
	updated := saveDraftWithCategory(t, database, ctx, created, PurchaseOrderBusinessCategoryFm, user)
	if updated.BusinessCategory != PurchaseOrderBusinessCategoryFm {
		t.Fatalf("saved category = %q; want %q",
			updated.BusinessCategory, PurchaseOrderBusinessCategoryFm)
	}

	// Read returns it.
	reloaded, err := database.GetPurchaseOrder(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.BusinessCategory != PurchaseOrderBusinessCategoryFm {
		t.Errorf("reloaded category = %q; want %q",
			reloaded.BusinessCategory, PurchaseOrderBusinessCategoryFm)
	}

	// List returns it and filters by exactly one value.
	all, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all.Rows) != 1 || all.Rows[0].BusinessCategory != PurchaseOrderBusinessCategoryFm {
		t.Fatalf("all rows = %+v; want one FM row", all.Rows)
	}
	fm, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		BusinessCategory: PurchaseOrderBusinessCategoryFm,
	})
	if err != nil {
		t.Fatalf("list FM: %v", err)
	}
	if fm.Total != 1 || len(fm.Rows) != 1 {
		t.Errorf("FM page total = %d rows = %d; want 1/1", fm.Total, len(fm.Rows))
	}
	fab, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		BusinessCategory: PurchaseOrderBusinessCategoryFab,
	})
	if err != nil {
		t.Fatalf("list FAB: %v", err)
	}
	if fab.Total != 0 || len(fab.Rows) != 0 {
		t.Errorf("FAB page total = %d rows = %d; want an empty page", fab.Total, len(fab.Rows))
	}

	// An unknown filter is refused rather than treated as "everything".
	if _, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		BusinessCategory: "NOT_A_CATEGORY",
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("unknown category filter = %v; want ErrPurchaseOrderInvalid", err)
	}
}

func TestPurchaseOrderBlankDraftPersistsItsBusinessCategory(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_blank")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft, err := database.CreatePurchaseOrderBlankDraft(ctx, PurchaseOrderBlankCreate{
		BusinessCategory: PurchaseOrderBusinessCategorySupplyGeneral,
		CreatedBy:        user.ID,
	})
	if err != nil {
		t.Fatalf("create blank draft: %v", err)
	}
	if draft.BusinessCategory != PurchaseOrderBusinessCategorySupplyGeneral {
		t.Fatalf("blank draft category = %q; want %q",
			draft.BusinessCategory, PurchaseOrderBusinessCategorySupplyGeneral)
	}
}

func TestPurchaseOrderDraftSaveRefusesAnUnknownBusinessCategory(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_unknown")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Unknown category")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID:               draft.ID,
		BusinessCategory: "NOT_A_CATEGORY",
		UpdatedBy:        user.ID,
		Items:            draft.Items,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("unknown category save = %v; want ErrPurchaseOrderInvalid", err)
	}
}

func TestFinalizePurchaseOrderRefusesAnEmptyBusinessCategoryAndKeepsTheDraft(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_finalize")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Category finalize")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	empty := finalizeInput(draft, user, "PO-CATEGORY-1")
	empty.BusinessCategory = ""
	if _, err := database.FinalizePurchaseOrder(ctx, empty); !errors.Is(err, ErrPurchaseOrderBusinessCategoryRequired) {
		t.Fatalf("empty-category finalize = %v; want ErrPurchaseOrderBusinessCategoryRequired", err)
	}

	// The refusal wrote nothing: the record is still an editable draft.
	reloaded, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after refusal: %v", err)
	}
	if reloaded.State != PurchaseOrderStateDraft || reloaded.SavedAt != nil {
		t.Errorf("a refused final save changed the record: %+v", reloaded)
	}
	if reloaded.BusinessCategory != "" {
		t.Errorf("a refused final save stored category %q; want empty", reloaded.BusinessCategory)
	}

	// The draft can still be saved with a category, and then finalised.
	selected := saveDraftWithCategory(t, database, ctx, draft, PurchaseOrderBusinessCategoryFab, user)
	finalized := finalizeInput(selected, user, "PO-CATEGORY-1")
	saved, err := database.FinalizePurchaseOrder(ctx, finalized)
	if err != nil {
		t.Fatalf("finalize with FAB: %v", err)
	}
	if saved.BusinessCategory != PurchaseOrderBusinessCategoryFab {
		t.Errorf("saved category = %q; want FAB", saved.BusinessCategory)
	}
}

func TestPurchaseOrderCorrectionKeepsThePreviousBusinessCategory(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_correction")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Category correction")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	withFab := saveDraftWithCategory(t, database, ctx, draft, PurchaseOrderBusinessCategoryFab, user)
	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(withFab, user, "PO-CATEGORY-2"))
	if err != nil {
		t.Fatalf("finalize FAB: %v", err)
	}

	received := saved.ReceivedDate
	corrected, err := database.CorrectPurchaseOrder(ctx, PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: PurchaseOrderUpdate{
			ID:               saved.ID,
			ExpectedVersion:  saved.Version,
			UpdatedBy:        user.ID,
			PONumber:         saved.PONumber,
			ReceivedDate:     &received,
			BusinessCategory: PurchaseOrderBusinessCategoryFm,
			Items:            saved.Items,
		},
		Reason: "The order belongs to facility management",
	})
	if err != nil {
		t.Fatalf("correct category: %v", err)
	}
	if corrected.BusinessCategory != PurchaseOrderBusinessCategoryFm {
		t.Errorf("corrected category = %q; want FM", corrected.BusinessCategory)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history))
	}
	if history[0].BusinessCategory != PurchaseOrderBusinessCategoryFab {
		t.Errorf("snapshot category = %q; want the replaced FAB", history[0].BusinessCategory)
	}
}

func TestSavedPurchaseOrderWithoutACategoryReadsAsUnassigned(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_legacy")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Legacy category")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(draft, user, "PO-CATEGORY-LEGACY"))
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// A record saved before the feature carries the migration's empty default,
	// never an invented category. The column is the only truth.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE `+database.configSchema()+`.purchase_orders
		SET business_category = ''
		WHERE id = $1`, saved.ID); err != nil {
		t.Fatalf("clear stored category: %v", err)
	}
	reloaded, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload legacy record: %v", err)
	}
	if reloaded.BusinessCategory != "" {
		t.Errorf("legacy category = %q; want empty (Unassigned)", reloaded.BusinessCategory)
	}
	all, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all.Rows) != 1 || all.Rows[0].BusinessCategory != "" {
		t.Errorf("legacy row = %+v; want one empty category (Unassigned)", all.Rows)
	}
}

// savedPurchaseOrderWithCategory finalizes one linked draft with an explicit
// category, so a source-repair test starts from a real saved record rather than
// from a direct column write.
func savedPurchaseOrderWithCategory(
	t *testing.T,
	database *Store,
	ctx context.Context,
	user AuthUser,
	quotation QuotationRecord,
	category, number string,
) PurchaseOrder {
	t.Helper()
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	withCategory := saveDraftWithCategory(t, database, ctx, draft, category, user)
	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(withCategory, user, number))
	if err != nil {
		t.Fatalf("finalize with category %q: %v", category, err)
	}
	return saved
}

// TestRepairPurchaseOrderSourcePreservesAnOmittedBusinessCategory pins the
// source-repair rule from #628: a repair moves the link, it is not a category
// edit. A payload that leaves the field out keeps the FAB the saved record
// carried, and the snapshot of the version the repair replaced keeps it too.
func TestRepairPurchaseOrderSourcePreservesAnOmittedBusinessCategory(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_repair")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved := savedPurchaseOrderWithCategory(t, database, ctx, staff, original,
		PurchaseOrderBusinessCategoryFab, "PO-CATEGORY-REPAIR")

	// repairInput deliberately leaves BusinessCategory at its zero value: the
	// repair form does not own the category, so omission means "keep".
	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "the order followed the wrong revision"))
	if err != nil {
		t.Fatalf("repair with omitted category: %v", err)
	}
	if repaired.BusinessCategory != PurchaseOrderBusinessCategoryFab {
		t.Errorf("repaired category = %q; want the preserved FAB", repaired.BusinessCategory)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want 1 source-repair snapshot", len(history))
	}
	if history[0].BusinessCategory != PurchaseOrderBusinessCategoryFab {
		t.Errorf("snapshot category = %q; want the preserved FAB", history[0].BusinessCategory)
	}
}

// TestRepairPurchaseOrderSourceKeepsALegacyUnassignedCategory proves omission
// is preservation, not defaulting: a record saved before #628 stays empty
// through a source repair rather than acquiring a category nobody chose.
func TestRepairPurchaseOrderSourceKeepsALegacyUnassignedCategory(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_repair_legacy")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved := savedPurchaseOrderWithCategory(t, database, ctx, staff, original,
		PurchaseOrderBusinessCategoryOthers, "PO-CATEGORY-REPAIR-LEGACY")

	// A record saved before migration 050 carries the empty default.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders SET business_category = ''
		WHERE id = $1`, database.configSchema()), saved.ID); err != nil {
		t.Fatalf("clear stored category: %v", err)
	}
	legacy, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload legacy record: %v", err)
	}
	if legacy.BusinessCategory != "" {
		t.Fatalf("legacy category = %q; want empty", legacy.BusinessCategory)
	}

	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, legacy, admin, target, "the order followed the wrong revision"))
	if err != nil {
		t.Fatalf("repair legacy record: %v", err)
	}
	if repaired.BusinessCategory != "" {
		t.Errorf("repaired legacy category = %q; want preserved empty", repaired.BusinessCategory)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history))
	}
	if history[0].BusinessCategory != "" {
		t.Errorf("snapshot category = %q; want preserved empty", history[0].BusinessCategory)
	}
}

// TestPurchaseOrderCorrectionRefusesAnEmptyCategoryWithNeutralWording proves
// the correction path shares the final save's refusal and that the message is
// not written for the Final Save button alone.
func TestPurchaseOrderCorrectionRefusesAnEmptyCategoryWithNeutralWording(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_correction_wording")
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Category wording")
	user := poTestUser(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-CATEGORY-WORDING")

	input := correctionInput(saved, user, "Reclassify the order", saved.Version)
	input.BusinessCategory = ""
	if _, err := database.CorrectPurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderBusinessCategoryRequired) {
		t.Fatalf("empty-category correction = %v; want ErrPurchaseOrderBusinessCategoryRequired", err)
	}
	message := ErrPurchaseOrderBusinessCategoryRequired.Error()
	if !strings.Contains(strings.ToLower(message), "business category") {
		t.Errorf("refusal %q does not name the business category", message)
	}
	if strings.Contains(strings.ToLower(message), "final save") {
		t.Errorf("refusal %q names Final Save, but a correction returns the same error", message)
	}
}

// TestMigration050CheckRejectsInvalidBusinessCategories drives raw SQL so the
// database constraint itself is proven, not only the Go validator: both the
// live row and the correction snapshot reject an invalid code with SQLSTATE
// 23514 from the migration's CHECK.
func TestMigration050CheckRejectsInvalidBusinessCategories(t *testing.T) {
	database := openTestSchema(t, "vera_po_category_check")
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Category check")
	user := poTestUser(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-CATEGORY-CHECK")

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	invalid := []string{"NOT_A_CATEGORY", "fab", "FM ", " FM"}
	for _, category := range invalid {
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			UPDATE %s.purchase_orders SET business_category = $1
			WHERE id = $2`, schema), category, saved.ID)
		requireBusinessCategoryCheck(t, err, "purchase_orders", category)
	}
	for _, category := range invalid {
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.purchase_order_corrections (
				org_id, purchase_order_id, previous_version, resulting_version,
				kind, state, reason, actor, quotation_id, source_revision,
				customer_id, currency, received_date, business_category
			) VALUES (
				(SELECT org_id FROM %s.purchase_orders WHERE id = $1),
				$1, $2, $3, 'correction', 'saved', 'check', $4, $5, $6, $7, $8, $9, $10
			)`, schema, schema),
			saved.ID, saved.Version, saved.Version+1, user.ID,
			saved.QuotationID, saved.SourceRevision, saved.CustomerID,
			saved.Currency, saved.ReceivedDate, category)
		requireBusinessCategoryCheck(t, err, "purchase_order_corrections", category)
	}
}

func requireBusinessCategoryCheck(t *testing.T, err error, table, category string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("%s accepted category %q: error = %v; want SQLSTATE 23514", table, category, err)
	}
	if !strings.Contains(pgErr.ConstraintName, "business_category_check") {
		t.Errorf("%s category %q failed %q; want the business_category_check constraint",
			table, category, pgErr.ConstraintName)
	}
}
