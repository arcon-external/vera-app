package store

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Issue #472: a customer PO PDF is stored only when it is a real, unlocked PDF.
// These tests pin the validation contract: the header is not enough, the
// document must carry a trailer/root and an end marker, an encrypted file is
// refused with its own error, and the byte bound is enforced before anything
// reaches Postgres.

// minimalPDF builds a structurally plausible classic PDF. The xref offsets are
// not consulted by the validator (which reads structure, not layout), but the
// header, objects, trailer, startxref and EOF are the tokens a real reader
// needs.
func minimalPDF(extraObjects []string, trailerExtra string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.7\n")
	buffer.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	buffer.WriteString("2 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n")
	for _, object := range extraObjects {
		buffer.WriteString(object)
	}
	buffer.WriteString("trailer\n<< /Size 9 /Root 1 0 R")
	if trailerExtra != "" {
		buffer.WriteString(" ")
		buffer.WriteString(trailerExtra)
	}
	buffer.WriteString(" >>\nstartxref\n123\n%%EOF\n")
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
	stream := "3 0 obj\n<< /Length 8 >>\nstream\n\x00\xff%PDF-\\\nendstream\nendobj\n"
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

func TestValidatePurchaseOrderPDFRejectsEncryptedXrefStreamPDF(t *testing.T) {
	// Modern PDFs use a cross-reference stream instead of a trailer keyword.
	// Its dictionary carries /Root and /Encrypt; refusing it is the same rule.
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.7\n")
	buffer.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	buffer.WriteString("2 0 obj\n<< /Type /Pages /Kids [] /Count 0 >>\nendobj\n")
	buffer.WriteString("9 0 obj\n<< /Type /XRef /Size 10 /Root 1 0 R /Encrypt 8 0 R /W [1 2 1] /Length 0 >>\nstream\n\nendstream\nendobj\n")
	buffer.WriteString("startxref\n200\n%%EOF\n")
	if err := ValidatePurchaseOrderPDF(buffer.Bytes()); !errors.Is(err, ErrPurchaseOrderDocumentEncrypted) {
		t.Fatalf("error = %v; want ErrPurchaseOrderDocumentEncrypted", err)
	}
}

func TestValidatePurchaseOrderPDFIgnoresEncryptInsideContent(t *testing.T) {
	// /Encrypt in a content stream is text, not a trailer key. A string scan
	// over the whole file would refuse this valid document.
	content := "3 0 obj\n<< /Length 18 >>\nstream\nBT (/Encrypt) Tj ET\nendstream\nendobj\n"
	if err := ValidatePurchaseOrderPDF(minimalPDF([]string{content}, "")); err != nil {
		t.Fatalf("valid PDF with /Encrypt text refused: %v", err)
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
	// has not exceeded it. Padding sits between the header and the trailer.
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
