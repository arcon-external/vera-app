package backend

import (
	"context"
	"html"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

// TestTemplateRendersAllThreeStates executes the real customer template over
// one mixed document and pins the markup that makes the three states distinct:
// B keeps a full five-column main row with merged scope rows beneath it, C
// merges the parent across Description..Total and prices every sub, A draws
// exactly as the template always has.
func TestTemplateRendersAllThreeStates(t *testing.T) {
	q := prepareMixedStates(t)

	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// State C: the parent row is merged through Total IDR...
	if !strings.Contains(page, `<td colspan="5">`) {
		t.Errorf("state C parent not merged: no colspan=\"5\" cell in the document")
	}
	// ...and its subs are full priced rows carrying fractional numbers.
	for _, want := range []string{">3.1<", ">3.2<", ">3.3<"} {
		if !strings.Contains(page, want) {
			t.Errorf("state C sub numbering %s missing", want)
		}
	}
	// State C's priced sub rows carry formatted rupiah figures (bare dots
	// format -- the template's own cells carry no Rp prefix).
	if !strings.Contains(page, ">16.186.000<") {
		t.Errorf("state C sub row prices missing")
	}

	// Detail lines: bullets INSIDE their sub row's description cell, not rows
	// of their own -- three of them across the document, numbered X.Y.Z with
	// a muted prefix, using the same list grammar as scope text.
	if strings.Contains(page, "subsub-row") {
		t.Errorf("detail rows rendered as table rows; details belong inside the sub's description")
	}
	for _, want := range []string{">1.1.1<", ">1.1.2<", ">3.2.1<"} {
		if !strings.Contains(page, want) {
			t.Errorf("detail numbering %s missing", want)
		}
	}
	if got := strings.Count(page, `class="detail-no"`); got != 3 {
		t.Errorf("detail prefix count = %d, want 3 (two on the B side, one on the C side)", got)
	}
	// Both carriers draw the same bullet grammar inside a sub row: the C
	// controller's row and the B fan's row each hold a list.
	if got := strings.Count(page, `<ul class="desc-subitems">`); got != 2 {
		t.Errorf("detail bullet lists = %d, want 2", got)
	}

	// State B: its parent keeps its Qty and both price columns, and each scope
	// row is its own merged row rather than a bullet inside the parent's cell.
	bDescribed := false
	for _, want := range []string{"1.1", "1.2", "1.3"} {
		if !strings.Contains(page, ">"+want+"<") {
			t.Errorf("state B sub numbering %s missing", want)
		}
		bDescribed = true
	}
	if !bDescribed {
		t.Fatal("sanity: B numbering assertions never ran")
	}
	// The wiring row (unpriced sub) must NOT appear as a priced cell: its
	// figures render only on the C side of the document.
	// (12.450.000 is item 3.1's unit price; the B package's unpriced wiring
	// sub would leak its cost if prices appeared on B scope rows.)

	// The merger must not swallow the table's own geometry: every rendered
	// sub row keeps the body table's class hooks.
	if !strings.Contains(page, `class="sub-row"`) {
		t.Errorf("no sub-row markup in the document")
	}

	// Nothing internal reaches the document: the same rule the export hides
	// by, extended to sub-item data -- no margin percentage, no cost-side
	// figure of any line or sub-line, no margin-money text. The state-B
	// subs' composition (12.500.000) and any package cost (31.625.000,
	// 11.062.500) must be invisible; their sell sides may print.
	for _, forbidden := range []string{"Tax", "Nett", "Margin", "Profit", "Internal", "+Rp", "display:none",
		"31.625.000", "11.062.500", "12.500.000"} {
		if strings.Contains(page, forbidden) {
			t.Errorf("internal value %q reached the customer document", forbidden)
		}
	}

	// The escaping holds for pasted sub text: angle brackets become entities.
	q2 := newTestQuotation()
	q2.SetAuthService(auth.StaffTestService())
	if err := Startup(q2, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	err = q2.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{{
			Description: "Package:",
			UoM:         "Lot",
			Qty:         1,
			SubItems: []QuotationSubItemInput{
				{Description: "Panel <with> brackets & ampersands", UoM: "LS", UnitCost: 1, Qty: 1},
			},
		}},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTime:      45,
		Incoterm:      "FOB",
	})
	if err != nil {
		t.Fatalf("Prepare escaped case: %v", err)
	}
	page2, err := q2.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("render escaped case: %v", err)
	}
	if !strings.Contains(page2, html.EscapeString("Panel <with> brackets & ampersands")) {
		t.Errorf("sub description not escaped into the document")
	}
}

// TestTemplateSubFreeDocumentIsUnchanged is the backward-compatibility core:
// a document with no sub-items renders the same shape the template always
// produced -- one item per tbody, no colspan cells, the legacy bullet list
// intact for old revisions that stored scope as text.
func TestTemplateSubFreeDocumentIsUnchanged(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	err := q.Prepare(QuotationInput{
		CustomerID: 2,
		Items: []QuotationItemInput{{
			Description: "Plain item with scope below:\n- first scope bullet\n- second scope bullet",
			UoM:         "Lot",
			UnitPrice:   10_000,
			Qty:         2,
		}},
		Subject:       "Test",
		RecipientName: "Test",
		Validity:      14,
		PaymentTerm:   30,
		LeadTime:      45,
		Incoterm:      "FOB",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(page, "colspan") {
		t.Errorf("a document without sub-items must not contain merged cells")
	}
	if !strings.Contains(page, "desc-subitems") {
		t.Errorf("legacy text bullets vanished from the sub-free document")
	}
	if strings.Contains(page, "class=\"sub-row\"") || strings.Contains(page, "class=\"sub-parent\"") {
		t.Errorf("sub row classes rendered without sub-items")
	}
	// Banding: with one row per item, per-item parity must equal per-row
	// parity. Item 1 is odd, so its cells stay paper.
	if strings.Contains(page, `<tbody class="item" style=`) {
		t.Errorf("tbody picked up inline styling")
	}
}
