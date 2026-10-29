package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Ticket 05 drives the whole comparable surface through the persisted
// session: stable line identities across saves and reorders, removal as
// line_removed, the link-time tax item, line-item validation and the Final
// Save blockers. All tests run against a disposable Postgres schema.

// lineDifferenceItem builds one line difference item keyed by stable identity.
func lineDifferenceItem(t *testing.T, lineKey, field, suffix, label string, poValue, sourceValue any, sourceRef string) POReconciliationItem {
	t.Helper()
	return POReconciliationItem{
		Key:           "line:" + lineKey + ":" + suffix,
		Field:         field,
		Kind:          POReconciliationKindDifference,
		LineKey:       strPointer(lineKey),
		SourceLineRef: strPointer(sourceRef),
		Label:         label,
		POValue:       mustMarshalValue(t, poValue),
		SourceValue:   mustMarshalValue(t, sourceValue),
		Status:        POReconciliationStatusUnresolved,
	}
}

// lineMatchItem builds one line-set item (missing/extra/duplicate).
func lineMatchItem(t *testing.T, key, field, kind, label string, lineKey, sourceRef *string, poValue, sourceValue any, status, resolution string) POReconciliationItem {
	t.Helper()
	item := POReconciliationItem{
		Key:           key,
		Field:         field,
		Kind:          kind,
		LineKey:       lineKey,
		SourceLineRef: sourceRef,
		Label:         label,
		Status:        status,
	}
	if poValue == nil {
		item.POValue = json.RawMessage("null")
	} else {
		item.POValue = mustMarshalValue(t, poValue)
	}
	if sourceValue == nil {
		item.SourceValue = json.RawMessage("null")
	} else {
		item.SourceValue = mustMarshalValue(t, sourceValue)
	}
	if resolution != "" {
		item.Resolution = strPointer(resolution)
	}
	return item
}

// saveLineItem appends one item to the live session.
func saveLineItem(t *testing.T, database *Store, ctx context.Context, poID, actorID int64, item POReconciliationItem) POReconciliationSession {
	t.Helper()
	loaded, err := database.GetPOReconciliation(ctx, poID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	loaded.Items = append(loaded.Items, item)
	saved, err := database.SavePOReconciliation(ctx, loaded, actorID)
	if err != nil {
		t.Fatalf("save session item %q: %v", item.Key, err)
	}
	return saved
}

// draftUpdateWithLines saves the given lines (carrying IDs and client keys)
// as one ordinary draft save.
func draftUpdateWithLines(t *testing.T, database *Store, ctx context.Context, draft PurchaseOrder, user AuthUser, lines []PurchaseOrderItem) PurchaseOrder {
	t.Helper()
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID:              draft.ID,
		BillingAddress:  draft.BillingAddress,
		DeliveryAddress: draft.DeliveryAddress,
		ContactEmail:    draft.ContactEmail,
		PONumber:        draft.PONumber,
		ReceivedDate:    datePointer(draft.ReceivedDate),
		PaymentTerms:    draft.PaymentTerms,
		Incoterms:       draft.Incoterms,
		UpdatedBy:       user.ID,
		Items:           lines,
	})
	if err != nil {
		t.Fatalf("draft save: %v", err)
	}
	return updated
}

func TestPOReconciliationIdentitySurvivesReorder(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_reorder")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines reorder")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	if len(draft.Items) != 2 || draft.Items[0].ID < 1 || draft.Items[1].ID < 1 {
		t.Fatalf("draft lines = %+v; want two persisted rows with ids", draft.Items)
	}
	idA, idB := draft.Items[0].ID, draft.Items[1].ID

	swapped := []PurchaseOrderItem{draft.Items[1], draft.Items[0]}
	updated := draftUpdateWithLines(t, database, ctx, draft, user, swapped)
	if updated.Items[0].ID != idB || updated.Items[1].ID != idA {
		t.Fatalf("reordered ids = %d/%d; want the persisted %d/%d",
			updated.Items[0].ID, updated.Items[1].ID, idB, idA)
	}
	if updated.Items[0].LineNumber != 1 || updated.Items[1].LineNumber != 2 {
		t.Fatalf("reordered numbers = %d/%d; want 1/2",
			updated.Items[0].LineNumber, updated.Items[1].LineNumber)
	}

	// A review item keyed to the stable identity follows the row, not the
	// position: quantity of A stays quantity of A after another reorder.
	keyA := purchaseOrderLineKeyForRow(idA)
	item := lineDifferenceItem(t, keyA, "line:quantity", "quantity", "Line 2 quantity", 20000, 20000, "1")
	saveLineItem(t, database, ctx, draft.ID, user.ID, item)
	back := []PurchaseOrderItem{updated.Items[1], updated.Items[0]}
	again := draftUpdateWithLines(t, database, ctx, updated, user, back)
	if again.Items[0].ID != idA || again.Items[1].ID != idB {
		t.Fatalf("second reorder ids = %d/%d; want %d/%d",
			again.Items[0].ID, again.Items[1].ID, idA, idB)
	}
	rehydrated, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	found := false
	for _, stored := range rehydrated.Items {
		if stored.Key == "line:"+keyA+":quantity" {
			found = true
		}
	}
	if !found {
		t.Errorf("stable item key %q missing after two reorders: %+v", "line:"+keyA+":quantity", rehydrated.Items)
	}

	// An unknown row id is refused, and one row cannot be claimed twice.
	ghost := append([]PurchaseOrderItem{}, again.Items...)
	ghost[0].ID = 999999
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: ghost,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("unknown row id = %v; want ErrPurchaseOrderInvalid", err)
	}
	dupe := append([]PurchaseOrderItem{}, again.Items...)
	dupe[1].ID = dupe[0].ID
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: dupe,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("duplicate row claim = %v; want ErrPurchaseOrderInvalid", err)
	}
}

