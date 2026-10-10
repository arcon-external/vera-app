//go:build chromedp

package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"vera-app/backend/render"
	chromedprenderer "vera-app/backend/render/chromedp"
)

// The mock harness generates a mixed three-state document (state B package, a
// plain line, a state-C package, detail bullets under parts) through the REAL
// customer pipeline: Prepare prices it and the shipping template executes it.
// Both engines render it -- the browser engine and the native engine -- so the
// two outputs can be compared side by side while the native sub-row drawing is
// being fitted. The native render goes through render.Active, which is the
// engine production actually exports with.
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
	browserPDF, err := chromedprenderer.New().Render(context.Background(), in)
	if err != nil {
		t.Fatalf("browser render: %v", err)
	}
	browserPath := filepath.Join(dir, "mixed-states-browser.pdf")
	if err := os.WriteFile(browserPath, browserPDF, 0o644); err != nil {
		t.Fatalf("write %s: %v", browserPath, err)
	}

	nativePDF, err := render.Active.Render(context.Background(), in)
	if err != nil {
		t.Fatalf("native render: %v", err)
	}
	nativePath := filepath.Join(dir, "mixed-states-native.pdf")
	if err := os.WriteFile(nativePath, nativePDF, 0o644); err != nil {
		t.Fatalf("write %s: %v", nativePath, err)
	}
	t.Logf("artifacts: %s, %s, %s", htmlPath, browserPath, nativePath)
}
