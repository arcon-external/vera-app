package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vera-app/backend/auth"
)

func multipartDump(t *testing.T, fieldName, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}

func postRestore(t *testing.T, server *Server, token string, body *bytes.Buffer, contentType string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/backup/restore", body)
	request.Header.Set("Content-Type", contentType)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	var decoded map[string]any
	if len(recorder.Body.Bytes()) > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("decode restore response: %v", err)
		}
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	return recorder.Code, decoded
}

func requireBinaries(t *testing.T) {
	t.Helper()
	// pg_dump refuses to talk to a newer server, so the client must be at
	// least as new as the test database (18, matching Neon production).
	// Prefer an explicit bindir, then Homebrew, then whatever is on PATH.
	candidates := []string{}
	if dir := os.Getenv("VERA_PG_BIN_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "pg_dump"))
	}
	candidates = append(candidates,
		"/opt/homebrew/opt/postgresql@18/bin/pg_dump",
		"/opt/homebrew/opt/postgresql@17/bin/pg_dump",
		"/opt/homebrew/opt/postgresql@16/bin/pg_dump",
		"/usr/lib/postgresql/18/bin/pg_dump",
		"pg_dump",
	)
	for _, candidate := range candidates {
		if major := pgBinMajor(candidate); major >= 18 {
			pgDumpBin = candidate
			pgRestoreBin = filepath.Join(filepath.Dir(candidate), "pg_restore")
			t.Cleanup(func() { pgDumpBin = "pg_dump"; pgRestoreBin = "pg_restore" })
			return
		}
	}
	t.Skip("no pg_dump >= 16 found; set VERA_PG_BIN_DIR to run the restore drill locally")
}

// pgBinMajor reports the major version of a pg binary, or -1 if it cannot
// be executed.
func pgBinMajor(path string) int {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return -1
	}
	var major int
	if _, err := fmt.Sscanf(string(out), "pg_dump (PostgreSQL) %d.", &major); err != nil {
		return -1
	}
	return major
}

func TestRestoreRejectsNonDump(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_restore_reject")
	server.SetBackupDir(t.TempDir())
	server.SetDSN(database.DSN())

	beforeQ, beforeR, err := database.RestoreRowCounts(context.Background())
	if err != nil {
		t.Fatalf("row counts before: %v", err)
	}

	body, contentType := multipartDump(t, "dump", "evil.txt", []byte("not a database dump"))
	code, response := postRestore(t, server, token, body, contentType)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("non-dump upload = %d; want 422", code)
	}
	if message, _ := response["error"].(string); !strings.Contains(message, "PGDMP") {
		t.Errorf("error = %q; want PGDMP explanation", message)
	}

	afterQ, afterR, err := database.RestoreRowCounts(context.Background())
	if err != nil {
		t.Fatalf("row counts after: %v", err)
	}
	if afterQ != beforeQ || afterR != beforeR {
		t.Errorf("row counts changed on rejected upload: (%d,%d) -> (%d,%d)", beforeQ, beforeR, afterQ, afterR)
	}
}

