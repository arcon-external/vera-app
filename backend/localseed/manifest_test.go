package localseed

import (
	"strings"
	"testing"

	"vera-app/backend/store"
)

func TestManifestMatchesTheApprovedMatrix(t *testing.T) {
	manifest := MockManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("the approved manifest is invalid: %v", err)
	}
	if len(manifest.Quotations) != 6 {
		t.Fatalf("live quotations = %d; want 6", len(manifest.Quotations))
	}
	if len(manifest.PurchaseOrders) != 8 {
		t.Fatalf("purchase orders = %d; want 8", len(manifest.PurchaseOrders))
	}
	if len(manifest.SyntheticSources) != 2 {
		t.Fatalf("synthetic sources = %d; want 2", len(manifest.SyntheticSources))
	}
	occupied, available := manifest.OccupiedQuotations()
	if occupied != 5 || available != 1 {
		t.Fatalf("occupied/available = %d/%d; want 5/1", occupied, available)
	}

	wantQuotations := []string{
		"ftz-goods-finalized-pdf",
		"nonftz-goods-finalized-differences",
		"services-nonconstruction-incomplete-draft",
		"services-construction-draft-differences",
		"nonftz-subitems-no-pdf-correction",
		"ftz-goods-available",
	}
	for index, want := range wantQuotations {
		if manifest.Quotations[index].Key != want {
			t.Errorf("quotation %d = %q; want %q", index, manifest.Quotations[index].Key, want)
		}
		if !strings.Contains(manifest.Quotations[index].Subject, Fictional) {
			t.Errorf("quotation %q is not marked fictional", manifest.Quotations[index].Key)
		}
	}

	wantOrders := []struct {
		key         string
		state       string
		sourceState string
		document    string
	}{
		{"po-linked-incomplete-draft", stateDraft, sourceLive, documentNone},
		{"po-linked-draft-differences", stateDraft, sourceLive, documentNone},
		{"po-finalized-pdf", stateSaved, sourceLive, documentText},
		{"po-finalized-differences-adjustment", stateSaved, sourceLive, documentNone},
		{"po-finalized-no-pdf-confirmation-correction", stateSaved, sourceLive, documentNone},
		{"po-never-linked-mixed-pdf", stateDraft, sourceNever, documentMixed},
		{"po-former-linked-draft", stateDraft, sourceFormer, documentNone},
		{"po-former-linked-finalized", stateSaved, sourceFormer, documentText},
	}
	for index, want := range wantOrders {
		got := manifest.PurchaseOrders[index]
		if got.Key != want.key || got.ExpectedState != want.state || got.SourceState != want.sourceState || got.Document != want.document {
			t.Errorf("purchase order %d = %+v; want key=%q state=%q source=%q document=%q",
				index, got, want.key, want.state, want.sourceState, want.document)
		}
	}

	coverage := manifest.Coverage()
	if !coverage.FTZGoods || !coverage.NonFTZGoodsWithPPN ||
		!coverage.ServicesNonConstruction || !coverage.ServicesConstruction ||
		!coverage.WholeDiscount || !coverage.RowDiscount ||
		!coverage.SubItems || !coverage.MultipleRevisions {
		t.Fatalf("coverage is incomplete: %+v", coverage)
	}
}

// TestManifestValidationRefusesDrift proves the guard can fail: a record shape
// the database would refuse (a finalized never-linked PO) must stop the seed
// before any write.
func TestManifestValidationRefusesDrift(t *testing.T) {
	mutated := MockManifest()
	mutated.PurchaseOrders[5].ExpectedState = stateSaved
	if err := mutated.Validate(); err == nil {
		t.Fatal("a finalized never-linked purchase order passed manifest validation")
	}

	extra := MockManifest()
	extra.Quotations = append(extra.Quotations, extra.Quotations[0])
	if err := extra.Validate(); err == nil {
		t.Fatal("seven live quotations passed manifest validation")
	}

	unmarked := MockManifest()
	unmarked.Quotations[0].CustomerName = "PT Not Marked"
	if err := unmarked.Validate(); err == nil {
		t.Fatal("an unmarked customer name passed manifest validation")
	}
}

