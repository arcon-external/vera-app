// Package render holds the application's choice of PDF renderer and nothing else.
//
// There are two renderers. chromedp drives a headless Chrome over the HTML
// template and is the one the application has always used. gpdf lays the same
// document out in Go and needs no browser. They are peers: two implementations
// of one job, not a new engine and an old fallback.
//
// ── THE ENGINE LINE ────────────────────────────────────────────────────────
//
// Everything below is the switch, and it is three lines: the two imports and
// Active. Both renderers are always compiled in, so the one you are not using
// costs nothing at runtime and nothing to restore later.
//
// To export with the native engine, name gpdf in the import block instead of
// chromedp and set Active to gpdf.New(). To go back, do the reverse.
//
// Go will not let you leave the unused renderer imported, which is deliberate:
// a swap that compiles is a swap you can finish, and the compiler is the only
// thing standing between "I meant to change the engine" and a build that fails
// halfway through with a confusing message.
//
// It is a compile-time change. There is no environment variable, no setting,
// and no way for a running application or a staff member to reach the renderer
// that is not named here.
//
// Do not add a third mechanism. An environment variable would put both engines
// within reach of a misconfigured launch, and this file's whole purpose is that
// swapping them is deliberate.
// ───────────────────────────────────────────────────────────────────────────

package render

import (
	"vera-app/backend/render/gpdf"
)

// Active is the renderer every export goes through.
//
// gpdf.New()     -- the native engine, and what this now uses.
//
//	chromedp.New() -- the browser, and what this always used. Reach for this if
//	                  a native render ever comes out wrong on a real document.
var Active = gpdf.New()
