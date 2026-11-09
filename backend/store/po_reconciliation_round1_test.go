package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
)

// Round-1 regressions for the #517 independent review findings (B1-B4).
// Each test first reproduced RED against f4304d7 and now pins the repaired
// desired behavior. Seams: the public Store methods plus Postgres behavior;
// literal expectations come from the frozen contract and the fix plan's
// canonical wire table.

// round1Update saves lines as one ordinary draft save, filling the version
// the record currently carries when the test does not state one.
func round1Update(t *testing.T, database *Store, ctx context.Context, draft PurchaseOrder, user AuthUser, lines []PurchaseOrderItem) PurchaseOrder {
	t.Helper()
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: lines,
	})
	if err != nil {
		t.Fatalf("draft save: %v", err)
	}
	return updated
}

// B1: adopting an editor alias onto a row that already has a first-seen
// session key must not emit a second identity for that row. Save, session
// save, reload, session save again must all succeed with one identity per
// row and unchanged decisions/history.
func TestPOReconciliationAliasAdoptionPreservesFirstSeenKey(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_aliasadopt")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 alias adoption")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	original, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if len(original.LineIdentities) != len(draft.Items) {
		t.Fatalf("seeded identities = %d; want one per row (%d)", len(original.LineIdentities), len(draft.Items))
	}
	// An ordinary draft update adopts a fresh editor alias onto each
	// persisted row, the way the editor's DTO-shaped payload does.
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	for i := range lines {
		lines[i].ClientKey = "client:adopt-" + string(rune('0'+i))
	}
	updated := round1Update(t, database, ctx, draft, user, lines)
	for i, line := range updated.Items {
		if line.ID != draft.Items[i].ID {
			t.Fatalf("row ids churned on alias adoption: %+v", updated.Items)
		}
	}

	adopted, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after adoption: %v", err)
	}
	if len(adopted.LineIdentities) != len(original.LineIdentities) {
		t.Fatalf("identities = %+v; want the %d first-seen keys unchanged, no second identity per row",
			adopted.LineIdentities, len(original.LineIdentities))
	}
	for i, identity := range adopted.LineIdentities {
		if identity.Key != original.LineIdentities[i].Key {
			t.Errorf("identity %d = %q; want first-seen %q", i, identity.Key, original.LineIdentities[i].Key)
		}
	}
	// The reloaded server session must be writable again: this is the save
	// that failed with "line id claimed twice" before the fix. Record a
	// decision first so the round trip must preserve keys and history too.
	decided, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload for decision: %v", err)
	}
	firstKey := original.LineIdentities[0].Key
	decided.Items = []POReconciliationItem{{
		Key: "line:" + firstKey + ":quantity", Field: "line:quantity", Kind: POReconciliationKindDifference,
		LineKey: strPointer(firstKey), SourceLineRef: strPointer("1"), Label: "Line 1 quantity",
		POValue: mustMarshalValue(t, 20000), SourceValue: mustMarshalValue(t, 20000),
		Status: POReconciliationStatusResolved, Resolution: strPointer(POReconciliationResolutionKeptPO),
	}}
	if _, err := database.SavePOReconciliation(ctx, decided, user.ID); err != nil {
		t.Fatalf("record decision: %v", err)
	}
	adopted, err = database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after decision: %v", err)
	}
	if _, err := database.SavePOReconciliation(ctx, adopted, user.ID); err != nil {
		t.Fatalf("second session save: %v", err)
	}
	reopened, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("final reload: %v", err)
	}
	if len(reopened.LineIdentities) != len(original.LineIdentities) {
		t.Errorf("identities grew to %+v across saves", reopened.LineIdentities)
	}
	kept := findSessionItem(t, reopened.Items, "line:"+firstKey+":quantity")
	if kept.Status != POReconciliationStatusResolved || kept.Resolution == nil ||
		*kept.Resolution != POReconciliationResolutionKeptPO || len(kept.History) != 1 {
		t.Errorf("decision did not survive the round trip: %+v", kept)
	}
	tax := findSessionItem(t, reopened.Items, "link:taxBasis")
	if tax.Status != POReconciliationStatusResolved || len(tax.History) != 1 {
		t.Errorf("tax item did not survive the round trip: %+v", tax)
	}
}

