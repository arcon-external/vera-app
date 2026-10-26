package store

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Issue #472, wave 3: two stream-isolation regressions the wave-2 validator did
// not survive.
//
//  1. A /Length that is an indirect reference to an integer object defined
//     later in the file (real writers emit this) was not resolved, so the
//     scanner fell back to the first "endstream" substring. The paired fixtures
//     below draw an "endstream" and a fake object header inside the page text:
//     with a direct /Length the file is accepted, with the forward reference the
//     page bytes were read as a second object 1 and the real catalog was
//     overwritten.
//  2. An /ObjStm whose body is /FlateDecode-compressed was read encoded, so a
//     correctly indexed compressed catalog was rejected as missing.

// forwardLengthContent is the page text the paired fixtures draw. It contains a
// whole "endstream" plus a fake object header, so a scanner that lets content
// escape the stream body reads it as structure.
const forwardLengthContent = "BT /F1 12 Tf 10 50 Td (endstream\n1 0 obj\n<< /Type /Pages >>\nendobj\n) Tj ET"

// forwardLengthPDF builds the Judge's pair. The two documents differ only in
// whether object 4's /Length is written inline or as a reference to object 5;
// the page text and every object body are otherwise identical.
func forwardLengthPDF(indirect bool) []byte {
	contentLength := fmt.Sprintf("%d", len(forwardLengthContent))
	lengthEntry := contentLength
	if indirect {
		lengthEntry = "5 0 R"
	}
	return classicPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F1 6 0 R >> >> >>",
		fmt.Sprintf("<< /Length %s >>\nstream\n%s\nendstream", lengthEntry, forwardLengthContent),
		contentLength,
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}, "")
}

// forwardLengthPageText extracts the page text from a paired fixture so the
// test can prove both files carry the same bytes, not that one was built from
// the other by accident.
func forwardLengthPageText(data []byte) []byte {
	marker := []byte("stream\n")
	start := bytes.Index(data, marker)
	if start < 0 {
		return nil
	}
	start += len(marker)
	end := start + len(forwardLengthContent)
	if end > len(data) {
		return nil
	}
	return data[start:end]
}

// TestValidatePurchaseOrderPDFAcceptsPairedDirectAndForwardLength is the
// Judge's paired regression: both spellings of the same document must be
// accepted, which is only possible when the forward /Length is resolved before
// the structure pass. The catalog must still resolve to object 1.
func TestValidatePurchaseOrderPDFAcceptsPairedDirectAndForwardLength(t *testing.T) {
	direct := forwardLengthPDF(false)
	forward := forwardLengthPDF(true)

	if got := forwardLengthPageText(direct); !bytes.Equal(got, []byte(forwardLengthContent)) {
		t.Fatalf("direct fixture page text = %q; want %q", got, forwardLengthContent)
	}
	if got := forwardLengthPageText(forward); !bytes.Equal(got, []byte(forwardLengthContent)) {
		t.Fatalf("forward fixture page text = %q; want %q", got, forwardLengthContent)
	}

	for name, data := range map[string][]byte{"direct": direct, "forward": forward} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePurchaseOrderPDF(data); err != nil {
				t.Fatalf("valid PDF with a %s /Length refused: %v", name, err)
			}
		})
	}
}

// flateCatalogObject is the object-stream content: a header pair "6 0" naming
// object 6, followed by its catalog dictionary. /First is 4, the header length.
const flateCatalogObject = "6 0\n<</Type /Catalog /Pages 2 0 R>>"

