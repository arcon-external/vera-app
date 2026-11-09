package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vera-app/backend/apiclient"
)

// Issue #537: the local recovery snapshot is device-local, per user and per
// API endpoint. These tests own the storage contract: a crash must leave the
// last debounced save readable, a different account or a different server must
// never read it, and a corrupt file must not be offered for restore.

func newRecoveryFixture(t *testing.T) (*RecoveryAPI, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "recovery")
	previous := apiClient
	apiClient = apiclient.New("https://api.example.test", func() string { return "" })
	t.Cleanup(func() { apiClient = previous })
	return &RecoveryAPI{dir: dir}, dir
}

func onlyRecoveryFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read recovery dir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("recovery dir holds %d entries %v; want exactly the snapshot", len(entries), names)
	}
	return filepath.Join(dir, entries[0].Name())
}

const recoveryPayload = `{"version":1,"savedAt":"2026-11-09T14:14:00Z","draft":{"items":[],"details":{},"customerId":2}}`

func TestRecoverySnapshotRoundTrip(t *testing.T) {
	recovery, _ := newRecoveryFixture(t)

	if err := recovery.SaveRecovery(7, recoveryPayload); err != nil {
		t.Fatalf("SaveRecovery failed: %v", err)
	}
	got, err := recovery.LoadRecovery(7)
	if err != nil {
		t.Fatalf("LoadRecovery failed: %v", err)
	}
	if got != recoveryPayload {
		t.Errorf("LoadRecovery = %q; want the payload verbatim", got)
	}

	if err := recovery.ClearRecovery(7); err != nil {
		t.Fatalf("ClearRecovery failed: %v", err)
	}
	got, err = recovery.LoadRecovery(7)
	if err != nil {
		t.Fatalf("LoadRecovery after clear failed: %v", err)
	}
	if got != "" {
		t.Errorf("LoadRecovery after clear = %q; want empty", got)
	}
	// Signing out twice, or clearing an already-cleared snapshot, is success.
	if err := recovery.ClearRecovery(7); err != nil {
		t.Errorf("ClearRecovery on a missing file = %v; want nil", err)
	}
}

func TestRecoverySnapshotIsOwnerOnly(t *testing.T) {
	recovery, dir := newRecoveryFixture(t)
	if err := recovery.SaveRecovery(7, recoveryPayload); err != nil {
		t.Fatalf("SaveRecovery failed: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat recovery dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("recovery dir mode = %o; want 700", perm)
	}

	path := onlyRecoveryFile(t, dir)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("snapshot mode = %o; want 600", perm)
	}
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("snapshot name = %q; want a .json suffix", path)
	}
	// Atomic write leaves no temporary file behind.
	if strings.Contains(filepath.Base(path), "tmp") {
		t.Errorf("snapshot name = %q; a temp file survived the rename", path)
	}
}

