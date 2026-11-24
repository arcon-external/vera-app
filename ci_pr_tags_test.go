package main

// The Gates workflow decides most of its own work from one merge-base diff.
// Nothing else in CI reads the shape of that decision or of the #495 tag
// matrix, so a gate dropped from one job, a `packages: read` lost, a login
// removed, an apt line reintroduced or the matrix copied inline would all
// still parse and still run -- silently restoring the serial cost, breaking
// the private pull, or skipping the work a PR needs.
//
// This test pins the #698 contract:
//
//   - exactly one `scope` job computes `needs_go` and `needs_tags` from the
//     merge base, with the fail-open rule (a crashed filter runs the work);
//   - `go-api-shard`, `go-store` and `go-api-union` consume it through
//     `needs: scope` plus a job-level `if`, with no per-job diff step and no
//     step-level scope condition left behind;
//   - `go-api-union` is gated on `needs_go`;
//   - `go-tags` runs the authenticated pull and the matrix in parallel with
//     the test jobs, with `contents: read` + `packages: read`, no services,
//     no Postgres/PDF/frontend tests, and `scripts/ci-tag-matrix.sh` -- not
//     an inline copy and never apt;
//   - `frontend` stays independent of the Go decisions.
//
// `TestPRWorkflowContractMutations` proves every guard above can fail: each
// mutation breaks one rule in the real file and must be rejected. It reads
// the workflow and script as text and YAML; it needs no Docker, no network
// and no token, so it runs in the ordinary `go test ./...` gate.

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const prWorkflowPath = ".github/workflows/pr.yml"
const ciTagMatrixScriptPath = "scripts/ci-tag-matrix.sh"

const (
	prNeedsGoGate   = "needs.scope.outputs.needs_go != 'false'"
	prNeedsTagsGate = "needs.scope.outputs.needs_tags != 'false'"
)

// prCheck collects every violation instead of stopping at the first, so one
// run reports the whole contract a mutation broke.
type prCheck struct {
	problems []string
}

func (c *prCheck) require(ok bool, format string, args ...any) {
	if !ok {
		c.problems = append(c.problems, fmt.Sprintf(format, args...))
	}
}

func (c *prCheck) contains(label, text string, needles ...string) {
	for _, needle := range needles {
		c.require(strings.Contains(text, needle), "%s is missing %q", label, needle)
	}
}

func (c *prCheck) notContains(label, text string, forbidden ...string) {
	for _, needle := range forbidden {
		c.require(!strings.Contains(text, needle), "%s must not contain %q", label, needle)
	}
}

func (c *prCheck) err() error {
	if len(c.problems) == 0 {
		return nil
	}
	slices.Sort(c.problems)
	return fmt.Errorf("pr.yml contract violations:\n  - %s", strings.Join(c.problems, "\n  - "))
}

func prStepUsing(steps []ciWorkflowStep, action string) int {
	for i, step := range steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			return i
		}
	}
	return -1
}

func prStepNamed(steps []ciWorkflowStep, needle string) int {
	for i, step := range steps {
		if strings.Contains(step.Name, needle) {
			return i
		}
	}
	return -1
}

func prStepWithID(steps []ciWorkflowStep, id string) int {
	for i, step := range steps {
		if step.ID == id {
			return i
		}
	}
	return -1
}

