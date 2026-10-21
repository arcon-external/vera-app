//go:build chromedp && !windows

// Comparing the two renderers on the same quotation.
//
// The native renderer's correctness claims are all measurements against a real
// browser, which means no unit test can check any of them. Pagination, glyph
// advances and wrap points are only knowable by rendering in both and diffing.
// This file is that diff, reduced to the assertions worth failing on.
//
// It needs a working Chrome, so it is opt-in: set VERA_RUN_CHROME_TESTS=1, the
// same switch backend/benchmarks_test.go uses. A green run without it means the
// browser tests did not run, not that the renderers agree.

package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
	"html/template"

	"vera-app/backend/currency"
	"vera-app/backend/labels"
	chromedprender "vera-app/backend/render/chromedp"
	gpdfrender "vera-app/backend/render/gpdf"
	"vera-app/backend/render/quotation"
)

// TestRenderersAgreeOnPagination is the assertion that matters most.
//
// Pagination is the one difference between these renderers that is not
// cosmetic. A row of pixels out of place produces a document that still looks
// right; a row on the wrong page produces a different document, so the same
// revision can reach a customer as two different quotations depending on which
// renderer produced it. RMSE cannot see this, and a pixel comparison would flag
// every glyph advance difference instead, so the fact is asserted directly.
//
// This is not hypothetical: the native renderer once fitted six rows onto page
// one where the browser fitted five, because a rounding difference of six
// pixels across the rows was enough to push the sixth item over.
func TestRenderersAgreeOnPagination(t *testing.T) {
	requireChrome(t)
	ctx := context.Background()
	in := equivalenceQuotation(t)

	browser, err := chromedprender.New().Render(ctx, in)
	if err != nil {
		t.Fatalf("browser render: %v", err)
	}
	native, err := gpdfrender.New().Render(ctx, in)
	if err != nil {
		t.Fatalf("native render: %v", err)
	}

	bPages, bFirst := pageShape(t, browser)
	nPages, nFirst := pageShape(t, native)

	if bPages != nPages {
		t.Errorf("page count differs: browser %d, native %d.\n"+
			"The two renderers are producing different documents, not the same document\n"+
			"drawn slightly differently.", bPages, nPages)
	}
	for i := range bFirst {
		if bFirst[i] != nFirst[i] {
			t.Errorf("page %d begins at item %d in the browser and %d in the native renderer",
				i+1, bFirst[i], nFirst[i])
		}
	}
	if bPages < 2 {
		t.Fatalf("the fixture paginated to %d page(s); it has to span two to test anything", bPages)
	}
}

// TestRenderersAgreeOnQuotationNumber guards the fact a reader checks first: that
// both renderers print the same quotation number. It is cheap, and it catches a
// renderer that dropped the header block entirely, which a page-count assertion
// would not notice.
func TestRenderersAgreeOnQuotationNumber(t *testing.T) {
	requireChrome(t)
	ctx := context.Background()
	in := equivalenceQuotation(t)

	for _, tc := range []struct {
		name string
		r    quotation.Renderer
	}{
		{"browser", chromedprender.New()},
		{"native", gpdfrender.New()},
	} {
		pdf, err := tc.r.Render(ctx, in)
		if err != nil {
			t.Fatalf("%s render: %v", tc.name, err)
		}
		if text := pdfText(t, pdf); !strings.Contains(text, in.QuoteNumber) {
			t.Errorf("%s render does not print the quotation number %s", tc.name, in.QuoteNumber)
		}
	}
}

