package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// B2 (#518): a source repair installs the next reconciliation generation.
//
// Before B2 the repair wrote the record, the correction and the satellite only;
// the reconciliation history still pointed at the old session, which stayed
// active and unchanged. These tests are the red side of that gap and then the
// contract of the new generation.

// poRepairSavedOrder finalizes one PO from an issued revision and returns the
// saved record plus the actors a repair needs.
func poRepairSavedOrder(t *testing.T, database *Store, ctx context.Context,
	quotation QuotationRecord, number string) (PurchaseOrder, AuthUser, AuthUser) {
	t.Helper()
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	draft := createFinalizeDraft(t, database, ctx, staff, quotation)
	input := finalizeInput(draft, staff, number)
	saved, err := database.FinalizePurchaseOrder(ctx, input)
	if err != nil {
		t.Fatalf("finalize %q: %v", number, err)
	}
	return saved, staff, admin
}

func poRepairHistory(t *testing.T, database *Store, ctx context.Context, poID int64) POReconciliationSessions {
	t.Helper()
	history, err := database.ListPOReconciliationSessions(ctx, poID)
	if err != nil {
		t.Fatalf("list reconciliation history: %v", err)
	}
	return history
}

func poRepairFindItem(t *testing.T, session POReconciliationSession, key string) POReconciliationItem {
	t.Helper()
	for _, item := range session.Items {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("session %d has no item %q; items: %v", session.ID, key, poRepairItemKeys(session))
	return POReconciliationItem{}
}

func poRepairItemKeys(session POReconciliationSession) []string {
	keys := make([]string, 0, len(session.Items))
	for _, item := range session.Items {
		keys = append(keys, item.Key)
	}
	return keys
}

// TestRepairInstallsReadOnlyReconciliationGeneration is B2's first red proof.
func TestRepairInstallsReadOnlyReconciliationGeneration(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_newgen")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-GEN")

	before := poRepairHistory(t, database, ctx, saved.ID)
	if len(before.Sessions) != 1 || !before.Sessions[0].Session.Active {
		t.Fatalf("pre-repair generations = %d; want one active session", len(before.Sessions))
	}

	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "follow the right revision"))
	if err != nil {
		t.Fatalf("repair: %v", err)
	}

	after := poRepairHistory(t, database, ctx, repaired.ID)
	if len(after.Sessions) != 2 {
		t.Fatalf("post-repair generations = %d; want 2 (retained + installed)", len(after.Sessions))
	}
	current := after.Sessions[0]
	previous := after.Sessions[1]

	if current.Generation != 2 {
		t.Errorf("installed generation = %d; want 2", current.Generation)
	}
	if !current.Session.Active || !current.Session.ReadOnly {
		t.Errorf("installed session active/read-only = %v/%v; want true/true",
			current.Session.Active, current.Session.ReadOnly)
	}
	if current.RepairCorrectionID == nil {
		t.Errorf("installed session carries no repair correction link")
	}
	if current.Session.POVersion != repaired.Version {
		t.Errorf("installed session version = %d; want the resulting %d",
			current.Session.POVersion, repaired.Version)
	}
	if current.Session.SourceQuotationID != target.ID ||
		current.Session.SourceRevision != target.RevisionNumber {
		t.Errorf("installed session source = %d/%d; want %d/%d",
			current.Session.SourceQuotationID, current.Session.SourceRevision,
			target.ID, target.RevisionNumber)
	}
	if !current.SourceAvailable {
		t.Errorf("installed session source pointer is not available")
	}
	if previous.RepairCorrectionID != nil {
		t.Errorf("the retained create generation gained a repair link")
	}
	if previous.Session.Active {
		t.Errorf("the old generation is still active")
	}
	if !previous.Session.ReadOnly {
		t.Errorf("the old generation is not read-only")
	}
	if len(previous.Session.Items) == 0 {
		t.Errorf("the old generation lost its retained items")
	}

	// The full seed always carries the adopted tax basis.
	tax := poRepairFindItem(t, current.Session, "link:taxBasis")
	if tax.Kind != POReconciliationKindTaxBasis || tax.Resolution == nil ||
		*tax.Resolution != POReconciliationResolutionUsedQuote {
		t.Errorf("tax item kind/resolution = %q/%v; want tax_basis/used_quotation",
			tax.Kind, tax.Resolution)
	}
}

