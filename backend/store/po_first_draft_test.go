package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Issue #496: a Purchase Order can start before its source Quotation exists.
// These tests drive the real store against a disposable schema and pin the
// three live row shapes: linked, formerly linked (with a complete former-source
// reference) and never linked (no fabricated source at all), plus the server's
// own currency/tax defaults and the audit snapshot's honest NULLs.

func createBlankDraft(t *testing.T, database *Store, ctx context.Context, user AuthUser) PurchaseOrder {
	t.Helper()
	draft, err := database.CreatePurchaseOrderBlankDraft(ctx, PurchaseOrderBlankCreate{
		ReceivedDate:    datePointer(time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)),
		TaxZone:         "FTZ",
		TaxType:         "Goods",
		ServiceCategory: "",
		CreatedBy:       user.ID,
	})
	if err != nil {
		t.Fatalf("create blank purchase order: %v", err)
	}
	return draft
}

func datePointer(value time.Time) *time.Time { return &value }

func blankDraftUpdate(draft PurchaseOrder, version int64, user AuthUser) PurchaseOrderUpdate {
	return PurchaseOrderUpdate{
		ID:              draft.ID,
		BillingAddress:  "Jl. Billing entered before the source",
		DeliveryAddress: "Jl. Delivery entered before the source",
		ContactEmail:    "buyer@example.com",
		PONumber:        "CUSTOMER-PO-1",
		ReceivedDate:    datePointer(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)),
		PaymentTerms:    "50% advance",
		Incoterms:       "FOB",
		UpdatedBy:       user.ID,
		ExpectedVersion: version,
		Items: []PurchaseOrderItem{{
			Description:    "Incomplete line entered first",
			QuantityScaled: 15000,
			UoM:            "Pc",
			UnitPriceMinor: 12500,
		}},
	}
}

func TestBlankPurchaseOrderDraftHasNoFabricatedSourceOrCustomer(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_blank")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft := createBlankDraft(t, database, ctx, user)

	if draft.ID < 1 || draft.State != PurchaseOrderStateDraft {
		t.Fatalf("blank draft = %+v; want a stored draft", draft)
	}
	if draft.SourceState != PurchaseOrderSourceStateNever {
		t.Errorf("source state = %q; want %q", draft.SourceState, PurchaseOrderSourceStateNever)
	}
	if !draft.SourceMissing {
		t.Error("a never-linked draft must read as Source missing")
	}
	if draft.PrintedTotalsKnown {
		t.Error("a never-linked draft has no printed source totals")
	}
	if draft.QuotationID != 0 || draft.CustomerID != 0 || draft.SourceRevision != 0 {
		t.Errorf("identity = quotation %d customer %d revision %d; want no source and no customer",
			draft.QuotationID, draft.CustomerID, draft.SourceRevision)
	}
	if draft.QuoteNumber != "" || draft.CustomerName != "" {
		t.Errorf("fabricated source/customer = %q / %q; want empty", draft.QuoteNumber, draft.CustomerName)
	}
	if draft.Currency != "IDR" {
		t.Errorf("currency = %q; want the company default IDR", draft.Currency)
	}
	if draft.TaxZone != "FTZ" || draft.TaxType != "Goods" || draft.ServiceCategory != "" {
		t.Errorf("tax basis = %q/%q/%q; want the supplied FTZ/Goods triple",
			draft.TaxZone, draft.TaxType, draft.ServiceCategory)
	}
	if draft.PpnBps != 0 {
		t.Errorf("PPN = %d bps; want 0 for FTZ goods", draft.PpnBps)
	}
	if len(draft.Items) != 0 {
		t.Errorf("lines = %d; an untouched blank draft invents none", len(draft.Items))
	}
	if draft.SubtotalMinor != 0 || draft.GrandTotalMinor != 0 ||
		draft.PrintedSubtotalMinor != 0 || draft.PrintedGrandTotalMinor != 0 {
		t.Errorf("totals were invented: %+v", draft)
	}
}

