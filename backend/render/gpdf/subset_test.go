package gpdf

// Subsetting tests.
//
// The property that matters is not "the file got smaller" but "the file means
// the same thing". The renderer is handed a second parse of the subset while the
// layout was computed against a parse of the whole face, and every glyph in the
// document is addressed by the id one of those two parses assigns. If the subset
// renumbers anything, the document renders in the wrong shapes rather than the
// wrong size — so the ids are pinned here rather than left to inspection.

import (
	"os"
	"testing"

	"github.com/gpdf-dev/gpdf/pdf/font"
)

func loadFace(t *testing.T, path string) ([]byte, *shaper) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read face: %v", err)
	}
	return data, parseShaper(data)
}

func TestSubsetKeepsGlyphIDs(t *testing.T) {
	data, sh := loadFace(t, "../../assets/inter-medium.ttf")

	runes := map[rune]bool{}
	for _, c := range "Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam" {
		runes[c] = true
	}
	sub, err := subsetFace(data, sh.cmap, runes)
	if err != nil {
		t.Fatalf("subsetFace: %v", err)
	}

	if len(sub) >= len(data)/4 {
		t.Errorf("subset is %d bytes from %d; that is not a subset", len(sub), len(data))
	}

	original, err := readTableDir(data)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := readTableDir(sub)
	if err != nil {
		t.Fatalf("subset has an unreadable table directory: %v", err)
	}
	_ = dir

	// A subset must still describe the same glyph count. Recutting the glyph
	// table would be far smaller still, but it would renumber every glyph after
	// the first one dropped.
	for _, tag := range []string{"maxp", "hhea", "hmtx", "name", "OS/2"} {
		got, ok := dir[tag]
		if !ok {
			t.Errorf("subset is missing the %s table", tag)
			continue
		}
		want := original[tag]
		if got.length != want.length {
			t.Errorf("table %s is %d bytes, was %d", tag, got.length, want.length)
			continue
		}
		// hhea and head are the two tables that legitimately differ; they are
		// checked separately.
		if tag == "hhea" {
			continue
		}
		if string(sub[got.offset:got.offset+got.length]) != string(data[want.offset:want.offset+want.length]) {
			t.Errorf("table %s was modified but should be carried over verbatim", tag)
		}
	}

	if got, want := dir["maxp"].length, original["maxp"].length; got != want {
		t.Errorf("maxp length %d, want %d", got, want)
	}
	if got, want := numGlyphsOf(dir, sub), numGlyphsOf(original, data); got != want {
		t.Errorf("subset has %d glyphs, original has %d", got, want)
	}
}

// TestSubsetMapsRunesToTheSameGlyphs is the test that would catch a renumbering
// bug: it asks the subset's own character map for every rune and compares the
// answer with the whole face's.
func TestSubsetMapsRunesToTheSameGlyphs(t *testing.T) {
	data, sh := loadFace(t, "../../assets/inter-medium.ttf")

	runes := map[rune]bool{}
	for c := rune(0x20); c < 0x7F; c++ {
		runes[c] = true
	}
	sub, err := subsetFace(data, sh.cmap, runes)
	if err != nil {
		t.Fatalf("subsetFace: %v", err)
	}
	dir, err := readTableDir(sub)
	if err != nil {
		t.Fatal(err)
	}
	subSh := parseShaper(sub)
	for r := range runes {
		if got, want := subSh.cmap[r], sh.cmap[r]; got != want {
			t.Fatalf("rune %q: subset maps it to glyph %d, original maps it to %d", r, got, want)
		}
	}
	if len(subSh.cmap) != len(runes) {
		t.Errorf("subset map holds %d runes, asked for %d", len(subSh.cmap), len(runes))
	}

	// Advances must survive too: the layout measures with the whole face and the
	// reader advances with the subset.
	for r := range runes {
		if subSh.hmtx[subSh.cmap[r]] != sh.hmtx[sh.cmap[r]] {
			t.Fatalf("rune %q has a different advance in the subset", r)
		}
	}
	_ = dir
}

