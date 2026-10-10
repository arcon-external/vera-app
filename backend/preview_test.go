package backend

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

func testPreviewInput() QuotationInput {
	return QuotationInput{
		Items: []QuotationItemInput{
			{Description: "Item A", Qty: 2, UoM: "Lot", UnitPrice: 100000, MarginEnabled: true, MarginPercent: 20},
			{Description: "Item B", Qty: 1, UoM: "Unit", UnitPrice: 50000, MarginEnabled: true, MarginPercent: 20},
		},
		Subject:         "Test",
		RecipientName:   "Test",
		Validity:        30,
		PaymentTerm:     30,
		LeadTime:        30,
		CustomerID:      2,
		Incoterm:        "EXW",
		DiscountPercent: 10,
		TaxZone:         TaxZoneNonFTZ,
		TaxCategory:     TaxCategorySupplyInstall,
	}
}

// Cost 250.000 (2x100.000 + 1x50.000); margin 20% -> sell 300.000
// (2x120.000 + 1x60.000); discount 10% -> grand 270.000;
// tax 2% -> 5.400; after-tax customer grand 264.600; nett profit 39.600.
func TestGetCustomerPreviewMatchesSellData(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	prev, err := q.GetCustomerPreview()
	if err != nil {
		t.Fatalf("GetCustomerPreview failed: %v", err)
	}

	if len(prev.Items) != 2 {
		t.Fatalf("preview items = %d; want 2", len(prev.Items))
	}
	if prev.Items[0].UnitPrice != 120000 || prev.Items[0].Total != 240000 {
		t.Errorf("item 0 = %d/%d; want 120000/240000", prev.Items[0].UnitPrice, prev.Items[0].Total)
	}
	if prev.SubTotal != 300000 || prev.DiscountAmount != 30000 || prev.GrandTotal != 270000 {
		t.Errorf("totals = %d/%d/%d; want 300000/30000/270000", prev.SubTotal, prev.DiscountAmount, prev.GrandTotal)
	}
	if prev.SubTotal != q.Data.SubTotal || prev.GrandTotal != q.Data.GrandTotal {
		t.Error("customer preview diverges from PDF-bound QuotationData")
	}
}

func TestGetInternalBreakdown(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	bd, err := q.GetInternalBreakdown()
	if err != nil {
		t.Fatalf("GetInternalBreakdown failed: %v", err)
	}

	if bd.CostSubtotal != 250000 {
		t.Errorf("CostSubtotal = %d; want 250000", bd.CostSubtotal)
	}
	if bd.SellSubtotal != 300000 {
		t.Errorf("SellSubtotal = %d; want 300000", bd.SellSubtotal)
	}
	// No cost-side discount, so the cost grand is the cost subtotal.
	if bd.DiscountCost != 0 {
		t.Errorf("DiscountCost = %d; want 0", bd.DiscountCost)
	}
	if bd.CostGrand != 250000 {
		t.Errorf("CostGrand = %d; want 250000", bd.CostGrand)
	}
	if bd.GrandTotal != 270000 {
		t.Errorf("GrandTotal = %d; want 270000", bd.GrandTotal)
	}
	if bd.ProfitAmount != 20000 {
		t.Errorf("ProfitAmount = %d; want 20000 (270000-250000)", bd.ProfitAmount)
	}
	if bd.TaxAmount != 5400 {
		t.Errorf("TaxAmount = %d; want 5400", bd.TaxAmount)
	}
	// Nett reads the after-tax grand (270.000 - 5.400), not the customer
	// total, so nothing collected for the state lands in it.
	if bd.NettProfit != 14600 {
		t.Errorf("NettProfit = %d; want 14600", bd.NettProfit)
	}
	if len(bd.Items) != 2 {
		t.Fatalf("breakdown items = %d; want 2", len(bd.Items))
	}
	if bd.Items[0].CostUnit != 100000 || bd.Items[0].SellUnit != 120000 {
		t.Errorf("item 0 cost/sell unit = %d/%d; want 100000/120000", bd.Items[0].CostUnit, bd.Items[0].SellUnit)
	}
}

