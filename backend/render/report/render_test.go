package report

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vera-app/backend/poreport"
)

func TestRenderSampleReportIsAPDF(t *testing.T) {
	snapshot := sampleReport()
	renderer, err := testReportRenderer(snapshot)
	if err != nil {
		t.Fatalf("layout report: %v", err)
	}
	pdf, err := renderer.canvas.render("Purchase Order Performance Report")
	if err != nil {
		t.Fatalf("render report: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("render did not produce a PDF (%d bytes)", len(pdf))
	}
	if len(pdf) < 10_000 {
		t.Errorf("sample PDF is only %d bytes; the document is probably empty", len(pdf))
	}
	if len(renderer.canvas.pages) < 3 {
		t.Errorf("sample report has %d pages; expected header, charts, tables and definitions to span several",
			len(renderer.canvas.pages))
	}
}

func TestRenderPublicAPIProducesAPDF(t *testing.T) {
	pdf, err := Render(sampleReport(), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("Render did not produce a PDF (%d bytes)", len(pdf))
	}
}

func TestRender120PeriodsPaginatesWithoutTruncatingTheTable(t *testing.T) {
	snapshot := longReport(120)
	renderer, err := testReportRenderer(snapshot)
	if err != nil {
		t.Fatalf("layout report: %v", err)
	}
	text := pageText(renderer.canvas.pages)
	if !strings.Contains(text, "2016") || !strings.Contains(text, "2025") {
		t.Errorf("period table does not span the requested history")
	}
	headers := strings.Count(text, "PERIOD")
	if headers < 3 {
		t.Errorf("period table header appears %d times; a 120-row table must repeat it across pages", headers)
	}
	if !strings.Contains(text, "Page 1 of") {
		t.Errorf("footer page number missing")
	}
	// Every period must survive: the chart may chunk, the table may paginate,
	// but no bucket is dropped.
	for index := 0; index < 120; index++ {
		date := time.Date(2016, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, index, 0)
		label := date.Format("Jan 2006")
		if !strings.Contains(text, label) {
			t.Fatalf("period %s is missing from the rendered report", label)
		}
	}
	// Seven chart panels of at most 18 buckets each, and every panel names its
	// place in the sequence.
	for index := 1; index <= 7; index++ {
		want := fmt.Sprintf("Actual periods  ·  %d of 7", index)
		if !strings.Contains(text, want) {
			t.Errorf("chart panel %q is missing", want)
		}
	}
}

func TestBinaryPDFTextMatchesTheLayout(t *testing.T) {
	snapshot := sampleReport()
	pdf, err := Render(snapshot, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	pages := binaryPDFText(t, pdf)
	if len(pages) < 3 {
		t.Errorf("extracted %d pages, want at least 3", len(pages))
	}
	text := strings.Join(pages, "\n")
	requireText(t, text,
		"Purchase Order Performance Report",
		"Internal report",
		"PPN is excluded",
		"Rp1.234.567.890",
		"Rp910.000.000",
		"Rp810.000.000",
		"Rp100.000.000",
		"12.35%",
		"73.71%",
		"Facility",
		"Management",
		"(FM)",
		"Unassigned",
		"(UNASSIGNED)",
		"Rp-30.000.000",
		"—",
		"Definitions",
	)
}

func TestBinaryPDFTextKeepsTheLongTableComplete(t *testing.T) {
	pdf, err := Render(longReport(120), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render 120 periods: %v", err)
	}
	pages := binaryPDFText(t, pdf)
	text := strings.Join(pages, "\n")
	requireText(t, text, "Jan 2016", "Dec 2025", "Page 1 of", "Page "+fmt.Sprint(len(pages))+" of "+fmt.Sprint(len(pages)))
	headers := strings.Count(text, "PERIOD")
	if headers < 3 {
		t.Errorf("extracted text repeats the period header %d times", headers)
	}
	if len(pages) < 6 {
		t.Errorf("120-period report is %d pages; expected the table and charts to paginate", len(pages))
	}
}

// TestBinaryPDFTextPrintsTheDerivedLongMarginPercentage is the regression
// guard for the fixture defect: 120 periods carry Rp960.000.000 of capital and
// Rp120.000.000 of margin, which is 12.50%, not the costed-denominator 11.11%.
// The extracted text is the finished PDF's, so the number a reader sees is what
// fails if the fixture regresses.
func TestBinaryPDFTextPrintsTheDerivedLongMarginPercentage(t *testing.T) {
	pdf, err := Render(longReport(poreport.MaxPeriods), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render 120 periods: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text, "Rp1.200.000.000", "Rp960.000.000", "Rp120.000.000", "12.50%")
	if got := strings.Count(text, "12.50%"); got < 2 {
		t.Errorf("extracted text carries 12.50%% %d times; want at least the KPI and the table total", got)
	}
	if strings.Contains(text, "11.11%") {
		t.Errorf("extracted text still carries the costed-denominator percentage 11.11%%")
	}
}

func pageText(pages []*page) string {
	var out strings.Builder
	for _, pg := range pages {
		for _, prim := range pg.prims {
			if value, ok := prim.(textPrim); ok {
				out.WriteString(value.text)
				out.WriteString("\n")
			}
		}
	}
	return out.String()
}

// binaryPDFText extracts the rendered document's text with whichever external
// reader this machine has. The repository's existing PDF tests use pdftotext
// and skip when it is absent; PyMuPDF is the second reader because some
// developer machines carry it without poppler. CI installs poppler-utils and
// must not skip: a check that cannot read the PDF has not verified the text.
func binaryPDFText(t *testing.T, pdf []byte) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		out, err := exec.Command("pdftotext", "-layout", path, "-").Output()
		if err != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("pdftotext failed in CI: %v", err)
			}
			t.Skipf("pdftotext failed: %v", err)
		}
		return strings.Split(string(out), "\f")
	}
	if os.Getenv("CI") != "" {
		t.Fatalf("pdftotext is required in CI; install poppler-utils")
	}
	python := "import fitz,sys\n" +
		"doc=fitz.open(sys.argv[1])\n" +
		"sys.stdout.write('\\f'.join(p.get_text() for p in doc))\n"
	out, err := exec.Command("python3", "-c", python, path).Output()
	if err != nil {
		t.Skipf("no PDF text reader available (pdftotext and PyMuPDF both failed): %v", err)
	}
	return strings.Split(string(out), "\f")
}

// TestPDFTextReaderIsInstalledInCI makes the CI contract explicit: the
// portable local skip is allowed, the CI skip is not. poppler-utils is
// installed by the Go gates workflow before this package runs.
func TestPDFTextReaderIsInstalledInCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("portable local run: the PDF text-reader checks may use PyMuPDF or skip")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Fatalf("CI must install poppler-utils so PDF text assertions actually run: %v", err)
	}
}
