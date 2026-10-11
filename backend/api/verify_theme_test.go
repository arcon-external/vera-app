package api

import (
	"os"
	"strings"
	"testing"

	"vera-app/backend/theme"
)

// TestPortalStatusColoursAreFixed proves a rebrand cannot restyle a verdict.
// The status set (valid/superseded/mismatch/not-found) stays literal in the
// template by design; this test fails if any of those values moves.
func TestPortalStatusColoursAreFixed(t *testing.T) {
	var b strings.Builder
	view := verifyView{Verdict: verdictValid, Portal: theme.DefaultTheme().ResolvePortal()}
	if err := verifyTemplate.ExecuteTemplate(&b, "verify_template.html", view); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"--ok: #1B7F3B;",
		"--ok-bg: #eef8f1;",
		"--warn: #9A6100;",
		"--warn-bg: #fdf6e7;",
		"--bad: #B3261E;",
		"--bad-bg: #fdeeed;",
		"--neutral: #5b6478;",
		"--neutral-bg: #f4f6fa;",
		".verdict-valid { border-color: #bfe0c8; background: var(--ok-bg); }",
		".verdict-superseded { border-color: #e8d9a8; background: var(--warn-bg); }",
		".verdict-mismatch { border-color: #f0c2be; background: var(--bad-bg); }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("portal output changed status styling %q", want)
		}
	}
}

// TestPortalBrandReachesTemplate proves the four portal brand colours land
// on the page as derived roles, and that ThemePSoft — previously defined
// but consumed nowhere — now drives the lookup panel and the page gradient.
func TestPortalBrandReachesTemplate(t *testing.T) {
	execute := func(t *testing.T, p theme.PortalPalette) string {
		t.Helper()
		var b strings.Builder
		view := verifyView{Verdict: verdictValid, Portal: p}
		if err := verifyTemplate.ExecuteTemplate(&b, "verify_template.html", view); err != nil {
			t.Fatalf("execute: %v", err)
		}
		return b.String()
	}

	def := execute(t, theme.DefaultTheme().ResolvePortal())
	for _, want := range []string{
		"--deep-blue: #1A237E;",
		"--accent-blue: #284AF3;",
		"--ink: #1a1a1a;",
		"--edge: #C9D8FE;",
		"--muted: #3B3F65;",
		"--section: #EDF3FF;",
		".sheet-section { background: var(--section); }",
	} {
		if !strings.Contains(def, want) {
			t.Errorf("portal output missing brand role %q", want)
		}
	}
	if strings.Contains(def, "--soft-blue") {
		t.Error("portal output still defines --soft-blue, the dead variable ThemePSoft used to feed")
	}

	// A custom soft colour must move the panel and the page, which is the
	// whole point of the setting existing.
	soft := execute(t, theme.Theme{Soft: "#FF6B4A"}.ResolvePortal())
	for _, want := range []string{
		"--section: #FFBCAE;",
		"--page-0: #FFD6CC;",
		"--page-1: #FFB19F;",
	} {
		if !strings.Contains(soft, want) {
			t.Errorf("portal output with custom soft missing %q", want)
		}
	}
	if strings.Contains(soft, "--section: #EDF3FF;") {
		t.Error("portal section did not move with ThemePSoft")
	}
}

// TestPortalTemplateHasNoDeadBrandActions proves the old per-variable
// fallbacks are gone: brand colours reach the template only through the
// resolved portal palette now.
func TestPortalTemplateHasNoDeadBrandActions(t *testing.T) {
	raw, err := os.ReadFile("verify_template.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	for _, want := range []string{".ThemePDeep", ".ThemePAccent", ".ThemePSoft", ".ThemePInk"} {
		if strings.Contains(string(raw), want) {
			t.Errorf("template still references %s; brand colours must resolve through .Portal", want)
		}
	}
}
