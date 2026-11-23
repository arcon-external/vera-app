package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Issue #654, wave 2: the aggregate group lives inside the ordinary purchase
// order write paths. These tests drive the real store against a disposable
// schema: the draft save, the final save, a correction, a source repair, the
// legacy-client refusal, the null-reference child seam and the report proofs.
//
// Group ids are not stable across saves, so every assertion reads the group the
// store returned instead of naming an id it wrote earlier.

// aggregateSaveFixture is one real draft from the standard seeded revision: a
// plain line (depth 1) and one split package of two sub-items (depths 2 and 3),
// so mixed-depth grouping is exercised by the ordinary fixture data.
type aggregateSaveFixture struct {
	database *Store
	ctx      context.Context
	user     AuthUser
	order    PurchaseOrder
}

func newAggregateSaveFixture(t *testing.T, schemaName string) aggregateSaveFixture {
	t.Helper()
	database := openTestSchema(t, schemaName)
	quotation := seedPOQuotation(t, database, "PT. Aggregate Save", "Aggregate save")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	order, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create aggregate save draft: %v", err)
	}
	if len(order.Items) != 3 {
		t.Fatalf("fixture lines = %d; want the plain line and two package children", len(order.Items))
	}
	return aggregateSaveFixture{database: database, ctx: ctx, user: user, order: order}
}

// draftUpdate is the ordinary draft save payload: every editable value as
// stored, the submitted line set, and the submitted groups. Tests change only
// the field they exercise.
func draftUpdate(order PurchaseOrder, user AuthUser, items []PurchaseOrderItem, groups []PurchaseOrderAggregateGroupInput) PurchaseOrderUpdate {
	return PurchaseOrderUpdate{
		ID:                    order.ID,
		BillingAddress:        order.BillingAddress,
		DeliveryAddress:       order.DeliveryAddress,
		ShipToCode:            order.ShipToCode,
		ContactEmail:          order.ContactEmail,
		ContactPhone:          order.ContactPhone,
		PONumber:              order.PONumber,
		PODate:                order.PODate,
		ReceivedDate:          &order.ReceivedDate,
		RequestedDeliveryDate: order.RequestedDeliveryDate,
		PaymentTerms:          order.PaymentTerms,
		Incoterms:             order.Incoterms,
		BusinessCategory:      order.BusinessCategory,
		UpdatedBy:             user.ID,
		ExpectedVersion:       order.Version,
		Items:                 items,
		DiscountPercent:       order.DiscountPercent,
		AdjustmentMinor:       order.AdjustmentMinor,
		AdjustmentReason:      order.AdjustmentReason,
		AggregateGroups:       groups,
	}
}

// indexedItems copies the stored lines and appends the submitted group index to
// each one in order, so a test can express "these lines are one group" without
// repeating the whole line payload.
func indexedItems(items []PurchaseOrderItem, indexes ...int) []PurchaseOrderItem {
	copied := append([]PurchaseOrderItem{}, items...)
	for index := range copied {
		copied[index].AggregateGroupIndex = 0
		if index < len(indexes) {
			copied[index].AggregateGroupIndex = indexes[index]
		}
	}
	return copied
}

// childAllocationCount is how many allocation rows one stored line owns. A
// grouped child keeps its own allocation; the header owns none.
func childAllocationCount(t *testing.T, database *Store, ctx context.Context, itemID int64) int {
	t.Helper()
	var count int
	if err := database.pool.QueryRow(ctx, `
		SELECT count(*) FROM `+database.configSchema()+`.purchase_order_allocations
		WHERE purchase_order_item_id = $1`, itemID).Scan(&count); err != nil {
		t.Fatalf("count allocations of line %d: %v", itemID, err)
	}
	return count
}

