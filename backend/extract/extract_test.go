package extract

import (
	"strconv"
	"strings"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #477: the pure text-PDF reader.
//
// These tests feed the reader the exact shape Poppler prints for a synthetic
// page: word boxes, not text lines. The fixture XML is generated in code, so
// no customer document is ever committed. They hold the contract the rest of
// the slice depends on: a proposal carries its page and region, uncertainty is
// `needs_review` rather than a guess, an image-only page is `absent` rather
// than OCR, and a table that cannot be read produces no invented rows.

type testWord struct {
	text   string
	x      float64
	y      float64
	width  float64
	height float64
}

// bboxXML writes the subset of pdftotext -bbox-layout this reader consumes.
// Putting every word in its own block is deliberate: the reader must group
// words by their y position, not trust Poppler's block boundaries (a real
// writer can emit one block per text run).
func bboxXML(pages ...[]testWord) []byte {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml"><body><doc>
`)
	for _, words := range pages {
		builder.WriteString(`<page width="595.000000" height="842.000000">` + "\n")
		for _, item := range words {
			builder.WriteString(`<flow><block><line><word xMin="`)
			builder.WriteString(strconv.FormatFloat(item.x, 'f', 3, 64))
			builder.WriteString(`" yMin="`)
			builder.WriteString(strconv.FormatFloat(item.y, 'f', 3, 64))
			builder.WriteString(`" xMax="`)
			builder.WriteString(strconv.FormatFloat(item.x+item.width, 'f', 3, 64))
			builder.WriteString(`" yMax="`)
			builder.WriteString(strconv.FormatFloat(item.y+item.height, 'f', 3, 64))
			builder.WriteString(`">`)
			builder.WriteString(escapeXML(item.text))
			builder.WriteString(`</word></line></block></flow>` + "\n")
		}
		builder.WriteString("</page>\n")
	}
	builder.WriteString("</doc></body></html>\n")
	return []byte(builder.String())
}

func escapeXML(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(value)
}

func word(text string, x, y, width float64) testWord {
	return testWord{text: text, x: x, y: y, width: width, height: 10}
}

const pageWidth = 595.0

func headerWords() []testWord {
	return []testWord{
		word("PURCHASE", 50, 60, 80),
		word("ORDER", 140, 60, 50),
		word("PO", 50, 90, 14),
		word("No:", 67, 90, 16),
		word("4500123456", 86, 90, 60),
		word("PO", 50, 105, 14),
		word("Date:", 67, 105, 25),
		word("2026-10-12", 95, 105, 60),
		word("Currency:", 50, 120, 45),
		word("USD", 98, 120, 22),
		word("Payment", 50, 135, 40),
		word("Terms:", 93, 135, 32),
		word("30", 128, 135, 12),
		word("days", 143, 135, 22),
		word("after", 168, 135, 22),
		word("delivery", 193, 135, 40),
		word("Incoterms:", 50, 150, 48),
		word("CIF", 101, 150, 18),
		word("Batam", 122, 150, 30),
	}
}

func tableHeaderWords() []testWord {
	return []testWord{
		word("No", 50, 200, 13),
		word("Description", 90, 200, 55),
		word("Qty", 300, 200, 16),
		word("UoM", 350, 200, 21),
		word("Unit", 400, 200, 18),
		word("Price", 421, 200, 23),
		word("Amount", 500, 200, 35),
	}
}

// rowWords lays out one table line. Each description word is placed on a
// fixed 38pt stride so its column assignment is unambiguous; this is a
// synthetic page, not a layout model.
func rowWords(number, description, qty, uom, price, amount string, y float64) []testWord {
	words := []testWord{word(number, 50, y, 6)}
	for index, part := range strings.Split(description, " ") {
		words = append(words, word(part, 90+float64(index)*38, y, 36))
	}
	return append(words,
		word(qty, 300, y, 18),
		word(uom, 350, y, 20),
		word(price, 400, y, 40),
		word(amount, 500, y, 40),
	)
}

func totalWords(total string, y float64) []testWord {
	return []testWord{
		word("Total:", 400, y, 35),
		word(total, 440, y, 45),
	}
}

