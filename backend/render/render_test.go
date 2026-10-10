package render

import "testing"

// The engine selection lives in two tagged files, so this file must compile in
// both modes and assert only what is true of either. What is true of a specific
// engine belongs in render_gpdf_test.go or render_chromedp_test.go.

// TestActiveRendererIsUsable is the one assertion that holds whichever engine the
// build selected.
//
// It is worth having because a mis-tagged pair of engine files would leave Active
// nil, and that would surface as a panic inside an export rather than as a build
// failure -- the tag that failed to apply being exactly the thing nobody looks at.
func TestActiveRendererIsUsable(t *testing.T) {
	if Active == nil {
		t.Fatal("no renderer was selected; the engine files' build tags do not " +
			"cover this configuration, so an export would panic")
	}
	if Active.Name() == "" {
		t.Error("the active renderer has no name, so nothing can report which engine a build carries")
	}
}
