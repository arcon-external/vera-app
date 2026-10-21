package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Issue #473: traceable corrections, optimistic concurrency and recoverable
// overwrites. These tests drive the real store against a disposable schema:
// the version every write bumps, the named conflict a stale save receives, the
// immutable snapshot a correction keeps, the reason it requires, the attachment
// reference it retains, and the rule that a deleted record is never recreated.

// savedPurchaseOrder seeds an issued quotation, finalizes one draft and returns
// the saved record. It carries the version the caller loaded, so every later
// write can state the base it expects.
func savedPurchaseOrder(t *testing.T, database *Store, ctx context.Context, user AuthUser, quotation QuotationRecord, number string) PurchaseOrder {
	t.Helper()
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	input := finalizeInput(draft, user, number)
	input.ExpectedVersion = draft.Version
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize %q: %v", number, err)
	}
	return saved
}

// correctionInput builds the payload a correction submits: the saved values,
// with one field the test changes.
func correctionInput(draft PurchaseOrder, user AuthUser, reason string, expectedVersion int64) PurchaseOrderCorrectionInput {
	input := PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: PurchaseOrderUpdate{
			ID:                    draft.ID,
			BillingAddress:        draft.BillingAddress,
			DeliveryAddress:       draft.DeliveryAddress,
			ShipToCode:            draft.ShipToCode,
			ContactEmail:          draft.ContactEmail,
			ContactPhone:          draft.ContactPhone,
			PONumber:              draft.PONumber,
			PODate:                draft.PODate,
			ReceivedDate:          &draft.ReceivedDate,
			RequestedDeliveryDate: draft.RequestedDeliveryDate,
			PaymentTerms:          draft.PaymentTerms,
			Incoterms:             draft.Incoterms,
			UpdatedBy:             user.ID,
			ExpectedVersion:       expectedVersion,
			Items:                 draft.Items,
			DiscountPercent:       draft.DiscountPercent,
			AdjustmentMinor:       draft.AdjustmentMinor,
			AdjustmentReason:      draft.AdjustmentReason,
		},
		Reason: reason,
	}
	return input
}

func TestPurchaseOrderEveryWriteBumpsItsVersionAndAStaleSaveIsNamed(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_version")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Version")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	if draft.Version != 1 {
		t.Fatalf("new draft version = %d; want 1", draft.Version)
	}

	first, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID:              draft.ID,
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
		PONumber:        "PO-A",
		Items:           draft.Items,
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if first.Version != 2 {
		t.Errorf("version after one save = %d; want 2", first.Version)
	}

	// A second editor still holding version 1 must be refused by name, and the
	// stored values must not move.
	stale, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID:              draft.ID,
		UpdatedBy:       user.ID,
		ExpectedVersion: 1,
		PONumber:        "PO-B",
		Items:           draft.Items,
	})
	if !errors.Is(err, ErrPurchaseOrderVersionConflict) {
		t.Fatalf("stale save error = %v; want ErrPurchaseOrderVersionConflict", err)
	}
	var conflict *PurchaseOrderVersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale save error = %T; want *PurchaseOrderVersionConflictError", err)
	}
	if conflict.CurrentVersion != 2 || conflict.CurrentState != PurchaseOrderStateDraft {
		t.Errorf("conflict = %d/%s; want version 2 of a draft",
			conflict.CurrentVersion, conflict.CurrentState)
	}
	if conflict.UpdatedBy != user.ID || conflict.UpdatedByName == "" || conflict.UpdatedAt.IsZero() {
		t.Errorf("conflict actor/time = %d/%q/%v; want the stored writer with a name and a time",
			conflict.UpdatedBy, conflict.UpdatedByName, conflict.UpdatedAt)
	}
	if stale.ID != 0 {
		t.Errorf("a refused save returned a record: %+v", stale)
	}
	stored, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen after refusal: %v", err)
	}
	if stored.PONumber != "PO-A" || stored.Version != 2 {
		t.Errorf("stored after refusal = %q version %d; want PO-A version 2",
			stored.PONumber, stored.Version)
	}
}