func TestPOReconciliationEditorAliasBindsAcrossSaves(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_alias")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines alias")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines = append(lines, PurchaseOrderItem{
		ClientKey:      "client:9d1a2b3c-4d5e-4f6a-8b9c-0d1e2f3a4b5c",
		Description:    "Editor freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 50,
	})
	updated := draftUpdateWithLines(t, database, ctx, draft, user, lines)
	var freight PurchaseOrderItem
	for _, line := range updated.Items {
		if line.Description == "Editor freight" {
			freight = line
		}
	}
	if freight.ID < 1 {
		t.Fatalf("freight line = %+v; want a persisted row", freight)
	}
	if freight.ClientKey != "client:9d1a2b3c-4d5e-4f6a-8b9c-0d1e2f3a4b5c" {
		t.Errorf("freight alias = %q; want the editor alias kept", freight.ClientKey)
	}
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	bound := false
	for _, identity := range session.LineIdentities {
		if identity.Key == freight.ClientKey {
			bound = true
			if identity.ID == nil || *identity.ID != freight.ID {
				t.Errorf("alias binds row %v; want %d", identity.ID, freight.ID)
			}
		}
	}
	if !bound {
		t.Errorf("alias %q missing from identities %+v", freight.ClientKey, session.LineIdentities)
	}

	// A second line claiming the same alias is refused, at the input and at
	// the row: the alias names exactly one row of its purchase order.
	second := append([]PurchaseOrderItem{}, updated.Items...)
	second = append(second, PurchaseOrderItem{
		ClientKey:      freight.ClientKey,
		Description:    "Copycat freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 60,
	})
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: second,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("duplicate alias claim = %v; want ErrPurchaseOrderInvalid", err)
	}
}

func TestPOReconciliationLineRemovalResolvesLineRemoved(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_remove")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines remove")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	idA, idB := draft.Items[0].ID, draft.Items[1].ID

	keyB := purchaseOrderLineKeyForRow(idB)
	item := lineDifferenceItem(t, keyB, "line:quantity", "quantity", "Line 2 quantity", 20000, 10000, "3")
	saveLineItem(t, database, ctx, draft.ID, user.ID, item)

	updated := draftUpdateWithLines(t, database, ctx, draft, user, draft.Items[:1])
	if len(updated.Items) != 1 || updated.Items[0].ID != idA {
		t.Fatalf("lines after removal = %+v; want only row %d", updated.Items, idA)
	}
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	removed := false
	for _, identity := range session.LineIdentities {
		if identity.ID != nil && *identity.ID == idB {
			removed = removed || identity.Removed
		}
	}
	if !removed {
		t.Errorf("identities %+v keep row %d live; want Removed", session.LineIdentities, idB)
	}
	resolved := false
	for _, stored := range session.Items {
		if stored.Key != "line:"+keyB+":quantity" {
			continue
		}
		resolved = true
		if stored.Status != POReconciliationStatusResolved || stored.Resolution == nil ||
			*stored.Resolution != POReconciliationResolutionRemoved {
			t.Errorf("removed item = %s/%v; want resolved line_removed", stored.Status, stored.Resolution)
		}
		if len(stored.History) != 2 {
			t.Errorf("history has %d events; want the decision plus the removal", len(stored.History))
		}
		if string(stored.POValue) != "20000" || string(stored.SourceValue) != "10000" {
			t.Errorf("removed values = %s/%s; want the retained 20000/10000", stored.POValue, stored.SourceValue)
		}
	}
	if !resolved {
		t.Errorf("no item for the removed line; want its key, values and history retained")
	}
}

