package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"

	"vera-app/backend/store"
)

// maxRestoreBytes caps an uploaded dump at 512MB. Production dumps are a
// few MB; anything larger is almost certainly a mistake.
const maxRestoreBytes = 512 << 20

// restoreDumpMagic is the custom-format header pg_dump -Fc always writes.
var restoreDumpMagic = []byte("PGDMP")

// execCommand runs external tools (pg_dump/pg_restore). It is a variable so
// tests can substitute a stub; production runs the real binaries.
var execCommand = exec.Command

// pgDumpBin and pgRestoreBin name the backup tools. They are variables so
// tests can point at a version-matched client without touching PATH.
var pgDumpBin = "pg_dump"
var pgRestoreBin = "pg_restore"

// validateRestoreDump rejects non-dump uploads before the database is
// touched. Only the custom-format magic is accepted: plain-SQL dumps would
// need a different restore path and are refused explicitly.
func validateRestoreDump(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("backup file is empty")
	}
	if len(data) > maxRestoreBytes {
		return fmt.Errorf("backup file exceeds 512MB")
	}
	if !bytes.HasPrefix(data, restoreDumpMagic) {
		return fmt.Errorf("not a Postgres custom-format dump (missing PGDMP header)")
	}
	return nil
}

func (s *Server) restoreBackup(context echo.Context) error {
	me, _ := context.Get("user").(store.AuthUser)

	file, _, err := context.Request().FormFile("dump")
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "missing dump file in upload"})
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRestoreBytes+1))
	if err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "read uploaded dump"})
	}
	if err := validateRestoreDump(data); err != nil {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}

	upload, err := os.CreateTemp("", "vera-restore-*.dump")
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "stage uploaded dump"})
	}
	uploadPath := upload.Name()
	defer os.Remove(uploadPath)
	if _, err := upload.Write(data); err != nil {
		upload.Close()
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "stage uploaded dump"})
	}
	upload.Close()

	dsn := s.dsnOrEmpty()
	if dsn == "" {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "server database is not configured for restore"})
	}

	// Safety snapshot first: the current state is preserved even if the
	// restore below fails halfway.
	stamp := time.Now().UTC().Format("20060102-150405")
	snapshotName := "vera-pre-restore-" + stamp + ".dump"
	snapshotPath := filepath.Join(s.backupDirOrDefault(), snapshotName)
	if err := os.MkdirAll(s.backupDirOrDefault(), 0o755); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "prepare backup directory"})
	}
	if out, err := execCommand(pgDumpBin, "-Fc", "-f", snapshotPath, "-d", dsn).CombinedOutput(); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("safety snapshot failed, database untouched: %v (%s)", err, firstLine(out)),
		})
	}

	requestCtx := context.Request().Context()
	if err := s.database.ResetSchema(requestCtx); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("reset schema: %v (safety snapshot: %s)", err, snapshotName),
		})
	}
	if out, err := execCommand(pgRestoreBin, "--clean", "--if-exists", "--no-owner", "-d", dsn, uploadPath).CombinedOutput(); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("restore failed, previous state preserved at %s: %v (%s)", snapshotName, err, firstLine(out)),
		})
	}
	// An older dump heals forward to the current schema on restore.
	if err := s.database.Migrate(requestCtx); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("migrate after restore: %v (safety snapshot: %s)", err, snapshotName),
		})
	}
	quotations, revisions, err := s.database.RestoreRowCounts(requestCtx)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("verify after restore: %v (safety snapshot: %s)", err, snapshotName),
		})
	}

	actorID := me.ID
	details := fmt.Sprintf("safety snapshot %s; quotations=%d revisions=%d", snapshotName, quotations, revisions)
	if err := s.database.Audit(requestCtx, &actorID, "backup.restore", nil, details); err != nil {
		// The restore itself succeeded; a missing audit entry must not
		// masquerade as a restore failure, but it must be visible.
		return context.JSON(http.StatusOK, map[string]any{
			"safetySnapshot": snapshotName,
			"quotations":     quotations,
			"revisions":      revisions,
			"auditWarning":   "restore succeeded but the audit entry could not be written",
		})
	}
	return context.JSON(http.StatusOK, map[string]any{
		"safetySnapshot": snapshotName,
		"quotations":     quotations,
		"revisions":      revisions,
	})
}

func firstLine(out []byte) string {
	if i := bytes.IndexByte(out, '\n'); i >= 0 {
		return string(out[:i])
	}
	return string(out)
}
