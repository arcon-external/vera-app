package gpdf

import (
	"math"

	"github.com/gpdf-dev/gpdf/document"
)

// The display list and page composition.
//
// Layout resolves the document to primitives — boxes, text runs, images and one
// gradient — before a single PDF operator is emitted. Keeping the two apart is
// what makes the geometry checkable: the layout can be compared against a
// browser's output without involving the PDF writer.
//
// Coordinates are PDF points with y measured DOWNWARD from the page top, which
// is how the stylesheet is written. The flip into PDF's bottom-up space happens
// once, in the draw layer.

type box struct {
	gradient *ramp
	path     *document.Path
	clip     *clipRegion
	fill     string
	stroke   string
	x        float64
	y        float64
	w        float64
	h        float64
	lw       float64
	radius   float64
}

type ramp struct{ c0, c1 string }

type run struct {
	text     string
	color    string
	x        float64
	baseline float64
	size     float64
	tracking float64
	bold     bool
}

type picture struct {
	data []byte
	x    float64
	y    float64
	w    float64
	h    float64
}

// clipRegion is a rounded outline that a set of boxes must be intersected with.
// The items table is drawn inside one; gpdf has no clip primitive, so the draw
// layer reinstates these as real PDF clips in a post-process.
type clipRegion struct {
	x, y, w, h     float64
	tl, tr, br, bl float64
}

// rounded reports whether any corner is actually round.
func (c *clipRegion) rounded() bool {
	return c.tl != 0 || c.tr != 0 || c.br != 0 || c.bl != 0
}

// even rounds all four corners, which is what a standalone card wants.
func even(x, y, w, h, r float64) *clipRegion {
	return &clipRegion{x: x, y: y, w: w, h: h, tl: r, tr: r, br: r, bl: r}
}

type page struct {
	prims []any
	clips []clipRegion

	// bottom is where the card closes on this page.
	//
	// `.page` has no height of its own, so it is as tall as what is on it: the
	// card ends on the last block's bottom edge, not at the bottom margin. The
	// padding that would have filled the gap is discarded where the page breaks,
	// which is what a browser does with a box that would otherwise overflow, and
	// a fixed-height card leaves a strip of empty border under every page that
	// does not fill.
	bottom float64
}

func (p *page) add(b any) { p.prims = append(p.prims, b) }

// Stylesheet constants in CSS pixels. Kept named so a template change lands in
// one place rather than in a dozen expressions.
const (
	cardRadiusPx  = 10
	contentPadPx  = 24
	ribbonH       = 10
	contentGapPx  = 18
	leadPadYTop   = 16
	leadPadYBot   = 14
	leadPadX      = 24
	h1SizePx      = 30
	leadPSizePx   = 12
	brandMaxW     = 380 // .top-brand p max-width
	eyebrowPx     = 13
	eyebrowTrack  = 0.32
	eyebrowPadTop = 14 // .doc-eyebrow padding-top
	// The space between the eyebrow and the content below it, which is also the
	// space above it: .doc-eyebrow carries 14px of top padding under the lead
	// divider, and this was 22px, so the block sat noticeably closer to the cards
	// below than to the divider above. They are 14px each now.
	contentPadTop = 14 // .content padding-top
	contentPadBot = 26 // .content padding-bottom
	cardPadX      = 14
	cardPadY      = 12
	metaPadX      = 16
	metaPadY      = 14
	metaGapX      = 14
	metaMinW      = 240 // .quote-meta min-width
	metaGapY      = 8
	subjectLblPx  = 110
	thPadPx       = 10
	tdPadPx       = 10
	headRowPx     = 48
	footPadPx     = 14
	footGapPx     = 14
	tncMarginH3   = 8
	tncPPGap      = 4
	totalsGap     = 8
	grandPadTop   = 8
	verifyGapPx   = 18

	// metaLineBox is one row of the quotation metadata card, in points. 11px at
	// the inherited 1.45 line height is 15.95px, which the browser rounds to the
	// 16px it lays out.
	metaLineBox = 16 * ptPerPx

	// metaStackGap is `.meta-stack`'s gap, in points. It is what stands between
	// the metadata card and the decoration, and it is subtracted before the
	// decoration's height is taken, so a wrong value here moves the decor rather
	// than the card.
	metaStackGap = 12 * ptPerPx
	bodyPx       = 11.0
	bodyLH       = 1.45
	descLH       = 1.4
	subGapPx     = 6 // .desc-subitems margin-top
	liMarginPx   = 3 // .desc-subitems li margin, collapses between siblings
	subIndentPx  = 16

	radiusSmPx = 8
	// qrCardPx is the verification QR card's box, padding and border included.
	qrCardPx = 110
)

