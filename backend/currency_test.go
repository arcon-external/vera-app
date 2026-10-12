package backend

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

// TestPrepareFormatsInCompanyCurrency pins the Wave A formatting contract end
// to end: every Display money string respects the company currency's exponent
// and symbol, the item cells stay bare, and the totals carry the symbol —
// the template's existing shape, now per currency.
//
// The fixture is 1000000 minor units at qty 1: Rp1.000.000 in IDR, $10,000.00
// in USD. The same integer reads differently because the exponent is applied
// at display; the arithmetic never sees it.
func TestPrepareFormatsInCompanyCurrency(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	templates := &Quotation{}
	if err := Startup(templates, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	prepare := func(currencyCode string) *Quotation {
		t.Helper()
		q := newTestQuotation()
		q.SetAuthService(auth.StaffTestService())
		q.companyIdentity.Currency = currencyCode
		q.Data.HtmlTemplate = templates.Data.HtmlTemplate
		q.Data.CustomCSS = templates.Data.CustomCSS
		q.Data.LogoBase64 = templates.Data.LogoBase64
		if err := q.Prepare(QuotationInput{
			Subject: "currency probe", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
			Validity: 30, PaymentTerm: 30, LeadTime: 30,
			TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
			Items: []QuotationItemInput{
				{Description: "plain line", UoM: "Lot", Qty: 1, UnitPrice: 1000000},
			},
		}); err != nil {
			t.Fatalf("Prepare(%q): %v", currencyCode, err)
		}
		return q
	}

	idr := prepare("")
	if idr.Data.Currency != "IDR" {
		t.Errorf("empty currency normalized to %q; want IDR", idr.Data.Currency)
	}
	if got := idr.Data.Items[0].DisplayUnitPrice; got != "1.000.000" {
		t.Errorf("IDR cell = %q; want %q", got, "1.000.000")
	}
	if got := idr.Data.DisplayGrandTotal; got != "1.000.000" {
		t.Errorf("IDR grand = %q; want %q", got, "1.000.000")
	}

	usd := prepare("USD")
	if usd.Data.Currency != "USD" {
		t.Fatalf("currency = %q; want USD", usd.Data.Currency)
	}
	item := usd.Data.Items[0]
	if item.DisplayUnitPrice != "10,000.00" || item.DisplayTotal != "10,000.00" {
		t.Errorf("USD cells = %q/%q; want bare 10,000.00 in both",
			item.DisplayUnitPrice, item.DisplayTotal)
	}
	if usd.Data.DisplaySubTotal != "10,000.00" || usd.Data.DisplayGrandTotal != "10,000.00" {
		t.Errorf("USD totals = %q/%q; want bare 10,000.00 in both",
			usd.Data.DisplaySubTotal, usd.Data.DisplayGrandTotal)
	}
	// The stored integers are untouched: the exponent never reaches the
	// arithmetic, only the display.
	if usd.Data.SubTotal != 1000000 || usd.Data.GrandTotal != 1000000 {
		t.Errorf("USD integers = %d/%d; want 1000000/1000000",
			usd.Data.SubTotal, usd.Data.GrandTotal)
	}
	if usd.Data.CurrencyPrefix != "$" || usd.Data.CurrencySuffix != "" {
		t.Errorf("USD affixes = %q/%q; want $ and empty",
			usd.Data.CurrencyPrefix, usd.Data.CurrencySuffix)
	}

	page, err := usd.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(page, "<span>$10,000.00</span>") {
		t.Error("the USD totals carry no $ prefix in the customer document")
	}
	// Matched on the cell opening, not the bare letters: the embedded
	// logo and font blobs contain "Rp" as base64 noise.
	if strings.Contains(page, "<span>Rp") || strings.Contains(page, "<td>Rp") {
		t.Error("an Rp survived into a USD document")
	}

	// An unknown stored code renders as the default rather than breaking a
	// document: the second fallback, after the store's write-time validation.
	mystery := prepare("XX")
	if mystery.Data.Currency != "IDR" {
		t.Errorf("unknown currency normalized to %q; want IDR", mystery.Data.Currency)
	}
	if got := mystery.Data.Items[0].DisplayUnitPrice; got != "1.000.000" {
		t.Errorf("unknown-currency cell = %q; want the IDR figure", got)
	}
}

// TestInternalBreakdownFormatsInCompanyCurrency pins that the internal-only
// economics view uses the same exponent: staff checking a USD quotation see
// USD figures, not rupiah grouping.
func TestInternalBreakdownFormatsInCompanyCurrency(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	q.companyIdentity.Currency = "USD"
	if err := q.Prepare(QuotationInput{
		Subject: "currency probe", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
		Validity: 30, PaymentTerm: 30, LeadTime: 30,
		TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
		Items: []QuotationItemInput{
			{Description: "plain line", UoM: "Lot", Qty: 1, UnitPrice: 1000000},
		},
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	internal, err := q.GetInternalBreakdown()
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	if internal.DisplaySell != "10,000.00" || internal.DisplayCost != "10,000.00" {
		t.Errorf("internal sell/cost = %q/%q; want 10,000.00 in both",
			internal.DisplaySell, internal.DisplayCost)
	}
	customer, err := q.GetCustomerPreview()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if customer.DisplayGrandTotal != "10,000.00" {
		t.Errorf("customer grand = %q; want 10,000.00", customer.DisplayGrandTotal)
	}
}