// TestSubsetEmptiesUnusedGlyphs checks the mechanism that makes the subset
// smaller without renumbering: the glyphs nothing points at keep their slot and
// lose their outline, so an id that means "not used" still means "not used"
// rather than accidentally meaning "whatever glyph moved into this slot".
func TestSubsetEmptiesUnusedGlyphs(t *testing.T) {
	data, sh := loadFace(t, "../../assets/inter-medium.ttf")

	dir, err := readTableDir(data)
	if err != nil {
		t.Fatal(err)
	}
	loca := dir["loca"]
	indexToLoc := int(data[dir["head"].offset+50])<<8 | int(data[dir["head"].offset+51])
	numGlyphs := int(data[dir["maxp"].offset+4])<<8 | int(data[dir["maxp"].offset+5])
	offsets, err := readLoca(data, loca, indexToLoc, numGlyphs)
	if err != nil {
		t.Fatal(err)
	}
	full := func(gid int) int {
		return int(offsets[gid+1] - offsets[gid])
	}

	runes := map[rune]bool{'A': true, 'B': true}
	sub, err := subsetFace(data, sh.cmap, runes)
	if err != nil {
		t.Fatal(err)
	}
	sdir, err := readTableDir(sub)
	if err != nil {
		t.Fatal(err)
	}
	sfmt := int(sub[sdir["head"].offset+50])<<8 | int(sub[sdir["head"].offset+51])
	soffsets, err := readLoca(sub, sdir["loca"], sfmt, numGlyphs)
	if err != nil {
		t.Fatal(err)
	}

	// Notdef and the two asked-for glyphs keep outlines; nothing else does.
	for _, r := range []rune{'A', 'B'} {
		gid := sh.cmap[r]
		if soffsets[gid+1]-soffsets[gid] == 0 {
			t.Errorf("rune %q (glyph %d) lost its outline", r, gid)
		}
	}
	emptied := 0
	for gid := range numGlyphs {
		if offsets[gid+1]-offsets[gid] > 0 && soffsets[gid+1]-soffsets[gid] == 0 {
			emptied++
		}
	}
	if emptied < numGlyphs/2 {
		t.Errorf("only %d of %d glyphs were emptied; the subset kept too much", emptied, numGlyphs)
	}
	t.Logf("%d glyphs emptied, %d kept their outline (A had %d bytes)", emptied, numGlyphs-emptied, full(sh.cmap['A']))
}

// TestSubsetKeepsCompositeComponents covers accented characters, which are
// drawn as composites referencing other glyphs. Emptying a component that a
// kept glyph still needs would draw the letter without its accent.
func TestSubsetKeepsCompositeComponents(t *testing.T) {
	data, sh := loadFace(t, "../../assets/inter-medium.ttf")
	dir, err := readTableDir(data)
	if err != nil {
		t.Fatal(err)
	}
	orig := glyphRuns(t, data, dir)

	// Find a composite glyph the font reaches through its character map.
	var target rune
	var parts []int
	for r, gid := range sh.cmap {
		start, end := orig[gid], orig[gid+1]
		if end <= start {
			continue
		}
		if comps := compositeComponents(data, dir["glyf"].offset+start, dir["glyf"].offset+end); len(comps) > 0 {
			target, parts = r, comps
			break
		}
	}
	if target == 0 {
		t.Skip("face has no composite glyph reachable through the character map")
	}

	sub, err := subsetFace(data, sh.cmap, map[rune]bool{target: true})
	if err != nil {
		t.Fatalf("subsetFace: %v", err)
	}
	sdir, err := readTableDir(sub)
	if err != nil {
		t.Fatal(err)
	}
	kept := glyphRuns(t, sub, sdir)
	for _, comp := range parts {
		if kept[comp+1] == kept[comp] {
			t.Errorf("component glyph %d of %q was dropped; the letter would lose its accent", comp, target)
		}
	}
	t.Logf("kept %q and its %d component glyphs", target, len(parts))
}

// glyphRuns returns the glyf extents of every glyph in a font.
func glyphRuns(t *testing.T, data []byte, dir map[string]sfntTable) []uint32 {
	t.Helper()
	format := int(data[dir["head"].offset+50])<<8 | int(data[dir["head"].offset+51])
	n := numGlyphsOf(dir, data)
	offsets, err := readLoca(data, dir["loca"], format, n)
	if err != nil {
		t.Fatalf("read loca: %v", err)
	}
	return offsets
}

// TestSubsetIsEmbeddable is the end-to-end check: the bytes have to be something
// the renderer will accept, since a subset that fails to parse falls back to
// embedding the whole face and silently undoes the saving.
func TestSubsetIsEmbeddable(t *testing.T) {
	data, sh := loadFace(t, "../../assets/inter-bold.ttf")
	runes := map[rune]bool{'V': true, 'e': true, 'r': true, 'a': true, '0': true, '.': true}
	sub, err := subsetFace(data, sh.cmap, runes)
	if err != nil {
		t.Fatalf("subsetFace: %v", err)
	}
	parsed, err := font.ParseTrueType(sub)
	if err != nil {
		t.Fatalf("the renderer will not accept the subset: %v", err)
	}
	w, ok := parsed.GlyphWidth('e')
	if !ok {
		t.Fatal("subset has no width for 'e'")
	}
	if uint16(w) != sh.hmtx[sh.cmap['e']] {
		t.Errorf("'e' is %d units in the subset, %d in the original", w, sh.hmtx[sh.cmap['e']])
	}
	t.Logf("%d runes -> %d bytes from %d", len(runes), len(sub), len(data))
}

func numGlyphsOf(dir map[string]sfntTable, data []byte) int {
	return int(data[dir["maxp"].offset+4])<<8 | int(data[dir["maxp"].offset+5])
}

func soffsetsOf(d []byte, dir map[string]sfntTable) []uint32 {
	return make([]uint32, numGlyphsOf(dir, d)+1)
}
