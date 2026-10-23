package extract

import (
	"strings"
	"testing"

	"vera-app/backend/apicontract"
)

// Issue #476: layouts the original label/column heuristic did not read.
//
// Every fixture here is invented in code with the same positioned-word shape
// Poppler prints. No customer document is committed, and no layout below is
// treated as a template: they are three structurally different documents used
// to hold the generic rules (label synonyms, a value on the next line, a table
// found by its header, row anchors and continuation) rather than one printed
// form.

// aribaLikePageOne puts the order number, the submission date and an amount
// (which carries the currency code) in a single header block. The payment
// terms value is on the line below its label.
func aribaLikePageOne() []testWord {
	return []testWord{
		word("Purchase", 50, 60, 80), word("Order:", 134, 60, 42), word("4500123456", 180, 60, 62),
		word("Amount:", 50, 92, 45), word("Rp145,760,000.00", 100, 92, 105), word("IDR", 210, 92, 22),
		word("Payment", 50, 108, 48), word("Terms", 102, 108, 32),
		word("NET", 50, 124, 24), word("90", 78, 124, 10),
	}
}

// aribaLikeTableHeader is the Ariba column set: a line number followed by a
// non-data "No. Schedule Lines" column, a merged part/description column, two
// non-data columns, a combined "Qty (Unit)" column, a delivery column and the
// two money columns.
func aribaLikeTableHeader(y float64) []testWord {
	return []testWord{
		word("Line", 40, y, 24), word("#", 66, y, 6),
		word("No.", 85, y, 16), word("Schedule", 105, y, 42), word("Lines", 150, y, 26),
		word("Part", 185, y, 22), word("#", 209, y, 6), word("/", 217, y, 5),
		word("Description", 225, y, 55),
		word("Type", 320, y, 22), word("Return", 350, y, 30),
		word("Qty", 385, y, 16), word("(Unit)", 403, y, 32),
		word("Need", 445, y, 22), word("By", 470, y, 10),
		word("Unit", 500, y, 20), word("Price", 522, y, 25),
		word("Subtotal", 560, y, 46),
	}
}

func aribaLikeRowOne() []testWord {
	return []testWord{
		word("1", 40, 180, 6),
		word("Not", 185, 180, 18), word("Available", 205, 180, 38),
		word("Material", 325, 180, 35),
		word("1", 385, 180, 6), word("(PK)", 392, 180, 24),
		word("31", 448, 180, 12), word("Jan", 462, 180, 16), word("2027", 478, 180, 20),
		word("Rp145,760,000.00", 500, 180, 50), word("IDR", 533, 180, 16),
		word("Rp145,760,000.00", 560, 180, 50), word("IDR", 575, 180, 16),
	}
}

// aribaNumericRow prints one Ariba-shaped numeric row: the line number, the
// part placeholder, the non-data Type value, a combined quantity, a delivery
// date and two currency-wrapped money cells.
func aribaNumericRow(number, qty, unit, day, month, year, price, amount string, y float64) []testWord {
	return []testWord{
		word(number, 40, y, 6),
		word("Not", 185, y, 18), word("Available", 205, y, 38),
		word("Material", 325, y, 35),
		word(qty, 385, y, 6), word(unit, 392, y, 24),
		word(day, 448, y, 12), word(month, 462, y, 16), word(year, 478, y, 20),
		word("IDR", 500, y, 18), word(price, 520, y, 50),
		word("IDR", 560, y, 18), word(amount, 580, y, 50),
	}
}

// aribaLikeWrappedDescription is the description printed below the numeric
// row. Its last token sits in the unit-price column, so only an anchor test
// (not "does a token land in a numeric column") can keep it a continuation.
func aribaLikeWrappedDescription() []testWord {
	return []testWord{
		word("BAT3", 185, 198, 30), word("Auto-Message", 205, 198, 55),
		word("Alert", 250, 198, 30), word("for", 270, 198, 18),
		word("Scrubber", 185, 216, 40), word("Scope", 215, 216, 30),
		word("of", 235, 216, 12), word("Work", 248, 216, 28),
		word("refer", 265, 216, 30), word("15020843", 500, 216, 42),
	}
}

