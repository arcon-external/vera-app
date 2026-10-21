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

func TestVextractAbstainsWhenThereIsNoText(t *testing.T) {
	pdftotext := fakePDFToText(t, `cat >/dev/null
cat <<'XML'
`+wordMapXML("")+`
XML`)
	code, stdout, stderr := runVextract(t, []byte("%PDF-1.7 synthetic"), "-pdftotext", pdftotext)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	var output apicontract.ExtractionOutput
	if err := json.Unmarshal(stdout, &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if output.PagesWithoutText != 1 || len(output.Rows) != 0 {
		t.Errorf("result = %+v", output)
	}
	for _, field := range output.Fields {
		if field.Status != apicontract.ExtractionStatusAbsent {
			t.Errorf("field %s = %q; want absent", field.Key, field.Status)
		}
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