// B2: a removed (retired) client alias cannot identify an unrelated new row.
// The old mapping stays Removed with its history; reuse is refused against
// the retained mappings for the PO, without rebinding or deleting history.
func TestPOReconciliationRetiredAliasCannotClaimNewRow(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_aliasreuse")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 alias reuse")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines = append(lines, PurchaseOrderItem{
		ClientKey:      "client:retired-alias",
		Description:    "First freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 50,
	})
	updated := round1Update(t, database, ctx, draft, user, lines)
	oldID := updated.Items[2].ID

	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	extraKey := "line:client:retired-alias:match"
	loaded.Items = append(loaded.Items, POReconciliationItem{
		Key:         extraKey,
		Field:       "line:match",
		Kind:        POReconciliationKindExtraLine,
		LineKey:     strPointer("client:retired-alias"),
		Label:       "Extra freight",
		POValue:     json.RawMessage("null"),
		SourceValue: json.RawMessage("null"),
		Status:      POReconciliationStatusResolved,
		Resolution:  strPointer(POReconciliationResolutionAckExtra),
	})
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("acknowledge extra: %v", err)
	}
	removed := round1Update(t, database, ctx, updated, user, updated.Items[:2])
	if len(removed.Items) != 2 {
		t.Fatalf("lines after removal = %d; want 2", len(removed.Items))
	}
	before, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after removal: %v", err)
	}
	retired := findSessionItem(t, before.Items, extraKey)
	if retired.Resolution == nil || *retired.Resolution != POReconciliationResolutionRemoved {
		t.Fatalf("removed line did not retire its item: %+v", retired)
	}

	replacement := append([]PurchaseOrderItem{}, removed.Items...)
	replacement = append(replacement, PurchaseOrderItem{
		ClientKey:      "client:retired-alias",
		Description:    "Unrelated replacement freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 60,
	})
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: replacement,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("retired alias accepted for a new row (old ID %d): %v; want refusal against retained mappings", oldID, err)
	}
	after, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after refusal: %v", err)
	}
	kept := findSessionItem(t, after.Items, extraKey)
	if kept.Resolution == nil || *kept.Resolution != POReconciliationResolutionRemoved {
		t.Errorf("refusal rebound or deleted history: %+v", kept)
	}
	if len(kept.History) != len(retired.History) {
		t.Errorf("history length changed %d -> %d on a refused write", len(retired.History), len(kept.History))
	}
}

// B3: Discount wire values require both non-null primitive members
// {Enabled:boolean, Percent:integer 0..100}. Partial, null-member and
// unknown-member shapes are refused; documented casing and numeric safety
// stay intact.
func TestPOReconciliationDiscountObjectsRequireExactMembers(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_discount")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 discount members")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	key := purchaseOrderLineKeyForRow(draft.Items[0].ID)

	cases := map[string]string{
		"empty object":   `{}`,
		"missing member": `{"Enabled":true}`,
		"other missing":  `{"Percent":25}`,
		"null enabled":   `{"Enabled":null,"Percent":0}`,
		"null percent":   `{"Enabled":false,"Percent":null}`,
		"unknown member": `{"Enabled":true,"Percent":5,"Extra":1}`,
		"text percent":   `{"Enabled":true,"Percent":"5"}`,
	}
	for name, raw := range cases {
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{{
			Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
			LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
			POValue: json.RawMessage(raw), SourceValue: json.RawMessage(`{"Enabled":false,"Percent":0}`),
			Status: POReconciliationStatusUnresolved,
		}}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err == nil {
			t.Errorf("discount %s saved; want ErrPurchaseOrderInvalid", name)
		} else if !isInvalidError(err) {
			t.Errorf("discount %s = %v; want ErrPurchaseOrderInvalid", name, err)
		}
	}
	// Compatible casing and the canonical disabled shape keep working, and a
	// genuine enabled difference still records.
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Items = []POReconciliationItem{{
		Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
		LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
		POValue:     json.RawMessage(`{"Enabled":true,"Percent":5}`),
		SourceValue: json.RawMessage(`{"Enabled":false,"Percent":0}`),
		Status:      POReconciliationStatusUnresolved,
	}}
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Errorf("canonical discount difference refused: %v", err)
	}
}

