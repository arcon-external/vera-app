// Package theme is the single source of every colour the quotation document
// and the verification portal paint with.
//
// The brand owns eight colours (four for the quotation, four for the portal),
// stored as text columns and chosen in the admin Company form. They used to
// reach only the handful of spots that named a CSS variable directly: every
// other colour in the document was a literal, written twice — once in the
// stylesheet (backend/template/css/template-fixed.css) and once as a Go
// constant in the native renderer (backend/render/gpdf/layout.go). Changing
// the brand colour therefore recoloured about a quarter of the document while
// the rest stayed blue (issue #335).
//
// Now both representations are generated from here. A Theme is the four raw
// brand colours (possibly empty: empty means "use the default"); Resolve
// derives a Palette of named roles from them, and the stylesheet and the
// native renderer both read the palette. There is exactly one formula per
// role, documented on the role, so a colour cannot drift between the two
// engines again.
//
// Derivation rules:
//
//   - A surface is the soft colour mixed toward white: pale fills are the
//     brand's own tint washed out, which is why they follow a rebrand.
//   - A border or decoration is the soft colour shaded toward the accent:
//     outlines lean into the brand's vivid colour rather than sitting in a
//     fixed blue.
//   - Edges and rules carry a contrast floor against paper white (1.25:1).
//     The default Edge (1.42) and Rule (1.30) sit above it, so the default
//     document is byte-identical; a near-white Soft would otherwise fade to
//     1.12/1.02, so those deepen toward the accent until the floor holds.
//     The floor is checked by widening the mix ratio, never by jumping to a
//     different formula, so a custom brand keeps its hue while gaining
//     visibility. A near-black Soft already contrasts strongly and is
//     untouched.
//   - A text tone holds the deep colour's hue at a fixed saturation and
//     lightness. The document's slates are one hue family at several depths,
//     so each role fixes (S, L) and takes H from deep: re-hue deep and every
//     tone follows. When the base is near-achromatic (saturation below
//     0.15), the role's saturation scales down proportionally, so a grey
//     brand yields a neutral text family instead of parking every tone on
//     red (H 0). Saturated brands sit above the threshold and are unchanged.
//   - Paper white and the discount green are fixed, not derived. White is the
//     absence of ink and green is a semantic colour (a discount reads as
//     good), so neither may move with the brand. They live here as named
//     constants so both representations still resolve through the theme.
//
// The mixing ratios were tuned so that, with the default theme, each derived
// value lands within a few units per channel of the literal it replaced (see
// the role table on Palette and the pinned values in theme_test.go). The
// largest deviation is the emphasis tone's green channel (7 units on
// #2F3654): the two literals it merges (#2F3654 and #26304D) sit on slightly
// different hues, so no single hue-locked value can match both exactly.
//
// This package imports nothing local, so the stylesheet path (backend), the
// native renderer (backend/render/gpdf) and the portal (backend/api) can all
// import it without an import cycle.
package theme

import (
	"html/template"
	"math"
	"strconv"
)

// The default brand: what an empty theme column means. These are the values
// the stylesheet baked in before the theme existed, kept so that a profile
// that never set a colour renders exactly what it always rendered.
const (
	DefaultDeep   = "#1A237E"
	DefaultAccent = "#284AF3"
	DefaultSoft   = "#D6E4FF"
	// DefaultInk is lowercase, exactly as the stylesheet and the theme
	// migration always wrote it. Hex case carries no colour, but the executed
	// stylesheet is byte-compared in tests, so the default keeps its bytes.
	DefaultInk = "#1a1a1a"
)

// Fixed colours. Deliberately not derived (see the package comment): their
// identity must survive any rebrand.
const (
	// Paper is the page card, the table header text and the QR card.
	Paper = "#FFFFFF"
	// Green is the whole-quotation discount row. A discount reads as good,
	// the way a price reads as ink; neither follows the brand.
	Green = "#228B22"
)

// Theme is the four raw brand colours, each "#RRGGBB" or empty. Empty means
// "use the default" and is what an unset theme column carries.
type Theme struct {
	Deep   string
	Accent string
	Soft   string
	Ink    string
}

