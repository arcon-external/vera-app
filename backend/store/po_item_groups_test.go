package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Issue #654: the aggregate placement rules. Validation runs before any row is
// written, so a malformed group or an ambiguous child set is refused with the
// group or line named instead of half-saved.

func itemGroupInput(title string, quantity int64, uom string) PurchaseOrderAggregateGroupInput {
	return PurchaseOrderAggregateGroupInput{
		Title:                 title,
		OrderedQuantityScaled: quantity,
		OrderedUoM:            uom,
	}
}

// groupedItem is one submitted line carrying a group index; groupKey is "" for a
// plain line and shared by the rows of one split display group (#573).
func groupedItem(groupKey string, groupIndex int) PurchaseOrderItem {
	return PurchaseOrderItem{GroupKey: groupKey, AggregateGroupIndex: groupIndex}
}

func TestPurchaseOrderAggregateGroupPlacementValidation(t *testing.T) {
	validGroup := func() PurchaseOrderAggregateGroupInput {
		return itemGroupInput("Spare parts package", QuantityScale, "Lot")
	}

	t.Run("ungrouped submission", func(t *testing.T) {
		if err := validatePurchaseOrderAggregatePlacement(nil,
			[]PurchaseOrderItem{groupedItem("", 0), groupedItem("q:2", 0)}); err != nil {
			t.Fatalf("ungrouped submission = %v; want nil", err)
		}
		if err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{}, []PurchaseOrderItem{groupedItem("", 0)}); err != nil {
			t.Fatalf("empty non-nil groups with ungrouped lines = %v; want nil", err)
		}
	})

	t.Run("nil groups cannot carry indexed lines", func(t *testing.T) {
		err := validatePurchaseOrderAggregatePlacement(nil,
			[]PurchaseOrderItem{groupedItem("", 1)})
		if !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
			t.Fatalf("nil groups with an indexed line = %v; want ErrPurchaseOrderAggregateInvalid", err)
		}
		if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("error %q does not name line 1", err.Error())
		}
	})

	t.Run("one valid group", func(t *testing.T) {
		err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{validGroup()},
			[]PurchaseOrderItem{groupedItem("", 1), groupedItem("q:2", 1), groupedItem("q:2", 1)})
		if err != nil {
			t.Fatalf("valid group = %v; want nil", err)
		}
	})

	t.Run("group header fields", func(t *testing.T) {
		cases := []struct {
			name  string
			group PurchaseOrderAggregateGroupInput
			label string
		}{
			{"blank title", itemGroupInput("   ", QuantityScale, "Lot"), "group 1"},
			{"empty title", itemGroupInput("", QuantityScale, "Lot"), "group 1"},
			{"title too long", itemGroupInput(strings.Repeat("x", 501), QuantityScale, "Lot"), "group 1"},
			{"zero quantity", itemGroupInput("Package", 0, "Lot"), "group 1"},
			{"negative quantity", itemGroupInput("Package", -1, "Lot"), "group 1"},
			{"quantity above the exact range", itemGroupInput("Package", MaxExactMinor+1, "Lot"), "group 1"},
			{"blank unit", itemGroupInput("Package", QuantityScale, "  "), "group 1"},
			{"unit too long", itemGroupInput("Package", QuantityScale, strings.Repeat("x", 81)), "group 1"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				err := validatePurchaseOrderAggregatePlacement(
					[]PurchaseOrderAggregateGroupInput{tc.group},
					[]PurchaseOrderItem{groupedItem("", 1)})
				if !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
					t.Fatalf("invalid group = %v; want ErrPurchaseOrderAggregateInvalid", err)
				}
				if !strings.Contains(err.Error(), tc.label) {
					t.Errorf("error %q does not name %s", err.Error(), tc.label)
				}
			})
		}
	})

	t.Run("boundary values are accepted", func(t *testing.T) {
		group := itemGroupInput(strings.Repeat("y", 500), MaxExactMinor, strings.Repeat("z", 80))
		if err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{group},
			[]PurchaseOrderItem{groupedItem("", 1)}); err != nil {
			t.Fatalf("boundary group = %v; want nil", err)
		}
	})

	t.Run("line indexes", func(t *testing.T) {
		group := validGroup()
		if err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{group},
			[]PurchaseOrderItem{groupedItem("", -1)}); !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
			t.Fatalf("negative index = %v; want ErrPurchaseOrderAggregateInvalid", err)
		}
		err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{group},
			[]PurchaseOrderItem{groupedItem("", 1), groupedItem("", 2)})
		if !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
			t.Fatalf("out-of-range index = %v; want ErrPurchaseOrderAggregateInvalid", err)
		}
		if !strings.Contains(err.Error(), "line 2") {
			t.Errorf("error %q does not name line 2", err.Error())
		}
	})

	t.Run("every group needs a child", func(t *testing.T) {
		err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{validGroup(), validGroup()},
			[]PurchaseOrderItem{groupedItem("", 1)})
		if !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
			t.Fatalf("childless group = %v; want ErrPurchaseOrderAggregateInvalid", err)
		}
		if !strings.Contains(err.Error(), "group 2") {
			t.Errorf("error %q does not name group 2", err.Error())
		}
	})

	t.Run("children are contiguous", func(t *testing.T) {
		group := validGroup()
		err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{group},
			[]PurchaseOrderItem{groupedItem("", 1), groupedItem("", 0), groupedItem("", 1)})
		if !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
			t.Fatalf("non-contiguous group = %v; want ErrPurchaseOrderAggregateInvalid", err)
		}
		if !strings.Contains(err.Error(), "group 1") || !strings.Contains(err.Error(), "contiguous") {
			t.Errorf("error %q does not say group 1 is not contiguous", err.Error())
		}
		if err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{group, validGroup()},
			[]PurchaseOrderItem{groupedItem("", 1), groupedItem("", 1), groupedItem("", 2), groupedItem("", 2)}); err != nil {
			t.Fatalf("two contiguous groups = %v; want nil", err)
		}
	})

	t.Run("a split display group is whole or absent", func(t *testing.T) {
		group := validGroup()
		err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{group},
			[]PurchaseOrderItem{groupedItem("q:2", 1), groupedItem("q:2", 0)})
		if !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
			t.Fatalf("half-package group = %v; want ErrPurchaseOrderAggregateInvalid", err)
		}
		if !strings.Contains(err.Error(), "line 2") {
			t.Errorf("error %q does not name the line outside the group", err.Error())
		}
		if err := validatePurchaseOrderAggregatePlacement(
			[]PurchaseOrderAggregateGroupInput{group},
			[]PurchaseOrderItem{groupedItem("q:2", 1), groupedItem("q:2", 1)}); err != nil {
			t.Fatalf("whole package = %v; want nil", err)
		}
	})
}

