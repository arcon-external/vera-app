package main

// .github/workflows/ci-image.yml is the only workflow that publishes the
// GTK/WebKit toolchain image, and nothing else in CI reads its shape. A
// permissions block could grow `packages: write` on the pull_request job, or
// the `latest` promotion could move above the validation step, and both would
// still parse and still run. This test pins what #685 promises:
//
//   - PRs build the Dockerfile only (`load`, `push: false`) and smoke-test
//     pkg-config; they never get a token that can write a package.
//   - Only non-PR runs publish, and only that job receives `packages: write`.
//   - The candidate is validated with the tag vet *before* `latest` moves, so
//     #686's consumers can never pull an image no Go toolchain has compiled.
//   - The digest is logged, and the Dockerfile stays digest-pinned, Go-free
//     and named after the packages and the `webkit2_41` build tag.
//
// It reads the files as text and YAML; it needs no Docker, no network and no
// token, so it runs in the ordinary `go test ./...` gate.

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const ciImageWorkflowPath = ".github/workflows/ci-image.yml"
const ciImageDockerfilePath = ".github/ci/gtk/Dockerfile"

type ciWorkflow struct {
	Name        string                   `yaml:"name"`
	On          map[string]any           `yaml:"on"`
	Permissions map[string]string        `yaml:"permissions"`
	Concurrency ciWorkflowConcurrency    `yaml:"concurrency"`
	Jobs        map[string]ciWorkflowJob `yaml:"jobs"`
}

type ciWorkflowConcurrency struct {
	Group            string `yaml:"group"`
	CancelInProgress any    `yaml:"cancel-in-progress"`
}

type ciWorkflowJob struct {
	Name           string            `yaml:"name"`
	If             string            `yaml:"if"`
	TimeoutMinutes int               `yaml:"timeout-minutes"`
	Permissions    map[string]string `yaml:"permissions"`
	Steps          []ciWorkflowStep  `yaml:"steps"`
}

type ciWorkflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	If   string            `yaml:"if"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

// ciStepMentions reports whether the step's name, run script or environment
// refers to needle. The digest check needs the env map: a step reads the build
// output through `env:` so the shell cannot interpolate it, which is the safe
// form and not something the test should force into `run:`.
func ciStepMentions(step ciWorkflowStep, needle string) bool {
	if strings.Contains(step.Name, needle) || strings.Contains(step.Run, needle) {
		return true
	}
	for _, value := range step.Env {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func loadCIWorkflow(t *testing.T) ciWorkflow {
	t.Helper()
	raw, err := os.ReadFile(ciImageWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", ciImageWorkflowPath, err)
	}
	var wf ciWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse %s: %v", ciImageWorkflowPath, err)
	}
	return wf
}

// ciMapping asserts the node is a mapping and returns it. Workflow triggers
// decode as map[string]any; a scalar where a mapping belongs is a shape error
// worth naming rather than a panic.
func ciMapping(t *testing.T, value any) map[string]any {
	t.Helper()
	mapping, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected a mapping, got %T (%v)", value, value)
	}
	return mapping
}

func ciStringList(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("expected a list, got %T (%v)", value, value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("expected a list of strings, got %T (%v)", item, item)
		}
		out = append(out, text)
	}
	return out
}

func ciWithString(t *testing.T, step ciWorkflowStep, key string) string {
	t.Helper()
	value, ok := step.With[key]
	if !ok {
		t.Fatalf("step %q has no with.%s", step.Name, key)
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("step %q with.%s is %T, want a string", step.Name, key, value)
	}
	return text
}

func ciWithBool(t *testing.T, step ciWorkflowStep, key string) bool {
	t.Helper()
	value, ok := step.With[key]
	if !ok {
		t.Fatalf("step %q has no with.%s", step.Name, key)
	}
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("step %q with.%s is %T, want a bool", step.Name, key, value)
	}
	return flag
}

// ciStepIndex finds the one step whose name contains needle. Names are part of
// the contract here, because the ordering assertions address steps by name.
func ciStepIndex(t *testing.T, steps []ciWorkflowStep, needle string) int {
	t.Helper()
	index := -1
	for i, step := range steps {
		if strings.Contains(step.Name, needle) {
			if index >= 0 {
				t.Fatalf("steps %q and %q both match %q; the ordering guard needs one",
					steps[index].Name, step.Name, needle)
			}
			index = i
		}
	}
	if index < 0 {
		t.Fatalf("no step name contains %q", needle)
	}
	return index
}

func ciStepUsing(t *testing.T, steps []ciWorkflowStep, action string) int {
	t.Helper()
	for i, step := range steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			return i
		}
	}
	t.Fatalf("no step uses %s", action)
	return -1
}

func TestCIImageWorkflowShape(t *testing.T) {
	wf := loadCIWorkflow(t)

	if strings.TrimSpace(wf.Name) == "" {
		t.Fatal("workflow has no name")
	}

	// Exactly four ways in. pull_request_target and workflow_run are the two
	// triggers that would hand a write token to a fork PR, and this file is
	// allowed to push a package: the absence of those keys is the guard.
	gotTriggers := slices.Sorted(maps.Keys(wf.On))
	wantTriggers := []string{"pull_request", "push", "schedule", "workflow_dispatch"}
	if !slices.Equal(gotTriggers, wantTriggers) {
		t.Fatalf("triggers = %v, want exactly %v", gotTriggers, wantTriggers)
	}

	wantPaths := []string{
		".github/ci/gtk/Dockerfile",
		".github/workflows/ci-image.yml",
	}
	for _, trigger := range []string{"pull_request", "push"} {
		triggerMap := ciMapping(t, wf.On[trigger])
		if got := ciStringList(t, triggerMap["paths"]); !slices.Equal(got, wantPaths) {
			t.Fatalf("%s.paths = %v, want %v", trigger, got, wantPaths)
		}
	}
	pushMap := ciMapping(t, wf.On["push"])
	if got := ciStringList(t, pushMap["branches"]); !slices.Equal(got, []string{"main"}) {
		t.Fatalf("push.branches = %v, want [main]", got)
	}
	if _, ok := wf.On["workflow_dispatch"]; !ok {
		t.Fatal("workflow_dispatch is missing; the first publish must be runnable by hand")
	}

	schedule, ok := wf.On["schedule"].([]any)
	if !ok || len(schedule) != 1 {
		t.Fatalf("schedule = %v, want exactly one cron entry", wf.On["schedule"])
	}
	cron, ok := ciMapping(t, schedule[0])["cron"].(string)
	if !ok {
		t.Fatalf("schedule[0].cron is not a string: %v", schedule[0])
	}
	fields := strings.Fields(cron)
	if len(fields) != 5 {
		t.Fatalf("schedule cron %q is not a five-field cron", cron)
	}
	if fields[4] == "*" {
		t.Fatalf("schedule cron %q runs every day; the image rebuilds weekly", cron)
	}

	// Least privilege at the workflow level; only the publish job may raise it.
	if got := wf.Permissions["contents"]; got != "read" {
		t.Fatalf("workflow permissions.contents = %q, want read", got)
	}
	if _, ok := wf.Permissions["packages"]; ok {
		t.Fatalf("workflow-level permissions grant packages: %q; only publish may", wf.Permissions["packages"])
	}
	if wf.Concurrency.Group == "" {
		t.Fatal("workflow has no concurrency group")
	}

	if got := slices.Sorted(maps.Keys(wf.Jobs)); !slices.Equal(got, []string{"build", "publish"}) {
		t.Fatalf("jobs = %v, want exactly [build publish]", got)
	}

	build := wf.Jobs["build"]
	publish := wf.Jobs["publish"]

	// The PR job: build only, read-only, and no package credential.
	if strings.TrimSpace(build.If) != "github.event_name == 'pull_request'" {
		t.Fatalf("build.if = %q; the build-only job must run only for pull_request", build.If)
	}
	if build.TimeoutMinutes != 45 {
		t.Fatalf("build.timeout-minutes = %d, want 45", build.TimeoutMinutes)
	}
	if len(build.Permissions) != 0 {
		t.Fatalf("build.permissions = %v; the PR job must inherit read-only, not declare its own", build.Permissions)
	}

	buildPushIdx := ciStepUsing(t, build.Steps, "docker/build-push-action")
	buildPush := build.Steps[buildPushIdx]
	if ciWithBool(t, buildPush, "push") {
		t.Fatal("the pull_request build pushes; it must only build")
	}
	if !ciWithBool(t, buildPush, "load") {
		t.Fatal("the pull_request build does not load the image; the smoke test cannot run it")
	}

	smokeIdx := ciStepIndex(t, build.Steps, "pkg-config")
	smoke := build.Steps[smokeIdx]
	for _, want := range []string{"docker run", "gtk+-3.0", "webkit2gtk-4.1"} {
		if !strings.Contains(smoke.Run, want) {
			t.Fatalf("the pkg-config smoke step does not mention %q:\n%s", want, smoke.Run)
		}
	}
	if smokeIdx < buildPushIdx {
		t.Fatal("the pkg-config smoke runs before the image is built")
	}

	// The publish job: the only writer, and only off the pull_request path.
	if strings.TrimSpace(publish.If) != "github.event_name != 'pull_request'" {
		t.Fatalf("publish.if = %q; publish must be limited to main, schedule and dispatch", publish.If)
	}
	if publish.TimeoutMinutes != 45 {
		t.Fatalf("publish.timeout-minutes = %d, want 45", publish.TimeoutMinutes)
	}
	if got := publish.Permissions["contents"]; got != "read" {
		t.Fatalf("publish permissions.contents = %q, want read", got)
	}
	if got := publish.Permissions["packages"]; got != "write" {
		t.Fatalf("publish permissions.packages = %q, want write", got)
	}
	if len(publish.Permissions) != 2 {
		t.Fatalf("publish.permissions = %v, want exactly contents: read and packages: write", publish.Permissions)
	}

	loginIdx := ciStepUsing(t, publish.Steps, "docker/login-action")
	login := publish.Steps[loginIdx]
	if got := ciWithString(t, login, "registry"); got != "ghcr.io" {
		t.Fatalf("login registry = %q, want ghcr.io", got)
	}
	if got := ciWithString(t, login, "password"); !strings.Contains(got, "GITHUB_TOKEN") {
		t.Fatalf("login password = %q; the publish job must use only GITHUB_TOKEN", got)
	}

	publishPushIdx := ciStepUsing(t, publish.Steps, "docker/build-push-action")
	publishPush := publish.Steps[publishPushIdx]
	if !ciWithBool(t, publishPush, "push") {
		t.Fatal("the publish build does not push")
	}
	tags := ciWithString(t, publishPush, "tags")
	for _, want := range []string{":sha-${{ github.sha }}", ":candidate"} {
		if !strings.Contains(tags, want) {
			t.Fatalf("publish tags %q do not include %q", tags, want)
		}
	}
	if got := ciWithString(t, publishPush, "cache-from"); got != "type=gha" {
		t.Fatalf("publish cache-from = %q, want type=gha", got)
	}
	if got := ciWithString(t, publishPush, "cache-to"); !strings.HasPrefix(got, "type=gha") {
		t.Fatalf("publish cache-to = %q, want type=gha", got)
	}

	digestIdx := ciStepIndex(t, publish.Steps, "digest")
	digestStep := publish.Steps[digestIdx]
	for _, want := range []string{"steps.build.outputs.digest", "GITHUB_STEP_SUMMARY"} {
		if !ciStepMentions(digestStep, want) {
			t.Fatalf("the digest step does not mention %q:\n%s", want, digestStep.Run)
		}
	}

	ciStepUsing(t, publish.Steps, "actions/checkout")
	nodeIdx := ciStepUsing(t, publish.Steps, "actions/setup-node")
	node := publish.Steps[nodeIdx]
	if got := ciWithString(t, node, "node-version-file"); got != ".nvmrc" {
		t.Fatalf("setup-node node-version-file = %q, want .nvmrc", got)
	}
	frontendIdx := ciStepIndex(t, publish.Steps, "Build frontend")
	if !strings.Contains(publish.Steps[frontendIdx].Run, "npm --prefix frontend run build") {
		t.Fatalf("the frontend step does not run the build:\n%s", publish.Steps[frontendIdx].Run)
	}
	goIdx := ciStepUsing(t, publish.Steps, "actions/setup-go")
	if got := ciWithString(t, publish.Steps[goIdx], "go-version-file"); got != "go.mod" {
		t.Fatalf("setup-go go-version-file = %q, want go.mod", got)
	}

	pullIdx := ciStepIndex(t, publish.Steps, "Pull the candidate")
	pull := publish.Steps[pullIdx]
	if !strings.Contains(pull.Run, "docker pull") || !strings.Contains(pull.Run, ":candidate") {
		t.Fatalf("the pull step does not pull the candidate:\n%s", pull.Run)
	}

	validateIdx := ciStepIndex(t, publish.Steps, "Validate the candidate")
	validate := publish.Steps[validateIdx]
	for _, want := range []string{
		"bash scripts/ci-tag-matrix.sh",
		"GOROOT",
		"/usr/local/go:ro",
		"GOMODCACHE",
		"GOCACHE",
		"GOTOOLCHAIN=local",
	} {
		if !strings.Contains(validate.Run, want) {
			t.Fatalf("the candidate validation does not mention %q:\n%s", want, validate.Run)
		}
	}
	// The publish gate and the PR step must run the same file. An inline copy
	// of the matrix here could validate a different set of commands than #495
	// executes, which is the drift this script exists to prevent.
	if strings.Contains(validate.Run, "go vet -tags") || strings.Contains(validate.Run, "go test -tags") {
		t.Fatalf("the candidate validation embeds the tag matrix instead of running scripts/ci-tag-matrix.sh:\n%s", validate.Run)
	}

	promoteIdx := ciStepIndex(t, publish.Steps, "Promote")
	promote := publish.Steps[promoteIdx]
	for _, want := range []string{"docker buildx imagetools create", ":latest", ":candidate"} {
		if !strings.Contains(promote.Run, want) {
			t.Fatalf("the promotion does not mention %q:\n%s", want, promote.Run)
		}
	}

	// The order that makes `latest` safe to consume: pull, validate, promote.
	if pullIdx > validateIdx {
		t.Fatal("the candidate is validated before it is pulled")
	}
	if validateIdx > promoteIdx {
		t.Fatal("latest is promoted before the candidate is validated")
	}
	for i, step := range publish.Steps {
		if i < validateIdx && strings.Contains(step.Run, "imagetools create") && strings.Contains(step.Run, ":latest") {
			t.Fatalf("step %q promotes latest before the validation step", step.Name)
		}
	}
	if !ciStepMentions(publish.Steps[ciStepIndex(t, publish.Steps, "Verify latest")], "steps.build.outputs.digest") {
		t.Fatal("the post-promotion check does not compare latest against the built digest")
	}

	// The verification step must not be the only place the digest is read:
	// GitHub's job summary is the durable record of what was published.
	if digestIdx > validateIdx {
		t.Fatal("the digest is logged after validation; the candidate's identity must be visible first")
	}
}

func TestCIImageDockerfile(t *testing.T) {
	raw, err := os.ReadFile(ciImageDockerfilePath)
	if err != nil {
		t.Fatalf("read %s: %v", ciImageDockerfilePath, err)
	}
	text := string(raw)

	var fromLines []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "FROM ") {
			fromLines = append(fromLines, trimmed)
		}
	}
	if len(fromLines) != 1 {
		t.Fatalf("Dockerfile has %d FROM lines (%v), want a single stage", len(fromLines), fromLines)
	}
	digestPin := regexp.MustCompile(`^FROM ubuntu:24\.04@sha256:[0-9a-f]{64}$`)
	if !digestPin.MatchString(fromLines[0]) {
		t.Fatalf("FROM = %q, want ubuntu:24.04 pinned by a sha256 digest", fromLines[0])
	}

	// No Go. The consuming job mounts the setup-go GOROOT, so the image cannot
	// drift from go.mod and a toolchain bump is not an image rebuild. Match a
	// real Go command rather than the prose in the comments.
	lower := strings.ToLower(text)
	for _, forbidden := range []string{"from golang:", "golang.org/dl"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("Dockerfile mentions %q; the Go toolchain must stay out of the image", forbidden)
		}
	}
	goCommand := regexp.MustCompile(`(?m)^\s*run\s+.*\bgo\s+(install|build|mod|test|vet)\b`)
	if goCommand.MatchString(lower) {
		t.Fatalf("Dockerfile runs a Go command; the Go toolchain must stay out of the image:\n%s",
			goCommand.FindString(lower))
	}

	for _, want := range []string{
		"--no-install-recommends",
		"--mount=type=cache,target=/var/cache/apt",
		"--mount=type=cache,target=/var/lib/apt",
		"rm -rf /var/lib/apt/lists",
		"ca-certificates",
		"git",
		"pkg-config",
		"build-essential",
		"libgtk-3-dev",
		"libwebkit2gtk-4.1-dev",
		"libsoup-3.0-dev",
		"org.opencontainers.image.source",
		"webkit2_41",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Dockerfile is missing %q", want)
		}
	}
}

func TestCIImageDependabotEntry(t *testing.T) {
	raw, err := os.ReadFile(".github/dependabot.yml")
	if err != nil {
		t.Fatalf("read .github/dependabot.yml: %v", err)
	}
	var config struct {
		Updates []struct {
			Ecosystem string `yaml:"package-ecosystem"`
			Directory string `yaml:"directory"`
			Schedule  struct {
				Interval string `yaml:"interval"`
			} `yaml:"schedule"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("parse .github/dependabot.yml: %v", err)
	}

	var found bool
	for _, update := range config.Updates {
		if update.Ecosystem == "docker" && update.Directory == "/.github/ci/gtk" {
			if update.Schedule.Interval != "weekly" {
				t.Fatalf("the toolchain image updates %q, want weekly", update.Schedule.Interval)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no docker Dependabot entry for /.github/ci/gtk; the pinned Ubuntu digest would never move")
	}
}
