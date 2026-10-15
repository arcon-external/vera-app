package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// releasePayload is the release contract filled in enough to exercise every
// branch of the platform lookup and the version comparison.
func releasePayload() map[string]any {
	return map[string]any{
		"latest":       "v0.4.0",
		"minSupported": "v0.2.0",
		"notesUrl":     "https://example.test/notes",
		"downloads": map[string]string{
			"windows":       "https://example.test/w.exe",
			"windows-arm64": "https://example.test/w-arm64.exe",
			"macos":         "https://example.test/m.dmg",
		},
	}
}

func releaseServer(t *testing.T, payload map[string]any, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/app/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("update check sent credentials to a public endpoint")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

func TestFetchUpdateInfo(t *testing.T) {
	full := releasePayload()
	cases := []struct {
		name          string
		payload       map[string]any
		status        int
		current       string
		goos, goarch  string
		wantAvailable bool
		wantRequired  bool
		wantDownload  string
		wantErr       bool
	}{
		{"newer on windows", full, 200, "v0.3.0", "windows", "amd64", true, false, "https://example.test/w.exe", false},
		{"newer on windows arm64", full, 200, "v0.3.0", "windows", "arm64", true, false, "https://example.test/w-arm64.exe", false},
		{"newer on mac arm", full, 200, "v0.3.0", "darwin", "arm64", true, false, "https://example.test/m.dmg", false},
		{"newer on mac intel", full, 200, "v0.3.0", "darwin", "amd64", true, false, "https://example.test/m.dmg", false},
		{"up to date", full, 200, "v0.4.0", "windows", "amd64", false, false, "https://example.test/w.exe", false},
		{"older latest", full, 200, "v0.5.0", "windows", "amd64", false, false, "https://example.test/w.exe", false},
		{"dev never updates", full, 200, "dev", "windows", "amd64", false, false, "https://example.test/w.exe", false},
		{"below minimum is required", full, 200, "v0.1.0", "windows", "amd64", true, true, "https://example.test/w.exe", false},
		{"at minimum is fine", full, 200, "v0.2.0", "windows", "amd64", true, false, "https://example.test/w.exe", false},
		{"unknown platform has no binary", full, 200, "v0.3.0", "linux", "amd64", true, false, "", false},
		{"server error", full, 500, "v0.3.0", "windows", "amd64", false, false, "", true},
		{"malformed tag is not an update", map[string]any{"latest": "soon", "downloads": map[string]string{}}, 200, "v0.3.0", "windows", "amd64", false, false, "", false},
		{"empty release", map[string]any{}, 200, "v0.3.0", "windows", "amd64", false, false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := releaseServer(t, tc.payload, tc.status)
			defer server.Close()
			// No fallback URL: this table is the API path on its own, so a
			// status of 500 stays an error rather than reaching for a second
			// source. The fallback is covered below.
			info, err := FetchUpdateInfo(server.Client(), server.URL, "", tc.current, tc.goos, tc.goarch, false)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a transport error")
				}
				return
			}
			if err != nil {
				t.Fatalf("FetchUpdateInfo failed: %v", err)
			}
			if info.UpdateAvailable != tc.wantAvailable {
				t.Errorf("UpdateAvailable = %v; want %v", info.UpdateAvailable, tc.wantAvailable)
			}
			if info.Required != tc.wantRequired {
				t.Errorf("Required = %v; want %v", info.Required, tc.wantRequired)
			}
			if info.DownloadURL != tc.wantDownload {
				t.Errorf("DownloadURL = %q; want %q", info.DownloadURL, tc.wantDownload)
			}
			if info.Current != tc.current {
				t.Errorf("Current = %q; want %q", info.Current, tc.current)
			}
		})
	}
}

func TestFetchUpdateInfoOffline(t *testing.T) {
	// Unroutable documentation address: must fail fast as an error, never hang,
	// and never touch authentication state (asserted structurally: this helper
	// takes no session and the request carries no credentials by construction).
	quick := &http.Client{Timeout: 3 * time.Second}
	_, err := FetchUpdateInfo(quick, "http://192.0.2.1:9", "", "v0.3.0", "windows", "amd64", false)
	if err == nil {
		t.Fatal("expected an offline error")
	}
}