func TestPurchaseOrderSaveWithoutItsLoadedVersionIsRefused(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_version_required")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Version required")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	if _, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: draft.Items,
	}); !errors.Is(err, ErrPurchaseOrderVersionRequired) {
		t.Fatalf("save without version = %v; want ErrPurchaseOrderVersionRequired", err)
	}
	finalize := finalizeInput(draft, user, "PO-473")
	finalize.ExpectedVersion = 0
	if _, err := database.FinalizePurchaseOrder(ctx, finalize); !errors.Is(err, ErrPurchaseOrderVersionRequired) {
		t.Fatalf("finalize without version = %v; want ErrPurchaseOrderVersionRequired", err)
	}
}

func TestPurchaseOrderCorrectionKeepsThePreviousValuesAndRequiresAReason(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_correction")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Correction")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-473")
	document := attachTestDocument(t, database, ctx, saved.ID, user, minimalPDF(nil, ""))

	// A blank reason is refused before any write, and keeps the version.
	blank := correctionInput(saved, user, " \t ", saved.Version)
	if _, err := database.CorrectPurchaseOrder(ctx, blank); !errors.Is(err, ErrPurchaseOrderCorrectionReasonRequired) {
		t.Fatalf("blank reason = %v; want ErrPurchaseOrderCorrectionReasonRequired", err)
	}
	if history, err := database.GetPurchaseOrderHistory(ctx, saved.ID); err != nil || len(history) != 0 {
		t.Fatalf("history after refusal = %v, %v; want none", history, err)
	}

	// A real correction keeps the values it replaced, including the attachment
	// version that was current, and moves the record to the next version.
	input := correctionInput(saved, user, "Customer changed the delivery address", saved.Version)
	input.DeliveryAddress = "Jl. New Delivery 9"
	corrected, err := database.CorrectPurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Version != saved.Version+1 {
		t.Errorf("version = %d; want %d", corrected.Version, saved.Version+1)
	}
	if corrected.DeliveryAddress != "Jl. New Delivery 9" {
		t.Errorf("delivery address = %q; want the corrected value", corrected.DeliveryAddress)
	}
	if corrected.State != PurchaseOrderStateSaved || corrected.SavedAt == nil {
		t.Errorf("correction changed the saved identity: %+v", corrected)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history))
	}
	entry := history[0]
	if entry.Kind != PurchaseOrderCorrectionKindCorrection ||
		entry.Reason != "Customer changed the delivery address" ||
		entry.ActorID != user.ID || entry.ActorName == "" || entry.CreatedAt.IsZero() {
		t.Errorf("history entry = %+v; want a named correction with its reason and actor", entry)
	}
	if entry.PreviousVersion != saved.Version || entry.ResultingVersion != saved.Version+1 {
		t.Errorf("history versions = %d -> %d; want %d -> %d",
			entry.PreviousVersion, entry.ResultingVersion, saved.Version, saved.Version+1)
	}
	if entry.State != PurchaseOrderStateSaved {
		t.Errorf("history state = %q; want saved", entry.State)
	}
	if entry.DeliveryAddress != saved.DeliveryAddress {
		t.Errorf("snapshot delivery address = %q; want the replaced %q",
			entry.DeliveryAddress, saved.DeliveryAddress)
	}
	if len(entry.Items) != len(saved.Items) {
		t.Fatalf("snapshot lines = %d; want %d", len(entry.Items), len(saved.Items))
	}
	if entry.Items[0].Description != saved.Items[0].Description ||
		entry.Items[0].UnitPriceMinor != saved.Items[0].UnitPriceMinor {
		t.Errorf("snapshot line 1 = %+v; want the replaced %+v", entry.Items[0], saved.Items[0])
	}
	if entry.Attachment == nil || entry.Attachment.Version != document.Version ||
		entry.Attachment.SHA256 != document.SHA256 {
		t.Errorf("snapshot attachment = %+v; want version %d sha %s",
			entry.Attachment, document.Version, document.SHA256)
	}
}

