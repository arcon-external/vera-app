package extract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Issue #477, wave 2: the production isolation boundary.
//
// A child process with a clean environment is not isolation. A same-UID child
// can read /proc/<server-pid>/environ and recover the DSN or the signing key,
// so running `vextract` inside the server container is not a supported
// production path. The dedicated restricted image (no network, read-only root,
// dropped capabilities, separate memory/PID limits) is the reviewed
// configuration, and the server image must not carry the extractor or Poppler
// at all.

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestServerImageShipsNoInProcessExtractor(t *testing.T) {
	dockerfile := repoFile(t, "Dockerfile.veraserver")
	for _, forbidden := range []string{"poppler-utils", "cmd/vextract", "/vextract"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Errorf("Dockerfile.veraserver still contains %q; the server image must not ship an in-process extractor", forbidden)
		}
	}
	if !strings.Contains(dockerfile, "/veraserver") {
		t.Error("Dockerfile.veraserver no longer builds the server binary")
	}
}

func TestExtractorIsConfiguredExplicitly(t *testing.T) {
	t.Setenv("VERA_EXTRACTOR_CMD", "")
	if engine := NewCommandEngineFromEnvironment(); engine != nil {
		t.Fatalf("NewCommandEngineFromEnvironment() = %+v without VERA_EXTRACTOR_CMD; want nil (extraction disabled)", engine)
	}

	t.Setenv("VERA_EXTRACTOR_CMD", "  /opt/vera/vextract-docker.sh --quiet  ")
	configured := NewCommandEngineFromEnvironment()
	if configured == nil {
		t.Fatal("an explicit VERA_EXTRACTOR_CMD did not configure the engine")
	}
	engine, ok := configured.(*CommandEngine)
	if !ok {
		t.Fatalf("configured engine = %T; want *CommandEngine", configured)
	}
	if len(engine.Command) != 2 || engine.Command[0] != "/opt/vera/vextract-docker.sh" || engine.Command[1] != "--quiet" {
		t.Errorf("engine command = %v; want the configured wrapper and flag", engine.Command)
	}
}

func TestIsolatedWrapperKeepsItsRestrictions(t *testing.T) {
	wrapper := repoFile(t, "scripts", "vextract-docker.sh")
	for _, restriction := range []string{
		"--network none",
		"--read-only",
		"--cap-drop ALL",
		"--security-opt no-new-privileges",
		"--pids-limit",
		"--memory",
	} {
		if !strings.Contains(wrapper, restriction) {
			t.Errorf("scripts/vextract-docker.sh no longer carries %q", restriction)
		}
	}
}

func TestExtractorImagePinsTheReviewedPopplerPackage(t *testing.T) {
	dockerfile := repoFile(t, "docker", "po-extract", "Dockerfile")
	pinned := regexp.MustCompile(`poppler-utils=\d+\.\d+\.\d+-[0-9A-Za-z.+~]+`).FindString(dockerfile)
	if pinned == "" {
		t.Fatal("docker/po-extract/Dockerfile installs poppler-utils without an exact package version")
	}
	if !strings.Contains(dockerfile, "LICENSE REVIEW BASIS") {
		t.Error("docker/po-extract/Dockerfile no longer records the license review basis")
	}
	if !strings.Contains(dockerfile, "source") {
		t.Error("docker/po-extract/Dockerfile no longer records the source-offer note")
	}
	if got := strings.Count(dockerfile, "@sha256:"); got < 2 {
		t.Errorf("base image digest pins = %d; want both bases pinned", got)
	}
}
