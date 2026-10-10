//go:build chromedp

package render

import "vera-app/backend/render/chromedp"

// Active is the renderer every export goes through.
//
// This file compiles only with `-tags chromedp`, which is why the browser stack
// is absent from every other build: it needs a Chrome installation on the machine
// that renders, and the native engine does not.
//
// Reach for it when the HTML/CSS template has changed and the native layout has
// not caught up. The template is the fastest place to revise how a quotation
// looks, and this is the engine that renders it exactly as written.
var Active Renderer = chromedp.New()
