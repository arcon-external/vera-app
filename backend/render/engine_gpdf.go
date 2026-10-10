//go:build !chromedp

package render

import "vera-app/backend/render/gpdf"

// Active is the renderer every export goes through.
//
// This file is the default: no build tag means the native engine, which lays the
// document out in Go and needs no browser on the machine that renders.
//
// Build with `-tags chromedp` to swap in the browser instead. See the package
// comment for what that does and does not bring with it.
var Active Renderer = gpdf.New()
