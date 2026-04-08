package backend

import (
	"context"
	"os"
	"testing"
)

func TestGeneratePDF(t *testing.T) {
	q := &Quotation{
		ctx: context.Background(),
	}

	document, err := q.build()

	if err != nil {
		t.Fatal(err)
	}

	outputFile := "bin/quotation.pdf"

	err = document.Save(outputFile)

	if err != nil {
		t.Fatalf("Failed to save document: %v", err)
	}

	t.Logf("Test PDF generated successfully: %s", outputFile)

	info, err := os.Stat(outputFile)
	if err != nil || info.Size() == 0 {
		t.Error("Generated PDF is empty or missing")
	}
}
