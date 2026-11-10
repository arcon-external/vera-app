package main

// scripts/dependabot-status.sh is the release checklist's read of Dependabot:
// the open alerts and the open Dependabot pull requests, before a tag. The
// script is bash, so it is proved the way announce-release.sh is proved --
// run for real against a fake `gh` in a temp PATH that serves fixture JSON.
//
// The fake is a `gh` that answers exactly the two queries the script makes and
// logs every call. That is enough to pin the contract the release doc and a
// human both rely on: which API path is read, that `--json` emits one parseable
// document, that the empty case says `Dependabot: 0 open` rather than nothing,
// and that a failed read names itself instead of printing an empty list.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Two pages, because `gh api --paginate` prints each page as its own JSON
// array and the script has to merge them. One page would never exercise that.
const dependabotAlertsPageOne = `[
  {
    "number": 3,
    "state": "open",
    "dependency": {
      "package": {"ecosystem": "go", "name": "golang.org/x/crypto"},
      "manifest_path": "go.mod"
    },
    "security_advisory": {"ghsa_id": "GHSA-xxxx", "severity": "high", "summary": "test fixture"},
    "security_vulnerability": {
      "severity": "high",
      "first_patched_version": {"identifier": "v0.31.0"}
    },
    "html_url": "https://github.com/acme/vera-app/security/dependabot/3"
  }
]`

const dependabotAlertsPageTwo = `[
  {
    "number": 7,
    "state": "open",
    "dependency": {
      "package": {"ecosystem": "npm", "name": "vite"},
      "manifest_path": "frontend/package.json"
    },
    "security_advisory": {"ghsa_id": "GHSA-yyyy", "severity": "medium", "summary": "test fixture"},
    "security_vulnerability": {
      "severity": "medium",
      "first_patched_version": null
    },
    "html_url": "https://github.com/acme/vera-app/security/dependabot/7"
  }
]`

const dependabotPullRequestsFixture = `[
  {
    "number": 42,
    "title": "Bump vite from 5.0.0 to 5.4.6",
    "url": "https://github.com/acme/vera-app/pull/42",
    "headRefName": "dependabot/npm_and_yarn/frontend/vite-5.4.6",
    "createdAt": "2026-10-01T08:00:00Z"
  }
]`

// stubDependabotGH is the fake `gh`. It logs the call, then answers the alerts
// endpoint with the fixture file and `pr list` with the PR fixture. Every other
// call is a failure, so a script that starts querying something else cannot
// pass by accident.
const stubDependabotGH = `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${STUB_LOG:?}"
if [ "${1:-}" = "api" ]; then
  if [ "${STUB_ALERTS_EXIT:-0}" != "0" ]; then
    printf '%s\n' "${STUB_ALERTS_ERROR:-stub gh: alerts query failed}" >&2
    exit "${STUB_ALERTS_EXIT}"
  fi
  cat "${STUB_ALERTS_FILE:?}"
  exit 0
fi
if [ "${1:-}" = "pr" ] && [ "${2:-}" = "list" ]; then
  cat "${STUB_PRS_FILE:?}"
  exit 0
fi
printf 'stub gh: unsupported call: %s\n' "$*" >&2
exit 1
`

// dependabotStatus mirrors the --json document. It is declared here, not shared
// with the script, so a rename on either side fails the test rather than both
// sides agreeing on a new name.
type dependabotAlert struct {
	Number       int    `json:"number"`
	Severity     string `json:"severity"`
	Package      string `json:"package"`
	FirstPatched string `json:"firstPatched"`
	URL          string `json:"url"`
}

type dependabotPullRequest struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
	CreatedAt   string `json:"createdAt"`
}

type dependabotAlertsList struct {
	Count int               `json:"count"`
	Items []dependabotAlert `json:"items"`
}

type dependabotPullRequestsList struct {
	Count int                     `json:"count"`
	Items []dependabotPullRequest `json:"items"`
}

type dependabotStatus struct {
	Repository   string                     `json:"repository"`
	Alerts       dependabotAlertsList       `json:"alerts"`
	PullRequests dependabotPullRequestsList `json:"pullRequests"`
}

type dependabotStub struct {
	alerts       string
	pullRequests string
	// alertsExit makes the alerts query fail with this status and message.
	alertsExit  int
	alertsError string
	// hideGH leaves `gh` off the PATH, so the script's dependency guard runs.
	hideGH bool
}

