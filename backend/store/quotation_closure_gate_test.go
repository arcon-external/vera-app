package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Issue #656 phase B: closing a quotation must stop new work at the store seam,
// not only in the selector. A stale draft, an old tab or a direct caller all
// reach these functions, and a close that the next create/finalize/export
// silently undoes is not a close.

// TestClosedQuotationRefusesADraftAndAFinalize covers the two ordering paths:
// a new draft cannot be opened, and a draft opened before the close cannot be
// finalized. Reopening restores both.
func TestClosedQuotationRefusesADraftAndAFinalize(t *testing.T) {
	database := openTestSchema(t, "vera_closure_gate_orders")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedVarianceQuotation(t, database, 2, "PT. Closed Gate", "Closure gate source")

	// Three of five units bought, so the quotation still has remaining leaves
	// and a second draft is a normal, allowed thing before the close.
	finalizeReportPO(t, database, ctx, user, quotation, "PO-GATE-3", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].QuantityScaled = 3 * QuantityScale
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:quantity"),
			}
		})

	stale, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:      quotation.ID,
		RevisionNumber:   quotation.RevisionNumber,
		BusinessCategory: PurchaseOrderBusinessCategoryFm,
		CreatedBy:        user.ID,
	})
	if err != nil {
		t.Fatalf("create the draft that will outlive the close: %v", err)
	}

	if _, err := database.CloseQuotation(ctx, quotation.ID,
		"customer cancelled the remainder", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}

	if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:      quotation.ID,
		RevisionNumber:   quotation.RevisionNumber,
		BusinessCategory: PurchaseOrderBusinessCategoryFm,
		CreatedBy:        user.ID,
	}); !errors.Is(err, ErrQuotationClosed) {
		t.Fatalf("create against a closed quotation error = %v; want ErrQuotationClosed", err)
	}

	finalize := finalizeInput(stale, user, "PO-GATE-STALE")
	finalize.Decisions = []PurchaseOrderDecision{varianceDecision("line:1:quantity")}
	if _, err := database.FinalizePurchaseOrder(ctx, finalize); !errors.Is(err, ErrQuotationClosed) {
		t.Fatalf("finalize a pre-close draft error = %v; want ErrQuotationClosed", err)
	}

	if _, err := database.ReopenQuotation(ctx, quotation.ID, admin.ID); err != nil {
		t.Fatalf("ReopenQuotation: %v", err)
	}
	if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:      quotation.ID,
		RevisionNumber:   quotation.RevisionNumber,
		BusinessCategory: PurchaseOrderBusinessCategoryFm,
		CreatedBy:        user.ID,
	}); err != nil {
		t.Fatalf("create after reopen: %v", err)
	}
	saved, err := database.FinalizePurchaseOrder(ctx, finalize)
	if err != nil {
		t.Fatalf("finalize after reopen: %v", err)
	}
	if saved.State != PurchaseOrderStateSaved {
		t.Errorf("saved state = %q; want %q", saved.State, PurchaseOrderStateSaved)
	}
}

// TestClosedQuotationRefusesANewIssuedRevisionUntilReopen pins the revision
// side of the same decision: an export would otherwise change the closed
// population under the report.
func TestClosedQuotationRefusesANewIssuedRevisionUntilReopen(t *testing.T) {
	database := openTestSchema(t, "vera_closure_gate_revisions")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedPOListQuotation(t, database, 2, "PT. Closed Revision", "Closure revision source")

	if _, err := database.CloseQuotation(ctx, quotation.ID,
		"no further revisions", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	next := RevisionDraft{
		QuoteNumber:      quotation.QuoteNumber,
		RevisionNumber:   quotation.RevisionNumber + 1,
		CustomerCode:     2,
		CustomerName:     "PT. Closed Revision",
		CustomerAddress:  "Jl. Closed 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          "Closure revision source",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     10,
		TaxZone:          "NonFTZ",
		TaxCategory:      "SupplyInstall",
		Currency:         "IDR",
		HTMLSnapshot:     "<html>snapshot</html>",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "List item",
			Qty:                   1,
			UoM:                   "Lot",
			UnitPrice:             1000,
			OriginalTotal:         1000,
			MarginalizedUnitPrice: 1000,
			MarginalizedTotal:     1000,
			ListUnitPrice:         1000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:          1000,
			OriginalGrand:             1000,
			MarginalizedSubtotal:      1000,
			MarginalizedGrand:         1000,
			MarginalizedGrandAfterTax: 1000,
			CustomerGrandTotal:        1000,
		},
	}
	if _, err := database.SaveRevision(ctx, next); !errors.Is(err, ErrQuotationClosed) {
		t.Fatalf("revision on a closed quotation error = %v; want ErrQuotationClosed", err)
	}

	if _, err := database.ReopenQuotation(ctx, quotation.ID, admin.ID); err != nil {
		t.Fatalf("ReopenQuotation: %v", err)
	}
	if _, err := database.SaveRevision(ctx, next); err != nil {
		t.Fatalf("revision after reopen: %v", err)
	}
}

