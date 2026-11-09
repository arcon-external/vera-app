package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// Round-2 regressions for the two remaining #517 counterexamples (K-S5
// adopted-alias retirement, K-S6 same-spelling duplicate members), plus the
// durable lifecycle/upgrade/rollback controls the frozen design requires.
// Seams: public Store methods and Postgres behavior against disposable
// schemas; expectations come from the contract and decision.md.

// round2Update saves lines as one ordinary draft save.
func round2Update(t *testing.T, database *Store, ctx context.Context, draft PurchaseOrder, user AuthUser, lines []PurchaseOrderItem) PurchaseOrder {
	t.Helper()
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: lines,
	})
	if err != nil {
		t.Fatalf("draft save: %v", err)
	}
	return updated
}

// B2-A RED: an original id:<row> row adopts a client alias, is removed, then
// an unrelated new row reuses that alias. The reuse must be refused
// atomically; row IDs, first-seen keys, kept/line_removed history and the
// link tax item must remain intact.
func TestRound2AdoptedAliasRetirementRefusesReuse(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_adoptretire")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 adopted retire")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	oldID := draft.Items[0].ID
	oldKey := purchaseOrderLineKeyForRow(oldID)
	alias := "client:adopted-retired"

	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines[0].ClientKey = alias
	adopted := round2Update(t, database, ctx, draft, user, lines)
	if adopted.Items[0].ID != oldID {
		t.Fatalf("adoption churned row ids: %+v", adopted.Items)
	}
	item := lineDifferenceItem(t, oldKey, "line:quantity", "quantity", "Line 1 quantity", 20000, 20000, "1")
	item.Status = POReconciliationStatusResolved
	item.Resolution = strPointer(POReconciliationResolutionKeptPO)
	saveLineItem(t, database, ctx, draft.ID, user.ID, item)

	trimmed := round2Update(t, database, ctx, adopted, user, adopted.Items[1:])
	if len(trimmed.Items) != 1 {
		t.Fatalf("lines after removal = %d; want 1", len(trimmed.Items))
	}
	before, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after removal: %v", err)
	}
	retired := findSessionItem(t, before.Items, item.Key)
	if retired.Resolution == nil || *retired.Resolution != POReconciliationResolutionRemoved {
		t.Fatalf("adopted review not retired: %+v", retired)
	}
	newLine := PurchaseOrderItem{ClientKey: alias, Description: "Unrelated freight", QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50}
	replacement := append(append([]PurchaseOrderItem{}, trimmed.Items...), newLine)
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: replacement,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("retired adopted alias claimed by a new row: %v; want ErrPurchaseOrderInvalid", err)
	}
	after, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload after refusal: %v", err)
	}
	kept := findSessionItem(t, after.Items, item.Key)
	if kept.Resolution == nil || *kept.Resolution != POReconciliationResolutionRemoved {
		t.Errorf("refusal rebound retired history: %+v", kept)
	}
	if len(kept.History) != len(retired.History) {
		t.Errorf("history length changed %d -> %d on a refused write", len(retired.History), len(kept.History))
	}
	tax := findSessionItem(t, after.Items, "link:taxBasis")
	if tax.Status != POReconciliationStatusResolved {
		t.Errorf("tax item disturbed by the refusal: %+v", tax)
	}
}

