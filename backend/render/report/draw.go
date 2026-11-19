package report

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"

	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/document/render"
	"github.com/gpdf-dev/gpdf/pdf"
	"github.com/gpdf-dev/gpdf/pdf/font"

	"vera-app/backend/theme"
)

// A4 portrait, in points. The report is an internal document rather than a
// quotation, so it uses the whole-millimetre A4 box rather than Chrome's
// device-pixel-snapped quotation MediaBox.
const (
	PageWidth  = 595.276
	PageHeight = 841.890

	marginX      = 42.0
	marginTop    = 40.0
	contentWidth = PageWidth - 2*marginX
	contentBot   = 790.0
	footerY      = 812.0
)

// The two Inter faces the report embeds. They are read from the fs.FS the
// caller supplies, so the desktop passes its embedded assets and a test can
// pass an on-disk directory without either knowing the other's paths.
const (
	RegularFont = "inter-medium.ttf"
	BoldFont    = "inter-bold.ttf"
)

type rgb struct{ r, g, b uint8 }

func hexRGB(value string) rgb {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return rgb{0x1a, 0x1a, 0x1a}
	}
	var out rgb
	for i, target := range []*uint8{&out.r, &out.g, &out.b} {
		high := unhex(value[i*2])
		low := unhex(value[i*2+1])
		if high == 0xff || low == 0xff {
			return rgb{0x1a, 0x1a, 0x1a}
		}
		*target = high<<4 | low
	}
	return out
}

func unhex(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0xff
	}
}

func (c rgb) pdf() pdf.Color {
	return pdf.RGB(float64(c.r)/255, float64(c.g)/255, float64(c.b)/255)
}

// palette is the report's colour set. It derives from the same default brand
// theme as the quotation, so an internal Vera document still looks like Vera,
// but it adds the semantic chart colours the quotation never needs.
type palette struct {
	deep, accent, soft, ink rgb
	surface, edge, rule     rgb
	secondary, emphasis     rgb
	subInk, muted           rgb
	paper                   rgb

	amount     rgb
	capital    rgb
	marginPos  rgb
	marginNeg  rgb
	marginLine rgb
	band       rgb
	category   [5]rgb
}

func defaultPalette() palette {
	resolved := theme.DefaultTheme().Resolve()
	return palette{
		deep:      hexRGB(resolved.Deep),
		accent:    hexRGB(resolved.Accent),
		soft:      hexRGB(resolved.Soft),
		ink:       hexRGB(resolved.Ink),
		surface:   hexRGB(resolved.Surface),
		edge:      hexRGB(resolved.Edge),
		rule:      hexRGB(resolved.Rule),
		secondary: hexRGB(resolved.Secondary),
		emphasis:  hexRGB(resolved.Emphasis),
		subInk:    hexRGB(resolved.SubInk),
		muted:     hexRGB(resolved.Muted),
		paper:     hexRGB(theme.Paper),

		amount:     hexRGB(resolved.Accent),
		capital:    hexRGB(resolved.Deep),
		marginPos:  hexRGB(theme.Green),
		marginNeg:  hexRGB("#B3261E"),
		marginLine: hexRGB("#C2410C"),
		band:       hexRGB(resolved.Soft),
		category: [5]rgb{
			hexRGB("#C2410C"),
			hexRGB("#0F766E"),
			hexRGB("#7C3AED"),
			hexRGB("#B45309"),
			hexRGB("#64748B"),
		},
	}
}

type faces struct {
	regular     *font.TrueTypeFont
	bold        *font.TrueTypeFont
	regularData []byte
	boldData    []byte
}

func loadFaces(fonts fs.FS) (*faces, error) {
	regularData, err := fs.ReadFile(fonts, RegularFont)
	if err != nil {
		return nil, fmt.Errorf("load regular face %q: %w", RegularFont, err)
	}
	boldData, err := fs.ReadFile(fonts, BoldFont)
	if err != nil {
		return nil, fmt.Errorf("load bold face %q: %w", BoldFont, err)
	}
	regular, err := font.ParseTrueType(regularData)
	if err != nil {
		return nil, fmt.Errorf("parse regular face: %w", err)
	}
	bold, err := font.ParseTrueType(boldData)
	if err != nil {
		return nil, fmt.Errorf("parse bold face: %w", err)
	}
	return &faces{regular: regular, bold: bold, regularData: regularData, boldData: boldData}, nil
}

func (f *faces) width(text string, size float64, bold bool) float64 {
	face := f.regular
	if bold {
		face = f.bold
	}
	return font.MeasureString(face, text, size)
}

