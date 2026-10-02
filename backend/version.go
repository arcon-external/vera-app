package backend

import (
	"fmt"
	"strconv"
	"strings"
)

// AppVersion is the single source of truth for the desktop release version.
// Tagged builds override it with ldflags:
//
//	wails build -ldflags "-X vera-app/backend.AppVersion=v0.3.0"
//
// Untagged (dev) builds report "dev" and must never satisfy an update.
var AppVersion = "dev"

// Semver holds a parsed MAJOR.MINOR.PATCH triple. Prerelease suffixes are
// accepted syntactically but ignored by Compare unless the caller opts in.
type Semver struct {
	Pre   string
	Major int
	Minor int
	Patch int
}

// ParseSemver parses "vMAJOR.MINOR.PATCH[-pre]" (the leading v is optional).
// Anything else is an error: version strings that cannot be parsed must
// never compare as newer or older, only as unknown.
func ParseSemver(raw string) (Semver, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "v")
	trimmed = strings.TrimPrefix(trimmed, "V")
	var pre string
	if idx := strings.IndexByte(trimmed, '-'); idx >= 0 {
		pre = trimmed[idx+1:]
		trimmed = trimmed[:idx]
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", raw)
	}
	numbers := make([]int, 3)
	for i, part := range parts {
		if part == "" {
			return Semver{}, fmt.Errorf("version %q has an empty component", raw)
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return Semver{}, fmt.Errorf("version %q has a non-numeric component", raw)
			}
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return Semver{}, fmt.Errorf("version %q is not numeric: %w", raw, err)
		}
		numbers[i] = number
	}
	return Semver{Major: numbers[0], Minor: numbers[1], Patch: numbers[2], Pre: pre}, nil
}

// Compare returns -1, 0, or +1. A prerelease is older than its release
// (1.2.0-rc1 < 1.2.0); two prereleases compare lexically, which is only a
// tiebreak for display, never for update policy.
func CompareSemver(a, b Semver) int {
	for _, pair := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	default:
		return strings.Compare(a.Pre, b.Pre)
	}
}

// IsDevBuild reports whether this binary carries no release version.
// Dev builds never satisfy an update check.
func IsDevBuild() bool {
	_, err := ParseSemver(AppVersion)
	return AppVersion == "dev" || err != nil
}

// IsUpdateAvailable reports whether latest is newer than current. Unknown
// versions (unparseable either side) and dev builds never update; prerelease
// latest versions are ignored unless allowPrerelease is set.
func IsUpdateAvailable(current, latest string, allowPrerelease bool) bool {
	if current == "dev" {
		return false
	}
	currentVersion, err := ParseSemver(current)
	if err != nil {
		return false
	}
	latestVersion, err := ParseSemver(latest)
	if err != nil {
		return false
	}
	if latestVersion.Pre != "" && !allowPrerelease {
		return false
	}
	return CompareSemver(currentVersion, latestVersion) < 0
}
