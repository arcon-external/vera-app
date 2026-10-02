package render

import (
	"context"
	"testing"

	"vera-app/backend/render/chromedp"
	"vera-app/backend/render/gpdf"
	"vera-app/backend/render/quotation"
)

// The tests below import the renderer that render.go does not select. That is the
// point: an engine that is switched away from is not otherwise compiled, and
// this is what notices when it stops being a working renderer.

// TestDefaultRendererIsGpdf pins the engine line.
//
// The line is the entire swap mechanism, and this is what stops it drifting
// silently. A change that moves it is a one-line diff in a file with no
// behavioural consequence to any other test in the tree, so without this the
// default could change in a commit whose tests all pass and be noticed only by a
// customer comparing a PDF against a browser.
//
// Swapping is deliberate, so this test exists to make it a loud one. If it
// fails, the renderer moved: that is sometimes exactly what you want, and it
// should be its own commit with its own review rather than a line that changed
// on the way past.
func TestDefaultRendererIsGpdf(t *testing.T) {
	if got := Active.Name(); got != "gpdf" {
		t.Errorf("default renderer is %q, want gpdf.\n"+
			"chromedp.New() is the other end of the switch and is still there.\n"+
			"Moving this line changes what every export produces.", got)
	}
}

// TestBothRenderersAreReachable checks that the renderer that is not selected
// still compiles and constructs.
//
// It is not selected, so Go would not complain if it were broken, and a swap
// would then fail to build on a later day for a reason that has nothing to do
// with the swap. Both constructors are cheap and side-effect free, so naming
// both here is what keeps "is the other one still good?" a question with an
// answer.
func TestBothRenderersAreReachable(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    quotation.Named
		want string
	}{
		{"chromedp", chromedp.New(), "chromedp"},
		{"gpdf", gpdf.New(), "gpdf"},
	} {
		if tc.r == nil {
			t.Fatalf("%s renderer is nil", tc.name)
		}
		if got := tc.r.Name(); got != tc.want {
			t.Errorf("%s renderer reports %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestRenderersAreDistinct guards against both constructors returning the same
// thing, which would make the engine line a no-op that still looked like a
// working swap.
func TestRenderersAreDistinct(t *testing.T) {
	if chromedp.New().Name() == gpdf.New().Name() {
		t.Fatal("both constructors return the same renderer, so the engine line cannot mean anything")
	}
}

// TestBrowserRendererRejectsEmptyHTML checks the browser renderer's one
// precondition.
//
// A quotation whose template executed to nothing would otherwise reach Chrome
// and come back as a single blank page that looks exactly like a successful
// export of an empty document.
func TestBrowserRendererRejectsEmptyHTML(t *testing.T) {
	_, err := chromedp.New().Render(context.Background(), quotation.Input{})
	if err == nil {
		t.Fatal("expected an error for an empty document, got nil")
	}
}