// TestRenderersAgreeOnRewordedLabels is the #339 guard: a wording change has
// to reach both renderers, and the only way to prove it is to render one in
// each and read what they printed.
//
// The document title is the case the issue opens with -- QUOTATION renamed to
// PROPOSAL -- and it is also the one with geometry attached: the browser fills
// the eyebrow's row with a rule that starts after the label, so a longer or
// shorter title moves it. Both engines have to move it the same way, or the
// same revision becomes two different documents depending on who rendered it.
//
// The other assertions in this file run on the same fixture with the default
// wording, so pagination is checked on the default document and this on a
// reworded one: a title that broke the layout would break one of the two.
func TestRenderersAgreeOnRewordedLabels(t *testing.T) {
	requireChrome(t)
	ctx := context.Background()
	// The edit is made to the prepared data bag, before the input is built, so
	// the browser's HTML and the native renderer's seam both read it. Editing
	// the returned input's Labels alone would leave the HTML carrying the
	// built-in title and compare two different documents.
	in := equivalenceFixture(t, func(q *Quotation) {
		q.Data.Labels.DocTitle = "PROPOSAL"
		q.Data.Labels.Subject = "Scope"
	})

	browser, err := chromedprender.New().Render(ctx, in)
	if err != nil {
		t.Fatalf("browser render: %v", err)
	}
	native, err := gpdfrender.New().Render(ctx, in)
	if err != nil {
		t.Fatalf("native render: %v", err)
	}

	for _, tc := range []struct {
		name string
		pdf  []byte
	}{
		{"browser", browser},
		{"native", native},
	} {
		text := pdfText(t, tc.pdf)
		if !strings.Contains(text, "PROPOSAL") {
			t.Errorf("%s render does not print the reworded title PROPOSAL; "+
				"a wording edit reached one renderer and not the other", tc.name)
		}
		if !strings.Contains(text, "Scope") {
			t.Errorf("%s render does not print the reworded subject label Scope", tc.name)
		}
		if strings.Contains(text, "QUOTATION") {
			t.Errorf("%s render still prints the built-in title QUOTATION", tc.name)
		}
	}
}

// TestEquivalenceFixtureCarriesCurrency pins that the hand-built equivalence
// document carries its currency on both paths to the renderers: the template
// HTML (the browser's whole input) and the seam's Currency field (what the
// native renderer prints its headers and totals card from). It needs no
// browser, unlike the agreement tests above.
func TestEquivalenceFixtureCarriesCurrency(t *testing.T) {
	in := equivalenceQuotation(t)
	if in.Currency != "IDR" {
		t.Errorf("seam currency = %q; want IDR", in.Currency)
	}
	if !strings.Contains(in.HTML, "Unit Price IDR") {
		t.Errorf("fixture HTML does not carry an IDR unit-price header; " +
			"the browser would render a different document than the native renderer")
	}
}

// noColumnMaxX separates the item-number column from the indented bullet list
// below each description.
//
// Measured off a real render of the reference quotation: item numbers start
// between 68.8 and 71.5, because they are right-aligned and two-digit ones sit
// further left than one-digit ones. Bullets start at 109.5. Anything at 90
// therefore has 38 points of margin on either side, which is enough that a
// template tweak would have to be large to break it.
const noColumnMaxX = 90.0

// pdftotextWord is one extracted word with the position pdftotext reported.
// Only x is read: see pageShape.
type pdftotextWord struct {
	text string
	x    float64
}

// pageShape reports the page count and, per page, the lowest item number in the
// No column. That is the fact pagination turns on: two renderers that begin
// every page with the same item broke at the same place.
//
// Item numbers are located by x position alone, and no line grouping is needed
// to do it. Grouping lines by baseline was tried first and was actively harmful:
// Chrome's own line spacing puts adjacent baselines within a fraction of a point
// of each other, so a tolerance-based grouping merged two table rows and lost an
// item number, reporting a pagination mismatch that did not exist. The column is
// unambiguous on its own, so the test does not depend on guessing where one line
// ends.
func pageShape(t *testing.T, pdf []byte) (pages int, firstPerPage []int) {
	t.Helper()
	for _, words := range extractPages(t, pdf) {
		pages++

		lowest := 0
		for _, w := range words {
			if w.x >= noColumnMaxX || !bareInt(w.text) {
				continue
			}
			n, err := strconv.Atoi(w.text)
			if err != nil || n < 1 {
				continue
			}
			if lowest == 0 || n < lowest {
				lowest = n
			}
		}
		firstPerPage = append(firstPerPage, lowest)
	}
	return pages, firstPerPage
}