func outputFields(result apicontract.ExtractionOutput) map[string]apicontract.ExtractionField {
	fields := make(map[string]apicontract.ExtractionField, len(result.Fields))
	for _, field := range result.Fields {
		fields[field.Key] = field
	}
	return fields
}

func assertField(t *testing.T, result apicontract.ExtractionOutput, key, value, status string) apicontract.ExtractionField {
	t.Helper()
	fields := outputFields(result)
	field, ok := fields[key]
	if !ok {
		t.Fatalf("field %q is missing from the result", key)
	}
	if field.Value != value {
		t.Errorf("%s value = %q; want %q", key, field.Value, value)
	}
	if field.Status != status {
		t.Errorf("%s status = %q; want %q", key, field.Status, status)
	}
	return field
}

func parseFixture(t *testing.T, pages ...[]testWord) apicontract.ExtractionOutput {
	t.Helper()
	parsed, err := ParseBBoxLayout(bboxXML(pages...))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	result, err := Propose(parsed, Options{MaxPages: 40})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return result
}

func TestParseBBoxLayoutReadsPagesWordsAndPageSizes(t *testing.T) {
	data := bboxXML(
		[]testWord{word("PO", 10, 20, 12), word("No:", 25, 20, 16)},
		[]testWord{word("Second", 30, 40, 40)},
	)
	pages, err := ParseBBoxLayout(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("pages = %d; want 2", len(pages))
	}
	if len(pages[0].Words) != 2 || pages[0].Words[0].Text != "PO" ||
		pages[0].Words[0].XMin != 10 || pages[0].Words[0].YMin != 20 {
		t.Errorf("page 1 words = %+v", pages[0].Words)
	}
	if pages[0].Width != 595 || pages[0].Height != 842 {
		t.Errorf("page 1 size = %fx%f; want 595x842", pages[0].Width, pages[0].Height)
	}
	if pages[1].Words[0].Text != "Second" {
		t.Errorf("page 2 words = %+v", pages[1].Words)
	}
}

func TestParseBBoxLayoutRejectsMalformedOutput(t *testing.T) {
	if _, err := ParseBBoxLayout([]byte("not xml")); err == nil {
		t.Fatal("malformed XML was accepted")
	}
	if _, err := ParseBBoxLayout([]byte("<html><body><doc></doc></body></html>")); err == nil {
		t.Fatal("a document with no pages was accepted")
	}
}

func TestProposeExtractsHeaderFieldsWithPageAndRegion(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	words = append(words, rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", 243)...)
	words = append(words, rowWords("3", "Freight", "1", "Lot", "150.00", "150.00", 261)...)
	words = append(words, totalWords("3091.00", 300)...)
	result := parseFixture(t, words)

	poNumber := assertField(t, result, apicontract.ExtractionFieldPONumber, "4500123456", apicontract.ExtractionStatusExtracted)
	if poNumber.Page == nil || *poNumber.Page != 1 {
		t.Errorf("poNumber page = %v; want 1", poNumber.Page)
	}
	if poNumber.BBox == nil || poNumber.BBox.X != 86 || poNumber.BBox.Width == 0 {
		t.Errorf("poNumber bbox = %+v; want the value words at x=86", poNumber.BBox)
	}
	if poNumber.Confidence != nil {
		t.Errorf("poNumber confidence = %v; Poppler reports none, so this must be null", *poNumber.Confidence)
	}
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-10-12", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldCurrency, "USD", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPaymentTerms, "30 days after delivery", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "CIF Batam", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldRequestedDeliveryDate, "", apicontract.ExtractionStatusAbsent)

	if result.PageCount != 1 || result.PagesWithText != 1 || result.PagesWithoutText != 0 {
		t.Errorf("page counts = %d/%d/%d; want 1/1/0",
			result.PageCount, result.PagesWithText, result.PagesWithoutText)
	}
	if len(result.Pages) != 1 || result.Pages[0].Width != pageWidth {
		t.Errorf("page sizes = %+v", result.Pages)
	}
	if result.Abstentions.Fields != 1 || result.Abstentions.Rows != 0 {
		t.Errorf("abstentions = %+v; want 1 field (requested date) and 0 rows", result.Abstentions)
	}
}

func TestProposeExtractsTableRowsWithCellsAndEvidence(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	words = append(words, rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", 243)...)
	words = append(words, rowWords("3", "Freight", "1", "Lot", "150.00", "150.00", 261)...)
	words = append(words, totalWords("3091.00", 300)...)
	result := parseFixture(t, words)

	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d; want 3 (%+v)", len(result.Rows), result.Rows)
	}
	first := result.Rows[0]
	if first.LineNumber.Value != "1" || first.Description.Value != "Electrical panel" ||
		first.Quantity.Value != "2" || first.UoM.Value != "Pc" ||
		first.UnitPrice.Value != "1250.00" || first.Amount.Value != "2500.00" {
		t.Errorf("row 1 = %+v", first)
	}
	if first.Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("row 1 status = %q; want extracted", first.Status)
	}
	if first.Page != 1 || first.BBox == nil || first.BBox.Y != 225 {
		t.Errorf("row 1 evidence = page %d bbox %+v; want page 1 at y=225", first.Page, first.BBox)
	}
	if first.Quantity.BBox == nil || first.Quantity.BBox.X != 300 {
		t.Errorf("quantity bbox = %+v; want the qty cell at x=300", first.Quantity.BBox)
	}
	if result.Rows[1].Quantity.Value != "10.5" {
		t.Errorf("row 2 quantity = %q; want 10.5", result.Rows[1].Quantity.Value)
	}
	for _, row := range result.Rows {
		if row.Amount.Status != apicontract.ExtractionStatusExtracted {
			t.Errorf("row %s amount status = %q; want extracted", row.LineNumber.Value, row.Amount.Status)
		}
	}
	if result.Abstentions.Rows != 0 {
		t.Errorf("abstention rows = %d; want 0", result.Abstentions.Rows)
	}
}

