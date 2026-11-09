package store

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Issue #472: PDF structure inspection for the customer PO document.
//
// The job is deliberately narrow: decide whether the file is a structurally
// real PDF, and whether it is encrypted. It is not a conforming PDF parser and
// does not render anything. It tokenizes objects and dictionaries, skips stream
// bodies so page content can never be mistaken for document structure, follows
// the last startxref to the cross-reference section, and requires the /Root
// reference in the real trailer (or xref stream) to name a real catalog object.
//
// Stream isolation is exact when a stream's /Length is a direct integer or an
// indirect reference: top-level integer objects are harvested in a first pass,
// so a /Length defined later in the file (real writers emit forward references)
// is resolved before the structure pass. A stream whose /Length cannot be resolved
// at all falls back to the first endstream marker, so content after that point
// is not isolated — but the trailer and the /Root it names are still read only
// through the xref section startxref points at.
//
// What it deliberately does not check: incremental updates (/Prev chains are
// not followed; a latest trailer without /Root is refused), xref entries
// pointing at the objects they claim (a scan-based object index is used
// instead), the compressed catalog dictionary itself, page trees, fonts, or
// whether a reader would render the file. Those are rendering concerns; this
// check answers "is this a document, or a bag of tokens that looks like one".

// pdfMaxParseDepth bounds dictionary and array nesting so a hostile file cannot
// exhaust the stack.
const pdfMaxParseDepth = 32

// pdfMaxObjectStreamHeaderBytes bounds the /ObjStm header this check will
// decompress. A header holds object-number/value index pairs only, so a
// megabyte is already generous; the cap keeps a decompression bomb cheap.
const pdfMaxObjectStreamHeaderBytes int64 = 1 << 20

// errPDFMalformed is an internal reason; the public error stays
// ErrPurchaseOrderDocumentInvalid so callers keep one sentinel.
var errPDFMalformed = errors.New("malformed PDF structure")

type pdfTokenKind uint8

const (
	pdfTokenEOF pdfTokenKind = iota
	pdfTokenNumber
	pdfTokenName
	pdfTokenKeyword
	pdfTokenString
	pdfTokenHexString
	pdfTokenDictStart
	pdfTokenDictEnd
	pdfTokenArrayStart
	pdfTokenArrayEnd
)

type pdfToken struct {
	text  string
	start int
	end   int
	kind  pdfTokenKind
	value int64
	// isFloat marks a number that is not an integer. References, lengths and
	// object numbers must be integers; a float is only ever skipped.
	isFloat bool
}

// pdfLexer walks PDF tokens outside stream bodies. The caller is responsible
// for moving pos past a stream body before asking for the next token.
type pdfLexer struct {
	data []byte
	pos  int
}

func newPDFLexer(data []byte) *pdfLexer {
	return &pdfLexer{data: data}
}