// B3: reference values are canonical references, not prose. Missing carries
// its reference as SourceValue; extras carry a reference or null; duplicates
// carry the contested reference on both sides.
func TestPOReconciliationReferenceValuesAreCanonical(t *testing.T) {
	newDraft := func(t *testing.T, schema string) (*Store, context.Context, AuthUser, PurchaseOrder) {
		database := openTestSchema(t, schema)
		quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 references")
		user := poTestUser(t, database)
		ctx := poContext(t, database)
		draft, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
			QuotationID: quotation.ID, RevisionNumber: quotation.RevisionNumber, CreatedBy: user.ID,
		})
		if err != nil {
			t.Fatalf("create draft: %v", err)
		}
		return database, ctx, user, draft
	}
	t.Run("missing prose source value refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_r1_ref1")
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{{
			Key: "source:3:match", Field: "source:match", Kind: POReconciliationKindMissingLine,
			SourceLineRef: strPointer("3"), Label: "Quotation line 3 missing",
			POValue: json.RawMessage("null"), SourceValue: mustMarshalValue(t, "Zero-value line"),
			Status: POReconciliationStatusUnresolved,
		}}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !isInvalidError(err) {
			t.Errorf("prose missing SourceValue saved: %v; want ErrPurchaseOrderInvalid", err)
		}
	})
	t.Run("missing mismatched reference refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_r1_ref2")
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{{
			Key: "source:3:match", Field: "source:match", Kind: POReconciliationKindMissingLine,
			SourceLineRef: strPointer("3"), Label: "Quotation line 3 missing",
			POValue: json.RawMessage("null"), SourceValue: mustMarshalValue(t, "1"),
			Status: POReconciliationStatusUnresolved,
		}}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !isInvalidError(err) {
			t.Errorf("mismatched missing reference saved: %v; want ErrPurchaseOrderInvalid", err)
		}
	})
	t.Run("extra prose value refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_r1_ref3")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{{
			Key: "line:" + key + ":match", Field: "line:match", Kind: POReconciliationKindExtraLine,
			LineKey: strPointer(key), Label: "Line 1 match",
			POValue:     mustMarshalValue(t, "Line 1 is not on the quotation"),
			SourceValue: json.RawMessage("null"),
			Status:      POReconciliationStatusUnresolved,
		}}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !isInvalidError(err) {
			t.Errorf("prose extra POValue saved: %v; want ErrPurchaseOrderInvalid", err)
		}
	})
	t.Run("duplicate incoherent references refused", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_r1_ref4")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		for name, values := range map[string]struct {
			po, ref string
		}{
			"mismatched pair": {"2", "1"},
			"malformed po":    {"1.2.3", "1.2.3"},
			"prose po":        {"Line 1 twice", "1"},
		} {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":match", Field: "line:match", Kind: POReconciliationKindDuplicateMatch,
				LineKey: strPointer(key), SourceLineRef: strPointer(values.ref), Label: "Line 1 match",
				POValue: mustMarshalValue(t, values.po), SourceValue: json.RawMessage("null"),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !isInvalidError(err) {
				t.Errorf("duplicate %s saved: %v; want ErrPurchaseOrderInvalid", name, err)
			}
		}
	})
	t.Run("canonical line-set shapes accepted", func(t *testing.T) {
		database, ctx, user, draft := newDraft(t, "vera_po_r1_ref5")
		key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Items = []POReconciliationItem{
			{
				Key: "source:3:match", Field: "source:match", Kind: POReconciliationKindMissingLine,
				SourceLineRef: strPointer("3"), Label: "Quotation line 3 missing",
				POValue: json.RawMessage("null"), SourceValue: mustMarshalValue(t, "3"),
				Status: POReconciliationStatusUnresolved,
			},
			{
				Key: "line:" + key + ":match", Field: "line:match", Kind: POReconciliationKindDuplicateMatch,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 match",
				POValue: mustMarshalValue(t, "1"), SourceValue: json.RawMessage("null"),
				Status: POReconciliationStatusUnresolved,
			},
		}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
			t.Errorf("canonical missing/duplicate shapes refused: %v", err)
		}
	})
}

func isInvalidError(err error) bool {
	return err != nil && errors.Is(err, ErrPurchaseOrderInvalid)
}