// flateCompressedCatalogPDF builds an xref-stream document whose catalog lives
// in a /FlateDecode-compressed /ObjStm. filterEntry is written verbatim after
// /Filter, so a test can pass /FlateDecode, its /Fl abbreviation, an array, or
// an unsupported name.
func flateCompressedCatalogPDF(filterEntry string) []byte {
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write([]byte(flateCatalogObject)); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	body := compressed.Bytes()

	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.7\n")
	filter := ""
	if filterEntry != "" {
		filter = " /Filter " + filterEntry
	}
	fmt.Fprintf(&buffer, "1 0 obj\n<< /Type /ObjStm /N 1 /First 4%s /Length %d >>\nstream\n", filter, len(body))
	buffer.Write(body)
	buffer.WriteString("\nendstream\nendobj\n")
	buffer.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	buffer.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>\nendobj\n")
	xrefOffset := buffer.Len()
	buffer.WriteString("8 0 obj\n<< /Type /XRef /Size 9 /W [1 4 2] /Root 6 0 R /Length 0 >>\nstream\n\nendstream\nendobj\n")
	fmt.Fprintf(&buffer, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return buffer.Bytes()
}

// TestValidatePurchaseOrderPDFAcceptsAFlateCompressedCatalog pins the Judge's
// compressed-catalog regression. The object-stream header must be decompressed
// before /N, /First and the object-number list are read; an unfiltered object
// stream (already covered above) and the Flate spellings must all be accepted.
func TestValidatePurchaseOrderPDFAcceptsAFlateCompressedCatalog(t *testing.T) {
	for name, filter := range map[string]string{
		"name":                 "/FlateDecode",
		"abbreviation":         "/Fl",
		"single-element array": "[/FlateDecode]",
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePurchaseOrderPDF(flateCompressedCatalogPDF(filter)); err != nil {
				t.Fatalf("valid Flate-compressed catalog refused: %v", err)
			}
		})
	}
}

// TestValidatePurchaseOrderPDFRejectsAnUnsupportedObjectStreamFilter pins the
// rejection policy: a filter this check cannot read is named in the refusal,
// rather than surfacing as a missing catalog or being silently accepted.
func TestValidatePurchaseOrderPDFRejectsAnUnsupportedObjectStreamFilter(t *testing.T) {
	err := ValidatePurchaseOrderPDF(flateCompressedCatalogPDF("/LZWDecode"))
	if !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
		t.Fatalf("error = %v; want ErrPurchaseOrderDocumentInvalid", err)
	}
	if !strings.Contains(err.Error(), "LZWDecode") {
		t.Errorf("message %q does not name the unsupported filter", err.Error())
	}
	if !strings.Contains(err.Error(), "FlateDecode") {
		t.Errorf("message %q does not name the supported filter", err.Error())
	}
}

// TestValidatePurchaseOrderPDFRejectsACorruptFlateObjectStream pins that a
// header that cannot be decompressed is a named rejection, not a crash and not
// a silent "missing catalog".
func TestValidatePurchaseOrderPDFRejectsACorruptFlateObjectStream(t *testing.T) {
	data := flateCompressedCatalogPDF("/FlateDecode")
	start := bytes.Index(data, []byte("stream\n")) + len("stream\n")
	if start < len("stream\n") || start >= len(data) {
		t.Fatalf("fixture has no stream body")
	}
	// The zlib header's first byte is 0x78 for every level; zero is not a
	// valid compression method and makes the reader fail immediately.
	data[start] = 0x00
	err := ValidatePurchaseOrderPDF(data)
	if !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
		t.Fatalf("error = %v; want ErrPurchaseOrderDocumentInvalid", err)
	}
	if !strings.Contains(err.Error(), "FlateDecode") {
		t.Errorf("message %q does not name the filter it could not read", err.Error())
	}
}

// TestValidatePurchaseOrderPDFRejectsAForwardLengthStillNeedingAFallback is a
// control: the forward-resolution pass must not make a truncated stream
// acceptable. Object 5 is missing, so the stream has no resolvable length and
// the file must still be judged on its actual structure.
func TestValidatePurchaseOrderPDFRejectsAForwardLengthStillNeedingAFallback(t *testing.T) {
	data := forwardLengthPDF(true)
	// Drop object 5's definition entirely; the reference now names nothing.
	missing := bytes.Replace(data, []byte("5 0 obj\n"+fmt.Sprintf("%d", len(forwardLengthContent))+"\nendobj\n"), nil, 1)
	if bytes.Equal(missing, data) {
		t.Fatalf("fixture did not contain object 5")
	}
	if err := ValidatePurchaseOrderPDF(missing); err != nil {
		// The fallback still finds the first endstream inside the page text, so
		// the fake object 1 overwrites the catalog and the document is refused.
		if !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
			t.Fatalf("error = %v; want ErrPurchaseOrderDocumentInvalid", err)
		}
		return
	}
	t.Fatalf("a forward /Length naming a missing object was silently accepted")
}