func TestProposeAbstainsWhenOCRFindsNoWords(t *testing.T) {
	parsed, err := ParseBBoxLayout(bboxXML([]testWord{}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	parsed[0].Mode = apicontract.ExtractionPageModeOCR
	result, err := Propose(parsed, Options{MaxPages: 40})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if result.PagesWithText != 0 || result.PagesWithoutText != 1 || result.PagesWithOCR != 1 {
		t.Errorf("page counts = %d text, %d without, %d ocr; want 0/1/1",
			result.PagesWithText, result.PagesWithoutText, result.PagesWithOCR)
	}
	if len(result.Rows) != 0 {
		t.Errorf("rows = %d; an OCR page with no words must not invent rows", len(result.Rows))
	}
	for _, field := range result.Fields {
		if field.Status != apicontract.ExtractionStatusAbsent {
			t.Errorf("%s status = %q; want absent", field.Key, field.Status)
		}
	}
	if result.Abstentions.Fields != len(apicontract.ExtractionFieldKeys) {
		t.Errorf("abstention fields = %d; want %d", result.Abstentions.Fields, len(apicontract.ExtractionFieldKeys))
	}
}

func TestProposeMarksAmbiguousConflictingAndUnparseableValuesNeedsReview(t *testing.T) {
	words := []testWord{
		word("PO", 50, 90, 14), word("No:", 67, 90, 16), word("111", 86, 90, 20),
		word("PO", 50, 105, 14), word("No:", 67, 105, 16), word("222", 86, 105, 20),
		word("PO", 50, 120, 14), word("Date:", 67, 120, 25), word("01/02/2026", 95, 120, 55),
		word("Currency:", 50, 135, 45), word("US", 98, 135, 14), word("Dollar", 115, 135, 30),
		word("Payment", 50, 150, 40), word("Terms:", 93, 150, 32),
		word("Incoterms:", 50, 165, 48), word("unknown", 101, 165, 40),
	}
	result := parseFixture(t, words)

	poNumber := assertField(t, result, apicontract.ExtractionFieldPONumber, "111", apicontract.ExtractionStatusNeedsReview)
	if !strings.Contains(strings.ToLower(poNumber.Note), "another value") {
		t.Errorf("poNumber note = %q; want the conflicting value named", poNumber.Note)
	}
	poDate := assertField(t, result, apicontract.ExtractionFieldPODate, "01/02/2026", apicontract.ExtractionStatusNeedsReview)
	if !strings.Contains(strings.ToLower(poDate.Note), "day") {
		t.Errorf("poDate note = %q; want the day/month ambiguity named", poDate.Note)
	}
	assertField(t, result, apicontract.ExtractionFieldCurrency, "US Dollar", apicontract.ExtractionStatusNeedsReview)
	payment := assertField(t, result, apicontract.ExtractionFieldPaymentTerms, "", apicontract.ExtractionStatusAbsent)
	if !strings.Contains(strings.ToLower(payment.Note), "label") {
		t.Errorf("paymentTerms note = %q; want the empty label named", payment.Note)
	}
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "unknown", apicontract.ExtractionStatusNeedsReview)
	assertField(t, result, apicontract.ExtractionFieldRequestedDeliveryDate, "", apicontract.ExtractionStatusAbsent)
	if result.Abstentions.Fields != 6 {
		t.Errorf("abstention fields = %d; want 6", result.Abstentions.Fields)
	}
}

func TestProposeMarksConflictingCurrenciesNeedsReview(t *testing.T) {
	// No "Currency:" label: the fallback collects every currency code it sees.
	// Two codes are two answers, never "the first one wins".
	words := []testWord{
		word("PO", 50, 90, 14), word("No:", 67, 90, 16), word("4500123456", 86, 90, 60),
		word("Amounts", 50, 120, 45), word("in", 99, 120, 12),
		word("USD", 115, 120, 25), word("and", 145, 120, 20), word("IDR", 170, 120, 25),
	}
	result := parseFixture(t, words)
	currency := assertField(t, result, apicontract.ExtractionFieldCurrency, "USD / IDR", apicontract.ExtractionStatusNeedsReview)
	if !strings.Contains(currency.Note, "USD") || !strings.Contains(currency.Note, "IDR") {
		t.Errorf("currency note = %q; want both printed codes named", currency.Note)
	}

	// The same conflict on a labelled line must not normalise to its first token.
	labelled := []testWord{
		word("Currency:", 50, 90, 50), word("USD", 104, 90, 22),
		word("/", 128, 90, 6), word("IDR", 136, 90, 22),
	}
	result = parseFixture(t, labelled)
	currency = assertField(t, result, apicontract.ExtractionFieldCurrency, "USD / IDR", apicontract.ExtractionStatusNeedsReview)
	if !strings.Contains(currency.Note, "USD") || !strings.Contains(currency.Note, "IDR") {
		t.Errorf("labelled currency note = %q; want both printed codes named", currency.Note)
	}
}

func TestNormaliseDecimalKeepsGroupingHonest(t *testing.T) {
	cases := []struct {
		raw    string
		kind   decimalKind
		want   string
		status string
	}{
		{"1.2.3", decimalQuantity, "1.2.3", apicontract.ExtractionStatusNeedsReview},
		{"1.25.000", decimalMoney, "1.25.000", apicontract.ExtractionStatusNeedsReview},
		{"1,25,000", decimalMoney, "1,25,000", apicontract.ExtractionStatusNeedsReview},
		{"1,2.30", decimalMoney, "1,2.30", apicontract.ExtractionStatusNeedsReview},
		{"1.250.000", decimalMoney, "1250000", apicontract.ExtractionStatusExtracted},
		{"1,250,000", decimalMoney, "1250000", apicontract.ExtractionStatusExtracted},
		{"1.250,00", decimalMoney, "1250.00", apicontract.ExtractionStatusExtracted},
		{"1,250.00", decimalMoney, "1250.00", apicontract.ExtractionStatusExtracted},
	}
	for _, testCase := range cases {
		got, status, _ := normaliseDecimal(testCase.raw, testCase.kind)
		if got != testCase.want || status != testCase.status {
			t.Errorf("normaliseDecimal(%q) = %q/%q; want %q/%q",
				testCase.raw, got, status, testCase.want, testCase.status)
		}
	}
}

func tableHeaderWithoutAmountWords() []testWord {
	return []testWord{
		word("No", 50, 200, 13),
		word("Description", 90, 200, 55),
		word("Qty", 300, 200, 16),
		word("UoM", 350, 200, 21),
		word("Unit", 400, 200, 18),
		word("Price", 421, 200, 23),
	}
}

func TestProposeKeepsTheQuantityWhenThereIsNoAmountColumn(t *testing.T) {
	words := append(headerWords(), tableHeaderWithoutAmountWords()...)
	words = append(words, word("1", 50, 225, 6))
	words = append(words, word("Electrical", 90, 225, 70), word("panel", 162, 225, 40))
	words = append(words, word("2", 300, 225, 18), word("Pc", 350, 225, 20),
		word("1250.00", 400, 225, 40))
	words = append(words, totalWords("2500.00", 260)...)
	result := parseFixture(t, words)

	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want 1 (%+v)", len(result.Rows), result.Rows)
	}
	row := result.Rows[0]
	if row.Quantity.Status != apicontract.ExtractionStatusExtracted || row.Quantity.Value != "2" {
		t.Errorf("quantity = %+v; want the printed qty 2 kept", row.Quantity)
	}
	if row.Amount.Status != apicontract.ExtractionStatusAbsent || row.Amount.Value != "" {
		t.Errorf("amount = %+v; a table with no Amount column must leave it absent", row.Amount)
	}
	if row.UnitPrice.Value != "1250.00" {
		t.Errorf("unit price = %q; want 1250.00", row.UnitPrice.Value)
	}
}