// Identical-valued alias rows keep their own IDs across edit/reorder/reopen;
// foreign row IDs, foreign session identities, cross-PO aliases and duplicate
// alias claims are all refused. (Adapted from the independent review probe;
// the alias double-claim half is also covered at the input layer.)
func TestPOReconciliationIdenticalRowsAndForeignClaims(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_identical")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 identical")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	a := PurchaseOrderItem{ClientKey: "client:identical-a", Description: "Same", QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 100}
	b := a
	b.ClientKey = "client:identical-b"
	updated := round1Update(t, database, ctx, draft, user, []PurchaseOrderItem{a, b})
	idA, idB := updated.Items[0].ID, updated.Items[1].ID
	if idA == idB {
		t.Fatalf("identical lines share row %d; want distinct rows", idA)
	}
	extra := POReconciliationItem{
		Key: "line:client:identical-a:match", Field: "line:match", Kind: POReconciliationKindExtraLine,
		LineKey: strPointer("client:identical-a"), Label: "Extra A",
		POValue: json.RawMessage("null"), SourceValue: json.RawMessage("null"),
		Status: POReconciliationStatusResolved, Resolution: strPointer(POReconciliationResolutionAckExtra),
	}
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Items = append(loaded.Items, extra)
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("acknowledge identical-row extra: %v", err)
	}
	reloaded, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	swap := []PurchaseOrderItem{reloaded.Items[1], reloaded.Items[0]}
	swap[0].Description = "Edited B"
	swapped := round1Update(t, database, ctx, reloaded, user, swap)
	if swapped.Items[0].ID != idB || swapped.Items[1].ID != idA {
		t.Fatalf("identical-valued rows inherited one another's IDs: %+v", swapped.Items)
	}
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	kept := findSessionItem(t, session.Items, extra.Key)
	if kept.LineKey == nil || *kept.LineKey != "client:identical-a" {
		t.Fatalf("extra item followed position instead of identity: %+v", kept)
	}

	otherQuotation := seedFinalizeQuotation(t, database, 9, "PT. Other", "R1 foreign")
	other := createFinalizeDraft(t, database, ctx, user, otherQuotation)
	lines := append([]PurchaseOrderItem{}, swapped.Items...)
	lines[0].ID = other.Items[0].ID
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: swapped.ID, UpdatedBy: user.ID, Items: lines,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("foreign PO row id accepted: %v; want ErrPurchaseOrderInvalid", err)
	}
	session.Items = append(session.Items, POReconciliationItem{
		Key: "line:id:" + itoa(other.Items[0].ID) + ":quantity", Field: "line:quantity",
		Kind: POReconciliationKindDifference, LineKey: strPointer("id:" + itoa(other.Items[0].ID)),
		SourceLineRef: strPointer("1"), Label: "Other PO line",
		POValue: mustMarshalValue(t, 30000), SourceValue: mustMarshalValue(t, 20000),
		Status: POReconciliationStatusUnresolved,
	})
	if _, err := database.SavePOReconciliation(ctx, session, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("foreign session line identity accepted: %v; want ErrPurchaseOrderInvalid", err)
	}
	fresh, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after refusals: %v", err)
	}
	fresh.Items = append(fresh.Items, POReconciliationItem{
		Key: "line:client:identical-a:quantity", Field: "line:quantity",
		Kind: POReconciliationKindDifference, LineKey: strPointer("client:foreign-only"),
		SourceLineRef: strPointer("1"), Label: "Foreign alias",
		POValue: mustMarshalValue(t, 30000), SourceValue: mustMarshalValue(t, 20000),
		Status: POReconciliationStatusUnresolved,
	})
	if _, err := database.SavePOReconciliation(ctx, fresh, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("cross-PO alias accepted: %v; want ErrPurchaseOrderInvalid", err)
	}
}

