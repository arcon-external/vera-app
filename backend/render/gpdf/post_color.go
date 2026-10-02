package gpdf

// Colour parsing shared by the draw layer and the post-processes.
//
// The engine carries colours as the same "#RRGGBB" strings the stylesheet uses.
// That keeps one representation end to end: the stylesheet constants, the layout
// constants and the sentinel colours are all readable as the hex they came from,
// and a post-process can restore a real colour from a string without a second
// colour type to convert through.

import "github.com/gpdf-dev/gpdf/pdf"

// parseHex reads "#RRGGBB". Unparseable input yields black rather than a panic:
// the caller has already validated the value, and a colour must never be able to
// take down a quotation.
func parseHex(s string) (uint8, uint8, uint8) {
	var v uint32
	for i := 1; i < len(s) && i <= 6; i++ {
		v <<= 4
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			v |= uint32(c - '0')
		case c >= 'a' && c <= 'f':
			v |= uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= uint32(c-'A') + 10
		}
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v)
}

// rgbOf builds a gpdf colour from 8-bit channels.
func rgbOf(r, g, b uint8) *pdf.Color {
	c := pdf.RGB(float64(r)/255, float64(g)/255, float64(b)/255)
	return &c
}

// colorOf returns the colour for a stylesheet hex, or nil when the box is
// unpainted.
func colorOf(hex string) *pdf.Color {
	if hex == "" {
		return nil
	}
	r, g, b := parseHex(hex)
	return rgbOf(r, g, b)
}

// derefColor yields a colour value for document.Style, which takes pdf.Color by
// value rather than by pointer the way the render styles do.
func derefColor(c *pdf.Color) pdf.Color {
	if c == nil {
		return pdf.Black
	}
	return *c
}
