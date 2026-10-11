package api

import (
	"net/http"
	"os"
	"regexp"
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
		"--paper: #FFFFFF;",
		"--edge: #C9D8FE;",
		"--muted: #3B3F65;",
		"--section: #EDF3FF;",
		"--field-rule: #E6EFFF;",
		"--label: #7E81A0;",
		"--strong: #292D4F;",
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
	// The field divider follows the soft knob too.
	if !strings.Contains(soft, "--field-rule:") || strings.Contains(soft, "--field-rule: #E6EFFF;") {
		t.Error("portal field-rule did not move with ThemePSoft")
	}

	// A custom deep colour must move the labels and values a rebrand left
	// behind in the old blue-grey.
	deep := execute(t, theme.Theme{Deep: "#0A1A5C"}.ResolvePortal())
	if strings.Contains(deep, "--label: #7E81A0;") || strings.Contains(deep, "--strong: #292D4F;") {
		t.Error("portal labels did not move with ThemePDeep")
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

// TestPortalTemplateHasNoNonStatusLiterals guards the single-source contract
// on the portal side: the style block must name no colour except the pinned
// status set. Every other paint resolves through a PortalPalette role, so a
// literal here means a spot a rebrand cannot reach — the field labels a
// rebrand left in the old blue-grey.
func TestPortalTemplateHasNoNonStatusLiterals(t *testing.T) {
	raw, err := os.ReadFile("verify_template.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	s := string(raw)
	style := s
	if i := strings.Index(s, "<style>"); i >= 0 {
		if j := strings.Index(s[i:], "</style>"); j >= 0 {
			style = s[i : i+j]
		}
	}
	allowed := map[string]bool{
		"1B7F3B": true, "EEF8F1": true, "9A6100": true, "FDF6E7": true,
		"B3261E": true, "FDEEED": true, "5B6478": true, "F4F6FA": true,
		"BFE0C8": true, "E8D9A8": true, "F0C2BE": true,
	}
	for _, m := range regexp.MustCompile(`#[0-9A-Fa-f]{3,8}\b`).FindAllString(style, -1) {
		key := strings.ToUpper(strings.TrimPrefix(m, "#"))
		if !allowed[key] {
			t.Errorf("portal template contains non-status colour literal %s; wire it through PortalPalette", m)
		}
	}
}

// TestPortalVarsResolve proves every var(--x) the portal page uses is defined
// in its :root block, the same silent-transparency trap as the quotation
// stylesheet.
func TestPortalVarsResolve(t *testing.T) {
	raw, err := os.ReadFile("verify_template.html")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	s := string(raw)
	defined := map[string]bool{}
	for _, m := range regexp.MustCompile(`--[a-zA-Z0-9-]+:`).FindAllString(s, -1) {
		defined[strings.TrimSuffix(m, ":")] = true
	}
	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`var\(\s*(--[a-zA-Z0-9-]+)`).FindAllStringSubmatch(s, -1) {
		used[m[1]] = true
	}
	for name := range used {
		if !defined[name] {
			t.Errorf("portal template uses var(%s) which is never defined", name)
		}
	}
}

// TestThemePSoftReachesPortalThroughVerifyDocument proves the soft knob
// travels the real render path: profile → verifyDocument → page. Checking the
// template for a var is not enough; this sets a custom ThemePSoft on the live
// profile, exports, and reads the served page.
func TestThemePSoftReachesPortalThroughVerifyDocument(t *testing.T) {
	schema := "vera_api_verify_psoft_path"
	resetTestSchema(t, schema)
	server, token := testQuotationServer(t, schema)
	attachedTestSigner(t, server, schema)

	code, _ := doJSON(t, server, http.MethodPut, "/v1/admin/company", token, map[string]any{
		"legalName": "PT Arcon Penta Persada", "themePSoft": "#FF6B4A",
	})
	if code != http.StatusOK {
		t.Fatalf("set ThemePSoft = %d; want 200", code)
	}
	shortID, _, _ := exportForVerification(t, server, token, schema)
	_, body := fetchVerifyPage(t, server, shortID)
	for _, want := range []string{"--section: #FFBCAE;", "--page-0: #FFD6CC;", "--page-1: #FFB19F;"} {
		if !strings.Contains(body, want) {
			t.Errorf("served verification page missing %q; ThemePSoft did not travel verifyDocument", want)
		}
	}
	if strings.Contains(body, "--section: #EDF3FF;") {
		t.Error("served page still carries the default section; the custom soft never arrived")
	}
}
