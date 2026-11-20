package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sharedSnapshotFixture is the one report JSON both surfaces read. The report
// is the only source of the figures; the expected block is the hand-checked
// formatting contract, so a money, percentage or date rendering change cannot
// land on one surface only.
type sharedSnapshotFixture struct {
	Description string `json:"description"`
	Expected    struct {
		Metrics []string `json:"metrics"`
		PDF     struct {
			FilterRange string `json:"filterRange"`
			GeneratedAt string `json:"generatedAt"`
			AsOf        string `json:"asOf"`
		} `json:"pdf"`
		Screen struct {
			AsOfDate    string `json:"asOfDate"`
			FilterRange string `json:"filterRange"`
		} `json:"screen"`
	} `json:"expected"`
	Report json.RawMessage `json:"report"`
}

func loadSharedSnapshotFixture(t *testing.T) sharedSnapshotFixture {
	t.Helper()
	path := filepath.Join("testdata", "report_snapshot.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture sharedSnapshotFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if fixture.Description == "" || len(fixture.Expected.Metrics) == 0 || len(fixture.Report) == 0 {
		t.Fatalf("%s is missing its description, expected metrics or report", path)
	}
	return fixture
}

// TestSharedSnapshotIsTheSampleFixture pins one canonical fixture: the shared
// file is the sample report, not a second financial example that could drift
// away from the guards on the programmatic fixture. Regenerate it with:
//
//	UPDATE_REPORT_SNAPSHOT=1 go test ./backend/render/report -run TestSharedSnapshotIsTheSampleFixture
func TestSharedSnapshotIsTheSampleFixture(t *testing.T) {
	fixture := loadSharedSnapshotFixture(t)
	snapshot, err := DecodeSnapshot(fixture.Report)
	if err != nil {
		t.Fatalf("decode shared snapshot: %v", err)
	}
	if os.Getenv("UPDATE_REPORT_SNAPSHOT") != "" {
		// Regenerate from sampleReport, the single source of the fixture. The
		// decoded copy is deliberately not the source: it cannot add a field
		// the committed file predates, which is exactly what a new projection
		// shape needs.
		raw, err := json.MarshalIndent(sampleReport(), "", "  ")
		if err != nil {
			t.Fatalf("marshal shared snapshot: %v", err)
		}
		updated := sharedSnapshotFixture{
			Description: fixture.Description,
			Expected:    fixture.Expected,
			Report:      raw,
		}
		encoded, err := json.MarshalIndent(updated, "", "  ")
		if err != nil {
			t.Fatalf("encode shared snapshot: %v", err)
		}
		path := filepath.Join("testdata", "report_snapshot.json")
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		fixture = loadSharedSnapshotFixture(t)
		snapshot, err = DecodeSnapshot(fixture.Report)
		if err != nil {
			t.Fatalf("decode regenerated shared snapshot: %v", err)
		}
	}
	if want := sampleReport(); !reflect.DeepEqual(snapshot, want) {
		t.Fatalf("shared snapshot differs from sampleReport(); regenerate it with UPDATE_REPORT_SNAPSHOT=1")
	}
}

// TestSharedSnapshotPDFPrintsTheSameMetricsAsTheScreen is the PDF half of the
// cross-surface contract. It renders the committed JSON through the real
// export path and extracts the text from the finished PDF, then requires every
// metric the TypeScript screen test requires. The negative money, the real
// zero and the null percentage therefore cannot render differently.
func TestSharedSnapshotPDFPrintsTheSameMetricsAsTheScreen(t *testing.T) {
	fixture := loadSharedSnapshotFixture(t)
	snapshot, err := DecodeSnapshot(fixture.Report)
	if err != nil {
		t.Fatalf("decode shared snapshot: %v", err)
	}

	pdf, err := Render(snapshot, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	pages := binaryPDFText(t, pdf)
	text := strings.Join(pages, "\n")
	requireText(t, text,
		fixture.Expected.PDF.FilterRange,
		fixture.Expected.PDF.GeneratedAt,
		fixture.Expected.PDF.AsOf,
	)
	requireText(t, text, fixture.Expected.Metrics...)
	if strings.Contains(text, "11.11%") {
		t.Errorf("the shared PDF printed the costed-denominator percentage 11.11%%")
	}

	// The in-memory page text is the same layout before PDF encoding. It is the
	// stricter read for the unknown-percentage placeholder, which some external
	// readers transliterate.
	renderer, err := testReportRenderer(snapshot)
	if err != nil {
		t.Fatalf("layout shared snapshot: %v", err)
	}
	requireText(t, pageText(renderer.canvas.pages), fixture.Expected.Metrics...)
}
