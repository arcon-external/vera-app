package gpdf

// Reinstating the clip regions Chrome used, which gpdf's render layer cannot
// express.
//
// Every rounded card, the subject chip and all 115-odd table cells on a page are
// painted by Chrome as a plain rectangle intersected with a bezier clip outline.
// The extractor's clip field carries that outline. gpdf has no clip support at
// any layer — document/render/pdftarget.go's RenderPath emits a complete
// "q … m/l/c … f Q" and offers no way to leave the path unpainted — so the
// outlines cannot be honoured at draw time.
//
// They can be honoured afterwards. gpdf's rectangle fill is written as a
// self-contained block,
//
//	q
//	<r> <g> <b> rg
//	<x0> <yBot> <w> <h> re
//	f
//	Q
//
// so a sentinel-coloured fill can be swapped for
//
//	q
//	<r> <g> <b> rg
//	<outline> W n
//	<x0> <yBot> <w> <h> re
//	f
//	Q
//
// which is a real clip. The outline must be applied rather than filled: on page 2
// the table's clip spans both pages (y -437.60 to 449.65) while a single cell
// occupies 121.50 to 219.75, so filling the outline would paint the whole table
// in the cell's colour.

import (
	"bytes"
	"fmt"
	"strings"
)

// clipSentinel is the fill colour standing in for "this rectangle needs
// clipping". It must differ from the gradient sentinel, because each post-process
// pass locates its own fills by colour and would otherwise consume the other's.
const (
	clipSentinelR = 0x01
	clipSentinelG = 0x02
	clipSentinelB = 0x03
)

// clippedFill is one rectangle that must be intersected with a clip outline.
type clippedFill struct {
	outline string
	fill    string
	x0      float64
	y0      float64
	w       float64
	h       float64
	pageH   float64
}

// roundedOutline renders a clipRegion as PDF path operators in PDF's bottom-up
// coordinate space.
//
// The corner radii use the same control-point offset a browser does
// (k = 0.5523 x r), because Chrome's own rounded corners are the beziers this
// has to agree with.
func roundedOutline(c clipRegion, pageH float64) string {
	r := [4]float64{c.tl, c.tr, c.br, c.bl}
	for i := range r {
		ew, eh := c.w, c.h
		if i%2 == 1 {
			ew, eh = c.h, c.w
		}
		r[i] = minF(r[i], minF(ew/2, eh/2))
	}
	k := func(v float64) float64 { return 0.5523 * v }

	// PDF's y runs upwards, so `top` is the box's upper edge and `bot` its lower
	// one, and a corner turns *down* by r on the way out of `top` and *up* by r
	// on the way out of `bot`. Writing `top+r` here put every corner's far end a
	// radius outside the box: the arc bulged past the edges it was meant to round
	// off, so the straight part of the clip sat a radius in from the fill and the
	// fill's own corners showed through square. That is what made the subject
	// strip's label a hard rectangle inside a rounded border.
	//
	// The radii are held in the order [top-left, top-right, bottom-right,
	// bottom-left] and the path is written clockwise from the top edge, so the
	// path's own order does not line up with the array's: the top-right corner is
	// written first and is r[1], and the two left-hand corners come last. Reading
	// the array positionally instead of by name swapped the two left radii, which
	// put the items table's rounding on the bottom of the table and left its top
	// square — the wrapper's `border-radius: 10px 10px 0 0` is the whole point of
	// that clip.
	tl, tr, br, bl := r[0], r[1], r[2], r[3]
	top, bot := pageH-c.y, pageH-c.y-c.h
	right, left := c.x+c.w, c.x
	var b strings.Builder
	fmt.Fprintf(&b, "%.5f %.5f m\n", left+tl, top)
	fmt.Fprintf(&b, "%.5f %.5f l\n", right-tr, top)
	if tr > 0 {
		fmt.Fprintf(&b, "%.5f %.5f %.5f %.5f %.5f %.5f c\n",
			right-tr+k(tr), top, right, top-tr+k(tr), right, top-tr)
	}
	fmt.Fprintf(&b, "%.5f %.5f l\n", right, bot+br)
	if br > 0 {
		fmt.Fprintf(&b, "%.5f %.5f %.5f %.5f %.5f %.5f c\n",
			right, bot+br-k(br), right-br+k(br), bot, right-br, bot)
	}
	fmt.Fprintf(&b, "%.5f %.5f l\n", left+bl, bot)
	if bl > 0 {
		fmt.Fprintf(&b, "%.5f %.5f %.5f %.5f %.5f %.5f c\n",
			left+bl-k(bl), bot, left, bot+bl-k(bl), left, bot+bl)
	}
	fmt.Fprintf(&b, "%.5f %.5f l\n", left, top-tl)
	if tl > 0 {
		fmt.Fprintf(&b, "%.5f %.5f %.5f %.5f %.5f %.5f c\n",
			left, top-tl+k(tl), left+tl-k(tl), top, left+tl, top)
	}
	b.WriteString("h\n")
	return b.String()
}

