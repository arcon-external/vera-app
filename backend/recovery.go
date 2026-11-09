package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Issue #537: a local recovery snapshot covers a crash or a power loss, which
// a session fix cannot. The desktop debounces the editor's own draft payload to
// a per-user, per-endpoint file; the payload stays opaque to Go because the
// frontend owns its shape, and nothing here enters the API contract, the
// database or a signed revision.
//
// The file is a last-resort copy on the same machine, so it is written owner
// only (0600) inside an owner-only directory (0700). It is scoped twice: by the
// signed-in user id and by a hash of the API endpoint, so two accounts on one
// laptop never read each other's work and a development server never offers a
// production snapshot.

const (
	// recoveryPayloadLimit bounds what one snapshot may hold. The editor's own
	// draft payload is far smaller; the cap exists so a bad caller cannot fill
	// the user's config directory.
	recoveryPayloadLimit = 1 << 20
	recoveryDirName      = "recovery"
)

// RecoveryAPI is the frontend binding for the local recovery snapshot.
type RecoveryAPI struct {
	// dir is the snapshot directory. Empty means the real user config dir;
	// tests set a temporary directory.
	dir string
}

// NewRecoveryAPI builds the frontend recovery binding.
func NewRecoveryAPI() *RecoveryAPI {
	return &RecoveryAPI{}
}

// recoveryDir resolves the directory that holds every account's snapshot.
func (a *RecoveryAPI) recoveryDir() (string, error) {
	if a.dir != "" {
		return a.dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the config directory: %w", err)
	}
	return filepath.Join(base, "vera", recoveryDirName), nil
}

// path is the snapshot file for one user on one endpoint.
//
// The endpoint hash keeps environments apart: a desktop pointed at a local
// server must not offer a snapshot written while pointed at production. The
// base URL is read from the shared client at call time so tests (and a future
// endpoint switch) never need to rebuild this binding.
func (a *RecoveryAPI) path(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("recovery snapshot needs a signed-in user")
	}
	client, err := api()
	if err != nil {
		return "", fmt.Errorf("locate the recovery snapshot: %w", err)
	}
	base := strings.TrimSpace(client.BaseURL())
	if base == "" {
		return "", fmt.Errorf("locate the recovery snapshot: the API endpoint is not configured")
	}
	sum := sha256.Sum256([]byte(base))
	scope := hex.EncodeToString(sum[:])[:16]
	dir, err := a.recoveryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d.json", scope, userID)), nil
}

// SaveRecovery writes the snapshot atomically with owner-only permissions.
// An empty payload or a non-positive userID is refused.
//
// The write goes through a temporary file in the same directory and a rename,
// so an interrupted save cannot leave a truncated snapshot for the next launch
// to offer. An identical payload is not rewritten: the editor debounces on a
// timer, and an idle editor must not churn the disk.
func (a *RecoveryAPI) SaveRecovery(userID int64, payload string) error {
	if strings.TrimSpace(payload) == "" {
		return fmt.Errorf("recovery snapshot is empty")
	}
	if len(payload) > recoveryPayloadLimit {
		return fmt.Errorf("recovery snapshot is %d bytes; the limit is %d", len(payload), recoveryPayloadLimit)
	}
	path, err := a.path(userID)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, []byte(payload)) {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".recovery-*.tmp")
	if err != nil {
		return fmt.Errorf("create a snapshot in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.WriteString(payload); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// LoadRecovery returns the stored snapshot, or "" when there is none.
// A corrupt file is removed and reads as "" (it cannot be restored).
func (a *RecoveryAPI) LoadRecovery(userID int64) (string, error) {
	path, err := a.path(userID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	// The shape is the frontend's, but a file that is not valid JSON cannot be
	// offered for restore. Removing it now means the next launch starts clean
	// instead of prompting for work that can never be applied.
	if len(data) > recoveryPayloadLimit || !json.Valid(data) {
		_ = os.Remove(path)
		return "", nil
	}
	return string(data), nil
}

// ClearRecovery removes the snapshot; a missing file is not an error.
func (a *RecoveryAPI) ClearRecovery(userID int64) error {
	path, err := a.path(userID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