// Palette, verbatim from the stylesheet.
const (
	colInk        = "#1A1A1A"
	colDeep       = "#1A237E"
	colAccent     = "#284AF3"
	colSoft       = "#D6E4FF"
	colGreen      = "#228B22"
	colPageEdge   = "#C6D4FF"
	colLeadRule   = "#D8E2FF"
	colEyebrowBar = "#C9D9FF"
	colCardEdge   = "#CCD9FF"
	colCardBg     = "#F8FBFF"
	colMetaBg     = "#F8FAFF"
	colMetaEdge   = "#C9D9FF"
	colSubjectBg  = "#F7FAFF"
	colSubjectInk = "#26304D"
	colSubjectEdg = "#CAD8FF"
	colTableEdge  = "#C5D5FF"
	colTableRule  = "#CFE0FF"
	colZebra      = "#F6F9FF"
	colTotalsBg   = "#F9FBFF"
	colTotalsEdge = "#CEDBFF"
	colTotalsInk  = "#2F3654"
	colClosing    = "#364066"
	colMuted      = "#8A93B2"
	colVerifyInk  = "#3A4466"
	colPaper      = "#FFFFFF"
	colLead       = "#3A4466"
	colMetaInk    = "#2F3654"
	colDecor0     = "#DCE8FF"
	colDecor1     = "#C9DCFF"
)

// headRowH is the items header row's height in points.
//
// The stylesheet asks for `height: 48px` on the cells, and `* { box-sizing:
// border-box }` makes that the border box: 48px, all told. The browser's own
// output draws the row 36.000pt tall, which is what this is.
const headRowH = 36.0

// tableClip is the items table's clipping outline for one page.
//
// It is one region for the whole table rather than one per row, because the
// wrapper the stylesheet clips with is `border-radius: 10px 10px 0 0`: the top
// corners are round and the bottom two are square. The region's height runs to
// the card's foot rather than to the table's last row, so the clip covers every
// row without the layout having to know the page's row count before drawing it.
// The clip runs to the card's edge on the page the table is on, which is not the
// same edge on every page: a page of rows closes its card on the last row, and a
// page carrying the footer closes it on the footer.
//
// The outline starts at `tableTop` — the header's fill — and not a pixel above
// it at the table's border box.
//
// That was worth getting wrong once. The reasoning was sound: the wrapper being
// clipped is a box with a border, so its top is the border box, not the fill. The
// measurement says otherwise. Fitting a circular corner to the browser's own
// pixels through this corner gives a top of 331.50pt to within 0.01pt of rms,
// which is the fill's top, not the 330.75pt border box. So whatever the browser
// clips the header's background with is flush with that background, and starting
// the arc a pixel high makes the corner flatter than the browser's and leaves the
// fill's corner showing outside where the light border has already stopped.
// tableClip is the region every table fill and border is clipped to: the width
// of the table, from where this page's table starts down to the card's bottom
// edge.
//
// Only the first page rounds its top corners, and the browser agrees. Measured
// off its own render at 300dpi, the table's top-left on page one eases in from
// 62.16pt to 56.16pt over six points, while on page two the left border is
// already at 56.16pt on the first row of ink and stays there. A continuation page
// begins with a body row, because the header does not repeat, so there is nothing
// at its top for a corner to curve around. Rounding it does not merely add an
// edge the browser does not draw: it clips the border away, and the first four
// points of a continuation page's left edge came out blank before this.
func tableClip(m metrics, tableTop, bottom float64, first bool) *clipRegion {
	c := &clipRegion{x: m.tableX, y: tableTop, w: m.tableW, h: bottom - tableTop}
	if first {
		c.tl = px(cardRadiusPx)
		c.tr = px(cardRadiusPx)
	}
	return c
}