func TestPOReconciliationDuplicateRemovalUnblocksSurvivor(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_dupe")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines duplicate")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	idA, idB := draft.Items[0].ID, draft.Items[1].ID

	// Both lines claim source line 1: a blocking duplicate (R-08).
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	one := 1
	lines[0].QuotationItemNo = &one
	lines[0].QuotationSubNo = nil
	lines[1].QuotationItemNo = &one
	lines[1].QuotationSubNo = nil
	matched := draftUpdateWithLines(t, database, ctx, draft, user, lines)

	keyA, keyB := purchaseOrderLineKeyForRow(idA), purchaseOrderLineKeyForRow(idB)
	dupA := lineMatchItem(t, "line:"+keyA+":match", "line:match", POReconciliationKindDuplicateMatch,
		"Line 1 match", strPointer(keyA), strPointer("1"), "1", nil,
		POReconciliationStatusUnresolved, "")
	dupB := lineMatchItem(t, "line:"+keyB+":match", "line:match", POReconciliationKindDuplicateMatch,
		"Line 2 match", strPointer(keyB), strPointer("1"), "1", nil,
		POReconciliationStatusUnresolved, "")
	saveLineItem(t, database, ctx, draft.ID, user.ID, dupA)
	saveLineItem(t, database, ctx, draft.ID, user.ID, dupB)

	// Removing one claimant resolves its item as line_removed and unblocks
	// the survivor as match_corrected; the survivor stays matched.
	survivor := draftUpdateWithLines(t, database, ctx, matched, user, matched.Items[:1])
	if len(survivor.Items) != 1 || survivor.Items[0].ID != idA {
		t.Fatalf("survivor = %+v; want only row %d", survivor.Items, idA)
	}
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	byKey := map[string]POReconciliationItem{}
	for _, stored := range session.Items {
		byKey[stored.Key] = stored
	}
	removed, ok := byKey["line:"+keyB+":match"]
	if !ok || removed.Status != POReconciliationStatusResolved || removed.Resolution == nil ||
		*removed.Resolution != POReconciliationResolutionRemoved {
		t.Errorf("removed duplicate = %+v; want resolved line_removed", removed)
	}
	surviving, ok := byKey["line:"+keyA+":match"]
	if !ok || surviving.Status != POReconciliationStatusResolved || surviving.Resolution == nil ||
		*surviving.Resolution != POReconciliationResolutionCorrected {
		t.Errorf("surviving duplicate = %+v; want resolved match_corrected", surviving)
	}
}

func taxTripleValue(t *testing.T, zone, taxType, service string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(POReconciliationTaxBasis{TaxZone: zone, TaxType: taxType, ServiceCategory: service})
	if err != nil {
		t.Fatalf("marshal tax triple: %v", err)
	}
	return raw
}

func findTaxItem(t *testing.T, session POReconciliationSession) POReconciliationItem {
	t.Helper()
	for _, item := range session.Items {
		if item.Field == "link:taxBasis" {
			return item
		}
	}
	t.Fatalf("no link:taxBasis item in session %+v", session.Items)
	return POReconciliationItem{}
}

func TestPOReconciliationTaxItemSeededAtCreation(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_taxcreate")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines tax create")
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
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	tax := findTaxItem(t, session)
	if tax.Key != "link:taxBasis" || tax.Kind != POReconciliationKindTaxBasis {
		t.Errorf("tax item = %+v; want key link:taxBasis kind tax_basis", tax)
	}
	if tax.LineKey != nil || tax.SourceLineRef != nil {
		t.Errorf("tax item names a line; want session context with null refs")
	}
	if tax.Status != POReconciliationStatusResolved || tax.Resolution == nil ||
		*tax.Resolution != POReconciliationResolutionUsedQuote {
		t.Errorf("tax item = %s/%v; want resolved used_quotation (source adoption)", tax.Status, tax.Resolution)
	}
	want := taxTripleValue(t, "FTZ", "Goods", "")
	var wantTriple, poTriple, sourceTriple POReconciliationTaxBasis
	if err := json.Unmarshal(want, &wantTriple); err != nil {
		t.Fatalf("decode want triple: %v", err)
	}
	if err := json.Unmarshal(tax.POValue, &poTriple); err != nil {
		t.Fatalf("decode tax PO value: %v", err)
	}
	if err := json.Unmarshal(tax.SourceValue, &sourceTriple); err != nil {
		t.Fatalf("decode tax source value: %v", err)
	}
	if poTriple != wantTriple || sourceTriple != wantTriple {
		t.Errorf("tax values = %+v/%+v; want the adopted %+v on both sides", poTriple, sourceTriple, wantTriple)
	}
	if len(tax.History) != 1 || tax.History[0].ActorID != user.ID {
		t.Errorf("tax history = %+v; want one server-attributed event", tax.History)
	}
}

