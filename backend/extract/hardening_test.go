package extract

import (
	"strings"
	"testing"

	"vera-app/backend/apicontract"
)

// Review hardening (#476): these hold the conservative boundary of the generic
// heuristics. A label alone is not a value, a separator is optional, an
// unsupported currency/identifier is not asserted, and an unseen label set
// abstains rather than being guessed.

func TestProposeRejectsANextLineThatIsNotAValue(t *testing.T) {
	// A party/address line below a label is not that label's value. Every field
	// here must abstain, never become an extracted supplier name, date, currency
	// or term.
	words := []testWord{
		word("Purchase", 50, 60, 70), word("Order", 124, 60, 40), word("No:", 168, 60, 24),
		word("Supplier", 50, 76, 45), word("PT", 100, 76, 14), word("Contoh", 116, 76, 40),
		word("Order", 50, 110, 38), word("Date:", 92, 110, 26),
		word("Supplier", 50, 126, 45), word("PT", 100, 126, 14), word("Contoh", 116, 126, 40),
		word("Currency:", 50, 160, 50),
		word("Supplier", 50, 176, 45), word("PT", 100, 176, 14), word("Contoh", 116, 176, 40),
		word("Payment", 50, 210, 48), word("Terms:", 102, 210, 32),
		word("Supplier", 50, 226, 45), word("PT", 100, 226, 14), word("Contoh", 116, 226, 40),
		word("Incoterms:", 50, 260, 55),
		word("Supplier", 50, 276, 45), word("PT", 100, 276, 14), word("Contoh", 116, 276, 40),
		word("Requested", 50, 310, 50), word("delivery", 104, 310, 40), word("date:", 148, 310, 26),
		word("Supplier", 50, 326, 45), word("PT", 100, 326, 14), word("Contoh", 116, 326, 40),
	}
	result := parseFixture(t, words)
	for _, key := range []string{
		apicontract.ExtractionFieldPONumber,
		apicontract.ExtractionFieldPODate,
		apicontract.ExtractionFieldCurrency,
		apicontract.ExtractionFieldPaymentTerms,
		apicontract.ExtractionFieldIncoterms,
		apicontract.ExtractionFieldRequestedDeliveryDate,
	} {
		field := assertField(t, result, key, "", apicontract.ExtractionStatusAbsent)
		if strings.Contains(strings.ToLower(field.Value), "supplier") {
			t.Errorf("%s adopted a supplier line as its value: %q", key, field.Value)
		}
	}
}

