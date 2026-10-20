package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Issue #472: a customer PO PDF is stored only when it is a real, unlocked PDF.
// These tests pin the validation contract: the header is not enough, the
// document must carry a resolvable cross-reference section, a catalog and an
// end marker, an encrypted file is refused with its own error, and the byte
// bound is enforced before anything reaches Postgres.

// minimalPDF builds a structurally real classic PDF: every object has its
// offset in an xref table, startxref points at that table, and the trailer
// names the catalog. The validator checks structure rather than rendering, but
// the xref offset has to be real because that is what a reader follows.
func minimalPDF(extraObjects []string, trailerExtra string) []byte {
	return classicPDF(
		append([]string{
			"<< /Type /Catalog /Pages 2 0 R >>",
			"<< /Type /Pages /Kids [] /Count 0 >>",
		}, extraObjects...),
		trailerExtra,
	)
}

// classicPDF builds a classic-xref document from object bodies. Object numbers
// start at 1 in slice order.
func classicPDF(objects []string, trailerExtra string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.7\n")
	offsets := make([]int, 0, len(objects))
	for index, object := range objects {
		offsets = append(offsets, buffer.Len())
		fmt.Fprintf(&buffer, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xrefOffset := buffer.Len()
	fmt.Fprintf(&buffer, "xref\n0 %d\n", len(objects)+1)
	buffer.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&buffer, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buffer, "trailer\n<< /Size %d /Root 1 0 R", len(objects)+1)
	if trailerExtra != "" {
		buffer.WriteString(" ")
		buffer.WriteString(trailerExtra)
	}
	fmt.Fprintf(&buffer, " >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)
	return buffer.Bytes()
}

func TestValidatePurchaseOrderPDFAcceptsARealPDF(t *testing.T) {
	data := minimalPDF(nil, "")
	if err := ValidatePurchaseOrderPDF(data); err != nil {
		t.Fatalf("valid PDF refused: %v", err)
	}
}

func TestValidatePurchaseOrderPDFAcceptsBinaryContentAndTrailingWhitespace(t *testing.T) {
	// A content stream may hold arbitrary bytes, including NUL and high bytes,
	// and real files commonly end with more than one newline.
	stream := "<< /Length 8 >>\nstream\n\x00\xff%PDF-\\\nendstream"
	data := append(minimalPDF([]string{stream}, ""), '\n', '\r', '\n')
	if err := ValidatePurchaseOrderPDF(data); err != nil {
		t.Fatalf("binary PDF refused: %v", err)
	}
}

func TestValidatePurchaseOrderPDFRejectsGarbage(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":       {},
		"plain text":  []byte("this is not a PDF at all"),
		"header only": []byte("%PDF-1.7\n"),
		"no trailer":  []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\nstartxref\n9\n%%EOF\n"),
		"no root":     []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Size 2 >>\nstartxref\n9\n%%EOF\n"),
		"no eof":      []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog /Root 1 0 R >>\nendobj\ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n9\n"),
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidatePurchaseOrderPDF(data)
			if !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
				t.Fatalf("error = %v; want ErrPurchaseOrderDocumentInvalid", err)
			}
		})
	}
}

func TestValidatePurchaseOrderPDFRejectsEncryptedPDF(t *testing.T) {
	encrypted := minimalPDF(nil, "/Encrypt 9 0 R")
	err := ValidatePurchaseOrderPDF(encrypted)
	if !errors.Is(err, ErrPurchaseOrderDocumentEncrypted) {
		t.Fatalf("error = %v; want ErrPurchaseOrderDocumentEncrypted", err)
	}
	// The message is what staff read, so it must ask for an unlocked copy
	// rather than leaving them guessing.
	if !strings.Contains(err.Error(), "unlocked") {
		t.Errorf("message %q does not ask for an unlocked copy", err.Error())
	}
}

