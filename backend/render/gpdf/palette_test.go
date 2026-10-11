package gpdf

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoColourLiteralsInEngine guards the single-source contract: no paint in
// this package may name a colour directly. Every fill, stroke, gradient stop
// and text run reads a role from the engine's resolved theme.Palette, so a
// literal here means a spot a rebrand cannot reach — the exact defect issue
// #335 removed.
//
// Column geometry (colX, colLayout, colOffsetsPx) shares the "col" prefix but
// carries coordinates, not colours; the pattern below only matches hex
// colours, so geometry never trips it. Test files are excluded: the formula
// pins in palette_test.go assert derived values against literals on purpose.
func TestNoColourLiteralsInEngine(t *testing.T) {
	hex := regexp.MustCompile(`#[0-9A-Fa-f]{6}\b`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range hex.FindAll(raw, -1) {
			t.Errorf("%s contains colour literal %s; resolve it through theme.Palette", name, m)
		}
	}
}