// TestPurchaseOrderCorrectionKeepsTheFinalizeMinimum is wave 2 of #473: a
// saved record that a correction produces must keep the minimum finalization
// enforces. Finalization refuses an empty record by design; a correction that
// accepted `items: []` would be a second, weaker door into the same state. The
// two paths share one validation function, so this test guards the shared rule,
// not a copy of it. A draft may still be saved incomplete.
func TestPurchaseOrderCorrectionKeepsTheFinalizeMinimum(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_minimum")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Minimum")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-473-MIN")

	// The reported shape: no lines at all, otherwise a valid correction.
	empty := correctionInput(saved, user, "Remove every line", saved.Version)
	empty.Items = nil
	if _, err := database.CorrectPurchaseOrder(ctx, empty); !errors.Is(err, ErrPurchaseOrderIncomplete) {
		t.Fatalf("empty correction = %v; want ErrPurchaseOrderIncomplete", err)
	}

	// A line finalization would refuse is refused here too, so the saved
	// minimum cannot be met by an invalid line.
	invalid := correctionInput(saved, user, "Zero the quantity", saved.Version)
	invalid.Items = append([]PurchaseOrderItem(nil), saved.Items...)
	invalid.Items[0].QuantityScaled = 0
	if _, err := database.CorrectPurchaseOrder(ctx, invalid); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("invalid line correction = %v; want ErrPurchaseOrderInvalid", err)
	}

	// Neither refusal wrote anything: no version bump, no history row, and the
	// stored lines are untouched.
	stored, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reopen after refusals: %v", err)
	}
	if stored.Version != saved.Version || len(stored.Items) != len(saved.Items) ||
		stored.PONumber != saved.PONumber {
		t.Errorf("a refused correction changed the record: %+v", stored)
	}
	if history, err := database.GetPurchaseOrderHistory(ctx, saved.ID); err != nil || len(history) != 0 {
		t.Fatalf("history after refusals = %+v, %v; want none", history, err)
	}

	// The same write on a draft is still allowed: the minimum belongs to the
	// saved state, not to every write. A draft is deliberately unfinished.
	draftQuotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Minimum draft")
	draft := createFinalizeDraft(t, database, ctx, user, draftQuotation)
	updated, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, ExpectedVersion: draft.Version,
	})
	if err != nil {
		t.Fatalf("incomplete draft save = %v; want a draft to remain incomplete", err)
	}
	if len(updated.Items) != 0 {
		t.Errorf("incomplete draft items = %d; want 0", len(updated.Items))
	}
}

func TestPurchaseOrderCorrectionPreservesProtectedIdentity(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_protected")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Protected")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-473-P")
	if _, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, user.ID); err != nil {
		t.Fatalf("assign confirmation: %v", err)
	}
	saved, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reopen assigned record: %v", err)
	}

	input := correctionInput(saved, user, "Fix the customer number", saved.Version)
	input.PONumber = "PO-473-P2"
	corrected, err := database.CorrectPurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}

	if corrected.ID != saved.ID || corrected.UUID != saved.UUID {
		t.Errorf("identity changed: id %d/%s -> %d/%s",
			saved.ID, saved.UUID, corrected.ID, corrected.UUID)
	}
	if corrected.QuotationID != saved.QuotationID || corrected.SourceRevision != saved.SourceRevision {
		t.Errorf("source link changed: %d/%d -> %d/%d", saved.QuotationID, saved.SourceRevision,
			corrected.QuotationID, corrected.SourceRevision)
	}
	if corrected.CustomerID != saved.CustomerID || corrected.Currency != saved.Currency ||
		corrected.TaxZone != saved.TaxZone || corrected.TaxCategory != saved.TaxCategory {
		t.Errorf("customer/currency/tax basis changed: %+v", corrected)
	}
	if corrected.ConfirmationNumber == nil || saved.ConfirmationNumber == nil ||
		*corrected.ConfirmationNumber != *saved.ConfirmationNumber {
		t.Errorf("confirmation number changed: %v -> %v",
			saved.ConfirmationNumber, corrected.ConfirmationNumber)
	}
}