// xrefStreamPDF builds a document whose cross-reference section is an object
// with /Type/XRef, written in the compact form MuPDF and PyMuPDF emit.
func xrefStreamPDF(root int, dictExtra string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.7\n")
	buffer.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	buffer.WriteString("2 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n")
	xrefOffset := buffer.Len()
	fmt.Fprintf(&buffer, "8 0 obj\n<< /Type/XRef /Size 9 /W [1 2 1] /Root %d 0 R %s/Length 0 >>\nstream\n\nendstream\nendobj\n", root, dictExtra)
	fmt.Fprintf(&buffer, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return buffer.Bytes()
}

func TestValidatePurchaseOrderPDFRejectsEncryptedXrefStreamPDF(t *testing.T) {
	// Modern PDFs use a cross-reference stream instead of a trailer keyword.
	// Its dictionary carries /Root and /Encrypt; refusing it is the same rule.
	if err := ValidatePurchaseOrderPDF(xrefStreamPDF(1, "/Encrypt 8 0 R ")); !errors.Is(err, ErrPurchaseOrderDocumentEncrypted) {
		t.Fatalf("error = %v; want ErrPurchaseOrderDocumentEncrypted", err)
	}
}

func TestValidatePurchaseOrderPDFAcceptsCompactXrefStream(t *testing.T) {
	// /Type/XRef with no spaces is the same name pair as /Type /XRef.
	if err := ValidatePurchaseOrderPDF(xrefStreamPDF(1, "")); err != nil {
		t.Fatalf("valid xref-stream PDF refused: %v", err)
	}
}

func TestValidatePurchaseOrderPDFIgnoresEncryptInsideContent(t *testing.T) {
	// The Judge's reproduction: a content stream draws the text of a trailer
	// dictionary, including /Encrypt. Those bytes are page content, not the
	// document trailer, and a scanner that reads them refuses a valid file.
	literal := "trailer << /Root 999 0 R /Encrypt 9 0 R >>"
	content := fmt.Sprintf("<< /Length %d >>\nstream\nBT (%s) Tj ET\nendstream", len(literal), literal)
	if err := ValidatePurchaseOrderPDF(minimalPDF([]string{content}, "")); err != nil {
		t.Fatalf("valid PDF with trailer text in its content refused: %v", err)
	}
}

// TestValidatePurchaseOrderPDFIgnoresStructureTokensInsideContent pins that a
// stream body is bytes, not structure: it contains a whole object header that
// would overwrite the real catalog if the scanner read it, plus a trailer
// dictionary with /Encrypt. Skipping the body by /Length is what keeps the
// document valid.
func TestValidatePurchaseOrderPDFIgnoresStructureTokensInsideContent(t *testing.T) {
	content := "1 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n" +
		"trailer\n<< /Root 999 0 R /Encrypt 9 0 R >>"
	stream := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
	if err := ValidatePurchaseOrderPDF(minimalPDF([]string{stream}, "")); err != nil {
		t.Fatalf("valid PDF with structure tokens inside a stream refused: %v", err)
	}
}

func TestValidatePurchaseOrderPDFIgnoresEncryptTextInContent(t *testing.T) {
	content := "<< /Length 18 >>\nstream\nBT (/Encrypt) Tj ET\nendstream"
	if err := ValidatePurchaseOrderPDF(minimalPDF([]string{content}, "")); err != nil {
		t.Fatalf("valid PDF with /Encrypt text refused: %v", err)
	}
}

// TestValidatePurchaseOrderPDFRejectsForgedStructure pins the Judge's forged
// file: the tokens look like a trailer, but startxref is not a number, there is
// no xref section, and /Root names an object that does not exist.
func TestValidatePurchaseOrderPDFRejectsForgedStructure(t *testing.T) {
	forged := []byte("%PDF-1.7\n1 0 obj\nnot-an-object endobj\ntrailer << /Root 999 0 R >>\nstartxref\nbanana\n%%EOF\n")
	if err := ValidatePurchaseOrderPDF(forged); !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
		t.Fatalf("forged file error = %v; want ErrPurchaseOrderDocumentInvalid", err)
	}
}

// TestValidatePurchaseOrderPDFRejectsAFakeRoot pins that /Root must name a real
// catalog object, not merely a number that appears in the file.
func TestValidatePurchaseOrderPDFRejectsAFakeRoot(t *testing.T) {
	for name, testCase := range map[string]struct {
		objects []string
		root    string
	}{
		"missing object": {
			objects: []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [] /Count 0 >>"},
			root:    "3 0 R",
		},
		"not a catalog": {
			objects: []string{"<< /Type /Pages /Kids [] /Count 0 >>", "<< /Type /Pages /Kids [] /Count 0 >>"},
			root:    "1 0 R",
		},
		"not a dict": {
			objects: []string{"42", "<< /Type /Pages /Kids [] /Count 0 >>"},
			root:    "1 0 R",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var buffer bytes.Buffer
			buffer.WriteString("%PDF-1.7\n")
			offsets := make([]int, 0, len(testCase.objects))
			for index, object := range testCase.objects {
				offsets = append(offsets, buffer.Len())
				fmt.Fprintf(&buffer, "%d 0 obj\n%s\nendobj\n", index+1, object)
			}
			xrefOffset := buffer.Len()
			fmt.Fprintf(&buffer, "xref\n0 %d\n", len(testCase.objects)+1)
			buffer.WriteString("0000000000 65535 f \n")
			for _, offset := range offsets {
				fmt.Fprintf(&buffer, "%010d 00000 n \n", offset)
			}
			fmt.Fprintf(&buffer, "trailer\n<< /Size %d /Root %s >>\nstartxref\n%d\n%%%%EOF\n",
				len(testCase.objects)+1, testCase.root, xrefOffset)
			if err := ValidatePurchaseOrderPDF(buffer.Bytes()); !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
				t.Fatalf("error = %v; want ErrPurchaseOrderDocumentInvalid", err)
			}
		})
	}
}