// extractPages runs pdftotext and returns each page's words.
//
// -bbox-layout is used rather than plain text because the column position is the
// whole point: without coordinates there is no way to tell an item number from
// the number that starts a bullet.
func extractPages(t *testing.T, pdf []byte) [][]pdftotextWord {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdftotext", "-bbox-layout", path, "-").Output()
	if err != nil {
		t.Skipf("pdftotext unavailable, cannot compare the renderers: %v", err)
	}

	var pages [][]pdftotextWord
	dec := xml.NewDecoder(bytes.NewReader(out))
	// A <word> element's text is the character data that follows it, so the
	// decoder has to be walked rather than scanned for start tags.
	var word *pdftotextWord
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse pdftotext output: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "page":
				pages = append(pages, nil)
			case "word":
				word = &pdftotextWord{}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "xMin":
						word.x, _ = strconv.ParseFloat(a.Value, 64)
					}
				}
				text.Reset()
			}
		case xml.CharData:
			if word != nil {
				text.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "word":
				if word != nil && len(pages) > 0 {
					word.text = strings.TrimSpace(text.String())
					pages[len(pages)-1] = append(pages[len(pages)-1], *word)
				}
				word, text = nil, strings.Builder{}
			case "page":
				word = nil
			}
		}
	}
	return pages
}

func bareInt(s string) bool {
	if s == "" || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// pdfText extracts a PDF's text with pdftotext.
func pdfText(t *testing.T, pdf []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := exec.Command("pdftotext", "-layout", path, "-")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Skipf("pdftotext unavailable, cannot compare the renderers: %v", err)
	}
	return out.String()
}

func requireChrome(t *testing.T) {
	t.Helper()
	if os.Getenv("VERA_RUN_CHROME_TESTS") == "" {
		t.Skip("set VERA_RUN_CHROME_TESTS=1; comparing the renderers needs a real browser")
	}
}

// fixtureItem is one line of the equivalence quotation.
type fixtureItem struct {
	No        int
	Qty       string
	UoM       string
	DescMain  string
	SubItems  []string
	UnitPrice string
	Total     string
}

func refItems() []fixtureItem {
	return []fixtureItem{
		{
			No:  1,
			Qty: "1", UoM: "Lot",
			DescMain: "Supply & Install lighting sensor system (PIR Schneider) at AHU block 3, " +
				"Mezzanine Front & Rear with following scopes:",
			SubItems: []string{
				"6 sensors at Front AHU and 6 sensors at Rear AHU",
				"6-units batten fixtures with guard (2x 1200mm LED tubes)",
				"Rewiring with Auto-Manual selector and lighting switch",
			},
			UnitPrice: "133.083.750", Total: "133.083.750",
		},
		{
			No:  2,
			Qty: "1", UoM: "Lot",
			DescMain: "Supply & Install lighting sensor system (PIR Schneider) 14 sensors " +
				"at CDA block 4.3",
			UnitPrice: "68.965.500", Total: "68.965.500",
		},
		{
			No:  3,
			Qty: "1", UoM: "Lot",
			DescMain: "Supply & Install lighting sensor system (PIR Schneider) 5 sensors " +
				"at office block 9",
			UnitPrice: "25.495.500", Total: "25.495.500",
		},
		{
			No:  4,
			Qty: "1", UoM: "Lot",
			DescMain:  "Supply and install aircond automation system at Mesjid Batam 1:",
			SubItems:  []string{"Metal enclosure panel with timers", "Zelio Schneider programmable controller", "Wiring and commissioning"},
			UnitPrice: "38.136.000", Total: "38.136.000",
		},
		{
			No:  5,
			Qty: "2", UoM: "Lot",
			DescMain: "Supply & install exhaust fan system at CDA block 2:",
			SubItems: []string{
				"4 units axial exhaust fan 18 1.5HP",
				"Galvanized ducting complete with damper and grille",
				"Electrical wiring and control panel",
			},
			UnitPrice: "31.625.000", Total: "63.250.000",
		},
		{
			No:  6,
			Qty: "12", UoM: "Lot",
			DescMain:  "Monthly preventive maintenance for HVAC system at main office building",
			UnitPrice: "9.180.000", Total: "110.160.000",
		},
		{
			No:  7,
			Qty: "48", UoM: "EA",
			DescMain: "Supply and delivery of LED batten light fixture 2x18W complete with:",
			SubItems: []string{
				"Emergency battery backup", "Surface mounting bracket",
				"3-year manufacturer warranty",
			},
			UnitPrice: "560.500", Total: "26.904.000",
		},
		{
			No:  8,
			Qty: "1", UoM: "Lot",
			DescMain: "Electrical installation and rewiring for production line extension:",
			SubItems: []string{
				"Main distribution board upgrade (200A)",
				"Cable tray and trunking installation",
				"Power point and lighting circuit wiring",
				"Testing and certification",
			},
			UnitPrice: "86.350.000", Total: "86.350.000",
		},
		{
			No:  9,
			Qty: "85", UoM: "Meter",
			DescMain:  "Supply, fabrication, and installation of cable ladder system:",
			SubItems:  []string{"Hot-dip galvanized steel, 600mm width", "Wall brackets and support arms", "Expansion joints and coupler plates"},
			UnitPrice: "476.000", Total: "40.460.000",
		},
		{
			No:  10,
			Qty: "1", UoM: "Lot",
			DescMain:  "Testing and commissioning services for entire electrical installation scope",
			UnitPrice: "16.830.000",
			Total:     "16.830.000",
		},
		{
			No:  11,
			Qty: "120", UoM: "PC",
			DescMain:  "Schneider MCB 1P 16A single pole circuit breaker",
			UnitPrice: "97.750",
			Total:     "11.730.000",
		},
		{
			No:  12,
			Qty: "250", UoM: "Meter",
			DescMain:  "PVC conduit pipe 25mm complete with fittings and saddles",
			UnitPrice: "20.700",
			Total:     "5.175.000",
		},
		{
			No:  13,
			Qty: "2", UoM: "Set",
			DescMain:  "Fire alarm control panel addressable 4-loop complete with battery backup",
			UnitPrice: "53.200.000",
			Total:     "106.400.000",
		},
	}
}

