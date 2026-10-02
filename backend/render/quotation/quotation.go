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
	// HTML is the executed quotation template. The browser prints it as-is; the
	// native engine ignores it entirely. If these two ever disagree about the
	// document's content that is a bug in build(), not in a renderer.
	HTML string

	// Fonts carries the Inter faces the native renderer embeds in the PDF. It is
	// the application's own embedded asset filesystem, so the faces are the same
	// bytes the stylesheet feeds the browser renderer and the two cannot drift.
	//
	// It is an fs.FS rather than a directory path on purpose. A path is resolved
	// against the process working directory, which a desktop application does not
	// control: launched from Finder or a Start-menu shortcut the working
	// directory is wherever the user happened to be, and the export failed with
	// "open assets/inter-medium.ttf: no such file or directory" on a machine
	// where the file was present in the application directory all along. The
	// bytes are already in the binary, so there is nothing to look up.
	Fonts fs.FS

	// Brand theme. Four independent hex columns; empty falls back to the
	// stylesheet default.
	Theme Theme

	// LogoPNG and QRPNG are decoded artwork, nil when absent.
	LogoPNG []byte
	QRPNG   []byte

	CompanyName    string
	CompanyAddress string

	Subject          string
	RecipientName    string
	RecipientCompany string
	RecipientAddress string

	QuoteNumber string
	Date        string
	ValidUntil  string
	RevisionID  uint16

	// Terms are the numbered conditions. The renderer turns these into the
	// sentences it prints, because the wording is part of the page and each
	// renderer has to match the other.
	Validity        uint8
	PaymentTerm     uint8
	LeadTime        uint8
	Incoterm        string
	ServiceCategory string

	Items []Item

	DisplaySubTotal   string
	DisplayDiscount   string
	DisplayPpn        string
	DisplayGrandTotal string
	DiscountPercent   uint8

	VerificationCode string
}

// Item is one quotation line as the quotation holds it. DisplayDesc is the HTML
// formatDescription produced, bullets included; deciding how to lay that out is
// the renderer's problem.
type Item struct {
	No          int
	DisplayDesc string
	Qty         uint32
	UoM         string
	UnitPrice   string
	Total       string
}

// Theme is the four-column brand palette. The quotation and portal sets are
// independent; only the quotation set reaches a PDF.
type Theme struct {
	Deep   string
	Accent string
	Soft   string
	Ink    string
}
