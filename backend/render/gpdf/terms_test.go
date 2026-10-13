package gpdf

// Guard: the native terms card must be the template's terms card.
//
// Coupled to backend/template/html/template-fixed.html's .tnc block, which
// hard-codes four <p> terms and no conditional. If the template gains or
// loses a term deliberately, update the expectations here alongside it; a
// mismatch fails naming the extra or missing term.
//
// The template half is read from disk rather than restated blindly, so a
// template-only change cannot pass silently. termsLines is then asserted
// against the same four with values substituted, for every ServiceCategory
// the application can hand it -- including the two raw enums
// ("NonConstruction", "Construction") that once reached the customer PDF
// as a native-only fifth term. The fifth term is gone on purpose: AGENTS.md
// §14 names the template the source of truth, and the template prints four.

import (
	"os"
	"strings"
	"testing"

	"vera-app/backend/render/quotation"
)

// templateTermsPath is the source of truth, relative to this package's
// directory (go test runs with the package directory as the working
// directory; layout_test.go already relies on the same with ../../assets).
const templateTermsPath = "../../template/html/template-fixed.html"

// wantTemplateTerms is the .tnc block's four terms exactly as the template
// carries them, placeholders included.
//
// The lead time line prints the composed sentence -- one string built by the
// application layer, so the template's {{.LeadTime}} and the native renderer's
// in.DisplayLeadTime read the same words.
func wantTemplateTerms() []string {
	return []string{
		"1. Payment terms agreed for this quotation is {{.PaymentTerm}} days.",
		"2. This quotation is valid for {{.Validity}} days.",
		"3. Lead time is {{.LeadTime}}.",
		"4. Incoterms: {{.Incoterm}}",
	}
}

// readTemplateTerms extracts the <p> inners of the template's .tnc block.
func readTemplateTerms(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(templateTermsPath)
	if err != nil {
		t.Fatalf("read template terms from %s: %v", templateTermsPath, err)
	}
	html := string(raw)
	start := strings.Index(html, `<div class="tnc">`)
	if start < 0 {
		t.Fatalf("template %s carries no <div class=\"tnc\"> block", templateTermsPath)
	}
	rest := html[start:]
	end := strings.Index(rest, `</div>`)
	if end < 0 {
		t.Fatalf("template %s .tnc block never closes", templateTermsPath)
	}
	block := rest[:end]
	var out []string
	for {
		i := strings.Index(block, "<p>")
		if i < 0 {
			break
		}
		block = block[i+len("<p>"):]
		j := strings.Index(block, "</p>")
		if j < 0 {
			t.Fatalf("template %s .tnc block has an unclosed <p>", templateTermsPath)
		}
		out = append(out, strings.TrimSpace(block[:j]))
		block = block[j+len("</p>"):]
	}
	return out
}

// diffTerms fails naming each extra, missing, or differing term.
func diffTerms(t *testing.T, what string, got, want []string) {
	t.Helper()
	n := len(got)
	if len(want) > n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		switch {
		case i >= len(want):
			t.Errorf("%s term %d extra: %q (want %d terms)", what, i+1, got[i], len(want))
		case i >= len(got):
			t.Errorf("%s term %d missing: want %q (got %d terms)", what, i+1, want[i], len(got))
		case got[i] != want[i]:
			t.Errorf("%s term %d differs:\n got %q\nwant %q", what, i+1, got[i], want[i])
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s carries %d terms, want %d", what, len(got), len(want))
	}
}

// TestNativeTermsMatchTheTemplate pins the two renderers' terms lists as
// agreeing, by count and by content.
func TestNativeTermsMatchTheTemplate(t *testing.T) {
	templateTerms := readTemplateTerms(t)
	diffTerms(t, "template .tnc block", templateTerms, wantTemplateTerms())
	for _, term := range templateTerms {
		if strings.Contains(term, "Service category") || strings.Contains(term, "ServiceCategory") {
			t.Errorf("template term reaches for the service category: %q", term)
		}
	}

	base := quotation.Input{
		PaymentTerm:     90,
		Validity:        30,
		DisplayLeadTime: "60 Days",
		Incoterm:        "FOB",
	}
	wantNative := []string{
		"1. Payment terms agreed for this quotation is 90 days.",
		"2. This quotation is valid for 30 days.",
		"3. Lead time is 60 Days.",
		"4. Incoterms: FOB",
	}
	for _, category := range []string{"", "NonConstruction", "Construction"} {
		in := base
		in.ServiceCategory = category
		got := termsLines(in)
		diffTerms(t, "native termsLines with ServiceCategory "+quoteCategory(category), got, wantNative)
		for _, term := range got {
			if strings.Contains(term, "Service category") {
				t.Errorf("native term prints a service-category line with category %s: %q",
					quoteCategory(category), term)
			}
			if strings.Contains(term, "NonConstruction") || strings.Contains(term, "Construction") {
				t.Errorf("native term leaks a raw service-category enum with category %s: %q",
					quoteCategory(category), term)
			}
		}
	}
}

func quoteCategory(s string) string {
	if s == "" {
		return "(empty)"
	}
	return `"` + s + `"`
}

// TestTermsHeightIgnoresServiceCategory pins the pagination consequence:
// the terms card's height must not depend on the service category, so a
// services quotation lays out exactly like a goods one.
func TestTermsHeightIgnoresServiceCategory(t *testing.T) {
	e := testEngine(t)
	base := quotation.Input{
		PaymentTerm:     90,
		Validity:        30,
		DisplayLeadTime: "60 Days",
		Incoterm:        "FOB",
	}
	var heights []float64
	for _, category := range []string{"", "NonConstruction", "Construction"} {
		in := base
		in.ServiceCategory = category
		eng := sampleInput()
		eng.Terms = termsLines(in)
		m := e.derive(eng)
		heights = append(heights, e.tncHeight(eng, m.footerTncW))
	}
	for i := 1; i < len(heights); i++ {
		if heights[i] != heights[0] {
			t.Errorf("terms card is %.3fpt with an empty category but %.3fpt otherwise: pagination moves with the tax treatment",
				heights[0], heights[i])
		}
	}
	t.Logf("terms card %.3fpt for every service category", heights[0])
}
