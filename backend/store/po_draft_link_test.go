package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Issue #497: Staff link an issued Quotation revision to a Purchase Order
// draft through the existing difference review. These tests drive the real
// store against a disposable schema: never-linked and formerly-linked drafts
// are eligible, a live draft is not re-linked, the source owns identity while
// every PO-owned value survives, currency is never converted, the tax basis is
// an explicit keep/source choice with a server-side PPN recompute, occupancy
// and the version CAS still hold, the link writes no correction/repair
// history, and a completed link is not a completed finalize review.

// seedLinkQuotation writes one issued revision with the exact values a link
// test needs. It is the repair helper with the link defaults (IDR so a blank
// draft links without a currency mismatch).
func seedLinkQuotation(t *testing.T, database *Store, opts repairQuotationOptions) QuotationRecord {
	t.Helper()
	if opts.Currency == "" {
		opts.Currency = "IDR"
	}
	return seedRepairQuotationWith(t, database, opts)
}

// linkableDraftUpdate keeps the entered values but makes the two compared
// header fields agree with the source, so a test only has to acknowledge the
// source lines that are missing from the manual order.
func linkableDraftUpdate(draft PurchaseOrder, user AuthUser, source PurchaseOrderSource) PurchaseOrderUpdate {
	return PurchaseOrderUpdate{
		ID:              draft.ID,
		BillingAddress:  draft.BillingAddress,
		DeliveryAddress: draft.DeliveryAddress,
		ContactEmail:    draft.ContactEmail,
		PONumber:        draft.PONumber,
		ReceivedDate:    datePointer(draft.ReceivedDate),
		PaymentTerms:    source.PaymentTerms,
		Incoterms:       source.Incoterms,
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
		Items: []PurchaseOrderItem{{
			Description:    "Manual line entered before linking",
			QuantityScaled: 15000,
			UoM:            "Pc",
			UnitPriceMinor: 12500,
		}},
	}
}

// draftLinkInput is the payload a link submits: the target pair, the review's
// choices and acknowledgements, and the explicit tax-basis choice.
func draftLinkInput(
	order PurchaseOrder,
	source QuotationRecord,
	actor AuthUser,
	decisions []PurchaseOrderDecision,
	missing []string,
	extra []int,
	taxChoice string,
) PurchaseOrderDraftLinkInput {
	return PurchaseOrderDraftLinkInput{
		ID:                  order.ID,
		ExpectedVersion:     order.Version,
		QuotationID:         source.ID,
		RevisionNumber:      source.RevisionNumber,
		Decisions:           decisions,
		AcknowledgedMissing: missing,
		AcknowledgedExtra:   extra,
		TaxChoice:           taxChoice,
		UpdatedBy:           actor.ID,
	}
}

// linkReviewSource loads the source exactly as the test expects the store to
// read it, so assertions compare against the frozen revision rather than a
// hand-written copy.
func linkReviewSource(t *testing.T, database *Store, quotation QuotationRecord) PurchaseOrderSource {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	source, err := loadPurchaseOrderSourceDetail(context.Background(), pool,
		database.configSchema(), quotation.ID, quotation.RevisionNumber)
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	return source
}

// seedNeverLinkableDraft creates and saves a never-linked draft whose PO-owned
// sentinels are all distinguishable from the source.
func seedNeverLinkableDraft(t *testing.T, database *Store, user AuthUser, ctx context.Context, sentinel bool) PurchaseOrder {
	t.Helper()
	draft := createBlankDraft(t, database, ctx, user)
	update := blankDraftUpdate(draft, draft.Version, user)
	if sentinel {
		update.BillingAddress = "Jl. Billing entered before linking"
		update.DeliveryAddress = "Jl. Delivery SENTINEL"
		update.ShipToCode = "SHIP-SENTINEL"
		update.ContactEmail = "contact-sentinel@example.com"
		update.ContactPhone = "+62-811-SENTINEL"
		update.PONumber = "PO-SENTINEL-497"
		update.PODate = datePointer(time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC))
		update.RequestedDeliveryDate = datePointer(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
		update.PaymentTerms = "50% advance SENTINEL"
		update.Incoterms = "FOB"
		update.DiscountPercent = 5
		update.AdjustmentMinor = -100
		update.AdjustmentReason = "printed discount SENTINEL"
		update.Items = []PurchaseOrderItem{{
			Description:     "SENTINEL manual line",
			QuantityScaled:  15000,
			UoM:             "Pc",
			UnitPriceMinor:  12500,
			DiscountEnabled: true,
			DiscountPercent: 10,
		}}
	}
	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("save the never-linked draft: %v", err)
	}
	return saved
}

