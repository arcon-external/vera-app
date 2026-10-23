package extract

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// Issue #478: Tesseract's TSV word map.
//
// Tesseract reports pixels and a 0..100 confidence. The contract stores PDF
// points, because that is what the page map and the text layer use, and keeps
// the confidence so a low-confidence value can become needs_review instead of a
// fact. These tests pin the conversion, the word-level filter and the refusal
// of a malformed map.

const tsvHeader = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"

func tsvRow(level int, left, top, width, height int, conf float64, text string) string {
	return strings.Join([]string{
		itoa(level), "1", "1", "1", "1", "1",
		itoa(left), itoa(top), itoa(width), itoa(height),
		formatFloat(conf), text,
	}, "\t") + "\n"
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func TestParseOCRTSVReadsWordsInPointsWithConfidence(t *testing.T) {
	data := tsvHeader +
		tsvRow(1, 0, 0, 2480, 3509, -1, "") +
		tsvRow(5, 211, 260, 221, 32, 95.51, "PURCHASE") +
		tsvRow(5, 453, 260, 142, 32, 96.45, "ORDER") +
		tsvRow(4, 211, 260, 384, 32, -1, "") +
		tsvRow(5, 358, 385, 227, 32, 88.0, "4500123456")
	words, err := ParseOCRTSV([]byte(data), 300)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(words) != 3 {
		t.Fatalf("words = %d; want 3 word-level rows (%+v)", len(words), words)
	}
	first := words[0]
	if first.Text != "PURCHASE" || first.Confidence == nil || *first.Confidence != 95.51 {
		t.Errorf("first word = %+v; want PURCHASE at 95.51", first)
	}
	// 211 px at 300 dpi = 50.64 pt; 260 px = 62.4 pt.
	if first.XMin < 50.5 || first.XMin > 50.8 || first.YMin < 62.3 || first.YMin > 62.5 {
		t.Errorf("first word box = %+v; want the pixel box converted to PDF points", first)
	}
	if first.XMax-first.XMin < 52.9 || first.XMax-first.XMin > 53.2 {
		t.Errorf("first word width = %f pt; want 221 px at 300 dpi (53.04 pt)", first.XMax-first.XMin)
	}
}

func TestParseOCRTSVAcceptsAWordMapWithNoWords(t *testing.T) {
	words, err := ParseOCRTSV([]byte(tsvHeader+tsvRow(1, 0, 0, 2480, 3509, -1, "")), 300)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(words) != 0 {
		t.Errorf("words = %+v; an empty OCR result is a valid answer", words)
	}
	if confidence := MeanConfidence(words); confidence != nil {
		t.Errorf("mean confidence = %v; want nil with no words", *confidence)
	}
}

func TestParseOCRTSVRejectsAMalformedMap(t *testing.T) {
	cases := map[string]string{
		"missing columns": "level\tpage_num\tblock_num\n5\t1\t1\n",
		"bad level":       tsvHeader + "five\t1\t1\t1\t1\t1\t0\t0\t10\t10\t90\tword\n",
		"bad left":        tsvHeader + "5\t1\t1\t1\t1\t1\tleft\t0\t10\t10\t90\tword\n",
		"negative box":    tsvHeader + "5\t1\t1\t1\t1\t1\t-5\t0\t10\t10\t90\tword\n",
		"zero width":      tsvHeader + "5\t1\t1\t1\t1\t1\t0\t0\t0\t10\t90\tword\n",
		"bad confidence":  tsvHeader + "5\t1\t1\t1\t1\t1\t0\t0\t10\t10\tgreat\tword\n",
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseOCRTSV([]byte(data), 300); !errors.Is(err, ErrInvalidOCRTSV) {
				t.Fatalf("error = %v; want ErrInvalidOCRTSV", err)
			}
		})
	}
	if _, err := ParseOCRTSV([]byte(tsvHeader), 0); !errors.Is(err, ErrInvalidOCRTSV) {
		t.Errorf("zero dpi error = %v; want ErrInvalidOCRTSV", err)
	}
}

func TestParseOCRTSVKeepsAnUnknownConfidenceNull(t *testing.T) {
	// A word-level row with a negative confidence is not a measured word; the
	// word is kept so its text is not silently lost, but no confidence is
	// claimed.
	words, err := ParseOCRTSV([]byte(tsvHeader+tsvRow(5, 10, 10, 20, 10, -1, "Pc")), 300)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(words) != 1 || words[0].Confidence != nil {
		t.Fatalf("words = %+v; want one word with no confidence", words)
	}
}

func TestParseOCRTSVReadsWindowsLineEndings(t *testing.T) {
	data := strings.ReplaceAll(tsvHeader+tsvRow(5, 0, 0, 10, 10, 90, "PO"), "\n", "\r\n")
	words, err := ParseOCRTSV([]byte(data), 150)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(words) != 1 || words[0].Text != "PO" {
		t.Errorf("words = %+v; want PO", words)
	}
}

func TestMeanConfidenceAveragesOnlyMeasuredWords(t *testing.T) {
	low := 50.0
	high := 90.0
	mean := MeanConfidence([]Word{{Confidence: &low}, {Confidence: &high}, {}})
	if mean == nil {
		t.Fatal("mean confidence is nil; want the average of the measured words")
	}
	if *mean != 70 {
		t.Errorf("mean confidence = %v; want 70", *mean)
	}
}