// subjectStripH is the subject strip's height in points for a value that fits on
// one line.
//
// It is a point value rather than a pixel one because the strip's height is set
// by its content -- one line of 11px text plus 12px of padding above and below,
// which the browser rounds to 30pt -- and not by a 30px figure that appears
// nowhere in the stylesheet.
//
// It is a floor, not a constant. `.subject` is a grid with no height of its own,
// so the strip grows with a value that wraps, and a subject three lines long makes
// it three lines taller. Treating 30pt as the height pushed the table down over
// the last line of a long subject.
const subjectStripH = 30.0

// subjectLineBox is one line of the subject's value, in points. 11px at the
// inherited 1.45 line height is 15.95px, which the browser rounds to the 16px it
// lays out.
const subjectLineBox = 16 * ptPerPx

// subjectValueW is the width the subject's value wraps to: the strip less the
// label track, the value's own padding, and the strip's border.
func subjectValueW(m metrics) float64 {
	return m.innerW - px(subjectLblPx) - 2*px(1) - 2*px(cardPadX)
}

// subjectHeight is the strip's height for this subject: its content's line count
// times the line box, plus the value's padding on both sides and the strip's
// border.
func (e *Engine) subjectHeight(in Input, m metrics) float64 {
	lines := len(e.Wrap(in.Subject, px(bodyPx), subjectValueW(m), true))
	if lines < 1 {
		lines = 1
	}
	// The line box is rounded to a whole pixel, the same 15.95px -> 16px the
	// metadata card's rows take. Carrying 15.95 through makes a one-line strip
	// 0.04pt short, which is a fortieth of a point and still enough to move the
	// table off the browser's line by the time it reaches the bottom of a page.
	return float64(lines)*subjectLineBox + 2*px(cardPadY) + 2*px(1)
}

// eyebrowLineBox is the eyebrow label's line box, and eyebrowLabelW its width.
//
// Both are the browser's. The line box is 13px of text at 1.45, which is 18.85px
// and lands on 19 once the browser rounds it to whole pixels; using the exact
// 18.85 puts the content 0.11pt high, and the content's top is where the info
// cards, the subject strip and the table all hang from.
const (
	eyebrowLineBox = 19 * ptPerPx
	eyebrowLabelW  = 85.50
)

// descLineH is the line box height inside a description cell, in points.
//
// The stylesheet asks for 1.4 x 11px = 15.4px. The browser lays line boxes out
// on whole pixels, so a 15.4px line box is 15px tall on one line and 16px on the
// next depending on where it lands: a description cell really does alternate. A
// layout engine that reproduces the arithmetic exactly lands 0.85pt short on
// every row, so the mean of the two is used instead and the error becomes a
// fraction of a point that does not accumulate visibly down the table.
const descLineH = 15.5 * ptPerPx

// colFrac are the table's column widths as fractions, matching the colgroup.
// The colgroup is 7/43/8/8/17/17 percent.
var colFrac = [6]float64{0.07, 0.43, 0.08, 0.08, 0.17, 0.17}