// wrap breaks at whitespace, exactly as the quotation engine wraps its one
// description column. A single overlong word is left intact rather than split
// mid-word; the address and method text are prose, not table cells.
func (f *faces) wrap(text string, size, maxWidth float64, bold bool) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range strings.Fields(text) {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if current == "" || f.width(candidate, size, bold) <= maxWidth {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

type textPrim struct {
	text     string
	x        float64
	baseline float64
	size     float64
	bold     bool
	color    rgb
}

type rectPrim struct {
	x, y, w, h float64
	fill       *rgb
	stroke     *rgb
	lw         float64
}

type pathPrim struct {
	path   document.Path
	fill   *rgb
	stroke *rgb
	lw     float64
	dash   []float64
}

type page struct {
	prims []any
}

type canvas struct {
	pages   []*page
	current *page
	y       float64
	faces   *faces
}

func newCanvas(f *faces) *canvas {
	c := &canvas{faces: f}
	c.newPage()
	return c
}

func (c *canvas) newPage() {
	c.current = &page{}
	c.pages = append(c.pages, c.current)
	c.y = marginTop
}

func (c *canvas) ensure(height float64) {
	if c.y+height > contentBot {
		c.newPage()
	}
}

func (c *canvas) text(text string, x, baseline, size float64, bold bool, color rgb) {
	if text == "" {
		return
	}
	c.current.prims = append(c.current.prims, textPrim{
		text: text, x: x, baseline: baseline, size: size, bold: bold, color: color,
	})
}

// textRight draws a run whose right edge lands at x.
func (c *canvas) textRight(text string, x, baseline, size float64, bold bool, color rgb) {
	c.text(text, x-c.faces.width(text, size, bold), baseline, size, bold, color)
}

func (c *canvas) textCenter(text string, center, baseline, size float64, bold bool, color rgb) {
	c.text(text, center-c.faces.width(text, size, bold)/2, baseline, size, bold, color)
}

func (c *canvas) rect(x, y, w, h float64, fill, stroke *rgb, lw float64) {
	c.current.prims = append(c.current.prims, rectPrim{x: x, y: y, w: w, h: h, fill: fill, stroke: stroke, lw: lw})
}

func (c *canvas) line(x1, y1, x2, y2 float64, color rgb, lw float64, dash ...float64) {
	path := document.Path{Segments: []document.PathSegment{
		{Op: document.PathMoveTo, Points: []document.Point{{X: x1, Y: y1}}},
		{Op: document.PathLineTo, Points: []document.Point{{X: x2, Y: y2}}},
	}}
	c.current.prims = append(c.current.prims, pathPrim{path: path, stroke: &color, lw: lw, dash: dash})
}

func (c *canvas) polygon(points []document.Point, fill, stroke *rgb, lw float64) {
	if len(points) < 3 {
		return
	}
	segments := make([]document.PathSegment, 0, len(points)+1)
	segments = append(segments, document.PathSegment{
		Op: document.PathMoveTo, Points: []document.Point{points[0]},
	})
	for _, point := range points[1:] {
		segments = append(segments, document.PathSegment{Op: document.PathLineTo, Points: []document.Point{point}})
	}
	segments = append(segments, document.PathSegment{Op: document.PathClose})
	c.current.prims = append(c.current.prims, pathPrim{
		path: document.Path{Segments: segments}, fill: fill, stroke: stroke, lw: lw,
	})
}

func (c *canvas) render(title string) ([]byte, error) {
	var buf bytes.Buffer
	writer := pdf.NewWriter(&buf)
	target := render.NewPDFRenderer(writer)
	target.RegisterTTFont("inter", c.faces.regular, c.faces.regularData)
	target.RegisterTTFont("interb", c.faces.bold, c.faces.boldData)

	if err := target.BeginDocument(document.DocumentMetadata{
		Title:    title,
		Creator:  "vera report engine",
		Producer: "vera report engine",
	}); err != nil {
		return nil, fmt.Errorf("begin report document: %w", err)
	}
	for index, pg := range c.pages {
		if err := target.BeginPage(document.Size{Width: PageWidth, Height: PageHeight}); err != nil {
			return nil, fmt.Errorf("begin report page %d: %w", index+1, err)
		}
		for _, prim := range pg.prims {
			switch value := prim.(type) {
			case textPrim:
				family := "inter"
				if value.bold {
					family = "interb"
				}
				if err := target.RenderText(value.text,
					document.Point{X: value.x, Y: value.baseline - value.size},
					document.Style{
						FontFamily: family,
						FontSize:   value.size,
						Color:      value.color.pdf(),
						TextAlign:  document.AlignLeft,
					}); err != nil {
					return nil, fmt.Errorf("draw report text %q: %w", value.text, err)
				}
			case rectPrim:
				style := render.RectStyle{StrokeWidth: value.lw}
				if value.fill != nil {
					color := value.fill.pdf()
					style.FillColor = &color
				}
				if value.stroke != nil {
					color := value.stroke.pdf()
					style.StrokeColor = &color
				}
				if err := target.RenderRect(document.Rectangle{
					X: value.x, Y: value.y, Width: value.w, Height: value.h,
				}, style); err != nil {
					return nil, fmt.Errorf("draw report rect: %w", err)
				}
			case pathPrim:
				style := render.PathStyle{StrokeWidth: value.lw, DashPattern: value.dash}
				if value.fill != nil {
					color := value.fill.pdf()
					style.FillColor = &color
				}
				if value.stroke != nil {
					color := value.stroke.pdf()
					style.StrokeColor = &color
				}
				if err := target.RenderPath(value.path, style); err != nil {
					return nil, fmt.Errorf("draw report path: %w", err)
				}
			}
		}
		if err := target.EndPage(); err != nil {
			return nil, fmt.Errorf("end report page %d: %w", index+1, err)
		}
	}
	if err := target.EndDocument(); err != nil {
		return nil, fmt.Errorf("end report document: %w", err)
	}
	return buf.Bytes(), nil
}