// TestPurchaseOrderDraftSaveRefusesALegacyNilGroupSubmission pins the
// compatibility contract at the store boundary: a client that cannot express
// groups (a nil slice) may still save a record that has none, but it must never
// silently ungroup a record saved by a newer client. The empty non-nil slice is
// the deliberate ungroup, and it keeps working.
func TestPurchaseOrderDraftSaveRefusesALegacyNilGroupSubmission(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_legacy")
	grouped, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, indexedItems(fixture.order.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Package", QuantityScale, "Lot")}))
	if err != nil {
		t.Fatalf("group the draft: %v", err)
	}
	if len(grouped.AggregateGroups) != 1 {
		t.Fatalf("groups before the legacy save = %d; want 1", len(grouped.AggregateGroups))
	}
	storedGroupID := grouped.AggregateGroups[0].ID

	// A legacy client cannot express groups. It also carries no persisted ids
	// for a line it edited, which is the ordinary older-editor payload: the
	// value match fails and the row is replaced, so the membership it cannot
	// see is lost. A changed number makes a silent header write visible too.
	legacyItems := indexedItems(grouped.Items)
	for index := range legacyItems {
		legacyItems[index].ID = 0
	}
	legacyItems[0].Description = "Plain widget (legacy edit)"
	legacy := draftUpdate(grouped, fixture.user, legacyItems, nil)
	legacy.PONumber = "PO-LEGACY"
	refused, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx, legacy)
	if !errors.Is(err, ErrPurchaseOrderClientOutdated) {
		after, readErr := fixture.database.GetPurchaseOrder(fixture.ctx, fixture.order.ID)
		if readErr != nil {
			t.Fatalf("read after the legacy attempt: %v", readErr)
		}
		members := 0
		for _, item := range after.Items {
			if item.AggregateGroupID == storedGroupID {
				members++
			}
		}
		t.Fatalf("legacy nil save on a grouped record = %v (returned version %d, %d lines); after the attempt: version %d, %d group(s), %d of %d children, po number %q; want the typed refusal and the untouched record",
			err, refused.Version, len(refused.Items), after.Version,
			len(after.AggregateGroups), members, len(grouped.Items), after.PONumber)
	}
	if !strings.Contains(err.Error(), "newer Vera") {
		t.Errorf("refusal %q does not name the update-Vera remedy", err.Error())
	}

	reopened, err := fixture.database.GetPurchaseOrder(fixture.ctx, fixture.order.ID)
	if err != nil {
		t.Fatalf("reopen after the refusal: %v", err)
	}
	if reopened.PONumber == "PO-LEGACY" || reopened.Version != grouped.Version {
		t.Errorf("a refused legacy save wrote the record: po number %q version %d; want %q version %d",
			reopened.PONumber, reopened.Version, grouped.PONumber, grouped.Version)
	}
	if len(reopened.AggregateGroups) != 1 || reopened.AggregateGroups[0].ID != storedGroupID {
		t.Fatalf("groups after the refusal = %+v; want the untouched group %d",
			reopened.AggregateGroups, storedGroupID)
	}
	for _, item := range reopened.Items {
		if item.AggregateGroupID != storedGroupID {
			t.Errorf("line %d membership after the refusal = %d; want %d",
				item.LineNumber, item.AggregateGroupID, storedGroupID)
		}
	}

	// The deliberate ungroup is an empty non-nil slice, and after it a legacy
	// client can save the record again: the window still covers every record an
	// old client could have created.
	ungrouped, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(reopened, fixture.user, indexedItems(reopened.Items),
			[]PurchaseOrderAggregateGroupInput{}))
	if err != nil {
		t.Fatalf("deliberate ungroup: %v", err)
	}
	if len(ungrouped.AggregateGroups) != 0 {
		t.Fatalf("groups after the deliberate ungroup = %d; want 0", len(ungrouped.AggregateGroups))
	}
	if _, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(ungrouped, fixture.user, indexedItems(ungrouped.Items), nil)); err != nil {
		t.Fatalf("legacy nil save on an ungrouped record = %v; want nil", err)
	}
}

// TestPurchaseOrderDraftSaveRefusesInvalidAggregatePlacementWithoutWriting
// drives every malformed placement through the real save: an empty group, a
// gap, half a split package, a bad index, a blank header, a zero display
// quantity and a client that names groups but submits none. Each is refused
// with the typed error, and the stored record keeps its version, its number
// and its lines with no group at all: the refusal must not half-apply a header
// or a membership.
func TestPurchaseOrderDraftSaveRefusesInvalidAggregatePlacementWithoutWriting(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_refusals")
	group := itemGroupInput("Package", QuantityScale, "Lot")

	cases := []struct {
		name     string
		groups   []PurchaseOrderAggregateGroupInput
		indexes  []int
		messages []string
		// plain submits free lines with no split display group, so the
		// contiguity and index rules are not shadowed by the package rule.
		plain bool
	}{
		{
			name:     "group without a child",
			groups:   []PurchaseOrderAggregateGroupInput{group},
			indexes:  []int{0, 0, 0},
			messages: []string{"group 1", "no child"},
		},
		{
			name:     "non-contiguous children",
			groups:   []PurchaseOrderAggregateGroupInput{group},
			indexes:  []int{1, 0, 1},
			messages: []string{"group 1", "contiguous"},
			plain:    true,
		},
		{
			name:     "half a split package",
			groups:   []PurchaseOrderAggregateGroupInput{group},
			indexes:  []int{0, 1, 0},
			messages: []string{"line 3", "splits display group"},
		},
		{
			name:     "index past the submitted groups",
			groups:   []PurchaseOrderAggregateGroupInput{group},
			indexes:  []int{1, 0, 2},
			messages: []string{"line 3", "group 2"},
			plain:    true,
		},
		{
			name:     "blank title",
			groups:   []PurchaseOrderAggregateGroupInput{itemGroupInput("   ", QuantityScale, "Lot")},
			indexes:  []int{1, 1, 1},
			messages: []string{"group 1", "title"},
		},
		{
			name:     "zero display quantity",
			groups:   []PurchaseOrderAggregateGroupInput{itemGroupInput("Package", 0, "Lot")},
			indexes:  []int{1, 1, 1},
			messages: []string{"group 1", "display quantity"},
		},
		{
			name:     "indexed lines with no groups at all",
			groups:   nil,
			indexes:  []int{0, 0, 1},
			messages: []string{"line 3", "no groups"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := indexedItems(fixture.order.Items, tc.indexes...)
			if tc.plain {
				items = plainIndexedItems(t, fixture, tc.indexes...)
			}
			input := draftUpdate(fixture.order, fixture.user, items, tc.groups)
			input.PONumber = "PO-REFUSED"
			if _, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx, input); !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
				t.Fatalf("invalid placement = %v; want ErrPurchaseOrderAggregateInvalid", err)
			} else {
				for _, message := range tc.messages {
					if !strings.Contains(err.Error(), message) {
						t.Errorf("error %q does not name %q", err.Error(), message)
					}
				}
			}

			stored, err := fixture.database.GetPurchaseOrder(fixture.ctx, fixture.order.ID)
			if err != nil {
				t.Fatalf("reopen after the refusal: %v", err)
			}
			if stored.Version != fixture.order.Version || stored.PONumber != fixture.order.PONumber {
				t.Errorf("a refused placement wrote the header: version %d number %q; want %d %q",
					stored.Version, stored.PONumber, fixture.order.Version, fixture.order.PONumber)
			}
			if len(stored.AggregateGroups) != 0 {
				t.Errorf("a refused placement stored %d groups; want 0", len(stored.AggregateGroups))
			}
			if len(stored.Items) != len(fixture.order.Items) {
				t.Errorf("a refused placement changed the line count: %d; want %d",
					len(stored.Items), len(fixture.order.Items))
			}
			for _, item := range stored.Items {
				if item.AggregateGroupID != 0 {
					t.Errorf("a refused placement attached line %d to group %d; want ungrouped",
						item.LineNumber, item.AggregateGroupID)
				}
			}
		})
	}
}

