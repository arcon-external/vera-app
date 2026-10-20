package store

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// Issue #472: one optional customer PO PDF.
//
// The document is evidence, not a second PO: its bytes are stored exactly as
// uploaded, replacing it never touches a purchase order field, and every
// earlier version stays addressable. Validation is deliberately structural
// rather than extension-based, because a staff member choosing a file is not
// evidence that the file is a PDF.
//
// The upload bound is a product decision the parent specification left to
// technical design. 20 MiB is far above a real PO document (typically under
// 2 MiB) and small enough that one request cannot exhaust the API process. It
// is enforced both before reading the request body and again on the decoded
// bytes, so neither a lying Content-Length nor a multipart envelope can slip
// past it.
const MaxPurchaseOrderDocumentBytes int64 = 20 << 20

var (
	// ErrPurchaseOrderDocumentInvalid refuses a file that is not a PDF at all,
	// or one whose structure is too broken to be a document.
	ErrPurchaseOrderDocumentInvalid = errors.New("the customer PO document is not a valid PDF")
	// ErrPurchaseOrderDocumentEncrypted is separate from Invalid because the
	// remedy is different: the staff member needs to supply an unlocked copy.
	// Vera never asks for, transports or stores a PDF password.
	ErrPurchaseOrderDocumentEncrypted = errors.New("the customer PO PDF is password-protected; an unlocked copy is required")
	// ErrPurchaseOrderDocumentTooLarge names the bound in the message staff
	// read, so the remedy is obvious.
	ErrPurchaseOrderDocumentTooLarge = fmt.Errorf("the customer PO PDF is larger than 20 MB")
	// ErrPurchaseOrderDocumentNotFound answers both "no active document" and
	// "another organization's document": saying which would confirm that an id
	// exists outside the caller's boundary.
	ErrPurchaseOrderDocumentNotFound = errors.New("customer PO document not found")
	// ErrPurchaseOrderDocumentRemovalNotConfirmed is the deliberate-removal
	// rule. The caller must repeat the intent explicitly; the API and the
	// editor both require it.
	ErrPurchaseOrderDocumentRemovalNotConfirmed = errors.New("confirm removing the customer PO PDF")
)

// ValidatePurchaseOrderPDF checks the bytes a caller wants to store as the
// customer PO document. It checks, in order:
//
//  1. the byte bound, before any parsing work;
//  2. the PDF header at the first byte;
//  3. the end-of-file marker, reached through trailing whitespace;
//  4. the object skeleton: every stream body is skipped using /Length when it
//     is a direct integer or an indirect reference. A first pass harvests the
//     top-level integer objects, so a forward reference is resolved before the
//     structure pass. Content is isolated exactly when that length is
//     resolvable and agrees with the file; otherwise the scanner falls back to
//     the first endstream marker;
//  5. the last startxref, which must be an integer offset in the file and must
//     point at a classic xref table or a /Type /XRef object;
//  6. the /Root reference in the real trailer or xref-stream dictionary — never
//     from a scanned substring — which must name a /Type /Catalog object that
//     exists in the file;
//  7. encryption, read from that same dictionary only, so a literal /Encrypt
//     inside page content is not mistaken for an encrypted document.
//
// The structure checks live in pdf_structure.go. The accepted object-stream
// policy is documented in docs/po-documents.md: a catalog that is not a plain
// object is accepted only when an /ObjStm header lists it. A /FlateDecode (or
// /Fl, or single-element [/FlateDecode]) header is decompressed to read that
// index; any other filter is a named rejection, and the catalog dictionary
// itself is never decompressed.
func ValidatePurchaseOrderPDF(data []byte) error {
	if int64(len(data)) > MaxPurchaseOrderDocumentBytes {
		return ErrPurchaseOrderDocumentTooLarge
	}
	if len(data) == 0 {
		return fmt.Errorf("%w: the file is empty", ErrPurchaseOrderDocumentInvalid)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) < 8 ||
		data[5] < '0' || data[5] > '9' || data[6] != '.' ||
		data[7] < '0' || data[7] > '9' {
		return fmt.Errorf("%w: it does not start with a PDF header", ErrPurchaseOrderDocumentInvalid)
	}
	if !pdfEndsWithEOF(data) {
		return fmt.Errorf("%w: it has no end-of-file marker", ErrPurchaseOrderDocumentInvalid)
	}
	encrypted, err := inspectPDFStructure(data)
	if err != nil {
		return err
	}
	if encrypted {
		return ErrPurchaseOrderDocumentEncrypted
	}
	return nil
}

// pdfEndsWithEOF trims the whitespace PDF permits after %%EOF. NUL is included
// because some writers pad the final block with zero bytes.
func pdfEndsWithEOF(data []byte) bool {
	trimmed := bytes.TrimRight(data, " \t\r\n\f\v\x00")
	return bytes.HasSuffix(trimmed, []byte("%%EOF"))
}

// ParsePurchaseOrderDocumentVersion is the small parser the API uses for a
// version path segment. It lives beside the validator so version 0 can never
// silently mean "active" in one layer and "missing" in another.
func ParsePurchaseOrderDocumentVersion(raw string) (int32, error) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%w: document version must be a positive number", ErrPurchaseOrderDocumentInvalid)
	}
	return int32(value), nil
}
