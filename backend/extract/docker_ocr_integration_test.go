//go:build !windows

package extract

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"vera-app/backend/apicontract"
	"vera-app/backend/extract/pdfgen"
)

// Issue #478: the real Tesseract path, opt-in.
//
// These tests build synthetic scans from the repository's text PDFs: a page is
// rendered by the pinned Poppler inside the extractor image at a chosen DPI,
// optionally skewed, and wrapped back into a PDF whose content is only that
// image. No customer document and no binary fixture is committed.
//
// They prove plumbing and the per-page decision, not accuracy: Tesseract still
// has to read the words, and #476 §2.5 is not validated (no ground truth or
// holdout exists). Run with:
//
//	docker build -f docker/po-extract/Dockerfile -t vera-po-extract:dev .
//	VERA_EXTRACTOR_DOCKER=1 go test ./backend/extract -run DockerOCR -v

func requireDockerExtractorImage(t *testing.T) string {
	t.Helper()
	if os.Getenv("VERA_EXTRACTOR_DOCKER") != "1" {
		t.Skip("set VERA_EXTRACTOR_DOCKER=1 to run the real OCR container tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	image := dockerImage()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("image %s is not built; run: docker build -f docker/po-extract/Dockerfile -t %s .",
			image, image)
	}
	return image
}

func runExtractorContainer(t *testing.T, image string, pdf []byte) (apicontract.ExtractionOutput, string) {
	t.Helper()
	command := exec.Command("docker", "run", "--rm", "-i",
		"--network", "none",
		"--read-only",
		"--tmpfs", "/tmp:rw,size=256m",
		"--memory", "512m",
		"--memory-swap", "512m",
		"--pids-limit", "64",
		"--cpus", "1",
		"--cap-drop", "ALL",
		"-e", "HOME=/tmp",
		"-e", "XDG_CACHE_HOME=/tmp",
		image)
	command.Stdin = bytes.NewReader(pdf)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("extractor image failed: %v\nstderr: %s", err, stderr.String())
	}
	var output apicontract.ExtractionOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode extractor output: %v\n%s", err, stdout.String())
	}
	return output, stderr.String()
}

// renderPDFPage rasterises one page with the pinned Poppler inside the image.
// Writing the PNG to stdout avoids a host bind mount, which keeps the test
// usable on any Docker host.
func renderPDFPage(t *testing.T, image string, pdf []byte, dpi int) image.Image {
	t.Helper()
	script := "cat > /tmp/in.pdf && pdftoppm -r " + strconv.Itoa(dpi) +
		" -png -singlefile /tmp/in.pdf /tmp/out && cat /tmp/out.png"
	command := exec.Command("docker", "run", "--rm", "-i",
		"--tmpfs", "/tmp:rw,size=256m",
		"-e", "HOME=/tmp",
		"-e", "XDG_CACHE_HOME=/tmp",
		"--entrypoint", "sh", image, "-c", script)
	command.Stdin = bytes.NewReader(pdf)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("render the synthetic scan: %v\nstderr: %s", err, stderr.String())
	}
	decoded, err := png.Decode(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		t.Fatalf("decode the rendered page: %v", err)
	}
	return decoded
}

func scannedPage(t *testing.T, image string, pdf []byte, dpi int, rotate float64) pdfgen.ImagePage {
	t.Helper()
	rendered := renderPDFPage(t, image, pdf, dpi)
	return pdfgen.NewImagePage(rendered, rotate)
}

func headerAndFirstRow() []pdfgen.Line {
	return []pdfgen.Line{
		{Text: "PURCHASE ORDER", X: 50, Y: 60},
		{Text: "PO No: 4500123456", X: 50, Y: 90},
		{Text: "PO Date: 2026-10-12", X: 50, Y: 105},
		{Text: "Currency: USD", X: 50, Y: 120},
		{Text: "No", X: 50, Y: 200}, {Text: "Description", X: 90, Y: 200},
		{Text: "Qty", X: 300, Y: 200}, {Text: "UoM", X: 350, Y: 200},
		{Text: "Unit Price", X: 400, Y: 200}, {Text: "Amount", X: 500, Y: 200},
		{Text: "1", X: 50, Y: 225}, {Text: "Electrical panel", X: 90, Y: 225},
		{Text: "2", X: 300, Y: 225}, {Text: "Pc", X: 350, Y: 225},
		{Text: "1250.00", X: 400, Y: 225}, {Text: "2500.00", X: 500, Y: 225},
	}
}

func continuedRows() []pdfgen.Line {
	return []pdfgen.Line{
		{Text: "3", X: 50, Y: 225}, {Text: "Freight", X: 90, Y: 225},
		{Text: "1", X: 300, Y: 225}, {Text: "Lot", X: 350, Y: 225},
		{Text: "150.00", X: 400, Y: 225}, {Text: "150.00", X: 500, Y: 225},
		{Text: "4", X: 50, Y: 243}, {Text: "Packing", X: 90, Y: 243},
		{Text: "2", X: 300, Y: 243}, {Text: "Pc", X: 350, Y: 243},
		{Text: "25.00", X: 400, Y: 243}, {Text: "50.00", X: 500, Y: 243},
	}
}

// The mixed fixture is the ticket's central case: a readable text header and
// first row on page 1, and a scanned continuation table on page 2 with no
// repeated column header.
func mixedTextHeaderAndScannedTable(t *testing.T, image string, scanDPI int, rotate float64) []byte {
	t.Helper()
	scan := scannedPage(t, image, pdfgen.Build([][]pdfgen.Line{continuedRows()}), scanDPI, rotate)
	return pdfgen.BuildMixed([]pdfgen.Page{
		{Lines: headerAndFirstRow()},
		{Image: &scan},
	})
}

