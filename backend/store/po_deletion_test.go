package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Issue #480: permanent deletion of one record or its document chain.
//
// These tests drive the real store against a disposable schema. They assert the
// externally observable outcome: what is gone, what survives, what the audit
// says, which counters moved, and which refusals stop an unconfirmed cascade.

func deletionUser(t *testing.T, database *Store, role string) AuthUser {
	t.Helper()
	user, err := database.CreateUser(context.Background(),
		fmt.Sprintf("delete-%s-%d@example.com", role, time.Now().UnixNano()),
		"Deletion "+role, "hash", role)
	if err != nil {
		t.Fatalf("create %s user: %v", role, err)
	}
	return user
}

// savedDeletionPO seeds one issued revision, creates its draft and finalizes it
// under a unique customer number. It returns the quotation and the saved record.
func savedDeletionPO(t *testing.T, database *Store, customerName, number string) (QuotationRecord, PurchaseOrder, AuthUser, context.Context) {
	t.Helper()
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, customerName, "Deletion subject")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	saved, err := database.FinalizePurchaseOrder(ctx, finalizeInput(draft, user, number))
	if err != nil {
		t.Fatalf("finalize deletion fixture: %v", err)
	}
	return quotation, saved, user, ctx
}

func countRows(t *testing.T, database *Store, table, where string, args ...any) int {
	t.Helper()
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", database.configSchema(), table)
	if where != "" {
		query += " WHERE " + where
	}
	if err := database.pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func counterValues(t *testing.T, database *Store) map[string]int64 {
	t.Helper()
	rows, err := database.pool.Query(context.Background(), fmt.Sprintf(
		"SELECT name, next_value FROM %s.sequence_counters", database.configSchema()))
	if err != nil {
		t.Fatalf("read sequence counters: %v", err)
	}
	defer rows.Close()
	values := map[string]int64{}
	for rows.Next() {
		var name string
		var value int64
		if err := rows.Scan(&name, &value); err != nil {
			t.Fatalf("scan sequence counter: %v", err)
		}
		values[name] = value
	}
	return values
}

func assertCountersUnchanged(t *testing.T, before, after map[string]int64) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("counter rows changed: %v -> %v", before, after)
	}
	for name, value := range before {
		if after[name] != value {
			t.Errorf("counter %q moved %d -> %d; deletion must never rewind a counter", name, value, after[name])
		}
	}
}

func previewFor(t *testing.T, database *Store, actor AuthUser, targetType string, targetID int64) DeletionPreview {
	t.Helper()
	preview, err := database.PreviewDeletion(poContext(t, database), DeletionPreviewInput{
		TargetType: targetType,
		TargetID:   targetID,
		ActorID:    actor.ID,
	})
	if err != nil {
		t.Fatalf("preview %s %d: %v", targetType, targetID, err)
	}
	return preview
}

func recordScope(t *testing.T, preview DeletionPreview) DeletionScopePlan {
	t.Helper()
	for _, plan := range preview.Scopes {
		if plan.Scope == DeletionScopeRecord {
			return plan
		}
	}
	t.Fatalf("preview has no record scope: %+v", preview.Scopes)
	return DeletionScopePlan{}
}

func chainScope(t *testing.T, preview DeletionPreview) DeletionScopePlan {
	t.Helper()
	for _, plan := range preview.Scopes {
		if plan.Scope == DeletionScopeChain {
			return plan
		}
	}
	t.Fatalf("preview has no chain scope: %+v", preview.Scopes)
	return DeletionScopePlan{}
}

func TestDeletionIsAdminOnly(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_admin")
	quotation, _, _, _ := savedDeletionPO(t, database, "PT. Admin Gate", "PO-DEL-ADMIN")
	staff := deletionUser(t, database, "staff")
	ctx := poContext(t, database)

	if _, err := database.PreviewDeletion(ctx, DeletionPreviewInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID, ActorID: staff.ID,
	}); !errors.Is(err, ErrDeletionForbidden) {
		t.Fatalf("staff preview error = %v; want ErrDeletionForbidden", err)
	}
	if _, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ActorID: staff.ID,
	}); !errors.Is(err, ErrDeletionForbidden) {
		t.Fatalf("staff challenge error = %v; want ErrDeletionForbidden", err)
	}
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeRecord, Reason: "cleanup", TypedNumber: quotation.QuoteNumber,
		ActorID: staff.ID,
	}); !errors.Is(err, ErrDeletionForbidden) {
		t.Fatalf("staff delete error = %v; want ErrDeletionForbidden", err)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 {
		t.Fatal("staff deletion removed the quotation")
	}
}

