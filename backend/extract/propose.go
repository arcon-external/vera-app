package extract

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"vera-app/backend/apicontract"
)

// Issue #477/#476: propose header fields and table rows from positioned words.
//
// The rules are deliberately conservative. A label with no value is `absent`;
// a value that cannot be normalised, a date that could be read two ways, a
// repeated label with a different value, and a row missing a required cell are
// `needs_review`. Nothing is inferred from arithmetic (an amount is what the
// page printed, never quantity x price) and nothing is OCRed: a page with no
// words is `absent` for every field.
//
// Layout handling is generic heuristic, not universal parsing. Header labels
// are looked for on every page, in a left or right block, with the value on the
// same or the next visual line; a table is found from a header row whose
// synonyms appear in any order. None of that is a layout model: a form this
// reader has not seen can still abstain, and abstaining is the intended answer
// when the evidence is not there. The benchmark ticket (#476) requires real
// authorised samples before any accuracy claim, and none of this is that claim.

// Options bounds the work. MaxPages is enforced here as well as by the
// extractor process, so a caller that parses XML directly cannot slip past it.
// MinConfidence gates OCR-read values: a cell whose lowest contributing word
// confidence is below it becomes needs_review. Zero means DefaultMinConfidence.
type Options struct {
	MaxPages      int
	MinConfidence float64
}

// DefaultMaxPages is the page cap this release extracts. It is intentionally
// modest: a customer PO longer than this is a document staff should read, not
// a table an assistant should guess at.
const DefaultMaxPages = 40

// DefaultMinConfidence is the confidence floor for an OCR-read value. It is a
// release policy, not a measured optimum: the benchmark (#476 §2.5) has not
// been run, so the floor is deliberately conservative and documented rather
// than tuned against customer documents.
const DefaultMinConfidence = 70

// Propose reads the word map and returns the extraction result. The engine
// name, version and duration are filled by the caller; this function is pure.
func Propose(pages []Page, options Options) (apicontract.ExtractionOutput, error) {
	maxPages := options.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}
	if len(pages) > maxPages {
		return apicontract.ExtractionOutput{}, fmt.Errorf("%w: %d pages, limit %d", ErrPageLimit, len(pages), maxPages)
	}
	minConfidence := options.MinConfidence
	if minConfidence <= 0 {
		minConfidence = DefaultMinConfidence
	}
	result := apicontract.ExtractionOutput{
		SchemaVersion: apicontract.ExtractionSchemaVersion,
		PageCount:     len(pages),
		Pages:         make([]apicontract.ExtractionPageInfo, 0, len(pages)),
		PageModes:     make([]apicontract.ExtractionPageMode, 0, len(pages)),
		Fields:        make([]apicontract.ExtractionField, 0, len(apicontract.ExtractionFieldKeys)),
		Rows:          make([]apicontract.ExtractionRow, 0),
	}
	for _, page := range pages {
		result.Pages = append(result.Pages, apicontract.ExtractionPageInfo{
			Number: page.Number,
			Width:  page.Width,
			Height: page.Height,
		})
		// The pipeline always sets the mode. An empty mode means a caller fed
		// the parser directly, which is the text-layer path.
		mode := page.Mode
		if mode == "" {
			mode = apicontract.ExtractionPageModeText
		}
		if mode == apicontract.ExtractionPageModeOCR {
			result.PagesWithoutText++
			result.PagesWithOCR++
		} else {
			mode = apicontract.ExtractionPageModeText
			result.PagesWithText++
		}
		result.PageModes = append(result.PageModes, apicontract.ExtractionPageMode{
			Number:     page.Number,
			Mode:       mode,
			TextWords:  page.TextWords,
			TextChars:  page.TextChars,
			OCRWords:   page.OCRWords,
			DPI:        page.DPI,
			Confidence: page.OCRConfidence,
			Reason:     page.Note,
		})
	}

	headerLines := headerArea(pages)
	rows, derivedDelivery := proposeRows(pages, minConfidence)
	result.Fields = proposeFields(headerLines, minConfidence, derivedDelivery)
	result.Rows = rows
	for _, row := range result.Rows {
		if row.Status != apicontract.ExtractionStatusExtracted {
			result.Abstentions.Rows++
		}
	}
	for _, field := range result.Fields {
		if field.Status != apicontract.ExtractionStatusExtracted {
			result.Abstentions.Fields++
		}
	}
	return result, nil
}

type visualLine struct {
	Page  *Page
	Words []Word
	Text  string
	Box   apicontract.ExtractionBBox
}

