package backend

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

// TestRenderedThemeRolesReachCSS proves every palette role lands in the
// executed stylesheet, so no paint in the document can sit outside the
// theme. It complements TestRenderedThemeColorsReachCSS, which pins the four
// bases: this one pins the thirteen derived roles plus the two fixed colours.
func TestRenderedThemeRolesReachCSS(t *testing.T) {
	q := newTestQuotation()
	q.SetAuthService(auth.StaffTestService())
	if err := Startup(q, context.Background()); err != nil {
		t.Fatalf("startup failed: %v", err)
	}
	if err := q.Prepare(testPreviewInput()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	css := string(q.Data.CustomCSS)
	for _, want := range []string{
		"--surface: #F8FAFF;",
		"--edge: #C9D8FE;",
		"--rule: #D3E2FF;",
		"--secondary: #3B3F65;",
		"--emphasis: #2C2F51;",
		"--sub-ink: #4D506F;",
		"--sub-no: #5E6178;",
		"--muted: #8E91B1;",
		"--rail: #B9C5E1;",
		"--decor-0: #DCE8FF;",
		"--decor-1: #CBDAFE;",
		"--page-0: #F4F7FF;",
		"--page-1: #E9F1FF;",
		"--paper: #FFFFFF;",
		"--green: #228B22;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("executed CSS missing derived role %q", want)
		}
	}
}

// TestStylesheetHasNoColourLiterals guards the single-source contract on the
// browser side: the stylesheet must name no colour directly. Every paint
// resolves through a :root variable executed from theme.Palette, so a hex
// literal here means a spot a rebrand cannot reach — the exact defect issue
// #335 removed. (The :root block itself carries {{.Palette.*}} actions, not
// literals, so the whole file must be clean.)
func TestStylesheetHasNoColourLiterals(t *testing.T) {
	raw, err := os.ReadFile("template/css/template-fixed.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	for _, m := range regexp.MustCompile(`#[0-9A-Fa-f]{3,8}\b`).FindAll(raw, -1) {
		t.Errorf("stylesheet contains colour literal %s; resolve it through a :root variable", m)
	}
}

// TestStylesheetVarsResolve proves every var(--x) the stylesheet uses is
// defined in its :root block. A typo makes the declaration invalid at
// computed-value time — the fill silently becomes transparent, with no failure
// anywhere — so the reference set must be a subset of the definition set.
func TestStylesheetVarsResolve(t *testing.T) {
	raw, err := os.ReadFile("template/css/template-fixed.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
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
			t.Errorf("stylesheet uses var(%s) which is never defined in :root", name)
		}
	}
	if len(used) == 0 {
		t.Error("stylesheet uses no var(); the extraction is broken")
	}
}
