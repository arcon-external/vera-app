package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	chromedprenderer "vera-app/backend/render/chromedp"
)

// The mock harness generates a mixed three-state document (state B package, a
// plain line, a state-C package, detail lines under both) through the REAL
// customer pipeline: Prepare prices it and the shipping template executes it.
// The PDF is drawn by constructing the browser engine directly here -- the
// same constructor the engine swap in backend/render names -- because the
// export path's build() refuses sub-item documents until the native renderer
// learns sub rows. This is a mock-generation tool, not a second selection
// mechanism: nothing outside this file reaches the browser engine.
//
// Run with VERA_VISUAL_ARTIFACT_DIR set; the normal suite does nothing.
func TestWriteSubItemRenderArtifact(t *testing.T) {
	dir := os.Getenv("VERA_VISUAL_ARTIFACT_DIR")
	if dir == "" {
		t.Skip("set VERA_VISUAL_ARTIFACT_DIR to write the render artifact")
	}
	q := prepareMixedStates(t)

	page, err := q.GetQuotationPreviewHTML()
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	htmlPath := filepath.Join(dir, "mixed-states.html")
	if err := os.WriteFile(htmlPath, []byte(page), 0o644); err != nil {
		t.Fatalf("write %s: %v", htmlPath, err)
	}

	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("renderer input: %v", err)
	}
	pdf, err := chromedprenderer.New().Render(context.Background(), in)
	if err != nil {
		t.Fatalf("browser render: %v", err)
	}
	pdfPath := filepath.Join(dir, "mixed-states.pdf")
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil {
		t.Fatalf("write %s: %v", pdfPath, err)
	}
	t.Logf("artifacts written to %s and %s", htmlPath, pdfPath)
}
