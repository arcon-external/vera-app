package gpdf

// TrueType subsetting.
//
// gpdf embeds a font by handing it the original file bytes and blanking the
// outlines of glyphs the document never used. That keeps the reader's idea of
// the font intact but leaves the file at its full size — every glyph's slot, the
// whole of GPOS, GSUB, the long-form post table, all of it. For Inter that is
// 343KB per face for a document that uses about ninety glyphs, and it is most of
// the exported PDF.
//
// This file writes a real subset instead: only the used glyph outlines are
// carried over, the layout tables are dropped, the glyph name table is replaced
// with the short "no names" form, and the character map is rebuilt from the
// runes actually present.
//
// Glyph ids are deliberately NOT renumbered. The engine lays text out against a
// full parse of the face, and the renderer is handed a second parse of whatever
// bytes are registered as the embeddable font. If the two disagree about which
// glyph a rune is, every glyph in the document is drawn as the wrong shape —
// which is a far worse failure than a large file. So unused glyphs keep their
// slots and become empty outlines, and everything that indexes by glyph id —
// hmtx, loca, maxp.numGlyphs — is left the length it was.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/gpdf-dev/gpdf/pdf/font"
)

// sfntTable is one entry of the font's table directory.
type sfntTable struct {
	tag    string
	offset uint32
	length uint32
}

// embeddable reduces a face to the runes the document draws and parses the
// result, so the renderer embeds a subset instead of the whole file.
//
// A face that cannot be subset — a CFF outline font, a rune outside the basic
// multilingual plane, a malformed table directory — falls back to being parsed
// whole. A large file is a far better outcome than a missing document.
func embeddable(data []byte, sh *shaper, runes map[rune]bool) (*font.TrueTypeFont, error) {
	full, err := font.ParseTrueType(data)
	if err != nil {
		return nil, err
	}
	if len(runes) == 0 {
		return full, nil
	}
	sub, err := subsetFace(data, sh.cmap, runes)
	if err != nil {
		return full, nil
	}
	parsed, err := font.ParseTrueType(sub)
	if err != nil {
		return full, nil
	}
	return parsed, nil
}

// glyphSets is the set of runes each face needs, gathered from the display list.
type glyphSets struct {
	regular map[rune]bool
	bold    map[rune]bool
}

func (g glyphSets) forBold(bold bool) map[rune]bool {
	if bold {
		return g.bold
	}
	return g.regular
}

// collectRunes walks a laid-out document and records, per face, every rune the
// document draws.
//
// This has to happen before rendering, not after: the font handed to the writer
// is chosen at registration time, and the set of glyphs to keep follows from the
// layout, which is the earlier of the two.
func collectRunes(pages []*page) glyphSets {
	gs := glyphSets{regular: map[rune]bool{}, bold: map[rune]bool{}}
	for _, p := range pages {
		for _, prim := range p.prims {
			r, ok := prim.(run)
			if !ok {
				continue
			}
			set := gs.forBold(r.bold)
			for _, c := range r.text {
				set[c] = true
			}
		}
	}
	return gs
}