func TestDeletePurchaseOrderRecordKeepsTheQuotationAndTheCounter(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_record")
	quotation, saved, user, ctx := savedDeletionPO(t, database, "PT. Record", "PO-480-RECORD")
	admin := deletionUser(t, database, "admin")

	assigned, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, user.ID)
	if err != nil {
		t.Fatalf("assign confirmation number: %v", err)
	}
	if assigned.ConfirmationNumber == nil {
		t.Fatal("confirmation number was not assigned")
	}
	counterBefore := counterValues(t, database)

	preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	if preview.DocumentNumber != "PO-480-RECORD" {
		t.Errorf("preview document number = %q; want the customer PO number", preview.DocumentNumber)
	}
	plan := recordScope(t, preview)
	if !plan.Available || plan.RequiresChallenge {
		t.Errorf("record plan = %+v; want available without a challenge", plan)
	}
	if len(plan.Removes) != 1 || plan.Removes[0].Type != DeletionTargetPurchaseOrder {
		t.Errorf("record removes = %+v; want the purchase order", plan.Removes)
	}

	result, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetPurchaseOrder,
		TargetID:            saved.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "  duplicate entry  ",
		TypedNumber:         "po-480-record",
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             admin.ID,
	})
	if err != nil {
		t.Fatalf("delete purchase order record: %v", err)
	}
	if len(result.Removed) != 1 || result.Removed[0].ID != saved.ID {
		t.Errorf("removed = %+v; want the purchase order", result.Removed)
	}
	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 0 {
		t.Fatal("purchase order row survived record deletion")
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 {
		t.Fatal("record deletion removed the quotation")
	}
	if countRows(t, database, "purchase_order_items", "purchase_order_id = $1", saved.ID) != 0 {
		t.Fatal("record deletion left the purchase order lines")
	}
	assertCountersUnchanged(t, counterBefore, counterValues(t, database))

	// The number is released: a new PO for the same quotation and customer may
	// use the same customer number again. There is no replacement link.
	replacementDraft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID: quotation.ID, RevisionNumber: quotation.RevisionNumber, CreatedBy: user.ID,
	})
	if err != nil {
		t.Fatalf("create replacement draft: %v", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, finalizeInput(replacementDraft, user, "PO-480-RECORD")); err != nil {
		t.Fatalf("reuse the released customer number: %v", err)
	}

	audits, err := database.ListDeletionAudits(ctx, 10)
	if err != nil {
		t.Fatalf("list deletion audits: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("audits = %+v; want one minimal audit row", audits)
	}
	audit := audits[0]
	if audit.TargetType != DeletionTargetPurchaseOrder || audit.TargetID != saved.ID {
		t.Errorf("audit target = %s/%d; want purchase_order/%d", audit.TargetType, audit.TargetID, saved.ID)
	}
	if audit.DocumentNumber != "PO-480-RECORD" {
		t.Errorf("audit document number = %q", audit.DocumentNumber)
	}
	if audit.SourceReference != quotation.QuoteNumber {
		t.Errorf("audit source = %q; want %q", audit.SourceReference, quotation.QuoteNumber)
	}
	if audit.Reason != "duplicate entry" {
		t.Errorf("audit reason = %q; want the trimmed reason", audit.Reason)
	}
	if audit.Actor != admin.ID {
		t.Errorf("audit actor = %d; want %d", audit.Actor, admin.ID)
	}
	if audit.Scope != DeletionScopeRecord {
		t.Errorf("audit scope = %q; want record", audit.Scope)
	}
}

func TestDeleteQuotationRecordKeepsItsPOAsSourceMissingAndReadable(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_source_missing")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Survives", "PO-480-SURVIVES")
	admin := deletionUser(t, database, "admin")

	// A document and a correction must be removed with the quotation chain;
	// the PO's own evidence stays because the PO survives.
	if _, err := database.AttachPurchaseOrderDocument(ctx, PurchaseOrderDocumentUpload{
		PurchaseOrderID: saved.ID,
		Filename:        "customer-po.pdf",
		Data:            minimalPDF(nil, ""),
		UploadedBy:      admin.ID,
	}); err != nil {
		t.Fatalf("attach purchase order document: %v", err)
	}

	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	plan := recordScope(t, preview)
	if !plan.Available {
		t.Fatalf("record plan unavailable: %+v", plan)
	}
	if len(plan.Preserves) != 1 || plan.Preserves[0].ID != saved.ID {
		t.Fatalf("record preserves = %+v; want the surviving purchase order", plan.Preserves)
	}
	if chainScope(t, preview).Scope != DeletionScopeChain {
		t.Fatal("quotation preview has no chain scope")
	}

	beforeVersion := saved.Version
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetQuotation,
		TargetID:            quotation.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "customer cancelled the quotation",
		TypedNumber:         quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             admin.ID,
	}); err != nil {
		t.Fatalf("delete quotation record: %v", err)
	}

	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 0 {
		t.Fatal("quotation row survived record deletion")
	}
	if countRows(t, database, "quotation_revisions", "quotation_id = $1", quotation.ID) != 0 {
		t.Fatal("quotation revisions survived record deletion")
	}

	surviving, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("read the surviving purchase order: %v", err)
	}
	if !surviving.SourceMissing {
		t.Error("surviving purchase order is not marked source missing")
	}
	if surviving.QuotationID != quotation.ID {
		t.Errorf("former quotation id = %d; want %d", surviving.QuotationID, quotation.ID)
	}
	if surviving.QuoteNumber != quotation.QuoteNumber {
		t.Errorf("former quote number = %q; want %q", surviving.QuoteNumber, quotation.QuoteNumber)
	}
	if surviving.SourceRevision != quotation.RevisionNumber {
		t.Errorf("source revision = %d; want the former revision %d", surviving.SourceRevision, quotation.RevisionNumber)
	}
	if surviving.PONumber != "PO-480-SURVIVES" || surviving.CustomerName != "PT. Survives" {
		t.Errorf("surviving readable values drifted: %+v", surviving)
	}
	if surviving.GrandTotalMinor != saved.GrandTotalMinor || len(surviving.Items) != len(saved.Items) {
		t.Errorf("surviving economics drifted: %+v", surviving)
	}
	if surviving.Version <= beforeVersion {
		t.Errorf("version = %d; want it forward from %d", surviving.Version, beforeVersion)
	}
	if state, err := database.GetPurchaseOrderDocumentState(ctx, saved.ID); err != nil {
		t.Fatalf("document state after source deletion: %v", err)
	} else if state.Active == nil {
		t.Error("the surviving purchase order lost its own customer PDF")
	}

	// The archive sees it as source missing, and the filter contract (#469)
	// now has a row that can match.
	only, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		SourceMissing: PurchaseOrderSourceMissingOnly, Page: 1,
	})
	if err != nil {
		t.Fatalf("list source-missing purchase orders: %v", err)
	}
	if len(only.Rows) != 1 || only.Rows[0].ID != saved.ID || !only.Rows[0].SourceMissing {
		t.Errorf("sourceMissing=only = %+v; want the surviving record", only.Rows)
	}
	if only.Rows[0].QuoteNumber != quotation.QuoteNumber {
		t.Errorf("list quote number = %q; want the former number", only.Rows[0].QuoteNumber)
	}

	// The quotation is gone, so a chain deletion is no longer offered for the
	// surviving record.
	poPreview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	if plan := chainScope(t, poPreview); plan.Available {
		t.Errorf("chain scope = %+v; want unavailable when the source is gone", plan)
	}
	if poPreview.DocumentNumber != "PO-480-SURVIVES" {
		t.Errorf("source-missing preview number = %q", poPreview.DocumentNumber)
	}

	if n := countRows(t, database, "deletion_audits", "target_type = $1 AND target_id = $2",
		DeletionTargetQuotation, quotation.ID); n != 1 {
		t.Errorf("quotation audits = %d; want 1", n)
	}
}

