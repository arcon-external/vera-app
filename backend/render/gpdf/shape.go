package gpdf

// Glyph shaping.
//
// Measuring a line by summing raw advance widths is wrong for a proportional
// font, and wrong enough to matter here: Inter kerns about two percent of a
// typical quotation line and substitutes the fi/ffi/ffl ligatures, which are
// narrower than the letters they replace. Both shrink the line, so an unkerned
// measurement wraps earlier than the browser does, and the two renderers then
// disagree about which word starts a line — which cascades into row heights and
// therefore into pagination.
//
// Inter carries no legacy `kern` table; the pair adjustments live in GPOS and
// the ligatures in GSUB, so both are read from the font program itself. Only the
// features the browser applies by default are read: `kern` and `liga`.
//
// This is a shaper for measurement and placement, not a general text engine.
// It does no bidirectional reordering, no mark positioning and no contextual
// substitution, because the document is left-to-right Latin text set from
// prepared strings. A document that needed any of those would need HarfBuzz.

// shaper turns a string into positioned glyphs for one font program.
type shaper struct {
	cmap        map[rune]int
	hmtx        map[int]uint16
	kern        map[[2]int]int
	ligatures   map[int][]ligature
	upem        float64
	fallbackGID int
	numHMetrics int
}

// ligature is one GSUB ligature: the glyphs it replaces after the first, and the
// glyph it becomes.
type ligature struct {
	components []int
	glyph      int
}

// glyphRun is one shaped glyph: its id, its x offset from the pen position, and
// its advance.
type glyphRun struct {
	text    string
	gid     int
	dx      float64
	advance float64
}

// parseShaper reads cmap, hmtx, GPOS kerning and GSUB ligatures from a font
// program. A table it cannot make sense of is skipped rather than fatal: the
// worst outcome of a missing kern table is a line that wraps a word early, and
// a quotation must still print.
func parseShaper(d []byte) *shaper {
	s := &shaper{
		cmap:        map[rune]int{},
		hmtx:        map[int]uint16{},
		kern:        map[[2]int]int{},
		ligatures:   map[int][]ligature{},
		fallbackGID: 0,
	}
	tabs := tableDir(d)

	if h, ok := tabs["head"]; ok && h[0]+20 <= len(d) {
		s.upem = float64(be16(d[h[0]+18 : h[0]+20]))
	}
	if h, ok := tabs["hhea"]; ok && h[0]+36 <= len(d) {
		s.numHMetrics = int(be16(d[h[0]+34 : h[0]+36]))
	}
	if hm, ok := tabs["hmtx"]; ok {
		for g := 0; g < s.numHMetrics; g++ {
			p := hm[0] + 4*g
			if p+2 > len(d) {
				break
			}
			s.hmtx[g] = be16(d[p : p+2])
		}
	}
	if cm, ok := tabs["cmap"]; ok {
		s.cmap = parseCmap4(d, cm[0])
	}
	if g, ok := tabs["GPOS"]; ok {
		readKerning(d, g[0], s.kern)
	}
	if g, ok := tabs["GSUB"]; ok {
		readLigatures(d, g[0], s.ligatures)
	}
	return s
}

// tableDir maps each table's tag to its offset and length.
func tableDir(d []byte) map[string][2]int {
	out := map[string][2]int{}
	if len(d) < 12 {
		return out
	}
	n := int(be16(d[4:6]))
	for i := 0; i < n; i++ {
		o := 12 + 16*i
		if o+16 > len(d) {
			break
		}
		out[string(d[o:o+4])] = [2]int{
			int(be32(d[o+8 : o+12])), int(be32(d[o+12 : o+16])),
		}
	}
	return out
}