// itemGroupDraft is the shared fixture for the store-level aggregate tests: a
// real draft from the standard seeded revision, whose three priced lines are one
// plain line (depth 1) and one split package of two sub-items (depths 2 and 3),
// so mixed-depth grouping is exercised by the ordinary data.
type itemGroupDraft struct {
	database *Store
	ctx      context.Context
	schema   string
	orgID    string
	order    PurchaseOrder
}

func newItemGroupDraft(t *testing.T, schemaName string) itemGroupDraft {
	t.Helper()
	database := openTestSchema(t, schemaName)
	quotation := seedPOQuotation(t, database, "PT. Aggregate Buyer", "Aggregate package")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	order, err := database.CreatePurchaseOrderDraft(ctx, PurchaseOrderCreate{
		QuotationID:    quotation.ID,
		RevisionNumber: quotation.RevisionNumber,
		CreatedBy:      user.ID,
	})
	if err != nil {
		t.Fatalf("create aggregate fixture draft: %v", err)
	}
	org, err := database.DefaultOrganization(context.Background())
	if err != nil {
		t.Fatalf("default organization: %v", err)
	}
	if len(order.Items) != 3 {
		t.Fatalf("fixture lines = %d; want the seeded plain line and two package children", len(order.Items))
	}
	return itemGroupDraft{database: database, ctx: ctx, schema: database.configSchema(), orgID: org.ID, order: order}
}