func TestUpdateHTTPClientTimesOutFast(t *testing.T) {
	// The API check and the manifest fallback share this client, so a slow
	// second source cannot hang a startup that the first source already
	// finished. The binding's 15s discipline is pinned here rather than left
	// to whichever value happened to be written next to it.
	if updateHTTPClient.Timeout != 15*time.Second {
		t.Errorf("updateHTTPClient.Timeout = %v; want 15s", updateHTTPClient.Timeout)
	}
}

// manifestServer serves the static manifest and records the paths it is asked
// for, so a test can prove whether the fallback was read at all.
func manifestServer(t *testing.T, payload map[string]any, status int, hits chan<- string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			hits <- r.URL.Path
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

// unparseableServer answers 200 with a body that is not the release contract.
func unparseableServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>gateway fell over</html>"))
	}))
}

func TestFetchUpdateInfoFallsBackToTheStaticManifest(t *testing.T) {
	// The manifest the announce script writes to the bucket: the same keys
	// /v1/app/latest serves, one version down the wire.
	want := UpdateInfo{
		Current:         "v0.3.0",
		Latest:          "v0.4.0",
		NotesURL:        "https://example.test/notes",
		DownloadURL:     "https://example.test/w.exe",
		UpdateAvailable: true,
	}

	// Every way the API check can fail, because the outage that motivated the
	// manifest was all of them at once. The first is a server started and
	// closed again: a port nothing listens on, which is the connection
	// refused half of an outage, and it fails in microseconds rather than
	// after a dial timeout.
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	refusedURL := refused.URL
	refused.Close()
	broken := releaseServer(t, releasePayload(), http.StatusInternalServerError)
	defer broken.Close()
	unparseable := unparseableServer()
	defer unparseable.Close()

	cases := []struct {
		name   string
		apiURL string
	}{
		{"network error", refusedURL},
		{"server error", broken.URL},
		{"unparseable body", unparseable.URL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := make(chan string, 4)
			manifest := manifestServer(t, releasePayload(), http.StatusOK, hits)
			defer manifest.Close()

			info, err := FetchUpdateInfo(http.DefaultClient, tc.apiURL, manifest.URL+"/latest.json", "v0.3.0", "windows", "amd64", false)
			if err != nil {
				t.Fatalf("the API failed but the manifest answered; FetchUpdateInfo: %v", err)
			}
			if info != want {
				t.Errorf("UpdateInfo = %+v; want %+v", info, want)
			}
			if len(hits) != 1 {
				t.Fatalf("manifest fetched %d times; want exactly one read of the fallback", len(hits))
			}
			if path := <-hits; path != "/latest.json" {
				t.Errorf("manifest fetched from %q; want /latest.json, the name the announce script writes", path)
			}
		})
	}
}

func TestFetchUpdateInfoSkipsTheManifestWhenTheAPIAnswers(t *testing.T) {
	api := releaseServer(t, releasePayload(), http.StatusOK)
	defer api.Close()
	hits := make(chan string, 4)
	manifest := manifestServer(t, map[string]any{
		"latest":       "v0.9.0",
		"minSupported": "v0.7.0",
		"notesUrl":     "https://example.test/other-notes",
		"downloads":    map[string]string{"macos": "https://example.test/other.dmg"},
	}, http.StatusOK, hits)
	defer manifest.Close()

	info, err := FetchUpdateInfo(api.Client(), api.URL, manifest.URL+"/latest.json", "v0.3.0", "darwin", "arm64", false)
	if err != nil {
		t.Fatalf("FetchUpdateInfo failed: %v", err)
	}
	if info.Latest != "v0.4.0" || info.DownloadURL != "https://example.test/m.dmg" {
		t.Errorf("UpdateInfo = %+v; want the API's release, not the manifest's", info)
	}
	if len(hits) != 0 {
		t.Errorf("manifest read %d times while the API was answering; want 0", len(hits))
	}
}

func TestFetchUpdateInfoErrorsWhenBothSourcesFail(t *testing.T) {
	// The API stays the source of record: its failure is what the UI reports,
	// and the manifest is only news when it worked. Both being down must still
	// surface an error -- the login page swallows it, silently, on purpose.
	api := releaseServer(t, releasePayload(), http.StatusServiceUnavailable)
	defer api.Close()
	missing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer missing.Close()

	info, err := FetchUpdateInfo(api.Client(), api.URL, missing.URL+"/latest.json", "v0.3.0", "windows", "amd64", false)
	if err == nil {
		t.Fatal("expected an error when neither source can be read")
	}
	if !strings.Contains(err.Error(), "update service returned 503") {
		t.Errorf("error = %v; want it to name the API's failure", err)
	}
	if !strings.Contains(err.Error(), missing.URL) {
		t.Errorf("error = %v; want it to name the manifest that was unavailable too", err)
	}
	if info.Current != "v0.3.0" {
		t.Errorf("Current = %q; the binding always fills the running build", info.Current)
	}
}