func TestRestoreRequiresAdmin(t *testing.T) {
	server, database, _ := adminTestSetup(t, "vera_api_restore_auth")
	seedStaff(t, database, "restore-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "restore-staff@example.com", "staff-password")

	body, contentType := multipartDump(t, "dump", "x.dump", []byte("PGDMPfake"))
	if code, _ := postRestore(t, server, staffToken, body, contentType); code != http.StatusForbidden {
		t.Fatalf("staff restore = %d; want 403", code)
	}
	body, contentType = multipartDump(t, "dump", "x.dump", []byte("PGDMPfake"))
	if code, _ := postRestore(t, server, "", body, contentType); code != http.StatusUnauthorized {
		t.Fatalf("anonymous restore = %d; want 401", code)
	}
}

func TestRestoreSnapshotFailureLeavesDBUntouched(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_restore_snapfail")
	server.SetBackupDir(t.TempDir())
	server.SetDSN(database.DSN())

	previous := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo snapshot exploded >&2; exit 1")
	}
	t.Cleanup(func() { execCommand = previous })

	beforeQ, _, err := database.RestoreRowCounts(context.Background())
	if err != nil {
		t.Fatalf("row counts before: %v", err)
	}
	body, contentType := multipartDump(t, "dump", "x.dump", []byte("PGDMPfake-but-valid-magic"))
	code, response := postRestore(t, server, token, body, contentType)
	if code != http.StatusInternalServerError {
		t.Fatalf("snapshot failure = %d; want 500", code)
	}
	if message, _ := response["error"].(string); !strings.Contains(message, "untouched") {
		t.Errorf("error = %q; want untouched guarantee", message)
	}
	afterQ, _, err := database.RestoreRowCounts(context.Background())
	if err != nil {
		t.Fatalf("row counts after: %v", err)
	}
	if afterQ != beforeQ {
		t.Error("row counts changed though the safety snapshot failed")
	}
}

func TestRestoreHappyPathRollsBackMarkerRow(t *testing.T) {
	requireBinaries(t)
	server, database, token := adminTestSetup(t, "vera_api_restore_happy")
	backupDir := t.TempDir()
	server.SetBackupDir(backupDir)
	server.SetDSN(database.DSN())
	ctx := context.Background()

	// A genuine dump of the current test database.
	dumpPath := filepath.Join(t.TempDir(), "source.dump")
	if out, err := exec.Command(pgDumpBin, "-Fc", "-f", dumpPath, "-d", database.DSN()).CombinedOutput(); err != nil {
		t.Fatalf("pg_dump source: %v (%s)", err, firstLine(out))
	}
	dumpBytes, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read source dump: %v", err)
	}

	// Marker row created AFTER the dump: a successful restore must remove it.
	hash, err := auth.HashPassword("marker-password")
	if err != nil {
		t.Fatalf("hash marker password: %v", err)
	}
	if _, err := database.CreateUser(ctx, "restore-marker@example.com", "Marker", hash, "staff"); err != nil {
		t.Fatalf("create marker user: %v", err)
	}

	body, contentType := multipartDump(t, "dump", "restore.dump", dumpBytes)
	code, response := postRestore(t, server, token, body, contentType)
	if code != http.StatusOK {
		t.Fatalf("restore = %d; want 200 (%v)", code, response["error"])
	}
	snapshot, _ := response["safetySnapshot"].(string)
	if snapshot == "" {
		t.Fatal("response has no safetySnapshot")
	}
	if _, err := os.Stat(filepath.Join(backupDir, snapshot)); err != nil {
		t.Errorf("safety snapshot %q missing from backup dir: %v", snapshot, err)
	}

	if _, err := database.GetUserByEmail(ctx, "restore-marker@example.com"); err == nil {
		t.Error("marker user survived the restore; want it rolled back")
	}
	count, err := database.AuditCount(ctx, "backup.restore")
	if err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if count != 1 {
		t.Errorf("backup.restore audit entries = %d; want 1", count)
	}
	if _, present := response["quotations"]; !present {
		t.Error("response has no quotations count")
	}
	if _, present := response["revisions"]; !present {
		t.Error("response has no revisions count")
	}
}

// fakeBackupSink records snapshot uploads in memory, optionally failing.
type fakeBackupSink struct {
	files map[string][]byte
	err   error
}

func (f *fakeBackupSink) put(ctx context.Context, name string, data []byte) error {
	if f.err != nil {
		return f.err
	}
	if f.files == nil {
		f.files = make(map[string][]byte)
	}
	f.files[name] = append([]byte(nil), data...)
	return nil
}

