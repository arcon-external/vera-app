// Package render holds the application's choice of PDF renderer and nothing else.
//
// There are two renderers, and which one a binary carries is decided by a build
// tag rather than by editing this file.
//
//   - gpdf lays the document out in Go and needs no browser. It is the default.
//   - chromedp drives a headless Chrome over the same HTML/CSS template. It is
//     the engine to reach for when the template has changed and the native layout
//     has not caught up yet, and it is the oracle every claim about gpdf's
//     fidelity was measured against.
//
// ── THE ENGINE LINE ────────────────────────────────────────────────────────
//
//	build                    Active     chromedp dependency
//	go build                 gpdf       absent
//	go build -tags chromedp  chromedp   present
//
// The choice is two files, each behind its tag: engine_gpdf.go and
// engine_chromedp.go. Exactly one is compiled, so the engine you are not using
// costs nothing -- not a runtime branch, and not the dependency either. That is
// the point: chromedp brings a browser-automation stack with it, and a build that
// cannot use it should not carry it.
//
// The HTML/CSS template stays the source of truth for how the document looks
// either way. It is the fastest place to revise a layout, and the intended
// workflow is to change it there, render with chromedp to see the real thing, and
// let gpdf follow.
//
// gpdf is compiled in both modes, deliberately. The equivalence test that checks
// the two engines against each other needs both present, and that test is the
// thing that tells you whether a template change and the native layout still
// agree -- which is exactly the question being asked when the browser engine is
// in use.
//
// It is a compile-time choice. There is no environment variable, no setting, and
// no way for a running application or a staff member to reach the renderer that
// is not compiled in.
//
// Do not add a third mechanism. An environment variable would put both engines
// within reach of a misconfigured launch, and this file's whole purpose is that
// swapping them is deliberate.
// ───────────────────────────────────────────────────────────────────────────

package render

import "vera-app/backend/render/quotation"

// Renderer is the interface both engines satisfy. Active is declared in
// engine_gpdf.go and engine_chromedp.go, whichever the build selected.
type Renderer interface {
	quotation.Renderer
	quotation.Named
}