func TestPurchaseOrderCorrectionOfADeletedRecordIsNotFoundAndDoesNotRecreateIt(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_deleted")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Deleted")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-473-D")

	// #480 owns deletion; this simulates its outcome at the store seam. The
	// correction must find nothing rather than insert a replacement row.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"DELETE FROM %s.purchase_orders WHERE id = $1", database.configSchema()), saved.ID); err != nil {
		t.Fatalf("delete purchase order: %v", err)
	}

	input := correctionInput(saved, user, "This row is gone", saved.Version)
	if _, err := database.CorrectPurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Fatalf("correction of a deleted record = %v; want ErrPurchaseOrderNotFound", err)
	}
	if _, err := database.GetPurchaseOrder(ctx, saved.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Fatalf("reopen deleted record = %v; want ErrPurchaseOrderNotFound", err)
	}
	var count int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.purchase_orders", database.configSchema())).Scan(&count); err != nil {
		t.Fatalf("count purchase orders: %v", err)
	}
	if count != 0 {
		t.Errorf("purchase order rows = %d; want 0 (a correction must never recreate one)", count)
	}
	var corrections int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.purchase_order_corrections", database.configSchema())).Scan(&corrections); err != nil {
		t.Fatalf("count corrections: %v", err)
	}
	if corrections != 0 {
		t.Errorf("correction snapshots = %d; want 0", corrections)
	}
}

func TestPurchaseOrderDraftOverwriteKeepsTheReplacedDraftAndItsAttachment(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_draft_overwrite")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Draft overwrite")
	user := poTestUser(t, database)
	other := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	document := attachTestDocument(t, database, ctx, draft.ID, user, minimalPDF(nil, ""))

	// The first editor saves normally: version 2, and a routine save keeps no
	// history.
	first, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID:              draft.ID,
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
		PONumber:        "PO-473-W",
		PaymentTerms:    "30 days",
		Items:           draft.Items,
	})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if history, err := database.GetPurchaseOrderHistory(ctx, draft.ID); err != nil || len(history) != 0 {
		t.Fatalf("routine save wrote history: %v, %v", history, err)
	}

	// The second editor still holds version 1. Even with overwrite intent, the
	// stale base is refused, so the warning can never be bypassed silently.
	stale := PurchaseOrderUpdate{
		ID:              draft.ID,
		UpdatedBy:       other.ID,
		ExpectedVersion: draft.Version,
		Overwrite:       true,
		PONumber:        "PO-473-W2",
		PaymentTerms:    "60 days",
		Items:           draft.Items,
	}
	if _, err := database.UpdatePurchaseOrderDraft(ctx, stale); !errors.Is(err, ErrPurchaseOrderVersionConflict) {
		t.Fatalf("stale overwrite = %v; want ErrPurchaseOrderVersionConflict", err)
	}

	// After the warning, the overwrite states the version it was warned about.
	stale.ExpectedVersion = first.Version
	overwritten, err := database.UpdatePurchaseOrderDraft(ctx, stale)
	if err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if overwritten.Version != first.Version+1 {
		t.Errorf("version = %d; want %d", overwritten.Version, first.Version+1)
	}
	if overwritten.PaymentTerms != "60 days" {
		t.Errorf("payment terms = %q; want the overwriting value", overwritten.PaymentTerms)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, draft.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history))
	}
	entry := history[0]
	if entry.Kind != PurchaseOrderCorrectionKindDraftOverwrite {
		t.Errorf("kind = %q; want %q", entry.Kind, PurchaseOrderCorrectionKindDraftOverwrite)
	}
	if entry.PreviousVersion != first.Version || entry.ResultingVersion != overwritten.Version {
		t.Errorf("versions = %d -> %d; want %d -> %d",
			entry.PreviousVersion, entry.ResultingVersion, first.Version, overwritten.Version)
	}
	if entry.PONumber != "PO-473-W" || entry.PaymentTerms != "30 days" {
		t.Errorf("replaced draft = %q / %q; want PO-473-W / 30 days", entry.PONumber, entry.PaymentTerms)
	}
	if entry.Attachment == nil || entry.Attachment.Version != document.Version ||
		entry.Attachment.Filename != document.OriginalFilename {
		t.Errorf("replaced draft attachment = %+v; want version %d of %q",
			entry.Attachment, document.Version, document.OriginalFilename)
	}
}

