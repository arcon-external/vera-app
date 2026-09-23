package backend

import "testing"

func TestParseSemver(t *testing.T) {
	cases := []struct {
		raw   string
		want  Semver
		valid bool
	}{
		{"v0.3.0", Semver{Major: 0, Minor: 3, Patch: 0}, true},
		{"1.2.3", Semver{Major: 1, Minor: 2, Patch: 3}, true},
		{"V2.0.1", Semver{Major: 2, Minor: 0, Patch: 1}, true},
		{"v1.2.0-rc1", Semver{Major: 1, Minor: 2, Patch: 0, Pre: "rc1"}, true},
		{"  v0.3.0  ", Semver{Major: 0, Minor: 3, Patch: 0}, true},
		{"dev", Semver{}, false},
		{"", Semver{}, false},
		{"v1.2", Semver{}, false},
		{"v1.2.3.4", Semver{}, false},
		{"v1.x.3", Semver{}, false},
		{"v1..3", Semver{}, false},
		{"latest", Semver{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := ParseSemver(tc.raw)
			if !tc.valid {
				if err == nil {
					t.Errorf("ParseSemver(%q) succeeded; want an error", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSemver(%q) failed: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("ParseSemver(%q) = %+v; want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.3.0", "v0.3.0", 0},
		{"v0.3.0", "v0.4.0", -1},
		{"v0.4.0", "v0.3.0", 1},
		{"v0.3.9", "v0.4.0", -1},
		{"v1.0.0", "v0.99.99", 1},
		{"v1.2.0-rc1", "v1.2.0", -1},
		{"v1.2.0", "v1.2.0-rc1", 1},
		{"v0.10.0", "v0.9.0", 1},
	}
	for _, tc := range cases {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			a, err := ParseSemver(tc.a)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.a, err)
			}
			b, err := ParseSemver(tc.b)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.b, err)
			}
			if got := CompareSemver(a, b); got != tc.want {
				t.Errorf("CompareSemver(%q, %q) = %d; want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestIsUpdateAvailable(t *testing.T) {
	cases := []struct {
		name            string
		current, latest string
		allowPre        bool
		want            bool
	}{
		{"newer available", "v0.3.0", "v0.4.0", false, true},
		{"same version", "v0.3.0", "v0.3.0", false, false},
		{"older latest", "v0.4.0", "v0.3.0", false, false},
		{"dev never updates", "dev", "v99.0.0", false, false},
		{"unparseable current", "nonsense", "v0.4.0", false, false},
		{"unparseable latest", "v0.3.0", "nonsense", false, false},
		{"prerelease ignored by default", "v0.3.0", "v0.4.0-rc1", false, false},
		{"prerelease opted in", "v0.3.0", "v0.4.0-rc1", true, true},
		{"prerelease to release", "v0.4.0-rc1", "v0.4.0", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUpdateAvailable(tc.current, tc.latest, tc.allowPre); got != tc.want {
				t.Errorf("IsUpdateAvailable(%q, %q, %v) = %v; want %v",
					tc.current, tc.latest, tc.allowPre, got, tc.want)
			}
		})
	}
}
