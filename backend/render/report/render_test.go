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
		"-",
		"Definitions",
	)
}

func TestBinaryPDFTextKeepsTheLongTableComplete(t *testing.T) {
	pdf, err := Render(longReport(120), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render 120 periods: %v", err)
	}
	pages := binaryPDFText(t, pdf)
	assertReportPages(t, "binaryPDFText", pages)
	text := strings.Join(pages, "\n")
	requireText(t, text, "Jan 2016", "Dec 2025", "Page 1 of")
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

// splitPDFPages turns a text reader's output into pages. pdftotext separates
// pages with a form feed and terminates the document with one more, so a naive
// strings.Split reports an extra empty page after the real last page. Trimming
// exactly one trailing form feed removes that record-separator artifact while
// preserving a genuinely blank middle or final page (which is represented by an
// empty span between two separators).
func splitPDFPages(text string) []string {
	if strings.HasSuffix(text, "\f") {
		text = strings.TrimSuffix(text, "\f")
	}
	return strings.Split(text, "\f")
}

func readPDFWithPoppler(path string) (string, error) {
	out, err := exec.Command("pdftotext", "-layout", path, "-").Output()
	return string(out), err
}

func readPDFWithPyMuPDF(path string) (string, error) {
	python := "import fitz,sys\n" +
		"doc=fitz.open(sys.argv[1])\n" +
		"sys.stdout.write('\\f'.join(p.get_text() for p in doc))\n"
	out, err := exec.Command("python3", "-c", python, path).Output()
	return string(out), err
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
		text, err := readPDFWithPoppler(path)
		if err != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("pdftotext failed in CI: %v", err)
			}
			t.Skipf("pdftotext failed: %v", err)
		}
		return splitPDFPages(text)
	}
	if os.Getenv("CI") != "" {
		t.Fatalf("pdftotext is required in CI; install poppler-utils")
	}
	text, err := readPDFWithPyMuPDF(path)
	if err != nil {
		t.Skipf("no PDF text reader available (pdftotext and PyMuPDF both failed): %v", err)
	}
	return splitPDFPages(text)
}

// assertReportPages verifies that an extracted page list has no empty
// artifact page and that every real page carries its own "Page i of n" footer.
// It is stronger than checking only the last footer: an extractor that appends
// or drops a page fails on the page that disagrees, not on a count the test
// computed from the same broken list.
func assertReportPages(t *testing.T, reader string, pages []string) {
	t.Helper()
	if len(pages) == 0 {
		t.Fatalf("%s returned no pages", reader)
	}
	for index, page := range pages {
		if strings.TrimSpace(page) == "" {
			t.Errorf("%s page %d is blank; that is a reader delimiter artifact or a renderer defect, never a report page",
				reader, index+1)
			continue
		}
		want := fmt.Sprintf("Page %d of %d", index+1, len(pages))
		if !strings.Contains(page, want) {
			t.Errorf("%s page %d does not carry its footer %q", reader, index+1, want)
		}
	}
}

// TestSplitPDFPagesNormalizesOnlyTheTerminalDelimiter pins the two reader
// shapes: pdftotext ends with a form feed, PyMuPDF does not. Both must produce
// the same real pages, and a blank middle or final page must survive.
func TestSplitPDFPagesNormalizesOnlyTheTerminalDelimiter(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"poppler terminal delimiter", "page1\fpage2\f", []string{"page1", "page2"}},
		{"pymupdf no terminal delimiter", "page1\fpage2", []string{"page1", "page2"}},
		{"single page terminal delimiter", "page1\f", []string{"page1"}},
		{"single page no delimiter", "page1", []string{"page1"}},
		{"blank middle page", "page1\f\fpage2\f", []string{"page1", "", "page2"}},
		{"blank final page", "page1\f\f", []string{"page1", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitPDFPages(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("splitPDFPages(%q) = %q; want %q", tc.raw, got, tc.want)
			}
			for index := range tc.want {
				if got[index] != tc.want[index] {
					t.Fatalf("splitPDFPages(%q) = %q; want %q", tc.raw, got, tc.want)
				}
			}
		})
	}
}

// TestPDFTextReadersAgreeOnPageFooters exercises both reader branches when the
// machine has them. poppler is mandatory in CI; PyMuPDF is optional. Both must
// agree on the real page count and every page's footer, which is the check that
// catches a terminal-delimiter page without inventing a renderer page.
func TestPDFTextReadersAgreeOnPageFooters(t *testing.T) {
	pdf, err := Render(longReport(poreport.MaxPeriods), os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render 120 periods: %v", err)
	}
	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}

	counts := map[string]int{}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		text, err := readPDFWithPoppler(path)
		if err != nil {
			// A present-but-unloadable poppler is an environment fault. CI
			// must fail; a local machine may fall back to PyMuPDF exactly as
			// binaryPDFText does.
			if os.Getenv("CI") != "" {
				t.Fatalf("pdftotext failed in CI: %v", err)
			}
			t.Logf("pdftotext is present but failed locally: %v", err)
		} else {
			pages := splitPDFPages(text)
			assertReportPages(t, "pdftotext", pages)
			counts["pdftotext"] = len(pages)
		}
	} else if os.Getenv("CI") != "" {
		t.Fatalf("pdftotext is required in CI; install poppler-utils")
	}

	if text, err := readPDFWithPyMuPDF(path); err == nil {
		pages := splitPDFPages(text)
		assertReportPages(t, "PyMuPDF", pages)
		counts["PyMuPDF"] = len(pages)
	}

	if len(counts) == 0 {
		t.Skip("no PDF text reader available (pdftotext and PyMuPDF both failed)")
	}
	if len(counts) == 2 {
		if counts["pdftotext"] != counts["PyMuPDF"] {
			t.Errorf("pdftotext reports %d pages, PyMuPDF reports %d; both must see the same document",
				counts["pdftotext"], counts["PyMuPDF"])
		}
	}
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

// TestBinaryPDFUSDAxisAppliesTheCurrencyExponent pins the axis/table agreement
// for a two-decimal currency. The axis takes minor-unit ticks like the table, so
// compacting before applying the exponent printed "$150M" for $1.5M on the same
// page whose table said "$12,345,678.90": a 100x contradiction.
func TestBinaryPDFUSDAxisAppliesTheCurrencyExponent(t *testing.T) {
	snapshot := sampleReport()
	snapshot.Filter.Currency = "USD"
	pdf, err := Render(snapshot, os.DirFS("../../assets"))
	if err != nil {
		t.Fatalf("Render USD: %v", err)
	}
	text := strings.Join(binaryPDFText(t, pdf), "\n")
	requireText(t, text, "$12,345,678.90", "$1.5M", "$500K")
	if strings.Contains(text, "Rp") {
		t.Errorf("USD report prints a rupiah amount")
	}
	if strings.Contains(text, "$150M") {
		t.Errorf("USD axis compacted minor units without the currency exponent: $150M for $1.5M")
	}
}
