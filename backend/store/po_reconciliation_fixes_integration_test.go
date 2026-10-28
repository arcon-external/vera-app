package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// saveHeaderItem is the smallest valid unresolved session write for a draft.
func saveHeaderItem(t *testing.T, database *Store, ctx context.Context, poID, actorID int64, item POReconciliationItem) POReconciliationSession {
	t.Helper()
	loaded, err := database.GetPOReconciliation(ctx, poID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	loaded.Items = []POReconciliationItem{item}
	saved, err := database.SavePOReconciliation(ctx, loaded, actorID)
	if err != nil {
		t.Fatalf("save session item %q: %v", item.Key, err)
	}
	return saved
}

func headerItem(key, label string, poValue, sourceValue any) POReconciliationItem {
	poRaw, _ := json.Marshal(poValue)
	sourceRaw, _ := json.Marshal(sourceValue)
	return POReconciliationItem{
		Key:         key,
		Field:       key,
		Kind:        POReconciliationKindDifference,
		Label:       label,
		POValue:     poRaw,
		SourceValue: sourceRaw,
		Status:      POReconciliationStatusUnresolved,
	}
}

func TestPOReconciliationFinalSaveRefusesUnresolved(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_gate")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon gate")
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
	item := headerItem("header:paymentTerms", "Payment terms", "45 days", "45 days")
	saved := saveHeaderItem(t, database, ctx, draft.ID, user.ID, item)

	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen draft: %v", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, finalizeInput(fresh, user, "PO-GATE-1")); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("finalize with an unresolved item = %v; want ErrPurchaseOrderUnresolvedDifference", err)
	}
	if saved.ID == 0 {
		t.Fatal("the saved session has no id")
	}

	again, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after refusal: %v", err)
	}
	if len(again.Items) != 1 || again.Items[0].Status != POReconciliationStatusUnresolved {
		t.Fatalf("the refused finalize changed the session: %+v", again.Items)
	}
	again.Items[0].Status = POReconciliationStatusResolved
	again.Items[0].Resolution = strPointer(POReconciliationResolutionKeptPO)
	if _, err := database.SavePOReconciliation(ctx, again, user.ID); err != nil {
		t.Fatalf("resolve item: %v", err)
	}
	fresh, err = database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen after resolve: %v", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, finalizeInput(fresh, user, "PO-GATE-2")); err != nil {
		t.Fatalf("finalize with a resolved item: %v", err)
	}
}

func TestPOReconciliationDraftSaveKeepsVersionCurrent(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_draftver")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon draft version")
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
	update := PurchaseOrderUpdate{
		ID:              draft.ID,
		BillingAddress:  draft.BillingAddress,
		DeliveryAddress: draft.DeliveryAddress,
		ContactEmail:    draft.ContactEmail,
		PONumber:        draft.PONumber,
		ReceivedDate:    datePointer(draft.ReceivedDate),
		PaymentTerms:    draft.PaymentTerms,
		Incoterms:       draft.Incoterms,
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
		Items: []PurchaseOrderItem{{
			Description:    "Draft line",
			QuantityScaled: 10000,
			UoM:            "Pc",
			UnitPriceMinor: 100,
		}},
	}
	updated, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("ordinary draft save: %v", err)
	}
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load after draft save: %v", err)
	}
	if loaded.POVersion != updated.Version {
		t.Fatalf("session POVersion = %d; want the current PO version %d", loaded.POVersion, updated.Version)
	}
	loaded.Items = []POReconciliationItem{headerItem("header:incoterms", "Incoterms", "FOB", "CIF")}
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("save after an ordinary draft save: %v", err)
	}
}