// featureLookupIndices returns the lookup indices the named features turn on.
//
// Both FeatureList and LookupList are read straight from the layout table rather
// than through the ScriptList, because the feature this needs -- liga -- is on by
// default in every script the document uses, and the script list exists to say
// which lookups apply to which language system. Reading it and unioning the
// languages would be more code and would reach the same answer for Latin text.
func featureLookupIndices(d []byte, base int, wanted map[string]bool) []uint16 {
	// Layout-table header: majorVersion, minorVersion, ScriptList, FeatureList,
	// LookupList. FeatureList is the third offset, at +6.
	featOff := base + int(be16(d[base+6:base+8]))
	if featOff+2 > len(d) {
		return nil
	}
	count := int(be16(d[featOff : featOff+2]))

	var out []uint16
	for i := 0; i < count; i++ {
		rec := featOff + 2 + 6*i
		if rec+6 > len(d) {
			break
		}
		if !wanted[string(d[rec:rec+4])] {
			continue
		}
		// The record's offset is relative to the FeatureList, and points at a
		// table of { lookupCount, lookupIndices[] }.
		tbl := featOff + int(be16(d[rec+4:rec+6]))
		if tbl+2 > len(d) {
			continue
		}
		n := int(be16(d[tbl : tbl+2]))
		for j := 0; j < n; j++ {
			p := tbl + 2 + 2*j
			if p+2 > len(d) {
				break
			}
			out = append(out, be16(d[p:p+2]))
		}
	}
	return out
}

// lookupSubtables resolves a lookup index to its subtable offsets.
func lookupSubtables(d []byte, base int, idx uint16) []int {
	// LookupList is { lookupCount, Offset16 lookupOffsets[] }.
	listOff := base + int(be16(d[base+8:base+10]))
	if listOff+2 > len(d) {
		return nil
	}
	p := listOff + 2 + 2*int(idx)
	if p+2 > len(d) {
		return nil
	}
	tbl := base + int(be16(d[p:p+2]))
	if tbl+6 > len(d) {
		return nil
	}
	n := int(be16(d[tbl+4 : tbl+6]))
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		sp := tbl + 6 + 2*i
		if sp+2 > len(d) {
			break
		}
		out = append(out, base+int(be16(d[sp:sp+2])))
	}
	return out
}

// lookupType reports a lookup's type, following an Extension lookup to the type
// it wraps.
func lookupType(d []byte, base int, idx uint16) int {
	listOff := base + int(be16(d[base+8:base+10]))
	p := listOff + 2 + 2*int(idx)
	if p+2 > len(d) {
		return 0
	}
	tbl := base + int(be16(d[p:p+2]))
	if tbl+6 > len(d) {
		return 0
	}
	lt := int(be16(d[tbl : tbl+2]))
	if lt != 7 { // 7 = Extension
		return lt
	}
	subs := lookupSubtables(d, base, idx)
	if len(subs) == 0 || subs[0]+4 > len(d) {
		return 0
	}
	return int(be16(d[subs[0]+2 : subs[0]+4]))
}

// readKerning fills pairs from the GPOS `kern` feature's pair-position lookups.
func readKerning(d []byte, base int, out map[[2]int]int) {
	for _, li := range featureLookupIndices(d, base, map[string]bool{"kern": true}) {
		if lookupType(d, base, li) != 2 { // 2 = PairPos
			continue
		}
		for _, sub := range lookupSubtables(d, base, li) {
			readPairPos(d, sub, out)
		}
	}
}

func readPairPos(d []byte, sub int, out map[[2]int]int) {
	if sub+10 > len(d) {
		return
	}
	covOff := sub + int(be16(d[sub+2:sub+4]))
	cov := coverage(d, covOff)
	if cov == nil {
		return
	}
	switch be16(d[sub : sub+2]) {
	case 1:
		setOff := sub + int(be16(d[sub+4:sub+6]))
		for gid, ci := range cov {
			p := setOff + 2*ci
			if p+2 > len(d) {
				continue
			}
			count := int(be16(d[p : p+2]))
			base := p + 2
			for j := 0; j < count; j++ {
				rp := base + 2*j
				if rp+6 > len(d) {
					break
				}
				second := int(be16(d[rp : rp+2]))
				val := int(int16(be16(d[rp+4 : rp+6])))
				if val != 0 {
					out[[2]int{gid, second}] = val
				}
			}
		}
	case 2:
		c1 := sub + int(be16(d[sub+6:sub+8]))
		c2 := sub + int(be16(d[sub+8:sub+10]))
		class1 := classDef(d, c1)
		class2 := classDef(d, c2)
		if class1 == nil || class2 == nil {
			return
		}
		v2Count := int(be16(d[sub+14 : sub+16]))
		matrix := sub + int(be16(d[sub+10:sub+12]))
		for gid, c := range cov {
			row := matrix + 2*c*v2Count
			for gid2, c2 := range class2 {
				p := row + 2*c2
				if p+2 > len(d) {
					break
				}
				val := int(int16(be16(d[p : p+2])))
				if val != 0 {
					out[[2]int{gid, gid2}] = val
				}
			}
			_ = class1
		}
	}
}