// stubDumpTools makes pg_dump write canned bytes to its -f target while
// every other tool invocation exits 0 without touching anything.
func stubDumpTools(t *testing.T, snapshotBytes string) {
	t.Helper()
	previous := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		for i, arg := range args {
			if arg == "-f" && i+1 < len(args) {
				path := args[i+1]
				return exec.Command("sh", "-c", "printf '%s' '"+snapshotBytes+"' > \""+path+"\"")
			}
		}
		return exec.Command("sh", "-c", "exit 0")
	}
	t.Cleanup(func() { execCommand = previous })
}

// stubRestoreWarning makes pg_dump run for real while pg_restore is routed
// through a wrapper that runs the real binary and then exits 1 with a
// benign warning on stderr. It reproduces the production incident where a
// non-superuser role is denied a privilege statement though all data lands.
func stubRestoreWarning(t *testing.T, realRestore string) {
	t.Helper()
	wrapper := "#!/bin/sh\n" +
		"REAL=\"$1\"\n" +
		"shift\n" +
		"\"$REAL\" \"$@\"\n" +
		"status=$?\n" +
		"if [ $status -eq 0 ]; then\n" +
		"  echo 'pg_restore: WARNING: permission denied to change owner of SCHEMA public' >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		"exit $status\n"
	wrapperPath := filepath.Join(t.TempDir(), "pg_restore_warn.sh")
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write restore wrapper: %v", err)
	}
	previous := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		if filepath.Base(name) == filepath.Base(realRestore) {
			return exec.Command(wrapperPath, append([]string{realRestore}, args...)...)
		}
		return exec.Command(name, args...)
	}
	t.Cleanup(func() { execCommand = previous })
}

func TestRestoreBenignWarningStillSucceeds(t *testing.T) {
	requireBinaries(t)
	server, database, token := adminTestSetup(t, "vera_api_restore_warn")
	server.SetBackupDir(t.TempDir())
	server.SetDSN(database.DSN())
	ctx := context.Background()

	// A genuine dump of the current test database.
	dumpPath := filepath.Join(t.TempDir(), "source.dump")
	if out, err := exec.Command(pgDumpBin, "-Fc", "-f", dumpPath, "-d", database.DSN()).CombinedOutput(); err != nil {
		t.Fatalf("pg_dump source: %v (%s)", err, firstLine(out))
	}
	dumpBytes, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read source dump: %v", err)
	}

	// Marker row created AFTER the dump: a successful restore must remove it.
	hash, err := auth.HashPassword("marker-password")
	if err != nil {
		t.Fatalf("hash marker password: %v", err)
	}
	if _, err := database.CreateUser(ctx, "restore-warn-marker@example.com", "Marker", hash, "staff"); err != nil {
		t.Fatalf("create marker user: %v", err)
	}

	stubRestoreWarning(t, pgRestoreBin)

	body, contentType := multipartDump(t, "dump", "restore.dump", dumpBytes)
	code, response := postRestore(t, server, token, body, contentType)
	if code != http.StatusOK {
		t.Fatalf("restore with benign warning = %d; want 200 (%v)", code, response["error"])
	}
	warnings, _ := response["restoreWarnings"].(string)
	if !strings.Contains(warnings, "permission denied") {
		t.Errorf("restoreWarnings = %q; want the pg_restore warning", warnings)
	}
	if _, present := response["quotations"]; !present {
		t.Error("response has no quotations count")
	}
	// The bootstrap admin comes back with the dump: the data restore
	// really ran, this is not an empty-schema false success.
	if _, err := database.GetUserByEmail(ctx, "api-admin@example.com"); err != nil {
		t.Error("bootstrap admin missing after restore; want the dumped data back")
	}
	if _, err := database.GetUserByEmail(ctx, "restore-warn-marker@example.com"); err == nil {
		t.Error("marker user survived the restore; want it rolled back")
	}
	count, err := database.AuditCount(ctx, "backup.restore")
	if err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if count != 1 {
		t.Errorf("backup.restore audit entries = %d; want 1", count)
	}
}