func prStepWith(step ciWorkflowStep, key string) (string, bool) {
	value, ok := step.With[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func prStepWithInt(step ciWorkflowStep, key string) (int, bool) {
	value, ok := step.With[key]
	if !ok {
		return 0, false
	}
	number, ok := value.(int)
	return number, ok
}

// prNeeds normalises `needs` to a list. YAML accepts one string or a list, and
// the shape is not the contract; the set of jobs is.
func prNeeds(job ciWorkflowJob) []string {
	switch needs := job.Needs.(type) {
	case string:
		if needs == "" {
			return nil
		}
		return []string{needs}
	case []any:
		out := make([]string, 0, len(needs))
		for _, value := range needs {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	case nil:
		return nil
	default:
		return []string{fmt.Sprintf("%v", needs)}
	}
}

func prContinueOnError(step ciWorkflowStep) bool {
	flag, ok := step.ContinueOnError.(bool)
	return ok && flag
}

// prCheckNoStepGates is the "a job-level gate cannot be bypassed by a
// step-level condition left behind" rule. Every consumer is admitted or
// skipped as a whole; a surviving step `if` can only narrow it again.
func prCheckNoStepGates(c *prCheck, id string, job ciWorkflowJob) {
	for _, step := range job.Steps {
		label := fmt.Sprintf("%s step %q", id, step.Name)
		c.require(!strings.Contains(step.If, "scope"),
			"%s has a step-level scope gate (%q); the job gate must be the only gate", label, step.If)
		c.require(step.ID != "scope", "%s still runs the per-job scope step", label)
		for _, text := range []string{step.Name, step.Run, step.If, step.Uses} {
			c.require(!strings.Contains(text, "steps.scope"), "%s still references steps.scope", label)
		}
	}
}

func validatePRWorkflow(raw []byte) error {
	var wf ciWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return fmt.Errorf("parse %s: %w", prWorkflowPath, err)
	}

	c := &prCheck{}

	// ---- The shared Scope job -------------------------------------------
	scope, ok := wf.Jobs["scope"]
	if !ok {
		return errors.New("pr.yml has no scope job")
	}
	c.require(scope.RunsOn == "ubuntu-24.04", "scope runs-on = %q, want ubuntu-24.04", scope.RunsOn)
	c.require(scope.TimeoutMinutes == 5, "scope timeout-minutes = %d, want 5", scope.TimeoutMinutes)
	c.require(len(scope.Permissions) == 1 && scope.Permissions["contents"] == "read",
		"scope permissions = %v, want exactly contents: read", scope.Permissions)
	c.require(len(scope.Outputs) == 2,
		"scope outputs = %v, want exactly needs_go and needs_tags", scope.Outputs)
	c.require(scope.Outputs["needs_go"] == "${{ steps.scope.outputs.needs_go }}",
		"scope outputs.needs_go = %q, want the scope step's output", scope.Outputs["needs_go"])
	c.require(scope.Outputs["needs_tags"] == "${{ steps.scope.outputs.needs_tags }}",
		"scope outputs.needs_tags = %q, want the scope step's output", scope.Outputs["needs_tags"])

	checkoutIdx := prStepUsing(scope.Steps, "actions/checkout")
	c.require(checkoutIdx >= 0, "scope job has no checkout step")
	if checkoutIdx >= 0 {
		depth, depthOK := prStepWithInt(scope.Steps[checkoutIdx], "fetch-depth")
		c.require(depthOK && depth == 0,
			"scope checkout fetch-depth = %v, want 0 so the merge base is present", depth)
	}
	scopeIdx := prStepWithID(scope.Steps, "scope")
	c.require(scopeIdx >= 0, "scope job has no step with id: scope")
	if scopeIdx >= 0 {
		filter := scope.Steps[scopeIdx]
		c.require(prContinueOnError(filter),
			"scope step must be continue-on-error: true so a crashed filter runs the work (fail-open)")
		c.contains("the scope filter", filter.Run,
			`if [ "${{ github.event_name }}" != "pull_request" ]`,
			`echo "needs_go=true"`,
			`echo "needs_tags=true"`,
			`base="${{ github.event.pull_request.base.sha }}"`,
			`git diff --no-renames --name-only "$base"...HEAD`,
			`grep -qvE '^frontend/(src|public)/'`,
			`^[^/]+\.go$|^backend/auth/|^wails\.json$|^go\.(mod|sum)$`,
			`needs_go=$needs_go`,
			`needs_tags=$needs_tags`,
			`GITHUB_OUTPUT`,
		)
		c.require(checkoutIdx >= 0 && checkoutIdx < scopeIdx,
			"scope filter runs before checkout (checkout=%d filter=%d)", checkoutIdx, scopeIdx)
	}

	// One diff, one place. A second copy is how the store and shard scope
	// steps drifted before #698.
	diffSites := 0
	for id, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Run, "git diff --no-renames --name-only") {
				diffSites++
				c.require(id == "scope", "the merge-base diff must live in scope, but %s also diffs", id)
			}
		}
	}
	c.require(diffSites == 1,
		"the merge-base diff must appear exactly once (shared Scope job), found %d", diffSites)

	// ---- Consumers of needs_go ------------------------------------------
	checkConsumer := func(id string) (ciWorkflowJob, bool) {
		job, found := wf.Jobs[id]
		if !found {
			c.require(false, "%s job is missing", id)
			return ciWorkflowJob{}, false
		}
		needs := prNeeds(job)
		c.require(slices.Equal(needs, []string{"scope"}),
			"%s needs = %v, want [scope]", id, needs)
		c.require(strings.TrimSpace(job.If) == prNeedsGoGate,
			"%s job-level gate = %q, want %q", id, strings.TrimSpace(job.If), prNeedsGoGate)
		prCheckNoStepGates(c, id, job)
		return job, true
	}

	shard, shardOK := checkConsumer("go-api-shard")
	if shardOK {
		_, hasPostgres := shard.Services["postgres"]
		c.require(hasPostgres, "go-api-shard lost its postgres service")
		c.contains("go-api-shard env", shard.Env["VERA_POSTGRES_TEST_DSN"], "postgres://")
	}

	store, storeOK := checkConsumer("go-store")
	if storeOK {
		c.require(len(store.Permissions) == 0,
			"go-store declares permissions %v; the image pull moved to go-tags", store.Permissions)
		_, hasPostgres := store.Services["postgres"]
		c.require(hasPostgres, "go-store lost its postgres service")
		for _, step := range store.Steps {
			label := "go-store step " + step.Name
			c.require(!strings.Contains(step.Name, "#495"),
				"go-store still owns the #495 step")
			c.require(!strings.HasPrefix(step.Uses, "docker/login-action@"),
				"%s still logs in to GHCR", label)
			c.notContains(label, step.Run, "docker pull", "scripts/ci-tag-matrix.sh", "CI_TOOLCHAIN_IMAGE")
		}
	}

	union, unionOK := checkConsumer("go-api-union")
	if unionOK {
		c.require(len(union.Services) == 0,
			"go-api-union declares services %v; the union guard needs no database", union.Services)
	}

	// ---- The tag matrix, now its own job --------------------------------
	tags, ok := wf.Jobs["go-tags"]
	if !ok {
		c.require(false, "go-tags job is missing")
	} else {
		needs := prNeeds(tags)
		c.require(slices.Equal(needs, []string{"scope"}),
			"go-tags needs = %v, want [scope]", needs)
		c.require(strings.TrimSpace(tags.If) == prNeedsTagsGate,
			"go-tags job-level gate = %q, want %q", strings.TrimSpace(tags.If), prNeedsTagsGate)
		c.require(tags.TimeoutMinutes == 20,
			"go-tags timeout-minutes = %d, want 20", tags.TimeoutMinutes)
		wantPermissions := map[string]string{"contents": "read", "packages": "read"}
		c.require(len(tags.Permissions) == len(wantPermissions),
			"go-tags permissions = %v, want exactly %v", tags.Permissions, wantPermissions)
		for name, want := range wantPermissions {
			c.require(tags.Permissions[name] == want,
				"go-tags permissions.%s = %q, want %q", name, tags.Permissions[name], want)
		}
		c.require(len(tags.Services) == 0,
			"go-tags declares services %v; the matrix needs none", tags.Services)
		_, hasDSN := tags.Env["VERA_POSTGRES_TEST_DSN"]
		c.require(!hasDSN, "go-tags must not receive a Postgres DSN")
		prCheckNoStepGates(c, "go-tags", tags)

		checkoutIdx := prStepUsing(tags.Steps, "actions/checkout")
		cacheIdx := prStepUsing(tags.Steps, "actions/cache/restore")
		nodeIdx := prStepUsing(tags.Steps, "actions/setup-node")
		installIdx := prStepNamed(tags.Steps, "Install frontend dependencies")
		buildIdx := prStepNamed(tags.Steps, "Build frontend")
		goIdx := prStepUsing(tags.Steps, "actions/setup-go")
		loginIdx := prStepUsing(tags.Steps, "docker/login-action")
		pullIdx := prStepNamed(tags.Steps, "Pull the CI toolchain image")
		matrixIdx := prStepNamed(tags.Steps, "#495")

		order := []int{checkoutIdx, cacheIdx, nodeIdx, installIdx, buildIdx, goIdx, loginIdx, pullIdx, matrixIdx}
		orderLabels := []string{"checkout", "cache", "node", "install", "build", "setup-go", "login", "pull", "matrix"}
		for i, index := range order {
			c.require(index >= 0, "go-tags is missing its %s step", orderLabels[i])
			if i > 0 {
				c.require(order[i-1] >= 0 && index > order[i-1],
					"go-tags step order puts %s before %s; want %v", orderLabels[i], orderLabels[i-1], orderLabels)
			}
		}

		if cacheIdx >= 0 {
			cache := tags.Steps[cacheIdx]
			path, _ := prStepWith(cache, "path")
			key, _ := prStepWith(cache, "key")
			restore, _ := prStepWith(cache, "restore-keys")
			c.contains("go-tags cache path", path, "~/.cache/go-build", "~/go/pkg/mod")
			c.require(strings.Contains(key, "hashFiles('go.sum')"),
				"go-tags cache key = %q, want hashFiles('go.sum')", key)
			c.contains("go-tags cache restore-keys", restore, "go-${{ runner.os }}-")
		}
		if nodeIdx >= 0 {
			nodeVersion, _ := prStepWith(tags.Steps[nodeIdx], "node-version-file")
			c.require(nodeVersion == ".nvmrc",
				"go-tags setup-node node-version-file = %q, want .nvmrc", nodeVersion)
		}
		if installIdx >= 0 {
			c.contains("go-tags npm ci", tags.Steps[installIdx].Run, "npm --prefix frontend ci")
		}
		if buildIdx >= 0 {
			c.contains("go-tags frontend build", tags.Steps[buildIdx].Run,
				"npm --prefix frontend run build")
		}
		if goIdx >= 0 {
			goVersion, _ := prStepWith(tags.Steps[goIdx], "go-version-file")
			c.require(goVersion == "go.mod",
				"go-tags setup-go go-version-file = %q, want go.mod", goVersion)
			cacheFlag, flagOK := tags.Steps[goIdx].With["cache"].(bool)
			c.require(flagOK && !cacheFlag,
				"go-tags setup-go cache = %v, want false; the explicit cache owns it", tags.Steps[goIdx].With["cache"])
		}

		if loginIdx >= 0 {
			login := tags.Steps[loginIdx]
			registry, _ := prStepWith(login, "registry")
			c.require(registry == "ghcr.io",
				"go-tags login registry = %q, want ghcr.io", registry)
			password, _ := prStepWith(login, "password")
			c.require(strings.Contains(password, "GITHUB_TOKEN"),
				"go-tags GHCR login must use GITHUB_TOKEN, got %q", password)
			username, _ := prStepWith(login, "username")
			c.require(strings.Contains(username, "github.actor"),
				"go-tags login username = %q, want github.actor", username)
		} else {
			c.require(false, "go-tags must log in to GHCR before pulling the private image")
		}

		if pullIdx >= 0 {
			pull := tags.Steps[pullIdx]
			c.contains("the toolchain pull", pull.Run,
				"ghcr.io/arcon-external/vera-app/ci-toolchain:latest",
				"for attempt in 1 2 3",
				"docker pull",
				"::error::",
				"exit 1",
				"RepoDigests",
				"::notice",
				"CI_TOOLCHAIN_IMAGE=${digest}",
				"could not pull",
			)
		}

		if matrixIdx >= 0 {
			matrix := tags.Steps[matrixIdx]
			c.contains("the #495 step", matrix.Run,
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
			c.require(strings.Contains(matrix.Run, "bash scripts/ci-tag-matrix.sh"),
				"go-tags must run scripts/ci-tag-matrix.sh, not an inline copy")
			c.require(!strings.Contains(matrix.Run, "go vet -tags") &&
				!strings.Contains(matrix.Run, "go test -tags"),
				"go-tags embeds the tag matrix instead of running scripts/ci-tag-matrix.sh")
		}

		for _, step := range tags.Steps {
			label := "go-tags step " + step.Name
			c.require(!strings.Contains(step.Run, "apt-get"),
				"%s runs apt-get; the image supplies the toolchain", label)
			c.notContains(label, step.Run,
				"VERA_POSTGRES_TEST_DSN", "TestGeneratePDF", "npm --prefix frontend test")
			c.require(!strings.Contains(step.Run, "go test -race"),
				"%s runs the race suite; it belongs to the test jobs", label)
		}
	}

	// ---- The frontend job stays independent ------------------------------
	frontend, ok := wf.Jobs["frontend"]
	if !ok {
		c.require(false, "frontend job is missing")
	} else {
		needs := prNeeds(frontend)
		c.require(len(needs) == 0,
			"frontend needs = %v; it must stay independent of the Go decisions", needs)
		c.require(strings.TrimSpace(frontend.If) == "",
			"frontend job-level if = %q; it must run on every PR", strings.TrimSpace(frontend.If))
	}

	return c.err()
}