// TestRepairUseKeepsThePreUseComparison is the plan's concrete proof: the
// admin applies the quotation's quantity (3 → 2) with an explicit choice, and
// the new session still records that 3 was compared against 2.
func TestRepairUseKeepsThePreUseComparison(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_use")
	ctx := poContext(t, database)
	// The stored record follows a revision that carries quantity 3; the target
	// revision carries 2, so Use is a real 3 -> 2 move.
	original := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Original",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "FTZ",
		TaxType: "Goods", UnitPrice: 100, Quantity: 3, PaymentTermDays: 45,
	})
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-USE")

	input := repairInput(t, saved, admin, target, "adopt the corrected quantity")
	input.Items[0].QuantityScaled = 2 * QuantityScale
	input.Decisions = []PurchaseOrderDecision{{
		Key:    "line:1:quantity",
		Choice: PurchaseOrderChoiceQuotation,
	}}
	repaired, err := database.RepairPurchaseOrderSource(ctx, input)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}

	history := poRepairHistory(t, database, ctx, repaired.ID)
	current := history.Sessions[0]
	// The identity is the first-seen key of the surviving row; resolve it from
	// the installed session rather than assuming the row id.
	key := current.Session.LineIdentities[0].Key
	item := poRepairFindItem(t, current.Session, "line:"+key+":quantity")

	if item.Status != POReconciliationStatusResolved {
		t.Errorf("item status = %q; want resolved", item.Status)
	}
	if item.Resolution == nil || *item.Resolution != POReconciliationResolutionUsedQuote {
		t.Fatalf("item resolution = %v; want used_quotation", item.Resolution)
	}
	var applied int64
	if err := json.Unmarshal(item.POValue, &applied); err != nil || applied != 2*QuantityScale {
		t.Errorf("item POValue = %s; want the applied %d (%v)", item.POValue, 2*QuantityScale, err)
	}
	var sourceValue int64
	if err := json.Unmarshal(item.SourceValue, &sourceValue); err != nil || sourceValue != 2*QuantityScale {
		t.Errorf("item SourceValue = %s; want %d (%v)", item.SourceValue, 2*QuantityScale, err)
	}
	if len(item.History) < 2 {
		t.Fatalf("item history has %d events; want the comparison and the decision", len(item.History))
	}
	var compared int64
	if err := json.Unmarshal(item.History[0].POValue, &compared); err != nil || compared != 3*QuantityScale {
		t.Errorf("first history POValue = %s; want the pre-Use %d (%v)",
			item.History[0].POValue, 3*QuantityScale, err)
	}
	last := item.History[len(item.History)-1]
	if last.Resolution == nil || *last.Resolution != POReconciliationResolutionUsedQuote {
		t.Errorf("terminal history resolution = %v; want used_quotation", last.Resolution)
	}
}

// TestRepairRecordsTheTaxTripleChange proves the semantic triple is persisted,
// not just the legacy category: a Goods record repaired onto a Services target
// must carry type and service category afterwards, with the old triple in the
// tax item.
func TestRepairRecordsTheTaxTripleChange(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_tax")
	ctx := poContext(t, database)
	original := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Original",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "NonFTZ",
		TaxType: "Goods", UnitPrice: 100, PaymentTermDays: 45,
	})
	target := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Services",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "NonFTZ",
		TaxType: "Services", ServiceCategory: "Construction",
		UnitPrice: 100, PaymentTermDays: 45,
	})
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-TAX")

	input := repairInput(t, saved, admin, target, "the basis changed to services")
	repaired, err := database.RepairPurchaseOrderSource(ctx, input)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if repaired.TaxType != "Services" || repaired.ServiceCategory != "Construction" {
		t.Errorf("persisted triple = %q/%q; want Services/Construction",
			repaired.TaxType, repaired.ServiceCategory)
	}

	history := poRepairHistory(t, database, ctx, repaired.ID)
	tax := poRepairFindItem(t, history.Sessions[0].Session, "link:taxBasis")
	var beforeTriple, afterTriple POReconciliationTaxBasis
	if err := json.Unmarshal(tax.POValue, &beforeTriple); err != nil {
		t.Fatalf("decode tax POValue: %v", err)
	}
	if err := json.Unmarshal(tax.SourceValue, &afterTriple); err != nil {
		t.Fatalf("decode tax SourceValue: %v", err)
	}
	if beforeTriple.TaxType != "Goods" {
		t.Errorf("tax history before = %+v; want the pre-repair Goods basis", beforeTriple)
	}
	if afterTriple.TaxType != "Services" || afterTriple.ServiceCategory != "Construction" {
		t.Errorf("tax history after = %+v; want the adopted Services basis", afterTriple)
	}
}

