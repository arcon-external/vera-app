package gpdf

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"vera-app/backend/theme"
)

// TestSetThemeThenEmptyResolvesToDefaults proves the per-field fallback:
// setting a full theme and then an empty one lands on the defaults, rather
// than keeping the previous values (the pre-theme sticky behaviour).
func TestSetThemeThenEmptyResolvesToDefaults(t *testing.T) {
	e := testEngine(t)
	e.SetTheme(Theme{Deep: "#0A1A5C", Accent: "#C2410C", Soft: "#FF6B4A", Ink: "#111111"})
	if e.pal.Deep != "#0A1A5C" {
		t.Fatalf("full theme did not apply (Deep = %s)", e.pal.Deep)
	}
	e.SetTheme(Theme{})
	want := theme.DefaultTheme().Resolve()
	if e.pal != want {
		t.Errorf("empty theme did not resolve to defaults:\n got %+v\nwant %+v", e.pal, want)
	}
}

// TestWithThemeCopiesWithoutMutating proves the copy-on-set contract: the
// receiver keeps its palette, the copy carries the new one, and an empty
// copy resolves to the defaults.
func TestWithThemeCopiesWithoutMutating(t *testing.T) {
	base := testEngine(t)
	basePal := base.pal

	custom := base.WithTheme(Theme{Deep: "#0A1A5C"})
	if base.pal != basePal {
		t.Error("WithTheme mutated the receiver; it must copy")
	}
	if custom.pal.Deep != "#0A1A5C" {
		t.Errorf("copy Deep = %s; want #0A1A5C", custom.pal.Deep)
	}
	if custom == base {
		t.Error("WithTheme returned the receiver itself; want a copy")
	}
	empty := base.WithTheme(Theme{})
	if empty.pal != theme.DefaultTheme().Resolve() {
		t.Error("empty WithTheme did not resolve to the defaults")
	}
}

// TestNonDefaultThemeChangesNativePDF proves a custom theme reaches the
// engine's output: nothing currently asserts the palette affects the file.
func TestNonDefaultThemeChangesNativePDF(t *testing.T) {
	base := testEngine(t)
	in := sampleInput()

	def, err := base.Render(in)
	if err != nil {
		t.Fatalf("default render: %v", err)
	}
	if len(def) == 0 {
		t.Fatal("default render produced no bytes")
	}
	custom := base.WithTheme(Theme{Deep: "#0A1A5C", Accent: "#C2410C", Soft: "#FF6B4A", Ink: "#111111"})
	alt, err := custom.Render(in)
	if err != nil {
		t.Fatalf("custom render: %v", err)
	}
	if bytes.Equal(def, alt) {
		t.Error("a non-default theme produced byte-identical PDF; the palette is not reaching the output")
	}
}

// TestConcurrentRendersWithDifferentThemes exercises the B4 fix: two renders
// with different themes run concurrently on copies of one engine. Under
// -race a shared mutation (the old lazy SetTheme path) fails here; the
// copy-on-set path is clean.
func TestConcurrentRendersWithDifferentThemes(t *testing.T) {
	base := testEngine(t)
	in := sampleInput()
	themes := []Theme{
		{Deep: "#1A237E", Accent: "#284AF3", Soft: "#D6E4FF", Ink: "#1a1a1a"},
		{Deep: "#0A1A5C", Accent: "#C2410C", Soft: "#FF6B4A", Ink: "#111111"},
	}
	var wg sync.WaitGroup
	outs := make([][]byte, len(themes))
	errs := make([]error, len(themes))
	for i := range themes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := base.WithTheme(themes[i])
			outs[i], errs[i] = e.Render(in)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		if len(outs[i]) == 0 {
			t.Fatalf("render %d produced no bytes", i)
		}
	}
	if bytes.Equal(outs[0], outs[1]) {
		t.Error("two different themes produced identical PDFs under concurrency")
	}
}

