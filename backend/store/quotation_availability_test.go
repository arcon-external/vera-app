package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Issue #651: one quotation may produce many purchase orders. Bought state is
// derived from saved allocations across revisions; drafts are advisory. These
// tests drive the normal workspace flow: a partial first PO leaves a ghost, a
// second PO exhausts it, and the server refuses a third.

func availabilityFor(t *testing.T, database *Store, ctx context.Context, quotationID int64) QuotationAvailability {
	t.Helper()
	availability, err := database.GetQuotationAvailability(ctx, quotationID)
	if err != nil {
		t.Fatalf("GetQuotationAvailability(%d): %v", quotationID, err)
	}
	return availability
}

func availabilityItem(t *testing.T, availability QuotationAvailability, itemNo int) QuotationAvailabilityItem {
	t.Helper()
	for _, item := range availability.Items {
		if item.QuotationItemNo == itemNo && item.QuotationSubNo == nil {
			return item
		}
	}
	t.Fatalf("availability has no item %d: %+v", itemNo, availability.Items)
	return QuotationAvailabilityItem{}
}

// finalizeAvailabilityQuantity creates a PO for the whole quotation, changes its
// single line to quantity, and finalizes it. It is the ordinary partial-order
// path: staff edit the prefilled quantity before the final save.
func finalizeAvailabilityQuantity(
	t *testing.T,
	database *Store,
	ctx context.Context,
	user AuthUser,
	quotation QuotationRecord,
	number string,
	quantity int64,
) PurchaseOrder {
	t.Helper()
	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create draft for %s: %v", number, err)
	}
	update := PurchaseOrderUpdate{
		ID:               draft.ID,
		BillingAddress:   draft.BillingAddress,
		DeliveryAddress:  draft.DeliveryAddress,
		ShipToCode:       draft.ShipToCode,
		ContactEmail:     draft.ContactEmail,
		ContactPhone:     draft.ContactPhone,
		PONumber:         draft.PONumber,
		Incoterms:        draft.Incoterms,
		PaymentTerms:     draft.PaymentTerms,
		PODate:           draft.PODate,
		ReceivedDate:     &draft.ReceivedDate,
		BusinessCategory: PurchaseOrderBusinessCategoryFm,
		Items:            append([]PurchaseOrderItem(nil), draft.Items...),
		UpdatedBy:        user.ID,
		ExpectedVersion:  draft.Version,
	}
	update.Items[0].QuantityScaled = quantity
	ready, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("set the %s quantity: %v", number, err)
	}
	input := finalizeInput(ready, user, number)
	input.BusinessCategory = PurchaseOrderBusinessCategoryFm
	if quantity != draft.Items[0].QuantityScaled {
		input.Decisions = []PurchaseOrderDecision{{
			Key: "line:1:quantity", Choice: PurchaseOrderChoicePO,
		}}
	}
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize %s: %v", number, err)
	}
	return saved
}

