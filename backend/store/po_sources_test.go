package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Issue #483: the selector's page is caller input and its offset is
// (page-1)*POSourcePageSize. A page the HTTP handler accepted could overflow
// that product into a negative OFFSET, which Postgres rejected as a database
// error and the API surfaced as a 500. The store refuses the page as invalid
// input instead.
func TestListPOSourceQuotationsRejectsAPageWhoseOffsetOverflows(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_page_overflow")
	ctx := poContext(t, database)

	// 614891469123651722 = floor(MaxInt64/15)+2, so (page-1)*15 wraps
	// negative and Postgres rejects the OFFSET. That must be invalid input,
	// never a database error the caller sees as a server failure.
	if _, _, err := database.ListPOSourceQuotations(ctx, 614891469123651722, "", false, 0); !errors.Is(err, ErrPOSourceInvalid) {
		t.Errorf("overflowing page error = %v; want ErrPOSourceInvalid", err)
	}

	// One less is representable: an empty page past the end, not a refusal.
	// This pins that the guard rejects only the unrepresentable offset and
	// never clamps.
	rows, total, err := database.ListPOSourceQuotations(ctx, 614891469123651721, "", false, 0)
	if err != nil {
		t.Fatalf("largest representable page: %v", err)
	}
	if len(rows) != 0 || total != 0 {
		t.Errorf("boundary page = %d rows, total %d; want an empty page past the end",
			len(rows), total)
	}
}

// TestPOSourcePageOverflowErrorIsNotNotFound keeps the new sentinel distinct
// from ErrPOSourceNotFound: a page past the offset limit is malformed input,
// not a missing Quotation, and the handler's 404 branch must not catch it.
func TestPOSourcePageOverflowErrorIsNotNotFound(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_page_overflow_kind")
	ctx := poContext(t, database)

	_, _, err := database.ListPOSourceQuotations(ctx, 614891469123651722, "", false, 0)
	if errors.Is(err, ErrPOSourceNotFound) {
		t.Fatalf("overflowing page error = %v; must not be ErrPOSourceNotFound", err)
	}
}