// B2-B RED: identically spelled duplicate JSON members must be refused by
// the real writer, including same-value duplicates. Valid single members
// keep working.
func TestRound2SameSpellingDuplicateMembersRefused(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_duplicates")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 duplicates")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	key := purchaseOrderLineKeyForRow(draft.Items[0].ID)

	for _, tc := range []struct{ name, field, raw string }{
		{"Enabled duplicate", "line:discount", `{"Enabled":false,"Enabled":true,"Percent":0}`},
		{"Percent duplicate", "line:discount", `{"Enabled":true,"Percent":25,"Percent":0}`},
		{"same-value duplicate", "line:discount", `{"Enabled":true,"Percent":5,"Percent":5}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
				POValue: json.RawMessage(tc.raw), SourceValue: json.RawMessage(`{"Enabled":false,"Percent":0}`),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Errorf("same-spelling duplicate saved: %v; want ErrPurchaseOrderInvalid", err)
			}
		})
	}
}

// B2-B matrix: typed-object member handling through the real writer, for
// each member in both values. Duplicates (same/mixed/escaped spelling,
// conflicting or identical values) fail; good singles (casing, escaped
// names, order, whitespace) pass; malformed shapes fail. Disabled-zero
// commercial comparison and numeric safety stay intact.
func TestRound2TypedObjectMemberMatrix(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_typematrix")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 type matrix")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	key := purchaseOrderLineKeyForRow(draft.Items[0].ID)

	goodDiscount := `{"Enabled":false,"Percent":0}`
	for _, tc := range []struct{ name, raw string }{
		{"same Enabled", `{"Enabled":true,"Enabled":false,"Percent":0}`},
		{"same Percent", `{"Enabled":true,"Percent":5,"Percent":5}`},
		{"mixed Enabled", `{"Enabled":true,"enabled":false,"Percent":0}`},
		{"mixed Percent", `{"Enabled":true,"Percent":5,"percent":0}`},
		{"escaped Enabled", "{\"\\u0045nabled\":true,\"Enabled\":false,\"Percent\":0}"},
		{"escaped enabled lower", `{"Enabled":true,"Percent":0,"\u0065nabled":false}`},
		{"trailing second value", `{"Enabled":true,"Percent":0} {}`},
		{"truncated object", `{"Enabled":true,"Percent":0`},
	} {
		t.Run("discount/"+tc.name, func(t *testing.T) {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
				POValue: json.RawMessage(tc.raw), SourceValue: json.RawMessage(goodDiscount),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Errorf("discount %s saved: %v; want ErrPurchaseOrderInvalid", tc.name, err)
			}
		})
	}
	for _, tc := range []struct{ name, raw string }{
		{"same TaxZone", `{"TaxZone":"FTZ","TaxZone":"NonFTZ","TaxType":"Goods","ServiceCategory":""}`},
		{"mixed TaxType", `{"TaxZone":"FTZ","TaxType":"Goods","taxType":"Services","ServiceCategory":""}`},
		{"escaped ServiceCategory", `{"TaxZone":"FTZ","TaxType":"Goods","ServiceCategory":"","\u0053erviceCategory":"X"}`},
	} {
		t.Run("tax/"+tc.name, func(t *testing.T) {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			for i := range loaded.Items {
				if loaded.Items[i].Field == "link:taxBasis" {
					loaded.Items[i].POValue = json.RawMessage(tc.raw)
				}
			}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Errorf("tax %s saved; want ErrPurchaseOrderInvalid", tc.name)
			}
			// The protected triple must be unchanged: the refusal came from
			// the member parser, not from a mutated-then-frozen value.
			reloaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			tax := findSessionItem(t, reloaded.Items, "link:taxBasis")
			var triple POReconciliationTaxBasis
			if err := json.Unmarshal(tax.POValue, &triple); err != nil {
				t.Fatalf("decode stored triple: %v", err)
			}
			if triple != (POReconciliationTaxBasis{TaxZone: "FTZ", TaxType: "Goods"}) {
				t.Errorf("stored triple mutated to %+v", triple)
			}
		})
	}
	for _, tc := range []struct{ name, raw string }{
		{"pascal", `{"Enabled":true,"Percent":5}`},
		{"lower", `{"enabled":false,"percent":0}`},
		{"mixed", `{"ENABLED":true,"pErCeNt":5}`},
		{"escaped name", "{\"\\u0045nabled\":true,\"Percent\":5}"},
		{"order", `{"Percent":5,"Enabled":true}`},
		{"whitespace", "{ \"Enabled\" : true , \"Percent\" : 5 }"},
		{"bounds", `{"Enabled":true,"Percent":100}`},
	} {
		t.Run("discount-good/"+tc.name, func(t *testing.T) {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
				POValue: json.RawMessage(tc.raw), SourceValue: json.RawMessage(goodDiscount),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
				t.Errorf("good discount %s refused: %v", tc.name, err)
			}
		})
	}
}

// Adopt/replace A→B on the same positive ID: both aliases stay owned by that
// row with exactly one session identity, and alias-only same-owner repeated
// saves keep working. Removing the owner retires both aliases.
func TestRound2AliasReplaceAndRepeat(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_replace")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 replace")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	rowID := draft.Items[0].ID

	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines[0].ClientKey = "client:replace-a"
	adopted := round2Update(t, database, ctx, draft, user, lines)
	lines = append([]PurchaseOrderItem{}, adopted.Items...)
	for i := range lines {
		if lines[i].ID == rowID {
			lines[i].ClientKey = "client:replace-b"
		}
	}
	replaced := round2Update(t, database, ctx, adopted, user, lines)
	var alias string
	for _, line := range replaced.Items {
		if line.ID == rowID {
			alias = line.ClientKey
		}
	}
	if alias != "client:replace-b" {
		t.Fatalf("row alias = %q; want the replacement carried", alias)
	}
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	owned := 0
	for _, identity := range session.LineIdentities {
		if identity.ID != nil && *identity.ID == rowID {
			owned++
		}
	}
	if owned != 1 {
		t.Fatalf("row %d has %d identities %+v; want exactly one first-seen key", rowID, owned, session.LineIdentities)
	}
	// Alias-only addressing by the older alias still resolves its original
	// live owner and makes it current again.
	again, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	only := []PurchaseOrderItem{again.Items[0], again.Items[1]}
	only[0].ClientKey = "client:replace-a"
	byAlias := round2Update(t, database, ctx, again, user, only)
	if byAlias.Items[0].ID != rowID {
		t.Fatalf("alias-only save moved row %d to %d", rowID, byAlias.Items[0].ID)
	}
	// Removing the owner retires both aliases.
	gone := round2Update(t, database, ctx, byAlias, user, byAlias.Items[1:])
	if len(gone.Items) != 1 {
		t.Fatalf("lines after removal = %d; want 1", len(gone.Items))
	}
	for _, retired := range []string{"client:replace-a", "client:replace-b"} {
		probe := append([]PurchaseOrderItem{}, gone.Items...)
		probe = append(probe, PurchaseOrderItem{
			ClientKey: retired, Description: "Reuse", QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 1,
		})
		if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
			ID: draft.ID, UpdatedBy: user.ID, Items: probe,
		}); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Errorf("retired alias %q reused: %v; want refusal", retired, err)
		}
	}
}

// Alias-less legacy payloads keep current addressing and IDs: exact-value
// matching preserves rows and their stored aliases without new identities.
func TestRound2LegacyPayloadKeepsAddressing(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_legacy")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 legacy")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	idA, idB := draft.Items[0].ID, draft.Items[1].ID

	bare := []PurchaseOrderItem{draft.Items[1], draft.Items[0]}
	for i := range bare {
		bare[i].ID = 0
		bare[i].ClientKey = ""
	}
	swapped := round2Update(t, database, ctx, draft, user, bare)
	if swapped.Items[0].ID != idB || swapped.Items[1].ID != idA {
		t.Fatalf("legacy reorder ids = %d/%d; want %d/%d", swapped.Items[0].ID, swapped.Items[1].ID, idB, idA)
	}
	session, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if len(session.LineIdentities) != 2 {
		t.Errorf("identities = %+v; want the two seeded keys, no extras", session.LineIdentities)
	}
}

// The same alias on another PO succeeds with an independent owner: alias
// scope is per purchase order, never global.
func TestRound2CrossPOCanReuseAlias(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_crosspo")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	firstQuotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 cross first")
	secondQuotation := seedFinalizeQuotation(t, database, 3, "PT. Other", "R2 cross second")
	first := createFinalizeDraft(t, database, ctx, user, firstQuotation)
	second := createFinalizeDraft(t, database, ctx, user, secondQuotation)

	alias := "client:shared-alias"
	firstLines := append([]PurchaseOrderItem{}, first.Items...)
	firstLines = append(firstLines, PurchaseOrderItem{
		ClientKey: alias, Description: "First freight", QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50,
	})
	firstUpdated := round2Update(t, database, ctx, first, user, firstLines)
	secondLines := append([]PurchaseOrderItem{}, second.Items...)
	secondLines = append(secondLines, PurchaseOrderItem{
		ClientKey: alias, Description: "Second freight", QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 60,
	})
	secondUpdated := round2Update(t, database, ctx, second, user, secondLines)
	var firstID, secondID int64
	for _, line := range firstUpdated.Items {
		if line.ClientKey == alias {
			firstID = line.ID
		}
	}
	for _, line := range secondUpdated.Items {
		if line.ClientKey == alias {
			secondID = line.ID
		}
	}
	if firstID < 1 || secondID < 1 || firstID == secondID {
		t.Fatalf("cross-PO owners = %d/%d; want distinct rows", firstID, secondID)
	}
}

// A NULL-reserved alias (retained history with unrecoverable owner) can
// never identify a new row.
func TestRound2NullReservationRefused(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_reserved")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 reserved")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)

	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	var orgID string
	if err := pool.QueryRow(ctx, `SELECT org_id::text FROM `+schema+`.purchase_orders WHERE id = $1`,
		draft.ID).Scan(&orgID); err != nil {
		t.Fatalf("read org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO `+schema+`.purchase_order_line_aliases
		(org_id, purchase_order_id, client_key, owner_line_id)
		VALUES ($1, $2, 'client:reserved-x', NULL)`, orgID, draft.ID); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines = append(lines, PurchaseOrderItem{
		ClientKey: "client:reserved-x", Description: "Reuse", QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 1,
	})
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: lines,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("reserved alias claimed: %v; want ErrPurchaseOrderInvalid", err)
	}
}