// TestInternalBreakdownCarriesSubItems proves the internal breakdown shows
// the composition and the blended margin the composition actually earned:
// total profit over total cost, rounded half-up and marked approximate when
// the division is not whole.
func TestInternalBreakdownCarriesSubItems(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{{
			Description:   "Package with mixed parts:",
			UoM:           "Lot",
			Qty:           1,
			ShowSubPrices: true,
			SubItems: []QuotationSubItemInput{
				// 100000 at 25% -> 125000; then 100000 x2 with no margin.
				// Cost 300000, sell 325000, profit 25000 -> 8.33% -> ≈8%.
				{Description: "First part", UoM: "EA", UnitCost: 100000, Qty: 1, MarginEnabled: true, MarginPercent: 25,
					SubSubItems: []QuotationSubSubItemInput{{Description: "Included hardware"}}},
				{Description: "Second part", UoM: "LS", UnitCost: 100000, Qty: 2},
			},
		}},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTime:      45,
		Incoterm:      "FOB",
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	bd, err := q.GetInternalBreakdown()
	if err != nil {
		t.Fatalf("GetInternalBreakdown failed: %v", err)
	}
	if len(bd.Items) != 1 {
		t.Fatalf("breakdown items = %d; want 1", len(bd.Items))
	}
	line := bd.Items[0]
	if len(line.SubItems) != 2 {
		t.Fatalf("sub-items = %d; want 2", len(line.SubItems))
	}
	first, second := line.SubItems[0], line.SubItems[1]
	if first.SubNo != "1.1" || second.SubNo != "1.2" {
		t.Errorf("sub numbers = %q/%q; want 1.1/1.2", first.SubNo, second.SubNo)
	}
	if first.OriginalUnitPrice != 100000 || first.MarginalizedUnitPrice != 125000 {
		t.Errorf("sub 1 cost/sell unit = %d/%d; want 100000/125000", first.OriginalUnitPrice, first.MarginalizedUnitPrice)
	}
	if second.Qty != 2 || second.OriginalTotal != 200000 || second.MarginalizedTotal != 200000 {
		t.Errorf("sub 2 qty/totals = %d/%d/%d; want 2/200000/200000", second.Qty, second.OriginalTotal, second.MarginalizedTotal)
	}
	if len(first.Details) != 1 || first.Details[0] != "Included hardware" {
		t.Errorf("sub 1 details = %v; want [Included hardware]", first.Details)
	}
	if line.MarginBlendedPercent != 8 || !line.MarginBlendedApprox || line.DisplayMarginBlended != "≈8%" {
		t.Errorf("blended = %d/%v/%q; want 8/true/≈8%%",
			line.MarginBlendedPercent, line.MarginBlendedApprox, line.DisplayMarginBlended)
	}
}

func TestPreviewRequiresPrepare(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if _, err := q.GetCustomerPreview(); err == nil {
		t.Error("expected GetCustomerPreview to fail before Prepare")
	}
	if _, err := q.GetInternalBreakdown(); err == nil {
		t.Error("expected GetInternalBreakdown to fail before Prepare")
	}
}

func TestRenderedQuotationHidesInternalValues(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	var body strings.Builder
	if err := q.Data.HtmlTemplate.Execute(&body, q.Data); err != nil {
		t.Fatalf("template execute failed: %v", err)
	}
	rendered := body.String()

	for _, forbidden := range []string{"Tax", "Nett", "Margin", "Profit", "Internal"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("rendered quotation leaks internal concept %q", forbidden)
		}
	}
	if !strings.Contains(rendered, "270.000") {
		t.Error("rendered quotation missing customer grand total 270.000")
	}
	if !strings.Contains(rendered, "120.000") || !strings.Contains(rendered, "60.000") {
		t.Error("rendered quotation is missing marginalized unit prices")
	}
	if strings.Contains(rendered, "100.000") || strings.Contains(rendered, "50.000") {
		t.Error("rendered quotation contains original unit prices")
	}
}