func TestProposeReadsLabelsWithoutSeparators(t *testing.T) {
	words := []testWord{
		word("Purchase", 50, 60, 70), word("Order", 124, 60, 40), word("No", 168, 60, 18),
		word("900123", 190, 60, 40),
		word("Order", 50, 90, 38), word("Date", 92, 90, 30),
		word("15", 126, 90, 12), word("March", 140, 90, 32), word("2026", 174, 90, 24),
		word("Incoterms", 50, 120, 55), word("DDP", 108, 120, 24),
	}
	result := parseFixture(t, words)
	assertField(t, result, apicontract.ExtractionFieldPONumber, "900123", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-03-15", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "DDP", apicontract.ExtractionStatusExtracted)
}

func TestProposeReadsAnOrderSubmittedValueOnTheNextLine(t *testing.T) {
	words := []testWord{
		word("Order", 50, 60, 38), word("submitted", 92, 60, 60), word("on", 156, 60, 14),
		word("Tuesday", 50, 76, 45), word("7", 98, 76, 6), word("Apr", 108, 76, 20),
		word("2026", 132, 76, 26),
	}
	result := parseFixture(t, words)
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-04-07", apicontract.ExtractionStatusExtracted)
}

func TestNormaliseCurrencyRejectsUnsupportedCodes(t *testing.T) {
	for _, raw := range []string{"ABC", "ZZZ", "US", "DOLLARS"} {
		value, status, _ := normaliseCurrency(raw)
		if status == apicontract.ExtractionStatusExtracted {
			t.Errorf("normaliseCurrency(%q) = %q/%q; unsupported codes must abstain", raw, value, status)
		}
	}
	value, status, _ := normaliseCurrency("IDR")
	if value != "IDR" || status != apicontract.ExtractionStatusExtracted {
		t.Errorf("normaliseCurrency(IDR) = %q/%q; want IDR/extracted", value, status)
	}
	result := parseFixture(t, []testWord{word("Currency:", 50, 60, 50), word("ABC", 104, 60, 24)})
	assertField(t, result, apicontract.ExtractionFieldCurrency, "ABC", apicontract.ExtractionStatusNeedsReview)
}

func TestProposeDerivesCurrencyOnlyFromAmountContext(t *testing.T) {
	prose := parseFixture(t, []testWord{
		word("Deposit", 50, 60, 45), word("payable", 100, 60, 45), word("in", 150, 60, 14),
		word("USD", 168, 60, 22),
	})
	assertField(t, prose, apicontract.ExtractionFieldCurrency, "", apicontract.ExtractionStatusAbsent)

	result := parseFixture(t,
		[]testWord{
			word("Deposit", 50, 60, 45), word("payable", 100, 60, 45), word("in", 150, 60, 14),
			word("USD", 168, 60, 22),
		},
		[]testWord{
			word("Amount:", 50, 60, 50), word("Rp", 105, 60, 14),
			word("1,000.00", 120, 60, 45), word("IDR", 170, 60, 22),
		},
	)
	currency := assertField(t, result, apicontract.ExtractionFieldCurrency, "IDR", apicontract.ExtractionStatusExtracted)
	if currency.Page == nil || *currency.Page != 2 {
		t.Errorf("currency page = %v; want the page that printed the amount context", currency.Page)
	}
}

func TestValidPONumberRejectsPlaceholders(t *testing.T) {
	for _, raw := range []string{"TBD", "N/A", "NA", "XXX", "-", "N.A.", "not available"} {
		if validPONumber(raw) {
			t.Errorf("validPONumber(%q) = true; a placeholder is not a PO number", raw)
		}
	}
	if !validPONumber("4500123456") || !validPONumber("PO-2026-7781") {
		t.Error("a real PO number was rejected")
	}
	result := parseFixture(t, []testWord{
		word("Purchase", 50, 60, 70), word("Order", 124, 60, 40), word("No:", 168, 60, 24),
		word("TBD", 196, 60, 24),
	})
	assertField(t, result, apicontract.ExtractionFieldPONumber, "TBD", apicontract.ExtractionStatusNeedsReview)
}

func TestProposeNamesTheRowBehindADerivedDeliveryDate(t *testing.T) {
	page := aribaLikeTableHeader(100)
	page = append(page,
		word("1", 40, 120, 6),
		word("Not", 185, 120, 18), word("Available", 205, 120, 38),
		word("Material", 325, 120, 35),
		word("2", 385, 120, 6), word("(EA)", 392, 120, 24),
		word("TBD", 448, 120, 24),
		word("IDR", 500, 120, 18), word("1,000.00", 520, 120, 50),
		word("IDR", 560, 120, 18), word("2,000.00", 580, 120, 50),
	)
	page = append(page, aribaNumericRow("2", "3", "(EA)", "12", "Jun", "2026", "500.00", "1,500.00", 160)...)
	page = append(page,
		word("Valve", 185, 178, 30),
		word("STATUS", 60, 200, 38),
	)
	result := parseFixture(t, page)
	delivery := assertField(t, result, apicontract.ExtractionFieldRequestedDeliveryDate, "2026-06-12", apicontract.ExtractionStatusNeedsReview)
	if !strings.Contains(delivery.Note, "line 2") {
		t.Errorf("delivery note = %q; want the actual row (line 2) named", delivery.Note)
	}
	if strings.Contains(delivery.Note, "first line's") {
		t.Errorf("delivery note = %q; it must not claim the first line when line 2 carried the date", delivery.Note)
	}
}

func TestProposeDerivedDeliveryCarriesTheRowConfidence(t *testing.T) {
	page := aribaLikeTableHeader(100)
	page = append(page, aribaNumericRow("1", "2", "(EA)", "12", "Jun", "2026", "1,000.00", "2,000.00", 120)...)
	parsed, err := ParseBBoxLayout(bboxXML(page))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	parsed[0].Mode = apicontract.ExtractionPageModeOCR
	parsed[0].Words = withConfidence(parsed[0].Words, 55)
	result, err := Propose(parsed, Options{MaxPages: 40})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	delivery := assertField(t, result, apicontract.ExtractionFieldRequestedDeliveryDate, "2026-06-12", apicontract.ExtractionStatusNeedsReview)
	if delivery.Confidence == nil || *delivery.Confidence != 55 {
		t.Errorf("delivery confidence = %v; want the row's measured 55", delivery.Confidence)
	}
	if !strings.Contains(delivery.Note, "OCR confidence") {
		t.Errorf("delivery note = %q; want the low-confidence reason", delivery.Note)
	}
}

func TestProposeReadsAGenericSynonymLayout(t *testing.T) {
	page := []testWord{
		word("Document", 50, 60, 55), word("No:", 108, 60, 22), word("DOC-9001", 134, 60, 55),
		word("Issued", 50, 90, 40), word("4", 94, 90, 6), word("May", 104, 90, 22),
		word("2026", 130, 90, 26),
		word("Currency:", 50, 120, 50), word("USD", 104, 120, 22),
		word("Payment", 50, 150, 48), word("Terms:", 102, 150, 32), word("NET", 138, 150, 24),
		word("30", 166, 150, 10),
		word("Incoterms:", 50, 180, 55), word("FOB", 108, 180, 20),
		word("Article", 50, 240, 40), word("Count", 300, 240, 32),
		word("Measure", 350, 240, 45), word("Tariff", 420, 240, 34),
		word("Sum", 500, 240, 22),
		word("Widget", 50, 265, 42), word("2", 300, 265, 8), word("EA", 350, 265, 18),
		word("125.50", 420, 265, 38), word("251.00", 500, 265, 40),
	}
	result := parseFixture(t, page)
	assertField(t, result, apicontract.ExtractionFieldPONumber, "DOC-9001", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-05-04", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldCurrency, "USD", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPaymentTerms, "NET 30", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "FOB", apicontract.ExtractionStatusExtracted)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want 1 (%+v)", len(result.Rows), result.Rows)
	}
	row := result.Rows[0]
	if row.Description.Value != "Widget" || row.Quantity.Value != "2" ||
		row.UoM.Value != "EA" || row.UnitPrice.Value != "125.50" ||
		row.Amount.Value != "251.00" || row.Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("row = %+v; want the generic synonym header read", row)
	}
}

func TestProposeAbstainsOnAnUnseenLabelSet(t *testing.T) {
	page := []testWord{
		word("Purchase", 50, 60, 60), word("Instrument", 114, 60, 58),
		word("Identifier:", 176, 60, 60), word("PI-77", 240, 60, 34),
		word("Committed:", 50, 90, 58), word("4", 112, 90, 6), word("May", 122, 90, 22),
		word("2026", 148, 90, 26),
		word("Commodity", 50, 240, 55), word("Wording", 110, 240, 42),
		word("Bundle", 250, 240, 34), word("Size", 288, 240, 22),
		word("Pricing", 340, 240, 34), word("Basis", 378, 240, 28),
		word("Aggregate", 470, 240, 48),
		word("Widget", 50, 265, 42), word("2", 250, 265, 8),
		word("125.50", 340, 265, 38), word("251.00", 470, 265, 40),
	}
	result := parseFixture(t, page)
	for _, field := range result.Fields {
		if field.Status == apicontract.ExtractionStatusExtracted {
			t.Errorf("%s = %q; an unseen label set must abstain", field.Key, field.Value)
		}
	}
	if len(result.Rows) != 0 {
		t.Errorf("rows = %d; an unseen table header must not fabricate rows", len(result.Rows))
	}
}