// TestCloseQuotationReasonIsBoundedAt500Runes keeps the reason auditable and
// storable. The boundary is characters, not bytes, and 501 is refused before
// the database's own CHECK ever sees it.
func TestCloseQuotationReasonIsBoundedAt500Runes(t *testing.T) {
	database := openTestSchema(t, "vera_closure_gate_reason")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedPOListQuotation(t, database, 2, "PT. Reason Bound", "Reason bound source")

	atLimit := strings.Repeat("界", MaxQuotationCloseReasonLength)
	if got := utf8.RuneCountInString(atLimit); got != MaxQuotationCloseReasonLength {
		t.Fatalf("fixture has %d runes; want %d", got, MaxQuotationCloseReasonLength)
	}
	closed, err := database.CloseQuotation(ctx, quotation.ID, atLimit, admin.ID)
	if err != nil {
		t.Fatalf("close at the character boundary: %v", err)
	}
	if closed.Reason != atLimit {
		t.Errorf("stored reason changed: got %d runes, want %d",
			utf8.RuneCountInString(closed.Reason), MaxQuotationCloseReasonLength)
	}

	second := seedPOListQuotation(t, database, 3, "PT. Reason Bound Two", "Reason bound source")
	overLimit := strings.Repeat("界", MaxQuotationCloseReasonLength+1)
	if _, err := database.CloseQuotation(ctx, second.ID, overLimit, admin.ID); !errors.Is(err, ErrQuotationCloseReasonTooLong) {
		t.Fatalf("501-rune reason error = %v; want ErrQuotationCloseReasonTooLong", err)
	}
	record, err := database.GetQuotation(ctx, second.ID, nil)
	if err != nil {
		t.Fatalf("GetQuotation: %v", err)
	}
	if record.ClosedAt != nil {
		t.Error("the refused close wrote closed_at anyway")
	}
	if events := closureEvents(t, database, second.ID); len(events) != 0 {
		t.Errorf("the refused close wrote %d audit events; want none", len(events))
	}
}