func TestProposeReadsAnAribaLikeLayoutAcrossPages(t *testing.T) {
	pageOne := aribaLikePageOne()

	pageTwo := []testWord{
		word("Incoterm", 50, 100, 50), word("Code:", 104, 100, 30), word("DDP", 138, 100, 24),
	}
	pageTwo = append(pageTwo, aribaLikeTableHeader(160)...)
	pageTwo = append(pageTwo, aribaLikeRowOne()...)
	pageTwo = append(pageTwo, aribaLikeWrappedDescription()...)

	pageThree := aribaLikeTableHeader(80)
	pageThree = append(pageThree,
		word("2", 40, 100, 6),
		word("Installation", 185, 100, 65),
		word("2", 385, 100, 6), word("(EA)", 392, 100, 24),
		word("15", 448, 100, 12), word("Feb", 462, 100, 16), word("2027", 478, 100, 20),
		word("IDR", 500, 100, 18), word("1,500,000.00", 520, 100, 55),
		word("IDR", 560, 100, 18), word("3,000,000.00", 580, 100, 55),
		word("and", 185, 118, 18), word("commissioning", 205, 118, 70),
		word("Sub-total:", 500, 180, 50), word("3,000,000.00", 555, 180, 55),
	)
	pageThree = append(pageThree,
		word("Order", 50, 220, 38), word("submitted", 92, 220, 60), word("on:", 156, 220, 20),
		word("Tuesday", 180, 220, 48), word("7", 232, 220, 6), word("Apr", 242, 220, 20),
		word("2026", 266, 220, 26),
	)
	result := parseFixture(t, pageOne, pageTwo, pageThree)

	assertField(t, result, apicontract.ExtractionFieldPONumber, "4500123456", apicontract.ExtractionStatusExtracted)
	poDate := assertField(t, result, apicontract.ExtractionFieldPODate, "2026-04-07", apicontract.ExtractionStatusExtracted)
	if poDate.Page == nil || *poDate.Page != 3 {
		t.Errorf("poDate page = %v; the order date is printed on page 3", poDate.Page)
	}
	assertField(t, result, apicontract.ExtractionFieldCurrency, "IDR", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPaymentTerms, "NET 90", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "DDP", apicontract.ExtractionStatusExtracted)

	delivery := assertField(t, result, apicontract.ExtractionFieldRequestedDeliveryDate, "2027-01-31", apicontract.ExtractionStatusNeedsReview)
	if !strings.Contains(strings.ToLower(delivery.Note), "line") {
		t.Errorf("delivery note = %q; want the first line named as the evidence", delivery.Note)
	}
	if delivery.Page == nil || *delivery.Page != 2 {
		t.Errorf("delivery page = %v; want the row's page 2", delivery.Page)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d; want 2 (%+v)", len(result.Rows), result.Rows)
	}
	first := result.Rows[0]
	if first.LineNumber.Value != "1" || first.Quantity.Value != "1" || first.UoM.Value != "PK" {
		t.Errorf("row 1 identity = %+v; want line 1, qty 1, uom PK", first)
	}
	wantDescription := "BAT3 Auto-Message Alert for Scrubber Scope of Work refer 15020843"
	if first.Description.Value != wantDescription {
		t.Errorf("row 1 description = %q; want %q", first.Description.Value, wantDescription)
	}
	if strings.Contains(first.Description.Value, "Not Available") ||
		strings.Contains(first.Description.Value, "Material") {
		t.Errorf("row 1 description = %q; the part placeholder and non-data columns must not survive", first.Description.Value)
	}
	if first.UnitPrice.Value != "145760000.00" || first.Amount.Value != "145760000.00" {
		t.Errorf("row 1 money = %q/%q; want the currency-wrapped prices normalised",
			first.UnitPrice.Value, first.Amount.Value)
	}
	if first.UoM.Value == "PK 31 Jan 2027" || strings.Contains(first.UoM.Value, "2027") {
		t.Errorf("row 1 uom = %q; the delivery date must not become a unit", first.UoM.Value)
	}

	second := result.Rows[1]
	if second.LineNumber.Value != "2" || second.Description.Value != "Installation and commissioning" ||
		second.Quantity.Value != "2" || second.UoM.Value != "EA" ||
		second.UnitPrice.Value != "1500000.00" || second.Amount.Value != "3000000.00" {
		t.Errorf("row 2 = %+v; want the repeated header to start a readable row, not a row of its own", second)
	}
}