// descHTML renders a description's main half the way formatDescriptionMain
// does: the plain item's lines travel separately as numbered DescriptionLines
// (see quotationDataFromEngineInput), so the browser lays out the same
// document the native renderer is told about. Both renderers must be given
// the same document or the comparison measures two documents.
//
// The fixture text is trusted the way refItems is: it carries no markup or
// entities, so no escaping is applied here, exactly as before.
func descHTML(f fixtureItem) string {
	return `<div class="desc-main">` + f.DescMain + `</div>`
}

// descLines numbers a fixture item's plain description lines the way
// buildItems numbers production lines: X.Y from the item's final No.
func descLines(f fixtureItem) []quotation.DescriptionLine {
	if len(f.SubItems) == 0 {
		return nil
	}
	out := make([]quotation.DescriptionLine, 0, len(f.SubItems))
	for i, s := range f.SubItems {
		out = append(out, quotation.DescriptionLine{
			No:   fmt.Sprintf("%d.%d", f.No, i+1),
			Text: s,
		})
	}
	return out
}

// equivalenceQuotation builds the quotation the two renderers are compared on.
//
// It is 13 items spanning two pages, and that is the point: the defect it guards
// against only appears on a document whose first page is nearly full, so a
// short fixture would pass no matter how wrong the row heights were.
//
// Artwork comes from the real assets and the same QR encoder production uses,
// so the comparison needs no binary fixtures of its own.
func equivalenceQuotation(t *testing.T) quotation.Input {
	t.Helper()
	return equivalenceFixture(t, nil)
}

// equivalenceFixture is equivalenceQuotation with an edit applied to the
// prepared data bag before the renderer input is built from it.
//
// The hook exists for the wording tests (#339): a label edit has to land in the
// data bag, because that is where the HTML template is executed from and where
// renderInput takes the seam's copy. Editing the built input instead would
// reword the native renderer's document and leave the browser's HTML carrying
// the old string -- a test that then "proves" the two renderers disagree.
func equivalenceFixture(t *testing.T, edit func(*Quotation)) quotation.Input {
	t.Helper()

	// Read the logo the way production does, from the embedded assets rather
	// than off disk, so this fixture has no working-directory dependency of its
	// own to confuse a test that is about working directories.
	logo, err := backAssets.ReadFile("assets/arcon-logo.png")
	if err != nil {
		t.Fatal(err)
	}
	const verifyURL = "https://verify.example.invalid/v/2e51f59464af4b00"
	qrPNG, err := qrcode.Encode(verifyURL, qrcode.Medium, 256)
	if err != nil {
		t.Fatal(err)
	}

	q := &Quotation{}
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup: %v", err)
	}
	quotationDataFromEngineInput(t, q, logo, qrPNG)
	if edit != nil {
		edit(q)
		// The template reads the terms sentences off the data bag, so an edit
		// that changes the label set has to rebuild them the way prepare does.
		q.Data.Terms = q.termsLines()
	}

	in, err := q.renderInput()
	if err != nil {
		t.Fatalf("build renderer input: %v", err)
	}
	return in
}