// TestRepairLeavesNoGenerationOnRefusal proves the negative path: a refused
// repair writes no generation and moves nothing.
func TestRepairLeavesNoGenerationOnRefusal(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_refusal")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, staff, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-REFUSE")

	before, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload before refusal: %v", err)
	}
	beforeHistory := poRepairHistory(t, database, ctx, saved.ID)

	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, staff, target, "staff may not repair")); !errors.Is(err, ErrPurchaseOrderRepairForbidden) {
		t.Fatalf("staff repair error = %v; want ErrPurchaseOrderRepairForbidden", err)
	}
	stale := repairInput(t, saved, admin, target, "stale version")
	stale.ExpectedVersion = saved.Version - 1
	if _, err := database.RepairPurchaseOrderSource(ctx, stale); err == nil {
		t.Fatalf("stale repair succeeded")
	}
	unchanged := repairInput(t, saved, admin, original, "same source")
	unchanged.QuotationID = original.ID
	unchanged.RevisionNumber = original.RevisionNumber
	if _, err := database.RepairPurchaseOrderSource(ctx, unchanged); !errors.Is(err, ErrPurchaseOrderSourceUnchanged) {
		t.Fatalf("same-source repair error = %v; want ErrPurchaseOrderSourceUnchanged", err)
	}

	after, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload after refusals: %v", err)
	}
	if after.Version != before.Version {
		t.Errorf("a refused repair bumped the version: %d -> %d", before.Version, after.Version)
	}
	afterHistory := poRepairHistory(t, database, ctx, saved.ID)
	if len(afterHistory.Sessions) != len(beforeHistory.Sessions) {
		t.Errorf("generations changed on a refused repair: %d -> %d",
			len(beforeHistory.Sessions), len(afterHistory.Sessions))
	}
	if afterHistory.Sessions[0].Session.ID != beforeHistory.Sessions[0].Session.ID {
		t.Errorf("the active session changed on a refused repair")
	}
}

// TestRepairGenerationRevisit keeps a return to a previously used revision legal
// and gives each move its own generation without touching the old ones.
func TestRepairGenerationRevisit(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_revisit")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-REVISIT")

	first, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "move to the replacement"))
	if err != nil {
		t.Fatalf("first repair: %v", err)
	}
	second, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, first, admin, original, "back to the original revision"))
	if err != nil {
		t.Fatalf("second repair: %v", err)
	}

	history := poRepairHistory(t, database, ctx, second.ID)
	if len(history.Sessions) != 3 {
		t.Fatalf("generations = %d; want 3", len(history.Sessions))
	}
	for index, want := range []int{3, 2, 1} {
		if history.Sessions[index].Generation != want {
			t.Errorf("position %d generation = %d; want %d", index, history.Sessions[index].Generation, want)
		}
	}
	active := 0
	for _, record := range history.Sessions {
		if record.Session.Active {
			active++
		}
	}
	if active != 1 {
		t.Errorf("active generations = %d; want 1", active)
	}
	if history.Sessions[2].Session.SourceRevision != original.RevisionNumber {
		t.Errorf("the oldest generation was re-keyed")
	}
	if history.Sessions[1].Session.SourceRevision != target.RevisionNumber {
		t.Errorf("the middle generation was re-keyed")
	}
}

// TestRepairRetainsUntouchedAndRemovedLineState covers the line-set outcomes:
// a newly extra line is acknowledged, a removed line is retired, and a line
// that matches throughout produces no item.
func TestRepairRetainsUntouchedAndRemovedLineState(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_lines")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-LINES")

	input := repairInput(t, saved, admin, target, "drop the zero-value line")
	// Remove the second line (the zero-value item) and keep the first, which
	// matches the target throughout.
	input.Items = input.Items[:1]
	input.AcknowledgedMissing = []string{"3"}
	input.Decisions = nil
	if _, err := database.RepairPurchaseOrderSource(ctx, input); err != nil {
		t.Fatalf("repair: %v", err)
	}

	history := poRepairHistory(t, database, ctx, saved.ID)
	current := history.Sessions[0]
	if current.Session.Active != true {
		t.Fatalf("installed generation is not active")
	}
	for _, item := range current.Session.Items {
		switch item.Field {
		case "line:quantity", "line:unitPrice", "line:description", "line:uom", "line:discount":
			if item.Key == "line:id:1:description" || item.Key == "line:id:1:uom" {
				t.Errorf("a field that matches throughout produced item %q", item.Key)
			}
		}
	}
	// The removed zero-value line carried no field difference against the
	// target, so its retirement is recorded only through the identity.
	removed := false
	for _, identity := range current.Session.LineIdentities {
		if identity.Removed {
			removed = true
		}
	}
	if !removed {
		t.Errorf("the removed line has no retired identity: %v", current.Session.LineIdentities)
	}
	// The source line it used to claim becomes a missing-line item that the
	// admin acknowledged.
	ack := poRepairFindItem(t, current.Session, "source:3:match")
	if ack.Kind != POReconciliationKindMissingLine || ack.Resolution == nil ||
		*ack.Resolution != POReconciliationResolutionAckMiss {
		t.Errorf("missing item = %q/%v; want missing_line/acknowledged_missing", ack.Kind, ack.Resolution)
	}
}