func TestSourceLessDraftSavesAndReopensIncompleteValues(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_save")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	saved, err := database.UpdatePurchaseOrderDraft(ctx, blankDraftUpdate(draft, draft.Version, user))
	if err != nil {
		t.Fatalf("save a never-linked draft: %v", err)
	}
	if saved.ID != draft.ID {
		t.Errorf("save created id %d; want the same row %d", saved.ID, draft.ID)
	}
	if saved.PONumber != "CUSTOMER-PO-1" || saved.PaymentTerms != "50% advance" ||
		saved.Incoterms != "FOB" || saved.BillingAddress == "" {
		t.Errorf("saved header = %+v; want every typed value kept", saved)
	}
	if len(saved.Items) != 1 || saved.Items[0].Description != "Incomplete line entered first" {
		t.Errorf("saved lines = %+v; want the incomplete line kept", saved.Items)
	}
	if saved.SourceState != PurchaseOrderSourceStateNever || saved.CustomerID != 0 {
		t.Errorf("save invented a source/customer: %+v", saved)
	}

	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen the never-linked draft: %v", err)
	}
	if reopened.PONumber != saved.PONumber || len(reopened.Items) != 1 ||
		reopened.Items[0].UnitPriceMinor != 12500 {
		t.Errorf("reopened = %+v; want the stored values", reopened)
	}

	// A second save updates the same row rather than creating another.
	update := blankDraftUpdate(reopened, reopened.Version, user)
	update.PONumber = "CUSTOMER-PO-1-REVISED"
	again, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if again.ID != draft.ID || again.PONumber != "CUSTOMER-PO-1-REVISED" {
		t.Errorf("second save = %+v; want the same row updated", again)
	}
	if countRows(t, database, "purchase_orders", "id = $1", draft.ID) != 1 {
		t.Error("the second save duplicated the row")
	}
}

func TestSourceLessDraftFinalizeAndReviewRefuseAtTheStoreBoundary(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_refuse")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)
	saved, err := database.UpdatePurchaseOrderDraft(ctx, blankDraftUpdate(draft, draft.Version, user))
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}

	if _, err := database.GetPurchaseOrderReview(ctx, saved.ID); !errors.Is(err, ErrPurchaseOrderSourceMissing) {
		t.Errorf("review of a never-linked draft = %v; want ErrPurchaseOrderSourceMissing", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, PurchaseOrderFinalize{
		PurchaseOrderUpdate: blankDraftUpdate(saved, saved.Version, user),
		DocumentConfirmed:   true,
	}); !errors.Is(err, ErrPurchaseOrderSourceMissing) {
		t.Errorf("finalize of a never-linked draft = %v; want ErrPurchaseOrderSourceMissing", err)
	}
	// The refusal is neutral: it names the missing link, not a deletion.
	if errors.Is(err, ErrPurchaseOrderSourceMissing) &&
		!containsAny(ErrPurchaseOrderSourceMissing.Error(), "link a quotation", "live source") {
		t.Errorf("refusal wording = %q; want neutral link guidance", ErrPurchaseOrderSourceMissing.Error())
	}
}

func TestFormerlyLinkedDraftCanStillSaveItsOwnValues(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_former_save")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Former", "Former source draft")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, quotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: quotation.ID,
		Scope: DeletionScopeRecord, Reason: "issued by mistake",
		TypedNumber: quotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete the source quotation: %v", err)
	}
	former, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read the former-source draft: %v", err)
	}
	if former.SourceState != PurchaseOrderSourceStateFormer {
		t.Fatalf("source state = %q; want former", former.SourceState)
	}
	if !former.PrintedTotalsKnown {
		t.Error("a formerly-linked record still knows the totals its source printed")
	}

	update := PurchaseOrderUpdate{
		ID:              former.ID,
		BillingAddress:  "Jl. Edited after the source disappeared",
		DeliveryAddress: former.DeliveryAddress,
		ReceivedDate:    datePointer(former.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: former.Version,
		Items: []PurchaseOrderItem{{
			Description:    "Edited while source missing",
			QuantityScaled: 20000,
			UoM:            "Pc",
			UnitPriceMinor: 300,
		}},
	}
	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("save a formerly-linked draft: %v", err)
	}
	if saved.BillingAddress != "Jl. Edited after the source disappeared" ||
		len(saved.Items) != 1 || saved.Items[0].UnitPriceMinor != 300 {
		t.Errorf("saved former draft = %+v; want the PO-owned values kept", saved)
	}
	if saved.SourceState != PurchaseOrderSourceStateFormer {
		t.Errorf("save changed the source state to %q", saved.SourceState)
	}
}

