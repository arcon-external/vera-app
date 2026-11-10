package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Issue #573: a PO line can carry a split-line group and description lines.
// These tests drive the real store against a disposable schema: the fields
// round-trip through a draft save and a reload, the caps refuse an oversized
// payload, and the flat pricing shape is unchanged (the grouped rows still sum
// as ordinary lines).

func groupDraftUpdate(draft PurchaseOrder, user AuthUser, items []PurchaseOrderItem) PurchaseOrderUpdate {
	return PurchaseOrderUpdate{
		ID:              draft.ID,
		BillingAddress:  "Jl. Billing",
		DeliveryAddress: "Jl. Delivery",
		ContactEmail:    "buyer@example.com",
		PONumber:        "PO-GROUP-1",
		ReceivedDate:    datePointer(time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)),
		PaymentTerms:    "30 days after delivery",
		Incoterms:       "DDP",
		UpdatedBy:       user.ID,
		ExpectedVersion: draft.Version,
		Items:           items,
	}
}

func TestPurchaseOrderLineGroupsRoundTrip(t *testing.T) {
	database := openTestSchema(t, "vera_po_line_groups")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	items := []PurchaseOrderItem{
		{
			Description:       "Main distribution panel, 24-way",
			QuantityScaled:    10000,
			UoM:               "Pc",
			UnitPriceMinor:    18500000,
			GroupKey:          "g:test-panel",
			ParentDescription: "LV distribution panel",
			DescriptionLines:  []string{"IP54 enclosure", "Busbar included"},
		},
		{
			Description:       "Wiring and termination",
			QuantityScaled:    10000,
			UoM:               "Lot",
			UnitPriceMinor:    6750000,
			GroupKey:          "g:test-panel",
			ParentDescription: "LV distribution panel",
			DescriptionLines:  []string{"Two technicians", "Three days"},
		},
		{
			Description:      "Freight",
			QuantityScaled:   10000,
			UoM:              "Lot",
			UnitPriceMinor:   1250000,
			DescriptionLines: []string{"Batam delivery"},
		},
	}

	saved, err := updateDraft(t, database, ctx, groupDraftUpdate(draft, user, items))
	if err != nil {
		t.Fatalf("save grouped draft: %v", err)
	}
	loaded, err := database.GetPurchaseOrder(ctx, saved.ID)
	if err != nil {
		t.Fatalf("reload grouped draft: %v", err)
	}
	if len(loaded.Items) != 3 {
		t.Fatalf("items = %d; want 3", len(loaded.Items))
	}
	panel := loaded.Items[0]
	if panel.GroupKey != "g:test-panel" || panel.ParentDescription != "LV distribution panel" {
		t.Errorf("first grouped row = %q / %q", panel.GroupKey, panel.ParentDescription)
	}
	if strings.Join(panel.DescriptionLines, "|") != "IP54 enclosure|Busbar included" {
		t.Errorf("first row description lines = %v", panel.DescriptionLines)
	}
	if loaded.Items[1].GroupKey != "g:test-panel" {
		t.Errorf("second grouped row lost its key: %q", loaded.Items[1].GroupKey)
	}
	if loaded.Items[2].GroupKey != "" || loaded.Items[2].ParentDescription != "" {
		t.Errorf("plain row grew a group: %q / %q",
			loaded.Items[2].GroupKey, loaded.Items[2].ParentDescription)
	}
	if strings.Join(loaded.Items[2].DescriptionLines, "|") != "Batam delivery" {
		t.Errorf("plain row description lines = %v", loaded.Items[2].DescriptionLines)
	}
	// Grouping is presentation only: the flat rows still sum as ordinary lines.
	if loaded.SubtotalMinor != 26500000 {
		t.Errorf("subtotal = %d; want 26500000", loaded.SubtotalMinor)
	}
}

func TestPurchaseOrderLineGroupCapsAreEnforced(t *testing.T) {
	database := openTestSchema(t, "vera_po_line_group_caps")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	base := PurchaseOrderItem{
		Description:       "Line",
		QuantityScaled:    10000,
		UoM:               "Pc",
		UnitPriceMinor:    1000,
		GroupKey:          "g:caps",
		ParentDescription: "Group",
	}

	cases := []struct {
		name  string
		item  PurchaseOrderItem
		error string
	}{
		{
			name: "over-long group key",
			item: func() PurchaseOrderItem {
				item := base
				item.GroupKey = "g:" + strings.Repeat("x", 120)
				return item
			}(),
			error: "group key must be at most 120 characters",
		},
		{
			name: "over-long parent description",
			item: func() PurchaseOrderItem {
				item := base
				item.ParentDescription = strings.Repeat("界", 501)
				return item
			}(),
			error: "parent description must be at most 500 characters",
		},
		{
			name: "too many description lines",
			item: func() PurchaseOrderItem {
				item := base
				item.DescriptionLines = make([]string, 51)
				for index := range item.DescriptionLines {
					item.DescriptionLines[index] = "line"
				}
				return item
			}(),
			error: "more than 50 description lines",
		},
		{
			name: "over-long description line",
			item: func() PurchaseOrderItem {
				item := base
				item.DescriptionLines = []string{strings.Repeat("界", 501)}
				return item
			}(),
			error: "description lines must be at most 500 characters",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			current, err := database.GetPurchaseOrder(ctx, draft.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			_, err = database.UpdatePurchaseOrderDraft(ctx,
				groupDraftUpdate(current, user, []PurchaseOrderItem{testCase.item}))
			if !errors.Is(err, ErrPurchaseOrderInvalid) {
				t.Fatalf("error = %v; want ErrPurchaseOrderInvalid", err)
			}
			if !strings.Contains(err.Error(), testCase.error) {
				t.Errorf("error = %v; want %q", err, testCase.error)
			}
		})
	}
}

func TestPurchaseOrderLineGroupsAreIgnoredByTheRetiredDiscount(t *testing.T) {
	database := openTestSchema(t, "vera_po_line_groups_discount")
	user := poTestUser(t, database)
	ctx := poContext(t, database)
	draft := createBlankDraft(t, database, ctx, user)

	input := groupDraftUpdate(draft, user, []PurchaseOrderItem{{
		Description:       "Panel",
		QuantityScaled:    10000,
		UoM:               "Pc",
		UnitPriceMinor:    1000,
		GroupKey:          "g:discount",
		ParentDescription: "Panel",
	}})
	input.DiscountPercent = 10
	input.AdjustmentMinor = -5
	input.AdjustmentReason = "Retired"

	saved, err := updateDraft(t, database, ctx, input)
	if err != nil {
		t.Fatalf("save with retired values: %v", err)
	}
	if saved.DiscountPercent != 0 || saved.AdjustmentMinor != 0 || saved.AdjustmentReason != "" {
		t.Errorf("retired values were stored: %d / %d / %q",
			saved.DiscountPercent, saved.AdjustmentMinor, saved.AdjustmentReason)
	}
	if saved.GrandTotalMinor != saved.SubtotalMinor+saved.PpnMinor {
		t.Errorf("grand = %d; want subtotal %d + PPN %d",
			saved.GrandTotalMinor, saved.SubtotalMinor, saved.PpnMinor)
	}
}