func TestProposeCarriesTheWholeWrappedDescription(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	// A wrapped description continues the same line item: the full text and a
	// region covering both lines, never a value whose evidence is only part.
	words = append(words, word("extension", 90, 239, 55))
	words = append(words, rowWords("2", "Cable", "1", "M", "42.00", "42.00", 261)...)
	result := parseFixture(t, words)

	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d; want 2", len(result.Rows))
	}
	first := result.Rows[0]
	if first.Description.Value != "Panel extension" || first.Description.Raw != "Panel extension" {
		t.Errorf("description = %q (raw %q); want the whole wrapped text",
			first.Description.Value, first.Description.Raw)
	}
	if first.Description.BBox == nil || first.Description.BBox.Height < 20 {
		t.Errorf("description bbox = %+v; want a union covering both wrapped lines", first.Description.BBox)
	}
	if first.BBox == nil || first.BBox.Height < 20 {
		t.Errorf("row bbox = %+v; want the wrapped line included", first.BBox)
	}
}

func TestProposeMarksIncompleteAndDuplicateRowsNeedsReview(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	// Row 2 has no quantity: the row exists, the cell is absent, and the row
	// cannot be called extracted.
	words = append(words, word("2", 50, 243, 6), word("Cable", 90, 243, 40),
		word("termination", 134, 243, 55), word("M", 350, 243, 20),
		word("42.00", 400, 243, 40), word("441.00", 500, 243, 40))
	// Row 3 repeats row 1's line number: a conflicting duplicate detection.
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 261)...)
	// Row 4's quantity is not a number.
	words = append(words, rowWords("4", "Freight", "abc", "Lot", "150.00", "150.00", 279)...)
	result := parseFixture(t, words)

	if len(result.Rows) != 4 {
		t.Fatalf("rows = %d; want 4", len(result.Rows))
	}
	incomplete := result.Rows[1]
	if incomplete.Status != apicontract.ExtractionStatusNeedsReview {
		t.Errorf("incomplete row status = %q; want needs_review", incomplete.Status)
	}
	if incomplete.Quantity.Status != apicontract.ExtractionStatusAbsent {
		t.Errorf("incomplete row quantity status = %q; want absent", incomplete.Quantity.Status)
	}
	duplicate := result.Rows[2]
	if duplicate.Status != apicontract.ExtractionStatusNeedsReview ||
		!strings.Contains(strings.ToLower(duplicate.Note), "more than once") {
		t.Errorf("duplicate row = %+v; want needs_review with a duplicate note", duplicate)
	}
	// Both occurrences are flagged: the extractor cannot know which printed
	// line is the repeat, and it must not pick one by dropping the other.
	if result.Rows[0].Status != apicontract.ExtractionStatusNeedsReview {
		t.Errorf("first occurrence status = %q; want needs_review as well", result.Rows[0].Status)
	}
	unparseable := result.Rows[3]
	if unparseable.Quantity.Status != apicontract.ExtractionStatusNeedsReview ||
		unparseable.Quantity.Raw != "abc" {
		t.Errorf("unparseable quantity = %+v; want needs_review with the printed text", unparseable.Quantity)
	}
	if result.Abstentions.Rows != 4 {
		t.Errorf("abstention rows = %d; want 4 (both duplicates, the incomplete row and the unparseable one)", result.Abstentions.Rows)
	}
}