func TestPOReconciliationOmittedItemsAreRetained(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_retain")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon retain")
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
	item := headerItem("header:paymentTerms", "Payment terms", "30 days", "45 days")
	item.Status = POReconciliationStatusResolved
	item.Resolution = strPointer(POReconciliationResolutionKeptPO)
	saved := saveHeaderItem(t, database, ctx, draft.ID, user.ID, item)
	if len(saved.Items) != 1 || len(saved.Items[0].History) != 1 {
		t.Fatalf("saved item = %+v; want one item with one event", saved.Items)
	}
	saved.Items = []POReconciliationItem{}
	if _, err := database.SavePOReconciliation(ctx, saved, user.ID); err != nil {
		t.Fatalf("save with an omitted item: %v", err)
	}
	reopened, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen after omission: %v", err)
	}
	if len(reopened.Items) != 1 || reopened.Items[0].Key != "header:paymentTerms" {
		t.Fatalf("omission erased the item: %+v", reopened.Items)
	}
	if len(reopened.Items[0].History) != 1 {
		t.Errorf("history has %d events after an omission; want the retained decision", len(reopened.Items[0].History))
	}
}

func TestPOReconciliationRelinkStartsNewGeneration(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_relink")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	first := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Relink One", Subject: "Relink one",
		Address: "Jl. One", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	saved, err := database.UpdatePurchaseOrderDraft(ctx, linkableDraftUpdate(draft, user, linkReviewSource(t, database, first)))
	if err != nil {
		t.Fatalf("save before first link: %v", err)
	}
	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		saved, first, user,
		[]PurchaseOrderDecision{
			{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
			{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
			{Key: "header:discountPercent", Choice: PurchaseOrderChoicePO},
		}, []string{"1", "3"}, []int{1}, ""))
	if err != nil {
		t.Fatalf("first link: %v", err)
	}
	oldSession, err := database.GetPOReconciliation(ctx, linked.ID)
	if err != nil {
		t.Fatalf("load first session: %v", err)
	}

	// Simulate the source deletion that leaves a formerly linked draft: the
	// PO no longer points at a quotation, but the old session remains.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	if _, err := pool.Exec(ctx, `
		UPDATE `+schema+`.purchase_orders SET quotation_id = NULL WHERE id = $1`, linked.ID); err != nil {
		t.Fatalf("clear source link: %v", err)
	}

	second := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 3, CustomerName: "PT. Relink Two", Subject: "Relink two",
		Address: "Jl. Two", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 210, PaymentTermDays: 30,
	})
	reloaded, err := database.GetPurchaseOrder(ctx, linked.ID)
	if err != nil {
		t.Fatalf("reopen before relink: %v", err)
	}
	relinked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		reloaded, second, user,
		[]PurchaseOrderDecision{
			{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
			{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
			{Key: "header:discountPercent", Choice: PurchaseOrderChoicePO},
		}, []string{"1", "3"}, []int{1}, ""))
	if err != nil {
		t.Fatalf("relink after source loss: %v", err)
	}
	active, err := database.GetPOReconciliation(ctx, relinked.ID)
	if err != nil {
		t.Fatalf("load relinked session: %v", err)
	}
	if active.SourceQuotationID != second.ID {
		t.Fatalf("relinked session source = %d; want the new quotation %d", active.SourceQuotationID, second.ID)
	}
	if active.ID == oldSession.ID {
		t.Fatal("relink reused the old session; want a new generation")
	}
	var oldActive, newActive bool
	var newGeneration int
	if err := pool.QueryRow(ctx, `
		SELECT active FROM `+schema+`.purchase_order_reconciliation_sessions WHERE id = $1`,
		oldSession.ID).Scan(&oldActive); err != nil {
		t.Fatalf("read old session: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT active, generation FROM `+schema+`.purchase_order_reconciliation_sessions WHERE id = $1`,
		active.ID).Scan(&newActive, &newGeneration); err != nil {
		t.Fatalf("read new session: %v", err)
	}
	if oldActive {
		t.Error("the old session stayed active after relink")
	}
	if !newActive || newGeneration < 2 {
		t.Errorf("new session active=%v generation=%d; want active generation 2", newActive, newGeneration)
	}
}

func TestPOReconciliationRejectsForeignSessionID(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_sessionid")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon session id")
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
	loaded.ID++
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("save with a foreign session id = %v; want ErrPurchaseOrderInvalid", err)
	}
}