// ---------------------------------------------------------------------------
// scripts/announce-release.sh -- the manifest's writer
//
// The script is the only place latest.json comes from. Its dry run prints the
// bytes it would publish without touching a bucket, so the tests below drive
// the real script with stubbed gh/gcloud/curl on PATH and read that output.
// ---------------------------------------------------------------------------

const announceScriptRelative = "scripts/announce-release.sh"

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join(".."))
	if err != nil {
		t.Fatalf("locate the repository root: %v", err)
	}
	return root
}

func readRepoFile(t *testing.T, relative string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(data)
}

const stubGH = `#!/usr/bin/env bash
# Test stub: serves one canned release, and "downloads" its assets with real
# checksums so steps 2 and 3 of the announce script run for real.
set -euo pipefail
if [ "$1" = release ] && [ "$2" = view ]; then
  tag="$3"
  assets=""
  for name in \
    "Vera-${tag}-windows-amd64-installer.exe" \
    "Vera-${tag}-windows-arm64-installer.exe" \
    "Vera-${tag}-macos-universal.dmg" \
    "Vera-CLI-${tag}-linux-amd64" \
    "Vera-CLI-${tag}-linux-arm64" \
    "Vera-CLI-${tag}-darwin-amd64" \
    "Vera-CLI-${tag}-darwin-arm64" \
    "Vera-CLI-${tag}-windows-amd64.exe" \
    "Vera-CLI-${tag}-windows-arm64.exe"; do
    assets+="{\"name\":\"$name\",\"size\":1234},"
  done
  printf '{"isDraft":false,"assets":[%s]}' "${assets%,}"
  exit 0
fi
if [ "$1" = release ] && [ "$2" = download ]; then
  dir=""
  patterns=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --dir) dir="$2"; shift 2 ;;
      --pattern) patterns+=("$2"); shift 2 ;;
      *) shift ;;
    esac
  done
  for pattern in "${patterns[@]}"; do
    base="$pattern"
    case "$pattern" in
      *.sha256) base="${pattern%.sha256}" ;;
    esac
    printf 'bytes of %s\n' "$base" > "$dir/$base"
    sha256sum "$dir/$base" | awk '{print $1}' > "$dir/$base.sha256"
  done
  exit 0
fi
echo "stub gh: unsupported: $*" >&2
exit 1
`

const stubGCloud = `#!/usr/bin/env bash
# Test stub: records every call, and answers only the "storage ls" shape the
# proof loop uses. Uploads and service updates are recorded, never performed.
set -euo pipefail
printf 'gcloud %s\n' "$*" >> "${STUB_LOG:?}"
case "$1 $2" in
  "storage ls")
    shift 2
    printf '%s\n' "$1"
    ;;
  "storage cp"|"run services"|"run describe") ;;
  *) echo "stub gcloud: unsupported: $*" >&2; exit 1 ;;
esac
`

const stubCurl = `#!/usr/bin/env bash
# Test stub: the canned /v1/app/latest body the script reads live, plus a
# status per URL for the probes.
set -euo pipefail
url=""
probe=false
for arg in "$@"; do
  case "$arg" in
    http*) url="$arg" ;;
    *'%{http_code}'*) probe=true ;;
  esac
done
printf 'curl %s\n' "$url" >> "${STUB_LOG:?}"
if [ "$probe" = true ]; then
  if [ "${STUB_MANIFEST_STATUS:-200}" != "200" ] && [ "${url##*/}" = "latest.json" ]; then
    printf '%s' "$STUB_MANIFEST_STATUS"
  else
    printf '200'
  fi
  exit 0
fi
case "$url" in
  */v1/app/latest)
    printf '%s' '{"latest":"v0.3.5","minSupported":"v0.2.0","notesUrl":"","downloads":{}}'
    ;;
esac
`