func TestProposeReadsContinuationRowsAcrossPages(t *testing.T) {
	pageOne := append(headerWords(), tableHeaderWords()...)
	pageOne = append(pageOne, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	pageOne = append(pageOne, rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", 243)...)
	pageTwo := append([]testWord{}, tableHeaderWords()...)
	pageTwo = append(pageTwo, rowWords("3", "Freight", "1", "Lot", "150.00", "150.00", 225)...)
	pageTwo = append(pageTwo, totalWords("3091.00", 300)...)
	result := parseFixture(t, pageOne, pageTwo)
	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d; want 3", len(result.Rows))
	}
	if result.Rows[0].Page != 1 || result.Rows[2].Page != 2 {
		t.Errorf("row pages = %d/%d/%d; want 1/1/2",
			result.Rows[0].Page, result.Rows[1].Page, result.Rows[2].Page)
	}
}

func TestProposeProducesNoRowsWhenThereIsNoReadableTable(t *testing.T) {
	words := []testWord{
		word("PO", 50, 90, 14), word("No:", 67, 90, 16), word("4500123456", 86, 90, 60),
		word("Some", 50, 130, 30), word("prose", 85, 130, 35), word("only.", 125, 130, 30),
	}
	result := parseFixture(t, words)
	if len(result.Rows) != 0 {
		t.Errorf("rows = %d; a page with no table header must not invent rows", len(result.Rows))
	}
	if result.Abstentions.Rows != 0 {
		t.Errorf("abstention rows = %d; no row was detected, so none was abstained", result.Abstentions.Rows)
	}
}

func TestProposeStopsAtFooterTextAfterTheTable(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	// A real PO often ends the table with requester or invoice prose. A line
	// whose first cell is a labelled footer is not a row, and the lines after
	// it are not rows either.
	words = append(words, word("Requester:", 50, 280, 55), word("Bintang", 110, 280, 40),
		word("PR783275", 155, 280, 50))
	words = append(words, word("Invoices", 90, 300, 45), word("c/w", 140, 300, 20))
	result := parseFixture(t, words)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want only the table row (%+v)", len(result.Rows), result.Rows)
	}
}