// runDependabotStatus runs the real script against the fake gh and returns its
// stdout, stderr, exit status and every call the fake saw.
func runDependabotStatus(t *testing.T, stub dependabotStub, args ...string) (string, string, int, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the status script is bash; its run is proved on the CI runner")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required by the script; install it to run this test")
	}

	stubDir := t.TempDir()
	alertsPath := filepath.Join(stubDir, "alerts.json")
	prsPath := filepath.Join(stubDir, "prs.json")
	if err := os.WriteFile(alertsPath, []byte(stub.alerts), 0o644); err != nil {
		t.Fatalf("write the alerts fixture: %v", err)
	}
	if err := os.WriteFile(prsPath, []byte(stub.pullRequests), 0o644); err != nil {
		t.Fatalf("write the pull request fixture: %v", err)
	}
	if !stub.hideGH {
		if err := os.WriteFile(filepath.Join(stubDir, "gh"), []byte(stubDependabotGH), 0o755); err != nil {
			t.Fatalf("write the gh stub: %v", err)
		}
	}
	// hideGH gets the stub dir alone: the real gh must not be reachable, or the
	// dependency guard would find it and the test would prove nothing.
	pathValue := stubDir
	if !stub.hideGH {
		pathValue += string(os.PathListSeparator) + os.Getenv("PATH")
	}

	logPath := filepath.Join(stubDir, "calls.log")
	script := filepath.Join("scripts", "dependabot-status.sh")
	command := exec.Command(bash, append([]string{script}, args...)...)
	command.Env = append(os.Environ(),
		"PATH="+pathValue,
		"GITHUB_REPOSITORY=acme/vera-app",
		"STUB_LOG="+logPath,
		"STUB_ALERTS_FILE="+alertsPath,
		"STUB_PRS_FILE="+prsPath,
		fmt.Sprintf("STUB_ALERTS_EXIT=%d", stub.alertsExit),
		"STUB_ALERTS_ERROR="+stub.alertsError,
	)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run the status script: %v", err)
		}
		code = exitErr.ExitCode()
	}

	calls := []string{}
	if data, readErr := os.ReadFile(logPath); readErr == nil {
		calls = strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	return stdout.String(), stderr.String(), code, calls
}

func TestDependabotStatusReportsAlertsAndPullRequests(t *testing.T) {
	stub := dependabotStub{
		alerts:       dependabotAlertsPageOne + "\n" + dependabotAlertsPageTwo,
		pullRequests: dependabotPullRequestsFixture,
	}

	stdout, stderr, code, calls := runDependabotStatus(t, stub, "--json")
	if code != 0 {
		t.Fatalf("--json exited %d; stderr:\n%s", code, stderr)
	}
	var got dependabotStatus
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("--json did not emit one JSON document: %v\nstdout:\n%s", err, stdout)
	}

	want := dependabotStatus{
		Repository: "acme/vera-app",
		Alerts: dependabotAlertsList{
			Count: 2,
			Items: []dependabotAlert{
				{
					Number: 3, Severity: "high", Package: "golang.org/x/crypto",
					FirstPatched: "v0.31.0",
					URL:          "https://github.com/acme/vera-app/security/dependabot/3",
				},
				{
					Number: 7, Severity: "medium", Package: "vite",
					URL: "https://github.com/acme/vera-app/security/dependabot/7",
				},
			},
		},
		PullRequests: dependabotPullRequestsList{
			Count: 1,
			Items: []dependabotPullRequest{
				{
					Number: 42, Title: "Bump vite from 5.0.0 to 5.4.6",
					URL:         "https://github.com/acme/vera-app/pull/42",
					HeadRefName: "dependabot/npm_and_yarn/frontend/vite-5.4.6",
					CreatedAt:   "2026-10-01T08:00:00Z",
				},
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("--json reported\n%+v\nwant\n%+v", got, want)
	}

	// The read is the contract: this API path, and a Dependabot-authored,
	// open PR list asked for as JSON. Not the whole argument string, so a
	// harmless flag addition does not break the test.
	alertCall := findCall(calls, "repos/acme/vera-app/dependabot/alerts?state=open")
	if alertCall == "" {
		t.Errorf("no alert read at repos/acme/vera-app/dependabot/alerts?state=open; calls were:\n%s", strings.Join(calls, "\n"))
	}
	prCall := findCall(calls, "--author app/dependabot")
	if prCall == "" {
		t.Errorf("no PR read authored by app/dependabot; calls were:\n%s", strings.Join(calls, "\n"))
	}
	for _, want := range []string{"--state open", "--json"} {
		if !strings.Contains(prCall, want) {
			t.Errorf("PR read %q is missing %q", prCall, want)
		}
	}

	// The human read is what goes into the release issue: a count line, then
	// one row per item, with the unpatched alert saying so rather than showing
	// an empty column.
	human, stderr, code, _ := runDependabotStatus(t, stub)
	if code != 0 {
		t.Fatalf("the human read exited %d; stderr:\n%s", code, stderr)
	}
	for _, want := range []string{
		"Dependabot: 2 open",
		"high",
		"golang.org/x/crypto",
		"v0.31.0",
		"medium",
		"vite",
		"no patch published",
		"Dependabot pull requests: 1 open",
		"#42",
		"Bump vite from 5.0.0 to 5.4.6",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("the human read is missing %q; output:\n%s", want, human)
		}
	}
	if strings.Contains(human, "{") {
		t.Errorf("the human read looks like JSON; output:\n%s", human)
	}
}