// rightBlockPageOne is a different layout: the labels and their values sit in a
// right-hand two-column block, a value may be on the next line, and the table
// has no line-number column.
func rightBlockPageOne() []testWord {
	return []testWord{
		word("Supplier", 50, 70, 45), word("PT", 50, 85, 14), word("Contoh", 66, 85, 40),
		word("Batam", 50, 100, 34), word("Indonesia", 50, 115, 50),
		word("Purchase", 350, 70, 55), word("Order", 408, 70, 40), word("Number:", 450, 70, 50),
		word("PO-2026-7781", 350, 85, 70),
		word("Order", 350, 110, 30), word("Date:", 382, 110, 26),
		word("15", 350, 125, 12), word("March", 364, 125, 32), word("2026", 398, 125, 24),
		word("Currency:", 350, 150, 50), word("USD", 404, 150, 22),
		word("Payment", 350, 175, 48), word("Terms:", 400, 175, 32),
		word("Net", 350, 190, 18), word("45", 370, 190, 10),
		word("Incoterms:", 350, 215, 55), word("FOB", 350, 230, 20), word("Singapore", 372, 230, 45),
		word("Requested", 350, 255, 50), word("Delivery", 402, 255, 40), word("Date:", 444, 255, 26),
		word("30", 350, 270, 12), word("April", 364, 270, 28), word("2026", 394, 270, 24),
	}
}

func rightBlockTableHeader(y float64) []testWord {
	return []testWord{
		word("Item", 50, y, 22), word("Description", 74, y, 55),
		word("Quantity", 300, y, 45), word("Unit", 360, y, 25),
		word("Unit", 400, y, 20), word("Price", 422, y, 25),
		word("Total", 500, y, 30),
	}
}

func rightBlockRow(description string, descriptionX []float64, qty, uom, price, amount string, y float64) []testWord {
	words := make([]testWord, 0, 8)
	for index, part := range strings.Split(description, " ") {
		words = append(words, word(part, descriptionX[index], y, 36))
	}
	return append(words,
		word(qty, 300, y, 10), word(uom, 360, y, 22),
		word(price, 400, y, 35), word(amount, 500, y, 35), word("USD", 538, y, 22),
	)
}