// DefaultTheme is the brand before anyone chose one.
func DefaultTheme() Theme {
	return Theme{Deep: DefaultDeep, Accent: DefaultAccent, Soft: DefaultSoft, Ink: DefaultInk}
}

// Palette is the quotation document's full colour set: the four bases plus
// every derived role.
//
// Role table: role → formula → default value → every literal it replaced.
// Deltas are per channel against the farthest literal.
//
//	Surface   mix(Soft, white, 0.82)      #F8FAFF  meta/card/subject/totals
//	          fills (#F8FAFF #F8FBFF #F7FAFF #F9FBFF #F6F9FF), max Δ2
//	Edge      mixFloor(Soft, Accent, 0.075, floor 1.25)
//	          #C9D8FE  every component outline (#C6D4FF #C9D9FF #CCD9FF
//	          #CAD8FF #C5D5FF #CEDBFF), max Δ5
//	Rule      mixFloor(Soft, Accent, 0.015, floor 1.25)
//	          #D3E2FF  hairline dividers (lead rule #D8E2FF, table rules
//	          #CFE0FF), max Δ5
//	Secondary deep hue, S 0.26, L 0.315  #3B3F65  second-paragraph text
//	          (lead #3A4466, verify caption #3A4466, closing #364066), max Δ5
//	Emphasis  deep hue, S 0.30, L 0.245  #2C2F51  strong values (meta
//	          #2F3654, subject #26304D, totals #2F3654), max Δ7
//	SubInk    deep hue, S 0.18, L 0.37   #4D506F  sub-item rows (#4A5470),
//	          max Δ4
//	SubNo     deep hue, S 0.12, L 0.42   #5E6178  sub numbers + per-row
//	          discount cells (#5B6478), max Δ3
//	Muted     deep hue, S 0.18, L 0.625  #8E91B1  quiet annotations
//	          (#8A93B2), max Δ4
//	Rail      mix(Soft, Emphasis, 0.17)   #B9C5E1  sub-item rail (#B7C6E2),
//	          max Δ2
//	Decor0    mix(Soft, white, 0.15)      #DCE8FF  decoration gradient
//	          start (#DCE8FF), Δ0
//	Decor1    mix(Soft, Accent, 0.065)    #CBDAFE  decoration gradient
//	          end (#C9DCFF), max Δ2
//	Page0     mix(Soft, white, 0.72)      #F4F7FF  page gradient start
//	          (CSS only; the native renderer paints no page background),
//	          (#F4F7FF), Δ0
//	Page1     mix(Soft, white, 0.47)      #E9F1FF  page gradient end
//	          (#EAF0FF), max Δ1
type Palette struct {
	Deep   string
	Accent string
	Soft   string
	Ink    string

	Surface   string
	Edge      string
	Rule      string
	Secondary string
	Emphasis  string
	SubInk    string
	SubNo     string
	Muted     string
	Rail      string
	Decor0    string
	Decor1    string
	Page0     string
	Page1     string

	Paper string
	Green string
}

// PortalPalette is the verification portal's brand set. The portal keeps its
// own status colours (valid/superseded/mismatch/not-found) as fixed literals
// in its template: a brand colour must never be able to make a "mismatch"
// verdict look like a "valid" one, so the status set is not derived from
// anything here. Everything else brand-facing is:
//
//	Edge      mixFloor(Soft, Accent, 0.075, floor 1.25)
//	          sheet outline + section rules (#C9D8FE family, max Δ5)
//	Secondary deep hue, S 0.26, L 0.315    muted copy (#3B3F65, max Δ5)
//	Page0     mix(Soft, white, 0.72)        page gradient start (#F4F7FF)
//	Page1     mix(Soft, white, 0.47)        page gradient end (#E9F1FF, max Δ1)
//	Section   mix(Soft, white, 0.55)        the lookup panel's background —
//	          the role ThemePSoft was added for and never wired to (#EDF3FF)
//	Paper     fixed #FFFFFF                 sheet + inputs (was #ffffff)
//	FieldRule mix(Soft, white, 0.40)        field dividers (was #E6ECFB,
//	          max Δ4)
//	Label     deep hue, S 0.15, L 0.56      field labels + footnotes
//	          (was #7A85A3, max Δ4)
//	Strong    deep hue, S 0.31, L 0.235     field values + diff records
//	          (was #26304D, max Δ3)
//	Focus     accent as an rgba() fragment  the lookup field's focus ring
//	Shadow    deep as an rgba() fragment    the sheet's drop shadow
//
// Focus and Shadow are template.CSS: they are complete rgba() expressions
// built from validated channels, trusted for the stylesheet context the way
// a plain colour string would be.
type PortalPalette struct {
	Deep   string
	Accent string
	Ink    string

	Edge      string
	Secondary string
	Page0     string
	Page1     string
	Section   string
	Paper     string
	FieldRule string
	Label     string
	Strong    string

	Focus  template.CSS
	Shadow template.CSS
}

