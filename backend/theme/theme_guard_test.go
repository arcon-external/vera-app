package theme

import (
	"strings"
	"testing"
)

// TestDerivedValuesStayWithinClaimedBounds converts the role table on Palette
// from a doc comment into a durable contract: every derived default must sit
// within the per-channel distance claimed for the literals it replaced. The
// largest deviation is the emphasis tone's green channel (7 units): the two
// literals it merges sit on slightly different hues, so no single hue-locked
// value can match both exactly.
func TestDerivedValuesStayWithinClaimedBounds(t *testing.T) {
	got := DefaultTheme().Resolve()
	derived := map[string]string{
		"Surface": got.Surface, "Edge": got.Edge, "Rule": got.Rule,
		"Secondary": got.Secondary, "Emphasis": got.Emphasis,
		"SubInk": got.SubInk, "SubNo": got.SubNo, "Muted": got.Muted,
		"Rail": got.Rail, "Decor0": got.Decor0, "Decor1": got.Decor1,
		"Page0": got.Page0, "Page1": got.Page1,
	}
	cases := []struct {
		role     string
		literals []string
		bound    int
	}{
		{"Surface", []string{"#F8FAFF", "#F8FBFF", "#F7FAFF", "#F9FBFF", "#F6F9FF"}, 2},
		{"Edge", []string{"#C6D4FF", "#C9D9FF", "#CCD9FF", "#CAD8FF", "#C5D5FF", "#CEDBFF"}, 5},
		{"Rule", []string{"#D8E2FF", "#CFE0FF"}, 5},
		{"Secondary", []string{"#3A4466", "#3A4466", "#364066"}, 5},
		{"Emphasis", []string{"#2F3654", "#26304D", "#2F3654"}, 7},
		{"SubInk", []string{"#4A5470"}, 4},
		{"SubNo", []string{"#5B6478"}, 3},
		{"Muted", []string{"#8A93B2"}, 4},
		{"Rail", []string{"#B7C6E2"}, 2},
		{"Decor0", []string{"#DCE8FF"}, 0},
		{"Decor1", []string{"#C9DCFF"}, 2},
		{"Page0", []string{"#F4F7FF"}, 0},
		{"Page1", []string{"#EAF0FF"}, 1},
	}
	overall := 0
	for _, tc := range cases {
		d := maxChannelDelta(derived[tc.role], tc.literals)
		if d > tc.bound {
			t.Errorf("%s = %s is %d units from its farthest literal; bound is %d",
				tc.role, derived[tc.role], d, tc.bound)
		}
		if d > overall {
			overall = d
		}
	}
	if overall > 7 {
		t.Errorf("overall max channel delta = %d; claimed ceiling is 7", overall)
	}
}

// TestPortalDerivedValuesStayWithinClaimedBounds is the portal half of the
// fidelity contract: the three roles that replaced literals in the second
// pass must sit within the claimed distance of them.
func TestPortalDerivedValuesStayWithinClaimedBounds(t *testing.T) {
	got := DefaultTheme().ResolvePortal()
	for _, tc := range []struct {
		role     string
		derived  string
		literals []string
		bound    int
	}{
		{"FieldRule", got.FieldRule, []string{"#E6ECFB"}, 4},
		{"Label", got.Label, []string{"#7A85A3"}, 4},
		{"Strong", got.Strong, []string{"#26304D"}, 3},
	} {
		if d := maxChannelDelta(tc.derived, tc.literals); d > tc.bound {
			t.Errorf("portal %s = %s is %d units from %v; bound is %d",
				tc.role, tc.derived, d, tc.literals, tc.bound)
		}
	}
}

func maxChannelDelta(derived string, literals []string) int {
	dr, dg, db := channels(normalizeHex(derived))
	max := 0
	for _, lit := range literals {
		lr, lg, lb := channels(normalizeHex(lit))
		for _, d := range []int{absDiff(dr, lr), absDiff(dg, lg), absDiff(db, lb)} {
			if d > max {
				max = d
			}
		}
	}
	return max
}

