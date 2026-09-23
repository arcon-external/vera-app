package backend

import (
	"context"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

// TestRenderedThemeColorsReachCSS proves stored brand colors land in the
// executed stylesheet, and that empty values fall back to the baked palette
// instead of printing blanks.
func TestRenderedThemeColorsReachCSS(t *testing.T) {
	newPreparedQuotation := func(t *testing.T, themeQDeep string) *Quotation {
		t.Helper()
		q := newTestQuotation()
		q.SetAuthService(auth.StaffTestService())
		if err := Startup(q, context.Background()); err != nil {
			t.Fatalf("startup failed: %v", err)
		}
		if err := q.Prepare(testPreviewInput()); err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}
		q.Data.ThemeQDeep = themeQDeep
		if err := q.refreshCustomCSS(); err != nil {
			t.Fatalf("refresh CSS failed: %v", err)
		}
		return q
	}

	t.Run("custom color", func(t *testing.T) {
		q := newPreparedQuotation(t, "#0A1A5C")
		if !strings.Contains(string(q.Data.CustomCSS), "--deep-blue: #0A1A5C;") {
			t.Error("executed CSS does not contain the stored deep color")
		}
		if strings.Contains(string(q.Data.CustomCSS), "--deep-blue: #1A237E;") {
			t.Error("executed CSS still contains the baked deep color")
		}
	})

	t.Run("empty falls back", func(t *testing.T) {
		q := newPreparedQuotation(t, "")
		for _, want := range []string{
			"--deep-blue: #1A237E;",
			"--accent-blue: #284AF3;",
			"--soft-blue: #D6E4FF;",
			"--ink: #1a1a1a;",
		} {
			if !strings.Contains(string(q.Data.CustomCSS), want) {
				t.Errorf("executed CSS missing baked default %q", want)
			}
		}
	})

	t.Run("prepare resolves theme", func(t *testing.T) {
		q := newTestQuotation()
		q.SetAuthService(auth.StaffTestService())
		if err := Startup(q, context.Background()); err != nil {
			t.Fatalf("startup failed: %v", err)
		}
		q.companyIdentity.ThemeQAccent = "#3B82F6"
		if err := q.Prepare(testPreviewInput()); err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}
		if q.Data.ThemeQAccent != "#3B82F6" {
			t.Errorf("prepared ThemeQAccent = %q; want the cached profile value", q.Data.ThemeQAccent)
		}
		if !strings.Contains(string(q.Data.CustomCSS), "--accent-blue: #3B82F6;") {
			t.Error("Prepare did not re-execute the stylesheet with the resolved theme")
		}
	})
}
