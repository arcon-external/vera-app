// Package quotation is the renderer-neutral view of a prepared quotation.
//
// It exists because the two renderers sit on either side of a package boundary
// that cannot be crossed both ways: render names the active renderer, and each
// renderer has to know what it is rendering. Putting the document model here, in
// a leaf package that imports nothing, lets both import it without a cycle.
//
// The model is a faithful copy of the quotation, not a rendering of it. Anything
// that is a decision about how the page looks — which lines a description wraps
// to, how a quantity is punctuated, what the closing sentence says — belongs to
// a renderer, not here. A renderer that ignored HTML below and the fields above
// would be reproducing the browser's output, not the document.
package quotation

import (
	"context"
	"io/fs"

	"vera-app/backend/labels"
)

// Renderer turns a prepared quotation into the customer-facing PDF bytes.
//
// The context is the caller's, not stored on the Input: the browser renderer
// derives a cancellable Chrome context from it and the native renderer ignores
// it, and a context parked in a data struct is the kind of thing that outlives
// the request it belonged to.
type Renderer interface {
	Render(ctx context.Context, in Input) ([]byte, error)
}

// Name identifies the renderer, for startup logging and for a test that asserts
// which one is selected. It is a property of the implementation rather than a
// string chosen at the call site, so it cannot drift from the code.
type Named interface {
	Renderer

	// Name is the renderer's short identifier, e.g. "chromedp" or "gpdf".
	Name() string
}

// Input is a prepared quotation as both renderers see it.
//
// The browser renderer reads HTML and nothing else. The native renderer reads
// everything except HTML, and lays the document out itself. Neither reads the
// other's fields, which is what keeps a change to one from silently altering the
// other.
type Input struct {
	Fonts      fs.FS
	Theme      Theme
	Date       string
	DisplayPpn string
	// DisplayLeadTime is the composed lead time sentence
	// ("2 Years, 10 Months, 4 Weeks, and 5 Days", "-" when none is stated).
	// It travels on the seam beside DisplayPpn for the same reason that one
	// does: the template prints it inside its HTML, and the native renderer
	// prints it in its own terms card, so both read one string rather than
	// each recomputing one (issue #377). The four numerics stay on the
	// application side -- no renderer needs them.
	DisplayLeadTime string
	// Labels is the document's effective wording: every customer-visible label
	// resolved to its override or its built-in default (see backend/labels).
	// It travels on the seam for the reason DisplayLeadTime does -- the two
	// renderers must print the same words, and the only way to guarantee that
	// is to hand them one string each. Before #339 both renderers baked their
	// own copy of every label, and the only thing keeping the copies equal was
	// a test that compared them.
	Labels labels.DocumentLabels
	// Terms is the four terms-and-conditions sentences with the label set's
	// placeholders already substituted ({payment}, {validity}, {leadtime},
	// {incoterm}). Substitution happens once, on the application side, for the
	// same reason: a renderer that substituted its own copy would be a
	// renderer reading the sentence differently.
	Terms []string
	// PpnAmount is the PPN charged on top of the price, in minor units. It is
	// the numeric the template's {{if .PpnAmount}} tests, so the native
	// renderer decides the row from the same value the browser does rather
	// than from the display string's emptiness (issue #377).
	PpnAmount        uint64
	VerificationCode string
	CompanyName      string
	CompanyAddress   string
	// Currency is the normalized company code. The browser renderer never
	// reads it (the HTML already carries the headers and totals); the native
	// renderer prints its headers and totals card from it.
	Currency          string
	Subject           string
	RecipientName     string
	RecipientCompany  string
	RecipientAddress  string
	QuoteNumber       string
	HTML              string
	ValidUntil        string
	DisplayGrandTotal string
	DisplayDiscount   string
	DisplaySubTotal   string
	// ShowLineDiscounts adds the item table's Disc. column, set by
	// applyRowDiscountColumn only when some row carries a discount. Both
	// renderers decide their column set from this, so the preview, the stored
	// snapshot and the PDF always print the same columns.
	ShowLineDiscounts bool
	// DiscountAmount is the raw whole-quotation discount, so the renderer can
	// omit the row entirely when there is none -- the same condition the HTML
	// template applies. Discounts are entered per line now, and a quotation
	// without one must not print "Discount (0%) Rp0".
	DiscountAmount  uint64
	ServiceCategory string
	Incoterm        string
	Items           []Item
	LogoPNG         []byte
	QRPNG           []byte
	RevisionID      uint16
	PaymentTerm     uint8
	Validity        uint8
	DiscountPercent uint8
}

// Item is one quotation line as the quotation holds it. DisplayDesc is the
// main-only HTML formatDescriptionMain produces; the description lines below
// travel separately so each renderer can lay them out as their own rows.
//
// SubItems and ShowSubPrices carry a modular package: the line's price is the
// sum of its parts, and ShowSubPrices decides whether the customer copy prints
// the parts with their prices (true) or as unpriced scope rows under a
// full-value main row (false). An item with parts never carries description
// bullets -- validation refuses both at once.
type Item struct {
	DisplayDesc string
	UoM         string
	UnitPrice   string
	Total       string
	// Disc is the net unit price -- the list price after the row's own
	// discount -- preformatted like UnitPrice and Total. Rendering it is the
	// renderer's problem; deciding what it says is not.
	Disc     string
	SubItems []SubItem
	// DescriptionLines are the plain item's "- " bullet lines, numbered X.Y
	// after the item's final No is known. They print as their own full-width
	// scope rows under the main row, which keeps its own values: the lines
	// are presentation, never pricing.
	DescriptionLines []DescriptionLine
	No               int
	Qty              uint32
	ShowSubPrices    bool
}

// DescriptionLine is one "- " bullet line of an unsplit description: its
// X.Y number and its plain text. It carries no values of its own, exactly
// like a scope row, which is what each renderer draws from it.
type DescriptionLine struct {
	No   string
	Text string
}

// SubItem is one priced part of a package line, as both renderers see it.
// Prices are the customer-facing display strings; Bullets are the part's
// detail lines, plain and positioned by order alone.
type SubItem struct {
	SubNo       string
	Description string
	UoM         string
	UnitPrice   string
	Total       string
	// Disc is the part's own discount, on the same terms as Item.Disc.
	Disc    string
	Bullets []string
	Qty     uint32
}

// Theme is the four-column brand palette. The quotation and portal sets are
// independent; only the quotation set reaches a PDF.
type Theme struct {
	Deep   string
	Accent string
	Soft   string
	Ink    string
}

// PriceSizePx returns the font size, in CSS pixels, for one price cell's
// text. Prices render at the document body size (11) until the text outgrows
// the column, stepping down to a floor of 8 (issue #398).
//
// The rule is length-based on purpose: money uses tabular figures, so a
// character count is the one width proxy both renderers can compute
// identically -- the browser and the native engine cannot share a pixel
// measurement, but they can share this. The thresholds are calibrated so
// the worst string at each step fits the price column's content width with
// margin, measured in both engines; the calibration is pinned by
// TestPriceSizeStepsFitTheColumn in backend/render/gpdf.
func PriceSizePx(text string) float64 {
	switch n := len(text); {
	case n <= 13:
		return 11
	case n <= 14:
		return 10
	case n <= 16:
		return 9
	default:
		return 8
	}
}