func normalizeHex(s string) string { return strings.ToUpper(s) }

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestEdgeAndRuleContrastFloor proves a near-white Soft cannot fade the
// document: edges and rules must keep a visible delta from paper white, for a
// near-white and a near-black Soft alike. The default brand already passes,
// so it resolves byte-identically; white is the case that deepens.
func TestEdgeAndRuleContrastFloor(t *testing.T) {
	const floor = 1.25
	for _, soft := range []string{"#FFFFFF", "#000000", "#F7F2EC", DefaultSoft} {
		pal := Theme{Deep: DefaultDeep, Accent: DefaultAccent, Soft: soft, Ink: DefaultInk}.Resolve()
		for _, tc := range []struct{ role, value string }{
			{"Edge", pal.Edge}, {"Rule", pal.Rule},
		} {
			if c := contrastAgainstPaper(tc.value); c < floor {
				t.Errorf("soft %s %s = %s contrasts %.3f against paper; floor is %.2f",
					soft, tc.role, tc.value, c, floor)
			}
		}
	}
	portal := Theme{Deep: DefaultDeep, Accent: DefaultAccent, Soft: "#FFFFFF", Ink: DefaultInk}.ResolvePortal()
	if c := contrastAgainstPaper(portal.Edge); c < floor {
		t.Errorf("portal white-soft Edge = %s contrasts %.3f; floor is %.2f", portal.Edge, c, floor)
	}
	// The default must not have moved: the floor sits below its contrasts.
	def := DefaultTheme().Resolve()
	if def.Edge != "#C9D8FE" || def.Rule != "#D3E2FF" {
		t.Errorf("default Edge/Rule moved to %s/%s; the floor must preserve the default", def.Edge, def.Rule)
	}
}

// TestGreyBrandYieldsNeutralTextFamily proves a monochrome brand does not park
// the text family on red. A grey deep has no meaningful hue, so the tones
// must come out near-achromatic; the default saturated brand must resolve
// exactly as before.
func TestGreyBrandYieldsNeutralTextFamily(t *testing.T) {
	grey := Theme{Deep: "#808080", Accent: DefaultAccent, Soft: DefaultSoft, Ink: DefaultInk}.Resolve()
	for _, tc := range []struct {
		role  string
		value string
	}{
		{"Secondary", grey.Secondary}, {"Emphasis", grey.Emphasis},
		{"SubInk", grey.SubInk}, {"SubNo", grey.SubNo}, {"Muted", grey.Muted},
	} {
		r, g, b := channels(strings.ToUpper(tc.value))
		spread := maxInt3(absDiff(r, g), absDiff(g, b), absDiff(r, b))
		if spread > 6 {
			t.Errorf("grey brand %s = %s spreads %d across channels; want a neutral tone (spread <= 6)", tc.role, tc.value, spread)
		}
	}
	if grey.Secondary == "#653B3B" {
		t.Error("grey brand Secondary is still the old red-parked #653B3B")
	}
	// A barely-tinted grey must also stay neutral rather than magenta.
	offGrey := Theme{Deep: "#807F80"}.Resolve()
	if r, g, b := channels(strings.ToUpper(offGrey.Secondary)); maxInt3(absDiff(r, g), absDiff(g, b), absDiff(r, b)) > 8 {
		t.Errorf("near-neutral brand Secondary = %s; want a neutral family", offGrey.Secondary)
	}
}

func maxInt3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

// TestDefaultPortalPalettePinsNewRoles pins the three portal roles the second
// pass wired to the remaining literals, plus the fixed paper.
func TestDefaultPortalPalettePinsNewRoles(t *testing.T) {
	got := DefaultTheme().ResolvePortal()
	for _, tc := range []struct{ role, got, want string }{
		{"Paper", got.Paper, "#FFFFFF"},
		{"FieldRule", got.FieldRule, "#E6EFFF"},
		{"Label", got.Label, "#7E81A0"},
		{"Strong", got.Strong, "#292D4F"},
	} {
		if tc.got != tc.want {
			t.Errorf("portal %s = %s; want %s", tc.role, tc.got, tc.want)
		}
	}
}