// subsetFace reduces one face to the runes the document uses.
//
// cmap is the face's rune-to-glyph map, taken from the full parse, because it is
// what says which glyph each rune is. The subset carries its own map, but it is
// derived from this one, so the two parses always agree.
func subsetFace(data []byte, cmap map[rune]int, runes map[rune]bool) ([]byte, error) {
	tabs, err := readTableDir(data)
	if err != nil {
		return nil, err
	}
	head, ok := tabs["head"]
	if !ok {
		return nil, errors.New("subset: font has no head table")
	}
	maxp, ok := tabs["maxp"]
	if !ok {
		return nil, errors.New("subset: font has no maxp table")
	}
	glyf, ok := tabs["glyf"]
	if !ok {
		return nil, errors.New("subset: font has no glyf table")
	}
	loca, ok := tabs["loca"]
	if !ok {
		return nil, errors.New("subset: font has no loca table")
	}

	numGlyphs := int(binary.BigEndian.Uint16(data[maxp.offset+4 : maxp.offset+6]))
	indexToLoc := binary.BigEndian.Uint16(data[head.offset+50 : head.offset+52])

	locaOffsets, err := readLoca(data, loca, int(indexToLoc), numGlyphs)
	if err != nil {
		return nil, err
	}

	// Start from .notdef and add every rune the document draws.
	keep := make([]bool, numGlyphs)
	keep[0] = true
	for r := range runes {
		gid, ok := cmap[r]
		if !ok || gid < 0 || gid >= numGlyphs {
			// A rune with no glyph is not an error: the shaper substitutes
			// .notdef for it, which is already kept.
			continue
		}
		keep[gid] = true
	}

	// A kept composite glyph is useless without the components it draws, and
	// the components' own components too, so walk the closure.
	pending := make([]int, 0, len(runes)+1)
	for gid, k := range keep {
		if k {
			pending = append(pending, gid)
		}
	}
	for len(pending) > 0 {
		gid := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		start, end := locaOffsets[gid], locaOffsets[gid+1]
		if end <= start || end > uint32(len(data)) {
			continue
		}
		for _, comp := range compositeComponents(data, glyf.offset+start, glyf.offset+end) {
			if comp < 0 || comp >= numGlyphs || keep[comp] {
				continue
			}
			keep[comp] = true
			pending = append(pending, comp)
		}
	}

	newGlyf, newLoca, format := compactGlyf(data, glyf.offset, locaOffsets, keep)

	// Rebuild the character map from the runes kept, so the subset does not
	// carry Inter's 25KB of mappings for scripts this document never sets.
	keptRunes := make(map[rune]bool, len(runes))
	for r := range runes {
		keptRunes[r] = true
	}
	newCmap, err := buildCmap(cmap, keptRunes, keep)
	if err != nil {
		return nil, err
	}

	var tables []sfntOut
	keep8 := func(tag string) {
		if t, ok := tabs[tag]; ok {
			tables = append(tables, sfntOut{tag: tag, data: data[t.offset : t.offset+t.length]})
		}
	}
	tables = append(tables,
		sfntOut{tag: "cmap", data: newCmap},
		sfntOut{tag: "glyf", data: newGlyf},
		sfntOut{tag: "head", data: headTable(data, head, format)},
		sfntOut{tag: "loca", data: newLoca},
		sfntOut{tag: "maxp", data: data[maxp.offset : maxp.offset+maxp.length]},
		// post is replaced rather than dropped: some readers expect to find it,
		// and version 3.0 is the sanctioned way to say "this font has no glyph
		// names". Keeping Inter's version 2.0 would carry 32KB for nothing.
		sfntOut{tag: "post", data: postV3()},
	)
	keep8("hhea")
	keep8("hmtx")
	keep8("name")
	keep8("OS/2")

	out, err := encodeSfnt(tables)
	if err != nil {
		return nil, err
	}
	if err := applyCheckSumAdjustment(out); err != nil {
		return nil, err
	}
	return out, nil
}

// compositeComponents returns the glyph ids a composite glyph draws.
//
// A composite glyph is a list of components, each with an index and a transform.
// The transform's length depends on two flag bits, so the walk has to read the
// flags rather than assume a stride.
func compositeComponents(data []byte, start, end uint32) []int {
	if end-start < 10 || end > uint32(len(data)) {
		return nil
	}
	base := int(start)
	if int16(binary.BigEndian.Uint16(data[base:base+2])) >= 0 {
		return nil // a simple glyph has contours, not components
	}
	var out []int
	// Components follow the same ten-byte glyph header a simple glyph has: the
	// contour count and then the bounding box. The contour count is negative to
	// mark the glyph as composite, and the bounding box is still there, so
	// skipping only the count lands in the middle of the box and reads the
	// components' own flags as glyph ids.
	p := base + 10
	for p+4 <= int(end) {
		flags := binary.BigEndian.Uint16(data[p : p+2])
		out = append(out, int(binary.BigEndian.Uint16(data[p+2:p+4])))
		p += 4
		if flags&0x0001 != 0 { // ARG_1_AND_2_ARE_WORDS
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&0x0008 != 0: // WE_HAVE_A_SCALE
			p += 2
		case flags&0x0040 != 0: // X_AND_Y_SCALE
			p += 4
		case flags&0x0080 != 0: // TWO_BY_TWO
			p += 8
		}
		if flags&0x0020 == 0 { // MORE_COMPONENTS
			break
		}
	}
	return out
}