// deleteQuotationRecordWithFreshPreview removes only the quotation, leaving the
// connected Purchase Order source-missing. It is the setup both regressions
// below need.
func deleteQuotationRecordWithFreshPreview(t *testing.T, database *Store, admin AuthUser, ctx context.Context, quotation QuotationRecord) {
	t.Helper()
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetQuotation,
		TargetID:            quotation.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "source removed to exercise the survivor",
		TypedNumber:         quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             admin.ID,
	}); err != nil {
		t.Fatalf("delete the quotation record: %v", err)
	}
}

// assertDeletionChange names one canonical fingerprint key and its new value.
func assertDeletionChange(t *testing.T, changes []DeletionChange, field, after string) {
	t.Helper()
	for _, change := range changes {
		if change.Field != field {
			continue
		}
		if change.After != after {
			t.Errorf("change %q after = %q; want %q", field, change.After, after)
		}
		return
	}
	t.Errorf("changes = %+v; want %q named", changes, field)
}

// TestSourceMissingDeletionFingerprintCoversADocumentUploadedAfterPreview is the
// Judge's #480 reproduction: the quotation is deleted first, the surviving
// Purchase Order is previewed, and a customer PDF arrives before the
// confirmation. The old fingerprint omitted the record's own attachment count
// whenever the source was missing, so confirming it deleted the new file too.
func TestSourceMissingDeletionFingerprintCoversADocumentUploadedAfterPreview(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_source_missing_document")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Late PDF", "PO-480-LATE-PDF")
	admin := deletionUser(t, database, "admin")
	deleteQuotationRecordWithFreshPreview(t, database, admin, ctx, quotation)

	preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	if _, err := database.AttachPurchaseOrderDocument(ctx, PurchaseOrderDocumentUpload{
		PurchaseOrderID: saved.ID,
		Filename:        "arrived-after-preview.pdf",
		Data:            minimalPDF(nil, ""),
		UploadedBy:      admin.ID,
	}); err != nil {
		t.Fatalf("attach a document after the preview: %v", err)
	}

	fresh := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	if fresh.Fingerprint == preview.Fingerprint {
		t.Fatal("a document uploaded after the preview left the fingerprint unchanged")
	}

	_, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetPurchaseOrder,
		TargetID:            saved.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "delete the record",
		TypedNumber:         saved.PONumber,
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             admin.ID,
	})
	var changed *DeletionChainChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("stale confirmation error = %v; want DeletionChainChangedError", err)
	}
	assertDeletionChange(t, changed.Changes,
		fmt.Sprintf("purchase_order_%d_documents", saved.ID), "1")

	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 1 {
		t.Fatal("a stale confirmation deleted the purchase order")
	}
	if countRows(t, database, "purchase_order_documents", "purchase_order_id = $1", saved.ID) != 1 {
		t.Fatal("a stale confirmation deleted the document uploaded after the preview")
	}
	if n := countRows(t, database, "deletion_audits",
		"target_type = $1 AND target_id = $2", DeletionTargetPurchaseOrder, saved.ID); n != 0 {
		t.Fatalf("audits = %d; want 0 after a refused confirmation", n)
	}

	// The fresh confirmation covers the file the admin has now been shown and
	// removes it with the record.
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetPurchaseOrder,
		TargetID:            saved.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "delete the record",
		TypedNumber:         saved.PONumber,
		ExpectedFingerprint: fresh.Fingerprint,
		ActorID:             admin.ID,
	}); err != nil {
		t.Fatalf("delete with the fresh fingerprint: %v", err)
	}
	if countRows(t, database, "purchase_order_documents", "purchase_order_id = $1", saved.ID) != 0 {
		t.Fatal("the confirmed deletion left the purchase order document behind")
	}
}

// TestSourceMissingDeletionFingerprintCoversHistoryAddedAfterPreview is the same
// hole for the history count: a correction snapshot that arrives after the
// preview must invalidate the confirmation, not be deleted unconfirmed.
func TestSourceMissingDeletionFingerprintCoversHistoryAddedAfterPreview(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_source_missing_history")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Late History", "PO-480-LATE-HISTORY")
	admin := deletionUser(t, database, "admin")
	deleteQuotationRecordWithFreshPreview(t, database, admin, ctx, quotation)

	preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	insertDeletionHistoryRow(t, database, saved.ID)

	fresh := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	if fresh.Fingerprint == preview.Fingerprint {
		t.Fatal("a history row added after the preview left the fingerprint unchanged")
	}

	_, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetPurchaseOrder,
		TargetID:            saved.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "delete the record",
		TypedNumber:         saved.PONumber,
		ExpectedFingerprint: preview.Fingerprint,
		ActorID:             admin.ID,
	})
	var changed *DeletionChainChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("stale confirmation error = %v; want DeletionChainChangedError", err)
	}
	assertDeletionChange(t, changed.Changes,
		fmt.Sprintf("purchase_order_%d_history", saved.ID), "1")

	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 1 {
		t.Fatal("a stale confirmation deleted the purchase order")
	}
	if countRows(t, database, "purchase_order_corrections", "purchase_order_id = $1", saved.ID) != 1 {
		t.Fatal("a stale confirmation deleted the history row added after the preview")
	}
	if n := countRows(t, database, "deletion_audits",
		"target_type = $1 AND target_id = $2", DeletionTargetPurchaseOrder, saved.ID); n != 0 {
		t.Fatalf("audits = %d; want 0 after a refused confirmation", n)
	}

	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType:          DeletionTargetPurchaseOrder,
		TargetID:            saved.ID,
		Scope:               DeletionScopeRecord,
		Reason:              "delete the record",
		TypedNumber:         saved.PONumber,
		ExpectedFingerprint: fresh.Fingerprint,
		ActorID:             admin.ID,
	}); err != nil {
		t.Fatalf("delete with the fresh fingerprint: %v", err)
	}
	if countRows(t, database, "purchase_order_corrections", "purchase_order_id = $1", saved.ID) != 0 {
		t.Fatal("the confirmed deletion left the history row behind")
	}
}