func TestGetQuotationPreviewHTMLUsesCustomerTemplate(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	previewHTML, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("GetQuotationPreviewHTML failed: %v", err)
	}

	if !strings.Contains(previewHTML, "<title>Quotation</title>") {
		t.Error("quotation preview does not use the customer template")
	}
	if !strings.Contains(previewHTML, "Revision") || !strings.Contains(previewHTML, ">00<") {
		t.Error("quotation preview is missing revision 00")
	}
	if !strings.Contains(previewHTML, "120.000") || !strings.Contains(previewHTML, "270.000") {
		t.Error("quotation preview is missing marginalized customer prices")
	}
	for _, forbidden := range []string{"Tax", "Nett", "Margin", "Profit", "Internal"} {
		if strings.Contains(previewHTML, forbidden) {
			t.Errorf("quotation preview leaks internal concept %q", forbidden)
		}
	}
}

func TestPreviewWithoutHandleShowsPlaceholder(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	previewHTML, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("GetQuotationPreviewHTML failed: %v", err)
	}
	if !strings.Contains(previewHTML, "Verification code is added when you export") {
		t.Error("preview without a handle does not show the verification placeholder")
	}
	if strings.Contains(previewHTML, `class="signature"`) {
		t.Error("preview renders the retired signature block")
	}
	if strings.Contains(previewHTML, `class="verify-qr"`) {
		t.Error("preview without a handle renders a QR code that would resolve to nothing")
	}
}

func TestPreviewWithHandleShowsVerificationCode(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if err := q.SetVerifyURL("https://vera.example.test/v/9f3a4c2e1b7d4a8e"); err != nil {
		t.Fatalf("SetVerifyURL failed: %v", err)
	}

	previewHTML, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("GetQuotationPreviewHTML failed: %v", err)
	}
	if !strings.Contains(previewHTML, `class="verify-qr"`) {
		t.Error("preview with a handle does not render the verification code")
	}
	if strings.Contains(previewHTML, "Verification code is added when you export") {
		t.Error("preview with a handle still shows the placeholder")
	}
}

func TestRenderedQuotationPPN(t *testing.T) {
	render := func(t *testing.T, zone, taxType, category string) string {
		t.Helper()
		q := newTestQuotation()
		q.SetAuthService(auth.StaffTestService())
		if err := Startup(q, context.Background()); err != nil {
			t.Fatalf("startup failed: %v", err)
		}
		in := testPreviewInput()
		in.TaxZone = zone
		in.TaxType = taxType
		in.ServiceCategory = category
		in.TaxCategory = ""
		if err := q.Prepare(in); err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}
		var body strings.Builder
		if err := q.Data.HtmlTemplate.Execute(&body, q.Data); err != nil {
			t.Fatalf("template execute failed: %v", err)
		}
		return body.String()
	}

	t.Run("non-FTZ goods show PPN and PPN-inclusive grand", func(t *testing.T) {
		// Sell 300.000, discount 10% -> 270.000; PPN 11% -> 29.700;
		// customer grand 299.700.
		rendered := render(t, TaxZoneNonFTZ, TaxTypeGoods, "")
		if !strings.Contains(rendered, "PPN 11%") {
			t.Error("non-FTZ goods quotation missing PPN row")
		}
		if !strings.Contains(rendered, "29.700") {
			t.Error("non-FTZ goods quotation missing PPN amount 29.700")
		}
		if !strings.Contains(rendered, "299.700") {
			t.Error("non-FTZ goods quotation missing PPN-inclusive grand 299.700")
		}
		// PPh22 is the internal service rate. It must never print: the customer
		// sees the price and, for non-FTZ goods, PPN -- nothing else.
		for _, forbidden := range []string{"Nett", "Margin", "Profit", "Internal", "withholding", "final", "PPh22", "PPH22"} {
			if strings.Contains(rendered, forbidden) {
				t.Errorf("rendered quotation leaks internal concept %q", forbidden)
			}
		}
	})

	t.Run("FTZ goods show no tax row", func(t *testing.T) {
		rendered := render(t, TaxZoneFTZ, TaxTypeGoods, "")
		if strings.Contains(rendered, "PPN") {
			t.Error("FTZ quotation must not show any tax row")
		}
		if !strings.Contains(rendered, "270.000") {
			t.Error("FTZ quotation missing grand total 270.000")
		}
	})

	t.Run("services show no tax row in any zone", func(t *testing.T) {
		for _, zone := range []string{TaxZoneFTZ, TaxZoneNonFTZ} {
			rendered := render(t, zone, TaxTypeServices, ServiceCategoryConstruction)
			if strings.Contains(rendered, "PPN") {
				t.Errorf("%s services quotation must not show any tax row", zone)
			}
		}
	})
}

