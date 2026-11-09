package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDevAPILauncherWiresFlagLoopbackAndEnv is the executable wiring guard for
// the reported repro. A fake `go` records its arguments and the environment the
// server would inherit, so no build, server, or database runs. The test fails
// if the guard flag is dropped, the loopback bind stops being forced, or a
// server-owned .env stops being sourced.
func TestDevAPILauncherWiresFlagLoopbackAndEnv(t *testing.T) {
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
	envFixture := "VERA_API_ADDR=0.0.0.0:9999\nVERA_DEV_API_LAUNCHER_MARKER=loaded-from-env\n"
	if err := os.WriteFile(filepath.Join(tree, ".env"), []byte(envFixture), 0o644); err != nil {
		t.Fatalf("write .env fixture: %v", err)
	}

	bin := filepath.Join(tree, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("create fake bin: %v", err)
	}
	argsLog := filepath.Join(tree, "go-args.txt")
	fakeGo := filepath.Join(bin, "go")
	fake := "#!/usr/bin/env bash\n" +
		"{\n" +
		"  printf 'ARG\\t%s\\n' \"$@\"\n" +
		"  printf 'ENV\\tVERA_API_ADDR\\t%s\\n' \"${VERA_API_ADDR:-}\"\n" +
		"  printf 'ENV\\tVERA_DEV_API_LAUNCHER_MARKER\\t%s\\n' \"${VERA_DEV_API_LAUNCHER_MARKER:-}\"\n" +
		"} > \"$FAKE_GO_LOG\"\n" +
		"printf 'MOCK_GO_REACHED\\n'\n"
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
	var args []string
	env := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(recorded)), "\n") {
		fields := strings.Split(line, "\t")
		switch {
		case len(fields) >= 2 && fields[0] == "ARG":
			args = append(args, fields[1])
		case len(fields) == 3 && fields[0] == "ENV":
			env[fields[1]] = fields[2]
		}
	}
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
	if got := env["VERA_API_ADDR"]; got != "127.0.0.1:8081" {
		t.Fatalf("VERA_API_ADDR = %q; launcher must force 127.0.0.1:8081", got)
	}
	if got := env["VERA_DEV_API_LAUNCHER_MARKER"]; got != "loaded-from-env" {
		t.Fatalf("server-owned .env marker = %q; launcher must source .env", got)
	}
}
