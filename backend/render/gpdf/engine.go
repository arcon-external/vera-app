package gpdf

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"io/fs"

	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/document/render"
	"github.com/gpdf-dev/gpdf/pdf"
	"github.com/gpdf-dev/gpdf/pdf/font"
)

// Page geometry, in points.
//
// Chrome's A4 MediaBox is 595.91998 x 841.91998, not the whole-millimetre
// 595.276 x 841.890 that a "A4" constant usually carries. The 0.64pt width
// difference shifts every rasteriser's pixel grid, so the MediaBox value is
// reproduced exactly rather than approximated.
const (
	PageW = 595.91998
	PageH = 841.91998

	// The page box, measured off Chrome's own output rather than derived.
	//
	// 1.3cm is 36.85pt and is not what the browser lays out with: the paper is
	// snapped to whole device pixels before the margins are taken off it, so the
	// content box comes out 522.00 x 768.75 and lands at (37.50, 36.75) — a
	// left margin a pixel wider than a right one, which is the signature of a
	// width being truncated rather than divided. Reading these off the stylesheet
	// puts every rule on the page a pixel out, and a pixel is a fifth of a point,
	// which is enough for a table's borders to miss their own cells.
	pageMarginX = 37.50
	pageMarginY = 36.75

	// cardWidth is `.page`'s width in print, where it is `max-width: none` and
	// fills the content box.
	cardWidth = 522.00

	// pageContentH is the card's height when content fills the page. A page that
	// ends before that closes its card on its last block instead.
	pageContentH = 768.75
)

// Input is everything the layout needs. It is deliberately a flat value type:
// the engine holds no reference to the quotation, so nothing in it can reach the
// database, the API client or the signing path.
type Input struct {
	CompanyName    string
	CompanyAddress []string
	LogoPNG        []byte

	Subject          string
	RecipientName    string
	RecipientCompany string
	RecipientAddress string

	QuoteNumber string
	Date        string
	ValidUntil  string
	Revision    string

	Terms []string

	Items []Item

	// Money strings are bare, exactly as the data bag carries them: the template
	// writes "Rp" in front of each one, and the engine has to agree with the
	// wording it is replacing rather than reformat it a second time.
	DisplaySubTotal   string
	DisplayDiscount   string
	DisplayPpn        string
	DisplayGrandTotal string
	DiscountPercent   string

	Closing             string
	VerificationCaption string
	VerificationCode    string
	QRPNG               []byte
}

// Item is one table row.
type Item struct {
	No        int
	DescMain  string
	SubItems  []string
	Qty       string
	UoM       string
	UnitPrice string
	Total     string
}

// Engine renders one document. It is not safe for concurrent use; make one per
// render. It owns no global state beyond the parsed font tables, which are
// immutable after construction.
type Engine struct {
	regular      *font.TrueTypeFont
	bold         *font.TrueTypeFont
	regularData  []byte
	boldData     []byte
	regularShape *shaper
	boldShape    *shaper

	// Brand colours. A quotation's four theme columns are per-document settings
	// that resolve to a stylesheet default when unset, so they live on the
	// engine rather than in the stylesheet constants the layout reads.
	deep, accent, soft, ink string
}

// Theme carries the four quotable brand colours.
type Theme struct{ Deep, Accent, Soft, Ink string }

// NewEngine loads the two faces the engine embeds. They are the same Inter
// programs the stylesheet loads for the browser path, so the two renderers agree
// on the typeface by construction rather than by convention.
//
// The faces come from an fs.FS, not from paths. Paths are resolved against the
// process working directory, which a desktop application does not control, and an
// export then fails on a machine where the file was never missing. The caller
// passes the application's embedded asset filesystem, so the bytes are already in
// the binary and there is nothing to look up.
//
// It is NewEngine rather than New because this package is also a Renderer, and
// the renderer's own constructor is the one the engine line names.
func NewEngine(fonts fs.FS, regularName, boldName string) (*Engine, error) {
	rd, err := fs.ReadFile(fonts, regularName)
	if err != nil {
		return nil, fmt.Errorf("load regular face %q: %w", regularName, err)
	}
	bd, err := fs.ReadFile(fonts, boldName)
	if err != nil {
		return nil, fmt.Errorf("load bold face %q: %w", boldName, err)
	}
	rm, err := font.ParseTrueType(rd)
	if err != nil {
		return nil, fmt.Errorf("parse regular face: %w", err)
	}
	bm, err := font.ParseTrueType(bd)
	if err != nil {
		return nil, fmt.Errorf("parse bold face: %w", err)
	}
	e := &Engine{
		regular: rm, bold: bm,
		regularData: rd, boldData: bd,
		regularShape: parseShaper(rd), boldShape: parseShaper(bd),
		deep:   colDeep,
		accent: colAccent,
		soft:   colSoft,
		ink:    colInk,
	}
	return e, nil
}