// colOffsetsPx are the table's column boundaries, in whole CSS pixels from the
// table's left edge.
//
// They are the browser's own, read off its output, and they are not the colgroup's
// percentages rounded. Those percentages give 45, 323, 375, 426 and 536 for the
// first five boundaries of this table, where the browser lays out 45, 323, 374,
// 426 and 535 — the third and fifth land a pixel left of any truncation of their
// own percentage, so no single rounding rule reproduces all five: floor and round
// each miss one.
//
// Reproducing that needed the exact arithmetic the browser uses to divide a
// truncated width between tracks, which is a LayoutUnit computation over a page
// box this engine does not otherwise model. The colgroup is part of a fixed
// template and this is the only table it describes, so the grid is a constant
// here rather than a computation — and it is the constant the browser uses, so
// the cell borders land where its borders land.
var colOffsetsPx = [7]int{0, 45, 323, 374, 426, 535, 646}

// columnEdges returns the table's column boundaries in points.
func columnEdges(tableX, tableW float64) [7]float64 {
	var out [7]float64
	for i, off := range colOffsetsPx {
		out[i] = tableX + float64(off)*ptPerPx
	}
	// The table's own right edge is its width rather than the last offset, so a
	// width that does not divide into a whole number of pixels still ends where
	// the browser's does.
	out[6] = tableX + tableW
	return out
}

// metrics holds the derived point-valued measures the composition shares.
//
// Everything here is derived once, before any page is built, because the
// vertical flow depends on the heights of blocks below it: the table cannot
// start until the info cards and the subject strip have been sized, and the
// table cannot run into the footer until the footer has been sized.
type metrics struct {
	cardX, cardY, cardW, cardH float64

	// Horizontal rhythm of the page interior.
	innerX float64
	innerW float64
	colX   [7]float64
	tableX float64
	tableW float64

	// Vertical flow, measured downward from the card's top.
	eyebrowTop  float64
	contentTopY float64
	subjectY    float64
	// subjectH is the strip's height for this subject. `.subject` is a grid with
	// no height of its own, so it is as tall as its value wraps to.
	subjectH   float64
	tableTop   float64
	cardBottom float64
	// pageBottom is the limit content is placed against: the card's bottom edge
	// when the page overflows. The card itself closes on the last block, which
	// is pageBottom or above it.
	pageBottom  float64
	firstRowTop float64

	// The info row's two tracks and the stack's height.
	infoLeftW  float64
	infoRightW float64
	infoH      float64

	// The footer's two tracks and block heights.
	footerTncW    float64
	footerTotW    float64
	footerTncH    float64
	footerTotalsH float64
	footerH       float64
}

