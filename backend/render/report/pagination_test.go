package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vera-app/backend/poreport"
)

// paginationManifest is a deterministic, human-readable record of what each
// page contains. It is text, not a PDF or a screenshot: the golden is reviewed
// in a diff, and the renderer's layout order is the only input.
func paginationManifest(pages []*page) string {
	var out strings.Builder
	for index, pg := range pages {
		fmt.Fprintf(&out, "=== page %d ===\n", index+1)
		for _, prim := range pg.prims {
			if value, ok := prim.(textPrim); ok {
				out.WriteString(value.text)
				out.WriteString("\n")
			}
		}
	}
	return out.String()
}

// TestLongReportPaginationGolden pins the 120-row report's page breaks and
// repeated headers. A rule change that silently drops a period, moves the
// table to a different page or stops repeating the header fails this diff.
// Regenerate intentionally with:
//
//	UPDATE_REPORT_GOLDEN=1 go test ./backend/render/report -run TestLongReportPaginationGolden
func TestLongReportPaginationGolden(t *testing.T) {
	renderer, err := testReportRenderer(longReport(poreport.MaxPeriods))
	if err != nil {
		t.Fatalf("layout 120-period report: %v", err)
	}
	assertPaginationGolden(t, "long_report_pagination.golden", renderer.canvas.pages)
}

// TestDayReportPaginationGolden pins the 31-row daily table's breaks and
// repeated header. A day report is the densest table the report can print, so
// the golden is the page-level proof that no day is dropped and the header
// returns on the continuation page.
func TestDayReportPaginationGolden(t *testing.T) {
	renderer, err := testReportRenderer(dayReport())
	if err != nil {
		t.Fatalf("layout day report: %v", err)
	}
	assertPaginationGolden(t, "day_report_pagination.golden", renderer.canvas.pages)
}

func assertPaginationGolden(t *testing.T, name string, pages []*page) {
	t.Helper()
	got := paginationManifest(pages)
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_REPORT_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got == string(want) {
		return
	}
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(string(want), "\n")
	for index := 0; index < len(gotLines) && index < len(wantLines); index++ {
		if gotLines[index] != wantLines[index] {
			t.Fatalf("pagination golden differs at line %d:\n got %q\nwant %q",
				index+1, gotLines[index], wantLines[index])
		}
	}
	t.Fatalf("pagination golden differs in length: got %d lines, want %d",
		len(gotLines), len(wantLines))
}