// Render lays out and writes the document.
func (e *Engine) Render(in Input) ([]byte, error) {
	pages := e.layout(in)

	var buf bytes.Buffer
	w := pdf.NewWriter(&buf)
	r := render.NewPDFRenderer(w)

	// Register under names with no weight suffix: RenderText resolves
	// FontFamily as given, and a suffix would miss the map silently and emit a
	// non-embedded Type 1 face.
	//
	// Each face is registered from a subset carrying only the glyphs this
	// document draws. gpdf derives the embeddable file from the parsed font's
	// own data rather than from the bytes handed to RegisterTTFont, so the
	// subset has to be what is *parsed* too: registering a subset alongside a
	// full parse would leave the whole file embedded and save nothing. Glyph ids
	// are preserved across the subset, so the subset's parse agrees with the
	// full parse that shaped the layout about which glyph every rune is.
	used := collectRunes(pages)
	regular, err := embeddable(e.regularData, e.regularShape, used.regular)
	if err != nil {
		return nil, fmt.Errorf("subset regular face: %w", err)
	}
	boldFont, err := embeddable(e.boldData, e.boldShape, used.bold)
	if err != nil {
		return nil, fmt.Errorf("subset bold face: %w", err)
	}
	r.RegisterTTFont("inter", regular, regular.Data())
	r.RegisterTTFont("interb", boldFont, boldFont.Data())

	meta := document.DocumentMetadata{
		Title:    "Quotation",
		Creator:  "vera pdfengine",
		Producer: "vera pdfengine",
	}
	if err := r.BeginDocument(meta); err != nil {
		return nil, err
	}

	perPageGrad := make([][]gradient, len(pages))
	perPageClip := make([][]clippedFill, len(pages))
	for i, pg := range pages {
		if err := r.BeginPage(document.Size{Width: PageW, Height: PageH}); err != nil {
			return nil, err
		}
		grads, err := e.drawPage(r, pg)
		if err != nil {
			return nil, err
		}
		perPageGrad[i] = grads
		perPageClip[i] = collectClips(pg)
		if err := r.EndPage(); err != nil {
			return nil, err
		}
	}
	if err := r.EndDocument(); err != nil {
		return nil, err
	}

	data := buf.Bytes()

	// Clips before gradients: each pass finds its own fills by sentinel colour,
	// and the two sentinels are distinct precisely so they cannot consume one
	// another's.
	for i, clips := range perPageClip {
		if len(clips) == 0 {
			continue
		}
		if data, err = injectClippedFills(data, clips, i); err != nil {
			return nil, fmt.Errorf("page %d clips: %w", i+1, err)
		}
	}
	for i, grads := range perPageGrad {
		if len(grads) == 0 {
			continue
		}
		if data, err = injectGradients(data, grads, i); err != nil {
			return nil, fmt.Errorf("page %d gradient: %w", i+1, err)
		}
	}

	// Restore the advance widths gpdf truncated to integers.
	advances := map[string]map[int]float64{}
	if t := e.regularShape.advanceTable(); t != nil {
		advances["inter"] = t
	}
	if t := e.boldShape.advanceTable(); t != nil {
		advances["interb"] = t
	}
	if data, err = fixGlyphWidths(data, advances); err != nil {
		return nil, fmt.Errorf("glyph widths: %w", err)
	}
	return data, nil
}

// collectClips gathers a page's clipped fills in paint order.
//
// The post-process rewrites one sentinel at a time, always taking the first
// remaining one, so the listing order has to be the order they appear in the
// stream -- which is the order the layout emitted them.
func collectClips(p *page) []clippedFill {
	var out []clippedFill
	for _, prim := range p.prims {
		b, ok := prim.(box)
		if !ok || b.clip == nil || b.fill == "" {
			continue
		}
		out = append(out, clippedFill{
			outline: roundedOutline(*b.clip, PageH),
			x0:      b.x, y0: b.y, w: b.w, h: b.h,
			pageH: PageH, fill: b.fill,
		})
	}
	return out
}

// pngSize reads a PNG's intrinsic size, which RenderImage needs for the
// XObject's /Width and /Height.
func pngSize(b []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// SetTheme applies a brand palette. Empty fields keep their stylesheet default,
// which is what an unset theme column means.
func (e *Engine) SetTheme(t Theme) {
	if t.Deep != "" {
		e.deep = t.Deep
	}
	if t.Accent != "" {
		e.accent = t.Accent
	}
	if t.Soft != "" {
		e.soft = t.Soft
	}
	if t.Ink != "" {
		e.ink = t.Ink
	}
}