// derive computes the horizontal measures and the vertical flow.
//
// The card's box is the @page margin: Chrome writes a 36.75pt top margin for the
// 1.3cm the stylesheet asks for, and the card runs to the bottom margin.
func (e *Engine) derive(in Input) metrics {
	var m metrics
	m.cardX = pageMarginX
	m.cardY = pageMarginY
	m.cardW = cardWidth
	m.cardH = pageContentH
	m.cardBottom = m.cardY + m.cardH
	m.pageBottom = m.cardBottom

	// Page interior: the card's 1px border, then .content's horizontal padding.
	m.innerX = m.cardX + px(1) + px(contentPadPx)
	m.innerW = m.cardW - 2*px(1) - 2*px(contentPadPx)
	m.tableX = m.innerX
	m.tableW = m.innerW
	m.colX = columnEdges(m.tableX, m.tableW)

	// Top-lead band: whichever is taller, the text block or the logo.
	textBottom := m.cardY + px(1) + px(ribbonH) + px(leadPadYTop) +
		leadAddrFirstBaseline +
		float64(maxInt(0, len(in.CompanyAddress)-1))*addrPitch + px(4)
	logoBottom := m.cardY + px(1) + px(ribbonH) + px(leadPadYTop) + px(86)
	band := textBottom
	if logoBottom > band {
		band = logoBottom
	}
	// The band's bottom border, then the eyebrow's top padding and its line box.
	m.eyebrowTop = band + px(leadPadYBot) + px(1) + px(eyebrowPadTop)
	m.contentTopY = m.eyebrowTop + eyebrowLineBox + px(contentPadTop)

	// Info row: two tracks with the gap between them subtracted first, then
	// `.quote-meta`'s `min-width: 240px` applied.
	//
	// The 1.5fr / 0.9fr split alone would give the metadata card 177.75pt, but it
	// carries a minimum width of 240px — 180pt — and a grid track that reaches
	// its minimum takes the difference out of the flexible track beside it. The
	// recipient card is therefore 294pt, not the 296.25pt the fractions suggest.
	trackW := m.innerW - px(metaGapX)
	m.infoRightW = trackW * 0.9 / 2.4
	if floor := px(metaMinW); m.infoRightW < floor {
		m.infoRightW = floor
	}
	m.infoLeftW = trackW - m.infoRightW
	m.infoH = e.infoHeight(in, m)

	m.subjectY = m.contentTopY + m.infoH + px(contentGapPx)
	m.subjectH = e.subjectHeight(in, m)
	m.tableTop = m.subjectY + m.subjectH + px(contentGapPx)

	m.footerTncW = (m.innerW - px(footGapPx)) * 1.2 / 2.0
	m.footerTotW = (m.innerW - px(footGapPx)) * 0.8 / 2.0
	rows := e.totalsRows(in)
	// The footer is a grid of two cards, and a grid stretches every track to the
	// height of the tallest. Measuring each card against its own content leaves
	// the shorter one short, which puts a visible step along the bottom edge
	// between two boxes the stylesheet puts side by side.
	footerH := maxF(e.tncHeight(in, m.footerTncW), e.totalsHeight(rows))
	m.footerTncH = footerH
	m.footerTotalsH = footerH
	m.footerH = maxF(m.footerTncH, m.footerTotalsH) +
		// The grid's gap, then the closing paragraph's own top margin.
		px(contentGapPx) + px(4) +
		float64(len(e.Wrap(in.Closing, px(bodyPx), m.innerW, false)))*px(bodyPx)*bodyLH +
		// The gap again, then the verification block's own top margin.
		px(contentGapPx) + px(verifyGapPx) + verifyHeight(in)
	return m
}

// rowBlock is one laid-out table row.
//
// This is the one type in the engine left in its natural order. The analyzer
// wants a different permutation for eight bytes of collector scan prefix, and
// there are about thirteen of these per document, so the saving is roughly a
// hundred bytes on an export. Leading with the pointer-free height was tried and
// is worse (152 leading bytes against 144). Readability wins at this volume;
// see alignment_test.go for the rule applied to the types where it pays.
type rowBlock struct {
	item   Item
	lines  []string
	subs   []subLine
	height float64
}

// subLine is one line of the item's bullet list. `first` marks the line that
// starts a list item, because only that line gets a disc: a bullet that wraps
// continues on the next line without another one, which is what the browser does
// and what putting a marker on every line gets wrong.
type subLine struct {
	text  string
	first bool
}

