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
//  3. at least one object header and its end marker;
//  4. a cross-reference pointer, a trailer or xref-stream dictionary, and a
//     document catalog (/Root), so a header-and-EOF forgery is refused;
//  5. the end-of-file marker, reached through trailing whitespace;
//  6. encryption, read from the trailer or xref-stream dictionary only, so a
//     literal /Encrypt inside page content does not reject a valid file.
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
	if !pdfHasObjectHeader(data) || !bytes.Contains(data, []byte("endobj")) {
		return fmt.Errorf("%w: it contains no PDF objects", ErrPurchaseOrderDocumentInvalid)
	}

	tailStart := 0
	if len(data) > 64<<10 {
		tailStart = len(data) - 64<<10
	}
	tail := data[tailStart:]
	if !bytes.Contains(tail, []byte("/Root")) {
		return fmt.Errorf("%w: it has no document catalog", ErrPurchaseOrderDocumentInvalid)
	}
	if !bytes.Contains(tail, []byte("startxref")) {
		return fmt.Errorf("%w: it has no cross-reference pointer", ErrPurchaseOrderDocumentInvalid)
	}
	if !pdfHasTrailerOrXrefStream(tail) {
		return fmt.Errorf("%w: it has no trailer", ErrPurchaseOrderDocumentInvalid)
	}
	if !pdfEndsWithEOF(data) {
		return fmt.Errorf("%w: it has no end-of-file marker", ErrPurchaseOrderDocumentInvalid)
	}
	if pdfHasEncryptionDictionary(tail) {
		return ErrPurchaseOrderDocumentEncrypted
	}
	return nil
}

// pdfHasObjectHeader looks for the "N G obj" shape at a token boundary,
// rather than the bare substring "obj" that any text could contain.
func pdfHasObjectHeader(data []byte) bool {
	search := 0
	for {
		index := bytes.Index(data[search:], []byte(" obj"))
		if index < 0 {
			return false
		}
		position := search + index
		cursor := position - 1
		for cursor >= 0 && (data[cursor] == ' ' || data[cursor] == '\t') {
			cursor--
		}
		generationEnd := cursor
		for cursor >= 0 && data[cursor] >= '0' && data[cursor] <= '9' {
			cursor--
		}
		if cursor == generationEnd {
			search = position + 4
			continue
		}
		if cursor < 0 || (data[cursor] != ' ' && data[cursor] != '\t' && data[cursor] != '\n' && data[cursor] != '\r') {
			search = position + 4
			continue
		}
		cursor--
		objectEnd := cursor
		for cursor >= 0 && data[cursor] >= '0' && data[cursor] <= '9' {
			cursor--
		}
		if cursor < objectEnd {
			return true
		}
		search = position + 4
	}
}

// pdfEndsWithEOF trims the whitespace PDF permits after %%EOF. NUL is included
// because some writers pad the final block with zero bytes.
func pdfEndsWithEOF(data []byte) bool {
	trimmed := bytes.TrimRight(data, " \t\r\n\f\v\x00")
	return bytes.HasSuffix(trimmed, []byte("%%EOF"))
}

// pdfHasTrailerOrXrefStream reports whether the tail carries either the
// classic trailer dictionary or a cross-reference stream dictionary.
func pdfHasTrailerOrXrefStream(tail []byte) bool {
	search := 0
	for {
		index := bytes.Index(tail[search:], []byte("trailer"))
		if index < 0 {
			break
		}
		position := search + index
		if _, _, ok := pdfDictionaryAfter(tail, position+len("trailer")); ok {
			return true
		}
		search = position + len("trailer")
	}
	return pdfHasDictionaryNamed(tail, "Type", "XRef")
}

// pdfHasEncryptionDictionary looks only at dictionary objects in the tail. A
// trailer or xref stream must carry /Encrypt for an encrypted document; a page
// content stream that happens to draw the word does not.
func pdfHasEncryptionDictionary(tail []byte) bool {
	search := 0
	for {
		index := bytes.Index(tail[search:], []byte("trailer"))
		if index < 0 {
			break
		}
		position := search + index
		if start, end, ok := pdfDictionaryAfter(tail, position+len("trailer")); ok {
			if pdfDictionaryHasName(tail[start:end], "Encrypt") {
				return true
			}
		}
		search = position + len("trailer")
	}
	return pdfHasDictionaryNamed(tail, "Type", "XRef", "Encrypt")
}

// pdfHasDictionaryNamed walks every dictionary in the tail and reports whether
// one carries all the named keys. It is used for the two xref-stream shapes:
// /Type /XRef alone, and /Type /XRef with /Encrypt.
func pdfHasDictionaryNamed(tail []byte, names ...string) bool {
	search := 0
	for {
		index := bytes.Index(tail[search:], []byte("<<"))
		if index < 0 {
			return false
		}
		position := search + index
		start, end, ok := pdfDictionaryAfter(tail, position)
		if !ok {
			search = position + 2
			continue
		}
		dictionary := tail[start:end]
		matches := true
		for _, name := range names {
			if !pdfDictionaryHasName(dictionary, name) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
		search = position + 2
	}
}

// pdfDictionaryAfter skips whitespace after a keyword and parses a balanced
// dictionary. Literal strings and comments are skipped so their contents
// cannot unbalance the nesting.
func pdfDictionaryAfter(data []byte, from int) (start, end int, ok bool) {
	cursor := from
	for cursor < len(data) && pdfWhitespace(data[cursor]) {
		cursor++
	}
	if cursor+1 >= len(data) || data[cursor] != '<' || data[cursor+1] != '<' {
		return 0, 0, false
	}
	start = cursor
	depth := 0
	for cursor < len(data) {
		switch {
		case data[cursor] == '(':
			cursor = pdfSkipString(data, cursor+1)
			continue
		case data[cursor] == '%':
			for cursor < len(data) && data[cursor] != '\n' && data[cursor] != '\r' {
				cursor++
			}
			continue
		case data[cursor] == '<' && cursor+1 < len(data) && data[cursor+1] == '<':
			depth++
			cursor += 2
			continue
		case data[cursor] == '>' && cursor+1 < len(data) && data[cursor+1] == '>':
			depth--
			cursor += 2
			if depth == 0 {
				return start, cursor, true
			}
			continue
		default:
			cursor++
		}
	}
	return 0, 0, false
}

func pdfSkipString(data []byte, from int) int {
	cursor := from
	depth := 1
	for cursor < len(data) {
		switch data[cursor] {
		case '\\':
			cursor += 2
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return cursor + 1
			}
		}
		cursor++
	}
	return cursor
}

func pdfWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n' || value == '\f' || value == '\v' || value == 0
}

// pdfDictionaryHasName reports whether /Name appears as a dictionary key or a
// value reference. The preceding byte must be whitespace or the opening of a
// dictionary/array, which excludes a longer name like /NotEncrypt and text
// inside a literal string.
func pdfDictionaryHasName(dictionary []byte, name string) bool {
	token := []byte("/" + name)
	search := 0
	for {
		index := bytes.Index(dictionary[search:], token)
		if index < 0 {
			return false
		}
		position := search + index
		before := byte(0)
		if position > 0 {
			before = dictionary[position-1]
		}
		after := byte(0)
		if position+len(token) < len(dictionary) {
			after = dictionary[position+len(token)]
		}
		preceded := position == 0 || pdfWhitespace(before) || before == '<' || before == '[' || before == '>'
		followed := position+len(token) == len(dictionary) || pdfWhitespace(after) ||
			after == '/' || after == '<' || after == '[' || after == '>' || after == '%'
		if preceded && followed {
			return true
		}
		search = position + len(token)
	}
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