func TestPricingMirrorsTheQuotationOrder(t *testing.T) {
	items, err := priceItems([]ItemSpec{
		{
			Description:     "Plain line (MOCK)",
			UoM:             "Unit",
			Qty:             2,
			UnitCost:        1_000,
			MarginEnabled:   true,
			MarginPercent:   50,
			DiscountEnabled: true,
			DiscountPercent: 10,
		},
	})
	if err != nil {
		t.Fatalf("price the plain line: %v", err)
	}
	if items[0].ListUnitPrice != 1_500 {
		t.Errorf("list unit = %d; want 1500 (margin first)", items[0].ListUnitPrice)
	}
	if items[0].MarginalizedUnitPrice != 1_350 {
		t.Errorf("net unit = %d; want 1350 (10%% off the list)", items[0].MarginalizedUnitPrice)
	}
	if items[0].MarginAmount != 350 {
		t.Errorf("margin amount = %d; want 350 per unit", items[0].MarginAmount)
	}
	totals, err := priceQuotationTotals(items, 10, "NonFTZ", "Goods", "")
	if err != nil {
		t.Fatalf("price the totals: %v", err)
	}
	if totals.OriginalSubtotal != 2_000 || totals.OriginalGrand != 2_000 || totals.OriginalDiscount != 0 {
		t.Errorf("cost side = %+v; want 2000 and no discount", totals)
	}
	if totals.MarginalizedSubtotal != 2_700 || totals.MarginalizedDiscount != 270 || totals.MarginalizedGrand != 2_430 {
		t.Errorf("sell side = %+v; want 2700, 270, 2430", totals)
	}
	if totals.PpnAmount != 267 || totals.CustomerGrandTotal != 2_697 {
		t.Errorf("PPN/customer = %d/%d; want 267/2697", totals.PpnAmount, totals.CustomerGrandTotal)
	}
	if totals.ProfitAmount != 430 || totals.NettProfit != 430 {
		t.Errorf("profit/nett = %d/%d; want 430/430", totals.ProfitAmount, totals.NettProfit)
	}

	packageItems, err := priceItems([]ItemSpec{
		{
			Description:   "Package (MOCK)",
			UoM:           "Lot",
			Qty:           1,
			ShowSubPrices: true,
			SubItems: []SubItemSpec{
				{Description: "Part A", UoM: "Unit", Qty: 2, UnitCost: 1_000, MarginEnabled: true, MarginPercent: 10},
				{Description: "Part B", UoM: "Unit", Qty: 1, UnitCost: 500, DiscountEnabled: true, DiscountPercent: 20},
			},
		},
	})
	if err != nil {
		t.Fatalf("price the package: %v", err)
	}
	parent := packageItems[0]
	if parent.Qty != 1 || parent.OriginalTotal != 2_500 || parent.MarginalizedTotal != 2_600 {
		t.Errorf("package rollup = qty %d cost %d net %d; want 1/2500/2600", parent.Qty, parent.OriginalTotal, parent.MarginalizedTotal)
	}
	if parent.ListUnitPrice != 2_700 || parent.DiscountAmount != 100 || parent.DiscountPercent != 3 {
		t.Errorf("package list/discount = %d/%d/%d%%; want 2700/100/3%%",
			parent.ListUnitPrice, parent.DiscountAmount, parent.DiscountPercent)
	}
	if len(parent.SubItems) != 2 || parent.SubItems[0].Total != 2_200 || parent.SubItems[1].Total != 400 {
		t.Errorf("package parts = %+v; want totals 2200 and 400", parent.SubItems)
	}
}

func TestSyntheticPDFsAreValidAndClearlyFictional(t *testing.T) {
	items, err := priceItems([]ItemSpec{
		{Description: "Mock line", UoM: "Unit", Qty: 1, UnitCost: 1_000, MarginEnabled: true, MarginPercent: 10},
	})
	if err != nil {
		t.Fatalf("price the fixture: %v", err)
	}
	totals, err := priceQuotationTotals(items, 0, "FTZ", "Goods", "")
	if err != nil {
		t.Fatalf("price totals: %v", err)
	}
	quotation := quotationPDF("150900001", 0, "PT Contoh (MOCK)", Fictional+" subject", items, totals)
	if err := store.ValidatePurchaseOrderPDF(quotation); err != nil {
		t.Fatalf("synthetic quotation PDF is not a valid PDF: %v", err)
	}
	for _, want := range []string{"FICTIONAL MOCK QUOTATION", "NOT A REAL CUSTOMER DOCUMENT", "SYNTHETIC PDF"} {
		if !strings.Contains(string(quotation), want) {
			t.Errorf("synthetic quotation PDF is missing %q", want)
		}
	}

	mixed := purchaseOrderMixedPDF("MOCK-CUSTOMER-PO-006", []store.PurchaseOrderItem{
		{Description: "Mock line", QuantityScaled: store.QuantityScale, UoM: "Lot", UnitPriceMinor: 1_000, NetMinor: 1_000},
	}, 1_000)
	if err := store.ValidatePurchaseOrderPDF(mixed); err != nil {
		t.Fatalf("synthetic mixed PDF is not a valid PDF: %v", err)
	}
	if !strings.Contains(string(mixed), "/Subtype /Image") {
		t.Error("the mixed fixture has no image-only page")
	}
	for _, want := range []string{"FICTIONAL MOCK CUSTOMER PURCHASE ORDER", "MIXED MANUAL/SCANNED", "SYNTHETIC"} {
		if !strings.Contains(string(mixed), want) {
			t.Errorf("synthetic mixed PDF is missing %q", want)
		}
	}
}