// writeItemGroups replaces the lines and the groups in one transaction, exactly
// as the save paths do (validate, replace lines, replace groups).
func (fixture itemGroupDraft) writeItemGroups(t *testing.T, items []PurchaseOrderItem, groups []PurchaseOrderAggregateGroupInput) {
	t.Helper()
	tx, err := fixture.database.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatalf("begin aggregate write: %v", err)
	}
	defer func() { _ = tx.Rollback(fixture.ctx) }()
	kept, _, err := replacePurchaseOrderItems(fixture.ctx, tx, fixture.schema, fixture.orgID, fixture.order.ID, items)
	if err != nil {
		t.Fatalf("replace fixture lines: %v", err)
	}
	if err := replacePurchaseOrderItemGroups(fixture.ctx, tx, fixture.schema, fixture.orgID, fixture.order.ID, groups, kept); err != nil {
		t.Fatalf("replace fixture groups: %v", err)
	}
	if err := tx.Commit(fixture.ctx); err != nil {
		t.Fatalf("commit aggregate write: %v", err)
	}
}

func (fixture itemGroupDraft) loadGroups(t *testing.T) []PurchaseOrderAggregateGroup {
	t.Helper()
	groups, err := loadPurchaseOrderItemGroups(fixture.ctx, fixture.database.pool, fixture.schema, fixture.orgID, fixture.order.ID)
	if err != nil {
		t.Fatalf("load aggregate groups: %v", err)
	}
	return groups
}