func TestPOReconciliationTaxItemSeededAtLinkWithKeepChoice(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_taxlink")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	// Move the blank draft to a NonFTZ basis before linking, so the link
	// records an explicit keep-PO choice against an FTZ source.
	update := blankDraftUpdate(draft, draft.Version, user)
	update.TaxZone = "NonFTZ"
	update.TaxType = "Goods"
	moved, err := database.UpdatePurchaseOrderDraft(ctx, update)
	if err != nil {
		t.Fatalf("move blank draft tax basis: %v", err)
	}
	quotation := seedLinkQuotation(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Tax Link", Subject: "Tax link",
		Address: "Jl. Source", Currency: "IDR", TaxZone: "FTZ", TaxType: "Goods",
		UnitPrice: 200, PaymentTermDays: 45,
	})
	review := linkReviewSource(t, database, quotation)
	saved, err := database.UpdatePurchaseOrderDraft(ctx, linkableDraftUpdate(moved, user, review))
	if err != nil {
		t.Fatalf("save before link: %v", err)
	}
	linked, err := database.LinkPurchaseOrderDraftSource(ctx, draftLinkInput(
		saved, quotation, user,
		[]PurchaseOrderDecision{
			{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
			{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
			{Key: "header:discountPercent", Choice: PurchaseOrderChoicePO},
		}, []string{"1", "3"}, []int{1}, PurchaseOrderDraftLinkTaxKeep))
	if err != nil {
		t.Fatalf("link with keep choice: %v", err)
	}
	if linked.TaxZone != "NonFTZ" {
		t.Fatalf("linked tax zone = %q; want the kept NonFTZ basis", linked.TaxZone)
	}
	session, err := database.GetPOReconciliation(ctx, linked.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	tax := findTaxItem(t, session)
	if tax.Resolution == nil || *tax.Resolution != POReconciliationResolutionKeptPO {
		t.Errorf("tax resolution = %v; want kept_po", tax.Resolution)
	}
	assertTriple := func(name string, raw json.RawMessage, zone, taxType, service string) {
		t.Helper()
		var got, want POReconciliationTaxBasis
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		want = POReconciliationTaxBasis{TaxZone: zone, TaxType: taxType, ServiceCategory: service}
		if got != want {
			t.Errorf("%s = %+v; want %+v", name, got, want)
		}
	}
	assertTriple("tax PO value", tax.POValue, "NonFTZ", "Goods", "")
	assertTriple("tax source value", tax.SourceValue, "FTZ", "Goods", "")

	// The generic writer refuses any tax mutation, while an identical
	// resubmission stays idempotent.
	loaded, err := database.GetPOReconciliation(ctx, linked.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	for i, item := range loaded.Items {
		if item.Field != "link:taxBasis" {
			continue
		}
		loaded.Items[i].POValue = taxTripleValue(t, "NonFTZ", "Services", "NonConstruction")
	}
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err == nil {
		t.Error("rewriting the link-time tax basis succeeded; want a read-only refusal")
	} else if !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("tax rewrite = %v; want ErrPurchaseOrderInvalid", err)
	}
	fresh, err := database.GetPOReconciliation(ctx, linked.ID)
	if err != nil {
		t.Fatalf("reload after refusal: %v", err)
	}
	if _, err := database.SavePOReconciliation(ctx, fresh, user.ID); err != nil {
		t.Errorf("identical tax resubmission = %v; want idempotent acceptance", err)
	}
}

func TestPOReconciliationLineItemValidation(t *testing.T) {
	newDraft := func(t *testing.T, schema string) (*Store, context.Context, AuthUser, PurchaseOrder) {
		database := openTestSchema(t, schema)
		quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines validation")
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
		return database, ctx, user, draft
	}
	newExtraLine := func(t *testing.T, database *Store, ctx context.Context, draft PurchaseOrder, user AuthUser) (PurchaseOrder, string) {
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		lines = append(lines, PurchaseOrderItem{
			ClientKey:      "client:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			Description:    "Unmatched freight",
			QuantityScaled: 10000,
			UoM:            "Lot",
			UnitPriceMinor: 50,
		})
		updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
			ID: draft.ID, UpdatedBy: user.ID, Items: lines,
		})
		if err != nil {
			t.Fatalf("add extra line: %v", err)
		}
		var key string
		for _, line := range updated.Items {
			if line.Description == "Unmatched freight" {
				// First-seen wins: this row entered with an editor alias,
				// so the recorded session key is the alias, not id:<row>.
				key = line.ClientKey
			}
		}
		if key == "" {
			t.Fatalf("extra line has no editor alias: %+v", updated.Items)
		}
		return updated, key
	}

	t.Run("unknown client identity refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val1")
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{
			lineDifferenceItem(t, "client:never-saved", "line:quantity", "quantity", "Line 9 quantity", 10000, 10000, "1"),
		}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("unknown client identity = %v; want ErrPurchaseOrderInvalid with draft-save guidance", err)
		}
	})
	t.Run("unknown row id refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val2")
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{
			lineDifferenceItem(t, "id:999999", "line:quantity", "quantity", "Line 9 quantity", 10000, 10000, "1"),
		}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("unknown row id = %v; want ErrPurchaseOrderInvalid", err)
		}
	})
	t.Run("key field mismatch refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val3")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		bad := lineDifferenceItem(t, key, "line:uom", "quantity", "Line 1 quantity", 10000, 10000, "1")
		loaded.Items = []POReconciliationItem{bad}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("key/field mismatch = %v; want ErrPurchaseOrderInvalid", err)
		}
	})
	t.Run("wrong kind refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val4")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		bad := lineDifferenceItem(t, key, "line:quantity", "quantity", "Line 1 quantity", 10000, 10000, "1")
		bad.Kind = POReconciliationKindMissingLine
		loaded.Items = []POReconciliationItem{bad}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("wrong kind = %v; want ErrPurchaseOrderInvalid", err)
		}
	})
	t.Run("derived totals boundary refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val5")
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{{
			Key: "derived:totals", Field: "derived:totals", Kind: POReconciliationKindDifference,
			Label: "Totals", POValue: mustMarshalValue(t, 200), SourceValue: mustMarshalValue(t, 200),
			Status: POReconciliationStatusUnresolved,
		}}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("derived totals item = %v; want ErrPurchaseOrderInvalid (totals stay derived)", err)
		}
	})
	t.Run("malformed discount refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val6")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		// Key casing follows Go's encoding/json semantics (case-insensitive
		// match, PascalCase on the wire); shape, not casing, is enforced.
		for name, discount := range map[string]string{
			"unknown field": `{"Enabled":true,"Percent":5,"Extra":1}`,
			"percent range": `{"Enabled":true,"Percent":101}`,
			"text percent":  `{"Enabled":true,"Percent":"5"}`,
		} {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
				POValue: json.RawMessage(discount), SourceValue: json.RawMessage(`{"Enabled":false,"Percent":0}`),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err == nil {
				t.Errorf("discount %s saved; want a refusal", name)
			}
		}
	})
	t.Run("malformed quantity refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val7")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		for name, raw := range map[string]string{
			"fraction": `1.5`,
			"text":     `"3"`,
			"zero":     `0`,
			"negative": `-10000`,
		} {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":quantity", Field: "line:quantity", Kind: POReconciliationKindDifference,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 quantity",
				POValue: json.RawMessage(raw), SourceValue: json.RawMessage(`20000`),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err == nil {
				t.Errorf("quantity %s (%s) saved; want a refusal", name, raw)
			}
		}
	})
	t.Run("line happy path round-trips", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_lines_val8")
		_, key := newExtraLine(t, database, ctx, draft, user)
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		quantity := lineDifferenceItem(t, key, "line:quantity", "quantity", "Line 3 quantity", 15000, 15000, "1")
		extra := lineMatchItem(t, "line:"+key+":match", "line:match", POReconciliationKindExtraLine,
			"Line 3 match", strPointer(key), nil, nil, nil,
			POReconciliationStatusUnresolved, "")
		loaded.Items = []POReconciliationItem{quantity, extra}
		saved, err := database.SavePOReconciliation(ctx, loaded, user.ID)
		if err != nil {
			t.Fatalf("save line items: %v", err)
		}
		if len(saved.Items) < 3 {
			t.Fatalf("saved items = %d; want tax plus the two line items", len(saved.Items))
		}
		rehydrated, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("rehydrate: %v", err)
		}
		byKey := map[string]POReconciliationItem{}
		for _, stored := range rehydrated.Items {
			byKey[stored.Key] = stored
		}
		if _, ok := byKey["line:"+key+":quantity"]; !ok {
			t.Errorf("quantity item missing after reopen: %+v", rehydrated.Items)
		}
		if _, ok := byKey["line:"+key+":match"]; !ok {
			t.Errorf("extra item missing after reopen: %+v", rehydrated.Items)
		}
	})
}