// insertDeletionHistoryRow appends one correction snapshot directly. While a
// record is source missing the store refuses every write that needs the frozen
// source revision (#481 reopens that surface), so the row is arranged at the
// table level: the fingerprint guard must cover the count however the row
// arrives.
func insertDeletionHistoryRow(t *testing.T, database *Store, id int64) {
	t.Helper()
	schema := database.configSchema()
	if _, err := database.pool.Exec(context.Background(), fmt.Sprintf(`
		INSERT INTO %s.purchase_order_corrections (
			org_id, purchase_order_id, previous_version, resulting_version,
			kind, state, reason, actor, quotation_id, source_revision,
			quote_number, customer_id, customer_name, currency, tax_zone,
			tax_category, received_date
		)
		SELECT po.org_id, po.id, po.version, po.version + 1,
		       'correction', po.state, 'row added after the preview',
		       po.updated_by, COALESCE(po.quotation_id, po.former_quotation_id, 0),
		       po.source_revision, COALESCE(q.quote_number, po.former_quote_number, ''),
		       po.customer_id, po.customer_name, po.currency, po.tax_zone,
		       po.tax_category, po.received_date
		FROM %s.purchase_orders po
		LEFT JOIN %s.quotations q ON q.id = po.quotation_id
		WHERE po.id = $1`, schema, schema, schema), id); err != nil {
		t.Fatalf("insert a history row after the preview: %v", err)
	}
}