// compactGlyf lays the kept outlines end to end and rebuilds the offsets.
//
// Unused glyphs get a zero-length entry pointing at the next kept outline, so
// their slot still exists at its original index but draws nothing. That is what
// keeps a glyph id meaning the same thing in the subset as in the original.
func compactGlyf(data []byte, glyfBase uint32, offsets []uint32, keep []bool) ([]byte, []byte, int) {
	var body []byte
	newOff := make([]uint32, len(offsets))
	for gid := range offsets {
		newOff[gid] = uint32(len(body))
		if gid == len(offsets)-1 {
			break
		}
		if !keep[gid] {
			continue
		}
		start, end := offsets[gid], offsets[gid+1]
		if end <= start || glyfBase+end > uint32(len(data)) {
			continue
		}
		body = append(body, data[glyfBase+start:glyfBase+end]...)
		if len(body)%2 == 1 {
			body = append(body, 0) // glyph data is read in words
		}
	}
	// An unused glyph must not inherit the offset of a kept one that follows it,
	// or it would draw that glyph. Point every skipped slot at its own start.
	for gid := len(newOff) - 2; gid >= 0; gid-- {
		if !keep[gid] {
			newOff[gid] = newOff[gid+1]
		}
	}

	// Short offsets halve the table, and this document's outlines are small
	// enough for them to fit.
	format := 1
	loca := make([]byte, 0, 4*len(newOff))
	maxOff := uint32(0)
	for _, o := range newOff {
		if o > maxOff {
			maxOff = o
		}
	}
	if maxOff%2 == 0 && maxOff/2 <= 0xFFFF {
		format = 0
		loca = make([]byte, 0, 2*len(newOff))
		for _, o := range newOff {
			loca = appendU16(loca, uint16(o/2))
		}
	} else {
		for _, o := range newOff {
			loca = appendU32(loca, o)
		}
	}
	return body, loca, format
}

// buildCmap writes a format 4 character map covering only the runes kept.
//
// Segments are run-length coded by requiring both the code points and the glyph
// ids to step by one, which is what lets idRangeOffset stay zero everywhere.
func buildCmap(cmap map[rune]int, runes map[rune]bool, keep []bool) ([]byte, error) {
	type pair struct{ cp, gid uint16 }
	var segs []pair
	for r := range runes {
		if r >= 0x10000 {
			return nil, fmt.Errorf("subset: rune %U is outside the basic multilingual plane", r)
		}
		gid, ok := cmap[r]
		if !ok || gid < 0 || gid >= len(keep) || !keep[gid] {
			continue
		}
		segs = append(segs, pair{cp: uint16(r), gid: uint16(gid)})
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].cp < segs[j].cp })

	type seg struct{ first, last, delta uint16 }
	var runs []seg
	for _, p := range segs {
		if len(runs) > 0 {
			last := &runs[len(runs)-1]
			// Extending is only valid while both code point and glyph step
			// together; otherwise the delta would change mid-segment.
			if uint32(p.cp) == uint32(last.last)+1 && uint32(p.gid) == uint32(last.delta)+uint32(last.last)+1 {
				last.last = p.cp
				continue
			}
		}
		runs = append(runs, seg{first: p.cp, last: p.cp, delta: p.gid - p.cp})
	}
	// The format requires a final segment mapping 0xFFFF to glyph 0, which also
	// terminates every real segment.
	runs = append(runs, seg{first: 0xFFFF, last: 0xFFFF, delta: 1})

	segCount := len(runs)
	sub := make([]byte, 0, 16+8*segCount)
	sub = appendU16(sub, 4)                                        // format
	sub = appendU16(sub, 0)                                        // length, patched below
	sub = appendU16(sub, 0)                                        // language
	sub = appendU16(sub, uint16(segCount*2))                       // segCountX2
	sub = appendU16(sub, uint16(2*pow2Floor(segCount)))            // searchRange
	sub = appendU16(sub, uint16(pow2Floor(segCount)))              // entrySelector
	sub = appendU16(sub, uint16(segCount*2-2*pow2Floor(segCount))) // rangeShift
	for _, s := range runs {
		sub = appendU16(sub, s.last)
	}
	sub = appendU16(sub, 0) // reservedPad
	for _, s := range runs {
		sub = appendU16(sub, s.first)
	}
	for _, s := range runs {
		sub = appendU16(sub, s.delta)
	}
	for range runs {
		sub = appendU16(sub, 0) // idRangeOffset
	}
	binary.BigEndian.PutUint16(sub[2:], uint16(len(sub)))

	// A cmap has its own small directory of encoding records — it is not an sfnt and
	// carries no version magic — so it is assembled here rather than through
	// encodeSfnt. One record is enough: Windows, Unicode BMP, pointing at the
	// format 4 subtable.
	out := make([]byte, 0, 12+len(sub))
	out = appendU16(out, 0)  // version
	out = appendU16(out, 1)  // numTables
	out = appendU16(out, 3)  // platform: Windows
	out = appendU16(out, 1)  // encoding: Unicode BMP
	out = appendU32(out, 12) // offset of the subtable, from the start of the cmap
	return append(out, sub...), nil
}

