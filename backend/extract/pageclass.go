package extract

import (
	"fmt"
	"math"
	"unicode"
)

// Issue #478: the per-page processing decision.
//
// A document is not "a text PDF" or "a scan": a text header page and a scanned
// table page coexist, so the decision is made for every page. A page whose
// text layer carries at least MinTextLayerWords words and MinTextLayerChars
// letters/digits is read from that layer and never OCRed. A page below that
// floor -- image-only, blank, or a scan with a stray page number -- is rendered
// and OCRed.
//
// The floor is deliberately low. It is a usability floor, not a quality score:
// a text page with ten short table rows passes, while a scan with a printed
// page number and a stamp does not. A page that passes is never re-OCRed, so
// the text layer's exact characters and positions stay the evidence.
const (
	// DefaultOCRDpi is the render resolution for OCR pages when the operator
	// does not configure one.
	DefaultOCRDpi = 300
	// MinOCRDpi is the floor a page is never rendered below. If a page cannot
	// fit the pixel budget at this resolution it is refused, not blurred.
	MinOCRDpi = 150
	// MaxOCRDpi bounds an operator-configured resolution.
	MaxOCRDpi = 600
	// DefaultMaxOCRPixels bounds one rendered page. A4 at 300 dpi is 8.7 MP;
	// A3 at 300 dpi is reduced to fit.
	DefaultMaxOCRPixels = 12_000_000
	// DefaultMaxOCRPages bounds how many pages one document may send to OCR.
	DefaultMaxOCRPages = 20

	// MinTextLayerWords and MinTextLayerChars define the usable text layer.
	MinTextLayerWords = 8
	MinTextLayerChars = 40
)

// PageTextStats is the text layer's size, counted in words and in letters/
// digits. Punctuation-only tokens ("-", "|", "—") are not text: a scan with a
// table rule often yields a few of those from the text layer.
type PageTextStats struct {
	Words int
	Chars int
}

// PageDecision is the per-page answer stored in the run envelope.
type PageDecision struct {
	NeedsOCR bool
	Stats    PageTextStats
	Reason   string
}

// TextLayerStats counts the usable words and characters in a page's text layer.
func TextLayerStats(page Page) PageTextStats {
	stats := PageTextStats{}
	for _, word := range page.Words {
		letters := 0
		for _, character := range word.Text {
			if unicode.IsLetter(character) || unicode.IsDigit(character) {
				letters++
			}
		}
		if letters == 0 {
			continue
		}
		stats.Words++
		stats.Chars += letters
	}
	return stats
}

// DecidePageMode answers whether a page must be OCRed, with the evidence that
// led to the answer. It never looks at other pages: that is the whole point.
func DecidePageMode(page Page) PageDecision {
	stats := TextLayerStats(page)
	usable := stats.Words >= MinTextLayerWords && stats.Chars >= MinTextLayerChars
	decision := PageDecision{NeedsOCR: !usable, Stats: stats}
	if usable {
		decision.Reason = fmt.Sprintf(
			"The page's text layer carries %d words and %d letters/digits, at the %d-word/%d-character usability floor; it was read without OCR.",
			stats.Words, stats.Chars, MinTextLayerWords, MinTextLayerChars)
		return decision
	}
	decision.Reason = fmt.Sprintf(
		"The page's text layer carries only %d words and %d letters/digits, below the %d-word/%d-character usability floor; the page was rendered and OCRed.",
		stats.Words, stats.Chars, MinTextLayerWords, MinTextLayerChars)
	return decision
}

// RenderDPI returns the largest whole DPI at or below the requested one that
// keeps the rendered page inside the pixel bound. A page that cannot fit the
// bound at MinOCRDpi is refused with ErrPixelLimit: a result rendered below the
// floor would be evidence nobody should trust.
func RenderDPI(widthPt, heightPt float64, requestedDPI, maxPixels int) (int, int, int, error) {
	if !finite(widthPt) || !finite(heightPt) || widthPt <= 0 || heightPt <= 0 {
		return 0, 0, 0, fmt.Errorf("%w: the page has no usable size", ErrPixelLimit)
	}
	if requestedDPI < MinOCRDpi || requestedDPI > MaxOCRDpi {
		return 0, 0, 0, fmt.Errorf("%w: a render resolution of %d dpi is outside %d..%d",
			ErrPixelLimit, requestedDPI, MinOCRDpi, MaxOCRDpi)
	}
	if maxPixels <= 0 {
		maxPixels = DefaultMaxOCRPixels
	}
	dpi := requestedDPI
	for {
		width, height, fits := pixelSize(widthPt, heightPt, dpi, maxPixels)
		if fits {
			return dpi, width, height, nil
		}
		if dpi <= MinOCRDpi {
			break
		}
		ideal := int(math.Floor(math.Sqrt(float64(maxPixels) * 72 * 72 / (widthPt * heightPt))))
		if ideal >= dpi {
			ideal = dpi - 1
		}
		if ideal < MinOCRDpi {
			ideal = MinOCRDpi
		}
		dpi = ideal
	}
	return 0, 0, 0, fmt.Errorf(
		"%w: a %.0fx%.0f point page does not fit %d pixels even at %d dpi",
		ErrPixelLimit, widthPt, heightPt, maxPixels, MinOCRDpi)
}

// pixelSize computes the rendered dimensions and whether they fit the bound,
// entirely in float until the fit is known, so an absurd page cannot overflow
// an int conversion.
func pixelSize(widthPt, heightPt float64, dpi, maxPixels int) (int, int, bool) {
	scale := float64(dpi) / 72
	width := math.Ceil(widthPt * scale)
	height := math.Ceil(heightPt * scale)
	if !finite(width) || !finite(height) || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	if width*height > float64(maxPixels) {
		return 0, 0, false
	}
	return int(width), int(height), true
}
