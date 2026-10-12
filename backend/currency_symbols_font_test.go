package backend

import (
	"testing"

	gpdffont "github.com/gpdf-dev/gpdf/pdf/font"

	"vera-app/backend/currency"
)

// TestCatalogueSymbolsAreInTheEmbeddedFaces pins that every rune of every
// catalogue symbol exists in the two Inter faces the documents actually
// embed (medium as the regular, bold as the weight). A symbol addition whose
// glyph is missing would otherwise ship a .notdef box on every total that
// carries it, in both renderers at once.
//
// The faces are parsed with gpdf's own parser — the same code that shapes
// the native renderer — so coverage here means renderable there, not merely
// present under some other parser's reading. The bytes come from backAssets,
// the same embedded filesystem both renderers read, so the test cannot pass
// against a font the binary does not ship.
//
// Only the two loaded faces are guarded: assets/inter-regular.ttf sits on
// disk (and inside the embed glob) but nothing reads it — template.go and
// gpdf.go both load medium and bold — so a gap there ships nothing.
func TestCatalogueSymbolsAreInTheEmbeddedFaces(t *testing.T) {
	faces := map[string]string{
		"regular": "assets/inter-medium.ttf",
		"bold":    "assets/inter-bold.ttf",
	}
	parsed := make(map[string]*gpdffont.TrueTypeFont, len(faces))
	for name, path := range faces {
		raw, err := backAssets.ReadFile(path)
		if err != nil {
			t.Fatalf("load embedded face %s (%s): %v", name, path, err)
		}
		face, err := gpdffont.ParseTrueType(raw)
		if err != nil {
			t.Fatalf("parse embedded face %s (%s): %v", name, path, err)
		}
		parsed[name] = face
	}
	for _, code := range currency.Codes() {
		info, ok := currency.Lookup(code)
		if !ok {
			t.Errorf("code %s is listed but has no catalogue entry", code)
			continue
		}
		for _, r := range info.Symbol {
			for name, face := range parsed {
				if face.GlyphID(r) == 0 {
					t.Errorf("code %s symbol rune %q (U+%04X) has no glyph in the embedded %s face; it would print as .notdef",
						code, r, r, name)
				}
			}
		}
	}
}