// TestPurchaseOrderItemGroupsRoundTrip groups a plain line and a whole split
// package, reads the derived total and order back, proves a child price edit
// moves the total while the PO's own money does not move, then ungroups.
func TestPurchaseOrderItemGroupsRoundTrip(t *testing.T) {
	fixture := newItemGroupDraft(t, "vera_po_654_roundtrip")
	items := fixture.order.Items
	// The two package rows share group_key "q:2"; the plain line carries none.
	if items[0].GroupKey != "" || items[1].GroupKey == "" || items[1].GroupKey != items[2].GroupKey {
		t.Fatalf("fixture group keys = %q/%q/%q; want plain then one shared split key",
			items[0].GroupKey, items[1].GroupKey, items[2].GroupKey)
	}
	wantTotal := items[0].NetMinor + items[1].NetMinor + items[2].NetMinor
	subtotalBefore := fixture.order.SubtotalMinor
	grandBefore := fixture.order.GrandTotalMinor
	for index := range items {
		items[index].AggregateGroupIndex = 1
	}

	fixture.writeItemGroups(t, items, []PurchaseOrderAggregateGroupInput{{
		Title:                  "  Skid package  ",
		OrderedQuantityScaled:  20000,
		OrderedUoM:             " Lot ",
		ShowChildrenToCustomer: false,
	}})

	groups := fixture.loadGroups(t)
	if len(groups) != 1 {
		t.Fatalf("groups = %d; want 1", len(groups))
	}
	group := groups[0]
	if group.DisplayOrder != 1 {
		t.Errorf("display order = %d; want the first child's line number 1", group.DisplayOrder)
	}
	if group.Title != "Skid package" {
		t.Errorf("title = %q; want the trimmed %q", group.Title, "Skid package")
	}
	if group.OrderedQuantityScaled != 20000 || group.OrderedUoM != "Lot" {
		t.Errorf("display quantity/unit = %d/%q; want 20000/Lot",
			group.OrderedQuantityScaled, group.OrderedUoM)
	}
	if group.ShowChildrenToCustomer {
		t.Error("show children to customer = true; want the safe default false")
	}
	if group.TotalMinor != wantTotal {
		t.Errorf("total = %d; want the sum of child nets %d", group.TotalMinor, wantTotal)
	}
	// round_half_away(total * 10000 / 20000) = round_half_away(total / 2).
	wantUnit := (wantTotal + 1) / 2
	if group.DisplayUnitPriceMinor == nil || *group.DisplayUnitPriceMinor != wantUnit {
		t.Errorf("display unit price = %v; want %d", group.DisplayUnitPriceMinor, wantUnit)
	}

	// Membership is persisted on every child, and a split package stays whole.
	var memberships int
	if err := fixture.database.pool.QueryRow(fixture.ctx, fmt.Sprintf(`
		SELECT count(*) FROM %s.purchase_order_items
		WHERE purchase_order_id = $1 AND aggregate_group_id = $2`,
		fixture.schema), fixture.order.ID, group.ID).Scan(&memberships); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if memberships != 3 {
		t.Errorf("child memberships = %d; want 3", memberships)
	}
	var splitRows int
	if err := fixture.database.pool.QueryRow(fixture.ctx, fmt.Sprintf(`
		SELECT count(*) FROM %s.purchase_order_items
		WHERE purchase_order_id = $1 AND group_key = $2 AND aggregate_group_id = $3`,
		fixture.schema), fixture.order.ID, items[1].GroupKey, group.ID).Scan(&splitRows); err != nil {
		t.Fatalf("count split package memberships: %v", err)
	}
	if splitRows != 2 {
		t.Errorf("split package children inside the group = %d; want 2 (the package stays whole)", splitRows)
	}

	// Grouping is presentation: it writes no money.
	var subtotal, grand int64
	if err := fixture.database.pool.QueryRow(fixture.ctx, fmt.Sprintf(
		`SELECT subtotal_minor, grand_total_minor FROM %s.purchase_orders WHERE id = $1`,
		fixture.schema), fixture.order.ID).Scan(&subtotal, &grand); err != nil {
		t.Fatalf("read purchase order money: %v", err)
	}
	if subtotal != subtotalBefore || grand != grandBefore {
		t.Errorf("money after grouping = %d/%d; want unchanged %d/%d",
			subtotal, grand, subtotalBefore, grandBefore)
	}

	// A child price edit moves the derived total; the header is never stored.
	if _, err := fixture.database.pool.Exec(fixture.ctx, fmt.Sprintf(`
		UPDATE %s.purchase_order_items
		SET unit_price_minor = unit_price_minor + 100,
		    gross_minor = gross_minor + 100, net_minor = net_minor + 100
		WHERE id = $1`, fixture.schema), items[1].ID); err != nil {
		t.Fatalf("edit a child price: %v", err)
	}
	edited := fixture.loadGroups(t)
	if len(edited) != 1 || edited[0].TotalMinor != wantTotal+100 {
		t.Fatalf("total after the child edit = %v; want %d", edited, wantTotal+100)
	}
	if edited[0].DisplayUnitPriceMinor == nil || *edited[0].DisplayUnitPriceMinor != (wantTotal+100+1)/2 {
		t.Errorf("display unit price after the child edit = %v; want %d",
			edited[0].DisplayUnitPriceMinor, (wantTotal+100+1)/2)
	}

	// Ungroup is an empty, non-nil submission with no indexed line: the groups
	// go, the lines stay exactly where they are.
	for index := range items {
		items[index].AggregateGroupIndex = 0
	}
	fixture.writeItemGroups(t, items, []PurchaseOrderAggregateGroupInput{})
	if groups := fixture.loadGroups(t); len(groups) != 0 {
		t.Errorf("groups after ungroup = %d; want 0", len(groups))
	}
	var grouped int
	if err := fixture.database.pool.QueryRow(fixture.ctx, fmt.Sprintf(`
		SELECT count(*) FROM %s.purchase_order_items
		WHERE purchase_order_id = $1 AND aggregate_group_id IS NOT NULL`,
		fixture.schema), fixture.order.ID).Scan(&grouped); err != nil {
		t.Fatalf("count memberships after ungroup: %v", err)
	}
	if grouped != 0 {
		t.Errorf("grouped lines after ungroup = %d; want 0", grouped)
	}
	var lines int
	if err := fixture.database.pool.QueryRow(fixture.ctx, fmt.Sprintf(`
		SELECT count(*) FROM %s.purchase_order_items WHERE purchase_order_id = $1`,
		fixture.schema), fixture.order.ID).Scan(&lines); err != nil {
		t.Fatalf("count lines after ungroup: %v", err)
	}
	if lines != 3 {
		t.Errorf("lines after ungroup = %d; want 3 (ungroup never deletes a priced line)", lines)
	}
}

