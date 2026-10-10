package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// backupCommands handle the server's own database dump.
//
// These are the only commands that can destroy data the company cannot re-create
// by hand, so they are the only ones that ask twice: `restore` refuses to run
// without an explicit --yes even in a terminal, and the server takes a safety
// snapshot before it starts.
func backupCommands() *command {
	return commandGroup("backup", "The server's database dump (admin).",
		backupDownloadCommand(),
		backupRestoreCommand(),
	)
}

func backupDownloadCommand() *command {
	var out string
	return &command{
		name:    "download",
		summary: "Download the most recent database dump.",
		usage:   "vera backup download [--out backups/vera.dump]",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&out, "out", "", "where to write it (default: the name the server reports, in this directory)")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			filename, data, err := client.Download("/v1/admin/backup/latest", "vera-backup.dump")
			if err != nil {
				// The route answers 404 when nothing has been taken yet, which
				// is a normal state on a young deployment rather than a fault.
				if strings.Contains(err.Error(), "no backups available") {
					return fmt.Errorf("the server has no backups yet")
				}
				return err
			}
			destination := out
			if destination == "" {
				destination = filename
			}
			if err := writeFile(destination, data); err != nil {
				return err
			}
			if err := a.emitJSON(map[string]any{
				"out": destination, "bytes": len(data), "serverFilename": filename,
			}); err != nil {
				return err
			}
			a.line("Wrote %s (%s).", destination, humanBytes(int64(len(data))))
			a.note("This is a Postgres custom-format dump: restore it with `vera backup restore --file %s`.", destination)
			return nil
		},
	}
}

func backupRestoreCommand() *command {
	var file string
	var force bool
	return &command{
		name:    "restore",
		summary: "Replace the server's database with a dump. Destructive, and deliberately awkward.",
		usage:   "vera backup restore --file backup.dump --yes",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&file, "file", "", "the dump to upload (required)")
			fs.BoolVar(&force, "force", false, "also proceed when the dump looks old or small")
		},
		run: func(a *app) error {
			if err := a.requireArgs(0, "no arguments"); err != nil {
				return err
			}
			if file == "" {
				return usagef(a.cmd, "--file is required")
			}
			info, err := os.Stat(file)
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}
			if info.IsDir() {
				return usagef(a.cmd, "%s is a directory", file)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}
			// The server checks for the Postgres custom-format magic itself;
			// checking here turns a 422 after a 300MB upload into an
			// immediate, obvious answer.
			if len(data) == 0 {
				return fmt.Errorf("%s is empty", file)
			}
			if !bytes.HasPrefix(data, []byte("PGDMP")) {
				return fmt.Errorf("%s is not a Postgres custom-format dump (it should start with PGDMP); a plain .sql file will not restore", file)
			}
			if !force && len(data) < 4096 {
				return fmt.Errorf("%s is only %s, too small to be a real backup; pass --force if you are sure", file, humanBytes(info.Size()))
			}

			// --yes is required, always. This is the one operation in the CLI
			// that can take the company's records away, and an unattended
			// script pointed at production is exactly how that happens by
			// accident.
			if !a.assumeYes {
				return usagef(a.cmd,
					"restoring replaces everything on %s with %s; re-run with --yes to confirm",
					a.apiBase(), file)
			}
			if err := a.confirm("Restore %s into %s? Every current record is replaced.", file, a.apiBase()); err != nil {
				return err
			}

			client, err := a.client()
			if err != nil {
				return err
			}
			var response struct {
				SafetySnapshot  string `json:"safetySnapshot"`
				Quotations      int    `json:"quotations"`
				Revisions       int    `json:"revisions"`
				RestoreWarnings string `json:"restoreWarnings"`
				AuditWarning    string `json:"auditWarning"`
			}
			a.note("Uploading %s…", humanBytes(info.Size()))
			if err := client.Upload("/v1/admin/backup/restore", "dump", filepath.Base(file), data, &response); err != nil {
				return err
			}

			if err := a.emitJSON(response); err != nil {
				return err
			}
			a.field("Restored", filepath.Base(file))
			a.field("Quotations", fmt.Sprintf("%d", response.Quotations))
			a.field("Revisions", fmt.Sprintf("%d", response.Revisions))
			if response.SafetySnapshot != "" {
				a.field("Safety snapshot", response.SafetySnapshot)
			}
			if response.RestoreWarnings != "" {
				a.warn("pg_restore reported warnings:\n%s", response.RestoreWarnings)
			}
			if response.AuditWarning != "" {
				a.warn("%s", response.AuditWarning)
			}
			return nil
		},
	}
}

// humanBytes keeps a 480MB upload from reading as "503316480 bytes".
func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TB", value/unit)
}