func TestPurchaseOrderCorrectionConflictPreservesBothHistories(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_both_histories")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Both histories")
	userA := poTestUser(t, database)
	userB := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := savedPurchaseOrder(t, database, ctx, userA, quotation, "PO-473-H")

	// B corrects first: version 2 -> 3 with a new PO number.
	b := correctionInput(saved, userB, "B corrects the number", saved.Version)
	b.PONumber = "PO-473-H-B"
	if _, err := database.CorrectPurchaseOrder(ctx, b); err != nil {
		t.Fatalf("B correction: %v", err)
	}

	// A still holds the version it loaded. The stale correction is named, not
	// applied.
	a := correctionInput(saved, userA, "A corrects the address", saved.Version)
	a.DeliveryAddress = "Jl. A 1"
	if _, err := database.CorrectPurchaseOrder(ctx, a); !errors.Is(err, ErrPurchaseOrderVersionConflict) {
		t.Fatalf("A stale correction = %v; want ErrPurchaseOrderVersionConflict", err)
	}

	// After the warning, A overwrites the intervening correction. The snapshot
	// it keeps is B's saved state, so neither correction is lost.
	a.ExpectedVersion = saved.Version + 1
	// Overwrite is the intent the UI records; the store still rechecks the
	// version it was given.
	a.Overwrite = true
	final, err := database.CorrectPurchaseOrder(ctx, a)
	if err != nil {
		t.Fatalf("A overwrite: %v", err)
	}
	if final.PONumber != saved.PONumber {
		t.Errorf("final number = %q; want A's working copy %q", final.PONumber, saved.PONumber)
	}
	if final.DeliveryAddress != "Jl. A 1" {
		t.Errorf("final address = %q; want A's correction", final.DeliveryAddress)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history entries = %d; want 2 (B's correction and A's overwrite)", len(history))
	}
	if history[0].PreviousVersion != saved.Version+1 || history[0].ActorID != userA.ID {
		t.Errorf("newest entry = %+v; want A's overwrite of version %d", history[0], saved.Version+1)
	}
	if history[0].PONumber != "PO-473-H-B" || history[0].DeliveryAddress != saved.DeliveryAddress {
		t.Errorf("A's snapshot = %q / %q; want B's saved values",
			history[0].PONumber, history[0].DeliveryAddress)
	}
	if history[1].PreviousVersion != saved.Version || history[1].ActorID != userB.ID ||
		history[1].PONumber != saved.PONumber {
		t.Errorf("older entry = %+v; want B's correction of version %d", history[1], saved.Version)
	}
}

