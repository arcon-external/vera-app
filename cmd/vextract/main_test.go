package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"vera-app/backend/apicontract"
	"vera-app/backend/extract"
)

// Issue #477: the extractor executable itself.
//
// The real engine is Poppler; these tests replace pdftotext with a tiny script
// that prints a synthetic word map, so the command's stdin/stdout contract,
// page cap, version pinning and failure shapes are exercised without Docker or
// a browser. The Docker integration test is the separate, opt-in proof that
// the same command reads a real PDF with the installed Poppler.

func fakePDFToText(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pdftotext")
	script := `#!/bin/sh
if [ "$1" = "-v" ]; then
  printf 'pdftotext version 0.0-test\n' >&2
  exit 0
fi
` + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pdftotext: %v", err)
	}
	return path
}

func wordMapXML(pages ...string) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?><html><body><doc>`)
	for _, words := range pages {
		builder.WriteString(`<page width="595" height="842">` + words + `</page>`)
	}
	builder.WriteString(`</doc></body></html>`)
	return builder.String()
}

func wordLine(x, y float64, text string) string {
	// The production reader groups by y; one block per word is valid input.
	return `<flow><block><line><word xMin="` + strconv.FormatFloat(x, 'f', 3, 64) +
		`" yMin="` + strconv.FormatFloat(y, 'f', 3, 64) +
		`" xMax="` + strconv.FormatFloat(x+40, 'f', 3, 64) +
		`" yMax="` + strconv.FormatFloat(y+10, 'f', 3, 64) + `">` + text + `</word></line></block></flow>`
}

func runVextract(t *testing.T, stdin []byte, args ...string) (int, []byte, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, bytes.NewReader(stdin), &stdout, &stderr)
	return code, stdout.Bytes(), stderr.String()
}

func TestVextractReadsStdinAndPrintsTheContract(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
cat <<'XML'
`+wordMapXML(
		wordLine(50, 90, "PO")+wordLine(67, 90, "No:")+wordLine(86, 90, "4500123456")+
			wordLine(50, 105, "PO")+wordLine(67, 105, "Date:")+wordLine(95, 105, "2026-10-12")+
			wordLine(50, 120, "Currency:")+wordLine(98, 120, "USD")+
			wordLine(50, 200, "No")+wordLine(90, 200, "Description")+wordLine(300, 200, "Qty")+
			wordLine(350, 200, "UoM")+wordLine(400, 200, "Unit")+wordLine(421, 200, "Price")+
			wordLine(500, 200, "Amount")+
			wordLine(50, 225, "1")+wordLine(90, 225, "Electrical")+wordLine(130, 225, "panel")+
			wordLine(300, 225, "2")+wordLine(350, 225, "Pc")+wordLine(400, 225, "1250.00")+
			wordLine(500, 225, "2500.00")+
			wordLine(400, 260, "Total:")+wordLine(440, 260, "2500.00"),
	)+`
XML`)

	code, stdout, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"),
		"-pdftotext", pdftotext, "-max-pages", "40")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var output apicontract.ExtractionOutput
	if err := json.Unmarshal(stdout, &output); err != nil {
		t.Fatalf("decode output: %v (%s)", err, stdout)
	}
	if output.Engine != EngineName || output.EngineVersion != "0.0-test" {
		t.Errorf("engine = %q/%q; want %q/0.0-test", output.Engine, output.EngineVersion, EngineName)
	}
	if output.PageCount != 1 || len(output.Rows) != 1 {
		t.Errorf("result = %+v", output)
	}
	if got := output.Fields[0]; got.Key != apicontract.ExtractionFieldPONumber || got.Value != "4500123456" {
		t.Errorf("poNumber = %+v", got)
	}
	if output.Rows[0].Quantity.Value != "2" || output.Rows[0].UnitPrice.Value != "1250.00" {
		t.Errorf("row = %+v", output.Rows[0])
	}
}

func TestVextractRefusesTooManyPagesAndBadInput(t *testing.T) {
	pages := make([]string, 3)
	for index := range pages {
		pages[index] = wordLine(50, 90, "text")
	}
	pdftotext := fakePDFToText(t, `cat >/dev/null
cat <<'XML'
`+wordMapXML(pages...)+`
XML`)
	code, _, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"),
		"-pdftotext", pdftotext, "-max-pages", "2")
	if code != 2 || !strings.Contains(stderr, "page_limit") {
		t.Fatalf("exit = %d, stderr = %q; want the typed page-limit refusal", code, stderr)
	}

	code, _, stderr = runVextract(t, nil, "-pdftotext", pdftotext)
	if code != 2 || !strings.Contains(stderr, "invalid_input") {
		t.Fatalf("empty stdin exit = %d, stderr = %q", code, stderr)
	}
}

func TestVextractReportsAnUnreadablePDF(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
printf 'Syntax Error: not a PDF\n' >&2
exit 1`)
	code, _, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"), "-pdftotext", pdftotext)
	if code != 2 || !strings.Contains(stderr, "invalid_pdf") {
		t.Fatalf("exit = %d, stderr = %q; want invalid_pdf", code, stderr)
	}
}

