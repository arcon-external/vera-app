package gpdf

// Turning the display list into PDF operators.
//
// Two primitives cannot be expressed through gpdf's render layer, and both are
// handled by rewriting the finished file:
//
//   - axial gradients. gpdf has no shading primitive at any layer, so the ribbon
//     and the meta decoration are painted with a sentinel colour and the real
//     /Shading is substituted afterwards. Painting the first stop instead would
//     be simpler and permanently flat, and the ribbon gradient is the most
//     visible thing on the page.
//   - clipping. The items table is clipped to a rounded outline so its zebra
//     rows and borders do not square off the corners. Rounded cards do not need
//     this: they are drawn as filled bezier outlines, which for an opaque fill is
//     equivalent to filling through a clip.
//
// Everything else is a rect, a path, an image or a text run.

import (
	"fmt"
	"strings"

	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/document/render"
	"github.com/gpdf-dev/gpdf/pdf"
)

// Baseline offsets, in points, measured from Chrome's own output.
//
// These are the one place where a hand-written layout cannot derive its answer
// from the stylesheet: the boxes all come from CSS, but a baseline is where a
// font's ascent lands inside a line box, and that depends on the font's own
// vertical metrics as well as the alignment context. Rather than guess per
// block, each context is pinned to the value the reference produces.
const (
	// bodyBaselineOffset is from a td's content top to the first baseline.
	bodyBaselineOffset = 9.0
	// subNumBaselineOffset is the same quantity for the sub rows' 10px number
	// cell. The cell's line box, half-leading and ascent all scale with the
	// font size, so it tracks the body value by 10/11 rather than needing a
	// fresh measurement.
	subNumBaselineOffset = 9.0 * 10.0 / 11.0
	// h1BaselineOffset is from the top-lead's content top to the h1 baseline.
	h1BaselineOffset = 19.5
	// addrBaselineOffset is within one address line box; addrPitch is the step
	// between successive lines.
	addrBaselineOffset = 9.0
	addrPitch          = 12.75
	// eyebrowBaselineOffset is within the eyebrow's line box. The eyebrow is a
	// flex container, so its baseline is not placed by block flow at all.
	eyebrowBaselineOffset = 9.75
	// centerBaselineOffset is from a flex container's vertical centre to the
	// baseline of a centred line, measured off the browser's own output for the
	// subject strip and the items table header. It is 6.41% of the font size --
	// the half-leading plus the ascent, less the half the line box sits above the
	// text -- and it is the same fraction at 11px and 10px, which is what makes it
	// a constant rather than a per-call number.
	centerBaselineOffset = 2.44

	// verifyLastBaseline is from the QR card's bottom edge up to the baseline of
	// the caption's last line, and it is measured rather than derived: the
	// caption carries a 2px bottom padding and a 9px line at 1.5, so the number
	// depends on the font's descent and half-leading, and those are properties of
	// the embedded face rather than of the stylesheet.
	verifyLastBaseline = 9.915

	// headBaselineOffset is from the header row's top to its labels' baseline.
	// The cells are  inside a 48px row, so the line box
	// is centred in the row's content box and the baseline follows from that --
	// but the value depends on the font's own ascent and half-leading, and the
	// browser's numbers for those are not recoverable from the stylesheet. This
	// is the number the browser puts there, and the header is the one place in
	// the document where a text row is centred in a cell rather than flowing.
	headBaselineOffset = 21.19
)

// The list marker: a 2.25pt disc sitting 9.75pt left of the item's text, which
// is where a browser puts an outside disc for this list's indent.
const (
	bulletSize = 3.0
	bulletGap  = 7.5
	// bulletCentre is how far the disc's centre sits above the text's baseline.
	bulletCentre = 2.6
)

// drawPage emits one page's primitives in paint order.
func (e *Engine) drawPage(r *render.PDFRenderer, pg *page, gradSent, clipSent [3]uint8) ([]gradient, error) {
	var grads []gradient
	for _, p := range pg.prims {
		switch v := p.(type) {
		case box:
			g, err := e.drawBox(r, v, gradSent, clipSent)
			if err != nil {
				return nil, err
			}
			if g != nil {
				grads = append(grads, *g)
			}
		case run:
			if err := e.drawRun(r, v); err != nil {
				return nil, err
			}
		case picture:
			if err := drawPicture(r, v); err != nil {
				return nil, err
			}
		}
	}
	return grads, nil
}

// drawBox paints one box, returning a pending gradient if it carries one.
func (e *Engine) drawBox(r *render.PDFRenderer, b box, gradSent, clipSent [3]uint8) (*gradient, error) {
	fill := colorOf(b.fill)
	stroke := colorOf(b.stroke)

	// A clipped or gradient rect is drawn with its per-render sentinel (see
	// sentinel.go): the post-pass locates its own fills by that colour, and
	// the chooser guarantees it collides with no palette role.
	clipped := b.clip != nil
	if clipped {
		fill = rgbOf(clipSent[0], clipSent[1], clipSent[2])
	}

	var g *gradient
	if b.gradient != nil {
		fill = rgbOf(gradSent[0], gradSent[1], gradSent[2])
		g = &gradient{
			x0: b.x, y0: b.y, w: b.w, h: b.h,
			c0: b.gradient.c0, c1: b.gradient.c1, pageH: PageH,
		}
	}
	if fill == nil && stroke == nil {
		return g, nil
	}

	if b.path != nil || b.radius > 0 {
		path := roundedPath(b.x, b.y, b.w, b.h, b.radius, b.radius, b.radius, b.radius)
		if b.path != nil {
			path = *b.path
		}
		if err := r.RenderPath(path, render.PathStyle{
			FillColor:   fill,
			StrokeColor: stroke,
			StrokeWidth: b.lw,
		}); err != nil {
			return nil, fmt.Errorf("rounded box: %w", err)
		}
	} else {
		if err := r.RenderRect(
			document.Rectangle{X: b.x, Y: b.y, Width: b.w, Height: b.h},
			render.RectStyle{
				FillColor:   fill,
				StrokeColor: stroke,
				StrokeWidth: b.lw,
			}); err != nil {
			return nil, fmt.Errorf("rect: %w", err)
		}
	}
	return g, nil
}

