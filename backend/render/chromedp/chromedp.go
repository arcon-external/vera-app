//go:build chromedp

// Package chromedp renders a quotation by driving a headless Chrome: the
// executed HTML template becomes the document, and Chrome prints it to PDF.
//
// This is the original renderer and it stays. It is the only oracle the native
// engine has -- the engine's fidelity claims are all measurements against this
// output, and every one of them needs a real browser to check. Removing it would
// remove the ability to detect that the other renderer is wrong.
//
// What it costs is the reason the native engine exists: roughly 500ms per
// export, a Chrome process on the staff member's machine, and the 1.4MB of
// embedded fonts crossing a debug protocol on every render.
package chromedp

import (
	"context"
	"fmt"
	"time"

	cdpcore "github.com/chromedp/cdproto/cdp"
	cdp "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"vera-app/backend/render/quotation"
)

// printTimeout bounds a single export. Chrome is allowed to be slow to start and
// slow to lay out, but not indefinitely: a wedged browser should fail the
// export rather than hang the application with the file dialog still open.
const printTimeout = 30 * time.Second

// boolPtr adapts a bool literal to the *bool fields of PrintToPDFParams.
func boolPtr(b bool) *bool { return &b }

type renderer struct{}

// New returns the browser renderer. It reads no files and starts no process, so
// it is safe to construct unconditionally and costs nothing until an export
// actually happens.
func New() quotation.Named { return renderer{} }

// Name implements quotation.Named.
func (renderer) Name() string { return "chromedp" }

// Render prints in.HTML to PDF. Every other field on the input is ignored: the
// browser lays the document out from the template, and the native renderer's
// reading of the same quotation is a separate implementation to be compared
// against this one, not a source of truth for it.
func (r renderer) Render(ctx context.Context, in quotation.Input) ([]byte, error) {
	if in.HTML == "" {
		return nil, fmt.Errorf("render quotation pdf: template executed but produced empty output")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, printTimeout)
	defer cancelTimeout()

	var buf []byte
	err := chromedp.Do(
		ctx,
		chromedp.Navigate("about:blank"),
		func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
			tree, err := cdpcore.Call(ctx, t, cdp.GetFrameTree, cdpcore.Empty{})
			if err != nil {
				return chromedp.Void{}, err
			}

			_, err = cdpcore.Call(ctx, t, cdp.SetDocumentContent, cdp.SetDocumentContentParams{
				FrameID: tree.FrameTree.Frame.ID,
				HTML:    in.HTML,
			})
			return chromedp.Void{}, err
		},
		chromedp.WaitVisible(chromedp.CSS("table")),
		func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
			res, err := cdpcore.Call(ctx, t, cdp.PrintToPDF, cdp.PrintToPDFParams{
				PrintBackground:     boolPtr(true),
				DisplayHeaderFooter: boolPtr(false),
				PaperWidth:          8.27,
				PaperHeight:         11.69,
			})
			buf = res.Data
			return chromedp.Void{}, err
		},
	)
	if err != nil {
		return nil, fmt.Errorf("render quotation pdf: %w", err)
	}

	if len(buf) == 0 {
		return nil, fmt.Errorf("render quotation pdf: empty output")
	}

	return buf, nil
}
