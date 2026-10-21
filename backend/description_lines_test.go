package backend

// Unsplit description lines (issue #485, feature 2).
//
// With split off, "- " bullet lines in the raw Description render as their own
// auto-numbered full-width sub-rows (1.1, 1.2, ...) under the main row -- the
// visual family of the split/prices-off sub-row. The main row keeps its own
// values; nothing reprices. This file pins the parser contract, the numbering,
// the main-values independence, the preview/HTML shape and the seam mapping.

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/render/quotation"
)

// prepareDescLines builds a two-item quotation whose first item carries two
// description lines. Item 1 is priced with margin and row discount so the
// independence assertions have figures to check against.
func prepareDescLines(t *testing.T) *Quotation {
	t.Helper()
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{
			{
				Description:     "Overhaul of pump P-101:\n- Replace mechanical seal\n- Realign coupling",
				UoM:             "Lot",
				UnitPrice:       10_000_000,
				Qty:             2,
				MarginEnabled:   true,
				MarginPercent:   25,
				DiscountEnabled: true,
				DiscountPercent: 10,
			},
			{
				Description: "Freight to site",
				UoM:         "Lot",
				UnitPrice:   1_500_000,
				Qty:         1,
			},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTimeDays:  45,
		Incoterm:      "FOB",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return q
}

// TestSplitDescriptionLinesContract pins the shared parser: trimmed lines
// starting with "- " are description lines; every other non-empty line joins
// the main description with single spaces, exactly as formatDescription does
// today.
func TestSplitDescriptionLinesContract(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		main  string
		lines []string
	}{
		{"plain single line", "Install lighting system", "Install lighting system", nil},
		{"plain multi-line", "Line one\nLine two", "Line one Line two", nil},
		{"main plus bullets", "Main scope\n- sensor 1\n- sensor 2", "Main scope", []string{"sensor 1", "sensor 2"}},
		{"bullets only", "- item a\n- item b", "", []string{"item a", "item b"}},
		{"windows line endings", "Scope\r\n- detail 1\r\n- detail 2", "Scope", []string{"detail 1", "detail 2"}},
		{"dangling dash treated as text", "- \n- valid item\n-  \n", "- -", []string{"valid item"}},
		{"blank lines ignored", "\n\nMain text\n\n- bullet\n\n", "Main text", []string{"bullet"}},
		{"leading dash without space is not a bullet", "Regular text - not a bullet", "Regular text - not a bullet", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			main, lines := splitDescriptionLines(tc.raw)
			if main != tc.main {
				t.Errorf("main = %q, want %q", main, tc.main)
			}
			if len(lines) != len(tc.lines) {
				t.Fatalf("lines = %q, want %q", lines, tc.lines)
			}
			for i := range lines {
				if lines[i] != tc.lines[i] {
					t.Errorf("line %d = %q, want %q", i, lines[i], tc.lines[i])
				}
			}
		})
	}
}

// TestDescriptionLinesNumberedAfterNo pins the numbering contract: each line
// is numbered from its item's final No, never from a stale pre-numbering
// index, and structured SubItems keep their own SubNo while DescriptionLines
// stays empty for them.
func TestDescriptionLinesNumberedAfterNo(t *testing.T) {
	q := prepareDescLines(t)

	first := q.Data.Items[0]
	if len(first.DescriptionLines) != 2 {
		t.Fatalf("DescriptionLines = %d, want 2", len(first.DescriptionLines))
	}
	if first.DescriptionLines[0] != (quotation.DescriptionLine{No: "1.1", Text: "Replace mechanical seal"}) {
		t.Errorf("line 0 = %+v, want {1.1 Replace mechanical seal}", first.DescriptionLines[0])
	}
	if first.DescriptionLines[1] != (quotation.DescriptionLine{No: "1.2", Text: "Realign coupling"}) {
		t.Errorf("line 1 = %+v, want {1.2 Realign coupling}", first.DescriptionLines[1])
	}

	// The plain second item carries no lines at all.
	if len(q.Data.Items[1].DescriptionLines) != 0 {
		t.Errorf("plain item DescriptionLines = %+v, want empty", q.Data.Items[1].DescriptionLines)
	}

	// The raw Description is untouched: parsing never rewrites what signs.
	if q.Data.Items[0].Description != "Overhaul of pump P-101:\n- Replace mechanical seal\n- Realign coupling" {
		t.Errorf("raw Description rewritten: %q", q.Data.Items[0].Description)
	}

	// DisplayDesc is the main-only HTML: no bullet list travels in it, or the
	// document would print every line twice.
	if got := string(q.Data.Items[0].DisplayDesc); strings.Contains(got, "<ul") || strings.Contains(got, "<li>") {
		t.Errorf("DisplayDesc carries bullets: %q", got)
	}
	if want := `<div class="desc-main">Overhaul of pump P-101:</div>`; string(q.Data.Items[0].DisplayDesc) != want {
		t.Errorf("DisplayDesc = %q, want %q", q.Data.Items[0].DisplayDesc, want)
	}
}