// Resolve derives the quotation palette from the four brand colours. A field
// that is not a strict #RRGGBB falls back to its default, so a colour can
// never break a render: the columns are validated on write, and this is the
// second net.
func (t Theme) Resolve() Palette {
	deep := fallback(t.Deep, DefaultDeep)
	accent := fallback(t.Accent, DefaultAccent)
	soft := fallback(t.Soft, DefaultSoft)
	ink := fallback(t.Ink, DefaultInk)
	emphasis := hslTone(deep, 0.30, 0.245)
	return Palette{
		Deep:   deep,
		Accent: accent,
		Soft:   soft,
		Ink:    ink,

		Surface:   mix(soft, "#FFFFFF", 0.82),
		Edge:      mixFloor(soft, accent, 0.075),
		Rule:      mixFloor(soft, accent, 0.015),
		Secondary: hslTone(deep, 0.26, 0.315),
		Emphasis:  emphasis,
		SubInk:    hslTone(deep, 0.18, 0.37),
		SubNo:     hslTone(deep, 0.12, 0.42),
		Muted:     hslTone(deep, 0.18, 0.625),
		Rail:      mix(soft, emphasis, 0.17),
		Decor0:    mix(soft, "#FFFFFF", 0.15),
		Decor1:    mix(soft, accent, 0.065),
		Page0:     mix(soft, "#FFFFFF", 0.72),
		Page1:     mix(soft, "#FFFFFF", 0.47),

		Paper: Paper,
		Green: Green,
	}
}

// ResolvePortal derives the portal's brand set. Same fallback rule as
// Resolve: an invalid field renders as its default, never as a broken page.
func (t Theme) ResolvePortal() PortalPalette {
	deep := fallback(t.Deep, DefaultDeep)
	accent := fallback(t.Accent, DefaultAccent)
	soft := fallback(t.Soft, DefaultSoft)
	ink := fallback(t.Ink, DefaultInk)
	ar, ag, ab := channels(accent)
	dr, dg, db := channels(deep)
	return PortalPalette{
		Deep:   deep,
		Accent: accent,
		Ink:    ink,

		Edge:      mixFloor(soft, accent, 0.075),
		Secondary: hslTone(deep, 0.26, 0.315),
		Page0:     mix(soft, "#FFFFFF", 0.72),
		Page1:     mix(soft, "#FFFFFF", 0.47),
		Section:   mix(soft, "#FFFFFF", 0.55),
		Paper:     Paper,
		FieldRule: mix(soft, "#FFFFFF", 0.40),
		Label:     hslTone(deep, 0.15, 0.56),
		Strong:    hslTone(deep, 0.31, 0.235),

		Focus:  template.CSS(formatRGBA(ar, ag, ab, 0.28)),
		Shadow: template.CSS(formatRGBA(dr, dg, db, 0.08)),
	}
}

// fallback keeps a strict #RRGGBB value and replaces anything else with the
// default. Stored colours are validated on write; this guards the template
// and renderer paths against anything that slipped through.
func fallback(v, def string) string {
	if isHexColor(v) {
		return v
	}
	return def
}

// isHexColor reports whether s is a strict #RRGGBB colour, the same rule the
// store validates theme columns with.
func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// channels splits a validated #RRGGBB colour into its 8-bit channels.
func channels(s string) (uint8, uint8, uint8) {
	return hexByte(s[1], s[2]), hexByte(s[3], s[4]), hexByte(s[5], s[6])
}

func hexByte(hi, lo byte) uint8 {
	return hexVal(hi)<<4 | hexVal(lo)
}

