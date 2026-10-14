package gpdf

// Guard: the native terms card must be the template's terms card.
//
// The template's .tnc block no longer carries the sentences -- #339 moved the
// wording into backend/labels, so the template interpolates the heading and
// prints the four sentences the seam hands it. This file therefore pins two
// things instead of the old one:
//
//  1. the template's .tnc block carries no wording of its own (it reads from
//     the seam), so a template-only change cannot pass silently, and
//  2. the four sentences this engine prints are the ones the seam carried,
//     substituted by the application -- including that the service category
//     never becomes a fifth term.
//
// The defaults those sentences come from are pinned against the pre-#339
// literals in backend/labels/labels_test.go, which is where a wording change
// has to be made and therefore where it is checked.

import (
	"os"
	"strings"
	"testing"

	"vera-app/backend/labels"
	"vera-app/backend/render/quotation"
)

// templateTermsPath is the source of truth, relative to this package's
// directory (go test runs with the package directory as the working
// directory; layout_test.go already relies on the same with ../../assets).
const templateTermsPath = "../../template/html/template-fixed.html"

// readTemplateTnC returns the .tnc block's inner markup.
func readTemplateTnC(t *testing.T) string {
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
	end := strings.Index(rest, "</div>")
	if end < 0 {
		t.Fatalf("template %s .tnc block never closes", templateTermsPath)
	}
	return rest[:end]
}

// TestTemplateTermsCarryNoWordingOfTheirOwn is the other half of the old
// guard: the block must take its heading from the label set and its sentences
// from the seam, because a sentence hard-coded back into the template would
// print in the browser and not in this engine -- the divergence #339 exists to
// end.
func TestTemplateTermsCarryNoWordingOfTheirOwn(t *testing.T) {
	block := readTemplateTnC(t)
	if !strings.Contains(block, "{{.Labels.TermsHeading}}") {
		t.Errorf("the .tnc block does not interpolate the terms heading from the label set:\n%s", block)
	}
	if !strings.Contains(block, "{{range .Terms}}") {
		t.Errorf("the .tnc block does not print the seam's terms sentences:\n%s", block)
	}
	for _, sentence := range []string{"Payment terms", "valid for", "Lead time is", "Incoterms:"} {
		if strings.Contains(block, sentence) {
			t.Errorf("the .tnc block hard-codes %q again; the wording lives in backend/labels", sentence)
		}
	}
}

// TestNativeTermsMatchTheSeam pins that this engine prints exactly the
// sentences the seam carried: four of them, in order, with the service
// category among them nowhere.
func TestNativeTermsMatchTheSeam(t *testing.T) {
	base := quotation.Input{
		PaymentTerm:     90,
		Validity:        30,
		DisplayLeadTime: "60 Days",
		Incoterm:        "FOB",
		// The seam carries the substituted sentences, exactly as the
		// application's renderInput builds them from the label set.
		Terms: labels.DefaultLabels().Terms(labels.TermsValues{
			PaymentTerm: 90,
			Validity:    30,
			LeadTime:    "60 Days",
			Incoterm:    "FOB",
		}),
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
		diffTerms(t, "seam terms with ServiceCategory "+quoteCategory(category), in.Terms, wantNative)
		for _, term := range in.Terms {
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

// TestRewordedTermsReachTheEngine pins the point of #339: a reworded sentence
// on the seam prints, so a company's wording reaches the native PDF rather
// than being replaced by this engine's own.
func TestRewordedTermsReachTheEngine(t *testing.T) {
	set := labels.DefaultLabels()
	set.TermsIncoterm = "4. Trading term: {incoterm}"
	in := sampleInput()
	in.Terms = set.Terms(labels.TermsValues{Incoterm: "FOB"})
	if in.Terms[3] != "4. Trading term: FOB" {
		t.Fatalf("substituted term = %q; want the reworded sentence", in.Terms[3])
	}
	e := testEngine(t)
	pages := e.layout(in)
	var drawn []string
	for _, pg := range pages {
		for _, prim := range pg.prims {
			if r, ok := prim.(run); ok {
				drawn = append(drawn, r.text)
			}
		}
	}
	if !containsString(drawn, "4. Trading term: FOB") {
		t.Errorf("the engine did not print the reworded term; it drew %q", drawn)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
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
	var heights []float64
	for _, category := range []string{"", "NonConstruction", "Construction"} {
		// The application substitutes the terms from the label set before the
		// seam, so the engine's input carries the four sentences whatever the
		// tax treatment was -- this is the whole reason the height cannot move
		// with the category.
		in := quotation.Input{
			PaymentTerm:     90,
			Validity:        30,
			DisplayLeadTime: "60 Days",
			Incoterm:        "FOB",
			ServiceCategory: category,
			Terms: labels.DefaultLabels().Terms(labels.TermsValues{
				PaymentTerm: 90, Validity: 30, LeadTime: "60 Days", Incoterm: "FOB",
			}),
		}
		eng := sampleInput()
		eng.Terms = in.Terms
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