// Issue #499/#651: the archived Quotation detail asks whether the quotation is
// fully bought and which record to reopen. Bought state spans revisions and is
// derived from saved allocations; drafts are advisory and never make a
// quotation look consumed.
func TestPOSourceOccupancyAnswersBoughtStateAcrossRevisions(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_occupancy")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Occupancy")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	// Free before anything occupies it.
	occupancy, err := database.GetPOSourceOccupancy(ctx, quotation.ID)
	if err != nil {
		t.Fatalf("get free occupancy: %v", err)
	}
	if occupancy.QuotationID != quotation.ID {
		t.Errorf("occupancy quotation = %d; want %d", occupancy.QuotationID, quotation.ID)
	}
	if occupancy.ActivePO != nil {
		t.Errorf("free quotation reported active PO %+v; want none", occupancy.ActivePO)
	}

	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create PO draft: %v", err)
	}
	occupancy, err = database.GetPOSourceOccupancy(ctx, quotation.ID)
	if err != nil {
		t.Fatalf("get draft occupancy: %v", err)
	}
	if occupancy.ActivePO == nil {
		t.Fatal("occupied quotation reported no active PO")
	}
	if occupancy.ActivePO.ID != draft.ID || occupancy.ActivePO.State != PurchaseOrderStateDraft {
		t.Errorf("active PO = %+v; want draft %d", occupancy.ActivePO, draft.ID)
	}

	// A second issued revision is still the same Quotation, and a displayed
	// revision the PO did not start from does not change the answer.
	if _, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode: 2, CustomerName: "PT. Buyer", CustomerAddress: "Jl. Billing 1",
		RecipientName: "Budi Santoso", RecipientAddress: "Jl. Recipient 9",
		Subject: "Occupancy revision one", Incoterm: "CIF", ValidityDays: 30,
		PaymentTermDays: 45, LeadTimeDays: 10, TaxZone: "FTZ", TaxType: "Goods",
		Currency: "USD", RevisionNumber: 1, QuoteNumber: quotation.QuoteNumber,
		HTMLSnapshot: "<html>revision one</html>",
		Items: []RevisionItem{{
			ItemNo: 1, Description: "Priced widget", Qty: 2, UoM: "Pc",
			UnitPrice: 100, OriginalTotal: 200, MarginalizedUnitPrice: 100,
			MarginalizedTotal: 200, ListUnitPrice: 100,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal: 200, OriginalGrand: 200,
			MarginalizedSubtotal: 200, MarginalizedGrand: 200,
			MarginalizedGrandAfterTax: 200, CustomerGrandTotal: 200,
		},
	}); err != nil {
		t.Fatalf("seed second revision: %v", err)
	}
	occupancy, err = database.GetPOSourceOccupancy(ctx, quotation.ID)
	if err != nil {
		t.Fatalf("get occupancy at revision one: %v", err)
	}
	if occupancy.ActivePO == nil || occupancy.ActivePO.ID != draft.ID {
		t.Errorf("active PO after a second revision = %+v; want the same draft %d",
			occupancy.ActivePO, draft.ID)
	}

	// One quotation may now produce many POs across revisions. Drafts do not
	// reserve quantity, so the second create is allowed and the occupancy read
	// reports two advisory drafts with no saved record.
	second, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: 1,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create the second draft: %v", err)
	}
	occupancy, err = database.GetPOSourceOccupancy(ctx, quotation.ID)
	if err != nil {
		t.Fatalf("get multi-draft occupancy: %v", err)
	}
	if occupancy.SavedPOCount != 0 || occupancy.DraftPOCount != 2 {
		t.Errorf("occupancy counts = %d saved / %d draft; want 0/2",
			occupancy.SavedPOCount, occupancy.DraftPOCount)
	}
	if occupancy.AllBought {
		t.Error("two advisory drafts made the quotation look fully bought")
	}
	if occupancy.ActivePO == nil || occupancy.ActivePO.ID != second.ID ||
		occupancy.ActivePO.State != PurchaseOrderStateDraft {
		t.Errorf("active PO = %+v; want the newest draft %d", occupancy.ActivePO, second.ID)
	}

	// Reduce the first draft's item 1 to one unit so saving it leaves room for
	// a later order. The draft save rewrites the allocation too (#651).
	firstUpdate := PurchaseOrderUpdate{
		ID:                    draft.ID,
		BillingAddress:        draft.BillingAddress,
		DeliveryAddress:       draft.DeliveryAddress,
		ShipToCode:            draft.ShipToCode,
		ContactEmail:          draft.ContactEmail,
		ContactPhone:          draft.ContactPhone,
		PONumber:              draft.PONumber,
		Incoterms:             draft.Incoterms,
		PaymentTerms:          draft.PaymentTerms,
		PODate:                draft.PODate,
		RequestedDeliveryDate: draft.RequestedDeliveryDate,
		ReceivedDate:          &draft.ReceivedDate,
		BusinessCategory:      draft.BusinessCategory,
		Items:                 append([]PurchaseOrderItem(nil), draft.Items...),
		DiscountPercent:       draft.DiscountPercent,
		AdjustmentMinor:       draft.AdjustmentMinor,
		AdjustmentReason:      draft.AdjustmentReason,
		UpdatedBy:             user.ID,
	}
	firstUpdate.Items[0].QuantityScaled = QuantityScale
	if _, err := updateDraft(t, database, ctx, firstUpdate); err != nil {
		t.Fatalf("reduce the first draft: %v", err)
	}

	// A saved record consumes its allocation, but the quotation still has room:
	// AllBought stays false until every leaf is exhausted.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_orders
		 SET state = 'saved', po_number = 'PO-499', po_number_key = 'po-499',
		     saved_at = NOW(), saved_by = $2
		 WHERE id = $1`, database.configSchema()),
		draft.ID, user.ID); err != nil {
		t.Fatalf("mark saved: %v", err)
	}
	occupancy, err = database.GetPOSourceOccupancy(ctx, quotation.ID)
	if err != nil {
		t.Fatalf("get saved occupancy: %v", err)
	}
	if occupancy.SavedPOCount != 1 || occupancy.DraftPOCount != 1 {
		t.Errorf("occupancy counts = %d saved / %d draft; want 1/1",
			occupancy.SavedPOCount, occupancy.DraftPOCount)
	}
	if occupancy.AllBought {
		t.Error("one saved partial PO made the quotation fully bought")
	}

	// With two saved records the newest is still offered for reopening and the
	// bought count is what the detail page labels.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.purchase_orders
		 SET state = 'saved', po_number = 'PO-499-B', po_number_key = 'po-499-b',
		     saved_at = NOW(), saved_by = $2
		 WHERE id = $1`, database.configSchema()),
		second.ID, user.ID); err != nil {
		t.Fatalf("mark the second saved: %v", err)
	}
	occupancy, err = database.GetPOSourceOccupancy(ctx, quotation.ID)
	if err != nil {
		t.Fatalf("get two-saved occupancy: %v", err)
	}
	if occupancy.SavedPOCount != 2 || occupancy.DraftPOCount != 0 {
		t.Errorf("occupancy counts = %d saved / %d draft; want 2/0",
			occupancy.SavedPOCount, occupancy.DraftPOCount)
	}
	if occupancy.ActivePO == nil || occupancy.ActivePO.ID != second.ID ||
		occupancy.ActivePO.State != PurchaseOrderStateSaved {
		t.Errorf("active PO = %+v; want the newest saved %d", occupancy.ActivePO, second.ID)
	}

}