func TestPOReconciliationFinalSaveLineBlockers(t *testing.T) {
	newGateDraft := func(t *testing.T, schema string) (*Store, context.Context, AuthUser, PurchaseOrder) {
		database := openTestSchema(t, schema)
		quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines gate")
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
		return database, ctx, user, draft
	}
	// finalizeWithDecisions reloads the draft and saves it with the given
	// positional decisions plus document confirmation.
	finalizeWithDecisions := func(t *testing.T, database *Store, ctx context.Context, draft PurchaseOrder, user AuthUser, number string, decisions []PurchaseOrderDecision, missing []string, extra []int) error {
		fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		input := finalizeInput(fresh, user, number)
		input.Decisions = decisions
		input.AcknowledgedMissing = missing
		input.AcknowledgedExtra = extra
		_, err = database.FinalizePurchaseOrder(ctx, input)
		return err
	}

	t.Run("unresolved line item blocks and names its key", func(t *testing.T) {
		database, ctx, user, draft := newGateDraft(t, "vera_po_gate_unresolved")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		saveLineItem(t, database, ctx, draft.ID, user.ID,
			lineDifferenceItem(t, key, "line:quantity", "quantity", "Line 1 quantity", 20000, 20000, "1"))
		err := finalizeWithDecisions(t, database, ctx, draft, user, "PO-LINE-1", nil, nil, nil)
		if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
			t.Fatalf("finalize = %v; want ErrPurchaseOrderUnresolvedDifference", err)
		}
		if !strings.Contains(err.Error(), "line:"+key+":quantity") {
			t.Errorf("error = %v; want the specific item key named", err)
		}
	})

	t.Run("duplicate still duplicated blocks", func(t *testing.T) {
		database, ctx, user, draft := newGateDraft(t, "vera_po_gate_dupe")
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		one := 1
		lines[0].QuotationItemNo = &one
		lines[0].QuotationSubNo = nil
		lines[1].QuotationItemNo = &one
		lines[1].QuotationSubNo = nil
		matched := draftUpdateWithLines(t, database, ctx, draft, user, lines)
		keyA := purchaseOrderLineKeyForRow(matched.Items[0].ID)
		keyB := purchaseOrderLineKeyForRow(matched.Items[1].ID)
		saveLineItem(t, database, ctx, draft.ID, user.ID, lineMatchItem(t,
			"line:"+keyA+":match", "line:match", POReconciliationKindDuplicateMatch,
			"Line 1 match", strPointer(keyA), strPointer("1"), "1", nil,
			POReconciliationStatusResolved, POReconciliationResolutionCorrected))
		saveLineItem(t, database, ctx, draft.ID, user.ID, lineMatchItem(t,
			"line:"+keyB+":match", "line:match", POReconciliationKindDuplicateMatch,
			"Line 2 match", strPointer(keyB), strPointer("1"), "1", nil,
			POReconciliationStatusResolved, POReconciliationResolutionCorrected))
		err := finalizeWithDecisions(t, database, ctx, draft, user, "PO-LINE-2",
			nil, []string{"3"}, nil)
		if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
			t.Fatalf("finalize = %v; want the duplicate refusal", err)
		}
		// The live duplicate is refused by the decisions validator before the
		// session gate runs; its message names the duplicating lines. The
		// gate's own still-duplicated branch (a match_corrected claim while
		// the clash persists) is covered directly below.
		if !strings.Contains(err.Error(), "match the same quotation line") {
			t.Errorf("error = %v; want the duplicate blocker named", err)
		}
	})

	t.Run("corrected claim while still duplicated blocks at the gate", func(t *testing.T) {
		database := openTestSchema(t, "vera_po_gate_dupegate")
		quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines gate direct")
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
		pool, err := database.poolOrError()
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		schema := database.configSchema()
		org, err := database.RequireOrg(ctx)
		if err != nil {
			t.Fatalf("org: %v", err)
		}
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		one := 1
		lines[0].QuotationItemNo = &one
		lines[0].QuotationSubNo = nil
		lines[1].QuotationItemNo = &one
		lines[1].QuotationSubNo = nil
		matched := draftUpdateWithLines(t, database, ctx, draft, user, lines)
		keyA := purchaseOrderLineKeyForRow(matched.Items[0].ID)
		saveLineItem(t, database, ctx, draft.ID, user.ID, lineMatchItem(t,
			"line:"+keyA+":match", "line:match", POReconciliationKindDuplicateMatch,
			"Line 1 match", strPointer(keyA), strPointer("1"), "1", nil,
			POReconciliationStatusResolved, POReconciliationResolutionCorrected))
		source, err := loadPurchaseOrderSourceDetail(ctx, pool, schema, quotation.ID, quotation.RevisionNumber)
		if err != nil {
			t.Fatalf("load source: %v", err)
		}
		entered := PurchaseOrderReviewState{Lines: matched.Items}
		comparison := PurchaseOrderComparison{DuplicateLineNumbers: []int{1, 2}}
		err = enforceReconciliationFinalGate(ctx, pool, schema, org.ID, draft.ID, comparison, nil, entered, source)
		if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
			t.Fatalf("gate = %v; want the still-duplicated refusal", err)
		}
		if !strings.Contains(err.Error(), "still duplicates") {
			t.Errorf("error = %v; want the duplicate blocker named", err)
		}
	})

	t.Run("item-less unknown reference blocks at the gate", func(t *testing.T) {
		database := openTestSchema(t, "vera_po_gate_unknowngate")
		quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines unknown gate")
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
		pool, err := database.poolOrError()
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		schema := database.configSchema()
		org, err := database.RequireOrg(ctx)
		if err != nil {
			t.Fatalf("org: %v", err)
		}
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		unknown := 99
		lines[1].QuotationItemNo = &unknown
		lines[1].QuotationSubNo = nil
		lines[1].LineNumber = 2
		source, err := loadPurchaseOrderSourceDetail(ctx, pool, schema, quotation.ID, quotation.RevisionNumber)
		if err != nil {
			t.Fatalf("load source: %v", err)
		}
		entered := PurchaseOrderReviewState{Lines: lines}
		comparison := PurchaseOrderComparison{ExtraLineNumbers: []int{2}}
		err = enforceReconciliationFinalGate(ctx, pool, schema, org.ID, draft.ID, comparison, nil, entered, source)
		if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
			t.Fatalf("gate = %v; want the unknown-reference refusal", err)
		}
		if !strings.Contains(err.Error(), "correct the reference") {
			t.Errorf("error = %v; want the correction named", err)
		}
	})

	t.Run("unknown reference acknowledgement cannot clear", func(t *testing.T) {
		database, ctx, user, draft := newGateDraft(t, "vera_po_gate_unknown")
		// A plain extra line (no reference) is acknowledged, but its recorded
		// item claims the unknown reference 99: acknowledgement alone must
		// never clear an unknown reference (R-02).
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		lines = lines[:1]
		lines = append(lines, PurchaseOrderItem{
			Description: "Freight", QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 0,
		})
		changed := draftUpdateWithLines(t, database, ctx, draft, user, lines)
		var key string
		for _, line := range changed.Items {
			if line.Description == "Freight" {
				key = purchaseOrderLineKeyForRow(line.ID)
			}
		}
		saveLineItem(t, database, ctx, draft.ID, user.ID, lineMatchItem(t,
			"line:"+key+":match", "line:match", POReconciliationKindExtraLine,
			"Line 2 match", strPointer(key), nil, "99", nil,
			POReconciliationStatusResolved, POReconciliationResolutionAckExtra))
		err := finalizeWithDecisions(t, database, ctx, draft, user, "PO-LINE-3",
			nil, []string{"3"}, []int{2})
		if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
			t.Fatalf("finalize = %v; want the unknown-reference refusal", err)
		}
		if !strings.Contains(err.Error(), "correct the reference") {
			t.Errorf("error = %v; want the correction named, not a bare acknowledgement path", err)
		}
	})

	t.Run("stale acknowledgement blocks", func(t *testing.T) {
		database, ctx, user, draft := newGateDraft(t, "vera_po_gate_stale")
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		lines = lines[:1]
		lines = append(lines, PurchaseOrderItem{
			Description: "Freight", QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 0,
		})
		changed := draftUpdateWithLines(t, database, ctx, draft, user, lines)
		var key string
		for _, line := range changed.Items {
			if line.Description == "Freight" {
				key = purchaseOrderLineKeyForRow(line.ID)
			}
		}
		saveLineItem(t, database, ctx, draft.ID, user.ID, lineMatchItem(t,
			"line:"+key+":match", "line:match", POReconciliationKindExtraLine,
			"Line 2 match", strPointer(key), nil, nil, nil,
			POReconciliationStatusResolved, POReconciliationResolutionAckExtra))
		// Correct the line to match source line 3: the acknowledgement is now
		// stale, and the save must say so instead of passing silently.
		fixed := append([]PurchaseOrderItem{}, changed.Items...)
		three := 3
		for i := range fixed {
			if fixed[i].Description == "Freight" {
				fixed[i].QuotationItemNo = &three
				fixed[i].Description = "Zero-value line"
				fixed[i].UnitPriceMinor = 0
			}
		}
		draftUpdateWithLines(t, database, ctx, changed, user, fixed)
		err := finalizeWithDecisions(t, database, ctx, draft, user, "PO-LINE-4", nil, nil, nil)
		if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
			t.Fatalf("finalize = %v; want the stale-acknowledgement refusal", err)
		}
		if !strings.Contains(err.Error(), "no longer extra") {
			t.Errorf("error = %v; want the staleness named", err)
		}
	})

	t.Run("missing unresolved blocks", func(t *testing.T) {
		database, ctx, user, draft := newGateDraft(t, "vera_po_gate_missing")
		changed := draftUpdateWithLines(t, database, ctx, draft, user, draft.Items[:1])
		_ = changed
		saveLineItem(t, database, ctx, draft.ID, user.ID, lineMatchItem(t,
			"source:3:match", "source:match", POReconciliationKindMissingLine,
			"Quotation line 3 missing", nil, strPointer("3"), nil, "3",
			POReconciliationStatusUnresolved, ""))
		err := finalizeWithDecisions(t, database, ctx, draft, user, "PO-LINE-5",
			nil, []string{"3"}, nil)
		if !errors.Is(err, ErrPurchaseOrderUnresolvedDifference) {
			t.Fatalf("finalize = %v; want the missing-line refusal", err)
		}
	})

	t.Run("resolved line flow finalizes", func(t *testing.T) {
		database, ctx, user, draft := newGateDraft(t, "vera_po_gate_happy")
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		lines[0].QuantityScaled = 30000
		changed := draftUpdateWithLines(t, database, ctx, draft, user, lines)
		key := purchaseOrderLineKeyForRow(changed.Items[0].ID)
		item := lineDifferenceItem(t, key, "line:quantity", "quantity", "Line 1 quantity", 30000, 20000, "1")
		item.Status = POReconciliationStatusResolved
		item.Resolution = strPointer(POReconciliationResolutionKeptPO)
		saveLineItem(t, database, ctx, draft.ID, user.ID, item)
		err := finalizeWithDecisions(t, database, ctx, draft, user, "PO-LINE-6",
			[]PurchaseOrderDecision{{Key: "line:1:quantity", Choice: PurchaseOrderChoicePO}}, nil, nil)
		if err != nil {
			t.Fatalf("finalize with a resolved kept line: %v", err)
		}
		saved, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		if saved.State != PurchaseOrderStateSaved {
			t.Errorf("state = %q; want saved", saved.State)
		}
	})
}

