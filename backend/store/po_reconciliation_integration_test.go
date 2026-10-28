package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func mustMarshalValue(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal reconciliation value: %v", err)
	}
	return raw
}

func strPointer(value string) *string { return &value }

func TestPOReconciliationMigration041Schema(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_schema")
	ctx := context.Background()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var applied int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM `+database.configSchema()+`.schema_migrations
		WHERE version IN (38, 40, 41)`).Scan(&applied); err != nil {
		t.Fatalf("read migration ledger: %v", err)
	}
	if applied != 3 {
		t.Fatalf("migration ledger has %d of versions 38/40/41; want all three", applied)
	}
	for _, table := range []string{
		"purchase_order_reconciliation_sessions",
		"purchase_order_reconciliation_items",
	} {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2`,
			database.config.Schema, table).Scan(&count); err != nil {
			t.Fatalf("read table %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %s is missing after migration 041", table)
		}
	}
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var sql041 string
	for _, migration := range migrations {
		if migration.version == 41 {
			sql041 = migration.sql
		}
	}
	if sql041 == "" {
		t.Fatal("migration 041 is missing from the ledger")
	}
	for _, expected := range []string{
		"purchase_order_reconciliation_sessions",
		"purchase_order_reconciliation_items",
		"purchase_order_id",
		"source_revision_id",
		"generation",
		"purchase_order_reconciliation_active_idx",
		"purchase_order_reconciliation_session_guard",
		"purchase_order_reconciliation_item_guard",
	} {
		if !strings.Contains(sql041, expected) {
			t.Errorf("migration 041 missing %q", expected)
		}
	}
}

func TestPOReconciliationSessionCreatedOnDraftCreation(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_create")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon create")
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
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session after creation: %v", err)
	}
	if session.PurchaseOrderID != draft.ID {
		t.Errorf("session PO = %d; want %d", session.PurchaseOrderID, draft.ID)
	}
	if session.SourceQuotationID != quotation.ID || session.SourceRevision != quotation.RevisionNumber {
		t.Errorf("session source = %d/%d; want %d/%d",
			session.SourceQuotationID, session.SourceRevision, quotation.ID, quotation.RevisionNumber)
	}
	if !session.Active || session.ReadOnly {
		t.Errorf("session active=%v readonly=%v; want active true readonly false", session.Active, session.ReadOnly)
	}
	if session.POVersion != draft.Version {
		t.Errorf("session POVersion = %d; want the draft version %d", session.POVersion, draft.Version)
	}
	if len(session.Items) != 0 {
		t.Errorf("new session has %d items; want none until staff decide", len(session.Items))
	}
	if session.LineIdentities == nil || len(session.LineIdentities) != 0 {
		t.Errorf("new session identities = %+v; want an empty array, never null", session.LineIdentities)
	}

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var otherOrg string
	if err := pool.QueryRow(ctx, `
		INSERT INTO `+database.configSchema()+`.organizations (name)
		VALUES ('Recon Other') RETURNING id::text`).Scan(&otherOrg); err != nil {
		t.Fatalf("seed second organization: %v", err)
	}
	other := ContextWithOrg(context.Background(), otherOrg)
	if _, err := database.GetPOReconciliation(other, draft.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("cross-org read = %v; want ErrPurchaseOrderNotFound", err)
	}
}