func TestRenderedQuotationTitle(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	var body strings.Builder
	if err := q.Data.HtmlTemplate.Execute(&body, q.Data); err != nil {
		t.Fatalf("template execute failed: %v", err)
	}
	rendered := body.String()
	if !strings.Contains(rendered, `<h2 class="doc-eyebrow">QUOTATION</h2>`) {
		t.Error("rendered quotation missing QUOTATION eyebrow block")
	}
	eyebrowIdx := strings.Index(rendered, "doc-eyebrow")
	recipientIdx := strings.Index(rendered, "recipient-row")
	if eyebrowIdx < 0 || recipientIdx < 0 || eyebrowIdx > recipientIdx {
		t.Error("QUOTATION eyebrow must sit between the header and the info cards")
	}
}

// TestRenderedQuotationHidesPackageInternals extends the no-leak rule to
// package lines: a state-C rendering prints every sub's SELL price (that is
// the customer's itemisation) but never a cost, a margin amount, or the
// composed cost of the package. The figures are chosen so cost and sell differ
// in every position, which makes the presence checks discriminating rather
// than coincidental.
func TestRenderedQuotationHidesPackageInternals(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{{
			Description:   "Package with mixed parts:",
			UoM:           "Lot",
			Qty:           1,
			ShowSubPrices: true,
			SubItems: []QuotationSubItemInput{
				// Cost 1.234.000 at 25% -> sell 1.542.500, margin 308.500.
				// No printed string contains another number as a substring,
				// so every presence and absence check below is discriminating.
				{Description: "First part", UoM: "EA", UnitCost: 1_234_000, Qty: 1, MarginEnabled: true, MarginPercent: 25,
					SubSubItems: []QuotationSubSubItemInput{{Description: "Included mounting hardware"}}},
				{Description: "Second part", UoM: "LS", UnitCost: 2_222, Qty: 1},
			},
		}},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTime:      45,
		Incoterm:      "FOB",
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	var body strings.Builder
	if err := q.Data.HtmlTemplate.Execute(&body, q.Data); err != nil {
		t.Fatalf("template execute failed: %v", err)
	}
	rendered := body.String()

	// The customer's itemisation: both parts' sell prices are printed.
	for _, want := range []string{"1.542.500", "2.222"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered package is missing sell price %s", want)
		}
	}
	// The internals: part costs, the composed package cost, and every
	// margin-shaped figure must not appear anywhere.
	for _, forbidden := range []string{
		"Tax", "Nett", "Margin", "Profit", "Internal",
		"1.234.000", // margined part's cost
		"1.236.222", // composed package cost
		"308.500",   // margin amount
		"25%",       // margin rate
		"+Rp",       // margin-money phrasing
	} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("rendered package leaks internal value %q", forbidden)
		}
	}
}

