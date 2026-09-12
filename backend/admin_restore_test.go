package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/apiclient"
)

func TestRestoreBackupUploadsSelectedDump(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	var gotFilename, gotContentType string
	var gotBytes []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotContentType = request.Header.Get("Content-Type")
		if err := request.ParseMultipartForm(600 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		file, header, err := request.FormFile("dump")
		if err != nil {
			t.Errorf("form file: %v", err)
		} else {
			defer file.Close()
			gotFilename = header.Filename
			buf := make([]byte, 64)
			n, _ := file.Read(buf)
			gotBytes = buf[:n]
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"safetySnapshot": "vera-pre-restore-test.dump",
			"quotations":     2,
			"revisions":      3,
		})
	}))
	t.Cleanup(server.Close)

	previous := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previous })
	if err := SetSessionToken("test-token"); err != nil {
		t.Fatalf("set test session: %v", err)
	}
	t.Cleanup(func() { _ = ClearSessionToken() })

	dumpPath := filepath.Join(t.TempDir(), "upload.dump")
	if err := os.WriteFile(dumpPath, []byte("PGDMPfake-dump-bytes"), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}
	previousDialog := openFileDialog
	openFileDialog = func(context.Context, runtime.OpenDialogOptions) (string, error) {
		return dumpPath, nil
	}
	t.Cleanup(func() { openFileDialog = previousDialog })

	result, err := NewAdminAPI().RestoreBackup()
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	if result.SafetySnapshot != "vera-pre-restore-test.dump" || result.Quotations != 2 || result.Revisions != 3 {
		t.Errorf("result = %+v; want snapshot + counts from server", result)
	}
	if gotFilename != "upload.dump" {
		t.Errorf("uploaded filename = %q; want upload.dump", gotFilename)
	}
	if string(gotBytes) != "PGDMPfake-dump-bytes" {
		t.Errorf("uploaded bytes = %q; want dump bytes", gotBytes)
	}
	if gotContentType == "" {
		t.Error("upload missing multipart content type")
	}
}

func TestRestoreBackupRejectsNonDumpBeforeUpload(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	previous := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previous })

	badPath := filepath.Join(t.TempDir(), "bad.dump")
	if err := os.WriteFile(badPath, []byte("plain text, not a dump"), 0o600); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	previousOpen := openFileDialog
	openFileDialog = func(context.Context, runtime.OpenDialogOptions) (string, error) {
		return badPath, nil
	}
	t.Cleanup(func() { openFileDialog = previousOpen })

	if _, err := NewAdminAPI().RestoreBackup(); err == nil {
		t.Fatal("non-dump file accepted; want PGDMP rejection")
	}
	if calls != 0 {
		t.Errorf("server hit %d times for a client-rejected file; want 0", calls)
	}
}
