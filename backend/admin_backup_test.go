package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vera-app/backend/apiclient"
)

func TestDownloadBackupWritesServerBytes(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	payload := []byte("fake-dump-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/admin/backup/latest" {
			t.Errorf("path = %s; want /v1/admin/backup/latest", request.URL.Path)
		}
		writer.Header().Set("Content-Disposition", `attachment; filename="vera-test.dump"`)
		_, _ = writer.Write(payload)
	}))
	t.Cleanup(server.Close)

	previous := apiClient
	ConfigureAPI(apiclient.New(server.URL, SessionToken))
	t.Cleanup(func() { apiClient = previous })
	if err := SetSessionToken("test-token"); err != nil {
		t.Fatalf("set test session: %v", err)
	}
	t.Cleanup(func() { _ = ClearSessionToken() })

	target := filepath.Join(t.TempDir(), "saved.dump")
	previousDialog := saveFileDialog
	saveFileDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) {
		return target, nil
	}
	t.Cleanup(func() { saveFileDialog = previousDialog })

	written, err := NewAdminAPI().DownloadBackup()
	if err != nil {
		t.Fatalf("DownloadBackup failed: %v", err)
	}
	if written != target {
		t.Errorf("written path = %q; want %q", written, target)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(content) != string(payload) {
		t.Error("written bytes differ from server bytes")
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v; want 0600", info.Mode())
	}
}