// TestRepairInstallsIdentitiesFromThePreviousSession proves the new generation
// reuses the row's first-seen identity instead of minting a parallel key.
func TestRepairInstallsIdentitiesFromThePreviousSession(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_identity")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-ID")

	before := poRepairHistory(t, database, ctx, saved.ID)
	firstKeys := map[int64]string{}
	for _, identity := range before.Sessions[0].Session.LineIdentities {
		if identity.ID != nil {
			firstKeys[*identity.ID] = identity.Key
		}
	}
	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "move")); err != nil {
		t.Fatalf("repair: %v", err)
	}
	after := poRepairHistory(t, database, ctx, saved.ID)
	for _, identity := range after.Sessions[0].Session.LineIdentities {
		if identity.ID == nil || identity.Removed {
			continue
		}
		if want, known := firstKeys[*identity.ID]; known && identity.Key != want {
			t.Errorf("row %d identity = %q; want the first-seen %q", *identity.ID, identity.Key, want)
		}
	}
}

// TestRepairChoicesSeedTheFullItemSet drives one repair across every kept/used
// path at once and asserts the canonical item set: no item for a field that
// matches, a kept_po item that shows both sides, and a used_quotation item that
// keeps the pre-Use comparison.
func TestRepairChoicesSeedTheFullItemSet(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_fullseed")
	ctx := poContext(t, database)
	original := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Original",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "FTZ",
		TaxType: "Goods", UnitPrice: 100, Quantity: 3, PaymentTermDays: 45,
	})
	target := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Replacement",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "FTZ",
		TaxType: "Goods", UnitPrice: 120, Quantity: 2, PaymentTermDays: 30,
	})
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-SEED")

	input := repairInput(t, saved, admin, target, "review every changed slot")
	if len(input.Items) < 1 {
		t.Fatalf("fixture has no lines")
	}
	input.Items[0].QuantityScaled = 2 * QuantityScale
	input.Items[0].UnitPriceMinor = saved.Items[0].UnitPriceMinor
	input.PaymentTerms = saved.PaymentTerms // keep the stored wording
	input.Decisions = []PurchaseOrderDecision{
		{Key: "line:1:quantity", Choice: PurchaseOrderChoiceQuotation},
		{Key: "line:1:unitPrice", Choice: PurchaseOrderChoicePO},
		{Key: "header:paymentTerms", Choice: PurchaseOrderChoicePO},
	}
	// The second, zero-value line matches the target throughout.
	input.AcknowledgedMissing = nil
	if _, err := database.RepairPurchaseOrderSource(ctx, input); err != nil {
		t.Fatalf("repair: %v", err)
	}

	history := poRepairHistory(t, database, ctx, saved.ID)
	current := history.Sessions[0]
	var key string
	for _, identity := range current.Session.LineIdentities {
		if !identity.Removed {
			key = identity.Key
			break
		}
	}
	if key == "" {
		t.Fatalf("no surviving identity: %v", current.Session.LineIdentities)
	}

	quantity := poRepairFindItem(t, current.Session, "line:"+key+":quantity")
	if quantity.Resolution == nil || *quantity.Resolution != POReconciliationResolutionUsedQuote {
		t.Errorf("quantity resolution = %v; want used_quotation", quantity.Resolution)
	}
	var compared int64
	if err := json.Unmarshal(quantity.History[0].POValue, &compared); err != nil || compared != 3*QuantityScale {
		t.Errorf("quantity history = %s; want the pre-Use %d", quantity.History[0].POValue, 3*QuantityScale)
	}

	price := poRepairFindItem(t, current.Session, "line:"+key+":unitPrice")
	if price.Resolution == nil || *price.Resolution != POReconciliationResolutionKeptPO {
		t.Errorf("unit price resolution = %v; want kept_po", price.Resolution)
	}
	var keptPrice, targetPrice int64
	if err := json.Unmarshal(price.POValue, &keptPrice); err != nil {
		t.Fatalf("decode kept price: %v", err)
	}
	if err := json.Unmarshal(price.SourceValue, &targetPrice); err != nil {
		t.Fatalf("decode target price: %v", err)
	}
	if keptPrice != saved.Items[0].UnitPriceMinor {
		t.Errorf("kept price = %d; want the stored %d", keptPrice, saved.Items[0].UnitPriceMinor)
	}
	if targetPrice == keptPrice {
		t.Errorf("target price equals the kept price; the target difference was not recorded")
	}

	terms := poRepairFindItem(t, current.Session, "header:paymentTerms")
	if terms.Resolution == nil || *terms.Resolution != POReconciliationResolutionKeptPO {
		t.Errorf("payment terms resolution = %v; want kept_po", terms.Resolution)
	}
	var keptTerms, targetTerms string
	_ = json.Unmarshal(terms.POValue, &keptTerms)
	_ = json.Unmarshal(terms.SourceValue, &targetTerms)
	if keptTerms != "45 days" || targetTerms != "30 days" {
		t.Errorf("payment terms = %q vs %q; want 45 days vs 30 days", keptTerms, targetTerms)
	}

	for _, absent := range []string{
		"line:" + key + ":description",
		"line:" + key + ":uom",
		"line:" + key + ":discount",
		"header:incoterms",
	} {
		for _, item := range current.Session.Items {
			if item.Key == absent {
				t.Errorf("a field that matches throughout produced item %q", absent)
			}
		}
	}

	// The B1 read returns the same item set for the installed generation.
	read := poRepairHistory(t, database, ctx, saved.ID)
	if read.POVersion != current.Session.POVersion && read.Sessions[0].Session.ID != current.Session.ID {
		t.Errorf("the history read and the repair disagree about the current generation")
	}
}

