package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// Issue #471: the final save. These tests drive the real store against a
// disposable schema: the minimum a saved record must carry, per-customer
// uniqueness (including a real concurrent race), the explicit difference
// decisions, the printed-total explanation, the no-PDF confirmation, and the
// promise that a final save never mutates the source Quotation revision.

// seedFinalizeQuotation writes a small, arithmetically consistent issued
// revision in FTZ so the PO carries no PPN: two lines, one priced (qty 2 at
// 100 = 200) and one free (qty 1 at 0). The free line lets a test omit a
// source line without changing the printed total.
func seedFinalizeQuotation(t *testing.T, database *Store, customerCode int, customerName, subject string) QuotationRecord {
	t.Helper()
	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     int16(customerCode),
		CustomerName:     customerName,
		CustomerAddress:  "Jl. Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          subject,
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		DiscountPercent:  0,
		TaxZone:          "FTZ",
		TaxType:          "Goods",
		Currency:         "USD",
		HTMLSnapshot:     "<html>finalize snapshot</html>",
		Items: []RevisionItem{
			{
				ItemNo:                1,
				Description:           "Priced widget",
				Qty:                   2,
				UoM:                   "Pc",
				UnitPrice:             100,
				MarginPercent:         0,
				MarginAmount:          0,
				OriginalTotal:         200,
				MarginalizedUnitPrice: 100,
				MarginalizedTotal:     200,
				ListUnitPrice:         100,
			},
			{
				ItemNo:                3,
				Description:           "Zero-value line",
				Qty:                   1,
				UoM:                   "Lot",
				UnitPrice:             0,
				MarginPercent:         0,
				MarginAmount:          0,
				OriginalTotal:         0,
				MarginalizedUnitPrice: 0,
				MarginalizedTotal:     0,
				ListUnitPrice:         0,
			},
		},
		Totals: RevisionTotals{
			OriginalSubtotal:          200,
			OriginalGrand:             200,
			MarginalizedSubtotal:      200,
			MarginalizedGrand:         200,
			MarginalizedGrandAfterTax: 200,
			CustomerGrandTotal:        200,
		},
	})
	if err != nil {
		t.Fatalf("seed finalize quotation %q: %v", subject, err)
	}
	return record
}

func createFinalizeDraft(t *testing.T, database *Store, ctx context.Context, user AuthUser, quotation QuotationRecord) PurchaseOrder {
	t.Helper()
	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	return draft
}

// finalizeInput builds the payload a final save would carry from the stored
// draft: every editable field as stored, in line order, plus an explicit
// number. Tests then change only the field they are exercising.
func finalizeInput(draft PurchaseOrder, user AuthUser, number string) PurchaseOrderFinalize {
	items := make([]PurchaseOrderItem, len(draft.Items))
	for index, item := range draft.Items {
		item.LineNumber = index + 1
		items[index] = item
	}
	return PurchaseOrderFinalize{
		PurchaseOrderUpdate: PurchaseOrderUpdate{
			ID:                    draft.ID,
			BillingAddress:        draft.BillingAddress,
			DeliveryAddress:       draft.DeliveryAddress,
			ShipToCode:            draft.ShipToCode,
			ContactEmail:          draft.ContactEmail,
			ContactPhone:          draft.ContactPhone,
			PONumber:              number,
			PODate:                draft.PODate,
			ReceivedDate:          &draft.ReceivedDate,
			RequestedDeliveryDate: draft.RequestedDeliveryDate,
			PaymentTerms:          draft.PaymentTerms,
			Incoterms:             draft.Incoterms,
			UpdatedBy:             user.ID,
			ExpectedVersion:       draft.Version,
			Items:                 items,
			DiscountPercent:       draft.DiscountPercent,
			AdjustmentMinor:       draft.AdjustmentMinor,
			AdjustmentReason:      draft.AdjustmentReason,
		},
		DocumentConfirmed: true,
	}
}