func TestNeverLinkedOverwriteSnapshotStoresNullSourceIdentities(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_snapshot")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	first, err := database.UpdatePurchaseOrderDraft(ctx, blankDraftUpdate(draft, draft.Version, user))
	if err != nil {
		t.Fatalf("first draft save: %v", err)
	}
	overwrite := blankDraftUpdate(first, first.Version, user)
	overwrite.Overwrite = true
	if _, err := database.UpdatePurchaseOrderDraft(ctx, overwrite); err != nil {
		t.Fatalf("overwrite draft: %v", err)
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
		t.Fatalf("read the overwrite snapshot: %v", err)
	}
	if !quotationNull || !revisionNull || !customerNull {
		t.Errorf("snapshot identities = quotation null %v / revision null %v / customer null %v; want all NULL, never 0",
			quotationNull, revisionNull, customerNull)
	}
	if quoteNumber != "" {
		t.Errorf("snapshot quote number = %q; want an empty display copy, not a fabricated number", quoteNumber)
	}

	history, err := database.GetPurchaseOrderHistory(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history = %d entries; want the one overwrite", len(history))
	}
	if !history[0].SourceMissing {
		t.Error("a never-linked snapshot must read as source missing")
	}
	if history[0].QuotationID != 0 || history[0].SourceRevision != 0 || history[0].CustomerID != 0 {
		t.Errorf("history numbers = %d/%d/%d; want the DTO's numeric zeros",
			history[0].QuotationID, history[0].SourceRevision, history[0].CustomerID)
	}
}

