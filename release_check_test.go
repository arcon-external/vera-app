package main

// scripts/release-check.sh is the release checklist's proof that the changelog
// names every fix/feat commit in the tag range. Like dependabot-status.sh it is
// bash, so it is proved by running the real script against a real, throwaway
// git repository and a fixture changelog. No network, no real tags, no state
// outside t.TempDir().
//
// The test pins the contract the release doc and the operator rely on:
//   - a fix whose issue reference is in the release section passes;
//   - a fix/feat the changelog does not name fails and is printed by reference;
//   - a commit with no issue reference can match on its subject;
//   - the --json document has a stable shape;
//   - the default range is the last two reachable tags.

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// releaseCheckEntry and releaseCheckReport mirror the --json document. They are
// declared here, not shared with the script, so a rename on either side fails
// the test instead of both sides agreeing on a new name.
type releaseCheckEntry struct {
	Subject    string   `json:"subject"`
	References []string `json:"references"`
	Matched    bool     `json:"matched"`
}

type releaseCheckReport struct {
	Range     string              `json:"range"`
	Changelog string              `json:"changelog"`
	Section   string              `json:"section"`
	Total     int                 `json:"total"`
	Matched   int                 `json:"matched"`
	Missing   int                 `json:"missing"`
	Entries   []releaseCheckEntry `json:"entries"`
}

type releaseCheckRepo struct {
	t      *testing.T
	dir    string
	bash   string
	script string
}

func newReleaseCheckRepo(t *testing.T) *releaseCheckRepo {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("release-check.sh is bash; its run is proved on the CI runner")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required by --json; install it to run this test")
	}
	script, err := filepath.Abs(filepath.Join("scripts", "release-check.sh"))
	if err != nil {
		t.Fatalf("resolve the release-check script: %v", err)
	}

	dir := t.TempDir()
	repo := &releaseCheckRepo{t: t, dir: dir, bash: bash, script: script}
	repo.git("init", "-q", "-b", "main")
	return repo
}