func (l *pdfLexer) next() pdfToken {
	l.skipSpace()
	if l.pos >= len(l.data) {
		return pdfToken{kind: pdfTokenEOF, start: l.pos, end: l.pos}
	}
	start := l.pos
	switch character := l.data[l.pos]; {
	case character == '<' && l.pos+1 < len(l.data) && l.data[l.pos+1] == '<':
		l.pos += 2
		return pdfToken{kind: pdfTokenDictStart, text: "<<", start: start, end: l.pos}
	case character == '>' && l.pos+1 < len(l.data) && l.data[l.pos+1] == '>':
		l.pos += 2
		return pdfToken{kind: pdfTokenDictEnd, text: ">>", start: start, end: l.pos}
	case character == '<':
		l.pos++
		for l.pos < len(l.data) && l.data[l.pos] != '>' {
			l.pos++
		}
		if l.pos >= len(l.data) {
			return pdfToken{kind: pdfTokenEOF, start: start, end: l.pos}
		}
		l.pos++
		return pdfToken{kind: pdfTokenHexString, text: "hex", start: start, end: l.pos}
	case character == '(':
		l.pos = pdfSkipLiteralString(l.data, l.pos)
		return pdfToken{kind: pdfTokenString, text: "string", start: start, end: l.pos}
	case character == '/':
		l.pos++
		nameStart := l.pos
		for l.pos < len(l.data) && !pdfDelimiter(l.data[l.pos]) {
			l.pos++
		}
		return pdfToken{kind: pdfTokenName, text: pdfDecodeName(l.data[nameStart:l.pos]), start: start, end: l.pos}
	case character == '[':
		l.pos++
		return pdfToken{kind: pdfTokenArrayStart, text: "[", start: start, end: l.pos}
	case character == ']':
		l.pos++
		return pdfToken{kind: pdfTokenArrayEnd, text: "]", start: start, end: l.pos}
	case character == '+' || character == '-' || character == '.' || (character >= '0' && character <= '9'):
		return l.number()
	default:
		keywordStart := l.pos
		for l.pos < len(l.data) && !pdfDelimiter(l.data[l.pos]) {
			l.pos++
		}
		if l.pos == keywordStart {
			// A lone delimiter this parser does not use (')', '>', '{', '}').
			l.pos++
		}
		return pdfToken{kind: pdfTokenKeyword, text: string(l.data[keywordStart:l.pos]), start: keywordStart, end: l.pos}
	}
}

func (l *pdfLexer) number() pdfToken {
	start := l.pos
	for l.pos < len(l.data) && !pdfDelimiter(l.data[l.pos]) {
		l.pos++
	}
	raw := string(l.data[start:l.pos])
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return pdfToken{kind: pdfTokenNumber, text: raw, value: value, start: start, end: l.pos}
	}
	return pdfToken{kind: pdfTokenNumber, text: raw, isFloat: true, start: start, end: l.pos}
}

func (l *pdfLexer) skipSpace() {
	for l.pos < len(l.data) {
		switch l.data[l.pos] {
		case ' ', '\t', '\r', '\n', '\f', '\v', 0:
			l.pos++
		case '%':
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
		default:
			return
		}
	}
}

func pdfDelimiter(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '\f', '\v', 0,
		'(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func pdfWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '\f', '\v', 0:
		return true
	}
	return false
}