func TestPOReconciliationUnknownReferenceWithoutItemRefused(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_unknownbare")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines unknown bare")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	// An unknown reference cannot even be saved on a live draft: correction
	// is required before any acknowledgement could exist (R-02).
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	unknown := 99
	lines[1].QuotationItemNo = &unknown
	lines[1].QuotationSubNo = nil
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: lines,
	}); !errors.Is(err, ErrPurchaseOrderBadReference) {
		t.Fatalf("draft save with an unknown reference = %v; want ErrPurchaseOrderBadReference", err)
	}
	// A hand-written unknown row still cannot finalize: the saved-reference
	// validator is the first backstop, and the session gate names the same
	// blocker for acknowledged unknowns.
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	if _, err := pool.Exec(ctx, `
		INSERT INTO `+schema+`.purchase_order_items (
			purchase_order_id, line_number, description, quotation_item_no,
			quotation_sub_no, quantity_scaled, uom, unit_price_minor,
			discount_enabled, discount_percent, gross_minor, discount_minor, net_minor
		) VALUES ($1, 3, 'Ghost', 99, NULL, 10000, 'Pc', 100, FALSE, 0, 100, 0, 100)`,
		draft.ID); err != nil {
		t.Fatalf("hand-write unknown row: %v", err)
	}
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	input := finalizeInput(fresh, user, "PO-UNKNOWN-1")
	input.AcknowledgedMissing = []string{"3"}
	input.AcknowledgedExtra = []int{2, 3}
	if _, err := database.FinalizePurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderBadReference) {
		t.Fatalf("finalize = %v; want ErrPurchaseOrderBadReference", err)
	}
}