func TestRecoverySnapshotDoesNotRewriteIdenticalBytes(t *testing.T) {
	recovery, dir := newRecoveryFixture(t)
	if err := recovery.SaveRecovery(7, recoveryPayload); err != nil {
		t.Fatalf("SaveRecovery failed: %v", err)
	}
	path := onlyRecoveryFile(t, dir)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	if err := recovery.SaveRecovery(7, recoveryPayload); err != nil {
		t.Fatalf("second SaveRecovery failed: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	// A rename over the existing file would change the inode; an unchanged
	// inode is the proof that an idle debounce wrote nothing.
	if !os.SameFile(before, after) {
		t.Error("an identical payload replaced the snapshot; want the write skipped")
	}
}

func TestRecoverySnapshotRefusesBadInput(t *testing.T) {
	recovery, dir := newRecoveryFixture(t)

	for _, userID := range []int64{0, -1} {
		if err := recovery.SaveRecovery(userID, recoveryPayload); err == nil {
			t.Errorf("SaveRecovery(%d) = nil; want an error", userID)
		}
	}
	if err := recovery.SaveRecovery(7, ""); err == nil {
		t.Error("SaveRecovery with an empty payload = nil; want an error")
	}
	if err := recovery.SaveRecovery(7, strings.Repeat("a", (1<<20)+1)); err == nil {
		t.Error("SaveRecovery over 1 MiB = nil; want an error")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("a refused save created %s; want nothing written", dir)
	}
}

func TestRecoverySnapshotIsScopedToTheUser(t *testing.T) {
	recovery, dir := newRecoveryFixture(t)
	if err := recovery.SaveRecovery(7, recoveryPayload); err != nil {
		t.Fatalf("SaveRecovery failed: %v", err)
	}

	got, err := recovery.LoadRecovery(8)
	if err != nil {
		t.Fatalf("LoadRecovery for another user failed: %v", err)
	}
	if got != "" {
		t.Errorf("another user read %q; want empty", got)
	}
	// Clearing one account must not remove another account's snapshot.
	if err := recovery.ClearRecovery(8); err != nil {
		t.Fatalf("ClearRecovery for another user failed: %v", err)
	}
	got, err = recovery.LoadRecovery(7)
	if err != nil || got != recoveryPayload {
		t.Errorf("owner's snapshot after another user cleared = %q, %v; want it intact", got, err)
	}
	if path := onlyRecoveryFile(t, dir); path == "" {
		t.Fatal("owner's snapshot file disappeared")
	}
}

func TestRecoverySnapshotIsScopedToTheAPIEndpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "recovery")
	previous := apiClient
	t.Cleanup(func() { apiClient = previous })

	apiClient = apiclient.New("https://one.example.test", nil)
	recovery := &RecoveryAPI{dir: dir}
	if err := recovery.SaveRecovery(7, recoveryPayload); err != nil {
		t.Fatalf("SaveRecovery failed: %v", err)
	}

	apiClient = apiclient.New("https://two.example.test", nil)
	got, err := recovery.LoadRecovery(7)
	if err != nil {
		t.Fatalf("LoadRecovery against another endpoint failed: %v", err)
	}
	if got != "" {
		t.Errorf("another API endpoint read %q; want empty", got)
	}

	// A trailing slash is the same endpoint, not a different one.
	apiClient = apiclient.New("https://one.example.test/", nil)
	got, err = recovery.LoadRecovery(7)
	if err != nil || got != recoveryPayload {
		t.Errorf("the same endpoint with a trailing slash read %q, %v; want the payload", got, err)
	}
}

func TestRecoverySnapshotCorruptFileIsRemoved(t *testing.T) {
	recovery, dir := newRecoveryFixture(t)
	if err := recovery.SaveRecovery(7, recoveryPayload); err != nil {
		t.Fatalf("SaveRecovery failed: %v", err)
	}
	path := onlyRecoveryFile(t, dir)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("corrupt the snapshot: %v", err)
	}

	got, err := recovery.LoadRecovery(7)
	if err != nil {
		t.Fatalf("LoadRecovery on a corrupt file failed: %v", err)
	}
	if got != "" {
		t.Errorf("LoadRecovery on a corrupt file = %q; want empty", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the corrupt snapshot still exists; want it removed (%v)", err)
	}
	// The next read is the ordinary missing-file case.
	got, err = recovery.LoadRecovery(7)
	if err != nil || got != "" {
		t.Errorf("LoadRecovery after removing corruption = %q, %v; want empty, nil", got, err)
	}
}

func TestRecoverySnapshotMissingFileIsNotAnError(t *testing.T) {
	recovery, _ := newRecoveryFixture(t)
	got, err := recovery.LoadRecovery(7)
	if err != nil {
		t.Fatalf("LoadRecovery with no snapshot = %v; want nil", err)
	}
	if got != "" {
		t.Errorf("LoadRecovery with no snapshot = %q; want empty", got)
	}
}

func TestRecoverySnapshotNeedsAConfiguredAPI(t *testing.T) {
	previous := apiClient
	apiClient = nil
	t.Cleanup(func() { apiClient = previous })

	recovery := &RecoveryAPI{dir: t.TempDir()}
	if err := recovery.SaveRecovery(7, recoveryPayload); err == nil {
		t.Error("SaveRecovery without a configured API = nil; want a clear error")
	} else if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("SaveRecovery error = %q; want it to name the missing configuration", err)
	}
	if _, err := recovery.LoadRecovery(7); err == nil {
		t.Error("LoadRecovery without a configured API = nil; want a clear error")
	}
}