func TestProposeReadsARightHandBlockAndATableWithoutLineNumbers(t *testing.T) {
	pageOne := rightBlockPageOne()
	pageOne = append(pageOne, rightBlockTableHeader(400)...)
	pageOne = append(pageOne, rightBlockRow("Stainless steel flange",
		[]float64{50, 98, 132}, "4", "PCS", "125.50", "502.00", 420)...)
	pageOne = append(pageOne, word("assembly", 50, 435, 42))
	pageOne = append(pageOne, rightBlockRow("Gasket kit",
		[]float64{50, 96}, "12", "PK", "8.25", "99.00", 455)...)

	pageTwo := []testWord{
		word("Page", 500, 40, 30), word("2", 535, 40, 8), word("of", 545, 40, 12), word("2", 560, 40, 8),
	}
	pageTwo = append(pageTwo, rightBlockRow("Bearing",
		[]float64{50}, "2", "EA", "45.00", "90.00", 100)...)
	result := parseFixture(t, pageOne, pageTwo)

	assertField(t, result, apicontract.ExtractionFieldPONumber, "PO-2026-7781", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-03-15", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldCurrency, "USD", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPaymentTerms, "Net 45", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "FOB Singapore", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldRequestedDeliveryDate, "2026-04-30", apicontract.ExtractionStatusExtracted)

	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d; want 3 across the page break (%+v)", len(result.Rows), result.Rows)
	}
	first := result.Rows[0]
	if first.Description.Value != "Stainless steel flange assembly" ||
		first.Quantity.Value != "4" || first.UoM.Value != "PCS" ||
		first.UnitPrice.Value != "125.50" || first.Amount.Value != "502.00" {
		t.Errorf("row 1 = %+v; want the wrapped description and the USD-suffixed total read", first)
	}
	if first.LineNumber.Status != apicontract.ExtractionStatusAbsent {
		t.Errorf("row 1 line number = %+v; a table without that column must leave it absent", first.LineNumber)
	}
	third := result.Rows[2]
	if third.Description.Value != "Bearing" || third.Page != 2 {
		t.Errorf("row 3 = %+v; want the continued page 2 row", third)
	}
	if third.Status != apicontract.ExtractionStatusNeedsReview ||
		!strings.Contains(third.Note, "column header is on page 1") {
		t.Errorf("row 3 = %+v; want needs_review naming the carried header", third)
	}
	if strings.Contains(result.Rows[1].Description.Value, "Page") {
		t.Errorf("page 2's running header was appended to page 1's last row: %q",
			result.Rows[1].Description.Value)
	}
	if first.Status != apicontract.ExtractionStatusExtracted {
		t.Errorf("row 1 status = %q; want extracted", first.Status)
	}
}

// indonesianPageOne prints Indonesian header labels and no Incoterms.
func indonesianPageOne() []testWord {
	return []testWord{
		word("Nomor", 50, 60, 40), word("PO:", 94, 60, 22), word("15020843", 122, 60, 55),
		word("Tanggal", 50, 80, 45), word("PO:", 99, 80, 22),
		word("2", 127, 80, 6), word("April", 136, 80, 30), word("2026", 170, 80, 27),
		word("Mata", 50, 100, 26), word("Uang:", 80, 100, 32), word("IDR", 118, 100, 22),
		word("Syarat", 50, 120, 36), word("Pembayaran:", 90, 120, 64), word("NET", 162, 120, 24),
		word("30", 189, 120, 10),
	}
}

func indonesianTableHeader(y float64) []testWord {
	return []testWord{
		word("Particulars", 50, y, 60),
		word("Rate", 300, y, 25), word("Qty", 360, y, 18),
		word("UOM", 400, y, 22), word("Amount", 480, y, 40),
	}
}

func TestProposeReadsAReorderedTableWithIndonesianLabels(t *testing.T) {
	pageOne := indonesianPageOne()
	pageTwo := indonesianTableHeader(100)
	pageTwo = append(pageTwo,
		word("Jasa", 50, 125, 22), word("instalasi", 76, 125, 40),
		word("250,000.00", 300, 125, 50), word("3", 360, 125, 8),
		word("OH", 400, 125, 16), word("750,000.00", 480, 125, 50), word("IDR", 540, 125, 20),
		word("listrik", 50, 143, 30),
		word("Material", 50, 161, 42), word("kabel", 96, 161, 30),
		word("1.250,00", 300, 161, 45), word("10", 360, 161, 12),
		word("M", 400, 161, 8), word("12.500,00", 480, 161, 45), word("IDR", 540, 161, 20),
	)
	pageThree := []testWord{
		word("Halaman", 500, 40, 48), word("3", 552, 40, 8),
		word("Transport", 50, 100, 50),
		word("500,000.00", 300, 100, 50), word("1", 360, 100, 6),
		word("Lot", 400, 100, 20), word("500,000.00", 480, 100, 50), word("IDR", 540, 100, 20),
	}
	result := parseFixture(t, pageOne, pageTwo, pageThree)

	assertField(t, result, apicontract.ExtractionFieldPONumber, "15020843", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-04-02", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldCurrency, "IDR", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPaymentTerms, "NET 30", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "", apicontract.ExtractionStatusAbsent)

	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d; want 3 (%+v)", len(result.Rows), result.Rows)
	}
	first := result.Rows[0]
	if first.Description.Value != "Jasa instalasi listrik" || first.Quantity.Value != "3" ||
		first.UoM.Value != "OH" || first.UnitPrice.Value != "250000.00" ||
		first.Amount.Value != "750000.00" {
		t.Errorf("row 1 = %+v; want the reordered columns read by their header", first)
	}
	second := result.Rows[1]
	if second.UnitPrice.Value != "1250.00" || second.Amount.Value != "12500.00" {
		t.Errorf("row 2 money = %q/%q; want the Indonesian grouping normalised",
			second.UnitPrice.Value, second.Amount.Value)
	}
	if result.Rows[2].Status != apicontract.ExtractionStatusNeedsReview ||
		!strings.Contains(result.Rows[2].Note, "column header is on page 2") {
		t.Errorf("row 3 = %+v; want needs_review naming the carried header", result.Rows[2])
	}
}

