//go:build chromedp

package render

import (
	"context"
	"testing"

	"vera-app/backend/render/chromedp"
	"vera-app/backend/render/gpdf"
	"vera-app/backend/render/quotation"
)

// This file compiles only with -tags chromedp, which is when the browser engine
// and its dependency are present at all.

// TestTaggedRendererIsChromedp pins the tagged engine, the mirror of
// TestDefaultRendererIsGpdf.
func TestTaggedRendererIsChromedp(t *testing.T) {
	if got := Active.Name(); got != "chromedp" {
		t.Errorf("renderer under -tags chromedp is %q, want chromedp.\n"+
			"The tag is meant to select the browser engine, and a build that "+
			"carries its dependency without using it is the worst of both.", got)
	}
}

// TestBothRenderersAreReachable checks that the engine which is *not* selected
// still compiles and constructs.
//
// This is the reason gpdf is compiled in both modes: the equivalence test that
// compares the two engines needs both present, and that comparison is the whole
// value of being able to switch. Naming both constructors here keeps "is the
// other one still good?" a question with an answer.
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
