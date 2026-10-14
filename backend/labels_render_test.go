package backend

// Guards for the wording on the application side (issue #339).
//
// The seam carries the effective labels and the HTML template interpolates
// them, so what these tests pin is the plumbing: a company's edit reaches the
// document that renders, a cleared label falls back to the built-in default,
// and the preview the editor sees is the document that exports.

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/auth"
	"vera-app/backend/labels"
)

// prepareWithLabels prepares a quotation whose company identity carries the
// given label set, the way a signed-in desktop or a server-side Prepare would.
func prepareWithLabels(t *testing.T, set labels.DocumentLabels) *Quotation {
	t.Helper()
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	identity := DefaultCompanyIdentity()
	identity.Labels = set
	q.companyIdentity = &identity
	if err := q.Prepare(QuotationInput{
		Subject: "probe", RecipientName: "x", CustomerID: 2, Incoterm: "DAP",
		Validity: 30, PaymentTerm: 30, LeadTimeDays: 30,
		TaxZone: TaxZoneFTZ, TaxType: TaxTypeGoods,
		Items: []QuotationItemInput{{Description: "plain line", UoM: "Lot", Qty: 1, UnitPrice: 1000000}},
	}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return q
}

// TestRenderedQuotationPrintsTheEffectiveLabels proves a reworded label set
// reaches the HTML document the browser renders -- which is also the snapshot
// the archive keeps and the preview the editor sees.
func TestRenderedQuotationPrintsTheEffectiveLabels(t *testing.T) {
	set := labels.DefaultLabels()
	set.DocTitle = "PROPOSAL"
	set.RecipientHeading = "Prepared for"
	set.TermsHeading = "Conditions"
	set.MetaQuotationNo = "Reference"
	set.MetaRevision = "Rev"
	set.Subject = "Scope"
	set.ColumnNo = "Item"
	set.ColumnUnitPrice = "Price"
	set.TotalSubTotal = "Net of Discount"
	set.TotalGrandTotal = "Total Payable"
	set.Closing = "Thank you for your business."
	set.VerifyScan = "Scan this code to check the document"
	set.VerifyCode = "or type the code"
	set.TermsIncoterm = "4. Trading term {incoterm}"

	q := prepareWithLabels(t, set)
	// The verification captions live in the exported branch of the block, so
	// the preview needs a handle to reach them -- exactly as the desktop gets
	// one from the export.
	if err := q.SetVerifyURL("https://vera.example.test/v/9f3a4c2e1b7d4a8e"); err != nil {
		t.Fatalf("set verify URL: %v", err)
	}
	html, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	for _, want := range []string{
		`<h2 class="doc-eyebrow">PROPOSAL</h2>`,
		"<h3>Prepared for</h3>",
		"<h3>Conditions</h3>",
		"<dt>Reference</dt>",
		"<dt>Rev</dt>",
		`<div class="label">Scope</div>`,
		"<th>Item</th>",
		"<th>Price IDR</th>",
		"<span>Net of Discount</span>",
		"<span>Total Payable</span>",
		"Thank you for your business.",
		"Scan this code to check the document",
		`<span class="verify-code">or type the code 9f3a4c2e1b7d4a8e</span>`,
		"<p>4. Trading term DAP</p>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the preview HTML does not carry %q", want)
		}
	}

	for _, unwanted := range []string{
		"QUOTATION", "Quotation to", "Terms and Conditions", "Quotation No.",
		"or enter code", "Incoterms:", "We will be happy to supply",
	} {
		if strings.Contains(html, unwanted) {
			t.Errorf("the preview HTML still carries the built-in %q", unwanted)
		}
	}
}

// TestDefaultLabelsPrintWhatTheTemplateAlwaysPrinted pins the other side: with
// no overrides at all, the document is the one that existed before #339. Each
// expectation is the exact fragment the old template emitted, so a default
// that drifted fails here rather than changing every quotation quietly.
func TestDefaultLabelsPrintWhatTheTemplateAlwaysPrinted(t *testing.T) {
	q := prepareWithLabels(t, labels.DocumentLabels{})
	if err := q.SetVerifyURL("https://vera.example.test/v/9f3a4c2e1b7d4a8e"); err != nil {
		t.Fatalf("set verify URL: %v", err)
	}
	html, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	for _, want := range []string{
		`<h2 class="doc-eyebrow">QUOTATION</h2>`,
		"<h3>Quotation to</h3>",
		"<h3>Terms and Conditions</h3>",
		"<dt>Quotation No.</dt>",
		"<dt>Issue Date</dt>",
		"<dt>Valid Until</dt>",
		"<dt>Revision</dt>",
		`<div class="label">Subject</div>`,
		"<th>No.</th>",
		"<th>Description</th>",
		"<th>Qty.</th>",
		"<th>UoM</th>",
		"<th>Unit Price IDR</th>",
		"<th>After Disc. IDR</th>",
		"<th>Total IDR</th>",
		"<span>Sub Total</span>",
		"<span>Grand Total</span>",
		"<p>1. Payment terms agreed for this quotation is 30 days.</p>",
		"<p>2. This quotation is valid for 30 days.</p>",
		"<p>3. Lead time is 30 Days.</p>",
		"<p>4. Incoterms: DAP</p>",
		"We will be happy to supply information that you may need and trust " +
			"that you call on us to fill your order, which will receive our " +
			"prompt and careful attention.",
		"Scan to verify this quotation",
		"or enter code 9f3a4c2e1b7d4a8e",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the default document does not print %q; a built-in default drifted", want)
		}
	}
}

// TestClearedLabelFallsBackToTheBuiltIn is the rule the issue states: an empty
// field falls back to the built-in default, so a cleared label cannot print
// blank. An empty override and an absent one are the same thing.
func TestClearedLabelFallsBackToTheBuiltIn(t *testing.T) {
	set := labels.DefaultLabels()
	set.DocTitle = ""   // cleared
	set.Subject = "   " // cleared, with whitespace
	set.Closing = "Hi." // kept
	q := prepareWithLabels(t, set)
	html, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(html, `<h2 class="doc-eyebrow">QUOTATION</h2>`) {
		t.Error("a cleared title printed blank instead of falling back to the built-in")
	}
	if !strings.Contains(html, `<div class="label">Subject</div>`) {
		t.Error("a cleared subject label printed blank instead of falling back to the built-in")
	}
	if !strings.Contains(html, "Hi.") {
		t.Error("a kept label was replaced by the built-in")
	}
}

// TestRenderInputCarriesTheEffectiveLabels pins the seam: what the HTML
// interpolates is what the native renderer is handed, so the two cannot be
// given different documents by a change that only reached one of them.
func TestRenderInputCarriesTheEffectiveLabels(t *testing.T) {
	set := labels.DefaultLabels()
	set.DocTitle = "ESTIMATE"
	set.TermsPayment = "1. Payment is {payment} days."
	q := prepareWithLabels(t, set)

	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("render input: %v", err)
	}
	if in.Labels.DocTitle != "ESTIMATE" {
		t.Errorf("seam docTitle = %q; want ESTIMATE", in.Labels.DocTitle)
	}
	want := []string{
		"1. Payment is 30 days.",
		"2. This quotation is valid for 30 days.",
		"3. Lead time is 30 Days.",
		"4. Incoterms: DAP",
	}
	if len(in.Terms) != len(want) {
		t.Fatalf("seam carries %d terms; want %d", len(in.Terms), len(want))
	}
	for i := range want {
		if in.Terms[i] != want[i] {
			t.Errorf("seam term %d = %q; want %q", i+1, in.Terms[i], want[i])
		}
	}
}
