package gpdf

import (
	"fmt"
	"strings"

	"vera-app/backend/theme"
)

// Render-time sentinel colours.
//
// A gradient rect and a clipped rect are drawn with a stand-in fill and the
// real shading or clip is substituted into the finished file by finding that
// fill. A fixed stand-in cannot be safe against a derived palette: the first
// iteration used a near-white fixed sentinel for gradients, but a soft cream
// resolves Surface to exactly that sentinel, so the post-pass consumed the
// recipient card as a gradient and left the decoration flat (issue 335,
// second pass). No fixed six-digit hex is outside a derived continuum.
//
// So the stand-ins are chosen per render from a small candidate list, skipping
// any candidate that collides with a resolved palette role, and failing loudly
// when none is free. The default palette takes the first candidate in each
// list, so the default document's bytes are unchanged. The gradients slice
// already carries coordinates, but matching by position would mean parsing
// every path's geometry out of the content stream; matching by a colour known
// to be absent is cheaper and, with the collision check, correct.

// gradientCandidates are the fills a gradient rect may be drawn with, in
// preference order. The first is the historic sentinel, kept so the default
// render is byte-identical; the rest are saturated colours no derived surface,
// edge, rule or text tone lands on except by an explicit brand choice, in
// which case the chooser skips them.
var gradientCandidates = [][3]uint8{
	{0xFE, 0xFD, 0xFC},
	{0xFF, 0x00, 0xFF},
	{0x00, 0xFF, 0xFF},
	{0x7F, 0xFF, 0x00},
}

// clipCandidates are the fills a clipped rect may be drawn with, in preference
// order. Same contract as the gradient list, and disjoint from it: the two
// post-passes find their own fills by colour, so the two choices must differ.
var clipCandidates = [][3]uint8{
	{0x01, 0x02, 0x03},
	{0x02, 0xFF, 0x01},
	{0xFF, 0x02, 0x02},
	{0x02, 0x02, 0xFF},
}

// chooseSentinels picks a gradient and a clip sentinel free of the palette.
// It returns an error when every candidate collides, which Render surfaces
// instead of silently corrupting the document. The error names every painted
// role and its value, so an admin reading the log can see exactly which of
// their brand colours collided.
func chooseSentinels(pal theme.Palette) ([3]uint8, [3]uint8, error) {
	used := paletteColours(pal)
	var grad [3]uint8
	foundGrad := false
	for _, c := range gradientCandidates {
		if !used[sentinelKey(c)] {
			grad = c
			foundGrad = true
			break
		}
	}
	if !foundGrad {
		return grad, [3]uint8{}, fmt.Errorf("gpdf: no free gradient sentinel colour: palette collides with all %d candidates (%s)", len(gradientCandidates), describePalette(pal))
	}
	used[sentinelKey(grad)] = true
	for _, c := range clipCandidates {
		if !used[sentinelKey(c)] {
			return grad, c, nil
		}
	}
	return grad, [3]uint8{}, fmt.Errorf("gpdf: no free clip sentinel colour: palette collides with all %d candidates (%s)", len(clipCandidates), describePalette(pal))
}

// paletteRole names one painted role: the role name an admin recognises from
// the theme, with the resolved value that actually collided.
type paletteRole struct {
	name  string
	value string
}

// paletteRoles lists every colour the document can paint with, in a fixed
// order, so both the collision set and the error text read from one list.
func paletteRoles(pal theme.Palette) []paletteRole {
	return []paletteRole{
		{"Deep", pal.Deep}, {"Accent", pal.Accent}, {"Soft", pal.Soft}, {"Ink", pal.Ink},
		{"Surface", pal.Surface}, {"Edge", pal.Edge}, {"Rule", pal.Rule},
		{"Secondary", pal.Secondary}, {"Emphasis", pal.Emphasis},
		{"SubInk", pal.SubInk}, {"SubNo", pal.SubNo}, {"Muted", pal.Muted},
		{"Rail", pal.Rail}, {"Decor0", pal.Decor0}, {"Decor1", pal.Decor1},
		{"Page0", pal.Page0}, {"Page1", pal.Page1},
		{"Paper", pal.Paper}, {"Green", pal.Green},
	}
}

// describePalette renders every painted role as "Name #VALUE" pairs for the
// exhaustion error, so the log names the colliding colours instead of only
// counting the candidates they hit.
func describePalette(pal theme.Palette) string {
	roles := paletteRoles(pal)
	parts := make([]string, 0, len(roles))
	for _, r := range roles {
		if r.value == "" {
			continue
		}
		parts = append(parts, r.name+" "+strings.ToUpper(r.value))
	}
	return strings.Join(parts, ", ")
}

// paletteColours returns every colour the document can paint with, keyed by
// uppercase hex without a leading hash, so a candidate matches regardless of
// the case the palette carries (Ink's default is lowercase).
func paletteColours(pal theme.Palette) map[string]bool {
	roles := paletteRoles(pal)
	out := make(map[string]bool, len(roles))
	for _, r := range roles {
		if r.value == "" {
			continue
		}
		out[strings.ToUpper(strings.TrimPrefix(r.value, "#"))] = true
	}
	return out
}

func sentinelKey(c [3]uint8) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{
		digits[c[0]>>4], digits[c[0]&0x0F],
		digits[c[1]>>4], digits[c[1]&0x0F],
		digits[c[2]>>4], digits[c[2]&0x0F],
	})
}
