package extract

import (
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"strings"
)

// Issue #477: Poppler's -bbox-layout output, parsed into pages and words.
//
// The XML is XHTML-shaped, but only three things matter here: the page size,
// the word text, and the word's four edges. Blocks and flows are deliberately
// ignored -- a writer may put each text run in its own block, so grouping by
// Poppler's containers would fail on a perfectly readable page. Lines are
// reconstructed from the word y positions in propose.go.

var (
	// ErrInvalidBBoxXML rejects output that is not the pdftotext word map, or
	// one that has no pages at all. A silent empty result would look like an
	// image-only document, which is a different answer.
	ErrInvalidBBoxXML = errors.New("the PDF text engine returned no readable word map")
	// ErrPageLimit is the bounded-document refusal. It is an error, not an
	// abstention: the document was not read, so nothing can be proposed.
	ErrPageLimit = errors.New("the PDF has more pages than this release extracts")
)

// Word is one printed word and its box in PDF points, with the origin at the
// page's top-left as pdftotext prints it.
type Word struct {
	Text string
	XMin float64
	YMin float64
	XMax float64
	YMax float64
}

// Page is one PDF page's size and words, in reading order.
type Page struct {
	Number int
	Width  float64
	Height float64
	Words  []Word
}

type bboxXMLDocument struct {
	Pages []bboxXMLPage `xml:"body>doc>page"`
}

type bboxXMLPage struct {
	Width  float64       `xml:"width,attr"`
	Height float64       `xml:"height,attr"`
	Words  []bboxXMLWord `xml:"flow>block>line>word"`
}

type bboxXMLWord struct {
	Text string  `xml:",chardata"`
	XMin float64 `xml:"xMin,attr"`
	YMin float64 `xml:"yMin,attr"`
	XMax float64 `xml:"xMax,attr"`
	YMax float64 `xml:"yMax,attr"`
}

// ParseBBoxLayout turns pdftotext -bbox-layout output into pages of words.
func ParseBBoxLayout(data []byte) ([]Page, error) {
	var document bboxXMLDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidBBoxXML, err)
	}
	if len(document.Pages) == 0 {
		return nil, fmt.Errorf("%w: it has no pages", ErrInvalidBBoxXML)
	}
	pages := make([]Page, 0, len(document.Pages))
	for index, source := range document.Pages {
		if source.Width <= 0 || source.Height <= 0 ||
			math.IsNaN(source.Width) || math.IsNaN(source.Height) ||
			math.IsInf(source.Width, 0) || math.IsInf(source.Height, 0) {
			return nil, fmt.Errorf("%w: page %d has no usable size", ErrInvalidBBoxXML, index+1)
		}
		page := Page{
			Number: index + 1,
			Width:  source.Width,
			Height: source.Height,
			Words:  make([]Word, 0, len(source.Words)),
		}
		for _, sourceWord := range source.Words {
			text := strings.TrimSpace(sourceWord.Text)
			if text == "" {
				continue
			}
			if !finite(sourceWord.XMin) || !finite(sourceWord.YMin) ||
				!finite(sourceWord.XMax) || !finite(sourceWord.YMax) ||
				sourceWord.XMax < sourceWord.XMin || sourceWord.YMax < sourceWord.YMin {
				return nil, fmt.Errorf("%w: page %d has an unusable word box", ErrInvalidBBoxXML, index+1)
			}
			page.Words = append(page.Words, Word{
				Text: text,
				XMin: sourceWord.XMin,
				YMin: sourceWord.YMin,
				XMax: sourceWord.XMax,
				YMax: sourceWord.YMax,
			})
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func unionBox(words []Word) (xMin, yMin, xMax, yMax float64) {
	if len(words) == 0 {
		return 0, 0, 0, 0
	}
	xMin, yMin = words[0].XMin, words[0].YMin
	xMax, yMax = words[0].XMax, words[0].YMax
	for _, word := range words[1:] {
		xMin = math.Min(xMin, word.XMin)
		yMin = math.Min(yMin, word.YMin)
		xMax = math.Max(xMax, word.XMax)
		yMax = math.Max(yMax, word.YMax)
	}
	return xMin, yMin, xMax, yMax
}

func joinWords(words []Word) string {
	var builder strings.Builder
	for index, word := range words {
		if index > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(word.Text)
	}
	return builder.String()
}
