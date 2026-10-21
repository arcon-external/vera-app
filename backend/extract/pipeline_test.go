package extract

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vera-app/backend/apicontract"
)

// Issue #478: the per-page pipeline.
//
// The text engine is stubbed and the OCR engine counts calls, so these tests
// prove the selection rule rather than Tesseract's accuracy: a page with a
// usable text layer never invokes OCR, an image-only or near-empty page does,
// and a mixed document turns its text header into fields and its scanned table
// into rows. The real Tesseract path is a separate, opt-in Docker test.

type stubTextLayer struct {
	pages []Page
	err   error
	calls int
}

func (stub *stubTextLayer) TextPages(context.Context, []byte, int) ([]Page, error) {
	stub.calls++
	if stub.err != nil {
		return nil, stub.err
	}
	copied := make([]Page, len(stub.pages))
	copy(copied, stub.pages)
	return copied, nil
}

type stubOCREngine struct {
	words   map[int][]Word
	calls   []int
	dpi     []int
	err     error
	version string
}

func (stub *stubOCREngine) Name() string    { return "stub-ocr" }
func (stub *stubOCREngine) Version() string { return stub.version }

func (stub *stubOCREngine) OCRPage(_ context.Context, _ []byte, page Page, dpi int) ([]Word, error) {
	stub.calls = append(stub.calls, page.Number)
	stub.dpi = append(stub.dpi, dpi)
	if stub.err != nil {
		return nil, stub.err
	}
	return stub.words[page.Number], nil
}

func fixtureWords(items []testWord) []Word {
	words := make([]Word, 0, len(items))
	for _, item := range items {
		words = append(words, Word{
			Text: item.text, XMin: item.x, YMin: item.y,
			XMax: item.x + item.width, YMax: item.y + item.height,
		})
	}
	return words
}

