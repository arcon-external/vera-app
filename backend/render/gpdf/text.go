// Package pdfengine lays out and renders a quotation without a browser.
//
// It replaces the chromedp path in Quotation.build(). The document is a fixed
// design — a card, a ribbon, a header, two info cards, an items table, a totals
// block and a verification caption — so the layout is written out explicitly
// rather than interpreted from CSS. That is deliberate: a general CSS engine
// would be a browser, and this template needs none of one. Only two things are
// genuinely dynamic and therefore measured properly:
//
//   - text advance widths, read from the embedded Inter hmtx, and
//   - the one wrapping cell, the item description.
//
// Everything else is arithmetic on constants taken from the stylesheet.
package gpdf

import (
	"strings"
	"sync"

	"github.com/gpdf-dev/gpdf/pdf/font"
)

// ptPerPx is the CSS-pixel to PDF-point ratio for this template.
//
// Chrome lays the document out in CSS pixels and Skia writes the PDF with a
// 0.24 transform, inside a layout layer that scales by 3.125; the product is
// 0.75. Every length in the stylesheet is therefore 0.75pt, and getting this
// wrong scales the entire document — a 30px heading is 22.5pt on the page, not
// 30pt.
const ptPerPx = 0.75

// px converts CSS pixels to points.
func px(v float64) float64 { return v * ptPerPx }

// Faces are the two weights the template uses.
type Face struct {
	Regular     *font.TrueTypeFont
	Bold        *font.TrueTypeFont
	RegularData []byte
	BoldData    []byte
	upem        float64
	hmtxR       map[int]float64
	hmtxB       map[int]float64
	cmapR       map[rune]int
	cmapB       map[rune]int
}

var facesOnce sync.Once

// metricFace holds the parsed tables for one weight.
type metricFace struct {
	cmap map[rune]int
	hmtx map[int]uint16
	numH int
	upem float64
}

func parseFace(d []byte) *metricFace {
	f := &metricFace{cmap: map[rune]int{}, hmtx: map[int]uint16{}}
	if len(d) < 12 {
		return f
	}
	n := int(be16(d[4:6]))
	tabs := map[string][2]int{}
	for i := 0; i < n; i++ {
		off := 12 + 16*i
		if off+16 > len(d) {
			break
		}
		tabs[string(d[off:off+4])] = [2]int{int(be32(d[off+8 : off+12])), int(be32(d[off+12 : off+16]))}
	}
	if h, ok := tabs["head"]; ok && h[0]+20 <= len(d) {
		f.upem = float64(be16(d[h[0]+18 : h[0]+20]))
	}
	if h, ok := tabs["hhea"]; ok && h[0]+36 <= len(d) {
		f.numH = int(be16(d[h[0]+34 : h[0]+36]))
	}
	if hm, ok := tabs["hmtx"]; ok {
		for g := 0; g < f.numH; g++ {
			p := hm[0] + 4*g
			if p+2 > len(d) {
				break
			}
			f.hmtx[g] = be16(d[p : p+2])
		}
	}
	if cm, ok := tabs["cmap"]; ok {
		f.cmap = parseCmap4(d, cm[0])
	}
	return f
}

func be16(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }
func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func parseCmap4(d []byte, base int) map[rune]int {
	out := map[rune]int{}
	if base+4 > len(d) {
		return out
	}
	for i := 0; i < int(be16(d[base+2:base+4])); i++ {
		p := base + 4 + 8*i
		if p+8 > len(d) {
			break
		}
		sub := base + int(be32(d[p+4:p+8]))
		if sub+14 > len(d) || be16(d[sub:sub+2]) != 4 {
			continue
		}
		segX2 := int(be16(d[sub+6 : sub+8]))
		seg := segX2 / 2
		ends := sub + 14
		// The two-byte reservedPad sits between the end and start arrays.
		// Reading the start array without stepping over it shifts every segment
		// by one, so the upper ranges of the map resolve to whatever glyph
		// happens to sit at the wrong index. The document's own characters come
		// out right either way because they fall in the dense lower ranges,
		// which is what made this survivable: the damage only reaches characters
		// nothing else in the pipeline had exercised.
		starts := ends + segX2 + 2
		deltas := starts + segX2
		ranges := deltas + segX2
		for k := 0; k < seg; k++ {
			e := int(be16(d[ends+2*k : ends+2*k+2]))
			s := int(be16(d[starts+2*k : starts+2*k+2]))
			delta := int(int16(be16(d[deltas+2*k : deltas+2*k+2])))
			ro := int(be16(d[ranges+2*k : ranges+2*k+2]))
			for c := s; c <= e && c <= 0xFFFF; c++ {
				var g int
				if ro == 0 {
					g = (c + delta) & 0xFFFF
				} else {
					gi := ranges + 2*k + ro + 2*(c-s)
					if gi+2 > len(d) {
						continue
					}
					g = int(be16(d[gi : gi+2]))
					if g != 0 {
						g = (g + delta) & 0xFFFF
					}
				}
				if g != 0 {
					out[rune(c)] = g
				}
			}
		}
	}
	return out
}

// advance returns a glyph's width in font units.
func (f *metricFace) advance(g int) float64 {
	if f == nil {
		return 0
	}
	if w, ok := f.hmtx[g]; ok {
		return float64(w)
	}
	if f.numH > 0 {
		if w, ok := f.hmtx[f.numH-1]; ok {
			return float64(w)
		}
	}
	return 0
}

// Width returns the advance width of s in points at sizePt.
func (f *metricFace) Width(s string, sizePt float64) float64 {
	if f == nil || f.upem == 0 {
		return 0
	}
	total := 0.0
	for _, r := range s {
		g, ok := f.cmap[r]
		if !ok {
			continue
		}
		total += f.advance(g)
	}
	return total / f.upem * sizePt
}

// ShapeFor returns the shaper for a weight.
func (e *Engine) ShapeFor(bold bool) *shaper {
	if bold {
		return e.boldShape
	}
	return e.regularShape
}

// Width measures s in points as the browser would set it: shaped, so ligatures
// substitute and pairs kern.
func (e *Engine) Width(s string, sizePt float64, bold bool) float64 {
	return e.ShapeFor(bold).ShapeWidth(s, sizePt)
}

// WidthTracked measures s with extra tracking per character, which the eyebrow's
// letterspaced label needs.
//
// The tracking is added per shaped glyph rather than per source character: the
// eyebrow's label has no ligatures, but counting source characters would add a
// step for each space in a run that has any, and the accumulated error shows up
// as a rule that does not reach the content edge.
func (e *Engine) WidthTracked(s string, sizePt float64, bold bool, tracking float64) float64 {
	gs := e.ShapeFor(bold).Shape(s, sizePt)
	if len(gs) == 0 {
		return 0
	}
	last := gs[len(gs)-1]
	return last.dx + last.advance + tracking*float64(len(gs))
}

// Wrap breaks s into lines no wider than maxW points.
//
// Breaking follows the same rules CSS does for the description cell: at spaces.
// A single word wider than the column is left overlong rather than broken
// mid-word, because the cell wraps at whitespace rather than anywhere.
func (e *Engine) Wrap(s string, sizePt, maxW float64, bold bool) []string {
	if s == "" {
		return []string{""}
	}
	sh := e.ShapeFor(bold)
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		cand := cur
		if cand == "" {
			cand = w
		} else {
			cand = cur + " " + w
		}
		if sh.ShapeWidth(cand, sizePt) <= maxW || cur == "" {
			cur = cand
			continue
		}
		lines = append(lines, cur)
		cur = w
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}
