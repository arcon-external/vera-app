//go:build dev && !production

package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	// devAPIURLVariable is the only variable a development desktop reads.
	// veraserver's database, signing and bootstrap settings stay server-owned
	// and never enter this process.
	devAPIURLVariable = "VERA_API_URL"
	// devEnvFile is the project-root file `wails dev` inherits via its working
	// directory. It is gitignored; .env.example documents its keys.
	devEnvFile = ".env"
)

// apiBaseURL resolves the API endpoint for a development build. `wails dev`
// runs from the project root, so VERA_API_URL comes from the exported
// environment or the gitignored .env, in that order. There is deliberately no
// compiled fallback: a missing endpoint is a configuration error, never a
// silent sign-in to the production API.
func apiBaseURL() (string, error) {
	return resolveDevAPIBaseURL(os.Getenv, devEnvFile)
}

// resolveDevAPIBaseURL is the testable seam: it takes the environment lookup
// and env-file path instead of reading globals, so a test can pin precedence
// and allowlisting without mutating the process environment.
func resolveDevAPIBaseURL(getenv func(string) string, path string) (string, error) {
	raw := strings.TrimSpace(getenv(devAPIURLVariable))
	if raw == "" {
		fromFile, err := readAllowlistedAPIURL(path)
		if err != nil {
			return "", err
		}
		raw = fromFile
	}
	if raw == "" {
		return "", fmt.Errorf("configure development API endpoint: %s is not set in the environment or %s", devAPIURLVariable, path)
	}
	if err := validateDevAPIBaseURL(raw); err != nil {
		return "", err
	}
	return strings.TrimRight(raw, "/"), nil
}

// readAllowlistedAPIURL extracts VERA_API_URL from a dotenv file and ignores
// every other key. A missing file is not an error by itself -- the exported
// value may already supply the endpoint.
func readAllowlistedAPIURL(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read development API configuration %s: %w", path, err)
	}
	value := ""
	for _, line := range strings.Split(string(content), "\n") {
		key, parsed, ok := parseEnvLine(line)
		if ok && key == devAPIURLVariable {
			// Later assignments win, matching `source .env` semantics.
			value = parsed
		}
	}
	return value, nil
}

// parseEnvLine reads one KEY=value line, tolerating a leading `export`, blank
// lines, comments and matching single or double quotes. It is deliberately not
// a general dotenv implementation: only the named allowlist uses the result.
func parseEnvLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	separator := strings.IndexByte(line, '=')
	if separator <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:separator])
	value = strings.TrimSpace(line[separator+1:])
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}

// validateDevAPIBaseURL accepts only a loopback http(s) endpoint with no
// userinfo. Errors name the variable and never repeat the supplied value: a
// URL can carry credentials, and a configuration error must not print them.
func validateDevAPIBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("configure development API endpoint: %s must be a valid http(s) URL", devAPIURLVariable)
	}
	if parsed.User != nil {
		return fmt.Errorf("configure development API endpoint: %s must not contain credentials", devAPIURLVariable)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("configure development API endpoint: %s must use http or https", devAPIURLVariable)
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return nil
	}
	return fmt.Errorf("configure development API endpoint: %s must point at a loopback host (localhost, 127.0.0.1 or ::1)", devAPIURLVariable)
}