// injectClippedFills rewrites page `page`'s content stream, replacing every
// clip-sentinel fill with a genuinely clipped one, and rebuilds the xref.
func injectClippedFills(doc []byte, clips []clippedFill, page int) ([]byte, error) {
	if len(clips) == 0 {
		return doc, nil
	}
	offsets, err := parseXrefOffsets(doc)
	if err != nil {
		return nil, err
	}
	contentObj, err := pageContentObject(doc, offsets, page)
	if err != nil {
		return nil, err
	}
	body, err := objectBody(doc, offsets[contentObj])
	if err != nil {
		return nil, err
	}
	content, err := decodeStream(body)
	if err != nil {
		return nil, err
	}
	for _, c := range clips {
		if content, err = swapSentinelForClippedRect(content, c); err != nil {
			return nil, err
		}
	}

	// The xref is rebuilt with no new objects: nothing but object lengths change.
	maxObj := 0
	for n := range offsets {
		if n > maxObj {
			maxObj = n
		}
	}
	return rebuildPDF(doc, offsets, contentObj, content, maxObj+1, nil, page)
}

// swapSentinelForClippedRect inserts the clip operator into one sentinel block.
//
// Each pass rewrites the FIRST remaining sentinel, so a page with a hundred
// clipped cells is handled by calling this a hundred times; already-rewritten
// blocks no longer contain the sentinel colour, so they are skipped.
func swapSentinelForClippedRect(content []byte, c clippedFill) ([]byte, error) {
	sr := float64(clipSentinelR) / 255
	sg := float64(clipSentinelG) / 255
	sb := float64(clipSentinelB) / 255

	head := sentinelFillEnd(content, 0, sr, sg, sb)
	if head < 0 {
		return nil, fmt.Errorf("%w: no clip sentinel fill %.6f %.6f %.6f in content stream",
			errNoSentinel, sr, sg, sb)
	}

	// The replacement has to start at the beginning of the colour OPERANDS, not
	// at the space before "rg". Cutting back only to " rg" leaves the sentinel's
	// three numbers in place and appends the real colour after them, so the
	// stream reads
	//
	//	<sentinel r> <sentinel g> <sentinel b><real r> <real g> <real b> rg
	//
	// — six operands where the operator takes three, with two of them glued into
	// one unparseable token. Poppler resynchronises and happens to take the last
	// three, which is why the page looks right under pdftoppm and wrong
	// everywhere else: the colour each renderer recovers is its own choice, and
	// a renderer that stops at the malformed token paints the sentinel instead.
	// Rewriting to the start of the line removes the ambiguity entirely.
	rgAt := strings.LastIndex(string(content[:head]), " rg")
	if rgAt < 0 {
		return nil, fmt.Errorf("%w: colour operator for clip sentinel not found",
			errNoSentinel)
	}
	lineStart := strings.LastIndex(string(content[:rgAt]), "\n")
	if lineStart < 0 {
		return nil, fmt.Errorf("%w: colour operator for clip sentinel has no line start",
			errNoSentinel)
	}

	outline := c.outline

	// Keep the q, restore the real colour in place of the sentinel, then add the
	// clip between the colour and the rectangle. The trailing newline matters:
	// "W n" and the following coordinate would otherwise fuse into one token.
	fr, fg, fb := parseHex(c.fill)
	var out bytes.Buffer
	out.Write(content[:lineStart+1])
	fmt.Fprintf(&out, "%.6f %.6f %.6f rg\n",
		float64(fr)/255, float64(fg)/255, float64(fb)/255)
	out.WriteString(outline)
	out.WriteString("W n\n")
	out.Write(content[head:])
	return out.Bytes(), nil
}

// patchPageContent rewrites one page's content stream with fn and rebuilds the
// xref around it. Used by the post-processes that only change stream bytes.
func patchPageContent(doc []byte, page int, fn func([]byte) ([]byte, error)) ([]byte, error) {
	offsets, err := parseXrefOffsets(doc)
	if err != nil {
		return nil, err
	}
	contentObj, err := pageContentObject(doc, offsets, page)
	if err != nil {
		return nil, err
	}
	body, err := objectBody(doc, offsets[contentObj])
	if err != nil {
		return nil, err
	}
	content, err := decodeStream(body)
	if err != nil {
		return nil, err
	}
	patched, err := fn(content)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(patched, content) {
		return doc, nil
	}
	maxObj := 0
	for n := range offsets {
		if n > maxObj {
			maxObj = n
		}
	}
	return rebuildPDF(doc, offsets, contentObj, patched, maxObj+1, nil, page)
}

// applyPageClip prepends the document's page-level clip to a page's content
// stream.
//
// Chrome sets it once, before anything is drawn, so it trims every fill at the
// page edge — most visibly the card's right and bottom hairlines, which a 0.75pt
// rect otherwise paints a third too wide. gpdf has no clip primitive to set at
// page scope, so the operator is inserted into the finished stream instead.
//
// The extra "q" is left unmatched, which is correct: a content stream is
// implicitly wrapped in q/Q by the renderer, so the state cannot leak to
// another page.
func applyPageClip(doc []byte, page int, x0, y0, x1, y1, pageH float64) ([]byte, error) {
	return patchPageContent(doc, page, func(c []byte) ([]byte, error) {
		if bytes.Contains(c, []byte("vera-page-clip")) {
			return c, nil
		}
		var b bytes.Buffer
		b.WriteString("% vera-page-clip\nq\n")
		fmt.Fprintf(&b, "%.5f %.5f %.5f %.5f re\nW n\n",
			x0, pageH-y1, x1-x0, y1-y0)
		b.Write(c)
		return b.Bytes(), nil
	})
}