func TestVextractReportsTimeout(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
sleep 5`)
	code, _, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"),
		"-pdftotext", pdftotext, "-timeout", "50ms")
	if code != 2 || !strings.Contains(stderr, "timeout") {
		t.Fatalf("exit = %d, stderr = %q; want timeout", code, stderr)
	}
}

func TestVextractRejectsMalformedWordMap(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
printf 'not xml'`)
	code, _, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"), "-pdftotext", pdftotext)
	if code != 2 || !strings.Contains(stderr, "invalid_output") {
		t.Fatalf("exit = %d, stderr = %q; want invalid_output", code, stderr)
	}
}

func TestVextractVersionFlag(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null`)
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-pdftotext", pdftotext, "-version"},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0.0-test") {
		t.Errorf("version output = %q", stdout.String())
	}
}

// The engine must be able to turn the command's typed failure prefix into its
// own error, or the service would store every refusal as a generic failure.
func TestCommandFailurePrefixesAreTyped(t *testing.T) {
	cases := []struct {
		stderr string
		want   error
	}{
		{"page_limit: too many pages", extract.ErrPageLimit},
		{"timeout: the text engine took too long", extract.ErrTimeout},
		{"invalid_output: not a word map", extract.ErrInvalidOutput},
	}
	for _, testCase := range cases {
		if got := extract.ClassifyFailure(testCase.stderr, nil); !errors.Is(got, testCase.want) {
			t.Errorf("ClassifyFailure(%q) = %v; want %v", testCase.stderr, got, testCase.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Issue #478: per-page OCR wiring
// ---------------------------------------------------------------------------

// The 1x1 PNG is only a container for the render step: these tests assert which
// pages are OCRed, not what an image looks like. The real renderer and the real
// Tesseract are exercised by the opt-in Docker test.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func fakeOCRBundle(t *testing.T, tsv string) (pdftoppm, tesseract, calls string) {
	t.Helper()
	directory := t.TempDir()
	pdftoppm = filepath.Join(directory, "pdftoppm")
	renderer := `#!/bin/sh
for last; do :; done
printf '%s' '` + onePixelPNG + `' | base64 -d > "$last.png"
`
	if err := os.WriteFile(pdftoppm, []byte(renderer), 0o755); err != nil {
		t.Fatalf("write fake pdftoppm: %v", err)
	}
	calls = filepath.Join(directory, "ocr-calls")
	tesseract = filepath.Join(directory, "tesseract")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'tesseract 9.9.9-test\n leptonica-1.84.1\n'
  exit 0
fi
printf 'call\n' >> "$OCR_CALLS"
cat <<'TSV'
` + tsv + `
TSV
`
	if err := os.WriteFile(tesseract, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tesseract: %v", err)
	}
	t.Setenv("OCR_CALLS", calls)
	return pdftoppm, tesseract, calls
}

func ocrTSV() string {
	return "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t50\t225\t20\t10\t91.0\t2\n" +
		"5\t1\t1\t1\t1\t2\t90\t225\t80\t10\t94.0\tFreight\n"
}

