package extract

import (
	"errors"
	"math"
	"testing"
)

// Issue #478: per-page processing decision.
//
// The decision is the reason a text header page cannot conceal a scanned table
// page: it is made for every page, never once for the document. These tests
// pin the two sides of the threshold (usable text stays text, near-empty text
// is OCRed) and the bounded render geometry.

func textLine(x, y float64, words ...string) []Word {
	result := make([]Word, 0, len(words))
	for index, text := range words {
		offset := float64(index) * 40
		result = append(result, Word{
			Text: text, XMin: x + offset, YMin: y, XMax: x + offset + 35, YMax: y + 10,
		})
	}
	return result
}

func textPage(number int, words []Word) Page {
	return Page{Number: number, Width: 595, Height: 842, Words: words}
}

func TestDecidePageModeUsesTheTextLayerForAUsablePage(t *testing.T) {
	page := textPage(1, append(
		textLine(50, 90, "PO", "No:", "4500123456"),
		textLine(50, 200, "No", "Description", "Qty", "UoM", "Unit", "Price", "Amount")...),
	)
	decision := DecidePageMode(page)
	if decision.NeedsOCR {
		t.Fatalf("decision = %+v; a page with a usable text layer must not be OCRed", decision)
	}
	if decision.Stats.Words < MinTextLayerWords || decision.Stats.Chars < MinTextLayerChars {
		t.Errorf("stats = %+v; want at least %d words and %d characters",
			decision.Stats, MinTextLayerWords, MinTextLayerChars)
	}
	if decision.Reason == "" {
		t.Error("the decision carries no reason; the run envelope must explain it")
	}
}

func TestDecidePageModeOCRsAnEmptyOrNearEmptyPage(t *testing.T) {
	cases := []struct {
		name string
		page Page
	}{
		{"no text at all", textPage(2, nil)},
		{"a page number only", textPage(2, textLine(300, 800, "3"))},
		{"punctuation only", textPage(2, textLine(50, 90, "-", "|", "—"))},
		{"a few stray characters", textPage(2, textLine(50, 90, "PO", "No"))},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decision := DecidePageMode(testCase.page)
			if !decision.NeedsOCR {
				t.Fatalf("decision = %+v; want OCR for a page with no usable text layer", decision)
			}
			if decision.Reason == "" {
				t.Error("the decision carries no reason")
			}
		})
	}
}

func TestDecidePageModeKeepsASparseButRealTextPageAsText(t *testing.T) {
	// A continuation page with three short rows is still a text page: whether
	// the rows continue a table is a row-parsing question, not a reason to
	// re-read words the PDF already carries.
	words := append(textLine(50, 225, "3", "Freight", "1", "Lot", "150.00", "150.00"),
		textLine(50, 243, "4", "Packing", "2", "Pc", "25.00", "50.00")...)
	words = append(words, textLine(50, 261, "5", "Insurance", "1", "Lot", "75.00", "75.00")...)
	page := textPage(2, words)
	decision := DecidePageMode(page)
	if decision.NeedsOCR {
		t.Fatalf("decision = %+v; a readable text table must stay on the text path", decision)
	}
}

func TestRenderDPIStaysWithinThePixelBound(t *testing.T) {
	dpi, width, height, err := RenderDPI(595, 842, 300, DefaultMaxOCRPixels)
	if err != nil {
		t.Fatalf("render geometry: %v", err)
	}
	if dpi != 300 {
		t.Errorf("dpi = %d; want the requested 300 for an A4 page", dpi)
	}
	if width*height > DefaultMaxOCRPixels {
		t.Errorf("pixels = %d; want at most %d", width*height, DefaultMaxOCRPixels)
	}
	if width != int(math.Ceil(595.0*300.0/72.0)) {
		t.Errorf("width = %d; want the 300 dpi pixel width", width)
	}
}

func TestRenderDPIReducesAnOversizedPageInsteadOfFailing(t *testing.T) {
	// A3 at 300 dpi is 17.4 MP, above the 12 MP bound. The renderer drops to
	// the largest whole DPI that fits rather than shipping a huge image or
	// refusing the document.
	dpi, width, height, err := RenderDPI(842, 1191, 300, DefaultMaxOCRPixels)
	if err != nil {
		t.Fatalf("render geometry: %v", err)
	}
	if dpi >= 300 || dpi < MinOCRDpi {
		t.Errorf("dpi = %d; want a reduced DPI at or above %d", dpi, MinOCRDpi)
	}
	if width*height > DefaultMaxOCRPixels {
		t.Errorf("pixels = %d; want at most %d", width*height, DefaultMaxOCRPixels)
	}
}

func TestRenderDPIRefusesAPageThatCannotFitAtTheFloor(t *testing.T) {
	_, _, _, err := RenderDPI(5000, 5000, 300, 1_000_000)
	if !errors.Is(err, ErrPixelLimit) {
		t.Fatalf("error = %v; want ErrPixelLimit", err)
	}
}

func TestRenderDPIRejectsUnusableGeometry(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		width, height float64
		dpi           int
	}{
		{"zero width", 0, 842, 300},
		{"negative height", 595, -1, 300},
		{"not a number", math.NaN(), 842, 300},
		{"infinite height", 595, math.Inf(1), 300},
		{"zero dpi", 595, 842, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, _, err := RenderDPI(testCase.width, testCase.height, testCase.dpi, DefaultMaxOCRPixels); !errors.Is(err, ErrPixelLimit) {
				t.Fatalf("error = %v; want ErrPixelLimit", err)
			}
		})
	}
}