func TestRestoreOutputTailCapsAndKeepsFailingStatement(t *testing.T) {
	var noisy strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&noisy, "pg_restore: line %03d of chatter\n", i)
	}
	noisy.WriteString("pg_restore: error: could not execute query: ERROR: permission denied to change owner of SCHEMA public\n")
	noisy.WriteString("Command was: ALTER SCHEMA public OWNER TO postgres;\n")

	tail := restoreOutputTail([]byte(noisy.String()))
	if len(tail) > 2100 {
		t.Errorf("tail is %d bytes; want capped near 2000", len(tail))
	}
	if !strings.Contains(tail, "Command was: ALTER SCHEMA public OWNER TO postgres;") {
		t.Errorf("tail = %q; want the failing statement to survive", tail)
	}
	if strings.Contains(tail, "line 000 of chatter") {
		t.Errorf("tail keeps ancient chatter; want only the tail")
	}
	if got := restoreOutputTail(nil); got != "" {
		t.Errorf("empty output tail = %q; want empty", got)
	}
}

func TestRestoreSnapshotUploadsToSink(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_restore_sink")
	backupDir := t.TempDir()
	server.SetBackupDir(backupDir)
	server.SetDSN(database.DSN())
	sink := &fakeBackupSink{}
	server.SetBackupSink(sink)
	stubDumpTools(t, "PGDMPsnapshot-bytes")

	body, contentType := multipartDump(t, "dump", "x.dump", []byte("PGDMPfake-but-valid-magic"))
	code, response := postRestore(t, server, token, body, contentType)
	if code != http.StatusOK {
		t.Fatalf("restore = %d; want 200 (%v)", code, response["error"])
	}
	snapshot, _ := response["safetySnapshot"].(string)
	if snapshot == "" || !strings.HasPrefix(snapshot, "vera-pre-restore-") {
		t.Fatalf("safetySnapshot = %q; want the snapshot name", snapshot)
	}
	got, ok := sink.files[snapshot]
	if !ok {
		t.Fatalf("snapshot %q never reached the sink", snapshot)
	}
	if string(got) != "PGDMPsnapshot-bytes" {
		t.Errorf("uploaded bytes = %q; want the pg_dump output", got)
	}
	// The staging file is dropped after a successful upload: the bucket
	// copy is the snapshot now.
	if entries, _ := os.ReadDir(backupDir); len(entries) != 0 {
		t.Errorf("backup dir holds %d files after upload; want empty", len(entries))
	}
}

func TestRestoreSnapshotUploadFailureLeavesDBUntouched(t *testing.T) {
	server, database, token := adminTestSetup(t, "vera_api_restore_sinkfail")
	server.SetBackupDir(t.TempDir())
	server.SetDSN(database.DSN())
	server.SetBackupSink(&fakeBackupSink{err: errors.New("bucket down")})
	stubDumpTools(t, "PGDMPsnapshot-bytes")

	beforeQ, beforeR, err := database.RestoreRowCounts(context.Background())
	if err != nil {
		t.Fatalf("row counts before: %v", err)
	}
	body, contentType := multipartDump(t, "dump", "x.dump", []byte("PGDMPfake-but-valid-magic"))
	code, response := postRestore(t, server, token, body, contentType)
	if code != http.StatusInternalServerError {
		t.Fatalf("upload failure = %d; want 500", code)
	}
	if message, _ := response["error"].(string); !strings.Contains(message, "untouched") {
		t.Errorf("error = %q; want untouched guarantee", message)
	}
	afterQ, afterR, err := database.RestoreRowCounts(context.Background())
	if err != nil {
		t.Fatalf("row counts after: %v", err)
	}
	if afterQ != beforeQ || afterR != beforeR {
		t.Errorf("row counts changed on upload failure: (%d,%d) -> (%d,%d)", beforeQ, beforeR, afterQ, afterR)
	}
}