// TestDescriptionLinesKeepMainValues pins the pricing independence: a plain
// item with description lines still prices from its own qty, unit cost,
// margin and row discount, and the quotation totals are exactly what the
// same item without lines would produce.
func TestDescriptionLinesKeepMainValues(t *testing.T) {
	with := prepareDescLines(t)
	first := with.Data.Items[0]

	// Margin first (10,000,000 x 1.25 = 12,500,000), then 10% off:
	// list 12,500,000/unit, net 11,250,000/unit, qty 2.
	if first.ListUnitPrice != 12_500_000 {
		t.Errorf("list unit = %d, want 12500000", first.ListUnitPrice)
	}
	if first.UnitPrice != 11_250_000 {
		t.Errorf("net unit = %d, want 11250000", first.UnitPrice)
	}
	if first.Total != 22_500_000 {
		t.Errorf("total = %d, want 22500000", first.Total)
	}
	if first.OriginalTotal != 20_000_000 {
		t.Errorf("cost total = %d, want 20000000", first.OriginalTotal)
	}
	// Totals: sell subtotal 22,500,000 + 1,500,000; cost 20,000,000 + 1,500,000.
	if with.Data.SubTotal != 24_000_000 {
		t.Errorf("SubTotal = %d, want 24000000", with.Data.SubTotal)
	}
	if with.Data.OriginalSubTotal != 21_500_000 {
		t.Errorf("OriginalSubTotal = %d, want 21500000", with.Data.OriginalSubTotal)
	}

	// The same document without the bullet lines must price identically.
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{
			{
				Description:     "Overhaul of pump P-101:",
				UoM:             "Lot",
				UnitPrice:       10_000_000,
				Qty:             2,
				MarginEnabled:   true,
				MarginPercent:   25,
				DiscountEnabled: true,
				DiscountPercent: 10,
			},
			{Description: "Freight to site", UoM: "Lot", UnitPrice: 1_500_000, Qty: 1},
		},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTimeDays:  45,
		Incoterm:      "FOB",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if q.Data.SubTotal != with.Data.SubTotal || q.Data.OriginalSubTotal != with.Data.OriginalSubTotal ||
		q.Data.GrandTotal != with.Data.GrandTotal {
		t.Errorf("totals moved with the lines present: with %+v, without %+v",
			with.Data.SubTotal, q.Data.SubTotal)
	}
}

// TestTemplateRendersDescriptionLines pins the customer HTML: the main row
// keeps its own values, then one merged sub-row per line numbered 1.1/1.2
// with no price cells.
func TestTemplateRendersDescriptionLines(t *testing.T) {
	q := prepareDescLines(t)
	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, want := range []string{">1.1<", ">1.2<"} {
		if !strings.Contains(page, want) {
			t.Errorf("description line numbering %s missing", want)
		}
	}
	if got := strings.Count(page, `class="sub-row"`); got != 2 {
		t.Errorf("sub-row count = %d, want 2 (one per description line)", got)
	}
	// Each line prints inside the merged description cell, never as a priced
	// row: the figures appear once each, on the main row only.
	for _, want := range []string{"Replace mechanical seal", "Realign coupling"} {
		if got := strings.Count(page, want); got != 1 {
			t.Errorf("line text %q appears %d times, want exactly once", want, got)
		}
	}
	// The main row's own total (22,500,000) prints once; no line row carries
	// a price cell of its own.
	if got := strings.Count(page, "22.500.000"); got != 1 {
		t.Errorf("main total appears %d times, want exactly once (on the main row)", got)
	}
	// The old bullet-inside-cell grammar is gone from the plain item: no
	// list, so no line can print twice.
	if strings.Contains(page, `<ul class="desc-subitems">`) {
		t.Errorf("plain item still renders bullets inside its description cell")
	}
}

// TestCustomerPreviewCarriesDescriptionLines pins the preview DTO mapping:
// the review panel reads the same lines the document prints.
func TestCustomerPreviewCarriesDescriptionLines(t *testing.T) {
	q := prepareDescLines(t)
	prev, err := q.GetCustomerPreview()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(prev.Items) != 2 {
		t.Fatalf("preview items = %d, want 2", len(prev.Items))
	}
	got := prev.Items[0].DescriptionLines
	if len(got) != 2 || got[0] != (quotation.DescriptionLine{No: "1.1", Text: "Replace mechanical seal"}) ||
		got[1] != (quotation.DescriptionLine{No: "1.2", Text: "Realign coupling"}) {
		t.Errorf("preview DescriptionLines = %+v, want the numbered lines", got)
	}
	if len(prev.Items[1].DescriptionLines) != 0 {
		t.Errorf("plain preview item carries lines: %+v", prev.Items[1].DescriptionLines)
	}
}

// TestRenderInputCarriesDescriptionLines pins the renderer seam: the
// description lines reach both renderers with their numbers.
func TestRenderInputCarriesDescriptionLines(t *testing.T) {
	q := prepareDescLines(t)
	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderInput: %v", err)
	}
	if len(in.Items) != 2 {
		t.Fatalf("seam items = %d, want 2", len(in.Items))
	}
	got := in.Items[0].DescriptionLines
	if len(got) != 2 || got[0] != (quotation.DescriptionLine{No: "1.1", Text: "Replace mechanical seal"}) ||
		got[1] != (quotation.DescriptionLine{No: "1.2", Text: "Realign coupling"}) {
		t.Errorf("seam DescriptionLines = %+v, want the numbered lines", got)
	}
}
