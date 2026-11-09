package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Issue #496 Judge round 1 fixes. Every test here is red against 415b74c and
// green only after the corresponding fix: legacy basis saves, never-draft
// deletion, former overwrite history, partial-basis keep and the confirmation
// nullable scan.

func legacyBasis(t *testing.T, database *Store, id int64, taxCategory string) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		UPDATE `+database.configSchema()+`.purchase_orders
		SET tax_type = '', service_category = '', tax_category = $2
		WHERE id = $1`, id, taxCategory); err != nil {
		t.Fatalf("make purchase order %d legacy: %v", id, err)
	}
}

func TestLegacyLinkedDraftSavesWithTheEditorBasis(t *testing.T) {
	database := openTestSchema(t, "vera_po_judge_legacy_linked")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Legacy Linked", "Legacy linked")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	legacyBasis(t, database, draft.ID, "SupplyOnly")
	legacy, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen legacy linked draft after legacy setup: %v", err)
	}

	// The editor derives Goods from the legacy category. A linked row must
	// accept that one-time population rather than reject it as a frozen change.
	update := PurchaseOrderUpdate{
		ID:              legacy.ID,
		Currency:        legacy.Currency,
		TaxZone:         legacy.TaxZone,
		TaxType:         "Goods",
		ServiceCategory: "",
		ReceivedDate:    datePointer(legacy.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: legacy.Version,
		Items:           legacy.Items,
	}
	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("legacy linked draft save = %v; want the legacy basis accepted", err)
	}
	if saved.TaxType != "Goods" || saved.ServiceCategory != "" {
		t.Errorf("legacy linked basis after save = %q/%q; want Goods/empty",
			saved.TaxType, saved.ServiceCategory)
	}
	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen legacy linked draft: %v", err)
	}
	if reopened.TaxType != "Goods" || reopened.PpnBps != saved.PpnBps {
		t.Errorf("reopened legacy linked = %+v; want the populated basis and frozen PPN", reopened)
	}
}

func TestLegacyFormerDraftSavesWithTheEditorBasis(t *testing.T) {
	database := openTestSchema(t, "vera_po_judge_legacy_former")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Legacy Former", "Legacy former")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	legacyBasis(t, database, draft.ID, "SupplyInstall")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeRecord, Reason: "legacy former save",
		TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete legacy linked source: %v", err)
	}
	former, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read legacy former draft: %v", err)
	}
	if former.SourceState != PurchaseOrderSourceStateFormer {
		t.Fatalf("legacy former source state = %q", former.SourceState)
	}

	update := PurchaseOrderUpdate{
		ID:              former.ID,
		Currency:        former.Currency,
		TaxZone:         former.TaxZone,
		TaxType:         "Services",
		ServiceCategory: "NonConstruction",
		ReceivedDate:    datePointer(former.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: former.Version,
		Items:           former.Items,
	}
	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("legacy former draft save = %v; want the legacy basis accepted", err)
	}
	if saved.TaxType != "Services" || saved.ServiceCategory != "NonConstruction" {
		t.Errorf("legacy former basis after save = %q/%q; want Services/NonConstruction",
			saved.TaxType, saved.ServiceCategory)
	}
	// The stored PPN is frozen at creation. Populating the display labels must
	// not recalculate the money on a former legacy row.
	if saved.PpnBps != 0 {
		t.Errorf("legacy former PPN = %d bps after the editor basis save; want the frozen 0", saved.PpnBps)
	}
	if saved.PpnMinor != 0 {
		t.Errorf("legacy former PPN amount = %d; want the frozen 0", saved.PpnMinor)
	}
	if saved.GrandTotalMinor != former.GrandTotalMinor {
		t.Errorf("legacy former grand total = %d; want the unchanged %d",
			saved.GrandTotalMinor, former.GrandTotalMinor)
	}
}

func TestDeletingANeverLinkedDraftPreviewsAndCommits(t *testing.T) {
	database := openTestSchema(t, "vera_po_judge_delete_never")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)
	saved, err := database.UpdatePurchaseOrderDraft(ctx, blankDraftUpdate(draft, draft.Version, user))
	if err != nil {
		t.Fatalf("seed never draft: %v", err)
	}
	admin := deletionUser(t, database, "admin")

	preview, err := database.PreviewDeletion(ctx, DeletionPreviewInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID, ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("deletion preview of a never-linked draft = %v; want a preview", err)
	}
	if !preview.SourceMissing {
		t.Error("never-linked deletion preview is not Source missing")
	}
	if preview.PONumber != saved.PONumber || preview.DocumentNumber != saved.PONumber {
		t.Errorf("preview document = %q / %q; want the customer PO number %q",
			preview.PONumber, preview.DocumentNumber, saved.PONumber)
	}
	if preview.QuoteNumber != "" {
		t.Errorf("preview invented a source quote number %q", preview.QuoteNumber)
	}

	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeRecord, Reason: "never-linked cleanup",
		TypedNumber: saved.PONumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete never-linked draft = %v; want the record removed", err)
	}
	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 0 {
		t.Error("never-linked draft survived its record deletion")
	}
}

func TestFormerOverwriteSnapshotStoresNullIdentities(t *testing.T) {
	database := openTestSchema(t, "vera_po_judge_former_history")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Former History", "Former history")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeRecord, Reason: "former history",
		TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete former source: %v", err)
	}
	former, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read former draft: %v", err)
	}
	update := PurchaseOrderUpdate{
		ID:              former.ID,
		BillingAddress:  "Jl. Former overwrite",
		ReceivedDate:    datePointer(former.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: former.Version,
		Overwrite:       true,
		Items:           former.Items,
	}
	if _, err := database.UpdatePurchaseOrderDraft(ctx, update); err != nil {
		t.Fatalf("overwrite former draft: %v", err)
	}

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var quotationNull, revisionNull, customerNull bool
	var quoteNumber string
	if err := pool.QueryRow(ctx, `
		SELECT quotation_id IS NULL, source_revision IS NULL,
		       customer_id IS NULL, quote_number
		FROM `+database.configSchema()+`.purchase_order_corrections
		WHERE purchase_order_id = $1
		ORDER BY id DESC LIMIT 1`, draft.ID).Scan(
		&quotationNull, &revisionNull, &customerNull, &quoteNumber); err != nil {
		t.Fatalf("read former overwrite snapshot: %v", err)
	}
	if !quotationNull || !revisionNull || !customerNull {
		t.Errorf("former snapshot identities = quotation null %v / revision null %v / customer null %v; want all NULL",
			quotationNull, revisionNull, customerNull)
	}
	if quoteNumber == "" {
		t.Error("former snapshot lost its historical quote-number display copy")
	}
	history, err := database.GetPurchaseOrderHistory(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read former history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("former history = %d entries; want one overwrite", len(history))
	}
	if !history[0].SourceMissing {
		t.Errorf("former overwrite history sourceMissing = false; want true")
	}
	if history[0].QuotationID != 0 || history[0].SourceRevision != 0 || history[0].CustomerID != 0 {
		t.Errorf("former history numbers = %d/%d/%d; want numeric zeros",
			history[0].QuotationID, history[0].SourceRevision, history[0].CustomerID)
	}
}

func TestPartialNeverDraftBasisUpdateKeepsTheOmittedZone(t *testing.T) {
	database := openTestSchema(t, "vera_po_judge_partial_basis")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft, err := database.CreatePurchaseOrderBlankDraft(ctx, PurchaseOrderBlankCreate{
		ReceivedDate: datePointer(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)),
		TaxZone:      "NonFTZ",
		TaxType:      "Goods",
		CreatedBy:    user.ID,
	})
	if err != nil {
		t.Fatalf("create NonFTZ blank draft: %v", err)
	}

	saved, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID:              draft.ID,
		TaxType:         "Services",
		ServiceCategory: "NonConstruction",
		ReceivedDate:    datePointer(draft.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
	})
	if err != nil {
		t.Fatalf("partial basis save = %v", err)
	}
	if saved.TaxZone != "NonFTZ" {
		t.Errorf("partial basis zone = %q; want the omitted zone kept as NonFTZ", saved.TaxZone)
	}
	if saved.TaxType != "Services" || saved.ServiceCategory != "NonConstruction" {
		t.Errorf("partial basis = %q/%q; want Services/NonConstruction",
			saved.TaxType, saved.ServiceCategory)
	}
}

func TestAssigningAConfirmationToANeverDraftRefusesCleanly(t *testing.T) {
	database := openTestSchema(t, "vera_po_judge_confirmation_never")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	if _, err := database.AssignPurchaseOrderConfirmationNumber(ctx, draft.ID, user.ID); !errors.Is(err, ErrPurchaseOrderNotSaved) {
		t.Fatalf("confirmation on a never draft = %v; want ErrPurchaseOrderNotSaved without a NULL scan", err)
	}
}