// Every line-writing route refuses a retired alias atomically: draft
// update, finalize, correction and source repair. Refusals leave version,
// state, lines, history and claims exactly as they were.
func TestRound2AllWritersRefuseRetiredAlias(t *testing.T) {
	setup := func(t *testing.T, schema string) (*Store, context.Context, AuthUser, PurchaseOrder) {
		database := openTestSchema(t, schema)
		user := poTestUser(t, database)
		ctx := poContext(t, database)
		quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 writers")
		draft := createFinalizeDraft(t, database, ctx, user, quotation)
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		lines = append(lines, PurchaseOrderItem{
			ClientKey: "client:retired-route", Description: "Route freight",
			QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50,
		})
		withExtra := round2Update(t, database, ctx, draft, user, lines)
		withoutExtra := round2Update(t, database, ctx, withExtra, user, withExtra.Items[:2])
		if len(withoutExtra.Items) != 2 {
			t.Fatalf("setup lines = %d; want the retired alias scenario", len(withoutExtra.Items))
		}
		return database, ctx, user, withoutExtra
	}
	claimCount := func(t *testing.T, database *Store, ctx context.Context, poID int64) int {
		t.Helper()
		pool, err := database.poolOrError()
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+database.configSchema()+`.purchase_order_line_aliases WHERE purchase_order_id = $1`,
			poID).Scan(&count); err != nil {
			t.Fatalf("count claims: %v", err)
		}
		return count
	}
	reuseLine := PurchaseOrderItem{
		ClientKey: "client:retired-route", Description: "Unrelated reuse",
		QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 70,
	}
	headerDecisions := []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}

	t.Run("draft update rolls back", func(t *testing.T) {
		database, ctx, user, draft := setup(t, "vera_po_r2_route_update")
		beforeClaims := claimCount(t, database, ctx, draft.ID)
		if beforeClaims < 1 {
			t.Fatalf("claims = %d; want the retained ownership", beforeClaims)
		}
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		lines = append(lines, reuseLine)
		if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
			ID: draft.ID, UpdatedBy: user.ID, Items: lines,
		}); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Fatalf("reuse via draft update = %v; want ErrPurchaseOrderInvalid", err)
		}
		after, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		if after.Version != draft.Version || len(after.Items) != len(draft.Items) {
			t.Errorf("refused draft save moved version %d->%d or lines %d->%d",
				draft.Version, after.Version, len(draft.Items), len(after.Items))
		}
		if got := claimCount(t, database, ctx, draft.ID); got != beforeClaims {
			t.Errorf("claims %d -> %d on a refused write; want no poisoned claim", beforeClaims, got)
		}
	})

	t.Run("finalize rolls back", func(t *testing.T) {
		database, ctx, user, draft := setup(t, "vera_po_r2_route_finalize")
		beforeClaims := claimCount(t, database, ctx, draft.ID)
		fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		items := append([]PurchaseOrderItem{}, fresh.Items...)
		items = append(items, reuseLine)
		for i := range items {
			items[i].LineNumber = i + 1
		}
		input := finalizeInput(fresh, user, "PO-R2-ROUTE")
		input.Items = items
		input.Decisions = headerDecisions
		input.AcknowledgedExtra = []int{3}
		if _, err := database.FinalizePurchaseOrder(ctx, input); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Fatalf("reuse via finalize = %v; want ErrPurchaseOrderInvalid", err)
		}
		after, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		if after.State != PurchaseOrderStateDraft || after.Version != draft.Version {
			t.Errorf("refused finalize moved state %q/%d; want draft/%d",
				after.State, after.Version, draft.Version)
		}
		if history, err := database.GetPurchaseOrderHistory(ctx, draft.ID); err != nil || len(history) != 0 {
			t.Errorf("history after refusal = %v, %v; want none", history, err)
		}
		if got := claimCount(t, database, ctx, draft.ID); got != beforeClaims {
			t.Errorf("claims %d -> %d on a refused write", beforeClaims, got)
		}
	})

	t.Run("correction rolls back", func(t *testing.T) {
		database, ctx, user, draft := setup(t, "vera_po_r2_route_correct")
		clean, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		input := finalizeInput(clean, user, "PO-R2-CORRECT")
		input.Decisions = headerDecisions
		saved, err := database.FinalizePurchaseOrder(ctx, input)
		if err != nil {
			t.Fatalf("finalize clean: %v", err)
		}
		beforeClaims := claimCount(t, database, ctx, saved.ID)
		correction := correctionInput(saved, user, "R2 correction probe", saved.Version)
		correction.Items = append(append([]PurchaseOrderItem{}, saved.Items...), reuseLine)
		if _, err := database.CorrectPurchaseOrder(ctx, correction); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Fatalf("reuse via correction = %v; want ErrPurchaseOrderInvalid", err)
		}
		after, err := database.GetPurchaseOrder(ctx, saved.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		if after.Version != saved.Version || len(after.Items) != len(saved.Items) {
			t.Errorf("refused correction moved version %d->%d or lines %d->%d",
				saved.Version, after.Version, len(saved.Items), len(after.Items))
		}
		if history, err := database.GetPurchaseOrderHistory(ctx, saved.ID); err != nil || len(history) != 0 {
			t.Errorf("history after refusal = %v, %v; want none", history, err)
		}
		if got := claimCount(t, database, ctx, saved.ID); got != beforeClaims {
			t.Errorf("claims %d -> %d on a refused write", beforeClaims, got)
		}
	})

	t.Run("source repair rolls back", func(t *testing.T) {
		database := openTestSchema(t, "vera_po_r2_route_repair")
		user := poTestUser(t, database)
		ctx := poContext(t, database)
		admin := poRepairAdmin(t, database)
		original := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 repair original")
		target := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 repair target")
		draft := createFinalizeDraft(t, database, ctx, user, original)
		lines := append([]PurchaseOrderItem{}, draft.Items...)
		lines = append(lines, PurchaseOrderItem{
			ClientKey: "client:retired-repair", Description: "Route freight",
			QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50,
		})
		withExtra := round2Update(t, database, ctx, draft, user, lines)
		trimmed := round2Update(t, database, ctx, withExtra, user, withExtra.Items[:2])
		if len(trimmed.Items) != 2 {
			t.Fatalf("setup lines = %d; want the retired-alias scenario", len(trimmed.Items))
		}
		fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		finInput := finalizeInput(fresh, user, "PO-R2-REPAIR")
		finInput.Decisions = headerDecisions
		saved, err := database.FinalizePurchaseOrder(ctx, finInput)
		if err != nil {
			t.Fatalf("finalize clean: %v", err)
		}
		beforeClaims := claimCount(t, database, ctx, saved.ID)
		input := repairInput(t, saved, admin, target, "R2 repair probe")
		repairReuse := PurchaseOrderItem{
			ClientKey: "client:retired-repair", Description: "Unrelated reuse",
			QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 70,
		}
		input.Items = append(append([]PurchaseOrderItem{}, saved.Items...), repairReuse)
		input.Decisions = headerDecisions
		input.AcknowledgedExtra = []int{3}
		if _, err := database.RepairPurchaseOrderSource(ctx, input); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Fatalf("reuse via repair = %v; want ErrPurchaseOrderInvalid", err)
		}
		after, err := database.GetPurchaseOrder(ctx, saved.ID)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		if after.Version != saved.Version || after.QuotationID != saved.QuotationID {
			t.Errorf("refused repair moved version/link %d/%d; want %d/%d",
				after.Version, after.QuotationID, saved.Version, saved.QuotationID)
		}
		if history, err := database.GetPurchaseOrderHistory(ctx, saved.ID); err != nil || len(history) != 0 {
			t.Errorf("history after refusal = %v, %v; want none", history, err)
		}
		if got := claimCount(t, database, ctx, saved.ID); got != beforeClaims {
			t.Errorf("claims %d -> %d on a refused write", beforeClaims, got)
		}
	})
}

// --- 042->043 upgrade proof, guards, and extended lifecycle ---

