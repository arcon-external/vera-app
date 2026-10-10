package main

import (
	"os"
	"strings"
	"testing"
)

// verdictTemplateAction is the exact template action the parser depends on.
//
// The page states its verdict in one class on one element, and the parser reads
// that class. If the template ever emits it differently, these tests fail here
// rather than letting `vera verify` start answering the wrong thing in
// production -- which, for a tool whose job is telling a customer their document
// is genuine, is the failure that matters.
const verdictTemplateAction = `{{if .IsVerified}}verdict-valid{{else if .IsSuperseded}}verdict-superseded{{else if .IsMismatch}}verdict-mismatch{{else if .IsUnavailable}}verdict-unavailable{{else}}verdict-notfound{{end}}`

// renderedPage builds the page the server would send for one verdict, from the
// real template rather than a hand-written approximation of it.
//
// An approximation would be a test of the test: the point is to catch the
// template moving, and only the template can tell us that.
func renderedPage(t *testing.T, verdict string) string {
	t.Helper()
	raw, err := os.ReadFile("../../backend/api/verify_template.html")
	if err != nil {
		t.Fatalf("read the verification template: %v", err)
	}
	if !strings.Contains(string(raw), verdictTemplateAction) {
		t.Fatal("backend/api/verify_template.html no longer states its verdict in the " +
			"shape cmd/vera/commands_verify.go parses; update verdictMarker and these tests together")
	}
	return strings.Replace(string(raw), verdictTemplateAction, "verdict-"+verdict, 1)
}

func TestParseVerdictReadsTheRealTemplate(t *testing.T) {
	for _, verdict := range []string{"valid", "superseded", "mismatch", "unavailable", "notfound"} {
		t.Run(verdict, func(t *testing.T) {
			page := renderedPage(t, verdict)
			got, err := parseVerdict(page)
			if err != nil {
				t.Fatalf("parseVerdict(%s page): %v", verdict, err)
			}
			if got != verdict {
				t.Errorf("parseVerdict = %q; want %q", got, verdict)
			}
		})
	}
}

// TestParseVerdictIsNotFooledByTheStylesheet is the regression this parser was
// rewritten for.
//
// The page carries every verdict name in its stylesheet -- `.verdict-valid`,
// `.verdict-mismatch` and the rest -- so a pattern that looked for the name
// anywhere in the document matched a CSS rule. The first version of this parser
// did exactly that, and reported a code that does not exist as `valid`.
func TestParseVerdictIsNotFooledByTheStylesheet(t *testing.T) {
	page := renderedPage(t, "notfound")
	// The decoys have to actually be there, or this test proves nothing.
	for _, decoy := range []string{".verdict-valid", ".verdict-mismatch", ".verdict-superseded"} {
		if !strings.Contains(page, decoy) {
			t.Fatalf("expected %s in the stylesheet as a decoy; the page changed", decoy)
		}
	}
	if strings.Index(page, ".verdict-valid") > strings.Index(page, `class="verdict verdict-notfound"`) {
		// The decoy must come *first* for this to be a real regression test --
		// that is the order that made the naive parser pick the wrong one.
		t.Fatal("the stylesheet decoy no longer precedes the verdict element; " +
			"this test would pass even with the buggy parser")
	}

	got, err := parseVerdict(page)
	if err != nil {
		t.Fatalf("parseVerdict: %v", err)
	}
	if got != "notfound" {
		t.Fatalf("parseVerdict = %q; want notfound -- a missing document was reported as %q", got, got)
	}
}

func TestParseVerdictRefusesAmbiguity(t *testing.T) {
	cases := map[string]string{
		"no verdict at all":        `<html><body><p>Server error</p></body></html>`,
		"a stylesheet only":        `<html><head><style>.verdict-valid{color:green}</style></head><body></body></html>`,
		"two verdict elements":     `<div class="verdict verdict-valid"></div><div class="verdict verdict-mismatch"></div>`,
		"the word without a class": `<p>verdict-mismatch</p>`,
	}
	for name, page := range cases {
		t.Run(name, func(t *testing.T) {
			verdict, err := parseVerdict(page)
			if err == nil {
				t.Fatalf("parseVerdict accepted a page it cannot read, returning %q", verdict)
			}
			if verdict != "" {
				t.Errorf("parseVerdict returned %q alongside an error; it must return nothing", verdict)
			}
		})
	}
}

// TestParseVerdictToleratesExtraClasses keeps the parser from breaking on a
// purely cosmetic template change, which would be a false alarm rather than a
// false all-clear.
func TestParseVerdictToleratesExtraClasses(t *testing.T) {
	page := `<div class="verdict  verdict-superseded  raised">`
	got, err := parseVerdict(page)
	if err != nil {
		t.Fatalf("parseVerdict: %v", err)
	}
	if got != "superseded" {
		t.Errorf("parseVerdict = %q; want superseded", got)
	}
}
