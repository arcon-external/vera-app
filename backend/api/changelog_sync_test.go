package api

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChangelogCopyMatchesRoot pins backend/api/changelog.md (the copy the
// /v1/app/changelog endpoint embeds) to the canonical root CHANGELOG.md.
// Release process: edit the root file, copy it over, commit both. Drift
// fails here instead of shipping stale notes to the What's New modal.
func TestChangelogCopyMatchesRoot(t *testing.T) {
	embedded, err := os.ReadFile(filepath.Join("changelog.md"))
	if err != nil {
		t.Fatalf("read embedded copy: %v", err)
	}
	root, err := os.ReadFile(filepath.Join("..", "..", "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read root CHANGELOG.md: %v", err)
	}
	if string(embedded) != string(root) {
		t.Error("backend/api/changelog.md differs from root CHANGELOG.md; copy it over")
	}
}