// ---------------------------------------------------------------------------
// Issue #478: multipage tables, OCR confidence and line-number risks
// ---------------------------------------------------------------------------

func TestProposeContinuesRowsAcrossAPageWithoutARepeatedHeader(t *testing.T) {
	pageOne := append(headerWords(), tableHeaderWords()...)
	pageOne = append(pageOne, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	pageOne = append(pageOne, rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", 243)...)
	// The continued page has a running header and the next two rows, but no
	// repeated column header. The rows continue the first page's table; the
	// running header is not appended to the previous row.
	pageTwo := []testWord{
		word("PURCHASE", 50, 50, 70), word("ORDER", 125, 50, 45),
		word("Page", 500, 50, 30), word("2", 534, 50, 8),
	}
	pageTwo = append(pageTwo, rowWords("3", "Freight", "1", "Lot", "150.00", "150.00", 225)...)
	pageTwo = append(pageTwo, rowWords("4", "Packing", "2", "Pc", "25.00", "50.00", 243)...)
	result := parseFixture(t, pageOne, pageTwo)

	if len(result.Rows) != 4 {
		t.Fatalf("rows = %d; want 4 across the page break (%+v)", len(result.Rows), result.Rows)
	}
	third := result.Rows[2]
	if third.Page != 2 || third.Description.Value != "Freight" ||
		third.Quantity.Value != "1" || third.UnitPrice.Value != "150.00" {
		t.Errorf("continued row = %+v; want page 2 row 3 read with the carried columns", third)
	}
	if third.Status != apicontract.ExtractionStatusNeedsReview ||
		!strings.Contains(third.Note, "column header is on page 1") {
		t.Errorf("continued row = %+v; want needs_review naming the carried header", third)
	}
	if strings.Contains(result.Rows[1].Description.Value, "PURCHASE") {
		t.Errorf("page 2's running header was appended to page 1's last row: %q",
			result.Rows[1].Description.Value)
	}
	if result.Rows[1].Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("page 1 row 2 status = %q; want extracted", result.Rows[1].Status)
	}
}

func TestProposeDoesNotCarryColumnsPastATableTerminator(t *testing.T) {
	pageOne := append(headerWords(), tableHeaderWords()...)
	pageOne = append(pageOne, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	pageOne = append(pageOne, totalWords("2500.00", 300)...)
	// Page 2 has no header and no open table: rows must not be invented from a
	// table that already ended on page 1.
	pageTwo := rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", 225)
	result := parseFixture(t, pageOne, pageTwo)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want only page 1's row (%+v)", len(result.Rows), result.Rows)
	}
}

func TestProposeFlagsAMissingLineNumberWithoutInventingTheRow(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	words = append(words, rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", 243)...)
	words = append(words, rowWords("4", "Freight", "1", "Lot", "150.00", "150.00", 261)...)
	result := parseFixture(t, words)
	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d; want the three printed rows and no invented row 3", len(result.Rows))
	}
	gap := result.Rows[2]
	if gap.Status != apicontract.ExtractionStatusNeedsReview ||
		!strings.Contains(gap.Note, "3 is not printed") {
		t.Errorf("row 4 = %+v; want needs_review naming the missing line 3", gap)
	}
	if result.Rows[0].Status != apicontract.ExtractionStatusExtracted ||
		result.Rows[1].Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("rows before the gap = %q/%q; want extracted", result.Rows[0].Status, result.Rows[1].Status)
	}
}