// TestRepairCorrectionLeavesSessionsUntouched proves a later ordinary
// correction advances the record and its correction history only.
func TestRepairCorrectionLeavesSessionsUntouched(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_correction")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, staff, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-CORR")
	repaired, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, saved, admin, target, "move"))
	if err != nil {
		t.Fatalf("repair: %v", err)
	}

	before := poRepairHistory(t, database, ctx, repaired.ID)
	correction := correctionInput(repaired, staff, "fix the delivery note", repaired.Version)
	corrected, err := database.CorrectPurchaseOrder(ctx, correction)
	if err != nil {
		t.Fatalf("correction: %v", err)
	}
	if corrected.Version != repaired.Version+1 {
		t.Fatalf("correction version = %d; want %d", corrected.Version, repaired.Version+1)
	}

	after := poRepairHistory(t, database, ctx, repaired.ID)
	if len(after.Sessions) != len(before.Sessions) {
		t.Fatalf("a correction changed the generation count: %d -> %d",
			len(before.Sessions), len(after.Sessions))
	}
	for index := range before.Sessions {
		if before.Sessions[index].Session.ID != after.Sessions[index].Session.ID ||
			before.Sessions[index].Session.POVersion != after.Sessions[index].Session.POVersion {
			t.Errorf("a correction rewrote generation %d", before.Sessions[index].Generation)
		}
	}
	if after.POVersion != corrected.Version {
		t.Errorf("outer POVersion = %d; want the corrected %d", after.POVersion, corrected.Version)
	}
}

// TestRepairRemovesADifferingLine proves a retired line's field differences are
// recorded as removals, not as values that happened to match.
func TestRepairRemovesADifferingLine(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_remove")
	ctx := poContext(t, database)
	// The stored record differs from the target on the second line's price.
	original := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Original",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "FTZ",
		TaxType: "Goods", UnitPrice: 100, PaymentTermDays: 45,
	})
	target := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Replacement",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "FTZ",
		TaxType: "Goods", UnitPrice: 120, PaymentTermDays: 45,
	})
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-REMOVE")

	input := repairInput(t, saved, admin, target, "drop the priced line")
	keptKey := ""
	before := poRepairHistory(t, database, ctx, saved.ID)
	for _, identity := range before.Sessions[0].Session.LineIdentities {
		if identity.ID != nil && *identity.ID == saved.Items[0].ID {
			keptKey = identity.Key
		}
	}
	if keptKey == "" {
		t.Fatalf("first line has no identity")
	}
	// Keep only the second (zero-value, matching) line and acknowledge the
	// priced source line as missing.
	input.Items = input.Items[1:]
	input.AcknowledgedMissing = []string{"1"}
	input.Decisions = nil
	if _, err := database.RepairPurchaseOrderSource(ctx, input); err != nil {
		t.Fatalf("repair: %v", err)
	}

	history := poRepairHistory(t, database, ctx, saved.ID)
	current := history.Sessions[0]
	item := poRepairFindItem(t, current.Session, "line:"+keptKey+":unitPrice")
	if item.Resolution == nil || *item.Resolution != POReconciliationResolutionRemoved {
		t.Errorf("removed line item resolution = %v; want line_removed", item.Resolution)
	}
	retired := false
	for _, identity := range current.Session.LineIdentities {
		if identity.Key == keptKey && identity.Removed {
			retired = true
		}
	}
	if !retired {
		t.Errorf("the removed row's identity is not marked Removed")
	}
	missing := poRepairFindItem(t, current.Session, "source:1:match")
	if missing.Resolution == nil || *missing.Resolution != POReconciliationResolutionAckMiss {
		t.Errorf("missing source line resolution = %v; want acknowledged_missing", missing.Resolution)
	}
}