// announceDryRun runs the real announce script in dry-run mode against stubbed
// gh/gcloud/curl and returns its stdout, stderr, exit status and the log of
// every call the stubs saw.
func announceDryRun(t *testing.T, manifestStatus string) (string, string, int, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the announce script is bash; its dry run is proved on the CI runner")
	}
	root := repoRoot(t)
	stubs := t.TempDir()
	for name, body := range map[string]string{"gh": stubGH, "gcloud": stubGCloud, "curl": stubCurl} {
		path := filepath.Join(stubs, name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("write the %s stub: %v", name, err)
		}
	}
	logPath := filepath.Join(t.TempDir(), "calls.log")

	command := exec.Command("bash", filepath.Join(root, announceScriptRelative), "--version", "v9.9.9", "--dry-run")
	command.Dir = root
	command.Env = append(os.Environ(),
		"PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_LOG="+logPath,
		"STUB_MANIFEST_STATUS="+manifestStatus,
		"ANNOUNCE_PROJECT=test-project",
		"ANNOUNCE_REGION=test-region",
		"ANNOUNCE_SERVICE=test-service",
		"ANNOUNCE_BUCKET=stub-bucket",
		"ANNOUNCE_API=http://stub.invalid",
		"GITHUB_REPOSITORY=acme/vera-app",
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	// The script retries its live read ten times at five seconds, so a stub
	// that fails to answer would hang the test for a minute. Bound it.
	wait := time.AfterFunc(60*time.Second, func() { _ = command.Process.Kill() })
	err := command.Run()
	wait.Stop()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run the announce script: %v", err)
		}
		code = exitErr.ExitCode()
	}
	calls, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read the stub log: %v", readErr)
	}
	return stdout.String(), stderr.String(), code, string(calls)
}

func TestAnnounceScriptDryRunWritesTheManifest(t *testing.T) {
	stdout, stderr, code, calls := announceDryRun(t, "200")
	if code != 0 {
		t.Fatalf("announce --dry-run exited %d; stderr:\n%s", code, stderr)
	}

	// Where it would write, and with what metadata: no-cache is what makes a
	// release visible to the next update check rather than the one after a
	// TTL, and a JSON content type is what makes the fallback readable. The
	// dry run prints the very command the run executes, so this asserts the
	// upload's flags rather than a hand-copied line about them.
	wantURL := "https://storage.googleapis.com/stub-bucket/latest.json"
	if !strings.Contains(stdout, "dry run: would write "+wantURL) {
		t.Errorf("the dry run does not report writing the manifest; stdout:\n%s", stdout)
	}
	for _, flag := range []string{"--content-type=application/json", "--cache-control=no-cache"} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("the manifest upload is missing %q; a manifest served as octet-stream or cached for a TTL", flag)
		}
	}

	// What it would write: the same release the API will serve once the
	// variables are set.
	want := serverRelease{
		Downloads: map[string]string{
			"windows":       "https://storage.googleapis.com/stub-bucket/Vera-v9.9.9-windows-amd64-installer.exe",
			"windows-arm64": "https://storage.googleapis.com/stub-bucket/Vera-v9.9.9-windows-arm64-installer.exe",
			"macos":         "https://storage.googleapis.com/stub-bucket/Vera-v9.9.9-macos-universal.dmg",
		},
		Latest:       "v9.9.9",
		MinSupported: "v0.2.0",
		NotesURL:     "https://github.com/acme/vera-app/releases/tag/v9.9.9",
	}
	got := printedManifest(t, stdout)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dry run would write %+v; want %+v", got, want)
	}

	// The manifest and the variables describe the same release, or an offline
	// desktop is offered something the online one never sees.
	for _, want := range []string{
		"VERA_APP_LATEST=v9.9.9",
		"VERA_APP_MIN_SUPPORTED=v0.2.0",
		"VERA_APP_NOTES_URL=https://github.com/acme/vera-app/releases/tag/v9.9.9",
		"VERA_APP_DOWNLOAD_WINDOWS=" + want.Downloads["windows"],
		"VERA_APP_DOWNLOAD_WINDOWS_ARM64=" + want.Downloads["windows-arm64"],
		"VERA_APP_DOWNLOAD_MACOS=" + want.Downloads["macos"],
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("dry run's updater variables are missing %q; the manifest and the variables must agree", want)
		}
	}
	if strings.Contains(calls, "gcloud run services update") {
		t.Errorf("a dry run touched the service; calls:\n%s", calls)
	}

	// The manifest is inside the public-URL proof, and a manifest that does
	// not serve is exactly as broken as an installer that does not.
	if !strings.Contains(calls, "gcloud storage ls gs://stub-bucket/latest.json") {
		t.Errorf("the manifest was not listed in the proof loop; calls:\n%s", calls)
	}
	if !strings.Contains(calls, "curl https://storage.googleapis.com/stub-bucket/latest.json") {
		t.Errorf("the manifest was not proved to serve; calls:\n%s", calls)
	}
}

