package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points the config directory at a temporary one, so a test can never
// read or write the developer's real session.
//
// Both variables are set because os.UserConfigDir reads XDG_CONFIG_HOME on Linux
// and $HOME/Library/Application Support on macOS, and this CLI ships to both.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("VERA_TOKEN", "")
	// Point at a closed port so a test that accidentally makes a request fails
	// immediately instead of talking to production.
	t.Setenv("VERA_API_URL", "http://127.0.0.1:1")
	return dir
}

// runCLI drives the whole CLI in-process and reports what a caller would see.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestUnknownCommandIsRefused(t *testing.T) {
	isolate(t)
	code, stdout, stderr := runCLI(t, "nonsense")
	if code != exitUsage {
		t.Errorf("exit = %d; want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q; want it to name the unknown command", stderr)
	}
	// Printing the whole help here would bury the error; the pointer at the
	// bottom of the message is enough.
	if strings.Contains(stdout, "Commands:") {
		t.Errorf("stdout printed the command list for an unknown command: %q", stdout)
	}
}

func TestUnknownCommandUnderAKnownNoun(t *testing.T) {
	isolate(t)
	code, _, stderr := runCLI(t, "quotations", "explode")
	if code != exitUsage {
		t.Errorf("exit = %d; want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, `"explode" is not a command under "vera quotations"`) {
		t.Errorf("stderr = %q; want it to name the missing verb", stderr)
	}
	if !strings.Contains(stderr, "available:") {
		t.Errorf("stderr = %q; want it to list what is available", stderr)
	}
}

func TestHelpIsAvailableForEveryCommand(t *testing.T) {
	isolate(t)
	// Every leaf has to answer `vera help <path>`, because the usage errors
	// point there: a usage error whose fix is a dead end is worse than none.
	root := newApp(&bytes.Buffer{}, &bytes.Buffer{}).tree()
	var walk func(c *command)
	walk = func(c *command) {
		if c.parent != nil {
			args := append([]string{"help"}, strings.Fields(strings.TrimPrefix(c.path(), "vera "))...)
			code, stdout, stderr := runCLI(t, args...)
			if code != exitOK {
				t.Errorf("vera %s: exit %d, stderr %q", strings.Join(args, " "), code, stderr)
			}
			// A leaf states its usage; a group lists its verbs. Both have to
			// say something, or the pointer from a usage error is a dead end.
			if c.run != nil && !strings.Contains(stdout, "Usage:") {
				t.Errorf("vera %s printed no usage line: %q", strings.Join(args, " "), stdout)
			}
			if len(c.sub) > 0 && !strings.Contains(stdout, "Commands:") {
				t.Errorf("vera %s printed no command list: %q", strings.Join(args, " "), stdout)
			}
		}
		for _, child := range c.sub {
			walk(child)
		}
	}
	walk(root)
}

func TestGlobalFlagBeforeTheVerb(t *testing.T) {
	isolate(t)
	code, stdout, _ := runCLI(t, "--json", "version")
	if code != exitOK {
		t.Fatalf("exit = %d; want 0", code)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("--json before the verb did not produce JSON: %v (%q)", err, stdout)
	}
	if payload.Version == "" {
		t.Error("--json produced no version")
	}
}

func TestGlobalFlagWithAValueBeforeTheVerb(t *testing.T) {
	isolate(t)
	// The URL must be read as the flag's value, not mistaken for a command
	// name -- and `version` must still be found behind it.
	code, stdout, stderr := runCLI(t, "--api", "http://127.0.0.1:1", "--timeout", "2s", "version")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr %q; want 0", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("version printed nothing")
	}
}

func TestGlobalFlagAfterTheVerb(t *testing.T) {
	isolate(t)
	code, stdout, _ := runCLI(t, "version", "--json")
	if code != exitOK {
		t.Fatalf("exit = %d; want 0", code)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Errorf("--json after the verb did not produce JSON: %q", stdout)
	}
}

func TestFlagsMayFollowPositionalArguments(t *testing.T) {
	isolate(t)
	// flag.FlagSet stops at the first non-flag token, so without the
	// interspersed parse this would silently ignore --out and proceed with an
	// empty value -- a wrong answer rather than an error.
	code, _, stderr := runCLI(t, "quotations", "rm", "not-a-number", "--yes")
	if code != exitUsage {
		t.Fatalf("exit = %d; want %d (stderr %q)", code, exitUsage, stderr)
	}
	if !strings.Contains(stderr, "quotation id must be a positive number") {
		t.Errorf("stderr = %q; want the id rejected", stderr)
	}
}