func hexVal(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// mix returns the linear interpolation between two validated colours in sRGB:
// t 0 is a, t 1 is b. Channels round half up, matching the tuning in the
// role table.
func mix(a, b string, t float64) string {
	ar, ag, ab := channels(a)
	br, bg, bb := channels(b)
	lerp := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x) + t*(float64(y)-float64(x))))
	}
	return formatHex(lerp(ar, br), lerp(ag, bg), lerp(ab, bb))
}

// hslTone returns the colour at hue H of the given base with saturation s
// and lightness l. Text tones are one hue family at several depths, so each
// role fixes (s, l) and takes H from the brand's deep colour: re-hue deep
// and every tone follows. When the base is near-achromatic (saturation below
// 0.15) the role's saturation scales down proportionally, so a grey brand
// yields a neutral text family instead of parking every tone on red (H 0).
// Saturated brands sit above the threshold and resolve exactly as before.
func hslTone(base string, s, l float64) string {
	r, g, b := channels(base)
	h, bs, _ := rgbToHSL(r, g, b)
	const achromaticThreshold = 0.15
	if bs < achromaticThreshold {
		s = s * bs / achromaticThreshold
	}
	return hslToRGB(h, s, l)
}

// rgbToHSL converts 8-bit channels to hue degrees [0,360) and unit
// saturation and lightness, following the CSS Color 3 definition.
func rgbToHSL(r, g, b uint8) (h, s, l float64) {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255
	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	l = (max + min) / 2
	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	return h * 60, s, l
}

// hslToRGB converts hue degrees and unit saturation/lightness back to a
// #RRGGBB colour, following the CSS Color 3 definition.
func hslToRGB(h, s, l float64) string {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var rf, gf, bf float64
	switch {
	case h < 60:
		rf, gf, bf = c, x, 0
	case h < 120:
		rf, gf, bf = x, c, 0
	case h < 180:
		rf, gf, bf = 0, c, x
	case h < 240:
		rf, gf, bf = 0, x, c
	case h < 300:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	toByte := func(v float64) uint8 {
		return uint8(math.Round((v + m) * 255))
	}
	return formatHex(toByte(rf), toByte(gf), toByte(bf))
}

func formatHex(r, g, b uint8) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{
		'#',
		digits[r>>4], digits[r&0x0F],
		digits[g>>4], digits[g&0x0F],
		digits[b>>4], digits[b&0x0F],
	})
}

// edgeContrastFloor is the minimum WCAG contrast of an edge or rule against
// paper white. The default Edge (1.42) and Rule (1.30) sit above it, so the
// default document resolves exactly as before; a near-white Soft would
// otherwise fade to 1.12/1.02, so those deepen toward the accent until the
// floor holds. A near-black Soft already contrasts strongly and is untouched.
const edgeContrastFloor = 1.25

// mixFloor mixes soft toward accent starting at t0, widening the ratio until
// the result contrasts against paper at the floor. The default brand already
// passes, so it returns mix(soft, accent, t0) byte-identically; a pale brand
// returns a deeper mix in the same hue family.
func mixFloor(soft, accent string, t0 float64) string {
	for t := t0; t < 1; t += 0.01 {
		if c := mix(soft, accent, t); contrastAgainstPaper(c) >= edgeContrastFloor {
			return c
		}
	}
	return mix(soft, accent, 1)
}

// contrastAgainstPaper reports the WCAG contrast of hex against paper white.
func contrastAgainstPaper(hex string) float64 {
	r, g, b := channels(hex)
	pr, pg, pb := channels(Paper)
	la := relativeLuminance(r, g, b)
	lb := relativeLuminance(pr, pg, pb)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance follows WCAG 2.1: linearised sRGB weighted by eye.
func relativeLuminance(r, g, b uint8) float64 {
	linear := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

func formatRGBA(r, g, b uint8, a float64) string {
	// The channels are integers and the alpha carries at most two decimals,
	// so 'f' with precision 2 renders exactly: "rgba(40, 74, 243, 0.28)".
	return "rgba(" + strconv.Itoa(int(r)) + ", " + strconv.Itoa(int(g)) + ", " +
		strconv.Itoa(int(b)) + ", " + strconv.FormatFloat(a, 'f', 2, 64) + ")"
}