func TestPreviewDraftLinkWritesNothingAndReportsTheTaxChoice(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_preview")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Link", Subject: "Link source",
		Address: "Jl. Source", Currency: "IDR", TaxZone: "NonFTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	draft := seedNeverLinkableDraft(t, database, user, ctx, false)

	review, err := database.PreviewPurchaseOrderDraftLink(ctx, draft.ID, quotation.ID, quotation.RevisionNumber)
	if err != nil {
		t.Fatalf("preview draft link: %v", err)
	}
	if review.Order.ID != draft.ID || review.Order.SourceState != PurchaseOrderSourceStateNever {
		t.Errorf("review order = %+v; want the never-linked draft", review.Order)
	}
	if review.Source.QuotationID != quotation.ID || review.Source.QuoteNumber != quotation.QuoteNumber {
		t.Errorf("review source = %d %q; want %d %q",
			review.Source.QuotationID, review.Source.QuoteNumber, quotation.ID, quotation.QuoteNumber)
	}
	if !review.TaxChoiceRequired {
		t.Error("a Non-FTZ source beside an FTZ draft must require a tax choice")
	}
	if review.DraftTax.TaxZone != "FTZ" || review.SourceTax.TaxZone != "NonFTZ" {
		t.Errorf("tax bases = draft %q / source %q; want FTZ / NonFTZ",
			review.DraftTax.TaxZone, review.SourceTax.TaxZone)
	}
	if review.SourceTax.PpnBps != 1100 || review.DraftTax.PpnBps != 0 {
		t.Errorf("PPN rates = draft %d / source %d; want 0 / 1100",
			review.DraftTax.PpnBps, review.SourceTax.PpnBps)
	}

	unchanged, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("re-read after preview: %v", err)
	}
	if unchanged.Version != draft.Version || unchanged.SourceState != PurchaseOrderSourceStateNever {
		t.Errorf("preview wrote the draft: version %d -> %d, source %q",
			draft.Version, unchanged.Version, unchanged.SourceState)
	}
	if n := countRows(t, database, "purchase_order_corrections", "purchase_order_id = $1", draft.ID); n != 0 {
		t.Errorf("preview wrote %d history rows; want none", n)
	}
}

func TestStaffLinksANeverLinkedDraftAndSourceOwnsIdentity(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_never")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Source Identity", Subject: "Link source",
		Address: "Jl. Source Billing", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	draft := seedNeverLinkableDraft(t, database, user, ctx, true)
	source := linkReviewSource(t, database, quotation)

	// The sentinel payment terms and Incoterms differ from the source. Choosing
	// the PO entry keeps them; choosing the quotation applies the source value
	// in the client before the link, so the store sees an equal value plus a
	// recorded decision. Here both are kept as PO-owned.
	decisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
		{Key: "header:discountPercent", Choice: PurchaseOrderChoicePO},
	}
	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		draft, quotation, user, decisions, []string{"1", "3"}, []int{1}, ""))
	if err != nil {
		t.Fatalf("link the never-linked draft: %v", err)
	}

	// Source-owned identity.
	if linked.SourceState != PurchaseOrderSourceStateLive || linked.SourceMissing {
		t.Errorf("source state = %q missing=%v; want live", linked.SourceState, linked.SourceMissing)
	}
	if linked.QuotationID != quotation.ID || linked.SourceRevision != quotation.RevisionNumber {
		t.Errorf("link = quotation %d revision %d; want %d/%d",
			linked.QuotationID, linked.SourceRevision, quotation.ID, quotation.RevisionNumber)
	}
	if linked.QuoteNumber != quotation.QuoteNumber || linked.CustomerID != source.CustomerID ||
		linked.CustomerName != source.CustomerName {
		t.Errorf("identity = %q / %d %q; want %q / %d %q",
			linked.QuoteNumber, linked.CustomerID, linked.CustomerName,
			quotation.QuoteNumber, source.CustomerID, source.CustomerName)
	}
	if linked.PrintedSubtotalMinor != source.PrintedSubtotalMinor ||
		linked.PrintedDiscountMinor != source.PrintedDiscountMinor ||
		linked.PrintedPpnMinor != source.PrintedPpnMinor ||
		linked.PrintedGrandTotalMinor != source.PrintedGrandTotalMinor {
		t.Errorf("printed totals = %+v; want the source's %+v",
			[]int64{linked.PrintedSubtotalMinor, linked.PrintedDiscountMinor, linked.PrintedPpnMinor, linked.PrintedGrandTotalMinor},
			[]int64{source.PrintedSubtotalMinor, source.PrintedDiscountMinor, source.PrintedPpnMinor, source.PrintedGrandTotalMinor})
	}
	if linked.BillingAddress != source.BillingAddress {
		t.Errorf("billing address = %q; a new customer's address comes from the source %q",
			linked.BillingAddress, source.BillingAddress)
	}

	// PO-owned values survive untouched.
	if linked.PONumber != "PO-SENTINEL-497" || linked.DeliveryAddress != "Jl. Delivery SENTINEL" ||
		linked.ShipToCode != "SHIP-SENTINEL" || linked.ContactEmail != "contact-sentinel@example.com" ||
		linked.ContactPhone != "+62-811-SENTINEL" {
		t.Errorf("PO-owned header = %+v; want every sentinel preserved", linked)
	}
	if linked.PaymentTerms != "50% advance SENTINEL" || linked.Incoterms != "FOB" {
		t.Errorf("PO-owned wording = %q / %q; want the entered values",
			linked.PaymentTerms, linked.Incoterms)
	}
	if linked.PODate == nil || linked.PODate.Format("2006-01-02") != "2026-09-09" ||
		linked.RequestedDeliveryDate == nil ||
		linked.RequestedDeliveryDate.Format("2006-01-02") != "2026-10-10" {
		t.Errorf("PO dates moved: poDate=%v requested=%v", linked.PODate, linked.RequestedDeliveryDate)
	}
	if linked.DiscountPercent != 5 || linked.AdjustmentMinor != -100 ||
		linked.AdjustmentReason != "printed discount SENTINEL" {
		t.Errorf("PO-owned discount/adjustment = %d / %d %q",
			linked.DiscountPercent, linked.AdjustmentMinor, linked.AdjustmentReason)
	}
	if len(linked.Items) != 1 || linked.Items[0].Description != "SENTINEL manual line" ||
		linked.Items[0].QuantityScaled != 15000 || linked.Items[0].UnitPriceMinor != 12500 ||
		!linked.Items[0].DiscountEnabled || linked.Items[0].DiscountPercent != 10 {
		t.Errorf("PO lines = %+v; want the entered line unchanged", linked.Items)
	}

	// An ordinary draft mutation: the version moved, and no history/repair row
	// was invented for a link.
	if linked.Version <= draft.Version {
		t.Errorf("version = %d after linking from %d; want a forward bump", linked.Version, draft.Version)
	}
	if n := countRows(t, database, "purchase_order_corrections", "purchase_order_id = $1", draft.ID); n != 0 {
		t.Errorf("link wrote %d correction snapshots; want none", n)
	}
	if n := countRows(t, database, "purchase_order_source_repairs", "correction_id IN (SELECT id FROM "+database.configSchema()+".purchase_order_corrections WHERE purchase_order_id = $1)", draft.ID); n != 0 {
		t.Errorf("link wrote %d source-repair satellites; want none", n)
	}
}