// pdfSkipLiteralString skips a balanced literal string, honouring backslash
// escapes and nested parentheses.
func pdfSkipLiteralString(data []byte, from int) int {
	cursor := from + 1
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

// pdfDecodeName expands #xx escapes, so /En#63rypt is the key Encrypt. A malformed
// escape is kept literally rather than guessed at.
func pdfDecodeName(raw []byte) string {
	if !bytes.Contains(raw, []byte("#")) {
		return string(raw)
	}
	decoded := make([]byte, 0, len(raw))
	for index := 0; index < len(raw); index++ {
		if raw[index] == '#' && index+2 < len(raw) {
			value, err := strconv.ParseUint(string(raw[index+1:index+3]), 16, 8)
			if err == nil {
				decoded = append(decoded, byte(value))
				index += 2
				continue
			}
		}
		decoded = append(decoded, raw[index])
	}
	return string(decoded)
}

type pdfValueKind uint8

const (
	pdfValueNull pdfValueKind = iota
	pdfValueNumber
	pdfValueName
	pdfValueString
	pdfValueKeyword
	pdfValueArray
	pdfValueDict
	pdfValueRef
)

type pdfValue struct {
	entries    map[string]*pdfValue
	items      []*pdfValue
	kind       pdfValueKind
	name       string
	keyword    string
	value      int64
	number     int
	generation int
	isInteger  bool
}

// parsePDFValue consumes one complete value, beginning with the token already
// read. It is the only place dictionary and array nesting is interpreted.
func parsePDFValue(l *pdfLexer, token pdfToken, depth int) (*pdfValue, error) {
	if depth > pdfMaxParseDepth {
		return nil, fmt.Errorf("%w: nesting is too deep", errPDFMalformed)
	}
	switch token.kind {
	case pdfTokenNumber:
		if token.isFloat {
			return &pdfValue{kind: pdfValueNumber}, nil
		}
		// "N G R" is an indirect reference; "N G" alone is two numbers.
		afterNumber := l.pos
		generation := l.next()
		if generation.kind == pdfTokenNumber && !generation.isFloat {
			afterGeneration := l.pos
			marker := l.next()
			if marker.kind == pdfTokenKeyword && marker.text == "R" {
				return &pdfValue{
					kind:       pdfValueRef,
					number:     int(token.value),
					generation: int(generation.value),
				}, nil
			}
			l.pos = afterGeneration
		}
		l.pos = afterNumber
		return &pdfValue{kind: pdfValueNumber, value: token.value, isInteger: true}, nil
	case pdfTokenName:
		return &pdfValue{kind: pdfValueName, name: token.text}, nil
	case pdfTokenString, pdfTokenHexString:
		return &pdfValue{kind: pdfValueString}, nil
	case pdfTokenKeyword:
		return &pdfValue{kind: pdfValueKeyword, keyword: token.text}, nil
	case pdfTokenDictStart:
		dictionary := &pdfValue{kind: pdfValueDict, entries: map[string]*pdfValue{}}
		for {
			key := l.next()
			if key.kind == pdfTokenDictEnd {
				return dictionary, nil
			}
			if key.kind == pdfTokenEOF {
				return nil, fmt.Errorf("%w: a dictionary is unterminated", errPDFMalformed)
			}
			if key.kind != pdfTokenName {
				return nil, fmt.Errorf("%w: a dictionary key is not a name", errPDFMalformed)
			}
			valueToken := l.next()
			value, err := parsePDFValue(l, valueToken, depth+1)
			if err != nil {
				return nil, err
			}
			dictionary.entries[key.text] = value
		}
	case pdfTokenArrayStart:
		array := &pdfValue{kind: pdfValueArray}
		for {
			valueToken := l.next()
			if valueToken.kind == pdfTokenArrayEnd {
				return array, nil
			}
			value, err := parsePDFValue(l, valueToken, depth+1)
			if err != nil {
				return nil, err
			}
			array.items = append(array.items, value)
		}
	case pdfTokenEOF:
		return nil, fmt.Errorf("%w: a value is missing", errPDFMalformed)
	default:
		return nil, fmt.Errorf("%w: unexpected token", errPDFMalformed)
	}
}

type pdfObjectKey struct {
	number     int
	generation int
}

type pdfObject struct {
	dict        *pdfValue
	key         pdfObjectKey
	value       int64
	streamStart int
	streamEnd   int
	isInteger   bool
	hasStream   bool
}

// scanPDFObjects walks the file once, recording every plain object and the last
// startxref pointer. Stream bodies are skipped, so a stream can contain the
// literal text "trailer", "/Encrypt" or "obj" without affecting the result.
// lengths holds the integer objects collected before this pass; it is what lets
// a forward /Length reference be resolved while the stream is being skipped.
func scanPDFObjects(data []byte, lengths map[pdfObjectKey]int64) (map[pdfObjectKey]*pdfObject, int, bool, error) {
	l := newPDFLexer(data)
	objects := make(map[pdfObjectKey]*pdfObject)
	startXref := -1
	startXrefSeen := false
	for {
		token := l.next()
		if token.kind == pdfTokenEOF {
			return objects, startXref, startXrefSeen, nil
		}
		if token.kind == pdfTokenKeyword && token.text == "startxref" {
			startXrefSeen = true
			offset := l.next()
			startXref = -1
			if offset.kind == pdfTokenNumber && !offset.isFloat &&
				offset.value >= 0 && offset.value < int64(len(data)) {
				startXref = int(offset.value)
			}
			continue
		}
		if token.kind != pdfTokenNumber || token.isFloat {
			continue
		}
		afterNumber := l.pos
		generation := l.next()
		if generation.kind != pdfTokenNumber || generation.isFloat {
			l.pos = afterNumber
			continue
		}
		marker := l.next()
		if marker.kind != pdfTokenKeyword || marker.text != "obj" {
			l.pos = afterNumber
			continue
		}
		object, err := parsePDFObject(l, pdfObjectKey{number: int(token.value), generation: int(generation.value)}, objects, lengths)
		if err != nil {
			// The objects found so far are still useful to the harvest pass: a
			// fallback stream boundary may be the only thing that failed.
			return objects, startXref, startXrefSeen, err
		}
		if object != nil {
			objects[object.key] = object
		}
	}
}

// parsePDFObject reads an object body and, when present, skips its stream body.
// The "obj" keyword is already consumed.
func parsePDFObject(l *pdfLexer, key pdfObjectKey, objects map[pdfObjectKey]*pdfObject, lengths map[pdfObjectKey]int64) (*pdfObject, error) {
	bodyToken := l.next()
	value, err := parsePDFValue(l, bodyToken, 0)
	if err != nil {
		return nil, err
	}
	object := &pdfObject{key: key}
	switch value.kind {
	case pdfValueDict:
		object.dict = value
	case pdfValueNumber:
		if value.isInteger {
			object.isInteger = true
			object.value = value.value
		}
	}
	afterBody := l.pos
	next := l.next()
	if next.kind == pdfTokenKeyword && next.text == "stream" {
		object.hasStream = true
		start := pdfStreamDataStart(l.data, l.pos)
		end, ok := pdfStreamDataEnd(l.data, start, value, lengths)
		if !ok {
			return nil, fmt.Errorf("%w: a stream has no endstream marker", errPDFMalformed)
		}
		object.streamStart = start
		object.streamEnd = end
		l.pos = end
		marker := l.next()
		if marker.kind != pdfTokenKeyword || marker.text != "endstream" {
			return nil, fmt.Errorf("%w: a stream has no endstream marker", errPDFMalformed)
		}
		afterBody = l.pos
		next = l.next()
	}
	if next.kind == pdfTokenKeyword && next.text == "endobj" {
		return object, nil
	}
	// Put the token back: an object without its end marker is tolerated unless
	// it is a stream, because only the referenced catalog has to be intact for
	// this check to be honest.
	l.pos = afterBody
	return object, nil
}

// pdfStreamDataStart skips the end-of-line that follows the stream keyword.
func pdfStreamDataStart(data []byte, pos int) int {
	if pos < len(data) && data[pos] == '\r' {
		pos++
	}
	if pos < len(data) && data[pos] == '\n' {
		pos++
	}
	return pos
}

// pdfStreamDataEnd finds the endstream keyword. /Length is used when it is a
// direct integer or a reference to a harvested integer object, including one
// defined later in the file; otherwise the first endstream marker is taken.
// This is what keeps content bytes out of the structural scan.
func pdfStreamDataEnd(data []byte, start int, value *pdfValue, lengths map[pdfObjectKey]int64) (int, bool) {
	if length, ok := pdfResolvedLength(value, lengths); ok &&
		length >= 0 && int64(start)+length <= int64(len(data)) {
		cursor := start + int(length)
		for cursor < len(data) && pdfWhitespace(data[cursor]) {
			cursor++
		}
		if bytes.HasPrefix(data[cursor:], []byte("endstream")) {
			return cursor, true
		}
	}
	index := bytes.Index(data[start:], []byte("endstream"))
	if index < 0 {
		return 0, false
	}
	return start + index, true
}

// pdfIntegerEntry resolves a dictionary entry that must be an integer, either
// written inline or through a reference to an integer object already seen.
func pdfIntegerEntry(dictionary *pdfValue, objects map[pdfObjectKey]*pdfObject, name string) (int64, bool) {
	if dictionary == nil || dictionary.kind != pdfValueDict {
		return 0, false
	}
	entry := dictionary.entries[name]
	if entry == nil {
		return 0, false
	}
	switch entry.kind {
	case pdfValueNumber:
		if entry.isInteger {
			return entry.value, true
		}
	case pdfValueRef:
		object := objects[pdfObjectKey{number: entry.number, generation: entry.generation}]
		if object != nil && object.isInteger {
			return object.value, true
		}
	}
	return 0, false
}

// pdfResolvedLength reads a stream's /Length from the harvested integer objects.
// A forward reference is the reason this is separate from pdfIntegerEntry: the
// object map for the structure pass is incomplete while that pass is running.
func pdfResolvedLength(dictionary *pdfValue, lengths map[pdfObjectKey]int64) (int64, bool) {
	if dictionary == nil || dictionary.kind != pdfValueDict {
		return 0, false
	}
	entry := dictionary.entries["Length"]
	if entry == nil {
		return 0, false
	}
	switch entry.kind {
	case pdfValueNumber:
		if entry.isInteger {
			return entry.value, true
		}
	case pdfValueRef:
		value, ok := lengths[pdfObjectKey{number: entry.number, generation: entry.generation}]
		return value, ok
	}
	return 0, false
}

// harvestPDFIntegerObjects collects top-level integer objects so the structure
// pass can skip a stream whose /Length is an indirect reference defined later
// in the file. This pass itself may read a stream body as structure when that
// stream's length is not yet known; it only keeps integer object values, which
// is the one thing the structure pass needs to resolve those references.
func harvestPDFIntegerObjects(data []byte) map[pdfObjectKey]int64 {
	objects, _, _, _ := scanPDFObjects(data, nil)
	lengths := make(map[pdfObjectKey]int64)
	for key, object := range objects {
		if object.isInteger && !object.hasStream {
			lengths[key] = object.value
		}
	}
	return lengths
}

func pdfDictNameIs(dictionary *pdfValue, key, name string) bool {
	if dictionary == nil || dictionary.kind != pdfValueDict {
		return false
	}
	entry := dictionary.entries[key]
	return entry != nil && entry.kind == pdfValueName && entry.name == name
}

// inspectPDFStructure parses the document skeleton and reports whether the real
// trailer or xref-stream dictionary carries /Encrypt. A structural problem is
// returned as ErrPurchaseOrderDocumentInvalid with a readable reason.
func inspectPDFStructure(data []byte) (bool, error) {
	lengths := harvestPDFIntegerObjects(data)
	objects, startXref, startXrefSeen, err := scanPDFObjects(data, lengths)
	if err != nil {
		return false, pdfInvalid(err)
	}
	if !startXrefSeen {
		return false, pdfInvalid(errors.New("it has no cross-reference pointer"))
	}
	if startXref < 0 {
		return false, pdfInvalid(errors.New("its cross-reference pointer is not an integer offset inside the file"))
	}
	if len(objects) == 0 {
		return false, pdfInvalid(errors.New("it contains no PDF objects"))
	}
	return inspectPDFCrossReference(data, startXref, objects)
}

func pdfInvalid(reason error) error {
	return fmt.Errorf("%w: %v", ErrPurchaseOrderDocumentInvalid, reason)
}

// inspectPDFCrossReference follows startxref to either a classic xref table
// followed by a trailer, or an object whose dictionary is /Type /XRef.
func inspectPDFCrossReference(data []byte, offset int, objects map[pdfObjectKey]*pdfObject) (bool, error) {
	l := newPDFLexer(data)
	l.pos = offset
	token := l.next()
	if token.kind == pdfTokenKeyword && token.text == "xref" {
		if err := scanClassicXrefTable(l); err != nil {
			return false, pdfInvalid(err)
		}
		dictionaryToken := l.next()
		dictionary, err := parsePDFValue(l, dictionaryToken, 0)
		if err != nil || dictionary.kind != pdfValueDict {
			return false, pdfInvalid(errors.New("its trailer is not a dictionary"))
		}
		return inspectPDFTrailer(dictionary, objects, data)
	}
	dictionary, err := pdfDictionaryObjectAt(data, offset)
	if err != nil {
		return false, pdfInvalid(err)
	}
	if !pdfDictNameIs(dictionary, "Type", "XRef") {
		return false, pdfInvalid(errors.New("its startxref does not point at an xref table or a /Type /XRef object"))
	}
	return inspectPDFTrailer(dictionary, objects, data)
}

// scanClassicXrefTable consumes subsection headers and entries, leaving the
// lexer immediately after the trailer keyword.
func scanClassicXrefTable(l *pdfLexer) error {
	for {
		token := l.next()
		if token.kind == pdfTokenKeyword && token.text == "trailer" {
			return nil
		}
		if token.kind != pdfTokenNumber || token.isFloat {
			return errors.New("its xref table is malformed")
		}
		count := l.next()
		if count.kind != pdfTokenNumber || count.isFloat || count.value < 0 || count.value > int64(len(l.data)) {
			return errors.New("its xref table is malformed")
		}
		for entry := int64(0); entry < count.value; entry++ {
			offset := l.next()
			generation := l.next()
			kind := l.next()
			if offset.kind != pdfTokenNumber || offset.isFloat ||
				generation.kind != pdfTokenNumber || generation.isFloat ||
				kind.kind != pdfTokenKeyword || (kind.text != "n" && kind.text != "f") {
				return errors.New("its xref table is malformed")
			}
		}
	}
}

// pdfDictionaryObjectAt reads only the dictionary of the object at offset. The
// stream body is not needed for an xref stream's /Root and /Encrypt.
func pdfDictionaryObjectAt(data []byte, offset int) (*pdfValue, error) {
	if offset < 0 || offset >= len(data) {
		return nil, errors.New("its cross-reference offset is outside the file")
	}
	l := newPDFLexer(data)
	l.pos = offset
	number := l.next()
	generation := l.next()
	marker := l.next()
	if number.kind != pdfTokenNumber || number.isFloat ||
		generation.kind != pdfTokenNumber || generation.isFloat ||
		marker.kind != pdfTokenKeyword || marker.text != "obj" {
		return nil, errors.New("its cross-reference offset is not an object")
	}
	dictionaryToken := l.next()
	dictionary, err := parsePDFValue(l, dictionaryToken, 0)
	if err != nil {
		return nil, err
	}
	if dictionary.kind != pdfValueDict {
		return nil, errors.New("its cross-reference object is not a dictionary")
	}
	return dictionary, nil
}

// inspectPDFTrailer reads only the real trailer/xref-stream dictionary. It
// checks /Encrypt before /Root so an encrypted document gets the actionable
// message even when its catalog is itself encrypted.
func inspectPDFTrailer(dictionary *pdfValue, objects map[pdfObjectKey]*pdfObject, data []byte) (bool, error) {
	if _, encrypted := dictionary.entries["Encrypt"]; encrypted {
		return true, nil
	}
	root := dictionary.entries["Root"]
	if root == nil || root.kind != pdfValueRef {
		return false, pdfInvalid(errors.New("its trailer has no /Root document catalog"))
	}
	if err := requirePDFCatalogObject(objects, root.number, root.generation, data); err != nil {
		return false, pdfInvalid(err)
	}
	return false, nil
}

// requirePDFCatalogObject enforces that /Root names a real catalog. A catalog
// may legally live in an object stream; that is accepted only when the object
// stream's own header lists the object number (generation 0), so an absent
// object is never silently accepted. The header is decompressed for a
// /FlateDecode stream; the catalog dictionary inside it is not decompressed,
// and the docs say so.
func requirePDFCatalogObject(objects map[pdfObjectKey]*pdfObject, number, generation int, data []byte) error {
	if object := objects[pdfObjectKey{number: number, generation: generation}]; object != nil {
		if object.dict == nil {
			return errors.New("its catalog object is not a dictionary")
		}
		if !pdfDictNameIs(object.dict, "Type", "Catalog") {
			return errors.New("its catalog object is not a /Type /Catalog dictionary")
		}
		return nil
	}
	if generation == 0 {
		listed, unreadable := pdfObjectStreamLists(objects, number, data)
		if listed {
			return nil
		}
		if unreadable != nil {
			return unreadable
		}
	}
	return errors.New("its catalog object is missing")
}

// pdfObjectStreamLists reports whether an /ObjStm object's header names the
// object number, and whether a stream that might have named it could not be
// read. An unreadable object stream returns a named reason instead of becoming
// a silent "catalog object is missing": an unsupported filter, or a body that
// does not decompress, is what staff should be told.
func pdfObjectStreamLists(objects map[pdfObjectKey]*pdfObject, number int, data []byte) (bool, error) {
	var unreadable error
	for _, object := range objects {
		if !object.hasStream || !pdfDictNameIs(object.dict, "Type", "ObjStm") {
			continue
		}
		count, ok := pdfIntegerEntry(object.dict, objects, "N")
		if !ok || count < 1 || count > 1_000_000 {
			continue
		}
		first, ok := pdfIntegerEntry(object.dict, objects, "First")
		if !ok || first < 0 || first > pdfMaxObjectStreamHeaderBytes {
			continue
		}
		header, err := pdfObjectStreamHeader(object.dict, data[object.streamStart:object.streamEnd], int(first))
		if err != nil {
			if unreadable == nil {
				unreadable = err
			}
			continue
		}
		numbers := pdfIntegerTokens(header, int(count)*2)
		for index := 0; index+1 < len(numbers); index += 2 {
			if numbers[index] == int64(number) {
				return true, nil
			}
		}
	}
	return false, unreadable
}

// pdfObjectStreamHeader returns the first /First bytes of the object stream's
// decoded data. FlateDecode (and its /Fl abbreviation, alone or in a
// single-element array) is decompressed with a bounded read, because only the
// object-number index lives there. Any other filter is a named rejection: this
// check does not claim to read it, and it must not report the catalog as absent
// when the reason is a filter it cannot decode.
func pdfObjectStreamHeader(dictionary *pdfValue, body []byte, first int) ([]byte, error) {
	filters, err := pdfStreamFilters(dictionary)
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		if len(body) < first {
			return nil, errors.New("its object stream is shorter than its /First header")
		}
		return body[:first], nil
	}
	if len(filters) != 1 || filters[0] != "FlateDecode" {
		return nil, fmt.Errorf("its object stream uses an unsupported filter (%s); only /FlateDecode is supported",
			strings.Join(filters, " + "))
	}
	reader, err := zlib.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("its /FlateDecode object stream cannot be decompressed")
	}
	defer reader.Close()
	header, err := io.ReadAll(io.LimitReader(reader, int64(first)))
	if err != nil || len(header) < first {
		return nil, errors.New("its /FlateDecode object stream cannot be decompressed")
	}
	return header, nil
}

