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
	UpdateAvailable bool
	Required        bool
	NotesURL        string
	DownloadURL     string
}

// serverRelease mirrors the public /v1/app/latest contract.
type serverRelease struct {
	Latest       string            `json:"latest"`
	MinSupported string            `json:"minSupported"`
	NotesURL     string            `json:"notesUrl"`
	Downloads    map[string]string `json:"downloads"`
}

// updateHTTPClient times out fast: version checks must never hang startup.
var updateHTTPClient = &http.Client{Timeout: 15 * time.Second}

// FetchUpdateInfo compares the running build against the server's release
// state. It is a pure function of its inputs so tests drive it with an
// httptest server; the Wails binding supplies the real values. Unknown
// versions on either side mean "no update", never an error the UI must
// explain — except transport failures, which the UI reports as offline.
func FetchUpdateInfo(httpClient *http.Client, baseURL, current, goos, goarch string, allowPrerelease bool) (UpdateInfo, error) {
	info := UpdateInfo{Current: current}
	if httpClient == nil {
		httpClient = updateHTTPClient
	}
	endpoint := baseURL + "/v1/app/latest"
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return info, fmt.Errorf("check for updates: %w", err)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return info, fmt.Errorf("reach the update service: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return info, fmt.Errorf("update service returned %d", response.StatusCode)
	}
	var release serverRelease
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return info, fmt.Errorf("read release state: %w", err)
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

// downloadForPlatform picks the installer asset for this build. Unknown
// platforms get an empty URL: the banner then points at the release notes
// instead of a wrong binary.
func downloadForPlatform(downloads map[string]string, goos, goarch string) string {
	switch goos {
	case "windows":
		if goarch == "amd64" {
			return downloads["windows"]
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

// CheckForUpdates compares this build against the server's release state
// for the current platform. It never touches authentication state: the
// endpoint is public, and transport failures surface as errors the UI
// renders as offline — never as a sign-out.
func (u *UpdateAPI) CheckForUpdates() (UpdateInfo, error) {
	client, err := api()
	if err != nil {
		return UpdateInfo{Current: AppVersion}, err
	}
	return FetchUpdateInfo(nil, client.BaseURL(), AppVersion, runtime.GOOS, runtime.GOARCH, false)
}