func TestQuotationAvailabilityOneToManyBoughtState(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_many")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Many POs", "Ten unit source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Ten unit line", UoM: "Pc", Qty: 10, Cost: 100, Sell: 250,
		}})

	fresh := availabilityFor(t, database, ctx, quotation.ID)
	item := availabilityItem(t, fresh, 1)
	if item.QuotedQuantityScaled != 100_000 || item.RemainingQuantityScaled != 100_000 ||
		item.BoughtQuantityScaled != 0 || fresh.AllBought {
		t.Fatalf("fresh availability = %+v / allBought=%v; want all ten remaining",
			item, fresh.AllBought)
	}

	// First PO buys four of ten.
	finalizeAvailabilityQuantity(t, database, ctx, user, quotation, "PO-651-A", 4*QuantityScale)
	afterFirst := availabilityFor(t, database, ctx, quotation.ID)
	item = availabilityItem(t, afterFirst, 1)
	if item.BoughtQuantityScaled != 4*QuantityScale {
		t.Errorf("bought after the first PO = %d; want 4 units", item.BoughtQuantityScaled)
	}
	if item.RemainingQuantityScaled != 6*QuantityScale {
		t.Errorf("remaining after four of ten = %d; want the 6-unit ghost",
			item.RemainingQuantityScaled)
	}
	if afterFirst.AllBought || afterFirst.SavedPOCount != 1 {
		t.Errorf("after the first PO = allBought %v / saved %d; want a live partial quotation",
			afterFirst.AllBought, afterFirst.SavedPOCount)
	}

	// Second PO takes the remaining six.
	second, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create the second PO from the six-unit ghost: %v", err)
	}
	if second.Items[0].QuantityScaled != 6*QuantityScale {
		t.Errorf("second draft quantity = %d; want the six remaining units",
			second.Items[0].QuantityScaled)
	}
	secondInput := finalizeInput(second, user, "PO-651-B")
	secondInput.Decisions = []PurchaseOrderDecision{{
		Key: "line:1:quantity", Choice: PurchaseOrderChoicePO,
	}}
	if _, err := database.FinalizePurchaseOrder(ctx, secondInput); err != nil {
		t.Fatalf("finalize the second PO: %v", err)
	}
	afterSecond := availabilityFor(t, database, ctx, quotation.ID)
	item = availabilityItem(t, afterSecond, 1)
	if item.BoughtQuantityScaled != 10*QuantityScale || item.RemainingQuantityScaled != 0 {
		t.Errorf("after ten of ten = bought %d / remaining %d; want the quotation exhausted",
			item.BoughtQuantityScaled, item.RemainingQuantityScaled)
	}
	if !afterSecond.AllBought || afterSecond.SavedPOCount != 2 {
		t.Errorf("exhausted quotation = allBought %v / saved %d; want all bought by two POs",
			afterSecond.AllBought, afterSecond.SavedPOCount)
	}

	// The third create is refused by the server, not just the UI.
	if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	}); !errors.Is(err, ErrQuotationFullyBought) {
		t.Fatalf("create on an exhausted quotation = %v; want ErrQuotationFullyBought", err)
	}
}

func TestQuotationAvailabilityDraftsAreAdvisory(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_drafts")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Drafts", "Draft advisory",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Draft line", UoM: "Pc", Qty: 10, Cost: 100, Sell: 250,
		}})

	for index := 0; index < 2; index++ {
		if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
			QuotationID:    quotation.ID,
			RevisionNumber: quotation.RevisionNumber,
			CreatedBy:      user.ID,
		}); err != nil {
			t.Fatalf("create advisory draft %d: %v", index+1, err)
		}
	}
	availability := availabilityFor(t, database, ctx, quotation.ID)
	item := availabilityItem(t, availability, 1)
	if item.DraftQuantityScaled != 20*QuantityScale {
		t.Errorf("draft quantity = %d; want both drafts counted as advisory",
			item.DraftQuantityScaled)
	}
	if item.BoughtQuantityScaled != 0 || item.RemainingQuantityScaled != 10*QuantityScale {
		t.Errorf("drafts reserved quantity: bought %d / remaining %d",
			item.BoughtQuantityScaled, item.RemainingQuantityScaled)
	}
	if availability.DraftPOCount != 2 || availability.AllBought {
		t.Errorf("draft state = %d drafts / allBought %v; want two advisory drafts",
			availability.DraftPOCount, availability.AllBought)
	}
}

