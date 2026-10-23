//go:build !windows

package extract

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"vera-app/backend/apicontract"
	"vera-app/backend/extract/pdfgen"
)

// Issue #477: the real Poppler path, opt-in.
//
// The default suite uses a stub engine so it needs no Docker. This test is the
// proof that the same binary and the same parser read a real text PDF with the
// installed Poppler, and it is skipped unless VERA_EXTRACTOR_DOCKER=1:
//
//	docker build -f docker/po-extract/Dockerfile -t vera-po-extract:dev .
//	VERA_EXTRACTOR_DOCKER=1 go test ./backend/extract -run Docker -v
//
// The PDF is generated in code with known values; no customer document is
// committed, and a pass here proves plumbing and contract shape only. It says
// nothing about real-PO accuracy (docs/po-extraction-benchmark.md §6).

func dockerImage() string {
	if image := strings.TrimSpace(os.Getenv("VERA_EXTRACTOR_IMAGE")); image != "" {
		return image
	}
	return "vera-po-extract:dev"
}

func syntheticPurchaseOrderPDF() []byte {
	line := func(text string, x, y float64) pdfgen.Line {
		return pdfgen.Line{Text: text, X: x, Y: y, Size: 10}
	}
	return pdfgen.Build([][]pdfgen.Line{{
		line("PURCHASE ORDER", 50, 60),
		line("PO No: 4500123456", 50, 90),
		line("PO Date: 2026-10-12", 50, 105),
		line("Currency: USD", 50, 120),
		line("Payment Terms: 30 days after delivery", 50, 135),
		line("Incoterms: CIF Batam", 50, 150),
		line("No", 50, 200),
		line("Description", 90, 200),
		line("Qty", 300, 200),
		line("UoM", 350, 200),
		line("Unit Price", 400, 200),
		line("Amount", 500, 200),
		line("1", 50, 225),
		line("Electrical panel", 90, 225),
		line("2", 300, 225),
		line("Pc", 350, 225),
		line("1250.00", 400, 225),
		line("2500.00", 500, 225),
		line("2", 50, 243),
		line("Cable termination", 90, 243),
		line("10.5", 300, 243),
		line("M", 350, 243),
		line("42.00", 400, 243),
		line("441.00", 500, 243),
		line("Total: 2941.00", 400, 280),
	}})
}

func TestDockerExtractorReadsSyntheticTextPDF(t *testing.T) {
	if os.Getenv("VERA_EXTRACTOR_DOCKER") != "1" {
		t.Skip("set VERA_EXTRACTOR_DOCKER=1 to run the real Poppler container test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	image := dockerImage()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("image %s is not built; run: docker build -f docker/po-extract/Dockerfile -t %s .",
			image, image)
	}

	command := exec.Command("docker", "run", "--rm", "-i",
		"--network", "none",
		"--read-only",
		"--tmpfs", "/tmp:rw,size=64m",
		"--memory", "512m",
		"--memory-swap", "512m",
		"--pids-limit", "64",
		"--cpus", "1",
		"--cap-drop", "ALL",
		"-e", "HOME=/tmp",
		image)
	command.Stdin = bytes.NewReader(syntheticPurchaseOrderPDF())
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
	if output.Engine != "poppler" || output.EngineVersion == "" {
		t.Errorf("engine = %q/%q; want poppler with a pinned version", output.Engine, output.EngineVersion)
	}
	if output.PageCount != 1 || output.PagesWithText != 1 {
		t.Errorf("page counts = %+v", output)
	}
	fields := map[string]apicontract.ExtractionField{}
	for _, field := range output.Fields {
		fields[field.Key] = field
	}
	for key, want := range map[string]string{
		apicontract.ExtractionFieldPONumber:     "4500123456",
		apicontract.ExtractionFieldPODate:       "2026-10-12",
		apicontract.ExtractionFieldCurrency:     "USD",
		apicontract.ExtractionFieldPaymentTerms: "30 days after delivery",
		apicontract.ExtractionFieldIncoterms:    "CIF Batam",
	} {
		field, ok := fields[key]
		if !ok {
			t.Errorf("field %s is missing", key)
			continue
		}
		if field.Value != want || field.Status != apicontract.ExtractionStatusExtracted {
			t.Errorf("%s = %q/%q; want %q/extracted", key, field.Value, field.Status, want)
		}
		if field.BBox == nil || field.Page == nil {
			t.Errorf("%s has no page/bbox evidence: %+v", key, field)
		}
		if field.Confidence != nil {
			t.Errorf("%s reported confidence %v; Poppler does not supply one", key, *field.Confidence)
		}
	}
	if len(output.Rows) != 2 {
		t.Fatalf("rows = %d; want 2 (%+v)", len(output.Rows), output.Rows)
	}
	first := output.Rows[0]
	if first.Description.Value != "Electrical panel" || first.Quantity.Value != "2" ||
		first.UoM.Value != "Pc" || first.UnitPrice.Value != "1250.00" ||
		first.Amount.Value != "2500.00" || first.Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("row 1 = %+v", first)
	}
	if output.Rows[1].Quantity.Value != "10.5" || output.Rows[1].UnitPrice.Value != "42.00" {
		t.Errorf("row 2 = %+v", output.Rows[1])
	}
	if output.Rows[0].Page != 1 || output.Rows[0].BBox == nil {
		t.Errorf("row 1 evidence = page %d bbox %+v", output.Rows[0].Page, output.Rows[0].BBox)
	}
}