func TestDraftLinkFormerRowClearsFormerSourceAndKeepsSameCustomerAddress(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_former")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedRepairQuotation(t, database, 2, "PT. Former Link", "Deleted source", "Jl. Old Billing", 100, 45)
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
		t.Fatalf("read the former draft: %v", err)
	}
	if former.SourceState != PurchaseOrderSourceStateFormer {
		t.Fatalf("source state = %q; want former", former.SourceState)
	}
	// The replacement follows the same customer but a different address: the
	// entered billing address is PO-owned and must not be replaced.
	replacement := seedRepairQuotation(t, database, 2, "PT. Former Link", "Replacement source", "Jl. New Billing", 100, 45)

	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		former, replacement, user, nil, nil, nil, ""))
	if err != nil {
		t.Fatalf("link the formerly-linked draft: %v", err)
	}
	if linked.SourceState != PurchaseOrderSourceStateLive || linked.SourceMissing {
		t.Errorf("source state = %q; want live", linked.SourceState)
	}
	if linked.QuotationID != replacement.ID || linked.SourceRevision != replacement.RevisionNumber {
		t.Errorf("link = %d/%d; want %d/%d",
			linked.QuotationID, linked.SourceRevision, replacement.ID, replacement.RevisionNumber)
	}
	if linked.BillingAddress != "Jl. Old Billing" {
		t.Errorf("billing address = %q; a same-customer link keeps the PO-owned address", linked.BillingAddress)
	}

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var formerIDNull, formerUUIDNull, formerQuoteNull, formerRevisionNull bool
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT former_quotation_id IS NULL, former_quotation_uuid IS NULL,
		       former_quote_number IS NULL, former_source_revision IS NULL
		FROM %s.purchase_orders WHERE id = $1`, database.configSchema()),
		draft.ID).Scan(&formerIDNull, &formerUUIDNull, &formerQuoteNull, &formerRevisionNull); err != nil {
		t.Fatalf("read former columns: %v", err)
	}
	if !formerIDNull || !formerUUIDNull || !formerQuoteNull || !formerRevisionNull {
		t.Errorf("former source survived the link: %v/%v/%v/%v",
			formerIDNull, formerUUIDNull, formerQuoteNull, formerRevisionNull)
	}
}

func TestDraftLinkRefusesSavedRecordsAndLiveDrafts(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_eligibility")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Eligible", Subject: "Eligibility",
		Address: "Jl. Eligible", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	liveQuotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 3, CustomerName: "PT. Live", Subject: "Live",
		Address: "Jl. Live", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	other := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 4, CustomerName: "PT. Other", Subject: "Other",
		Address: "Jl. Other", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	saved := savedPurchaseOrder(t, database, ctx, staff, quotation, "PO-497-SAVED")

	if _, err := database.PreviewPurchaseOrderDraftLink(ctx, saved.ID, other.ID, other.RevisionNumber); !errors.Is(err, ErrPurchaseOrderNotDraft) {
		t.Errorf("preview of a saved record = %v; want ErrPurchaseOrderNotDraft", err)
	}
	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		saved, other, staff, nil, nil, nil, "")); !errors.Is(err, ErrPurchaseOrderNotDraft) {
		t.Errorf("link of a saved record = %v; want ErrPurchaseOrderNotDraft", err)
	}

	// A live-linked draft is not re-linked by this ticket.
	live := createFinalizeDraft(t, database, ctx, staff, liveQuotation)
	if _, err := database.PreviewPurchaseOrderDraftLink(ctx, live.ID, other.ID, other.RevisionNumber); !errors.Is(err, ErrPurchaseOrderDraftLinkNotEligible) {
		t.Errorf("preview of a live draft = %v; want ErrPurchaseOrderDraftLinkNotEligible", err)
	}
	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		live, other, staff, nil, nil, nil, "")); !errors.Is(err, ErrPurchaseOrderDraftLinkNotEligible) {
		t.Errorf("link of a live draft = %v; want ErrPurchaseOrderDraftLinkNotEligible", err)
	}
	reloaded, err := database.GetPurchaseOrder(ctx, live.ID)
	if err != nil {
		t.Fatalf("re-read live draft: %v", err)
	}
	if reloaded.QuotationID != liveQuotation.ID || reloaded.SourceRevision != liveQuotation.RevisionNumber {
		t.Errorf("a refused re-link moved the live draft to %d/%d",
			reloaded.QuotationID, reloaded.SourceRevision)
	}
}

func TestDraftLinkRefusesAnOccupiedQuotationAndRacesToOneWinner(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_occupied")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Occupied", Subject: "Occupied",
		Address: "Jl. Occupied", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	source := linkReviewSource(t, database, quotation)

	first := seedNeverLinkableDraft(t, database, staff, ctx, false)
	second := seedNeverLinkableDraft(t, database, staff, ctx, false)
	decisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	missing := []string{"1", "3"}
	extra := []int{1}

	var wg sync.WaitGroup
	results := make([]error, 2)
	inputs := []PurchaseOrderDraftLinkInput{
		draftLinkInput(first, quotation, staff, decisions, missing, extra, ""),
		draftLinkInput(second, quotation, staff, decisions, missing, extra, ""),
	}
	for index := range inputs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, results[index] = database.LinkPurchaseOrderDraftSource(ctx, inputs[index])
		}(index)
	}
	wg.Wait()

	winners := 0
	losers := 0
	for _, err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrPurchaseOrderTargetOccupied):
			losers++
		default:
			t.Errorf("concurrent link error = %v; want success or ErrPurchaseOrderTargetOccupied", err)
		}
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("concurrent links = %d winners / %d occupied; want exactly one of each", winners, losers)
	}

	// The pre-check answers normally once the row is committed.
	third := seedNeverLinkableDraft(t, database, staff, ctx, false)
	if _, err := database.PreviewPurchaseOrderDraftLink(ctx, third.ID, quotation.ID, quotation.RevisionNumber); !errors.Is(err, ErrPurchaseOrderTargetOccupied) {
		t.Errorf("preview of an occupied target = %v; want ErrPurchaseOrderTargetOccupied", err)
	}
	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		third, quotation, staff, decisions, missing, extra, "")); !errors.Is(err, ErrPurchaseOrderTargetOccupied) {
		t.Errorf("link of an occupied target = %v; want ErrPurchaseOrderTargetOccupied", err)
	}
	_ = source
}

func TestDraftLinkCurrencyMismatchBlocksWithoutConversion(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_currency")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	// The source is USD; the never-linked draft was seeded by the company
	// profile and stays IDR until staff edit it.
	usdSource := seedRepairQuotation(t, database, 2, "PT. Currency", "USD source", "Jl. Currency", 100, 45)
	draft := seedNeverLinkableDraft(t, database, staff, ctx, false)
	if draft.Currency != "IDR" {
		t.Fatalf("draft currency = %q; want the company default IDR", draft.Currency)
	}
	decisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	missing := []string{"1", "3"}
	extra := []int{1}

	// The preview shows both sides so the UI can explain the remedy; it does
	// not convert anything and writes nothing.
	review, err := database.PreviewPurchaseOrderDraftLink(ctx, draft.ID, usdSource.ID, usdSource.RevisionNumber)
	if err != nil {
		t.Fatalf("preview across currencies: %v", err)
	}
	if review.Source.Currency != "USD" || review.Order.Currency != "IDR" {
		t.Errorf("review currencies = draft %q / source %q; want IDR / USD",
			review.Order.Currency, review.Source.Currency)
	}

	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		draft, usdSource, staff, decisions, missing, extra, "")); !errors.Is(err, ErrPurchaseOrderSourceCurrencyMismatch) {
		t.Fatalf("link across currencies = %v; want ErrPurchaseOrderSourceCurrencyMismatch", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("re-read after the currency refusal: %v", err)
	}
	if unchanged.Currency != "IDR" || unchanged.SourceState != PurchaseOrderSourceStateNever {
		t.Errorf("refused link changed the draft: %q / %q", unchanged.Currency, unchanged.SourceState)
	}
	if unchanged.SubtotalMinor != draft.SubtotalMinor || unchanged.GrandTotalMinor != draft.GrandTotalMinor {
		t.Errorf("refused link converted amounts: %d/%d -> %d/%d",
			draft.SubtotalMinor, draft.GrandTotalMinor, unchanged.SubtotalMinor, unchanged.GrandTotalMinor)
	}

	// The remedy is editing the never-linked draft's currency, then retrying.
	update := linkableDraftUpdate(unchanged, staff, linkReviewSource(t, database, usdSource))
	update.Currency = "USD"
	update.TaxZone = "" // keep the stored basis; only the currency changes
	edited, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("edit the draft currency: %v", err)
	}
	beforeSubtotal := edited.SubtotalMinor
	beforeGrand := edited.GrandTotalMinor
	relinked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		edited, usdSource, staff, decisions, missing, extra, ""))
	if err != nil {
		t.Fatalf("link after editing the currency: %v", err)
	}
	if relinked.Currency != "USD" || relinked.SourceState != PurchaseOrderSourceStateLive {
		t.Errorf("linked row = %q / %q; want USD / live", relinked.Currency, relinked.SourceState)
	}
	if relinked.SubtotalMinor != beforeSubtotal || relinked.GrandTotalMinor != beforeGrand {
		t.Errorf("the currency edit converted amounts: %d/%d -> %d/%d",
			beforeSubtotal, beforeGrand, relinked.SubtotalMinor, relinked.GrandTotalMinor)
	}
}

func TestDraftLinkTaxChoiceKeepsOrUsesTheSourceAndRecomputesPPN(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_tax")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	sourceQuotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Tax", Subject: "Tax source",
		Address: "Jl. Tax", Currency: "IDR", TaxZone: "NonFTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	sourceAgain := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Tax", Subject: "Tax source again",
		Address: "Jl. Tax", Currency: "IDR", TaxZone: "NonFTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	decisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	missing := []string{"1", "3"}
	extra := []int{1}

	// No choice while the bases differ is refused, not silently adopted.
	keep := seedNeverLinkableDraft(t, database, staff, ctx, false)
	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		keep, sourceQuotation, staff, decisions, missing, extra, "")); !errors.Is(err, ErrPurchaseOrderDraftLinkTaxChoiceRequired) {
		t.Fatalf("link without a tax choice = %v; want ErrPurchaseOrderDraftLinkTaxChoiceRequired", err)
	}
	kept, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		keep, sourceQuotation, staff, decisions, missing, extra, PurchaseOrderDraftLinkTaxKeep))
	if err != nil {
		t.Fatalf("link keeping the draft basis: %v", err)
	}
	if kept.TaxZone != "FTZ" || kept.TaxType != "Goods" || kept.PpnBps != 0 || kept.PpnMinor != 0 {
		t.Errorf("kept basis = %q/%q ppn %d/%d; want the draft's FTZ 0",
			kept.TaxZone, kept.TaxType, kept.PpnBps, kept.PpnMinor)
	}
	if kept.SubtotalMinor != keep.SubtotalMinor || kept.GrandTotalMinor != keep.SubtotalMinor {
		t.Errorf("kept totals = %d/%d; want the PO-owned subtotal and no PPN",
			kept.SubtotalMinor, kept.GrandTotalMinor)
	}
	if kept.GrandTotalMinor != kept.SubtotalMinor-kept.DiscountMinor+kept.PpnMinor+kept.AdjustmentMinor {
		t.Errorf("kept grand total %d does not follow subtotal/discount/PPN/adjustment %d/%d/%d/%d",
			kept.GrandTotalMinor, kept.SubtotalMinor, kept.DiscountMinor, kept.PpnMinor, kept.AdjustmentMinor)
	}

	// The source basis is adopted only when chosen, and the server recomputes
	// PPN from it while the PO-owned line prices stay put.
	useSource := seedNeverLinkableDraft(t, database, staff, ctx, false)
	beforeSubtotal := useSource.SubtotalMinor
	adopted, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		useSource, sourceAgain, staff, decisions, missing, extra, PurchaseOrderDraftLinkTaxSource))
	if err != nil {
		t.Fatalf("link using the source basis: %v", err)
	}
	if adopted.TaxZone != "NonFTZ" || adopted.TaxType != "Goods" || adopted.TaxCategory != "" {
		t.Errorf("adopted basis = %q/%q/%q; want the source's NonFTZ/Goods",
			adopted.TaxZone, adopted.TaxType, adopted.TaxCategory)
	}
	if adopted.PpnBps != 1100 {
		t.Errorf("adopted PPN = %d bps; want 1100", adopted.PpnBps)
	}
	if adopted.PpnMinor <= 0 {
		t.Errorf("adopted PPN amount = %d; want a recomputed positive amount", adopted.PpnMinor)
	}
	if adopted.SubtotalMinor != beforeSubtotal {
		t.Errorf("adoption changed the PO subtotal %d -> %d", beforeSubtotal, adopted.SubtotalMinor)
	}
	if len(adopted.Items) != 1 || adopted.Items[0].UnitPriceMinor != useSource.Items[0].UnitPriceMinor ||
		adopted.Items[0].QuantityScaled != useSource.Items[0].QuantityScaled {
		t.Errorf("adoption changed the PO lines: %+v", adopted.Items)
	}
	if adopted.GrandTotalMinor != adopted.SubtotalMinor-adopted.DiscountMinor+adopted.PpnMinor+adopted.AdjustmentMinor {
		t.Errorf("adopted grand total %d does not follow subtotal/discount/PPN/adjustment %d/%d/%d/%d",
			adopted.GrandTotalMinor, adopted.SubtotalMinor, adopted.DiscountMinor, adopted.PpnMinor, adopted.AdjustmentMinor)
	}
}

func TestDraftLinkRequiresExplicitLineMatchingAndRefusesDuplicates(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_lines")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedTenLineQuotation(t, database, ctx, "Ten line link")
	matchedQuotation := seedTenLineQuotation(t, database, ctx, "Ten line link matched")
	duplicateQuotation := seedTenLineQuotation(t, database, ctx, "Ten line link duplicate")
	draft := seedTenLineManualDraft(t, database, ctx, staff)

	// Nothing acknowledged: the link is blocked by the difference review.
	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		draft, quotation, staff, nil, nil, nil, "")); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("link without acknowledgements = %v; want ErrPurchaseOrderUnresolvedDifference", err)
	}

	// Every manual line is extra and every source line is missing until the
	// reviewer acknowledges or matches them.
	allExtra := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	allMissing := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	acknowledged, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		draft, quotation, staff, nil, allMissing, allExtra, ""))
	if err != nil {
		t.Fatalf("link an acknowledged ten-line manual draft: %v", err)
	}
	if acknowledged.SourceState != PurchaseOrderSourceStateLive {
		t.Errorf("source state = %q; want live", acknowledged.SourceState)
	}
	for _, item := range acknowledged.Items {
		if item.QuotationItemNo != nil {
			t.Errorf("acknowledged extra line kept reference %v; a manual line stays extra", *item.QuotationItemNo)
		}
	}

	// A second draft matches all ten lines explicitly. The stored references
	// are what the link validates, so the applied review is saved first.
	matched := seedTenLineManualDraft(t, database, ctx, staff)
	matched, err = database.UpdatePurchaseOrderDraft(ctx, tenLineMatchedUpdate(matched, staff))
	if err != nil {
		t.Fatalf("save the explicitly matched lines: %v", err)
	}
	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		matched, matchedQuotation, staff, tenLineDescriptionDecisions(), nil, nil, ""))
	if err != nil {
		t.Fatalf("link the explicitly matched ten-line draft: %v", err)
	}
	if len(linked.Items) != 10 {
		t.Fatalf("linked lines = %d; want 10", len(linked.Items))
	}
	for index, item := range linked.Items {
		if item.QuotationItemNo == nil || *item.QuotationItemNo != index+1 {
			t.Errorf("line %d reference = %v; want %d", index+1, item.QuotationItemNo, index+1)
		}
	}

	// Two lines cannot claim one source line.
	duplicate := seedTenLineManualDraft(t, database, ctx, staff)
	duplicateUpdate := tenLineMatchedUpdate(duplicate, staff)
	duplicateUpdate.Items[1].QuotationItemNo = duplicateUpdate.Items[0].QuotationItemNo
	duplicate, err = database.UpdatePurchaseOrderDraft(ctx, duplicateUpdate)
	if err != nil {
		t.Fatalf("save the duplicate match: %v", err)
	}
	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		duplicate, duplicateQuotation, staff, nil, nil, nil, "")); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Errorf("link with a duplicate match = %v; want ErrPurchaseOrderUnresolvedDifference", err)
	}

	// A reference the revision does not carry is not an "extra" line: a stale
	// or invented link is refused by name rather than stored.
	invented := seedTenLineManualDraft(t, database, ctx, staff)
	inventedUpdate := tenLineMatchedUpdate(invented, staff)
	unknown := 99
	inventedUpdate.Items[0].QuotationItemNo = &unknown
	invented, err = database.UpdatePurchaseOrderDraft(ctx, inventedUpdate)
	if err != nil {
		t.Fatalf("save the invented match: %v", err)
	}
	if _, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		invented, duplicateQuotation, staff, tenLineDescriptionDecisions(), nil, nil, "")); !errors.Is(err, ErrPurchaseOrderBadReference) {
		t.Errorf("link with an invented reference = %v; want ErrPurchaseOrderBadReference", err)
	}
}

func TestDraftLinkStaleVersionLeavesTheDraftUntouched(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_stale")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Stale", Subject: "Stale",
		Address: "Jl. Stale", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	draft := seedNeverLinkableDraft(t, database, staff, ctx, false)
	input := draftLinkInput(draft, quotation, staff,
		[]PurchaseOrderDecision{
			{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
			{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
		}, []string{"1", "3"}, []int{1}, "")
	input.ExpectedVersion = draft.Version + 1

	_, err := database.LinkPurchaseOrderDraftSource(ctx, input)
	var conflict *PurchaseOrderVersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale link = %v; want a version conflict", err)
	}
	unchanged, readErr := database.GetPurchaseOrder(ctx, draft.ID)
	if readErr != nil {
		t.Fatalf("re-read after the stale link: %v", readErr)
	}
	if unchanged.Version != draft.Version || unchanged.SourceState != PurchaseOrderSourceStateNever {
		t.Errorf("stale link wrote the draft: version %d -> %d, source %q",
			draft.Version, unchanged.Version, unchanged.SourceState)
	}
	if n := countRows(t, database, "purchase_order_corrections", "purchase_order_id = $1", draft.ID); n != 0 {
		t.Errorf("stale link wrote %d history rows; want none", n)
	}
}

func TestDraftLinkFormerSameCustomerTakesTheRenamedSourceName(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_rename")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedRepairQuotation(t, database, 2, "PT. Name Before", "Original source", "Jl. Old Billing", 100, 45)
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
		t.Fatalf("read the former draft: %v", err)
	}
	if former.SourceState != PurchaseOrderSourceStateFormer {
		t.Fatalf("source state = %q; want former", former.SourceState)
	}
	if former.CustomerName != "PT. Name Before" {
		t.Fatalf("former customer name = %q; want the pre-rename snapshot", former.CustomerName)
	}

	// The Customer directory is the name authority. Renaming the same customer
	// must be visible after the draft is linked to a replacement revision: the
	// name is source-owned even when the customer did not change.
	customerID := customerIDByCode(t, database, 2)
	if _, err := database.UpdateClient(ctx, customerID, "PT. Name After", "Jl. Directory Address"); err != nil {
		t.Fatalf("rename the customer: %v", err)
	}
	replacement := seedRepairQuotation(t, database, 2, "PT. Name After", "Replacement source", "Jl. New Billing", 100, 45)
	source := linkReviewSource(t, database, replacement)
	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		former, replacement, user, nil, nil, nil, ""))
	if err != nil {
		t.Fatalf("link the former draft to the same customer's replacement: %v", err)
	}
	if linked.CustomerID != source.CustomerID {
		t.Errorf("customer id = %d; want the source's %d", linked.CustomerID, source.CustomerID)
	}
	if linked.CustomerName != "PT. Name After" {
		t.Errorf("customer name = %q; the source-owned name is %q", linked.CustomerName, source.CustomerName)
	}
	if linked.BillingAddress != "Jl. Old Billing" {
		t.Errorf("billing address = %q; a same-customer link keeps the PO-owned address", linked.BillingAddress)
	}
}

func TestDraftLinkIsNotAFinalizeReview(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_finalize")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Finalize", Subject: "Finalize after link",
		Address: "Jl. Finalize", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	draft := seedNeverLinkableDraft(t, database, staff, ctx, false)
	decisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		draft, quotation, staff, decisions, []string{"1", "3"}, []int{1}, ""))
	if err != nil {
		t.Fatalf("link before finalize: %v", err)
	}

	// The link's decisions are not the final save's decisions: finalize still
	// runs its own review against the stored values.
	unresolved := finalizeInput(linked, staff, "PO-497-FINAL")
	unresolved.DocumentConfirmed = true
	if _, err := database.FinalizePurchaseOrder(ctx, unresolved); !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
		t.Fatalf("finalize without its own review = %v; want ErrPurchaseOrderUnresolvedDifference", err)
	}

	withReview := finalizeInput(linked, staff, "PO-497-FINAL")
	withReview.DocumentConfirmed = true
	withReview.Decisions = decisions
	withReview.AcknowledgedMissing = []string{"1", "3"}
	withReview.AcknowledgedExtra = []int{1}
	saved, err := database.FinalizePurchaseOrder(ctx, withReview)
	if err != nil {
		t.Fatalf("finalize after its own review: %v", err)
	}
	if saved.State != PurchaseOrderStateSaved || saved.QuotationID != quotation.ID {
		t.Errorf("finalized record = %+v; want a saved live record", saved)
	}
}

func TestDraftLinkIsIsolatedToItsOrganization(t *testing.T) {
	database := openTestSchema(t, "vera_po_497_isolation")
	staff := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Isolation", Subject: "Isolation",
		Address: "Jl. Isolation", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	mine := seedNeverLinkableDraft(t, database, staff, ctx, false)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var otherOrg string
	if err := pool.QueryRow(ctx, `
		INSERT INTO `+database.configSchema()+`.organizations (name)
		VALUES ('Other Link Tenant') RETURNING id::text`).Scan(&otherOrg); err != nil {
		t.Fatalf("seed second organization: %v", err)
	}
	other := ContextWithOrg(context.Background(), otherOrg)

	if _, err := database.PreviewPurchaseOrderDraftLink(other, mine.ID, quotation.ID, quotation.RevisionNumber); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("cross-org preview = %v; want ErrPurchaseOrderNotFound", err)
	}
	if _, err := database.LinkPurchaseOrderDraftSource(other, draftLinkInput(
		mine, quotation, staff, nil, nil, nil, "")); !errors.Is(err, ErrPurchaseOrderNotFound) {
		t.Errorf("cross-org link = %v; want ErrPurchaseOrderNotFound", err)
	}
	unchanged, err := database.GetPurchaseOrder(ctx, mine.ID)
	if err != nil {
		t.Fatalf("re-read my draft: %v", err)
	}
	if unchanged.SourceState != PurchaseOrderSourceStateNever {
		t.Errorf("a cross-org link moved my draft to %q", unchanged.SourceState)
	}
}

// seedTenLineQuotation writes an issued revision with ten priced lines whose
// item numbers are 1..10, so a test can deliberately decide every match.
func seedTenLineQuotation(t *testing.T, database *Store, ctx context.Context, subject string) QuotationRecord {
	t.Helper()
	items := make([]RevisionItem, 0, 10)
	for index := 1; index <= 10; index++ {
		items = append(items, RevisionItem{
			ItemNo:                index,
			Description:           fmt.Sprintf("Source line %d", index),
			Qty:                   1,
			UoM:                   "Pc",
			UnitPrice:             100,
			MarginPercent:         0,
			MarginAmount:          0,
			OriginalTotal:         100,
			MarginalizedUnitPrice: 100,
			MarginalizedTotal:     100,
			ListUnitPrice:         100,
		})
	}
	record, err := database.SaveRevision(ctx, RevisionDraft{
		CustomerCode:     2,
		CustomerName:     "PT. Ten Lines",
		CustomerAddress:  "Jl. Ten",
		RecipientName:    "Budi Santoso",
		RecipientAddress: "Jl. Ten Recipient",
		Subject:          subject,
		Incoterm:         "CIF",
		ValidityDays:     30,
		PaymentTermDays:  45,
		LeadTimeDays:     10,
		DiscountPercent:  0,
		TaxZone:          "FTZ",
		TaxType:          "Goods",
		Currency:         "IDR",
		HTMLSnapshot:     "<html>ten line snapshot</html>",
		Items:            items,
		Totals: RevisionTotals{
			OriginalSubtotal:          1000,
			OriginalGrand:             1000,
			MarginalizedSubtotal:      1000,
			MarginalizedGrand:         1000,
			MarginalizedGrandAfterTax: 1000,
			CustomerGrandTotal:        1000,
		},
	})
	if err != nil {
		t.Fatalf("seed ten-line quotation: %v", err)
	}
	return record
}

// seedTenLineManualDraft creates a never-linked draft with ten manual lines,
// each entered without a quotation reference.
func seedTenLineManualDraft(t *testing.T, database *Store, ctx context.Context, user AuthUser) PurchaseOrder {
	t.Helper()
	draft := createBlankDraft(t, database, ctx, user)
	update := PurchaseOrderUpdate{
		ID:              draft.ID,
		PaymentTerms:    "45 days",
		Incoterms:       "CIF",
		ReceivedDate:    datePointer(draft.ReceivedDate),
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
	}
	for index := 1; index <= 10; index++ {
		update.Items = append(update.Items, PurchaseOrderItem{
			Description:    fmt.Sprintf("Manual line %d", index),
			QuantityScaled: 10000,
			UoM:            "Pc",
			UnitPriceMinor: 100,
		})
	}
	saved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("save the ten-line manual draft: %v", err)
	}
	return saved
}

// tenLineDescriptionDecisions records "Use PO entry" for the ten descriptions
// a manual line keeps when it is matched to a source line.
func tenLineDescriptionDecisions() []PurchaseOrderDecision {
	decisions := make([]PurchaseOrderDecision, 0, 10)
	for index := 1; index <= 10; index++ {
		decisions = append(decisions, PurchaseOrderDecision{
			Key:    fmt.Sprintf("line:%d:description", index),
			Choice: PurchaseOrderChoicePO,
		})
	}
	return decisions
}

// tenLineMatchedUpdate is the applied difference review: every manual line
// claims the source line with the same number.
func tenLineMatchedUpdate(draft PurchaseOrder, user AuthUser) PurchaseOrderUpdate {
	items := make([]PurchaseOrderItem, len(draft.Items))
	for index, item := range draft.Items {
		itemNo := index + 1
		item.LineNumber = index + 1
		item.QuotationItemNo = &itemNo
		item.QuotationSubNo = nil
		items[index] = item
	}
	return PurchaseOrderUpdate{
		ID:              draft.ID,
		BillingAddress:  draft.BillingAddress,
		DeliveryAddress: draft.DeliveryAddress,
		ContactEmail:    draft.ContactEmail,
		PONumber:        draft.PONumber,
		ReceivedDate:    datePointer(draft.ReceivedDate),
		PaymentTerms:    "45 days",
		Incoterms:       "CIF",
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
		Items:           items,
	}
}