// pdfStreamFilters returns /Filter as a list of filter names, expanding the
// /Fl abbreviation so a caller compares one spelling. A /Filter chain is
// returned whole, so an unsupported combination is named rather than guessed.
func pdfStreamFilters(dictionary *pdfValue) ([]string, error) {
	if dictionary == nil || dictionary.kind != pdfValueDict {
		return nil, nil
	}
	entry := dictionary.entries["Filter"]
	if entry == nil {
		return nil, nil
	}
	switch entry.kind {
	case pdfValueName:
		return []string{pdfFilterName(entry.name)}, nil
	case pdfValueArray:
		names := make([]string, 0, len(entry.items))
		for _, item := range entry.items {
			if item.kind != pdfValueName {
				return nil, errors.New("its object stream /Filter array is not a list of names")
			}
			names = append(names, pdfFilterName(item.name))
		}
		return names, nil
	}
	return nil, errors.New("its object stream /Filter is neither a name nor a list of names")
}

// pdfFilterName expands the one abbreviation this check understands.
func pdfFilterName(name string) string {
	if name == "Fl" {
		return "FlateDecode"
	}
	return name
}

func pdfIntegerTokens(data []byte, limit int) []int64 {
	if limit > len(data) {
		limit = len(data)
	}
	l := newPDFLexer(data)
	values := make([]int64, 0, limit)
	for len(values) < limit {
		token := l.next()
		if token.kind == pdfTokenEOF {
			return values
		}
		if token.kind == pdfTokenNumber && !token.isFloat {
			values = append(values, token.value)
		}
	}
	return values
}