// headTable copies head, records which offset format loca now uses, and clears
// checkSumAdjustment.
//
// Clearing it here rather than leaving it to the last step matters: the value in
// the table is part of the table, so the table's own directory checksum is
// computed over it. applyCheckSumAdjustment puts the right value back after
// every checksum is settled, which is the only order in which the two agree.
func headTable(data []byte, head sfntTable, format int) []byte {
	out := make([]byte, head.length)
	copy(out, data[head.offset:head.offset+head.length])
	binary.BigEndian.PutUint16(out[50:], uint16(format))
	binary.BigEndian.PutUint32(out[8:12], 0)
	return out
}

// postV3 is the "no glyph names" post table.
func postV3() []byte {
	out := make([]byte, 32)
	binary.BigEndian.PutUint32(out[0:], 0x00030000)
	binary.BigEndian.PutUint32(out[4:], 0)  // italicAngle
	binary.BigEndian.PutUint16(out[8:], 0)  // underlinePosition
	binary.BigEndian.PutUint16(out[10:], 0) // underlineThickness
	binary.BigEndian.PutUint32(out[12:], 1)
	binary.BigEndian.PutUint32(out[16:], 0)
	binary.BigEndian.PutUint32(out[20:], 0)
	binary.BigEndian.PutUint32(out[24:], 0)
	binary.BigEndian.PutUint32(out[28:], 0)
	return out
}

// sfntOut is one table queued for a new font file.
type sfntOut struct {
	tag  string
	data []byte
}

// encodeSfnt assembles tables into a font file: a directory sorted by tag, every
// table four-byte aligned, with each table's checksum recorded.
func encodeSfnt(tables []sfntOut) ([]byte, error) {
	tables = append([]sfntOut(nil), tables...)
	sort.Slice(tables, func(i, j int) bool { return tables[i].tag < tables[j].tag })

	n := len(tables)
	if n == 0 {
		return nil, errors.New("subset: no tables to write")
	}
	log2n := pow2Floor(n)
	dirLen := 16 * pow2Floor(n)

	d := make([]byte, 0, 12+16*n)
	d = appendU32(d, 0x00010000)          // sfnt version
	d = appendU16(d, uint16(n))           // numTables
	d = appendU16(d, uint16(dirLen))      // searchRange
	d = appendU16(d, uint16(log2n))       // entrySelector
	d = appendU16(d, uint16(16*n-dirLen)) // rangeShift

	type entry struct {
		tag            string
		checksum       uint32
		offset, length uint32
	}
	entries := make([]entry, 0, n)
	var body []byte
	// Offsets count from the start of the file, so the body begins after both
	// the twelve-byte header and the directory records — which have not been
	// written yet, but whose length is known.
	base := len(d) + 16*n
	for _, t := range tables {
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		e := entry{tag: t.tag, offset: uint32(base + len(body)), length: uint32(len(t.data))}
		e.checksum = tableChecksum(t.data)
		entries = append(entries, e)
		body = append(body, t.data...)
	}
	for len(body)%4 != 0 {
		body = append(body, 0)
	}
	for _, e := range entries {
		d = append(d, e.tag...)
		d = appendU32(d, e.checksum)
		d = appendU32(d, e.offset)
		d = appendU32(d, e.length)
	}

	out := append(d, body...)
	return out, nil
}

