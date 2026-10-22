package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDevAPILauncherPassesLocalDevelopmentFlag is the executable wiring guard
// for the reported repro: the helper must ask the server to enforce the local
// target policy. A fake `go` records its arguments, so no build, server, or
// database runs -- and the test fails if the flag is dropped from the script.
func TestDevAPILauncherPassesLocalDevelopmentFlag(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	script, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "dev-api.sh"))
	if err != nil {
		t.Fatalf("read launcher: %v", err)
	}

	// Hermetic copy: the real script is executed byte-for-byte, but from a
	// temporary tree so a developer's ignored .env cannot influence the test.
	tree := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, "scripts"), 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	launcher := filepath.Join(tree, "scripts", "dev-api.sh")
	if err := os.WriteFile(launcher, script, 0o755); err != nil {
		t.Fatalf("write launcher copy: %v", err)
	}

	bin := filepath.Join(tree, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("create fake bin: %v", err)
	}
	argsLog := filepath.Join(tree, "go-args.txt")
	fakeGo := filepath.Join(bin, "go")
	fake := "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" > \"$FAKE_GO_LOG\"\nprintf 'MOCK_GO_REACHED\\n'\n"
	if err := os.WriteFile(fakeGo, []byte(fake), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	cmd := exec.Command("bash", launcher)
	cmd.Dir = tree
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GO_LOG="+argsLog,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("launcher failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "MOCK_GO_REACHED") {
		t.Fatalf("fake go was not reached; output:\n%s", output)
	}

	recorded, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read fake go arguments: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	for _, want := range []string{"run", "./cmd/veraserver", "--local-development"} {
		found := false
		for _, arg := range args {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("launcher did not pass %q; args = %q", want, args)
		}
	}
}