func TestPurchaseOrderHistoryIsOrganizationScoped(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_org")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Org")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-473-O")
	input := correctionInput(saved, user, "Corrected inside the owning organization", saved.Version)
	if _, err := database.CorrectPurchaseOrder(ctx, input); err != nil {
		t.Fatalf("correct: %v", err)
	}

	// A second organization exists as a row only: the store's boundary is the
	// context identity, so its caller must see nothing of the first.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var otherOrgID string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"INSERT INTO %s.organizations (name) VALUES ('Other Org') RETURNING id::text",
		database.configSchema())).Scan(&otherOrgID); err != nil {
		t.Fatalf("seed other organization: %v", err)
	}
	otherCtx := ContextWithOrg(context.Background(), otherOrgID)
	if _, err := database.GetPurchaseOrderHistory(otherCtx, saved.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Fatalf("foreign history = %v; want ErrPurchaseOrderNotFound", err)
	}
	if _, err := database.CorrectPurchaseOrder(otherCtx, input); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Fatalf("foreign correction = %v; want ErrPurchaseOrderNotFound", err)
	}
	stored, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if stored.Version != saved.Version+1 || stored.DeliveryAddress != saved.DeliveryAddress {
		t.Errorf("foreign correction changed the record: %+v", stored)
	}
}

func TestPurchaseOrderCorrectionSnapshotCannotBeRewritten(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_immutable")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Immutable")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	saved := savedPurchaseOrder(t, database, ctx, user, quotation, "PO-473-I")
	input := correctionInput(saved, user, "A reason kept forever", saved.Version)
	input.DeliveryAddress = "Jl. Corrected"
	if _, err := database.CorrectPurchaseOrder(ctx, input); err != nil {
		t.Fatalf("correct: %v", err)
	}
	history, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %v, %v; want one snapshot", history, err)
	}

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	// The trigger is the guard: no writer may rewrite evidence.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.purchase_order_corrections SET reason = 'rewritten' WHERE id = $1",
		database.configSchema()), history[0].ID); err == nil {
		t.Fatal("rewriting a correction snapshot succeeded; want the immutability trigger to refuse")
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.purchase_order_correction_items SET description = 'rewritten' WHERE correction_id = $1",
		database.configSchema()), history[0].ID); err == nil {
		t.Fatal("rewriting a snapshot line succeeded; want the immutability trigger to refuse")
	}
	reloaded, err := database.GetPurchaseOrderHistory(ctx, saved.ID)
	if err != nil || len(reloaded) != 1 || reloaded[0].Reason != "A reason kept forever" ||
		reloaded[0].Items[0].Description != history[0].Items[0].Description {
		t.Errorf("snapshot changed after refused rewrites: %+v (%v)", reloaded, err)
	}
}

func TestPurchaseOrderSaveAfterAnInterveningFinalizeWarnsBeforeTheStateRule(t *testing.T) {
	database := openTestSchema(t, "vera_po_473_finalize_race")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Finalized meanwhile")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	input := finalizeInput(draft, user, "PO-473-F")
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// The draft editor still holds version 1. Its save must warn with who saved
	// and the current version -- the record became saved, which is exactly a
	// change since load -- rather than a bare "not a draft" dead end.
	_, err = database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, ExpectedVersion: draft.Version,
		PONumber: "PO-473-F-EDIT", Items: draft.Items,
	})
	if !errors.Is(err, ErrPurchaseOrderVersionConflict) {
		t.Fatalf("save after finalize = %v; want ErrPurchaseOrderVersionConflict", err)
	}
	var conflict *PurchaseOrderVersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T; want *PurchaseOrderVersionConflictError", err)
	}
	if conflict.CurrentVersion != saved.Version || conflict.CurrentState != PurchaseOrderStateSaved {
		t.Errorf("conflict = version %d state %q; want version %d saved",
			conflict.CurrentVersion, conflict.CurrentState, saved.Version)
	}

	// Overwrite at the current version then meets the state rule: a saved
	// record is corrected, never re-drafted.
	_, err = database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, ExpectedVersion: conflict.CurrentVersion,
		Overwrite: true, PONumber: "PO-473-F-EDIT", Items: draft.Items,
	})
	if !errors.Is(err, ErrPurchaseOrderNotDraft) {
		t.Errorf("current-version save after finalize = %v; want ErrPurchaseOrderNotDraft", err)
	}
	stored, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if stored.State != PurchaseOrderStateSaved || stored.PONumber != "PO-473-F" {
		t.Errorf("a refused save changed the record: %+v", stored)
	}
}