func TestQuotationAvailabilityAcrossRevisions(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_revisions")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	revisionZero := seedReportQuotation(t, database, 2, "PT. Revision", "Revision source",
		"IDR", "FTZ", []reportQuotationLine{
			{Description: "Kept line", UoM: "Pc", Qty: 10, Cost: 100, Sell: 250},
			{Description: "Removed line", UoM: "Pc", Qty: 5, Cost: 50, Sell: 80},
		})

	// Buy four of the first line in revision zero.
	finalizeAvailabilityQuantity(t, database, ctx, user, revisionZero, "PO-651-REV", 4*QuantityScale)

	// Revision one changes the kept line's quantity and price, removes the
	// second line, and adds a third. Bought state follows item/sub identity, so
	// the four bought units stay bought against the new quantity.
	if _, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode: 2, CustomerName: "PT. Revision", CustomerAddress: "Jl. Report Billing 1",
		RecipientName: "Budi Santoso", RecipientAddress: "Jl. Report Recipient 9",
		Subject: "Revision two", Incoterm: "CIF", ValidityDays: 30,
		PaymentTermDays: 45, LeadTimeDays: 10, TaxZone: "FTZ", TaxType: "Goods",
		Currency: "IDR", RevisionNumber: 1, QuoteNumber: revisionZero.QuoteNumber,
		HTMLSnapshot: "<html>revision two</html>",
		Items: []RevisionItem{
			{ItemNo: 1, Description: "Kept line", Qty: 12, UoM: "Pc", UnitPrice: 120,
				OriginalTotal: 1440, MarginalizedUnitPrice: 300, MarginalizedTotal: 3600,
				ListUnitPrice: 300},
			{ItemNo: 3, Description: "Added line", Qty: 3, UoM: "Pc", UnitPrice: 40,
				OriginalTotal: 120, MarginalizedUnitPrice: 90, MarginalizedTotal: 270,
				ListUnitPrice: 90},
		},
		Totals: RevisionTotals{
			OriginalSubtotal: 1560, OriginalGrand: 1560,
			MarginalizedSubtotal: 3870, MarginalizedGrand: 3870,
			MarginalizedGrandAfterTax: 3870, CustomerGrandTotal: 3870,
		},
	}); err != nil {
		t.Fatalf("issue revision one: %v", err)
	}

	latest := availabilityFor(t, database, ctx, revisionZero.ID)
	if latest.RevisionNumber != 1 || latest.LatestRevision != 1 {
		t.Fatalf("availability revision = %d (latest %d); want revision one",
			latest.RevisionNumber, latest.LatestRevision)
	}
	kept := availabilityItem(t, latest, 1)
	if kept.QuotedQuantityScaled != 12*QuantityScale ||
		kept.BoughtQuantityScaled != 4*QuantityScale ||
		kept.RemainingQuantityScaled != 8*QuantityScale {
		t.Errorf("revised kept line = quoted %d / bought %d / remaining %d; want 12/4/8",
			kept.QuotedQuantityScaled, kept.BoughtQuantityScaled, kept.RemainingQuantityScaled)
	}
	if kept.QuotedUnitPriceMinor != 300 {
		t.Errorf("revised baseline = %d; want the new 300 price", kept.QuotedUnitPriceMinor)
	}
	for _, item := range latest.Items {
		if item.QuotationItemNo == 2 {
			t.Errorf("removed item 2 still appears as a ghost: %+v", item)
		}
	}
	added := availabilityItem(t, latest, 3)
	if added.QuotedQuantityScaled != 3*QuantityScale || added.BoughtQuantityScaled != 0 ||
		added.RemainingQuantityScaled != 3*QuantityScale {
		t.Errorf("added item = quoted %d / bought %d / remaining %d; want 3/0/3",
			added.QuotedQuantityScaled, added.BoughtQuantityScaled, added.RemainingQuantityScaled)
	}

	// The price change does not rewrite the saved allocation's frozen baseline.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var frozen int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT quoted_unit_price_minor FROM %s.purchase_order_allocations
		WHERE quotation_id = $1 AND quotation_item_no = 1
		ORDER BY id LIMIT 1`, database.configSchema()), revisionZero.ID).Scan(&frozen); err != nil {
		t.Fatalf("read the frozen baseline: %v", err)
	}
	if frozen != 250 {
		t.Errorf("saved allocation baseline = %d; want the revision-zero 250", frozen)
	}
}

func TestQuotationAvailabilityDeletingASavedPORestoresTheGhost(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_delete")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	admin := deletionUser(t, database, "admin")
	quotation := seedReportQuotation(t, database, 2, "PT. Delete Ghost", "Delete source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Ghost line", UoM: "Pc", Qty: 10, Cost: 100, Sell: 250,
		}})
	saved := finalizeAvailabilityQuantity(t, database, ctx, user, quotation, "PO-651-DEL", 4*QuantityScale)

	before := availabilityFor(t, database, ctx, quotation.ID)
	if got := availabilityItem(t, before, 1).RemainingQuantityScaled; got != 6*QuantityScale {
		t.Fatalf("remaining before deletion = %d; want 6 units", got)
	}
	preview := previewFor(t, database, admin, DeletionTargetPurchaseOrder, saved.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetPurchaseOrder, TargetID: saved.ID,
		Scope: DeletionScopeRecord, Reason: "cancel the order",
		TypedNumber: preview.DocumentNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete the saved PO: %v", err)
	}
	after := availabilityFor(t, database, ctx, quotation.ID)
	item := availabilityItem(t, after, 1)
	if item.BoughtQuantityScaled != 0 || item.RemainingQuantityScaled != 10*QuantityScale {
		t.Errorf("after deletion = bought %d / remaining %d; want the full ghost back",
			item.BoughtQuantityScaled, item.RemainingQuantityScaled)
	}
	if after.SavedPOCount != 0 || after.AllBought {
		t.Errorf("after deletion = saved %d / allBought %v; want a free quotation",
			after.SavedPOCount, after.AllBought)
	}
}

func TestFinalizeRefusesAnItemExhaustedByAConcurrentSave(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_finalize_race")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Race Exhaust", "Exhaustion race",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Race line", UoM: "Pc", Qty: 10, Cost: 100, Sell: 250,
		}})

	// Both drafts were created while the quotation still had all ten units.
	first, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID: quotation.ID, RevisionNumber: quotation.RevisionNumber, CreatedBy: user.ID,
	})
	if err != nil {
		t.Fatalf("create first draft: %v", err)
	}
	second, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID: quotation.ID, RevisionNumber: quotation.RevisionNumber, CreatedBy: user.ID,
	})
	if err != nil {
		t.Fatalf("create second draft: %v", err)
	}

	raceCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	inputs := []PurchaseOrderFinalize{
		finalizeInput(first, user, "PO-651-RACE-A"),
		finalizeInput(second, user, "PO-651-RACE-B"),
	}
	results := make([]error, len(inputs))
	var wait sync.WaitGroup
	start := make(chan struct{})
	for index := range inputs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, results[index] = database.FinalizePurchaseOrder(raceCtx, inputs[index])
		}(index)
	}
	close(start)
	wait.Wait()

	successes, exhausted := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrPurchaseOrderItemAlreadyBought):
			exhausted++
		case errors.Is(err, context.DeadlineExceeded):
			t.Fatalf("finalize timed out under the quotation lock: %v", err)
		default:
			t.Errorf("concurrent finalize error = %v; want success or ErrPurchaseOrderItemAlreadyBought", err)
		}
	}
	if successes != 1 || exhausted != 1 {
		t.Fatalf("concurrent finalizes = %d winners / %d exhausted; want one of each",
			successes, exhausted)
	}
	availability := availabilityFor(t, database, ctx, quotation.ID)
	if !availability.AllBought || availability.SavedPOCount != 1 {
		t.Errorf("after the race = allBought %v / saved %d; want exactly one ten-unit PO",
			availability.AllBought, availability.SavedPOCount)
	}
}

// TestDraftSaveRewritesAllocationsAndSnapshotsOverwrite pins the advisory
// contract: every draft save rewrites its allocation, so "in drafts" is never
// stale, and an explicit overwrite keeps the replaced allocation picture beside
// the replaced lines.
func TestDraftSaveRewritesAllocationsAndSnapshotsOverwrite(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_draft_save")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Draft Save", "Draft save source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Draft save line", UoM: "Pc", Qty: 10, Cost: 100, Sell: 250,
		}})
	draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	before := readAllocations(t, database, ctx, draft.ID)["1.0"]
	if before.QuantityScaled != 10*QuantityScale {
		t.Fatalf("initial allocation quantity = %d; want the full ten units",
			before.QuantityScaled)
	}

	update := PurchaseOrderUpdate{
		ID:               draft.ID,
		BillingAddress:   draft.BillingAddress,
		DeliveryAddress:  draft.DeliveryAddress,
		ShipToCode:       draft.ShipToCode,
		ContactEmail:     draft.ContactEmail,
		ContactPhone:     draft.ContactPhone,
		PONumber:         draft.PONumber,
		Incoterms:        draft.Incoterms,
		PaymentTerms:     draft.PaymentTerms,
		PODate:           draft.PODate,
		ReceivedDate:     &draft.ReceivedDate,
		BusinessCategory: draft.BusinessCategory,
		Items:            append([]PurchaseOrderItem(nil), draft.Items...),
		UpdatedBy:        user.ID,
		ExpectedVersion:  draft.Version,
		Overwrite:        true,
	}
	update.Items[0].QuantityScaled = 4 * QuantityScale
	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("save the reduced draft: %v", err)
	}

	after := readAllocations(t, database, ctx, saved.ID)["1.0"]
	if after.QuantityScaled != 4*QuantityScale {
		t.Errorf("allocation after the draft save = %d; want the edited four units",
			after.QuantityScaled)
	}
	if after.RevenueMinor != saved.Items[0].NetMinor {
		t.Errorf("draft allocation revenue = %d; want the saved line net %d",
			after.RevenueMinor, saved.Items[0].NetMinor)
	}
	availability := availabilityFor(t, database, ctx, quotation.ID)
	if got := availabilityItem(t, availability, 1).DraftQuantityScaled; got != 4*QuantityScale {
		t.Errorf("advisory draft quantity = %d; want the rewritten four units", got)
	}

	var correctionID int64
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM %s.purchase_order_corrections
		WHERE purchase_order_id = $1 AND kind = $2
		ORDER BY id DESC LIMIT 1`, database.configSchema()),
		draft.ID, PurchaseOrderCorrectionKindDraftOverwrite).Scan(&correctionID); err != nil {
		t.Fatalf("read the overwrite correction: %v", err)
	}
	snapshot := readCorrectionAllocations(t, database, ctx, correctionID)
	if len(snapshot) != 1 {
		t.Fatalf("overwrite allocation snapshot = %d rows; want the replaced one", len(snapshot))
	}
	if snapshot[0].QuantityScaled != 10*QuantityScale ||
		snapshot[0].RevenueMinor != before.RevenueMinor {
		t.Errorf("overwrite snapshot = %+v; want the pre-edit allocation %+v",
			snapshot[0], before)
	}
}