// plainIndexedItems builds a submitted line set with no quotation reference and
// no split display group, so the contiguity and index rules can be exercised
// without the package rule firing first. The lines are not saved: they are only
// the payload of a refused write.
func plainIndexedItems(t *testing.T, fixture aggregateSaveFixture, indexes ...int) []PurchaseOrderItem {
	t.Helper()
	base := fixture.order.Items[:len(indexes)]
	items := make([]PurchaseOrderItem, len(indexes))
	for index := range indexes {
		items[index] = PurchaseOrderItem{
			Description:         base[index].Description,
			QuantityScaled:      QuantityScale,
			UoM:                 "Pc",
			UnitPriceMinor:      1000,
			AggregateGroupIndex: indexes[index],
		}
	}
	return items
}

// aggregateInputsFromStored converts the read shape back to a submitted group
// set, which is what every later write path sends: the ids are internal and are
// never resubmitted.
func aggregateInputsFromStored(groups []PurchaseOrderAggregateGroup) []PurchaseOrderAggregateGroupInput {
	inputs := make([]PurchaseOrderAggregateGroupInput, len(groups))
	for index, group := range groups {
		inputs[index] = PurchaseOrderAggregateGroupInput{
			Title:                  group.Title,
			OrderedQuantityScaled:  group.OrderedQuantityScaled,
			OrderedUoM:             group.OrderedUoM,
			ShowChildrenToCustomer: group.ShowChildrenToCustomer,
		}
	}
	return inputs
}