// TestRepairSeedsTheFirstGenerationForALegacyRecord covers D1/R07's backend
// half: a pre-041 saved record has no session, and the first real repair seeds
// generation 1 rather than failing on the missing predecessor.
func TestRepairSeedsTheFirstGenerationForALegacyRecord(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_legacy")
	ctx := poContext(t, database)
	staff := poTestUser(t, database)
	admin := poRepairAdmin(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	draft := createFinalizeDraft(t, database, ctx, staff, original)
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DELETE FROM %s.purchase_order_reconciliation_sessions WHERE purchase_order_id = $1`, schema),
		draft.ID); err != nil {
		t.Fatalf("remove the pre-041 session: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.purchase_orders
		SET state = 'saved', po_number = $2, po_number_key = $3,
		    saved_at = NOW(), saved_by = $4
		WHERE id = $1`, schema), draft.ID, "PO-518-B2-LEGACY", "po-518-b2-legacy", staff.ID); err != nil {
		t.Fatalf("mark the legacy record saved: %v", err)
	}
	legacy, err := database.GetPurchaseOrder(ctx, draft.ID)
	if err != nil {
		t.Fatalf("reload legacy record: %v", err)
	}
	if history := poRepairHistory(t, database, ctx, legacy.ID); len(history.Sessions) != 0 {
		t.Fatalf("legacy fixture has %d sessions; want none", len(history.Sessions))
	}

	if _, err := database.RepairPurchaseOrderSource(ctx,
		repairInput(t, legacy, admin, target, "repair the legacy link")); err != nil {
		t.Fatalf("legacy repair: %v", err)
	}
	after := poRepairHistory(t, database, ctx, legacy.ID)
	if len(after.Sessions) != 1 {
		t.Fatalf("legacy repair seeded %d generations; want 1", len(after.Sessions))
	}
	current := after.Sessions[0]
	if current.Generation != 1 || !current.Session.Active || !current.Session.ReadOnly {
		t.Errorf("first generation = %d active=%v readonly=%v; want 1/true/true",
			current.Generation, current.Session.Active, current.Session.ReadOnly)
	}
	if current.RepairCorrectionID == nil {
		t.Errorf("first generation has no repair link")
	}
	if current.Session.SourceQuotationID != target.ID {
		t.Errorf("first generation source = %d; want %d", current.Session.SourceQuotationID, target.ID)
	}
	if len(current.Session.Items) == 0 {
		t.Errorf("first generation has no items")
	}
}

// TestRepairSameVersionRaceHasOneWinnerAndOneGeneration proves the row lock and
// version CAS still let exactly one repair install a generation.
func TestRepairSameVersionRaceHasOneWinnerAndOneGeneration(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_race")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 100, 45)
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-RACE")

	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := database.RepairPurchaseOrderSource(ctx,
				repairInput(t, saved, admin, target, "simultaneous repair"))
			results <- err
		}()
	}
	var wins, conflicts int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrPurchaseOrderVersionConflict):
			conflicts++
		default:
			t.Fatalf("racing repair error = %v; want nil or a version conflict", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("race outcome = %d wins / %d conflicts; want 1/1", wins, conflicts)
	}

	history := poRepairHistory(t, database, ctx, saved.ID)
	if len(history.Sessions) != 2 {
		t.Fatalf("generations after the race = %d; want 2", len(history.Sessions))
	}
	active := 0
	for _, record := range history.Sessions {
		if record.Session.Active {
			active++
		}
	}
	if active != 1 {
		t.Errorf("active generations after the race = %d; want 1", active)
	}
}

