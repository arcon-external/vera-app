// package backend
//
// import (
// 	"context"
// 	"os"
// 	"testing"
// )
//
// func TestGeneratePDF(t *testing.T) {
// 	q := &Quotation{
// 		ctx: context.Background(),
// 	}
//
// 	document, err := q.build()
//
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	outputFile := "bin/quotation.pdf"
//
// 	err = document.Save(outputFile)
//
// 	if err != nil {
// 		t.Fatalf("Failed to save document: %v", err)
// 	}
//
// 	t.Logf("Test PDF generated successfully: %s", outputFile)
//
// 	info, err := os.Stat(outputFile)
// 	if err != nil || info.Size() == 0 {
// 		t.Error("Generated PDF is empty or missing")
// 	}
// }

package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePDF(t *testing.T) {
	// Use a background context for chromedp
	ctx := context.Background()
	q := &Quotation{}

	// 1. Setup the data (Mocking what Startup normally does)
	// IMPORTANT: Make sure Startup doesn't crash if q.ctx is nil
	err := Startup(q, ctx)
	if err != nil {
		t.Fatalf("startup failed: %v", err)
	}

	// 2. Test the specific logic path, NOT the Wails GUI dialog
	tempPath := "bin/quotation.pdf"
	err = q.save(tempPath)

	if err != nil {
		t.Fatalf("Failed to generate and save PDF: %v", err)
	}

	// 3. Verify the file exists and has content
	info, err := os.Stat(tempPath)
	if os.IsNotExist(err) {
		t.Fatal("PDF file was not created")
	}
	if info.Size() == 0 {
		t.Fatal("PDF file is empty")
	}

	// Clean up after test (optional)
	// os.Remove(tempPath)
}

func TestValidateSavePathRejectsNonPDF(t *testing.T) {
	tempDir := t.TempDir()
	_, err := validateSavePath(filepath.Join(tempDir, "quotation.txt"))
	if err == nil {
		t.Fatal("expected non-pdf path to be rejected")
	}
}

func TestValidateSavePathRejectsDirectory(t *testing.T) {
	tempDir := t.TempDir()
	_, err := validateSavePath(tempDir)
	if err == nil {
		t.Fatal("expected directory path to be rejected")
	}
}

func TestValidateSavePathAcceptsPDF(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "quotation.pdf")
	absPath, err := validateSavePath(inputPath)
	if err != nil {
		t.Fatalf("expected valid pdf path, got error: %v", err)
	}

	expectedPath, err := filepath.Abs(inputPath)
	if err != nil {
		t.Fatalf("resolve expected absolute path: %v", err)
	}

	if absPath != expectedPath {
		t.Fatalf("unexpected validated path: got %q want %q", absPath, expectedPath)
	}
}
