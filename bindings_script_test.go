package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The generated bindings are committed build output, because the frontend
// imports them and the Wails CLI is not a build dependency. That makes
// "regenerate and commit" a step a human has to remember, and the generator's
// whitespace churn makes the resulting diff unreadable: at v2.17.1 it puts a
// tab on every otherwise-blank line of models.ts (552 lines) and marks every
// file it writes executable.
//
// scripts/gen-bindings.sh owns that step. These tests drive it against a
// throwaway repository so they can prove the two properties that matter: the
// churn is normalized away, and a bound method the generator missed still
// fails.

func TestGenBindingsScriptNormalizesGeneratorWhitespace(t *testing.T) {
	root := newBindingsScriptRoot(t)
	fakeBin := writeFakeWails(t)

	out, code := runBindingsScript(t, root, fakeBin, "churn")
	if code != 0 {
		t.Fatalf("gen-bindings.sh exited %d, want 0\n%s", code, out)
	}

	target := filepath.Join(root, "frontend", "wailsjs", "go", "backend", "Quotation.js")
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.TrimRight(line, " \t") != line {
			t.Errorf("line %d still carries trailing whitespace after normalization: %q", i+1, line)
		}
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("generated file mode = %04o, want 0644", got)
	}

	if status := gitOutput(t, root, "status", "--porcelain", "--", "frontend/wailsjs"); status != "" {
		t.Errorf("normalization left a diff in frontend/wailsjs:\n%s", status)
	}
}

func TestGenBindingsScriptCheckLeavesTheTreeAlone(t *testing.T) {
	root := newBindingsScriptRoot(t)
	fakeBin := writeFakeWails(t)

	wailsjs := filepath.Join(root, "frontend", "wailsjs")
	before := treeFingerprint(t, wailsjs)

	out, code := runBindingsScript(t, root, fakeBin, "churn", "--check")
	if code != 0 {
		t.Fatalf("gen-bindings.sh --check exited %d, want 0\n%s", code, out)
	}

	if after := treeFingerprint(t, wailsjs); after != before {
		t.Errorf("--check changed frontend/wailsjs:\nbefore %s\nafter  %s", before, after)
	}
	if status := gitOutput(t, root, "status", "--porcelain", "--", "frontend/wailsjs"); status != "" {
		t.Errorf("--check left a diff in frontend/wailsjs:\n%s", status)
	}
}

func TestGenBindingsScriptFailsWhenABoundMethodIsMissing(t *testing.T) {
	root := newBindingsScriptRoot(t)
	fakeBin := writeFakeWails(t)

	out, code := runBindingsScript(t, root, fakeBin, "missing", "--check")
	if code == 0 {
		t.Fatalf("gen-bindings.sh exited 0 with a bound method missing\n%s", out)
	}
	if !strings.Contains(out, "ExportTo") {
		t.Errorf("failure does not name the missing method ExportTo:\n%s", out)
	}
}

// newBindingsScriptRoot builds a throwaway repository that the script can
// generate into. It carries only what the bindings guard needs: the generated
// files, bindings_test.go, and the Go API that test reflects over.
func newBindingsScriptRoot(t *testing.T) string {
	t.Helper()
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()

	for _, dir := range []string{"backend", filepath.Join("frontend", "wailsjs")} {
		if err := os.CopyFS(filepath.Join(root, dir), os.DirFS(filepath.Join(repo, dir))); err != nil {
			t.Fatalf("copying %s: %v", dir, err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum", "bindings_test.go"} {
		copyFile(t, filepath.Join(repo, name), filepath.Join(root, name))
	}
	// The script refuses a root without wails.json, the way it refuses a
	// directory that is not a repository.
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte(`{"info":{"productVersion":"0.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Generated text is not executable. Wails chmods it 0755 on every run; the
	// script normalizes it back, and the committed copy is the baseline.
	if err := filepath.WalkDir(filepath.Join(root, "frontend", "wailsjs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			return os.Chmod(path, 0o644)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	git(t, root, "init", "-q")
	git(t, root, "config", "core.fileMode", "true")
	git(t, root, "add", "-A")
	git(t, root, "-c", "user.email=gen-bindings@example.test", "-c", "user.name=gen-bindings test", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "base")
	return root
}

// writeFakeWails puts a stand-in for the Wails CLI at the front of the script's
// PATH. It reproduces the two shapes the real generator produces: whitespace
// churn on an otherwise clean file, and a file that is missing a bound method.
func writeFakeWails(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/bash
set -euo pipefail
if [ "${1:-}" != "generate" ] || [ "${2:-}" != "module" ]; then
  echo "fake wails: expected 'generate module', got: $*" >&2
  exit 2
fi
target="frontend/wailsjs/go/backend/Quotation.js"
case "${FAKE_WAILS:-churn}" in
  churn)
    awk '{ print ($0 == "" ? "\t" : $0) }' "$target" > "$target.tmp"
    mv "$target.tmp" "$target"
    chmod 755 "$target"
    ;;
  missing)
    grep -vF "['ExportTo']" "$target" > "$target.tmp"
    mv "$target.tmp" "$target"
    ;;
  *)
    echo "fake wails: unknown FAKE_WAILS mode ${FAKE_WAILS}" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "wails"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runBindingsScript(t *testing.T, root, fakeBin, mode string, args ...string) (string, int) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("scripts", "gen-bindings.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmdArgs := append([]string{script, "--root", root}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	// The guard runs `go test`. A test binary can start from a shell whose PATH
	// does not carry the Go toolchain, so put this build's toolchain first.
	path := strings.Join([]string{
		fakeBin,
		filepath.Join(runtime.GOROOT(), "bin"),
		os.Getenv("PATH"),
	}, string(os.PathListSeparator))
	cmd.Env = append(envWithout("PATH"), "PATH="+path, "FAKE_WAILS="+mode)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running gen-bindings.sh: %v\n%s", err, out)
	}
	return string(out), exitErr.ExitCode()
}

func envWithout(name string) []string {
	prefix := name + "="
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, prefix) {
			env = append(env, entry)
		}
	}
	return env
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(git(t, dir, args...))
}

// treeFingerprint is content and mode, not mtime: a restore that preserves the
// bytes but rewrites the clock is still a dry run.
func treeFingerprint(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %04o %x\n", rel, info.Mode().Perm(), sha256.Sum256(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
