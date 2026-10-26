package store

import (
	"context"
	"testing"
)

// Issue #504: the extraction table is gone, and the manual Purchase Order path
// that shared the schema is untouched.
//
// The test drives a real disposable schema through every migration, so 038
// creates the table and 040 drops it in the same run — the fresh-deployment
// path. The existing-deployment path (038 already applied, then 040) was
// verified separately during the #503 review; 040 is a plain DROP TABLE IF
// EXISTS and is safe on both. This test does not re-apply the earlier
// migrations, so it does not by itself detect a rewritten 038.
func TestMigration040DropsExtractionTableAndKeepsThePODocumentPath(t *testing.T) {
	database := openTestSchema(t, "vera_po_migration_040")
	ctx := context.Background()

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}

	// 038 stays in the ledger as applied history; 040 is the migration that
	// removes its table. A rewritten 038 would silently change what an existing
	// database is believed to have run.
	var applied int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM `+database.configSchema()+`.schema_migrations
		WHERE version IN (38, 40)`).Scan(&applied); err != nil {
		t.Fatalf("read migration ledger: %v", err)
	}
	if applied != 2 {
		t.Errorf("migration ledger has %d of versions 38 and 40; want both", applied)
	}

	var tableCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = 'purchase_order_extractions'`,
		database.configSchema()).Scan(&tableCount); err != nil {
		t.Fatalf("read extraction table: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("purchase_order_extractions still exists after migration 040")
	}

	// The kept path: a PO with a customer PO document still reads, finalizes
	// and appears in the archive after the table is gone.
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Migration 040")
	user := poTestUser(t, database)
	poCtx := poContext(t, database)
	draft := createFinalizeDraft(t, database, poCtx, user, quotation)
	document := attachTestDocument(t, database, poCtx, draft.ID, user, minimalPDF(nil, ""))

	saved, err := database.FinalizePurchaseOrder(poCtx, finalizeInput(draft, user, "PO-040"))
	if err != nil {
		t.Fatalf("finalize after migration 040: %v", err)
	}
	if saved.State != PurchaseOrderStateSaved {
		t.Errorf("state = %q; want saved", saved.State)
	}

	readBack, err := database.GetPurchaseOrder(poCtx, draft.ID)
	if err != nil {
		t.Fatalf("read back after migration 040: %v", err)
	}
	if readBack.PONumber != "PO-040" {
		t.Errorf("number = %q; want PO-040", readBack.PONumber)
	}
	active, err := database.GetActivePurchaseOrderDocument(poCtx, draft.ID)
	if err != nil {
		t.Fatalf("read document after migration 040: %v", err)
	}
	if active.SHA256 != document.SHA256 {
		t.Errorf("document sha = %q; want the attached %q", active.SHA256, document.SHA256)
	}

	page, err := database.ListPurchaseOrders(poCtx, PurchaseOrderListQuery{State: PurchaseOrderStateSaved})
	if err != nil {
		t.Fatalf("list archive after migration 040: %v", err)
	}
	found := false
	for _, row := range page.Rows {
		if row.ID == draft.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("saved PO %d is missing from the archive page %+v", draft.ID, page.Rows)
	}
}