// applyCheckSumAdjustment fills in head's checkSumAdjustment.
//
// The field is defined as the difference between a magic constant and the
// checksum of the whole file read with that field itself taken as zero. It is
// not decoration: font tooling that validates it rejects a font that does not
// satisfy it, and a rejected font is one some readers will not draw at all.
//
// This is separate from encodeSfnt because a cmap is itself a small sfnt and
// has no head table to adjust.
func applyCheckSumAdjustment(out []byte) error {
	n := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < n; i++ {
		o := 12 + 16*i
		if o+16 > len(out) || string(out[o:o+4]) != "head" {
			continue
		}
		// The field lives at offset 8 *within the table*, so it has to be
		// reached through the table's own offset rather than the directory
		// record, whose layout happens to put a uint32 in the same place.
		at := int(binary.BigEndian.Uint32(out[o+8:o+12])) + 8
		if at+4 > len(out) {
			return errors.New("subset: head table offset is out of range")
		}
		binary.BigEndian.PutUint32(out[at:at+4], 0)
		binary.BigEndian.PutUint32(out[at:at+4], 0xB1B0AFBA-tableChecksum(out))
		return nil
	}
	return errors.New("subset: built font without a head table")
}

func pow2Floor(n int) int {
	p := 1
	for p*2 <= n {
		p *= 2
	}
	return p
}

// --- small sfnt readers -----------------------------------------------------

func readTableDir(data []byte) (map[string]sfntTable, error) {
	if len(data) < 12 {
		return nil, errors.New("subset: font too short")
	}
	n := int(binary.BigEndian.Uint16(data[4:6]))
	out := make(map[string]sfntTable, n)
	for i := 0; i < n; i++ {
		o := 12 + 16*i
		if o+16 > len(data) {
			return nil, errors.New("subset: truncated table directory")
		}
		t := sfntTable{
			tag:    string(data[o : o+4]),
			offset: binary.BigEndian.Uint32(data[o+8 : o+12]),
			length: binary.BigEndian.Uint32(data[o+12 : o+16]),
		}
		if uint64(t.offset)+uint64(t.length) > uint64(len(data)) {
			return nil, fmt.Errorf("subset: table %q runs past the end of the font", t.tag)
		}
		out[t.tag] = t
	}
	return out, nil
}

func readLoca(data []byte, loca sfntTable, format, numGlyphs int) ([]uint32, error) {
	need := numGlyphs + 1
	stride := 2
	if format == 1 {
		stride = 4
	}
	if int(loca.length) < need*stride {
		return nil, errors.New("subset: loca table is too short for maxp.numGlyphs")
	}
	out := make([]uint32, need)
	base := int(loca.offset)
	for i := 0; i < need; i++ {
		if stride == 2 {
			out[i] = uint32(binary.BigEndian.Uint16(data[base+2*i:base+2*i+2])) * 2
		} else {
			out[i] = binary.BigEndian.Uint32(data[base+4*i : base+4*i+4])
		}
	}
	return out, nil
}

func tableChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i+4 <= len(data); i += 4 {
		sum += binary.BigEndian.Uint32(data[i : i+4])
	}
	if r := len(data) % 4; r != 0 {
		var tail [4]byte
		copy(tail[:], data[len(data)-r:])
		sum += binary.BigEndian.Uint32(tail[:])
	}
	return sum
}

// --- byte helpers -----------------------------------------------------------

func appendU16(b []byte, v uint16) []byte { return append(b, byte(v>>8), byte(v)) }

func appendU32(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