// TestPurchaseOrderItemGroupsTwoGroupsKeepTheirFirstChildOrder proves
// display_order is the first child's line number, that the read is ordered by
// it, and that a group over the split package alone is valid.
func TestPurchaseOrderItemGroupsTwoGroupsKeepTheirFirstChildOrder(t *testing.T) {
	fixture := newItemGroupDraft(t, "vera_po_654_two_groups")
	items := fixture.order.Items
	items[0].AggregateGroupIndex = 1
	items[1].AggregateGroupIndex = 2
	items[2].AggregateGroupIndex = 2

	fixture.writeItemGroups(t, items, []PurchaseOrderAggregateGroupInput{
		{Title: "Plain line group", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"},
		{Title: "Package group", OrderedQuantityScaled: 3 * QuantityScale, OrderedUoM: "Set", ShowChildrenToCustomer: true},
	})

	groups := fixture.loadGroups(t)
	if len(groups) != 2 {
		t.Fatalf("groups = %d; want 2", len(groups))
	}
	if groups[0].DisplayOrder != 1 || groups[1].DisplayOrder != 2 {
		t.Errorf("display orders = %d/%d; want 1/2", groups[0].DisplayOrder, groups[1].DisplayOrder)
	}
	if groups[0].Title != "Plain line group" || groups[1].Title != "Package group" {
		t.Errorf("titles = %q/%q; want them in display order",
			groups[0].Title, groups[1].Title)
	}
	if groups[0].TotalMinor != items[0].NetMinor {
		t.Errorf("first group total = %d; want %d", groups[0].TotalMinor, items[0].NetMinor)
	}
	if groups[1].TotalMinor != items[1].NetMinor+items[2].NetMinor {
		t.Errorf("package group total = %d; want %d",
			groups[1].TotalMinor, items[1].NetMinor+items[2].NetMinor)
	}
	if !groups[1].ShowChildrenToCustomer {
		t.Error("package group does not show its children; the submitted toggle was lost")
	}
	// round_half_away(total * 10000 / 30000).
	wantUnit := (groups[1].TotalMinor*QuantityScale + 15000) / 30000
	if groups[1].DisplayUnitPriceMinor == nil || *groups[1].DisplayUnitPriceMinor != wantUnit {
		t.Errorf("package display unit price = %v; want %d",
			groups[1].DisplayUnitPriceMinor, wantUnit)
	}
}

// TestPurchaseOrderItemGroupsNilSubmissionLeavesGroupsAlone pins the
// compatibility contract at the helper: a nil slice is "this client cannot
// express groups", so it must never delete them. The empty slice is the
// ungroup. Wave 2 refuses a nil submission outright on a grouped record; this
// test makes the helper itself non-destructive so a missed refusal cannot
// silently ungroup.
func TestPurchaseOrderItemGroupsNilSubmissionLeavesGroupsAlone(t *testing.T) {
	fixture := newItemGroupDraft(t, "vera_po_654_nil_groups")
	items := fixture.order.Items
	for index := range items {
		items[index].AggregateGroupIndex = 1
	}
	fixture.writeItemGroups(t, items, []PurchaseOrderAggregateGroupInput{{
		Title: "Package", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot",
	}})
	before := fixture.loadGroups(t)
	if len(before) != 1 {
		t.Fatalf("groups before the nil save = %d; want 1", len(before))
	}

	// A legacy client submits no groups and no indexes.
	for index := range items {
		items[index].AggregateGroupIndex = 0
	}
	fixture.writeItemGroups(t, items, nil)

	after := fixture.loadGroups(t)
	if len(after) != 1 || after[0].ID != before[0].ID || after[0].TotalMinor != before[0].TotalMinor {
		t.Fatalf("groups after a nil submission = %v; want the unchanged %v", after, before)
	}
}

// TestPurchaseOrderItemGroupsDerivedUnitPriceRefusesOverflow proves the derived
// display unit price is nil, never wrapped, when a tiny display quantity would
// push it past the exact transport range. The total stays exact.
func TestPurchaseOrderItemGroupsDerivedUnitPriceRefusesOverflow(t *testing.T) {
	cases := []struct {
		name     string
		total    int64
		quantity int64
		want     *int64
	}{
		{"exact division", 200, 20000, ptrInt64(100)},
		{"round half away from zero", 101, 20000, ptrInt64(51)},
		{"round down below the half", 100, 30000, ptrInt64(33)},
		{"zero total", 0, 10000, ptrInt64(0)},
		{"beyond the exact range", MaxExactMinor, 1, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := aggregateDisplayUnitPriceMinor(tc.total, tc.quantity)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("display unit price = %v; want %v", got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("display unit price = %d; want %d", *got, *tc.want)
			}
		})
	}
}