// A corrected unknown keeps its line key and records the change: the extra
// resolves match_corrected once the line matches, history retained, and the
// order finalizes.
func TestPOReconciliationCorrectedUnknownKeepsKeyAndFinalizes(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_unknownfix")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 unknown fix")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	lines := append([]PurchaseOrderItem{}, draft.Items[:1]...)
	lines = append(lines, PurchaseOrderItem{
		ClientKey:      "client:freight-1",
		Description:    "Freight",
		QuantityScaled: 10000,
		UoM:            "Lot",
		UnitPriceMinor: 0,
	})
	changed := round1Update(t, database, ctx, draft, user, lines)
	var key string
	for _, line := range changed.Items {
		if line.Description == "Freight" {
			// First-seen wins for a brand-new row carrying an editor alias:
			// the recorded session key is the alias itself.
			key = line.ClientKey
		}
	}
	if key != "client:freight-1" {
		t.Fatalf("freight alias = %q; want the first-seen client key recorded", key)
	}
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Items = append(loaded.Items, POReconciliationItem{
		Key: "line:" + key + ":match", Field: "line:match", Kind: POReconciliationKindExtraLine,
		LineKey: strPointer(key), Label: "Line 2 match",
		POValue: json.RawMessage("null"), SourceValue: json.RawMessage("null"),
		Status: POReconciliationStatusUnresolved,
	})
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("record plain extra: %v", err)
	}
	// Correct the reference to source line 3 and resolve the same item as
	// match_corrected: same key, change recorded, history retained.
	fixed := append([]PurchaseOrderItem{}, changed.Items...)
	three := 3
	for i := range fixed {
		if fixed[i].Description == "Freight" {
			fixed[i].QuotationItemNo = &three
			fixed[i].Description = "Zero-value line"
		}
	}
	round1Update(t, database, ctx, changed, user, fixed)
	rematch, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	done := false
	for i := range rematch.Items {
		if rematch.Items[i].Key == "line:"+key+":match" {
			rematch.Items[i].Status = POReconciliationStatusResolved
			rematch.Items[i].Resolution = strPointer(POReconciliationResolutionCorrected)
			done = true
		}
	}
	if !done {
		t.Fatalf("extra item missing after correction: %+v", rematch.Items)
	}
	resolved, err := database.SavePOReconciliation(ctx, rematch, user.ID)
	if err != nil {
		t.Fatalf("resolve corrected match: %v", err)
	}
	kept := findSessionItem(t, resolved.Items, "line:"+key+":match")
	if len(kept.History) != 2 {
		t.Errorf("history has %d events; want the extra plus the rematch", len(kept.History))
	}
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	input := finalizeInput(fresh, user, "PO-R1-REMATCH")
	input.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	if _, err := database.FinalizePurchaseOrder(ctx, input); err != nil {
		t.Fatalf("finalize after corrected match: %v", err)
	}
}

// An alias adopted onto an already-bound row does not become a second
// session key: items addressed by the adopted alias are refused, and the
// pane must use the recorded first-seen key. The row keeps its alias for
// persistence addressing.
func TestPOReconciliationAdoptedAliasIsNotASecondKey(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_aliasfinal")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 alias finalize")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	lines := append([]PurchaseOrderItem{}, draft.Items...)
	for i := range lines {
		lines[i].ClientKey = "client:final-" + string(rune('a'+i))
	}
	lines[0].QuantityScaled = 30000
	updated := round1Update(t, database, ctx, draft, user, lines)
	_ = updated
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Items = append(loaded.Items, POReconciliationItem{
		Key: "line:client:final-a:quantity", Field: "line:quantity", Kind: POReconciliationKindDifference,
		LineKey: strPointer("client:final-a"), SourceLineRef: strPointer("1"), Label: "Line 1 quantity",
		POValue: mustMarshalValue(t, 30000), SourceValue: mustMarshalValue(t, 20000),
		Status: POReconciliationStatusUnresolved,
	})
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("adopted-alias addressed item saved: %v; want refusal naming the recorded key", err)
	}
}

// A canonical acknowledged missing line finalizes: the recorded reference
// matches the recomputed comparison.
func TestPOReconciliationCanonicalMissingAcknowledgedFinalizes(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_missfinal")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 missing finalize")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	changed := round1Update(t, database, ctx, draft, user, draft.Items[:1])
	_ = changed
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Items = append(loaded.Items, POReconciliationItem{
		Key: "source:3:match", Field: "source:match", Kind: POReconciliationKindMissingLine,
		SourceLineRef: strPointer("3"), Label: "Quotation line 3 missing",
		POValue: json.RawMessage("null"), SourceValue: mustMarshalValue(t, "3"),
		Status: POReconciliationStatusResolved, Resolution: strPointer(POReconciliationResolutionAckMiss),
	})
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("acknowledge canonical missing: %v", err)
	}
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	input := finalizeInput(fresh, user, "PO-R1-MISSFINAL")
	input.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	input.AcknowledgedMissing = []string{"3"}
	if _, err := database.FinalizePurchaseOrder(ctx, input); err != nil {
		t.Fatalf("finalize with acknowledged missing: %v", err)
	}
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}