// TestNoPaletteRoleEqualsSentinel proves the chooser's contract for the
// default palette and for a colliding soft: no resolved role may equal a
// chosen sentinel, or the post-pass would consume a real fill.
func TestNoPaletteRoleEqualsSentinel(t *testing.T) {
	for _, th := range []theme.Theme{
		theme.DefaultTheme(),
		{Deep: theme.DefaultDeep, Accent: theme.DefaultAccent, Soft: "#F7F2EC", Ink: theme.DefaultInk},
		{Deep: "#808080", Accent: theme.DefaultAccent, Soft: theme.DefaultSoft, Ink: theme.DefaultInk},
	} {
		pal := th.Resolve()
		grad, clip, err := chooseSentinels(pal)
		if err != nil {
			t.Fatalf("chooseSentinels(%+v): %v", th, err)
		}
		roles := []string{
			pal.Deep, pal.Accent, pal.Soft, pal.Ink, pal.Surface, pal.Edge,
			pal.Rule, pal.Secondary, pal.Emphasis, pal.SubInk, pal.SubNo,
			pal.Muted, pal.Rail, pal.Decor0, pal.Decor1, pal.Page0, pal.Page1,
			pal.Paper, pal.Green,
		}
		for _, role := range roles {
			if strings.EqualFold(strings.TrimPrefix(role, "#"), sentinelKey(grad)) {
				t.Errorf("theme %+v: gradient sentinel %v equals palette role %s", th, grad, role)
			}
			if strings.EqualFold(strings.TrimPrefix(role, "#"), sentinelKey(clip)) {
				t.Errorf("theme %+v: clip sentinel %v equals palette role %s", th, clip, role)
			}
		}
		if grad == clip {
			t.Errorf("theme %+v: gradient and clip sentinels coincide (%v)", th, grad)
		}
	}
}

// TestCollidingSoftChoosesAFreeSentinel is the B1 regression: Soft F7F2EC
// resolves Surface to the historic gradient sentinel, so a fixed sentinel
// corrupts the render. The chooser must skip it, and the render must succeed.
func TestCollidingSoftChoosesAFreeSentinel(t *testing.T) {
	pal := theme.Theme{Soft: "#F7F2EC"}.Resolve()
	if !strings.EqualFold(pal.Surface, "#FEFDFC") {
		t.Fatalf("precondition: Surface with Soft F7F2EC = %s; want #FEFDFC (the historic sentinel)", pal.Surface)
	}
	grad, _, err := chooseSentinels(pal)
	if err != nil {
		t.Fatalf("chooseSentinels with colliding soft: %v", err)
	}
	if sentinelKey(grad) == "FEFDFC" {
		t.Errorf("chooser returned the colliding sentinel FEFDFC despite Surface = %s", pal.Surface)
	}
	e := testEngine(t)
	renderEng := e.WithTheme(theme.Theme{Soft: "#F7F2EC"})
	out, err := renderEng.Render(sampleInput())
	if err != nil {
		t.Fatalf("render with colliding soft: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("render with colliding soft produced no bytes")
	}
}

// TestSentinelExhaustionFailsLoudly proves the chooser errors instead of
// silently corrupting when every candidate collides.
func TestSentinelExhaustionFailsLoudly(t *testing.T) {
	pal := theme.Palette{
		Deep: "FF00FF", Accent: "00FFFF", Soft: "7FFF00", Ink: "FF7F00",
		Surface: "010203", Edge: "02FF01", Rule: "FF0202", Secondary: "0202FF",
		Emphasis: "FF00FF", SubInk: "00FFFF", SubNo: "7FFF00", Muted: "FF7F00",
		Rail: "010203", Decor0: "02FF01", Decor1: "FF0202", Page0: "0202FF",
		Page1: "FF00FF", Paper: "00FFFF", Green: "7FFF00",
	}
	// Normalise to #-prefixed form the way Resolve produces.
	for _, s := range []*string{
		&pal.Deep, &pal.Accent, &pal.Soft, &pal.Ink, &pal.Surface, &pal.Edge,
		&pal.Rule, &pal.Secondary, &pal.Emphasis, &pal.SubInk, &pal.SubNo,
		&pal.Muted, &pal.Rail, &pal.Decor0, &pal.Decor1, &pal.Page0, &pal.Page1,
		&pal.Paper, &pal.Green,
	} {
		if *s != "" && (*s)[0] != '#' {
			*s = "#" + *s
		}
	}
	if _, _, err := chooseSentinels(pal); err == nil {
		t.Error("chooser with every candidate colliding returned no error; want a loud failure")
	}
}