// TestRepairCorrectsAMissingSourceMatch covers the case the plan calls out: a
// target line the locked record never claimed becomes the corrected match. The
// missing item is resolved match_corrected, the old claim is acknowledged, and
// the rematched line keeps the pre-correction scalars in its history.
func TestRepairCorrectsAMissingSourceMatch(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_rematch")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotationWith(t, database, repairQuotationOptions{
		CustomerCode: 2, CustomerName: "PT. Buyer", Subject: "Replacement",
		Address: "Jl. Billing A", Currency: "USD", TaxZone: "FTZ",
		TaxType: "Goods", UnitPrice: 100, PaymentTermDays: 45,
		ExtraItemNo: 5, ExtraItemQuantity: 2, ExtraItemPrice: 50,
	})
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-REMATCH")
	if len(saved.Items) < 2 {
		t.Fatalf("fixture has %d lines; want 2", len(saved.Items))
	}
	rematchedRowID := saved.Items[1].ID

	input := repairInput(t, saved, admin, target, "correct the second line's match")
	// The second line claimed reference 3; the target's later issued line is 5.
	line := input.Items[1]
	line.QuotationItemNo = intPointer(5)
	line.QuotationSubNo = nil
	line.UnitPriceMinor = 50
	line.QuantityScaled = 2 * QuantityScale
	line.Description = "Later issued line"
	line.UoM = "Pc"
	input.Items[1] = line
	input.AcknowledgedMissing = []string{"3"}
	input.AcknowledgedExtra = nil
	input.Decisions = []PurchaseOrderDecision{
		{Key: "line:2:unitPrice", Choice: PurchaseOrderChoiceQuotation},
		{Key: "line:2:quantity", Choice: PurchaseOrderChoiceQuotation},
		{Key: "line:2:description", Choice: PurchaseOrderChoiceQuotation},
		{Key: "line:2:uom", Choice: PurchaseOrderChoiceQuotation},
	}
	if _, err := database.RepairPurchaseOrderSource(ctx, input); err != nil {
		t.Fatalf("repair: %v", err)
	}

	history := poRepairHistory(t, database, ctx, saved.ID)
	current := history.Sessions[0]

	// The target line the stored record never claimed is now corrected, not
	// merely acknowledged.
	corrected := poRepairFindItem(t, current.Session, "source:5:match")
	if corrected.Kind != POReconciliationKindMissingLine {
		t.Errorf("corrected match kind = %q; want missing_line", corrected.Kind)
	}
	if corrected.Resolution == nil || *corrected.Resolution != POReconciliationResolutionCorrected {
		t.Errorf("corrected match resolution = %v; want match_corrected", corrected.Resolution)
	}
	var correctedRef string
	if err := json.Unmarshal(corrected.SourceValue, &correctedRef); err != nil || correctedRef != "5" {
		t.Errorf("corrected match source value = %s; want \"5\"", corrected.SourceValue)
	}
	if len(corrected.History) < 2 {
		t.Errorf("corrected match history has %d events; want the missing observation and the correction",
			len(corrected.History))
	}

	// The previously claimed reference is acknowledged, so the existing path is
	// not weakened.
	acknowledged := poRepairFindItem(t, current.Session, "source:3:match")
	if acknowledged.Resolution == nil || *acknowledged.Resolution != POReconciliationResolutionAckMiss {
		t.Errorf("the released reference resolution = %v; want acknowledged_missing", acknowledged.Resolution)
	}

	// The rematched row keeps its identity and the scalars it was compared with.
	var lineKey string
	for _, identity := range current.Session.LineIdentities {
		if identity.ID != nil && *identity.ID == rematchedRowID && !identity.Removed {
			lineKey = identity.Key
		}
	}
	if lineKey == "" {
		t.Fatalf("the rematched row has no surviving identity: %v", current.Session.LineIdentities)
	}
	quantity := poRepairFindItem(t, current.Session, "line:"+lineKey+":quantity")
	if quantity.Resolution == nil || *quantity.Resolution != POReconciliationResolutionUsedQuote {
		t.Errorf("rematched quantity resolution = %v; want used_quotation", quantity.Resolution)
	}
	var compared, applied int64
	if err := json.Unmarshal(quantity.History[0].POValue, &compared); err != nil || compared != 1*QuantityScale {
		t.Errorf("pre-correction quantity = %s; want the stored %d", quantity.History[0].POValue, 1*QuantityScale)
	}
	if err := json.Unmarshal(quantity.POValue, &applied); err != nil || applied != 2*QuantityScale {
		t.Errorf("applied quantity = %s; want %d", quantity.POValue, 2*QuantityScale)
	}
	price := poRepairFindItem(t, current.Session, "line:"+lineKey+":unitPrice")
	var comparedPrice int64
	if err := json.Unmarshal(price.History[0].POValue, &comparedPrice); err != nil || comparedPrice != 0 {
		t.Errorf("pre-correction price = %s; want the stored 0", price.History[0].POValue)
	}
}