func TestPOReconciliationRejectsMalformedHeaderValues(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_values")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon values")
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
	cases := map[string]string{
		"bool":           `true`,
		"text":           `"ten"`,
		"object":         `{"unexpected":1}`,
		"over-range":     `101`,
		"negative":       `-1`,
		"fraction":       `1.5`,
		"unsafe-integer": `9007199254740993`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "header:discountPercent", Field: "header:discountPercent",
				Kind: POReconciliationKindDifference, Label: "Whole-PO discount",
				POValue: []byte(raw), SourceValue: []byte(`0`),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err == nil {
				t.Fatalf("save with %s value %s succeeded; want a refusal", name, raw)
			}
		})
	}
}

func TestPOReconciliationRepeatedSaveIsIdempotent(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_idempotent")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon idempotent")
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
	item := headerItem("header:paymentTerms", "Payment terms", "30 days", "45 days")
	item.Status = POReconciliationStatusResolved
	item.Resolution = strPointer(POReconciliationResolutionKeptPO)
	first := saveHeaderItem(t, database, ctx, draft.ID, user.ID, item)
	if len(first.Items[0].History) != 1 {
		t.Fatalf("history = %d; want one event", len(first.Items[0].History))
	}
	second, err := database.SavePOReconciliation(ctx, first, user.ID)
	if err != nil {
		t.Fatalf("repeat the same save: %v", err)
	}
	if len(second.Items[0].History) != 1 {
		t.Errorf("history = %d after an unchanged save; want it idempotent", len(second.Items[0].History))
	}
}

func TestPOReconciliationDatabaseGuards(t *testing.T) {
	database := openTestSchema(t, "vera_po_recon_guards")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Recon guards")
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
	item := headerItem("header:paymentTerms", "Payment terms", "45 days", "45 days")
	item.Status = POReconciliationStatusResolved
	item.Resolution = strPointer(POReconciliationResolutionKeptPO)
	session := saveHeaderItem(t, database, ctx, draft.ID, user.ID, item)
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(fresh, user, "PO-GUARD-1"))
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	var orgID string
	if err := pool.QueryRow(ctx, `
		SELECT org_id::text FROM `+schema+`.purchase_orders WHERE id = $1`, saved.ID).Scan(&orgID); err != nil {
		t.Fatalf("read purchase order organisation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO `+schema+`.purchase_order_reconciliation_items
			(org_id, session_id, purchase_order_id, key, field, kind, label, status, actor_id)
		VALUES ($1, $2, $3, 'header:incoterms', 'header:incoterms', 'difference', 'Incoterms', 'unresolved', $4)`,
		orgID, session.ID, saved.ID, user.ID); err == nil {
		t.Error("inserting an item into a saved session succeeded; the INSERT guard did not hold")
	}

	otherQuotation := seedFinalizeQuotation(t, database, 9, "PT. Other", "Recon guard other")
	var otherRevision int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM `+schema+`.quotation_revisions
		WHERE quotation_id = $1 AND revision_number = $2`,
		otherQuotation.ID, otherQuotation.RevisionNumber).Scan(&otherRevision); err != nil {
		t.Fatalf("read other revision: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE `+schema+`.purchase_order_reconciliation_sessions
		SET source_revision_id = $1
		WHERE id = $2`, otherRevision, session.ID); err == nil {
		t.Error("retargeting a saved session's source revision succeeded; the guard did not hold")
	}

	if _, err := pool.Exec(ctx, `
		UPDATE `+schema+`.purchase_order_reconciliation_sessions SET active = FALSE WHERE id = $1`,
		session.ID); err != nil {
		t.Errorf("deactivating a saved session failed: %v; a repair needs this transition", err)
	}

	var otherOrg string
	if err := pool.QueryRow(ctx, `
		INSERT INTO `+schema+`.organizations (name) VALUES ('Recon Guard Other') RETURNING id::text`).Scan(&otherOrg); err != nil {
		t.Fatalf("seed second organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO `+schema+`.purchase_order_reconciliation_sessions
			(org_id, purchase_order_id, source_quotation_id, source_revision,
			 source_revision_id, generation, active, readonly, po_version, line_identities, created_by)
		VALUES ($1, $2, 999999, 99, NULL, 7, FALSE, TRUE, 1, '[]', $3)`,
		otherOrg, saved.ID, user.ID); err == nil {
		t.Error("a cross-organisation session with an invented source pair was accepted; the composite FK did not hold")
	}
}