func TestProposeFlagsDuplicateLineNumbersWithoutDroppingEitherRow(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	words = append(words, rowWords("2", "Cable termination", "10.5", "M", "42.00", "441.00", 243)...)
	words = append(words, rowWords("2", "Freight", "1", "Lot", "150.00", "150.00", 261)...)
	result := parseFixture(t, words)
	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d; a duplicate line number must not remove a row", len(result.Rows))
	}
	for _, index := range []int{1, 2} {
		row := result.Rows[index]
		if row.Status != apicontract.ExtractionStatusNeedsReview ||
			!strings.Contains(row.Note, "appears more than once") {
			t.Errorf("row %d = %+v; want needs_review naming the duplicated line number", index, row)
		}
	}
	if result.Rows[0].Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("row 1 status = %q; want extracted", result.Rows[0].Status)
	}
}

func TestProposeMarksLowConfidenceOCRValuesNeedsReview(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	parsed, err := ParseBBoxLayout(bboxXML(words))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	parsed[0].Mode = apicontract.ExtractionPageModeOCR
	parsed[0].Words = withConfidence(parsed[0].Words, 55)
	result, err := Propose(parsed, Options{MaxPages: 40})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	poNumber := assertField(t, result, apicontract.ExtractionFieldPONumber, "4500123456", apicontract.ExtractionStatusNeedsReview)
	if poNumber.Confidence == nil || *poNumber.Confidence != 55 {
		t.Errorf("poNumber confidence = %v; want the measured 55", poNumber.Confidence)
	}
	if !strings.Contains(poNumber.Note, "OCR confidence") {
		t.Errorf("poNumber note = %q; want the low-confidence reason", poNumber.Note)
	}
	row := result.Rows[0]
	if row.Status != apicontract.ExtractionStatusNeedsReview {
		t.Errorf("row status = %q; want needs_review", row.Status)
	}
	if row.Quantity.Status != apicontract.ExtractionStatusNeedsReview ||
		!strings.Contains(row.Quantity.Note, "55%") {
		t.Errorf("quantity = %+v; want needs_review naming the 55%% confidence", row.Quantity)
	}
	if !strings.Contains(row.Note, "low OCR confidence") {
		t.Errorf("row note = %q; want the low-confidence warning", row.Note)
	}
}