func TestBlankDraftSourceShapeOnlyAllowsDraftState(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_shape")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	_, err = pool.Exec(ctx, `UPDATE `+database.configSchema()+`.purchase_orders
		SET state = 'saved' WHERE id = $1`, draft.ID)
	if err == nil {
		t.Fatal("a never-linked row was allowed into the saved state; the shape CHECK did not hold")
	}
}

func TestSourceLessDraftPersistsFullTaxTripleAndRecomputesPPN(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_tax")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	update := blankDraftUpdate(draft, draft.Version, user)
	update.TaxZone = "NonFTZ"
	update.TaxType = "Goods"
	update.ServiceCategory = ""
	update.Currency = "USD"
	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("save with an edited basis: %v", err)
	}
	if saved.Currency != "USD" {
		t.Errorf("currency = %q; want the edited USD", saved.Currency)
	}
	if saved.TaxZone != "NonFTZ" || saved.TaxType != "Goods" || saved.ServiceCategory != "" {
		t.Errorf("tax triple = %q/%q/%q; want the full persisted triple",
			saved.TaxZone, saved.TaxType, saved.ServiceCategory)
	}
	if saved.PpnBps != 1100 {
		t.Errorf("PPN = %d bps; want 1100 for Non-FTZ goods", saved.PpnBps)
	}

	reopened, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Currency != "USD" || reopened.TaxType != "Goods" || reopened.PpnBps != 1100 {
		t.Errorf("reopened basis = %+v; want the persisted triple and rate", reopened)
	}
}

func TestLinkedDraftKeepsItsFrozenCurrencyAndTaxBasis(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_linked_frozen")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Frozen", "Frozen basis")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	update := PurchaseOrderUpdate{
		ID:              draft.ID,
		Currency:        "USD",
		TaxZone:         "NonFTZ",
		TaxType:         "Goods",
		ReceivedDate:    datePointer(draft.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
		Items:           draft.Items,
	}
	// The seeded source is USD/FTZ, so changing it to NonFTZ must be refused.
	if _, err := database.UpdatePurchaseOrderDraft(ctx, update); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("change a linked draft's tax basis = %v; want ErrPurchaseOrderInvalid", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read unchanged linked draft: %v", err)
	}
	if unchanged.TaxZone != draft.TaxZone || unchanged.Currency != draft.Currency ||
		unchanged.PpnBps != draft.PpnBps {
		t.Errorf("linked basis moved: %+v -> %+v", draft, unchanged)
	}
}

func TestListPurchaseOrdersScansNeverAndFormerSourceMissing(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_list")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	never := createBlankDraft(t, database, ctx, user)

	formerQuotation := seedFinalizeQuotation(t, database, 2, "PT. Former List", "Former list")
	former := createFinalizeDraft(t, database, ctx, user, formerQuotation)
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, formerQuotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: formerQuotation.ID,
		Scope: DeletionScopeRecord, Reason: "list coverage",
		TypedNumber: formerQuotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete former source: %v", err)
	}

	liveQuotation := seedFinalizeQuotation(t, database, 3, "PT. Live List", "Live list")
	live := createFinalizeDraft(t, database, ctx, user, liveQuotation)

	only, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		SourceMissing: PurchaseOrderSourceMissingOnly,
	})
	if err != nil {
		t.Fatalf("list source missing only: %v", err)
	}
	if len(only.Rows) != 2 {
		t.Fatalf("source-missing rows = %d; want the never-linked and former-linked rows", len(only.Rows))
	}
	states := map[int64]string{}
	for _, row := range only.Rows {
		states[row.ID] = row.SourceState
		if row.ID == never.ID && row.CustomerID != 0 {
			t.Errorf("never-linked row customer = %d; want no customer", row.CustomerID)
		}
	}
	if states[never.ID] != PurchaseOrderSourceStateNever {
		t.Errorf("never row state = %q; want never", states[never.ID])
	}
	if states[former.ID] != PurchaseOrderSourceStateFormer {
		t.Errorf("former row state = %q; want former", states[former.ID])
	}
	for _, customer := range only.Customers {
		if customer.ID == 0 {
			t.Error("the customer facet invented a zero-id customer for a source-less row")
		}
	}

	exclude, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{
		SourceMissing: PurchaseOrderSourceMissingExclude,
	})
	if err != nil {
		t.Fatalf("list source present only: %v", err)
	}
	if len(exclude.Rows) != 1 || exclude.Rows[0].ID != live.ID {
		t.Errorf("source-present rows = %+v; want only the linked draft %d", exclude.Rows, live.ID)
	}
	if exclude.Rows[0].SourceState != PurchaseOrderSourceStateLive ||
		exclude.Rows[0].SourceRevision != live.SourceRevision {
		t.Errorf("live row = %+v; want live revision %d", exclude.Rows[0], live.SourceRevision)
	}
}

func TestSourceLessDraftIsIsolatedToItsOrganization(t *testing.T) {
	database := openTestSchema(t, "vera_po_first_isolation")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	mine := createBlankDraft(t, database, ctx, user)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var otherOrg string
	if err := pool.QueryRow(ctx, `
		INSERT INTO `+database.configSchema()+`.organizations (name)
		VALUES ('Other Tenant') RETURNING id::text`).Scan(&otherOrg); err != nil {
		t.Fatalf("seed second organization: %v", err)
	}
	other := ContextWithOrg(context.Background(), otherOrg)

	if _, err := database.GetPurchaseOrder(other, mine.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("read another organization's source-less draft = %v; want ErrPurchaseOrderNotFound", err)
	}
	if _, err := database.UpdatePurchaseOrderDraft(other, blankDraftUpdate(mine, mine.Version, user)); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("save another organization's source-less draft = %v; want ErrPurchaseOrderNotFound", err)
	}
	if page, err := database.ListPurchaseOrders(other, PurchaseOrderListQuery{}); err != nil {
		t.Errorf("list another organization's archive: %v", err)
	} else if len(page.Rows) != 0 {
		t.Errorf("another organization saw %d source-less drafts; want none", len(page.Rows))
	}

	// Creation is tenant-scoped too: the other organization gets its own row,
	// and neither can read the other's.
	theirs := createBlankDraft(t, database, other, user)
	if theirs.ID == mine.ID || theirs.OrgID == mine.OrgID {
		t.Fatalf("second organization draft = %+v; want its own id and org", theirs)
	}
	if _, err := database.GetPurchaseOrder(ctx, theirs.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("read the other organization's draft from mine = %v; want ErrPurchaseOrderNotFound", err)
	}
	if page, err := database.ListPurchaseOrders(ctx, PurchaseOrderListQuery{}); err != nil {
		t.Errorf("list my archive: %v", err)
	} else if len(page.Rows) != 1 || page.Rows[0].ID != mine.ID {
		t.Errorf("my archive = %+v; want only my source-less draft", page.Rows)
	}
}

func containsAny(value string, needles ...string) bool {
	lowered := strings.ToLower(value)
	for _, needle := range needles {
		if len(needle) > 0 && strings.Contains(lowered, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
