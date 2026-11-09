//go:build dev && !production

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDevEnvFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

// TestAPIBaseURLDevUsesExportedURL covers a supported developer who exported
// the variable and has no project .env yet: that must work, and it must still
// be loopback-only.
func TestAPIBaseURLDevUsesExportedURL(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("VERA_API_URL", "http://127.0.0.1:8081/")

	base, err := apiBaseURL()
	if err != nil {
		t.Fatalf("apiBaseURL() error = %v", err)
	}
	if base != "http://127.0.0.1:8081" {
		t.Fatalf("apiBaseURL() = %q; want the normalized exported loopback URL", base)
	}
}

// TestAPIBaseURLDevReadsAllowlistedEnvFile proves the automatic wails dev path:
// no environment shuffling, only VERA_API_URL is read, and every server-owned
// key in the same file stays out of the desktop process.
func TestAPIBaseURLDevReadsAllowlistedEnvFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("VERA_API_URL", "")
	t.Setenv("VERA_POSTGRES_DSN", "")
	t.Setenv("VERA_SIGNING_KEY", "")
	writeDevEnvFile(t, dir,
		"# local development\n"+
			"VERA_POSTGRES_DSN=postgres://vera:secret@127.0.0.1:5432/veratest\n"+
			"VERA_SIGNING_KEY=secret-signing-key\n"+
			"export VERA_API_URL='http://localhost:8081'\n")

	base, err := apiBaseURL()
	if err != nil {
		t.Fatalf("apiBaseURL() error = %v", err)
	}
	if base != "http://localhost:8081" {
		t.Fatalf("apiBaseURL() = %q; want the .env loopback URL", base)
	}
	for _, name := range []string{"VERA_POSTGRES_DSN", "VERA_SIGNING_KEY"} {
		if value := os.Getenv(name); value != "" {
			t.Fatalf("%s leaked into the desktop environment as %q", name, value)
		}
	}
}

// TestAPIBaseURLDevExportedWinsOverEnvFile pins the documented precedence.
func TestAPIBaseURLDevExportedWinsOverEnvFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("VERA_API_URL", "http://127.0.0.1:9000")
	writeDevEnvFile(t, dir, "VERA_API_URL=http://localhost:8081\n")

	base, err := apiBaseURL()
	if err != nil {
		t.Fatalf("apiBaseURL() error = %v", err)
	}
	if base != "http://127.0.0.1:9000" {
		t.Fatalf("apiBaseURL() = %q; the exported value must win over .env", base)
	}
}

// TestAPIBaseURLDevMissingConfigurationFailsClearly is the no-production-
// fallback rule: absent from both places is a named configuration error, not
// a silent sign-in to the live API.
func TestAPIBaseURLDevMissingConfigurationFailsClearly(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("VERA_API_URL", "")

	base, err := apiBaseURL()
	if err == nil {
		t.Fatalf("apiBaseURL() = %q; want a configuration error", base)
	}
	if !strings.Contains(err.Error(), "VERA_API_URL") {
		t.Fatalf("error %q does not name VERA_API_URL", err)
	}
	if strings.Contains(err.Error(), "arconpentapersada") {
		t.Fatalf("error %q fell back to the production API", err)
	}
}

// TestAPIBaseURLDevRejectsNonLoopback covers a tailnet address like the one the
// old .env.example suggested: development must not send credentials there.
func TestAPIBaseURLDevRejectsNonLoopback(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, raw := range []string{
		"http://100.93.103.83:8081",
		"https://api.arconpentapersada.co.id",
		"http://192.168.1.10:8081",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("VERA_API_URL", raw)
			if _, err := apiBaseURL(); err == nil {
				t.Fatalf("apiBaseURL() accepted non-loopback %q", raw)
			} else if strings.Contains(err.Error(), raw) {
				t.Fatalf("error %q echoed the rejected URL", err)
			}
		})
	}
}

// TestAPIBaseURLDevNeverEchoesCredentials is the leak guard: a poisoned value
// with userinfo must fail without printing the password.
func TestAPIBaseURLDevNeverEchoesCredentials(t *testing.T) {
	t.Chdir(t.TempDir())
	const poisoned = "http://desk:sup3r-secret@127.0.0.1:8081"
	t.Setenv("VERA_API_URL", poisoned)

	_, err := apiBaseURL()
	if err == nil {
		t.Fatal("apiBaseURL() accepted a URL carrying credentials")
	}
	if strings.Contains(err.Error(), "sup3r-secret") || strings.Contains(err.Error(), poisoned) {
		t.Fatalf("error %q leaked the credential-bearing input", err)
	}
	if !strings.Contains(err.Error(), "VERA_API_URL") {
		t.Fatalf("error %q does not name VERA_API_URL", err)
	}
}

// TestAPIBaseURLDevRejectsUnusableURLs covers malformed and non-http(s)
// schemes with the same non-echoing error contract.
func TestAPIBaseURLDevRejectsUnusableURLs(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, raw := range []string{"127.0.0.1:8081", "ftp://127.0.0.1:8081", "http://"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("VERA_API_URL", raw)
			if _, err := apiBaseURL(); err == nil {
				t.Fatalf("apiBaseURL() accepted unusable URL %q", raw)
			}
		})
	}
}