// poRepairFingerprint digests every row a repair can touch, so a rolled-back
// failure can be compared byte for byte against the state before it.
func poRepairFingerprint(t *testing.T, ctx context.Context, database *Store, poID int64) map[string]string {
	t.Helper()
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()
	queries := map[string]string{
		"purchase_orders": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.id), ''))
			FROM %s.purchase_orders t WHERE t.id = $1`,
		"purchase_order_items": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.id), ''))
			FROM %s.purchase_order_items t WHERE t.purchase_order_id = $1`,
		"purchase_order_line_aliases": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.client_key), ''))
			FROM %s.purchase_order_line_aliases t WHERE t.purchase_order_id = $1`,
		"purchase_order_corrections": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.id), ''))
			FROM %s.purchase_order_corrections t WHERE t.purchase_order_id = $1`,
		"purchase_order_correction_items": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.correction_id, t.line_number), ''))
			FROM %s.purchase_order_correction_items t
			WHERE t.correction_id IN (
				SELECT id FROM %s.purchase_order_corrections WHERE purchase_order_id = $1)`,
		"purchase_order_source_repairs": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.correction_id), ''))
			FROM %s.purchase_order_source_repairs t
			WHERE t.correction_id IN (
				SELECT id FROM %s.purchase_order_corrections WHERE purchase_order_id = $1)`,
		"purchase_order_source_repair_decisions": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.correction_id, t.key), ''))
			FROM %s.purchase_order_source_repair_decisions t
			WHERE t.correction_id IN (
				SELECT id FROM %s.purchase_order_corrections WHERE purchase_order_id = $1)`,
		"purchase_order_reconciliation_sessions": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.id), ''))
			FROM %s.purchase_order_reconciliation_sessions t WHERE t.purchase_order_id = $1`,
		"purchase_order_reconciliation_items": `
			SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t.session_id, t.key), ''))
			FROM %s.purchase_order_reconciliation_items t WHERE t.purchase_order_id = $1`,
	}
	fingerprint := make(map[string]string, len(queries))
	for name, query := range queries {
		// Some queries name the schema twice.
		placeholders := strings.Count(query, "%s")
		var digest string
		if err := pool.QueryRow(ctx, fmt.Sprintf(query, repeatString(schema, placeholders)...), poID).Scan(&digest); err != nil {
			t.Fatalf("fingerprint %s: %v", name, err)
		}
		fingerprint[name] = digest
	}
	return fingerprint
}

func repeatString(value string, count int) []any {
	values := make([]any, count)
	for index := range values {
		values[index] = value
	}
	return values
}

// TestRepairLateFailureRollsBackEverything forces a failure after the PO update
// and the line/alias write but before commit. The PO, correction snapshot,
// satellite, lines, alias claims, deactivation and the absent new session must
// all be byte-identical afterwards, and the same repair must then succeed.
func TestRepairLateFailureRollsBackEverything(t *testing.T) {
	database := openTestSchema(t, "vera_po_518_b2_latefail")
	ctx := poContext(t, database)
	original := seedRepairQuotation(t, database, 2, "PT. Buyer", "Original", "Jl. Billing A", 100, 45)
	target := seedRepairQuotation(t, database, 2, "PT. Buyer", "Replacement", "Jl. Billing A", 120, 45)
	saved, _, admin := poRepairSavedOrder(t, database, ctx, original, "PO-518-B2-LATE")
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	schema := database.configSchema()

	before := poRepairFingerprint(t, ctx, database, saved.ID)

	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s.vera_test_fail_repair_session()
		RETURNS trigger LANGUAGE plpgsql AS $fault$
		BEGIN
			RAISE EXCEPTION 'test-only late repair failure' USING ERRCODE = '23514';
		END;
		$fault$`, schema)); err != nil {
		t.Fatalf("create fault function: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER vera_test_fail_repair_session_trigger
		BEFORE INSERT ON %s.purchase_order_reconciliation_sessions
		FOR EACH ROW EXECUTE FUNCTION %s.vera_test_fail_repair_session()`, schema, schema)); err != nil {
		t.Fatalf("create fault trigger: %v", err)
	}

	input := repairInput(t, saved, admin, target, "late failure")
	input.Items[0].UnitPriceMinor = 120
	input.Decisions = []PurchaseOrderDecision{{Key: "line:1:unitPrice", Choice: PurchaseOrderChoiceQuotation}}
	if _, err := database.RepairPurchaseOrderSource(ctx, input); err == nil {
		t.Fatalf("the forced late failure did not abort the repair")
	} else if !strings.Contains(err.Error(), "test-only late repair failure") {
		t.Fatalf("repair failed for the wrong reason: %v", err)
	}

	after := poRepairFingerprint(t, ctx, database, saved.ID)
	for table, digest := range before {
		if after[table] != digest {
			t.Errorf("%s changed across the rolled-back repair: %s -> %s", table, digest, after[table])
		}
	}

	// Remove the fault and prove the transaction left nothing behind.
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DROP TRIGGER vera_test_fail_repair_session_trigger ON %s.purchase_order_reconciliation_sessions`, schema)); err != nil {
		t.Fatalf("drop fault trigger: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(
		`DROP FUNCTION %s.vera_test_fail_repair_session()`, schema)); err != nil {
		t.Fatalf("drop fault function: %v", err)
	}
	if _, err := database.RepairPurchaseOrderSource(ctx, input); err != nil {
		t.Fatalf("retry after the rolled-back failure: %v", err)
	}
	history := poRepairHistory(t, database, ctx, saved.ID)
	if len(history.Sessions) != 2 {
		t.Fatalf("generations after the retry = %d; want 2", len(history.Sessions))
	}
	if !history.Sessions[0].Session.Active || history.Sessions[1].Session.Active {
		t.Errorf("active generation after the retry is wrong: %v", history.Sessions)
	}
}