// layout composes the whole document into pages.
//
// Pagination follows what a browser does with this template. The header row does
// not repeat, because it is a separate table rather than a thead, so a
// continuation page starts straight into body rows at the top of the card.
//
// The footer is not reserved for. In normal flow the items table simply runs to
// the bottom of the page and the footer follows it, landing on the next page if
// it does not fit -- which is what `break-inside: avoid` on the footer achieves.
// Subtracting the footer's height from every page's available space instead
// would leave the table with almost a fifth of a page to fill and push rows onto
// pages they do not belong on.
func (e *Engine) layout(in Input) []*page {
	m := e.derive(in)
	rows := e.measureRows(in, m)
	// The table runs to the page's bottom edge rather than to the content box's.
	// `.content` reserves 26px of bottom padding, but a row is never split across
	// pages, so the browser fills into that reserve when a row would otherwise be
	// stranded and only gives up once the next row will not fit at all.
	bottom := m.cardBottom

	var pages []*page
	cur := &page{}
	e.drawPageChrome(cur, in, m)

	// The card is painted first so everything else sits on top of it, but its
	// height is only known once the page's content has been placed, so it is
	// added last and moved to the front of the display list.
	finish := func(p *page, bottom float64) {
		p.bottom = bottom
		p.prims = append(cardFor(p, m, bottom), p.prims...)
	}

	tableTop := m.tableTop
	avail := bottom - tableTop
	used := 0.0
	first := 0

	for {
		// The header row belongs to the first page only.
		if first == 0 {
			e.drawTableHead(cur, m, tableClip(m, tableTop, bottom, true), tableTop)
			used += headRowH
		}
		placed := 0
		for first+placed < len(rows) {
			rb := rows[first+placed]
			if used+rb.height > avail {
				break
			}
			// first == 0 marks the page carrying the header, and only that page
			// has a rounded top to the table.
			e.drawTableRow(cur, m, tableClip(m, tableTop, bottom, first == 0), tableTop+used, rb, (rb.item.No-1)%2 == 1)
			used += rb.height
			placed++
		}
		first += placed

		if first >= len(rows) {
			// The footer goes below the table, on this page if it fits and on a
			// page of its own if it does not.
			footTop := tableTop + used + px(contentGapPx)
			ends := tableTop + used
			if m.footerH > 0 && footTop+m.footerH > bottom {
				finish(cur, ends)
				pages = append(pages, cur)
				cur = &page{}
				e.drawContinuationChrome(cur, m)
				footTop = m.cardY + px(1) + px(contentGapPx)
				ends = footTop
			}
			if m.footerH > 0 {
				e.drawFooter(cur, in, m, footTop)
				// The content's bottom padding is real on a page that ends
				// within the page box. On a page the table overflows it is not,
				// because the rows have already spent it — which is why a page of
				// rows closes its card on the last row and a page with the footer
				// closes it a padding's worth lower.
				ends = footTop + m.footerH + px(contentPadBot)
			}
			finish(cur, ends)
			pages = append(pages, cur)
			break
		}

		// More rows to place, so this page ran to the limit rather than to its
		// content: the card is the page's full height, not the height of what
		// landed on it.
		finish(cur, m.cardBottom)
		pages = append(pages, cur)
		cur = &page{}
		e.drawContinuationChrome(cur, m)
		tableTop = m.cardY + px(1)
		avail = bottom - tableTop
		used = 0
	}
	return pages
}

// descLinePx is the description cell's line box in CSS pixels. The stylesheet
// asks for `line-height: 1.4` on 11px text, which is 15.4px and not a whole
// pixel; the browser carries the fraction from one block to the next, so a run
// of lines lands on 15 or 16 each depending on where it starts.
const descLinePx = 15.4

// measureRows lays out every row's description and derives its height from the
// CSS box model: cell padding, one line box per wrapped line, the sub-item list's
// top margin, and the li margins collapsing between siblings.
func (e *Engine) measureRows(in Input, m metrics) []rowBlock {
	// The cell box runs from one column boundary to the next, and its 1px
	// collapsed border is drawn inside the leading edge, so the content box is
	// narrower than the track by that border as well as by the padding.
	descW := m.colX[2] - m.colX[1] - px(1) - 2*px(tdPadPx)
	subW := descW - px(subIndentPx)
	padY := 2 * px(tdPadPx)

	out := make([]rowBlock, 0, len(in.Items))
	for _, it := range in.Items {
		lines := e.Wrap(it.DescMain, px(bodyPx), descW, false)
		var subs []subLine
		for _, s := range it.SubItems {
			for i, ln := range e.Wrap(s, px(bodyPx), subW, false) {
				subs = append(subs, subLine{text: ln, first: i == 0})
			}
		}
		// The row's height is its content box plus the cell's padding on both
		// sides plus both of its 1px collapsed borders. The border is inside the
		// row rather than between rows, so it belongs in the height: dropping it
		// takes a pixel off every row and the table ends up short of where the
		// browser's does.
		//
		// The content is then resolved to a whole pixel, because the browser
		// resolves the row's height in whole pixels: a row of two lines comes out
		// 51px or 52px depending on where the 15.4px line box falls, and that
		// pixel moves every row below it.
		//
		// The resolution floors, and that is the whole of the fix. A browser lays
		// this table out in fractional pixels and snaps each row's bottom edge
		// down to a whole pixel, so a row is the floor of its own content and
		// never the round. Rounding up put every two-line row a pixel tall, and
		// over the six rows of the reference document that was six pixels, two
		// and a quarter points -- enough to push the sixth item onto page two
		// and make the engine paginate a different document from the browser's.
		// Flooring lands within one pixel of the browser on every row of it and
		// the table ends up the same length.
		content := float64(len(lines)) * descLinePx
		if n := countItems(subs); n > 0 {
			// The list's 6px top margin absorbs the first item's own 3px, so it
			// stands once; then one 3px between each pair of items, and the last
			// item's 3px bottom margin, which collapses with nothing.
			content += 6 + 3*float64(n)
			content += float64(len(subs)) * descLinePx
		}
		h := math.Floor(content)*ptPerPx + padY + 2*px(1)
		out = append(out, rowBlock{item: it, lines: lines, subs: subs, height: h})
	}
	return out
}