// TestDeletionRefusesAQuotationFollowedByAnotherOrganization covers the
// residual the Judge recorded: the quotation is invisible to this
// organization's chain, so the foreign key used to abort the deletion as a
// generic failure after rollback. The refusal is now named before any write.
func TestDeletionRefusesAQuotationFollowedByAnotherOrganization(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_cross_org")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Cross Tenant", "Cross tenant")
	admin := deletionUser(t, database, "admin")
	schema := database.configSchema()

	var otherOrg string
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`INSERT INTO %s.organizations (name) VALUES ('PT. Other Tenant') RETURNING id::text`,
		schema)).Scan(&otherOrg); err != nil {
		t.Fatalf("insert a second organization: %v", err)
	}
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.purchase_orders (
			org_id, quotation_id, source_revision, customer_id, customer_name,
			received_date, currency, tax_zone, created_by, updated_by
		)
		SELECT $1::uuid, q.id, 0, q.customer_id, c.legal_name, CURRENT_DATE,
		       'IDR', 'FTZ', $3, $3
		FROM %s.quotations q
		JOIN %s.customers c ON c.id = q.customer_id
		WHERE q.id = $2`, schema, schema, schema), otherOrg, quotation.ID, user.ID); err != nil {
		t.Fatalf("insert a purchase order in the other organization: %v", err)
	}

	if _, err := database.PreviewDeletion(ctx, DeletionPreviewInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID, ActorID: admin.ID,
	}); !errors.Is(err, ErrDeletionCrossOrgLinked) {
		t.Fatalf("cross-organization preview error = %v; want ErrDeletionCrossOrgLinked", err)
	}
	if _, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: "v1", ActorID: admin.ID,
	}); !errors.Is(err, ErrDeletionCrossOrgLinked) {
		t.Fatalf("cross-organization challenge error = %v; want ErrDeletionCrossOrgLinked", err)
	}
	for _, scope := range []string{DeletionScopeRecord, DeletionScopeChain} {
		if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: scope, Reason: "cross-tenant attempt",
			TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: "v1",
			ActorID: admin.ID,
		}); !errors.Is(err, ErrDeletionCrossOrgLinked) {
			t.Fatalf("%s confirmation error = %v; want ErrDeletionCrossOrgLinked", scope, err)
		}
	}

	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 {
		t.Fatal("a refused cross-organization deletion removed the quotation")
	}
	if n := countRows(t, database, "deletion_audits", ""); n != 0 {
		t.Fatalf("audits = %d; want 0 after a refused cross-organization deletion", n)
	}
}

func TestDeletionRequiresAReasonAndTheTypedNumber(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_confirm")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Confirm", "PO-480-CONFIRM")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)

	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeRecord, Reason: "   ", TypedNumber: "PO-480-CONFIRM",
		ExpectedFingerprint: preview.Fingerprint, ActorID: admin.ID,
	}); !errors.Is(err, ErrDeletionReasonRequired) {
		t.Fatalf("blank reason error = %v; want ErrDeletionReasonRequired", err)
	}
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeRecord, Reason: "cleanup", TypedNumber: "PO-OTHER",
		ExpectedFingerprint: preview.Fingerprint, ActorID: admin.ID,
	}); !errors.Is(err, ErrDeletionTypedNumberMismatch) {
		t.Fatalf("wrong typed number error = %v; want ErrDeletionTypedNumberMismatch", err)
	}
	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 1 {
		t.Fatal("a refused confirmation deleted the record")
	}
	// Nothing was audited and the quotation is untouched: a refusal is not a
	// partial deletion.
	if n := countRows(t, database, "deletion_audits", ""); n != 0 {
		t.Errorf("audits = %d; want 0 after refusals", n)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 {
		t.Fatal("a refused confirmation removed the quotation")
	}
}

func TestDeletionStopsWhenTheRecordMovedSinceThePreview(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_changed_record")
	quotation, saved, user, ctx := savedDeletionPO(t, database, "PT. Moved", "PO-480-MOVED")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	originalFingerprint := preview.Fingerprint

	// An ordinary correction moves the record's version, which is exactly the
	// change the confirmation is bound to.
	correction := finalizeInput(saved, user, saved.PONumber)
	if _, err := database.CorrectPurchaseOrder(ctx, PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: correction.PurchaseOrderUpdate,
		Reason:              "fix a typo",
	}); err != nil {
		t.Fatalf("correct before deletion: %v", err)
	}

	_, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeRecord, Reason: "cleanup", TypedNumber: "PO-480-MOVED",
		ExpectedFingerprint: originalFingerprint, ActorID: admin.ID,
	})
	var changed *DeletionChainChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("stale confirmation error = %v; want DeletionChainChangedError", err)
	}
	if len(changed.Changes) == 0 {
		t.Fatal("stale confirmation reported no changes")
	}
	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 1 {
		t.Fatal("a stale confirmation deleted the moved record")
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 {
		t.Fatal("a stale confirmation removed the quotation")
	}
}

func TestDeletionChallengeLettersAvoidLookalikes(t *testing.T) {
	seen := map[rune]bool{}
	for attempt := 0; attempt < 200; attempt++ {
		letters, err := newDeletionLetters()
		if err != nil {
			t.Fatalf("generate challenge letters: %v", err)
		}
		if len(letters) != DeletionChallengeLength {
			t.Fatalf("letters = %q; want %d characters", letters, DeletionChallengeLength)
		}
		for _, letter := range letters {
			if letter < 'A' || letter > 'Z' {
				t.Fatalf("letters = %q; want uppercase A-Z", letters)
			}
			if strings.ContainsRune("ILO", letter) {
				t.Fatalf("letters = %q contain a lookalike %q", letters, letter)
			}
			seen[letter] = true
		}
	}
	if len(seen) != len(deletionChallengeAlphabet) {
		t.Errorf("observed %d distinct letters; want all %d of the alphabet", len(seen), len(deletionChallengeAlphabet))
	}
}

func TestDeleteQuotationChainNeedsTheChallengeAndRemovesEverything(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_chain")
	quotation, saved, user, ctx := savedDeletionPO(t, database, "PT. Chain", "PO-480-CHAIN")
	admin := deletionUser(t, database, "admin")
	if _, err := database.AssignPurchaseOrderConfirmationNumber(ctx, saved.ID, user.ID); err != nil {
		t.Fatalf("assign confirmation number: %v", err)
	}
	if _, err := database.AttachPurchaseOrderDocument(ctx, PurchaseOrderDocumentUpload{
		PurchaseOrderID: saved.ID, Filename: "po.pdf", Data: minimalPDF(nil, ""), UploadedBy: admin.ID,
	}); err != nil {
		t.Fatalf("attach document: %v", err)
	}

	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	plan := chainScope(t, preview)
	if !plan.Available || !plan.RequiresChallenge {
		t.Fatalf("chain plan = %+v; want available and challenge-bound", plan)
	}
	if len(plan.Removes) != 2 {
		t.Fatalf("chain removes = %+v; want the quotation and its purchase order", plan.Removes)
	}
	counterBefore := counterValues(t, database)

	challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create chain challenge: %v", err)
	}
	if challenge.Letters == "" || len(challenge.Letters) != DeletionChallengeLength {
		t.Fatalf("challenge letters = %q", challenge.Letters)
	}
	if challenge.Fingerprint != preview.Fingerprint {
		t.Errorf("challenge fingerprint = %q; want %q", challenge.Fingerprint, preview.Fingerprint)
	}

	confirm := DeletionConfirmInput{
		TargetType:          DeletionTargetQuotation,
		TargetID:            quotation.ID,
		Scope:               DeletionScopeChain,
		Reason:              "entered by mistake",
		TypedNumber:         quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint,
		ChallengeID:         challenge.ID,
		ActorID:             admin.ID,
	}
	confirm.ChallengeLetters = "0000000000"
	if _, err := database.ConfirmDeletion(ctx, confirm); !errors.Is(err, ErrDeletionChallengeInvalid) {
		t.Fatalf("wrong letters error = %v; want ErrDeletionChallengeInvalid", err)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 {
		t.Fatal("a wrong challenge deleted the chain")
	}

	confirm.ChallengeLetters = strings.ToLower(challenge.Letters)
	result, err := database.ConfirmDeletion(ctx, confirm)
	if err != nil {
		t.Fatalf("delete chain with a lower-case challenge: %v", err)
	}
	if len(result.Removed) != 2 {
		t.Errorf("removed = %+v; want quotation and purchase order", result.Removed)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 0 {
		t.Fatal("quotation survived chain deletion")
	}
	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 0 {
		t.Fatal("purchase order survived chain deletion")
	}
	if countRows(t, database, "purchase_order_documents", "purchase_order_id = $1", saved.ID) != 0 {
		t.Fatal("attachments survived chain deletion")
	}
	if countRows(t, database, "purchase_order_corrections", "purchase_order_id = $1", saved.ID) != 0 {
		t.Fatal("correction history survived chain deletion")
	}
	assertCountersUnchanged(t, counterBefore, counterValues(t, database))
	if n := countRows(t, database, "deletion_audits", ""); n != 2 {
		t.Errorf("audits = %d; want one per deleted document", n)
	}
	if n := countRows(t, database, "deletion_challenges", "id = $1", challenge.ID); n != 0 {
		t.Error("the used challenge was not consumed")
	}

	// Replaying the consumed challenge cannot delete anything twice. The
	// target is already gone, so the refusal may be either the missing target
	// or the dead challenge; both stop a second removal.
	if _, err := database.ConfirmDeletion(ctx, confirm); err == nil {
		t.Fatal("replayed confirmation succeeded; want a refusal")
	}
}

func TestDeleteChainStopsWhenTheChainChangedAndRenewsTheChallenge(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_chain_changed")
	quotation, saved, user, ctx := savedDeletionPO(t, database, "PT. Changing", "PO-480-CHANGING")
	admin := deletionUser(t, database, "admin")

	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}

	// A correction moves the connected PO's version after the challenge was
	// issued. The typed confirmation is now bound to a chain that no longer
	// exists, so the deletion must stop and name what moved.
	correction := finalizeInput(saved, user, saved.PONumber)
	correction.DeliveryAddress = "Jl. Changed 7"
	if _, err := database.CorrectPurchaseOrder(ctx, PurchaseOrderCorrectionInput{
		PurchaseOrderUpdate: correction.PurchaseOrderUpdate,
		Reason:              "changed delivery address",
	}); err != nil {
		t.Fatalf("correct the connected PO: %v", err)
	}

	consult := DeletionConfirmInput{
		TargetType:          DeletionTargetQuotation,
		TargetID:            quotation.ID,
		Scope:               DeletionScopeChain,
		Reason:              "entered by mistake",
		TypedNumber:         quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint,
		ChallengeID:         challenge.ID,
		ChallengeLetters:    challenge.Letters,
		ActorID:             admin.ID,
	}
	_, err = database.ConfirmDeletion(ctx, consult)
	var changed *DeletionChainChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("changed chain error = %v; want DeletionChainChangedError", err)
	}
	if len(changed.Changes) == 0 {
		t.Fatal("changed chain reported no explanation")
	}
	joined := ""
	for _, change := range changed.Changes {
		joined += change.Label + " "
	}
	if !strings.Contains(joined, "purchase order") && !strings.Contains(joined, "Purchase order") {
		t.Errorf("changes = %+v; want the connected purchase order named", changed.Changes)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 ||
		countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 1 {
		t.Fatal("a changed chain was deleted anyway")
	}
	if n := countRows(t, database, "deletion_challenges", "id = $1", challenge.ID); n != 1 {
		t.Error("the stale challenge was consumed by a refused confirmation")
	}

	// The renewal path: preview again, take a fresh challenge, confirm.
	renewed := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	fresh, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: renewed.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("renew challenge: %v", err)
	}
	if fresh.Fingerprint == preview.Fingerprint {
		t.Fatal("renewed challenge kept the stale fingerprint")
	}
	consult.ExpectedFingerprint = renewed.Fingerprint
	consult.ChallengeID = fresh.ID
	consult.ChallengeLetters = fresh.Letters
	if _, err := database.ConfirmDeletion(ctx, consult); err != nil {
		t.Fatalf("delete with the renewed challenge: %v", err)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 0 ||
		countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 0 {
		t.Fatal("the renewed confirmation did not delete the chain")
	}
}

func TestDeletionChallengesExpire(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_expired")
	quotation, _, _, ctx := savedDeletionPO(t, database, "PT. Expired", "PO-480-EXPIRED")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.deletion_challenges SET created_at = NOW() - INTERVAL '20 minutes', expires_at = NOW() - INTERVAL '5 minutes' WHERE id = $1",
		database.configSchema()), challenge.ID); err != nil {
		t.Fatalf("expire challenge: %v", err)
	}
	_, err = database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, Reason: "expired", TypedNumber: quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint, ChallengeID: challenge.ID,
		ChallengeLetters: challenge.Letters, ActorID: admin.ID,
	})
	if !errors.Is(err, ErrDeletionChallengeExpired) {
		t.Fatalf("expired challenge error = %v; want ErrDeletionChallengeExpired", err)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 1 {
		t.Fatal("an expired challenge deleted the chain")
	}
}

// expireDeletionChallenge moves a challenge's life into the past without
// deleting it, which is the state a challenge is in when it has outlived its
// fifteen minutes.
func expireDeletionChallenge(t *testing.T, database *Store, id string) {
	t.Helper()
	if _, err := database.pool.Exec(context.Background(), fmt.Sprintf(
		"UPDATE %s.deletion_challenges SET created_at = NOW() - INTERVAL '20 minutes', expires_at = NOW() - INTERVAL '5 minutes' WHERE id = $1",
		database.configSchema()), id); err != nil {
		t.Fatalf("expire challenge: %v", err)
	}
}

// assertNoChallengeFingerprintNames proves no challenge row still quotes one of
// the deleted values. A fingerprint is a snapshot of a chain, so retaining one
// after the chain is gone keeps the deleted document's number and counts alive.
func assertNoChallengeFingerprintNames(t *testing.T, database *Store, values ...string) {
	t.Helper()
	rows, err := database.pool.Query(context.Background(), fmt.Sprintf(
		"SELECT id::text, target_type, target_id, fingerprint FROM %s.deletion_challenges",
		database.configSchema()))
	if err != nil {
		t.Fatalf("read deletion challenges: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, targetType, fingerprint string
		var targetID int64
		if err := rows.Scan(&id, &targetType, &targetID, &fingerprint); err != nil {
			t.Fatalf("scan deletion challenge: %v", err)
		}
		for _, value := range values {
			if value == "" {
				continue
			}
			if strings.Contains(fingerprint, value) {
				t.Errorf("challenge %s (%s %d) still retains %q in its fingerprint",
					id, targetType, targetID, value)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate deletion challenges: %v", err)
	}
}

// TestExpiredChallengeIsRemovedWhenAnotherChallengeDeletesTheChain is the
// Judge's reproduction of the retention defect: challenge A expires, challenge
// B is issued for the same chain, and B deletes it. A used to survive with its
// full fingerprint -- the deleted quotation number, revisions and record counts
// -- because cleanup only ran on later challenge creation and only for rows
// expired over a day. A successful deletion must leave no challenge that names
// a record it removed.
func TestExpiredChallengeIsRemovedWhenAnotherChallengeDeletesTheChain(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_expired_sibling")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Sibling", "PO-480-SIBLING")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)

	createChallenge := func() DeletionChallenge {
		t.Helper()
		challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
			ActorID: admin.ID,
		})
		if err != nil {
			t.Fatalf("create challenge: %v", err)
		}
		return challenge
	}

	stale := createChallenge()
	expireDeletionChallenge(t, database, stale.ID)

	// Issuing B is itself a path that observes A's expiry, so A must already be
	// gone here; the deletion below is the second, stronger path. This is not
	// fatal so the run still reaches the deletion assertions on a regression.
	fresh := createChallenge()
	if n := countRows(t, database, "deletion_challenges", "id = $1", stale.ID); n != 0 {
		t.Errorf("expired challenge A survived the creation of challenge B (%d row)", n)
	}

	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, Reason: "entered by mistake",
		TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ChallengeID: fresh.ID, ChallengeLetters: fresh.Letters, ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete the chain with challenge B: %v", err)
	}
	if n := countRows(t, database, "quotations", "id = $1", quotation.ID); n != 0 {
		t.Fatal("challenge B did not delete the quotation")
	}
	if n := countRows(t, database, "purchase_orders", "id = $1", saved.ID); n != 0 {
		t.Fatal("challenge B did not delete the purchase order")
	}
	if n := countRows(t, database, "deletion_challenges", "id = $1", fresh.ID); n != 0 {
		t.Fatal("the spent challenge B survived its own deletion")
	}
	if n := countRows(t, database, "deletion_challenges", ""); n != 0 {
		t.Errorf("challenges = %d after the deletion; want every challenge for a removed record purged", n)
	}
	assertNoChallengeFingerprintNames(t, database,
		quotation.QuoteNumber, quotation.UUID, saved.PONumber)
}

// TestDeletionRemovesEveryChallengeItsOwnWriteInvalidated keeps the other
// sibling: a challenge issued against the quotation and one issued against the
// purchase order. Deleting the chain through the purchase order spends that
// challenge and must take the quotation's still-usable one with it, because the
// record it named is gone.
func TestDeletionRemovesEveryChallengeItsOwnWriteInvalidated(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_sibling_challenge")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Both", "PO-480-BOTH")
	admin := deletionUser(t, database, "admin")

	quotationPreview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	quotationChallenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: quotationPreview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create quotation challenge: %v", err)
	}
	orderPreview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	orderChallenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: orderPreview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create purchase order challenge: %v", err)
	}

	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeChain, Reason: "entered by mistake",
		TypedNumber: saved.PONumber, ExpectedFingerprint: orderPreview.Fingerprint,
		ChallengeID: orderChallenge.ID, ChallengeLetters: orderChallenge.Letters,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete the chain through the purchase order: %v", err)
	}
	if n := countRows(t, database, "deletion_challenges", "id = $1", orderChallenge.ID); n != 0 {
		t.Fatal("the spent purchase order challenge survived")
	}
	if n := countRows(t, database, "deletion_challenges", "id = $1", quotationChallenge.ID); n != 0 {
		t.Fatal("a challenge for the deleted quotation survived the chain deletion")
	}
	if n := countRows(t, database, "deletion_challenges", ""); n != 0 {
		t.Errorf("challenges = %d after the deletion; want none", n)
	}
}

// TestExpiredChallengesArePurgedByEveryDeletionObservation pins the retention
// contract: a challenge row is removed by the next preview, the next challenge
// issuance, or the confirmation that observes it as expired -- not merely by a
// later challenge for one target, and not only after an extra day.
func TestExpiredChallengesArePurgedByEveryDeletionObservation(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_expired_paths")
	quotation, _, _, ctx := savedDeletionPO(t, database, "PT. Paths", "PO-480-PATHS")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)

	createChallenge := func(fingerprint string) DeletionChallenge {
		t.Helper()
		challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeChain, ExpectedFingerprint: fingerprint,
			ActorID: admin.ID,
		})
		if err != nil {
			t.Fatalf("create challenge: %v", err)
		}
		return challenge
	}

	// A preview observes the expiry.
	observedByPreview := createChallenge(preview.Fingerprint)
	expireDeletionChallenge(t, database, observedByPreview.ID)
	preview = previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if n := countRows(t, database, "deletion_challenges", "id = $1", observedByPreview.ID); n != 0 {
		t.Fatal("a preview left an expired challenge in place")
	}

	// Issuing the next challenge observes it too.
	observedByIssue := createChallenge(preview.Fingerprint)
	expireDeletionChallenge(t, database, observedByIssue.ID)
	renewed := createChallenge(preview.Fingerprint)
	if n := countRows(t, database, "deletion_challenges", "id = $1", observedByIssue.ID); n != 0 {
		t.Fatal("challenge issuance left an expired challenge in place")
	}

	// The confirmation that is refused for expiry removes the row it refused.
	expireDeletionChallenge(t, database, renewed.ID)
	_, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, Reason: "too late", TypedNumber: quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint, ChallengeID: renewed.ID,
		ChallengeLetters: renewed.Letters, ActorID: admin.ID,
	})
	if !errors.Is(err, ErrDeletionChallengeExpired) {
		t.Fatalf("expired confirmation error = %v; want ErrDeletionChallengeExpired", err)
	}
	if n := countRows(t, database, "deletion_challenges", "id = $1", renewed.ID); n != 0 {
		t.Fatal("the confirmation left the expired challenge it refused")
	}
	if n := countRows(t, database, "deletion_challenges", ""); n != 0 {
		t.Errorf("challenges = %d; want every expired row purged", n)
	}
	if n := countRows(t, database, "quotations", "id = $1", quotation.ID); n != 1 {
		t.Fatal("an expired confirmation deleted the chain")
	}
}

func TestDeletePurchaseOrderChainRemovesTheSourceAndEveryRevision(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_po_chain")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. From PO", "PO-480-FROM-PO")
	admin := deletionUser(t, database, "admin")
	// A second issued revision makes "all revisions" observable.
	if _, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode: 2, CustomerName: "PT. From PO", CustomerAddress: "Jl. Billing 1",
		RecipientName: "Budi Santoso", RecipientAddress: "Jl. Recipient 9",
		Subject: "Deletion subject", Incoterm: "CIF", ValidityDays: 30,
		PaymentTermDays: 45, LeadTimeDays: 10, TaxZone: "FTZ", TaxType: "Goods",
		Currency: "USD", RevisionNumber: 1, QuoteNumber: quotation.QuoteNumber,
		HTMLSnapshot: "<html>revision one</html>",
		Items: []RevisionItem{{
			ItemNo: 1, Description: "Priced widget", Qty: 1, UoM: "Pc",
			UnitPrice: 100, OriginalTotal: 100, MarginalizedUnitPrice: 100,
			MarginalizedTotal: 100, ListUnitPrice: 100,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal: 100, OriginalGrand: 100,
			MarginalizedSubtotal: 100, MarginalizedGrand: 100,
			MarginalizedGrandAfterTax: 100, CustomerGrandTotal: 100,
		},
	}); err != nil {
		t.Fatalf("seed second revision: %v", err)
	}

	preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	plan := chainScope(t, preview)
	if !plan.Available || !plan.RequiresChallenge {
		t.Fatalf("chain plan = %+v; want available with a challenge", plan)
	}
	if len(plan.Removes) != 2 {
		t.Fatalf("chain removes = %+v; want the quotation and its PO", plan.Removes)
	}
	challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeChain, Reason: "whole chain entered by mistake",
		TypedNumber: "PO-480-FROM-PO", ExpectedFingerprint: preview.Fingerprint,
		ChallengeID: challenge.ID, ChallengeLetters: challenge.Letters,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete chain from the purchase order: %v", err)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 0 {
		t.Fatal("quotation survived a chain deletion started from its PO")
	}
	if countRows(t, database, "quotation_revisions", "quotation_id = $1", quotation.ID) != 0 {
		t.Fatal("quotation revisions survived a chain deletion started from its PO")
	}
	if countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 0 {
		t.Fatal("purchase order survived its own chain deletion")
	}
	if n := countRows(t, database, "deletion_audits", ""); n != 2 {
		t.Errorf("audits = %d; want one per deleted document", n)
	}
}

func TestDeleteQuotationRecordKeepsADraftAsSourceMissing(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_draft_survives")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Draft", "Draft survives")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeRecord, Reason: "issued by mistake",
		TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete quotation with a draft PO: %v", err)
	}
	surviving, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read surviving draft: %v", err)
	}
	if !surviving.SourceMissing || surviving.State != PurchaseOrderStateDraft {
		t.Errorf("surviving draft = %+v; want a source-missing draft", surviving)
	}
	// The draft cannot be saved while it has no source; the refusal is named so
	// #481's relink surface can explain it.
	if _, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
		ID: draft.ID, ExpectedVersion: surviving.Version, UpdatedBy: user.ID,
		Items: []PurchaseOrderItem{{Description: "still editable", QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 1}},
	}); !errors.Is(err, ErrPurchaseOrderSourceMissing) {
		t.Fatalf("update of a source-missing draft = %v; want ErrPurchaseOrderSourceMissing", err)
	}
}

func TestConcurrentChainDeletionsRunOnce(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_race")
	quotation, saved, _, ctx := savedDeletionPO(t, database, "PT. Race", "PO-480-RACE")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}

	confirm := DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, Reason: "race", TypedNumber: quotation.QuoteNumber,
		ExpectedFingerprint: preview.Fingerprint, ChallengeID: challenge.ID,
		ChallengeLetters: challenge.Letters, ActorID: admin.ID,
	}
	var wait sync.WaitGroup
	errs := make([]error, 2)
	for index := range errs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errs[index] = database.ConfirmDeletion(ctx, confirm)
		}(index)
	}
	wait.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent confirmations succeeded %d times; want exactly one (%v)", successes, errs)
	}
	if countRows(t, database, "quotations", "id = $1", quotation.ID) != 0 ||
		countRows(t, database, "purchase_orders", "id = $1", saved.ID) != 0 {
		t.Fatal("the chain survived the winning deletion")
	}
	if n := countRows(t, database, "deletion_audits", ""); n != 2 {
		t.Errorf("audits = %d; want one per deleted document, not one per attempt", n)
	}
}

func TestChainDeletionRaceWithACorrectionNeverCascadesUnconfirmed(t *testing.T) {
	database := openTestSchema(t, "vera_po_delete_race_correction")
	quotation, saved, user, ctx := savedDeletionPO(t, database, "PT. Race Correction", "PO-480-RACE-C")
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	challenge, err := database.CreateDeletionChallenge(ctx, DeletionChallengeInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeChain, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}

	correction := finalizeInput(saved, user, saved.PONumber)
	correction.PaymentTerms = "30 days after delivery"

	var wait sync.WaitGroup
	var deletionErr, correctionErr error
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, deletionErr = database.ConfirmDeletion(ctx, DeletionConfirmInput{
			TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
			Scope: DeletionScopeChain, Reason: "race", TypedNumber: quotation.QuoteNumber,
			ExpectedFingerprint: preview.Fingerprint, ChallengeID: challenge.ID,
			ChallengeLetters: challenge.Letters, ActorID: admin.ID,
		})
	}()
	go func() {
		defer wait.Done()
		_, correctionErr = database.CorrectPurchaseOrder(ctx, PurchaseOrderCorrectionInput{
			PurchaseOrderUpdate: correction.PurchaseOrderUpdate,
			Reason:              "concurrent correction",
		})
	}()
	wait.Wait()

	// The invariant is not which goroutine wins. It is that a deletion which
	// reports success never ran on a chain a correction changed first, and a
	// successful correction is never silently destroyed by a confirmation that
	// was bound to the earlier version.
	quotationsLeft := countRows(t, database, "quotations", "id = $1", quotation.ID)
	ordersLeft := countRows(t, database, "purchase_orders", "id = $1", saved.ID)
	if deletionErr == nil {
		if quotationsLeft != 0 || ordersLeft != 0 {
			t.Fatal("the deletion reported success but the chain survived")
		}
		if correctionErr == nil {
			t.Fatal("both the deletion and the correction reported success on one row")
		}
	} else {
		var changed *DeletionChainChangedError
		if !errors.As(deletionErr, &changed) && !errors.Is(deletionErr, ErrDeletionChallengeInvalid) {
			t.Fatalf("deletion error = %v; want a chain-changed or challenge refusal", deletionErr)
		}
		if quotationsLeft != 1 || ordersLeft != 1 {
			t.Fatal("a refused deletion still removed part of the chain")
		}
		if correctionErr != nil {
			t.Fatalf("correction failed while the deletion was refused: %v", correctionErr)
		}
	}
}
