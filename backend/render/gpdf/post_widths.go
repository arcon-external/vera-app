package gpdf

// Restoring full-precision glyph advances after gpdf has written the file.
//
// gpdf builds a CID font's /W array with
//
//	width: w * 1000 / unitsPerEm
//
// on integers. For a font with unitsPerEm 2048 that turns Helvetica's 666.99219
// per-mille advance into 666: the per-glyph error is whatever the division
// truncated, up to 1/1000 em. At the 8.25pt body size of a quotation that is
// under a hundredth of a point per glyph, but it accumulates along a line — a
// forty-glyph row drifts a third of a point, which is half a pixel at 110dpi,
// and the row then no longer registers with the reference at all.
//
// The error is invisible in any single glyph and shows up only as a uniform
// "ghosting" of a whole line, so it is easy to misread as a rasteriser
// difference and go looking in the wrong place.
//
// It cannot be fixed from outside the render layer: gpdf emits the width array
// and the text stream together, and the stream addresses glyphs by ID, so
// adjusting the /W values after the fact is the only lever. PDF allows real
// numbers in /W, so the integers are simply replaced with the exact per-mille
// advances read back out of the embedded font's own hmtx.

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reCIDFontBase = regexp.MustCompile(`/BaseFont /([A-Za-z0-9]+)`)
	reDefaultW    = regexp.MustCompile(`/DW (\d+)`)
)

// fixGlyphWidths rewrites every CID font's /W and /DW at full precision.
//
// The document is rebuilt with a fresh xref rather than patched in place,
// because replacing `666` with `666.99219` changes object lengths and every
// offset after the first edited object would be wrong.
func fixGlyphWidths(doc []byte, advances map[string]map[int]float64) ([]byte, error) {
	offsets, err := parseXrefOffsets(doc)
	if err != nil {
		return nil, err
	}

	maxObj := 0
	for n := range offsets {
		if n > maxObj {
			maxObj = n
		}
	}

	edited := 0
	bodies := make(map[int][]byte, maxObj)
	for n := 1; n <= maxObj; n++ {
		body, err := objectBody(doc, offsets[n])
		if err != nil {
			continue
		}
		if fixed, ok := repairWidths(body, advances); ok {
			body = fixed
			edited++
		}
		bodies[n] = body
	}
	if edited == 0 {
		return doc, nil
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	newOffsets := make(map[int]int, maxObj)
	for n := 1; n <= maxObj; n++ {
		body, ok := bodies[n]
		if !ok {
			continue
		}
		newOffsets[n] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", n)
		out.Write(body)
		out.WriteString("\nendobj\n")
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", maxObj+1)
	out.WriteString("0000000000 65535 f \n")
	for n := 1; n <= maxObj; n++ {
		if _, ok := newOffsets[n]; !ok {
			out.WriteString("0000000000 65535 f \n")
			continue
		}
		fmt.Fprintf(&out, "%010d 00000 n \n", newOffsets[n])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		maxObj+1, xref)
	return out.Bytes(), nil
}

// repairWidths replaces the integers in one object's /W and /DW.
//
// Only CIDFontType2 dictionaries are touched: /W appears nowhere else in the
// file, but /DW is a common key name and a blanket substitution would corrupt
// any other dictionary that happened to carry one.
func repairWidths(body []byte, advances map[string]map[int]float64) ([]byte, bool) {
	if !bytes.Contains(body, []byte("/CIDFontType2")) {
		return body, false
	}
	fam := reCIDFontBase.FindSubmatch(body)
	if len(fam) < 2 {
		return body, false
	}
	table, ok := advances[string(fam[1])]
	if !ok {
		return body, false
	}

	widened, changed := widenWidthsArray(body, table)
	if !changed {
		return body, false
	}

	out := reDefaultW.ReplaceAllFunc(widened, func(m []byte) []byte {
		tok := strings.TrimSpace(string(m[strings.Index(string(m), " ")+1:]))
		g, err := strconv.Atoi(tok)
		if err != nil {
			return m
		}
		w, ok := table[g]
		if !ok {
			return m
		}
		return []byte(fmt.Sprintf("/DW %.5f", w))
	})

	return out, !bytes.Equal(out, body)
}

// widenWidthsArray rewrites the integers in a /W array to full precision.
//
// The array nests: `/W [ 3 [277] 17 [277] 36 [722] ]` pairs a starting glyph ID
// with a list of advances for consecutive glyphs. A regex cannot delimit it —
// `[^\]]*` stops at the first inner bracket and leaves the remainder of the
// array dangling in the dictionary, which parses as garbage keys. The extent is
// therefore found by matching brackets.
func widenWidthsArray(body []byte, table map[int]float64) ([]byte, bool) {
	start := bytes.Index(body, []byte("/W ["))
	if start < 0 {
		return body, false
	}
	open := start + len("/W ")
	depth := 0
	end := -1
	for i := open; i < len(body); i++ {
		switch body[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return body, false
	}

	inner := body[open+1 : end]

	// Structure is: a bare number is the run's starting glyph ID; the bracketed
	// list that follows holds that run's advances, one per consecutive glyph, so
	// the Nth value in a run belongs to startGID+N.
	var out bytes.Buffer
	out.WriteString("/W [ ")
	changed := false
	gid := 0
	for i := 0; i < len(inner); {
		switch {
		case inner[i] == '[':
			depth := 1
			i++
			off := 0
			// The run's own brackets are re-emitted: without them the array
			// becomes a flat list in which PDF cannot tell a run header from a
			// width, and every glyph after the first is read at the wrong
			// advance.
			out.WriteString("[ ")
			for i < len(inner) && depth > 0 {
				switch {
				case inner[i] == '[':
					depth++
				case inner[i] == ']':
					depth--
					if depth == 0 {
						out.WriteString("] ")
					}
				case inner[i] >= '0' && inner[i] <= '9':
					j := i
					for j < len(inner) && inner[j] >= '0' && inner[j] <= '9' {
						j++
					}
					g, _ := strconv.Atoi(string(inner[i:j]))
					if w, ok := table[gid+off]; ok && float64(g) != w {
						fmt.Fprintf(&out, "%.5f ", w)
						changed = true
					} else {
						fmt.Fprintf(&out, "%d ", g)
					}
					off++
					i = j
					continue
				}
				i++
			}
		case inner[i] >= '0' && inner[i] <= '9':
			j := i
			for j < len(inner) && inner[j] >= '0' && inner[j] <= '9' {
				j++
			}
			g, _ := strconv.Atoi(string(inner[i:j]))
			fmt.Fprintf(&out, "%d ", g)
			gid = g
			i = j
		default:
			i++
		}
	}
	out.WriteByte(']')

	res := make([]byte, 0, len(body)+64)
	res = append(res, body[:start]...)
	res = append(res, out.Bytes()...)
	res = append(res, body[end+1:]...)
	return res, changed
}

// advanceTable returns gid -> per-mille advance for one embedded font program.
//
// The values are read back out of the same Inter program gpdf embeds, so the
// /W array the rasteriser measures against is the font's own metric rather than
// a truncated copy of it.
func (f *metricFace) advanceTable() map[int]float64 {
	if f == nil || f.upem == 0 {
		return nil
	}
	out := make(map[int]float64, len(f.hmtx))
	for gid, w := range f.hmtx {
		out[gid] = float64(w) * 1000 / f.upem
	}
	return out
}