func TestPOReconciliationUniqueActiveRule(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_unique")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon unique")
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
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	var revisionID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM `+schema+`.quotation_revisions
		WHERE quotation_id = $1 AND revision_number = $2`,
		quotation.ID, quotation.RevisionNumber).Scan(&revisionID); err != nil {
		t.Fatalf("read source revision id: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO `+schema+`.purchase_order_reconciliation_sessions (
			org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by
		) VALUES (
			(SELECT org_id FROM `+schema+`.purchase_orders WHERE id = $1),
			$1, $2, $3, $4, 1, TRUE, FALSE, 1, '[]', $5
		)`, draft.ID, quotation.ID, quotation.RevisionNumber, revisionID, user.ID)
	if err == nil {
		t.Fatal("second active session for the same PO and generation succeeded; the unique-active rule did not hold")
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO `+schema+`.purchase_order_reconciliation_sessions (
			org_id, purchase_order_id, source_quotation_id, source_revision,
			source_revision_id, generation, active, readonly, po_version,
			line_identities, created_by
		) VALUES (
			(SELECT org_id FROM `+schema+`.purchase_orders WHERE id = $1),
			$1, $2, $3, $4, 2, TRUE, FALSE, 1, '[]', $5
		)`, draft.ID, quotation.ID, quotation.RevisionNumber, revisionID, user.ID)
	if err == nil {
		t.Fatal("second active session for the same PO succeeded; the one-active-session rule did not hold")
	}
}

func TestPOReconciliationLinkCreatesSession(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_link")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	if _, err := database.GetPOReconciliation(ctx, draft.ID); !errors.Is(err, ErrPOReconciliationNotFound) {
		t.Fatalf("never-linked session = %v; want ErrPOReconciliationNotFound (no invented session)", err)
	}
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Link Recon", Subject: "Link recon",
		Address: "Jl. Source", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	saved, err := database.UpdatePurchaseOrderDraft(ctx, linkableDraftUpdate(draft, user, linkReviewSource(t, database, quotation)))
	if err != nil {
		t.Fatalf("save blank draft before link: %v", err)
	}
	source := linkReviewSource(t, database, quotation)
	decisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
		{Key: "header:discountPercent", Choice: PurchaseOrderChoicePO},
	}
	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		saved, quotation, user, decisions, []string{"1", "3"}, []int{1}, ""))
	_ = source
	if err != nil {
		t.Fatalf("link draft: %v", err)
	}
	session, err := database.GetPOReconciliation(ctx, linked.ID)
	if err != nil {
		t.Fatalf("load session after link: %v", err)
	}
	if session.SourceQuotationID != quotation.ID || session.POVersion != linked.Version {
		t.Errorf("linked session = %+v; want source %d at PO version %d",
			session, quotation.ID, linked.Version)
	}
	if session.ReadOnly {
		t.Error("a draft session must not be read-only before Final Save")
	}
}

func TestPOReconciliationSaveAndReopen(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_save")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon save")
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
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	loaded.Items = []POReconciliationItem{{
		Key:         "header:paymentTerms",
		Field:       "header:paymentTerms",
		Kind:        POReconciliationKindDifference,
		Label:       "Payment terms",
		POValue:     mustMarshalValue(t, "30 days"),
		SourceValue: mustMarshalValue(t, "45 days"),
		Status:      POReconciliationStatusUnresolved,
		ActorID:     999999,
		History:     []POReconciliationHistoryEvent{{Kind: "difference", Status: "unresolved", ActorID: 1}},
	}}
	saved, err := database.SavePOReconciliation(ctx, loaded, user.ID)
	if err != nil {
		t.Fatalf("save session: %v", err)
	}
	if len(saved.Items) != 1 || saved.Items[0].Status != POReconciliationStatusUnresolved {
		t.Fatalf("saved items = %+v; want the one unresolved header item", saved.Items)
	}
	if saved.Items[0].ActorID != user.ID {
		t.Errorf("item actor = %d; want the authenticated %d, never the client-supplied 999999",
			saved.Items[0].ActorID, user.ID)
	}
	if len(saved.Items[0].History) != 1 || saved.Items[0].History[0].ActorID != user.ID {
		t.Errorf("history = %+v; want one server-attributed event, not the client replacement",
			saved.Items[0].History)
	}
	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen PO: %v", err)
	}
	if saved.POVersion != reopened.Version {
		t.Errorf("session POVersion = %d; want the bumped PO version %d", saved.POVersion, reopened.Version)
	}
	if reopened.Version != draft.Version+1 {
		t.Errorf("PO version = %d after one session write from %d; want a forward bump",
			reopened.Version, draft.Version)
	}
	rehydrated, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("rehydrate after reopen: %v", err)
	}
	if len(rehydrated.Items) != 1 || rehydrated.Items[0].Key != "header:paymentTerms" {
		t.Fatalf("rehydrated items = %+v; want the saved header item", rehydrated.Items)
	}
	if string(rehydrated.Items[0].POValue) != `"30 days"` {
		t.Errorf("rehydrated PO value = %s; want %q", rehydrated.Items[0].POValue, "30 days")
	}
	rehydrated.Items[0].Status = POReconciliationStatusResolved
	kept := POReconciliationResolutionKeptPO
	rehydrated.Items[0].Resolution = &kept
	resolved, err := database.SavePOReconciliation(ctx, rehydrated, user.ID)
	if err != nil {
		t.Fatalf("resolve item: %v", err)
	}
	if resolved.Items[0].Status != POReconciliationStatusResolved {
		t.Errorf("resolved status = %q; want resolved", resolved.Items[0].Status)
	}
	if len(resolved.Items[0].History) != 2 {
		t.Errorf("history has %d events; want the retained decision plus the new choice",
			len(resolved.Items[0].History))
	}
}

func TestPOReconciliationStaleVersionConflicts(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_stale")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon stale")
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
	first, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	first.Items = []POReconciliationItem{{
		Key: "header:paymentTerms", Field: "header:paymentTerms",
		Kind: POReconciliationKindDifference, Label: "Payment terms",
		POValue: mustMarshalValue(t, "30 days"), SourceValue: mustMarshalValue(t, "45 days"),
		Status: POReconciliationStatusUnresolved,
	}}
	moved, err := database.SavePOReconciliation(ctx, first, user.ID)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	stale := first
	stale.Items = []POReconciliationItem{{
		Key: "header:incoterms", Field: "header:incoterms",
		Kind: POReconciliationKindDifference, Label: "Incoterms",
		POValue: mustMarshalValue(t, "FOB"), SourceValue: mustMarshalValue(t, "CIF"),
		Status: POReconciliationStatusUnresolved,
	}}
	if _, err := database.SavePOReconciliation(ctx, stale, user.ID); !errors.Is(err, ErrPurchaseOrderVersionConflict) {
		t.Fatalf("stale save = %v; want ErrPurchaseOrderVersionConflict instead of an overwrite", err)
	}
	current, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after conflict: %v", err)
	}
	if len(current.Items) != 1 || current.Items[0].Key != "header:paymentTerms" {
		t.Errorf("conflict overwrote the session: %+v; want the first save retained", current.Items)
	}
	if current.POVersion != moved.POVersion {
		t.Errorf("POVersion moved on a refused write: %d vs %d", current.POVersion, moved.POVersion)
	}
}

func TestPOReconciliationImmutability(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_immutable")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon immutable")
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
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	loaded.Items = []POReconciliationItem{{
		Key: "header:paymentTerms", Field: "header:paymentTerms",
		Kind: POReconciliationKindDifference, Label: "Payment terms",
		POValue: mustMarshalValue(t, "45 days"), SourceValue: mustMarshalValue(t, "45 days"),
		Status: POReconciliationStatusUnresolved,
	}}
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("save before finalize: %v", err)
	}
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen before finalize: %v", err)
	}
	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(fresh, user, "PO-RECON-IMM"))
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	frozen, err := database.GetPOReconciliation(ctx, saved.ID)
	if err != nil {
		t.Fatalf("load frozen session: %v", err)
	}
	if !frozen.ReadOnly {
		t.Error("a finalized session must be read-only")
	}
	frozen.Items = []POReconciliationItem{{
		Key: "header:incoterms", Field: "header:incoterms",
		Kind: POReconciliationKindDifference, Label: "Incoterms",
		POValue: mustMarshalValue(t, "FOB"), SourceValue: mustMarshalValue(t, "CIF"),
		Status: POReconciliationStatusUnresolved,
	}}
	if _, err := database.SavePOReconciliation(ctx, frozen, user.ID); !errors.Is(err, ErrPOReconciliationReadOnly) && !errors.Is(err, ErrPurchaseOrderNotDraft) {
		t.Errorf("write to a saved session = %v; want a read-only refusal, not an overwrite", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	_, err = pool.Exec(ctx, `
		UPDATE `+schema+`.purchase_order_reconciliation_sessions
		SET po_version = po_version WHERE purchase_order_id = $1`, saved.ID)
	if err == nil {
		t.Error("direct SQL update of a saved session succeeded; the trigger did not hold")
	} else if !strings.Contains(strings.ToLower(err.Error()), "read-only") &&
		!strings.Contains(strings.ToLower(err.Error()), "immutable") {
		t.Errorf("trigger error = %v; want the read-only/immutable refusal", err)
	}
	_, err = pool.Exec(ctx, `
		UPDATE `+schema+`.purchase_order_reconciliation_items
		SET status = 'resolved', resolution = 'kept_po'
		WHERE purchase_order_id = $1`, saved.ID)
	if err == nil {
		t.Error("direct SQL update of a saved item succeeded; the item trigger did not hold")
	}
	_ = time.Now
	_ = strPointer
}
