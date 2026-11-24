package main

// The `#495` step in pr.yml used to install GTK/WebKit with apt on every run
// that needed the tag matrix. #685 publishes a prebuilt image and #686 makes
// the step pull and run it. Nothing else in CI reads the shape of that step, so
// a dropped `packages: read`, a missing login, a lost mount or a reintroduced
// apt line would all still parse and still run -- silently reintroducing the
// 22-minute install or producing a confusing pkg-config failure.
//
// This test pins the #495 contract:
//
//   - the go-store job declares exactly `contents: read` and `packages: read`;
//   - it logs in to ghcr.io with GITHUB_TOKEN before pulling;
//   - the pull retries three times, fails closed with an error, and logs the
//     resolved image digest;
//   - the matrix runs in the image as the runner's uid, with the workspace,
//     the read-only host GOROOT and both Go caches mounted, and the
//     CGO/GOTOOLCHAIN/GOFLAGS settings the build depends on;
//   - the command is scripts/ci-tag-matrix.sh -- not an inline copy -- and no
//     apt-get survives in the step.
//
// It reads the workflow and script as text and YAML; it needs no Docker, no
// network and no token, so it runs in the ordinary `go test ./...` gate.

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const prWorkflowPath = ".github/workflows/pr.yml"
const ciTagMatrixScriptPath = "scripts/ci-tag-matrix.sh"

func loadPRWorkflow(t *testing.T) ciWorkflow {
	t.Helper()
	raw, err := os.ReadFile(prWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", prWorkflowPath, err)
	}
	var wf ciWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse %s: %v", prWorkflowPath, err)
	}
	return wf
}

// ciRequireContains asserts every needle appears in text and reports all the
// missing ones at once, so a mutation shows the whole contract it broke.
func ciRequireContains(t *testing.T, label, text string, needles ...string) {
	t.Helper()
	var missing []string
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			missing = append(missing, needle)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%s is missing %v:\n%s", label, missing, text)
	}
}

func TestPR495TagMatrixWorkflowShape(t *testing.T) {
	wf := loadPRWorkflow(t)

	job, ok := wf.Jobs["go-store"]
	if !ok {
		t.Fatalf("%s has no go-store job", prWorkflowPath)
	}

	// The authenticated pull is the contract. Naming a permission scope zeroes
	// every scope not named, so contents must be explicit; packages: read is
	// the only extra grant and it must not be write.
	wantPermissions := map[string]string{"contents": "read", "packages": "read"}
	if len(job.Permissions) != len(wantPermissions) {
		t.Fatalf("go-store permissions = %v, want exactly %v", job.Permissions, wantPermissions)
	}
	for scope, want := range wantPermissions {
		if got := job.Permissions[scope]; got != want {
			t.Fatalf("go-store permissions.%s = %q, want %q", scope, got, want)
		}
	}

	loginIdx := ciStepUsing(t, job.Steps, "docker/login-action")
	login := job.Steps[loginIdx]
	if got := ciWithString(t, login, "registry"); got != "ghcr.io" {
		t.Fatalf("login registry = %q, want ghcr.io", got)
	}
	if got := ciWithString(t, login, "password"); !strings.Contains(got, "GITHUB_TOKEN") {
		t.Fatalf("login password = %q; the private pull must use GITHUB_TOKEN", got)
	}
	if got := ciWithString(t, login, "username"); !strings.Contains(got, "github.actor") {
		t.Fatalf("login username = %q, want github.actor", got)
	}

	pullIdx := ciStepIndex(t, job.Steps, "Pull the CI toolchain image")
	pull := job.Steps[pullIdx]
	ciRequireContains(t, "the toolchain pull",
		pull.Run,
		"ghcr.io/arcon-external/vera-app/ci-toolchain:latest",
		"for attempt in 1 2 3",
		"docker pull",
		"::error::",
		"exit 1",
		"RepoDigests",
		"::notice",
		"CI_TOOLCHAIN_IMAGE=${digest}",
	)
	if !strings.Contains(pull.Run, "could not pull") {
		t.Fatalf("the pull failure does not explain itself:\n%s", pull.Run)
	}

	tagsIdx := ciStepIndex(t, job.Steps, "#495")
	tags := job.Steps[tagsIdx]
	ciRequireContains(t, "the #495 step",
		tags.Run,
		"docker run",
		`--user "$(id -u):$(id -g)"`,
		`${GITHUB_WORKSPACE}:/src`,
		`$(go env GOROOT):/goroot:ro`,
		`${HOME}/go/pkg/mod:/go/pkg/mod`,
		`${HOME}/.cache/go-build:/go-build`,
		"GOROOT=/goroot",
		"GOMODCACHE=/go/pkg/mod",
		"GOCACHE=/go-build",
		"CGO_ENABLED=1",
		"GOTOOLCHAIN=local",
		"GOFLAGS=-mod=readonly",
		"bash scripts/ci-tag-matrix.sh",
	)
	if strings.Contains(tags.Run, "apt-get") {
		t.Fatalf("the #495 step still runs apt-get:\n%s", tags.Run)
	}
	// One source of truth: the workflow invokes the script, it does not carry
	// its own copy of the matrix.
	if strings.Contains(tags.Run, "go vet -tags") || strings.Contains(tags.Run, "go test -tags") {
		t.Fatalf("the #495 step embeds the tag matrix instead of running the script:\n%s", tags.Run)
	}

	// Login, then pull, then run. A skipped gate on any of them is also a
	// regression: the login must not run on unrelated PRs, and the matrix must
	// never run without the pull.
	if !(loginIdx < pullIdx && pullIdx < tagsIdx) {
		t.Fatalf("step order login=%d pull=%d matrix=%d, want login < pull < matrix",
			loginIdx, pullIdx, tagsIdx)
	}
	for _, step := range []ciWorkflowStep{login, pull, tags} {
		ciRequireContains(t, "step "+step.Name, step.If,
			"steps.scope.outputs.needs_tags != 'false'",
		)
	}
}

func TestCITagMatrixScript(t *testing.T) {
	raw, err := os.ReadFile(ciTagMatrixScriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", ciTagMatrixScriptPath, err)
	}
	text := string(raw)

	info, err := os.Stat(ciTagMatrixScriptPath)
	if err != nil {
		t.Fatalf("stat %s: %v", ciTagMatrixScriptPath, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s is not executable", ciTagMatrixScriptPath)
	}

	ciRequireContains(t, "the tag matrix script",
		text,
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"go vet -tags dev,webkit2_41 ./ ./backend/auth",
		"-run 'TestAPIBaseURL|TestProductionAPIURL|TestKeyring' ./ ./backend/auth",
		"for tags in production dev production,debug; do",
		`go test -tags "$tags,webkit2_41" -count=1`,
		"go test -tags dev,production -count=1",
	)
}