// TestPurchaseOrderItemGroupsPlacementRefusalsWriteNothing proves validation
// runs before any row is written: a refused placement leaves neither a header
// nor a membership behind, so a failed save cannot half-apply a group.
func TestPurchaseOrderItemGroupsPlacementRefusalsWriteNothing(t *testing.T) {
	fixture := newItemGroupDraft(t, "vera_po_654_refusals")
	base := fixture.order.Items

	cases := []struct {
		name   string
		groups []PurchaseOrderAggregateGroupInput
		mutate func([]PurchaseOrderItem) []PurchaseOrderItem
	}{
		{
			name:   "empty group",
			groups: []PurchaseOrderAggregateGroupInput{{Title: "Empty", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"}},
			mutate: func(items []PurchaseOrderItem) []PurchaseOrderItem { return items },
		},
		{
			name:   "half a split package",
			groups: []PurchaseOrderAggregateGroupInput{{Title: "Half", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"}},
			mutate: func(items []PurchaseOrderItem) []PurchaseOrderItem {
				items[1].AggregateGroupIndex = 1
				return items
			},
		},
		{
			name:   "non-contiguous children",
			groups: []PurchaseOrderAggregateGroupInput{{Title: "Gap", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"}},
			mutate: func(items []PurchaseOrderItem) []PurchaseOrderItem {
				items[0].AggregateGroupIndex = 1
				items[2].AggregateGroupIndex = 1
				return items
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := append([]PurchaseOrderItem{}, base...)
			items = tc.mutate(items)
			tx, err := fixture.database.pool.Begin(fixture.ctx)
			if err != nil {
				t.Fatalf("begin refused write: %v", err)
			}
			defer func() { _ = tx.Rollback(fixture.ctx) }()
			kept, _, err := replacePurchaseOrderItems(fixture.ctx, tx, fixture.schema, fixture.orgID, fixture.order.ID, items)
			if err != nil {
				t.Fatalf("replace lines for the refusal: %v", err)
			}
			err = replacePurchaseOrderItemGroups(fixture.ctx, tx, fixture.schema, fixture.orgID, fixture.order.ID, tc.groups, kept)
			if !errors.Is(err, ErrPurchaseOrderAggregateInvalid) {
				t.Fatalf("refusal = %v; want ErrPurchaseOrderAggregateInvalid", err)
			}
			// The refusal happens before the group delete, so the transaction holds
			// no header to roll back; assert that on the open transaction.
			var groups int
			if err := tx.QueryRow(fixture.ctx, fmt.Sprintf(
				`SELECT count(*) FROM %s.purchase_order_item_groups WHERE purchase_order_id = $1`,
				fixture.schema), fixture.order.ID).Scan(&groups); err != nil {
				t.Fatalf("count groups inside the refused transaction: %v", err)
			}
			if groups != 0 {
				t.Errorf("groups written by a refused placement = %d; want 0", groups)
			}
			var grouped int
			if err := tx.QueryRow(fixture.ctx, fmt.Sprintf(`
				SELECT count(*) FROM %s.purchase_order_items
				WHERE purchase_order_id = $1 AND aggregate_group_id IS NOT NULL`,
				fixture.schema), fixture.order.ID).Scan(&grouped); err != nil {
				t.Fatalf("count memberships inside the refused transaction: %v", err)
			}
			if grouped != 0 {
				t.Errorf("memberships written by a refused placement = %d; want 0", grouped)
			}
		})
	}
}
