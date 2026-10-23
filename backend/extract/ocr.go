package extract

import (
	"fmt"
	"strconv"
	"strings"
)

// Issue #478: Tesseract's TSV word map.
//
// Tesseract reports one row per layout level (page, block, paragraph, line,
// word) in pixels, with a 0..100 confidence on word rows. Only level 5 rows are
// words; every other level is structure this reader does not need because the
// shared line grouper rebuilds lines from word positions. Pixel boxes are
// converted to PDF points so OCR evidence and text-layer evidence share one
// coordinate system.
//
// An empty word map (a blank page) is a valid answer. A malformed map is not:
// silently returning no words would look exactly like a blank page.

const (
	ocrTSVWordLevel = 5
	ocrTSVColumns   = 12
)

// ParseOCRTSV turns Tesseract's TSV output into words in PDF points.
func ParseOCRTSV(data []byte, dpi int) ([]Word, error) {
	if dpi <= 0 {
		return nil, fmt.Errorf("%w: the render resolution is %d dpi", ErrInvalidOCRTSV, dpi)
	}
	lines := strings.Split(string(data), "\n")
	headerSeen := false
	words := make([]Word, 0, len(lines)/2)
	for index, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !headerSeen {
			if !strings.HasPrefix(line, "level\t") {
				return nil, fmt.Errorf("%w: line %d is not the TSV header", ErrInvalidOCRTSV, index+1)
			}
			headerSeen = true
			continue
		}
		fields := strings.SplitN(line, "\t", ocrTSVColumns)
		if len(fields) != ocrTSVColumns {
			return nil, fmt.Errorf("%w: line %d has %d columns, want %d",
				ErrInvalidOCRTSV, index+1, len(fields), ocrTSVColumns)
		}
		level, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			return nil, fmt.Errorf("%w: line %d has an unreadable level", ErrInvalidOCRTSV, index+1)
		}
		left, err := strconv.Atoi(strings.TrimSpace(fields[6]))
		if err != nil {
			return nil, fmt.Errorf("%w: line %d has an unreadable left edge", ErrInvalidOCRTSV, index+1)
		}
		top, err := strconv.Atoi(strings.TrimSpace(fields[7]))
		if err != nil {
			return nil, fmt.Errorf("%w: line %d has an unreadable top edge", ErrInvalidOCRTSV, index+1)
		}
		width, err := strconv.Atoi(strings.TrimSpace(fields[8]))
		if err != nil {
			return nil, fmt.Errorf("%w: line %d has an unreadable width", ErrInvalidOCRTSV, index+1)
		}
		height, err := strconv.Atoi(strings.TrimSpace(fields[9]))
		if err != nil {
			return nil, fmt.Errorf("%w: line %d has an unreadable height", ErrInvalidOCRTSV, index+1)
		}
		confidence, err := strconv.ParseFloat(strings.TrimSpace(fields[10]), 64)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d has an unreadable confidence", ErrInvalidOCRTSV, index+1)
		}
		if left < 0 || top < 0 || width < 0 || height < 0 {
			return nil, fmt.Errorf("%w: line %d has a pixel box outside the image", ErrInvalidOCRTSV, index+1)
		}
		if level != ocrTSVWordLevel {
			continue
		}
		text := strings.TrimSpace(fields[11])
		if text == "" {
			continue
		}
		if width == 0 || height == 0 {
			return nil, fmt.Errorf("%w: word %q has an empty pixel box", ErrInvalidOCRTSV, text)
		}
		if confidence > 100 {
			return nil, fmt.Errorf("%w: word %q has confidence %.1f", ErrInvalidOCRTSV, text, confidence)
		}
		scale := 72 / float64(dpi)
		word := Word{
			Text: text,
			XMin: float64(left) * scale,
			YMin: float64(top) * scale,
			XMax: float64(left+width) * scale,
			YMax: float64(top+height) * scale,
		}
		if confidence >= 0 {
			measured := confidence
			word.Confidence = &measured
		}
		words = append(words, word)
	}
	if !headerSeen {
		return nil, fmt.Errorf("%w: the output has no TSV header", ErrInvalidOCRTSV)
	}
	return words, nil
}

// MeanConfidence is the average confidence of the words the OCR engine
// measured, or nil when it measured none. It describes the page, not any one
// cell: the per-cell gate uses the minimum word confidence.
func MeanConfidence(words []Word) *float64 {
	total := 0.0
	count := 0
	for _, word := range words {
		if word.Confidence == nil {
			continue
		}
		total += *word.Confidence
		count++
	}
	if count == 0 {
		return nil
	}
	mean := total / float64(count)
	return &mean
}
