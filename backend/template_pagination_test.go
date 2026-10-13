package backend

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestItemRowsAreAtomicInTheStylesheet guards the pagination fix for #391.
//
// The native renderer keeps an item row whole across a page break; Chrome
// fragments it instead (measured on the equivalence fixture, Chrome
// 155.0.8059.39, 2026-10-13: item 5's 120pt row straddled the page 1/2
// boundary, its last two bullets landing on page 2), which is why the oracle's
// pagination test failed. The stylesheet declares break-inside: avoid on
// .items-body-table tr so the browser refuses the split and both renderers
// begin every page at the same item.
//
// The oracle test only runs with a browser, so this rule-guard exists to
// fail in a plain `go test`: removing the declaration silently restores the
// divergence and no always-on test would notice.
func TestItemRowsAreAtomicInTheStylesheet(t *testing.T) {
	raw, err := os.ReadFile("template/css/template-fixed.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}

	re := regexp.MustCompile(`(?s)\.items-body-table\s+tr\s*\{([^}]*)\}`)
	m := re.FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("no `.items-body-table tr { ... }` rule in the stylesheet; " +
			"the row-atomicity declaration is gone")
	}
	for _, decl := range []string{"break-inside: avoid", "page-break-inside: avoid"} {
		if !strings.Contains(m[1], decl) {
			t.Errorf(".items-body-table tr rule missing %q; "+
				"Chrome will split item rows across page breaks again", decl)
		}
	}
}