// git runs a git command in the throwaway repository with the user's global
// config and system config disabled, so a signing key or a hook cannot leak in.
func (r *releaseCheckRepo) git(args ...string) string {
	r.t.Helper()
	command := exec.Command("git", args...)
	command.Dir = r.dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	out, err := command.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (r *releaseCheckRepo) commit(subject string) {
	r.t.Helper()
	r.git("-c", "user.name=Release Check",
		"-c", "user.email=release-check@example.com",
		"-c", "commit.gpgsign=false",
		"commit", "-q", "--allow-empty", "-m", subject)
}

func (r *releaseCheckRepo) tag(name string) {
	r.t.Helper()
	r.git("-c", "tag.gpgsign=false", "tag", name)
}

func (r *releaseCheckRepo) writeFile(name, body string) {
	r.t.Helper()
	path := filepath.Join(r.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatalf("create the directory for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		r.t.Fatalf("write %s: %v", name, err)
	}
}

// run executes the real script in the throwaway repository and returns stdout,
// stderr and the exit status.
func (r *releaseCheckRepo) run(args ...string) (string, string, int) {
	r.t.Helper()
	command := exec.Command(r.bash, append([]string{r.script}, args...)...)
	command.Dir = r.dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			r.t.Fatalf("run the release check: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

// releaseChangelog builds a fixture with one version section. The check reads
// the release section only, so the section name matters.
func releaseChangelog(section string, entries ...string) string {
	var b strings.Builder
	b.WriteString("# Changelog\n\n")
	b.WriteString("## [" + section + "] - 2026-01-02\n\n### Fixed\n\n")
	for _, entry := range entries {
		b.WriteString("- " + entry + "\n")
	}
	return b.String()
}

func TestReleaseCheckMatchedFixPasses(t *testing.T) {
	repo := newReleaseCheckRepo(t)
	repo.commit("chore: initial")
	repo.tag("v0.1.0")
	repo.commit("fix: keep the widget aligned (#562)")
	// A custom path proves --changelog is honoured, not just accepted.
	repo.writeFile("release-notes.md",
		releaseChangelog("0.2.0", "**Widgets stay aligned.** The grid lines up (#562)."))

	stdout, stderr, code := repo.run("--range", "v0.1.0..HEAD", "--changelog", "release-notes.md")
	if code != 0 {
		t.Fatalf("a matched fix exited %d; stderr:\n%s\nstdout:\n%s", code, stderr, stdout)
	}
	for _, want := range []string{
		"v0.1.0..HEAD",
		"section 0.2.0",
		"release-notes.md",
		"fix/feat commits: 1",
		"matched: 1",
		"missing: 0",
		"OK",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the report is missing %q; stdout:\n%s", want, stdout)
		}
	}
}

func TestReleaseCheckUnmatchedFixFails(t *testing.T) {
	repo := newReleaseCheckRepo(t)
	repo.commit("chore: initial")
	repo.tag("v0.1.0")
	repo.commit("fix: keep the widget aligned (#562)")
	repo.commit("fix: repair the export button (#563)")
	repo.writeFile("CHANGELOG.md",
		releaseChangelog("0.2.0", "**Widgets stay aligned.** The grid lines up (#562)."))

	// --check and --dry-run are the read-only modes the checklist may name;
	// both must report the same gap as the bare run.
	for _, args := range [][]string{
		{"--range", "v0.1.0..HEAD"},
		{"--check", "--range", "v0.1.0..HEAD"},
		{"--dry-run", "--range", "v0.1.0..HEAD"},
	} {
		stdout, stderr, code := repo.run(args...)
		if code != 1 {
			t.Fatalf("%v exited %d; want 1. stderr:\n%s\nstdout:\n%s", args, code, stderr, stdout)
		}
		for _, want := range []string{
			"fix/feat commits: 2",
			"matched: 1",
			"missing: 1",
			"#563",
			"fix: repair the export button",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v: the report is missing %q; stdout:\n%s", args, want, stdout)
			}
		}
		if strings.Contains(stdout, "#562\n") {
			t.Errorf("%v: the report lists the matched reference as missing; stdout:\n%s", args, stdout)
		}
	}
}

func TestReleaseCheckJSONShapeIsStable(t *testing.T) {
	repo := newReleaseCheckRepo(t)
	repo.commit("chore: initial")
	repo.tag("v0.1.0")
	repo.commit("fix: keep the widget aligned (#562)")
	// No issue reference: this one must match on its subject.
	repo.commit("fix: repair the export button")
	repo.commit("feat: add the export button (#563)")
	repo.writeFile("CHANGELOG.md", releaseChangelog("0.2.0",
		"**Widgets stay aligned.** The grid lines up (#562).",
		"**Repair the export button.** It works again.",
	))

	stdout, stderr, code := repo.run("--json", "--range", "v0.1.0..HEAD")
	if code != 1 {
		t.Fatalf("--json exited %d; want 1. stderr:\n%s\nstdout:\n%s", code, stderr, stdout)
	}
	var got releaseCheckReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("--json did not emit one JSON document: %v\nstdout:\n%s", err, stdout)
	}

	want := releaseCheckReport{
		Range:     "v0.1.0..HEAD",
		Changelog: "CHANGELOG.md",
		Section:   "0.2.0",
		Total:     3,
		Matched:   2,
		Missing:   1,
		Entries: []releaseCheckEntry{
			// git log is newest first.
			{Subject: "feat: add the export button (#563)", References: []string{"#563"}, Matched: false},
			{Subject: "fix: repair the export button", References: []string{}, Matched: true},
			{Subject: "fix: keep the widget aligned (#562)", References: []string{"#562"}, Matched: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("--json reported\n%+v\nwant\n%+v", got, want)
	}

	// The struct comparison above ignores unknown fields. The shape is part of
	// the contract too, so pin the exact key sets.
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("decode the JSON document: %v", err)
	}
	assertKeys(t, "the report", document,
		"range", "changelog", "section", "total", "matched", "missing", "entries")
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(document["entries"], &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries has %d items; want 3", len(entries))
	}
	for _, entry := range entries {
		assertKeys(t, "entry", entry, "subject", "references", "matched")
	}
}

func TestReleaseCheckDefaultsToTheLastTwoTags(t *testing.T) {
	repo := newReleaseCheckRepo(t)
	repo.commit("chore: initial")
	repo.tag("v0.1.0")
	repo.commit("fix: keep the widget aligned (#562)")
	repo.tag("v0.1.1")
	repo.writeFile("CHANGELOG.md",
		releaseChangelog("0.1.1", "**Widgets stay aligned.** The grid lines up (#562)."))

	stdout, stderr, code := repo.run()
	if code != 0 {
		t.Fatalf("the default range exited %d; stderr:\n%s\nstdout:\n%s", code, stderr, stdout)
	}
	for _, want := range []string{"v0.1.0..v0.1.1", "section 0.1.1", "matched: 1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the default-range report is missing %q; stdout:\n%s", want, stdout)
		}
	}
}

func assertKeys(t *testing.T, what string, object map[string]json.RawMessage, want ...string) {
	t.Helper()
	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	if len(got) != len(want) {
		t.Fatalf("%s has keys %v; want %v", what, got, want)
	}
	for _, key := range want {
		if _, ok := object[key]; !ok {
			t.Errorf("%s is missing the key %q; got %v", what, key, got)
		}
	}
}
