package theme

import "testing"

// TestDefaultPalettePinsDerivedValues pins the derivation's output for the
// default brand. The formulas are tuned so these land within a few units per
// channel of the literals they replaced (see the role table on Palette); if
// a formula changes, this test shows exactly which roles moved and by how
// much, instead of silently shifting the default document.
func TestDefaultPalettePinsDerivedValues(t *testing.T) {
	got := DefaultTheme().Resolve()
	want := map[string]string{
		"Deep":      "#1A237E",
		"Accent":    "#284AF3",
		"Soft":      "#D6E4FF",
		"Ink":       "#1a1a1a",
		"Surface":   "#F8FAFF",
		"Edge":      "#C9D8FE",
		"Rule":      "#D3E2FF",
		"Secondary": "#3B3F65",
		"Emphasis":  "#2C2F51",
		"SubInk":    "#4D506F",
		"SubNo":     "#5E6178",
		"Muted":     "#8E91B1",
		"Rail":      "#B9C5E1",
		"Decor0":    "#DCE8FF",
		"Decor1":    "#CBDAFE",
		"Page0":     "#F4F7FF",
		"Page1":     "#E9F1FF",
		"Paper":     "#FFFFFF",
		"Green":     "#228B22",
	}
	have := map[string]string{
		"Deep": got.Deep, "Accent": got.Accent, "Soft": got.Soft, "Ink": got.Ink,
		"Surface": got.Surface, "Edge": got.Edge, "Rule": got.Rule,
		"Secondary": got.Secondary, "Emphasis": got.Emphasis,
		"SubInk": got.SubInk, "SubNo": got.SubNo, "Muted": got.Muted,
		"Rail": got.Rail, "Decor0": got.Decor0, "Decor1": got.Decor1,
		"Page0": got.Page0, "Page1": got.Page1,
		"Paper": got.Paper, "Green": got.Green,
	}
	for role, w := range want {
		if have[role] != w {
			t.Errorf("%s = %s; want %s", role, have[role], w)
		}
	}
}

// TestResolveFallsBackPerField proves an empty or malformed column renders
// as its default instead of breaking the document. The fallback is per
// field, matching the old stylesheet's per-variable guards.
func TestResolveFallsBackPerField(t *testing.T) {
	got := Theme{Deep: "#0A1A5C", Accent: "not-a-colour", Soft: "", Ink: "#GGGGGG"}.Resolve()
	if got.Deep != "#0A1A5C" {
		t.Errorf("Deep = %s; valid input must pass through", got.Deep)
	}
	if got.Accent != DefaultAccent {
		t.Errorf("Accent = %s; malformed input must fall back to %s", got.Accent, DefaultAccent)
	}
	if got.Soft != DefaultSoft {
		t.Errorf("Soft = %s; empty input must fall back to %s", got.Soft, DefaultSoft)
	}
	if got.Ink != DefaultInk {
		t.Errorf("Ink = %s; malformed input must fall back to %s", got.Ink, DefaultInk)
	}
}

// TestAccentMovesDerivedRoles is the recolour contract: changing the accent
// must move the roles shaded toward it (borders, hairlines, decoration end)
// while leaving the soft-derived surfaces and the deep-derived text tones
// alone. One knob moves its own family, never the whole document by itself.
func TestAccentMovesDerivedRoles(t *testing.T) {
	before := DefaultTheme().Resolve()
	after := Theme{Deep: DefaultDeep, Accent: "#C2410C", Soft: DefaultSoft, Ink: DefaultInk}.Resolve()
	for _, role := range []string{"Edge", "Rule", "Decor1"} {
		var b, a string
		switch role {
		case "Edge":
			b, a = before.Edge, after.Edge
		case "Rule":
			b, a = before.Rule, after.Rule
		default:
			b, a = before.Decor1, after.Decor1
		}
		if b == a {
			t.Errorf("%s did not move with the accent (%s)", role, b)
		}
	}
	for _, role := range []string{"Surface", "Secondary", "Emphasis", "Page0"} {
		var b, a string
		switch role {
		case "Surface":
			b, a = before.Surface, after.Surface
		case "Secondary":
			b, a = before.Secondary, after.Secondary
		case "Emphasis":
			b, a = before.Emphasis, after.Emphasis
		default:
			b, a = before.Page0, after.Page0
		}
		if b != a {
			t.Errorf("%s moved with the accent (%s -> %s); it belongs to another knob", role, b, a)
		}
	}
}

// TestDefaultPortalPalette pins the portal's derived brand set, including
// the two roles the dead ThemePSoft setting now drives (Section, Page0/1)
// and the resolved accent/deep fragments behind the focus ring and shadow.
func TestDefaultPortalPalette(t *testing.T) {
	got := DefaultTheme().ResolvePortal()
	want := map[string]string{
		"Edge": "#C9D8FE", "Secondary": "#3B3F65",
		"Page0": "#F4F7FF", "Page1": "#E9F1FF", "Section": "#EDF3FF",
		"Focus": "rgba(40, 74, 243, 0.28)", "Shadow": "rgba(26, 35, 126, 0.08)",
	}
	have := map[string]string{
		"Edge": got.Edge, "Secondary": got.Secondary,
		"Page0": got.Page0, "Page1": got.Page1, "Section": got.Section,
		"Focus": string(got.Focus), "Shadow": string(got.Shadow),
	}
	for role, w := range want {
		if have[role] != w {
			t.Errorf("portal %s = %s; want %s", role, have[role], w)
		}
	}
}
