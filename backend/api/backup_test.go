package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeDump(t *testing.T, dir, name, content string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func TestDownloadLatestBackupStreamsNewestDump(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_backup_dl")

	dir := t.TempDir()
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	writeDump(t, dir, "vera-20240101-000000.dump", "older-dump-bytes", older)
	writeDump(t, dir, "vera-20240201-000000.dump", "newer-dump-bytes-longer", newer)
	server.SetBackupDir(dir)

	request := httptest.NewRequest(http.MethodGet, "/v1/admin/backup/latest", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("download = %d; want 200", recorder.Code)
	}
	body, err := io.ReadAll(recorder.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "newer-dump-bytes-longer" {
		t.Errorf("downloaded %q; want the newest dump", firstBytes(body))
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, "vera-20240201-000000.dump") {
		t.Errorf("Content-Disposition = %q; want newest filename", disposition)
	}
}

func TestDownloadBackupRequiresAdmin(t *testing.T) {
	server, database, _ := adminTestSetup(t, "vera_api_backup_auth")
	seedStaff(t, database, "backup-staff@example.com", "staff-password")
	staffToken := doLogin(t, server, "backup-staff@example.com", "staff-password")

	if code, _ := doAdminRequest(t, server, staffToken, http.MethodGet, "/v1/admin/backup/latest", ""); code != http.StatusForbidden {
		t.Fatalf("staff download = %d; want 403", code)
	}
	if code, _ := doAdminRequest(t, server, "", http.MethodGet, "/v1/admin/backup/latest", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous download = %d; want 401", code)
	}
}

func TestDownloadBackupEmptyDirIs404(t *testing.T) {
	server, _, token := adminTestSetup(t, "vera_api_backup_empty")
	server.SetBackupDir(t.TempDir())

	code, body := doAdminRequest(t, server, token, http.MethodGet, "/v1/admin/backup/latest", "")
	if code != http.StatusNotFound {
		t.Fatalf("empty backup dir = %d; want 404", code)
	}
	if message, _ := body["error"].(string); message == "" {
		t.Error("empty backup dir should explain that no backups are available")
	}
}

func firstBytes(body []byte) string {
	if len(body) > 24 {
		return string(body[:24]) + "…"
	}
	return string(body)
}