func TestPR495TagMatrixWorkflowShape(t *testing.T) {
	raw, err := os.ReadFile(prWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", prWorkflowPath, err)
	}
	if err := validatePRWorkflow(raw); err != nil {
		t.Fatal(err)
	}
}

// prJobHeader matches a top-level job key inside `jobs:`. Nested keys are
// indented four spaces or more, and the env block's program is indented, so
// only job headers are at exactly two.
var prJobHeader = regexp.MustCompile(`(?m)^  ([A-Za-z0-9_-]+):\s*$`)

func prJobSpan(raw, id string) (start, end int, ok bool) {
	matches := prJobHeader.FindAllStringSubmatchIndex(raw, -1)
	for i, match := range matches {
		if raw[match[2]:match[3]] != id {
			continue
		}
		start = match[0]
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = len(raw)
		}
		return start, end, true
	}
	return 0, 0, false
}

func prRemoveJobLine(raw, id, needle string) (string, bool) {
	start, end, ok := prJobSpan(raw, id)
	if !ok {
		return raw, false
	}
	block := raw[start:end]
	index := strings.Index(block, needle)
	if index < 0 {
		return raw, false
	}
	lineStart := strings.LastIndex(block[:index], "\n") + 1
	lineEnd := strings.Index(block[index:], "\n")
	if lineEnd < 0 {
		lineEnd = len(block)
	} else {
		lineEnd = index + lineEnd + 1
	}
	return raw[:start] + block[:lineStart] + block[lineEnd:] + raw[end:], true
}