// The recipient card's internal rhythm, in points, measured from a browser
// render of the same markup.
//
// These are measured rather than derived because the card's blocks do not add up
// the way their declared sizes suggest: the heading, name, company and address
// label each carry a line box sized by their own font size, the margins between
// them collapse against the neighbouring line box rather than adding to it, and
// the template always emits an address paragraph that is empty whenever the
// recipient has no address. Each constant is the distance Chrome puts between
// two consecutive baselines in that specific context, so the card comes out at
// the height Chrome gives it rather than at the height the arithmetic implies.
const (
	cardPadTop    = 9.75  // card content top -> heading baseline
	cardH3ToName  = 19.50 // heading -> recipient name
	cardNameToCpy = 12.00 // name -> company
	cardCpyToAddr = 16.50 // company -> the "Address:" label

	// cardAfterAddr is from the last baseline in the card to the content's bottom
	// edge: the descent of the last line plus its half-leading, which is all that
	// sits below a line's baseline inside its own line box.
	cardAfterAddr = 3.75
)

// The address block: a 11px line at 1.45 is 15.95px, which the browser rounds
// to the 16px it lays out, and a line of text stands 5px below its baseline in
// its own box.
//
// An *empty* address is not free. The template emits the paragraph
// unconditionally, so with no address it is still a block, and it still occupies
// 24px -- a line box plus 8px. Counting it as nothing made the card 6pt short,
// which is short enough to have swallowed the decoration entirely at one point.
const (
	addrLineBox = 16 * ptPerPx
	addrEmpty   = 24 * ptPerPx
)

// infoHeight is the height of the recipient row.
//
// Both tracks stretch to the taller of the two, and neither is fixed: the
// recipient card is as tall as its own content, and the metadata stack is as tall
// as its card plus the gap plus the decoration's minimum. The row is the larger.
//
// Taking only the recipient card's height was the source of two bugs. The
// decoration is drawn from the row's leftover, so when the stack was the taller
// side the two disagreed and the decoration came out short; and when the
// recipient card was the taller side the card itself was short by however much
// the stack overhung.
func (e *Engine) infoHeight(in Input, m metrics) float64 {
	recv := 2*px(1) + 2*px(cardPadY)
	recv += cardPadTop + cardH3ToName + cardNameToCpy + cardCpyToAddr
	// The address paragraph is present whether or not there is anything to put in
	// it, so it always contributes a block.
	// `Wrap` returns one empty line for an empty string, which is not a line
	// box: an empty paragraph has no lines in it, and counting the empty one
	// takes 6px off the card.
	n := 0
	if in.RecipientAddress != "" {
		n = len(e.Wrap(in.RecipientAddress, px(bodyPx),
			m.infoLeftW-2*px(1)-2*px(cardPadX), false))
	}
	addr := addrEmpty
	if n > 0 {
		addr = float64(n) * addrLineBox
	}
	recv += addr + cardAfterAddr
	stack := e.metaHeight(in, m) + metaStackGap
	return maxF(recv, stack)
}