// readLigatures fills the ligature map from the GSUB `liga` feature.
func readLigatures(d []byte, base int, out map[int][]ligature) {
	for _, li := range featureLookupIndices(d, base, map[string]bool{"liga": true}) {
		if lookupType(d, base, li) != 4 { // 4 = LigatureSubst
			continue
		}
		for _, sub := range lookupSubtables(d, base, li) {
			readLigatureSubst(d, sub, out)
		}
	}
}

func readLigatureSubst(d []byte, sub int, out map[int][]ligature) {
	if sub+6 > len(d) {
		return
	}
	cov := coverage(d, sub+int(be16(d[sub+2:sub+4])))
	if cov == nil {
		return
	}
	setOff := sub + int(be16(d[sub+4:sub+6]))
	for gid, ci := range cov {
		p := setOff + 2*ci
		if p+2 > len(d) {
			continue
		}
		count := int(be16(d[p : p+2]))
		base := p + 2
		for j := 0; j < count; j++ {
			lp := base + 2*j
			if lp+2 > len(d) {
				break
			}
			ligOff := base + int(be16(d[lp:lp+2]))
			if ligOff+4 > len(d) {
				continue
			}
			out[gid] = append(out[gid], ligature{
				glyph:      int(be16(d[ligOff : ligOff+2])),
				components: readGlyphList(d, ligOff+4),
			})
		}
	}
	// Longest match wins, so the substitution loop can take the first hit.
	for gid, ligs := range out {
		for i := 1; i < len(ligs); i++ {
			for j := i; j > 0 && len(ligs[j].components) > len(ligs[j-1].components); j-- {
				ligs[j], ligs[j-1] = ligs[j-1], ligs[j]
			}
		}
		out[gid] = ligs
	}
}

// readGlyphList reads a glyph array: a count followed by that many glyph ids.
func readGlyphList(d []byte, p int) []int {
	if p+2 > len(d) {
		return nil
	}
	n := int(be16(d[p : p+2]))
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		q := p + 2 + 2*i
		if q+2 > len(d) {
			break
		}
		out = append(out, int(be16(d[q:q+2])))
	}
	return out
}

// coverage reads a Coverage table into glyph -> coverage index.
func coverage(d []byte, p int) map[int]int {
	if p+4 > len(d) {
		return nil
	}
	out := map[int]int{}
	switch be16(d[p : p+2]) {
	case 1:
		n := int(be16(d[p+2 : p+4]))
		for i := 0; i < n; i++ {
			q := p + 4 + 2*i
			if q+2 > len(d) {
				break
			}
			out[int(be16(d[q:q+2]))] = i
		}
	case 2:
		n := int(be16(d[p+2 : p+4]))
		for i := 0; i < n; i++ {
			q := p + 4 + 6*i
			if q+6 > len(d) {
				break
			}
			start := int(be16(d[q : q+2]))
			end := int(be16(d[q+2 : q+4]))
			idx := int(be16(d[q+4 : q+6]))
			for g := start; g <= end && g <= 0xFFFF; g++ {
				out[g] = idx + (g - start)
			}
		}
	}
	return out
}