func TestAnnounceScriptFailsWhenTheManifestDoesNotServe(t *testing.T) {
	_, stderr, code, _ := announceDryRun(t, "404")
	if code == 0 {
		t.Fatal("the announce script exited 0 with the manifest returning 404; the job must fail")
	}
	if !strings.Contains(stderr, "latest.json") || !strings.Contains(stderr, "404") {
		t.Errorf("failure does not name the manifest: %s", stderr)
	}
}

// printedManifest pulls the JSON the dry run said it would write out of the
// script's stdout.
func printedManifest(t *testing.T, stdout string) serverRelease {
	t.Helper()
	const marker = "dry run: would write"
	at := strings.Index(stdout, marker)
	if at < 0 {
		t.Fatalf("the dry run never said it would write the manifest; stdout:\n%s", stdout)
	}
	body := stdout[at:]
	if brace := strings.Index(body, "{"); brace >= 0 {
		body = body[brace:]
	}
	var release serverRelease
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&release); err != nil {
		t.Fatalf("the printed manifest is not the release contract: %v", err)
	}
	return release
}

// TestStaticManifestIsOnTheBucketTheAnnounceScriptMirrorsTo keeps the URL the
// client reads and the bucket the announcer writes the same place. They are
// two files in two languages, which is exactly how they drift: a renamed
// bucket leaves the client reading a 404 and calling it "no update".
func TestStaticManifestIsOnTheBucketTheAnnounceScriptMirrorsTo(t *testing.T) {
	script := readRepoFile(t, announceScriptRelative)
	match := regexp.MustCompile(`(?m)^BUCKET="\$\{ANNOUNCE_BUCKET:-([^}]+)\}"$`).FindStringSubmatch(script)
	if match == nil {
		t.Fatal("the announce script declares no bucket to mirror to")
	}
	want := "https://storage.googleapis.com/" + match[1] + "/latest.json"
	if staticManifestURL != want {
		t.Errorf("staticManifestURL = %q; want %q, the bucket the announce script mirrors to", staticManifestURL, want)
	}
}

// TestAnnounceManifestMatchesServerRelease pins the keys the script publishes
// to the keys the client parses. They are two files in two languages, which is
// how they drift: a rename in one that nobody repeats in the other turns the
// fallback into an empty UpdateInfo, silently, and only when the API is down.
func TestAnnounceManifestMatchesServerRelease(t *testing.T) {
	script := readRepoFile(t, announceScriptRelative)

	program := manifestProgram(t, script)
	topLevel, nested := jqObjectKeys(program)
	if len(topLevel) == 0 {
		t.Fatalf("manifest program %q declares no keys", program)
	}

	// The contract, taken from the struct that parses the response rather
	// than from a list somebody has to remember to update.
	var tags []string
	releaseType := reflect.TypeOf(serverRelease{})
	for i := 0; i < releaseType.NumField(); i++ {
		tags = append(tags, releaseType.Field(i).Tag.Get("json"))
	}
	if !reflect.DeepEqual(sortedKeys(topLevel), sortedKeys(tags)) {
		t.Errorf("manifest declares %v; serverRelease parses %v", sortedKeys(topLevel), sortedKeys(tags))
	}
	if seen, want := len(nested["downloads"]), 3; seen != want {
		t.Errorf("downloads carries %d keys (%v); want %d, the three the platform lookup reads", seen, nested["downloads"], want)
	}
	if !reflect.DeepEqual(sortedKeys(nested["downloads"]), []string{"macos", "windows", "windows-arm64"}) {
		t.Errorf("downloads keys = %v; want the keys downloadForPlatform reads", sortedKeys(nested["downloads"]))
	}

	// Running the script's own program is the stronger check: a rename, an
	// extra key, or a value written into the wrong slot all change the bytes
	// this decodes. jq is the script's own dependency (it refuses to start
	// without it), so it is present wherever the script can run.
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("jq is not installed; the key comparison above is the guard")
	}
	dummy := func(variable string) string { return "https://example.test/" + variable }
	arguments := []string{"-n"}
	for _, binding := range manifestBindings(t, script) {
		arguments = append(arguments, "--arg", binding.name, dummy(binding.variable))
	}
	arguments = append(arguments, program)
	output, err := exec.Command(jq, arguments...).Output()
	if err != nil {
		t.Fatalf("run the manifest program: %v", err)
	}
	var got serverRelease
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("the manifest program produced %q, which is not the release contract: %v", output, err)
	}
	want := serverRelease{
		Downloads: map[string]string{
			"windows":       dummy("WINDOWS_URL"),
			"windows-arm64": dummy("WINDOWS_ARM64_URL"),
			"macos":         dummy("MACOS_URL"),
		},
		Latest:       dummy("VERSION"),
		MinSupported: dummy("LIVE_MIN"),
		NotesURL:     dummy("NOTES_URL"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("manifest program produced %+v; want %+v", got, want)
	}
}

