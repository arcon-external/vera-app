package api

import (
	_ "embed"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

//go:embed changelog.md
var changelogRaw string

// appRelease mirrors the public update contract. Every field comes from
// deploy-time environment so a release is a redeploy, not a code change.
// Empty means unconfigured: clients treat that as "no update info", and the
// updater never prompts on what it cannot name.
type appRelease struct {
	Latest       string            `json:"latest"`
	MinSupported string            `json:"minSupported"`
	NotesURL     string            `json:"notesUrl"`
	Downloads    map[string]string `json:"downloads"`
}

// latestRelease reads the release state from the environment. Values are
// trimmed; missing keys stay empty rather than inventing defaults.
func latestRelease() appRelease {
	downloads := map[string]string{}
	if windows := strings.TrimSpace(os.Getenv("VERA_APP_DOWNLOAD_WINDOWS")); windows != "" {
		downloads["windows"] = windows
	}
	if windowsARM64 := strings.TrimSpace(os.Getenv("VERA_APP_DOWNLOAD_WINDOWS_ARM64")); windowsARM64 != "" {
		downloads["windows-arm64"] = windowsARM64
	}
	if macos := strings.TrimSpace(os.Getenv("VERA_APP_DOWNLOAD_MACOS")); macos != "" {
		downloads["macos"] = macos
	}
	return appRelease{
		Latest:       strings.TrimSpace(os.Getenv("VERA_APP_LATEST")),
		MinSupported: strings.TrimSpace(os.Getenv("VERA_APP_MIN_SUPPORTED")),
		NotesURL:     strings.TrimSpace(os.Getenv("VERA_APP_NOTES_URL")),
		Downloads:    downloads,
	}
}

// getAppLatest serves the release state to every caller, signed in or not.
// Public by design (like /healthz): the desktop checks pre-login, and a
// version endpoint must never be able to sign anyone out.
func (s *Server) getAppLatest(context echo.Context) error {
	return context.JSON(http.StatusOK, latestRelease())
}

// getAppChangelog serves the staff-facing release notes embedded in the
// server binary. Public like getAppLatest: the What's New modal needs it
// right after sign-in, offline launches simply skip the modal until the
// server is reachable again.
func (s *Server) getAppChangelog(context echo.Context) error {
	return context.JSON(http.StatusOK, map[string]string{
		"changelog": strings.TrimSpace(changelogRaw),
	})
}