func TestVextractNeverRunsOCRForAUsableTextPage(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
cat <<'XML'
`+wordMapXML(
		wordLine(50, 90, "PO")+wordLine(67, 90, "No:")+wordLine(86, 90, "4500123456")+
			wordLine(50, 105, "PO")+wordLine(67, 105, "Date:")+wordLine(95, 105, "2026-10-12")+
			wordLine(50, 200, "No")+wordLine(90, 200, "Description")+wordLine(300, 200, "Qty")+
			wordLine(350, 200, "UoM")+wordLine(400, 200, "Unit")+wordLine(421, 200, "Price")+
			wordLine(500, 200, "Amount")+
			wordLine(50, 225, "1")+wordLine(90, 225, "Panel")+wordLine(300, 225, "2")+
			wordLine(350, 225, "Pc")+wordLine(400, 225, "1250.00")+wordLine(500, 225, "2500.00"),
	)+`
XML`)
	// The OCR binaries do not exist. A text page must succeed anyway: the
	// only way this test can fail is if a text page is sent to OCR.
	code, stdout, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"),
		"-pdftotext", pdftotext,
		"-pdftoppm", filepath.Join(t.TempDir(), "missing-pdftoppm"),
		"-tesseract", filepath.Join(t.TempDir(), "missing-tesseract"))
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q; a text page must not touch OCR", code, stderr)
	}
	var output apicontract.ExtractionOutput
	if err := json.Unmarshal(stdout, &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output.PageModes) != 1 || output.PageModes[0].Mode != apicontract.ExtractionPageModeText {
		t.Errorf("page modes = %+v; want one text page", output.PageModes)
	}
}

func TestVextractOCRsOnlyThePagesWithoutAUsableTextLayer(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
cat <<'XML'
`+wordMapXML(
		wordLine(50, 90, "PO")+wordLine(67, 90, "No:")+wordLine(86, 90, "4500123456")+
			wordLine(50, 105, "PO")+wordLine(67, 105, "Date:")+wordLine(95, 105, "2026-10-12")+
			wordLine(50, 200, "No")+wordLine(90, 200, "Description")+wordLine(300, 200, "Qty")+
			wordLine(350, 200, "UoM")+wordLine(400, 200, "Unit")+wordLine(421, 200, "Price")+
			wordLine(500, 200, "Amount")+
			wordLine(50, 225, "1")+wordLine(90, 225, "Panel")+wordLine(300, 225, "2")+
			wordLine(350, 225, "Pc")+wordLine(400, 225, "1250.00")+wordLine(500, 225, "2500.00"),
		"",
	)+`
XML`)
	pdftoppm, tesseract, calls := fakeOCRBundle(t, ocrTSV())
	code, stdout, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"),
		"-pdftotext", pdftotext, "-pdftoppm", pdftoppm, "-tesseract", tesseract)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var output apicontract.ExtractionOutput
	if err := json.Unmarshal(stdout, &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output.PageModes) != 2 ||
		output.PageModes[0].Mode != apicontract.ExtractionPageModeText ||
		output.PageModes[1].Mode != apicontract.ExtractionPageModeOCR {
		t.Fatalf("page modes = %+v; want text then ocr", output.PageModes)
	}
	if output.PageModes[1].OCRWords != 2 || output.PageModes[1].DPI != 300 {
		t.Errorf("ocr page = %+v; want two words at 300 dpi", output.PageModes[1])
	}
	if output.OCREngine != OCREngineName || output.OCREngineVersion != "9.9.9-test" {
		t.Errorf("ocr engine = %q/%q; want %s/9.9.9-test", output.OCREngine, output.OCREngineVersion, OCREngineName)
	}
	counted, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("read OCR call log: %v", err)
	}
	if got := strings.Count(string(counted), "call\n"); got != 1 {
		t.Errorf("OCR ran %d times; want once, for the image-only page", got)
	}
}

func TestVextractFailsWithATypedCodeWhenOCRIsUnavailable(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
cat <<'XML'
`+wordMapXML("")+`
XML`)
	pdftoppm, _, _ := fakeOCRBundle(t, ocrTSV())
	code, _, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"),
		"-pdftotext", pdftotext,
		"-pdftoppm", pdftoppm,
		"-tesseract", filepath.Join(t.TempDir(), "missing-tesseract"))
	if code != 2 || !strings.Contains(stderr, "ocr_unavailable") {
		t.Fatalf("exit = %d, stderr = %q; want the typed OCR-unavailable refusal", code, stderr)
	}
}

func TestVextractRefusesAnOCRPageThatCannotFitThePixelBound(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
cat <<'XML'
`+wordMapXML("")+`
XML`)
	pdftoppm, tesseract, _ := fakeOCRBundle(t, ocrTSV())
	code, _, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"),
		"-pdftotext", pdftotext, "-pdftoppm", pdftoppm, "-tesseract", tesseract,
		"-max-ocr-pixels", "1000")
	if code != 2 || !strings.Contains(stderr, "page_too_large") {
		t.Fatalf("exit = %d, stderr = %q; want page_too_large", code, stderr)
	}
}

func TestVextractVersionReportsBothEngines(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null`)
	_, tesseract, _ := fakeOCRBundle(t, ocrTSV())
	var stdout, stderr bytes.Buffer
	code := run(context.Background(),
		[]string{"-pdftotext", pdftotext, "-tesseract", tesseract, "-version"},
		strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0.0-test") || !strings.Contains(stdout.String(), "9.9.9-test") {
		t.Errorf("version output = %q; want both engine versions", stdout.String())
	}
}

func TestVextractClassifiesTheNewTypedFailures(t *testing.T) {
	cases := []struct {
		stderr string
		want   error
	}{
		{"ocr_page_limit: too many scanned pages", extract.ErrOCRLimit},
		{"page_too_large: the page cannot be rendered", extract.ErrPixelLimit},
		{"ocr_unavailable: tesseract is not installed", extract.ErrOCREngine},
	}
	for _, testCase := range cases {
		if got := extract.ClassifyFailure(testCase.stderr, nil); !errors.Is(got, testCase.want) {
			t.Errorf("ClassifyFailure(%q) = %v; want %v", testCase.stderr, got, testCase.want)
		}
	}
}
