package store

import (
	"strings"
	"testing"
)

func TestLoadMigrationsReplacesSchemaPlaceholder(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("migration count = %d; want 1", len(migrations))
	}
	if migrations[0].version != 1 {
		t.Errorf("migration version = %d; want 1", migrations[0].version)
	}
	if strings.Contains(migrations[0].sql, "{{schema}}") {
		t.Error("migration still contains an unresolved schema placeholder")
	}
	if !strings.Contains(migrations[0].sql, `"vera_test".quotation_revisions`) {
		t.Error("migration does not qualify quotation revisions with the configured schema")
	}
}

func TestInitialMigrationContainsRevisionIntegrityConstraints(t *testing.T) {
	migrations, err := loadMigrations("vera_test")
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	sql := migrations[0].sql
	for _, expected := range []string{
		"UNIQUE (quotation_id, revision_number)",
		"UNIQUE (revision_id, item_no)",
		"marginalized_grand_after_tax BIGINT",
		"quotation_documents",
		"sequence_counters",
	} {
		if !strings.Contains(sql, expected) {
			t.Errorf("migration missing %q", expected)
		}
	}
}