func TestDockerOCRReadsAMixedTextHeaderAndScannedTable(t *testing.T) {
	image := requireDockerExtractorImage(t)
	document := mixedTextHeaderAndScannedTable(t, image, 300, 0)
	output, stderr := runExtractorContainer(t, image, document)

	if output.PageCount != 2 || len(output.PageModes) != 2 {
		t.Fatalf("page evidence = %+v (stderr %s)", output, stderr)
	}
	if output.PageModes[0].Mode != apicontract.ExtractionPageModeText {
		t.Errorf("page 1 mode = %q; want text", output.PageModes[0].Mode)
	}
	if output.PageModes[1].Mode != apicontract.ExtractionPageModeOCR || output.PageModes[1].OCRWords == 0 {
		t.Fatalf("page 2 mode = %+v; want OCR with words", output.PageModes[1])
	}
	if output.PageModes[1].DPI != 300 {
		t.Errorf("page 2 dpi = %d; want 300", output.PageModes[1].DPI)
	}
	if output.PagesWithOCR != 1 || output.PagesWithoutText != 1 || output.PagesWithText != 1 {
		t.Errorf("page counts = %d text/%d without/%d ocr; want 1/1/1",
			output.PagesWithText, output.PagesWithoutText, output.PagesWithOCR)
	}
	if output.OCREngine != "tesseract" || output.OCREngineVersion == "" {
		t.Errorf("OCR engine = %q/%q; want a pinned tesseract", output.OCREngine, output.OCREngineVersion)
	}
	fields := map[string]apicontract.ExtractionField{}
	for _, field := range output.Fields {
		fields[field.Key] = field
	}
	poNumber, ok := fields[apicontract.ExtractionFieldPONumber]
	if !ok || poNumber.Status == apicontract.ExtractionStatusAbsent || poNumber.Value != "4500123456" {
		t.Errorf("poNumber = %+v; want the text header's value", poNumber)
	}
	if len(output.Rows) < 2 {
		t.Fatalf("rows = %d; want the text row and at least one OCR row (%+v)", len(output.Rows), output.Rows)
	}
	var ocrRow *apicontract.ExtractionRow
	for index := range output.Rows {
		if output.Rows[index].Page == 2 {
			ocrRow = &output.Rows[index]
			break
		}
	}
	if ocrRow == nil {
		t.Fatalf("no row came from the scanned page: %+v", output.Rows)
	}
	if !strings.Contains(ocrRow.Note, "column header is on page 1") {
		t.Errorf("OCR row note = %q; want the carried-header warning", ocrRow.Note)
	}
	if ocrRow.Status != apicontract.ExtractionStatusNeedsReview {
		t.Errorf("OCR row status = %q; a carried header must be needs_review", ocrRow.Status)
	}
	if ocrRow.Quantity.Confidence == nil {
		t.Errorf("OCR row quantity has no confidence: %+v", ocrRow.Quantity)
	}
}

func TestDockerOCRReadsA150DPIScanAndASkewedScan(t *testing.T) {
	image := requireDockerExtractorImage(t)
	for _, variant := range []struct {
		name   string
		dpi    int
		rotate float64
	}{
		{"150 dpi", 150, 0},
		{"300 dpi skewed", 300, 3},
	} {
		t.Run(variant.name, func(t *testing.T) {
			scan := scannedPage(t, image, pdfgen.Build([][]pdfgen.Line{continuedRows()}), variant.dpi, variant.rotate)
			document := pdfgen.BuildImagePDF([]pdfgen.ImagePage{scan})
			output, stderr := runExtractorContainer(t, image, document)
			if len(output.PageModes) != 1 || output.PageModes[0].Mode != apicontract.ExtractionPageModeOCR {
				t.Fatalf("page mode = %+v; want OCR (stderr %s)", output.PageModes, stderr)
			}
			if output.PageModes[0].OCRWords == 0 {
				t.Errorf("OCR read no words at %d dpi; the render or OCR wiring is broken", variant.dpi)
			}
			if output.OCREngine != "tesseract" || output.OCREngineVersion == "" {
				t.Errorf("OCR engine = %q/%q", output.OCREngine, output.OCREngineVersion)
			}
		})
	}
}

func TestDockerOCRReadsAFullyScannedDocument(t *testing.T) {
	image := requireDockerExtractorImage(t)
	firstPage := scannedPage(t, image, pdfgen.Build([][]pdfgen.Line{headerAndFirstRow()}), 300, 0)
	secondPage := scannedPage(t, image, pdfgen.Build([][]pdfgen.Line{continuedRows()}), 300, 0)
	output, stderr := runExtractorContainer(t, image, pdfgen.BuildImagePDF([]pdfgen.ImagePage{firstPage, secondPage}))

	if len(output.PageModes) != 2 || output.PagesWithOCR != 2 {
		t.Fatalf("page evidence = %+v (stderr %s)", output.PageModes, stderr)
	}
	fields := map[string]apicontract.ExtractionField{}
	for _, field := range output.Fields {
		fields[field.Key] = field
	}
	poNumber := fields[apicontract.ExtractionFieldPONumber]
	if poNumber.Status == apicontract.ExtractionStatusAbsent {
		t.Errorf("poNumber = %+v; OCR should have read the header, even if it needs review", poNumber)
	}
	if len(output.Rows) == 0 {
		t.Errorf("rows = 0; OCR should have proposed at least one line (%+v)", output.Rows)
	}
}
