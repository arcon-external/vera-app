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

func TestProposeAbstainsOnImageOnlyPages(t *testing.T) {
	result := parseFixture(t, []testWord{})
	if result.PagesWithoutText != 1 || result.PagesWithText != 0 {
		t.Errorf("page counts = %d with text, %d without; want 0/1",
			result.PagesWithText, result.PagesWithoutText)
	}
	if len(result.Rows) != 0 {
		t.Errorf("rows = %d; an image-only page must not invent rows", len(result.Rows))
	}
	for _, field := range result.Fields {
		if field.Status != apicontract.ExtractionStatusAbsent {
			t.Errorf("%s status = %q; want absent (no OCR in this slice)", field.Key, field.Status)
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
	unparseable := result.Rows[3]
	if unparseable.Quantity.Status != apicontract.ExtractionStatusNeedsReview ||
		unparseable.Quantity.Raw != "abc" {
		t.Errorf("unparseable quantity = %+v; want needs_review with the printed text", unparseable.Quantity)
	}
	if result.Abstentions.Rows != 3 {
		t.Errorf("abstention rows = %d; want 3", result.Abstentions.Rows)
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
