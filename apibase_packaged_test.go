//go:build !dev || production

package main

import (
	"os"
	"path/filepath"
	"testing"

	"vera-app/backend/apiclient"
)

// TestAPIBaseURLPackagedIgnoresDevelopmentOverrides is the guard for the
// packaged contract: an exported VERA_API_URL and a conflicting project .env
// must not move an installed (or debug-packaged) desktop off production.
func TestAPIBaseURLPackagedIgnoresDevelopmentOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("VERA_API_URL", "http://127.0.0.1:8081")
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("VERA_API_URL=http://localhost:9999\n"), 0o644); err != nil {
		t.Fatalf("write conflicting .env: %v", err)
	}

	base, err := apiBaseURL()
	if err != nil {
		t.Fatalf("apiBaseURL() error = %v", err)
	}
	if base != productionAPIURL {
		t.Fatalf("apiBaseURL() = %q; packaged builds must use %q", base, productionAPIURL)
	}
}

// TestProductionAPIURLIsPinned pins the exact production host. Comparing
// against the constant alone would accept any valid replacement, including a
// loopback or attacker HTTPS host.
func TestProductionAPIURLIsPinned(t *testing.T) {
	const want = "https://api.arconpentapersada.co.id"
	if productionAPIURL != want {
		t.Fatalf("productionAPIURL = %q; want %q", productionAPIURL, want)
	}
}

// TestProductionAPIURLIsValid keeps the compiled literal usable by the same
// validator every desktop request passes through.
func TestProductionAPIURLIsValid(t *testing.T) {
	if err := apiclient.ValidateBaseURL(productionAPIURL); err != nil {
		t.Fatalf("production API URL rejected by ValidateBaseURL: %v", err)
	}
}
