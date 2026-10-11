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

// TestAccentSwapRecoloursOnlyTheAccentFamily renders the same quotation with
// two accents and asserts the accent family moved while the rest did not.
// TestAccentMovesDerivedRoles proves the palette partition; this proves the
// document follows it: every accent-family paint (Accent itself, Edge, Rule,
// the decoration end) changes, and every other role paints byte-identical
// counts of the same colour in both layouts.
func TestAccentSwapRecoloursOnlyTheAccentFamily(t *testing.T) {
	base := testEngine(t)
	in := sampleInput()
	// A wrapping recipient address makes the recipient card taller than the
	// metadata card, which paints the decoration gradient: without it Decor1
	// appears nowhere and the test could not see it move.
	in.RecipientAddress = "Jl.Boulevard Raya Blok A5 No. 8, Panakkukang, Makassar, Sulawesi Selatan, Indonesia 90231, Gedung Annex Lantai 4 Ruang 401"
	before := theme.DefaultTheme().Resolve()
	afterTheme := Theme{Deep: theme.DefaultDeep, Accent: "#C2410C", Soft: theme.DefaultSoft, Ink: theme.DefaultInk}
	after := afterTheme.Resolve()

	beforeCounts := paintCounts(base.layout(in))
	afterCounts := paintCounts(base.WithTheme(afterTheme).layout(in))

	moved := map[string]string{"Accent": "", "Edge": "", "Rule": "", "Decor1": ""}
	for role := range moved {
		var b, a string
		switch role {
		case "Accent":
			b, a = before.Accent, after.Accent
		case "Edge":
			b, a = before.Edge, after.Edge
		case "Rule":
			b, a = before.Rule, after.Rule
		default:
			b, a = before.Decor1, after.Decor1
		}
		if b == a {
			t.Errorf("precondition: %s did not move with the accent (%s)", role, b)
			continue
		}
		b, a = strings.ToUpper(b), strings.ToUpper(a)
		if beforeCounts[b] == 0 {
			t.Errorf("%s = %s paints nowhere in the default layout; the test cannot see it move", role, b)
		}
		if afterCounts[b] != 0 {
			t.Errorf("%s = %s still paints %d time(s) with accent %s; the family did not move",
				role, b, afterCounts[b], afterTheme.Accent)
		}
		if afterCounts[a] == 0 {
			t.Errorf("%s = %s paints nowhere with accent %s; the recolour did not land", role, a, afterTheme.Accent)
		}
		if beforeCounts[a] != 0 {
			t.Errorf("%s = %s already paints %d time(s) in the default layout; move/stay is ambiguous",
				role, a, beforeCounts[a])
		}
	}

	stayed := []string{
		before.Surface, before.Secondary, before.Emphasis,
		before.SubInk, before.SubNo, before.Muted, before.Rail,
		before.Decor0, before.Page0, before.Page1,
		before.Deep, before.Soft, before.Ink, before.Paper, before.Green,
	}
	for _, s := range stayed {
		s = strings.ToUpper(s)
		if beforeCounts[s] != afterCounts[s] {
			t.Errorf("non-accent colour %s paints %d time(s) by default but %d with accent %s; only the accent family may move",
				s, beforeCounts[s], afterCounts[s], afterTheme.Accent)
		}
	}
}

// paintCounts tallies every colour the layout paints: box fills and strokes,
// gradient ramp stops and text-run colours, upper-cased so Ink's lowercase
// default counts with the rest.
func paintCounts(pages []*page) map[string]int {
	out := map[string]int{}
	add := func(c string) {
		if c != "" {
			out[strings.ToUpper(c)]++
		}
	}
	for _, pg := range pages {
		for _, p := range pg.prims {
			switch v := p.(type) {
			case box:
				add(v.fill)
				add(v.stroke)
				if v.gradient != nil {
					add(v.gradient.c0)
					add(v.gradient.c1)
				}
			case run:
				add(v.color)
			}
		}
	}
	return out
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
// silently corrupting when every candidate collides, and that the error names
// the colliding palette roles with their values, so an admin reading the log
// can see which of their brand colours is at fault.
func TestSentinelExhaustionFailsLoudly(t *testing.T) {
	pal := theme.Palette{
		Deep: "FF00FF", Accent: "00FFFF", Soft: "7FFF00", Ink: "FF7F00",
		Surface: "FEFDFC", Edge: "02FF01", Rule: "FF0202", Secondary: "0202FF",
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
	_, _, err := chooseSentinels(pal)
	if err == nil {
		t.Fatal("chooser with every candidate colliding returned no error; want a loud failure")
	}
	if !strings.Contains(err.Error(), "gradient sentinel") {
		t.Errorf("expected the gradient exhaustion error, got %q", err)
	}
	for _, want := range []string{"Deep #FF00FF", "Accent #00FFFF", "Surface #FEFDFC"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("gradient exhaustion error %q does not name %q; it must say which colours collided", err, want)
		}
	}

	// The clip side carries the same contract: exhaust only the clip
	// candidates and the error must still name the roles.
	def := theme.DefaultTheme().Resolve()
	def.Deep, def.Accent, def.Soft, def.Ink = "#010203", "#02FF01", "#FF0202", "#0202FF"
	_, _, err = chooseSentinels(def)
	if err == nil {
		t.Fatal("chooser with every clip candidate colliding returned no error; want a loud failure")
	}
	if !strings.Contains(err.Error(), "clip sentinel") {
		t.Errorf("expected the clip exhaustion error, got %q", err)
	}
	for _, want := range []string{"Deep #010203", "Accent #02FF01", "Soft #FF0202", "Ink #0202FF"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("clip exhaustion error %q does not name %q; it must say which colours collided", err, want)
		}
	}
}
