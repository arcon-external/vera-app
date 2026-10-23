package extract

import (
	"context"
	"errors"
	"fmt"
	"time"

	"vera-app/backend/apicontract"
)

// Issue #478: the per-page extraction pipeline.
//
// The text layer is read first for every page. Each page then decides for
// itself whether that layer is usable: text pages keep their own words and are
// never OCRed, near-empty pages are rendered at a bounded resolution and
// handed to the OCR engine. Both kinds of words go through the same proposer,
// so a mixed document produces one coherent header and one coherent row set.
//
// Everything here is bounded before it happens. The page cap, OCR page cap,
// render pixel cap and OCR time budget are checked before a page is rendered,
// and a bound that cannot be met fails the run with a typed reason: a partial
// table looks exactly like a complete one.

// TextLayer is the text engine seam: PDF bytes in, pages of positioned words
// out. Poppler implements it in cmd/vextract; tests stub it.
type TextLayer interface {
	TextPages(ctx context.Context, pdf []byte, maxPages int) ([]Page, error)
}

// OCREngine is the OCR seam: one page rendered at a bounded DPI, words with
// confidence out. The adapter owns rendering and the OCR binary; the pipeline
// owns the decision and the bounds.
type OCREngine interface {
	Name() string
	Version() string
	OCRPage(ctx context.Context, pdf []byte, page Page, dpi int) ([]Word, error)
}

// PipelineOptions bounds a run. Zero values take the documented defaults.
type PipelineOptions struct {
	MaxPages      int
	OCRDpi        int
	MaxOCRPixels  int
	MaxOCRPages   int
	MinConfidence float64
	// OCRTimeout bounds all OCR pages together, not each page. Zero leaves the
	// caller's context as the only bound.
	OCRTimeout time.Duration
}

// Pipeline processes one document.
type Pipeline struct {
	Text    TextLayer
	OCR     OCREngine
	Options PipelineOptions
}

// Run reads the document, decides per page, OCRs what needs it, and proposes
// the contract. The extraction timestamp and duration are the caller's.
func (pipeline *Pipeline) Run(ctx context.Context, pdf []byte) (apicontract.ExtractionOutput, error) {
	if pipeline.Text == nil {
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w: no text engine is configured", ErrEngineFailed)
	}
	options := pipeline.Options
	if options.MaxPages <= 0 {
		options.MaxPages = DefaultMaxPages
	}
	if options.OCRDpi <= 0 {
		options.OCRDpi = DefaultOCRDpi
	}
	if options.MaxOCRPixels <= 0 {
		options.MaxOCRPixels = DefaultMaxOCRPixels
	}
	if options.MaxOCRPages <= 0 {
		options.MaxOCRPages = DefaultMaxOCRPages
	}
	pages, err := pipeline.Text.TextPages(ctx, pdf, options.MaxPages)
	if err != nil {
		return apicontract.ExtractionOutput{}, err
	}
	if len(pages) == 0 {
		return apicontract.ExtractionOutput{}, ErrInvalidBBoxXML
	}
	if len(pages) > options.MaxPages {
		return apicontract.ExtractionOutput{}, fmt.Errorf(
			"%w: %d pages, limit %d", ErrPageLimit, len(pages), options.MaxPages)
	}
	ocrPages := 0
	var ocrDeadline time.Time
	if options.OCRTimeout > 0 {
		ocrDeadline = time.Now().Add(options.OCRTimeout)
	}
	for index := range pages {
		page := &pages[index]
		decision := DecidePageMode(*page)
		page.TextWords = decision.Stats.Words
		page.TextChars = decision.Stats.Chars
		page.Note = decision.Reason
		if !decision.NeedsOCR {
			page.Mode = apicontract.ExtractionPageModeText
			continue
		}
		page.Mode = apicontract.ExtractionPageModeOCR
		if ocrPages >= options.MaxOCRPages {
			return apicontract.ExtractionOutput{}, fmt.Errorf(
				"%w: more than %d pages need OCR", ErrOCRLimit, options.MaxOCRPages)
		}
		if pipeline.OCR == nil {
			return apicontract.ExtractionOutput{}, fmt.Errorf(
				"%w: page %d has no usable text layer", ErrOCREngine, page.Number)
		}
		dpi, _, _, err := RenderDPI(page.Width, page.Height, options.OCRDpi, options.MaxOCRPixels)
		if err != nil {
			return apicontract.ExtractionOutput{}, fmt.Errorf("page %d: %w", page.Number, err)
		}
		pageContext := ctx
		cancel := func() {}
		if !ocrDeadline.IsZero() {
			if !time.Now().Before(ocrDeadline) {
				return apicontract.ExtractionOutput{}, fmt.Errorf(
					"%w: the OCR time budget ran out before page %d", ErrTimeout, page.Number)
			}
			pageContext, cancel = context.WithDeadline(ctx, ocrDeadline)
		}
		words, err := pipeline.OCR.OCRPage(pageContext, pdf, *page, dpi)
		deadlineExceeded := errors.Is(pageContext.Err(), context.DeadlineExceeded)
		cancel()
		if err != nil {
			if deadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
				return apicontract.ExtractionOutput{}, fmt.Errorf(
					"%w: OCR did not finish page %d in time", ErrTimeout, page.Number)
			}
			return apicontract.ExtractionOutput{}, err
		}
		page.Words = words
		page.DPI = dpi
		page.OCRWords = len(words)
		page.OCRConfidence = MeanConfidence(words)
		ocrPages++
	}
	result, err := Propose(pages, Options{MaxPages: options.MaxPages, MinConfidence: options.MinConfidence})
	if err != nil {
		return apicontract.ExtractionOutput{}, err
	}
	if pipeline.OCR != nil && ocrPages > 0 {
		result.OCREngine = pipeline.OCR.Name()
		result.OCREngineVersion = pipeline.OCR.Version()
	}
	return result, nil
}
