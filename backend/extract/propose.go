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

// Issue #477: propose header fields and table rows from positioned words.
//
// The rules are deliberately conservative. A label with no value is `absent`;
// a value that cannot be normalised, a date that could be read two ways, a
// repeated label with a different value, and a row missing a required cell are
// `needs_review`. Nothing is inferred from arithmetic (an amount is what the
// page printed, never quantity x price) and nothing is OCRed: a page with no
// words is `absent` for every field.
//
// Layout handling is a heuristic, not universal parsing. It understands a
// label/value header and a table whose column headers appear on the page; a
// continued table without a repeated header is left unproposed rather than
// guessed. The benchmark ticket (#476) requires real authorised samples before
// any accuracy claim, and none of this is that claim.

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
	result.Fields = proposeFields(headerLines, minConfidence)
	result.Rows = proposeRows(pages, minConfidence)
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
	Page   *Page
	Words  []Word
	Text   string
	Center float64
	Box    apicontract.ExtractionBBox
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
			Page:   page,
			Words:  current,
			Text:   joinWords(current),
			Center: center,
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

func headerArea(pages []Page) []visualLine {
	if len(pages) == 0 {
		return nil
	}
	lines := linesOf(&pages[0])
	for index, line := range lines {
		if _, ok := tableColumns(line); ok {
			return lines[:index]
		}
	}
	// No table header on the first page: the header block is the top half.
	// A label lower than that is more likely body text than a PO header.
	limit := pages[0].Height * 0.55
	header := make([]visualLine, 0, len(lines))
	for _, line := range lines {
		if line.Box.Y >= limit {
			break
		}
		header = append(header, line)
	}
	return header
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
		`(?i)\b(?:p\.?\s*o\.?\s*(?:no|number|#)|purchase\s+order\s+(?:no|number|#)|nomor\s+p\.?\s*o\.?|no\.?\s+p\.?\s*o\.?)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldPODate: regexp.MustCompile(
		`(?i)\b(?:p\.?\s*o\.?\s+date|purchase\s+order\s+date|tanggal\s+p\.?\s*o\.?|order\s+date)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldRequestedDeliveryDate: regexp.MustCompile(
		`(?i)\b(?:requested\s+delivery\s+date|required\s+delivery\s+date|delivery\s+date|tanggal\s+(?:kirim|pengiriman)|ship\s+date|required\s+date)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldCurrency: regexp.MustCompile(
		`(?i)\b(?:currency|mata\s+uang)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldPaymentTerms: regexp.MustCompile(
		`(?i)\b(?:payment\s+terms?|terms\s+of\s+payment|syarat\s+pembayaran)\s*[:#-]?\s*(.*)$`),
	apicontract.ExtractionFieldIncoterms: regexp.MustCompile(
		`(?i)\b(?:incoterms?|terms\s+of\s+delivery|delivery\s+terms|syarat\s+pengiriman)\s*[:#-]?\s*(.*)$`),
}

func proposeFields(lines []visualLine, minConfidence float64) []apicontract.ExtractionField {
	matches := make(map[string][]fieldMatch, len(fieldPatterns))
	for _, line := range lines {
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
			matches[key] = append(matches[key], fieldMatch{
				words: wordsInRange(spans, start, end),
				raw:   raw,
				page:  line.Page.Number,
			})
		}
	}

	fields := make([]apicontract.ExtractionField, 0, len(apicontract.ExtractionFieldKeys))
	for _, key := range apicontract.ExtractionFieldKeys {
		fields = append(fields, proposeField(key, matches[key], lines, minConfidence))
	}
	return fields
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

func proposeField(key string, matches []fieldMatch, headerLines []visualLine, minConfidence float64) apicontract.ExtractionField {
	field := apicontract.ExtractionField{
		Key: key,
		ExtractionCell: apicontract.ExtractionCell{
			Status: apicontract.ExtractionStatusAbsent,
		},
	}
	if key == apicontract.ExtractionFieldCurrency && len(matches) == 0 {
		if match, ok := fallbackCurrency(headerLines); ok {
			matches = []fieldMatch{match}
		}
	}
	if len(matches) == 0 {
		return field
	}
	chosen := matches[0]
	for _, match := range matches {
		if strings.TrimSpace(match.raw) != "" {
			chosen = match
			break
		}
	}
	field.Raw = chosen.raw
	field.Page = pointerTo(chosen.page)
	field.BBox = bboxOf(chosen.words)

	distinct := map[string]bool{}
	for _, match := range matches {
		if strings.TrimSpace(match.raw) == "" {
			continue
		}
		distinct[strings.ToUpper(strings.Join(strings.Fields(match.raw), " "))] = true
	}
	value, status, note := normaliseField(key, chosen.raw)
	field.Value = value
	field.Status = status
	field.Note = note
	if strings.TrimSpace(chosen.raw) == "" {
		field.Status = apicontract.ExtractionStatusAbsent
		field.Page = nil
		field.BBox = nil
		field.Note = "The label was found but no value followed it."
		return field
	}
	if len(distinct) > 1 {
		field.Status = apicontract.ExtractionStatusNeedsReview
		field.Note = fmt.Sprintf("Another value was printed for this field (%q); confirm which one applies.", firstDifferent(matches, chosen.raw))
	}
	applyConfidence(&field.ExtractionCell, chosen.words, minConfidence)
	return field
}

func firstDifferent(matches []fieldMatch, chosen string) string {
	for _, match := range matches {
		if strings.TrimSpace(match.raw) != strings.TrimSpace(chosen) {
			return strings.TrimSpace(match.raw)
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

func fallbackCurrency(lines []visualLine) (fieldMatch, bool) {
	seen := map[string]bool{}
	values := make([]string, 0, 2)
	words := make([]Word, 0, 4)
	page := 0
	for _, line := range lines {
		for _, word := range line.Words {
			token := strings.ToUpper(strings.Trim(word.Text, ".,:;()"))
			if currencyCandidates[token] && !seen[token] {
				seen[token] = true
				values = append(values, token)
				words = append(words, word)
				if page == 0 {
					page = line.Page.Number
				}
			}
		}
	}
	if len(values) == 0 {
		return fieldMatch{}, false
	}
	return fieldMatch{words: words, raw: strings.Join(values, " / "), page: page}, true
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

func validPONumber(value string) bool {
	return poNumberPattern.MatchString(value)
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

var dayMonthYear = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})$`)
var isoDate = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
var dayNameMonthYear = regexp.MustCompile(`^(\d{1,2})\s+([A-Za-z]+)\s+(\d{4})$`)
var monthDayYear = regexp.MustCompile(`^([A-Za-z]+)\s+(\d{1,2}),?\s+(\d{4})$`)
var monthYearDay = regexp.MustCompile(`^([A-Za-z]+)\s+(\d{1,2})\s+(\d{4})$`)

func normaliseDate(raw string) (string, string, string) {
	value := strings.TrimSpace(strings.Trim(raw, ".,;"))
	if match := isoDate.FindStringSubmatch(value); match != nil {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		if validDate(year, month, day) {
			return formatDate(year, month, day), apicontract.ExtractionStatusExtracted, ""
		}
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed date is not a real calendar date."
	}
	if match := dayMonthYear.FindStringSubmatch(value); match != nil {
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
	if match := dayNameMonthYear.FindStringSubmatch(value); match != nil {
		day, _ := strconv.Atoi(match[1])
		month, ok := monthNames[strings.ToLower(match[2])]
		year, _ := strconv.Atoi(match[3])
		if ok && validDate(year, int(month), day) {
			return formatDate(year, int(month), day), apicontract.ExtractionStatusExtracted, ""
		}
		return raw, apicontract.ExtractionStatusNeedsReview, "The printed date could not be read."
	}
	for _, pattern := range []*regexp.Regexp{monthDayYear, monthYearDay} {
		if match := pattern.FindStringSubmatch(value); match != nil {
			month, ok := monthNames[strings.ToLower(match[1])]
			day, _ := strconv.Atoi(match[2])
			year, _ := strconv.Atoi(match[3])
			if ok && validDate(year, int(month), day) {
				return formatDate(year, int(month), day), apicontract.ExtractionStatusExtracted, ""
			}
			return raw, apicontract.ExtractionStatusNeedsReview, "The printed date could not be read."
		}
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
	if len(trimmed) == 3 && isLetters(strings.ToUpper(trimmed)) {
		return strings.ToUpper(trimmed), apicontract.ExtractionStatusExtracted, ""
	}
	return raw, apicontract.ExtractionStatusNeedsReview, "The printed currency is not a three-letter code; confirm the currency."
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

func isLetters(value string) bool {
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return value != ""
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
)

type tableColumn struct {
	Kind columnKind
	XMin float64
	XMax float64
}

var totalLinePattern = regexp.MustCompile(
	`(?i)^\s*(sub\s*total|grand\s*total|total|jumlah|amount\s+due|balance\s+due|ppn|vat|tax|discount|pph)\b`)
var footerLinePattern = regexp.MustCompile(
	`(?i)^\s*(notes?|remarks?|terms\s+and\s+conditions|signature|prepared\s+by|approved\s+by|authorized\s+by|received\s+by|page\s+\d+)\b`)
var footerLabelPattern = regexp.MustCompile(
	`(?i)^\s*(requester|requested\s+by|buyer|purchaser|attention|attn|to|from|ship\s+to|bill\s+to|delivery\s+address|invoice|payment|bank|account|notes?|remarks?|terms|conditions|signature|prepared\s+by|approved\s+by|authorized\s+by|received\s+by|page)\b\s*:`)

func tableColumns(line visualLine) ([]tableColumn, bool) {
	columns := make([]tableColumn, 0, 8)
	for index := 0; index < len(line.Words); index++ {
		first := line.Words[index]
		token := strings.ToLower(strings.Trim(first.Text, ".:#()-/"))
		next := ""
		if index+1 < len(line.Words) {
			next = strings.ToLower(strings.Trim(line.Words[index+1].Text, ".:#()-/"))
		}
		kind := columnUnknown
		xMin, xMax := first.XMin, first.XMax
		switch {
		case token == "unit" && (next == "price" || next == "cost" || next == "rate"):
			kind = columnUnitPrice
			xMax = line.Words[index+1].XMax
			index++ // the pair is one column
		case token == "harga" && (next == "satuan" || next == "unit"):
			kind = columnUnitPrice
			xMax = line.Words[index+1].XMax
			index++
		case token == "line" && next == "total":
			kind = columnAmount
			xMax = line.Words[index+1].XMax
			index++
		case token == "no" || token == "nomor" || token == "line" || token == "seq" || token == "#":
			kind = columnLineNumber
		case token == "description" || token == "deskripsi" || token == "uraian" ||
			token == "goods" || token == "product" || token == "particulars" || token == "item":
			kind = columnDescription
		case token == "qty" || token == "quantity" || token == "kuantitas" || token == "jumlah":
			kind = columnQuantity
		case token == "uom" || token == "satuan" || token == "unit" || token == "u/m":
			kind = columnUoM
		case token == "price" || token == "harga" || token == "rate" || token == "cost":
			kind = columnUnitPrice
		case token == "amount" || token == "total" || token == "subtotal" || token == "nilai":
			kind = columnAmount
		}
		if kind == columnUnknown {
			continue
		}
		columns = append(columns, tableColumn{Kind: kind, XMin: xMin, XMax: xMax})
	}
	if len(columns) == 0 {
		return nil, false
	}
	sort.SliceStable(columns, func(left, right int) bool { return columns[left].XMin < columns[right].XMin })

	// Keep the leftmost occurrence of each kind; a second "price" column is
	// not something this reader can disambiguate.
	seen := map[columnKind]bool{}
	unique := make([]tableColumn, 0, len(columns))
	for _, column := range columns {
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

func proposeRows(pages []Page, minConfidence float64) []apicontract.ExtractionRow {
	rows := make([]apicontract.ExtractionRow, 0, 16)
	var previous *apicontract.ExtractionRow
	var carried []tableColumn
	carriedFrom := 0
	tableOpen := false
	for pageIndex := range pages {
		page := &pages[pageIndex]
		lines := linesOf(page)
		headerIndex := -1
		var columns []tableColumn
		for index, line := range lines {
			if found, ok := tableColumns(line); ok {
				headerIndex = index
				columns = found
				break
			}
		}
		continuation := false
		switch {
		case headerIndex >= 0:
			carried = columns
			carriedFrom = page.Number
			// A fresh table header starts a new table: a wrapped description
			// above its first row must not be appended to another page's row.
			previous = nil
		case tableOpen && len(carried) > 0:
			// The previous page ended inside a table and this page repeats no
			// column header. The columns are carried, and every row read this
			// way is needs_review: carrying positions is an inference.
			columns = carried
			continuation = true
		default:
			tableOpen = false
			previous = nil
			continue
		}

		start := headerIndex + 1
		if continuation {
			start = 0
		}
		ended := false
		started := headerIndex >= 0
		pageRows := 0
		for index := start; index < len(lines); index++ {
			line := lines[index]
			if _, isHeader := tableColumns(line); isHeader {
				continue
			}
			if totalLinePattern.MatchString(line.Text) || footerLinePattern.MatchString(line.Text) {
				if continuation && !started {
					// A running page header ("Page 2 of 3") above a continued
					// table is not the table's end.
					continue
				}
				ended = true
				break
			}
			cells := assignCells(line, columns)
			if cells.empty() || cells.descriptionEmpty() {
				continue
			}
			if footerLabelPattern.MatchString(cells.descriptionText) ||
				footerLabelPattern.MatchString(cells.lineNumberRaw) {
				if continuation && !started {
					continue
				}
				ended = true
				break
			}
			if cells.numericEmpty() {
				// A wrapped description continues the previous row. This is a
				// layout fact, not a new line item. The proposal must carry the
				// whole text and a region covering every wrapped line: a value
				// whose raw or bbox shows only the first line is worse than an
				// abstention.
				if continuation && !started {
					// Above a continued table sits a page header, not the
					// previous line's description.
					continue
				}
				if previous != nil {
					full := strings.TrimSpace(previous.Description.Value + " " + cells.descriptionText)
					previous.Description.Value = full
					previous.Description.Raw = strings.TrimSpace(previous.Description.Raw + " " + cells.descriptionText)
					previous.Description.BBox = unionExtractionBoxes(previous.Description.BBox, cells.descriptionBox)
					previous.BBox = unionExtractionBoxes(previous.BBox, cells.descriptionBox)
					continue
				}
				continue
			}
			if continuation && !started {
				// Before a continued table starts: skip the running page header
				// at the top of the page, then require a numeric token. A header
				// that happens to print a number in the amount column
				// ("Page 2") must not become a line item.
				if line.Box.Y < page.Height*0.10 {
					continue
				}
				if !hasNumericToken(cells) {
					continue
				}
				started = true
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
			previous = &rows[len(rows)-1]
			pageRows++
			// A table row must carry at least one readable quantity or price.
			// If neither parses, the line is more likely prose below the table
			// than a malformed row; stop rather than propose the rest of the
			// page as line items. The row itself stays as a reviewable
			// abstention.
			if row.Quantity.Status != apicontract.ExtractionStatusExtracted &&
				row.UnitPrice.Status != apicontract.ExtractionStatusExtracted {
				ended = true
				break
			}
		}
		tableOpen = pageRows > 0 && !ended
	}
	markRowRisks(rows)
	return rows
}

// hasNumericToken reports whether a line's quantity, unit-price or amount
// column carries at least one digit. It is the start test for a continued
// table: a running page header above the table usually has text in the
// description column and, at most, a page number in the amount column, which
// the top-margin skip keeps out. A first row whose quantity was unreadable but
// whose amount is printed still starts the table instead of being dropped.
func hasNumericToken(cells rowCells) bool {
	if containsDigit(cells.quantityRaw) || containsDigit(cells.unitPriceRaw) {
		return true
	}
	return containsDigit(cells.amountRaw) && !containsLetter(cells.amountRaw)
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
	descriptionText string
	descriptionBox  *apicontract.ExtractionBBox
	lineNumberRaw   string
	quantityRaw     string
	uomRaw          string
	unitPriceRaw    string
	amountRaw       string
}

func (cells rowCells) descriptionEmpty() bool { return len(cells.description) == 0 }
func (cells rowCells) numericEmpty() bool {
	return len(cells.quantity) == 0 && len(cells.uom) == 0 &&
		len(cells.unitPrice) == 0 && len(cells.amount) == 0
}
func (cells rowCells) empty() bool {
	return len(cells.lineNumber) == 0 && cells.descriptionEmpty() && cells.numericEmpty()
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
	}
	cells.descriptionText = joinWords(cells.description)
	cells.descriptionBox = bboxOf(cells.description)
	cells.lineNumberRaw = joinWords(cells.lineNumber)
	cells.quantityRaw = joinWords(cells.quantity)
	cells.uomRaw = joinWords(cells.uom)
	cells.unitPriceRaw = joinWords(cells.unitPrice)
	cells.amountRaw = joinWords(cells.amount)
	return cells
}

func buildRow(page *Page, line visualLine, cells rowCells, columns []tableColumn, minConfidence float64) apicontract.ExtractionRow {
	row := apicontract.ExtractionRow{
		LineNumber:  cellForLineNumber(cells.lineNumberRaw, page, cells.lineNumber, minConfidence),
		Description: textCell(cells.descriptionText, page, cells.description, minConfidence),
		Quantity:    decimalCell(cells.quantityRaw, page, cells.quantity, decimalQuantity, minConfidence),
		UoM:         uomCell(cells.uomRaw, page, cells.uom, minConfidence),
		UnitPrice:   decimalCell(cells.unitPriceRaw, page, cells.unitPrice, decimalMoney, minConfidence),
		Amount:      decimalCell(cells.amountRaw, page, cells.amount, decimalMoney, minConfidence),
		Page:        page.Number,
		BBox:        &line.Box,
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

func cellForLineNumber(raw string, page *Page, words []Word, minConfidence float64) apicontract.ExtractionCell {
	cell := emptyCell()
	trimmed := strings.TrimSpace(strings.TrimRight(raw, ".)"))
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
// a thousands-grouped number: an optional currency token, one to three leading
// digits, then groups of exactly three digits (the last may carry a decimal
// fraction), and an optional trailing punctuation token. "1 250.00" is
// grouping; "1250 00" and "10" "." "5" are splits whose meaning is unknown.
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
	if isCurrencyToken(tokens[start]) {
		start++
	}
	if end > start && isPunctuationToken(tokens[end-1]) {
		end--
	}
	if end-start < 1 {
		return "", false
	}
	body := tokens[start:end]
	if len(body) == 1 {
		// "Rp 1.250,00": the only split is between the currency and the
		// number, which normaliseDecimal already understands exactly.
		return strings.Join(tokens, ""), true
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
	return strings.Join(tokens, ""), true
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