// TestPreviewNoNilFieldsOnTheWire pins the JSON contract the desktop relies on:
// a Go nil slice marshals to null, and null where the client expects an array
// took the whole UI down (ReviewPanel read .length on it after Prepare). Every
// repeated field must therefore arrive as [] rather than null, even when empty.
func TestPreviewNoNilFieldsOnTheWire(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	internal, err := q.GetInternalBreakdown()
	if err != nil {
		t.Fatalf("GetInternalBreakdown failed: %v", err)
	}
	internalJSON, err := json.Marshal(internal)
	if err != nil {
		t.Fatalf("marshal internal breakdown: %v", err)
	}
	// Plain lines carry no sub-items; that is exactly the empty case.
	if strings.Contains(string(internalJSON), `"SubItems":null`) {
		t.Error(`internal breakdown serialized "SubItems":null; the desktop reads .length on it and the whole app blanks`)
	}
	if !strings.Contains(string(internalJSON), `"SubItems":[]`) {
		t.Error(`internal breakdown did not serialize "SubItems":[] for a plain line`)
	}

	customer, err := q.GetCustomerPreview()
	if err != nil {
		t.Fatalf("GetCustomerPreview failed: %v", err)
	}
	customerJSON, err := json.Marshal(customer)
	if err != nil {
		t.Fatalf("marshal customer preview: %v", err)
	}
	if strings.Contains(string(customerJSON), `"Items":null`) {
		t.Error(`customer preview serialized "Items":null`)
	}
}

// internalEconomics are the field names that describe what a quotation earns the
// company: per-line margin, the PPH22 withheld, and the nett profit.
//
// The list is spelled out rather than derived from InternalBreakdown, because
// the two structs legitimately share names -- GrandTotal, Items, DiscountPercent
// are all customer-facing. Only these belong to the operator alone.
var internalEconomics = []string{
	// What a line earned.
	"MarginAmount", "MarginPercent", "MarginEnabled",
	"DisplayMarginAmount", "DisplayMarginBlended", "MarginBlendedPercent",
	// What the state withholds, and what is left.
	"TaxAmount", "DisplayTax", "TaxRateBps", "TaxLabel",
	"ProfitAmount", "NettProfit", "DisplayProfit", "DisplayNett",
	"MarginalizedSubtotal", "MarginalizedDiscount", "MarginalizedGrand",
	"MarginalizedGrandAfterTax",
}

// TestCustomerPreviewCarriesNoInternalEconomics pins the boundary between the
// customer-facing view and the operator's.
//
// CustomerPreview was written in #28 as "sell prices only", with the internal
// economics deliberately on InternalBreakdown instead. #48 then added three nett
// totals to it as collateral, while adding the cost side the review screen
// needed, and nothing read them for a month. A boundary described only by a
// comment is not a boundary; this is the enforcement.
//
// PPN is deliberately absent from the list above. It is charged on top and
// printed on the customer's quotation, so PpnAmount is customer-facing, while
// TaxAmount (PPH22) is not. That distinction is the whole point of the rule.
func TestCustomerPreviewCarriesNoInternalEconomics(t *testing.T) {
	for _, target := range []any{CustomerPreview{}, CustomerPreviewItem{}} {
		typ := reflect.TypeOf(target)
		for _, name := range internalEconomics {
			if _, carried := typ.FieldByName(name); carried {
				t.Errorf("%s carries %s, which describes what the company earns. "+
					"It belongs on InternalBreakdown/InternalPreviewItem, where the "+
					"review panel reads it from.", typ.Name(), name)
			}
		}
	}
}

// TestCustomerPreviewKeepsTheCostSide is the other half of the same rule, and
// guards against over-correcting.
//
// ReviewPanel renders its "Unit before margin / Total before margin" table from
// CustomerPreview.Items, so these figures have a live consumer. Removing them to
// make the struct look purer would blank a table staff check the margin against.
func TestCustomerPreviewKeepsTheCostSide(t *testing.T) {
	item := reflect.TypeOf(CustomerPreviewItem{})
	for _, name := range []string{
		"Description", "No", "Qty", "UoM", "DisplayOriginalUnitPrice", "DisplayOriginalTotal",
	} {
		if _, carried := item.FieldByName(name); !carried {
			t.Errorf("CustomerPreviewItem lost %s; ReviewPanel renders its before-margin "+
				"table from it, so this removes a figure staff rely on rather than "+
				"tightening a boundary", name)
		}
	}
	preview := reflect.TypeOf(CustomerPreview{})
	for _, name := range []string{"Items", "DisplaySubTotal", "DisplayGrandTotal", "DiscountPercent"} {
		if _, carried := preview.FieldByName(name); !carried {
			t.Errorf("CustomerPreview lost %s, which the review panel renders", name)
		}
	}
}