func usableTextPage(number int) Page {
	items := append(headerWords(), tableHeaderWords()...)
	items = append(items, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	return textPage(number, fixtureWords(items))
}

func emptyPage(number int) Page {
	return Page{Number: number, Width: 595, Height: 842}
}

func withConfidence(words []Word, confidence float64) []Word {
	copied := make([]Word, len(words))
	copy(copied, words)
	for index := range copied {
		value := confidence
		copied[index].Confidence = &value
	}
	return copied
}

func ocrRowWords(y float64) []Word {
	return withConfidence(fixtureWords(rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", y)), 92)
}

func newPipeline(text TextLayer, ocr OCREngine, options PipelineOptions) *Pipeline {
	return &Pipeline{Text: text, OCR: ocr, Options: options}
}

func TestPipelineNeverOCRsAPageWithAUsableTextLayer(t *testing.T) {
	text := &stubTextLayer{pages: []Page{usableTextPage(1)}}
	ocr := &stubOCREngine{}
	pipeline := newPipeline(text, ocr, PipelineOptions{})
	result, err := pipeline.Run(context.Background(), []byte("%PDF-1.7 test"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(ocr.calls) != 0 {
		t.Fatalf("OCR was called for pages %v; a page with a usable text layer must never be OCRed", ocr.calls)
	}
	if result.PagesWithText != 1 || result.PagesWithoutText != 0 || result.PagesWithOCR != 0 {
		t.Errorf("page counts = %d text / %d without / %d ocr; want 1/0/0",
			result.PagesWithText, result.PagesWithoutText, result.PagesWithOCR)
	}
	if len(result.PageModes) != 1 || result.PageModes[0].Mode != apicontract.ExtractionPageModeText {
		t.Errorf("page modes = %+v; want one text page", result.PageModes)
	}
	if result.PageModes[0].Reason == "" {
		t.Error("the text page carries no decision reason")
	}
	if result.OCREngine != "" {
		t.Errorf("OCR engine = %q; no OCR ran, so none should be claimed", result.OCREngine)
	}
}

func TestPipelineOCRsOnlyThePagesThatNeedItAndKeepsHeaderAndRowsCoherent(t *testing.T) {
	// Page 1 is a text header with its first row; page 2 is a scan in the same
	// document. A document-level decision would either OCR the readable header
	// or trust the scan to have no text; the per-page rule does neither.
	text := &stubTextLayer{pages: []Page{usableTextPage(1), emptyPage(2)}}
	ocr := &stubOCREngine{words: map[int][]Word{2: ocrRowWords(225)}, version: "5.5.0-test"}
	result, err := newPipeline(text, ocr, PipelineOptions{OCRDpi: 300}).Run(context.Background(), []byte("%PDF-1.7 test"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(ocr.calls) != 1 || ocr.calls[0] != 2 {
		t.Fatalf("OCR calls = %v; want page 2 only", ocr.calls)
	}
	if len(ocr.dpi) != 1 || ocr.dpi[0] != 300 {
		t.Errorf("render dpi = %v; want 300", ocr.dpi)
	}
	if result.PagesWithText != 1 || result.PagesWithoutText != 1 || result.PagesWithOCR != 1 {
		t.Errorf("page counts = %d/%d/%d; want 1/1/1",
			result.PagesWithText, result.PagesWithoutText, result.PagesWithOCR)
	}
	if result.OCREngine != "stub-ocr" || result.OCREngineVersion != "5.5.0-test" {
		t.Errorf("OCR engine = %q/%q; want the adapter identity", result.OCREngine, result.OCREngineVersion)
	}
	if len(result.PageModes) != 2 ||
		result.PageModes[0].Mode != apicontract.ExtractionPageModeText ||
		result.PageModes[1].Mode != apicontract.ExtractionPageModeOCR {
		t.Fatalf("page modes = %+v; want text then ocr", result.PageModes)
	}
	ocrPage := result.PageModes[1]
	if ocrPage.DPI != 300 || ocrPage.OCRWords != len(ocrRowWords(225)) {
		t.Errorf("ocr page evidence = %+v; want 300 dpi and the OCR word count", ocrPage)
	}
	if ocrPage.Confidence == nil || *ocrPage.Confidence != 92 {
		t.Errorf("ocr page confidence = %v; want the mean 92", ocrPage.Confidence)
	}
	fields := outputFields(result)
	if fields[apicontract.ExtractionFieldPONumber].Value != "4500123456" {
		t.Errorf("poNumber = %q; want the text header's value", fields[apicontract.ExtractionFieldPONumber].Value)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d; want the text page's row and the OCR page's row (%+v)", len(result.Rows), result.Rows)
	}
	if result.Rows[0].Page != 1 || result.Rows[1].Page != 2 {
		t.Errorf("row pages = %d/%d; want 1/2", result.Rows[0].Page, result.Rows[1].Page)
	}
	if result.Rows[1].Quantity.Confidence == nil {
		t.Errorf("OCR row quantity has no confidence; the engine supplied one")
	}
	if !strings.Contains(result.Rows[1].Note, "continues that table") {
		t.Errorf("OCR row note = %q; want the carried-header warning", result.Rows[1].Note)
	}
}

func TestPipelineRefusesMoreOCRPagesThanItsBound(t *testing.T) {
	text := &stubTextLayer{pages: []Page{emptyPage(1), emptyPage(2), emptyPage(3)}}
	ocr := &stubOCREngine{}
	_, err := newPipeline(text, ocr, PipelineOptions{MaxOCRPages: 2}).Run(context.Background(), []byte("%PDF-1.7 test"))
	if !errors.Is(err, ErrOCRLimit) {
		t.Fatalf("error = %v; want ErrOCRLimit", err)
	}
	if len(ocr.calls) != 2 || ocr.calls[0] != 1 || ocr.calls[1] != 2 {
		t.Errorf("OCR calls = %v; want pages 1 and 2 only, with the refusal before page 3", ocr.calls)
	}
}

func TestPipelineRefusesAPageThatCannotFitThePixelBound(t *testing.T) {
	text := &stubTextLayer{pages: []Page{{Number: 1, Width: 5000, Height: 5000}}}
	ocr := &stubOCREngine{}
	_, err := newPipeline(text, ocr, PipelineOptions{OCRDpi: 300, MaxOCRPixels: 1_000_000}).
		Run(context.Background(), []byte("%PDF-1.7 test"))
	if !errors.Is(err, ErrPixelLimit) {
		t.Fatalf("error = %v; want ErrPixelLimit", err)
	}
	if len(ocr.calls) != 0 {
		t.Errorf("OCR calls = %v; an unfit page must not be rendered", ocr.calls)
	}
}

func TestPipelineRequiresAnOCREngineForAnImagePage(t *testing.T) {
	text := &stubTextLayer{pages: []Page{emptyPage(1)}}
	_, err := newPipeline(text, nil, PipelineOptions{}).Run(context.Background(), []byte("%PDF-1.7 test"))
	if !errors.Is(err, ErrOCREngine) {
		t.Fatalf("error = %v; want ErrOCREngine", err)
	}
}

func TestPipelinePropagatesATypedOCRFailure(t *testing.T) {
	text := &stubTextLayer{pages: []Page{emptyPage(1)}}
	ocr := &stubOCREngine{err: ErrTimeout}
	_, err := newPipeline(text, ocr, PipelineOptions{}).Run(context.Background(), []byte("%PDF-1.7 test"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v; want ErrTimeout", err)
	}
}

func TestPipelineStopsWhenTheOCRBudgetIsExhausted(t *testing.T) {
	text := &stubTextLayer{pages: []Page{emptyPage(1)}}
	ocr := &stubOCREngine{}
	_, err := newPipeline(text, ocr, PipelineOptions{OCRTimeout: time.Nanosecond}).
		Run(context.Background(), []byte("%PDF-1.7 test"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v; want ErrTimeout once the OCR budget is gone", err)
	}
	if len(ocr.calls) != 0 {
		t.Errorf("OCR calls = %v; the budget is checked before rendering", ocr.calls)
	}
}

func TestPipelineDefaultsToOneHundredPercentTextWhenNoOCRIsNeeded(t *testing.T) {
	text := &stubTextLayer{pages: []Page{usableTextPage(1), usableTextPage(2)}}
	result, err := newPipeline(text, &stubOCREngine{}, PipelineOptions{}).
		Run(context.Background(), []byte("%PDF-1.7 test"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.PageCount != 2 || result.PagesWithText != 2 || result.PagesWithOCR != 0 {
		t.Errorf("result = %+v; want two text pages", result)
	}
	for index, mode := range result.PageModes {
		if mode.Number != index+1 || mode.Mode != apicontract.ExtractionPageModeText {
			t.Errorf("page mode %d = %+v", index, mode)
		}
	}
}
