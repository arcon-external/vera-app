package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// UpdateInfo is the desktop's view of release state. Required means the
// running build is below the server's minimum supported version.
type UpdateInfo struct {
	Current         string
	Latest          string
	NotesURL        string
	DownloadURL     string
	UpdateAvailable bool
	Required        bool
}

// serverRelease mirrors the public /v1/app/latest contract.
type serverRelease struct {
	Downloads    map[string]string `json:"downloads"`
	Latest       string            `json:"latest"`
	MinSupported string            `json:"minSupported"`
	NotesURL     string            `json:"notesUrl"`
}

// updateHTTPClient times out fast: version checks must never hang startup.
var updateHTTPClient = &http.Client{Timeout: 15 * time.Second}

// staticManifestURL is the offline copy of /v1/app/latest, written to the
// public download bucket by scripts/announce-release.sh at release time.
//
// The API is the first source for release state, but it is a Cloud Run service
// that scales to zero and can be unreachable; when it is, the update check
// used to have no path at all. The bucket survives that, so the check falls
// back to this file. It carries the same shape the API serves -- the same
// serverRelease, read by the same decode -- so a fallback cannot describe a
// different release than the online endpoint.
const staticManifestURL = "https://storage.googleapis.com/vera-downloads-508411/latest.json"

// FetchUpdateInfo compares the running build against the server's release
// state. It is a pure function of its inputs so tests drive it with an
// httptest server; the Wails binding supplies the real values. Unknown
// versions on either side mean "no update", never an error the UI must
// explain — except transport failures, which the UI reports as offline.
//
// fallbackURL is read only when the API check fails: a network error, a
// non-200, or a body that will not parse. An empty fallbackURL disables
// the fallback — what callers that want transport-only behaviour pass.
// Both requests share httpClient, so the fallback inherits the same 15s
// timeout discipline as the primary check.
func FetchUpdateInfo(httpClient *http.Client, baseURL, fallbackURL, current, goos, goarch string, allowPrerelease bool) (UpdateInfo, error) {
	info := UpdateInfo{Current: current}
	if httpClient == nil {
		httpClient = updateHTTPClient
	}
	release, err := fetchServerRelease(httpClient, baseURL+"/v1/app/latest")
	if err != nil {
		if fallbackURL == "" {
			return info, err
		}
		staticRelease, staticErr := fetchServerRelease(httpClient, fallbackURL)
		if staticErr != nil {
			// The API is the source of record, so its failure is the one
			// worth reporting; the fallback is only news when it worked.
			return info, fmt.Errorf("%w (the static manifest at %s was unavailable too)", err, fallbackURL)
		}
		release = staticRelease
	}
	info.Latest = release.Latest
	info.NotesURL = release.NotesURL
	info.DownloadURL = downloadForPlatform(release.Downloads, goos, goarch)
	info.UpdateAvailable = IsUpdateAvailable(current, release.Latest, allowPrerelease)
	if minSupported := release.MinSupported; minSupported != "" {
		if currentVersion, err := ParseSemver(current); err == nil {
			if minVersion, err := ParseSemver(minSupported); err == nil {
				info.Required = CompareSemver(currentVersion, minVersion) < 0
			}
		}
	}
	return info, nil
}

// fetchServerRelease reads one endpoint of the release contract. The API path
// and the static manifest share it so the two cannot drift: same status
// check, same decode, same error wording.
func fetchServerRelease(client *http.Client, endpoint string) (serverRelease, error) {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return serverRelease{}, fmt.Errorf("check for updates: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return serverRelease{}, fmt.Errorf("reach the update service: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return serverRelease{}, fmt.Errorf("update service returned %d", response.StatusCode)
	}
	var release serverRelease
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return serverRelease{}, fmt.Errorf("read release state: %w", err)
	}
	return release, nil
}

// downloadForPlatform picks the installer asset for this build. Unknown
// platforms get an empty URL: the banner then points at the release notes
// instead of a wrong binary.
func downloadForPlatform(downloads map[string]string, goos, goarch string) string {
	switch goos {
	case "windows":
		switch goarch {
		case "amd64":
			return downloads["windows"]
		case "arm64":
			return downloads["windows-arm64"]
		}
	case "darwin":
		if goarch == "amd64" || goarch == "arm64" {
			return downloads["macos"]
		}
	}
	return ""
}

// UpdateAPI exposes the version check to the desktop frontend.
type UpdateAPI struct{}

// NewUpdateAPI builds the frontend update binding. It carries no state;
// the API base comes from the shared client at call time.
func NewUpdateAPI() *UpdateAPI {
	return &UpdateAPI{}
}

// Version reports the running build's version ("dev" for unreleased builds).
func (u *UpdateAPI) Version() (string, error) {
	return AppVersion, nil
}

// GetChangelog returns the staff-facing release notes served by the API
// server (embedded from CHANGELOG.md at build time). Transport failures
// surface as errors so the What's New modal can stay silent offline.
func (u *UpdateAPI) GetChangelog() (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	var response struct {
		Changelog string `json:"changelog"`
	}
	if err := client.Get("/v1/app/changelog", &response); err != nil {
		return "", err
	}
	return response.Changelog, nil
}

// CheckForUpdates compares this build against the server's release state
// for the current platform. It never touches authentication state: the
// endpoint is public, and transport failures surface as errors the UI
// renders as offline — never as a sign-out.
//
// The API is the first source and the static manifest on the public bucket is
// the second: an unreachable API hides no update, which is the whole reason
// the manifest exists.
func (u *UpdateAPI) CheckForUpdates() (UpdateInfo, error) {
	client, err := api()
	if err != nil {
		return UpdateInfo{Current: AppVersion}, err
	}
	return FetchUpdateInfo(nil, client.BaseURL(), staticManifestURL, AppVersion, runtime.GOOS, runtime.GOARCH, false)
}
