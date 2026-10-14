package gpdf

// Guards for the effective wording (issue #339).
//
// The engine used to bake every one of these strings. Now it reads them off the
// seam, and these tests are what prove it: a reworded label that reached the
// browser and not this engine would be a document that changed on one side
// only, which is the failure the label set exists to end. They need no browser
// and no PDF text extractor -- they read the layout's own runs.

import (
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"

	"vera-app/backend/labels"
)

// everyRun collects every text run the layout draws, across all pages.
func everyRun(e *Engine, in Input) []string {
	var out []string
	for _, pg := range e.layout(in) {
		for _, prim := range pg.prims {
			if r, ok := prim.(run); ok {
				out = append(out, r.text)
			}
		}
	}
	return out
}

// rewordedInput is the default document with every label the engine prints
// changed, so a baked string left anywhere shows up as a want that never
// appears.
func rewordedInput(t *testing.T) Input {
	in := sampleInput()
	set := labels.DefaultLabels()
	set.DocTitle = "PROPOSAL"
	set.RecipientHeading = "Prepared for"
	set.TermsHeading = "Conditions"
	set.MetaQuotationNo = "Reference"
	set.MetaIssueDate = "Issued"
	set.MetaValidUntil = "Expires"
	set.MetaRevision = "Rev"
	set.Subject = "Scope"
	set.ColumnNo = "Item"
	set.ColumnDescription = "Work"
	set.ColumnQty = "Quantity"
	set.ColumnUoM = "Unit"
	set.ColumnUnitPrice = "Price"
	set.ColumnAfterDisc = "Net Price"
	set.ColumnTotal = "Amount"
	set.TotalSubTotal = "Net of Discount"
	set.TotalDiscount = "Rebate"
	set.TotalPpn = "VAT 11%"
	set.TotalGrandTotal = "Total Payable"
	set.Closing = "Thank you for your business."
	set.VerifyScan = "Scan this code to check the document"
	set.VerifyCode = "or type the code"
	set.TermsPayment = "1. Payment is agreed for {payment} days."
	set.TermsValidity = "2. Valid for {validity} days."
	set.TermsLeadTime = "3. Lead time {leadtime}."
	set.TermsIncoterm = "4. Trading term {incoterm}"
	in.Labels = set
	in.Terms = set.Terms(labels.TermsValues{
		PaymentTerm: 90, Validity: 30, LeadTime: "60 Days", Incoterm: "FOB",
	})
	// The closing travels beside the labels on the engine's input, exactly as
	// the renderer maps it, so it is set from the same set.
	in.Closing = set.Closing
	// A discount row, a PPN row and a QR, so both conditional labels and both
	// verification captions are drawn rather than short-circuited.
	in.DiscountAmount = 1000
	in.DiscountPercent = "5"
	in.PpnAmount = 76592203
	in.QRPNG = qrPNG(t)
	return in
}

// qrPNG is a real QR code, so the verification block takes its exported branch
// and the two captions are drawn.
func qrPNG(t *testing.T) []byte {
	t.Helper()
	png, err := qrcode.Encode("https://verify.example.invalid/v/2e51f59464af4b00", qrcode.Medium, 256)
	if err != nil {
		t.Fatalf("encode fixture QR: %v", err)
	}
	return png
}

// TestEnginePrintsTheEffectiveLabels is the pin: every label the native
// renderer prints comes from the seam, and the values it prints are the ones
// the company chose.
func TestEnginePrintsTheEffectiveLabels(t *testing.T) {
	e := testEngine(t)
	runs := everyRun(e, rewordedInput(t))
	joined := strings.Join(runs, "\n")

	for _, want := range []string{
		"PROPOSAL", "Prepared for", "Conditions", "Reference", "Issued",
		"Expires", "Rev", "Scope", "Item", "Work", "Quantity", "Unit",
		"Price IDR", "Net Price IDR", "Amount IDR",
		"Net of Discount", "Rebate (5%)", "VAT 11%", "Total Payable",
		"Thank you for your business.",
		"Scan this code to check the document", "or type the code 2e51f59464af4b00",
		"1. Payment is agreed for 90 days.",
		"2. Valid for 30 days.",
		"3. Lead time 60 Days.",
		"4. Trading term FOB",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the engine did not print %q; it drew:\n%s", want, joined)
		}
	}

	// And none of the built-ins survives as a drawn run. The comparison is
	// exact per run rather than a substring scan, because a reworded value can
	// legitimately contain a built-in's letters ("Total Payable" contains
	// "Total") -- what must not survive is the built-in string being drawn.
	for _, unwanted := range []string{
		"QUOTATION", "Quotation to", "Terms and Conditions", "Quotation No.",
		"Issue Date", "Valid Until", "Revision", "Subject", "No.", "Description",
		"Qty.", "UoM", "Unit Price IDR", "After Disc. IDR", "Total IDR",
		"Sub Total", "Discount (5%)", "PPN 11%", "Grand Total",
		"Scan to verify this quotation", "or enter code 2e51f59464af4b00",
		"1. Payment terms agreed for this quotation is 90 days.",
		"2. This quotation is valid for 30 days.",
		"3. Lead time is 60 Days.",
		"4. Incoterms: FOB",
	} {
		if containsRun(runs, unwanted) {
			t.Errorf("the engine still draws the built-in %q; that string is baked "+
				"somewhere instead of read from the seam", unwanted)
		}
	}
}