// drawRun places one text run.
//
// Every glyph is placed at its own x. That is not an optimisation and not a
// shortcut around kerning -- the shaping already applied the kerning, and the
// advance is what moves the pen. Placing each glyph explicitly means the
// rasteriser is told where every glyph starts rather than being left to
// RenderText derives its baseline as pageHeight-pos.Y-fontSize, so a baseline b
// measured downward from the page top maps to pos.Y = b - fontSize.
//
// A whole run goes out as one call, which emits one BT/Td/Tj. The glyphs inside
// it are then positioned by the reader from the font's /W array rather than from
// the text stream, and /W is rewritten to the font's exact advances before the
// file is closed — see post_widths.go. That makes the two routes agree to within
// a rounding error of the per-mille width, which is a ten-thousandth of a point,
// while saving a text object per glyph. The letterspaced eyebrow is the one run
// that cannot: extra space between glyphs is not something /W can express, so
// that one still places each glyph itself.
func (e *Engine) drawRun(r *render.PDFRenderer, ru run) error {
	if ru.text == "" {
		return nil
	}
	family := "inter"
	if ru.bold {
		family = "interb"
	}
	col := derefColor(colorOf(ru.color))

	if ru.tracking == 0 {
		return r.RenderText(ru.text,
			document.Point{X: ru.x, Y: ru.baseline - ru.size},
			document.Style{
				FontFamily: family,
				FontSize:   ru.size,
				Color:      col,
			})
	}

	// With tracking, glyph i sits one tracking step further right than the
	// untracked shaping put it.
	for i, g := range e.ShapeFor(ru.bold).Shape(ru.text, ru.size) {
		if g.text == "" {
			continue
		}
		if err := r.RenderText(g.text,
			document.Point{X: ru.x + g.dx + ru.tracking*float64(i), Y: ru.baseline - ru.size},
			document.Style{
				FontFamily: family,
				FontSize:   ru.size,
				Color:      col,
			}); err != nil {
			return fmt.Errorf("glyph %q: %w", g.text, err)
		}
	}
	return nil
}

// drawPicture places a raster asset.
func drawPicture(r *render.PDFRenderer, im picture) error {
	if len(im.data) == 0 {
		return nil
	}
	w, h := pngSize(im.data)
	return r.RenderImage(document.ImageSource{
		Data:   im.data,
		Format: document.ImagePNG,
		Width:  w,
		Height: h,
	}, document.Point{X: im.x, Y: im.y}, document.Size{Width: im.w, Height: im.h})
}

// roundedPath builds a rounded rectangle as gpdf path segments, with y measured
// downward from the page top like everything else in the layout.
//
// The corners use the same control-point offset a browser does, k = 0.5523r,
// because these corners have to agree with the ones Chrome draws. Radii are
// per-corner so a shape can be round at the top and square at the bottom, which
// is what a shape clipped by `border-radius: 10px 10px 0 0` looks like.
func roundedPath(x, y, w, h float64, tl, tr, br, bl float64) document.Path {
	r := [4]float64{tl, tr, br, bl}
	for i := range r {
		ew, eh := w, h
		if i%2 == 1 {
			ew, eh = h, w
		}
		r[i] = minF(r[i], minF(ew/2, eh/2))
	}
	k := func(v float64) float64 { return 0.5523 * v }
	left, right, top, bot := x, x+w, y, y+h

	pt := func(a, b float64) document.Point { return document.Point{X: a, Y: b} }
	seg := func(op document.PathOp, ps ...document.Point) document.PathSegment {
		return document.PathSegment{Op: op, Points: ps}
	}
	// Each corner is one cubic from the end of the preceding straight edge to the
	// start of the next, with its two control points laid off the corner's
	// tangent directions.
	corner := func(op document.PathOp, c1, c2, end document.Point) document.PathSegment {
		return seg(op, c1, c2, end)
	}

	segs := []document.PathSegment{seg(document.PathMoveTo, pt(left+r[3], top))}
	segs = append(segs, seg(document.PathLineTo, pt(right-r[1], top)))
	if r[1] > 0 {
		segs = append(segs, corner(document.PathCurveTo,
			pt(right-r[1]+k(r[1]), top), pt(right, top+r[1]-k(r[1])), pt(right, top+r[1])))
	}
	segs = append(segs, seg(document.PathLineTo, pt(right, bot-r[2])))
	if r[2] > 0 {
		segs = append(segs, corner(document.PathCurveTo,
			pt(right, bot-r[2]+k(r[2])), pt(right-r[2]+k(r[2]), bot), pt(right-r[2], bot)))
	}
	segs = append(segs, seg(document.PathLineTo, pt(left+r[0], bot)))
	if r[0] > 0 {
		segs = append(segs, corner(document.PathCurveTo,
			pt(left+r[0]-k(r[0]), bot), pt(left, bot-r[0]+k(r[0])), pt(left, bot-r[0])))
	}
	segs = append(segs, seg(document.PathLineTo, pt(left, top+r[3])))
	if r[3] > 0 {
		segs = append(segs, corner(document.PathCurveTo,
			pt(left, top+r[3]-k(r[3])), pt(left+r[3]-k(r[3]), top), pt(left+r[3], top)))
	}
	segs = append(segs, seg(document.PathClose))
	return document.Path{Segments: segs}
}

