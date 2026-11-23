package store

import (
	"reflect"
	"testing"
)

// Issue #654, wave 2: the Actual report reads priced lines and their
// allocations. A group header is not a line, so grouping must change no figure.
// These tests are proofs, not new SQL: they group a saved record and require the
// whole report snapshot to stay byte-for-byte equal, while the per-child price
// variance still resolves against each child's own allocation.

// TestPurchaseOrderReportIsUnchangedByAggregatePresentation saves a PO with one
// child below and one child above its quotation baseline, records the report,
// then groups the children and toggles the customer view with a different
// display quantity. Every report snapshot must be identical: the header is
// presentation only and must never be counted as a priced row or a costed one.
func TestPurchaseOrderReportIsUnchangedByAggregatePresentation(t *testing.T) {
	database := openTestSchema(t, "vera_po_654w2_report_grouping")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Report Grouping", "Grouped variance", "IDR", "FTZ",
		[]reportQuotationLine{
			{Description: "Below baseline", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000},
			{Description: "Above baseline", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000},
		})

	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-654-REPORT-GROUP", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items[0].UnitPriceMinor = 9_000
			input.Items[1].UnitPriceMinor = 11_000
			input.Decisions = []PurchaseOrderDecision{
				varianceDecision("line:1:unitPrice"),
				varianceDecision("line:2:unitPrice"),
			}
		})

	before := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", "year", "all", "IDR"))
	if before.Totals.AmountMinor != 40_000 {
		t.Fatalf("actual amount = %d; want 2x9.000 + 2x11.000 = 40.000", before.Totals.AmountMinor)
	}
	// One child lost 1.000 on two units, the other gained the same. The two
	// realized figures must resolve per child, not against the group.
	assertVarianceTotals(t, before, 40_000, 0, 0, 0, 0, 2_000, 2_000, 0, 0)

	grouped := correctionInput(saved, user, "group the customer lines", saved.Version)
	grouped.Items = indexedItems(saved.Items, 1, 1)
	grouped.AggregateGroups = []PurchaseOrderAggregateGroupInput{
		{Title: "Combined items", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"},
	}
	corrected, err := database.CorrectPurchaseOrder(ctx, grouped)
	if err != nil {
		t.Fatalf("group the saved record: %v", err)
	}
	if len(corrected.AggregateGroups) != 1 {
		t.Fatalf("live groups after grouping = %d; want 1 (the equality must not be vacuous)",
			len(corrected.AggregateGroups))
	}
	afterGrouping := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", "year", "all", "IDR"))
	if !reflect.DeepEqual(before, afterGrouping) {
		t.Errorf("grouping changed the report:\nbefore = %+v\nafter  = %+v", before, afterGrouping)
	}

	toggled := correctionInput(corrected, user, "show the children on the customer view", corrected.Version)
	toggled.Items = indexedItems(corrected.Items, 1, 1)
	toggled.AggregateGroups = []PurchaseOrderAggregateGroupInput{{
		Title:                  "Combined items",
		OrderedQuantityScaled:  3 * QuantityScale,
		OrderedUoM:             "Set",
		ShowChildrenToCustomer: true,
	}}
	afterToggle, err := database.CorrectPurchaseOrder(ctx, toggled)
	if err != nil {
		t.Fatalf("toggle the customer view: %v", err)
	}
	if !afterToggle.AggregateGroups[0].ShowChildrenToCustomer {
		t.Fatal("the toggle was not stored; the report equality would prove nothing")
	}
	afterToggling := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", "year", "all", "IDR"))
	if !reflect.DeepEqual(before, afterToggling) {
		t.Errorf("the customer toggle or display quantity changed the report:\nbefore = %+v\nafter  = %+v",
			before, afterToggling)
	}
}

// TestPurchaseOrderReportDecomposesAMixedDepthGroupWithANullReferenceChild
// groups a mixed-depth source set (a plain line and a priced package) together
// with a free line that has no quotation reference. The report keeps decomposing
// per child: the free line is uncosted, the package's children keep their own
// cost, and the group header adds nothing to the amount.
func TestPurchaseOrderReportDecomposesAMixedDepthGroupWithANullReferenceChild(t *testing.T) {
	database := openTestSchema(t, "vera_po_654w2_report_mixed")
	ctx := poContext(t, database)
	user := poTestUser(t, database)
	quotation := seedReportQuotation(t, database, 2, "PT. Report Mixed", "Mixed depth source", "IDR", "FTZ",
		[]reportQuotationLine{
			{Description: "Plain line", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000},
			{
				Description: "Priced package", UoM: "Lot", Qty: 1,
				SubItems: []reportQuotationSub{
					{Description: "Part one", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000},
					{Description: "Part two", UoM: "Pc", Qty: 2, Cost: 6_000, Sell: 10_000},
				},
			},
		})

	saved := finalizeReportPO(t, database, ctx, user, quotation, "PO-654-REPORT-MIXED", "2026-03-10",
		PurchaseOrderBusinessCategoryFm, func(input *PurchaseOrderFinalize) {
			input.Items = append(input.Items, PurchaseOrderItem{
				Description:    "Free freight line",
				QuantityScaled: QuantityScale,
				UoM:            "Lot",
				UnitPriceMinor: 5_000,
			})
			input.AcknowledgedExtra = []int{4}
		})
	if len(saved.Items) != 4 {
		t.Fatalf("saved lines = %d; want the plain line, two package children and the free line", len(saved.Items))
	}

	before := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", "year", "all", "IDR"))
	// Amount is the four children's nets: 20.000 + 20.000 + 20.000 + 5.000.
	if before.Totals.AmountMinor != 65_000 {
		t.Errorf("actual amount = %d; want the four children's 65.000", before.Totals.AmountMinor)
	}
	// Capital is the quoted cost of the three source children; the free line has
	// no cost basis and stays uncosted. Margin is costed minus capital: the free
	// line's revenue is real but has no known cost, so it adds no margin.
	if before.Totals.CapitalMinor != 36_000 || before.Totals.MarginMinor != 24_000 {
		t.Errorf("actual capital/margin = %d/%d; want 36.000/24.000",
			before.Totals.CapitalMinor, before.Totals.MarginMinor)
	}

	grouped := correctionInput(saved, user, "one customer line over mixed depths", saved.Version)
	grouped.Items = indexedItems(saved.Items, 1, 1, 1, 1)
	grouped.AggregateGroups = []PurchaseOrderAggregateGroupInput{
		{Title: "Combined package and freight", OrderedQuantityScaled: QuantityScale, OrderedUoM: "Lot"},
	}
	corrected, err := database.CorrectPurchaseOrder(ctx, grouped)
	if err != nil {
		t.Fatalf("group the mixed-depth record: %v", err)
	}
	if len(corrected.AggregateGroups) != 1 || corrected.AggregateGroups[0].TotalMinor != 65_000 {
		t.Fatalf("live group = %+v; want one header with the 65.000 child total",
			corrected.AggregateGroups)
	}
	afterGrouping := mustReport(t, database, ctx,
		reportQuery("2026-01-01", "2027-01-01", "year", "all", "IDR"))
	if !reflect.DeepEqual(before, afterGrouping) {
		t.Errorf("grouping a mixed-depth record changed the report:\nbefore = %+v\nafter  = %+v",
			before, afterGrouping)
	}
}