func TestProposeStripsAPartNumberDescriptionWhenTheRealDescriptionFollows(t *testing.T) {
	words := []testWord{
		word("Line", 50, 200, 24), word("#", 76, 200, 6),
		word("Part", 120, 200, 22), word("#", 144, 200, 6), word("/", 152, 200, 5),
		word("Description", 160, 200, 55),
		word("Qty", 300, 200, 18), word("Unit", 350, 200, 20), word("Price", 372, 200, 25),
		word("1", 50, 225, 6),
		word("PN-7781", 125, 225, 40),
		word("2", 300, 225, 8), word("10.00", 350, 225, 35),
		word("Steel", 125, 243, 28), word("bracket", 156, 243, 35),
	}
	result := parseFixture(t, words)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want 1 (%+v)", len(result.Rows), result.Rows)
	}
	description := result.Rows[0].Description
	if description.Value != "Steel bracket" || description.Raw != "Steel bracket" {
		t.Errorf("description = %q (raw %q); want the part number replaced by the printed description",
			description.Value, description.Raw)
	}
	if strings.Contains(description.Value, "PN-7781") {
		t.Errorf("description = %q; the part number must not be part of the description", description.Value)
	}
}

func TestProposeNeverInfersAnAmountFromQuantityAndPrice(t *testing.T) {
	words := append([]testWord{}, tableHeaderWords()...)
	words = append(words,
		word("1", 50, 225, 6), word("Panel", 90, 225, 40),
		word("2", 300, 225, 8), word("Pc", 350, 225, 20),
		word("10.00", 400, 225, 35), word("25.00", 500, 225, 35),
	)
	result := parseFixture(t, words)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d; want 1", len(result.Rows))
	}
	if result.Rows[0].Amount.Value != "25.00" {
		t.Errorf("amount = %q; the printed amount 25.00 must not be recomputed as 2 x 10.00",
			result.Rows[0].Amount.Value)
	}
}