func TestProposeKeepsAConfidentOCRValueExtracted(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	parsed, err := ParseBBoxLayout(bboxXML(words))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	parsed[0].Mode = apicontract.ExtractionPageModeOCR
	parsed[0].Words = withConfidence(parsed[0].Words, 96)
	result, err := Propose(parsed, Options{MaxPages: 40})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	assertField(t, result, apicontract.ExtractionFieldPONumber, "4500123456", apicontract.ExtractionStatusExtracted)
	if result.Rows[0].Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("row status = %q; a confident OCR value stays extracted", result.Rows[0].Status)
	}
	if result.Rows[0].Quantity.Confidence == nil || *result.Rows[0].Quantity.Confidence != 96 {
		t.Errorf("quantity confidence = %v; want 96", result.Rows[0].Quantity.Confidence)
	}
}

func TestProposeKeepsAConfidenceFloorAboveTheDefault(t *testing.T) {
	words := append(headerWords(), tableHeaderWords()...)
	words = append(words, rowWords("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00", 225)...)
	parsed, err := ParseBBoxLayout(bboxXML(words))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	parsed[0].Mode = apicontract.ExtractionPageModeOCR
	parsed[0].Words = withConfidence(parsed[0].Words, 80)
	result, err := Propose(parsed, Options{MaxPages: 40, MinConfidence: 90})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if result.Rows[0].Status != apicontract.ExtractionStatusNeedsReview {
		t.Errorf("row status = %q; 80%% is below a configured 90%% floor", result.Rows[0].Status)
	}
	if !strings.Contains(result.Rows[0].Quantity.Note, "90%") {
		t.Errorf("quantity note = %q; want the configured floor named", result.Rows[0].Quantity.Note)
	}
}

func TestProposeJoinsAThousandsGroupedNumberButNotASplitDecimal(t *testing.T) {
	// OCR and text layers both split a printed number across words. A
	// grouping-shaped split is joined ("1" "250.00" is 1250.00); a split that
	// does not form a grouping ("1250" "00") must abstain, because joining it
	// would silently turn 1,250.00 into 125000.
	grouped := append(headerWords(), tableHeaderWords()...)
	grouped = append(grouped,
		word("1", 50, 225, 6), word("Panel", 90, 225, 40),
		word("2", 300, 225, 5), word("Pc", 350, 225, 20),
		word("1", 400, 225, 5), word("250.00", 410, 225, 40),
		word("2500.00", 500, 225, 40))
	result := parseFixture(t, grouped)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(result.Rows))
	}
	if result.Rows[0].UnitPrice.Value != "1250.00" ||
		result.Rows[0].UnitPrice.Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("unit price = %+v; want the grouped 1250.00", result.Rows[0].UnitPrice)
	}

	split := append(headerWords(), tableHeaderWords()...)
	split = append(split,
		word("1", 50, 225, 6), word("Panel", 90, 225, 40),
		word("2", 300, 225, 5), word("Pc", 350, 225, 20),
		word("1250", 400, 225, 30), word("00", 430, 225, 10),
		word("2500.00", 500, 225, 40))
	result = parseFixture(t, split)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(result.Rows))
	}
	price := result.Rows[0].UnitPrice
	if price.Status != apicontract.ExtractionStatusNeedsReview ||
		!strings.Contains(price.Note, "separate tokens") {
		t.Errorf("unit price = %+v; want needs_review with the split-token reason", price)
	}
	if price.Value != "1250 00" {
		t.Errorf("unit price value = %q; want the printed tokens preserved", price.Value)
	}
}

func TestProposeCarriesAWrappedDescriptionAfterManyRows(t *testing.T) {
	// The proposer holds a pointer to the last row so a wrapped description can
	// extend it. That pointer must survive the slice growing: with more rows
	// than the initial capacity, append reallocates and a stale pointer would
	// silently drop the continuation text.
	words := append(headerWords(), tableHeaderWords()...)
	for index := 1; index <= 20; index++ {
		words = append(words, rowWords(strconv.Itoa(index), "Panel", "1", "Pc", "10.00", "10.00", 225+float64(index-1)*18)...)
	}
	words = append(words, word("extension", 90, 225+20*18, 55))
	result := parseFixture(t, words)
	if len(result.Rows) != 20 {
		t.Fatalf("rows = %d; want 20", len(result.Rows))
	}
	last := result.Rows[len(result.Rows)-1]
	if last.Description.Value != "Panel extension" {
		t.Errorf("last description = %q; want the wrapped text carried after 20 rows", last.Description.Value)
	}
}