// A canonical acknowledged plain extra (null reference values) finalizes:
// acknowledgement clears a true extra, and only unknown references demand
// correction.
func TestPOReconciliationCanonicalExtraAcknowledgedFinalizes(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_extrafinal")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 extra finalize")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	lines := append([]PurchaseOrderItem{}, draft.Items[:1]...)
	lines = append(lines, PurchaseOrderItem{
		Description: "Freight", QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 0,
	})
	changed := round1Update(t, database, ctx, draft, user, lines)
	var key string
	for _, line := range changed.Items {
		if line.Description == "Freight" {
			key = purchaseOrderLineKeyForRow(line.ID)
		}
	}
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Items = append(loaded.Items, POReconciliationItem{
		Key: "line:" + key + ":match", Field: "line:match", Kind: POReconciliationKindExtraLine,
		LineKey: strPointer(key), Label: "Line 2 match",
		POValue: json.RawMessage("null"), SourceValue: json.RawMessage("null"),
		Status: POReconciliationStatusResolved, Resolution: strPointer(POReconciliationResolutionAckExtra),
	})
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("acknowledge canonical extra: %v", err)
	}
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	input := finalizeInput(fresh, user, "PO-R1-EXTRAFINAL")
	input.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	// The extra line is still extra, so both acknowledgements are needed.
	input.AcknowledgedExtra = []int{2}
	input.AcknowledgedMissing = []string{"3"}
	if _, err := database.FinalizePurchaseOrder(ctx, input); err != nil {
		t.Fatalf("finalize with acknowledged plain extra: %v", err)
	}
}

// R-08 end to end: a duplicate cleared by removal finalizes once the
// remaining missing line is acknowledged. The removed claimant stays
// line_removed with history; the survivor stays matched.
func TestPOReconciliationDuplicateRemovalFinalizes(t *testing.T) {
	database := openTestSchema(t, "vera_po_r1_dupefinal")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R1 duplicate finalize")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	lines := append([]PurchaseOrderItem{}, draft.Items...)
	one := 1
	lines[0].QuotationItemNo = &one
	lines[0].QuotationSubNo = nil
	lines[1].QuotationItemNo = &one
	lines[1].QuotationSubNo = nil
	matched := round1Update(t, database, ctx, draft, user, lines)
	keyA := purchaseOrderLineKeyForRow(matched.Items[0].ID)
	keyB := purchaseOrderLineKeyForRow(matched.Items[1].ID)
	loaded, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, entry := range []struct{ key, line string }{{keyA, keyA}, {keyB, keyB}} {
		loaded.Items = append(loaded.Items, POReconciliationItem{
			Key: "line:" + entry.key + ":match", Field: "line:match", Kind: POReconciliationKindDuplicateMatch,
			LineKey: strPointer(entry.line), SourceLineRef: strPointer("1"), Label: "Duplicate match",
			POValue: mustMarshalValue(t, "1"), SourceValue: json.RawMessage("null"),
			Status: POReconciliationStatusUnresolved,
		})
	}
	if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
		t.Fatalf("record duplicates: %v", err)
	}
	survivor := round1Update(t, database, ctx, matched, user, matched.Items[:1])
	if len(survivor.Items) != 1 || survivor.Items[0].ID != matched.Items[0].ID {
		t.Fatalf("survivor = %+v; want only the first row", survivor.Items)
	}
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	input := finalizeInput(fresh, user, "PO-R1-DUPEFINAL")
	input.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	input.AcknowledgedMissing = []string{"3"}
	if _, err := database.FinalizePurchaseOrder(ctx, input); err != nil {
		t.Fatalf("finalize after duplicate removal: %v", err)
	}
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("read saved session: %v", err)
	}
	if !session.ReadOnly {
		t.Errorf("saved session is writable")
	}
	byKey := map[string]POReconciliationItem{}
	for _, item := range session.Items {
		byKey[item.Key] = item
	}
	if removed, ok := byKey["line:"+keyB+":match"]; !ok || removed.Resolution == nil ||
		*removed.Resolution != POReconciliationResolutionRemoved {
		t.Errorf("removed duplicate = %+v; want retained line_removed", removed)
	}
	if kept, ok := byKey["line:"+keyA+":match"]; !ok || kept.Resolution == nil ||
		*kept.Resolution != POReconciliationResolutionCorrected {
		t.Errorf("surviving duplicate = %+v; want retained match_corrected", kept)
	}
}