// classDef reads a ClassDef table into glyph -> class.
func classDef(d []byte, p int) map[int]int {
	if p+4 > len(d) {
		return nil
	}
	out := map[int]int{}
	switch be16(d[p : p+2]) {
	case 1:
		start := int(be16(d[p+2 : p+4]))
		n := int(be16(d[p+4 : p+6]))
		for i := 0; i < n; i++ {
			q := p + 6 + 2*i
			if q+2 > len(d) {
				break
			}
			out[start+i] = int(be16(d[q : q+2]))
		}
	case 2:
		n := int(be16(d[p+2 : p+4]))
		for i := 0; i < n; i++ {
			q := p + 4 + 6*i
			if q+6 > len(d) {
				break
			}
			start := int(be16(d[q : q+2]))
			end := int(be16(d[q+2 : q+4]))
			c := int(be16(d[q+4 : q+6]))
			for g := start; g <= end && g <= 0xFFFF; g++ {
				out[g] = c
			}
		}
	}
	return out
}

// matchLigature returns the longest ligature starting at gids[i], and how many
// glyphs it consumes including its own.
func (s *shaper) matchLigature(gids []int, i int) (ligature, int, bool) {
	ligs, ok := s.ligatures[gids[i]]
	if !ok {
		return ligature{}, 0, false
	}
	for _, lg := range ligs {
		n := 1 + len(lg.components)
		if i+n > len(gids) {
			continue
		}
		fits := true
		for k, comp := range lg.components {
			if gids[i+1+k] != comp {
				fits = false
				break
			}
		}
		if fits {
			return lg, n, true
		}
	}
	return ligature{}, 0, false
}

// advance returns a glyph's advance in font units.
func (s *shaper) advance(gid int) float64 {
	if s == nil {
		return 0
	}
	if w, ok := s.hmtx[gid]; ok {
		return float64(w)
	}
	if s.numHMetrics > 0 {
		if w, ok := s.hmtx[s.numHMetrics-1]; ok {
			return float64(w)
		}
	}
	return 0
}

// Shape lays out s into positioned glyphs at sizePt.
func (s *shaper) Shape(text string, sizePt float64) []glyphRun {
	if s == nil || s.upem == 0 || text == "" {
		return nil
	}
	runes := []rune(text)
	gids := make([]int, len(runes))
	for i, r := range runes {
		if g, ok := s.cmap[r]; ok {
			gids[i] = g
		} else {
			gids[i] = s.fallbackGID
		}
	}
	// Ligature substitution first, longest match at each position, then kerning
	// over what survives. That order is the one a shaper uses, and it matters:
	// kerning is defined between the glyphs that are actually drawn.
	out := make([]glyphRun, 0, len(runes))
	src := make([]string, 0, len(runes))
	i := 0
	for i < len(gids) {
		if lig, n, ok := s.matchLigature(gids, i); ok {
			out = append(out, glyphRun{gid: lig.glyph})
			src = append(src, joinRunes(runes[i:i+n]))
			i += n
			continue
		}
		out = append(out, glyphRun{gid: gids[i]})
		src = append(src, string(runes[i]))
		i++
	}

	pen := 0.0
	for k := range out {
		if k > 0 {
			pen += float64(s.kern[[2]int{out[k-1].gid, out[k].gid}])
		}
		out[k].dx = pen * sizePt / s.upem
		pen += s.advance(out[k].gid)
		out[k].advance = s.advance(out[k].gid) * sizePt / s.upem
		out[k].text = src[k]
	}
	return out
}

// ShapeWidth is the shaped advance of a string, in points.
func (s *shaper) ShapeWidth(text string, sizePt float64) float64 {
	gs := s.Shape(text, sizePt)
	if len(gs) == 0 {
		return 0
	}
	return gs[len(gs)-1].dx + gs[len(gs)-1].advance
}

func joinRunes(rs []rune) string {
	b := make([]byte, 0, len(rs))
	for _, r := range rs {
		b = append(b, []byte(string(r))...)
	}
	return string(b)
}

// advanceTable returns gid -> per-mille advance, for the /W precision repair.
//
// The values are read back out of the same font program gpdf embeds, so the /W
// array a rasteriser measures against is the font's own metric rather than a
// truncated copy of it. See post_widths.go for why that matters.
func (s *shaper) advanceTable() map[int]float64 {
	if s == nil || s.upem == 0 {
		return nil
	}
	out := make(map[int]float64, len(s.hmtx))
	for gid, w := range s.hmtx {
		out[gid] = float64(w) * 1000 / s.upem
	}
	return out
}