// quotationDataFromEngineInput fills a QuotationData with the equivalence
// document. The totals are stated rather than computed: this asks whether the
// two renderers lay out the same document, and re-running the pricing engine
// would make a rounding difference look like a layout difference.
func quotationDataFromEngineInput(t *testing.T, q *Quotation, logo, qrPNG []byte) {
	t.Helper()

	d := &q.Data
	d.Subject = "Supply & Installation of Electrical and Mechanical Systems at PT Example Plant"
	d.Date = "September 23, 2026"
	d.ValidUntil = "October 23, 2026"
	d.CompanyName = "PT. Arcon Penta Persada"
	d.CompanyAddress = "Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam\n" +
		"Kota, Kota Batam, Provinsi Kepulauan Riau, 29431, Indonesia"
	d.LogoBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(logo)
	d.VerifyQRBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG)

	d.RecipientName = "Budi Santoso"
	d.RecipientCompany = "PT. Infineon Technologies Batam"
	d.RecipientAddress = "Komp. Ruko Legenda Gemilang, Blok D. NO 12, Baloi Permai, Batam\n" +
		"Kota, Kota Batam, Provinsi Kepulauan Riau, 29431, Indonesia"

	d.QuoteNumber = "1502260002"
	d.RevisionID = 0
	d.VerifyShortID = "2e51f59464af4b00"
	d.VerifyURL = verifyURLForTest

	d.Validity = 30
	d.PaymentTerm = 90
	// The terms line prints the composed sentence on both paths: this is the
	// QuotationData field the template reads, and renderInput carries the same
	// string to the native renderer, so the two documents cannot disagree
	// about what the lead time says.
	d.LeadTime = "60 Days"
	d.LeadTimeDays = 60
	d.DiscountPercent = 5
	d.Incoterm = "FOB"

	// The wording, carried explicitly rather than left implicit. Before #339
	// both renderers baked their own copy of every label, so this fixture
	// printed the defaults without saying so; now it states the set it renders
	// with, and TestRenderersAgreeOnRewordedLabels changes one of these labels
	// to prove a wording edit reaches both engines.
	d.Labels = labels.DefaultLabels()
	d.Terms = d.Labels.Terms(labels.TermsValues{
		PaymentTerm: d.PaymentTerm,
		Validity:    d.Validity,
		LeadTime:    d.LeadTime,
		Incoterm:    d.Incoterm,
	})

	// The hand-built document carries its currency like Prepare would set
	// it: an empty Currency renders "<th>Unit Price </th>" in the browser
	// while gpdf normalises empty to IDR, so the two renderers would receive
	// two different documents while asserting they agree.
	d.Currency = currency.Normalize("IDR")
	d.CurrencyPrefix, d.CurrencySuffix = currency.Affixes(d.Currency)

	d.DisplaySubTotal = "732.939.750"
	d.DisplayDiscountAmount = "36.646.987"
	d.DisplayPpnAmount = "76.592.203"
	d.DisplayGrandTotal = "772.884.966"

	for _, f := range refItems() {
		d.Items = append(d.Items, QuotationItem{
			No:               uint32(f.No),
			DisplayDesc:      template.HTML(descHTML(f)),
			DescriptionLines: descLines(f),
			UoM:              f.UoM,
			DisplayUnitPrice: f.UnitPrice,
			DisplayTotal:     f.Total,
			Qty:              mustAtoi(t, f.Qty),
		})
	}
}

const verifyURLForTest = "https://verify.example.invalid/v/2e51f59464af4b00"

func mustAtoi(t *testing.T, s string) uint32 {
	t.Helper()
	var n uint32
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		t.Fatalf("bad fixture quantity %q: %v", s, err)
	}
	return n
}