// revisionFingerprint composes the source revision's stored bytes. A final save
// must not change any of them.
func revisionFingerprint(t *testing.T, database *Store, ctx context.Context, quotationID int64, revision int16) string {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var fingerprint string
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT md5(r::text)
		       || ':' || COALESCE((
		           SELECT md5(string_agg(i::text, ',' ORDER BY i.id))
		           FROM %s.quotation_revision_items i WHERE i.revision_id = r.id), '')
		       || ':' || COALESCE((
		           SELECT d.sha256 FROM %s.quotation_documents d WHERE d.revision_id = r.id), '')
		FROM %s.quotation_revisions r
		WHERE r.quotation_id = $1 AND r.revision_number = $2`,
		database.configSchema(), database.configSchema(), database.configSchema()),
		quotationID, revision).Scan(&fingerprint); err != nil {
		t.Fatalf("fingerprint revision: %v", err)
	}
	return fingerprint
}

func TestFinalizePurchaseOrderSavesTheRecordAndKeepsTheQuotation(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Finalize")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	before := revisionFingerprint(t, database, ctx, quotation.ID, quotation.RevisionNumber)

	input := finalizeInput(draft, user, "  PO-471  ")
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	if saved.State != PurchaseOrderStateSaved {
		t.Errorf("state = %q; want saved", saved.State)
	}
	if saved.PONumber != "PO-471" {
		t.Errorf("number = %q; want the trimmed PO-471", saved.PONumber)
	}
	if saved.SavedAt == nil || saved.SavedBy == nil || *saved.SavedBy != user.ID {
		t.Errorf("saved actor/time = %v/%v; want %d with a time", saved.SavedAt, saved.SavedBy, user.ID)
	}
	if !saved.PDFAbsentConfirmed {
		t.Error("the no-PDF confirmation was not recorded")
	}
	// 2 x 100 with no tax, no discount and no adjustment: the calculated total
	// equals the document's printed total.
	if saved.GrandTotalMinor != 200 {
		t.Errorf("grand = %d; want the printed 200", saved.GrandTotalMinor)
	}

	// The source revision is bit-for-bit unchanged.
	if after := revisionFingerprint(t, database, ctx, quotation.ID, quotation.RevisionNumber); after != before {
		t.Errorf("the final save rewrote the quotation revision:\nbefore %s\nafter  %s", before, after)
	}

	// Saved is one-way: neither another draft write nor another final save is
	// allowed.
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: input.Items,
	}); !errors.Is(err, ErrPurchaseOrderNotDraft) {
		t.Errorf("update after save = %v; want ErrPurchaseOrderNotDraft", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderNotDraft) {
		t.Errorf("second finalize = %v; want ErrPurchaseOrderNotDraft", err)
	}
}

func TestFinalizePurchaseOrderRequiresTheFinalSaveMinimum(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_min")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Minimum")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	blankNumber := finalizeInput(draft, user, "   \t ")
	if _, err := database.FinalizePurchaseOrder(ctx, blankNumber); !errors.Is(err, ErrPurchaseOrderIncomplete) {
		t.Fatalf("blank number error = %v; want ErrPurchaseOrderIncomplete", err)
	}

	// Zero lines: a draft may reach that state, a saved record may not.
	emptyDraft, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, PONumber: "PO-EMPTY", Items: []PurchaseOrderItem{},
	})
	if err != nil {
		t.Fatalf("empty draft update: %v", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, finalizeInput(emptyDraft, user, "PO-EMPTY")); !errors.Is(err, ErrPurchaseOrderIncomplete) {
		t.Fatalf("zero lines error = %v; want ErrPurchaseOrderIncomplete", err)
	}

	// The record is still a draft: neither refusal wrote anything.
	reloaded, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.State != PurchaseOrderStateDraft || reloaded.SavedAt != nil {
		t.Errorf("a refused final save changed the record: %+v", reloaded)
	}
}

func TestFinalizePurchaseOrderRequiresAUniqueNumberPerCustomer(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_number")
	firstQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "First")
	secondQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Second")
	otherQuote := seedFinalizeQuotation(t, database, 3, "PT. Other Buyer", "Other customer")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	first := createFinalizeDraft(t, database, ctx, user, firstQuote)
	second := createFinalizeDraft(t, database, ctx, user, secondQuote)
	other := createFinalizeDraft(t, database, ctx, user, otherQuote)

	// Drafts may share a number, including case and whitespace variants, and
	// carry no comparison key until the final save.
	secondSavedDraft, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: second.ID, UpdatedBy: user.ID, PONumber: "PO-471", Items: second.Items,
	})
	if err != nil {
		t.Fatalf("draft number: %v", err)
	}
	savedFirst, err := database.FinalizePurchaseOrder(ctx, finalizeInput(first, user, " PO-471 "))
	if err != nil {
		t.Fatalf("first finalize: %v", err)
	}
	if savedFirst.PONumber != "PO-471" {
		t.Fatalf("first number = %q", savedFirst.PONumber)
	}

	secondInput := finalizeInput(second, user, "po-471")
	secondInput.ExpectedVersion = secondSavedDraft.Version
	if _, err := database.FinalizePurchaseOrder(ctx, secondInput); !errors.Is(err, ErrPurchaseOrderNumberTaken) {
		t.Fatalf("case variant error = %v; want ErrPurchaseOrderNumberTaken", err)
	}
	// The refused record is still a draft with no key.
	reloaded, err := database.GetPurchaseOrder(ctx, second.ID)
	if err != nil {
		t.Fatalf("reload second: %v", err)
	}
	if reloaded.State != PurchaseOrderStateDraft || reloaded.SavedAt != nil {
		t.Errorf("refused duplicate changed the draft: %+v", reloaded)
	}

	// A different customer may reuse the number: uniqueness is per customer.
	if _, err := database.FinalizePurchaseOrder(ctx, finalizeInput(other, user, "PO-471")); err != nil {
		t.Fatalf("different customer finalize: %v", err)
	}
}

func TestFinalizePurchaseOrderNumberRaceHasOneWinner(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_race")
	firstQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Race one")
	secondQuote := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Race two")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	first := createFinalizeDraft(t, database, ctx, user, firstQuote)
	second := createFinalizeDraft(t, database, ctx, user, secondQuote)

	inputs := []PurchaseOrderFinalize{
		finalizeInput(first, user, "PO-RACE"),
		finalizeInput(second, user, "po-race"),
	}
	errs := make([]error, len(inputs))
	var wait sync.WaitGroup
	for index := range inputs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			// Both requests carry the same organization context, exactly as two
			// simultaneous API requests would.
			_, errs[index] = database.FinalizePurchaseOrder(ctx, inputs[index])
		}(index)
	}
	wait.Wait()

	successes, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrPurchaseOrderNumberTaken):
			refused++
		default:
			t.Fatalf("race error = %v; want a success or ErrPurchaseOrderNumberTaken", err)
		}
	}
	if successes != 1 || refused != 1 {
		t.Fatalf("race = %d successes / %d refusals; want exactly one winner", successes, refused)
	}

	// Exactly one saved row carries the number.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var saved int
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.purchase_orders WHERE state = 'saved' AND po_number_key = 'po-race'",
		database.configSchema())).Scan(&saved); err != nil {
		t.Fatalf("count saved: %v", err)
	}
	if saved != 1 {
		t.Errorf("saved rows with the key = %d; want 1", saved)
	}
}

func TestFinalizePurchaseOrderRequiresExplicitDifferenceChoices(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_diff")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Differences")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	// A changed payment-term wording is a difference, and no decision blocks
	// the save.
	changedTerms := finalizeInput(draft, user, "PO-DIFF")
	changedTerms.PaymentTerms = "30 days after delivery"
	if _, err := database.FinalizePurchaseOrder(ctx, changedTerms); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("no decision error = %v; want ErrPurchaseOrderUnresolvedDifference", err)
	}

	// Claiming the quotation while sending a different value is refused.
	lying := changedTerms
	lying.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoiceQuotation},
	}
	if _, err := database.FinalizePurchaseOrder(ctx, lying); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("quotation choice error = %v; want refusal", err)
	}

	// An explicit PO choice saves the wording change.
	explicit := changedTerms
	explicit.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
	}
	saved, err := database.FinalizePurchaseOrder(ctx, explicit)
	if err != nil {
		t.Fatalf("explicit choice finalize: %v", err)
	}
	if saved.PaymentTerms != "30 days after delivery" {
		t.Errorf("saved terms = %q; want the entered wording", saved.PaymentTerms)
	}
}

func TestFinalizePurchaseOrderRequiresMissingAndExtraAcknowledgement(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_lines")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	// Drop the free source line and add a zero-priced extra: the printed total
	// is unchanged, so only the missing/extra review is exercised.
	input := finalizeInput(draft, user, "PO-LINES")
	input.Items = []PurchaseOrderItem{{
		LineNumber:      1,
		QuotationItemNo: intPointer(1),
		Description:     "Priced widget",
		QuantityScaled:  20000,
		UoM:             "Pc",
		UnitPriceMinor:  100,
	}, {
		LineNumber:     2,
		Description:    "Freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 0,
	}}
	if _, err := database.FinalizePurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("unacknowledged lines error = %v; want refusal", err)
	}
	input.AcknowledgedMissing = []string{"3"}
	input.AcknowledgedExtra = []int{2}
	if _, err := database.FinalizePurchaseOrder(ctx, input); err != nil {
		t.Fatalf("acknowledged lines finalize: %v", err)
	}
}

func TestFinalizePurchaseOrderRefusesDuplicateMatches(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_dupe")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Duplicate")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	input := finalizeInput(draft, user, "PO-DUPE")
	input.Items = []PurchaseOrderItem{{
		LineNumber:      1,
		QuotationItemNo: intPointer(1),
		Description:     "Priced widget",
		QuantityScaled:  20000,
		UoM:             "Pc",
		UnitPriceMinor:  100,
	}, {
		LineNumber:      2,
		QuotationItemNo: intPointer(1),
		Description:     "Priced widget again",
		QuantityScaled:  10000,
		UoM:             "Pc",
		UnitPriceMinor:  0,
	}}
	input.AcknowledgedMissing = []string{"3"}
	if _, err := database.FinalizePurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("duplicate match error = %v; want refusal", err)
	}
}

// TestFinalizePurchaseOrderKeepsThePurchaseOrdersOwnTotal is the judge's
// regression for the monetary rule: the saved grand total belongs to the
// purchase order, not to the quotation. The quotation printed 200; the
// customer's order adds a 50 freight line the quotation never carried.
// Acknowledging that extra line is the whole rule -- no adjustment is needed to
// reverse the difference, and the saved total is 250.
func TestFinalizePurchaseOrderKeepsThePurchaseOrdersOwnTotal(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_own_total")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Own total")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	input := finalizeInput(draft, user, "PO-OWN-TOTAL")
	input.Items = append(input.Items, PurchaseOrderItem{
		LineNumber:     3,
		Description:    "Freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 50,
	})
	input.AcknowledgedExtra = []int{3}
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize with an acknowledged extra line: %v", err)
	}
	if saved.GrandTotalMinor != 250 {
		t.Errorf("saved grand = %d; want the purchase order's own 250", saved.GrandTotalMinor)
	}
	if saved.AdjustmentMinor != 0 || saved.AdjustmentReason != "" {
		t.Errorf("adjustment = %d/%q; want none",
			saved.AdjustmentMinor, saved.AdjustmentReason)
	}
}

// TestFinalizePurchaseOrderAcceptsAReasonedDocumentAdjustment pins the other
// half of the rule: an adjustment exists to reconcile what the customer's PO
// document actually printed. The order calculates 250 while the document says
// 260, so staff record +10 with a reason and the saved grand total is 260.
func TestFinalizePurchaseOrderAcceptsAReasonedDocumentAdjustment(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_document_adjustment")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Document adjustment")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	input := finalizeInput(draft, user, "PO-DOC-ADJUSTMENT")
	input.Items = append(input.Items, PurchaseOrderItem{
		LineNumber:     3,
		Description:    "Freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 50,
	})
	input.AcknowledgedExtra = []int{3}
	input.AdjustmentMinor = 10
	input.AdjustmentReason = "Customer PO document printed 260.00"
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize with a document adjustment: %v", err)
	}
	if saved.GrandTotalMinor != 260 || saved.AdjustmentMinor != 10 {
		t.Errorf("saved total = %d adjustment = %d; want 260 / 10",
			saved.GrandTotalMinor, saved.AdjustmentMinor)
	}
	if saved.AdjustmentReason != "Customer PO document printed 260.00" {
		t.Errorf("saved reason = %q", saved.AdjustmentReason)
	}
}

func TestFinalizePurchaseOrderRequiresTheNoPDFConfirmation(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_pdf")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "PDF")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	input := finalizeInput(draft, user, "PO-PDF")
	input.DocumentConfirmed = false
	if _, err := database.FinalizePurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderPDFConfirmationRequired) {
		t.Fatalf("unconfirmed error = %v; want ErrPurchaseOrderPDFConfirmationRequired", err)
	}
	stillDraft, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stillDraft.State != PurchaseOrderStateDraft || stillDraft.PDFAbsentConfirmed {
		t.Errorf("the unconfirmed save wrote something: %+v", stillDraft)
	}

	input.DocumentConfirmed = true
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("confirmed finalize: %v", err)
	}
	if !saved.PDFAbsentConfirmed {
		t.Error("the confirmation is not recorded on the saved record")
	}
}

func TestGetPurchaseOrderReviewReturnsTheExactSource(t *testing.T) {
	database := openTestSchema(t, "vera_po_review_load")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Review")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	review, err := database.GetPurchaseOrderReview(ctx, draft.ID)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if review.Order.ID != draft.ID || review.Order.State != PurchaseOrderStateDraft {
		t.Errorf("review order = %+v; want the draft", review.Order)
	}
	if review.Source.Currency != "USD" || review.Source.PaymentTerms != "45 days" ||
		review.Source.Incoterms != "CIF" {
		t.Errorf("source header = %q/%q/%q; want USD/45 days/CIF",
			review.Source.Currency, review.Source.PaymentTerms, review.Source.Incoterms)
	}
	if review.Source.PrintedGrandTotalMinor != 200 ||
		review.Source.PrintedSubtotalMinor != 200 ||
		review.Source.PrintedPpnMinor != 0 {
		t.Errorf("printed totals = %d/%d/%d; want 200/200/0",
			review.Source.PrintedSubtotalMinor, review.Source.PrintedPpnMinor,
			review.Source.PrintedGrandTotalMinor)
	}
	if len(review.Source.Lines) != 2 {
		t.Fatalf("source lines = %+v; want 2", review.Source.Lines)
	}
	if review.Source.Lines[0].Label() != "1" || review.Source.Lines[0].QuantityScaled != 20000 ||
		review.Source.Lines[0].UnitPriceMinor != 100 || review.Source.Lines[0].Description != "Priced widget" {
		t.Errorf("source line 1 = %+v", review.Source.Lines[0])
	}
	if review.Source.Lines[1].Label() != "3" || review.Source.Lines[1].UnitPriceMinor != 0 {
		t.Errorf("source line 3 = %+v", review.Source.Lines[1])
	}
	if review.DocumentState != PurchaseOrderDocumentNone {
		t.Errorf("document state = %q; want %q", review.DocumentState, PurchaseOrderDocumentNone)
	}

	// Another organization cannot read it.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var otherOrg string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"INSERT INTO %s.organizations (name) VALUES ('Review Other') RETURNING id::text",
		database.configSchema())).Scan(&otherOrg); err != nil {
		t.Fatalf("insert other org: %v", err)
	}
	if _, err := database.GetPurchaseOrderReview(
		ContextWithOrg(context.Background(), otherOrg), draft.ID); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("org B review = %v; want ErrPurchaseOrderNotFound", err)
	}
}

func TestFinalizePurchaseOrderIsOrganizationScoped(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_org")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Scoped")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var otherOrg string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		"INSERT INTO %s.organizations (name) VALUES ('Finalize Other') RETURNING id::text",
		database.configSchema())).Scan(&otherOrg); err != nil {
		t.Fatalf("insert other org: %v", err)
	}
	input := finalizeInput(draft, user, "PO-ORG")
	if _, err := database.FinalizePurchaseOrder(
		ContextWithOrg(context.Background(), otherOrg), input); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("org B finalize = %v; want ErrPurchaseOrderNotFound", err)
	}
	if reloaded, err := database.GetPurchaseOrder(ctx, draft.ID); err != nil ||
		reloaded.State != PurchaseOrderStateDraft {
		t.Errorf("a refused cross-org save changed the draft: %+v / %v", reloaded, err)
	}
}

func TestFinalizePurchaseOrderRefusesACorruptedSourceLink(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_corrupt")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Corrupt")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}

	// Currency is copied from the revision and is not editable; a row that
	// disagrees with its source cannot be compared as if the amounts were equal.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.purchase_orders SET currency = 'EUR' WHERE id = $1",
		database.configSchema()), draft.ID); err != nil {
		t.Fatalf("corrupt currency: %v", err)
	}
	// The row's version trigger moved the record forward, so the writer must
	// state the version it now carries.
	corrupted, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen corrupted record: %v", err)
	}
	if _, err := database.GetPurchaseOrderReview(ctx, draft.ID); !errors.Is(err, ErrPurchaseOrderSourceCurrencyMismatch) {
		t.Fatalf("review currency mismatch = %v; want ErrPurchaseOrderSourceCurrencyMismatch", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, finalizeInput(corrupted, user, "PO-EUR")); !errors.Is(err, ErrPurchaseOrderSourceCurrencyMismatch) {
		t.Fatalf("finalize currency mismatch = %v; want ErrPurchaseOrderSourceCurrencyMismatch", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.purchase_orders SET currency = 'USD' WHERE id = $1",
		database.configSchema()), draft.ID); err != nil {
		t.Fatalf("restore currency: %v", err)
	}

	// A revision that is no longer the record's source cannot be reviewed.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		"UPDATE %s.purchase_orders SET source_revision = 99 WHERE id = $1",
		database.configSchema()), draft.ID); err != nil {
		t.Fatalf("corrupt revision: %v", err)
	}
	// The currency restore and the revision change each moved the version on.
	missing, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen missing-revision record: %v", err)
	}
	if _, err := database.GetPurchaseOrderReview(ctx, draft.ID); !errors.Is(err, ErrPurchaseOrderSourceNotFound) {
		t.Fatalf("review missing revision = %v; want ErrPurchaseOrderSourceNotFound", err)
	}
	if _, err := database.FinalizePurchaseOrder(ctx, finalizeInput(missing, user, "PO-REV")); !errors.Is(err, ErrPurchaseOrderSourceNotFound) {
		t.Fatalf("finalize missing revision = %v; want ErrPurchaseOrderSourceNotFound", err)
	}
}

func TestPurchaseOrderDraftCopiesTheSourceRowAndWholeDiscounts(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_seed")
	record, err := database.SaveRevision(context.Background(), RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "PT. Buyer",
		CustomerAddress:  "Jl. Billing 1",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Recipient 9",
		Subject:          "Discounted source",
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		DiscountPercent:  10,
		TaxZone:          "FTZ",
		TaxType:          "Goods",
		Currency:         "USD",
		HTMLSnapshot:     "<html>discount snapshot</html>",
		Items: []RevisionItem{{
			ItemNo:                1,
			Description:           "Discounted widget",
			Qty:                   2,
			UoM:                   "Pc",
			UnitPrice:             100,
			MarginPercent:         0,
			MarginAmount:          0,
			OriginalTotal:         200,
			MarginalizedUnitPrice: 100,
			MarginalizedTotal:     200,
			ListUnitPrice:         120,
			DiscountEnabled:       true,
			DiscountPercent:       5,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal:          200,
			OriginalGrand:             200,
			MarginalizedSubtotal:      240,
			MarginalizedDiscount:      24,
			MarginalizedGrand:         216,
			MarginalizedGrandAfterTax: 216,
			CustomerGrandTotal:        216,
		},
	})
	if err != nil {
		t.Fatalf("seed discounted quotation: %v", err)
	}
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, record)

	if draft.DiscountPercent != 10 {
		t.Errorf("whole-PO discount = %d; want the source's 10", draft.DiscountPercent)
	}
	if len(draft.Items) != 1 {
		t.Fatalf("lines = %d; want 1", len(draft.Items))
	}
	if !draft.Items[0].DiscountEnabled || draft.Items[0].DiscountPercent != 5 {
		t.Errorf("row discount = %v/%d; want enabled 5%%",
			draft.Items[0].DiscountEnabled, draft.Items[0].DiscountPercent)
	}
	if draft.Items[0].UnitPriceMinor != 120 {
		t.Errorf("unit price = %d; want the printed list 120", draft.Items[0].UnitPriceMinor)
	}
}

func TestFinalizePurchaseOrderPersistsCorrectedMatches(t *testing.T) {
	database := openTestSchema(t, "vera_po_finalize_match")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Matches")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	// The customer's document order is changed: line 1 now follows the free
	// source line and line 2 the priced one. The customer line numbers stay
	// 1 and 2; only the match references move, which is the correction this
	// slice exists to persist.
	one, three := 1, 3
	input := finalizeInput(draft, user, "PO-MATCH")
	input.Items = []PurchaseOrderItem{
		{
			LineNumber:      1,
			QuotationItemNo: &three,
			Description:     "Zero-value line",
			QuantityScaled:  10000,
			UoM:             "Lot",
			UnitPriceMinor:  0,
		},
		{
			LineNumber:      2,
			QuotationItemNo: &one,
			Description:     "Priced widget",
			QuantityScaled:  20000,
			UoM:             "Pc",
			UnitPriceMinor:  100,
		},
	}
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize corrected matches: %v", err)
	}
	if len(saved.Items) != 2 {
		t.Fatalf("saved lines = %d; want 2", len(saved.Items))
	}
	if saved.Items[0].LineNumber != 1 || saved.Items[0].QuotationItemNo == nil ||
		*saved.Items[0].QuotationItemNo != 3 || saved.Items[0].Description != "Zero-value line" {
		t.Errorf("saved line 1 = %+v; want line 1 matched to item 3", saved.Items[0])
	}
	if saved.Items[1].LineNumber != 2 || saved.Items[1].QuotationItemNo == nil ||
		*saved.Items[1].QuotationItemNo != 1 || saved.Items[1].Description != "Priced widget" {
		t.Errorf("saved line 2 = %+v; want line 2 matched to item 1", saved.Items[1])
	}
}
