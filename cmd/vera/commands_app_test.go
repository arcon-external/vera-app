package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"vera-app/backend"
)

// releaseContract is the wire shape of /v1/app/latest, written out as literal
// keys on purpose. The backend owns the struct that parses it; this test owns
// the bytes the server serves. A json tag renamed on either side alone makes
// the two disagree, and the CLI test below fails -- which is the whole point:
// before #443 the CLI kept its own private copy of these keys, and a rename
// broke the server's readers while leaving the CLI parsing a release that no
// longer existed.
func releaseContract() map[string]any {
	return map[string]any{
		"latest":       "v9.9.9",
		"minSupported": "v0.2.0",
		"notesUrl":     "https://example.test/notes",
		"downloads": map[string]string{
			"windows":       "https://example.test/w.exe",
			"windows-arm64": "https://example.test/w-arm64.exe",
			"macos":         "https://example.test/m.dmg",
		},
	}
}

// releaseServer serves the release contract and records nothing: the CLI is
// the only reader under test here.
func releaseServer(t *testing.T, payload map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/app/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

// updatesCheckJSON runs `vera updates check --json` and decodes the CLI's
// machine-readable output, which is the shape a script reads.
func updatesCheckJSON(t *testing.T, apiBase string) (int, struct {
	Current      string            `json:"current"`
	Latest       string            `json:"latest"`
	MinSupported string            `json:"minSupported"`
	NotesURL     string            `json:"notesUrl"`
	Downloads    map[string]string `json:"downloads"`
	UpToDate     bool              `json:"upToDate"`
}) {
	t.Helper()
	var got struct {
		Current      string            `json:"current"`
		Latest       string            `json:"latest"`
		MinSupported string            `json:"minSupported"`
		NotesURL     string            `json:"notesUrl"`
		Downloads    map[string]string `json:"downloads"`
		UpToDate     bool              `json:"upToDate"`
	}
	code, stdout, stderr := runCLI(t, "updates", "check", "--api", apiBase, "--json")
	if code != exitOK {
		t.Fatalf("updates check exited %d; want 0 (stderr %q)", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("updates check --json did not produce JSON: %v (%q)", err, stdout)
	}
	return code, got
}

// TestUpdatesCheckPrintsTheReleaseContract pins what `updates check` reports
// against the bytes the server served. A key the CLI parses with the wrong
// name -- its own private struct again, or a renamed tag on the shared type --
// prints as an empty field here rather than as a plausible value.
func TestUpdatesCheckPrintsTheReleaseContract(t *testing.T) {
	isolate(t)
	payload := releaseContract()
	server := releaseServer(t, payload)
	defer server.Close()

	_, got := updatesCheckJSON(t, server.URL)
	if got.Current != backend.AppVersion {
		t.Errorf("current = %q; want the running build %q", got.Current, backend.AppVersion)
	}
	if got.Latest != payload["latest"] {
		t.Errorf("latest = %q; want %q -- a key the CLI does not parse arrives empty", got.Latest, payload["latest"])
	}
	if got.MinSupported != payload["minSupported"] {
		t.Errorf("minSupported = %q; want %q", got.MinSupported, payload["minSupported"])
	}
	if got.NotesURL != payload["notesUrl"] {
		t.Errorf("notesUrl = %q; want %q", got.NotesURL, payload["notesUrl"])
	}
	if want := payload["downloads"].(map[string]string); !reflect.DeepEqual(got.Downloads, want) {
		t.Errorf("downloads = %v; want %v", got.Downloads, want)
	}
	if want := backend.AppVersion == payload["latest"]; got.UpToDate != want {
		t.Errorf("upToDate = %v; want %v", got.UpToDate, want)
	}

	// The human output is the same numbers a person reads, so it is pinned
	// here too: the JSON is emitted from the values the table prints, and a
	// divergence between the two is the bug this says out loud.
	code, stdout, stderr := runCLI(t, "updates", "check", "--api", server.URL)
	if code != exitOK {
		t.Fatalf("updates check exited %d; want 0 (stderr %q)", code, stderr)
	}
	for _, want := range []string{
		"This build:",
		backend.AppVersion,
		"Latest:",
		"v9.9.9",
		"Minimum supported:",
		"v0.2.0",
		"Notes:",
		"https://example.test/notes",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the human output is missing %q:\n%s", want, stdout)
		}
	}
	// "dev" never matches a release tag, and neither build is the other's
	// equal unless AppVersion happens to be stamped to the served latest.
	if backend.AppVersion == "v9.9.9" {
		if !strings.Contains(stdout, "Up to date.") {
			t.Errorf("the build equals the served latest but the output says otherwise:\n%s", stdout)
		}
	} else if !strings.Contains(stdout, "An update is available.") {
		t.Errorf("the output does not say an update is available:\n%s", stdout)
	}
}

// TestUpdatesCheckDecodesTheSharedReleaseType pins the CLI's parsed fields to
// the shared decode: the same bytes, read through backend's release contract,
// must be exactly what the CLI reported. A second private struct reintroduced
// in the CLI -- the #443 defect -- decodes differently from the shared one and
// shows up here as two views of one response that disagree.
func TestUpdatesCheckDecodesTheSharedReleaseType(t *testing.T) {
	isolate(t)
	server := releaseServer(t, releaseContract())
	defer server.Close()

	_, got := updatesCheckJSON(t, server.URL)
	shared, err := backend.FetchServerRelease(server.Client(), server.URL+"/v1/app/latest")
	if err != nil {
		t.Fatalf("the shared decode failed on the same bytes the CLI read: %v", err)
	}
	if got.Latest != shared.Latest {
		t.Errorf("the CLI reads latest = %q; the shared decode reads %q", got.Latest, shared.Latest)
	}
	if got.MinSupported != shared.MinSupported {
		t.Errorf("the CLI reads minSupported = %q; the shared decode reads %q", got.MinSupported, shared.MinSupported)
	}
	if got.NotesURL != shared.NotesURL {
		t.Errorf("the CLI reads notesUrl = %q; the shared decode reads %q", got.NotesURL, shared.NotesURL)
	}
	if !reflect.DeepEqual(got.Downloads, shared.Downloads) {
		t.Errorf("the CLI reads downloads = %v; the shared decode reads %v", got.Downloads, shared.Downloads)
	}
}