func TestDependabotStatusEmptySaysZero(t *testing.T) {
	stub := dependabotStub{alerts: "[]", pullRequests: "[]"}

	stdout, stderr, code, _ := runDependabotStatus(t, stub, "--json")
	if code != 0 {
		t.Fatalf("--json exited %d; stderr:\n%s", code, stderr)
	}
	var got dependabotStatus
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("--json did not emit one JSON document: %v\nstdout:\n%s", err, stdout)
	}
	if got.Alerts.Count != 0 || got.PullRequests.Count != 0 {
		t.Errorf("empty read reported %d alerts and %d PRs; want 0 and 0", got.Alerts.Count, got.PullRequests.Count)
	}
	// `null` would make a consumer's `.items | length` differ from jq's and is
	// a different contract from an empty list.
	if got.Alerts.Items == nil {
		t.Error("alerts.items is null; an empty read must report []")
	}
	if got.PullRequests.Items == nil {
		t.Error("pullRequests.items is null; an empty read must report []")
	}

	human, stderr, code, _ := runDependabotStatus(t, stub)
	if code != 0 {
		t.Fatalf("the human read exited %d; stderr:\n%s", code, stderr)
	}
	// The literal AGENTS.md §11 asks a PR body to carry when nothing is open.
	for _, want := range []string{"Dependabot: 0 open", "Dependabot pull requests: 0 open"} {
		if !strings.Contains(human, want) {
			t.Errorf("the empty human read is missing %q; output:\n%s", want, human)
		}
	}
}

func TestDependabotStatusFailsClearlyWhenTheAlertQueryFails(t *testing.T) {
	stub := dependabotStub{
		alerts:       "[]",
		pullRequests: "[]",
		alertsExit:   1,
		alertsError:  "HTTP 403: Resource not accessible by integration",
	}
	stdout, stderr, code, _ := runDependabotStatus(t, stub, "--json")
	if code == 0 {
		t.Fatalf("a failed alerts query exited 0; stdout:\n%s", stdout)
	}
	if stdout != "" {
		t.Errorf("a failed read printed to stdout: %q", stdout)
	}
	for _, want := range []string{"dependabot-status:", "alerts", "acme/vera-app", "HTTP 403"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr is missing %q; stderr:\n%s", want, stderr)
		}
	}
}

func TestDependabotStatusRefusesToRunWithoutGH(t *testing.T) {
	stdout, stderr, code, _ := runDependabotStatus(t, dependabotStub{alerts: "[]", pullRequests: "[]", hideGH: true})
	if code != 2 {
		t.Fatalf("missing gh exited %d; want 2. stderr:\n%s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("missing gh printed to stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "gh is required") {
		t.Errorf("stderr does not name the missing dependency; stderr:\n%s", stderr)
	}
}

func findCall(calls []string, substring string) string {
	for _, call := range calls {
		if strings.Contains(call, substring) {
			return call
		}
	}
	return ""
}