// discPath builds a filled circle, which is what `list-style: disc` renders.
//
// A circle cannot be expressed as a rounded rectangle: its radius equals half the
// diameter, so every straight edge collapses to zero length and the four corner
// curves meet at the same points, leaving the fill with a notch.
func discPath(cx, cy, d float64) *document.Path {
	r := d / 2
	k := 0.5523 * r
	pt := func(x, y float64) document.Point { return document.Point{X: x, Y: y} }
	seg := func(op document.PathOp, ps ...document.Point) document.PathSegment {
		return document.PathSegment{Op: op, Points: ps}
	}
	return &document.Path{Segments: []document.PathSegment{
		seg(document.PathMoveTo, pt(cx-r, cy)),
		seg(document.PathCurveTo, pt(cx-r, cy-k), pt(cx-k, cy-r), pt(cx, cy-r)),
		seg(document.PathCurveTo, pt(cx+k, cy-r), pt(cx+r, cy-k), pt(cx+r, cy)),
		seg(document.PathCurveTo, pt(cx+r, cy+k), pt(cx+k, cy+r), pt(cx, cy+r)),
		seg(document.PathCurveTo, pt(cx-k, cy+r), pt(cx-r, cy+k), pt(cx-r, cy)),
		seg(document.PathClose),
	}}
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// ---- page composition -----------------------------------------------------

// drawPageChrome draws everything above the items table: the ribbon gradient,
// the brand block, the eyebrow, the info cards, the subject strip and the table's
// header row. The card itself is added by the layout, which knows how far down
// the content on this page reaches.
func (e *Engine) drawPageChrome(p *page, in Input, m metrics) {
	e.drawRibbon(p, m)
	e.drawBrand(p, in, m)
	e.drawEyebrow(p, m)
	e.drawInfoRow(p, in, m)
	e.drawSubject(p, in, m)
}

// drawContinuationChrome resets a page that carries only more rows.
func (e *Engine) drawContinuationChrome(p *page, m metrics) {}

// cardFor is the page card and its border as display-list entries. It is a
// method because the card paints in the paper and edge roles, which live on
// the engine's resolved palette.
//
// The card is returned rather than drawn because its height depends on where the
// page's content ends, which is only known once the content has been placed, and
// it has to be painted underneath all of it.
//
// A browser closes the card on the last block's bottom edge: `.page` has no
// height, and the padding that would otherwise fill the rest of the page is
// discarded at the break. A fixed-height card leaves a strip of empty border
// under every page that does not fill, which on the second page of a short
// quotation is most of the page. The browser fragments a block's border box
// across pages, so the first fragment fills the page and only the last is
// content-height. Reproducing that is what keeps the left and right rules
// continuous down a multi-page quotation.
func (e *Engine) cardFor(p *page, m metrics, bottom float64) []any {
	h := bottom - m.cardY
	// Border rules are drawn as thin rects on the edges rather than as a stroked
	// rectangle, matching what the browser emits: a collapsed table border is
	// half a pixel wide on each side, and four explicit rules make that visible
	// in the corners.
	return []any{
		box{x: m.cardX, y: m.cardY, w: m.cardW, h: h, fill: e.pal.Paper},
		box{x: m.cardX, y: m.cardY, w: m.cardW, h: px(1), fill: e.pal.Edge},
		box{x: m.cardX, y: bottom - px(1), w: m.cardW, h: px(1), fill: e.pal.Edge},
		box{x: m.cardX, y: m.cardY, w: px(1), h: h, fill: e.pal.Edge},
		box{x: m.cardX + m.cardW - px(1), y: m.cardY, w: px(1), h: h, fill: e.pal.Edge},
	}
}

// drawRibbon paints the top gradient bar.
func (e *Engine) drawRibbon(p *page, m metrics) {
	// Square, not rounded: `.page` carries a 1px border and no border-radius, so
	// the gradient bar is a plain rectangle sitting inside it. Rounding it puts a
	// pale notch in each of the two top corners where the card's white shows
	// through, which is visible at any zoom.
	p.add(box{
		x: m.cardX + px(1), y: m.cardY + px(1),
		w: m.cardW - 2*px(1), h: px(ribbonH),
		gradient: &ramp{c0: e.pal.Deep, c1: e.pal.Accent},
	})
}

// drawBrand paints the company name, the address lines, the logo and the rule
// that closes the band.
func (e *Engine) drawBrand(p *page, in Input, m metrics) {
	top := m.cardY + px(1) + px(ribbonH) + px(leadPadYTop)

	// The heading is in a flex row beside an 86px logo, so it takes the rest of
	// the width and wraps inside it. A company name is set once by an
	// administrator, but it is a free-text field and a long one would otherwise
	// run off the right of the paper.
	h1W := m.innerW - px(86) - px(16)
	by := top + h1BaselineOffset
	for _, line := range e.Wrap(in.CompanyName, px(h1SizePx), h1W, false) {
		p.add(run{
			x: m.innerX, baseline: by,
			size: px(h1SizePx), text: line, bold: true, color: e.pal.Accent,
		})
		by += px(h1SizePx) * 1.05
	}
	// The address paragraph carries `max-width: 380px`, so each line of it wraps
	// inside that and the band grows a line rather than running off the page. The
	// lines are supplied one per entry and are joined before wrapping, because
	// that is one paragraph in the template and a wrap point can fall in any of
	// them.
	addrLines := e.Wrap(strings.Join(in.CompanyAddress, " "), px(leadPSizePx), px(brandMaxW), false)
	for i, line := range addrLines {
		p.add(run{
			x:        m.innerX,
			baseline: top + leadAddrFirstBaseline + float64(i)*addrPitch,
			size:     px(leadPSizePx), text: line, color: e.pal.Secondary,
		})
	}
	if len(in.LogoPNG) > 0 {
		size := px(86)
		p.add(picture{
			x: m.innerX + m.innerW - size, y: top,
			w: size, h: size, data: in.LogoPNG,
		})
	}

	// The band closes at whichever is taller, the text block or the logo.
	// `leadAddrFirstBaseline` is measured from the top of the band and already
	// includes the heading and its margin, so the extra lines are what the two
	// wrapped blocks add on top of it, not the whole stack again.
	headingLines := len(e.Wrap(in.CompanyName, px(h1SizePx), h1W, false))
	textBottom := top + leadAddrFirstBaseline +
		float64(maxInt(0, headingLines-1))*px(h1SizePx)*1.05 +
		float64(maxInt(0, len(addrLines)-1))*addrPitch + px(4)
	logoBottom := top + px(86)
	band := textBottom
	if logoBottom > band {
		band = logoBottom
	}
	p.add(box{
		x: m.cardX + px(1), y: band + px(leadPadYBot),
		w: m.cardW - 2*px(1), h: px(1), fill: e.pal.Rule,
	})
}

// leadAddrFirstBaseline is where the first address line's baseline sits below
// the top-lead's content top.
//
// The address is 12px at the inherited 1.45 line height, which is 17.4px, and the
// browser rounds its line box to the 18px it lays out. Carrying 17.4 through put
// the address a whole pixel above where the browser puts it, which showed as the
// lead's closing rule landing a pixel off the heading it belongs to.
const leadAddrFirstBaseline = 39.75

// drawEyebrow paints the QUOTATION label and the rule that fills the rest of
// its row.
func (e *Engine) drawEyebrow(p *page, m metrics) {
	top := m.eyebrowTop
	p.add(run{
		x: m.innerX, baseline: top + eyebrowBaselineOffset,
		size: px(eyebrowPx), text: "QUOTATION", bold: true,
		color: e.pal.Deep, tracking: px(eyebrowTrack * eyebrowPx),
	})
	// The rule starts one gap after the letterspaced label and runs to the
	// content's right edge, which is what `flex: 1` on the ::after does. It sits
	// centred in the eyebrow's line box, because the eyebrow is a flex
	// container with `align-items: center`.
	//
	// The label's width is the browser's, not this engine's measurement of the
	// same nine letters. The label is fixed text in a fixed template, so its width
	// is a constant like any other, and taking it from the browser puts the rule
	// where the browser's rule starts — a difference of a quarter of a point here
	// is two pixels of rule starting somewhere else.
	ruleX := m.innerX + eyebrowLabelW + px(12)
	p.add(box{
		x: ruleX, y: top + (eyebrowLineBox-px(1))/2,
		w: m.innerX + m.innerW - ruleX, h: px(1), fill: e.pal.Edge,
	})
}

// drawInfoRow paints the recipient card and the quote-meta stack.
func (e *Engine) drawInfoRow(p *page, in Input, m metrics) {
	top := m.contentTopY
	leftW := m.infoLeftW
	rightW := m.infoRightW
	rightX := m.innerX + leftW + px(metaGapX)
	h := m.infoH

	// Recipient card.
	p.add(box{
		x: m.innerX, y: top, w: leftW, h: h,
		fill: e.pal.Surface, stroke: e.pal.Edge, lw: px(1), radius: px(cardRadiusPx),
	})
	ix := m.innerX + px(1) + px(cardPadX)
	// The card's baselines step by the constants infoHeight is built from, so the
	// card is exactly as tall as it draws itself.
	by := top + px(1) + px(cardPadY) + cardPadTop
	p.add(run{x: ix, baseline: by, size: px(12), text: "Quotation to", bold: true, color: e.pal.Accent})
	by += cardH3ToName
	p.add(run{x: ix, baseline: by, size: px(13), text: in.RecipientName, bold: true, color: e.pal.Deep})
	by += cardNameToCpy
	p.add(run{x: ix, baseline: by, size: px(bodyPx), text: in.RecipientCompany, bold: true, color: e.pal.Ink})
	by += cardCpyToAddr
	// The template emits the "Address:" label unconditionally, so it appears
	// whether or not there is an address to put under it.
	p.add(run{x: ix, baseline: by, size: px(bodyPx), text: "Address:", bold: true, color: e.pal.Ink})
	// The address steps one line box from the label, not by `cardAfterAddr`.
	// That constant is how far the *last* baseline sits above the content's
	// bottom edge, which is a different distance entirely — three-quarters of a
	// point of descent rather than a whole line box. Using it here dropped the
	// first address line onto the label it belongs to.
	if in.RecipientAddress != "" {
		by += addrLineBox
		for _, line := range e.Wrap(in.RecipientAddress, px(bodyPx),
			leftW-2*px(1)-2*px(cardPadX), false) {
			p.add(run{x: ix, baseline: by, size: px(bodyPx), text: line, color: e.pal.Ink})
			by += addrLineBox
		}
	}

	// Quote-meta card: a two-column grid of label and value.
	//
	// The rows' line boxes are 11px of text at 1.45, which is 15.95px, and the
	// browser rounds each to the 16px it lays out rather than keeping the
	// fraction. Carrying 15.95 through makes the card 0.9pt short, and the card
	// is what the decoration below it is measured against: the leftover is
	// whatever the card does not use, so a card that is short leaves the
	// decoration a pixel too tall.
	metaH := e.metaHeight(in, m)
	p.add(box{
		x: rightX, y: top, w: rightW, h: metaH,
		fill: e.pal.Surface, stroke: e.pal.Edge, lw: px(1), radius: px(cardRadiusPx),
	})
	mx := rightX + px(1) + px(metaPadX)
	my := top + px(1) + px(metaPadY)
	valueX := rightX + rightW - px(1) - px(metaPadX)
	rows := e.metaRows(in)
	// The grid's value track takes whatever the label leaves, and a value that
	// does not fit wraps onto further lines in that track rather than running
	// over its label. The card grows by the lines it gains, which is what the
	// browser does: `auto` tracks are capped by the grid's width, so the extra
	// height goes into the card and the row, not off the side of the page.
	valueW := valueX - mx - e.Width(longestLabel(rows), px(bodyPx), true)
	for i, kv := range rows {
		lines := e.Wrap(kv[1], px(bodyPx), valueW, false)
		if len(lines) == 0 {
			lines = []string{""}
		}
		p.add(run{x: mx, baseline: my + bodyBaselineOffset, size: px(bodyPx), text: kv[0], bold: true, color: e.pal.Deep})
		// dd is right-aligned; measure back from the value column's right edge.
		for j, line := range lines {
			vw := e.Width(line, px(bodyPx), true)
			p.add(run{x: valueX - vw, baseline: my + float64(j)*metaLineBox + bodyBaselineOffset,
				size: px(bodyPx), text: line, bold: true, color: e.pal.Emphasis})
		}
		my += float64(len(lines)) * metaLineBox
		if i < len(rows)-1 {
			my += px(metaGapY)
		}
	}

	// The decoration takes whatever the recipient card is taller than the
	// metadata card and the gap between them. `meta-stack` is a flex column
	// whose last child flexes, and the stack is as tall as the grid row, which is
	// as tall as the taller of the two cards -- so the leftover is exactly that
	// difference, and it grows on its own when the recipient's address wraps to
	// another line.
	//
	// The height is a height, not a position: adding `top` to it here once drew a
	// box as tall as the whole card and let it run down over the table.
	if d := h - metaH - metaStackGap; d > 0 {
		p.add(box{
			x: rightX, y: top + metaH + metaStackGap, w: rightW, h: d,
			stroke: e.pal.Edge, lw: px(1), radius: px(cardRadiusPx),
			gradient: &ramp{c0: e.pal.Decor0, c1: e.pal.Decor1},
		})
	}
}

// drawSubject paints the labelled subject strip.
//
// The strip is a rounded card with `overflow: hidden`, so its label and value are
// clipped to the card's rounded interior rather than drawn as rounded shapes of
// their own. Drawing them as paths would also work, but the clip is what the
// browser does and it keeps the two fills flush against each other -- the label's
// right edge and the value's left edge are square, because neither is a corner.
func (e *Engine) drawSubject(p *page, in Input, m metrics) {
	y := m.subjectY + px(1) // inside the card's 1px border
	// The strip is as tall as its value wraps to, and the label is centred in it,
	// so a value of two lines makes the label a two-line-tall box.
	h := m.subjectH - 2*px(1)
	lblW := px(subjectLblPx)
	clip := even(m.innerX+px(1), y, m.innerW-2*px(1), h, px(cardRadiusPx))

	// The strip's own 1px border, drawn as a rounded outline around the fills.
	p.add(box{
		x: m.innerX, y: m.subjectY, w: m.innerW, h: m.subjectH,
		fill: e.pal.Surface, stroke: e.pal.Edge, lw: px(1), radius: px(cardRadiusPx),
	})
	p.add(box{x: m.innerX + px(1), y: y, w: lblW, h: h, fill: e.pal.Deep, clip: clip})
	p.add(box{x: m.innerX + px(1) + lblW, y: y, w: m.innerW - 2*px(1) - lblW, h: h,
		fill: e.pal.Surface, clip: clip})

	// The strip is `display: flex; align-items: center`, so both the label and
	// the value are centred on the strip's middle line rather than sitting on the
	// top of it. A single line's baseline is therefore the strip's centre plus
	// this offset, which is measured off the browser's own output; the label's
	// block is the strip's middle line, so it lands on the same place.
	midY := m.subjectY + m.subjectH/2

	lbl := "Subject"
	p.add(run{
		x:        m.innerX + px(1) + (lblW-e.Width(lbl, px(bodyPx), true))/2,
		baseline: midY + centerBaselineOffset,
		size:     px(bodyPx), text: lbl, bold: true, color: e.pal.Paper,
	})
	valX := m.innerX + px(1) + lblW + px(cardPadX)
	valW := subjectValueW(m)
	lines := e.Wrap(in.Subject, px(bodyPx), valW, true)
	// A value that wraps is a block centred on the strip, so its first line
	// starts half the block's height higher and the lines then step by the line
	// box.
	top := midY + centerBaselineOffset -
		float64(len(lines)-1)*px(bodyPx)*bodyLH/2
	for i, line := range lines {
		p.add(run{
			x:        valX,
			baseline: top + float64(i)*px(bodyPx)*bodyLH,
			size:     px(bodyPx), text: line, bold: true, color: e.pal.Emphasis,
		})
	}
}

// drawTableHead paints the items table's header row.
//
// The table's outer corners are rounded, which is why every fill in it carries
// the table's outline as a clip: without it the header bar and the zebra rows
// would square off the corners.
func (e *Engine) drawTableHead(p *page, m metrics, clip *clipRegion, top float64) {
	h := headRowH

	p.add(box{x: m.tableX, y: top, w: m.tableW, h: h, fill: e.pal.Deep, clip: clip})
	// The table's top edge. It is the header table's own border, and a collapsed
	// border is drawn per cell: a run of border between each pair of cell edges,
	// not one rule across the lot. The two outer runs are the table's own border
	// colour and the five inner ones too, because on this row the cell borders are
	// only on the right.
	for i := 0; i < m.cols.last; i++ {
		x := m.colX[i]
		if i > 0 {
			x += px(1)
		}
		p.add(box{x: x, y: top, w: m.colX[i+1] - x, h: px(1), fill: e.pal.Edge, clip: clip})
	}
	// Column separators. Every cell carries `border-right` and the last one has
	// none, so there is one fewer than there are columns, at the leading edges
	// of every column but the first.
	for i := 1; i < m.cols.last; i++ {
		p.add(box{x: m.colX[i], y: top, w: px(1), h: h, fill: e.pal.Rule, clip: clip})
	}
	// The header's outer edges are the table's border, not a cell's: the cells
	// carry `border-right` and the last one has none, so nothing of theirs reaches
	// the outside of the row.
	tableSides(p, m, clip, top, h, e.pal.Edge)
	labels := itemHeadLabels(m.cols)
	for i, lbl := range labels {
		p.add(run{
			x:        m.colX[i] + (m.colX[i+1]-m.colX[i]-e.Width(lbl, px(10), true))/2,
			baseline: top + headBaselineOffset,
			size:     px(10), text: lbl, bold: true, color: e.pal.Paper,
		})
	}
	// The rule that closes the header row.
	p.add(box{x: m.tableX, y: top + h, w: m.tableW, h: px(1), fill: e.pal.Rule, clip: clip})
}

// tableSides paints the table's left and right edges for one band of its height.
// The colour is passed in because the header's edges are the table's own border
// and the body's are the cells'.
func tableSides(p *page, m metrics, clip *clipRegion, top, h float64, colour string) {
	p.add(box{x: m.tableX, y: top, w: px(1), h: h, fill: colour, clip: clip})
	p.add(box{x: m.tableX + m.tableW - px(1), y: top, w: px(1), h: h, fill: colour, clip: clip})
}

// drawTableRow paints one body row's zebra fill, its cell rules and its text.
// drawTableRow paints one laid-out row block: the full-value main row, a merged
// package title row, an unpriced scope row, or a priced part row.
func (e *Engine) drawTableRow(p *page, m metrics, clip *clipRegion, top float64, rb rowBlock, isEven bool) {
	switch rb.kind {
	case rowParentTitle:
		e.drawMergedRow(p, m, clip, top, rb, isEven, parentPadTopPx, true)
	case rowScope:
		e.drawMergedRow(p, m, clip, top, rb, isEven, subRowPadPx, false)
	case rowPart:
		e.drawPartRow(p, m, clip, top, rb, isEven)
	default:
		e.drawMainRow(p, m, clip, top, rb, isEven)
	}
}

// drawBullets paints a bullet list: the disc marker on each item's first line,
// the text at textX, the 6px gap above the list and the 3px between items —
// exactly the box model rowHeight mirrors. Shared by main rows (description
// bullets), scope rows and part rows (their detail bullets).
func (e *Engine) drawBullets(p *page, textX, dy float64, subs []subLine, color string) {
	if len(subs) == 0 {
		return
	}
	markerX := textX - bulletGap - bulletSize
	dy += px(subGapPx)
	for i, sl := range subs {
		// The list's disc marker is drawn, not typed: `list-style: disc`
		// renders as a shape the browser positions itself, and a bullet
		// character in the text would be a glyph the document does not have.
		// Only the line that opens a list item carries one.
		if sl.first {
			p.add(box{
				x: markerX, y: dy - bulletCentre,
				w: bulletSize, h: bulletSize,
				fill: color, path: discPath(markerX+bulletSize/2, dy-bulletCentre+bulletSize/2, bulletSize),
			})
		}
		p.add(run{
			x: textX, baseline: dy, size: px(bodyPx),
			text: sl.text, color: color,
		})
		// The 3px margin sits between list items, so it is added on the way
		// into a new item rather than after every line.
		if i > 0 && sl.first {
			dy += px(liMarginPx)
		}
		dy += descLineH
	}
}

// drawMergedRow paints a row whose Description..Total IDR is one cell: the
// state-C package title, or a state-B scope row. Both keep the No. column
// (scope rows carry the part's X.Y there); neither draws internal column rules.
func (e *Engine) drawMergedRow(p *page, m metrics, clip *clipRegion, top float64, rb rowBlock, isEven bool, padTopPx float64, title bool) {
	bodyH := rb.height - px(1)

	if isEven {
		p.add(box{
			x: m.tableX + px(1), y: top,
			w: m.tableW - 2*px(1), h: bodyH,
			fill: e.pal.Surface, clip: clip,
		})
	}
	tableSides(p, m, clip, top, bodyH, e.pal.Rule)
	p.add(box{x: m.tableX, y: top + bodyH, w: m.tableW, h: px(1), fill: e.pal.Rule, clip: clip})
	// The No. cell's right edge, which is the merged cell's own left border in
	// the browser -- every cell is bordered, so the boundary is there on the
	// title and on every scope row, exactly as it is on a full row.
	p.add(box{x: m.colX[1], y: top, w: px(1), h: bodyH, fill: e.pal.Rule, clip: clip})

	// The number cell keeps its size and its centred number. Scope rows carry
	// the part number in the lighter sub-item treatment; the title row carries
	// the item number in body ink.
	if title {
		num := itoa(rb.item.No)
		p.add(run{
			x:        m.colX[0] + (m.colX[1]-m.colX[0]-e.Width(num, px(bodyPx), false))/2,
			baseline: centeredBlockTop(top, bodyH, px(padTopPx), 1) + bodyBaselineOffset,
			size:     px(bodyPx), text: num, color: e.pal.Ink,
		})
	} else {
		e.drawSubNumber(p, m, top, bodyH, px(padTopPx), rb.part.SubNo)
		// The rail hangs from the row's padding box, exactly as the ::before
		// pseudo-element does: full height, 2px, at its stylesheet offset.
		p.add(box{
			x: m.colX[1] + px(1) + px(railLeftPx), y: top + px(1),
			w: px(railWidthPx), h: bodyH - px(1),
			fill: e.pal.Rail, clip: clip,
		})
	}

	textColor := e.pal.Ink
	indent := float64(tdPadPx)
	if !title {
		textColor = e.pal.SubInk
		indent = subRowIndentPx
	}
	dx := m.colX[1] + px(1) + px(indent)
	dy := top + px(padTopPx) + bodyBaselineOffset
	for _, line := range rb.lines {
		p.add(run{x: dx, baseline: dy, size: px(bodyPx), text: line, color: textColor})
		dy += descLineH
	}
	// Scope rows carry their part's detail bullets inside the merged cell,
	// one indent further in, in the same faded ink as the part text.
	if !title {
		e.drawBullets(p, dx+px(subBulletIndentPx), dy, rb.subs, e.pal.SubInk)
	}
}

// centeredBlockTop is the engine's `vertical-align: middle`: the y of a text
// block's first line box when the block is centred in a cell's content box.
// A block at or taller than its box starts at the padding edge, which keeps
// single-line cells at exactly the position they had when every cell was
// top-aligned -- the fitted geometry of single-line rows does not move.
func centeredBlockTop(top, bodyH, padY, lineCount float64) float64 {
	inner := bodyH - 2*padY
	block := lineCount * descLineH
	offset := (inner - block) / 2
	if offset < 0 {
		offset = 0
	}
	return top + padY + offset
}

// drawSubNumber centres a sub row's X.Y number in the No. column, in the
// smaller, lighter type the stylesheet gives `tr.sub-row td:first-child`. The
// number sits in the middle of the row, like every other numeric cell.
func (e *Engine) drawSubNumber(p *page, m metrics, top, bodyH, padY float64, number string) {
	p.add(run{
		x:        m.colX[0] + (m.colX[1]-m.colX[0]-e.Width(number, px(subNumPx), false))/2,
		baseline: centeredBlockTop(top, bodyH, padY, 1) + subNumBaselineOffset,
		size:     px(subNumPx), text: number, color: e.pal.SubNo,
	})
}

// drawPartRow paints a state-C part: a full six-column row like a main row,
// drawn one step lighter, with the description indented past the rail and the
// part's detail bullets beneath it.
func (e *Engine) drawPartRow(p *page, m metrics, clip *clipRegion, top float64, rb rowBlock, isEven bool) {
	h := rb.height
	bodyH := h - px(1)

	if isEven {
		for i := 0; i < m.cols.last; i++ {
			p.add(box{
				x: m.colX[i] + px(1), y: top,
				w: m.colX[i+1] - m.colX[i] - px(1), h: bodyH,
				fill: e.pal.Surface, clip: clip,
			})
		}
	}
	for i := 1; i < m.cols.last; i++ {
		p.add(box{x: m.colX[i], y: top, w: px(1), h: bodyH, fill: e.pal.Rule, clip: clip})
	}
	tableSides(p, m, clip, top, bodyH, e.pal.Rule)
	p.add(box{x: m.tableX, y: top + bodyH, w: m.tableW, h: px(1), fill: e.pal.Rule, clip: clip})

	e.drawSubNumber(p, m, top, bodyH, px(subRowPadPx), rb.part.SubNo)

	// The rail, full height of the padding box.
	p.add(box{
		x: m.colX[1] + px(1) + px(railLeftPx), y: top + px(1),
		w: px(railWidthPx), h: bodyH - px(1),
		fill: e.pal.Rail, clip: clip,
	})

	// Description and bullets, both a step lighter and both indented past the
	// rail; the bullets sit one more indent into the description's content box.
	dx := m.colX[1] + px(1) + px(subRowIndentPx)
	bulletX := dx + px(subBulletIndentPx)
	dy := top + px(subRowPadPx) + bodyBaselineOffset
	for _, line := range rb.lines {
		p.add(run{x: dx, baseline: dy, size: px(bodyPx), text: line, color: e.pal.SubInk})
		dy += descLineH
	}
	e.drawBullets(p, bulletX, dy, rb.subs, e.pal.SubInk)

	// One renderer for every numeric cell in the row: each is centred in its
	// track, wrapped if it is wider than the track, and centred vertically in
	// the row. Only the ink differs, and only for the discount column, which the
	// stylesheet colours on its own. Sharing the geometry is what keeps that
	// column aligned with the numbers beside it -- drawing it separately is how
	// it came to sit differently from all of them.
	cell := func(i int, text, colour string) {
		// i < 0 means the column is not in this layout at all: the discount
		// column's text is "—" on a row that carries none, which is not empty,
		// so without this the call indexes colX[-1] and takes the export down.
		if i < 0 || text == "" {
			return
		}
		trackW := m.colX[i+1] - m.colX[i] - px(1) - 2*px(tdPadPx)
		lines := e.Wrap(text, px(bodyPx), trackW, false)
		cy := centeredBlockTop(top, bodyH, px(subRowPadPx), float64(len(lines))) + bodyBaselineOffset
		for _, line := range lines {
			p.add(run{
				x:        m.colX[i] + px(1) + px(tdPadPx) + (trackW-e.Width(line, px(bodyPx), false))/2,
				baseline: cy,
				size:     px(bodyPx), text: line, color: colour,
			})
			cy += descLineH
		}
	}
	cell(m.cols.qty, rb.part.Qty, e.pal.SubInk)
	cell(m.cols.uom, rb.part.UoM, e.pal.SubInk)
	cell(m.cols.unit, rb.part.UnitPrice, e.pal.SubInk)
	cell(m.cols.disc, rb.part.Disc, e.pal.SubNo)
	cell(m.cols.total, rb.part.Total, e.pal.SubInk)
}

// drawMainRow paints one full-value row: an item's own row, and the state-B
// package's main row, which carries the rolled-up values.
func (e *Engine) drawMainRow(p *page, m metrics, clip *clipRegion, top float64, rb rowBlock, isEven bool) {
	h := rb.height

	// The closing rule is drawn inside this row rather than at its bottom edge,
	// because the next row's fill is laid down afterwards and a rule sitting on
	// the boundary between them would be painted over. A browser draws a
	// collapsed border on top of the row backgrounds, which is the same ordering
	// by a different route: put the border inside the row whose background it
	// separates, and no later fill can reach it.
	bodyH := h - px(1)
	if isEven {
		for i := 0; i < m.cols.last; i++ {
			p.add(box{
				x: m.colX[i] + px(1), y: top,
				w: m.colX[i+1] - m.colX[i] - px(1), h: bodyH,
				fill: e.pal.Surface, clip: clip,
			})
		}
	}
	// Cell rules.
	for i := 1; i < m.cols.last; i++ {
		p.add(box{x: m.colX[i], y: top, w: px(1), h: bodyH, fill: e.pal.Rule, clip: clip})
	}
	tableSides(p, m, clip, top, bodyH, e.pal.Rule)
	p.add(box{x: m.tableX, y: top + bodyH, w: m.tableW, h: px(1), fill: e.pal.Rule, clip: clip})

	// The row number, quantity, unit and both money columns are centred in
	// their cells; only the description is left-aligned.
	num := itoa(rb.item.No)
	p.add(run{
		x:        m.colX[0] + (m.colX[1]-m.colX[0]-e.Width(num, px(bodyPx), false))/2,
		baseline: centeredBlockTop(top, bodyH, px(tdPadPx), 1) + bodyBaselineOffset,
		size:     px(bodyPx), text: num, color: e.pal.Ink,
	})

	// Description: wrapped main text, then the bullet list.
	dx := m.colX[1] + px(tdPadPx)
	subTextX := dx + px(subIndentPx)
	dy := top + px(tdPadPx) + bodyBaselineOffset
	for _, line := range rb.lines {
		p.add(run{x: dx, baseline: dy, size: px(bodyPx), text: line, color: e.pal.Ink})
		dy += descLineH
	}
	e.drawBullets(p, subTextX, dy, rb.subs, e.pal.Ink)

	// One renderer for every numeric cell in the row: each is centred in its
	// track, wrapped if it is wider than the track, and centred vertically in
	// the row. Only the ink differs, and only for the discount column, which the
	// stylesheet colours on its own. Sharing the geometry is what keeps that
	// column aligned with the numbers beside it -- drawing it separately is how
	// it came to sit differently from all of them.
	cell := func(i int, text, colour string) {
		// i < 0 means the column is not in this layout at all: the discount
		// column's text is "—" on a row that carries none, which is not empty,
		// so without this the call indexes colX[-1] and takes the export down.
		if i < 0 || text == "" {
			return
		}
		trackW := m.colX[i+1] - m.colX[i] - px(1) - 2*px(tdPadPx)
		lines := e.Wrap(text, px(bodyPx), trackW, false)
		cy := centeredBlockTop(top, bodyH, px(tdPadPx), float64(len(lines))) + bodyBaselineOffset
		for _, line := range lines {
			p.add(run{
				x:        m.colX[i] + px(1) + px(tdPadPx) + (trackW-e.Width(line, px(bodyPx), false))/2,
				baseline: cy,
				size:     px(bodyPx), text: line, color: colour,
			})
			cy += descLineH
		}
	}
	cell(m.cols.qty, rb.item.Qty, e.pal.Ink)
	cell(m.cols.uom, rb.item.UoM, e.pal.Ink)
	cell(m.cols.unit, rb.item.UnitPrice, e.pal.Ink)
	cell(m.cols.disc, rb.item.Disc, e.pal.SubNo)
	// The Total column is centred but not bold: the stylesheet gives every
	// numeric cell the same treatment and only the header row carries a weight.
	cell(m.cols.total, rb.item.Total, e.pal.Ink)
}

// drawFooterCards paints the terms and totals cards, the first of the three
// trailer blocks. Each block is placed independently by the page assembler so
// the trailer can fragment across pages the way the browser's grid does.
func (e *Engine) drawFooterCards(p *page, in Input, m metrics, top float64) {
	tncW := m.footerTncW
	totW := m.footerTotW
	totX := m.innerX + tncW + px(footGapPx)
	rows := e.totalsRows(in)

	// Terms and conditions.
	p.add(box{
		x: m.innerX, y: top, w: tncW, h: m.footerTncH,
		fill: e.pal.Soft, stroke: e.pal.Edge, lw: px(1), radius: px(cardRadiusPx),
	})
	tnx := m.innerX + px(1) + px(footPadPx)
	ty := top + px(1) + px(footPadPx)
	p.add(run{x: tnx, baseline: ty + addrBaselineOffset,
		size: px(12), text: "Terms and Conditions", bold: true, color: e.pal.Deep})
	ty += px(12)*bodyLH + px(tncMarginH3)
	for i, term := range in.Terms {
		for _, line := range e.Wrap(term, px(bodyPx), tncW-2*px(1)-2*px(footPadPx), false) {
			p.add(run{x: tnx, baseline: ty + bodyBaselineOffset, size: px(bodyPx),
				text: line, color: e.pal.Ink})
			ty += px(bodyPx) * bodyLH
		}
		if i < len(in.Terms)-1 {
			ty += px(tncPPGap)
		}
	}

	// Totals.
	p.add(box{
		x: totX, y: top, w: totW, h: m.footerTotalsH,
		fill: e.pal.Surface, stroke: e.pal.Edge, lw: px(1), radius: px(cardRadiusPx),
	})
	// The card is a grid of auto rows inside a box the sibling terms card
	// stretches taller than their content, and `align-content: stretch` -- the
	// grid default -- shares that surplus equally across the rows. Drawing at
	// the natural pitch would bunch the lines at the top of a taller card; the
	// browser spreads them, and the row pitch is what a reader sees.
	ry := top + px(1) + px(footPadPx)
	labelX := totX + px(1) + px(footPadPx)
	valueX := totX + totW - px(1) - px(footPadPx)
	innerH := m.footerCardsH - 2*px(1) - 2*px(footPadPx)
	natural := make([]float64, len(rows))
	naturalTotal := 0.0
	for i, rw := range rows {
		if rw.grand {
			natural[i] = px(12) * bodyLH
		} else {
			natural[i] = px(bodyPx) * bodyLH
		}
		naturalTotal += natural[i]
	}
	if len(rows) > 1 {
		naturalTotal += px(totalsGap) * float64(len(rows)-1)
	}
	stretch := 0.0
	if len(rows) > 0 && innerH > naturalTotal {
		stretch = (innerH - naturalTotal) / float64(len(rows))
	}
	for i, rw := range rows {
		if i > 0 {
			ry += px(totalsGap)
		}
		if rw.grand {
			// The rule is the grand row's `border-top`, so it runs the width of
			// the row's own content box — inside the card's padding, not merely
			// inside its border. Insetting it by the border alone drew it wider
			// than the card's text, edge to edge across the padding.
			p.add(box{
				x: totX + px(1) + px(footPadPx), y: ry,
				w: totW - 2*(px(1)+px(footPadPx)), h: px(2), fill: e.pal.Deep,
			})
			ry += px(2) + px(grandPadTop)
			p.add(run{x: labelX, baseline: ry + bodyBaselineOffset, size: px(12),
				text: rw.label, bold: true, color: e.pal.Deep})
			vw := e.Width(rw.value, px(12), true)
			p.add(run{x: valueX - vw, baseline: ry + bodyBaselineOffset, size: px(12),
				text: rw.value, bold: true, color: e.pal.Deep})
			ry += natural[i] + stretch
			continue
		}
		ink := e.pal.Emphasis
		if rw.colour != "" {
			ink = rw.colour
		}
		p.add(run{x: labelX, baseline: ry + bodyBaselineOffset, size: px(bodyPx),
			text: rw.label, bold: true, color: ink})
		vw := e.Width(rw.value, px(bodyPx), true)
		p.add(run{x: valueX - vw, baseline: ry + bodyBaselineOffset, size: px(bodyPx),
			text: rw.value, bold: true, color: ink})
		ry += natural[i] + stretch
	}
}

// drawClosing paints the closing paragraph, centred across the content width,
// with `top` as the position of its first line box. The block's own 4px top
// margin is applied by the caller -- preserved when the paragraph opens a page,
// because a margin at the top of a fragmentainer is not truncated; only the
// grid gap around a page break is.
func (e *Engine) drawClosing(p *page, in Input, m metrics, top float64) {
	inner := m.innerW
	cy := top
	for _, line := range e.Wrap(in.Closing, px(bodyPx), inner, false) {
		w := e.Width(line, px(bodyPx), false)
		p.add(run{x: m.innerX + (inner-w)/2, baseline: cy + bodyBaselineOffset,
			size: px(bodyPx), text: line, color: e.pal.Secondary})
		cy += px(bodyPx) * bodyLH
	}
}

// totalRow is one line of the totals card.
type totalRow struct {
	label  string
	value  string
	colour string
	grand  bool
}

// totalsRows builds the totals card's lines. The labels and the conditional PPN
// line match the template exactly.
func (e *Engine) totalsRows(in Input) []totalRow {
	// Keyed, not positional. The field order here is chosen for memory layout, so
	// an unkeyed literal would silently move a value into the wrong field when
	// that order changes -- two of these four fields are strings, so the compiler
	// would not catch it.
	rows := []totalRow{
		{label: "Sub Total", value: "Rp" + in.DisplaySubTotal},
	}
	// Conditional exactly as the template is: no discount, no row.
	if in.DiscountAmount > 0 {
		rows = append(rows, totalRow{
			label:  "Discount (" + in.DiscountPercent + "%)",
			value:  "Rp" + in.DisplayDiscount,
			colour: e.pal.Green,
		})
	}
	if in.DisplayPpn != "" {
		rows = append(rows, totalRow{label: "PPN 11%", value: "Rp" + in.DisplayPpn})
	}
	rows = append(rows, totalRow{label: "Grand Total", value: "Rp" + in.DisplayGrandTotal, grand: true})
	return rows
}

// drawVerify paints the verification block: a right-aligned caption carrying the
// issuer identity and the short code, and the QR card beside it. With no QR the
// template prints a pending notice instead, and so does the engine.
func (e *Engine) drawVerify(p *page, in Input, m metrics, top float64) {
	rx := m.innerX + m.innerW

	if len(in.QRPNG) == 0 {
		rx = m.innerX + m.innerW
		p.add(run{x: rx - e.Width(in.CompanyName, px(11), true),
			baseline: top + bodyBaselineOffset, size: px(11),
			text: in.CompanyName, bold: true, color: e.pal.Accent})
		note := "Verification code is added when you export"
		p.add(box{
			x: rx - e.Width(note, px(10), false) - 2*px(cardPadX) - 2*px(1),
			y: top - px(9), w: e.Width(note, px(10), false) + 2*px(cardPadX) + 2*px(1),
			h:    px(10)*1.5 + 2*px(10) + 2*px(1),
			fill: e.pal.Surface, stroke: e.pal.Edge, lw: px(1), radius: px(radiusSmPx),
		})
		p.add(run{x: rx - px(cardPadX) - px(1) - e.Width(note, px(10), false),
			baseline: top + px(10) + bodyBaselineOffset, size: px(10),
			text: note, color: e.pal.Muted})
		return
	}

	// The card is a 110px box and the image fills it, so the image is the card
	// less its padding and border: 100px. `* { box-sizing: border-box }` is what
	// makes the card's declared width the width outside its border, and the
	// image's `width: 100%` then resolves against what is left inside.
	card := px(qrCardPx)
	img := card - 2*px(1) - 2*px(4)
	qrX := rx - card
	p.add(box{
		x: qrX, y: top, w: card, h: card,
		fill: e.pal.Paper, stroke: e.pal.Edge, lw: px(1), radius: px(radiusSmPx),
	})
	p.add(picture{x: qrX + px(1) + px(4), y: top + px(1) + px(4),
		w: img, h: img, data: in.QRPNG})

	// The caption is right-aligned and sits one gap to the left of the card.
	//
	// `.verify` is `align-items: flex-end`, so the caption hangs off the card's
	// bottom edge rather than starting at the block's top — the whole caption
	// is three lines, and starting it at the top left it floating a line and a
	// half above the code it belongs to. The caption's own 2px bottom padding
	// keeps the last line off the edge.
	//
	// The lines are placed from the last one upwards, because the last line's
	// position is the one `flex-end` fixes and the rest follow from it. The two
	// steps are the line boxes the stylesheet asks for: 11px at 1.5 plus the
	// strong's 1px margin below it, and 10px at 1.5.
	//
	// The caption is a block in a flex row, so it is only as wide as the space
	// beside the card, and a company name too long for that wraps rather than
	// running off the right of the page. The block is bottom-aligned, so the
	// extra lines go upwards from the last one and the first line's step grows by
	// however many there are.
	cx := qrX - px(14)
	captionW := cx - m.innerX
	last := top + card - verifyLastBaseline

	nameLines := e.Wrap(in.CompanyName, px(11), captionW, false)
	if len(nameLines) == 0 {
		nameLines = []string{""}
	}
	// Each line above the first is a further step up, and the strong's own
	// margin sits below the last of them.
	nameTop := last - px(14.97) - px(17.96) - float64(len(nameLines)-1)*px(11)*1.5
	for i, line := range nameLines {
		p.add(run{x: cx - e.Width(line, px(11), true),
			baseline: nameTop + float64(i)*px(11)*1.5,
			size:     px(11), text: line, bold: true, color: e.pal.Accent})
	}
	p.add(run{x: cx - e.Width("Scan to verify this quotation", px(10), false),
		baseline: last - px(14.97), size: px(10),
		text: "Scan to verify this quotation", color: e.pal.Secondary})
	code := "or enter code " + in.VerificationCode
	for i, line := range e.Wrap(code, px(9), captionW, false) {
		_ = i
		p.add(run{x: cx - e.WidthTracked(line, px(9), false, px(0.02*9)),
			baseline: last, size: px(9),
			text: line, color: e.pal.Muted, tracking: px(0.02 * 9)})
	}
}

// itoa avoids pulling strconv in for a single call.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// unusedGuard keeps strings imported for the post-process helpers in this file's
// package from being flagged; the post-processes use their own imports.
var _ = strings.TrimSpace
var _ pdf.Color