// Issue #651 NB1: a split package's priced sub-items are the orderable leaves.
// Buying 2.1 alone leaves 2.2 as the next ghost; buying 2.2 exhausts the
// package. This is the QuotationSubNo != nil path through availability,
// explicit creation and finalize.
func TestQuotationAvailabilitySplitPackageBoughtBySubItem(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_subitems")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Sub Items", "Split package source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Installed package",
			UoM:         "Lot",
			Qty:         1,
			Cost:        0,
			Sell:        0,
			SubItems: []reportQuotationSub{
				{Description: "Mounting", UoM: "Pc", Qty: 2, Cost: 100, Sell: 200},
				{Description: "Cabling", UoM: "Pc", Qty: 3, Cost: 50, Sell: 90},
			},
		}})

	available := availabilityFor(t, database, ctx, quotation.ID)
	if len(available.Items) != 2 {
		t.Fatalf("split package leaves = %d; want the two priced sub-items", len(available.Items))
	}
	firstLeaf, ok := availabilityLeaf(available, 1, intPointer(1))
	if !ok {
		t.Fatalf("no availability leaf for 1.1: %+v", available.Items)
	}
	secondLeaf, ok := availabilityLeaf(available, 1, intPointer(2))
	if !ok {
		t.Fatalf("no availability leaf for 1.2: %+v", available.Items)
	}
	if firstLeaf.QuotedQuantityScaled != 2*QuantityScale ||
		secondLeaf.QuotedQuantityScaled != 3*QuantityScale {
		t.Errorf("split quantities = %d/%d; want 2/3 units",
			firstLeaf.QuotedQuantityScaled, secondLeaf.QuotedQuantityScaled)
	}

	// Buy only 2.1.
	firstPick := PurchaseOrderCreateItem{QuotationItemNo: 1, QuotationSubNo: intPointer(1)}
	firstDraft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		Items:          []PurchaseOrderCreateItem{firstPick},
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create the sub-item draft: %v", err)
	}
	if len(firstDraft.Items) != 1 || firstDraft.Items[0].QuotationSubNo == nil ||
		*firstDraft.Items[0].QuotationSubNo != 1 {
		t.Fatalf("sub-item draft lines = %+v; want only 2.1", firstDraft.Items)
	}
	if firstDraft.Items[0].QuantityScaled != 2*QuantityScale {
		t.Errorf("sub-item draft quantity = %d; want the two remaining units",
			firstDraft.Items[0].QuantityScaled)
	}
	firstInput := finalizeInput(firstDraft, user, "PO-651-SUB-A")
	firstInput.AcknowledgedMissing = []string{"1.2"}
	if _, err := database.FinalizePurchaseOrder(ctx, firstInput); err != nil {
		t.Fatalf("finalize the 2.1 purchase order: %v", err)
	}

	afterFirst := availabilityFor(t, database, ctx, quotation.ID)
	firstLeaf, _ = availabilityLeaf(afterFirst, 1, intPointer(1))
	secondLeaf, _ = availabilityLeaf(afterFirst, 1, intPointer(2))
	if firstLeaf.RemainingQuantityScaled != 0 {
		t.Errorf("2.1 remaining = %d; want the sub-item exhausted",
			firstLeaf.RemainingQuantityScaled)
	}
	if secondLeaf.RemainingQuantityScaled != 3*QuantityScale {
		t.Errorf("2.2 remaining = %d; want the three-unit ghost",
			secondLeaf.RemainingQuantityScaled)
	}
	if afterFirst.AllBought {
		t.Error("buying 2.1 alone marked the split package All bought")
	}

	// Buy the remaining 2.2 in a second PO.
	secondPick := PurchaseOrderCreateItem{QuotationItemNo: 1, QuotationSubNo: intPointer(2)}
	secondDraft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		Items:          []PurchaseOrderCreateItem{secondPick},
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create the second sub-item draft: %v", err)
	}
	if len(secondDraft.Items) != 1 || secondDraft.Items[0].QuotationSubNo == nil ||
		*secondDraft.Items[0].QuotationSubNo != 2 ||
		secondDraft.Items[0].QuantityScaled != 3*QuantityScale {
		t.Fatalf("second sub-item draft = %+v; want 2.2 x 3", secondDraft.Items)
	}
	secondInput := finalizeInput(secondDraft, user, "PO-651-SUB-B")
	secondInput.AcknowledgedMissing = []string{"1.1"}
	if _, err := database.FinalizePurchaseOrder(ctx, secondInput); err != nil {
		t.Fatalf("finalize the 2.2 purchase order: %v", err)
	}
	final := availabilityFor(t, database, ctx, quotation.ID)
	if !final.AllBought {
		t.Errorf("split package after both sub-items = %+v; want All bought", final.Items)
	}
	if _, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	}); !errors.Is(err, ErrQuotationFullyBought) {
		t.Errorf("create on the exhausted split package = %v; want ErrQuotationFullyBought", err)
	}
}