// round2Claims reads durable alias ownership for one PO: alias to owner row
// ID, where nil is a permanent NULL reservation.
func round2Claims(t *testing.T, database *Store, ctx context.Context, poID int64) map[string]*int64 {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	rows, err := pool.Query(ctx, `SELECT client_key, owner_line_id FROM `+database.configSchema()+`.purchase_order_line_aliases WHERE purchase_order_id = $1 ORDER BY client_key`, poID)
	if err != nil {
		t.Fatalf("read claims: %v", err)
	}
	defer rows.Close()
	out := map[string]*int64{}
	for rows.Next() {
		var alias string
		var owner pgtype.Int8
		if err := rows.Scan(&alias, &owner); err != nil {
			t.Fatalf("scan claim: %v", err)
		}
		if owner.Valid {
			id := owner.Int64
			out[alias] = &id
		} else {
			out[alias] = nil
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate claims: %v", err)
	}
	return out
}

// round2ResetTo042 simulates the pre-043 state for upgrade tests: the live
// rows and retained sessions stay exactly as the writers left them, while the
// alias table and its ledger row are removed. The next Migrate must rebuild
// ownership purely from those inputs. Seeding uses the real Store writers on
// a full schema, so the backfill inputs are actual persisted rows/sessions;
// wiping only removes the derived claims, never the source evidence.
func round2ResetTo042(t *testing.T, database *Store, ctx context.Context) {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS `+database.configSchema()+`.purchase_order_line_aliases CASCADE`); err != nil {
		t.Fatalf("drop alias table: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM `+database.configSchema()+`.schema_migrations WHERE version = 43`); err != nil {
		t.Fatalf("drop ledger 43: %v", err)
	}
}

func round2LedgerHas43(t *testing.T, database *Store, ctx context.Context) bool {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+database.configSchema()+`.schema_migrations WHERE version = 43`).Scan(&count); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return count == 1
}

// Meaningful 042->043 proof: live adopted + client-first aliases, a retired
// alias, a readonly finalized generation, a never-linked live alias, and a
// null-ID reservation candidate. Snapshots/versions are unchanged by the
// upgrade; a second Migrate is idempotent; removing the live id-first owner
// after upgrading refuses reuse through the public writer.
func TestRound2Upgrade043BackfillsLiveAndRetained(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_upgrade")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	// Main PO: id-first row adopts A (live), client-first B (live), C added
	// then removed (retired).
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 upgrade main")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	row1 := draft.Items[0].ID
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines[0].ClientKey = "client:upgrade-a"
	lines = append(lines, PurchaseOrderItem{
		ClientKey: "client:upgrade-b", Description: "Live freight",
		QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50,
	}, PurchaseOrderItem{
		ClientKey: "client:upgrade-c", Description: "Doomed freight",
		QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 60,
	})
	withExtra := round2Update(t, database, ctx, draft, user, lines)
	var rowB, rowC int64
	for _, line := range withExtra.Items {
		switch line.ClientKey {
		case "client:upgrade-b":
			rowB = line.ID
		case "client:upgrade-c":
			rowC = line.ID
		}
	}
	if rowB < 1 || rowC < 1 {
		t.Fatalf("new rows not acquired: %+v", withExtra.Items)
	}
	trimmed := round2Update(t, database, ctx, withExtra, user, withExtra.Items[:3])
	if len(trimmed.Items) != 3 {
		t.Fatalf("lines = %d; want 3 (2 source + B, C removed)", len(trimmed.Items))
	}
	// Null-ID reservation candidate: retained history proves alias D was
	// used, but no row owner can be recovered. Crafted via SQL on the live
	// session, exactly the shape a pre-043 null-ID identity persists.
	sessionRow := func() int64 {
		var id int64
		if err := pool.QueryRow(ctx, `SELECT id FROM `+schema+`.purchase_order_reconciliation_sessions WHERE purchase_order_id = $1 AND active`, draft.ID).Scan(&id); err != nil {
			t.Fatalf("find session: %v", err)
		}
		return id
	}
	sessID := sessionRow()
	if _, err := pool.Exec(ctx, `UPDATE `+schema+`.purchase_order_reconciliation_sessions SET line_identities = line_identities || $2::jsonb WHERE id = $1`,
		sessID, `[{"Key":"client:upgrade-null","ID":null,"Removed":false}]`); err != nil {
		t.Fatalf("seed null evidence: %v", err)
	}

	// Never-linked draft with a live alias: no session may exist, the row
	// alias alone must backfill.
	blank := createBlankDraft(t, database, ctx, user)
	blankLines := []PurchaseOrderItem{{
		ClientKey: "client:upgrade-blank", Description: "Blank freight",
		QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 10,
	}}
	blankUpdated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: blank.ID, UpdatedBy: user.ID, Items: blankLines,
	})
	if err != nil {
		t.Fatalf("blank draft save: %v", err)
	}
	var blankRow int64
	for _, line := range blankUpdated.Items {
		if line.ClientKey == "client:upgrade-blank" {
			blankRow = line.ID
		}
	}
	if blankRow < 1 {
		t.Fatalf("blank alias row not acquired: %+v", blankUpdated.Items)
	}

	// Finalized PO: its session is readonly; backfill must include readonly
	// generations without mutating them.
	finQuotation := seedFinalizeQuotation(t, database, 4, "PT. Final", "R2 upgrade final")
	finDraft := createFinalizeDraft(t, database, ctx, user, finQuotation)
	finLines := append([]PurchaseOrderItem{}, finDraft.Items...)
	finLines[0].ClientKey = "client:upgrade-fin"
	finAdopted := round2Update(t, database, ctx, finDraft, user, finLines)
	finFresh, err := database.GetPurchaseOrder(ctx, finDraft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_ = finAdopted
	finInput := finalizeInput(finFresh, user, "PO-R2-UPGRADE")
	finInput.Decisions = []PurchaseOrderDecision{
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
		{Key: "header:incoterms", Choice: PurchaseOrderChoicePO},
	}
	savedFin, err := database.FinalizePurchaseOrder(ctx, finInput)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	var finRow int64
	for _, line := range savedFin.Items {
		if line.ClientKey == "client:upgrade-fin" {
			finRow = line.ID
		}
	}

	// Snapshots before the upgrade simulation.
	verMain, _ := database.GetPurchaseOrder(ctx, draft.ID)
	verBlank, _ := database.GetPurchaseOrder(ctx, blank.ID)
	sessBefore, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	sessBeforeRaw, _ := json.Marshal(sessBefore)
	histBefore, err := database.GetPurchaseOrderHistory(ctx, draft.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	// Simulate 042 (no claims) and re-apply 043 through the real runner.
	round2ResetTo042(t, database, ctx)
	if round2LedgerHas43(t, database, ctx) {
		t.Fatal("ledger still has 43 after reset")
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("re-apply 043: %v", err)
	}
	if !round2LedgerHas43(t, database, ctx) {
		t.Fatal("ledger missing 43 after re-apply")
	}

	// Exact claims.
	claims := round2Claims(t, database, ctx, draft.ID)
	for alias, want := range map[string]int64{
		"client:upgrade-a": row1,
		"client:upgrade-b": rowB,
		"client:upgrade-c": rowC,
	} {
		got, ok := claims[alias]
		if !ok || got == nil || *got != want {
			t.Errorf("claim %q = %v; want owner %d (claims %+v)", alias, got, want, claims)
		}
	}
	if got, ok := claims["client:upgrade-null"]; !ok || got != nil {
		t.Errorf("null reservation = %v, present %v; want present NULL", got, ok)
	}
	blankClaims := round2Claims(t, database, ctx, blank.ID)
	if got := blankClaims["client:upgrade-blank"]; got == nil || *got != blankRow {
		t.Errorf("blank claim = %v; want owner %d", got, blankRow)
	}
	finClaims := round2Claims(t, database, ctx, savedFin.ID)
	if got := finClaims["client:upgrade-fin"]; got == nil || *got != finRow {
		t.Errorf("finalized claim = %v; want owner %d", got, finRow)
	}

	// Snapshots/versions unchanged.
	verMainAfter, _ := database.GetPurchaseOrder(ctx, draft.ID)
	verBlankAfter, _ := database.GetPurchaseOrder(ctx, blank.ID)
	if verMainAfter.Version != verMain.Version || verBlankAfter.Version != verBlank.Version {
		t.Errorf("versions moved %d/%d -> %d/%d on upgrade",
			verMain.Version, verBlank.Version, verMainAfter.Version, verBlankAfter.Version)
	}
	sessAfter, err := database.GetPOReconciliation(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	sessAfterRaw, _ := json.Marshal(sessAfter)
	if string(sessBeforeRaw) != string(sessAfterRaw) {
		t.Error("session bytes changed on upgrade; backfill must not touch sessions")
	}
	histAfter, err := database.GetPurchaseOrderHistory(ctx, draft.ID)
	if err != nil {
		t.Fatalf("history after: %v", err)
	}
	if len(histAfter) != len(histBefore) {
		t.Errorf("history %d -> %d on upgrade", len(histBefore), len(histAfter))
	}

	// Idempotent second Migrate.
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	claims2 := round2Claims(t, database, ctx, draft.ID)
	if len(claims2) != len(claims) {
		t.Errorf("claims %d -> %d on idempotent Migrate", len(claims), len(claims2))
	}

	// Remove the live id-first owner after upgrading: reuse refused.
	after, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	kept := []PurchaseOrderItem{}
	for _, line := range after.Items {
		if line.ClientKey == "client:upgrade-a" {
			continue
		}
		kept = append(kept, line)
	}
	removed := round2Update(t, database, ctx, after, user, kept)
	_ = removed
	probe := append([]PurchaseOrderItem{}, kept...)
	probe = append(probe, PurchaseOrderItem{
		ClientKey: "client:upgrade-a", Description: "Reuse after upgrade",
		QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 1,
	})
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: probe,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("upgraded retired alias reused: %v; want ErrPurchaseOrderInvalid", err)
	}
}

// Competing positive owners abort the whole migration with no ledger row and
// no partial claims.
func TestRound2Upgrade043AbortsOnCompetingOwners(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_upg_conflict")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 conflict")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines[0].ClientKey = "client:conflict-a"
	adopted := round2Update(t, database, ctx, draft, user, lines)
	row1 := adopted.Items[0].ID
	row2 := adopted.Items[1].ID
	// A second retained identity binds the same alias to a different row.
	// Crafted via SQL: two positive owners for one (PO, alias).
	var sessID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM `+schema+`.purchase_order_reconciliation_sessions WHERE purchase_order_id = $1 AND active`, draft.ID).Scan(&sessID); err != nil {
		t.Fatalf("find session: %v", err)
	}
	conflictJSON := fmt.Sprintf(`[{"Key":"client:conflict-a","ID":%d,"Removed":false}]`, row2)
	if _, err := pool.Exec(ctx, `UPDATE `+schema+`.purchase_order_reconciliation_sessions SET line_identities = line_identities || $2::jsonb WHERE id = $1`, sessID, conflictJSON); err != nil {
		t.Fatalf("seed conflict: %v", err)
	}
	_ = row1

	round2ResetTo042(t, database, ctx)
	if err := database.Migrate(ctx); err == nil {
		t.Fatal("competing owners migrated; want abort")
	} else if got := err.Error(); !containsFold(got, "competing owners") {
		t.Fatalf("abort = %v; want competing-owners diagnostic", err)
	}
	if round2LedgerHas43(t, database, ctx) {
		t.Error("ledger has 43 after aborted migration")
	}
	var tables int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'purchase_order_line_aliases'`, database.config.Schema).Scan(&tables); err != nil {
		t.Fatalf("check table: %v", err)
	}
	if tables != 0 {
		// The migration runs in one transaction: DDL rolls back with data.
		// If the table exists here it must be empty (defensive; normally 0).
		var claims int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+schema+`.purchase_order_line_aliases`).Scan(&claims); err != nil {
			t.Fatalf("count claims: %v", err)
		}
		if claims != 0 {
			t.Errorf("partial claims = %d after abort; want none", claims)
		}
	}
}

