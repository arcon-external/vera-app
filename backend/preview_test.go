package backend

import (
	"context"
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
	q := &Quotation{}
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
	q := &Quotation{}
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
	if bd.DiscountCost != 25000 {
		t.Errorf("DiscountCost = %d; want 25000", bd.DiscountCost)
	}
	if bd.CostGrand != 225000 {
		t.Errorf("CostGrand = %d; want 225000", bd.CostGrand)
	}
	if bd.GrandTotal != 270000 {
		t.Errorf("GrandTotal = %d; want 270000", bd.GrandTotal)
	}
	if bd.ProfitAmount != 45000 {
		t.Errorf("ProfitAmount = %d; want 45000 (270000-225000)", bd.ProfitAmount)
	}
	if bd.TaxAmount != 5400 {
		t.Errorf("TaxAmount = %d; want 5400", bd.TaxAmount)
	}
	if bd.NettProfit != 39600 {
		t.Errorf("NettProfit = %d; want 39600", bd.NettProfit)
	}
	if len(bd.Items) != 2 {
		t.Fatalf("breakdown items = %d; want 2", len(bd.Items))
	}
	if bd.Items[0].CostUnit != 100000 || bd.Items[0].SellUnit != 120000 {
		t.Errorf("item 0 cost/sell unit = %d/%d; want 100000/120000", bd.Items[0].CostUnit, bd.Items[0].SellUnit)
	}
}

func TestPreviewRequiresPrepare(t *testing.T) {
	q := &Quotation{}
	q.SetAuthService(auth.StaffTestService())
	if _, err := q.GetCustomerPreview(); err == nil {
		t.Error("expected GetCustomerPreview to fail before Prepare")
	}
	if _, err := q.GetInternalBreakdown(); err == nil {
		t.Error("expected GetInternalBreakdown to fail before Prepare")
	}
}

func TestRenderedQuotationHidesInternalValues(t *testing.T) {
	q := &Quotation{}
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
	q := &Quotation{}
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

func TestRenderedQuotationPPN(t *testing.T) {
	render := func(t *testing.T, zone, taxType, category string) string {
		t.Helper()
		q := &Quotation{}
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
		for _, forbidden := range []string{"Nett", "Margin", "Profit", "Internal", "withholding", "final"} {
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
	q := &Quotation{}
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
	if !strings.Contains(rendered, `<h2 class="doc-title">QUOTATION</h2>`) {
		t.Error("rendered quotation missing QUOTATION title block")
	}
	titleIdx := strings.Index(rendered, "doc-title")
	recipientIdx := strings.Index(rendered, "recipient-row")
	if titleIdx < 0 || recipientIdx < 0 || titleIdx > recipientIdx {
		t.Error("QUOTATION title must sit between the header and the info cards")
	}
}