func prReplaceJobText(raw, id, old, replacement string) (string, bool) {
	start, end, ok := prJobSpan(raw, id)
	if !ok {
		return raw, false
	}
	block := raw[start:end]
	if !strings.Contains(block, old) {
		return raw, false
	}
	return raw[:start] + strings.Replace(block, old, replacement, 1) + raw[end:], true
}

func prRemoveJobStep(raw, id, name string) (string, bool) {
	start, end, ok := prJobSpan(raw, id)
	if !ok {
		return raw, false
	}
	block := raw[start:end]
	marker := "      - name: " + name + "\n"
	index := strings.Index(block, marker)
	if index < 0 {
		return raw, false
	}
	next := strings.Index(block[index+len(marker):], "\n      - ")
	stepEnd := len(block)
	if next >= 0 {
		stepEnd = index + len(marker) + next + 1
	}
	return raw[:start] + block[:index] + block[stepEnd:] + raw[end:], true
}

type prMutation struct {
	name  string
	apply func(string) (string, bool)
	want  string
}

func TestPRWorkflowContractMutations(t *testing.T) {
	raw, err := os.ReadFile(prWorkflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", prWorkflowPath, err)
	}
	original := string(raw)

	mutations := []prMutation{
		{
			name:  "store gate dropped",
			apply: func(s string) (string, bool) { return prRemoveJobLine(s, "go-store", "if: "+prNeedsGoGate) },
			want:  "go-store job-level gate",
		},
		{
			name:  "shard gate dropped",
			apply: func(s string) (string, bool) { return prRemoveJobLine(s, "go-api-shard", "if: "+prNeedsGoGate) },
			want:  "go-api-shard job-level gate",
		},
		{
			name:  "union gate dropped",
			apply: func(s string) (string, bool) { return prRemoveJobLine(s, "go-api-union", "if: "+prNeedsGoGate) },
			want:  "go-api-union job-level gate",
		},
		{
			name:  "tags gate dropped",
			apply: func(s string) (string, bool) { return prRemoveJobLine(s, "go-tags", "if: "+prNeedsTagsGate) },
			want:  "go-tags job-level gate",
		},
		{
			name:  "tags no longer needs scope",
			apply: func(s string) (string, bool) { return prRemoveJobLine(s, "go-tags", "needs: scope") },
			want:  "go-tags needs",
		},
		{
			name:  "packages read dropped",
			apply: func(s string) (string, bool) { return prRemoveJobLine(s, "go-tags", "      packages: read") },
			want:  "go-tags permissions",
		},
		{
			name:  "GHCR login dropped",
			apply: func(s string) (string, bool) { return prRemoveJobStep(s, "go-tags", "Log in to GHCR") },
			want:  "go-tags must log in to GHCR",
		},
		{
			name: "apt reintroduced",
			apply: func(s string) (string, bool) {
				return prReplaceJobText(s, "go-tags", "bash scripts/ci-tag-matrix.sh",
					"sudo apt-get update && bash scripts/ci-tag-matrix.sh")
			},
			want: "go-tags step API configuration and session build modes (#495) runs apt-get",
		},
		{
			name: "matrix inlined",
			apply: func(s string) (string, bool) {
				return prReplaceJobText(s, "go-tags", "bash scripts/ci-tag-matrix.sh",
					"go vet -tags dev,webkit2_41 ./ ./backend/auth")
			},
			want: "must run scripts/ci-tag-matrix.sh, not an inline copy",
		},
		{
			name: "fail-open broken",
			apply: func(s string) (string, bool) {
				return prReplaceJobText(s, "scope", "continue-on-error: true", "continue-on-error: false")
			},
			want: "scope step must be continue-on-error: true",
		},
		{
			name: "step-level gate reintroduced",
			apply: func(s string) (string, bool) {
				return prReplaceJobText(s, "go-api-shard", "      - name: Generate this shard's test list",
					"      - name: Generate this shard's test list\n        if: "+prNeedsGoGate)
			},
			want: "has a step-level scope gate",
		},
		{
			name: "store owns #495 again",
			apply: func(s string) (string, bool) {
				return prReplaceJobText(s, "go-store", "      - name: gofmt check",
					"      - name: API configuration and session build modes (#495)")
			},
			want: "go-store still owns the #495 step",
		},
		{
			name: "second merge-base diff",
			apply: func(s string) (string, bool) {
				return prReplaceJobText(s, "go-store", "      - name: gofmt check",
					"      - name: Diff again\n        run: git diff --no-renames --name-only base...HEAD\n\n      - name: gofmt check")
			},
			want: "merge-base diff must appear exactly once",
		},
		{
			name: "scope output dropped",
			apply: func(s string) (string, bool) {
				return prRemoveJobLine(s, "scope", "needs_tags: ${{ steps.scope.outputs.needs_tags }}")
			},
			want: "scope outputs",
		},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, ok := mutation.apply(original)
			if !ok {
				t.Fatalf("mutation %q did not apply; the workflow text changed and this needs updating", mutation.name)
			}
			if mutated == original {
				t.Fatalf("mutation %q left the workflow unchanged", mutation.name)
			}
			err := validatePRWorkflow([]byte(mutated))
			if err == nil {
				t.Fatalf("mutation %q survived; the contract does not detect it", mutation.name)
			}
			if !strings.Contains(err.Error(), mutation.want) {
				t.Fatalf("mutation %q was caught for the wrong reason (want %q):\n%v",
					mutation.name, mutation.want, err)
			}
		})
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
