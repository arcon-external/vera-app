package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppLatestServesEnvironment(t *testing.T) {
	t.Setenv("VERA_APP_LATEST", "v0.4.0")
	t.Setenv("VERA_APP_MIN_SUPPORTED", "v0.2.0")
	t.Setenv("VERA_APP_NOTES_URL", "https://example.test/notes")
	t.Setenv("VERA_APP_DOWNLOAD_WINDOWS", "https://example.test/w.exe")
	t.Setenv("VERA_APP_DOWNLOAD_WINDOWS_ARM64", "https://example.test/w-arm64.exe")
	t.Setenv("VERA_APP_DOWNLOAD_MACOS", "https://example.test/m.dmg")

	server, _, _ := adminTestSetup(t, "vera_api_app_latest")
	request := httptest.NewRequest(http.MethodGet, "/v1/app/latest", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("latest = %d; want 200", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["latest"] != "v0.4.0" || body["minSupported"] != "v0.2.0" {
		t.Errorf("body = %v; want configured release state", body)
	}
	downloads, _ := body["downloads"].(map[string]any)
	if downloads["windows"] != "https://example.test/w.exe" || downloads["macos"] != "https://example.test/m.dmg" || downloads["windows-arm64"] != "https://example.test/w-arm64.exe" {
		t.Errorf("downloads = %v; want all three assets", downloads)
	}
}

func TestAppLatestEmptyWhenUnconfigured(t *testing.T) {
	for _, key := range []string{
		"VERA_APP_LATEST", "VERA_APP_MIN_SUPPORTED", "VERA_APP_NOTES_URL",
		"VERA_APP_DOWNLOAD_WINDOWS", "VERA_APP_DOWNLOAD_WINDOWS_ARM64", "VERA_APP_DOWNLOAD_MACOS",
	} {
		t.Setenv(key, "")
	}
	server, _, _ := adminTestSetup(t, "vera_api_app_latest_empty")
	request := httptest.NewRequest(http.MethodGet, "/v1/app/latest", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("latest = %d; want 200 even unconfigured", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["latest"] != "" {
		t.Errorf("latest = %v; want empty when unconfigured", body["latest"])
	}
}

func TestAppLatestNeedsNoSession(t *testing.T) {
	server, _, _ := adminTestSetup(t, "vera_api_app_latest_anon")
	request := httptest.NewRequest(http.MethodGet, "/v1/app/latest", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusUnauthorized {
		t.Fatal("public endpoint demanded a session; update checks must work pre-login")
	}
}

func TestAppChangelogServesEmbeddedNotes(t *testing.T) {
	server, _, _ := adminTestSetup(t, "vera_api_app_changelog")
	request := httptest.NewRequest(http.MethodGet, "/v1/app/changelog", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("changelog = %d; want 200", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, want := range []string{"# Changelog", "## [0.2.2]", "## [0.1.0]"} {
		if !strings.Contains(body["changelog"], want) {
			t.Errorf("changelog missing %q", want)
		}
	}
}