// containsRun reports whether any single drawn run equals want exactly. A
// wrapped string is drawn as several runs, so it is checked whole and per line.
func containsRun(runs []string, want string) bool {
	for _, r := range runs {
		if r == want {
			return true
		}
	}
	return false
}

// TestEngineFallsBackToTheDefaults proves the empty-field rule holds in the
// renderer too: a seam that arrived with no labels (an old caller, a zero
// value) prints the built-in document rather than a blank one.
func TestEngineFallsBackToTheDefaults(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	in.Labels = labels.DocumentLabels{}
	runs := everyRun(e, in)
	joined := strings.Join(runs, "\n")
	for _, want := range []string{"QUOTATION", "Quotation to", "Subject", "Sub Total", "Grand Total"} {
		if !strings.Contains(joined, want) {
			t.Errorf("an empty label set did not fall back to %q", want)
		}
	}
}

// TestEngineColumnHeadersFollowTheLabels pins the table head specifically: it
// is the one place the labels compose with live data (the currency code), and
// the header row is where a baked string would be least visible.
func TestEngineColumnHeadersFollowTheLabels(t *testing.T) {
	e := testEngine(t)
	in := sampleInput()
	in.Labels = labels.DefaultLabels()
	in.Labels.ColumnUnitPrice = "Harga Satuan"
	in.Labels.ColumnTotal = "Jumlah"
	in.Currency = "USD"

	runs := everyRun(e, in)
	joined := strings.Join(runs, "\n")
	for _, want := range []string{"Harga Satuan USD", "Jumlah USD"} {
		if !strings.Contains(joined, want) {
			t.Errorf("table head does not print %q; it drew:\n%s", want, joined)
		}
	}
}

// TestEyebrowRuleFollowsTheTitle is the geometry half of a reworded title. The
// browser fills the eyebrow's row with a rule that starts after the label, so
// the rule has to move when the label does -- a fixed offset would leave it
// crossing a longer title or floating far from a shorter one.
func TestEyebrowRuleFollowsTheTitle(t *testing.T) {
	e := testEngine(t)
	base := sampleInput()

	ruleX := func(title string) float64 {
		in := base
		in.Labels = labels.DefaultLabels()
		in.Labels.DocTitle = title
		for _, pg := range e.layout(in) {
			for _, prim := range pg.prims {
				b, ok := prim.(box)
				if !ok || b.fill == "" || b.h != px(1) {
					continue
				}
				// The eyebrow rule is the 1px-filled box on the eyebrow's row.
				if b.y < e.derive(in).eyebrowTop || b.y > e.derive(in).eyebrowTop+px(19) {
					continue
				}
				return b.x
			}
		}
		t.Fatalf("no eyebrow rule drawn for title %q", title)
		return 0
	}

	defaultX := ruleX("QUOTATION")
	longerX := ruleX("PROFORMA INVOICE")
	shorterX := ruleX("QUOTE")
	if !(longerX > defaultX) {
		t.Errorf("a longer title did not push the rule right: default %.2f, longer %.2f", defaultX, longerX)
	}
	if !(shorterX < defaultX) {
		t.Errorf("a shorter title did not pull the rule left: default %.2f, shorter %.2f", defaultX, shorterX)
	}

	// The default title must keep the rule exactly where the browser measured
	// it, so a document nobody reworded is byte-identical: the constant is the
	// browser's own measurement of these nine letters, and this is what stops
	// the engine's own (different) measurement from moving the default rule.
	if got, want := defaultX, baseLabelRuleX(e); got != want {
		t.Errorf("the default title's rule sits at %.2f; want %.2f (the browser's measurement)", got, want)
	}
}

// baseLabelRuleX is the rule position the default document draws: the engine's
// own page metrics plus the pinned browser constant.
func baseLabelRuleX(e *Engine) float64 {
	m := e.derive(sampleInput())
	return m.innerX + eyebrowLabelW + px(12)
}

// TestSubjectLabelWrapsLikeTheBrowser pins the subject strip's height when a
// company types a long label: the browser wraps the label inside its fixed
// 110px track and grows the strip, and this engine has to agree or a reworded
// label moves the table down on one renderer only.
func TestSubjectLabelWrapsLikeTheBrowser(t *testing.T) {
	e := testEngine(t)
	short := sampleInput()
	tall := sampleInput()
	tall.Labels = labels.DefaultLabels()
	tall.Labels.Subject = "Description of the Works"

	shortH := e.derive(short).subjectH
	tallH := e.derive(tall).subjectH
	if tallH <= shortH {
		t.Errorf("a long subject label did not grow the strip: short %.2fpt, long %.2fpt", shortH, tallH)
	}

	// The label's own lines must be drawn, not clipped to one run.
	runs := everyRun(e, tall)
	if !containsRun(runs, "Description of") || !containsRun(runs, "the Works") {
		t.Errorf("the subject label did not wrap into drawn lines; it drew:\n%s",
			strings.Join(runs, "\n"))
	}

	// The default label is one line and must not change the default geometry.
	def := sampleInput()
	if got := e.derive(def).subjectH; got != shortH {
		t.Errorf("the default subject label changed the strip height: %.2f vs %.2f", got, shortH)
	}
}