// TestPOSourceOccupancyIsOrganizationScopedAndNamesAMissingQuotation keeps the
// narrow read inside the same tenancy boundary as every other PO read. A second
// org sees the Quotation (quotations carry no org column) but never org A's
// active record; a Quotation id that does not exist is a 404, not a free row.
func TestPOSourceOccupancyIsOrganizationScopedAndNamesAMissingQuotation(t *testing.T) {
	database := openTestSchema(t, "vera_po_source_occupancy_org")
	quotation := seedPOQuotation(t, database, "PT. Buyer", "Scoped occupancy")
	user := poTestUser(t, database)
	ctxA := poContext(t, database)
	if _, err := database.CreatePurchaseOrderDraft(ctxA, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	}); err != nil {
		t.Fatalf("create in org A: %v", err)
	}

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var orgB string
	if err := pool.QueryRow(context.Background(), fmt.Sprintf(
		`INSERT INTO %s.organizations (name) VALUES ('Other Org') RETURNING id::text`,
		database.configSchema())).Scan(&orgB); err != nil {
		t.Fatalf("insert second org: %v", err)
	}
	ctxB := ContextWithOrg(context.Background(), orgB)

	occupancy, err := database.GetPOSourceOccupancy(ctxB, quotation.ID)
	if err != nil {
		t.Fatalf("get org B occupancy: %v", err)
	}
	if occupancy.ActivePO != nil {
		t.Errorf("org B saw org A's active PO: %+v", occupancy.ActivePO)
	}
	if occupancy.QuotationID != quotation.ID {
		t.Errorf("org B occupancy quotation = %d; want %d", occupancy.QuotationID, quotation.ID)
	}

	if _, err := database.GetPOSourceOccupancy(ctxA, quotation.ID+9999); !errors.Is(err, ErrPOSourceNotFound) {
		t.Errorf("unknown quotation error = %v; want ErrPOSourceNotFound", err)
	}
}

// Issue #651: migration 053 removes the one-active-PO unique index and adds the
// lookup the selector and availability read use. A fresh schema must carry the
// replacement and not the old rule.
func TestMigration053ReplacesTheOneActivePOIndex(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_index")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var oldExists, lookupExists bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = $1 AND indexname = 'purchase_orders_active_quotation_idx'
		)`, database.config.Schema).Scan(&oldExists); err != nil {
		t.Fatalf("check the old index: %v", err)
	}
	if oldExists {
		t.Error("migration 053 left the one-active-PO unique index in place")
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = $1 AND indexname = 'purchase_orders_source_lookup_idx'
		)`, database.config.Schema).Scan(&lookupExists); err != nil {
		t.Fatalf("check the lookup index: %v", err)
	}
	if !lookupExists {
		t.Error("migration 053 did not create the source lookup index")
	}
}