// TestProposeReadsSeveralLineItemBlocksOnOnePage holds the generic rules that
// the multi-block Ariba pages depend on: every block repeats its own header, a
// STATUS block between blocks is not a row, a status note is not the delivery
// header, and a "Note:" line printed inside the description column is
// description text rather than a table footer.
func TestProposeReadsSeveralLineItemBlocksOnOnePage(t *testing.T) {
	pageOne := []testWord{
		word("Purchase", 50, 60, 80), word("Order:", 134, 60, 42), word("9001002003", 180, 60, 62),
		word("Payment", 50, 90, 48), word("Terms:", 102, 90, 34), word("NET", 140, 90, 24), word("60", 168, 90, 10),
		word("Incoterm", 50, 110, 50), word("Code:", 104, 110, 30), word("DAP", 138, 110, 24),
	}

	pageTwo := aribaLikeTableHeader(100)
	pageTwo = append(pageTwo, aribaNumericRow("1", "2", "(EA)", "10", "Jun", "2026", "1,000.00", "2,000.00", 120)...)
	pageTwo = append(pageTwo,
		word("Pump", 185, 138, 30), word("with", 220, 138, 24), word("mounting", 248, 138, 50),
		word("kit", 270, 138, 20),
		word("STATUS", 60, 168, 38),
		word("Confirmed", 74, 186, 50), word("(Estimated", 128, 186, 55), word("Delivery", 188, 186, 40),
		word("Date:", 232, 186, 28), word("20", 264, 186, 12), word("Jun", 280, 186, 16),
		word("2026)", 300, 186, 28),
	)
	pageTwo = append(pageTwo, aribaLikeTableHeader(220)...)
	pageTwo = append(pageTwo, aribaNumericRow("2", "3", "(EA)", "12", "Jun", "2026", "500.00", "1,500.00", 240)...)
	pageTwo = append(pageTwo,
		word("Valve", 185, 258, 30),
		word("Note:", 185, 276, 30), word("includes", 220, 276, 42), word("gasket", 266, 276, 36),
		word("STATUS", 60, 300, 38),
	)

	pageThree := []testWord{
		word("Order", 50, 60, 38), word("submitted", 92, 60, 60), word("on:", 156, 60, 20),
		word("Monday", 180, 60, 45), word("1", 230, 60, 6), word("Jun", 240, 60, 20),
		word("2026", 264, 60, 26),
	}
	result := parseFixture(t, pageOne, pageTwo, pageThree)

	assertField(t, result, apicontract.ExtractionFieldPONumber, "9001002003", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPODate, "2026-06-01", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldPaymentTerms, "NET 60", apicontract.ExtractionStatusExtracted)
	assertField(t, result, apicontract.ExtractionFieldIncoterms, "DAP", apicontract.ExtractionStatusExtracted)

	delivery := assertField(t, result, apicontract.ExtractionFieldRequestedDeliveryDate, "2026-06-10", apicontract.ExtractionStatusNeedsReview)
	if !strings.Contains(delivery.Note, "first line") {
		t.Errorf("delivery note = %q; want the first line named", delivery.Note)
	}
	if strings.Contains(delivery.Value, "2026-06-20") {
		t.Errorf("delivery = %q; the STATUS note's estimated date must not become the header", delivery.Value)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d; want 2 (%+v)", len(result.Rows), result.Rows)
	}
	first := result.Rows[0]
	if first.Description.Value != "Pump with mounting kit" ||
		first.Quantity.Value != "2" || first.UoM.Value != "EA" ||
		first.UnitPrice.Value != "1000.00" || first.Amount.Value != "2000.00" {
		t.Errorf("row 1 = %+v; want the wrapped description and the first block's values", first)
	}
	second := result.Rows[1]
	if second.LineNumber.Value != "2" || second.Description.Value != "Valve Note: includes gasket" ||
		second.Quantity.Value != "3" || second.UnitPrice.Value != "500.00" {
		t.Errorf("row 2 = %+v; want the second block and its in-description Note kept", second)
	}
	for _, row := range result.Rows {
		if strings.Contains(row.Description.Value, "STATUS") ||
			strings.Contains(row.Description.Value, "Confirmed") {
			t.Errorf("row %s description = %q; the status block is not a line item",
				row.LineNumber.Value, row.Description.Value)
		}
	}
}

func TestNormaliseDateReadsWeekdayAndTimeButRefusesTwoDates(t *testing.T) {
	cases := []struct {
		raw    string
		value  string
		status string
	}{
		{"Tuesday 7 Apr 2026 8:02 AM GMT+07:00", "2026-04-07", apicontract.ExtractionStatusExtracted},
		{"7 April 2026", "2026-04-07", apicontract.ExtractionStatusExtracted},
		{"31 Jan 2027", "2027-01-31", apicontract.ExtractionStatusExtracted},
		{"From 1 Jan 2026 to 2 Feb 2026", "From 1 Jan 2026 to 2 Feb 2026", apicontract.ExtractionStatusNeedsReview},
	}
	for _, testCase := range cases {
		value, status, _ := normaliseDate(testCase.raw)
		if value != testCase.value || status != testCase.status {
			t.Errorf("normaliseDate(%q) = %q/%q; want %q/%q",
				testCase.raw, value, status, testCase.value, testCase.status)
		}
	}
}