// TestQuotationDetailReportsClosureAndLatestExpiry is the detail seam the UI
// reads: closure comes from the quotation row, expiry from the latest issued
// revision even when an older one is requested.
func TestQuotationDetailReportsClosureAndLatestExpiry(t *testing.T) {
	database := openTestSchema(t, "vera_closure_gate_detail")
	ctx := poContext(t, database)
	admin := quotationClosureAdmin(t, database)
	quotation := seedPOListQuotation(t, database, 2, "PT. Detail Closure", "Detail closure source")

	// Make the latest revision old enough to have expired while the requested
	// older revision was issued today: the answer must follow the latest one.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE "+database.configSchema()+
		".quotation_revisions SET created_at = $1, validity_days = 30 WHERE quotation_id = $2",
		time.Now().AddDate(0, -2, 0), quotation.ID); err != nil {
		t.Fatalf("age the revision: %v", err)
	}
	if _, err := database.SaveRevision(ctx, RevisionDraft{
		QuoteNumber:      quotation.QuoteNumber,
		RevisionNumber:   quotation.RevisionNumber + 1,
		CustomerCode:     2,
		CustomerName:     "PT. Detail Closure",
		CustomerAddress:  "Jl. Detail 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          "Detail closure source",
		Incoterm:         "EXW",
		ValidityDays:     30,
		PaymentTermDays:  30,
		LeadTimeDays:     10,
		TaxZone:          "NonFTZ",
		TaxCategory:      "SupplyInstall",
		Currency:         "IDR",
		HTMLSnapshot:     "<html>snapshot</html>",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "List item",
			Qty:                   1,
			UoM:                   "Lot",
			UnitPrice:             1000,
			OriginalTotal:         1000,
			MarginalizedUnitPrice: 1000,
			MarginalizedTotal:     1000,
			ListUnitPrice:         1000,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:          1000,
			OriginalGrand:             1000,
			MarginalizedSubtotal:      1000,
			MarginalizedGrand:         1000,
			MarginalizedGrandAfterTax: 1000,
			CustomerGrandTotal:        1000,
		},
	}); err != nil {
		t.Fatalf("add the fresh revision: %v", err)
	}

	older := int16(0)
	record, err := database.GetQuotation(ctx, quotation.ID, &older)
	if err != nil {
		t.Fatalf("GetQuotation older revision: %v", err)
	}
	if record.ValidUntil == nil {
		t.Fatal("detail has no ValidUntil; the header badge needs it")
	}
	if record.ExpiredUnclosed {
		t.Error("detail reports expired from the older revision; expiry must follow the latest")
	}

	if _, err := database.CloseQuotation(ctx, quotation.ID,
		"customer went quiet", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	record, err = database.GetQuotation(ctx, quotation.ID, &older)
	if err != nil {
		t.Fatalf("GetQuotation after close: %v", err)
	}
	if record.ClosedAt == nil || record.ClosedReason != "customer went quiet" ||
		record.ClosedByName != admin.DisplayName {
		t.Errorf("detail closure = %v/%q/%q; want the stored close",
			record.ClosedAt, record.ClosedReason, record.ClosedByName)
	}
	if record.ExpiredUnclosed {
		t.Error("a closed quotation reports expiredUnclosed; closure wins")
	}
}

// TestListPOSourceQuotationsKeepsAClosedRowUnderTheUsedFilter pins the visible
// half of the selector rule (#656): "Hide quotations already used" hides a
// fully bought open quotation, but a closed one stays on the page as a
// historical record. Without the server-side clause the client's filter would
// never see the row it promises to show disabled.
func TestListPOSourceQuotationsKeepsAClosedRowUnderTheUsedFilter(t *testing.T) {
	database := openTestSchema(t, "vera_closure_gate_selector")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := quotationClosureAdmin(t, database)
	closedRow := seedVarianceQuotation(t, database, 2, "PT. Closed Filter", "Closed filter source")
	openRow := seedVarianceQuotation(t, database, 3, "PT. Open Filter", "Open filter source")

	for _, quotation := range []QuotationRecord{closedRow, openRow} {
		finalizeReportPO(t, database, ctx, user, quotation, "PO-FILTER-"+quotation.QuoteNumber, "2026-03-10",
			PurchaseOrderBusinessCategoryFm, nil)
	}

	listed := func() map[int64]POSourceQuotation {
		rows, _, err := database.ListPOSourceQuotations(ctx, 1, "", true, 0)
		if err != nil {
			t.Fatalf("ListPOSourceQuotations: %v", err)
		}
		found := make(map[int64]POSourceQuotation, len(rows))
		for _, row := range rows {
			found[row.ID] = row
		}
		return found
	}

	if rows := listed(); len(rows) != 0 {
		t.Fatalf("hideUsed listed %d fully bought quotations; want none: %+v", len(rows), rows)
	}

	if _, err := database.CloseQuotation(ctx, closedRow.ID,
		"customer cancelled the remainder", admin.ID); err != nil {
		t.Fatalf("CloseQuotation: %v", err)
	}
	rows := listed()
	if row, ok := rows[closedRow.ID]; !ok || !row.Closed {
		t.Errorf("hideUsed dropped the closed quotation (found=%v closed=%v); want it visible",
			ok, row.Closed)
	}
	if _, ok := rows[openRow.ID]; ok {
		t.Error("hideUsed kept the fully bought open quotation")
	}

	if _, err := database.ReopenQuotation(ctx, closedRow.ID, admin.ID); err != nil {
		t.Fatalf("ReopenQuotation: %v", err)
	}
	if rows := listed(); len(rows) != 0 {
		t.Errorf("hideUsed listed %d quotations after reopen; want the fully bought row hidden again", len(rows))
	}
}
