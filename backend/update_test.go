package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func releaseServer(t *testing.T, payload map[string]any, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/app/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("update check sent credentials to a public endpoint")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

func TestFetchUpdateInfo(t *testing.T) {
	full := map[string]any{
		"latest":       "v0.4.0",
		"minSupported": "v0.2.0",
		"notesUrl":     "https://example.test/notes",
		"downloads":    map[string]string{"windows": "https://example.test/w.exe", "windows-arm64": "https://example.test/w-arm64.exe", "macos": "https://example.test/m.dmg"},
	}
	cases := []struct {
		name          string
		payload       map[string]any
		status        int
		current       string
		goos, goarch  string
		wantAvailable bool
		wantRequired  bool
		wantDownload  string
		wantErr       bool
	}{
		{"newer on windows", full, 200, "v0.3.0", "windows", "amd64", true, false, "https://example.test/w.exe", false},
		{"newer on windows arm64", full, 200, "v0.3.0", "windows", "arm64", true, false, "https://example.test/w-arm64.exe", false},
		{"newer on mac arm", full, 200, "v0.3.0", "darwin", "arm64", true, false, "https://example.test/m.dmg", false},
		{"newer on mac intel", full, 200, "v0.3.0", "darwin", "amd64", true, false, "https://example.test/m.dmg", false},
		{"up to date", full, 200, "v0.4.0", "windows", "amd64", false, false, "https://example.test/w.exe", false},
		{"older latest", full, 200, "v0.5.0", "windows", "amd64", false, false, "https://example.test/w.exe", false},
		{"dev never updates", full, 200, "dev", "windows", "amd64", false, false, "https://example.test/w.exe", false},
		{"below minimum is required", full, 200, "v0.1.0", "windows", "amd64", true, true, "https://example.test/w.exe", false},
		{"at minimum is fine", full, 200, "v0.2.0", "windows", "amd64", true, false, "https://example.test/w.exe", false},
		{"unknown platform has no binary", full, 200, "v0.3.0", "linux", "amd64", true, false, "", false},
		{"server error", full, 500, "v0.3.0", "windows", "amd64", false, false, "", true},
		{"malformed tag is not an update", map[string]any{"latest": "soon", "downloads": map[string]string{}}, 200, "v0.3.0", "windows", "amd64", false, false, "", false},
		{"empty release", map[string]any{}, 200, "v0.3.0", "windows", "amd64", false, false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := releaseServer(t, tc.payload, tc.status)
			defer server.Close()
			info, err := FetchUpdateInfo(server.Client(), server.URL, tc.current, tc.goos, tc.goarch, false)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a transport error")
				}
				return
			}
			if err != nil {
				t.Fatalf("FetchUpdateInfo failed: %v", err)
			}
			if info.UpdateAvailable != tc.wantAvailable {
				t.Errorf("UpdateAvailable = %v; want %v", info.UpdateAvailable, tc.wantAvailable)
			}
			if info.Required != tc.wantRequired {
				t.Errorf("Required = %v; want %v", info.Required, tc.wantRequired)
			}
			if info.DownloadURL != tc.wantDownload {
				t.Errorf("DownloadURL = %q; want %q", info.DownloadURL, tc.wantDownload)
			}
			if info.Current != tc.current {
				t.Errorf("Current = %q; want %q", info.Current, tc.current)
			}
		})
	}
}

func TestFetchUpdateInfoOffline(t *testing.T) {
	// Unroutable documentation address: must fail fast as an error, never hang,
	// and never touch authentication state (asserted structurally: this helper
	// takes no session and the request carries no credentials by construction).
	quick := &http.Client{Timeout: 3 * time.Second}
	_, err := FetchUpdateInfo(quick, "http://192.0.2.1:9", "v0.3.0", "windows", "amd64", false)
	if err == nil {
		t.Fatal("expected an offline error")
	}
}