// Malformed retained evidence aborts the migration.
func TestRound2Upgrade043AbortsOnMalformedEvidence(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_upg_malformed")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 malformed")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	var sessID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM `+schema+`.purchase_order_reconciliation_sessions WHERE purchase_order_id = $1 AND active`, draft.ID).Scan(&sessID); err != nil {
		t.Fatalf("find session: %v", err)
	}
	// ID as a non-numeric string: not recoverable as ownership.
	if _, err := pool.Exec(ctx, `UPDATE `+schema+`.purchase_order_reconciliation_sessions SET line_identities = line_identities || $2::jsonb WHERE id = $1`,
		sessID, `[{"Key":"client:malformed-a","ID":"oops","Removed":false}]`); err != nil {
		t.Fatalf("seed malformed: %v", err)
	}
	round2ResetTo042(t, database, ctx)
	if err := database.Migrate(ctx); err == nil {
		t.Fatal("malformed evidence migrated; want abort")
	} else if got := err.Error(); !containsFold(got, "malformed retained alias owner") {
		t.Fatalf("abort = %v; want malformed-owner diagnostic", err)
	}
	if round2LedgerHas43(t, database, ctx) {
		t.Error("ledger has 43 after malformed abort")
	}
}

// A positive owner belonging to another PO aborts the migration.
func TestRound2Upgrade043AbortsOnForeignOwner(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_upg_foreign")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	firstQuotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 foreign first")
	secondQuotation := seedFinalizeQuotation(t, database, 3, "PT. Other", "R2 foreign second")
	first := createFinalizeDraft(t, database, ctx, user, firstQuotation)
	second := createFinalizeDraft(t, database, ctx, user, secondQuotation)
	foreignRow := first.Items[0].ID
	var sessID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM `+schema+`.purchase_order_reconciliation_sessions WHERE purchase_order_id = $1 AND active`, second.ID).Scan(&sessID); err != nil {
		t.Fatalf("find session: %v", err)
	}
	foreignJSON := fmt.Sprintf(`[{"Key":"client:foreign-a","ID":%d,"Removed":false}]`, foreignRow)
	if _, err := pool.Exec(ctx, `UPDATE `+schema+`.purchase_order_reconciliation_sessions SET line_identities = line_identities || $2::jsonb WHERE id = $1`, sessID, foreignJSON); err != nil {
		t.Fatalf("seed foreign: %v", err)
	}
	round2ResetTo042(t, database, ctx)
	if err := database.Migrate(ctx); err == nil {
		t.Fatal("foreign owner migrated; want abort")
	} else if got := err.Error(); !containsFold(got, "another purchase order") {
		t.Fatalf("abort = %v; want foreign-owner diagnostic", err)
	}
	if round2LedgerHas43(t, database, ctx) {
		t.Error("ledger has 43 after foreign abort")
	}
}

