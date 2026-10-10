//go:build !chromedp

package render

import "testing"

// TestDefaultRendererIsGpdf pins the default engine.
//
// A build with no tag carries the native renderer. That is the shipping
// configuration, and this is what stops it drifting silently: the engine line is
// two files with no behavioural consequence to any other test in the tree, so
// without this the default could change in a commit whose tests all pass and be
// noticed only by a customer comparing a PDF against a browser.
//
// Swapping is deliberate, so this test exists to make it a loud one. If it fails,
// the renderer moved: that is sometimes exactly what you want, and it should be
// its own commit with its own review rather than a tag that changed on the way
// past.
func TestDefaultRendererIsGpdf(t *testing.T) {
	if got := Active.Name(); got != "gpdf" {
		t.Errorf("default renderer is %q, want gpdf.\n"+
			"Building with -tags chromedp selects the browser engine instead.\n"+
			"Changing which engine an untagged build carries changes what every "+
			"export produces.", got)
	}
}