// TestFinalizePurchaseOrderKeepsGroupsAndChildAllocations proves the final save
// writes the submitted groups, leaves every child's own allocation in place and
// does not move the record's money. A grouped draft finalized by a client that
// cannot express groups is refused and stays a draft.
func TestFinalizePurchaseOrderKeepsGroupsAndChildAllocations(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_finalize")
	grouped, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, indexedItems(fixture.order.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Draft package", 3*QuantityScale, "Set")}))
	if err != nil {
		t.Fatalf("group the draft: %v", err)
	}

	// A legacy final save cannot represent the groups: it is refused before the
	// state transition and the draft keeps them.
	legacy := finalizeInput(grouped, fixture.user, "PO-654-LEGACY")
	legacy.Items = indexedItems(grouped.Items)
	if _, err := fixture.database.FinalizePurchaseOrder(fixture.ctx, legacy); !errors.Is(err, ErrPurchaseOrderClientOutdated) {
		t.Fatalf("legacy final save = %v; want ErrPurchaseOrderClientOutdated", err)
	}
	stillDraft, err := fixture.database.GetPurchaseOrder(fixture.ctx, fixture.order.ID)
	if err != nil {
		t.Fatalf("reopen after the refused final save: %v", err)
	}
	if stillDraft.State != PurchaseOrderStateDraft || stillDraft.Version != grouped.Version {
		t.Errorf("refused final save = state %q version %d; want the untouched draft %d",
			stillDraft.State, stillDraft.Version, grouped.Version)
	}
	if len(stillDraft.AggregateGroups) != 1 {
		t.Fatalf("groups after the refused final save = %d; want 1", len(stillDraft.AggregateGroups))
	}

	// The final save carries the groups: a renamed title and the toggle are
	// written, every child keeps its allocation, and the money is unchanged.
	finalize := finalizeInput(grouped, fixture.user, "PO-654-FINAL")
	finalize.Items = indexedItems(grouped.Items, 1, 1, 1)
	finalize.AggregateGroups = []PurchaseOrderAggregateGroupInput{{
		Title:                  "Final package",
		OrderedQuantityScaled:  3 * QuantityScale,
		OrderedUoM:             "Set",
		ShowChildrenToCustomer: true,
	}}
	saved, err := fixture.database.FinalizePurchaseOrder(fixture.ctx, finalize)
	if err != nil {
		t.Fatalf("final save with groups: %v", err)
	}
	if saved.State != PurchaseOrderStateSaved {
		t.Fatalf("state after the final save = %q; want saved", saved.State)
	}
	if len(saved.AggregateGroups) != 1 {
		t.Fatalf("groups after the final save = %d; want 1", len(saved.AggregateGroups))
	}
	stored := saved.AggregateGroups[0]
	if stored.Title != "Final package" || stored.OrderedUoM != "Set" || !stored.ShowChildrenToCustomer {
		t.Errorf("stored group = %+v; want the submitted title, unit and toggle", stored)
	}
	var wantTotal int64
	for _, item := range saved.Items {
		wantTotal += item.NetMinor
		if item.AggregateGroupID != stored.ID {
			t.Errorf("line %d membership = %d; want %d", item.LineNumber, item.AggregateGroupID, stored.ID)
		}
		if count := childAllocationCount(t, fixture.database, fixture.ctx, item.ID); count != 1 {
			t.Errorf("line %d allocations = %d; want each child to keep its own",
				item.LineNumber, count)
		}
	}
	if stored.TotalMinor != wantTotal {
		t.Errorf("group total = %d; want the children's %d", stored.TotalMinor, wantTotal)
	}
	if saved.SubtotalMinor != grouped.SubtotalMinor ||
		saved.PpnMinor != grouped.PpnMinor ||
		saved.GrandTotalMinor != grouped.GrandTotalMinor {
		t.Errorf("final-save money = %d/%d/%d; want the unchanged %d/%d/%d",
			saved.SubtotalMinor, saved.PpnMinor, saved.GrandTotalMinor,
			grouped.SubtotalMinor, grouped.PpnMinor, grouped.GrandTotalMinor)
	}

	// The review reads the same groups through the ordinary read path.
	review, err := fixture.database.GetPurchaseOrderReview(fixture.ctx, fixture.order.ID)
	if err != nil {
		t.Fatalf("review the saved record: %v", err)
	}
	if len(review.Order.AggregateGroups) != 1 || review.Order.AggregateGroups[0].ID != stored.ID {
		t.Errorf("review groups = %+v; want the saved group %d", review.Order.AggregateGroups, stored.ID)
	}
}

// TestPurchaseOrderCorrectionSnapshotsTheReplacedGroupLayout regroups a saved
// record and proves the history keeps the layout it replaced: the old group's
// title, display order and toggle, and each snapshot line's group order. A
// legacy correction that cannot express groups is refused before the snapshot.
func TestPurchaseOrderCorrectionSnapshotsTheReplacedGroupLayout(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_correction")
	grouped, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, indexedItems(fixture.order.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Original package", 3*QuantityScale, "Set")}))
	if err != nil {
		t.Fatalf("group the draft: %v", err)
	}
	finalize := finalizeInput(grouped, fixture.user, "PO-654-CORR")
	finalize.Items = indexedItems(grouped.Items, 1, 1, 1)
	finalize.AggregateGroups = aggregateInputsFromStored(grouped.AggregateGroups)
	saved, err := fixture.database.FinalizePurchaseOrder(fixture.ctx, finalize)
	if err != nil {
		t.Fatalf("finalize the grouped draft: %v", err)
	}

	// A legacy correction names no groups: refused before the snapshot, so the
	// record and its history are exactly as the final save left them.
	legacy := correctionInput(saved, fixture.user, "legacy correction", saved.Version)
	legacy.Items = indexedItems(saved.Items)
	if _, err := fixture.database.CorrectPurchaseOrder(fixture.ctx, legacy); !errors.Is(err, ErrPurchaseOrderClientOutdated) {
		t.Fatalf("legacy correction = %v; want ErrPurchaseOrderClientOutdated", err)
	}
	if history, err := fixture.database.GetPurchaseOrderHistory(fixture.ctx, saved.ID); err != nil || len(history) != 0 {
		t.Fatalf("history after the refused correction = %v, %v; want none", history, err)
	}

	// The real correction splits the record into two groups: the plain line
	// alone, then the package's two children. Both are renamed and toggled.
	correction := correctionInput(saved, fixture.user, "split the customer lines", saved.Version)
	correction.Items = indexedItems(saved.Items, 1, 2, 2)
	correction.AggregateGroups = []PurchaseOrderAggregateGroupInput{
		{Title: "Widget line", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Pc"},
		{Title: "Package rows", OrderedQuantityScaled: 2 * QuantityScale, OrderedUoM: "Set", ShowChildrenToCustomer: true},
	}
	corrected, err := fixture.database.CorrectPurchaseOrder(fixture.ctx, correction)
	if err != nil {
		t.Fatalf("correct the grouped record: %v", err)
	}
	if len(corrected.AggregateGroups) != 2 {
		t.Fatalf("live groups after the correction = %d; want 2", len(corrected.AggregateGroups))
	}
	if corrected.AggregateGroups[0].Title != "Widget line" ||
		corrected.AggregateGroups[1].Title != "Package rows" ||
		!corrected.AggregateGroups[1].ShowChildrenToCustomer {
		t.Errorf("live groups = %+v; want the corrected titles and toggle", corrected.AggregateGroups)
	}
	if corrected.AggregateGroups[0].DisplayOrder != 1 || corrected.AggregateGroups[1].DisplayOrder != 2 {
		t.Errorf("live display orders = %d/%d; want 1/2",
			corrected.AggregateGroups[0].DisplayOrder, corrected.AggregateGroups[1].DisplayOrder)
	}
	for _, item := range corrected.Items {
		wantGroup := corrected.AggregateGroups[0].ID
		if item.LineNumber > 1 {
			wantGroup = corrected.AggregateGroups[1].ID
		}
		if item.AggregateGroupID != wantGroup {
			t.Errorf("live line %d membership = %d; want %d", item.LineNumber, item.AggregateGroupID, wantGroup)
		}
	}

	history, err := fixture.database.GetPurchaseOrderHistory(fixture.ctx, saved.ID)
	if err != nil {
		t.Fatalf("history after the correction: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history))
	}
	entry := history[0]
	if len(entry.Groups) != 1 {
		t.Fatalf("snapshot groups = %d; want the one replaced group", len(entry.Groups))
	}
	snapshot := entry.Groups[0]
	if snapshot.DisplayOrder != 1 || snapshot.Title != "Original package" ||
		snapshot.OrderedQuantityScaled != 3*QuantityScale || snapshot.OrderedUoM != "Set" ||
		snapshot.ShowChildrenToCustomer {
		t.Errorf("snapshot group = %+v; want the replaced title/unit/quantity and the false toggle", snapshot)
	}
	if len(entry.Items) != len(saved.Items) {
		t.Fatalf("snapshot lines = %d; want %d", len(entry.Items), len(saved.Items))
	}
	for _, item := range entry.Items {
		if item.AggregateGroupOrder != 1 {
			t.Errorf("snapshot line %d group order = %d; want 1 (the replaced group)",
				item.LineNumber, item.AggregateGroupOrder)
		}
	}
}

// TestPurchaseOrderCorrectionCanUngroupAndHistoryKeepsTheOldLayout pins the
// ungroup correction: the live record drops both groups, and the history still
// says what the layout was. Group ids are never stable, so the snapshot is read
// by display order.
func TestPurchaseOrderCorrectionCanUngroupAndHistoryKeepsTheOldLayout(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_ungroup_correction")
	grouped, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, indexedItems(fixture.order.Items, 1, 2, 2),
			[]PurchaseOrderAggregateGroupInput{
				{Title: "Widget line", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Pc"},
				{Title: "Package rows", OrderedQuantityScaled: 2 * QuantityScale, OrderedUoM: "Set"},
			}))
	if err != nil {
		t.Fatalf("group the draft: %v", err)
	}
	finalize := finalizeInput(grouped, fixture.user, "PO-654-UNGROUP")
	finalize.Items = indexedItems(grouped.Items, 1, 2, 2)
	finalize.AggregateGroups = aggregateInputsFromStored(grouped.AggregateGroups)
	saved, err := fixture.database.FinalizePurchaseOrder(fixture.ctx, finalize)
	if err != nil {
		t.Fatalf("finalize the grouped draft: %v", err)
	}

	ungroup := correctionInput(saved, fixture.user, "customer wants the flat list", saved.Version)
	ungroup.Items = indexedItems(saved.Items)
	ungroup.AggregateGroups = []PurchaseOrderAggregateGroupInput{}
	corrected, err := fixture.database.CorrectPurchaseOrder(fixture.ctx, ungroup)
	if err != nil {
		t.Fatalf("ungroup correction: %v", err)
	}
	if len(corrected.AggregateGroups) != 0 {
		t.Fatalf("live groups after the ungroup = %d; want 0", len(corrected.AggregateGroups))
	}
	for _, item := range corrected.Items {
		if item.AggregateGroupID != 0 {
			t.Errorf("live line %d membership after the ungroup = %d; want 0",
				item.LineNumber, item.AggregateGroupID)
		}
	}

	history, err := fixture.database.GetPurchaseOrderHistory(fixture.ctx, saved.ID)
	if err != nil {
		t.Fatalf("history after the ungroup: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want 1", len(history))
	}
	if len(history[0].Groups) != 2 {
		t.Fatalf("snapshot groups = %d; want the two replaced groups", len(history[0].Groups))
	}
	if history[0].Groups[0].DisplayOrder != 1 || history[0].Groups[1].DisplayOrder != 2 {
		t.Errorf("snapshot display orders = %d/%d; want 1/2",
			history[0].Groups[0].DisplayOrder, history[0].Groups[1].DisplayOrder)
	}
	for _, item := range history[0].Items {
		wantOrder := 1
		if item.LineNumber > 1 {
			wantOrder = 2
		}
		if item.AggregateGroupOrder != wantOrder {
			t.Errorf("snapshot line %d group order = %d; want %d",
				item.LineNumber, item.AggregateGroupOrder, wantOrder)
		}
	}
}

// TestPurchaseOrderOverwriteSnapshotsCarryGroups proves both overwrite kinds
// keep the group layout they replaced: the explicit draft overwrite and a final
// save that overwrote a draft. The live record moves on to the submitted
// layout; the history keeps the replaced one.
func TestPurchaseOrderOverwriteSnapshotsCarryGroups(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_overwrite")
	first, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, indexedItems(fixture.order.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("First package", QuantityScale, "Lot")}))
	if err != nil {
		t.Fatalf("first grouped save: %v", err)
	}
	second, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(first, fixture.user, indexedItems(first.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Second package", 2*QuantityScale, "Set")}))
	if err != nil {
		t.Fatalf("second grouped save: %v", err)
	}

	overwrite := draftUpdate(second, fixture.user, indexedItems(second.Items, 1, 1, 1),
		[]PurchaseOrderAggregateGroupInput{itemGroupInput("Third package", 3*QuantityScale, "Lot")})
	overwrite.Overwrite = true
	third, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx, overwrite)
	if err != nil {
		t.Fatalf("overwrite save: %v", err)
	}
	if len(third.AggregateGroups) != 1 || third.AggregateGroups[0].Title != "Third package" {
		t.Fatalf("live groups after the overwrite = %+v; want the submitted Third package",
			third.AggregateGroups)
	}
	history, err := fixture.database.GetPurchaseOrderHistory(fixture.ctx, fixture.order.ID)
	if err != nil {
		t.Fatalf("history after the draft overwrite: %v", err)
	}
	if len(history) != 1 || history[0].Kind != PurchaseOrderCorrectionKindDraftOverwrite {
		t.Fatalf("history = %+v; want one draft-overwrite snapshot", history)
	}
	if len(history[0].Groups) != 1 || history[0].Groups[0].Title != "Second package" ||
		history[0].Groups[0].OrderedUoM != "Set" {
		t.Fatalf("draft-overwrite snapshot groups = %+v; want the replaced Second package",
			history[0].Groups)
	}
	for _, item := range history[0].Items {
		if item.AggregateGroupOrder != 1 {
			t.Errorf("draft-overwrite snapshot line %d group order = %d; want 1",
				item.LineNumber, item.AggregateGroupOrder)
		}
	}

	fourth, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(third, fixture.user, indexedItems(third.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Fourth package", QuantityScale, "Lot")}))
	if err != nil {
		t.Fatalf("fourth grouped save: %v", err)
	}
	finalize := finalizeInput(fourth, fixture.user, "PO-654-OVERWRITE")
	finalize.Items = indexedItems(fourth.Items, 1, 1, 1)
	finalize.AggregateGroups = []PurchaseOrderAggregateGroupInput{
		{Title: "Final package", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"},
	}
	finalize.Overwrite = true
	saved, err := fixture.database.FinalizePurchaseOrder(fixture.ctx, finalize)
	if err != nil {
		t.Fatalf("final save over a grouped draft: %v", err)
	}
	if len(saved.AggregateGroups) != 1 || saved.AggregateGroups[0].Title != "Final package" {
		t.Fatalf("live groups after the final save = %+v; want the submitted Final package",
			saved.AggregateGroups)
	}
	history, err = fixture.database.GetPurchaseOrderHistory(fixture.ctx, fixture.order.ID)
	if err != nil {
		t.Fatalf("history after the final overwrite: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history entries = %d; want the two overwrite snapshots", len(history))
	}
	newest := history[0]
	if newest.Kind != PurchaseOrderCorrectionKindDraftOverwrite {
		t.Errorf("newest history kind = %q; want %q", newest.Kind, PurchaseOrderCorrectionKindDraftOverwrite)
	}
	if len(newest.Groups) != 1 || newest.Groups[0].Title != "Fourth package" {
		t.Errorf("final-overwrite snapshot groups = %+v; want the replaced Fourth package", newest.Groups)
	}
	for _, item := range newest.Items {
		if item.AggregateGroupOrder != 1 {
			t.Errorf("final-overwrite snapshot line %d group order = %d; want 1",
				item.LineNumber, item.AggregateGroupOrder)
		}
	}
}

// TestRepairPurchaseOrderSourcePreservesGroups moves a grouped saved record to
// another revision and proves the repair carries the aggregate layout with it:
// the live groups, each child's membership and allocation, and the snapshot of
// the replaced record. A repair that cannot express groups is refused before
// anything is written.
func TestRepairPurchaseOrderSourcePreservesGroups(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_repair")
	target := seedPOQuotation(t, fixture.database, "PT. Aggregate Save", "Aggregate replacement")
	admin := poRepairAdmin(t, fixture.database)

	grouped, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, indexedItems(fixture.order.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Repair package", 2*QuantityScale, "Lot")}))
	if err != nil {
		t.Fatalf("group the draft: %v", err)
	}
	finalize := finalizeInput(grouped, fixture.user, "PO-654-REPAIR")
	finalize.Items = indexedItems(grouped.Items, 1, 1, 1)
	finalize.AggregateGroups = aggregateInputsFromStored(grouped.AggregateGroups)
	saved, err := fixture.database.FinalizePurchaseOrder(fixture.ctx, finalize)
	if err != nil {
		t.Fatalf("finalize the grouped draft: %v", err)
	}

	// A legacy repair names no groups: refused before the snapshot and before
	// the source moves.
	legacy := repairInput(t, saved, admin, target, "legacy repair")
	legacy.Items = indexedItems(saved.Items)
	if _, err := fixture.database.RepairPurchaseOrderSource(fixture.ctx, legacy); !errors.Is(err, ErrPurchaseOrderClientOutdated) {
		t.Fatalf("legacy repair = %v; want ErrPurchaseOrderClientOutdated", err)
	}
	unchanged, err := fixture.database.GetPurchaseOrder(fixture.ctx, saved.ID)
	if err != nil {
		t.Fatalf("reopen after the refused repair: %v", err)
	}
	if unchanged.QuotationID != saved.QuotationID || unchanged.Version != saved.Version {
		t.Errorf("a refused repair wrote the record: source %d version %d; want %d/%d",
			unchanged.QuotationID, unchanged.Version, saved.QuotationID, saved.Version)
	}
	if history, err := fixture.database.GetPurchaseOrderHistory(fixture.ctx, saved.ID); err != nil || len(history) != 0 {
		t.Fatalf("history after the refused repair = %d entries, %v; want none", len(history), err)
	}

	// The real repair resubmits the same layout; the record follows the target
	// revision and keeps its groups, memberships and per-child allocations.
	repair := repairInput(t, saved, admin, target, "the order followed the wrong revision")
	repair.Items = indexedItems(saved.Items, 1, 1, 1)
	repair.AggregateGroups = aggregateInputsFromStored(saved.AggregateGroups)
	repaired, err := fixture.database.RepairPurchaseOrderSource(fixture.ctx, repair)
	if err != nil {
		t.Fatalf("repair the grouped record: %v", err)
	}
	if repaired.QuotationID != target.ID || repaired.SourceRevision != target.RevisionNumber {
		t.Errorf("source after the repair = %d/%d; want %d/%d",
			repaired.QuotationID, repaired.SourceRevision, target.ID, target.RevisionNumber)
	}
	if len(repaired.AggregateGroups) != 1 || repaired.AggregateGroups[0].Title != "Repair package" {
		t.Fatalf("live groups after the repair = %+v; want the preserved Repair package",
			repaired.AggregateGroups)
	}
	stored := repaired.AggregateGroups[0]
	for _, item := range repaired.Items {
		if item.AggregateGroupID != stored.ID {
			t.Errorf("repaired line %d membership = %d; want %d",
				item.LineNumber, item.AggregateGroupID, stored.ID)
		}
		if count := childAllocationCount(t, fixture.database, fixture.ctx, item.ID); count != 1 {
			t.Errorf("repaired line %d allocations = %d; want each child to keep its own",
				item.LineNumber, count)
		}
	}

	history, err := fixture.database.GetPurchaseOrderHistory(fixture.ctx, saved.ID)
	if err != nil {
		t.Fatalf("history after the repair: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries = %d; want the one repair snapshot", len(history))
	}
	entry := history[0]
	if len(entry.Groups) != 1 || entry.Groups[0].Title != "Repair package" ||
		entry.Groups[0].OrderedUoM != "Lot" {
		t.Errorf("repair snapshot groups = %+v; want the replaced Repair package", entry.Groups)
	}
	for _, item := range entry.Items {
		if item.AggregateGroupOrder != 1 {
			t.Errorf("repair snapshot line %d group order = %d; want 1",
				item.LineNumber, item.AggregateGroupOrder)
		}
	}
}

// TestPurchaseOrderNullReferenceChildrenGroupWithoutAnAllocation pins the #655
// seam: a line with no quotation reference can be a group child before any UI
// path exists. The group persists with its derived total, and the free lines
// own no allocation (there is no quotation leaf to allocate).
func TestPurchaseOrderNullReferenceChildrenGroupWithoutAnAllocation(t *testing.T) {
	database := openTestSchema(t, "vera_po_654w2_null_reference")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	first := blankDraftUpdate(draft, draft.Version, user)
	first.Items = []PurchaseOrderItem{
		{Description: "Free line A", QuantityScaled: 2 * QuantityScale, UoM: "Pc", UnitPriceMinor: 1500},
		{Description: "Free line B", QuantityScaled: QuantityScale, UoM: "Lot", UnitPriceMinor: 2500},
		{Description: "Free line C", QuantityScaled: 3 * QuantityScale, UoM: "Set", UnitPriceMinor: 700},
	}
	lines, err := database.UpdatePurchaseOrderDraft(ctx, first)
	if err != nil {
		t.Fatalf("save the free lines: %v", err)
	}

	grouped, err := database.UpdatePurchaseOrderDraft(ctx,
		draftUpdate(lines, user, indexedItems(lines.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Free package", QuantityScale, "Lot")}))
	if err != nil {
		t.Fatalf("group the free lines: %v", err)
	}
	if len(grouped.AggregateGroups) != 1 {
		t.Fatalf("groups = %d; want 1", len(grouped.AggregateGroups))
	}
	group := grouped.AggregateGroups[0]
	var wantTotal int64
	for _, item := range grouped.Items {
		if item.QuotationItemNo != nil {
			t.Errorf("free line %d carries quotation item %d; want no reference",
				item.LineNumber, *item.QuotationItemNo)
		}
		if item.AggregateGroupID != group.ID {
			t.Errorf("free line %d membership = %d; want %d",
				item.LineNumber, item.AggregateGroupID, group.ID)
		}
		if count := childAllocationCount(t, database, ctx, item.ID); count != 0 {
			t.Errorf("free line %d allocations = %d; want 0 (no quotation leaf)",
				item.LineNumber, count)
		}
		wantTotal += item.NetMinor
	}
	if group.TotalMinor != wantTotal {
		t.Errorf("group total = %d; want the free lines' %d", group.TotalMinor, wantTotal)
	}
}

// TestAggregateGroupStoreTypesCarryNoInternalEconomics is the Story 42 guard
// for the new store shapes: a group header is customer-facing presentation, so
// no field may name a supplier cost, a margin, PPH23, profit, nett or variance.
// The two derived money fields are the group's total and its display unit
// price, which are the customer's own figures.
func TestAggregateGroupStoreTypesCarryNoInternalEconomics(t *testing.T) {
	types := []any{
		PurchaseOrderAggregateGroup{},
		PurchaseOrderAggregateGroupInput{},
		PurchaseOrderCorrectionGroup{},
		PurchaseOrderCorrectionItem{},
	}
	forbidden := []string{"cost", "margin", "pph", "profit", "nett", "variance"}
	for _, value := range types {
		typ := reflect.TypeOf(value)
		for index := 0; index < typ.NumField(); index++ {
			name := strings.ToLower(typ.Field(index).Name)
			for _, banned := range forbidden {
				if strings.Contains(name, banned) {
					t.Errorf("%s.%s names internal economics (%q); a group is presentation only",
						typ.Name(), typ.Field(index).Name, banned)
				}
			}
		}
	}
	groupType := reflect.TypeOf(PurchaseOrderAggregateGroup{})
	for _, allowed := range []string{"TotalMinor", "DisplayUnitPriceMinor"} {
		if _, ok := groupType.FieldByName(allowed); !ok {
			t.Errorf("PurchaseOrderAggregateGroup.%s is missing; it is the customer-facing derived figure",
				allowed)
		}
	}
}

// TestPurchaseOrderStaleSaveWithGroupChangesWritesNothing proves the version
// check still runs before the group write: a save holding an old version cannot
// replace the lines or the layout, and the stored groups are untouched.
func TestPurchaseOrderStaleSaveWithGroupChangesWritesNothing(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_stale")
	grouped, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, indexedItems(fixture.order.Items, 1, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Kept package", QuantityScale, "Lot")}))
	if err != nil {
		t.Fatalf("group the draft: %v", err)
	}
	before := grouped.AggregateGroups[0]

	stale := draftUpdate(grouped, fixture.user, indexedItems(grouped.Items, 1, 1, 1),
		[]PurchaseOrderAggregateGroupInput{itemGroupInput("Rejected package", 2*QuantityScale, "Set")})
	stale.ExpectedVersion = grouped.Version - 1
	if _, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx, stale); !errors.Is(err, ErrPurchaseOrderVersionConflict) {
		t.Fatalf("stale save with new groups = %v; want ErrPurchaseOrderVersionConflict", err)
	}

	stored, err := fixture.database.GetPurchaseOrder(fixture.ctx, fixture.order.ID)
	if err != nil {
		t.Fatalf("reopen after the stale save: %v", err)
	}
	if stored.Version != grouped.Version {
		t.Errorf("version after the stale save = %d; want the unchanged %d",
			stored.Version, grouped.Version)
	}
	if len(stored.AggregateGroups) != 1 || stored.AggregateGroups[0].ID != before.ID ||
		stored.AggregateGroups[0].Title != "Kept package" {
		t.Errorf("groups after the stale save = %+v; want the untouched %+v",
			stored.AggregateGroups, before)
	}
	for _, item := range stored.Items {
		if item.AggregateGroupID != before.ID {
			t.Errorf("line %d membership after the stale save = %d; want %d",
				item.LineNumber, item.AggregateGroupID, before.ID)
		}
	}
}

// TestPurchaseOrderDraftSaveRoundTripsAggregateGroups groups a plain line and a
// whole split package through the real draft save, then reads the group, its
// derived total, the children's membership and their allocations back. The PO's
// own money must not move: grouping is presentation, not pricing.
func TestPurchaseOrderDraftSaveRoundTripsAggregateGroups(t *testing.T) {
	fixture := newAggregateSaveFixture(t, "vera_po_654w2_roundtrip")
	items := indexedItems(fixture.order.Items, 1, 1, 1)
	group := itemGroupInput("  Skid package  ", 2*QuantityScale, " Lot ")
	group.ShowChildrenToCustomer = true

	saved, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(fixture.order, fixture.user, items, []PurchaseOrderAggregateGroupInput{group}))
	if err != nil {
		t.Fatalf("draft save with one aggregate group: %v", err)
	}

	if len(saved.AggregateGroups) != 1 {
		t.Fatalf("groups after the save = %d; want 1", len(saved.AggregateGroups))
	}
	stored := saved.AggregateGroups[0]
	if stored.Title != "Skid package" || stored.OrderedUoM != "Lot" {
		t.Errorf("group title/unit = %q/%q; want the trimmed %q/%q",
			stored.Title, stored.OrderedUoM, "Skid package", "Lot")
	}
	if stored.ID < 1 {
		t.Errorf("group id = %d; want the persisted id", stored.ID)
	}
	if !stored.ShowChildrenToCustomer {
		t.Error("show children to customer = false; the submitted toggle was lost")
	}

	var wantTotal int64
	for _, item := range saved.Items {
		if item.AggregateGroupID != stored.ID {
			t.Errorf("line %d membership = %d; want the group id %d",
				item.LineNumber, item.AggregateGroupID, stored.ID)
		}
		wantTotal += item.NetMinor
		if count := childAllocationCount(t, fixture.database, fixture.ctx, item.ID); count != 1 {
			t.Errorf("line %d allocations = %d; want each child to keep its own",
				item.LineNumber, count)
		}
	}
	if stored.TotalMinor != wantTotal {
		t.Errorf("group total = %d; want the sum of child nets %d", stored.TotalMinor, wantTotal)
	}
	if stored.DisplayUnitPriceMinor == nil || *stored.DisplayUnitPriceMinor != wantTotal/2 {
		t.Errorf("display unit price = %v; want %d", stored.DisplayUnitPriceMinor, wantTotal/2)
	}

	// Grouping writes no money: the header, PPN and grand total are the line
	// set's totals, which did not change.
	if saved.SubtotalMinor != fixture.order.SubtotalMinor ||
		saved.PpnMinor != fixture.order.PpnMinor ||
		saved.GrandTotalMinor != fixture.order.GrandTotalMinor {
		t.Errorf("PO money after grouping = %d/%d/%d; want %d/%d/%d",
			saved.SubtotalMinor, saved.PpnMinor, saved.GrandTotalMinor,
			fixture.order.SubtotalMinor, fixture.order.PpnMinor,
			fixture.order.GrandTotalMinor)
	}

	// Removing a child rewrites the membership: the group survives over the
	// remaining lines with the smaller derived total. Here the plain line is
	// removed and the whole split package stays grouped.
	withoutPlain := saved.Items[1:]
	shrunk, err := fixture.database.UpdatePurchaseOrderDraft(fixture.ctx,
		draftUpdate(saved, fixture.user, indexedItems(withoutPlain, 1, 1),
			[]PurchaseOrderAggregateGroupInput{itemGroupInput("Skid package", QuantityScale, "Lot")}))
	if err != nil {
		t.Fatalf("remove one grouped child: %v", err)
	}
	if len(shrunk.Items) != 2 {
		t.Fatalf("lines after removing the plain child = %d; want 2", len(shrunk.Items))
	}
	if len(shrunk.AggregateGroups) != 1 {
		t.Fatalf("groups after removing a child = %d; want the surviving group", len(shrunk.AggregateGroups))
	}
	if shrunk.AggregateGroups[0].TotalMinor != shrunk.Items[0].NetMinor+shrunk.Items[1].NetMinor {
		t.Errorf("surviving group total = %d; want the two children's %d",
			shrunk.AggregateGroups[0].TotalMinor, shrunk.Items[0].NetMinor+shrunk.Items[1].NetMinor)
	}

	// Reopening the record reads the same picture back: groups and membership
	// are persisted, not a response-only convenience. Group ids are not stable
	// across saves, so the reopened group is the surviving one's id.
	surviving := shrunk.AggregateGroups[0]
	reopened, err := fixture.database.GetPurchaseOrder(fixture.ctx, fixture.order.ID)
	if err != nil {
		t.Fatalf("reopen the grouped draft: %v", err)
	}
	if len(reopened.AggregateGroups) != 1 || reopened.AggregateGroups[0].ID != surviving.ID {
		t.Fatalf("reopened groups = %+v; want the surviving group %d", reopened.AggregateGroups, surviving.ID)
	}
	for _, item := range reopened.Items {
		if item.AggregateGroupID != surviving.ID {
			t.Errorf("reopened line %d membership = %d; want %d",
				item.LineNumber, item.AggregateGroupID, surviving.ID)
		}
	}
}