// manifestProgram extracts the jq object literal the script builds the
// manifest with.
func manifestProgram(t *testing.T, script string) string {
	t.Helper()
	match := regexp.MustCompile(`(?m)^MANIFEST_PROGRAM='(.*)'$`).FindStringSubmatch(script)
	if match == nil {
		t.Fatal("the announce script has no MANIFEST_PROGRAM line to pin")
	}
	return match[1]
}

type manifestBinding struct {
	name     string
	variable string
}

// manifestBindings maps each variable the manifest program reads to the script
// value it is bound to, so a swap between them shows up as a wrong field
// rather than as a plausible-looking URL.
func manifestBindings(t *testing.T, script string) []manifestBinding {
	t.Helper()
	at := strings.Index(script, "MANIFEST_JSON=$(jq -n")
	if at < 0 {
		t.Fatal("the announce script builds no manifest json to inspect")
	}
	end := strings.Index(script[at:], "\"$MANIFEST_PROGRAM\")")
	if end < 0 {
		t.Fatal("the manifest jq call does not end at $MANIFEST_PROGRAM")
	}
	invocation := script[at : at+end]
	var bindings []manifestBinding
	for _, match := range regexp.MustCompile(`--arg (\w+) "\$(\w+)"`).FindAllStringSubmatch(invocation, -1) {
		bindings = append(bindings, manifestBinding{name: match[1], variable: match[2]})
	}
	if len(bindings) == 0 {
		t.Fatal("the manifest jq call binds no values")
	}
	return bindings
}

// jqObjectKeys walks a jq object literal, returning the keys declared at the
// top level and the keys of each first-level nested object. Enough of the
// grammar to compare a manifest's shape against a Go struct without running
// jq -- which is the fallback on a machine that has the script's shape but not
// its tooling.
func jqObjectKeys(program string) ([]string, map[string][]string) {
	var (
		top     []string
		nested  = map[string][]string{}
		current strings.Builder
		lastTop string
		depth   int
		inStr   bool
	)
	record := func() {
		switch {
		case depth == 1:
			lastTop = current.String()
			top = append(top, lastTop)
		case depth == 2:
			nested[lastTop] = append(nested[lastTop], current.String())
		}
	}
	for i := 0; i < len(program); i++ {
		c := program[i]
		if inStr {
			if c != '"' {
				current.WriteByte(c)
			}
			inStr = c != '"'
			continue
		}
		switch {
		case c == '"':
			current.Reset()
			inStr = true
		case c == '{':
			depth++
			current.Reset()
		case c == '}':
			depth--
			current.Reset()
		case c == ':':
			if current.Len() > 0 {
				record()
			}
			current.Reset()
		case isKeyRune(c):
			current.WriteByte(c)
		default:
			current.Reset()
		}
	}
	return top, nested
}

// isKeyRune reports whether a byte can be part of a bare jq identifier, which
// is how the manifest program spells "downloads" and "latest". Quoted keys --
// "windows-arm64" among them -- are read literally instead.
func isKeyRune(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_':
		return true
	}
	return false
}

func sortedKeys(keys []string) []string {
	out := append([]string(nil), keys...)
	sort.Strings(out)
	return out
}
