//go:build !windows

package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vera-app/backend/extract"
	"vera-app/backend/extract/pdfgen"
)

// Issue #477: the full server path with the real restricted extractor.
//
// The default API suite uses a stub engine; this opt-in test proves the same
// endpoint, store lifecycle and response shape work with the actual container
// and Poppler. It is skipped unless VERA_EXTRACTOR_DOCKER=1 and the pinned
// image exists:
//
//	docker build -f docker/po-extract/Dockerfile -t vera-po-extract:dev .
//	VERA_EXTRACTOR_DOCKER=1 go test ./backend/api -run Docker -v
//
// The PDF is generated in code; no customer document is committed, and a pass
// says nothing about real-PO accuracy (docs/po-extraction-benchmark.md §6).

func syntheticAPIPDF() []byte {
	line := func(text string, x, y float64) pdfgen.Line {
		return pdfgen.Line{Text: text, X: x, Y: y, Size: 10}
	}
	return pdfgen.Build([][]pdfgen.Line{{
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
		line("Total: 2500.00", 400, 260),
	}})
}

func TestDockerExtractionEndToEnd(t *testing.T) {
	if os.Getenv("VERA_EXTRACTOR_DOCKER") != "1" {
		t.Skip("set VERA_EXTRACTOR_DOCKER=1 to run the real extractor end to end")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	image := strings.TrimSpace(os.Getenv("VERA_EXTRACTOR_IMAGE"))
	if image == "" {
		image = "vera-po-extract:dev"
	}
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("image %s is not built; run: docker build -f docker/po-extract/Dockerfile -t %s .",
			image, image)
	}
	wrapper, err := filepath.Abs(filepath.Join("..", "..", "scripts", "vextract-docker.sh"))
	if err != nil {
		t.Fatalf("resolve wrapper: %v", err)
	}
	if _, err := os.Stat(wrapper); err != nil {
		t.Fatalf("extractor wrapper missing: %v", err)
	}

	server, token := testQuotationServer(t, "vera_api_po_extract_docker")
	server.SetExtractor(&extract.CommandEngine{
		Command:        []string{wrapper},
		Environment:    []string{"VERA_PO_EXTRACT_IMAGE=" + image},
		Timeout:        60 * time.Second,
		MaxOutputBytes: 8 << 20,
		MaxPages:       40,
	})
	quotationID, _ := exportForSelector(t, server, token, quotationInput())
	code, body := createPODraft(t, server, token, quotationID, 0, nil)
	if code != http.StatusCreated {
		t.Fatalf("create draft = %d (%v)", code, body)
	}
	draft := decodePurchaseOrder(t, body)
	code, body = uploadPurchaseOrderDocument(t, server, token, draft.ID, "Synthetic PO.pdf", syntheticAPIPDF())
	if code != http.StatusCreated {
		t.Fatalf("upload = %d (%v)", code, body)
	}
	if code, body := startExtraction(t, server, token, draft.ID); code != http.StatusAccepted {
		t.Fatalf("start = %d (%v)", code, body)
	}
	state := waitForExtraction(t, server, token, draft.ID)
	if state.Run == nil || state.Run.Status != "succeeded" {
		t.Fatalf("run = %+v; want succeeded", state.Run)
	}
	if state.Run.Engine != "poppler" || state.Run.EngineVersion == "" {
		t.Errorf("engine = %q/%q; want poppler with a pinned version", state.Run.Engine, state.Run.EngineVersion)
	}
	if len(state.Run.Result) == 0 {
		t.Fatal("succeeded run has no result")
	}
	result := state.Run.Result
	for _, want := range []string{`"key":"poNumber"`, `"value":"4500123456"`, `"key":"incoterms"`, `"value":"CIF Batam"`, `"lineNumber"`} {
		if !strings.Contains(string(result), want) {
			t.Errorf("result is missing %s: %s", want, string(result))
		}
	}
}