func TestPOReconciliationEditorIdentityRoundTrip(t *testing.T) {
	database := openTestSchema(t, "vera_po_lines_dto")
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "Lines DTO")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	for _, line := range draft.Items {
		if line.ID < 1 {
			t.Fatalf("read line = %+v; want the persisted id exposed", line)
		}
	}
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines = append(lines, PurchaseOrderItem{
		ClientKey:      "client:11111111-2222-3333-4444-555555555555",
		Description:    "Keyed freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 70,
	})
	updated := draftUpdateWithLines(t, database, ctx, draft, user, lines)
	seenAlias := false
	for _, line := range updated.Items {
		if line.ClientKey == "client:11111111-2222-3333-4444-555555555555" {
			seenAlias = true
			if line.ID < 1 {
				t.Errorf("aliased line has no id after save")
			}
		}
	}
	if !seenAlias {
		t.Fatalf("editor alias missing after reopen: %+v", updated.Items)
	}
}

// findSessionItem returns the session item with the given key, failing the
// test when no such item exists. Sessions now seed the link-time tax item at
// creation, so tests look items up by key instead of assuming positions.
func findSessionItem(t *testing.T, items []POReconciliationItem, key string) POReconciliationItem {
	t.Helper()
	for _, item := range items {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("session item %q missing from %+v", key, items)
	return POReconciliationItem{}
}