func TestDoubleDashSeparatesArgumentsFromFlags(t *testing.T) {
	isolate(t)
	// After --, a token that looks like a flag is an argument, so version is
	// handed "--json" and has to refuse it rather than printing JSON.
	code, _, stderr := runCLI(t, "version", "--", "--json")
	if code != exitUsage {
		t.Errorf("exit = %d; want %d (stderr %q)", code, exitUsage, stderr)
	}
	if !strings.Contains(stderr, "takes no arguments") {
		t.Errorf("stderr = %q; want the extra argument refused", stderr)
	}
}

func TestConfigShowNeverPrintsTheToken(t *testing.T) {
	dir := isolate(t)
	// Plant a session the way `vera login` would.
	seed := filepath.Join(dir, "vera")
	if err := os.MkdirAll(seed, 0o700); err != nil {
		t.Fatal(err)
	}
	token := "SECRET-TOKEN-DO-NOT-PRINT"
	session := `{"api":"https://example.test","token":"` + token + `","email":"a@b.test"}`
	if err := os.WriteFile(filepath.Join(seed, "session.json"), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLI(t, "config", "show", "--api", "https://example.test")
	if code != exitOK {
		t.Fatalf("exit = %d, stderr %q", code, stderr)
	}
	if strings.Contains(stdout, token) || strings.Contains(stderr, token) {
		t.Fatal("config show printed the session token; it is the command people paste into bug reports")
	}
	if !strings.Contains(stdout, "a@b.test") {
		t.Errorf("config show did not report the signed-in account: %q", stdout)
	}
}

func TestConfigSetAndClearRoundTrip(t *testing.T) {
	dir := isolate(t)
	if code, _, stderr := runCLI(t, "config", "set", "--default-api", "https://staging.test"); code != exitOK {
		t.Fatalf("config set: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := runCLI(t, "config", "set", "--json-output"); code != exitOK {
		t.Fatalf("config set --json-output: exit %d, stderr %q", code, stderr)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "vera", "config.json"))
	if err != nil {
		t.Fatalf("the config file was not written: %v", err)
	}
	if !strings.Contains(string(raw), "https://staging.test") {
		t.Errorf("config.json = %s; want the API address in it", raw)
	}

	if code, _, stderr := runCLI(t, "config", "clear"); code != exitOK {
		t.Fatalf("config clear: exit %d, stderr %q", code, stderr)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "vera", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "staging.test") {
		t.Errorf("config clear left the address behind: %s", raw)
	}
}

func TestConfigSetWithNothingToSetIsAUsageError(t *testing.T) {
	isolate(t)
	code, _, stderr := runCLI(t, "config", "set")
	if code != exitUsage {
		t.Errorf("exit = %d; want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "nothing to set") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestPlainHTTPToARemoteHostIsRefused(t *testing.T) {
	isolate(t)
	// A password or a bearer token crossing an open network in clear is the
	// one mistake worth refusing outright.
	code, _, stderr := runCLI(t, "clients", "list", "--api", "http://vera.example.com")
	if code != exitError {
		t.Errorf("exit = %d; want %d", code, exitError)
	}
	if !strings.Contains(stderr, "plain http") {
		t.Errorf("stderr = %q; want it to explain the refusal", stderr)
	}
}

func TestPlainHTTPToLoopbackIsAllowed(t *testing.T) {
	isolate(t)
	// Local development is exactly this, and it must not be blocked.
	code, _, stderr := runCLI(t, "updates", "check", "--api", "http://127.0.0.1:1", "--timeout", "2s")
	if code != exitServer {
		t.Errorf("exit = %d; want %d (unreachable, not refused)", code, exitServer)
	}
	if strings.Contains(stderr, "plain http") {
		t.Errorf("loopback was refused as insecure: %q", stderr)
	}
}

func TestSignedOutCommandsSayHowToSignIn(t *testing.T) {
	isolate(t)
	code, _, stderr := runCLI(t, "clients", "list")
	if code != exitAuth {
		t.Errorf("exit = %d; want %d", code, exitAuth)
	}
	if !strings.Contains(stderr, "vera login") {
		t.Errorf("stderr = %q; want it to name the fix", stderr)
	}
}

func TestQuietSuppressesHumanOutput(t *testing.T) {
	isolate(t)
	code, stdout, _ := runCLI(t, "version", "--quiet")
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	if stdout != "" {
		t.Errorf("--quiet printed %q; want nothing on success", stdout)
	}
}