// metaHeight is the metadata card's own height: its border, its padding, one line
// box per line of every row, and the gaps between rows.
//
// A value too long for the track adds a line, and a line adds a box: the grid's
// `auto` tracks are capped by the card's width, so the text wraps and the card
// grows to hold it. Counting one line per row regardless would clip the card's
// bottom border over the text that wrapped.
func (e *Engine) metaHeight(in Input, m metrics) float64 {
	rows := e.metaRows(in)
	if len(rows) == 0 {
		return 2*px(1) + 2*px(metaPadY)
	}
	// The value track is what is left of the card after the label, and every row
	// gets the same track, so the widest label sets it.
	valueX := m.infoRightW - px(1) - px(metaPadX)
	valueW := valueX - px(1) - px(metaPadX) - e.Width(longestLabel(rows), px(bodyPx), true)

	lines := 0
	for _, kv := range rows {
		n := len(e.Wrap(kv[1], px(bodyPx), valueW, false))
		if n == 0 {
			n = 1
		}
		lines += n
	}
	return 2*px(1) + 2*px(metaPadY) +
		float64(lines)*metaLineBox +
		float64(len(rows)-1)*px(metaGapY)
}

// countItems is the number of list items in a flattened sub-item list, which is
// not the number of lines: an item that wraps still counts once.
func countItems(subs []subLine) int {
	n := 0
	for _, sl := range subs {
		if sl.first {
			n++
		}
	}
	return n
}

// tncHeight is the height of the terms card.
func (e *Engine) tncHeight(in Input, w float64) float64 {
	inner := w - 2*px(1) - 2*px(footPadPx)
	h := px(12)*bodyLH + px(tncMarginH3)
	lines := 0
	for _, t := range in.Terms {
		lines += len(e.Wrap(t, px(bodyPx), inner, false))
	}
	h += float64(lines)*px(bodyPx)*bodyLH + float64(maxInt(0, len(in.Terms)-1))*px(tncPPGap)
	h += 2*px(1) + 2*px(footPadPx)
	return h
}

// totalsHeight is the height of the totals card, which is a grid of its rows
// with the grand total carrying a rule and its own top padding.
func (e *Engine) totalsHeight(rows []totalRow) float64 {
	h := 2*px(1) + 2*px(footPadPx)
	for i, rw := range rows {
		if i > 0 {
			h += px(totalsGap)
		}
		if rw.grand {
			h += px(2) + px(grandPadTop) + px(12)*bodyLH
			continue
		}
		h += px(bodyPx) * bodyLH
	}
	return h
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// verifyHeight is the height of the verification block: the QR card, or the
// pending notice when there is no QR yet.
func verifyHeight(in Input) float64 {
	if len(in.QRPNG) == 0 {
		return px(10)*1.5 + 2*px(10) + 2*px(1)
	}
	return px(qrCardPx)
}

// metaRows is the quotation metadata card's four label/value pairs, in the order
// the template prints them. It is the one place they are listed: the card's
// height and its drawing both read it, so a row cannot be added to one and
// forgotten in the other.
func (e *Engine) metaRows(in Input) [][2]string {
	return [][2]string{
		{"Quotation No.", in.QuoteNumber},
		{"Issue Date", in.Date},
		{"Valid Until", in.ValidUntil},
		{"Revision", in.Revision},
	}
}

// longestLabel is the widest label in the metadata card. The value track starts
// after the label track, so this is what a value has to fit into.
func longestLabel(rows [][2]string) string {
	longest := ""
	for _, kv := range rows {
		if len(kv[0]) > len(longest) {
			longest = kv[0]
		}
	}
	return longest
}