func linesOf(page *Page) []visualLine {
	if len(page.Words) == 0 {
		return nil
	}
	words := append([]Word(nil), page.Words...)
	sort.SliceStable(words, func(left, right int) bool {
		if words[left].YMin != words[right].YMin {
			return words[left].YMin < words[right].YMin
		}
		return words[left].XMin < words[right].XMin
	})
	lines := make([]visualLine, 0, len(words)/4+4)
	current := make([]Word, 0, 8)
	center := 0.0
	flush := func() {
		if len(current) == 0 {
			return
		}
		sort.SliceStable(current, func(left, right int) bool {
			return current[left].XMin < current[right].XMin
		})
		xMin, yMin, xMax, yMax := unionBox(current)
		lines = append(lines, visualLine{
			Page:  page,
			Words: current,
			Text:  joinWords(current),
			Box: apicontract.ExtractionBBox{
				X: xMin, Y: yMin, Width: xMax - xMin, Height: yMax - yMin,
			},
		})
		current = make([]Word, 0, 8)
	}
	for _, word := range words {
		wordCenter := (word.YMin + word.YMax) / 2
		if len(current) == 0 {
			center = wordCenter
			current = append(current, word)
			continue
		}
		// A line is words sharing a baseline. 2.5pt is a quarter of a normal
		// ten-point glyph: tight enough that two table rows never merge, loose
		// enough that a superscript does not split a line on its own.
		if absFloat(wordCenter-center) > 2.5 {
			flush()
			center = wordCenter
			current = append(current, word)
			continue
		}
		current = append(current, word)
		center = (center*float64(len(current)-1) + wordCenter) / float64(len(current))
	}
	flush()
	return lines
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

// headerArea collects every line that can carry a header field, on every page.
// A PO is not required to print its number, order date or Incoterms on page 1
// or above the table: a right-hand block, a page 2 Incoterms block and a page 3
// submission stamp are all ordinary. The lines belonging to a table are masked
// out so a column header ("Need By") or a row's date is not read as a
// label/value pair; the field/table proposers then see only prose and labels.
func headerArea(pages []Page) []visualLine {
	header := make([]visualLine, 0, 32)
	var carried []tableColumn
	tableOpen := false
	for index := range pages {
		page := &pages[index]
		lines := linesOf(page)
		headerIndex, columns, found := findTableHeader(lines)
		if found {
			header = append(header, lines[:headerIndex]...)
			end := tableEnd(lines, headerIndex, columns)
			header = append(header, lines[end:]...)
			carried = columns
			tableOpen = end >= len(lines)
			continue
		}
		if tableOpen && len(carried) > 0 {
			end := continuationTableEnd(lines, carried)
			header = append(header, lines[end:]...)
			tableOpen = end >= len(lines)
			continue
		}
		header = append(header, lines...)
	}
	// A second table on the same page can leave its column header in the
	// collected prose. It is structure, never a field label line.
	filtered := make([]visualLine, 0, len(header))
	for _, line := range header {
		if _, isHeader := tableColumns(line); isHeader {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func findTableHeader(lines []visualLine) (int, []tableColumn, bool) {
	for index, line := range lines {
		if columns, ok := tableColumns(line); ok {
			return index, columns, true
		}
	}
	return -1, nil, false
}

// tableEnd returns the index of the first line after the table that starts at
// headerIndex. A total, a footer or another labelled header block ends it. A
// table with no readable terminator is treated as running to the page end:
// guessing that prose below it is a header field would be worse than an
// abstention.
// lineAtTableMargin reports whether a line starts left of the description
// column. A footer label starts at the page margin; a "Note:" line inside a
// wrapped description starts at the description column and is data.
func lineAtTableMargin(line visualLine, columns []tableColumn) bool {
	descriptionX := -1.0
	for _, column := range columns {
		if column.Kind == columnDescription {
			descriptionX = column.XMin
			break
		}
	}
	if descriptionX < 0 {
		return true
	}
	return line.Box.X < descriptionX-20
}

// isTableEndLine reports a total, a footer label or a block heading at the
// table's left margin. The margin test is what keeps a line item's own "Note:"
// text from ending its table.
func isTableEndLine(line visualLine, columns []tableColumn) bool {
	// A total is right-aligned and a block heading is left-aligned; both are
	// structural wherever they sit on the line.
	if totalLinePattern.MatchString(line.Text) ||
		tableSectionEndPattern.MatchString(strings.TrimSpace(line.Text)) {
		return true
	}
	if !lineAtTableMargin(line, columns) {
		return false
	}
	return footerLinePattern.MatchString(line.Text) || footerLabelPattern.MatchString(line.Text)
}

func tableEnd(lines []visualLine, headerIndex int, columns []tableColumn) int {
	rows := 0
	for index := headerIndex + 1; index < len(lines); index++ {
		line := lines[index]
		if _, isHeader := tableColumns(line); isHeader {
			continue
		}
		if isTableEndLine(line, columns) {
			if tableSectionEndPattern.MatchString(strings.TrimSpace(line.Text)) {
				// The status/metadata block below the line items is not a
				// header. Keep scanning only for a real field label printed
				// after it (an "Order submitted on" stamp, for example).
				return afterSectionHeading(lines, index)
			}
			return index + 1
		}
		if rows > 0 && matchesFieldLabelNonStatus(line.Text) {
			return index
		}
		cells := assignCells(line, columns)
		if lineAnchor, qtyAnchor, priceAnchor := cells.anchors(); lineAnchor || qtyAnchor || priceAnchor {
			rows++
		}
	}
	return len(lines)
}

// continuationTableEnd finds where a table that began on an earlier page stops
// when this page repeats no column header. A header field label is the end, so
// an "Order submitted on" stamp below a continued table is still read.
func continuationTableEnd(lines []visualLine, columns []tableColumn) int {
	for index, line := range lines {
		if isTableEndLine(line, columns) {
			if tableSectionEndPattern.MatchString(strings.TrimSpace(line.Text)) {
				return afterSectionHeading(lines, index)
			}
			return index + 1
		}
		if matchesFieldLabelNonStatus(line.Text) {
			return index
		}
	}
	return len(lines)
}

// matchesFieldLabel reports whether a line carries any header label, whether or
// not a value follows it on the same line. It is the guard that stops a
// label-only line's "next line value" from being the next label.
func matchesFieldLabel(text string) bool {
	for _, pattern := range fieldPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// matchesFieldLabelNonStatus is matchesFieldLabel without the status-note
// forms of a delivery date ("Estimated Delivery Date"). Those belong to a
// confirmation block, not to the document's header.
func matchesFieldLabelNonStatus(text string) bool {
	for key, pattern := range fieldPatterns {
		location := pattern.FindStringSubmatchIndex(text)
		if location == nil {
			continue
		}
		// A label that starts mid-sentence ("With Delivery Date: ...") is prose
		// from a wrapped status block, not the document's header.
		if location[0] != 0 {
			continue
		}
		if key == apicontract.ExtractionFieldRequestedDeliveryDate && statusQualified(text, location[0]) {
			continue
		}
		return true
	}
	return false
}

// afterSectionHeading skips a status/confirmation/metadata block and returns
// the first later line that carries a real field label, or the page end. It is
// how an "Order submitted on" stamp below the metadata stays readable while the
// metadata itself does not become a header value.
func afterSectionHeading(lines []visualLine, index int) int {
	for next := index + 1; next < len(lines); next++ {
		if matchesFieldLabelNonStatus(lines[next].Text) {
			return next
		}
	}
	return len(lines)
}

// ---------------------------------------------------------------------------
// Header fields
// ---------------------------------------------------------------------------

type textSpan struct {
	Word  Word
	Start int
	End   int
}

func textWithSpans(words []Word) (string, []textSpan) {
	var builder strings.Builder
	spans := make([]textSpan, 0, len(words))
	for index, word := range words {
		if index > 0 {
			builder.WriteByte(' ')
		}
		start := builder.Len()
		builder.WriteString(word.Text)
		spans = append(spans, textSpan{Word: word, Start: start, End: builder.Len()})
	}
	return builder.String(), spans
}

func wordsInRange(spans []textSpan, start, end int) []Word {
	words := make([]Word, 0, 4)
	for _, span := range spans {
		if span.End <= start || span.Start >= end {
			continue
		}
		words = append(words, span.Word)
	}
	return words
}

type fieldMatch struct {
	words []Word
	raw   string
	page  int
}

var fieldPatterns = map[string]*regexp.Regexp{
	apicontract.ExtractionFieldPONumber: regexp.MustCompile(
		`(?i)(?:` +
			// A labelled number may print "No", "Number" or "#". A bare
			// "Purchase Order" may omit the separator; a value that reads as a
			// label fragment is discarded before it can be mistaken for a
			// number. A bare "PO" needs either a separator or whitespace, so
			// "PO-2026-7781" is a value, not a second label.
			`\bpurchase\s+order\s+(?:no|number|#)\.?\s*[:#]?\s*` +
			`|\bpurchase\s+order\s*[:#]?\s*` +
			`|\bp\.?\s*o\.?\s+(?:no|number|#)\.?\s*[:#]?\s*` +
			`|^\s*p\.?\s*o\.?\s*[:#]\s*` +
			`|^\s*p\.?\s*o\.?\s+` +
			`|\bnomor\s+(?:p\.?\s*o\.?|pesanan)\s*[:#]?\s*` +
			`|\bno\.?\s+p\.?\s*o\.?\s*[:#]?\s*` +
			`|\bdocument\s+(?:no|number)\.?\s*[:#]?\s*` +
			`|\bdoc\s+no\.?\s*[:#]?\s*` +
			`)(.*)$`),
	apicontract.ExtractionFieldPODate: regexp.MustCompile(
		`(?i)\b(?:` +
			// "Order submitted on" is the order's own date, not the date a
			// quote or the PDF was produced.
			`order\s+submitted(?:\s+(?:on|at))?` +
			`|purchase\s+order\s+date` +
			`|p\.?\s*o\.?\s+date` +
			`|order\s+date` +
			`|issued(?:\s+(?:on|at))?` +
			`|issue\s+date` +
			`|document\s+date` +
			`|date\s+of\s+(?:the\s+)?(?:purchase\s+)?order` +
			`|tanggal\s+(?:p\.?\s*o\.?|pesanan|order)` +
			`)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldRequestedDeliveryDate: regexp.MustCompile(
		`(?i)\b(?:requested\s+delivery\s+date|required\s+delivery\s+date|` +
			`requested\s+date|required\s+date|need\s+by(?:\s+date)?|delivery\s+date|` +
			`ship\s+date|tanggal\s+(?:kirim|pengiriman)` +
			`)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldCurrency: regexp.MustCompile(
		`(?i)\b(?:currency|mata\s+uang)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldPaymentTerms: regexp.MustCompile(
		`(?i)\b(?:payment\s+terms?|terms\s+of\s+payment|syarat\s+pembayaran)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldIncoterms: regexp.MustCompile(
		`(?i)\b(?:incoterms?(?:\s+code)?|incoterm\s+code|terms\s+of\s+delivery|` +
			`delivery\s+terms|syarat\s+pengiriman)\s*[:#-]?\s*(.*)$`),
}

// deliveryEvidence is a delivery date read from one table row. It is used only
// when no header label prints a delivery date; the derived proposal is flagged
// needs_review because deriving a header value from a line is an inference.
// LineNumber names the row that actually carried the date, which is not
// necessarily line 1: line 1's delivery cell may be unparseable.
type deliveryEvidence struct {
	Value      string
	Raw        string
	LineNumber string
	Page       int
	Words      []Word
}

func proposeFields(lines []visualLine, minConfidence float64, derived *deliveryEvidence) []apicontract.ExtractionField {
	matches := make(map[string][]fieldMatch, len(fieldPatterns))
	for index, line := range lines {
		text, spans := textWithSpans(line.Words)
		for key, pattern := range fieldPatterns {
			location := pattern.FindStringSubmatchIndex(text)
			if location == nil || len(location) < 4 || location[2] < 0 {
				continue
			}
			start, end := trimRange(text, location[2], location[3])
			raw := ""
			if end > start {
				raw = text[start:end]
			}
			words := wordsInRange(spans, start, end)
			page := line.Page.Number
			raw = sanitiseFieldRaw(key, raw)
			if strings.TrimSpace(raw) == "" {
				words = nil
				// A label may print its value on the next visual line. The guard
				// rejects a next line that is itself a label or table header, and
				// only a plausible value for this field is adopted.
				labelX := line.Box.X
				if labelWords := wordsInRange(spans, location[0], location[1]); len(labelWords) > 0 {
					labelX = labelWords[0].XMin
				}
				if next, ok := nextLineValue(lines, index, labelX, key); ok {
					raw = next.Text
					words = next.Words
					page = next.Page.Number
				}
			}
			if key == apicontract.ExtractionFieldPONumber && looksLikeProseValue(raw) {
				// "Supplier to ensure correct PO number and/or Part numbers ..."
				// is a sentence, not a PO number. A generic layout rule would
				// otherwise report it as a second value and flag a conflict.
				continue
			}
			if key == apicontract.ExtractionFieldRequestedDeliveryDate && statusQualified(text, location[0]) {
				// "Confirmed (Estimated Delivery Date: ...)" is a status note,
				// not the PO's requested delivery header.
				continue
			}
			matches[key] = append(matches[key], fieldMatch{
				words: words,
				raw:   raw,
				page:  page,
			})
		}
	}

	fields := make([]apicontract.ExtractionField, 0, len(apicontract.ExtractionFieldKeys))
	for _, key := range apicontract.ExtractionFieldKeys {
		fields = append(fields, proposeField(key, matches[key], lines, minConfidence, derived))
	}
	return fields
}

// looksLikeProseValue reports a value that reads as a sentence rather than an
// identifier. A PO number can contain spaces ("PO 123"), so only a spaced value
// with a run of three lowercase letters is refused.
func looksLikeProseValue(value string) bool {
	if !strings.Contains(value, " ") {
		return false
	}
	run := 0
	for _, character := range value {
		if character >= 'a' && character <= 'z' {
			run++
			if run >= 3 {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

// statusQualified reports whether the text before a label match names a status
// statement ("Estimated Delivery Date", "Confirmed Delivery Date") rather than
// the document's own header.
func statusQualified(text string, start int) bool {
	if start <= 0 {
		return false
	}
	fields := strings.Fields(strings.ToLower(text[:start]))
	if len(fields) == 0 {
		return false
	}
	switch strings.Trim(fields[len(fields)-1], "(:;,[") {
	case "estimated", "estimate", "confirmed", "confirmation", "status", "expected":
		return true
	}
	return false
}

// nextLineValue returns the value printed below a label whose own line carried
// none. It searches up to three visual lines down (a two-column page can put an
// unrelated block between the label and its value), keeps only the words in the
// label's column, and adopts the line only when it is a plausible value for
// this specific field: a supplier or address line below "Payment Terms:" is
// not payment terms.
func nextLineValue(lines []visualLine, index int, labelX float64, key string) (visualLine, bool) {
	if index+1 >= len(lines) {
		return visualLine{}, false
	}
	current := lines[index]
	maxGap := 3.0 * (current.Box.Height + 1)
	for offset := 1; offset <= 3 && index+offset < len(lines); offset++ {
		next := lines[index+offset]
		if next.Page.Number != current.Page.Number {
			return visualLine{}, false
		}
		gap := next.Box.Y - current.Box.Y
		if gap <= 0 {
			continue
		}
		if gap > maxGap {
			return visualLine{}, false
		}
		candidate, ok := columnBlock(next, labelX)
		if !ok {
			continue
		}
		text := strings.TrimSpace(candidate.Text)
		if text == "" {
			continue
		}
		if matchesFieldLabel(text) ||
			totalLinePattern.MatchString(text) ||
			footerLinePattern.MatchString(text) ||
			footerLabelPattern.MatchString(text) ||
			looksLikePartyOrAddress(text) {
			continue
		}
		if _, isHeader := tableColumns(candidate); isHeader {
			continue
		}
		if !plausibleNextLineValue(key, text) {
			continue
		}
		return candidate, true
	}
	return visualLine{}, false
}

// sanitiseFieldRaw drops a value that is only punctuation or a label fragment.
// A bare "Purchase Order Date" line must become a label with no value, not a
// PO number "Date ..."; "Incoterms Information" is a section title, not DDP.
func sanitiseFieldRaw(key, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !containsLetter(trimmed) && !containsDigit(trimmed) {
		return ""
	}
	switch key {
	case apicontract.ExtractionFieldPONumber:
		if looksLikePOValueFragment(trimmed) || looksLikeProseValue(trimmed) {
			return ""
		}
	case apicontract.ExtractionFieldIncoterms:
		switch strings.ToLower(strings.Trim(trimmed, ".:;")) {
		case "information", "details", "data", "location", "code", "terms":
			return ""
		}
	}
	return trimmed
}

// looksLikePOValueFragment reports a captured value that is really the start of
// another label on the same line ("Purchase Order Date: ...").
func looksLikePOValueFragment(value string) bool {
	fields := strings.Fields(strings.ToLower(value))
	if len(fields) == 0 {
		return false
	}
	switch strings.Trim(fields[0], ".:;()") {
	case "date", "no", "number", "currency", "terms", "incoterm", "incoterms", "submitted", "confirmed":
		return true
	}
	return false
}

// plausibleNextLineValue is the field-specific guard for a label-only line.
// A value is adopted only when the field's own normaliser would accept it (or,
// for a date, when it visibly reads as a date but is merely ambiguous).
func plausibleNextLineValue(key, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || looksLikePartyOrAddress(text) {
		return false
	}
	switch key {
	case apicontract.ExtractionFieldPONumber:
		return validPONumber(text) && !looksLikeProseValue(text)
	case apicontract.ExtractionFieldPODate, apicontract.ExtractionFieldRequestedDeliveryDate:
		if _, status, _ := normaliseDate(text); status == apicontract.ExtractionStatusExtracted {
			return true
		}
		return looksLikeDateValue(text) && !looksLikeProseValue(text)
	case apicontract.ExtractionFieldCurrency:
		_, status, _ := normaliseCurrency(text)
		return status == apicontract.ExtractionStatusExtracted
	case apicontract.ExtractionFieldPaymentTerms:
		return plausiblePaymentTerms(text)
	case apicontract.ExtractionFieldIncoterms:
		_, status, _ := normaliseIncoterms(text)
		return status == apicontract.ExtractionStatusExtracted
	}
	return true
}

// looksLikeDateValue is the permissive test for an ambiguous date: it must
// contain a digit and a month name or a date separator. It is only used when
// normaliseDate has already returned needs_review.
func looksLikeDateValue(text string) bool {
	if !containsDigit(text) {
		return false
	}
	lower := strings.ToLower(text)
	for month := range monthNames {
		if strings.Contains(lower, month) {
			return true
		}
	}
	return dayMonthYear.MatchString(text) || isoDate.MatchString(text)
}

// plausiblePaymentTerms accepts a term line with a payment marker or a number.
// A bare company/address line has neither and is rejected before this point.
func plausiblePaymentTerms(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{
		"net", "day", "month", "week", "advance", "cash", "cod", "credit",
		"invoice", "receipt", "due", "dp", "tt", "t/t", "lc", "l/c",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return containsDigit(text)
}

var (
	partyWordsPattern = regexp.MustCompile(
		`(?i)\b(supplier|vendor|customer|buyer|purchaser|bill\s+to|ship\s+to|sold\s+to|deliver\s+to|address|contact|attention|attn|tel|telephone|phone|fax|email)\b`)
	companySuffixPattern = regexp.MustCompile(
		`(?i)\b(pty|ltd|limited|inc|llc|gmbh|corp|corporation|company|co|sdn|bhd|pt|pte)\b`)
	addressWordsPattern = regexp.MustCompile(
		`(?i)\b(street|st|road|rd|avenue|ave|lane|blvd|boulevard|jalan|jl|blok|block|lot|kecamatan|kelurahan|kota|island|park|industrial|zip|postal)\b`)
)

// looksLikePartyOrAddress rejects a party or address line as a next-line
// value. A company name, a street address, an email or a phone number is never
// payment terms, an Incoterm, a currency or a PO number.
func looksLikePartyOrAddress(text string) bool {
	if partyWordsPattern.MatchString(text) ||
		companySuffixPattern.MatchString(text) ||
		addressWordsPattern.MatchString(text) {
		return true
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "@") || strings.Contains(lower, "www.") ||
		longDigitsPattern.MatchString(text)
}

// columnBlock keeps the words of one visual line that belong to the column
// starting at x. The 25-point left tolerance absorbs a label and a value that
// are not perfectly left-aligned; a gap wider than 60 points ends the block, so
// the next column on the same line is not adopted as part of the value.
func columnBlock(line visualLine, x float64) (visualLine, bool) {
	words := make([]Word, 0, len(line.Words))
	lastCenter := 0.0
	for _, word := range line.Words {
		center := (word.XMin + word.XMax) / 2
		if center < x-25 {
			continue
		}
		if len(words) > 0 && center-lastCenter > 60 {
			break
		}
		words = append(words, word)
		lastCenter = center
	}
	if len(words) == 0 {
		return visualLine{}, false
	}
	xMin, yMin, xMax, yMax := unionBox(words)
	return visualLine{
		Page:  line.Page,
		Words: words,
		Text:  joinWords(words),
		Box: apicontract.ExtractionBBox{
			X: xMin, Y: yMin, Width: xMax - xMin, Height: yMax - yMin,
		},
	}, true
}

func trimRange(text string, start, end int) (int, int) {
	for start < end && (text[start] == ' ' || text[start] == '\t') {
		start++
	}
	for end > start && (text[end-1] == ' ' || text[end-1] == '\t') {
		end--
	}
	return start, end
}

func proposeField(key string, matches []fieldMatch, headerLines []visualLine, minConfidence float64, derived *deliveryEvidence) apicontract.ExtractionField {
	field := apicontract.ExtractionField{
		Key: key,
		ExtractionCell: apicontract.ExtractionCell{
			Status: apicontract.ExtractionStatusAbsent,
		},
	}
	if key == apicontract.ExtractionFieldCurrency {
		// A direct "Currency:" label is one answer; an amount line that prints
		// a code is another. Both are kept so a disagreement stays visible.
		matches = append(matches, fallbackCurrency(headerLines)...)
	}
	if len(matches) == 0 {
		return derivedDeliveryField(field, derived, minConfidence)
	}
	// A field label can be a lookalike in prose ("... issued to the supplier")
	// or a section title ("Incoterm Location"). Prefer candidates that the
	// field's normalizer actually accepts; only fall back to an unwelcome
	// candidate when the document printed no valid value, so a real invalid
	// value still arrives as needs_review rather than disappearing.
	selection := validFieldSelection(key, matches)
	hasValue := false
	for _, match := range selection {
		if strings.TrimSpace(match.raw) != "" {
			hasValue = true
			break
		}
	}
	if !hasValue {
		if filled := derivedDeliveryField(field, derived, minConfidence); filled.Status != apicontract.ExtractionStatusAbsent || filled.Note != "" {
			return filled
		}
		field.Status = apicontract.ExtractionStatusAbsent
		field.Note = "The label was found without a usable value."
		return field
	}
	chosen := selection[0]
	for _, match := range selection {
		if strings.TrimSpace(match.raw) != "" {
			chosen = match
			break
		}
	}
	field.Raw = chosen.raw
	field.Page = pointerTo(chosen.page)
	field.BBox = bboxOf(chosen.words)

	distinct := map[string]bool{}
	for _, match := range selection {
		raw := strings.TrimSpace(match.raw)
		if raw == "" {
			continue
		}
		// Two printings of the same value in different formats are not a
		// conflict: page 1 may print "Tuesday 7 Apr 2026" and the final page the
		// same date with a time. Compare the normalised value when there is one.
		distinctKey := strings.ToUpper(strings.Join(strings.Fields(raw), " "))
		if value, status, _ := normaliseField(key, raw); status == apicontract.ExtractionStatusExtracted {
			distinctKey = "value:" + value
		}
		distinct[distinctKey] = true
	}
	value, status, note := normaliseField(key, chosen.raw)
	field.Value = value
	field.Status = status
	field.Note = note
	if len(distinct) > 1 {
		field.Status = apicontract.ExtractionStatusNeedsReview
		field.Note = fmt.Sprintf("Another value was printed for this field (%q); confirm which one applies.", firstDifferent(selection, chosen.raw))
	}
	applyConfidence(&field.ExtractionCell, chosen.words, minConfidence)
	return field
}

// validFieldSelection prefers candidates the field's own normalizer accepts.
// A legal sentence after a lookalike label, a section title ("Incoterm
// Location"), or an address below a label must not outrank the real value that
// follows. If nothing is valid, the raw candidates remain so a genuinely
// malformed printed value still becomes a reviewable field, never a silent
// absence.
func validFieldSelection(key string, matches []fieldMatch) []fieldMatch {
	valid := make([]fieldMatch, 0, len(matches))
	for _, match := range matches {
		raw := strings.TrimSpace(match.raw)
		if raw == "" {
			continue
		}
		if key == apicontract.ExtractionFieldPaymentTerms && !plausiblePaymentTerms(raw) {
			continue
		}
		if _, status, _ := normaliseField(key, raw); status == apicontract.ExtractionStatusExtracted {
			valid = append(valid, match)
		}
	}
	if len(valid) > 0 {
		// Currency is the one field with a context fallback (the amount line).
		// A valid fallback must not silently erase an invalid labelled
		// currency: keep both candidates so the printed label becomes a
		// reviewable conflict instead of disappearing.
		if key == apicontract.ExtractionFieldCurrency && len(valid) < len(matches) {
			return matches
		}
		return valid
	}
	return matches
}

// derivedDeliveryField fills the delivery date from the row that actually
// carried a readable date. The value is still an inference, so it is
// needs_review, its note names that row, and a measured OCR confidence is
// applied exactly as it would be for a printed header value.
func derivedDeliveryField(field apicontract.ExtractionField, derived *deliveryEvidence, minConfidence float64) apicontract.ExtractionField {
	if derived == nil || field.Key != apicontract.ExtractionFieldRequestedDeliveryDate {
		return field
	}
	field.Value = derived.Value
	field.Raw = derived.Raw
	field.Page = pointerTo(derived.Page)
	field.BBox = bboxOf(derived.Words)
	field.Status = apicontract.ExtractionStatusNeedsReview
	if derived.LineNumber != "" {
		field.Note = fmt.Sprintf(
			"The header does not print a delivery date; this is line %s's delivery date on page %d. Confirm that it applies to the whole order.",
			derived.LineNumber, derived.Page)
	} else {
		field.Note = fmt.Sprintf(
			"The header does not print a delivery date; this is the first line with a readable delivery date on page %d. Confirm that it applies to the whole order.",
			derived.Page)
	}
	applyConfidence(&field.ExtractionCell, derived.Words, minConfidence)
	return field
}

func firstDifferent(matches []fieldMatch, chosen string) string {
	for _, match := range matches {
		raw := strings.TrimSpace(match.raw)
		// A label-only occurrence has no value and must never become the
		// quoted "other value" in a conflict note.
		if raw != "" && raw != strings.TrimSpace(chosen) {
			return raw
		}
	}
	return ""
}

func bboxOf(words []Word) *apicontract.ExtractionBBox {
	if len(words) == 0 {
		return nil
	}
	xMin, yMin, xMax, yMax := unionBox(words)
	return &apicontract.ExtractionBBox{X: xMin, Y: yMin, Width: xMax - xMin, Height: yMax - yMin}
}

// applyConfidence records the lowest word confidence behind a cell and turns a
// low-confidence value into needs_review. A cell whose words are not all
// measured claims no confidence: mixing measured and unmeasured words would
// present a text-layer word as if OCR had scored it.
func applyConfidence(cell *apicontract.ExtractionCell, words []Word, minConfidence float64) {
	confidence := cellConfidence(words)
	if confidence == nil {
		return
	}
	cell.Confidence = confidence
	if *confidence >= minConfidence {
		return
	}
	if cell.Status == apicontract.ExtractionStatusExtracted {
		cell.Status = apicontract.ExtractionStatusNeedsReview
	}
	cell.Note = appendNote(cell.Note, fmt.Sprintf(
		"Read with OCR confidence %.0f%%, below the %.0f%% this release accepts; confirm the printed value.",
		*confidence, minConfidence))
}

func cellConfidence(words []Word) *float64 {
	var minimum *float64
	for _, word := range words {
		if word.Confidence == nil {
			return nil
		}
		if minimum == nil || *word.Confidence < *minimum {
			value := *word.Confidence
			minimum = &value
		}
	}
	return minimum
}

func appendNote(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + " " + addition
}

var currencyCandidates = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "JPY": true, "CNY": true,
	"IDR": true, "SGD": true, "MYR": true, "THB": true, "HKD": true,
	"AUD": true, "NZD": true, "CHF": true, "CAD": true, "KRW": true,
	"INR": true, "PHP": true, "VND": true, "SAR": true, "AED": true,
}

var (
	currencyContextPattern = regexp.MustCompile(
		`(?i)\b(amounts?|totals?|sub[-\s]*totals?|nilai|currency|mata\s+uang)\b`)
	amountShapePattern = regexp.MustCompile(`\d[\d.,]*\d`)
	longDigitsPattern  = regexp.MustCompile(`\d{5,}`)
)

// fallbackCurrency reads a currency code only from a line that prints it in an
// amount context: an amount/total/currency label, or a printed amount. A prose
// sentence ("Deposit payable in USD") is not a currency field. Each match keeps
// only its own line's words, so a bbox never spans pages.
func fallbackCurrency(lines []visualLine) []fieldMatch {
	matches := make([]fieldMatch, 0, 2)
	for _, line := range lines {
		if !currencyContextPattern.MatchString(line.Text) && !amountShapePattern.MatchString(line.Text) {
			continue
		}
		words := make([]Word, 0, 2)
		codes := make([]string, 0, 2)
		seen := map[string]bool{}
		for _, word := range line.Words {
			token := strings.ToUpper(strings.Trim(word.Text, ".,:;()"))
			if !currencyCandidates[token] {
				continue
			}
			words = append(words, word)
			if !seen[token] {
				seen[token] = true
				codes = append(codes, token)
			}
		}
		if len(codes) == 0 {
			continue
		}
		matches = append(matches, fieldMatch{
			words: words,
			raw:   strings.Join(codes, " / "),
			page:  line.Page.Number,
		})
	}
	return matches
}

func normaliseField(key, raw string) (string, string, string) {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if trimmed == "" {
		return "", apicontract.ExtractionStatusAbsent, ""
	}
	switch key {
	case apicontract.ExtractionFieldPONumber:
		if !validPONumber(trimmed) {
			return trimmed, apicontract.ExtractionStatusNeedsReview,
				"The printed value does not look like a purchase order number; confirm it."
		}
		return trimmed, apicontract.ExtractionStatusExtracted, ""
	case apicontract.ExtractionFieldPODate, apicontract.ExtractionFieldRequestedDeliveryDate:
		return normaliseDate(trimmed)
	case apicontract.ExtractionFieldCurrency:
		return normaliseCurrency(trimmed)
	case apicontract.ExtractionFieldPaymentTerms:
		if len([]rune(trimmed)) > 500 {
			return trimmed, apicontract.ExtractionStatusNeedsReview,
				"The printed terms are longer than the 500 characters this field accepts; shorten them before applying."
		}
		return trimmed, apicontract.ExtractionStatusExtracted, ""
	case apicontract.ExtractionFieldIncoterms:
		return normaliseIncoterms(trimmed)
	}
	return trimmed, apicontract.ExtractionStatusExtracted, ""
}

var poNumberPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._/#-]{0,63}$`)

var poNumberPlaceholders = map[string]bool{
	"tbd": true, "tba": true, "n/a": true, "na": true, "n.a.": true,
	"xxx": true, "xx": true, "xxxx": true, "none": true, "pending": true,
	"not available": true, "to be advised": true, "to be determined": true,
}

func validPONumber(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Trim(trimmed, "-–—_. ") == "" {
		return false
	}
	if poNumberPlaceholders[strings.ToLower(strings.Join(strings.Fields(trimmed), " "))] {
		return false
	}
	return poNumberPattern.MatchString(trimmed)
}

var monthNames = map[string]time.Month{
	"jan": time.January, "january": time.January, "januari": time.January,
	"feb": time.February, "february": time.February, "februari": time.February,
	"mar": time.March, "march": time.March, "maret": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May, "mei": time.May,
	"jun": time.June, "june": time.June, "juni": time.June,
	"jul": time.July, "july": time.July, "juli": time.July,
	"aug": time.August, "august": time.August, "agu": time.August, "agustus": time.August,
	"sep": time.September, "sept": time.September, "september": time.September,
	"oct": time.October, "october": time.October, "okt": time.October, "oktober": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December, "des": time.December, "desember": time.December,
}

// Date patterns are unanchored on purpose: a printed value often carries a
// weekday and a time ("Tuesday 7 Apr 2026 8:02 AM GMT+07:00"), and the date is
// the part that matters. Two distinct dates in one value is a conflict, never a
// preference for the first one.
var (
	isoDate          = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	dayMonthYear     = regexp.MustCompile(`(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})`)
	dayNameMonthYear = regexp.MustCompile(`(\d{1,2})(?:st|nd|rd|th)?\s+([A-Za-z]+)\s+(\d{4})`)
	monthDayYear     = regexp.MustCompile(`([A-Za-z]+)\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(\d{4})`)
)

func normaliseDate(raw string) (string, string, string) {
	value := strings.TrimSpace(strings.Trim(raw, ".,;"))
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "", apicontract.ExtractionStatusAbsent, ""
	}
	if matches := isoDate.FindAllStringSubmatch(value, -1); len(matches) > 0 {
		if len(matches) > 1 {
			return raw, apicontract.ExtractionStatusNeedsReview, "More than one date is printed; confirm which one applies."
		}
		match := matches[0]
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		if validDate(year, month, day) {
			return formatDate(year, month, day), apicontract.ExtractionStatusExtracted, ""
		}
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed date is not a real calendar date."
	}
	if matches := dayMonthYear.FindAllStringSubmatch(value, -1); len(matches) > 0 {
		if len(matches) > 1 {
			return raw, apicontract.ExtractionStatusNeedsReview, "More than one date is printed; confirm which one applies."
		}
		match := matches[0]
		first, _ := strconv.Atoi(match[1])
		second, _ := strconv.Atoi(match[2])
		year, _ := strconv.Atoi(match[3])
		// Day-first and month-first are both plausible unless one component is
		// greater than 12. Guessing would put a reviewable date into the wrong
		// month, so the extractor abstains and keeps the printed text.
		switch {
		case first > 12 && second <= 12:
			if validDate(year, second, first) {
				return formatDate(year, second, first), apicontract.ExtractionStatusExtracted, ""
			}
		case second > 12 && first <= 12:
			if validDate(year, first, second) {
				return formatDate(year, first, second), apicontract.ExtractionStatusExtracted, ""
			}
		default:
			return raw, apicontract.ExtractionStatusNeedsReview,
				"The date could be read day-first or month-first; confirm which one is printed."
		}
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed date is not a real calendar date."
	}
	if matches := dayNameMonthYear.FindAllStringSubmatch(value, -1); len(matches) > 0 {
		if len(matches) > 1 {
			return raw, apicontract.ExtractionStatusNeedsReview, "More than one date is printed; confirm which one applies."
		}
		match := matches[0]
		day, _ := strconv.Atoi(match[1])
		month, ok := monthNames[strings.ToLower(match[2])]
		year, _ := strconv.Atoi(match[3])
		if ok && validDate(year, int(month), day) {
			return formatDate(year, int(month), day), apicontract.ExtractionStatusExtracted, ""
		}
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed date could not be read."
	}
	if matches := monthDayYear.FindAllStringSubmatch(value, -1); len(matches) > 0 {
		if len(matches) > 1 {
			return raw, apicontract.ExtractionStatusNeedsReview, "More than one date is printed; confirm which one applies."
		}
		match := matches[0]
		month, ok := monthNames[strings.ToLower(match[1])]
		day, _ := strconv.Atoi(match[2])
		year, _ := strconv.Atoi(match[3])
		if ok && validDate(year, int(month), day) {
			return formatDate(year, int(month), day), apicontract.ExtractionStatusExtracted, ""
		}
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed date could not be read."
	}
	return raw, apicontract.ExtractionStatusNeedsReview, "The printed value is not a recognised date format."
}

func validDate(year, month, day int) bool {
	if year < 1900 || year > 2200 || month < 1 || month > 12 || day < 1 || day > 31 {
		return false
	}
	parsed := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return parsed.Year() == year && int(parsed.Month()) == month && parsed.Day() == day
}

func formatDate(year, month, day int) string {
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

func normaliseCurrency(raw string) (string, string, string) {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	codes := currencyCodes(strings.ToUpper(trimmed))
	if len(codes) > 1 {
		return trimmed, apicontract.ExtractionStatusNeedsReview,
			fmt.Sprintf("More than one currency is printed (%s); confirm which one applies.", strings.Join(codes, ", "))
	}
	if len(codes) == 1 {
		return codes[0], apicontract.ExtractionStatusExtracted, ""
	}
	return raw, apicontract.ExtractionStatusNeedsReview,
		"The printed currency is not one of the supported three-letter codes; confirm the currency."
}

// currencyCodes returns the distinct supported codes printed in the value, in
// order. A value naming two codes is two answers, not a preference: the caller
// must abstain rather than take the first token as fact.
func currencyCodes(upper string) []string {
	seen := map[string]bool{}
	codes := make([]string, 0, 2)
	for _, token := range strings.FieldsFunc(upper, func(character rune) bool {
		return character < 'A' || character > 'Z'
	}) {
		if len(token) != 3 || !currencyCandidates[token] || seen[token] {
			continue
		}
		seen[token] = true
		codes = append(codes, token)
	}
	return codes
}

// validThousandsGrouping reports whether value is digits in groups of three,
// with the first group one to three digits. A separator that is not grouping
// (1.2.3) must abstain, never be silently flattened into 123.
func validThousandsGrouping(value string, separator byte) bool {
	groups := strings.Split(value, string(separator))
	if len(groups) < 2 {
		return false
	}
	for index, group := range groups {
		if group == "" {
			return false
		}
		for _, character := range group {
			if character < '0' || character > '9' {
				return false
			}
		}
		if index == 0 {
			if len(group) > 3 {
				return false
			}
			continue
		}
		if len(group) != 3 {
			return false
		}
	}
	return true
}

var incoterms = map[string]bool{
	"EXW": true, "FCA": true, "FAS": true, "FOB": true, "CFR": true,
	"CIF": true, "CPT": true, "CIP": true, "DAP": true, "DPU": true, "DDP": true,
}

func normaliseIncoterms(raw string) (string, string, string) {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", apicontract.ExtractionStatusAbsent, ""
	}
	code := strings.ToUpper(strings.Trim(fields[0], ".,:;"))
	if !incoterms[code] {
		return trimmed, apicontract.ExtractionStatusNeedsReview,
			"The printed delivery term is not one of the recognised Incoterms codes."
	}
	fields[0] = code
	return strings.Join(fields, " "), apicontract.ExtractionStatusExtracted, ""
}

// ---------------------------------------------------------------------------
// Table rows
// ---------------------------------------------------------------------------

type columnKind int

const (
	columnUnknown columnKind = iota
	columnLineNumber
	columnDescription
	columnQuantity
	columnUoM
	columnUnitPrice
	columnAmount
	// columnDeliveryDate is a date column. Its values are evidence for a
	// header delivery date, never a unit or a price.
	columnDeliveryDate
	// columnIgnored is a printed column this reader does not consume
	// ("Type", "Return", "No. Schedule Lines"). Keeping its span explicit is
	// what stops its values falling into the description or quantity cells.
	columnIgnored
)

type tableColumn struct {
	Kind columnKind
	XMin float64
	XMax float64
}

var totalLinePattern = regexp.MustCompile(
	`(?i)^\s*(sub[-\s]*total|grand\s*total|total|jumlah|amount\s+due|balance\s+due|ppn|vat|tax|discount|pph)\b`)
var footerLinePattern = regexp.MustCompile(
	`(?i)^\s*(notes?|remarks?|terms\s+and\s+conditions|signature|prepared\s+by|approved\s+by|authorized\s+by|received\s+by|page\s+\d+)\b`)
var footerLabelPattern = regexp.MustCompile(
	`(?i)^\s*(requester|requested\s+by|buyer|purchaser|attention|attn|to|from|ship\s+to|bill\s+to|delivery\s+address|invoice|payment|bank|account|notes?|remarks?|terms|conditions|signature|prepared\s+by|approved\s+by|authorized\s+by|received\s+by|page|catatan|keterangan|alamat|tanggal|req\.?\s*line\s*no|requisition|hazardous|doc(?:ument)?\s*type|product\s*/\s*service|classification|gift|hide\s+item\s+prices)\b\s*:`)

// tableSectionEndPattern matches a printed block heading that follows the line
// items ("STATUS", "Other Information"). It ends the table without guessing at
// the rows below it: those lines are confirmation or metadata, not line items.
var tableSectionEndPattern = regexp.MustCompile(
	`(?i)^\s*(status|other\s+information|additional\s+information|line\s+items|item\s+details|confirmations?|comments|attachments?)\s*$`)

// runningHeaderPattern matches the repeated page furniture above a continued
// table. It is deliberately narrow: a wrapped description at the top of a
// continuation page is data and must still be appended to its row.
var runningHeaderPattern = regexp.MustCompile(
	`(?i)^\s*(page\s+\d+|purchase\s+order|invoice|delivery\s+order)\b`)

// tableColumns reads one printed line as a column header. It is a phrase
// matcher, not a template: the synonyms the authorised POs print are accepted
// in any order and at any page position, multi-word labels are consumed whole,
// and a known non-data column keeps its own span so its values are dropped.
// A line that does not carry a description plus at least one quantity/price
// column is not a table header.
func tableColumns(line visualLine) ([]tableColumn, bool) {
	tokens := make([]string, len(line.Words))
	for index, word := range line.Words {
		tokens[index] = headerToken(word)
	}
	columns := make([]tableColumn, 0, 10)
	for index := 0; index < len(line.Words); {
		kind, span := classifyHeaderColumn(tokens, index)
		if span <= 0 {
			index++
			continue
		}
		last := index + span - 1
		if last >= len(line.Words) {
			last = len(line.Words) - 1
		}
		columns = append(columns, tableColumn{
			Kind: kind,
			XMin: line.Words[index].XMin,
			XMax: line.Words[last].XMax,
		})
		index = last + 1
	}
	if len(columns) == 0 {
		return nil, false
	}
	sort.SliceStable(columns, func(left, right int) bool { return columns[left].XMin < columns[right].XMin })

	// Keep the leftmost data column of each kind; a second "price" column is
	// not something this reader can disambiguate. Ignored columns are all
	// kept, because each one is a boundary that keeps its values out.
	seen := map[columnKind]bool{}
	unique := make([]tableColumn, 0, len(columns))
	for _, column := range columns {
		if column.Kind == columnIgnored {
			unique = append(unique, column)
			continue
		}
		if column.Kind == columnUnknown || seen[column.Kind] {
			continue
		}
		seen[column.Kind] = true
		unique = append(unique, column)
	}
	if len(unique) < 2 || !seen[columnDescription] ||
		(!seen[columnQuantity] && !seen[columnUnitPrice] && !seen[columnAmount]) {
		return nil, false
	}
	return unique, true
}

// headerToken lowercases a header word and removes the punctuation printers put
// around labels. "#" and "/" survive because "Part # / Description" is one
// column, and "No. Schedule Lines" is not the line number.
func headerToken(word Word) string {
	value := strings.ToLower(strings.TrimSpace(word.Text))
	return strings.Trim(value, ".:()")
}

func classifyHeaderColumn(tokens []string, index int) (columnKind, int) {
	token := tokens[index]
	next, third := "", ""
	if index+1 < len(tokens) {
		next = tokens[index+1]
	}
	if index+2 < len(tokens) {
		third = tokens[index+2]
	}
	switch {
	// Description phrases. The merged part/description column comes first so
	// its placeholder or part number can be stripped from the description.
	case token == "part":
		if span := partDescriptionSpan(tokens, index); span > 0 {
			return columnDescription, span
		}
		return columnUnknown, 0
	case token == "item" && next == "description":
		return columnDescription, 2
	case token == "product" && next == "description":
		return columnDescription, 2
	case token == "description" && next == "of" &&
		(third == "goods" || third == "items" || third == "services"):
		return columnDescription, 3
	case token == "description" || token == "deskripsi" || token == "uraian" ||
		token == "particulars" || token == "goods" || token == "product" ||
		token == "article":
		return columnDescription, 1

	// Reference columns that are not the line number. "No. Schedule Lines"
	// is the recurring example; consuming it keeps its values out of the
	// description and quantity cells.
	case token == "no" && (next == "schedule" || next == "scheduled"):
		if third == "lines" || third == "line" {
			return columnIgnored, 3
		}
		return columnIgnored, 2
	case token == "schedule" && (next == "lines" || next == "line"):
		return columnIgnored, 2

	// Money columns before the bare unit/rate and total/line words.
	case token == "unit" && (next == "price" || next == "cost" || next == "rate"):
		return columnUnitPrice, 2
	case token == "harga" && (next == "satuan" || next == "unit"):
		return columnUnitPrice, 2
	case token == "price" || token == "harga" || token == "rate" || token == "cost" ||
		token == "tariff":
		return columnUnitPrice, 1
	case token == "line" && next == "total":
		return columnAmount, 2
	case token == "extended" && next == "price":
		return columnAmount, 2
	case token == "sub" && next == "total":
		return columnAmount, 2
	case token == "sub-total" || token == "subtotal" || token == "amount" ||
		token == "total" || token == "nilai" || token == "sum":
		return columnAmount, 1

	// A delivery date has its own span, so "31 Jan 2027" never lands in the
	// quantity or unit-of-measure cell.
	case token == "need" && next == "by":
		return columnDeliveryDate, 2
	case (token == "delivery" || token == "required" || token == "requested") && next == "date":
		return columnDeliveryDate, 2
	case token == "eta":
		return columnDeliveryDate, 1

	// "Qty (Unit)" is one combined column whose values print "1 (PK)"; a
	// separate "Qty" + "Unit Price" is two columns and is not combined.
	case (token == "qty" || token == "quantity" || token == "kuantitas") && isCombinedUnitHeader(tokens, index):
		return columnQuantity, 2
	case token == "qty" || token == "quantity" || token == "kuantitas" ||
		token == "jumlah" || token == "count":
		return columnQuantity, 1

	// Line number, after "Line Total" and after the schedule-lines phrases.
	case token == "line" && (next == "#" || next == "no" || next == "number"):
		return columnLineNumber, 2
	case token == "sl" && next == "no":
		return columnLineNumber, 2
	case token == "line" || token == "no" || token == "nomor" || token == "#" ||
		token == "seq" || token == "item":
		return columnLineNumber, 1

	case token == "uom" || token == "um" || token == "u/m" || token == "unit" ||
		token == "satuan" || token == "measure":
		return columnUoM, 1

	case token == "type" || token == "return" || token == "returnable" ||
		token == "status" || token == "currency" || token == "discount" ||
		token == "tax" || token == "vat":
		return columnIgnored, 1
	}
	return columnUnknown, 0
}

// partDescriptionSpan returns how many header words form a merged part and
// description column, or zero when the words are not that phrase.
func partDescriptionSpan(tokens []string, index int) int {
	offset := 1
	if index+offset < len(tokens) {
		switch tokens[index+offset] {
		case "no", "number", "#":
			offset++
		}
	}
	if index+offset < len(tokens) && tokens[index+offset] == "/" {
		offset++
	}
	if index+offset < len(tokens) &&
		(tokens[index+offset] == "description" || tokens[index+offset] == "desc") {
		return offset + 1
	}
	return 0
}

func isCombinedUnitHeader(tokens []string, index int) bool {
	if index+1 >= len(tokens) {
		return false
	}
	next := tokens[index+1]
	if next != "unit" && next != "uom" {
		return false
	}
	if index+2 < len(tokens) {
		switch tokens[index+2] {
		case "price", "cost", "rate", "harga", "unit", "uom":
			return false
		}
	}
	return true
}

func proposeRows(pages []Page, minConfidence float64) ([]apicontract.ExtractionRow, *deliveryEvidence) {
	rows := make([]apicontract.ExtractionRow, 0, 16)
	previous := -1
	var carried []tableColumn
	carriedFrom := 0
	tableOpen := false
	var derived *deliveryEvidence
	for pageIndex := range pages {
		page := &pages[pageIndex]
		lines := linesOf(page)

		// A page either continues the previous page's table or starts on its
		// own first column header. Several headers on one page are normal: a
		// PO can print each line item in its own "Line Items" block, with a
		// repeated header and a STATUS block between them.
		var columns []tableColumn
		continuation := false
		if tableOpen && len(carried) > 0 {
			columns = carried
			continuation = true
		}
		started := false
		for index := 0; index < len(lines); index++ {
			line := lines[index]
			if header, ok := tableColumns(line); ok {
				columns = header
				carried = header
				carriedFrom = page.Number
				continuation = false
				started = true // a header starts a fresh table
				// A fresh header starts a new table: a wrapped description
				// above its first row must not be appended to another row.
				previous = -1
				continue
			}
			if len(columns) == 0 {
				continue
			}
			if continuation && !started && runningHeaderPattern.MatchString(line.Text) {
				// A running page header ("PURCHASE ORDER", "Page 2 of 3") above
				// a continued table is not the table's end and not a line item.
				continue
			}
			if isTableEndLine(line, columns) {
				columns = nil
				continuation = false
				started = false
				previous = -1
				continue
			}
			cells := assignCells(line, columns)
			lineAnchor, quantityAnchor, priceAnchor := cells.anchors()
			rowAnchor := lineAnchor || quantityAnchor
			if !rowAnchor && priceAnchor && previous < 0 && !cells.descriptionEmpty() {
				// A first row may carry no line number and no quantity when the
				// layout omits those columns. Once a row exists, a price alone
				// is not enough to start another: a description line whose last
				// word happens to sit in the price column must continue the row
				// above it instead.
				rowAnchor = true
			}
			if !rowAnchor {
				if previous >= 0 && !cells.empty() && strings.TrimSpace(line.Text) != "" {
					appendDescription(&rows[previous], line.Text, line.Words, &line.Box)
				}
				continue
			}
			if !lineAnchor && previous >= 0 &&
				descriptionIsPlaceholder(rows[previous].Description.Value) &&
				!cells.descriptionEmpty() {
				// The numeric row printed only "Not Available" or a part number;
				// this line is its real description, even if a number in it lands
				// in the quantity column.
				appendDescription(&rows[previous], line.Text, line.Words, &line.Box)
				continue
			}
			started = true
			row := buildRow(page, line, cells, columns, minConfidence)
			if continuation {
				row.Status = apicontract.ExtractionStatusNeedsReview
				row.Note = appendNote(row.Note, fmt.Sprintf(
					"The column header is on page %d; this row continues that table without a repeated header. Confirm the line and its column positions.",
					carriedFrom))
			}
			rows = append(rows, row)
			previous = len(rows) - 1
			if derived == nil {
				if value, status, _ := normaliseDate(cells.deliveryRaw); status == apicontract.ExtractionStatusExtracted {
					derived = &deliveryEvidence{
						Value:      value,
						Raw:        cells.deliveryRaw,
						LineNumber: row.LineNumber.Value,
						Page:       page.Number,
						Words:      cells.delivery,
					}
				}
			}
			// A table row must carry at least one readable quantity, price or
			// printed amount. If none parses, the line is more likely prose
			// below the table than a malformed row; end the segment rather than
			// propose the rest of the page as line items. The row itself stays
			// as a reviewable abstention.
			if row.Quantity.Status != apicontract.ExtractionStatusExtracted &&
				row.UnitPrice.Status != apicontract.ExtractionStatusExtracted &&
				row.Amount.Status != apicontract.ExtractionStatusExtracted {
				columns = nil
				continuation = false
				started = false
				previous = -1
			}
		}
		// A page that ended while a table was still open continues on the next
		// page without a repeated header. A header printed at the bottom of a
		// page counts as open even before its first row is read.
		tableOpen = len(columns) > 0
	}
	markRowRisks(rows)
	return rows, derived
}

// appendDescription extends the previous row with a wrapped description line.
// A part number or placeholder on the numeric row is replaced by the real
// description when one follows; otherwise the printed text is kept whole.
func appendDescription(row *apicontract.ExtractionRow, text string, words []Word, box *apicontract.ExtractionBBox) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return
	}
	if descriptionIsPlaceholder(row.Description.Value) {
		row.Description.Value = text
		row.Description.Raw = text
		row.Description.BBox = bboxOf(words)
		row.BBox = unionExtractionBoxes(row.BBox, box)
		return
	}
	row.Description.Value = strings.TrimSpace(row.Description.Value + " " + text)
	row.Description.Raw = strings.TrimSpace(row.Description.Raw + " " + text)
	row.Description.BBox = unionExtractionBoxes(row.Description.BBox, bboxOf(words))
	row.BBox = unionExtractionBoxes(row.BBox, box)
}

// descriptionIsPlaceholder reports whether a description cell is only a
// placeholder or a part number. A single alphanumeric token is treated as a
// part number only when it carries a separator or at least three digits, so an
// ordinary one-word description ("BAT3") is not thrown away. The 32-character
// cap keeps a short sentence from being mistaken for a part number.
func descriptionIsPlaceholder(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch trimmed {
	case "", "not available", "n/a", "na", "not applicable", "none", "-", "--", "tbd", "no description":
		return true
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 1 || len(fields[0]) > 32 {
		return false
	}
	token := fields[0]
	digits, letters := 0, false
	separator := false
	for _, character := range token {
		switch {
		case character >= '0' && character <= '9':
			digits++
		case (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z'):
			letters = true
		case character == '-' || character == '/':
			separator = true
		}
	}
	if digits > 0 && letters && (separator || digits >= 3) {
		return true
	}
	return digits >= 4 && !letters
}

func containsLetter(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') {
			return true
		}
	}
	return false
}

type rowCells struct {
	lineNumber      []Word
	description     []Word
	quantity        []Word
	uom             []Word
	unitPrice       []Word
	amount          []Word
	delivery        []Word
	descriptionText string
	lineNumberRaw   string
	quantityRaw     string
	uomRaw          string
	unitPriceRaw    string
	amountRaw       string
	deliveryRaw     string
}

func (cells rowCells) descriptionEmpty() bool { return len(cells.description) == 0 }
func (cells rowCells) numericEmpty() bool {
	return len(cells.quantity) == 0 && len(cells.uom) == 0 &&
		len(cells.unitPrice) == 0 && len(cells.amount) == 0
}
func (cells rowCells) empty() bool {
	return len(cells.lineNumber) == 0 && cells.descriptionEmpty() && cells.numericEmpty()
}

// anchors decides whether a printed line starts a new line item. A line
// number, a readable quantity or a readable price is an anchor; a description
// that merely contains a number is not. The quantity is split from its unit
// first, so "1 (PK)" is a quantity anchor while "31 Jan 2027" is not.
func (cells rowCells) anchors() (lineAnchor, quantityAnchor, priceAnchor bool) {
	if value := trimLineNumber(cells.lineNumberRaw); value != "" && validLineNumber(value) {
		lineAnchor = true
	}
	if number, _ := splitQuantity(cells.quantityRaw); strings.TrimSpace(number) != "" {
		if _, status, _ := normaliseDecimal(number, decimalQuantity); status == apicontract.ExtractionStatusExtracted {
			quantityAnchor = true
		}
	}
	for _, raw := range []string{cells.unitPriceRaw, cells.amountRaw} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if _, status, _ := normaliseDecimal(raw, decimalMoney); status == apicontract.ExtractionStatusExtracted {
			priceAnchor = true
			break
		}
	}
	return lineAnchor, quantityAnchor, priceAnchor
}

func assignCells(line visualLine, columns []tableColumn) rowCells {
	grouped := make(map[columnKind][]Word, len(columns))
	boundaries := make([]float64, 0, len(columns)-1)
	for index := 0; index+1 < len(columns); index++ {
		boundaries = append(boundaries, (columns[index].XMax+columns[index+1].XMin)/2)
	}
	for _, word := range line.Words {
		center := (word.XMin + word.XMax) / 2
		assigned := columns[len(columns)-1].Kind
		for index, boundary := range boundaries {
			if center < boundary {
				assigned = columns[index].Kind
				break
			}
		}
		grouped[assigned] = append(grouped[assigned], word)
	}
	cells := rowCells{
		lineNumber:  grouped[columnLineNumber],
		description: grouped[columnDescription],
		quantity:    grouped[columnQuantity],
		uom:         grouped[columnUoM],
		unitPrice:   grouped[columnUnitPrice],
		amount:      grouped[columnAmount],
		delivery:    grouped[columnDeliveryDate],
	}
	cells.descriptionText = joinWords(cells.description)
	cells.lineNumberRaw = joinWords(cells.lineNumber)
	cells.quantityRaw = joinWords(cells.quantity)
	cells.uomRaw = joinWords(cells.uom)
	cells.unitPriceRaw = joinWords(cells.unitPrice)
	cells.amountRaw = joinWords(cells.amount)
	cells.deliveryRaw = joinWords(cells.delivery)
	return cells
}

func buildRow(page *Page, line visualLine, cells rowCells, columns []tableColumn, minConfidence float64) apicontract.ExtractionRow {
	quantityRaw, combinedUoM := splitQuantity(cells.quantityRaw)
	row := apicontract.ExtractionRow{
		LineNumber:  cellForLineNumber(cells.lineNumberRaw, page, cells.lineNumber, minConfidence),
		Description: textCell(cells.descriptionText, page, cells.description, minConfidence),
		Quantity:    quantityCell(quantityRaw, page, cells.quantity, minConfidence),
		UoM:         uomCell(cells.uomRaw, page, cells.uom, minConfidence),
		UnitPrice:   decimalCell(cells.unitPriceRaw, page, cells.unitPrice, decimalMoney, minConfidence),
		Amount:      decimalCell(cells.amountRaw, page, cells.amount, decimalMoney, minConfidence),
		Page:        page.Number,
		BBox:        &line.Box,
	}
	if row.UoM.Status == apicontract.ExtractionStatusAbsent && combinedUoM != "" {
		// "Qty (Unit)" is one column: the printed "1 (PK)" carries both.
		row.UoM = uomCell(combinedUoM, page, cells.quantity, minConfidence)
	}
	required := row.Description.Status == apicontract.ExtractionStatusExtracted &&
		row.Quantity.Status == apicontract.ExtractionStatusExtracted &&
		row.UoM.Status == apicontract.ExtractionStatusExtracted &&
		row.UnitPrice.Status == apicontract.ExtractionStatusExtracted
	lineColumnPresent := false
	for _, column := range columns {
		if column.Kind == columnLineNumber {
			lineColumnPresent = true
			break
		}
	}
	if lineColumnPresent && row.LineNumber.Status != apicontract.ExtractionStatusExtracted {
		required = false
	}
	// A low-confidence cell anywhere in the row -- including the amount or line
	// number, which are not required for "extracted" -- makes the whole line a
	// review item. Applying a line whose printed amount may be misread is not
	// assistance.
	for _, cell := range []apicontract.ExtractionCell{
		row.LineNumber, row.Description, row.Quantity, row.UoM, row.UnitPrice, row.Amount,
	} {
		if cell.Confidence != nil && *cell.Confidence < minConfidence {
			required = false
			row.Note = appendNote(row.Note,
				"One or more cells were read with low OCR confidence; confirm the line.")
			break
		}
	}
	if required {
		row.Status = apicontract.ExtractionStatusExtracted
	} else {
		row.Status = apicontract.ExtractionStatusNeedsReview
	}
	return row
}

// quantityCell normalises the numeric part of a quantity cell. The unit has
// already been split off by splitQuantity, so the multi-word grouping rule used
// for prices does not apply: a quantity printed as "1 (PK)" is the number 1.
func quantityCell(raw string, page *Page, words []Word, minConfidence float64) apicontract.ExtractionCell {
	cell := emptyCell()
	cell.Raw = raw
	trimmed := strings.Join(strings.Fields(raw), " ")
	if trimmed == "" {
		return cell
	}
	cell.Page = pointerTo(page.Number)
	cell.BBox = bboxOf(words)
	value, status, note := normaliseDecimal(trimmed, decimalQuantity)
	cell.Value = value
	cell.Status = status
	cell.Note = note
	applyConfidence(&cell, words, minConfidence)
	return cell
}

// splitQuantity separates a printed quantity from its unit. Both "12 PK" and
// the combined "1 (PK)" shape are understood; a date ("31 Jan 2027") is not a
// quantity and its month is not a unit.
func splitQuantity(raw string) (string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ""
	}
	unit := ""
	if open := strings.Index(value, "("); open >= 0 {
		if close := strings.LastIndex(value, ")"); close > open {
			unit = strings.TrimSpace(value[open+1 : close])
			value = strings.TrimSpace(value[:open] + " " + value[close+1:])
		}
	}
	fields := strings.Fields(value)
	if len(fields) >= 2 && isUnitToken(fields[len(fields)-1]) {
		unit = strings.TrimSpace(strings.Join([]string{unit, fields[len(fields)-1]}, " "))
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " "), unit
}

// isUnitToken accepts a short alphabetic unit ("PK", "EA", "Lot") and refuses a
// month name, so "31 Jan 2027" cannot turn its month into a UoM.
func isUnitToken(token string) bool {
	token = strings.Trim(token, "().,")
	if token == "" || len(token) > 12 {
		return false
	}
	for _, character := range token {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') {
			continue
		}
		return false
	}
	_, month := monthNames[strings.ToLower(token)]
	return !month
}

func trimLineNumber(raw string) string {
	return strings.TrimSpace(strings.TrimRight(raw, ".)"))
}

func cellForLineNumber(raw string, page *Page, words []Word, minConfidence float64) apicontract.ExtractionCell {
	cell := emptyCell()
	trimmed := trimLineNumber(raw)
	cell.Raw = raw
	if trimmed == "" {
		return cell
	}
	cell.Value = trimmed
	cell.Page = pointerTo(page.Number)
	cell.BBox = bboxOf(words)
	if validLineNumber(trimmed) {
		cell.Status = apicontract.ExtractionStatusExtracted
	} else {
		cell.Status = apicontract.ExtractionStatusNeedsReview
		cell.Note = "The printed line number is not a plain number."
	}
	applyConfidence(&cell, words, minConfidence)
	return cell
}

var lineNumberPattern = regexp.MustCompile(`^\d{1,4}(?:\.\d{1,3})?$`)

func validLineNumber(value string) bool {
	return lineNumberPattern.MatchString(value)
}

func textCell(raw string, page *Page, words []Word, minConfidence float64) apicontract.ExtractionCell {
	cell := emptyCell()
	cell.Raw = raw
	trimmed := strings.Join(strings.Fields(raw), " ")
	if trimmed == "" {
		return cell
	}
	cell.Value = trimmed
	cell.Page = pointerTo(page.Number)
	cell.BBox = bboxOf(words)
	cell.Status = apicontract.ExtractionStatusExtracted
	applyConfidence(&cell, words, minConfidence)
	return cell
}

func uomCell(raw string, page *Page, words []Word, minConfidence float64) apicontract.ExtractionCell {
	cell := emptyCell()
	cell.Raw = raw
	trimmed := strings.Join(strings.Fields(raw), " ")
	trimmed = strings.Trim(strings.TrimSpace(trimmed), "()")
	trimmed = strings.TrimRight(trimmed, ".,;")
	if trimmed == "" {
		return cell
	}
	cell.Value = trimmed
	cell.Page = pointerTo(page.Number)
	cell.BBox = bboxOf(words)
	if len([]rune(trimmed)) <= 20 && !containsDigit(trimmed) {
		cell.Status = apicontract.ExtractionStatusExtracted
	} else {
		cell.Status = apicontract.ExtractionStatusNeedsReview
		cell.Note = "The printed unit of measure is not a short unit name."
	}
	applyConfidence(&cell, words, minConfidence)
	return cell
}

func containsDigit(value string) bool {
	for _, character := range value {
		if character >= '0' && character <= '9' {
			return true
		}
	}
	return false
}

type decimalKind int

const (
	decimalQuantity decimalKind = iota
	decimalMoney
)

var decimalPattern = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)

func decimalCell(raw string, page *Page, words []Word, kind decimalKind, minConfidence float64) apicontract.ExtractionCell {
	cell := emptyCell()
	cell.Raw = raw
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return cell
	}
	cell.Page = pointerTo(page.Number)
	cell.BBox = bboxOf(words)
	if len(words) > 1 {
		// OCR regularly splits one printed number into several words. Removing
		// the spaces blindly can turn a split decimal ("1250" "00") into a
		// number ten times too large, so only a grouping-shaped token sequence
		// is joined; anything else abstains with the printed text kept.
		joined, ok := groupedNumericText(words)
		if !ok {
			cell.Value = trimmed
			cell.Status = apicontract.ExtractionStatusNeedsReview
			cell.Note = "The number is printed as separate tokens that do not form a clear grouping; confirm the printed value."
			applyConfidence(&cell, words, minConfidence)
			return cell
		}
		trimmed = joined
	}
	value, status, note := normaliseDecimal(trimmed, kind)
	cell.Value = value
	cell.Status = status
	cell.Note = note
	applyConfidence(&cell, words, minConfidence)
	return cell
}

// groupedNumericText joins a multi-word numeric cell only when the words form
// a thousands-grouped number: optional currency tokens (a code, a symbol, or a
// prefix/suffix attached to the number), one to three leading digits, then
// groups of exactly three digits (the last may carry a decimal fraction), and
// an optional trailing punctuation token. "1 250.00" and "Rp145,760,000.00 IDR"
// are grouping; "1250" "00" and "10" "." "5" are splits whose meaning is
// unknown and therefore abstain.
func groupedNumericText(words []Word) (string, bool) {
	tokens := make([]string, 0, len(words))
	for _, word := range words {
		text := strings.TrimSpace(word.Text)
		if text != "" {
			tokens = append(tokens, text)
		}
	}
	if len(tokens) < 2 {
		return "", false
	}
	start, end := 0, len(tokens)
	for start < end && isCurrencyToken(tokens[start]) {
		start++
	}
	for end > start && isCurrencyToken(tokens[end-1]) {
		end--
	}
	if end > start && isPunctuationToken(tokens[end-1]) {
		end--
	}
	if end-start < 1 {
		return "", false
	}
	body := make([]string, 0, end-start)
	for _, token := range tokens[start:end] {
		body = append(body, stripCurrencyAffixes(token))
	}
	if len(body) == 1 {
		// The only split was between a currency token and the number, or a
		// currency affix and the number; normaliseDecimal reads the body.
		return body[0], true
	}
	if !leadingGroupPattern.MatchString(body[0]) {
		return "", false
	}
	for index, token := range body[1:] {
		last := index == len(body)-2
		if last {
			if !trailingGroupPattern.MatchString(token) {
				return "", false
			}
			continue
		}
		if len(token) != 3 || !digitsOnly(token) {
			return "", false
		}
	}
	return strings.Join(body, ""), true
}

func stripCurrencyAffixes(token string) string {
	value := strings.TrimSpace(token)
	value = currencyPrefix.ReplaceAllString(value, "")
	value = currencySuffix.ReplaceAllString(value, "")
	return value
}

var (
	leadingGroupPattern  = regexp.MustCompile(`^\d{1,3}$`)
	trailingGroupPattern = regexp.MustCompile(`^\d{3}(?:[.,]\d{1,6})?$`)
)

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

var currencyTokenPattern = regexp.MustCompile(
	`(?i)^(rp|idr|usd|eur|gbp|jpy|sgd|myr|thb|hkd|aud|nzd|chf|cad|krw|inr|php|vnd|sar|aed)\.?$`)

func isCurrencyToken(token string) bool {
	if currencyTokenPattern.MatchString(token) {
		return true
	}
	switch token {
	case "$", "€", "£", "¥", "₩", "₹", "฿":
		return true
	}
	return false
}

func isPunctuationToken(token string) bool {
	return strings.Trim(token, ".,-–—") == ""
}

var currencyPrefix = regexp.MustCompile(`(?i)^(rp|idr|usd|eur|gbp|jpy|sgd|myr|thb|hkd|aud|nzd|chf|cad|krw|inr|php|vnd|sar|aed)\.?\s*`)
var currencySuffix = regexp.MustCompile(`(?i)\s*(rp|idr|usd|eur|gbp|jpy|sgd|myr|thb|hkd|aud|nzd|chf|cad|krw|inr|php|vnd|sar|aed)\.?$`)

// normaliseDecimal turns printed money/quantity text into a plain decimal with
// a dot. It never guesses the thousands-versus-decimal ambiguity: a single
// separator followed by exactly three digits is returned for review.
func normaliseDecimal(raw string, kind decimalKind) (string, string, string) {
	value := strings.TrimSpace(raw)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.ReplaceAll(value, " ", "")
	value = strings.NewReplacer(
		"$", "", "€", "", "£", "", "¥", "", "₩", "", "₹", "", "฿", "",
		"Rp", "", "RP", "", "rp", "",
	).Replace(value)
	value = currencyPrefix.ReplaceAllString(value, "")
	value = currencySuffix.ReplaceAllString(value, "")
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ",-")
	value = strings.TrimSuffix(value, ".-")
	value = strings.TrimSuffix(value, "-")
	value = strings.TrimPrefix(value, "+")
	if value == "" {
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed cell has no number."
	}

	negative := strings.HasPrefix(value, "-")
	body := strings.TrimPrefix(value, "-")
	if strings.Contains(body, "-") {
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed cell has a misplaced minus sign."
	}
	for _, character := range body {
		if (character < '0' || character > '9') && character != '.' && character != ',' {
			return raw, apicontract.ExtractionStatusNeedsReview, "The printed cell is not a number."
		}
	}
	if body == "" || body == "." || body == "," {
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed cell is not a number."
	}

	dots := strings.Count(body, ".")
	commas := strings.Count(body, ",")
	fractionLength := 0

	switch {
	case dots > 0 && commas > 0:
		// Both separators: the rightmost is the decimal point, the other is
		// grouping. 1,250.00 and 1.250,00 both resolve exactly, and a
		// malformed grouping (1,2.30) abstains rather than inventing digits.
		if strings.LastIndex(body, ",") > strings.LastIndex(body, ".") {
			integer := body[:strings.LastIndex(body, ",")]
			if strings.Contains(integer, ".") && !validThousandsGrouping(integer, '.') {
				return raw, apicontract.ExtractionStatusNeedsReview,
					"The thousands separators are not grouped in threes; confirm the printed value."
			}
			fractionLength = len(body) - strings.LastIndex(body, ",") - 1
			body = strings.ReplaceAll(body, ".", "")
			body = strings.ReplaceAll(body, ",", ".")
		} else {
			integer := body[:strings.LastIndex(body, ".")]
			if strings.Contains(integer, ",") && !validThousandsGrouping(integer, ',') {
				return raw, apicontract.ExtractionStatusNeedsReview,
					"The thousands separators are not grouped in threes; confirm the printed value."
			}
			fractionLength = len(body) - strings.LastIndex(body, ".") - 1
			body = strings.ReplaceAll(body, ",", "")
		}
	case dots > 0 && commas == 0:
		if dots > 1 {
			// 1.250.000 is grouping only, but 1.2.3 is not a number.
			if !validThousandsGrouping(body, '.') {
				return raw, apicontract.ExtractionStatusNeedsReview,
					"The thousands separators are not grouped in threes; confirm the printed value."
			}
			body = strings.ReplaceAll(body, ".", "")
			break
		}
		fractionLength = len(body) - strings.LastIndex(body, ".") - 1
		if fractionLength == 3 {
			return raw, apicontract.ExtractionStatusNeedsReview,
				"The separator could mean thousands or a decimal point; confirm the printed value."
		}
	case commas > 0 && dots == 0:
		if commas > 1 {
			// 1,250,000 is grouping only, but 1,25,000 is not a number.
			if !validThousandsGrouping(body, ',') {
				return raw, apicontract.ExtractionStatusNeedsReview,
					"The thousands separators are not grouped in threes; confirm the printed value."
			}
			body = strings.ReplaceAll(body, ",", "")
			break
		}
		fractionLength = len(body) - strings.LastIndex(body, ",") - 1
		if fractionLength == 3 {
			return raw, apicontract.ExtractionStatusNeedsReview,
				"The separator could mean thousands or a decimal point; confirm the printed value."
		}
		body = strings.ReplaceAll(body, ",", ".")
	}
	if !decimalPattern.MatchString(body) {
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed cell is not a number."
	}

	parts := strings.SplitN(body, ".", 2)
	integer := strings.TrimLeft(parts[0], "0")
	if integer == "" {
		integer = "0"
	}
	if len(parts) == 2 {
		body = integer + "." + parts[1]
		fractionLength = len(parts[1])
	} else {
		body = integer
	}
	if negative {
		body = "-" + body
	}
	if negative {
		return raw, apicontract.ExtractionStatusNeedsReview, "A negative amount is not a valid order value."
	}
	if kind == decimalQuantity {
		if isZero(body) {
			return raw, apicontract.ExtractionStatusNeedsReview, "A zero quantity is not a valid order line."
		}
		if fractionLength > 4 {
			return raw, apicontract.ExtractionStatusNeedsReview, "The quantity has more than four decimal places."
		}
	} else if fractionLength > 6 {
		return raw, apicontract.ExtractionStatusNeedsReview, "The amount has more decimal places than this release can read."
	}
	return body, apicontract.ExtractionStatusExtracted, ""
}

func isZero(value string) bool {
	for _, character := range value {
		if character != '0' && character != '.' {
			return false
		}
	}
	return true
}

func emptyCell() apicontract.ExtractionCell {
	return apicontract.ExtractionCell{Status: apicontract.ExtractionStatusAbsent}
}

func pointerTo(value int) *int { return &value }

func unionExtractionBoxes(left, right *apicontract.ExtractionBBox) *apicontract.ExtractionBBox {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	x := minFloat(left.X, right.X)
	y := minFloat(left.Y, right.Y)
	xMax := maxFloat(left.X+left.Width, right.X+right.Width)
	yMax := maxFloat(left.Y+left.Height, right.Y+right.Height)
	return &apicontract.ExtractionBBox{X: x, Y: y, Width: xMax - x, Height: yMax - y}
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

// markRowRisks flags duplicate and missing line numbers. It never dedupes,
// merges, renumbers or drops a row: a duplicate delivered as two review items
// is recoverable, a silently removed row is not. A gap in numbered lines is
// flagged on the row after the gap, never filled with an invented row.
func markRowRisks(rows []apicontract.ExtractionRow) {
	contentCounts := map[string]int{}
	keys := make([]string, len(rows))
	for index := range rows {
		row := &rows[index]
		key := strings.Join([]string{
			row.LineNumber.Value,
			strings.ToLower(row.Description.Value),
			row.Quantity.Value,
			row.UnitPrice.Value,
			row.Amount.Value,
		}, "\x1f")
		if row.LineNumber.Value == "" && row.Description.Value == "" {
			continue
		}
		keys[index] = key
		contentCounts[key]++
	}
	lineCounts := map[int]int{}
	lineNumbers := make([]int, len(rows))
	for index := range rows {
		number, ok := wholeLineNumber(rows[index])
		if !ok {
			lineNumbers[index] = 0
			continue
		}
		lineNumbers[index] = number
		lineCounts[number]++
	}

	previous := 0
	previousPrinted := ""
	for index := range rows {
		row := &rows[index]
		if keys[index] != "" && contentCounts[keys[index]] > 1 {
			row.Status = apicontract.ExtractionStatusNeedsReview
			row.Note = appendNote(row.Note,
				"This line appears more than once in the document; confirm that both printed lines are separate.")
		}
		number := lineNumbers[index]
		if number == 0 {
			continue
		}
		if lineCounts[number] > 1 {
			row.Status = apicontract.ExtractionStatusNeedsReview
			row.Note = appendNote(row.Note, fmt.Sprintf(
				"Line number %d appears more than once; confirm which printed line each is.", number))
		}
		if previous > 0 && number > previous+1 {
			row.Status = apicontract.ExtractionStatusNeedsReview
			row.Note = appendNote(row.Note, fmt.Sprintf(
				"Line number %s is not printed between line %s and line %d; a line may be missing from the rows this extractor read.",
				missingRange(previous, number), previousPrinted, number))
		}
		if number > previous {
			previous = number
			previousPrinted = row.LineNumber.Value
		}
	}
}

// wholeLineNumber returns the printed line number only when it is a plain
// integer. A sub-line ("2.1") is a different numbering scheme and is not used
// for gap detection.
func wholeLineNumber(row apicontract.ExtractionRow) (int, bool) {
	value := strings.TrimSpace(row.LineNumber.Value)
	if value == "" || strings.Contains(value, ".") {
		return 0, false
	}
	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

func missingRange(previous, current int) string {
	if current == previous+2 {
		return strconv.Itoa(previous + 1)
	}
	return fmt.Sprintf("%d–%d", previous+1, current-1)
}