// Issue #651 NB3: a later revision that lowers the quantity below what is
// already bought clamps remaining to zero and reports the excess as over-bought.
func TestQuotationAvailabilityRevisionReductionIsOverBought(t *testing.T) {
	database := openTestSchema(t, "vera_po_651_overbought")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	rev0 := seedReportQuotation(t, database, 2, "PT. Over Bought", "Over bought source",
		"IDR", "FTZ", []reportQuotationLine{{
			Description: "Reduced line", UoM: "Pc", Qty: 10, Cost: 100, Sell: 250,
		}})
	finalizeAvailabilityQuantity(t, database, ctx, user, rev0, "PO-651-OVER", 4*QuantityScale)

	if _, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode: 2, CustomerName: "PT. Over Bought", CustomerAddress: "Jl. Report Billing 1",
		RecipientName: "Budi Santoso", RecipientAddress: "Jl. Report Recipient 9",
		Subject: "Reduced revision", Incoterm: "CIF", ValidityDays: 30,
		PaymentTermDays: 45, LeadTimeDays: 10, TaxZone: "FTZ", TaxType: "Goods",
		Currency: "IDR", RevisionNumber: 1, QuoteNumber: rev0.QuoteNumber,
		HTMLSnapshot: "<html>reduced revision</html>",
		Items: []RevisionItem{{
			ItemNo: 1, Description: "Reduced line", Qty: 3, UoM: "Pc",
			UnitPrice: 100, OriginalTotal: 300, MarginalizedUnitPrice: 250,
			MarginalizedTotal: 750, ListUnitPrice: 250,
		}},
		Totals: RevisionTotals{
			OriginalSubtotal: 300, OriginalGrand: 300,
			MarginalizedSubtotal: 750, MarginalizedGrand: 750,
			MarginalizedGrandAfterTax: 750, CustomerGrandTotal: 750,
		},
	}); err != nil {
		t.Fatalf("issue the reduced revision: %v", err)
	}

	available := availabilityFor(t, database, ctx, rev0.ID)
	item := availabilityItem(t, available, 1)
	if item.QuotedQuantityScaled != 3*QuantityScale {
		t.Errorf("quoted after reduction = %d; want 3 units", item.QuotedQuantityScaled)
	}
	if item.BoughtQuantityScaled != 4*QuantityScale {
		t.Errorf("bought = %d; want the historical 4 units", item.BoughtQuantityScaled)
	}
	if item.RemainingQuantityScaled != 0 {
		t.Errorf("remaining = %d; want zero after the downward revision",
			item.RemainingQuantityScaled)
	}
	if item.OverBoughtQuantityScaled != QuantityScale {
		t.Errorf("over-bought = %d; want the one excess unit",
			item.OverBoughtQuantityScaled)
	}
	if !available.AllBought {
		t.Error("a downward revision below bought did not report All bought")
	}
}