// TestValidatePurchaseOrderPDFAcceptsACatalogInsideAnObjectStream pins the
// documented policy for compressed catalogs: the catalog may live in an
// /ObjStm only when that object stream's own index names it. A catalog that is
// simply absent stays rejected.
func TestValidatePurchaseOrderPDFAcceptsACatalogInsideAnObjectStream(t *testing.T) {
	data := compressedCatalogPDF(false)
	if err := ValidatePurchaseOrderPDF(data); err != nil {
		t.Fatalf("catalog inside an object stream refused: %v", err)
	}
	missing := compressedCatalogPDF(true)
	if err := ValidatePurchaseOrderPDF(missing); !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
		t.Fatalf("missing catalog error = %v; want ErrPurchaseOrderDocumentInvalid", err)
	}
}

func compressedCatalogPDF(omitHeader bool) []byte {
	body := "6 0\n<</Type /Catalog>>"
	if omitHeader {
		body = "<</Type /Catalog>>"
	}
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.7\n")
	fmt.Fprintf(&buffer, "1 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length %d >>\nstream\n%s\nendstream\nendobj\n",
		len(body), body)
	xrefOffset := buffer.Len()
	fmt.Fprintf(&buffer, "8 0 obj\n<< /Type/XRef /Size 9 /W [1 2 1] /Root 6 0 R /Length 0 >>\nstream\n\nendstream\nendobj\n")
	fmt.Fprintf(&buffer, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return buffer.Bytes()
}

func TestValidatePurchaseOrderPDFRejectsATruncatedXrefStream(t *testing.T) {
	// The last startxref must be a number inside the file.
	data := xrefStreamPDF(1, "")
	index := bytes.LastIndex(data, []byte("startxref"))
	truncated := append([]byte{}, data[:index]...)
	truncated = append(truncated, []byte("startxref\n999999\n%%EOF\n")...)
	if err := ValidatePurchaseOrderPDF(truncated); !errors.Is(err, ErrPurchaseOrderDocumentInvalid) {
		t.Fatalf("error = %v; want ErrPurchaseOrderDocumentInvalid", err)
	}
}

func TestValidatePurchaseOrderPDFRejectsEncryptedFixturesFromTheJudge(t *testing.T) {
	for _, name := range []string{"real-encrypted.pdf", "real-escaped-encrypted.pdf"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../testdata/pdf/" + name)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if err := ValidatePurchaseOrderPDF(data); !errors.Is(err, ErrPurchaseOrderDocumentEncrypted) {
				t.Fatalf("error = %v; want ErrPurchaseOrderDocumentEncrypted", err)
			}
		})
	}
}

func TestValidatePurchaseOrderPDFAcceptsTheJudgeValidLiteralFixture(t *testing.T) {
	data, err := os.ReadFile("../testdata/pdf/real-valid-literal.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := ValidatePurchaseOrderPDF(data); err != nil {
		t.Fatalf("real MuPDF PDF refused: %v", err)
	}
}

func TestValidatePurchaseOrderPDFEnforcesTheByteBound(t *testing.T) {
	oversized := make([]byte, MaxPurchaseOrderDocumentBytes+1)
	copy(oversized, "%PDF-1.7\n")
	copy(oversized[len(oversized)-16:], "\nstartxref\n9\n%%EOF\n")
	err := ValidatePurchaseOrderPDF(oversized)
	if !errors.Is(err, ErrPurchaseOrderDocumentTooLarge) {
		t.Fatalf("error = %v; want ErrPurchaseOrderDocumentTooLarge", err)
	}
	if !strings.Contains(err.Error(), "20 MB") {
		t.Errorf("message %q does not name the bound", err.Error())
	}
}

func TestValidatePurchaseOrderPDFAcceptsExactlyTheByteBound(t *testing.T) {
	// The boundary belongs to the accepted side: a file of exactly the limit
	// has not exceeded it. Padding sits between the xref table and the trailer.
	base := minimalPDF(nil, "")
	trailerStart := bytes.Index(base, []byte("trailer"))
	exact := make([]byte, 0, int(MaxPurchaseOrderDocumentBytes))
	exact = append(exact, base[:trailerStart]...)
	exact = append(exact, bytes.Repeat([]byte(" "), int(MaxPurchaseOrderDocumentBytes)-len(base))...)
	exact = append(exact, base[trailerStart:]...)
	if len(exact) != int(MaxPurchaseOrderDocumentBytes) {
		t.Fatalf("exact fixture is %d bytes; want %d", len(exact), MaxPurchaseOrderDocumentBytes)
	}
	if err := ValidatePurchaseOrderPDF(exact); err != nil {
		t.Fatalf("PDF at exactly the bound refused: %v", err)
	}
}