func containsFold(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		h, n := haystack, needle
		for i := 0; i+len(n) <= len(h); i++ {
			match := true
			for j := 0; j < len(n); j++ {
				a, b := h[i+j], n[j]
				if a >= 'A' && a <= 'Z' {
					a += 'a' - 'A'
				}
				if b >= 'A' && b <= 'Z' {
					b += 'a' - 'A'
				}
				if a != b {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
		return false
	})()
}

// Claim ownership is append-only: direct UPDATE always fails; direct DELETE
// fails while the PO exists. Physical PO deletion cascades claims; source
// deletion preserves them.
func TestRound2AliasGuardAndCascade(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_guard")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 guard")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines[0].ClientKey = "client:guard-a"
	adopted := round2Update(t, database, ctx, draft, user, lines)
	_ = adopted

	if _, err := pool.Exec(ctx, `UPDATE `+schema+`.purchase_order_line_aliases SET owner_line_id = 999999 WHERE purchase_order_id = $1`, draft.ID); err == nil {
		t.Error("direct claim UPDATE succeeded; ownership must be append-only")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM `+schema+`.purchase_order_line_aliases WHERE purchase_order_id = $1`, draft.ID); err == nil {
		t.Error("direct claim DELETE with live parent succeeded; claims must be retained")
	}
	if got := round2Claims(t, database, ctx, draft.ID); got["client:guard-a"] == nil || *got["client:guard-a"] < 1 {
		t.Errorf("guard probe disturbed claims: %+v", got)
	}

	// Physical PO deletion cascades its claims.
	cascadeQuotation := seedFinalizeQuotation(t, database, 5, "PT. Cascade", "R2 cascade")
	cascadeDraft := createFinalizeDraft(t, database, ctx, user, cascadeQuotation)
	cLines := append([]PurchaseOrderItem{}, cascadeDraft.Items...)
	cLines[0].ClientKey = "client:cascade-a"
	cAdopted := round2Update(t, database, ctx, cascadeDraft, user, cLines)
	_ = cAdopted
	if _, err := pool.Exec(ctx, `DELETE FROM `+schema+`.purchase_orders WHERE id = $1`, cascadeDraft.ID); err != nil {
		t.Fatalf("delete PO: %v", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+schema+`.purchase_order_line_aliases WHERE purchase_order_id = $1`, cascadeDraft.ID).Scan(&remaining); err != nil {
		t.Fatalf("count cascaded: %v", err)
	}
	if remaining != 0 {
		t.Errorf("claims after PO delete = %d; want 0 via cascade", remaining)
	}

	// Source deletion preserves alias ownership (former-source drafts keep
	// their removed-claim refusal without a session requirement).
	formerQuotation := seedFinalizeQuotation(t, database, 6, "PT. Former", "R2 former")
	formerDraft := createFinalizeDraft(t, database, ctx, user, formerQuotation)
	fLines := append([]PurchaseOrderItem{}, formerDraft.Items...)
	fLines = append(fLines, PurchaseOrderItem{
		ClientKey: "client:former-a", Description: "Former freight",
		QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50,
	})
	fExtra := round2Update(t, database, ctx, formerDraft, user, fLines)
	fTrimmed := round2Update(t, database, ctx, fExtra, user, fExtra.Items[:2])
	_ = fTrimmed
	admin := deletionUser(t, database, "admin")
	preview := previewFor(t, database, admin, DeletionTargetQuotation, formerQuotation.ID)
	if _, err := database.ConfirmDeletion(ctx, DeletionConfirmInput{
		TargetType: DeletionTargetQuotation, TargetID: formerQuotation.ID,
		Scope: DeletionScopeRecord, Reason: "issued by mistake",
		TypedNumber: formerQuotation.QuoteNumber, ExpectedFingerprint: preview.Fingerprint,
		ActorID: admin.ID,
	}); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	claims := round2Claims(t, database, ctx, formerDraft.ID)
	if got := claims["client:former-a"]; got == nil {
		t.Errorf("source deletion erased alias ownership: %+v", claims)
	}
	former, err := database.GetPurchaseOrder(ctx, formerDraft.ID)
	if err != nil {
		t.Fatalf("reopen former: %v", err)
	}
	if former.SourceState != PurchaseOrderSourceStateFormer {
		t.Fatalf("source state = %q; want former", former.SourceState)
	}
	probe := append([]PurchaseOrderItem{}, former.Items...)
	probe = append(probe, PurchaseOrderItem{
		ClientKey: "client:former-a", Description: "Reuse after source delete",
		QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 1,
	})
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: former.ID, UpdatedBy: user.ID, Items: probe,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("former-source retired alias reused: %v; want ErrPurchaseOrderInvalid", err)
	}
}

// A lost adopted alias with no retained evidence cannot be backfilled and is
// honestly indistinguishable from a never-used alias: reuse succeeds. This
// documents the frozen historical limit; it must not invent a claim.
func TestRound2LostAliasIsUnrecoverable(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_lost")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 lost")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	// Simulate a pre-fix loss: alias used and erased from both live rows and
	// retained identities. No evidence row exists for it.
	round2ResetTo042(t, database, ctx)
	// Erase any session client evidence (keep only id: keys) to model total loss.
	var sessID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM `+schema+`.purchase_order_reconciliation_sessions WHERE purchase_order_id = $1 AND active`, draft.ID).Scan(&sessID); err != nil {
		t.Fatalf("find session: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE `+schema+`.purchase_order_reconciliation_sessions SET line_identities = (
		SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb) FROM jsonb_array_elements(line_identities) elem WHERE (elem->>'Key') NOT LIKE 'client:%'
	) WHERE id = $1`, sessID); err != nil {
		t.Fatalf("erase client evidence: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	claims := round2Claims(t, database, ctx, draft.ID)
	if _, ok := claims["client:never-existed"]; ok {
		t.Error("unseen alias has a claim; backfill must not invent aliases")
	}
	// Reuse of an unseen spelling succeeds: the limit is honest, not a hidden refusal.
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	probe := append([]PurchaseOrderItem{}, fresh.Items...)
	probe = append(probe, PurchaseOrderItem{
		ClientKey: "client:never-existed", Description: "First use",
		QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 1,
	})
	updated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: probe,
	})
	if err != nil {
		t.Fatalf("first use of unseen alias refused: %v", err)
	}
	_ = updated
}

// Identical-valued lines keep distinct rows/owners/choices through edit and
// reorder; a foreign PO row ID and a cross-PO alias are refused.
func TestRound2IdenticalRowsAndForeignClaims(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_identical")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 identical")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	a := PurchaseOrderItem{ClientKey: "client:identical-a", Description: "Same", QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 100}
	b := a
	b.ClientKey = "client:identical-b"
	// Replace both source lines with two identical-valued alias rows.
	updated := round2Update(t, database, ctx, draft, user, []PurchaseOrderItem{a, b})
	idA, idB := updated.Items[0].ID, updated.Items[1].ID
	if idA == idB {
		t.Fatalf("identical lines share row %d", idA)
	}
	claims := round2Claims(t, database, ctx, draft.ID)
	if claims["client:identical-a"] == nil || *claims["client:identical-a"] != idA {
		t.Errorf("claim A = %v; want %d", claims["client:identical-a"], idA)
	}
	if claims["client:identical-b"] == nil || *claims["client:identical-b"] != idB {
		t.Errorf("claim B = %v; want %d", claims["client:identical-b"], idB)
	}
	// Reorder with an edit: IDs must follow values, not positions.
	reloaded, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	swap := []PurchaseOrderItem{reloaded.Items[1], reloaded.Items[0]}
	swap[0].Description = "Edited B"
	swapped := round2Update(t, database, ctx, reloaded, user, swap)
	// swap[0] was idB (now edited), swap[1] was idA.
	if swapped.Items[0].ID != idB || swapped.Items[1].ID != idA {
		t.Fatalf("identical rows inherited IDs: %+v; want %d/%d", swapped.Items, idB, idA)
	}

	// A foreign PO row ID is refused.
	otherQuotation := seedFinalizeQuotation(t, database, 9, "PT. Other", "R2 foreign")
	other := createFinalizeDraft(t, database, ctx, user, otherQuotation)
	lines := append([]PurchaseOrderItem{}, swapped.Items...)
	lines[0].ID = other.Items[0].ID
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: swapped.ID, UpdatedBy: user.ID, Items: lines,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("foreign row id accepted: %v; want ErrPurchaseOrderInvalid", err)
	}

	// Claiming A with another live row ID is refused.
	fresh, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	conflict := append([]PurchaseOrderItem{}, fresh.Items...)
	// conflict[0] owns B; pointing it at A's alias claims another row's key.
	conflict[0].ClientKey = "client:identical-a"
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: conflict,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Errorf("duplicate alias claim accepted: %v; want ErrPurchaseOrderInvalid", err)
	}
}

// One save carrying a valid new alias plus a conflicting alias rolls back
// entirely and leaves no poisoned claim for the valid alias.
func TestRound2ConflictingBatchLeavesNoPoisonedClaim(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_poison")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 poison")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	lines := append([]PurchaseOrderItem{}, draft.Items...)
	lines = append(lines, PurchaseOrderItem{
		ClientKey: "client:poison-retired", Description: "Doomed",
		QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50,
	})
	withExtra := round2Update(t, database, ctx, draft, user, lines)
	clean := round2Update(t, database, ctx, withExtra, user, withExtra.Items[:2])
	before := round2Claims(t, database, ctx, draft.ID)

	poison := append([]PurchaseOrderItem{}, clean.Items...)
	poison = append(poison,
		PurchaseOrderItem{
			ClientKey: "client:poison-fresh", Description: "Valid new",
			QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 10,
		},
		PurchaseOrderItem{
			ClientKey: "client:poison-retired", Description: "Conflicting reuse",
			QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 20,
		},
	)
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: poison,
	}); !errors.Is(err, ErrPurchaseOrderInvalid) {
		t.Fatalf("conflicting batch = %v; want ErrPurchaseOrderInvalid", err)
	}
	after, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if len(after.Items) != len(clean.Items) {
		t.Errorf("lines %d -> %d on refused batch", len(clean.Items), len(after.Items))
	}
	claims := round2Claims(t, database, ctx, draft.ID)
	if _, ok := claims["client:poison-fresh"]; ok {
		t.Errorf("poisoned claim for valid alias retained: %+v", claims)
	}
	if len(claims) != len(before) {
		t.Errorf("claims %d -> %d on refused batch", len(before), len(claims))
	}
	// The valid alias remains usable alone afterward.
	retry := append([]PurchaseOrderItem{}, clean.Items...)
	retry = append(retry, PurchaseOrderItem{
		ClientKey: "client:poison-fresh", Description: "Valid new",
		QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 10,
	})
	if _, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: draft.ID, UpdatedBy: user.ID, Items: retry,
	}); err != nil {
		t.Errorf("valid alias refused after rollback: %v", err)
	}
}

// Source-created and blank-created drafts acquire claims through the same
// seam; a second PO reusing the alias stays independent.
func TestRound2CreatePathsAcquireClaims(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_create")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 create")
	created := createFinalizeDraft(t, database, ctx, user, quotation)
	claims := round2Claims(t, database, ctx, created.ID)
	if len(claims) != 0 {
		t.Errorf("source-created claims = %+v; want none before aliases", claims)
	}
	lines := append([]PurchaseOrderItem{}, created.Items...)
	lines[0].ClientKey = "client:create-a"
	adopted := round2Update(t, database, ctx, created, user, lines)
	if got := round2Claims(t, database, ctx, created.ID)["client:create-a"]; got == nil || *got != adopted.Items[0].ID {
		t.Errorf("adopted claim = %v; want row %d", got, adopted.Items[0].ID)
	}

	blank := createBlankDraft(t, database, ctx, user)
	if got := len(round2Claims(t, database, ctx, blank.ID)); got != 0 {
		t.Errorf("blank claims = %d; want none", got)
	}
	blankLines := []PurchaseOrderItem{{
		ClientKey: "client:create-blank", Description: "Blank line",
		QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 5,
	}}
	blankUpdated, err := updateDraft(t, database, ctx, PurchaseOrderUpdate{
		ID: blank.ID, UpdatedBy: user.ID, Items: blankLines,
	})
	if err != nil {
		t.Fatalf("blank save: %v", err)
	}
	if got := round2Claims(t, database, ctx, blank.ID)["client:create-blank"]; got == nil || *got != blankUpdated.Items[0].ID {
		t.Errorf("blank claim = %v; want row %d", got, blankUpdated.Items[0].ID)
	}
}

// Typed duplicates in both values leave version/items/history unchanged;
// range/type/trailing/malformed shapes stay refused; good boundaries pass.
func TestRound2TypedDuplicatesBothValuesAndSafety(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_typeboth")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 both values")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	key := purchaseOrderLineKeyForRow(draft.Items[0].ID)
	goodDiscount := `{"Enabled":false,"Percent":0}`

	dupCases := []struct{ name, raw string }{
		{"same Enabled", `{"Enabled":true,"Enabled":false,"Percent":0}`},
		{"same Percent identical", `{"Enabled":true,"Percent":5,"Percent":5}`},
		{"mixed Percent", `{"Enabled":true,"Percent":5,"percent":0}`},
		{"escaped Enabled", "{\"\\u0045nabled\":true,\"Enabled\":false,\"Percent\":0}"},
	}
	for _, side := range []string{"PO", "Source"} {
		for _, tc := range dupCases {
			t.Run(side+"/"+tc.name, func(t *testing.T) {
				loaded, err := database.GetPOReconciliation(ctx, draft.ID)
				if err != nil {
					t.Fatalf("load: %v", err)
				}
				poRaw, srcRaw := goodDiscount, goodDiscount
				if side == "PO" {
					poRaw = tc.raw
				} else {
					srcRaw = tc.raw
				}
				loaded.Items = []POReconciliationItem{{
					Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
					LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
					POValue: json.RawMessage(poRaw), SourceValue: json.RawMessage(srcRaw),
					Status: POReconciliationStatusUnresolved,
				}}
				beforePO, _ := database.GetPurchaseOrder(ctx, draft.ID)
				beforeSess, _ := database.GetPOReconciliation(ctx, draft.ID)
				if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
					t.Fatalf("duplicate in %s value saved: %v; want ErrPurchaseOrderInvalid", side, err)
				}
				afterPO, _ := database.GetPurchaseOrder(ctx, draft.ID)
				afterSess, _ := database.GetPOReconciliation(ctx, draft.ID)
				if afterPO.Version != beforePO.Version {
					t.Errorf("PO version %d -> %d on refused duplicate", beforePO.Version, afterPO.Version)
				}
				if len(afterSess.Items) != len(beforeSess.Items) {
					t.Errorf("session items %d -> %d on refused duplicate", len(beforeSess.Items), len(afterSess.Items))
				}
			})
		}
	}

	// Range/type/trailing negatives via the real writer.
	negatives := []struct{ name, raw string }{
		{"percent -1", `{"Enabled":true,"Percent":-1}`},
		{"percent 101", `{"Enabled":true,"Percent":101}`},
		{"percent fraction", `{"Enabled":true,"Percent":2.5}`},
		{"percent string", `{"Enabled":true,"Percent":"5"}`},
		{"percent exponent", `{"Enabled":true,"Percent":1e2}`},
		{"percent overflow", `{"Enabled":true,"Percent":999999999999999999999}`},
		{"enabled string", `{"Enabled":"true","Percent":5}`},
		{"null root", `null`},
		{"array root", `[]`},
		{"nested percent", `{"Enabled":true,"Percent":{"v":5}}`},
		{"trailing junk", `{"Enabled":true,"Percent":5} xyz`},
		{"second value", `{"Enabled":true,"Percent":5} {"Enabled":false,"Percent":0}`},
	}
	for _, tc := range negatives {
		t.Run("discount-neg/"+tc.name, func(t *testing.T) {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
				POValue: json.RawMessage(tc.raw), SourceValue: json.RawMessage(goodDiscount),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Errorf("negative %s saved; want ErrPurchaseOrderInvalid", tc.name)
			}
		})
	}

	// Boundary positives pass.
	for _, tc := range []struct{ name, raw string }{
		{"zero", `{"Enabled":true,"Percent":0}`},
		{"hundred", `{"Enabled":true,"Percent":100}`},
		{"disabled nonzero canonicalizes", `{"Enabled":false,"Percent":5}`},
	} {
		t.Run("discount-pos/"+tc.name, func(t *testing.T) {
			loaded, err := database.GetPOReconciliation(ctx, draft.ID)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			loaded.Items = []POReconciliationItem{{
				Key: "line:" + key + ":discount", Field: "line:discount", Kind: POReconciliationKindDifference,
				LineKey: strPointer(key), SourceLineRef: strPointer("1"), Label: "Line 1 discount",
				POValue: json.RawMessage(tc.raw), SourceValue: json.RawMessage(goodDiscount),
				Status: POReconciliationStatusUnresolved,
			}}
			if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
				t.Errorf("positive %s refused: %v", tc.name, err)
			}
		})
	}

	// Tax duplicates in the source value end in the unchanged protected
	// triple, so read-only rejection cannot hide the parser defect.
	t.Run("tax-source-duplicate-keeps-triple", func(t *testing.T) {
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		for i := range loaded.Items {
			if loaded.Items[i].Field == "link:taxBasis" {
				loaded.Items[i].SourceValue = json.RawMessage(`{"TaxZone":"FTZ","TaxZone":"NonFTZ","TaxType":"Goods","ServiceCategory":""}`)
			}
		}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); !errors.Is(err, ErrPurchaseOrderInvalid) {
			t.Fatalf("source tax duplicate saved; want ErrPurchaseOrderInvalid")
		}
		reloaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		tax := findSessionItem(t, reloaded.Items, "link:taxBasis")
		var triple POReconciliationTaxBasis
		if err := json.Unmarshal(tax.POValue, &triple); err != nil {
			t.Fatalf("decode triple: %v", err)
		}
		if triple != (POReconciliationTaxBasis{TaxZone: "FTZ", TaxType: "Goods"}) {
			t.Errorf("triple mutated to %+v", triple)
		}
	})

	// Valid full tax triples pass, including empty ServiceCategory.
	t.Run("tax-good-triple", func(t *testing.T) {
		loaded, err := database.GetPOReconciliation(ctx, draft.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		for i := range loaded.Items {
			if loaded.Items[i].Field == "link:taxBasis" {
				loaded.Items[i].POValue = json.RawMessage(`{"TaxZone":"FTZ","TaxType":"Goods","ServiceCategory":""}`)
				loaded.Items[i].SourceValue = json.RawMessage(`{"TaxZone":"FTZ","TaxType":"Goods","ServiceCategory":""}`)
			}
		}
		if _, err := database.SavePOReconciliation(ctx, loaded, user.ID); err != nil {
			t.Errorf("good triple refused: %v", err)
		}
	})
}

// Same-PO concurrent writers keep version-conflict semantics; distinct POs
// sharing one alias do not contend on alias uniqueness.
func TestRound2ConcurrentAliasWriters(t *testing.T) {
	database := openTestSchema(t, "vera_po_r2_concurrent")
	user := poTestUser(t, database)
	ctx := poContext(t, database)

	quotation := seedFinalizeQuotation(t, database, 2, "PT. Buyer", "R2 concurrent")
	draft := createFinalizeDraft(t, database, ctx, user, quotation)
	base, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	// Two writers race on the same base version with different aliases.
	results := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	done := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			items := append([]PurchaseOrderItem{}, base.Items...)
			items = append(items, PurchaseOrderItem{
				ClientKey: fmt.Sprintf("client:race-%d", i), Description: fmt.Sprintf("Race %d", i),
				QuantityScaled: 10000, UoM: "Pc", UnitPriceMinor: 10,
			})
			_, err := database.UpdatePurchaseOrderDraft(ctx, PurchaseOrderUpdate{
				ID: base.ID, UpdatedBy: user.ID, Items: items, ExpectedVersion: base.Version,
			})
			results[i] = err
			done <- i
		}(i)
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrPurchaseOrderVersionConflict):
			conflicts++
		default:
			t.Fatalf("race error = %v; want success or version conflict", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("same-PO race = %d success / %d conflict; want 1/1", successes, conflicts)
	}

	// Distinct POs sharing one alias both succeed.
	firstQuotation := seedFinalizeQuotation(t, database, 7, "PT. A", "R2 conc A")
	secondQuotation := seedFinalizeQuotation(t, database, 8, "PT. B", "R2 conc B")
	first := createFinalizeDraft(t, database, ctx, user, firstQuotation)
	second := createFinalizeDraft(t, database, ctx, user, secondQuotation)
	alias := "client:concurrent-shared"
	errs := make([]error, 2)
	done2 := make(chan int, 2)
	go func() {
		lines := append([]PurchaseOrderItem{}, first.Items...)
		lines = append(lines, PurchaseOrderItem{
			ClientKey: alias, Description: "Shared A",
			QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 50,
		})
		_, errs[0] = updateDraft(t, database, ctx, PurchaseOrderUpdate{
			ID: first.ID, UpdatedBy: user.ID, Items: lines,
		})
		done2 <- 0
	}()
	go func() {
		lines := append([]PurchaseOrderItem{}, second.Items...)
		lines = append(lines, PurchaseOrderItem{
			ClientKey: alias, Description: "Shared B",
			QuantityScaled: 10000, UoM: "Lot", UnitPriceMinor: 60,
		})
		_, errs[1] = updateDraft(t, database, ctx, PurchaseOrderUpdate{
			ID: second.ID, UpdatedBy: user.ID, Items: lines,
		})
		done2 <- 1
	}()
	<-done2
	<-done2
	for i, err := range errs {
		if err != nil {
			t.Errorf("cross-PO writer %d = %v; want success", i, err)
		}
	}
}
