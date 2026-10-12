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
	Fonts            fs.FS
	Theme            Theme
	Date             string
	DisplayPpn       string
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
	LeadTime        uint8
	PaymentTerm     uint8
	Validity        uint8
	DiscountPercent uint8
}

// Item is one quotation line as the quotation holds it. DisplayDesc is the HTML
// formatDescription produced, bullets included; deciding how to lay that out is
// the renderer's problem.
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
	// Disc is this row's own discount, preformatted ("10%", or "—" when the row
	// carries none), exactly as UnitPrice and Total are. Rendering it is the
	// renderer's problem; deciding what it says is not.
	Disc          string
	SubItems      []SubItem
	No            int
	Qty           uint32
	ShowSubPrices bool
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
